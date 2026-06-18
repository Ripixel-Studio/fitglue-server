package database

import (
	"context"
	"testing"
	"time"

	shared "github.com/fitglue/server/src/go/pkg"
	pbactivity "github.com/fitglue/server/src/go/pkg/types/pb/models/activity"
	pbpipeline "github.com/fitglue/server/src/go/pkg/types/pb/models/pipeline"
	pbplugin "github.com/fitglue/server/src/go/pkg/types/pb/models/plugin"
	pbuser "github.com/fitglue/server/src/go/pkg/types/pb/models/user"
)

// newAdapter builds a FirestoreAdapter backed by an unreachable emulator so every
// query fails fast. This exercises each method's query-building and error-handling
// paths without a live backend.
func newAdapter(t *testing.T) *FirestoreAdapter {
	t.Helper()
	c := offlineFS(t)
	t.Cleanup(func() { _ = c.Close() })
	return NewFirestoreAdapter(c)
}

func methodsCtx(t *testing.T) context.Context {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	t.Cleanup(cancel)
	return ctx
}

// TestFirestoreAdapterMethods drives every adapter method against a dead Firestore
// client. The objective is coverage of the request-construction and error branches;
// most calls must return an error (or, for methods that swallow "not found" into a
// nil result, must at least not panic).
func TestFirestoreAdapterMethods(t *testing.T) {
	a := newAdapter(t)
	ctx := methodsCtx(t)

	// Helper: assert a call returned an error.
	wantErr := func(name string, err error) {
		if err == nil {
			t.Errorf("%s: expected error from unreachable Firestore, got nil", name)
		}
	}

	t.Run("Executions", func(t *testing.T) {
		uid := "u1"
		wantErr("SetExecution/orphaned", a.SetExecution(ctx, &pbpipeline.ExecutionRecord{ExecutionId: "e1"}))
		wantErr("SetExecution/user", a.SetExecution(ctx, &pbpipeline.ExecutionRecord{ExecutionId: "e1", UserId: &uid}))
		wantErr("UpdateExecution/orphaned", a.UpdateExecution(ctx, "", "e1", map[string]interface{}{"x": 1}))
		wantErr("UpdateExecution/user", a.UpdateExecution(ctx, "u1", "e1", map[string]interface{}{"x": 1}))
	})

	t.Run("Users", func(t *testing.T) {
		_, err := a.GetUser(ctx, "u1")
		wantErr("GetUser", err)
		wantErr("UpdateUser", a.UpdateUser(ctx, "u1", map[string]interface{}{"x": 1}))
	})

	t.Run("BillingEvents", func(t *testing.T) {
		wantErr("RecordBillingEvent", a.RecordBillingEvent(ctx, "u1", shared.BillingEvent{ActivityID: "a1"}))
		_, err := a.CountBillingEvents(ctx, "u1")
		wantErr("CountBillingEvents", err)
		_, err = a.CountBillingEventsForPeriod(ctx, "u1", "2026-01")
		wantErr("CountBillingEventsForPeriod", err)
	})

	t.Run("SyncCount", func(t *testing.T) {
		wantErr("IncrementSyncCount", a.IncrementSyncCount(ctx, "u1"))
		wantErr("IncrementPreventedSyncCount", a.IncrementPreventedSyncCount(ctx, "u1"))
		wantErr("ResetSyncCount", a.ResetSyncCount(ctx, "u1"))
	})

	t.Run("PendingInputs", func(t *testing.T) {
		_, err := a.GetPendingInput(ctx, "u1", "p1")
		wantErr("GetPendingInput", err)
		wantErr("CreatePendingInput", a.CreatePendingInput(ctx, "u1", &pbpipeline.PendingInput{ActivityId: "a1"}))
		wantErr("UpdatePendingInput", a.UpdatePendingInput(ctx, "u1", "p1", map[string]interface{}{"x": 1}))
		wantErr("DeletePendingInput", a.DeletePendingInput(ctx, "u1", "p1"))
		_, err = a.ListPendingInputs(ctx, "u1")
		wantErr("ListPendingInputs", err)
		_, err = a.ListPendingInputsByEnricher(ctx, "enricher1", pbpipeline.PendingInput_STATUS_WAITING)
		wantErr("ListPendingInputsByEnricher", err)
	})

	t.Run("Counters", func(t *testing.T) {
		_, err := a.GetCounter(ctx, "u1", "c1")
		wantErr("GetCounter", err)
		wantErr("SetCounter", a.SetCounter(ctx, "u1", &pbuser.Counter{Id: "c1"}))
		_, err = a.ListCounters(ctx, "u1")
		wantErr("ListCounters", err)
		wantErr("DeleteCounter", a.DeleteCounter(ctx, "u1", "c1"))
	})

	t.Run("PersonalRecords", func(t *testing.T) {
		_, err := a.GetPersonalRecord(ctx, "u1", "rt1")
		wantErr("GetPersonalRecord", err)
		wantErr("SetPersonalRecord", a.SetPersonalRecord(ctx, "u1", &pbuser.PersonalRecord{RecordType: "rt1"}))
		_, err = a.ListPersonalRecords(ctx, "u1")
		wantErr("ListPersonalRecords", err)
		wantErr("DeletePersonalRecord", a.DeletePersonalRecord(ctx, "u1", "rt1"))
	})

	t.Run("Showcase", func(t *testing.T) {
		_, err := a.ShowcaseActivityExists(ctx, "s1")
		wantErr("ShowcaseActivityExists", err)
		wantErr("SetShowcasedActivity", a.SetShowcasedActivity(ctx, "u1", &pbactivity.ShowcasedActivity{ShowcaseId: "s1"}))
		_, err = a.GetShowcasedActivity(ctx, "s1")
		wantErr("GetShowcasedActivity", err)

		// GetShowcasedActivityByPipelineExecutionId swallows errors and returns nil,nil.
		act, err := a.GetShowcasedActivityByPipelineExecutionId(ctx, "pe1")
		if err != nil {
			t.Errorf("GetShowcasedActivityByPipelineExecutionId: expected nil err (swallowed), got %v", err)
		}
		if act != nil {
			t.Errorf("GetShowcasedActivityByPipelineExecutionId: expected nil activity, got %v", act)
		}

		wantErr("SetShowcaseProfile", a.SetShowcaseProfile(ctx, &pbactivity.ShowcaseProfile{Slug: "slug1"}))
		_, err = a.GetShowcaseProfile(ctx, "slug1")
		wantErr("GetShowcaseProfile", err)

		// GetShowcaseProfileByUserId swallows errors and returns nil,nil.
		prof, err := a.GetShowcaseProfileByUserId(ctx, "u1")
		if err != nil {
			t.Errorf("GetShowcaseProfileByUserId: expected nil err (swallowed), got %v", err)
		}
		if prof != nil {
			t.Errorf("GetShowcaseProfileByUserId: expected nil profile, got %v", prof)
		}

		wantErr("DeleteShowcaseProfile", a.DeleteShowcaseProfile(ctx, "slug1"))
		wantErr("SetShowcaseProfileEntry", a.SetShowcaseProfileEntry(ctx, "u1", &pbactivity.ShowcaseProfileEntry{ShowcaseId: "s1"}))
	})

	t.Run("Pipelines", func(t *testing.T) {
		_, err := a.GetUserPipelines(ctx, "u1")
		wantErr("GetUserPipelines", err)
	})

	t.Run("PluginDefaults", func(t *testing.T) {
		// GetPluginDefault returns the raw error for non-not-found failures.
		_, err := a.GetPluginDefault(ctx, "u1", "plugin1")
		wantErr("GetPluginDefault", err)
		wantErr("SetPluginDefault", a.SetPluginDefault(ctx, "u1", &pbpipeline.PluginDefault{PluginId: "plugin1"}))
	})

	t.Run("UploadedActivities", func(t *testing.T) {
		wantErr("SetUploadedActivity", a.SetUploadedActivity(ctx, "u1", &pbactivity.UploadedActivityRecord{Id: "rec1"}))
		_, err := a.GetUploadedActivity(ctx, "u1", pbplugin.DestinationType_DESTINATION_HEVY, "ext1")
		wantErr("GetUploadedActivity", err)
	})

	t.Run("DestinationClaims", func(t *testing.T) {
		_, err := a.TryClaimDestinationCreate(ctx, "u1", "claim1", time.Minute)
		wantErr("TryClaimDestinationCreate", err)
		wantErr("ReleaseDestinationCreate", a.ReleaseDestinationCreate(ctx, "u1", "claim1"))
	})

	t.Run("PipelineRuns", func(t *testing.T) {
		wantErr("CreatePipelineRun", a.CreatePipelineRun(ctx, "u1", &pbpipeline.PipelineRun{Id: "r1"}))
		_, err := a.GetPipelineRun(ctx, "u1", "r1")
		wantErr("GetPipelineRun", err)
		_, err = a.GetPipelineRunByActivityId(ctx, "u1", "a1")
		wantErr("GetPipelineRunByActivityId", err)
		wantErr("UpdatePipelineRun", a.UpdatePipelineRun(ctx, "u1", "r1", map[string]interface{}{"x": 1}))
	})

	t.Run("DestinationOutcomes", func(t *testing.T) {
		ext := "ext1"
		errStr := "boom"
		wantErr("SetDestinationOutcome", a.SetDestinationOutcome(ctx, "u1", "r1", &pbpipeline.DestinationOutcome{
			Destination: pbplugin.DestinationType_DESTINATION_STRAVA,
			Status:      pbpipeline.DestinationStatus_DESTINATION_STATUS_SUCCESS,
			ExternalId:  &ext,
			Error:       &errStr,
		}))
		_, err := a.GetDestinationOutcomes(ctx, "u1", "r1")
		wantErr("GetDestinationOutcomes", err)
	})

	t.Run("BoosterData", func(t *testing.T) {
		// GetBoosterData returns the raw error for non-not-found failures.
		_, err := a.GetBoosterData(ctx, "u1", "b1")
		wantErr("GetBoosterData", err)
		wantErr("SetBoosterData", a.SetBoosterData(ctx, "u1", "b1", map[string]interface{}{"x": 1}))
		wantErr("DeleteBoosterData", a.DeleteBoosterData(ctx, "u1", "b1"))
	})
}

// TestNotFoundHelpers covers the package-level not-found error classifiers, which
// otherwise only run on real Firestore errors.
func TestNotFoundHelpers(t *testing.T) {
	if isNotFoundError(nil) {
		t.Error("isNotFoundError(nil) should be false")
	}
	for _, s := range []string{"rpc error: code = NotFound desc = x", "document not found"} {
		if !isNotFoundError(errString(s)) {
			t.Errorf("isNotFoundError(%q) should be true", s)
		}
		if !isNotFound(errString(s)) {
			t.Errorf("isNotFound(%q) should be true", s)
		}
	}
	if isNotFoundError(errString("some other error")) {
		t.Error("isNotFoundError on unrelated error should be false")
	}
	if isNotFound(errString("some other error")) {
		t.Error("isNotFound on unrelated error should be false")
	}
}

func TestBuildUploadedActivityID(t *testing.T) {
	got := buildUploadedActivityID(pbplugin.DestinationType_DESTINATION_HEVY, "workout-xyz")
	if got != "hevy:workout-xyz" {
		t.Errorf("buildUploadedActivityID = %q, want %q", got, "hevy:workout-xyz")
	}
}

// errString is a trivial error type for exercising the not-found helpers.
type errString string

func (e errString) Error() string { return string(e) }
