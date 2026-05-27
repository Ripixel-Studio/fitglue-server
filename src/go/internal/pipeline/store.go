package pipeline

import (
	"context"
	"time"

	"github.com/fitglue/server/src/go/pkg/types/pb/models/pipeline"
)

// PipelineStore defines the data access contract for pipeline configurations, runs, and pending inputs.
type PipelineStore interface {
	// Pipeline Configurations
	ListPipelines(ctx context.Context, userID string) ([]*pipeline.PipelineConfig, error)
	GetPipeline(ctx context.Context, userID, pipelineID string) (*pipeline.PipelineConfig, error)
	CreatePipeline(ctx context.Context, userID string, cfg *pipeline.PipelineConfig) (*pipeline.PipelineConfig, error)
	UpdatePipeline(ctx context.Context, userID string, cfg *pipeline.PipelineConfig) (*pipeline.PipelineConfig, error)
	DeletePipeline(ctx context.Context, userID, pipelineID string) error

	// Pending Inputs
	ListPendingInputs(ctx context.Context, userID string) ([]*pipeline.PendingInput, error)
	GetPendingInput(ctx context.Context, userID, inputID string) (*pipeline.PendingInput, error)
	UpdatePendingInput(ctx context.Context, userID string, input *pipeline.PendingInput) error

	// Pipeline Runs
	GetPipelineRun(ctx context.Context, userID, runID string) (*pipeline.PipelineRun, error)
	FindPipelineRunByActivityId(ctx context.Context, userID, activityID string) (*pipeline.PipelineRun, error)
	FindPipelineRunBySourceActivityID(ctx context.Context, userID, pipelineID, sourceActivityID string) (*pipeline.PipelineRun, error)
	FindAnyPipelineRunBySourceActivityID(ctx context.Context, userID, sourceActivityID string) (*pipeline.PipelineRun, error)
	FindPipelineRunByPendingInputId(ctx context.Context, userID, pendingInputID string) (*pipeline.PipelineRun, error)
	ListPipelineRuns(ctx context.Context, userID, pipelineID string, limit int32, pageToken string, since, until *time.Time) ([]*pipeline.PipelineRun, string, error)
	UpdatePipelineRun(ctx context.Context, userID, runID string, updateData map[string]interface{}) error
}
