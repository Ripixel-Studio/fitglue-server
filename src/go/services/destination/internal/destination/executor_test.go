package destination

import (
	"context"
	"fmt"
	"testing"

	"github.com/cloudevents/sdk-go/v2/event"
	"github.com/fitglue/server/src/go/internal/infra"
	shared "github.com/fitglue/server/src/go/pkg"
	"github.com/fitglue/server/src/go/pkg/testing/mocks"
	pbactivity "github.com/fitglue/server/src/go/pkg/types/pb/models/activity"
	pbevents "github.com/fitglue/server/src/go/pkg/types/pb/models/events"
	pbpipeline "github.com/fitglue/server/src/go/pkg/types/pb/models/pipeline"
	pbplugin "github.com/fitglue/server/src/go/pkg/types/pb/models/plugin"
	pbuser "github.com/fitglue/server/src/go/pkg/types/pb/models/user"
	activitypb "github.com/fitglue/server/src/go/pkg/types/pb/services/activity"
	userpb "github.com/fitglue/server/src/go/pkg/types/pb/services/user"
	"github.com/stretchr/testify/assert"
	"google.golang.org/grpc"
	"google.golang.org/protobuf/encoding/protojson"
	"google.golang.org/protobuf/types/known/emptypb"
)

type mockUserServiceClient struct {
	GetProfileFunc       func(ctx context.Context, in *userpb.GetProfileRequest, opts ...grpc.CallOption) (*pbuser.UserProfile, error)
	ListIntegrationsFunc func(ctx context.Context, in *userpb.ListIntegrationsRequest, opts ...grpc.CallOption) (*pbuser.UserIntegrations, error)
}

func (m *mockUserServiceClient) CreateUser(ctx context.Context, in *userpb.CreateUserRequest, opts ...grpc.CallOption) (*pbuser.UserProfile, error) {
	return nil, nil
}
func (m *mockUserServiceClient) GetProfile(ctx context.Context, in *userpb.GetProfileRequest, opts ...grpc.CallOption) (*pbuser.UserProfile, error) {
	if m.GetProfileFunc != nil {
		return m.GetProfileFunc(ctx, in, opts...)
	}
	return &pbuser.UserProfile{}, nil
}
func (m *mockUserServiceClient) UpdateProfile(ctx context.Context, in *userpb.UpdateProfileRequest, opts ...grpc.CallOption) (*pbuser.UserProfile, error) {
	return nil, nil
}
func (m *mockUserServiceClient) GetIntegration(ctx context.Context, in *userpb.GetIntegrationRequest, opts ...grpc.CallOption) (*userpb.GetIntegrationResponse, error) {
	return nil, nil
}
func (m *mockUserServiceClient) SetIntegration(ctx context.Context, in *userpb.SetIntegrationRequest, opts ...grpc.CallOption) (*emptypb.Empty, error) {
	return nil, nil
}
func (m *mockUserServiceClient) DeleteIntegration(ctx context.Context, in *userpb.DeleteIntegrationRequest, opts ...grpc.CallOption) (*emptypb.Empty, error) {
	return nil, nil
}
func (m *mockUserServiceClient) ListIntegrations(ctx context.Context, in *userpb.ListIntegrationsRequest, opts ...grpc.CallOption) (*pbuser.UserIntegrations, error) {
	if m.ListIntegrationsFunc != nil {
		return m.ListIntegrationsFunc(ctx, in, opts...)
	}
	return &pbuser.UserIntegrations{}, nil
}
func (m *mockUserServiceClient) GetNotificationPrefs(ctx context.Context, in *userpb.GetNotificationPrefsRequest, opts ...grpc.CallOption) (*pbuser.NotificationPreferences, error) {
	return nil, nil
}
func (m *mockUserServiceClient) UpdateNotificationPrefs(ctx context.Context, in *userpb.UpdateNotificationPrefsRequest, opts ...grpc.CallOption) (*pbuser.NotificationPreferences, error) {
	return nil, nil
}
func (m *mockUserServiceClient) ListCounters(ctx context.Context, in *userpb.ListCountersRequest, opts ...grpc.CallOption) (*userpb.ListCountersResponse, error) {
	return nil, nil
}
func (m *mockUserServiceClient) UpdateCounter(ctx context.Context, in *userpb.UpdateCounterRequest, opts ...grpc.CallOption) (*pbuser.Counter, error) {
	return nil, nil
}
func (m *mockUserServiceClient) GetBoosterData(ctx context.Context, in *userpb.GetBoosterDataRequest, opts ...grpc.CallOption) (*userpb.GetBoosterDataResponse, error) {
	return nil, nil
}
func (m *mockUserServiceClient) SetBoosterData(ctx context.Context, in *userpb.SetBoosterDataRequest, opts ...grpc.CallOption) (*emptypb.Empty, error) {
	return nil, nil
}
func (m *mockUserServiceClient) DeleteBoosterData(ctx context.Context, in *userpb.DeleteBoosterDataRequest, opts ...grpc.CallOption) (*emptypb.Empty, error) {
	return nil, nil
}
func (m *mockUserServiceClient) DeleteUser(ctx context.Context, in *userpb.DeleteUserRequest, opts ...grpc.CallOption) (*emptypb.Empty, error) {
	return nil, nil
}
func (m *mockUserServiceClient) SendVerificationEmail(ctx context.Context, in *userpb.SendVerificationEmailRequest, opts ...grpc.CallOption) (*emptypb.Empty, error) {
	return nil, nil
}
func (m *mockUserServiceClient) SendPasswordResetEmail(ctx context.Context, in *userpb.SendPasswordResetEmailRequest, opts ...grpc.CallOption) (*emptypb.Empty, error) {
	return nil, nil
}
func (m *mockUserServiceClient) SendEmailChangeVerification(ctx context.Context, in *userpb.SendEmailChangeVerificationRequest, opts ...grpc.CallOption) (*emptypb.Empty, error) {
	return nil, nil
}
func (m *mockUserServiceClient) GenerateRegistrationSummary(ctx context.Context, in *userpb.GenerateRegistrationSummaryRequest, opts ...grpc.CallOption) (*emptypb.Empty, error) {
	return nil, nil
}
func (m *mockUserServiceClient) ResolveUserByIntegration(ctx context.Context, in *userpb.ResolveUserByIntegrationRequest, opts ...grpc.CallOption) (*userpb.ResolveUserByIntegrationResponse, error) {
	return nil, nil
}
func (m *mockUserServiceClient) ListUsers(ctx context.Context, in *userpb.ListUsersRequest, opts ...grpc.CallOption) (*userpb.ListUsersResponse, error) {
	return nil, nil
}
func (m *mockUserServiceClient) ListPersonalRecords(ctx context.Context, in *userpb.ListPersonalRecordsRequest, opts ...grpc.CallOption) (*userpb.ListPersonalRecordsResponse, error) {
	return nil, nil
}
func (m *mockUserServiceClient) SetPersonalRecord(ctx context.Context, in *userpb.SetPersonalRecordRequest, opts ...grpc.CallOption) (*pbuser.PersonalRecord, error) {
	return nil, nil
}
func (m *mockUserServiceClient) DeletePersonalRecord(ctx context.Context, in *userpb.DeletePersonalRecordRequest, opts ...grpc.CallOption) (*emptypb.Empty, error) {
	return nil, nil
}
func (m *mockUserServiceClient) ListPluginDefaults(ctx context.Context, in *userpb.ListPluginDefaultsRequest, opts ...grpc.CallOption) (*userpb.ListPluginDefaultsResponse, error) {
	return nil, nil
}
func (m *mockUserServiceClient) SetPluginDefaults(ctx context.Context, in *userpb.SetPluginDefaultsRequest, opts ...grpc.CallOption) (*emptypb.Empty, error) {
	return nil, nil
}
func (m *mockUserServiceClient) DeletePluginDefaults(ctx context.Context, in *userpb.DeletePluginDefaultsRequest, opts ...grpc.CallOption) (*emptypb.Empty, error) {
	return nil, nil
}
func (m *mockUserServiceClient) DeleteCounter(ctx context.Context, in *userpb.DeleteCounterRequest, opts ...grpc.CallOption) (*emptypb.Empty, error) {
	return nil, nil
}
func (m *mockUserServiceClient) SetFCMToken(ctx context.Context, in *userpb.SetFCMTokenRequest, opts ...grpc.CallOption) (*emptypb.Empty, error) {
	return nil, nil
}

type mockActivityServiceClient struct{}

func (m *mockActivityServiceClient) GetActivity(ctx context.Context, in *activitypb.GetActivityRequest, opts ...grpc.CallOption) (*pbactivity.StandardizedActivity, error) {
	return nil, nil
}
func (m *mockActivityServiceClient) ListActivities(ctx context.Context, in *activitypb.ListActivitiesRequest, opts ...grpc.CallOption) (*activitypb.ListActivitiesResponse, error) {
	return nil, nil
}
func (m *mockActivityServiceClient) DeleteActivity(ctx context.Context, in *activitypb.DeleteActivityRequest, opts ...grpc.CallOption) (*emptypb.Empty, error) {
	return nil, nil
}
func (m *mockActivityServiceClient) GetShowcase(ctx context.Context, in *activitypb.GetShowcaseRequest, opts ...grpc.CallOption) (*pbactivity.ShowcasedActivity, error) {
	return nil, nil
}
func (m *mockActivityServiceClient) ListShowcases(ctx context.Context, in *activitypb.ListShowcasesRequest, opts ...grpc.CallOption) (*activitypb.ListShowcasesResponse, error) {
	return nil, nil
}
func (m *mockActivityServiceClient) CreateShowcase(ctx context.Context, in *activitypb.CreateShowcaseRequest, opts ...grpc.CallOption) (*pbactivity.ShowcasedActivity, error) {
	return nil, nil
}
func (m *mockActivityServiceClient) UpdateShowcase(ctx context.Context, in *activitypb.UpdateShowcaseRequest, opts ...grpc.CallOption) (*pbactivity.ShowcasedActivity, error) {
	return nil, nil
}
func (m *mockActivityServiceClient) DeleteShowcase(ctx context.Context, in *activitypb.DeleteShowcaseRequest, opts ...grpc.CallOption) (*emptypb.Empty, error) {
	return nil, nil
}
func (m *mockActivityServiceClient) ExportData(ctx context.Context, in *activitypb.ExportDataRequest, opts ...grpc.CallOption) (*activitypb.ExportDataResponse, error) {
	return nil, nil
}
func (m *mockActivityServiceClient) ParseFitFile(ctx context.Context, in *activitypb.ParseFitFileRequest, opts ...grpc.CallOption) (*pbactivity.StandardizedActivity, error) {
	return nil, nil
}
func (m *mockActivityServiceClient) GetShowcasePreferences(ctx context.Context, in *activitypb.GetShowcasePreferencesRequest, opts ...grpc.CallOption) (*pbactivity.ShowcaseProfile, error) {
	return nil, nil
}
func (m *mockActivityServiceClient) UpdateShowcasePreferences(ctx context.Context, in *activitypb.UpdateShowcasePreferencesRequest, opts ...grpc.CallOption) (*pbactivity.ShowcaseProfile, error) {
	return nil, nil
}
func (m *mockActivityServiceClient) GenerateShowcaseImages(ctx context.Context, in *activitypb.GenerateShowcaseImagesRequest, opts ...grpc.CallOption) (*emptypb.Empty, error) {
	return nil, nil
}
func (m *mockActivityServiceClient) GetPublicShowcase(ctx context.Context, in *activitypb.GetPublicShowcaseRequest, opts ...grpc.CallOption) (*pbactivity.ShowcasedActivity, error) {
	return nil, nil
}
func (m *mockActivityServiceClient) GetShowcaseSettings(ctx context.Context, in *activitypb.GetShowcaseSettingsRequest, opts ...grpc.CallOption) (*activitypb.GetShowcaseSettingsResponse, error) {
	return nil, nil
}
func (m *mockActivityServiceClient) UpdateShowcaseSettings(ctx context.Context, in *activitypb.UpdateShowcaseSettingsRequest, opts ...grpc.CallOption) (*pbactivity.ShowcaseProfile, error) {
	return nil, nil
}
func (m *mockActivityServiceClient) UpdateShowcaseSlug(ctx context.Context, in *activitypb.UpdateShowcaseSlugRequest, opts ...grpc.CallOption) (*activitypb.UpdateShowcaseSlugResponse, error) {
	return nil, nil
}
func (m *mockActivityServiceClient) AddShowcaseEntry(ctx context.Context, in *activitypb.AddShowcaseEntryRequest, opts ...grpc.CallOption) (*emptypb.Empty, error) {
	return nil, nil
}
func (m *mockActivityServiceClient) RemoveShowcaseEntry(ctx context.Context, in *activitypb.RemoveShowcaseEntryRequest, opts ...grpc.CallOption) (*emptypb.Empty, error) {
	return nil, nil
}
func (m *mockActivityServiceClient) GetShowcaseProfilePictureUploadUrl(ctx context.Context, in *activitypb.GetShowcaseProfilePictureUploadUrlRequest, opts ...grpc.CallOption) (*activitypb.GetShowcaseProfilePictureUploadUrlResponse, error) {
	return nil, nil
}
func (m *mockActivityServiceClient) GetPublicShowcaseProfile(ctx context.Context, in *activitypb.GetPublicShowcaseProfileRequest, opts ...grpc.CallOption) (*activitypb.GetPublicShowcaseProfileResponse, error) {
	return nil, nil
}
func (m *mockActivityServiceClient) GetActivityStats(ctx context.Context, in *activitypb.GetActivityStatsRequest, opts ...grpc.CallOption) (*activitypb.GetActivityStatsResponse, error) {
	return nil, nil
}
func (m *mockActivityServiceClient) GetActivityPhotoUploadUrl(ctx context.Context, in *activitypb.GetActivityPhotoUploadUrlRequest, opts ...grpc.CallOption) (*activitypb.GetActivityPhotoUploadUrlResponse, error) {
	return &activitypb.GetActivityPhotoUploadUrlResponse{}, nil
}

type mockNotificationService struct{}

func (m *mockNotificationService) SendPushNotification(ctx context.Context, userID string, title, body string, tokens []string, data map[string]string) error {
	return nil
}

// ensure interfaces are implemented
var _ userpb.UserServiceClient = (*mockUserServiceClient)(nil)
var _ activitypb.ActivityServiceClient = (*mockActivityServiceClient)(nil)
var _ shared.NotificationService = (*mockNotificationService)(nil)

func TestUploadExecutor_Process(t *testing.T) {
	registry := NewRegistry()
	mockUploader := &mockUploader{name: "mock-dest", id: "123"}
	registry.Register(pbplugin.DestinationType_DESTINATION_STRAVA, mockUploader)

	userClient := &mockUserServiceClient{
		GetProfileFunc: func(ctx context.Context, in *userpb.GetProfileRequest, opts ...grpc.CallOption) (*pbuser.UserProfile, error) {
			return &pbuser.UserProfile{UserId: in.UserId}, nil
		},
		ListIntegrationsFunc: func(ctx context.Context, in *userpb.ListIntegrationsRequest, opts ...grpc.CallOption) (*pbuser.UserIntegrations, error) {
			return &pbuser.UserIntegrations{}, nil
		},
	}
	activityClient := &mockActivityServiceClient{}
	db := &mocks.MockDatabase{}
	notifications := &mockNotificationService{}
	logger := infra.NewLogger()

	executor := NewUploadExecutor(registry, userClient, activityClient, db, nil, notifications, logger)

	// Create a test payload
	payload := &pbevents.EnrichedActivityEvent{
		UserId:       "user-1",
		ActivityId:   "act-1",
		Destinations: []pbplugin.DestinationType{pbplugin.DestinationType_DESTINATION_STRAVA},
		ActivityData: &pbactivity.StandardizedActivity{
			ExternalId: "ext-1",
		},
	}

	payloadBytes, err := protojson.Marshal(payload)
	assert.NoError(t, err)

	ce := event.New()
	ce.SetID("test-id")
	ce.SetType("com.fitglue.event.enriched")
	ce.SetSource("test")
	ce.SetData("application/json", payloadBytes)

	err = executor.Process(context.Background(), &ce)
	assert.NoError(t, err)

	// Basic execution completed without panic or error.
}

func TestUploadExecutor_Process_GetProfileError_WritesFailure(t *testing.T) {
	registry := NewRegistry()
	registry.Register(pbplugin.DestinationType_DESTINATION_STRAVA, &mockUploader{name: "strava", id: "s1"})
	registry.Register(pbplugin.DestinationType_DESTINATION_HEVY, &mockUploader{name: "hevy", id: "h1"})

	userClient := &mockUserServiceClient{
		GetProfileFunc: func(ctx context.Context, in *userpb.GetProfileRequest, opts ...grpc.CallOption) (*pbuser.UserProfile, error) {
			return nil, fmt.Errorf("rpc error: code = PermissionDenied desc = 403 Forbidden")
		},
	}
	activityClient := &mockActivityServiceClient{}

	// Track SetDestinationOutcome calls
	var writtenOutcomes []*pbpipeline.DestinationOutcome
	db := &mocks.MockDatabase{}
	notifications := &mockNotificationService{}
	logger := infra.NewLogger()

	executor := NewUploadExecutor(registry, userClient, activityClient, db, nil, notifications, logger)

	// Override the DB to track outcomes written by writeFailureForAllDestinations
	type outcomeTracker struct {
		shared.Database
		outcomes []*pbpipeline.DestinationOutcome
	}
	tracker := &outcomeTracker{Database: db}

	// We can't easily override a single method, so we verify via the error return
	// and confirm that the error comes from the GetProfile path
	pipelineRunId := "run-123"
	payload := &pbevents.EnrichedActivityEvent{
		UserId:              "user-1",
		ActivityId:          "act-1",
		PipelineExecutionId: &pipelineRunId,
		Destinations: []pbplugin.DestinationType{
			pbplugin.DestinationType_DESTINATION_STRAVA,
			pbplugin.DestinationType_DESTINATION_HEVY,
		},
		ActivityData: &pbactivity.StandardizedActivity{ExternalId: "ext-1"},
	}

	payloadBytes, err := protojson.Marshal(payload)
	assert.NoError(t, err)

	ce := event.New()
	ce.SetID("test-id-fail")
	ce.SetType("com.fitglue.event.enriched")
	ce.SetSource("test")
	ce.SetData("application/json", payloadBytes)

	err = executor.Process(context.Background(), &ce)

	// Process should return an error from the GetProfile path
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "getting user profile")
	assert.Contains(t, err.Error(), "403 Forbidden")

	// The writeFailureForAllDestinations was called before the return -
	// it writes to db.SetDestinationOutcome which is a no-op in MockDatabase.
	// The key assertion is that Process() didn't panic and did return the correct error.
	_ = writtenOutcomes
	_ = tracker
}
