package loopprevention

import (
	"context"
	"errors"
	"fmt"
	"testing"

	pbactivity "github.com/fitglue/server/src/go/pkg/types/pb/models/activity"
	pbplugin "github.com/fitglue/server/src/go/pkg/types/pb/models/plugin"
)

// mockStore is a simple test implementation of UploadedActivityStore.
type mockStore struct {
	records map[string]*pbactivity.UploadedActivityRecord
	err     error
}

func newMockStore(record *pbactivity.UploadedActivityRecord) *mockStore {
	m := &mockStore{records: make(map[string]*pbactivity.UploadedActivityRecord)}
	if record != nil {
		key := BuildUploadedActivityID(record.Destination, record.DestinationId)
		m.records[key] = record
	}
	return m
}

func (m *mockStore) SetUploadedActivity(_ context.Context, _ string, record *pbactivity.UploadedActivityRecord) error {
	return m.err
}

func (m *mockStore) GetUploadedActivity(_ context.Context, _ string, dest pbplugin.DestinationType, destId string) (*pbactivity.UploadedActivityRecord, error) {
	if m.err != nil {
		return nil, m.err
	}
	key := BuildUploadedActivityID(dest, destId)
	if r, ok := m.records[key]; ok {
		return r, nil
	}
	return nil, nil
}

// --- IsBounceback ---

func TestIsBounceback_SourceWithNoDestination(t *testing.T) {
	// SOURCE_PARKRUN_RESULTS is not in the SourceToDestinationMap
	store := newMockStore(nil)
	isBounceback, err := IsBounceback(context.Background(), store, "user1",
		pbactivity.ActivitySource_SOURCE_PARKRUN_RESULTS, "any_id", 0)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if isBounceback {
		t.Error("expected false for source with no destination")
	}
}

func TestIsBounceback_NotABounceback(t *testing.T) {
	// Hevy source but no record of uploading this activity
	store := newMockStore(nil)
	isBounceback, err := IsBounceback(context.Background(), store, "user1",
		pbactivity.ActivitySource_SOURCE_HEVY, "hevy_workout_999", 0)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if isBounceback {
		t.Error("expected false when no upload record exists")
	}
}

func TestIsBounceback_IsBounceback(t *testing.T) {
	// We have a record for this activity being uploaded to Hevy
	record := &pbactivity.UploadedActivityRecord{
		Destination:   pbplugin.DestinationType_DESTINATION_HEVY,
		DestinationId: "hevy_workout_123",
	}
	store := newMockStore(record)
	isBounceback, err := IsBounceback(context.Background(), store, "user1",
		pbactivity.ActivitySource_SOURCE_HEVY, "hevy_workout_123", 0)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !isBounceback {
		t.Error("expected true when an upload record exists for this activity")
	}
}

func TestIsBounceback_StoreError(t *testing.T) {
	// Store returns an error
	store := newMockStore(nil)
	store.err = errors.New("firestore unavailable")
	isBounceback, err := IsBounceback(context.Background(), store, "user1",
		pbactivity.ActivitySource_SOURCE_HEVY, "hevy_workout_456", 0)
	if err == nil {
		t.Error("expected error when store fails")
	}
	if isBounceback {
		t.Error("expected false on store error (fail open)")
	}
}

func TestIsBounceback_StravaBBounceback(t *testing.T) {
	record := &pbactivity.UploadedActivityRecord{
		Destination:   pbplugin.DestinationType_DESTINATION_STRAVA,
		DestinationId: "strava_act_42",
	}
	store := newMockStore(record)
	isBounceback, err := IsBounceback(context.Background(), store, "user1",
		pbactivity.ActivitySource_SOURCE_STRAVA, "strava_act_42", 0)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !isBounceback {
		t.Error("expected true for bounceback from Strava")
	}
}

func TestIsBounceback_PendingPreRecord(t *testing.T) {
	// Simulates the Create race condition: the pending record exists (startTime-keyed)
	// but the real Hevy workout ID hasn't been written yet.
	const startTimeUnix = int64(1748599314)
	pendingId := fmt.Sprintf("pending:%d", startTimeUnix)
	record := &pbactivity.UploadedActivityRecord{
		Destination:   pbplugin.DestinationType_DESTINATION_HEVY,
		DestinationId: pendingId,
	}
	store := newMockStore(record)
	// ExternalId is the (not yet known) Hevy workout ID — not in the store
	isBounceback, err := IsBounceback(context.Background(), store, "user1",
		pbactivity.ActivitySource_SOURCE_HEVY, "hevy_workout_unknown", startTimeUnix)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !isBounceback {
		t.Error("expected true: pending pre-record should catch the race condition")
	}
}

func TestIsBounceback_PendingPreRecord_ZeroStartTime(t *testing.T) {
	// startTimeUnix=0 disables the secondary pending check — no false positives.
	const startTimeUnix = int64(1748599314)
	pendingId := fmt.Sprintf("pending:%d", startTimeUnix)
	record := &pbactivity.UploadedActivityRecord{
		Destination:   pbplugin.DestinationType_DESTINATION_HEVY,
		DestinationId: pendingId,
	}
	store := newMockStore(record)
	isBounceback, err := IsBounceback(context.Background(), store, "user1",
		pbactivity.ActivitySource_SOURCE_HEVY, "hevy_workout_unknown", 0)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if isBounceback {
		t.Error("expected false: secondary check should be skipped when startTimeUnix=0")
	}
}
