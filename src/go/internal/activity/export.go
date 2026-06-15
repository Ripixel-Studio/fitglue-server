package activity

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	pbsvc "github.com/fitglue/server/src/go/pkg/types/pb/services/activity"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/encoding/protojson"
)

// ExportData generates a privacy compliance data export for the user.
// It aggregates all activity-domain data (pipeline runs + showcased activities),
// writes a JSON file to GCS, and returns a signed download URL.
func (s *Service) ExportData(ctx context.Context, req *pbsvc.ExportDataRequest) (*pbsvc.ExportDataResponse, error) {
	if req.UserId == "" {
		return nil, status.Error(codes.InvalidArgument, "user_id is required")
	}

	marshaller := protojson.MarshalOptions{EmitUnpopulated: true}

	// 1. Aggregate pipeline runs (paginate until exhausted)
	var allRunsJSON []json.RawMessage
	pageToken := ""
	for {
		runs, nextToken, err := s.store.ListPipelineRuns(ctx, req.UserId, 100, pageToken)
		if err != nil {
			s.logger.Error(ctx, "ExportData: failed to list pipeline runs", "userId", req.UserId, "error", err)
			return nil, status.Error(codes.Internal, "failed to list pipeline runs")
		}
		for _, run := range runs {
			b, err := marshaller.Marshal(run)
			if err != nil {
				s.logger.Error(ctx, "ExportData: failed to marshal pipeline run", "error", err)
				return nil, status.Error(codes.Internal, "failed to serialize pipeline run")
			}
			allRunsJSON = append(allRunsJSON, b)
		}
		if nextToken == "" {
			break
		}
		pageToken = nextToken
	}

	// 2. Aggregate showcased activities
	var allShowcasesJSON []json.RawMessage
	showcases, _, err := s.store.ListShowcasedActivitiesByUser(ctx, req.UserId, 0, 0)
	if err != nil {
		s.logger.Error(ctx, "ExportData: failed to list showcased activities", "userId", req.UserId, "error", err)
		return nil, status.Error(codes.Internal, "failed to list showcased activities")
	}
	for _, sc := range showcases {
		b, err := marshaller.Marshal(sc)
		if err != nil {
			s.logger.Error(ctx, "ExportData: failed to marshal showcase", "error", err)
			return nil, status.Error(codes.Internal, "failed to serialize showcase")
		}
		allShowcasesJSON = append(allShowcasesJSON, b)
	}

	// 3. Build the export envelope
	export := map[string]interface{}{
		"userId":              req.UserId,
		"exportedAt":          time.Now().UTC().Format(time.RFC3339),
		"pipelineRuns":        allRunsJSON,
		"showcasedActivities": allShowcasesJSON,
	}

	data, err := json.MarshalIndent(export, "", "  ")
	if err != nil {
		s.logger.Error(ctx, "ExportData: failed to marshal export JSON", "error", err)
		return nil, status.Error(codes.Internal, "failed to build export file")
	}

	// 4. Write to GCS
	objectPath := fmt.Sprintf("exports/%s/%d.json", req.UserId, time.Now().UnixMilli())
	if err := s.blobStore.Write(ctx, s.bucketName, objectPath, data); err != nil {
		s.logger.Error(ctx, "ExportData: failed to write export to GCS", "error", err, "path", objectPath)
		return nil, status.Error(codes.Internal, "failed to write export file")
	}

	// 5. Generate signed download URL (24-hour expiry)
	signedURL, err := s.blobStore.SignedURL(ctx, s.bucketName, objectPath, "", 0, 24*time.Hour)
	if err != nil {
		s.logger.Error(ctx, "ExportData: failed to generate signed URL", "error", err, "path", objectPath)
		return nil, status.Error(codes.Internal, "failed to generate download link")
	}

	s.logger.Info(ctx, "ExportData completed", "userId", req.UserId, "path", objectPath)
	return &pbsvc.ExportDataResponse{DownloadUrl: signedURL}, nil
}
