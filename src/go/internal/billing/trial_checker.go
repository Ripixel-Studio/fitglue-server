package billing

import (
	"fmt"
	"net/http"
	"time"

	"github.com/fitglue/server/src/go/internal/infra"
	shared "github.com/fitglue/server/src/go/pkg"
	"github.com/fitglue/server/src/go/pkg/notificationpub"
	pbnotification "github.com/fitglue/server/src/go/pkg/types/pb/models/notification"
)

// TrialChecker scans for users with trials expiring soon or recently expired
// and publishes the appropriate billing notifications.
// Triggered by Cloud Scheduler → Pub/Sub push → /pubsub/trial-check (daily).
type TrialChecker struct {
	store     Store
	publisher shared.Publisher
	logger    infra.Logger
}

func NewTrialChecker(store Store, publisher shared.Publisher, logger infra.Logger) *TrialChecker {
	return &TrialChecker{store: store, publisher: publisher, logger: logger}
}

// HandleTrialCheck is the HTTP handler for POST /pubsub/trial-check.
// Returns 200 on success (per-user errors are logged but don't fail the batch).
// Returns 500 only on Firestore query failure so Pub/Sub retries the whole run.
func (c *TrialChecker) HandleTrialCheck(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	now := time.Now().UTC()

	// --- 7-day warning pass ---
	// Window: trial ends between 6.5 and 7.5 days from now (24h window → at most
	// one match per user per daily run). Skip if warning already sent.
	warnFrom := now.Add(6*24*time.Hour + 12*time.Hour)
	warnTo := now.Add(7*24*time.Hour + 12*time.Hour)

	warnUsers, err := c.store.ListUsersWithTrialsEndingBetween(ctx, warnFrom, warnTo)
	if err != nil {
		c.logger.Error(ctx, "trial checker: failed to list expiring-soon users", "error", err)
		http.Error(w, "firestore error", http.StatusInternalServerError)
		return
	}

	warned := 0
	for _, u := range warnUsers {
		if u.WarningSentAt != nil {
			continue // already sent
		}
		daysLeft := int(time.Until(u.TrialEndsAt).Hours()/24) + 1
		req := &pbnotification.NotificationRequest{
			UserId: u.UserID,
			Type:   pbnotification.NotificationType_NOTIFICATION_TYPE_TRIAL_EXPIRING,
			Title:  fmt.Sprintf("Your Athlete trial ends in %d day(s)", daysLeft),
			Body:   "Upgrade to Athlete to keep all your premium features.",
			Data:   map[string]string{"days_left": fmt.Sprintf("%d", daysLeft)},
		}
		if err := notificationpub.Enqueue(ctx, c.publisher, req); err != nil {
			c.logger.Error(ctx, "trial checker: failed to enqueue warning notification", "error", err, "user_id", u.UserID)
			continue
		}
		if err := c.store.SetTrialWarningSent(ctx, u.UserID, now); err != nil {
			c.logger.Error(ctx, "trial checker: failed to mark warning sent", "error", err, "user_id", u.UserID)
		}
		warned++
	}

	// --- Expired pass ---
	// Window: trial ended in the last 48h (looks back 2 days to survive a missed run).
	// Skip if expired notification already sent.
	expFrom := now.Add(-48 * time.Hour)
	expTo := now

	expUsers, err := c.store.ListUsersWithTrialsEndingBetween(ctx, expFrom, expTo)
	if err != nil {
		c.logger.Error(ctx, "trial checker: failed to list expired users", "error", err)
		http.Error(w, "firestore error", http.StatusInternalServerError)
		return
	}

	notified := 0
	for _, u := range expUsers {
		if u.ExpiredNotifiedAt != nil {
			continue // already sent
		}
		req := &pbnotification.NotificationRequest{
			UserId: u.UserID,
			Type:   pbnotification.NotificationType_NOTIFICATION_TYPE_TRIAL_EXPIRED,
			Title:  "Your Athlete trial has ended",
			Body:   "Your account has moved to Hobbyist. Upgrade anytime to restore full access.",
		}
		if err := notificationpub.Enqueue(ctx, c.publisher, req); err != nil {
			c.logger.Error(ctx, "trial checker: failed to enqueue expired notification", "error", err, "user_id", u.UserID)
			continue
		}
		if err := c.store.SetTrialExpiredNotified(ctx, u.UserID, now); err != nil {
			c.logger.Error(ctx, "trial checker: failed to mark expired notified", "error", err, "user_id", u.UserID)
		}
		notified++
	}

	c.logger.Info(ctx, "trial checker complete",
		"warned", warned, "total_expiring", len(warnUsers),
		"notified_expired", notified, "total_expired", len(expUsers),
	)
	w.WriteHeader(http.StatusOK)
}
