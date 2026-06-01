package database

import (
	"context"
	"fmt"
	"strings"
	"time"

	"cloud.google.com/go/firestore"
	shared "github.com/fitglue/server/src/go/pkg"
	"github.com/fitglue/server/src/go/pkg/domain/user"
	pbplugin "github.com/fitglue/server/src/go/pkg/types/pb/models/plugin"
	pbuser "github.com/fitglue/server/src/go/pkg/types/pb/models/user"

	storage "github.com/fitglue/server/src/go/pkg/storage/firestore"

	pbactivity "github.com/fitglue/server/src/go/pkg/types/pb/models/activity"

	pbpipeline "github.com/fitglue/server/src/go/pkg/types/pb/models/pipeline"

	"google.golang.org/protobuf/types/known/timestamppb"
)

// FirestoreAdapter provides database operations using Firestore
// It wraps our typed storage client
type FirestoreAdapter struct {
	Client  *firestore.Client
	storage *storage.Client // internal typed wrapper
}

func NewFirestoreAdapter(client *firestore.Client) *FirestoreAdapter {
	return &FirestoreAdapter{
		Client:  client, // Keep raw client accessible if needed? OR remove it if unused.
		storage: storage.NewClient(client),
	}
}

func (a *FirestoreAdapter) SetExecution(ctx context.Context, record *pbpipeline.ExecutionRecord) error {
	userId := record.GetUserId()
	if userId == "" {
		// ORPHANED: No userId - this is a code smell that should be investigated
		// Store in orphaned_executions collection for alerting
		return a.storage.OrphanedExecutions().Doc(record.ExecutionId).Set(ctx, record)
	}
	// Use typed storage with user sub-collection for direct Firestore client access
	return a.storage.UserExecutions(userId).Doc(record.ExecutionId).Set(ctx, record)
}

func (a *FirestoreAdapter) UpdateExecution(ctx context.Context, userId string, id string, data map[string]interface{}) error {
	if userId == "" {
		// ORPHANED: No userId - update in orphaned_executions collection
		return a.storage.OrphanedExecutions().Doc(id).Update(ctx, data)
	}
	// Use user sub-collection for direct Firestore client access
	return a.storage.UserExecutions(userId).Doc(id).Update(ctx, data)
}

func (a *FirestoreAdapter) GetUser(ctx context.Context, id string) (*user.Record, error) {
	doc, err := a.storage.Users().Doc(id).Get(ctx)
	if err != nil {
		return nil, err
	}
	// Manually populate ID since it's the doc key
	doc.UserId = id
	return doc, nil
}

func (a *FirestoreAdapter) UpdateUser(ctx context.Context, id string, data map[string]interface{}) error {
	return a.storage.Users().Doc(id).Update(ctx, data)
}

// --- Billing Events (durable audit log of destination uploads) ---

// RecordBillingEvent writes a billing_events document and increments sync_count_this_month.
// It replaces the standalone IncrementSyncCount call in destination uploaders.
func (a *FirestoreAdapter) RecordBillingEvent(ctx context.Context, userID string, event shared.BillingEvent) error {
	period := time.Now().Format("2006-01")
	data := map[string]interface{}{
		"activity_id":     event.ActivityID,
		"pipeline_run_id": event.PipelineRunID,
		"pipeline_id":     event.PipelineID,
		"source":          event.Source,
		"destination":     event.Destination,
		"period":          period,
		"created_at":      time.Now(),
	}
	_, _, err := a.Client.Collection("users").Doc(userID).Collection("billing_events").Add(ctx, data)
	if err != nil {
		return err
	}
	_, err = a.Client.Collection("users").Doc(userID).Update(ctx, []firestore.Update{
		{Path: "sync_count_this_month", Value: firestore.Increment(1)},
	})
	return err
}

// CountBillingEvents returns the all-time count of successful destination uploads for a user.
func (a *FirestoreAdapter) CountBillingEvents(ctx context.Context, userID string) (int32, error) {
	q := a.Client.Collection("users").Doc(userID).Collection("billing_events")
	result, err := q.NewAggregationQuery().WithCount("total").Get(ctx)
	if err != nil {
		return 0, err
	}
	total, ok := result["total"]
	if !ok {
		return 0, nil
	}
	if v, ok := total.(int64); ok {
		return int32(v), nil
	}
	return 0, nil
}

// CountBillingEventsForPeriod returns the count of successful destination uploads for a given month.
// period should be formatted as "YYYY-MM" (e.g. "2026-05").
func (a *FirestoreAdapter) CountBillingEventsForPeriod(ctx context.Context, userID, period string) (int32, error) {
	q := a.Client.Collection("users").Doc(userID).Collection("billing_events").Where("period", "==", period)
	result, err := q.NewAggregationQuery().WithCount("total").Get(ctx)
	if err != nil {
		return 0, err
	}
	total, ok := result["total"]
	if !ok {
		return 0, nil
	}
	if v, ok := total.(int64); ok {
		return int32(v), nil
	}
	return 0, nil
}

// --- Sync Count (for tier limits) ---

func (a *FirestoreAdapter) IncrementSyncCount(ctx context.Context, userID string) error {
	_, err := a.Client.Collection("users").Doc(userID).Update(ctx, []firestore.Update{
		{Path: "sync_count_this_month", Value: firestore.Increment(1)},
	})
	return err
}

func (a *FirestoreAdapter) IncrementPreventedSyncCount(ctx context.Context, userID string) error {
	_, err := a.Client.Collection("users").Doc(userID).Update(ctx, []firestore.Update{
		{Path: "prevented_sync_count", Value: firestore.Increment(1)},
	})
	return err
}

func (a *FirestoreAdapter) ResetSyncCount(ctx context.Context, userID string) error {
	_, err := a.Client.Collection("users").Doc(userID).Update(ctx, []firestore.Update{
		{Path: "sync_count_this_month", Value: 0},
		{Path: "sync_count_reset_at", Value: firestore.ServerTimestamp},
	})
	return err
}

// --- Pending Inputs ---

func (a *FirestoreAdapter) GetPendingInput(ctx context.Context, userId string, id string) (*pbpipeline.PendingInput, error) {
	doc, err := a.storage.UserPendingInputs(userId).Doc(id).Get(ctx)
	if err != nil {
		return nil, err
	}
	return doc, nil
}

func (a *FirestoreAdapter) CreatePendingInput(ctx context.Context, userId string, input *pbpipeline.PendingInput) error {
	// Use Set to handle potential retries/race conditions
	// Store in user sub-collection for direct Firestore client access
	return a.storage.UserPendingInputs(userId).Doc(input.ActivityId).Set(ctx, input)
}

func (a *FirestoreAdapter) UpdatePendingInput(ctx context.Context, userId string, id string, data map[string]interface{}) error {
	return a.storage.UserPendingInputs(userId).Doc(id).Update(ctx, data)
}

// DeletePendingInput removes a pending input document
func (a *FirestoreAdapter) DeletePendingInput(ctx context.Context, userId string, id string) error {
	_, err := a.Client.Collection("users").Doc(userId).Collection("pending_inputs").Doc(id).Delete(ctx)
	return err
}

func (a *FirestoreAdapter) ListPendingInputs(ctx context.Context, userID string) ([]*pbpipeline.PendingInput, error) {
	// Query user sub-collection directly - no need for where clause on user_id
	iter := a.Client.Collection("users").Doc(userID).Collection("pending_inputs").Documents(ctx)
	docs, err := iter.GetAll()
	if err != nil {
		return nil, err
	}

	var results []*pbpipeline.PendingInput
	for _, d := range docs {
		// Manually convert using our converter
		m := d.Data()
		p := storage.FirestoreToPendingInput(m)
		// Ensure ActivityID is set from doc ID if missing (though it should be in data)
		if p.ActivityId == "" {
			p.ActivityId = d.Ref.ID
		}
		results = append(results, p)
	}
	return results, nil
}

// --- Counters ---

func (a *FirestoreAdapter) GetCounter(ctx context.Context, userId string, id string) (*pbuser.Counter, error) {
	doc, err := a.storage.Counters(userId).Doc(id).Get(ctx)
	if err != nil {
		return nil, err
	}
	doc.Id = id
	return doc, nil
}

func (a *FirestoreAdapter) SetCounter(ctx context.Context, userId string, counter *pbuser.Counter) error {
	// Set (overwrite/create)
	return a.storage.Counters(userId).Doc(counter.Id).Set(ctx, counter)
}

// ListCounters returns all counters for a user
func (a *FirestoreAdapter) ListCounters(ctx context.Context, userId string) ([]*pbuser.Counter, error) {
	iter := a.Client.Collection("users").Doc(userId).Collection("counters").Documents(ctx)
	docs, err := iter.GetAll()
	if err != nil {
		return nil, err
	}

	var counters []*pbuser.Counter
	for _, d := range docs {
		m := d.Data()
		counter := storage.FirestoreToCounter(m)
		if counter.Id == "" {
			counter.Id = d.Ref.ID
		}
		counters = append(counters, counter)
	}
	return counters, nil
}

// DeleteCounter removes a counter by ID
func (a *FirestoreAdapter) DeleteCounter(ctx context.Context, userId string, id string) error {
	_, err := a.Client.Collection("users").Doc(userId).Collection("counters").Doc(id).Delete(ctx)
	return err
}

// --- Personal Records ---

// GetPersonalRecord retrieves a personal record by type
func (a *FirestoreAdapter) GetPersonalRecord(ctx context.Context, userId string, recordType string) (*pbuser.PersonalRecord, error) {
	doc, err := a.storage.PersonalRecords(userId).Doc(recordType).Get(ctx)
	if err != nil {
		return nil, err
	}
	doc.RecordType = recordType
	return doc, nil
}

// SetPersonalRecord creates or updates a personal record
func (a *FirestoreAdapter) SetPersonalRecord(ctx context.Context, userId string, record *pbuser.PersonalRecord) error {
	return a.storage.PersonalRecords(userId).Doc(record.RecordType).Set(ctx, record)
}

// ListPersonalRecords returns all personal records for a user
func (a *FirestoreAdapter) ListPersonalRecords(ctx context.Context, userId string) ([]*pbuser.PersonalRecord, error) {
	iter := a.Client.Collection("users").Doc(userId).Collection("personal_records").Documents(ctx)
	docs, err := iter.GetAll()
	if err != nil {
		return nil, err
	}

	var records []*pbuser.PersonalRecord
	for _, d := range docs {
		m := d.Data()
		record := storage.FirestoreToPersonalRecord(m)
		if record.RecordType == "" {
			record.RecordType = d.Ref.ID
		}
		records = append(records, record)
	}
	return records, nil
}

// DeletePersonalRecord removes a personal record by type
func (a *FirestoreAdapter) DeletePersonalRecord(ctx context.Context, userId string, recordType string) error {
	_, err := a.Client.Collection("users").Doc(userId).Collection("personal_records").Doc(recordType).Delete(ctx)
	return err
}

func (a *FirestoreAdapter) ListPendingInputsByEnricher(ctx context.Context, enricherId string, status pbpipeline.PendingInput_Status) ([]*pbpipeline.PendingInput, error) {
	// Query across all pending inputs using collection group query
	iter := a.Client.CollectionGroup("pending_inputs").
		Where("enricher_provider_id", "==", enricherId).
		Where("status", "==", int32(status)).
		Documents(ctx)

	docs, err := iter.GetAll()
	if err != nil {
		return nil, err
	}

	var inputs []*pbpipeline.PendingInput
	for _, d := range docs {
		m := d.Data()
		input := storage.FirestoreToPendingInput(m)
		if input.ActivityId == "" {
			input.ActivityId = d.Ref.ID
		}
		inputs = append(inputs, input)
	}

	return inputs, nil
}

// --- Showcased Activities (public shareable snapshots) ---

// ShowcaseActivityExists checks if a showcase ID already exists
func (a *FirestoreAdapter) ShowcaseActivityExists(ctx context.Context, showcaseId string) (bool, error) {
	_, err := a.storage.ShowcasedActivities().Doc(showcaseId).Ref.Get(ctx)
	if err != nil {
		// Check if it's a "not found" error
		if err.Error() == "rpc error: code = NotFound desc = Document not found" ||
			err.Error() == "document not found" ||
			isNotFoundError(err) {
			return false, nil
		}
		return false, err
	}
	return true, nil
}

// SetShowcasedActivity creates or updates a showcased activity.
// userID is stored as a top-level field for the ListShowcases Firestore query (user_id filter).
func (a *FirestoreAdapter) SetShowcasedActivity(ctx context.Context, userID string, activity *pbactivity.ShowcasedActivity) error {
	data := storage.ShowcasedActivityToFirestore(activity)
	data["user_id"] = userID
	_, err := a.Client.Collection("showcased_activities").Doc(activity.ShowcaseId).Set(ctx, data)
	return err
}

// GetShowcasedActivity retrieves a showcased activity by ID
func (a *FirestoreAdapter) GetShowcasedActivity(ctx context.Context, showcaseId string) (*pbactivity.ShowcasedActivity, error) {
	activity, err := a.storage.ShowcasedActivities().Doc(showcaseId).Get(ctx)
	if err != nil {
		return nil, err
	}
	// Ensure showcase ID is set
	if activity != nil && activity.ShowcaseId == "" {
		activity.ShowcaseId = showcaseId
	}
	return activity, nil
}

// --- Showcase Profiles (materialized user profile for homepage) ---

// SetShowcaseProfile creates or updates a showcase profile
func (a *FirestoreAdapter) SetShowcaseProfile(ctx context.Context, profile *pbactivity.ShowcaseProfile) error {
	return a.storage.ShowcaseProfiles().Doc(profile.Slug).Set(ctx, profile)
}

// GetShowcaseProfile retrieves a showcase profile by slug
func (a *FirestoreAdapter) GetShowcaseProfile(ctx context.Context, slug string) (*pbactivity.ShowcaseProfile, error) {
	profile, err := a.storage.ShowcaseProfiles().Doc(slug).Get(ctx)
	if err != nil {
		return nil, err
	}
	// Ensure slug is set
	if profile != nil && profile.Slug == "" {
		profile.Slug = slug
	}
	return profile, nil
}

// GetShowcaseProfileByUserId finds a showcase profile by the user_id field
func (a *FirestoreAdapter) GetShowcaseProfileByUserId(ctx context.Context, userId string) (*pbactivity.ShowcaseProfile, error) {
	col := a.storage.ShowcaseProfiles()
	iter := col.Ref.Where("user_id", "==", userId).Limit(1).Documents(ctx)
	defer iter.Stop()
	doc, err := iter.Next()
	if err != nil {
		return nil, nil // Not found or error - treat as no existing profile
	}
	profile := col.FromFirestore(doc.Data())
	if profile != nil && profile.Slug == "" {
		profile.Slug = doc.Ref.ID
	}
	return profile, nil
}

// DeleteShowcaseProfile deletes a showcase profile by slug
func (a *FirestoreAdapter) DeleteShowcaseProfile(ctx context.Context, slug string) error {
	_, err := a.storage.ShowcaseProfiles().Ref.Doc(slug).Delete(ctx)
	return err
}

// SetShowcaseProfileEntry writes an entry to the user's showcase_profile_entries sub-collection
func (a *FirestoreAdapter) SetShowcaseProfileEntry(ctx context.Context, userID string, entry *pbactivity.ShowcaseProfileEntry) error {
	return a.storage.ShowcaseProfileEntries(userID).Doc(entry.ShowcaseId).Set(ctx, entry)
}

// isNotFoundError checks if error is a Firestore not found error
func isNotFoundError(err error) bool {
	if err == nil {
		return false
	}
	errStr := err.Error()
	return strings.Contains(errStr, "NotFound") || strings.Contains(errStr, "not found")
}

// --- Pipelines (Sub-collection) ---

// GetUserPipelines retrieves all pipelines for a user from the sub-collection
func (a *FirestoreAdapter) GetUserPipelines(ctx context.Context, userId string) ([]*pbpipeline.PipelineConfig, error) {
	iter := a.Client.Collection("users").Doc(userId).Collection("pipelines").Documents(ctx)
	docs, err := iter.GetAll()
	if err != nil {
		return nil, err
	}

	pipelines := make([]*pbpipeline.PipelineConfig, len(docs))
	for i, doc := range docs {
		pipelines[i] = storage.FirestoreToPipeline(doc.Data())
		// Ensure ID is set from doc ID if missing
		if pipelines[i].Id == "" {
			pipelines[i].Id = doc.Ref.ID
		}
	}

	return pipelines, nil
}

// --- Plugin Defaults (user-level default config for sources/destinations) ---

// GetPluginDefault retrieves a plugin default by plugin ID
func (a *FirestoreAdapter) GetPluginDefault(ctx context.Context, userId string, pluginId string) (*pbpipeline.PluginDefault, error) {
	doc, err := a.storage.PluginDefaults(userId).Doc(pluginId).Get(ctx)
	if err != nil {
		if isNotFoundError(err) {
			return nil, nil // Not found - return nil (no default set)
		}
		return nil, err
	}
	if doc != nil && doc.PluginId == "" {
		doc.PluginId = pluginId
	}
	return doc, nil
}

// SetPluginDefault creates or updates a plugin default
func (a *FirestoreAdapter) SetPluginDefault(ctx context.Context, userId string, pluginDefault *pbpipeline.PluginDefault) error {
	return a.storage.PluginDefaults(userId).Doc(pluginDefault.PluginId).Set(ctx, pluginDefault)
}

// --- Uploaded Activities (for loop prevention) ---

// SetUploadedActivity records that an activity was uploaded to a destination.
// Used for loop prevention: when a webhook comes back, we check if we just uploaded it.
func (a *FirestoreAdapter) SetUploadedActivity(ctx context.Context, userId string, record *pbactivity.UploadedActivityRecord) error {
	return a.storage.UploadedActivities(userId).Doc(record.Id).Set(ctx, record)
}

// GetUploadedActivity retrieves an uploaded activity record by destination and destination ID.
// Uses a direct document lookup by the composite ID (e.g. "hevy:workout-xyz") — faster than
// a compound query and requires no composite Firestore index.
func (a *FirestoreAdapter) GetUploadedActivity(ctx context.Context, userId string, destination pbplugin.DestinationType, destinationId string) (*pbactivity.UploadedActivityRecord, error) {
	docId := buildUploadedActivityID(destination, destinationId)
	snap, err := a.Client.Collection("users").Doc(userId).Collection("uploaded_activities").Doc(docId).Get(ctx)
	if err != nil {
		if isNotFound(err) {
			return nil, nil
		}
		return nil, err
	}
	record := storage.FirestoreToUploadedActivity(snap.Data())
	if record.Id == "" {
		record.Id = snap.Ref.ID
	}
	return record, nil
}

// buildUploadedActivityID mirrors loopprevention.BuildUploadedActivityID without the import cycle.
func buildUploadedActivityID(destination pbplugin.DestinationType, destinationId string) string {
	destName := strings.TrimPrefix(destination.String(), "DESTINATION_")
	return strings.ToLower(destName) + ":" + destinationId
}

// isNotFound returns true for Firestore "document not found" errors.
func isNotFound(err error) bool {
	return strings.Contains(err.Error(), "NotFound") || strings.Contains(err.Error(), "not found")
}

// --- Pipeline Runs (lifecycle tracking) ---

// CreatePipelineRun creates a new pipeline run document
func (a *FirestoreAdapter) CreatePipelineRun(ctx context.Context, userId string, run *pbpipeline.PipelineRun) error {
	return a.storage.PipelineRuns(userId).Doc(run.Id).Set(ctx, run)
}

// GetPipelineRun retrieves a pipeline run by ID
func (a *FirestoreAdapter) GetPipelineRun(ctx context.Context, userId string, id string) (*pbpipeline.PipelineRun, error) {
	run, err := a.storage.PipelineRuns(userId).Doc(id).Get(ctx)
	if err != nil {
		return nil, err
	}
	// Ensure ID is set
	if run != nil && run.Id == "" {
		run.Id = id
	}
	return run, nil
}

// GetPipelineRunByActivityId retrieves the most recent pipeline run for an activity
// Returns nil (not an error) if no run found for the activity
func (a *FirestoreAdapter) GetPipelineRunByActivityId(ctx context.Context, userId string, activityId string) (*pbpipeline.PipelineRun, error) {
	iter := a.Client.Collection("users").Doc(userId).Collection("pipeline_runs").
		Where("activity_id", "==", activityId).
		OrderBy("created_at", firestore.Desc).
		Limit(1).
		Documents(ctx)

	docs, err := iter.GetAll()
	if err != nil {
		return nil, err
	}

	if len(docs) == 0 {
		return nil, nil // Not found - not an error
	}

	m := docs[0].Data()
	run := storage.FirestoreToPipelineRun(m)
	if run.Id == "" {
		run.Id = docs[0].Ref.ID
	}
	return run, nil
}

// UpdatePipelineRun updates specific fields on a pipeline run
func (a *FirestoreAdapter) UpdatePipelineRun(ctx context.Context, userId string, id string, data map[string]interface{}) error {
	return a.storage.PipelineRuns(userId).Doc(id).Update(ctx, data)
}

// --- Destination Outcomes (subcollection of Pipeline Runs) ---
// Each destination outcome is stored as a separate document to avoid race conditions
// when multiple uploaders update their status in parallel.

// SetDestinationOutcome writes a destination outcome to the subcollection
// Document ID is the destination enum value (e.g., "1" for STRAVA, "2" for SHOWCASE)
func (a *FirestoreAdapter) SetDestinationOutcome(ctx context.Context, userId string, pipelineRunId string, outcome *pbpipeline.DestinationOutcome) error {
	docId := fmt.Sprintf("%d", outcome.Destination)
	data := map[string]interface{}{
		"destination": int32(outcome.Destination),
		"status":      int32(outcome.Status),
		"updated_at":  time.Now(),
	}
	if outcome.ExternalId != nil {
		data["external_id"] = *outcome.ExternalId
	}
	if outcome.Error != nil {
		data["error"] = *outcome.Error
	}
	if outcome.CompletedAt != nil {
		data["completed_at"] = outcome.CompletedAt.AsTime()
	}

	_, err := a.Client.Collection("users").Doc(userId).
		Collection("pipeline_runs").Doc(pipelineRunId).
		Collection("destination_outcomes").Doc(docId).
		Set(ctx, data, firestore.MergeAll)
	return err
}

// GetDestinationOutcomes retrieves all destination outcomes for a pipeline run
func (a *FirestoreAdapter) GetDestinationOutcomes(ctx context.Context, userId string, pipelineRunId string) ([]*pbpipeline.DestinationOutcome, error) {
	iter := a.Client.Collection("users").Doc(userId).
		Collection("pipeline_runs").Doc(pipelineRunId).
		Collection("destination_outcomes").
		Documents(ctx)

	docs, err := iter.GetAll()
	if err != nil {
		return nil, err
	}

	outcomes := make([]*pbpipeline.DestinationOutcome, 0, len(docs))
	for _, doc := range docs {
		m := doc.Data()
		outcome := &pbpipeline.DestinationOutcome{}

		if v, ok := m["destination"]; ok {
			switch val := v.(type) {
			case int64:
				outcome.Destination = pbplugin.DestinationType(val)
			case float64:
				outcome.Destination = pbplugin.DestinationType(int32(val))
			}
		}
		if v, ok := m["status"]; ok {
			switch val := v.(type) {
			case int64:
				outcome.Status = pbpipeline.DestinationStatus(val)
			case float64:
				outcome.Status = pbpipeline.DestinationStatus(int32(val))
			}
		}
		if v, ok := m["external_id"].(string); ok {
			outcome.ExternalId = &v
		}
		if v, ok := m["error"].(string); ok {
			outcome.Error = &v
		}
		if v, ok := m["completed_at"].(time.Time); ok {
			outcome.CompletedAt = timestamppb.New(v)
		}

		outcomes = append(outcomes, outcome)
	}

	return outcomes, nil
}

// --- Booster Data (generic key-value storage for enrichers) ---

// GetBoosterData retrieves booster-specific data by ID
func (a *FirestoreAdapter) GetBoosterData(ctx context.Context, userId string, boosterId string) (map[string]interface{}, error) {
	doc, err := a.Client.Collection("users").Doc(userId).Collection("booster_data").Doc(boosterId).Get(ctx)
	if err != nil {
		if isNotFoundError(err) {
			return nil, nil // Not found - return empty map
		}
		return nil, err
	}
	return doc.Data(), nil
}

// SetBoosterData creates or updates booster-specific data
func (a *FirestoreAdapter) SetBoosterData(ctx context.Context, userId string, boosterId string, data map[string]interface{}) error {
	// Add timestamp
	data["last_updated"] = time.Now()
	_, err := a.Client.Collection("users").Doc(userId).Collection("booster_data").Doc(boosterId).Set(ctx, data, firestore.MergeAll)
	return err
}

// DeleteBoosterData removes booster-specific data by ID
func (a *FirestoreAdapter) DeleteBoosterData(ctx context.Context, userId string, boosterId string) error {
	_, err := a.Client.Collection("users").Doc(userId).Collection("booster_data").Doc(boosterId).Delete(ctx)
	return err
}
