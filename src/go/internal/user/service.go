package user

import (
	"context"
	"fmt"
	"net/http"
	"net/url"
	"os"
	"strings"
	"time"

	firebaseAuth "firebase.google.com/go/v4/auth" // Renamed to avoid conflict with local auth package
	"github.com/fitglue/server/src/go/internal/infra"
	"github.com/fitglue/server/src/go/pkg/domain/email"
	emailsender "github.com/fitglue/server/src/go/pkg/infrastructure/email" // New import
	pbuser "github.com/fitglue/server/src/go/pkg/types/pb/models/user"
	pbsvc "github.com/fitglue/server/src/go/pkg/types/pb/services/user"
	"google.golang.org/api/iterator"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/emptypb"
)

type UserIterator interface {
	PageInfo() *iterator.PageInfo
	Next() (*firebaseAuth.ExportedUserRecord, error)
}

type AuthClient interface {
	GetUser(ctx context.Context, uid string) (*firebaseAuth.UserRecord, error)
	EmailVerificationLinkWithSettings(ctx context.Context, email string, settings *firebaseAuth.ActionCodeSettings) (string, error)
	PasswordResetLinkWithSettings(ctx context.Context, email string, settings *firebaseAuth.ActionCodeSettings) (string, error)
	Users(ctx context.Context, nextPageToken string) UserIterator
}

type Service struct {
	pbsvc.UnimplementedUserServiceServer
	store      Store
	logger     infra.Logger
	sender     emailsender.Sender
	authClient AuthClient
	baseURL    string
}

func NewService(store Store, logger infra.Logger, sender emailsender.Sender, authClient AuthClient, baseURL string) *Service {
	return &Service{
		store:      store,
		logger:     logger,
		sender:     sender,
		authClient: authClient,
		baseURL:    baseURL,
	}
}

func (s *Service) GetProfile(ctx context.Context, req *pbsvc.GetProfileRequest) (*pbuser.UserProfile, error) {
	if req.UserId == "" {
		return nil, status.Error(codes.InvalidArgument, "user_id is required")
	}

	profile, err := s.store.GetProfile(ctx, req.UserId)
	if err != nil {
		s.logger.Error(ctx, "failed to get profile", "err", err, "user_id", req.UserId)
		return nil, status.Error(codes.Internal, "failed to get profile")
	}
	if profile == nil {
		return nil, status.Error(codes.NotFound, "profile not found")
	}

	return profile, nil
}

func (s *Service) UpdateProfile(ctx context.Context, req *pbsvc.UpdateProfileRequest) (*pbuser.UserProfile, error) {
	if req.UserId == "" || req.Profile == nil {
		return nil, status.Error(codes.InvalidArgument, "user_id and profile are required")
	}

	err := s.store.UpdateProfile(ctx, req.UserId, req.Profile)
	if err != nil {
		s.logger.Error(ctx, "failed to update profile", "err", err, "user_id", req.UserId)
		return nil, status.Error(codes.Internal, "failed to update profile")
	}

	return req.Profile, nil
}

func (s *Service) DeleteUser(ctx context.Context, req *pbsvc.DeleteUserRequest) (*emptypb.Empty, error) {
	if req.UserId == "" {
		return nil, status.Error(codes.InvalidArgument, "user_id is required")
	}

	err := s.store.DeleteUser(ctx, req.UserId)
	if err != nil {
		s.logger.Error(ctx, "failed to delete user", "err", err, "user_id", req.UserId)
		return nil, status.Error(codes.Internal, "failed to delete user")
	}

	return &emptypb.Empty{}, nil
}

func (s *Service) GetIntegration(ctx context.Context, req *pbsvc.GetIntegrationRequest) (*pbsvc.GetIntegrationResponse, error) {
	if req.UserId == "" {
		return nil, status.Error(codes.InvalidArgument, "user_id is required")
	}

	integrations, err := s.store.GetIntegrations(ctx, req.UserId)
	if err != nil {
		s.logger.Error(ctx, "failed to get integrations", "err", err, "user_id", req.UserId)
		return nil, status.Error(codes.Internal, "failed to get integrations")
	}

	return &pbsvc.GetIntegrationResponse{Integrations: integrations}, nil
}

func (s *Service) ResolveUserByIntegration(ctx context.Context, req *pbsvc.ResolveUserByIntegrationRequest) (*pbsvc.ResolveUserByIntegrationResponse, error) {
	if req.Provider == "" || req.ProviderUid == "" {
		return nil, status.Error(codes.InvalidArgument, "provider and provider_uid are required")
	}

	profile, err := s.store.FindUserByIntegration(ctx, req.Provider, req.ProviderUid)
	if err != nil {
		if status.Code(err) == codes.NotFound {
			return nil, err
		}
		s.logger.Error(ctx, "failed to resolve user by integration", "err", err, "provider", req.Provider, "provider_uid", req.ProviderUid)
		return nil, status.Error(codes.Internal, "failed to resolve user by integration")
	}

	return &pbsvc.ResolveUserByIntegrationResponse{Profile: profile}, nil
}

func (s *Service) SetIntegration(ctx context.Context, req *pbsvc.SetIntegrationRequest) (*emptypb.Empty, error) {
	if req.UserId == "" || req.Provider == "" {
		return nil, status.Error(codes.InvalidArgument, "user_id and provider are required")
	}
	if req.IntegrationData == nil {
		return nil, status.Error(codes.InvalidArgument, "integration_data is required")
	}

	err := s.store.SetIntegration(ctx, req.UserId, req.Provider, req.IntegrationData.AsMap())
	if err != nil {
		s.logger.Error(ctx, "failed to set integration", "err", err, "user_id", req.UserId, "provider", req.Provider)
		return nil, status.Error(codes.Internal, "failed to set integration")
	}

	return &emptypb.Empty{}, nil
}

func (s *Service) DeleteIntegration(ctx context.Context, req *pbsvc.DeleteIntegrationRequest) (*emptypb.Empty, error) {
	if req.UserId == "" || req.Provider == "" {
		return nil, status.Error(codes.InvalidArgument, "user_id and provider are required")
	}

	// Best-effort token revocation before removing credentials from the store.
	if req.Provider == "strava" {
		integ, err := s.store.GetIntegrations(ctx, req.UserId)
		if err == nil && integ.GetStrava().GetAccessToken() != "" {
			if rErr := revokeStravaToken(ctx, integ.GetStrava().GetAccessToken()); rErr != nil {
				s.logger.Error(ctx, "failed to revoke strava token", "err", rErr, "user_id", req.UserId)
			}
		}
	}

	err := s.store.DeleteIntegration(ctx, req.UserId, req.Provider)
	if err != nil {
		s.logger.Error(ctx, "failed to delete integration", "err", err, "user_id", req.UserId, "provider", req.Provider)
		return nil, status.Error(codes.Internal, "failed to delete integration")
	}

	return &emptypb.Empty{}, nil
}

func revokeStravaToken(ctx context.Context, accessToken string) error {
	body := strings.NewReader(url.Values{"access_token": {accessToken}}.Encode())
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, "https://www.strava.com/oauth/revoke", body)
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return err
	}
	resp.Body.Close()
	if resp.StatusCode >= 400 {
		return fmt.Errorf("strava revoke returned status %d", resp.StatusCode)
	}
	return nil
}

func (s *Service) ListIntegrations(ctx context.Context, req *pbsvc.ListIntegrationsRequest) (*pbuser.UserIntegrations, error) {
	if req.UserId == "" {
		return nil, status.Error(codes.InvalidArgument, "user_id is required")
	}

	integrations, err := s.store.GetIntegrations(ctx, req.UserId)
	if err != nil {
		s.logger.Error(ctx, "failed to list integrations", "err", err, "user_id", req.UserId)
		return nil, status.Error(codes.Internal, "failed to list integrations")
	}

	return integrations, nil
}

func (s *Service) ListCounters(ctx context.Context, req *pbsvc.ListCountersRequest) (*pbsvc.ListCountersResponse, error) {
	if req.UserId == "" {
		return nil, status.Error(codes.InvalidArgument, "user_id is required")
	}

	counters, err := s.store.ListCounters(ctx, req.UserId)
	if err != nil {
		s.logger.Error(ctx, "failed to list counters", "err", err, "user_id", req.UserId)
		return nil, status.Error(codes.Internal, "failed to list counters")
	}

	return &pbsvc.ListCountersResponse{Counters: counters}, nil
}

func (s *Service) UpdateCounter(ctx context.Context, req *pbsvc.UpdateCounterRequest) (*pbuser.Counter, error) {
	if req.UserId == "" || req.CounterId == "" {
		return nil, status.Error(codes.InvalidArgument, "user_id and counter_id are required")
	}

	counter, err := s.store.UpdateCounter(ctx, req.UserId, req.CounterId, req.Count)
	if err != nil {
		s.logger.Error(ctx, "failed to update counter", "err", err, "user_id", req.UserId, "counter_id", req.CounterId)
		return nil, status.Error(codes.Internal, "failed to update counter")
	}

	return counter, nil
}

func (s *Service) GetNotificationPrefs(ctx context.Context, req *pbsvc.GetNotificationPrefsRequest) (*pbuser.NotificationPreferences, error) {
	if req.UserId == "" {
		return nil, status.Error(codes.InvalidArgument, "user_id is required")
	}

	prefs, err := s.store.GetNotificationPrefs(ctx, req.UserId)
	if err != nil {
		s.logger.Error(ctx, "failed to get notification prefs", "err", err, "user_id", req.UserId)
		return nil, status.Error(codes.Internal, "failed to get notification prefs")
	}

	return prefs, nil
}

func (s *Service) UpdateNotificationPrefs(ctx context.Context, req *pbsvc.UpdateNotificationPrefsRequest) (*pbuser.NotificationPreferences, error) {
	if req.UserId == "" || req.Prefs == nil {
		return nil, status.Error(codes.InvalidArgument, "user_id and prefs are required")
	}

	err := s.store.UpdateNotificationPrefs(ctx, req.UserId, req.Prefs)
	if err != nil {
		s.logger.Error(ctx, "failed to update notification prefs", "err", err, "user_id", req.UserId)
		return nil, status.Error(codes.Internal, "failed to update notification prefs")
	}

	return req.Prefs, nil
}

func (s *Service) GetBoosterData(ctx context.Context, req *pbsvc.GetBoosterDataRequest) (*pbsvc.GetBoosterDataResponse, error) {
	if req.UserId == "" {
		return nil, status.Error(codes.InvalidArgument, "user_id is required")
	}

	data, err := s.store.GetBoosterData(ctx, req.UserId, req.BoosterId)
	if err != nil {
		s.logger.Error(ctx, "failed to get booster data", "err", err, "user_id", req.UserId, "booster_id", req.BoosterId)
		return nil, status.Error(codes.Internal, "failed to get booster data")
	}

	return &pbsvc.GetBoosterDataResponse{
		Data: data,
	}, nil
}

func (s *Service) CreateUser(ctx context.Context, req *pbsvc.CreateUserRequest) (*pbuser.UserProfile, error) {
	if req.UserId == "" {
		return nil, status.Error(codes.InvalidArgument, "user_id is required")
	}

	profile, err := s.store.CreateUser(ctx, req.UserId)
	if err != nil {
		s.logger.Error(ctx, "failed to create user", "err", err, "user_id", req.UserId)
		return nil, status.Error(codes.Internal, "failed to create user")
	}

	return profile, nil
}

func (s *Service) SetBoosterData(ctx context.Context, req *pbsvc.SetBoosterDataRequest) (*emptypb.Empty, error) {
	if req.UserId == "" || req.BoosterId == "" || req.Data == nil {
		return nil, status.Error(codes.InvalidArgument, "user_id, booster_id, and data are required")
	}

	err := s.store.SetBoosterData(ctx, req.UserId, req.BoosterId, req.Data)
	if err != nil {
		s.logger.Error(ctx, "failed to set booster data", "err", err, "user_id", req.UserId, "booster_id", req.BoosterId)
		return nil, status.Error(codes.Internal, "failed to set booster data")
	}

	return &emptypb.Empty{}, nil
}

func (s *Service) DeleteBoosterData(ctx context.Context, req *pbsvc.DeleteBoosterDataRequest) (*emptypb.Empty, error) {
	if req.UserId == "" || req.BoosterId == "" {
		return nil, status.Error(codes.InvalidArgument, "user_id and booster_id are required")
	}

	err := s.store.DeleteBoosterData(ctx, req.UserId, req.BoosterId)
	if err != nil {
		s.logger.Error(ctx, "failed to delete booster data", "err", err, "user_id", req.UserId, "booster_id", req.BoosterId)
		return nil, status.Error(codes.Internal, "failed to delete booster data")
	}

	return &emptypb.Empty{}, nil
}

func (s *Service) DeleteCounter(ctx context.Context, req *pbsvc.DeleteCounterRequest) (*emptypb.Empty, error) {
	if req.UserId == "" || req.CounterId == "" {
		return nil, status.Error(codes.InvalidArgument, "user_id and counter_id are required")
	}

	err := s.store.DeleteCounter(ctx, req.UserId, req.CounterId)
	if err != nil {
		s.logger.Error(ctx, "failed to delete counter", "err", err, "user_id", req.UserId, "counter_id", req.CounterId)
		return nil, status.Error(codes.Internal, "failed to delete counter")
	}

	return &emptypb.Empty{}, nil
}

func (s *Service) ListPersonalRecords(ctx context.Context, req *pbsvc.ListPersonalRecordsRequest) (*pbsvc.ListPersonalRecordsResponse, error) {
	if req.UserId == "" {
		return nil, status.Error(codes.InvalidArgument, "user_id is required")
	}

	records, err := s.store.ListPersonalRecords(ctx, req.UserId)
	if err != nil {
		s.logger.Error(ctx, "failed to list personal records", "err", err, "user_id", req.UserId)
		return nil, status.Error(codes.Internal, "failed to list personal records")
	}

	return &pbsvc.ListPersonalRecordsResponse{Records: records}, nil
}

func (s *Service) SetPersonalRecord(ctx context.Context, req *pbsvc.SetPersonalRecordRequest) (*pbuser.PersonalRecord, error) {
	if req.UserId == "" || req.RecordType == "" {
		return nil, status.Error(codes.InvalidArgument, "user_id and record_type are required")
	}

	record := &pbuser.PersonalRecord{
		RecordType: req.RecordType,
		Value:      req.Value,
		Unit:       req.Unit,
		ActivityId: req.ActivityId,
	}

	err := s.store.SetPersonalRecord(ctx, req.UserId, req.RecordType, record)
	if err != nil {
		s.logger.Error(ctx, "failed to set personal record", "err", err, "user_id", req.UserId, "record_type", req.RecordType)
		return nil, status.Error(codes.Internal, "failed to set personal record")
	}

	return record, nil
}

func (s *Service) DeletePersonalRecord(ctx context.Context, req *pbsvc.DeletePersonalRecordRequest) (*emptypb.Empty, error) {
	if req.UserId == "" || req.RecordType == "" {
		return nil, status.Error(codes.InvalidArgument, "user_id and record_type are required")
	}

	err := s.store.DeletePersonalRecord(ctx, req.UserId, req.RecordType)
	if err != nil {
		s.logger.Error(ctx, "failed to delete personal record", "err", err, "user_id", req.UserId, "record_type", req.RecordType)
		return nil, status.Error(codes.Internal, "failed to delete personal record")
	}

	return &emptypb.Empty{}, nil
}

func (s *Service) ListPluginDefaults(ctx context.Context, req *pbsvc.ListPluginDefaultsRequest) (*pbsvc.ListPluginDefaultsResponse, error) {
	if req.UserId == "" {
		return nil, status.Error(codes.InvalidArgument, "user_id is required")
	}

	defaults, err := s.store.ListPluginDefaults(ctx, req.UserId)
	if err != nil {
		s.logger.Error(ctx, "failed to list plugin defaults", "err", err, "user_id", req.UserId)
		return nil, status.Error(codes.Internal, "failed to list plugin defaults")
	}

	return &pbsvc.ListPluginDefaultsResponse{Defaults: defaults}, nil
}

func (s *Service) SetPluginDefaults(ctx context.Context, req *pbsvc.SetPluginDefaultsRequest) (*emptypb.Empty, error) {
	if req.UserId == "" || req.PluginId == "" || req.Defaults == nil {
		return nil, status.Error(codes.InvalidArgument, "user_id, plugin_id, and defaults are required")
	}

	err := s.store.SetPluginDefaults(ctx, req.UserId, req.PluginId, req.Defaults)
	if err != nil {
		s.logger.Error(ctx, "failed to set plugin defaults", "err", err, "user_id", req.UserId, "plugin_id", req.PluginId)
		return nil, status.Error(codes.Internal, "failed to set plugin defaults")
	}

	return &emptypb.Empty{}, nil
}

func (s *Service) DeletePluginDefaults(ctx context.Context, req *pbsvc.DeletePluginDefaultsRequest) (*emptypb.Empty, error) {
	if req.UserId == "" || req.PluginId == "" {
		return nil, status.Error(codes.InvalidArgument, "user_id and plugin_id are required")
	}

	err := s.store.DeletePluginDefaults(ctx, req.UserId, req.PluginId)
	if err != nil {
		s.logger.Error(ctx, "failed to delete plugin defaults", "err", err, "user_id", req.UserId, "plugin_id", req.PluginId)
		return nil, status.Error(codes.Internal, "failed to delete plugin defaults")
	}

	return &emptypb.Empty{}, nil
}

func (s *Service) SendVerificationEmail(ctx context.Context, req *pbsvc.SendVerificationEmailRequest) (*emptypb.Empty, error) {
	if req.UserId == "" {
		return nil, status.Error(codes.InvalidArgument, "user_id is required")
	}

	userRecord, err := s.authClient.GetUser(ctx, req.UserId)
	if err != nil {
		s.logger.Error(ctx, "failed to get user from auth", "err", err, "user_id", req.UserId)
		return nil, status.Error(codes.Internal, "failed to get user")
	}

	if userRecord.UserInfo == nil || userRecord.UserInfo.Email == "" {
		return nil, status.Error(codes.FailedPrecondition, "user has no email address")
	}

	if userRecord.EmailVerified {
		return nil, status.Error(codes.FailedPrecondition, "email is already verified")
	}

	settings := &firebaseAuth.ActionCodeSettings{
		URL:             s.baseURL + "/auth/verify-email",
		HandleCodeInApp: true,
	}

	link, err := s.authClient.EmailVerificationLinkWithSettings(ctx, userRecord.Email, settings)
	if err != nil {
		s.logger.Error(ctx, "failed to generate email verification link", "err", err, "email", userRecord.Email)
		return nil, status.Error(codes.Internal, "failed to generate link")
	}

	html := email.VerifyEmailTemplate(link, s.baseURL)

	err = s.sender.SendEmail(ctx, userRecord.Email, "Verify your FitGlue email", html)
	if err != nil {
		s.logger.Error(ctx, "failed to send verification email", "err", err, "email", userRecord.Email)
		return nil, status.Error(codes.Internal, "failed to send email")
	}

	s.logger.Info(ctx, "Verification email sent", "user_id", req.UserId, "email", userRecord.Email)
	return &emptypb.Empty{}, nil
}

func (s *Service) SendPasswordResetEmail(ctx context.Context, req *pbsvc.SendPasswordResetEmailRequest) (*emptypb.Empty, error) {
	if req.Email == "" {
		return nil, status.Error(codes.InvalidArgument, "email is required")
	}

	settings := &firebaseAuth.ActionCodeSettings{
		URL:             s.baseURL + "/auth/reset-password",
		HandleCodeInApp: true,
	}

	link, err := s.authClient.PasswordResetLinkWithSettings(ctx, req.Email, settings)
	if err != nil {
		// Log but return success to prevent email enumeration
		s.logger.Info(ctx, "Failed to generate password reset link (likely user not found)", "email", req.Email, "err", err)
		return &emptypb.Empty{}, nil
	}

	html := email.PasswordResetTemplate(link, s.baseURL)

	err = s.sender.SendEmail(ctx, req.Email, "Reset your FitGlue password", html)
	if err != nil {
		s.logger.Error(ctx, "failed to send password reset email", "err", err, "email", req.Email)
		return nil, status.Error(codes.Internal, "failed to send email")
	}

	s.logger.Info(ctx, "Password reset email sent", "email", req.Email)
	return &emptypb.Empty{}, nil
}

func (s *Service) SendEmailChangeVerification(ctx context.Context, req *pbsvc.SendEmailChangeVerificationRequest) (*emptypb.Empty, error) {
	if req.UserId == "" || req.NewEmail == "" {
		return nil, status.Error(codes.InvalidArgument, "user_id and new_email are required")
	}

	userRecord, err := s.authClient.GetUser(ctx, req.UserId)
	if err != nil {
		s.logger.Error(ctx, "failed to get user from auth", "err", err, "user_id", req.UserId)
		return nil, status.Error(codes.Internal, "failed to get user")
	}

	if userRecord.UserInfo == nil || userRecord.UserInfo.Email == "" {
		return nil, status.Error(codes.FailedPrecondition, "user has no current email address")
	}

	settings := &firebaseAuth.ActionCodeSettings{
		URL:             "https://fitglue.tech/auth/verify-email-change",
		HandleCodeInApp: true,
	}

	link, err := s.authClient.EmailVerificationLinkWithSettings(ctx, req.NewEmail, settings)
	if err != nil {
		s.logger.Error(ctx, "failed to generate email change verification link", "err", err, "newEmail", req.NewEmail)
		return nil, status.Error(codes.Internal, "failed to generate verification link")
	}

	html := email.ChangeEmailTemplate(link, req.NewEmail, "https://fitglue.tech")

	err = s.sender.SendEmail(ctx, req.NewEmail, "Confirm your new FitGlue email", html)
	if err != nil {
		s.logger.Error(ctx, "failed to send email change verification", "err", err, "newEmail", req.NewEmail)
		return nil, status.Error(codes.Internal, "failed to send verification email")
	}

	s.logger.Info(ctx, "Email change verification sent", "user_id", req.UserId, "newEmail", req.NewEmail)
	return &emptypb.Empty{}, nil
}

func (s *Service) SendWelcomeEmail(ctx context.Context, req *pbsvc.SendWelcomeEmailRequest) (*emptypb.Empty, error) {
	if req.UserId == "" {
		return nil, status.Error(codes.InvalidArgument, "user_id is required")
	}

	userRecord, err := s.authClient.GetUser(ctx, req.UserId)
	if err != nil {
		s.logger.Error(ctx, "failed to get user from auth", "err", err, "user_id", req.UserId)
		return nil, status.Error(codes.Internal, "failed to get user")
	}

	if userRecord.UserInfo == nil || userRecord.UserInfo.Email == "" {
		return nil, status.Error(codes.FailedPrecondition, "user has no email address")
	}

	html := email.WelcomeTemplate(s.baseURL)

	err = s.sender.SendEmail(ctx, userRecord.Email, "Welcome to FitGlue! 🎉", html)
	if err != nil {
		s.logger.Error(ctx, "failed to send welcome email", "err", err, "email", userRecord.Email)
		return nil, status.Error(codes.Internal, "failed to send email")
	}

	s.logger.Info(ctx, "Welcome email sent", "user_id", req.UserId, "email", userRecord.Email)
	return &emptypb.Empty{}, nil
}

func (s *Service) GenerateRegistrationSummary(ctx context.Context, req *pbsvc.GenerateRegistrationSummaryRequest) (*emptypb.Empty, error) {
	s.logger.Info(ctx, "Generating registration summary")

	now := time.Now()
	var endDate, startDate time.Time

	if req.DateOverride != "" {
		// Use provided date
		parsedDate, err := time.Parse("2006-01-02", req.DateOverride)
		if err != nil {
			return nil, status.Errorf(codes.InvalidArgument, "Invalid date format: %v", err)
		}
		endDate = parsedDate.Add(24 * time.Hour)
		startDate = parsedDate
	} else {
		// Use UTC midnight of current day
		endDate = time.Date(now.UTC().Year(), now.UTC().Month(), now.UTC().Day(), 0, 0, 0, 0, time.UTC)
		startDate = endDate.Add(-24 * time.Hour)
	}

	users, err := s.store.FindUsersByDateRange(ctx, startDate, endDate)
	if err != nil {
		s.logger.Error(ctx, "Failed to fetch users by date range", "error", err)
		return nil, status.Errorf(codes.Internal, "Failed to fetch users: %v", err)
	}

	s.logger.Info(ctx, "Found users created in range", "count", len(users), "start", startDate, "end", endDate)

	var summaries []email.RegistrationSummaryUser
	for _, user := range users {
		summary := email.RegistrationSummaryUser{
			CreatedAt:     user.CreatedAt.AsTime().Format("2006-01-02 15:04"),
			AccessEnabled: user.AccessEnabled,
		}

		authUser, authErr := s.authClient.GetUser(ctx, user.UserId)
		if authErr != nil {
			s.logger.Warn(ctx, "Could not fetch auth user for summary", "userId", user.UserId, "error", authErr)
			summary.Email = "Unknown"
		} else {
			summary.Email = authUser.Email
			if summary.Email == "" {
				summary.Email = "No email"
			}
		}

		summaries = append(summaries, summary)
	}

	dateStr := startDate.Format("2006-01-02")

	baseURL := os.Getenv("FITGLUE_WEB_URL")
	if baseURL == "" {
		baseURL = "https://app.fitglue.tech" // Fallback
	}

	htmlContent := email.RegistrationSummaryTemplate(dateStr, summaries, baseURL)

	subject := ""
	if len(summaries) == 0 {
		subject = "[FitGlue] No new registrations - " + dateStr
	} else if len(summaries) == 1 {
		subject = "[FitGlue] 1 new registration - " + dateStr
	} else {
		subject = fmt.Sprintf("[FitGlue] %d new registrations - %s", len(summaries), dateStr)
	}

	recipient := "james@fitglue.tech" // To be moved to env var if needed

	err = s.sender.SendEmail(ctx, recipient, subject, htmlContent)
	if err != nil {
		s.logger.Error(ctx, "Failed to send registration summary email", "error", err)
		return nil, status.Errorf(codes.Internal, "Failed to send summary email: %v", err)
	}

	s.logger.Info(ctx, "Successfully sent registration summary email", "recipient", recipient)
	return &emptypb.Empty{}, nil
}

func (s *Service) ListUsers(ctx context.Context, req *pbsvc.ListUsersRequest) (*pbsvc.ListUsersResponse, error) {
	limit := req.Limit
	if limit <= 0 {
		limit = 50
	}

	iter := s.authClient.Users(ctx, req.PageToken)
	if iter == nil {
		return nil, status.Error(codes.Internal, "auth client users returned nil iterator")
	}

	pager := iterator.NewPager(iter, int(limit), req.PageToken)
	var authUsers []*firebaseAuth.ExportedUserRecord
	nextToken, err := pager.NextPage(&authUsers)
	if err != nil {
		s.logger.Error(ctx, "failed to get users page", "err", err)
		return nil, status.Error(codes.Internal, "failed to list users")
	}

	var pbUsers []*pbuser.UserProfile
	for _, u := range authUsers {
		profile, err := s.store.GetProfile(ctx, u.UID)
		if err != nil {
			s.logger.Warn(ctx, "failed to get user profile", "uid", u.UID, "error", err)
		}
		if profile == nil {
			profile = &pbuser.UserProfile{
				UserId: u.UID,
				Tier:   pbuser.UserTier_USER_TIER_HOBBYIST,
			}
		}
		profile.Email = u.Email
		profile.DisplayName = u.DisplayName

		pbUsers = append(pbUsers, profile)
	}

	return &pbsvc.ListUsersResponse{
		Users:         pbUsers,
		NextPageToken: nextToken,
	}, nil
}
