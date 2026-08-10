// nolint:proto-json
package activity

import (
	"context"
	"encoding/json"
	"time"

	"cloud.google.com/go/firestore"
	firestorepb "cloud.google.com/go/firestore/apiv1/firestorepb"
	pbactivity "github.com/fitglue/server/src/go/pkg/types/pb/models/activity"
	pbpipeline "github.com/fitglue/server/src/go/pkg/types/pb/models/pipeline"
	"google.golang.org/api/iterator"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/encoding/protojson"
	"google.golang.org/protobuf/reflect/protoreflect"
	"google.golang.org/protobuf/types/known/timestamppb"
)

type FirestoreStore struct {
	client *firestore.Client
}

func NewFirestoreStore(client *firestore.Client) *FirestoreStore {
	return &FirestoreStore{client: client}
}

func (s *FirestoreStore) GetActivity(ctx context.Context, userID, activityID string) (*pbactivity.StandardizedActivity, error) {
	doc, err := s.client.Collection("users").Doc(userID).Collection("activities").Doc(activityID).Get(ctx)
	if err != nil {
		if status.Code(err) == codes.NotFound {
			return nil, nil
		}
		return nil, err
	}
	var act pbactivity.StandardizedActivity
	if err := decodeProtoMap(doc.Data(), &act); err != nil {
		return nil, err
	}
	return &act, nil
}

// maxListLimit caps a caller-supplied page size so one request can't scan an
// unbounded slice of the collection.
const maxListLimit = 200

// paginate applies the shared cursor scheme to a created_at-descending
// collection query: fetch limit+1 docs, return the first limit, and hand back
// the last *returned* doc's ID as the next-page cursor. The page window is
// positional over fetched docs, so rows skipped by the caller's decode step
// don't shift subsequent pages. An unknown cursor doc is an error — silently
// restarting from the first page would make paginating clients loop forever.
func paginate(ctx context.Context, col *firestore.CollectionRef, limit int32, pageToken string,
	decode func(doc *firestore.DocumentSnapshot)) (string, error) {
	if limit <= 0 {
		limit = 50
	}
	if limit > maxListLimit {
		limit = maxListLimit
	}

	query := col.OrderBy("created_at", firestore.Desc).Limit(int(limit) + 1)
	if pageToken != "" {
		cursorDoc, err := col.Doc(pageToken).Get(ctx)
		if err != nil {
			return "", status.Errorf(codes.InvalidArgument, "invalid page_token: %v", err)
		}
		query = query.StartAfter(cursorDoc)
	}

	iter := query.Documents(ctx)
	defer iter.Stop()

	var fetched int32
	var lastReturnedID string
	for {
		doc, err := iter.Next()
		if err == iterator.Done {
			return "", nil // fewer than limit+1 docs — no next page
		}
		if err != nil {
			return "", err
		}
		fetched++
		if fetched > limit {
			// The limit+1 sentinel doc: don't return it, just signal a next page
			// starting after the last doc we did return.
			return lastReturnedID, nil
		}
		lastReturnedID = doc.Ref.ID
		decode(doc)
	}
}

func (s *FirestoreStore) ListActivities(ctx context.Context, userID string, limit int32, pageToken string) ([]*pbactivity.StandardizedActivity, string, error) {
	col := s.client.Collection("users").Doc(userID).Collection("activities")
	var activities []*pbactivity.StandardizedActivity
	var decodeErr error
	next, err := paginate(ctx, col, limit, pageToken, func(doc *firestore.DocumentSnapshot) {
		var act pbactivity.StandardizedActivity
		if err := decodeProtoMap(doc.Data(), &act); err != nil {
			decodeErr = err
			return
		}
		activities = append(activities, &act)
	})
	if err != nil {
		return nil, "", err
	}
	if decodeErr != nil {
		return nil, "", decodeErr
	}
	return activities, next, nil
}
func (s *FirestoreStore) ListPipelineRuns(ctx context.Context, userID string, limit int32, pageToken string) ([]*pbpipeline.PipelineRun, string, error) {
	col := s.client.Collection("users").Doc(userID).Collection("pipeline_runs")
	var runs []*pbpipeline.PipelineRun
	next, err := paginate(ctx, col, limit, pageToken, func(doc *firestore.DocumentSnapshot) {
		var run pbpipeline.PipelineRun
		if err := decodeProtoMap(doc.Data(), &run); err != nil {
			// A single malformed record (e.g. a legacy/corrupt timestamp) must not
			// abort the whole listing — skip it so exports and activity lists are
			// resilient to bad rows. The pagination window is positional, so a
			// skipped row doesn't shift later pages.
			return
		}
		runs = append(runs, &run)
	})
	if err != nil {
		return nil, "", err
	}
	return runs, next, nil
}
func (s *FirestoreStore) DeleteActivity(ctx context.Context, userID, activityID string) error {
	_, err := s.client.Collection("users").Doc(userID).Collection("activities").Doc(activityID).Delete(ctx)
	return err
}
func (s *FirestoreStore) GetShowcase(ctx context.Context, userID, showcaseID string) (*pbactivity.ShowcasedActivity, error) {
	doc, err := s.client.Collection("showcased_activities").Doc(showcaseID).Get(ctx)
	if err != nil {
		if status.Code(err) == codes.NotFound {
			return nil, nil
		}
		return nil, err
	}
	var act pbactivity.ShowcasedActivity
	if err := decodeProtoMap(doc.Data(), &act); err != nil {
		return nil, err
	}
	return &act, nil
}
func (s *FirestoreStore) ListShowcases(ctx context.Context, userID string) ([]*pbactivity.ShowcaseProfileEntry, error) {
	iter := s.client.Collection("showcased_activities").
		Where("user_id", "==", userID).
		OrderBy("created_at", firestore.Desc).
		Documents(ctx)
	defer iter.Stop()

	var showcases []*pbactivity.ShowcaseProfileEntry
	for {
		doc, err := iter.Next()
		if err == iterator.Done {
			break
		}
		if err != nil {
			return nil, err
		}
		// Documents are stored as ShowcasedActivity — decode that, then project
		// the fields the caller needs into ShowcaseProfileEntry.
		var act pbactivity.ShowcasedActivity
		if err := decodeProtoMap(doc.Data(), &act); err != nil {
			return nil, err
		}
		showcases = append(showcases, &pbactivity.ShowcaseProfileEntry{
			ShowcaseId:   act.ShowcaseId,
			Title:        act.Title,
			ActivityType: act.ActivityType,
			Source:       act.Source,
			StartTime:    act.StartTime,
			CreatedAt:    act.CreatedAt,
		})
	}
	return showcases, nil
}
func (s *FirestoreStore) CreateShowcase(ctx context.Context, userID string, showcase *pbactivity.ShowcasedActivity) (*pbactivity.ShowcasedActivity, error) {
	data, err := encodeProtoMap(showcase)
	if err != nil {
		return nil, err
	}
	// user_id is no longer a proto field (removed for security) but is still required
	// as a Firestore index field for user-scoped queries.
	data["user_id"] = userID
	_, err = s.client.Collection("showcased_activities").Doc(showcase.ShowcaseId).Set(ctx, data)
	return showcase, err
}
func (s *FirestoreStore) UpdateShowcase(ctx context.Context, userID string, showcase *pbactivity.ShowcasedActivity) (*pbactivity.ShowcasedActivity, error) {
	data, err := encodeProtoMap(showcase)
	if err != nil {
		return nil, err
	}
	// user_id is no longer a proto field (removed for security) but is still required
	// as a Firestore index field for user-scoped queries.
	data["user_id"] = userID
	_, err = s.client.Collection("showcased_activities").Doc(showcase.ShowcaseId).Set(ctx, data)
	return showcase, err
}
func (s *FirestoreStore) DeleteShowcase(ctx context.Context, userID, showcaseID string) error {
	_, err := s.client.Collection("showcased_activities").Doc(showcaseID).Delete(ctx)
	return err
}

func (s *FirestoreStore) GetShowcasePreferences(ctx context.Context, userID string) (*pbactivity.ShowcaseProfile, error) {
	doc, err := s.client.Collection("users").Doc(userID).Collection("settings").Doc("showcase_profile").Get(ctx)
	if err != nil {
		if status.Code(err) == codes.NotFound {
			return nil, nil
		}
		return nil, err
	}
	var prefs pbactivity.ShowcaseProfile
	if err := decodeProtoMap(doc.Data(), &prefs); err != nil {
		return nil, err
	}
	return &prefs, nil
}

func (s *FirestoreStore) UpdateShowcasePreferences(ctx context.Context, userID string, prefs *pbactivity.ShowcaseProfile) (*pbactivity.ShowcaseProfile, error) {
	data, err := encodeProtoMap(prefs)
	if err != nil {
		return nil, err
	}
	_, err = s.client.Collection("users").Doc(userID).Collection("settings").Doc("showcase_profile").Set(ctx, data, firestore.MergeAll)
	return prefs, err
}

// PatchShowcaseProfile writes only the provided fields to the showcase profile document.
// This avoids overwriting unrelated sections when the frontend sends a partial update.
func (s *FirestoreStore) PatchShowcaseProfile(ctx context.Context, userID string, fields map[string]interface{}) (*pbactivity.ShowcaseProfile, error) {
	_, err := s.client.Collection("users").Doc(userID).Collection("settings").Doc("showcase_profile").Set(ctx, fields, firestore.MergeAll)
	if err != nil {
		return nil, err
	}
	return s.GetShowcasePreferences(ctx, userID)
}

func (s *FirestoreStore) GetPublicShowcase(ctx context.Context, showcaseID string) (*pbactivity.ShowcasedActivity, string, error) {
	doc, err := s.client.Collection("showcased_activities").Doc(showcaseID).Get(ctx)
	if err != nil {
		if status.Code(err) == codes.NotFound {
			return nil, "", nil
		}
		return nil, "", err
	}
	data := doc.Data()
	var act pbactivity.ShowcasedActivity
	if err := decodeProtoMap(data, &act); err != nil {
		return nil, "", err
	}
	ownerUserID, _ := data["user_id"].(string)
	return &act, ownerUserID, nil
}
func (s *FirestoreStore) GetPipelineRun(ctx context.Context, userID, runID string) (*pbpipeline.PipelineRun, error) {
	doc, err := s.client.Collection("users").Doc(userID).Collection("pipeline_runs").Doc(runID).Get(ctx)
	if err != nil {
		if status.Code(err) == codes.NotFound {
			return nil, nil
		}
		return nil, err
	}
	var run pbpipeline.PipelineRun
	if err := decodeProtoMap(doc.Data(), &run); err != nil {
		return nil, err
	}
	return &run, nil
}
func (s *FirestoreStore) DeletePipelineRun(ctx context.Context, userID, runID string) error {
	_, err := s.client.Collection("users").Doc(userID).Collection("pipeline_runs").Doc(runID).Delete(ctx)
	return err
}

func (s *FirestoreStore) UpdateShowcaseSlug(ctx context.Context, userID, slug string) error {
	// Check if slug is already taken by querying the slugs index
	slugDoc, err := s.client.Collection("showcase_slugs").Doc(slug).Get(ctx)
	if err == nil && slugDoc.Exists() {
		data := slugDoc.Data()
		if ownerID, ok := data["user_id"].(string); ok && ownerID != userID {
			return status.Error(codes.AlreadyExists, "slug is already taken")
		}
	}

	// Set the showcase profile's slug (create-or-update safe via MergeAll)
	_, err = s.client.Collection("users").Doc(userID).Collection("settings").Doc("showcase_profile").Set(ctx, map[string]interface{}{
		"slug": slug,
	}, firestore.MergeAll)
	if err != nil {
		return err
	}

	// Reserve the slug in the lookup collection
	_, err = s.client.Collection("showcase_slugs").Doc(slug).Set(ctx, map[string]interface{}{
		"user_id": userID,
	})
	return err
}

func (s *FirestoreStore) GetShowcaseProfileBySlug(ctx context.Context, slug string) (*pbactivity.ShowcaseProfile, error) {
	// Look up the user ID from the slug index
	slugDoc, err := s.client.Collection("showcase_slugs").Doc(slug).Get(ctx)
	if err != nil {
		if status.Code(err) == codes.NotFound {
			return nil, nil
		}
		return nil, err
	}

	data := slugDoc.Data()
	userID, ok := data["user_id"].(string)
	if !ok || userID == "" {
		return nil, nil
	}

	// Fetch the profile
	return s.GetShowcasePreferences(ctx, userID)
}

func (s *FirestoreStore) ListShowcasedActivitiesByUser(ctx context.Context, userID string, limit int32, offset int32) ([]*pbactivity.ShowcasedActivity, int32, error) {
	if limit <= 0 {
		limit = 20
	}

	// Uses the showcased_activities_user_created index (user_id ASC, created_at DESC)
	query := s.client.Collection("showcased_activities").
		Where("user_id", "==", userID).
		OrderBy("created_at", firestore.Desc).
		Limit(int(limit))

	if offset > 0 {
		query = query.Offset(int(offset))
	}

	iter := query.Documents(ctx)
	defer iter.Stop()

	var activities []*pbactivity.ShowcasedActivity
	for {
		doc, err := iter.Next()
		if err == iterator.Done {
			break
		}
		if err != nil {
			return nil, 0, err
		}
		var act pbactivity.ShowcasedActivity
		if err := decodeProtoMap(doc.Data(), &act); err != nil {
			return nil, 0, err
		}
		activities = append(activities, &act)
	}

	return activities, int32(len(activities)), nil
}

func (s *FirestoreStore) CountPipelineRunsByStatus(ctx context.Context, userID, pipelineStatus string) (int32, error) {
	// Uses pipeline_runs_status_created index (status ASC, created_at DESC)
	// Pipeline runs are at users/{userId}/pipeline_runs
	var q firestore.Query
	if pipelineStatus != "" {
		q = s.client.Collection("users").Doc(userID).Collection("pipeline_runs").
			Where("status", "==", pipelineStatus)
	} else {
		q = s.client.Collection("users").Doc(userID).Collection("pipeline_runs").
			OrderBy("created_at", firestore.Desc)
	}

	countResult, err := q.NewAggregationQuery().WithCount("total").Get(ctx)
	if err != nil {
		return 0, err
	}

	total, ok := countResult["total"]
	if !ok {
		return 0, nil
	}
	if pbVal, ok := total.(*firestorepb.Value); ok {
		return int32(pbVal.GetIntegerValue()), nil
	}
	return 0, nil
}

func (s *FirestoreStore) CountShowcasedActivities(ctx context.Context, userID string) (int32, error) {
	// Uses showcased_activities_user_created index (user_id ASC, created_at DESC)
	q := s.client.Collection("showcased_activities").
		Where("user_id", "==", userID)

	countResult, err := q.NewAggregationQuery().WithCount("total").Get(ctx)
	if err != nil {
		return 0, err
	}

	total, ok := countResult["total"]
	if !ok {
		return 0, nil
	}
	if pbVal, ok := total.(*firestorepb.Value); ok {
		return int32(pbVal.GetIntegerValue()), nil
	}
	return 0, nil
}

func (s *FirestoreStore) CountBillingEvents(ctx context.Context, userID string) (int32, error) {
	q := s.client.Collection("users").Doc(userID).Collection("billing_events")
	countResult, err := q.NewAggregationQuery().WithCount("total").Get(ctx)
	if err != nil {
		return 0, err
	}
	total, ok := countResult["total"]
	if !ok {
		return 0, nil
	}
	if pbVal, ok := total.(*firestorepb.Value); ok {
		return int32(pbVal.GetIntegerValue()), nil
	}
	return 0, nil
}

func (s *FirestoreStore) CountBillingEventsForPeriod(ctx context.Context, userID, period string) (int32, error) {
	q := s.client.Collection("users").Doc(userID).Collection("billing_events").Where("period", "==", period)
	countResult, err := q.NewAggregationQuery().WithCount("total").Get(ctx)
	if err != nil {
		return 0, err
	}
	total, ok := countResult["total"]
	if !ok {
		return 0, nil
	}
	if pbVal, ok := total.(*firestorepb.Value); ok {
		return int32(pbVal.GetIntegerValue()), nil
	}
	return 0, nil
}

func (s *FirestoreStore) CountBillingEventsSince(ctx context.Context, userID string, since time.Time) (int32, error) {
	q := s.client.Collection("users").Doc(userID).Collection("billing_events").Where("created_at", ">=", since)
	countResult, err := q.NewAggregationQuery().WithCount("total").Get(ctx)
	if err != nil {
		return 0, err
	}
	total, ok := countResult["total"]
	if !ok {
		return 0, nil
	}
	if pbVal, ok := total.(*firestorepb.Value); ok {
		return int32(pbVal.GetIntegerValue()), nil
	}
	return 0, nil
}

func (s *FirestoreStore) CountDistinctActivitiesForPeriod(ctx context.Context, userID, period string) (int32, error) {
	iter := s.client.Collection("users").Doc(userID).Collection("billing_events").
		Where("period", "==", period).
		Select("activity_id").
		Documents(ctx)
	defer iter.Stop()

	seen := make(map[string]struct{})
	for {
		doc, err := iter.Next()
		if err == iterator.Done {
			break
		}
		if err != nil {
			return 0, err
		}
		if id, ok := doc.Data()["activity_id"].(string); ok && id != "" {
			seen[id] = struct{}{}
		}
	}
	return int32(len(seen)), nil
}

func (s *FirestoreStore) CountDistinctActivitiesSince(ctx context.Context, userID string, since time.Time) (int32, error) {
	iter := s.client.Collection("users").Doc(userID).Collection("billing_events").
		Where("created_at", ">=", since).
		Select("activity_id").
		Documents(ctx)
	defer iter.Stop()

	seen := make(map[string]struct{})
	for {
		doc, err := iter.Next()
		if err == iterator.Done {
			break
		}
		if err != nil {
			return 0, err
		}
		if id, ok := doc.Data()["activity_id"].(string); ok && id != "" {
			seen[id] = struct{}{}
		}
	}
	return int32(len(seen)), nil
}

// entryCollectionRef returns the sub-collection ref for showcase profile entries.
func (s *FirestoreStore) entryCollectionRef(userID string) *firestore.CollectionRef {
	return s.client.Collection("users").Doc(userID).Collection("showcase_profile_entries")
}

func (s *FirestoreStore) ListShowcaseProfileEntries(ctx context.Context, userID string) ([]*pbactivity.ShowcaseProfileEntry, error) {
	iter := s.entryCollectionRef(userID).
		OrderBy("start_time", firestore.Desc).
		Documents(ctx)
	defer iter.Stop()

	var entries []*pbactivity.ShowcaseProfileEntry
	for {
		doc, err := iter.Next()
		if err == iterator.Done {
			break
		}
		if err != nil {
			return nil, err
		}
		var entry pbactivity.ShowcaseProfileEntry
		if err := decodeProtoMap(doc.Data(), &entry); err != nil {
			return nil, err
		}
		entries = append(entries, &entry)
	}
	return entries, nil
}

func (s *FirestoreStore) SetShowcaseProfileEntry(ctx context.Context, userID string, entry *pbactivity.ShowcaseProfileEntry) error {
	data, err := encodeProtoMap(entry)
	if err != nil {
		return err
	}
	_, err = s.entryCollectionRef(userID).Doc(entry.ShowcaseId).Set(ctx, data, firestore.MergeAll)
	return err
}

func (s *FirestoreStore) DeleteShowcaseProfileEntry(ctx context.Context, userID, showcaseID string) error {
	_, err := s.entryCollectionRef(userID).Doc(showcaseID).Delete(ctx)
	return err
}

// firestorePRDoc is a minimal local struct for reading personal_records documents.
// The user service writes these as proto-JSON, so fields use snake_case JSON keys.
type firestorePRDoc struct {
	RecordType    string   `json:"record_type"`
	Value         float64  `json:"value"`
	Unit          string   `json:"unit"`
	AchievedAt    string   `json:"achieved_at,omitempty"`
	PreviousValue *float64 `json:"previous_value,omitempty"`
}

func (s *FirestoreStore) ListUserPersonalRecords(ctx context.Context, userID string) ([]*pbactivity.ShowcaseTopPR, error) {
	iter := s.client.Collection("users").Doc(userID).Collection("personal_records").Documents(ctx)
	defer iter.Stop()

	var result []*pbactivity.ShowcaseTopPR
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
			continue
		}
		var raw firestorePRDoc
		if err := json.Unmarshal(b, &raw); err != nil || raw.RecordType == "" {
			continue
		}

		pr := &pbactivity.ShowcaseTopPR{
			RecordType:    raw.RecordType,
			Value:         raw.Value,
			Unit:          raw.Unit,
			PreviousValue: raw.PreviousValue,
		}
		if raw.AchievedAt != "" {
			if t, parseErr := time.Parse(time.RFC3339Nano, raw.AchievedAt); parseErr == nil {
				pr.AchievedAt = timestamppb.New(t)
			}
		}
		result = append(result, pr)
	}
	return result, nil
}

func (s *FirestoreStore) GetRoundup(ctx context.Context, slug, periodKey string) (*pbactivity.ShowcaseRoundup, error) {
	id := slug + "-" + periodKey
	doc, err := s.client.Collection("showcased_roundups").Doc(id).Get(ctx)
	if err != nil {
		if status.Code(err) == codes.NotFound {
			return nil, nil
		}
		return nil, err
	}
	var r pbactivity.ShowcaseRoundup
	if err := decodeProtoMap(doc.Data(), &r); err != nil {
		return nil, err
	}
	return &r, nil
}

func (s *FirestoreStore) SetRoundup(ctx context.Context, roundup *pbactivity.ShowcaseRoundup) error {
	id := roundup.Slug + "-" + roundup.PeriodKey
	data, err := encodeProtoMap(roundup)
	if err != nil {
		return err
	}
	_, err = s.client.Collection("showcased_roundups").Doc(id).Set(ctx, data)
	return err
}

func (s *FirestoreStore) ListRecentRoundups(ctx context.Context, slug string, limit int) ([]*pbactivity.ShowcaseRoundup, error) {
	if limit <= 0 {
		limit = 3
	}
	iter := s.client.Collection("showcased_roundups").
		Where("slug", "==", slug).
		OrderBy("period_end", firestore.Desc).
		Limit(limit).
		Documents(ctx)
	defer iter.Stop()

	var results []*pbactivity.ShowcaseRoundup
	for {
		doc, err := iter.Next()
		if err == iterator.Done {
			break
		}
		if err != nil {
			return nil, err
		}
		var r pbactivity.ShowcaseRoundup
		if err := decodeProtoMap(doc.Data(), &r); err != nil {
			return nil, err
		}
		results = append(results, &r)
	}
	return results, nil
}

// ListAllRoundups returns every roundup for slug, newest first — used for
// in-memory pagination in GetRecentPublicRoundups, mirroring the
// GetPublicShowcaseProfile pattern until real Firestore offset pagination
// is needed here.
func (s *FirestoreStore) ListAllRoundups(ctx context.Context, slug string) ([]*pbactivity.ShowcaseRoundup, error) {
	iter := s.client.Collection("showcased_roundups").
		Where("slug", "==", slug).
		OrderBy("period_end", firestore.Desc).
		Documents(ctx)
	defer iter.Stop()

	var results []*pbactivity.ShowcaseRoundup
	for {
		doc, err := iter.Next()
		if err == iterator.Done {
			break
		}
		if err != nil {
			return nil, err
		}
		var r pbactivity.ShowcaseRoundup
		if err := decodeProtoMap(doc.Data(), &r); err != nil {
			return nil, err
		}
		results = append(results, &r)
	}
	return results, nil
}

func (s *FirestoreStore) ListShowcaseEntriesInRange(ctx context.Context, userID string, from, to time.Time) ([]*pbactivity.ShowcaseProfileEntry, error) {
	// start_time is stored as a protojson RFC3339 string, not a Firestore Timestamp,
	// so range queries against time.Time values don't work. Load all entries and filter in Go.
	all, err := s.ListShowcaseProfileEntries(ctx, userID)
	if err != nil {
		return nil, err
	}
	var entries []*pbactivity.ShowcaseProfileEntry
	for _, e := range all {
		if e.StartTime == nil {
			continue
		}
		t := e.StartTime.AsTime()
		if !t.Before(from) && t.Before(to) {
			entries = append(entries, e)
		}
	}
	return entries, nil
}

// ListAllShowcaseUserIDs returns all user IDs that have a showcase profile, via the slug index.
func (s *FirestoreStore) ListAllShowcaseUserIDs(ctx context.Context) ([]string, error) {
	iter := s.client.Collection("showcase_slugs").Documents(ctx)
	defer iter.Stop()

	seen := make(map[string]struct{})
	var userIDs []string
	for {
		doc, err := iter.Next()
		if err == iterator.Done {
			break
		}
		if err != nil {
			return nil, err
		}
		data := doc.Data()
		userID, _ := data["user_id"].(string)
		if userID == "" {
			continue
		}
		if _, ok := seen[userID]; !ok {
			seen[userID] = struct{}{}
			userIDs = append(userIDs, userID)
		}
	}
	return userIDs, nil
}

// Helpers
func encodeProtoMap(msg protoreflect.ProtoMessage) (map[string]interface{}, error) {
	b, err := protojson.MarshalOptions{EmitUnpopulated: true, UseProtoNames: true}.Marshal(msg)
	if err != nil {
		return nil, err
	}
	var m map[string]interface{}
	err = json.Unmarshal(b, &m)
	return m, err
}

func decodeProtoMap(m map[string]interface{}, msg protoreflect.ProtoMessage) error {
	b, err := json.Marshal(m)
	if err != nil {
		return err
	}
	return protojson.UnmarshalOptions{DiscardUnknown: true}.Unmarshal(b, msg)
}
