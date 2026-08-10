# FitGlue MCP Server (stdio)

`cmd/mcp` is a [Model Context Protocol](https://modelcontextprotocol.io) server
that exposes a read-only FitGlue tool surface to MCP clients (Claude Code,
Claude Desktop, MCP Inspector). It wraps the `api-client` HTTP gateway and
authenticates as a single user with a Firebase ID token.

This is **V0 of the connector roadmap**: a local stdio binary for development
and personal use. The remote (Streamable HTTP + OAuth) server that lets other
users connect from claude.ai will build on the same tool surface.

## Build

```bash
make build              # builds bin/fitglue-mcp along with the other tools
# or directly:
cd src/go && go build -o ../../bin/fitglue-mcp ./cmd/mcp
```

## Configuration

All configuration is via environment variables:

| Variable | Required | Description |
|----------|----------|-------------|
| `FITGLUE_API_URL` | no | Deployment base URL. Defaults to `https://fitglue.tech`; use `https://dev.fitglue.tech` for dev. |
| `FITGLUE_ID_TOKEN` | one of | A Firebase ID token, used as-is. Expires after ~1 hour — fine for short sessions. |
| `FITGLUE_REFRESH_TOKEN` | one of | A Firebase refresh token. The server exchanges it for ID tokens automatically (and keeps rotated refresh tokens), so long-lived sessions keep working. Requires `FITGLUE_FIREBASE_API_KEY`. |
| `FITGLUE_FIREBASE_API_KEY` | with refresh token | The Firebase Web API key of the target project (visible in the web app's Firebase config — it is not a secret). |

Getting tokens for development: sign in to the FitGlue web app and grab
`idToken` / `refreshToken` from the Firebase Auth state (Application →
IndexedDB → `firebaseLocalStorage` in devtools), or mint a custom token with
`firebase-admin` and exchange it via the Identity Toolkit REST API.

## Register with Claude Code

```bash
claude mcp add fitglue \
  -e FITGLUE_REFRESH_TOKEN=... \
  -e FITGLUE_FIREBASE_API_KEY=... \
  -- /path/to/server/bin/fitglue-mcp
```

Then ask Claude things like "which of my pipelines failed this week and why?".

## Tools

All V0 tools are read-only (annotated `readOnlyHint: true`) and return the
gateway's JSON responses with credential-keyed fields redacted.

The recent-activities list is backed by pipeline runs; the **showcase index**
covers the full account history in summary form. Pass
`include_showcase: true` to `list_activities` to get both in one call, then
`get_showcase` for a historical activity's full snapshot (heart-rate records
included, where the payload is still retained).

| Tool | Endpoint |
|------|----------|
| `get_profile` | `GET /users/me` |
| `list_integrations` | `GET /users/me/integrations` |
| `list_activities` | `GET /users/me/activities` (+ `GET /users/me/showcases` when `include_showcase` is set) |
| `get_activity` | `GET /users/me/activities/{id}` |
| `get_showcase` | `GET /users/me/showcases/{id}` |
| `get_activity_stats` | `GET /users/me/activities/stats` |
| `list_pipelines` | `GET /users/me/pipelines` |
| `get_pipeline` | `GET /users/me/pipelines/{id}` |
| `list_pipeline_runs` | `GET /users/me/pipelines/{id}/runs` |
| `get_pipeline_run` | `GET /users/me/pipelines/{id}/runs/{runId}` |

Write tools (create/update pipelines, backfills, reposts) are deliberately
deferred until the remote server exists with scope-based OAuth to gate them.

## Testing

```bash
cd src/go && go test -short ./cmd/mcp/
```

The tests cover config validation, token refresh/rotation, the gateway client,
and an in-memory MCP session exercising the registered tools. To poke at the
server interactively:

```bash
npx @modelcontextprotocol/inspector ./bin/fitglue-mcp
```
