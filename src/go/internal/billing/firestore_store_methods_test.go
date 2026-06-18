package billing

import (
	"context"
	"testing"
	"time"

	pbuser "github.com/fitglue/server/src/go/pkg/types/pb/models/user"
)

// billingStoreCtx returns a short-lived context so dead-Firestore calls fail fast.
func billingStoreCtx(t *testing.T) context.Context {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	t.Cleanup(cancel)
	return ctx
}

// TestFirestoreStoreMethods drives every FirestoreStore method against an
// unreachable emulator to cover the query-building and error-handling paths.
func TestFirestoreStoreMethods(t *testing.T) {
	c := offlineFS(t)
	t.Cleanup(func() { _ = c.Close() })
	s := NewFirestoreStore(c)
	ctx := billingStoreCtx(t)

	wantErr := func(name string, err error) {
		if err == nil {
			t.Errorf("%s: expected error from unreachable Firestore, got nil", name)
		}
	}

	_, err := s.GetSubscription(ctx, "u1")
	wantErr("GetSubscription", err)

	wantErr("UpsertSubscription", s.UpsertSubscription(ctx, &pbuser.SubscriptionState{
		UserId:           "u1",
		StripeCustomerId: "cus_1",
	}))

	// No StripeCustomerId — exercises the branch that skips the secondary update.
	wantErr("UpsertSubscription/noCustomer", s.UpsertSubscription(ctx, &pbuser.SubscriptionState{UserId: "u1"}))

	_, err = s.GetUserIDByStripeCustomer(ctx, "cus_1")
	wantErr("GetUserIDByStripeCustomer", err)

	now := time.Now()
	wantErr("UpdateUserTier/withTrial", s.UpdateUserTier(ctx, "u1", pbuser.UserTier_USER_TIER_ATHLETE, &now))
	wantErr("UpdateUserTier/noTrial", s.UpdateUserTier(ctx, "u1", pbuser.UserTier_USER_TIER_HOBBYIST, nil))

	_, err = s.ListUsersWithTrialsEndingBetween(ctx, now, now.Add(time.Hour))
	wantErr("ListUsersWithTrialsEndingBetween", err)

	wantErr("SetTrialWarningSent", s.SetTrialWarningSent(ctx, "u1", now))
	wantErr("SetTrialExpiredNotified", s.SetTrialExpiredNotified(ctx, "u1", now))

	_, _, _, err = s.GetTierStatus(ctx, "u1")
	wantErr("GetTierStatus", err)
}

// TestLegacyTierMap asserts the legacy tier string mapping used by GetTierStatus.
func TestLegacyTierMap(t *testing.T) {
	cases := map[string]pbuser.UserTier{
		"hobbyist": pbuser.UserTier_USER_TIER_HOBBYIST,
		"athlete":  pbuser.UserTier_USER_TIER_ATHLETE,
	}
	for in, want := range cases {
		if got, ok := legacyTierMap[in]; !ok || got != want {
			t.Errorf("legacyTierMap[%q] = %v (ok=%v), want %v", in, got, ok, want)
		}
	}
	if _, ok := legacyTierMap["unknown"]; ok {
		t.Error("legacyTierMap should not contain 'unknown'")
	}
}
