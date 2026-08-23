package metatools

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
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

const (
	maxActionProjectionCandidates  = 128
	maxActionProjectionQueryRunes  = 512
	maxActionProjectionQueryTokens = 96
)

type preparedActionProjectionQuery struct {
	normalized string
	tokens     []string
}

// ActionProjectionPreparationHint is the server-authorized contract that
// relates one executable read Action to target values consumed by a projected
// Action. ResultPaths are evaluated only against the successful provider result
// of ActionID; they are never accepted from model-authored plan metadata.
type ActionProjectionPreparationHint struct {
	ActionID        string
	Relation        string
	TargetArguments []string
	ResultPaths     []string
	ResultTransform string `json:"result_transform,omitempty"`
}

// ActionProjection is a model-facing alias for one currently executable
// integration Action. Execution remains bound to external-apps/execute_action;
// the alias never carries a Connection identifier or credentials.
type ActionProjection struct {
	IntegrationID        string
	ActionID             string
	ConnectionID         string `json:"-"`
	ToolName             string
	Description          string
	InputSchema          map[string]interface{}
	SchemaHash           string
	SchemaRevision       string
	CatalogRevision      string
	Effect               string
	IntentMatched        bool
	IntentGroup          string
	IntentTokens         []string
	BindingFingerprint   string
	Pinned               bool
	ProjectionPriority   int `json:"-"`
	RequiresApproval     bool
	PreparationToolIDs   []string
	PreparationActionIDs []string
	PreparationHints     []ActionProjectionPreparationHint
	TargetArgumentPaths  []string
}

type ActionProjectionRequest struct {
	ExecutionContext skills.ExecutionContext
	Query            string
	PinnedActionKeys []string
}

type rankedActionProjection struct {
	projection ActionProjection
	score      int
	pinned     bool
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
	preparedQuery := prepareActionProjectionQuery(request.Query)
	mentionedProviders := actionProjectionMentionedProviders(preparedQuery, service.registry, integrationIDs)
	pinnedActionKeys := make(map[string]struct{}, len(request.PinnedActionKeys))
	for _, key := range request.PinnedActionKeys {
		if key = strings.ToLower(strings.TrimSpace(key)); key != "" {
			pinnedActionKeys[key] = struct{}{}
		}
	}
	ranked := make([]rankedActionProjection, 0)
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
			preparationActionIDs := projectionPreparationActionIDs(hints)
			preparationHints := projectionPreparationHints(hints)
			requiresApproval, _ := status["requires_approval"].(bool)
			score := preparedActionProjectionScore(preparedQuery, definition, action)
			intentMatched, intentTokens := actionProjectionIntentMatch(preparedQuery, definition, action)
			providerIntentTokens := mentionedProviders[integrationID]
			if len(mentionedProviders) > 0 && len(providerIntentTokens) == 0 {
				// Once the user names one or more providers, do not infer the same
				// operation for an unmentioned provider from generic verbs alone.
				intentMatched = false
				intentTokens = nil
			}
			intentGroup := actionProjectionIntentGroup(integrationID, action.ID)
			if intentMatched && len(providerIntentTokens) > 0 {
				intentGroup = strings.ToLower(strings.TrimSpace(integrationID)) + ":" + intentGroup
				intentTokens = appendUniqueProjectionStrings(intentTokens, providerIntentTokens...)
			}
			targetArgumentPaths := projectionTargetArgumentPaths(action)
			pinned := false
			if _, ok := pinnedActionKeys[actionProjectionActionKey(integrationID, action.ID)]; ok {
				pinned = true
			}
			projectionPriority := 0
			if intentMatched {
				projectionPriority = 1
			}
			if pinned {
				projectionPriority = 2
			}
			projection := ActionProjection{
				IntegrationID: integrationID,
				ActionID:      action.ID,
				ConnectionID:  preferred.record.ID.String(),
				ToolName:      action.ToolName,
				Description: directActionProjectionDescription(
					definition,
					action,
					hints,
					preparationToolIDs,
					requiresApproval,
				),
				InputSchema:          cloneMap(action.InputSchema),
				SchemaHash:           action.SchemaHash,
				SchemaRevision:       action.SchemaRevision,
				CatalogRevision:      action.CatalogRevision,
				Effect:               string(action.Effect),
				IntentMatched:        intentMatched,
				IntentGroup:          intentGroup,
				IntentTokens:         intentTokens,
				BindingFingerprint:   actionProjectionBindingFingerprint(integrationID, fmt.Sprint(preferred.record.ID), action, targetArgumentPaths, preparationActionIDs, preparationHints),
				Pinned:               pinned,
				ProjectionPriority:   projectionPriority,
				RequiresApproval:     requiresApproval,
				PreparationToolIDs:   preparationToolIDs,
				PreparationActionIDs: preparationActionIDs,
				PreparationHints:     preparationHints,
				TargetArgumentPaths:  targetArgumentPaths,
			}
			ranked = append(ranked, rankedActionProjection{
				projection: projection,
				score:      score,
				pinned:     pinned,
			})
		}
	}
	sort.SliceStable(ranked, func(i, j int) bool {
		if ranked[i].pinned != ranked[j].pinned {
			return ranked[i].pinned
		}
		if ranked[i].score != ranked[j].score {
			return ranked[i].score > ranked[j].score
		}
		left := ranked[i].projection.IntegrationID + "/" + ranked[i].projection.ActionID
		right := ranked[j].projection.IntegrationID + "/" + ranked[j].projection.ActionID
		return left < right
	})
	ranked = dependencyClosedActionProjectionCandidates(ranked, maxActionProjectionCandidates)
	out := make([]ActionProjection, 0, len(ranked))
	for _, item := range ranked {
		out = append(out, item.projection)
	}
	return out, nil
}

// dependencyClosedActionProjectionCandidates applies the candidate bound only
// after closing each selected Action over its server-declared preparation
// Actions. The input contains only projections that have already passed
// connection selection, caller ACL, scope, and policy checks. A dependency is
// eligible only when it resolved to the exact same selected connection as its
// target Action; this helper never synthesizes or broadens a capability.
func dependencyClosedActionProjectionCandidates(
	ranked []rankedActionProjection,
	limit int,
) []rankedActionProjection {
	if limit <= 0 || len(ranked) == 0 {
		return nil
	}
	if limit > len(ranked) {
		limit = len(ranked)
	}
	byAction := make(map[string]int, len(ranked))
	for index, item := range ranked {
		key := actionProjectionActionKey(item.projection.IntegrationID, item.projection.ActionID)
		if key != ":" {
			byAction[key] = index
		}
	}

	orderedRoots := make([]int, 0, len(ranked))
	for priority := 0; priority < 3; priority++ {
		for index, item := range ranked {
			matches := false
			switch priority {
			case 0:
				matches = item.pinned || item.projection.Pinned
			case 1:
				matches = !(item.pinned || item.projection.Pinned) && item.projection.IntentMatched
			default:
				matches = !(item.pinned || item.projection.Pinned) && !item.projection.IntentMatched
			}
			if matches {
				orderedRoots = append(orderedRoots, index)
			}
		}
	}

	selected := make(map[int]struct{}, limit)
	dependencyPriorities := make(map[int]int, limit)
	out := make([]rankedActionProjection, 0, limit)
	for _, rootIndex := range orderedRoots {
		group := actionProjectionDependencyGroup(ranked, byAction, rootIndex)
		newCount := 0
		for _, index := range group {
			if _, exists := selected[index]; !exists {
				newCount++
			}
		}
		// Keep a target and every eligible preparation dependency atomic. If
		// the group cannot fit, a later smaller group may still be projected.
		if len(out)+newCount > limit {
			continue
		}
		for _, index := range group {
			if _, exists := selected[index]; exists {
				continue
			}
			selected[index] = struct{}{}
			out = append(out, ranked[index])
		}
		rootPriority := ranked[rootIndex].projection.ProjectionPriority
		if ranked[rootIndex].projection.IntentMatched && rootPriority < 1 {
			rootPriority = 1
		}
		if (ranked[rootIndex].pinned || ranked[rootIndex].projection.Pinned) && rootPriority < 2 {
			rootPriority = 2
		}
		for _, index := range group {
			if dependencyPriorities[index] < rootPriority {
				dependencyPriorities[index] = rootPriority
			}
		}
		if len(out) == limit {
			break
		}
	}
	for index := range out {
		key := actionProjectionActionKey(out[index].projection.IntegrationID, out[index].projection.ActionID)
		originalIndex, exists := byAction[key]
		if !exists {
			continue
		}
		if out[index].projection.ProjectionPriority < dependencyPriorities[originalIndex] {
			out[index].projection.ProjectionPriority = dependencyPriorities[originalIndex]
		}
	}
	return out
}

func actionProjectionDependencyGroup(
	ranked []rankedActionProjection,
	byAction map[string]int,
	rootIndex int,
) []int {
	if rootIndex < 0 || rootIndex >= len(ranked) {
		return nil
	}
	root := ranked[rootIndex].projection
	integrationID := strings.ToLower(strings.TrimSpace(root.IntegrationID))
	connectionID := strings.TrimSpace(root.ConnectionID)
	seen := map[int]struct{}{}
	group := make([]int, 0, 1+len(root.PreparationActionIDs))
	var visit func(int)
	visit = func(index int) {
		if index < 0 || index >= len(ranked) {
			return
		}
		if _, exists := seen[index]; exists {
			return
		}
		candidate := ranked[index].projection
		if !strings.EqualFold(strings.TrimSpace(candidate.IntegrationID), integrationID) ||
			strings.TrimSpace(candidate.ConnectionID) != connectionID {
			return
		}
		seen[index] = struct{}{}
		group = append(group, index)
		for _, dependencyActionID := range candidate.PreparationActionIDs {
			dependencyIndex, exists := byAction[actionProjectionActionKey(integrationID, dependencyActionID)]
			if exists {
				visit(dependencyIndex)
			}
		}
	}
	visit(rootIndex)
	return group
}

func actionProjectionMentionedProviders(
	query preparedActionProjectionQuery,
	registry *integrations.Registry,
	integrationIDs []string,
) map[string][]string {
	out := map[string][]string{}
	if registry == nil || query.normalized == "" {
		return out
	}
	for _, integrationID := range integrationIDs {
		definition, ok := registry.ProviderDefinition(integrationID)
		if !ok {
			continue
		}
		if tokens := actionProjectionProviderIntentTokens(query, definition); len(tokens) > 0 {
			out[integrationID] = tokens
		}
	}
	return out
}

func actionProjectionProviderIntentTokens(
	query preparedActionProjectionQuery,
	definition integrations.ProviderDefinition,
) []string {
	providerFields := []string{definition.ID, definition.Name}
	providerFields = append(providerFields, sortedLocalizedProjectionValues(definition.NameI18n)...)
	providerHaystack := strings.ToLower(strings.Join(providerFields, " "))
	tokens := make([]string, 0, 2)
	for _, token := range query.tokens {
		if strings.Contains(providerHaystack, token) {
			tokens = appendUniqueProjectionStrings(tokens, token)
		}
	}
	return tokens
}

func appendUniqueProjectionStrings(values []string, additions ...string) []string {
	seen := make(map[string]struct{}, len(values)+len(additions))
	for _, value := range values {
		seen[strings.ToLower(strings.TrimSpace(value))] = struct{}{}
	}
	for _, value := range additions {
		value = strings.ToLower(strings.TrimSpace(value))
		if value == "" {
			continue
		}
		if _, exists := seen[value]; exists {
			continue
		}
		seen[value] = struct{}{}
		values = append(values, value)
	}
	return values
}

func actionProjectionActionKey(integrationID string, actionID string) string {
	return strings.ToLower(strings.TrimSpace(integrationID)) + ":" + strings.ToLower(strings.TrimSpace(actionID))
}

func actionProjectionIntentGroup(integrationID string, actionID string) string {
	integrationID = strings.ToLower(strings.TrimSpace(integrationID))
	actionID = strings.ToLower(strings.TrimSpace(actionID))
	actionID = strings.TrimPrefix(actionID, integrationID+".")
	parts := strings.Split(actionID, ".")
	if len(parts) == 0 {
		return actionID
	}
	last := parts[len(parts)-1]
	if strings.HasPrefix(last, "send_") || strings.HasPrefix(last, "notify_") {
		parts[len(parts)-1] = strings.SplitN(last, "_", 2)[0]
	}
	return strings.Join(parts, ".")
}

func actionProjectionBindingFingerprint(
	integrationID string,
	connectionID string,
	action integrations.ActionDefinition,
	targetArgumentPaths []string,
	preparationActionIDs []string,
	preparationHints []ActionProjectionPreparationHint,
) string {
	payload := struct {
		IntegrationID       string                            `json:"integration_id"`
		ConnectionID        string                            `json:"connection_id"`
		ActionID            string                            `json:"action_id"`
		SchemaHash          string                            `json:"schema_hash"`
		SchemaRevision      string                            `json:"schema_revision"`
		CatalogRevision     string                            `json:"catalog_revision"`
		Effect              string                            `json:"effect"`
		TargetArgumentPaths []string                          `json:"target_argument_paths"`
		PreparationActions  []string                          `json:"preparation_action_ids"`
		PreparationHints    []ActionProjectionPreparationHint `json:"preparation_hints"`
	}{
		IntegrationID:       strings.ToLower(strings.TrimSpace(integrationID)),
		ConnectionID:        strings.ToLower(strings.TrimSpace(connectionID)),
		ActionID:            strings.ToLower(strings.TrimSpace(action.ID)),
		SchemaHash:          strings.TrimSpace(action.SchemaHash),
		SchemaRevision:      strings.TrimSpace(action.SchemaRevision),
		CatalogRevision:     strings.TrimSpace(action.CatalogRevision),
		Effect:              strings.ToLower(strings.TrimSpace(string(action.Effect))),
		TargetArgumentPaths: append([]string(nil), targetArgumentPaths...),
		PreparationActions:  append([]string(nil), preparationActionIDs...),
		PreparationHints:    cloneActionProjectionPreparationHints(preparationHints),
	}
	encoded, err := json.Marshal(payload)
	if err != nil {
		return ""
	}
	digest := sha256.Sum256(encoded)
	return hex.EncodeToString(digest[:])
}

func projectionPreparationHints(hints []interface{}) []ActionProjectionPreparationHint {
	out := make([]ActionProjectionPreparationHint, 0, len(hints))
	for _, raw := range hints {
		hint, _ := raw.(map[string]interface{})
		actionID := strings.ToLower(strings.TrimSpace(normalizedString(hint["action_id"])))
		relation := strings.ToLower(strings.TrimSpace(normalizedString(hint["relation"])))
		if actionID == "" || relation == "" {
			continue
		}
		targetArguments := orderedUniqueProjectionStrings(stringSlice(hint["target_arguments"]), 8)
		resultPaths := orderedUniqueProjectionStrings(stringSlice(hint["result_paths"]), 16)
		resultTransform := strings.ToLower(strings.TrimSpace(normalizedString(hint["result_transform"])))
		if len(targetArguments) == 0 || len(resultPaths) == 0 {
			continue
		}
		if resultTransform == "" && len(targetArguments) != 1 {
			continue
		}
		if resultTransform != "" && (resultTransform != string(integrations.ActionPreparationSplitSlashPair) || len(targetArguments) != 2 || len(resultPaths) != 1) {
			continue
		}
		out = append(out, ActionProjectionPreparationHint{
			ActionID: actionID, Relation: relation,
			TargetArguments: targetArguments, ResultPaths: resultPaths, ResultTransform: resultTransform,
		})
	}
	sort.SliceStable(out, func(i, j int) bool {
		if out[i].ActionID == out[j].ActionID {
			return out[i].Relation < out[j].Relation
		}
		return out[i].ActionID < out[j].ActionID
	})
	return out
}

func orderedUniqueProjectionStrings(values []string, limit int) []string {
	seen := map[string]struct{}{}
	out := make([]string, 0, min(len(values), limit))
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value == "" {
			continue
		}
		if _, exists := seen[value]; exists {
			continue
		}
		seen[value] = struct{}{}
		out = append(out, value)
		if len(out) == limit {
			break
		}
	}
	return out
}

func cloneActionProjectionPreparationHints(input []ActionProjectionPreparationHint) []ActionProjectionPreparationHint {
	out := make([]ActionProjectionPreparationHint, 0, len(input))
	for _, hint := range input {
		cloned := hint
		cloned.TargetArguments = append([]string(nil), hint.TargetArguments...)
		cloned.ResultPaths = append([]string(nil), hint.ResultPaths...)
		out = append(out, cloned)
	}
	return out
}

func projectionPreparationActionIDs(hints []interface{}) []string {
	seen := map[string]struct{}{}
	out := make([]string, 0, len(hints))
	for _, raw := range hints {
		hint, _ := raw.(map[string]interface{})
		actionID := strings.ToLower(strings.TrimSpace(normalizedString(hint["action_id"])))
		if actionID == "" {
			continue
		}
		if _, exists := seen[actionID]; exists {
			continue
		}
		seen[actionID] = struct{}{}
		out = append(out, actionID)
	}
	sort.Strings(out)
	return out
}

func projectionTargetArgumentPaths(action integrations.ActionDefinition) []string {
	seen := map[string]struct{}{}
	out := make([]string, 0)
	appendPath := func(path string) {
		path = strings.TrimSpace(path)
		if path == "" {
			return
		}
		if _, exists := seen[path]; exists {
			return
		}
		seen[path] = struct{}{}
		out = append(out, path)
	}
	if action.SuccessDeduplication != nil {
		for _, path := range action.SuccessDeduplication.TargetArgumentPaths {
			appendPath(path)
		}
	}
	for _, hint := range action.PreparationHints {
		for _, path := range hint.TargetArguments {
			appendPath(path)
		}
	}
	sort.Strings(out)
	return out
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
		parts = append(parts, "If a required target argument is unknown, use an available visible read Action first; if none is visible, use the visible external-action guide or search fallback before execution.")
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
	return preparedActionProjectionScore(prepareActionProjectionQuery(query), definition, action)
}

func prepareActionProjectionQuery(query string) preparedActionProjectionQuery {
	normalized := []rune(strings.ToLower(strings.TrimSpace(query)))
	if len(normalized) > maxActionProjectionQueryRunes {
		normalized = normalized[:maxActionProjectionQueryRunes]
	}
	value := strings.TrimSpace(string(normalized))
	return preparedActionProjectionQuery{
		normalized: value,
		tokens:     actionProjectionSearchTokens(value, maxActionProjectionQueryTokens),
	}
}

func preparedActionProjectionScore(query preparedActionProjectionQuery, definition integrations.ProviderDefinition, action integrations.ActionDefinition) int {
	if query.normalized == "" {
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
	if strings.Contains(haystack, query.normalized) {
		score += 1000
	}
	for _, token := range query.tokens {
		if strings.Contains(haystack, token) {
			score += 20 + utf8.RuneCountInString(token)
		}
	}
	return score
}

func actionProjectionIntentMatched(
	query preparedActionProjectionQuery,
	definition integrations.ProviderDefinition,
	action integrations.ActionDefinition,
) bool {
	matched, _ := actionProjectionIntentMatch(query, definition, action)
	return matched
}

func actionProjectionIntentMatch(
	query preparedActionProjectionQuery,
	definition integrations.ProviderDefinition,
	action integrations.ActionDefinition,
) (bool, []string) {
	if query.normalized == "" {
		return false, nil
	}
	providerFields := []string{definition.ID, definition.Name}
	providerFields = append(providerFields, sortedLocalizedProjectionValues(definition.NameI18n)...)
	providerHaystack := strings.ToLower(strings.Join(providerFields, " "))
	actionFields := []string{action.Name, action.Description}
	actionFields = append(actionFields, sortedLocalizedProjectionValues(action.NameI18n)...)
	actionFields = append(actionFields, sortedLocalizedProjectionValues(action.DescriptionI18n)...)
	actionHaystack := strings.ToLower(strings.Join(actionFields, " "))
	// An exact action phrase is sufficient. Otherwise require both a provider
	// identity token and a distinct action-capability token. This prevents a
	// generic internal request such as "create a file" from turning an available
	// calendar.create projection into external-operation intent merely because
	// both descriptions contain the verb "create".
	if strings.Contains(actionHaystack, query.normalized) {
		return true, []string{query.normalized}
	}
	providerMatched := false
	actionSpecificTokens := map[string]struct{}{}
	for _, token := range query.tokens {
		inProvider := strings.Contains(providerHaystack, token)
		inAction := strings.Contains(actionHaystack, token)
		providerMatched = providerMatched || inProvider
		if inAction && !inProvider {
			actionSpecificTokens[token] = struct{}{}
		}
	}
	tokens := make([]string, 0, len(actionSpecificTokens))
	for token := range actionSpecificTokens {
		tokens = append(tokens, token)
	}
	sort.Strings(tokens)
	return len(tokens) >= 2 || (providerMatched && len(tokens) > 0), tokens
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

func actionProjectionSearchTokens(value string, limit int) []string {
	if limit <= 0 || limit > maxActionProjectionQueryTokens {
		limit = maxActionProjectionQueryTokens
	}
	seen := map[string]struct{}{}
	out := make([]string, 0)
	appendToken := func(token string) {
		if len(out) >= limit {
			return
		}
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
		if len(out) >= limit {
			break
		}
		runes := []rune(token)
		if len(runes) > 2 && !allActionProjectionASCII(runes) {
			for index := 0; index+1 < len(runes); index++ {
				appendToken(string(runes[index : index+2]))
				if len(out) >= limit {
					break
				}
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
