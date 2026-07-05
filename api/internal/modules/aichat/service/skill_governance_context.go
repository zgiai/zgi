//go:build legacy_aichat_service
// +build legacy_aichat_service

package service

import (
	"strings"

	"github.com/zgiai/zgi/api/internal/capabilities/toolgovernance"
)

const (
	skillToolGovernanceRuntimeKey    = "tool_governance"
	skillToolGovernancePermissionKey = "tool_governance_permission_tier"
)

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
	if strings.TrimSpace(stringFromAny(governance["permission_tier"])) == "" {
		if tier := strings.TrimSpace(stringFromAny(params[skillToolGovernancePermissionKey])); tier != "" {
			governance["permission_tier"] = tier
		} else {
			governance["permission_tier"] = string(toolgovernance.PermissionTierBasic)
		}
	}
	// The legacy runtime follows the current governance contract: permission
	// policy is injected here, while operation targets must come from explicit
	// tool arguments or caller-provided runtime assets.
	params[skillToolGovernanceRuntimeKey] = governance
	return params
}
