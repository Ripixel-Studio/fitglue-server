package user

import (
	"context"
	"testing"
	"time"

	pbuser "github.com/fitglue/server/src/go/pkg/types/pb/models/user"
	"github.com/stretchr/testify/assert"
	"google.golang.org/protobuf/types/known/structpb"
)

// TestFirestoreStore_OfflineCoverageExtra covers store methods not exercised by
// TestFirestoreStore_OfflineCoverage, using the same offline (dead) Firestore
// client so each call hits its query-building + error path.
func TestFirestoreStore_OfflineCoverageExtra(t *testing.T) {
	client := setupOfflineFirestore(t)
	defer client.Close()

	store := NewFirestoreStore(client)
	ctx, cancel := context.WithTimeout(context.Background(), 1*time.Millisecond)
	defer cancel()

	t.Run("FindUserByIntegration_ApiKeyProvider", func(t *testing.T) {
		// hevy routes through the ingress_api_keys lookup path.
		_, err := store.FindUserByIntegration(ctx, "hevy", "raw-key")
		assert.Error(t, err)
	})

	t.Run("FindUserByIntegration_OAuthProviders", func(t *testing.T) {
		for _, p := range []string{"fitbit", "polar", "wahoo", "oura", "github", "spotify", "intervals", "trainingpeaks", "parkrun"} {
			_, err := store.FindUserByIntegration(ctx, p, "uid-"+p)
			assert.Error(t, err, "provider %s", p)
		}
	})

	t.Run("FindUserByIntegration_StravaNonNumeric", func(t *testing.T) {
		// Non-numeric strava UID keeps the string query value branch.
		_, err := store.FindUserByIntegration(ctx, "strava", "not-a-number")
		assert.Error(t, err)
	})

	t.Run("AddFCMToken", func(t *testing.T) {
		assert.Error(t, store.AddFCMToken(ctx, "user1", "token1"))
	})

	t.Run("DeleteCounter", func(t *testing.T) {
		assert.Error(t, store.DeleteCounter(ctx, "user1", "c1"))
	})

	t.Run("PersonalRecords", func(t *testing.T) {
		_, err := store.ListPersonalRecords(ctx, "user1")
		assert.Error(t, err)
		assert.Error(t, store.SetPersonalRecord(ctx, "user1", "rt1", &pbuser.PersonalRecord{}))
		assert.Error(t, store.SetPersonalRecord(ctx, "user1", "rt1", nil)) // nil validation
		assert.Error(t, store.DeletePersonalRecord(ctx, "user1", "rt1"))
	})

	t.Run("PluginDefaults", func(t *testing.T) {
		_, err := store.ListPluginDefaults(ctx, "user1")
		assert.Error(t, err)
		assert.Error(t, store.SetPluginDefaults(ctx, "user1", "plugin1", &structpb.Struct{}))
		assert.Error(t, store.SetPluginDefaults(ctx, "user1", "plugin1", nil)) // nil validation
		assert.Error(t, store.DeletePluginDefaults(ctx, "user1", "plugin1"))
	})
}

func TestNormalizeUserData(t *testing.T) {
	t.Run("MapsLegacyTier", func(t *testing.T) {
		assert.Equal(t, "USER_TIER_HOBBYIST", normalizeUserData(map[string]interface{}{"tier": "hobbyist"})["tier"])
		assert.Equal(t, "USER_TIER_ATHLETE", normalizeUserData(map[string]interface{}{"tier": "athlete"})["tier"])
	})

	t.Run("LeavesProtoEnumNameUntouched", func(t *testing.T) {
		assert.Equal(t, "USER_TIER_ATHLETE", normalizeUserData(map[string]interface{}{"tier": "USER_TIER_ATHLETE"})["tier"])
	})

	t.Run("NonStringTierIgnored", func(t *testing.T) {
		assert.Equal(t, 2, normalizeUserData(map[string]interface{}{"tier": 2})["tier"])
	})

	t.Run("DropsCamelCaseDuplicates", func(t *testing.T) {
		out := normalizeUserData(map[string]interface{}{
			"isAdmin":       true,
			"is_admin":      false,
			"trialEndsAt":   "x",
			"trial_ends_at": "y",
		})
		_, hasCamelAdmin := out["isAdmin"]
		assert.False(t, hasCamelAdmin, "isAdmin should be dropped")
		_, hasCamelTrial := out["trialEndsAt"]
		assert.False(t, hasCamelTrial, "trialEndsAt should be dropped")
		assert.Equal(t, false, out["is_admin"], "snake_case is_admin retained")
	})

	t.Run("KeepsCamelCaseWithoutSnakeCounterpart", func(t *testing.T) {
		out := normalizeUserData(map[string]interface{}{"isAdmin": true})
		_, ok := out["isAdmin"]
		assert.True(t, ok, "isAdmin retained when no snake counterpart")
	})
}

func TestIsApiKeyProvider(t *testing.T) {
	assert.True(t, isApiKeyProvider("hevy"))
	assert.False(t, isApiKeyProvider("strava"))
	assert.False(t, isApiKeyProvider(""))
}

func TestDefaultNotificationPreferences(t *testing.T) {
	prefs := defaultNotificationPreferences()
	assert.NotNil(t, prefs)
	for _, p := range []*pbuser.NotificationTypePreference{
		prefs.PendingInput, prefs.PipelineSuccess, prefs.PipelineFailure,
		prefs.ConnectionAction, prefs.ShowcaseRoundup, prefs.PipelineCancelled,
	} {
		assert.NotNil(t, p)
		assert.Equal(t, []pbuser.NotificationChannel{pbuser.NotificationChannel_NOTIFICATION_CHANNEL_PUSH}, p.Channels)
	}
}

func TestUserLegacyTierMap(t *testing.T) {
	assert.Equal(t, "USER_TIER_HOBBYIST", legacyTierMap["hobbyist"])
	assert.Equal(t, "USER_TIER_ATHLETE", legacyTierMap["athlete"])
	_, ok := legacyTierMap["unknown"]
	assert.False(t, ok)
}
