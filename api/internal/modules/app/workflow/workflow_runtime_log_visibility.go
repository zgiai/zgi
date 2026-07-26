package workflow

import (
	"fmt"
	"strings"

	"github.com/zgiai/zgi/api/internal/modules/app/workflow/graph_engine"
	workflowshared "github.com/zgiai/zgi/api/internal/modules/app/workflow/shared"
)

// workflowNodeRuntimeStatusIsVisible reports whether a node state represents an
// execution attempt that belongs in the runtime log. The graph engine keeps a
// snapshot for every node in the graph, including nodes that were never reached
// because execution paused or selected another branch. Those pending/skipped
// graph states are scheduling facts, not execution records.
func workflowNodeRuntimeStatusIsVisible(status string) bool {
	switch strings.ToLower(strings.TrimSpace(status)) {
	case string(workflowshared.PENDING), string(workflowshared.SKIPPED):
		return false
	default:
		return true
	}
}

// workflowNodeRuntimeLogStepCount follows the same interaction grouping used by
// the runtime-log UI. Approval and question nodes are persisted once when they
// pause and again when they resume; those rows describe one logical step, while
// repeated container child executions remain separate steps.
func workflowNodeRuntimeLogStepCount(logs []WorkflowNodeRuntimeLog) int {
	count := 0
	interactionNodes := make(map[string]struct{})
	for _, nodeLog := range logs {
		if !workflowNodeRuntimeStatusIsVisible(string(nodeLog.Status)) {
			continue
		}

		nodeType := strings.ToLower(strings.TrimSpace(nodeLog.NodeType))
		if nodeLog.NodeID != "" && (nodeType == string(workflowshared.Approval) || nodeType == string(workflowshared.QuestionAnswer)) {
			if _, exists := interactionNodes[nodeLog.NodeID]; exists {
				continue
			}
			interactionNodes[nodeLog.NodeID] = struct{}{}
		}
		count++
	}
	return count
}

func workflowExecutionResultStepCount(result *WorkflowExecutionResult) int {
	if result == nil {
		return 0
	}

	if len(result.NodeExecutions) > 0 {
		count := 0
		for _, snapshot := range result.NodeExecutions {
			if workflowNodeRuntimeStatusIsVisible(string(snapshot.Status)) {
				count++
			}
		}
		return count
	}

	count := 0
	for _, rawResult := range result.NodeResults {
		status := workflowNodeResultStatus(rawResult)
		if status == "" || workflowNodeRuntimeStatusIsVisible(status) {
			count++
		}
	}
	return count
}

func workflowNodeResultStatus(value interface{}) string {
	switch typed := value.(type) {
	case map[string]interface{}:
		status, exists := typed["status"]
		if !exists || status == nil {
			return ""
		}
		return strings.TrimSpace(fmt.Sprint(status))
	case graph_engine.NodeExecutionSnapshot:
		return string(typed.Status)
	case *graph_engine.NodeExecutionSnapshot:
		if typed != nil {
			return string(typed.Status)
		}
	}
	return ""
}
