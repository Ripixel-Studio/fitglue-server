package server

import (
	"net/http"
	"os"
	"strings"
	"testing"

	"github.com/go-chi/chi/v5"
	"gopkg.in/yaml.v3"

	"github.com/fitglue/server/src/go/internal/infra"
)

// openAPISpec represents the subset of OpenAPI 3.0 needed for route validation
type openAPISpec struct {
	Paths map[string]map[string]struct {
		Tags        []string `yaml:"tags"`
		OperationID string   `yaml:"operationId"`
	} `yaml:"paths"`
}

// TestRouteCoverage validates that every path in the gateway OpenAPI spec
// tagged as ClientGatewayService has a corresponding Chi route registered.
//
// This prevents the common drift scenario where a new endpoint is defined in
// the gateway proto but not wired up in the Go handler, or vice versa.
func TestRouteCoverage(t *testing.T) {
	specPath := "../../../../../../docs/api/openapi.yaml"

	specBytes, err := os.ReadFile(specPath)
	if err != nil {
		t.Skipf("OpenAPI spec not found at %s — run 'make generate' first: %v", specPath, err)
		return
	}

	var spec openAPISpec
	if err := yaml.Unmarshal(specBytes, &spec); err != nil {
		t.Fatalf("Failed to parse OpenAPI spec: %v", err)
	}

	// Extract all ClientGatewayService paths from the spec
	type specRoute struct {
		path   string
		method string
	}
	var expectedRoutes []specRoute

	for path, methods := range spec.Paths {
		for method, detail := range methods {
			for _, tag := range detail.Tags {
				if tag == "ClientGatewayService" {
					expectedRoutes = append(expectedRoutes, specRoute{
						path:   "/api/v2" + path, // Routes are registered under /api/v2
						method: strings.ToUpper(method),
					})
				}
			}
		}
	}

	if len(expectedRoutes) == 0 {
		t.Fatal("No ClientGatewayService routes found in OpenAPI spec — is the spec generated correctly?")
	}

	t.Logf("Found %d ClientGatewayService routes in OpenAPI spec", len(expectedRoutes))

	// Build the actual router using NewAPIServer with mock service clients.
	// We pass nil for authClient since we only need the router structure, not auth behaviour.
	srv := NewAPIServer(
		infra.NewLogger(),
		nil, // authClient
		&mockPublisher{},
		nil, // apiKeyStore
		nil, // gcsSigner (not needed for route structure)
		nil, // statsStore (not needed for route structure)
		&mockUserServiceClient{},
		&mockBillingServiceClient{},
		&mockPipelineServiceClient{},
		&mockActivityServiceClient{},
		&mockRegistryServiceClient{},
	)

	registeredRoutes := make(map[string]bool)

	err = chi.Walk(srv.router, func(method, route string, handler http.Handler, middlewares ...func(http.Handler) http.Handler) error {
		// Normalize Chi route patterns
		registeredRoutes[method+" "+route] = true
		return nil
	})
	if err != nil {
		t.Fatalf("Failed to walk Chi routes: %v", err)
	}

	t.Logf("Found %d registered Chi routes", len(registeredRoutes))

	// Check each spec route is registered
	var missing []string
	for _, sr := range expectedRoutes {
		// OpenAPI and Chi both use {param} for path parameters
		key := sr.method + " " + sr.path
		if !registeredRoutes[key] {
			missing = append(missing, key)
		}
	}

	if len(missing) > 0 {
		t.Errorf("The following %d routes are defined in the OpenAPI spec but not registered in the Chi router:", len(missing))
		for _, m := range missing {
			t.Errorf("  - %s", m)
		}
		t.Error("\nEither add the route in the Go handler, or update the gateway proto to remove it.")
	}
}
