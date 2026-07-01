package activity

import (
	"context"
	"testing"
	"time"

	fs "cloud.google.com/go/firestore"
	pbactivity "github.com/fitglue/server/src/go/pkg/types/pb/models/activity"
)

// deadStore builds a FirestoreStore whose client cannot reach a backend, so every
// query fails fast. This exercises the query-construction and error-handling code
// of each store method without a live emulator.
func deadStore(t *testing.T) *FirestoreStore {
	t.Helper()
	t.Setenv("FIRESTORE_EMULATOR_HOST", "127.0.0.1:1")
	c, err := fs.NewClient(context.Background(), "test-project")
	if err != nil {
		t.Skipf("firestore client: %v", err)
	}
	t.Cleanup(func() { _ = c.Close() })
	return NewFirestoreStore(c)
}

// TestFirestoreStore_Smoke calls every store method against an unreachable
// backend; each should return an error (or empty) rather than panic.
func TestFirestoreStore_Smoke(t *testing.T) {
	s := deadStore(t)
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	now := time.Now()
	showcase := &pbactivity.ShowcasedActivity{ShowcaseId: "s1"}
	profile := &pbactivity.ShowcaseProfile{Slug: "slug"}
	entry := &pbactivity.ShowcaseProfileEntry{ShowcaseId: "s1"}
	roundup := &pbactivity.ShowcaseRoundup{Slug: "slug", PeriodKey: "month-01-2024"}

	// Export jobs
	_ = s.CreateExportJob(ctx, "u1", &ExportJobRecord{JobID: "j1"})
	_, _ = s.GetExportJob(ctx, "u1", "j1")
	_ = s.UpdateExportJob(ctx, "u1", "j1", map[string]interface{}{"status": "done"})
	_, _ = s.LatestActiveExportJob(ctx, "u1")

	// Views
	_ = s.RecordShowcaseView(ctx, "activity:s1", "visitor-hash")
	_, _ = s.GetShowcaseViewStats(ctx, "activity:s1")

	// Activities
	_, _ = s.GetActivity(ctx, "u1", "a1")
	_, _, _ = s.ListActivities(ctx, "u1", 10, "")
	_ = s.DeleteActivity(ctx, "u1", "a1")

	// Pipeline runs
	_, _, _ = s.ListPipelineRuns(ctx, "u1", 10, "")
	_, _ = s.GetPipelineRun(ctx, "u1", "r1")
	_ = s.DeletePipelineRun(ctx, "u1", "r1")
	_, _ = s.CountPipelineRunsByStatus(ctx, "u1", "COMPLETED")

	// Showcases
	_, _ = s.GetShowcase(ctx, "u1", "s1")
	_, _ = s.ListShowcases(ctx, "u1")
	_, _ = s.CreateShowcase(ctx, "u1", showcase)
	_, _ = s.UpdateShowcase(ctx, "u1", showcase)
	_ = s.DeleteShowcase(ctx, "u1", "s1")
	_, _, _ = s.GetPublicShowcase(ctx, "s1")
	_, _ = s.CountShowcasedActivities(ctx, "u1")
	_, _, _ = s.ListShowcasedActivitiesByUser(ctx, "u1", 10, 0)

	// Showcase preferences / profile
	_, _ = s.GetShowcasePreferences(ctx, "u1")
	_, _ = s.UpdateShowcasePreferences(ctx, "u1", profile)
	_, _ = s.PatchShowcaseProfile(ctx, "u1", map[string]interface{}{"bio": "hi"})
	_ = s.UpdateShowcaseSlug(ctx, "u1", "slug")
	_, _ = s.GetShowcaseProfileBySlug(ctx, "slug")

	// Profile entries
	_, _ = s.ListShowcaseProfileEntries(ctx, "u1")
	_ = s.SetShowcaseProfileEntry(ctx, "u1", entry)
	_ = s.DeleteShowcaseProfileEntry(ctx, "u1", "s1")
	_, _ = s.ListShowcaseEntriesInRange(ctx, "u1", now.Add(-24*time.Hour), now)
	_, _ = s.ListAllShowcaseUserIDs(ctx)
	_, _ = s.ListUserPersonalRecords(ctx, "u1")

	// Roundups
	_, _ = s.GetRoundup(ctx, "slug", "month-01-2024")
	_ = s.SetRoundup(ctx, roundup)
	_, _ = s.ListRecentRoundups(ctx, "slug", 5)
	_, _ = s.ListAllRoundups(ctx, "slug")

	// Billing / counting
	_, _ = s.CountBillingEvents(ctx, "u1")
	_, _ = s.CountBillingEventsForPeriod(ctx, "u1", "2024-01")
	_, _ = s.CountBillingEventsSince(ctx, "u1", now.Add(-720*time.Hour))
	_, _ = s.CountDistinctActivitiesForPeriod(ctx, "u1", "2024-01")
	_, _ = s.CountDistinctActivitiesSince(ctx, "u1", now.Add(-720*time.Hour))
}
