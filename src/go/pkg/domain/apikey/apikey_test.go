package apikey

import (
	"strings"
	"testing"
)

func TestHashIngressKey_Deterministic(t *testing.T) {
	h1 := HashIngressKey("my-key")
	h2 := HashIngressKey("my-key")
	if h1 != h2 {
		t.Error("hash should be deterministic")
	}
	if len(h1) != 64 {
		t.Errorf("expected 64 hex chars (SHA-256), got %d", len(h1))
	}
	if h1 == HashIngressKey("other-key") {
		t.Error("different inputs should hash differently")
	}
}

func TestGenerateRandomIngressKey(t *testing.T) {
	raw, hash, err := GenerateRandomIngressKey()
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	if len(raw) != 64 {
		t.Errorf("expected 64 hex chars raw key, got %d", len(raw))
	}
	if hash != HashIngressKey(raw) {
		t.Error("returned hash should match HashIngressKey(raw)")
	}
	// Two generated keys must differ.
	raw2, _, _ := GenerateRandomIngressKey()
	if raw == raw2 {
		t.Error("expected unique keys")
	}
	if strings.ContainsAny(raw, "ghijklmnopqrstuvwxyz") {
		t.Error("raw key should be lowercase hex only")
	}
}
