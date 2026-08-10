package main

import (
	"context"
	"fmt"
	"net/http"
	"strings"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// startSession wires the tool surface to an httptest-backed APIClient and
// returns a connected in-memory client session.
func startSession(t *testing.T, handler http.HandlerFunc) *mcp.ClientSession {
	t.Helper()
	api := newTestAPI(t, handler)

	server := mcp.NewServer(&mcp.Implementation{Name: "fitglue-test", Version: "test"}, nil)
	registerTools(server, api)

	serverT, clientT := mcp.NewInMemoryTransports()
	ctx := context.Background()
	serverSession, err := server.Connect(ctx, serverT, nil)
	if err != nil {
		t.Fatalf("server.Connect() error = %v", err)
	}
	t.Cleanup(func() { _ = serverSession.Wait() })

	client := mcp.NewClient(&mcp.Implementation{Name: "test-client", Version: "test"}, nil)
	clientSession, err := client.Connect(ctx, clientT, nil)
	if err != nil {
		t.Fatalf("client.Connect() error = %v", err)
	}
	t.Cleanup(func() { _ = clientSession.Close() })
	return clientSession
}

func TestToolsAreRegisteredWithReadOnlyAnnotations(t *testing.T) {
	session := startSession(t, func(w http.ResponseWriter, r *http.Request) {})

	res, err := session.ListTools(context.Background(), nil)
	if err != nil {
		t.Fatalf("ListTools() error = %v", err)
	}

	want := map[string]bool{
		"get_profile": false, "list_integrations": false,
		"list_activities": false, "get_activity": false, "get_activity_stats": false,
		"list_pipelines": false, "get_pipeline": false,
		"list_pipeline_runs": false, "get_pipeline_run": false,
		"get_showcase": false,
	}
	for _, tool := range res.Tools {
		if _, ok := want[tool.Name]; !ok {
			t.Errorf("unexpected tool %q", tool.Name)
			continue
		}
		want[tool.Name] = true
		if tool.Annotations == nil || !tool.Annotations.ReadOnlyHint {
			t.Errorf("tool %q missing readOnlyHint", tool.Name)
		}
		if tool.Annotations == nil || tool.Annotations.Title == "" {
			t.Errorf("tool %q missing annotation title", tool.Name)
		}
	}
	for name, seen := range want {
		if !seen {
			t.Errorf("tool %q not registered", name)
		}
	}
}

func TestCallToolRoutesToGateway(t *testing.T) {
	var gotPath string
	session := startSession(t, func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		fmt.Fprint(w, `{"id":"p1","name":"Morning runs"}`)
	})

	res, err := session.CallTool(context.Background(), &mcp.CallToolParams{
		Name:      "get_pipeline",
		Arguments: map[string]any{"id": "p1"},
	})
	if err != nil {
		t.Fatalf("CallTool() error = %v", err)
	}
	if res.IsError {
		t.Fatalf("CallTool() returned tool error: %+v", res.Content)
	}
	if gotPath != "/api/v2/users/me/pipelines/p1" {
		t.Errorf("gateway path = %q", gotPath)
	}
	text, ok := res.Content[0].(*mcp.TextContent)
	if !ok || !strings.Contains(text.Text, "Morning runs") {
		t.Errorf("content = %+v, want pipeline JSON text", res.Content)
	}
}

func TestCallToolSurfacesAPIErrors(t *testing.T) {
	session := startSession(t, func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "nope", http.StatusUnauthorized)
	})

	res, err := session.CallTool(context.Background(), &mcp.CallToolParams{Name: "get_profile"})
	if err != nil {
		t.Fatalf("CallTool() transport error = %v", err)
	}
	if !res.IsError {
		t.Fatal("CallTool() IsError = false, want tool error for 401")
	}
	text, _ := res.Content[0].(*mcp.TextContent)
	if text == nil || !strings.Contains(text.Text, "unauthorized") {
		t.Errorf("error content = %+v, want unauthorized message", res.Content)
	}
}

func TestListActivitiesPassesPagination(t *testing.T) {
	var gotQuery string
	session := startSession(t, func(w http.ResponseWriter, r *http.Request) {
		gotQuery = r.URL.RawQuery
		fmt.Fprint(w, `{"activities":[]}`)
	})

	_, err := session.CallTool(context.Background(), &mcp.CallToolParams{
		Name:      "list_activities",
		Arguments: map[string]any{"limit": 10, "page_token": "tok123"},
	})
	if err != nil {
		t.Fatalf("CallTool() error = %v", err)
	}
	if !strings.Contains(gotQuery, "limit=10") || !strings.Contains(gotQuery, "page_token=tok123") {
		t.Errorf("query = %q, want limit and page_token", gotQuery)
	}
}

func TestListActivitiesIncludeShowcase(t *testing.T) {
	session := startSession(t, func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/v2/users/me/activities":
			fmt.Fprint(w, `{"activities":[{"id":"a1"}],"nextPageToken":""}`)
		case "/api/v2/users/me/showcases":
			fmt.Fprint(w, `{"showcases":[{"showcaseId":"sc-1","title":"Tower Pilates"}]}`)
		default:
			http.Error(w, "unexpected "+r.URL.Path, http.StatusTeapot)
		}
	})

	res, err := session.CallTool(context.Background(), &mcp.CallToolParams{
		Name:      "list_activities",
		Arguments: map[string]any{"include_showcase": true},
	})
	if err != nil || res.IsError {
		t.Fatalf("CallTool() err=%v isError=%v content=%+v", err, res != nil && res.IsError, res.Content)
	}
	text := res.Content[0].(*mcp.TextContent).Text
	for _, want := range []string{`"showcase_index"`, "sc-1", "Tower Pilates", `"activities"`} {
		if !strings.Contains(text, want) {
			t.Errorf("merged output missing %q", want)
		}
	}
}

func TestGetShowcaseRoutes(t *testing.T) {
	var gotPath string
	session := startSession(t, func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		fmt.Fprint(w, `{"activityData":{"name":"Slow Flow"}}`)
	})
	res, err := session.CallTool(context.Background(), &mcp.CallToolParams{
		Name:      "get_showcase",
		Arguments: map[string]any{"id": "slow-flow-2026-03-09-ab12"},
	})
	if err != nil || res.IsError {
		t.Fatalf("CallTool() err=%v", err)
	}
	if gotPath != "/api/v2/users/me/showcases/slow-flow-2026-03-09-ab12" {
		t.Errorf("path = %q", gotPath)
	}
}

func TestMergeJSON(t *testing.T) {
	out, err := mergeJSON(`{"a":1}`, "extra", `{"showcases":[1,2]}`)
	if err != nil || !strings.Contains(out, `"extra": [`) {
		t.Errorf("envelope unwrap failed: %v %q", err, out)
	}
	out, err = mergeJSON(`{"a":1}`, "extra", `{"x":1,"y":2}`)
	if err != nil || !strings.Contains(out, `"x": 1`) {
		t.Errorf("multi-key object should embed whole: %v %q", err, out)
	}
	if _, err = mergeJSON(`[]`, "k", `{}`); err == nil {
		t.Error("non-object base should error")
	}
}
