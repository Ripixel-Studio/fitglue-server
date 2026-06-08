package server

import (
	"context"

	"cloud.google.com/go/firestore"
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
}

func NewFirestorePlatformStatsStore(client *firestore.Client) *FirestorePlatformStatsStore {
	return &FirestorePlatformStatsStore{client: client}
}

// GetPlatformStats counts users and boosted activities via Firestore aggregation queries.
// Partial failures are silently ignored — the calling handler falls back to zeros.
func (s *FirestorePlatformStatsStore) GetPlatformStats(ctx context.Context) (*PlatformStats, error) {
	stats := &PlatformStats{}

	if result, err := s.client.Collection("users").NewAggregationQuery().WithCount("total").Get(ctx); err == nil {
		if v, ok := result["total"].(int64); ok {
			stats.AthleteCount = v
		}
	}

	if result, err := s.client.CollectionGroup("activities").NewAggregationQuery().WithCount("total").Get(ctx); err == nil {
		if v, ok := result["total"].(int64); ok {
			stats.ActivitiesBoostedCount = v
		}
	}

	return stats, nil
}
