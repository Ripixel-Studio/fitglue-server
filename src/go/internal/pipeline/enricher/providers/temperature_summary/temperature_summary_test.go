package temperature_summary

import (
	"context"
	"io"
	"log/slog"
	"testing"

	pbactivity "github.com/fitglue/server/src/go/pkg/types/pb/models/activity"
	pbplugin "github.com/fitglue/server/src/go/pkg/types/pb/models/plugin"
)

func tempLogger() *slog.Logger { return slog.New(slog.NewTextHandler(io.Discard, nil)) }

func i32(v int32) *int32 { return &v }

func recordWithTemp(temp *int32) *pbactivity.Record {
	return &pbactivity.Record{Temperature: temp}
}

func TestTemperatureSummary_Metadata(t *testing.T) {
	p := NewTemperatureSummary()
	if p.Name() != "temperature-summary" {
		t.Errorf("unexpected name %q", p.Name())
	}
	if p.ProviderType() != pbplugin.EnricherProviderType_ENRICHER_PROVIDER_TEMPERATURE_SUMMARY {
		t.Errorf("unexpected provider type %v", p.ProviderType())
	}
	p.SetService(nil)
}

func TestTemperatureSummary_Enrich_NoData(t *testing.T) {
	p := NewTemperatureSummary()
	act := &pbactivity.StandardizedActivity{
		Sessions: []*pbactivity.Session{{Laps: []*pbactivity.Lap{{Records: []*pbactivity.Record{
			recordWithTemp(nil),
		}}}}},
	}
	res, err := p.Enrich(context.Background(), tempLogger(), act, nil, nil, false)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if res.Metadata["temp_summary_status"] != "skipped" {
		t.Errorf("expected skipped, got %v", res.Metadata)
	}
}

func TestTemperatureSummary_Enrich_Success(t *testing.T) {
	p := NewTemperatureSummary()
	act := &pbactivity.StandardizedActivity{
		Sessions: []*pbactivity.Session{{Laps: []*pbactivity.Lap{{Records: []*pbactivity.Record{
			recordWithTemp(i32(10)),
			recordWithTemp(i32(20)),
			recordWithTemp(i32(30)),
		}}}}},
	}
	res, err := p.Enrich(context.Background(), tempLogger(), act, nil, nil, false)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if res.Metadata["temp_summary_status"] != "success" {
		t.Fatalf("expected success, got %v", res.Metadata)
	}
	if res.Metadata["temp_min"] != "10" || res.Metadata["temp_max"] != "30" || res.Metadata["temp_avg"] != "20" {
		t.Errorf("unexpected min/avg/max: %v", res.Metadata)
	}
	if res.Enrichments == nil || res.Enrichments.Temperature == nil {
		t.Fatal("expected temperature enrichment")
	}
	temp := res.Enrichments.Temperature
	if temp.MinC != 10 || temp.MaxC != 30 || temp.AvgC != 20 {
		t.Errorf("unexpected enrichment values: %+v", temp)
	}
	if res.Description == "" {
		t.Error("expected a description")
	}
}
