package shared

import (
	"context"
	"time"

	"github.com/fitglue/server/src/go/pkg/domain/user"

	"github.com/cloudevents/sdk-go/v2/event"
	pbplugin "github.com/fitglue/server/src/go/pkg/types/pb/models/plugin"
	pbuser "github.com/fitglue/server/src/go/pkg/types/pb/models/user"

	pbactivity "github.com/fitglue/server/src/go/pkg/types/pb/models/activity"

	pbpipeline "github.com/fitglue/server/src/go/pkg/types/pb/models/pipeline"
)

// BillingEvent records one successful destination upload for audit and stats purposes.
type BillingEvent struct {
	ActivityID    string
	PipelineRunID string
	PipelineID    string
	Source        string
	Destination   string
}

// --- Persistence Interfaces ---

type Database interface {
	SetExecution(ctx context.Context, record *pbpipeline.ExecutionRecord) error
	UpdateExecution(ctx context.Context, userId string, id string, data map[string]interface{}) error
	GetUser(ctx context.Context, id string) (*user.Record, error)
	UpdateUser(ctx context.Context, id string, data map[string]interface{}) error

	// Billing Events (durable audit log of successful destination uploads)
	RecordBillingEvent(ctx context.Context, userID string, event BillingEvent) error
	CountBillingEvents(ctx context.Context, userID string) (int32, error)
	CountBillingEventsForPeriod(ctx context.Context, userID, period string) (int32, error)

	// Sync Count (for tier limits)
	IncrementSyncCount(ctx context.Context, userID string) error
	IncrementPreventedSyncCount(ctx context.Context, userID string) error
	ResetSyncCount(ctx context.Context, userID string) error

	// Pending Inputs
	GetPendingInput(ctx context.Context, userId string, id string) (*pbpipeline.PendingInput, error)
	CreatePendingInput(ctx context.Context, userId string, input *pbpipeline.PendingInput) error
	UpdatePendingInput(ctx context.Context, userId string, id string, data map[string]interface{}) error
	DeletePendingInput(ctx context.Context, userId string, id string) error
	ListPendingInputs(ctx context.Context, userID string) ([]*pbpipeline.PendingInput, error)
	ListPendingInputsByEnricher(ctx context.Context, enricherId string, status pbpipeline.PendingInput_Status) ([]*pbpipeline.PendingInput, error)

	// Counters
	GetCounter(ctx context.Context, userId string, id string) (*pbuser.Counter, error)
	SetCounter(ctx context.Context, userId string, counter *pbuser.Counter) error
	ListCounters(ctx context.Context, userId string) ([]*pbuser.Counter, error)
	DeleteCounter(ctx context.Context, userId string, id string) error

	// Personal Records
	GetPersonalRecord(ctx context.Context, userId string, recordType string) (*pbuser.PersonalRecord, error)
	SetPersonalRecord(ctx context.Context, userId string, record *pbuser.PersonalRecord) error
	ListPersonalRecords(ctx context.Context, userId string) ([]*pbuser.PersonalRecord, error)
	DeletePersonalRecord(ctx context.Context, userId string, recordType string) error

	// Pipelines (Sub-collection)
	GetUserPipelines(ctx context.Context, userId string) ([]*pbpipeline.PipelineConfig, error)

	// Plugin Defaults (user-level default config for sources/destinations)
	GetPluginDefault(ctx context.Context, userId string, pluginId string) (*pbpipeline.PluginDefault, error)
	SetPluginDefault(ctx context.Context, userId string, pluginDefault *pbpipeline.PluginDefault) error

	// Showcased Activities (public shareable snapshots)
	ShowcaseActivityExists(ctx context.Context, showcaseId string) (bool, error)
	SetShowcasedActivity(ctx context.Context, userID string, activity *pbactivity.ShowcasedActivity) error
	GetShowcasedActivity(ctx context.Context, showcaseId string) (*pbactivity.ShowcasedActivity, error)
	GetShowcasedActivityByPipelineExecutionId(ctx context.Context, pipelineExecutionId string) (*pbactivity.ShowcasedActivity, error)

	// Showcase Profiles (materialized user profile for homepage)
	SetShowcaseProfile(ctx context.Context, profile *pbactivity.ShowcaseProfile) error
	GetShowcaseProfile(ctx context.Context, slug string) (*pbactivity.ShowcaseProfile, error)
	GetShowcaseProfileByUserId(ctx context.Context, userId string) (*pbactivity.ShowcaseProfile, error)
	DeleteShowcaseProfile(ctx context.Context, slug string) error

	// Showcase Profile Entries (user sub-collection)
	SetShowcaseProfileEntry(ctx context.Context, userID string, entry *pbactivity.ShowcaseProfileEntry) error

	// Uploaded Activities (for loop prevention - tracks what we've posted to destinations)
	SetUploadedActivity(ctx context.Context, userId string, record *pbactivity.UploadedActivityRecord) error
	GetUploadedActivity(ctx context.Context, userId string, destination pbplugin.DestinationType, destinationId string) (*pbactivity.UploadedActivityRecord, error)

	// Destination Create Claims (cross-execution dedup). TryClaimDestinationCreate atomically
	// records an intent to CREATE an activity on a destination. Only the first caller within the
	// TTL window gets acquired=true; concurrent or later callers get false until the claim is
	// released or lapses. This stops two pipeline executions — e.g. two pending-input resumes
	// resolved at the same instant — from both creating a duplicate (two Strava activities).
	TryClaimDestinationCreate(ctx context.Context, userId string, claimKey string, ttl time.Duration) (bool, error)
	ReleaseDestinationCreate(ctx context.Context, userId string, claimKey string) error

	// Pipeline Runs (lifecycle tracking)
	CreatePipelineRun(ctx context.Context, userId string, run *pbpipeline.PipelineRun) error
	GetPipelineRun(ctx context.Context, userId string, id string) (*pbpipeline.PipelineRun, error)
	GetPipelineRunByActivityId(ctx context.Context, userId string, activityId string) (*pbpipeline.PipelineRun, error)
	UpdatePipelineRun(ctx context.Context, userId string, id string, data map[string]interface{}) error

	// Destination Outcomes (subcollection of Pipeline Runs - avoids race conditions)
	SetDestinationOutcome(ctx context.Context, userId string, pipelineRunId string, outcome *pbpipeline.DestinationOutcome) error
	GetDestinationOutcomes(ctx context.Context, userId string, pipelineRunId string) ([]*pbpipeline.DestinationOutcome, error)

	// Booster Data (generic key-value storage for enrichers that need persistence)
	GetBoosterData(ctx context.Context, userId string, boosterId string) (map[string]interface{}, error)
	SetBoosterData(ctx context.Context, userId string, boosterId string, data map[string]interface{}) error
	DeleteBoosterData(ctx context.Context, userId string, boosterId string) error
}

// --- Messaging Interfaces ---

type Publisher interface {
	PublishCloudEvent(ctx context.Context, topic string, e event.Event) (string, error)
	PublishJSON(ctx context.Context, topic string, data []byte) error
}

// --- Storage Interfaces ---

type BlobStore interface {
	Write(ctx context.Context, bucket, object string, data []byte) error
	Get(ctx context.Context, bucket, object string) ([]byte, error)
	Delete(ctx context.Context, bucket, object string) error
}

// --- Notification Interfaces ---

type NotificationService interface {
	SendPushNotification(ctx context.Context, userID string, title, body string, tokens []string, data map[string]string) error
}
