package activity

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"

	"github.com/fitglue/server/src/go/pkg/notificationpub"
	pbnotification "github.com/fitglue/server/src/go/pkg/types/pb/models/notification"
)

// HandleExportTrigger is the Pub/Sub push handler for /pubsub/data-export. It
// builds the whole-account ZIP for the job referenced in the message, stores it,
// marks the job READY, and emails the user a download link.
//
// It always returns 200 once the message is parsed: the job's own FAILED status
// carries the error to the client, and acking avoids Pub/Sub redelivering a
// (potentially expensive) export that deterministically fails.
func (s *Service) HandleExportTrigger(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	body, err := io.ReadAll(r.Body)
	if err != nil {
		s.logger.Error(ctx, "export worker: failed to read body", "error", err)
		http.Error(w, "bad request", http.StatusBadRequest)
		return
	}
	defer r.Body.Close()

	var envelope struct {
		Message struct {
			Data []byte `json:"data"`
		} `json:"message"`
	}
	msg := body
	if err := json.Unmarshal(body, &envelope); err == nil && len(envelope.Message.Data) > 0 {
		msg = envelope.Message.Data
	}

	var trigger exportTriggerMessage
	if err := json.Unmarshal(msg, &trigger); err != nil || trigger.UserID == "" || trigger.JobID == "" {
		s.logger.Error(ctx, "export worker: invalid message", "error", err)
		http.Error(w, "bad request", http.StatusBadRequest)
		return
	}

	s.runExportJob(ctx, trigger.UserID, trigger.JobID)
	w.WriteHeader(http.StatusOK)
}

func (s *Service) runExportJob(ctx context.Context, userID, jobID string) {
	if err := s.store.UpdateExportJob(ctx, userID, jobID, map[string]interface{}{
		"status": ExportStatusProcessing,
	}); err != nil {
		s.logger.Warn(ctx, "export worker: failed to mark processing", "jobId", jobID, "error", err)
	}

	data, err := s.buildAccountArchive(ctx, userID)
	if err != nil {
		s.failExportJob(ctx, userID, jobID, "failed to build export archive", err)
		return
	}

	objectPath := fmt.Sprintf("exports/%s/%s.zip", userID, jobID)
	if err := s.blobStore.Write(ctx, s.bucketName, objectPath, data); err != nil {
		s.failExportJob(ctx, userID, jobID, "failed to store export archive", err)
		return
	}

	if err := s.store.UpdateExportJob(ctx, userID, jobID, map[string]interface{}{
		"status":      ExportStatusReady,
		"object_path": objectPath,
		"size_bytes":  int64(len(data)),
		"error":       "",
	}); err != nil {
		s.logger.Error(ctx, "export worker: failed to mark ready", "jobId", jobID, "error", err)
		return
	}

	s.logger.Info(ctx, "export worker: job ready", "userId", userID, "jobId", jobID, "sizeBytes", len(data))
	s.notifyExportReady(ctx, userID, objectPath)
}

func (s *Service) failExportJob(ctx context.Context, userID, jobID, msg string, cause error) {
	s.logger.Error(ctx, "export worker: "+msg, "userId", userID, "jobId", jobID, "error", cause)
	_ = s.store.UpdateExportJob(ctx, userID, jobID, map[string]interface{}{
		"status": ExportStatusFailed,
		"error":  msg,
	})
}

// notifyExportReady emails the user a fresh signed download link (best-effort).
func (s *Service) notifyExportReady(ctx context.Context, userID, objectPath string) {
	url, err := s.blobStore.SignedURL(ctx, s.bucketName, objectPath, "", 0, exportSignedURLExpiry)
	if err != nil {
		s.logger.Warn(ctx, "export worker: failed to sign URL for email", "error", err)
		return
	}
	req := &pbnotification.NotificationRequest{
		UserId: userID,
		Type:   pbnotification.NotificationType_NOTIFICATION_TYPE_DATA_EXPORT_READY,
		Title:  "Your data export is ready",
		Body:   "Your FitGlue data export is ready to download.",
		Data:   map[string]string{"download_url": url},
	}
	if err := notificationpub.Enqueue(ctx, s.publisher, req); err != nil {
		s.logger.Warn(ctx, "export worker: failed to enqueue notification", "error", err)
	}
}
