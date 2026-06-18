package activity

import (
	"context"
	"time"

	pbactivity "github.com/fitglue/server/src/go/pkg/types/pb/models/activity"
	pbpipeline "github.com/fitglue/server/src/go/pkg/types/pb/models/pipeline"
)

// MockActivityStore implements ActivityStore for testing
type MockActivityStore struct {
	GetPipelineRunFunc    func(ctx context.Context, userID, runID string) (*pbpipeline.PipelineRun, error)
	ListPipelineRunsFunc  func(ctx context.Context, userID string, limit int32, pageToken string) ([]*pbpipeline.PipelineRun, string, error)
	DeletePipelineRunFunc func(ctx context.Context, userID, runID string) error

	GetShowcaseFunc    func(ctx context.Context, userID, showcaseID string) (*pbactivity.ShowcasedActivity, error)
	ListShowcasesFunc  func(ctx context.Context, userID string) ([]*pbactivity.ShowcaseProfileEntry, error)
	CreateShowcaseFunc func(ctx context.Context, userID string, showcase *pbactivity.ShowcasedActivity) (*pbactivity.ShowcasedActivity, error)
	UpdateShowcaseFunc func(ctx context.Context, userID string, showcase *pbactivity.ShowcasedActivity) (*pbactivity.ShowcasedActivity, error)
	DeleteShowcaseFunc func(ctx context.Context, userID, showcaseID string) error

	GetShowcasePreferencesFunc    func(ctx context.Context, userID string) (*pbactivity.ShowcaseProfile, error)
	UpdateShowcasePreferencesFunc func(ctx context.Context, userID string, prefs *pbactivity.ShowcaseProfile) (*pbactivity.ShowcaseProfile, error)
	GetPublicShowcaseFunc         func(ctx context.Context, showcaseID string) (*pbactivity.ShowcasedActivity, string, error)

	UpdateShowcaseSlugFunc               func(ctx context.Context, userID, slug string) error
	GetShowcaseProfileBySlugFunc         func(ctx context.Context, slug string) (*pbactivity.ShowcaseProfile, error)
	ListShowcasedActivitiesByUserFunc    func(ctx context.Context, userID string, limit int32, offset int32) ([]*pbactivity.ShowcasedActivity, int32, error)
	CountPipelineRunsByStatusFunc        func(ctx context.Context, userID, status string) (int32, error)
	CountShowcasedActivitiesFunc         func(ctx context.Context, userID string) (int32, error)
	CountBillingEventsFunc               func(ctx context.Context, userID string) (int32, error)
	CountBillingEventsForPeriodFunc      func(ctx context.Context, userID, period string) (int32, error)
	CountBillingEventsSinceFunc          func(ctx context.Context, userID string, since time.Time) (int32, error)
	CountDistinctActivitiesForPeriodFunc func(ctx context.Context, userID, period string) (int32, error)
	CountDistinctActivitiesSinceFunc     func(ctx context.Context, userID string, since time.Time) (int32, error)

	ListShowcaseProfileEntriesFunc func(ctx context.Context, userID string) ([]*pbactivity.ShowcaseProfileEntry, error)
	SetShowcaseProfileEntryFunc    func(ctx context.Context, userID string, entry *pbactivity.ShowcaseProfileEntry) error
	DeleteShowcaseProfileEntryFunc func(ctx context.Context, userID, showcaseID string) error
	PatchShowcaseProfileFunc       func(ctx context.Context, userID string, fields map[string]interface{}) (*pbactivity.ShowcaseProfile, error)

	RecordShowcaseViewFunc   func(ctx context.Context, targetKey, visitorHash string) error
	GetShowcaseViewStatsFunc func(ctx context.Context, targetKey string) (*pbactivity.ShowcaseViewStats, error)

	CreateExportJobFunc       func(ctx context.Context, userID string, job *ExportJobRecord) error
	GetExportJobFunc          func(ctx context.Context, userID, jobID string) (*ExportJobRecord, error)
	UpdateExportJobFunc       func(ctx context.Context, userID, jobID string, fields map[string]interface{}) error
	LatestActiveExportJobFunc func(ctx context.Context, userID string) (*ExportJobRecord, error)

	GetRoundupFunc                 func(ctx context.Context, slug, periodKey string) (*pbactivity.ShowcaseRoundup, error)
	ListRecentRoundupsFunc         func(ctx context.Context, slug string, limit int) ([]*pbactivity.ShowcaseRoundup, error)
	ListShowcaseEntriesInRangeFunc func(ctx context.Context, userID string, from, to time.Time) ([]*pbactivity.ShowcaseProfileEntry, error)
	ListUserPersonalRecordsFunc    func(ctx context.Context, userID string) ([]*pbactivity.ShowcaseTopPR, error)
	ListAllShowcaseUserIDsFunc     func(ctx context.Context) ([]string, error)
}

func (m *MockActivityStore) CreateExportJob(ctx context.Context, userID string, job *ExportJobRecord) error {
	if m.CreateExportJobFunc != nil {
		return m.CreateExportJobFunc(ctx, userID, job)
	}
	return nil
}

func (m *MockActivityStore) GetExportJob(ctx context.Context, userID, jobID string) (*ExportJobRecord, error) {
	if m.GetExportJobFunc != nil {
		return m.GetExportJobFunc(ctx, userID, jobID)
	}
	return nil, nil
}

func (m *MockActivityStore) UpdateExportJob(ctx context.Context, userID, jobID string, fields map[string]interface{}) error {
	if m.UpdateExportJobFunc != nil {
		return m.UpdateExportJobFunc(ctx, userID, jobID, fields)
	}
	return nil
}

func (m *MockActivityStore) LatestActiveExportJob(ctx context.Context, userID string) (*ExportJobRecord, error) {
	if m.LatestActiveExportJobFunc != nil {
		return m.LatestActiveExportJobFunc(ctx, userID)
	}
	return nil, nil
}

func (m *MockActivityStore) GetPipelineRun(ctx context.Context, userID, runID string) (*pbpipeline.PipelineRun, error) {
	if m.GetPipelineRunFunc != nil {
		return m.GetPipelineRunFunc(ctx, userID, runID)
	}
	return nil, nil
}

func (m *MockActivityStore) ListPipelineRuns(ctx context.Context, userID string, limit int32, pageToken string) ([]*pbpipeline.PipelineRun, string, error) {
	if m.ListPipelineRunsFunc != nil {
		return m.ListPipelineRunsFunc(ctx, userID, limit, pageToken)
	}
	return nil, "", nil
}

func (m *MockActivityStore) DeletePipelineRun(ctx context.Context, userID, runID string) error {
	if m.DeletePipelineRunFunc != nil {
		return m.DeletePipelineRunFunc(ctx, userID, runID)
	}
	return nil
}

func (m *MockActivityStore) GetShowcase(ctx context.Context, userID, showcaseID string) (*pbactivity.ShowcasedActivity, error) {
	if m.GetShowcaseFunc != nil {
		return m.GetShowcaseFunc(ctx, userID, showcaseID)
	}
	return nil, nil
}

func (m *MockActivityStore) ListShowcases(ctx context.Context, userID string) ([]*pbactivity.ShowcaseProfileEntry, error) {
	if m.ListShowcasesFunc != nil {
		return m.ListShowcasesFunc(ctx, userID)
	}
	return nil, nil
}

func (m *MockActivityStore) CreateShowcase(ctx context.Context, userID string, showcase *pbactivity.ShowcasedActivity) (*pbactivity.ShowcasedActivity, error) {
	if m.CreateShowcaseFunc != nil {
		return m.CreateShowcaseFunc(ctx, userID, showcase)
	}
	return showcase, nil
}

func (m *MockActivityStore) UpdateShowcase(ctx context.Context, userID string, showcase *pbactivity.ShowcasedActivity) (*pbactivity.ShowcasedActivity, error) {
	if m.UpdateShowcaseFunc != nil {
		return m.UpdateShowcaseFunc(ctx, userID, showcase)
	}
	return showcase, nil
}

func (m *MockActivityStore) DeleteShowcase(ctx context.Context, userID, showcaseID string) error {
	if m.DeleteShowcaseFunc != nil {
		return m.DeleteShowcaseFunc(ctx, userID, showcaseID)
	}
	return nil
}

func (m *MockActivityStore) GetShowcasePreferences(ctx context.Context, userID string) (*pbactivity.ShowcaseProfile, error) {
	if m.GetShowcasePreferencesFunc != nil {
		return m.GetShowcasePreferencesFunc(ctx, userID)
	}
	return nil, nil
}

func (m *MockActivityStore) UpdateShowcasePreferences(ctx context.Context, userID string, prefs *pbactivity.ShowcaseProfile) (*pbactivity.ShowcaseProfile, error) {
	if m.UpdateShowcasePreferencesFunc != nil {
		return m.UpdateShowcasePreferencesFunc(ctx, userID, prefs)
	}
	return prefs, nil
}

func (m *MockActivityStore) PatchShowcaseProfile(ctx context.Context, userID string, fields map[string]interface{}) (*pbactivity.ShowcaseProfile, error) {
	if m.PatchShowcaseProfileFunc != nil {
		return m.PatchShowcaseProfileFunc(ctx, userID, fields)
	}
	return nil, nil
}

func (m *MockActivityStore) GetPublicShowcase(ctx context.Context, showcaseID string) (*pbactivity.ShowcasedActivity, string, error) {
	if m.GetPublicShowcaseFunc != nil {
		return m.GetPublicShowcaseFunc(ctx, showcaseID)
	}
	return nil, "", nil
}

func (m *MockActivityStore) UpdateShowcaseSlug(ctx context.Context, userID, slug string) error {
	if m.UpdateShowcaseSlugFunc != nil {
		return m.UpdateShowcaseSlugFunc(ctx, userID, slug)
	}
	return nil
}

func (m *MockActivityStore) GetShowcaseProfileBySlug(ctx context.Context, slug string) (*pbactivity.ShowcaseProfile, error) {
	if m.GetShowcaseProfileBySlugFunc != nil {
		return m.GetShowcaseProfileBySlugFunc(ctx, slug)
	}
	return nil, nil
}

func (m *MockActivityStore) RecordShowcaseView(ctx context.Context, targetKey, visitorHash string) error {
	if m.RecordShowcaseViewFunc != nil {
		return m.RecordShowcaseViewFunc(ctx, targetKey, visitorHash)
	}
	return nil
}

func (m *MockActivityStore) GetShowcaseViewStats(ctx context.Context, targetKey string) (*pbactivity.ShowcaseViewStats, error) {
	if m.GetShowcaseViewStatsFunc != nil {
		return m.GetShowcaseViewStatsFunc(ctx, targetKey)
	}
	return &pbactivity.ShowcaseViewStats{TargetKey: targetKey}, nil
}

func (m *MockActivityStore) ListShowcasedActivitiesByUser(ctx context.Context, userID string, limit int32, offset int32) ([]*pbactivity.ShowcasedActivity, int32, error) {
	if m.ListShowcasedActivitiesByUserFunc != nil {
		return m.ListShowcasedActivitiesByUserFunc(ctx, userID, limit, offset)
	}
	return nil, 0, nil
}

func (m *MockActivityStore) CountPipelineRunsByStatus(ctx context.Context, userID, pipelineStatus string) (int32, error) {
	if m.CountPipelineRunsByStatusFunc != nil {
		return m.CountPipelineRunsByStatusFunc(ctx, userID, pipelineStatus)
	}
	return 0, nil
}

func (m *MockActivityStore) CountShowcasedActivities(ctx context.Context, userID string) (int32, error) {
	if m.CountShowcasedActivitiesFunc != nil {
		return m.CountShowcasedActivitiesFunc(ctx, userID)
	}
	return 0, nil
}

func (m *MockActivityStore) CountBillingEvents(ctx context.Context, userID string) (int32, error) {
	if m.CountBillingEventsFunc != nil {
		return m.CountBillingEventsFunc(ctx, userID)
	}
	return 0, nil
}

func (m *MockActivityStore) CountBillingEventsForPeriod(ctx context.Context, userID, period string) (int32, error) {
	if m.CountBillingEventsForPeriodFunc != nil {
		return m.CountBillingEventsForPeriodFunc(ctx, userID, period)
	}
	return 0, nil
}

func (m *MockActivityStore) CountBillingEventsSince(ctx context.Context, userID string, since time.Time) (int32, error) {
	if m.CountBillingEventsSinceFunc != nil {
		return m.CountBillingEventsSinceFunc(ctx, userID, since)
	}
	return 0, nil
}

func (m *MockActivityStore) CountDistinctActivitiesForPeriod(ctx context.Context, userID, period string) (int32, error) {
	if m.CountDistinctActivitiesForPeriodFunc != nil {
		return m.CountDistinctActivitiesForPeriodFunc(ctx, userID, period)
	}
	return 0, nil
}

func (m *MockActivityStore) CountDistinctActivitiesSince(ctx context.Context, userID string, since time.Time) (int32, error) {
	if m.CountDistinctActivitiesSinceFunc != nil {
		return m.CountDistinctActivitiesSinceFunc(ctx, userID, since)
	}
	return 0, nil
}

func (m *MockActivityStore) ListShowcaseProfileEntries(ctx context.Context, userID string) ([]*pbactivity.ShowcaseProfileEntry, error) {
	if m.ListShowcaseProfileEntriesFunc != nil {
		return m.ListShowcaseProfileEntriesFunc(ctx, userID)
	}
	return nil, nil
}

func (m *MockActivityStore) SetShowcaseProfileEntry(ctx context.Context, userID string, entry *pbactivity.ShowcaseProfileEntry) error {
	if m.SetShowcaseProfileEntryFunc != nil {
		return m.SetShowcaseProfileEntryFunc(ctx, userID, entry)
	}
	return nil
}

func (m *MockActivityStore) DeleteShowcaseProfileEntry(ctx context.Context, userID, showcaseID string) error {
	if m.DeleteShowcaseProfileEntryFunc != nil {
		return m.DeleteShowcaseProfileEntryFunc(ctx, userID, showcaseID)
	}
	return nil
}

func (m *MockActivityStore) ListUserPersonalRecords(ctx context.Context, userID string) ([]*pbactivity.ShowcaseTopPR, error) {
	if m.ListUserPersonalRecordsFunc != nil {
		return m.ListUserPersonalRecordsFunc(ctx, userID)
	}
	return nil, nil
}

func (m *MockActivityStore) GetRoundup(ctx context.Context, slug, periodKey string) (*pbactivity.ShowcaseRoundup, error) {
	if m.GetRoundupFunc != nil {
		return m.GetRoundupFunc(ctx, slug, periodKey)
	}
	return nil, nil
}

func (m *MockActivityStore) SetRoundup(ctx context.Context, roundup *pbactivity.ShowcaseRoundup) error {
	return nil
}

func (m *MockActivityStore) ListRecentRoundups(ctx context.Context, slug string, limit int) ([]*pbactivity.ShowcaseRoundup, error) {
	if m.ListRecentRoundupsFunc != nil {
		return m.ListRecentRoundupsFunc(ctx, slug, limit)
	}
	return nil, nil
}

func (m *MockActivityStore) ListShowcaseEntriesInRange(ctx context.Context, userID string, from, to time.Time) ([]*pbactivity.ShowcaseProfileEntry, error) {
	if m.ListShowcaseEntriesInRangeFunc != nil {
		return m.ListShowcaseEntriesInRangeFunc(ctx, userID, from, to)
	}
	return nil, nil
}

func (m *MockActivityStore) ListAllShowcaseUserIDs(ctx context.Context) ([]string, error) {
	if m.ListAllShowcaseUserIDsFunc != nil {
		return m.ListAllShowcaseUserIDsFunc(ctx)
	}
	return nil, nil
}

func (m *MockActivityStore) GetUserNotificationData(ctx context.Context, userID string) (fcmTokens []string, notifyRoundup bool, err error) {
	return nil, false, nil
}

// MockBlobStore implements BlobStore for testing
type MockBlobStore struct {
	GetFunc       func(ctx context.Context, bucket, object string) ([]byte, error)
	DeleteFunc    func(ctx context.Context, bucket, object string) error
	WriteFunc     func(ctx context.Context, bucket, object string, data []byte) error
	SignedURLFunc func(ctx context.Context, bucket, path, contentType string, contentLength int64, expiry time.Duration) (string, error)
}

func (m *MockBlobStore) Get(ctx context.Context, bucket, object string) ([]byte, error) {
	if m.GetFunc != nil {
		return m.GetFunc(ctx, bucket, object)
	}
	return nil, nil
}

func (m *MockBlobStore) Delete(ctx context.Context, bucket, object string) error {
	if m.DeleteFunc != nil {
		return m.DeleteFunc(ctx, bucket, object)
	}
	return nil
}

func (m *MockBlobStore) Write(ctx context.Context, bucket, object string, data []byte) error {
	if m.WriteFunc != nil {
		return m.WriteFunc(ctx, bucket, object, data)
	}
	return nil
}

func (m *MockBlobStore) SignedURL(ctx context.Context, bucket, path, contentType string, contentLength int64, expiry time.Duration) (string, error) {
	if m.SignedURLFunc != nil {
		return m.SignedURLFunc(ctx, bucket, path, contentType, contentLength, expiry)
	}
	return "https://storage.googleapis.com/test-signed-url", nil
}

// MockPublisher implements Publisher for testing
type MockPublisher struct {
	PublishCloudEventFunc func(ctx context.Context, topic string, e interface{}) (string, error)
}

func (m *MockPublisher) PublishCloudEvent(ctx context.Context, topic string, e interface{}) (string, error) {
	if m.PublishCloudEventFunc != nil {
		return m.PublishCloudEventFunc(ctx, topic, e)
	}
	return "test-msg-id", nil
}
