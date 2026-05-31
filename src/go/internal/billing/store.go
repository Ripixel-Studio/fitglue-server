package billing

import (
	"context"
	"time"

	pbuser "github.com/fitglue/server/src/go/pkg/types/pb/models/user"
)

// TrialUser is a lightweight projection of a user doc used by the trial checker.
type TrialUser struct {
	UserID            string
	TrialEndsAt       time.Time
	WarningSentAt     *time.Time // non-nil if the 7-day warning email has already been sent
	ExpiredNotifiedAt *time.Time // non-nil if the trial-expired email has already been sent
}

type Store interface {
	GetSubscription(ctx context.Context, userID string) (*pbuser.SubscriptionState, error)
	UpsertSubscription(ctx context.Context, sub *pbuser.SubscriptionState) error

	// Helper to find a user by Stripe customer ID (needed for webhook handling)
	GetUserIDByStripeCustomer(ctx context.Context, customerID string) (string, error)

	// Update user tier and trial state (touches the user document's core fields)
	UpdateUserTier(ctx context.Context, userID string, tier pbuser.UserTier, trialEndsAt *time.Time) error

	// Get effective tier (needs to read user doc fields: tier, is_admin, trial_ends_at)
	GetTierStatus(ctx context.Context, userID string) (pbuser.UserTier, bool, *time.Time, error)

	// Trial checker: range query over users by trial_ends_at, and dedup flags.
	ListUsersWithTrialsEndingBetween(ctx context.Context, from, to time.Time) ([]TrialUser, error)
	SetTrialWarningSent(ctx context.Context, userID string, at time.Time) error
	SetTrialExpiredNotified(ctx context.Context, userID string, at time.Time) error
}
