package main

import (
	"context"
	"testing"
)

func TestNewGCSBlobStore_InvalidURI(t *testing.T) {
	s := NewGCSBlobStore(nil)
	if s == nil {
		t.Fatal("expected store")
	}
	// Invalid URI is rejected before any GCS client access.
	if _, err := s.Get(context.Background(), "http://not-gcs"); err == nil {
		t.Error("expected invalid URI error")
	}
}
