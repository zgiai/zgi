package service

import (
	"strings"

	actiondto "github.com/zgiai/zgi/api/internal/capabilities/actionruntime/dto"
	"github.com/zgiai/zgi/api/internal/capabilities/toolgovernance"
)

const skillToolGovernanceRuntimeKey = "tool_governance"

func applySkillToolGovernanceRuntimeParameters(params map[string]interface{}, prepared *PreparedChat) map[string]interface{} {
	if params == nil {
		params = map[string]interface{}{}
	}
	governance := map[string]interface{}{}
	if existing, ok := params[skillToolGovernanceRuntimeKey].(map[string]interface{}); ok {
		governance = copyStringAnyMap(existing)
	}
	if governance == nil {
		governance = map[string]interface{}{}
	}
	if strings.TrimSpace(stringMetadataValue(governance["permission_tier"])) == "" {
		if tier := strings.TrimSpace(stringMetadataValue(params["tool_governance_permission_tier"])); tier != "" {
			governance["permission_tier"] = tier
		} else {
			governance["permission_tier"] = string(toolgovernance.PermissionTierBasic)
		}
	}
	if _, exists := governance["assets"]; !exists {
		if _, flatAssetsExist := params["tool_governance_assets"]; !flatAssetsExist {
			if assets := skillToolGovernanceAssetsFromPrepared(prepared); len(assets) > 0 {
				governance["assets"] = assets
			}
		}
	}
	if _, exists := governance["session_grants"]; !exists {
		if _, flatGrantsExist := params["tool_governance_session_grants"]; !flatGrantsExist {
			if grants := skillToolGovernanceSessionGrantsFromPrepared(prepared); len(grants) > 0 {
				governance["session_grants"] = grants
			}
		}
	}
	params[skillToolGovernanceRuntimeKey] = governance
	return params
}

func skillToolGovernanceAssetsFromPrepared(prepared *PreparedChat) []map[string]interface{} {
	if prepared == nil || prepared.parts == nil {
		return nil
	}
	refGroups := make([][]PlannerResourceRef, 0, 2)
	if refs := plannerResourceRefsFromConsoleFilesQuery(prepared.parts); len(refs) > 0 {
		refGroups = append(refGroups, refs)
	}
	refGroups = append(refGroups, []PlannerResourceRef{{Type: resourceTypeFile}})

	for _, refs := range refGroups {
		result := resolveChatResourceRefs(prepared.parts, refs)
		if !allResourceRefsResolved(result.Results) || len(result.Resources) == 0 {
			continue
		}
		return toolGovernanceAssetMapsFromResources(result.Resources)
	}
	return nil
}

func skillToolGovernanceSessionGrantsFromPrepared(prepared *PreparedChat) []map[string]interface{} {
	if prepared == nil || prepared.Conversation == nil {
		return nil
	}
	grants := mapSliceFromAny(prepared.Conversation.Metadata["tool_governance_session_grants"])
	if prepared.Message != nil {
		grants = append(grants, mapSliceFromAny(prepared.Message.Metadata["tool_governance_one_shot_grants"])...)
	}
	return grants
}

func toolGovernanceAssetMapsFromResources(resources []actiondto.ResourceRef) []map[string]interface{} {
	if len(resources) == 0 {
		return nil
	}
	seen := map[string]struct{}{}
	assets := make([]map[string]interface{}, 0, len(resources))
	for _, resource := range resources {
		id := strings.TrimSpace(resource.ID)
		name := strings.TrimSpace(resource.Name)
		if id == "" && name == "" {
			continue
		}
		resourceType := strings.TrimSpace(resource.Type)
		if resourceType == "" {
			resourceType = resourceTypeFile
		}
		key := resourceType + ":" + id + ":" + name
		if _, exists := seen[key]; exists {
			continue
		}
		seen[key] = struct{}{}

		asset := map[string]interface{}{
			"type": resourceType,
		}
		if id != "" {
			asset["id"] = id
		}
		if name != "" {
			asset["name"] = name
		}
		if source := strings.TrimSpace(resource.Source); source != "" {
			asset["source"] = source
		}
		if metadata := copyStringAnyMap(resource.Metadata); len(metadata) > 0 {
			if workspaceID := strings.TrimSpace(stringMetadataValue(firstMapValue(metadata, "workspace_id", "workspaceId"))); workspaceID != "" {
				asset["workspace_id"] = workspaceID
			}
			asset["metadata"] = metadata
		}
		assets = append(assets, asset)
	}
	return assets
}
