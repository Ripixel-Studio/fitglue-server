// nolint:proto-json
package pipeline

import (
	"context"
	"encoding/json"
	"strconv"
	"strings"
	"time"

	"cloud.google.com/go/firestore"
	"github.com/fitglue/server/src/go/internal/infra"
	"github.com/fitglue/server/src/go/pkg/types/pb/models/pipeline"
	"google.golang.org/api/iterator"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/encoding/protojson"
	"google.golang.org/protobuf/reflect/protoreflect"
)

type FirestoreStore struct {
	client *firestore.Client
	logger infra.Logger
}

func NewFirestoreStore(client *firestore.Client) *FirestoreStore {
	return &FirestoreStore{client: client, logger: infra.NewLoggerWithComponent("pipeline-store")}
}

func (s *FirestoreStore) ListPipelines(ctx context.Context, userID string) ([]*pipeline.PipelineConfig, error) {
	iter := s.client.Collection("users").Doc(userID).Collection("pipelines").Documents(ctx)
	defer iter.Stop()

	var pipelines []*pipeline.PipelineConfig
	for {
		doc, err := iter.Next()
		if err == iterator.Done {
			break
		}
		if err != nil {
			return nil, err
		}

		var cfg pipeline.PipelineConfig
		if err := decodeProtoMap(doc.Data(), &cfg); err != nil {
			return nil, err
		}
		pipelines = append(pipelines, &cfg)
	}
	return pipelines, nil
}

func (s *FirestoreStore) GetPipeline(ctx context.Context, userID, pipelineID string) (*pipeline.PipelineConfig, error) {
	doc, err := s.client.Collection("users").Doc(userID).Collection("pipelines").Doc(pipelineID).Get(ctx)
	if err != nil {
		if status.Code(err) == codes.NotFound {
			return nil, nil // Return nil, nil for not found (service layer handles it)
		}
		return nil, err
	}

	var cfg pipeline.PipelineConfig
	if err := decodeProtoMap(doc.Data(), &cfg); err != nil {
		return nil, err
	}
	return &cfg, nil
}

func (s *FirestoreStore) CreatePipeline(ctx context.Context, userID string, cfg *pipeline.PipelineConfig) (*pipeline.PipelineConfig, error) {
	data, err := encodeProtoMap(cfg)
	if err != nil {
		return nil, err
	}

	_, err = s.client.Collection("users").Doc(userID).Collection("pipelines").Doc(cfg.Id).Set(ctx, data)
	if err != nil {
		return nil, err
	}
	return cfg, nil
}

func (s *FirestoreStore) UpdatePipeline(ctx context.Context, userID string, cfg *pipeline.PipelineConfig) (*pipeline.PipelineConfig, error) {
	// Full copy update based on the given proto message
	data, err := encodeProtoMap(cfg)
	if err != nil {
		return nil, err
	}

	_, err = s.client.Collection("users").Doc(userID).Collection("pipelines").Doc(cfg.Id).Set(ctx, data)
	if err != nil {
		return nil, err
	}
	return cfg, nil
}

func (s *FirestoreStore) DeletePipeline(ctx context.Context, userID, pipelineID string) error {
	_, err := s.client.Collection("users").Doc(userID).Collection("pipelines").Doc(pipelineID).Delete(ctx)
	return err
}

func (s *FirestoreStore) ListPendingInputs(ctx context.Context, userID string) ([]*pipeline.PendingInput, error) {
	iter := s.client.Collection("users").Doc(userID).Collection("pending_inputs").
		Where("status", "==", int32(pipeline.PendingInput_STATUS_WAITING)).
		OrderBy("created_at", firestore.Desc).
		Documents(ctx)
	defer iter.Stop()

	var inputs []*pipeline.PendingInput
	for {
		doc, err := iter.Next()
		if err == iterator.Done {
			break
		}
		if err != nil {
			return nil, err
		}

		var input pipeline.PendingInput
		if err := decodeProtoMap(doc.Data(), &input); err != nil {
			return nil, err
		}
		inputs = append(inputs, &input)
	}
	return inputs, nil
}

func (s *FirestoreStore) GetPendingInput(ctx context.Context, userID, inputID string) (*pipeline.PendingInput, error) {
	doc, err := s.client.Collection("users").Doc(userID).Collection("pending_inputs").Doc(inputID).Get(ctx)
	if err != nil {
		if status.Code(err) == codes.NotFound {
			return nil, nil
		}
		return nil, err
	}

	var input pipeline.PendingInput
	if err := decodeProtoMap(doc.Data(), &input); err != nil {
		return nil, err
	}
	return &input, nil
}

func (s *FirestoreStore) UpdatePendingInput(ctx context.Context, userID string, input *pipeline.PendingInput) error {
	data, err := encodeProtoMap(input)
	if err != nil {
		return err
	}
	_, err = s.client.Collection("users").Doc(userID).Collection("pending_inputs").Doc(input.ActivityId).Set(ctx, data, firestore.MergeAll)
	return err
}

func (s *FirestoreStore) GetPipelineRun(ctx context.Context, userID, runID string) (*pipeline.PipelineRun, error) {
	doc, err := s.client.Collection("users").Doc(userID).Collection("pipeline_runs").Doc(runID).Get(ctx)
	if err != nil {
		if status.Code(err) == codes.NotFound {
			return nil, nil
		}
		return nil, err
	}

	var run pipeline.PipelineRun
	if err := decodeProtoMap(doc.Data(), &run); err != nil {
		return nil, err
	}
	return &run, nil
}

func (s *FirestoreStore) FindPipelineRunByActivityId(ctx context.Context, userID, activityID string) (*pipeline.PipelineRun, error) {
	iter := s.client.Collection("users").Doc(userID).Collection("pipeline_runs").
		Where("activity_id", "==", activityID).
		OrderBy("created_at", firestore.Desc).
		Limit(1).
		Documents(ctx)
	defer iter.Stop()

	doc, err := iter.Next()
	if err == iterator.Done {
		return nil, nil // No run found for this activity
	}
	if err != nil {
		return nil, err
	}

	var run pipeline.PipelineRun
	if err := decodeProtoMap(doc.Data(), &run); err != nil {
		return nil, err
	}
	return &run, nil
}

func (s *FirestoreStore) FindPipelineRunByPendingInputId(ctx context.Context, userID, pendingInputID string) (*pipeline.PipelineRun, error) {
	iter := s.client.Collection("users").Doc(userID).Collection("pipeline_runs").
		Where("pending_input_id", "==", pendingInputID).
		Limit(1).
		Documents(ctx)
	defer iter.Stop()

	doc, err := iter.Next()
	if err == iterator.Done {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}

	var run pipeline.PipelineRun
	if err := decodeProtoMap(doc.Data(), &run); err != nil {
		return nil, err
	}
	return &run, nil
}

func (s *FirestoreStore) FindPipelineRunBySourceActivityID(ctx context.Context, userID, pipelineID, sourceActivityID string) (*pipeline.PipelineRun, error) {
	iter := s.client.Collection("users").Doc(userID).Collection("pipeline_runs").
		Where("pipeline_id", "==", pipelineID).
		Where("source_activity_id", "==", sourceActivityID).
		OrderBy("created_at", firestore.Desc).
		Limit(1).
		Documents(ctx)
	defer iter.Stop()

	doc, err := iter.Next()
	if err == iterator.Done {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}

	var run pipeline.PipelineRun
	if err := decodeProtoMap(doc.Data(), &run); err != nil {
		return nil, err
	}
	return &run, nil
}

func (s *FirestoreStore) FindAnyPipelineRunBySourceActivityID(ctx context.Context, userID, sourceActivityID string) (*pipeline.PipelineRun, error) {
	iter := s.client.Collection("users").Doc(userID).Collection("pipeline_runs").
		Where("source_activity_id", "==", sourceActivityID).
		Limit(1).
		Documents(ctx)
	defer iter.Stop()

	doc, err := iter.Next()
	if err == iterator.Done {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}

	var run pipeline.PipelineRun
	if err := decodeProtoMap(doc.Data(), &run); err != nil {
		return nil, err
	}
	return &run, nil
}

func (s *FirestoreStore) ListPipelineRuns(ctx context.Context, userID, pipelineID string, limit int32, pageToken string, since, until *time.Time) ([]*pipeline.PipelineRun, string, error) {
	if limit <= 0 {
		limit = 50
	}

	query := s.client.Collection("users").Doc(userID).Collection("pipeline_runs").
		OrderBy("created_at", firestore.Desc)

	if pipelineID != "" {
		query = query.Where("pipeline_id", "==", pipelineID)
	}
	if since != nil {
		query = query.Where("created_at", ">=", *since)
	}
	if until != nil {
		query = query.Where("created_at", "<=", *until)
	}

	iter := query.Limit(int(limit)).Documents(ctx)
	defer iter.Stop()

	var runs []*pipeline.PipelineRun
	for {
		doc, err := iter.Next()
		if err == iterator.Done {
			break
		}
		if err != nil {
			return nil, "", err
		}

		var run pipeline.PipelineRun
		if err := decodeProtoMap(doc.Data(), &run); err != nil {
			return nil, "", err
		}
		runs = append(runs, &run)
	}
	// Note: Pagination not completely implemented
	return runs, "", nil
}

func (s *FirestoreStore) AdminListPipelineRuns(ctx context.Context, status, source, userID string, limit int32) ([]*pipeline.PipelineRun, error) {
	if limit <= 0 {
		limit = 50
	}

	var q firestore.Query
	if userID != "" {
		q = s.client.Collection("users").Doc(userID).Collection("pipeline_runs").
			OrderBy("created_at", firestore.Desc)
	} else {
		q = s.client.CollectionGroup("pipeline_runs").
			OrderBy("created_at", firestore.Desc)
	}

	// status is stored as the proto enum *name* string (protojson), not its
	// numeric value, so filter by the resolved name. Accept enum names, short
	// names ("SYNCED"), or numeric values from older clients.
	if status != "" {
		if name, ok := resolvePipelineRunStatusName(status); ok {
			q = q.Where("status", "==", name)
		}
	}
	if source != "" {
		q = q.Where("source", "==", source)
	}

	iter := q.Limit(int(limit)).Documents(ctx)
	defer iter.Stop()

	var runs []*pipeline.PipelineRun
	for {
		doc, err := iter.Next()
		if err == iterator.Done {
			break
		}
		if err != nil {
			return nil, err
		}
		var run pipeline.PipelineRun
		// A single legacy/dirty document must not fail the whole admin listing.
		// Normalise common legacy shapes, then skip (with a warning) anything
		// that still won't decode rather than returning an error to the caller.
		if err := decodeProtoMap(normalizeRunData(doc.Data()), &run); err != nil {
			s.logger.Warn(ctx, "skipping pipeline run that failed to decode",
				"doc", doc.Ref.Path, "error", err)
			continue
		}
		// user_id is not persisted on the per-user run document; derive it so the
		// admin console can act on the run. For the collection-group query the
		// owning user is the grandparent of the run document.
		run.UserId = userID
		if run.UserId == "" && doc.Ref.Parent != nil && doc.Ref.Parent.Parent != nil {
			run.UserId = doc.Ref.Parent.Parent.ID
		}
		runs = append(runs, &run)
	}
	return runs, nil
}

// resolvePipelineRunStatusName converts a status filter supplied as an enum name
// ("PIPELINE_RUN_STATUS_SYNCED"), a short name ("SYNCED"), or a numeric enum
// value ("2") into the canonical proto enum name used in Firestore.
func resolvePipelineRunStatusName(in string) (string, bool) {
	in = strings.TrimSpace(in)
	if in == "" {
		return "", false
	}
	if n, err := strconv.Atoi(in); err == nil {
		if name, ok := pipeline.PipelineRunStatus_name[int32(n)]; ok {
			return name, true
		}
		return "", false
	}
	up := strings.ToUpper(in)
	if !strings.HasPrefix(up, "PIPELINE_RUN_STATUS_") {
		up = "PIPELINE_RUN_STATUS_" + up
	}
	if _, ok := pipeline.PipelineRunStatus_value[up]; ok {
		return up, true
	}
	return "", false
}

// normalizeRunData converts legacy pipeline_run documents into a shape protojson
// can decode. Old TypeScript-written docs used camelCase keys; later Go merges
// added snake_case keys, leaving documents with both forms which protojson
// rejects as duplicate fields. We drop the camelCase form where a snake_case
// equivalent exists. We also coerce a numeric status into its enum name.
func normalizeRunData(data map[string]interface{}) map[string]interface{} {
	camelToSnake := map[string]string{
		"pipelineId":         "pipeline_id",
		"activityId":         "activity_id",
		"sourceActivityId":   "source_activity_id",
		"startTime":          "start_time",
		"createdAt":          "created_at",
		"updatedAt":          "updated_at",
		"statusMessage":      "status_message",
		"pendingInputId":     "pending_input_id",
		"originalPayloadUri": "original_payload_uri",
		"enrichedEventUri":   "enriched_event_uri",
	}
	for camel, snake := range camelToSnake {
		if _, hasCamel := data[camel]; hasCamel {
			if _, hasSnake := data[snake]; hasSnake {
				delete(data, camel)
			}
		}
	}

	if statusVal, ok := data["status"]; ok {
		switch n := statusVal.(type) {
		case int64:
			if name, ok := pipeline.PipelineRunStatus_name[int32(n)]; ok {
				data["status"] = name
			}
		case float64:
			if name, ok := pipeline.PipelineRunStatus_name[int32(n)]; ok {
				data["status"] = name
			}
		}
	}

	return data
}

func (s *FirestoreStore) UpdatePipelineRun(ctx context.Context, userID, runID string, updateData map[string]interface{}) error {
	_, err := s.client.Collection("users").Doc(userID).Collection("pipeline_runs").Doc(runID).Set(ctx, updateData, firestore.MergeAll)
	return err
}

// Helpers
func encodeProtoMap(msg protoreflect.ProtoMessage) (map[string]interface{}, error) {
	b, err := protojson.MarshalOptions{EmitUnpopulated: false, UseProtoNames: true}.Marshal(msg)
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
