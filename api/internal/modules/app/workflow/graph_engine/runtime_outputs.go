package graph_engine

import (
	"fmt"
	"strings"

	"github.com/zgiai/zgi/api/internal/modules/app/workflow/shared"
)

func (e *WorkflowEngine) updateRuntimeOutputsForNode(nodeType shared.NodeType, outputs map[string]any) {
	if e == nil || e.runtimeState == nil || outputs == nil {
		return
	}
	if nodeType != shared.Answer && nodeType != shared.End {
		return
	}
	e.runtimeState.UpdateOutputs(func(current map[string]any) map[string]any {
		return mergeRuntimeOutputs(current, outputs)
	})
}

func mergeRuntimeOutputs(base map[string]any, incoming map[string]any) map[string]any {
	if base == nil {
		base = make(map[string]any)
	}
	for key, value := range incoming {
		if strings.HasPrefix(key, "sys.") {
			continue
		}
		if key == "answer" {
			if current, ok := base[key].(string); ok {
				if next, ok := value.(string); ok {
					base[key] = current + next
					continue
				}
			}
		}
		base[key] = value
	}
	return base
}

func buildInternalExecutionID(nodeID string, metadata map[string]interface{}) string {
	if metadata == nil {
		return fmt.Sprintf("exec-%s", nodeID)
	}

	if loopID, ok := metadata[string(shared.LoopId)]; ok {
		if index, ok := metadata[string(shared.LoopIndex)]; ok {
			return fmt.Sprintf("exec-%s-%v-%v", nodeID, loopID, index)
		}
		return fmt.Sprintf("exec-%s-%v", nodeID, loopID)
	}

	if iterationID, ok := metadata[string(shared.ITERATION_ID)]; ok {
		if index, ok := metadata[string(shared.IterationIndex)]; ok {
			return fmt.Sprintf("exec-%s-%v-%v", nodeID, iterationID, index)
		}
		return fmt.Sprintf("exec-%s-%v", nodeID, iterationID)
	}

	return fmt.Sprintf("exec-%s", nodeID)
}
