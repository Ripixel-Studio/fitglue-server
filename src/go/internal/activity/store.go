package activity

import (
	"context"
	"time"

	pbactivity "github.com/fitglue/server/src/go/pkg/types/pb/models/activity"
	pbpipeline "github.com/fitglue/server/src/go/pkg/types/pb/models/pipeline"
)

// ActivityStore defines the data access contract for activity records and showcases.
type ActivityStore interface {
	GetPipelineRun(ctx context.Context, userID, runID string) (*pbpipeline.PipelineRun, error)
	ListPipelineRuns(ctx context.Context, userID string, limit int32, pageToken string) ([]*pbpipeline.PipelineRun, string, error)
	DeletePipelineRun(ctx context.Context, userID, runID string) error

	GetShowcase(ctx context.Context, userID, showcaseID string) (*pbactivity.ShowcasedActivity, error)
	ListShowcases(ctx context.Context, userID string) ([]*pbactivity.ShowcaseProfileEntry, error)
	CreateShowcase(ctx context.Context, userID string, showcase *pbactivity.ShowcasedActivity) (*pbactivity.ShowcasedActivity, error)
	UpdateShowcase(ctx context.Context, userID string, showcase *pbactivity.ShowcasedActivity) (*pbactivity.ShowcasedActivity, error)
	DeleteShowcase(ctx context.Context, userID, showcaseID string) error

	// Showcase Management RPCs
	GetShowcasePreferences(ctx context.Context, userID string) (*pbactivity.ShowcaseProfile, error)
	UpdateShowcasePreferences(ctx context.Context, userID string, prefs *pbactivity.ShowcaseProfile) (*pbactivity.ShowcaseProfile, error)
	PatchShowcaseProfile(ctx context.Context, userID string, fields map[string]interface{}) (*pbactivity.ShowcaseProfile, error)
	GetPublicShowcase(ctx context.Context, showcaseID string) (*pbactivity.ShowcasedActivity, string, error)

	// Showcase Settings & Profile
	UpdateShowcaseSlug(ctx context.Context, userID, slug string) error
	GetShowcaseProfileBySlug(ctx context.Context, slug string) (*pbactivity.ShowcaseProfile, error)
	ListShowcasedActivitiesByUser(ctx context.Context, userID string, limit int32, offset int32) ([]*pbactivity.ShowcasedActivity, int32, error)

	// Showcase Profile Entries (sub-collection: users/{userId}/settings/showcase_profile_entries/{showcaseId})
	ListShowcaseProfileEntries(ctx context.Context, userID string) ([]*pbactivity.ShowcaseProfileEntry, error)
	SetShowcaseProfileEntry(ctx context.Context, userID string, entry *pbactivity.ShowcaseProfileEntry) error
	DeleteShowcaseProfileEntry(ctx context.Context, userID, showcaseID string) error

	// Personal Records (read-only from users/{userId}/personal_records/ for public profile display)
	ListUserPersonalRecords(ctx context.Context, userID string) ([]*pbactivity.ShowcaseTopPR, error)

	// Activity Stats
	CountPipelineRunsByStatus(ctx context.Context, userID, status string) (int32, error)
	CountShowcasedActivities(ctx context.Context, userID string) (int32, error)
	CountBillingEvents(ctx context.Context, userID string) (int32, error)
	CountBillingEventsForPeriod(ctx context.Context, userID, period string) (int32, error)
	CountBillingEventsSince(ctx context.Context, userID string, since time.Time) (int32, error)
	CountDistinctActivitiesForPeriod(ctx context.Context, userID, period string) (int32, error)
	CountDistinctActivitiesSince(ctx context.Context, userID string, since time.Time) (int32, error)

	// Roundup — snapshot storage
	GetRoundup(ctx context.Context, slug, periodKey string) (*pbactivity.ShowcaseRoundup, error)
	SetRoundup(ctx context.Context, roundup *pbactivity.ShowcaseRoundup) error
	ListRecentRoundups(ctx context.Context, slug string, limit int) ([]*pbactivity.ShowcaseRoundup, error)

	// Roundup — generation inputs
	ListShowcaseEntriesInRange(ctx context.Context, userID string, from, to time.Time) ([]*pbactivity.ShowcaseProfileEntry, error)
	ListAllShowcaseUserIDs(ctx context.Context) ([]string, error)
}
