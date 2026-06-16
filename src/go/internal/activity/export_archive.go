// nolint:proto-json
package activity

import (
	"archive/zip"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	pbpipeline "github.com/fitglue/server/src/go/pkg/types/pb/models/pipeline"
	"google.golang.org/protobuf/encoding/protojson"
	"google.golang.org/protobuf/proto"
)

var exportMarshaler = protojson.MarshalOptions{EmitUnpopulated: true, Indent: "  "}

const exportRetentionNote = "FitGlue retains processed activity artifacts (FIT files, payloads) for 7 days. " +
	"Items older than that are no longer held and are therefore omitted from this export."

// sensitiveKeySubstrings are matched case-insensitively against JSON object keys;
// matching values are replaced with "[REDACTED]" before being written to an export.
var sensitiveKeySubstrings = []string{
	"token", "secret", "password", "passwd", "apikey", "api_key",
	"authorization", "credential", "private",
}

func isSensitiveKey(key string) bool {
	k := strings.ToLower(key)
	for _, s := range sensitiveKeySubstrings {
		if strings.Contains(k, s) {
			return true
		}
	}
	return false
}

// redactJSON walks a decoded JSON value and masks any sensitive fields in place.
func redactJSON(v interface{}) interface{} {
	switch val := v.(type) {
	case map[string]interface{}:
		for k, child := range val {
			if isSensitiveKey(k) {
				val[k] = "[REDACTED]"
				continue
			}
			val[k] = redactJSON(child)
		}
		return val
	case []interface{}:
		for i, child := range val {
			val[i] = redactJSON(child)
		}
		return val
	default:
		return v
	}
}

// redactRawJSON re-encodes a JSON document with sensitive fields masked. If the
// input is not a JSON object/array it is returned unchanged.
func redactRawJSON(raw []byte) []byte {
	var v interface{}
	if err := json.Unmarshal(raw, &v); err != nil {
		return raw
	}
	out, err := json.MarshalIndent(redactJSON(v), "", "  ")
	if err != nil {
		return raw
	}
	return out
}

// marshalRedacted renders a proto message to indented JSON with sensitive fields masked.
func marshalRedacted(m proto.Message) ([]byte, error) {
	raw, err := exportMarshaler.Marshal(m)
	if err != nil {
		return nil, err
	}
	return redactRawJSON(raw), nil
}

// extractFitURI returns the top-level fit_file_uri from an enriched/payload JSON blob.
func extractFitURI(raw []byte) string {
	var m map[string]interface{}
	if err := json.Unmarshal(raw, &m); err != nil {
		return ""
	}
	for _, k := range []string{"fit_file_uri", "fitFileUri"} {
		if v, ok := m[k].(string); ok && v != "" {
			return v
		}
	}
	return ""
}

func writeZipFile(zw *zip.Writer, name string, data []byte) error {
	w, err := zw.Create(name)
	if err != nil {
		return err
	}
	_, err = w.Write(data)
	return err
}

// fetchBlobBytes fetches a GCS object by full gs:// URI. A missing object (the
// common case for artifacts past the 7-day lifecycle) returns found=false rather
// than an error, so an expired artifact never fails the whole export.
func (s *Service) fetchBlobBytes(ctx context.Context, gsURI string) ([]byte, bool) {
	if gsURI == "" {
		return nil, false
	}
	data, err := s.blobStore.Get(ctx, "", gsURI)
	if err != nil {
		s.logger.Debug(ctx, "export: artifact not available (likely expired)", "uri", gsURI, "error", err)
		return nil, false
	}
	return data, true
}

// addRunToZip writes a single pipeline run's files under prefix ("" for a per-run
// archive, "runs/{id}/" for the account archive). Returns whether a FIT file was
// included. Run metadata is always written; enriched/payload/FIT are best-effort
// (omitted silently once their GCS artifacts have expired).
func (s *Service) addRunToZip(ctx context.Context, zw *zip.Writer, prefix string, run *pbpipeline.PipelineRun) (bool, error) {
	if meta, err := marshalRedacted(run); err == nil {
		if err := writeZipFile(zw, prefix+"run.json", meta); err != nil {
			return false, err
		}
	}

	var fitURI string
	if data, ok := s.fetchBlobBytes(ctx, run.GetEnrichedEventUri()); ok {
		if err := writeZipFile(zw, prefix+"enriched.json", redactRawJSON(data)); err != nil {
			return false, err
		}
		fitURI = extractFitURI(data)
	}
	if data, ok := s.fetchBlobBytes(ctx, run.GetOriginalPayloadUri()); ok {
		if err := writeZipFile(zw, prefix+"payload.json", redactRawJSON(data)); err != nil {
			return false, err
		}
		if fitURI == "" {
			fitURI = extractFitURI(data)
		}
	}
	if fitURI != "" {
		if data, ok := s.fetchBlobBytes(ctx, fitURI); ok {
			if err := writeZipFile(zw, prefix+"activity.fit", data); err != nil {
				return false, err
			}
			return true, nil
		}
	}
	return false, nil
}

// buildRunArchive builds a per-run ZIP (run metadata + enriched/payload JSON +
// raw FIT, where still retained). It always contains at least run.json.
func (s *Service) buildRunArchive(ctx context.Context, run *pbpipeline.PipelineRun) ([]byte, error) {
	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)

	readme := fmt.Sprintf("FitGlue activity export\nRun: %s\nGenerated: %s\n\nContents:\n  run.json       — pipeline run metadata\n  enriched.json  — processed activity data (if still retained)\n  payload.json   — original source payload (if still retained)\n  activity.fit   — raw FIT file (if still retained)\n\n%s\n",
		run.GetId(), time.Now().UTC().Format(time.RFC3339), exportRetentionNote)
	if err := writeZipFile(zw, "README.txt", []byte(readme)); err != nil {
		return nil, err
	}
	if _, err := s.addRunToZip(ctx, zw, "", run); err != nil {
		return nil, err
	}
	if err := zw.Close(); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

// buildAccountArchive builds the whole-account GDPR export ZIP: showcase profile,
// all pipeline runs (with per-run enriched/payload/FIT where retained), and all
// showcased activities, plus a manifest and README.
func (s *Service) buildAccountArchive(ctx context.Context, userID string) ([]byte, error) {
	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)

	manifest := map[string]interface{}{
		"userId":     userID,
		"exportedAt": time.Now().UTC().Format(time.RFC3339),
		"note":       exportRetentionNote,
	}

	if profile, err := s.store.GetShowcasePreferences(ctx, userID); err == nil && profile != nil {
		if b, err := marshalRedacted(profile); err == nil {
			_ = writeZipFile(zw, "profile.json", b)
		}
	}

	// Pipeline runs — paginate the full history, and add a per-run folder.
	runCount, fitCount := 0, 0
	runsArr := []json.RawMessage{}
	pageToken := ""
	for {
		runs, next, err := s.store.ListPipelineRuns(ctx, userID, 100, pageToken)
		if err != nil {
			return nil, fmt.Errorf("list pipeline runs: %w", err)
		}
		for _, run := range runs {
			runCount++
			if b, err := marshalRedacted(run); err == nil {
				runsArr = append(runsArr, json.RawMessage(b))
			}
			included, err := s.addRunToZip(ctx, zw, "runs/"+sanitizeDir(run.GetId())+"/", run)
			if err != nil {
				return nil, err
			}
			if included {
				fitCount++
			}
		}
		if next == "" {
			break
		}
		pageToken = next
	}
	if b, err := json.MarshalIndent(runsArr, "", "  "); err == nil {
		_ = writeZipFile(zw, "pipeline_runs.json", b)
	}

	// Showcased activities — request a high limit to capture the full set.
	showcaseCount := 0
	showcases, _, err := s.store.ListShowcasedActivitiesByUser(ctx, userID, 100000, 0)
	if err != nil {
		return nil, fmt.Errorf("list showcased activities: %w", err)
	}
	showcasesArr := []json.RawMessage{}
	for _, sc := range showcases {
		showcaseCount++
		if b, err := marshalRedacted(sc); err == nil {
			showcasesArr = append(showcasesArr, json.RawMessage(b))
		}
	}
	if b, err := json.MarshalIndent(showcasesArr, "", "  "); err == nil {
		_ = writeZipFile(zw, "showcased_activities.json", b)
	}

	manifest["pipelineRunCount"] = runCount
	manifest["fitFilesIncluded"] = fitCount
	manifest["showcasedActivityCount"] = showcaseCount
	if b, err := json.MarshalIndent(manifest, "", "  "); err == nil {
		_ = writeZipFile(zw, "manifest.json", b)
	}

	readme := fmt.Sprintf("FitGlue data export\nUser: %s\nGenerated: %s\n\nContents:\n  manifest.json             — summary of this export\n  profile.json              — your showcase profile\n  pipeline_runs.json        — all pipeline runs (metadata)\n  showcased_activities.json — all your showcased activities\n  runs/{id}/                — per-run enriched/payload JSON and raw FIT file (where retained)\n\n%s\nSensitive fields (tokens, secrets) are redacted.\n",
		userID, time.Now().UTC().Format(time.RFC3339), exportRetentionNote)
	if err := writeZipFile(zw, "README.txt", []byte(readme)); err != nil {
		return nil, err
	}

	if err := zw.Close(); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

// sanitizeDir keeps ZIP entry paths safe and flat for a run identifier.
func sanitizeDir(id string) string {
	if id == "" {
		return "unknown"
	}
	return strings.NewReplacer("/", "_", "..", "_", "\\", "_").Replace(id)
}
