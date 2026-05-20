package workflow

import (
	"encoding/json"
	"fmt"
	"reflect"
	"regexp"
	"strings"
	"time"

	"github.com/zgiai/zgi/api/internal/modules/app/workflow/graph_engine"
	graphentities "github.com/zgiai/zgi/api/internal/modules/app/workflow/graph_engine/entities"
	workflow_shared "github.com/zgiai/zgi/api/internal/modules/app/workflow/shared"
)

const (
	workflowDefaultOutputHandle      = "source"
	workflowInternalEdgeSourceHandle = "__edge_source_handle__"
)

var (
	answerTemplateSelectorPattern      = regexp.MustCompile(`\{\{#([^.#]+)\.([^.#]+)#\}\}`)
	exactAnswerTemplateSelectorPattern = regexp.MustCompile(`^\s*\{\{#([^.#]+)\.([^.#]+)#\}\}\s*$`)
)

type streamSelectorWatchConfig struct {
	watchedSelectors             map[string]bool
	conversationMessageSelectors map[string]bool
}

func mergeWorkflowOutputsForNode(existing map[string]any, nodeType string, outputs map[string]any) map[string]any {
	if existing == nil {
		existing = make(map[string]any)
	}
	if outputs == nil {
		return existing
	}
	switch nodeType {
	case "end":
		// End node defines the workflow's output contract. Its selected variables
		// must replace any previously aggregated intermediate outputs.
		return mergeWorkflowOutputs(make(map[string]any, len(outputs)), outputs)
	case "answer", "loop":
		return mergeWorkflowOutputs(existing, outputs)
	default:
		return existing
	}
}

func mergeWorkflowOutputs(existing map[string]any, incoming map[string]any) map[string]any {
	if existing == nil {
		existing = make(map[string]any)
	}
	for key, value := range incoming {
		if strings.HasPrefix(key, "sys.") {
			continue
		}
		if key == "answer" {
			if current, ok := existing[key].(string); ok {
				if next, ok := value.(string); ok {
					existing[key] = current + next
					continue
				}
			}
		}
		existing[key] = value
	}
	return existing
}

func workflowOutputHandleForNode(edgeMap map[string]map[string][]string, nodeID, status, edgeSourceHandle string) string {
	if status == string(workflow_shared.SKIPPED) ||
		status == string(workflow_shared.FAILED) ||
		status == "stopped" ||
		status == string(workflow_shared.PAUSED) {
		return ""
	}

	handle := strings.TrimSpace(edgeSourceHandle)
	if handle == "" {
		handle = workflowDefaultOutputHandle
	}

	targetsByHandle := edgeMap[nodeID]
	if len(targetsByHandle) == 0 {
		return ""
	}
	if len(targetsByHandle[handle]) == 0 {
		return ""
	}
	return handle
}

func workflowOutputHandleFromOutputs(edgeMap map[string]map[string][]string, nodeID, status string, outputs map[string]interface{}) string {
	edgeSourceHandle := ""
	if outputs != nil {
		edgeSourceHandle, _ = outputs[workflowInternalEdgeSourceHandle].(string)
	}
	return workflowOutputHandleForNode(edgeMap, nodeID, status, edgeSourceHandle)
}

func addWorkflowStreamOutputsToVariablePool(pool *graphentities.VariablePool, nodeID string, outputs map[string]interface{}) {
	if pool == nil || strings.TrimSpace(nodeID) == "" || outputs == nil {
		return
	}
	for key, value := range outputs {
		if key == workflowInternalEdgeSourceHandle {
			continue
		}
		pool.Add([]string{nodeID, key}, workflowStreamVariablePoolValue(value))
	}
}

func workflowStreamVariablePoolValue(value interface{}) interface{} {
	if value == nil {
		return value
	}
	kind := reflect.TypeOf(value).Kind()
	if kind != reflect.Slice && kind != reflect.Array && kind != reflect.Map && kind != reflect.Struct {
		return value
	}
	data, err := json.Marshal(value)
	if err != nil {
		return value
	}
	var normalized interface{}
	if err := json.Unmarshal(data, &normalized); err != nil {
		return value
	}
	return normalized
}

func extractWorkflowAnswer(outputs map[string]interface{}) string {
	if answerText, ok := outputs["answer"].(string); ok {
		return answerText
	}

	for _, nodeOutput := range outputs {
		if nodeOutputMap, ok := nodeOutput.(map[string]interface{}); ok {
			if nodeAnswer, ok := nodeOutputMap["answer"].(string); ok && nodeAnswer != "" {
				return nodeAnswer
			}
		}
	}

	if text, ok := outputs["text"].(string); ok {
		return text
	}
	if llmOutput, ok := outputs["llm"].(map[string]interface{}); ok {
		if llmText, ok := llmOutput["text"].(string); ok {
			return llmText
		}
	}
	for _, nodeOutput := range outputs {
		if nodeOutputMap, ok := nodeOutput.(map[string]interface{}); ok {
			if nodeText, ok := nodeOutputMap["text"].(string); ok {
				return nodeText
			}
		}
	}

	return ""
}

func buildInternalNodeWorkflowStreamEvent(event *graph_engine.NodeEvent, nodeType string, nodeTitle string) *WorkflowStreamEvent {
	if event == nil {
		return nil
	}

	iterationID, iterationIndex, loopID, loopIndex := internalEventContext(event)
	startedAt := event.StartedAt
	if startedAt.IsZero() {
		startedAt = event.Timestamp
	}
	baseData := map[string]any{
		"id":                  resolveInternalExecutionID(event),
		"node_id":             event.NodeID,
		"node_type":           nodeType,
		"title":               nodeTitle,
		"index":               1,
		"predecessor_node_id": nil,
		"inputs":              FilterFrontendInputs(nodeType, event.Inputs),
		"created_at":          startedAt.Unix(),
		"created_at_ms":       startedAt.UnixMilli(),
		"iteration_id":        iterationID,
		"iteration_index":     iterationIndex,
		"loop_id":             loopID,
		"loop_index":          loopIndex,
	}

	switch event.Type {
	case "started":
		baseData["inputs_truncated"] = false
		baseData["extras"] = map[string]any{}
		baseData["agent_strategy"] = nil
		return &WorkflowStreamEvent{
			EventType: "node_started",
			Data:      baseData,
		}
	case "finished":
		baseData["process_data"] = map[string]any{}
		baseData["outputs"] = FilterFrontendOutputs(nodeType, event.Outputs)
		baseData["output_handle"] = ""
		baseData["status"] = event.Status
		if event.Error == "" {
			baseData["error"] = nil
		} else {
			baseData["error"] = event.Error
		}
		finishedAt := event.FinishedAt
		if finishedAt.IsZero() {
			finishedAt = event.Timestamp
		}
		baseData["elapsed_time"] = elapsedMillisecondsBetween(startedAt, finishedAt)
		baseData["execution_metadata"] = event.Metadata
		baseData["finished_at"] = finishedAt.Unix()
		baseData["finished_at_ms"] = finishedAt.UnixMilli()
		baseData["files"] = []interface{}{}
		return &WorkflowStreamEvent{
			EventType: "node_finished",
			Data:      baseData,
		}
	default:
		return nil
	}
}

func internalEventContext(event *graph_engine.NodeEvent) (iterationID any, iterationIndex any, loopID any, loopIndex any) {
	if event == nil || event.Metadata == nil {
		return nil, nil, nil, nil
	}
	return event.Metadata["iteration_id"], event.Metadata["iteration_index"], event.Metadata["loop_id"], event.Metadata["loop_index"]
}

func resolveInternalExecutionID(event *graph_engine.NodeEvent) string {
	if event == nil {
		return ""
	}
	if event.ExecutionID != "" {
		return event.ExecutionID
	}
	if event.Metadata != nil {
		if loopID, ok := event.Metadata["loop_id"]; ok {
			if loopIndex, ok := event.Metadata["loop_index"]; ok {
				return fmt.Sprintf("exec-%s-%v-%v", event.NodeID, loopID, loopIndex)
			}
			return fmt.Sprintf("exec-%s-%v", event.NodeID, loopID)
		}
		if iterationID, ok := event.Metadata["iteration_id"]; ok {
			if iterationIndex, ok := event.Metadata["iteration_index"]; ok {
				return fmt.Sprintf("exec-%s-%v-%v", event.NodeID, iterationID, iterationIndex)
			}
			return fmt.Sprintf("exec-%s-%v", event.NodeID, iterationID)
		}
	}
	return fmt.Sprintf("exec-%s", event.NodeID)
}

func collectStreamSelectorWatchConfig(nodeMap map[string]map[string]interface{}) streamSelectorWatchConfig {
	config := streamSelectorWatchConfig{
		watchedSelectors:             make(map[string]bool),
		conversationMessageSelectors: make(map[string]bool),
	}

	for nodeID, node := range nodeMap {
		data, ok := node["data"].(map[string]interface{})
		if !ok {
			continue
		}

		nodeType, _ := data["type"].(string)
		if nodeType != "end" && nodeType != "answer" {
			continue
		}

		if outputs, ok := data["outputs"].([]interface{}); ok {
			for _, output := range outputs {
				outputMap, ok := output.(map[string]interface{})
				if !ok {
					continue
				}
				valueSelector, ok := outputMap["value_selector"].([]interface{})
				if !ok || len(valueSelector) < 2 {
					continue
				}

				fromNode, _ := valueSelector[0].(string)
				key, _ := valueSelector[1].(string)
				if fromNode == "" || key == "" {
					continue
				}

				config.watchedSelectors[fromNode+"|"+key] = true
			}
		}

		if nodeType != "answer" {
			continue
		}

		// Answer nodes can emit processed template chunks from their own `text` selector.
		config.watchedSelectors[nodeID+"|text"] = true

		answerTemplate, _ := data["answer"].(string)
		if answerTemplate == "" {
			continue
		}

		matches := answerTemplateSelectorPattern.FindAllStringSubmatch(answerTemplate, -1)
		for _, match := range matches {
			if len(match) < 3 {
				continue
			}
			config.watchedSelectors[match[1]+"|"+match[2]] = true
		}

		match := exactAnswerTemplateSelectorPattern.FindStringSubmatch(answerTemplate)
		if len(match) >= 3 {
			config.conversationMessageSelectors[match[1]+"|"+match[2]] = true
		}
	}

	return config
}

func shouldForwardConversationMessageChunk(nodeType, selector string, config streamSelectorWatchConfig) bool {
	if !config.watchedSelectors[selector] {
		return false
	}
	if nodeType == "answer" {
		return true
	}
	return config.conversationMessageSelectors[selector]
}

func buildConversationAnswerMessageEvent(runType, workflowRunID, conversationID, nodeID, nodeType string, outputs map[string]interface{}, alreadyStreamed bool) *WorkflowStreamEvent {
	if runType != "CONVERSATION_WORKFLOW" || nodeType != "answer" || alreadyStreamed {
		return nil
	}
	answer, ok := outputs["answer"].(string)
	if !ok || answer == "" {
		return nil
	}
	return &WorkflowStreamEvent{
		EventType: "message",
		Data: map[string]interface{}{
			"id":              workflowRunID,
			"message_id":      workflowRunID,
			"conversation_id": conversationID,
			"node_id":         nodeID,
			"answer":          answer,
			"created_at":      time.Now().Unix(),
		},
	}
}

func workflowConversationIDFromSystemInputs(systemInputs map[string]interface{}) string {
	if value, ok := systemInputs["sys.conversation_id"].(string); ok {
		return value
	}
	return ""
}

func workflowMessageEventText(data map[string]interface{}) string {
	for _, key := range []string{"answer", "text", "content", "delta"} {
		if value, ok := data[key].(string); ok {
			return value
		}
	}
	if outputs, ok := data["outputs"].(map[string]interface{}); ok {
		for _, key := range []string{"answer", "text"} {
			if value, ok := outputs[key].(string); ok {
				return value
			}
		}
	}
	return ""
}

func workflowMessageEventKind(data map[string]interface{}) string {
	if data == nil {
		return ""
	}
	value, _ := data["message_kind"].(string)
	return strings.TrimSpace(value)
}

// workflowStreamNodeData extracts the "data" sub-map for a node from the node map.
func workflowStreamNodeData(nodeMap map[string]map[string]interface{}, nodeID string) map[string]interface{} {
	if node, exists := nodeMap[nodeID]; exists {
		if nodeData, ok := node["data"].(map[string]interface{}); ok {
			return nodeData
		}
	}
	return nil
}
