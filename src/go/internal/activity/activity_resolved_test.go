package activity

import (
	"context"
	"testing"

	"google.golang.org/protobuf/encoding/protojson"
	"google.golang.org/protobuf/proto"

	"github.com/fitglue/server/src/go/internal/infra"
	pbactivity "github.com/fitglue/server/src/go/pkg/types/pb/models/activity"
	pbpipeline "github.com/fitglue/server/src/go/pkg/types/pb/models/pipeline"
	pbsvc "github.com/fitglue/server/src/go/pkg/types/pb/services/activity"
)

// layeredRecord builds a small layered ActivityRecord exercising all three layers:
// source sets the type, an enricher replaces the name, and the user overlay sets the
// description. It is returned protojson-encoded, as it is stored in GCS.
func layeredRecord(t *testing.T, activityID string) []byte {
	t.Helper()
	rec := &pbactivity.ActivityRecord{
		ActivityId: activityID,
		UserId:     "u1",
		SourceLayer: &pbactivity.ActivitySourceLayer{
			Parsed: &pbactivity.StandardizedActivity{
				Name: "Morning Run",
				Type: pbactivity.ActivityType_ACTIVITY_TYPE_RUN,
			},
		},
		EnricherLayers: []*pbactivity.EnricherRunLayer{
			{
				ProviderName: "ai_title",
				ExecutionId:  "exec-1",
				Contribution: &pbactivity.EnricherContribution{Name: proto.String("Epic AI Title")},
			},
		},
		UserEditOverlay: &pbactivity.ActivityUserEditOverlay{
			Description: proto.String("user-written description"),
		},
		// Derived = source folded through enrichers (pre-overlay).
		DerivedActivity: &pbactivity.StandardizedActivity{
			Name: "Epic AI Title",
			Type: pbactivity.ActivityType_ACTIVITY_TYPE_RUN,
		},
	}
	data, err := protojson.Marshal(rec)
	if err != nil {
		t.Fatalf("failed to marshal record: %v", err)
	}
	return data
}

func provFor(provs []*pbactivity.FieldProvenance, field string) *pbactivity.FieldProvenance {
	for _, p := range provs {
		if p.GetField() == field {
			return p
		}
	}
	return nil
}

func TestGetResolvedActivity_WithRecord(t *testing.T) {
	ctx := context.Background()
	logger := infra.NewLogger()

	store := &MockActivityStore{
		GetPipelineRunFunc: func(ctx context.Context, userID, runID string) (*pbpipeline.PipelineRun, error) {
			return &pbpipeline.PipelineRun{
				ActivityId:        runID,
				ActivityRecordUri: "gs://test-bucket/activity_records/u1/a1.json",
			}, nil
		},
	}
	blobStore := &MockBlobStore{
		GetFunc: func(ctx context.Context, bucket, object string) ([]byte, error) {
			return layeredRecord(t, "a1"), nil
		},
	}
	svc := NewService(store, blobStore, nil, "test-bucket", "test-showcase-bucket", logger)

	res, err := svc.GetResolvedActivity(ctx, &pbsvc.GetResolvedActivityRequest{UserId: "u1", ActivityId: "a1"})
	if err != nil {
		t.Fatalf("expected success, got %v", err)
	}
	if res.GetActivity().GetName() != "Epic AI Title" {
		t.Errorf("expected resolved name from enricher, got %q", res.GetActivity().GetName())
	}
	if res.GetActivity().GetDescription() != "user-written description" {
		t.Errorf("expected resolved description from overlay, got %q", res.GetActivity().GetDescription())
	}

	nameProv := provFor(res.GetProvenance(), "name")
	if nameProv == nil || nameProv.GetLayer() != pbactivity.ProvenanceLayer_PROVENANCE_LAYER_ENRICHER {
		t.Fatalf("expected name provenance from enricher, got %+v", nameProv)
	}
	if nameProv.GetProviderName() != "ai_title" {
		t.Errorf("expected name provenance provider ai_title, got %q", nameProv.GetProviderName())
	}
	if typeProv := provFor(res.GetProvenance(), "type"); typeProv == nil || typeProv.GetLayer() != pbactivity.ProvenanceLayer_PROVENANCE_LAYER_SOURCE {
		t.Errorf("expected type provenance from source, got %+v", typeProv)
	}
	if descProv := provFor(res.GetProvenance(), "description"); descProv == nil || descProv.GetLayer() != pbactivity.ProvenanceLayer_PROVENANCE_LAYER_USER_OVERLAY {
		t.Errorf("expected description provenance from user overlay, got %+v", descProv)
	}
}

// A run with no layered record (pre-layered / legacy) still resolves: the activity is
// reconstructed via the GetActivity fallback and provenance is empty.
func TestGetResolvedActivity_LegacyFallback(t *testing.T) {
	ctx := context.Background()
	logger := infra.NewLogger()

	store := &MockActivityStore{
		GetPipelineRunFunc: func(ctx context.Context, userID, runID string) (*pbpipeline.PipelineRun, error) {
			return &pbpipeline.PipelineRun{
				ActivityId: runID,
				Source:     "SOURCE_STRAVA",
				Title:      "Legacy Activity",
				Type:       pbactivity.ActivityType_ACTIVITY_TYPE_RUN,
				// No ActivityRecordUri, no EnrichedEventUri: built from run metadata.
			}, nil
		},
	}
	svc := NewService(store, &MockBlobStore{}, nil, "test-bucket", "test-showcase-bucket", logger)

	res, err := svc.GetResolvedActivity(ctx, &pbsvc.GetResolvedActivityRequest{UserId: "u1", ActivityId: "a1"})
	if err != nil {
		t.Fatalf("expected success, got %v", err)
	}
	if res.GetActivity().GetName() != "Legacy Activity" {
		t.Errorf("expected reconstructed name, got %q", res.GetActivity().GetName())
	}
	if len(res.GetProvenance()) != 0 {
		t.Errorf("expected empty provenance for legacy activity, got %d entries", len(res.GetProvenance()))
	}
}

func TestGetResolvedActivity_NotFound(t *testing.T) {
	ctx := context.Background()
	logger := infra.NewLogger()

	store := &MockActivityStore{
		GetPipelineRunFunc: func(ctx context.Context, userID, runID string) (*pbpipeline.PipelineRun, error) {
			return nil, nil
		},
	}
	svc := NewService(store, &MockBlobStore{}, nil, "test-bucket", "test-showcase-bucket", logger)

	if _, err := svc.GetResolvedActivity(ctx, &pbsvc.GetResolvedActivityRequest{UserId: "u1", ActivityId: "missing"}); err == nil {
		t.Fatal("expected NotFound error for missing activity")
	}
}

func TestGetResolvedActivity_Validation(t *testing.T) {
	svc := NewService(&MockActivityStore{}, &MockBlobStore{}, nil, "b", "sb", infra.NewLogger())
	if _, err := svc.GetResolvedActivity(context.Background(), &pbsvc.GetResolvedActivityRequest{UserId: "u1"}); err == nil {
		t.Fatal("expected InvalidArgument when activity_id is missing")
	}
}

// ListResolvedActivities mixes a layered run (resolved with provenance) with a legacy run
// (lightweight, no provenance), and passes through the pagination token.
func TestListResolvedActivities(t *testing.T) {
	ctx := context.Background()
	logger := infra.NewLogger()

	store := &MockActivityStore{
		ListPipelineRunsFunc: func(ctx context.Context, userID string, limit int32, pageToken string) ([]*pbpipeline.PipelineRun, string, error) {
			return []*pbpipeline.PipelineRun{
				{
					Id:                "run-layered",
					ActivityId:        "a1",
					ActivityRecordUri: "gs://test-bucket/activity_records/u1/a1.json",
				},
				{
					Id:         "run-legacy",
					ActivityId: "a2",
					Source:     "SOURCE_HEVY",
					Title:      "Legacy Lift",
					Type:       pbactivity.ActivityType_ACTIVITY_TYPE_WEIGHT_TRAINING,
				},
			}, "next-token", nil
		},
	}
	blobStore := &MockBlobStore{
		GetFunc: func(ctx context.Context, bucket, object string) ([]byte, error) {
			return layeredRecord(t, "a1"), nil
		},
	}
	svc := NewService(store, blobStore, nil, "test-bucket", "test-showcase-bucket", logger)

	res, err := svc.ListResolvedActivities(ctx, &pbsvc.ListResolvedActivitiesRequest{UserId: "u1", Limit: 10})
	if err != nil {
		t.Fatalf("expected success, got %v", err)
	}
	if len(res.GetActivities()) != 2 {
		t.Fatalf("expected 2 resolved activities, got %d", len(res.GetActivities()))
	}
	if res.GetNextPageToken() != "next-token" {
		t.Errorf("expected pagination token to pass through, got %q", res.GetNextPageToken())
	}

	// First entry: resolved from the layered record, provenance present.
	first := res.GetActivities()[0]
	if first.GetActivity().GetName() != "Epic AI Title" {
		t.Errorf("expected resolved name for layered run, got %q", first.GetActivity().GetName())
	}
	if provFor(first.GetProvenance(), "name") == nil {
		t.Error("expected name provenance for layered run")
	}

	// Second entry: legacy lightweight, no provenance.
	second := res.GetActivities()[1]
	if second.GetActivity().GetName() != "Legacy Lift" {
		t.Errorf("expected legacy name, got %q", second.GetActivity().GetName())
	}
	if len(second.GetProvenance()) != 0 {
		t.Errorf("expected no provenance for legacy run, got %d", len(second.GetProvenance()))
	}
}

func TestListResolvedActivities_Validation(t *testing.T) {
	svc := NewService(&MockActivityStore{}, &MockBlobStore{}, nil, "b", "sb", infra.NewLogger())
	if _, err := svc.ListResolvedActivities(context.Background(), &pbsvc.ListResolvedActivitiesRequest{}); err == nil {
		t.Fatal("expected InvalidArgument when user_id is missing")
	}
}
