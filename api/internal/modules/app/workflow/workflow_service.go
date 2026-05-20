// TODO: This file needs to be refactored to avoid circular dependencies
// Currently imports external packages that cause import cycles

package workflow

import (
	"context"
	"crypto/md5"
	"encoding/json"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"
	appconfig "github.com/zgiai/zgi/api/config"
	"github.com/zgiai/zgi/api/internal/dto"
	"github.com/zgiai/zgi/api/internal/modules/app/agents"
	"github.com/zgiai/zgi/api/internal/modules/app/conversation"
	"github.com/zgiai/zgi/api/internal/modules/app/workflow/diagnosis"
	"github.com/zgiai/zgi/api/internal/modules/app/workflow/file"
	"github.com/zgiai/zgi/api/internal/modules/app/workflow/graph_engine"
	"github.com/zgiai/zgi/api/internal/modules/app/workflow/graph_engine/entities"
	"github.com/zgiai/zgi/api/internal/modules/app/workflow/shared"
	automationdefinition "github.com/zgiai/zgi/api/internal/modules/automation/service/definition"
	"github.com/zgiai/zgi/api/internal/modules/dataset/graphflow"
	promptservice "github.com/zgiai/zgi/api/internal/modules/prompts/service"
	quota_model "github.com/zgiai/zgi/api/internal/modules/quota/model"
	interfaces "github.com/zgiai/zgi/api/internal/modules/shared/interface"
	"github.com/zgiai/zgi/api/pkg/database"
	"github.com/zgiai/zgi/api/pkg/logger"
)

// WorkflowService workflow service
type WorkflowService struct {
	executor                   *WorkflowExecutor
	repo                       WorkflowRepository
	agentsRepo                 agents.AgentsRepository
	workflowRunLogRepo         WorkflowRunLogRepository
	workflowNodeRuntimeLogRepo WorkflowNodeRuntimeLogRepository
	accountService             interfaces.AccountService
	advancedChatHandler        *AdvancedChatWorkflowHandler
	workflowRunMessageLookup   workflowRunMessageLookup
	quotaService               interfaces.QuotaService
	enterpriseService          interfaces.OrganizationService
	// runningWorkflows stores cancel functions for running workflows, keyed by workflow run ID
	runningWorkflows   map[string]context.CancelFunc
	runningWorkflowsMu sync.RWMutex
	diagnoser          *diagnosis.Diagnoser
}

type workflowRunMessageLookup interface {
	GetFirstMessagesByWorkflowRunIDs(ctx context.Context, workflowRunIDs []string) (map[string]*conversation.AgentMessage, error)
}

type runLinkedMessage struct {
	MessageID      string
	ConversationID string
}

type reusableSessionCleaner interface {
	StopReusableSessionsByWorkflowRunID(ctx context.Context, workflowRunID string) (int, error)
	SweepStaleReusableSessions(ctx context.Context, maxAge time.Duration) (int, error)
}

func (s *WorkflowService) getOrganizationIDByWorkspace(ctx context.Context, workspaceID string) string {
	if s == nil || s.enterpriseService == nil || workspaceID == "" {
		return ""
	}

	organization, err := s.enterpriseService.GetOrganizationByWorkspaceID(ctx, workspaceID)
	if err != nil || organization == nil {
		return ""
	}

	return organization.ID
}

func isUnsetWorkflowWorkspaceID(workspaceID string) bool {
	workspaceID = strings.TrimSpace(workspaceID)
	return workspaceID == "" || workspaceID == uuid.Nil.String()
}

func ensureWorkflowSystemInputs(inputs map[string]interface{}, workspaceID, agentID, workflowID, workflowRunID, accountID, organizationID string) {
	if inputs == nil {
		return
	}

	inputs["sys.user_id"] = accountID
	inputs["sys.agent_id"] = agentID
	inputs["sys.tenant_id"] = workspaceID
	inputs["sys.workspace_id"] = workspaceID
	if organizationID != "" {
		inputs["sys.organization_id"] = organizationID
	}
	inputs["sys.workflow_id"] = workflowID
	inputs["sys.workflow_run_id"] = workflowRunID
}

func effectiveWorkflowWorkspaceID(workflow *Workflow, requestedWorkspaceID string) string {
	if workflow != nil && !isUnsetWorkflowWorkspaceID(workflow.TenantID) {
		return workflow.TenantID
	}
	if !isUnsetWorkflowWorkspaceID(requestedWorkspaceID) {
		return strings.TrimSpace(requestedWorkspaceID)
	}
	return ""
}

func resolveServiceWebAppRunWorkspaceID(ctx context.Context, accountService currentWorkspaceGetter, accountID string, agent *agents.Agent) (string, error) {
	fallbackWorkspaceID := ""
	if agent != nil && agent.TenantID != uuid.Nil {
		fallbackWorkspaceID = agent.TenantID.String()
	}
	return resolveWebAppRunWorkspaceID(ctx, accountService, accountID, fallbackWorkspaceID)
}

// normalizeVariables ensures variables (both environment and conversation) have consistent structure
// by only including fields that have non-zero values, matching the input format
// This prevents the API from returning extra fields with empty/default values
// hideSecrets: if true, secret values will be replaced with SecretHiddenValue
func normalizeVariables(variables []interface{}, hideSecrets ...bool) []map[string]any {
	result := make([]map[string]any, 0, len(variables))
	shouldHideSecrets := len(hideSecrets) > 0 && hideSecrets[0]

	for _, v := range variables {
		varMap, ok := v.(map[string]any)
		if !ok {
			continue
		}

		// Create normalized variable with only non-empty fields
		normalized := make(map[string]any)

		// Always include these core fields if present
		if id, exists := varMap["id"]; exists && id != "" {
			normalized["id"] = id
		}
		if name, exists := varMap["name"]; exists && name != "" {
			normalized["name"] = name
		}
		if valueType, exists := varMap["value_type"]; exists && valueType != "" {
			normalized["value_type"] = valueType
		}

		// Include optional fields only if they have meaningful values
		if label, exists := varMap["label"]; exists && label != "" {
			normalized["label"] = label
		}
		if typ, exists := varMap["type"]; exists && typ != "" {
			normalized["type"] = typ
		}
		if required, exists := varMap["required"]; exists {
			// Only include if explicitly set to true
			if reqBool, ok := required.(bool); ok && reqBool {
				normalized["required"] = reqBool
			}
		}
		if value, exists := varMap["value"]; exists {
			normalized["value"] = value

			// Hide secret values if requested
			if shouldHideSecrets {
				valType := ""
				if vt, exists := varMap["value_type"].(string); exists && vt != "" {
					valType = vt
				} else if vt, exists := varMap["type"].(string); exists && vt != "" {
					valType = vt
				}

				if valType == entities.SecretValueType {
					normalized["value"] = entities.SecretHiddenValue
				}
			}
		}
		if description, exists := varMap["description"]; exists && description != "" {
			normalized["description"] = description
		}

		result = append(result, normalized)
	}

	return result
}

func mergeRootVariablesIntoGraph(workflowMap map[string]any) (map[string]any, error) {
	graphData, ok := workflowMap["graph"].(map[string]any)
	if !ok {
		return nil, fmt.Errorf("graph data is not a map")
	}

	if envVars, exists := workflowMap["environment_variables"]; exists {
		graphData["environment_variables"] = envVars
	}
	if convVars, exists := workflowMap["conversation_variables"]; exists {
		graphData["conversation_variables"] = convVars
	}

	return graphData, nil
}

// NewWorkflowService creates a new workflow service
func NewWorkflowService(repo WorkflowRepository, agentsRepo agents.AgentsRepository, workflowRunLogRepo WorkflowRunLogRepository, workflowNodeRuntimeLogRepo WorkflowNodeRuntimeLogRepository, accountService interfaces.AccountService) *WorkflowService {
	advancedChatHandler := NewAdvancedChatWorkflowHandler()
	return &WorkflowService{
		executor:                   NewWorkflowExecutor(),
		repo:                       repo,
		agentsRepo:                 agentsRepo,
		workflowRunLogRepo:         workflowRunLogRepo,
		workflowNodeRuntimeLogRepo: workflowNodeRuntimeLogRepo,
		accountService:             accountService,
		advancedChatHandler:        advancedChatHandler,
		workflowRunMessageLookup:   advancedChatHandler,
		quotaService:               nil,
		enterpriseService:          nil,
		runningWorkflows:           make(map[string]context.CancelFunc),
	}
}

// NewWorkflowServiceWithDeps creates a new workflow service with dependencies
func NewWorkflowServiceWithDeps(repo WorkflowRepository, agentsRepo agents.AgentsRepository, workflowRunLogRepo WorkflowRunLogRepository, workflowNodeRuntimeLogRepo WorkflowNodeRuntimeLogRepository, accountService interfaces.AccountService, fileService interfaces.FileService) *WorkflowService {
	advancedChatHandler := NewAdvancedChatWorkflowHandler()
	return &WorkflowService{
		executor:                   NewWorkflowExecutorWithDeps(fileService),
		repo:                       repo,
		agentsRepo:                 agentsRepo,
		workflowRunLogRepo:         workflowRunLogRepo,
		workflowNodeRuntimeLogRepo: workflowNodeRuntimeLogRepo,
		accountService:             accountService,
		advancedChatHandler:        advancedChatHandler,
		workflowRunMessageLookup:   advancedChatHandler,
		quotaService:               nil,
		enterpriseService:          nil,
		runningWorkflows:           make(map[string]context.CancelFunc),
	}
}

// NewWorkflowServiceWithContentExtractor creates a new workflow service with ContentExtractor
func NewWorkflowServiceWithContentExtractor(repo WorkflowRepository, agentsRepo agents.AgentsRepository, workflowRunLogRepo WorkflowRunLogRepository, workflowNodeRuntimeLogRepo WorkflowNodeRuntimeLogRepository, accountService interfaces.AccountService, fileService interfaces.FileService, contentExtractor interface{}, quotaService interfaces.QuotaService, enterpriseService interfaces.OrganizationService, llmClient interface{}, toolEngine interface{}, graphFlowService *graphflow.Service, promptResolver promptservice.PromptService, automationDefinitionService automationdefinition.Service, engineFactories ...*graph_engine.EngineFactory) *WorkflowService {
	var engineFactory *graph_engine.EngineFactory
	if len(engineFactories) > 0 {
		engineFactory = engineFactories[0]
	}

	workflowContentExtractor, _ := contentExtractor.(file.ContentExtractor)
	executor := NewWorkflowExecutorWithRuntimeDeps(WorkflowExecutorDeps{
		FileService:                 fileService,
		ContentExtractor:            workflowContentExtractor,
		LLMClient:                   llmClient,
		ToolEngine:                  toolEngine,
		GraphFlowService:            graphFlowService,
		PromptResolver:              promptResolver,
		AutomationDefinitionService: automationDefinitionService,
		EngineFactory:               engineFactory,
	})
	advancedChatHandler := NewAdvancedChatWorkflowHandler()

	return &WorkflowService{
		executor:                   executor,
		repo:                       repo,
		agentsRepo:                 agentsRepo,
		workflowRunLogRepo:         workflowRunLogRepo,
		workflowNodeRuntimeLogRepo: workflowNodeRuntimeLogRepo,
		accountService:             accountService,
		advancedChatHandler:        advancedChatHandler,
		workflowRunMessageLookup:   advancedChatHandler,
		quotaService:               quotaService,
		enterpriseService:          enterpriseService,
		runningWorkflows:           make(map[string]context.CancelFunc),
	}
}

func (s *WorkflowService) SetDiagnoser(diagnoser *diagnosis.Diagnoser) {
	s.diagnoser = diagnoser
}

// RegisterRunningWorkflow registers a cancel function for a running workflow
func (s *WorkflowService) RegisterRunningWorkflow(runID string, cancel context.CancelFunc) {
	s.runningWorkflowsMu.Lock()
	defer s.runningWorkflowsMu.Unlock()
	if s.runningWorkflows == nil {
		s.runningWorkflows = make(map[string]context.CancelFunc)
	}
	s.runningWorkflows[runID] = cancel
	logger.Info("Registered running workflow", "runID", runID)
}

// UnregisterRunningWorkflow removes a workflow from the running workflows map
func (s *WorkflowService) UnregisterRunningWorkflow(runID string) {
	s.runningWorkflowsMu.Lock()
	defer s.runningWorkflowsMu.Unlock()
	if s.runningWorkflows != nil {
		delete(s.runningWorkflows, runID)
		logger.Info("Unregistered running workflow", "runID", runID)
	}
}

// CancelRunningWorkflow cancels a running workflow by its run ID
func (s *WorkflowService) CancelRunningWorkflow(runID string) bool {
	s.runningWorkflowsMu.RLock()
	cancel, exists := s.runningWorkflows[runID]
	s.runningWorkflowsMu.RUnlock()

	if exists && cancel != nil {
		cancel()
		logger.Info("Cancelled running workflow", "runID", runID)
		return true
	}
	return false
}

func (s *WorkflowService) getReusableSessionCleaner() reusableSessionCleaner {
	if s.executor == nil {
		return nil
	}
	toolEngine := s.executor.GetToolEngine()
	if toolEngine == nil {
		return nil
	}
	cleaner, ok := toolEngine.(reusableSessionCleaner)
	if !ok {
		return nil
	}
	return cleaner
}

// CleanupWorkflowReusableSessions forces cleanup of reusable plugin sessions for one workflow run.
func (s *WorkflowService) CleanupWorkflowReusableSessions(ctx context.Context, workflowRunID string) (int, error) {
	workflowRunID = strings.TrimSpace(workflowRunID)
	if workflowRunID == "" {
		return 0, nil
	}
	cleaner := s.getReusableSessionCleaner()
	if cleaner == nil {
		return 0, nil
	}
	return cleaner.StopReusableSessionsByWorkflowRunID(ctx, workflowRunID)
}

func (s *WorkflowService) cleanupWorkflowReusableSessionsWithTimeout(workflowRunID string) {
	workflowRunID = strings.TrimSpace(workflowRunID)
	if workflowRunID == "" {
		return
	}

	timeout := 30 * time.Second
	if cfg := appconfig.GlobalConfig; cfg != nil && cfg.Workflow.CleanupTimeout > 0 {
		timeout = time.Duration(cfg.Workflow.CleanupTimeout) * time.Second
	}

	cleanupCtx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	stoppedCount, err := s.CleanupWorkflowReusableSessions(cleanupCtx, workflowRunID)
	if err != nil {
		logger.Warn("failed to cleanup reusable workflow sessions",
			"workflow_run_id", workflowRunID,
			"error", err)
		return
	}
	if stoppedCount > 0 {
		logger.Info("cleaned reusable workflow sessions",
			"workflow_run_id", workflowRunID,
			"stopped_count", stoppedCount)
	}
}

// GetAgentWorkspaceID gets the workspace that owns the agent.
func (s *WorkflowService) GetAgentWorkspaceID(ctx context.Context, agentID string) (string, error) {
	if s.agentsRepo == nil {
		return "", fmt.Errorf("agents repository not initialized")
	}
	agent, err := s.agentsRepo.GetByID(ctx, agentID)
	if err != nil {
		return "", fmt.Errorf("failed to get agent: %w", err)
	}
	if agent == nil {
		return "", fmt.Errorf("agent not found")
	}
	return agent.TenantID.String(), nil
}

// Deprecated: use GetAgentWorkspaceID. The legacy name reflects the old tenant/workspace terminology.
func (s *WorkflowService) GetAgentTenantID(ctx context.Context, agentID string) (string, error) {
	return s.GetAgentWorkspaceID(ctx, agentID)
}

// GetDraftWorkflow gets draft workflow
// hideSecrets: if true, secret values will be replaced with SecretHiddenValue
func (s *WorkflowService) GetDraftWorkflow(ctx context.Context, agentID string, hideSecrets ...bool) (interface{}, error) {
	// Return error if repository is not initialized
	if s.repo == nil {
		return nil, fmt.Errorf("workflow repository not initialized")
	}

	// Get draft workflow from database
	workflow, err := s.repo.GetDraftWorkflow(ctx, agentID)
	if err != nil {
		if err.Error() == "draft workflow not found" {
			// Return empty object if draft workflow not found
			return map[string]interface{}{}, nil
		}
		return nil, err
	}

	// Hide or show environment variables based on hideSecrets parameter
	shouldHideSecrets := len(hideSecrets) > 0 && hideSecrets[0]
	// Build response
	response := map[string]interface{}{
		"id":                     workflow.ID,
		"tenant_id":              workflow.TenantID,
		"workspace_id":           workflow.TenantID,
		"agent_id":               workflow.AgentID,
		"type":                   workflow.Type,
		"version":                workflow.Version,
		"created_by":             workflow.CreatedBy,
		"created_at":             workflow.CreatedAt.Unix(),
		"updated_at":             workflow.UpdatedAt.Unix(),
		"graph":                  workflow.GetGraphDict(),
		"features":               workflow.GetFeaturesDict(),
		"environment_variables":  normalizeVariables(workflow.GetEnvironmentVariablesDict(), shouldHideSecrets),
		"conversation_variables": normalizeVariables(workflow.GetConversationVariablesDict()),
		"internal":               workflow.Internal,
	}

	if workflow.UpdatedBy != nil {
		response["updated_by"] = *workflow.UpdatedBy
	}

	return response, nil
}

// SyncDraftWorkflow syncs draft workflow
func (s *WorkflowService) SyncDraftWorkflow(ctx context.Context, workspaceID, agentID string, req interface{}, accountID string) (interface{}, error) {
	logger.Info("Syncing draft workflow", "agentID", agentID, "accountID", accountID)

	// Type assertion
	syncReq, ok := req.(*dto.SyncDraftWorkflowRequest)
	if !ok {
		return nil, fmt.Errorf("invalid request type")
	}

	if s.repo == nil {
		return nil, fmt.Errorf("workflow repository not initialized")
	}

	graphJSON, err := json.Marshal(syncReq.Graph)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal graph: %w", err)
	}

	featuresJSON, err := json.Marshal(syncReq.Features)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal features: %w", err)
	}

	envVarsJSON := "[]"
	if len(syncReq.EnvironmentVariables) > 0 {
		// Replace SecretHiddenValue with actual values from persisted variables
		envVarsForSave := make([]map[string]interface{}, len(syncReq.EnvironmentVariables))
		for i, v := range syncReq.EnvironmentVariables {
			// Use type field for value_type (frontend sends value_type in type field)
			valueType := v.Type

			envVarsForSave[i] = map[string]interface{}{
				"id":          v.ID,
				"name":        v.Name,
				"label":       v.Label,
				"type":        v.Type,
				"required":    v.Required,
				"description": v.Description,
				"value":       v.Value,
				"selector":    v.Selector,
				"value_type":  valueType,
				"config":      v.Config,
			}
		}

		// Get existing workflow to get persisted variables
		existingWorkflow, _ := s.repo.GetDraftWorkflow(ctx, agentID)

		for i := range envVarsForSave {
			if envVarsForSave[i]["value_type"] == entities.SecretValueType {
				if str, ok := envVarsForSave[i]["value"].(string); ok && str == entities.SecretHiddenValue {
					// Replace SecretHiddenValue with actual value from persisted variables
					if existingWorkflow != nil {
						persistedEnvVars := existingWorkflow.GetEnvironmentVariablesDict()
						for _, v := range persistedEnvVars {
							if varMap, ok := v.(map[string]interface{}); ok {
								if name, ok := varMap["name"].(string); ok && name == envVarsForSave[i]["name"].(string) {
									if value, exists := varMap["value"]; exists {
										envVarsForSave[i]["value"] = value
									}
									break
								}
							}
						}
					}
				}
			}
		}

		if b, err := json.Marshal(envVarsForSave); err == nil {
			envVarsJSON = string(b)
		}
	}

	convVarsJSON := "[]"
	if len(syncReq.ConversationVariables) > 0 {
		// Validate conversation variables before saving
		if err := ValidateConversationVariables(syncReq.ConversationVariables); err != nil {
			logger.Error("Conversation variables validation failed", err)
			return nil, fmt.Errorf("invalid conversation_variables: %w", err)
		}

		if b, err := json.Marshal(syncReq.ConversationVariables); err == nil {
			convVarsJSON = string(b)
			logger.Info("Conversation variables validated and marshaled successfully", "count", len(syncReq.ConversationVariables))
		} else {
			logger.Error("Failed to marshal conversation variables", err)
			return nil, fmt.Errorf("failed to marshal conversation_variables: %w", err)
		}
	}

	// Check if draft workflow already exists
	existingWorkflow, err := s.repo.GetDraftWorkflow(ctx, agentID)
	if err != nil && err.Error() != "draft workflow not found" {
		return nil, fmt.Errorf("failed to check existing workflow: %w", err)
	}

	now := time.Now()
	var workflow *Workflow

	// Determine internal value - use false as default if not provided
	internal := false
	if syncReq.Internal != nil {
		internal = *syncReq.Internal
	}

	if existingWorkflow == nil {
		// Note: Workflow quota check is done when creating agent, not here.
		// Saving workflow for an existing agent should not be blocked by quota.

		// Create new draft workflow
		workflow = &Workflow{
			ID:                    uuid.New().String(),
			TenantID:              workspaceID,
			AppID:                 agentID,
			AgentID:               agentID, // For workflow apps, agent_id equals app_id
			Type:                  syncReq.Type,
			Version:               "draft",
			Graph:                 string(graphJSON),
			Features:              string(featuresJSON),
			EnvironmentVariables:  envVarsJSON,
			ConversationVariables: convVarsJSON,
			Internal:              internal,
			CreatedBy:             accountID,
			CreatedAt:             now,
			UpdatedAt:             now,
		}

		if err := s.repo.Create(ctx, workflow); err != nil {
			return nil, fmt.Errorf("failed to create workflow: %w", err)
		}
		// Note: Quota usage is recorded when creating agent, not here.
	} else {
		// Update existing draft workflow
		existingWorkflow.Graph = string(graphJSON)
		existingWorkflow.Features = string(featuresJSON)
		existingWorkflow.EnvironmentVariables = envVarsJSON
		existingWorkflow.ConversationVariables = convVarsJSON
		existingWorkflow.Internal = internal
		existingWorkflow.UpdatedBy = &accountID
		existingWorkflow.UpdatedAt = now

		if err := s.repo.Update(ctx, existingWorkflow); err != nil {
			return nil, fmt.Errorf("failed to update workflow: %w", err)
		}
		workflow = existingWorkflow
	}

	// Update agent's workflow_config for conversational workflows
	if workflow.Type == "chat" && s.agentsRepo != nil {
		agent, err := s.agentsRepo.GetByID(ctx, agentID)
		if err == nil && agent != nil {
			// Parse existing workflow_config or create new one
			var workflowConfig map[string]interface{}
			if agent.WorkflowConfig != nil && *agent.WorkflowConfig != "" {
				if err := json.Unmarshal([]byte(*agent.WorkflowConfig), &workflowConfig); err != nil {
					// If parsing fails, create new config
					workflowConfig = make(map[string]interface{})
				}
			} else {
				workflowConfig = make(map[string]interface{})
			}

			// Update workflow_id in config (but keep it empty for draft)
			workflowConfig["workflow_id"] = workflow.ID

			// Preserve or initialize other fields
			if _, exists := workflowConfig["history_window_size"]; !exists {
				workflowConfig["history_window_size"] = 10 // default
			}
			if _, exists := workflowConfig["variable_config"]; !exists {
				workflowConfig["variable_config"] = make(map[string]interface{})
			}
			if _, exists := workflowConfig["conversation_id"]; !exists {
				workflowConfig["conversation_id"] = ""
			}

			// Save updated config
			configJSON, err := json.Marshal(workflowConfig)
			if err == nil {
				configStr := string(configJSON)
				agent.WorkflowConfig = &configStr
				if updateErr := s.agentsRepo.Update(ctx, agent); updateErr != nil {
					logger.Error("Failed to update agent workflow_config", updateErr)
					// Don't fail the whole operation
				} else {
					logger.Info("Updated agent workflow_config", "agentID", agentID, "workflowID", workflow.ID)
				}
			}
		}
	}

	// Generate response
	response := map[string]interface{}{
		"result":     "success",
		"hash":       fmt.Sprintf("%x", md5.Sum([]byte(workflow.ID+workflow.UpdatedAt.String()))),
		"updated_at": workflow.UpdatedAt,
	}

	return response, nil
}

// RunDraftWorkflow runs draft workflow
func (s *WorkflowService) RunDraftWorkflow(ctx context.Context, workspaceID, agentID string, req interface{}, accountID string) (interface{}, error) {
	logger.Info("Running draft workflow", "agentID", agentID, "accountID", accountID)

	// Type assertion
	if draftReq, ok := req.(*dto.DraftWorkflowRunRequest); ok {
		// Get actual workflow configuration from database (don't hide secrets for execution)
		workflowData, err := s.GetDraftWorkflow(ctx, agentID, false)
		if err != nil {
			logger.Error("Failed to get workflow configuration", err)
			return nil, fmt.Errorf("failed to get workflow configuration: %w", err)
		}

		// Parse workflow data
		workflowMap, ok := workflowData.(map[string]any)
		if !ok {
			logger.Error("Invalid workflow data format", fmt.Errorf("workflow data is not a map"))
			return nil, fmt.Errorf("invalid workflow data format")
		}

		graphData, err := mergeRootVariablesIntoGraph(workflowMap)
		if err != nil {
			logger.Error("Invalid graph data format", err)
			return nil, fmt.Errorf("invalid graph data format: %w", err)
		}

		// Handle conversation workflow if needed
		var conversationID *string
		var workflowRunIDStr string
		var workflowRunLogID string
		defer func() {
			s.cleanupWorkflowReusableSessionsWithTimeout(workflowRunLogID)
		}()

		// Check if this is a conversation workflow
		if workflowType, exists := workflowMap["type"].(string); exists && workflowType == "chat" {
			// Extract conversation parameters from inputs
			fromSource := "account"                  // default
			invokeFrom := string(InvokeFromDebugger) // For draft workflow, always use debugger
			var overrideModelConfigs *string

			// Check if we have conversation parameters in the request
			if convParams, ok := draftReq.Inputs["conversation_params"].(map[string]any); ok {
				if fs, ok := convParams["from_source"].(string); ok {
					fromSource = fs
				}
				// Note: We don't override invokeFrom from params for draft workflow
				if omc, ok := convParams["override_model_configs"].(string); ok {
					overrideModelConfigs = &omc
				}
			}

			// Get or create conversation
			var convIDStr string
			if convID, exists := draftReq.Inputs["sys.conversation_id"].(string); exists && convID != "" {
				convIDStr = convID
			} else {
				// Create new conversation with invoke_from = "debugger" and workflow_version_uuid = null
				newConvID, err := s.advancedChatHandler.CreateConversationRecordWithParams(
					workspaceID, agentID, accountID, fromSource, invokeFrom, draftReq.Inputs, overrideModelConfigs, nil)
				if err != nil {
					logger.Error("Failed to create conversation", err)
					return nil, fmt.Errorf("failed to create conversation: %w", err)
				}
				convIDStr = newConvID
			}

			conversationID = &convIDStr
			// Add conversation ID to inputs for workflow execution
			draftReq.Inputs["sys.conversation_id"] = convIDStr
		}

		// Get the actual workflow ID from database
		actualWorkflowID := agentID // fallback to agentID if query fails
		if s.repo != nil {
			workflow, err := s.repo.GetDraftWorkflow(ctx, agentID)
			if err == nil && workflow != nil {
				actualWorkflowID = workflow.ID
				logger.DebugContext(ctx, "draft workflow id loaded", "workflow_id", actualWorkflowID, "agent_id", agentID)
			} else {
				logger.WarnContext(ctx, "failed to load draft workflow id, using agent id fallback", "agent_id", agentID, err)
			}
		}

		// Resolve organization scope for workflow system variables.
		organizationID := ""
		if s.enterpriseService != nil {
			organization, err := s.enterpriseService.GetOrganizationByWorkspaceID(ctx, workspaceID)
			if err == nil && organization != nil {
				organizationID = organization.ID
			}
		}

		// Create workflow run log with triggered_from = "debugger" for draft workflow
		workflowRunLog, err := s.CreateWorkflowRunLog(ctx, workspaceID, agentID, actualWorkflowID, "debugger", draftReq.Inputs, accountID)
		if err != nil {
			logger.Error("Failed to create workflow run log", err)
			// Continue execution even if logging fails
		} else if workflowRunLogTyped, ok := workflowRunLog.(*WorkflowRunLog); ok {
			workflowRunLogID = workflowRunLogTyped.ID
			workflowRunIDStr = workflowRunLogID
		}
		if draftReq.Inputs == nil {
			draftReq.Inputs = make(map[string]interface{})
		}
		ensureWorkflowSystemInputs(draftReq.Inputs, workspaceID, agentID, actualWorkflowID, workflowRunLogID, accountID, organizationID)

		// Execute workflow with actual graph data
		startTime := time.Now()
		executionResult, err := s.executor.ExecuteSimpleWorkflowWithRunID(ctx, workflowRunLogID, graphData, draftReq.Inputs)
		elapsedTime := workflowExecutionResultElapsedMilliseconds(executionResult, ElapsedMillisecondsSince(startTime))

		// Update workflow run log with results
		if workflowRunLog != nil {
			status, errorMessage := workflowExecutionLogStatusAndError(executionResult, err)

			if workflowRunLogTyped, ok := workflowRunLog.(*WorkflowRunLog); ok {
				nodeResults := map[string]interface{}{}
				if executionResult != nil {
					nodeResults = executionResult.NodeResults
				}
				updateErr := s.UpdateWorkflowRunLogStatus(ctx, workflowRunLogTyped.ID, status, nodeResults, elapsedTime, 0, len(nodeResults), errorMessage)
				if updateErr != nil {
					logger.Error("Failed to update workflow run log", updateErr)
				}
			}
		}

		if err != nil {
			logger.Error("Failed to execute workflow", err)
			return nil, fmt.Errorf("workflow execution failed: %w", err)
		}

		// For conversation workflows, create message records
		if conversationID != nil {
			// Extract query and answer from inputs and outputs
			query := ""
			if q, exists := draftReq.Inputs["sys.query"].(string); exists {
				query = q
			} else if q, exists := draftReq.Inputs["query"].(string); exists {
				query = q
			}

			answer := ""
			// Try different possible output keys based on node types
			if textOutput, exists := executionResult.NodeResults["text"].(string); exists {
				answer = textOutput
			} else if answerOutput, exists := executionResult.NodeResults["answer"].(string); exists {
				// Answer node outputs "answer" key
				answer = answerOutput
			} else if endNodeOutputs, ok := executionResult.NodeResults["end"]; ok {
				if endOutputsMap, ok := endNodeOutputs.(map[string]interface{}); ok {
					if textOutput, exists := endOutputsMap["text"].(string); exists {
						answer = textOutput
					} else if answerOutput, exists := endOutputsMap["answer"].(string); exists {
						answer = answerOutput
					}
				}
			} else {
				// Try to find answer from any node output
				for _, nodeOutput := range executionResult.NodeResults {
					if nodeOutputMap, ok := nodeOutput.(map[string]interface{}); ok {
						if nodeText, exists := nodeOutputMap["text"].(string); exists {
							answer = nodeText
							break
						} else if nodeAnswer, exists := nodeOutputMap["answer"].(string); exists {
							answer = nodeAnswer
							break
						}
					}
				}
			}

			// Parse UUIDs
			agentUUID, err := uuid.Parse(agentID)
			if err != nil {
				logger.Error("Invalid agent ID", err)
				return nil, fmt.Errorf("invalid agent ID: %w", err)
			}

			conversationUUID, err := uuid.Parse(*conversationID)
			if err != nil {
				logger.Error("Invalid conversation ID", err)
				return nil, fmt.Errorf("invalid conversation ID: %w", err)
			}

			workflowRunUUID, err := uuid.Parse(workflowRunIDStr)
			if err != nil {
				logger.Error("Invalid workflow run ID", err)
				return nil, fmt.Errorf("invalid workflow run ID: %w", err)
			}

			accountUUID, err := uuid.Parse(accountID)
			if err != nil {
				logger.Error("Invalid account ID", err)
				return nil, fmt.Errorf("invalid account ID: %w", err)
			}

			// Create message record with invoke_from = "debugger" and workflow_version_uuid = null
			_, err = s.advancedChatHandler.CreateWorkflowMessage(
				agentUUID,
				conversationUUID,
				workflowRunUUID,
				query,
				answer,
				"account",
				string(InvokeFromDebugger), // invokeFrom - debugger for draft workflow
				accountUUID,
				&accountUUID,
				nil, // workflowVersionUUID - null for draft/debug mode
			)
			if err != nil {
				logger.Error("Failed to create workflow message", err)
				// Don't fail the entire workflow execution if message creation fails
			}
		}

		// Build response with readable final output for blocking callers such as batch tests.
		workflowRunID := "run-" + agentID
		if workflowRunLog != nil {
			if workflowRunLogTyped, ok := workflowRunLog.(*WorkflowRunLog); ok {
				workflowRunID = workflowRunLogTyped.ID
			}
		}
		response := buildBlockingWorkflowRunResponse(agentID, workflowRunID, executionResult, elapsedTime)

		return response, nil
	}

	// Return error if type doesn't match
	return nil, fmt.Errorf("invalid request type")
}

// RunDraftWorkflowNode runs draft workflow node
func (s *WorkflowService) RunDraftWorkflowNode(ctx context.Context, workspaceID, agentID, nodeID string, req interface{}, accountID string) (interface{}, error) {
	logger.Info("Running draft workflow node", "agentID", agentID, "nodeID", nodeID, "accountID", accountID)

	// Type assertion
	var inputs map[string]interface{}
	if draftReq, ok := req.(*dto.DraftWorkflowNodeRunRequest); ok {
		inputs = draftReq.Inputs
	} else {
		// Use empty input if type doesn't match
		inputs = make(map[string]interface{})
	}

	// Get the actual workflow ID from database
	actualWorkflowID := agentID // fallback to agentID if query fails
	if s.repo != nil {
		workflow, err := s.repo.GetDraftWorkflow(ctx, agentID)
		if err == nil && workflow != nil {
			actualWorkflowID = workflow.ID
			logger.DebugContext(ctx, "draft workflow node workflow id loaded", "workflow_id", actualWorkflowID, "agent_id", agentID)
		} else {
			logger.WarnContext(ctx, "failed to load draft workflow node workflow id, using agent id fallback", "agent_id", agentID, err)
		}
	}

	// Add system variables to inputs
	// Create workflow run log for single node execution
	workflowRunLog, err := s.CreateWorkflowRunLog(ctx, workspaceID, agentID, actualWorkflowID, "debugging", inputs, accountID)
	var workflowRunLogID string
	var persistedWorkflowRunLogID string
	defer func() {
		s.cleanupWorkflowReusableSessionsWithTimeout(workflowRunLogID)
	}()
	if err != nil {
		logger.Error("Failed to create workflow run log for single node", err)
		// Use fallback ID if database creation fails
		workflowRunLogID = fmt.Sprintf("run-%s-%d", agentID, time.Now().UnixNano())
	} else if workflowRunLogTyped, ok := workflowRunLog.(*WorkflowRunLog); ok {
		workflowRunLogID = workflowRunLogTyped.ID
		persistedWorkflowRunLogID = workflowRunLogTyped.ID
	} else {
		// Use fallback ID if type assertion fails
		workflowRunLogID = fmt.Sprintf("run-%s-%d", agentID, time.Now().UnixNano())
	}

	inputs["sys.user_id"] = accountID
	inputs["sys.agent_id"] = agentID
	inputs["sys.tenant_id"] = workspaceID
	inputs["sys.workspace_id"] = workspaceID
	if organizationID := s.getOrganizationIDByWorkspace(ctx, workspaceID); organizationID != "" {
		inputs["sys.organization_id"] = organizationID
	}
	inputs["sys.workflow_id"] = actualWorkflowID // Use actual workflow ID
	inputs["sys.workflow_run_id"] = workflowRunLogID

	logger.DebugContext(ctx, "draft workflow node system variables added",
		"tenant_id", workspaceID,
		"agent_id", agentID,
		"account_id", accountID,
		"workflow_id", actualWorkflowID,
		"workflow_run_id", workflowRunLogID,
		"node_id", nodeID,
	)

	// Get node configuration from database instead of mock data
	nodeConfig, nodeType, err := s.getNodeConfigFromDatabase(ctx, agentID, nodeID)
	if err != nil {
		logger.Error("Failed to get node config from database", err)
		// Fallback to inferred node type and simple configuration
		nodeConfig = map[string]interface{}{
			"id":   nodeID,
			"type": "unknown",
			"data": map[string]interface{}{},
		}
	}

	// Create workflow node runtime log
	nodeTitle := nodeID
	if nodeData, ok := nodeConfig["data"].(map[string]interface{}); ok {
		if title, exists := nodeData["title"].(string); exists && title != "" {
			nodeTitle = title
		}
	}

	nodeLog, err := s.CreateWorkflowNodeRuntimeLog(
		ctx,
		workspaceID,
		agentID,
		actualWorkflowID,
		string(WorkflowNodeExecutionTriggeredFromSingleStep),
		persistedWorkflowRunLogID,
		nodeID,
		string(nodeType),
		nodeTitle,
		1,
		nil,
		inputs,
		accountID,
	)
	if err != nil {
		logger.Error("Failed to create workflow node runtime log", err)
		// Continue execution even if logging fails
	}

	// Execute single node
	startTime := time.Now()
	result, err := s.executor.ExecuteWorkflowNode(ctx, nodeID, nodeType, nodeConfig, inputs)
	elapsedTime := ElapsedMillisecondsSince(startTime)

	status, errorMsg, executionErr := singleNodeExecutionStatus(result, err)

	// Update node runtime log with results
	if nodeLog != nil {
		var outputs map[string]interface{}
		var processData map[string]interface{}
		var executionMetadata map[string]interface{}
		if result != nil {
			outputs = result.Outputs
			processData = result.ProcessData
			executionMetadata = workflowExecutionMetadataToMap(result.Metadata)
		}

		if nodeLogTyped, ok := nodeLog.(*WorkflowNodeRuntimeLog); ok {
			updateErr := s.UpdateWorkflowNodeRuntimeLog(ctx, nodeLogTyped.ID, status, outputs, processData, executionMetadata, elapsedTime, errorMsg)
			if updateErr != nil {
				logger.Error("Failed to update workflow node runtime log", updateErr)
			}
		}
	}

	// Update workflow run log with results
	if workflowRunLog != nil {
		if workflowRunLogTyped, ok := workflowRunLog.(*WorkflowRunLog); ok {
			nodeResults := make(map[string]interface{})
			if result != nil {
				nodeResults[nodeID] = map[string]interface{}{
					"status":  result.Status,
					"outputs": result.Outputs,
					"error":   errorMsg,
				}
			} else if errorMsg != "" {
				nodeResults[nodeID] = map[string]interface{}{
					"status":  "failed",
					"outputs": map[string]interface{}{},
					"error":   errorMsg,
				}
			}

			updateErr := s.UpdateWorkflowRunLogStatus(ctx, workflowRunLogTyped.ID, status, nodeResults, elapsedTime, 0, 1, errorMsg)
			if updateErr != nil {
				logger.Error("Failed to update workflow run log for single node", updateErr)
			}
		}
	}

	if executionErr != nil {
		logger.Error("Failed to execute workflow node", executionErr)
		return nil, executionErr
	}

	// Build response
	response := map[string]interface{}{
		"task_id": "task-" + nodeID,
		"status":  string(result.Status),
		"outputs": result.Outputs,
	}
	if persistedWorkflowRunLogID != "" {
		response["workflow_run_id"] = persistedWorkflowRunLogID
	}

	if result.ErrMsg != "" {
		response["error"] = result.ErrMsg
	}

	return response, nil
}

func singleNodeExecutionStatus(result *shared.NodeRunResult, err error) (status string, errorMsg string, returnErr error) {
	if err != nil {
		returnErr = fmt.Errorf("node execution failed: %w", err)
		return "failed", returnErr.Error(), returnErr
	}
	if result == nil {
		returnErr = fmt.Errorf("node execution failed: empty node result")
		return "failed", returnErr.Error(), returnErr
	}
	if result.ErrMsg != "" {
		return "failed", result.ErrMsg, nil
	}
	return "succeeded", "", nil
}

// getNodeConfigFromDatabase gets node configuration from database
func (s *WorkflowService) getNodeConfigFromDatabase(ctx context.Context, agentID, nodeID string) (map[string]interface{}, shared.NodeType, error) {
	// Return error if repository is not initialized
	if s.repo == nil {
		return nil, shared.Start, fmt.Errorf("workflow repository not initialized")
	}

	// Get draft workflow from database
	workflow, err := s.repo.GetDraftWorkflow(ctx, agentID)
	if err != nil {
		return nil, shared.Start, fmt.Errorf("failed to get draft workflow: %w", err)
	}

	// Get graph data from workflow
	graphDict := workflow.GetGraphDict()
	if graphDict == nil {
		return nil, shared.Start, fmt.Errorf("workflow graph is empty")
	}

	// Extract nodes from graph
	nodes, ok := graphDict["nodes"].([]interface{})
	if !ok {
		return nil, shared.Start, fmt.Errorf("invalid nodes format in workflow graph")
	}

	// Find the specific node
	for _, nodeInterface := range nodes {
		node, ok := nodeInterface.(map[string]interface{})
		if !ok {
			continue
		}

		if nodeIDValue, exists := node["id"]; exists {
			if nodeIDValue == nodeID {
				// ReactFlow nodes may use a renderer-level top-level type such as "custom".
				// The executable workflow node type is stored in data.type.
				nodeTypeStr, _ := node["type"].(string)
				nodeType := shared.NodeType(nodeTypeStr)
				if !shared.IsExecutableNodeType(nodeType) {
					if data, ok := node["data"].(map[string]interface{}); ok {
						if dataType, ok := data["type"].(string); ok && dataType != "" {
							nodeType = shared.NodeType(dataType)
						}
					}
				}
				if !shared.IsExecutableNodeType(nodeType) {
					return nil, shared.Start, fmt.Errorf("invalid node type for node %s", nodeID)
				}

				return node, nodeType, nil
			}
		}
	}

	return nil, shared.Start, fmt.Errorf("node %s not found in workflow", nodeID)
}

// GetExecutor returns the workflow executor
func (s *WorkflowService) GetExecutor() interface{} {
	return s.executor
}

func (s *WorkflowService) getDialogueCount(conversationID string) int {
	if conversationID == "" {
		return 1
	}

	conversationUUID, err := uuid.Parse(conversationID)
	if err != nil {
		return 1
	}

	db := database.GetDB()
	conversationRepo := conversation.NewAgentConversationRepository(db)

	ctx := context.Background()
	conv, err := conversationRepo.GetByID(ctx, conversationUUID)
	if err != nil {
		return 1
	}

	return conv.DialogueCount + 1
}

// Other method implementations...
func (s *WorkflowService) GetWorkflowConfig(ctx context.Context, workspaceID, agentID string) (interface{}, error) {
	_ = workspaceID
	return map[string]interface{}{
		"parallel_depth_limit": 10,
	}, nil
}

func (s *WorkflowService) RunAdvancedChatDraftWorkflow(ctx context.Context, workspaceID, agentID string, req interface{}, accountID string) (interface{}, error) {
	if advancedReq, ok := req.(*dto.AdvancedChatDraftWorkflowRunRequest); ok {
		draftReq := &dto.DraftWorkflowRunRequest{
			Inputs: advancedReq.Inputs,
		}

		if draftReq.Inputs == nil {
			draftReq.Inputs = make(map[string]interface{})
		}

		if advancedReq.ConversationID != "" {
			draftReq.Inputs["sys.conversation_id"] = advancedReq.ConversationID
			draftReq.Inputs["sys.dialogue_count"] = s.getDialogueCount(advancedReq.ConversationID)
		}
		if advancedReq.Query != "" {
			draftReq.Inputs["sys.query"] = advancedReq.Query
		}
		if draftReq.Inputs == nil {
			draftReq.Inputs = make(map[string]interface{})
		}
		draftReq.Inputs["sys.workflow_type"] = "chat"

		return s.RunDraftWorkflow(ctx, workspaceID, agentID, draftReq, accountID)
	}
	return nil, fmt.Errorf("invalid request type")
}

// RunAdvancedChatWorkflow runs advanced chat published workflow (non-streaming mode)
func (s *WorkflowService) RunAdvancedChatWorkflow(ctx context.Context, workspaceID, agentID string, req interface{}, accountID string) (interface{}, error) {
	if advancedReq, ok := req.(*dto.AdvancedChatDraftWorkflowRunRequest); ok {
		draftReq := &dto.DraftWorkflowRunRequest{
			Inputs: advancedReq.Inputs,
		}

		if draftReq.Inputs == nil {
			draftReq.Inputs = make(map[string]interface{})
		}

		if advancedReq.ConversationID != "" {
			draftReq.Inputs["sys.conversation_id"] = advancedReq.ConversationID
			draftReq.Inputs["sys.dialogue_count"] = s.getDialogueCount(advancedReq.ConversationID)
		}
		if advancedReq.Query != "" {
			draftReq.Inputs["sys.query"] = advancedReq.Query
		}
		if draftReq.Inputs == nil {
			draftReq.Inputs = make(map[string]interface{})
		}
		draftReq.Inputs["sys.workflow_type"] = "chat"

		invokeFrom, _ := ctx.Value("invoke_from").(string)
		createdFrom, _ := ctx.Value("created_from").(string)
		createdByRole, _ := ctx.Value("created_by_role").(string)

		if invokeFrom == "" {
			invokeFrom = string(InvokeFromWebApp)
		}
		if createdFrom == "" {
			createdFrom = "web-app"
		}
		if createdByRole == "" {
			createdByRole = "account"
		}

		draftReq.Inputs["conversation_params"] = map[string]interface{}{
			"from_source": createdByRole,
			"invoke_from": invokeFrom,
		}

		draftReq.Inputs["sys.tenant_id"] = workspaceID
		draftReq.Inputs["sys.workspace_id"] = workspaceID
		if organizationID := s.getOrganizationIDByWorkspace(ctx, workspaceID); organizationID != "" {
			draftReq.Inputs["sys.organization_id"] = organizationID
		}
		draftReq.Inputs["sys.agent_id"] = agentID
		draftReq.Inputs["sys.user_id"] = advancedReq.UserID

		return s.RunPublishedWorkflow(ctx, workspaceID, agentID, draftReq, accountID)
	}
	return nil, fmt.Errorf("invalid request type")
}

func (s *WorkflowService) StopWorkflowTask(ctx context.Context, tenantID, agentID, taskID string, accountID string) error {
	// Validate repos
	if s.workflowRunLogRepo == nil || s.workflowNodeRuntimeLogRepo == nil {
		return fmt.Errorf("workflow repositories not initialized")
	}

	// Try to get workflow run by ID (treat taskID as primary key ID)
	run, err := s.workflowRunLogRepo.GetByID(ctx, taskID)
	if err != nil {
		return fmt.Errorf("workflow run not found: %w", err)
	}

	// Verify ownership
	if run.TenantID != tenantID || run.AgentID != agentID {
		return fmt.Errorf("workflow run not found or access denied")
	}

	// Cancel the running workflow context first (this will trigger immediate stop)
	if s.CancelRunningWorkflow(taskID) {
		logger.Info("Successfully cancelled running workflow context for run: %s", taskID)
	} else {
		logger.Warn("No running workflow context found for run: %s (may have already completed)", taskID)
	}

	// Stop the workflow engine if it's running
	if s.executor != nil {
		if stopErr := s.executor.StopWorkflow(taskID); stopErr != nil {
			logger.Warn("Failed to stop workflow engine: %v", stopErr)
			// Continue with database update even if engine stop fails
		} else {
			logger.Info("Successfully stopped workflow engine for run: %s", taskID)
		}
	}

	// Update run status to stopped
	now := time.Now()
	if err := s.workflowRunLogRepo.UpdateStatus(ctx, run.ID, string(dto.WorkflowRunStatusStopped), &now); err != nil {
		return fmt.Errorf("failed to stop workflow run: %w", err)
	}

	// Stop any running node logs for this run
	nodeLogs, err := s.workflowNodeRuntimeLogRepo.GetByWorkflowRunID(ctx, run.ID)
	if err == nil {
		for _, nl := range nodeLogs {
			if string(nl.Status) == string(dto.NodeStatusRunning) {
				_ = s.workflowNodeRuntimeLogRepo.UpdateStatus(ctx, nl.ID, string(dto.WorkflowRunStatusStopped), &now)
			}
		}
	}

	s.cleanupWorkflowReusableSessionsWithTimeout(run.ID)

	// Unregister the engine and running workflow
	if s.executor != nil {
		s.executor.UnregisterEngine(taskID)
	}
	s.UnregisterRunningWorkflow(taskID)

	return nil
}

// GetWorkflowRunByID gets workflow run by ID
func (s *WorkflowService) GetWorkflowRunByID(ctx context.Context, runID string) (interface{}, error) {
	if s.workflowRunLogRepo == nil {
		return nil, fmt.Errorf("workflow run repository not initialized")
	}

	run, err := s.workflowRunLogRepo.GetByID(ctx, runID)
	if err != nil {
		return nil, fmt.Errorf("workflow run not found: %w", err)
	}

	return run, nil
}

func (s *WorkflowService) RunDraftIterationNode(ctx context.Context, tenantID, agentID, nodeID string, req interface{}, accountID string) (interface{}, error) {
	// Type assertion
	if draftReq, ok := req.(*dto.DraftWorkflowNodeRunRequest); ok {
		return s.RunDraftWorkflowNode(ctx, tenantID, agentID, nodeID, draftReq, accountID)
	}
	// Return error if type doesn't match
	return nil, fmt.Errorf("invalid request type")
}

func (s *WorkflowService) RunAdvancedChatDraftIterationNode(ctx context.Context, tenantID, agentID, nodeID string, req interface{}, accountID string) (interface{}, error) {
	// Type assertion
	if draftReq, ok := req.(*dto.DraftWorkflowNodeRunRequest); ok {
		return s.RunDraftWorkflowNode(ctx, tenantID, agentID, nodeID, draftReq, accountID)
	}
	// Return error if type doesn't match
	return nil, fmt.Errorf("invalid request type")
}

func (s *WorkflowService) RunAdvancedChatDraftLoopNode(ctx context.Context, tenantID, agentID, nodeID string, req interface{}, accountID string) (interface{}, error) {
	// Type assertion
	if draftReq, ok := req.(*dto.DraftWorkflowNodeRunRequest); ok {
		return s.RunDraftWorkflowNode(ctx, tenantID, agentID, nodeID, draftReq, accountID)
	}
	// Return error if type doesn't match
	return nil, fmt.Errorf("invalid request type")
}

func (s *WorkflowService) RunDraftLoopNode(ctx context.Context, tenantID, agentID, nodeID string, req interface{}, accountID string) (interface{}, error) {
	// Type assertion
	if draftReq, ok := req.(*dto.DraftWorkflowNodeRunRequest); ok {
		return s.RunDraftWorkflowNode(ctx, tenantID, agentID, nodeID, draftReq, accountID)
	}
	// Return error if type doesn't match
	return nil, fmt.Errorf("invalid request type")
}

func (s *WorkflowService) GetDraftWorkflowNodeLastRun(ctx context.Context, tenantID, agentID, nodeID string, accountID string) (interface{}, error) {
	return map[string]interface{}{
		"node_id": nodeID,
		"status":  "completed",
	}, nil
}

func (s *WorkflowService) PublishWorkflow(ctx context.Context, workspaceID, agentID string, req interface{}, accountID string) (interface{}, error) {
	// Extract description from request if provided
	description := ""
	if req != nil {
		if reqMap, ok := req.(map[string]interface{}); ok {
			if desc, exists := reqMap["description"]; exists {
				if descStr, ok := desc.(string); ok {
					description = descStr
				}
			}
		}
	}

	// Call the new PublishWorkflowWithDescription method
	response, err := s.PublishWorkflowWithDescription(ctx, workspaceID, agentID, accountID, description)
	if err != nil {
		return nil, err
	}

	// Convert response to interface{}
	return map[string]interface{}{
		"workflow_id": response.WorkflowID,
		"version":     response.Version,
		"created_at":  response.CreatedAt,
	}, nil
}

func (s *WorkflowService) GetPublishedWorkflows(ctx context.Context, tenantID, agentID string) (interface{}, error) {
	return map[string]interface{}{
		"data":     []map[string]interface{}{},
		"has_more": false,
		"limit":    10,
	}, nil
}

// GetLatestPublishedWorkflow gets the latest published workflow
// hideSecrets: if true, secret values will be replaced with SecretHiddenValue
func (s *WorkflowService) GetLatestPublishedWorkflow(ctx context.Context, requestedWorkspaceID, agentID string, hideSecrets ...bool) (interface{}, error) {
	// Return error if repository is not initialized
	if s.repo == nil {
		return nil, fmt.Errorf("workflow repository not initialized")
	}

	// Get latest published workflow from database
	workflow, err := s.repo.GetLatestPublishedWorkflow(ctx, agentID)
	if err != nil {
		logger.Error("Failed to get latest published workflow", err)
		return nil, fmt.Errorf("failed to get latest published workflow: %w", err)
	}

	if workflow == nil {
		return nil, fmt.Errorf("no published workflow found for agent %s", agentID)
	}

	// Hide or show environment variables based on hideSecrets parameter
	shouldHideSecrets := len(hideSecrets) > 0 && hideSecrets[0]
	workspaceID := effectiveWorkflowWorkspaceID(workflow, requestedWorkspaceID)

	// Build response
	response := map[string]interface{}{
		"id":                     workflow.ID,
		"tenant_id":              workspaceID,
		"workspace_id":           workspaceID,
		"agent_id":               workflow.AgentID,
		"type":                   string(workflow.Type),
		"version":                workflow.Version,
		"version_uuid":           workflowVersionSelectorID(workflow),
		"created_by":             workflow.CreatedBy,
		"created_at":             workflow.CreatedAt.Unix(),
		"updated_at":             workflow.UpdatedAt.Unix(),
		"graph":                  workflow.GetGraphDict(),
		"features":               workflow.GetFeaturesDict(),
		"environment_variables":  normalizeVariables(workflow.GetEnvironmentVariablesDict(), shouldHideSecrets),
		"conversation_variables": normalizeVariables(workflow.GetConversationVariablesDict()),
	}

	if workflow.UpdatedBy != nil {
		response["updated_by"] = *workflow.UpdatedBy
	}

	return response, nil
}

// RunPublishedWorkflow runs published workflow
func (s *WorkflowService) RunPublishedWorkflow(ctx context.Context, workspaceID, agentID string, req interface{}, accountID string) (interface{}, error) {
	// Get invoke_from, created_from, and created_by_role from context
	invokeFrom := "app-run" // Default
	if val := ctx.Value("invoke_from"); val != nil {
		if str, ok := val.(string); ok {
			invokeFrom = str
		}
	}
	createdFrom := "web-app" // Default
	if val := ctx.Value("created_from"); val != nil {
		if str, ok := val.(string); ok {
			createdFrom = str
		}
	}
	// createdByRole removed as live agents runtime log creation was removed

	logger.Info("Running published workflow", "agentID", agentID, "accountID", accountID, "invokeFrom", invokeFrom, "createdFrom", createdFrom)

	// Type assertion
	if draftReq, ok := req.(*dto.DraftWorkflowRunRequest); ok {
		// Get latest published workflow configuration from database
		workflowData, err := s.GetLatestPublishedWorkflow(ctx, workspaceID, agentID)
		if err != nil {
			logger.Error("Failed to get published workflow configuration", err)
			return nil, fmt.Errorf("failed to get published workflow configuration: %w", err)
		}

		// Parse workflow data
		workflowMap, ok := workflowData.(map[string]any)
		if !ok {
			logger.Error("Invalid workflow data format", fmt.Errorf("workflow data is not a map"))
			return nil, fmt.Errorf("invalid workflow data format")
		}

		graphData, err := mergeRootVariablesIntoGraph(workflowMap)
		if err != nil {
			logger.Error("Invalid graph data format", err)
			return nil, fmt.Errorf("invalid graph data format: %w", err)
		}

		// Resolve organization scope for workflow system variables.
		organizationID := ""
		if s.enterpriseService != nil {
			organization, err := s.enterpriseService.GetOrganizationByWorkspaceID(ctx, workspaceID)
			if err == nil && organization != nil {
				organizationID = organization.ID
			}
		}

		// Create workflow run log with invoke_from from context
		workflowRunLog, err := s.CreateWorkflowRunLog(ctx, workspaceID, agentID, agentID, invokeFrom, draftReq.Inputs, accountID)
		var workflowRunLogID string
		defer func() {
			s.cleanupWorkflowReusableSessionsWithTimeout(workflowRunLogID)
		}()
		if err != nil {
			logger.Error("Failed to create workflow run log", err)
			// Use fallback ID if database creation fails
			workflowRunLogID = fmt.Sprintf("run-%s-%d", agentID, time.Now().UnixNano())
		} else if workflowRunLogTyped, ok := workflowRunLog.(*WorkflowRunLog); ok {
			workflowRunLogID = workflowRunLogTyped.ID
		} else {
			workflowRunLogID = fmt.Sprintf("run-%s-%d", agentID, time.Now().UnixNano())
		}

		if draftReq.Inputs == nil {
			draftReq.Inputs = make(map[string]interface{})
		}
		if organizationID != "" {
			draftReq.Inputs["sys.organization_id"] = organizationID
		}

		// Execute workflow using executor
		result, err := s.executor.ExecuteSimpleWorkflowWithRunID(ctx, workflowRunLogID, graphData, draftReq.Inputs)

		if err != nil {
			logger.Error("Failed to execute published workflow", err)
			return nil, fmt.Errorf("failed to execute published workflow: %w", err)
		}

		// Update workflow run log with results
		if workflowRunLogID != "" {
			updateErr := s.UpdateWorkflowRunLogStatus(ctx, workflowRunLogID, "succeeded", result.NodeResults, workflowExecutionResultElapsedMilliseconds(result, 0), 0, 0, "")
			if updateErr != nil {
				logger.Error("Failed to update workflow run log", updateErr)
				// Don't return error here as workflow execution was successful
			}
		}

		// Live agents runtime log creation removed as requested

		return result, nil
	}

	// Return error if type doesn't match
	return nil, fmt.Errorf("invalid request type")
}

func (s *WorkflowService) GetWorkflowByID(ctx context.Context, workspaceID, agentID, workflowID string) (interface{}, error) {
	return map[string]interface{}{
		"id":           workflowID,
		"tenant_id":    workspaceID,
		"workspace_id": workspaceID,
		"agent_id":     agentID,
		"type":         "workflow",
		"version":      "1.0.0",
		"graph":        make(map[string]interface{}),
		"features":     make(map[string]interface{}),
		"created_by":   "system",
		"created_at":   time.Now(),
		"updated_by":   nil,
		"updated_at":   time.Now(),
	}, nil
}

func (s *WorkflowService) DeleteWorkflow(ctx context.Context, tenantID, agentID, workflowID string, accountID string) error {
	// Get workflow information before deletion
	var workflow *Workflow
	var err error

	if workflowID != "" {
		workflow, err = s.repo.GetByID(ctx, workflowID)
	} else {
		// If no workflowID provided, try to get draft workflow
		workflow, err = s.repo.GetDraftWorkflow(ctx, agentID)
	}

	if err != nil {
		logger.Error("Failed to get workflow for deletion", err)
		return fmt.Errorf("failed to get workflow: %w", err)
	}

	// Delete the workflow
	if err := s.repo.Delete(ctx, workflow.ID); err != nil {
		logger.Error("Failed to delete workflow", err)
		return fmt.Errorf("failed to delete workflow: %w", err)
	}

	// Record quota usage decrease after successful deletion
	if s.quotaService != nil && s.enterpriseService != nil {
		organization, err := s.enterpriseService.GetOrganizationByWorkspaceID(ctx, tenantID)
		if err == nil && organization != nil {
			organizationUUID, parseErr := uuid.Parse(organization.ID)
			if parseErr == nil {
				accountUUID, _ := uuid.Parse(accountID)
				tenantUUID, _ := uuid.Parse(tenantID)
				workflowName := fmt.Sprintf("Workflow-%s", workflow.ID[:8])

				// Create metadata
				metadata := quota_model.JSONMap{
					"workflow_id":   workflow.ID,
					"workflow_name": workflowName,
					"action":        "deleted",
				}

				record := &quota_model.QuotaUsageHistory{
					ID:           uuid.New().String(),
					GroupID:      organizationUUID,
					AccountID:    accountUUID,
					TenantID:     &tenantUUID,
					ResourceType: quota_model.ResourceTypeWorkflows,
					Delta:        -1, // Negative delta for deletion
					ResourceID:   &workflow.ID,
					ResourceName: &workflowName,
					Metadata:     &metadata,
				}

				if err := s.quotaService.RecordUsage(ctx, record); err != nil {
					logger.Error("Failed to record workflow quota usage decrease", err)
					// Don't fail the operation if quota recording fails
				}
			}
		}
	}

	return nil
}

func (s *WorkflowService) GetDefaultBlockConfigs(ctx context.Context, tenantID, agentID string) (interface{}, error) {
	return map[string]interface{}{
		"data": []map[string]interface{}{},
	}, nil
}

func (s *WorkflowService) GetDefaultBlockConfig(ctx context.Context, tenantID, agentID, blockType string) (interface{}, error) {
	return map[string]interface{}{
		"block_type":   blockType,
		"config":       map[string]interface{}{},
		"is_list_type": false,
	}, nil
}

func (s *WorkflowService) GetWorkflowVariables(ctx context.Context, tenantID, agentID string) (interface{}, error) {
	return map[string]interface{}{
		"data": []map[string]interface{}{},
	}, nil
}

func (s *WorkflowService) DeleteWorkflowVariables(ctx context.Context, tenantID, agentID string, accountID string) error {
	return nil
}

func (s *WorkflowService) GetNodeVariables(ctx context.Context, tenantID, agentID, nodeID string) (interface{}, error) {
	return map[string]interface{}{
		"data": []map[string]interface{}{},
	}, nil
}

func (s *WorkflowService) DeleteNodeVariables(ctx context.Context, tenantID, agentID, nodeID string, accountID string) error {
	return nil
}

func (s *WorkflowService) GetVariable(ctx context.Context, tenantID, agentID, variableID string) (interface{}, error) {
	return map[string]interface{}{
		"id":   variableID,
		"name": "variable",
		"type": "string",
	}, nil
}

func (s *WorkflowService) UpdateVariable(ctx context.Context, tenantID, agentID, variableID string, req interface{}, accountID string) (interface{}, error) {
	return map[string]interface{}{
		"id":   variableID,
		"name": "variable",
		"type": "string",
	}, nil
}

func (s *WorkflowService) DeleteVariable(ctx context.Context, tenantID, agentID, variableID string, accountID string) error {
	return nil
}

func (s *WorkflowService) ResetVariable(ctx context.Context, tenantID, agentID, variableID string, accountID string) (interface{}, error) {
	return map[string]interface{}{
		"id":   variableID,
		"name": "variable",
		"type": "string",
	}, nil
}

func (s *WorkflowService) GetConversationVariables(ctx context.Context, tenantID, agentID string) (interface{}, error) {
	return map[string]interface{}{
		"data": []map[string]interface{}{},
	}, nil
}

func (s *WorkflowService) GetSystemVariables(ctx context.Context, tenantID, agentID string) (interface{}, error) {
	workflow, err := s.repo.GetDraftWorkflow(ctx, agentID)
	if err != nil {
		return s.getDefaultSystemVariables("workflow"), nil
	}

	systemVars := []map[string]interface{}{
		{"name": "sys.user_id", "type": "string", "description": "User ID"},
		{"name": "sys.app_id", "type": "string", "description": "App ID"},
		{"name": "sys.workflow_id", "type": "string", "description": "Workflow ID"},
		{"name": "sys.tenant_id", "type": "string", "description": "Tenant ID"},
		{"name": "sys.workflow_run_id", "type": "string", "description": "Workflow Run ID"},
	}

	if workflow.Type == "chat" {
		chatVars := []map[string]interface{}{
			{"name": "sys.query", "type": "string", "description": "User query"},
			{"name": "sys.conversation_id", "type": "string", "description": "Conversation ID"},
			{"name": "sys.dialogue_count", "type": "number", "description": "Dialogue count"},
		}
		systemVars = append(systemVars, chatVars...)
	}

	return map[string]interface{}{
		"data": systemVars,
	}, nil
}

func inferValueType(value interface{}) string {
	switch value.(type) {
	case string:
		return "string"
	case int, int32, int64, float32, float64:
		return "number"
	case bool:
		return "boolean"
	case []interface{}:
		return "array"
	case map[string]interface{}:
		return "object"
	default:
		return "string"
	}
}

func (s *WorkflowService) GetEnvironmentVariables(ctx context.Context, tenantID, agentID string) (interface{}, error) {
	workflow, err := s.repo.GetDraftWorkflow(ctx, agentID)
	if err != nil {
		return map[string]interface{}{
			"data": []map[string]interface{}{},
		}, nil
	}

	envVars := workflow.GetEnvironmentVariablesDict()
	envVarList := make([]map[string]interface{}, 0)

	for _, v := range envVars {
		if varMap, ok := v.(map[string]interface{}); ok {
			name, ok := varMap["name"].(string)
			if !ok || name == "" {
				continue
			}
			value := varMap["value"]

			// Use value_type if provided, otherwise fall back to type
			valType := ""
			if vt, ok := varMap["value_type"].(string); ok && vt != "" {
				valType = vt
			} else if vt, ok := varMap["type"].(string); ok && vt != "" {
				valType = vt
			}

			if valType == "" {
				valType = inferValueType(value)
			}
			description, _ := varMap["description"].(string)

			// Hide secret values for API responses
			if valType == entities.SecretValueType {
				value = entities.SecretHiddenValue
			}

			envVarList = append(envVarList, map[string]interface{}{
				"name":        name,
				"value":       value,
				"type":        valType,
				"description": description,
			})
		}
	}

	return map[string]interface{}{
		"data": envVarList,
	}, nil
}

func (s *WorkflowService) GetAdvancedChatWorkflowRuns(ctx context.Context, tenantID, agentID string) (interface{}, error) {
	if s.workflowRunLogRepo == nil {
		return map[string]interface{}{
			"data": []map[string]interface{}{},
		}, nil
	}

	result := make([]map[string]interface{}, 0)

	return map[string]interface{}{
		"data": result,
	}, nil
}

func (s *WorkflowService) GetWorkflowRunNodeExecutions(ctx context.Context, tenantID, agentID, runID string) (interface{}, error) {
	if s.workflowNodeRuntimeLogRepo == nil {
		return nil, fmt.Errorf("workflow node runtime log repository not initialized")
	}

	// fetch logs ordered by index and created_at
	logs, err := s.workflowNodeRuntimeLogRepo.GetByWorkflowRunID(ctx, runID)
	if err != nil {
		return nil, fmt.Errorf("failed to get node runtime logs: %w", err)
	}

	// map to response DTOs
	results := make([]dto.WorkflowRunNodeExecutionResponse, 0, len(logs))
	for _, l := range logs {
		// Historically, tenant_id was overloaded for both organization and workspace.
		// Node runtime logs still persist the execution workspace in TenantID, so
		// agent ownership is the stable filter for legacy console paths.
		if l.AgentID != agentID {
			continue
		}

		inputsMap, _ := l.GetInputsDict()
		processDataMap, _ := l.GetProcessDataDict()
		outputsMap, _ := l.GetOutputsDict()

		// Filter inputs/outputs for frontend display (database retains full data)
		inputsMap = FilterFrontendInputs(l.NodeType, inputsMap)
		outputsMap = FilterFrontendOutputs(l.NodeType, outputsMap)
		execMetaMap, _ := l.GetExecutionMetadataDict()

		// marshal maps to raw messages
		marshal := func(m map[string]interface{}) json.RawMessage {
			if m == nil {
				return json.RawMessage([]byte("null"))
			}
			b, _ := json.Marshal(m)
			return json.RawMessage(b)
		}

		extras := json.RawMessage([]byte("{}"))
		predecessor := ""
		if l.PredecessorNodeID != nil {
			predecessor = *l.PredecessorNodeID
		}

		// graph & features from node runtime log
		var nodeGraph json.RawMessage = json.RawMessage([]byte("null"))
		if l.Graph != nil && *l.Graph != "" {
			nodeGraph = json.RawMessage([]byte(*l.Graph))
		}
		var nodeFeatures json.RawMessage = json.RawMessage([]byte("null"))
		if l.Features != nil && *l.Features != "" {
			nodeFeatures = json.RawMessage([]byte(*l.Features))
		}

		resp := dto.WorkflowRunNodeExecutionResponse{
			ID:                l.ID,
			Index:             l.Index,
			PredecessorNodeID: predecessor,
			NodeID:            l.NodeID,
			NodeType:          l.NodeType,
			Title:             l.Title,
			TriggeredFrom:     l.TriggeredFrom,
			Inputs:            marshal(inputsMap),
			ProcessData:       marshal(processDataMap),
			Outputs:           marshal(outputsMap),
			Graph:             nodeGraph,
			Features:          nodeFeatures,
			Status:            string(l.Status),
			Error:             getStringValue(l.Error),
			ElapsedTime:       workflowNodeElapsedMilliseconds(l),
			ErrorType:         getStringValue(l.ErrorType),
			DiagnosisResult:   getStringValue(l.DiagnosisResult),
			IsLLMDiagnosed:    l.IsLLMDiagnosed,
			ExecutionMetadata: marshal(execMetaMap),
			Extras:            extras,
			CreatedAt:         l.CreatedAt,
			CreatedByRole:     string(l.CreatedByRole),
			CreatedByAccount:  nil,
			CreatedByEndUser:  nil,
			FinishedAt:        l.FinishedAt,
		}

		// enrich created_by account if possible
		if s.accountService != nil && l.CreatedBy != "" {
			if acc, err := s.accountService.GetAccountByID(ctx, l.CreatedBy); err == nil && acc != nil {
				resp.CreatedByAccount = &dto.Account{ID: acc.ID, Name: acc.Name, Email: acc.Email}
			}
		}

		results = append(results, resp)
	}

	return &dto.WorkflowRunNodeExecutionListResponse{Data: results}, nil
}

func (s *WorkflowService) ConvertToWorkflow(ctx context.Context, tenantID, agentID string, req interface{}, accountID string) (interface{}, error) {
	return map[string]interface{}{
		"workflow_id": "converted-" + agentID,
		"status":      "converted",
	}, nil
}

func (s *WorkflowService) GetWorkflowAppLogs(ctx context.Context, tenantID, agentID string) (interface{}, error) {
	return map[string]interface{}{
		"data": []map[string]interface{}{},
	}, nil
}

func (s *WorkflowService) GetWorkflowDailyRuns(ctx context.Context, tenantID, agentID string) (interface{}, error) {
	return map[string]interface{}{
		"daily_runs": []map[string]interface{}{},
	}, nil
}

func (s *WorkflowService) GetWorkflowDailyTerminals(ctx context.Context, tenantID, agentID string) (interface{}, error) {
	return map[string]interface{}{
		"daily_terminals": []map[string]interface{}{},
	}, nil
}

func (s *WorkflowService) GetWorkflowDailyTokenCost(ctx context.Context, tenantID, agentID string) (interface{}, error) {
	return map[string]interface{}{
		"daily_token_costs": []map[string]interface{}{},
	}, nil
}

func (s *WorkflowService) GetWorkflowAverageAppInteraction(ctx context.Context, tenantID, agentID string) (interface{}, error) {
	return map[string]interface{}{
		"average_interactions": 0,
	}, nil
}

// CreateWorkflowRunLog creates a new workflow run log entry
func (s *WorkflowService) CreateWorkflowRunLog(ctx context.Context, tenantID, agentID, workflowID string, triggeredFrom string, inputs map[string]interface{}, accountID string) (interface{}, error) {
	if s.workflowRunLogRepo == nil {
		return nil, fmt.Errorf("workflow run log repository not initialized")
	}

	// Get next sequence number
	sequenceNumber, err := s.workflowRunLogRepo.GetNextSequenceNumber(ctx, tenantID, agentID)
	if err != nil {
		return nil, fmt.Errorf("failed to get next sequence number: %w", err)
	}

	// Marshal inputs to JSON
	inputsJSON, err := json.Marshal(inputs)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal inputs: %w", err)
	}

	// Determine which workflow version to use based on triggered_from
	var graphStr *string
	var featuresStr *string
	var version string
	var versionUUID *string
	var workflowType dto.WorkflowType = dto.WorkflowTypeWorkflow // Default to workflow

	if s.repo != nil {
		var wf *Workflow
		var err error

		// For debugging, use draft version
		if triggeredFrom == "debugging" {
			wf, err = s.repo.GetDraftWorkflow(ctx, agentID)
			version = "draft"
		} else {
			// For web-app and external-api, use published version
			wf, err = s.repo.GetLatestPublishedVersion(ctx, agentID)
			if err == nil && wf != nil {
				version = wf.Version
			} else {
				// Fallback to draft if no published version exists
				wf, err = s.repo.GetDraftWorkflow(ctx, agentID)
				version = "draft"
			}
		}

		if err == nil && wf != nil {
			// Get the actual workflow type from the workflow definition
			workflowType = wf.Type

			if wf.Graph != "" {
				g := wf.Graph
				graphStr = &g
			}
			if wf.Features != "" {
				f := wf.Features
				featuresStr = &f
			}
		}
	}

	// Default to "draft" if version is still empty
	if version == "" {
		version = "draft"
	}

	// Get created_by_role from context, default to "account" for internal calls
	createdByRole := CreatedByRoleAccount
	if role, ok := ctx.Value("created_by_role").(string); ok && role != "" {
		createdByRole = CreatedByRole(role)
	}

	// Create workflow run log
	inputsJSONStr := string(inputsJSON)
	workflowRunLog := &WorkflowRunLog{
		TenantID:        tenantID,
		AgentID:         agentID,
		SequenceNumber:  sequenceNumber,
		WorkflowID:      workflowID,
		Type:            workflowType, // Use actual workflow type from workflow definition
		TriggeredFrom:   triggeredFrom,
		Version:         version,
		WebAppID:        versionUUID, // Store version UUID in web_app_id field
		Graph:           graphStr,
		Inputs:          &inputsJSONStr,
		Status:          dto.WorkflowRunStatusRunning,
		ElapsedTime:     0,
		TotalTokens:     0,
		TotalSteps:      0,
		CreatedByRole:   createdByRole,
		CreatedBy:       accountID,
		CreatedAt:       time.Now(),
		ExceptionsCount: 0,
		Features:        featuresStr,
	}

	if err := s.workflowRunLogRepo.Create(ctx, workflowRunLog); err != nil {
		return nil, fmt.Errorf("failed to create workflow run log: %w", err)
	}

	return workflowRunLog, nil
}

// UpdateWorkflowRunLogStatus updates workflow run log status
func (s *WorkflowService) UpdateWorkflowRunLogStatus(ctx context.Context, workflowRunLogID string, status string, outputs map[string]interface{}, elapsedTime float64, totalTokens int64, totalSteps int, errorMsg string) error {
	if s.workflowRunLogRepo == nil {
		return fmt.Errorf("workflow run log repository not initialized")
	}

	finishedAt := time.Now()

	// Marshal outputs to JSON
	outputsJSON := "{}"
	if outputs != nil {
		if b, err := json.Marshal(outputs); err == nil {
			outputsJSON = string(b)
		}
	}

	// Update status and outputs
	if err := s.workflowRunLogRepo.UpdateStatus(ctx, workflowRunLogID, status, &finishedAt); err != nil {
		return fmt.Errorf("failed to update workflow run log status: %w", err)
	}

	// Update outputs and tokens
	if err := s.workflowRunLogRepo.UpdateOutputsAndTokens(ctx, workflowRunLogID, outputsJSON, totalTokens, elapsedTime); err != nil {
		return fmt.Errorf("failed to update workflow run log outputs: %w", err)
	}

	// Persist error message / total_steps / exceptions_count if applicable
	if run, err := s.workflowRunLogRepo.GetByID(ctx, workflowRunLogID); err == nil && run != nil {
		if errorMsg != "" {
			run.Error = &errorMsg
		}
		// Maintain total steps from caller if provided
		if totalSteps >= 0 {
			run.TotalSteps = totalSteps
		}
		// Increase exceptions count when run failed
		if status == string(dto.WorkflowRunStatusFailed) || status == "failed" {
			run.ExceptionsCount = run.ExceptionsCount + 1
		}
		if updateErr := s.workflowRunLogRepo.Update(ctx, run); updateErr != nil {
			return fmt.Errorf("failed to update workflow run log details: %w", updateErr)
		}
	}

	return nil
}

func (s *WorkflowService) PauseWorkflowRunLog(ctx context.Context, workflowRunLogID string, outputs map[string]interface{}, elapsedTime float64, totalTokens int64, totalSteps int) error {
	if s.workflowRunLogRepo == nil {
		return fmt.Errorf("workflow run log repository not initialized")
	}

	outputsJSON := "{}"
	if outputs != nil {
		if b, err := json.Marshal(outputs); err == nil {
			outputsJSON = string(b)
		}
	}

	if err := s.workflowRunLogRepo.UpdateStatus(ctx, workflowRunLogID, string(dto.WorkflowRunStatusPaused), nil); err != nil {
		return fmt.Errorf("failed to pause workflow run log: %w", err)
	}
	if err := s.workflowRunLogRepo.UpdateOutputsAndTokens(ctx, workflowRunLogID, outputsJSON, totalTokens, elapsedTime); err != nil {
		return fmt.Errorf("failed to update paused workflow outputs: %w", err)
	}
	if run, err := s.workflowRunLogRepo.GetByID(ctx, workflowRunLogID); err == nil && run != nil {
		run.TotalSteps = totalSteps
		if updateErr := s.workflowRunLogRepo.Update(ctx, run); updateErr != nil {
			return fmt.Errorf("failed to update paused workflow details: %w", updateErr)
		}
	}
	return nil
}

func (s *WorkflowService) ResumeWorkflowRunLog(ctx context.Context, workflowRunLogID string) error {
	if s.workflowRunLogRepo == nil {
		return fmt.Errorf("workflow run log repository not initialized")
	}
	if err := s.workflowRunLogRepo.UpdateStatus(ctx, workflowRunLogID, string(dto.WorkflowRunStatusRunning), nil); err != nil {
		return fmt.Errorf("failed to resume workflow run log: %w", err)
	}
	return nil
}

// CreateWorkflowNodeRuntimeLog creates a new workflow node runtime log entry
func (s *WorkflowService) CreateWorkflowNodeRuntimeLog(ctx context.Context, tenantID, agentID, workflowID, triggeredFrom, workflowRunID, nodeID, nodeType, title string, index int, predecessorNodeID *string, inputs map[string]interface{}, accountID string) (interface{}, error) {
	if s.workflowNodeRuntimeLogRepo == nil {
		return nil, fmt.Errorf("workflow node runtime log repository not initialized")
	}

	// Marshal inputs to JSON
	inputsJSONStr := "{}"
	if inputs != nil {
		inputsJSON, err := json.Marshal(inputs)
		if err != nil {
			logger.Error("failed to marshal inputs for workflow node runtime log, using empty fallback", "error", err)
		} else {
			inputsJSONStr = string(inputsJSON)
		}
	}

	// Include current workflow graph/features snapshot
	var graphStr *string
	var featuresStr *string
	if s.repo != nil {
		if wf, err := s.repo.GetDraftWorkflow(ctx, agentID); err == nil && wf != nil {
			if wf.Graph != "" {
				g := wf.Graph
				graphStr = &g
			}
			if wf.Features != "" {
				f := wf.Features
				featuresStr = &f
			}
		}
	}

	// Create workflow node runtime log
	var workflowRunIDPtr *string
	if workflowRunID != "" {
		workflowRunIDPtr = &workflowRunID
	} else {
		workflowRunIDPtr = nil
	}

	nodeLog := &WorkflowNodeRuntimeLog{
		TenantID:          tenantID,
		AgentID:           agentID,
		WorkflowID:        workflowID,
		TriggeredFrom:     triggeredFrom,
		WorkflowRunID:     workflowRunIDPtr,
		Index:             index,
		PredecessorNodeID: predecessorNodeID,
		NodeID:            nodeID,
		NodeType:          nodeType,
		Title:             title,
		Inputs:            &inputsJSONStr,
		Status:            "running",
		ElapsedTime:       0,
		CreatedByRole:     string(CreatedByRoleAccount),
		CreatedBy:         accountID,
		CreatedAt:         time.Now(),
		Graph:             graphStr,
		Features:          featuresStr,
	}

	if err := s.workflowNodeRuntimeLogRepo.Create(ctx, nodeLog); err != nil {
		return nil, fmt.Errorf("failed to create workflow node runtime log: %w", err)
	}

	return nodeLog, nil
}

// updateWorkflowNodeRuntimeLog updates workflow node runtime log with results
func (s *WorkflowService) UpdateWorkflowNodeRuntimeLog(ctx context.Context, nodeLogID string, status string, outputs map[string]interface{}, processData map[string]interface{}, executionMetadata map[string]interface{}, elapsedTime float64, errorMsg string) error {
	if s.workflowNodeRuntimeLogRepo == nil {
		return fmt.Errorf("workflow node runtime log repository not initialized")
	}

	finishedAt := time.Now()

	// Marshal data to JSON
	outputsJSON := ""
	if outputs != nil {
		if b, err := json.Marshal(outputs); err == nil {
			outputsJSON = string(b)
		}
	}

	processDataJSON := ""
	if processData != nil {
		if b, err := json.Marshal(processData); err == nil {
			processDataJSON = string(b)
		}
	}

	executionMetadataJSON := ""
	if executionMetadata != nil {
		if b, err := json.Marshal(executionMetadata); err == nil {
			executionMetadataJSON = string(b)
		}
	}

	// Update status
	if err := s.workflowNodeRuntimeLogRepo.UpdateStatus(ctx, nodeLogID, status, &finishedAt); err != nil {
		return fmt.Errorf("failed to update workflow node runtime log status: %w", err)
	}

	// Update outputs and metadata
	if err := s.workflowNodeRuntimeLogRepo.UpdateOutputsAndMetadata(ctx, nodeLogID, &outputsJSON, &processDataJSON, &executionMetadataJSON, elapsedTime); err != nil {
		return fmt.Errorf("failed to update workflow node runtime log outputs: %w", err)
	}

	if errorMsg != "" {
		if nodeLog, err := s.workflowNodeRuntimeLogRepo.GetByID(ctx, nodeLogID); err == nil && nodeLog != nil {
			nodeLog.Error = &errorMsg
			if updateErr := s.workflowNodeRuntimeLogRepo.Update(ctx, nodeLog); updateErr != nil {
				return fmt.Errorf("failed to update workflow node runtime log error: %w", updateErr)
			}
		}
	}

	return nil
}

func (s *WorkflowService) PauseWorkflowNodeRuntimeLog(ctx context.Context, nodeLogID string, outputs map[string]interface{}, processData map[string]interface{}, executionMetadata map[string]interface{}, elapsedTime float64) error {
	if s.workflowNodeRuntimeLogRepo == nil {
		return fmt.Errorf("workflow node runtime log repository not initialized")
	}

	outputsJSON := ""
	if outputs != nil {
		if b, err := json.Marshal(outputs); err == nil {
			outputsJSON = string(b)
		}
	}
	processDataJSON := ""
	if processData != nil {
		if b, err := json.Marshal(processData); err == nil {
			processDataJSON = string(b)
		}
	}
	executionMetadataJSON := ""
	if executionMetadata != nil {
		if b, err := json.Marshal(executionMetadata); err == nil {
			executionMetadataJSON = string(b)
		}
	}

	if err := s.workflowNodeRuntimeLogRepo.UpdateStatus(ctx, nodeLogID, string(dto.NodeStatusPaused), nil); err != nil {
		return fmt.Errorf("failed to pause workflow node runtime log: %w", err)
	}
	if err := s.workflowNodeRuntimeLogRepo.UpdateOutputsAndMetadata(ctx, nodeLogID, &outputsJSON, &processDataJSON, &executionMetadataJSON, elapsedTime); err != nil {
		return fmt.Errorf("failed to update paused workflow node outputs: %w", err)
	}
	return nil
}

// GetWorkflowRuns gets workflow runs for an agent.
func (s *WorkflowService) GetWorkflowRuns(ctx context.Context, agentID string, req *dto.WorkflowRunsRequest, appWorkspaceID string, accountID string) (*dto.WorkflowRunsResponse, error) {
	// Validate input parameters
	if s.workflowRunLogRepo == nil {
		return nil, fmt.Errorf("workflow run log repository not initialized")
	}

	// Set default values
	page := req.Page
	if page <= 0 {
		page = 1
	}
	limit := req.Limit
	if limit <= 0 {
		limit = 20
	}

	// Get workflow run logs from repository.
	logs, total, err := s.workflowRunLogRepo.GetByAgentID(ctx, agentID, page, limit, req.TriggeredFrom, appWorkspaceID, accountID)
	if err != nil {
		return nil, fmt.Errorf("failed to get workflow run logs: %w", err)
	}

	// Convert to response format
	workflowRuns := make([]dto.WorkflowRunLogResponse, len(logs))
	for i, log := range logs {
		// Get account information for created_by
		var createdByAccount *dto.CreatedByAccountResponse
		if s.accountService != nil {
			if account, err := s.accountService.GetAccountByID(ctx, log.CreatedBy); err == nil && account != nil {
				createdByAccount = &dto.CreatedByAccountResponse{
					ID:    account.ID,
					Name:  account.Name,
					Email: account.Email,
				}
			}
		}

		// Convert timestamps to Unix timestamps
		createdAt := log.CreatedAt.Unix()
		var finishedAt *int64
		if log.FinishedAt != nil {
			finishedAtUnix := log.FinishedAt.Unix()
			finishedAt = &finishedAtUnix
		}

		workflowRuns[i] = dto.WorkflowRunLogResponse{
			ID:               log.ID,
			SequenceNumber:   log.SequenceNumber,
			Version:          log.Version,
			TriggeredFrom:    log.TriggeredFrom,
			Status:           string(log.Status),
			ElapsedTime:      workflowRunElapsedMilliseconds(log),
			TotalTokens:      log.TotalTokens,
			TotalSteps:       log.TotalSteps,
			CreatedByAccount: createdByAccount,
			CreatedAt:        createdAt,
			FinishedAt:       finishedAt,
			ExceptionsCount:  log.ExceptionsCount,
			RetryIndex:       0, // Default retry index
		}
	}

	runMessages, err := s.getRunLinkedMessages(ctx, workflowRuns)
	if err != nil {
		return nil, fmt.Errorf("failed to load workflow run messages: %w", err)
	}
	for i := range workflowRuns {
		if linkedMessage, exists := runMessages[workflowRuns[i].ID]; exists {
			workflowRuns[i].ConversationID = newStringPointer(linkedMessage.ConversationID)
			workflowRuns[i].MessageID = newStringPointer(linkedMessage.MessageID)
		} else if conversationID := workflowRunInputConversationID(logs[i]); conversationID != "" {
			workflowRuns[i].ConversationID = newStringPointer(conversationID)
		} else if conversationID := s.workflowRunNodeInputConversationID(ctx, workflowRuns[i].ID); conversationID != "" {
			workflowRuns[i].ConversationID = newStringPointer(conversationID)
		}
		if workflowRuns[i].TotalSteps == 0 {
			if totalSteps := s.workflowRunNodeStepCount(ctx, workflowRuns[i].ID); totalSteps > 0 {
				workflowRuns[i].TotalSteps = totalSteps
			}
		}
	}

	// Calculate has_more
	hasMore := int64(page*limit) < total

	return &dto.WorkflowRunsResponse{
		Limit:   limit,
		HasMore: hasMore,
		Data:    workflowRuns,
	}, nil
}

// GetWorkflowRunDetail gets workflow run detail by run ID
func (s *WorkflowService) GetWorkflowRunDetail(ctx context.Context, tenantID, agentID, runID string) (*dto.WorkflowRunDetailResponse, error) {
	// Validate input parameters
	if s.workflowRunLogRepo == nil {
		return nil, fmt.Errorf("workflow run log repository not initialized")
	}

	// Get workflow run log from repository
	log, err := s.workflowRunLogRepo.GetByID(ctx, runID)
	if err != nil {
		return nil, fmt.Errorf("failed to get workflow run log: %w", err)
	}

	// Historically, tenant_id was overloaded for both organization and workspace.
	// Some console paths still pass organization_id via tenantID, while workflow run
	// logs persist the execution workspace in log.TenantID. For this legacy path,
	// agent ownership is the stable check until the remaining callers are migrated.
	if log.AgentID != agentID {
		return nil, fmt.Errorf("workflow run not found or access denied")
	}

	// Get account information for created_by
	var createdByAccount *dto.CreatedByAccountResponse
	if s.accountService != nil {
		if account, err := s.accountService.GetAccountByID(ctx, log.CreatedBy); err == nil && account != nil {
			createdByAccount = &dto.CreatedByAccountResponse{
				ID:    account.ID,
				Name:  account.Name,
				Email: account.Email,
			}
		}
	}

	// Convert timestamps to Unix timestamps
	createdAt := log.CreatedAt.Unix()
	var finishedAt *int64
	if log.FinishedAt != nil {
		finishedAtUnix := log.FinishedAt.Unix()
		finishedAt = &finishedAtUnix
	}

	// Prefer graph/features snapshot stored with the run log
	graph := make(map[string]interface{})
	if log.Graph != nil && *log.Graph != "" {
		_ = json.Unmarshal([]byte(*log.Graph), &graph)
	} else if s.repo != nil { // fallback
		if workflow, err := s.repo.GetByAgentID(ctx, tenantID, agentID); err == nil && workflow != nil {
			if workflow.Graph != "" {
				_ = json.Unmarshal([]byte(workflow.Graph), &graph)
			}
		}
	}

	// features snapshot if available
	features := make(map[string]interface{})
	if log.Features != nil && *log.Features != "" {
		_ = json.Unmarshal([]byte(*log.Features), &features)
	}

	var inputs map[string]interface{}
	if log.Inputs != nil && *log.Inputs != "" {
		if err := json.Unmarshal([]byte(*log.Inputs), &inputs); err != nil {
			// If parsing fails, return empty object
			inputs = make(map[string]interface{})
		}
	} else {
		inputs = make(map[string]interface{})
	}

	var outputs map[string]interface{}
	if log.Outputs != nil && *log.Outputs != "" {
		if err := json.Unmarshal([]byte(*log.Outputs), &outputs); err != nil {
			// If parsing fails, return empty object
			outputs = make(map[string]interface{})
		}
	} else {
		outputs = make(map[string]interface{})
	}

	// Determine created_by_role based on from_source
	createdByRole := "account" // default
	if log.CreatedByRole != "" {
		createdByRole = string(log.CreatedByRole)
	}

	var conversationID *string
	var messageID *string
	runMessages, err := s.getRunLinkedMessages(ctx, []dto.WorkflowRunLogResponse{{ID: log.ID}})
	if err != nil {
		return nil, fmt.Errorf("failed to load workflow run message: %w", err)
	}
	if linkedMessage, exists := runMessages[log.ID]; exists {
		conversationID = newStringPointer(linkedMessage.ConversationID)
		messageID = newStringPointer(linkedMessage.MessageID)
	} else if inputConversationID := workflowRunInputConversationID(*log); inputConversationID != "" {
		conversationID = newStringPointer(inputConversationID)
	} else if nodeInputConversationID := s.workflowRunNodeInputConversationID(ctx, log.ID); nodeInputConversationID != "" {
		conversationID = newStringPointer(nodeInputConversationID)
	}

	return &dto.WorkflowRunDetailResponse{
		ID:               log.ID,
		SequenceNumber:   log.SequenceNumber,
		Version:          log.Version,
		Graph:            graph,
		Features:         features,
		Inputs:           inputs,
		Status:           string(log.Status),
		Outputs:          outputs,
		Error:            getStringValue(log.Error),
		ElapsedTime:      workflowRunElapsedMilliseconds(*log),
		TotalTokens:      log.TotalTokens,
		TotalSteps:       s.workflowRunTotalSteps(ctx, *log),
		ConversationID:   conversationID,
		MessageID:        messageID,
		CreatedByRole:    createdByRole,
		CreatedByAccount: createdByAccount,
		CreatedByEndUser: nil, // TODO: implement end user support if needed
		CreatedAt:        createdAt,
		FinishedAt:       finishedAt,
		ExceptionsCount:  log.ExceptionsCount,
	}, nil
}

// getStringValue safely gets string value from pointer
func getStringValue(s *string) string {
	if s == nil {
		return ""
	}
	return *s
}

// getStringPointerValue safely returns pointer-to-string value; if nil returns nil
func getStringPointerValue(s *string) *string {
	if s == nil {
		return nil
	}
	v := *s
	return &v
}

func newStringPointer(value string) *string {
	return &value
}

func workflowRunElapsedMilliseconds(log WorkflowRunLog) float64 {
	return workflowStoredRunElapsedMilliseconds(log.ElapsedTime, log.CreatedAt, log.FinishedAt)
}

func workflowNodeElapsedMilliseconds(log WorkflowNodeRuntimeLog) float64 {
	return workflowStoredElapsedMilliseconds(log.ElapsedTime, log.CreatedAt, log.FinishedAt)
}

const workflowInternalElapsedTimeKey = "__workflow_elapsed_time__"

type workflowElapsedTracker struct {
	total       float64
	nodeElapsed map[string]float64
}

func newWorkflowElapsedTrackerFromNodeLogs(nodeLogs []WorkflowNodeRuntimeLog) *workflowElapsedTracker {
	return newWorkflowElapsedTrackerFromNodeLogsWithPausedPolicy(nodeLogs, false)
}

func newWorkflowElapsedTrackerFromNodeLogsWithPausedPolicy(nodeLogs []WorkflowNodeRuntimeLog, includePaused bool) *workflowElapsedTracker {
	tracker := &workflowElapsedTracker{
		nodeElapsed: make(map[string]float64),
	}
	for _, nodeLog := range nodeLogs {
		if !includePaused && nodeLog.Status == string(dto.NodeStatusPaused) {
			continue
		}
		tracker.recordNodeElapsed(nodeLog.ID, workflowNodeElapsedMilliseconds(nodeLog))
	}
	return tracker
}

func (s *WorkflowService) newWorkflowElapsedTracker(ctx context.Context, workflowRunID string) *workflowElapsedTracker {
	if s == nil || s.workflowNodeRuntimeLogRepo == nil || workflowRunID == "" {
		return newWorkflowElapsedTrackerFromNodeLogs(nil)
	}
	nodeLogs, err := s.workflowNodeRuntimeLogRepo.GetByWorkflowRunID(ctx, workflowRunID)
	if err != nil {
		return newWorkflowElapsedTrackerFromNodeLogs(nil)
	}
	return newWorkflowElapsedTrackerFromNodeLogs(nodeLogs)
}

func (t *workflowElapsedTracker) recordNodeElapsed(nodeLogID string, elapsed float64) float64 {
	if t == nil {
		return 0
	}
	if elapsed <= 0 {
		return t.total
	}
	if nodeLogID == "" {
		t.total += elapsed
		t.total = roundElapsedMilliseconds(t.total)
		return t.total
	}
	if previous, exists := t.nodeElapsed[nodeLogID]; exists {
		t.total -= previous
	}
	t.nodeElapsed[nodeLogID] = elapsed
	t.total += elapsed
	t.total = roundElapsedMilliseconds(t.total)
	return t.total
}

func (t *workflowElapsedTracker) elapsedOrFallback(fallback float64) float64 {
	if t != nil && t.total > 0 {
		return roundElapsedMilliseconds(t.total)
	}
	return roundElapsedMilliseconds(fallback)
}

func workflowElapsedMillisecondsFromOutputs(outputs map[string]interface{}, fallback float64) float64 {
	if outputs == nil {
		return roundElapsedMilliseconds(fallback)
	}
	raw, exists := outputs[workflowInternalElapsedTimeKey]
	if !exists {
		return roundElapsedMilliseconds(fallback)
	}
	delete(outputs, workflowInternalElapsedTimeKey)
	switch value := raw.(type) {
	case int:
		if value > 0 {
			return roundElapsedMilliseconds(float64(value))
		}
	case int64:
		if value > 0 {
			return roundElapsedMilliseconds(float64(value))
		}
	case float64:
		if value > 0 {
			return roundElapsedMilliseconds(value)
		}
	}
	return roundElapsedMilliseconds(fallback)
}

func workflowExecutionResultElapsedMilliseconds(result *WorkflowExecutionResult, fallback float64) float64 {
	if result == nil {
		return roundElapsedMilliseconds(fallback)
	}
	if elapsed := workflowNodeResultsElapsedMilliseconds(result.NodeResults); elapsed > 0 {
		return elapsed
	}
	if result.ExecutionTime > 0 {
		return durationMilliseconds(result.ExecutionTime)
	}
	return roundElapsedMilliseconds(fallback)
}

// WorkflowElapsedMillisecondsFromResult returns node-sum workflow elapsed time in milliseconds.
func WorkflowElapsedMillisecondsFromResult(result interface{}, fallback float64) float64 {
	switch typed := result.(type) {
	case *WorkflowExecutionResult:
		return workflowExecutionResultElapsedMilliseconds(typed, fallback)
	case map[string]interface{}:
		if elapsed := workflowElapsedMillisecondsFromOutputs(typed, 0); elapsed > 0 {
			return elapsed
		}
		if nodeResults, ok := typed["NodeResults"].(map[string]interface{}); ok {
			if elapsed := workflowNodeResultsElapsedMilliseconds(nodeResults); elapsed > 0 {
				return elapsed
			}
		}
		if nodeResults, ok := typed["node_results"].(map[string]interface{}); ok {
			if elapsed := workflowNodeResultsElapsedMilliseconds(nodeResults); elapsed > 0 {
				return elapsed
			}
		}
		if elapsed := workflowNodeResultsElapsedMilliseconds(typed); elapsed > 0 {
			return elapsed
		}
	}
	return roundElapsedMilliseconds(fallback)
}

func buildBlockingWorkflowRunResponse(agentID string, workflowRunID string, result *WorkflowExecutionResult, elapsedTime float64) map[string]interface{} {
	response := map[string]interface{}{
		"task_id":         "task-" + agentID,
		"workflow_run_id": workflowRunID,
		"elapsed_time":    elapsedTime,
	}
	outputs := workflowExecutionOutputs(result)
	if len(outputs) > 0 {
		response["outputs"] = outputs
		if answer := extractWorkflowAnswer(outputs); answer != "" {
			response["answer"] = answer
		}
	} else if result != nil && len(result.NodeResults) > 0 {
		if answer := extractWorkflowAnswer(result.NodeResults); answer != "" {
			response["answer"] = answer
		}
	}
	if result != nil && len(result.NodeResults) > 0 {
		response["node_results"] = result.NodeResults
	}
	if result != nil {
		if result.Status != "" {
			response["status"] = result.Status
		}
		if errorMessage := workflowExecutionErrorMessage(result); errorMessage != "" {
			response["error"] = errorMessage
		}
		if nodeErrors := workflowExecutionNodeErrors(result); len(nodeErrors) > 0 {
			response["node_errors"] = nodeErrors
		}
	}
	return response
}

func workflowExecutionLogStatusAndError(result *WorkflowExecutionResult, err error) (string, string) {
	if err != nil {
		return "failed", err.Error()
	}
	if result == nil {
		return "succeeded", ""
	}
	if result.Status == "failed" {
		return "failed", workflowExecutionErrorMessage(result)
	}
	if result.Status == "paused" {
		return "paused", workflowExecutionErrorMessage(result)
	}
	if result.Status != "" {
		return result.Status, workflowExecutionErrorMessage(result)
	}
	return "succeeded", workflowExecutionErrorMessage(result)
}

func workflowExecutionErrorMessage(result *WorkflowExecutionResult) string {
	if result == nil {
		return ""
	}
	if result.Error != nil {
		return result.Error.Error()
	}
	nodeErrors := workflowExecutionNodeErrors(result)
	if len(nodeErrors) == 0 {
		return ""
	}
	for nodeID, errorMessage := range nodeErrors {
		if errorMessage != "" {
			return fmt.Sprintf("node %s failed: %s", nodeID, errorMessage)
		}
	}
	return ""
}

func workflowExecutionNodeErrors(result *WorkflowExecutionResult) map[string]string {
	if result == nil {
		return nil
	}
	nodeErrors := map[string]string{}
	for _, snapshot := range result.NodeExecutions {
		if snapshot.Error != "" {
			nodeErrors[snapshot.NodeID] = snapshot.Error
		}
	}
	for nodeID, raw := range result.NodeResults {
		nodeResult, ok := raw.(map[string]interface{})
		if !ok {
			continue
		}
		status, _ := nodeResult["status"].(string)
		if status != "failed" && status != "exception" {
			continue
		}
		if _, exists := nodeErrors[nodeID]; exists {
			continue
		}
		if errorMessage, ok := nodeResult["error"].(string); ok && errorMessage != "" {
			nodeErrors[nodeID] = errorMessage
		}
	}
	if len(nodeErrors) == 0 {
		return nil
	}
	return nodeErrors
}

func workflowExecutionOutputs(result *WorkflowExecutionResult) map[string]interface{} {
	if result == nil {
		return nil
	}
	if result.RuntimeState != nil {
		if snapshot := result.RuntimeState.OutputsSnapshot(); len(snapshot) > 0 {
			outputs := make(map[string]interface{}, len(snapshot))
			for key, value := range snapshot {
				outputs[key] = value
			}
			return outputs
		}
	}
	if len(result.NodeResults) == 0 {
		return nil
	}
	outputs := make(map[string]interface{})
	for _, nodeResult := range result.NodeResults {
		nodeOutput, ok := nodeResult.(map[string]interface{})
		if !ok {
			continue
		}
		for key, value := range nodeOutput {
			if key == "status" || key == "error" || key == "startTime" || key == "endTime" {
				continue
			}
			outputs[key] = value
		}
	}
	return outputs
}

func workflowNodeResultsElapsedMilliseconds(nodeResults map[string]interface{}) float64 {
	if len(nodeResults) == 0 {
		return 0
	}
	var total float64
	for _, raw := range nodeResults {
		nodeResult, ok := raw.(map[string]interface{})
		if !ok {
			continue
		}
		startTime, hasStart := workflowTimeValue(nodeResult["startTime"])
		endTime, hasEnd := workflowTimeValue(nodeResult["endTime"])
		if !hasStart || !hasEnd || !endTime.After(startTime) {
			continue
		}
		total += durationMilliseconds(endTime.Sub(startTime))
	}
	return roundElapsedMilliseconds(total)
}

func workflowTimeValue(value interface{}) (time.Time, bool) {
	switch typed := value.(type) {
	case time.Time:
		return typed, !typed.IsZero()
	case *time.Time:
		if typed == nil || typed.IsZero() {
			return time.Time{}, false
		}
		return *typed, true
	case string:
		parsed, err := time.Parse(time.RFC3339Nano, typed)
		return parsed, err == nil && !parsed.IsZero()
	default:
		return time.Time{}, false
	}
}

// WorkflowRunElapsedMillisecondsForEvent returns node-sum workflow elapsed time in milliseconds.
func (s *WorkflowService) WorkflowRunElapsedMillisecondsForEvent(ctx context.Context, workflowRunID string, fallback float64) float64 {
	return s.workflowRunElapsedMillisecondsForEvent(ctx, workflowRunID, fallback)
}

func (s *WorkflowService) workflowRunTotalSteps(ctx context.Context, log WorkflowRunLog) int {
	if log.TotalSteps > 0 {
		return log.TotalSteps
	}
	return s.workflowRunNodeStepCount(ctx, log.ID)
}

func workflowRunInputConversationID(log WorkflowRunLog) string {
	if log.Inputs == nil || *log.Inputs == "" {
		return ""
	}
	return inputMapConversationID(*log.Inputs)
}

func (s *WorkflowService) workflowRunNodeInputConversationID(ctx context.Context, workflowRunID string) string {
	if s == nil || s.workflowNodeRuntimeLogRepo == nil || workflowRunID == "" {
		return ""
	}

	nodeLogs, err := s.workflowNodeRuntimeLogRepo.GetByWorkflowRunID(ctx, workflowRunID)
	if err != nil {
		return ""
	}
	for _, nodeLog := range nodeLogs {
		if nodeLog.Inputs == nil || *nodeLog.Inputs == "" {
			continue
		}

		conversationID := inputMapConversationID(*nodeLog.Inputs)
		if conversationID != "" {
			return conversationID
		}
	}
	return ""
}

func (s *WorkflowService) workflowRunNodeElapsedMilliseconds(ctx context.Context, workflowRunID string) float64 {
	if s == nil || s.workflowNodeRuntimeLogRepo == nil || workflowRunID == "" {
		return 0
	}

	nodeLogs, err := s.workflowNodeRuntimeLogRepo.GetByWorkflowRunID(ctx, workflowRunID)
	if err != nil {
		return 0
	}

	return newWorkflowElapsedTrackerFromNodeLogsWithPausedPolicy(nodeLogs, true).elapsedOrFallback(0)
}

func (s *WorkflowService) workflowRunElapsedMillisecondsForEvent(ctx context.Context, workflowRunID string, fallback float64) float64 {
	elapsed := s.workflowRunNodeElapsedMilliseconds(ctx, workflowRunID)
	if elapsed > 0 {
		return elapsed
	}
	return roundElapsedMilliseconds(fallback)
}

func (s *WorkflowService) workflowRunNodeStepCount(ctx context.Context, workflowRunID string) int {
	if s == nil || s.workflowNodeRuntimeLogRepo == nil || workflowRunID == "" {
		return 0
	}

	nodeLogs, err := s.workflowNodeRuntimeLogRepo.GetByWorkflowRunID(ctx, workflowRunID)
	if err != nil {
		return 0
	}
	return len(nodeLogs)
}

func inputMapConversationID(rawInputs string) string {
	var inputs map[string]interface{}
	if err := json.Unmarshal([]byte(rawInputs), &inputs); err != nil {
		return ""
	}

	if conversationID, ok := inputs["sys.conversation_id"].(string); ok {
		return conversationID
	}
	if conversationID, ok := inputs["conversation_id"].(string); ok {
		return conversationID
	}
	return ""
}

func (s *WorkflowService) getRunLinkedMessages(ctx context.Context, runs []dto.WorkflowRunLogResponse) (map[string]*runLinkedMessage, error) {
	result := make(map[string]*runLinkedMessage)
	if s == nil || s.workflowRunMessageLookup == nil || len(runs) == 0 {
		return result, nil
	}

	runIDs := make([]string, 0, len(runs))
	for _, run := range runs {
		if run.ID == "" {
			continue
		}
		runIDs = append(runIDs, run.ID)
	}
	if len(runIDs) == 0 {
		return result, nil
	}

	messages, err := s.workflowRunMessageLookup.GetFirstMessagesByWorkflowRunIDs(ctx, runIDs)
	if err != nil {
		return nil, err
	}

	for runID, message := range messages {
		if message == nil {
			continue
		}
		result[runID] = &runLinkedMessage{
			MessageID:      message.ID.String(),
			ConversationID: message.ConversationID.String(),
		}
	}

	return result, nil
}

func (s *WorkflowService) getDefaultSystemVariables(workflowType string) map[string]interface{} {
	systemVars := []map[string]interface{}{
		{"name": "sys.user_id", "type": "string", "description": "User ID"},
		{"name": "sys.app_id", "type": "string", "description": "App ID"},
		{"name": "sys.workflow_id", "type": "string", "description": "Workflow ID"},
		{"name": "sys.tenant_id", "type": "string", "description": "Tenant ID"},
		{"name": "sys.workflow_run_id", "type": "string", "description": "Workflow Run ID"},
	}

	if workflowType == "chat" {
		chatVars := []map[string]interface{}{
			{"name": "sys.query", "type": "string", "description": "User query"},
			{"name": "sys.conversation_id", "type": "string", "description": "Conversation ID"},
			{"name": "sys.dialogue_count", "type": "number", "description": "Dialogue count"},
		}
		systemVars = append(systemVars, chatVars...)
	}

	return map[string]interface{}{
		"data": systemVars,
	}
}

// RunWorkflowByVersionUUID runs a specific version of workflow by version UUID (legacy method)
func (s *WorkflowService) RunWorkflowByVersionUUID(ctx context.Context, versionUUID string, req interface{}, accountID string) (interface{}, error) {
	logger.Info("Running workflow by web_app_id (legacy)", "webAppID", versionUUID, "accountID", accountID)

	// Type assertion
	draftReq, ok := req.(*dto.DraftWorkflowRunRequest)
	if !ok {
		return nil, fmt.Errorf("invalid request type")
	}

	// Get workflow by version UUID
	workflow, err := s.repo.GetByVersionUUID(ctx, versionUUID)
	if err != nil {
		logger.Error("Failed to get workflow by web_app_id (legacy)", err)
		return nil, fmt.Errorf("failed to get workflow by version UUID: %w", err)
	}

	tenantID := workflow.TenantID
	agentID := workflow.AgentID

	// Parse graph and merge root environment/conversation variables.
	workflowMap := map[string]any{
		"graph":                  workflow.GetGraphDict(),
		"environment_variables":  normalizeVariables(workflow.GetEnvironmentVariablesDict()),
		"conversation_variables": normalizeVariables(workflow.GetConversationVariablesDict()),
	}
	graphData, err := mergeRootVariablesIntoGraph(workflowMap)
	if err != nil || graphData == nil || len(graphData) == 0 {
		logger.Error("Invalid graph data", fmt.Errorf("graph is empty"))
		return nil, fmt.Errorf("invalid graph data")
	}

	// Handle conversation workflow if needed
	var conversationID *string
	var workflowRunIDStr string
	var workflowRunLogID string
	defer func() {
		s.cleanupWorkflowReusableSessionsWithTimeout(workflowRunLogID)
	}()

	// Check if this is a conversation workflow
	if workflow.Type == "chat" {
		// Extract conversation parameters from inputs
		fromSource := "account"                // default
		invokeFrom := string(InvokeFromWebApp) // For web app calls
		var overrideModelConfigs *string

		// Check if we have conversation parameters in the request
		if convParams, ok := draftReq.Inputs["conversation_params"].(map[string]any); ok {
			if fs, ok := convParams["from_source"].(string); ok {
				fromSource = fs
			}
			if iv, ok := convParams["invoke_from"].(string); ok {
				invokeFrom = iv
			}
			if omc, ok := convParams["override_model_configs"].(string); ok {
				overrideModelConfigs = &omc
			}
		}

		// Get or create conversation
		var convIDStr string
		logger.Info("Checking for existing conversation ID", "sys.conversation_id", draftReq.Inputs["sys.conversation_id"])
		if convID, exists := draftReq.Inputs["sys.conversation_id"].(string); exists && convID != "" {
			convIDStr = convID
			// Update existing conversation with web_app_id
			logger.Info("Updating existing conversation with web_app_id", "conversationID", convIDStr, "webAppID", versionUUID)
			err := s.advancedChatHandler.UpdateConversationWebAppID(convIDStr, versionUUID)
			if err != nil {
				logger.Warn("Failed to update conversation web_app_id", "error", err)
				// Don't fail the workflow execution, just log the warning
			}
		} else {
			// Create new conversation with invoke_from = "web-app" and web_app_id = versionUUID
			logger.Info("Creating NEW conversation with web_app_id", "webAppID", versionUUID, "fromSource", fromSource, "invokeFrom", invokeFrom)
			logger.Info("Inputs for new conversation", "inputs", draftReq.Inputs)
			newConvID, err := s.advancedChatHandler.CreateConversationRecordWithParams(
				tenantID, agentID, accountID, fromSource, invokeFrom, draftReq.Inputs, overrideModelConfigs, &versionUUID)
			if err != nil {
				logger.Error("Failed to create conversation", err)
				return nil, fmt.Errorf("failed to create conversation: %w", err)
			}
			convIDStr = newConvID
		}

		conversationID = &convIDStr
		// Add conversation ID to inputs for workflow execution
		draftReq.Inputs["sys.conversation_id"] = convIDStr
	}

	// Resolve organization scope for workflow system variables.
	organizationID := ""
	if s.enterpriseService != nil {
		organization, err := s.enterpriseService.GetOrganizationByWorkspaceID(ctx, tenantID)
		if err == nil && organization != nil {
			organizationID = organization.ID
		}
	}

	// Create workflow run log with triggered_from = "web-app" and workflow_version_uuid = versionUUID
	workflowRunLog, err := s.CreateWorkflowRunLogWithVersion(ctx, tenantID, agentID, workflow.ID, "web-app", versionUUID, draftReq.Inputs, accountID)
	if err != nil {
		logger.Error("Failed to create workflow run log", err)
		// Continue execution even if logging fails
	} else if workflowRunLogTyped, ok := workflowRunLog.(*WorkflowRunLog); ok {
		workflowRunLogID = workflowRunLogTyped.ID
		workflowRunIDStr = workflowRunLogID
	}

	// Add system variables to inputs for workflow execution
	if draftReq.Inputs == nil {
		draftReq.Inputs = make(map[string]interface{})
	}
	draftReq.Inputs["sys.tenant_id"] = tenantID
	draftReq.Inputs["sys.workspace_id"] = tenantID
	if organizationID != "" {
		draftReq.Inputs["sys.organization_id"] = organizationID
	}
	draftReq.Inputs["sys.agent_id"] = agentID
	draftReq.Inputs["sys.workflow_id"] = workflow.ID
	draftReq.Inputs["sys.workflow_run_id"] = workflowRunLogID
	draftReq.Inputs["sys.user_id"] = accountID
	// Get query from inputs if exists
	if query, ok := draftReq.Inputs["query"].(string); ok && query != "" {
		draftReq.Inputs["sys.query"] = query
	}
	if conversationID != nil {
		draftReq.Inputs["sys.conversation_id"] = *conversationID
	}

	// Execute workflow with actual graph data
	startTime := time.Now()
	executionResult, err := s.executor.ExecuteSimpleWorkflowWithRunID(ctx, workflowRunLogID, graphData, draftReq.Inputs)
	elapsedTime := workflowExecutionResultElapsedMilliseconds(executionResult, ElapsedMillisecondsSince(startTime))

	// Update workflow run log with results
	if workflowRunLog != nil {
		status := "succeeded"
		if err != nil {
			status = "failed"
		}

		if workflowRunLogTyped, ok := workflowRunLog.(*WorkflowRunLog); ok {
			nodeResults := map[string]interface{}{}
			if executionResult != nil {
				nodeResults = executionResult.NodeResults
			}
			updateErr := s.UpdateWorkflowRunLogStatus(ctx, workflowRunLogTyped.ID, status, nodeResults, elapsedTime, 0, len(nodeResults), "")
			if updateErr != nil {
				logger.Error("Failed to update workflow run log", updateErr)
			}
		}
	}

	if err != nil {
		logger.Error("Failed to execute workflow", err)
		return nil, fmt.Errorf("workflow execution failed: %w", err)
	}

	// For conversation workflows, create message records
	if conversationID != nil {
		// Extract query and answer from inputs and outputs
		query := ""
		if q, exists := draftReq.Inputs["sys.query"].(string); exists {
			query = q
		} else if q, exists := draftReq.Inputs["query"].(string); exists {
			query = q
		}

		answer := ""
		// Try different possible output keys based on node types
		if textOutput, exists := executionResult.NodeResults["text"].(string); exists {
			answer = textOutput
		} else if answerOutput, exists := executionResult.NodeResults["answer"].(string); exists {
			answer = answerOutput
		} else if endNodeOutputs, ok := executionResult.NodeResults["end"]; ok {
			if endOutputsMap, ok := endNodeOutputs.(map[string]interface{}); ok {
				if textOutput, exists := endOutputsMap["text"].(string); exists {
					answer = textOutput
				} else if answerOutput, exists := endOutputsMap["answer"].(string); exists {
					answer = answerOutput
				}
			}
		} else {
			// Try to find answer from any node output
			for _, nodeOutput := range executionResult.NodeResults {
				if nodeOutputMap, ok := nodeOutput.(map[string]interface{}); ok {
					if nodeText, exists := nodeOutputMap["text"].(string); exists {
						answer = nodeText
						break
					} else if nodeAnswer, exists := nodeOutputMap["answer"].(string); exists {
						answer = nodeAnswer
						break
					}
				}
			}
		}

		// Parse UUIDs
		agentUUID, err := uuid.Parse(agentID)
		if err != nil {
			logger.Error("Invalid agent ID", err)
			return nil, fmt.Errorf("invalid agent ID: %w", err)
		}

		conversationUUID, err := uuid.Parse(*conversationID)
		if err != nil {
			logger.Error("Invalid conversation ID", err)
			return nil, fmt.Errorf("invalid conversation ID: %w", err)
		}

		workflowRunUUID, err := uuid.Parse(workflowRunIDStr)
		if err != nil {
			logger.Error("Invalid workflow run ID", err)
			return nil, fmt.Errorf("invalid workflow run ID: %w", err)
		}

		accountUUID, err := uuid.Parse(accountID)
		if err != nil {
			logger.Error("Invalid account ID", err)
			return nil, fmt.Errorf("invalid account ID: %w", err)
		}

		// Create message record with invoke_from = "web-app" and workflow_version_uuid = versionUUID
		_, err = s.advancedChatHandler.CreateWorkflowMessage(
			agentUUID,
			conversationUUID,
			workflowRunUUID,
			query,
			answer,
			"account",
			string(InvokeFromWebApp), // invokeFrom - web-app for version-specific calls
			accountUUID,
			&accountUUID,
			&versionUUID, // workflowVersionUUID - set to the version UUID
		)
		if err != nil {
			logger.Error("Failed to create workflow message", err)
			// Don't fail the entire workflow execution if message creation fails
		}
	}

	// Build response
	response := map[string]interface{}{
		"task_id":         "task-" + agentID,
		"workflow_run_id": "run-" + agentID,
	}

	if workflowRunLog != nil {
		if workflowRunLogTyped, ok := workflowRunLog.(*WorkflowRunLog); ok {
			response["workflow_run_id"] = workflowRunLogTyped.ID
		}
	}

	return response, nil
}

// RunWorkflowByWebAppID runs a workflow by web_app_id
func (s *WorkflowService) RunWorkflowByWebAppID(ctx context.Context, webAppID string, req interface{}, accountID string) (interface{}, error) {
	logger.Info("Running workflow by web_app_id", "webAppID", webAppID, "accountID", accountID)

	// Type assertion
	draftReq, ok := req.(*dto.DraftWorkflowRunRequest)
	if !ok {
		return nil, fmt.Errorf("invalid request type")
	}

	// Get agent by web_app_id
	agent, err := s.agentsRepo.GetByWebAppID(ctx, webAppID)
	if err != nil {
		logger.Error("Failed to get agent by web_app_id", err)
		return nil, fmt.Errorf("agent not found")
	}
	if !agent.IsWebAppActive() {
		logger.Warn("Web app is offline", map[string]interface{}{
			"web_app_id": webAppID,
			"agent_id":   agent.ID.String(),
		})
		return nil, fmt.Errorf("agent not found")
	}

	tenantID := agent.TenantID.String()
	agentID := agent.ID.String()
	if resolvedWorkspaceID, err := resolveServiceWebAppRunWorkspaceID(ctx, s.accountService, accountID, agent); err == nil {
		tenantID = resolvedWorkspaceID
	} else if tenantID == "" {
		return nil, fmt.Errorf("resolve workspace for web app workflow run: %w", err)
	}

	// Get latest published workflow for this agent
	workflow, err := s.repo.GetLatestPublishedWorkflow(ctx, agentID)
	if err != nil {
		logger.Error("Failed to get latest published workflow", err)
		return nil, fmt.Errorf("workflow not found")
	}

	// Parse graph and merge root environment/conversation variables.
	workflowMap := map[string]any{
		"graph":                  workflow.GetGraphDict(),
		"environment_variables":  normalizeVariables(workflow.GetEnvironmentVariablesDict()),
		"conversation_variables": normalizeVariables(workflow.GetConversationVariablesDict()),
	}
	graphData, err := mergeRootVariablesIntoGraph(workflowMap)
	if err != nil || graphData == nil || len(graphData) == 0 {
		logger.Error("Invalid graph data", fmt.Errorf("graph is empty"))
		return nil, fmt.Errorf("invalid graph data")
	}

	// Handle conversation workflow if needed
	var conversationID *string
	var workflowRunIDStr string
	var workflowRunLogID string
	defer func() {
		s.cleanupWorkflowReusableSessionsWithTimeout(workflowRunLogID)
	}()

	// Check if this is a conversation workflow
	if workflow.Type == "chat" {
		// Extract conversation parameters from inputs
		fromSource := "account"                // default
		invokeFrom := string(InvokeFromWebApp) // For web app calls
		var overrideModelConfigs *string

		// Check if we have conversation parameters in the request
		if convParams, ok := draftReq.Inputs["conversation_params"].(map[string]any); ok {
			if fs, ok := convParams["from_source"].(string); ok {
				fromSource = fs
			}
			if iv, ok := convParams["invoke_from"].(string); ok {
				invokeFrom = iv
			}
			if omc, ok := convParams["override_model_configs"].(string); ok {
				overrideModelConfigs = &omc
			}
		}

		// Get or create conversation
		var convIDStr string
		logger.Info("Checking for existing conversation ID", "sys.conversation_id", draftReq.Inputs["sys.conversation_id"])
		if convID, exists := draftReq.Inputs["sys.conversation_id"].(string); exists && convID != "" {
			convIDStr = convID
			// Update existing conversation with web_app_id
			logger.Info("Updating existing conversation with web_app_id", "conversationID", convIDStr, "webAppID", webAppID)
			err := s.advancedChatHandler.UpdateConversationWebAppID(convIDStr, webAppID)
			if err != nil {
				logger.Warn("Failed to update conversation web_app_id", "error", err)
				// Don't fail the workflow execution, just log the warning
			}
		} else {
			// Create new conversation with invoke_from = "web-app" and web_app_id = webAppID
			logger.Info("Creating NEW conversation with web_app_id", "webAppID", webAppID, "fromSource", fromSource, "invokeFrom", invokeFrom)
			logger.Info("Inputs for new conversation", "inputs", draftReq.Inputs)
			newConvID, err := s.advancedChatHandler.CreateConversationRecordWithParams(
				tenantID, agentID, accountID, fromSource, invokeFrom, draftReq.Inputs, overrideModelConfigs, &webAppID)
			if err != nil {
				logger.Error("Failed to create conversation", err)
				return nil, fmt.Errorf("failed to create conversation: %w", err)
			}
			convIDStr = newConvID
		}

		conversationID = &convIDStr
		// Add conversation ID to inputs for workflow execution
		draftReq.Inputs["sys.conversation_id"] = convIDStr
	}

	// Resolve organization scope for workflow system variables.
	organizationID := ""
	if s.enterpriseService != nil {
		organization, err := s.enterpriseService.GetOrganizationByWorkspaceID(ctx, tenantID)
		if err == nil && organization != nil {
			organizationID = organization.ID
		}
	}

	// Create workflow run log with triggered_from = "web-app" and web_app_id = webAppID
	workflowRunLog, err := s.CreateWorkflowRunLogWithWebAppID(ctx, tenantID, agentID, workflow.ID, "web-app", webAppID, draftReq.Inputs, accountID)
	if err != nil {
		logger.Error("Failed to create workflow run log", err)
		// Continue execution even if logging fails
	} else if workflowRunLogTyped, ok := workflowRunLog.(*WorkflowRunLog); ok {
		workflowRunLogID = workflowRunLogTyped.ID
		workflowRunIDStr = workflowRunLogID
	}

	// Add system variables to inputs for workflow execution
	if draftReq.Inputs == nil {
		draftReq.Inputs = make(map[string]interface{})
	}
	draftReq.Inputs["sys.tenant_id"] = tenantID
	draftReq.Inputs["sys.workspace_id"] = tenantID
	if organizationID != "" {
		draftReq.Inputs["sys.organization_id"] = organizationID
	}
	draftReq.Inputs["sys.agent_id"] = agentID
	draftReq.Inputs["sys.workflow_id"] = workflow.ID
	draftReq.Inputs["sys.workflow_run_id"] = workflowRunLogID
	draftReq.Inputs["sys.user_id"] = accountID
	draftReq.Inputs["sys.web_app_id"] = webAppID
	// Get query from inputs if exists
	if query, ok := draftReq.Inputs["query"].(string); ok && query != "" {
		draftReq.Inputs["sys.query"] = query
	}
	if conversationID != nil {
		draftReq.Inputs["sys.conversation_id"] = *conversationID
	}

	// Execute workflow with actual graph data
	startTime := time.Now()
	executionResult, err := s.executor.ExecuteSimpleWorkflowWithRunID(ctx, workflowRunLogID, graphData, draftReq.Inputs)
	elapsedTime := workflowExecutionResultElapsedMilliseconds(executionResult, ElapsedMillisecondsSince(startTime))

	// Update workflow run log with results
	if workflowRunLog != nil {
		status := "succeeded"
		if err != nil {
			status = "failed"
		}

		if workflowRunLogTyped, ok := workflowRunLog.(*WorkflowRunLog); ok {
			nodeResults := map[string]interface{}{}
			if executionResult != nil {
				nodeResults = executionResult.NodeResults
			}
			updateErr := s.UpdateWorkflowRunLogStatus(ctx, workflowRunLogTyped.ID, status, nodeResults, elapsedTime, 0, len(nodeResults), "")
			if updateErr != nil {
				logger.Error("Failed to update workflow run log", updateErr)
			}
		}
	}

	if err != nil {
		logger.Error("Failed to execute workflow", err)
		return nil, fmt.Errorf("workflow execution failed: %w", err)
	}

	// For conversation workflows, create message records
	if conversationID != nil {
		// Extract query and answer from inputs and outputs
		query := ""
		if q, exists := draftReq.Inputs["sys.query"].(string); exists {
			query = q
		} else if q, exists := draftReq.Inputs["query"].(string); exists {
			query = q
		}

		answer := ""
		// Try different possible output keys based on node types
		if textOutput, exists := executionResult.NodeResults["text"].(string); exists {
			answer = textOutput
		} else if answerOutput, exists := executionResult.NodeResults["answer"].(string); exists {
			answer = answerOutput
		} else if endNodeOutputs, ok := executionResult.NodeResults["end"]; ok {
			if endOutputsMap, ok := endNodeOutputs.(map[string]interface{}); ok {
				if textOutput, exists := endOutputsMap["text"].(string); exists {
					answer = textOutput
				} else if answerOutput, exists := endOutputsMap["answer"].(string); exists {
					answer = answerOutput
				}
			}
		} else {
			// Try to find answer from any node output
			for _, nodeOutput := range executionResult.NodeResults {
				if nodeOutputMap, ok := nodeOutput.(map[string]interface{}); ok {
					if nodeText, exists := nodeOutputMap["text"].(string); exists {
						answer = nodeText
						break
					} else if nodeAnswer, exists := nodeOutputMap["answer"].(string); exists {
						answer = nodeAnswer
						break
					}
				}
			}
		}

		// Parse UUIDs
		agentUUID, err := uuid.Parse(agentID)
		if err != nil {
			logger.Error("Invalid agent ID", err)
			return nil, fmt.Errorf("invalid agent ID: %w", err)
		}

		conversationUUID, err := uuid.Parse(*conversationID)
		if err != nil {
			logger.Error("Invalid conversation ID", err)
			return nil, fmt.Errorf("invalid conversation ID: %w", err)
		}

		workflowRunUUID, err := uuid.Parse(workflowRunIDStr)
		if err != nil {
			logger.Error("Invalid workflow run ID", err)
			return nil, fmt.Errorf("invalid workflow run ID: %w", err)
		}

		accountUUID, err := uuid.Parse(accountID)
		if err != nil {
			logger.Error("Invalid account ID", err)
			return nil, fmt.Errorf("invalid account ID: %w", err)
		}

		// Create message record with invoke_from = "web-app" and web_app_id = webAppID
		_, err = s.advancedChatHandler.CreateWorkflowMessage(
			agentUUID,
			conversationUUID,
			workflowRunUUID,
			query,
			answer,
			"account",
			string(InvokeFromWebApp), // invokeFrom - web-app for web app calls
			accountUUID,
			&accountUUID,
			&webAppID, // web_app_id - set to the web app ID
		)
		if err != nil {
			logger.Error("Failed to create workflow message", err)
			// Don't fail the entire workflow execution if message creation fails
		}
	}

	// Build response
	response := map[string]interface{}{
		"task_id":         "task-" + agentID,
		"workflow_run_id": "run-" + agentID,
	}

	if workflowRunLog != nil {
		if workflowRunLogTyped, ok := workflowRunLog.(*WorkflowRunLog); ok {
			response["workflow_run_id"] = workflowRunLogTyped.ID
		}
	}

	return response, nil
}

// CreateWorkflowRunLogWithWebAppID creates a new workflow run log entry with web_app_id
func (s *WorkflowService) CreateWorkflowRunLogWithWebAppID(ctx context.Context, tenantID, agentID, workflowID string, triggeredFrom string, webAppID string, inputs map[string]interface{}, accountID string) (interface{}, error) {
	if s.workflowRunLogRepo == nil {
		return nil, fmt.Errorf("workflow run log repository not initialized")
	}

	// Get next sequence number
	sequenceNumber, err := s.workflowRunLogRepo.GetNextSequenceNumber(ctx, tenantID, agentID)
	if err != nil {
		return nil, fmt.Errorf("failed to get next sequence number: %w", err)
	}

	// Marshal inputs to JSON
	inputsJSON, err := json.Marshal(inputs)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal inputs: %w", err)
	}

	inputsStr := string(inputsJSON)

	// Create workflow run log with web_app_id
	workflowRunLog := &WorkflowRunLog{
		ID:             uuid.New().String(),
		TenantID:       tenantID,
		AgentID:        agentID,
		WorkflowID:     workflowID,
		TriggeredFrom:  triggeredFrom,
		WebAppID:       &webAppID, // Set web_app_id
		Version:        "1.0",
		Status:         "running",
		Inputs:         &inputsStr,
		SequenceNumber: sequenceNumber,
		CreatedBy:      accountID,
		CreatedAt:      time.Now(),
	}

	// Save to database
	if err := s.workflowRunLogRepo.Create(ctx, workflowRunLog); err != nil {
		return nil, fmt.Errorf("failed to create workflow run log: %w", err)
	}

	return workflowRunLog, nil
}

// CreateWorkflowRunLogWithVersion creates a new workflow run log entry with version UUID
func (s *WorkflowService) CreateWorkflowRunLogWithVersion(ctx context.Context, tenantID, agentID, workflowID string, triggeredFrom string, versionUUID string, inputs map[string]interface{}, accountID string) (interface{}, error) {
	if s.workflowRunLogRepo == nil {
		return nil, fmt.Errorf("workflow run log repository not initialized")
	}

	// Get next sequence number
	sequenceNumber, err := s.workflowRunLogRepo.GetNextSequenceNumber(ctx, tenantID, agentID)
	if err != nil {
		return nil, fmt.Errorf("failed to get next sequence number: %w", err)
	}

	// Marshal inputs to JSON
	inputsJSON, err := json.Marshal(inputs)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal inputs: %w", err)
	}

	// Try to capture current workflow graph/features/type from the version
	var graphStr *string
	var featuresStr *string
	var workflowType dto.WorkflowType = dto.WorkflowTypeWorkflow // Default to workflow
	// Note: GetByVersionUUID was removed as part of web_app_id migration
	// Workflow graph/features/type will use defaults

	// Get created_by_role from context, default to "account" for internal calls
	createdByRole := CreatedByRoleAccount
	if role, ok := ctx.Value("created_by_role").(string); ok && role != "" {
		createdByRole = CreatedByRole(role)
	}

	// Create workflow run log with version UUID
	inputsJSONStr := string(inputsJSON)
	workflowRunLog := &WorkflowRunLog{
		TenantID:        tenantID,
		AgentID:         agentID,
		SequenceNumber:  sequenceNumber,
		WorkflowID:      workflowID,
		Type:            workflowType, // Use actual workflow type from workflow definition
		TriggeredFrom:   triggeredFrom,
		Version:         versionUUID,  // Use version UUID as version
		WebAppID:        &versionUUID, // Store version UUID in web_app_id field for backward compatibility
		Graph:           graphStr,
		Features:        featuresStr,
		Inputs:          &inputsJSONStr,
		Status:          dto.WorkflowRunStatusRunning,
		ElapsedTime:     0,
		TotalTokens:     0,
		TotalSteps:      0,
		CreatedByRole:   createdByRole,
		CreatedBy:       accountID,
		CreatedAt:       time.Now(),
		ExceptionsCount: 0,
	}

	if err := s.workflowRunLogRepo.Create(ctx, workflowRunLog); err != nil {
		return nil, fmt.Errorf("failed to create workflow run log: %w", err)
	}

	return workflowRunLog, nil
}

// GetLatestWorkflowVersion gets the latest workflow version UUID for an agent
func (s *WorkflowService) GetLatestWorkflowVersion(ctx context.Context, workspaceID, agentID string) (string, error) {
	workflowData, err := s.GetLatestPublishedWorkflow(ctx, workspaceID, agentID)
	if err != nil {
		return "", err
	}

	if workflowData == nil {
		return "", fmt.Errorf("no published workflow found")
	}

	workflowMap, ok := workflowData.(map[string]interface{})
	if !ok {
		return "", fmt.Errorf("invalid workflow data format")
	}

	if versionUUID, exists := workflowMap["version_uuid"]; exists && versionUUID != nil {
		if versionStr, ok := versionUUID.(string); ok {
			return versionStr, nil
		}
	}

	return "", fmt.Errorf("workflow version not found")
}

// ManualDiagnoseNode performs a manual diagnosis for a failed workflow node
func (s *WorkflowService) ManualDiagnoseNode(ctx context.Context, nodeLogID string, model string, lang string) (interface{}, error) {
	if s.diagnoser == nil {
		return nil, fmt.Errorf("diagnoser not initialized")
	}

	// 1. Fetch the node log
	nodeLog, err := s.workflowNodeRuntimeLogRepo.GetByID(ctx, nodeLogID)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch node log: %w", err)
	}

	if nodeLog.Error == nil || *nodeLog.Error == "" {
		return nil, fmt.Errorf("node log does not contain an error to diagnose")
	}

	workflowRunID := ""
	if nodeLog.WorkflowRunID != nil {
		workflowRunID = *nodeLog.WorkflowRunID
	}

	// 2. Re-construct ErrorContext from saved JSON fields
	errCtx := &diagnosis.ErrorContext{
		NodeLogID:     nodeLog.ID,
		WorkflowID:    nodeLog.WorkflowID,
		WorkflowRunID: workflowRunID,
		NodeID:        nodeLog.NodeID,
		NodeType:      nodeLog.NodeType,
		NodeName:      nodeLog.Title,
		ErrorMessage:  *nodeLog.Error,
		ErrorStack:    "", // Will be populated if needed, but we mostly use snapshots
		Timestamp:     nodeLog.CreatedAt,
		WorkspaceID:   nodeLog.TenantID,
		UserID:        nodeLog.CreatedBy,
	}

	if nodeLog.ErrorType != nil {
		errCtx.ErrorType = diagnosis.ErrorType(*nodeLog.ErrorType)
	}
	if nodeLog.ErrorStack != nil {
		errCtx.ErrorStack = *nodeLog.ErrorStack
	}

	// Parse snapshots from JSONB strings
	if nodeLog.DiagnosisNodeConfig != nil && *nodeLog.DiagnosisNodeConfig != "" {
		json.Unmarshal([]byte(*nodeLog.DiagnosisNodeConfig), &errCtx.NodeConfig)
	}
	if nodeLog.DiagnosisInputSnapshot != nil && *nodeLog.DiagnosisInputSnapshot != "" {
		json.Unmarshal([]byte(*nodeLog.DiagnosisInputSnapshot), &errCtx.InputSnapshot)
	}
	if nodeLog.DiagnosisUpstreamConfig != nil && *nodeLog.DiagnosisUpstreamConfig != "" {
		var upstreamConfigs map[string]any
		if err := json.Unmarshal([]byte(*nodeLog.DiagnosisUpstreamConfig), &upstreamConfigs); err == nil {
			errCtx.UpstreamContexts = make(map[string]diagnosis.UpstreamNodeContext)
			for id, config := range upstreamConfigs {
				ctxEntry := errCtx.UpstreamContexts[id]
				if cfgMap, ok := config.(map[string]any); ok {
					ctxEntry.Config = cfgMap
				}
				errCtx.UpstreamContexts[id] = ctxEntry
			}
		}
	}
	if nodeLog.DiagnosisUpstreamOutputs != nil && *nodeLog.DiagnosisUpstreamOutputs != "" {
		var upstreamOutputs map[string]any
		if err := json.Unmarshal([]byte(*nodeLog.DiagnosisUpstreamOutputs), &upstreamOutputs); err == nil {
			if errCtx.UpstreamContexts == nil {
				errCtx.UpstreamContexts = make(map[string]diagnosis.UpstreamNodeContext)
			}
			for id, output := range upstreamOutputs {
				ctxEntry := errCtx.UpstreamContexts[id]
				if outMap, ok := output.(map[string]any); ok {
					ctxEntry.Output = outMap
				}
				errCtx.UpstreamContexts[id] = ctxEntry
			}
		}
	}

	// Resolve OrgID
	if s.enterpriseService != nil {
		org, err := s.enterpriseService.GetOrganizationByWorkspaceID(ctx, nodeLog.TenantID)
		if err == nil && org != nil {
			errCtx.OrgID = org.ID
		}
	}

	// 3. Call Diagnoser
	dc := &diagnosis.DiagnoseContext{
		ErrCtx: errCtx,
		Lang:   lang,
	}

	res := s.diagnoser.Diagnose(ctx, dc, model)

	// 4. Update the database with the result
	err = s.workflowNodeRuntimeLogRepo.UpdateDiagnosisResult(
		ctx,
		nodeLogID,
		res.ResultText,
		res.ModelUsed,
		res.Tokens,
		res.LatencyMs,
		res.IsDiagnosed,
	)
	if err != nil {
		logger.Error("Failed to update node diagnosis result in manual trigger", err)
	}

	return map[string]interface{}{
		"result":       res.ResultText,
		"model":        res.ModelUsed,
		"tokens":       res.Tokens,
		"latency_ms":   res.LatencyMs,
		"is_diagnosed": res.IsDiagnosed,
	}, nil
}
