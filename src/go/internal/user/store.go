package user

import (
	"context"
	"time"

	pbuser "github.com/fitglue/server/src/go/pkg/types/pb/models/user"
	"google.golang.org/protobuf/types/known/structpb"
)

// Store defines the data access interface for User entities.
type Store interface {
	CreateUser(ctx context.Context, userID string) (*pbuser.UserProfile, error)
	GetProfile(ctx context.Context, userID string) (*pbuser.UserProfile, error)
	UpdateProfile(ctx context.Context, userID string, profile *pbuser.UserProfile) error
	DeleteUser(ctx context.Context, userID string) error
	ListShowcaseExecutionIDs(ctx context.Context, userID string) ([]string, error)
	FindUsersByDateRange(ctx context.Context, start, end time.Time) ([]*pbuser.UserProfile, error)

	GetIntegrations(ctx context.Context, userID string) (*pbuser.UserIntegrations, error)
	SetIntegration(ctx context.Context, userID, provider string, data interface{}) error
	DeleteIntegration(ctx context.Context, userID, provider string) error
	FindUserByIntegration(ctx context.Context, provider string, providerUID string) (*pbuser.UserProfile, error)

	ListCounters(ctx context.Context, userID string) ([]*pbuser.Counter, error)
	UpdateCounter(ctx context.Context, userID, counterID string, count int64) (*pbuser.Counter, error)

	GetNotificationPrefs(ctx context.Context, userID string) (*pbuser.NotificationPreferences, error)
	UpdateNotificationPrefs(ctx context.Context, userID string, prefs *pbuser.NotificationPreferences) error

	AddFCMToken(ctx context.Context, userID, token string) error

	GetBoosterData(ctx context.Context, userID, boosterID string) (map[string]*structpb.Struct, error)
	SetBoosterData(ctx context.Context, userID, boosterID string, data *structpb.Struct) error
	DeleteBoosterData(ctx context.Context, userID, boosterID string) error

	DeleteCounter(ctx context.Context, userID, counterID string) error

	ListPersonalRecords(ctx context.Context, userID string) ([]*pbuser.PersonalRecord, error)
	SetPersonalRecord(ctx context.Context, userID, recordType string, record *pbuser.PersonalRecord) error
	DeletePersonalRecord(ctx context.Context, userID, recordType string) error

	ListPluginDefaults(ctx context.Context, userID string) (map[string]*structpb.Struct, error)
	SetPluginDefaults(ctx context.Context, userID, pluginID string, defaults *structpb.Struct) error
	DeletePluginDefaults(ctx context.Context, userID, pluginID string) error
}
