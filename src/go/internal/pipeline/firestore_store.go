// nolint:proto-json
package pipeline

import (
	"context"
	"encoding/json"
	"time"

	"cloud.google.com/go/firestore"
	"github.com/fitglue/server/src/go/pkg/types/pb/models/pipeline"
	"google.golang.org/api/iterator"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/encoding/protojson"
	"google.golang.org/protobuf/reflect/protoreflect"
)

type FirestoreStore struct {
	client *firestore.Client
}

func NewFirestoreStore(client *firestore.Client) *FirestoreStore {
	return &FirestoreStore{client: client}
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

	if status != "" {
		if v, ok := pipeline.PipelineRunStatus_value[status]; ok {
			q = q.Where("status", "==", v)
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
		if err := decodeProtoMap(doc.Data(), &run); err != nil {
			return nil, err
		}
		runs = append(runs, &run)
	}
	return runs, nil
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
