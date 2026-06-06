package workflow

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/zgiai/zgi/api/internal/dto"
	"github.com/zgiai/zgi/api/internal/modules/app/workflow/graph_engine"
	graph_entities "github.com/zgiai/zgi/api/internal/modules/app/workflow/graph_engine/entities"
	workflowpause "github.com/zgiai/zgi/api/internal/modules/app/workflow/pause"
	workflowshared "github.com/zgiai/zgi/api/internal/modules/app/workflow/shared"
	automationaction "github.com/zgiai/zgi/api/internal/modules/automation/service/action"
	"github.com/zgiai/zgi/api/pkg/database"
	"github.com/zgiai/zgi/api/pkg/logger"
	"go.uber.org/zap"
)

const automationTriggeredFrom = string(InvokeFromAutomation)

const automationFinalizationTimeout = 30 * time.Second

const (
	automationApprovalFormIDKey       = "__approval_form_id"
	automationApprovalTokenKey        = "__approval_token"
	automationApprovalFormKey         = "__approval_form"
	automationApprovalFormAliasKey    = "approval_form"
	automationApprovalFormIDAliasKey  = "approval_form_id"
	automationApprovalTokenAliasKey   = "approval_token"
	automationApprovalFormURLAliasKey = "approval_url"
)

const (
	automationWorkflowEventStarted           = "workflow_started"
	automationWorkflowEventNodeStarted       = "node_started"
	automationWorkflowEventNodeFinished      = "node_finished"
	automationWorkflowEventPaused            = "workflow_paused"
	automationWorkflowEventApprovalRequested = "approval_requested"
	automationWorkflowEventFinished          = "workflow_finished"
	automationWorkflowEventFailed            = "workflow_failed"
)

// RunAutomationWorkflow executes a published workflow from an automation action.
func (s *WorkflowService) RunAutomationWorkflow(ctx context.Context, req automationaction.WorkflowRunRequest) (*automationaction.WorkflowRunResult, error) {
	if s == nil {
		return nil, fmt.Errorf("workflow service is not configured")
	}
	if s.repo == nil {
		return nil, fmt.Errorf("workflow repository is not configured")
	}
	if s.executor == nil {
		return nil, fmt.Errorf("workflow executor is not configured")
	}
	if req.WorkspaceID == "" {
		return nil, fmt.Errorf("workspace_id is required")
	}
	if req.AccountID == "" {
		return nil, fmt.Errorf("account_id is required")
	}

	target, err := s.resolveAutomationWorkflowTarget(ctx, req.WorkflowRef)
	if err != nil {
		return nil, err
	}
	if target.TenantID != "" && target.TenantID != req.WorkspaceID {
		return nil, fmt.Errorf("workflow %s does not belong to workspace %s", target.ID, req.WorkspaceID)
	}

	graphData, err := automationWorkflowGraphData(target)
	if err != nil {
		return nil, err
	}

	inputs := automationWorkflowInputs(req, target)
	runCtx := context.WithValue(ctx, "invoke_from", automationTriggeredFrom)
	runCtx = context.WithValue(runCtx, "created_from", automationTriggeredFrom)
	runCtx = context.WithValue(runCtx, "created_by_role", string(CreatedByRoleAccount))

	workflowRunLog, err := s.CreateWorkflowRunLog(runCtx, req.WorkspaceID, target.AgentID, target.ID, automationTriggeredFrom, inputs, req.AccountID)
	if err != nil {
		return nil, fmt.Errorf("create automation workflow run log: %w", err)
	}

	workflowRunLogID := ""
	if typed, ok := workflowRunLog.(*WorkflowRunLog); ok && typed != nil {
		workflowRunLogID = typed.ID
	}
	if workflowRunLogID == "" {
		return nil, fmt.Errorf("create automation workflow run log returned empty id")
	}

	inputs["sys.workflow_run_id"] = workflowRunLogID
	nodeMetas := automationWorkflowNodeMetas(graphData)
	emitAutomationWorkflowStarted(req.EventSink, req, target, workflowRunLogID)

	startedAt := time.Now()
	executionResult, execErr := s.executor.ExecuteSimpleWorkflowWithRunIDAndCallbacks(runCtx, workflowRunLogID, graphData, inputs, graph_engine.EngineCallbacks{
		NodeStarted: func(nodeID string, nodeType string, inputs map[string]any) {
			meta := automationWorkflowEventNodeMeta(nodeMetas, nodeID, nodeType)
			emitAutomationWorkflowNodeStarted(req.EventSink, req, target, workflowRunLogID, meta, inputs)
		},
		NodeFinished: func(nodeID string, nodeType string, status string, outputs map[string]any, edgeSourceHandle string, err error) {
			_ = edgeSourceHandle
			meta := automationWorkflowEventNodeMeta(nodeMetas, nodeID, nodeType)
			emitAutomationWorkflowNodeFinished(req.EventSink, req, target, workflowRunLogID, meta, status, outputs, err)
		},
	})
	elapsedTime := time.Since(startedAt).Seconds()
	finalizeCtx, cancelFinalize := automationFinalizationContext(runCtx)
	defer cancelFinalize()

	outputs := map[string]interface{}{}
	totalSteps := 0
	if executionResult != nil {
		outputs = automationWorkflowOutputs(executionResult)
		totalSteps = len(executionResult.NodeResults)
		if executionResult.ExecutionTime > 0 {
			elapsedTime = executionResult.ExecutionTime.Seconds()
		}
	}

	if err := s.persistAutomationWorkflowNodeRuntimeLogs(finalizeCtx, req.WorkspaceID, req.AccountID, target, graphData, workflowRunLogID, executionResult); err != nil {
		logger.ErrorContext(finalizeCtx, "failed to persist automation workflow node runtime logs",
			zap.String("workflow_run_id", workflowRunLogID),
			zap.String("workflow_id", target.ID),
			zap.String("agent_id", target.AgentID),
			zap.Error(err))
	}

	status := string(dto.WorkflowRunStatusSucceeded)
	errorMessage := ""
	if execErr != nil {
		status = string(dto.WorkflowRunStatusFailed)
		errorMessage = execErr.Error()
	} else if executionResult != nil && strings.EqualFold(strings.TrimSpace(executionResult.Status), string(dto.WorkflowRunStatusPaused)) {
		status = string(dto.WorkflowRunStatusPaused)
	}

	if status == string(dto.WorkflowRunStatusPaused) {
		if err := s.PauseWorkflowRunLog(finalizeCtx, workflowRunLogID, outputs, elapsedTime, 0, totalSteps); err != nil {
			return nil, fmt.Errorf("pause automation workflow run log: %w", err)
		}
		if err := persistAutomationWorkflowPause(finalizeCtx, req, target, workflowRunLogID, executionResult, inputs, outputs, totalSteps); err != nil {
			return nil, fmt.Errorf("save automation workflow pause: %w", err)
		}
		emitAutomationWorkflowPaused(req.EventSink, req, target, workflowRunLogID, executionResult, outputs, elapsedTime, nodeMetas)
	} else if err := s.UpdateWorkflowRunLogStatus(finalizeCtx, workflowRunLogID, status, outputs, elapsedTime, 0, totalSteps, errorMessage); err != nil {
		return nil, fmt.Errorf("update automation workflow run log: %w", err)
	}
	if execErr != nil {
		emitAutomationWorkflowFailed(req.EventSink, req, target, workflowRunLogID, outputs, elapsedTime, execErr)
		return &automationaction.WorkflowRunResult{
			WorkflowRunID: workflowRunLogID,
			WorkflowID:    target.ID,
			AgentID:       target.AgentID,
			Version:       target.Version,
			Status:        status,
			Outputs:       outputs,
			ElapsedTime:   elapsedTime,
		}, fmt.Errorf("workflow execution failed: %w", execErr)
	}
	if status != string(dto.WorkflowRunStatusPaused) {
		emitAutomationWorkflowFinished(req.EventSink, req, target, workflowRunLogID, outputs, elapsedTime)
	}

	return &automationaction.WorkflowRunResult{
		WorkflowRunID: workflowRunLogID,
		WorkflowID:    target.ID,
		AgentID:       target.AgentID,
		Version:       target.Version,
		Status:        status,
		Outputs:       outputs,
		ElapsedTime:   elapsedTime,
	}, nil
}

func emitAutomationWorkflowStarted(sink automationaction.WorkflowRunEventSink, req automationaction.WorkflowRunRequest, workflow *Workflow, workflowRunID string) {
	emitAutomationWorkflowEvent(sink, automationWorkflowEventStarted, automationWorkflowBasePayload(req, workflow, workflowRunID, map[string]interface{}{
		"id":         workflowRunID,
		"status":     "running",
		"inputs":     copyWorkflowAnyMap(req.Inputs),
		"created_at": time.Now().Unix(),
	}))
}

func emitAutomationWorkflowNodeStarted(sink automationaction.WorkflowRunEventSink, req automationaction.WorkflowRunRequest, workflow *Workflow, workflowRunID string, meta automationWorkflowNodeMeta, inputs map[string]any) {
	payload := automationWorkflowBasePayload(req, workflow, workflowRunID, automationWorkflowNodePayload(meta))
	payload["status"] = "running"
	payload["inputs"] = copyWorkflowAnyMap(inputs)
	payload["created_at"] = time.Now().Unix()
	emitAutomationWorkflowEvent(sink, automationWorkflowEventNodeStarted, payload)
}

func emitAutomationWorkflowNodeFinished(sink automationaction.WorkflowRunEventSink, req automationaction.WorkflowRunRequest, workflow *Workflow, workflowRunID string, meta automationWorkflowNodeMeta, status string, outputs map[string]any, err error) {
	payload := automationWorkflowBasePayload(req, workflow, workflowRunID, automationWorkflowNodePayload(meta))
	payload["status"] = strings.TrimSpace(status)
	payload["outputs"] = copyWorkflowAnyMap(outputs)
	payload["created_at"] = time.Now().Unix()
	if err != nil {
		payload["error"] = err.Error()
	}
	emitAutomationWorkflowEvent(sink, automationWorkflowEventNodeFinished, payload)
}

func emitAutomationWorkflowPaused(sink automationaction.WorkflowRunEventSink, req automationaction.WorkflowRunRequest, workflow *Workflow, workflowRunID string, result *WorkflowExecutionResult, outputs map[string]interface{}, elapsedTime float64, nodeMetas map[string]automationWorkflowNodeMeta) {
	pausedSnapshots := workflowGraphPausedSnapshots(nil)
	if result != nil {
		pausedSnapshots = workflowGraphPausedSnapshots(result.NodeExecutions)
	}
	nodeIDs := make([]string, 0, len(pausedSnapshots))
	for _, snapshot := range pausedSnapshots {
		nodeIDs = append(nodeIDs, snapshot.NodeID)
	}
	payload := automationWorkflowBasePayload(req, workflow, workflowRunID, map[string]interface{}{
		"status":       "pending_approval",
		"node_ids":     nodeIDs,
		"elapsed_time": elapsedTime,
		"outputs":      copyWorkflowAnyMap(outputs),
		"created_at":   time.Now().Unix(),
	})
	if len(pausedSnapshots) > 0 {
		meta := automationWorkflowEventNodeMeta(nodeMetas, pausedSnapshots[0].NodeID, string(pausedSnapshots[0].NodeType))
		for key, value := range automationWorkflowNodePayload(meta) {
			payload[key] = value
		}
	}
	emitAutomationWorkflowEvent(sink, automationWorkflowEventPaused, payload)
	emitAutomationWorkflowApprovalRequested(sink, req, workflow, workflowRunID, outputs, elapsedTime, pausedSnapshots, nodeMetas)
}

func emitAutomationWorkflowApprovalRequested(sink automationaction.WorkflowRunEventSink, req automationaction.WorkflowRunRequest, workflow *Workflow, workflowRunID string, outputs map[string]interface{}, elapsedTime float64, pausedSnapshots []graph_engine.NodeExecutionSnapshot, nodeMetas map[string]automationWorkflowNodeMeta) {
	payload := automationWorkflowBasePayload(req, workflow, workflowRunID, map[string]interface{}{
		"status":       "pending_approval",
		"elapsed_time": elapsedTime,
		"created_at":   time.Now().Unix(),
	})
	for key, value := range automationApprovalEventFields(outputs) {
		payload[key] = value
	}
	if len(pausedSnapshots) > 0 {
		meta := automationWorkflowEventNodeMeta(nodeMetas, pausedSnapshots[0].NodeID, string(pausedSnapshots[0].NodeType))
		for key, value := range automationWorkflowNodePayload(meta) {
			payload[key] = value
		}
	}
	emitAutomationWorkflowEvent(sink, automationWorkflowEventApprovalRequested, payload)
}

func emitAutomationWorkflowFinished(sink automationaction.WorkflowRunEventSink, req automationaction.WorkflowRunRequest, workflow *Workflow, workflowRunID string, outputs map[string]interface{}, elapsedTime float64) {
	emitAutomationWorkflowEvent(sink, automationWorkflowEventFinished, automationWorkflowBasePayload(req, workflow, workflowRunID, map[string]interface{}{
		"status":       "completed",
		"outputs":      copyWorkflowAnyMap(outputs),
		"elapsed_time": elapsedTime,
		"created_at":   time.Now().Unix(),
	}))
}

func emitAutomationWorkflowFailed(sink automationaction.WorkflowRunEventSink, req automationaction.WorkflowRunRequest, workflow *Workflow, workflowRunID string, outputs map[string]interface{}, elapsedTime float64, err error) {
	payload := automationWorkflowBasePayload(req, workflow, workflowRunID, map[string]interface{}{
		"status":       "failed",
		"outputs":      copyWorkflowAnyMap(outputs),
		"elapsed_time": elapsedTime,
		"created_at":   time.Now().Unix(),
	})
	if err != nil {
		payload["error"] = err.Error()
	}
	emitAutomationWorkflowEvent(sink, automationWorkflowEventFailed, payload)
}

func persistAutomationWorkflowPause(ctx context.Context, req automationaction.WorkflowRunRequest, workflow *Workflow, workflowRunID string, result *WorkflowExecutionResult, inputs map[string]interface{}, outputs map[string]interface{}, totalSteps int) error {
	if result == nil {
		return nil
	}
	pausedSnapshots := workflowGraphPausedSnapshots(result.NodeExecutions)
	if len(pausedSnapshots) == 0 {
		return nil
	}
	pausedNodeIDs := make([]string, 0, len(pausedSnapshots))
	reasons := make([]workflowpause.Reason, 0, len(pausedSnapshots))
	for _, snapshot := range pausedSnapshots {
		if snapshot.NodeID == "" {
			continue
		}
		pausedNodeIDs = append(pausedNodeIDs, snapshot.NodeID)
		reasons = append(reasons, workflowpause.Reason{
			Type:   workflowpause.ReasonTypeApprovalRequired,
			NodeID: snapshot.NodeID,
			FormID: automationApprovalFormID(snapshot.Outputs, outputs),
		})
	}
	if len(pausedNodeIDs) == 0 {
		return nil
	}
	nodeQueue, completedNodes, failedNodes, executionOutputs := workflowGraphPauseExecutorState(result.NodeExecutions, pausedNodeIDs)

	var variablePool *graph_entities.VariablePool
	if result.RuntimeState != nil {
		variablePool = result.RuntimeState.VariablePool
	}
	appID := req.WorkflowRef.AgentID
	workflowID := req.WorkflowRef.WorkflowID
	runType := "WORKFLOW"
	if workflow != nil {
		if workflow.AgentID != "" {
			appID = workflow.AgentID
		}
		if workflow.ID != "" {
			workflowID = workflow.ID
		}
		runType = automationWorkflowPauseRunType(workflow)
	}
	pauseState := workflowpause.State{
		Version:       workflowpause.StateVersion,
		WorkflowRunID: workflowRunID,
		WorkflowID:    workflowID,
		AppID:         appID,
		TenantID:      req.WorkspaceID,
		RunType:       runType,
		TriggeredFrom: automationTriggeredFrom,
		Request: workflowpause.RequestState{
			Inputs:       copyWorkflowAnyMap(inputs),
			ResponseMode: "streaming",
		},
		ExecutorState: workflowpause.ExecutorState{
			PausedNodeID:     pausedNodeIDs[0],
			PausedNodeIDs:    append([]string(nil), pausedNodeIDs...),
			NodeQueue:        append([]string(nil), nodeQueue...),
			CompletedNodes:   copyWorkflowBoolMap(completedNodes),
			FailedNodes:      copyWorkflowStringMap(failedNodes),
			ExecutionOutputs: copyWorkflowNestedMap(executionOutputs),
			AllNodeOutputs:   copyWorkflowAnyMap(outputs),
			NodeIndex:        totalSteps,
			TotalTokens:      0,
		},
		VariablePool: workflowpause.SnapshotVariablePool(variablePool),
	}
	service := workflowpause.NewService(database.GetDB())
	_, err := service.Save(ctx, workflowpause.SaveParams{
		TenantID:       req.WorkspaceID,
		AppID:          appID,
		WorkflowRunID:  workflowRunID,
		NodeID:         pausedNodeIDs[0],
		Reason:         workflowpause.ReasonTypeApprovalRequired,
		ConversationID: automationWorkflowConversationID(inputs),
		State:          pauseState,
		Reasons:        reasons,
	})
	return err
}

func automationWorkflowPauseRunType(workflow *Workflow) string {
	if workflow != nil && workflow.Type == dto.WorkflowTypeChat {
		return "CONVERSATION_WORKFLOW"
	}
	return "WORKFLOW"
}

func automationApprovalFormID(values ...map[string]interface{}) string {
	for _, value := range values {
		if value == nil {
			continue
		}
		if formID, ok := value[automationApprovalFormIDKey].(string); ok && strings.TrimSpace(formID) != "" {
			return strings.TrimSpace(formID)
		}
		if form, ok := value[automationApprovalFormKey].(map[string]interface{}); ok {
			if formID, ok := form["id"].(string); ok && strings.TrimSpace(formID) != "" {
				return strings.TrimSpace(formID)
			}
		}
	}
	return ""
}

func automationWorkflowConversationID(inputs map[string]interface{}) string {
	if inputs == nil {
		return ""
	}
	if conversationID, ok := inputs["sys.conversation_id"].(string); ok {
		return strings.TrimSpace(conversationID)
	}
	return ""
}

func emitAutomationWorkflowEvent(sink automationaction.WorkflowRunEventSink, eventType string, payload map[string]interface{}) {
	if sink == nil {
		return
	}
	if payload == nil {
		payload = map[string]interface{}{}
	}
	sink(automationaction.WorkflowRunEvent{Type: eventType, Payload: payload})
}

func automationWorkflowBasePayload(req automationaction.WorkflowRunRequest, workflow *Workflow, workflowRunID string, payload map[string]interface{}) map[string]interface{} {
	if payload == nil {
		payload = map[string]interface{}{}
	}
	payload["workflow_run_id"] = workflowRunID
	payload["task_id"] = workflowRunID
	payload["workflow_id"] = req.WorkflowRef.WorkflowID
	payload["agent_id"] = req.WorkflowRef.AgentID
	payload["version"] = req.WorkflowRef.VersionUUID
	if workflow != nil {
		payload["workflow_id"] = workflow.ID
		payload["agent_id"] = workflow.AgentID
		payload["version"] = workflow.Version
	}
	return payload
}

func automationWorkflowEventNodeMeta(nodeMetas map[string]automationWorkflowNodeMeta, nodeID string, nodeType string) automationWorkflowNodeMeta {
	meta := nodeMetas[nodeID]
	if meta.NodeID == "" {
		meta.NodeID = nodeID
	}
	if meta.NodeType == "" {
		meta.NodeType = nodeType
	}
	if meta.Title == "" {
		meta.Title = meta.NodeID
	}
	return meta
}

func automationWorkflowNodePayload(meta automationWorkflowNodeMeta) map[string]interface{} {
	payload := map[string]interface{}{
		"id":        meta.NodeID,
		"node_id":   meta.NodeID,
		"node_type": meta.NodeType,
		"title":     meta.Title,
		"index":     meta.Index,
	}
	if meta.PredecessorNodeID != nil {
		payload["predecessor_node_id"] = *meta.PredecessorNodeID
	}
	return payload
}

func automationApprovalEventFields(outputs map[string]interface{}) map[string]interface{} {
	fields := map[string]interface{}{}
	if outputs == nil {
		return fields
	}
	copyAutomationApprovalField(fields, outputs, automationApprovalFormIDKey, automationApprovalFormIDAliasKey)
	copyAutomationApprovalField(fields, outputs, automationApprovalTokenKey, automationApprovalTokenAliasKey)
	copyAutomationApprovalField(fields, outputs, automationApprovalFormURLAliasKey, automationApprovalFormURLAliasKey)
	copyAutomationApprovalField(fields, outputs, automationApprovalFormKey, automationApprovalFormAliasKey)
	copyAutomationApprovalField(fields, outputs, automationApprovalFormAliasKey, automationApprovalFormAliasKey)
	return fields
}

// GetAutomationWorkflowRunStatus returns a safe run summary for an automation-triggered workflow.
func (s *WorkflowService) GetAutomationWorkflowRunStatus(ctx context.Context, req automationaction.WorkflowRunStatusRequest) (*automationaction.WorkflowRunStatusResult, error) {
	if s == nil {
		return nil, fmt.Errorf("workflow service is not configured")
	}
	if s.workflowRunLogRepo == nil {
		return nil, fmt.Errorf("workflow run log repository is not configured")
	}
	workflowRunID := strings.TrimSpace(req.WorkflowRunID)
	if workflowRunID == "" {
		return nil, fmt.Errorf("workflow_run_id is required")
	}
	run, err := s.workflowRunLogRepo.GetByID(ctx, workflowRunID)
	if err != nil {
		return nil, err
	}
	if run == nil {
		return nil, fmt.Errorf("workflow run not found")
	}
	if workspaceID := strings.TrimSpace(req.WorkspaceID); workspaceID != "" && strings.TrimSpace(run.TenantID) != workspaceID {
		return nil, fmt.Errorf("workflow run %s does not belong to workspace %s", workflowRunID, workspaceID)
	}
	if agentID := strings.TrimSpace(req.AgentID); agentID != "" && strings.TrimSpace(run.AgentID) != agentID {
		return nil, fmt.Errorf("workflow run %s does not belong to agent %s", workflowRunID, agentID)
	}
	if accountID := strings.TrimSpace(req.AccountID); accountID != "" && strings.TrimSpace(run.CreatedBy) != accountID {
		return nil, fmt.Errorf("workflow run %s is not owned by account %s", workflowRunID, accountID)
	}

	outputs := run.GetOutputsDict()
	errorMessage := ""
	if run.Error != nil {
		errorMessage = strings.TrimSpace(*run.Error)
	}
	result := &automationaction.WorkflowRunStatusResult{
		WorkflowRunID: workflowRunID,
		WorkflowID:    run.WorkflowID,
		AgentID:       run.AgentID,
		Version:       run.Version,
		Status:        string(run.Status),
		Outputs:       outputs,
		Error:         errorMessage,
		ElapsedTime:   run.ElapsedTime,
		CreatedAtUnix: run.CreatedAt.Unix(),
	}
	if run.FinishedAt != nil {
		result.FinishedAtUnix = run.FinishedAt.Unix()
	}
	return result, nil
}

func automationWorkflowOutputs(result *WorkflowExecutionResult) map[string]interface{} {
	if result == nil {
		return map[string]interface{}{}
	}
	outputs := workflowExecutionOutputs(result)
	if outputs == nil {
		outputs = map[string]interface{}{}
	}
	mergeAutomationApprovalOutputs(outputs, result.NodeExecutions)
	return outputs
}

func mergeAutomationApprovalOutputs(outputs map[string]interface{}, snapshots []graph_engine.NodeExecutionSnapshot) {
	if outputs == nil {
		return
	}
	for _, snapshot := range snapshots {
		if len(snapshot.Outputs) == 0 || !isAutomationApprovalSnapshot(snapshot) {
			continue
		}
		copyAutomationApprovalField(outputs, snapshot.Outputs, automationApprovalFormIDKey, automationApprovalFormIDKey)
		copyAutomationApprovalField(outputs, snapshot.Outputs, automationApprovalTokenKey, automationApprovalTokenKey)
		copyAutomationApprovalField(outputs, snapshot.Outputs, automationApprovalFormKey, automationApprovalFormKey)
		copyAutomationApprovalField(outputs, snapshot.Outputs, automationApprovalFormAliasKey, automationApprovalFormAliasKey)
		copyAutomationApprovalField(outputs, snapshot.Outputs, automationApprovalFormIDAliasKey, automationApprovalFormIDAliasKey)
		copyAutomationApprovalField(outputs, snapshot.Outputs, automationApprovalTokenAliasKey, automationApprovalTokenAliasKey)
		copyAutomationApprovalField(outputs, snapshot.Outputs, automationApprovalFormURLAliasKey, automationApprovalFormURLAliasKey)
	}
}

func isAutomationApprovalSnapshot(snapshot graph_engine.NodeExecutionSnapshot) bool {
	if snapshot.NodeType == workflowshared.Approval {
		return true
	}
	if _, ok := snapshot.Outputs[automationApprovalFormIDKey]; ok {
		return true
	}
	if _, ok := snapshot.Outputs[automationApprovalTokenKey]; ok {
		return true
	}
	if _, ok := snapshot.Outputs[automationApprovalFormKey]; ok {
		return true
	}
	return false
}

func copyAutomationApprovalField(target, source map[string]interface{}, sourceKey, targetKey string) {
	value, ok := source[sourceKey]
	if !ok || isEmptyAutomationApprovalValue(value) {
		return
	}
	if existing, exists := target[targetKey]; exists && !isEmptyAutomationApprovalValue(existing) {
		return
	}
	target[targetKey] = value
}

func isEmptyAutomationApprovalValue(value interface{}) bool {
	if value == nil {
		return true
	}
	if typed, ok := value.(string); ok {
		return strings.TrimSpace(typed) == ""
	}
	return false
}

func (s *WorkflowService) resolveAutomationWorkflowTarget(ctx context.Context, ref automationaction.WorkflowRef) (*Workflow, error) {
	if ref.AgentID == "" {
		return nil, fmt.Errorf("workflow_ref.agent_id is required")
	}

	switch ref.VersionStrategy {
	case "", automationaction.WorkflowVersionStrategyLatestPublished:
		workflow, err := s.repo.GetLatestPublishedVersion(ctx, ref.AgentID)
		if err != nil {
			return nil, fmt.Errorf("get latest published workflow for agent %s: %w", ref.AgentID, err)
		}
		if workflow == nil {
			return nil, fmt.Errorf("latest published workflow not found for agent %s", ref.AgentID)
		}
		if err := validateAutomationWorkflowTarget(workflow, ref, false); err != nil {
			return nil, err
		}
		return workflow, nil
	case automationaction.WorkflowVersionStrategyPinned:
		if ref.VersionUUID == "" {
			return nil, fmt.Errorf("workflow_ref.version_uuid is required when version_strategy is pinned")
		}
		workflow, err := s.repo.GetByVersionUUID(ctx, ref.VersionUUID)
		if err != nil {
			return nil, fmt.Errorf("get pinned workflow version %s: %w", ref.VersionUUID, err)
		}
		if err := validateAutomationWorkflowTarget(workflow, ref, true); err != nil {
			return nil, err
		}
		return workflow, nil
	default:
		return nil, fmt.Errorf("workflow_ref.version_strategy %q is not supported", ref.VersionStrategy)
	}
}

func validateAutomationWorkflowTarget(workflow *Workflow, ref automationaction.WorkflowRef, requireWorkflowID bool) error {
	if workflow == nil {
		return fmt.Errorf("workflow target is required")
	}
	if agentID := strings.TrimSpace(ref.AgentID); agentID != "" && workflow.AgentID != agentID {
		return fmt.Errorf("workflow %s does not belong to agent %s", workflow.ID, agentID)
	}
	if workflowID := strings.TrimSpace(ref.WorkflowID); requireWorkflowID && workflowID != "" && workflow.ID != workflowID {
		return fmt.Errorf("workflow version %s does not belong to workflow %s", ref.VersionUUID, workflowID)
	}
	return nil
}

func automationFinalizationContext(ctx context.Context) (context.Context, context.CancelFunc) {
	return context.WithTimeout(context.WithoutCancel(ctx), automationFinalizationTimeout)
}

func automationWorkflowGraphData(workflow *Workflow) (map[string]interface{}, error) {
	if workflow == nil {
		return nil, fmt.Errorf("workflow is required")
	}

	workflowMap := map[string]any{
		"graph":                  workflow.GetGraphDict(),
		"environment_variables":  normalizeVariables(workflow.GetEnvironmentVariablesDict()),
		"conversation_variables": normalizeVariables(workflow.GetConversationVariablesDict()),
	}
	graphData, err := mergeRootVariablesIntoGraph(workflowMap)
	if err != nil {
		return nil, fmt.Errorf("invalid workflow graph: %w", err)
	}
	if len(graphData) == 0 {
		return nil, fmt.Errorf("workflow graph is empty")
	}
	return graphData, nil
}

func automationWorkflowInputs(req automationaction.WorkflowRunRequest, workflow *Workflow) map[string]interface{} {
	inputs := make(map[string]interface{}, len(req.Inputs)+12)
	for key, value := range req.Inputs {
		inputs[key] = value
	}

	inputs["sys.organization_id"] = req.OrganizationID
	inputs["sys.workspace_id"] = req.WorkspaceID
	inputs["sys.tenant_id"] = req.WorkspaceID
	inputs["sys.agent_id"] = workflow.AgentID
	inputs["sys.workflow_id"] = workflow.ID
	inputs["sys.user_id"] = req.AccountID
	inputs["sys.automation_task_id"] = req.TaskID
	inputs["sys.automation_task_run_id"] = req.TaskRunID
	inputs["sys.automation_action_id"] = req.ActionID
	inputs["sys.scheduled_for"] = req.ScheduledFor.Format(time.RFC3339)
	if query, ok := inputs["query"].(string); ok && query != "" {
		inputs["sys.query"] = query
	}

	return inputs
}

type automationWorkflowNodeMeta struct {
	Index             int
	NodeID            string
	NodeType          string
	Title             string
	PredecessorNodeID *string
}

func (s *WorkflowService) persistAutomationWorkflowNodeRuntimeLogs(ctx context.Context, workspaceID, accountID string, workflow *Workflow, graphData map[string]interface{}, workflowRunID string, result *WorkflowExecutionResult) error {
	if s == nil || s.workflowNodeRuntimeLogRepo == nil || workflow == nil || result == nil || workflowRunID == "" {
		return nil
	}

	nodeMetas := automationWorkflowNodeMetas(graphData)
	graphSnapshot := optionalStringPointer(workflow.Graph)
	featuresSnapshot := optionalStringPointer(workflow.Features)

	for index, snapshot := range result.NodeExecutions {
		meta := nodeMetas[snapshot.NodeID]
		if meta.NodeID == "" {
			meta = automationWorkflowNodeMeta{
				Index:    index + 1,
				NodeID:   snapshot.NodeID,
				NodeType: string(snapshot.NodeType),
				Title:    snapshot.NodeID,
			}
		}
		if meta.NodeType == "" {
			meta.NodeType = string(snapshot.NodeType)
		}
		if meta.Title == "" {
			meta.Title = meta.NodeID
		}

		nodeLog, err := automationWorkflowNodeRuntimeLog(workspaceID, accountID, workflow, workflowRunID, graphSnapshot, featuresSnapshot, meta, snapshot)
		if err != nil {
			return err
		}
		if err := s.workflowNodeRuntimeLogRepo.Create(ctx, nodeLog); err != nil {
			return fmt.Errorf("create node runtime log for node %s: %w", snapshot.NodeID, err)
		}
	}

	return nil
}

func automationWorkflowNodeRuntimeLog(workspaceID, accountID string, workflow *Workflow, workflowRunID string, graphSnapshot, featuresSnapshot *string, meta automationWorkflowNodeMeta, snapshot graph_engine.NodeExecutionSnapshot) (*WorkflowNodeRuntimeLog, error) {
	inputs, err := jsonMapStringPointer(snapshot.Inputs)
	if err != nil {
		return nil, fmt.Errorf("marshal node inputs for node %s: %w", snapshot.NodeID, err)
	}
	processData, err := jsonMapStringPointer(snapshot.ProcessData)
	if err != nil {
		return nil, fmt.Errorf("marshal node process data for node %s: %w", snapshot.NodeID, err)
	}
	outputs, err := jsonMapStringPointer(snapshot.Outputs)
	if err != nil {
		return nil, fmt.Errorf("marshal node outputs for node %s: %w", snapshot.NodeID, err)
	}
	metadata, err := jsonMapStringPointer(snapshot.Metadata)
	if err != nil {
		return nil, fmt.Errorf("marshal node metadata for node %s: %w", snapshot.NodeID, err)
	}

	status := string(snapshot.Status)
	if status == "" {
		status = string(dto.NodeStatusSucceeded)
	}

	createdAt := snapshot.StartTime
	if createdAt.IsZero() {
		createdAt = time.Now()
	}
	var finishedAt *time.Time
	if !snapshot.EndTime.IsZero() {
		endTime := snapshot.EndTime
		finishedAt = &endTime
	}

	elapsedTime := 0.0
	if finishedAt != nil && !createdAt.IsZero() && !finishedAt.Before(createdAt) {
		elapsedTime = finishedAt.Sub(createdAt).Seconds()
	}

	var errorMessage *string
	if snapshot.Error != "" {
		errorMessage = &snapshot.Error
	}

	return &WorkflowNodeRuntimeLog{
		TenantID:          workspaceID,
		AgentID:           workflow.AgentID,
		WorkflowID:        workflow.ID,
		TriggeredFrom:     automationTriggeredFrom,
		WorkflowRunID:     &workflowRunID,
		Index:             meta.Index,
		PredecessorNodeID: meta.PredecessorNodeID,
		NodeID:            meta.NodeID,
		NodeType:          meta.NodeType,
		Title:             meta.Title,
		Inputs:            inputs,
		ProcessData:       processData,
		Outputs:           outputs,
		Graph:             graphSnapshot,
		Features:          featuresSnapshot,
		Status:            status,
		Error:             errorMessage,
		ElapsedTime:       elapsedTime,
		ExecutionMetadata: metadata,
		CreatedAt:         createdAt,
		CreatedByRole:     string(CreatedByRoleAccount),
		CreatedBy:         accountID,
		FinishedAt:        finishedAt,
	}, nil
}

func automationWorkflowNodeMetas(graphData map[string]interface{}) map[string]automationWorkflowNodeMeta {
	metas := make(map[string]automationWorkflowNodeMeta)
	nodes, _ := graphData["nodes"].([]interface{})
	for index, rawNode := range nodes {
		node, ok := rawNode.(map[string]interface{})
		if !ok {
			continue
		}
		nodeID, _ := node["id"].(string)
		if nodeID == "" {
			continue
		}
		data, _ := node["data"].(map[string]interface{})
		nodeType, _ := data["type"].(string)
		title, _ := data["title"].(string)
		if title == "" {
			title = nodeID
		}
		metas[nodeID] = automationWorkflowNodeMeta{
			Index:    index + 1,
			NodeID:   nodeID,
			NodeType: nodeType,
			Title:    title,
		}
	}

	edges, _ := graphData["edges"].([]interface{})
	for _, rawEdge := range edges {
		edge, ok := rawEdge.(map[string]interface{})
		if !ok {
			continue
		}
		source, _ := edge["source"].(string)
		target, _ := edge["target"].(string)
		if source == "" || target == "" {
			continue
		}
		meta := metas[target]
		if meta.NodeID == "" || meta.PredecessorNodeID != nil {
			continue
		}
		predecessor := source
		meta.PredecessorNodeID = &predecessor
		metas[target] = meta
	}

	return metas
}

func jsonMapStringPointer(value map[string]interface{}) (*string, error) {
	if value == nil {
		return nil, nil
	}
	payload, err := json.Marshal(value)
	if err != nil {
		return nil, err
	}
	result := string(payload)
	return &result, nil
}

func optionalStringPointer(value string) *string {
	if value == "" {
		return nil
	}
	return &value
}
