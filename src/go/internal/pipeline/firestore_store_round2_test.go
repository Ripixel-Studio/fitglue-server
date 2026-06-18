// nolint:proto-json
package pipeline

import (
	"context"
	"testing"
	"time"

	"github.com/fitglue/server/src/go/pkg/types/pb/models/pipeline"
)

// shortCtx returns a context that times out quickly so dead-Firestore calls
// fail fast rather than waiting on the default RPC deadline.
func shortCtx(t *testing.T) context.Context {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	t.Cleanup(cancel)
	return ctx
}

// TestFirestoreStore_ErrorPaths drives every FirestoreStore method against an
// unreachable Firestore emulator host. Each read/query must surface the
// transport error; each write must too. This exercises the error-return
// branches that the in-memory MockPipelineStore can't reach.
func TestFirestoreStore_ErrorPaths(t *testing.T) {
	c := offlineFSPipeline(t)
	defer c.Close()
	s := NewFirestoreStore(c)
	ctx := shortCtx(t)

	now := time.Now()
	cfg := &pipeline.PipelineConfig{Id: "p1"}
	pend := &pipeline.PendingInput{ActivityId: "i1"}

	t.Run("GetPipeline", func(t *testing.T) {
		if _, err := s.GetPipeline(ctx, "u1", "p1"); err == nil {
			t.Error("expected error")
		}
	})
	t.Run("CreatePipeline", func(t *testing.T) {
		if _, err := s.CreatePipeline(ctx, "u1", cfg); err == nil {
			t.Error("expected error")
		}
	})
	t.Run("UpdatePipeline", func(t *testing.T) {
		if _, err := s.UpdatePipeline(ctx, "u1", cfg); err == nil {
			t.Error("expected error")
		}
	})
	t.Run("DeletePipeline", func(t *testing.T) {
		if err := s.DeletePipeline(ctx, "u1", "p1"); err == nil {
			t.Error("expected error")
		}
	})
	t.Run("ListPendingInputs", func(t *testing.T) {
		if _, err := s.ListPendingInputs(ctx, "u1"); err == nil {
			t.Error("expected error")
		}
	})
	t.Run("GetPendingInput", func(t *testing.T) {
		if _, err := s.GetPendingInput(ctx, "u1", "i1"); err == nil {
			t.Error("expected error")
		}
	})
	t.Run("UpdatePendingInput", func(t *testing.T) {
		if err := s.UpdatePendingInput(ctx, "u1", pend); err == nil {
			t.Error("expected error")
		}
	})
	t.Run("GetPipelineRun", func(t *testing.T) {
		if _, err := s.GetPipelineRun(ctx, "u1", "r1"); err == nil {
			t.Error("expected error")
		}
	})
	t.Run("FindPipelineRunByActivityId", func(t *testing.T) {
		if _, err := s.FindPipelineRunByActivityId(ctx, "u1", "a1"); err == nil {
			t.Error("expected error")
		}
	})
	t.Run("FindPipelineRunByPendingInputId", func(t *testing.T) {
		if _, err := s.FindPipelineRunByPendingInputId(ctx, "u1", "i1"); err == nil {
			t.Error("expected error")
		}
	})
	t.Run("FindPipelineRunBySourceActivityID", func(t *testing.T) {
		if _, err := s.FindPipelineRunBySourceActivityID(ctx, "u1", "p1", "src1"); err == nil {
			t.Error("expected error")
		}
	})
	t.Run("FindAnyPipelineRunBySourceActivityID", func(t *testing.T) {
		if _, err := s.FindAnyPipelineRunBySourceActivityID(ctx, "u1", "src1"); err == nil {
			t.Error("expected error")
		}
	})
	t.Run("ListPipelineRuns_AllFilters", func(t *testing.T) {
		since := now.Add(-time.Hour)
		until := now
		// limit<=0 path exercises the default-50 branch too.
		if _, _, err := s.ListPipelineRuns(ctx, "u1", "p1", 0, "", &since, &until); err == nil {
			t.Error("expected error")
		}
	})
	t.Run("AdminListPipelineRuns_WithUser", func(t *testing.T) {
		// userID set → per-user collection query, status + source filters applied.
		if _, err := s.AdminListPipelineRuns(ctx, "PIPELINE_RUN_STATUS_RUNNING", "SOURCE_STRAVA", "u1", 5); err == nil {
			t.Error("expected error")
		}
	})
	t.Run("AdminListPipelineRuns_CollectionGroup", func(t *testing.T) {
		// userID empty → collection-group query, limit<=0 default branch.
		if _, err := s.AdminListPipelineRuns(ctx, "", "", "", 0); err == nil {
			t.Error("expected error")
		}
	})
	t.Run("UpdatePipelineRun", func(t *testing.T) {
		if err := s.UpdatePipelineRun(ctx, "u1", "r1", map[string]interface{}{"status": 1}); err == nil {
			t.Error("expected error")
		}
	})
}

// TestEncodeDecodeProtoMap exercises the pure protojson<->map helpers, including
// the round-trip that ListPipelineRuns / GetPipeline rely on for decoding.
func TestEncodeDecodeProtoMap(t *testing.T) {
	run := &pipeline.PipelineRun{
		Id:               "r1",
		PipelineId:       "p1",
		ActivityId:       "a1",
		SourceActivityId: "src1",
		Status:           pipeline.PipelineRunStatus_PIPELINE_RUN_STATUS_SYNCED,
	}
	m, err := encodeProtoMap(run)
	if err != nil {
		t.Fatalf("encode: %v", err)
	}
	if m["id"] != "r1" {
		t.Errorf("expected id r1 in map, got %v", m["id"])
	}

	var out pipeline.PipelineRun
	if err := decodeProtoMap(m, &out); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if out.Id != "r1" || out.PipelineId != "p1" || out.Status != run.Status {
		t.Errorf("round-trip mismatch: %+v", &out)
	}

	// DiscardUnknown: extra keys must not break decoding.
	m["totally_unknown_field"] = "ignored"
	var out2 pipeline.PipelineRun
	if err := decodeProtoMap(m, &out2); err != nil {
		t.Fatalf("decode with unknown field should be discarded: %v", err)
	}
}
