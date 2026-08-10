package main

import (
	"context"
	"fmt"
	"net/url"
	"strconv"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// V0 exposes a read-only tool surface: every tool wraps a GET endpoint on the
// api-client gateway and carries readOnlyHint annotations. Write tools come
// later, once scope-based OAuth exists to gate them.

func boolPtr(b bool) *bool { return &b }

func readOnly(title string) *mcp.ToolAnnotations {
	return &mcp.ToolAnnotations{
		Title:         title,
		ReadOnlyHint:  true,
		OpenWorldHint: boolPtr(false),
	}
}

type emptyArgs struct{}

type pageArgs struct {
	Limit     int    `json:"limit,omitempty" jsonschema:"Maximum number of items to return"`
	PageToken string `json:"page_token,omitempty" jsonschema:"Opaque cursor from a previous response's next_page_token"`
}

type listActivitiesArgs struct {
	Limit           int    `json:"limit,omitempty" jsonschema:"Maximum number of items to return"`
	PageToken       string `json:"page_token,omitempty" jsonschema:"Opaque cursor from a previous response's next_page_token"`
	IncludeShowcase bool   `json:"include_showcase,omitempty" jsonschema:"Also return the user's showcase index — one summary entry per showcased activity covering the full account history. Use it when activities older than the recent list are needed; fetch detail for an entry with get_showcase"`
}

func (p pageArgs) query() url.Values {
	q := url.Values{}
	if p.Limit > 0 {
		q.Set("limit", strconv.Itoa(p.Limit))
	}
	if p.PageToken != "" {
		q.Set("page_token", p.PageToken)
	}
	return q
}

type idArgs struct {
	ID string `json:"id" jsonschema:"Resource identifier"`
}

type pipelineRunsArgs struct {
	PipelineID string `json:"pipeline_id" jsonschema:"Pipeline identifier"`
	Limit      int    `json:"limit,omitempty" jsonschema:"Maximum number of runs to return"`
	PageToken  string `json:"page_token,omitempty" jsonschema:"Opaque cursor from a previous response's next_page_token"`
}

type pipelineRunArgs struct {
	PipelineID string `json:"pipeline_id" jsonschema:"Pipeline identifier"`
	RunID      string `json:"run_id" jsonschema:"Pipeline run identifier"`
}

// registerTools attaches the FitGlue tool surface to the MCP server.
func registerTools(server *mcp.Server, api *APIClient) {
	text := func(s string, err error) (*mcp.CallToolResult, any, error) {
		if err != nil {
			return nil, nil, err
		}
		return &mcp.CallToolResult{Content: []mcp.Content{&mcp.TextContent{Text: s}}}, nil, nil
	}

	mcp.AddTool(server, &mcp.Tool{
		Name:        "get_profile",
		Description: "Get the authenticated FitGlue user's profile, including tier and settings.",
		Annotations: readOnly("Get profile"),
	}, func(ctx context.Context, _ *mcp.CallToolRequest, _ emptyArgs) (*mcp.CallToolResult, any, error) {
		return text(api.Get(ctx, "/users/me", nil))
	})

	mcp.AddTool(server, &mcp.Tool{
		Name:        "list_integrations",
		Description: "List the user's connected source and destination integrations (Strava, Fitbit, Hevy, …) with their connection status.",
		Annotations: readOnly("List integrations"),
	}, func(ctx context.Context, _ *mcp.CallToolRequest, _ emptyArgs) (*mcp.CallToolResult, any, error) {
		return text(api.Get(ctx, "/users/me/integrations", nil))
	})

	mcp.AddTool(server, &mcp.Tool{
		Name:        "list_activities",
		Description: "List the user's synchronized activities, newest first. Paginate with limit and page_token. Set include_showcase to also get the showcase index, which covers the full account history in summary form.",
		Annotations: readOnly("List activities"),
	}, func(ctx context.Context, _ *mcp.CallToolRequest, args listActivitiesArgs) (*mcp.CallToolResult, any, error) {
		q := pageArgs{Limit: args.Limit, PageToken: args.PageToken}.query()
		acts, err := api.Get(ctx, "/users/me/activities", q)
		if err != nil {
			return nil, nil, err
		}
		if !args.IncludeShowcase {
			return text(acts, nil)
		}
		sc, err := api.Get(ctx, "/users/me/showcases", nil)
		if err != nil {
			return nil, nil, fmt.Errorf("activities fetched, but showcase index failed: %w", err)
		}
		return text(mergeJSON(acts, "showcase_index", sc))
	})

	mcp.AddTool(server, &mcp.Tool{
		Name:        "get_activity",
		Description: "Get one activity by ID, including its enriched fields and upload results.",
		Annotations: readOnly("Get activity"),
	}, func(ctx context.Context, _ *mcp.CallToolRequest, args idArgs) (*mcp.CallToolResult, any, error) {
		return text(api.Get(ctx, "/users/me/activities/"+url.PathEscape(args.ID), nil))
	})

	mcp.AddTool(server, &mcp.Tool{
		Name:        "get_activity_stats",
		Description: "Get aggregate statistics across the user's activities (counts, totals, by-sport breakdowns).",
		Annotations: readOnly("Get activity stats"),
	}, func(ctx context.Context, _ *mcp.CallToolRequest, _ emptyArgs) (*mcp.CallToolResult, any, error) {
		return text(api.Get(ctx, "/users/me/activities/stats", nil))
	})

	mcp.AddTool(server, &mcp.Tool{
		Name:        "list_pipelines",
		Description: "List the user's pipelines: source → enricher chain → destinations.",
		Annotations: readOnly("List pipelines"),
	}, func(ctx context.Context, _ *mcp.CallToolRequest, _ emptyArgs) (*mcp.CallToolResult, any, error) {
		return text(api.Get(ctx, "/users/me/pipelines", nil))
	})

	mcp.AddTool(server, &mcp.Tool{
		Name:        "get_pipeline",
		Description: "Get one pipeline's full configuration by ID.",
		Annotations: readOnly("Get pipeline"),
	}, func(ctx context.Context, _ *mcp.CallToolRequest, args idArgs) (*mcp.CallToolResult, any, error) {
		return text(api.Get(ctx, "/users/me/pipelines/"+url.PathEscape(args.ID), nil))
	})

	mcp.AddTool(server, &mcp.Tool{
		Name:        "list_pipeline_runs",
		Description: "List recent execution runs for a pipeline (last 7 days), including status and errors. Paginate with limit and page_token.",
		Annotations: readOnly("List pipeline runs"),
	}, func(ctx context.Context, _ *mcp.CallToolRequest, args pipelineRunsArgs) (*mcp.CallToolResult, any, error) {
		q := pageArgs{Limit: args.Limit, PageToken: args.PageToken}.query()
		return text(api.Get(ctx, "/users/me/pipelines/"+url.PathEscape(args.PipelineID)+"/runs", q))
	})

	mcp.AddTool(server, &mcp.Tool{
		Name:        "get_showcase",
		Description: "Get one showcased activity by showcase ID, including the full activity snapshot (sessions, laps, heart-rate records) when the payload is still retained. Works for historical activities where get_activity returns not-found — find IDs via list_activities with include_showcase.",
		Annotations: readOnly("Get showcase"),
	}, func(ctx context.Context, _ *mcp.CallToolRequest, args idArgs) (*mcp.CallToolResult, any, error) {
		return text(api.Get(ctx, "/users/me/showcases/"+url.PathEscape(args.ID), nil))
	})

	mcp.AddTool(server, &mcp.Tool{
		Name:        "get_pipeline_run",
		Description: "Get one pipeline run by ID, including per-step enricher results and failure detail.",
		Annotations: readOnly("Get pipeline run"),
	}, func(ctx context.Context, _ *mcp.CallToolRequest, args pipelineRunArgs) (*mcp.CallToolResult, any, error) {
		return text(api.Get(ctx, "/users/me/pipelines/"+url.PathEscape(args.PipelineID)+"/runs/"+url.PathEscape(args.RunID), nil))
	})
}
