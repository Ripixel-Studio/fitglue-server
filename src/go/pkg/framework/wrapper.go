// nolint:proto-json
package framework

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"github.com/cloudevents/sdk-go/v2/event"
	"github.com/fitglue/server/src/go/pkg/bootstrap"
	"github.com/fitglue/server/src/go/pkg/execution"

	sentryPkg "github.com/fitglue/server/src/go/pkg/infrastructure/sentry"
	"github.com/fitglue/server/src/go/pkg/types"

	pbpipeline "github.com/fitglue/server/src/go/pkg/types/pb/models/pipeline"
	"github.com/getsentry/sentry-go"
)

// FrameworkContext contains dependencies injected by the framework
// Similar to TypeScript's FrameworkContext
type FrameworkContext struct {
	Service             *bootstrap.Service
	Logger              *slog.Logger
	ExecutionID         string
	PipelineExecutionId string
}

// HandlerFunc is the signature for a cloud function handler
// Similar to TypeScript's FrameworkHandler
type HandlerFunc func(ctx context.Context, e event.Event, fwCtx *FrameworkContext) (interface{}, error)

// WrapCloudEvent wraps a handler with automatic execution logging
// Handles both HTTP and Pub/Sub triggers
// Similar to TypeScript's createCloudFunction
func WrapCloudEvent(serviceName string, svc *bootstrap.Service, handler HandlerFunc) func(context.Context, event.Event) error {
	return func(ctx context.Context, e event.Event) error {
		// 0. Log execution pending (IMMEDIATELY)
		// We don't have metadata yet, so pass empty options.
		// Note: We use a basic logger or fmt if needed, but LogPending handles DB interaction.
		execID, err := execution.LogPending(ctx, svc.DB, serviceName, execution.ExecutionOptions{})
		if err != nil {
			// We can't log nicely yet as we haven't set up the logger context fully,
			// but we proceed.
			// Ideally fmt.Printf or similar to stderr
		}

		// --- 1. CloudEvent Unwrapping (Pub/Sub Envelope) ---
		// Check if this is a Pub/Sub event wrapping a structured CloudEvent
		if e.Type() == "google.cloud.pubsub.topic.v1.messagePublished" {
			var msg types.PubSubMessage
			if err := e.DataAs(&msg); err == nil && len(msg.Message.Data) > 0 {
				// Try to unmarshal the inner data as a CloudEvent
				var innerEvent event.Event
				if err := json.Unmarshal(msg.Message.Data, &innerEvent); err == nil {
					// Check if it looks valid
					if innerEvent.Type() != "" && innerEvent.Source() != "" {
						e = innerEvent
					}
				}
			}
		}

		// Extract metadata
		var userID string
		var testRunID string
		var pipelineExecutionID string
		var triggerType = e.Type()

		// Try to parse data to find user_id/test_run_id (best effort)
		var rawData map[string]interface{}
		if err := json.Unmarshal(e.Data(), &rawData); err == nil {
			if uid, ok := rawData["user_id"].(string); ok {
				userID = uid
			}
			if uid, ok := rawData["userId"].(string); ok {
				userID = uid
			}
			if tid, ok := rawData["test_run_id"].(string); ok {
				testRunID = tid
			}
			if tid, ok := rawData["testRunId"].(string); ok {
				testRunID = tid
			}
			if peid, ok := rawData["pipeline_execution_id"].(string); ok {
				pipelineExecutionID = peid
			}
			if peid, ok := rawData["pipelineExecutionId"].(string); ok {
				pipelineExecutionID = peid
			}
		}

		// For HTTP requests, or extensions on any event type
		if testRunID == "" {
			extensions := e.Extensions()
			if trid, ok := extensions["test_run_id"].(string); ok {
				testRunID = trid
			}
			if trid, ok := extensions["testrunid"].(string); ok {
				testRunID = trid
			}
		}

		// Extract pipeline_execution_id from extensions
		extensions := e.Extensions()
		if peid, ok := extensions["pipeline_execution_id"].(string); ok {
			pipelineExecutionID = peid
		}

		// If this is the first function (no pipeline_execution_id in event), use our execution ID
		if pipelineExecutionID == "" {
			pipelineExecutionID = execID // This is the root execution
		}

		// Setup Logger
		baseLogger := bootstrap.NewLogger(serviceName, false)
		if testRunID != "" {
			baseLogger = baseLogger.With("test_run_id", testRunID)
		}
		if userID != "" {
			baseLogger = baseLogger.With("user_id", userID)
		}

		// Set up Sentry scope with execution context
		sentry.ConfigureScope(func(scope *sentry.Scope) {
			scope.SetTag("service", serviceName)
			scope.SetTag("trigger_type", e.Type())
			if userID != "" {
				scope.SetUser(sentry.User{ID: userID})
			}
		})

		// Framework Logger (Preamble)
		logger := baseLogger.With("component", "framework")

		// If pending log failed earlier, log it now
		if err != nil {
			logger.Error("Failed to log execution pending", "error", err)
		} else {
			logger.Info("Execution pending logged", "execution_id", execID)
			logger = logger.With("execution_id", execID)
			// Re-apply component after adding ID to ensure it stays (though With keeps it)
		}

		// Extract inputs for logging
		var inputs interface{}
		var rawInputs map[string]interface{}
		if err := e.DataAs(&rawInputs); err == nil {
			inputs = rawInputs
		} else {
			inputs = string(e.Data())
		}

		// Log execution start (handler ready) -> Now includes metadata updates
		startOpts := &execution.ExecutionOptions{
			UserID:              userID,
			TestRunID:           testRunID,
			TriggerType:         triggerType,
			PipelineExecutionID: pipelineExecutionID,
		}
		if err := execution.LogStart(ctx, svc.DB, userID, execID, inputs, startOpts); err != nil {
			logger.Warn("Failed to log execution start", "error", err)
		}
		// Log start with context tag as it denotes start of business logic? No, start of function execution is framework.
		logger.Info("Function started")

		// Create framework context with Context Logger
		fwCtx := &FrameworkContext{
			Service:             svc,
			Logger:              baseLogger.With("execution_id", execID).With("component", "context"),
			ExecutionID:         execID,
			PipelineExecutionId: pipelineExecutionID,
		}

		// Defer panic recovery
		defer func() {
			if r := recover(); r != nil {
				// Log the panic
				logger.Error("Function panicked", "panic", r)

				// Attempt to log execution failure
				// Create an error from the panic
				panicErr := fmt.Errorf("runtime panic: %v", r)

				// Capture in Sentry
				sentryPkg.CaptureException(panicErr, map[string]interface{}{
					"service":      serviceName,
					"execution_id": execID,
					"user_id":      userID,
				}, logger)
				sentryPkg.Flush(2 * time.Second)

				// Use the context we hopefully created, or the parent context if not
				logCtx := ctx
				if logErr := execution.LogFailure(logCtx, svc.DB, userID, execID, panicErr, nil); logErr != nil {
					logger.Warn("Failed to log execution failure after panic", "error", logErr)
				}
			}
		}()

		// Execute handler
		outputs, handlerErr := handler(ctx, e, fwCtx)

		// Log execution result
		if handlerErr != nil {
			// Transient/retryable failures (e.g. upstream activity data still lagging)
			// are an expected, self-healing condition: we NACK below so Pub/Sub
			// redelivers with backoff, but we must NOT report them to Sentry — doing so
			// turns routine retry traffic into a recurring incident (SERVER-7). A
			// message that never stops lagging is bounded by the handler itself (the
			// enricher forces partial enrichment once the event ages past its lag
			// window), so a genuinely stuck activity still surfaces as a real error.
			var retryErr *RetryableError
			if errors.As(handlerErr, &retryErr) {
				// Warn, not Error: SentryHandler only auto-captures Error-level logs,
				// so this keeps a breadcrumb in the logs without alerting Sentry.
				logger.Warn("Function retrying (transient failure)", "error", handlerErr)
			} else {
				logger.Error("Function failed", "error", handlerErr)

				// Capture error in Sentry
				sentryPkg.CaptureException(handlerErr, map[string]interface{}{
					"service":      serviceName,
					"execution_id": execID,
					"user_id":      userID,
					"trigger_type": triggerType,
				}, logger)
			}

			if logErr := execution.LogFailure(ctx, svc.DB, userID, execID, handlerErr, outputs); logErr != nil {
				logger.Warn("Failed to log execution failure", "error", logErr)
			}

			// Flush Sentry before returning
			sentryPkg.Flush(2 * time.Second)

			// Prevent infinite retries for terminal errors by pretending they succeeded for the CloudEvents SDK.
			var termErr *TerminalError
			if errors.As(handlerErr, &termErr) {
				logger.Info("Intercepted TerminalError, dropping event to prevent infinite retries.", "error", handlerErr.Error())
				return nil
			}

			return handlerErr
		}

		logger.Info("Function completed successfully")

		// Check if outputs has a "status" field we should use
		customStatus := ""
		if outputsMap, ok := outputs.(map[string]interface{}); ok {
			if s, ok := outputsMap["status"].(string); ok {
				customStatus = s
			}
		}

		if customStatus != "" {
			var statusEnum pbpipeline.ExecutionStatus
			if val, ok := pbpipeline.ExecutionStatus_value[customStatus]; ok {
				statusEnum = pbpipeline.ExecutionStatus(val)
			} else if val, ok := pbpipeline.ExecutionStatus_value["STATUS_"+strings.ToUpper(customStatus)]; ok {
				statusEnum = pbpipeline.ExecutionStatus(val)
			} else if val, ok := pbpipeline.ExecutionStatus_value[strings.ToUpper(customStatus)]; ok {
				statusEnum = pbpipeline.ExecutionStatus(val)
			} else {
				statusEnum = pbpipeline.ExecutionStatus_STATUS_UNSPECIFIED
				logger.Warn("Unknown custom status returned", "status", customStatus)
			}

			if logErr := execution.LogExecutionStatus(ctx, svc.DB, userID, execID, statusEnum, outputs); logErr != nil {
				logger.Warn("Failed to log execution status", "error", logErr)
			}
		} else {
			if logErr := execution.LogSuccess(ctx, svc.DB, userID, execID, outputs); logErr != nil {
				logger.Warn("Failed to log execution success", "error", logErr)
			}
		}

		// Flush Sentry events before function terminates
		sentryPkg.Flush(2 * time.Second)

		return nil
	}
}
