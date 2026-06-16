package skills

import (
	"context"
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
)

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
	manifest := governanceManifestForArguments(req.Manifest, req.SkillID, req.ToolName, req.Arguments)
	return toolgovernance.Decide(toolgovernance.Request{
		Manifest:       manifest,
		PermissionTier: governancePermissionTier(params, governance),
		ConversationID: req.ExecutionContext.ConversationID,
		OrganizationID: req.ExecutionContext.OrganizationID,
		UserID:         req.ExecutionContext.UserID,
		SkillID:        req.SkillID,
		ProviderType:   string(req.ProviderType),
		ProviderID:     req.ProviderID,
		Assets:         governanceAssets(params, governance, manifest, req.Arguments),
		ExpectedAssets: governanceExpectedAssets(params, governance, manifest),
		SessionGrants:  governanceSessionGrants(params, governance),
		CorrelationID:  governanceString(params, governance, governanceCorrelationIDKey, "correlation_id"),
	}, g.policy), nil
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

func governanceManifestForArguments(manifest toolgovernance.Manifest, skillID, toolName string, arguments map[string]interface{}) toolgovernance.Manifest {
	if !isFileGeneratorTool(skillID, toolName) {
		return manifest
	}
	if isManagedFileGenerationTarget(stringMapValue(arguments, "target")) {
		return manifest
	}
	manifest.DefaultApprovalPolicy = toolgovernance.ApprovalPolicyNeverAsk
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

func isManagedFileGenerationTarget(target string) bool {
	switch strings.ToLower(strings.TrimSpace(target)) {
	case "managed_file", "file_management", "managed", "workspace_file":
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
	visibleFiles := assetRefsFromAny(params["console_files_visible_files"])
	if len(visibleFiles) == 0 {
		return runtimeAssets
	}
	if len(runtimeAssets) == 0 {
		return visibleFiles
	}
	out := make([]toolgovernance.AssetRef, 0, len(runtimeAssets)+len(visibleFiles))
	out = append(out, runtimeAssets...)
	out = append(out, visibleFiles...)
	return out
}

func governanceExpectedAssets(params map[string]interface{}, governance map[string]interface{}, manifest toolgovernance.Manifest) []toolgovernance.AssetRef {
	manifest = toolgovernance.NormalizeManifest(manifest)
	if !manifest.RequiresAssetResolution || strings.TrimSpace(manifest.AssetType) == "" {
		return nil
	}
	return assetRefsFromAny(firstRuntimeValue(params, governance, governanceAssetsKey, "assets"))
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
		if out[idx].Name == "" || matchedByID {
			out[idx].Name = runtimeAsset.Name
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
	default:
		return nil
	}
}

func assetRefFromMap(input map[string]interface{}) toolgovernance.AssetRef {
	return toolgovernance.AssetRef{
		ID:          stringMapValue(input, "id", "asset_id", "file_id"),
		Type:        stringMapValue(input, "type", "asset_type"),
		Name:        stringMapValue(input, "name", "asset_name", "filename", "file_name"),
		WorkspaceID: stringMapValue(input, "workspace_id", "workspaceId"),
		Source:      stringMapValue(input, "source"),
		Metadata:    copyGovernanceMetadata(input["metadata"]),
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
		idKeys = []string{"table_id", "tableId", "database_table_id", "databaseTableId", "asset_id", "resource_id", "id"}
		nameKeys = []string{"table_name", "tableName", "database_table_name", "databaseTableName", "asset_name", "resource_name", "name"}
	case "workflow":
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
		Name:        stringMapValue(arguments, nameKeys...),
		WorkspaceID: stringMapValue(arguments, "workspace_id", "workspaceId"),
		Source:      "tool_arguments",
		Metadata:    metadata,
	}
	if asset.ID == "" && asset.Name == "" {
		return nil
	}
	return []toolgovernance.AssetRef{asset}
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
		parts := strings.Split(typed, ",")
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
