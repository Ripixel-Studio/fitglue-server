package billing

import (
	"context"
	"errors"
	"sync"
	"testing"

	"github.com/fitglue/server/src/go/pkg/testing/mocks"
	pbuser "github.com/fitglue/server/src/go/pkg/types/pb/models/user"
	pbsvc "github.com/fitglue/server/src/go/pkg/types/pb/services/billing"
)

const checkoutCompletedPayload = `{
	"type": "checkout.session.completed",
	"api_version": "2023-10-16",
	"data": {
		"object": {
			"id": "cs_test_round2",
			"customer": "cus_round2",
			"subscription": "sub_round2",
			"metadata": { "fitglue_user_id": "userN" }
		}
	}
}`

// TestEnqueueNotification_PublishesOnWebhook covers the Enqueue success path of
// enqueueNotification by driving a checkout.session.completed webhook with a
// publisher configured.
func TestEnqueueNotification_PublishesOnWebhook(t *testing.T) {
	store := NewMockStore()
	store.Users["userN"] = &MockUser{Tier: pbuser.UserTier_USER_TIER_HOBBYIST}

	var mu sync.Mutex
	var topics []string
	pub := &mocks.MockPublisher{
		PublishJSONFunc: func(_ context.Context, topic string, _ []byte) error {
			mu.Lock()
			topics = append(topics, topic)
			mu.Unlock()
			return nil
		},
	}
	svc := NewService(store, mockLogger{}, NewMockStripe(), pub, "price_123", "whsec_123")

	_, err := svc.HandleWebhookEvent(context.Background(), &pbsvc.HandleWebhookEventRequest{
		Payload:   []byte(checkoutCompletedPayload),
		Signature: stripeTestSignature([]byte(checkoutCompletedPayload), "whsec_123"),
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	mu.Lock()
	defer mu.Unlock()
	if len(topics) != 1 {
		t.Fatalf("expected exactly one notification published, got %d", len(topics))
	}
}

// TestEnqueueNotification_PublishErrorIsSwallowed covers the error branch of
// enqueueNotification: a publish failure is logged but does not fail the webhook.
func TestEnqueueNotification_PublishErrorIsSwallowed(t *testing.T) {
	store := NewMockStore()
	store.Users["userN"] = &MockUser{Tier: pbuser.UserTier_USER_TIER_HOBBYIST}
	pub := &mocks.MockPublisher{
		PublishJSONFunc: func(_ context.Context, _ string, _ []byte) error {
			return errors.New("pubsub unavailable")
		},
	}
	svc := NewService(store, mockLogger{}, NewMockStripe(), pub, "price_123", "whsec_123")

	_, err := svc.HandleWebhookEvent(context.Background(), &pbsvc.HandleWebhookEventRequest{
		Payload:   []byte(checkoutCompletedPayload),
		Signature: stripeTestSignature([]byte(checkoutCompletedPayload), "whsec_123"),
	})
	if err != nil {
		t.Fatalf("webhook should succeed despite publish error, got: %v", err)
	}
}

// TestStartTrial_StoreErrors covers StartTrial's tier-lookup error and
// update-tier error branches.
func TestStartTrial_StoreErrors(t *testing.T) {
	t.Run("TierLookupError", func(t *testing.T) {
		store := NewMockStore()
		store.GetTierErr = errors.New("read failed")
		svc := NewService(store, mockLogger{}, NewMockStripe(), nil, "price_123", "whsec_123")
		_, err := svc.StartTrial(context.Background(), &pbsvc.StartTrialRequest{UserId: "u1"})
		if err == nil {
			t.Error("expected error when GetTierStatus fails")
		}
	})

	t.Run("UpdateTierError", func(t *testing.T) {
		store := NewMockStore()
		store.Users["u1"] = &MockUser{Tier: pbuser.UserTier_USER_TIER_HOBBYIST}
		store.UpdateTierErr = errors.New("write failed")
		svc := NewService(store, mockLogger{}, NewMockStripe(), nil, "price_123", "whsec_123")
		_, err := svc.StartTrial(context.Background(), &pbsvc.StartTrialRequest{UserId: "u1"})
		if err == nil {
			t.Error("expected error when UpdateUserTier fails")
		}
	})

	t.Run("EmptyUserID", func(t *testing.T) {
		svc := NewService(NewMockStore(), mockLogger{}, NewMockStripe(), nil, "price_123", "whsec_123")
		_, err := svc.StartTrial(context.Background(), &pbsvc.StartTrialRequest{})
		if err == nil {
			t.Error("expected InvalidArgument for empty user_id")
		}
	})
}

// TestGetTierStatus_StoreError covers the store-error branch of GetTierStatus.
func TestGetTierStatus_StoreError(t *testing.T) {
	store := NewMockStore()
	store.GetTierErr = errors.New("read failed")
	svc := NewService(store, mockLogger{}, NewMockStripe(), nil, "price_123", "whsec_123")
	_, err := svc.GetTierStatus(context.Background(), &pbsvc.GetTierStatusRequest{UserId: "u1"})
	if err == nil {
		t.Error("expected error when GetTierStatus store fails")
	}
}

// TestGetSubscription_StoreError covers the store-error branch of GetSubscription.
func TestGetSubscription_StoreError(t *testing.T) {
	store := NewMockStore()
	store.GetSubErr = errors.New("read failed")
	svc := NewService(store, mockLogger{}, NewMockStripe(), nil, "price_123", "whsec_123")
	_, err := svc.GetSubscription(context.Background(), &pbsvc.GetSubscriptionRequest{UserId: "u1"})
	if err == nil {
		t.Error("expected error when GetSubscription store fails")
	}
}
