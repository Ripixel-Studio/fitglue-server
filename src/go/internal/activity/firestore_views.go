package activity

import (
	"context"
	"time"

	"cloud.google.com/go/firestore"
	pbactivity "github.com/fitglue/server/src/go/pkg/types/pb/models/activity"
	"google.golang.org/api/iterator"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/timestamppb"
)

const (
	viewStatsCollection = "showcase_view_stats"
	viewDedupCollection = "showcase_view_dedup"
	viewDailyCollection = "daily"
	// dedupTTL bounds how long a daily-salted visitor hash is remembered. The
	// hash already rotates daily, so 48h comfortably covers UTC day boundaries.
	dedupTTL = 48 * time.Hour
	// dailyHistoryLimit caps how many days of timeseries are returned for sparklines.
	dailyHistoryLimit = 30
)

// RecordShowcaseView transactionally increments view metrics for targetKey.
// Total views increment on every call; unique visitors increment only when
// visitorHash has not been seen before (the hash already encodes the day via a
// rotating salt, so a marker's existence means "seen today"). A dedup marker
// with a TTL field is written on first sight so Firestore can expire it.
func (s *FirestoreStore) RecordShowcaseView(ctx context.Context, targetKey, visitorHash string) error {
	if targetKey == "" {
		return status.Error(codes.InvalidArgument, "targetKey is required")
	}

	now := time.Now().UTC()
	day := now.Format("2006-01-02")
	statsRef := s.client.Collection(viewStatsCollection).Doc(targetKey)
	dailyRef := statsRef.Collection(viewDailyCollection).Doc(day)
	var dedupRef *firestore.DocumentRef
	if visitorHash != "" {
		dedupRef = s.client.Collection(viewDedupCollection).Doc(visitorHash)
	}

	return s.client.RunTransaction(ctx, func(ctx context.Context, tx *firestore.Transaction) error {
		// Firestore requires all reads before any writes.
		isNewVisitor := false
		if dedupRef != nil {
			_, err := tx.Get(dedupRef)
			if err != nil {
				if status.Code(err) == codes.NotFound {
					isNewVisitor = true
				} else {
					return err
				}
			}
		}

		statsUpdate := map[string]interface{}{
			"target_key":     targetKey,
			"views":          firestore.Increment(1),
			"last_viewed_at": now,
			"updated_at":     now,
		}
		dailyUpdate := map[string]interface{}{
			"day":   day,
			"views": firestore.Increment(1),
		}
		if isNewVisitor {
			statsUpdate["visitors"] = firestore.Increment(1)
			dailyUpdate["visitors"] = firestore.Increment(1)
		}

		if err := tx.Set(statsRef, statsUpdate, firestore.MergeAll); err != nil {
			return err
		}
		if err := tx.Set(dailyRef, dailyUpdate, firestore.MergeAll); err != nil {
			return err
		}
		if isNewVisitor {
			if err := tx.Set(dedupRef, map[string]interface{}{"expires_at": now.Add(dedupTTL)}); err != nil {
				return err
			}
		}
		return nil
	})
}

// GetShowcaseViewStats reads aggregate counters plus a recent daily timeseries
// for targetKey. A target that has never been viewed returns zero-valued stats
// rather than an error.
func (s *FirestoreStore) GetShowcaseViewStats(ctx context.Context, targetKey string) (*pbactivity.ShowcaseViewStats, error) {
	if targetKey == "" {
		return nil, status.Error(codes.InvalidArgument, "targetKey is required")
	}

	stats := &pbactivity.ShowcaseViewStats{TargetKey: targetKey}

	doc, err := s.client.Collection(viewStatsCollection).Doc(targetKey).Get(ctx)
	if err != nil {
		if status.Code(err) != codes.NotFound {
			return nil, err
		}
		// Never viewed — return zeroes.
		return stats, nil
	}

	data := doc.Data()
	stats.Views = toInt64(data["views"])
	stats.Visitors = toInt64(data["visitors"])
	if t, ok := data["last_viewed_at"].(time.Time); ok {
		stats.LastViewedAt = timestamppb.New(t)
	}

	daily, err := s.listDailyViewCounts(ctx, targetKey)
	if err != nil {
		return nil, err
	}
	stats.Daily = daily

	return stats, nil
}

// listDailyViewCounts returns up to dailyHistoryLimit days of counts, ordered
// most-recent-last so callers can render a left-to-right sparkline.
func (s *FirestoreStore) listDailyViewCounts(ctx context.Context, targetKey string) ([]*pbactivity.ShowcaseDailyViewCount, error) {
	iter := s.client.Collection(viewStatsCollection).Doc(targetKey).
		Collection(viewDailyCollection).
		OrderBy("day", firestore.Desc).
		Limit(dailyHistoryLimit).
		Documents(ctx)
	defer iter.Stop()

	var rows []*pbactivity.ShowcaseDailyViewCount
	for {
		d, err := iter.Next()
		if err == iterator.Done {
			break
		}
		if err != nil {
			return nil, err
		}
		data := d.Data()
		rows = append(rows, &pbactivity.ShowcaseDailyViewCount{
			Day:      d.Ref.ID,
			Views:    toInt64(data["views"]),
			Visitors: toInt64(data["visitors"]),
		})
	}

	// Reverse to most-recent-last.
	for i, j := 0, len(rows)-1; i < j; i, j = i+1, j-1 {
		rows[i], rows[j] = rows[j], rows[i]
	}
	return rows, nil
}

// toInt64 coerces a Firestore numeric field (stored as int64) to int64.
func toInt64(v interface{}) int64 {
	switch n := v.(type) {
	case int64:
		return n
	case int:
		return int64(n)
	case float64:
		return int64(n)
	default:
		return 0
	}
}
