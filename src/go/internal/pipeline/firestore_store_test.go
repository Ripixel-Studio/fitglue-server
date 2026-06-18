package pipeline

import (
	"context"
	"testing"
	"time"

	fs "cloud.google.com/go/firestore"
)

func offlineFSPipeline(t *testing.T) *fs.Client {
	t.Helper()
	t.Setenv("FIRESTORE_EMULATOR_HOST", "127.0.0.1:1")
	c, err := fs.NewClient(context.Background(), "test-project")
	if err != nil {
		t.Skipf("firestore client: %v", err)
	}
	return c
}

func TestPipelineFirestoreStore(t *testing.T) {
	c := offlineFSPipeline(t)
	defer c.Close()
	s := NewFirestoreStore(c)
	if s == nil {
		t.Fatal("expected store")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	if _, err := s.ListPipelines(ctx, "u1"); err == nil {
		t.Error("expected error from unreachable Firestore")
	}
}
