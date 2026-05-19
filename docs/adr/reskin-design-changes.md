# ADR: Brutal × Aurora Data Layer — Design Decisions

_Recorded during the backend-handoff implementation. See `claude-design/backend-handoff/` for the full brief._

## Decision 1 — Proposal 02 shape: sibling `ActivityEnrichments` message

**Chosen:** New sibling `ActivityEnrichments` message embedded into `ShowcasedActivity` and `EnrichedActivityEvent` (option B).

**Rejected:** Repeated optional fields directly on `StandardizedActivity` (option A).

**Rationale:** Clean separation between raw source data (what came from the wearable/app) and derived pipeline outputs. `StandardizedActivity` remains a faithful representation of the source; enrichments are an additive layer on top. This also makes the proto evolution story cleaner — enrichers can be added/removed without touching the core activity message.

## Decision 2 — Proposal 04 stage source-of-truth: server-driven

**Chosen:** `stage` field on `PluginManifest` (server registry owns it).

**Rejected:** Hard-coded client-side lookup table in the web.

**Rationale:** The registry already owns `category`, `sort_order`, `is_premium`. Adding `stage` keeps the plugin metadata contract in one place. New enrichers get the right stage without any web code changes. A client-side table would drift as enrichers are added.

## Decision 3 — Proposal 06 TTL: ship `since`/`until` filter immediately, show 7D stats

**Chosen:** Ship the `since`/`until` date filter on `ListPipelineRuns` now. Web shows "RUNS / 7D" because the pipeline_runs collection has a 7-day TTL.

**Rejected (deferred):** Extending TTL to 35 days, or building a `pipeline_stats` daily-rollup document.

**Rationale:** The filter is useful immediately for other purposes (e.g. `UnsynchronizedDetailPage`). The 30-day design stat requires either a TTL extension or a rollup job — neither is worth the complexity before we have real users generating meaningful volumes. Revisit if users request monthly pipeline stats.

## Implementation notes

- `enrichment_metadata map<string,string>` on `ShowcasedActivity` is reserved at field 12 — no new writers, legacy Firestore documents are silently skipped on read.
- `BoosterExecution.status` (freeform string, field 2) is reserved — field 6 carries the typed `ExecutionStepStatus` enum for new writes.
- Typed `ActivityEnrichments` are serialised as a `protojson` string in the `enrichments` Firestore field of `showcased_activities` documents. Old documents without this field return nil enrichments — modules hide or show "—" on the public showcase until the activity re-syncs.
- `ExecutionStep` SOURCE and PARSE steps are written with 0ms duration (not individually timed at the orchestrator level). The enricher batch is the only timed step. Destination steps are written by the destination uploader service.
