package workflow

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/zgiai/zgi/api/internal/dto"
	"github.com/zgiai/zgi/api/internal/modules/app/workflow/graph_engine"
	automationaction "github.com/zgiai/zgi/api/internal/modules/automation/service/action"
	"github.com/zgiai/zgi/api/pkg/logger"
	"go.uber.org/zap"
)

const automationTriggeredFrom = string(InvokeFromAutomation)

const automationFinalizationTimeout = 30 * time.Second

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

	startedAt := time.Now()
	executionResult, execErr := s.executor.ExecuteSimpleWorkflowWithRunID(runCtx, workflowRunLogID, graphData, inputs)
	elapsedTime := time.Since(startedAt).Seconds()
	finalizeCtx, cancelFinalize := automationFinalizationContext(runCtx)
	defer cancelFinalize()

	outputs := map[string]interface{}{}
	totalSteps := 0
	if executionResult != nil {
		outputs = executionResult.NodeResults
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
	}

	if err := s.UpdateWorkflowRunLogStatus(finalizeCtx, workflowRunLogID, status, outputs, elapsedTime, 0, totalSteps, errorMessage); err != nil {
		return nil, fmt.Errorf("update automation workflow run log: %w", err)
	}
	if execErr != nil {
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
		if err := validateAutomationWorkflowTarget(workflow, ref); err != nil {
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
		if err := validateAutomationWorkflowTarget(workflow, ref); err != nil {
			return nil, err
		}
		return workflow, nil
	default:
		return nil, fmt.Errorf("workflow_ref.version_strategy %q is not supported", ref.VersionStrategy)
	}
}

func validateAutomationWorkflowTarget(workflow *Workflow, ref automationaction.WorkflowRef) error {
	if workflow == nil {
		return fmt.Errorf("workflow target is required")
	}
	if agentID := strings.TrimSpace(ref.AgentID); agentID != "" && workflow.AgentID != agentID {
		return fmt.Errorf("workflow %s does not belong to agent %s", workflow.ID, agentID)
	}
	if workflowID := strings.TrimSpace(ref.WorkflowID); workflowID != "" && workflow.ID != workflowID {
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
