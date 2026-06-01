package personal_records

import (
	"github.com/fitglue/server/src/go/internal/pipeline/enricher/shared/distanceeffort"
	pbactivity "github.com/fitglue/server/src/go/pkg/types/pb/models/activity"
)

func findFastestSegment(activity *pbactivity.StandardizedActivity, targetDistanceM float64) float64 {
	return distanceeffort.FindFastestSegment(activity, targetDistanceM)
}
