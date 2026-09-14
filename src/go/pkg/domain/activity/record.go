package activity

import (
	"context"
	"fmt"
	"time"

	"google.golang.org/protobuf/encoding/protojson"
	"google.golang.org/protobuf/proto"

	shared "github.com/fitglue/server/src/go/pkg"
	pbactivity "github.com/fitglue/server/src/go/pkg/types/pb/models/activity"
)

// ActivityRecordSchemaVersion is the current on-disk version of the layered
// ActivityRecord. Bump when the record structure changes in a way that older
// readers need to migrate.
const ActivityRecordSchemaVersion = 1

// RawPayloadRetentionDays is how long the raw provider payload
// (source_layer.raw_payload_uri, written under the payloads/ prefix) is kept before
// it is pruned and reprocessing relies on a re-pull from the source. Spec DECISION 1a.
// The parsed source and enricher outputs held in the ActivityRecord itself are kept
// indefinitely. Deletion of the raw payload is enforced by the GCS lifecycle rule on
// the payloads/ prefix (see terraform/storage.tf); this constant sets the informational
// raw_payload_expires_at stamped onto the record and the pipeline run.
const RawPayloadRetentionDays = 30

// RawPayloadRetention is RawPayloadRetentionDays expressed as a duration.
const RawPayloadRetention = RawPayloadRetentionDays * 24 * time.Hour

// ActivityRecordObjectPath returns the GCS object path (within the artifact bucket)
// where a user's layered ActivityRecord is stored. Stable per activity so a resume or
// re-enrichment loads and appends to the same record rather than orphaning a new blob.
func ActivityRecordObjectPath(userID, activityID string) string {
	return fmt.Sprintf("activity_records/%s/%s.json", userID, activityID)
}

// ActivityRecordURI returns the full gs:// URI for a user's layered ActivityRecord.
func ActivityRecordURI(bucket, userID, activityID string) string {
	return fmt.Sprintf("gs://%s/%s", bucket, ActivityRecordObjectPath(userID, activityID))
}

// LoadActivityRecord fetches and unmarshals a layered ActivityRecord from GCS.
// Returns (nil, nil) when uri is empty so callers can treat "no record yet" as a
// non-error (e.g. the first run for an activity, or a legacy pre-layered run).
func LoadActivityRecord(ctx context.Context, uri string, store shared.BlobStore) (*pbactivity.ActivityRecord, error) {
	if uri == "" {
		return nil, nil
	}
	bucket, object, ok := ParseGCSURI(uri)
	if !ok {
		return nil, fmt.Errorf("invalid activity_record_uri: %s", uri)
	}
	data, err := store.Get(ctx, bucket, object)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch activity record from GCS: %w", err)
	}
	var rec pbactivity.ActivityRecord
	if err := protojson.Unmarshal(data, &rec); err != nil {
		return nil, fmt.Errorf("failed to unmarshal activity record: %w", err)
	}
	return &rec, nil
}

// EffectiveActivity returns the user-facing view of a layered record: the derived
// activity (source folded through every enricher) with the mutable user-edit overlay
// applied on top. The record is not modified. Returns nil if the record or its derived
// activity is nil.
func EffectiveActivity(rec *pbactivity.ActivityRecord) *pbactivity.StandardizedActivity {
	if rec == nil || rec.DerivedActivity == nil {
		return nil
	}
	act := proto.Clone(rec.DerivedActivity).(*pbactivity.StandardizedActivity)
	ApplyUserEditOverlay(act, rec.UserEditOverlay)
	return act
}

// ApplyUserEditOverlay mutates base in place, applying each set field of the overlay.
// Unset overlay fields fall through to the derived value. Tags, when present on the
// overlay, replace the derived tags (the user's edit is the authoritative tag set).
func ApplyUserEditOverlay(base *pbactivity.StandardizedActivity, overlay *pbactivity.ActivityUserEditOverlay) {
	if base == nil || overlay == nil {
		return
	}
	if overlay.Name != nil {
		base.Name = overlay.GetName()
	}
	if overlay.Description != nil {
		base.Description = overlay.GetDescription()
	}
	if overlay.Type != nil {
		base.Type = overlay.GetType()
	}
	if overlay.Tags != nil {
		base.Tags = append([]string(nil), overlay.Tags...)
	}
}
