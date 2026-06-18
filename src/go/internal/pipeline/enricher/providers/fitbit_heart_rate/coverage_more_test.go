package fitbit_heart_rate

import (
	"bytes"
	"context"
	"io"
	"log/slog"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/fitglue/server/src/go/pkg/bootstrap"
	user "github.com/fitglue/server/src/go/pkg/domain/user"
	pbactivity "github.com/fitglue/server/src/go/pkg/types/pb/models/activity"
	pbplugin "github.com/fitglue/server/src/go/pkg/types/pb/models/plugin"
	pbuser "github.com/fitglue/server/src/go/pkg/types/pb/models/user"
	"google.golang.org/protobuf/types/known/timestamppb"
)

func fbUser() *user.Record {
	return &user.Record{
		UserProfile: &pbuser.UserProfile{UserId: "test-user"},
		Integrations: &pbuser.UserIntegrations{
			Fitbit: &pbuser.FitbitIntegration{Enabled: true, AccessToken: "t"},
		},
	}
}

func TestFitBitHeartRate_SetService_ProviderType(t *testing.T) {
	p := NewFitBitHeartRate()
	p.SetService(nil)
	if p.Service != nil {
		t.Error("expected nil service")
	}
	svc := &bootstrap.Service{}
	p.SetService(svc)
	if p.Service != svc {
		t.Error("expected service to be set")
	}
	if p.ProviderType() != pbplugin.EnricherProviderType_ENRICHER_PROVIDER_FITBIT_HEART_RATE {
		t.Errorf("unexpected provider type %v", p.ProviderType())
	}
}

func TestFitBit_ExtractGPSTimestamps(t *testing.T) {
	start := time.Date(2025, 12, 25, 10, 0, 0, 0, time.UTC)
	activity := &pbactivity.StandardizedActivity{
		Sessions: []*pbactivity.Session{{
			Laps: []*pbactivity.Lap{{
				Records: []*pbactivity.Record{
					{Timestamp: timestamppb.New(start)},
					{Timestamp: timestamppb.New(start.Add(time.Second))},
					{}, // nil -> skipped
				},
			}},
		}},
	}
	if got := len(extractGPSTimestamps(activity)); got != 2 {
		t.Errorf("expected 2 timestamps, got %d", got)
	}
}

// TestFitBitHeartRate_Enrich_NonUTCTimezone exercises fetchFitbitTimezone's success
// path (a valid IANA zone loaded from the profile) plus the local-time logging branch.
func TestFitBitHeartRate_Enrich_NonUTCTimezone(t *testing.T) {
	mockHTTPClient := &http.Client{
		Transport: &mockTransport{
			DoFunc: func(req *http.Request) (*http.Response, error) {
				if strings.Contains(req.URL.Path, "/profile.json") {
					return &http.Response{
						StatusCode: 200,
						Body:       io.NopCloser(bytes.NewBufferString(`{"user":{"timezone":"America/New_York"}}`)),
					}, nil
				}
				return &http.Response{
					StatusCode: 200,
					Body: io.NopCloser(bytes.NewBufferString(`{"activities-heart-intraday":{"dataset":[
						{"time":"06:00:00","value":120},{"time":"06:30:00","value":130}]}}`)),
				}, nil
			},
		},
	}
	p := NewFitBitHeartRate()
	p.Service = &bootstrap.Service{}

	// A past date so the activity is not "recent" and partial data is accepted.
	start := time.Date(2025, 6, 15, 10, 0, 0, 0, time.UTC)
	activity := &pbactivity.StandardizedActivity{
		StartTime: timestamppb.New(start),
		Sessions:  []*pbactivity.Session{{TotalElapsedTime: 3600}},
	}

	res, err := p.EnrichWithClient(context.Background(), slog.Default(), activity, fbUser(), nil, mockHTTPClient, false)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// New York is UTC-4 in June; 10:00 UTC -> 06:00 local.
	if res.Metadata["query_start"] != "06:00" {
		t.Errorf("expected query_start=06:00 (EDT), got %s", res.Metadata["query_start"])
	}
}

// TestFitBitHeartRate_Enrich_GPSAlignmentPath drives the hasGPSData branch which
// applies absolute-timestamp HR alignment via providers.AlignTimeSeries.
func TestFitBitHeartRate_Enrich_GPSAlignmentPath(t *testing.T) {
	mockHTTPClient := &http.Client{
		Transport: &mockTransport{
			DoFunc: func(req *http.Request) (*http.Response, error) {
				if strings.Contains(req.URL.Path, "/profile.json") {
					return &http.Response{
						StatusCode: 200,
						Body:       io.NopCloser(bytes.NewBufferString(mockProfileUTC)),
					}, nil
				}
				// One HR sample per minute across the session.
				var sb strings.Builder
				sb.WriteString(`{"activities-heart-intraday":{"dataset":[`)
				for i := 0; i < 10; i++ {
					if i > 0 {
						sb.WriteString(",")
					}
					sb.WriteString(`{"time":"10:0` + string(rune('0'+i)) + `:00","value":` + itoa(120+i) + `}`)
				}
				sb.WriteString(`]}}`)
				return &http.Response{StatusCode: 200, Body: io.NopCloser(bytes.NewBufferString(sb.String()))}, nil
			},
		},
	}
	p := NewFitBitHeartRate()
	p.Service = &bootstrap.Service{}

	// Past date (not recent). Records carry GPS coords + timestamps to trigger alignment.
	start := time.Date(2025, 6, 15, 10, 0, 0, 0, time.UTC)
	records := make([]*pbactivity.Record, 0, 600)
	for i := 0; i < 600; i++ {
		records = append(records, &pbactivity.Record{
			Timestamp:    timestamppb.New(start.Add(time.Duration(i) * time.Second)),
			PositionLat:  51.5 + float64(i)*0.00001,
			PositionLong: -0.12,
		})
	}
	activity := &pbactivity.StandardizedActivity{
		StartTime: timestamppb.New(start),
		Sessions: []*pbactivity.Session{{
			TotalElapsedTime: 600,
			Laps:             []*pbactivity.Lap{{Records: records}},
		}},
	}

	res, err := p.EnrichWithClient(context.Background(), slog.Default(), activity, fbUser(), nil, mockHTTPClient, false)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if res.Metadata["hr_source"] != "fitbit" {
		t.Errorf("expected hr_source=fitbit, got %s", res.Metadata["hr_source"])
	}
	// alignment_status skipped_no_gps must NOT be present since GPS data exists.
	if res.Metadata["alignment_status"] == "skipped_no_gps" {
		t.Errorf("expected GPS alignment to run, got skipped_no_gps")
	}
}

func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	neg := n < 0
	if neg {
		n = -n
	}
	var b [20]byte
	i := len(b)
	for n > 0 {
		i--
		b[i] = byte('0' + n%10)
		n /= 10
	}
	if neg {
		i--
		b[i] = '-'
	}
	return string(b[i:])
}
