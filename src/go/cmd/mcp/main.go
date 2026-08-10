// Command mcp is a stdio Model Context Protocol server exposing a read-only
// FitGlue tool surface. It wraps the api-client HTTP gateway, authenticating
// as a single user via a Firebase ID token (or a refresh token that is
// exchanged automatically).
//
// Configuration (environment):
//
//	FITGLUE_API_URL           Deployment base URL (default https://fitglue.tech)
//	FITGLUE_ID_TOKEN          Static Firebase ID token, or
//	FITGLUE_REFRESH_TOKEN     Firebase refresh token (with FITGLUE_FIREBASE_API_KEY)
//	FITGLUE_FIREBASE_API_KEY  Firebase Web API key for the target project
//
// Register with Claude Code:
//
//	claude mcp add fitglue -e FITGLUE_ID_TOKEN=... -- go run ./src/go/cmd/mcp
//
// See docs/guides/mcp-server.md for details.
package main

import (
	"context"
	"fmt"
	"os"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

const serverVersion = "0.1.0"

func main() {
	cfg, err := ConfigFromEnv()
	if err != nil {
		fmt.Fprintf(os.Stderr, "fitglue-mcp: %v\n", err)
		os.Exit(1)
	}

	api := NewAPIClient(cfg.BaseURL, cfg.TokenSource())

	server := mcp.NewServer(&mcp.Implementation{
		Name:    "fitglue",
		Title:   "FitGlue",
		Version: serverVersion,
	}, nil)
	registerTools(server, api)

	if err := server.Run(context.Background(), &mcp.StdioTransport{}); err != nil {
		fmt.Fprintf(os.Stderr, "fitglue-mcp: server exited: %v\n", err)
		os.Exit(1)
	}
}
