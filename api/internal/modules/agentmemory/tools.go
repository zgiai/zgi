package agentmemory

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/google/uuid"
	"github.com/zgiai/zgi/api/internal/modules/tools"
	"github.com/zgiai/zgi/api/internal/modules/tools/builtin"
)

type Provider struct {
	*builtin.BuiltinProvider
	service *Service
}

const HiddenProviderTag = "__hidden"

func NewProvider(service *Service) *Provider {
	identity := tools.ToolProviderIdentity{
		Name:   ProviderID,
		Author: "System",
		Label: tools.I18nText{
			"en_US":   "Agent Memory",
			"zh_Hans": "Agent Memory",
		},
		Description: tools.I18nText{
			"en_US":   "Agent-scoped fixed-slot memory tools.",
			"zh_Hans": "Agent-scoped fixed-slot memory tools.",
		},
		Icon: "brain",
		Tags: []string{"system", "memory", HiddenProviderTag},
	}
	provider := &Provider{
		BuiltinProvider: builtin.NewBuiltinProvider(identity),
		service:         service,
	}
	provider.RegisterTool(newReadAgentMemoryTool(service))
	provider.RegisterTool(newUpdateAgentMemoryTool(service))
	provider.RegisterTool(newClearAgentMemoryTool(service))
	return provider
}

type agentMemoryTool struct {
	*builtin.BuiltinTool
	service *Service
	kind    string
}

func newReadAgentMemoryTool(service *Service) tools.Tool {
	return newAgentMemoryTool(service, "read_agent_memory", "Read Agent Memory", "Read the fixed memory slots configured by this agent's organizer for the current user. Do not invent new keys.", nil)
}

func newUpdateAgentMemoryTool(service *Service) tools.Tool {
	return newAgentMemoryTool(service, "update_agent_memory", "Update Agent Memory", "Update one existing fixed memory key for this agent and current user. The key must already be listed by read_agent_memory.", []tools.ToolParameter{
		stringParam("key", "Key", "Existing memory key configured by the agent organizer.", true),
		stringParam("content", "Content", "Concise content to store in this fixed memory key. Must fit the key's max_chars limit.", true),
	})
}

func newClearAgentMemoryTool(service *Service) tools.Tool {
	return newAgentMemoryTool(service, "clear_agent_memory", "Clear Agent Memory", "Clear one existing fixed memory key for this agent and current user.", []tools.ToolParameter{
		stringParam("key", "Key", "Existing memory key configured by the agent organizer.", true),
	})
}

func newAgentMemoryTool(service *Service, name, label, description string, params []tools.ToolParameter) *agentMemoryTool {
	entity := tools.ToolEntity{
		Identity: tools.ToolIdentity{
			Name:     name,
			Author:   "System",
			Provider: ProviderID,
			Label:    tools.I18nText{"en_US": label, "zh_Hans": label},
			Icon:     "brain",
		},
		Description: tools.ToolDescription{
			Human: tools.I18nText{"en_US": description, "zh_Hans": description},
			LLM:   description,
		},
		Parameters: params,
		OutputType: "json",
		Tags:       []string{"memory", "system", "agent"},
	}
	return &agentMemoryTool{
		BuiltinTool: builtin.NewBuiltinTool(entity, ""),
		service:     service,
		kind:        name,
	}
}

func (t *agentMemoryTool) Invoke(ctx context.Context, userID string, params map[string]interface{}, conversationID *string, appID *string, messageID *string) ([]tools.ToolInvokeMessage, error) {
	runtime := t.Runtime()
	if runtime == nil || runtime.InvokeFrom != tools.ToolInvokeFromAgent {
		return nil, ErrUnauthorized
	}
	scope, err := resolveToolScope(runtime, userID, appID)
	if err != nil {
		return nil, err
	}

	switch t.kind {
	case "read_agent_memory":
		entries, err := t.service.ReadUserMemory(ctx, scope.workspaceID, scope.agentID, scope.slots, scope.userScope, scope.userID)
		if err != nil {
			return nil, err
		}
		out := make([]map[string]interface{}, 0, len(entries))
		for _, entry := range entries {
			out = append(out, agentMemoryEntryMap(entry))
		}
		return jsonMessages(map[string]interface{}{"entries": out})
	case "update_agent_memory":
		entry, err := t.service.UpdateValue(ctx, scope.workspaceID, scope.agentID, scope.slots, scope.userScope, scope.userID, UpdateValueRequest{
			Key:     stringValue(params, "key"),
			Content: stringValue(params, "content"),
		}, modelMetadata(conversationID, messageID))
		if err != nil {
			return nil, err
		}
		return jsonMessages(agentMemoryEntryMap(*entry))
	case "clear_agent_memory":
		entry, err := t.service.ClearValue(ctx, scope.workspaceID, scope.agentID, scope.slots, scope.userScope, scope.userID, stringValue(params, "key"), modelMetadata(conversationID, messageID))
		if err != nil {
			return nil, err
		}
		return jsonMessages(agentMemoryEntryMap(*entry))
	default:
		return nil, fmt.Errorf("unknown agent memory tool %s", t.kind)
	}
}

func (t *agentMemoryTool) ForkToolRuntime(runtime *tools.ToolRuntime) tools.Tool {
	return &agentMemoryTool{
		BuiltinTool: t.BuiltinTool.ForkToolRuntime(runtime),
		service:     t.service,
		kind:        t.kind,
	}
}

type toolScope struct {
	workspaceID uuid.UUID
	agentID     uuid.UUID
	userScope   string
	userID      uuid.UUID
	slots       []RuntimeSlot
}

func resolveToolScope(runtime *tools.ToolRuntime, userID string, appID *string) (toolScope, error) {
	if runtime == nil || runtime.RuntimeParameters == nil {
		return toolScope{}, ErrUnauthorized
	}
	workspaceID, err := uuid.Parse(stringValue(runtime.RuntimeParameters, "workspace_id"))
	if err != nil {
		return toolScope{}, ErrUnauthorized
	}
	agentRaw := stringValue(runtime.RuntimeParameters, "agent_id")
	if agentRaw == "" && appID != nil {
		agentRaw = *appID
	}
	if agentRaw == "" {
		return toolScope{}, ErrUnauthorized
	}
	agentID, err := uuid.Parse(agentRaw)
	if err != nil {
		return toolScope{}, ErrUnauthorized
	}
	scopedUserID, err := uuid.Parse(userID)
	if err != nil {
		return toolScope{}, ErrUnauthorized
	}
	userScope := normalizeUserScope(stringValue(runtime.RuntimeParameters, "user_scope"))
	slots := runtimeSlotsFromValue(runtime.RuntimeParameters["agent_memory_slots"])
	if len(slots) == 0 {
		return toolScope{}, fmt.Errorf("%w: agent memory has no configured slots", ErrInvalidInput)
	}
	return toolScope{
		workspaceID: workspaceID,
		agentID:     agentID,
		userScope:   userScope,
		userID:      scopedUserID,
		slots:       slots,
	}, nil
}

func runtimeSlotsFromValue(raw interface{}) []RuntimeSlot {
	if raw == nil {
		return nil
	}
	data, err := json.Marshal(raw)
	if err != nil {
		return nil
	}
	var slots []RuntimeSlot
	if err := json.Unmarshal(data, &slots); err != nil {
		return nil
	}
	return normalizeRuntimeSlots(slots)
}

func stringParam(name, label, description string, required bool) tools.ToolParameter {
	return tools.ToolParameter{
		Name:           name,
		Label:          tools.I18nText{"en_US": label, "zh_Hans": label},
		LLMDescription: description,
		Type:           tools.ToolParameterTypeString,
		Form:           tools.ToolParameterFormLLM,
		Required:       required,
	}
}

func stringValue(params map[string]interface{}, key string) string {
	if params == nil {
		return ""
	}
	value, _ := params[key].(string)
	return value
}

func agentMemoryEntryMap(entry SlotValueResponse) map[string]interface{} {
	return map[string]interface{}{
		"id":          entry.ID,
		"key":         entry.Key,
		"description": entry.Description,
		"max_chars":   entry.MaxChars,
		"enabled":     entry.Enabled,
		"sort_order":  entry.SortOrder,
		"content":     entry.Content,
		"created_at":  entry.CreatedAt,
		"updated_at":  entry.UpdatedAt,
	}
}

func jsonMessages(value interface{}) ([]tools.ToolInvokeMessage, error) {
	data, err := json.Marshal(value)
	if err != nil {
		return nil, err
	}
	return []tools.ToolInvokeMessage{
		builtin.CreateTextMessage(string(data)),
	}, nil
}

var _ tools.ToolProvider = (*Provider)(nil)
var _ tools.Tool = (*agentMemoryTool)(nil)
