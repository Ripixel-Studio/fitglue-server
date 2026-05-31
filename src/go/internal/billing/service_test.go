package billing

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"testing"
	"time"

	"github.com/fitglue/server/src/go/internal/infra"
	pbuser "github.com/fitglue/server/src/go/pkg/types/pb/models/user"
	pbsvc "github.com/fitglue/server/src/go/pkg/types/pb/services/billing"
	"github.com/stripe/stripe-go/v76"
)

// stripeTestSignature computes a Stripe-Signature header value for unit tests.
func stripeTestSignature(payload []byte, secret string) string {
	ts := fmt.Sprintf("%d", time.Now().Unix())
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write([]byte(ts + "." + string(payload)))
	return fmt.Sprintf("t=%s,v1=%s", ts, hex.EncodeToString(mac.Sum(nil)))
}

type mockLogger struct{}

func (m mockLogger) Debug(ctx context.Context, msg string, args ...any) {}
func (m mockLogger) Info(ctx context.Context, msg string, args ...any)  {}
func (m mockLogger) Warn(ctx context.Context, msg string, args ...any)  {}
func (m mockLogger) Error(ctx context.Context, msg string, args ...any) {}
func (m mockLogger) With(args ...any) infra.Logger                      { return m }

type MockStore struct {
	Subs          map[string]*pbuser.SubscriptionState
	Users         map[string]*MockUser
	Customers     map[string]string // customerID -> userID
	UpsertSubErr  error
	GetSubErr     error
	GetTierErr    error
	UpdateTierErr error
}

type MockUser struct {
	Tier        pbuser.UserTier
	IsAdmin     bool
	TrialEndsAt *time.Time
}

func NewMockStore() *MockStore {
	return &MockStore{
		Subs:      make(map[string]*pbuser.SubscriptionState),
		Users:     make(map[string]*MockUser),
		Customers: make(map[string]string),
	}
}

func (m *MockStore) GetSubscription(ctx context.Context, userID string) (*pbuser.SubscriptionState, error) {
	if m.GetSubErr != nil {
		return nil, m.GetSubErr
	}
	return m.Subs[userID], nil
}

func (m *MockStore) UpsertSubscription(ctx context.Context, sub *pbuser.SubscriptionState) error {
	if m.UpsertSubErr != nil {
		return m.UpsertSubErr
	}
	m.Subs[sub.UserId] = sub
	if sub.StripeCustomerId != "" {
		m.Customers[sub.StripeCustomerId] = sub.UserId
	}
	return nil
}

func (m *MockStore) GetUserIDByStripeCustomer(ctx context.Context, customerID string) (string, error) {
	return m.Customers[customerID], nil
}

func (m *MockStore) UpdateUserTier(ctx context.Context, userID string, tier pbuser.UserTier, trialEndsAt *time.Time) error {
	if m.UpdateTierErr != nil {
		return m.UpdateTierErr
	}
	if _, ok := m.Users[userID]; !ok {
		m.Users[userID] = &MockUser{}
	}
	m.Users[userID].Tier = tier
	if trialEndsAt != nil {
		m.Users[userID].TrialEndsAt = trialEndsAt
	} else {
		m.Users[userID].TrialEndsAt = nil
	}
	return nil
}

func (m *MockStore) GetTierStatus(ctx context.Context, userID string) (pbuser.UserTier, bool, *time.Time, error) {
	if m.GetTierErr != nil {
		return pbuser.UserTier_USER_TIER_UNSPECIFIED, false, nil, m.GetTierErr
	}
	if user, ok := m.Users[userID]; ok {
		return user.Tier, user.IsAdmin, user.TrialEndsAt, nil
	}
	return pbuser.UserTier_USER_TIER_HOBBYIST, false, nil, nil
}

func (m *MockStore) ListUsersWithTrialsEndingBetween(_ context.Context, _, _ time.Time) ([]TrialUser, error) {
	return nil, nil
}

func (m *MockStore) SetTrialWarningSent(_ context.Context, _ string, _ time.Time) error {
	return nil
}

func (m *MockStore) SetTrialExpiredNotified(_ context.Context, _ string, _ time.Time) error {
	return nil
}

type MockStripe struct {
	Customers map[string]*stripe.Customer
	Sessions  map[string]*stripe.CheckoutSession
	Subs      map[string]*stripe.Subscription

	CreateCustomerErr error
	CreateSessionErr  error
	GetSubErr         error
	GetCustErr        error
	CancelSubErr      error

	idCounter int
}

func NewMockStripe() *MockStripe {
	return &MockStripe{
		Customers: make(map[string]*stripe.Customer),
		Sessions:  make(map[string]*stripe.CheckoutSession),
		Subs:      make(map[string]*stripe.Subscription),
	}
}

func (m *MockStripe) CreateCustomer(ctx context.Context, userID string) (*stripe.Customer, error) {
	if m.CreateCustomerErr != nil {
		return nil, m.CreateCustomerErr
	}
	m.idCounter++
	id := "cus_mock_" + string(rune(m.idCounter))
	cust := &stripe.Customer{ID: id, Metadata: map[string]string{"fitglue_user_id": userID}}
	m.Customers[id] = cust
	return cust, nil
}

func (m *MockStripe) CreateCheckoutSession(ctx context.Context, customerID string, priceID string, successURL string, cancelURL string, userID string) (*stripe.CheckoutSession, error) {
	if m.CreateSessionErr != nil {
		return nil, m.CreateSessionErr
	}
	m.idCounter++
	id := "cs_mock_" + string(rune(m.idCounter))
	session := &stripe.CheckoutSession{
		ID:  id,
		URL: "https://checkout.stripe.com/c/pay/" + id,
		Metadata: map[string]string{
			"fitglue_user_id": userID,
		},
	}
	m.Sessions[id] = session
	return session, nil
}

func (m *MockStripe) GetSubscription(ctx context.Context, subscriptionID string) (*stripe.Subscription, error) {
	if m.GetSubErr != nil {
		return nil, m.GetSubErr
	}
	return m.Subs[subscriptionID], nil
}

func (m *MockStripe) GetCustomer(ctx context.Context, customerID string) (*stripe.Customer, error) {
	if m.GetCustErr != nil {
		return nil, m.GetCustErr
	}
	return m.Customers[customerID], nil
}

func (m *MockStripe) CancelSubscription(ctx context.Context, subscriptionID string) (*stripe.Subscription, error) {
	if m.CancelSubErr != nil {
		return nil, m.CancelSubErr
	}
	sub, ok := m.Subs[subscriptionID]
	if !ok {
		sub = &stripe.Subscription{ID: subscriptionID}
	}
	sub.Status = stripe.SubscriptionStatusCanceled
	sub.CancelAtPeriodEnd = true
	m.Subs[subscriptionID] = sub
	return sub, nil
}

func (m *MockStripe) CreateBillingPortalSession(ctx context.Context, customerID string, returnURL string) (*stripe.BillingPortalSession, error) {
	return &stripe.BillingPortalSession{
		URL: "https://billing.stripe.com/p/session/test_portal",
	}, nil
}

func TestGetSubscription(t *testing.T) {
	store := NewMockStore()
	stripe := NewMockStripe()
	logger := mockLogger{}
	svc := NewService(store, logger, stripe, nil, "price_123", "whsec_123")
	ctx := context.Background()

	// Empty sub
	res, err := svc.GetSubscription(ctx, &pbsvc.GetSubscriptionRequest{UserId: "user1"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if res.UserId != "user1" {
		t.Errorf("expected user1, got %v", res.UserId)
	}

	// Populated sub
	store.Subs["user2"] = &pbuser.SubscriptionState{UserId: "user2", StripeCustomerId: "cus_123"}
	res2, err := svc.GetSubscription(ctx, &pbsvc.GetSubscriptionRequest{UserId: "user2"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if res2.StripeCustomerId != "cus_123" {
		t.Errorf("expected cus_123, got %v", res2.StripeCustomerId)
	}
}

func TestGetTierStatus(t *testing.T) {
	store := NewMockStore()
	stripe := NewMockStripe()
	logger := mockLogger{}
	svc := NewService(store, logger, stripe, nil, "price_123", "whsec_123")
	ctx := context.Background()

	// Base hobbyist
	res, err := svc.GetTierStatus(ctx, &pbsvc.GetTierStatusRequest{UserId: "user1"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if res.EffectiveTier != pbuser.UserTier_USER_TIER_HOBBYIST {
		t.Errorf("expected hobbyist, got %v", res.EffectiveTier)
	}
	if res.IsTrial != false {
		t.Errorf("expected no trial")
	}

	// Admin is athlete
	store.Users["user2"] = &MockUser{IsAdmin: true, Tier: pbuser.UserTier_USER_TIER_HOBBYIST}
	res2, _ := svc.GetTierStatus(ctx, &pbsvc.GetTierStatusRequest{UserId: "user2"})
	if res2.EffectiveTier != pbuser.UserTier_USER_TIER_ATHLETE {
		t.Errorf("admin should be athlete")
	}

	// Active trial is athlete + isTrial
	future := time.Now().Add(time.Hour)
	store.Users["user3"] = &MockUser{Tier: pbuser.UserTier_USER_TIER_HOBBYIST, TrialEndsAt: &future}
	res3, _ := svc.GetTierStatus(ctx, &pbsvc.GetTierStatusRequest{UserId: "user3"})
	if res3.EffectiveTier != pbuser.UserTier_USER_TIER_ATHLETE || !res3.IsTrial {
		t.Errorf("active trial should be athlete and isTrial=true")
	}

	// Expired trial is hobbyist
	past := time.Now().Add(-time.Hour)
	store.Users["user4"] = &MockUser{Tier: pbuser.UserTier_USER_TIER_HOBBYIST, TrialEndsAt: &past}
	res4, _ := svc.GetTierStatus(ctx, &pbsvc.GetTierStatusRequest{UserId: "user4"})
	if res4.EffectiveTier != pbuser.UserTier_USER_TIER_HOBBYIST || res4.IsTrial {
		t.Errorf("expired trial should revert to hobbyist and isTrial=false")
	}
}

func TestStartTrial(t *testing.T) {
	store := NewMockStore()
	stripe := NewMockStripe()
	logger := mockLogger{}
	svc := NewService(store, logger, stripe, nil, "price_123", "whsec_123")
	ctx := context.Background()

	// Start trial for hobbyist
	store.Users["user1"] = &MockUser{Tier: pbuser.UserTier_USER_TIER_HOBBYIST}
	res, err := svc.StartTrial(ctx, &pbsvc.StartTrialRequest{UserId: "user1"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if res.TrialEndsAt == nil {
		t.Fatalf("trial ends at should be populated")
	}

	if store.Users["user1"].Tier != pbuser.UserTier_USER_TIER_HOBBYIST {
		t.Errorf("tier should still be hobbyist (effective tier is computed on read)")
	}

	// Start trial for existing athlete -> fails
	store.Users["user2"] = &MockUser{Tier: pbuser.UserTier_USER_TIER_ATHLETE}
	_, err2 := svc.StartTrial(ctx, &pbsvc.StartTrialRequest{UserId: "user2"})
	if err2 == nil {
		t.Fatalf("expected error starting trial for existing athlete")
	}
}

func TestHandleWebhookEvent(t *testing.T) {
	store := NewMockStore()
	stripeClient := NewMockStripe()
	logger := mockLogger{}
	svc := NewService(store, logger, stripeClient, nil, "price_123", "whsec_123")
	ctx := context.Background()

	// 1. checkout.session.completed
	sessionPayload := `{
		"type": "checkout.session.completed",
		"api_version": "2023-10-16",
		"data": {
			"object": {
				"id": "cs_test_123",
				"customer": "cus_123",
				"subscription": "sub_123",
				"metadata": {
					"fitglue_user_id": "user1"
				}
			}
		}
	}`

	// Create mock user before webhook
	store.Users["user1"] = &MockUser{Tier: pbuser.UserTier_USER_TIER_HOBBYIST}

	_, err := svc.HandleWebhookEvent(ctx, &pbsvc.HandleWebhookEventRequest{
		Payload:   []byte(sessionPayload),
		Signature: stripeTestSignature([]byte(sessionPayload), "whsec_123"),
	})
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}

	if store.Users["user1"].Tier != pbuser.UserTier_USER_TIER_ATHLETE {
		t.Errorf("user should be upgraded to athlete")
	}
	if store.Subs["user1"].StripeCustomerId != "cus_123" {
		t.Errorf("customer id not saved")
	}

	// 2. customer.subscription.deleted
	stripeClient.Subs["sub_123"] = &stripe.Subscription{ID: "sub_123", Status: stripe.SubscriptionStatusCanceled, Customer: &stripe.Customer{ID: "cus_123"}}
	delPayload := `{
		"type": "customer.subscription.deleted",
		"api_version": "2023-10-16",
		"data": {
			"object": {
				"id": "sub_123",
				"customer": "cus_123",
				"status": "canceled"
			}
		}
	}`

	_, err2 := svc.HandleWebhookEvent(ctx, &pbsvc.HandleWebhookEventRequest{
		Payload:   []byte(delPayload),
		Signature: stripeTestSignature([]byte(delPayload), "whsec_123"),
	})
	if err2 != nil {
		t.Fatalf("unexpected err: %v", err2)
	}

	if store.Users["user1"].Tier != pbuser.UserTier_USER_TIER_HOBBYIST {
		t.Errorf("user should be downgraded to hobbyist")
	}
	if store.Subs["user1"].Status != "canceled" {
		t.Errorf("sub status should be canceled")
	}
}
