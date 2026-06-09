package server

import (
	"context"

	"cloud.google.com/go/firestore"
	firestorepb "cloud.google.com/go/firestore/apiv1/firestorepb"
	"github.com/fitglue/server/src/go/internal/infra"
)

// PlatformStats holds marketing stats derived from Firestore aggregation queries.
type PlatformStats struct {
	AthleteCount           int64 `json:"athleteCount"`
	ActivitiesBoostedCount int64 `json:"activitiesBoostedCount"`
}

// PlatformStatsStore retrieves platform-wide marketing stats.
type PlatformStatsStore interface {
	GetPlatformStats(ctx context.Context) (*PlatformStats, error)
}

// FirestorePlatformStatsStore queries Firestore aggregates for live counts.
type FirestorePlatformStatsStore struct {
	client *firestore.Client
	logger infra.Logger
}

func NewFirestorePlatformStatsStore(client *firestore.Client, logger infra.Logger) *FirestorePlatformStatsStore {
	return &FirestorePlatformStatsStore{client: client, logger: logger}
}

// GetPlatformStats counts users and boosted activities via Firestore aggregation queries.
// Partial failures are logged and ignored — the calling handler falls back to zeros.
// Activities boosted = pipeline_runs with status SYNCED (2) or SYNCED_WITH_PENDING (10),
// counted via two separate queries because Firestore aggregation doesn't support OR filters.
func (s *FirestorePlatformStatsStore) GetPlatformStats(ctx context.Context) (*PlatformStats, error) {
	stats := &PlatformStats{}

	if result, err := s.client.Collection("users").NewAggregationQuery().WithCount("total").Get(ctx); err != nil {
		s.logger.Warn(ctx, "platform_stats: failed to count users", "error", err)
	} else if pbVal, ok := result["total"].(*firestorepb.Value); ok {
		stats.AthleteCount = pbVal.GetIntegerValue()
	}

	// PIPELINE_RUN_STATUS_SYNCED = 2, PIPELINE_RUN_STATUS_SYNCED_WITH_PENDING = 10
	for _, statusVal := range []int32{2, 10} {
		q := s.client.CollectionGroup("pipeline_runs").Where("status", "==", statusVal)
		result, err := q.NewAggregationQuery().WithCount("total").Get(ctx)
		if err != nil {
			s.logger.Warn(ctx, "platform_stats: failed to count pipeline_runs", "status", statusVal, "error", err)
			continue
		}
		if pbVal, ok := result["total"].(*firestorepb.Value); ok {
			stats.ActivitiesBoostedCount += pbVal.GetIntegerValue()
		}
	}

	return stats, nil
}
