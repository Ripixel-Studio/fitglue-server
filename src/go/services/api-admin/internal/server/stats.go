package server

import (
	"net/http"
	"time"

	"google.golang.org/api/iterator"
)

func (s *APIServer) handleGetStats(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	// Count users by querying Firestore directly
	totalUsers := 0
	athleteUsers := 0
	adminUsers := 0
	totalSyncs := 0

	iter := s.firestoreClient.Collection("users").Documents(ctx)
	for {
		doc, err := iter.Next()
		if err == iterator.Done {
			break
		}
		if err != nil {
			s.logger.Error(ctx, "failed to iterate users for stats", "error", err)
			WriteError(w, err)
			return
		}

		totalUsers++
		data := doc.Data()

		if tier, ok := data["tier"].(string); ok && tier == "ATHLETE" {
			athleteUsers++
		}
		if isAdmin, ok := data["is_admin"].(bool); ok && isAdmin {
			adminUsers++
		}
		if syncCount, ok := data["syncCountThisMonth"].(int64); ok {
			totalSyncs += int(syncCount)
		}
	}

	// Count recent pipeline runs (last 24h) by status
	cutoff := time.Now().Add(-24 * time.Hour)
	successCount := 0
	failedCount := 0
	startedCount := 0

	runIter := s.firestoreClient.CollectionGroup("pipeline_runs").
		Where("createdAt", ">=", cutoff).
		Documents(ctx)
	for {
		doc, err := runIter.Next()
		if err == iterator.Done {
			break
		}
		if err != nil {
			s.logger.Error(ctx, "failed to iterate pipeline runs for stats", "error", err)
			break // non-fatal, return partial stats
		}

		data := doc.Data()
		status, _ := data["status"].(string)
		switch status {
		case "COMPLETED":
			successCount++
		case "FAILED", "ERROR":
			failedCount++
		default:
			startedCount++
		}
	}

	WriteJSON(w, map[string]interface{}{
		"totalUsers":          totalUsers,
		"athleteUsers":        athleteUsers,
		"adminUsers":          adminUsers,
		"totalSyncsThisMonth": totalSyncs,
		"recentExecutions": map[string]int{
			"success": successCount,
			"failed":  failedCount,
			"started": startedCount,
		},
	})
}
