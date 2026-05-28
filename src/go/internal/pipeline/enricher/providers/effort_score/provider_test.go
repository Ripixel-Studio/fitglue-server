package effort_score

import (
	user "github.com/fitglue/server/src/go/pkg/domain/user"

	pbuser "github.com/fitglue/server/src/go/pkg/types/pb/models/user"

	pbactivity "github.com/fitglue/server/src/go/pkg/types/pb/models/activity"

	"context"
	"log/slog"
	"testing"

	"github.com/fitglue/server/src/go/pkg/bootstrap"
	"github.com/fitglue/server/src/go/pkg/testing/mocks"
)

func makeActivity(durationMinutes int, heartRate int32, speed float64, altitude float64) *pbactivity.StandardizedActivity {
	var records []*pbactivity.Record
	for i := 0; i < durationMinutes; i++ {
		records = append(records, &pbactivity.Record{
			HeartRate: heartRate,
			Speed:     speed,
			Altitude:  altitude + float64(i)*0.5, // simulate gradual climb
		})
	}
	return &pbactivity.StandardizedActivity{
		Name: "Test Activity",
		Sessions: []*pbactivity.Session{
			{
				TotalElapsedTime: float64(durationMinutes * 60),
				Laps: []*pbactivity.Lap{
					{Records: records},
				},
			},
		},
	}
}

func makeHistoryData(count int, avgHR, avgPace, duration, elevGain, trimp float64) []interface{} {
	var entries []interface{}
	for i := 0; i < count; i++ {
		entries = append(entries, map[string]interface{}{
			"date":      "2026-02-01",
			"avg_hr":    avgHR,
			"avg_pace":  avgPace,
			"duration":  duration,
			"elev_gain": elevGain,
			"trimp":     trimp,
		})
	}
	return entries
}

func TestEffortScore_BasicCalculation(t *testing.T) {
	// History: 5 activities with avg HR 140, pace 5.0, 30min, 50m elev
	historyEntries := makeHistoryData(5, 140, 5.0, 30, 50, 60)

	var savedData map[string]interface{}
	mockDB := &mocks.MockDatabase{
		GetBoosterDataFunc: func(ctx context.Context, userId string, boosterId string) (map[string]interface{}, error) {
			return map[string]interface{}{
				"activities": historyEntries,
			}, nil
		},
		SetBoosterDataFunc: func(ctx context.Context, userId string, boosterId string, data map[string]interface{}) error {
			savedData = data
			return nil
		},
	}

	provider := NewEffortScore()
	provider.Service = &bootstrap.Service{DB: mockDB}

	// Activity at similar effort to history
	activity := makeActivity(30, 140, 3.33, 100) // 3.33 m/s ≈ 5 min/km
	user := &user.Record{UserProfile: &pbuser.UserProfile{UserId: "test-user"}}

	result, err := provider.Enrich(context.Background(), slog.Default(), activity, user, nil, false)
	if err != nil {
		t.Fatalf("Enrich failed: %v", err)
	}

	if result.Metadata["status"] != "success" {
		t.Errorf("Expected status success, got %s", result.Metadata["status"])
	}
	if result.Metadata["score"] == "" {
		t.Error("Expected non-empty score")
	}
	if result.Metadata["label"] == "" {
		t.Error("Expected non-empty label")
	}
	if result.Description == "" {
		t.Error("Expected non-empty description")
	}

	// Check history was persisted
	if savedData == nil {
		t.Fatal("Expected history to be saved")
	}
	activities, ok := savedData["activities"].([]interface{})
	if !ok {
		t.Fatal("Expected activities array in saved data")
	}
	// Should now have 6 entries (5 history + 1 current)
	if len(activities) != 6 {
		t.Errorf("Expected 6 history entries, got %d", len(activities))
	}
}

func TestEffortScore_NoHistory(t *testing.T) {
	mockDB := &mocks.MockDatabase{
		GetBoosterDataFunc: func(ctx context.Context, userId string, boosterId string) (map[string]interface{}, error) {
			return nil, nil // No history
		},
		SetBoosterDataFunc: func(ctx context.Context, userId string, boosterId string, data map[string]interface{}) error {
			return nil
		},
	}

	provider := NewEffortScore()
	provider.Service = &bootstrap.Service{DB: mockDB}

	activity := makeActivity(30, 150, 3.33, 100)
	user := &user.Record{UserProfile: &pbuser.UserProfile{UserId: "test-user"}}

	result, err := provider.Enrich(context.Background(), slog.Default(), activity, user, nil, false)
	if err != nil {
		t.Fatalf("Enrich failed: %v", err)
	}

	if result.Metadata["status"] != "skipped" {
		t.Errorf("Expected status skipped, got %s", result.Metadata["status"])
	}
	if result.Metadata["status_detail"] != "insufficient_history (0/3)" {
		t.Errorf("Expected insufficient_history detail, got %s", result.Metadata["status_detail"])
	}
}

func TestEffortScore_InsufficientHistory(t *testing.T) {
	// Only 2 entries — should skip (minimum is 3)
	historyEntries := makeHistoryData(2, 140, 5.0, 30, 50, 60)

	mockDB := &mocks.MockDatabase{
		GetBoosterDataFunc: func(ctx context.Context, userId string, boosterId string) (map[string]interface{}, error) {
			return map[string]interface{}{
				"activities": historyEntries,
			}, nil
		},
		SetBoosterDataFunc: func(ctx context.Context, userId string, boosterId string, data map[string]interface{}) error {
			return nil
		},
	}

	provider := NewEffortScore()
	provider.Service = &bootstrap.Service{DB: mockDB}

	activity := makeActivity(30, 150, 3.33, 100)
	user := &user.Record{UserProfile: &pbuser.UserProfile{UserId: "test-user"}}

	result, err := provider.Enrich(context.Background(), slog.Default(), activity, user, nil, false)
	if err != nil {
		t.Fatalf("Enrich failed: %v", err)
	}

	if result.Metadata["status"] != "skipped" {
		t.Errorf("Expected status skipped for 2 entries, got %s", result.Metadata["status"])
	}
}

func TestEffortScore_NoHRFallback(t *testing.T) {
	// History with HR data, but current activity has no HR
	historyEntries := makeHistoryData(5, 140, 5.0, 30, 50, 60)

	mockDB := &mocks.MockDatabase{
		GetBoosterDataFunc: func(ctx context.Context, userId string, boosterId string) (map[string]interface{}, error) {
			return map[string]interface{}{
				"activities": historyEntries,
			}, nil
		},
		SetBoosterDataFunc: func(ctx context.Context, userId string, boosterId string, data map[string]interface{}) error {
			return nil
		},
	}

	provider := NewEffortScore()
	provider.Service = &bootstrap.Service{DB: mockDB}

	// No HR (0 heart rate)
	activity := makeActivity(30, 0, 3.33, 100)
	user := &user.Record{UserProfile: &pbuser.UserProfile{UserId: "test-user"}}

	result, err := provider.Enrich(context.Background(), slog.Default(), activity, user, nil, false)
	if err != nil {
		t.Fatalf("Enrich failed: %v", err)
	}

	// Should still compute a score from pace/duration/elevation
	if result.Metadata["status"] != "success" {
		t.Errorf("Expected success even without HR, got %s", result.Metadata["status"])
	}
}

func TestEffortScore_HistoryTrimming(t *testing.T) {
	// Start with 14 entries (max)
	historyEntries := makeHistoryData(14, 140, 5.0, 30, 50, 60)

	var savedData map[string]interface{}
	mockDB := &mocks.MockDatabase{
		GetBoosterDataFunc: func(ctx context.Context, userId string, boosterId string) (map[string]interface{}, error) {
			return map[string]interface{}{
				"activities": historyEntries,
			}, nil
		},
		SetBoosterDataFunc: func(ctx context.Context, userId string, boosterId string, data map[string]interface{}) error {
			savedData = data
			return nil
		},
	}

	provider := NewEffortScore()
	provider.Service = &bootstrap.Service{DB: mockDB}

	activity := makeActivity(30, 150, 3.33, 100)
	user := &user.Record{UserProfile: &pbuser.UserProfile{UserId: "test-user"}}

	_, err := provider.Enrich(context.Background(), slog.Default(), activity, user, nil, false)
	if err != nil {
		t.Fatalf("Enrich failed: %v", err)
	}

	// After adding 1 to 14, should still be 14 (trimmed)
	activities, ok := savedData["activities"].([]interface{})
	if !ok {
		t.Fatal("Expected activities array")
	}
	if len(activities) != 14 {
		t.Errorf("Expected 14 entries after trimming, got %d", len(activities))
	}
}

func TestScoreLabels(t *testing.T) {
	tests := []struct {
		score float64
		want  string
	}{
		{0, "Easy"},
		{15, "Easy"},
		{30, "Easy"},
		{31, "Moderate"},
		{50, "Moderate"},
		{51, "Hard"},
		{70, "Hard"},
		{71, "Very Hard"},
		{85, "Very Hard"},
		{86, "All-Out"},
		{100, "All-Out"},
	}

	for _, tt := range tests {
		got := getScoreLabel(tt.score)
		if got != tt.want {
			t.Errorf("getScoreLabel(%.0f) = %q, want %q", tt.score, got, tt.want)
		}
	}
}

func TestComputeEffortScore_NormalEffort(t *testing.T) {
	// Activity exactly matching averages → score ≈ 50
	current := activitySnapshot{
		AvgHR:    140,
		AvgPace:  5.0,
		Duration: 30,
		ElevGain: 50,
		TRIMP:    60,
	}

	score, factors := computeEffortScore(current, 140, 5.0, 30, 50, 60)

	if score != 50 {
		t.Errorf("Expected score 50 for identical metrics, got %.0f", score)
	}
	if len(factors) != 5 {
		t.Errorf("Expected 5 factors, got %d", len(factors))
	}
}

func TestComputeEffortScore_HarderEffort(t *testing.T) {
	// Activity significantly harder than averages
	current := activitySnapshot{
		AvgHR:    170, // much higher than avg 140
		AvgPace:  4.0, // faster than avg 5.0
		Duration: 60,  // longer than avg 30
		ElevGain: 100, // more than avg 50
		TRIMP:    120, // higher than avg 60
	}

	score, _ := computeEffortScore(current, 140, 5.0, 30, 50, 60)

	if score <= 70 {
		t.Errorf("Expected score > 70 for harder effort, got %.0f", score)
	}
}

func TestEffortScore_SameSourceDedup(t *testing.T) {
	historyEntries := makeHistoryData(5, 140, 5.0, 30, 50, 60)

	var saveCount int
	mockDB := &mocks.MockDatabase{
		GetBoosterDataFunc: func(ctx context.Context, userId string, boosterId string) (map[string]interface{}, error) {
			return map[string]interface{}{
				"activities":              historyEntries,
				"last_external_id":        "activity-abc",
				"last_result_description": "💥 Effort Score: 50/100 (Moderate)\n• ➡️ Typical effort",
				"last_result_metadata": map[string]interface{}{
					"status": "success",
					"score":  "50",
					"label":  "Moderate",
				},
			}, nil
		},
		SetBoosterDataFunc: func(ctx context.Context, userId string, boosterId string, data map[string]interface{}) error {
			saveCount++
			return nil
		},
	}

	provider := NewEffortScore()
	provider.Service = &bootstrap.Service{DB: mockDB}

	activity := makeActivity(30, 140, 3.33, 100)
	u := &user.Record{UserProfile: &pbuser.UserProfile{UserId: "test-user"}}
	inputs := map[string]string{"external_id": "activity-abc"}

	result, err := provider.Enrich(context.Background(), slog.Default(), activity, u, inputs, false)
	if err != nil {
		t.Fatalf("Enrich failed: %v", err)
	}

	if result.Metadata["dedup"] != "true" {
		t.Error("Expected dedup=true for same-source activity")
	}
	if saveCount != 0 {
		t.Errorf("Expected no Firestore writes on dedup, got %d", saveCount)
	}
}

func TestComputeEffortScore_EasierEffort(t *testing.T) {
	// Activity easier than averages
	current := activitySnapshot{
		AvgHR:    110, // lower than avg 140
		AvgPace:  7.0, // slower than avg 5.0
		Duration: 15,  // shorter than avg 30
		ElevGain: 20,  // less than avg 50
		TRIMP:    20,  // lower than avg 60
	}

	score, _ := computeEffortScore(current, 140, 5.0, 30, 50, 60)

	if score >= 40 {
		t.Errorf("Expected score < 40 for easier effort, got %.0f", score)
	}
}
