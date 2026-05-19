package firestore

import (
	"strings"
	"time"

	"github.com/fitglue/server/src/go/pkg/domain/user"
	pbplugin "github.com/fitglue/server/src/go/pkg/types/pb/models/plugin"
	pbuser "github.com/fitglue/server/src/go/pkg/types/pb/models/user"

	pbactivity "github.com/fitglue/server/src/go/pkg/types/pb/models/activity"

	pbpipeline "github.com/fitglue/server/src/go/pkg/types/pb/models/pipeline"

	"google.golang.org/protobuf/encoding/protojson"
	"google.golang.org/protobuf/types/known/timestamppb"
)

// Helper to safely get string from map
func getString(m map[string]interface{}, key string) string {
	if v, ok := m[key]; ok {
		if s, ok := v.(string); ok {
			return s
		}
	}
	return ""
}

// Helper to convert string to pointer, returns nil for empty strings
func stringPtrOrNil(s string) *string {
	if s == "" {
		return nil
	}
	return &s
}

// Helper to safely get bool from map
func getBool(m map[string]interface{}, key string) bool {
	if v, ok := m[key]; ok {
		if b, ok := v.(bool); ok {
			return b
		}
	}
	return false
}

// Helper to safely get string slice from map (handles Firestore's []interface{})
func getStringSlice(m map[string]interface{}, key string) []string {
	if v, ok := m[key].([]interface{}); ok {
		strs := make([]string, 0, len(v))
		for _, s := range v {
			if str, ok := s.(string); ok {
				strs = append(strs, str)
			}
		}
		return strs
	}
	if v, ok := m[key].([]string); ok {
		return v
	}
	return nil
}

// Helper to safely get time from map (handles time.Time from Firestore)
func getTime(m map[string]interface{}, key string) *timestamppb.Timestamp {
	if v, ok := m[key]; ok {
		if t, ok := v.(time.Time); ok {
			return timestamppb.New(t)
		}
	}
	return nil
}

// --- UserRecord Converters ---

func UserToFirestore(u *user.Record) map[string]interface{} {
	m := map[string]interface{}{
		"user_id":    u.UserId,
		"created_at": u.CreatedAt.AsTime(),
	}

	if u.Integrations != nil {
		integrations := make(map[string]interface{})
		if u.Integrations.Hevy != nil {
			integrations["hevy"] = map[string]interface{}{
				"enabled": u.Integrations.Hevy.Enabled,
				"api_key": u.Integrations.Hevy.ApiKey,
				"user_id": u.Integrations.Hevy.UserId,
			}
		}
		if u.Integrations.Fitbit != nil {
			integrations["fitbit"] = map[string]interface{}{
				"enabled":        u.Integrations.Fitbit.Enabled,
				"access_token":   u.Integrations.Fitbit.AccessToken,
				"refresh_token":  u.Integrations.Fitbit.RefreshToken,
				"expires_at":     u.Integrations.Fitbit.ExpiresAt.AsTime(),
				"fitbit_user_id": u.Integrations.Fitbit.FitbitUserId,
			}
		}
		if u.Integrations.Strava != nil {
			integrations["strava"] = map[string]interface{}{
				"enabled":       u.Integrations.Strava.Enabled,
				"access_token":  u.Integrations.Strava.AccessToken,
				"refresh_token": u.Integrations.Strava.RefreshToken,
				"expires_at":    u.Integrations.Strava.ExpiresAt.AsTime(),
				"athlete_id":    u.Integrations.Strava.AthleteId,
			}
		}
		if u.Integrations.Parkrun != nil {
			integrations["parkrun"] = map[string]interface{}{
				"enabled":       u.Integrations.Parkrun.Enabled,
				"athlete_id":    u.Integrations.Parkrun.AthleteId,
				"country_url":   u.Integrations.Parkrun.CountryUrl,
				"consent_given": u.Integrations.Parkrun.ConsentGiven,
				"created_at":    u.Integrations.Parkrun.CreatedAt.AsTime(),
				"last_used_at":  u.Integrations.Parkrun.LastUsedAt.AsTime(),
			}
		}
		if u.Integrations.Spotify != nil {
			integrations["spotify"] = map[string]interface{}{
				"enabled":         u.Integrations.Spotify.Enabled,
				"access_token":    u.Integrations.Spotify.AccessToken,
				"refresh_token":   u.Integrations.Spotify.RefreshToken,
				"expires_at":      u.Integrations.Spotify.ExpiresAt.AsTime(),
				"spotify_user_id": u.Integrations.Spotify.SpotifyUserId,
				"created_at":      u.Integrations.Spotify.CreatedAt.AsTime(),
				"last_used_at":    u.Integrations.Spotify.LastUsedAt.AsTime(),
			}
		}
		if u.Integrations.Trainingpeaks != nil {
			integrations["trainingpeaks"] = map[string]interface{}{
				"enabled":       u.Integrations.Trainingpeaks.Enabled,
				"access_token":  u.Integrations.Trainingpeaks.AccessToken,
				"refresh_token": u.Integrations.Trainingpeaks.RefreshToken,
				"expires_at":    u.Integrations.Trainingpeaks.ExpiresAt.AsTime(),
				"athlete_id":    u.Integrations.Trainingpeaks.AthleteId,
				"created_at":    u.Integrations.Trainingpeaks.CreatedAt.AsTime(),
				"last_used_at":  u.Integrations.Trainingpeaks.LastUsedAt.AsTime(),
			}
		}
		if u.Integrations.Intervals != nil {
			integrations["intervals"] = map[string]interface{}{
				"enabled":      u.Integrations.Intervals.Enabled,
				"api_key":      u.Integrations.Intervals.ApiKey,
				"athlete_id":   u.Integrations.Intervals.AthleteId,
				"created_at":   u.Integrations.Intervals.CreatedAt.AsTime(),
				"last_used_at": u.Integrations.Intervals.LastUsedAt.AsTime(),
			}
		}
		if u.Integrations.Oura != nil {
			integrations["oura"] = map[string]interface{}{
				"enabled":       u.Integrations.Oura.Enabled,
				"access_token":  u.Integrations.Oura.AccessToken,
				"refresh_token": u.Integrations.Oura.RefreshToken,
				"expires_at":    u.Integrations.Oura.ExpiresAt.AsTime(),
				"oura_user_id":  u.Integrations.Oura.OuraUserId,
				"created_at":    u.Integrations.Oura.CreatedAt.AsTime(),
				"last_used_at":  u.Integrations.Oura.LastUsedAt.AsTime(),
			}
		}
		if u.Integrations.Google != nil {
			integrations["google"] = map[string]interface{}{
				"enabled":        u.Integrations.Google.Enabled,
				"access_token":   u.Integrations.Google.AccessToken,
				"refresh_token":  u.Integrations.Google.RefreshToken,
				"expires_at":     u.Integrations.Google.ExpiresAt.AsTime(),
				"google_user_id": u.Integrations.Google.GoogleUserId,
				"created_at":     u.Integrations.Google.CreatedAt.AsTime(),
				"last_used_at":   u.Integrations.Google.LastUsedAt.AsTime(),
			}
		}
		if u.Integrations.Polar != nil {
			integrations["polar"] = map[string]interface{}{
				"enabled":       u.Integrations.Polar.Enabled,
				"access_token":  u.Integrations.Polar.AccessToken,
				"refresh_token": u.Integrations.Polar.RefreshToken,
				"expires_at":    u.Integrations.Polar.ExpiresAt.AsTime(),
				"polar_user_id": u.Integrations.Polar.PolarUserId,
				"created_at":    u.Integrations.Polar.CreatedAt.AsTime(),
				"last_used_at":  u.Integrations.Polar.LastUsedAt.AsTime(),
			}
		}
		if u.Integrations.Wahoo != nil {
			integrations["wahoo"] = map[string]interface{}{
				"enabled":       u.Integrations.Wahoo.Enabled,
				"access_token":  u.Integrations.Wahoo.AccessToken,
				"refresh_token": u.Integrations.Wahoo.RefreshToken,
				"expires_at":    u.Integrations.Wahoo.ExpiresAt.AsTime(),
				"wahoo_user_id": u.Integrations.Wahoo.WahooUserId,
				"created_at":    u.Integrations.Wahoo.CreatedAt.AsTime(),
				"last_used_at":  u.Integrations.Wahoo.LastUsedAt.AsTime(),
			}
		}
		if u.Integrations.Github != nil {
			integrations["github"] = map[string]interface{}{
				"enabled":         u.Integrations.Github.Enabled,
				"access_token":    u.Integrations.Github.AccessToken,
				"refresh_token":   u.Integrations.Github.RefreshToken,
				"expires_at":      u.Integrations.Github.ExpiresAt.AsTime(),
				"github_user_id":  u.Integrations.Github.GithubUserId,
				"github_username": u.Integrations.Github.GithubUsername,
				"scope":           u.Integrations.Github.Scope,
				"created_at":      u.Integrations.Github.CreatedAt.AsTime(),
				"last_used_at":    u.Integrations.Github.LastUsedAt.AsTime(),
			}
		}
		if u.Integrations.AppleHealth != nil {
			integrations["apple_health"] = map[string]interface{}{
				"enabled":      u.Integrations.AppleHealth.Enabled,
				"created_at":   u.Integrations.AppleHealth.CreatedAt.AsTime(),
				"last_used_at": u.Integrations.AppleHealth.LastUsedAt.AsTime(),
			}
		}
		if u.Integrations.HealthConnect != nil {
			integrations["health_connect"] = map[string]interface{}{
				"enabled":      u.Integrations.HealthConnect.Enabled,
				"created_at":   u.Integrations.HealthConnect.CreatedAt.AsTime(),
				"last_used_at": u.Integrations.HealthConnect.LastUsedAt.AsTime(),
			}
		}
		m["integrations"] = integrations
	}

	if len(u.FcmTokens) > 0 {
		m["fcm_tokens"] = u.FcmTokens
	}

	// Pipelines moved to sub-collection users/{userId}/pipelines

	// Tier management fields
	if u.Tier == pbuser.UserTier_USER_TIER_ATHLETE {
		m["tier"] = "athlete"
	} else {
		m["tier"] = "hobbyist"
	}
	if u.TrialEndsAt != nil {
		m["trial_ends_at"] = u.TrialEndsAt.AsTime()
	}
	m["is_admin"] = u.IsAdmin
	m["sync_count_this_month"] = u.SyncCountThisMonth
	if u.Billing != nil {
		m["stripe_customer_id"] = u.Billing.StripeCustomerId
	}
	m["access_enabled"] = u.AccessEnabled
	m["prevented_sync_count"] = u.PreventedSyncCount

	return m
}

func FirestoreToUser(m map[string]interface{}) *user.Record {
	u := &user.Record{
		UserProfile: &pbuser.UserProfile{
			UserId:    getString(m, "user_id"),
			CreatedAt: getTime(m, "created_at"),
		},
		Billing: &pbuser.SubscriptionState{
			StripeCustomerId: getString(m, "stripe_customer_id"),
		},
	}

	if iMap, ok := m["integrations"].(map[string]interface{}); ok {
		u.Integrations = &pbuser.UserIntegrations{}
		if hMap, ok := iMap["hevy"].(map[string]interface{}); ok {
			u.Integrations.Hevy = &pbuser.HevyIntegration{
				Enabled: getBool(hMap, "enabled"),
				ApiKey:  getString(hMap, "api_key"),
				UserId:  getString(hMap, "user_id"),
			}
		}
		if fMap, ok := iMap["fitbit"].(map[string]interface{}); ok {
			u.Integrations.Fitbit = &pbuser.FitbitIntegration{
				Enabled:      getBool(fMap, "enabled"),
				AccessToken:  getString(fMap, "access_token"),
				RefreshToken: getString(fMap, "refresh_token"),
				ExpiresAt:    getTime(fMap, "expires_at"),
				FitbitUserId: getString(fMap, "fitbit_user_id"),
			}
		}
		if sMap, ok := iMap["strava"].(map[string]interface{}); ok {
			u.Integrations.Strava = &pbuser.StravaIntegration{
				Enabled:      getBool(sMap, "enabled"),
				AccessToken:  getString(sMap, "access_token"),
				RefreshToken: getString(sMap, "refresh_token"),
				ExpiresAt:    getTime(sMap, "expires_at"),
			}
			// Safe int64 conversion
			if v, ok := sMap["athlete_id"]; ok {
				// Firestore stores numbers as int64, float64 or int
				switch n := v.(type) {
				case int64:
					u.Integrations.Strava.AthleteId = n
				case int:
					u.Integrations.Strava.AthleteId = int64(n)
				case float64:
					u.Integrations.Strava.AthleteId = int64(n)
				}
			}
		}
		if pMap, ok := iMap["parkrun"].(map[string]interface{}); ok {
			u.Integrations.Parkrun = &pbuser.ParkrunIntegration{
				Enabled:      getBool(pMap, "enabled"),
				AthleteId:    getString(pMap, "athlete_id"),
				CountryUrl:   getString(pMap, "country_url"),
				ConsentGiven: getBool(pMap, "consent_given"),
				CreatedAt:    getTime(pMap, "created_at"),
				LastUsedAt:   getTime(pMap, "last_used_at"),
			}
		}
		if spMap, ok := iMap["spotify"].(map[string]interface{}); ok {
			u.Integrations.Spotify = &pbuser.SpotifyIntegration{
				Enabled:       getBool(spMap, "enabled"),
				AccessToken:   getString(spMap, "access_token"),
				RefreshToken:  getString(spMap, "refresh_token"),
				ExpiresAt:     getTime(spMap, "expires_at"),
				SpotifyUserId: getString(spMap, "spotify_user_id"),
				CreatedAt:     getTime(spMap, "created_at"),
				LastUsedAt:    getTime(spMap, "last_used_at"),
			}
		}
		if tpMap, ok := iMap["trainingpeaks"].(map[string]interface{}); ok {
			u.Integrations.Trainingpeaks = &pbuser.TrainingPeaksIntegration{
				Enabled:      getBool(tpMap, "enabled"),
				AccessToken:  getString(tpMap, "access_token"),
				RefreshToken: getString(tpMap, "refresh_token"),
				ExpiresAt:    getTime(tpMap, "expires_at"),
				AthleteId:    getString(tpMap, "athlete_id"),
				CreatedAt:    getTime(tpMap, "created_at"),
				LastUsedAt:   getTime(tpMap, "last_used_at"),
			}
		}
		if ivMap, ok := iMap["intervals"].(map[string]interface{}); ok {
			u.Integrations.Intervals = &pbuser.IntervalsIntegration{
				Enabled:    getBool(ivMap, "enabled"),
				ApiKey:     getString(ivMap, "api_key"),
				AthleteId:  getString(ivMap, "athlete_id"),
				CreatedAt:  getTime(ivMap, "created_at"),
				LastUsedAt: getTime(ivMap, "last_used_at"),
			}
		}
		if oMap, ok := iMap["oura"].(map[string]interface{}); ok {
			u.Integrations.Oura = &pbuser.OuraIntegration{
				Enabled:      getBool(oMap, "enabled"),
				AccessToken:  getString(oMap, "access_token"),
				RefreshToken: getString(oMap, "refresh_token"),
				ExpiresAt:    getTime(oMap, "expires_at"),
				OuraUserId:   getString(oMap, "oura_user_id"),
				CreatedAt:    getTime(oMap, "created_at"),
				LastUsedAt:   getTime(oMap, "last_used_at"),
			}
		}
		if gMap, ok := iMap["google"].(map[string]interface{}); ok {
			u.Integrations.Google = &pbuser.GoogleIntegration{
				Enabled:      getBool(gMap, "enabled"),
				AccessToken:  getString(gMap, "access_token"),
				RefreshToken: getString(gMap, "refresh_token"),
				ExpiresAt:    getTime(gMap, "expires_at"),
				GoogleUserId: getString(gMap, "google_user_id"),
				CreatedAt:    getTime(gMap, "created_at"),
				LastUsedAt:   getTime(gMap, "last_used_at"),
			}
		}
		if polMap, ok := iMap["polar"].(map[string]interface{}); ok {
			u.Integrations.Polar = &pbuser.PolarIntegration{
				Enabled:      getBool(polMap, "enabled"),
				AccessToken:  getString(polMap, "access_token"),
				RefreshToken: getString(polMap, "refresh_token"),
				ExpiresAt:    getTime(polMap, "expires_at"),
				PolarUserId:  getString(polMap, "polar_user_id"),
				CreatedAt:    getTime(polMap, "created_at"),
				LastUsedAt:   getTime(polMap, "last_used_at"),
			}
		}
		if wMap, ok := iMap["wahoo"].(map[string]interface{}); ok {
			u.Integrations.Wahoo = &pbuser.WahooIntegration{
				Enabled:      getBool(wMap, "enabled"),
				AccessToken:  getString(wMap, "access_token"),
				RefreshToken: getString(wMap, "refresh_token"),
				ExpiresAt:    getTime(wMap, "expires_at"),
				WahooUserId:  getString(wMap, "wahoo_user_id"),
				CreatedAt:    getTime(wMap, "created_at"),
				LastUsedAt:   getTime(wMap, "last_used_at"),
			}
		}
		if ghMap, ok := iMap["github"].(map[string]interface{}); ok {
			u.Integrations.Github = &pbuser.GitHubIntegration{
				Enabled:        getBool(ghMap, "enabled"),
				AccessToken:    getString(ghMap, "access_token"),
				RefreshToken:   getString(ghMap, "refresh_token"),
				ExpiresAt:      getTime(ghMap, "expires_at"),
				GithubUserId:   getString(ghMap, "github_user_id"),
				GithubUsername: getString(ghMap, "github_username"),
				Scope:          getString(ghMap, "scope"),
				CreatedAt:      getTime(ghMap, "created_at"),
				LastUsedAt:     getTime(ghMap, "last_used_at"),
			}
		}
		if ahMap, ok := iMap["apple_health"].(map[string]interface{}); ok {
			u.Integrations.AppleHealth = &pbuser.AppleHealthIntegration{
				Enabled:    getBool(ahMap, "enabled"),
				CreatedAt:  getTime(ahMap, "created_at"),
				LastUsedAt: getTime(ahMap, "last_used_at"),
			}
		}
		if hcMap, ok := iMap["health_connect"].(map[string]interface{}); ok {
			u.Integrations.HealthConnect = &pbuser.HealthConnectIntegration{
				Enabled:    getBool(hcMap, "enabled"),
				CreatedAt:  getTime(hcMap, "created_at"),
				LastUsedAt: getTime(hcMap, "last_used_at"),
			}
		}
	}

	// Tier management fields
	if v, ok := m["tier"]; ok {
		switch val := v.(type) {
		case string:
			switch val {
			case "athlete", "pro":
				u.Tier = pbuser.UserTier_USER_TIER_ATHLETE
			default:
				u.Tier = pbuser.UserTier_USER_TIER_HOBBYIST
			}
		case int64:
			// Handle legacy numeric values (1=Hobbyist, 2=Athlete)
			if val == 2 {
				u.Tier = pbuser.UserTier_USER_TIER_ATHLETE
			} else {
				u.Tier = pbuser.UserTier_USER_TIER_HOBBYIST
			}
		case int:
			if val == 2 {
				u.Tier = pbuser.UserTier_USER_TIER_ATHLETE
			} else {
				u.Tier = pbuser.UserTier_USER_TIER_HOBBYIST
			}
		case float64:
			if val == 2 {
				u.Tier = pbuser.UserTier_USER_TIER_ATHLETE
			} else {
				u.Tier = pbuser.UserTier_USER_TIER_HOBBYIST
			}
		default:
			u.Tier = pbuser.UserTier_USER_TIER_HOBBYIST
		}
	} else {
		u.Tier = pbuser.UserTier_USER_TIER_HOBBYIST
	}
	u.IsAdmin = getBool(m, "is_admin")
	u.AccessEnabled = getBool(m, "access_enabled")
	u.TrialEndsAt = getTime(m, "trial_ends_at")
	u.SyncCountResetAt = getTime(m, "sync_count_reset_at")

	if v, ok := m["sync_count_this_month"]; ok {
		switch n := v.(type) {
		case int64:
			u.SyncCountThisMonth = int32(n)
		case int:
			u.SyncCountThisMonth = int32(n)
		case float64:
			u.SyncCountThisMonth = int32(n)
		}
	}

	if v, ok := m["prevented_sync_count"]; ok {
		switch n := v.(type) {
		case int64:
			u.PreventedSyncCount = int32(n)
		case int:
			u.PreventedSyncCount = int32(n)
		case float64:
			u.PreventedSyncCount = int32(n)
		}
	}

	if tokens, ok := m["fcm_tokens"].([]interface{}); ok {
		u.FcmTokens = make([]string, len(tokens))
		for i, v := range tokens {
			if s, ok := v.(string); ok {
				u.FcmTokens[i] = s
			}
		}
	} else if tokens, ok := m["fcm_tokens"].([]string); ok {
		u.FcmTokens = tokens
	}

	// Pipelines moved to sub-collection users/{userId}/pipelines

	return u
}

// --- PipelineConfig Converters ---

func PipelineToFirestore(p *pbpipeline.PipelineConfig) map[string]interface{} {
	enrichers := make([]map[string]interface{}, len(p.Enrichers))
	for i, e := range p.Enrichers {
		enrichers[i] = map[string]interface{}{
			"provider_type": int32(e.ProviderType),
			"typed_config":  e.TypedConfig,
		}
	}

	m := map[string]interface{}{
		"id":           p.Id,
		"name":         p.Name,
		"source":       p.Source,
		"destinations": p.Destinations,
		"enrichers":    enrichers,
		"disabled":     p.Disabled,
	}

	// Source config
	if len(p.SourceConfig) > 0 {
		m["source_config"] = p.SourceConfig
	}

	// Destination configs
	if len(p.DestinationConfigs) > 0 {
		destConfigs := make(map[string]interface{})
		for k, v := range p.DestinationConfigs {
			if v != nil {
				dc := map[string]interface{}{
					"config": v.Config,
				}
				if len(v.ExcludedEnrichers) > 0 {
					dc["excluded_enrichers"] = v.ExcludedEnrichers
				}
				destConfigs[k] = dc
			}
		}
		m["destination_configs"] = destConfigs
	}

	return m
}

func FirestoreToPipeline(m map[string]interface{}) *pbpipeline.PipelineConfig {
	// Enrichers
	var enrichers []*pbpipeline.EnricherConfig
	if eList, ok := m["enrichers"].([]interface{}); ok {
		enrichers = make([]*pbpipeline.EnricherConfig, len(eList))
		for j, eRaw := range eList {
			if eMap, ok := eRaw.(map[string]interface{}); ok {
				// TypedConfig
				typedConfig := make(map[string]string)
				if cMap, ok := eMap["typed_config"].(map[string]interface{}); ok {
					for k, v := range cMap {
						if s, ok := v.(string); ok {
							typedConfig[k] = s
						}
					}
				}

				ptype := pbplugin.EnricherProviderType_ENRICHER_PROVIDER_UNSPECIFIED
				if v, ok := eMap["provider_type"]; ok {
					switch n := v.(type) {
					case int64:
						ptype = pbplugin.EnricherProviderType(n)
					case int:
						ptype = pbplugin.EnricherProviderType(n)
					case float64:
						ptype = pbplugin.EnricherProviderType(int32(n))
					case string:
						if val, ok := pbplugin.EnricherProviderType_value[n]; ok {
							ptype = pbplugin.EnricherProviderType(val)
						}
					}
				}

				enrichers[j] = &pbpipeline.EnricherConfig{
					ProviderType: ptype,
					TypedConfig:  typedConfig,
				}
			}
		}
	}

	// Destinations - handle both legacy strings and new enum ints
	var dests []pbplugin.DestinationType
	if dList, ok := m["destinations"].([]interface{}); ok {
		for _, d := range dList {
			switch val := d.(type) {
			case int64:
				dests = append(dests, pbplugin.DestinationType(val))
			case int:
				dests = append(dests, pbplugin.DestinationType(val))
			case float64:
				dests = append(dests, pbplugin.DestinationType(int32(val)))
			case string:
				// String support: try proto enum name first, then short aliases
				upper := strings.ToUpper(val)
				if enumVal, ok := pbplugin.DestinationType_value[upper]; ok {
					dests = append(dests, pbplugin.DestinationType(enumVal))
				} else {
					prefixed := "DESTINATION_" + upper
					if enumVal, ok := pbplugin.DestinationType_value[prefixed]; ok {
						dests = append(dests, pbplugin.DestinationType(enumVal))
					}
				}
			}
		}
	}

	// Source config
	sourceConfig := make(map[string]string)
	if scMap, ok := m["source_config"].(map[string]interface{}); ok {
		for k, v := range scMap {
			if s, ok := v.(string); ok {
				sourceConfig[k] = s
			}
		}
	}

	// Destination configs
	destConfigs := make(map[string]*pbpipeline.DestinationConfig)
	if dcMap, ok := m["destination_configs"].(map[string]interface{}); ok {
		for destId, dcRaw := range dcMap {
			if dcObj, ok := dcRaw.(map[string]interface{}); ok {
				cfg := make(map[string]string)
				if cMap, ok := dcObj["config"].(map[string]interface{}); ok {
					for k, v := range cMap {
						if s, ok := v.(string); ok {
							cfg[k] = s
						}
					}
				}
				destConfigs[destId] = &pbpipeline.DestinationConfig{
					Config:            cfg,
					ExcludedEnrichers: getStringSlice(dcObj, "excluded_enrichers"),
				}
			}
		}
	}

	return &pbpipeline.PipelineConfig{
		Id:                 getString(m, "id"),
		Name:               getString(m, "name"),
		Source:             getString(m, "source"),
		Enrichers:          enrichers,
		Destinations:       dests,
		Disabled:           getBool(m, "disabled"),
		SourceConfig:       sourceConfig,
		DestinationConfigs: destConfigs,
	}
}

// --- Execution Record ---

func ExecutionToFirestore(e *pbpipeline.ExecutionRecord) map[string]interface{} {
	m := map[string]interface{}{
		"execution_id":          e.ExecutionId,
		"service":               e.Service,
		"status":                int32(e.Status), // Store enum as int or string? Protocol is int usually, logger used String()
		"timestamp":             e.Timestamp.AsTime(),
		"user_id":               e.UserId,
		"test_run_id":           e.TestRunId,
		"trigger_type":          e.TriggerType,
		"start_time":            e.StartTime.AsTime(),
		"end_time":              e.EndTime.AsTime(),
		"error_message":         e.ErrorMessage,
		"inputs_json":           e.InputsJson,
		"outputs_json":          e.OutputsJson,
		"pipeline_execution_id": e.PipelineExecutionId,
	}
	return m
}

func FirestoreToExecution(m map[string]interface{}) *pbpipeline.ExecutionRecord {
	e := &pbpipeline.ExecutionRecord{
		ExecutionId:         getString(m, "execution_id"),
		Service:             getString(m, "service"),
		Timestamp:           getTime(m, "timestamp"),
		TriggerType:         getString(m, "trigger_type"), // Required field, not a pointer
		UserId:              stringPtrOrNil(getString(m, "user_id")),
		TestRunId:           stringPtrOrNil(getString(m, "test_run_id")),
		StartTime:           getTime(m, "start_time"),
		EndTime:             getTime(m, "end_time"),
		ErrorMessage:        stringPtrOrNil(getString(m, "error_message")),
		InputsJson:          stringPtrOrNil(getString(m, "inputs_json")),
		OutputsJson:         stringPtrOrNil(getString(m, "outputs_json")),
		PipelineExecutionId: stringPtrOrNil(getString(m, "pipeline_execution_id")),
	}

	if v, ok := m["status"]; ok {
		// Handle int or string legacy
		switch val := v.(type) {
		case int64:
			e.Status = pbpipeline.ExecutionStatus(val)
		case int:
			e.Status = pbpipeline.ExecutionStatus(int32(val))
		case string:
			// Use proto-generated map for all status values
			if enumVal, ok := pbpipeline.ExecutionStatus_value[val]; ok {
				e.Status = pbpipeline.ExecutionStatus(enumVal)
			} else {
				e.Status = pbpipeline.ExecutionStatus_STATUS_UNSPECIFIED
			}
		}
	}

	return e
}

// --- Counter Converters ---

func CounterToFirestore(c *pbuser.Counter) map[string]interface{} {
	return map[string]interface{}{
		"id":           c.Id,
		"count":        c.Count,
		"last_updated": c.LastUpdated.AsTime(),
	}
}

func FirestoreToCounter(m map[string]interface{}) *pbuser.Counter {
	c := &pbuser.Counter{
		Id:          getString(m, "id"),
		LastUpdated: getTime(m, "last_updated"),
	}
	// Handle number types
	if v, ok := m["count"]; ok {
		switch n := v.(type) {
		case int64:
			c.Count = n
		case int:
			c.Count = int64(n)
		case float64:
			c.Count = int64(n)
		}
	}
	return c
}

// --- PersonalRecord Converters ---

func PersonalRecordToFirestore(r *pbuser.PersonalRecord) map[string]interface{} {
	m := map[string]interface{}{
		"record_type":   r.RecordType,
		"value":         r.Value,
		"unit":          r.Unit,
		"activity_id":   r.ActivityId,
		"achieved_at":   r.AchievedAt.AsTime(),
		"activity_type": int32(r.ActivityType),
	}
	if r.PreviousValue != nil {
		m["previous_value"] = *r.PreviousValue
	}
	if r.Improvement != nil {
		m["improvement"] = *r.Improvement
	}
	return m
}

func FirestoreToPersonalRecord(m map[string]interface{}) *pbuser.PersonalRecord {
	r := &pbuser.PersonalRecord{
		RecordType: getString(m, "record_type"),
		Unit:       getString(m, "unit"),
		ActivityId: getString(m, "activity_id"),
		AchievedAt: getTime(m, "achieved_at"),
	}

	// Value
	if v, ok := m["value"]; ok {
		switch n := v.(type) {
		case float64:
			r.Value = n
		case int64:
			r.Value = float64(n)
		case int:
			r.Value = float64(n)
		}
	}

	// Activity type
	if v, ok := m["activity_type"]; ok {
		switch val := v.(type) {
		case int64:
			r.ActivityType = pbactivity.ActivityType(val)
		case int:
			r.ActivityType = pbactivity.ActivityType(int32(val))
		case float64:
			r.ActivityType = pbactivity.ActivityType(int32(val))
		case string:
			if enumVal, ok := pbactivity.ActivityType_value[val]; ok {
				r.ActivityType = pbactivity.ActivityType(enumVal)
			}
		}
	}

	// Optional previous_value
	if v, ok := m["previous_value"]; ok {
		switch n := v.(type) {
		case float64:
			r.PreviousValue = &n
		case int64:
			f := float64(n)
			r.PreviousValue = &f
		case int:
			f := float64(n)
			r.PreviousValue = &f
		}
	}

	// Optional improvement
	if v, ok := m["improvement"]; ok {
		switch n := v.(type) {
		case float64:
			r.Improvement = &n
		case int64:
			f := float64(n)
			r.Improvement = &f
		case int:
			f := float64(n)
			r.Improvement = &f
		}
	}

	return r
}

// --- PendingInput Converters ---

func PendingInputToFirestore(p *pbpipeline.PendingInput) map[string]interface{} {
	m := map[string]interface{}{
		"activity_id":                  p.ActivityId,
		"user_id":                      p.UserId,
		"status":                       int32(p.Status),
		"required_fields":              p.RequiredFields,
		"input_data":                   p.InputData,
		"created_at":                   p.CreatedAt.AsTime(),
		"updated_at":                   p.UpdatedAt.AsTime(),
		"completed_at":                 p.CompletedAt.AsTime(),
		"auto_populated":               p.AutoPopulated,
		"continued_without_resolution": p.ContinuedWithoutResolution,
		"enricher_provider_id":         p.EnricherProviderId,
		"linked_activity_id":           p.LinkedActivityId,
		"pipeline_id":                  p.PipelineId,
	}

	if p.AutoDeadline != nil {
		m["auto_deadline"] = p.AutoDeadline.AsTime()
	}

	if p.OriginalPayloadUri != "" {
		m["original_payload_uri"] = p.OriginalPayloadUri
	}

	// Provider metadata
	if len(p.ProviderMetadata) > 0 {
		m["provider_metadata"] = p.ProviderMetadata
	}
	return m
}

func FirestoreToPendingInput(m map[string]interface{}) *pbpipeline.PendingInput {
	p := &pbpipeline.PendingInput{
		ActivityId: getString(m, "activity_id"),
		UserId:     getString(m, "user_id"),
		RequiredFields: func() []string {
			if v, ok := m["required_fields"].([]string); ok {
				return v
			}
			// Handle []interface{} from Firestore
			if v, ok := m["required_fields"].([]interface{}); ok {
				strs := make([]string, len(v))
				for i, s := range v {
					if str, ok := s.(string); ok {
						strs[i] = str
					}
				}
				return strs
			}
			return nil
		}(),
		InputData: func() map[string]string {
			if v, ok := m["input_data"].(map[string]interface{}); ok {
				out := make(map[string]string)
				for k, val := range v {
					if str, ok := val.(string); ok {
						out[k] = str
					}
				}
				return out
			}
			return nil
		}(),
		CreatedAt:                  getTime(m, "created_at"),
		UpdatedAt:                  getTime(m, "updated_at"),
		CompletedAt:                getTime(m, "completed_at"),
		AutoPopulated:              getBool(m, "auto_populated"),
		ContinuedWithoutResolution: getBool(m, "continued_without_resolution"),
		EnricherProviderId:         getString(m, "enricher_provider_id"),
		AutoDeadline:               getTime(m, "auto_deadline"),
		LinkedActivityId:           getString(m, "linked_activity_id"),
		PipelineId:                 getString(m, "pipeline_id"),
		OriginalPayloadUri:         getString(m, "original_payload_uri"),
	}

	if v, ok := m["status"]; ok {
		switch n := v.(type) {
		case int64:
			p.Status = pbpipeline.PendingInput_Status(n)
		case int:
			p.Status = pbpipeline.PendingInput_Status(int32(n))
		case string:
			if enumVal, ok := pbpipeline.PendingInput_Status_value[n]; ok {
				p.Status = pbpipeline.PendingInput_Status(enumVal)
			}
		}
	}

	// Note: original_payload is now stored in GCS via original_payload_uri

	// Provider metadata
	if v, ok := m["provider_metadata"].(map[string]interface{}); ok {
		p.ProviderMetadata = make(map[string]string)
		for k, val := range v {
			if str, ok := val.(string); ok {
				p.ProviderMetadata[k] = str
			}
		}
	}
	return p
}

// --- ShowcasedActivity Converters ---

func ShowcasedActivityToFirestore(s *pbactivity.ShowcasedActivity) map[string]interface{} {
	m := map[string]interface{}{
		"showcase_id":         s.ShowcaseId,
		"activity_id":         s.ActivityId,
		"user_id":             s.UserId,
		"title":               s.Title,
		"description":         s.Description,
		"activity_type":       int32(s.ActivityType),
		"source":              int32(s.Source),
		"applied_enrichments": s.AppliedEnrichments,
		"tags":                s.Tags,
		"fit_file_uri":        s.FitFileUri,
		"owner_display_name":  s.OwnerDisplayName,
		"activity_data_uri":   s.ActivityDataUri,
		"photo_urls":          s.PhotoUrls,
	}

	if s.StartTime != nil {
		m["start_time"] = s.StartTime.AsTime()
	} else {
		// Ensure field exists for indexing even if null/zero
		m["start_time"] = nil
	}
	if s.CreatedAt != nil {
		m["created_at"] = s.CreatedAt.AsTime()
	}
	if s.ExpiresAt != nil {
		m["expires_at"] = s.ExpiresAt.AsTime()
	} else {
		m["expires_at"] = nil
	}
	if s.PipelineExecutionId != nil {
		m["pipeline_execution_id"] = *s.PipelineExecutionId
	} else {
		m["pipeline_execution_id"] = ""
	}

	return m
}

func FirestoreToShowcasedActivity(m map[string]interface{}) *pbactivity.ShowcasedActivity {
	s := &pbactivity.ShowcasedActivity{
		ShowcaseId:          getString(m, "showcase_id"),
		ActivityId:          getString(m, "activity_id"),
		UserId:              getString(m, "user_id"),
		Title:               getString(m, "title"),
		Description:         getString(m, "description"),
		FitFileUri:          getString(m, "fit_file_uri"),
		ActivityDataUri:     getString(m, "activity_data_uri"), // GCS URI for activity JSON
		StartTime:           getTime(m, "start_time"),
		CreatedAt:           getTime(m, "created_at"),
		ExpiresAt:           getTime(m, "expires_at"),
		PipelineExecutionId: stringPtrOrNil(getString(m, "pipeline_execution_id")),
		OwnerDisplayName:    getString(m, "owner_display_name"),
	}

	// ActivityType
	if v, ok := m["activity_type"]; ok {
		switch val := v.(type) {
		case int64:
			s.ActivityType = pbactivity.ActivityType(val)
		case int:
			s.ActivityType = pbactivity.ActivityType(int32(val))
		case float64:
			s.ActivityType = pbactivity.ActivityType(int32(val))
		case string:
			if enumVal, ok := pbactivity.ActivityType_value[val]; ok {
				s.ActivityType = pbactivity.ActivityType(enumVal)
			}
		}
	}

	// Source
	if v, ok := m["source"]; ok {
		switch val := v.(type) {
		case int64:
			s.Source = pbactivity.ActivitySource(val)
		case int:
			s.Source = pbactivity.ActivitySource(int32(val))
		case float64:
			s.Source = pbactivity.ActivitySource(int32(val))
		case string:
			if enumVal, ok := pbactivity.ActivitySource_value[val]; ok {
				s.Source = pbactivity.ActivitySource(enumVal)
			}
		}
	}

	// Applied enrichments
	if v, ok := m["applied_enrichments"].([]interface{}); ok {
		s.AppliedEnrichments = make([]string, len(v))
		for i, val := range v {
			if str, ok := val.(string); ok {
				s.AppliedEnrichments[i] = str
			}
		}
	}

	// Tags
	if v, ok := m["tags"].([]interface{}); ok {
		s.Tags = make([]string, len(v))
		for i, val := range v {
			if str, ok := val.(string); ok {
				s.Tags[i] = str
			}
		}
	}

	// Photo URLs
	if v, ok := m["photo_urls"].([]interface{}); ok {
		s.PhotoUrls = make([]string, len(v))
		for i, val := range v {
			if str, ok := val.(string); ok {
				s.PhotoUrls[i] = str
			}
		}
	}

	// enrichment_metadata (field 12) was removed from ShowcasedActivity — skipped on read

	// Activity data - deserialize from JSON
	if v, ok := m["activity_data"]; ok {
		var jsonStr string
		switch val := v.(type) {
		case string:
			jsonStr = val
		case []byte:
			jsonStr = string(val)
		}
		if jsonStr != "" {
			var data pbactivity.StandardizedActivity
			if err := protojson.Unmarshal([]byte(jsonStr), &data); err == nil {
				s.ActivityData = &data
			}
		}
	}

	return s
}

// --- ShowcaseProfile Converters ---

func ShowcaseProfileEntryToFirestore(e *pbactivity.ShowcaseProfileEntry) map[string]interface{} {
	m := map[string]interface{}{
		"showcase_id":       e.ShowcaseId,
		"title":             e.Title,
		"activity_type":     int32(e.ActivityType),
		"source":            int32(e.Source),
		"route_thumbnail_url": e.RouteThumbnailUrl,
		"distance_meters":   e.DistanceMeters,
		"duration_seconds":  e.DurationSeconds,
		"total_sets":        e.TotalSets,
		"total_reps":        e.TotalReps,
		"total_weight_kg":   e.TotalWeightKg,
		"booster_count":     e.BoosterCount,
		"destination_count": e.DestinationCount,
	}
	if e.StartTime != nil {
		m["start_time"] = e.StartTime.AsTime()
	} else {
		m["start_time"] = nil
	}
	if e.AvgHeartRate != nil {
		m["avg_heart_rate"] = *e.AvgHeartRate
	}
	if e.CaloriesKcal != nil {
		m["calories_kcal"] = *e.CaloriesKcal
	}
	if e.Sparkline != nil {
		m["sparkline"] = map[string]interface{}{
			"metric": e.Sparkline.Metric,
			"values": e.Sparkline.Values,
		}
	}
	return m
}

func FirestoreToShowcaseProfileEntry(m map[string]interface{}) *pbactivity.ShowcaseProfileEntry {
	e := &pbactivity.ShowcaseProfileEntry{
		ShowcaseId:        getString(m, "showcase_id"),
		Title:             getString(m, "title"),
		RouteThumbnailUrl: getString(m, "route_thumbnail_url"),
		StartTime:         getTime(m, "start_time"),
	}

	// ActivityType
	if v, ok := m["activity_type"]; ok {
		switch val := v.(type) {
		case int64:
			e.ActivityType = pbactivity.ActivityType(val)
		case float64:
			e.ActivityType = pbactivity.ActivityType(int32(val))
		case string:
			if enumVal, ok := pbactivity.ActivityType_value[val]; ok {
				e.ActivityType = pbactivity.ActivityType(enumVal)
			}
		}
	}

	// Source
	if v, ok := m["source"]; ok {
		switch val := v.(type) {
		case int64:
			e.Source = pbactivity.ActivitySource(val)
		case float64:
			e.Source = pbactivity.ActivitySource(int32(val))
		case string:
			if enumVal, ok := pbactivity.ActivitySource_value[val]; ok {
				e.Source = pbactivity.ActivitySource(enumVal)
			}
		}
	}

	// DistanceMeters
	if v, ok := m["distance_meters"]; ok {
		switch n := v.(type) {
		case float64:
			e.DistanceMeters = n
		case int64:
			e.DistanceMeters = float64(n)
		}
	}

	// DurationSeconds
	if v, ok := m["duration_seconds"]; ok {
		switch n := v.(type) {
		case float64:
			e.DurationSeconds = n
		case int64:
			e.DurationSeconds = float64(n)
		}
	}

	// TotalSets
	if v, ok := m["total_sets"]; ok {
		switch n := v.(type) {
		case int64:
			e.TotalSets = int32(n)
		case float64:
			e.TotalSets = int32(n)
		}
	}

	// TotalReps
	if v, ok := m["total_reps"]; ok {
		switch n := v.(type) {
		case int64:
			e.TotalReps = int32(n)
		case float64:
			e.TotalReps = int32(n)
		}
	}

	// TotalWeightKg
	if v, ok := m["total_weight_kg"]; ok {
		switch n := v.(type) {
		case float64:
			e.TotalWeightKg = n
		case int64:
			e.TotalWeightKg = float64(n)
		}
	}

	// BoosterCount
	if v, ok := m["booster_count"]; ok {
		switch n := v.(type) {
		case int64:
			e.BoosterCount = int32(n)
		case float64:
			e.BoosterCount = int32(n)
		}
	}

	// DestinationCount
	if v, ok := m["destination_count"]; ok {
		switch n := v.(type) {
		case int64:
			e.DestinationCount = int32(n)
		case float64:
			e.DestinationCount = int32(n)
		}
	}

	// AvgHeartRate (optional)
	if v, ok := m["avg_heart_rate"]; ok {
		switch n := v.(type) {
		case int64:
			hr := int32(n)
			e.AvgHeartRate = &hr
		case float64:
			hr := int32(n)
			e.AvgHeartRate = &hr
		}
	}

	// CaloriesKcal (optional)
	if v, ok := m["calories_kcal"]; ok {
		switch n := v.(type) {
		case int64:
			kcal := int32(n)
			e.CaloriesKcal = &kcal
		case float64:
			kcal := int32(n)
			e.CaloriesKcal = &kcal
		}
	}

	// Sparkline (optional)
	if v, ok := m["sparkline"]; ok {
		if sparkMap, ok := v.(map[string]interface{}); ok {
			sparkline := &pbactivity.EntrySparkline{
				Metric: getString(sparkMap, "metric"),
			}
			if vals, ok := sparkMap["values"]; ok {
				switch vs := vals.(type) {
				case []interface{}:
					for _, fv := range vs {
						if f, ok := fv.(float64); ok {
							sparkline.Values = append(sparkline.Values, f)
						}
					}
				case []float64:
					sparkline.Values = vs
				}
			}
			e.Sparkline = sparkline
		}
	}

	return e
}

func ShowcaseProfileToFirestore(p *pbactivity.ShowcaseProfile) map[string]interface{} {
	entries := make([]map[string]interface{}, len(p.Entries))
	for i, e := range p.Entries {
		entries[i] = ShowcaseProfileEntryToFirestore(e)
	}

	m := map[string]interface{}{
		"slug":                   p.Slug,
		"user_id":                p.UserId,
		"display_name":           p.DisplayName,
		"entries":                entries,
		"total_activities":       p.TotalActivities,
		"total_distance_meters":  p.TotalDistanceMeters,
		"total_duration_seconds": p.TotalDurationSeconds,
		"total_sets":             p.TotalSets,
		"total_reps":             p.TotalReps,
		"total_weight_kg":        p.TotalWeightKg,
		"subtitle":               p.Subtitle,
		"bio":                    p.Bio,
		"profile_picture_url":    p.ProfilePictureUrl,
		"visible":                p.Visible,
	}

	if p.LatestActivityAt != nil {
		m["latest_activity_at"] = p.LatestActivityAt.AsTime()
	} else {
		m["latest_activity_at"] = nil
	}
	if p.CreatedAt != nil {
		m["created_at"] = p.CreatedAt.AsTime()
	} else {
		m["created_at"] = nil
	}
	if p.UpdatedAt != nil {
		m["updated_at"] = p.UpdatedAt.AsTime()
	} else {
		m["updated_at"] = nil
	}

	if p.Theme != nil {
		m["theme"] = map[string]interface{}{
			"theme_id":            p.Theme.ThemeId,
			"custom_accent_color": p.Theme.CustomAccentColor,
			"animation_id":        p.Theme.AnimationId,
			"card_style":          p.Theme.CardStyle,
		}
	} else {
		m["theme"] = nil
	}

	m["default_destination"] = p.DefaultDestination

	return m
}

func FirestoreToShowcaseProfile(m map[string]interface{}) *pbactivity.ShowcaseProfile {
	p := &pbactivity.ShowcaseProfile{
		Slug:              getString(m, "slug"),
		UserId:            getString(m, "user_id"),
		DisplayName:       getString(m, "display_name"),
		LatestActivityAt:  getTime(m, "latest_activity_at"),
		CreatedAt:         getTime(m, "created_at"),
		UpdatedAt:         getTime(m, "updated_at"),
		Subtitle:          getString(m, "subtitle"),
		Bio:               getString(m, "bio"),
		ProfilePictureUrl: getString(m, "profile_picture_url"),
	}

	// Visible defaults to true for backward compat
	if v, ok := m["visible"]; ok {
		if b, ok := v.(bool); ok {
			p.Visible = b
		}
	} else {
		p.Visible = true
	}

	// TotalActivities
	if v, ok := m["total_activities"]; ok {
		switch n := v.(type) {
		case int64:
			p.TotalActivities = int32(n)
		case float64:
			p.TotalActivities = int32(n)
		}
	}

	// TotalDistanceMeters
	if v, ok := m["total_distance_meters"]; ok {
		switch n := v.(type) {
		case float64:
			p.TotalDistanceMeters = n
		case int64:
			p.TotalDistanceMeters = float64(n)
		}
	}

	// TotalDurationSeconds
	if v, ok := m["total_duration_seconds"]; ok {
		switch n := v.(type) {
		case float64:
			p.TotalDurationSeconds = n
		case int64:
			p.TotalDurationSeconds = float64(n)
		}
	}

	// Entries
	if eList, ok := m["entries"].([]interface{}); ok {
		p.Entries = make([]*pbactivity.ShowcaseProfileEntry, len(eList))
		for i, eRaw := range eList {
			if eMap, ok := eRaw.(map[string]interface{}); ok {
				p.Entries[i] = FirestoreToShowcaseProfileEntry(eMap)
			}
		}
	}

	// Theme
	if tRaw, ok := m["theme"].(map[string]interface{}); ok {
		p.Theme = &pbactivity.ShowcaseTheme{
			ThemeId:           getString(tRaw, "theme_id"),
			CustomAccentColor: getString(tRaw, "custom_accent_color"),
			AnimationId:       getString(tRaw, "animation_id"),
			CardStyle:         getString(tRaw, "card_style"),
		}
	}

	// DefaultDestination
	if v, ok := m["default_destination"]; ok {
		if b, ok := v.(bool); ok {
			p.DefaultDestination = b
		}
	}

	return p
}

// --- PluginDefault Converters ---

func PluginDefaultToFirestore(p *pbpipeline.PluginDefault) map[string]interface{} {
	m := map[string]interface{}{
		"plugin_id": p.PluginId,
		"config":    p.Config,
	}
	if p.CreatedAt != 0 {
		m["created_at"] = p.CreatedAt
	}
	if p.UpdatedAt != 0 {
		m["updated_at"] = p.UpdatedAt
	}
	return m
}

func FirestoreToPluginDefault(m map[string]interface{}) *pbpipeline.PluginDefault {
	p := &pbpipeline.PluginDefault{
		PluginId: getString(m, "plugin_id"),
	}
	if cfg, ok := m["config"].(map[string]interface{}); ok {
		p.Config = make(map[string]string, len(cfg))
		for k, v := range cfg {
			if s, ok := v.(string); ok {
				p.Config[k] = s
			}
		}
	}
	if v, ok := m["created_at"]; ok {
		switch n := v.(type) {
		case int64:
			p.CreatedAt = n
		case float64:
			p.CreatedAt = int64(n)
		case int:
			p.CreatedAt = int64(n)
		}
	}
	if v, ok := m["updated_at"]; ok {
		switch n := v.(type) {
		case int64:
			p.UpdatedAt = n
		case float64:
			p.UpdatedAt = int64(n)
		case int:
			p.UpdatedAt = int64(n)
		}
	}
	return p
}

// --- UploadedActivityRecord Converters (Loop Prevention) ---

func UploadedActivityToFirestore(r *pbactivity.UploadedActivityRecord) map[string]interface{} {
	m := map[string]interface{}{
		"id":             r.Id,
		"user_id":        r.UserId,
		"source":         int32(r.Source),
		"external_id":    r.ExternalId,
		"destination":    int32(r.Destination),
		"destination_id": r.DestinationId,
	}

	if r.StartTime != nil {
		m["start_time"] = r.StartTime.AsTime()
	}
	if r.UploadedAt != nil {
		m["uploaded_at"] = r.UploadedAt.AsTime()
	}

	return m
}

func FirestoreToUploadedActivity(m map[string]interface{}) *pbactivity.UploadedActivityRecord {
	r := &pbactivity.UploadedActivityRecord{
		Id:            getString(m, "id"),
		UserId:        getString(m, "user_id"),
		ExternalId:    getString(m, "external_id"),
		DestinationId: getString(m, "destination_id"),
		StartTime:     getTime(m, "start_time"),
		UploadedAt:    getTime(m, "uploaded_at"),
	}

	// Source
	if v, ok := m["source"]; ok {
		switch val := v.(type) {
		case int64:
			r.Source = pbactivity.ActivitySource(val)
		case int:
			r.Source = pbactivity.ActivitySource(int32(val))
		case float64:
			r.Source = pbactivity.ActivitySource(int32(val))
		case string:
			if enumVal, ok := pbactivity.ActivitySource_value[val]; ok {
				r.Source = pbactivity.ActivitySource(enumVal)
			}
		}
	}

	// Destination
	if v, ok := m["destination"]; ok {
		switch val := v.(type) {
		case int64:
			r.Destination = pbplugin.DestinationType(val)
		case int:
			r.Destination = pbplugin.DestinationType(int32(val))
		case float64:
			r.Destination = pbplugin.DestinationType(int32(val))
		case string:
			upper := strings.ToUpper(val)
			if enumVal, ok := pbplugin.DestinationType_value[upper]; ok {
				r.Destination = pbplugin.DestinationType(enumVal)
			} else {
				prefixed := "DESTINATION_" + upper
				if enumVal, ok := pbplugin.DestinationType_value[prefixed]; ok {
					r.Destination = pbplugin.DestinationType(enumVal)
				}
			}
		}
	}

	return r
}

// --- PipelineRun Converters ---

func PipelineRunToFirestore(p *pbpipeline.PipelineRun) map[string]interface{} {
	m := map[string]interface{}{
		"id":                 p.Id,
		"pipeline_id":        p.PipelineId,
		"activity_id":        p.ActivityId,
		"source":             p.Source,
		"source_activity_id": p.SourceActivityId,
		"title":              p.Title,
		"description":        p.Description,
		"type":               int32(p.Type),
		"status":             int32(p.Status),
	}

	if p.StartTime != nil {
		m["start_time"] = p.StartTime.AsTime()
	}
	if p.CreatedAt != nil {
		m["created_at"] = p.CreatedAt.AsTime()
	}
	if p.UpdatedAt != nil {
		m["updated_at"] = p.UpdatedAt.AsTime()
	}
	if p.StatusMessage != nil {
		m["status_message"] = *p.StatusMessage
	}
	if p.PendingInputId != nil {
		m["pending_input_id"] = *p.PendingInputId
	}
	if p.OriginalPayloadUri != "" {
		m["original_payload_uri"] = p.OriginalPayloadUri
	}

	// Serialize boosters
	if len(p.Boosters) > 0 {
		boosters := make([]map[string]interface{}, len(p.Boosters))
		for i, b := range p.Boosters {
			booster := map[string]interface{}{
				"provider_name": b.ProviderName,
				"status":        b.Status,
				"duration_ms":   b.DurationMs,
				"metadata":      b.Metadata,
			}
			if b.Error != nil {
				booster["error"] = *b.Error
			}
			boosters[i] = booster
		}
		m["boosters"] = boosters
	}

	// Serialize destinations
	if len(p.Destinations) > 0 {
		dests := make([]map[string]interface{}, len(p.Destinations))
		for i, d := range p.Destinations {
			dest := map[string]interface{}{
				"destination": int32(d.Destination),
				"status":      int32(d.Status),
			}
			if d.ExternalId != nil {
				dest["external_id"] = *d.ExternalId
			}
			if d.Error != nil {
				dest["error"] = *d.Error
			}
			if d.CompletedAt != nil {
				dest["completed_at"] = d.CompletedAt.AsTime()
			}
			dests[i] = dest
		}
		m["destinations"] = dests
	}

	// Note: enriched_event is now stored in GCS via enriched_event_uri
	if p.EnrichedEventUri != "" {
		m["enriched_event_uri"] = p.EnrichedEventUri
	}
	// Note: original_payload is now stored in GCS via original_payload_uri

	return m
}

func FirestoreToPipelineRun(m map[string]interface{}) *pbpipeline.PipelineRun {
	p := &pbpipeline.PipelineRun{
		Id:                 getString(m, "id"),
		PipelineId:         getString(m, "pipeline_id"),
		ActivityId:         getString(m, "activity_id"),
		Source:             getString(m, "source"),
		SourceActivityId:   getString(m, "source_activity_id"),
		Title:              getString(m, "title"),
		Description:        getString(m, "description"),
		StartTime:          getTime(m, "start_time"),
		CreatedAt:          getTime(m, "created_at"),
		UpdatedAt:          getTime(m, "updated_at"),
		StatusMessage:      stringPtrOrNil(getString(m, "status_message")),
		PendingInputId:     stringPtrOrNil(getString(m, "pending_input_id")),
		OriginalPayloadUri: getString(m, "original_payload_uri"),
	}

	// Type
	if v, ok := m["type"]; ok {
		switch val := v.(type) {
		case int64:
			p.Type = pbactivity.ActivityType(val)
		case int:
			p.Type = pbactivity.ActivityType(int32(val))
		case float64:
			p.Type = pbactivity.ActivityType(int32(val))
		case string:
			if enumVal, ok := pbactivity.ActivityType_value[val]; ok {
				p.Type = pbactivity.ActivityType(enumVal)
			}
		}
	}

	// Status
	if v, ok := m["status"]; ok {
		switch val := v.(type) {
		case int64:
			p.Status = pbpipeline.PipelineRunStatus(val)
		case int:
			p.Status = pbpipeline.PipelineRunStatus(int32(val))
		case float64:
			p.Status = pbpipeline.PipelineRunStatus(int32(val))
		case string:
			if enumVal, ok := pbpipeline.PipelineRunStatus_value[val]; ok {
				p.Status = pbpipeline.PipelineRunStatus(enumVal)
			}
		}
	}

	// Boosters
	if bList, ok := m["boosters"].([]interface{}); ok {
		p.Boosters = make([]*pbpipeline.BoosterExecution, len(bList))
		for i, bRaw := range bList {
			if bMap, ok := bRaw.(map[string]interface{}); ok {
				booster := &pbpipeline.BoosterExecution{
					ProviderName: getString(bMap, "provider_name"),
					Error:        stringPtrOrNil(getString(bMap, "error")),
				}
				// "status" was a freeform string (field 2, now reserved); map to typed enum
				if v, ok := bMap["status"]; ok {
					switch val := v.(type) {
					case int64:
						booster.Status = pbpipeline.ExecutionStepStatus(val)
					case int:
						booster.Status = pbpipeline.ExecutionStepStatus(int32(val))
					case float64:
						booster.Status = pbpipeline.ExecutionStepStatus(int32(val))
					case string:
						if enumVal, ok := pbpipeline.ExecutionStepStatus_value[val]; ok {
							booster.Status = pbpipeline.ExecutionStepStatus(enumVal)
						}
					}
				}
				if v, ok := bMap["duration_ms"]; ok {
					switch n := v.(type) {
					case int64:
						booster.DurationMs = n
					case int:
						booster.DurationMs = int64(n)
					case float64:
						booster.DurationMs = int64(n)
					}
				}
				if meta, ok := bMap["metadata"].(map[string]interface{}); ok {
					booster.Metadata = make(map[string]string)
					for k, v := range meta {
						if s, ok := v.(string); ok {
							booster.Metadata[k] = s
						}
					}
				}
				p.Boosters[i] = booster
			}
		}
	}

	// Destinations
	if dList, ok := m["destinations"].([]interface{}); ok {
		p.Destinations = make([]*pbpipeline.DestinationOutcome, len(dList))
		for i, dRaw := range dList {
			if dMap, ok := dRaw.(map[string]interface{}); ok {
				dest := &pbpipeline.DestinationOutcome{
					ExternalId:  stringPtrOrNil(getString(dMap, "external_id")),
					Error:       stringPtrOrNil(getString(dMap, "error")),
					CompletedAt: getTime(dMap, "completed_at"),
				}
				if v, ok := dMap["destination"]; ok {
					switch val := v.(type) {
					case int64:
						dest.Destination = pbplugin.DestinationType(val)
					case int:
						dest.Destination = pbplugin.DestinationType(int32(val))
					case float64:
						dest.Destination = pbplugin.DestinationType(int32(val))
					case string:
						upper := strings.ToUpper(val)
						if enumVal, ok := pbplugin.DestinationType_value[upper]; ok {
							dest.Destination = pbplugin.DestinationType(enumVal)
						} else {
							prefixed := "DESTINATION_" + upper
							if enumVal, ok := pbplugin.DestinationType_value[prefixed]; ok {
								dest.Destination = pbplugin.DestinationType(enumVal)
							}
						}
					}
				}
				if v, ok := dMap["status"]; ok {
					switch val := v.(type) {
					case int64:
						dest.Status = pbpipeline.DestinationStatus(val)
					case int:
						dest.Status = pbpipeline.DestinationStatus(int32(val))
					case float64:
						dest.Status = pbpipeline.DestinationStatus(int32(val))
					case string:
						if enumVal, ok := pbpipeline.DestinationStatus_value[val]; ok {
							dest.Status = pbpipeline.DestinationStatus(enumVal)
						}
					}
				}
				p.Destinations[i] = dest
			}
		}
	}

	// Note: enriched_event is now stored in GCS via enriched_event_uri
	p.EnrichedEventUri = getString(m, "enriched_event_uri")

	// Note: original_payload is now stored in GCS via original_payload_uri

	return p
}
