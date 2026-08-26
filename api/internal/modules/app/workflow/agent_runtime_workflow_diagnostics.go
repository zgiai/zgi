package workflow

import (
	"context"
	"strings"

	"github.com/google/uuid"
	runtimemodel "github.com/zgiai/zgi/api/internal/capabilities/chatruntime/model"
	runtimeservice "github.com/zgiai/zgi/api/internal/capabilities/chatruntime/service"
)

type agentRuntimeWorkflowRunLogRepository interface {
	GetByID(ctx context.Context, id string) (*WorkflowRunLog, error)
}

type agentRuntimeWorkflowNodeLogRepository interface {
	GetByWorkflowRunID(ctx context.Context, workflowRunID string) ([]WorkflowNodeRuntimeLog, error)
}

func (h *AgentRuntimeLogsHandler) withAgentRuntimeWorkflowDiagnostics(
	ctx context.Context,
	message *runtimemodel.Message,
	scope runtimeservice.Scope,
	agentID uuid.UUID,
) *runtimemodel.Message {
	if h == nil || message == nil || h.workflowRunLogs == nil || scope.WorkspaceID == nil {
		return message
	}
	metadata := cloneAgentRuntimeDiagnosticMap(message.Metadata)
	runs := runtimeSkillInvocations(metadata["workflow_runs"])
	if continuationRunID := agentRuntimeContinuationWorkflowRunID(metadata); continuationRunID != "" && !agentRuntimeRunsContainID(runs, continuationRunID) {
		runs = append(runs, map[string]interface{}{"workflow_run_id": continuationRunID})
	}
	if len(runs) == 0 {
		return message
	}

	enriched := make([]interface{}, 0, len(runs))
	for _, run := range runs {
		runCopy := cloneAgentRuntimeDiagnosticMap(run)
		runID := strings.TrimSpace(runtimeString(runCopy["workflow_run_id"]))
		if runID == "" {
			enriched = append(enriched, runCopy)
			continue
		}
		runLog, err := h.workflowRunLogs.GetByID(ctx, runID)
		if err != nil || runLog == nil || runLog.TenantID != scope.WorkspaceID.String() || runLog.AgentID != agentID.String() {
			enriched = append(enriched, runCopy)
			continue
		}
		enriched = append(enriched, h.enrichAgentRuntimeWorkflowRun(ctx, runCopy, runLog))
	}
	metadata["workflow_runs"] = enriched
	messageCopy := *message
	messageCopy.Metadata = metadata
	return &messageCopy
}

func (h *AgentRuntimeLogsHandler) enrichAgentRuntimeWorkflowRun(
	ctx context.Context,
	run map[string]interface{},
	runLog *WorkflowRunLog,
) map[string]interface{} {
	run["workflow_run_id"] = runLog.ID
	run["workflow_id"] = runLog.WorkflowID
	run["agent_id"] = runLog.AgentID
	run["status"] = string(runLog.Status)
	run["version"] = runLog.Version
	run["inputs"] = runLog.GetInputsDict()
	run["outputs"] = runLog.GetOutputsDict()
	run["elapsed_time"] = runLog.ElapsedTime
	run["created_at"] = runLog.CreatedAt.Unix()
	run["created_at_ms"] = runLog.CreatedAt.UnixMilli()
	if runLog.Error != nil {
		run["error"] = *runLog.Error
	}
	if h.workflowNodeRuntimeLogs == nil {
		return run
	}
	nodeLogs, err := h.workflowNodeRuntimeLogs.GetByWorkflowRunID(ctx, runLog.ID)
	if err != nil {
		return run
	}
	nodes := make([]interface{}, 0, len(nodeLogs))
	for _, nodeLog := range nodeLogs {
		if nodeLog.TenantID != runLog.TenantID || nodeLog.AgentID != runLog.AgentID {
			continue
		}
		nodes = append(nodes, agentRuntimeWorkflowNodeDiagnostic(nodeLog))
	}
	if len(nodes) > 0 {
		run["nodes"] = nodes
	}
	return run
}

func agentRuntimeWorkflowNodeDiagnostic(node WorkflowNodeRuntimeLog) map[string]interface{} {
	diagnostic := map[string]interface{}{
		"id":            node.ID,
		"index":         node.Index,
		"node_id":       node.NodeID,
		"node_type":     node.NodeType,
		"title":         node.Title,
		"status":        node.Status,
		"elapsed_time":  node.ElapsedTime,
		"created_at":    node.CreatedAt.Unix(),
		"created_at_ms": node.CreatedAt.UnixMilli(),
	}
	if inputs, err := node.GetInputsDict(); err == nil {
		diagnostic["inputs"] = inputs
	}
	if processData, err := node.GetProcessDataDict(); err == nil {
		diagnostic["process_data"] = processData
	}
	if outputs, err := node.GetOutputsDict(); err == nil {
		diagnostic["outputs"] = outputs
	}
	if node.Error != nil {
		diagnostic["error"] = *node.Error
	}
	if node.ErrorType != nil {
		diagnostic["error_type"] = *node.ErrorType
	}
	if node.ErrorStack != nil {
		diagnostic["error_stack"] = *node.ErrorStack
	}
	if node.FinishedAt != nil {
		diagnostic["finished_at"] = node.FinishedAt.Unix()
	}
	return diagnostic
}

func agentRuntimeContinuationWorkflowRunID(metadata map[string]interface{}) string {
	continuation := runtimeMap(metadata["agent_workflow_continuation"])
	return strings.TrimSpace(runtimeString(continuation["workflow_run_id"]))
}

func agentRuntimeRunsContainID(runs []map[string]interface{}, runID string) bool {
	for _, run := range runs {
		if strings.TrimSpace(runtimeString(run["workflow_run_id"])) == runID {
			return true
		}
	}
	return false
}

func cloneAgentRuntimeDiagnosticMap(source map[string]interface{}) map[string]interface{} {
	if source == nil {
		return map[string]interface{}{}
	}
	cloned, _ := cloneAgentRuntimeDiagnosticValue(source).(map[string]interface{})
	return cloned
}

func cloneAgentRuntimeDiagnosticValue(value interface{}) interface{} {
	switch typed := value.(type) {
	case map[string]interface{}:
		cloned := make(map[string]interface{}, len(typed))
		for key, item := range typed {
			cloned[key] = cloneAgentRuntimeDiagnosticValue(item)
		}
		return cloned
	case []interface{}:
		cloned := make([]interface{}, 0, len(typed))
		for _, item := range typed {
			cloned = append(cloned, cloneAgentRuntimeDiagnosticValue(item))
		}
		return cloned
	case []map[string]interface{}:
		cloned := make([]map[string]interface{}, 0, len(typed))
		for _, item := range typed {
			cloned = append(cloned, cloneAgentRuntimeDiagnosticMap(item))
		}
		return cloned
	default:
		return value
	}
}
