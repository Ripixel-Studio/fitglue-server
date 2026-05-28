package goal_tracker

import (
	"strings"
	"testing"
	"time"

	pbactivity "github.com/fitglue/server/src/go/pkg/types/pb/models/activity"
)

func TestBuildProgressBar(t *testing.T) {
	tests := []struct {
		percent  float64
		contains []string
	}{
		{0, []string{"0%", "░░░░░░░░░░"}},
		{50, []string{"50%", "█████░░░░░"}},
		{100, []string{"100%", "██████████"}},
		{75, []string{"75%", "███████░░░"}},
		{10, []string{"10%", "█░░░░░░░░░"}},
	}

	for _, tt := range tests {
		bar := buildProgressBar(tt.percent)
		for _, expected := range tt.contains {
			if !strings.Contains(bar, expected) {
				t.Errorf("buildProgressBar(%.0f) = %q, want contains %q", tt.percent, bar, expected)
			}
		}
	}
}

func TestGetMetricValue(t *testing.T) {
	act := &pbactivity.StandardizedActivity{
		Sessions: []*pbactivity.Session{
			{
				TotalDistance:    5000, // 5km
				TotalElapsedTime: 1800, // 0.5 hours
			},
			{
				TotalDistance:    5000, // 5km
				TotalElapsedTime: 1800, // 0.5 hours
			},
		},
	}

	tests := []struct {
		metric   string
		expected float64
	}{
		{"distance", 10.0},  // 5km + 5km
		{"duration", 1.0},   // 0.5h + 0.5h
		{"activities", 1.0}, // always 1
		{"other", 0.0},      // unknown metric
	}

	for _, tt := range tests {
		got := getMetricValue(act, tt.metric)
		if got != tt.expected {
			t.Errorf("getMetricValue(act, %q) = %.2f, want %.2f", tt.metric, got, tt.expected)
		}
	}
}

func TestGetMetricValueEmpty(t *testing.T) {
	act := &pbactivity.StandardizedActivity{}
	if got := getMetricValue(act, "distance"); got != 0 {
		t.Errorf("expected 0 for empty activity, got %.2f", got)
	}
}

func TestGetPeriodLabel(t *testing.T) {
	now := time.Now()
	tests := []struct {
		period string
		want   string
	}{
		{"week", "Weekly"},
		{"year", ""},  // dynamic – just check non-empty
		{"month", ""}, // dynamic – just check non-empty
		{"", ""},      // empty defaults to month
	}

	for _, tt := range tests {
		got := getPeriodLabel(tt.period, now)
		if tt.want != "" && got != tt.want {
			t.Errorf("getPeriodLabel(%q) = %q, want %q", tt.period, got, tt.want)
		}
		if got == "" {
			t.Errorf("getPeriodLabel(%q) returned empty string", tt.period)
		}
	}
}

func TestGetPeriodKey(t *testing.T) {
	now := time.Now()
	tests := []string{"week", "year", "month", ""}
	for _, period := range tests {
		key := getPeriodKey(period, now)
		if key == "" {
			t.Errorf("getPeriodKey(%q) returned empty string", period)
		}
	}

	weekKey := getPeriodKey("week", now)
	if !strings.Contains(weekKey, "W") {
		t.Errorf("week key should contain 'W', got %q", weekKey)
	}

	yearKey := getPeriodKey("year", now)
	if len(yearKey) != 4 {
		t.Errorf("year key should be 4 digits, got %q", yearKey)
	}

	monthKey := getPeriodKey("month", now)
	if !strings.Contains(monthKey, "-") {
		t.Errorf("month key should contain '-', got %q", monthKey)
	}
}

func TestGetPeriodKeyPastActivity(t *testing.T) {
	now := time.Now()
	lastMonth := now.AddDate(0, -1, 0)

	nowKey := getPeriodKey("month", now)
	pastKey := getPeriodKey("month", lastMonth)

	if nowKey == pastKey {
		t.Errorf("current and past month should produce different keys, both got %q", nowKey)
	}
}

func TestGetMetricLabel(t *testing.T) {
	tests := []struct {
		metric string
		want   string
	}{
		{"distance", "km"},
		{"duration", "hours"},
		{"activities", "activities"},
		{"elevation", "m elevation"},
		{"other", "km"}, // default
		{"", "km"},
	}

	for _, tt := range tests {
		got := getMetricLabel(tt.metric)
		if got != tt.want {
			t.Errorf("getMetricLabel(%q) = %q, want %q", tt.metric, got, tt.want)
		}
	}
}

func TestGetDaysRemaining(t *testing.T) {
	now := time.Now()
	tests := []string{"week", "year", "month", ""}
	for _, period := range tests {
		days := getDaysRemaining(period, now)
		if days < 0 {
			t.Errorf("getDaysRemaining(%q) = %d, want >= 0", period, days)
		}
	}
}

func TestGoalTrackerName(t *testing.T) {
	p := NewGoalTracker()
	if p.Name() != "goal-tracker" {
		t.Errorf("unexpected name: %s", p.Name())
	}
}

func TestGoalTrackerProviderType(t *testing.T) {
	p := NewGoalTracker()
	if p.ProviderType() == 0 {
		t.Error("expected non-zero provider type")
	}
}
