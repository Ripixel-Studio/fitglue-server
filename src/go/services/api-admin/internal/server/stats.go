package server

import (
	"net/http"
	"strings"
	"time"

	"google.golang.org/api/iterator"

	pipelinemodel "github.com/fitglue/server/src/go/pkg/types/pb/models/pipeline"
)

func (s *APIServer) handleGetStats(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	// Count users by querying Firestore directly. User profiles are written with
	// protojson UseProtoNames=true, so fields are snake_case and the tier enum is
	// stored as its full name ("USER_TIER_ATHLETE"). We also tolerate legacy
	// camelCase/short-string values written by the old TypeScript system.
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

		if isAthleteTier(data["tier"]) {
			athleteUsers++
		}
		if asBool(data["is_admin"]) || asBool(data["isAdmin"]) {
			adminUsers++
		}
		totalSyncs += firstInt(data, "sync_count_this_month", "syncCountThisMonth")
	}

	// Count recent pipeline runs (last 24h) by status. created_at is stored as an
	// RFC3339 string (protojson serialises timestamps to strings), so compare
	// lexicographically against an RFC3339 cutoff — this sorts chronologically.
	cutoff := time.Now().Add(-24 * time.Hour).UTC().Format(time.RFC3339)
	successCount := 0
	failedCount := 0
	startedCount := 0

	runIter := s.firestoreClient.CollectionGroup("pipeline_runs").
		Where("created_at", ">=", cutoff).
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

		switch classifyRunStatus(doc.Data()["status"]) {
		case runStatusSuccess:
			successCount++
		case runStatusFailed:
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

// isAthleteTier reports whether a stored tier value represents the Athlete tier,
// tolerating the proto enum name and legacy short strings.
func isAthleteTier(v interface{}) bool {
	s, ok := v.(string)
	if !ok {
		return false
	}
	switch strings.ToUpper(s) {
	case "USER_TIER_ATHLETE", "ATHLETE":
		return true
	}
	return false
}

func asBool(v interface{}) bool {
	b, _ := v.(bool)
	return b
}

func asInt(v interface{}) int {
	switch n := v.(type) {
	case int64:
		return int(n)
	case int:
		return n
	case float64:
		return int(n)
	}
	return 0
}

// firstInt returns the first non-zero integer found under the given keys.
func firstInt(data map[string]interface{}, keys ...string) int {
	for _, k := range keys {
		if v, ok := data[k]; ok {
			if n := asInt(v); n != 0 {
				return n
			}
		}
	}
	return 0
}

const (
	runStatusSuccess = "success"
	runStatusFailed  = "failed"
	runStatusStarted = "started"
)

// classifyRunStatus buckets a stored pipeline run status (enum name, legacy
// string, or numeric enum value) into success / failed / started.
func classifyRunStatus(v interface{}) string {
	var name string
	switch t := v.(type) {
	case string:
		name = strings.ToUpper(t)
	case int64:
		name = pipelinemodel.PipelineRunStatus_name[int32(t)]
	case float64:
		name = pipelinemodel.PipelineRunStatus_name[int32(t)]
	}
	switch name {
	case "PIPELINE_RUN_STATUS_SYNCED", "PIPELINE_RUN_STATUS_SYNCED_WITH_PENDING", "SYNCED", "COMPLETED":
		return runStatusSuccess
	case "PIPELINE_RUN_STATUS_FAILED", "FAILED", "ERROR":
		return runStatusFailed
	}
	return runStatusStarted
}
