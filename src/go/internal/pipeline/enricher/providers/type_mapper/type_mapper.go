// nolint:proto-json
package type_mapper

import (
	"context"
	"encoding/json"
	"fmt"
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

	// Check for type_rules JSON object (from web UI: {"title substring": "ACTIVITY_TYPE_..."})
	//
	// Rules are matched in the order they appear in the config ("first match wins"), so
	// order must be preserved. Unmarshalling into a map[string]string would lose it: Go
	// randomizes map iteration order, which made matching non-deterministic when more than
	// one substring matched a title. We therefore stream-decode the object to keep the
	// author's ordering.
	typeRulesJson, hasTypeRules := inputConfig["type_rules"]
	if hasTypeRules && typeRulesJson != "" {
		if orderedRules, err := parseOrderedTypeRules(typeRulesJson); err == nil {
			rules = append(rules, orderedRules...)
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

// parseOrderedTypeRules decodes a JSON object of {"substring": "target_type"} pairs while
// preserving the order in which keys appear in the source. This matters because rules are
// applied "first match wins": decoding into a map[string]string would discard order and,
// because Go randomizes map iteration, make the winning rule non-deterministic whenever more
// than one substring matches a title.
func parseOrderedTypeRules(raw string) ([]TypeMapperRule, error) {
	dec := json.NewDecoder(strings.NewReader(raw))

	// Expect the opening object delimiter.
	tok, err := dec.Token()
	if err != nil {
		return nil, err
	}
	if delim, ok := tok.(json.Delim); !ok || delim != '{' {
		return nil, fmt.Errorf("type_rules: expected JSON object, got %v", tok)
	}

	var rules []TypeMapperRule
	for dec.More() {
		keyTok, err := dec.Token()
		if err != nil {
			return nil, err
		}
		substring, ok := keyTok.(string)
		if !ok {
			return nil, fmt.Errorf("type_rules: expected string key, got %v", keyTok)
		}

		var targetType string
		if err := dec.Decode(&targetType); err != nil {
			return nil, err
		}

		if substring != "" && targetType != "" {
			rules = append(rules, TypeMapperRule{
				Substring:  substring,
				TargetType: targetType,
			})
		}
	}

	return rules, nil
}
