package metatools

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/google/uuid"
	"github.com/zgiai/zgi/api/internal/modules/integrations"
	"github.com/zgiai/zgi/api/internal/modules/skills"
	"github.com/zgiai/zgi/api/internal/modules/tools"
)

const maxActionProjectionCandidates = 128

// ActionProjection is a model-facing alias for one currently executable
// integration Action. Execution remains bound to external-apps/execute_action;
// the alias never carries a Connection identifier or credentials.
type ActionProjection struct {
	IntegrationID      string
	ActionID           string
	ToolName           string
	Description        string
	InputSchema        map[string]interface{}
	SchemaHash         string
	SchemaRevision     string
	CatalogRevision    string
	RequiresApproval   bool
	PreparationToolIDs []string
}

type ActionProjectionRequest struct {
	ExecutionContext skills.ExecutionContext
	Query            string
}

type ActionProjectionResolver interface {
	ProjectActions(context.Context, ActionProjectionRequest) ([]ActionProjection, error)
}

// ActionProjectionService reuses the same selected-connection, ACL, scope,
// caller, and policy gates as the external-apps catalog tools. It is read-only
// and does not hold an Action executor.
type ActionProjectionService struct {
	registry    *integrations.Registry
	connections ConnectionLookup
	access      ConnectionAuthorizer
	policies    integrations.ActionPolicyResolver
}

func NewActionProjectionService(
	registry *integrations.Registry,
	connections ConnectionLookup,
	access ConnectionAuthorizer,
	policies integrations.ActionPolicyResolver,
) (*ActionProjectionService, error) {
	if registry == nil || connections == nil || access == nil || policies == nil {
		return nil, fmt.Errorf("external Action projection requires registry, connection lookup, access authorizer, and action policy resolver")
	}
	return &ActionProjectionService{
		registry: registry, connections: connections, access: access, policies: policies,
	}, nil
}

func (service *ActionProjectionService) ProjectActions(
	ctx context.Context,
	request ActionProjectionRequest,
) ([]ActionProjection, error) {
	if service == nil || service.registry == nil || service.connections == nil || service.access == nil || service.policies == nil {
		return nil, fmt.Errorf("external Action projection service is unavailable")
	}
	execution := request.ExecutionContext
	if execution.InvokeFrom != tools.ToolInvokeFromAIChat && execution.InvokeFrom != tools.ToolInvokeFromAgent {
		return nil, nil
	}
	tool := &Tool{
		registry: service.registry, connections: service.connections, access: service.access, policies: service.policies,
		runtime: &tools.ToolRuntime{
			TenantID: strings.TrimSpace(execution.OrganizationID), InvokeFrom: execution.InvokeFrom,
			RuntimeParameters: cloneMap(execution.RuntimeParameters),
		},
	}
	connections, err := tool.availableConnections(ctx, execution.UserID)
	if err != nil {
		return nil, err
	}
	byIntegration := groupConnectionsByIntegration(connections)
	integrationIDs := sortedKeys(byIntegration)
	projectionPolicies, policyErr := newActionProjectionPolicyResolver(
		ctx,
		service.policies,
		execution.OrganizationID,
		integrationIDs,
	)
	if policyErr != nil {
		return nil, policyErr
	}
	tool.policies = projectionPolicies
	type rankedProjection struct {
		projection ActionProjection
		score      int
	}
	ranked := make([]rankedProjection, 0)
	for _, integrationID := range integrationIDs {
		definition, ok := service.registry.ProviderDefinition(integrationID)
		if !ok {
			continue
		}
		for _, action := range service.registry.Actions(integrationID) {
			if !supportsCaller(action, execution.InvokeFrom) {
				continue
			}
			preferred, resolveErr := tool.resolvePreferredFromAvailable(
				ctx,
				execution.UserID,
				integrationID,
				action.ID,
				action.Effect,
				byIntegration[integrationID],
			)
			if resolveErr != nil {
				continue
			}
			status := actionSummaryOutput(
				actionSummary(definition, action),
				preferred.record,
				tool.connectionAuthScopeEvidence(preferred.record),
			)
			if policyErr := tool.applyActionPolicyStatus(ctx, execution.UserID, integrationID, action, status); policyErr != nil {
				return nil, policyErr
			}
			canExecute, _ := status["can_execute"].(bool)
			if !canExecute {
				continue
			}
			hints := tool.preparationHintsOutput(ctx, execution.UserID, action, preferred.record)
			preparationToolIDs := projectionPreparationToolIDs(service.registry, integrationID, hints)
			requiresApproval, _ := status["requires_approval"].(bool)
			projection := ActionProjection{
				IntegrationID: integrationID,
				ActionID:      action.ID,
				ToolName:      action.ToolName,
				Description: directActionProjectionDescription(
					definition,
					action,
					hints,
					preparationToolIDs,
					requiresApproval,
				),
				InputSchema:        cloneMap(action.InputSchema),
				SchemaHash:         action.SchemaHash,
				SchemaRevision:     action.SchemaRevision,
				CatalogRevision:    action.CatalogRevision,
				RequiresApproval:   requiresApproval,
				PreparationToolIDs: preparationToolIDs,
			}
			ranked = append(ranked, rankedProjection{
				projection: projection,
				score:      actionProjectionScore(request.Query, definition, action),
			})
		}
	}
	sort.SliceStable(ranked, func(i, j int) bool {
		if ranked[i].score != ranked[j].score {
			return ranked[i].score > ranked[j].score
		}
		left := ranked[i].projection.IntegrationID + "/" + ranked[i].projection.ActionID
		right := ranked[j].projection.IntegrationID + "/" + ranked[j].projection.ActionID
		return left < right
	})
	if len(ranked) > maxActionProjectionCandidates {
		ranked = ranked[:maxActionProjectionCandidates]
	}
	out := make([]ActionProjection, 0, len(ranked))
	for _, item := range ranked {
		out = append(out, item.projection)
	}
	return out, nil
}

type actionProjectionPolicyLister interface {
	List(context.Context, uuid.UUID, string) ([]integrations.ActionPolicyView, error)
}

type actionProjectionPolicyResolver struct {
	fallback  integrations.ActionPolicyResolver
	decisions map[string]integrations.ActionPolicyDecision
}

func newActionProjectionPolicyResolver(
	ctx context.Context,
	fallback integrations.ActionPolicyResolver,
	organizationID string,
	integrationIDs []string,
) (integrations.ActionPolicyResolver, error) {
	lister, ok := fallback.(actionProjectionPolicyLister)
	if !ok || len(integrationIDs) == 0 {
		return fallback, nil
	}
	parsedOrganizationID, err := uuid.Parse(strings.TrimSpace(organizationID))
	if err != nil || parsedOrganizationID == uuid.Nil {
		return nil, integrations.NewError(integrations.ErrorCodeInvalidInput, "integration invocation identity is invalid", err)
	}
	resolver := &actionProjectionPolicyResolver{
		fallback:  fallback,
		decisions: map[string]integrations.ActionPolicyDecision{},
	}
	for _, integrationID := range integrationIDs {
		views, listErr := lister.List(ctx, parsedOrganizationID, integrationID)
		if listErr != nil {
			return nil, listErr
		}
		for _, view := range views {
			key := actionProjectionPolicyKey(view.IntegrationID, view.ActionID)
			if key == "/" {
				continue
			}
			resolver.decisions[key] = integrations.ActionPolicyDecision{
				Enabled: view.Enabled, ApprovalPolicy: view.ApprovalPolicy, DataEgressAllowed: view.DataEgressAllowed,
			}
		}
	}
	return resolver, nil
}

func (resolver *actionProjectionPolicyResolver) Resolve(
	ctx context.Context,
	organizationID string,
	integrationID string,
	action integrations.ActionDefinition,
) (integrations.ActionPolicyDecision, error) {
	if resolver == nil {
		return integrations.ActionPolicyDecision{}, fmt.Errorf("integration action policy resolver is unavailable")
	}
	if decision, ok := resolver.decisions[actionProjectionPolicyKey(integrationID, action.ID)]; ok {
		return decision, nil
	}
	return resolver.fallback.Resolve(ctx, organizationID, integrationID, action)
}

func actionProjectionPolicyKey(integrationID string, actionID string) string {
	return strings.ToLower(strings.TrimSpace(integrationID)) + "/" + strings.ToLower(strings.TrimSpace(actionID))
}

func projectionPreparationToolIDs(
	registry *integrations.Registry,
	integrationID string,
	hints []interface{},
) []string {
	if registry == nil || len(hints) == 0 {
		return nil
	}
	seen := map[string]struct{}{}
	out := make([]string, 0, len(hints))
	for _, raw := range hints {
		hint, _ := raw.(map[string]interface{})
		actionID := normalizedString(hint["action_id"])
		action, ok := registry.ActionDetail(integrationID, actionID)
		toolName := strings.TrimSpace(action.ToolName)
		if !ok || toolName == "" {
			continue
		}
		if _, exists := seen[toolName]; exists {
			continue
		}
		seen[toolName] = struct{}{}
		out = append(out, toolName)
	}
	sort.Strings(out)
	return out
}

func directActionProjectionDescription(
	definition integrations.ProviderDefinition,
	action integrations.ActionDefinition,
	hints []interface{},
	preparationToolIDs []string,
	requiresApproval bool,
) string {
	parts := []string{
		"Directly execute the " + strings.TrimSpace(definition.Name) + " Action " + strings.TrimSpace(action.Name) + ".",
		strings.TrimSpace(action.Description),
		"Pass only the business arguments declared by this function; the server binds the integration, Action revision, and preferred selected connection.",
	}
	if requiresApproval {
		parts = append(parts, "This Action may pause for explicit user approval before execution.")
	}
	if len(preparationToolIDs) > 0 {
		parts = append(parts, "Resolve required targets first with: "+strings.Join(preparationToolIDs, ", ")+".")
	}
	for _, raw := range hints {
		hint, _ := raw.(map[string]interface{})
		if description := strings.TrimSpace(stringValue(hint["description"])); description != "" {
			parts = append(parts, description)
		}
	}
	return boundedString(strings.Join(nonEmptyProjectionParts(parts), " "), 2400)
}

func nonEmptyProjectionParts(parts []string) []string {
	out := make([]string, 0, len(parts))
	for _, part := range parts {
		if part = strings.TrimSpace(part); part != "" {
			out = append(out, part)
		}
	}
	return out
}

func actionProjectionScore(query string, definition integrations.ProviderDefinition, action integrations.ActionDefinition) int {
	query = strings.ToLower(strings.TrimSpace(query))
	if query == "" {
		return 0
	}
	fields := []string{
		definition.ID,
		definition.Name,
		action.ID,
		action.ToolName,
		action.Name,
		action.Description,
	}
	fields = append(fields, sortedLocalizedProjectionValues(definition.NameI18n)...)
	fields = append(fields, sortedLocalizedProjectionValues(definition.DescriptionI18n)...)
	fields = append(fields, sortedLocalizedProjectionValues(action.NameI18n)...)
	fields = append(fields, sortedLocalizedProjectionValues(action.DescriptionI18n)...)
	haystack := strings.ToLower(strings.Join(fields, " "))
	score := 0
	if strings.Contains(haystack, query) {
		score += 1000
	}
	for _, token := range actionProjectionSearchTokens(query) {
		if strings.Contains(haystack, token) {
			score += 20 + utf8.RuneCountInString(token)
		}
	}
	return score
}

func sortedLocalizedProjectionValues(values integrations.LocalizedText) []string {
	if len(values) == 0 {
		return nil
	}
	locales := make([]string, 0, len(values))
	for locale := range values {
		locales = append(locales, locale)
	}
	sort.Strings(locales)
	out := make([]string, 0, len(locales))
	for _, locale := range locales {
		if value := strings.TrimSpace(values[locale]); value != "" {
			out = append(out, value)
		}
	}
	return out
}

func actionProjectionSearchTokens(value string) []string {
	seen := map[string]struct{}{}
	out := make([]string, 0)
	appendToken := func(token string) {
		token = strings.TrimFunc(strings.ToLower(token), func(current rune) bool {
			return unicode.IsSpace(current) || unicode.IsPunct(current)
		})
		if utf8.RuneCountInString(token) < 2 {
			return
		}
		if _, ok := seen[token]; ok {
			return
		}
		seen[token] = struct{}{}
		out = append(out, token)
	}
	for _, token := range strings.FieldsFunc(value, func(current rune) bool {
		return unicode.IsSpace(current) || unicode.IsPunct(current)
	}) {
		appendToken(token)
		runes := []rune(token)
		if len(runes) > 2 && !allActionProjectionASCII(runes) {
			for index := 0; index+1 < len(runes); index++ {
				appendToken(string(runes[index : index+2]))
			}
		}
	}
	return out
}

func allActionProjectionASCII(value []rune) bool {
	for _, current := range value {
		if current > unicode.MaxASCII {
			return false
		}
	}
	return true
}
