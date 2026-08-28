package recovery_advisor

import (
	user "github.com/fitglue/server/src/go/pkg/domain/user"

	pbuser "github.com/fitglue/server/src/go/pkg/types/pb/models/user"

	pbactivity "github.com/fitglue/server/src/go/pkg/types/pb/models/activity"

	"context"
	"fmt"
	"log/slog"
	"strings"
	"testing"
	"time"

	"github.com/fitglue/server/src/go/pkg/bootstrap"
	"github.com/fitglue/server/src/go/pkg/testing/mocks"
	"google.golang.org/protobuf/types/known/timestamppb"
)

func makeActivity(durationMinutes int, heartRate int32) *pbactivity.StandardizedActivity {
	var records []*pbactivity.Record
	for i := 0; i < durationMinutes; i++ {
		records = append(records, &pbactivity.Record{
			HeartRate: heartRate,
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

func TestRecoveryAdvisor_SingleActivity(t *testing.T) {
	provider := NewRecoveryAdvisor()
	provider.Service = &bootstrap.Service{}

	activity := makeActivity(30, 150)
	user := &user.Record{UserProfile: &pbuser.UserProfile{UserId: "test-user"}}

	result, err := provider.Enrich(context.Background(), slog.Default(), activity, user, nil, false)
	if err != nil {
		t.Fatalf("Enrich failed: %v", err)
	}

	if !strings.Contains(result.Description, "💤 Recovery Advisor") {
		t.Errorf("Missing header in description: %s", result.Description)
	}
	if !strings.Contains(result.Description, "Session load:") {
		t.Errorf("Missing session load in description: %s", result.Description)
	}
	if !strings.Contains(result.Description, "7-day load:") {
		t.Errorf("Missing 7-day load in description: %s", result.Description)
	}
	if !strings.Contains(result.Description, "28-day avg:") {
		t.Errorf("Missing 28-day avg in description: %s", result.Description)
	}
	if !strings.Contains(result.Description, "Suggested recovery:") {
		t.Errorf("Missing recovery suggestion in description: %s", result.Description)
	}
	if result.Metadata["trimp"] == "" || result.Metadata["trimp"] == "0" {
		t.Errorf("Expected non-zero trimp, got %s", result.Metadata["trimp"])
	}
	if result.Metadata["acwr"] == "" {
		t.Errorf("Expected acwr in metadata")
	}
}

func TestRecoveryAdvisor_MultiActivitySameDay(t *testing.T) {
	today := time.Now().Format("2006-01-02")
	existingTrimp := 80.0

	mockDB := &mocks.MockDatabase{
		GetBoosterDataFunc: func(ctx context.Context, userId string, boosterId string) (map[string]interface{}, error) {
			return map[string]interface{}{
				today:         existingTrimp,
				"last_update": time.Now().Format(time.RFC3339),
			}, nil
		},
		SetBoosterDataFunc: func(ctx context.Context, userId string, boosterId string, data map[string]interface{}) error {
			savedLoad, ok := data[today].(float64)
			if !ok {
				t.Errorf("Expected today's load to be float64, got %T", data[today])
				return nil
			}
			// The saved load must be greater than the existing TRIMP (accumulated)
			if savedLoad <= existingTrimp {
				t.Errorf("Expected accumulated load > %.0f, got %.0f (overwrite bug!)", existingTrimp, savedLoad)
			}
			return nil
		},
	}

	provider := NewRecoveryAdvisor()
	provider.Service = &bootstrap.Service{DB: mockDB}

	// Second activity of the day: 20 min at 140 bpm
	activity := makeActivity(20, 140)
	user := &user.Record{UserProfile: &pbuser.UserProfile{UserId: "test-user"}}

	result, err := provider.Enrich(context.Background(), slog.Default(), activity, user, nil, false)
	if err != nil {
		t.Fatalf("Enrich failed: %v", err)
	}

	// The acute load should include the existing TRIMP for today
	acuteLoad := result.Metadata["acute_load"]
	if acuteLoad == "" {
		t.Fatal("Expected acute_load in metadata")
	}
	var acute float64
	fmt.Sscanf(acuteLoad, "%f", &acute)
	if acute <= existingTrimp {
		t.Errorf("Acute load %.0f should be > existing today TRIMP %.0f", acute, existingTrimp)
	}
}

func TestRecoveryAdvisor_NoHeartRate(t *testing.T) {
	provider := NewRecoveryAdvisor()
	provider.Service = &bootstrap.Service{}

	// Activity with no HR data - should fall back to duration-based estimation
	activity := &pbactivity.StandardizedActivity{
		Name: "No HR Activity",
		Sessions: []*pbactivity.Session{
			{
				TotalElapsedTime: 1800, // 30 minutes
				Laps: []*pbactivity.Lap{
					{Records: []*pbactivity.Record{{HeartRate: 0}}},
				},
			},
		},
	}

	result, err := provider.Enrich(context.Background(), slog.Default(), activity, &user.Record{UserProfile: &pbuser.UserProfile{UserId: "test"}}, nil, false)
	if err != nil {
		t.Fatalf("Enrich failed: %v", err)
	}

	// Duration-based: 30 min * 0.5 = 15 TRIMP
	if result.Metadata["trimp"] != "15" {
		t.Errorf("Expected trimp 15 for no-HR fallback, got %s", result.Metadata["trimp"])
	}
	if result.Metadata["intensity"] != "Recovery" {
		t.Errorf("Expected Recovery intensity, got %s", result.Metadata["intensity"])
	}
}

func TestRecoveryAdvisor_HighWeeklyLoad(t *testing.T) {
	now := time.Now()

	mockDB := &mocks.MockDatabase{
		GetBoosterDataFunc: func(ctx context.Context, userId string, boosterId string) (map[string]interface{}, error) {
			// Simulate heavy training week: ~100 TRIMP per day for 6 days = 600
			data := map[string]interface{}{}
			for i := 1; i <= 6; i++ {
				dateKey := now.AddDate(0, 0, -i).Format("2006-01-02")
				data[dateKey] = 100.0
			}
			return data, nil
		},
		SetBoosterDataFunc: func(ctx context.Context, userId string, boosterId string, data map[string]interface{}) error {
			return nil
		},
	}

	provider := NewRecoveryAdvisor()
	provider.Service = &bootstrap.Service{DB: mockDB}

	// Hard session: 60 min at 170 bpm
	activity := makeActivity(60, 170)
	user := &user.Record{UserProfile: &pbuser.UserProfile{UserId: "test-user"}}

	result, err := provider.Enrich(context.Background(), slog.Default(), activity, user, nil, false)
	if err != nil {
		t.Fatalf("Enrich failed: %v", err)
	}

	// Acute load should be > 500
	var acute float64
	fmt.Sscanf(result.Metadata["acute_load"], "%f", &acute)
	if acute <= 500 {
		t.Errorf("Expected acute load > 500, got %.0f", acute)
	}

	// Recovery hours should include graduated load adjustment
	var hours float64
	fmt.Sscanf(result.Metadata["recovery_hours"], "%f", &hours)
	if hours < 48 {
		t.Errorf("Expected >= 48h recovery with high weekly load, got %.0f", hours)
	}
}

func TestRecoveryAdvisor_ACWROverreaching(t *testing.T) {
	now := time.Now()

	mockDB := &mocks.MockDatabase{
		GetBoosterDataFunc: func(ctx context.Context, userId string, boosterId string) (map[string]interface{}, error) {
			data := map[string]interface{}{}
			// Very heavy recent week: 150 TRIMP/day for days 1-6
			for i := 1; i <= 6; i++ {
				dateKey := now.AddDate(0, 0, -i).Format("2006-01-02")
				data[dateKey] = 150.0
			}
			// Light chronic base: 30 TRIMP/day for days 8-28
			for i := 8; i <= 28; i++ {
				dateKey := now.AddDate(0, 0, -i).Format("2006-01-02")
				data[dateKey] = 30.0
			}
			return data, nil
		},
		SetBoosterDataFunc: func(ctx context.Context, userId string, boosterId string, data map[string]interface{}) error {
			return nil
		},
	}

	provider := NewRecoveryAdvisor()
	provider.Service = &bootstrap.Service{DB: mockDB}

	// Hard session today
	activity := makeActivity(60, 170)
	user := &user.Record{UserProfile: &pbuser.UserProfile{UserId: "test-user"}}

	result, err := provider.Enrich(context.Background(), slog.Default(), activity, user, nil, false)
	if err != nil {
		t.Fatalf("Enrich failed: %v", err)
	}

	// ACWR should be > 1.5 (overreaching)
	var acwr float64
	fmt.Sscanf(result.Metadata["acwr"], "%f", &acwr)
	if acwr <= 1.5 {
		t.Errorf("Expected ACWR > 1.5 (overreaching), got %.2f", acwr)
	}

	if result.Metadata["acwr_label"] != "Overreaching" {
		t.Errorf("Expected Overreaching label, got %s", result.Metadata["acwr_label"])
	}

	// Should contain overreaching warning in description
	if !strings.Contains(result.Description, "Overreaching") {
		t.Errorf("Expected Overreaching in description: %s", result.Description)
	}
}

func TestRecoveryAdvisor_ConsecutiveHardDays(t *testing.T) {
	now := time.Now()

	mockDB := &mocks.MockDatabase{
		GetBoosterDataFunc: func(ctx context.Context, userId string, boosterId string) (map[string]interface{}, error) {
			data := map[string]interface{}{}
			// 4 consecutive hard days before today (TRIMP > 60)
			for i := 1; i <= 4; i++ {
				dateKey := now.AddDate(0, 0, -i).Format("2006-01-02")
				data[dateKey] = 80.0
			}
			return data, nil
		},
		SetBoosterDataFunc: func(ctx context.Context, userId string, boosterId string, data map[string]interface{}) error {
			return nil
		},
	}

	provider := NewRecoveryAdvisor()
	provider.Service = &bootstrap.Service{DB: mockDB}

	// Another hard session today (TRIMP > 60)
	activity := makeActivity(60, 160)
	user := &user.Record{UserProfile: &pbuser.UserProfile{UserId: "test-user"}}

	result, err := provider.Enrich(context.Background(), slog.Default(), activity, user, nil, false)
	if err != nil {
		t.Fatalf("Enrich failed: %v", err)
	}

	// Should detect 5 consecutive hard days (4 prior + today)
	if result.Metadata["consecutive_hard_days"] != "5" {
		t.Errorf("Expected 5 consecutive hard days, got %s", result.Metadata["consecutive_hard_days"])
	}

	// Should contain fatigue warning
	if !strings.Contains(result.Description, "consecutive hard days") {
		t.Errorf("Expected fatigue warning in description: %s", result.Description)
	}
}

func TestRecoveryAdvisor_ConfigurableInputs(t *testing.T) {
	provider := NewRecoveryAdvisor()
	provider.Service = &bootstrap.Service{}

	activity := makeActivity(30, 150)
	user := &user.Record{UserProfile: &pbuser.UserProfile{UserId: "test-user"}}

	// Run with default inputs
	resultDefault, err := provider.Enrich(context.Background(), slog.Default(), activity, user, nil, false)
	if err != nil {
		t.Fatalf("Enrich failed: %v", err)
	}

	// Run with custom inputs (wider HR range = lower HRR = lower TRIMP)
	customInputs := map[string]string{
		"max_hr":  "210",
		"rest_hr": "40",
		"gender":  "female",
	}
	resultCustom, err := provider.Enrich(context.Background(), slog.Default(), activity, user, customInputs, false)
	if err != nil {
		t.Fatalf("Enrich with custom inputs failed: %v", err)
	}

	// Different HR range and gender coefficient should produce different TRIMP
	if resultDefault.Metadata["trimp"] == resultCustom.Metadata["trimp"] {
		t.Errorf("Expected different TRIMP with custom inputs, both got %s", resultDefault.Metadata["trimp"])
	}
}

func TestGetRecoveryRecommendation(t *testing.T) {
	tests := []struct {
		trimp               float64
		acuteLoad           float64
		acwr                float64
		consecutiveHardDays int
		wantMinHours        float64
		wantMaxHours        float64
		wantLabel           string
	}{
		// Base intensity tests (no load/ACWR adjustments)
		{15, 200, 1.0, 0, 8, 12, "Recovery"},
		{45, 200, 1.0, 0, 12, 16, "Easy"},
		{75, 200, 1.0, 0, 24, 28, "Moderate"},
		{120, 200, 1.0, 0, 36, 40, "Hard"},
		{200, 200, 1.0, 0, 48, 52, "Very Hard"},

		// Graduated load adjustments
		{15, 400, 1.0, 0, 12, 16, "Recovery"}, // +4 for 300-500 range
		{15, 600, 1.0, 0, 16, 20, "Recovery"}, // +8 for 500-700 range
		{15, 800, 1.0, 0, 20, 24, "Recovery"}, // +12 for 700+ range

		// ACWR adjustments
		{75, 200, 1.6, 0, 40, 44, "Moderate"}, // +16 for overreaching
		{75, 200, 1.3, 0, 32, 36, "Moderate"}, // +8 for building
		{75, 200, 0.5, 0, 20, 24, "Moderate"}, // -4 for detraining

		// Consecutive hard days
		{75, 200, 1.0, 3, 32, 36, "Moderate"}, // +8 for 3+ consecutive

		// Combined: high load + overreaching + consecutive
		{200, 800, 1.6, 4, 80, 88, "Very Hard"}, // 48 + 12 + 16 + 8 = 84
	}

	for _, tt := range tests {
		hours, intensity := getRecoveryRecommendation(tt.trimp, tt.acuteLoad, tt.acwr, tt.consecutiveHardDays)
		if hours < tt.wantMinHours || hours > tt.wantMaxHours {
			t.Errorf("trimp=%.0f acuteLoad=%.0f acwr=%.1f consecutiveHard=%d: got hours=%.0f, want %.0f-%.0f",
				tt.trimp, tt.acuteLoad, tt.acwr, tt.consecutiveHardDays, hours, tt.wantMinHours, tt.wantMaxHours)
		}
		if intensity != tt.wantLabel {
			t.Errorf("trimp=%.0f acuteLoad=%.0f acwr=%.1f: got intensity=%s, want %s",
				tt.trimp, tt.acuteLoad, tt.acwr, intensity, tt.wantLabel)
		}
	}
}

func TestGetACWRLabel(t *testing.T) {
	tests := []struct {
		acwr     float64
		expected string
	}{
		{1.8, "Overreaching"},
		{1.3, "Building"},
		{1.0, "Optimal"},
		{0.5, "Detraining"},
		{0.0, "No History"},
	}

	for _, tt := range tests {
		label := getACWRLabel(tt.acwr)
		if label != tt.expected {
			t.Errorf("getACWRLabel(%.1f) = %q, want %q", tt.acwr, label, tt.expected)
		}
	}
}

func TestFormatRecoveryTime(t *testing.T) {
	tests := []struct {
		hours    float64
		expected string
	}{
		{12, "12 hours"},
		{24, "24 hours"},
		{36, "36 hours"},
		{48, "48 hours"},
		{60, "60 hours"},
		{72, "72 hours"},
		{84, "84 hours"},
	}

	for _, tt := range tests {
		result := formatRecoveryTime(tt.hours)
		if result != tt.expected {
			t.Errorf("formatRecoveryTime(%.0f) = %q, want %q", tt.hours, result, tt.expected)
		}
	}
}

func TestCountConsecutiveHardDays(t *testing.T) {
	now := time.Now()

	tests := []struct {
		name      string
		todayLoad float64
		data      map[string]interface{}
		want      int
	}{
		{
			name:      "today not hard",
			todayLoad: 30,
			data:      nil,
			want:      0,
		},
		{
			name:      "today hard, no history",
			todayLoad: 80,
			data:      nil,
			want:      1,
		},
		{
			name:      "3 consecutive hard days",
			todayLoad: 80,
			data: map[string]interface{}{
				now.AddDate(0, 0, -1).Format("2006-01-02"): 70.0,
				now.AddDate(0, 0, -2).Format("2006-01-02"): 90.0,
				now.AddDate(0, 0, -3).Format("2006-01-02"): 20.0, // breaks streak
			},
			want: 3,
		},
		{
			name:      "gap breaks streak",
			todayLoad: 80,
			data: map[string]interface{}{
				now.AddDate(0, 0, -1).Format("2006-01-02"): 40.0, // not hard
				now.AddDate(0, 0, -2).Format("2006-01-02"): 90.0,
			},
			want: 1,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := countConsecutiveHardDays(tt.data, tt.todayLoad, now)
			if got != tt.want {
				t.Errorf("countConsecutiveHardDays() = %d, want %d", got, tt.want)
			}
		})
	}
}

// A backdated activity (history import, late upload) must land on its own date:
// the 7/28-day windows are anchored on the activity's start time, not on when
// the pipeline happened to run.
func TestRecoveryAdvisor_AnchorsOnActivityDate(t *testing.T) {
	activityDay := time.Now().AddDate(0, 0, -40).UTC().Truncate(24 * time.Hour)
	dayKey := activityDay.Format("2006-01-02")
	weekBefore := activityDay.AddDate(0, 0, -3).Format("2006-01-02")
	today := time.Now().Format("2006-01-02")

	var saved map[string]interface{}
	mockDB := &mocks.MockDatabase{
		GetBoosterDataFunc: func(ctx context.Context, userId string, boosterId string) (map[string]interface{}, error) {
			// 100 TRIMP three days before the activity, and a big load "today" that
			// must NOT be counted because it is 40 days after the activity.
			return map[string]interface{}{weekBefore: 100.0, today: 900.0}, nil
		},
		SetBoosterDataFunc: func(ctx context.Context, userId string, boosterId string, data map[string]interface{}) error {
			saved = data
			return nil
		},
	}
	provider := NewRecoveryAdvisor()
	provider.Service = &bootstrap.Service{DB: mockDB}

	activity := makeActivity(30, 150)
	activity.StartTime = timestamppb.New(activityDay.Add(8 * time.Hour))
	user := &user.Record{UserProfile: &pbuser.UserProfile{UserId: "test-user"}}

	result, err := provider.Enrich(context.Background(), slog.Default(), activity, user, nil, false)
	if err != nil {
		t.Fatalf("Enrich failed: %v", err)
	}
	if _, ok := saved[dayKey]; !ok {
		t.Fatalf("expected today's load saved under the activity date %s, got keys %v", dayKey, keysOf(saved))
	}
	if _, ok := saved[today]; ok {
		t.Errorf("load must not be written under processing date %s", today)
	}
	var acute float64
	fmt.Sscanf(result.Metadata["acute_load"], "%f", &acute)
	// 100 (three days earlier) + this session; the 900 from 40 days later is excluded.
	if acute < 100 || acute >= 900 {
		t.Errorf("acute load %.0f should include the earlier 100 and exclude the later 900", acute)
	}
}

func keysOf(m map[string]interface{}) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	return out
}
