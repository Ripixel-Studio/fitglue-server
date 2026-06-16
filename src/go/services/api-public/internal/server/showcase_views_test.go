package server

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/fitglue/server/src/go/internal/infra"
	activitypb "github.com/fitglue/server/src/go/pkg/types/pb/services/activity"
	"google.golang.org/grpc"
	"google.golang.org/protobuf/types/known/emptypb"
)

// fakeActivityClient embeds the full client interface so we only implement the
// one method under test; any unexpected call would nil-panic and fail the test.
type fakeActivityClient struct {
	activitypb.ActivityServiceClient
	calls   int
	lastReq *activitypb.RecordShowcaseViewRequest
}

func (f *fakeActivityClient) RecordShowcaseView(ctx context.Context, in *activitypb.RecordShowcaseViewRequest, opts ...grpc.CallOption) (*emptypb.Empty, error) {
	f.calls++
	f.lastReq = in
	return &emptypb.Empty{}, nil
}

func postView(t *testing.T, srv *APIServer, path, userAgent string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(http.MethodPost, path, nil)
	if userAgent != "" {
		req.Header.Set("User-Agent", userAgent)
	}
	rec := httptest.NewRecorder()
	srv.ServeHTTP(rec, req)
	return rec
}

func TestRecordView_HumanCountsWithResolvedKey(t *testing.T) {
	fake := &fakeActivityClient{}
	srv := NewAPIServer(infra.NewLogger(), fake, nil, "salt")

	rec := postView(t, srv, "/api/public/showcase/s1/view", "Mozilla/5.0 (iPhone) AppleWebKit/605")
	if rec.Code != http.StatusNoContent {
		t.Fatalf("status = %d, want 204", rec.Code)
	}
	if fake.calls != 1 {
		t.Fatalf("expected 1 record call, got %d", fake.calls)
	}
	if fake.lastReq.TargetKey != "activity:s1" {
		t.Errorf("target_key = %q, want activity:s1", fake.lastReq.TargetKey)
	}
	if fake.lastReq.VisitorHash == "" {
		t.Error("expected a visitor hash to be computed")
	}
}

func TestRecordView_ProfileAndRoundupKeys(t *testing.T) {
	fake := &fakeActivityClient{}
	srv := NewAPIServer(infra.NewLogger(), fake, nil, "salt")

	postView(t, srv, "/api/public/showcase/profile/jane/view", "Mozilla/5.0")
	if fake.lastReq.TargetKey != "profile:jane" {
		t.Errorf("profile target_key = %q", fake.lastReq.TargetKey)
	}

	postView(t, srv, "/api/public/showcase/jane/roundup/week-23-2025/view", "Mozilla/5.0")
	if fake.lastReq.TargetKey != "roundup:jane:week-23-2025" {
		t.Errorf("roundup target_key = %q", fake.lastReq.TargetKey)
	}
}

func TestRecordView_BotIsNotCounted(t *testing.T) {
	fake := &fakeActivityClient{}
	srv := NewAPIServer(infra.NewLogger(), fake, nil, "salt")

	rec := postView(t, srv, "/api/public/showcase/s1/view", "Twitterbot/1.0")
	if rec.Code != http.StatusNoContent {
		t.Fatalf("status = %d, want 204 even for bots", rec.Code)
	}
	if fake.calls != 0 {
		t.Fatalf("bot should not be counted, got %d calls", fake.calls)
	}
}
