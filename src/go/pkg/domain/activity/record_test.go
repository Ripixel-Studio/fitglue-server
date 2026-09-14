package activity

import (
	"testing"

	pbactivity "github.com/fitglue/server/src/go/pkg/types/pb/models/activity"
	"google.golang.org/protobuf/proto"
)

func TestApplyUserEditOverlay_OverridesOnlySetFields(t *testing.T) {
	base := &pbactivity.StandardizedActivity{
		Name:        "Morning Run",
		Description: "derived description",
		Type:        pbactivity.ActivityType_ACTIVITY_TYPE_RUN,
		Tags:        []string{"derived-tag"},
	}
	overlay := &pbactivity.ActivityUserEditOverlay{
		Name: proto.String("My Custom Title"),
		Tags: []string{"user-tag-a", "user-tag-b"},
	}

	ApplyUserEditOverlay(base, overlay)

	if base.Name != "My Custom Title" {
		t.Errorf("name not overridden: got %q", base.Name)
	}
	// Description was not set on the overlay — must fall through to the derived value.
	if base.Description != "derived description" {
		t.Errorf("description should be unchanged: got %q", base.Description)
	}
	// Type was not set on the overlay — must fall through.
	if base.Type != pbactivity.ActivityType_ACTIVITY_TYPE_RUN {
		t.Errorf("type should be unchanged: got %v", base.Type)
	}
	// Tags were set — the user's set replaces the derived tags.
	if len(base.Tags) != 2 || base.Tags[0] != "user-tag-a" || base.Tags[1] != "user-tag-b" {
		t.Errorf("tags not replaced by overlay: got %v", base.Tags)
	}
}

func TestApplyUserEditOverlay_EmptyStringOverrideIsHonoured(t *testing.T) {
	base := &pbactivity.StandardizedActivity{Description: "derived"}
	// A user clearing the description sets an explicit empty string (pointer non-nil).
	overlay := &pbactivity.ActivityUserEditOverlay{Description: proto.String("")}

	ApplyUserEditOverlay(base, overlay)

	if base.Description != "" {
		t.Errorf("explicit empty-string override not honoured: got %q", base.Description)
	}
}

func TestApplyUserEditOverlay_NilOverlayIsNoop(t *testing.T) {
	base := &pbactivity.StandardizedActivity{Name: "unchanged"}
	ApplyUserEditOverlay(base, nil)
	if base.Name != "unchanged" {
		t.Errorf("nil overlay mutated base: got %q", base.Name)
	}
}

func TestEffectiveActivity_AppliesOverlayWithoutMutatingRecord(t *testing.T) {
	rec := &pbactivity.ActivityRecord{
		DerivedActivity: &pbactivity.StandardizedActivity{
			Name: "Derived Name",
			Type: pbactivity.ActivityType_ACTIVITY_TYPE_RUN,
		},
		UserEditOverlay: &pbactivity.ActivityUserEditOverlay{
			Name: proto.String("Overlaid Name"),
		},
	}

	eff := EffectiveActivity(rec)
	if eff == nil {
		t.Fatal("expected an effective activity")
	}
	if eff.Name != "Overlaid Name" {
		t.Errorf("overlay not applied: got %q", eff.Name)
	}
	// The record's derived activity must not be mutated by the read.
	if rec.DerivedActivity.Name != "Derived Name" {
		t.Errorf("EffectiveActivity mutated the stored derived activity: got %q", rec.DerivedActivity.Name)
	}
}

func TestEffectiveActivity_NilInputs(t *testing.T) {
	if EffectiveActivity(nil) != nil {
		t.Error("expected nil for nil record")
	}
	if EffectiveActivity(&pbactivity.ActivityRecord{}) != nil {
		t.Error("expected nil when derived activity is absent")
	}
}

func TestActivityRecordPathHelpers(t *testing.T) {
	if got := ActivityRecordObjectPath("user1", "act1"); got != "activity_records/user1/act1.json" {
		t.Errorf("unexpected object path: %q", got)
	}
	if got := ActivityRecordURI("my-bucket", "user1", "act1"); got != "gs://my-bucket/activity_records/user1/act1.json" {
		t.Errorf("unexpected uri: %q", got)
	}
}
