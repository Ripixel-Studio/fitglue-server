package main

import (
	"context"
	"testing"

	"cloud.google.com/go/storage"
	gcsstorage "github.com/fitglue/server/src/go/pkg/infrastructure/storage"
)

// offlineStorageClient builds a GCS client that does not require credentials by
// pointing at a (non-listening) emulator endpoint. Operations will fail, which
// is enough to exercise the GCSBlobStore delegation paths.
func offlineStorageClient(t *testing.T) *storage.Client {
	t.Helper()
	t.Setenv("STORAGE_EMULATOR_HOST", "127.0.0.1:1")
	c, err := storage.NewClient(context.Background())
	if err != nil {
		t.Skipf("could not build storage client: %v", err)
	}
	return c
}

func TestGCSBlobStore_DelegatesToAdapter(t *testing.T) {
	client := offlineStorageClient(t)
	defer client.Close()
	store := &GCSBlobStore{adapter: &gcsstorage.StorageAdapter{Client: client}}

	ctx, cancel := context.WithCancel(context.Background())
	cancel() // cancelled so network ops fail fast

	if _, err := store.Get(ctx, "bucket", "obj"); err == nil {
		t.Error("Get: expected error")
	}
	if err := store.Write(ctx, "bucket", "obj", []byte("data")); err == nil {
		t.Error("Write: expected error")
	}
	if err := store.Delete(ctx, "bucket", "obj"); err == nil {
		t.Error("Delete: expected error")
	}
}
