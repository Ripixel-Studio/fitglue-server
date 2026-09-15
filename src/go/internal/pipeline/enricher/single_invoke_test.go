// nolint:proto-json
package enricher

import (
	"context"
	"errors"
	"log/slog"
	"testing"

	"github.com/fitglue/server/src/go/internal/pipeline/enricher/providers"
	"github.com/fitglue/server/src/go/pkg/bootstrap"
	"github.com/fitglue/server/src/go/pkg/domain/user"
	pbactivity "github.com/fitglue/server/src/go/pkg/types/pb/models/activity"
	pbplugin "github.com/fitglue/server/src/go/pkg/types/pb/models/plugin"
)

// stubProvider is a minimal enricher registered under a unique name for these tests. It
// returns a fixed result, so InvokeSingle's registry lookup + contribution capture can be
// exercised without the real provider graph. ProviderType is UNSPECIFIED so it never
// collides in the type registry.
type stubProvider struct {
	name       string
	res        *providers.EnrichmentResult
	err        error
	idempotent *bool // when non-nil, stub implements NonIdempotentProvider
}

func (s *stubProvider) Name() string { return s.name }
func (s *stubProvider) ProviderType() pbplugin.EnricherProviderType {
	return pbplugin.EnricherProviderType_ENRICHER_PROVIDER_UNSPECIFIED
}
func (s *stubProvider) Enrich(ctx context.Context, logger *slog.Logger, activity *pbactivity.StandardizedActivity, u *user.Record, cfg map[string]string, doNotRetry bool) (*providers.EnrichmentResult, error) {
	return s.res, s.err
}

// nonIdempotentStub embeds stubProvider and reports IsIdempotent from the flag.
type nonIdempotentStub struct{ stubProvider }

func (s *nonIdempotentStub) IsIdempotent() bool { return *s.idempotent }

func newInvoker() *SingleInvoker {
	return NewSingleInvoker(&bootstrap.Service{}, slog.Default())
}

func TestInvokeSingle_CapturesContribution(t *testing.T) {
	providers.Register(&stubProvider{
		name: "test-si-ok",
		res:  &providers.EnrichmentResult{Description: "hello", Tags: []string{"t"}},
	})

	layer, err := newInvoker().InvokeSingle(context.Background(), "test-si-ok", &pbactivity.StandardizedActivity{Name: "x"}, &user.Record{})
	if err != nil {
		t.Fatalf("InvokeSingle: %v", err)
	}
	if layer == nil {
		t.Fatal("expected a layer")
	}
	if layer.GetStatus() != "SUCCESS" {
		t.Errorf("status: got %q", layer.GetStatus())
	}
	if layer.GetContribution().GetDescription() != "hello" {
		t.Errorf("contribution description: got %q", layer.GetContribution().GetDescription())
	}
	if layer.GetProviderName() != "test-si-ok" || layer.GetExecutionId() == "" {
		t.Errorf("layer identity incomplete: %+v", layer)
	}
}

func TestInvokeSingle_DoesNotMutateInput(t *testing.T) {
	providers.Register(&stubProvider{
		name: "test-si-mutate",
		res:  &providers.EnrichmentResult{Tags: []string{"added"}},
	})
	in := &pbactivity.StandardizedActivity{Name: "keep", Tags: []string{"orig"}}
	if _, err := newInvoker().InvokeSingle(context.Background(), "test-si-mutate", in, &user.Record{}); err != nil {
		t.Fatalf("InvokeSingle: %v", err)
	}
	if len(in.Tags) != 1 {
		t.Errorf("input activity was mutated: tags=%v", in.Tags)
	}
}

func TestInvokeSingle_UnknownProvider(t *testing.T) {
	_, err := newInvoker().InvokeSingle(context.Background(), "does-not-exist", &pbactivity.StandardizedActivity{}, &user.Record{})
	if !errors.Is(err, ErrProviderNotFound) {
		t.Errorf("got %v, want ErrProviderNotFound", err)
	}
}

func TestInvokeSingle_RejectsNonIdempotent(t *testing.T) {
	no := false
	providers.Register(&nonIdempotentStub{stubProvider{name: "test-si-nonidem", idempotent: &no}})
	_, err := newInvoker().InvokeSingle(context.Background(), "test-si-nonidem", &pbactivity.StandardizedActivity{}, &user.Record{})
	if !errors.Is(err, ErrNotIndividuallyInvokable) {
		t.Errorf("got %v, want ErrNotIndividuallyInvokable", err)
	}
}

func TestInvokeSingle_SkippedYieldsNoProposal(t *testing.T) {
	providers.Register(&stubProvider{name: "test-si-skip", res: &providers.EnrichmentResult{Skipped: true}})
	layer, err := newInvoker().InvokeSingle(context.Background(), "test-si-skip", &pbactivity.StandardizedActivity{}, &user.Record{})
	if err != nil {
		t.Fatalf("InvokeSingle: %v", err)
	}
	if layer != nil {
		t.Errorf("skipped enricher should yield no proposal, got %+v", layer)
	}
}

func TestInvokeSingle_NilResultYieldsNoProposal(t *testing.T) {
	providers.Register(&stubProvider{name: "test-si-nil", res: nil})
	layer, err := newInvoker().InvokeSingle(context.Background(), "test-si-nil", &pbactivity.StandardizedActivity{}, &user.Record{})
	if err != nil {
		t.Fatalf("InvokeSingle: %v", err)
	}
	if layer != nil {
		t.Errorf("nil result should yield no proposal, got %+v", layer)
	}
}
