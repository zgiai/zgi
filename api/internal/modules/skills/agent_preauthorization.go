package skills

import (
	"encoding/json"
	"strings"
	"time"

	"github.com/zgiai/zgi/api/internal/capabilities/toolgovernance"
	"github.com/zgiai/zgi/api/internal/modules/tools"
)

const (
	agentAuthorizationSourceBinding = "agent_binding"

	agentBindingMissingCode        = "agent_binding_missing"
	agentResourceNotBoundCode      = "agent_resource_not_bound"
	agentDatabaseTableReadOnlyCode = "agent_database_table_read_only"
	agentToolNotPreauthorizedCode  = "agent_tool_not_preauthorized"
)

type agentDatabaseGovernanceBinding struct {
	DataSourceID     string   `json:"data_source_id"`
	TableIDs         []string `json:"table_ids"`
	WritableTableIDs []string `json:"writable_table_ids"`
}

type agentWorkflowGovernanceBinding struct {
	BindingID  string `json:"binding_id"`
	AgentID    string `json:"agent_id"`
	WorkflowID string `json:"workflow_id"`
}

func governanceAgentAuthorization(
	params map[string]interface{},
	governance map[string]interface{},
	skillID string,
	toolName string,
	manifest toolgovernance.Manifest,
	arguments map[string]interface{},
) (toolgovernance.ApprovalMode, *toolgovernance.Preauthorization) {
	callerType := governanceString(params, governance, "tool_governance_caller_type", "caller_type")
	if !strings.EqualFold(callerType, SkillCallerAgent) {
		return toolgovernance.ApprovalModeInteractive, nil
	}

	skillID = normalizeSkillID(skillID)
	switch skillID {
	case SkillAgentKnowledge:
		return toolgovernance.ApprovalModeNonInteractive, agentKnowledgePreauthorization(params, toolName)
	case SkillAgentDatabase:
		return toolgovernance.ApprovalModeNonInteractive, agentDatabasePreauthorization(params, toolName, manifest, arguments)
	case SkillAgentWorkflow:
		return toolgovernance.ApprovalModeNonInteractive, agentWorkflowPreauthorization(params, toolName, arguments)
	default:
		return toolgovernance.ApprovalModeNonInteractive, &toolgovernance.Preauthorization{
			Source: "agent_runtime",
			Code:   agentToolNotPreauthorizedCode,
			Reason: "the current Agent does not have persistent authorization for this tool action",
		}
	}
}

func agentKnowledgePreauthorization(params map[string]interface{}, toolName string) *toolgovernance.Preauthorization {
	datasetIDs := normalizeGovernanceIDs(stringSliceMapValue(params, "knowledge_dataset_ids"))
	resources := make([]toolgovernance.AssetRef, 0, len(datasetIDs))
	for _, datasetID := range datasetIDs {
		resources = append(resources, toolgovernance.AssetRef{ID: datasetID, Type: "knowledge_base"})
	}
	preauthorization := agentBindingPreauthorization(params, "knowledge", resources)
	if !agentKnowledgeBindingGrantValid(params, datasetIDs) || len(resources) == 0 {
		preauthorization.Code = agentBindingMissingCode
		preauthorization.Reason = "the current Agent has no valid knowledge binding authorization"
		return preauthorization
	}
	if strings.TrimSpace(toolName) != "retrieve_agent_knowledge" {
		preauthorization.Code = agentToolNotPreauthorizedCode
		preauthorization.Reason = "the current Agent knowledge binding does not authorize this tool action"
		return preauthorization
	}
	preauthorization.Matched = true
	applyCommonAgentBindingAuthorization(
		preauthorization,
		agentKnowledgeBindingAuthorizations(params, datasetIDs),
		len(resources),
	)
	preauthorization.Reason = "allowed by the current Agent knowledge binding"
	return preauthorization
}

func agentDatabasePreauthorization(
	params map[string]interface{},
	toolName string,
	manifest toolgovernance.Manifest,
	arguments map[string]interface{},
) *toolgovernance.Preauthorization {
	bindings := agentDatabaseGovernanceBindingsFromAny(params["database_bindings"])
	allResources := agentDatabaseGovernanceResources(bindings)
	preauthorization := agentBindingPreauthorization(params, "database", allResources)
	if !agentBindingGrantValid(params, "database") || len(allResources) == 0 {
		preauthorization.Code = agentBindingMissingCode
		preauthorization.Reason = "the current Agent has no valid database binding authorization"
		return preauthorization
	}

	dataSourceID := stringMapValue(arguments, "data_source_id", "dataSourceId", "database_id", "databaseId")
	tableID := stringMapValue(arguments, "table_id", "tableId", "database_table_id", "databaseTableId")
	matchedBinding, dataSourceBound := matchingAgentDatabaseBinding(bindings, dataSourceID)
	switch strings.TrimSpace(toolName) {
	case "list_accessible_databases":
		preauthorization.Matched = true
	case "list_database_tables":
		preauthorization.Matched = dataSourceBound && len(matchedBinding.TableIDs) > 0
	case "describe_database_table", "query_table_records", "insert_table_records", "update_table_records", "delete_table_records":
		preauthorization.Matched = dataSourceBound && containsString(matchedBinding.TableIDs, tableID)
	default:
		preauthorization.Code = agentToolNotPreauthorizedCode
		preauthorization.Reason = "the current Agent database binding does not authorize this tool action"
		return preauthorization
	}
	if !preauthorization.Matched {
		preauthorization.Code = agentResourceNotBoundCode
		preauthorization.Reason = "the requested database or table is not bound to the current Agent"
		return preauthorization
	}
	if manifest.Effect != toolgovernance.EffectRead && !containsString(matchedBinding.WritableTableIDs, tableID) {
		preauthorization.Matched = false
		preauthorization.Code = agentDatabaseTableReadOnlyCode
		preauthorization.Reason = "the requested database table is bound read-only for the current Agent"
		return preauthorization
	}
	multiResourceTool := strings.TrimSpace(toolName) == "list_accessible_databases" || strings.TrimSpace(toolName) == "list_database_tables"
	if authorization, ok := databaseAuthorizationForTool(params, toolName, dataSourceID, tableID, manifest); ok {
		if !multiResourceTool {
			applyAgentBindingAuthorization(preauthorization, authorization)
		}
	} else if !legacyAgentBindingGrantValid(params, "database") {
		preauthorization.Matched = false
		preauthorization.Code = agentBindingMissingCode
		preauthorization.Reason = "the requested database binding has no valid persistent authorization evidence"
		return preauthorization
	}

	switch strings.TrimSpace(toolName) {
	case "list_accessible_databases":
		applyCommonAgentBindingAuthorization(
			preauthorization,
			agentDatabaseTableBindingAuthorizations(params, bindings),
			len(preauthorization.Resources),
		)
	case "list_database_tables":
		preauthorization.Resources = agentDatabaseGovernanceResources([]agentDatabaseGovernanceBinding{matchedBinding})
		applyCommonAgentBindingAuthorization(
			preauthorization,
			agentDatabaseTableBindingAuthorizations(params, []agentDatabaseGovernanceBinding{matchedBinding}),
			len(preauthorization.Resources),
		)
	default:
		if tableID != "" {
			preauthorization.Resources = []toolgovernance.AssetRef{databaseTableAuthorizationResource(dataSourceID, tableID)}
		} else if dataSourceID != "" {
			preauthorization.Resources = []toolgovernance.AssetRef{{ID: dataSourceID, Type: "database"}}
		}
	}
	preauthorization.Reason = "allowed by the current Agent database binding"
	return preauthorization
}

func agentWorkflowPreauthorization(params map[string]interface{}, toolName string, arguments map[string]interface{}) *toolgovernance.Preauthorization {
	bindings := agentWorkflowGovernanceBindingsFromAny(params["workflow_bindings"])
	allResources := agentWorkflowGovernanceResources(bindings)
	preauthorization := agentBindingPreauthorization(params, "workflow", allResources)
	if !agentBindingGrantValid(params, "workflow") || len(allResources) == 0 {
		preauthorization.Code = agentBindingMissingCode
		preauthorization.Reason = "the current Agent has no valid workflow binding authorization"
		return preauthorization
	}

	switch strings.TrimSpace(toolName) {
	case "list_agent_workflows", "get_workflow_run_status":
		if !allWorkflowBindingGrantsValid(params, bindings) && !legacyAgentBindingGrantValid(params, "workflow") {
			preauthorization.Code = agentBindingMissingCode
			preauthorization.Reason = "the current Agent workflow bindings have no valid persistent authorization evidence"
			return preauthorization
		}
		preauthorization.Matched = true
		applyCommonAgentBindingAuthorization(
			preauthorization,
			agentWorkflowBindingAuthorizations(params, bindings),
			len(preauthorization.Resources),
		)
		preauthorization.Reason = "allowed by the current Agent workflow bindings"
		return preauthorization
	case "run_agent_workflow":
	default:
		preauthorization.Code = agentToolNotPreauthorizedCode
		preauthorization.Reason = "the current Agent workflow binding does not authorize this tool action"
		return preauthorization
	}
	bindingID := stringMapValue(arguments, "binding_id", "bindingId", "workflow_binding_id", "workflowBindingId")
	for _, binding := range bindings {
		if binding.BindingID != bindingID {
			continue
		}
		preauthorization.Matched = true
		if authorization, ok := tools.AgentBindingAuthorizationFor(params, "workflow", binding.AgentID, binding.BindingID, "execute"); ok {
			applyAgentBindingAuthorization(preauthorization, authorization)
		} else if !legacyAgentBindingGrantValid(params, "workflow") {
			preauthorization.Matched = false
			preauthorization.Code = agentBindingMissingCode
			preauthorization.Reason = "the requested workflow binding has no valid persistent authorization evidence"
			return preauthorization
		}
		preauthorization.Resources = []toolgovernance.AssetRef{workflowAuthorizationResource(binding)}
		preauthorization.Reason = "allowed by the current Agent workflow binding"
		return preauthorization
	}
	preauthorization.Code = agentResourceNotBoundCode
	preauthorization.Reason = "the requested workflow binding is not bound to the current Agent"
	return preauthorization
}

func agentBindingPreauthorization(params map[string]interface{}, bindingType string, resources []toolgovernance.AssetRef) *toolgovernance.Preauthorization {
	preauthorization := &toolgovernance.Preauthorization{
		Required:     true,
		Source:       agentAuthorizationSourceBinding,
		BindingType:  bindingType,
		AuthorizedBy: stringMapValue(params, bindingType+"_bound_by_account_id"),
		Resources:    resources,
	}
	if boundAtUnix := int64MapValue(params, bindingType+"_bound_at_unix"); boundAtUnix > 0 {
		authorizedAt := time.Unix(boundAtUnix, 0).UTC()
		preauthorization.AuthorizedAt = &authorizedAt
	}
	return preauthorization
}

func agentBindingGrantValid(params map[string]interface{}, bindingType string) bool {
	if legacyAgentBindingGrantValid(params, bindingType) {
		return true
	}
	for _, authorization := range agentBindingAuthorizationsForCategory(params, bindingType) {
		if authorization.BoundByAccountID != "" && authorization.BoundAtUnix > 0 {
			return true
		}
	}
	return false
}

func legacyAgentBindingGrantValid(params map[string]interface{}, bindingType string) bool {
	return boolMapValue(params, bindingType+"_binding_grant") &&
		stringMapValue(params, bindingType+"_bound_by_account_id") != "" &&
		int64MapValue(params, bindingType+"_bound_at_unix") > 0
}

func agentKnowledgeBindingGrantValid(params map[string]interface{}, datasetIDs []string) bool {
	if legacyAgentBindingGrantValid(params, "knowledge") {
		return true
	}
	return len(datasetIDs) > 0 && len(agentKnowledgeBindingAuthorizations(params, datasetIDs)) == len(datasetIDs)
}

func allWorkflowBindingGrantsValid(params map[string]interface{}, bindings []agentWorkflowGovernanceBinding) bool {
	if len(bindings) == 0 {
		return false
	}
	return len(agentWorkflowBindingAuthorizations(params, bindings)) == len(bindings)
}

func databaseAuthorizationForTool(
	params map[string]interface{},
	toolName string,
	dataSourceID string,
	tableID string,
	manifest toolgovernance.Manifest,
) (tools.AgentBindingAuthorization, bool) {
	switch strings.TrimSpace(toolName) {
	case "list_accessible_databases":
		return tools.AgentBindingAuthorization{}, len(tools.AgentBindingAuthorizationsForType(params, "database")) > 0
	case "list_database_tables":
		return tools.AgentBindingAuthorizationFor(params, "database", "", dataSourceID, "read")
	case "describe_database_table", "query_table_records", "insert_table_records", "update_table_records", "delete_table_records":
		accessMode := "read"
		if manifest.Effect != toolgovernance.EffectRead {
			accessMode = "write"
		}
		return tools.AgentBindingAuthorizationFor(params, "database_table", dataSourceID, tableID, accessMode)
	default:
		return tools.AgentBindingAuthorization{}, false
	}
}

func agentKnowledgeBindingAuthorizations(params map[string]interface{}, datasetIDs []string) []tools.AgentBindingAuthorization {
	authorizations := make([]tools.AgentBindingAuthorization, 0, len(datasetIDs))
	for _, datasetID := range datasetIDs {
		authorization, ok := tools.AgentBindingAuthorizationFor(params, "knowledge_dataset", "", datasetID, "read")
		if ok {
			authorizations = append(authorizations, authorization)
		}
	}
	return authorizations
}

func agentDatabaseTableBindingAuthorizations(params map[string]interface{}, bindings []agentDatabaseGovernanceBinding) []tools.AgentBindingAuthorization {
	var authorizations []tools.AgentBindingAuthorization
	for _, binding := range bindings {
		for _, tableID := range binding.TableIDs {
			authorization, ok := tools.AgentBindingAuthorizationFor(params, "database_table", binding.DataSourceID, tableID, "read")
			if ok {
				authorizations = append(authorizations, authorization)
			}
		}
	}
	return authorizations
}

func agentWorkflowBindingAuthorizations(params map[string]interface{}, bindings []agentWorkflowGovernanceBinding) []tools.AgentBindingAuthorization {
	authorizations := make([]tools.AgentBindingAuthorization, 0, len(bindings))
	for _, binding := range bindings {
		authorization, ok := tools.AgentBindingAuthorizationFor(params, "workflow", binding.AgentID, binding.BindingID, "execute")
		if ok {
			authorizations = append(authorizations, authorization)
		}
	}
	return authorizations
}

func agentBindingAuthorizationsForCategory(params map[string]interface{}, bindingType string) []tools.AgentBindingAuthorization {
	switch strings.TrimSpace(bindingType) {
	case "knowledge":
		return tools.AgentBindingAuthorizationsForType(params, "knowledge_dataset")
	case "database":
		return append(
			tools.AgentBindingAuthorizationsForType(params, "database"),
			tools.AgentBindingAuthorizationsForType(params, "database_table")...,
		)
	case "workflow":
		return tools.AgentBindingAuthorizationsForType(params, "workflow")
	default:
		return nil
	}
}

func applyAgentBindingAuthorization(preauthorization *toolgovernance.Preauthorization, authorization tools.AgentBindingAuthorization) {
	if preauthorization == nil {
		return
	}
	preauthorization.AuthorizedBy = authorization.BoundByAccountID
	if authorization.BoundAtUnix > 0 {
		authorizedAt := time.Unix(authorization.BoundAtUnix, 0).UTC()
		preauthorization.AuthorizedAt = &authorizedAt
	}
}

// applyCommonAgentBindingAuthorization only sets the top-level audit actor when
// every governed resource has the same persistent authorization evidence.
func applyCommonAgentBindingAuthorization(
	preauthorization *toolgovernance.Preauthorization,
	authorizations []tools.AgentBindingAuthorization,
	expectedResourceCount int,
) {
	if preauthorization == nil || len(authorizations) == 0 {
		return
	}
	preauthorization.AuthorizedBy = ""
	preauthorization.AuthorizedAt = nil
	if expectedResourceCount <= 0 || len(authorizations) != expectedResourceCount {
		return
	}
	common := authorizations[0]
	for _, authorization := range authorizations[1:] {
		if authorization.BoundByAccountID != common.BoundByAccountID || authorization.BoundAtUnix != common.BoundAtUnix {
			return
		}
	}
	applyAgentBindingAuthorization(preauthorization, common)
}

func agentDatabaseGovernanceBindingsFromAny(value interface{}) []agentDatabaseGovernanceBinding {
	var bindings []agentDatabaseGovernanceBinding
	if !decodeGovernanceBindings(value, &bindings) {
		return nil
	}
	out := make([]agentDatabaseGovernanceBinding, 0, len(bindings))
	indexes := make(map[string]int, len(bindings))
	for _, binding := range bindings {
		binding.DataSourceID = strings.TrimSpace(binding.DataSourceID)
		binding.TableIDs = normalizeGovernanceIDs(binding.TableIDs)
		binding.WritableTableIDs = normalizeGovernanceIDs(binding.WritableTableIDs)
		if binding.DataSourceID == "" || len(binding.TableIDs) == 0 {
			continue
		}
		if index, ok := indexes[binding.DataSourceID]; ok {
			out[index].TableIDs = normalizeGovernanceIDs(append(out[index].TableIDs, binding.TableIDs...))
			out[index].WritableTableIDs = normalizeGovernanceIDs(append(
				out[index].WritableTableIDs,
				intersectGovernanceIDs(binding.WritableTableIDs, out[index].TableIDs)...,
			))
			continue
		}
		indexes[binding.DataSourceID] = len(out)
		binding.WritableTableIDs = intersectGovernanceIDs(binding.WritableTableIDs, binding.TableIDs)
		out = append(out, binding)
	}
	return out
}

func agentWorkflowGovernanceBindingsFromAny(value interface{}) []agentWorkflowGovernanceBinding {
	var bindings []agentWorkflowGovernanceBinding
	if !decodeGovernanceBindings(value, &bindings) {
		return nil
	}
	out := make([]agentWorkflowGovernanceBinding, 0, len(bindings))
	seen := make(map[string]struct{}, len(bindings))
	for _, binding := range bindings {
		binding.BindingID = strings.TrimSpace(binding.BindingID)
		binding.AgentID = strings.TrimSpace(binding.AgentID)
		binding.WorkflowID = strings.TrimSpace(binding.WorkflowID)
		if binding.BindingID == "" || binding.AgentID == "" || binding.WorkflowID == "" {
			continue
		}
		if _, ok := seen[binding.BindingID]; ok {
			continue
		}
		seen[binding.BindingID] = struct{}{}
		out = append(out, binding)
	}
	return out
}

func decodeGovernanceBindings(value interface{}, target interface{}) bool {
	if value == nil {
		return false
	}
	payload, err := json.Marshal(value)
	if err != nil {
		return false
	}
	return json.Unmarshal(payload, target) == nil
}

func agentDatabaseGovernanceResources(bindings []agentDatabaseGovernanceBinding) []toolgovernance.AssetRef {
	var resources []toolgovernance.AssetRef
	for _, binding := range bindings {
		for _, tableID := range binding.TableIDs {
			resources = append(resources, databaseTableAuthorizationResource(binding.DataSourceID, tableID))
		}
	}
	return resources
}

func agentWorkflowGovernanceResources(bindings []agentWorkflowGovernanceBinding) []toolgovernance.AssetRef {
	resources := make([]toolgovernance.AssetRef, 0, len(bindings))
	for _, binding := range bindings {
		resources = append(resources, workflowAuthorizationResource(binding))
	}
	return resources
}

func databaseTableAuthorizationResource(dataSourceID string, tableID string) toolgovernance.AssetRef {
	return toolgovernance.AssetRef{
		ID:   tableID,
		Type: "database_table",
		Metadata: map[string]interface{}{
			"data_source_id": dataSourceID,
		},
	}
}

func workflowAuthorizationResource(binding agentWorkflowGovernanceBinding) toolgovernance.AssetRef {
	return toolgovernance.AssetRef{
		ID:   binding.BindingID,
		Type: "workflow",
		Metadata: map[string]interface{}{
			"agent_id":    binding.AgentID,
			"workflow_id": binding.WorkflowID,
		},
	}
}

func matchingAgentDatabaseBinding(bindings []agentDatabaseGovernanceBinding, dataSourceID string) (agentDatabaseGovernanceBinding, bool) {
	dataSourceID = strings.TrimSpace(dataSourceID)
	for _, binding := range bindings {
		if binding.DataSourceID == dataSourceID {
			return binding, true
		}
	}
	return agentDatabaseGovernanceBinding{}, false
}

func containsString(values []string, target string) bool {
	target = strings.TrimSpace(target)
	for _, value := range values {
		if value == target {
			return true
		}
	}
	return false
}

func normalizeGovernanceIDs(values []string) []string {
	seen := make(map[string]struct{}, len(values))
	out := make([]string, 0, len(values))
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value == "" {
			continue
		}
		if _, ok := seen[value]; ok {
			continue
		}
		seen[value] = struct{}{}
		out = append(out, value)
	}
	return out
}

func intersectGovernanceIDs(values []string, allowed []string) []string {
	out := make([]string, 0, len(values))
	for _, value := range values {
		if containsString(allowed, value) {
			out = append(out, value)
		}
	}
	return out
}

func int64MapValue(input map[string]interface{}, keys ...string) int64 {
	value := firstMapValue(input, keys...)
	switch typed := value.(type) {
	case int:
		return int64(typed)
	case int32:
		return int64(typed)
	case int64:
		return typed
	case uint:
		if uint64(typed) <= uint64(^uint64(0)>>1) {
			return int64(typed)
		}
	case uint32:
		return int64(typed)
	case uint64:
		if typed <= uint64(^uint64(0)>>1) {
			return int64(typed)
		}
	case float64:
		return int64(typed)
	case json.Number:
		parsed, _ := typed.Int64()
		return parsed
	}
	return 0
}
