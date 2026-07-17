package agentmanagement

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"reflect"
	"strconv"
	"strings"
	"unicode"
	"unicode/utf8"

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
	defaultAgentModelListUseCase             = string(llmmodelmodel.UseCaseAgent)
	defaultAgentModelListPageSize            = 20
	maxAgentModelListPageSize                = 100
	defaultAgentTextIconBackground           = "#0847f7"
	maxManagedFileSystemPromptChars          = 16000
	maxSystemPromptPatchSeparatorChars       = 64
	systemPromptPatchOperationAppend         = "append"
	systemPromptPatchOperationUpsertSection  = "upsert_section"
	defaultSystemPromptPatchSeparator        = "\n\n"
	maxSystemPromptPatchSectionIDChars       = 64
	maxSystemPromptPatchSectionTitleChars    = 128
	systemPromptSourceTypeManagedFile        = "managed_file"
	systemPromptSourceTypeText               = "text"
	agentSystemPromptSourceChangedCode       = "agent_system_prompt_source_changed"
	agentSystemPromptPatchInvalidCode        = "agent_system_prompt_patch_invalid"
)

var allowedAgentThemeColors = []string{"default", "blue", "emerald", "violet", "rose", "amber", "slate"}

type WorkspacePermissionService interface {
	CheckWorkspacePermission(ctx context.Context, organizationID, workspaceID, accountID string, permissionCode workspacemodel.WorkspacePermissionCode) (bool, error)
}

type AvailableModelsService interface {
	ListAvailable(ctx context.Context, organizationID uuid.UUID, provider string, useCase string) ([]*llmmodelservice.AvailableModel, error)
}

type ManagedFileService interface {
	GetFileByID(ctx context.Context, fileID string) (*dto.UploadFile, error)
	GetFile(ctx context.Context, fileID string) (string, error)
}

type AgentSystemPromptPatchService interface {
	UpdateAgentConfigWithSystemPromptPatch(ctx context.Context, agentID, accountID string, req dto.AgentSystemPromptPatchRequest) (*dto.AgentConfigResponse, error)
}

type stableCodeError interface {
	ErrorCode() string
}

type agentConfigDisplayNames struct {
	Skills         map[string]string
	KnowledgeBases map[string]string
	DatabaseTables map[string]string
	Workflows      map[string]string
}

type Provider struct {
	*builtin.BuiltinProvider
	agentsService   interfaces.AgentsService
	workspacePerms  WorkspacePermissionService
	availableModels AvailableModelsService
	managedFiles    ManagedFileService
}

type ProviderOption func(*Provider)

func WithManagedFileService(service ManagedFileService) ProviderOption {
	return func(provider *Provider) {
		provider.managedFiles = service
	}
}

func NewProvider(agentsService interfaces.AgentsService, workspacePerms WorkspacePermissionService, availableModels AvailableModelsService, options ...ProviderOption) *Provider {
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
	for _, option := range options {
		if option != nil {
			option(provider)
		}
	}
	provider.RegisterTool(newListAgentsTool(agentsService))
	provider.RegisterTool(newGetAgentTool(agentsService))
	provider.RegisterTool(newCreateAgentTool(agentsService, workspacePerms))
	provider.RegisterTool(newUpdateAgentIdentityTool(agentsService))
	provider.RegisterTool(newDeleteAgentTool(agentsService))
	provider.RegisterTool(newDeleteAgentsTool(agentsService))
	provider.RegisterTool(newGetAgentConfigTool(agentsService))
	provider.RegisterTool(newUpdateAgentConfigToolWithManagedFiles(agentsService, availableModels, provider.managedFiles, workspacePerms))
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
type updateAgentConfigTool struct {
	agentToolBase
	managedFiles ManagedFileService
}

type resolvedSystemPromptMutation struct {
	Source     map[string]interface{}
	Patch      map[string]interface{}
	Digest     string
	Characters int
	Request    *dto.AgentSystemPromptPatchRequest
}
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
		"List or search Agent assets from the authoritative backend in the current workspace. Use the returned order for ordinal targets such as the first visible Agent. For named mutation targets, do at most one exact-name search and one broader list/check before reporting a missing target; do not repeat near-duplicate searches.",
		"list",
		[]tools.ToolParameter{
			stringParameter("workspace_id", "Workspace ID", "Optional workspace ID. Usually omit so current AIChat workspace context is used.", false),
			stringParameter("keyword", "Keyword", "Optional search keyword for Agent name or description.", false),
			numberParameter("page", "Page", "Optional one-based page number.", false),
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
		"Create one AGENT asset in the current workspace after governance and permission checks. This only creates a draft Agent and does not publish it. The result's agent_id/detail_href is the authoritative target for later config edits in the same turn; do not rediscover the new Agent by name. Creating an Agent does not by itself set model, prompt, file upload, memory, skills, knowledge, databases, or workflows.",
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
		"Read the current draft runtime configuration for one resolved AGENT asset. Use this as authoritative verification evidence after config or binding updates, and as evidence for answering read-only capability questions. Do not call candidate-list tools just to inspect already bound counts.",
		"settings",
		[]tools.ToolParameter{
			stringParameter("agent_id", "Agent ID", "Required Agent ID from page context, list_agents, or governed asset resolution. Do not invent IDs.", true),
		},
	), agentsService, nil, nil)}
}

func newUpdateAgentConfigTool(agentsService interfaces.AgentsService, availableModels ...AvailableModelsService) tools.Tool {
	var models AvailableModelsService
	if len(availableModels) > 0 {
		models = availableModels[0]
	}
	return newUpdateAgentConfigToolWithManagedFiles(agentsService, models, nil, nil)
}

func newUpdateAgentConfigToolWithManagedFiles(agentsService interfaces.AgentsService, availableModels AvailableModelsService, managedFiles ManagedFileService, workspacePerms WorkspacePermissionService) tools.Tool {
	return &updateAgentConfigTool{agentToolBase: newAgentToolBase(agentToolEntity(
		ToolUpdateAgentConfig,
		"Update Agent Config",
		"Patch selected draft runtime configuration fields for one resolved AGENT asset. One call may update multiple requested config sections such as model, prompt, file_upload_enabled, suggested questions, and add/remove bindings. Omitted fields are preserved. A system-prompt change must send the complete final prompt after reading the current config, preserving unrelated content and applying the user's requested transformation. Prefer add/remove binding parameters for specific bind/unbind requests, and use full replacement lists only when the user asks to replace or clear an entire section. The result's updated_fields/config_changes/binding_changes are authoritative; do not claim omitted fields changed.",
		"sliders-horizontal",
		[]tools.ToolParameter{
			stringParameter("agent_id", "Agent ID", "Required Agent ID from page context, list_agents, or governed asset resolution. Do not invent IDs.", true),
			stringParameter("system_prompt", "System prompt", "Optional complete replacement system prompt. Read the current config first, preserve every unrelated part, apply only the user's requested change, and send the full final prompt. When source material is involved, treat it as input rather than content to copy by default; match the user's requested scope and level of detail, and reproduce the source verbatim only when explicitly requested.", false),
			stringParameter("model_provider", "Model provider", "Required whenever model is provided. When changing this field, also provide model from the same list_available_models item.", false),
			stringParameter("model", "Model", "Optional replacement model ID. When changing this field, also provide model_provider from the same list_available_models item.", false),
			objectParameter("model_parameters", "Model parameters", "Optional replacement model parameter object.", false),
			stringArrayParameter("enabled_skill_ids", "Enabled skill IDs", "Optional full list of enabled user-selectable skill IDs.", false),
			boolParameter("agent_memory_enabled", "Agent memory enabled", "Optional Agent memory switch.", false),
			boolParameter("file_upload_enabled", "File upload enabled", "Optional file upload switch.", false),
			stringParameter("home_title", "Home title", "Optional Agent home title.", false),
			stringParameter("opening_statement", "Opening statement", "Optional Markdown landing guide shown before the first message.", false),
			stringParameter("input_placeholder", "Input placeholder", "Optional chat input placeholder.", false),
			stringParameter("theme_color", "Theme color", "Optional theme color: default, blue, emerald, violet, rose, amber, or slate.", false),
			stringArrayParameter("suggested_questions", "Suggested questions", "Optional full list of suggested questions.", false),
			stringArrayParameter("knowledge_dataset_ids", "Knowledge dataset IDs", "Optional full replacement list of knowledge dataset IDs. Use [] to clear knowledge bindings.", false),
			stringArrayParameter("add_enabled_skill_ids", "Add enabled skill IDs", "Optional skill IDs to add to the current enabled skill list. Prefer this for binding one or more skills without replacing the whole list.", false),
			stringArrayParameter("remove_enabled_skill_ids", "Remove enabled skill IDs", "Optional skill IDs to remove from the current enabled skill list. Prefer this for unbinding specific skills.", false),
			stringArrayParameter("add_knowledge_dataset_ids", "Add knowledge dataset IDs", "Optional knowledge dataset IDs to add to the current binding list.", false),
			stringArrayParameter("remove_knowledge_dataset_ids", "Remove knowledge dataset IDs", "Optional knowledge dataset IDs to remove from the current binding list.", false),
			objectParameter("knowledge_retrieval_config", "Knowledge retrieval config", "Optional replacement knowledge retrieval config object. Omit to preserve it.", false),
			stringParameter("database_bindings", "Database bindings", "Optional JSON array replacing database bindings. Each item supports data_source_id, table_ids, optional writable_table_ids, or id/database_table_ids in data_source_id:table_id form. Prefer copying binding_candidates[].binding from list_agent_database_tables. Use [] to clear.", false),
			stringParameter("add_database_bindings", "Add database bindings", "Optional JSON array of database table bindings to add to the current bindings. Prefer copying binding_candidates[].binding from list_agent_database_tables.", false),
			stringParameter("remove_database_bindings", "Remove database bindings", "Optional JSON array of database table bindings to remove from the current bindings. Prefer copying current database_bindings from get_agent_config.", false),
			stringParameter("workflow_bindings", "Workflow bindings", "Optional JSON array replacing workflow bindings. Each item supports binding_id, label, agent_id, workflow_id, version_strategy, optional version_uuid, and timeout_seconds. Use [] to clear.", false),
			stringParameter("add_workflow_bindings", "Add workflow bindings", "Optional JSON array of workflow bindings to add to the current bindings.", false),
			stringParameter("remove_workflow_bindings", "Remove workflow bindings", "Optional JSON array of workflow bindings to remove from the current bindings.", false),
			objectParameter("display_names", "Display names", "Optional evidence-only display names for governance cards and event summaries. Supports skills, knowledge_bases, database_tables, and workflows.", false),
		},
	), agentsService, workspacePerms, availableModels), managedFiles: managedFiles}
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
		"List user-selectable, Agent-bindable skills for one resolved AGENT asset. The backend filters hidden runtime/router/page-management skills and skills that do not support the Agent caller. Query by the user's capability phrase such as file generation, chart generation, or image generation; bind only returned candidate skill_ids.",
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
		"Replace the complete user-selectable Agent skill list for one resolved AGENT asset. Prefer update_agent_config add_enabled_skill_ids/remove_enabled_skill_ids for specific bind or unbind requests. Use this only for full replacement or clearing all user-selected skills.",
		"sparkles",
		[]tools.ToolParameter{
			stringParameter("agent_id", "Agent ID", "Required Agent ID from page context, list_agents, or governed asset resolution. Do not invent IDs.", true),
			stringArrayParameter("skill_ids", "Skill IDs", "Required full list of enabled user-selectable skill IDs. Use [] to clear all user-selectable skills.", true),
			objectParameter("display_names", "Display names", "Optional evidence-only display names for governance cards and event summaries. Supports skills.", false),
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
		"List tables from one database candidate that can be bound to the resolved Agent. The database must belong to the Agent's workspace. For binding, copy a returned binding_candidates[].binding object into update_agent_config add_database_bindings instead of manually recombining data_source_id and table_id.",
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
		"Replace the Agent's complete knowledge dataset binding list while preserving all other draft config fields. Prefer update_agent_config add_knowledge_dataset_ids/remove_knowledge_dataset_ids for specific bind or unbind requests. Use [] only when clearing all knowledge bindings.",
		"library",
		[]tools.ToolParameter{
			stringParameter("agent_id", "Agent ID", "Required Agent ID from page context, list_agents, or governed asset resolution. Do not invent IDs.", true),
			stringArrayParameter("dataset_ids", "Dataset IDs", "Required full replacement list of knowledge dataset IDs. Use [] to clear knowledge bindings.", true),
			objectParameter("retrieval_config", "Retrieval config", "Optional JSON object replacing knowledge retrieval config. Omit to preserve current retrieval config.", false),
			objectParameter("display_names", "Display names", "Optional evidence-only display names for governance cards and event summaries. Supports knowledge_bases.", false),
		},
	), agentsService, nil, nil)}
}

func newReplaceAgentDatabaseBindingsTool(agentsService interfaces.AgentsService) tools.Tool {
	return &replaceAgentDatabaseBindingsTool{agentToolBase: newAgentToolBase(agentToolEntity(
		ToolReplaceAgentDatabaseBindings,
		"Replace Agent Database Bindings",
		"Replace the Agent's complete database binding list while preserving all other draft config fields. Prefer update_agent_config add_database_bindings/remove_database_bindings for specific bind or unbind requests. Use [] only when clearing all database bindings.",
		"database",
		[]tools.ToolParameter{
			stringParameter("agent_id", "Agent ID", "Required Agent ID from page context, list_agents, or governed asset resolution. Do not invent IDs.", true),
			stringParameter("bindings", "Bindings", "Required JSON array replacing database bindings. Each item supports data_source_id, table_ids, and optional writable_table_ids. Use [] to clear.", true),
			objectParameter("display_names", "Display names", "Optional evidence-only display names for governance cards and event summaries. Supports database_tables.", false),
		},
	), agentsService, nil, nil)}
}

func newReplaceAgentWorkflowBindingsTool(agentsService interfaces.AgentsService) tools.Tool {
	return &replaceAgentWorkflowBindingsTool{agentToolBase: newAgentToolBase(agentToolEntity(
		ToolReplaceAgentWorkflowBindings,
		"Replace Agent Workflow Bindings",
		"Replace the Agent's complete workflow binding list while preserving all other draft config fields. Prefer update_agent_config add_workflow_bindings/remove_workflow_bindings for specific bind or unbind requests. Use [] only when clearing all workflow bindings.",
		"workflow",
		[]tools.ToolParameter{
			stringParameter("agent_id", "Agent ID", "Required Agent ID from page context, list_agents, or governed asset resolution. Do not invent IDs.", true),
			stringParameter("bindings", "Bindings", "Required JSON array replacing workflow bindings. Each item supports binding_id, label, agent_id, workflow_id, version_strategy, optional version_uuid, and timeout_seconds. Use [] to clear.", true),
			objectParameter("display_names", "Display names", "Optional evidence-only display names for governance cards and event summaries. Supports workflows.", false),
		},
	), agentsService, nil, nil)}
}

func newListAvailableModelsTool(availableModels AvailableModelsService) tools.Tool {
	return &listAvailableModelsTool{agentToolBase: newAgentToolBase(agentToolEntity(
		ToolListAvailableModels,
		"List Available Models",
		"List models available to the current user organization for Agent configuration. Use this before changing an Agent model. Choose one returned item and pass that item's provider and model together to update_agent_config; do not infer or mix provider/model pairs. Agent models are returned by default.",
		"brain-circuit",
		[]tools.ToolParameter{
			stringParameter("use_case", "Use case", "Optional use_case filter. Defaults to agent for Agent runtime model replacement.", false),
			stringParameter("provider", "Provider", "Optional provider slug filter, for example openai, deepseek, or anthropic.", false),
			stringParameter("query", "Query", "Optional natural-language or partial model query, for example deepseek flash. Matching models are ranked first and include match evidence.", false),
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
	page := intParam(params, "page", 1, 100000)
	req := dto.GetAgentsListRequest{
		Page:        page,
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
		"source":       "backend_api",
		"page":         page,
		"page_size":    limit,
		"count":        len(agents),
		"total":        resp.Total,
		"has_more":     resp.HasMore,
		"workspace_id": workspaceID,
		"query": map[string]interface{}{
			"page": page, "page_size": limit, "keyword": stringValue(params, "keyword"), "workspace_id": workspaceID, "asset_kind": "AGENT",
		},
		"agents": agents,
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
	agentID := t.resolveAgentID(ctx, scope, params)
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
	agentID := t.resolveAgentID(ctx, scope, params)
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
	agentID := t.resolveAgentID(ctx, scope, params)
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
	if _, ok := payload["agent_name"]; !ok {
		if name := strings.TrimSpace(firstStringFromMap(params, "agent_name", "agentName", "name", "asset_name", "resource_name")); name != "" {
			payload["agent_name"] = name
			agentPayload := mapFromAny(payload["agent"])
			if len(agentPayload) == 0 {
				agentPayload = map[string]interface{}{}
			}
			agentPayload["name"] = name
			agentPayload["agent_name"] = name
			payload["agent"] = agentPayload
		}
	}
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
	agentID := t.resolveAgentID(ctx, scope, params)
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
	params = agentConfigParams(params)
	scope, err := t.scope(userID)
	if err != nil {
		return nil, err
	}
	agentID := t.resolveAgentID(ctx, scope, params)
	if agentID == "" {
		return nil, fmt.Errorf("agent_id is required")
	}
	current, err := t.agentsService.GetAgentDraftRuntimeConfig(t.scopedContext(ctx, scope), agentID, scope.AccountID)
	if err != nil {
		return nil, err
	}
	params, promptMutation, err := t.resolveSystemPromptMutation(ctx, scope, current.WorkspaceID, current.Config.SystemPrompt, params)
	if err != nil {
		return nil, err
	}
	_, hasModel := optionalStringParam(params, "model")
	requestedProvider, hasProvider := optionalStringParam(params, "model_provider")
	if hasModel {
		if !hasProvider {
			return nil, fmt.Errorf("model_provider is required when changing model; call list_available_models and pass the selected model.provider with model.model")
		}
	}
	if hasProvider && !hasModel && !strings.EqualFold(strings.TrimSpace(requestedProvider), strings.TrimSpace(current.Config.ModelProvider)) {
		return nil, fmt.Errorf("model is required when changing model_provider; call list_available_models and pass the selected model.provider with model.model")
	}
	if hasModel || hasProvider {
		if err := t.validateRequestedModelPair(ctx, scope, params); err != nil {
			return nil, err
		}
	}
	if err := t.enrichAgentWorkflowBindingParams(ctx, scope, agentID, params,
		agentWorkflowBindingParamSpec{Key: "workflow_bindings", RequirePersistable: true},
		agentWorkflowBindingParamSpec{Key: "add_workflow_bindings", RequirePersistable: true},
		agentWorkflowBindingParamSpec{Key: "remove_workflow_bindings"},
	); err != nil {
		return nil, err
	}
	t.enrichAgentDatabaseRemovalParams(ctx, scope, agentID, current, params)
	req, requestedFields, err := mergeAgentConfigRequest(current.Config, params)
	if err != nil {
		return nil, err
	}
	if len(requestedFields) == 0 {
		return nil, fmt.Errorf("at least one config field is required")
	}
	changedFields := actualAgentConfigChangedFields(current.Config, req, requestedFields)
	var updated *dto.AgentConfigResponse
	if promptMutation != nil && promptMutation.Request != nil {
		patchService, ok := t.agentsService.(AgentSystemPromptPatchService)
		if !ok {
			return nil, fmt.Errorf("agent system prompt patch service is not configured")
		}
		req.BindingRevision = current.Config.BindingRevision
		patchRequest := *promptMutation.Request
		patchRequest.Config = req
		patchRequest.RequestedFields = append([]string(nil), requestedFields...)
		updated, err = patchService.UpdateAgentConfigWithSystemPromptPatch(t.scopedContext(ctx, scope), agentID, scope.AccountID, patchRequest)
	} else {
		updated, err = t.agentsService.UpdateAgentConfig(t.scopedContext(ctx, scope), agentID, scope.AccountID, req)
	}
	if err != nil {
		if promptMutation != nil && promptMutation.Request != nil {
			var coded stableCodeError
			if errors.As(err, &coded) && strings.TrimSpace(coded.ErrorCode()) != "" {
				return nil, fmt.Errorf("%s: %w", coded.ErrorCode(), err)
			}
		}
		return nil, err
	}
	agent := agentPayloadWithParamFallback(t.agentPayloadForResult(ctx, scope, agentID), params)
	displayNames := t.enrichAgentConfigDisplayNames(ctx, scope, agentID, &current.Config, updated, agentConfigDisplayNamesFromParams(params))
	payload := agentConfigMutationPayload("updated", agentID, current.WorkspaceID, &current.Config, updated, agent, displayNames)
	if updated != nil {
		changedFields = actualAgentConfigChangedFields(current.Config, agentConfigRequestFromResponse(*updated), requestedFields)
		payload["satisfied_fields"] = append([]string(nil), requestedFields...)
	}
	payload["requested_fields"] = append([]string(nil), requestedFields...)
	payload["updated_fields"] = append([]string(nil), changedFields...)
	if promptMutation != nil {
		if len(promptMutation.Source) > 0 {
			payload["system_prompt_source"] = promptMutation.Source
		}
		if len(promptMutation.Patch) > 0 {
			payload["system_prompt_patch"] = promptMutation.Patch
		}
		payload["system_prompt_digest"] = promptMutation.Digest
		payload["system_prompt_chars"] = promptMutation.Characters
		payload["config"] = agentConfigResponseWithoutSystemPrompt(updated)
	}
	mergeAgentConfigBindingFinalStates(payload, requestedFields, updated, displayNames)
	return []tools.ToolInvokeMessage{builtin.CreateJSONMessage(payload)}, nil
}

func (t *updateAgentConfigTool) EnrichGovernanceArguments(ctx context.Context, userID string, params map[string]interface{}) map[string]interface{} {
	enriched := t.agentToolBase.EnrichGovernanceArguments(ctx, userID, params)
	source, hasSource, sourceErr := optionalMapParam(enriched, "system_prompt_source")
	patch, hasPatch, patchErr := optionalMapParam(enriched, "system_prompt_patch")
	if (sourceErr == nil && hasSource) || (patchErr == nil && hasPatch) {
		preview, _ := stringSliceParam(enriched, "changed_fields_preview")
		enriched["changed_fields_preview"] = appendUniqueStrings(preview, "system_prompt")
	}
	if sourceErr == nil && hasSource && strings.EqualFold(strings.TrimSpace(stringValue(source, "type")), systemPromptSourceTypeManagedFile) {
		enriched["system_prompt_source"] = t.enrichManagedSystemPromptSourceDigest(ctx, source)
	}
	if patchErr != nil || !hasPatch {
		return enriched
	}
	patchSource, hasPatchSource, err := optionalMapParam(patch, "source")
	if err != nil || !hasPatchSource {
		return enriched
	}
	separator, err := systemPromptPatchSeparator(patch)
	if err != nil {
		return enriched
	}
	patch["separator"] = separator
	patch["separator_characters"] = utf8.RuneCountInString(separator)
	patch["separator_sha256"] = systemPromptDigest(separator)
	switch strings.ToLower(strings.TrimSpace(stringValue(patchSource, "type"))) {
	case systemPromptSourceTypeManagedFile:
		patchSource = t.enrichManagedSystemPromptSourceDigest(ctx, patchSource)
	case systemPromptSourceTypeText:
		text, ok := rawStringParam(patchSource, "text")
		if !ok || !utf8.ValidString(text) {
			return enriched
		}
		digest := sha256.Sum256([]byte(text))
		patchSource["characters"] = utf8.RuneCountInString(text)
		patchSource["sha256"] = fmt.Sprintf("sha256:%x", digest[:])
	default:
		return enriched
	}
	patch["source"] = patchSource
	t.enrichSystemPromptPatchBaseline(ctx, userID, enriched, patch)
	enriched["system_prompt_patch"] = patch
	return enriched
}

func (t *updateAgentConfigTool) enrichSystemPromptPatchBaseline(ctx context.Context, userID string, params map[string]interface{}, patch map[string]interface{}) {
	if strings.TrimSpace(stringValue(patch, "expected_base_sha256")) != "" || t.agentsService == nil {
		return
	}
	scope, err := t.scope(userID)
	if err != nil {
		return
	}
	agentID := requiredAgentID(params)
	if agentID == "" {
		return
	}
	current, err := t.agentsService.GetAgentDraftRuntimeConfig(t.scopedContext(ctx, scope), agentID, scope.AccountID)
	if err != nil || current == nil {
		return
	}
	patch["expected_base_sha256"] = systemPromptDigest(current.Config.SystemPrompt)
	patch["expected_base_characters"] = utf8.RuneCountInString(current.Config.SystemPrompt)
}

func (t *updateAgentConfigTool) enrichManagedSystemPromptSourceDigest(ctx context.Context, source map[string]interface{}) map[string]interface{} {
	source = copyStringAnyMap(source)
	// A frozen invocation may pass through enrichment again during approval
	// continuation. Preserve the original snapshot so the execution-time check
	// can detect a file that changed while approval was pending.
	if expected := strings.TrimSpace(stringValue(source, "expected_sha256")); expected != "" || t.managedFiles == nil {
		return source
	}
	fileID := strings.TrimSpace(stringValue(source, "file_id"))
	if fileID == "" {
		return source
	}
	content, err := t.managedFiles.GetFile(ctx, fileID)
	if err != nil || !utf8.ValidString(content) {
		return source
	}
	digest := sha256.Sum256([]byte(content))
	source["expected_sha256"] = fmt.Sprintf("sha256:%x", digest[:])
	source["expected_characters"] = utf8.RuneCountInString(content)
	return source
}

func (t *updateAgentConfigTool) resolveSystemPromptMutation(ctx context.Context, scope agentScope, agentWorkspaceID string, currentPrompt string, params map[string]interface{}) (map[string]interface{}, *resolvedSystemPromptMutation, error) {
	source, hasSource, err := optionalMapParam(params, "system_prompt_source")
	if err != nil {
		return nil, nil, fmt.Errorf("invalid system_prompt_source: %w", err)
	}
	patch, hasPatch, err := optionalMapParam(params, "system_prompt_patch")
	if err != nil {
		return nil, nil, systemPromptPatchInvalidError("invalid system_prompt_patch: %v", err)
	}
	_, hasPrompt := optionalStringParam(params, "system_prompt")
	mutationCount := 0
	for _, present := range []bool{hasPrompt, hasSource, hasPatch} {
		if present {
			mutationCount++
		}
	}
	if mutationCount > 1 {
		if hasPatch {
			return nil, nil, systemPromptPatchInvalidError("system_prompt, system_prompt_source, and system_prompt_patch are mutually exclusive")
		}
		return nil, nil, fmt.Errorf("system_prompt and system_prompt_source are mutually exclusive")
	}
	if !hasSource && !hasPatch {
		return params, nil, nil
	}
	if hasSource {
		content, sourceEvidence, err := t.resolveManagedSystemPromptContent(ctx, scope, agentWorkspaceID, source, "system_prompt_source")
		if err != nil {
			return nil, nil, err
		}
		content = strings.TrimSpace(content)
		normalized := copyStringAnyMap(params)
		normalized["system_prompt"] = content
		return normalized, &resolvedSystemPromptMutation{
			Source:     sourceEvidence,
			Digest:     systemPromptDigest(content),
			Characters: utf8.RuneCountInString(content),
		}, nil
	}
	operation := strings.ToLower(strings.TrimSpace(stringValue(patch, "operation")))
	if operation != systemPromptPatchOperationAppend && operation != systemPromptPatchOperationUpsertSection {
		return nil, nil, systemPromptPatchInvalidError("system_prompt_patch.operation must be append or upsert_section")
	}
	patchSource, hasPatchSource, err := optionalMapParam(patch, "source")
	if err != nil {
		return nil, nil, systemPromptPatchInvalidError("invalid system_prompt_patch.source: %v", err)
	}
	if !hasPatchSource {
		return nil, nil, systemPromptPatchInvalidError("system_prompt_patch.source is required")
	}
	separator, err := systemPromptPatchSeparator(patch)
	if err != nil {
		return nil, nil, err
	}
	var addition string
	var sourceEvidence map[string]interface{}
	switch strings.ToLower(strings.TrimSpace(stringValue(patchSource, "type"))) {
	case systemPromptSourceTypeManagedFile:
		addition, sourceEvidence, err = t.resolveManagedSystemPromptContent(ctx, scope, agentWorkspaceID, patchSource, "system_prompt_patch.source")
	case systemPromptSourceTypeText:
		addition, sourceEvidence, err = resolveTextSystemPromptPatchSource(patchSource)
	default:
		err = systemPromptPatchInvalidError("system_prompt_patch.source.type must be managed_file or text")
	}
	if err != nil {
		if strings.HasPrefix(err.Error(), agentSystemPromptSourceChangedCode+":") || strings.HasPrefix(err.Error(), agentSystemPromptPatchInvalidCode+":") {
			return nil, nil, err
		}
		return nil, nil, systemPromptPatchInvalidError("%v", err)
	}
	sectionID := ""
	sectionTitle := ""
	if operation == systemPromptPatchOperationUpsertSection {
		sectionID, sectionTitle, err = systemPromptPatchSection(patch)
		if err != nil {
			return nil, nil, err
		}
	}
	result, err := applySystemPromptPatch(currentPrompt, operation, addition, separator, sectionID, sectionTitle)
	if err != nil {
		return nil, nil, err
	}
	expectedBaseDigest := strings.TrimSpace(stringValue(patch, "expected_base_sha256"))
	if expectedBaseDigest == "" {
		expectedBaseDigest = systemPromptDigest(currentPrompt)
	}
	normalized := copyStringAnyMap(params)
	normalized["system_prompt"] = result
	patchEvidence := map[string]interface{}{
		"operation":            operation,
		"source":               sourceEvidence,
		"base_sha256":          expectedBaseDigest,
		"content_characters":   utf8.RuneCountInString(addition),
		"resulting_characters": utf8.RuneCountInString(result),
		"separator_characters": utf8.RuneCountInString(separator),
		"separator_sha256":     systemPromptDigest(separator),
	}
	if operation == systemPromptPatchOperationAppend {
		patchEvidence["appended_characters"] = utf8.RuneCountInString(addition)
	} else {
		patchEvidence["section_characters"] = utf8.RuneCountInString(addition)
	}
	if sectionID != "" {
		patchEvidence["section_id"] = sectionID
	}
	if sectionTitle != "" {
		patchEvidence["section_title"] = sectionTitle
	}
	return normalized, &resolvedSystemPromptMutation{
		Patch:      patchEvidence,
		Digest:     systemPromptDigest(result),
		Characters: utf8.RuneCountInString(result),
		Request: &dto.AgentSystemPromptPatchRequest{
			Operation:          operation,
			AppendContent:      addition,
			Separator:          separator,
			SectionID:          sectionID,
			SectionTitle:       sectionTitle,
			ExpectedBaseSHA256: expectedBaseDigest,
		},
	}, nil
}

func (t *updateAgentConfigTool) resolveManagedSystemPromptContent(ctx context.Context, scope agentScope, agentWorkspaceID string, source map[string]interface{}, fieldName string) (string, map[string]interface{}, error) {
	if !strings.EqualFold(strings.TrimSpace(stringValue(source, "type")), systemPromptSourceTypeManagedFile) {
		return "", nil, fmt.Errorf("%s.type must be managed_file", fieldName)
	}
	fileID := strings.TrimSpace(stringValue(source, "file_id"))
	if fileID == "" {
		return "", nil, fmt.Errorf("%s.file_id is required", fieldName)
	}
	if t.managedFiles == nil {
		return "", nil, fmt.Errorf("managed file service is not configured")
	}
	file, err := t.managedFiles.GetFileByID(ctx, fileID)
	if err != nil || file == nil {
		return "", nil, fmt.Errorf("managed system prompt file is not available")
	}
	if file.IsTemporary {
		return "", nil, fmt.Errorf("managed system prompt file must be saved to file management first")
	}
	organizationID := strings.TrimSpace(file.OrganizationID)
	if organizationID == "" {
		organizationID = strings.TrimSpace(file.TenantID)
	}
	if organizationID == "" || organizationID != scope.OrganizationID {
		return "", nil, fmt.Errorf("managed system prompt file is not accessible")
	}
	fileWorkspaceID := ""
	if file.WorkspaceID != nil {
		fileWorkspaceID = strings.TrimSpace(*file.WorkspaceID)
	}
	if fileWorkspaceID == "" || fileWorkspaceID != strings.TrimSpace(agentWorkspaceID) {
		return "", nil, fmt.Errorf("managed system prompt file must belong to the Agent workspace")
	}
	if t.workspacePerms == nil {
		return "", nil, fmt.Errorf("workspace permission service is not configured")
	}
	allowed, err := t.workspacePerms.CheckWorkspacePermission(ctx, organizationID, fileWorkspaceID, scope.AccountID, workspacemodel.WorkspacePermissionFilePreview)
	if err != nil {
		return "", nil, fmt.Errorf("check managed system prompt file permission: %w", err)
	}
	if !allowed {
		return "", nil, fmt.Errorf("managed system prompt file is not accessible")
	}
	if !managedSystemPromptTextFile(file) {
		return "", nil, fmt.Errorf("managed system prompt file must be a TXT or Markdown file")
	}
	content, err := t.managedFiles.GetFile(ctx, fileID)
	if err != nil {
		return "", nil, fmt.Errorf("managed system prompt file is not ready")
	}
	if !utf8.ValidString(content) {
		return "", nil, fmt.Errorf("managed system prompt file must contain valid UTF-8 text")
	}
	characters := utf8.RuneCountInString(content)
	if characters > maxManagedFileSystemPromptChars {
		return "", nil, fmt.Errorf("managed system prompt file exceeds %d characters", maxManagedFileSystemPromptChars)
	}
	digest := sha256.Sum256([]byte(content))
	actualDigest := fmt.Sprintf("sha256:%x", digest[:])
	if expected := strings.TrimSpace(stringValue(source, "expected_sha256")); expected != "" && expected != actualDigest {
		return "", nil, fmt.Errorf("%s: managed system prompt file changed after approval was requested", agentSystemPromptSourceChangedCode)
	}
	return content, map[string]interface{}{
		"type":       "managed_file",
		"file_id":    file.ID,
		"name":       file.Name,
		"size":       file.Size,
		"characters": characters,
		"sha256":     actualDigest,
	}, nil
}

func resolveTextSystemPromptPatchSource(source map[string]interface{}) (string, map[string]interface{}, error) {
	text, ok := rawStringParam(source, "text")
	if !ok || strings.TrimSpace(text) == "" {
		return "", nil, systemPromptPatchInvalidError("system_prompt_patch.source.text is required")
	}
	if !utf8.ValidString(text) {
		return "", nil, systemPromptPatchInvalidError("system_prompt_patch.source.text must contain valid UTF-8 text")
	}
	characters := utf8.RuneCountInString(text)
	if characters > maxManagedFileSystemPromptChars {
		return "", nil, systemPromptPatchInvalidError("system_prompt_patch.source.text exceeds %d characters", maxManagedFileSystemPromptChars)
	}
	return text, map[string]interface{}{
		"type":       systemPromptSourceTypeText,
		"characters": characters,
		"sha256":     systemPromptDigest(text),
	}, nil
}

func appendSystemPrompt(current string, addition string, separator string) (string, error) {
	if strings.TrimSpace(addition) == "" {
		return "", systemPromptPatchInvalidError("system_prompt_patch append content must not be empty")
	}
	result := addition
	if current != "" {
		result = current + separator + addition
	}
	if characters := utf8.RuneCountInString(result); characters > maxManagedFileSystemPromptChars {
		return "", systemPromptPatchInvalidError("appended system prompt exceeds %d characters", maxManagedFileSystemPromptChars)
	}
	return result, nil
}

func applySystemPromptPatch(current string, operation string, content string, separator string, sectionID string, sectionTitle string) (string, error) {
	switch strings.ToLower(strings.TrimSpace(operation)) {
	case systemPromptPatchOperationAppend:
		return appendSystemPrompt(current, content, separator)
	case systemPromptPatchOperationUpsertSection:
		return upsertSystemPromptSection(current, content, separator, sectionID, sectionTitle)
	default:
		return "", systemPromptPatchInvalidError("system_prompt_patch.operation must be append or upsert_section")
	}
}

func systemPromptPatchSection(patch map[string]interface{}) (string, string, error) {
	sectionID := strings.TrimSpace(stringValue(patch, "section_id"))
	if sectionID == "" {
		return "", "", systemPromptPatchInvalidError("system_prompt_patch.section_id is required for upsert_section")
	}
	if utf8.RuneCountInString(sectionID) > maxSystemPromptPatchSectionIDChars || !validSystemPromptPatchSectionID(sectionID) {
		return "", "", systemPromptPatchInvalidError("system_prompt_patch.section_id must use 1-%d ASCII letters, digits, dots, underscores, or hyphens", maxSystemPromptPatchSectionIDChars)
	}
	sectionTitle, hasTitle := rawStringParam(patch, "section_title")
	if !hasTitle {
		return sectionID, "", nil
	}
	sectionTitle = strings.TrimSpace(sectionTitle)
	if !utf8.ValidString(sectionTitle) || utf8.RuneCountInString(sectionTitle) > maxSystemPromptPatchSectionTitleChars || strings.ContainsAny(sectionTitle, "\r\n") {
		return "", "", systemPromptPatchInvalidError("system_prompt_patch.section_title must be a single valid UTF-8 line no longer than %d characters", maxSystemPromptPatchSectionTitleChars)
	}
	return sectionID, sectionTitle, nil
}

func validSystemPromptPatchSectionID(value string) bool {
	for index := 0; index < len(value); index++ {
		char := value[index]
		if (char >= 'a' && char <= 'z') || (char >= 'A' && char <= 'Z') || (char >= '0' && char <= '9') || char == '.' || char == '_' || char == '-' {
			continue
		}
		return false
	}
	return value != ""
}

func upsertSystemPromptSection(current string, content string, separator string, sectionID string, sectionTitle string) (string, error) {
	if strings.TrimSpace(content) == "" {
		return "", systemPromptPatchInvalidError("system_prompt_patch section content must not be empty")
	}
	startMarker, endMarker := systemPromptSectionMarkers(sectionID)
	if strings.Contains(content, startMarker) || strings.Contains(content, endMarker) {
		return "", systemPromptPatchInvalidError("system_prompt_patch section content contains reserved section markers")
	}
	body := content
	if sectionTitle != "" {
		body = "## " + sectionTitle + "\n\n" + content
	}
	block := startMarker + "\n" + body + "\n" + endMarker
	startCount := strings.Count(current, startMarker)
	endCount := strings.Count(current, endMarker)
	var result string
	switch {
	case startCount == 0 && endCount == 0:
		result = block
		if current != "" {
			result = current + separator + block
		}
	case startCount == 1 && endCount == 1:
		start := strings.Index(current, startMarker)
		endRelative := strings.Index(current[start+len(startMarker):], endMarker)
		if endRelative < 0 {
			return "", systemPromptPatchInvalidError("system_prompt_patch section markers are malformed")
		}
		end := start + len(startMarker) + endRelative + len(endMarker)
		result = current[:start] + block + current[end:]
	default:
		return "", systemPromptPatchInvalidError("system_prompt_patch section markers are duplicated or malformed")
	}
	if characters := utf8.RuneCountInString(result); characters > maxManagedFileSystemPromptChars {
		return "", systemPromptPatchInvalidError("updated system prompt exceeds %d characters", maxManagedFileSystemPromptChars)
	}
	return result, nil
}

func systemPromptSectionMarkers(sectionID string) (string, string) {
	prefix := "<!-- zgi:system-prompt-section:" + sectionID
	return prefix + ":start -->", prefix + ":end -->"
}

func systemPromptPatchSeparator(patch map[string]interface{}) (string, error) {
	value, exists := patch["separator"]
	if !exists {
		return defaultSystemPromptPatchSeparator, nil
	}
	separator, ok := value.(string)
	if !ok {
		return "", systemPromptPatchInvalidError("system_prompt_patch.separator must be a string")
	}
	if !utf8.ValidString(separator) {
		return "", systemPromptPatchInvalidError("system_prompt_patch.separator must contain valid UTF-8 text")
	}
	if characters := utf8.RuneCountInString(separator); characters > maxSystemPromptPatchSeparatorChars {
		return "", systemPromptPatchInvalidError("system_prompt_patch.separator exceeds %d characters", maxSystemPromptPatchSeparatorChars)
	}
	return separator, nil
}

func rawStringParam(params map[string]interface{}, key string) (string, bool) {
	if params == nil {
		return "", false
	}
	value, ok := params[key]
	if !ok || value == nil {
		return "", false
	}
	text, ok := value.(string)
	return text, ok
}

func systemPromptPatchInvalidError(format string, args ...interface{}) error {
	return fmt.Errorf("%s: %s", agentSystemPromptPatchInvalidCode, fmt.Sprintf(format, args...))
}

func systemPromptDigest(content string) string {
	digest := sha256.Sum256([]byte(content))
	return fmt.Sprintf("sha256:%x", digest[:])
}

func managedSystemPromptTextFile(file *dto.UploadFile) bool {
	if file == nil {
		return false
	}
	extension := strings.ToLower(strings.TrimPrefix(strings.TrimSpace(file.Extension), "."))
	if extension == "txt" || extension == "md" || extension == "markdown" {
		return true
	}
	mimeType := strings.ToLower(strings.TrimSpace(strings.Split(file.MimeType, ";")[0]))
	return mimeType == "text/plain" || mimeType == "text/markdown"
}

func agentConfigResponseWithoutSystemPrompt(config *dto.AgentConfigResponse) interface{} {
	if config == nil {
		return nil
	}
	raw, err := json.Marshal(config)
	if err != nil {
		return map[string]interface{}{}
	}
	out := map[string]interface{}{}
	if err := json.Unmarshal(raw, &out); err != nil {
		return map[string]interface{}{}
	}
	delete(out, "system_prompt")
	return out
}

func (t *updateAgentConfigTool) validateRequestedModelPair(ctx context.Context, scope agentScope, params map[string]interface{}) error {
	if t.availableModels == nil {
		return nil
	}
	provider, hasProvider := optionalStringParam(params, "model_provider")
	model, hasModel := optionalStringParam(params, "model")
	if !hasProvider && !hasModel {
		return nil
	}
	if strings.TrimSpace(provider) == "" || strings.TrimSpace(model) == "" {
		return fmt.Errorf("model_provider and model are required together when changing agent model")
	}
	organizationID, err := uuid.Parse(scope.OrganizationID)
	if err != nil {
		return fmt.Errorf("invalid organization_id: %w", err)
	}
	models, err := t.availableModels.ListAvailable(ctx, organizationID, provider, llmmodelservice.AgentRuntimeUseCase)
	if err != nil {
		return fmt.Errorf("list available models for agent model validation: %w", err)
	}
	for _, item := range models {
		if item == nil {
			continue
		}
		if strings.EqualFold(strings.TrimSpace(item.Provider), provider) && strings.TrimSpace(item.Name) == model {
			return nil
		}
	}
	return fmt.Errorf("model %q is not available for provider %q; call list_available_models and pass a returned provider/model pair", model, provider)
}

func availableAgentModelMatchesHint(model *llmmodelservice.AvailableModel, hint string) bool {
	if model == nil {
		return false
	}
	tokens := agentModelHintTokens(hint)
	if len(tokens) == 0 {
		return false
	}
	candidate := compactAgentModelText(strings.Join([]string{
		model.Provider,
		model.Name,
		model.DisplayName,
	}, " "))
	if candidate == "" {
		return false
	}
	for _, token := range tokens {
		if !strings.Contains(candidate, token) {
			return false
		}
	}
	return true
}

func agentModelHintTokens(hint string) []string {
	normalized := strings.ToLower(strings.TrimSpace(hint))
	if normalized == "" {
		return nil
	}
	tokens := []string{}
	seen := map[string]struct{}{}
	for _, raw := range strings.FieldsFunc(normalized, func(r rune) bool {
		return !unicode.IsLetter(r) && !unicode.IsDigit(r)
	}) {
		token := compactAgentModelText(raw)
		if token == "" || agentModelHintTokenIsGeneric(token) {
			continue
		}
		if _, ok := seen[token]; ok {
			continue
		}
		seen[token] = struct{}{}
		tokens = append(tokens, token)
	}
	if compact := compactAgentModelText(normalized); compact != "" && len(tokens) == 0 {
		tokens = append(tokens, compact)
	}
	return tokens
}

func compactAgentModelText(value string) string {
	var b strings.Builder
	for _, r := range strings.ToLower(value) {
		if unicode.IsLetter(r) || unicode.IsDigit(r) {
			b.WriteRune(r)
		}
	}
	return b.String()
}

func agentModelHintTokenIsGeneric(token string) bool {
	switch token {
	case "model", "llm", "ai", "agent", "chat", "text", "use", "using", "to", "as", "is", "the":
		return true
	default:
		return false
	}
}

func (t *replaceAgentMemorySlotsTool) Invoke(ctx context.Context, userID string, params map[string]interface{}, conversationID *string, appID *string, messageID *string) ([]tools.ToolInvokeMessage, error) {
	_ = conversationID
	_ = appID
	_ = messageID
	scope, err := t.scope(userID)
	if err != nil {
		return nil, err
	}
	agentID := t.resolveAgentID(ctx, scope, params)
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
	agentID := t.resolveAgentID(ctx, scope, params)
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
	agentID := t.resolveAgentID(ctx, scope, params)
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
	agentID := t.resolveAgentID(ctx, scope, params)
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
	agentID := t.resolveAgentID(ctx, scope, params)
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
		"status":             "completed",
		"agent_id":           resp.AgentID,
		"workspace_id":       resp.WorkspaceID,
		"data_source_id":     resp.DataSourceID,
		"query":              resp.Query,
		"count":              resp.Count,
		"include_columns":    resp.IncludeColumns,
		"include_selected":   resp.IncludeSelected,
		"tables":             resp.Data,
		"binding_candidates": agentDatabaseTableBindingCandidates(resp.Data),
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
	agentID := t.resolveAgentID(ctx, scope, params)
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
	agentID := t.resolveAgentID(ctx, scope, params)
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
	agent := agentPayloadWithParamFallback(t.agentPayloadForResult(ctx, scope, agentID), params)
	displayNames := t.enrichAgentConfigDisplayNames(ctx, scope, agentID, &current.Config, updated, agentConfigDisplayNamesFromParams(params))
	payload := agentConfigMutationPayload("updated", agentID, current.WorkspaceID, &current.Config, updated, agent, displayNames)
	mergeAgentConfigBindingFinalStates(payload, []string{"enabled_skill_ids"}, updated, displayNames)
	return []tools.ToolInvokeMessage{builtin.CreateJSONMessage(payload)}, nil
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
	agentID := t.resolveAgentID(ctx, scope, params)
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
	agent := agentPayloadWithParamFallback(t.agentPayloadForResult(ctx, scope, agentID), params)
	displayNames := t.enrichAgentConfigDisplayNames(ctx, scope, agentID, &current.Config, updated, agentConfigDisplayNamesFromParams(params))
	payload := agentConfigMutationPayload("updated", agentID, current.WorkspaceID, &current.Config, updated, agent, displayNames)
	mergeAgentConfigBindingFinalStates(payload, []string{"knowledge_dataset_ids"}, updated, displayNames)
	return []tools.ToolInvokeMessage{builtin.CreateJSONMessage(payload)}, nil
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
	agentID := t.resolveAgentID(ctx, scope, params)
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
	agent := agentPayloadWithParamFallback(t.agentPayloadForResult(ctx, scope, agentID), params)
	displayNames := t.enrichAgentConfigDisplayNames(ctx, scope, agentID, &current.Config, updated, agentConfigDisplayNamesFromParams(params))
	payload := agentConfigMutationPayload("updated", agentID, current.WorkspaceID, &current.Config, updated, agent, displayNames)
	mergeAgentConfigBindingFinalStates(payload, []string{"database_bindings"}, updated, displayNames)
	return []tools.ToolInvokeMessage{builtin.CreateJSONMessage(payload)}, nil
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
	agentID := t.resolveAgentID(ctx, scope, params)
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
	params["bindings"] = bindings
	if err := t.enrichAgentWorkflowBindingParams(ctx, scope, agentID, params,
		agentWorkflowBindingParamSpec{Key: "bindings", RequirePersistable: true},
	); err != nil {
		return nil, err
	}
	bindings, _, err = agentWorkflowBindingsParam(params, "bindings")
	if err != nil {
		return nil, err
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
	agent := agentPayloadWithParamFallback(t.agentPayloadForResult(ctx, scope, agentID), params)
	displayNames := t.enrichAgentConfigDisplayNames(ctx, scope, agentID, &current.Config, updated, agentConfigDisplayNamesFromParams(params))
	payload := agentConfigMutationPayload("updated", agentID, current.WorkspaceID, &current.Config, updated, agent, displayNames)
	mergeAgentConfigBindingFinalStates(payload, []string{"workflow_bindings"}, updated, displayNames)
	return []tools.ToolInvokeMessage{builtin.CreateJSONMessage(payload)}, nil
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
	query := strings.TrimSpace(firstNonEmptyString(
		stringValue(params, "query"),
		stringValue(params, "model_query"),
	))
	models = rankAgentModelsForQuery(models, query)
	matchedCount := countAgentModelMatches(models, query)
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
		items = append(items, availableAgentModelPayloadWithQuery(model, query))
	}
	payload := map[string]interface{}{
		"status":          "completed",
		"use_case":        useCase,
		"provider":        provider,
		"query":           query,
		"matched_count":   matchedCount,
		"count":           len(items),
		"total":           total,
		"truncated":       truncated,
		"models":          items,
		"valid_use_cases": validAgentModelUseCases(),
		"usage_hint":      "Choose the best matching returned model from this list. Use model.provider as update_agent_config model_provider and model.model as update_agent_config model; do not guess provider/model pairs.",
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

func (t agentToolBase) enrichAgentConfigDisplayNames(ctx context.Context, scope agentScope, agentID string, before *dto.AgentConfigResponse, after *dto.AgentConfigResponse, displayNames agentConfigDisplayNames) agentConfigDisplayNames {
	if t.agentsService == nil || before == nil || after == nil {
		return displayNames
	}
	displayNames = t.enrichAgentConfigSkillDisplayNames(ctx, scope, agentID, before, after, displayNames)
	displayNames = t.enrichAgentConfigKnowledgeDisplayNames(ctx, scope, agentID, before, after, displayNames)
	displayNames = t.enrichAgentConfigDatabaseDisplayNames(ctx, scope, agentID, before, after, displayNames)
	displayNames = t.enrichAgentConfigWorkflowDisplayNames(ctx, scope, agentID, before, after, displayNames)
	return displayNames
}

func (t agentToolBase) enrichAgentConfigSkillDisplayNames(ctx context.Context, scope agentScope, agentID string, before *dto.AgentConfigResponse, after *dto.AgentConfigResponse, displayNames agentConfigDisplayNames) agentConfigDisplayNames {
	neededSkillIDs := changedStringSetKeys(before.EnabledSkillIDs, after.EnabledSkillIDs, displayNames.Skills)
	if len(neededSkillIDs) == 0 {
		return displayNames
	}
	resp, err := t.agentsService.ListAgentSkillCandidates(t.scopedContext(ctx, scope), agentID, scope.AccountID, dto.AgentSkillCandidatesRequest{
		Limit:           maxAgentBindingCandidateListPageSize,
		IncludeSelected: true,
	})
	if err != nil || resp == nil {
		return displayNames
	}
	needed := stringSetFromSlice(neededSkillIDs)
	if displayNames.Skills == nil {
		displayNames.Skills = map[string]string{}
	}
	for _, candidate := range resp.Data {
		skillID := strings.TrimSpace(candidate.SkillID)
		if skillID == "" {
			continue
		}
		if _, ok := needed[skillID]; !ok {
			continue
		}
		if name := agentSkillDisplayName(candidate.SkillID, candidate.Name); name != "" && !strings.EqualFold(name, skillID) {
			displayNames.Skills[skillID] = name
		}
	}
	for _, skillID := range neededSkillIDs {
		if name := strings.TrimSpace(displayNames.Skills[skillID]); name != "" && !strings.EqualFold(name, skillID) {
			continue
		}
		if name := agentSkillDisplayName(skillID, ""); name != "" && !strings.EqualFold(name, skillID) {
			displayNames.Skills[skillID] = name
		}
	}
	return displayNames
}

func agentSkillDisplayName(skillID string, candidates ...string) string {
	skillID = strings.TrimSpace(skillID)
	for _, candidate := range candidates {
		candidate = strings.TrimSpace(candidate)
		if candidate != "" && !strings.EqualFold(candidate, skillID) {
			return candidate
		}
	}
	return titleFromIdentifier(skillID)
}

func titleFromIdentifier(value string) string {
	words := strings.FieldsFunc(strings.TrimSpace(value), func(r rune) bool {
		return r == '-' || r == '_' || unicode.IsSpace(r)
	})
	if len(words) == 0 {
		return ""
	}
	for i, word := range words {
		runes := []rune(strings.ToLower(word))
		if len(runes) == 0 {
			continue
		}
		runes[0] = unicode.ToUpper(runes[0])
		words[i] = string(runes)
	}
	return strings.Join(words, " ")
}

func (t agentToolBase) enrichAgentConfigKnowledgeDisplayNames(ctx context.Context, scope agentScope, agentID string, before *dto.AgentConfigResponse, after *dto.AgentConfigResponse, displayNames agentConfigDisplayNames) agentConfigDisplayNames {
	neededDatasetIDs := changedStringSetKeys(before.KnowledgeDatasetIDs, after.KnowledgeDatasetIDs, displayNames.KnowledgeBases)
	if len(neededDatasetIDs) == 0 {
		return displayNames
	}
	resp, err := t.agentsService.ListAgentKnowledgeCandidates(t.scopedContext(ctx, scope), agentID, scope.AccountID, dto.AgentKnowledgeCandidatesRequest{
		Limit:           maxAgentBindingCandidateListPageSize,
		IncludeSelected: true,
	})
	if err != nil || resp == nil {
		return displayNames
	}
	needed := stringSetFromSlice(neededDatasetIDs)
	if displayNames.KnowledgeBases == nil {
		displayNames.KnowledgeBases = map[string]string{}
	}
	for _, candidate := range resp.Data {
		datasetID := strings.TrimSpace(candidate.DatasetID)
		if datasetID == "" {
			continue
		}
		if _, ok := needed[datasetID]; !ok {
			continue
		}
		if name := strings.TrimSpace(candidate.Name); name != "" && !strings.EqualFold(name, datasetID) {
			displayNames.KnowledgeBases[datasetID] = name
		}
	}
	return displayNames
}

func (t agentToolBase) enrichAgentConfigDatabaseDisplayNames(ctx context.Context, scope agentScope, agentID string, before *dto.AgentConfigResponse, after *dto.AgentConfigResponse, displayNames agentConfigDisplayNames) agentConfigDisplayNames {
	neededByDataSource := changedDatabaseTableKeys(before.DatabaseBindings, after.DatabaseBindings, displayNames.DatabaseTables)
	if len(neededByDataSource) == 0 {
		return displayNames
	}
	if displayNames.DatabaseTables == nil {
		displayNames.DatabaseTables = map[string]string{}
	}
	for dataSourceID, tableIDs := range neededByDataSource {
		for _, tableID := range tableIDs {
			clearSyntheticDatabaseDisplayName(displayNames.DatabaseTables, dataSourceID, tableID)
		}
		resp, err := t.agentsService.ListAgentDatabaseTables(t.scopedContext(ctx, scope), agentID, scope.AccountID, dto.AgentDatabaseTablesRequest{
			DataSourceID:    dataSourceID,
			Limit:           maxAgentBindingCandidateListPageSize,
			IncludeSelected: true,
		})
		if err != nil || resp == nil {
			continue
		}
		needed := stringSetFromSlice(tableIDs)
		for _, candidate := range resp.Data {
			tableID := strings.TrimSpace(candidate.TableID)
			if tableID == "" {
				continue
			}
			if _, ok := needed[tableID]; !ok {
				continue
			}
			if name := strings.TrimSpace(candidate.Name); name != "" && !strings.EqualFold(name, tableID) {
				displayNames.DatabaseTables[dataSourceID+":"+tableID] = name
				displayNames.DatabaseTables[tableID] = name
			}
		}
	}
	return displayNames
}

func (t agentToolBase) enrichAgentConfigWorkflowDisplayNames(ctx context.Context, scope agentScope, agentID string, before *dto.AgentConfigResponse, after *dto.AgentConfigResponse, displayNames agentConfigDisplayNames) agentConfigDisplayNames {
	neededWorkflowKeys := changedWorkflowKeys(before.WorkflowBindings, after.WorkflowBindings, displayNames.Workflows)
	if len(neededWorkflowKeys) == 0 {
		return displayNames
	}
	resp, err := t.agentsService.ListAgentWorkflowBindingCandidates(t.scopedContext(ctx, scope), agentID, scope.AccountID, dto.AgentWorkflowBindingCandidatesRequest{
		Limit:              maxAgentBindingCandidateListPageSize,
		IncludeStartInputs: false,
		IncludeSelected:    true,
	})
	if err != nil || resp == nil {
		return displayNames
	}
	needed := stringSetFromSlice(neededWorkflowKeys)
	if displayNames.Workflows == nil {
		displayNames.Workflows = map[string]string{}
	}
	for _, candidate := range resp.Data {
		name := strings.TrimSpace(candidate.Label)
		if name == "" {
			continue
		}
		for _, key := range []string{candidate.BindingID, candidate.WorkflowID, candidate.AgentID} {
			key = strings.TrimSpace(key)
			if key == "" {
				continue
			}
			if _, ok := needed[key]; !ok {
				continue
			}
			if !strings.EqualFold(name, key) {
				displayNames.Workflows[key] = name
			}
		}
	}
	return displayNames
}

func changedStringSetKeys(before []string, after []string, displayNames map[string]string) []string {
	beforeSet := stringSetFromSlice(before)
	afterSet := stringSetFromSlice(after)
	if stringSetsEqual(beforeSet, afterSet) {
		return nil
	}
	return missingDisplayNameKeys(appendUniqueStrings(sortedSetKeys(beforeSet), sortedSetKeys(afterSet)...), displayNames)
}

func changedDatabaseTableKeys(before []dto.AgentDatabaseBinding, after []dto.AgentDatabaseBinding, displayNames map[string]string) map[string][]string {
	beforeSet, _ := databaseBindingResourceSet(before, nil)
	afterSet, _ := databaseBindingResourceSet(after, nil)
	if stringSetsEqual(beforeSet, afterSet) {
		return nil
	}
	keys := missingDatabaseDisplayNameKeys(appendUniqueStrings(sortedSetKeys(beforeSet), sortedSetKeys(afterSet)...), displayNames)
	if len(keys) == 0 {
		return nil
	}
	out := map[string][]string{}
	for _, key := range keys {
		dataSourceID, tableID, ok := strings.Cut(key, ":")
		if !ok {
			continue
		}
		dataSourceID = strings.TrimSpace(dataSourceID)
		tableID = strings.TrimSpace(tableID)
		if dataSourceID == "" || tableID == "" {
			continue
		}
		out[dataSourceID] = appendUniqueStrings(out[dataSourceID], tableID)
	}
	return out
}

func missingDatabaseDisplayNameKeys(keys []string, displayNames map[string]string) []string {
	out := make([]string, 0, len(keys))
	for _, key := range keys {
		key = strings.TrimSpace(key)
		if key == "" {
			continue
		}
		_, tableID, ok := strings.Cut(key, ":")
		if !ok {
			continue
		}
		name := displayNameForResource(displayNames, key, tableID)
		if name != "" && !syntheticAgentConfigDisplayName(name) {
			continue
		}
		out = append(out, key)
	}
	return out
}

func clearSyntheticDatabaseDisplayName(displayNames map[string]string, dataSourceID string, tableID string) {
	if displayNames == nil {
		return
	}
	for _, key := range []string{strings.TrimSpace(dataSourceID) + ":" + strings.TrimSpace(tableID), strings.TrimSpace(tableID)} {
		if key == "" || key == ":" {
			continue
		}
		if syntheticAgentConfigDisplayName(displayNames[key]) {
			delete(displayNames, key)
		}
	}
}

func syntheticAgentConfigDisplayName(name string) bool {
	normalized := strings.Join(strings.Fields(strings.ToLower(strings.TrimSpace(name))), " ")
	if normalized == "" {
		return false
	}
	prefixes := []string{
		"database binding",
		"database table binding",
		"database table",
		"data table binding",
		"workflow binding",
		"knowledge binding",
		"skill binding",
		"agent binding",
		"binding",
	}
	for _, prefix := range prefixes {
		if strings.HasPrefix(normalized, prefix+" ") && syntheticDisplayNameOrdinalSuffix(strings.TrimSpace(strings.TrimPrefix(normalized, prefix))) {
			return true
		}
	}

	compact := strings.ReplaceAll(normalized, " ", "")
	for _, prefix := range []string{"数据库绑定", "数据表绑定", "数据库表", "工作流绑定", "知识库绑定", "技能绑定", "智能体绑定", "绑定"} {
		if strings.HasPrefix(compact, prefix) && syntheticDisplayNameOrdinalSuffix(strings.TrimPrefix(compact, prefix)) {
			return true
		}
	}
	return false
}

func syntheticDisplayNameOrdinalSuffix(value string) bool {
	value = strings.TrimSpace(strings.TrimPrefix(value, "#"))
	if value == "" {
		return false
	}
	for _, r := range value {
		if !unicode.IsDigit(r) {
			return false
		}
	}
	return true
}

func changedWorkflowKeys(before []dto.AgentWorkflowBinding, after []dto.AgentWorkflowBinding, displayNames map[string]string) []string {
	beforeSet, _ := workflowBindingResourceSet(before, nil)
	afterSet, _ := workflowBindingResourceSet(after, nil)
	if stringSetsEqual(beforeSet, afterSet) {
		return nil
	}
	return missingDisplayNameKeys(appendUniqueStrings(sortedSetKeys(beforeSet), sortedSetKeys(afterSet)...), displayNames)
}

func missingDisplayNameKeys(keys []string, displayNames map[string]string) []string {
	out := make([]string, 0, len(keys))
	for _, key := range keys {
		key = strings.TrimSpace(key)
		if key == "" {
			continue
		}
		if displayNameForResource(displayNames, key) != "" {
			continue
		}
		out = append(out, key)
	}
	return out
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
	if latest := recentAgentPayloadForResult(t.Runtime(), agentID); len(latest) > 0 {
		if len(payload) == 0 {
			payload = latest
		} else {
			overlayAgentPayloadFields(payload, latest)
		}
	}
	if fallback := visibleAgentPayloadForResult(t.Runtime(), agentID); len(fallback) > 0 {
		if len(payload) == 0 {
			payload = fallback
		} else {
			fillMissingAgentPayloadFields(payload, fallback)
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

func (t agentToolBase) resolveAgentIDParam(params map[string]interface{}) string {
	agentID := requiredAgentID(params)
	if agentID == "" {
		return ""
	}
	if visible := visibleAgentPayloadForIdentifier(t.Runtime(), agentID); len(visible) > 0 {
		if resolvedID := strings.TrimSpace(firstStringFromMap(visible, "id", "agent_id")); resolvedID != "" {
			return resolvedID
		}
	}
	return agentID
}

func (t agentToolBase) resolveAgentID(ctx context.Context, scope agentScope, params map[string]interface{}) string {
	agentID := t.resolveAgentIDParam(params)
	if agentID == "" {
		agentID = t.resolveCurrentAgentIDFromRuntime()
	}
	if agentID == "" {
		return ""
	}
	if _, err := uuid.Parse(agentID); err == nil {
		return agentID
	}
	if resolvedID := t.resolveAgentIDByUniqueWorkspaceName(ctx, scope, agentID); resolvedID != "" {
		return resolvedID
	}
	return agentID
}

func (t agentToolBase) resolveCurrentAgentIDFromRuntime() string {
	runtime := t.Runtime()
	if runtime == nil || len(runtime.RuntimeParameters) == 0 {
		return ""
	}
	if explicit := strings.TrimSpace(firstStringFromMap(
		runtime.RuntimeParameters,
		"current_agent_id",
		"console_current_agent_id",
		"console_agent_id",
	)); explicit != "" {
		return explicit
	}
	for _, key := range []string{
		"console_current_route",
		"console_agents_current_route",
		"current_page",
		"runtime_route",
		"route",
	} {
		if agentID := agentIDFromConsoleAgentRoute(stringValue(runtime.RuntimeParameters, key)); agentID != "" {
			return agentID
		}
	}
	visible := visibleAgentsFromRuntime(runtime)
	selectedID := ""
	selectedCount := 0
	for _, agent := range visible {
		selected, _ := boolParam(agent, "selected")
		if !selected {
			continue
		}
		if id := strings.TrimSpace(firstStringFromMap(agent, "agent_id", "id")); id != "" {
			selectedID = id
			selectedCount++
		}
	}
	if selectedCount == 1 {
		return selectedID
	}
	return ""
}

func agentIDFromConsoleAgentRoute(route string) string {
	route = strings.TrimSpace(route)
	if route == "" {
		return ""
	}
	const prefix = "/console/agents/"
	idx := strings.Index(route, prefix)
	if idx < 0 {
		return ""
	}
	rest := strings.TrimPrefix(route[idx:], prefix)
	if rest == "" {
		return ""
	}
	agentID, _, _ := strings.Cut(rest, "/")
	agentID = strings.TrimSpace(agentID)
	if agentID == "" || agentID == "new" {
		return ""
	}
	return agentID
}

func (t agentToolBase) resolveAgentIDByUniqueWorkspaceName(ctx context.Context, scope agentScope, name string) string {
	name = strings.TrimSpace(name)
	if name == "" || t.agentsService == nil {
		return ""
	}
	workspaceID := strings.TrimSpace(scope.WorkspaceID)
	if workspaceID == "" {
		return ""
	}
	resp, err := t.agentsService.GetAgentsListWithPermissions(t.scopedContext(ctx, scope), scope.AccountID, dto.GetAgentsListRequest{
		Page:        1,
		Limit:       defaultAgentListPageSize,
		PageSize:    defaultAgentListPageSize,
		WorkspaceID: workspaceID,
		Keyword:     name,
	})
	if err != nil || resp == nil {
		return ""
	}
	resolvedID := ""
	matchCount := 0
	for _, item := range resp.Data {
		if !strings.EqualFold(strings.TrimSpace(item.Name), name) {
			continue
		}
		id := strings.TrimSpace(item.ID)
		if id == "" {
			continue
		}
		matchCount++
		resolvedID = id
	}
	if matchCount == 1 {
		return resolvedID
	}
	if resp.Total == 1 && len(resp.Data) == 1 {
		return strings.TrimSpace(resp.Data[0].ID)
	}
	return ""
}

func (t agentToolBase) EnrichGovernanceArguments(ctx context.Context, userID string, params map[string]interface{}) map[string]interface{} {
	enriched := agentConfigParams(params)
	normalizeAgentConfigGovernanceParams(t.GetEntity().Identity.Name, enriched)
	scope, err := t.scope(userID)
	if err != nil {
		return enriched
	}
	t.enrichSingleAgentGovernanceArguments(ctx, scope, enriched)
	t.enrichAgentConfigGovernanceChangedFieldsPreview(ctx, scope, enriched)
	t.enrichAgentConfigGovernanceDisplayNames(ctx, scope, enriched)
	t.enrichBatchAgentGovernanceArguments(ctx, scope, enriched)
	return enriched
}

func normalizeAgentConfigGovernanceParams(toolName string, params map[string]interface{}) {
	if len(params) == 0 {
		return
	}
	setIfMissing := func(target string, source string) {
		if _, exists := params[target]; exists {
			return
		}
		if value, ok := params[source]; ok {
			params[target] = value
		}
	}
	switch strings.TrimSpace(toolName) {
	case ToolReplaceAgentSkillBindings:
		setIfMissing("enabled_skill_ids", "skill_ids")
	case ToolReplaceAgentKnowledgeBindings:
		setIfMissing("knowledge_dataset_ids", "dataset_ids")
	case ToolReplaceAgentDatabaseBindings:
		setIfMissing("database_bindings", "bindings")
	case ToolReplaceAgentWorkflowBindings:
		setIfMissing("workflow_bindings", "bindings")
	}
}

func (t agentToolBase) enrichAgentConfigGovernanceChangedFieldsPreview(ctx context.Context, scope agentScope, params map[string]interface{}) {
	if t.agentsService == nil {
		return
	}
	agentID := requiredAgentID(params)
	if agentID == "" {
		return
	}
	current, err := t.agentsService.GetAgentDraftRuntimeConfig(t.scopedContext(ctx, scope), agentID, scope.AccountID)
	if err != nil || current == nil {
		return
	}
	t.enrichAgentDatabaseRemovalParams(ctx, scope, agentID, current, params)
	req, requestedFields, err := mergeAgentConfigRequest(current.Config, params)
	if err != nil || len(requestedFields) == 0 {
		return
	}
	params["changed_fields_preview"] = actualAgentConfigChangedFields(current.Config, req, requestedFields)
}

func (t agentToolBase) enrichAgentConfigGovernanceDisplayNames(ctx context.Context, scope agentScope, params map[string]interface{}) {
	if t.agentsService == nil || !hasAgentConfigBindingParam(params) {
		return
	}
	agentID := requiredAgentID(params)
	if agentID == "" {
		return
	}
	current, err := t.agentsService.GetAgentDraftRuntimeConfig(t.scopedContext(ctx, scope), agentID, scope.AccountID)
	if err != nil || current == nil {
		return
	}
	t.enrichAgentDatabaseRemovalParams(ctx, scope, agentID, current, params)
	req, changedFields, err := mergeAgentConfigRequest(current.Config, params)
	if err != nil || len(changedFields) == 0 {
		return
	}
	if !agentConfigChangedFieldsIncludeBinding(changedFields) {
		return
	}
	after := agentConfigResponseFromRequest(current.Config, req)
	displayNames := t.enrichAgentConfigDisplayNames(ctx, scope, agentID, &current.Config, &after, agentConfigDisplayNamesFromParams(params))
	mergeAgentConfigDisplayNamesIntoParams(params, displayNames)
	mergeAgentConfigBindingChangesPreviewIntoParams(params, agentConfigBindingChanges(&current.Config, &after, displayNames))
}

func (t agentToolBase) enrichSingleAgentGovernanceArguments(ctx context.Context, scope agentScope, params map[string]interface{}) {
	agentID := requiredAgentID(params)
	if agentID == "" {
		return
	}
	visibleAgent := visibleAgentPayloadForIdentifier(t.Runtime(), agentID)
	if resolvedID := strings.TrimSpace(firstStringFromMap(visibleAgent, "id", "agent_id")); resolvedID != "" {
		agentID = resolvedID
		params["agent_id"] = resolvedID
	} else {
		if resolvedID := t.resolveAgentIDByUniqueWorkspaceName(ctx, scope, agentID); resolvedID != "" {
			agentID = resolvedID
			params["agent_id"] = resolvedID
		}
	}
	agent := t.agentPayloadForResult(ctx, scope, agentID)
	if len(agent) == 0 {
		agent = visibleAgent
	}
	if name := strings.TrimSpace(firstStringFromMap(agent, "name", "agent_name")); name != "" {
		params["agent_name"] = name
	}
	if workspaceID := strings.TrimSpace(firstStringFromMap(agent, "workspace_id", "workspaceId")); workspaceID != "" {
		if strings.TrimSpace(firstStringFromMap(params, "workspace_id", "workspaceId")) == "" {
			params["workspace_id"] = workspaceID
		}
	}
	syncSingleAgentGovernanceTarget(params, agentID, agent)
}

func (t agentToolBase) enrichBatchAgentGovernanceArguments(ctx context.Context, scope agentScope, params map[string]interface{}) {
	if _, ok := params["agents"]; ok {
		return
	}
	targets, err := agentBatchDeleteTargets(params, t.Runtime())
	if err != nil || len(targets) == 0 {
		return
	}
	agents := make([]map[string]interface{}, 0, len(targets))
	for _, target := range targets {
		agent := t.agentPayloadForResult(ctx, scope, target.ID)
		item := map[string]interface{}{
			"agent_id": target.ID,
			"id":       target.ID,
			"type":     "agent",
		}
		if name := firstNonEmptyString(target.Name, firstStringFromMap(agent, "name", "agent_name")); name != "" {
			item["name"] = name
			item["agent_name"] = name
		}
		if workspaceID := firstNonEmptyString(target.WorkspaceID, firstStringFromMap(agent, "workspace_id", "workspaceId"), scope.WorkspaceID); workspaceID != "" {
			item["workspace_id"] = workspaceID
		}
		agents = append(agents, item)
	}
	params["agents"] = agents
}

func syncSingleAgentGovernanceTarget(params map[string]interface{}, agentID string, agent map[string]interface{}) {
	if len(params) == 0 {
		return
	}
	agentID = strings.TrimSpace(firstNonEmptyString(agentID, firstStringFromMap(params, "agent_id", "agentId", "id")))
	if agentID == "" {
		return
	}
	name := strings.TrimSpace(firstNonEmptyString(
		firstStringFromMap(agent, "name", "agent_name"),
		firstStringFromMap(params, "agent_name", "agentName", "name"),
	))
	workspaceID := strings.TrimSpace(firstNonEmptyString(
		firstStringFromMap(agent, "workspace_id", "workspaceId"),
		firstStringFromMap(params, "workspace_id", "workspaceId"),
	))
	if name == "" && workspaceID == "" {
		return
	}
	for _, key := range []string{"agents", "targets", "assets"} {
		value, ok := params[key]
		if !ok || value == nil {
			continue
		}
		targets, err := agentBatchDeleteTargetsFromAny(value)
		if err != nil || len(targets) == 0 {
			continue
		}
		matched := false
		for _, target := range targets {
			if strings.TrimSpace(target.ID) == agentID {
				matched = true
				break
			}
		}
		if !matched && len(targets) != 1 {
			continue
		}
		records := make([]map[string]interface{}, 0, len(targets))
		for _, target := range targets {
			id := strings.TrimSpace(target.ID)
			if id == "" {
				id = agentID
			}
			item := map[string]interface{}{
				"agent_id": id,
				"id":       id,
				"type":     "agent",
			}
			targetName := strings.TrimSpace(target.Name)
			if id == agentID && name != "" {
				targetName = name
			}
			if targetName != "" {
				item["name"] = targetName
				item["agent_name"] = targetName
			}
			targetWorkspaceID := strings.TrimSpace(target.WorkspaceID)
			if id == agentID && workspaceID != "" {
				targetWorkspaceID = workspaceID
			}
			if targetWorkspaceID != "" {
				item["workspace_id"] = targetWorkspaceID
			}
			records = append(records, item)
		}
		params[key] = records
		return
	}
	if name == "" {
		return
	}
	params["agents"] = []map[string]interface{}{{
		"agent_id":     agentID,
		"id":           agentID,
		"type":         "agent",
		"name":         name,
		"agent_name":   name,
		"workspace_id": workspaceID,
	}}
}

func visibleAgentPayloadForResult(runtime *tools.ToolRuntime, agentID string) map[string]interface{} {
	agentID = strings.TrimSpace(agentID)
	if agentID == "" {
		return nil
	}
	for _, item := range visibleAgentsFromRuntime(runtime) {
		payload := agentPayload(item)
		if firstStringFromMap(payload, "id", "agent_id") == agentID {
			return payload
		}
	}
	return nil
}

func recentAgentPayloadForResult(runtime *tools.ToolRuntime, agentID string) map[string]interface{} {
	agentID = strings.TrimSpace(agentID)
	if agentID == "" {
		return nil
	}
	for _, item := range recentAgentPayloadsFromRuntime(runtime) {
		payload := agentPayload(item)
		if firstStringFromMap(payload, "id", "agent_id") == agentID {
			return payload
		}
	}
	return nil
}

func visibleAgentPayloadForIdentifier(runtime *tools.ToolRuntime, identifier string) map[string]interface{} {
	identifier = strings.TrimSpace(identifier)
	if identifier == "" {
		return nil
	}
	var nameMatch map[string]interface{}
	nameMatches := 0
	for _, item := range recentAgentPayloadsFromRuntime(runtime) {
		payload := agentPayload(item)
		if firstStringFromMap(payload, "id", "agent_id") == identifier {
			return payload
		}
		if strings.EqualFold(strings.TrimSpace(firstStringFromMap(payload, "name", "agent_name")), identifier) {
			nameMatches++
			nameMatch = payload
		}
	}
	for _, item := range visibleAgentsFromRuntime(runtime) {
		payload := agentPayload(item)
		if firstStringFromMap(payload, "id", "agent_id") == identifier {
			return payload
		}
		if strings.EqualFold(strings.TrimSpace(firstStringFromMap(payload, "name", "agent_name")), identifier) {
			nameMatches++
			nameMatch = payload
		}
	}
	if nameMatches == 1 {
		return nameMatch
	}
	return nil
}

func recentAgentPayloadsFromRuntime(runtime *tools.ToolRuntime) []map[string]interface{} {
	if runtime == nil || len(runtime.RuntimeParameters) == 0 {
		return nil
	}
	return mapsFromAny(runtime.RuntimeParameters["console_agents_recent_agent_updates"])
}

func fillMissingAgentPayloadFields(payload map[string]interface{}, fallback map[string]interface{}) {
	for _, key := range []string{
		"name",
		"agent_name",
		"description",
		"workspace_id",
		"icon_url",
		"icon_type",
		"icon",
		"href",
	} {
		if _, ok := payload[key]; ok {
			continue
		}
		if value, ok := fallback[key]; ok {
			payload[key] = value
		}
	}
}

func overlayAgentPayloadFields(payload map[string]interface{}, latest map[string]interface{}) {
	if payload == nil || len(latest) == 0 {
		return
	}
	for _, key := range []string{
		"name",
		"agent_name",
		"description",
		"icon_url",
		"icon_type",
		"icon",
		"href",
		"workspace_id",
	} {
		if value, ok := latest[key]; ok && value != nil && strings.TrimSpace(fmt.Sprint(value)) != "" {
			payload[key] = value
		}
	}
}

func agentPayloadWithParamFallback(agent map[string]interface{}, params map[string]interface{}) map[string]interface{} {
	payload := agentPayload(agent)
	if len(payload) == 0 {
		payload = map[string]interface{}{}
	}
	fallback := map[string]interface{}{}
	if name := strings.TrimSpace(firstStringFromMap(params, "agent_name", "agentName", "name", "asset_name", "resource_name")); name != "" {
		fallback["name"] = name
		fallback["agent_name"] = name
	}
	if workspaceID := strings.TrimSpace(firstStringFromMap(params, "workspace_id", "workspaceId")); workspaceID != "" {
		fallback["workspace_id"] = workspaceID
	}
	if iconType := strings.TrimSpace(firstStringFromMap(params, "icon_type", "iconType")); iconType != "" {
		fallback["icon_type"] = iconType
	}
	if icon := strings.TrimSpace(firstStringFromMap(params, "icon")); icon != "" {
		fallback["icon"] = icon
	}
	if len(fallback) > 0 {
		fillMissingAgentPayloadFields(payload, fallback)
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
	return &updateAgentConfigTool{agentToolBase: t.forkAgentToolBase(runtime), managedFiles: t.managedFiles}
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
	params = agentConfigParams(params)
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
		changedFields = appendAgentConfigChangedField(changedFields, "enabled_skill_ids")
	}
	if value, ok := stringSliceParam(params, "add_enabled_skill_ids"); ok {
		req.EnabledSkillIDs = addStringsToSet(req.EnabledSkillIDs, value)
		changedFields = appendAgentConfigChangedField(changedFields, "enabled_skill_ids")
	}
	if value, ok := stringSliceParam(params, "remove_enabled_skill_ids"); ok {
		req.EnabledSkillIDs = removeStringsFromSet(req.EnabledSkillIDs, value)
		changedFields = appendAgentConfigChangedField(changedFields, "enabled_skill_ids")
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
	if value, ok := optionalStringParam(params, "opening_statement"); ok {
		req.OpeningStatement = value
		changedFields = append(changedFields, "opening_statement")
	}
	if value, ok := optionalStringParam(params, "input_placeholder"); ok {
		req.InputPlaceholder = value
		changedFields = append(changedFields, "input_placeholder")
	}
	if value, ok := optionalStringParam(params, "theme_color"); ok {
		normalized, err := normalizeAgentThemeColor(value)
		if err != nil {
			return dto.AgentConfigRequest{}, nil, err
		}
		value = normalized
		req.ThemeColor = value
		changedFields = append(changedFields, "theme_color")
	}
	if value, ok := stringSliceParam(params, "suggested_questions"); ok {
		req.SuggestedQuestions = value
		changedFields = append(changedFields, "suggested_questions")
	}
	if value, ok := stringSliceParam(params, "knowledge_dataset_ids"); ok {
		req.KnowledgeDatasetIDs = value
		changedFields = appendAgentConfigChangedField(changedFields, "knowledge_dataset_ids")
	}
	if value, ok := stringSliceParam(params, "dataset_ids"); ok {
		req.KnowledgeDatasetIDs = value
		changedFields = appendAgentConfigChangedField(changedFields, "knowledge_dataset_ids")
	}
	if value, ok := stringSliceParam(params, "add_knowledge_dataset_ids"); ok {
		req.KnowledgeDatasetIDs = addStringsToSet(req.KnowledgeDatasetIDs, value)
		changedFields = appendAgentConfigChangedField(changedFields, "knowledge_dataset_ids")
	}
	if value, ok := stringSliceParam(params, "remove_knowledge_dataset_ids"); ok {
		req.KnowledgeDatasetIDs = removeStringsFromSet(req.KnowledgeDatasetIDs, value)
		changedFields = appendAgentConfigChangedField(changedFields, "knowledge_dataset_ids")
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
		changedFields = appendAgentConfigChangedField(changedFields, "database_bindings")
	}
	if bindings, ok, err := agentDatabaseBindingsParam(params, "add_database_bindings"); err != nil {
		return req, nil, err
	} else if ok {
		req.DatabaseBindings = addAgentDatabaseBindings(req.DatabaseBindings, bindings)
		changedFields = appendAgentConfigChangedField(changedFields, "database_bindings")
	}
	if bindings, ok, err := agentDatabaseBindingsParam(params, "remove_database_bindings"); err != nil {
		return req, nil, err
	} else if ok {
		req.DatabaseBindings = removeAgentDatabaseBindings(req.DatabaseBindings, bindings)
		changedFields = appendAgentConfigChangedField(changedFields, "database_bindings")
	}
	if bindings, ok, err := agentWorkflowBindingsParam(params, "workflow_bindings"); err != nil {
		return req, nil, err
	} else if ok {
		req.WorkflowBindings = bindings
		changedFields = appendAgentConfigChangedField(changedFields, "workflow_bindings")
	}
	if bindings, ok, err := agentWorkflowBindingsParam(params, "add_workflow_bindings"); err != nil {
		return req, nil, err
	} else if ok {
		req.WorkflowBindings = addAgentWorkflowBindings(req.WorkflowBindings, bindings)
		changedFields = appendAgentConfigChangedField(changedFields, "workflow_bindings")
	}
	if bindings, ok, err := agentWorkflowBindingsParam(params, "remove_workflow_bindings"); err != nil {
		return req, nil, err
	} else if ok {
		req.WorkflowBindings = removeAgentWorkflowBindings(req.WorkflowBindings, bindings)
		changedFields = appendAgentConfigChangedField(changedFields, "workflow_bindings")
	}
	return req, changedFields, nil
}

func appendAgentConfigChangedField(fields []string, field string) []string {
	field = strings.TrimSpace(field)
	if field == "" {
		return fields
	}
	for _, existing := range fields {
		if existing == field {
			return fields
		}
	}
	return append(fields, field)
}

func addStringsToSet(current []string, additions []string) []string {
	return appendUniqueStrings(current, additions...)
}

func removeStringsFromSet(current []string, removals []string) []string {
	if len(current) == 0 || len(removals) == 0 {
		return append([]string(nil), current...)
	}
	removeSet := stringSetFromSlice(removals)
	out := make([]string, 0, len(current))
	for _, value := range current {
		trimmed := strings.TrimSpace(value)
		if trimmed == "" {
			continue
		}
		if _, remove := removeSet[trimmed]; remove {
			continue
		}
		out = append(out, trimmed)
	}
	return out
}

func addAgentDatabaseBindings(current []dto.AgentDatabaseBinding, additions []dto.AgentDatabaseBinding) []dto.AgentDatabaseBinding {
	out := normalizeAgentDatabaseBindings(current)
	indexByDataSource := make(map[string]int, len(out))
	for idx, binding := range out {
		indexByDataSource[strings.TrimSpace(binding.DataSourceID)] = idx
	}
	for _, addition := range normalizeAgentDatabaseBindings(additions) {
		dataSourceID := strings.TrimSpace(addition.DataSourceID)
		if dataSourceID == "" {
			continue
		}
		if idx, ok := indexByDataSource[dataSourceID]; ok {
			out[idx].TableIDs = appendUniqueStrings(out[idx].TableIDs, addition.TableIDs...)
			out[idx].WritableTableIDs = appendUniqueStrings(out[idx].WritableTableIDs, addition.WritableTableIDs...)
			continue
		}
		indexByDataSource[dataSourceID] = len(out)
		out = append(out, addition)
	}
	return out
}

func removeAgentDatabaseBindings(current []dto.AgentDatabaseBinding, removals []dto.AgentDatabaseBinding) []dto.AgentDatabaseBinding {
	if len(current) == 0 || len(removals) == 0 {
		return normalizeAgentDatabaseBindings(current)
	}
	removeSources := map[string]struct{}{}
	removeTables := map[string]struct{}{}
	for _, removal := range normalizeAgentDatabaseBindings(removals) {
		dataSourceID := strings.TrimSpace(removal.DataSourceID)
		if dataSourceID == "" {
			continue
		}
		if len(removal.TableIDs) == 0 {
			removeSources[dataSourceID] = struct{}{}
			continue
		}
		for _, tableID := range removal.TableIDs {
			removeTables[dataSourceID+":"+strings.TrimSpace(tableID)] = struct{}{}
		}
	}
	out := make([]dto.AgentDatabaseBinding, 0, len(current))
	for _, binding := range normalizeAgentDatabaseBindings(current) {
		dataSourceID := strings.TrimSpace(binding.DataSourceID)
		if _, removeSource := removeSources[dataSourceID]; removeSource {
			continue
		}
		next := dto.AgentDatabaseBinding{
			DataSourceID:     dataSourceID,
			TableIDs:         make([]string, 0, len(binding.TableIDs)),
			WritableTableIDs: make([]string, 0, len(binding.WritableTableIDs)),
		}
		for _, tableID := range binding.TableIDs {
			tableID = strings.TrimSpace(tableID)
			if tableID == "" {
				continue
			}
			if _, remove := removeTables[dataSourceID+":"+tableID]; remove {
				continue
			}
			next.TableIDs = append(next.TableIDs, tableID)
		}
		for _, tableID := range binding.WritableTableIDs {
			tableID = strings.TrimSpace(tableID)
			if tableID == "" {
				continue
			}
			if _, remove := removeTables[dataSourceID+":"+tableID]; remove {
				continue
			}
			next.WritableTableIDs = append(next.WritableTableIDs, tableID)
		}
		if len(next.TableIDs) == 0 {
			continue
		}
		out = append(out, next)
	}
	return out
}

func (t agentToolBase) enrichAgentDatabaseRemovalParams(ctx context.Context, scope agentScope, agentID string, current *dto.AgentDraftRuntimeConfigResponse, params map[string]interface{}) {
	if current == nil || len(params) == 0 {
		return
	}
	removals, ok, err := agentDatabaseBindingsParam(params, "remove_database_bindings")
	if err != nil || !ok || len(removals) == 0 {
		return
	}
	normalized := t.resolveAgentDatabaseRemovalBindings(ctx, scope, agentID, current.Config.DatabaseBindings, removals, agentConfigDisplayNamesFromParams(params).DatabaseTables)
	if len(normalized) == 0 {
		return
	}
	params["remove_database_bindings"] = normalized
}

type agentDatabaseTableRef struct {
	DataSourceID string
	TableID      string
}

func (t agentToolBase) resolveAgentDatabaseRemovalBindings(ctx context.Context, scope agentScope, agentID string, current []dto.AgentDatabaseBinding, removals []dto.AgentDatabaseBinding, displayNames map[string]string) []dto.AgentDatabaseBinding {
	current = normalizeAgentDatabaseBindings(current)
	removals = normalizeAgentDatabaseBindings(removals)
	if len(current) == 0 || len(removals) == 0 {
		return removals
	}
	exact := map[string]agentDatabaseTableRef{}
	byTableID := map[string][]agentDatabaseTableRef{}
	for _, binding := range current {
		dataSourceID := strings.TrimSpace(binding.DataSourceID)
		for _, tableID := range binding.TableIDs {
			tableID = strings.TrimSpace(tableID)
			if dataSourceID == "" || tableID == "" {
				continue
			}
			ref := agentDatabaseTableRef{DataSourceID: dataSourceID, TableID: tableID}
			exact[dataSourceID+":"+tableID] = ref
			byTableID[tableID] = append(byTableID[tableID], ref)
		}
	}
	if displayNames == nil {
		displayNames = map[string]string{}
	}
	displayNames = t.enrichCurrentDatabaseDisplayNames(ctx, scope, agentID, current, displayNames)
	byName := map[string][]agentDatabaseTableRef{}
	for key, ref := range exact {
		if name := databaseRemovalNameKey(displayNameForResource(displayNames, key, ref.TableID)); name != "" {
			byName[name] = append(byName[name], ref)
		}
	}

	collector := map[string][]string{}
	add := func(ref agentDatabaseTableRef) {
		if strings.TrimSpace(ref.DataSourceID) == "" || strings.TrimSpace(ref.TableID) == "" {
			return
		}
		collector[ref.DataSourceID] = appendUniqueStrings(collector[ref.DataSourceID], ref.TableID)
	}
	addOriginal := func(removal dto.AgentDatabaseBinding, tableID string) {
		dataSourceID := strings.TrimSpace(removal.DataSourceID)
		tableID = strings.TrimSpace(tableID)
		if dataSourceID == "" {
			return
		}
		if tableID == "" {
			if _, ok := collector[dataSourceID]; !ok {
				collector[dataSourceID] = nil
			}
			return
		}
		collector[dataSourceID] = appendUniqueStrings(collector[dataSourceID], tableID)
	}

	for _, removal := range removals {
		dataSourceID := strings.TrimSpace(removal.DataSourceID)
		if dataSourceID == "" {
			continue
		}
		if len(removal.TableIDs) == 0 {
			addOriginal(removal, "")
			continue
		}
		for _, tableID := range removal.TableIDs {
			tableID = strings.TrimSpace(tableID)
			if tableID == "" {
				continue
			}
			if ref, ok := exact[dataSourceID+":"+tableID]; ok {
				add(ref)
				continue
			}
			if refs := byTableID[tableID]; len(refs) == 1 {
				add(refs[0])
				continue
			}
			if name := databaseRemovalNameKey(displayNameForResource(displayNames, dataSourceID+":"+tableID, tableID)); name != "" {
				if refs := byName[name]; len(refs) == 1 {
					add(refs[0])
					continue
				}
			}
			addOriginal(removal, tableID)
		}
	}
	return databaseRemovalBindingsFromCollector(collector)
}

func (t agentToolBase) enrichCurrentDatabaseDisplayNames(ctx context.Context, scope agentScope, agentID string, bindings []dto.AgentDatabaseBinding, displayNames map[string]string) map[string]string {
	if t.agentsService == nil || len(bindings) == 0 {
		return displayNames
	}
	if displayNames == nil {
		displayNames = map[string]string{}
	}
	for _, binding := range bindings {
		dataSourceID := strings.TrimSpace(binding.DataSourceID)
		if dataSourceID == "" {
			continue
		}
		missing := false
		for _, tableID := range binding.TableIDs {
			tableID = strings.TrimSpace(tableID)
			if tableID == "" {
				continue
			}
			if displayNameForResource(displayNames, dataSourceID+":"+tableID, tableID) == "" {
				missing = true
				break
			}
		}
		if !missing {
			continue
		}
		resp, err := t.agentsService.ListAgentDatabaseTables(t.scopedContext(ctx, scope), agentID, scope.AccountID, dto.AgentDatabaseTablesRequest{
			DataSourceID:    dataSourceID,
			Limit:           maxAgentBindingCandidateListPageSize,
			IncludeSelected: true,
		})
		if err != nil || resp == nil {
			continue
		}
		for _, candidate := range resp.Data {
			tableID := strings.TrimSpace(candidate.TableID)
			name := strings.TrimSpace(candidate.Name)
			if tableID == "" || name == "" || strings.EqualFold(name, tableID) {
				continue
			}
			displayNames[dataSourceID+":"+tableID] = name
			displayNames[tableID] = name
		}
	}
	return displayNames
}

func databaseRemovalNameKey(name string) string {
	return strings.Join(strings.Fields(strings.ToLower(strings.TrimSpace(name))), " ")
}

func databaseRemovalBindingsFromCollector(values map[string][]string) []dto.AgentDatabaseBinding {
	dataSourceIDs := make([]string, 0, len(values))
	for dataSourceID := range values {
		dataSourceIDs = append(dataSourceIDs, dataSourceID)
	}
	dataSourceIDs = sortedStrings(dataSourceIDs)
	out := make([]dto.AgentDatabaseBinding, 0, len(dataSourceIDs))
	for _, dataSourceID := range dataSourceIDs {
		tables := sortedStrings(append([]string(nil), values[dataSourceID]...))
		out = append(out, dto.AgentDatabaseBinding{
			DataSourceID: dataSourceID,
			TableIDs:     tables,
		})
	}
	return out
}

func normalizeAgentDatabaseBindings(bindings []dto.AgentDatabaseBinding) []dto.AgentDatabaseBinding {
	out := make([]dto.AgentDatabaseBinding, 0, len(bindings))
	for _, binding := range bindings {
		dataSourceID := strings.TrimSpace(binding.DataSourceID)
		if dataSourceID == "" {
			continue
		}
		normalized := dto.AgentDatabaseBinding{
			DataSourceID:     dataSourceID,
			TableIDs:         appendUniqueStrings(nil, binding.TableIDs...),
			WritableTableIDs: appendUniqueStrings(nil, binding.WritableTableIDs...),
		}
		if len(normalized.TableIDs) == 0 && len(normalized.WritableTableIDs) > 0 {
			normalized.TableIDs = appendUniqueStrings(nil, normalized.WritableTableIDs...)
		}
		out = append(out, normalized)
	}
	return out
}

func addAgentWorkflowBindings(current []dto.AgentWorkflowBinding, additions []dto.AgentWorkflowBinding) []dto.AgentWorkflowBinding {
	out := normalizeAgentWorkflowBindings(current)
	indexByKey := make(map[string]int, len(out))
	for idx, binding := range out {
		if key := agentWorkflowBindingKey(binding); key != "" {
			indexByKey[key] = idx
		}
	}
	for _, addition := range normalizeAgentWorkflowBindings(additions) {
		key := agentWorkflowBindingKey(addition)
		if key == "" {
			continue
		}
		if idx, ok := indexByKey[key]; ok {
			out[idx] = addition
			continue
		}
		indexByKey[key] = len(out)
		out = append(out, addition)
	}
	return out
}

func removeAgentWorkflowBindings(current []dto.AgentWorkflowBinding, removals []dto.AgentWorkflowBinding) []dto.AgentWorkflowBinding {
	if len(current) == 0 || len(removals) == 0 {
		return normalizeAgentWorkflowBindings(current)
	}
	removeKeys := map[string]struct{}{}
	for _, removal := range normalizeAgentWorkflowBindings(removals) {
		for _, key := range agentWorkflowBindingKeys(removal) {
			removeKeys[key] = struct{}{}
		}
	}
	out := make([]dto.AgentWorkflowBinding, 0, len(current))
	for _, binding := range normalizeAgentWorkflowBindings(current) {
		remove := false
		for _, key := range agentWorkflowBindingKeys(binding) {
			if _, ok := removeKeys[key]; ok {
				remove = true
				break
			}
		}
		if remove {
			continue
		}
		out = append(out, binding)
	}
	return out
}

func normalizeAgentWorkflowBindings(bindings []dto.AgentWorkflowBinding) []dto.AgentWorkflowBinding {
	out := make([]dto.AgentWorkflowBinding, 0, len(bindings))
	for _, binding := range bindings {
		binding.BindingID = strings.TrimSpace(binding.BindingID)
		binding.WorkflowID = strings.TrimSpace(binding.WorkflowID)
		binding.AgentID = strings.TrimSpace(binding.AgentID)
		if agentWorkflowBindingKey(binding) == "" {
			continue
		}
		out = append(out, binding)
	}
	return out
}

type agentWorkflowBindingParamSpec struct {
	Key                string
	RequirePersistable bool
}

func (t agentToolBase) enrichAgentWorkflowBindingParams(ctx context.Context, scope agentScope, agentID string, params map[string]interface{}, specs ...agentWorkflowBindingParamSpec) error {
	if len(params) == 0 || agentID == "" || len(specs) == 0 {
		return nil
	}
	var candidates map[string]dto.AgentWorkflowBindingCandidate
	var candidateErr error
	for _, spec := range specs {
		key := strings.TrimSpace(spec.Key)
		if key == "" {
			continue
		}
		bindings, ok, err := agentWorkflowBindingsParam(params, key)
		if err != nil {
			return err
		}
		if !ok {
			continue
		}
		if len(bindings) == 0 {
			params[key] = bindings
			continue
		}
		if agentWorkflowBindingsNeedCandidateCompletion(bindings, spec.RequirePersistable) && candidates == nil && candidateErr == nil {
			candidates, candidateErr = t.agentWorkflowBindingCandidatesByKey(ctx, scope, agentID)
		}
		if candidateErr != nil {
			return candidateErr
		}
		completed := make([]dto.AgentWorkflowBinding, 0, len(bindings))
		for _, binding := range bindings {
			binding = completeAgentWorkflowBindingFromCandidates(binding, candidates)
			if spec.RequirePersistable && !agentWorkflowBindingPersistable(binding) {
				return fmt.Errorf("%s contains workflow binding %q without agent_id and workflow_id; call list_agent_workflow_binding_candidates and pass the returned candidate object", key, agentWorkflowBindingKey(binding))
			}
			completed = append(completed, binding)
		}
		params[key] = completed
	}
	return nil
}

func agentWorkflowBindingsNeedCandidateCompletion(bindings []dto.AgentWorkflowBinding, requirePersistable bool) bool {
	if !requirePersistable {
		return false
	}
	for _, binding := range bindings {
		if !agentWorkflowBindingPersistable(binding) {
			return true
		}
	}
	return false
}

func (t agentToolBase) agentWorkflowBindingCandidatesByKey(ctx context.Context, scope agentScope, agentID string) (map[string]dto.AgentWorkflowBindingCandidate, error) {
	resp, err := t.agentsService.ListAgentWorkflowBindingCandidates(t.scopedContext(ctx, scope), agentID, scope.AccountID, dto.AgentWorkflowBindingCandidatesRequest{
		Limit:              maxAgentBindingCandidateListPageSize,
		IncludeStartInputs: false,
		IncludeSelected:    true,
	})
	if err != nil {
		return nil, fmt.Errorf("list agent workflow binding candidates: %w", err)
	}
	out := map[string]dto.AgentWorkflowBindingCandidate{}
	if resp == nil {
		return out, nil
	}
	for _, candidate := range resp.Data {
		for _, key := range []string{candidate.BindingID, candidate.WorkflowID, candidate.AgentID} {
			key = strings.TrimSpace(key)
			if key == "" {
				continue
			}
			out[key] = candidate
		}
	}
	return out, nil
}

func completeAgentWorkflowBindingFromCandidates(binding dto.AgentWorkflowBinding, candidates map[string]dto.AgentWorkflowBindingCandidate) dto.AgentWorkflowBinding {
	for _, key := range agentWorkflowBindingKeys(binding) {
		candidate, ok := candidates[strings.TrimSpace(key)]
		if !ok {
			continue
		}
		return completeAgentWorkflowBindingFromCandidate(binding, candidate)
	}
	return binding
}

func completeAgentWorkflowBindingFromCandidate(binding dto.AgentWorkflowBinding, candidate dto.AgentWorkflowBindingCandidate) dto.AgentWorkflowBinding {
	if strings.TrimSpace(binding.BindingID) == "" {
		binding.BindingID = candidate.BindingID
	}
	if strings.TrimSpace(binding.Label) == "" {
		binding.Label = candidate.Label
	}
	if strings.TrimSpace(binding.Description) == "" {
		binding.Description = candidate.Description
	}
	if strings.TrimSpace(binding.AgentID) == "" {
		binding.AgentID = candidate.AgentID
	}
	if strings.TrimSpace(binding.WorkflowID) == "" {
		binding.WorkflowID = candidate.WorkflowID
	}
	if strings.TrimSpace(binding.AgentType) == "" {
		binding.AgentType = candidate.AgentType
	}
	if strings.TrimSpace(binding.VersionStrategy) == "" {
		binding.VersionStrategy = candidate.VersionStrategy
	}
	if strings.TrimSpace(binding.VersionUUID) == "" {
		binding.VersionUUID = candidate.VersionUUID
	}
	if len(binding.StartInputs) == 0 && len(candidate.StartInputs) > 0 {
		binding.StartInputs = append([]dto.AgentWorkflowStartInput(nil), candidate.StartInputs...)
	}
	if len(binding.RequiredInputs) == 0 && len(candidate.RequiredInputs) > 0 {
		binding.RequiredInputs = append([]string(nil), candidate.RequiredInputs...)
	}
	if strings.TrimSpace(binding.DefaultInputKey) == "" {
		binding.DefaultInputKey = candidate.DefaultInputKey
	}
	return binding
}

func agentWorkflowBindingPersistable(binding dto.AgentWorkflowBinding) bool {
	return strings.TrimSpace(binding.BindingID) != "" &&
		strings.TrimSpace(binding.AgentID) != "" &&
		strings.TrimSpace(binding.WorkflowID) != ""
}

func agentWorkflowBindingKey(binding dto.AgentWorkflowBinding) string {
	return firstNonEmptyString(binding.BindingID, binding.WorkflowID, binding.AgentID)
}

func agentWorkflowBindingKeys(binding dto.AgentWorkflowBinding) []string {
	return appendUniqueStrings(nil, binding.BindingID, binding.WorkflowID, binding.AgentID)
}

func actualAgentConfigChangedFields(before dto.AgentConfigResponse, after dto.AgentConfigRequest, requestedFields []string) []string {
	if len(requestedFields) == 0 {
		return nil
	}
	out := make([]string, 0, len(requestedFields))
	seen := map[string]struct{}{}
	for _, field := range requestedFields {
		field = strings.TrimSpace(field)
		if field == "" {
			continue
		}
		if _, ok := seen[field]; ok {
			continue
		}
		seen[field] = struct{}{}
		if !agentConfigFieldActuallyChanged(before, after, field) {
			continue
		}
		out = append(out, field)
	}
	return out
}

func agentConfigFieldActuallyChanged(before dto.AgentConfigResponse, after dto.AgentConfigRequest, field string) bool {
	switch strings.TrimSpace(field) {
	case "system_prompt":
		return before.SystemPrompt != after.SystemPrompt
	case "model_provider":
		return before.ModelProvider != after.ModelProvider
	case "model":
		return before.Model != after.Model
	case "model_parameters":
		return !reflect.DeepEqual(before.ModelParameters, after.ModelParameters)
	case "enabled_skill_ids":
		return !agentConfigStringSetsEqual(before.EnabledSkillIDs, after.EnabledSkillIDs)
	case "agent_memory_enabled":
		return before.AgentMemoryEnabled != after.AgentMemoryEnabled
	case "file_upload_enabled":
		return before.FileUpload != after.FileUpload
	case "home_title":
		return before.HomeTitle != after.HomeTitle
	case "opening_statement":
		return before.OpeningStatement != after.OpeningStatement
	case "input_placeholder":
		return before.InputPlaceholder != after.InputPlaceholder
	case "theme_color":
		return before.ThemeColor != after.ThemeColor
	case "suggested_questions":
		return !reflect.DeepEqual(before.SuggestedQuestions, after.SuggestedQuestions)
	case "knowledge_dataset_ids":
		return !agentConfigStringSetsEqual(before.KnowledgeDatasetIDs, after.KnowledgeDatasetIDs)
	case "knowledge_retrieval_config":
		return !reflect.DeepEqual(before.KnowledgeRetrievalConfig, after.KnowledgeRetrievalConfig)
	case "database_bindings":
		return !agentConfigDatabaseBindingsEqual(before.DatabaseBindings, after.DatabaseBindings)
	case "workflow_bindings":
		return !agentConfigWorkflowBindingsEqual(before.WorkflowBindings, after.WorkflowBindings)
	default:
		return true
	}
}

func agentConfigStringSetsEqual(before []string, after []string) bool {
	return stringSetsEqual(stringSetFromSlice(before), stringSetFromSlice(after))
}

func agentConfigDatabaseBindingsEqual(before []dto.AgentDatabaseBinding, after []dto.AgentDatabaseBinding) bool {
	beforeSet, _ := databaseBindingResourceSet(before, nil)
	afterSet, _ := databaseBindingResourceSet(after, nil)
	return stringSetsEqual(beforeSet, afterSet)
}

func agentConfigWorkflowBindingsEqual(before []dto.AgentWorkflowBinding, after []dto.AgentWorkflowBinding) bool {
	beforeSet, _ := workflowBindingResourceSet(before, nil)
	afterSet, _ := workflowBindingResourceSet(after, nil)
	return stringSetsEqual(beforeSet, afterSet)
}

func agentConfigParams(params map[string]interface{}) map[string]interface{} {
	normalized := copyStringAnyMap(params)
	config := agentConfigWrapperParam(params)
	if len(config) == 0 {
		return normalized
	}
	for _, key := range agentConfigWrapperKeys() {
		if _, exists := normalized[key]; exists {
			continue
		}
		if value, ok := config[key]; ok {
			normalized[key] = value
		}
	}
	return normalized
}

func agentConfigWrapperParam(params map[string]interface{}) map[string]interface{} {
	if params == nil {
		return nil
	}
	value, ok := params["config"]
	if !ok || value == nil {
		return nil
	}
	if text, ok := value.(string); ok {
		text = strings.TrimSpace(text)
		if text == "" {
			return nil
		}
		out := map[string]interface{}{}
		if err := json.Unmarshal([]byte(text), &out); err != nil {
			return nil
		}
		return out
	}
	return mapFromAny(value)
}

func agentConfigWrapperKeys() []string {
	return []string{
		"agent_id",
		"id",
		"asset_id",
		"system_prompt",
		"system_prompt_source",
		"system_prompt_patch",
		"model_provider",
		"model",
		"model_parameters",
		"enabled_skill_ids",
		"add_enabled_skill_ids",
		"remove_enabled_skill_ids",
		"agent_memory_enabled",
		"file_upload_enabled",
		"home_title",
		"opening_statement",
		"input_placeholder",
		"theme_color",
		"suggested_questions",
		"knowledge_dataset_ids",
		"dataset_ids",
		"add_knowledge_dataset_ids",
		"remove_knowledge_dataset_ids",
		"knowledge_retrieval_config",
		"database_bindings",
		"add_database_bindings",
		"remove_database_bindings",
		"workflow_bindings",
		"add_workflow_bindings",
		"remove_workflow_bindings",
		"display_names",
	}
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
		OpeningStatement:         current.OpeningStatement,
		InputPlaceholder:         current.InputPlaceholder,
		ThemeColor:               current.ThemeColor,
		SuggestedQuestions:       append([]string(nil), current.SuggestedQuestions...),
		KnowledgeDatasetIDs:      append([]string(nil), current.KnowledgeDatasetIDs...),
		KnowledgeRetrievalConfig: copyStringAnyMap(current.KnowledgeRetrievalConfig),
		DatabaseBindings:         append([]dto.AgentDatabaseBinding(nil), current.DatabaseBindings...),
		WorkflowBindings:         append([]dto.AgentWorkflowBinding(nil), current.WorkflowBindings...),
	}
}

func agentConfigResponseFromRequest(current dto.AgentConfigResponse, req dto.AgentConfigRequest) dto.AgentConfigResponse {
	after := current
	after.SystemPrompt = req.SystemPrompt
	after.ModelProvider = req.ModelProvider
	after.Model = req.Model
	after.ModelParameters = copyStringAnyMap(req.ModelParameters)
	after.EnabledSkillIDs = append([]string(nil), req.EnabledSkillIDs...)
	after.AgentMemoryEnabled = req.AgentMemoryEnabled
	after.FileUpload = req.FileUpload
	after.HomeTitle = req.HomeTitle
	after.OpeningStatement = req.OpeningStatement
	after.InputPlaceholder = req.InputPlaceholder
	after.ThemeColor = req.ThemeColor
	after.SuggestedQuestions = append([]string(nil), req.SuggestedQuestions...)
	after.KnowledgeDatasetIDs = append([]string(nil), req.KnowledgeDatasetIDs...)
	after.KnowledgeRetrievalConfig = copyStringAnyMap(req.KnowledgeRetrievalConfig)
	after.DatabaseBindings = append([]dto.AgentDatabaseBinding(nil), req.DatabaseBindings...)
	after.WorkflowBindings = append([]dto.AgentWorkflowBinding(nil), req.WorkflowBindings...)
	return after
}

func hasAgentConfigBindingParam(params map[string]interface{}) bool {
	if len(params) == 0 {
		return false
	}
	for _, key := range []string{
		"enabled_skill_ids",
		"add_enabled_skill_ids",
		"remove_enabled_skill_ids",
		"knowledge_dataset_ids",
		"dataset_ids",
		"add_knowledge_dataset_ids",
		"remove_knowledge_dataset_ids",
		"database_bindings",
		"add_database_bindings",
		"remove_database_bindings",
		"workflow_bindings",
		"add_workflow_bindings",
		"remove_workflow_bindings",
	} {
		if _, ok := params[key]; ok {
			return true
		}
	}
	return false
}

func agentConfigChangedFieldsIncludeBinding(fields []string) bool {
	for _, field := range fields {
		switch field {
		case "enabled_skill_ids", "knowledge_dataset_ids", "database_bindings", "workflow_bindings":
			return true
		}
	}
	return false
}

func mergeAgentConfigDisplayNamesIntoParams(params map[string]interface{}, displayNames agentConfigDisplayNames) {
	if len(params) == 0 {
		return
	}
	nested := mapFromAny(params["display_names"])
	if nested == nil {
		nested = map[string]interface{}{}
	}
	mergeAgentConfigDisplayNameGroup(nested, "skills", displayNames.Skills)
	mergeAgentConfigDisplayNameGroup(nested, "knowledge_bases", displayNames.KnowledgeBases)
	mergeAgentConfigDisplayNameGroup(nested, "database_tables", displayNames.DatabaseTables)
	mergeAgentConfigDisplayNameGroup(nested, "workflows", displayNames.Workflows)
	if len(nested) > 0 {
		params["display_names"] = nested
	}
}

func mergeAgentConfigBindingChangesPreviewIntoParams(params map[string]interface{}, changes []map[string]interface{}) {
	if len(params) == 0 || len(changes) == 0 {
		return
	}
	params["binding_changes_preview"] = changes
	params["config_changes_preview"] = changes
	if primary := primaryAgentConfigBindingChange(changes); len(primary) > 0 {
		params["binding_change_preview"] = primary
	}
}

func mergeAgentConfigDisplayNameGroup(target map[string]interface{}, key string, values map[string]string) {
	if len(values) == 0 {
		return
	}
	group := mapFromAny(target[key])
	if group == nil {
		group = map[string]interface{}{}
	}
	for id, name := range values {
		id = strings.TrimSpace(id)
		name = strings.TrimSpace(name)
		if id == "" || name == "" || strings.EqualFold(id, name) {
			continue
		}
		group[id] = name
	}
	if len(group) > 0 {
		target[key] = group
	}
}

func agentConfigMutationPayload(effect string, agentID string, workspaceID string, before *dto.AgentConfigResponse, config *dto.AgentConfigResponse, agent map[string]interface{}, displayNames agentConfigDisplayNames) map[string]interface{} {
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
		payload["model_provider"] = config.ModelProvider
		payload["model"] = config.Model
		payload["agent_memory_enabled"] = config.AgentMemoryEnabled
		payload["file_upload_enabled"] = config.FileUpload
		payload["home_title"] = config.HomeTitle
		payload["opening_statement"] = config.OpeningStatement
		payload["input_placeholder"] = config.InputPlaceholder
		payload["theme_color"] = config.ThemeColor
		payload["suggested_questions"] = append([]string(nil), config.SuggestedQuestions...)
		payload["suggested_question_count"] = len(config.SuggestedQuestions)
		payload["enabled_skill_ids"] = append([]string(nil), config.EnabledSkillIDs...)
		payload["knowledge_dataset_ids"] = append([]string(nil), config.KnowledgeDatasetIDs...)
		payload["knowledge_retrieval_config"] = copyStringAnyMap(config.KnowledgeRetrievalConfig)
		payload["database_bindings"] = append([]dto.AgentDatabaseBinding(nil), config.DatabaseBindings...)
		payload["workflow_bindings"] = append([]dto.AgentWorkflowBinding(nil), config.WorkflowBindings...)
	}
	if changes := agentConfigBindingChanges(before, config, displayNames); len(changes) > 0 {
		payload["config_changes"] = changes
		payload["binding_changes"] = changes
		if primary := primaryAgentConfigBindingChange(changes); len(primary) > 0 {
			for _, field := range []string{
				"binding_kind",
				"change_action",
				"resource_count",
				"resource_ids",
				"resource_names",
				"added_resource_count",
				"added_resource_ids",
				"added_resource_names",
				"removed_resource_count",
				"removed_resource_ids",
				"removed_resource_names",
				"final_resource_count",
				"final_resource_ids",
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

func mergeAgentConfigBindingFinalStates(payload map[string]interface{}, requestedFields []string, config *dto.AgentConfigResponse, displayNames agentConfigDisplayNames) {
	if len(payload) == 0 || config == nil {
		return
	}
	states := agentConfigBindingFinalStates(requestedFields, config, displayNames)
	if len(states) == 0 {
		return
	}
	payload["binding_final_states"] = states
	if len(states) != 1 {
		return
	}
	state := states[0]
	for _, field := range []string{
		"field",
		"binding_kind",
		"change_action",
		"final_resource_count",
		"final_resource_ids",
		"final_resource_names",
	} {
		if _, exists := payload[field]; exists {
			continue
		}
		if value, ok := state[field]; ok {
			payload[field] = value
		}
	}
}

func agentConfigBindingFinalStates(requestedFields []string, config *dto.AgentConfigResponse, displayNames agentConfigDisplayNames) []map[string]interface{} {
	if config == nil {
		return nil
	}
	seen := map[string]struct{}{}
	states := make([]map[string]interface{}, 0, 4)
	add := func(field string, state map[string]interface{}) {
		field = strings.TrimSpace(field)
		if field == "" || len(state) == 0 {
			return
		}
		if _, ok := seen[field]; ok {
			return
		}
		seen[field] = struct{}{}
		states = append(states, state)
	}
	for _, field := range requestedFields {
		switch strings.TrimSpace(field) {
		case "enabled_skill_ids":
			add("enabled_skill_ids", agentConfigStringBindingFinalState("enabled_skill_ids", "agent_skill", config.EnabledSkillIDs, displayNames.Skills))
		case "knowledge_dataset_ids", "dataset_ids":
			add("knowledge_dataset_ids", agentConfigStringBindingFinalState("knowledge_dataset_ids", "knowledge_base", config.KnowledgeDatasetIDs, displayNames.KnowledgeBases))
		case "database_bindings":
			add("database_bindings", agentConfigDatabaseBindingFinalState(config.DatabaseBindings, displayNames.DatabaseTables))
		case "workflow_bindings":
			add("workflow_bindings", agentConfigWorkflowBindingFinalState(config.WorkflowBindings, displayNames.Workflows))
		}
	}
	return states
}

func agentConfigStringBindingFinalState(field string, kind string, values []string, displayNames map[string]string) map[string]interface{} {
	finalKeys := sortedSetKeys(stringSetFromSlice(values))
	finalNames := make([]string, 0, len(finalKeys))
	for _, key := range finalKeys {
		if name := displayNameForResource(displayNames, key); name != "" {
			finalNames = append(finalNames, name)
		}
	}
	state := agentConfigChangePayload(field, kind, len(finalKeys), finalNames, 0, nil, 0, nil)
	state["change_action"] = "satisfied"
	addAgentConfigChangeResourceIDs(state, finalKeys, nil, nil)
	return state
}

func agentConfigDatabaseBindingFinalState(bindings []dto.AgentDatabaseBinding, displayNames map[string]string) map[string]interface{} {
	finalSet, finalNamesByKey := databaseBindingResourceSet(bindings, displayNames)
	finalKeys := sortedSetKeys(finalSet)
	state := agentConfigChangePayload(
		"database_bindings",
		"database_table",
		len(finalKeys),
		resourceNamesForKeys(finalKeys, finalNamesByKey),
		0,
		nil,
		0,
		nil,
	)
	state["change_action"] = "satisfied"
	addAgentConfigChangeResourceIDs(state, finalKeys, nil, nil)
	return state
}

func agentConfigWorkflowBindingFinalState(bindings []dto.AgentWorkflowBinding, displayNames map[string]string) map[string]interface{} {
	finalSet, finalNamesByKey := workflowBindingResourceSet(bindings, displayNames)
	finalKeys := sortedSetKeys(finalSet)
	state := agentConfigChangePayload(
		"workflow_bindings",
		"workflow",
		len(finalKeys),
		resourceNamesForKeys(finalKeys, finalNamesByKey),
		0,
		nil,
		0,
		nil,
	)
	state["change_action"] = "satisfied"
	addAgentConfigChangeResourceIDs(state, finalKeys, nil, nil)
	return state
}

func agentConfigBindingChanges(before *dto.AgentConfigResponse, after *dto.AgentConfigResponse, displayNames agentConfigDisplayNames) []map[string]interface{} {
	if before == nil || after == nil {
		return nil
	}
	changes := make([]map[string]interface{}, 0, 4)
	if change := agentConfigStringSetChange("enabled_skill_ids", "agent_skill", before.EnabledSkillIDs, after.EnabledSkillIDs, displayNames.Skills); len(change) > 0 {
		changes = append(changes, change)
	}
	if change := agentConfigStringSetChange("knowledge_dataset_ids", "knowledge_base", before.KnowledgeDatasetIDs, after.KnowledgeDatasetIDs, displayNames.KnowledgeBases); len(change) > 0 {
		changes = append(changes, change)
	}
	if change := agentConfigDatabaseBindingChange(before.DatabaseBindings, after.DatabaseBindings, displayNames.DatabaseTables); len(change) > 0 {
		changes = append(changes, change)
	}
	if change := agentConfigWorkflowBindingChange(before.WorkflowBindings, after.WorkflowBindings, displayNames.Workflows); len(change) > 0 {
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
		"final_resource_ids":     agentConfigChangesValues(changes, "final_resource_ids"),
		"final_resource_names":   agentConfigChangesNames(changes, "final_resource_names"),
		"added_resource_ids":     agentConfigChangesValues(changes, "added_resource_ids"),
		"added_resource_names":   agentConfigChangesNames(changes, "added_resource_names"),
		"removed_resource_ids":   agentConfigChangesValues(changes, "removed_resource_ids"),
		"removed_resource_names": agentConfigChangesNames(changes, "removed_resource_names"),
		"resource_ids":           agentConfigPrimaryChangeValues(action, changes),
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
	change := agentConfigChangePayload(field, kind, len(finalKeys), names(finalKeys), len(addedKeys), names(addedKeys), len(removedKeys), names(removedKeys))
	addAgentConfigChangeResourceIDs(change, finalKeys, addedKeys, removedKeys)
	return change
}

func addAgentConfigChangeResourceIDs(change map[string]interface{}, finalKeys []string, addedKeys []string, removedKeys []string) {
	if len(change) == 0 {
		return
	}
	if len(finalKeys) > 0 {
		change["final_resource_ids"] = append([]string(nil), finalKeys...)
	}
	if len(addedKeys) > 0 {
		change["added_resource_ids"] = append([]string(nil), addedKeys...)
	}
	if len(removedKeys) > 0 {
		change["removed_resource_ids"] = append([]string(nil), removedKeys...)
	}
	switch firstStringFromMap(change, "change_action") {
	case "bind":
		if len(addedKeys) > 0 {
			change["resource_ids"] = append([]string(nil), addedKeys...)
		}
	case "unbind":
		if len(removedKeys) > 0 {
			change["resource_ids"] = append([]string(nil), removedKeys...)
		}
	case "replace":
		ids := appendUniqueStrings(addedKeys, removedKeys...)
		if len(ids) > 0 {
			change["resource_ids"] = ids
		}
	default:
		if len(finalKeys) > 0 {
			change["resource_ids"] = append([]string(nil), finalKeys...)
		}
	}
}

func agentConfigDatabaseBindingChange(before []dto.AgentDatabaseBinding, after []dto.AgentDatabaseBinding, displayNames map[string]string) map[string]interface{} {
	beforeSet, beforeNames := databaseBindingResourceSet(before, displayNames)
	afterSet, afterNames := databaseBindingResourceSet(after, displayNames)
	if stringSetsEqual(beforeSet, afterSet) {
		return nil
	}
	addedKeys := sortedSetDiff(afterSet, beforeSet)
	removedKeys := sortedSetDiff(beforeSet, afterSet)
	finalKeys := sortedSetKeys(afterSet)
	change := agentConfigChangePayload(
		"database_bindings",
		"database_table",
		len(finalKeys),
		resourceNamesForKeys(finalKeys, afterNames),
		len(addedKeys),
		resourceNamesForKeys(addedKeys, afterNames),
		len(removedKeys),
		resourceNamesForKeys(removedKeys, beforeNames),
	)
	addAgentConfigChangeResourceIDs(change, finalKeys, addedKeys, removedKeys)
	return change
}

func agentConfigWorkflowBindingChange(before []dto.AgentWorkflowBinding, after []dto.AgentWorkflowBinding, displayNames map[string]string) map[string]interface{} {
	beforeSet, beforeNames := workflowBindingResourceSet(before, displayNames)
	afterSet, afterNames := workflowBindingResourceSet(after, displayNames)
	if stringSetsEqual(beforeSet, afterSet) {
		return nil
	}
	addedKeys := sortedSetDiff(afterSet, beforeSet)
	removedKeys := sortedSetDiff(beforeSet, afterSet)
	finalKeys := sortedSetKeys(afterSet)
	change := agentConfigChangePayload(
		"workflow_bindings",
		"workflow",
		len(finalKeys),
		resourceNamesForKeys(finalKeys, afterNames),
		len(addedKeys),
		resourceNamesForKeys(addedKeys, afterNames),
		len(removedKeys),
		resourceNamesForKeys(removedKeys, beforeNames),
	)
	addAgentConfigChangeResourceIDs(change, finalKeys, addedKeys, removedKeys)
	return change
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

func databaseBindingResourceSet(bindings []dto.AgentDatabaseBinding, displayNames map[string]string) (map[string]struct{}, map[string]string) {
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
			if name := displayNameForResource(displayNames, key, tableID); name != "" {
				names[key] = name
			}
		}
	}
	return set, names
}

func workflowBindingResourceSet(bindings []dto.AgentWorkflowBinding, displayNames map[string]string) (map[string]struct{}, map[string]string) {
	set := map[string]struct{}{}
	names := map[string]string{}
	for _, binding := range bindings {
		key := firstNonEmptyString(binding.BindingID, binding.WorkflowID, binding.AgentID)
		if key == "" {
			continue
		}
		set[key] = struct{}{}
		if name := firstNonEmptyString(
			displayNameForResource(displayNames, key, binding.WorkflowID, binding.AgentID),
			strings.TrimSpace(binding.Label),
		); name != "" && !strings.EqualFold(name, key) {
			names[key] = name
		}
	}
	return set, names
}

func agentConfigDisplayNamesFromParams(params map[string]interface{}) agentConfigDisplayNames {
	return agentConfigDisplayNames{
		Skills: mergeAgentConfigDisplayNameMaps(params,
			[]string{"skill_names", "enabled_skill_names", "agent_skill_names"},
			[]string{"skills", "agent_skills", "enabled_skills"},
		),
		KnowledgeBases: mergeAgentConfigDisplayNameMaps(params,
			[]string{"knowledge_dataset_names", "dataset_names", "knowledge_base_names"},
			[]string{"knowledge", "knowledge_bases", "datasets"},
		),
		DatabaseTables: mergeAgentConfigDisplayNameMaps(params,
			[]string{"database_table_names", "table_names"},
			[]string{"database_tables", "tables"},
		),
		Workflows: mergeAgentConfigDisplayNameMaps(params,
			[]string{"workflow_names", "workflow_binding_names"},
			[]string{"workflows", "workflow_bindings"},
		),
	}
}

func mergeAgentConfigDisplayNameMaps(params map[string]interface{}, topLevelKeys []string, nestedKeys []string) map[string]string {
	out := map[string]string{}
	for _, key := range topLevelKeys {
		appendAgentConfigDisplayNames(out, valueForKey(params, key))
	}
	if nested := mapFromAny(valueForKey(params, "display_names")); len(nested) > 0 {
		for _, key := range append(append([]string{}, topLevelKeys...), nestedKeys...) {
			appendAgentConfigDisplayNames(out, nested[key])
		}
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

func appendAgentConfigDisplayNames(out map[string]string, value interface{}) {
	if out == nil {
		return
	}
	if mapped := mapFromAny(value); len(mapped) > 0 {
		for key, rawName := range mapped {
			key = strings.TrimSpace(key)
			name := displayNameFromAny(rawName)
			if key == "" || name == "" || strings.EqualFold(key, name) {
				continue
			}
			out[key] = name
		}
		return
	}
	for _, item := range interfaceSlice(value) {
		mapped := mapFromAny(item)
		if len(mapped) == 0 {
			continue
		}
		dataSourceID := firstStringFromMap(mapped, "data_source_id")
		tableID := firstStringFromMap(mapped, "table_id")
		key := firstStringFromMap(
			mapped,
			"id",
			"skill_id",
			"dataset_id",
			"data_source_id",
			"table_id",
			"binding_id",
			"workflow_id",
			"agent_id",
		)
		name := firstStringFromMap(mapped, "name", "display_name", "label", "title")
		if key == "" || name == "" || strings.EqualFold(key, name) {
			continue
		}
		out[key] = name
		if dataSourceID != "" && tableID != "" {
			out[dataSourceID+":"+tableID] = name
		}
	}
}

func displayNameForResource(names map[string]string, keys ...string) string {
	for _, key := range keys {
		key = strings.TrimSpace(key)
		if key == "" {
			continue
		}
		if name := displayNameFromAny(names[key]); name != "" && !strings.EqualFold(name, key) {
			return name
		}
	}
	return ""
}

func displayNameFromAny(value interface{}) string {
	if value == nil {
		return ""
	}
	mapped := mapFromAny(value)
	if len(mapped) > 0 {
		return firstStringFromMap(mapped, "name", "display_name", "label", "title")
	}
	return strings.TrimSpace(fmt.Sprint(value))
}

func valueForKey(params map[string]interface{}, key string) interface{} {
	if params == nil {
		return nil
	}
	return params[key]
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
	return agentConfigChangesValues(changes, field)
}

func agentConfigChangesValues(changes []map[string]interface{}, field string) []string {
	out := make([]string, 0)
	for _, change := range changes {
		for _, value := range stringSliceFromAny(change[field]) {
			out = appendUniqueStrings(out, value)
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
	return agentConfigPrimaryChangeValuesForField(action, changes, "added_resource_names", "removed_resource_names")
}

func agentConfigPrimaryChangeValues(action string, changes []map[string]interface{}) []string {
	return agentConfigPrimaryChangeValuesForField(action, changes, "added_resource_ids", "removed_resource_ids")
}

func agentConfigPrimaryChangeValuesForField(action string, changes []map[string]interface{}, addedField string, removedField string) []string {
	switch action {
	case "bind":
		return agentConfigChangesValues(changes, addedField)
	case "unbind":
		return agentConfigChangesValues(changes, removedField)
	default:
		values := agentConfigChangesValues(changes, addedField)
		for _, value := range agentConfigChangesValues(changes, removedField) {
			values = appendUniqueStrings(values, value)
		}
		return values
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
	if description := strings.TrimSpace(stringValue(agentMap, "description")); description != "" {
		payload["agent_description"] = description
	}
	if iconType := strings.TrimSpace(stringValue(agentMap, "icon_type")); iconType != "" {
		payload["agent_icon_type"] = iconType
	}
	if icon := strings.TrimSpace(stringValue(agentMap, "icon")); icon != "" {
		payload["agent_icon"] = icon
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

func normalizeAgentThemeColor(raw string) (string, error) {
	value := strings.ToLower(strings.TrimSpace(raw))
	for _, allowed := range allowedAgentThemeColors {
		if value == allowed {
			return value, nil
		}
	}
	return "", fmt.Errorf("theme_color must be one of %s", strings.Join(allowedAgentThemeColors, ", "))
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
	return availableAgentModelPayloadWithQuery(model, "")
}

func availableAgentModelPayloadWithQuery(model *llmmodelservice.AvailableModel, query string) map[string]interface{} {
	if model == nil {
		return map[string]interface{}{}
	}
	payload := map[string]interface{}{
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
	if match := agentModelQueryMatchPayload(model, query); len(match) > 0 {
		payload["match"] = match
	}
	return payload
}

func rankAgentModelsForQuery(models []*llmmodelservice.AvailableModel, query string) []*llmmodelservice.AvailableModel {
	if strings.TrimSpace(query) == "" || len(models) <= 1 {
		return models
	}
	matched := make([]*llmmodelservice.AvailableModel, 0, len(models))
	unmatched := make([]*llmmodelservice.AvailableModel, 0, len(models))
	for _, model := range models {
		if availableAgentModelMatchesHint(model, query) {
			matched = append(matched, model)
			continue
		}
		unmatched = append(unmatched, model)
	}
	if len(matched) == 0 {
		return models
	}
	out := make([]*llmmodelservice.AvailableModel, 0, len(models))
	out = append(out, matched...)
	out = append(out, unmatched...)
	return out
}

func countAgentModelMatches(models []*llmmodelservice.AvailableModel, query string) int {
	if strings.TrimSpace(query) == "" {
		return 0
	}
	count := 0
	for _, model := range models {
		if availableAgentModelMatchesHint(model, query) {
			count++
		}
	}
	return count
}

func agentModelQueryMatchPayload(model *llmmodelservice.AvailableModel, query string) map[string]interface{} {
	query = strings.TrimSpace(query)
	if query == "" || model == nil {
		return nil
	}
	tokens := agentModelHintTokens(query)
	if len(tokens) == 0 {
		return map[string]interface{}{
			"query":   query,
			"matched": false,
			"reason":  "query did not contain model-specific tokens",
		}
	}
	candidate := compactAgentModelText(strings.Join([]string{
		model.Provider,
		model.Name,
		model.DisplayName,
	}, " "))
	matchedTokens := []string{}
	missingTokens := []string{}
	for _, token := range tokens {
		if strings.Contains(candidate, token) {
			matchedTokens = append(matchedTokens, token)
			continue
		}
		missingTokens = append(missingTokens, token)
	}
	matched := len(missingTokens) == 0
	reason := "not all query tokens matched this model"
	if matched {
		reason = "all query tokens matched provider, model id, or display name"
	}
	return map[string]interface{}{
		"query":          query,
		"matched":        matched,
		"score":          len(matchedTokens),
		"matched_tokens": matchedTokens,
		"missing_tokens": missingTokens,
		"reason":         reason,
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
	var rawItems []map[string]interface{}
	if err := json.Unmarshal(data, &rawItems); err == nil {
		for idx, raw := range rawItems {
			if idx >= len(bindings) {
				break
			}
			bindings[idx] = normalizeAgentDatabaseBindingAliases(bindings[idx], raw)
		}
	}
	return bindings, nil
}

func agentDatabaseTableBindingCandidates(tables []dto.AgentDatabaseTableCandidate) []map[string]interface{} {
	out := make([]map[string]interface{}, 0, len(tables))
	for _, table := range tables {
		dataSourceID := strings.TrimSpace(table.DataSourceID)
		tableID := strings.TrimSpace(table.TableID)
		if dataSourceID == "" || tableID == "" {
			continue
		}
		binding := map[string]interface{}{
			"data_source_id": dataSourceID,
			"table_ids":      []string{tableID},
		}
		out = append(out, map[string]interface{}{
			"id":             dataSourceID + ":" + tableID,
			"data_source_id": dataSourceID,
			"table_id":       tableID,
			"name":           strings.TrimSpace(table.Name),
			"selected":       table.Selected,
			"writable":       table.Writable,
			"binding":        binding,
		})
	}
	return out
}

func normalizeAgentDatabaseBindingAliases(binding dto.AgentDatabaseBinding, raw map[string]interface{}) dto.AgentDatabaseBinding {
	if len(raw) == 0 {
		return binding
	}
	if tableID := firstStringFromMap(raw, "table_id"); tableID != "" {
		binding.TableIDs = appendUniqueStrings(binding.TableIDs, tableID)
	}
	if writableTableID := firstStringFromMap(raw, "writable_table_id"); writableTableID != "" {
		binding.WritableTableIDs = appendUniqueStrings(binding.WritableTableIDs, writableTableID)
	}
	for _, key := range []string{"database_table_id", "database_table_ids", "database_table_keys", "resource_id", "resource_ids", "id", "ids"} {
		for _, compound := range stringSliceFromAny(raw[key]) {
			dataSourceID, tableID, ok := splitAgentDatabaseTableBindingKey(compound)
			if !ok {
				continue
			}
			if strings.TrimSpace(binding.DataSourceID) == "" {
				binding.DataSourceID = dataSourceID
			}
			if !strings.EqualFold(strings.TrimSpace(binding.DataSourceID), dataSourceID) {
				continue
			}
			binding.TableIDs = appendUniqueStrings(binding.TableIDs, tableID)
		}
	}
	return binding
}

func splitAgentDatabaseTableBindingKey(value string) (string, string, bool) {
	left, right, ok := strings.Cut(strings.TrimSpace(value), ":")
	if !ok {
		return "", "", false
	}
	dataSourceID := strings.TrimSpace(left)
	tableID := strings.TrimSpace(right)
	if dataSourceID == "" || tableID == "" {
		return "", "", false
	}
	return dataSourceID, tableID, true
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

func mapsFromAny(value interface{}) []map[string]interface{} {
	switch typed := value.(type) {
	case nil:
		return nil
	case []map[string]interface{}:
		out := make([]map[string]interface{}, 0, len(typed))
		for _, item := range typed {
			if len(item) > 0 {
				out = append(out, copyStringAnyMap(item))
			}
		}
		return out
	case []interface{}:
		out := make([]map[string]interface{}, 0, len(typed))
		for _, item := range typed {
			if mapped := mapFromAny(item); len(mapped) > 0 {
				out = append(out, mapped)
			}
		}
		return out
	default:
		mapped := mapFromAny(value)
		if len(mapped) == 0 {
			return nil
		}
		return []map[string]interface{}{mapped}
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
