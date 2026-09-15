package activity

import (
	"context"
	"testing"

	"google.golang.org/protobuf/encoding/protojson"
	"google.golang.org/protobuf/proto"

	pbactivity "github.com/fitglue/server/src/go/pkg/types/pb/models/activity"
	pbpipeline "github.com/fitglue/server/src/go/pkg/types/pb/models/pipeline"
	pbsvc "github.com/fitglue/server/src/go/pkg/types/pb/services/activity"
)

// layeredRecord builds a small ActivityRecord whose resolved fields come from three
// different layers: the name from the user overlay, the type from the source, and the
// description from an enricher — so provenance attribution can be asserted end to end.
func layeredRecord() *pbactivity.ActivityRecord {
	return &pbactivity.ActivityRecord{
		ActivityId: "a1",
		UserId:     "u1",
		SourceLayer: &pbactivity.ActivitySourceLayer{
			Parsed: &pbactivity.StandardizedActivity{
				Name: "Morning Run",
				Type: pbactivity.ActivityType_ACTIVITY_TYPE_RUN,
			},
		},
		EnricherLayers: []*pbactivity.EnricherRunLayer{
			{
				ProviderName: "ai_description",
				ExecutionId:  "exec-1",
				Contribution: &pbactivity.EnricherContribution{
					Description: proto.String("A brisk 5k."),
				},
			},
		},
		UserEditOverlay: &pbactivity.ActivityUserEditOverlay{
			Name: proto.String("My Title"),
		},
		DerivedActivity: &pbactivity.StandardizedActivity{
			Name:        "Morning Run",
			Type:        pbactivity.ActivityType_ACTIVITY_TYPE_RUN,
			Description: "A brisk 5k.",
		},
	}
}

func provenanceFor(prov []*pbactivity.FieldProvenance, field string) *pbactivity.FieldProvenance {
	for _, p := range prov {
		if p.GetField() == field {
			return p
		}
	}
	return nil
}

func TestGetResolvedActivity(t *testing.T) {
	ctx := context.Background()
	recBytes, err := protojson.Marshal(layeredRecord())
	if err != nil {
		t.Fatalf("marshal record: %v", err)
	}

	store := &MockActivityStore{
		GetPipelineRunFunc: func(ctx context.Context, userID, runID string) (*pbpipeline.PipelineRun, error) {
			if runID == "missing" {
				return nil, nil
			}
			return &pbpipeline.PipelineRun{
				ActivityId:        runID,
				ActivityRecordUri: "gs://test-bucket/activity_records/u1/a1.json",
			}, nil
		},
	}
	blob := &MockBlobStore{
		GetFunc: func(ctx context.Context, bucket, object string) ([]byte, error) {
			return recBytes, nil
		},
	}
	svc := newTestSvc(store, blob)

	res, err := svc.GetResolvedActivity(ctx, &pbsvc.GetResolvedActivityRequest{UserId: "u1", ActivityId: "a1"})
	if err != nil {
		t.Fatalf("expected success, got %v", err)
	}

	// Effective activity: overlay name wins over the derived value.
	if got := res.GetActivity().GetName(); got != "My Title" {
		t.Errorf("expected overlaid name 'My Title', got %q", got)
	}

	// Provenance attributes each field to the layer that set it.
	if p := provenanceFor(res.GetProvenance(), "name"); p == nil || p.GetLayer() != pbactivity.ProvenanceLayer_PROVENANCE_LAYER_USER_OVERLAY {
		t.Errorf("expected name provenance USER_OVERLAY, got %v", p)
	}
	if p := provenanceFor(res.GetProvenance(), "type"); p == nil || p.GetLayer() != pbactivity.ProvenanceLayer_PROVENANCE_LAYER_SOURCE {
		t.Errorf("expected type provenance SOURCE, got %v", p)
	}
	if p := provenanceFor(res.GetProvenance(), "description"); p == nil || p.GetLayer() != pbactivity.ProvenanceLayer_PROVENANCE_LAYER_ENRICHER {
		t.Errorf("expected description provenance ENRICHER, got %v", p)
	} else if p.GetProviderName() != "ai_description" {
		t.Errorf("expected description provider 'ai_description', got %q", p.GetProviderName())
	}
}

func TestGetResolvedActivityValidation(t *testing.T) {
	svc := newTestSvc(&MockActivityStore{}, &MockBlobStore{})

	if _, err := svc.GetResolvedActivity(context.Background(), &pbsvc.GetResolvedActivityRequest{ActivityId: "a1"}); err == nil {
		t.Error("expected error when user_id is missing")
	}

	store := &MockActivityStore{
		GetPipelineRunFunc: func(ctx context.Context, userID, runID string) (*pbpipeline.PipelineRun, error) {
			return nil, nil // not found
		},
	}
	svc = newTestSvc(store, &MockBlobStore{})
	if _, err := svc.GetResolvedActivity(context.Background(), &pbsvc.GetResolvedActivityRequest{UserId: "u1", ActivityId: "missing"}); err == nil {
		t.Error("expected NotFound error for missing activity")
	}
}

// TestListResolvedActivities covers both a layered run (with provenance) and a legacy run
// with no record URI (falls back to the effective activity with empty provenance).
func TestListResolvedActivities(t *testing.T) {
	ctx := context.Background()
	recBytes, _ := protojson.Marshal(layeredRecord())

	store := &MockActivityStore{
		ListPipelineRunsFunc: func(ctx context.Context, userID string, limit int32, pageToken string) ([]*pbpipeline.PipelineRun, string, error) {
			return []*pbpipeline.PipelineRun{
				{ActivityId: "a1", ActivityRecordUri: "gs://test-bucket/activity_records/u1/a1.json"},
				{ActivityId: "legacy", Source: "SOURCE_STRAVA", Title: "Legacy Ride"},
			}, "next-token", nil
		},
	}
	blob := &MockBlobStore{
		GetFunc: func(ctx context.Context, bucket, object string) ([]byte, error) {
			return recBytes, nil
		},
	}
	svc := newTestSvc(store, blob)

	res, err := svc.ListResolvedActivities(ctx, &pbsvc.ListResolvedActivitiesRequest{UserId: "u1", Limit: 10})
	if err != nil {
		t.Fatalf("expected success, got %v", err)
	}
	if len(res.GetActivities()) != 2 {
		t.Fatalf("expected 2 resolved activities, got %d", len(res.GetActivities()))
	}
	if res.GetNextPageToken() != "next-token" {
		t.Errorf("expected next page token to be threaded through, got %q", res.GetNextPageToken())
	}

	// First is the layered run — has provenance.
	if len(res.GetActivities()[0].GetProvenance()) == 0 {
		t.Error("expected layered run to carry provenance")
	}
	// Second is the legacy run — effective activity, empty provenance.
	legacy := res.GetActivities()[1]
	if legacy.GetActivity().GetName() != "Legacy Ride" {
		t.Errorf("expected legacy activity name 'Legacy Ride', got %q", legacy.GetActivity().GetName())
	}
	if len(legacy.GetProvenance()) != 0 {
		t.Errorf("expected legacy run to carry no provenance, got %d entries", len(legacy.GetProvenance()))
	}
}
