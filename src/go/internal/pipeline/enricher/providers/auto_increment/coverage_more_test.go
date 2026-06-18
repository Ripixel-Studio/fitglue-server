package auto_increment

import (
	"context"
	"errors"
	"log/slog"
	"testing"

	"github.com/fitglue/server/src/go/pkg/bootstrap"
	user "github.com/fitglue/server/src/go/pkg/domain/user"
	"github.com/fitglue/server/src/go/pkg/testing/mocks"
	pbactivity "github.com/fitglue/server/src/go/pkg/types/pb/models/activity"
	pbplugin "github.com/fitglue/server/src/go/pkg/types/pb/models/plugin"
	pbuser "github.com/fitglue/server/src/go/pkg/types/pb/models/user"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

func aiUser() *user.Record {
	return &user.Record{UserProfile: &pbuser.UserProfile{UserId: "u1"}}
}

func TestAutoIncrement_IsIdempotent(t *testing.T) {
	p := &AutoIncrementProvider{}
	if p.IsIdempotent() {
		t.Error("auto_increment must not be idempotent (it mutates a counter)")
	}
}

func TestAutoIncrement_ProviderType_Name(t *testing.T) {
	p := &AutoIncrementProvider{}
	if p.Name() != "auto_increment" {
		t.Errorf("unexpected name %q", p.Name())
	}
	if p.ProviderType() != pbplugin.EnricherProviderType_ENRICHER_PROVIDER_AUTO_INCREMENT {
		t.Errorf("unexpected provider type %v", p.ProviderType())
	}
}

// TestAutoIncrement_NoServiceError covers the p.service == nil branch.
func TestAutoIncrement_NoServiceError(t *testing.T) {
	p := &AutoIncrementProvider{} // no SetService
	activity := &pbactivity.StandardizedActivity{Name: "Parkrun"}
	inputs := map[string]string{"counter_key": "parkrun"}

	res, err := p.Enrich(context.Background(), slog.Default(), activity, aiUser(), inputs, false)
	if err == nil {
		t.Fatal("Expected error when service is nil")
	}
	if res == nil || res.Metadata["auto_increment_applied"] != "false" {
		t.Errorf("Expected applied=false metadata on no-service error")
	}
}

// TestAutoIncrement_DedupCacheHit covers the same-source dedup early return.
func TestAutoIncrement_DedupCacheHit(t *testing.T) {
	setCounterCalled := false
	mockDB := &mocks.MockDatabase{
		GetBoosterDataFunc: func(ctx context.Context, userId, boosterId string) (map[string]interface{}, error) {
			return map[string]interface{}{
				"last_external_id":   "ext-123",
				"last_result_suffix": " (#7)",
				"last_result_val":    "7",
			}, nil
		},
		SetCounterFunc: func(ctx context.Context, userId string, counter *pbuser.Counter) error {
			setCounterCalled = true
			return nil
		},
	}
	p := &AutoIncrementProvider{}
	p.SetService(&bootstrap.Service{DB: mockDB})

	activity := &pbactivity.StandardizedActivity{Name: "Parkrun"}
	inputs := map[string]string{"counter_key": "parkrun", "external_id": "ext-123"}

	res, err := p.Enrich(context.Background(), slog.Default(), activity, aiUser(), inputs, false)
	if err != nil {
		t.Fatalf("Unexpected error: %v", err)
	}
	if res.NameSuffix != " (#7)" {
		t.Errorf("Expected cached suffix ' (#7)', got %q", res.NameSuffix)
	}
	if res.Metadata["dedup"] != "true" {
		t.Errorf("Expected dedup=true metadata")
	}
	if setCounterCalled {
		t.Error("Expected counter NOT to be incremented on dedup cache hit")
	}
}

// TestAutoIncrement_DedupDifferentExternalIDProceeds covers the dedup miss path:
// stored external id differs, so increment proceeds and the cache is written.
func TestAutoIncrement_DedupDifferentExternalIDProceeds(t *testing.T) {
	var savedBooster map[string]interface{}
	mockDB := &mocks.MockDatabase{
		GetBoosterDataFunc: func(ctx context.Context, userId, boosterId string) (map[string]interface{}, error) {
			return map[string]interface{}{"last_external_id": "old-ext"}, nil
		},
		GetCounterFunc: func(ctx context.Context, userId, id string) (*pbuser.Counter, error) {
			return &pbuser.Counter{Id: id, Count: 2}, nil
		},
		SetCounterFunc: func(ctx context.Context, userId string, counter *pbuser.Counter) error {
			return nil
		},
		SetBoosterDataFunc: func(ctx context.Context, userId, boosterId string, data map[string]interface{}) error {
			savedBooster = data
			return nil
		},
	}
	p := &AutoIncrementProvider{}
	p.SetService(&bootstrap.Service{DB: mockDB})

	activity := &pbactivity.StandardizedActivity{Name: "Parkrun"}
	inputs := map[string]string{"counter_key": "parkrun", "external_id": "new-ext"}

	res, err := p.Enrich(context.Background(), slog.Default(), activity, aiUser(), inputs, false)
	if err != nil {
		t.Fatalf("Unexpected error: %v", err)
	}
	if res.NameSuffix != " (#3)" {
		t.Errorf("Expected suffix ' (#3)', got %q", res.NameSuffix)
	}
	if savedBooster == nil {
		t.Fatal("Expected dedup cache to be written")
	}
	if savedBooster["last_external_id"] != "new-ext" {
		t.Errorf("Expected cached external id 'new-ext', got %v", savedBooster["last_external_id"])
	}
	if savedBooster["last_result_val"] != "3" {
		t.Errorf("Expected cached val '3', got %v", savedBooster["last_result_val"])
	}
}

// TestAutoIncrement_GetCounterNotFoundInitializes covers the codes.NotFound branch.
func TestAutoIncrement_GetCounterNotFoundInitializes(t *testing.T) {
	var saved *pbuser.Counter
	mockDB := &mocks.MockDatabase{
		GetCounterFunc: func(ctx context.Context, userId, id string) (*pbuser.Counter, error) {
			return nil, status.Error(codes.NotFound, "missing")
		},
		SetCounterFunc: func(ctx context.Context, userId string, counter *pbuser.Counter) error {
			saved = counter
			return nil
		},
	}
	p := &AutoIncrementProvider{}
	p.SetService(&bootstrap.Service{DB: mockDB})

	activity := &pbactivity.StandardizedActivity{Name: "Parkrun"}
	inputs := map[string]string{"counter_key": "parkrun"}

	res, err := p.Enrich(context.Background(), slog.Default(), activity, aiUser(), inputs, false)
	if err != nil {
		t.Fatalf("Unexpected error: %v", err)
	}
	if res.NameSuffix != " (#1)" {
		t.Errorf("Expected suffix ' (#1)' after NotFound init, got %q", res.NameSuffix)
	}
	if saved == nil || saved.Count != 1 {
		t.Errorf("Expected persisted counter at 1")
	}
}

// TestAutoIncrement_GetCounterRealError covers the non-NotFound DB error branch.
func TestAutoIncrement_GetCounterRealError(t *testing.T) {
	mockDB := &mocks.MockDatabase{
		GetCounterFunc: func(ctx context.Context, userId, id string) (*pbuser.Counter, error) {
			return nil, status.Error(codes.Internal, "boom")
		},
	}
	p := &AutoIncrementProvider{}
	p.SetService(&bootstrap.Service{DB: mockDB})

	activity := &pbactivity.StandardizedActivity{Name: "Parkrun"}
	inputs := map[string]string{"counter_key": "parkrun"}

	res, err := p.Enrich(context.Background(), slog.Default(), activity, aiUser(), inputs, false)
	if err == nil {
		t.Fatal("Expected error from GetCounter failure")
	}
	if res == nil || res.Metadata["auto_increment_applied"] != "false" {
		t.Errorf("Expected applied=false metadata on GetCounter error")
	}
}

// TestAutoIncrement_SetCounterError covers the persistence error branch.
func TestAutoIncrement_SetCounterError(t *testing.T) {
	mockDB := &mocks.MockDatabase{
		GetCounterFunc: func(ctx context.Context, userId, id string) (*pbuser.Counter, error) {
			return &pbuser.Counter{Id: id, Count: 0}, nil
		},
		SetCounterFunc: func(ctx context.Context, userId string, counter *pbuser.Counter) error {
			return errors.New("write failed")
		},
	}
	p := &AutoIncrementProvider{}
	p.SetService(&bootstrap.Service{DB: mockDB})

	activity := &pbactivity.StandardizedActivity{Name: "Parkrun"}
	inputs := map[string]string{"counter_key": "parkrun"}

	_, err := p.Enrich(context.Background(), slog.Default(), activity, aiUser(), inputs, false)
	if err == nil {
		t.Fatal("Expected error when SetCounter fails")
	}
}
