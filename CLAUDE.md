# FitGlue Server

Go backend — 11 Cloud Run microservices communicating via gRPC, fronted by 4 HTTP API gateways.

## Stack

| Concern | Technology |
|---------|-----------|
| Language | Go 1.25.5 |
| HTTP | go-chi/v5 |
| RPC | gRPC + Protocol Buffers 3 |
| Database | Cloud Firestore (no ORM) |
| Messaging | Cloud Pub/Sub |
| File storage | Google Cloud Storage |
| Auth | Firebase Authentication (JWT) |
| Billing | Stripe |
| AI | Gemini (AI enrichers) |
| FIT files | muktihari/fit |
| Error tracking | Sentry |
| Container | Docker (distroless, multi-stage) |
| Infra | Terraform (Cloud Run, Firestore, Pub/Sub, GCS) |
| Node (tooling) | 22+ (proto generation, integration tests) |

## Key Commands

```bash
make setup              # Go mod download + npm install (run once)
make generate           # Regenerate protos (Go stubs + TS types + OpenAPI specs)
make build              # Compile all 10 services + CLI tools
make test               # Unit tests (go test -short ./...)
make lint               # gofmt check + go vet (excludes generated integrations/)
make preflight          # Full CI mirror: proto lint → generate → build → lint → test → coverage
make local              # docker-compose up --build (all 10 services)
make local-down         # docker-compose down
make integration        # Jest integration tests against local stack
make test-coverage      # Enforce per-package coverage thresholds

# Plugin scaffolding
make plugin-source name=<name>        # Scaffold new webhook source provider
make plugin-enricher name=<name>      # Scaffold new enricher step
make plugin-destination name=<name>   # Scaffold new destination uploader

# Integration tests against deployed envs
npm run test:dev        # Against dev environment
npm run test:test       # Against test environment
```

## Service Architecture

11 independent Cloud Run services. **4 HTTP gateways** (public-facing) + **7 domain services** (private).

```
Internet
    │
    ├── api-client   (HTTP, Firebase JWT)    → user, billing, pipeline, activity, registry
    ├── api-admin    (HTTP, admin auth)       → user, billing, pipeline, activity
    ├── api-public   (HTTP, no auth)          → registry, activity (showcases)
    └── api-webhook  (HTTP, HMAC/OAuth)       → pipeline (via Pub/Sub)
                                                user (resolve integration)
Pub/Sub
    ├── topic-raw-activity       → pipeline service (splitter + enricher)
    ├── topic-pipeline-activity  → pipeline service (per-pipeline routing)
    ├── topic-enriched-activity  → destination service
    ├── topic-destination-upload → destination service (upload jobs)
    └── topic-notifications      → notification service (FCM push dispatch)

Domain Services (private):
    ├── user         (gRPC, port 50051) — profiles, integrations, OAuth tokens
    ├── billing      (gRPC, port 50052) — Stripe subscriptions, tier enforcement
    ├── pipeline     (gRPC, port 50053) — pipeline CRUD + enrichment orchestration
    ├── activity     (gRPC, port 50054) — activity CRUD, showcases, exports
    ├── registry     (gRPC, port 50055) — plugin manifests (sources, enrichers, destinations)
    ├── destination  (Pub/Sub)          — uploads to Strava, TrainingPeaks, Intervals, Hevy, Fitbit, etc.
    └── notification (Pub/Sub, HTTP)    — FCM push dispatch, per-user preference routing
```

## Directory Structure

```
server/
├── src/
│   ├── go/
│   │   ├── go.mod, go.sum          # Single root module (all services share it)
│   │   ├── services/               # One directory per Cloud Run service
│   │   │   ├── api-client/         # HTTP gateway (Firebase JWT)
│   │   │   ├── api-admin/          # HTTP gateway (admin auth)
│   │   │   ├── api-public/         # HTTP gateway (no auth)
│   │   │   ├── api-webhook/        # HTTP gateway (HMAC/OAuth)
│   │   │   │   └── internal/webhook/sources/  # strava/, fitbit/, hevy/, polar/, oura/, wahoo/, mobile/, parkrun/
│   │   │   ├── user/               # gRPC domain service
│   │   │   ├── billing/            # gRPC domain service
│   │   │   ├── pipeline/           # gRPC domain service + enrichment orchestration
│   │   │   ├── activity/           # gRPC domain service
│   │   │   ├── registry/           # gRPC domain service
│   │   │   ├── destination/        # Pub/Sub domain service
│   │   │   │   └── internal/destination/uploaders/  # strava/, trainingpeaks/, intervals/, hevy/, fitbit/, googlesheets/, github/, showcase/
│   │   │   └── notification/       # Pub/Sub service — FCM push dispatch, per-user preference routing
│   │   ├── internal/               # Shared domain logic (used by multiple services)
│   │   │   ├── user/               # User domain (profiles, integrations, Firestore store)
│   │   │   ├── billing/            # Billing domain
│   │   │   ├── pipeline/           # Pipeline orchestration + enrichers (45+ providers)
│   │   │   │   └── enricher/providers/   # One directory per enricher
│   │   │   ├── activity/           # Activity CRUD + showcases
│   │   │   ├── registry/           # Plugin registry
│   │   │   └── infra/              # Logger, Firestore client, gRPC helpers
│   │   ├── pkg/                    # Shared packages (exported)
│   │   │   ├── types/pb/           # Generated gRPC stubs — DO NOT EDIT manually
│   │   │   ├── bootstrap/          # Service init helpers (Firebase, Firestore, Pub/Sub)
│   │   │   ├── domain/             # Domain models (activity, user, tier, FIT parser/generator)
│   │   │   ├── integrations/       # Generated OpenAPI clients — DO NOT EDIT manually
│   │   │   ├── infrastructure/     # Email, Pub/Sub adapter, storage, secrets, FCM
│   │   │   ├── sourceplugins/      # Historical activity sync providers (Fitbit, Hevy, Intervals, Strava)
│   │   │   ├── storage/            # Firestore client utilities
│   │   │   ├── errors/             # Error types + retryability
│   │   │   ├── loopprevention/     # Duplicate detection
│   │   │   ├── parkrun/            # Parkrun results parser
│   │   │   ├── pending_input/      # Pending user input models
│   │   │   └── testing/            # Test mocks
│   │   ├── cmd/                    # CLI tools (fit-gen, fit-inspect, fit-combine, etc.)
│   │   └── tests/e2e/              # Godog/Cucumber E2E tests
│   ├── proto/                      # Protocol Buffer definitions (source of truth)
│   │   ├── services/               # Service RPC definitions
│   │   ├── models/                 # Shared message types (activity, user, pipeline, plugin)
│   │   └── gateway/                # HTTP gateway API definitions
│   └── openapi/                    # Third-party OpenAPI specs (input for generating clients)
├── terraform/                      # GCP infrastructure as code
├── scripts/                        # Code gen, migrations, secret config, scaffolding
├── docs/                           # Architecture docs, guides, ADRs, API specs
│   └── api/gateway/                # Generated OpenAPI specs (output) — commit these
├── docker-compose.yaml             # Local dev stack (all 10 services)
├── Dockerfile                      # Multi-stage build (ARG SERVICE_NAME)
├── Makefile                        # All build/test/deploy automation
└── jest.config.js                  # Integration test config
```

## Code Generation Workflow

All generated files are committed. Run `make generate` after any proto or OpenAPI spec change.

```
src/proto/**/*.proto
    │
    ├── protoc → src/go/pkg/types/pb/**          (Go gRPC stubs)
    ├── ts-proto → ../web/src/types/pb/**        (TypeScript types for web)
    ├── buf → docs/api/gateway/*.openapi.yaml    (OpenAPI specs — consumed by web)
    └── oapi-codegen → src/go/pkg/integrations/ (Go clients for third-party APIs)

src/openapi/{provider}/swagger.json
    └── oapi-codegen → src/go/pkg/integrations/{provider}/client.gen.go
```

After `make generate`, if `../web` exists, web's protobuf types are also updated. Run `npm run gen-api` in `web/` to update the REST schema types.

## Plugin System

Three categories of plugins, each following the same provider interface pattern:

### Sources (webhook ingestion)
- Location: `services/api-webhook/internal/webhook/sources/{name}/`
- Interface: `SourceProvider`
- Registered in `services/api-webhook/main.go`
- Scaffold: `make plugin-source name=<name>`
- Current: strava, fitbit, hevy, polar, oura, wahoo, mobile, parkrun

### Enrichers (pipeline steps)
- Location: `internal/pipeline/enricher/providers/{name}/`
- Interface: `EnricherProvider` (runs in sequence, can modify `StandardizedActivity`)
- Registered via `init()` in each provider package
- Scaffold: `make plugin-enricher name=<name>`
- 45+ enrichers: Fitbit HR, Spotify, Weather, Parkrun Detector, Personal Records, AI Companion, etc.

### Destinations (uploaders)
- Location: `services/destination/internal/destination/uploaders/{name}/`
- Interface: uploader interface
- Registered in `services/destination/internal/destination/registry.go`
- Scaffold: `make plugin-destination name=<name>`
- Current: Strava, TrainingPeaks, Intervals.icu, Hevy, Fitbit, Google Sheets, GitHub, Showcase

## Data Flow

```
Webhook → api-webhook
    → verify signature (HMAC/OAuth per provider)
    → resolve user via UserService.GetIntegration()
    → fetch activity from source API
    → normalize to StandardizedActivity proto
    → publish to topic-raw-activity

topic-raw-activity → pipeline service (splitter)
    → look up user's pipelines from Firestore
    → publish one message per matching pipeline to topic-pipeline-activity

topic-pipeline-activity → pipeline service (enricher)
    → run enricher chain (sequential, each can modify StandardizedActivity)
    → publish to topic-enriched-activity

topic-enriched-activity → destination service
    → store activity in Firestore
    → upload to each configured destination
    → retry with exponential backoff on failure
```

## Firestore Schema

Collections follow a user-subcollection pattern:
```
users/{userId}/
    integrations/{provider}   — OAuth tokens, scopes, config
    pipelines/{pipelineId}    — Pipeline configs
    pipeline_runs/{runId}     — Execution history (7-day TTL)
    pending_inputs/{inputId}  — Paused pipeline prompts
    activities/{activityId}   — Synchronized activities
    api_keys/{keyId}          — API keys
    billing/subscription      — Stripe subscription data

showcased_activities/{id}     — Public showcase records
```

No ORM. All Firestore access uses the Go Firestore SDK directly via service-specific store files (e.g., `internal/user/firestore_store.go`).

## Authentication

- **Web/mobile clients**: Firebase JWT in `Authorization: Bearer <token>` header
- **Webhooks**: Per-provider signature verification (HMAC, OAuth token, API key)
- **Service-to-service**: gRPC (private networking on Cloud Run — no auth between services)
- **OAuth flows**: Handled by `api-client` at `GET /oauth/{provider}/callback`; tokens stored in Firestore under user integrations

## Testing

```bash
make test               # Unit tests (fast, no external deps)
make test-coverage      # Unit tests with coverage enforcement
make test-integration   # Go integration tests (needs running services)
make test-e2e           # Godog BDD tests
npm run test:local      # Jest integration tests vs local docker-compose stack
npm run test:dev        # Jest integration tests vs dev deployment
```

- Unit tests: `*_test.go` throughout `internal/` and `pkg/`
- Integration tests: `TEST_ENVIRONMENT=local jest --testPathPatterns=local.test.ts`
- E2E: Godog/Cucumber at `src/go/tests/e2e/`

## Important Conventions

1. **Single root go.mod** — All services use `src/go/go.mod`. No per-service go.mod. Enforced by `make preflight`.
2. **IoC pattern** — Dependencies injected via constructor, no package-level globals.
3. **Each service owns its Firestore data** — No cross-service Firestore writes.
4. **Provider interfaces** — Sources, enrichers, destinations are pluggable via interfaces. Register via `init()`.
5. **Structured logging** — Use `infra.NewLoggerWithComponent(name)` (slog + Sentry integration).
6. **Error types** — Use `pkg/errors` for retryability classification.
7. **gofmt required** — `make lint` fails if formatting is off. Run `gofmt -w pkg services cmd internal`.
8. **Generated code** — Never manually edit `pkg/types/pb/`, `pkg/integrations/`, or `docs/api/`. Regenerate instead.

## Adding a New Enricher

```bash
make plugin-enricher name=my-enricher
# Creates scaffold at internal/pipeline/enricher/providers/my-enricher/
# (Sources: services/api-webhook/internal/webhook/sources/{name}/)
# (Destinations: services/destination/internal/destination/uploaders/{name}/)
```

Then:
1. Implement the `EnricherProvider` interface
2. Add manifest to `internal/registry/`
3. Register via `init()` in the provider package (auto-imported by the enricher runner)
4. Write tests in `*_test.go`

## Dockerfile

Single multi-stage Dockerfile for all services. The `SERVICE_NAME` build arg selects which service to compile:

```bash
docker build --build-arg SERVICE_NAME=api-client -t api-client .
```

## CI/CD

- Push to main → build + test → auto-deploy to `dev`
- Manual approval → deploy to `prod`
- Docker images tagged with 12-char commit hash
- Content-hash caching: unchanged services are not rebuilt
- Terraform manages all Cloud Run + Firestore + Pub/Sub infrastructure
