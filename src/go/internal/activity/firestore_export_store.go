package activity

import (
	"context"
	"time"

	"cloud.google.com/go/firestore"
	"google.golang.org/api/iterator"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

const exportJobsCollection = "export_jobs"

func (s *FirestoreStore) CreateExportJob(ctx context.Context, userID string, job *ExportJobRecord) error {
	_, err := s.client.Collection("users").Doc(userID).
		Collection(exportJobsCollection).Doc(job.JobID).Set(ctx, exportJobToMap(job))
	return err
}

func (s *FirestoreStore) GetExportJob(ctx context.Context, userID, jobID string) (*ExportJobRecord, error) {
	doc, err := s.client.Collection("users").Doc(userID).
		Collection(exportJobsCollection).Doc(jobID).Get(ctx)
	if err != nil {
		if status.Code(err) == codes.NotFound {
			return nil, nil
		}
		return nil, err
	}
	return exportJobFromMap(jobID, doc.Data()), nil
}

func (s *FirestoreStore) UpdateExportJob(ctx context.Context, userID, jobID string, fields map[string]interface{}) error {
	fields["updated_at"] = time.Now().UTC()
	_, err := s.client.Collection("users").Doc(userID).
		Collection(exportJobsCollection).Doc(jobID).Set(ctx, fields, firestore.MergeAll)
	return err
}

// LatestActiveExportJob returns an in-flight (PENDING/PROCESSING) job for the user.
// The `in` filter on a single field uses Firestore's automatic single-field index,
// so this needs no composite index.
func (s *FirestoreStore) LatestActiveExportJob(ctx context.Context, userID string) (*ExportJobRecord, error) {
	iter := s.client.Collection("users").Doc(userID).Collection(exportJobsCollection).
		Where("status", "in", []string{ExportStatusPending, ExportStatusProcessing}).
		Limit(1).Documents(ctx)
	defer iter.Stop()

	doc, err := iter.Next()
	if err == iterator.Done {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return exportJobFromMap(doc.Ref.ID, doc.Data()), nil
}

func exportJobToMap(j *ExportJobRecord) map[string]interface{} {
	return map[string]interface{}{
		"job_id":      j.JobID,
		"status":      j.Status,
		"object_path": j.ObjectPath,
		"size_bytes":  j.SizeBytes,
		"error":       j.Error,
		"created_at":  j.CreatedAt,
		"updated_at":  j.UpdatedAt,
	}
}

func exportJobFromMap(id string, m map[string]interface{}) *ExportJobRecord {
	rec := &ExportJobRecord{JobID: id}
	if v, ok := m["status"].(string); ok {
		rec.Status = v
	}
	if v, ok := m["object_path"].(string); ok {
		rec.ObjectPath = v
	}
	if v, ok := m["error"].(string); ok {
		rec.Error = v
	}
	if v, ok := m["size_bytes"].(int64); ok {
		rec.SizeBytes = v
	}
	if t, ok := m["created_at"].(time.Time); ok {
		rec.CreatedAt = t
	}
	if t, ok := m["updated_at"].(time.Time); ok {
		rec.UpdatedAt = t
	}
	return rec
}
