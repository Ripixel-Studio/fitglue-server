package fitbit_heart_rate

import (
	user "github.com/fitglue/server/src/go/pkg/domain/user"

	pbuser "github.com/fitglue/server/src/go/pkg/types/pb/models/user"

	pbactivity "github.com/fitglue/server/src/go/pkg/types/pb/models/activity"

	"bytes"
	"context"
	"io"
	"log/slog"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/fitglue/server/src/go/internal/pipeline/enricher/providers"
	"github.com/fitglue/server/src/go/pkg/bootstrap"
	"google.golang.org/protobuf/types/known/timestamppb"
)

const mockProfileUTC = `{"user":{"timezone":"UTC"}}`

func TestFitBitHeartRate_Enrich(t *testing.T) {
	// Setup mock HTTP client
	mockHTTPClient := &http.Client{
		Transport: &mockTransport{
			DoFunc: func(req *http.Request) (*http.Response, error) {
				if strings.Contains(req.URL.Path, "/profile.json") {
					return &http.Response{
						StatusCode: 200,
						Body:       io.NopCloser(bytes.NewBufferString(mockProfileUTC)),
					}, nil
				}
				mockResponse := `{
					"activities-heart-intraday": {
						"dataset": [
							{"time": "10:00:00", "value": 120},
							{"time": "10:00:30", "value": 125},
							{"time": "10:01:00", "value": 130}
						]
					}
				}`
				return &http.Response{
					StatusCode: 200,
					Body:       io.NopCloser(bytes.NewBufferString(mockResponse)),
				}, nil
			},
		},
	}

	// Create provider with mock service
	provider := NewFitBitHeartRate()
	provider.Service = &bootstrap.Service{}

	// Create test activity
	startTime := time.Date(2025, 12, 25, 10, 0, 0, 0, time.UTC)
	activity := &pbactivity.StandardizedActivity{
		StartTime: timestamppb.New(startTime),
		Sessions: []*pbactivity.Session{
			{TotalElapsedTime: 3600}, // 1 hour
		},
	}

	// Create test user with Fitbit integration
	user := &user.Record{
		UserProfile: &pbuser.UserProfile{
			UserId: "test-user",
		},
		Integrations: &pbuser.UserIntegrations{
			Fitbit: &pbuser.FitbitIntegration{
				Enabled:     true,
				AccessToken: "test-token",
			},
		},
	}

	// Execute enrichment
	result, err := provider.EnrichWithClient(context.Background(), slog.Default(), activity, user, nil, mockHTTPClient, false)

	if err != nil {
		t.Fatalf("Enrich failed: %v", err)
	}

	// Verify result
	if result == nil {
		t.Fatal("Expected non-nil result")
	}

	if result.Metadata["hr_source"] != "fitbit" {
		t.Errorf("Expected hr_source=fitbit, got %s", result.Metadata["hr_source"])
	}
	if result.Metadata["status_detail"] != "Success" {
		t.Errorf("Expected status_detail=Success, got %s", result.Metadata["status_detail"])
	}
	if result.Metadata["query_start"] != "10:00" {
		t.Errorf("Expected query_start=10:00, got %s", result.Metadata["query_start"])
	}

	if len(result.HeartRateStream) != 3600 {
		t.Errorf("Expected heart rate stream of 3600 seconds, got %d", len(result.HeartRateStream))
	}

	// Verify heart rate stream has data
	foundData := false
	for _, val := range result.HeartRateStream {
		if val > 0 {
			foundData = true
			break
		}
	}
	if !foundData {
		t.Error("Heart rate stream contains only zeros, expected populated data")
	}
}

func TestFitBitHeartRate_Enrich_IntegrationDisabled(t *testing.T) {
	provider := NewFitBitHeartRate()
	provider.Service = &bootstrap.Service{}

	activity := &pbactivity.StandardizedActivity{
		StartTime: timestamppb.New(time.Now()),
	}

	user := &user.Record{
		UserProfile: &pbuser.UserProfile{
			UserId: "test-user",
		},
		Integrations: &pbuser.UserIntegrations{
			Fitbit: &pbuser.FitbitIntegration{
				Enabled: false,
			},
		},
	}

	_, err := provider.Enrich(context.Background(), slog.Default(), activity, user, nil, false)
	if err == nil {
		t.Error("Expected error when Fitbit integration is disabled")
	}
}

func TestFitBitHeartRate_Enrich_APIError(t *testing.T) {
	mockHTTPClient := &http.Client{
		Transport: &mockTransport{
			DoFunc: func(req *http.Request) (*http.Response, error) {
				if strings.Contains(req.URL.Path, "/profile.json") {
					return &http.Response{
						StatusCode: 200,
						Body:       io.NopCloser(bytes.NewBufferString(mockProfileUTC)),
					}, nil
				}
				return &http.Response{
					StatusCode: 401,
					Body:       io.NopCloser(bytes.NewBufferString(`{"errors":[{"errorType":"invalid_token"}]}`)),
				}, nil
			},
		},
	}

	provider := NewFitBitHeartRate()
	provider.Service = &bootstrap.Service{}

	activity := &pbactivity.StandardizedActivity{
		StartTime: timestamppb.New(time.Now()),
		Sessions:  []*pbactivity.Session{{TotalElapsedTime: 3600}},
	}

	user := &user.Record{
		UserProfile: &pbuser.UserProfile{
			UserId: "test-user",
		},
		Integrations: &pbuser.UserIntegrations{
			Fitbit: &pbuser.FitbitIntegration{
				Enabled:     true,
				AccessToken: "invalid-token",
			},
		},
	}

	_, err := provider.EnrichWithClient(context.Background(), slog.Default(), activity, user, nil, mockHTTPClient, false)
	if err == nil {
		t.Error("Expected error when API returns 401")
	}
}

// mockTransport implements http.RoundTripper
type mockTransport struct {
	DoFunc func(req *http.Request) (*http.Response, error)
}

func (m *mockTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	if m.DoFunc != nil {
		return m.DoFunc(req)
	}
	if strings.Contains(req.URL.Path, "/profile.json") {
		return &http.Response{
			StatusCode: 200,
			Body:       io.NopCloser(bytes.NewBufferString(mockProfileUTC)),
		}, nil
	}
	return &http.Response{
		StatusCode: 200,
		Body:       io.NopCloser(bytes.NewBufferString(`{"activities-heart-intraday":{"dataset":[]}}`)),
	}, nil
}

func TestFitBitHeartRate_Enrich_LagDetected(t *testing.T) {
	mockHTTPClient := &http.Client{
		Transport: &mockTransport{
			DoFunc: func(req *http.Request) (*http.Response, error) {
				if strings.Contains(req.URL.Path, "/profile.json") {
					return &http.Response{
						StatusCode: 200,
						Body:       io.NopCloser(bytes.NewBufferString(mockProfileUTC)),
					}, nil
				}
				return &http.Response{
					StatusCode: 200,
					Body:       io.NopCloser(bytes.NewBufferString(`{"activities-heart-intraday":{"dataset":[]}}`)),
				}, nil
			},
		},
	}

	provider := NewFitBitHeartRate()
	provider.Service = &bootstrap.Service{}

	// Activity ended 5 minutes ago (Recent -> Should Retry)
	endTime := time.Now().Add(-5 * time.Minute)
	startTime := endTime.Add(-1 * time.Hour)

	activity := &pbactivity.StandardizedActivity{
		StartTime: timestamppb.New(startTime),
		Sessions:  []*pbactivity.Session{{TotalElapsedTime: 3600}},
	}

	user := &user.Record{
		UserProfile: &pbuser.UserProfile{
			UserId: "test-user",
		},
		Integrations: &pbuser.UserIntegrations{
			Fitbit: &pbuser.FitbitIntegration{Enabled: true, AccessToken: "t"},
		},
	}

	_, err := provider.EnrichWithClient(context.Background(), slog.Default(), activity, user, nil, mockHTTPClient, false)
	if err == nil {
		t.Fatal("Expected error for recent missing data")
	}

	if retryErr, ok := err.(*providers.RetryableError); !ok {
		t.Errorf("Expected RetryableError, got %T: %v", err, err)
	} else {
		if retryErr.RetryAfter == 0 {
			t.Error("Expected non-zero RetryAfter")
		}
	}
}

func TestFitBitHeartRate_Enrich_LagExpired(t *testing.T) {
	mockHTTPClient := &http.Client{
		Transport: &mockTransport{
			DoFunc: func(req *http.Request) (*http.Response, error) {
				if strings.Contains(req.URL.Path, "/profile.json") {
					return &http.Response{
						StatusCode: 200,
						Body:       io.NopCloser(bytes.NewBufferString(mockProfileUTC)),
					}, nil
				}
				return &http.Response{
					StatusCode: 200,
					Body:       io.NopCloser(bytes.NewBufferString(`{"activities-heart-intraday":{"dataset":[]}}`)),
				}, nil
			},
		},
	}

	provider := NewFitBitHeartRate()
	provider.Service = &bootstrap.Service{}

	// Activity ended 2 hours ago (Old -> Should Accept Empty)
	endTime := time.Now().Add(-2 * time.Hour)
	startTime := endTime.Add(-1 * time.Hour)

	activity := &pbactivity.StandardizedActivity{
		StartTime: timestamppb.New(startTime),
		Sessions:  []*pbactivity.Session{{TotalElapsedTime: 3600}},
	}

	user := &user.Record{
		UserProfile: &pbuser.UserProfile{
			UserId: "test-user",
		},
		Integrations: &pbuser.UserIntegrations{
			Fitbit: &pbuser.FitbitIntegration{Enabled: true, AccessToken: "t"},
		},
	}

	res, err := provider.EnrichWithClient(context.Background(), slog.Default(), activity, user, nil, mockHTTPClient, false)
	if err != nil {
		t.Fatalf("Expected success for old missing data, got: %v", err)
	}
	if len(res.HeartRateStream) != 3600 {
		t.Errorf("Expected stream length 3600, got %d", len(res.HeartRateStream))
	}
}

func TestFitBitHeartRate_Enrich_LagReArmedOnResume(t *testing.T) {
	// An old activity (not "recent") with incomplete Fitbit data would normally be accepted as
	// partial. But when resolving a pending input (is_resume=true) we re-arm the lag retry so
	// Fitbit gets time to finish syncing rather than committing to partial coverage.
	mockHTTPClient := &http.Client{
		Transport: &mockTransport{
			DoFunc: func(req *http.Request) (*http.Response, error) {
				if strings.Contains(req.URL.Path, "/profile.json") {
					return &http.Response{
						StatusCode: 200,
						Body:       io.NopCloser(bytes.NewBufferString(mockProfileUTC)),
					}, nil
				}
				return &http.Response{
					StatusCode: 200,
					Body:       io.NopCloser(bytes.NewBufferString(`{"activities-heart-intraday":{"dataset":[]}}`)),
				}, nil
			},
		},
	}

	provider := NewFitBitHeartRate()
	provider.Service = &bootstrap.Service{}

	// Activity ended 2 hours ago — not recent — but we are resuming.
	endTime := time.Now().Add(-2 * time.Hour)
	startTime := endTime.Add(-1 * time.Hour)

	activity := &pbactivity.StandardizedActivity{
		StartTime: timestamppb.New(startTime),
		Sessions:  []*pbactivity.Session{{TotalElapsedTime: 3600}},
	}

	user := &user.Record{
		UserProfile: &pbuser.UserProfile{UserId: "test-user"},
		Integrations: &pbuser.UserIntegrations{
			Fitbit: &pbuser.FitbitIntegration{Enabled: true, AccessToken: "t"},
		},
	}

	inputs := map[string]string{"is_resume": "true"}
	_, err := provider.EnrichWithClient(context.Background(), slog.Default(), activity, user, inputs, mockHTTPClient, false)
	if err == nil {
		t.Fatal("Expected RetryableError when resuming with incomplete data")
	}
	if _, ok := err.(*providers.RetryableError); !ok {
		t.Errorf("Expected RetryableError, got %T: %v", err, err)
	}

	// Sanity: without the resume flag, the same old activity is accepted as partial (no retry).
	_, err = provider.EnrichWithClient(context.Background(), slog.Default(), activity, user, nil, mockHTTPClient, false)
	if err != nil {
		t.Fatalf("Expected success for old activity without resume, got: %v", err)
	}
}

func TestFitBitHeartRate_Name(t *testing.T) {
	provider := NewFitBitHeartRate()
	expected := "fitbit-heart-rate"
	if provider.Name() != expected {
		t.Errorf("Expected provider name %q, got %q", expected, provider.Name())
	}
}

// TestFitBitHeartRate_IsIdempotent is a regression test: the fetched stream depends on
// Fitbit's own sync lag at query time, so re-running on resume can silently commit a
// less-complete stream than the one already applied to the activity. The provider must
// report itself as non-idempotent so the orchestrator replays the journaled stream on
// resume instead of re-querying Fitbit.
func TestFitBitHeartRate_IsIdempotent(t *testing.T) {
	provider := NewFitBitHeartRate()
	if provider.IsIdempotent() {
		t.Error("Expected IsIdempotent() to return false so resume replays the journaled stream instead of re-fetching")
	}
}

func TestFitBitHeartRate_Enrich_LagExhausted(t *testing.T) {
	mockHTTPClient := &http.Client{
		Transport: &mockTransport{
			DoFunc: func(req *http.Request) (*http.Response, error) {
				if strings.Contains(req.URL.Path, "/profile.json") {
					return &http.Response{
						StatusCode: 200,
						Body:       io.NopCloser(bytes.NewBufferString(mockProfileUTC)),
					}, nil
				}
				return &http.Response{
					StatusCode: 200,
					Body:       io.NopCloser(bytes.NewBufferString(`{"activities-heart-intraday":{"dataset":[]}}`)),
				}, nil
			},
		},
	}

	provider := NewFitBitHeartRate()
	provider.Service = &bootstrap.Service{}

	// Activity ended 5 minutes ago (Recent -> Should Normally Retry)
	endTime := time.Now().Add(-5 * time.Minute)
	startTime := endTime.Add(-1 * time.Hour)

	activity := &pbactivity.StandardizedActivity{
		StartTime: timestamppb.New(startTime),
		Sessions:  []*pbactivity.Session{{TotalElapsedTime: 3600}},
	}

	user := &user.Record{
		UserProfile: &pbuser.UserProfile{
			UserId: "test-user",
		},
		Integrations: &pbuser.UserIntegrations{
			Fitbit: &pbuser.FitbitIntegration{Enabled: true, AccessToken: "t"},
		},
	}

	// Should not return error despite missing data (doNotRetry=true)
	_, err := provider.EnrichWithClient(context.Background(), slog.Default(), activity, user, nil, mockHTTPClient, true)
	if err != nil {
		t.Fatalf("Expected success when doNotRetry is set, got error: %v", err)
	}

}

func TestFitBitHeartRate_Enrich_SkipIfExistingHRData(t *testing.T) {
	provider := NewFitBitHeartRate()
	provider.Service = &bootstrap.Service{}

	// Create activity WITH existing heart rate data
	startTime := time.Date(2025, 12, 25, 10, 0, 0, 0, time.UTC)
	activity := &pbactivity.StandardizedActivity{
		StartTime: timestamppb.New(startTime),
		Sessions: []*pbactivity.Session{
			{
				TotalElapsedTime: 3600,
				Laps: []*pbactivity.Lap{
					{
						Records: []*pbactivity.Record{
							{HeartRate: 120}, // Existing HR data
							{HeartRate: 130},
						},
					},
				},
			},
		},
	}

	user := &user.Record{
		UserProfile: &pbuser.UserProfile{
			UserId: "test-user",
		},
		Integrations: &pbuser.UserIntegrations{
			Fitbit: &pbuser.FitbitIntegration{
				Enabled:     true,
				AccessToken: "test-token",
			},
		},
	}

	// Without force=true, should skip
	result, err := provider.Enrich(context.Background(), slog.Default(), activity, user, nil, false)
	if err != nil {
		t.Fatalf("Expected no error, got: %v", err)
	}

	if result.Metadata["hr_source"] != "skipped" {
		t.Errorf("Expected hr_source=skipped, got %s", result.Metadata["hr_source"])
	}
	if result.Metadata["force"] != "false" {
		t.Errorf("Expected force=false in metadata, got %s", result.Metadata["force"])
	}
}

// TestFitBitHeartRate_Enrich_BSTTimezone is a regression test for the UTC/BST bug.
// Fitbit's intraday API uses the user's profile timezone, not UTC. When a user in BST
// (UTC+1) does a workout at 10:00 BST (= 09:00 UTC), sending "09:00" to Fitbit causes
// it to fetch data for 09:00 BST (= 08:00 UTC) — one full hour too early. This means
// pre-workout resting HR (Zone 1) gets mapped onto the activity instead of real data.
func TestFitBitHeartRate_Enrich_BSTTimezone(t *testing.T) {
	var capturedURL string
	mockHTTPClient := &http.Client{
		Transport: &mockTransport{
			DoFunc: func(req *http.Request) (*http.Response, error) {
				if strings.Contains(req.URL.Path, "/profile.json") {
					return &http.Response{
						StatusCode: 200,
						Body:       io.NopCloser(bytes.NewBufferString(`{"user":{"timezone":"Europe/London"}}`)),
					}, nil
				}
				capturedURL = req.URL.String()
				// Fitbit returns times in local (BST) format
				mockResponse := `{
					"activities-heart-intraday": {
						"dataset": [
							{"time": "10:00:00", "value": 150},
							{"time": "10:01:00", "value": 155},
							{"time": "10:02:00", "value": 160}
						]
					}
				}`
				return &http.Response{
					StatusCode: 200,
					Body:       io.NopCloser(bytes.NewBufferString(mockResponse)),
				}, nil
			},
		},
	}

	provider := NewFitBitHeartRate()
	provider.Service = &bootstrap.Service{}

	// Activity at 09:00 UTC on a past summer date = 10:00 BST
	startTime := time.Date(2025, 6, 15, 9, 0, 0, 0, time.UTC)
	activity := &pbactivity.StandardizedActivity{
		StartTime: timestamppb.New(startTime),
		Sessions:  []*pbactivity.Session{{TotalElapsedTime: 3600}},
	}

	user := &user.Record{
		UserProfile: &pbuser.UserProfile{UserId: "test-user"},
		Integrations: &pbuser.UserIntegrations{
			Fitbit: &pbuser.FitbitIntegration{Enabled: true, AccessToken: "test-token"},
		},
	}

	result, err := provider.EnrichWithClient(context.Background(), slog.Default(), activity, user, nil, mockHTTPClient, false)
	if err != nil {
		t.Fatalf("Enrich failed: %v", err)
	}

	// The query must use local BST time "10:00", not UTC "09:00"
	if result.Metadata["query_start"] != "10:00" {
		t.Errorf("Expected query_start=10:00 (BST local time), got %s — UTC was sent instead of local time", result.Metadata["query_start"])
	}
	if !strings.Contains(capturedURL, "10:00") {
		t.Errorf("Expected Fitbit API URL to contain local time 10:00, got: %s", capturedURL)
	}

	// The HR stream should contain the elevated heart rate data (not Zone 1 resting HR)
	if result.HeartRateStream[0] == 0 {
		t.Error("Expected populated heart rate stream at start, got zero")
	}
}

func TestFitBitHeartRate_Enrich_ForceOverwrite(t *testing.T) {
	mockHTTPClient := &http.Client{
		Transport: &mockTransport{
			DoFunc: func(req *http.Request) (*http.Response, error) {
				if strings.Contains(req.URL.Path, "/profile.json") {
					return &http.Response{
						StatusCode: 200,
						Body:       io.NopCloser(bytes.NewBufferString(mockProfileUTC)),
					}, nil
				}
				mockResponse := `{
					"activities-heart-intraday": {
						"dataset": [
							{"time": "10:00:00", "value": 120}
						]
					}
				}`
				return &http.Response{
					StatusCode: 200,
					Body:       io.NopCloser(bytes.NewBufferString(mockResponse)),
				}, nil
			},
		},
	}

	provider := NewFitBitHeartRate()
	provider.Service = &bootstrap.Service{}

	// Create activity WITH existing heart rate data
	startTime := time.Date(2025, 12, 25, 10, 0, 0, 0, time.UTC)
	activity := &pbactivity.StandardizedActivity{
		StartTime: timestamppb.New(startTime),
		Sessions: []*pbactivity.Session{
			{
				TotalElapsedTime: 3600,
				Laps: []*pbactivity.Lap{
					{
						Records: []*pbactivity.Record{
							{HeartRate: 120}, // Existing HR data
						},
					},
				},
			},
		},
	}

	user := &user.Record{
		UserProfile: &pbuser.UserProfile{
			UserId: "test-user",
		},
		Integrations: &pbuser.UserIntegrations{
			Fitbit: &pbuser.FitbitIntegration{
				Enabled:     true,
				AccessToken: "test-token",
			},
		},
	}

	// With force=true, should proceed to fetch from Fitbit
	result, err := provider.EnrichWithClient(context.Background(), slog.Default(), activity, user, map[string]string{"force": "true"}, mockHTTPClient, false)
	if err != nil {
		t.Fatalf("Expected no error with force=true, got: %v", err)
	}

	if result.Metadata["hr_source"] != "fitbit" {
		t.Errorf("Expected hr_source=fitbit with force=true, got %s", result.Metadata["hr_source"])
	}
}

func TestFitBitHeartRate_Enrich_SpansMidnight(t *testing.T) {
	var capturedURL string
	mockHTTPClient := &http.Client{
		Transport: &mockTransport{
			DoFunc: func(req *http.Request) (*http.Response, error) {
				if strings.Contains(req.URL.Path, "/profile.json") {
					return &http.Response{
						StatusCode: 200,
						Body:       io.NopCloser(bytes.NewBufferString(mockProfileUTC)),
					}, nil
				}
				capturedURL = req.URL.String()
				mockResponse := `{
					"activities-heart-intraday": {
						"dataset": [
							{"time": "23:58:00", "value": 120},
							{"time": "23:59:00", "value": 125},
							{"time": "00:00:00", "value": 130},
							{"time": "00:05:00", "value": 135},
							{"time": "00:10:00", "value": 140}
						]
					}
				}`
				return &http.Response{
					StatusCode: 200,
					Body:       io.NopCloser(bytes.NewBufferString(mockResponse)),
				}, nil
			},
		},
	}

	provider := NewFitBitHeartRate()
	provider.Service = &bootstrap.Service{}

	// Activity starts at 23:58 on Jan 1 and ends at 00:10 on Jan 2 (12 minutes spanning midnight)
	startTime := time.Date(2026, 1, 1, 23, 58, 0, 0, time.UTC)
	activity := &pbactivity.StandardizedActivity{
		StartTime: timestamppb.New(startTime),
		Sessions: []*pbactivity.Session{
			{TotalElapsedTime: 720}, // 12 minutes
		},
	}

	user := &user.Record{
		UserProfile: &pbuser.UserProfile{
			UserId: "test-user",
		},
		Integrations: &pbuser.UserIntegrations{
			Fitbit: &pbuser.FitbitIntegration{
				Enabled:     true,
				AccessToken: "test-token",
			},
		},
	}

	result, err := provider.EnrichWithClient(context.Background(), slog.Default(), activity, user, nil, mockHTTPClient, false)
	if err != nil {
		t.Fatalf("Enrich failed for midnight-spanning activity: %v", err)
	}

	// Verify the date range API was called (URL should contain both dates)
	if !bytes.Contains([]byte(capturedURL), []byte("2026-01-01")) {
		t.Errorf("Expected URL to contain start date 2026-01-01, got: %s", capturedURL)
	}
	if !bytes.Contains([]byte(capturedURL), []byte("2026-01-02")) {
		t.Errorf("Expected URL to contain end date 2026-01-02, got: %s", capturedURL)
	}

	// Verify metadata indicates this spanned midnight
	if result.Metadata["spans_midnight"] != "true" {
		t.Errorf("Expected spans_midnight=true, got %s", result.Metadata["spans_midnight"])
	}
	if result.Metadata["query_date"] != "2026-01-01" {
		t.Errorf("Expected query_date=2026-01-01, got %s", result.Metadata["query_date"])
	}
	if result.Metadata["query_end_date"] != "2026-01-02" {
		t.Errorf("Expected query_end_date=2026-01-02, got %s", result.Metadata["query_end_date"])
	}

	// Verify heart rate stream has data
	if len(result.HeartRateStream) != 720 {
		t.Errorf("Expected heart rate stream of 720 seconds, got %d", len(result.HeartRateStream))
	}
}
