# Makefile

# --- Variables ---
GOCMD=go
GOBUILD=$(GOCMD) build
GOCLEAN=$(GOCMD) clean
GOTEST=$(GOCMD) test
GOGET=$(GOCMD) get
GOLINT=golangci-lint run
GO_SRC_DIR=src/go
TS_SRC_DIR=src/typescript

# --- Phony Targets ---
.PHONY: all clean build test lint build-go test-go lint-go clean-go build-ts test-ts lint-ts typecheck-ts clean-ts plugin-source plugin-enricher plugin-destination lint-codebase lint-shared-modules tools build-tools-go build-tools-ts prepare prepare-go prepare-ts test-integration test-e2e test-coverage preflight

all: generate clean lint build test

preflight:
	@echo "\n========== PREFLIGHT: Mirroring CI Pipeline =========="
	@echo "\n[1/7] Proto linting..."
	buf lint src/proto
	@echo "\n[2/7] Checking for service-level go.mod files..."
	@if find $(GO_SRC_DIR)/services -name "go.mod" 2>/dev/null | grep -q .; then \
		echo "❌ Found service-level go.mod files — all services must use the root module:"; \
		find $(GO_SRC_DIR)/services -name "go.mod"; \
		exit 1; \
	fi
	@echo "✅ All services use the root module."
	@echo "\n[3/7] Verifying generated code is in sync..."
	$(MAKE) generate
	@if ! git diff --quiet; then \
		echo "❌ Generated files are out of sync. Run 'make generate' and commit the changes."; \
		git diff --stat; \
		exit 1; \
	fi
	@echo "✅ Generated code is in sync."
	@echo "\n[4/7] Building..."
	$(MAKE) build
	@echo "\n[5/7] Linting..."
	$(MAKE) lint
	@echo "\n[6/7] Running tests..."
	$(MAKE) test
	@echo "\n[7/7] Checking coverage..."
	$(MAKE) test-coverage
	@echo "\n========== ✅ PREFLIGHT PASSED — safe to push =========="


setup:
	@echo "Setting up dependencies..."
	@echo "Installing Go dependencies..."
	cd $(GO_SRC_DIR) && $(GOCMD) mod download
	@echo "Installing Node dependencies for generation..."
	npm install
	@echo "Setup complete."

generate:
	@echo "Generating Protocol Buffers..."
	# Generate Go
	# Find all proto files recursively and pass them to protoc
	find src/proto -type f -name "*.proto" ! -path "src/proto/google/*" | xargs protoc \
		--go_out=$(GO_SRC_DIR)/pkg/types/pb \
		--go_opt=module=github.com/fitglue/server/src/go/pkg/types/pb \
		--go-grpc_out=$(GO_SRC_DIR)/pkg/types/pb \
		--go-grpc_opt=module=github.com/fitglue/server/src/go/pkg/types/pb \
		--experimental_allow_proto3_optional \
		--proto_path=src/proto
	# Generate TypeScript (requires ts-proto installed centrally)
	@echo "Generating TypeScript Protobufs..."
	@if [ -d "../web" ]; then \
		mkdir -p ../web/src/types/pb; \
		find src/proto -type f -name "*.proto" ! -path "src/proto/google/*" | xargs protoc \
			--plugin=./node_modules/.bin/protoc-gen-ts_proto \
			--experimental_allow_proto3_optional \
			--ts_proto_out=../web/src/types/pb --ts_proto_opt=outputEncodeMethods=false,outputJsonMethods=false,outputClientImpl=false,useOptionals=messages \
			--proto_path=src/proto; \
		echo "TypeScript protobufs updated at ../web/src/types/pb/"; \
	else \
		echo "Skipping TypeScript Protobuf generation (../web not found)"; \
	fi
	# Generate OpenAPI Clients
	@echo "Generating OpenAPI Clients..."
	@set -e; for dir in src/openapi/*; do \
		if [ -d "$$dir" ]; then \
			SERVICE=$$(basename $$dir); \
			echo "Processing $$SERVICE..."; \
			echo "  [GO] Generating client for $$SERVICE..."; \
			mkdir -p $(GO_SRC_DIR)/pkg/integrations/$$SERVICE; \
			oapi-codegen -package $$SERVICE -generate types,client \
				-o $(GO_SRC_DIR)/pkg/integrations/$$SERVICE/client.gen.go \
				$$dir/swagger.json; \
		fi \
	done
	# Generate per-gateway OpenAPI specs from gateway protos
	@echo "Generating per-gateway OpenAPI specs..."
	@mkdir -p docs/api/gateway
	@for proto in client admin public webhook; do \
		echo "  Generating $$proto gateway spec..."; \
		cd src/proto && buf generate --template buf.gen.openapi.yaml --path gateway/$$proto.proto && cd ../..; \
		mv docs/api/openapi.yaml docs/api/gateway/$$proto.openapi.yaml; \
	done
	@echo "Per-gateway OpenAPI specs updated at docs/api/gateway/"
	# Generate Frontend API Types from per-gateway OpenAPI specs
	@echo "Generating Frontend API Types via openapi-typescript..."
	@if [ -d "../web" ]; then \
		for spec in client admin public; do \
			echo "  Generating schema-$$spec.ts..."; \
			cd ../web && npx -y openapi-typescript ../server/docs/api/gateway/$$spec.openapi.yaml -o src/shared/api/schema-$$spec.ts; \
			cd ../server; \
		done; \
		echo "Frontend API types updated at ../web/src/shared/api/schema-{client,admin,public}.ts"; \
	else \
		echo "Skipping web frontend api type generation (../web not found)"; \
	fi
	# Generate enum formatters (TS + Go)
	@echo "Generating enum formatters..."
	@npx ts-node scripts/generate-enum-formatters.ts


# --- Go Targets ---
build-go:
	@echo "Building Go services..."
	cd $(GO_SRC_DIR) && $(GOBUILD) -v ./...

build: build-go build-tools-go

build-tools-go:
	@echo "Building Go tools..."
	@mkdir -p bin
	@echo "  Building fit-gen tool..."
	cd $(GO_SRC_DIR) && $(GOBUILD) -o ../../bin/fit-gen ./cmd/fit-gen
	@echo "  Building fit-inspect tool..."
	cd $(GO_SRC_DIR) && $(GOBUILD) -o ../../bin/fit-inspect ./cmd/fit-inspect
	@echo "  Building fit-combine tool..."
	cd $(GO_SRC_DIR) && $(GOBUILD) -o ../../bin/fit-combine ./cmd/fit-combine

test:
	@echo "Testing Go services (Unit)..."
	cd $(GO_SRC_DIR) && $(GOTEST) -short -v ./pkg/... ./services/... ./cmd/... ./internal/...

test-integration:
	@echo "Running Integration Tests..."
	cd $(GO_SRC_DIR) && $(GOTEST) -run Integration -v ./...

test-e2e:
	@echo "Running E2E tests via godog..."
	cd $(GO_SRC_DIR)/tests/e2e && go run github.com/cucumber/godog/cmd/godog@latest run

test-coverage:
	@echo "Enforcing test coverage requirements..."
	@./scripts/check-coverage.sh

lint:
	@echo "Linting Go..."
	@echo "Checking formatting..."
	@cd $(GO_SRC_DIR) && test -z "$$(gofmt -l pkg services cmd internal)" || (echo "Go files need formatting. Run 'gofmt -w pkg services cmd internal'" && exit 1)
	@echo "Running go vet (excluding generated clients)..."
	@cd $(GO_SRC_DIR) && go vet $$(go list ./pkg/... ./services/... ./cmd/... ./internal/... | grep -v '/integrations/')
	@echo "Checking for direct sentry-go imports (use infra.Logger.Error instead)..."
	@if grep -rn '"github.com/getsentry/sentry-go"' $(GO_SRC_DIR) --include="*.go" \
		| grep -v "_test.go" \
		| grep -v "pkg/infrastructure/sentry/sentry.go" \
		| grep -v "pkg/framework/wrapper.go"; then \
		echo "❌ Direct sentry-go import found. Use logger.Error(ctx, ...) via infra.Logger — the SentryHandler forwards Error-level logs to Sentry automatically."; \
		exit 1; \
	fi
	@echo "✅ No direct sentry-go imports found."
	@echo "Checking for Protobuf JSON misuse..."
	@./scripts/lint-proto-json.sh

SERVICES := activity api-admin api-client api-public api-webhook billing destination pipeline registry user

docker:
	@for service in $(SERVICES); do \
		echo "Building docker image for $$service..."; \
		docker build -t "fitglue-$$service" --build-arg SERVICE_NAME=$$service .; \
	done

local:
	@echo "Starting up 10 Cloud Run Emulators via Docker Compose..."
	docker-compose up --build

local-down:
	@echo "Tearing down local execution stack..."
	docker-compose down

integration:
	@echo "Running local integration test suite against live containers..."
	npm run test:local

clean:
	@echo "Cleaning Go..."
	cd $(GO_SRC_DIR) && $(GOCLEAN)

# --- Codebase Consistency Check ---

# --- Plugin Scaffolding ---
# Usage: make plugin-source name=garmin
#        make plugin-enricher name=weather
#        make plugin-destination name=runkeeper
plugin-source:
ifndef name
	$(error Usage: make plugin-source name=<name>)
endif
	./scripts/new-plugin.sh source $(name)

plugin-enricher:
ifndef name
	$(error Usage: make plugin-enricher name=<name>)
endif
	./scripts/new-plugin.sh enricher $(name)

plugin-destination:
ifndef name
	$(error Usage: make plugin-destination name=<name>)
endif
	./scripts/new-plugin.sh destination $(name)
