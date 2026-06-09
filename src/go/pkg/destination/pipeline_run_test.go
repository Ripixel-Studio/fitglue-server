package destination

import (
	"context"
	"testing"

	cloudevents "github.com/cloudevents/sdk-go/v2"
	pbplugin "github.com/fitglue/server/src/go/pkg/types/pb/models/plugin"

	pbpipeline "github.com/fitglue/server/src/go/pkg/types/pb/models/pipeline"

	"github.com/fitglue/server/src/go/internal/infra"
	pbnotification "github.com/fitglue/server/src/go/pkg/types/pb/models/notification"
	"google.golang.org/protobuf/encoding/protojson"
)

// --- Mock Database ---

type MockDatabase struct {
	Outcomes       []*pbpipeline.DestinationOutcome
	SetOutcomeFunc func(ctx context.Context, userId string, pipelineRunId string, outcome *pbpipeline.DestinationOutcome) error
	UpdateRunFunc  func(ctx context.Context, userId string, id string, data map[string]interface{}) error
}

func (m *MockDatabase) SetDestinationOutcome(ctx context.Context, userId string, pipelineRunId string, outcome *pbpipeline.DestinationOutcome) error {
	if m.SetOutcomeFunc != nil {
		return m.SetOutcomeFunc(ctx, userId, pipelineRunId, outcome)
	}
	m.Outcomes = append(m.Outcomes, outcome)
	return nil
}

func (m *MockDatabase) GetDestinationOutcomes(ctx context.Context, userId string, pipelineRunId string) ([]*pbpipeline.DestinationOutcome, error) {
	return m.Outcomes, nil
}

func (m *MockDatabase) UpdatePipelineRun(ctx context.Context, userId string, id string, data map[string]interface{}) error {
	if m.UpdateRunFunc != nil {
		return m.UpdateRunFunc(ctx, userId, id, data)
	}
	return nil
}

// --- Mock Publisher ---

type MockPublisher struct {
	Published [][]byte
}

func (m *MockPublisher) PublishCloudEvent(_ context.Context, _ string, _ cloudevents.Event) (string, error) {
	return "msg-id", nil
}

func (m *MockPublisher) PublishJSON(_ context.Context, _ string, data []byte) error {
	m.Published = append(m.Published, data)
	return nil
}

func (m *MockPublisher) lastNotification(t *testing.T) *pbnotification.NotificationRequest {
	t.Helper()
	if len(m.Published) == 0 {
		return nil
	}
	var req pbnotification.NotificationRequest
	if err := protojson.Unmarshal(m.Published[len(m.Published)-1], &req); err != nil {
		t.Fatalf("failed to unmarshal notification: %v", err)
	}
	return &req
}

// --- Tests ---

func TestComputePipelineRunStatus_Empty(t *testing.T) {
	status := ComputePipelineRunStatus(nil, nil)
	if status != pbpipeline.PipelineRunStatus_PIPELINE_RUN_STATUS_RUNNING {
		t.Errorf("expected RUNNING for empty outcomes, got %v", status)
	}
}

func TestComputePipelineRunStatus_AllSuccess(t *testing.T) {
	outcomes := []*pbpipeline.DestinationOutcome{
		{Destination: pbplugin.DestinationType_DESTINATION_STRAVA, Status: pbpipeline.DestinationStatus_DESTINATION_STATUS_SUCCESS},
		{Destination: pbplugin.DestinationType_DESTINATION_HEVY, Status: pbpipeline.DestinationStatus_DESTINATION_STATUS_SUCCESS},
	}
	status := ComputePipelineRunStatus(outcomes, nil)
	if status != pbpipeline.PipelineRunStatus_PIPELINE_RUN_STATUS_SYNCED {
		t.Errorf("expected SYNCED, got %v", status)
	}
}

func TestComputePipelineRunStatus_SomePending(t *testing.T) {
	outcomes := []*pbpipeline.DestinationOutcome{
		{Destination: pbplugin.DestinationType_DESTINATION_STRAVA, Status: pbpipeline.DestinationStatus_DESTINATION_STATUS_SUCCESS},
		{Destination: pbplugin.DestinationType_DESTINATION_HEVY, Status: pbpipeline.DestinationStatus_DESTINATION_STATUS_PENDING},
	}
	status := ComputePipelineRunStatus(outcomes, nil)
	if status != pbpipeline.PipelineRunStatus_PIPELINE_RUN_STATUS_RUNNING {
		t.Errorf("expected RUNNING, got %v", status)
	}
}

func TestComputePipelineRunStatus_SomeFailed(t *testing.T) {
	outcomes := []*pbpipeline.DestinationOutcome{
		{Destination: pbplugin.DestinationType_DESTINATION_STRAVA, Status: pbpipeline.DestinationStatus_DESTINATION_STATUS_SUCCESS},
		{Destination: pbplugin.DestinationType_DESTINATION_HEVY, Status: pbpipeline.DestinationStatus_DESTINATION_STATUS_FAILED},
	}
	status := ComputePipelineRunStatus(outcomes, nil)
	if status != pbpipeline.PipelineRunStatus_PIPELINE_RUN_STATUS_PARTIAL {
		t.Errorf("expected PARTIAL, got %v", status)
	}
}

func TestComputePipelineRunStatus_SuccessAndSkipped(t *testing.T) {
	outcomes := []*pbpipeline.DestinationOutcome{
		{Destination: pbplugin.DestinationType_DESTINATION_STRAVA, Status: pbpipeline.DestinationStatus_DESTINATION_STATUS_SUCCESS},
		{Destination: pbplugin.DestinationType_DESTINATION_HEVY, Status: pbpipeline.DestinationStatus_DESTINATION_STATUS_SKIPPED},
	}
	status := ComputePipelineRunStatus(outcomes, nil)
	if status != pbpipeline.PipelineRunStatus_PIPELINE_RUN_STATUS_SYNCED {
		t.Errorf("expected SYNCED (skipped doesn't count as failure), got %v", status)
	}
}

func TestUpdateStatus_EnqueuesNotificationOnSynced(t *testing.T) {
	pub := &MockPublisher{}
	db := &MockDatabase{
		Outcomes: []*pbpipeline.DestinationOutcome{
			{Destination: pbplugin.DestinationType_DESTINATION_STRAVA, Status: pbpipeline.DestinationStatus_DESTINATION_STATUS_SUCCESS},
		},
	}
	logger := infra.NewLogger()

	UpdateStatus(context.Background(), db, pub, "user1", "run1",
		pbplugin.DestinationType_DESTINATION_HEVY, pbpipeline.DestinationStatus_DESTINATION_STATUS_SUCCESS,
		"hevy-123", "", "Morning Run", "activity-1", logger, nil)

	n := pub.lastNotification(t)
	if n == nil {
		t.Fatal("expected notification to be enqueued")
	}
	if n.Type != pbnotification.NotificationType_NOTIFICATION_TYPE_PIPELINE_SUCCESS {
		t.Errorf("expected PIPELINE_SUCCESS, got %v", n.Type)
	}
	if n.Title != "Activity Synced: Morning Run" {
		t.Errorf("unexpected title: %s", n.Title)
	}
	if n.Data["activity_id"] != "activity-1" {
		t.Errorf("expected activity_id 'activity-1', got: %s", n.Data["activity_id"])
	}
}

func TestUpdateStatus_EnqueuesNotificationOnPartial(t *testing.T) {
	pub := &MockPublisher{}
	db := &MockDatabase{
		Outcomes: []*pbpipeline.DestinationOutcome{
			{Destination: pbplugin.DestinationType_DESTINATION_STRAVA, Status: pbpipeline.DestinationStatus_DESTINATION_STATUS_SUCCESS},
		},
	}
	logger := infra.NewLogger()

	UpdateStatus(context.Background(), db, pub, "user1", "run1",
		pbplugin.DestinationType_DESTINATION_HEVY, pbpipeline.DestinationStatus_DESTINATION_STATUS_FAILED,
		"", "API error", "Morning Run", "activity-2", logger, nil)

	n := pub.lastNotification(t)
	if n == nil {
		t.Fatal("expected notification to be enqueued")
	}
	if n.Type != pbnotification.NotificationType_NOTIFICATION_TYPE_PIPELINE_FAILURE {
		t.Errorf("expected PIPELINE_FAILURE, got %v", n.Type)
	}
	if n.Title != "Partial Sync: Morning Run" {
		t.Errorf("unexpected title: %s", n.Title)
	}
}

func TestUpdateStatus_NoNotificationWhileRunning(t *testing.T) {
	pub := &MockPublisher{}
	db := &MockDatabase{
		Outcomes: []*pbpipeline.DestinationOutcome{
			{Destination: pbplugin.DestinationType_DESTINATION_STRAVA, Status: pbpipeline.DestinationStatus_DESTINATION_STATUS_PENDING},
		},
	}
	logger := infra.NewLogger()

	// Only Hevy completes — Strava still pending → RUNNING → no notification
	UpdateStatus(context.Background(), db, pub, "user1", "run1",
		pbplugin.DestinationType_DESTINATION_HEVY, pbpipeline.DestinationStatus_DESTINATION_STATUS_SUCCESS,
		"hevy-123", "", "Morning Run", "activity-3", logger, nil)

	if len(pub.Published) != 0 {
		t.Fatalf("expected 0 notifications while RUNNING, got %d", len(pub.Published))
	}
}

func TestUpdateStatus_NilPublisher(t *testing.T) {
	db := &MockDatabase{Outcomes: []*pbpipeline.DestinationOutcome{}}
	logger := infra.NewLogger()

	// Should not panic with nil publisher
	UpdateStatus(context.Background(), db, nil, "user1", "run1",
		pbplugin.DestinationType_DESTINATION_STRAVA, pbpipeline.DestinationStatus_DESTINATION_STATUS_SUCCESS,
		"strava-123", "", "Morning Run", "activity-4", logger, nil)
}

func TestUpdateStatus_NoPipelineRunId(t *testing.T) {
	pub := &MockPublisher{}
	db := &MockDatabase{}
	logger := infra.NewLogger()

	// Empty pipelineRunId → early return (legacy flow)
	UpdateStatus(context.Background(), db, pub, "user1", "",
		pbplugin.DestinationType_DESTINATION_STRAVA, pbpipeline.DestinationStatus_DESTINATION_STATUS_SUCCESS,
		"strava-123", "", "Morning Run", "activity-5", logger, nil)

	if len(pub.Published) != 0 {
		t.Fatalf("expected 0 notifications for empty pipelineRunId, got %d", len(pub.Published))
	}
}

func TestFormatDestinationName(t *testing.T) {
	tests := []struct {
		dest     pbplugin.DestinationType
		expected string
	}{
		{pbplugin.DestinationType_DESTINATION_STRAVA, "Strava"},
		{pbplugin.DestinationType_DESTINATION_HEVY, "Hevy"},
		{pbplugin.DestinationType_DESTINATION_SHOWCASE, "Showcase"},
		{pbplugin.DestinationType_DESTINATION_TRAININGPEAKS, "TrainingPeaks"},
		{pbplugin.DestinationType_DESTINATION_INTERVALS, "Intervals.icu"},
		{pbplugin.DestinationType_DESTINATION_GOOGLESHEETS, "Google Sheets"},
		{pbplugin.DestinationType_DESTINATION_GITHUB, "GitHub"},
		{pbplugin.DestinationType_DESTINATION_MOCK, "Mock"},
	}
	for _, tt := range tests {
		t.Run(tt.expected, func(t *testing.T) {
			got := FormatDestinationName(tt.dest)
			if got != tt.expected {
				t.Errorf("FormatDestinationName(%v) = %q, want %q", tt.dest, got, tt.expected)
			}
		})
	}
}
