package mocks

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/cloudevents/sdk-go/v2/event"
	shared "github.com/fitglue/server/src/go/pkg"
	"github.com/fitglue/server/src/go/pkg/domain/user"
	pbpipeline "github.com/fitglue/server/src/go/pkg/types/pb/models/pipeline"
	pbplugin "github.com/fitglue/server/src/go/pkg/types/pb/models/plugin"
)

// TestMockDatabase_DefaultBranches calls every MockDatabase method with no
// override functions set, exercising the default return paths.
func TestMockDatabase_DefaultBranches(t *testing.T) {
	m := &MockDatabase{}
	ctx := context.Background()

	_ = m.SetExecution(ctx, nil)
	_ = m.UpdateExecution(ctx, "u", "id", nil)
	_, _ = m.GetUser(ctx, "u")
	_ = m.UpdateUser(ctx, "u", nil)
	_ = m.CreatePendingInput(ctx, "u", nil)
	_, _ = m.GetPendingInput(ctx, "u", "id")
	_ = m.UpdatePendingInput(ctx, "u", "id", nil)
	_, _ = m.ListPendingInputs(ctx, "u")
	_ = m.DeletePendingInput(ctx, "u", "id")
	_, _ = m.GetCounter(ctx, "u", "id")
	_ = m.SetCounter(ctx, "u", nil)
	_, _ = m.ListCounters(ctx, "u")
	_ = m.DeleteCounter(ctx, "u", "id")
	_ = m.RecordBillingEvent(ctx, "u", shared.BillingEvent{})
	_, _ = m.CountBillingEvents(ctx, "u")
	_, _ = m.CountBillingEventsForPeriod(ctx, "u", "2024-01")
	_ = m.IncrementSyncCount(ctx, "u")
	_ = m.IncrementPreventedSyncCount(ctx, "u")
	_ = m.ResetSyncCount(ctx, "u")
	_, _ = m.ListPendingInputsByEnricher(ctx, "parkrun", pbpipeline.PendingInput_STATUS_WAITING)
	_, _ = m.ShowcaseActivityExists(ctx, "s")
	_ = m.SetShowcasedActivity(ctx, "u", nil)
	_, _ = m.GetShowcasedActivity(ctx, "s")
	_, _ = m.GetShowcasedActivityByPipelineExecutionId(ctx, "p")
	_ = m.SetShowcaseProfile(ctx, nil)
	_, _ = m.GetShowcaseProfile(ctx, "slug")
	_, _ = m.GetShowcaseProfileByUserId(ctx, "u")
	_ = m.DeleteShowcaseProfile(ctx, "slug")
	_ = m.SetShowcaseProfileEntry(ctx, "u", nil)
	_, _ = m.GetPersonalRecord(ctx, "u", "5k")
	_ = m.SetPersonalRecord(ctx, "u", nil)
	_, _ = m.ListPersonalRecords(ctx, "u")
	_ = m.DeletePersonalRecord(ctx, "u", "5k")
	_, _ = m.GetUserPipelines(ctx, "u")
	_, _ = m.GetPluginDefault(ctx, "u", "plugin")
	_ = m.SetPluginDefault(ctx, "u", nil)
	_ = m.SetUploadedActivity(ctx, "u", nil)
	_, _ = m.GetUploadedActivity(ctx, "u", pbplugin.DestinationType_DESTINATION_STRAVA, "d")
	_, _ = m.TryClaimDestinationCreate(ctx, "u", "key", time.Minute)
	_ = m.ReleaseDestinationCreate(ctx, "u", "key")
	_ = m.CreatePipelineRun(ctx, "u", nil)
	_, _ = m.GetPipelineRun(ctx, "u", "id")
	_, _ = m.GetPipelineRunByActivityId(ctx, "u", "a")
	_ = m.UpdatePipelineRun(ctx, "u", "id", nil)
	_ = m.SetDestinationOutcome(ctx, "u", "run", nil)
	_, _ = m.GetDestinationOutcomes(ctx, "u", "run")
	_, _ = m.GetBoosterData(ctx, "u", "b")
	_ = m.SetBoosterData(ctx, "u", "b", nil)
	_ = m.DeleteBoosterData(ctx, "u", "b")
}

// TestMockDatabase_OverrideFunc verifies the override path is taken when a Func is set.
func TestMockDatabase_OverrideFunc(t *testing.T) {
	want := errors.New("boom")
	m := &MockDatabase{
		GetUserFunc: func(ctx context.Context, id string) (*user.Record, error) {
			return nil, want
		},
	}
	if _, err := m.GetUser(context.Background(), "u"); err != want {
		t.Errorf("expected override error, got %v", err)
	}
}

func TestMockPublisher(t *testing.T) {
	m := &MockPublisher{}
	if id, err := m.PublishCloudEvent(context.Background(), "t", event.New()); err != nil || id != "msg-id" {
		t.Errorf("default PublishCloudEvent: id=%q err=%v", id, err)
	}
	if err := m.PublishJSON(context.Background(), "t", []byte("x")); err != nil {
		t.Errorf("default PublishJSON: %v", err)
	}

	called := false
	m.PublishJSONFunc = func(ctx context.Context, topic string, data []byte) error {
		called = true
		return nil
	}
	_ = m.PublishJSON(context.Background(), "t", nil)
	if !called {
		t.Error("expected override PublishJSON to be called")
	}
}

func TestMockBlobStore(t *testing.T) {
	m := &MockBlobStore{}
	ctx := context.Background()
	if err := m.Write(ctx, "b", "o", []byte("x")); err != nil {
		t.Errorf("Write: %v", err)
	}
	if data, err := m.Get(ctx, "b", "o"); err != nil || string(data) != "mock-data" {
		t.Errorf("Get default: %q %v", data, err)
	}
	if err := m.Delete(ctx, "b", "o"); err != nil {
		t.Errorf("Delete: %v", err)
	}
}
