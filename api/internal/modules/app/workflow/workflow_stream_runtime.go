package workflow

import (
	"context"
	"fmt"
	"strings"

	"github.com/google/uuid"
	"github.com/zgiai/zgi/api/internal/dto"
	graph_entities "github.com/zgiai/zgi/api/internal/modules/app/workflow/graph_engine/entities"
	"github.com/zgiai/zgi/api/pkg/logger"
	"go.uber.org/zap"
)

type workflowStreamRuntime struct {
	WorkflowService *WorkflowService
	ElapsedTracker  *workflowElapsedTracker
	Executor        *WorkflowExecutor
}

type workflowStreamVariablePoolScope struct {
	ConversationAccess webAppConversationAccessService
	VariableLoader     workflowStreamConversationVariableLoader
	AgentID            string
	AccountID          string
}

type workflowStreamConversationVariableLoader interface {
	LoadConversationVariables(conversationID uuid.UUID) (map[string]interface{}, error)
}

func (h *WorkflowHandler) loadWorkflowStreamData(ctx context.Context, workspaceID, appID, runType string, isDraft bool) (map[string]any, error) {
	logger.DebugContext(ctx, "workflow configuration load started",
		zap.String("run_type", runType),
		zap.String("workspace_id", workspaceID),
		zap.Bool("is_draft", isDraft),
	)

	var workflow interface{}
	var err error
	switch runType {
	case "WORKFLOW", "CONVERSATION_WORKFLOW":
		if isDraft {
			workflow, err = h.workflowService.GetDraftWorkflow(ctx, appID, false)
		} else {
			workflow, err = h.workflowService.GetLatestPublishedWorkflow(ctx, workspaceID, appID, false)
		}
	default:
		err = fmt.Errorf("unsupported workflow run type: %s", runType)
	}
	if err != nil {
		return nil, fmt.Errorf("failed to get workflow configuration: %w", err)
	}

	workflowData, ok := workflow.(map[string]any)
	if !ok {
		return nil, fmt.Errorf("invalid workflow data format")
	}
	return workflowData, nil
}

func (h *WorkflowHandler) prepareWorkflowStreamRuntime(ctx context.Context, appID, workflowID, workflowRunID string) (workflowStreamRuntime, error) {
	runtime := workflowStreamRuntime{
		ElapsedTracker: newWorkflowElapsedTrackerFromNodeLogs(nil),
	}

	logger.Info("Attempting to cast workflowService to *WorkflowService", "type", fmt.Sprintf("%T", h.workflowService))
	if ws, ok := h.workflowService.(*WorkflowService); ok {
		logger.Info("Successfully cast workflowService to *WorkflowService")
		runtime.WorkflowService = ws
		runtime.ElapsedTracker = ws.newWorkflowElapsedTracker(ctx, workflowRunID)
		logger.Info("Using workflowRunID from parameter", "workflowRunID", workflowRunID)
	} else {
		logger.CriticalContext(ctx, "failed to cast workflow service", fmt.Errorf("actual_type: %T", h.workflowService))
	}

	logger.Info("Getting workflow executor", map[string]interface{}{
		"appID":      appID,
		"workflowID": workflowID,
	})
	executorInterface := h.workflowService.GetExecutor()
	if executorInterface == nil {
		logger.CriticalContext(ctx, "workflow executor not available", fmt.Errorf("executor is nil"))
		return runtime, fmt.Errorf("workflow executor not available")
	}

	logger.Info("Executor interface obtained, checking type", map[string]interface{}{
		"executorType": fmt.Sprintf("%T", executorInterface),
	})
	executor, ok := executorInterface.(*WorkflowExecutor)
	if !ok {
		logger.CriticalContext(ctx, "invalid workflow executor type", fmt.Errorf("expected *WorkflowExecutor, got %T", executorInterface))
		return runtime, fmt.Errorf("invalid executor type")
	}
	runtime.Executor = executor
	logger.Info("Workflow executor ready", map[string]interface{}{
		"appID": appID,
	})
	return runtime, nil
}

func hydrateWorkflowStreamInputs(ctx context.Context, executor *WorkflowExecutor, req *dto.DraftWorkflowRunRequest) {
	if req.Inputs == nil {
		return
	}
	hydratedInputs, err := executor.HydrateInputs(ctx, req.Inputs)
	if err != nil {
		logger.WarnContext(ctx, "failed to hydrate inputs in execute workflow stream, using original inputs", err)
		return
	}
	req.Inputs = hydratedInputs
	logger.Info("Successfully hydrated inputs in executeWorkflowStream")
}

type workflowStreamGraph struct {
	GraphData          map[string]any
	ExecutionGraphData map[string]any
	NodeMap            map[string]map[string]interface{}
	RuntimeNodeMap     map[string]map[string]interface{}
	EdgeMap            map[string]map[string][]string
	ReverseEdgeMap     map[string][]string
	StartNodeID        string
	WatchConfig        streamSelectorWatchConfig
}

func buildWorkflowStreamGraph(ctx context.Context, workflowData map[string]any) (*workflowStreamGraph, error) {
	graphData, err := mergeRootVariablesIntoGraph(workflowData)
	if err != nil {
		return nil, fmt.Errorf("invalid graph data format: %w", err)
	}

	reachabilityViews, err := buildWorkflowGraphReachabilityViews(graphData)
	if err != nil {
		return nil, err
	}

	executionNodeMap, executionEdgeMap, executionReverseEdgeMap, err := buildWorkflowStreamGraphIndexes(reachabilityViews.ExecutionGraphData)
	if err != nil {
		return nil, err
	}
	runtimeNodeMap, _, _, err := buildWorkflowStreamGraphIndexes(reachabilityViews.RuntimeGraphData)
	if err != nil {
		return nil, err
	}

	streamWatchConfig := collectStreamSelectorWatchConfig(runtimeNodeMap)
	watchedSelectors := streamWatchConfig.watchedSelectors
	logger.Info("Watched selectors for text_chunk", "count", len(watchedSelectors))
	logger.Info("Conversation message selectors", "count", len(streamWatchConfig.conversationMessageSelectors))
	logger.DebugContext(ctx, "text chunk stream selectors collected", zap.Int("selector_count", len(watchedSelectors)))

	logger.Debug("workflow edge map built", "edge_count", len(executionEdgeMap))

	return &workflowStreamGraph{
		GraphData:          reachabilityViews.RuntimeGraphData,
		ExecutionGraphData: reachabilityViews.ExecutionGraphData,
		NodeMap:            executionNodeMap,
		RuntimeNodeMap:     runtimeNodeMap,
		EdgeMap:            executionEdgeMap,
		ReverseEdgeMap:     executionReverseEdgeMap,
		StartNodeID:        reachabilityViews.StartNodeID,
		WatchConfig:        streamWatchConfig,
	}, nil
}

func buildWorkflowStreamGraphIndexes(graphData map[string]any) (map[string]map[string]interface{}, map[string]map[string][]string, map[string][]string, error) {
	nodesData, ok := graphData["nodes"].([]interface{})
	if !ok {
		return nil, nil, nil, fmt.Errorf("invalid nodes data format")
	}

	var edgesData []interface{}
	if edgesInterface, exists := graphData["edges"]; exists && edgesInterface != nil {
		if edges, ok := edgesInterface.([]interface{}); ok {
			edgesData = edges
		}
	}

	nodeMap := make(map[string]map[string]interface{})
	edgeMap := make(map[string]map[string][]string)
	reverseEdgeMap := make(map[string][]string)

	for _, nodeInterface := range nodesData {
		if node, ok := nodeInterface.(map[string]interface{}); ok {
			if nodeID, ok := node["id"].(string); ok {
				nodeMap[nodeID] = node
			}
		}
	}

	for _, edgeInterface := range edgesData {
		if edge, ok := edgeInterface.(map[string]interface{}); ok {
			if source, ok := edge["source"].(string); ok {
				if target, ok := edge["target"].(string); ok {
					sourceHandle, _ := edge["sourceHandle"].(string)
					if sourceHandle == "" {
						sourceHandle = "source"
					}

					if edgeMap[source] == nil {
						edgeMap[source] = make(map[string][]string)
					}

					edgeMap[source][sourceHandle] = append(edgeMap[source][sourceHandle], target)
					reverseEdgeMap[target] = append(reverseEdgeMap[target], source)
				}
			}
		}
	}

	return nodeMap, edgeMap, reverseEdgeMap, nil
}

// addVariablesToPool adds variables from graphData to the shared variable pool.
func addVariablesToPool(pool *graph_entities.VariablePool, graphData map[string]any, key string, nodeID string, persistedVars map[string]any) {
	vars := extractVariableConfigList(graphData[key])

	for i, variable := range vars {
		name, _ := variable["name"].(string)
		name = strings.TrimSpace(name)

		if name == "" {
			logger.Warn("Skip variable config with missing name", map[string]any{
				"key":   key,
				"index": i,
			})
			continue
		}

		valueType := resolveVariableType(variable)
		value := variable["value"]
		if persistedVars != nil {
			if persistedValue, exists := persistedVars[name]; exists {
				value = persistedValue
			}
		}

		segment, warnings, err := graph_entities.ConvertValue(valueType, value, graph_entities.ValueConversionLenient)
		if err != nil {
			logger.Warn("Variable conversion failed in streaming path, defaulting to string", map[string]any{
				"key":        key,
				"name":       name,
				"value_type": valueType,
				"error":      err.Error(),
			})
			segment = &graph_entities.StringSegment{Value: fmt.Sprintf("%v", value)}
		}

		for _, warning := range warnings {
			logger.Warn("Variable conversion warning in streaming path", map[string]any{
				"key":        key,
				"name":       name,
				"value_type": valueType,
				"detail":     warning,
			})
		}

		selector := []string{nodeID, name}
		pool.AddSegment(selector, segment)
	}
}

func buildWorkflowStreamVariablePool(ctx context.Context, graphData map[string]any, systemInputs map[string]interface{}, reqInputs map[string]interface{}, startNodeID string, scope workflowStreamVariablePoolScope) (*graph_entities.VariablePool, error) {
	sharedVariablePool := &graph_entities.VariablePool{
		VariableDictionary:    make(map[string]map[string]graph_entities.Variable),
		UserInputs:            make(map[string]interface{}),
		SystemVariables:       graph_entities.SystemVariableEmpty(),
		EnvironmentVariables:  make([]graph_entities.Variable, 0),
		ConversationVariables: make([]graph_entities.Variable, 0),
	}

	addVariablesToPool(sharedVariablePool, graphData, "environment_variables", graph_entities.EnvironmentVariableNodeId, nil)

	var persistedConversationVars map[string]interface{}
	conversationVariableConfigs := extractVariableConfigList(graphData["conversation_variables"])
	if conversationID, ok := systemInputs["sys.conversation_id"].(string); ok && conversationID != "" && len(conversationVariableConfigs) > 0 {
		var err error
		persistedConversationVars, err = loadWorkflowStreamConversationVariables(ctx, conversationID, scope)
		if err != nil {
			return nil, err
		}
	}

	addVariablesToPool(sharedVariablePool, graphData, "conversation_variables", graph_entities.ConversationVariableNodeId, persistedConversationVars)
	addWorkflowStreamSystemVariables(ctx, sharedVariablePool, systemInputs)
	addWorkflowStreamUserInputs(ctx, sharedVariablePool, reqInputs, startNodeID)

	return sharedVariablePool, nil
}

func loadWorkflowStreamConversationVariables(ctx context.Context, conversationID string, scope workflowStreamVariablePoolScope) (map[string]interface{}, error) {
	conversationID = strings.TrimSpace(conversationID)
	if conversationID == "" {
		return nil, nil
	}
	if scope.ConversationAccess == nil || scope.VariableLoader == nil || strings.TrimSpace(scope.AgentID) == "" || strings.TrimSpace(scope.AccountID) == "" {
		return nil, fmt.Errorf("workflow stream conversation variable scope missing")
	}
	if err := validateWebAppConversationAccess(ctx, scope.ConversationAccess, conversationID, scope.AgentID, scope.AccountID); err != nil {
		return nil, fmt.Errorf("workflow stream conversation variable access denied: %w", err)
	}
	convUUID, err := uuid.Parse(conversationID)
	if err != nil {
		return nil, fmt.Errorf("invalid workflow stream conversation id: %w", err)
	}
	persistedConversationVars, err := scope.VariableLoader.LoadConversationVariables(convUUID)
	if err != nil {
		logger.WarnContext(ctx, "failed to load persisted conversation variables",
			zap.String("conversation_id", conversationID),
			zap.Error(err),
		)
		return nil, nil
	}
	logger.DebugContext(ctx, "persisted conversation variables loaded",
		zap.String("conversation_id", conversationID),
		zap.Int("conversation_variable_count", len(persistedConversationVars)),
	)
	return persistedConversationVars, nil
}

func addWorkflowStreamSystemVariables(ctx context.Context, sharedVariablePool *graph_entities.VariablePool, systemInputs map[string]interface{}) {
	if userID, ok := systemInputs["sys.user_id"].(string); ok {
		sharedVariablePool.SystemVariables.UserID = userID
	}
	if agentID, ok := systemInputs["sys.agent_id"].(string); ok {
		sharedVariablePool.SystemVariables.AppID = agentID
	}
	if workflowID, ok := systemInputs["sys.workflow_id"].(string); ok {
		sharedVariablePool.SystemVariables.WorkflowID = workflowID
	}
	if workspaceID, ok := systemInputs["sys.workspace_id"].(string); ok {
		sharedVariablePool.SystemVariables.WorkspaceID = workspaceID
		sharedVariablePool.SystemVariables.TenantID = workspaceID
	}
	if organizationID, ok := systemInputs["sys.organization_id"].(string); ok {
		sharedVariablePool.SystemVariables.OrganizationID = organizationID
	}
	if billingSubjectType, ok := systemInputs["sys.billing_subject_type"].(string); ok {
		sharedVariablePool.SystemVariables.BillingSubjectType = billingSubjectType
	}
	if query, ok := systemInputs["sys.query"].(string); ok {
		sharedVariablePool.SystemVariables.Query = query
	}
	if conversationID, ok := systemInputs["sys.conversation_id"].(string); ok {
		sharedVariablePool.SystemVariables.ConversationID = conversationID
		logger.DebugContext(ctx, "set conversation id in shared variable pool",
			zap.String("conversation_id", conversationID),
		)
	} else {
		logger.WarnContext(ctx, "conversation id missing from workflow system inputs",
			zap.Int("system_input_count", len(systemInputs)),
		)
		sharedVariablePool.SystemVariables.ConversationID = ""
	}
	if dialogueCount, ok := systemInputs["sys.dialogue_count"].(int); ok {
		sharedVariablePool.SystemVariables.DialogueCount = dialogueCount
	} else {
		sharedVariablePool.SystemVariables.DialogueCount = 1
	}
	if workflowRunID, ok := systemInputs["sys.workflow_run_id"].(string); ok {
		sharedVariablePool.SystemVariables.WorkflowRunID = workflowRunID
	}

	for key, value := range sharedVariablePool.SystemVariables.ToDict() {
		if value != nil {
			sharedVariablePool.Add([]string{"sys", key}, value)
		}
	}

	if conversationHistory, exists := systemInputs["sys.conversation_history"]; exists {
		sharedVariablePool.Add([]string{"sys", "conversation_history"}, conversationHistory)
		logger.DebugContext(ctx, "added conversation history to shared variable pool",
			zap.Int("history_messages_count", conversationHistoryCount(conversationHistory)),
		)
	}
	if files, exists := systemInputs["sys.files"]; exists && files != nil {
		sharedVariablePool.Add([]string{"sys", "files"}, files)
	}
	if parentMessageID, exists := systemInputs["sys.parent_message_id"]; exists {
		sharedVariablePool.Add([]string{"sys", "parent_message_id"}, parentMessageID)
		logger.DebugContext(ctx, "added parent message id to shared variable pool",
			zap.Bool("has_parent_message_id", parentMessageID != nil),
		)
	}
}

func addWorkflowStreamUserInputs(ctx context.Context, sharedVariablePool *graph_entities.VariablePool, reqInputs map[string]interface{}, startNodeID string) {
	if reqInputs == nil {
		return
	}

	for k, v := range reqInputs {
		sharedVariablePool.UserInputs[k] = v

		if k == "model_config" {
			sharedVariablePool.Add([]string{"sys", "model_config"}, v)
			logger.DebugContext(ctx, "added model config override to shared variable pool")
		}

		sharedVariablePool.Add([]string{startNodeID, k}, v)
	}
}
