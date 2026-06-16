package activity

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	shared "github.com/fitglue/server/src/go/pkg"
	pbsvc "github.com/fitglue/server/src/go/pkg/types/pb/services/activity"
	"github.com/google/uuid"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// exportSignedURLExpiry is how long whole-account and per-run download links live.
const exportSignedURLExpiry = 24 * time.Hour

// exportTriggerMessage is the Pub/Sub payload that drives the async export worker.
type exportTriggerMessage struct {
	UserID string `json:"user_id"`
	JobID  string `json:"job_id"`
}

// ExportData enqueues an asynchronous whole-account GDPR export. It de-duplicates
// against any in-flight job, records a PENDING job, publishes a trigger to the
// worker, and returns the job immediately. Clients poll GetExportJob.
func (s *Service) ExportData(ctx context.Context, req *pbsvc.ExportDataRequest) (*pbsvc.ExportDataResponse, error) {
	if req.UserId == "" {
		return nil, status.Error(codes.InvalidArgument, "user_id is required")
	}

	// De-dupe: return the existing in-flight job rather than starting another.
	if active, err := s.store.LatestActiveExportJob(ctx, req.UserId); err == nil && active != nil {
		return &pbsvc.ExportDataResponse{Job: s.toProtoExportJob(ctx, active)}, nil
	}

	now := time.Now().UTC()
	job := &ExportJobRecord{
		JobID:     uuid.NewString(),
		Status:    ExportStatusPending,
		CreatedAt: now,
		UpdatedAt: now,
	}
	if err := s.store.CreateExportJob(ctx, req.UserId, job); err != nil {
		s.logger.Error(ctx, "ExportData: failed to create job", "userId", req.UserId, "error", err)
		return nil, status.Error(codes.Internal, "failed to start export")
	}

	msg, _ := json.Marshal(exportTriggerMessage{UserID: req.UserId, JobID: job.JobID})
	if err := s.publisher.PublishJSON(ctx, shared.TopicDataExport, msg); err != nil {
		s.logger.Error(ctx, "ExportData: failed to publish trigger", "userId", req.UserId, "jobId", job.JobID, "error", err)
		_ = s.store.UpdateExportJob(ctx, req.UserId, job.JobID, map[string]interface{}{
			"status": ExportStatusFailed,
			"error":  "failed to enqueue export",
		})
		return nil, status.Error(codes.Internal, "failed to enqueue export")
	}

	s.logger.Info(ctx, "ExportData: job enqueued", "userId", req.UserId, "jobId", job.JobID)
	return &pbsvc.ExportDataResponse{Job: s.toProtoExportJob(ctx, job)}, nil
}

// GetExportJob returns the current state of a whole-account export job. When the
// job is READY the download URL is freshly signed so it is never stale.
func (s *Service) GetExportJob(ctx context.Context, req *pbsvc.GetExportJobRequest) (*pbsvc.ExportJob, error) {
	if req.UserId == "" || req.JobId == "" {
		return nil, status.Error(codes.InvalidArgument, "user_id and job_id are required")
	}
	rec, err := s.store.GetExportJob(ctx, req.UserId, req.JobId)
	if err != nil {
		s.logger.Error(ctx, "GetExportJob: lookup failed", "userId", req.UserId, "jobId", req.JobId, "error", err)
		return nil, status.Error(codes.Internal, "failed to load export job")
	}
	if rec == nil {
		return nil, status.Error(codes.NotFound, "export job not found")
	}
	return s.toProtoExportJob(ctx, rec), nil
}

// ExportPipelineRun synchronously builds a per-run ZIP and returns a signed URL.
func (s *Service) ExportPipelineRun(ctx context.Context, req *pbsvc.ExportPipelineRunRequest) (*pbsvc.ExportPipelineRunResponse, error) {
	if req.UserId == "" || req.RunId == "" {
		return nil, status.Error(codes.InvalidArgument, "user_id and run_id are required")
	}

	run, err := s.store.GetPipelineRun(ctx, req.UserId, req.RunId)
	if err != nil {
		s.logger.Error(ctx, "ExportPipelineRun: failed to get run", "userId", req.UserId, "runId", req.RunId, "error", err)
		return nil, status.Error(codes.Internal, "failed to load pipeline run")
	}
	if run == nil {
		return nil, status.Error(codes.NotFound, "pipeline run not found")
	}

	data, err := s.buildRunArchive(ctx, run)
	if err != nil {
		s.logger.Error(ctx, "ExportPipelineRun: failed to build archive", "userId", req.UserId, "runId", req.RunId, "error", err)
		return nil, status.Error(codes.Internal, "failed to build export")
	}

	objectPath := fmt.Sprintf("exports/%s/runs/%s-%d.zip", req.UserId, sanitizeDir(req.RunId), time.Now().UnixMilli())
	if err := s.blobStore.Write(ctx, s.bucketName, objectPath, data); err != nil {
		s.logger.Error(ctx, "ExportPipelineRun: failed to write archive", "userId", req.UserId, "error", err)
		return nil, status.Error(codes.Internal, "failed to store export")
	}

	url, err := s.blobStore.SignedURL(ctx, s.bucketName, objectPath, "", 0, exportSignedURLExpiry)
	if err != nil {
		s.logger.Error(ctx, "ExportPipelineRun: failed to sign URL", "userId", req.UserId, "error", err)
		return nil, status.Error(codes.Internal, "failed to generate download link")
	}

	return &pbsvc.ExportPipelineRunResponse{
		DownloadUrl: url,
		SizeBytes:   int64(len(data)),
		ExpiresAt:   time.Now().UTC().Add(exportSignedURLExpiry).Format(time.RFC3339),
	}, nil
}

// toProtoExportJob maps a stored record to the API type, signing a fresh download
// URL when the job is READY.
func (s *Service) toProtoExportJob(ctx context.Context, rec *ExportJobRecord) *pbsvc.ExportJob {
	job := &pbsvc.ExportJob{
		JobId:     rec.JobID,
		Status:    exportStatusToProto(rec.Status),
		SizeBytes: rec.SizeBytes,
		Error:     rec.Error,
		CreatedAt: rec.CreatedAt.Format(time.RFC3339),
	}
	if rec.Status == ExportStatusReady && rec.ObjectPath != "" {
		url, err := s.blobStore.SignedURL(ctx, s.bucketName, rec.ObjectPath, "", 0, exportSignedURLExpiry)
		if err != nil {
			s.logger.Error(ctx, "toProtoExportJob: failed to sign URL", "jobId", rec.JobID, "error", err)
		} else {
			job.DownloadUrl = url
			job.ExpiresAt = time.Now().UTC().Add(exportSignedURLExpiry).Format(time.RFC3339)
		}
	}
	return job
}

func exportStatusToProto(s string) pbsvc.ExportJobStatus {
	switch s {
	case ExportStatusPending:
		return pbsvc.ExportJobStatus_EXPORT_JOB_STATUS_PENDING
	case ExportStatusProcessing:
		return pbsvc.ExportJobStatus_EXPORT_JOB_STATUS_PROCESSING
	case ExportStatusReady:
		return pbsvc.ExportJobStatus_EXPORT_JOB_STATUS_READY
	case ExportStatusFailed:
		return pbsvc.ExportJobStatus_EXPORT_JOB_STATUS_FAILED
	default:
		return pbsvc.ExportJobStatus_EXPORT_JOB_STATUS_UNSPECIFIED
	}
}
