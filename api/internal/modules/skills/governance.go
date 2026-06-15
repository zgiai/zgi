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
	return toolgovernance.Decide(toolgovernance.Request{
		Manifest:       req.Manifest,
		PermissionTier: governancePermissionTier(params, governance),
		ConversationID: req.ExecutionContext.ConversationID,
		Assets:         governanceAssets(params, governance, req.Manifest, req.Arguments),
		ExpectedAssets: governanceExpectedAssets(params, governance, req.Manifest),
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

func governanceAssets(params map[string]interface{}, governance map[string]interface{}, manifest toolgovernance.Manifest, arguments map[string]interface{}) []toolgovernance.AssetRef {
	argumentAssets := assetRefsFromToolArguments(manifest, arguments)
	runtimeAssets := governanceExpectedAssets(params, governance, manifest)
	if len(argumentAssets) > 0 {
		return enrichArgumentAssetRefs(argumentAssets, runtimeAssets)
	}
	manifest = toolgovernance.NormalizeManifest(manifest)
	if manifest.RequiresAssetResolution && len(runtimeAssets) > 0 {
		return nil
	}
	return runtimeAssets
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
	if assetType == "file" {
		idKeys = []string{"file_id", "upload_file_id", "asset_id", "resource_id", "id"}
		nameKeys = []string{"file_name", "filename", "asset_name", "resource_name", "name"}
	}
	asset := toolgovernance.AssetRef{
		ID:          stringMapValue(arguments, idKeys...),
		Type:        assetType,
		Name:        stringMapValue(arguments, nameKeys...),
		WorkspaceID: stringMapValue(arguments, "workspace_id", "workspaceId"),
		Source:      "tool_arguments",
	}
	if asset.ID == "" && asset.Name == "" {
		return nil
	}
	return []toolgovernance.AssetRef{asset}
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
		ToolID:         stringMapValue(input, "tool_id", "toolId"),
		Effect:         toolgovernance.Effect(stringMapValue(input, "effect")),
		AssetType:      stringMapValue(input, "asset_type", "assetType"),
		Assets:         assetRefsFromAny(firstMapValue(input, "assets", "asset_refs", "assetRefs")),
		RiskLevel:      toolgovernance.RiskLevel(stringMapValue(input, "risk_level", "riskLevel")),
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
