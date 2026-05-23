package memory

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/google/uuid"
	"github.com/zgiai/zgi/api/internal/modules/tools"
	"github.com/zgiai/zgi/api/internal/modules/tools/builtin"
)

const HiddenProviderTag = "__hidden"

type Provider struct {
	*builtin.BuiltinProvider
	service *Service
}

func NewProvider(service *Service) *Provider {
	identity := tools.ToolProviderIdentity{
		Name:   ProviderID,
		Author: "System",
		Label: tools.I18nText{
			"en_US":   "User Memory",
			"zh_Hans": "用户记忆",
		},
		Description: tools.I18nText{
			"en_US":   "Private account-level user memory tools.",
			"zh_Hans": "账号级私有用户记忆工具。",
		},
		Icon: "brain",
		Tags: []string{"system", HiddenProviderTag},
	}
	provider := &Provider{
		BuiltinProvider: builtin.NewBuiltinProvider(identity),
		service:         service,
	}
	provider.RegisterTool(newReadMemoryTool(service))
	provider.RegisterTool(newAddMemoryTool(service))
	provider.RegisterTool(newUpdateMemoryTool(service))
	provider.RegisterTool(newDeleteMemoryTool(service))
	provider.RegisterTool(newListTemporaryMemoriesTool(service))
	return provider
}

type memoryTool struct {
	*builtin.BuiltinTool
	service *Service
	kind    string
}

func newReadMemoryTool(service *Service) tools.Tool {
	return newMemoryTool(service, "read_user_memory", "Read User Memory", "Read the current user's saved account memory entries.", nil)
}

func newAddMemoryTool(service *Service) tools.Tool {
	return newMemoryTool(service, "add_user_memory", "Add User Memory", "Save a new memory entry for the current user.", []tools.ToolParameter{
		stringParam("content", "Memory content", "The concise user memory to save. Use this field for the memory text.", true),
		categoryParam(),
		memoryTypeParam(),
		stringParam("expires_at", "Expires at", "RFC3339 expiration time for temporary memory. Required when memory_type is temporary. Do not use relative dates.", false),
	})
}

func newUpdateMemoryTool(service *Service) tools.Tool {
	return newMemoryTool(service, "update_user_memory", "Update User Memory", "Update one memory entry that belongs to the current user.", []tools.ToolParameter{
		stringParam("entry_id", "Entry ID", "The memory entry id returned by read_user_memory.", true),
		stringParam("content", "Memory content", "Updated memory content. Omit when only changing category or enabled.", false),
		categoryParam(),
		memoryTypeParam(),
		stringParam("expires_at", "Expires at", "RFC3339 expiration time for temporary memory. Required when memory_type is temporary. Use an empty value only when converting to long_term.", false),
		{
			Name:           "enabled",
			Label:          tools.I18nText{"en_US": "Enabled", "zh_Hans": "启用"},
			LLMDescription: "Whether this memory entry should be included in future memory context.",
			Type:           tools.ToolParameterTypeBoolean,
			Form:           tools.ToolParameterFormLLM,
			Required:       false,
		},
	})
}

func newDeleteMemoryTool(service *Service) tools.Tool {
	return newMemoryTool(service, "delete_user_memory", "Delete User Memory", "Delete one memory entry that belongs to the current user.", []tools.ToolParameter{
		stringParam("entry_id", "Entry ID", "The memory entry id returned by read_user_memory.", true),
	})
}

func newListTemporaryMemoriesTool(service *Service) tools.Tool {
	return newMemoryTool(service, "list_temporary_memories", "List Temporary Memories", "List active or expired temporary memory entries for retrospective user questions. Expired memories are historical and not current facts.", []tools.ToolParameter{
		statusParam(),
		{
			Name:           "limit",
			Label:          tools.I18nText{"en_US": "Limit", "zh_Hans": "Limit"},
			LLMDescription: "Maximum number of temporary memories to return. Defaults to 20 and is capped at 100.",
			Type:           tools.ToolParameterTypeNumber,
			Form:           tools.ToolParameterFormLLM,
			Required:       false,
		},
	})
}

func newMemoryTool(service *Service, name, label, description string, params []tools.ToolParameter) *memoryTool {
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
		Tags:       []string{"memory", "system"},
	}
	return &memoryTool{
		BuiltinTool: builtin.NewBuiltinTool(entity, ""),
		service:     service,
		kind:        name,
	}
}

func (t *memoryTool) Invoke(ctx context.Context, userID string, params map[string]interface{}, conversationID *string, appID *string, messageID *string) ([]tools.ToolInvokeMessage, error) {
	accountID, err := ResolveToolAccountID(userID)
	if err != nil {
		return nil, err
	}
	enabled, err := t.service.IsEnabled(ctx, accountID)
	if err != nil {
		return nil, err
	}
	if !enabled {
		return nil, ErrDisabled
	}
	switch t.kind {
	case "read_user_memory":
		state, err := t.service.GetMe(ctx, accountID)
		if err != nil {
			return nil, err
		}
		return jsonMessages(memoryToolStateResponse(state))
	case "add_user_memory":
		content := memoryContentValue(params)
		entry, err := t.service.CreateEntryWithMetadata(ctx, accountID, CreateEntryRequest{
			Content:    content,
			Category:   stringValue(params, "category"),
			MemoryType: stringValue(params, "memory_type"),
			ExpiresAt:  stringValue(params, "expires_at"),
		}, toolMutationMetadata(conversationID, messageID))
		if err != nil {
			return nil, err
		}
		return jsonMessages(memoryToolEntryResponse(entry))
	case "update_user_memory":
		entryID, err := parseEntryID(params)
		if err != nil {
			return nil, err
		}
		req := UpdateEntryRequest{}
		if content := memoryContentValue(params); content != "" {
			req.Content = &content
		}
		if category := stringValue(params, "category"); category != "" {
			req.Category = &category
		}
		if memoryType := stringValue(params, "memory_type"); memoryType != "" {
			req.MemoryType = &memoryType
		}
		if expiresAt := stringValue(params, "expires_at"); expiresAt != "" {
			req.ExpiresAt = &expiresAt
		}
		if enabled, ok := boolValue(params, "enabled"); ok {
			req.Enabled = &enabled
		}
		entry, err := t.service.UpdateEntryWithMetadata(ctx, accountID, entryID, req, toolMutationMetadata(conversationID, messageID))
		if err != nil {
			return nil, err
		}
		return jsonMessages(memoryToolEntryResponse(entry))
	case "delete_user_memory":
		entryID, err := parseEntryID(params)
		if err != nil {
			return nil, err
		}
		if err := t.service.DeleteEntryWithMetadata(ctx, accountID, entryID, toolMutationMetadata(conversationID, messageID)); err != nil {
			return nil, err
		}
		return jsonMessages(map[string]interface{}{"result": "success", "entry_id": entryID.String()})
	case "list_temporary_memories":
		entries, err := t.service.ListTemporaryEntries(ctx, accountID, stringValue(params, "status"), intValue(params, "limit"))
		if err != nil {
			return nil, err
		}
		out := make([]map[string]interface{}, 0, len(entries))
		for _, entry := range entries {
			out = append(out, memoryToolEntryMap(entry))
		}
		return jsonMessages(map[string]interface{}{"entries": out})
	default:
		return nil, fmt.Errorf("unknown memory tool %s", t.kind)
	}
}

func (t *memoryTool) ForkToolRuntime(runtime *tools.ToolRuntime) tools.Tool {
	return &memoryTool{
		BuiltinTool: t.BuiltinTool.ForkToolRuntime(runtime),
		service:     t.service,
		kind:        t.kind,
	}
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

func categoryParam() tools.ToolParameter {
	return tools.ToolParameter{
		Name:           "category",
		Label:          tools.I18nText{"en_US": "Category", "zh_Hans": "分类"},
		LLMDescription: "One of preference, profile, instruction, fact, or other.",
		Type:           tools.ToolParameterTypeSelect,
		Form:           tools.ToolParameterFormLLM,
		Required:       false,
		Default:        CategoryOther,
		Options: []tools.ToolParameterOption{
			{Value: CategoryPreference, Label: tools.I18nText{"en_US": "Preference", "zh_Hans": "偏好"}},
			{Value: CategoryProfile, Label: tools.I18nText{"en_US": "Profile", "zh_Hans": "画像"}},
			{Value: CategoryInstruction, Label: tools.I18nText{"en_US": "Instruction", "zh_Hans": "指令"}},
			{Value: CategoryFact, Label: tools.I18nText{"en_US": "Fact", "zh_Hans": "事实"}},
			{Value: CategoryOther, Label: tools.I18nText{"en_US": "Other", "zh_Hans": "其他"}},
		},
	}
}

func memoryTypeParam() tools.ToolParameter {
	return tools.ToolParameter{
		Name:           "memory_type",
		Label:          tools.I18nText{"en_US": "Memory type", "zh_Hans": "Memory type"},
		LLMDescription: "Use long_term for durable preferences/facts. Use temporary for time-limited plans, one-off constraints, or short-lived context. Temporary memory requires expires_at.",
		Type:           tools.ToolParameterTypeSelect,
		Form:           tools.ToolParameterFormLLM,
		Required:       false,
		Default:        MemoryTypeLongTerm,
		Options: []tools.ToolParameterOption{
			{Value: MemoryTypeLongTerm, Label: tools.I18nText{"en_US": "Long-term", "zh_Hans": "Long-term"}},
			{Value: MemoryTypeTemporary, Label: tools.I18nText{"en_US": "Temporary", "zh_Hans": "Temporary"}},
		},
	}
}

func statusParam() tools.ToolParameter {
	return tools.ToolParameter{
		Name:           "status",
		Label:          tools.I18nText{"en_US": "Status", "zh_Hans": "Status"},
		LLMDescription: "Which temporary memories to list. Use expired only for retrospective questions; expired memories are not current.",
		Type:           tools.ToolParameterTypeSelect,
		Form:           tools.ToolParameterFormLLM,
		Required:       false,
		Default:        memoryStatusActive,
		Options: []tools.ToolParameterOption{
			{Value: memoryStatusActive, Label: tools.I18nText{"en_US": "Active", "zh_Hans": "Active"}},
			{Value: memoryStatusExpired, Label: tools.I18nText{"en_US": "Expired", "zh_Hans": "Expired"}},
			{Value: "all", Label: tools.I18nText{"en_US": "All", "zh_Hans": "All"}},
		},
	}
}

func parseEntryID(params map[string]interface{}) (uuid.UUID, error) {
	raw := stringValue(params, "entry_id")
	if raw == "" {
		raw = stringValue(params, "id")
	}
	entryID, err := uuid.Parse(raw)
	if err != nil {
		return uuid.Nil, fmt.Errorf("%w: invalid entry_id", ErrInvalidInput)
	}
	return entryID, nil
}

func memoryToolStateResponse(state *MemoryMeResponse) map[string]interface{} {
	if state == nil {
		return map[string]interface{}{
			"enabled":    false,
			"entries":    []interface{}{},
			"updated_at": int64(0),
		}
	}
	entries := make([]map[string]interface{}, 0, len(state.Entries))
	for _, entry := range state.Entries {
		entries = append(entries, memoryToolEntryMap(entry))
	}
	return map[string]interface{}{
		"enabled":    state.Enabled,
		"entries":    entries,
		"updated_at": state.UpdatedAt,
	}
}

func memoryToolEntryResponse(entry *MemoryEntryResponse) map[string]interface{} {
	if entry == nil {
		return map[string]interface{}{}
	}
	return memoryToolEntryMap(*entry)
}

func memoryToolEntryMap(entry MemoryEntryResponse) map[string]interface{} {
	return map[string]interface{}{
		"id":          entry.ID,
		"entry_id":    entry.ID,
		"content":     entry.Content,
		"category":    entry.Category,
		"memory_type": entry.MemoryType,
		"expires_at":  entry.ExpiresAt,
		"status":      entry.Status,
		"enabled":     entry.Enabled,
		"created_at":  entry.CreatedAt,
		"updated_at":  entry.UpdatedAt,
	}
}

func stringValue(params map[string]interface{}, key string) string {
	if params == nil {
		return ""
	}
	value, _ := params[key].(string)
	return value
}

func memoryContentValue(params map[string]interface{}) string {
	for _, key := range []string{"content", "memory", "text"} {
		if value := stringValue(params, key); value != "" {
			return value
		}
	}
	return ""
}

func boolValue(params map[string]interface{}, key string) (bool, bool) {
	if params == nil {
		return false, false
	}
	value, ok := params[key].(bool)
	return value, ok
}

func intValue(params map[string]interface{}, key string) int {
	if params == nil {
		return 0
	}
	switch value := params[key].(type) {
	case int:
		return value
	case int64:
		return int(value)
	case float64:
		return int(value)
	case json.Number:
		intValue, _ := value.Int64()
		return int(intValue)
	default:
		return 0
	}
}

func toolMutationMetadata(conversationID *string, messageID *string) MutationMetadata {
	meta := MutationMetadata{
		ActorType: EventActorModel,
		Source:    EventSourceAIChat,
	}
	if conversationID != nil {
		if id, err := uuid.Parse(*conversationID); err == nil {
			meta.SourceConversationID = &id
		}
	}
	if messageID != nil {
		if id, err := uuid.Parse(*messageID); err == nil {
			meta.SourceMessageID = &id
		}
	}
	return meta
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
var _ tools.Tool = (*memoryTool)(nil)
