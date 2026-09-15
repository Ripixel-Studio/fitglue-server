package activity

import (
	"context"
	"fmt"
	"time"

	"google.golang.org/protobuf/encoding/protojson"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/timestamppb"

	shared "github.com/fitglue/server/src/go/pkg"
	pbactivity "github.com/fitglue/server/src/go/pkg/types/pb/models/activity"
)

// EditableOverlayFields are the field names UpdateActivity accepts in its update mask.
// They mirror the mutable fields of ActivityUserEditOverlay (the highest-precedence layer
// of the resolution stack). time_markers and hybrid_race_summary are enricher-owned and
// are deliberately not user-editable via the overlay (see resolveProvenance).
var EditableOverlayFields = map[string]bool{
	FieldName:        true,
	FieldDescription: true,
	FieldType:        true,
	FieldTags:        true,
}

// ApplyOverlayEdit mutates rec's user-edit overlay in place, applying only the fields named
// in the mask from edit. It implements the edit half of the correction loop: each named
// field either sets/replaces the user override (when edit carries a value) or clears it (when
// edit leaves that field unset), so a later read resolves the field from the derived layers
// again. Fields not named in the mask are left exactly as stored.
//
// Semantics per field:
//   - "name" / "description": edit.Name/edit.Description non-nil sets the override (an
//     explicit empty string is a valid override); nil clears it.
//   - "type": edit.Type non-nil and != UNSPECIFIED sets the override; nil or UNSPECIFIED
//     clears it.
//   - "tags": a non-empty edit.Tags replaces the tag override; an empty/omitted list clears
//     it. An explicitly empty (but present) tag set is not distinguishable from clearing.
//
// stamps edited_at with now when at least one field is applied. Returns an error for an
// empty mask or an unknown field name (the caller maps this to InvalidArgument), leaving rec
// untouched in that case.
func ApplyOverlayEdit(rec *pbactivity.ActivityRecord, edit *pbactivity.ActivityUserEditOverlay, fields []string, now time.Time) error {
	if rec == nil {
		return fmt.Errorf("nil activity record")
	}
	if len(fields) == 0 {
		return fmt.Errorf("update_fields must name at least one field")
	}
	for _, f := range fields {
		if !EditableOverlayFields[f] {
			return fmt.Errorf("unknown or non-editable update field %q (editable: name, description, type, tags)", f)
		}
	}
	if edit == nil {
		edit = &pbactivity.ActivityUserEditOverlay{}
	}
	if rec.UserEditOverlay == nil {
		rec.UserEditOverlay = &pbactivity.ActivityUserEditOverlay{}
	}
	ov := rec.UserEditOverlay

	for _, f := range fields {
		switch f {
		case FieldName:
			if edit.Name != nil {
				ov.Name = proto.String(edit.GetName())
			} else {
				ov.Name = nil
			}
		case FieldDescription:
			if edit.Description != nil {
				ov.Description = proto.String(edit.GetDescription())
			} else {
				ov.Description = nil
			}
		case FieldType:
			if edit.Type != nil && edit.GetType() != pbactivity.ActivityType_ACTIVITY_TYPE_UNSPECIFIED {
				t := edit.GetType()
				ov.Type = &t
			} else {
				ov.Type = nil
			}
		case FieldTags:
			if len(edit.Tags) > 0 {
				ov.Tags = append([]string(nil), edit.Tags...)
			} else {
				ov.Tags = nil
			}
		}
	}

	ov.EditedAt = timestamppb.New(now)
	rec.UpdatedAt = timestamppb.New(now)
	return nil
}

// WriteActivityRecord marshals rec and writes it back to the GCS object identified by uri.
// It is the write-side counterpart of LoadActivityRecord: same protojson encoding, same
// gs://bucket/object addressing, so a record round-trips losslessly through GetActivity /
// UpdateActivity. Returns an error for an empty or malformed uri.
func WriteActivityRecord(ctx context.Context, uri string, store shared.BlobStore, rec *pbactivity.ActivityRecord) error {
	if uri == "" {
		return fmt.Errorf("empty activity_record_uri")
	}
	if rec == nil {
		return fmt.Errorf("nil activity record")
	}
	bucket, object, ok := ParseGCSURI(uri)
	if !ok {
		return fmt.Errorf("invalid activity_record_uri: %s", uri)
	}
	data, err := protojson.Marshal(rec)
	if err != nil {
		return fmt.Errorf("failed to marshal activity record: %w", err)
	}
	if err := store.Write(ctx, bucket, object, data); err != nil {
		return fmt.Errorf("failed to write activity record to GCS: %w", err)
	}
	return nil
}
