// nolint:proto-json
package enricher

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	cloudevents "github.com/cloudevents/sdk-go/v2"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/encoding/protojson"
	"google.golang.org/protobuf/types/known/timestamppb"

	"github.com/fitglue/server/src/go/internal/pipeline/enricher/providers"
	"github.com/fitglue/server/src/go/internal/pipeline/enricher/providers/user_input"
	"github.com/fitglue/server/src/go/pkg/bootstrap"
	pbactivity "github.com/fitglue/server/src/go/pkg/types/pb/models/activity"
	pbevents "github.com/fitglue/server/src/go/pkg/types/pb/models/events"
	pbpipeline "github.com/fitglue/server/src/go/pkg/types/pb/models/pipeline"
	pbplugin "github.com/fitglue/server/src/go/pkg/types/pb/models/plugin"

	"github.com/fitglue/server/src/go/pkg/testing/mocks"
)

func covLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(io.Discard, nil))
}

func strPtr(s string) *string { return &s }

// --- resolvePipeline ---

func TestResolvePipeline_Coverage(t *testing.T) {
	logger := covLogger()

	t.Run("DatabaseError", func(t *testing.T) {
		db := &mocks.MockDatabase{
			GetUserPipelinesFunc: func(ctx context.Context, userId string) ([]*pbpipeline.PipelineConfig, error) {
				return nil, errors.New("boom")
			},
		}
		o := NewOrchestrator(db, nil, "bucket", nil)
		got, err := o.resolvePipeline(context.Background(), "p1", "u1", logger)
		assert.Error(t, err)
		assert.Nil(t, got)
	})

	t.Run("Disabled", func(t *testing.T) {
		db := &mocks.MockDatabase{
			GetUserPipelinesFunc: func(ctx context.Context, userId string) ([]*pbpipeline.PipelineConfig, error) {
				return []*pbpipeline.PipelineConfig{{Id: "p1", Disabled: true}}, nil
			},
		}
		o := NewOrchestrator(db, nil, "bucket", nil)
		got, err := o.resolvePipeline(context.Background(), "p1", "u1", logger)
		assert.NoError(t, err)
		assert.Nil(t, got)
	})

	t.Run("NotFound", func(t *testing.T) {
		db := &mocks.MockDatabase{
			GetUserPipelinesFunc: func(ctx context.Context, userId string) ([]*pbpipeline.PipelineConfig, error) {
				return []*pbpipeline.PipelineConfig{{Id: "other"}}, nil
			},
		}
		o := NewOrchestrator(db, nil, "bucket", nil)
		got, err := o.resolvePipeline(context.Background(), "p1", "u1", logger)
		assert.NoError(t, err)
		assert.Nil(t, got)
	})

	t.Run("PrefersSourcesArrayOverLegacySource", func(t *testing.T) {
		db := &mocks.MockDatabase{
			GetUserPipelinesFunc: func(ctx context.Context, userId string) ([]*pbpipeline.PipelineConfig, error) {
				return []*pbpipeline.PipelineConfig{{
					Id:           "p1",
					Source:       "SOURCE_LEGACY",
					Sources:      []string{"SOURCE_HEVY"},
					Destinations: []pbplugin.DestinationType{pbplugin.DestinationType_DESTINATION_STRAVA},
					Enrichers: []*pbpipeline.EnricherConfig{
						{ProviderType: pbplugin.EnricherProviderType_ENRICHER_PROVIDER_MOCK, NonBlocking: true},
					},
				}}, nil
			},
		}
		o := NewOrchestrator(db, nil, "bucket", nil)
		got, err := o.resolvePipeline(context.Background(), "p1", "u1", logger)
		require.NoError(t, err)
		require.NotNil(t, got)
		assert.Equal(t, "SOURCE_HEVY", got.Source)
		require.Len(t, got.Enrichers, 1)
		assert.True(t, got.Enrichers[0].NonBlocking)
	})

	t.Run("FallsBackToLegacySource", func(t *testing.T) {
		db := &mocks.MockDatabase{
			GetUserPipelinesFunc: func(ctx context.Context, userId string) ([]*pbpipeline.PipelineConfig, error) {
				return []*pbpipeline.PipelineConfig{{Id: "p1", Source: "SOURCE_LEGACY"}}, nil
			},
		}
		o := NewOrchestrator(db, nil, "bucket", nil)
		got, err := o.resolvePipeline(context.Background(), "p1", "u1", logger)
		require.NoError(t, err)
		require.NotNil(t, got)
		assert.Equal(t, "SOURCE_LEGACY", got.Source)
	})
}

func basicPayload() *pbevents.ActivityPayload {
	return &pbevents.ActivityPayload{
		UserId:     "u1",
		Source:     pbactivity.ActivitySource_SOURCE_HEVY,
		PipelineId: strPtr("p1"),
		StandardizedActivity: &pbactivity.StandardizedActivity{
			Name:      "Morning Run",
			Type:      pbactivity.ActivityType_ACTIVITY_TYPE_RUN,
			StartTime: timestamppb.New(time.Now()),
			Source:    pbactivity.ActivitySource_SOURCE_HEVY,
		},
	}
}

// --- createNonBlockingPendingInput ---

func TestCreateNonBlockingPendingInput_Coverage(t *testing.T) {
	logger := covLogger()

	t.Run("ExistingReturnsActivityID", func(t *testing.T) {
		db := &mocks.MockDatabase{
			GetPendingInputFunc: func(ctx context.Context, userId, id string) (*pbpipeline.PendingInput, error) {
				return &pbpipeline.PendingInput{ActivityId: id}, nil
			},
		}
		o := NewOrchestrator(db, nil, "bucket", nil)
		waitErr := &user_input.WaitForInputError{ActivityID: "act-1", EnricherProviderID: "user_input"}
		got := o.createNonBlockingPendingInput(context.Background(), logger, basicPayload(), waitErr, "linked", "")
		assert.Equal(t, "act-1", got)
	})

	t.Run("CreatesWithUploadedPayload", func(t *testing.T) {
		var written bool
		db := &mocks.MockDatabase{}
		store := &mocks.MockBlobStore{
			WriteFunc: func(ctx context.Context, bucket, object string, data []byte) error {
				written = true
				return nil
			},
		}
		o := NewOrchestrator(db, store, "bucket", nil)
		waitErr := &user_input.WaitForInputError{
			ActivityID:         "act-2",
			EnricherProviderID: "user_input",
			RequiredFields:     []string{"fit_file"},
			Metadata:           map[string]string{"k": "v"},
		}
		got := o.createNonBlockingPendingInput(context.Background(), logger, basicPayload(), waitErr, "linked", "")
		assert.Equal(t, "act-2", got)
		assert.True(t, written, "expected payload upload when no originalPayloadUri")
	})

	t.Run("CreateFailsReturnsEmpty", func(t *testing.T) {
		db := &errCreatePendingDB{}
		o := NewOrchestrator(db, nil, "", nil)
		waitErr := &user_input.WaitForInputError{ActivityID: "act-3", EnricherProviderID: "user_input"}
		got := o.createNonBlockingPendingInput(context.Background(), logger, basicPayload(), waitErr, "", "gs://existing/uri")
		assert.Equal(t, "", got)
	})
}

// errCreatePendingDB embeds mocks.MockDatabase but forces CreatePendingInput to fail.
type errCreatePendingDB struct {
	mocks.MockDatabase
}

func (e *errCreatePendingDB) CreatePendingInput(ctx context.Context, userId string, input *pbpipeline.PendingInput) error {
	return errors.New("create failed")
}

// --- handleWaitError ---

func TestHandleWaitError_Coverage(t *testing.T) {
	logger := covLogger()
	execs := []ProviderExecution{{ProviderName: "user_input", Status: "WAITING"}}

	t.Run("ExistingCompletedShortCircuits", func(t *testing.T) {
		db := &mocks.MockDatabase{
			GetPendingInputFunc: func(ctx context.Context, userId, id string) (*pbpipeline.PendingInput, error) {
				return &pbpipeline.PendingInput{Status: pbpipeline.PendingInput_STATUS_COMPLETED}, nil
			},
		}
		o := NewOrchestrator(db, nil, "bucket", nil)
		waitErr := &user_input.WaitForInputError{ActivityID: "act-1"}
		res, err := o.handleWaitError(context.Background(), logger, basicPayload(), execs, waitErr, "linked", "gs://uri")
		require.NoError(t, err)
		assert.Equal(t, pbpipeline.ExecutionStatus_STATUS_WAITING, res.Status)
		assert.Empty(t, res.Events)
	})

	t.Run("ExistingWaitingResendsNotification", func(t *testing.T) {
		var published bool
		db := &mocks.MockDatabase{
			GetPendingInputFunc: func(ctx context.Context, userId, id string) (*pbpipeline.PendingInput, error) {
				return &pbpipeline.PendingInput{Status: pbpipeline.PendingInput_STATUS_WAITING}, nil
			},
		}
		pub := &mocks.MockPublisher{
			PublishJSONFunc: func(ctx context.Context, topic string, data []byte) error {
				published = true
				return nil
			},
		}
		o := NewOrchestrator(db, nil, "bucket", pub)
		waitErr := &user_input.WaitForInputError{ActivityID: "act-1"}
		res, err := o.handleWaitError(context.Background(), logger, basicPayload(), execs, waitErr, "linked", "gs://uri")
		require.NoError(t, err)
		assert.Equal(t, pbpipeline.ExecutionStatus_STATUS_WAITING, res.Status)
		assert.True(t, published, "expected notification re-send for waiting input")
	})

	t.Run("CreatesNewWithFallbackUpload", func(t *testing.T) {
		var written bool
		db := &mocks.MockDatabase{}
		store := &mocks.MockBlobStore{
			WriteFunc: func(ctx context.Context, bucket, object string, data []byte) error {
				written = true
				return nil
			},
		}
		pub := &mocks.MockPublisher{
			PublishJSONFunc: func(ctx context.Context, topic string, data []byte) error {
				return nil
			},
		}
		o := NewOrchestrator(db, store, "bucket", pub)
		waitErr := &user_input.WaitForInputError{
			ActivityID:     "act-new",
			RequiredFields: []string{"fit_file"},
			Metadata:       map[string]string{"display.summary": "Upload FIT"},
		}
		res, err := o.handleWaitError(context.Background(), logger, basicPayload(), execs, waitErr, "linked", "")
		require.NoError(t, err)
		assert.Equal(t, pbpipeline.ExecutionStatus_STATUS_WAITING, res.Status)
		assert.True(t, written, "expected fallback payload upload when no URI")
	})
}

// --- sendPendingInputNotification ---

func TestSendPendingInputNotification_Coverage(t *testing.T) {
	logger := covLogger()

	t.Run("NilPublisherNoPanic", func(t *testing.T) {
		o := NewOrchestrator(&mocks.MockDatabase{}, nil, "bucket", nil)
		assert.NotPanics(t, func() {
			o.sendPendingInputNotification(context.Background(), logger, "u1", "act-1")
		})
	})

	t.Run("PublishesNotification", func(t *testing.T) {
		var topic string
		pub := &mocks.MockPublisher{
			PublishJSONFunc: func(ctx context.Context, t string, data []byte) error {
				topic = t
				return nil
			},
		}
		o := NewOrchestrator(&mocks.MockDatabase{}, nil, "bucket", pub)
		o.sendPendingInputNotification(context.Background(), logger, "u1", "act-1")
		assert.NotEmpty(t, topic)
	})

	t.Run("PublishErrorLogged", func(t *testing.T) {
		pub := &mocks.MockPublisher{
			PublishJSONFunc: func(ctx context.Context, t string, data []byte) error {
				return errors.New("pub fail")
			},
		}
		o := NewOrchestrator(&mocks.MockDatabase{}, nil, "bucket", pub)
		assert.NotPanics(t, func() {
			o.sendPendingInputNotification(context.Background(), logger, "u1", "act-1")
		})
	})
}

// --- enqueuePipelineFailureNotification ---

func TestEnqueuePipelineFailureNotification_Coverage(t *testing.T) {
	logger := covLogger()

	t.Run("NilPublisher", func(t *testing.T) {
		o := NewOrchestrator(&mocks.MockDatabase{}, nil, "bucket", nil)
		assert.NotPanics(t, func() {
			o.enqueuePipelineFailureNotification(logger, context.Background(), "u1", "act-1", "Run", "branding")
		})
	})

	t.Run("Publishes", func(t *testing.T) {
		var published bool
		pub := &mocks.MockPublisher{
			PublishJSONFunc: func(ctx context.Context, t string, data []byte) error {
				published = true
				return nil
			},
		}
		o := NewOrchestrator(&mocks.MockDatabase{}, nil, "bucket", pub)
		o.enqueuePipelineFailureNotification(logger, context.Background(), "u1", "act-1", "Run", "branding")
		assert.True(t, published)
	})

	t.Run("PublishErrorLogged", func(t *testing.T) {
		pub := &mocks.MockPublisher{
			PublishJSONFunc: func(ctx context.Context, t string, data []byte) error {
				return errors.New("fail")
			},
		}
		o := NewOrchestrator(&mocks.MockDatabase{}, nil, "bucket", pub)
		assert.NotPanics(t, func() {
			o.enqueuePipelineFailureNotification(logger, context.Background(), "u1", "act-1", "Run", "branding")
		})
	})
}

// --- createInitialPipelineRun ---

func TestCreateInitialPipelineRun_Coverage(t *testing.T) {
	logger := covLogger()
	payload := basicPayload()
	payload.StandardizedActivity.Sessions = []*pbactivity.Session{{StartTime: timestamppb.New(time.Now())}}
	dests := []pbplugin.DestinationType{pbplugin.DestinationType_DESTINATION_STRAVA}

	t.Run("ResumeModeOnlyResetsStatus", func(t *testing.T) {
		o := NewOrchestrator(&mocks.MockDatabase{}, nil, "bucket", nil)
		assert.NotPanics(t, func() {
			o.createInitialPipelineRun(context.Background(), logger, "u1", "exec-1", "p1", "act-1", payload, dests, true)
		})
	})

	t.Run("FreshCreatesRunAndOutcomes", func(t *testing.T) {
		o := NewOrchestrator(&mocks.MockDatabase{}, nil, "bucket", nil)
		assert.NotPanics(t, func() {
			o.createInitialPipelineRun(context.Background(), logger, "u1", "exec-1", "p1", "act-1", payload, dests, false)
		})
	})
}

// --- updatePipelineRunStatus ---

func TestUpdatePipelineRunStatus_Coverage(t *testing.T) {
	logger := covLogger()
	execs := []ProviderExecution{{ProviderName: "hr_zones", Status: "SUCCESS", DurationMs: 10}}

	t.Run("WithMessageAndExecs", func(t *testing.T) {
		var captured map[string]interface{}
		db := &updateCaptureDB{onUpdate: func(data map[string]interface{}) { captured = data }}
		o := NewOrchestrator(db, nil, "bucket", nil)
		o.updatePipelineRunStatus(context.Background(), logger, "u1", "run-1",
			pbpipeline.PipelineRunStatus_PIPELINE_RUN_STATUS_FAILED, "it failed", execs)
		require.NotNil(t, captured)
		assert.Equal(t, "it failed", captured["status_message"])
		assert.NotNil(t, captured["steps"])
	})

	t.Run("GateSkippedNoExecs", func(t *testing.T) {
		o := NewOrchestrator(&mocks.MockDatabase{}, nil, "bucket", nil)
		assert.NotPanics(t, func() {
			o.updatePipelineRunStatus(context.Background(), logger, "u1", "run-1",
				pbpipeline.PipelineRunStatus_PIPELINE_RUN_STATUS_SKIPPED, "", nil)
		})
	})

	t.Run("UpdateErrorLogged", func(t *testing.T) {
		o := NewOrchestrator(&updateErrDB{}, nil, "bucket", nil)
		assert.NotPanics(t, func() {
			o.updatePipelineRunStatus(context.Background(), logger, "u1", "run-1",
				pbpipeline.PipelineRunStatus_PIPELINE_RUN_STATUS_RUNNING, "", execs)
		})
	})
}

type updateCaptureDB struct {
	mocks.MockDatabase
	onUpdate func(map[string]interface{})
}

func (u *updateCaptureDB) UpdatePipelineRun(ctx context.Context, userId, id string, data map[string]interface{}) error {
	if u.onUpdate != nil {
		u.onUpdate(data)
	}
	return nil
}

type updateErrDB struct {
	mocks.MockDatabase
}

func (u *updateErrDB) UpdatePipelineRun(ctx context.Context, userId, id string, data map[string]interface{}) error {
	return errors.New("update failed")
}

// --- finalizePipelineRun ---

func TestFinalizePipelineRun_Coverage(t *testing.T) {
	logger := covLogger()
	event := &pbevents.EnrichedActivityEvent{
		UserId:              "u1",
		ActivityId:          "act-1",
		PipelineExecutionId: strPtr("exec-1"),
		Name:                "Run",
		Description:         "desc",
		ActivityType:        pbactivity.ActivityType_ACTIVITY_TYPE_RUN,
		StartTime:           timestamppb.New(time.Now()),
	}
	execs := []ProviderExecution{{ProviderName: "hr_zones", Status: "SUCCESS"}}

	t.Run("Success", func(t *testing.T) {
		o := NewOrchestrator(&mocks.MockDatabase{}, nil, "bucket", nil)
		assert.NotPanics(t, func() {
			o.finalizePipelineRun(context.Background(), logger, "u1", event, execs, "gs://uri")
		})
	})

	t.Run("UpdateErrorLogged", func(t *testing.T) {
		o := NewOrchestrator(&updateErrDB{}, nil, "bucket", nil)
		assert.NotPanics(t, func() {
			o.finalizePipelineRun(context.Background(), logger, "u1", event, execs, "gs://uri")
		})
	})
}

// --- buildAllSteps / buildEnricherBatchStep / pipelineRunStatusToStepStatus ---

func TestBuildAllSteps_Coverage(t *testing.T) {
	t.Run("GateSkipped", func(t *testing.T) {
		steps := buildAllSteps("run", nil, true, false)
		assert.Len(t, steps, 3) // source, parse, gate(skip)
		assert.Equal(t, int32(pbpipeline.ExecutionStepKind_EXECUTION_STEP_KIND_GATE), steps[2]["kind"])
	})

	t.Run("EnricherFailedNoExecs", func(t *testing.T) {
		steps := buildAllSteps("run", nil, false, true)
		// source, parse, gate(failed) — no enricher batch since no execs
		assert.Len(t, steps, 3)
	})

	t.Run("WithExecsSuccessAddsRouter", func(t *testing.T) {
		execs := []ProviderExecution{{ProviderName: "a", Status: "SUCCESS"}}
		steps := buildAllSteps("run", execs, false, false)
		// source, parse, gate, enricher batch, router
		assert.Len(t, steps, 5)
	})

	t.Run("WithExecsFailedNoRouter", func(t *testing.T) {
		execs := []ProviderExecution{{ProviderName: "a", Status: "FAILED"}}
		steps := buildAllSteps("run", execs, false, true)
		// source, parse, gate, enricher batch (no router on failure)
		assert.Len(t, steps, 4)
	})
}

func TestBuildEnricherBatchStep_Coverage(t *testing.T) {
	t.Run("AllSuccess", func(t *testing.T) {
		execs := []ProviderExecution{
			{Status: "SUCCESS", DurationMs: 5},
			{Status: "SUCCESS", DurationMs: 7},
		}
		step := buildEnricherBatchStep("run", execs, pbpipeline.ExecutionStepStatus_EXECUTION_STEP_STATUS_OK)
		assert.Equal(t, int64(12), step["duration_ms"])
		assert.Equal(t, "✓ 2/2", step["status_label"])
	})

	t.Run("SomeFailed", func(t *testing.T) {
		execs := []ProviderExecution{
			{Status: "SUCCESS"},
			{Status: "FAILED"},
		}
		step := buildEnricherBatchStep("run", execs, pbpipeline.ExecutionStepStatus_EXECUTION_STEP_STATUS_FAILED)
		assert.Equal(t, "⚠ 1/2", step["status_label"])
	})
}

func TestPipelineRunStatusToStepStatus_Coverage(t *testing.T) {
	cases := []struct {
		in   pbpipeline.PipelineRunStatus
		want pbpipeline.ExecutionStepStatus
	}{
		{pbpipeline.PipelineRunStatus_PIPELINE_RUN_STATUS_FAILED, pbpipeline.ExecutionStepStatus_EXECUTION_STEP_STATUS_FAILED},
		{pbpipeline.PipelineRunStatus_PIPELINE_RUN_STATUS_SKIPPED, pbpipeline.ExecutionStepStatus_EXECUTION_STEP_STATUS_SKIPPED},
		{pbpipeline.PipelineRunStatus_PIPELINE_RUN_STATUS_PENDING, pbpipeline.ExecutionStepStatus_EXECUTION_STEP_STATUS_QUEUED},
		{pbpipeline.PipelineRunStatus_PIPELINE_RUN_STATUS_SYNCED_WITH_PENDING, pbpipeline.ExecutionStepStatus_EXECUTION_STEP_STATUS_OK},
		{pbpipeline.PipelineRunStatus_PIPELINE_RUN_STATUS_RUNNING, pbpipeline.ExecutionStepStatus_EXECUTION_STEP_STATUS_RETRIED},
	}
	for _, c := range cases {
		assert.Equal(t, c.want, pipelineRunStatusToStepStatus(c.in))
	}
}

// --- buildBoosterMetadata extra branches ---

func TestBuildBoosterMetadata_IdempotentReturnsPlainMetadata(t *testing.T) {
	res := &providers.EnrichmentResult{Metadata: map[string]string{"a": "b"}}
	// MockProvider is NOT a NonIdempotentProvider → branch returns plain metadata.
	m := buildBoosterMetadata(res, &MockProvider{NameFunc: func() string { return "x" }})
	assert.Equal(t, "b", m["a"])
	_, hasReplay := m["replay_completed"]
	assert.False(t, hasReplay)
}

func TestBuildBoosterMetadata_NonIdempotentAllFields(t *testing.T) {
	res := &providers.EnrichmentResult{
		Name:         "Run",
		NameSuffix:   " (#5)",
		Description:  "  desc  ",
		ActivityType: pbactivity.ActivityType_ACTIVITY_TYPE_RUN,
		Tags:         []string{"race", "pb"},
		Metadata:     map[string]string{"k": "v"},
	}
	m := buildBoosterMetadata(res, &fakeNonIdempotentProvider{})
	assert.Equal(t, "true", m["replay_completed"])
	assert.Equal(t, " (#5)", m["replay_name_suffix"])
	assert.Equal(t, "Run", m["replay_name"])
	assert.Equal(t, "desc", m["replay_description"])
	assert.NotEmpty(t, m["replay_activity_type"])
	var tags []string
	require.NoError(t, json.Unmarshal([]byte(m["replay_tags"]), &tags))
	assert.Equal(t, []string{"race", "pb"}, tags)
}

// --- mergeEnrichments edge cases ---

func TestMergeEnrichments_Coverage(t *testing.T) {
	t.Run("NilSrcReturnsDst", func(t *testing.T) {
		dst := &pbactivity.ActivityEnrichments{}
		got := mergeEnrichments(dst, nil)
		assert.Same(t, dst, got)
	})

	t.Run("NilDstCreatesNew", func(t *testing.T) {
		src := &pbactivity.ActivityEnrichments{AiSummary: &pbactivity.AiSummary{Html: "x"}}
		got := mergeEnrichments(nil, src)
		require.NotNil(t, got)
		assert.Equal(t, "x", got.GetAiSummary().GetHtml())
	})
}

// --- parseCloudEventFromPubSubPush ---

func makePushRequest(t *testing.T, body []byte) *http.Request {
	t.Helper()
	return httptest.NewRequest(http.MethodPost, "/", bytes.NewReader(body))
}

func TestParseCloudEventFromPubSubPush_Coverage(t *testing.T) {
	t.Run("InvalidJSON", func(t *testing.T) {
		_, err := parseCloudEventFromPubSubPush(makePushRequest(t, []byte("not json")))
		assert.Error(t, err)
	})

	t.Run("NoData", func(t *testing.T) {
		body, _ := json.Marshal(map[string]interface{}{
			"message":      map[string]interface{}{"messageId": "m1"},
			"subscription": "sub",
		})
		_, err := parseCloudEventFromPubSubPush(makePushRequest(t, body))
		assert.Error(t, err)
	})

	t.Run("DataIsCloudEvent", func(t *testing.T) {
		ce := cloudevents.NewEvent()
		ce.SetID("id-1")
		ce.SetType("com.fitglue.test")
		ce.SetSource("/src")
		_ = ce.SetData(cloudevents.ApplicationJSON, map[string]string{"k": "v"})
		ceBytes, _ := json.Marshal(ce)
		body, _ := json.Marshal(map[string]interface{}{
			"message":      map[string]interface{}{"data": ceBytes, "messageId": "m1"},
			"subscription": "sub",
		})
		ev, err := parseCloudEventFromPubSubPush(makePushRequest(t, body))
		require.NoError(t, err)
		assert.Equal(t, "com.fitglue.test", ev.Type())
	})

	t.Run("DataIsRawPayloadWrapped", func(t *testing.T) {
		raw := []byte(`{"userId":"u1"}`)
		body, _ := json.Marshal(map[string]interface{}{
			"message": map[string]interface{}{
				"data":       raw,
				"messageId":  "m1",
				"attributes": map[string]string{"origin": "lag-queue"},
			},
			"subscription": "sub",
		})
		ev, err := parseCloudEventFromPubSubPush(makePushRequest(t, body))
		require.NoError(t, err)
		assert.Equal(t, "m1", ev.ID())
		ext := ev.Extensions()
		assert.Equal(t, "lag-queue", ext["origin"])
	})
}

// --- isRetryable ---

func TestIsRetryable_Coverage(t *testing.T) {
	assert.True(t, isRetryable(&providers.RetryableError{Reason: "lag"}))
	assert.False(t, isRetryable(errors.New("plain")))
}

// --- EnrichActivityHTTP ---

func TestEnrichActivityHTTP_Coverage(t *testing.T) {
	// Inject a non-nil service so initService short-circuits (svc != nil) and never
	// calls bootstrap.NewService, which needs live GCP clients.
	t.Run("ParseFailureReturns400", func(t *testing.T) {
		svc = &bootstrap.Service{
			DB:     &mocks.MockDatabase{},
			Pub:    &mocks.MockPublisher{},
			Store:  &mocks.MockBlobStore{},
			Config: &bootstrap.Config{ProjectID: "test"},
		}
		defer func() { svc = nil }()

		// A body that is neither a CloudEvents structured/binary request nor a
		// valid Pub/Sub push wrapper → both parse attempts fail → HTTP 400.
		req := httptest.NewRequest(http.MethodPost, "/", bytes.NewReader([]byte("garbage")))
		rec := httptest.NewRecorder()
		EnrichActivityHTTP(rec, req)
		assert.Equal(t, http.StatusBadRequest, rec.Code)
	})

	t.Run("ValidPubSubPushDispatchesToHandler", func(t *testing.T) {
		providers.ClearRegistry()
		defer providers.ClearRegistry()

		svc = &bootstrap.Service{
			DB: &mocks.MockDatabase{
				GetUserPipelinesFunc: func(ctx context.Context, userId string) ([]*pbpipeline.PipelineConfig, error) {
					return nil, nil // no pipelines → handler returns SKIPPED, no error
				},
			},
			Pub:    &mocks.MockPublisher{},
			Store:  &mocks.MockBlobStore{},
			Config: &bootstrap.Config{ProjectID: "test"},
		}
		defer func() { svc = nil }()

		payload := &pbevents.ActivityPayload{UserId: "u1", PipelineId: strPtr("p1")}
		payloadBytes, _ := protojson.Marshal(payload)
		body, _ := json.Marshal(map[string]interface{}{
			"message":      map[string]interface{}{"data": payloadBytes, "messageId": "m1"},
			"subscription": "sub",
		})
		req := httptest.NewRequest(http.MethodPost, "/", bytes.NewReader(body))
		rec := httptest.NewRecorder()
		EnrichActivityHTTP(rec, req)
		// No matching pipeline → handler succeeds (SKIPPED) → HTTP 200.
		assert.Equal(t, http.StatusOK, rec.Code)
	})
}
