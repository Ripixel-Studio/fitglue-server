package best_efforts

import (
	"context"
	"io"
	"log/slog"
	"testing"

	pbactivity "github.com/fitglue/server/src/go/pkg/types/pb/models/activity"
	pbplugin "github.com/fitglue/server/src/go/pkg/types/pb/models/plugin"
	"google.golang.org/protobuf/types/known/timestamppb"
)

func beLogger() *slog.Logger { return slog.New(slog.NewTextHandler(io.Discard, nil)) }

// runActivity builds a constant-pace running activity with native distance.
func runActivity(actType pbactivity.ActivityType, paceMS float64, durationSec int) *pbactivity.StandardizedActivity {
	var records []*pbactivity.Record
	base := int64(1_000_000)
	for i := 0; i <= durationSec; i++ {
		records = append(records, &pbactivity.Record{
			Timestamp: &timestamppb.Timestamp{Seconds: base + int64(i)},
			Distance:  paceMS * float64(i),
			Speed:     paceMS,
		})
	}
	return &pbactivity.StandardizedActivity{
		Type:     actType,
		Sessions: []*pbactivity.Session{{Laps: []*pbactivity.Lap{{Records: records}}}},
	}
}

func TestBestEfforts_Metadata(t *testing.T) {
	p := NewBestEfforts()
	if p.Name() != "best-efforts" {
		t.Errorf("name %q", p.Name())
	}
	if p.ProviderType() != pbplugin.EnricherProviderType_ENRICHER_PROVIDER_BEST_EFFORTS {
		t.Errorf("type %v", p.ProviderType())
	}
	p.SetService(nil)
}

func TestBestEfforts_Running(t *testing.T) {
	// 5 m/s for 1200s -> 6000m, covers 400m/1k/1mile/5k.
	act := runActivity(pbactivity.ActivityType_ACTIVITY_TYPE_RUN, 5.0, 1200)
	res, err := NewBestEfforts().Enrich(context.Background(), beLogger(), act, nil, nil, false)
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	if res.Metadata["best_efforts_status"] != "success" {
		t.Fatalf("expected success, got %v", res.Metadata)
	}
	if res.Enrichments == nil || res.Enrichments.BestEfforts == nil || len(res.Enrichments.BestEfforts.Efforts) == 0 {
		t.Fatal("expected best efforts")
	}
}

func TestBestEfforts_Cycling(t *testing.T) {
	// 10 m/s for 2500s -> 25000m, covers 5k/10k/20k.
	act := runActivity(pbactivity.ActivityType_ACTIVITY_TYPE_RIDE, 10.0, 2500)
	res, err := NewBestEfforts().Enrich(context.Background(), beLogger(), act, nil, nil, false)
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	if res.Metadata["best_efforts_status"] != "success" {
		t.Fatalf("expected success, got %v", res.Metadata)
	}
}

func TestBestEfforts_SkipNonRunOrCycle(t *testing.T) {
	act := &pbactivity.StandardizedActivity{Type: pbactivity.ActivityType_ACTIVITY_TYPE_YOGA}
	res, _ := NewBestEfforts().Enrich(context.Background(), beLogger(), act, nil, nil, false)
	if res.Metadata["best_efforts_status"] != "skipped" {
		t.Errorf("expected skipped, got %v", res.Metadata)
	}
}

func TestBestEfforts_DisabledViaInputs(t *testing.T) {
	act := runActivity(pbactivity.ActivityType_ACTIVITY_TYPE_RUN, 5.0, 1200)
	res, _ := NewBestEfforts().Enrich(context.Background(), beLogger(), act, nil, map[string]string{"running": "false"}, false)
	if res.Metadata["best_efforts_status"] != "skipped" {
		t.Errorf("expected skipped when running disabled, got %v", res.Metadata)
	}
}

func TestBestEfforts_NoData(t *testing.T) {
	// Running type but no record/lap/session data -> no_data.
	act := &pbactivity.StandardizedActivity{Type: pbactivity.ActivityType_ACTIVITY_TYPE_RUN}
	res, _ := NewBestEfforts().Enrich(context.Background(), beLogger(), act, nil, nil, false)
	if res.Metadata["best_efforts_status"] != "no_data" {
		t.Errorf("expected no_data, got %v", res.Metadata)
	}
}

func TestIsRunningIsCycling(t *testing.T) {
	if !isRunning(pbactivity.ActivityType_ACTIVITY_TYPE_TRAIL_RUN) {
		t.Error("trail run should be running")
	}
	if isRunning(pbactivity.ActivityType_ACTIVITY_TYPE_RIDE) {
		t.Error("ride is not running")
	}
	if !isCycling(pbactivity.ActivityType_ACTIVITY_TYPE_MOUNTAIN_BIKE_RIDE) {
		t.Error("MTB should be cycling")
	}
	if isCycling(pbactivity.ActivityType_ACTIVITY_TYPE_RUN) {
		t.Error("run is not cycling")
	}
}
