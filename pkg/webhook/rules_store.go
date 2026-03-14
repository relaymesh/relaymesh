package webhook

import (
	"context"
	"errors"
	"fmt"
	"log"
	"strings"

	"github.com/relaymesh/relaymesh/pkg/core"
	"github.com/relaymesh/relaymesh/pkg/storage"
)

func matchRulesFromStore(ctx context.Context, event core.Event, tenantID string, ruleStore storage.RuleStore, driverStore storage.DriverStore, strict bool, logger *log.Logger) []core.MatchedRule {
	if ruleStore == nil {
		return nil
	}
	tenantCtx := storage.WithTenant(ctx, tenantID)
	if logger != nil {
		logger.Printf("rule load requested tenant=%s provider=%s event=%s", tenantID, event.Provider, event.Name)
		logger.Printf("rule engine start tenant=%s provider=%s event=%s", tenantID, event.Provider, event.Name)
	}
	rules, err := loadRulesForTenant(tenantCtx, ruleStore, driverStore, logger)
	if err != nil {
		if logger != nil {
			logger.Printf("rule load failed: %v", err)
		}
		return nil
	}
	ruleMap := make(map[string]core.Rule, len(rules))
	for _, rule := range rules {
		ruleMap[rule.ID] = rule
	}
	if logger != nil {
		logger.Printf("rule engine loaded %d rules tenant=%s provider=%s", len(rules), tenantID, event.Provider)
	}
	if len(rules) == 0 {
		return nil
	}
	engine, err := core.NewRuleEngine(core.RulesConfig{
		Rules:  rules,
		Strict: strict,
		Logger: logger,
	})
	if err != nil {
		if logger != nil {
			logger.Printf("rule engine compile failed: %v", err)
		}
		return nil
	}
	matches := engine.EvaluateRulesWithLogger(event, logger)
	if logger != nil {
		logger.Printf("rule evaluation complete matches=%d tenant=%s provider=%s event=%s", len(matches), tenantID, event.Provider, event.Name)
	}
	matchMap := make(map[string]struct{}, len(matches))
	if logger != nil && len(matches) > 0 {
		for _, match := range matches {
			matchMap[match.ID] = struct{}{}
			rule := ruleMap[match.ID]
			emit := rule.Emit
			if len(emit) == 0 {
				emit = []string{}
			}
			for _, topic := range emit {
				logger.Printf("rule match id=%s when=%q emit=%v topic=%s driver_id=%s driver_name=%s",
					match.ID,
					rule.When,
					emit,
					topic,
					match.DriverID,
					match.DriverName,
				)
			}
			if len(emit) == 0 {
				logger.Printf("rule match id=%s when=%q emit=%v topic= driver_id=%s driver_name=%s",
					match.ID,
					rule.When,
					emit,
					match.DriverID,
					match.DriverName,
				)
			}
		}
	}
	if logger != nil && len(matches) == 0 {
		logger.Printf("rule match none tenant=%s provider=%s event=%s", tenantID, event.Provider, event.Name)
	}
	if logger != nil {
		for _, rule := range rules {
			if _, ok := matchMap[rule.ID]; ok {
				continue
			}
			logger.Printf("rule unmatched id=%s when=%q emit=%v", rule.ID, rule.When, rule.Emit)
		}
	}
	return matches
}

func loadRulesForTenant(ctx context.Context, store storage.RuleStore, driverStore storage.DriverStore, logger *log.Logger) ([]core.Rule, error) {
	if store == nil {
		return nil, nil
	}
	records, err := store.ListRules(ctx)
	if err != nil {
		return nil, err
	}
	rules := make([]core.Rule, 0, len(records))
	for _, record := range records {
		if logger != nil {
			logger.Printf("rule record raw id=%s tenant=%s when=%q emit=%v driver_id=%s driver_name=%s enabled=%t config=%s",
				record.ID,
				record.TenantID,
				record.When,
				record.Emit,
				record.DriverID,
				record.DriverName,
				record.DriverEnabled,
				record.DriverConfigJSON,
			)
		}
		if strings.TrimSpace(record.When) == "" {
			continue
		}
		if len(record.Emit) == 0 {
			continue
		}
		driverID := strings.TrimSpace(record.DriverID)
		if driverID == "" {
			continue
		}
		driverName := strings.TrimSpace(record.DriverName)
		if driverName == "" {
			var err error
			driverName, err = driverNameForID(ctx, driverStore, driverID)
			if err != nil {
				if logger != nil {
					logger.Printf("rule driver resolve failed: %v", err)
				}
				continue
			}
		}
		if logger != nil {
			logger.Printf(
				"rule fetched id=%s driver_id=%s driver_name=%s enabled=%t emit=%v config=%s",
				record.ID,
				driverID,
				driverName,
				record.DriverEnabled,
				record.Emit,
				record.DriverConfigJSON,
			)
		}
		rules = append(rules, core.Rule{
			ID:               record.ID,
			When:             record.When,
			Emit:             core.EmitList(record.Emit),
			DriverID:         driverID,
			TransformJS:      strings.TrimSpace(record.TransformJS),
			DriverName:       driverName,
			DriverConfigJSON: strings.TrimSpace(record.DriverConfigJSON),
			DriverEnabled:    record.DriverEnabled,
		})
	}
	return rules, nil
}

func driverNameForID(ctx context.Context, store storage.DriverStore, driverID string) (string, error) {
	if strings.TrimSpace(driverID) == "" {
		return "", errors.New("driver_id is required")
	}
	if store == nil {
		return "", errors.New("driver store not configured")
	}
	record, err := store.GetDriverByID(ctx, driverID)
	if err != nil {
		return "", err
	}
	if record == nil {
		return "", fmt.Errorf("driver not found: %s", driverID)
	}
	name := strings.TrimSpace(record.Name)
	if name == "" {
		return "", fmt.Errorf("driver %s has empty name", driverID)
	}
	return name, nil
}
