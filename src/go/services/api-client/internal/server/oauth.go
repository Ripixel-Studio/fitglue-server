package server

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"net/url"
	"os"
	"strconv"
	"strings"
	"time"

	userpb "github.com/fitglue/server/src/go/pkg/types/pb/services/user"
	"github.com/go-chi/chi/v5"
	"google.golang.org/protobuf/types/known/structpb"
)

// webURL returns the WEB_URL env var, falling back to local dev and appending the /app basename.
func webURL() string {
	url := os.Getenv("WEB_URL")
	if url == "" {
		url = "http://localhost:5173"
	}
	return strings.TrimRight(url, "/") + "/app"
}

// apiURL returns the base API URL for OAuth callbacks.
// API_URL must be set; falling back to the request Host header is unsafe (Host header injection).
func apiURL() string {
	url := os.Getenv("API_URL")
	if url == "" {
		panic("API_URL environment variable must be set")
	}
	return strings.TrimRight(url, "/")
}

func (s *APIServer) registerOAuthRoutes(r chi.Router) {
	// OAuth endpoints require user authentication because we need to know WHICH user is connecting the integration
	r.Post("/users/me/integrations/{provider}/connect", s.handleOAuthConnect)
}

func (s *APIServer) handleOAuthConnect(w http.ResponseWriter, r *http.Request) {
	token := getUserToken(r)
	if token == nil {
		WriteError(w, statusError(http.StatusUnauthorized, "missing user context"))
		return
	}

	provider := chi.URLParam(r, "provider")
	config := GetOAuthConfig(provider)
	if config == nil {
		WriteError(w, statusError(http.StatusBadRequest, "unsupported provider"))
		return
	}

	stateData := map[string]string{
		"uid": token.UID,
		"ts":  strconv.FormatInt(time.Now().Unix(), 10),
	}
	stateJSON, _ := json.Marshal(stateData)
	h := hmac.New(sha256.New, []byte(os.Getenv("OAUTH_STATE_SECRET")))
	h.Write(stateJSON)
	signature := hex.EncodeToString(h.Sum(nil))

	stateArg := base64.URLEncoding.EncodeToString(stateJSON) + "." + signature

	redirectURI := apiURL() + "/api/v2/oauth/" + provider + "/callback"

	authURL, _ := url.Parse(config.AuthURL)
	q := authURL.Query()
	q.Set("client_id", config.ClientID)
	q.Set("redirect_uri", redirectURI)
	q.Set("response_type", "code")
	q.Set("state", stateArg)
	if len(config.Scopes) > 0 {
		// Strava requires comma-separated scopes; all other providers use space (OAuth 2.0 standard)
		sep := " "
		if provider == "strava" {
			sep = ","
		}
		q.Set("scope", strings.Join(config.Scopes, sep))
	}
	authURL.RawQuery = q.Encode()

	WriteJSON(w, map[string]string{"url": authURL.String()})
}

func (s *APIServer) handleOAuthCallback(w http.ResponseWriter, r *http.Request) {
	provider := chi.URLParam(r, "provider")
	config := GetOAuthConfig(provider)
	if config == nil {
		http.Redirect(w, r, webURL()+"/connections?error=unsupported_provider", http.StatusFound)
		return
	}

	code := r.URL.Query().Get("code")
	stateArg := r.URL.Query().Get("state")
	errDesc := r.URL.Query().Get("error_description")

	if errDesc != "" {
		http.Redirect(w, r, webURL()+"/connections?error="+url.QueryEscape(errDesc), http.StatusFound)
		return
	}

	if code == "" || stateArg == "" {
		http.Redirect(w, r, webURL()+"/connections?error=missing_code_or_state", http.StatusFound)
		return
	}

	// Verify state
	parts := strings.Split(stateArg, ".")
	if len(parts) != 2 {
		http.Redirect(w, r, webURL()+"/connections?error=invalid_state", http.StatusFound)
		return
	}
	stateJSON, err := base64.URLEncoding.DecodeString(parts[0])
	if err != nil {
		http.Redirect(w, r, webURL()+"/connections?error=invalid_state_encoding", http.StatusFound)
		return
	}

	h := hmac.New(sha256.New, []byte(os.Getenv("OAUTH_STATE_SECRET")))
	h.Write(stateJSON)
	expectedSig := hex.EncodeToString(h.Sum(nil))
	if !hmac.Equal([]byte(parts[1]), []byte(expectedSig)) {
		http.Redirect(w, r, webURL()+"/connections?error=invalid_state_signature", http.StatusFound)
		return
	}

	var stateData map[string]string
	if err := json.Unmarshal(stateJSON, &stateData); err != nil {
		http.Redirect(w, r, webURL()+"/connections?error=invalid_state_payload", http.StatusFound)
		return
	}

	ts, _ := strconv.ParseInt(stateData["ts"], 10, 64)
	if time.Now().Unix()-ts > 600 {
		http.Redirect(w, r, webURL()+"/connections?error=state_token_expired", http.StatusFound)
		return
	}

	userID := stateData["uid"]
	if userID == "" {
		http.Redirect(w, r, webURL()+"/connections?error=missing_uid", http.StatusFound)
		return
	}

	// Exchange code for tokens
	redirectURI := apiURL() + "/api/v2/oauth/" + provider + "/callback"
	data := url.Values{}
	data.Set("client_id", config.ClientID)
	data.Set("client_secret", config.ClientSecret)
	data.Set("code", code)
	data.Set("grant_type", "authorization_code")
	data.Set("redirect_uri", redirectURI)

	req, _ := http.NewRequest("POST", config.TokenURL, strings.NewReader(data.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("Accept", "application/json")

	if provider == "fitbit" || provider == "spotify" {
		req.SetBasicAuth(config.ClientID, config.ClientSecret)
	}

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		http.Redirect(w, r, webURL()+"/connections?error=token_exchange_failed", http.StatusFound)
		return
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		http.Redirect(w, r, webURL()+"/connections?error=token_exchange_error", http.StatusFound)
		return
	}

	var tokenResp map[string]interface{}
	if err := json.NewDecoder(resp.Body).Decode(&tokenResp); err != nil {
		http.Redirect(w, r, webURL()+"/connections?error=invalid_token_response", http.StatusFound)
		return
	}

	// Enrich with standard connection metadata
	tokenResp["enabled"] = true
	tokenResp["consent_given"] = true
	tokenResp["created_at"] = time.Now().UTC().Format(time.RFC3339)

	// Normalize expires_at or expires_in
	if exp, ok := tokenResp["expires_at"]; ok {
		switch v := exp.(type) {
		case float64:
			tokenResp["expires_at"] = time.Unix(int64(v), 0).UTC().Format(time.RFC3339)
		case int64:
			tokenResp["expires_at"] = time.Unix(v, 0).UTC().Format(time.RFC3339)
		}
	} else if expIn, ok := tokenResp["expires_in"]; ok {
		switch v := expIn.(type) {
		case float64:
			tokenResp["expires_at"] = time.Now().Add(time.Duration(v) * time.Second).UTC().Format(time.RFC3339)
		case int64:
			tokenResp["expires_at"] = time.Now().Add(time.Duration(v) * time.Second).UTC().Format(time.RFC3339)
		}
	}

	// Provider specific mapping for user IDs
	if provider == "strava" {
		if athlete, ok := tokenResp["athlete"].(map[string]interface{}); ok {
			if idFloat, ok := athlete["id"].(float64); ok {
				tokenResp["athlete_id"] = int64(idFloat)
			}
		}
	} else if provider == "fitbit" {
		if uid, ok := tokenResp["user_id"].(string); ok {
			tokenResp["fitbit_user_id"] = uid
		}
	}

	// Create protobuf Struct containing the tokens
	pbStruct, _ := structpb.NewStruct(tokenResp)
	_, err = s.userService.SetIntegration(r.Context(), &userpb.SetIntegrationRequest{
		UserId:          userID,
		Provider:        provider,
		IntegrationData: pbStruct,
	})

	if err != nil {
		s.logger.Error(r.Context(), "failed to save integration", "error", err)
		http.Redirect(w, r, webURL()+"/connections?error=failed_to_save_integration", http.StatusFound)
		return
	}

	http.Redirect(w, r, webURL()+"/connections/"+provider+"/success", http.StatusFound)
}
