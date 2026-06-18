package providers

import (
	"context"
	"errors"
	"log/slog"
	"testing"
	"time"

	"github.com/fitglue/server/src/go/pkg/domain/user"
	pbactivity "github.com/fitglue/server/src/go/pkg/types/pb/models/activity"
	pbplugin "github.com/fitglue/server/src/go/pkg/types/pb/models/plugin"
)

func TestToFloat64(t *testing.T) {
	tests := []struct {
		name string
		in   interface{}
		want float64
	}{
		{"float64", float64(3.5), 3.5},
		{"int64", int64(7), 7},
		{"int", int(9), 9},
		{"string defaults zero", "nope", 0},
		{"nil defaults zero", nil, 0},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := ToFloat64(tc.in); got != tc.want {
				t.Errorf("ToFloat64(%v) = %v, want %v", tc.in, got, tc.want)
			}
		})
	}
}

func TestRetryableError(t *testing.T) {
	inner := errors.New("boom")
	re := NewRetryableError(inner, 2*time.Minute, "data lag")
	if re.RetryAfter != 2*time.Minute {
		t.Errorf("RetryAfter = %v", re.RetryAfter)
	}
	if re.Unwrap() != inner {
		t.Error("Unwrap should return inner error")
	}
	if !errors.Is(re, inner) {
		t.Error("errors.Is should match inner")
	}
	msg := re.Error()
	if msg == "" {
		t.Error("Error() should not be empty")
	}
}

// fakeProvider is a minimal Provider for exercising the registry.
type fakeProvider struct {
	name string
	typ  pbplugin.EnricherProviderType
}

func (f *fakeProvider) Name() string                                { return f.name }
func (f *fakeProvider) ProviderType() pbplugin.EnricherProviderType { return f.typ }
func (f *fakeProvider) Enrich(_ context.Context, _ *slog.Logger, _ *pbactivity.StandardizedActivity, _ *user.Record, _ map[string]string, _ bool) (*EnrichmentResult, error) {
	return &EnrichmentResult{}, nil
}

func TestRegistry(t *testing.T) {
	ClearRegistry()
	defer ClearRegistry()

	p := &fakeProvider{name: "reg-test", typ: pbplugin.EnricherProviderType_ENRICHER_PROVIDER_MOCK}
	Register(p)

	got, ok := GetByName("reg-test")
	if !ok || got != p {
		t.Fatal("GetByName failed")
	}
	gotT, ok := GetByType(pbplugin.EnricherProviderType_ENRICHER_PROVIDER_MOCK)
	if !ok || gotT != p {
		t.Fatal("GetByType failed")
	}
	all := GetAll()
	if len(all) != 1 {
		t.Fatalf("GetAll len = %d, want 1", len(all))
	}

	if _, ok := GetByName("missing"); ok {
		t.Error("expected missing provider lookup to fail")
	}
}

func TestRegistry_DuplicateNamePanics(t *testing.T) {
	ClearRegistry()
	defer ClearRegistry()
	defer func() {
		if r := recover(); r == nil {
			t.Error("expected panic on duplicate name registration")
		}
	}()
	// Use UNSPECIFIED type so only the name collision triggers the panic.
	Register(&fakeProvider{name: "dup", typ: pbplugin.EnricherProviderType_ENRICHER_PROVIDER_UNSPECIFIED})
	Register(&fakeProvider{name: "dup", typ: pbplugin.EnricherProviderType_ENRICHER_PROVIDER_UNSPECIFIED})
}

func TestRegistry_UnspecifiedTypeNotIndexed(t *testing.T) {
	ClearRegistry()
	defer ClearRegistry()
	Register(&fakeProvider{name: "no-type", typ: pbplugin.EnricherProviderType_ENRICHER_PROVIDER_UNSPECIFIED})
	if _, ok := GetByType(pbplugin.EnricherProviderType_ENRICHER_PROVIDER_UNSPECIFIED); ok {
		t.Error("UNSPECIFIED type should not be indexed in typeRegistry")
	}
}
