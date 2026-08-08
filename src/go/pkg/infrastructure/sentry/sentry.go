package sentry

import (
	"context"
	"fmt"
	"log/slog"
	"reflect"
	"time"

	"github.com/getsentry/sentry-go"
)

type Config struct {
	DSN                string
	Environment        string
	Release            string
	ServerName         string
	TracesSampleRate   float64
	ProfilesSampleRate float64

	// Transport overrides the Sentry transport. Left nil in production (the SDK
	// picks its default HTTP transport); set by tests to capture events.
	Transport sentry.Transport
}

// pkgPath is the import path of this instrumentation package. Every error in
// FitGlue is captured to Sentry through CaptureException / SentryHandler.Handle
// in this package, and none of our error types (*fmt.wrapError, *FitGlueError)
// carry their own stack trace, so sentry-go synthesises one at capture time.
// That synthetic stack is rooted here, which made this package the reported
// "culprit" for every error and collapsed unrelated failures into a single
// issue. We strip our own frames in beforeSend so the culprit is the real
// application call site. See stripInstrumentationFrames.
var pkgPath = reflect.TypeOf(Config{}).PkgPath()

// Init initializes Sentry for Go Cloud Functions.
// Safe to call multiple times - will only initialize once.
func Init(cfg Config, logger *slog.Logger) error {
	if cfg.DSN == "" {
		if logger != nil {
			logger.Warn("Sentry DSN not configured - error tracking disabled")
		}
		return nil
	}

	err := sentry.Init(sentry.ClientOptions{
		Dsn:                cfg.DSN,
		Environment:        cfg.Environment,
		Release:            cfg.Release,
		ServerName:         cfg.ServerName,
		TracesSampleRate:   cfg.TracesSampleRate,
		ProfilesSampleRate: cfg.ProfilesSampleRate,
		Transport:          cfg.Transport,
		BeforeSend:         beforeSend,
	})

	if err != nil {
		if logger != nil {
			logger.Error("Failed to initialize Sentry", "error", err)
		}
		return fmt.Errorf("sentry init: %w", err)
	}

	if logger != nil {
		logger.Info("Sentry initialized", "environment", cfg.Environment, "release", cfg.Release)
	}

	return nil
}

// beforeSend runs on every outgoing event. It scrubs sensitive request headers
// and strips this package's instrumentation frames from stack traces so Sentry
// attributes each error to its real call site instead of to CaptureException.
func beforeSend(event *sentry.Event, _ *sentry.EventHint) *sentry.Event {
	if event == nil {
		return event
	}

	// Filter out sensitive data.
	if event.Request != nil && event.Request.Headers != nil {
		delete(event.Request.Headers, "Authorization")
		delete(event.Request.Headers, "Cookie")
	}

	stripInstrumentationFrames(event)
	return event
}

// stripInstrumentationFrames removes frames belonging to this package from every
// exception's stack trace. These frames (CaptureException, SentryHandler.Handle,
// RecoverAndCapture) are the same for every captured error, so leaving them in
// makes this package the reported culprit and prevents Sentry from telling
// distinct failures apart. The slog frames beneath them are already flagged
// not-in-app by sentry-go, so once our frames are gone the youngest in-app frame
// is the real application code that failed.
func stripInstrumentationFrames(event *sentry.Event) {
	for i := range event.Exception {
		st := event.Exception[i].Stacktrace
		if st == nil {
			continue
		}
		filtered := make([]sentry.Frame, 0, len(st.Frames))
		for _, f := range st.Frames {
			if f.Module == pkgPath {
				continue
			}
			filtered = append(filtered, f)
		}
		// Never emit an empty stack trace: if an error somehow originated
		// entirely within this package, keep the frames we have.
		if len(filtered) > 0 {
			st.Frames = filtered
		}
	}
}

// CaptureException captures an exception in Sentry with additional context.
func CaptureException(err error, context map[string]interface{}, logger *slog.Logger) {
	if err == nil {
		return
	}

	if context != nil {
		sentry.ConfigureScope(func(scope *sentry.Scope) {
			for key, value := range context {
				scope.SetContext(key, sentry.Context(map[string]interface{}{
					"value": value,
				}))
			}
		})
	}

	sentry.CaptureException(err)

	if logger != nil {
		logger.Debug("Exception captured in Sentry", "error", err.Error())
	}
}

// CaptureMessage captures a message in Sentry.
func CaptureMessage(message string, level sentry.Level, context map[string]interface{}, logger *slog.Logger) {
	if context != nil {
		sentry.ConfigureScope(func(scope *sentry.Scope) {
			for key, value := range context {
				scope.SetContext(key, sentry.Context(map[string]interface{}{
					"value": value,
				}))
			}
		})
	}

	sentry.CaptureMessage(message)

	if logger != nil {
		logger.Debug("Message captured in Sentry", "message", message, "level", level)
	}
}

// Flush waits for all events to be sent to Sentry.
// Call this before function termination to ensure events are sent.
func Flush(timeout time.Duration) bool {
	return sentry.Flush(timeout)
}

// RecoverAndCapture recovers from a panic and captures it in Sentry.
func RecoverAndCapture(logger *slog.Logger) {
	if r := recover(); r != nil {
		err, ok := r.(error)
		if !ok {
			err = fmt.Errorf("panic: %v", r)
		}
		CaptureException(err, nil, logger)
		Flush(2 * time.Second)
		panic(r) // Re-panic after capturing
	}
}

// SentryHandler wraps a slog.Handler to automatically capture Error-level logs to Sentry.
// This ensures all logger.Error() calls are automatically reported without manual intervention.
type SentryHandler struct {
	slog.Handler
}

// NewSentryHandler creates a new SentryHandler wrapping the provided handler.
func NewSentryHandler(h slog.Handler) *SentryHandler {
	return &SentryHandler{Handler: h}
}

// Handle implements slog.Handler. For Error-level logs, it captures the message to Sentry.
func (h *SentryHandler) Handle(ctx context.Context, r slog.Record) error {
	// Capture Error-level logs to Sentry
	if r.Level >= slog.LevelError {
		// Build context from attributes
		context := make(map[string]interface{})
		r.Attrs(func(a slog.Attr) bool {
			context[a.Key] = a.Value.Any()
			return true
		})

		// Check if an error attribute exists
		if errVal, ok := context["error"]; ok {
			if err, isErr := errVal.(error); isErr {
				CaptureException(err, context, nil)
			} else {
				// Error is a string or other type
				sentry.CaptureMessage(fmt.Sprintf("%s: %v", r.Message, errVal))
			}
		} else {
			// No error attribute, capture as message
			sentry.CaptureMessage(r.Message)
		}
	}

	// Always delegate to the wrapped handler
	return h.Handler.Handle(ctx, r)
}

// WithGroup implements slog.Handler
func (h *SentryHandler) WithGroup(name string) slog.Handler {
	return &SentryHandler{Handler: h.Handler.WithGroup(name)}
}

// WithAttrs implements slog.Handler
func (h *SentryHandler) WithAttrs(attrs []slog.Attr) slog.Handler {
	return &SentryHandler{Handler: h.Handler.WithAttrs(attrs)}
}

// Enabled implements slog.Handler
func (h *SentryHandler) Enabled(ctx context.Context, level slog.Level) bool {
	return h.Handler.Enabled(ctx, level)
}
