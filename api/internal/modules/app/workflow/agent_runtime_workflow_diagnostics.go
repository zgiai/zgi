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
	parentAgentID uuid.UUID,
) *runtimemodel.Message {
	if h == nil || message == nil || h.workflowRunLogs == nil || scope.WorkspaceID == nil || parentAgentID == uuid.Nil {
		return message
	}
	metadata := cloneAgentRuntimeDiagnosticMap(message.Metadata)
	runs := runtimeSkillInvocations(metadata["workflow_runs"])
	if continuationRun := agentRuntimeContinuationWorkflowRun(metadata); continuationRun != nil {
		continuationRunID := strings.TrimSpace(runtimeString(continuationRun["workflow_run_id"]))
		if !agentRuntimeRunsContainID(runs, continuationRunID) {
			runs = append(runs, continuationRun)
		}
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
		if err != nil || !agentRuntimeWorkflowRunBelongsToMessage(runCopy, runLog, message, scope.WorkspaceID.String()) {
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

func agentRuntimeWorkflowRunBelongsToMessage(run map[string]interface{}, runLog *WorkflowRunLog, message *runtimemodel.Message, workspaceID string) bool {
	if runLog == nil || message == nil || strings.TrimSpace(runLog.TenantID) != strings.TrimSpace(workspaceID) {
		return false
	}
	if strings.TrimSpace(getStringValue(runLog.ParentMessageID)) != message.ID.String() {
		return false
	}
	if parentConversationID := strings.TrimSpace(getStringValue(runLog.ParentConversationID)); parentConversationID != "" && parentConversationID != message.ConversationID.String() {
		return false
	}

	identities := []struct {
		metadataKey string
		storedValue string
	}{
		{metadataKey: "invocation_id", storedValue: getStringValue(runLog.ParentInvocationID)},
		{metadataKey: "binding_id", storedValue: getStringValue(runLog.InvocationBindingID)},
		{metadataKey: "workflow_id", storedValue: runLog.WorkflowID},
		{metadataKey: "agent_id", storedValue: runLog.AgentID},
	}
	for _, identity := range identities {
		expected := strings.TrimSpace(runtimeString(run[identity.metadataKey]))
		if expected != "" && expected != strings.TrimSpace(identity.storedValue) {
			return false
		}
	}
	return true
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

func agentRuntimeContinuationWorkflowRun(metadata map[string]interface{}) map[string]interface{} {
	continuation := runtimeMap(metadata["agent_workflow_continuation"])
	runID := strings.TrimSpace(runtimeString(continuation["workflow_run_id"]))
	if runID == "" {
		return nil
	}
	run := map[string]interface{}{"workflow_run_id": runID}
	for _, key := range []string{"workflow_id", "agent_id", "invocation_id", "binding_id"} {
		if value := strings.TrimSpace(runtimeString(continuation[key])); value != "" {
			run[key] = value
		}
	}
	return run
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
