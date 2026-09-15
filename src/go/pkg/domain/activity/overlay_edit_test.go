package activity

import (
	"testing"
	"time"

	pbactivity "github.com/fitglue/server/src/go/pkg/types/pb/models/activity"
	"google.golang.org/protobuf/proto"
)

func TestApplyOverlayEdit_SetsOnlyNamedFields(t *testing.T) {
	rec := &pbactivity.ActivityRecord{
		UserEditOverlay: &pbactivity.ActivityUserEditOverlay{
			Description: proto.String("existing desc override"),
		},
	}
	edit := &pbactivity.ActivityUserEditOverlay{
		Name: proto.String("New Title"),
		Type: pbactivity.ActivityType_ACTIVITY_TYPE_RIDE.Enum(),
		Tags: []string{"a", "b"},
	}

	now := time.Unix(1700000000, 0)
	if err := ApplyOverlayEdit(rec, edit, []string{FieldName, FieldType, FieldTags}, now); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	ov := rec.UserEditOverlay
	if ov.GetName() != "New Title" {
		t.Errorf("name not set: %q", ov.GetName())
	}
	if ov.GetType() != pbactivity.ActivityType_ACTIVITY_TYPE_RIDE {
		t.Errorf("type not set: %v", ov.GetType())
	}
	if len(ov.Tags) != 2 || ov.Tags[0] != "a" {
		t.Errorf("tags not set: %v", ov.Tags)
	}
	// description was NOT in the mask — must be left exactly as stored.
	if ov.GetDescription() != "existing desc override" {
		t.Errorf("unmasked description was mutated: %q", ov.GetDescription())
	}
	if !ov.GetEditedAt().AsTime().Equal(now) {
		t.Errorf("edited_at not stamped: %v", ov.GetEditedAt().AsTime())
	}
}

func TestApplyOverlayEdit_ClearsOverrideWhenValueUnset(t *testing.T) {
	rec := &pbactivity.ActivityRecord{
		UserEditOverlay: &pbactivity.ActivityUserEditOverlay{
			Name: proto.String("old override"),
			Tags: []string{"x"},
		},
	}
	// Naming a field with no value in the edit clears that override.
	edit := &pbactivity.ActivityUserEditOverlay{}
	if err := ApplyOverlayEdit(rec, edit, []string{FieldName, FieldTags}, time.Unix(1, 0)); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if rec.UserEditOverlay.Name != nil {
		t.Errorf("name override not cleared: %v", rec.UserEditOverlay.Name)
	}
	if rec.UserEditOverlay.Tags != nil {
		t.Errorf("tags override not cleared: %v", rec.UserEditOverlay.Tags)
	}
}

func TestApplyOverlayEdit_EmptyStringIsAValidOverride(t *testing.T) {
	rec := &pbactivity.ActivityRecord{}
	edit := &pbactivity.ActivityUserEditOverlay{Description: proto.String("")}
	if err := ApplyOverlayEdit(rec, edit, []string{FieldDescription}, time.Unix(1, 0)); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if rec.UserEditOverlay.Description == nil || rec.UserEditOverlay.GetDescription() != "" {
		t.Errorf("explicit empty-string override not honoured: %v", rec.UserEditOverlay.Description)
	}
}

func TestApplyOverlayEdit_Rejects(t *testing.T) {
	rec := &pbactivity.ActivityRecord{}
	if err := ApplyOverlayEdit(rec, &pbactivity.ActivityUserEditOverlay{}, nil, time.Unix(1, 0)); err == nil {
		t.Error("expected error for empty mask")
	}
	if err := ApplyOverlayEdit(rec, &pbactivity.ActivityUserEditOverlay{}, []string{"time_markers"}, time.Unix(1, 0)); err == nil {
		t.Error("expected error for non-editable field")
	}
	// A rejected call must not have mutated the overlay.
	if rec.UserEditOverlay != nil {
		t.Errorf("overlay mutated on rejected edit: %v", rec.UserEditOverlay)
	}
}
