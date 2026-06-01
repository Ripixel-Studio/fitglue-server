package enricher

import (
	user "github.com/fitglue/server/src/go/pkg/domain/user"
	"google.golang.org/protobuf/reflect/protoreflect"
	"google.golang.org/protobuf/reflect/protoregistry"

	pbuser "github.com/fitglue/server/src/go/pkg/types/pb/models/user"

	pbplugin "github.com/fitglue/server/src/go/pkg/types/pb/models/plugin"

	pbevents "github.com/fitglue/server/src/go/pkg/types/pb/models/events"
	pbpipeline "github.com/fitglue/server/src/go/pkg/types/pb/models/pipeline"

	pbactivity "github.com/fitglue/server/src/go/pkg/types/pb/models/activity"

	"context"
	"log/slog"
	"strings"
	"testing"
	"time"

	"google.golang.org/protobuf/types/known/timestamppb"

	"github.com/fitglue/server/src/go/internal/pipeline/enricher/providers"
	shared "github.com/fitglue/server/src/go/pkg"
)

// MockDatabase implements shared.Database
type MockDatabase struct {
	GetUserFunc          func(ctx context.Context, id string) (*user.Record, error)
	GetUserPipelinesFunc func(ctx context.Context, userId string) ([]*pbpipeline.PipelineConfig, error)
}

func (m *MockDatabase) GetUser(ctx context.Context, id string) (*user.Record, error) {
	if m.GetUserFunc != nil {
		return m.GetUserFunc(ctx, id)
	}
	return nil, nil
}
func (m *MockDatabase) SetExecution(ctx context.Context, record *pbpipeline.ExecutionRecord) error {
	return nil
}
func (m *MockDatabase) UpdateExecution(ctx context.Context, userId string, id string, data map[string]interface{}) error {
	return nil
}
func (m *MockDatabase) UpdateUser(ctx context.Context, id string, data map[string]interface{}) error {
	return nil
}
func (m *MockDatabase) CreatePendingInput(ctx context.Context, userId string, input *pbpipeline.PendingInput) error {
	return nil
}
func (m *MockDatabase) GetPendingInput(ctx context.Context, userId string, id string) (*pbpipeline.PendingInput, error) {
	return nil, nil
}
func (m *MockDatabase) UpdatePendingInput(ctx context.Context, userId string, id string, data map[string]interface{}) error {
	return nil
}
func (m *MockDatabase) ListPendingInputs(ctx context.Context, userID string) ([]*pbpipeline.PendingInput, error) {
	return nil, nil
}
func (m *MockDatabase) DeletePendingInput(ctx context.Context, userId string, id string) error {
	return nil
}
func (m *MockDatabase) GetCounter(ctx context.Context, userId string, id string) (*pbuser.Counter, error) {
	return nil, nil
}
func (m *MockDatabase) SetCounter(ctx context.Context, userId string, counter *pbuser.Counter) error {
	return nil
}
func (m *MockDatabase) ListCounters(ctx context.Context, userId string) ([]*pbuser.Counter, error) {
	return nil, nil
}
func (m *MockDatabase) DeleteCounter(ctx context.Context, userId string, id string) error {
	return nil
}
func (m *MockDatabase) RecordBillingEvent(_ context.Context, _ string, _ shared.BillingEvent) error {
	return nil
}
func (m *MockDatabase) CountBillingEvents(_ context.Context, _ string) (int32, error) {
	return 0, nil
}
func (m *MockDatabase) CountBillingEventsForPeriod(_ context.Context, _, _ string) (int32, error) {
	return 0, nil
}
func (m *MockDatabase) IncrementSyncCount(ctx context.Context, userID string) error {
	return nil
}
func (m *MockDatabase) IncrementPreventedSyncCount(ctx context.Context, userID string) error {
	return nil
}
func (m *MockDatabase) ResetSyncCount(ctx context.Context, userID string) error {
	return nil
}
func (m *MockDatabase) ListPendingInputsByEnricher(ctx context.Context, enricherId string, status pbpipeline.PendingInput_Status) ([]*pbpipeline.PendingInput, error) {
	return nil, nil
}
func (m *MockDatabase) ShowcaseActivityExists(ctx context.Context, showcaseId string) (bool, error) {
	return false, nil
}
func (m *MockDatabase) SetShowcasedActivity(ctx context.Context, userID string, activity *pbactivity.ShowcasedActivity) error {
	return nil
}
func (m *MockDatabase) GetShowcasedActivity(ctx context.Context, showcaseId string) (*pbactivity.ShowcasedActivity, error) {
	return nil, nil
}
func (m *MockDatabase) SetShowcaseProfile(ctx context.Context, profile *pbactivity.ShowcaseProfile) error {
	return nil
}
func (m *MockDatabase) GetShowcaseProfile(ctx context.Context, slug string) (*pbactivity.ShowcaseProfile, error) {
	return nil, nil
}
func (m *MockDatabase) GetShowcaseProfileByUserId(ctx context.Context, userId string) (*pbactivity.ShowcaseProfile, error) {
	return nil, nil
}
func (m *MockDatabase) DeleteShowcaseProfile(ctx context.Context, slug string) error {
	return nil
}
func (m *MockDatabase) SetShowcaseProfileEntry(ctx context.Context, userID string, entry *pbactivity.ShowcaseProfileEntry) error {
	return nil
}
func (m *MockDatabase) GetPersonalRecord(ctx context.Context, userId string, recordType string) (*pbuser.PersonalRecord, error) {
	return nil, nil
}
func (m *MockDatabase) SetPersonalRecord(ctx context.Context, userId string, record *pbuser.PersonalRecord) error {
	return nil
}
func (m *MockDatabase) ListPersonalRecords(ctx context.Context, userId string) ([]*pbuser.PersonalRecord, error) {
	return nil, nil
}
func (m *MockDatabase) DeletePersonalRecord(ctx context.Context, userId string, recordType string) error {
	return nil
}
func (m *MockDatabase) GetUserPipelines(ctx context.Context, userId string) ([]*pbpipeline.PipelineConfig, error) {
	if m.GetUserPipelinesFunc != nil {
		return m.GetUserPipelinesFunc(ctx, userId)
	}
	return []*pbpipeline.PipelineConfig{}, nil
}
func (m *MockDatabase) GetPluginDefault(ctx context.Context, userId string, pluginId string) (*pbpipeline.PluginDefault, error) {
	return nil, nil
}
func (m *MockDatabase) SetPluginDefault(ctx context.Context, userId string, pluginDefault *pbpipeline.PluginDefault) error {
	return nil
}
func (m *MockDatabase) SetUploadedActivity(ctx context.Context, userId string, record *pbactivity.UploadedActivityRecord) error {
	return nil
}
func (m *MockDatabase) GetUploadedActivity(ctx context.Context, userId string, destination pbplugin.DestinationType, destinationId string) (*pbactivity.UploadedActivityRecord, error) {
	return nil, nil
}
func (m *MockDatabase) CreatePipelineRun(ctx context.Context, userId string, run *pbpipeline.PipelineRun) error {
	return nil
}
func (m *MockDatabase) GetPipelineRun(ctx context.Context, userId string, id string) (*pbpipeline.PipelineRun, error) {
	return nil, nil
}
func (m *MockDatabase) GetPipelineRunByActivityId(ctx context.Context, userId string, activityId string) (*pbpipeline.PipelineRun, error) {
	return nil, nil
}
func (m *MockDatabase) UpdatePipelineRun(ctx context.Context, userId string, id string, data map[string]interface{}) error {
	return nil
}
func (m *MockDatabase) SetDestinationOutcome(ctx context.Context, userId string, pipelineRunId string, outcome *pbpipeline.DestinationOutcome) error {
	return nil
}
func (m *MockDatabase) GetDestinationOutcomes(ctx context.Context, userId string, pipelineRunId string) ([]*pbpipeline.DestinationOutcome, error) {
	return nil, nil
}
func (m *MockDatabase) GetBoosterData(ctx context.Context, userId string, boosterId string) (map[string]interface{}, error) {
	return nil, nil
}
func (m *MockDatabase) SetBoosterData(ctx context.Context, userId string, boosterId string, data map[string]interface{}) error {
	return nil
}
func (m *MockDatabase) DeleteBoosterData(ctx context.Context, userId string, boosterId string) error {
	return nil
}

type MockBlobStore struct {
	WriteFunc  func(ctx context.Context, bucket, object string, data []byte) error
	GetFunc    func(ctx context.Context, bucket, object string) ([]byte, error)
	DeleteFunc func(ctx context.Context, bucket, object string) error
}

func (m *MockBlobStore) Write(ctx context.Context, bucket, object string, data []byte) error {
	if m.WriteFunc != nil {
		return m.WriteFunc(ctx, bucket, object, data)
	}
	return nil
}
func (m *MockBlobStore) Get(ctx context.Context, bucket, object string) ([]byte, error) {
	if m.GetFunc != nil {
		return m.GetFunc(ctx, bucket, object)
	}
	return []byte("mock-data"), nil
}
func (m *MockBlobStore) Delete(ctx context.Context, bucket, object string) error {
	if m.DeleteFunc != nil {
		return m.DeleteFunc(ctx, bucket, object)
	}
	return nil
}

// MockProvider implements providers.Provider
type MockProvider struct {
	NameFunc         func() string
	ProviderTypeFunc func() pbplugin.EnricherProviderType
	EnrichFunc       func(ctx context.Context, logger *slog.Logger, activity *pbactivity.StandardizedActivity, user *user.Record, inputConfig map[string]string, doNotRetry bool) (*providers.EnrichmentResult, error)
}

func (m *MockProvider) Name() string {
	if m.NameFunc != nil {
		return m.NameFunc()
	}
	return "mock-provider"
}

func (m *MockProvider) ProviderType() pbplugin.EnricherProviderType {
	if m.ProviderTypeFunc != nil {
		return m.ProviderTypeFunc()
	}
	return pbplugin.EnricherProviderType_ENRICHER_PROVIDER_MOCK
}

func (m *MockProvider) Enrich(ctx context.Context, logger *slog.Logger, activity *pbactivity.StandardizedActivity, user *user.Record, inputConfig map[string]string, doNotRetry bool) (*providers.EnrichmentResult, error) {
	if m.EnrichFunc != nil {
		return m.EnrichFunc(ctx, logger, activity, user, inputConfig, doNotRetry)
	}
	return &providers.EnrichmentResult{}, nil
}

func TestOrchestrator_Process(t *testing.T) {
	ctx := context.Background()

	t.Run("Executes configured pipeline", func(t *testing.T) {
		mockDB := &MockDatabase{
			GetUserFunc: func(ctx context.Context, id string) (*user.Record, error) {
				return &user.Record{UserProfile: &pbuser.UserProfile{UserId: id}}, nil
			},
			GetUserPipelinesFunc: func(ctx context.Context, userId string) ([]*pbpipeline.PipelineConfig, error) {
				return []*pbpipeline.PipelineConfig{
					{
						Id:           "pipeline-1",
						Source:       "SOURCE_HEVY",
						Destinations: []pbplugin.DestinationType{pbplugin.DestinationType_DESTINATION_STRAVA},
						Enrichers: []*pbpipeline.EnricherConfig{
							{
								ProviderType: pbplugin.EnricherProviderType_ENRICHER_PROVIDER_MOCK,
								TypedConfig:  map[string]string{"key": "val"},
							},
						},
					},
				}, nil
			},
		}

		mockStorage := &MockBlobStore{
			WriteFunc: func(ctx context.Context, bucket, object string, data []byte) error {
				return nil
			},
		}

		orchestrator := NewOrchestrator(mockDB, mockStorage, "test-bucket", nil)

		mockProvider := &MockProvider{
			NameFunc: func() string { return "mock-enricher" },
			EnrichFunc: func(ctx context.Context, _ *slog.Logger, activity *pbactivity.StandardizedActivity, user *user.Record, inputConfig map[string]string, doNotRetry bool) (*providers.EnrichmentResult, error) {
				return &providers.EnrichmentResult{
					Name:        "Enriched Activity",
					Description: "Added by mock",
					Metadata: map[string]string{
						"processed_by": "mock",
					},
				}, nil
			},
		}
		orchestrator.Register(mockProvider)

		pipelineID := "pipeline-1"
		payload := &pbevents.ActivityPayload{
			UserId:     "user-123",
			Source:     pbactivity.ActivitySource_SOURCE_HEVY,
			PipelineId: &pipelineID, // Required by Rule E25
			Timestamp:  timestamppb.New(time.Date(2023, 1, 1, 10, 0, 0, 0, time.UTC)),
			StandardizedActivity: &pbactivity.StandardizedActivity{
				Name: "Original Run",
				Sessions: []*pbactivity.Session{
					{
						StartTime:        timestamppb.New(time.Date(2023, 1, 1, 10, 0, 0, 0, time.UTC)),
						TotalElapsedTime: 60,
					},
				},
			},
		}

		// Update calls
		result, err := orchestrator.Process(ctx, slog.Default(), payload, "test-parent-exec-id", "test-pipeline-id", false) // false = doNotRetry

		if err != nil {
			t.Fatalf("Process failed: %v", err)
		}

		if len(result.Events) != 1 {
			t.Fatalf("Expected 1 event, got %d", len(result.Events))
		}

		event := result.Events[0]
		if event.Name != "Enriched Activity" {
			t.Errorf("Expected name 'Enriched Activity', got '%s'", event.Name)
		}
		if event.Description != "Added by mock" {
			t.Errorf("Expected description 'Added by mock', got '%s'", event.Description)
		}
		if event.EnrichmentMetadata["processed_by"] != "mock" {
			t.Errorf("Expected metadata 'processed_by'='mock'")
		}
		if len(event.Destinations) != 1 || event.Destinations[0] != pbplugin.DestinationType_DESTINATION_STRAVA {
			t.Errorf("Expected destination 'strava'")
		}
	})

	t.Run("Returns skipped if targeted pipeline not found", func(t *testing.T) {
		// With mandatory pipeline_id, if the targeted pipeline is not found,
		// the orchestrator should return STATUS_SKIPPED.
		mockDB := &MockDatabase{
			GetUserFunc: func(ctx context.Context, id string) (*user.Record, error) {
				return &user.Record{UserProfile: &pbuser.UserProfile{UserId: id}}, nil
			},
			GetUserPipelinesFunc: func(ctx context.Context, userId string) ([]*pbpipeline.PipelineConfig, error) {
				// Return no pipelines
				return []*pbpipeline.PipelineConfig{}, nil
			},
		}

		orchestrator := NewOrchestrator(mockDB, &MockBlobStore{}, "test-bucket", nil)

		nonExistentPipelineID := "non-existent-pipeline"
		payload := &pbevents.ActivityPayload{
			UserId:     "user-123",
			Source:     pbactivity.ActivitySource_SOURCE_HEVY,
			PipelineId: &nonExistentPipelineID,
			StandardizedActivity: &pbactivity.StandardizedActivity{
				Name: "Run",
				Sessions: []*pbactivity.Session{
					{
						StartTime:        timestamppb.New(time.Date(2023, 1, 1, 10, 0, 0, 0, time.UTC)),
						TotalElapsedTime: 60,
					},
				},
			},
			Timestamp: timestamppb.New(time.Date(2023, 1, 1, 10, 0, 0, 0, time.UTC)),
		}

		result, err := orchestrator.Process(ctx, slog.Default(), payload, "test-parent-exec-id", "test-pipeline-id", false)

		if err != nil {
			t.Fatalf("Process should not error when pipeline not found, got: %v", err)
		}

		if len(result.Events) != 0 {
			t.Fatalf("Expected 0 events when pipeline not found, got %d", len(result.Events))
		}
		if result.Status != pbpipeline.ExecutionStatus_STATUS_SKIPPED {
			t.Errorf("Expected STATUS_SKIPPED, got %v", result.Status)
		}
	})

	t.Run("Fails if multiple sessions present", func(t *testing.T) {
		mockDB := &MockDatabase{
			GetUserFunc: func(ctx context.Context, id string) (*user.Record, error) {
				return &user.Record{UserProfile: &pbuser.UserProfile{UserId: id}}, nil
			},
		}
		orchestrator := NewOrchestrator(mockDB, &MockBlobStore{}, "test-bucket", nil)
		payload := &pbevents.ActivityPayload{
			UserId: "user-1",
			StandardizedActivity: &pbactivity.StandardizedActivity{
				Sessions: []*pbactivity.Session{{}, {}}, // Two sessions
			},
		}
		_, err := orchestrator.Process(ctx, slog.Default(), payload, "exec-1", "pipe-1", false)
		if err == nil || err.Error() != "multiple sessions not supported" {
			t.Errorf("Expected 'multiple sessions not supported' error, got %v", err)
		}
	})

	t.Run("Fails if session duration is zero", func(t *testing.T) {
		mockDB := &MockDatabase{
			GetUserFunc: func(ctx context.Context, id string) (*user.Record, error) {
				return &user.Record{UserProfile: &pbuser.UserProfile{UserId: id}}, nil
			},
		}
		orchestrator := NewOrchestrator(mockDB, &MockBlobStore{}, "test-bucket", nil)
		payload := &pbevents.ActivityPayload{
			UserId: "user-1",
			StandardizedActivity: &pbactivity.StandardizedActivity{
				Sessions: []*pbactivity.Session{
					{TotalElapsedTime: 0},
				},
			},
		}
		_, err := orchestrator.Process(ctx, slog.Default(), payload, "exec-1", "pipe-1", false)
		if err == nil || err.Error() != "session total elapsed time is 0" {
			t.Errorf("Expected 'session total elapsed time is 0' error, got %v", err)
		}
	})

	t.Run("Aggregates HR stream into Records", func(t *testing.T) {
		mockDB := &MockDatabase{
			GetUserFunc: func(ctx context.Context, id string) (*user.Record, error) {
				return &user.Record{UserProfile: &pbuser.UserProfile{UserId: id}}, nil
			},
			GetUserPipelinesFunc: func(ctx context.Context, userId string) ([]*pbpipeline.PipelineConfig, error) {
				return []*pbpipeline.PipelineConfig{
					{
						Id:     "p1",
						Source: "SOURCE_HEVY",
						Enrichers: []*pbpipeline.EnricherConfig{
							{ProviderType: pbplugin.EnricherProviderType_ENRICHER_PROVIDER_MOCK},
						},
					},
				}, nil
			},
		}
		mockProvider := &MockProvider{
			NameFunc: func() string { return "mock-enricher" },
			EnrichFunc: func(ctx context.Context, _ *slog.Logger, activity *pbactivity.StandardizedActivity, user *user.Record, inputConfig map[string]string, doNotRetry bool) (*providers.EnrichmentResult, error) {
				return &providers.EnrichmentResult{
					HeartRateStream: []int{100, 110, 120}, // 3 data points
				}, nil
			},
		}
		orchestrator := NewOrchestrator(mockDB, &MockBlobStore{}, "test-bucket", nil)
		orchestrator.Register(mockProvider)

		pipelineID := "p1"
		payload := &pbevents.ActivityPayload{ // Set source explicitly
			Source:     pbactivity.ActivitySource_SOURCE_HEVY,
			UserId:     "u1",
			PipelineId: &pipelineID, // Required by Rule E25
			StandardizedActivity: &pbactivity.StandardizedActivity{
				StartTime: timestamppb.New(time.Date(2024, 1, 1, 10, 0, 0, 0, time.UTC)),
				Sessions: []*pbactivity.Session{
					{
						StartTime:        timestamppb.New(time.Date(2024, 1, 1, 10, 0, 0, 0, time.UTC)),
						TotalElapsedTime: 3,
						// No initial records
					},
				},
			},
		}

		result, err := orchestrator.Process(ctx, slog.Default(), payload, "exec-1", "pipe-1", false)
		if err != nil {
			t.Fatalf("Process failed: %v", err)
		}

		// Verify enriched activity in OUTPUT events (not mutated payload due to Pointer Isolation)
		if len(result.Events) == 0 {
			t.Fatal("Expected at least one event")
		}
		enrichedActivity := result.Events[0].ActivityData
		if len(enrichedActivity.Sessions) == 0 {
			t.Fatal("Session missing in enriched event")
		}
		session := enrichedActivity.Sessions[0]
		if len(session.Laps) == 0 {
			t.Fatal("Lap missing in enriched event") // Orchestrator adds default lap
		}
		records := session.Laps[0].Records
		if len(records) != 3 {
			t.Errorf("Expected 3 records, got %d", len(records))
		} else {
			if records[0].HeartRate != 100 {
				t.Errorf("Expected HR 100, got %d", records[0].HeartRate)
			}
			if records[1].HeartRate != 110 {
				t.Errorf("Expected HR 110, got %d", records[1].HeartRate)
			}
			if records[2].HeartRate != 120 {
				t.Errorf("Expected HR 120, got %d", records[2].HeartRate)
			}
		}
	})

	t.Run("Single pipeline execution - targeted by pipeline_id", func(t *testing.T) {
		// With the Pipeline Splitter Architecture (Rule E25), each enricher invocation
		// processes exactly one pipeline. Verify that only the targeted pipeline is executed.
		mockDB := &MockDatabase{
			GetUserFunc: func(ctx context.Context, id string) (*user.Record, error) {
				return &user.Record{UserProfile: &pbuser.UserProfile{UserId: id}}, nil
			},
			GetUserPipelinesFunc: func(ctx context.Context, userId string) ([]*pbpipeline.PipelineConfig, error) {
				return []*pbpipeline.PipelineConfig{
					{
						Id:           "pipeline-A",
						Source:       "SOURCE_HEVY",
						Destinations: []pbplugin.DestinationType{pbplugin.DestinationType_DESTINATION_STRAVA},
						Enrichers: []*pbpipeline.EnricherConfig{
							{
								ProviderType: pbplugin.EnricherProviderType_ENRICHER_PROVIDER_MOCK,
								TypedConfig:  map[string]string{"id": "A"},
							},
						},
					},
					{
						Id:           "pipeline-B",
						Source:       "SOURCE_HEVY",
						Destinations: []pbplugin.DestinationType{pbplugin.DestinationType_DESTINATION_INTERVALS},
						Enrichers: []*pbpipeline.EnricherConfig{
							{
								ProviderType: pbplugin.EnricherProviderType_ENRICHER_PROVIDER_MOCK,
								TypedConfig:  map[string]string{"id": "B"},
							},
						},
					},
				}, nil
			},
		}

		mockStorage := &MockBlobStore{
			WriteFunc: func(ctx context.Context, bucket, object string, data []byte) error {
				return nil
			},
		}

		orchestrator := NewOrchestrator(mockDB, mockStorage, "test-bucket", nil)

		// Mock provider returns a description based on its config ID
		mockProvider := &MockProvider{
			NameFunc: func() string { return "mock-enricher" },
			EnrichFunc: func(ctx context.Context, _ *slog.Logger, activity *pbactivity.StandardizedActivity, user *user.Record, inputConfig map[string]string, doNotRetry bool) (*providers.EnrichmentResult, error) {
				id := inputConfig["id"]
				return &providers.EnrichmentResult{
					Name:        "Activity " + id,
					Description: "Description from pipeline " + id,
					Metadata: map[string]string{
						"pipeline_id": id,
					},
				}, nil
			},
		}
		orchestrator.Register(mockProvider)

		// Target pipeline-A specifically
		pipelineID := "pipeline-A"
		payload := &pbevents.ActivityPayload{
			UserId:     "user-123",
			Source:     pbactivity.ActivitySource_SOURCE_HEVY,
			PipelineId: &pipelineID, // Targeting only pipeline-A
			Timestamp:  timestamppb.New(time.Date(2023, 1, 1, 10, 0, 0, 0, time.UTC)),
			StandardizedActivity: &pbactivity.StandardizedActivity{
				Name:        "Original Run",
				Description: "", // Start with empty description
				Sessions: []*pbactivity.Session{
					{
						StartTime:        timestamppb.New(time.Date(2023, 1, 1, 10, 0, 0, 0, time.UTC)),
						TotalElapsedTime: 60,
					},
				},
			},
		}

		// Note: In production, pipeline-splitter already appends the pipeline ID to the execution ID
		// so we simulate that here (base-pipeline-exec-pipeline-A)
		result, err := orchestrator.Process(ctx, slog.Default(), payload, "parent-exec", "base-pipeline-exec-pipeline-A", false)

		if err != nil {
			t.Fatalf("Process failed: %v", err)
		}

		// Should produce exactly 1 event (only the targeted pipeline)
		if len(result.Events) != 1 {
			t.Fatalf("Expected 1 event (single pipeline), got %d", len(result.Events))
		}

		event := result.Events[0]

		// Verify it's the targeted pipeline
		if event.PipelineId != "pipeline-A" {
			t.Errorf("Expected pipeline-A, got '%s'", event.PipelineId)
		}

		// Verify Pipeline A contains ONLY its own description
		if event.Description != "Description from pipeline A" {
			t.Errorf("Pipeline A: expected 'Description from pipeline A', got '%s'", event.Description)
		}

		// Verify execution ID contains the pipeline ID
		if event.PipelineExecutionId == nil {
			t.Fatal("Expected event to have pipelineExecutionId")
		}
		if !strings.Contains(*event.PipelineExecutionId, "pipeline-A") {
			t.Errorf("Execution ID should contain 'pipeline-A', got '%s'", *event.PipelineExecutionId)
		}
	})

	t.Run("Filters destinations on targeted repost", func(t *testing.T) {
		mockDB := &MockDatabase{
			GetUserFunc: func(ctx context.Context, id string) (*user.Record, error) {
				return &user.Record{UserProfile: &pbuser.UserProfile{UserId: id}}, nil
			},
			GetUserPipelinesFunc: func(ctx context.Context, userId string) ([]*pbpipeline.PipelineConfig, error) {
				return []*pbpipeline.PipelineConfig{
					{
						Id:           "pipeline-repost",
						Source:       "SOURCE_HEVY",
						Destinations: []pbplugin.DestinationType{pbplugin.DestinationType_DESTINATION_STRAVA, pbplugin.DestinationType_DESTINATION_INTERVALS},
						Enrichers: []*pbpipeline.EnricherConfig{
							{
								ProviderType: pbplugin.EnricherProviderType_ENRICHER_PROVIDER_MOCK,
							},
						},
					},
				}, nil
			},
		}

		orchestrator := NewOrchestrator(mockDB, &MockBlobStore{}, "test-bucket", nil)
		orchestrator.Register(&MockProvider{})

		pipelineID := "pipeline-repost"
		payload := &pbevents.ActivityPayload{
			UserId:            "user-123",
			Source:            pbactivity.ActivitySource_SOURCE_HEVY,
			PipelineId:        &pipelineID,
			IsRepost:          true,
			RepostMode:        "missed-destination",
			RepostDestination: "DESTINATION_INTERVALS",
			Timestamp:         timestamppb.New(time.Date(2023, 1, 1, 10, 0, 0, 0, time.UTC)),
			StandardizedActivity: &pbactivity.StandardizedActivity{
				Name: "Ghost Run",
				Sessions: []*pbactivity.Session{
					{
						StartTime:        timestamppb.New(time.Date(2023, 1, 1, 10, 0, 0, 0, time.UTC)),
						TotalElapsedTime: 60,
					},
				},
			},
		}

		result, err := orchestrator.Process(ctx, slog.Default(), payload, "test-parent-exec", "test-pipeline-exec", false)

		if err != nil {
			t.Fatalf("Process failed: %v", err)
		}

		if len(result.Events) != 1 {
			t.Fatalf("Expected 1 event, got %d", len(result.Events))
		}

		event := result.Events[0]
		if len(event.Destinations) != 1 {
			t.Fatalf("Expected destinations to be filtered to 1, got %d", len(event.Destinations))
		}

		if event.Destinations[0] != pbplugin.DestinationType_DESTINATION_INTERVALS {
			t.Errorf("Expected destination DESTINATION_INTERVALS, got %v", event.Destinations[0])
		}
	})
}

// MockDeferrableProvider implements both providers.Provider and providers.DeferrableProvider
type MockDeferrableProvider struct {
	NameFunc         func() string
	ProviderTypeFunc func() pbplugin.EnricherProviderType
	EnrichFunc       func(ctx context.Context, logger *slog.Logger, activity *pbactivity.StandardizedActivity, user *user.Record, inputConfig map[string]string, doNotRetry bool) (*providers.EnrichmentResult, error)
	ShouldDeferFunc  func() bool
}

func (m *MockDeferrableProvider) Name() string {
	if m.NameFunc != nil {
		return m.NameFunc()
	}
	return "mock-deferrable"
}

func (m *MockDeferrableProvider) ProviderType() pbplugin.EnricherProviderType {
	if m.ProviderTypeFunc != nil {
		return m.ProviderTypeFunc()
	}
	return pbplugin.EnricherProviderType_ENRICHER_PROVIDER_MOCK
}

func (m *MockDeferrableProvider) Enrich(ctx context.Context, logger *slog.Logger, activity *pbactivity.StandardizedActivity, user *user.Record, inputConfig map[string]string, doNotRetry bool) (*providers.EnrichmentResult, error) {
	if m.EnrichFunc != nil {
		return m.EnrichFunc(ctx, logger, activity, user, inputConfig, doNotRetry)
	}
	return &providers.EnrichmentResult{}, nil
}

func (m *MockDeferrableProvider) ShouldDefer() bool {
	if m.ShouldDeferFunc != nil {
		return m.ShouldDeferFunc()
	}
	return true
}

func TestOrchestrator_DeferredExecution(t *testing.T) {
	ctx := context.Background()

	t.Run("Deferred enricher receives enriched_description", func(t *testing.T) {
		// Pipeline: [normalProvider(WEATHER), deferredProvider(AI_COMPANION)]
		// Normal provider contributes description "Weather: Sunny"
		// Deferred provider should receive "Weather: Sunny" in its enriched_description config
		var capturedConfig map[string]string

		mockDB := &MockDatabase{
			GetUserFunc: func(ctx context.Context, id string) (*user.Record, error) {
				return &user.Record{UserProfile: &pbuser.UserProfile{UserId: id}}, nil
			},
			GetUserPipelinesFunc: func(ctx context.Context, userId string) ([]*pbpipeline.PipelineConfig, error) {
				return []*pbpipeline.PipelineConfig{
					{
						Id:           "p1",
						Source:       "SOURCE_HEVY",
						Destinations: []pbplugin.DestinationType{pbplugin.DestinationType_DESTINATION_STRAVA},
						Enrichers: []*pbpipeline.EnricherConfig{
							{ProviderType: pbplugin.EnricherProviderType_ENRICHER_PROVIDER_WEATHER},
							{ProviderType: pbplugin.EnricherProviderType_ENRICHER_PROVIDER_AI_COMPANION},
						},
					},
				}, nil
			},
		}

		normalProvider := &MockProvider{
			NameFunc:         func() string { return "weather" },
			ProviderTypeFunc: func() pbplugin.EnricherProviderType { return pbplugin.EnricherProviderType_ENRICHER_PROVIDER_WEATHER },
			EnrichFunc: func(ctx context.Context, _ *slog.Logger, activity *pbactivity.StandardizedActivity, user *user.Record, inputConfig map[string]string, doNotRetry bool) (*providers.EnrichmentResult, error) {
				return &providers.EnrichmentResult{
					Description: "☀️ Weather: Sunny, 22°C",
				}, nil
			},
		}

		deferredProvider := &MockDeferrableProvider{
			NameFunc: func() string { return "ai-companion" },
			ProviderTypeFunc: func() pbplugin.EnricherProviderType {
				return pbplugin.EnricherProviderType_ENRICHER_PROVIDER_AI_COMPANION
			},
			EnrichFunc: func(ctx context.Context, _ *slog.Logger, activity *pbactivity.StandardizedActivity, user *user.Record, inputConfig map[string]string, doNotRetry bool) (*providers.EnrichmentResult, error) {
				capturedConfig = inputConfig
				return &providers.EnrichmentResult{
					Description: "✨ AI Summary: Great sunny run!",
				}, nil
			},
		}

		orchestrator := NewOrchestrator(mockDB, &MockBlobStore{}, "test-bucket", nil)
		orchestrator.Register(normalProvider)
		orchestrator.Register(deferredProvider)

		pipelineID := "p1"
		payload := &pbevents.ActivityPayload{
			UserId:     "user-1",
			Source:     pbactivity.ActivitySource_SOURCE_HEVY,
			PipelineId: &pipelineID,
			Timestamp:  timestamppb.New(time.Date(2023, 1, 1, 10, 0, 0, 0, time.UTC)),
			StandardizedActivity: &pbactivity.StandardizedActivity{
				Name: "Morning Run",
				Sessions: []*pbactivity.Session{
					{
						StartTime:        timestamppb.New(time.Date(2023, 1, 1, 10, 0, 0, 0, time.UTC)),
						TotalElapsedTime: 60,
					},
				},
			},
		}

		result, err := orchestrator.Process(ctx, slog.Default(), payload, "exec-1", "pipe-exec-1", false)
		if err != nil {
			t.Fatalf("Process failed: %v", err)
		}

		// Verify deferred provider received enriched_description
		if capturedConfig == nil {
			t.Fatal("Deferred provider was not called")
		}
		enrichedDesc := capturedConfig["enriched_description"]
		if !strings.Contains(enrichedDesc, "Weather: Sunny") {
			t.Errorf("Expected enriched_description to contain weather description, got: %q", enrichedDesc)
		}

		// Verify final description has both (in pipeline order)
		if len(result.Events) != 1 {
			t.Fatalf("Expected 1 event, got %d", len(result.Events))
		}
		desc := result.Events[0].Description
		weatherIdx := strings.Index(desc, "Weather")
		aiIdx := strings.Index(desc, "AI Summary")
		if weatherIdx < 0 || aiIdx < 0 {
			t.Errorf("Expected both descriptions in output, got: %q", desc)
		}
		if weatherIdx > aiIdx {
			t.Errorf("Expected Weather before AI Summary (pipeline order), got: %q", desc)
		}
	})

	t.Run("Description ordering preserves pipeline position", func(t *testing.T) {
		// Pipeline: [deferredProvider, normalProvider]
		// Even though deferred runs AFTER normal, its description should appear FIRST
		mockDB := &MockDatabase{
			GetUserFunc: func(ctx context.Context, id string) (*user.Record, error) {
				return &user.Record{UserProfile: &pbuser.UserProfile{UserId: id}}, nil
			},
			GetUserPipelinesFunc: func(ctx context.Context, userId string) ([]*pbpipeline.PipelineConfig, error) {
				return []*pbpipeline.PipelineConfig{
					{
						Id:           "p1",
						Source:       "SOURCE_HEVY",
						Destinations: []pbplugin.DestinationType{pbplugin.DestinationType_DESTINATION_STRAVA},
						Enrichers: []*pbpipeline.EnricherConfig{
							{ProviderType: pbplugin.EnricherProviderType_ENRICHER_PROVIDER_AI_COMPANION}, // Deferred, but FIRST in pipeline
							{ProviderType: pbplugin.EnricherProviderType_ENRICHER_PROVIDER_WEATHER},      // Normal, SECOND in pipeline
						},
					},
				}, nil
			},
		}

		executionOrder := []string{}

		deferredProvider := &MockDeferrableProvider{
			NameFunc: func() string { return "ai-companion" },
			ProviderTypeFunc: func() pbplugin.EnricherProviderType {
				return pbplugin.EnricherProviderType_ENRICHER_PROVIDER_AI_COMPANION
			},
			EnrichFunc: func(ctx context.Context, _ *slog.Logger, activity *pbactivity.StandardizedActivity, user *user.Record, inputConfig map[string]string, doNotRetry bool) (*providers.EnrichmentResult, error) {
				executionOrder = append(executionOrder, "ai-companion")
				return &providers.EnrichmentResult{
					Description: "✨ AI Summary",
				}, nil
			},
		}

		normalProvider := &MockProvider{
			NameFunc:         func() string { return "weather" },
			ProviderTypeFunc: func() pbplugin.EnricherProviderType { return pbplugin.EnricherProviderType_ENRICHER_PROVIDER_WEATHER },
			EnrichFunc: func(ctx context.Context, _ *slog.Logger, activity *pbactivity.StandardizedActivity, user *user.Record, inputConfig map[string]string, doNotRetry bool) (*providers.EnrichmentResult, error) {
				executionOrder = append(executionOrder, "weather")
				return &providers.EnrichmentResult{
					Description: "☀️ Weather",
				}, nil
			},
		}

		orchestrator := NewOrchestrator(mockDB, &MockBlobStore{}, "test-bucket", nil)
		orchestrator.Register(deferredProvider)
		orchestrator.Register(normalProvider)

		pipelineID := "p1"
		payload := &pbevents.ActivityPayload{
			UserId:     "user-1",
			Source:     pbactivity.ActivitySource_SOURCE_HEVY,
			PipelineId: &pipelineID,
			Timestamp:  timestamppb.New(time.Date(2023, 1, 1, 10, 0, 0, 0, time.UTC)),
			StandardizedActivity: &pbactivity.StandardizedActivity{
				Name: "Morning Run",
				Sessions: []*pbactivity.Session{
					{
						StartTime:        timestamppb.New(time.Date(2023, 1, 1, 10, 0, 0, 0, time.UTC)),
						TotalElapsedTime: 60,
					},
				},
			},
		}

		result, err := orchestrator.Process(ctx, slog.Default(), payload, "exec-1", "pipe-exec-1", false)
		if err != nil {
			t.Fatalf("Process failed: %v", err)
		}

		// Verify execution order: weather runs first (Phase 1), ai-companion runs second (Phase 2)
		if len(executionOrder) != 2 {
			t.Fatalf("Expected 2 executions, got %d: %v", len(executionOrder), executionOrder)
		}
		if executionOrder[0] != "weather" {
			t.Errorf("Expected weather to execute first (Phase 1), got %s", executionOrder[0])
		}
		if executionOrder[1] != "ai-companion" {
			t.Errorf("Expected ai-companion to execute second (Phase 2), got %s", executionOrder[1])
		}

		// Verify description ordering: AI Summary appears FIRST (pipeline position 0), Weather SECOND (pipeline position 1)
		if len(result.Events) != 1 {
			t.Fatalf("Expected 1 event, got %d", len(result.Events))
		}
		desc := result.Events[0].Description
		aiIdx := strings.Index(desc, "AI Summary")
		weatherIdx := strings.Index(desc, "Weather")
		if aiIdx < 0 || weatherIdx < 0 {
			t.Errorf("Expected both descriptions in output, got: %q", desc)
		}
		if aiIdx > weatherIdx {
			t.Errorf("Expected AI Summary BEFORE Weather (pipeline order, not execution order), got: %q", desc)
		}
	})
}

// TestMergeEnrichmentsAllFields verifies that mergeEnrichments preserves every
// message-valued field defined on ActivityEnrichments. It uses protoreflect to
// enumerate fields automatically, so the test stays correct when new enrichment
// fields are added to the proto without any manual update here.
func TestMergeEnrichmentsAllFields(t *testing.T) {
	src := &pbactivity.ActivityEnrichments{}
	srcReflect := src.ProtoReflect()
	fields := srcReflect.Descriptor().Fields()

	// Set every message-valued field to a non-nil (empty) instance.
	// protoregistry.GlobalTypes resolves the concrete MessageType from the
	// descriptor so we can call New() on it.
	for i := 0; i < fields.Len(); i++ {
		fd := fields.Get(i)
		if fd.Kind() != protoreflect.MessageKind {
			continue
		}
		mt, err := protoregistry.GlobalTypes.FindMessageByName(fd.Message().FullName())
		if err != nil {
			t.Fatalf("field %q: cannot resolve MessageType: %v", fd.FullName(), err)
		}
		srcReflect.Set(fd, protoreflect.ValueOfMessage(mt.New()))
	}

	result := mergeEnrichments(nil, src)
	if result == nil {
		t.Fatal("mergeEnrichments returned nil for non-nil src")
	}

	resultReflect := result.ProtoReflect()
	for i := 0; i < fields.Len(); i++ {
		fd := fields.Get(i)
		if fd.Kind() == protoreflect.MessageKind && srcReflect.Has(fd) {
			if !resultReflect.Has(fd) {
				t.Errorf("mergeEnrichments dropped field %q — update the merge function to include it", fd.FullName())
			}
		}
	}
}
