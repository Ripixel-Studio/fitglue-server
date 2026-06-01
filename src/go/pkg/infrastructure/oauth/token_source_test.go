package oauth

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestGetSecret_ReadsFromEnv(t *testing.T) {
	src := &FirestoreTokenSource{provider: "strava"}

	t.Setenv("STRAVA_CLIENT_ID", "test-client-id")
	val, err := src.getSecret("client-id")
	require.NoError(t, err)
	assert.Equal(t, "test-client-id", val)
}

func TestGetSecret_ReadsClientSecret(t *testing.T) {
	src := &FirestoreTokenSource{provider: "fitbit"}
	t.Setenv("FITBIT_CLIENT_SECRET", "secret123")
	val, err := src.getSecret("client-secret")
	require.NoError(t, err)
	assert.Equal(t, "secret123", val)
}

func TestGetSecret_ErrorWhenMissing(t *testing.T) {
	src := &FirestoreTokenSource{provider: "polar"}
	// Make sure the env var is not set
	t.Setenv("POLAR_CLIENT_ID", "")
	_, err := src.getSecret("client-id")
	assert.ErrorContains(t, err, "POLAR_CLIENT_ID")
}

func TestGetSecret_HandlesHyphenatedProvider(t *testing.T) {
	src := &FirestoreTokenSource{provider: "training-peaks"}
	t.Setenv("TRAINING_PEAKS_CLIENT_ID", "tp-id")
	val, err := src.getSecret("client-id")
	require.NoError(t, err)
	assert.Equal(t, "tp-id", val)
}

func TestNonRefreshableProviders(t *testing.T) {
	assert.True(t, nonRefreshableProviders["github"], "github should be non-refreshable")
	assert.False(t, nonRefreshableProviders["strava"], "strava should be refreshable")
	assert.False(t, nonRefreshableProviders["fitbit"], "fitbit should be refreshable")
}
