// nolint:proto-json
package enricher

import (
	"context"
	"log/slog"
	"strings"
	"time"

	"github.com/fitglue/server/src/go/internal/pipeline/enricher/providers"
	activityPkg "github.com/fitglue/server/src/go/pkg/domain/activity"
	pbactivity "github.com/fitglue/server/src/go/pkg/types/pb/models/activity"
	pbevents "github.com/fitglue/server/src/go/pkg/types/pb/models/events"
	"google.golang.org/protobuf/encoding/protojson"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/timestamppb"
)

// writeActivityRecord persists the durable, layered ActivityRecord on pipeline
// completion. It replaces the pass-through model where only the enriched-event blob
// survived: the parsed source layer and per-enricher run layers are now kept
// alongside the derived activity, indefinitely (spec DECISION 1a).
//
// The record lives at a stable per-activity GCS path, so a resume or re-enrichment of
// the same activity loads the existing record and APPENDS its enricher run layers
// while leaving the immutable source layer and the mutable user-edit overlay intact.
// A pointer (activity_record_uri) and the raw-payload prune boundary
// (raw_payload_expires_at) are written back onto the pipeline run.
//
// Best-effort: any failure is logged and does not fail the pipeline — destinations
// have already been finalised by this point.
func (o *Orchestrator) writeActivityRecord(
	ctx context.Context,
	logger *slog.Logger,
	userID string,
	activityID string,
	pipelineID string,
	pipelineRunID string,
	sourceSnapshot *pbactivity.StandardizedActivity,
	finalEvent *pbevents.EnrichedActivityEvent,
	providerExecs []ProviderExecution,
	results []*providers.EnrichmentResult,
	configs []configuredEnricher,
	rawPayloadURI string,
) {
	if o.storage == nil || o.bucketName == "" {
		return
	}

	now := time.Now()

	// Correlate each enricher's output (indexed by config) to its provider name, so run
	// layers can carry both the typed enrichments that provider produced and the
	// layerable field contribution the resolution engine attributes provenance from.
	enrichByProvider := make(map[string]*pbactivity.ActivityEnrichments)
	contribByProvider := make(map[string]*pbactivity.EnricherContribution)
	typeByProvider := make(map[string]string)
	for i, res := range results {
		if res == nil || i >= len(configs) {
			continue
		}
		p, ok := o.providersByType[configs[i].ProviderType]
		if !ok {
			continue
		}
		typeByProvider[p.Name()] = configs[i].ProviderType.String()
		if res.Enrichments != nil {
			enrichByProvider[p.Name()] = res.Enrichments
		}
		if c := enricherContribution(res); c != nil {
			contribByProvider[p.Name()] = c
		}
	}

	// Load an existing record (resume / re-enrichment) so we append rather than orphan.
	recordURI := activityPkg.ActivityRecordURI(o.bucketName, userID, activityID)
	existing, err := activityPkg.LoadActivityRecord(ctx, recordURI, o.storage)
	if err != nil {
		// A missing blob surfaces as an error from GCS Get; treat as "no prior record".
		logger.Debug("No existing activity record to append to (treating as first write)", "activity_id", activityID, "error", err)
		existing = nil
	}

	record := buildActivityRecord(
		existing,
		userID, activityID, pipelineID, pipelineRunID,
		finalEvent.Source, sourceSnapshot.GetExternalId(),
		sourceSnapshot, finalEvent.ActivityData, finalEvent.Enrichments,
		rawPayloadURI, now,
		providerExecs, enrichByProvider, contribByProvider, typeByProvider,
	)

	data, err := protojson.Marshal(record)
	if err != nil {
		logger.Warn("Failed to marshal activity record", "error", err, "activity_id", activityID)
		return
	}
	object := activityPkg.ActivityRecordObjectPath(userID, activityID)
	if err := o.storage.Write(ctx, o.bucketName, object, data); err != nil {
		logger.Warn("Failed to write activity record to GCS", "error", err, "activity_id", activityID)
		return
	}
	logger.Info("Persisted layered activity record", "activity_id", activityID, "uri", recordURI, "enricher_layers", len(record.EnricherLayers))

	// Point the run at the record and record the raw-payload prune boundary.
	update := map[string]interface{}{
		"activity_record_uri": recordURI,
	}
	if sl := record.GetSourceLayer(); sl != nil && sl.RawPayloadExpiresAt != nil {
		update["raw_payload_expires_at"] = sl.RawPayloadExpiresAt.AsTime()
	}
	if err := o.database.UpdatePipelineRun(ctx, userID, pipelineRunID, update); err != nil {
		logger.Warn("Failed to link activity record to pipeline run", "error", err, "pipeline_run_id", pipelineRunID)
	}
}

// buildActivityRecord composes the layered record from the source snapshot, the derived
// (fully enriched) activity, and the per-run enricher executions. When `existing` is
// non-nil (resume / re-enrichment) the immutable source layer, the user-edit overlay and
// the created_at timestamp are preserved, and the new run's enricher layers are appended.
// Pure and deterministic (given `now`) so it can be unit-tested without GCS.
func buildActivityRecord(
	existing *pbactivity.ActivityRecord,
	userID, activityID, pipelineID, pipelineRunID string,
	source pbactivity.ActivitySource,
	externalID string,
	sourceParsed *pbactivity.StandardizedActivity,
	derived *pbactivity.StandardizedActivity,
	mergedEnrichments *pbactivity.ActivityEnrichments,
	rawPayloadURI string,
	now time.Time,
	execs []ProviderExecution,
	enrichByProvider map[string]*pbactivity.ActivityEnrichments,
	contribByProvider map[string]*pbactivity.EnricherContribution,
	typeByProvider map[string]string,
) *pbactivity.ActivityRecord {
	nowPb := timestamppb.New(now)

	var record *pbactivity.ActivityRecord
	if existing != nil {
		record = existing
	} else {
		record = &pbactivity.ActivityRecord{
			ActivityId:      activityID,
			UserId:          userID,
			Source:          source,
			ExternalId:      externalID,
			PipelineId:      pipelineID,
			PipelineRunId:   pipelineRunID,
			CreatedAt:       nowPb,
			UserEditOverlay: &pbactivity.ActivityUserEditOverlay{},
			SourceLayer: &pbactivity.ActivitySourceLayer{
				// Immutable snapshot of the parsed source, before any enricher ran.
				Parsed:              cloneActivity(sourceParsed),
				RawPayloadUri:       rawPayloadURI,
				IngestedAt:          nowPb,
				RawPayloadExpiresAt: timestamppb.New(now.Add(activityPkg.RawPayloadRetention)),
			},
		}
	}

	// Append this run's enricher layers (append-only across resumes).
	record.EnricherLayers = append(record.EnricherLayers, buildEnricherLayers(execs, enrichByProvider, contribByProvider, typeByProvider, nowPb)...)

	// Refresh the composed views and bookkeeping.
	record.DerivedActivity = cloneActivity(derived)
	record.Enrichments = mergedEnrichments
	record.PipelineRunId = pipelineRunID
	record.UpdatedAt = nowPb
	record.SchemaVersion = activityPkg.ActivityRecordSchemaVersion
	if record.UserEditOverlay == nil {
		record.UserEditOverlay = &pbactivity.ActivityUserEditOverlay{}
	}
	return record
}

// buildEnricherLayers converts this run's provider executions into append-only run
// layers, attaching the typed enrichments and the layerable field contribution each
// provider produced where available. The contribution is what the resolution engine
// walks to attribute per-field provenance.
func buildEnricherLayers(execs []ProviderExecution, enrichByProvider map[string]*pbactivity.ActivityEnrichments, contribByProvider map[string]*pbactivity.EnricherContribution, typeByProvider map[string]string, runAt *timestamppb.Timestamp) []*pbactivity.EnricherRunLayer {
	layers := make([]*pbactivity.EnricherRunLayer, 0, len(execs))
	for _, pe := range execs {
		layer := &pbactivity.EnricherRunLayer{
			ProviderName: pe.ProviderName,
			ProviderType: typeByProvider[pe.ProviderName],
			ExecutionId:  pe.ExecutionID,
			Status:       pe.Status,
			DurationMs:   pe.DurationMs,
			Metadata:     pe.Metadata,
			RunAt:        runAt,
		}
		if enr, ok := enrichByProvider[pe.ProviderName]; ok {
			layer.Enrichments = enr
		}
		if c, ok := contribByProvider[pe.ProviderName]; ok {
			layer.Contribution = c
		}
		layers = append(layers, layer)
	}
	return layers
}

// enricherContribution extracts the layerable field contribution from an enrichment
// result — the subset of fields the orchestrator folds onto the running activity (see
// the apply block in orchestrator.Process). Returns nil when the enricher touched none
// of these fields, so layers stay contribution-less when there is nothing to attribute.
func enricherContribution(res *providers.EnrichmentResult) *pbactivity.EnricherContribution {
	if res == nil {
		return nil
	}
	c := &pbactivity.EnricherContribution{}
	touched := false
	if res.Name != "" {
		c.Name = proto.String(res.Name)
		touched = true
	}
	if res.NameSuffix != "" {
		c.NameSuffix = proto.String(res.NameSuffix)
		touched = true
	}
	if res.ActivityType != pbactivity.ActivityType_ACTIVITY_TYPE_UNSPECIFIED {
		t := res.ActivityType
		c.Type = &t
		touched = true
	}
	if len(res.Tags) > 0 {
		c.Tags = append([]string(nil), res.Tags...)
		touched = true
	}
	if len(res.TimeMarkers) > 0 {
		c.TimeMarkers = res.TimeMarkers
		touched = true
	}
	if trimmed := strings.TrimSpace(res.Description); trimmed != "" {
		c.Description = proto.String(trimmed)
		touched = true
	}
	if res.HybridRaceSummary != nil {
		c.HybridRaceSummary = res.HybridRaceSummary
		touched = true
	}
	if !touched {
		return nil
	}
	return c
}

// cloneActivity deep-copies a StandardizedActivity so the stored layer is independent of
// the live pipeline value. Returns nil for a nil input.
func cloneActivity(a *pbactivity.StandardizedActivity) *pbactivity.StandardizedActivity {
	if a == nil {
		return nil
	}
	return proto.Clone(a).(*pbactivity.StandardizedActivity)
}
