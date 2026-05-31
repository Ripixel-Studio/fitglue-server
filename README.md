# FitGlue Server

**FitGlue** is a fitness data aggregation and routing platform built on Google Cloud Platform. It ingests workout data from multiple sources (Hevy, Fitbit, Strava, Polar, Oura, Wahoo, Apple Health, Health Connect), enriches it through configurable pipelines, and routes it to connected services.

## Architecture

FitGlue uses a domain-service architecture: 11 Go Cloud Run services communicating via gRPC, with a thin API gateway layer for HTTP clients.

```text
┌─────────────────┐      ┌──────────────────────┐
│  Data Sources   │─────▶│  service.api.webhook  │
│ (Hevy/Strava/   │      │  (HMAC / OAuth auth)  │
│  Fitbit / etc.) │      └──────────┬────────────┘
└─────────────────┘                 │ Pub/Sub
                                    ▼
                          ┌──────────────────────┐
                          │   service.pipeline    │
                          │   (enrich, split,     │
                          │    route)             │
                          └──────────┬────────────┘
                                     │ Pub/Sub
                                     ▼
                          ┌──────────────────────┐
                          │  service.destination  │
                          │  (upload to Strava,   │
                          │   TrainingPeaks, etc.)│
                          └──────────────────────┘
```

**API gateways** sit in front for HTTP clients:
- `service.api.client` — Web app / mobile (Firebase JWT)
- `service.api.admin` — Admin tooling
- `service.api.public` — Unauthenticated (registry, showcase)

**Domain services** own their Firestore data and expose gRPC interfaces:
- `service.user`, `service.billing`, `service.pipeline`, `service.activity`, `service.registry`

**Worker services** consume Pub/Sub topics:
- `service.destination` — uploads to Strava, TrainingPeaks, Fitbit, etc.
- `service.notification` — FCM push dispatch, respects per-user notification preferences

See [Architecture Overview](docs/architecture/overview.md) for the full system diagram.

## Tech Stack

| Component | Technology |
|-----------|------------|
| Language | Go 1.25+ |
| Compute | Cloud Run (Docker containers) |
| Messaging | Cloud Pub/Sub |
| Database | Cloud Firestore |
| Storage | Cloud Storage (FIT files, GCS payloads) |
| Inter-service | gRPC (protobuf) |
| CI/CD | CircleCI + OIDC |
| Infrastructure | Terraform |
| Observability | Sentry, structured `slog` |

## Quick Start

### Prerequisites

- Go 1.25+
- Docker + Docker Compose
- `protoc` (Protocol Buffers compiler)
- Google Cloud SDK

### Setup

```bash
# Install Go dependencies and generate proto stubs
make setup
make generate

# Build all services
make build

# Run unit tests
make test

# Start local development stack (all 11 services)
make local
```

See [Local Development](docs/development/local-development.md) for detailed instructions.

## Project Structure

```text
fitglue-server/
├── src/
│   ├── go/                    # Go monorepo
│   │   ├── services/          # 11 Cloud Run services (main.go per service)
│   │   ├── internal/          # Domain logic (owned by services)
│   │   ├── pkg/               # Shared packages (proto stubs, integrations)
│   │   ├── cmd/               # CLI tools (fit-gen, fit-inspect)
│   │   └── tests/e2e/         # Cucumber/godog E2E tests
│   └── proto/                 # Protocol Buffer definitions
├── terraform/                 # Infrastructure as Code
├── docs/                      # Documentation
│   ├── api/                   # OpenAPI spec (auto-generated)
│   ├── architecture/          # Architecture guides
│   ├── decisions/             # ADRs + history
│   ├── development/           # Dev guides
│   ├── guides/                # How-to guides
│   ├── infrastructure/        # CI/CD, Terraform
│   └── reference/             # Admin CLI, errors, registry
├── scripts/                   # Dev and CI scripts
└── Makefile                   # Build, test, generate, deploy commands
```

## Documentation

### Getting Started
- [Architecture Overview](docs/architecture/overview.md) — System topology and data flow
- [Go Services](docs/architecture/go-services.md) — Service structure, IoC pattern, directory map
- [Local Development](docs/development/local-development.md) — Running the stack locally
- [API Layers](docs/architecture/api-layers.md) — Admin API via `service.api.admin`

### Architecture
- [API Layers](docs/architecture/api-layers.md) — The four HTTP gateways
- [Service Communication](docs/architecture/service-communication.md) — gRPC inter-service RPC
- [Services & Stores](docs/architecture/services-and-stores.md) — Domain service patterns
- [Plugin System](docs/architecture/plugin-system.md) — Sources, enrichers, destinations
- [Security](docs/architecture/security.md) — Auth and access control

### Development
- [Testing Guide](docs/development/testing.md) — Unit, E2E, and integration tests
- [Enricher Testing](docs/guides/enricher-testing.md) — Testing enrichment providers
- [OAuth Integration](docs/guides/oauth-integration.md) — Strava and Fitbit OAuth setup

### Troubleshooting
- [Troubleshooting Guide](docs/guides/troubleshooting.md) — **Start here** when debugging any issue
- [Error Codes Reference](docs/reference/errors.md) — All error codes and retryability
- [Monitoring & Analytics](docs/infrastructure/monitoring.md) — Dashboards and alerts

### Infrastructure
- [CI/CD Guide](docs/infrastructure/cicd.md) — Pipeline and Cloud Run deployments

### Decisions
- [Architecture Decisions (ADR)](docs/decisions/ADR.md) — Key design choices
- [Go Migration Proposal](docs/decisions/history/go-migration-proposal.md) — Historical migration rationale

## Contributing

This is a personal project, but suggestions and feedback are welcome via issues.

## License

MIT
