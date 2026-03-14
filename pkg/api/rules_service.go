package api

import (
	"context"
	"errors"
	"fmt"
	"log"
	"strings"

	"connectrpc.com/connect"

	"github.com/relaymesh/relaymesh/pkg/core"
	cloudv1 "github.com/relaymesh/relaymesh/pkg/gen/cloud/v1"
	"github.com/relaymesh/relaymesh/pkg/storage"
)

// RulesService implements rule matching over a payload with inline rules.
type RulesService struct {
	Store       storage.RuleStore
	DriverStore storage.DriverStore
	Engine      *core.RuleEngine
	Strict      bool
	Logger      *log.Logger
}

func (s *RulesService) MatchRules(
	ctx context.Context,
	req *connect.Request[cloudv1.MatchRulesRequest],
) (*connect.Response[cloudv1.MatchRulesResponse], error) {
	event := req.Msg.GetEvent()
	rules := make([]core.Rule, 0, len(req.Msg.GetRules()))
	for _, rule := range req.Msg.GetRules() {
		if rule == nil {
			continue
		}
		rules = append(rules, core.Rule{
			When:        rule.GetWhen(),
			Emit:        core.EmitList(rule.GetEmit()),
			DriverID:    strings.TrimSpace(rule.GetDriverId()),
			TransformJS: strings.TrimSpace(rule.GetTransformJs()),
		})
	}

	engine, err := core.NewRuleEngine(core.RulesConfig{
		Rules:  rules,
		Strict: req.Msg.GetStrict(),
		Logger: s.Logger,
	})
	if err != nil {
		return nil, connect.NewError(connect.CodeInvalidArgument, err)
	}

	matches := engine.EvaluateRules(core.Event{
		Provider:   event.GetProvider(),
		Name:       event.GetName(),
		RawPayload: event.GetPayload(),
	})

	resp := &cloudv1.MatchRulesResponse{
		Matches: toProtoRuleMatches(matches),
	}
	return connect.NewResponse(resp), nil
}

func (s *RulesService) ListRules(
	ctx context.Context,
	req *connect.Request[cloudv1.ListRulesRequest],
) (*connect.Response[cloudv1.ListRulesResponse], error) {
	_ = req
	if s.Store == nil {
		return nil, connect.NewError(connect.CodeFailedPrecondition, errors.New("storage not configured"))
	}
	records, err := s.Store.ListRules(ctx)
	if err != nil {
		logError(s.Logger, "list rules failed", err)
		return nil, connect.NewError(connect.CodeInternal, errors.New("list rules failed"))
	}
	resp := &cloudv1.ListRulesResponse{
		Rules: toProtoRuleRecords(records),
	}
	return connect.NewResponse(resp), nil
}

func (s *RulesService) GetRule(
	ctx context.Context,
	req *connect.Request[cloudv1.GetRuleRequest],
) (*connect.Response[cloudv1.GetRuleResponse], error) {
	if s.Store == nil {
		return nil, connect.NewError(connect.CodeFailedPrecondition, errors.New("storage not configured"))
	}
	id := strings.TrimSpace(req.Msg.GetId())
	record, err := s.Store.GetRule(ctx, id)
	if err != nil {
		logError(s.Logger, "get rule failed", err)
		return nil, connect.NewError(connect.CodeInternal, errors.New("get rule failed"))
	}
	if record == nil {
		return nil, connect.NewError(connect.CodeNotFound, errors.New("rule not found"))
	}
	resp := &cloudv1.GetRuleResponse{
		Rule: toProtoRuleRecord(*record),
	}
	return connect.NewResponse(resp), nil
}

func (s *RulesService) CreateRule(
	ctx context.Context,
	req *connect.Request[cloudv1.CreateRuleRequest],
) (*connect.Response[cloudv1.CreateRuleResponse], error) {
	if s.Store == nil {
		return nil, connect.NewError(connect.CodeFailedPrecondition, errors.New("storage not configured"))
	}
	incoming := req.Msg.GetRule()
	when, emit, driverID, transformJS, err := parseRuleInput(incoming)
	if err != nil {
		return nil, connect.NewError(connect.CodeInvalidArgument, err)
	}
	driverName, err := s.resolveDriverName(ctx, driverID)
	if err != nil {
		return nil, connect.NewError(connect.CodeInvalidArgument, err)
	}
	normalized, err := normalizeCoreRule(when, emit, driverName, driverID)
	if err != nil {
		return nil, connect.NewError(connect.CodeInvalidArgument, err)
	}
	record, err := s.Store.CreateRule(ctx, storage.RuleRecord{
		When:        normalized.When,
		Emit:        normalized.Emit.Values(),
		DriverID:    driverID,
		TransformJS: transformJS,
	})
	if err != nil {
		logError(s.Logger, "create rule failed", err)
		return nil, connect.NewError(connect.CodeInternal, errors.New("create rule failed"))
	}
	if err := validateRuleSubscriber(ctx, s.Logger, s.DriverStore, driverID); err != nil {
		logError(s.Logger, "rule subscriber validation failed", err)
		return nil, connect.NewError(connect.CodeInternal, errors.New("rule subscriber validation failed"))
	}
	if err := s.refreshEngine(ctx); err != nil {
		logError(s.Logger, "rule engine refresh failed", err)
	}
	resp := &cloudv1.CreateRuleResponse{
		Rule: toProtoRuleRecord(*record),
	}
	return connect.NewResponse(resp), nil
}

func (s *RulesService) UpdateRule(
	ctx context.Context,
	req *connect.Request[cloudv1.UpdateRuleRequest],
) (*connect.Response[cloudv1.UpdateRuleResponse], error) {
	if s.Store == nil {
		return nil, connect.NewError(connect.CodeFailedPrecondition, errors.New("storage not configured"))
	}
	id := strings.TrimSpace(req.Msg.GetId())
	incoming := req.Msg.GetRule()
	existing, err := s.Store.GetRule(ctx, id)
	if err != nil {
		logError(s.Logger, "get rule failed", err)
		return nil, connect.NewError(connect.CodeInternal, errors.New("get rule failed"))
	}
	if existing == nil {
		return nil, connect.NewError(connect.CodeNotFound, errors.New("rule not found"))
	}
	when, emit, driverID, transformJS, err := parseRuleInput(incoming)
	if err != nil {
		return nil, connect.NewError(connect.CodeInvalidArgument, err)
	}
	driverName, err := s.resolveDriverName(ctx, driverID)
	if err != nil {
		return nil, connect.NewError(connect.CodeInvalidArgument, err)
	}
	normalized, err := normalizeCoreRule(when, emit, driverName, driverID)
	if err != nil {
		return nil, connect.NewError(connect.CodeInvalidArgument, err)
	}
	record, err := s.Store.UpdateRule(ctx, storage.RuleRecord{
		ID:          id,
		When:        normalized.When,
		Emit:        normalized.Emit.Values(),
		DriverID:    driverID,
		TransformJS: transformJS,
		CreatedAt:   existing.CreatedAt,
	})
	if err != nil {
		logError(s.Logger, "update rule failed", err)
		return nil, connect.NewError(connect.CodeInternal, errors.New("update rule failed"))
	}
	if err := s.refreshEngine(ctx); err != nil {
		logError(s.Logger, "rule engine refresh failed", err)
	}
	resp := &cloudv1.UpdateRuleResponse{
		Rule: toProtoRuleRecord(*record),
	}
	return connect.NewResponse(resp), nil
}

func (s *RulesService) DeleteRule(
	ctx context.Context,
	req *connect.Request[cloudv1.DeleteRuleRequest],
) (*connect.Response[cloudv1.DeleteRuleResponse], error) {
	if s.Store == nil {
		return nil, connect.NewError(connect.CodeFailedPrecondition, errors.New("storage not configured"))
	}
	id := strings.TrimSpace(req.Msg.GetId())
	if err := s.Store.DeleteRule(ctx, id); err != nil {
		logError(s.Logger, "delete rule failed", err)
		return nil, connect.NewError(connect.CodeInternal, errors.New("delete rule failed"))
	}
	if err := s.refreshEngine(ctx); err != nil {
		logError(s.Logger, "rule engine refresh failed", err)
	}
	return connect.NewResponse(&cloudv1.DeleteRuleResponse{}), nil
}

func parseRuleInput(rule *cloudv1.Rule) (string, []string, string, string, error) {
	if rule == nil {
		return "", nil, "", "", errors.New("missing rule")
	}
	when := strings.TrimSpace(rule.GetWhen())
	rawEmit := rule.GetEmit()
	driverID := strings.TrimSpace(rule.GetDriverId())
	transformJS := strings.TrimSpace(rule.GetTransformJs())
	if driverID == "" {
		return "", nil, "", "", errors.New("driver_id is required")
	}
	if len(rawEmit) != 1 {
		return "", nil, "", "", errors.New("emit must contain exactly one topic")
	}
	topic := strings.TrimSpace(rawEmit[0])
	if topic == "" {
		return "", nil, "", "", errors.New("emit topic is required")
	}
	return when, []string{topic}, driverID, transformJS, nil
}

func normalizeCoreRule(when string, emit []string, driverName string, driverID string) (core.Rule, error) {
	coreRule := core.Rule{
		When:       strings.TrimSpace(when),
		Emit:       core.EmitList(emit),
		DriverID:   driverID,
		DriverName: driverName,
	}
	normalized, err := core.NormalizeRules([]core.Rule{coreRule})
	if err != nil {
		return core.Rule{}, err
	}
	if len(normalized) == 0 {
		return core.Rule{}, errors.New("rule is empty")
	}
	return normalized[0], nil
}

func (s *RulesService) resolveDriverName(ctx context.Context, driverID string) (string, error) {
	if driverID == "" {
		return "", errors.New("driver_id is required")
	}
	if s.DriverStore == nil {
		return "", errors.New("driver store not configured")
	}
	trimmed := strings.TrimSpace(driverID)
	if trimmed == "" {
		return "", errors.New("driver_id is required")
	}
	record, err := s.DriverStore.GetDriverByID(ctx, trimmed)
	if err != nil {
		return "", err
	}
	if record == nil {
		return "", fmt.Errorf("driver not found: %s", trimmed)
	}
	name := strings.TrimSpace(record.Name)
	if name == "" {
		return "", fmt.Errorf("driver %s has empty name", trimmed)
	}
	return name, nil
}

func (s *RulesService) refreshEngine(ctx context.Context) error {
	if s.Store == nil || s.Engine == nil {
		return nil
	}
	records, err := s.Store.ListRules(ctx)
	if err != nil {
		return err
	}
	tenantID := storage.TenantFromContext(ctx)
	if tenantID != "" {
		loaded := make([]core.Rule, 0, len(records))
		for _, record := range records {
			driverName, err := s.resolveDriverName(ctx, record.DriverID)
			if err != nil {
				logError(s.Logger, "rule driver resolve failed", err)
				continue
			}
			loaded = append(loaded, core.Rule{
				ID:          record.ID,
				When:        record.When,
				Emit:        core.EmitList(record.Emit),
				DriverID:    record.DriverID,
				TransformJS: strings.TrimSpace(record.TransformJS),
				DriverName:  driverName,
			})
		}
		normalized, err := core.NormalizeRules(loaded)
		if err != nil {
			return err
		}
		return s.Engine.Update(core.RulesConfig{
			Rules:    normalized,
			Strict:   s.Strict,
			TenantID: tenantID,
			Logger:   s.Logger,
		})
	}

	grouped := make(map[string][]core.Rule)
	for _, record := range records {
		tenantCtx := storage.WithTenant(ctx, record.TenantID)
		driverName, err := s.resolveDriverName(tenantCtx, record.DriverID)
		if err != nil {
			logError(s.Logger, "rule driver resolve failed", err)
			continue
		}
		grouped[record.TenantID] = append(grouped[record.TenantID], core.Rule{
			ID:          record.ID,
			When:        record.When,
			Emit:        core.EmitList(record.Emit),
			DriverID:    record.DriverID,
			TransformJS: strings.TrimSpace(record.TransformJS),
			DriverName:  driverName,
		})
	}
	for id, rules := range grouped {
		normalized, err := core.NormalizeRules(rules)
		if err != nil {
			return err
		}
		if err := s.Engine.Update(core.RulesConfig{
			Rules:    normalized,
			Strict:   s.Strict,
			TenantID: id,
			Logger:   s.Logger,
		}); err != nil {
			return err
		}
	}
	return nil
}
