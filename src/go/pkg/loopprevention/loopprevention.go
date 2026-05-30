// Package loopprevention provides source-level loop detection for FitGlue activity pipelines.
//
// When activities are uploaded to destinations (Strava, Hevy, etc.), those platforms send
// webhooks back which appear as new activities. This package prevents infinite loops by:
// 1. Tracking uploaded activities in Firestore keyed by {destination}:{destination_id}
// 2. Checking if incoming webhook activities are "bouncebacks" from our own uploads
//
// Key insight: When Hevy sends a webhook, the external ID is Hevy's workout ID, not
// the original source's ID. So we must store records using the destination's ID.
package loopprevention

import (
	"context"
	"fmt"
	"strings"

	pbactivity "github.com/fitglue/server/src/go/pkg/types/pb/models/activity"
	pbplugin "github.com/fitglue/server/src/go/pkg/types/pb/models/plugin"
)

// SourceToDestinationMap maps ActivitySource enums to their corresponding Destination enums.
// Sources without destinations (e.g., FILE_UPLOAD) are not included.
var SourceToDestinationMap = map[pbactivity.ActivitySource]pbplugin.DestinationType{
	pbactivity.ActivitySource_SOURCE_HEVY:   pbplugin.DestinationType_DESTINATION_HEVY,
	pbactivity.ActivitySource_SOURCE_STRAVA: pbplugin.DestinationType_DESTINATION_STRAVA,
	pbactivity.ActivitySource_SOURCE_FITBIT: pbplugin.DestinationType_DESTINATION_UNSPECIFIED, // Future: DESTINATION_FITBIT
	// Note: SOURCE_FILE_UPLOAD, SOURCE_PARKRUN_RESULTS, etc. are source-only
}

// GetCorrespondingDestination returns the destination that corresponds to the given source.
// Returns DESTINATION_UNSPECIFIED if the source has no corresponding destination.
func GetCorrespondingDestination(source pbactivity.ActivitySource) pbplugin.DestinationType {
	if dest, ok := SourceToDestinationMap[source]; ok {
		return dest
	}
	return pbplugin.DestinationType_DESTINATION_UNSPECIFIED
}

// UploadedActivityStore defines the interface for persisting uploaded activity records.
type UploadedActivityStore interface {
	// SetUploadedActivity records that an activity was uploaded to a destination.
	SetUploadedActivity(ctx context.Context, userId string, record *pbactivity.UploadedActivityRecord) error

	// GetUploadedActivity retrieves an uploaded activity record by destination and destination ID.
	// This matches how webhooks arrive: the destination (e.g., Hevy) sends its own ID.
	GetUploadedActivity(ctx context.Context, userId string, destination pbplugin.DestinationType, destinationId string) (*pbactivity.UploadedActivityRecord, error)
}

// IsBounceback checks if an incoming activity from a webhook is a "bounceback" from
// our own upload. Returns true if we have a record of uploading this activity.
//
// The check works by:
//  1. Getting the destination that corresponds to the webhook source (e.g., SOURCE_HEVY -> DESTINATION_HEVY)
//  2. Looking up if we have a record of uploading to that destination with this external ID
//  3. If startTimeUnix > 0, also checking for a pending pre-record (handles the Create race
//     condition where Hevy fires the webhook before we've written the real record ID)
func IsBounceback(
	ctx context.Context,
	store UploadedActivityStore,
	userId string,
	source pbactivity.ActivitySource,
	externalId string,
	startTimeUnix int64,
) (bool, error) {
	correspondingDest := GetCorrespondingDestination(source)
	if correspondingDest == pbplugin.DestinationType_DESTINATION_UNSPECIFIED {
		// Source has no destination counterpart, not a bounceback scenario
		return false, nil
	}

	// Primary check: when a webhook arrives from Hevy with hevy_workout_123 we look
	// for a record stored as DESTINATION_HEVY:hevy_workout_123
	record, err := store.GetUploadedActivity(ctx, userId, correspondingDest, externalId)
	if err != nil {
		return false, fmt.Errorf("failed to check uploaded activity: %w", err)
	}
	if record != nil {
		return true, nil
	}

	// Secondary check: pending pre-record written before the destination POST (Create
	// race condition — the real Hevy workout ID isn't known until after the response,
	// but Hevy may fire the webhook before we write the real record).
	if startTimeUnix > 0 {
		pendingId := fmt.Sprintf("pending:%d", startTimeUnix)
		pendingRecord, pendingErr := store.GetUploadedActivity(ctx, userId, correspondingDest, pendingId)
		if pendingErr != nil {
			return false, fmt.Errorf("failed to check pending uploaded activity: %w", pendingErr)
		}
		if pendingRecord != nil {
			return true, nil
		}
	}

	return false, nil
}

// BuildUploadedActivityID creates a composite ID for an uploaded activity record.
// Format: "{destination}:{destination_id}" for efficient lookup.
//
// This is the KEY insight: when Hevy sends a webhook, the external ID will be
// Hevy's workout ID. So we store records using the destination's ID, not the source's.
func BuildUploadedActivityID(destination pbplugin.DestinationType, destinationId string) string {
	// Use lowercase destination name without prefix
	destName := strings.TrimPrefix(destination.String(), "DESTINATION_")
	return fmt.Sprintf("%s:%s", strings.ToLower(destName), destinationId)
}
