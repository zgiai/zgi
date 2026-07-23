package tools

import (
	"encoding/json"
	"strings"
)

const AgentBindingAuthorizationsParameter = "agent_binding_authorizations"

// AgentBindingAuthorization is persisted runtime evidence for one concrete
// Agent resource binding.
type AgentBindingAuthorization struct {
	BindingType      string `json:"binding_type"`
	ResourceID       string `json:"resource_id"`
	ParentResourceID string `json:"parent_resource_id,omitempty"`
	AccessMode       string `json:"access_mode"`
	BoundByAccountID string `json:"bound_by_account_id"`
	BoundAtUnix      int64  `json:"bound_at_unix"`
}

// AgentBindingAuthorizations returns normalized, valid per-resource evidence.
func AgentBindingAuthorizations(parameters map[string]interface{}) []AgentBindingAuthorization {
	if parameters == nil {
		return nil
	}
	payload, err := json.Marshal(parameters[AgentBindingAuthorizationsParameter])
	if err != nil {
		return nil
	}
	var parsed []AgentBindingAuthorization
	if err := json.Unmarshal(payload, &parsed); err != nil {
		return nil
	}
	result := make([]AgentBindingAuthorization, 0, len(parsed))
	seen := make(map[string]struct{}, len(parsed))
	for _, authorization := range parsed {
		authorization.BindingType = strings.TrimSpace(authorization.BindingType)
		authorization.ResourceID = strings.TrimSpace(authorization.ResourceID)
		authorization.ParentResourceID = strings.TrimSpace(authorization.ParentResourceID)
		authorization.AccessMode = strings.TrimSpace(authorization.AccessMode)
		authorization.BoundByAccountID = strings.TrimSpace(authorization.BoundByAccountID)
		if authorization.BindingType == "" || authorization.ResourceID == "" || authorization.AccessMode == "" || authorization.BoundByAccountID == "" || authorization.BoundAtUnix <= 0 {
			continue
		}
		key := agentBindingAuthorizationKey(authorization.BindingType, authorization.ParentResourceID, authorization.ResourceID, authorization.AccessMode)
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
		result = append(result, authorization)
	}
	return result
}

// AgentBindingAuthorizationFor finds evidence for an exact binding. A write
// grant also authorizes read access to the same database table.
func AgentBindingAuthorizationFor(
	parameters map[string]interface{},
	bindingType string,
	parentResourceID string,
	resourceID string,
	accessMode string,
) (AgentBindingAuthorization, bool) {
	bindingType = strings.TrimSpace(bindingType)
	parentResourceID = strings.TrimSpace(parentResourceID)
	resourceID = strings.TrimSpace(resourceID)
	accessMode = strings.TrimSpace(accessMode)
	writableFallback := AgentBindingAuthorization{}
	hasWritableFallback := false
	for _, authorization := range AgentBindingAuthorizations(parameters) {
		if authorization.BindingType != bindingType || authorization.ParentResourceID != parentResourceID || authorization.ResourceID != resourceID {
			continue
		}
		if authorization.AccessMode == accessMode {
			return authorization, true
		}
		if accessMode == "read" && authorization.AccessMode == "write" {
			writableFallback = authorization
			hasWritableFallback = true
		}
	}
	return writableFallback, hasWritableFallback
}

// AgentBindingAuthorizationsForType returns all valid evidence for a binding type.
func AgentBindingAuthorizationsForType(parameters map[string]interface{}, bindingType string) []AgentBindingAuthorization {
	bindingType = strings.TrimSpace(bindingType)
	result := make([]AgentBindingAuthorization, 0)
	for _, authorization := range AgentBindingAuthorizations(parameters) {
		if authorization.BindingType == bindingType {
			result = append(result, authorization)
		}
	}
	return result
}

func agentBindingAuthorizationKey(bindingType, parentResourceID, resourceID, accessMode string) string {
	return strings.Join([]string{bindingType, parentResourceID, resourceID, accessMode}, "\x00")
}
