package activity

import (
	"testing"

	pbactivity "github.com/fitglue/server/src/go/pkg/types/pb/models/activity"
	"google.golang.org/protobuf/proto"
)

func TestApplyEnricherContribution_FoldsSetFields(t *testing.T) {
	base := &pbactivity.StandardizedActivity{
		Name:        "Morning Run",
		Description: "base section",
		Type:        pbactivity.ActivityType_ACTIVITY_TYPE_RUN,
		Tags:        []string{"existing"},
	}
	c := &pbactivity.EnricherContribution{
		NameSuffix:  proto.String(" (#5)"),
		Tags:        []string{"sunny"},
		Description: proto.String("Weather: 20°C"),
	}

	ApplyEnricherContribution(base, c)

	if base.Name != "Morning Run (#5)" {
		t.Errorf("name suffix not appended: got %q", base.Name)
	}
	// Description sections are joined with a blank line, matching the orchestrator.
	if base.Description != "base section\n\nWeather: 20°C" {
		t.Errorf("description not appended as a section: got %q", base.Description)
	}
	if len(base.Tags) != 2 || base.Tags[1] != "sunny" {
		t.Errorf("tags not appended: got %v", base.Tags)
	}
	// Type was not set on the contribution — must be untouched.
	if base.Type != pbactivity.ActivityType_ACTIVITY_TYPE_RUN {
		t.Errorf("type should be unchanged: got %v", base.Type)
	}
}

func TestApplyEnricherContribution_ReplacesNameAndType(t *testing.T) {
	base := &pbactivity.StandardizedActivity{Name: "old", Type: pbactivity.ActivityType_ACTIVITY_TYPE_RUN}
	c := &pbactivity.EnricherContribution{
		Name: proto.String("new name"),
		Type: pbactivity.ActivityType_ACTIVITY_TYPE_RIDE.Enum(),
	}

	ApplyEnricherContribution(base, c)

	if base.Name != "new name" {
		t.Errorf("name not replaced: got %q", base.Name)
	}
	if base.Type != pbactivity.ActivityType_ACTIVITY_TYPE_RIDE {
		t.Errorf("type not replaced: got %v", base.Type)
	}
}

func TestApplyEnricherContribution_SetsDescriptionWhenBaseEmpty(t *testing.T) {
	base := &pbactivity.StandardizedActivity{}
	ApplyEnricherContribution(base, &pbactivity.EnricherContribution{Description: proto.String("only section")})
	if base.Description != "only section" {
		t.Errorf("description should be set verbatim when base empty: got %q", base.Description)
	}
}

func TestApplyEnricherContribution_NilInputs(t *testing.T) {
	// Must not panic.
	ApplyEnricherContribution(nil, &pbactivity.EnricherContribution{})
	base := &pbactivity.StandardizedActivity{Name: "keep"}
	ApplyEnricherContribution(base, nil)
	if base.Name != "keep" {
		t.Errorf("nil contribution must be a no-op: got %q", base.Name)
	}
}
