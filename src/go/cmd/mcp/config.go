package main

import (
	"fmt"
	"os"
)

// Config holds the environment-derived settings for the MCP server.
//
// Auth is one of:
//   - FITGLUE_ID_TOKEN: a Firebase ID token used as-is (expires ~1h; fine for
//     one-off sessions)
//   - FITGLUE_REFRESH_TOKEN + FITGLUE_FIREBASE_API_KEY: a Firebase refresh
//     token exchanged (and re-exchanged before expiry) via the Secure Token
//     API, so long-lived sessions keep working
type Config struct {
	// BaseURL is the FitGlue deployment to talk to, e.g. https://fitglue.tech.
	// The api-client gateway is expected at {BaseURL}/api/v2.
	BaseURL string

	IDToken        string
	RefreshToken   string
	FirebaseAPIKey string
}

const defaultBaseURL = "https://fitglue.tech"

// ConfigFromEnv builds a Config from FITGLUE_* environment variables.
func ConfigFromEnv() (Config, error) {
	cfg := Config{
		BaseURL:        os.Getenv("FITGLUE_API_URL"),
		IDToken:        os.Getenv("FITGLUE_ID_TOKEN"),
		RefreshToken:   os.Getenv("FITGLUE_REFRESH_TOKEN"),
		FirebaseAPIKey: os.Getenv("FITGLUE_FIREBASE_API_KEY"),
	}
	if cfg.BaseURL == "" {
		cfg.BaseURL = defaultBaseURL
	}
	if err := cfg.Validate(); err != nil {
		return Config{}, err
	}
	return cfg, nil
}

// Validate checks that exactly enough auth material is present.
func (c Config) Validate() error {
	hasRefresh := c.RefreshToken != ""
	hasKey := c.FirebaseAPIKey != ""
	if hasRefresh != hasKey {
		return fmt.Errorf("FITGLUE_REFRESH_TOKEN and FITGLUE_FIREBASE_API_KEY must be set together")
	}
	if c.IDToken == "" && !hasRefresh {
		return fmt.Errorf("set FITGLUE_ID_TOKEN, or FITGLUE_REFRESH_TOKEN with FITGLUE_FIREBASE_API_KEY")
	}
	return nil
}

// TokenSource returns the token source implied by the config, preferring the
// auto-refreshing source when a refresh token is available.
func (c Config) TokenSource() TokenSource {
	if c.RefreshToken != "" {
		return NewRefreshTokenSource(c.FirebaseAPIKey, c.RefreshToken)
	}
	return StaticTokenSource(c.IDToken)
}
