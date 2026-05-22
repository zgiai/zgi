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
	return newMemoryTool(service, "add_user_memory", "Add User Memory", "Save a new durable memory entry for the current user.", []tools.ToolParameter{
		stringParam("content", "Memory content", "The concise user memory to save.", true),
		categoryParam(),
	})
}

func newUpdateMemoryTool(service *Service) tools.Tool {
	return newMemoryTool(service, "update_user_memory", "Update User Memory", "Update one memory entry that belongs to the current user.", []tools.ToolParameter{
		stringParam("entry_id", "Entry ID", "The memory entry id returned by read_user_memory.", true),
		stringParam("content", "Memory content", "Updated memory content. Omit when only changing category or enabled.", false),
		categoryParam(),
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
	switch t.kind {
	case "read_user_memory":
		state, err := t.service.GetMe(ctx, accountID)
		if err != nil {
			return nil, err
		}
		return jsonMessages(state)
	case "add_user_memory":
		content := stringValue(params, "content")
		entry, err := t.service.CreateEntry(ctx, accountID, CreateEntryRequest{
			Content:  content,
			Category: stringValue(params, "category"),
		})
		if err != nil {
			return nil, err
		}
		return jsonMessages(entry)
	case "update_user_memory":
		entryID, err := parseEntryID(params)
		if err != nil {
			return nil, err
		}
		req := UpdateEntryRequest{}
		if content := stringValue(params, "content"); content != "" {
			req.Content = &content
		}
		if category := stringValue(params, "category"); category != "" {
			req.Category = &category
		}
		if enabled, ok := boolValue(params, "enabled"); ok {
			req.Enabled = &enabled
		}
		entry, err := t.service.UpdateEntry(ctx, accountID, entryID, req)
		if err != nil {
			return nil, err
		}
		return jsonMessages(entry)
	case "delete_user_memory":
		entryID, err := parseEntryID(params)
		if err != nil {
			return nil, err
		}
		if err := t.service.DeleteEntry(ctx, accountID, entryID); err != nil {
			return nil, err
		}
		return jsonMessages(map[string]interface{}{"result": "success", "entry_id": entryID.String()})
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

func parseEntryID(params map[string]interface{}) (uuid.UUID, error) {
	entryID, err := uuid.Parse(stringValue(params, "entry_id"))
	if err != nil {
		return uuid.Nil, fmt.Errorf("%w: invalid entry_id", ErrInvalidInput)
	}
	return entryID, nil
}

func stringValue(params map[string]interface{}, key string) string {
	if params == nil {
		return ""
	}
	value, _ := params[key].(string)
	return value
}

func boolValue(params map[string]interface{}, key string) (bool, bool) {
	if params == nil {
		return false, false
	}
	value, ok := params[key].(bool)
	return value, ok
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
