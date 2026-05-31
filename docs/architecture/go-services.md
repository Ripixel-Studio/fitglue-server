# Go Services Architecture

FitGlue's server is composed of **11 Go Cloud Run services** organized under `src/go/services/`. All services use struct-based IoC dependency injection with `main.go` as the composition root.

## Service Directory Map

```
src/go/
├── services/                          # All 11 Cloud Run services
│   ├── api-client/                    # Authenticated user HTTP gateway
│   │   ├── main.go                    # Composition root: wires gRPC clients → chi router
│   │   └── internal/
│   │       └── server/                # HTTP handlers + middleware
│   ├── api-admin/                     # Admin HTTP gateway
│   │   ├── main.go
│   │   └── internal/server/
│   ├── api-public/                    # Unauthenticated HTTP gateway
│   │   ├── main.go
│   │   └── internal/server/
│   ├── api-webhook/                   # Inbound webhook processor
│   │   ├── main.go
│   │   └── internal/
│   │       ├── server/                # HTTP router
│   │       └── webhook/               # Webhook processing
│   │           ├── processor.go       # Generic WebhookProcessor orchestrator
│   │           └── sources/           # One package per source provider
│   │               ├── strava/
│   │               ├── fitbit/
│   │               ├── hevy/
│   │               ├── polar/
│   │               ├── mobile/
│   │               └── ...
│   ├── user/                          # User profiles, integrations, OAuth tokens
│   │   └── main.go
│   ├── billing/                       # Subscriptions, Stripe, tier enforcement
│   │   └── main.go
│   ├── pipeline/                      # Pipeline CRUD, splitting, enrichment, routing
│   │   └── main.go
│   ├── activity/                      # Activities, showcases, exports
│   │   └── main.go
│   ├── registry/                      # Plugin manifests, categories, icons
│   │   └── main.go
│   ├── destination/                   # All destination uploaders
│   │   └── main.go
│   └── notification/                  # FCM push dispatch, per-user preference routing
│       └── main.go
│
├── internal/                          # Shared internal implementations
│   ├── user/                          # service.user business logic + store
│   ├── billing/                       # service.billing business logic + store
│   ├── pipeline/                      # Pipeline orchestration, enrichers (45+), routing
│   ├── activity/                      # Activity CRUD, showcases, exports
│   ├── registry/                      # Plugin registry logic
│   └── infra/                         # Shared infrastructure (logger, Firestore client)
│
└── pkg/                               # Shared packages used across services
    ├── types/pb/                      # Generated protobuf Go + gRPC stubs
    ├── plugin/                        # Plugin manifest types and registration
    ├── integrations/                  # Generated OpenAPI clients (oapi-codegen)
    └── infrastructure/                # Email, secrets, etc.
```

## IoC Composition Pattern

Every service wires its dependencies explicitly in `main.go` — no globals, no `sync.Once` singletons:

```go
// services/user/main.go
func main() {
    // 1. Infrastructure
    logger := infra.NewLogger()
    fsClient := infra.NewFirestoreClient(ctx)

    // 2. Store (data access layer)
    store := user.NewFirestoreStore(fsClient)

    // 3. Domain service (business logic)
    svc := user.NewService(store, logger, emailSender, authClient)

    // 4. gRPC server
    server := grpc.NewServer()
    pbsvc.RegisterUserServiceServer(server, svc)
    server.Serve(listener)
}
```

If it compiles, it's wired correctly — no runtime dependency resolution.

## Service Responsibilities

| Service | Owns | Transport | Data Store |
|---------|------|-----------|-----------|
| `service.api.client` | None (thin marshaller) | HTTP (Firebase JWT) | None |
| `service.api.admin` | None (thin marshaller) | HTTP (admin auth) | None |
| `service.api.public` | None (thin marshaller) | HTTP (no auth) | None |
| `service.api.webhook` | None (thin orchestrator) | HTTP (HMAC / mobile JWT) | Transient |
| `service.user` | User profiles, integrations, OAuth tokens, counters | gRPC | Firestore `users/` |
| `service.billing` | Subscriptions, trial, tier enforcement | gRPC | Firestore billing subcollections |
| `service.pipeline` | Pipeline config, enrichment, routing, pending inputs | gRPC + Pub/Sub | Firestore `users/*/pipelines` |
| `service.activity` | Activity records, showcases, FIT parsing, exports | gRPC + Pub/Sub | Firestore activities + GCS |
| `service.registry` | Plugin manifests, categories | gRPC | Static config |
| `service.destination` | Route and upload to destinations | Pub/Sub | Transient |
| `service.notification` | FCM push dispatch, respects per-user channel preferences | Pub/Sub | Firestore `users/` (reads prefs + FCM tokens) |

## Source Provider Pattern

`service.api.webhook` uses an interface + registry to avoid a monolith. Each source provider is a separate Go package:

```go
// internal/webhook/sources/interfaces.go
type SourceProvider interface {
    Source() string
    VerifyWebhook(r *http.Request) error
    ResolveUser(ctx context.Context, body []byte) (userID string, err error)
    FetchActivity(ctx context.Context, externalID string, creds *pb.OAuthTokens) (*pb.StandardizedActivity, error)
    WebhookRoutes() []Route
}
```

Each provider registers itself via `init()`. The generic `WebhookProcessor` handles all shared lifecycle:
1. Provider-specific: verify signature, resolve user, fetch & map activity
2. Shared: get credentials via RPC to `service.user`, dedup check, publish to Pub/Sub

**Adding a new source:** create `services/api-webhook/internal/webhook/sources/{name}/provider.go`, implement `SourceProvider`, register in `init()`. No other files touched.

## Proto-Generated gRPC Stubs

All service interfaces are defined in `src/proto/services/` and generated to `src/go/pkg/types/pb/`:

```bash
make generate   # Runs protoc for Go gRPC stubs + buf for OpenAPI spec
```

The `buf.gen.yaml` generates:
- **Go gRPC stubs** → `src/go/pkg/types/pb/`
- **OpenAPI 3.x spec** → `docs/api/openapi.yaml`
- **TypeScript types** (via Makefile ts-proto) → `../web/src/types/pb/`

## Health Checks

All domain services register gRPC health protocol:

```go
healthcheck := health.NewServer()
grpc_health_v1.RegisterHealthServer(server, healthcheck)
```

Cloud Run uses this for liveness/readiness probes.

## Related Documentation

- [Architecture Overview](overview.md) - System topology and data flow
- [API Layers](api-layers.md) - The four HTTP gateways in detail
- [Service Communication](service-communication.md) - gRPC inter-service RPC
- [Services & Stores](services-and-stores.md) - Store and service patterns
