package skills

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/zgiai/zgi/api/internal/capabilities/toolgovernance"
)

const (
	governanceRuntimeParametersKey = "tool_governance"
	governancePermissionTierKey    = "tool_governance_permission_tier"
	governanceAssetsKey            = "tool_governance_assets"
	governanceSessionGrantsKey     = "tool_governance_session_grants"
	governanceCorrelationIDKey     = "tool_governance_correlation_id"
	agentBindingVerifierKey        = "_agent_binding_verifier"
)

// AgentBindingCheck identifies one persistent Agent resource authorization
// that must still exist immediately before a governed tool invocation.
type AgentBindingCheck struct {
	BindingType      string
	ResourceID       string
	ParentResourceID string
	AccessMode       string
}

type AgentBindingVerifier func(context.Context, AgentBindingCheck) (bool, error)

// WithAgentBindingVerifier attaches an in-process verifier to a single runtime
// request. It is intentionally not serializable and never leaves the backend.
func WithAgentBindingVerifier(params map[string]interface{}, verifier AgentBindingVerifier) map[string]interface{} {
	if params == nil {
		params = map[string]interface{}{}
	}
	if verifier != nil {
		params[agentBindingVerifierKey] = verifier
	}
	return params
}

type PolicyToolGovernanceGateway struct {
	policy toolgovernance.Policy
}

func NewPolicyToolGovernanceGateway(policy toolgovernance.Policy) *PolicyToolGovernanceGateway {
	return &PolicyToolGovernanceGateway{policy: policy}
}

func (g *PolicyToolGovernanceGateway) DecideSkillTool(ctx context.Context, req ToolGovernanceRequest) (toolgovernance.Decision, error) {
	_ = ctx
	if g == nil {
		return toolgovernance.Decision{}, fmt.Errorf("tool governance gateway is not configured")
	}
	params := req.ExecutionContext.RuntimeParameters
	governance := governanceRuntimeParameters(params)
	manifest := governanceManifestForArguments(req.Manifest, req.SkillID, req.ToolName, req.Arguments, params)
	approvalMode, preauthorization := governanceAgentAuthorization(params, governance, req.SkillID, req.ToolName, manifest, req.Arguments)
	preauthorization = verifyCurrentAgentBindings(ctx, params, manifest, preauthorization)
	return toolgovernance.Decide(toolgovernance.Request{
		Manifest:         manifest,
		PermissionTier:   governancePermissionTier(params, governance),
		ApprovalMode:     approvalMode,
		Preauthorization: preauthorization,
		ConversationID:   req.ExecutionContext.ConversationID,
		OrganizationID:   req.ExecutionContext.OrganizationID,
		UserID:           req.ExecutionContext.UserID,
		SkillID:          req.SkillID,
		ProviderType:     string(req.ProviderType),
		ProviderID:       req.ProviderID,
		Assets:           governanceAssets(params, governance, manifest, req.Arguments),
		ExpectedAssets:   governanceExpectedAssets(params, governance, manifest),
		SessionGrants:    governanceSessionGrants(params, governance),
		CorrelationID:    governanceString(params, governance, governanceCorrelationIDKey, "correlation_id"),
	}, g.policy), nil
}

func verifyCurrentAgentBindings(
	ctx context.Context,
	params map[string]interface{},
	manifest toolgovernance.Manifest,
	preauthorization *toolgovernance.Preauthorization,
) *toolgovernance.Preauthorization {
	if preauthorization == nil || !preauthorization.Required || !preauthorization.Matched ||
		preauthorization.Source != agentAuthorizationSourceBinding {
		return preauthorization
	}
	verifier, _ := params[agentBindingVerifierKey].(AgentBindingVerifier)
	if verifier == nil {
		return preauthorization
	}
	checks := agentBindingChecks(preauthorization, manifest)
	if len(checks) == 0 {
		return deniedCurrentAgentBinding(preauthorization)
	}
	for _, check := range checks {
		matched, err := verifier(ctx, check)
		if err != nil || !matched {
			return deniedCurrentAgentBinding(preauthorization)
		}
	}
	return preauthorization
}

func deniedCurrentAgentBinding(preauthorization *toolgovernance.Preauthorization) *toolgovernance.Preauthorization {
	denied := *preauthorization
	denied.Matched = false
	denied.Code = agentResourceNotBoundCode
	denied.Reason = "the requested resource is no longer bound to the current Agent"
	return &denied
}

func agentBindingChecks(
	preauthorization *toolgovernance.Preauthorization,
	manifest toolgovernance.Manifest,
) []AgentBindingCheck {
	accessMode := "read"
	if manifest.Effect != toolgovernance.EffectRead {
		accessMode = "write"
	}
	out := make([]AgentBindingCheck, 0, len(preauthorization.Resources))
	for _, resource := range preauthorization.Resources {
		check := AgentBindingCheck{ResourceID: strings.TrimSpace(resource.ID), AccessMode: accessMode}
		switch preauthorization.BindingType {
		case "knowledge":
			check.BindingType = "knowledge_dataset"
		case "database":
			if strings.EqualFold(strings.TrimSpace(resource.Type), "database_table") {
				check.BindingType = "database_table"
				check.ParentResourceID = strings.TrimSpace(fmt.Sprint(resource.Metadata["data_source_id"]))
			} else {
				check.BindingType = "database"
			}
		case "workflow":
			check.BindingType = "workflow"
			check.AccessMode = "execute"
			check.ParentResourceID = stringMapValue(resource.Metadata, "agent_id", "workflow_id")
		}
		if check.BindingType == "" || check.ResourceID == "" {
			continue
		}
		out = append(out, check)
	}
	return out
}

func governanceRuntimeParameters(params map[string]interface{}) map[string]interface{} {
	if len(params) == 0 {
		return nil
	}
	if nested, ok := params[governanceRuntimeParametersKey].(map[string]interface{}); ok {
		return nested
	}
	return nil
}

func governancePermissionTier(params map[string]interface{}, governance map[string]interface{}) toolgovernance.PermissionTier {
	return toolgovernance.PermissionTier(governanceString(params, governance, governancePermissionTierKey, "permission_tier"))
}

func governanceManifestForArguments(manifest toolgovernance.Manifest, skillID, toolName string, arguments map[string]interface{}, runtimeParams map[string]interface{}) toolgovernance.Manifest {
	_ = arguments
	_ = runtimeParams
	if isFileGeneratorTool(skillID, toolName) {
		manifest.DefaultApprovalPolicy = toolgovernance.ApprovalPolicyNeverAsk
	}
	return manifest
}

func isFileGeneratorTool(skillID, toolName string) bool {
	if strings.TrimSpace(skillID) != "file-generator" {
		return false
	}
	switch strings.TrimSpace(toolName) {
	case "generate_file", "generate_docx", "generate_pdf", "generate_pptx":
		return true
	default:
		return false
	}
}

func governanceAssets(params map[string]interface{}, governance map[string]interface{}, manifest toolgovernance.Manifest, arguments map[string]interface{}) []toolgovernance.AssetRef {
	argumentAssets := assetRefsFromToolArguments(manifest, arguments)
	runtimeAssets := governanceExpectedAssets(params, governance, manifest)
	if len(argumentAssets) > 0 {
		return enrichArgumentAssetRefs(argumentAssets, governanceAssetEnrichmentCandidates(params, runtimeAssets))
	}
	manifest = toolgovernance.NormalizeManifest(manifest)
	if manifest.RequiresAssetResolution && len(runtimeAssets) > 0 {
		return nil
	}
	return runtimeAssets
}

func governanceAssetEnrichmentCandidates(params map[string]interface{}, runtimeAssets []toolgovernance.AssetRef) []toolgovernance.AssetRef {
	if len(params) == 0 {
		return runtimeAssets
	}
	visibleAssets := append([]toolgovernance.AssetRef{}, assetRefsFromAny(params["console_files_visible_files"])...)
	visibleAssets = append(visibleAssets, assetRefsFromAny(params["console_agents_recent_agent_updates"])...)
	for _, key := range []string{
		"console_agents_visible_agents",
		"console_agent_visible_agents",
		"visible_agents",
		"agent_management_binding_candidates",
		"knowledge_candidates",
		"knowledge_binding_candidates",
		"agent_knowledge_candidates",
		"database_candidates",
		"database_binding_candidates",
		"agent_database_candidates",
		"database_table_candidates",
		"agent_database_table_candidates",
		"workflow_candidates",
		"workflow_binding_candidates",
		"agent_knowledge_binding_candidates",
		"agent_database_binding_candidates",
		"agent_workflow_binding_candidates",
		"available_knowledge_bases",
		"available_databases",
		"available_database_tables",
		"available_workflows",
	} {
		visibleAssets = append(visibleAssets, assetRefsFromAny(params[key])...)
	}
	if len(visibleAssets) == 0 {
		return runtimeAssets
	}
	if len(runtimeAssets) == 0 {
		return visibleAssets
	}
	out := make([]toolgovernance.AssetRef, 0, len(runtimeAssets)+len(visibleAssets))
	out = append(out, runtimeAssets...)
	out = append(out, visibleAssets...)
	return out
}

func governanceExpectedAssets(params map[string]interface{}, governance map[string]interface{}, manifest toolgovernance.Manifest) []toolgovernance.AssetRef {
	manifest = toolgovernance.NormalizeManifest(manifest)
	if !manifest.RequiresAssetResolution || strings.TrimSpace(manifest.AssetType) == "" {
		return nil
	}
	return filterGovernanceAssetsByType(
		assetRefsFromAny(firstRuntimeValue(params, governance, governanceAssetsKey, "assets")),
		manifest.AssetType,
	)
}

func filterGovernanceAssetsByType(assets []toolgovernance.AssetRef, assetType string) []toolgovernance.AssetRef {
	assetType = strings.TrimSpace(assetType)
	if len(assets) == 0 || assetType == "" {
		return assets
	}
	filtered := make([]toolgovernance.AssetRef, 0, len(assets))
	for _, asset := range assets {
		if typ := strings.TrimSpace(asset.Type); typ != "" && !strings.EqualFold(typ, assetType) {
			continue
		}
		filtered = append(filtered, asset)
	}
	if len(filtered) == 0 {
		return nil
	}
	return filtered
}

func enrichArgumentAssetRefs(argumentAssets []toolgovernance.AssetRef, runtimeAssets []toolgovernance.AssetRef) []toolgovernance.AssetRef {
	if len(argumentAssets) == 0 || len(runtimeAssets) == 0 {
		return argumentAssets
	}
	out := make([]toolgovernance.AssetRef, len(argumentAssets))
	copy(out, argumentAssets)
	for idx := range out {
		runtimeAsset, matchedByID, ok := matchingRuntimeAsset(out[idx], runtimeAssets)
		if !ok {
			continue
		}
		runtimeName := strings.TrimSpace(runtimeAsset.Name)
		if runtimeName != "" && (out[idx].Name == "" || matchedByID) {
			out[idx].Name = runtimeName
		}
		if out[idx].WorkspaceID == "" {
			out[idx].WorkspaceID = runtimeAsset.WorkspaceID
		}
		if out[idx].Source == "" {
			out[idx].Source = runtimeAsset.Source
		}
		if out[idx].Metadata == nil {
			out[idx].Metadata = copyGovernanceMetadata(runtimeAsset.Metadata)
		}
	}
	return out
}

func rewriteReadToolArgumentsFromResolvedAsset(manifest *toolgovernance.Manifest, arguments map[string]interface{}, execCtx ExecutionContext) (map[string]interface{}, map[string]interface{}, bool) {
	if manifest == nil {
		return nil, nil, false
	}
	normalized := toolgovernance.NormalizeManifest(*manifest)
	if normalized.Effect != toolgovernance.EffectRead ||
		!strings.EqualFold(strings.TrimSpace(normalized.AssetType), "file") ||
		!normalized.RequiresAssetResolution {
		return nil, nil, false
	}
	governance := governanceRuntimeParameters(execCtx.RuntimeParameters)
	expected := governanceExpectedAssets(execCtx.RuntimeParameters, governance, normalized)
	if len(expected) != 1 {
		return nil, nil, false
	}
	expectedID := strings.TrimSpace(expected[0].ID)
	if expectedID == "" {
		return nil, nil, false
	}
	actual := assetRefsFromToolArguments(normalized, arguments)
	if len(actual) == 1 && strings.TrimSpace(actual[0].ID) == expectedID {
		return nil, nil, false
	}
	rewritten := copyStringAnyMap(arguments)
	if rewritten == nil {
		rewritten = map[string]interface{}{}
	}
	var fromID string
	var fromName string
	if len(actual) > 0 {
		fromID = strings.TrimSpace(actual[0].ID)
		fromName = strings.TrimSpace(actual[0].Name)
	}
	rewritten["file_id"] = expectedID
	summary := map[string]interface{}{
		"reason":     "resolved_asset_override",
		"effect":     string(normalized.Effect),
		"asset_type": normalized.AssetType,
		"to_file_id": expectedID,
	}
	if fromID != "" {
		summary["from_file_id"] = fromID
	}
	if fromName != "" {
		summary["from_file_name"] = fromName
	}
	if expectedName := strings.TrimSpace(expected[0].Name); expectedName != "" {
		summary["to_file_name"] = expectedName
	}
	return rewritten, summary, true
}

func matchingRuntimeAsset(asset toolgovernance.AssetRef, runtimeAssets []toolgovernance.AssetRef) (toolgovernance.AssetRef, bool, bool) {
	assetID := strings.TrimSpace(asset.ID)
	assetType := strings.TrimSpace(asset.Type)
	assetName := strings.TrimSpace(asset.Name)
	for _, candidate := range runtimeAssets {
		if assetType != "" && strings.TrimSpace(candidate.Type) != "" && !strings.EqualFold(assetType, candidate.Type) {
			continue
		}
		if assetID != "" && strings.TrimSpace(candidate.ID) == assetID {
			return candidate, true, true
		}
		if assetID == "" && assetName != "" && strings.EqualFold(strings.TrimSpace(candidate.Name), assetName) {
			return candidate, false, true
		}
	}
	return toolgovernance.AssetRef{}, false, false
}

func governanceSessionGrants(params map[string]interface{}, governance map[string]interface{}) []toolgovernance.SessionGrant {
	return sessionGrantsFromAny(firstRuntimeValue(params, governance, governanceSessionGrantsKey, "session_grants"))
}

func governanceString(params map[string]interface{}, governance map[string]interface{}, flatKey string, nestedKey string) string {
	value := firstRuntimeValue(params, governance, flatKey, nestedKey)
	text, _ := value.(string)
	return strings.TrimSpace(text)
}

func firstRuntimeValue(params map[string]interface{}, governance map[string]interface{}, flatKey string, nestedKey string) interface{} {
	if len(governance) > 0 {
		if value, ok := governance[nestedKey]; ok {
			return value
		}
	}
	if len(params) > 0 {
		if value, ok := params[flatKey]; ok {
			return value
		}
	}
	return nil
}

func assetRefsFromAny(value interface{}) []toolgovernance.AssetRef {
	switch typed := value.(type) {
	case []toolgovernance.AssetRef:
		return typed
	case []map[string]interface{}:
		out := make([]toolgovernance.AssetRef, 0, len(typed))
		for _, item := range typed {
			out = append(out, assetRefFromMap(item))
		}
		return out
	case []interface{}:
		out := make([]toolgovernance.AssetRef, 0, len(typed))
		for _, item := range typed {
			if asset, ok := item.(toolgovernance.AssetRef); ok {
				out = append(out, asset)
				continue
			}
			if mapped, ok := item.(map[string]interface{}); ok {
				out = append(out, assetRefFromMap(mapped))
			}
		}
		return out
	case map[string]interface{}:
		for _, key := range []string{"data", "items", "candidates", "resources", "assets"} {
			if assets := assetRefsFromAny(typed[key]); len(assets) > 0 {
				return assets
			}
		}
		asset := assetRefFromMap(typed)
		if asset.ID == "" && asset.Name == "" && asset.Type == "" {
			return nil
		}
		return []toolgovernance.AssetRef{asset}
	default:
		return nil
	}
}

func assetRefFromMap(input map[string]interface{}) toolgovernance.AssetRef {
	assetType := stringMapValue(input, "type", "asset_type", "resource_type", "resourceType")
	if assetType == "" {
		assetType = inferredAssetTypeFromMap(input)
	}
	return toolgovernance.AssetRef{
		ID:          stringMapValue(input, assetIDKeysForType(assetType)...),
		Type:        assetType,
		Name:        governanceAssetNameFromMap(assetType, input, assetNameKeysForType(assetType)...),
		WorkspaceID: stringMapValue(input, "workspace_id", "workspaceId"),
		Source:      stringMapValue(input, "source"),
		Metadata:    copyGovernanceMetadata(input["metadata"]),
	}
}

func inferredAssetTypeFromMap(input map[string]interface{}) string {
	switch {
	case hasAnyMapKey(input, "dataset_id", "datasetId", "knowledge_dataset_id", "knowledgeDatasetId", "knowledge_base_id", "knowledgeBaseId"):
		return "knowledge_base"
	case hasAnyMapKey(input, "table_id", "tableId", "database_table_id", "databaseTableId"):
		return "database_table"
	case hasAnyMapKey(input, "binding_id", "bindingId", "workflow_binding_id", "workflowBindingId", "workflow_id", "workflowId"):
		return "workflow"
	case hasAnyMapKey(input, "data_source_id", "dataSourceId", "database_id", "databaseId"):
		return "database"
	case hasAnyMapKey(input, "file_id", "fileId", "upload_file_id", "uploadFileId", "filename", "file_name"):
		return "file"
	case hasAnyMapKey(input, "agent_id", "agentId"):
		return "agent"
	case hasAnyMapKey(input, "skill_id", "skillId", "agent_skill_id", "agentSkillId"):
		return "agent_skill"
	default:
		return ""
	}
}

func assetIDKeysForType(assetType string) []string {
	switch strings.ToLower(strings.TrimSpace(assetType)) {
	case "knowledge_base":
		return []string{"dataset_id", "datasetId", "knowledge_dataset_id", "knowledgeDatasetId", "knowledge_base_id", "knowledgeBaseId", "asset_id", "resource_id", "id"}
	case "database_table":
		return []string{"table_id", "tableId", "database_table_id", "databaseTableId", "asset_id", "resource_id", "id"}
	case "database":
		return []string{"data_source_id", "dataSourceId", "database_id", "databaseId", "asset_id", "resource_id", "id"}
	case "workflow":
		return []string{"binding_id", "bindingId", "workflow_binding_id", "workflowBindingId", "asset_id", "resource_id", "workflow_id", "workflowId", "id"}
	case "agent":
		return []string{"agent_id", "agentId", "asset_id", "resource_id", "id"}
	case "agent_skill":
		return []string{"skill_id", "skillId", "agent_skill_id", "agentSkillId", "asset_id", "resource_id", "id"}
	case "file":
		return []string{"file_id", "fileId", "upload_file_id", "uploadFileId", "asset_id", "resource_id", "id"}
	default:
		return []string{"id", "asset_id", "resource_id", "file_id", "fileId", "dataset_id", "datasetId", "knowledge_base_id", "knowledgeBaseId", "table_id", "tableId", "binding_id", "bindingId", "workflow_binding_id", "workflowBindingId", "workflow_id", "workflowId", "data_source_id", "dataSourceId", "database_id", "databaseId", "agent_id", "agentId", "skill_id", "skillId", "agent_skill_id", "agentSkillId"}
	}
}

func assetNameKeysForType(assetType string) []string {
	switch strings.ToLower(strings.TrimSpace(assetType)) {
	case "knowledge_base":
		return []string{"dataset_name", "datasetName", "knowledge_dataset_name", "knowledgeDatasetName", "knowledge_base_name", "knowledgeBaseName", "asset_name", "resource_name", "title", "name"}
	case "database_table":
		return []string{"table_name", "tableName", "database_table_name", "databaseTableName", "asset_name", "resource_name", "title", "name"}
	case "database":
		return []string{"data_source_name", "dataSourceName", "database_name", "databaseName", "schema_name", "schemaName", "asset_name", "resource_name", "title", "name"}
	case "workflow":
		return []string{"binding_name", "bindingName", "label", "workflow_name", "workflowName", "asset_name", "resource_name", "title", "name"}
	case "agent":
		return []string{"agent_name", "agentName", "asset_name", "resource_name", "title", "name"}
	case "agent_skill":
		return []string{"skill_name", "skillName", "agent_skill_name", "agentSkillName", "asset_name", "resource_name", "title", "name"}
	case "file":
		return []string{"file_name", "fileName", "filename", "output_filename", "title", "asset_name", "resource_name", "name"}
	default:
		return []string{"name", "asset_name", "resource_name", "title", "label", "filename", "file_name", "fileName", "agent_name", "agentName", "skill_name", "skillName", "agent_skill_name", "agentSkillName", "dataset_name", "datasetName", "knowledge_base_name", "knowledgeBaseName", "table_name", "tableName", "database_table_name", "databaseTableName", "binding_name", "bindingName", "workflow_name", "workflowName", "data_source_name", "dataSourceName", "database_name", "databaseName"}
	}
}

func governanceAssetNameFromMap(assetType string, input map[string]interface{}, keys ...string) string {
	name := stringMapValue(input, keys...)
	if !strings.EqualFold(strings.TrimSpace(assetType), "file") {
		return name
	}
	return fileAssetDisplayName(name, stringMapValue(input, "format"))
}

func fileAssetDisplayName(filename string, format string) string {
	filename = strings.TrimSpace(filename)
	if filename == "" {
		return filename
	}
	extension := fileAssetFormatExtension(format)
	if extension == "" {
		return filename
	}
	if dot := strings.LastIndex(filename, "."); dot > 0 {
		filename = filename[:dot]
	}
	return filename + extension
}

func fileAssetFormatExtension(format string) string {
	switch strings.ToLower(strings.TrimPrefix(strings.TrimSpace(format), ".")) {
	case "txt", "text":
		return ".txt"
	case "md", "markdown":
		return ".md"
	case "html", "htm":
		return ".html"
	case "json":
		return ".json"
	case "csv":
		return ".csv"
	case "svg":
		return ".svg"
	case "docx", "word":
		return ".docx"
	case "xlsx", "excel":
		return ".xlsx"
	case "pdf":
		return ".pdf"
	case "pptx", "powerpoint":
		return ".pptx"
	default:
		return ""
	}
}

func assetRefsFromToolArguments(manifest toolgovernance.Manifest, arguments map[string]interface{}) []toolgovernance.AssetRef {
	if len(arguments) == 0 {
		return nil
	}
	manifest = toolgovernance.NormalizeManifest(manifest)
	assetType := strings.TrimSpace(manifest.AssetType)
	if assetType == "" {
		return nil
	}

	idKeys := []string{"asset_id", "resource_id", assetType + "_id", "id"}
	nameKeys := []string{"asset_name", "resource_name", assetType + "_name", "name"}
	switch assetType {
	case "file":
		idKeys = []string{"file_id", "upload_file_id", "asset_id", "resource_id", "id"}
		nameKeys = []string{"file_name", "filename", "output_filename", "title", "asset_name", "resource_name", "name"}
	case "knowledge_base":
		if assets := knowledgeBindingAssetRefs(arguments); len(assets) > 0 {
			return assets
		}
		ids := stringSliceMapValue(arguments, "dataset_ids", "datasetIds", "knowledge_base_ids", "knowledgeBaseIds", "asset_ids", "assetIds")
		if len(ids) == 0 {
			ids = stringSliceMapValue(arguments, "dataset_id", "datasetId", "knowledge_base_id", "knowledgeBaseId", "asset_id", "assetId", "id")
		}
		if len(ids) > 0 {
			return assetRefsFromIDs(assetType, ids, "tool_arguments", nil)
		}
		idKeys = []string{"dataset_id", "datasetId", "knowledge_base_id", "knowledgeBaseId", "asset_id", "resource_id", "id"}
		nameKeys = []string{"dataset_name", "datasetName", "knowledge_base_name", "knowledgeBaseName", "asset_name", "resource_name", "name"}
	case "database":
		idKeys = []string{"data_source_id", "dataSourceId", "database_id", "databaseId", "asset_id", "resource_id", "id"}
		nameKeys = []string{"data_source_name", "dataSourceName", "database_name", "databaseName", "asset_name", "resource_name", "name"}
	case "database_table":
		if assets := databaseBindingAssetRefs(arguments); len(assets) > 0 {
			return assets
		}
		idKeys = []string{"table_id", "tableId", "database_table_id", "databaseTableId", "asset_id", "resource_id", "id"}
		nameKeys = []string{"table_name", "tableName", "database_table_name", "databaseTableName", "asset_name", "resource_name", "name"}
	case "agent":
		if assets := assetRefsFromRecords("agent", firstMapSliceMapValue(arguments, "agents", "targets", "assets"), nil); len(assets) > 0 {
			return assets
		}
		ids := stringSliceMapValue(arguments, "agent_ids", "agentIds", "asset_ids", "assetIds")
		if len(ids) > 0 {
			return assetRefsFromIDs(assetType, ids, "tool_arguments", nil)
		}
		idKeys = []string{"agent_id", "agentId", "asset_id", "resource_id", "id"}
		nameKeys = []string{"agent_name", "agentName", "asset_name", "resource_name", "title", "name"}
	case "agent_skill":
		if assets := agentSkillBindingAssetRefs(arguments); len(assets) > 0 {
			return assets
		}
		idKeys = []string{"skill_id", "skillId", "agent_skill_id", "agentSkillId", "asset_id", "resource_id", "id"}
		nameKeys = []string{"skill_name", "skillName", "agent_skill_name", "agentSkillName", "asset_name", "resource_name", "name"}
	case "workflow":
		if assets := workflowBindingAssetRefs(arguments); len(assets) > 0 {
			return assets
		}
		idKeys = []string{"binding_id", "bindingId", "workflow_id", "workflowId", "workflow_binding_id", "workflowBindingId", "asset_id", "resource_id", "id"}
		nameKeys = []string{"binding_name", "bindingName", "workflow_name", "workflowName", "asset_name", "resource_name", "name"}
	case "workflow_run":
		idKeys = []string{"workflow_run_id", "workflowRunId", "run_id", "runId", "asset_id", "resource_id", "id"}
		nameKeys = []string{"workflow_run_name", "workflowRunName", "run_name", "runName", "asset_name", "resource_name", "name"}
	}

	metadata := assetMetadataFromToolArguments(assetType, arguments)
	asset := toolgovernance.AssetRef{
		ID:          stringMapValue(arguments, idKeys...),
		Type:        assetType,
		Name:        governanceAssetNameFromMap(assetType, arguments, nameKeys...),
		WorkspaceID: stringMapValue(arguments, "workspace_id", "workspaceId"),
		Source:      "tool_arguments",
		Metadata:    metadata,
	}
	if asset.ID == "" && asset.Name == "" {
		return nil
	}
	return []toolgovernance.AssetRef{asset}
}

func agentSkillBindingAssetRefs(arguments map[string]interface{}) []toolgovernance.AssetRef {
	if !hasAnyMapKey(arguments, "skill_ids", "skillIds", "enabled_skill_ids", "enabledSkillIds", "skills", "agent_skills", "agentSkills") {
		return nil
	}
	metadata := bindingOwnerMetadata(arguments)
	assets := assetRefsFromRecords("agent_skill", firstMapSliceMapValue(arguments, "skills", "agent_skills", "agentSkills"), metadata)
	if len(assets) == 0 {
		ids := stringSliceMapValue(arguments, "skill_ids", "skillIds", "enabled_skill_ids", "enabledSkillIds", "agent_skill_ids", "agentSkillIds")
		assets = assetRefsFromIDs("agent_skill", ids, "tool_arguments", metadata)
	}
	return withBindingOwnerAsset(arguments, assets)
}

func knowledgeBindingAssetRefs(arguments map[string]interface{}) []toolgovernance.AssetRef {
	if !hasAnyMapKey(arguments, "knowledge_dataset_ids", "knowledgeDatasetIds", "dataset_ids", "datasetIds", "knowledge_bindings", "knowledgeBindings", "knowledge_bases", "knowledgeBases", "datasets") {
		return nil
	}
	metadata := bindingOwnerMetadata(arguments)
	assets := assetRefsFromRecords("knowledge_base", firstMapSliceMapValue(arguments, "knowledge_bindings", "knowledgeBindings", "knowledge_bases", "knowledgeBases", "datasets"), metadata)
	if len(assets) == 0 {
		ids := stringSliceMapValue(arguments, "knowledge_dataset_ids", "knowledgeDatasetIds", "dataset_ids", "datasetIds", "knowledge_base_ids", "knowledgeBaseIds")
		assets = assetRefsFromIDs("knowledge_base", ids, "tool_arguments", metadata)
	}
	return withBindingOwnerAsset(arguments, assets)
}

func databaseBindingAssetRefs(arguments map[string]interface{}) []toolgovernance.AssetRef {
	if !hasAnyMapKey(arguments, "database_bindings", "databaseBindings", "bindings", "table_ids", "tableIds", "writable_table_ids", "writableTableIds") {
		return nil
	}
	ownerMetadata := bindingOwnerMetadata(arguments)
	assets := make([]toolgovernance.AssetRef, 0)
	for _, binding := range firstMapSliceMapValue(arguments, "database_bindings", "databaseBindings", "bindings") {
		metadata := mergeGovernanceMetadata(ownerMetadata, databaseBindingMetadata(binding))
		assets = append(assets, databaseTableAssetRefsFromBinding(binding, metadata)...)
	}
	if len(assets) == 0 {
		metadata := mergeGovernanceMetadata(ownerMetadata, databaseBindingMetadata(arguments))
		assets = append(assets, databaseTableAssetRefsFromBinding(arguments, metadata)...)
	}
	return withBindingOwnerAsset(arguments, dedupeAssetRefs(assets))
}

func workflowBindingAssetRefs(arguments map[string]interface{}) []toolgovernance.AssetRef {
	if !hasAnyMapKey(arguments, "workflow_bindings", "workflowBindings", "bindings", "binding_ids", "bindingIds") {
		return nil
	}
	ownerMetadata := bindingOwnerMetadata(arguments)
	assets := assetRefsFromRecords("workflow", firstMapSliceMapValue(arguments, "workflow_bindings", "workflowBindings", "bindings"), ownerMetadata)
	if len(assets) == 0 {
		ids := stringSliceMapValue(arguments, "binding_ids", "bindingIds", "workflow_binding_ids", "workflowBindingIds")
		assets = assetRefsFromIDs("workflow", ids, "tool_arguments", ownerMetadata)
	}
	return withBindingOwnerAsset(arguments, assets)
}

func databaseTableAssetRefsFromBinding(binding map[string]interface{}, metadata map[string]interface{}) []toolgovernance.AssetRef {
	if len(binding) == 0 {
		return nil
	}
	assets := assetRefsFromRecords("database_table", firstMapSliceMapValue(binding, "tables", "database_tables", "databaseTables", "table_bindings", "tableBindings"), metadata)
	namesByID := map[string]string{}
	for _, asset := range assets {
		if asset.ID != "" && asset.Name != "" {
			namesByID[asset.ID] = asset.Name
		}
	}
	writable := stringSet(stringSliceMapValue(binding, "writable_table_ids", "writableTableIds", "writable_ids", "writableIds"))
	seen := map[string]struct{}{}
	for idx := range assets {
		asset := assets[idx]
		if asset.ID != "" {
			seen[asset.ID] = struct{}{}
		}
		if _, ok := writable[asset.ID]; ok {
			if assets[idx].Metadata == nil {
				assets[idx].Metadata = map[string]interface{}{}
			}
			assets[idx].Metadata["writable"] = true
		}
	}
	for _, id := range stringSliceMapValue(binding, "table_ids", "tableIds", "database_table_ids", "databaseTableIds") {
		if _, ok := seen[id]; ok {
			continue
		}
		seen[id] = struct{}{}
		assetMetadata := copyGovernanceMetadata(metadata)
		if _, ok := writable[id]; ok {
			if assetMetadata == nil {
				assetMetadata = map[string]interface{}{}
			}
			assetMetadata["writable"] = true
		}
		assets = append(assets, toolgovernance.AssetRef{
			ID:       id,
			Type:     "database_table",
			Name:     namesByID[id],
			Source:   "tool_arguments",
			Metadata: assetMetadata,
		})
	}
	for id := range writable {
		if _, ok := seen[id]; ok {
			continue
		}
		assetMetadata := copyGovernanceMetadata(metadata)
		if assetMetadata == nil {
			assetMetadata = map[string]interface{}{}
		}
		assetMetadata["writable"] = true
		assets = append(assets, toolgovernance.AssetRef{
			ID:       id,
			Type:     "database_table",
			Name:     namesByID[id],
			Source:   "tool_arguments",
			Metadata: assetMetadata,
		})
	}
	return assets
}

func assetRefsFromRecords(assetType string, records []map[string]interface{}, metadata map[string]interface{}) []toolgovernance.AssetRef {
	if len(records) == 0 {
		return nil
	}
	out := make([]toolgovernance.AssetRef, 0, len(records))
	for _, record := range records {
		asset := assetRefFromMap(record)
		if strings.TrimSpace(asset.Type) == "" {
			asset.Type = assetType
		}
		if asset.Source == "" {
			asset.Source = "tool_arguments"
		}
		asset.Metadata = mergeGovernanceMetadata(metadata, bindingRecordMetadata(assetType, record), asset.Metadata)
		if asset.ID == "" && asset.Name == "" {
			continue
		}
		out = append(out, asset)
	}
	return out
}

func withBindingOwnerAsset(arguments map[string]interface{}, assets []toolgovernance.AssetRef) []toolgovernance.AssetRef {
	owner, ok := bindingOwnerAsset(arguments)
	if !ok {
		return assets
	}
	out := make([]toolgovernance.AssetRef, 0, len(assets)+1)
	out = append(out, owner)
	out = append(out, assets...)
	return dedupeAssetRefs(out)
}

func bindingOwnerAsset(arguments map[string]interface{}) (toolgovernance.AssetRef, bool) {
	agentID := stringMapValue(arguments, "agent_id", "agentId")
	agentName := stringMapValue(arguments, "agent_name", "agentName")
	if agentID == "" && agentName == "" {
		return toolgovernance.AssetRef{}, false
	}
	return toolgovernance.AssetRef{
		ID:       agentID,
		Type:     "agent",
		Name:     agentName,
		Source:   "tool_arguments",
		Metadata: map[string]interface{}{"binding_owner": true},
	}, true
}

func bindingOwnerMetadata(arguments map[string]interface{}) map[string]interface{} {
	metadata := map[string]interface{}{}
	if agentID := stringMapValue(arguments, "agent_id", "agentId"); agentID != "" {
		metadata["agent_id"] = agentID
	}
	if agentName := stringMapValue(arguments, "agent_name", "agentName"); agentName != "" {
		metadata["agent_name"] = agentName
	}
	if workspaceID := stringMapValue(arguments, "workspace_id", "workspaceId"); workspaceID != "" {
		metadata["workspace_id"] = workspaceID
	}
	if len(metadata) == 0 {
		return nil
	}
	return metadata
}

func databaseBindingMetadata(input map[string]interface{}) map[string]interface{} {
	metadata := map[string]interface{}{}
	if dataSourceID := stringMapValue(input, "data_source_id", "dataSourceId", "database_id", "databaseId"); dataSourceID != "" {
		metadata["data_source_id"] = dataSourceID
	}
	if dataSourceName := stringMapValue(input, "data_source_name", "dataSourceName", "database_name", "databaseName", "name"); dataSourceName != "" {
		metadata["database_name"] = dataSourceName
	}
	if len(metadata) == 0 {
		return nil
	}
	return metadata
}

func bindingRecordMetadata(assetType string, input map[string]interface{}) map[string]interface{} {
	metadata := map[string]interface{}{}
	switch strings.ToLower(strings.TrimSpace(assetType)) {
	case "database_table":
		if dataSourceID := stringMapValue(input, "data_source_id", "dataSourceId", "database_id", "databaseId"); dataSourceID != "" {
			metadata["data_source_id"] = dataSourceID
		}
		if dataSourceName := stringMapValue(input, "data_source_name", "dataSourceName", "database_name", "databaseName"); dataSourceName != "" {
			metadata["database_name"] = dataSourceName
		}
		if boolMapValue(input, "writable", "can_write", "canWrite") {
			metadata["writable"] = true
		}
	case "workflow":
		if workflowID := stringMapValue(input, "workflow_id", "workflowId"); workflowID != "" {
			metadata["workflow_id"] = workflowID
		}
		if workflowAgentID := stringMapValue(input, "agent_id", "agentId"); workflowAgentID != "" {
			metadata["workflow_agent_id"] = workflowAgentID
		}
		if strategy := stringMapValue(input, "version_strategy", "versionStrategy"); strategy != "" {
			metadata["version_strategy"] = strategy
		}
		if versionUUID := stringMapValue(input, "version_uuid", "versionUUID", "version_id", "versionId"); versionUUID != "" {
			metadata["version_uuid"] = versionUUID
		}
	case "knowledge_base":
		if description := stringMapValue(input, "description", "desc"); description != "" {
			metadata["description"] = description
		}
	}
	if len(metadata) == 0 {
		return nil
	}
	return metadata
}

func mergeGovernanceMetadata(values ...map[string]interface{}) map[string]interface{} {
	out := map[string]interface{}{}
	for _, value := range values {
		for key, item := range value {
			out[key] = item
		}
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

func dedupeAssetRefs(input []toolgovernance.AssetRef) []toolgovernance.AssetRef {
	if len(input) == 0 {
		return nil
	}
	seen := map[string]int{}
	out := make([]toolgovernance.AssetRef, 0, len(input))
	for _, asset := range input {
		key := strings.Join([]string{
			strings.ToLower(strings.TrimSpace(asset.Type)),
			strings.TrimSpace(asset.ID),
			strings.ToLower(strings.TrimSpace(asset.Name)),
		}, "\x00")
		if index, ok := seen[key]; ok {
			if out[index].Name == "" {
				out[index].Name = asset.Name
			}
			if out[index].Metadata == nil {
				out[index].Metadata = copyGovernanceMetadata(asset.Metadata)
			}
			continue
		}
		seen[key] = len(out)
		out = append(out, asset)
	}
	return out
}

func firstMapSliceMapValue(input map[string]interface{}, keys ...string) []map[string]interface{} {
	value := firstMapValue(input, keys...)
	switch typed := value.(type) {
	case []map[string]interface{}:
		out := make([]map[string]interface{}, 0, len(typed))
		for _, item := range typed {
			out = append(out, item)
		}
		return out
	case []interface{}:
		out := make([]map[string]interface{}, 0, len(typed))
		for _, item := range typed {
			if mapped, ok := item.(map[string]interface{}); ok {
				out = append(out, mapped)
			}
		}
		return out
	case string:
		text := strings.TrimSpace(typed)
		if text == "" {
			return nil
		}
		var records []map[string]interface{}
		if err := json.Unmarshal([]byte(text), &records); err == nil {
			return records
		}
		var record map[string]interface{}
		if err := json.Unmarshal([]byte(text), &record); err == nil && len(record) > 0 {
			return []map[string]interface{}{record}
		}
		return nil
	default:
		if mapped, ok := typed.(map[string]interface{}); ok {
			return []map[string]interface{}{mapped}
		}
		return nil
	}
}

func stringSet(values []string) map[string]struct{} {
	out := make(map[string]struct{}, len(values))
	for _, value := range values {
		if value = strings.TrimSpace(value); value != "" {
			out[value] = struct{}{}
		}
	}
	return out
}

func hasAnyMapKey(input map[string]interface{}, keys ...string) bool {
	if len(input) == 0 {
		return false
	}
	for _, key := range keys {
		if _, ok := input[key]; ok {
			return true
		}
	}
	return false
}

func assetRefsFromIDs(assetType string, ids []string, source string, metadata map[string]interface{}) []toolgovernance.AssetRef {
	out := make([]toolgovernance.AssetRef, 0, len(ids))
	for _, id := range ids {
		id = strings.TrimSpace(id)
		if id == "" {
			continue
		}
		out = append(out, toolgovernance.AssetRef{
			ID:       id,
			Type:     assetType,
			Source:   strings.TrimSpace(source),
			Metadata: copyGovernanceMetadata(metadata),
		})
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

func assetMetadataFromToolArguments(assetType string, arguments map[string]interface{}) map[string]interface{} {
	if len(arguments) == 0 {
		return nil
	}
	metadata := map[string]interface{}{}
	if assetType == "database_table" {
		if dataSourceID := stringMapValue(arguments, "data_source_id", "dataSourceId", "database_id", "databaseId"); dataSourceID != "" {
			metadata["data_source_id"] = dataSourceID
		}
		if count := recordCountFromArguments(arguments); count > 0 {
			metadata["record_count"] = count
		}
	}
	if assetType == "file" {
		if format := stringMapValue(arguments, "format", "chart_type"); format != "" {
			metadata["format"] = format
		}
		if lifecycle := stringMapValue(arguments, "lifecycle"); lifecycle != "" {
			metadata["lifecycle"] = lifecycle
		}
	}
	if len(metadata) == 0 {
		return nil
	}
	return metadata
}

func recordCountFromArguments(arguments map[string]interface{}) int {
	value := firstMapValue(arguments, "records", "rows")
	switch typed := value.(type) {
	case []interface{}:
		return len(typed)
	case []map[string]interface{}:
		return len(typed)
	default:
		return 0
	}
}

func sessionGrantsFromAny(value interface{}) []toolgovernance.SessionGrant {
	switch typed := value.(type) {
	case []toolgovernance.SessionGrant:
		return typed
	case []map[string]interface{}:
		out := make([]toolgovernance.SessionGrant, 0, len(typed))
		for _, item := range typed {
			out = append(out, sessionGrantFromMap(item))
		}
		return out
	case []interface{}:
		out := make([]toolgovernance.SessionGrant, 0, len(typed))
		for _, item := range typed {
			if grant, ok := item.(toolgovernance.SessionGrant); ok {
				out = append(out, grant)
				continue
			}
			if mapped, ok := item.(map[string]interface{}); ok {
				out = append(out, sessionGrantFromMap(mapped))
			}
		}
		return out
	default:
		return nil
	}
}

func sessionGrantFromMap(input map[string]interface{}) toolgovernance.SessionGrant {
	grant := toolgovernance.SessionGrant{
		ConversationID: stringMapValue(input, "conversation_id", "conversationId"),
		OrganizationID: stringMapValue(input,
			"organization_id",
			"organizationId",
			"tenant_id",
			"tenantId",
		),
		UserID:       stringMapValue(input, "user_id", "userId", "account_id", "accountId"),
		SkillID:      stringMapValue(input, "skill_id", "skillId"),
		ProviderType: stringMapValue(input, "provider_type", "providerType"),
		ProviderID:   stringMapValue(input, "provider_id", "providerId"),
		ToolID:       stringMapValue(input, "tool_id", "toolId"),
		Effect:       toolgovernance.Effect(stringMapValue(input, "effect")),
		AssetType:    stringMapValue(input, "asset_type", "assetType"),
		Assets:       assetRefsFromAny(firstMapValue(input, "assets", "asset_refs", "assetRefs")),
		RiskLevel:    toolgovernance.RiskLevel(stringMapValue(input, "risk_level", "riskLevel")),
		ApprovalCorrelationID: stringMapValue(input,
			"approval_correlation_id",
			"approvalCorrelationId",
			"approved_by_correlation_id",
			"approvedByCorrelationId",
			"source_correlation_id",
			"sourceCorrelationId",
		),
		GrantedAt: timeFromAny(firstMapValue(input, "granted_at", "grantedAt")),
		ExpiresAt: timeFromAny(firstMapValue(input, "expires_at", "expiresAt")),
	}
	if len(grant.Assets) == 0 {
		grant.Assets = assetRefsFromIDList(firstMapValue(input, "asset_ids", "assetIds", "file_ids", "fileIds"), grant.AssetType)
	}
	return grant
}

func assetRefsFromIDList(value interface{}, assetType string) []toolgovernance.AssetRef {
	switch typed := value.(type) {
	case []string:
		out := make([]toolgovernance.AssetRef, 0, len(typed))
		for _, id := range typed {
			if id = strings.TrimSpace(id); id != "" {
				out = append(out, toolgovernance.AssetRef{ID: id, Type: assetType})
			}
		}
		return out
	case []interface{}:
		out := make([]toolgovernance.AssetRef, 0, len(typed))
		for _, item := range typed {
			if id := strings.TrimSpace(stringMapScalar(item)); id != "" {
				out = append(out, toolgovernance.AssetRef{ID: id, Type: assetType})
			}
		}
		return out
	default:
		return nil
	}
}

func stringMapValue(input map[string]interface{}, keys ...string) string {
	value := firstMapValue(input, keys...)
	return stringMapScalar(value)
}

func boolMapValue(input map[string]interface{}, keys ...string) bool {
	value := firstMapValue(input, keys...)
	switch typed := value.(type) {
	case bool:
		return typed
	case string:
		switch strings.ToLower(strings.TrimSpace(typed)) {
		case "true", "1", "yes", "y", "on":
			return true
		default:
			return false
		}
	default:
		return false
	}
}

func stringSliceMapValue(input map[string]interface{}, keys ...string) []string {
	value := firstMapValue(input, keys...)
	switch typed := value.(type) {
	case []string:
		out := make([]string, 0, len(typed))
		for _, item := range typed {
			if item = strings.TrimSpace(item); item != "" {
				out = append(out, item)
			}
		}
		return out
	case []interface{}:
		out := make([]string, 0, len(typed))
		for _, item := range typed {
			if text := stringMapScalar(item); text != "" {
				out = append(out, text)
			}
		}
		return out
	case string:
		text := strings.TrimSpace(typed)
		if text == "" {
			return nil
		}
		if strings.HasPrefix(text, "[") {
			var out []string
			if err := json.Unmarshal([]byte(text), &out); err == nil {
				return out
			}
			var raw []interface{}
			if err := json.Unmarshal([]byte(text), &raw); err == nil {
				out := make([]string, 0, len(raw))
				for _, item := range raw {
					if text := stringMapScalar(item); text != "" {
						out = append(out, text)
					}
				}
				return out
			}
		}
		parts := strings.Split(text, ",")
		out := make([]string, 0, len(parts))
		for _, part := range parts {
			if part = strings.TrimSpace(part); part != "" {
				out = append(out, part)
			}
		}
		return out
	default:
		return nil
	}
}

func stringMapScalar(value interface{}) string {
	switch typed := value.(type) {
	case string:
		return strings.TrimSpace(typed)
	case fmt.Stringer:
		return strings.TrimSpace(typed.String())
	default:
		return ""
	}
}

func firstMapValue(input map[string]interface{}, keys ...string) interface{} {
	if len(input) == 0 {
		return nil
	}
	for _, key := range keys {
		if value, ok := input[key]; ok {
			return value
		}
	}
	return nil
}

func timeFromAny(value interface{}) time.Time {
	switch typed := value.(type) {
	case time.Time:
		return typed
	case string:
		parsed, err := time.Parse(time.RFC3339, strings.TrimSpace(typed))
		if err == nil {
			return parsed
		}
	}
	return time.Time{}
}

func copyGovernanceMetadata(value interface{}) map[string]interface{} {
	input, ok := value.(map[string]interface{})
	if !ok || len(input) == 0 {
		return nil
	}
	out := make(map[string]interface{}, len(input))
	for key, item := range input {
		out[key] = item
	}
	return out
}
