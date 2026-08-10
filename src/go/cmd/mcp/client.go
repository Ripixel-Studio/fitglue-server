package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"time"
)

// APIClient is a minimal authenticated wrapper over the api-client gateway's
// /api/v2 REST surface. Responses are passed through as JSON rather than
// re-modelled: the MCP consumer (an LLM) reads the payloads directly, so the
// gateway's OpenAPI-documented shapes are the contract.
type APIClient struct {
	baseURL string
	tokens  TokenSource
	client  *http.Client
}

func NewAPIClient(baseURL string, tokens TokenSource) *APIClient {
	return &APIClient{
		baseURL: strings.TrimRight(baseURL, "/"),
		tokens:  tokens,
		client:  &http.Client{Timeout: 30 * time.Second},
	}
}

// Get performs an authenticated GET against /api/v2 and returns the response
// body, pretty-printed when it is valid JSON.
func (c *APIClient) Get(ctx context.Context, path string, query url.Values) (string, error) {
	token, err := c.tokens.Token(ctx)
	if err != nil {
		return "", fmt.Errorf("obtaining auth token: %w", err)
	}

	u := c.baseURL + "/api/v2" + path
	if len(query) > 0 {
		u += "?" + query.Encode()
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
	if err != nil {
		return "", fmt.Errorf("building request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Accept", "application/json")

	resp, err := c.client.Do(req)
	if err != nil {
		return "", fmt.Errorf("calling %s: %w", path, err)
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(io.LimitReader(resp.Body, 8<<20))
	if err != nil {
		return "", fmt.Errorf("reading response from %s: %w", path, err)
	}

	switch {
	case resp.StatusCode == http.StatusUnauthorized:
		return "", fmt.Errorf("unauthorized (HTTP 401) — the Firebase token was rejected; refresh FITGLUE_ID_TOKEN or check the refresh-token config")
	case resp.StatusCode == http.StatusNotFound:
		return "", fmt.Errorf("not found (HTTP 404): %s", path)
	case resp.StatusCode < 200 || resp.StatusCode > 299:
		return "", fmt.Errorf("%s returned HTTP %d: %s", path, resp.StatusCode, truncate(string(body), 500))
	}

	return prettyJSON(scrubSecrets(body)), nil
}

// prettyJSON re-indents valid JSON and passes anything else through untouched.
func prettyJSON(body []byte) string {
	var out bytes.Buffer
	if err := json.Indent(&out, bytes.TrimSpace(body), "", "  "); err != nil {
		return string(body)
	}
	return out.String()
}

// mergeJSON embeds a second JSON document into a base JSON object under the
// given key, returning pretty-printed JSON. The gateway wraps list responses
// in a single-key envelope (e.g. {"showcases": [...]}); when the extra
// document has that shape its inner value is embedded directly.
func mergeJSON(base, key, extra string) (string, error) {
	var b map[string]any
	if err := json.Unmarshal([]byte(base), &b); err != nil {
		return "", fmt.Errorf("merging %s: base response is not a JSON object: %w", key, err)
	}
	var e any
	if err := json.Unmarshal([]byte(extra), &e); err != nil {
		return "", fmt.Errorf("merging %s: %w", key, err)
	}
	if obj, ok := e.(map[string]any); ok && len(obj) == 1 {
		for _, v := range obj {
			e = v
		}
	}
	b[key] = e
	out, err := json.MarshalIndent(b, "", "  ")
	if err != nil {
		return "", err
	}
	return string(out), nil
}

// secretKeys matches JSON object keys whose values must never reach the MCP
// client. The gateway returns the user's own integration credentials (e.g.
// GET /users/me/integrations includes provider OAuth tokens); an MCP consumer
// feeds tool output into LLM context and conversation logs, so those values
// are redacted here regardless of endpoint.
var secretKeys = regexp.MustCompile(`(?i)^(accesstoken|refreshtoken|access_token|refresh_token|apikey|api_key|clientsecret|client_secret|password|fcmtokens|fcm_tokens)$`)

// scrubSecrets redacts secret-keyed values anywhere in a JSON document.
// Non-JSON bodies pass through untouched.
func scrubSecrets(body []byte) []byte {
	var doc any
	if err := json.Unmarshal(body, &doc); err != nil {
		return body
	}
	scrubbed, err := json.Marshal(scrubValue(doc))
	if err != nil {
		return body
	}
	return scrubbed
}

func scrubValue(v any) any {
	switch t := v.(type) {
	case map[string]any:
		for k, val := range t {
			if secretKeys.MatchString(k) {
				t[k] = "[redacted]"
				continue
			}
			t[k] = scrubValue(val)
		}
		return t
	case []any:
		for i, val := range t {
			t[i] = scrubValue(val)
		}
		return t
	default:
		return v
	}
}
