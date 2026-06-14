package service

import (
	"sort"
	"strings"

	actionmodel "github.com/zgiai/zgi/api/internal/capabilities/actionruntime/model"
)

const defaultInvocationTokenTTLSeconds = 300

type Registry struct {
	items map[string]CapabilityManifest
}

func NewRegistry(capabilities []CapabilityManifest) *Registry {
	items := make(map[string]CapabilityManifest, len(capabilities))
	for _, capability := range capabilities {
		capability.ID = strings.TrimSpace(capability.ID)
		if capability.ID == "" {
			continue
		}
		capability.RiskLevel = normalizeRiskLevel(capability.RiskLevel)
		if capability.AuthMode == "" {
			capability.AuthMode = AuthModeActorContext
		}
		if capability.TokenTTLSeconds == 0 && capability.AuthMode == AuthModeInvocationToken {
			capability.TokenTTLSeconds = defaultInvocationTokenTTLSeconds
		}
		items[capability.ID] = cloneCapability(capability)
	}
	return &Registry{items: items}
}

func NewDefaultRegistry() *Registry {
	return NewRegistry(defaultCapabilities())
}

func (r *Registry) List() []CapabilityManifest {
	if r == nil {
		return nil
	}
	out := make([]CapabilityManifest, 0, len(r.items))
	for _, item := range r.items {
		out = append(out, cloneCapability(item))
	}
	sort.Slice(out, func(i, j int) bool {
		return out[i].ID < out[j].ID
	})
	return out
}

func (r *Registry) Get(id string) (CapabilityManifest, bool) {
	if r == nil {
		return CapabilityManifest{}, false
	}
	item, ok := r.items[strings.TrimSpace(id)]
	if !ok {
		return CapabilityManifest{}, false
	}
	return cloneCapability(item), true
}

func defaultCapabilities() []CapabilityManifest {
	return []CapabilityManifest{
		{
			ID:                   "agent.create",
			Domain:               "agent",
			Action:               "create",
			Name:                 "Create agent",
			Description:          "Draft and create a new agent from user intent",
			Runtime:              RuntimeInternal,
			AuthMode:             AuthModeActorContext,
			RiskLevel:            actionmodel.RiskLevelMedium,
			RequiresConfirmation: true,
			IdempotencyRequired:  true,
			AllowedResources:     []string{"workspace"},
			Scopes:               []string{"agent:write"},
		},
		{
			ID:                   "agent.publish",
			Domain:               "agent",
			Action:               "publish",
			Name:                 "Publish agent",
			Description:          "Publish an agent version for users to invoke",
			Runtime:              RuntimeAgent,
			AuthMode:             AuthModeActorContext,
			RiskLevel:            actionmodel.RiskLevelHigh,
			RequiresConfirmation: true,
			IdempotencyRequired:  true,
			AllowedResources:     []string{"agent"},
			Scopes:               []string{"agent:publish"},
		},
		{
			ID:                   "agent.invoke",
			Domain:               "agent",
			Action:               "invoke",
			Name:                 "Invoke agent",
			Description:          "Invoke an existing agent on behalf of the current user",
			Runtime:              RuntimeAgent,
			AuthMode:             AuthModeInvocationToken,
			RiskLevel:            actionmodel.RiskLevelLow,
			RequiresConfirmation: false,
			IdempotencyRequired:  false,
			AllowedResources:     []string{"agent"},
			Scopes:               []string{"agent:invoke"},
		},
		{
			ID:                   "workflow.invoke",
			Domain:               "workflow",
			Action:               "invoke",
			Name:                 "Invoke workflow",
			Description:          "Run a workflow with scoped user authority",
			Runtime:              RuntimeWorkflow,
			AuthMode:             AuthModeInvocationToken,
			RiskLevel:            actionmodel.RiskLevelMedium,
			RequiresConfirmation: true,
			IdempotencyRequired:  true,
			AllowedResources:     []string{"workflow"},
			Scopes:               []string{"workflow:run"},
		},
		{
			ID:                   "file.read",
			Domain:               "file",
			Action:               "read",
			Name:                 "Read file",
			Description:          "Read metadata or extracted text from files the user can access",
			Runtime:              RuntimeFile,
			AuthMode:             AuthModeActorContext,
			RiskLevel:            actionmodel.RiskLevelLow,
			RequiresConfirmation: false,
			IdempotencyRequired:  false,
			AllowedResources:     []string{"file"},
			Scopes:               []string{"file:read"},
		},
		{
			ID:                   "file.create",
			Domain:               "file",
			Action:               "create",
			Name:                 "Create file",
			Description:          "Create a new file or generated artifact in the workspace",
			Runtime:              RuntimeFile,
			AuthMode:             AuthModeActorContext,
			RiskLevel:            actionmodel.RiskLevelMedium,
			RequiresConfirmation: true,
			IdempotencyRequired:  true,
			AllowedResources:     []string{"workspace", "folder"},
			Scopes:               []string{"file:write"},
		},
		{
			ID:                   "automation.create",
			Domain:               "automation",
			Action:               "create",
			Name:                 "Create automation",
			Description:          "Create a scheduled or event-driven automation",
			Runtime:              RuntimeAutomation,
			AuthMode:             AuthModeInvocationToken,
			RiskLevel:            actionmodel.RiskLevelHigh,
			RequiresConfirmation: true,
			IdempotencyRequired:  true,
			AllowedResources:     []string{"automation", "workflow", "agent"},
			Scopes:               []string{"automation:write"},
		},
		{
			ID:                   "skill.execute",
			Domain:               "skill",
			Action:               "execute",
			Name:                 "Execute skill tool",
			Description:          "Execute an approved skill tool through the action control plane",
			Runtime:              RuntimeSkill,
			AuthMode:             AuthModeInvocationToken,
			RiskLevel:            actionmodel.RiskLevelMedium,
			RequiresConfirmation: true,
			IdempotencyRequired:  true,
			AllowedResources:     []string{"skill", "tool"},
			Scopes:               []string{"skill:execute"},
		},
	}
}

func cloneCapability(item CapabilityManifest) CapabilityManifest {
	item.AllowedResources = append([]string(nil), item.AllowedResources...)
	item.Scopes = append([]string(nil), item.Scopes...)
	return item
}
