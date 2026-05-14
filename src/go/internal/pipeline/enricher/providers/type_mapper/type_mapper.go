// nolint:proto-json
package type_mapper

import (
	"context"
	"encoding/json"
	"github.com/fitglue/server/src/go/pkg/domain/user"
	"log/slog"
	"strings"

	"github.com/fitglue/server/src/go/internal/pipeline/enricher/providers"
	"github.com/fitglue/server/src/go/pkg/domain/activity"

	pbactivity "github.com/fitglue/server/src/go/pkg/types/pb/models/activity"

	pbplugin "github.com/fitglue/server/src/go/pkg/types/pb/models/plugin"
)

type TypeMapperProvider struct{}

func init() {
	providers.Register(NewTypeMapperProvider())
}

func NewTypeMapperProvider() *TypeMapperProvider {
	return &TypeMapperProvider{}
}

func (p *TypeMapperProvider) Name() string {
	return "type-mapper"
}

func (p *TypeMapperProvider) ProviderType() pbplugin.EnricherProviderType {
	return pbplugin.EnricherProviderType_ENRICHER_PROVIDER_TYPE_MAPPER
}

// TypeMapperRule represents a single rule for mapping activity types based on title
type TypeMapperRule struct {
	Substring  string `json:"substring"`
	TargetType string `json:"target_type"`
}

func (p *TypeMapperProvider) Enrich(ctx context.Context, logger *slog.Logger, act *pbactivity.StandardizedActivity, user *user.Record, inputConfig map[string]string, doNotRetry bool) (*providers.EnrichmentResult, error) {
	logger.Debug("type_mapper: starting",
		"activity_type", act.Type.String(),
		"activity_title", act.Name,
		"has_type_rules", inputConfig["type_rules"] != "",
		"has_rules", inputConfig["rules"] != "",
	)

	var rules []TypeMapperRule

	// Check for type_rules JSON map (from web UI: {"title substring": "ACTIVITY_TYPE_..."})
	typeRulesJson, hasTypeRules := inputConfig["type_rules"]
	if hasTypeRules && typeRulesJson != "" {
		var typeRulesMap map[string]string
		if err := json.Unmarshal([]byte(typeRulesJson), &typeRulesMap); err == nil {
			for substring, targetType := range typeRulesMap {
				if substring != "" && targetType != "" {
					rules = append(rules, TypeMapperRule{
						Substring:  substring,
						TargetType: targetType,
					})
				}
			}
			logger.Debug("type_mapper: parsed type_rules",
				"rule_count", len(rules),
			)
		} else {
			logger.Debug("type_mapper: failed to parse type_rules JSON",
				"error", err.Error(),
			)
		}
	}

	// Also check for rules JSON array (from admin-cli)
	rulesJson, ok := inputConfig["rules"]
	if ok && rulesJson != "" {
		var jsonRules []TypeMapperRule
		if err := json.Unmarshal([]byte(rulesJson), &jsonRules); err == nil {
			rules = append(rules, jsonRules...)
			logger.Debug("type_mapper: parsed rules array",
				"additional_rules", len(jsonRules),
				"total_rules", len(rules),
			)
		}
	}

	// No rules configured, nothing to do
	if len(rules) == 0 {
		logger.Debug("type_mapper: skipping - no rules configured")
		return &providers.EnrichmentResult{
			Metadata: map[string]string{"status": "skipped", "reason": "no_rules_configured"},
		}, nil
	}

	// Get the current activity title
	activityTitle := act.Name
	if activityTitle == "" {
		logger.Debug("type_mapper: skipping - no activity title")
		return &providers.EnrichmentResult{
			Metadata: map[string]string{"status": "skipped", "reason": "no_activity_title"},
		}, nil
	}

	// Get original type for metadata
	originalType := act.Type
	originalTypeName := activity.GetStravaActivityType(originalType)

	logger.Debug("type_mapper: checking rules against title",
		"title", activityTitle,
		"original_type", originalTypeName,
		"rule_count", len(rules),
	)

	// Check each rule - first match wins
	for i, rule := range rules {
		if rule.Substring == "" {
			continue
		}
		// Match case-insensitively against the activity title
		if strings.Contains(strings.ToLower(activityTitle), strings.ToLower(rule.Substring)) {
			// Parse the target type
			newType := activity.ParseActivityTypeFromString(rule.TargetType)
			if newType != pbactivity.ActivityType_ACTIVITY_TYPE_UNSPECIFIED {
				logger.Debug("type_mapper: matched rule - changing type",
					"rule_index", i,
					"matched_substring", rule.Substring,
					"original_type", originalTypeName,
					"new_type", activity.GetStravaActivityType(newType),
				)
				return &providers.EnrichmentResult{
					ActivityType: newType,
					Metadata: map[string]string{
						"original_type":   originalTypeName,
						"new_type":        activity.GetStravaActivityType(newType),
						"matched_title":   activityTitle,
						"matched_pattern": rule.Substring,
					},
				}, nil
			} else {
				logger.Debug("type_mapper: rule matched but target type invalid",
					"rule_index", i,
					"matched_substring", rule.Substring,
					"target_type", rule.TargetType,
				)
			}
		}
	}

	// No matching rule found
	logger.Debug("type_mapper: no rules matched",
		"title", activityTitle,
		"rules_checked", len(rules),
	)
	return &providers.EnrichmentResult{
		Metadata: map[string]string{"status": "skipped", "reason": "no_matching_rule", "title": activityTitle},
	}, nil
}
