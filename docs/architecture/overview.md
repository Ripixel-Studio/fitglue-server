# Architecture Overview

FitGlue is a fitness data aggregation and routing platform built on Google Cloud Platform. It ingests workout data from multiple sources, enriches it through configurable pipelines, and routes it to connected services.

## System Topology

```
┌─────────────────────────────────────────────────────────────────────┐
│                       Google Cloud Platform                         │
│                                                                     │
│  ┌─────────────────────────────────────────────────────────────┐    │
│  │              API GATEWAY LAYER (HTTP entry points)          │    │
│  │                                                             │    │
│  │   service.api.client   ←── Web App / Mobile (Firebase JWT) │    │
│  │   service.api.admin    ←── Admin tooling                    │    │
│  │   service.api.public   ←── Marketing site (no auth)         │    │
│  │   service.api.webhook  ←── Third-party webhooks (HMAC)      │    │
│  └────────────────────────┬────────────────────────────────────┘    │
│                           │ gRPC                                    │
│  ┌────────────────────────▼────────────────────────────────────┐    │
│  │             DOMAIN SERVICES (own their Firestore data)      │    │
│  │                                                             │    │
│  │  service.user          service.billing   service.registry   │    │
│  │  service.pipeline      service.activity                     │    │
│  └────────────────────────┬────────────────────────────────────┘    │
│                           │ Pub/Sub                                 │
│  ┌────────────────────────▼────────────────────────────────────┐    │
│  │              WORKER SERVICES (Pub/Sub consumers)             │    │
│  │                                                             │    │
│  │  service.destination  (all uploaders)                       │    │
│  │  service.notification (FCM push dispatch)                   │    │
│  └─────────────────────────────────────────────────────────────┘    │
│                                                                     │
│  ┌──────────────┐  ┌────────────────┐  ┌────────────────────────┐   │
│  │   Firestore  │  │ Cloud Storage  │  │  Pub/Sub (4 topics)    │   │
│  │  (User Data) │  │  (FIT / GCS)   │  │  Sentry / Secret Mgr  │   │
│  └──────────────┘  └────────────────┘  └────────────────────────┘   │
└─────────────────────────────────────────────────────────────────────┘
```

**11 Cloud Run services** replace the previous 48 Cloud Functions. All services are pure Go. See [Go Services](go-services.md) for the full directory map.

## Data Flow

### 1. Ingestion

Data enters via `service.api.webhook`, which hosts a generic `WebhookProcessor` backed by a per-provider `SourceProvider` interface (`services/api-webhook/internal/webhook/sources/`):

| Source | Auth | Mechanism |
|--------|------|-----------|
| Hevy | API Key / HMAC | Webhook push |
| Strava | OAuth | Webhook push |
| Fitbit | OAuth | Notification + poll |
| Polar | OAuth | Webhook push |
| Wahoo | OAuth | Webhook push |
| Oura | OAuth | Notification + poll |
| Apple Health | Mobile push (JWT) | Direct API call |
| Health Connect | Mobile push (JWT) | Direct API call |
| FIT Upload | Firebase JWT | Direct file upload |

Each source:
1. Verifies signature/auth
2. Resolves user via `service.user.GetIntegration()` RPC
3. Fetches activity from source API
4. Normalises to `StandardizedActivity` protobuf
5. Publishes to `topic-raw-activity` Pub/Sub

### 2. Pipeline Splitting

`service.pipeline` subscribes to `topic-raw-activity`. For each event it:
1. Looks up user pipelines from Firestore
2. For each matching pipeline, generates a unique `pipelineExecutionId`
3. Publishes a targeted message to `topic-pipeline-activity`

### 3. Enrichment

`service.pipeline` also handles enrichment — one pipeline per invocation:
1. Receives from `topic-pipeline-activity`
2. Runs enrichers sequentially (Go `Provider` interface, 45+ implementations)
3. Generates FIT file → Cloud Storage
4. Creates `PipelineRun` document for lifecycle tracking
5. Publishes `EnrichedActivityEvent` to `topic-enriched-activity`

**Enricher categories:**
- **Data**: Fitbit HR, FIT File HR, Spotify Tracks, Weather, Running Dynamics
- **Stats**: Heart Rate Summary, Pace/Speed/Power/Cadence, Elevation, Training Load, Personal Records
- **Visual**: Muscle Heatmap, Muscle Heatmap Image, Route Thumbnail
- **Detection**: Parkrun, Location Naming, Condition Matcher
- **Transform**: Type Mapper, Auto Increment, Logic Gate, Activity Filter
- **Input**: User Input, Hybrid Race Tagger
- **AI**: AI Companion, AI Banner

### 4. Routing & Destination Upload

`service.pipeline` routes enriched events to `service.destination` via `topic-enriched-activity`. `service.destination` handles all uploaders (Strava, TrainingPeaks, Intervals.icu, Hevy, Fitbit, Google Sheets, GitHub, Showcase).

### 5. Notifications

After pipeline completion or significant events (pending inputs, connection actions), services publish to `topic-notifications`. `service.notification` fans out to active channels (FCM push, with email as a future channel) based on per-user `NotificationPreferences` stored in Firestore.

### 5. Pending Inputs

Enrichers can pause for user input via `WaitForInputError`:
1. Original payload stored in GCS (`originalPayloadUri`)
2. `PendingInput` document created in Firestore
3. User resolves via `service.api.client` → `service.pipeline.SubmitInput()` RPC
4. Pipeline resumes from GCS payload

## Data Model

```
users/{userId}/
  ├── pipelines/{pipelineId}          # Pipeline configurations
  ├── pipeline_runs/{pipelineRunId}   # Execution lifecycle tracking
  ├── pending_inputs/{inputId}        # Paused pipeline inputs
  └── activities/{activityId}         # Synchronized activities

integrations/{provider}/ids/{externalId}  # Reverse-lookup maps
showcased_activities/{id}                 # Public showcase records
```

## Plugin Architecture

| Plugin Type | Language | Purpose | Location |
|-------------|----------|---------|----------|
| **Source** | Go | Ingests data from external services | `services/api-webhook/internal/webhook/sources/` |
| **Enricher** | Go | Transforms activities in pipeline | `internal/pipeline/` |
| **Destination** | Go | Uploads processed activities | `services/destination/internal/` |
| **Registry** | Go | Plugin manifests, categories, icons | `services/registry/` |

All plugins self-register via `init()`. The registry serves configuration via `service.registry` → `GET /api/registry`.

## Observability

- **Sentry**: `slog`-based `SentryHandler` wraps Go logging; all services integrate
- **Health checks**: gRPC health protocol on all domain services
- **Execution tracking**: `PipelineRun` documents in Firestore per pipeline execution

## Key Technologies

| Component | Technology |
|-----------|------------|
| Compute | Cloud Run (Go binaries) |
| Messaging | Cloud Pub/Sub (4 topics) |
| Database | Cloud Firestore |
| Storage | Cloud Storage |
| Secrets | Secret Manager |
| Inter-service | gRPC (protobuf) |
| Observability | Sentry |
| Infrastructure | Terraform |

## Environments

| Environment | Project ID | URL |
|-------------|------------|-----|
| Dev | `fitglue-server-dev` | `https://dev.fitglue.tech` |
| Test | `fitglue-server-test` | `https://test.fitglue.tech` |
| Prod | `fitglue-server-prod` | `https://fitglue.tech` |

## Related Documentation

- [Go Services](go-services.md) - Service directory structure and patterns
- [API Layers](api-layers.md) - The four HTTP gateways
- [Service Communication](service-communication.md) - gRPC inter-service RPC
- [Plugin System](plugin-system.md) - How plugins work
- [Services & Stores](services-and-stores.md) - Domain service patterns
- [Security](security.md) - Authorization and access control
