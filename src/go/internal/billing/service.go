// nolint:proto-json
package billing

import (
	"context"
	"encoding/json"
	"net/url"
	"time"

	"github.com/fitglue/server/src/go/internal/infra"
	shared "github.com/fitglue/server/src/go/pkg"
	"github.com/fitglue/server/src/go/pkg/notificationpub"
	pbnotification "github.com/fitglue/server/src/go/pkg/types/pb/models/notification"
	pbuser "github.com/fitglue/server/src/go/pkg/types/pb/models/user"
	pbsvc "github.com/fitglue/server/src/go/pkg/types/pb/services/billing"
	"github.com/stripe/stripe-go/v76"
	"github.com/stripe/stripe-go/v76/webhook"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/emptypb"
	"google.golang.org/protobuf/types/known/timestamppb"
)

type Service struct {
	pbsvc.UnimplementedBillingServiceServer
	store         Store
	logger        infra.Logger
	stripeClient  StripeClient
	publisher     shared.Publisher
	priceID       string
	webhookSecret string
}

func NewService(store Store, logger infra.Logger, stripeClient StripeClient, publisher shared.Publisher, priceID, webhookSecret string) *Service {
	return &Service{
		store:         store,
		logger:        logger,
		stripeClient:  stripeClient,
		publisher:     publisher,
		priceID:       priceID,
		webhookSecret: webhookSecret,
	}
}

func (s *Service) enqueueNotification(ctx context.Context, userID string, notifType pbnotification.NotificationType, title, body string, data map[string]string) {
	if s.publisher == nil {
		return
	}
	req := &pbnotification.NotificationRequest{
		UserId: userID,
		Type:   notifType,
		Title:  title,
		Body:   body,
		Data:   data,
	}
	if err := notificationpub.Enqueue(ctx, s.publisher, req); err != nil {
		s.logger.Error(ctx, "failed to enqueue billing notification", "error", err, "type", notifType.String())
	}
}

func (s *Service) GetSubscription(ctx context.Context, req *pbsvc.GetSubscriptionRequest) (*pbuser.SubscriptionState, error) {
	if req.UserId == "" {
		return nil, status.Error(codes.InvalidArgument, "user_id is required")
	}

	sub, err := s.store.GetSubscription(ctx, req.UserId)
	if err != nil {
		s.logger.Error(ctx, "failed to get subscription", "error", err)
		return nil, status.Error(codes.Internal, "failed to read subscription")
	}

	if sub == nil {
		return &pbuser.SubscriptionState{UserId: req.UserId}, nil
	}

	return sub, nil
}

func (s *Service) GetTierStatus(ctx context.Context, req *pbsvc.GetTierStatusRequest) (*pbsvc.GetTierStatusResponse, error) {
	if req.UserId == "" {
		return nil, status.Error(codes.InvalidArgument, "user_id is required")
	}

	tier, isAdmin, trialEndsAt, err := s.store.GetTierStatus(ctx, req.UserId)
	if err != nil {
		s.logger.Error(ctx, "failed to get tier status", "error", err)
		return nil, status.Error(codes.Internal, "failed to read user")
	}

	// Calculate effective tier
	effectiveTier := tier

	// Admins get athlete tier automatically
	if isAdmin {
		effectiveTier = pbuser.UserTier_USER_TIER_ATHLETE
	}

	isTrial := false
	if trialEndsAt != nil {
		if trialEndsAt.After(time.Now()) {
			effectiveTier = pbuser.UserTier_USER_TIER_ATHLETE
			isTrial = true
		}
	}

	return &pbsvc.GetTierStatusResponse{
		EffectiveTier: effectiveTier,
		IsTrial:       isTrial,
	}, nil
}

func (s *Service) CreateCheckoutSession(ctx context.Context, req *pbsvc.CreateCheckoutSessionRequest) (*pbsvc.CreateCheckoutSessionResponse, error) {
	if req.UserId == "" || req.SuccessUrl == "" || req.CancelUrl == "" {
		return nil, status.Error(codes.InvalidArgument, "missing required fields")
	}

	sub, err := s.store.GetSubscription(ctx, req.UserId)
	if err != nil {
		return nil, status.Error(codes.Internal, "failed to read user billing state")
	}
	if sub == nil {
		sub = &pbuser.SubscriptionState{UserId: req.UserId}
	}

	customerID := sub.StripeCustomerId
	if customerID == "" {
		customer, err := s.stripeClient.CreateCustomer(ctx, req.UserId)
		if err != nil {
			s.logger.Error(ctx, "failed to create stripe customer", "error", err)
			return nil, status.Error(codes.Internal, "failed to setup billing customer")
		}
		customerID = customer.ID
		sub.StripeCustomerId = customerID
		if err := s.store.UpsertSubscription(ctx, sub); err != nil {
			s.logger.Error(ctx, "failed to save new customer ID", "error", err)
		}
	}

	session, err := s.stripeClient.CreateCheckoutSession(ctx, customerID, s.priceID, req.SuccessUrl, req.CancelUrl, req.UserId)
	if err != nil {
		s.logger.Error(ctx, "failed to create checkout session", "error", err)
		return nil, status.Error(codes.Internal, "failed to create checkout session")
	}

	return &pbsvc.CreateCheckoutSessionResponse{
		SessionUrl: session.URL,
	}, nil
}

func (s *Service) StartTrial(ctx context.Context, req *pbsvc.StartTrialRequest) (*pbuser.SubscriptionState, error) {
	if req.UserId == "" {
		return nil, status.Error(codes.InvalidArgument, "user_id is required")
	}

	tier, _, trialEndsAt, err := s.store.GetTierStatus(ctx, req.UserId)
	if err != nil {
		return nil, status.Error(codes.Internal, "failed to read user")
	}

	if tier == pbuser.UserTier_USER_TIER_ATHLETE && trialEndsAt == nil {
		return nil, status.Error(codes.FailedPrecondition, "user is already a paid athlete")
	}

	if trialEndsAt != nil {
		return nil, status.Error(codes.FailedPrecondition, "user has already used a trial")
	}

	now := time.Now()
	newTrialEnd := now.Add(30 * 24 * time.Hour)

	if err := s.store.UpdateUserTier(ctx, req.UserId, tier, &newTrialEnd); err != nil {
		s.logger.Error(ctx, "failed to update user trial", "error", err)
		return nil, status.Error(codes.Internal, "failed to start trial")
	}

	sub, _ := s.store.GetSubscription(ctx, req.UserId)
	if sub == nil {
		sub = &pbuser.SubscriptionState{UserId: req.UserId}
	}
	sub.TrialEndsAt = timestamppb.New(newTrialEnd)
	_ = s.store.UpsertSubscription(ctx, sub)

	return sub, nil
}

func (s *Service) CancelSubscription(ctx context.Context, req *pbsvc.CancelSubscriptionRequest) (*pbuser.SubscriptionState, error) {
	if req.UserId == "" {
		return nil, status.Error(codes.InvalidArgument, "user_id is required")
	}

	sub, err := s.store.GetSubscription(ctx, req.UserId)
	if err != nil || sub == nil || sub.StripeSubscriptionId == "" {
		return nil, status.Error(codes.FailedPrecondition, "no active subscription found")
	}

	canceled, err := s.stripeClient.CancelSubscription(ctx, sub.StripeSubscriptionId)
	if err != nil {
		s.logger.Error(ctx, "failed to cancel subscription in stripe", "error", err)
		return nil, status.Error(codes.Internal, "failed to cancel subscription")
	}

	sub.Status = string(canceled.Status)
	sub.CancelAtPeriodEnd = canceled.CancelAtPeriodEnd

	if err := s.store.UpsertSubscription(ctx, sub); err != nil {
		s.logger.Error(ctx, "failed to update subscription state", "error", err)
	}

	return sub, nil
}

func (s *Service) CreateBillingPortalSession(ctx context.Context, req *pbsvc.CreateBillingPortalSessionRequest) (*pbsvc.CreateBillingPortalSessionResponse, error) {
	if req.UserId == "" {
		return nil, status.Error(codes.InvalidArgument, "user_id is required")
	}

	sub, err := s.store.GetSubscription(ctx, req.UserId)
	if err != nil {
		s.logger.Error(ctx, "failed to get subscription", "error", err)
		return nil, status.Error(codes.Internal, "failed to read subscription")
	}
	if sub == nil || sub.StripeCustomerId == "" {
		return nil, status.Error(codes.FailedPrecondition, "no billing customer found — user has never subscribed")
	}

	returnURL := req.ReturnUrl
	if returnURL == "" {
		returnURL = "https://app.fitglue.tech/settings"
	}

	allowedOrigins := []string{"https://fitglue.tech", "https://app.fitglue.tech", "https://dev.fitglue.tech"}
	if !isAllowedURL(returnURL, allowedOrigins) {
		return nil, status.Errorf(codes.InvalidArgument, "return_url host is not permitted")
	}

	session, err := s.stripeClient.CreateBillingPortalSession(ctx, sub.StripeCustomerId, returnURL)
	if err != nil {
		s.logger.Error(ctx, "failed to create billing portal session", "error", err)
		return nil, status.Error(codes.Internal, "failed to create billing portal session")
	}

	return &pbsvc.CreateBillingPortalSessionResponse{
		Url: session.URL,
	}, nil
}

func (s *Service) HandleWebhookEvent(ctx context.Context, req *pbsvc.HandleWebhookEventRequest) (*emptypb.Empty, error) {
	// Webhook signature verification should be done by the webhook gateway before it reaches this RPC.
	// But in Stripe's case, signature verification requires the raw body.
	// Our API gateway will pass the raw payload here.

	event, err := webhook.ConstructEvent(req.Payload, req.Signature, s.webhookSecret)
	if err != nil {
		return nil, status.Errorf(codes.Unauthenticated, "invalid stripe signature: %v", err)
	}

	switch event.Type {
	case "checkout.session.completed":
		var session stripe.CheckoutSession
		if err := json.Unmarshal(event.Data.Raw, &session); err != nil {
			return nil, err
		}

		userID := session.Metadata["fitglue_user_id"]
		if userID != "" {
			if err := s.store.UpdateUserTier(ctx, userID, pbuser.UserTier_USER_TIER_ATHLETE, nil); err != nil {
				s.logger.Error(ctx, "failed to update user tier to athlete", "error", err, "userId", userID)
			}

			sub, _ := s.store.GetSubscription(ctx, userID)
			if sub == nil {
				sub = &pbuser.SubscriptionState{UserId: userID}
			}
			if session.Customer != nil {
				sub.StripeCustomerId = session.Customer.ID
			}
			if session.Subscription != nil {
				sub.StripeSubscriptionId = session.Subscription.ID
			}
			_ = s.store.UpsertSubscription(ctx, sub)

			s.enqueueNotification(ctx, userID,
				pbnotification.NotificationType_NOTIFICATION_TYPE_SUBSCRIPTION_STARTED,
				"Welcome to Athlete!",
				"Your FitGlue Athlete subscription is confirmed. Enjoy unlimited syncs and all premium features.",
				nil,
			)
		}

	case "invoice.payment_failed":
		var invoice stripe.Invoice
		if err := json.Unmarshal(event.Data.Raw, &invoice); err != nil {
			return nil, err
		}

		customerID := ""
		if invoice.Customer != nil {
			customerID = invoice.Customer.ID
		}

		if customerID != "" {
			userID, err := s.store.GetUserIDByStripeCustomer(ctx, customerID)
			if err == nil && userID != "" {
				// Don't downgrade yet — Stripe retries the charge. Just notify.
				s.enqueueNotification(ctx, userID,
					pbnotification.NotificationType_NOTIFICATION_TYPE_PAYMENT_FAILED,
					"Payment failed — action required",
					"We couldn't process your FitGlue Athlete payment. Please update your payment method to avoid losing access.",
					nil,
				)
			}
		}

	case "customer.subscription.deleted":
		var subscription stripe.Subscription
		if err := json.Unmarshal(event.Data.Raw, &subscription); err != nil {
			return nil, err
		}

		customerID := ""
		if subscription.Customer != nil {
			customerID = subscription.Customer.ID
		}

		if customerID != "" {
			userID, err := s.store.GetUserIDByStripeCustomer(ctx, customerID)
			if err == nil && userID != "" {
				s.store.UpdateUserTier(ctx, userID, pbuser.UserTier_USER_TIER_HOBBYIST, nil)

				sub, _ := s.store.GetSubscription(ctx, userID)
				if sub != nil {
					sub.Status = string(subscription.Status)
					_ = s.store.UpsertSubscription(ctx, sub)
				}

				s.enqueueNotification(ctx, userID,
					pbnotification.NotificationType_NOTIFICATION_TYPE_SUBSCRIPTION_ENDED,
					"Your Athlete subscription has ended",
					"Your FitGlue Athlete subscription has been cancelled. Your account has moved to the Hobbyist tier.",
					nil,
				)
			}
		}
	}

	return &emptypb.Empty{}, nil
}

// isAllowedURL returns true if u parses successfully and its host appears in allowed.
func isAllowedURL(u string, allowed []string) bool {
	parsed, err := url.Parse(u)
	if err != nil {
		return false
	}
	for _, a := range allowed {
		base, err := url.Parse(a)
		if err != nil {
			continue
		}
		if parsed.Host == base.Host {
			return true
		}
	}
	return false
}
