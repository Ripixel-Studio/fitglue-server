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
func (s *FirestorePlatformStatsStore) GetPlatformStats(ctx context.Context) (*PlatformStats, error) {
	stats := &PlatformStats{}

	if result, err := s.client.Collection("users").NewAggregationQuery().WithCount("total").Get(ctx); err != nil {
		s.logger.Warn(ctx, "platform_stats: failed to count users", "error", err)
	} else if pbVal, ok := result["total"].(*firestorepb.Value); ok {
		stats.AthleteCount = pbVal.GetIntegerValue()
	}

	if result, err := s.client.CollectionGroup("activities").NewAggregationQuery().WithCount("total").Get(ctx); err != nil {
		s.logger.Warn(ctx, "platform_stats: failed to count activities", "error", err)
	} else if pbVal, ok := result["total"].(*firestorepb.Value); ok {
		stats.ActivitiesBoostedCount = pbVal.GetIntegerValue()
	}

	return stats, nil
}
