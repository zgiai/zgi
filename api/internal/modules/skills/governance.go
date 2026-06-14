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
	assets := assetRefsFromAny(firstRuntimeValue(params, governance, governanceAssetsKey, "assets"))
	if len(assets) > 0 {
		return assets
	}
	return assetRefsFromToolArguments(manifest, arguments)
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
	return toolgovernance.SessionGrant{
		ConversationID: stringMapValue(input, "conversation_id", "conversationId"),
		ToolID:         stringMapValue(input, "tool_id", "toolId"),
		Effect:         toolgovernance.Effect(stringMapValue(input, "effect")),
		AssetType:      stringMapValue(input, "asset_type", "assetType"),
		RiskLevel:      toolgovernance.RiskLevel(stringMapValue(input, "risk_level", "riskLevel")),
		GrantedAt:      timeFromAny(firstMapValue(input, "granted_at", "grantedAt")),
		ExpiresAt:      timeFromAny(firstMapValue(input, "expires_at", "expiresAt")),
	}
}

func stringMapValue(input map[string]interface{}, keys ...string) string {
	value := firstMapValue(input, keys...)
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
