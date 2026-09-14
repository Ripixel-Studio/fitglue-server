package activity

import (
	"google.golang.org/protobuf/proto"

	pbactivity "github.com/fitglue/server/src/go/pkg/types/pb/models/activity"
)

// Provenance field names. These identify which resolved field a FieldProvenance entry
// attributes and are stable strings consumers (and the web/editing UI) can key off.
const (
	FieldName              = "name"
	FieldType              = "type"
	FieldDescription       = "description"
	FieldTags              = "tags"
	FieldTimeMarkers       = "time_markers"
	FieldHybridRaceSummary = "hybrid_race_summary"
)

// Resolve is the resolution engine: it applies the deterministic resolution stack over a
// layered ActivityRecord and returns the resolved activity together with per-field
// provenance.
//
// The stack is layered in a fixed order and the later layer wins:
//  1. source        — the immutable parsed source activity (the base),
//  2. enricher runs — each enricher_layers entry folded on top, in execution order,
//  3. user overlay  — the user's field-level edits, which always win.
//
// The resolved activity is identical to EffectiveActivity (the derived fold with the
// overlay applied), so it carries full fidelity — sessions, records and stream data the
// enrichers produced are preserved. The provenance is the first-class addition: for each
// layerable/editable field it records which layer/run set the resolved value.
//
// Provenance is derived from the field contributions captured on each enricher run layer.
// A layer written before contributions were captured (legacy record) simply contributes
// no provenance for its fields, which then fall through to the source (or to a later
// layer that did capture a contribution). On a re-enriched activity the enricher layers
// are append-only across runs, so the provenance reflects the cumulative run history.
//
// Returns nil for a nil record.
func Resolve(rec *pbactivity.ActivityRecord) *pbactivity.ResolvedActivity {
	if rec == nil {
		return nil
	}
	return &pbactivity.ResolvedActivity{
		Activity:   EffectiveActivity(rec),
		Provenance: resolveProvenance(rec),
	}
}

// resolveProvenance walks the resolution stack and attributes each layerable field to the
// layer that set it. Entries are emitted in a stable field order (name, type, description,
// tags, time_markers, hybrid_race_summary) so the output is deterministic. Scalar fields
// yield at most one entry (the winning layer); composite fields yield one entry per
// element, and description yields one per contributing section.
func resolveProvenance(rec *pbactivity.ActivityRecord) []*pbactivity.FieldProvenance {
	src := rec.GetSourceLayer().GetParsed()
	overlay := rec.GetUserEditOverlay()
	layers := rec.GetEnricherLayers()

	var out []*pbactivity.FieldProvenance

	// --- name (scalar, last writer wins) ---
	var nameWinner *pbactivity.FieldProvenance
	if src.GetName() != "" {
		nameWinner = sourceProv(FieldName)
	}
	for i, l := range layers {
		c := l.GetContribution()
		if c == nil {
			continue
		}
		// An enricher that replaces the name or appends a suffix has set the name.
		if c.Name != nil || c.NameSuffix != nil {
			nameWinner = enricherProv(FieldName, i, l)
		}
	}
	if overlay != nil && overlay.Name != nil {
		nameWinner = overlayProv(FieldName)
	}
	if nameWinner != nil {
		out = append(out, nameWinner)
	}

	// --- type (scalar, last writer wins) ---
	var typeWinner *pbactivity.FieldProvenance
	if src.GetType() != pbactivity.ActivityType_ACTIVITY_TYPE_UNSPECIFIED {
		typeWinner = sourceProv(FieldType)
	}
	for i, l := range layers {
		if c := l.GetContribution(); c != nil && c.Type != nil {
			typeWinner = enricherProv(FieldType, i, l)
		}
	}
	if overlay != nil && overlay.Type != nil {
		typeWinner = overlayProv(FieldType)
	}
	if typeWinner != nil {
		out = append(out, typeWinner)
	}

	// --- description (composed of sections; overlay replaces the whole field) ---
	if overlay != nil && overlay.Description != nil {
		out = append(out, overlayProv(FieldDescription))
	} else {
		if src.GetDescription() != "" {
			out = append(out, sourceProv(FieldDescription))
		}
		for i, l := range layers {
			if c := l.GetContribution(); c != nil && c.Description != nil {
				out = append(out, enricherProv(FieldDescription, i, l))
			}
		}
	}

	// --- tags (composite; overlay replaces the whole set) ---
	if overlay != nil && overlay.Tags != nil {
		for i := range overlay.Tags {
			out = append(out, withElement(overlayProv(FieldTags), i))
		}
	} else {
		idx := 0
		for range src.GetTags() {
			out = append(out, withElement(sourceProv(FieldTags), idx))
			idx++
		}
		for li, l := range layers {
			c := l.GetContribution()
			if c == nil {
				continue
			}
			for range c.Tags {
				out = append(out, withElement(enricherProv(FieldTags, li, l), idx))
				idx++
			}
		}
	}

	// --- time_markers (composite; not editable via the overlay) ---
	{
		idx := 0
		for range src.GetTimeMarkers() {
			out = append(out, withElement(sourceProv(FieldTimeMarkers), idx))
			idx++
		}
		for li, l := range layers {
			c := l.GetContribution()
			if c == nil {
				continue
			}
			for range c.TimeMarkers {
				out = append(out, withElement(enricherProv(FieldTimeMarkers, li, l), idx))
				idx++
			}
		}
	}

	// --- hybrid_race_summary (scalar, last writer wins; not editable via the overlay) ---
	var hybridWinner *pbactivity.FieldProvenance
	if src.GetHybridRaceSummary() != nil {
		hybridWinner = sourceProv(FieldHybridRaceSummary)
	}
	for i, l := range layers {
		if c := l.GetContribution(); c != nil && c.HybridRaceSummary != nil {
			hybridWinner = enricherProv(FieldHybridRaceSummary, i, l)
		}
	}
	if hybridWinner != nil {
		out = append(out, hybridWinner)
	}

	return out
}

func sourceProv(field string) *pbactivity.FieldProvenance {
	return &pbactivity.FieldProvenance{
		Field:         field,
		Layer:         pbactivity.ProvenanceLayer_PROVENANCE_LAYER_SOURCE,
		EnricherIndex: -1,
	}
}

func overlayProv(field string) *pbactivity.FieldProvenance {
	return &pbactivity.FieldProvenance{
		Field:         field,
		Layer:         pbactivity.ProvenanceLayer_PROVENANCE_LAYER_USER_OVERLAY,
		EnricherIndex: -1,
	}
}

func enricherProv(field string, idx int, layer *pbactivity.EnricherRunLayer) *pbactivity.FieldProvenance {
	return &pbactivity.FieldProvenance{
		Field:         field,
		Layer:         pbactivity.ProvenanceLayer_PROVENANCE_LAYER_ENRICHER,
		ProviderName:  layer.GetProviderName(),
		ExecutionId:   layer.GetExecutionId(),
		EnricherIndex: int32(idx),
	}
}

func withElement(p *pbactivity.FieldProvenance, idx int) *pbactivity.FieldProvenance {
	p.ElementIndex = proto.Int32(int32(idx))
	return p
}
