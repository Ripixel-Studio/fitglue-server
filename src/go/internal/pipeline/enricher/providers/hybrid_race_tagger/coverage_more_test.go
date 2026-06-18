package hybrid_race_tagger

import (
	"context"
	"log/slog"
	"testing"

	"github.com/fitglue/server/src/go/internal/pipeline/enricher/providers/user_input"
	"github.com/fitglue/server/src/go/pkg/bootstrap"
	user "github.com/fitglue/server/src/go/pkg/domain/user"
	"github.com/fitglue/server/src/go/pkg/testing/mocks"
	pbactivity "github.com/fitglue/server/src/go/pkg/types/pb/models/activity"
	pbpipeline "github.com/fitglue/server/src/go/pkg/types/pb/models/pipeline"
	pbplugin "github.com/fitglue/server/src/go/pkg/types/pb/models/plugin"
	pbuser "github.com/fitglue/server/src/go/pkg/types/pb/models/user"
)

func hrtUser() *user.Record {
	return &user.Record{UserProfile: &pbuser.UserProfile{UserId: "u1"}}
}

// hyroxLikeSession builds a session passing the structural heuristic (16 laps, ~9.7km).
func hyroxLikeSession() *pbactivity.Session {
	laps := make([]*pbactivity.Lap, 16)
	for i := range laps {
		laps[i] = makeLap(600, 200, 0)
	}
	return &pbactivity.Session{Laps: laps, TotalDistance: 9700}
}

func TestHRT_SetService_NameType(t *testing.T) {
	p := &HybridRaceTaggerProvider{}
	p.SetService(nil)
	if p.service != nil {
		t.Error("expected nil service")
	}
	svc := &bootstrap.Service{}
	p.SetService(svc)
	if p.service != svc {
		t.Error("expected service set")
	}
	if p.Name() != "hybrid_race_tagger" {
		t.Errorf("unexpected name %q", p.Name())
	}
	if p.ProviderType() != pbplugin.EnricherProviderType_ENRICHER_PROVIDER_HYBRID_RACE_TAGGER {
		t.Errorf("unexpected provider type %v", p.ProviderType())
	}
}

func TestLooksLikeHybridRace(t *testing.T) {
	t.Run("keyword in name", func(t *testing.T) {
		a := &pbactivity.StandardizedActivity{Name: "My HYROX Race"}
		if !looksLikeHybridRace(a) {
			t.Error("expected keyword match in name")
		}
	})

	t.Run("keyword in tag", func(t *testing.T) {
		a := &pbactivity.StandardizedActivity{Name: "Workout", Tags: []string{"hybrid"}}
		if !looksLikeHybridRace(a) {
			t.Error("expected keyword match in tag")
		}
	})

	t.Run("structural pass", func(t *testing.T) {
		a := &pbactivity.StandardizedActivity{Name: "Untitled", Sessions: []*pbactivity.Session{hyroxLikeSession()}}
		if !looksLikeHybridRace(a) {
			t.Error("expected structural heuristic to pass")
		}
	})

	t.Run("no sessions", func(t *testing.T) {
		a := &pbactivity.StandardizedActivity{Name: "Untitled"}
		if looksLikeHybridRace(a) {
			t.Error("expected false with no sessions")
		}
	})

	t.Run("structural fail wrong distance", func(t *testing.T) {
		s := hyroxLikeSession()
		s.TotalDistance = 1000 // too short
		a := &pbactivity.StandardizedActivity{Name: "Untitled", Sessions: []*pbactivity.Session{s}}
		if looksLikeHybridRace(a) {
			t.Error("expected false for out-of-range distance")
		}
	})

	t.Run("structural fail wrong lap count", func(t *testing.T) {
		s := &pbactivity.Session{Laps: []*pbactivity.Lap{makeLap(600, 200, 0)}, TotalDistance: 9700}
		a := &pbactivity.StandardizedActivity{Name: "Untitled", Sessions: []*pbactivity.Session{s}}
		if looksLikeHybridRace(a) {
			t.Error("expected false for too few laps")
		}
	})
}

func TestHRT_Enrich_NoLapsSkips(t *testing.T) {
	p := &HybridRaceTaggerProvider{}
	a := &pbactivity.StandardizedActivity{Name: "Empty"}
	res, err := p.Enrich(context.Background(), slog.Default(), a, hrtUser(), nil, false)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if res.Metadata["status"] != "skipped" || res.Metadata["reason"] != "no_laps" {
		t.Errorf("expected no_laps skip, got %v", res.Metadata)
	}
}

func TestHRT_Enrich_NotCandidateSkips(t *testing.T) {
	p := &HybridRaceTaggerProvider{}
	// Has laps but does not look like a hybrid race (single lap, short distance, plain name).
	s := &pbactivity.Session{Laps: []*pbactivity.Lap{makeLap(500, 200, 0)}, TotalDistance: 500}
	a := &pbactivity.StandardizedActivity{Name: "Easy Jog", Sessions: []*pbactivity.Session{s}}
	res, err := p.Enrich(context.Background(), slog.Default(), a, hrtUser(), nil, false)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if res.Metadata["reason"] != "not_hybrid_race_candidate" {
		t.Errorf("expected not_hybrid_race_candidate, got %v", res.Metadata)
	}
}

func TestHRT_Enrich_CandidateReturnsWaitForInput(t *testing.T) {
	p := &HybridRaceTaggerProvider{}
	// No service set -> skips the completed-pending lookup and returns WaitForInputError.
	a := &pbactivity.StandardizedActivity{Name: "HYROX London", Sessions: []*pbactivity.Session{hyroxLikeSession()}}
	_, err := p.Enrich(context.Background(), slog.Default(), a, hrtUser(), nil, false)
	if err == nil {
		t.Fatal("expected WaitForInputError")
	}
	waitErr, ok := err.(*user_input.WaitForInputError)
	if !ok {
		t.Fatalf("expected *WaitForInputError, got %T", err)
	}
	if len(waitErr.RequiredFields) == 0 || waitErr.RequiredFields[0] != "race_selection" {
		t.Errorf("expected race_selection required field, got %v", waitErr.RequiredFields)
	}
	if waitErr.Metadata["lap_count"] != "16" {
		t.Errorf("expected lap_count=16, got %s", waitErr.Metadata["lap_count"])
	}
}

// TestHRT_Enrich_ReappliesCompletedPendingInput covers the branch where a completed
// pending input exists and Enrich delegates to EnrichResume.
func TestHRT_Enrich_ReappliesCompletedPendingInput(t *testing.T) {
	mockDB := &mocks.MockDatabase{
		GetPendingInputFunc: func(ctx context.Context, userId, id string) (*pbpipeline.PendingInput, error) {
			return &pbpipeline.PendingInput{
				Status:    pbpipeline.PendingInput_STATUS_COMPLETED,
				InputData: map[string]string{"race_selection": `{"preset_id":"hyrox_men_open"}`},
			}, nil
		},
	}
	p := &HybridRaceTaggerProvider{}
	p.SetService(&bootstrap.Service{DB: mockDB})

	a := &pbactivity.StandardizedActivity{Name: "HYROX London", Sessions: []*pbactivity.Session{hyroxLikeSession()}}
	res, err := p.Enrich(context.Background(), slog.Default(), a, hrtUser(), nil, false)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if res.Metadata["status"] != "success" {
		t.Errorf("expected success status from re-applied pending input, got %v", res.Metadata)
	}
	if res.Metadata["race_type"] != "hyrox" {
		t.Errorf("expected race_type=hyrox, got %s", res.Metadata["race_type"])
	}
}
