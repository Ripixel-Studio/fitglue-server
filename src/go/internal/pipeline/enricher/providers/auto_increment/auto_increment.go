// nolint:proto-json
package auto_increment

import (
	"context"
	"encoding/json"
	"fmt"
	"github.com/fitglue/server/src/go/pkg/domain/user"
	"log/slog"
	"strings"

	"github.com/fitglue/server/src/go/internal/pipeline/enricher/providers"
	"github.com/fitglue/server/src/go/pkg/bootstrap"
	pbuser "github.com/fitglue/server/src/go/pkg/types/pb/models/user"

	pbactivity "github.com/fitglue/server/src/go/pkg/types/pb/models/activity"

	pbplugin "github.com/fitglue/server/src/go/pkg/types/pb/models/plugin"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/timestamppb"
)

type AutoIncrementProvider struct {
	service *bootstrap.Service
}

func init() {
	providers.Register(&AutoIncrementProvider{})
}

func (p *AutoIncrementProvider) SetService(s *bootstrap.Service) {
	p.service = s
}

func (p *AutoIncrementProvider) Name() string {
	return "auto_increment"
}

func (p *AutoIncrementProvider) ProviderType() pbplugin.EnricherProviderType {
	return pbplugin.EnricherProviderType_ENRICHER_PROVIDER_AUTO_INCREMENT
}

func (p *AutoIncrementProvider) IsIdempotent() bool { return false }

func (p *AutoIncrementProvider) Enrich(ctx context.Context, logger *slog.Logger, activity *pbactivity.StandardizedActivity, user *user.Record, inputs map[string]string, doNotRetry bool) (*providers.EnrichmentResult, error) {
	logger.Debug("auto_increment: starting",
		"activity_name", activity.Name,
		"has_counter_rules", inputs["counter_rules"] != "",
		"counter_key", inputs["counter_key"],
		"initial_value", inputs["initial_value"],
	)

	// 1. Resolve counter key — new counter_rules map or legacy counter_key field
	key := p.resolveCounterKey(logger, activity.Name, inputs)
	if key == "" {
		logger.Debug("auto_increment: skipping - no matching counter key")
		return &providers.EnrichmentResult{
			Metadata: map[string]string{
				"auto_increment_applied": "false",
				"reason":                 "No matching rule",
			},
		}, nil
	}

	if p.service == nil {
		logger.Debug("auto_increment: error - service not initialized")
		return &providers.EnrichmentResult{
			Metadata: map[string]string{
				"auto_increment_applied": "false",
			},
		}, fmt.Errorf("service not initialized")
	}

	// Same-source dedup: check if this activity was already processed for this counter
	externalId := inputs["external_id"]
	if externalId != "" && p.service.DB != nil {
		boosterId := fmt.Sprintf("auto_increment_%s", key)
		data, err := p.service.DB.GetBoosterData(ctx, user.UserId, boosterId)
		if err == nil && data != nil {
			if storedExtId, ok := data["last_external_id"].(string); ok && storedExtId == externalId {
				// Return cached result
				cachedSuffix := ""
				if v, ok := data["last_result_suffix"].(string); ok {
					cachedSuffix = v
				}
				cachedVal := ""
				if v, ok := data["last_result_val"].(string); ok {
					cachedVal = v
				}
				logger.Info("auto_increment: returning cached result for same-source activity",
					"external_id", externalId, "cached_suffix", cachedSuffix)
				return &providers.EnrichmentResult{
					NameSuffix: cachedSuffix,
					Metadata: map[string]string{
						"auto_increment_applied": "true",
						"auto_increment_key":     key,
						"auto_increment_val":     cachedVal,
						"dedup":                  "true",
					},
				}, nil
			}
		}
	}

	// 2. Get/Increment Counter
	counter, err := p.service.DB.GetCounter(ctx, user.UserId, key)
	if err != nil {
		if status.Code(err) == codes.NotFound {
			logger.Debug("auto_increment: counter not found, will initialize",
				"key", key,
			)
			counter = nil // Treat as missing -> initialize below
		} else {
			// Real error from DB
			logger.Debug("auto_increment: error getting counter",
				"error", err.Error(),
			)
			return &providers.EnrichmentResult{
				Metadata: map[string]string{
					"auto_increment_applied": "false",
				},
			}, fmt.Errorf("failed to get counter: %w", err)
		}
	}

	if counter == nil {
		// Not found - initialize at 0 so first increment yields 1
		counter = &pbuser.Counter{
			Id:    key,
			Count: 0,
		}
	}

	newCount := counter.Count + 1
	counter.Count = newCount
	counter.LastUpdated = timestamppb.Now()

	logger.Debug("auto_increment: incrementing counter",
		"key", key,
		"previous_count", counter.Count-1,
		"new_count", newCount,
	)

	// Persist counter
	if err := p.service.DB.SetCounter(ctx, user.UserId, counter); err != nil {
		logger.Debug("auto_increment: error persisting counter",
			"error", err.Error(),
		)
		return nil, fmt.Errorf("failed to update counter: %w", err)
	}

	nameSuffix := fmt.Sprintf(" (#%d)", newCount)

	// Cache result for same-source dedup
	if externalId != "" && p.service.DB != nil {
		boosterId := fmt.Sprintf("auto_increment_%s", key)
		cacheData := map[string]interface{}{
			"last_external_id":   externalId,
			"last_result_suffix": nameSuffix,
			"last_result_val":    fmt.Sprintf("%d", newCount),
		}
		if err := p.service.DB.SetBoosterData(ctx, user.UserId, boosterId, cacheData); err != nil {
			logger.Warn("auto_increment: failed to cache dedup result", "error", err)
		}
	}

	logger.Debug("auto_increment: successfully applied",
		"key", key,
		"new_count", newCount,
		"suffix", nameSuffix,
	)

	return &providers.EnrichmentResult{
		NameSuffix: nameSuffix,
		Metadata: map[string]string{
			"auto_increment_applied": "true",
			"auto_increment_key":     key,
			"auto_increment_val":     fmt.Sprintf("%d", newCount),
		},
	}, nil
}

// resolveCounterKey determines the counter key to use based on inputs.
// New format: counter_rules JSON map {"title substring": "counter_key"} — first match wins.
// Legacy format: counter_key + optional title_contains filter.
func (p *AutoIncrementProvider) resolveCounterKey(logger *slog.Logger, activityName string, inputs map[string]string) string {
	// New format: counter_rules JSON map
	if rulesJSON, ok := inputs["counter_rules"]; ok && rulesJSON != "" {
		var rules map[string]string
		if err := json.Unmarshal([]byte(rulesJSON), &rules); err != nil {
			logger.Debug("auto_increment: failed to parse counter_rules JSON",
				"error", err.Error(),
			)
			return ""
		}

		for substring, counterKey := range rules {
			if substring == "" || counterKey == "" {
				continue
			}
			if strings.Contains(strings.ToLower(activityName), strings.ToLower(substring)) {
				logger.Debug("auto_increment: counter_rules matched",
					"matched_substring", substring,
					"counter_key", counterKey,
				)
				return counterKey
			}
		}

		logger.Debug("auto_increment: no counter_rules matched",
			"activity_name", activityName,
			"rule_count", len(rules),
		)
		return ""
	}

	// Legacy format: counter_key + optional title_contains
	key := inputs["counter_key"]
	if key == "" {
		logger.Debug("auto_increment: skipping - no counter_key configured")
		return ""
	}

	if filter, ok := inputs["title_contains"]; ok && filter != "" {
		if !strings.Contains(strings.ToLower(activityName), strings.ToLower(filter)) {
			logger.Debug("auto_increment: skipping - title does not match filter",
				"filter", filter,
				"activity_name", activityName,
			)
			return ""
		}
		logger.Debug("auto_increment: title filter matched",
			"filter", filter,
		)
	}

	return key
}
