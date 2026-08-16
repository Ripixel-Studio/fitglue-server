package pubsub

import (
	"context"
	"testing"
	"time"

	"cloud.google.com/go/pubsub"
	"cloud.google.com/go/pubsub/pstest"
	"github.com/fitglue/server/src/go/internal/infra"
	"google.golang.org/api/option"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
)

// newFakeAdapter spins up an in-memory Pub/Sub server, creates the given topic
// and returns an adapter wired to it plus a cleanup func.
func newFakeAdapter(t *testing.T, topicID string) (*PubSubAdapter, func()) {
	t.Helper()

	srv := pstest.NewServer()
	conn, err := grpc.NewClient(srv.Addr, grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		t.Fatalf("dial fake pubsub: %v", err)
	}

	client, err := pubsub.NewClient(context.Background(), "test-project", option.WithGRPCConn(conn))
	if err != nil {
		t.Fatalf("new pubsub client: %v", err)
	}

	if _, err := client.CreateTopic(context.Background(), topicID); err != nil {
		t.Fatalf("create topic: %v", err)
	}

	a := &PubSubAdapter{Client: client, Logger: infra.NewLogger()}
	cleanup := func() {
		_ = client.Close()
		_ = conn.Close()
		_ = srv.Close()
	}
	return a, cleanup
}

// TestPublish_SucceedsWithHealthyContext is the baseline: a normal context
// publishes and the message lands on the server.
func TestPublish_SucceedsWithHealthyContext(t *testing.T) {
	a, cleanup := newFakeAdapter(t, "healthy-topic")
	defer cleanup()

	id, err := a.publish(context.Background(), "healthy-topic", []byte(`{"ok":true}`))
	if err != nil {
		t.Fatalf("publish returned error: %v", err)
	}
	if id == "" {
		t.Fatal("expected a non-empty message id")
	}
}

// TestPublish_SurvivesExpiredRequestContext reproduces SERVER-5:
// context.DeadlineExceeded raised from (*PubSubAdapter).publishWithAttrs.
//
// A handler triggered by a Pub/Sub push shares the inbound request context. If
// upstream work has already burned the request budget, that context is expired
// by the time we publish. Before the fix, res.Get(ctx) returned
// context.DeadlineExceeded and the valid event was dropped/retried. The adapter
// now runs the publish on a detached, freshly-bounded context, so an
// already-expired caller context no longer fails the publish.
func TestPublish_SurvivesExpiredRequestContext(t *testing.T) {
	a, cleanup := newFakeAdapter(t, "lagged-topic")
	defer cleanup()

	// A context whose deadline is already in the past — the state a slow handler
	// hands to the publisher.
	ctx, cancel := context.WithDeadline(context.Background(), time.Now().Add(-time.Second))
	defer cancel()

	if err := ctx.Err(); err == nil {
		t.Fatal("test setup: expected the caller context to be already expired")
	}

	id, err := a.publish(ctx, "lagged-topic", []byte(`{"activity":"enriched"}`))
	if err != nil {
		t.Fatalf("publish should survive an expired request context, got: %v", err)
	}
	if id == "" {
		t.Fatal("expected a non-empty message id")
	}
}
