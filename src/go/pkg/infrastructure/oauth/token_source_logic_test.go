package oauth

import (
	"context"
	"errors"
	"testing"
	"time"

	domainuser "github.com/fitglue/server/src/go/pkg/domain/user"
	"github.com/fitglue/server/src/go/pkg/testing/mocks"
	pbuser "github.com/fitglue/server/src/go/pkg/types/pb/models/user"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/types/known/timestamppb"
)

// newSource builds a FirestoreTokenSource backed by the shared MockDatabase.
func newSource(provider string, db *mocks.MockDatabase) *FirestoreTokenSource {
	return NewFirestoreTokenSourceFromDB(db, "user-123", provider)
}

func userWith(integrations *pbuser.UserIntegrations) *domainuser.Record {
	return &domainuser.Record{Integrations: integrations}
}

func TestToken_GetUserError(t *testing.T) {
	db := &mocks.MockDatabase{
		GetUserFunc: func(_ context.Context, _ string) (*domainuser.Record, error) {
			return nil, errors.New("boom")
		},
	}
	_, err := newSource("strava", db).Token(context.Background())
	assert.ErrorContains(t, err, "failed to get user")
}

func TestToken_NoIntegrations(t *testing.T) {
	db := &mocks.MockDatabase{
		GetUserFunc: func(_ context.Context, _ string) (*domainuser.Record, error) {
			return userWith(nil), nil
		},
	}
	_, err := newSource("strava", db).Token(context.Background())
	assert.ErrorContains(t, err, "no integrations linked")
}

func TestToken_ProviderNotLinked(t *testing.T) {
	db := &mocks.MockDatabase{
		GetUserFunc: func(_ context.Context, _ string) (*domainuser.Record, error) {
			return userWith(&pbuser.UserIntegrations{}), nil
		},
	}
	_, err := newSource("strava", db).Token(context.Background())
	assert.ErrorContains(t, err, "strava not linked/enabled")
}

func TestToken_ProviderDisabled(t *testing.T) {
	db := &mocks.MockDatabase{
		GetUserFunc: func(_ context.Context, _ string) (*domainuser.Record, error) {
			return userWith(&pbuser.UserIntegrations{
				Strava: &pbuser.StravaIntegration{Enabled: false, AccessToken: "a", RefreshToken: "r"},
			}), nil
		},
	}
	_, err := newSource("strava", db).Token(context.Background())
	assert.ErrorContains(t, err, "strava not linked/enabled")
}

func TestToken_UnknownProvider(t *testing.T) {
	db := &mocks.MockDatabase{
		GetUserFunc: func(_ context.Context, _ string) (*domainuser.Record, error) {
			return userWith(&pbuser.UserIntegrations{}), nil
		},
	}
	_, err := newSource("nope", db).Token(context.Background())
	assert.ErrorContains(t, err, "unknown provider nope")
}

func TestToken_MissingAccessToken(t *testing.T) {
	db := &mocks.MockDatabase{
		GetUserFunc: func(_ context.Context, _ string) (*domainuser.Record, error) {
			return userWith(&pbuser.UserIntegrations{
				Strava: &pbuser.StravaIntegration{Enabled: true, AccessToken: "", RefreshToken: "r"},
			}), nil
		},
	}
	_, err := newSource("strava", db).Token(context.Background())
	assert.ErrorContains(t, err, "missing access token for strava")
}

func TestToken_MissingRefreshToken(t *testing.T) {
	db := &mocks.MockDatabase{
		GetUserFunc: func(_ context.Context, _ string) (*domainuser.Record, error) {
			return userWith(&pbuser.UserIntegrations{
				Strava: &pbuser.StravaIntegration{Enabled: true, AccessToken: "a", RefreshToken: ""},
			}), nil
		},
	}
	_, err := newSource("strava", db).Token(context.Background())
	assert.ErrorContains(t, err, "missing refresh token for strava")
}

func TestToken_ValidNotExpired(t *testing.T) {
	future := timestamppb.New(time.Now().Add(2 * time.Hour))
	db := &mocks.MockDatabase{
		GetUserFunc: func(_ context.Context, _ string) (*domainuser.Record, error) {
			return userWith(&pbuser.UserIntegrations{
				Strava: &pbuser.StravaIntegration{
					Enabled: true, AccessToken: "access-x", RefreshToken: "refresh-x", ExpiresAt: future,
				},
			}), nil
		},
	}
	tok, err := newSource("strava", db).Token(context.Background())
	require.NoError(t, err)
	assert.Equal(t, "access-x", tok.AccessToken)
	assert.Equal(t, "refresh-x", tok.RefreshToken)
	assert.WithinDuration(t, future.AsTime(), tok.Expiry, time.Second)
}

func TestToken_ZeroExpiryReturnsTokenWithoutRefresh(t *testing.T) {
	// Zero expiry (ExpiresAt nil) means the proactive-refresh branch is skipped.
	db := &mocks.MockDatabase{
		GetUserFunc: func(_ context.Context, _ string) (*domainuser.Record, error) {
			return userWith(&pbuser.UserIntegrations{
				Fitbit: &pbuser.FitbitIntegration{Enabled: true, AccessToken: "a", RefreshToken: "r"},
			}), nil
		},
	}
	tok, err := newSource("fitbit", db).Token(context.Background())
	require.NoError(t, err)
	assert.Equal(t, "a", tok.AccessToken)
	assert.True(t, tok.Expiry.IsZero())
}

func TestToken_GithubNonRefreshableReturnsDirectly(t *testing.T) {
	// GitHub is non-refreshable: returns access token with empty refresh token,
	// even though no refresh token is stored.
	db := &mocks.MockDatabase{
		GetUserFunc: func(_ context.Context, _ string) (*domainuser.Record, error) {
			return userWith(&pbuser.UserIntegrations{
				Github: &pbuser.GitHubIntegration{Enabled: true, AccessToken: "gh-token"},
			}), nil
		},
	}
	tok, err := newSource("github", db).Token(context.Background())
	require.NoError(t, err)
	assert.Equal(t, "gh-token", tok.AccessToken)
	assert.Empty(t, tok.RefreshToken)
}

func TestToken_ExpiredTriggersRefresh_FailsOnMissingSecret(t *testing.T) {
	past := timestamppb.New(time.Now().Add(-2 * time.Hour))
	db := &mocks.MockDatabase{
		GetUserFunc: func(_ context.Context, _ string) (*domainuser.Record, error) {
			return userWith(&pbuser.UserIntegrations{
				Strava: &pbuser.StravaIntegration{
					Enabled: true, AccessToken: "a", RefreshToken: "r", ExpiresAt: past,
				},
			}), nil
		},
	}
	// No STRAVA_CLIENT_ID set -> refreshToken() fails at getSecret.
	t.Setenv("STRAVA_CLIENT_ID", "")
	_, err := newSource("strava", db).Token(context.Background())
	assert.ErrorContains(t, err, "STRAVA_CLIENT_ID")
}

// --- ForceRefresh ---

func TestForceRefresh_NonRefreshableProvider(t *testing.T) {
	db := &mocks.MockDatabase{}
	_, err := newSource("github", db).ForceRefresh(context.Background())
	assert.ErrorContains(t, err, "cannot be refreshed")
}

func TestForceRefresh_GetUserError(t *testing.T) {
	db := &mocks.MockDatabase{
		GetUserFunc: func(_ context.Context, _ string) (*domainuser.Record, error) {
			return nil, errors.New("boom")
		},
	}
	_, err := newSource("strava", db).ForceRefresh(context.Background())
	assert.ErrorContains(t, err, "failed to get user")
}

func TestForceRefresh_NoIntegrations(t *testing.T) {
	db := &mocks.MockDatabase{
		GetUserFunc: func(_ context.Context, _ string) (*domainuser.Record, error) {
			return userWith(nil), nil
		},
	}
	_, err := newSource("strava", db).ForceRefresh(context.Background())
	assert.ErrorContains(t, err, "no integrations linked")
}

func TestForceRefresh_ProviderNotLinked(t *testing.T) {
	db := &mocks.MockDatabase{
		GetUserFunc: func(_ context.Context, _ string) (*domainuser.Record, error) {
			return userWith(&pbuser.UserIntegrations{}), nil
		},
	}
	_, err := newSource("polar", db).ForceRefresh(context.Background())
	assert.ErrorContains(t, err, "polar not linked/enabled")
}

func TestForceRefresh_UnknownProvider(t *testing.T) {
	db := &mocks.MockDatabase{
		GetUserFunc: func(_ context.Context, _ string) (*domainuser.Record, error) {
			return userWith(&pbuser.UserIntegrations{}), nil
		},
	}
	_, err := newSource("nope", db).ForceRefresh(context.Background())
	assert.ErrorContains(t, err, "unknown provider nope")
}

func TestForceRefresh_MissingRefreshToken(t *testing.T) {
	db := &mocks.MockDatabase{
		GetUserFunc: func(_ context.Context, _ string) (*domainuser.Record, error) {
			return userWith(&pbuser.UserIntegrations{
				Strava: &pbuser.StravaIntegration{Enabled: true, RefreshToken: ""},
			}), nil
		},
	}
	_, err := newSource("strava", db).ForceRefresh(context.Background())
	assert.ErrorContains(t, err, "missing refresh token for strava")
}

func TestForceRefresh_HasTokenButRefreshFailsOnSecret(t *testing.T) {
	db := &mocks.MockDatabase{
		GetUserFunc: func(_ context.Context, _ string) (*domainuser.Record, error) {
			return userWith(&pbuser.UserIntegrations{
				Strava: &pbuser.StravaIntegration{Enabled: true, RefreshToken: "r"},
			}), nil
		},
	}
	t.Setenv("STRAVA_CLIENT_ID", "")
	_, err := newSource("strava", db).ForceRefresh(context.Background())
	assert.ErrorContains(t, err, "STRAVA_CLIENT_ID")
}
