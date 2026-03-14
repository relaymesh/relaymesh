package webhook

import (
	"context"
	"log"

	"github.com/relaymesh/relaymesh/pkg/core"
)

func (h *GitHubHandler) matchRules(ctx context.Context, event core.Event, tenantID string, logger *log.Logger) []core.MatchedRule {
	if h.ruleStore != nil {
		return matchRulesFromStore(ctx, event, tenantID, h.ruleStore, h.driverStore, h.rulesStrict, logger)
	}
	if h.rules == nil {
		return nil
	}
	return h.rules.EvaluateRulesForTenantWithLogger(event, tenantID, logger)
}
