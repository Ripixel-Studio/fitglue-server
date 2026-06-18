package mock

import (
	"context"
	"io"
	"log/slog"
	"testing"

	"github.com/fitglue/server/src/go/internal/pipeline/enricher/providers"
	pbactivity "github.com/fitglue/server/src/go/pkg/types/pb/models/activity"
	pbplugin "github.com/fitglue/server/src/go/pkg/types/pb/models/plugin"
)

func mockLogger() *slog.Logger { return slog.New(slog.NewTextHandler(io.Discard, nil)) }

func TestMock_Metadata(t *testing.T) {
	p := NewMockProvider()
	if p.Name() != "mock" {
		t.Errorf("name %q", p.Name())
	}
	if p.ProviderType() != pbplugin.EnricherProviderType_ENRICHER_PROVIDER_MOCK {
		t.Errorf("type %v", p.ProviderType())
	}
}

func enrich(t *testing.T, inputs map[string]string, doNotRetry bool) (*providers.EnrichmentResult, error) {
	t.Helper()
	return NewMockProvider().Enrich(context.Background(), mockLogger(), &pbactivity.StandardizedActivity{}, nil, inputs, doNotRetry)
}

func TestMock_SuccessDefaults(t *testing.T) {
	res, err := enrich(t, nil, false)
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	if res.Name != "Mock Activity" || res.Description == "" {
		t.Errorf("unexpected defaults: %+v", res)
	}
}

func TestMock_SuccessCustom(t *testing.T) {
	res, err := enrich(t, map[string]string{"behavior": "success", "name": "Custom", "description": "Desc"}, false)
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	if res.Name != "Custom" || res.Description != "Desc" {
		t.Errorf("unexpected: %+v", res)
	}
}

func TestMock_LagRetryable(t *testing.T) {
	_, err := enrich(t, map[string]string{"behavior": "lag"}, false)
	if err == nil {
		t.Fatal("expected retryable error")
	}
	if _, ok := err.(*providers.RetryableError); !ok {
		t.Errorf("expected *RetryableError, got %T", err)
	}
}

func TestMock_LagExhausted(t *testing.T) {
	res, err := enrich(t, map[string]string{"behavior": "lag"}, true)
	if err != nil {
		t.Fatalf("expected success when doNotRetry, got %v", err)
	}
	if res.Metadata["lag_exhausted"] != "true" {
		t.Errorf("expected lag_exhausted, got %v", res.Metadata)
	}
}

func TestMock_Fail(t *testing.T) {
	_, err := enrich(t, map[string]string{"behavior": "fail", "error_message": "kaboom"}, false)
	if err == nil || err.Error() != "kaboom" {
		t.Errorf("expected kaboom error, got %v", err)
	}
}

func TestMock_FailDefaultMessage(t *testing.T) {
	_, err := enrich(t, map[string]string{"behavior": "fail"}, false)
	if err == nil {
		t.Fatal("expected error")
	}
}

func TestMock_UnknownBehavior(t *testing.T) {
	_, err := enrich(t, map[string]string{"behavior": "bogus"}, false)
	if err == nil {
		t.Error("expected error for unknown behavior")
	}
}
