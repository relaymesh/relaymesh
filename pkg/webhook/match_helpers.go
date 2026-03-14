package webhook

import (
	"strings"

	"github.com/relaymesh/relaymesh/pkg/core"
)

func ruleMatchesFromRules(rules []core.MatchedRule) []core.RuleMatch {
	matches := make([]core.RuleMatch, 0, len(rules))
	for _, rule := range rules {
		for _, topic := range rule.Emit {
			matches = append(matches, core.RuleMatch{
				Topic:            topic,
				DriverID:         rule.DriverID,
				TransformJS:      rule.TransformJS,
				DriverName:       rule.DriverName,
				DriverConfigJSON: rule.DriverConfigJSON,
				DriverEnabled:    rule.DriverEnabled,
				RuleID:           rule.ID,
				RuleWhen:         rule.When,
			})
		}
	}
	return matches
}

func topicsFromMatches(matches []core.RuleMatch) []string {
	topics := make([]string, 0, len(matches))
	for _, match := range matches {
		if match.Topic == "" {
			continue
		}
		topics = append(topics, match.Topic)
	}
	return topics
}

func driverListFromMatch(match core.RuleMatch) []string {
	driverName := strings.TrimSpace(match.DriverName)
	if driverName == "" {
		driverName = strings.TrimSpace(match.DriverID)
	}
	if driverName == "" {
		return nil
	}
	return []string{driverName}
}
