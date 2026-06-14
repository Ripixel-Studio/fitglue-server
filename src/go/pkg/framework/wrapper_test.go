// nolint:proto-json
package framework

import (
	user "github.com/fitglue/server/src/go/pkg/domain/user"

	pbuser "github.com/fitglue/server/src/go/pkg/types/pb/models/user"

	pbplugin "github.com/fitglue/server/src/go/pkg/types/pb/models/plugin"

	pbpipeline "github.com/fitglue/server/src/go/pkg/types/pb/models/pipeline"

	pbactivity "github.com/fitglue/server/src/go/pkg/types/pb/models/activity"

	"context"
	"encoding/json"
	"errors"
	"testing"

	"github.com/cloudevents/sdk-go/v2/event"
	shared "github.com/fitglue/server/src/go/pkg"
	"github.com/fitglue/server/src/go/pkg/bootstrap"
	"github.com/fitglue/server/src/go/pkg/types"
)

// MockDB for Wrapper Test
type MockDB struct {
	SetExecutionFunc    func(ctx context.Context, record *pbpipeline.ExecutionRecord) error
	UpdateExecutionFunc func(ctx context.Context, userId string, id string, data map[string]interface{}) error
}

func (m *MockDB) SetExecution(ctx context.Context, record *pbpipeline.ExecutionRecord) error {
	if m.SetExecutionFunc != nil {
		return m.SetExecutionFunc(ctx, record)
	}
	return nil
}
func (m *MockDB) UpdateExecution(ctx context.Context, userId string, id string, data map[string]interface{}) error {
	if m.UpdateExecutionFunc != nil {
		return m.UpdateExecutionFunc(ctx, userId, id, data)
	}
	return nil
}
func (m *MockDB) GetUser(ctx context.Context, id string) (*user.Record, error) {
	return nil, nil
}
func (m *MockDB) UpdateUser(ctx context.Context, id string, data map[string]interface{}) error {
	return nil
}
func (m *MockDB) CreatePendingInput(ctx context.Context, userId string, input *pbpipeline.PendingInput) error {
	return nil
}
func (m *MockDB) GetPendingInput(ctx context.Context, userId string, id string) (*pbpipeline.PendingInput, error) {
	return nil, nil
}
func (m *MockDB) UpdatePendingInput(ctx context.Context, userId string, id string, data map[string]interface{}) error {
	return nil
}
func (m *MockDB) ListPendingInputs(ctx context.Context, userID string) ([]*pbpipeline.PendingInput, error) {
	return nil, nil
}
func (m *MockDB) DeletePendingInput(ctx context.Context, userId string, id string) error {
	return nil
}
func (m *MockDB) GetCounter(ctx context.Context, userId string, id string) (*pbuser.Counter, error) {
	return nil, nil
}
func (m *MockDB) SetCounter(ctx context.Context, userId string, counter *pbuser.Counter) error {
	return nil
}
func (m *MockDB) ListCounters(ctx context.Context, userId string) ([]*pbuser.Counter, error) {
	return nil, nil
}
func (m *MockDB) DeleteCounter(ctx context.Context, userId string, id string) error {
	return nil
}
func (m *MockDB) RecordBillingEvent(_ context.Context, _ string, _ shared.BillingEvent) error {
	return nil
}
func (m *MockDB) CountBillingEvents(_ context.Context, _ string) (int32, error) {
	return 0, nil
}
func (m *MockDB) CountBillingEventsForPeriod(_ context.Context, _, _ string) (int32, error) {
	return 0, nil
}
func (m *MockDB) IncrementSyncCount(ctx context.Context, userID string) error {
	return nil
}
func (m *MockDB) IncrementPreventedSyncCount(ctx context.Context, userID string) error {
	return nil
}
func (m *MockDB) ResetSyncCount(ctx context.Context, userID string) error {
	return nil
}
func (m *MockDB) ListPendingInputsByEnricher(ctx context.Context, enricherId string, status pbpipeline.PendingInput_Status) ([]*pbpipeline.PendingInput, error) {
	return nil, nil
}
func (m *MockDB) ShowcaseActivityExists(ctx context.Context, showcaseId string) (bool, error) {
	return false, nil
}
func (m *MockDB) SetShowcasedActivity(ctx context.Context, userID string, activity *pbactivity.ShowcasedActivity) error {
	return nil
}
func (m *MockDB) GetShowcasedActivity(ctx context.Context, showcaseId string) (*pbactivity.ShowcasedActivity, error) {
	return nil, nil
}
func (m *MockDB) GetShowcasedActivityByPipelineExecutionId(ctx context.Context, pipelineExecutionId string) (*pbactivity.ShowcasedActivity, error) {
	return nil, nil
}
func (m *MockDB) SetShowcaseProfile(ctx context.Context, profile *pbactivity.ShowcaseProfile) error {
	return nil
}
func (m *MockDB) GetShowcaseProfile(ctx context.Context, slug string) (*pbactivity.ShowcaseProfile, error) {
	return nil, nil
}
func (m *MockDB) GetShowcaseProfileByUserId(ctx context.Context, userId string) (*pbactivity.ShowcaseProfile, error) {
	return nil, nil
}
func (m *MockDB) DeleteShowcaseProfile(ctx context.Context, slug string) error {
	return nil
}
func (m *MockDB) SetShowcaseProfileEntry(ctx context.Context, userID string, entry *pbactivity.ShowcaseProfileEntry) error {
	return nil
}
func (m *MockDB) GetPersonalRecord(ctx context.Context, userId string, recordType string) (*pbuser.PersonalRecord, error) {
	return nil, nil
}
func (m *MockDB) SetPersonalRecord(ctx context.Context, userId string, record *pbuser.PersonalRecord) error {
	return nil
}
func (m *MockDB) ListPersonalRecords(ctx context.Context, userId string) ([]*pbuser.PersonalRecord, error) {
	return nil, nil
}
func (m *MockDB) DeletePersonalRecord(ctx context.Context, userId string, recordType string) error {
	return nil
}
func (m *MockDB) GetUserPipelines(ctx context.Context, userId string) ([]*pbpipeline.PipelineConfig, error) {
	return []*pbpipeline.PipelineConfig{}, nil
}
func (m *MockDB) GetPluginDefault(ctx context.Context, userId string, pluginId string) (*pbpipeline.PluginDefault, error) {
	return nil, nil
}
func (m *MockDB) SetPluginDefault(ctx context.Context, userId string, pluginDefault *pbpipeline.PluginDefault) error {
	return nil
}
func (m *MockDB) SetUploadedActivity(ctx context.Context, userId string, record *pbactivity.UploadedActivityRecord) error {
	return nil
}
func (m *MockDB) GetUploadedActivity(ctx context.Context, userId string, destination pbplugin.DestinationType, destinationId string) (*pbactivity.UploadedActivityRecord, error) {
	return nil, nil
}
func (m *MockDB) CreatePipelineRun(ctx context.Context, userId string, run *pbpipeline.PipelineRun) error {
	return nil
}
func (m *MockDB) GetPipelineRun(ctx context.Context, userId string, id string) (*pbpipeline.PipelineRun, error) {
	return nil, nil
}
func (m *MockDB) GetPipelineRunByActivityId(ctx context.Context, userId string, activityId string) (*pbpipeline.PipelineRun, error) {
	return nil, nil
}
func (m *MockDB) UpdatePipelineRun(ctx context.Context, userId string, id string, data map[string]interface{}) error {
	return nil
}
func (m *MockDB) SetDestinationOutcome(ctx context.Context, userId string, pipelineRunId string, outcome *pbpipeline.DestinationOutcome) error {
	return nil
}
func (m *MockDB) GetDestinationOutcomes(ctx context.Context, userId string, pipelineRunId string) ([]*pbpipeline.DestinationOutcome, error) {
	return nil, nil
}
func (m *MockDB) GetBoosterData(ctx context.Context, userId string, boosterId string) (map[string]interface{}, error) {
	return nil, nil
}
func (m *MockDB) SetBoosterData(ctx context.Context, userId string, boosterId string, data map[string]interface{}) error {
	return nil
}
func (m *MockDB) DeleteBoosterData(ctx context.Context, userId string, boosterId string) error {
	return nil
}

// Update Wrapper Test to expect metadata in LogStart updates
func TestWrapCloudEvent(t *testing.T) {
	mockDB := &MockDB{
		SetExecutionFunc: func(ctx context.Context, record *pbpipeline.ExecutionRecord) error {
			if record.Status != pbpipeline.ExecutionStatus_STATUS_PENDING {
				t.Errorf("Expected status pending, got %v", record.Status)
			}
			return nil
		},
		UpdateExecutionFunc: func(ctx context.Context, userId string, id string, data map[string]interface{}) error {
			status, ok := data["status"].(int32)
			if !ok {
				// Check for metadata updates
				if _, ok := data["user_id"]; ok {
					return nil // User ID update
				}
				return nil // some other update
			}
			s := pbpipeline.ExecutionStatus(status)
			// Should be either STARTED or SUCCESS
			if s != pbpipeline.ExecutionStatus_STATUS_STARTED && s != pbpipeline.ExecutionStatus_STATUS_SUCCESS {
				t.Errorf("Unexpected status update: %v", s)
			}
			return nil
		},
	}
	// ... existing setup ...

	svc := &bootstrap.Service{
		DB: mockDB,
	}

	handler := func(ctx context.Context, e event.Event, fwCtx *FrameworkContext) (interface{}, error) {
		if fwCtx.Service != svc {
			t.Error("Service not injected correctly")
		}
		if fwCtx.ExecutionID == "" {
			t.Error("ExecutionID not generated")
		}
		return "ok", nil
	}

	wrapped := WrapCloudEvent("test-service", svc, handler)

	e := event.New()
	e.SetType("google.cloud.pubsub.topic.v1.messagePublished")
	e.SetSource("test-source")

	err := wrapped(context.Background(), e)
	if err != nil {
		t.Fatalf("Handler failed: %v", err)
	}
}

func TestWrapCloudEvent_Failure(t *testing.T) {
	mockDB := &MockDB{
		SetExecutionFunc: func(ctx context.Context, record *pbpipeline.ExecutionRecord) error {
			if record.Status != pbpipeline.ExecutionStatus_STATUS_PENDING {
				t.Errorf("Expected status pending, got %v", record.Status)
			}
			return nil
		},
		UpdateExecutionFunc: func(ctx context.Context, userId string, id string, data map[string]interface{}) error {
			status, ok := data["status"].(int32)
			if !ok {
				return nil
			}
			s := pbpipeline.ExecutionStatus(status)
			// Should be STARTED then FAILED
			if s != pbpipeline.ExecutionStatus_STATUS_STARTED && s != pbpipeline.ExecutionStatus_STATUS_FAILED {
				t.Errorf("Unexpected status update: %v", s)
			}
			return nil
		},
	}

	svc := &bootstrap.Service{
		DB: mockDB,
	}

	handler := func(ctx context.Context, e event.Event, fwCtx *FrameworkContext) (interface{}, error) {
		return nil, errors.New("simulated error")
	}

	wrapped := WrapCloudEvent("test-service", svc, handler)

	e := event.New()
	err := wrapped(context.Background(), e)
	if err == nil {
		t.Fatal("Expected error, got nil")
	}
}

func TestWrapCloudEvent_UnwrapsNestedEvent(t *testing.T) {
	svc := &bootstrap.Service{
		DB: &MockDB{},
	}

	expectedID := "inner-event-123"
	expectedType := "com.fitglue.activity.created"

	handler := func(ctx context.Context, e event.Event, fwCtx *FrameworkContext) (interface{}, error) {
		// Assert that 'e' is the INNER event
		if e.ID() != expectedID {
			t.Errorf("Expected event ID %s, got %s", expectedID, e.ID())
		}
		if e.Type() != expectedType {
			t.Errorf("Expected event type %s, got %s", expectedType, e.Type())
		}
		return "ok", nil
	}

	wrapped := WrapCloudEvent("test-service", svc, handler)

	// 1. Create Inner CloudEvent
	inner := event.New()
	inner.SetID(expectedID)
	inner.SetType(expectedType)
	inner.SetSource("/test/source")
	inner.SetData(event.ApplicationJSON, map[string]string{"foo": "bar"})

	innerBytes, _ := json.Marshal(inner)

	// 2. Wrap in Pub/Sub Envelope (as if coming from GCP)
	psMsg := types.PubSubMessage{
		Message: struct {
			Data       []byte            `json:"data"`
			Attributes map[string]string `json:"attributes"`
		}{
			Data: innerBytes,
		},
	}

	outer := event.New()
	outer.SetID("outer-msg-id")
	outer.SetType("google.cloud.pubsub.topic.v1.messagePublished")
	outer.SetSource("//pubsub")
	outer.SetData(event.ApplicationJSON, psMsg)

	// 3. Execute
	err := wrapped(context.Background(), outer)
	if err != nil {
		t.Fatalf("Handler failed: %v", err)
	}
}
