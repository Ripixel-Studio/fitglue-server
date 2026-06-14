package mocks

import (
	"context"
	"fmt"

	shared "github.com/fitglue/server/src/go/pkg"
	"github.com/fitglue/server/src/go/pkg/domain/user"

	"github.com/cloudevents/sdk-go/v2/event"
	pbplugin "github.com/fitglue/server/src/go/pkg/types/pb/models/plugin"
	pbuser "github.com/fitglue/server/src/go/pkg/types/pb/models/user"

	pbactivity "github.com/fitglue/server/src/go/pkg/types/pb/models/activity"

	pbpipeline "github.com/fitglue/server/src/go/pkg/types/pb/models/pipeline"
)

// --- Mock Database ---
type MockDatabase struct {
	SetExecutionFunc    func(ctx context.Context, record *pbpipeline.ExecutionRecord) error
	UpdateExecutionFunc func(ctx context.Context, userId string, id string, data map[string]interface{}) error
	GetUserFunc         func(ctx context.Context, id string) (*user.Record, error)
	UpdateUserFunc      func(ctx context.Context, id string, data map[string]interface{}) error

	CreatePendingInputFunc func(ctx context.Context, userId string, input *pbpipeline.PendingInput) error
	GetPendingInputFunc    func(ctx context.Context, userId string, id string) (*pbpipeline.PendingInput, error)
	UpdatePendingInputFunc func(ctx context.Context, userId string, id string, data map[string]interface{}) error
	ListPendingInputsFunc  func(ctx context.Context, userID string) ([]*pbpipeline.PendingInput, error)

	GetCounterFunc       func(ctx context.Context, userId string, id string) (*pbuser.Counter, error)
	SetCounterFunc       func(ctx context.Context, userId string, counter *pbuser.Counter) error
	ListCountersFunc     func(ctx context.Context, userId string) ([]*pbuser.Counter, error)
	GetUserPipelinesFunc func(ctx context.Context, userId string) ([]*pbpipeline.PipelineConfig, error)

	GetBoosterDataFunc func(ctx context.Context, userId string, boosterId string) (map[string]interface{}, error)
	SetBoosterDataFunc func(ctx context.Context, userId string, boosterId string, data map[string]interface{}) error
}

func (m *MockDatabase) SetExecution(ctx context.Context, record *pbpipeline.ExecutionRecord) error {
	if m.SetExecutionFunc != nil {
		return m.SetExecutionFunc(ctx, record)
	}
	return nil
}
func (m *MockDatabase) UpdateExecution(ctx context.Context, userId string, id string, data map[string]interface{}) error {
	if m.UpdateExecutionFunc != nil {
		return m.UpdateExecutionFunc(ctx, userId, id, data)
	}
	return nil
}
func (m *MockDatabase) GetUser(ctx context.Context, id string) (*user.Record, error) {
	if m.GetUserFunc != nil {
		return m.GetUserFunc(ctx, id)
	}
	return nil, fmt.Errorf("user not found")
}
func (m *MockDatabase) UpdateUser(ctx context.Context, id string, data map[string]interface{}) error {
	if m.UpdateUserFunc != nil {
		return m.UpdateUserFunc(ctx, id, data)
	}
	return nil
}

func (m *MockDatabase) CreatePendingInput(ctx context.Context, userId string, input *pbpipeline.PendingInput) error {
	if m.CreatePendingInputFunc != nil {
		return m.CreatePendingInputFunc(ctx, userId, input)
	}
	return nil
}

func (m *MockDatabase) GetPendingInput(ctx context.Context, userId string, id string) (*pbpipeline.PendingInput, error) {
	if m.GetPendingInputFunc != nil {
		return m.GetPendingInputFunc(ctx, userId, id)
	}
	return nil, nil
}

func (m *MockDatabase) UpdatePendingInput(ctx context.Context, userId string, id string, data map[string]interface{}) error {
	if m.UpdatePendingInputFunc != nil {
		return m.UpdatePendingInputFunc(ctx, userId, id, data)
	}
	return nil
}

func (m *MockDatabase) ListPendingInputs(ctx context.Context, userID string) ([]*pbpipeline.PendingInput, error) {
	if m.ListPendingInputsFunc != nil {
		return m.ListPendingInputsFunc(ctx, userID)
	}
	return nil, nil
}

func (m *MockDatabase) DeletePendingInput(ctx context.Context, userId string, id string) error {
	// No-op for tests by default
	return nil
}

func (m *MockDatabase) GetCounter(ctx context.Context, userId string, id string) (*pbuser.Counter, error) {
	if m.GetCounterFunc != nil {
		return m.GetCounterFunc(ctx, userId, id)
	}
	return nil, nil
}

func (m *MockDatabase) SetCounter(ctx context.Context, userId string, counter *pbuser.Counter) error {
	if m.SetCounterFunc != nil {
		return m.SetCounterFunc(ctx, userId, counter)
	}
	return nil
}

func (m *MockDatabase) ListCounters(ctx context.Context, userId string) ([]*pbuser.Counter, error) {
	if m.ListCountersFunc != nil {
		return m.ListCountersFunc(ctx, userId)
	}
	return nil, nil
}

func (m *MockDatabase) DeleteCounter(ctx context.Context, userId string, id string) error {
	// No-op for tests by default
	return nil
}

// --- Billing Events (durable audit log) ---

func (m *MockDatabase) RecordBillingEvent(_ context.Context, _ string, _ shared.BillingEvent) error {
	return nil
}

func (m *MockDatabase) CountBillingEvents(_ context.Context, _ string) (int32, error) {
	return 0, nil
}

func (m *MockDatabase) CountBillingEventsForPeriod(_ context.Context, _, _ string) (int32, error) {
	return 0, nil
}

// --- Sync Count (for tier limits) ---

func (m *MockDatabase) IncrementSyncCount(ctx context.Context, userID string) error {
	// No-op for tests by default
	return nil
}

func (m *MockDatabase) IncrementPreventedSyncCount(ctx context.Context, userID string) error {
	// No-op for tests by default
	return nil
}

func (m *MockDatabase) ResetSyncCount(ctx context.Context, userID string) error {
	// No-op for tests by default
	return nil
}

func (m *MockDatabase) ListPendingInputsByEnricher(ctx context.Context, enricherId string, status pbpipeline.PendingInput_Status) ([]*pbpipeline.PendingInput, error) {
	// No-op for tests by default
	return nil, nil
}

// --- Showcased Activities (public shareable snapshots) ---

func (m *MockDatabase) ShowcaseActivityExists(ctx context.Context, showcaseId string) (bool, error) {
	// No-op for tests by default
	return false, nil
}

func (m *MockDatabase) SetShowcasedActivity(ctx context.Context, userID string, activity *pbactivity.ShowcasedActivity) error {
	// No-op for tests by default
	return nil
}

func (m *MockDatabase) GetShowcasedActivity(ctx context.Context, showcaseId string) (*pbactivity.ShowcasedActivity, error) {
	// No-op for tests by default
	return nil, nil
}

func (m *MockDatabase) GetShowcasedActivityByPipelineExecutionId(ctx context.Context, pipelineExecutionId string) (*pbactivity.ShowcasedActivity, error) {
	// No-op for tests by default
	return nil, nil
}

func (m *MockDatabase) SetShowcaseProfile(ctx context.Context, profile *pbactivity.ShowcaseProfile) error {
	// No-op for tests by default
	return nil
}

func (m *MockDatabase) GetShowcaseProfile(ctx context.Context, slug string) (*pbactivity.ShowcaseProfile, error) {
	// No-op for tests by default
	return nil, nil
}

func (m *MockDatabase) GetShowcaseProfileByUserId(ctx context.Context, userId string) (*pbactivity.ShowcaseProfile, error) {
	// No-op for tests by default
	return nil, nil
}

func (m *MockDatabase) DeleteShowcaseProfile(ctx context.Context, slug string) error {
	// No-op for tests by default
	return nil
}

func (m *MockDatabase) SetShowcaseProfileEntry(ctx context.Context, userID string, entry *pbactivity.ShowcaseProfileEntry) error {
	// No-op for tests by default
	return nil
}

// --- Personal Records ---

func (m *MockDatabase) GetPersonalRecord(ctx context.Context, userId string, recordType string) (*pbuser.PersonalRecord, error) {
	// No-op for tests by default
	return nil, nil
}

func (m *MockDatabase) SetPersonalRecord(ctx context.Context, userId string, record *pbuser.PersonalRecord) error {
	// No-op for tests by default
	return nil
}

func (m *MockDatabase) ListPersonalRecords(ctx context.Context, userId string) ([]*pbuser.PersonalRecord, error) {
	// No-op for tests by default
	return nil, nil
}

func (m *MockDatabase) DeletePersonalRecord(ctx context.Context, userId string, recordType string) error {
	// No-op for tests by default
	return nil
}

// --- Pipelines (Sub-collection) ---

func (m *MockDatabase) GetUserPipelines(ctx context.Context, userId string) ([]*pbpipeline.PipelineConfig, error) {
	if m.GetUserPipelinesFunc != nil {
		return m.GetUserPipelinesFunc(ctx, userId)
	}
	// No-op for tests by default - return empty slice
	return []*pbpipeline.PipelineConfig{}, nil
}

// --- Plugin Defaults (user-level default config) ---

func (m *MockDatabase) GetPluginDefault(ctx context.Context, userId string, pluginId string) (*pbpipeline.PluginDefault, error) {
	// No-op for tests by default
	return nil, nil
}

func (m *MockDatabase) SetPluginDefault(ctx context.Context, userId string, pluginDefault *pbpipeline.PluginDefault) error {
	// No-op for tests by default
	return nil
}

// --- Uploaded Activities (for loop prevention) ---

func (m *MockDatabase) SetUploadedActivity(ctx context.Context, userId string, record *pbactivity.UploadedActivityRecord) error {
	// No-op for tests by default
	return nil
}

func (m *MockDatabase) GetUploadedActivity(ctx context.Context, userId string, destination pbplugin.DestinationType, destinationId string) (*pbactivity.UploadedActivityRecord, error) {
	// No-op for tests by default - return nil (not found)
	return nil, nil
}

// --- Pipeline Runs (lifecycle tracking) ---

func (m *MockDatabase) CreatePipelineRun(ctx context.Context, userId string, run *pbpipeline.PipelineRun) error {
	// No-op for tests by default
	return nil
}

func (m *MockDatabase) GetPipelineRun(ctx context.Context, userId string, id string) (*pbpipeline.PipelineRun, error) {
	// No-op for tests by default
	return nil, nil
}

func (m *MockDatabase) GetPipelineRunByActivityId(ctx context.Context, userId string, activityId string) (*pbpipeline.PipelineRun, error) {
	// No-op for tests by default
	return nil, nil
}

func (m *MockDatabase) UpdatePipelineRun(ctx context.Context, userId string, id string, data map[string]interface{}) error {
	// No-op for tests by default
	return nil
}

// --- Destination Outcomes (subcollection of Pipeline Runs) ---

func (m *MockDatabase) SetDestinationOutcome(ctx context.Context, userId string, pipelineRunId string, outcome *pbpipeline.DestinationOutcome) error {
	// No-op for tests by default
	return nil
}

func (m *MockDatabase) GetDestinationOutcomes(ctx context.Context, userId string, pipelineRunId string) ([]*pbpipeline.DestinationOutcome, error) {
	// No-op for tests by default
	return nil, nil
}

// --- Booster Data (generic key-value storage for enrichers) ---

func (m *MockDatabase) GetBoosterData(ctx context.Context, userId string, boosterId string) (map[string]interface{}, error) {
	if m.GetBoosterDataFunc != nil {
		return m.GetBoosterDataFunc(ctx, userId, boosterId)
	}
	return nil, nil
}

func (m *MockDatabase) SetBoosterData(ctx context.Context, userId string, boosterId string, data map[string]interface{}) error {
	if m.SetBoosterDataFunc != nil {
		return m.SetBoosterDataFunc(ctx, userId, boosterId, data)
	}
	return nil
}

func (m *MockDatabase) DeleteBoosterData(ctx context.Context, userId string, boosterId string) error {
	// No-op for tests by default
	return nil
}

// --- Mock Publisher ---
type MockPublisher struct {
	PublishCloudEventFunc func(ctx context.Context, topic string, e event.Event) (string, error)
	PublishJSONFunc       func(ctx context.Context, topic string, data []byte) error
}

func (m *MockPublisher) PublishCloudEvent(ctx context.Context, topic string, e event.Event) (string, error) {
	if m.PublishCloudEventFunc != nil {
		return m.PublishCloudEventFunc(ctx, topic, e)
	}
	return "msg-id", nil
}

func (m *MockPublisher) PublishJSON(ctx context.Context, topic string, data []byte) error {
	if m.PublishJSONFunc != nil {
		return m.PublishJSONFunc(ctx, topic, data)
	}
	return nil
}

// --- Mock Storage ---
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
