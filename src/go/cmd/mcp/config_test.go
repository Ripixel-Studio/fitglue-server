package main

import "testing"

func TestConfigValidate(t *testing.T) {
	cases := []struct {
		name    string
		cfg     Config
		wantErr bool
	}{
		{"id token only", Config{IDToken: "tok"}, false},
		{"refresh pair", Config{RefreshToken: "rt", FirebaseAPIKey: "key"}, false},
		{"both mechanisms", Config{IDToken: "tok", RefreshToken: "rt", FirebaseAPIKey: "key"}, false},
		{"nothing", Config{}, true},
		{"refresh token without api key", Config{RefreshToken: "rt"}, true},
		{"api key without refresh token", Config{FirebaseAPIKey: "key"}, true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			err := tc.cfg.Validate()
			if (err != nil) != tc.wantErr {
				t.Fatalf("Validate() error = %v, wantErr %v", err, tc.wantErr)
			}
		})
	}
}

func TestConfigFromEnv(t *testing.T) {
	t.Setenv("FITGLUE_API_URL", "")
	t.Setenv("FITGLUE_ID_TOKEN", "tok")
	t.Setenv("FITGLUE_REFRESH_TOKEN", "")
	t.Setenv("FITGLUE_FIREBASE_API_KEY", "")

	cfg, err := ConfigFromEnv()
	if err != nil {
		t.Fatalf("ConfigFromEnv() error = %v", err)
	}
	if cfg.BaseURL != defaultBaseURL {
		t.Errorf("BaseURL = %q, want default %q", cfg.BaseURL, defaultBaseURL)
	}
	if _, ok := cfg.TokenSource().(StaticTokenSource); !ok {
		t.Errorf("TokenSource() = %T, want StaticTokenSource", cfg.TokenSource())
	}

	t.Setenv("FITGLUE_API_URL", "https://dev.fitglue.tech")
	t.Setenv("FITGLUE_REFRESH_TOKEN", "rt")
	t.Setenv("FITGLUE_FIREBASE_API_KEY", "key")
	cfg, err = ConfigFromEnv()
	if err != nil {
		t.Fatalf("ConfigFromEnv() error = %v", err)
	}
	if cfg.BaseURL != "https://dev.fitglue.tech" {
		t.Errorf("BaseURL = %q, want override", cfg.BaseURL)
	}
	if _, ok := cfg.TokenSource().(*RefreshTokenSource); !ok {
		t.Errorf("TokenSource() = %T, want *RefreshTokenSource (preferred over static)", cfg.TokenSource())
	}
}
