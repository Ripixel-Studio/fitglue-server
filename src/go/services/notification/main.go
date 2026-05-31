package main

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"sync"

	"cloud.google.com/go/firestore"
	firebase "firebase.google.com/go/v4"
	"github.com/fitglue/server/src/go/internal/infra"
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

	svc := &notificationService{
		fs:     fsClient,
		fcm:    fcmAdapter,
		logger: logger,
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
	fs     *firestore.Client
	fcm    *notifications.FCMAdapter
	logger infra.Logger
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
				// Email dispatcher: not yet wired — placeholder for future implementation
				s.logger.Info(ctx, "email channel selected but not yet implemented", "user_id", req.UserId)
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

// activeChannels returns the channels configured for this notification type.
// If the preference is absent (nil), defaults to [PUSH] — existing users stay notified.
func activeChannels(prefs *pbuser.NotificationPreferences, t pbnotification.NotificationType) []pbuser.NotificationChannel {
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
	default:
		return "UNKNOWN"
	}
}
