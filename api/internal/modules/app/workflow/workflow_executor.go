package workflow

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/zgiai/zgi/api/internal/modules/app/conversation"
	"github.com/zgiai/zgi/api/internal/modules/app/workflow/file"
	"github.com/zgiai/zgi/api/internal/modules/app/workflow/graph_engine"
	"github.com/zgiai/zgi/api/internal/modules/app/workflow/graph_engine/entities"
	"github.com/zgiai/zgi/api/internal/modules/app/workflow/graphconfig"
	workflowruntime "github.com/zgiai/zgi/api/internal/modules/app/workflow/runtime"
	"github.com/zgiai/zgi/api/internal/modules/app/workflow/shared"
	automationdefinition "github.com/zgiai/zgi/api/internal/modules/automation/service/definition"
	"github.com/zgiai/zgi/api/internal/modules/dataset/graphflow"
	"github.com/zgiai/zgi/api/internal/modules/file_process/service/extractor"
	notificationsms "github.com/zgiai/zgi/api/internal/modules/notification/sms"
	promptservice "github.com/zgiai/zgi/api/internal/modules/prompts/service"
	interfaces "github.com/zgiai/zgi/api/internal/modules/shared/interface"
	"github.com/zgiai/zgi/api/pkg/database"
	"github.com/zgiai/zgi/api/pkg/logger"
	"github.com/zgiai/zgi/api/pkg/storage"
	"go.uber.org/zap"
)

// WorkflowExecutor workflow executor
type WorkflowExecutor struct {
	maxConcurrency              int
	activeEngines               map[string]*graph_engine.WorkflowEngine // Map of workflow run ID to engine
	enginesMu                   sync.RWMutex
	engineFactory               *graph_engine.EngineFactory
	contentExtractor            file.ContentExtractor
	fileService                 interfaces.FileService
	variableService             conversation.WorkflowConversationVariableService
	llmClient                   interface{} // LLM client for LLM nodes
	toolEngine                  interface{} // Tool engine for tools nodes
	graphFlowService            *graphflow.Service
	promptResolver              promptservice.PromptService
	automationDefinitionService automationdefinition.Service
	notificationSMSService      notificationsms.Service
}

type WorkflowExecutorDeps struct {
	FileService                 interfaces.FileService
	ContentExtractor            file.ContentExtractor
	LLMClient                   interface{}
	ToolEngine                  interface{}
	GraphFlowService            *graphflow.Service
	PromptResolver              promptservice.PromptService
	AutomationDefinitionService automationdefinition.Service
	NotificationSMSService      notificationsms.Service
	EngineFactory               *graph_engine.EngineFactory
}

// NewWorkflowExecutor creates a new workflow executor
func NewWorkflowExecutor() *WorkflowExecutor {
	return NewWorkflowExecutorWithRuntimeDeps(WorkflowExecutorDeps{})
}

// NewWorkflowExecutorWithDeps creates a new workflow executor with dependencies
func NewWorkflowExecutorWithDeps(fileService interfaces.FileService) *WorkflowExecutor {
	return NewWorkflowExecutorWithRuntimeDeps(WorkflowExecutorDeps{
		FileService: fileService,
	})
}

func NewWorkflowExecutorWithRuntimeDeps(deps WorkflowExecutorDeps) *WorkflowExecutor {
	// Initialize variable service
	db := database.GetDB()
	variableRepo := conversation.NewWorkflowConversationVariableRepository(db)
	variableService := conversation.NewWorkflowConversationVariableService(variableRepo)

	executor := &WorkflowExecutor{
		maxConcurrency:              graph_engine.DefaultMaxConcurrency,
		activeEngines:               make(map[string]*graph_engine.WorkflowEngine),
		engineFactory:               deps.EngineFactory,
		contentExtractor:            deps.ContentExtractor,
		fileService:                 deps.FileService,
		variableService:             variableService,
		llmClient:                   deps.LLMClient,
		toolEngine:                  deps.ToolEngine,
		graphFlowService:            deps.GraphFlowService,
		promptResolver:              deps.PromptResolver,
		automationDefinitionService: deps.AutomationDefinitionService,
		notificationSMSService:      deps.NotificationSMSService,
	}

	// Create ContentExtractor if fileService is provided
	if executor.fileService != nil && executor.contentExtractor == nil {
		executor.contentExtractor = executor.createContentExtractor()
	}

	return executor
}

// createContentExtractor creates a ContentExtractor instance with dependencies
func (e *WorkflowExecutor) createContentExtractor() file.ContentExtractor {
	// Get storage instance
	storageClient := storage.GetStorage()

	// Create ExtractProcessor with storage
	extractProcessor := extractor.NewExtractProcessor(storageClient)

	// Get content extractor configuration
	config := file.GetContentExtractorConfig()

	// Create ContentExtractor with FileService, ExtractProcessor, and Config
	contentExtractor := file.NewContentExtractor(e.fileService, extractProcessor, config)

	logger.Info("ContentExtractor created successfully for workflow executor",
		"enabled", config.Enabled,
		"max_content_size", config.MaxContentSize,
		"extraction_timeout", config.ExtractionTimeout,
		"cache_enabled", config.CacheEnabled)

	return contentExtractor
}

// GetContentExtractor returns the content extractor instance
func (e *WorkflowExecutor) GetContentExtractor() file.ContentExtractor {
	return e.contentExtractor
}

// SetContentExtractor sets the content extractor instance
func (e *WorkflowExecutor) SetContentExtractor(contentExtractor interface{}) {
	if ce, ok := contentExtractor.(file.ContentExtractor); ok {
		e.contentExtractor = ce
		logger.Info("ContentExtractor set on workflow executor")
	} else {
		logger.Warn("Invalid ContentExtractor type provided to SetContentExtractor")
	}
}

// SetLLMClient sets the LLM client for LLM nodes
func (e *WorkflowExecutor) SetLLMClient(llmClient interface{}) {
	e.llmClient = llmClient
	logger.Info("LLMClient set on workflow executor")
}

// GetLLMClient returns the LLM client instance
func (e *WorkflowExecutor) GetLLMClient() interface{} {
	return e.llmClient
}

// SetToolEngine sets the tool engine for tools nodes
func (e *WorkflowExecutor) SetToolEngine(toolEngine interface{}) {
	e.toolEngine = toolEngine
	logger.Info("ToolEngine set on workflow executor")
}

// GetToolEngine returns the tool engine instance
func (e *WorkflowExecutor) GetToolEngine() interface{} {
	return e.toolEngine
}

// SetGraphFlowService sets the GraphFlow service
func (e *WorkflowExecutor) SetGraphFlowService(graphFlowService *graphflow.Service) {
	e.graphFlowService = graphFlowService
	logger.Info("GraphFlowService set on workflow executor")
}

// GetGraphFlowService returns the GraphFlow service
func (e *WorkflowExecutor) GetGraphFlowService() *graphflow.Service {
	return e.graphFlowService
}

// SetAutomationDefinitionService sets the automation definition service for automation workflow nodes.
func (e *WorkflowExecutor) SetAutomationDefinitionService(service automationdefinition.Service) {
	e.automationDefinitionService = service
	logger.Info("AutomationDefinitionService set on workflow executor")
}

func (e *WorkflowExecutor) SetNotificationSMSService(service notificationsms.Service) {
	e.notificationSMSService = service
	logger.Info("NotificationSMSService set on workflow executor")
}

func (e *WorkflowExecutor) runtimeDependencies() workflowruntime.Dependencies {
	return workflowruntime.Dependencies{
		ContentExtractor:            e.contentExtractor,
		LLMClient:                   e.llmClient,
		ToolEngine:                  e.toolEngine,
		GraphFlowService:            e.graphFlowService,
		FileService:                 e.fileService,
		PromptResolver:              e.promptResolver,
		AutomationDefinitionService: e.automationDefinitionService,
		NotificationSMSService:      e.notificationSMSService,
	}
}

func (e *WorkflowExecutor) newEngineFactory() *graph_engine.EngineFactory {
	if e.engineFactory != nil {
		return e.engineFactory
	}
	return graph_engine.NewEngineFactory(e.maxConcurrency, workflowruntime.NewNodeRunner(e.runtimeDependencies()))
}

func graphMaxConcurrency(config map[string]interface{}) int {
	if config == nil {
		return 0
	}

	if maxConcurrency := graphMaxConcurrencyValue(config["parallel_nums"]); maxConcurrency > 0 {
		return maxConcurrency
	}

	if nestedConfig, ok := config["config"].(map[string]interface{}); ok {
		return graphMaxConcurrency(nestedConfig)
	}

	return 0
}

func graphMaxConcurrencyValue(value interface{}) int {
	switch value := value.(type) {
	case int:
		return value
	case int64:
		return int(value)
	case float64:
		return int(value)
	case float32:
		return int(value)
	case json.Number:
		parsed, err := value.Int64()
		if err != nil {
			return 0
		}
		return int(parsed)
	default:
		return 0
	}
}

func (e *WorkflowExecutor) newEngine(
	runtimeState *entities.GraphRuntimeState,
	graph *entities.Graph,
	callbacks graph_engine.EngineCallbacks,
) *graph_engine.WorkflowEngine {
	maxConcurrency := 0
	if graph != nil {
		maxConcurrency = graphMaxConcurrency(graph.Config)
	}
	return e.newEngineFactory().New(graph_engine.EngineRunConfig{
		RuntimeState:   runtimeState,
		Graph:          graph,
		Callbacks:      callbacks,
		MaxConcurrency: maxConcurrency,
	})
}

func (e *WorkflowExecutor) registerEngineForRun(runID string, engine *graph_engine.WorkflowEngine) func() {
	if runID == "" {
		return func() {}
	}
	e.RegisterEngine(runID, engine)
	return func() {
		e.UnregisterEngine(runID)
	}
}

// ExecuteSimpleWorkflow executes simple workflow (start -> http -> end)
func (e *WorkflowExecutor) ExecuteSimpleWorkflow(ctx context.Context, workflowData map[string]interface{}, inputs map[string]interface{}) (*WorkflowExecutionResult, error) {
	return e.ExecuteSimpleWorkflowWithRunID(ctx, "", workflowData, inputs)
}

// ExecuteSimpleWorkflowWithRunID executes simple workflow with run ID for stop functionality
func (e *WorkflowExecutor) ExecuteSimpleWorkflowWithRunID(ctx context.Context, runID string, workflowData map[string]interface{}, inputs map[string]interface{}) (*WorkflowExecutionResult, error) {
	return e.ExecuteSimpleWorkflowWithRunIDAndCallbacks(ctx, runID, workflowData, inputs, graph_engine.EngineCallbacks{})
}

// ExecuteSimpleWorkflowWithRunIDAndCallbacks executes simple workflow with run-scoped streaming callbacks.
func (e *WorkflowExecutor) ExecuteSimpleWorkflowWithRunIDAndCallbacks(ctx context.Context, runID string, workflowData map[string]interface{}, inputs map[string]interface{}, callbacks graph_engine.EngineCallbacks) (*WorkflowExecutionResult, error) {
	logger.Info("Starting simple workflow execution")

	reachabilityViews, err := buildWorkflowGraphReachabilityViews(workflowData)
	if err != nil {
		return nil, err
	}
	executionGraphData := reachabilityViews.ExecutionGraphData
	runtimeGraphData := reachabilityViews.RuntimeGraphData

	// 1. Parse workflow data
	nodes, edges, err := e.parseWorkflowData(executionGraphData)
	if err != nil {
		return nil, fmt.Errorf("failed to parse workflow data: %w", err)
	}

	// 2. Extract conversation/environment variables from workflow data.
	conversationVarsConfig := extractVariableConfigList(runtimeGraphData["conversation_variables"])
	environmentVarsConfig := extractVariableConfigList(runtimeGraphData["environment_variables"])
	logger.DebugContext(ctx, "workflow variable configuration loaded",
		zap.Int("conversation_variables_count", len(conversationVarsConfig)),
		zap.Int("environment_variables_count", len(environmentVarsConfig)),
		zap.String("workflow_run_id", runID),
	)

	// 2.5 Hydrate inputs with file metadata (fetch from upload_files table)
	if e.fileService != nil {
		logger.Info("Hydrating inputs with file metadata")
		hydratedInputs, err := e.HydrateInputs(ctx, inputs)
		if err != nil {
			logger.Warn("Failed to hydrate inputs, using original inputs", "error", err)
		} else {
			inputs = hydratedInputs
			logger.Info("Successfully hydrated inputs")
		}
	}

	// 3. Create variable pool with environment and conversation variables
	variablePool := e.createVariablePoolWithVars(inputs, environmentVarsConfig, conversationVarsConfig)

	// 4. Create runtime state
	runtimeState := &entities.GraphRuntimeState{
		VariablePool: variablePool,
	}

	// 5. Create graph configuration
	graph := &entities.Graph{
		Config: runtimeGraphData,
	}

	// 6. Create new engine instance for this execution
	engine := e.newEngine(runtimeState, graph, callbacks)

	// Register engine if runID is provided
	defer e.registerEngineForRun(runID, engine)()

	// 7. Add nodes to engine
	for nodeID, nodeConfig := range nodes {
		nodeType, err := e.getNodeType(nodeConfig)
		if err != nil {
			return nil, fmt.Errorf("failed to parse node type for %s: %w", nodeID, err)
		}
		engine.AddNode(nodeID, nodeType, nodeConfig)
	}

	// 8. Add dependencies
	for _, edge := range edges {
		if err := engine.AddDependencyWithHandle(edge.From, edge.To, edge.SourceHandle); err != nil {
			return nil, fmt.Errorf("failed to add dependency: %w", err)
		}
	}

	// 9. Execute workflow
	startTime := time.Now()
	err = engine.Execute(ctx)
	executionTime := time.Since(startTime)

	// 10. Save conversation variables if conversation ID is present
	if len(conversationVarsConfig) > 0 {
		conversationID := ""
		appID := ""

		// Extract from inputs
		if convID, ok := inputs["sys.conversation_id"].(string); ok {
			conversationID = convID
		}
		if agentID, ok := inputs["sys.agent_id"].(string); ok {
			appID = agentID
		}

		if conversationID != "" && appID != "" {
			e.saveConversationVariables(ctx, conversationID, appID, variablePool)
		}
	}

	// 11. Collect results
	workflowStatus := e.getWorkflowStatus(err)
	if engine.IsPaused() {
		workflowStatus = "paused"
	}
	result := &WorkflowExecutionResult{
		Status:         workflowStatus,
		ExecutionTime:  executionTime,
		NodeResults:    e.collectNodeResultsFromEngine(engine),
		NodeExecutions: engine.GetNodeExecutionSnapshots(),
		RuntimeState:   runtimeState,
		Error:          err,
	}

	logger.Info("Workflow execution completed", "status", result.Status, "duration", executionTime)
	return result, nil
}

// parseWorkflowData parses workflow data
func (e *WorkflowExecutor) parseWorkflowData(workflowData map[string]interface{}) (map[string]map[string]interface{}, []Edge, error) {
	nodes := make(map[string]map[string]interface{})
	var edges []Edge

	// Parse nodes
	if nodesData, ok := workflowData["nodes"].([]interface{}); ok {
		for _, nodeData := range nodesData {
			if nodeMap, ok := nodeData.(map[string]interface{}); ok {
				if nodeID, ok := nodeMap["id"].(string); ok {
					nodes[nodeID] = nodeMap
				}
			}
		}
	}

	// Parse edges
	if edgesData, ok := workflowData["edges"].([]interface{}); ok {
		for _, edgeData := range edgesData {
			if edgeMap, ok := edgeData.(map[string]interface{}); ok {
				if from, ok := edgeMap["source"].(string); ok {
					if to, ok := edgeMap["target"].(string); ok {
						sourceHandle, _ := edgeMap["sourceHandle"].(string)
						if sourceHandle == "" {
							sourceHandle = "source"
						}
						edges = append(edges, Edge{From: from, To: to, SourceHandle: sourceHandle})
					}
				}
			}
		}
	}

	return nodes, edges, nil
}

func (e *WorkflowExecutor) getNodeType(nodeConfig map[string]interface{}) (shared.NodeType, error) {
	return graphconfig.ExtractNodeType(nodeConfig)
}

func extractVariableConfigList(raw any) []map[string]any {
	switch vars := raw.(type) {
	case []map[string]any:
		return vars
	case []any:
		result := make([]map[string]any, 0, len(vars))
		for _, item := range vars {
			if m, ok := item.(map[string]any); ok {
				result = append(result, m)
			}
		}
		return result
	default:
		return nil
	}
}

func resolveVariableType(varConfig map[string]any) string {
	if typeVal, ok := varConfig["type"].(string); ok && strings.TrimSpace(typeVal) != "" {
		return strings.TrimSpace(typeVal)
	}
	if valueType, ok := varConfig["value_type"].(string); ok && strings.TrimSpace(valueType) != "" {
		return strings.TrimSpace(valueType)
	}
	return "string"
}

// buildConversationVariables builds conversation variables from workflow configuration
// Loads persisted values from database and merges with configuration defaults (database values take precedence)
// Supports all value_type: string, number, object, boolean, array_string, array_number, array_object, array_boolean
// Requirements: 5.2, 5.3, 5.4, 5.5
func (e *WorkflowExecutor) buildConversationVariables(configVars []map[string]any, conversationID, appID string) []entities.Variable {
	logger.Debug("Building conversation variables", map[string]any{
		"config_vars_count": len(configVars),
		"conversation_id":   conversationID,
		"app_id":            appID,
	})

	// Load persisted variables from database if conversationID is provided
	// Requirement 5.2: Database load failures are logged but don't fail workflow
	var persistedVars map[string]any
	if conversationID != "" {
		convUUID, err := uuid.Parse(conversationID)
		if err != nil {
			logger.Warn("Invalid conversation ID format, using config defaults only", map[string]any{
				"conversation_id": conversationID,
				"error":           err.Error(),
			})
		} else {
			ctx := context.Background()
			loadedVars, err := e.variableService.LoadConversationVariables(ctx, convUUID)
			if err != nil {
				// Requirement 5.2: Log warning and continue with config defaults
				logger.Warn("Failed to load persisted conversation variables, using config defaults", map[string]any{
					"conversation_id": conversationID,
					"error":           err.Error(),
				})
			} else {
				persistedVars = loadedVars
				logger.Debug("Loaded persisted conversation variables from database", map[string]any{
					"count": len(persistedVars),
				})
			}
		}
	}

	conversationVars := make([]entities.Variable, 0, len(configVars))

	for i, varMap := range configVars {
		// Extract required fields
		name, nameOk := varMap["name"].(string)
		if !nameOk || name == "" {
			// Requirement 5.3: Missing name is logged and skipped
			logger.Warn("Conversation variable missing 'name' field, skipping", map[string]any{
				"index": i,
			})
			continue
		}

		variable, err := e.buildVariableFromConfig(varMap, entities.ConversationVariableNodeId, persistedVars)
		if err != nil {
			logger.Warn("Failed to build conversation variable, skipping", map[string]any{
				"index": i,
				"name":  name,
				"error": err.Error(),
			})
			continue
		}
		conversationVars = append(conversationVars, variable)

		// Requirement 5.4: Debug logging for variable creation and access
		logger.Debug("Created conversation variable", map[string]any{
			"name":       name,
			"value_type": variable.GetType(),
			"selector":   variable.GetSelector(),
			"value":      variable.GetValue(),
		})
	}

	logger.Info("Conversation variables built successfully", map[string]any{
		"total_created": len(conversationVars),
		"total_config":  len(configVars),
	})
	return conversationVars
}

// saveConversationVariables saves updated conversation variables to the database
// Requirement 5.4: Database operation failures are logged but don't fail workflow
func (e *WorkflowExecutor) saveConversationVariables(ctx context.Context, conversationID, appID string, variablePool *entities.VariablePool) {
	logger.Debug("Saving conversation variables", map[string]interface{}{
		"conversation_id": conversationID,
		"app_id":          appID,
	})

	if variablePool == nil {
		logger.Debug("No variable pool provided, skipping save")
		return
	}

	// Parse UUIDs
	convUUID, err := uuid.Parse(conversationID)
	if err != nil {
		// Requirement 5.4: Invalid IDs are logged but don't fail workflow
		logger.Error(fmt.Sprintf("Invalid conversation ID format '%s', cannot save variables", conversationID), err)
		return
	}

	appUUID, err := uuid.Parse(appID)
	if err != nil {
		// Requirement 5.4: Invalid IDs are logged but don't fail workflow
		logger.Error(fmt.Sprintf("Invalid app ID format '%s', cannot save variables", appID), err)
		return
	}

	// Collect current values of all conversation variables from VariableDictionary
	// This ensures we get the updated values after workflow execution
	variables := make(map[string]interface{})
	if conversationDict, exists := variablePool.VariableDictionary[entities.ConversationVariableNodeId]; exists {
		for name, variable := range conversationDict {
			value := variable.GetValue()
			variables[name] = value

			// Requirement 5.4: Debug logging for variable access
			logger.Debug("Collecting conversation variable for save", map[string]interface{}{
				"name":  name,
				"value": value,
				"type":  fmt.Sprintf("%T", value),
			})
		}
	}

	if len(variables) == 0 {
		logger.Debug("No conversation variables to save")
		return
	}

	// Save to database
	// Requirement 5.4: Database save failures are logged but don't fail workflow
	err = e.variableService.SaveConversationVariables(ctx, convUUID, appUUID, variables)
	if err != nil {
		logger.Error(fmt.Sprintf("Failed to save %d conversation variables for conversation %s, workflow will continue", len(variables), conversationID), err)
		// Don't fail the workflow, just log the error
	} else {
		logger.Info("Successfully saved conversation variables to database", map[string]interface{}{
			"conversation_id": conversationID,
			"variable_count":  len(variables),
		})
	}
}

// buildVariableFromConfig builds a variable from configuration.
// It supports both "type" and "value_type" with "type" taking precedence.
func (e *WorkflowExecutor) buildVariableFromConfig(varConfig map[string]any, nodeID string, persistedVars map[string]any) (entities.Variable, error) {
	name, _ := varConfig["name"].(string)
	name = strings.TrimSpace(name)
	if name == "" {
		return nil, fmt.Errorf("variable missing name")
	}

	valueType := resolveVariableType(varConfig)

	value := varConfig["value"]
	if persistedVars != nil {
		if persistedValue, exists := persistedVars[name]; exists {
			value = persistedValue
		}
	}

	// For secret type (environment variables only), replace SecretHiddenValue with actual value
	if nodeID == entities.EnvironmentVariableNodeId && valueType == entities.SecretValueType {
		if str, ok := value.(string); ok && str == entities.SecretHiddenValue {
			if persistedVars != nil {
				if persistedValue, exists := persistedVars[name]; exists {
					value = persistedValue
				}
			}
		}
	}

	// Convert value to appropriate Segment based on value_type
	segment, warnings, err := entities.ConvertValue(valueType, value, entities.ValueConversionLenient)
	if err != nil {
		logger.Warn("Variable conversion failed, defaulting to string", map[string]any{
			"name":       name,
			"value_type": valueType,
			"error":      err.Error(),
		})
		segment = &entities.StringSegment{Value: fmt.Sprintf("%v", value)}
	}
	for _, warning := range warnings {
		logger.Warn("Variable conversion warning", map[string]any{
			"name":       name,
			"value_type": valueType,
			"detail":     warning,
		})
	}

	selector := []string{nodeID, name}

	return &conversationVariableWrapper{
		segment:  segment,
		name:     name,
		selector: selector,
	}, nil
}

// buildEnvironmentVariables builds environment variables from configuration.
func (e *WorkflowExecutor) buildEnvironmentVariables(configVars []map[string]any) []entities.Variable {
	envVars := make([]entities.Variable, 0, len(configVars))

	for i, varMap := range configVars {
		variable, err := e.buildVariableFromConfig(varMap, entities.EnvironmentVariableNodeId, nil)
		if err != nil {
			logger.Warn("Environment variable missing 'name' field, skipping", map[string]any{
				"index": i,
				"error": err.Error(),
			})
			continue
		}
		envVars = append(envVars, variable)
	}

	return envVars
}

// conversationVariableWrapper wraps a Segment to implement the Variable interface
// This is a local implementation for conversation variables created in the executor
type conversationVariableWrapper struct {
	segment  entities.Segment
	name     string
	selector []string
}

func (cvw *conversationVariableWrapper) ToObject() interface{} {
	return cvw.segment.ToObject()
}

func (cvw *conversationVariableWrapper) GetValue() interface{} {
	return cvw.segment.GetValue()
}

func (cvw *conversationVariableWrapper) GetType() shared.SegmentType {
	return cvw.segment.GetType()
}

func (cvw *conversationVariableWrapper) GetName() string {
	return cvw.name
}

func (cvw *conversationVariableWrapper) GetSelector() []string {
	return cvw.selector
}

func (cvw *conversationVariableWrapper) Text() string {
	return cvw.segment.Text()
}

func (cvw *conversationVariableWrapper) Log() string {
	return cvw.segment.Log()
}

func (cvw *conversationVariableWrapper) Markdown() string {
	return cvw.segment.Markdown()
}

func (cvw *conversationVariableWrapper) Size() int {
	return cvw.segment.Size()
}

// createVariablePool creates variable pool
func (e *WorkflowExecutor) createVariablePool(inputs map[string]any) *entities.VariablePool {
	return e.createVariablePoolWithVars(inputs, nil, nil)
}

// createVariablePoolWithVars creates variable pool with environment and conversation variables
func (e *WorkflowExecutor) createVariablePoolWithVars(inputs map[string]any, envVarsConfig []map[string]any, conversationVarsConfig []map[string]any) *entities.VariablePool {
	logger.Debug("workflow variable pool creation started",
		zap.Int("input_count", len(inputs)),
		zap.Int("environment_variables_count", len(envVarsConfig)),
		zap.Int("conversation_variables_count", len(conversationVarsConfig)),
	)

	// Create system variables
	systemVariables := entities.SystemVariableEmpty()

	// Extract system variables from inputs if available
	if userID, ok := inputs["sys.user_id"].(string); ok {
		systemVariables.UserID = userID
	} else {
		systemVariables.UserID = "default-user"
	}

	if agentID, ok := inputs["sys.agent_id"].(string); ok {
		systemVariables.AppID = agentID
	} else {
		systemVariables.AppID = "default-agent"
	}

	if workflowID, ok := inputs["sys.workflow_id"].(string); ok {
		systemVariables.WorkflowID = workflowID
	} else {
		systemVariables.WorkflowID = "default-workflow"
	}

	// workspace_id is the canonical workflow subject identifier.
	if workspaceID, ok := inputs["sys.workspace_id"].(string); ok {
		systemVariables.WorkspaceID = workspaceID
		systemVariables.TenantID = workspaceID // Legacy mirror for old code paths.
	}
	if organizationID, ok := inputs["sys.organization_id"].(string); ok {
		systemVariables.OrganizationID = organizationID
	}
	if billingSubjectType, ok := inputs["sys.billing_subject_type"].(string); ok {
		systemVariables.BillingSubjectType = billingSubjectType
	}

	if query, ok := inputs["sys.query"].(string); ok {
		systemVariables.Query = query
		logger.Debug("workflow variable pool found system query",
			zap.Int("query_length", len(query)),
		)
	}
	if wft, ok := inputs["sys.workflow_type"].(string); ok {
		systemVariables.WorkflowType = wft
	}
	if conversationID, ok := inputs["sys.conversation_id"].(string); ok {
		systemVariables.ConversationID = conversationID
	}
	if dialogueCount, ok := inputs["sys.dialogue_count"].(int); ok {
		systemVariables.DialogueCount = dialogueCount
	}
	if workflowRunID, ok := inputs["sys.workflow_run_id"].(string); ok {
		systemVariables.WorkflowRunID = workflowRunID
	}

	logger.Debug("workflow variable pool system variables prepared",
		zap.String("workspace_id", systemVariables.WorkspaceID),
		zap.String("tenant_id", systemVariables.TenantID),
		zap.String("organization_id", systemVariables.OrganizationID),
		zap.String("user_id", systemVariables.UserID),
		zap.String("app_id", systemVariables.AppID),
		zap.String("workflow_id", systemVariables.WorkflowID),
		zap.Int("query_length", len(systemVariables.Query)),
		zap.String("conversation_id", systemVariables.ConversationID),
		zap.Int("dialogue_count", systemVariables.DialogueCount),
	)

	// Create variable pool using NewVariablePool which calls modelPostInit internally
	variablePool := entities.NewVariablePool()

	// Set the fields after creation
	variablePool.UserInputs = inputs
	variablePool.SystemVariables = systemVariables

	// Build environment variables from configuration if provided
	if len(envVarsConfig) > 0 {
		envVars := e.buildEnvironmentVariables(envVarsConfig)
		variablePool.EnvironmentVariables = envVars
	}

	// Build conversation variables from configuration if provided
	if len(conversationVarsConfig) > 0 {
		// Extract conversationID and appID from system variables
		conversationID := systemVariables.ConversationID
		appID := systemVariables.AppID

		conversationVars := e.buildConversationVariables(conversationVarsConfig, conversationID, appID)
		variablePool.ConversationVariables = conversationVars
	}

	// Re-initialize the variable dictionary with all variables (system, environment, conversation)
	// This is necessary because we changed SystemVariables and ConversationVariables after creation
	variablePool.VariableDictionary = make(map[string]map[string]entities.Variable)
	variablePool.Initialize()

	return variablePool
}

// getWorkflowStatus gets workflow status
func (e *WorkflowExecutor) getWorkflowStatus(err error) string {
	if err != nil {
		return "failed"
	}
	return "succeeded"
}

// collectNodeResultsFromEngine collects node execution results from a specific engine
func (e *WorkflowExecutor) collectNodeResultsFromEngine(engine *graph_engine.WorkflowEngine) map[string]interface{} {
	results := make(map[string]interface{})

	// Get workflow status
	workflowStatus := engine.GetWorkflowStatus()

	for nodeID, status := range workflowStatus {
		if statusMap, ok := status.(map[string]interface{}); ok {
			results[nodeID] = statusMap
		}
	}

	return results
}

// Edge edge definition
type Edge struct {
	From         string `json:"from"`
	To           string `json:"to"`
	SourceHandle string `json:"sourceHandle"`
}

// WorkflowExecutionResult workflow execution result
type WorkflowExecutionResult struct {
	Status         string                               `json:"status"`
	ExecutionTime  time.Duration                        `json:"execution_time"`
	NodeResults    map[string]interface{}               `json:"node_results"`
	NodeExecutions []graph_engine.NodeExecutionSnapshot `json:"node_executions"`
	RuntimeState   *entities.GraphRuntimeState          `json:"-"`
	Error          error                                `json:"error,omitempty"`
}

// ExecuteWorkflowNode executes single workflow node
func (e *WorkflowExecutor) ExecuteWorkflowNode(ctx context.Context, nodeID string, nodeType shared.NodeType, config map[string]interface{}, inputs map[string]interface{}) (*shared.NodeRunResult, error) {
	return e.ExecuteWorkflowNodeWithVariablePool(ctx, nodeID, nodeType, config, inputs, nil)
}

// ExecuteWorkflowNodeWithVariablePool executes single workflow node with shared variable pool
func (e *WorkflowExecutor) ExecuteWorkflowNodeWithVariablePool(ctx context.Context, nodeID string, nodeType shared.NodeType, config map[string]interface{}, inputs map[string]interface{}, sharedVariablePool *entities.VariablePool) (*shared.NodeRunResult, error) {
	logger.Info("Executing single workflow node", "nodeID", nodeID, "nodeType", nodeType)

	// Create or use shared variable pool
	var variablePool *entities.VariablePool
	if sharedVariablePool != nil {
		variablePool = sharedVariablePool
		// Add inputs to the shared variable pool.
		for k, v := range inputs {
			if shouldSeedWorkflowNodeInput(nodeType, k) {
				selector := []string{nodeID, k}
				variablePool.Add(selector, v)
			}
		}
	} else {
		variablePool = e.createVariablePool(inputs)
	}

	// Create runtime state
	runtimeState := &entities.GraphRuntimeState{
		VariablePool: variablePool,
	}

	// Create graph configuration
	graph := &entities.Graph{
		Config: make(map[string]interface{}),
	}

	engine := e.newEngine(runtimeState, graph, graph_engine.EngineCallbacks{})

	// Ensure config contains the node ID
	if config == nil {
		config = make(map[string]interface{})
	}
	// Only set the id if it's missing, don't overwrite existing config
	if _, exists := config["id"]; !exists {
		config["id"] = nodeID
	}

	// Add node to engine
	engine.AddNode(nodeID, nodeType, config)

	// Execute node
	logger.Info("Starting node execution in engine", map[string]interface{}{
		"nodeID":   nodeID,
		"nodeType": nodeType,
	})
	if err := engine.Execute(ctx); err != nil {
		logger.Error(fmt.Sprintf("Engine execution failed for nodeID: %s, nodeType: %s", nodeID, nodeType), err)
		// Get detailed node state for debugging
		if nodeState, exists := engine.GetNodeStatus(nodeID); exists {
			var nodeErrorStr string
			if nodeState.Error != nil {
				nodeErrorStr = nodeState.Error.Error()
			}
			logger.Error(fmt.Sprintf("Failed node state details - nodeID: %s, status: %s, error: %s, startTime: %v, endTime: %v",
				nodeID, nodeState.Status, nodeErrorStr, nodeState.StartTime, nodeState.EndTime),
				fmt.Errorf("node execution failed"))

			// Prefer returning the concrete node error to the client for better diagnosis
			if nodeState.Error != nil {
				return nil, wrapNodeExecutionError(nodeID, nodeState.Error)
			}
		}
		return nil, fmt.Errorf("failed to execute node: %w", err)
	}
	logger.Info("Engine execution completed successfully", map[string]interface{}{
		"nodeID":   nodeID,
		"nodeType": nodeType,
	})

	// Get node status
	nodeState, exists := engine.GetNodeStatus(nodeID)
	if !exists {
		return nil, fmt.Errorf("node %s not found", nodeID)
	}

	// Add node outputs to variable pool
	if sharedVariablePool != nil && nodeState.Outputs != nil {
		for k, v := range nodeState.Outputs {
			// For start nodes, add all variables including system variables
			// For other nodes, filter out system variables
			if nodeType == shared.Start || (k != "sys.agent_id" && k != "sys.files" && k != "sys.user_id" && k != "sys.workflow_id" && k != "sys.workflow_run_id") {
				selector := []string{nodeID, k}
				sharedVariablePool.Add(selector, v)
				logger.Info("Added node output to variable pool", "nodeID", nodeID, "variable", k, "selector", selector)
			}
		}
	}

	// Build result
	result := &shared.NodeRunResult{
		Status:  nodeState.Status,
		Inputs:  nodeState.Inputs,
		Outputs: nodeState.Outputs,
	}

	if nodeState.Error != nil {
		result.Err = nodeState.Error
		result.ErrMsg = nodeState.Error.Error()
		result.ErrType = "NodeExecutionError"
	}

	return result, nil
}

func shouldSeedWorkflowNodeInput(nodeType shared.NodeType, key string) bool {
	if nodeType == shared.QuestionAnswer {
		return false
	}
	if nodeType == shared.Start {
		return true
	}
	switch key {
	case "sys.agent_id", "sys.files", "sys.user_id", "sys.workflow_id", "sys.workflow_run_id":
		return false
	default:
		return true
	}
}

// ExecuteWorkflowNodeWithStreamCallback executes single workflow node with shared variable pool and stream event callback
func (e *WorkflowExecutor) ExecuteWorkflowNodeWithStreamCallback(ctx context.Context, nodeID string, nodeType shared.NodeType, config map[string]interface{}, inputs map[string]interface{}, sharedVariablePool *entities.VariablePool, graphConfig map[string]interface{}, streamCallback func(nodeID string, event *shared.RunStreamChunkEvent)) (*shared.NodeRunResult, error) {
	return e.ExecuteWorkflowNodeWithCallbacks(ctx, nodeID, nodeType, config, inputs, sharedVariablePool, graphConfig, streamCallback, nil)
}

// ExecuteWorkflowNodeWithCallbacks executes single workflow node with shared variable pool, stream event callback, and iteration event callback
func (e *WorkflowExecutor) ExecuteWorkflowNodeWithCallbacks(
	ctx context.Context,
	nodeID string,
	nodeType shared.NodeType,
	config map[string]interface{},
	inputs map[string]interface{},
	sharedVariablePool *entities.VariablePool,
	graphConfig map[string]interface{},
	streamCallback func(nodeID string, event *shared.RunStreamChunkEvent),
	iterationCallback func(event *graph_engine.IterationEvent),
) (*shared.NodeRunResult, error) {
	return e.ExecuteWorkflowNodeWithAllCallbacks(
		ctx,
		nodeID,
		nodeType,
		config,
		inputs,
		sharedVariablePool,
		graphConfig,
		streamCallback,
		iterationCallback,
		nil,
		nil,
	)
}

// ExecuteWorkflowNodeWithAllCallbacks executes single workflow node with all callbacks including internal node events
func (e *WorkflowExecutor) ExecuteWorkflowNodeWithAllCallbacks(
	ctx context.Context,
	nodeID string,
	nodeType shared.NodeType,
	config map[string]interface{},
	inputs map[string]interface{},
	sharedVariablePool *entities.VariablePool,
	graphConfig map[string]interface{},
	streamCallback func(nodeID string, event *shared.RunStreamChunkEvent),
	iterationCallback func(event *graph_engine.IterationEvent),
	loopCallback func(event *graph_engine.LoopEvent),
	internalNodeCallback func(event *graph_engine.NodeEvent),
) (*shared.NodeRunResult, error) {
	logger.Info("Executing single workflow node with all callbacks", "nodeID", nodeID, "nodeType", nodeType)

	// Create or use shared variable pool
	var variablePool *entities.VariablePool
	if sharedVariablePool != nil {
		variablePool = sharedVariablePool
		// Add inputs to the shared variable pool.
		for k, v := range inputs {
			if shouldSeedWorkflowNodeInput(nodeType, k) {
				selector := []string{nodeID, k}
				variablePool.Add(selector, v)
			}
		}
	} else {
		variablePool = e.createVariablePool(inputs)
	}

	// Create runtime state
	runtimeState := &entities.GraphRuntimeState{
		VariablePool: variablePool,
	}

	// Create graph configuration with full workflow data for iteration nodes
	graph := &entities.Graph{
		Config: graphConfig,
	}
	if graph.Config == nil {
		graph.Config = make(map[string]interface{})
	}

	engine := e.newEngine(runtimeState, graph, graph_engine.EngineCallbacks{
		Stream:       streamCallback,
		Iteration:    iterationCallback,
		Loop:         loopCallback,
		InternalNode: internalNodeCallback,
	})

	// Ensure config contains the node ID
	if config == nil {
		config = make(map[string]interface{})
	}
	// Only set the id if it's missing, don't overwrite existing config
	if _, exists := config["id"]; !exists {
		config["id"] = nodeID
	}

	// Add node to engine
	engine.AddNode(nodeID, nodeType, config)

	// Execute node
	logger.Info("Starting node execution in engine with stream callback", map[string]interface{}{
		"nodeID":   nodeID,
		"nodeType": nodeType,
	})
	if err := engine.Execute(ctx); err != nil {
		logger.Error(fmt.Sprintf("Engine execution failed for nodeID: %s, nodeType: %s", nodeID, nodeType), err)
		// Get detailed node state for debugging
		if nodeState, exists := engine.GetNodeStatus(nodeID); exists {
			partialResult := buildNodeRunResultFromState(nodeState)
			var nodeErrorStr string
			if nodeState.Error != nil {
				nodeErrorStr = nodeState.Error.Error()
			}
			logger.Error(fmt.Sprintf("Failed node state details - nodeID: %s, status: %s, error: %s, startTime: %v, endTime: %v",
				nodeID, nodeState.Status, nodeErrorStr, nodeState.StartTime, nodeState.EndTime),
				fmt.Errorf("node execution failed"))

			// Prefer returning the concrete node error to the client for better diagnosis
			if nodeState.Error != nil {
				return partialResult, wrapNodeExecutionError(nodeID, nodeState.Error)
			}
			return partialResult, fmt.Errorf("failed to execute node: %w", err)
		}
		return nil, fmt.Errorf("failed to execute node: %w", err)
	}
	logger.Info("Engine execution completed successfully with stream callback", map[string]interface{}{
		"nodeID":   nodeID,
		"nodeType": nodeType,
	})

	// Get node status
	nodeState, exists := engine.GetNodeStatus(nodeID)
	if !exists {
		return nil, fmt.Errorf("node %s not found", nodeID)
	}

	// Add node outputs to variable pool
	if sharedVariablePool != nil && nodeState.Outputs != nil {
		for k, v := range nodeState.Outputs {
			// For start nodes, add all variables including system variables
			// For other nodes, filter out system variables
			if nodeType == shared.Start || (k != "sys.agent_id" && k != "sys.files" && k != "sys.user_id" && k != "sys.workflow_id" && k != "sys.workflow_run_id") {
				selector := []string{nodeID, k}
				sharedVariablePool.Add(selector, v)
				logger.Info("Added node output to variable pool", "nodeID", nodeID, "variable", k, "selector", selector)
			}
		}
	}

	// Build result
	result := buildNodeRunResultFromState(nodeState)

	return result, nil
}

func buildNodeRunResultFromState(nodeState *graph_engine.NodeState) *shared.NodeRunResult {
	if nodeState == nil {
		return nil
	}

	result := &shared.NodeRunResult{
		Status:           nodeState.Status,
		Inputs:           nodeState.Inputs,
		ProcessData:      nodeState.ProcessData,
		Outputs:          nodeState.Outputs,
		Metadata:         nodeState.Metadata,
		EdgeSourceHandle: nodeState.EdgeSourceHandle,
	}

	if nodeState.Outputs != nil {
		if usage, ok := nodeState.Outputs["usage"].(*shared.LLMUsage); ok && usage != nil {
			result.LLMUsage = usage
		}
	}

	if nodeState.Error != nil {
		result.Err = nodeState.Error
		result.ErrMsg = nodeState.Error.Error()
		result.ErrType = "NodeExecutionError"
	}

	return result
}

// StopWorkflow stops a running workflow by its run ID
func (e *WorkflowExecutor) StopWorkflow(runID string) error {
	e.enginesMu.RLock()
	engine, exists := e.activeEngines[runID]
	e.enginesMu.RUnlock()

	if !exists {
		return fmt.Errorf("workflow run %s not found or not running", runID)
	}

	engine.Stop()
	logger.Info("Stopped workflow engine for run ID: %s", runID)
	return nil
}

// RegisterEngine registers an engine for a workflow run
func (e *WorkflowExecutor) RegisterEngine(workflowRunID string, engine *graph_engine.WorkflowEngine) {
	e.enginesMu.Lock()
	defer e.enginesMu.Unlock()
	e.activeEngines[workflowRunID] = engine
	logger.Info("Registered engine for run ID: %s", workflowRunID)
}

// GetEngine returns the workflow engine instance for a given run ID
func (e *WorkflowExecutor) GetEngine(workflowRunID string) *graph_engine.WorkflowEngine {
	e.enginesMu.RLock()
	defer e.enginesMu.RUnlock()
	return e.activeEngines[workflowRunID]
}

// UnregisterEngine removes an engine from the tracking map
func (e *WorkflowExecutor) UnregisterEngine(workflowRunID string) {
	e.enginesMu.Lock()
	defer e.enginesMu.Unlock()
	delete(e.activeEngines, workflowRunID)
	logger.Info("Unregistered engine for run ID: %s", workflowRunID)
}
