// nolint:proto-json
package enricher

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"strings"
	"testing"
	"time"

	cloudevents "github.com/cloudevents/sdk-go/v2"
	"google.golang.org/protobuf/types/known/timestamppb"

	"github.com/fitglue/server/src/go/internal/pipeline/enricher/providers"
	"github.com/fitglue/server/src/go/internal/pipeline/enricher/providers/user_input"
	user "github.com/fitglue/server/src/go/pkg/domain/user"
	"github.com/fitglue/server/src/go/pkg/testing/mocks"
	pbactivity "github.com/fitglue/server/src/go/pkg/types/pb/models/activity"
	pbevents "github.com/fitglue/server/src/go/pkg/types/pb/models/events"
	pbpipeline "github.com/fitglue/server/src/go/pkg/types/pb/models/pipeline"
	pbplugin "github.com/fitglue/server/src/go/pkg/types/pb/models/plugin"
	pbuser "github.com/fitglue/server/src/go/pkg/types/pb/models/user"
)

// enricherPipelineDB returns one MOCK-enricher pipeline with the given destinations.
func enricherPipelineDB(dests ...pbplugin.DestinationType) *MockDatabase {
	return &MockDatabase{
		GetUserFunc: func(ctx context.Context, id string) (*user.Record, error) {
			return &user.Record{UserProfile: &pbuser.UserProfile{UserId: id}}, nil
		},
		GetUserPipelinesFunc: func(ctx context.Context, userId string) ([]*pbpipeline.PipelineConfig, error) {
			return []*pbpipeline.PipelineConfig{{
				Id:           "p1",
				Source:       "SOURCE_HEVY",
				Destinations: dests,
				Enrichers: []*pbpipeline.EnricherConfig{
					{ProviderType: pbplugin.EnricherProviderType_ENRICHER_PROVIDER_MOCK},
				},
			}}, nil
		},
	}
}

func enricherPayload() *pbevents.ActivityPayload {
	pipelineID := "p1"
	return &pbevents.ActivityPayload{
		UserId:               "u1",
		PipelineId:           &pipelineID,
		Source:               pbactivity.ActivitySource_SOURCE_HEVY,
		StandardizedActivity: oneSessionActivity(),
	}
}

// TestProcess_EnricherRetryableError covers the RetryableError branch: Process
// must return STATUS_LAGGED_RETRY and propagate the retryable error.
func TestProcess_EnricherRetryableError(t *testing.T) {
	o := NewOrchestrator(enricherPipelineDB(), &MockBlobStore{}, "bucket", nil)
	o.Register(&MockProvider{
		NameFunc: func() string { return "mock-enricher" },
		EnrichFunc: func(ctx context.Context, _ *slog.Logger, _ *pbactivity.StandardizedActivity, _ *user.Record, _ map[string]string, _ bool) (*providers.EnrichmentResult, error) {
			return nil, providers.NewRetryableError(errors.New("data lag"), time.Minute, "source data not ready")
		},
	})

	res, err := o.Process(context.Background(), covLogger(), enricherPayload(), "parent", "exec", false)
	if err == nil {
		t.Fatal("expected retryable error to propagate")
	}
	if _, ok := err.(*providers.RetryableError); !ok {
		t.Fatalf("expected *RetryableError, got %T", err)
	}
	if res.Status != pbpipeline.ExecutionStatus_STATUS_LAGGED_RETRY {
		t.Errorf("expected LAGGED_RETRY, got %v", res.Status)
	}
}

// TestProcess_EnricherGenericError covers the path where an enricher returns a
// plain (non-retryable, non-wait) error: Process aborts the pipeline and returns
// "enricher failed: ...".
func TestProcess_EnricherGenericError(t *testing.T) {
	o := NewOrchestrator(enricherPipelineDB(pbplugin.DestinationType_DESTINATION_STRAVA), &MockBlobStore{}, "bucket", nil)
	o.Register(&MockProvider{
		NameFunc: func() string { return "mock-enricher" },
		EnrichFunc: func(ctx context.Context, _ *slog.Logger, _ *pbactivity.StandardizedActivity, _ *user.Record, _ map[string]string, _ bool) (*providers.EnrichmentResult, error) {
			return nil, errors.New("transient provider boom")
		},
	})

	_, err := o.Process(context.Background(), covLogger(), enricherPayload(), "parent", "exec", false)
	if err == nil {
		t.Fatal("expected a generic enricher error to abort the pipeline")
	}
	if !strings.Contains(err.Error(), "enricher failed") {
		t.Errorf("expected 'enricher failed' error, got %v", err)
	}
}

func okUserDB() *MockDatabase {
	return &MockDatabase{
		GetUserFunc: func(ctx context.Context, id string) (*user.Record, error) {
			return &user.Record{UserProfile: &pbuser.UserProfile{UserId: id}}, nil
		},
	}
}

func oneSessionActivity() *pbactivity.StandardizedActivity {
	now := timestamppb.New(time.Date(2024, 1, 1, 10, 0, 0, 0, time.UTC))
	return &pbactivity.StandardizedActivity{
		Name:      "Run",
		StartTime: now,
		Sessions:  []*pbactivity.Session{{StartTime: now, TotalElapsedTime: 60}},
	}
}

// TestProcess_HaltPipeline covers the HaltPipeline branch: the run is marked
// SKIPPED and no events are emitted.
func TestProcess_HaltPipeline(t *testing.T) {
	o := NewOrchestrator(enricherPipelineDB(pbplugin.DestinationType_DESTINATION_STRAVA), &MockBlobStore{}, "bucket", nil)
	o.Register(&MockProvider{
		NameFunc: func() string { return "mock-enricher" },
		EnrichFunc: func(ctx context.Context, _ *slog.Logger, _ *pbactivity.StandardizedActivity, _ *user.Record, _ map[string]string, _ bool) (*providers.EnrichmentResult, error) {
			return &providers.EnrichmentResult{HaltPipeline: true, HaltReason: "filtered out", Metadata: map[string]string{}}, nil
		},
	})
	res, err := o.Process(context.Background(), covLogger(), enricherPayload(), "parent", "exec", false)
	if err != nil {
		t.Fatalf("halt should not error, got %v", err)
	}
	if res.Status != pbpipeline.ExecutionStatus_STATUS_SKIPPED {
		t.Errorf("expected SKIPPED on halt, got %v", res.Status)
	}
	if len(res.Events) != 0 {
		t.Errorf("expected no events on halt, got %d", len(res.Events))
	}
}

// TestProcess_SkippedProvider covers the res.Skipped branch: the provider is
// recorded SKIPPED but the pipeline continues and still emits an event.
func TestProcess_SkippedProvider(t *testing.T) {
	o := NewOrchestrator(enricherPipelineDB(pbplugin.DestinationType_DESTINATION_STRAVA), &MockBlobStore{}, "bucket", nil)
	o.Register(&MockProvider{
		NameFunc: func() string { return "mock-enricher" },
		EnrichFunc: func(ctx context.Context, _ *slog.Logger, _ *pbactivity.StandardizedActivity, _ *user.Record, _ map[string]string, _ bool) (*providers.EnrichmentResult, error) {
			return &providers.EnrichmentResult{Skipped: true, SkipReason: "nothing to do"}, nil
		},
	})
	res, err := o.Process(context.Background(), covLogger(), enricherPayload(), "parent", "exec", false)
	if err != nil {
		t.Fatalf("skipped provider should not error, got %v", err)
	}
	if len(res.Events) != 1 {
		t.Fatalf("expected 1 event when provider skips, got %d", len(res.Events))
	}
}

// TestProcess_NilResultProvider covers the res == nil branch.
func TestProcess_NilResultProvider(t *testing.T) {
	o := NewOrchestrator(enricherPipelineDB(pbplugin.DestinationType_DESTINATION_STRAVA), &MockBlobStore{}, "bucket", nil)
	o.Register(&MockProvider{
		NameFunc: func() string { return "mock-enricher" },
		EnrichFunc: func(ctx context.Context, _ *slog.Logger, _ *pbactivity.StandardizedActivity, _ *user.Record, _ map[string]string, _ bool) (*providers.EnrichmentResult, error) {
			return nil, nil
		},
	})
	res, err := o.Process(context.Background(), covLogger(), enricherPayload(), "parent", "exec", false)
	if err != nil {
		t.Fatalf("nil-result provider should not error, got %v", err)
	}
	if len(res.Events) != 1 {
		t.Fatalf("expected 1 event, got %d", len(res.Events))
	}
}

// TestProcess_RichResultApplied covers the result-field application block:
// Name, NameSuffix, ActivityType, Tags, TimeMarkers and Description are all
// merged into the produced event.
func TestProcess_RichResultApplied(t *testing.T) {
	o := NewOrchestrator(enricherPipelineDB(pbplugin.DestinationType_DESTINATION_STRAVA), &MockBlobStore{}, "bucket", nil)
	o.Register(&MockProvider{
		NameFunc: func() string { return "mock-enricher" },
		EnrichFunc: func(ctx context.Context, _ *slog.Logger, _ *pbactivity.StandardizedActivity, _ *user.Record, _ map[string]string, _ bool) (*providers.EnrichmentResult, error) {
			return &providers.EnrichmentResult{
				Name:         "Renamed",
				NameSuffix:   " (#5)",
				ActivityType: pbactivity.ActivityType_ACTIVITY_TYPE_HIKE,
				Tags:         []string{"tag-a", "tag-b"},
				Description:  "enriched description",
				Metadata:     map[string]string{"k": "v"},
			}, nil
		},
	})
	res, err := o.Process(context.Background(), covLogger(), enricherPayload(), "parent", "exec", false)
	if err != nil {
		t.Fatalf("rich result should not error, got %v", err)
	}
	if len(res.Events) != 1 {
		t.Fatalf("expected 1 event, got %d", len(res.Events))
	}
	ev := res.Events[0]
	if ev.Name != "Renamed (#5)" {
		t.Errorf("expected name 'Renamed (#5)', got %q", ev.Name)
	}
	if !strings.Contains(ev.Description, "enriched description") {
		t.Errorf("expected description applied, got %q", ev.Description)
	}
}

// twoEnricherPipelineDB returns a pipeline with two MOCK enrichers so an
// upstream enricher can exclude the downstream one.
func twoEnricherPipelineDB() *MockDatabase {
	return &MockDatabase{
		GetUserFunc: func(ctx context.Context, id string) (*user.Record, error) {
			return &user.Record{UserProfile: &pbuser.UserProfile{UserId: id}}, nil
		},
		GetUserPipelinesFunc: func(ctx context.Context, userId string) ([]*pbpipeline.PipelineConfig, error) {
			return []*pbpipeline.PipelineConfig{{
				Id:           "p1",
				Source:       "SOURCE_HEVY",
				Destinations: []pbplugin.DestinationType{pbplugin.DestinationType_DESTINATION_STRAVA},
				Enrichers: []*pbpipeline.EnricherConfig{
					{ProviderType: pbplugin.EnricherProviderType_ENRICHER_PROVIDER_MOCK},
					{ProviderType: pbplugin.EnricherProviderType_ENRICHER_PROVIDER_WEATHER},
				},
			}}, nil
		},
	}
}

// TestProcess_StreamDataApplied covers the record-expansion and stream-merge
// block: an enricher returning HR/power/GPS streams populates session records.
func TestProcess_StreamDataApplied(t *testing.T) {
	o := NewOrchestrator(enricherPipelineDB(pbplugin.DestinationType_DESTINATION_STRAVA), &MockBlobStore{}, "bucket", nil)
	o.Register(&MockProvider{
		NameFunc: func() string { return "mock-enricher" },
		EnrichFunc: func(ctx context.Context, _ *slog.Logger, _ *pbactivity.StandardizedActivity, _ *user.Record, _ map[string]string, _ bool) (*providers.EnrichmentResult, error) {
			return &providers.EnrichmentResult{
				HeartRateStream:    []int{100, 110, 120, 130, 140, 150},
				PowerStream:        []int{200, 210, 220, 230, 240, 250},
				PositionLatStream:  []float64{1, 2, 3, 4, 5, 6},
				PositionLongStream: []float64{6, 5, 4, 3, 2, 1},
			}, nil
		},
	})
	res, err := o.Process(context.Background(), covLogger(), enricherPayload(), "parent", "exec", false)
	if err != nil {
		t.Fatalf("stream-data run should not error, got %v", err)
	}
	if len(res.Events) != 1 {
		t.Fatalf("expected 1 event, got %d", len(res.Events))
	}
}

// TestProcess_ExcludeEnrichers covers the ExcludeEnrichers branch: an upstream
// enricher excludes the downstream WEATHER enricher, which is then SKIPPED.
func TestProcess_ExcludeEnrichers(t *testing.T) {
	o := NewOrchestrator(twoEnricherPipelineDB(), &MockBlobStore{}, "bucket", nil)
	// First (MOCK) enricher excludes WEATHER.
	o.Register(&MockProvider{
		NameFunc:         func() string { return "mock-enricher" },
		ProviderTypeFunc: func() pbplugin.EnricherProviderType { return pbplugin.EnricherProviderType_ENRICHER_PROVIDER_MOCK },
		EnrichFunc: func(ctx context.Context, _ *slog.Logger, _ *pbactivity.StandardizedActivity, _ *user.Record, _ map[string]string, _ bool) (*providers.EnrichmentResult, error) {
			return &providers.EnrichmentResult{
				ExcludeEnrichers: []pbplugin.EnricherProviderType{pbplugin.EnricherProviderType_ENRICHER_PROVIDER_WEATHER},
			}, nil
		},
	})
	// Downstream WEATHER enricher would error if it ran — assert it doesn't.
	ran := false
	o.Register(&MockProvider{
		NameFunc:         func() string { return "weather" },
		ProviderTypeFunc: func() pbplugin.EnricherProviderType { return pbplugin.EnricherProviderType_ENRICHER_PROVIDER_WEATHER },
		EnrichFunc: func(ctx context.Context, _ *slog.Logger, _ *pbactivity.StandardizedActivity, _ *user.Record, _ map[string]string, _ bool) (*providers.EnrichmentResult, error) {
			ran = true
			return &providers.EnrichmentResult{}, nil
		},
	})

	res, err := o.Process(context.Background(), covLogger(), enricherPayload(), "parent", "exec", false)
	if err != nil {
		t.Fatalf("exclude run should not error, got %v", err)
	}
	if ran {
		t.Error("excluded WEATHER enricher should not have run")
	}
	var weatherSkipped bool
	for _, pe := range res.ProviderExecutions {
		if pe.ProviderName == "weather" && pe.Status == "SKIPPED" {
			weatherSkipped = true
		}
	}
	if !weatherSkipped {
		t.Error("expected WEATHER enricher recorded as SKIPPED")
	}
}

// TestProcess_WaitForInput covers the blocking WaitForInputError path: a pending
// input is created and the run returns STATUS_WAITING with no events.
func TestProcess_WaitForInput(t *testing.T) {
	created := false
	db := enricherPipelineDB(pbplugin.DestinationType_DESTINATION_STRAVA)
	db.CreatePendingInputFunc = func(ctx context.Context, userId string, input *pbpipeline.PendingInput) error {
		created = true
		return nil
	}

	pub := &mocks.MockPublisher{
		PublishCloudEventFunc: func(ctx context.Context, topic string, e cloudevents.Event) (string, error) {
			return "m1", nil
		},
	}
	o := NewOrchestrator(db, &MockBlobStore{}, "bucket", pub)
	o.Register(&MockProvider{
		NameFunc: func() string { return "mock-enricher" },
		EnrichFunc: func(ctx context.Context, _ *slog.Logger, _ *pbactivity.StandardizedActivity, _ *user.Record, _ map[string]string, _ bool) (*providers.EnrichmentResult, error) {
			return nil, &user_input.WaitForInputError{
				ActivityID:         "pending-1",
				RequiredFields:     []string{"description"},
				EnricherProviderID: "mock-enricher",
			}
		},
	})

	res, err := o.Process(context.Background(), covLogger(), enricherPayload(), "parent", "exec", false)
	if err != nil {
		t.Fatalf("wait-for-input should not error, got %v", err)
	}
	if res.Status != pbpipeline.ExecutionStatus_STATUS_WAITING {
		t.Errorf("expected STATUS_WAITING, got %v", res.Status)
	}
	if len(res.Events) != 0 {
		t.Errorf("expected no events while waiting, got %d", len(res.Events))
	}
	if !created {
		t.Error("expected a pending input to be created")
	}
}

// TestProcess_ProviderNotRegistered covers the path where a pipeline references
// an enricher type with no registered provider: it is recorded SKIPPED.
func TestProcess_ProviderNotRegistered(t *testing.T) {
	// Pipeline references WEATHER but we register nothing.
	db := &MockDatabase{
		GetUserFunc: func(ctx context.Context, id string) (*user.Record, error) {
			return &user.Record{UserProfile: &pbuser.UserProfile{UserId: id}}, nil
		},
		GetUserPipelinesFunc: func(ctx context.Context, userId string) ([]*pbpipeline.PipelineConfig, error) {
			return []*pbpipeline.PipelineConfig{{
				Id:           "p1",
				Source:       "SOURCE_HEVY",
				Destinations: []pbplugin.DestinationType{pbplugin.DestinationType_DESTINATION_STRAVA},
				Enrichers: []*pbpipeline.EnricherConfig{
					{ProviderType: pbplugin.EnricherProviderType_ENRICHER_PROVIDER_WEATHER},
				},
			}}, nil
		},
	}
	o := NewOrchestrator(db, &MockBlobStore{}, "bucket", nil)
	res, err := o.Process(context.Background(), covLogger(), enricherPayload(), "parent", "exec", false)
	if err != nil {
		t.Fatalf("unregistered provider should not error, got %v", err)
	}
	var skipped bool
	for _, pe := range res.ProviderExecutions {
		if pe.Error == "provider not registered" {
			skipped = true
		}
	}
	if !skipped {
		t.Error("expected an unregistered-provider SKIPPED execution")
	}
}

// resumableMockProvider implements ResumableProvider for resume-mode tests.
type resumableMockProvider struct {
	MockProvider
	resumeCalled *bool
}

func (r *resumableMockProvider) EnrichResume(ctx context.Context, activity *pbactivity.StandardizedActivity, u *user.Record, pendingInput *pbpipeline.PendingInput) (*providers.EnrichmentResult, error) {
	*r.resumeCalled = true
	return &providers.EnrichmentResult{Description: "resumed: " + pendingInput.InputData["description"]}, nil
}

// TestProcess_ResumeEnrichResume covers the resume-mode branch where a resolved
// pending input drives EnrichResume instead of Enrich.
func TestProcess_ResumeEnrichResume(t *testing.T) {
	resumeCalled := false
	db := &MockDatabase{
		GetUserFunc: func(ctx context.Context, id string) (*user.Record, error) {
			return &user.Record{UserProfile: &pbuser.UserProfile{UserId: id}}, nil
		},
		GetUserPipelinesFunc: func(ctx context.Context, userId string) ([]*pbpipeline.PipelineConfig, error) {
			return []*pbpipeline.PipelineConfig{{
				Id:           "p1",
				Source:       "SOURCE_HEVY",
				Destinations: []pbplugin.DestinationType{pbplugin.DestinationType_DESTINATION_STRAVA},
				Enrichers: []*pbpipeline.EnricherConfig{
					{ProviderType: pbplugin.EnricherProviderType_ENRICHER_PROVIDER_MOCK},
				},
			}}, nil
		},
		GetPendingInputFunc: func(ctx context.Context, userID, id string) (*pbpipeline.PendingInput, error) {
			return &pbpipeline.PendingInput{
				ActivityId:         id,
				Status:             pbpipeline.PendingInput_STATUS_COMPLETED,
				EnricherProviderId: "mock-enricher",
				InputData:          map[string]string{"description": "user text"},
			}, nil
		},
	}
	o := NewOrchestrator(db, &MockBlobStore{}, "bucket", nil)

	rp := &resumableMockProvider{resumeCalled: &resumeCalled}
	rp.NameFunc = func() string { return "mock-enricher" }
	rp.ProviderTypeFunc = func() pbplugin.EnricherProviderType {
		return pbplugin.EnricherProviderType_ENRICHER_PROVIDER_MOCK
	}
	o.Register(rp)

	pipelineID := "p1"
	activityID := "resume-activity-1"
	pendingID := "pending-1"
	payload := &pbevents.ActivityPayload{
		UserId:               "u1",
		PipelineId:           &pipelineID,
		Source:               pbactivity.ActivitySource_SOURCE_HEVY,
		IsResume:             true,
		ActivityId:           &activityID,
		ResumePendingInputId: &pendingID,
		StandardizedActivity: oneSessionActivity(),
	}
	res, err := o.Process(context.Background(), covLogger(), payload, "parent", "exec", false)
	if err != nil {
		t.Fatalf("resume run should not error, got %v", err)
	}
	if !resumeCalled {
		t.Error("expected EnrichResume to be called")
	}
	if len(res.Events) != 1 || !strings.Contains(res.Events[0].Description, "resumed: user text") {
		t.Errorf("expected resumed description in event, got %+v", res.Events)
	}
}

// nonIdempotentMockProvider implements NonIdempotentProvider (IsIdempotent=false)
// so the orchestrator replays its journal entry instead of re-running it.
type nonIdempotentMockProvider struct {
	MockProvider
	ran *bool
}

func (n *nonIdempotentMockProvider) IsIdempotent() bool { return false }

func (n *nonIdempotentMockProvider) Enrich(ctx context.Context, l *slog.Logger, a *pbactivity.StandardizedActivity, u *user.Record, cfg map[string]string, dnr bool) (*providers.EnrichmentResult, error) {
	*n.ran = true
	return &providers.EnrichmentResult{}, nil
}

// TestProcess_ResumeReplaysJournal covers the resume-mode journal replay: a
// non-idempotent enricher that already completed is replayed (its stored
// mutations re-applied) rather than re-executed.
func TestProcess_ResumeReplaysJournal(t *testing.T) {
	enrichRan := false
	db := &MockDatabase{
		GetUserFunc: func(ctx context.Context, id string) (*user.Record, error) {
			return &user.Record{UserProfile: &pbuser.UserProfile{UserId: id}}, nil
		},
		GetUserPipelinesFunc: func(ctx context.Context, userId string) ([]*pbpipeline.PipelineConfig, error) {
			return []*pbpipeline.PipelineConfig{{
				Id:           "p1",
				Source:       "SOURCE_HEVY",
				Destinations: []pbplugin.DestinationType{pbplugin.DestinationType_DESTINATION_STRAVA},
				Enrichers: []*pbpipeline.EnricherConfig{
					{ProviderType: pbplugin.EnricherProviderType_ENRICHER_PROVIDER_MOCK},
				},
			}}, nil
		},
		// Existing run carries the replay journal for "mock-enricher".
		GetPipelineRunFunc: func(ctx context.Context, userID, id string) (*pbpipeline.PipelineRun, error) {
			return &pbpipeline.PipelineRun{
				Id:     id,
				Status: pbpipeline.PipelineRunStatus_PIPELINE_RUN_STATUS_PENDING,
				Boosters: []*pbpipeline.BoosterExecution{{
					ProviderName: "mock-enricher",
					Metadata: map[string]string{
						"replay_completed":   "true",
						"replay_name":        "Replayed Name",
						"replay_description": "replayed description",
					},
				}},
			}, nil
		},
	}
	o := NewOrchestrator(db, &MockBlobStore{}, "bucket", nil)

	nip := &nonIdempotentMockProvider{ran: &enrichRan}
	nip.NameFunc = func() string { return "mock-enricher" }
	nip.ProviderTypeFunc = func() pbplugin.EnricherProviderType {
		return pbplugin.EnricherProviderType_ENRICHER_PROVIDER_MOCK
	}
	o.Register(nip)

	pipelineID := "p1"
	activityID := "resume-1"
	payload := &pbevents.ActivityPayload{
		UserId:               "u1",
		PipelineId:           &pipelineID,
		Source:               pbactivity.ActivitySource_SOURCE_HEVY,
		IsResume:             true,
		ActivityId:           &activityID,
		StandardizedActivity: oneSessionActivity(),
	}
	res, err := o.Process(context.Background(), covLogger(), payload, "parent", "exec", false)
	if err != nil {
		t.Fatalf("replay run should not error, got %v", err)
	}
	if enrichRan {
		t.Error("non-idempotent enricher should be replayed, not re-run")
	}
	var replayed bool
	for _, pe := range res.ProviderExecutions {
		if pe.Status == "REPLAYED" {
			replayed = true
		}
	}
	if !replayed {
		t.Error("expected a REPLAYED provider execution")
	}
	if len(res.Events) == 1 && res.Events[0].Name != "Replayed Name" {
		t.Errorf("expected replayed name applied, got %q", res.Events[0].Name)
	}
}

// TestProcess_ResumeReplaysHeartRateStream is a regression test: resolving a pending
// input (e.g. an unrelated non-blocking enricher) republishes the pipeline from the
// clean pre-enrichment payload, which has no heart rate data merged in. A non-idempotent
// stream provider must have its previously-fetched stream replayed from the journal
// instead of being re-run, otherwise it silently re-queries the source API and can
// commit a worse (partial) stream than the one already applied on the first run.
func TestProcess_ResumeReplaysHeartRateStream(t *testing.T) {
	enrichRan := false
	streamJSON, err := json.Marshal([]int{111, 112, 113, 114, 115})
	if err != nil {
		t.Fatalf("failed to marshal fixture stream: %v", err)
	}
	db := &MockDatabase{
		GetUserFunc: func(ctx context.Context, id string) (*user.Record, error) {
			return &user.Record{UserProfile: &pbuser.UserProfile{UserId: id}}, nil
		},
		GetUserPipelinesFunc: func(ctx context.Context, userId string) ([]*pbpipeline.PipelineConfig, error) {
			return []*pbpipeline.PipelineConfig{{
				Id:           "p1",
				Source:       "SOURCE_HEVY",
				Destinations: []pbplugin.DestinationType{pbplugin.DestinationType_DESTINATION_STRAVA},
				Enrichers: []*pbpipeline.EnricherConfig{
					{ProviderType: pbplugin.EnricherProviderType_ENRICHER_PROVIDER_MOCK},
				},
			}}, nil
		},
		// Existing run carries the previously-fetched heart rate stream in the journal.
		GetPipelineRunFunc: func(ctx context.Context, userID, id string) (*pbpipeline.PipelineRun, error) {
			return &pbpipeline.PipelineRun{
				Id:     id,
				Status: pbpipeline.PipelineRunStatus_PIPELINE_RUN_STATUS_PENDING,
				Boosters: []*pbpipeline.BoosterExecution{{
					ProviderName: "mock-enricher",
					Metadata: map[string]string{
						"replay_completed":         "true",
						"replay_heart_rate_stream": string(streamJSON),
					},
				}},
			}, nil
		},
	}
	o := NewOrchestrator(db, &MockBlobStore{}, "bucket", nil)

	nip := &nonIdempotentMockProvider{ran: &enrichRan}
	nip.NameFunc = func() string { return "mock-enricher" }
	nip.ProviderTypeFunc = func() pbplugin.EnricherProviderType {
		return pbplugin.EnricherProviderType_ENRICHER_PROVIDER_MOCK
	}
	o.Register(nip)

	pipelineID := "p1"
	activityID := "resume-hr-1"
	activity := oneSessionActivity()
	activity.Sessions[0].TotalElapsedTime = 5 // match the 5-point fixture stream
	payload := &pbevents.ActivityPayload{
		UserId:               "u1",
		PipelineId:           &pipelineID,
		Source:               pbactivity.ActivitySource_SOURCE_HEVY,
		IsResume:             true,
		ActivityId:           &activityID,
		StandardizedActivity: activity,
	}
	res, err := o.Process(context.Background(), covLogger(), payload, "parent", "exec", false)
	if err != nil {
		t.Fatalf("replay run should not error, got %v", err)
	}
	if enrichRan {
		t.Error("non-idempotent stream provider should be replayed, not re-run against the source API")
	}
	if len(res.Events) != 1 {
		t.Fatalf("expected 1 event, got %d", len(res.Events))
	}

	var records []*pbactivity.Record
	for _, lap := range res.Events[0].ActivityData.Sessions[0].Laps {
		records = append(records, lap.Records...)
	}
	if len(records) != 5 {
		t.Fatalf("expected 5 records from replayed stream expansion, got %d", len(records))
	}
	for i, want := range []int32{111, 112, 113, 114, 115} {
		if records[i].HeartRate != want {
			t.Errorf("record[%d].HeartRate = %d, want %d", i, records[i].HeartRate, want)
		}
	}
}

// nonBlockingMockProvider implements SupportsNonBlocking + ResumableProvider so a
// WaitForInputError in NonBlocking config defers without halting the pipeline.
type nonBlockingMockProvider struct {
	MockProvider
}

func (n *nonBlockingMockProvider) EnrichResume(ctx context.Context, a *pbactivity.StandardizedActivity, u *user.Record, pi *pbpipeline.PendingInput) (*providers.EnrichmentResult, error) {
	return &providers.EnrichmentResult{}, nil
}

// TestProcess_NonBlockingWaitContinues covers the non-blocking WaitForInputError
// branch: the pipeline continues, a pending input is created, and an event is
// still emitted with the non-blocking pending input id attached.
func TestProcess_NonBlockingWaitContinues(t *testing.T) {
	created := false
	db := &MockDatabase{
		GetUserFunc: func(ctx context.Context, id string) (*user.Record, error) {
			return &user.Record{UserProfile: &pbuser.UserProfile{UserId: id}}, nil
		},
		GetUserPipelinesFunc: func(ctx context.Context, userId string) ([]*pbpipeline.PipelineConfig, error) {
			return []*pbpipeline.PipelineConfig{{
				Id:           "p1",
				Source:       "SOURCE_HEVY",
				Destinations: []pbplugin.DestinationType{pbplugin.DestinationType_DESTINATION_STRAVA},
				Enrichers: []*pbpipeline.EnricherConfig{
					{ProviderType: pbplugin.EnricherProviderType_ENRICHER_PROVIDER_MOCK, NonBlocking: true},
				},
			}}, nil
		},
		CreatePendingInputFunc: func(ctx context.Context, userId string, input *pbpipeline.PendingInput) error {
			created = true
			return nil
		},
	}
	pub := &mocks.MockPublisher{
		PublishCloudEventFunc: func(ctx context.Context, topic string, e cloudevents.Event) (string, error) { return "m", nil },
	}
	o := NewOrchestrator(db, &MockBlobStore{}, "bucket", pub)

	nb := &nonBlockingMockProvider{}
	nb.NameFunc = func() string { return "mock-enricher" }
	nb.ProviderTypeFunc = func() pbplugin.EnricherProviderType {
		return pbplugin.EnricherProviderType_ENRICHER_PROVIDER_MOCK
	}
	nb.EnrichFunc = func(ctx context.Context, _ *slog.Logger, _ *pbactivity.StandardizedActivity, _ *user.Record, _ map[string]string, _ bool) (*providers.EnrichmentResult, error) {
		return nil, &user_input.WaitForInputError{
			ActivityID:         "nb-pending",
			RequiredFields:     []string{"description"},
			EnricherProviderID: "mock-enricher",
		}
	}
	o.Register(nb)

	res, err := o.Process(context.Background(), covLogger(), enricherPayload(), "parent", "exec", false)
	if err != nil {
		t.Fatalf("non-blocking wait should not error, got %v", err)
	}
	if !created {
		t.Error("expected a non-blocking pending input to be created")
	}
	if len(res.Events) != 1 {
		t.Fatalf("expected pipeline to continue with 1 event, got %d", len(res.Events))
	}
	if len(res.Events[0].NonBlockingPendingInputIds) == 0 {
		t.Error("expected non-blocking pending input id attached to event")
	}
}

// deferrableMockProvider is a MockProvider that defers to Phase 2.
type deferrableMockProvider struct {
	MockProvider
}

func (d *deferrableMockProvider) ShouldDefer() bool { return true }

// TestProcess_DeferredAndBranding covers the Phase 2 deferred-execution block and
// the branding provider application (hobbyist users get branding).
func TestProcess_DeferredAndBranding(t *testing.T) {
	db := &MockDatabase{
		GetUserFunc: func(ctx context.Context, id string) (*user.Record, error) {
			// Hobbyist (default tier) → ShouldShowBranding true.
			return &user.Record{UserProfile: &pbuser.UserProfile{UserId: id, Tier: pbuser.UserTier_USER_TIER_HOBBYIST}}, nil
		},
		GetUserPipelinesFunc: func(ctx context.Context, userId string) ([]*pbpipeline.PipelineConfig, error) {
			return []*pbpipeline.PipelineConfig{{
				Id:           "p1",
				Source:       "SOURCE_HEVY",
				Destinations: []pbplugin.DestinationType{pbplugin.DestinationType_DESTINATION_STRAVA},
				Enrichers: []*pbpipeline.EnricherConfig{
					{ProviderType: pbplugin.EnricherProviderType_ENRICHER_PROVIDER_AI_COMPANION},
				},
			}}, nil
		},
	}
	o := NewOrchestrator(db, &MockBlobStore{}, "bucket", nil)

	// Deferred provider mapped to the AI_COMPANION enricher type.
	deferred := &deferrableMockProvider{}
	deferred.NameFunc = func() string { return "ai-companion" }
	deferred.ProviderTypeFunc = func() pbplugin.EnricherProviderType {
		return pbplugin.EnricherProviderType_ENRICHER_PROVIDER_AI_COMPANION
	}
	deferred.EnrichFunc = func(ctx context.Context, _ *slog.Logger, _ *pbactivity.StandardizedActivity, _ *user.Record, cfg map[string]string, _ bool) (*providers.EnrichmentResult, error) {
		// Phase 2 injects the accumulated description into the config.
		if _, ok := cfg["enriched_description"]; !ok {
			t.Error("expected enriched_description injected into deferred config")
		}
		return &providers.EnrichmentResult{Description: "deferred line", NameSuffix: " (AI)"}, nil
	}
	o.Register(deferred)

	// Branding provider (looked up by name "branding").
	o.Register(&MockProvider{
		NameFunc: func() string { return "branding" },
		ProviderTypeFunc: func() pbplugin.EnricherProviderType {
			return pbplugin.EnricherProviderType_ENRICHER_PROVIDER_UNSPECIFIED
		},
		EnrichFunc: func(ctx context.Context, _ *slog.Logger, _ *pbactivity.StandardizedActivity, _ *user.Record, _ map[string]string, _ bool) (*providers.EnrichmentResult, error) {
			return &providers.EnrichmentResult{Description: "Powered by FitGlue"}, nil
		},
	})

	res, err := o.Process(context.Background(), covLogger(), enricherPayload(), "parent", "exec", false)
	if err != nil {
		t.Fatalf("deferred+branding run should not error, got %v", err)
	}
	if len(res.Events) != 1 {
		t.Fatalf("expected 1 event, got %d", len(res.Events))
	}
	desc := res.Events[0].Description
	if !strings.Contains(desc, "deferred line") {
		t.Errorf("expected deferred description applied, got %q", desc)
	}
	if !strings.Contains(desc, "Powered by FitGlue") {
		t.Errorf("expected branding applied, got %q", desc)
	}
}

// TestProcess_RepostSameSource covers the targeted-repost destination filtering
// and the same-source detection metadata: a Strava-sourced activity reposted to
// Strava marks the same-source override.
func TestProcess_RepostSameSource(t *testing.T) {
	db := &MockDatabase{
		GetUserFunc: func(ctx context.Context, id string) (*user.Record, error) {
			return &user.Record{UserProfile: &pbuser.UserProfile{UserId: id}}, nil
		},
		GetUserPipelinesFunc: func(ctx context.Context, userId string) ([]*pbpipeline.PipelineConfig, error) {
			return []*pbpipeline.PipelineConfig{{
				Id:           "p1",
				Source:       "SOURCE_STRAVA",
				Destinations: []pbplugin.DestinationType{pbplugin.DestinationType_DESTINATION_STRAVA, pbplugin.DestinationType_DESTINATION_HEVY},
				Enrichers: []*pbpipeline.EnricherConfig{
					{ProviderType: pbplugin.EnricherProviderType_ENRICHER_PROVIDER_MOCK},
				},
			}}, nil
		},
	}
	o := NewOrchestrator(db, &MockBlobStore{}, "bucket", nil)
	o.Register(&MockProvider{
		NameFunc: func() string { return "mock-enricher" },
		EnrichFunc: func(ctx context.Context, _ *slog.Logger, _ *pbactivity.StandardizedActivity, _ *user.Record, _ map[string]string, _ bool) (*providers.EnrichmentResult, error) {
			return &providers.EnrichmentResult{Description: "x"}, nil
		},
	})

	pipelineID := "p1"
	payload := &pbevents.ActivityPayload{
		UserId:               "u1",
		PipelineId:           &pipelineID,
		Source:               pbactivity.ActivitySource_SOURCE_STRAVA,
		IsRepost:             true,
		RepostMode:           "missed-destination",
		RepostDestination:    "DESTINATION_STRAVA",
		StandardizedActivity: oneSessionActivity(),
	}
	res, err := o.Process(context.Background(), covLogger(), payload, "parent", "exec", false)
	if err != nil {
		t.Fatalf("repost run should not error, got %v", err)
	}
	if len(res.Events) != 1 {
		t.Fatalf("expected 1 event, got %d", len(res.Events))
	}
	ev := res.Events[0]
	// Targeted repost filtered destinations down to just Strava.
	if len(ev.Destinations) != 1 || ev.Destinations[0] != pbplugin.DestinationType_DESTINATION_STRAVA {
		t.Errorf("expected destinations filtered to [STRAVA], got %v", ev.Destinations)
	}
	if ev.EnrichmentMetadata["is_repost"] != "true" {
		t.Errorf("expected is_repost metadata, got %v", ev.EnrichmentMetadata)
	}
	if ev.EnrichmentMetadata["same_source_destination_strava"] != "true" {
		t.Errorf("expected same-source detection for strava, got %v", ev.EnrichmentMetadata)
	}
}

// TestProcess_GetUserError covers the failed-GetUser early return.
func TestProcess_GetUserError(t *testing.T) {
	db := &MockDatabase{
		GetUserFunc: func(ctx context.Context, id string) (*user.Record, error) {
			return nil, errors.New("db down")
		},
	}
	o := NewOrchestrator(db, &MockBlobStore{}, "bucket", nil)
	pipelineID := "p1"
	_, err := o.Process(context.Background(), covLogger(), &pbevents.ActivityPayload{
		UserId:               "u1",
		PipelineId:           &pipelineID,
		StandardizedActivity: oneSessionActivity(),
	}, "parent", "exec", false)
	if err == nil {
		t.Fatal("expected error on GetUser failure")
	}
}

// TestProcess_NilActivity covers the terminal "standardized activity is nil".
func TestProcess_NilActivity(t *testing.T) {
	o := NewOrchestrator(okUserDB(), &MockBlobStore{}, "bucket", nil)
	pipelineID := "p1"
	_, err := o.Process(context.Background(), covLogger(), &pbevents.ActivityPayload{
		UserId:     "u1",
		PipelineId: &pipelineID,
	}, "parent", "exec", false)
	if err == nil || err.Error() != "standardized activity is nil" {
		t.Fatalf("expected nil-activity terminal error, got %v", err)
	}
}

// TestProcess_MissingPipelineID covers the terminal "pipeline_id is required".
func TestProcess_MissingPipelineID(t *testing.T) {
	o := NewOrchestrator(okUserDB(), &MockBlobStore{}, "bucket", nil)
	_, err := o.Process(context.Background(), covLogger(), &pbevents.ActivityPayload{
		UserId:               "u1",
		StandardizedActivity: oneSessionActivity(),
	}, "parent", "exec", false)
	if err == nil || err.Error() != "pipeline_id is required" {
		t.Fatalf("expected pipeline_id terminal error, got %v", err)
	}
}

// TestProcess_TierBlocked covers the tier-limit SKIPPED path, including creation
// of a visible TIER_BLOCKED pipeline run.
func TestProcess_TierBlocked(t *testing.T) {
	created := false
	db := &MockDatabase{
		GetUserFunc: func(ctx context.Context, id string) (*user.Record, error) {
			return &user.Record{UserProfile: &pbuser.UserProfile{
				UserId:             id,
				Tier:               pbuser.UserTier_USER_TIER_HOBBYIST,
				SyncCountThisMonth: 100000,                      // way over the hobbyist limit
				SyncCountResetAt:   timestamppb.New(time.Now()), // same month → no reset
			}}, nil
		},
		CreatePipelineRunFunc: func(ctx context.Context, userId string, run *pbpipeline.PipelineRun) error {
			created = true
			if run.Status != pbpipeline.PipelineRunStatus_PIPELINE_RUN_STATUS_TIER_BLOCKED {
				t.Errorf("expected TIER_BLOCKED run, got %v", run.Status)
			}
			return nil
		},
	}

	o := NewOrchestrator(db, &MockBlobStore{}, "bucket", nil)
	pipelineID := "p1"
	res, err := o.Process(context.Background(), covLogger(), &pbevents.ActivityPayload{
		UserId:               "u1",
		PipelineId:           &pipelineID,
		Source:               pbactivity.ActivitySource_SOURCE_HEVY,
		StandardizedActivity: oneSessionActivity(),
	}, "parent", "exec", false)
	if err != nil {
		t.Fatalf("tier-blocked should not error, got %v", err)
	}
	if res.Status != pbpipeline.ExecutionStatus_STATUS_SKIPPED {
		t.Errorf("expected SKIPPED, got %v", res.Status)
	}
	if !created {
		t.Error("expected a TIER_BLOCKED pipeline run to be created")
	}
}

// TestProcess_CancelledRunSkips covers the redelivery guard: an already-cancelled
// run short-circuits with STATUS_UNSPECIFIED and no events.
func TestProcess_CancelledRunSkips(t *testing.T) {
	db := &MockDatabase{
		GetUserFunc: func(ctx context.Context, id string) (*user.Record, error) {
			return &user.Record{UserProfile: &pbuser.UserProfile{UserId: id}}, nil
		},
		GetUserPipelinesFunc: func(ctx context.Context, userId string) ([]*pbpipeline.PipelineConfig, error) {
			return []*pbpipeline.PipelineConfig{{Id: "p1", Source: "SOURCE_HEVY"}}, nil
		},
		GetPipelineRunFunc: func(ctx context.Context, userId, id string) (*pbpipeline.PipelineRun, error) {
			return &pbpipeline.PipelineRun{
				Id:     id,
				Status: pbpipeline.PipelineRunStatus_PIPELINE_RUN_STATUS_CANCELLED,
			}, nil
		},
	}
	o := NewOrchestrator(db, &MockBlobStore{}, "bucket", nil)
	pipelineID := "p1"
	res, err := o.Process(context.Background(), covLogger(), &pbevents.ActivityPayload{
		UserId:               "u1",
		PipelineId:           &pipelineID,
		Source:               pbactivity.ActivitySource_SOURCE_HEVY,
		StandardizedActivity: oneSessionActivity(),
	}, "parent", "exec", false)
	if err != nil {
		t.Fatalf("cancelled-run guard should not error, got %v", err)
	}
	if len(res.Events) != 0 {
		t.Errorf("expected 0 events for cancelled run, got %d", len(res.Events))
	}
	if res.Status != pbpipeline.ExecutionStatus_STATUS_UNSPECIFIED {
		t.Errorf("expected STATUS_UNSPECIFIED, got %v", res.Status)
	}
}

// TestProcess_ResumeMissingActivityID covers the resume-mode validation that
// activity_id must be provided.
func TestProcess_ResumeMissingActivityID(t *testing.T) {
	db := &MockDatabase{
		GetUserFunc: func(ctx context.Context, id string) (*user.Record, error) {
			return &user.Record{UserProfile: &pbuser.UserProfile{UserId: id}}, nil
		},
		GetUserPipelinesFunc: func(ctx context.Context, userId string) ([]*pbpipeline.PipelineConfig, error) {
			return []*pbpipeline.PipelineConfig{{Id: "p1", Source: "SOURCE_HEVY"}}, nil
		},
	}
	o := NewOrchestrator(db, &MockBlobStore{}, "bucket", nil)
	pipelineID := "p1"
	_, err := o.Process(context.Background(), covLogger(), &pbevents.ActivityPayload{
		UserId:               "u1",
		PipelineId:           &pipelineID,
		Source:               pbactivity.ActivitySource_SOURCE_HEVY,
		IsResume:             true,
		StandardizedActivity: oneSessionActivity(),
	}, "parent", "exec", false)
	if err == nil {
		t.Fatal("expected error when resume mode lacks activity_id")
	}
}
