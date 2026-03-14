package core

import (
	"fmt"
	"log"
	"strings"
	"sync"

	"github.com/Knetic/govaluate"
	"gopkg.in/yaml.v3"

	"github.com/relaymesh/relaymesh/pkg/cache"
)

// Rule defines a condition and an action to take when the condition is met.
type Rule struct {
	// ID is an optional identifier for the rule.
	ID string `yaml:"id"`
	// When is a govaluate expression that is evaluated against the event data.
	When string `yaml:"when"`
	// Emit is the topic to publish the event to if the 'When' expression is true.
	Emit EmitList `yaml:"emit"`
	// DriverID is the identifier of the driver in storage.
	DriverID    string `yaml:"driver_id"`
	TransformJS string `yaml:"transform_js"`
	// DriverName is used internally to track the driver name resolved from storage.
	DriverName string `yaml:"-"`
	// DriverConfigJSON stores the Relaybus driver configuration (optional).
	DriverConfigJSON string `yaml:"driver_config_json"`
	// DriverEnabled indicates whether the associated driver is enabled.
	DriverEnabled bool `yaml:"driver_enabled"`
}

// compiledRule is a pre-processed version of a Rule.
type compiledRule struct {
	id               string
	when             string
	emit             []string
	driverID         string
	transformJS      string
	driverName       string
	driverConfigJSON string
	driverEnabled    bool
	vars             []string
	varMap           map[string]string
	expr             *govaluate.EvaluableExpression
}

// RuleEngine evaluates events against a set of rules.
type RuleEngine struct {
	mu     sync.RWMutex
	rules  []compiledRule
	strict bool
	logger *log.Logger
	tenant *cache.TenantCache[ruleSet]
}

type ruleSet struct {
	rules  []compiledRule
	strict bool
}

// RuleMatch represents a successful rule evaluation.
type RuleMatch struct {
	Topic            string
	DriverID         string
	TransformJS      string
	DriverName       string
	RuleID           string
	RuleWhen         string
	DriverConfigJSON string
	DriverEnabled    bool
}

// MatchedRule represents a successful rule evaluation with the original rule data.
type MatchedRule struct {
	ID               string
	When             string
	Emit             []string
	DriverID         string
	TransformJS      string
	DriverName       string
	DriverConfigJSON string
	DriverEnabled    bool
}

// NewRuleEngine creates a new RuleEngine from a set of rules.
// It pre-compiles the expressions in the rules for faster evaluation.
func NewRuleEngine(cfg RulesConfig) (*RuleEngine, error) {
	logger := cfg.Logger
	if logger == nil {
		logger = log.Default()
	}
	engine := &RuleEngine{logger: logger, strict: cfg.Strict}
	if err := engine.Update(cfg); err != nil {
		return nil, err
	}
	return engine, nil
}

// Update replaces the rule set and strict mode in the engine.
func (r *RuleEngine) Update(cfg RulesConfig) error {
	logger := cfg.Logger
	if logger == nil {
		logger = r.logger
	}
	rules := make([]compiledRule, 0, len(cfg.Rules))
	for _, rule := range cfg.Rules {
		rewritten, varMap := rewriteExpression(rule.When)
		expr, err := govaluate.NewEvaluableExpressionWithFunctions(rewritten, ruleFunctions())
		if err != nil {
			return err
		}
		emit := rule.Emit.Values()
		ruleID := strings.TrimSpace(rule.ID)
		if ruleID == "" {
			ruleID = ruleIDFromParts(rule.When, emit, rule.DriverID)
		}
		rules = append(rules, compiledRule{
			id:               ruleID,
			when:             rule.When,
			emit:             emit,
			driverID:         strings.TrimSpace(rule.DriverID),
			transformJS:      strings.TrimSpace(rule.TransformJS),
			driverName:       strings.TrimSpace(rule.DriverName),
			driverConfigJSON: strings.TrimSpace(rule.DriverConfigJSON),
			driverEnabled:    rule.DriverEnabled,
			vars:             expr.Vars(),
			varMap:           varMap,
			expr:             expr,
		})
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	tenantID := strings.TrimSpace(cfg.TenantID)
	if tenantID == "" {
		r.rules = rules
		r.strict = cfg.Strict
	} else {
		if r.tenant == nil {
			r.tenant = cache.NewTenantCache[ruleSet]()
		}
		if len(rules) == 0 {
			r.tenant.Delete(tenantID)
		} else {
			r.tenant.Set(tenantID, ruleSet{rules: rules, strict: cfg.Strict})
		}
	}
	if logger != nil {
		r.logger = logger
	}
	return nil
}

// EmitList supports either a string or list of strings in YAML.
type EmitList []string

func (e *EmitList) UnmarshalYAML(value *yaml.Node) error {
	switch value.Kind {
	case yaml.ScalarNode:
		if value.Value == "" {
			*e = nil
			return nil
		}
		*e = EmitList{value.Value}
		return nil
	case yaml.SequenceNode:
		out := make([]string, 0, len(value.Content))
		for _, item := range value.Content {
			if item.Kind != yaml.ScalarNode {
				return fmt.Errorf("emit items must be strings")
			}
			out = append(out, item.Value)
		}
		*e = EmitList(out)
		return nil
	default:
		return fmt.Errorf("emit must be a string or list of strings")
	}
}

func (e EmitList) Values() []string {
	out := make([]string, 0, len(e))
	for _, val := range e {
		trimmed := strings.TrimSpace(val)
		if trimmed != "" {
			out = append(out, trimmed)
		}
	}
	return out
}

// Evaluate runs an event through the rule engine and returns a list of topics to publish to.
func (r *RuleEngine) Evaluate(event Event) []RuleMatch {
	return r.evaluateWithLogger(event, r.logger)
}

func (r *RuleEngine) EvaluateWithLogger(event Event, logger *log.Logger) []RuleMatch {
	return r.evaluateWithLogger(event, logger)
}

func (r *RuleEngine) EvaluateForTenant(event Event, tenantID string) []RuleMatch {
	return r.evaluateWithLoggerForTenant(event, tenantID, r.logger)
}

func (r *RuleEngine) EvaluateForTenantWithLogger(event Event, tenantID string, logger *log.Logger) []RuleMatch {
	return r.evaluateWithLoggerForTenant(event, tenantID, logger)
}

func (r *RuleEngine) evaluateWithLogger(event Event, logger *log.Logger) []RuleMatch {
	return r.evaluateWithLoggerForTenant(event, "", logger)
}

func (r *RuleEngine) evaluateWithLoggerForTenant(event Event, tenantID string, logger *log.Logger) []RuleMatch {
	r.mu.RLock()
	defer r.mu.RUnlock()
	tenantID = strings.TrimSpace(tenantID)
	rules := r.rules
	strict := r.strict
	if tenantID != "" {
		if r.tenant == nil {
			return nil
		}
		set, ok := r.tenant.Get(tenantID)
		if !ok {
			return nil
		}
		rules = set.rules
		strict = set.strict
	}
	if len(rules) == 0 {
		return nil
	}
	if logger == nil {
		logger = log.Default()
	}

	matches := make([]RuleMatch, 0, 1)
	for _, rule := range rules {
		params, missing := resolveRuleParams(logger, event, rule.vars, rule.varMap)
		logger.Printf("rule debug: when=%q params=%v", rule.expr.String(), params)
		if strict && len(missing) > 0 {
			logger.Printf("rule strict missing params: %v", missing)
			continue
		}
		result, err := rule.expr.Evaluate(params)
		if err != nil {
			logger.Printf("rule eval failed: %v", err)
			continue
		}
		ok, _ := result.(bool)
		if ok {
			for _, topic := range rule.emit {
				matches = append(matches, RuleMatch{
					Topic:            topic,
					DriverID:         rule.driverID,
					TransformJS:      rule.transformJS,
					DriverName:       rule.driverName,
					RuleID:           rule.id,
					RuleWhen:         rule.when,
					DriverConfigJSON: rule.driverConfigJSON,
					DriverEnabled:    rule.driverEnabled,
				})
			}
		}
	}
	return matches
}

// EvaluateRules returns rule-level matches with the original rule metadata.
func (r *RuleEngine) EvaluateRules(event Event) []MatchedRule {
	return r.evaluateRulesWithLogger(event, r.logger)
}

// EvaluateRulesWithLogger returns rule-level matches using the provided logger.
func (r *RuleEngine) EvaluateRulesWithLogger(event Event, logger *log.Logger) []MatchedRule {
	return r.evaluateRulesWithLogger(event, logger)
}

func (r *RuleEngine) EvaluateRulesForTenant(event Event, tenantID string) []MatchedRule {
	return r.evaluateRulesWithLoggerForTenant(event, tenantID, r.logger)
}

// EvaluateRulesForTenantWithLogger returns rule-level matches scoped to a tenant using the provided logger.
func (r *RuleEngine) EvaluateRulesForTenantWithLogger(event Event, tenantID string, logger *log.Logger) []MatchedRule {
	return r.evaluateRulesWithLoggerForTenant(event, tenantID, logger)
}

func (r *RuleEngine) evaluateRulesWithLogger(event Event, logger *log.Logger) []MatchedRule {
	return r.evaluateRulesWithLoggerForTenant(event, "", logger)
}

func (r *RuleEngine) evaluateRulesWithLoggerForTenant(event Event, tenantID string, logger *log.Logger) []MatchedRule {
	r.mu.RLock()
	defer r.mu.RUnlock()
	tenantID = strings.TrimSpace(tenantID)
	rules := r.rules
	strict := r.strict
	if tenantID != "" {
		if r.tenant == nil {
			return nil
		}
		set, ok := r.tenant.Get(tenantID)
		if !ok {
			return nil
		}
		rules = set.rules
		strict = set.strict
	}
	if len(rules) == 0 {
		return nil
	}
	if logger == nil {
		logger = log.Default()
	}

	matches := make([]MatchedRule, 0, 1)
	for _, rule := range rules {
		params, missing := resolveRuleParams(logger, event, rule.vars, rule.varMap)
		logger.Printf("rule debug: when=%q params=%v", rule.expr.String(), params)
		if strict && len(missing) > 0 {
			logger.Printf("rule strict missing params: %v", missing)
			continue
		}
		result, err := rule.expr.Evaluate(params)
		if err != nil {
			logger.Printf("rule eval failed: %v", err)
			continue
		}
		ok, _ := result.(bool)
		if ok {
			matches = append(matches, MatchedRule{
				ID:               rule.id,
				When:             rule.when,
				Emit:             append([]string(nil), rule.emit...),
				DriverID:         rule.driverID,
				TransformJS:      rule.transformJS,
				DriverName:       rule.driverName,
				DriverConfigJSON: rule.driverConfigJSON,
				DriverEnabled:    rule.driverEnabled,
			})
		}
	}
	return matches
}
