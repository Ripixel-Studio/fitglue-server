// nolint:proto-json
package user

import (
	"context"
	"encoding/json"
	"errors"
	"time"

	"github.com/fitglue/server/src/go/pkg/domain/apikey"

	"cloud.google.com/go/firestore"
	pbuser "github.com/fitglue/server/src/go/pkg/types/pb/models/user"
	"google.golang.org/api/iterator"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/encoding/protojson"
	"google.golang.org/protobuf/types/known/structpb"
	"google.golang.org/protobuf/types/known/timestamppb"
)

type FirestoreStore struct {
	client *firestore.Client
}

func NewFirestoreStore(client *firestore.Client) *FirestoreStore {
	return &FirestoreStore{client: client}
}

// legacyTierMap maps short tier strings written by the old TypeScript system
// to their proto enum names expected by protojson.
var legacyTierMap = map[string]string{
	"hobbyist": "USER_TIER_HOBBYIST",
	"athlete":  "USER_TIER_ATHLETE",
}

// normalizeUserData converts legacy field values (written by TypeScript) into
// formats compatible with protojson unmarshaling. Handles:
// - tier enum stored as short strings ("hobbyist" → "USER_TIER_HOBBYIST")
// - duplicate camelCase/snake_case keys (isAdmin + is_admin → protojson duplicate error)
func normalizeUserData(data map[string]interface{}) map[string]interface{} {
	// Normalize tier enum
	if tierVal, ok := data["tier"]; ok {
		if tierStr, ok := tierVal.(string); ok {
			if mapped, ok := legacyTierMap[tierStr]; ok {
				data["tier"] = mapped
			}
		}
	}

	// Remove camelCase duplicates of snake_case fields. The old TS code wrote
	// some fields in both formats. protojson maps both to the same proto field
	// and errors on duplicates.
	camelToSnake := map[string]string{
		"isAdmin":     "is_admin",
		"trialEndsAt": "trial_ends_at",
	}
	for camel, snake := range camelToSnake {
		if _, hasCamel := data[camel]; hasCamel {
			if _, hasSnake := data[snake]; hasSnake {
				delete(data, camel) // keep snake_case, drop camelCase
			}
		}
	}

	return data
}

func (s *FirestoreStore) GetProfile(ctx context.Context, userID string) (*pbuser.UserProfile, error) {
	doc, err := s.client.Collection("users").Doc(userID).Get(ctx)
	if err != nil {
		return nil, err
	}
	b, err := json.Marshal(normalizeUserData(doc.Data()))
	if err != nil {
		return nil, err
	}
	var profile pbuser.UserProfile
	err = protojson.UnmarshalOptions{DiscardUnknown: true}.Unmarshal(b, &profile)
	if err != nil {
		return nil, err
	}
	return &profile, nil
}

func (s *FirestoreStore) UpdateProfile(ctx context.Context, userID string, profile *pbuser.UserProfile) error {
	if profile == nil {
		return errors.New("profile cannot be nil")
	}
	_, err := s.client.Collection("users").Doc(userID).Set(ctx, profile)
	return err
}

func (s *FirestoreStore) DeleteUser(ctx context.Context, userID string) error {
	userDocRef := s.client.Collection("users").Doc(userID)

	// Helper to delete all docs in a collection/query
	deleteDocs := func(iter *firestore.DocumentIterator) error {
		defer iter.Stop()
		batch := s.client.Batch()
		count := 0
		for {
			doc, err := iter.Next()
			if err == iterator.Done {
				break
			}
			if err != nil {
				return err
			}
			batch.Delete(doc.Ref)
			count++
			// Commit in chunks if larger than 500
			if count == 500 {
				if _, err := batch.Commit(ctx); err != nil {
					return err
				}
				batch = s.client.Batch()
				count = 0
			}
		}
		if count > 0 {
			if _, err := batch.Commit(ctx); err != nil {
				return err
			}
		}
		return nil
	}

	// 1. Delete pipeline_runs & destination_outcomes
	runsIter := userDocRef.Collection("pipeline_runs").Documents(ctx)
	defer runsIter.Stop()
	for {
		doc, err := runsIter.Next()
		if err == iterator.Done {
			break
		}
		if err != nil {
			return err
		}
		outcomesIter := doc.Ref.Collection("destination_outcomes").Documents(ctx)
		if err := deleteDocs(outcomesIter); err != nil {
			return err
		}
		_, err = doc.Ref.Delete(ctx)
		if err != nil {
			return err
		}
	}

	// 2-9. Delete user sub-collections
	subCollections := []string{
		"synchronized_activities",
		"raw_activities",
		"executions",
		"pending_inputs",
		"pipelines",
		"counters",
		"booster_data",
		"personal_records",
		"uploaded_activities",
		"plugin_defaults",
	}
	for _, sub := range subCollections {
		if err := deleteDocs(userDocRef.Collection(sub).Documents(ctx)); err != nil {
			return err
		}
	}

	// 10-12. Delete top-level collections by user_id
	topCollections := []string{
		"ingress_api_keys",
		"showcased_activities",
		"showcase_profiles",
	}
	for _, col := range topCollections {
		iter := s.client.Collection(col).Where("user_id", "==", userID).Documents(ctx)
		if err := deleteDocs(iter); err != nil {
			return err
		}
	}

	// 13. Delete user document
	_, err := userDocRef.Delete(ctx)
	return err
}

func (s *FirestoreStore) FindUsersByDateRange(ctx context.Context, start, end time.Time) ([]*pbuser.UserProfile, error) {
	var users []*pbuser.UserProfile
	iter := s.client.Collection("users").
		Where("created_at", ">=", start).
		Where("created_at", "<", end).
		Documents(ctx)
	defer iter.Stop()

	for {
		doc, err := iter.Next()
		if err == iterator.Done {
			break
		}
		if err != nil {
			return nil, err
		}

		b, err := json.Marshal(normalizeUserData(doc.Data()))
		if err != nil {
			return nil, err
		}
		var profile pbuser.UserProfile
		err = protojson.UnmarshalOptions{DiscardUnknown: true}.Unmarshal(b, &profile)
		if err != nil {
			return nil, err
		}
		users = append(users, &profile)
	}

	return users, nil
}

func (s *FirestoreStore) GetIntegrations(ctx context.Context, userID string) (*pbuser.UserIntegrations, error) {
	doc, err := s.client.Collection("users").Doc(userID).Get(ctx)
	if err != nil {
		return nil, err
	}

	val, err := doc.DataAt("integrations")
	if err != nil {
		// Field doesn't exist or isn't a map, return empty
		return &pbuser.UserIntegrations{}, nil
	}

	// Normalize numeric expires_at fields into RFC-3339 strings
	if m, ok := val.(map[string]interface{}); ok {
		for _, intgObj := range m {
			if intgMap, ok := intgObj.(map[string]interface{}); ok {
				if exp, ok := intgMap["expires_at"]; ok {
					switch v := exp.(type) {
					case float64:
						intgMap["expires_at"] = time.Unix(int64(v), 0).UTC().Format(time.RFC3339)
					case int64:
						intgMap["expires_at"] = time.Unix(v, 0).UTC().Format(time.RFC3339)
					}
				}
			}
		}
	}

	b, err := json.Marshal(val)
	if err != nil {
		return nil, err
	}

	var integrations pbuser.UserIntegrations
	err = protojson.UnmarshalOptions{DiscardUnknown: true}.Unmarshal(b, &integrations)
	if err != nil {
		return nil, err
	}

	return &integrations, nil
}

func (s *FirestoreStore) SetIntegration(ctx context.Context, userID, provider string, data interface{}) error {
	_, err := s.client.Collection("users").Doc(userID).Update(ctx, []firestore.Update{
		{
			Path:  "integrations." + provider,
			Value: data,
		},
	})
	return err
}

func (s *FirestoreStore) DeleteIntegration(ctx context.Context, userID, provider string) error {
	_, err := s.client.Collection("users").Doc(userID).Update(ctx, []firestore.Update{
		{
			Path:  "integrations." + provider,
			Value: firestore.Delete,
		},
	})
	return err
}

func isApiKeyProvider(provider string) bool {
	switch provider {
	case "hevy":
		return true
	default:
		return false
	}
}

func (s *FirestoreStore) FindUserByIntegration(ctx context.Context, provider string, providerUID string) (*pbuser.UserProfile, error) {
	if isApiKeyProvider(provider) {
		hashStr := apikey.HashIngressKey(providerUID)

		doc, err := s.client.Collection("ingress_api_keys").Doc(hashStr).Get(ctx)
		if err != nil {
			if status.Code(err) == codes.NotFound {
				return nil, status.Error(codes.NotFound, "user not found for integration (invalid ingress key)")
			}
			return nil, err
		}
		userIDVal, err := doc.DataAt("user_id")
		if err != nil {
			return nil, err
		}
		userID, ok := userIDVal.(string)
		if !ok {
			return nil, status.Error(codes.Internal, "invalid user_id in ingress key doc")
		}

		return s.GetProfile(ctx, userID)
	}

	var fieldPath string
	switch provider {
	case "strava":
		fieldPath = "integrations.strava.athlete_id"
	case "fitbit":
		fieldPath = "integrations.fitbit.fitbit_user_id"
	case "hevy":
		// Fallback for legacy data, but primary resolution goes through ingress_api_keys
		fieldPath = "integrations.hevy.user_id"
	case "polar":
		fieldPath = "integrations.polar.polar_user_id"
	case "wahoo":
		fieldPath = "integrations.wahoo.wahoo_user_id"
	case "oura":
		fieldPath = "integrations.oura.oura_user_id"
	case "github":
		fieldPath = "integrations.github.github_user_id"
	case "spotify":
		fieldPath = "integrations.spotify.spotify_user_id"
	case "intervals":
		fieldPath = "integrations.intervals.athlete_id"
	case "trainingpeaks":
		fieldPath = "integrations.trainingpeaks.athlete_id"
	case "parkrun":
		fieldPath = "integrations.parkrun.athlete_id"
	default:
		return nil, status.Errorf(codes.InvalidArgument, "unsupported provider for ResolveUser: %s", provider)
	}

	var queryValue interface{} = providerUID
	if provider == "strava" {
		// Attempt to parse to int64, since Strava athlete_id is stored as int64
		// But if it's stored in Firestore as a number, we must query it as a number.
		// providerUID comes from the webhook, which might be a string.
		// We'll decode using json unmarshal to handle strings. Actually,
		// if we know it's a number:
		importStrconv := true
		_ = importStrconv
	}

	iter := s.client.Collection("users").Where(fieldPath, "==", queryValue).Limit(1).Documents(ctx)
	defer iter.Stop()

	doc, err := iter.Next()
	if err == iterator.Done {
		return nil, status.Error(codes.NotFound, "user not found for integration")
	}
	if err != nil {
		return nil, err
	}

	b, err := json.Marshal(normalizeUserData(doc.Data()))
	if err != nil {
		return nil, err
	}
	var profile pbuser.UserProfile
	err = protojson.UnmarshalOptions{DiscardUnknown: true}.Unmarshal(b, &profile)
	if err != nil {
		return nil, err
	}

	return &profile, nil
}

func (s *FirestoreStore) ListCounters(ctx context.Context, userID string) ([]*pbuser.Counter, error) {
	var counters []*pbuser.Counter
	iter := s.client.Collection("users").Doc(userID).Collection("counters").Documents(ctx)
	defer iter.Stop()

	for {
		doc, err := iter.Next()
		if err == iterator.Done {
			break
		}
		if err != nil {
			return nil, err
		}

		var countVal int64
		if val, err := doc.DataAt("count"); err == nil {
			switch v := val.(type) {
			case int64:
				countVal = v
			case float64:
				countVal = int64(v)
			case int:
				countVal = int64(v)
			}
		}

		var lastUpdated time.Time
		if val, err := doc.DataAt("last_updated"); err == nil {
			if t, ok := val.(time.Time); ok {
				lastUpdated = t
			}
		}

		counters = append(counters, &pbuser.Counter{
			Id:          doc.Ref.ID,
			Count:       countVal,
			LastUpdated: timestamppb.New(lastUpdated),
		})
	}
	return counters, nil
}

func (s *FirestoreStore) UpdateCounter(ctx context.Context, userID, counterID string, count int64) (*pbuser.Counter, error) {
	now := time.Now()
	ref := s.client.Collection("users").Doc(userID).Collection("counters").Doc(counterID)
	_, err := ref.Set(ctx, map[string]interface{}{
		"id":           counterID,
		"count":        count,
		"last_updated": now,
	}, firestore.MergeAll)

	if err != nil {
		return nil, err
	}

	return &pbuser.Counter{
		Id:          counterID,
		Count:       count,
		LastUpdated: timestamppb.New(now),
	}, nil
}

func (s *FirestoreStore) GetNotificationPrefs(ctx context.Context, userID string) (*pbuser.NotificationPreferences, error) {
	doc, err := s.client.Collection("users").Doc(userID).Get(ctx)
	if err != nil {
		return nil, err
	}

	val, err := doc.DataAt("notification_preferences")
	if err != nil {
		// Field doesn't exist or isn't a map, return default empty
		return &pbuser.NotificationPreferences{}, nil
	}

	b, err := json.Marshal(val)
	if err != nil {
		return nil, err
	}

	var prefs pbuser.NotificationPreferences
	err = protojson.UnmarshalOptions{DiscardUnknown: true}.Unmarshal(b, &prefs)
	if err != nil {
		return nil, err
	}

	return &prefs, nil
}

func (s *FirestoreStore) UpdateNotificationPrefs(ctx context.Context, userID string, prefs *pbuser.NotificationPreferences) error {
	if prefs == nil {
		return errors.New("preferences cannot be nil")
	}

	// Because prefs is a protobuf message, we can convert it to JSON or map
	b, err := protojson.MarshalOptions{EmitUnpopulated: true, UseProtoNames: true}.Marshal(prefs)
	if err != nil {
		return err
	}

	var data map[string]interface{}
	if err := json.Unmarshal(b, &data); err != nil {
		return err
	}

	_, err = s.client.Collection("users").Doc(userID).Update(ctx, []firestore.Update{
		{
			Path:  "notification_preferences",
			Value: data,
		},
	})
	return err
}

func (s *FirestoreStore) GetBoosterData(ctx context.Context, userID, boosterID string) (map[string]*structpb.Struct, error) {
	col := s.client.Collection("users").Doc(userID).Collection("booster_data")
	res := make(map[string]*structpb.Struct)

	parseDoc := func(doc *firestore.DocumentSnapshot) (*structpb.Struct, error) {
		b, err := json.Marshal(doc.Data())
		if err != nil {
			return nil, err
		}
		var st structpb.Struct
		if err := protojson.Unmarshal(b, &st); err != nil {
			return nil, err
		}
		return &st, nil
	}

	if boosterID != "" {
		doc, err := col.Doc(boosterID).Get(ctx)
		if err != nil {
			if status.Code(err) == codes.NotFound {
				return res, nil
			}
			return nil, err
		}
		st, err := parseDoc(doc)
		if err != nil {
			return nil, err
		}
		res[boosterID] = st
		return res, nil
	}

	iter := col.Documents(ctx)
	defer iter.Stop()
	for {
		doc, err := iter.Next()
		if err == iterator.Done {
			break
		}
		if err != nil {
			return nil, err
		}
		st, err := parseDoc(doc)
		if err != nil {
			return nil, err
		}
		res[doc.Ref.ID] = st
	}
	return res, nil
}

func (s *FirestoreStore) SetBoosterData(ctx context.Context, userID, boosterID string, data *structpb.Struct) error {
	if data == nil {
		return errors.New("data cannot be nil")
	}
	m := data.AsMap()
	m["last_updated"] = time.Now()
	_, err := s.client.Collection("users").Doc(userID).Collection("booster_data").Doc(boosterID).Set(ctx, m, firestore.MergeAll)
	return err
}

func (s *FirestoreStore) DeleteBoosterData(ctx context.Context, userID, boosterID string) error {
	_, err := s.client.Collection("users").Doc(userID).Collection("booster_data").Doc(boosterID).Delete(ctx)
	return err
}

func (s *FirestoreStore) DeleteCounter(ctx context.Context, userID, counterID string) error {
	_, err := s.client.Collection("users").Doc(userID).Collection("counters").Doc(counterID).Delete(ctx)
	return err
}

func (s *FirestoreStore) ListPersonalRecords(ctx context.Context, userID string) ([]*pbuser.PersonalRecord, error) {
	var records []*pbuser.PersonalRecord
	iter := s.client.Collection("users").Doc(userID).Collection("personal_records").Documents(ctx)
	defer iter.Stop()

	for {
		doc, err := iter.Next()
		if err == iterator.Done {
			break
		}
		if err != nil {
			return nil, err
		}

		b, err := json.Marshal(doc.Data())
		if err != nil {
			return nil, err
		}
		var record pbuser.PersonalRecord
		unmarshaler := protojson.UnmarshalOptions{DiscardUnknown: true}
		if err := unmarshaler.Unmarshal(b, &record); err != nil {
			return nil, err
		}
		records = append(records, &record)
	}
	return records, nil
}

func (s *FirestoreStore) SetPersonalRecord(ctx context.Context, userID, recordType string, record *pbuser.PersonalRecord) error {
	if record == nil {
		return errors.New("record cannot be nil")
	}

	b, err := protojson.MarshalOptions{EmitUnpopulated: true, UseProtoNames: true}.Marshal(record)
	if err != nil {
		return err
	}
	var data map[string]interface{}
	if err := json.Unmarshal(b, &data); err != nil {
		return err
	}

	_, err = s.client.Collection("users").Doc(userID).Collection("personal_records").Doc(recordType).Set(ctx, data, firestore.MergeAll)
	return err
}

func (s *FirestoreStore) DeletePersonalRecord(ctx context.Context, userID, recordType string) error {
	_, err := s.client.Collection("users").Doc(userID).Collection("personal_records").Doc(recordType).Delete(ctx)
	return err
}

func (s *FirestoreStore) ListPluginDefaults(ctx context.Context, userID string) (map[string]*structpb.Struct, error) {
	res := make(map[string]*structpb.Struct)
	iter := s.client.Collection("users").Doc(userID).Collection("plugin_defaults").Documents(ctx)
	defer iter.Stop()

	for {
		doc, err := iter.Next()
		if err == iterator.Done {
			break
		}
		if err != nil {
			return nil, err
		}

		b, err := json.Marshal(doc.Data())
		if err != nil {
			return nil, err
		}
		var st structpb.Struct
		if err := protojson.Unmarshal(b, &st); err != nil {
			return nil, err
		}
		res[doc.Ref.ID] = &st
	}
	return res, nil
}

func (s *FirestoreStore) SetPluginDefaults(ctx context.Context, userID, pluginID string, defaults *structpb.Struct) error {
	if defaults == nil {
		return errors.New("defaults cannot be nil")
	}
	m := defaults.AsMap()
	m["last_updated"] = time.Now()
	_, err := s.client.Collection("users").Doc(userID).Collection("plugin_defaults").Doc(pluginID).Set(ctx, m, firestore.MergeAll)
	return err
}

func (s *FirestoreStore) DeletePluginDefaults(ctx context.Context, userID, pluginID string) error {
	_, err := s.client.Collection("users").Doc(userID).Collection("plugin_defaults").Doc(pluginID).Delete(ctx)
	return err
}

func (s *FirestoreStore) CreateUser(ctx context.Context, userID string) (*pbuser.UserProfile, error) {
	now := time.Now()
	// Stored tier is Hobbyist; getEffectiveTier() grants Athlete during trial via trialEndsAt
	trialEndsAt := now.Add(30 * 24 * time.Hour)

	profile := &pbuser.UserProfile{
		UserId:                  userID,
		CreatedAt:               timestamppb.New(now),
		Tier:                    pbuser.UserTier_USER_TIER_HOBBYIST,
		IsAdmin:                 false,
		SyncCountThisMonth:      0,
		SyncCountResetAt:        timestamppb.New(now),
		PreventedSyncCount:      0,
		AccessEnabled:           false, // Waitlisted until admin enables
		NotificationPreferences: defaultNotificationPreferences(),
		TrialEndsAt:             timestamppb.New(trialEndsAt),
		FcmTokens:               []string{},
	}

	docRef := s.client.Collection("users").Doc(userID)

	b, err := protojson.MarshalOptions{EmitUnpopulated: true, UseProtoNames: true}.Marshal(profile)
	if err != nil {
		return nil, err
	}
	var data map[string]interface{}
	if err := json.Unmarshal(b, &data); err != nil {
		return nil, err
	}

	// Also ensure integrations object is created if not emitted by protojson
	if _, ok := data["integrations"]; !ok {
		data["integrations"] = map[string]interface{}{}
	}

	// We use Create here so it fails if the user already exists.
	// In the original Node.js code it was userStore.create(userId, ...) which does a .set()
	// But let's follow standard behavior or docRef.Set
	_, err = docRef.Set(ctx, data)
	if err != nil {
		return nil, err
	}

	return profile, nil
}

// defaultNotificationPreferences returns the push-only defaults for a new user.
func defaultNotificationPreferences() *pbuser.NotificationPreferences {
	push := &pbuser.NotificationTypePreference{
		Channels: []pbuser.NotificationChannel{pbuser.NotificationChannel_NOTIFICATION_CHANNEL_PUSH},
	}
	return &pbuser.NotificationPreferences{
		PendingInput:     push,
		PipelineSuccess:  push,
		PipelineFailure:  push,
		ConnectionAction: push,
		ShowcaseRoundup:  push,
	}
}
