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
// tagged as PublicGatewayService has a corresponding Chi route registered.
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

	type specRoute struct {
		path   string
		method string
	}
	var expectedRoutes []specRoute

	for path, methods := range spec.Paths {
		for method, detail := range methods {
			for _, tag := range detail.Tags {
				if tag == "PublicGatewayService" {
					expectedRoutes = append(expectedRoutes, specRoute{
						path:   "/api/public" + path,
						method: strings.ToUpper(method),
					})
				}
			}
		}
	}

	if len(expectedRoutes) == 0 {
		t.Fatal("No PublicGatewayService routes found in OpenAPI spec — is the spec generated correctly?")
	}

	t.Logf("Found %d PublicGatewayService routes in OpenAPI spec", len(expectedRoutes))

	// NewAPIServer only needs activity + registry service clients
	srv := NewAPIServer(
		infra.NewLogger(),
		nil, // activitySvc — only need router structure, not handler logic
		nil, // registrySvc
		"",  // viewSalt — unused for route structure
	)

	registeredRoutes := make(map[string]bool)

	err = chi.Walk(srv.router, func(method, route string, handler http.Handler, middlewares ...func(http.Handler) http.Handler) error {
		registeredRoutes[method+" "+route] = true
		return nil
	})
	if err != nil {
		t.Fatalf("Failed to walk Chi routes: %v", err)
	}

	t.Logf("Found %d registered Chi routes", len(registeredRoutes))

	var missing []string
	for _, sr := range expectedRoutes {
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
