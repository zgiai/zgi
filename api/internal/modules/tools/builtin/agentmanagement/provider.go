package agentmanagement

import (
	"context"
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
	"unicode"

	"github.com/google/uuid"
	"github.com/zgiai/zgi/api/internal/dto"
	llmmodelmodel "github.com/zgiai/zgi/api/internal/modules/llm/llmmodel/model"
	llmmodelservice "github.com/zgiai/zgi/api/internal/modules/llm/llmmodel/service"
	interfaces "github.com/zgiai/zgi/api/internal/modules/shared/interface"
	"github.com/zgiai/zgi/api/internal/modules/tools"
	"github.com/zgiai/zgi/api/internal/modules/tools/builtin"
	workspacemodel "github.com/zgiai/zgi/api/internal/modules/workspace/model"
)

const (
	ProviderID                               = "agent_management"
	ToolListAgents                           = "list_agents"
	ToolGetAgent                             = "get_agent"
	ToolCreateAgent                          = "create_agent"
	ToolUpdateAgentIdentity                  = "update_agent_identity"
	ToolDeleteAgent                          = "delete_agent"
	ToolDeleteAgents                         = "delete_agents"
	ToolGetAgentConfig                       = "get_agent_config"
	ToolUpdateAgentConfig                    = "update_agent_config"
	ToolReplaceAgentMemorySlots              = "replace_agent_memory_slots"
	ToolListAgentSkillCandidates             = "list_agent_skill_candidates"
	ToolListAgentKnowledgeCandidates         = "list_agent_knowledge_candidates"
	ToolListAgentDatabaseCandidates          = "list_agent_database_candidates"
	ToolListAgentDatabaseTables              = "list_agent_database_tables"
	ToolListAgentWorkflowBindingCandidates   = "list_agent_workflow_binding_candidates"
	ToolReplaceAgentSkillBindings            = "replace_agent_skill_bindings"
	ToolReplaceAgentKnowledgeBindings        = "replace_agent_knowledge_bindings"
	ToolReplaceAgentDatabaseBindings         = "replace_agent_database_bindings"
	ToolReplaceAgentWorkflowBindings         = "replace_agent_workflow_bindings"
	ToolListAvailableModels                  = "list_available_models"
	defaultAgentListPageSize                 = 20
	defaultAgentBindingCandidateListPageSize = 20
	maxAgentBindingCandidateListPageSize     = 100
	defaultAgentModelListUseCase             = string(llmmodelmodel.UseCaseTextChat)
	defaultAgentModelListPageSize            = 20
	maxAgentModelListPageSize                = 100
	defaultAgentTextIconBackground           = "#0847f7"
)

type WorkspacePermissionService interface {
	CheckWorkspacePermission(ctx context.Context, organizationID, workspaceID, accountID string, permissionCode workspacemodel.WorkspacePermissionCode) (bool, error)
}

type AvailableModelsService interface {
	ListAvailable(ctx context.Context, organizationID uuid.UUID, provider string, useCase string) ([]*llmmodelservice.AvailableModel, error)
}

type Provider struct {
	*builtin.BuiltinProvider
	agentsService   interfaces.AgentsService
	workspacePerms  WorkspacePermissionService
	availableModels AvailableModelsService
}

func NewProvider(agentsService interfaces.AgentsService, workspacePerms WorkspacePermissionService, availableModels AvailableModelsService) *Provider {
	identity := tools.ToolProviderIdentity{
		Name:   ProviderID,
		Author: "System",
		Label: tools.I18nText{
			"en_US":   "Agent Management",
			"zh_Hans": "Agent Management",
		},
		Description: tools.I18nText{
			"en_US":   "Built-in governed AIChat tools for managing Agent assets.",
			"zh_Hans": "Built-in governed AIChat tools for managing Agent assets.",
		},
		Icon: "bot",
		Tags: []string{"agent", "management", "aichat", "system"},
	}
	provider := &Provider{
		BuiltinProvider: builtin.NewBuiltinProvider(identity),
		agentsService:   agentsService,
		workspacePerms:  workspacePerms,
		availableModels: availableModels,
	}
	provider.RegisterTool(newListAgentsTool(agentsService))
	provider.RegisterTool(newGetAgentTool(agentsService))
	provider.RegisterTool(newCreateAgentTool(agentsService, workspacePerms))
	provider.RegisterTool(newUpdateAgentIdentityTool(agentsService))
	provider.RegisterTool(newDeleteAgentTool(agentsService))
	provider.RegisterTool(newDeleteAgentsTool(agentsService))
	provider.RegisterTool(newGetAgentConfigTool(agentsService))
	provider.RegisterTool(newUpdateAgentConfigTool(agentsService))
	provider.RegisterTool(newReplaceAgentMemorySlotsTool(agentsService))
	provider.RegisterTool(newListAgentSkillCandidatesTool(agentsService))
	provider.RegisterTool(newListAgentKnowledgeCandidatesTool(agentsService))
	provider.RegisterTool(newListAgentDatabaseCandidatesTool(agentsService))
	provider.RegisterTool(newListAgentDatabaseTablesTool(agentsService))
	provider.RegisterTool(newListAgentWorkflowBindingCandidatesTool(agentsService))
	provider.RegisterTool(newReplaceAgentSkillBindingsTool(agentsService))
	provider.RegisterTool(newReplaceAgentKnowledgeBindingsTool(agentsService))
	provider.RegisterTool(newReplaceAgentDatabaseBindingsTool(agentsService))
	provider.RegisterTool(newReplaceAgentWorkflowBindingsTool(agentsService))
	provider.RegisterTool(newListAvailableModelsTool(availableModels))
	return provider
}

func (p *Provider) ValidateCredentials(ctx context.Context, credentials map[string]interface{}) error {
	return nil
}

type agentToolBase struct {
	*builtin.BuiltinTool
	agentsService   interfaces.AgentsService
	workspacePerms  WorkspacePermissionService
	availableModels AvailableModelsService
}

type listAgentsTool struct{ agentToolBase }
type getAgentTool struct{ agentToolBase }
type createAgentTool struct{ agentToolBase }
type updateAgentIdentityTool struct{ agentToolBase }
type deleteAgentTool struct{ agentToolBase }
type deleteAgentsTool struct{ agentToolBase }
type getAgentConfigTool struct{ agentToolBase }
type updateAgentConfigTool struct{ agentToolBase }
type replaceAgentMemorySlotsTool struct{ agentToolBase }
type listAgentSkillCandidatesTool struct{ agentToolBase }
type listAgentKnowledgeCandidatesTool struct{ agentToolBase }
type listAgentDatabaseCandidatesTool struct{ agentToolBase }
type listAgentDatabaseTablesTool struct{ agentToolBase }
type listAgentWorkflowBindingCandidatesTool struct{ agentToolBase }
type replaceAgentSkillBindingsTool struct{ agentToolBase }
type replaceAgentKnowledgeBindingsTool struct{ agentToolBase }
type replaceAgentDatabaseBindingsTool struct{ agentToolBase }
type replaceAgentWorkflowBindingsTool struct{ agentToolBase }
type listAvailableModelsTool struct{ agentToolBase }

type agentScope struct {
	OrganizationID string
	WorkspaceID    string
	AccountID      string
	InvokeFrom     tools.ToolInvokeFrom
}

func newListAgentsTool(agentsService interfaces.AgentsService) tools.Tool {
	return &listAgentsTool{agentToolBase: newAgentToolBase(agentToolEntity(
		ToolListAgents,
		"List Agents",
		"List or search Agent assets visible to the current AIChat user in the current workspace. Use this only when current page visible_agents are missing, insufficient, ambiguous, or the user asks to search beyond the visible page.",
		"list",
		[]tools.ToolParameter{
			stringParameter("workspace_id", "Workspace ID", "Optional workspace ID. Usually omit so current AIChat workspace context is used.", false),
			stringParameter("keyword", "Keyword", "Optional search keyword for Agent name or description.", false),
			numberParameter("limit", "Limit", "Optional maximum number of Agents to return, capped at 100.", false),
		},
	), agentsService, nil, nil)}
}

func newGetAgentTool(agentsService interfaces.AgentsService) tools.Tool {
	return &getAgentTool{agentToolBase: newAgentToolBase(agentToolEntity(
		ToolGetAgent,
		"Get Agent",
		"Read basic details for one resolved Agent asset visible to the current AIChat user.",
		"bot",
		[]tools.ToolParameter{
			stringParameter("agent_id", "Agent ID", "Required Agent ID from page context, list_agents, or governed asset resolution. Do not invent IDs.", true),
		},
	), agentsService, nil, nil)}
}

func newCreateAgentTool(agentsService interfaces.AgentsService, workspacePerms WorkspacePermissionService) tools.Tool {
	return &createAgentTool{agentToolBase: newAgentToolBase(agentToolEntity(
		ToolCreateAgent,
		"Create Agent",
		"Create one AGENT asset in the current workspace after governance and permission checks. This only creates a draft Agent and does not publish it.",
		"bot-message-square",
		[]tools.ToolParameter{
			stringParameter("name", "Name", "Required Agent name shown in the Agent list.", true),
			stringParameter("description", "Description", "Optional Agent description.", false),
			stringParameter("icon_type", "Icon type", "Optional icon type. Use text for a text or emoji icon, or image for an uploaded image file ID/URL.", false),
			stringParameter("icon", "Icon", "Optional icon value. For text icons pass the visible text, for example AI or BOT.", false),
			stringParameter("icon_background", "Icon background", "Optional text icon background color such as #0f766e. Provide with icon_type=text and icon.", false),
			stringParameter("workspace_id", "Workspace ID", "Optional target workspace ID. Usually omit so current AIChat workspace context is used.", false),
		},
	), agentsService, workspacePerms, nil)}
}

func newUpdateAgentIdentityTool(agentsService interfaces.AgentsService) tools.Tool {
	return &updateAgentIdentityTool{agentToolBase: newAgentToolBase(agentToolEntity(
		ToolUpdateAgentIdentity,
		"Update Agent Identity",
		"Update one resolved Agent's name, description, or icon. This does not publish the Agent.",
		"edit-3",
		[]tools.ToolParameter{
			stringParameter("agent_id", "Agent ID", "Required Agent ID from page context, list_agents, or governed asset resolution. Do not invent IDs.", true),
			stringParameter("name", "Name", "Optional new Agent name.", false),
			stringParameter("description", "Description", "Optional new Agent description.", false),
			stringParameter("icon_type", "Icon type", "Optional new icon type. Use text for a text or emoji icon, or image for an uploaded image file ID/URL.", false),
			stringParameter("icon", "Icon", "Optional new icon value. For text icons pass the visible text, for example AI or BOT.", false),
			stringParameter("icon_background", "Icon background", "Optional text icon background color such as #0f766e. Provide with icon_type=text and icon.", false),
		},
	), agentsService, nil, nil)}
}

func newDeleteAgentTool(agentsService interfaces.AgentsService) tools.Tool {
	return &deleteAgentTool{agentToolBase: newAgentToolBase(agentToolEntity(
		ToolDeleteAgent,
		"Delete Agent",
		"Delete one resolved Agent after explicit governance approval. This operation is destructive.",
		"trash-2",
		[]tools.ToolParameter{
			stringParameter("agent_id", "Agent ID", "Required Agent ID from page context, list_agents, or governed asset resolution. Do not invent IDs.", true),
		},
	), agentsService, nil, nil)}
}

func newDeleteAgentsTool(agentsService interfaces.AgentsService) tools.Tool {
	return &deleteAgentsTool{agentToolBase: newAgentToolBase(agentToolEntity(
		ToolDeleteAgents,
		"Delete Agents",
		"Delete multiple resolved Agent assets as one governed frozen batch. Use this for plural/range requests such as deleting the first N visible Agents; pass the exact target list in agents and do not call delete_agent repeatedly.",
		"trash-2",
		[]tools.ToolParameter{
			stringParameter("agents", "Agents", "Required JSON array of resolved target Agents. Each item should include agent_id and visible name, for example [{\"agent_id\":\"...\",\"name\":\"...\"}]. The approved target list is frozen.", true),
			stringArrayParameter("agent_ids", "Agent IDs", "Optional fallback list of Agent IDs when agents is unavailable. Prefer agents so governance can show names.", false),
		},
	), agentsService, nil, nil)}
}

func newGetAgentConfigTool(agentsService interfaces.AgentsService) tools.Tool {
	return &getAgentConfigTool{agentToolBase: newAgentToolBase(agentToolEntity(
		ToolGetAgentConfig,
		"Get Agent Config",
		"Read the current draft runtime configuration for one resolved AGENT asset.",
		"settings",
		[]tools.ToolParameter{
			stringParameter("agent_id", "Agent ID", "Required Agent ID from page context, list_agents, or governed asset resolution. Do not invent IDs.", true),
		},
	), agentsService, nil, nil)}
}

func newUpdateAgentConfigTool(agentsService interfaces.AgentsService) tools.Tool {
	return &updateAgentConfigTool{agentToolBase: newAgentToolBase(agentToolEntity(
		ToolUpdateAgentConfig,
		"Update Agent Config",
		"Patch selected draft runtime configuration fields for one resolved AGENT asset. Omitted fields are preserved. Prefer this tool for one-step config changes, including complete skill/knowledge/database/workflow binding replacement after exact candidates are known.",
		"sliders-horizontal",
		[]tools.ToolParameter{
			stringParameter("agent_id", "Agent ID", "Required Agent ID from page context, list_agents, or governed asset resolution. Do not invent IDs.", true),
			stringParameter("system_prompt", "System prompt", "Optional replacement system prompt.", false),
			stringParameter("model_provider", "Model provider", "Required whenever model is provided. Use the exact provider returned by list_available_models.", false),
			stringParameter("model", "Model", "Optional replacement model ID. When changing this field, also provide model_provider from the same list_available_models item.", false),
			objectParameter("model_parameters", "Model parameters", "Optional replacement model parameter object.", false),
			stringArrayParameter("enabled_skill_ids", "Enabled skill IDs", "Optional full list of enabled user-selectable skill IDs.", false),
			boolParameter("agent_memory_enabled", "Agent memory enabled", "Optional Agent memory switch.", false),
			boolParameter("file_upload_enabled", "File upload enabled", "Optional file upload switch.", false),
			stringParameter("home_title", "Home title", "Optional Agent home title.", false),
			stringParameter("input_placeholder", "Input placeholder", "Optional chat input placeholder.", false),
			stringParameter("theme_color", "Theme color", "Optional theme color: default, blue, emerald, violet, rose, amber, or slate.", false),
			stringArrayParameter("suggested_questions", "Suggested questions", "Optional full list of suggested questions.", false),
			stringArrayParameter("knowledge_dataset_ids", "Knowledge dataset IDs", "Optional full replacement list of knowledge dataset IDs. Use [] to clear knowledge bindings.", false),
			objectParameter("knowledge_retrieval_config", "Knowledge retrieval config", "Optional replacement knowledge retrieval config object. Omit to preserve it.", false),
			stringParameter("database_bindings", "Database bindings", "Optional JSON array replacing database bindings. Each item supports data_source_id, table_ids, and optional writable_table_ids. Use [] to clear.", false),
			stringParameter("workflow_bindings", "Workflow bindings", "Optional JSON array replacing workflow bindings. Each item supports binding_id, label, agent_id, workflow_id, version_strategy, optional version_uuid, and timeout_seconds. Use [] to clear.", false),
		},
	), agentsService, nil, nil)}
}

func newReplaceAgentMemorySlotsTool(agentsService interfaces.AgentsService) tools.Tool {
	return &replaceAgentMemorySlotsTool{agentToolBase: newAgentToolBase(agentToolEntity(
		ToolReplaceAgentMemorySlots,
		"Replace Agent Memory Slots",
		"Replace the complete draft Agent memory slot list for one resolved AGENT asset. Use together with update_agent_config when enabling or disabling Agent memory.",
		"database",
		[]tools.ToolParameter{
			stringParameter("agent_id", "Agent ID", "Required Agent ID from page context, list_agents, or governed asset resolution. Do not invent IDs.", true),
			stringParameter("agent_memory_slots", "Agent memory slots", "Required JSON array replacing all memory slots. Each item supports key, description, enabled, and optional sort_order. Use [] to clear slots.", true),
		},
	), agentsService, nil, nil)}
}

func newListAgentSkillCandidatesTool(agentsService interfaces.AgentsService) tools.Tool {
	return &listAgentSkillCandidatesTool{agentToolBase: newAgentToolBase(agentToolEntity(
		ToolListAgentSkillCandidates,
		"List Agent Skill Candidates",
		"List user-selectable skills that can be enabled for one resolved AGENT asset. The backend filters hidden system skills and skills that do not support the Agent caller.",
		"sparkles",
		[]tools.ToolParameter{
			stringParameter("agent_id", "Agent ID", "Required Agent ID from page context, list_agents, or governed asset resolution. Do not invent IDs.", true),
			stringParameter("query", "Query", "Optional search query for skill ID, name, description, or usage hint.", false),
			numberParameter("limit", "Limit", "Optional maximum number of candidates to return, capped at 100.", false),
			boolParameter("include_selected", "Include selected", "Optional. Defaults to true. Set false to exclude skills already enabled for the Agent.", false),
		},
	), agentsService, nil, nil)}
}

func newReplaceAgentSkillBindingsTool(agentsService interfaces.AgentsService) tools.Tool {
	return &replaceAgentSkillBindingsTool{agentToolBase: newAgentToolBase(agentToolEntity(
		ToolReplaceAgentSkillBindings,
		"Replace Agent Skill Bindings",
		"Replace the complete user-selectable Agent skill list for one resolved AGENT asset. Use candidate data already present in page context or prior tool results when exact; otherwise call list_agent_skill_candidates first. Prefer update_agent_config for multi-section config edits, and preserve existing skill IDs unless the user asked to remove them.",
		"sparkles",
		[]tools.ToolParameter{
			stringParameter("agent_id", "Agent ID", "Required Agent ID from page context, list_agents, or governed asset resolution. Do not invent IDs.", true),
			stringArrayParameter("skill_ids", "Skill IDs", "Required full list of enabled user-selectable skill IDs. Use [] to clear all user-selectable skills.", true),
		},
	), agentsService, nil, nil)}
}

func newListAgentKnowledgeCandidatesTool(agentsService interfaces.AgentsService) tools.Tool {
	return &listAgentKnowledgeCandidatesTool{agentToolBase: newAgentToolBase(agentToolEntity(
		ToolListAgentKnowledgeCandidates,
		"List Agent Knowledge Candidates",
		"List knowledge bases that can be bound to the resolved Agent. The backend uses the Agent's workspace, not any model-provided workspace.",
		"library",
		[]tools.ToolParameter{
			stringParameter("agent_id", "Agent ID", "Required Agent ID from page context, list_agents, or governed asset resolution. Do not invent IDs.", true),
			stringParameter("query", "Query", "Optional search query for knowledge base name or description.", false),
			numberParameter("limit", "Limit", "Optional maximum number of candidates to return, capped at 100.", false),
			boolParameter("include_selected", "Include selected", "Optional. Defaults to true. Set false to exclude knowledge bases already bound to the Agent.", false),
		},
	), agentsService, nil, nil)}
}

func newListAgentDatabaseCandidatesTool(agentsService interfaces.AgentsService) tools.Tool {
	return &listAgentDatabaseCandidatesTool{agentToolBase: newAgentToolBase(agentToolEntity(
		ToolListAgentDatabaseCandidates,
		"List Agent Database Candidates",
		"List databases that can be bound to the resolved Agent. The backend uses the Agent's workspace, not any model-provided workspace.",
		"database",
		[]tools.ToolParameter{
			stringParameter("agent_id", "Agent ID", "Required Agent ID from page context, list_agents, or governed asset resolution. Do not invent IDs.", true),
			stringParameter("query", "Query", "Optional search query for database name or description.", false),
			numberParameter("limit", "Limit", "Optional maximum number of candidates to return, capped at 100.", false),
			boolParameter("include_selected", "Include selected", "Optional. Defaults to true. Set false to exclude databases already bound to the Agent.", false),
			boolParameter("require_write", "Require write", "Optional. Set true to list only databases whose tables can be bound as writable by the current user.", false),
		},
	), agentsService, nil, nil)}
}

func newListAgentDatabaseTablesTool(agentsService interfaces.AgentsService) tools.Tool {
	return &listAgentDatabaseTablesTool{agentToolBase: newAgentToolBase(agentToolEntity(
		ToolListAgentDatabaseTables,
		"List Agent Database Tables",
		"List tables from one database candidate that can be bound to the resolved Agent. The database must belong to the Agent's workspace.",
		"table",
		[]tools.ToolParameter{
			stringParameter("agent_id", "Agent ID", "Required Agent ID from page context, list_agents, or governed asset resolution. Do not invent IDs.", true),
			stringParameter("data_source_id", "Data source ID", "Required database/data source ID returned by list_agent_database_candidates.", true),
			stringParameter("query", "Query", "Optional search query for table name or description.", false),
			numberParameter("limit", "Limit", "Optional maximum number of tables to return, capped at 100.", false),
			boolParameter("include_columns", "Include columns", "Optional. Defaults to false. Set true when table column details are needed before binding.", false),
			boolParameter("include_selected", "Include selected", "Optional. Defaults to true. Set false to exclude tables already bound to the Agent for this database.", false),
		},
	), agentsService, nil, nil)}
}

func newListAgentWorkflowBindingCandidatesTool(agentsService interfaces.AgentsService) tools.Tool {
	return &listAgentWorkflowBindingCandidatesTool{agentToolBase: newAgentToolBase(agentToolEntity(
		ToolListAgentWorkflowBindingCandidates,
		"List Agent Workflow Binding Candidates",
		"List published workflow Agents that can be bound to the resolved Agent. The backend uses the Agent's workspace, not any model-provided workspace.",
		"workflow",
		[]tools.ToolParameter{
			stringParameter("agent_id", "Agent ID", "Required Agent ID from page context, list_agents, or governed asset resolution. Do not invent IDs.", true),
			stringParameter("query", "Query", "Optional search query for workflow name, description, or ID.", false),
			stringParameter("agent_type", "Agent type", "Optional workflow Agent type filter: WORKFLOW or CONVERSATIONAL_WORKFLOW.", false),
			numberParameter("limit", "Limit", "Optional maximum number of candidates to return, capped at 100.", false),
			boolParameter("include_start_inputs", "Include start inputs", "Optional. Defaults to true. Set false to omit workflow start input schemas.", false),
			boolParameter("include_selected", "Include selected", "Optional. Defaults to true. Set false to exclude workflows already bound to the Agent.", false),
		},
	), agentsService, nil, nil)}
}

func newReplaceAgentKnowledgeBindingsTool(agentsService interfaces.AgentsService) tools.Tool {
	return &replaceAgentKnowledgeBindingsTool{agentToolBase: newAgentToolBase(agentToolEntity(
		ToolReplaceAgentKnowledgeBindings,
		"Replace Agent Knowledge Bindings",
		"Replace the Agent's complete knowledge dataset binding list while preserving all other draft config fields. Use exact candidate data already present in page context or prior tool results when available; otherwise call list_agent_knowledge_candidates first. Prefer update_agent_config for multi-section config edits.",
		"library",
		[]tools.ToolParameter{
			stringParameter("agent_id", "Agent ID", "Required Agent ID from page context, list_agents, or governed asset resolution. Do not invent IDs.", true),
			stringArrayParameter("dataset_ids", "Dataset IDs", "Required full replacement list of knowledge dataset IDs. Use [] to clear knowledge bindings.", true),
			objectParameter("retrieval_config", "Retrieval config", "Optional JSON object replacing knowledge retrieval config. Omit to preserve current retrieval config.", false),
		},
	), agentsService, nil, nil)}
}

func newReplaceAgentDatabaseBindingsTool(agentsService interfaces.AgentsService) tools.Tool {
	return &replaceAgentDatabaseBindingsTool{agentToolBase: newAgentToolBase(agentToolEntity(
		ToolReplaceAgentDatabaseBindings,
		"Replace Agent Database Bindings",
		"Replace the Agent's complete database binding list while preserving all other draft config fields. Use exact candidate/table data already present in page context or prior tool results when available; otherwise call database candidate tools first. Prefer update_agent_config for multi-section config edits.",
		"database",
		[]tools.ToolParameter{
			stringParameter("agent_id", "Agent ID", "Required Agent ID from page context, list_agents, or governed asset resolution. Do not invent IDs.", true),
			stringParameter("bindings", "Bindings", "Required JSON array replacing database bindings. Each item supports data_source_id, table_ids, and optional writable_table_ids. Use [] to clear.", true),
		},
	), agentsService, nil, nil)}
}

func newReplaceAgentWorkflowBindingsTool(agentsService interfaces.AgentsService) tools.Tool {
	return &replaceAgentWorkflowBindingsTool{agentToolBase: newAgentToolBase(agentToolEntity(
		ToolReplaceAgentWorkflowBindings,
		"Replace Agent Workflow Bindings",
		"Replace the Agent's complete workflow binding list while preserving all other draft config fields. Use exact candidate data already present in page context or prior tool results when available; otherwise call list_agent_workflow_binding_candidates first. Prefer update_agent_config for multi-section config edits.",
		"workflow",
		[]tools.ToolParameter{
			stringParameter("agent_id", "Agent ID", "Required Agent ID from page context, list_agents, or governed asset resolution. Do not invent IDs.", true),
			stringParameter("bindings", "Bindings", "Required JSON array replacing workflow bindings. Each item supports binding_id, label, agent_id, workflow_id, version_strategy, optional version_uuid, and timeout_seconds. Use [] to clear.", true),
		},
	), agentsService, nil, nil)}
}

func newListAvailableModelsTool(availableModels AvailableModelsService) tools.Tool {
	return &listAvailableModelsTool{agentToolBase: newAgentToolBase(agentToolEntity(
		ToolListAvailableModels,
		"List Available Models",
		"List models available to the current user organization for Agent configuration. Use this before changing an Agent model, and pass a use_case such as text-chat, reasoning, vision, or function-calling when relevant.",
		"brain-circuit",
		[]tools.ToolParameter{
			stringParameter("use_case", "Use case", "Optional use_case filter. Defaults to text-chat for Agent runtime model replacement. Valid values include text-chat, vision, image-gen, embedding, rerank, speech-to-text, text-to-speech, realtime-audio, video-gen, moderation, reasoning, and function-calling.", false),
			stringParameter("provider", "Provider", "Optional provider slug filter, for example openai, deepseek, or anthropic.", false),
			numberParameter("limit", "Limit", "Optional maximum number of models to return, capped at 100.", false),
		},
	), nil, nil, availableModels)}
}

func newAgentToolBase(entity tools.ToolEntity, agentsService interfaces.AgentsService, workspacePerms WorkspacePermissionService, availableModels AvailableModelsService) agentToolBase {
	return agentToolBase{
		BuiltinTool:     builtin.NewBuiltinTool(entity, ""),
		agentsService:   agentsService,
		workspacePerms:  workspacePerms,
		availableModels: availableModels,
	}
}

func agentToolEntity(name string, label string, llm string, icon string, parameters []tools.ToolParameter) tools.ToolEntity {
	return tools.ToolEntity{
		Identity: tools.ToolIdentity{
			Name:     name,
			Author:   "System",
			Provider: ProviderID,
			Label: tools.I18nText{
				"en_US":   label,
				"zh_Hans": label,
			},
			Icon: icon,
		},
		Description: tools.ToolDescription{
			Human: tools.I18nText{
				"en_US":   label,
				"zh_Hans": label,
			},
			LLM: llm,
		},
		Parameters: parameters,
		OutputType: "json",
		Tags:       []string{"agent", "management", "system"},
	}
}

func stringParameter(name string, label string, description string, required bool) tools.ToolParameter {
	return tools.ToolParameter{
		Name:            name,
		Label:           tools.I18nText{"en_US": label, "zh_Hans": label},
		LLMDescription:  description,
		Type:            tools.ToolParameterTypeString,
		Form:            tools.ToolParameterFormLLM,
		Required:        required,
		SupportVariable: true,
	}
}

func numberParameter(name string, label string, description string, required bool) tools.ToolParameter {
	return tools.ToolParameter{
		Name:            name,
		Label:           tools.I18nText{"en_US": label, "zh_Hans": label},
		LLMDescription:  description,
		Type:            tools.ToolParameterTypeNumber,
		Form:            tools.ToolParameterFormLLM,
		Required:        required,
		SupportVariable: true,
	}
}

func boolParameter(name string, label string, description string, required bool) tools.ToolParameter {
	return tools.ToolParameter{
		Name:            name,
		Label:           tools.I18nText{"en_US": label, "zh_Hans": label},
		LLMDescription:  description,
		Type:            tools.ToolParameterTypeBoolean,
		Form:            tools.ToolParameterFormLLM,
		Required:        required,
		SupportVariable: true,
	}
}

func objectParameter(name string, label string, description string, required bool) tools.ToolParameter {
	return tools.ToolParameter{
		Name:            name,
		Label:           tools.I18nText{"en_US": label, "zh_Hans": label},
		LLMDescription:  description,
		Type:            tools.ToolParameterTypeString,
		Form:            tools.ToolParameterFormLLM,
		Required:        required,
		SupportVariable: true,
	}
}

func stringArrayParameter(name string, label string, description string, required bool) tools.ToolParameter {
	return tools.ToolParameter{
		Name:            name,
		Label:           tools.I18nText{"en_US": label, "zh_Hans": label},
		LLMDescription:  description,
		Type:            tools.ToolParameterTypeString,
		Form:            tools.ToolParameterFormLLM,
		Required:        required,
		SupportVariable: true,
	}
}

func (t *listAgentsTool) Invoke(ctx context.Context, userID string, params map[string]interface{}, conversationID *string, appID *string, messageID *string) ([]tools.ToolInvokeMessage, error) {
	_ = conversationID
	_ = appID
	_ = messageID
	if t.agentsService == nil {
		return nil, fmt.Errorf("agent service is not configured")
	}
	scope, err := t.scope(userID)
	if err != nil {
		return nil, err
	}
	workspaceID := firstNonEmptyString(stringValue(params, "workspace_id"), scope.WorkspaceID)
	limit := intParam(params, "limit", defaultAgentListPageSize, 100)
	req := dto.GetAgentsListRequest{
		Page:        1,
		Limit:       limit,
		PageSize:    limit,
		WorkspaceID: workspaceID,
		Keyword:     stringValue(params, "keyword"),
		AgentType:   "AGENT",
	}
	resp, err := t.agentsService.GetAgentsListWithPermissions(t.scopedContext(ctx, scope), scope.AccountID, req)
	if err != nil {
		return nil, err
	}
	agents := make([]map[string]interface{}, 0, len(resp.Data))
	for _, item := range resp.Data {
		agents = append(agents, agentListItemPayload(item))
	}
	return []tools.ToolInvokeMessage{builtin.CreateJSONMessage(map[string]interface{}{
		"status":       "completed",
		"count":        len(agents),
		"total":        resp.Total,
		"has_more":     resp.HasMore,
		"workspace_id": workspaceID,
		"agents":       agents,
		"visible_page": visibleAgentsFromRuntime(t.Runtime()),
	})}, nil
}

func (t *getAgentTool) Invoke(ctx context.Context, userID string, params map[string]interface{}, conversationID *string, appID *string, messageID *string) ([]tools.ToolInvokeMessage, error) {
	_ = conversationID
	_ = appID
	_ = messageID
	scope, err := t.scope(userID)
	if err != nil {
		return nil, err
	}
	agentID := requiredAgentID(params)
	if agentID == "" {
		return nil, fmt.Errorf("agent_id is required")
	}
	agent, err := t.agentsService.GetAgent(t.scopedContext(ctx, scope), agentID)
	if err != nil {
		return nil, err
	}
	payload := map[string]interface{}{
		"status": "completed",
		"agent":  agentPayload(agent),
	}
	return []tools.ToolInvokeMessage{builtin.CreateJSONMessage(payload)}, nil
}

func (t *createAgentTool) Invoke(ctx context.Context, userID string, params map[string]interface{}, conversationID *string, appID *string, messageID *string) ([]tools.ToolInvokeMessage, error) {
	_ = conversationID
	_ = appID
	_ = messageID
	if t.agentsService == nil {
		return nil, fmt.Errorf("agent service is not configured")
	}
	scope, err := t.scope(userID)
	if err != nil {
		return nil, err
	}
	workspaceID := firstNonEmptyString(stringValue(params, "workspace_id"), scope.WorkspaceID)
	if workspaceID == "" {
		return nil, fmt.Errorf("workspace_id is required to create an Agent")
	}
	if err := t.ensureWorkspaceAgentManage(ctx, scope, workspaceID); err != nil {
		return nil, err
	}
	name := strings.TrimSpace(stringValue(params, "name"))
	if name == "" {
		return nil, fmt.Errorf("name is required")
	}
	rawIcon := stringValue(params, "icon")
	iconBackground := stringValue(params, "icon_background")
	if strings.TrimSpace(rawIcon) == "" && strings.TrimSpace(iconBackground) != "" {
		rawIcon = defaultAgentTextIcon(name)
	}
	iconType, icon := normalizeAgentIconParams(stringValue(params, "icon_type"), rawIcon, iconBackground)
	if iconType == "" && icon == "" {
		iconType, icon = normalizeAgentIconParams("text", defaultAgentTextIcon(name), iconBackground)
	}
	req := dto.CreateAgentRequest{
		Name:        name,
		Description: stringValue(params, "description"),
		IconType:    iconType,
		Icon:        icon,
		AgentType:   "AGENT",
		WorkspaceID: workspaceID,
	}
	agent, err := t.agentsService.CreateAgent(t.scopedContext(ctx, scope), workspaceID, req, scope.AccountID)
	if err != nil {
		return nil, err
	}
	payload := agentOperationPayload("created", agent)
	payload["workspace_id"] = workspaceID
	return []tools.ToolInvokeMessage{builtin.CreateJSONMessage(payload)}, nil
}

func (t *updateAgentIdentityTool) Invoke(ctx context.Context, userID string, params map[string]interface{}, conversationID *string, appID *string, messageID *string) ([]tools.ToolInvokeMessage, error) {
	_ = conversationID
	_ = appID
	_ = messageID
	scope, err := t.scope(userID)
	if err != nil {
		return nil, err
	}
	agentID := requiredAgentID(params)
	if agentID == "" {
		return nil, fmt.Errorf("agent_id is required")
	}
	update := map[string]interface{}{}
	changedFields := []string{}
	for _, key := range []string{"name", "description"} {
		if value, ok := optionalStringParam(params, key); ok {
			update[key] = value
			changedFields = append(changedFields, key)
		}
	}
	iconType, hasIconType := optionalStringParam(params, "icon_type")
	icon, hasIcon := optionalStringParam(params, "icon")
	iconBackground, hasIconBackground := optionalStringParam(params, "icon_background")
	if hasIconBackground && !hasIcon {
		return nil, fmt.Errorf("icon is required when changing icon_background")
	}
	if hasIconType || hasIcon || hasIconBackground {
		normalizedType, normalizedIcon := normalizeAgentIconParams(iconType, icon, iconBackground)
		if hasIconType || normalizedType != "" {
			update["icon_type"] = normalizedType
			changedFields = appendUniqueString(changedFields, "icon_type")
		}
		if hasIcon || normalizedIcon != "" {
			update["icon"] = normalizedIcon
			changedFields = appendUniqueString(changedFields, "icon")
		}
		if hasIconBackground {
			changedFields = appendUniqueString(changedFields, "icon_background")
		}
	}
	if len(update) == 0 {
		return nil, fmt.Errorf("at least one identity field is required")
	}
	agent, err := t.agentsService.UpdateAgent(t.scopedContext(ctx, scope), agentID, update)
	if err != nil {
		return nil, err
	}
	payload := agentOperationPayload("updated", agent)
	payload["updated_fields"] = append([]string(nil), changedFields...)
	return []tools.ToolInvokeMessage{builtin.CreateJSONMessage(payload)}, nil
}

func (t *deleteAgentTool) Invoke(ctx context.Context, userID string, params map[string]interface{}, conversationID *string, appID *string, messageID *string) ([]tools.ToolInvokeMessage, error) {
	_ = conversationID
	_ = appID
	_ = messageID
	scope, err := t.scope(userID)
	if err != nil {
		return nil, err
	}
	agentID := requiredAgentID(params)
	if agentID == "" {
		return nil, fmt.Errorf("agent_id is required")
	}
	agent, _ := t.agentsService.GetAgent(t.scopedContext(ctx, scope), agentID)
	if err := t.agentsService.DeleteAgent(t.scopedContext(ctx, scope), agentID); err != nil {
		return nil, err
	}
	payload := agentOperationPayload("deleted", agent)
	payload["agent_id"] = agentID
	payload["href"] = "/console/agents"
	payload["route_after_delete"] = "/console/agents"
	payload["reversible"] = false
	return []tools.ToolInvokeMessage{builtin.CreateJSONMessage(payload)}, nil
}

func (t *deleteAgentsTool) Invoke(ctx context.Context, userID string, params map[string]interface{}, conversationID *string, appID *string, messageID *string) ([]tools.ToolInvokeMessage, error) {
	_ = conversationID
	_ = appID
	_ = messageID
	if t.agentsService == nil {
		return nil, fmt.Errorf("agent service is not configured")
	}
	scope, err := t.scope(userID)
	if err != nil {
		return nil, err
	}
	targets, err := agentBatchDeleteTargets(params, t.Runtime())
	if err != nil {
		return nil, err
	}
	if len(targets) == 0 {
		return nil, fmt.Errorf("at least one agent target is required")
	}

	itemResults := make([]map[string]interface{}, 0, len(targets))
	deletedCount := 0
	failedCount := 0
	for index, target := range targets {
		item := map[string]interface{}{
			"index":    index,
			"agent_id": target.ID,
			"status":   "pending",
		}
		if target.Name != "" {
			item["agent_name"] = target.Name
		}
		if target.WorkspaceID != "" {
			item["workspace_id"] = target.WorkspaceID
		}
		agent, _ := t.agentsService.GetAgent(t.scopedContext(ctx, scope), target.ID)
		if agentMap := agentPayload(agent); len(agentMap) > 0 {
			item["agent"] = agentMap
			if name := strings.TrimSpace(stringValue(agentMap, "name")); name != "" {
				item["agent_name"] = name
			}
			if workspaceID := strings.TrimSpace(firstNonEmptyString(stringValue(agentMap, "workspace_id"), stringValue(agentMap, "tenant_id"))); workspaceID != "" {
				item["workspace_id"] = workspaceID
			}
		}
		if err := t.agentsService.DeleteAgent(t.scopedContext(ctx, scope), target.ID); err != nil {
			item["status"] = "failed"
			item["error"] = err.Error()
			failedCount++
			itemResults = append(itemResults, item)
			continue
		}
		item["status"] = "succeeded"
		item["effect"] = "deleted"
		item["href"] = "/console/agents"
		deletedCount++
		itemResults = append(itemResults, item)
	}

	status := "completed"
	switch {
	case failedCount == len(targets):
		status = "failed"
	case failedCount > 0:
		status = "partial_failed"
	}
	groupID := "agent.delete.batch:" + uuid.NewString()
	targetPayload := agentBatchDeleteTargetsPayload(targets)
	payload := map[string]interface{}{
		"status":             status,
		"effect":             "deleted",
		"operation_type":     "agent.delete.batch",
		"operation_group_id": groupID,
		"target_count":       len(targets),
		"deleted_count":      deletedCount,
		"failed_count":       failedCount,
		"item_results":       itemResults,
		"assets":             targetPayload,
		"requires_refresh":   true,
		"refresh_target":     "/console/agents",
		"reversible":         false,
		"operation_group": map[string]interface{}{
			"id":            groupID,
			"type":          "batch",
			"operation":     "agent.delete",
			"asset_type":    "agent",
			"status":        status,
			"target_count":  len(targets),
			"success_count": deletedCount,
			"failed_count":  failedCount,
			"targets":       targetPayload,
			"item_results":  itemResults,
		},
	}
	if scope.WorkspaceID != "" {
		payload["workspace_id"] = scope.WorkspaceID
	}
	return []tools.ToolInvokeMessage{builtin.CreateJSONMessage(payload)}, nil
}

func (t *getAgentConfigTool) Invoke(ctx context.Context, userID string, params map[string]interface{}, conversationID *string, appID *string, messageID *string) ([]tools.ToolInvokeMessage, error) {
	_ = conversationID
	_ = appID
	_ = messageID
	scope, err := t.scope(userID)
	if err != nil {
		return nil, err
	}
	agentID := requiredAgentID(params)
	if agentID == "" {
		return nil, fmt.Errorf("agent_id is required")
	}
	cfg, err := t.agentsService.GetAgentDraftRuntimeConfig(t.scopedContext(ctx, scope), agentID, scope.AccountID)
	if err != nil {
		return nil, err
	}
	return []tools.ToolInvokeMessage{builtin.CreateJSONMessage(map[string]interface{}{
		"status":       "completed",
		"agent_id":     cfg.AgentID,
		"workspace_id": cfg.WorkspaceID,
		"config":       cfg.Config,
	})}, nil
}

func (t *updateAgentConfigTool) Invoke(ctx context.Context, userID string, params map[string]interface{}, conversationID *string, appID *string, messageID *string) ([]tools.ToolInvokeMessage, error) {
	_ = conversationID
	_ = appID
	_ = messageID
	scope, err := t.scope(userID)
	if err != nil {
		return nil, err
	}
	agentID := requiredAgentID(params)
	if agentID == "" {
		return nil, fmt.Errorf("agent_id is required")
	}
	current, err := t.agentsService.GetAgentDraftRuntimeConfig(t.scopedContext(ctx, scope), agentID, scope.AccountID)
	if err != nil {
		return nil, err
	}
	if _, hasModel := optionalStringParam(params, "model"); hasModel {
		if _, hasProvider := optionalStringParam(params, "model_provider"); !hasProvider {
			return nil, fmt.Errorf("model_provider is required when changing model; call list_available_models and pass the selected model.provider with model.model")
		}
	}
	req, changedFields, err := mergeAgentConfigRequest(current.Config, params)
	if err != nil {
		return nil, err
	}
	if len(changedFields) == 0 {
		return nil, fmt.Errorf("at least one config field is required")
	}
	updated, err := t.agentsService.UpdateAgentConfig(t.scopedContext(ctx, scope), agentID, scope.AccountID, req)
	if err != nil {
		return nil, err
	}
	agent := t.agentPayloadForResult(ctx, scope, agentID)
	payload := agentConfigMutationPayload("updated", agentID, current.WorkspaceID, &current.Config, updated, agent)
	payload["updated_fields"] = append([]string(nil), changedFields...)
	return []tools.ToolInvokeMessage{builtin.CreateJSONMessage(payload)}, nil
}

func (t *replaceAgentMemorySlotsTool) Invoke(ctx context.Context, userID string, params map[string]interface{}, conversationID *string, appID *string, messageID *string) ([]tools.ToolInvokeMessage, error) {
	_ = conversationID
	_ = appID
	_ = messageID
	scope, err := t.scope(userID)
	if err != nil {
		return nil, err
	}
	agentID := requiredAgentID(params)
	if agentID == "" {
		return nil, fmt.Errorf("agent_id is required")
	}
	slots, ok, err := agentMemorySlotsParam(params, "agent_memory_slots", "slots")
	if err != nil {
		return nil, err
	}
	if !ok {
		return nil, fmt.Errorf("agent_memory_slots is required")
	}
	updated, err := t.agentsService.ReplaceAgentMemorySlots(t.scopedContext(ctx, scope), agentID, scope.AccountID, slots)
	if err != nil {
		return nil, err
	}
	payload := map[string]interface{}{
		"status":             "completed",
		"effect":             "updated",
		"agent_id":           agentID,
		"href":               "/console/agents/" + agentID + "/agent",
		"agent_memory_slots": updated,
		"config": map[string]interface{}{
			"agent_memory_slots": updated,
		},
		"agent": map[string]interface{}{
			"id":   agentID,
			"href": "/console/agents/" + agentID + "/agent",
		},
	}
	if scope.WorkspaceID != "" {
		payload["workspace_id"] = scope.WorkspaceID
	}
	return []tools.ToolInvokeMessage{builtin.CreateJSONMessage(payload)}, nil
}

func (t *listAgentSkillCandidatesTool) Invoke(ctx context.Context, userID string, params map[string]interface{}, conversationID *string, appID *string, messageID *string) ([]tools.ToolInvokeMessage, error) {
	_ = conversationID
	_ = appID
	_ = messageID
	if t.agentsService == nil {
		return nil, fmt.Errorf("agent service is not configured")
	}
	scope, err := t.scope(userID)
	if err != nil {
		return nil, err
	}
	agentID := requiredAgentID(params)
	if agentID == "" {
		return nil, fmt.Errorf("agent_id is required")
	}
	req := dto.AgentSkillCandidatesRequest{
		Query:           stringValue(params, "query"),
		Limit:           intParam(params, "limit", defaultAgentBindingCandidateListPageSize, maxAgentBindingCandidateListPageSize),
		IncludeSelected: boolParamDefault(params, "include_selected", true),
	}
	resp, err := t.agentsService.ListAgentSkillCandidates(t.scopedContext(ctx, scope), agentID, scope.AccountID, req)
	if err != nil {
		return nil, err
	}
	return []tools.ToolInvokeMessage{builtin.CreateJSONMessage(map[string]interface{}{
		"status":           "completed",
		"agent_id":         resp.AgentID,
		"workspace_id":     resp.WorkspaceID,
		"query":            resp.Query,
		"count":            resp.Count,
		"include_selected": resp.IncludeSelected,
		"skills":           resp.Data,
	})}, nil
}

func (t *listAgentKnowledgeCandidatesTool) Invoke(ctx context.Context, userID string, params map[string]interface{}, conversationID *string, appID *string, messageID *string) ([]tools.ToolInvokeMessage, error) {
	_ = conversationID
	_ = appID
	_ = messageID
	if t.agentsService == nil {
		return nil, fmt.Errorf("agent service is not configured")
	}
	scope, err := t.scope(userID)
	if err != nil {
		return nil, err
	}
	agentID := requiredAgentID(params)
	if agentID == "" {
		return nil, fmt.Errorf("agent_id is required")
	}
	req := dto.AgentKnowledgeCandidatesRequest{
		Query:           stringValue(params, "query"),
		Limit:           intParam(params, "limit", defaultAgentBindingCandidateListPageSize, maxAgentBindingCandidateListPageSize),
		IncludeSelected: boolParamDefault(params, "include_selected", true),
	}
	resp, err := t.agentsService.ListAgentKnowledgeCandidates(t.scopedContext(ctx, scope), agentID, scope.AccountID, req)
	if err != nil {
		return nil, err
	}
	return []tools.ToolInvokeMessage{builtin.CreateJSONMessage(map[string]interface{}{
		"status":           "completed",
		"agent_id":         resp.AgentID,
		"workspace_id":     resp.WorkspaceID,
		"query":            resp.Query,
		"count":            resp.Count,
		"include_selected": resp.IncludeSelected,
		"warnings":         resp.Warnings,
		"knowledge_bases":  resp.Data,
	})}, nil
}

func (t *listAgentDatabaseCandidatesTool) Invoke(ctx context.Context, userID string, params map[string]interface{}, conversationID *string, appID *string, messageID *string) ([]tools.ToolInvokeMessage, error) {
	_ = conversationID
	_ = appID
	_ = messageID
	if t.agentsService == nil {
		return nil, fmt.Errorf("agent service is not configured")
	}
	scope, err := t.scope(userID)
	if err != nil {
		return nil, err
	}
	agentID := requiredAgentID(params)
	if agentID == "" {
		return nil, fmt.Errorf("agent_id is required")
	}
	req := dto.AgentDatabaseCandidatesRequest{
		Query:           stringValue(params, "query"),
		Limit:           intParam(params, "limit", defaultAgentBindingCandidateListPageSize, maxAgentBindingCandidateListPageSize),
		IncludeSelected: boolParamDefault(params, "include_selected", true),
		RequireWrite:    boolParamDefault(params, "require_write", false),
	}
	resp, err := t.agentsService.ListAgentDatabaseCandidates(t.scopedContext(ctx, scope), agentID, scope.AccountID, req)
	if err != nil {
		return nil, err
	}
	return []tools.ToolInvokeMessage{builtin.CreateJSONMessage(map[string]interface{}{
		"status":           "completed",
		"agent_id":         resp.AgentID,
		"workspace_id":     resp.WorkspaceID,
		"query":            resp.Query,
		"count":            resp.Count,
		"include_selected": resp.IncludeSelected,
		"require_write":    resp.RequireWrite,
		"warnings":         resp.Warnings,
		"databases":        resp.Data,
	})}, nil
}

func (t *listAgentDatabaseTablesTool) Invoke(ctx context.Context, userID string, params map[string]interface{}, conversationID *string, appID *string, messageID *string) ([]tools.ToolInvokeMessage, error) {
	_ = conversationID
	_ = appID
	_ = messageID
	if t.agentsService == nil {
		return nil, fmt.Errorf("agent service is not configured")
	}
	scope, err := t.scope(userID)
	if err != nil {
		return nil, err
	}
	agentID := requiredAgentID(params)
	if agentID == "" {
		return nil, fmt.Errorf("agent_id is required")
	}
	dataSourceID := stringValue(params, "data_source_id")
	if dataSourceID == "" {
		return nil, fmt.Errorf("data_source_id is required")
	}
	req := dto.AgentDatabaseTablesRequest{
		DataSourceID:    dataSourceID,
		Query:           stringValue(params, "query"),
		Limit:           intParam(params, "limit", defaultAgentBindingCandidateListPageSize, maxAgentBindingCandidateListPageSize),
		IncludeColumns:  boolParamDefault(params, "include_columns", false),
		IncludeSelected: boolParamDefault(params, "include_selected", true),
	}
	resp, err := t.agentsService.ListAgentDatabaseTables(t.scopedContext(ctx, scope), agentID, scope.AccountID, req)
	if err != nil {
		return nil, err
	}
	return []tools.ToolInvokeMessage{builtin.CreateJSONMessage(map[string]interface{}{
		"status":           "completed",
		"agent_id":         resp.AgentID,
		"workspace_id":     resp.WorkspaceID,
		"data_source_id":   resp.DataSourceID,
		"query":            resp.Query,
		"count":            resp.Count,
		"include_columns":  resp.IncludeColumns,
		"include_selected": resp.IncludeSelected,
		"tables":           resp.Data,
	})}, nil
}

func (t *listAgentWorkflowBindingCandidatesTool) Invoke(ctx context.Context, userID string, params map[string]interface{}, conversationID *string, appID *string, messageID *string) ([]tools.ToolInvokeMessage, error) {
	_ = conversationID
	_ = appID
	_ = messageID
	if t.agentsService == nil {
		return nil, fmt.Errorf("agent service is not configured")
	}
	scope, err := t.scope(userID)
	if err != nil {
		return nil, err
	}
	agentID := requiredAgentID(params)
	if agentID == "" {
		return nil, fmt.Errorf("agent_id is required")
	}
	req := dto.AgentWorkflowBindingCandidatesRequest{
		Query:              stringValue(params, "query"),
		AgentType:          stringValue(params, "agent_type"),
		Limit:              intParam(params, "limit", defaultAgentBindingCandidateListPageSize, maxAgentBindingCandidateListPageSize),
		IncludeStartInputs: boolParamDefault(params, "include_start_inputs", true),
		IncludeSelected:    boolParamDefault(params, "include_selected", true),
	}
	resp, err := t.agentsService.ListAgentWorkflowBindingCandidates(t.scopedContext(ctx, scope), agentID, scope.AccountID, req)
	if err != nil {
		return nil, err
	}
	return []tools.ToolInvokeMessage{builtin.CreateJSONMessage(map[string]interface{}{
		"status":               "completed",
		"agent_id":             resp.AgentID,
		"workspace_id":         resp.WorkspaceID,
		"query":                resp.Query,
		"agent_type":           resp.AgentType,
		"count":                resp.Count,
		"include_start_inputs": resp.IncludeStartInputs,
		"include_selected":     resp.IncludeSelected,
		"workflows":            resp.Data,
	})}, nil
}

func (t *replaceAgentSkillBindingsTool) Invoke(ctx context.Context, userID string, params map[string]interface{}, conversationID *string, appID *string, messageID *string) ([]tools.ToolInvokeMessage, error) {
	_ = conversationID
	_ = appID
	_ = messageID
	if t.agentsService == nil {
		return nil, fmt.Errorf("agent service is not configured")
	}
	scope, err := t.scope(userID)
	if err != nil {
		return nil, err
	}
	agentID := requiredAgentID(params)
	if agentID == "" {
		return nil, fmt.Errorf("agent_id is required")
	}
	skillIDs, ok := stringSliceParam(params, "skill_ids")
	if !ok {
		return nil, fmt.Errorf("skill_ids is required")
	}
	current, err := t.agentsService.GetAgentDraftRuntimeConfig(t.scopedContext(ctx, scope), agentID, scope.AccountID)
	if err != nil {
		return nil, err
	}
	req := agentConfigRequestFromResponse(current.Config)
	req.EnabledSkillIDs = skillIDs
	updated, err := t.agentsService.UpdateAgentConfig(t.scopedContext(ctx, scope), agentID, scope.AccountID, req)
	if err != nil {
		return nil, err
	}
	agent := t.agentPayloadForResult(ctx, scope, agentID)
	return []tools.ToolInvokeMessage{builtin.CreateJSONMessage(agentConfigMutationPayload("updated", agentID, current.WorkspaceID, &current.Config, updated, agent))}, nil
}

func (t *replaceAgentKnowledgeBindingsTool) Invoke(ctx context.Context, userID string, params map[string]interface{}, conversationID *string, appID *string, messageID *string) ([]tools.ToolInvokeMessage, error) {
	_ = conversationID
	_ = appID
	_ = messageID
	if t.agentsService == nil {
		return nil, fmt.Errorf("agent service is not configured")
	}
	scope, err := t.scope(userID)
	if err != nil {
		return nil, err
	}
	agentID := requiredAgentID(params)
	if agentID == "" {
		return nil, fmt.Errorf("agent_id is required")
	}
	datasetIDs, ok := stringSliceParam(params, "dataset_ids")
	if !ok {
		return nil, fmt.Errorf("dataset_ids is required")
	}
	current, err := t.agentsService.GetAgentDraftRuntimeConfig(t.scopedContext(ctx, scope), agentID, scope.AccountID)
	if err != nil {
		return nil, err
	}
	req := agentConfigRequestFromResponse(current.Config)
	req.KnowledgeDatasetIDs = datasetIDs
	if retrievalConfig, ok, err := optionalMapParam(params, "retrieval_config"); err != nil {
		return nil, err
	} else if ok {
		req.KnowledgeRetrievalConfig = retrievalConfig
	}
	updated, err := t.agentsService.UpdateAgentConfig(t.scopedContext(ctx, scope), agentID, scope.AccountID, req)
	if err != nil {
		return nil, err
	}
	agent := t.agentPayloadForResult(ctx, scope, agentID)
	return []tools.ToolInvokeMessage{builtin.CreateJSONMessage(agentConfigMutationPayload("updated", agentID, current.WorkspaceID, &current.Config, updated, agent))}, nil
}

func (t *replaceAgentDatabaseBindingsTool) Invoke(ctx context.Context, userID string, params map[string]interface{}, conversationID *string, appID *string, messageID *string) ([]tools.ToolInvokeMessage, error) {
	_ = conversationID
	_ = appID
	_ = messageID
	if t.agentsService == nil {
		return nil, fmt.Errorf("agent service is not configured")
	}
	scope, err := t.scope(userID)
	if err != nil {
		return nil, err
	}
	agentID := requiredAgentID(params)
	if agentID == "" {
		return nil, fmt.Errorf("agent_id is required")
	}
	bindings, ok, err := agentDatabaseBindingsParam(params, "bindings", "database_bindings")
	if err != nil {
		return nil, err
	}
	if !ok {
		return nil, fmt.Errorf("bindings is required")
	}
	current, err := t.agentsService.GetAgentDraftRuntimeConfig(t.scopedContext(ctx, scope), agentID, scope.AccountID)
	if err != nil {
		return nil, err
	}
	req := agentConfigRequestFromResponse(current.Config)
	req.DatabaseBindings = bindings
	updated, err := t.agentsService.UpdateAgentConfig(t.scopedContext(ctx, scope), agentID, scope.AccountID, req)
	if err != nil {
		return nil, err
	}
	agent := t.agentPayloadForResult(ctx, scope, agentID)
	return []tools.ToolInvokeMessage{builtin.CreateJSONMessage(agentConfigMutationPayload("updated", agentID, current.WorkspaceID, &current.Config, updated, agent))}, nil
}

func (t *replaceAgentWorkflowBindingsTool) Invoke(ctx context.Context, userID string, params map[string]interface{}, conversationID *string, appID *string, messageID *string) ([]tools.ToolInvokeMessage, error) {
	_ = conversationID
	_ = appID
	_ = messageID
	if t.agentsService == nil {
		return nil, fmt.Errorf("agent service is not configured")
	}
	scope, err := t.scope(userID)
	if err != nil {
		return nil, err
	}
	agentID := requiredAgentID(params)
	if agentID == "" {
		return nil, fmt.Errorf("agent_id is required")
	}
	bindings, ok, err := agentWorkflowBindingsParam(params, "bindings", "workflow_bindings")
	if err != nil {
		return nil, err
	}
	if !ok {
		return nil, fmt.Errorf("bindings is required")
	}
	current, err := t.agentsService.GetAgentDraftRuntimeConfig(t.scopedContext(ctx, scope), agentID, scope.AccountID)
	if err != nil {
		return nil, err
	}
	req := agentConfigRequestFromResponse(current.Config)
	req.WorkflowBindings = bindings
	updated, err := t.agentsService.UpdateAgentConfig(t.scopedContext(ctx, scope), agentID, scope.AccountID, req)
	if err != nil {
		return nil, err
	}
	agent := t.agentPayloadForResult(ctx, scope, agentID)
	return []tools.ToolInvokeMessage{builtin.CreateJSONMessage(agentConfigMutationPayload("updated", agentID, current.WorkspaceID, &current.Config, updated, agent))}, nil
}

func (t *listAvailableModelsTool) Invoke(ctx context.Context, userID string, params map[string]interface{}, conversationID *string, appID *string, messageID *string) ([]tools.ToolInvokeMessage, error) {
	_ = conversationID
	_ = appID
	_ = messageID
	if t.availableModels == nil {
		return nil, fmt.Errorf("available models service is not configured")
	}
	scope, err := t.scope(userID)
	if err != nil {
		return nil, err
	}
	organizationID, err := uuid.Parse(scope.OrganizationID)
	if err != nil {
		return nil, fmt.Errorf("invalid organization_id: %w", err)
	}
	useCase := normalizeAgentModelUseCase(firstNonEmptyString(
		stringValue(params, "use_case"),
		stringValue(params, "usecase"),
		stringValue(params, "useCase"),
	))
	if useCase != "" && !isValidAgentModelUseCase(useCase) {
		return nil, fmt.Errorf("invalid use_case %q; valid values are: %s", useCase, strings.Join(validAgentModelUseCases(), ", "))
	}
	provider := stringValue(params, "provider")
	models, err := t.availableModels.ListAvailable(ctx, organizationID, provider, useCase)
	if err != nil {
		return nil, err
	}
	limit := intParam(params, "limit", defaultAgentModelListPageSize, maxAgentModelListPageSize)
	total := len(models)
	if limit <= 0 || limit > maxAgentModelListPageSize {
		limit = defaultAgentModelListPageSize
	}
	truncated := total > limit
	if truncated {
		models = models[:limit]
	}
	items := make([]map[string]interface{}, 0, len(models))
	for _, model := range models {
		items = append(items, availableAgentModelPayload(model))
	}
	payload := map[string]interface{}{
		"status":          "completed",
		"use_case":        useCase,
		"provider":        provider,
		"count":           len(items),
		"total":           total,
		"truncated":       truncated,
		"models":          items,
		"valid_use_cases": validAgentModelUseCases(),
		"usage_hint":      "Use model.provider as update_agent_config model_provider and model.model as update_agent_config model.",
	}
	return []tools.ToolInvokeMessage{builtin.CreateJSONMessage(payload)}, nil
}

func (t agentToolBase) scope(userID string) (agentScope, error) {
	return agentScopeFromRuntime(t.Runtime(), t.GetTenantID(), userID)
}

func (t agentToolBase) scopedContext(ctx context.Context, scope agentScope) context.Context {
	ctx = context.WithValue(ctx, "account_id", scope.AccountID)
	if scope.OrganizationID != "" {
		ctx = context.WithValue(ctx, "tenant_id", scope.OrganizationID)
	}
	return ctx
}

func (t agentToolBase) ensureWorkspaceAgentManage(ctx context.Context, scope agentScope, workspaceID string) error {
	if t.workspacePerms == nil {
		return fmt.Errorf("workspace permission service is not configured")
	}
	allowed, err := t.workspacePerms.CheckWorkspacePermission(ctx, scope.OrganizationID, workspaceID, scope.AccountID, workspacemodel.WorkspacePermissionAgentManage)
	if err != nil {
		return fmt.Errorf("failed to check workspace agent permission: %w", err)
	}
	if !allowed {
		return fmt.Errorf("user does not have permission to manage Agents in this workspace")
	}
	return nil
}

func (t agentToolBase) agentPayloadForResult(ctx context.Context, scope agentScope, agentID string) map[string]interface{} {
	agentID = strings.TrimSpace(agentID)
	payload := map[string]interface{}{}
	if t.agentsService != nil && agentID != "" {
		if agent, err := t.agentsService.GetAgent(t.scopedContext(ctx, scope), agentID); err == nil {
			payload = agentPayload(agent)
		}
	}
	if len(payload) == 0 {
		payload = map[string]interface{}{}
	}
	if agentID != "" {
		payload["id"] = agentID
		payload["agent_id"] = agentID
		if _, ok := payload["href"]; !ok {
			payload["href"] = "/console/agents/" + agentID + "/agent"
		}
	}
	if workspaceID := strings.TrimSpace(scope.WorkspaceID); workspaceID != "" {
		if _, ok := payload["workspace_id"]; !ok {
			payload["workspace_id"] = workspaceID
		}
	}
	return payload
}

func (t *listAgentsTool) ForkToolRuntime(runtime *tools.ToolRuntime) tools.Tool {
	return &listAgentsTool{agentToolBase: t.forkAgentToolBase(runtime)}
}

func (t *getAgentTool) ForkToolRuntime(runtime *tools.ToolRuntime) tools.Tool {
	return &getAgentTool{agentToolBase: t.forkAgentToolBase(runtime)}
}

func (t *createAgentTool) ForkToolRuntime(runtime *tools.ToolRuntime) tools.Tool {
	return &createAgentTool{agentToolBase: t.forkAgentToolBase(runtime)}
}

func (t *updateAgentIdentityTool) ForkToolRuntime(runtime *tools.ToolRuntime) tools.Tool {
	return &updateAgentIdentityTool{agentToolBase: t.forkAgentToolBase(runtime)}
}

func (t *deleteAgentTool) ForkToolRuntime(runtime *tools.ToolRuntime) tools.Tool {
	return &deleteAgentTool{agentToolBase: t.forkAgentToolBase(runtime)}
}

func (t *deleteAgentsTool) ForkToolRuntime(runtime *tools.ToolRuntime) tools.Tool {
	return &deleteAgentsTool{agentToolBase: t.forkAgentToolBase(runtime)}
}

func (t *getAgentConfigTool) ForkToolRuntime(runtime *tools.ToolRuntime) tools.Tool {
	return &getAgentConfigTool{agentToolBase: t.forkAgentToolBase(runtime)}
}

func (t *updateAgentConfigTool) ForkToolRuntime(runtime *tools.ToolRuntime) tools.Tool {
	return &updateAgentConfigTool{agentToolBase: t.forkAgentToolBase(runtime)}
}

func (t *replaceAgentMemorySlotsTool) ForkToolRuntime(runtime *tools.ToolRuntime) tools.Tool {
	return &replaceAgentMemorySlotsTool{agentToolBase: t.forkAgentToolBase(runtime)}
}

func (t *listAgentSkillCandidatesTool) ForkToolRuntime(runtime *tools.ToolRuntime) tools.Tool {
	return &listAgentSkillCandidatesTool{agentToolBase: t.forkAgentToolBase(runtime)}
}

func (t *listAgentKnowledgeCandidatesTool) ForkToolRuntime(runtime *tools.ToolRuntime) tools.Tool {
	return &listAgentKnowledgeCandidatesTool{agentToolBase: t.forkAgentToolBase(runtime)}
}

func (t *listAgentDatabaseCandidatesTool) ForkToolRuntime(runtime *tools.ToolRuntime) tools.Tool {
	return &listAgentDatabaseCandidatesTool{agentToolBase: t.forkAgentToolBase(runtime)}
}

func (t *listAgentDatabaseTablesTool) ForkToolRuntime(runtime *tools.ToolRuntime) tools.Tool {
	return &listAgentDatabaseTablesTool{agentToolBase: t.forkAgentToolBase(runtime)}
}

func (t *listAgentWorkflowBindingCandidatesTool) ForkToolRuntime(runtime *tools.ToolRuntime) tools.Tool {
	return &listAgentWorkflowBindingCandidatesTool{agentToolBase: t.forkAgentToolBase(runtime)}
}

func (t *replaceAgentSkillBindingsTool) ForkToolRuntime(runtime *tools.ToolRuntime) tools.Tool {
	return &replaceAgentSkillBindingsTool{agentToolBase: t.forkAgentToolBase(runtime)}
}

func (t *replaceAgentKnowledgeBindingsTool) ForkToolRuntime(runtime *tools.ToolRuntime) tools.Tool {
	return &replaceAgentKnowledgeBindingsTool{agentToolBase: t.forkAgentToolBase(runtime)}
}

func (t *replaceAgentDatabaseBindingsTool) ForkToolRuntime(runtime *tools.ToolRuntime) tools.Tool {
	return &replaceAgentDatabaseBindingsTool{agentToolBase: t.forkAgentToolBase(runtime)}
}

func (t *replaceAgentWorkflowBindingsTool) ForkToolRuntime(runtime *tools.ToolRuntime) tools.Tool {
	return &replaceAgentWorkflowBindingsTool{agentToolBase: t.forkAgentToolBase(runtime)}
}

func (t *listAvailableModelsTool) ForkToolRuntime(runtime *tools.ToolRuntime) tools.Tool {
	return &listAvailableModelsTool{agentToolBase: t.forkAgentToolBase(runtime)}
}

func (t agentToolBase) forkAgentToolBase(runtime *tools.ToolRuntime) agentToolBase {
	return agentToolBase{
		BuiltinTool:     t.BuiltinTool.ForkToolRuntime(runtime),
		agentsService:   t.agentsService,
		workspacePerms:  t.workspacePerms,
		availableModels: t.availableModels,
	}
}

func agentScopeFromRuntime(runtime *tools.ToolRuntime, tenantID string, userID string) (agentScope, error) {
	tenantID = strings.TrimSpace(tenantID)
	organizationID := ""
	workspaceID := ""
	invokeFrom := tools.ToolInvokeFromAIChat
	if runtime != nil {
		if strings.TrimSpace(runtime.TenantID) != "" {
			tenantID = strings.TrimSpace(runtime.TenantID)
		}
		organizationID = strings.TrimSpace(stringValue(runtime.RuntimeParameters, "organization_id"))
		workspaceID = strings.TrimSpace(stringValue(runtime.RuntimeParameters, "workspace_id"))
		if runtime.InvokeFrom != "" {
			invokeFrom = runtime.InvokeFrom
		}
	}
	if invokeFrom != tools.ToolInvokeFromAIChat {
		return agentScope{}, fmt.Errorf("agent management is only available from AIChat runtime")
	}
	if organizationID == "" {
		organizationID = tenantID
	}
	accountID := strings.TrimSpace(userID)
	if accountID == "" {
		return agentScope{}, fmt.Errorf("account_id is required")
	}
	if organizationID == "" {
		return agentScope{}, fmt.Errorf("organization_id is required")
	}
	return agentScope{
		OrganizationID: organizationID,
		WorkspaceID:    workspaceID,
		AccountID:      accountID,
		InvokeFrom:     invokeFrom,
	}, nil
}

func mergeAgentConfigRequest(current dto.AgentConfigResponse, params map[string]interface{}) (dto.AgentConfigRequest, []string, error) {
	req := agentConfigRequestFromResponse(current)
	changedFields := make([]string, 0, 12)
	if value, ok := optionalStringParam(params, "system_prompt"); ok {
		req.SystemPrompt = value
		changedFields = append(changedFields, "system_prompt")
	}
	if value, ok := optionalStringParam(params, "model_provider"); ok {
		req.ModelProvider = value
		changedFields = append(changedFields, "model_provider")
	}
	if value, ok := optionalStringParam(params, "model"); ok {
		req.Model = value
		changedFields = append(changedFields, "model")
	}
	if value, ok := mapParam(params, "model_parameters"); ok {
		req.ModelParameters = value
		changedFields = append(changedFields, "model_parameters")
	}
	if value, ok := stringSliceParam(params, "enabled_skill_ids"); ok {
		req.EnabledSkillIDs = value
		changedFields = append(changedFields, "enabled_skill_ids")
	}
	if value, ok := boolParam(params, "agent_memory_enabled"); ok {
		req.AgentMemoryEnabled = value
		changedFields = append(changedFields, "agent_memory_enabled")
	}
	if value, ok := boolParam(params, "file_upload_enabled"); ok {
		req.FileUpload = value
		changedFields = append(changedFields, "file_upload_enabled")
	}
	if value, ok := optionalStringParam(params, "home_title"); ok {
		req.HomeTitle = value
		changedFields = append(changedFields, "home_title")
	}
	if value, ok := optionalStringParam(params, "input_placeholder"); ok {
		req.InputPlaceholder = value
		changedFields = append(changedFields, "input_placeholder")
	}
	if value, ok := optionalStringParam(params, "theme_color"); ok {
		req.ThemeColor = value
		changedFields = append(changedFields, "theme_color")
	}
	if value, ok := stringSliceParam(params, "suggested_questions"); ok {
		req.SuggestedQuestions = value
		changedFields = append(changedFields, "suggested_questions")
	}
	if value, ok := stringSliceParam(params, "knowledge_dataset_ids"); ok {
		req.KnowledgeDatasetIDs = value
		changedFields = append(changedFields, "knowledge_dataset_ids")
	}
	if value, ok := stringSliceParam(params, "dataset_ids"); ok {
		req.KnowledgeDatasetIDs = value
		changedFields = append(changedFields, "knowledge_dataset_ids")
	}
	if value, ok, err := optionalMapParam(params, "knowledge_retrieval_config"); err != nil {
		return req, nil, err
	} else if ok {
		req.KnowledgeRetrievalConfig = value
		changedFields = append(changedFields, "knowledge_retrieval_config")
	}
	if bindings, ok, err := agentDatabaseBindingsParam(params, "database_bindings"); err != nil {
		return req, nil, err
	} else if ok {
		req.DatabaseBindings = bindings
		changedFields = append(changedFields, "database_bindings")
	}
	if bindings, ok, err := agentWorkflowBindingsParam(params, "workflow_bindings"); err != nil {
		return req, nil, err
	} else if ok {
		req.WorkflowBindings = bindings
		changedFields = append(changedFields, "workflow_bindings")
	}
	return req, changedFields, nil
}

func agentConfigRequestFromResponse(current dto.AgentConfigResponse) dto.AgentConfigRequest {
	return dto.AgentConfigRequest{
		SystemPrompt:             current.SystemPrompt,
		ModelProvider:            current.ModelProvider,
		Model:                    current.Model,
		ModelParameters:          copyStringAnyMap(current.ModelParameters),
		EnabledSkillIDs:          append([]string(nil), current.EnabledSkillIDs...),
		UseMemory:                false,
		AgentMemoryEnabled:       current.AgentMemoryEnabled,
		FileUpload:               current.FileUpload,
		HomeTitle:                current.HomeTitle,
		InputPlaceholder:         current.InputPlaceholder,
		ThemeColor:               current.ThemeColor,
		SuggestedQuestions:       append([]string(nil), current.SuggestedQuestions...),
		KnowledgeDatasetIDs:      append([]string(nil), current.KnowledgeDatasetIDs...),
		KnowledgeRetrievalConfig: copyStringAnyMap(current.KnowledgeRetrievalConfig),
		DatabaseBindings:         append([]dto.AgentDatabaseBinding(nil), current.DatabaseBindings...),
		WorkflowBindings:         append([]dto.AgentWorkflowBinding(nil), current.WorkflowBindings...),
	}
}

func agentConfigMutationPayload(effect string, agentID string, workspaceID string, before *dto.AgentConfigResponse, config *dto.AgentConfigResponse, agent map[string]interface{}) map[string]interface{} {
	agent = agentPayload(agent)
	if len(agent) == 0 {
		agent = map[string]interface{}{}
	}
	if agentID = strings.TrimSpace(agentID); agentID != "" {
		agent["id"] = agentID
		agent["agent_id"] = agentID
		if _, ok := agent["href"]; !ok {
			agent["href"] = "/console/agents/" + agentID + "/agent"
		}
	}
	payload := map[string]interface{}{
		"status":       "completed",
		"effect":       effect,
		"agent_id":     agentID,
		"workspace_id": workspaceID,
		"href":         "/console/agents/" + agentID + "/agent",
		"config":       config,
		"agent":        agent,
	}
	if name := strings.TrimSpace(firstStringFromMap(agent, "name", "agent_name")); name != "" {
		payload["agent_name"] = name
	}
	if config != nil {
		payload["knowledge_dataset_ids"] = append([]string(nil), config.KnowledgeDatasetIDs...)
		payload["knowledge_retrieval_config"] = copyStringAnyMap(config.KnowledgeRetrievalConfig)
		payload["database_bindings"] = append([]dto.AgentDatabaseBinding(nil), config.DatabaseBindings...)
		payload["workflow_bindings"] = append([]dto.AgentWorkflowBinding(nil), config.WorkflowBindings...)
	}
	if changes := agentConfigBindingChanges(before, config); len(changes) > 0 {
		payload["config_changes"] = changes
		payload["binding_changes"] = changes
		if primary := primaryAgentConfigBindingChange(changes); len(primary) > 0 {
			for _, field := range []string{
				"binding_kind",
				"change_action",
				"resource_count",
				"resource_names",
				"added_resource_count",
				"added_resource_names",
				"removed_resource_count",
				"removed_resource_names",
				"final_resource_count",
				"final_resource_names",
			} {
				if value, ok := primary[field]; ok {
					payload[field] = value
				}
			}
		}
	}
	return payload
}

func agentConfigBindingChanges(before *dto.AgentConfigResponse, after *dto.AgentConfigResponse) []map[string]interface{} {
	if before == nil || after == nil {
		return nil
	}
	changes := make([]map[string]interface{}, 0, 4)
	if change := agentConfigStringSetChange("enabled_skill_ids", "agent_skill", before.EnabledSkillIDs, after.EnabledSkillIDs, nil); len(change) > 0 {
		changes = append(changes, change)
	}
	if change := agentConfigStringSetChange("knowledge_dataset_ids", "knowledge_base", before.KnowledgeDatasetIDs, after.KnowledgeDatasetIDs, nil); len(change) > 0 {
		changes = append(changes, change)
	}
	if change := agentConfigDatabaseBindingChange(before.DatabaseBindings, after.DatabaseBindings); len(change) > 0 {
		changes = append(changes, change)
	}
	if change := agentConfigWorkflowBindingChange(before.WorkflowBindings, after.WorkflowBindings); len(change) > 0 {
		changes = append(changes, change)
	}
	return changes
}

func primaryAgentConfigBindingChange(changes []map[string]interface{}) map[string]interface{} {
	if len(changes) == 0 {
		return nil
	}
	if len(changes) == 1 {
		return changes[0]
	}
	added := 0
	removed := 0
	for _, change := range changes {
		added += intFromMap(change, "added_resource_count")
		removed += intFromMap(change, "removed_resource_count")
	}
	action := "update"
	switch {
	case removed > 0 && added == 0:
		action = "unbind"
	case added > 0 && removed == 0:
		action = "bind"
	case added > 0 && removed > 0:
		action = "replace"
	}
	return map[string]interface{}{
		"binding_kind":           "multiple",
		"change_action":          action,
		"resource_count":         added + removed,
		"added_resource_count":   added,
		"removed_resource_count": removed,
		"final_resource_count":   agentConfigChangesFinalCount(changes),
		"final_resource_names":   agentConfigChangesNames(changes, "final_resource_names"),
		"added_resource_names":   agentConfigChangesNames(changes, "added_resource_names"),
		"removed_resource_names": agentConfigChangesNames(changes, "removed_resource_names"),
		"resource_names":         agentConfigPrimaryChangeNames(action, changes),
	}
}

func agentConfigStringSetChange(field string, kind string, before []string, after []string, displayNames map[string]string) map[string]interface{} {
	beforeSet := stringSetFromSlice(before)
	afterSet := stringSetFromSlice(after)
	if stringSetsEqual(beforeSet, afterSet) {
		return nil
	}
	names := func(keys []string) []string {
		out := make([]string, 0, len(keys))
		for _, key := range keys {
			if name := strings.TrimSpace(displayNames[key]); name != "" && !strings.EqualFold(name, key) {
				out = append(out, name)
			}
		}
		return out
	}
	addedKeys := sortedSetDiff(afterSet, beforeSet)
	removedKeys := sortedSetDiff(beforeSet, afterSet)
	finalKeys := sortedSetKeys(afterSet)
	return agentConfigChangePayload(field, kind, len(finalKeys), names(finalKeys), len(addedKeys), names(addedKeys), len(removedKeys), names(removedKeys))
}

func agentConfigDatabaseBindingChange(before []dto.AgentDatabaseBinding, after []dto.AgentDatabaseBinding) map[string]interface{} {
	beforeSet, beforeNames := databaseBindingResourceSet(before)
	afterSet, afterNames := databaseBindingResourceSet(after)
	if stringSetsEqual(beforeSet, afterSet) {
		return nil
	}
	addedKeys := sortedSetDiff(afterSet, beforeSet)
	removedKeys := sortedSetDiff(beforeSet, afterSet)
	finalKeys := sortedSetKeys(afterSet)
	return agentConfigChangePayload(
		"database_bindings",
		"database_table",
		len(finalKeys),
		resourceNamesForKeys(finalKeys, afterNames),
		len(addedKeys),
		resourceNamesForKeys(addedKeys, afterNames),
		len(removedKeys),
		resourceNamesForKeys(removedKeys, beforeNames),
	)
}

func agentConfigWorkflowBindingChange(before []dto.AgentWorkflowBinding, after []dto.AgentWorkflowBinding) map[string]interface{} {
	beforeSet, beforeNames := workflowBindingResourceSet(before)
	afterSet, afterNames := workflowBindingResourceSet(after)
	if stringSetsEqual(beforeSet, afterSet) {
		return nil
	}
	addedKeys := sortedSetDiff(afterSet, beforeSet)
	removedKeys := sortedSetDiff(beforeSet, afterSet)
	finalKeys := sortedSetKeys(afterSet)
	return agentConfigChangePayload(
		"workflow_bindings",
		"workflow",
		len(finalKeys),
		resourceNamesForKeys(finalKeys, afterNames),
		len(addedKeys),
		resourceNamesForKeys(addedKeys, afterNames),
		len(removedKeys),
		resourceNamesForKeys(removedKeys, beforeNames),
	)
}

func agentConfigChangePayload(field string, kind string, finalCount int, finalNames []string, addedCount int, addedNames []string, removedCount int, removedNames []string) map[string]interface{} {
	action := "update"
	switch {
	case removedCount > 0 && addedCount == 0:
		action = "unbind"
	case addedCount > 0 && removedCount == 0:
		action = "bind"
	case addedCount > 0 && removedCount > 0:
		action = "replace"
	}
	resourceCount := addedCount + removedCount
	resourceNames := finalNames
	switch action {
	case "bind":
		resourceCount = addedCount
		resourceNames = addedNames
	case "unbind":
		resourceCount = removedCount
		resourceNames = removedNames
	case "replace":
		resourceNames = appendUniqueStrings(addedNames, removedNames...)
	}
	change := map[string]interface{}{
		"field":                  field,
		"binding_kind":           kind,
		"change_action":          action,
		"resource_count":         resourceCount,
		"added_resource_count":   addedCount,
		"removed_resource_count": removedCount,
		"final_resource_count":   finalCount,
	}
	if len(resourceNames) > 0 {
		change["resource_names"] = resourceNames
	}
	if len(addedNames) > 0 {
		change["added_resource_names"] = addedNames
	}
	if len(removedNames) > 0 {
		change["removed_resource_names"] = removedNames
	}
	if len(finalNames) > 0 {
		change["final_resource_names"] = finalNames
	}
	return change
}

func databaseBindingResourceSet(bindings []dto.AgentDatabaseBinding) (map[string]struct{}, map[string]string) {
	set := map[string]struct{}{}
	names := map[string]string{}
	for _, binding := range bindings {
		dataSourceID := strings.TrimSpace(binding.DataSourceID)
		for _, tableID := range binding.TableIDs {
			tableID = strings.TrimSpace(tableID)
			if tableID == "" {
				continue
			}
			key := dataSourceID + ":" + tableID
			set[key] = struct{}{}
		}
	}
	return set, names
}

func workflowBindingResourceSet(bindings []dto.AgentWorkflowBinding) (map[string]struct{}, map[string]string) {
	set := map[string]struct{}{}
	names := map[string]string{}
	for _, binding := range bindings {
		key := firstNonEmptyString(binding.BindingID, binding.WorkflowID, binding.AgentID)
		if key == "" {
			continue
		}
		set[key] = struct{}{}
		if name := strings.TrimSpace(binding.Label); name != "" && !strings.EqualFold(name, key) {
			names[key] = name
		}
	}
	return set, names
}

func resourceNamesForKeys(keys []string, names map[string]string) []string {
	out := make([]string, 0, len(keys))
	for _, key := range keys {
		if name := strings.TrimSpace(names[key]); name != "" && !strings.EqualFold(name, key) {
			out = append(out, name)
		}
	}
	return out
}

func stringSetFromSlice(values []string) map[string]struct{} {
	set := map[string]struct{}{}
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value == "" {
			continue
		}
		set[value] = struct{}{}
	}
	return set
}

func stringSetsEqual(left map[string]struct{}, right map[string]struct{}) bool {
	if len(left) != len(right) {
		return false
	}
	for key := range left {
		if _, ok := right[key]; !ok {
			return false
		}
	}
	return true
}

func sortedSetDiff(left map[string]struct{}, right map[string]struct{}) []string {
	out := make([]string, 0)
	for key := range left {
		if _, ok := right[key]; ok {
			continue
		}
		out = append(out, key)
	}
	return sortedStrings(out)
}

func sortedSetKeys(set map[string]struct{}) []string {
	out := make([]string, 0, len(set))
	for key := range set {
		out = append(out, key)
	}
	return sortedStrings(out)
}

func sortedStrings(values []string) []string {
	for i := 1; i < len(values); i++ {
		for j := i; j > 0 && values[j] < values[j-1]; j-- {
			values[j], values[j-1] = values[j-1], values[j]
		}
	}
	return values
}

func agentConfigChangesNames(changes []map[string]interface{}, field string) []string {
	out := make([]string, 0)
	for _, change := range changes {
		for _, name := range stringSliceFromAny(change[field]) {
			out = appendUniqueStrings(out, name)
		}
	}
	return out
}

func agentConfigChangesFinalCount(changes []map[string]interface{}) int {
	total := 0
	for _, change := range changes {
		total += intFromMap(change, "final_resource_count")
	}
	return total
}

func agentConfigPrimaryChangeNames(action string, changes []map[string]interface{}) []string {
	switch action {
	case "bind":
		return agentConfigChangesNames(changes, "added_resource_names")
	case "unbind":
		return agentConfigChangesNames(changes, "removed_resource_names")
	default:
		names := agentConfigChangesNames(changes, "added_resource_names")
		for _, name := range agentConfigChangesNames(changes, "removed_resource_names") {
			names = appendUniqueStrings(names, name)
		}
		return names
	}
}

func stringSliceFromAny(value interface{}) []string {
	switch typed := value.(type) {
	case []string:
		return append([]string(nil), typed...)
	case []interface{}:
		out := make([]string, 0, len(typed))
		for _, item := range typed {
			if text := strings.TrimSpace(fmt.Sprint(item)); text != "" && text != "<nil>" {
				out = append(out, text)
			}
		}
		return out
	default:
		if text := strings.TrimSpace(fmt.Sprint(value)); text != "" && text != "<nil>" {
			return []string{text}
		}
		return nil
	}
}

func appendUniqueStrings(values []string, additions ...string) []string {
	seen := make(map[string]struct{}, len(values)+len(additions))
	out := make([]string, 0, len(values)+len(additions))
	for _, value := range append(values, additions...) {
		value = strings.TrimSpace(value)
		if value == "" {
			continue
		}
		key := strings.ToLower(value)
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
		out = append(out, value)
	}
	return out
}

func intFromMap(input map[string]interface{}, key string) int {
	if input == nil {
		return 0
	}
	switch typed := input[key].(type) {
	case int:
		return typed
	case int64:
		return int(typed)
	case float64:
		return int(typed)
	case string:
		parsed, _ := strconv.Atoi(strings.TrimSpace(typed))
		return parsed
	default:
		return 0
	}
}

func agentOperationPayload(effect string, agent interface{}) map[string]interface{} {
	agentMap := agentPayload(agent)
	payload := map[string]interface{}{
		"status": "completed",
		"effect": effect,
		"agent":  agentMap,
	}
	if id := strings.TrimSpace(stringValue(agentMap, "id")); id != "" {
		payload["agent_id"] = id
		payload["href"] = "/console/agents/" + id + "/agent"
	}
	if name := strings.TrimSpace(stringValue(agentMap, "name")); name != "" {
		payload["agent_name"] = name
	}
	if workspaceID := strings.TrimSpace(firstNonEmptyString(stringValue(agentMap, "workspace_id"), stringValue(agentMap, "tenant_id"))); workspaceID != "" {
		payload["workspace_id"] = workspaceID
	}
	return payload
}

func agentPayload(agent interface{}) map[string]interface{} {
	mapped := mapFromAny(agent)
	if len(mapped) == 0 {
		return map[string]interface{}{}
	}
	if id := strings.TrimSpace(firstStringFromMap(mapped, "id", "agent_id")); id != "" {
		mapped["id"] = id
		mapped["agent_id"] = id
		if _, ok := mapped["href"]; !ok {
			mapped["href"] = "/console/agents/" + id + "/agent"
		}
	}
	if workspaceID := strings.TrimSpace(firstStringFromMap(mapped, "workspace_id", "tenant_id")); workspaceID != "" {
		mapped["workspace_id"] = workspaceID
	}
	return mapped
}

func agentListItemPayload(item dto.AgentListItem) map[string]interface{} {
	payload := map[string]interface{}{
		"id":             item.ID,
		"agent_id":       item.ID,
		"name":           item.Name,
		"description":    item.Description,
		"agent_type":     item.AgentType,
		"workspace_id":   firstNonEmptyString(item.WorkspaceID, item.TenantID),
		"icon_url":       item.IconUrl,
		"is_published":   item.IsPublished,
		"web_app_status": item.WebAppStatus,
		"can_edit":       item.CanEdit,
		"href":           "/console/agents/" + item.ID + "/agent",
	}
	if item.IconType != nil {
		payload["icon_type"] = *item.IconType
	}
	if item.Icon != nil {
		payload["icon"] = *item.Icon
	}
	return payload
}

func visibleAgentsFromRuntime(runtime *tools.ToolRuntime) []map[string]interface{} {
	if runtime == nil || len(runtime.RuntimeParameters) == 0 {
		return nil
	}
	items := interfaceSlice(firstNonNil(
		runtime.RuntimeParameters["console_agents_visible_agents"],
		runtime.RuntimeParameters["console_agent_visible_agents"],
		runtime.RuntimeParameters["visible_agents"],
	))
	out := make([]map[string]interface{}, 0, len(items))
	for _, item := range items {
		mapped := mapFromAny(item)
		if len(mapped) == 0 || hasNonAgentResourceMarker(mapped) {
			continue
		}
		agentID := strings.TrimSpace(firstStringFromMap(mapped, "agent_id", "id", "resource_id"))
		name := strings.TrimSpace(firstStringFromMap(mapped, "name", "title", "agent_name"))
		if agentID == "" && name == "" {
			continue
		}
		agent := map[string]interface{}{}
		if agentID != "" {
			agent["agent_id"] = agentID
			agent["id"] = agentID
			agent["href"] = "/console/agents/" + agentID + "/agent"
		}
		if name != "" {
			agent["name"] = name
		}
		if workspaceID := strings.TrimSpace(firstStringFromMap(mapped, "workspace_id", "workspaceId", "tenant_id")); workspaceID != "" {
			agent["workspace_id"] = workspaceID
		}
		if agentType := strings.TrimSpace(firstStringFromMap(mapped, "agent_type", "type")); agentType != "" {
			agent["agent_type"] = agentType
		}
		out = append(out, agent)
	}
	return out
}

type agentBatchDeleteTarget struct {
	ID          string
	Name        string
	WorkspaceID string
}

func agentBatchDeleteTargets(params map[string]interface{}, runtime *tools.ToolRuntime) ([]agentBatchDeleteTarget, error) {
	visibleByID, visibleByName := visibleAgentTargetIndexes(runtime)
	targets := make([]agentBatchDeleteTarget, 0)
	seen := map[string]struct{}{}
	for _, key := range []string{"agents", "targets", "assets"} {
		value, ok := params[key]
		if !ok || value == nil {
			continue
		}
		parsed, err := agentBatchDeleteTargetsFromAny(value)
		if err != nil {
			return nil, err
		}
		for _, target := range parsed {
			targets = appendAgentBatchDeleteTarget(targets, seen, enrichAgentBatchDeleteTarget(target, visibleByID, visibleByName))
		}
	}
	for _, key := range []string{"agent_ids", "ids", "asset_ids"} {
		ids, ok := stringSliceParam(params, key)
		if !ok {
			continue
		}
		for _, id := range ids {
			targets = appendAgentBatchDeleteTarget(targets, seen, enrichAgentBatchDeleteTarget(agentBatchDeleteTarget{ID: id}, visibleByID, visibleByName))
		}
	}
	if len(targets) == 0 {
		if id := requiredAgentID(params); id != "" {
			targets = appendAgentBatchDeleteTarget(targets, seen, enrichAgentBatchDeleteTarget(agentBatchDeleteTarget{ID: id}, visibleByID, visibleByName))
		}
	}
	for _, target := range targets {
		if strings.TrimSpace(target.ID) == "" {
			return nil, fmt.Errorf("agent_id is required for every batch delete target")
		}
	}
	return targets, nil
}

func agentBatchDeleteTargetsFromAny(value interface{}) ([]agentBatchDeleteTarget, error) {
	switch typed := value.(type) {
	case []agentBatchDeleteTarget:
		return append([]agentBatchDeleteTarget(nil), typed...), nil
	case []map[string]interface{}:
		out := make([]agentBatchDeleteTarget, 0, len(typed))
		for _, item := range typed {
			out = append(out, agentBatchDeleteTargetFromMap(item))
		}
		return out, nil
	case []interface{}:
		out := make([]agentBatchDeleteTarget, 0, len(typed))
		for _, item := range typed {
			mapped := mapFromAny(item)
			if len(mapped) == 0 {
				text := strings.TrimSpace(fmt.Sprint(item))
				if text != "" {
					out = append(out, agentBatchDeleteTarget{ID: text})
				}
				continue
			}
			out = append(out, agentBatchDeleteTargetFromMap(mapped))
		}
		return out, nil
	case string:
		text := strings.TrimSpace(typed)
		if text == "" {
			return nil, nil
		}
		var records []map[string]interface{}
		if strings.HasPrefix(text, "[") {
			if err := json.Unmarshal([]byte(text), &records); err == nil {
				return agentBatchDeleteTargetsFromAny(records)
			}
			var ids []string
			if err := json.Unmarshal([]byte(text), &ids); err == nil {
				out := make([]agentBatchDeleteTarget, 0, len(ids))
				for _, id := range ids {
					out = append(out, agentBatchDeleteTarget{ID: id})
				}
				return out, nil
			}
		}
		ids := strings.Split(text, ",")
		out := make([]agentBatchDeleteTarget, 0, len(ids))
		for _, id := range ids {
			if id = strings.TrimSpace(id); id != "" {
				out = append(out, agentBatchDeleteTarget{ID: id})
			}
		}
		return out, nil
	default:
		mapped := mapFromAny(value)
		if len(mapped) == 0 {
			return nil, fmt.Errorf("agents must be a JSON array of agent targets")
		}
		return []agentBatchDeleteTarget{agentBatchDeleteTargetFromMap(mapped)}, nil
	}
}

func agentBatchDeleteTargetFromMap(input map[string]interface{}) agentBatchDeleteTarget {
	if len(input) == 0 {
		return agentBatchDeleteTarget{}
	}
	return agentBatchDeleteTarget{
		ID:          strings.TrimSpace(firstStringFromMap(input, "agent_id", "agentId", "id", "asset_id", "resource_id")),
		Name:        strings.TrimSpace(firstStringFromMap(input, "agent_name", "agentName", "name", "title", "asset_name", "resource_name")),
		WorkspaceID: strings.TrimSpace(firstStringFromMap(input, "workspace_id", "workspaceId", "tenant_id")),
	}
}

func visibleAgentTargetIndexes(runtime *tools.ToolRuntime) (map[string]agentBatchDeleteTarget, map[string]agentBatchDeleteTarget) {
	byID := map[string]agentBatchDeleteTarget{}
	byName := map[string]agentBatchDeleteTarget{}
	for _, item := range visibleAgentsFromRuntime(runtime) {
		target := agentBatchDeleteTargetFromMap(item)
		if target.ID != "" {
			byID[target.ID] = target
		}
		if target.Name != "" {
			byName[strings.ToLower(target.Name)] = target
		}
	}
	return byID, byName
}

func enrichAgentBatchDeleteTarget(target agentBatchDeleteTarget, visibleByID map[string]agentBatchDeleteTarget, visibleByName map[string]agentBatchDeleteTarget) agentBatchDeleteTarget {
	target.ID = strings.TrimSpace(target.ID)
	target.Name = strings.TrimSpace(target.Name)
	target.WorkspaceID = strings.TrimSpace(target.WorkspaceID)
	if target.ID != "" {
		if visible, ok := visibleByID[target.ID]; ok {
			if target.Name == "" {
				target.Name = visible.Name
			}
			if target.WorkspaceID == "" {
				target.WorkspaceID = visible.WorkspaceID
			}
		}
		return target
	}
	if target.Name == "" {
		return target
	}
	if visible, ok := visibleByName[strings.ToLower(target.Name)]; ok {
		if target.ID == "" {
			target.ID = visible.ID
		}
		if target.WorkspaceID == "" {
			target.WorkspaceID = visible.WorkspaceID
		}
	}
	return target
}

func appendAgentBatchDeleteTarget(targets []agentBatchDeleteTarget, seen map[string]struct{}, target agentBatchDeleteTarget) []agentBatchDeleteTarget {
	target.ID = strings.TrimSpace(target.ID)
	target.Name = strings.TrimSpace(target.Name)
	target.WorkspaceID = strings.TrimSpace(target.WorkspaceID)
	key := target.ID
	if key == "" {
		key = strings.ToLower(target.Name)
	}
	if key == "" {
		return targets
	}
	if _, ok := seen[key]; ok {
		return targets
	}
	seen[key] = struct{}{}
	return append(targets, target)
}

func agentBatchDeleteTargetsPayload(targets []agentBatchDeleteTarget) []map[string]interface{} {
	out := make([]map[string]interface{}, 0, len(targets))
	for _, target := range targets {
		item := map[string]interface{}{
			"agent_id": target.ID,
			"id":       target.ID,
			"type":     "agent",
			"href":     "/console/agents/" + target.ID + "/agent",
		}
		if target.Name != "" {
			item["name"] = target.Name
			item["agent_name"] = target.Name
		}
		if target.WorkspaceID != "" {
			item["workspace_id"] = target.WorkspaceID
		}
		out = append(out, item)
	}
	return out
}

func normalizeAgentModelUseCase(raw string) string {
	value := strings.ToLower(strings.TrimSpace(raw))
	switch value {
	case "":
		return defaultAgentModelListUseCase
	case "all", "any", "*":
		return ""
	case "chat", "llm", "text", "text_chat":
		return string(llmmodelmodel.UseCaseTextChat)
	case "image", "image_generation", "image-generation":
		return string(llmmodelmodel.UseCaseImageGen)
	case "function_calling", "function calling", "tool", "tools", "tool-call", "tool_call":
		return string(llmmodelmodel.UseCaseFuncCalling)
	case "speech_to_text", "stt":
		return string(llmmodelmodel.UseCaseSpeechToText)
	case "text_to_speech", "tts":
		return string(llmmodelmodel.UseCaseTextToSpeech)
	case "realtime_audio", "realtime audio":
		return string(llmmodelmodel.UseCaseRealtimeAudio)
	case "video", "video_generation", "video-generation":
		return string(llmmodelmodel.UseCaseVideoGen)
	case "推理":
		return string(llmmodelmodel.UseCaseReasoning)
	case "视觉", "图片理解":
		return string(llmmodelmodel.UseCaseVision)
	case "函数调用", "工具调用":
		return string(llmmodelmodel.UseCaseFuncCalling)
	default:
		return value
	}
}

func validAgentModelUseCases() []string {
	useCases := llmmodelmodel.ValidUseCases()
	out := make([]string, 0, len(useCases))
	for _, useCase := range useCases {
		out = append(out, string(useCase))
	}
	return out
}

func isValidAgentModelUseCase(value string) bool {
	for _, useCase := range llmmodelmodel.ValidUseCases() {
		if string(useCase) == value {
			return true
		}
	}
	return false
}

func availableAgentModelPayload(model *llmmodelservice.AvailableModel) map[string]interface{} {
	if model == nil {
		return map[string]interface{}{}
	}
	return map[string]interface{}{
		"id":                model.ID.String(),
		"provider":          model.Provider,
		"model":             model.Name,
		"model_name":        model.DisplayName,
		"display_name":      model.DisplayName,
		"use_cases":         append([]string(nil), model.UseCases...),
		"context_window":    model.ContextWindow,
		"max_output_tokens": model.MaxOutputTokens,
		"is_custom":         model.IsCustom,
		"source_channel":    model.SourceChannel,
		"capabilities": map[string]interface{}{
			"streaming":          model.Features.Streaming,
			"function_calling":   model.Features.FunctionCalling,
			"structured_output":  model.Features.StructuredOutput,
			"json_mode":          model.Features.JsonMode,
			"reasoning":          model.Features.Reasoning,
			"system_prompt":      model.Features.SystemPrompt,
			"vision":             model.Endpoints.Vision,
			"image_generation":   model.Endpoints.ImageGeneration,
			"speech_generation":  model.Endpoints.SpeechGeneration,
			"transcription":      model.Endpoints.Transcription,
			"parallel_tool_call": model.Tools.ParallelToolCalls,
		},
		"parameters": map[string]interface{}{
			"supports_temperature":       model.Parameters.SupportsTemperature,
			"supports_top_p":             model.Parameters.SupportsTopP,
			"supports_presence_penalty":  model.Parameters.SupportsPresencePenalty,
			"supports_frequency_penalty": model.Parameters.SupportsFrequencyPenalty,
			"supports_stop":              model.Parameters.SupportsStop,
			"max_stop_sequences":         model.Parameters.MaxStopSequences,
		},
	}
}

func hasNonAgentResourceMarker(mapped map[string]interface{}) bool {
	for _, key := range []string{"type", "resource_type", "kind", "resource_kind"} {
		if value := stringValue(mapped, key); value != "" && !strings.EqualFold(value, "agent") && !strings.EqualFold(value, "AGENT") {
			return true
		}
	}
	metadata := mapFromAny(mapped["metadata"])
	if value := firstStringFromMap(metadata, "resource_kind"); value != "" && !strings.EqualFold(value, "agent") {
		return true
	}
	return false
}

func requiredAgentID(params map[string]interface{}) string {
	return firstNonEmptyString(stringValue(params, "agent_id"), stringValue(params, "id"), stringValue(params, "asset_id"))
}

func normalizeAgentIconParams(iconType string, icon string, iconBackground string) (string, string) {
	iconType = strings.ToLower(strings.TrimSpace(iconType))
	icon = strings.TrimSpace(icon)
	iconBackground = strings.TrimSpace(iconBackground)
	switch iconType {
	case "emoji", "icon", "color":
		iconType = "text"
	case "":
		if icon != "" || iconBackground != "" {
			iconType = "text"
		}
	}
	if iconType != "text" || (icon == "" && iconBackground == "") {
		return iconType, icon
	}
	if json.Valid([]byte(icon)) {
		if iconBackground == "" {
			return iconType, icon
		}
		var payload map[string]interface{}
		if err := json.Unmarshal([]byte(icon), &payload); err != nil {
			return iconType, icon
		}
		payload["icon_background"] = iconBackground
		encoded, err := json.Marshal(payload)
		if err != nil {
			return iconType, icon
		}
		return iconType, string(encoded)
	}
	if icon == "" {
		return iconType, icon
	}
	if iconBackground == "" {
		iconBackground = defaultAgentTextIconBackground
	}
	payload := map[string]interface{}{
		"icon":            icon,
		"icon_background": iconBackground,
	}
	encoded, err := json.Marshal(payload)
	if err != nil {
		return iconType, icon
	}
	return iconType, string(encoded)
}

func appendUniqueString(values []string, value string) []string {
	value = strings.TrimSpace(value)
	if value == "" {
		return values
	}
	for _, existing := range values {
		if existing == value {
			return values
		}
	}
	return append(values, value)
}

func defaultAgentTextIcon(name string) string {
	name = strings.TrimSpace(name)
	if name == "" {
		return "AI"
	}
	words := strings.FieldsFunc(name, isAgentIconWordSeparator)
	if len(words) > 1 {
		initials := make([]rune, 0, 2)
		for _, word := range words {
			if r, ok := firstAgentIconRune(word); ok {
				initials = append(initials, unicode.ToUpper(r))
				if len(initials) == 2 {
					return string(initials)
				}
			}
		}
	}
	runes := collectAgentIconRunes(name, 2)
	if len(runes) == 0 {
		return "AI"
	}
	return string(runes)
}

func isAgentIconWordSeparator(r rune) bool {
	if unicode.IsSpace(r) {
		return true
	}
	switch r {
	case '-', '_', '.', '/', '\\', '|', ':', ';', ',', '+', '(', ')', '[', ']', '{', '}':
		return true
	default:
		return false
	}
}

func firstAgentIconRune(value string) (rune, bool) {
	for _, r := range value {
		if isAgentIconRune(r) {
			return r, true
		}
	}
	return 0, false
}

func collectAgentIconRunes(value string, limit int) []rune {
	out := make([]rune, 0, limit)
	for _, r := range value {
		if !isAgentIconRune(r) {
			continue
		}
		out = append(out, unicode.ToUpper(r))
		if len(out) == limit {
			return out
		}
	}
	return out
}

func isAgentIconRune(r rune) bool {
	return unicode.IsLetter(r) || unicode.IsNumber(r)
}

func optionalStringParam(params map[string]interface{}, key string) (string, bool) {
	if params == nil {
		return "", false
	}
	value, ok := params[key]
	if !ok || value == nil {
		return "", false
	}
	return strings.TrimSpace(fmt.Sprint(value)), true
}

func stringValue(params map[string]interface{}, key string) string {
	if params == nil {
		return ""
	}
	value, ok := params[key]
	if !ok || value == nil {
		return ""
	}
	return strings.TrimSpace(fmt.Sprint(value))
}

func intParam(params map[string]interface{}, key string, defaultValue int, maxValue int) int {
	value := strings.TrimSpace(stringValue(params, key))
	if value == "" {
		return defaultValue
	}
	parsed, err := strconv.Atoi(value)
	if err != nil || parsed <= 0 {
		return defaultValue
	}
	if maxValue > 0 && parsed > maxValue {
		return maxValue
	}
	return parsed
}

func boolParam(params map[string]interface{}, key string) (bool, bool) {
	if params == nil {
		return false, false
	}
	value, ok := params[key]
	if !ok || value == nil {
		return false, false
	}
	switch typed := value.(type) {
	case bool:
		return typed, true
	case string:
		parsed, err := strconv.ParseBool(strings.TrimSpace(typed))
		return parsed, err == nil
	default:
		return false, false
	}
}

func boolParamDefault(params map[string]interface{}, key string, defaultValue bool) bool {
	if value, ok := boolParam(params, key); ok {
		return value
	}
	return defaultValue
}

func optionalMapParam(params map[string]interface{}, key string) (map[string]interface{}, bool, error) {
	if params == nil {
		return nil, false, nil
	}
	value, ok := params[key]
	if !ok || value == nil {
		return nil, false, nil
	}
	switch typed := value.(type) {
	case map[string]interface{}:
		return copyStringAnyMap(typed), true, nil
	case map[string]string:
		mapped := make(map[string]interface{}, len(typed))
		for itemKey, itemValue := range typed {
			mapped[itemKey] = itemValue
		}
		return mapped, true, nil
	case string:
		text := strings.TrimSpace(typed)
		if text == "" {
			return map[string]interface{}{}, true, nil
		}
		var out map[string]interface{}
		if err := json.Unmarshal([]byte(text), &out); err != nil {
			return nil, true, fmt.Errorf("%s must be a JSON object: %w", key, err)
		}
		if out == nil {
			out = map[string]interface{}{}
		}
		return out, true, nil
	default:
		mapped := mapFromAny(value)
		if mapped == nil {
			return nil, true, fmt.Errorf("%s must be a JSON object", key)
		}
		return mapped, true, nil
	}
}

func mapParam(params map[string]interface{}, key string) (map[string]interface{}, bool) {
	if params == nil {
		return nil, false
	}
	value, ok := params[key]
	if !ok || value == nil {
		return nil, false
	}
	if typed, ok := value.(string); ok {
		text := strings.TrimSpace(typed)
		if text == "" {
			return map[string]interface{}{}, true
		}
		var out map[string]interface{}
		if json.Unmarshal([]byte(text), &out) == nil {
			return out, true
		}
		return nil, false
	}
	mapped := mapFromAny(value)
	return mapped, len(mapped) > 0
}

func stringSliceParam(params map[string]interface{}, key string) ([]string, bool) {
	if params == nil {
		return nil, false
	}
	value, ok := params[key]
	if !ok || value == nil {
		return nil, false
	}
	switch typed := value.(type) {
	case []string:
		return append([]string(nil), typed...), true
	case []interface{}:
		out := make([]string, 0, len(typed))
		for _, item := range typed {
			text := strings.TrimSpace(fmt.Sprint(item))
			if text != "" {
				out = append(out, text)
			}
		}
		return out, true
	case string:
		text := strings.TrimSpace(typed)
		if text == "" {
			return []string{}, true
		}
		var out []string
		if strings.HasPrefix(text, "[") && json.Unmarshal([]byte(text), &out) == nil {
			return out, true
		}
		parts := strings.Split(text, ",")
		out = make([]string, 0, len(parts))
		for _, part := range parts {
			part = strings.TrimSpace(part)
			if part != "" {
				out = append(out, part)
			}
		}
		return out, true
	default:
		return nil, false
	}
}

func agentMemorySlotsParam(params map[string]interface{}, keys ...string) ([]dto.AgentMemorySlotConfig, bool, error) {
	if params == nil {
		return nil, false, nil
	}
	for _, key := range keys {
		value, ok := params[key]
		if !ok || value == nil {
			continue
		}
		slots, err := agentMemorySlotsFromAny(value)
		if err != nil {
			return nil, true, err
		}
		return slots, true, nil
	}
	return nil, false, nil
}

func agentMemorySlotsFromAny(value interface{}) ([]dto.AgentMemorySlotConfig, error) {
	switch typed := value.(type) {
	case []dto.AgentMemorySlotConfig:
		return append([]dto.AgentMemorySlotConfig(nil), typed...), nil
	case string:
		text := strings.TrimSpace(typed)
		if text == "" {
			return []dto.AgentMemorySlotConfig{}, nil
		}
		return decodeAgentMemorySlots([]byte(text))
	default:
		data, err := json.Marshal(value)
		if err != nil {
			return nil, fmt.Errorf("agent_memory_slots must be a JSON array of slot objects: %w", err)
		}
		return decodeAgentMemorySlots(data)
	}
}

func decodeAgentMemorySlots(data []byte) ([]dto.AgentMemorySlotConfig, error) {
	var slots []dto.AgentMemorySlotConfig
	if err := json.Unmarshal(data, &slots); err != nil {
		return nil, fmt.Errorf("agent_memory_slots must be a JSON array of slot objects: %w", err)
	}
	return slots, nil
}

func agentDatabaseBindingsParam(params map[string]interface{}, keys ...string) ([]dto.AgentDatabaseBinding, bool, error) {
	if params == nil {
		return nil, false, nil
	}
	for _, key := range keys {
		value, ok := params[key]
		if !ok || value == nil {
			continue
		}
		bindings, err := agentDatabaseBindingsFromAny(value)
		if err != nil {
			return nil, true, err
		}
		return bindings, true, nil
	}
	return nil, false, nil
}

func agentDatabaseBindingsFromAny(value interface{}) ([]dto.AgentDatabaseBinding, error) {
	switch typed := value.(type) {
	case []dto.AgentDatabaseBinding:
		return append([]dto.AgentDatabaseBinding(nil), typed...), nil
	case string:
		text := strings.TrimSpace(typed)
		if text == "" {
			return []dto.AgentDatabaseBinding{}, nil
		}
		return decodeAgentDatabaseBindings([]byte(text))
	default:
		data, err := json.Marshal(value)
		if err != nil {
			return nil, fmt.Errorf("bindings must be a JSON array of database binding objects: %w", err)
		}
		return decodeAgentDatabaseBindings(data)
	}
}

func decodeAgentDatabaseBindings(data []byte) ([]dto.AgentDatabaseBinding, error) {
	var bindings []dto.AgentDatabaseBinding
	if err := json.Unmarshal(data, &bindings); err != nil {
		return nil, fmt.Errorf("bindings must be a JSON array of database binding objects: %w", err)
	}
	return bindings, nil
}

func agentWorkflowBindingsParam(params map[string]interface{}, keys ...string) ([]dto.AgentWorkflowBinding, bool, error) {
	if params == nil {
		return nil, false, nil
	}
	for _, key := range keys {
		value, ok := params[key]
		if !ok || value == nil {
			continue
		}
		bindings, err := agentWorkflowBindingsFromAny(value)
		if err != nil {
			return nil, true, err
		}
		return bindings, true, nil
	}
	return nil, false, nil
}

func agentWorkflowBindingsFromAny(value interface{}) ([]dto.AgentWorkflowBinding, error) {
	switch typed := value.(type) {
	case []dto.AgentWorkflowBinding:
		return append([]dto.AgentWorkflowBinding(nil), typed...), nil
	case string:
		text := strings.TrimSpace(typed)
		if text == "" {
			return []dto.AgentWorkflowBinding{}, nil
		}
		return decodeAgentWorkflowBindings([]byte(text))
	default:
		data, err := json.Marshal(value)
		if err != nil {
			return nil, fmt.Errorf("bindings must be a JSON array of workflow binding objects: %w", err)
		}
		return decodeAgentWorkflowBindings(data)
	}
}

func decodeAgentWorkflowBindings(data []byte) ([]dto.AgentWorkflowBinding, error) {
	var bindings []dto.AgentWorkflowBinding
	if err := json.Unmarshal(data, &bindings); err != nil {
		return nil, fmt.Errorf("bindings must be a JSON array of workflow binding objects: %w", err)
	}
	return bindings, nil
}

func mapFromAny(value interface{}) map[string]interface{} {
	switch typed := value.(type) {
	case nil:
		return nil
	case map[string]interface{}:
		out := make(map[string]interface{}, len(typed))
		for key, item := range typed {
			out[key] = item
		}
		return out
	case map[string]string:
		out := make(map[string]interface{}, len(typed))
		for key, item := range typed {
			out[key] = item
		}
		return out
	default:
		data, err := json.Marshal(value)
		if err != nil {
			return nil
		}
		out := map[string]interface{}{}
		if err := json.Unmarshal(data, &out); err != nil {
			return nil
		}
		return out
	}
}

func copyStringAnyMap(input map[string]interface{}) map[string]interface{} {
	if input == nil {
		return map[string]interface{}{}
	}
	out := make(map[string]interface{}, len(input))
	for key, value := range input {
		out[key] = value
	}
	return out
}

func interfaceSlice(value interface{}) []interface{} {
	switch typed := value.(type) {
	case []interface{}:
		return typed
	case []map[string]interface{}:
		out := make([]interface{}, 0, len(typed))
		for _, item := range typed {
			out = append(out, item)
		}
		return out
	default:
		if value == nil {
			return nil
		}
		return []interface{}{value}
	}
}

func firstStringFromMap(input map[string]interface{}, keys ...string) string {
	if len(input) == 0 {
		return ""
	}
	for _, key := range keys {
		if value := strings.TrimSpace(fmt.Sprint(input[key])); value != "" && value != "<nil>" {
			return value
		}
	}
	return ""
}

func firstNonEmptyString(values ...string) string {
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value != "" {
			return value
		}
	}
	return ""
}

func firstNonNil(values ...interface{}) interface{} {
	for _, value := range values {
		if value != nil {
			return value
		}
	}
	return nil
}

var _ tools.ToolProvider = (*Provider)(nil)
