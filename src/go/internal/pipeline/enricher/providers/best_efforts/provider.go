package best_efforts

import (
	"context"
	"fmt"
	"log/slog"

	"github.com/fitglue/server/src/go/internal/pipeline/enricher/providers"
	"github.com/fitglue/server/src/go/internal/pipeline/enricher/shared/distanceeffort"
	"github.com/fitglue/server/src/go/pkg/bootstrap"
	"github.com/fitglue/server/src/go/pkg/domain/user"
	pbactivity "github.com/fitglue/server/src/go/pkg/types/pb/models/activity"
	pbplugin "github.com/fitglue/server/src/go/pkg/types/pb/models/plugin"
)

// standardEffort is an entry in the curated best-efforts distance list.
type standardEffort struct {
	key       string
	display   string
	distanceM float64
}

// runningEfforts is the Strava-style curated set for running activities.
var runningEfforts = []standardEffort{
	{"400m", "400m", 400},
	{"1k", "1K", 1000},
	{"1_mile", "1 Mile", 1609.344},
	{"5k", "5K", 5000},
	{"10k", "10K", 10000},
	{"half_marathon", "Half Marathon", 21097.5},
	{"marathon", "Marathon", 42195},
}

// cyclingEfforts is the curated set for cycling activities.
var cyclingEfforts = []standardEffort{
	{"5k", "5K", 5000},
	{"10k", "10K", 10000},
	{"20k", "20K", 20000},
	{"40k", "40K", 40000},
	{"100k", "100K", 100000},
}

// BestEfforts calculates per-activity best efforts across standard distances
// for running and cycling. Unlike the personal_records enricher it does not
// compare against stored records — it always outputs the effort times for the
// current activity so they can be surfaced on the showcase and activity view.
type BestEfforts struct {
	Service *bootstrap.Service
}

func init() {
	providers.Register(NewBestEfforts())
}

func NewBestEfforts() *BestEfforts {
	return &BestEfforts{}
}

func (p *BestEfforts) SetService(service *bootstrap.Service) {
	p.Service = service
}

func (p *BestEfforts) Name() string {
	return "best-efforts"
}

func (p *BestEfforts) ProviderType() pbplugin.EnricherProviderType {
	return pbplugin.EnricherProviderType_ENRICHER_PROVIDER_BEST_EFFORTS
}

func (p *BestEfforts) Enrich(
	ctx context.Context,
	logger *slog.Logger,
	activity *pbactivity.StandardizedActivity,
	u *user.Record,
	inputs map[string]string,
	doNotRetry bool,
) (*providers.EnrichmentResult, error) {
	actType := activity.Type

	showRunning := inputs["running"] != "false"
	showCycling := inputs["cycling"] != "false"

	var candidates []standardEffort
	switch {
	case showRunning && isRunning(actType):
		candidates = runningEfforts
	case showCycling && isCycling(actType):
		candidates = cyclingEfforts
	default:
		return &providers.EnrichmentResult{
			Metadata: map[string]string{"best_efforts_status": "skipped"},
		}, nil
	}

	var efforts []*pbactivity.BestEffort
	for _, c := range candidates {
		secs := distanceeffort.FindFastestSegment(activity, c.distanceM)
		if secs <= 0 {
			continue
		}
		efforts = append(efforts, &pbactivity.BestEffort{
			DistanceKey: c.key,
			Display:     c.display,
			DistanceM:   c.distanceM,
			TimeSeconds: secs,
		})
	}

	if len(efforts) == 0 {
		return &providers.EnrichmentResult{
			Metadata: map[string]string{"best_efforts_status": "no_data"},
		}, nil
	}

	logger.Info("best_efforts: calculated efforts", "count", len(efforts), "activity_type", actType)

	return &providers.EnrichmentResult{
		Metadata: map[string]string{
			"best_efforts_status": "success",
			"best_efforts_count":  fmt.Sprintf("%d", len(efforts)),
		},
		Enrichments: &pbactivity.ActivityEnrichments{
			BestEfforts: &pbactivity.BestEffortsSummary{
				Efforts: efforts,
			},
		},
	}, nil
}

func isRunning(t pbactivity.ActivityType) bool {
	switch t {
	case pbactivity.ActivityType_ACTIVITY_TYPE_RUN,
		pbactivity.ActivityType_ACTIVITY_TYPE_TRAIL_RUN,
		pbactivity.ActivityType_ACTIVITY_TYPE_VIRTUAL_RUN:
		return true
	}
	return false
}

func isCycling(t pbactivity.ActivityType) bool {
	switch t {
	case pbactivity.ActivityType_ACTIVITY_TYPE_RIDE,
		pbactivity.ActivityType_ACTIVITY_TYPE_VIRTUAL_RIDE,
		pbactivity.ActivityType_ACTIVITY_TYPE_GRAVEL_RIDE,
		pbactivity.ActivityType_ACTIVITY_TYPE_MOUNTAIN_BIKE_RIDE,
		pbactivity.ActivityType_ACTIVITY_TYPE_EBIKE_RIDE:
		return true
	}
	return false
}
