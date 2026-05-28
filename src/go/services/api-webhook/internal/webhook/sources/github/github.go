// nolint:proto-json
package github

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"

	activitypb "github.com/fitglue/server/src/go/pkg/types/pb/models/activity"
	pbevents "github.com/fitglue/server/src/go/pkg/types/pb/models/events"
	userpb "github.com/fitglue/server/src/go/pkg/types/pb/services/user"
	"github.com/fitglue/server/src/go/services/api-webhook/internal/webhook"
)

// Provider implements webhook.SourceProvider for Github
type Provider struct {
	// webhookSecret is the shared HMAC secret configured in the GitHub webhook settings.
	// If empty, signature validation is skipped (development / not-yet-configured).
	// TODO(SEC-03): needs per-user webhook secret storage — this is a single shared secret
	// and does not prevent spoofing between different GitHub accounts.
	webhookSecret string
}

// NewProvider creates a new Github SourceProvider.
// webhookSecret should be sourced from the GITHUB_WEBHOOK_SECRET environment variable.
// If empty, X-Hub-Signature-256 validation is skipped.
func NewProvider(webhookSecret string) *Provider {
	return &Provider{webhookSecret: webhookSecret}
}

// ID returns the provider identifier
func (p *Provider) ID() string {
	return "github"
}

// VerifySubscription handles Github webhook verification
func (p *Provider) VerifySubscription(w http.ResponseWriter, r *http.Request) {
	w.WriteHeader(http.StatusOK)
}

type githubPushEvent struct {
	Ref        string `json:"ref"`
	Repository struct {
		Owner struct {
			Login string `json:"login"`
		} `json:"owner"`
	} `json:"repository"`
	Commits []struct {
		ID        string `json:"id"`
		Committer struct {
			Name  string `json:"name"`
			Email string `json:"email"`
		} `json:"committer"`
	} `json:"commits"`
	HeadCommit struct {
		ID string `json:"id"`
	} `json:"head_commit"`
}

// ParseEvent extracts events from a Github push webhook
func (p *Provider) ParseEvent(r *http.Request) ([]*webhook.WebhookEvent, error) {
	if r.Header.Get("X-GitHub-Event") != "push" {
		return nil, nil // Ignore non-push events
	}

	body, err := io.ReadAll(r.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read body: %w", err)
	}

	// Validate X-Hub-Signature-256 when a shared secret is configured.
	// GitHub sends "sha256=<hex-digest>"; we reject requests that are missing or
	// incorrect so that unauthenticated callers cannot inject arbitrary push events.
	if p.webhookSecret != "" {
		sigHeader := r.Header.Get("X-Hub-Signature-256")
		if sigHeader == "" {
			return nil, fmt.Errorf("github: missing X-Hub-Signature-256 header")
		}
		// Expected format: "sha256=<hex>"
		const prefix = "sha256="
		if len(sigHeader) <= len(prefix) || sigHeader[:len(prefix)] != prefix {
			return nil, fmt.Errorf("github: malformed X-Hub-Signature-256 header")
		}
		gotHex := sigHeader[len(prefix):]

		mac := hmac.New(sha256.New, []byte(p.webhookSecret))
		mac.Write(body)
		expectedHex := hex.EncodeToString(mac.Sum(nil))

		if !hmac.Equal([]byte(gotHex), []byte(expectedHex)) {
			return nil, fmt.Errorf("github: invalid X-Hub-Signature-256")
		}
	}

	var payload githubPushEvent
	if err := json.Unmarshal(body, &payload); err != nil {
		return nil, fmt.Errorf("invalid json: %w", err)
	}

	// Skip FitGlue Bot commits
	allBot := len(payload.Commits) > 0
	for _, c := range payload.Commits {
		if c.Committer.Name != "FitGlue Bot" && c.Committer.Email != "bot@fitglue.com" {
			allBot = false
			break
		}
	}
	if allBot {
		return nil, nil
	}

	commitID := payload.HeadCommit.ID
	if commitID == "" && len(payload.Commits) > 0 {
		commitID = payload.Commits[0].ID
	}
	if commitID == "" {
		return nil, nil
	}

	username := payload.Repository.Owner.Login
	if username == "" {
		return nil, fmt.Errorf("missing repository.owner.login")
	}

	evt := &webhook.WebhookEvent{
		Provider:    p.ID(),
		ProviderUID: username,
		ActivityID:  commitID,
		Event:       "push",
		RawPayload:  body,
	}

	return []*webhook.WebhookEvent{evt}, nil
}

func (p *Provider) FetchActivity(ctx context.Context, userSvc userpb.UserServiceClient, internalUserID string, evt *webhook.WebhookEvent) (*pbevents.ActivityPayload, error) {
	if len(evt.RawPayload) == 0 {
		return nil, fmt.Errorf("missing raw payload for github activity push")
	}

	payload := &pbevents.ActivityPayload{
		Source:              activitypb.ActivitySource_SOURCE_GITHUB,
		UserId:              internalUserID,
		OriginalPayloadJson: string(evt.RawPayload),
		ActivityId:          &evt.ActivityID,
	}

	return payload, nil
}
