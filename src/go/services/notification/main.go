package main

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"strconv"
	"sync"

	"cloud.google.com/go/firestore"
	firebase "firebase.google.com/go/v4"
	"github.com/fitglue/server/src/go/internal/infra"
	emaildomain "github.com/fitglue/server/src/go/pkg/domain/email"
	emailinfra "github.com/fitglue/server/src/go/pkg/infrastructure/email"
	"github.com/fitglue/server/src/go/pkg/infrastructure/notifications"
	pbnotification "github.com/fitglue/server/src/go/pkg/types/pb/models/notification"
	pbuser "github.com/fitglue/server/src/go/pkg/types/pb/models/user"
	"google.golang.org/protobuf/encoding/protojson"
)

func main() {
	ctx := context.Background()
	logger := infra.NewLoggerWithComponent("notification")
	infra.InitSentry()

	projectID := os.Getenv("PROJECT_ID")
	if projectID == "" {
		projectID = "fitglue-server-dev"
	}

	fsClient, err := firestore.NewClient(ctx, projectID)
	if err != nil {
		log.Fatalf("firestore init: %v", err)
	}
	defer fsClient.Close()

	fbApp, err := firebase.NewApp(ctx, &firebase.Config{ProjectID: projectID})
	if err != nil {
		log.Fatalf("firebase init: %v", err)
	}

	fcmAdapter, err := notifications.NewFCMAdapter(ctx, fbApp, fsClient, logger)
	if err != nil {
		log.Fatalf("FCM init: %v", err)
	}

	baseURL := os.Getenv("BASE_URL")
	if baseURL == "" {
		baseURL = "https://fitglue.tech"
	}

	// Email sender — required for billing transactional emails.
	emailUser := os.Getenv("SYSTEM_EMAIL")
	emailPass := os.Getenv("EMAIL_APP_PASSWORD")
	smtpHost := os.Getenv("EMAIL_SMTP_HOST")
	if smtpHost == "" {
		smtpHost = "smtp.gmail.com"
	}
	smtpPort := 465
	if p := os.Getenv("EMAIL_SMTP_PORT"); p != "" {
		if n, err := strconv.Atoi(p); err == nil {
			smtpPort = n
		}
	}

	var emailSender emailinfra.Sender
	if emailUser != "" && emailPass != "" {
		emailSender = emailinfra.NewSMTPSender(smtpHost, smtpPort, emailUser, emailPass)
	} else {
		logger.Info(ctx, "SYSTEM_EMAIL/EMAIL_APP_PASSWORD not set — email channel will be skipped")
	}

	svc := &notificationService{
		fs:          fsClient,
		fcm:         fcmAdapter,
		emailSender: emailSender,
		baseURL:     baseURL,
		logger:      logger,
	}

	mux := http.NewServeMux()
	mux.HandleFunc("/pubsub/notifications", svc.handlePubSub)
	mux.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	port := os.Getenv("PORT")
	if port == "" {
		port = "8080"
	}
	logger.Info(ctx, "Starting service.notification", "port", port)
	if err := http.ListenAndServe(":"+port, infra.LoggingMiddleware(logger, mux)); err != nil {
		log.Fatalf("serve: %v", err)
	}
}

type notificationService struct {
	fs          *firestore.Client
	fcm         *notifications.FCMAdapter
	emailSender emailinfra.Sender
	baseURL     string
	logger      infra.Logger
}

func (s *notificationService) handlePubSub(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	body, err := io.ReadAll(r.Body)
	if err != nil {
		s.logger.Error(ctx, "read body", "error", err)
		http.Error(w, "bad request", http.StatusBadRequest)
		return
	}
	defer r.Body.Close()

	// Unwrap Pub/Sub push envelope
	var envelope struct {
		Message struct {
			Data []byte `json:"data"`
		} `json:"message"`
	}
	msg := body
	if err := json.Unmarshal(body, &envelope); err == nil && len(envelope.Message.Data) > 0 {
		msg = envelope.Message.Data
	}

	var req pbnotification.NotificationRequest
	unmarshaler := protojson.UnmarshalOptions{DiscardUnknown: true}
	if err := unmarshaler.Unmarshal(msg, &req); err != nil {
		s.logger.Error(ctx, "unmarshal NotificationRequest", "error", err)
		http.Error(w, "bad request", http.StatusBadRequest)
		return
	}

	if err := s.dispatch(ctx, &req); err != nil {
		s.logger.Error(ctx, "dispatch failed", "error", err, "user_id", req.UserId, "type", req.Type.String())
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}
	w.WriteHeader(http.StatusOK)
}

// dispatch reads the user's notification preferences and fans out to every active channel.
func (s *notificationService) dispatch(ctx context.Context, req *pbnotification.NotificationRequest) error {
	doc, err := s.fs.Collection("users").Doc(req.UserId).Get(ctx)
	if err != nil {
		return fmt.Errorf("get user: %w", err)
	}

	// Decode FCM tokens
	var fcmTokens []string
	if raw, ok := doc.Data()["fcm_tokens"]; ok {
		if slice, ok := raw.([]interface{}); ok {
			for _, v := range slice {
				if t, ok := v.(string); ok {
					fcmTokens = append(fcmTokens, t)
				}
			}
		}
	}

	// Decode notification preferences
	var prefs pbuser.NotificationPreferences
	if rawPrefs, ok := doc.Data()["notification_preferences"]; ok {
		if prefsMap, ok := rawPrefs.(map[string]interface{}); ok {
			prefsJSON, _ := json.Marshal(prefsMap)
			_ = protojson.UnmarshalOptions{DiscardUnknown: true}.Unmarshal(prefsJSON, &prefs)
		}
	}

	// Read user email for email channel
	userEmail, _ := doc.Data()["email"].(string)

	channels := activeChannels(&prefs, req.Type)
	if len(channels) == 0 {
		s.logger.Info(ctx, "notification suppressed by user prefs", "user_id", req.UserId, "type", req.Type.String())
		return nil
	}

	// Fan-out: dispatch to each channel concurrently
	var wg sync.WaitGroup
	for _, ch := range channels {
		ch := ch
		wg.Add(1)
		go func() {
			defer wg.Done()
			switch ch {
			case pbuser.NotificationChannel_NOTIFICATION_CHANNEL_PUSH:
				s.dispatchPush(ctx, req, fcmTokens)
			case pbuser.NotificationChannel_NOTIFICATION_CHANNEL_EMAIL:
				s.dispatchEmail(ctx, req, userEmail)
			}
		}()
	}
	wg.Wait()
	return nil
}

func (s *notificationService) dispatchPush(ctx context.Context, req *pbnotification.NotificationRequest, tokens []string) {
	if len(tokens) == 0 {
		return
	}
	// Merge the notification type into the data map so clients can deep-link
	data := make(map[string]string, len(req.Data)+1)
	for k, v := range req.Data {
		data[k] = v
	}
	data["type"] = notificationTypeString(req.Type)
	data["user_id"] = req.UserId

	if err := s.fcm.SendPushNotification(ctx, req.UserId, req.Title, req.Body, tokens, data); err != nil {
		s.logger.Error(ctx, "FCM send failed", "error", err, "user_id", req.UserId)
	}
}

func (s *notificationService) dispatchEmail(ctx context.Context, req *pbnotification.NotificationRequest, toEmail string) {
	if s.emailSender == nil {
		s.logger.Info(ctx, "email sender not configured, skipping", "user_id", req.UserId, "type", req.Type.String())
		return
	}
	if toEmail == "" {
		s.logger.Info(ctx, "user has no email address, skipping email dispatch", "user_id", req.UserId)
		return
	}

	subject, html := s.renderEmail(req)
	if html == "" {
		s.logger.Info(ctx, "no email template for notification type", "type", req.Type.String())
		return
	}

	if err := s.emailSender.SendEmail(ctx, toEmail, subject, html); err != nil {
		s.logger.Error(ctx, "email send failed", "error", err, "user_id", req.UserId, "to", toEmail)
	}
}

// renderEmail returns the subject and HTML body for a given notification.
// Returns empty strings if there is no email template for this type.
func (s *notificationService) renderEmail(req *pbnotification.NotificationRequest) (subject, html string) {
	switch req.Type {
	case pbnotification.NotificationType_NOTIFICATION_TYPE_SUBSCRIPTION_STARTED:
		return "You're now a FitGlue Athlete!", emaildomain.SubscriptionConfirmationTemplate(s.baseURL)

	case pbnotification.NotificationType_NOTIFICATION_TYPE_PAYMENT_FAILED:
		return "Action required: your FitGlue payment failed", emaildomain.PaymentFailedTemplate(s.baseURL)

	case pbnotification.NotificationType_NOTIFICATION_TYPE_SUBSCRIPTION_ENDED:
		return "Your FitGlue Athlete subscription has ended", emaildomain.SubscriptionCancelledTemplate(s.baseURL)

	case pbnotification.NotificationType_NOTIFICATION_TYPE_TRIAL_EXPIRING:
		daysLeft := 7
		if d, ok := req.Data["days_left"]; ok {
			if n, err := strconv.Atoi(d); err == nil {
				daysLeft = n
			}
		}
		plural := ""
		if daysLeft != 1 {
			plural = "s"
		}
		return fmt.Sprintf("Your Athlete trial ends in %d day%s", daysLeft, plural),
			emaildomain.TrialExpiringTemplate(daysLeft, s.baseURL)

	case pbnotification.NotificationType_NOTIFICATION_TYPE_TRIAL_EXPIRED:
		return "Your FitGlue Athlete trial has ended", emaildomain.TrialExpiredTemplate(s.baseURL)

	default:
		return "", ""
	}
}

// activeChannels returns the channels configured for this notification type.
// Billing notification types are transactional — they always go to email regardless
// of user preferences. All other types default to [PUSH] when no preference is set.
func activeChannels(prefs *pbuser.NotificationPreferences, t pbnotification.NotificationType) []pbuser.NotificationChannel {
	switch t {
	// Transactional billing types: always email, never suppressed by prefs.
	case pbnotification.NotificationType_NOTIFICATION_TYPE_SUBSCRIPTION_STARTED,
		pbnotification.NotificationType_NOTIFICATION_TYPE_PAYMENT_FAILED,
		pbnotification.NotificationType_NOTIFICATION_TYPE_SUBSCRIPTION_ENDED,
		pbnotification.NotificationType_NOTIFICATION_TYPE_TRIAL_EXPIRED:
		return []pbuser.NotificationChannel{pbuser.NotificationChannel_NOTIFICATION_CHANNEL_EMAIL}

	// Trial expiring: push reminder + email.
	case pbnotification.NotificationType_NOTIFICATION_TYPE_TRIAL_EXPIRING:
		return []pbuser.NotificationChannel{
			pbuser.NotificationChannel_NOTIFICATION_CHANNEL_PUSH,
			pbuser.NotificationChannel_NOTIFICATION_CHANNEL_EMAIL,
		}
	}

	// Pipeline / other types: respect user preferences.
	var typePref *pbuser.NotificationTypePreference
	switch t {
	case pbnotification.NotificationType_NOTIFICATION_TYPE_PENDING_INPUT:
		typePref = prefs.GetPendingInput()
	case pbnotification.NotificationType_NOTIFICATION_TYPE_PIPELINE_SUCCESS:
		typePref = prefs.GetPipelineSuccess()
	case pbnotification.NotificationType_NOTIFICATION_TYPE_PIPELINE_FAILURE:
		typePref = prefs.GetPipelineFailure()
	case pbnotification.NotificationType_NOTIFICATION_TYPE_CONNECTION_ACTION:
		typePref = prefs.GetConnectionAction()
	case pbnotification.NotificationType_NOTIFICATION_TYPE_SHOWCASE_ROUNDUP:
		typePref = prefs.GetShowcaseRoundup()
	case pbnotification.NotificationType_NOTIFICATION_TYPE_PIPELINE_CANCELLED:
		typePref = prefs.GetPipelineCancelled()
	}
	if typePref == nil {
		return []pbuser.NotificationChannel{pbuser.NotificationChannel_NOTIFICATION_CHANNEL_PUSH}
	}
	return typePref.GetChannels()
}

// notificationTypeString converts the proto enum to the string the web clients expect.
func notificationTypeString(t pbnotification.NotificationType) string {
	switch t {
	case pbnotification.NotificationType_NOTIFICATION_TYPE_PENDING_INPUT:
		return "PENDING_INPUT"
	case pbnotification.NotificationType_NOTIFICATION_TYPE_PIPELINE_SUCCESS:
		return "PIPELINE_SUCCESS"
	case pbnotification.NotificationType_NOTIFICATION_TYPE_PIPELINE_FAILURE:
		return "PIPELINE_FAILED"
	case pbnotification.NotificationType_NOTIFICATION_TYPE_CONNECTION_ACTION:
		return "CONNECTION_ACTION"
	case pbnotification.NotificationType_NOTIFICATION_TYPE_SHOWCASE_ROUNDUP:
		return "SHOWCASE_ROUNDUP"
	case pbnotification.NotificationType_NOTIFICATION_TYPE_PIPELINE_CANCELLED:
		return "PIPELINE_CANCELLED"
	case pbnotification.NotificationType_NOTIFICATION_TYPE_SUBSCRIPTION_STARTED:
		return "SUBSCRIPTION_STARTED"
	case pbnotification.NotificationType_NOTIFICATION_TYPE_PAYMENT_FAILED:
		return "PAYMENT_FAILED"
	case pbnotification.NotificationType_NOTIFICATION_TYPE_SUBSCRIPTION_ENDED:
		return "SUBSCRIPTION_ENDED"
	case pbnotification.NotificationType_NOTIFICATION_TYPE_TRIAL_EXPIRING:
		return "TRIAL_EXPIRING"
	case pbnotification.NotificationType_NOTIFICATION_TYPE_TRIAL_EXPIRED:
		return "TRIAL_EXPIRED"
	default:
		return "UNKNOWN"
	}
}
