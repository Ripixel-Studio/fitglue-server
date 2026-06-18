package billing

import (
	"context"
	"errors"
	"testing"

	pbuser "github.com/fitglue/server/src/go/pkg/types/pb/models/user"
	pbsvc "github.com/fitglue/server/src/go/pkg/types/pb/services/billing"
	"github.com/stripe/stripe-go/v76"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

func newBillingSvc() (*Service, *MockStore, *MockStripe) {
	store := NewMockStore()
	st := NewMockStripe()
	svc := NewService(store, mockLogger{}, st, nil, "price_123", "whsec_123")
	return svc, store, st
}

func TestCreateCheckoutSession(t *testing.T) {
	ctx := context.Background()

	t.Run("MissingFields", func(t *testing.T) {
		svc, _, _ := newBillingSvc()
		_, err := svc.CreateCheckoutSession(ctx, &pbsvc.CreateCheckoutSessionRequest{UserId: "u1"})
		if status.Code(err) != codes.InvalidArgument {
			t.Fatalf("expected InvalidArgument, got %v", err)
		}
	})

	t.Run("CreatesCustomerAndSession", func(t *testing.T) {
		svc, store, _ := newBillingSvc()
		res, err := svc.CreateCheckoutSession(ctx, &pbsvc.CreateCheckoutSessionRequest{
			UserId:     "u1",
			SuccessUrl: "https://app.fitglue.tech/ok",
			CancelUrl:  "https://app.fitglue.tech/no",
		})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if res.SessionUrl == "" {
			t.Error("expected a session URL")
		}
		if store.Subs["u1"] == nil || store.Subs["u1"].StripeCustomerId == "" {
			t.Error("expected new customer id persisted")
		}
	})

	t.Run("ReusesExistingCustomer", func(t *testing.T) {
		svc, store, _ := newBillingSvc()
		store.Subs["u1"] = &pbuser.SubscriptionState{UserId: "u1", StripeCustomerId: "cus_existing"}
		_, err := svc.CreateCheckoutSession(ctx, &pbsvc.CreateCheckoutSessionRequest{
			UserId:     "u1",
			SuccessUrl: "https://app.fitglue.tech/ok",
			CancelUrl:  "https://app.fitglue.tech/no",
		})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if store.Subs["u1"].StripeCustomerId != "cus_existing" {
			t.Error("should reuse existing customer id")
		}
	})

	t.Run("GetSubscriptionError", func(t *testing.T) {
		svc, store, _ := newBillingSvc()
		store.GetSubErr = errors.New("boom")
		_, err := svc.CreateCheckoutSession(ctx, &pbsvc.CreateCheckoutSessionRequest{
			UserId:     "u1",
			SuccessUrl: "https://app.fitglue.tech/ok",
			CancelUrl:  "https://app.fitglue.tech/no",
		})
		if status.Code(err) != codes.Internal {
			t.Fatalf("expected Internal, got %v", err)
		}
	})

	t.Run("CreateCustomerError", func(t *testing.T) {
		svc, _, st := newBillingSvc()
		st.CreateCustomerErr = errors.New("boom")
		_, err := svc.CreateCheckoutSession(ctx, &pbsvc.CreateCheckoutSessionRequest{
			UserId:     "u1",
			SuccessUrl: "https://app.fitglue.tech/ok",
			CancelUrl:  "https://app.fitglue.tech/no",
		})
		if status.Code(err) != codes.Internal {
			t.Fatalf("expected Internal, got %v", err)
		}
	})

	t.Run("CreateSessionError", func(t *testing.T) {
		svc, _, st := newBillingSvc()
		st.CreateSessionErr = errors.New("boom")
		_, err := svc.CreateCheckoutSession(ctx, &pbsvc.CreateCheckoutSessionRequest{
			UserId:     "u1",
			SuccessUrl: "https://app.fitglue.tech/ok",
			CancelUrl:  "https://app.fitglue.tech/no",
		})
		if status.Code(err) != codes.Internal {
			t.Fatalf("expected Internal, got %v", err)
		}
	})
}

func TestCancelSubscription(t *testing.T) {
	ctx := context.Background()

	t.Run("EmptyUserId", func(t *testing.T) {
		svc, _, _ := newBillingSvc()
		_, err := svc.CancelSubscription(ctx, &pbsvc.CancelSubscriptionRequest{})
		if status.Code(err) != codes.InvalidArgument {
			t.Fatalf("expected InvalidArgument, got %v", err)
		}
	})

	t.Run("NoActiveSubscription", func(t *testing.T) {
		svc, _, _ := newBillingSvc()
		_, err := svc.CancelSubscription(ctx, &pbsvc.CancelSubscriptionRequest{UserId: "u1"})
		if status.Code(err) != codes.FailedPrecondition {
			t.Fatalf("expected FailedPrecondition, got %v", err)
		}
	})

	t.Run("StripeError", func(t *testing.T) {
		svc, store, st := newBillingSvc()
		store.Subs["u1"] = &pbuser.SubscriptionState{UserId: "u1", StripeSubscriptionId: "sub_1"}
		st.CancelSubErr = errors.New("boom")
		_, err := svc.CancelSubscription(ctx, &pbsvc.CancelSubscriptionRequest{UserId: "u1"})
		if status.Code(err) != codes.Internal {
			t.Fatalf("expected Internal, got %v", err)
		}
	})

	t.Run("Success", func(t *testing.T) {
		svc, store, _ := newBillingSvc()
		store.Subs["u1"] = &pbuser.SubscriptionState{UserId: "u1", StripeSubscriptionId: "sub_1"}
		res, err := svc.CancelSubscription(ctx, &pbsvc.CancelSubscriptionRequest{UserId: "u1"})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if res.Status != string(stripe.SubscriptionStatusCanceled) {
			t.Errorf("expected canceled status, got %v", res.Status)
		}
		if !res.CancelAtPeriodEnd {
			t.Error("expected CancelAtPeriodEnd true")
		}
	})
}

func TestCreateBillingPortalSession(t *testing.T) {
	ctx := context.Background()

	t.Run("EmptyUserId", func(t *testing.T) {
		svc, _, _ := newBillingSvc()
		_, err := svc.CreateBillingPortalSession(ctx, &pbsvc.CreateBillingPortalSessionRequest{})
		if status.Code(err) != codes.InvalidArgument {
			t.Fatalf("expected InvalidArgument, got %v", err)
		}
	})

	t.Run("GetSubscriptionError", func(t *testing.T) {
		svc, store, _ := newBillingSvc()
		store.GetSubErr = errors.New("boom")
		_, err := svc.CreateBillingPortalSession(ctx, &pbsvc.CreateBillingPortalSessionRequest{UserId: "u1"})
		if status.Code(err) != codes.Internal {
			t.Fatalf("expected Internal, got %v", err)
		}
	})

	t.Run("NoCustomer", func(t *testing.T) {
		svc, _, _ := newBillingSvc()
		_, err := svc.CreateBillingPortalSession(ctx, &pbsvc.CreateBillingPortalSessionRequest{UserId: "u1"})
		if status.Code(err) != codes.FailedPrecondition {
			t.Fatalf("expected FailedPrecondition, got %v", err)
		}
	})

	t.Run("DisallowedReturnURL", func(t *testing.T) {
		svc, store, _ := newBillingSvc()
		store.Subs["u1"] = &pbuser.SubscriptionState{UserId: "u1", StripeCustomerId: "cus_1"}
		_, err := svc.CreateBillingPortalSession(ctx, &pbsvc.CreateBillingPortalSessionRequest{
			UserId:    "u1",
			ReturnUrl: "https://evil.example.com/steal",
		})
		if status.Code(err) != codes.InvalidArgument {
			t.Fatalf("expected InvalidArgument, got %v", err)
		}
	})

	t.Run("Success_DefaultReturnURL", func(t *testing.T) {
		svc, store, _ := newBillingSvc()
		store.Subs["u1"] = &pbuser.SubscriptionState{UserId: "u1", StripeCustomerId: "cus_1"}
		res, err := svc.CreateBillingPortalSession(ctx, &pbsvc.CreateBillingPortalSessionRequest{UserId: "u1"})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if res.Url == "" {
			t.Error("expected a portal URL")
		}
	})

	t.Run("Success_AllowedReturnURL", func(t *testing.T) {
		svc, store, _ := newBillingSvc()
		store.Subs["u1"] = &pbuser.SubscriptionState{UserId: "u1", StripeCustomerId: "cus_1"}
		_, err := svc.CreateBillingPortalSession(ctx, &pbsvc.CreateBillingPortalSessionRequest{
			UserId:    "u1",
			ReturnUrl: "https://fitglue.tech/settings",
		})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
	})
}

func TestHandleWebhookEvent_AdditionalCases(t *testing.T) {
	ctx := context.Background()

	t.Run("InvalidSignature", func(t *testing.T) {
		svc, _, _ := newBillingSvc()
		_, err := svc.HandleWebhookEvent(ctx, &pbsvc.HandleWebhookEventRequest{
			Payload:   []byte(`{"type":"checkout.session.completed"}`),
			Signature: "t=1,v1=deadbeef",
		})
		if status.Code(err) != codes.Unauthenticated {
			t.Fatalf("expected Unauthenticated, got %v", err)
		}
	})

	t.Run("PaymentFailedNotifies", func(t *testing.T) {
		svc, store, _ := newBillingSvc()
		store.Customers["cus_123"] = "u1"
		payload := `{
			"type": "invoice.payment_failed",
			"api_version": "2023-10-16",
			"data": {"object": {"id": "in_1", "customer": "cus_123"}}
		}`
		_, err := svc.HandleWebhookEvent(ctx, &pbsvc.HandleWebhookEventRequest{
			Payload:   []byte(payload),
			Signature: stripeTestSignature([]byte(payload), "whsec_123"),
		})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
	})

	t.Run("UnhandledEventTypeIsNoop", func(t *testing.T) {
		svc, _, _ := newBillingSvc()
		payload := `{
			"type": "customer.updated",
			"api_version": "2023-10-16",
			"data": {"object": {"id": "cus_1"}}
		}`
		_, err := svc.HandleWebhookEvent(ctx, &pbsvc.HandleWebhookEventRequest{
			Payload:   []byte(payload),
			Signature: stripeTestSignature([]byte(payload), "whsec_123"),
		})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
	})
}

func TestIsAllowedURL(t *testing.T) {
	allowed := []string{"https://fitglue.tech", "https://app.fitglue.tech"}
	cases := []struct {
		in   string
		want bool
	}{
		{"https://fitglue.tech/settings", true},
		{"https://app.fitglue.tech/x", true},
		{"https://evil.example.com", false},
		{"://bad-url", false},
	}
	for _, c := range cases {
		if got := isAllowedURL(c.in, allowed); got != c.want {
			t.Errorf("isAllowedURL(%q) = %v, want %v", c.in, got, c.want)
		}
	}
}
