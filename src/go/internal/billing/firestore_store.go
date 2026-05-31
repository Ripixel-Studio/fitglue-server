// nolint:proto-json
package billing

import (
	"context"
	"encoding/json"
	"time"

	"cloud.google.com/go/firestore"
	pbuser "github.com/fitglue/server/src/go/pkg/types/pb/models/user"
	"google.golang.org/api/iterator"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/encoding/protojson"
)

type FirestoreStore struct {
	client *firestore.Client
}

func NewFirestoreStore(client *firestore.Client) *FirestoreStore {
	return &FirestoreStore{client: client}
}

// GetSubscription reads billing state from the user's billing subcollection
func (s *FirestoreStore) GetSubscription(ctx context.Context, userID string) (*pbuser.SubscriptionState, error) {
	doc, err := s.client.Collection("users").Doc(userID).Collection("billing").Doc("subscription").Get(ctx)
	if err != nil {
		if status.Code(err) == codes.NotFound {
			return nil, nil // Not found is not an error, just no subscription yet
		}
		return nil, err
	}

	b, err := json.Marshal(doc.Data())
	if err != nil {
		return nil, err
	}

	var state pbuser.SubscriptionState
	err = protojson.UnmarshalOptions{DiscardUnknown: true}.Unmarshal(b, &state)
	if err != nil {
		return nil, err
	}
	return &state, nil
}

func (s *FirestoreStore) UpsertSubscription(ctx context.Context, sub *pbuser.SubscriptionState) error {
	b, err := protojson.MarshalOptions{EmitUnpopulated: true, UseProtoNames: true}.Marshal(sub)
	if err != nil {
		return err
	}

	var data map[string]interface{}
	if err := json.Unmarshal(b, &data); err != nil {
		return err
	}

	_, err = s.client.Collection("users").Doc(sub.UserId).Collection("billing").Doc("subscription").Set(ctx, data, firestore.MergeAll)
	if err != nil {
		return err
	}

	// Also ensure stripeCustomerId is synced to the root user doc for legacy/query purposes
	if sub.StripeCustomerId != "" {
		_, _ = s.client.Collection("users").Doc(sub.UserId).Update(ctx, []firestore.Update{
			{Path: "stripeCustomerId", Value: sub.StripeCustomerId},
		})
	}

	return nil
}

func (s *FirestoreStore) GetUserIDByStripeCustomer(ctx context.Context, customerID string) (string, error) {
	iter := s.client.Collection("users").Where("stripeCustomerId", "==", customerID).Limit(1).Documents(ctx)
	defer iter.Stop()

	doc, err := iter.Next()
	if err == iterator.Done {
		return "", status.Error(codes.NotFound, "user not found for stripe customer")
	}
	if err != nil {
		return "", err
	}

	return doc.Ref.ID, nil
}

// legacyTierMap maps short tier strings from old TypeScript code to proto enum values.
var legacyTierMap = map[string]pbuser.UserTier{
	"hobbyist": pbuser.UserTier_USER_TIER_HOBBYIST,
	"athlete":  pbuser.UserTier_USER_TIER_ATHLETE,
}

func (s *FirestoreStore) UpdateUserTier(ctx context.Context, userID string, tier pbuser.UserTier, trialEndsAt *time.Time) error {
	updates := []firestore.Update{
		{Path: "tier", Value: tier.String()}, // Write as proto enum name for protojson compat
	}

	if trialEndsAt == nil {
		updates = append(updates, firestore.Update{Path: "trial_ends_at", Value: firestore.Delete})
	} else {
		updates = append(updates, firestore.Update{Path: "trial_ends_at", Value: *trialEndsAt})
	}

	_, err := s.client.Collection("users").Doc(userID).Update(ctx, updates)
	return err
}

func (s *FirestoreStore) ListUsersWithTrialsEndingBetween(ctx context.Context, from, to time.Time) ([]TrialUser, error) {
	iter := s.client.Collection("users").
		Where("trial_ends_at", ">=", from).
		Where("trial_ends_at", "<", to).
		Documents(ctx)
	defer iter.Stop()

	var users []TrialUser
	for {
		doc, err := iter.Next()
		if err == iterator.Done {
			break
		}
		if err != nil {
			return nil, err
		}

		trialEnd, ok := doc.Data()["trial_ends_at"].(time.Time)
		if !ok {
			continue
		}

		u := TrialUser{
			UserID:      doc.Ref.ID,
			TrialEndsAt: trialEnd,
		}
		if t, ok := doc.Data()["trial_warning_sent_at"].(time.Time); ok {
			u.WarningSentAt = &t
		}
		if t, ok := doc.Data()["trial_expired_notified_at"].(time.Time); ok {
			u.ExpiredNotifiedAt = &t
		}
		users = append(users, u)
	}
	return users, nil
}

func (s *FirestoreStore) SetTrialWarningSent(ctx context.Context, userID string, at time.Time) error {
	_, err := s.client.Collection("users").Doc(userID).Update(ctx, []firestore.Update{
		{Path: "trial_warning_sent_at", Value: at},
	})
	return err
}

func (s *FirestoreStore) SetTrialExpiredNotified(ctx context.Context, userID string, at time.Time) error {
	_, err := s.client.Collection("users").Doc(userID).Update(ctx, []firestore.Update{
		{Path: "trial_expired_notified_at", Value: at},
	})
	return err
}

func (s *FirestoreStore) GetTierStatus(ctx context.Context, userID string) (pbuser.UserTier, bool, *time.Time, error) {
	doc, err := s.client.Collection("users").Doc(userID).Get(ctx)
	if err != nil {
		return pbuser.UserTier_USER_TIER_UNSPECIFIED, false, nil, err
	}

	var tier pbuser.UserTier = pbuser.UserTier_USER_TIER_HOBBYIST
	if t, err := doc.DataAt("tier"); err == nil {
		switch v := t.(type) {
		case int64:
			tier = pbuser.UserTier(v)
		case string:
			// Handle legacy short strings (e.g., "hobbyist", "athlete")
			if mapped, ok := legacyTierMap[v]; ok {
				tier = mapped
			} else if enumVal, ok := pbuser.UserTier_value[v]; ok {
				// Handle proto enum name strings (e.g., "USER_TIER_HOBBYIST")
				tier = pbuser.UserTier(enumVal)
			}
		}
	}

	isAdmin := false
	if a, err := doc.DataAt("is_admin"); err == nil {
		if adminBool, ok := a.(bool); ok {
			isAdmin = adminBool
		}
	}

	var trialEnds *time.Time
	if tr, err := doc.DataAt("trial_ends_at"); err == nil {
		if tTime, ok := tr.(time.Time); ok {
			trialEnds = &tTime
		}
	}

	return tier, isAdmin, trialEnds, nil
}
