package workflow

import (
	"math"
	"testing"
	"time"

	approvalruntime "github.com/zgiai/zgi/api/internal/modules/app/workflow/approval"
	"github.com/zgiai/zgi/api/internal/modules/app/workflow/graph_engine"
	graphentities "github.com/zgiai/zgi/api/internal/modules/app/workflow/graph_engine/entities"
	workflowpause "github.com/zgiai/zgi/api/internal/modules/app/workflow/pause"
)

func TestMergeWorkflowOutputsForNode_AllowsLoopOutputs(t *testing.T) {
	outputs := map[string]any{}

	outputs = mergeWorkflowOutputsForNode(outputs, "loop", map[string]any{
		"answer": "loop-output",
		"count":  2,
	})

	if got := outputs["answer"]; got != "loop-output" {
		t.Fatalf("outputs[answer] = %#v, want %#v", got, "loop-output")
	}
	if got := outputs["count"]; got != 2 {
		t.Fatalf("outputs[count] = %#v, want %#v", got, 2)
	}
}

func TestMergeWorkflowOutputsForNode_IgnoresNonTerminalNonLoopNodes(t *testing.T) {
	outputs := map[string]any{
		"answer": "kept",
	}

	outputs = mergeWorkflowOutputsForNode(outputs, "llm", map[string]any{
		"answer": "ignored",
	})

	if got := outputs["answer"]; got != "kept" {
		t.Fatalf("outputs[answer] = %#v, want %#v", got, "kept")
	}
}

func TestMergeWorkflowOutputsForNode_EndNodeOverridesPreviouslyMergedOutputs(t *testing.T) {
	outputs := map[string]any{
		"answer": "leaked-answer",
		"extra":  "should-not-survive",
	}

	outputs = mergeWorkflowOutputsForNode(outputs, "end", map[string]any{
		"text": "final-text",
	})

	if len(outputs) != 1 {
		t.Fatalf("len(outputs) = %d, want 1; outputs = %#v", len(outputs), outputs)
	}
	if got := outputs["text"]; got != "final-text" {
		t.Fatalf("outputs[text] = %#v, want %#v", got, "final-text")
	}
	if _, exists := outputs["answer"]; exists {
		t.Fatalf("outputs[answer] should be removed, outputs = %#v", outputs)
	}
	if _, exists := outputs["extra"]; exists {
		t.Fatalf("outputs[extra] should be removed, outputs = %#v", outputs)
	}
}

func TestWorkflowOutputHandleForNode_ReturnsSelectedHandleWithDownstream(t *testing.T) {
	edgeMap := map[string]map[string][]string{
		"if": {
			"true":  []string{"code"},
			"false": []string{"end"},
		},
	}

	got := workflowOutputHandleForNode(edgeMap, "if", "succeeded", "true")

	if got != "true" {
		t.Fatalf("output handle = %q, want true", got)
	}
}

func TestWorkflowOutputHandleForNode_DefaultsToSourceWhenActive(t *testing.T) {
	edgeMap := map[string]map[string][]string{
		"code": {
			"source": []string{"end"},
		},
	}

	got := workflowOutputHandleForNode(edgeMap, "code", "succeeded", "")

	if got != "source" {
		t.Fatalf("output handle = %q, want source", got)
	}
}

func TestWorkflowOutputHandleForNode_ReturnsEmptyWithoutDownstream(t *testing.T) {
	edgeMap := map[string]map[string][]string{
		"end": map[string][]string{},
	}

	got := workflowOutputHandleForNode(edgeMap, "end", "succeeded", "")

	if got != "" {
		t.Fatalf("output handle = %q, want empty", got)
	}
}

func TestWorkflowOutputHandleForNode_ReturnsEmptyForSkippedNode(t *testing.T) {
	edgeMap := map[string]map[string][]string{
		"code": {
			"source": []string{"end"},
		},
	}

	got := workflowOutputHandleForNode(edgeMap, "code", "skipped", "")

	if got != "" {
		t.Fatalf("output handle = %q, want empty", got)
	}
}

func TestAddWorkflowStreamOutputsToVariablePool_SkipsInternalEdgeHandle(t *testing.T) {
	type choiceOutput struct {
		ID    string `json:"id"`
		Label string `json:"label"`
		Value string `json:"value"`
	}

	pool := graphentities.NewVariablePool()
	outputs := map[string]interface{}{
		"choices": []choiceOutput{
			{ID: "A", Label: "Option A", Value: "A"},
		},
		workflowInternalEdgeSourceHandle: "A",
	}

	addWorkflowStreamOutputsToVariablePool(pool, "qa", outputs)

	choices := pool.GetWithPath([]string{"qa", "choices"})
	if choices == nil {
		t.Fatal("choices variable is nil, want stored output")
	}
	if got, ok := choices.ToObject().([]any); !ok || len(got) != 1 {
		t.Fatalf("choices = %#v, want one choice array", choices.ToObject())
	}
	if internal := pool.GetWithPath([]string{"qa", workflowInternalEdgeSourceHandle}); internal != nil {
		t.Fatalf("internal edge handle should not be exposed, got %#v", internal.ToObject())
	}
}

func TestBuildWorkflowNodeFinishedStreamEvent_IncludesOutputHandle(t *testing.T) {
	streamEvent := buildWorkflowNodeFinishedStreamEvent(workflowNodeFinishedEventParams{
		NodeExecutionID: "exec-if",
		NodeID:          "if",
		NodeType:        "if-else",
		Outputs:         map[string]interface{}{"result": true},
		OutputHandle:    "true",
		Status:          "succeeded",
		CreatedAt:       time.Unix(1700000000, 0),
		FinishedAt:      time.Unix(1700000001, 0),
	})

	if streamEvent.EventType != "node_finished" {
		t.Fatalf("event type = %q, want node_finished", streamEvent.EventType)
	}
	if got := streamEvent.Data["output_handle"]; got != "true" {
		t.Fatalf("output_handle = %#v, want true", got)
	}
}

func TestBuildInternalNodeWorkflowStreamEvent_UsesStableExecutionIDAndLoopIndex(t *testing.T) {
	ts := time.Unix(1700000000, 123000000)
	streamEvent := buildInternalNodeWorkflowStreamEvent(&graph_engine.NodeEvent{
		Type:        "started",
		NodeID:      "loop-llm",
		ExecutionID: "exec-loop-llm-loop-1-0",
		Inputs:      map[string]any{"query": "test"},
		Metadata: map[string]any{
			"loop_id":    "loop-1",
			"loop_index": 0,
		},
		Timestamp: ts,
	}, "llm", "LLM")

	if streamEvent == nil {
		t.Fatal("buildInternalNodeWorkflowStreamEvent() = nil")
	}
	if streamEvent.EventType != "node_started" {
		t.Fatalf("streamEvent.EventType = %q, want %q", streamEvent.EventType, "node_started")
	}
	if got := streamEvent.Data["id"]; got != "exec-loop-llm-loop-1-0" {
		t.Fatalf("streamEvent.Data[id] = %#v, want %#v", got, "exec-loop-llm-loop-1-0")
	}
	if got := streamEvent.Data["loop_index"]; got != 0 {
		t.Fatalf("streamEvent.Data[loop_index] = %#v, want %#v", got, 0)
	}
}

func TestBuildInternalNodeWorkflowStreamEvent_UsesMillisecondElapsedTime(t *testing.T) {
	startedAt := time.Unix(1700000000, 100000000)
	finishedAt := time.Unix(1700000001, 350000000)
	streamEvent := buildInternalNodeWorkflowStreamEvent(&graph_engine.NodeEvent{
		Type:       "finished",
		NodeID:     "iteration-llm",
		Outputs:    map[string]any{"text": "ok"},
		Status:     "succeeded",
		Timestamp:  finishedAt,
		StartedAt:  startedAt,
		FinishedAt: finishedAt,
		Metadata: map[string]any{
			"iteration_id":    "iter-1",
			"iteration_index": 2,
		},
	}, "llm", "LLM")

	if streamEvent == nil {
		t.Fatal("buildInternalNodeWorkflowStreamEvent() = nil")
	}
	if streamEvent.EventType != "node_finished" {
		t.Fatalf("streamEvent.EventType = %q, want %q", streamEvent.EventType, "node_finished")
	}
	if got := streamEvent.Data["created_at_ms"]; got != startedAt.UnixMilli() {
		t.Fatalf("created_at_ms = %#v, want %#v", got, startedAt.UnixMilli())
	}
	if got := streamEvent.Data["finished_at_ms"]; got != finishedAt.UnixMilli() {
		t.Fatalf("finished_at_ms = %#v, want %#v", got, finishedAt.UnixMilli())
	}
	elapsed, ok := streamEvent.Data["elapsed_time"].(float64)
	if !ok {
		t.Fatalf("elapsed_time type = %T, want float64", streamEvent.Data["elapsed_time"])
	}
	if math.Abs(elapsed-1250.0) > 0.000001 {
		t.Fatalf("elapsed_time = %.9f, want 1250.0 ms", elapsed)
	}
}

func TestBuildInternalNodeWorkflowStreamEvent_FiltersInputs(t *testing.T) {
	streamEvent := buildInternalNodeWorkflowStreamEvent(&graph_engine.NodeEvent{
		Type:   "started",
		NodeID: "loop-image",
		Inputs: map[string]any{
			"prompt_variables": map[string]any{"start.subject": "cat"},
			"prompt":           "draw cat",
			"model":            map[string]any{"name": "image"},
			"generation":       map[string]any{"n": 1},
		},
		Timestamp: time.Unix(1700000000, 0),
		Metadata: map[string]any{
			"loop_id":    "loop-1",
			"loop_index": 0,
		},
	}, "image-gen", "Image Generation")

	if streamEvent == nil {
		t.Fatal("buildInternalNodeWorkflowStreamEvent() = nil")
	}
	inputs, ok := streamEvent.Data["inputs"].(map[string]interface{})
	if !ok {
		t.Fatalf("inputs type = %T, want map", streamEvent.Data["inputs"])
	}
	if inputs["prompt"] != "draw cat" {
		t.Fatalf("prompt should be kept in internal node inputs: %#v", inputs)
	}
	if _, exists := inputs["model"]; exists {
		t.Fatalf("model should be removed from internal node inputs: %#v", inputs)
	}
	if _, exists := inputs["generation"]; exists {
		t.Fatalf("generation should be removed from internal node inputs: %#v", inputs)
	}
	promptVariables, ok := inputs["prompt_variables"].(map[string]any)
	if !ok || promptVariables["start.subject"] != "cat" {
		t.Fatalf("prompt_variables = %#v, want selected variable value", inputs["prompt_variables"])
	}
}

func TestSanitizeWorkflowEventData_RemovesInternalApprovalKeys(t *testing.T) {
	input := map[string]interface{}{
		"node_id": "approval-1",
		"outputs": map[string]interface{}{
			"comment":                "ok",
			"__action_id":            "approve",
			"__rendered_content":     "hidden",
			"__edge_source_handle__": "approve",
		},
		"inputs": []interface{}{
			map[string]interface{}{
				"visible":                      true,
				"__approval_form":              map[string]interface{}{"token": "secret"},
				"sys.workflow_resume_state":    "hidden",
				"sys.workflow_resume_pause_id": "hidden",
			},
		},
	}

	got := sanitizeWorkflowEventData(input)
	outputs, ok := got["outputs"].(map[string]interface{})
	if !ok {
		t.Fatalf("outputs type = %T, want map", got["outputs"])
	}
	if outputs["comment"] != "ok" {
		t.Fatalf("outputs[comment] = %#v, want ok", outputs["comment"])
	}
	for _, key := range []string{"__action_id", "__rendered_content", "__edge_source_handle__"} {
		if _, exists := outputs[key]; exists {
			t.Fatalf("internal output key %s should be removed: %#v", key, outputs)
		}
	}

	inputs, ok := got["inputs"].([]interface{})
	if !ok || len(inputs) != 1 {
		t.Fatalf("inputs = %#v, want one item", got["inputs"])
	}
	nested, ok := inputs[0].(map[string]interface{})
	if !ok {
		t.Fatalf("nested input type = %T, want map", inputs[0])
	}
	if nested["visible"] != true {
		t.Fatalf("nested[visible] = %#v, want true", nested["visible"])
	}
	for _, key := range []string{"__approval_form", "sys.workflow_resume_state", "sys.workflow_resume_pause_id"} {
		if _, exists := nested[key]; exists {
			t.Fatalf("internal nested key %s should be removed: %#v", key, nested)
		}
	}
}

func TestSanitizeWorkflowEventData_DoesNotInferSecondsFromElapsedTime(t *testing.T) {
	got := sanitizeWorkflowEventData(map[string]interface{}{
		"elapsed_time": 0.068759625,
		"created_at":   int64(1700000000),
		"finished_at":  int64(1700000000),
	})

	elapsed, ok := got["elapsed_time"].(float64)
	if !ok {
		t.Fatalf("elapsed_time type = %T, want float64", got["elapsed_time"])
	}
	if math.Abs(elapsed-0.068759625) > 0.000001 {
		t.Fatalf("elapsed_time = %.9f, want %.9f", elapsed, 0.068759625)
	}
}

func TestSanitizeWorkflowEventData_KeepsElapsedMilliseconds(t *testing.T) {
	got := sanitizeWorkflowEventData(map[string]interface{}{
		"elapsed_time": 68.7,
		"created_at":   int64(1700000000),
		"finished_at":  int64(1700000000),
	})

	elapsed, ok := got["elapsed_time"].(float64)
	if !ok {
		t.Fatalf("elapsed_time type = %T, want float64", got["elapsed_time"])
	}
	if math.Abs(elapsed-68.7) > 0.000001 {
		t.Fatalf("elapsed_time = %.9f, want %.9f", elapsed, 68.7)
	}
}

func TestSanitizeWorkflowEventData_KeepsSubMillisecondElapsedMilliseconds(t *testing.T) {
	got := sanitizeWorkflowEventData(map[string]interface{}{
		"elapsed_time": 0.7,
		"created_at":   int64(1700000000),
		"finished_at":  int64(1700000000),
	})

	elapsed, ok := got["elapsed_time"].(float64)
	if !ok {
		t.Fatalf("elapsed_time type = %T, want float64", got["elapsed_time"])
	}
	if math.Abs(elapsed-0.7) > 0.000001 {
		t.Fatalf("elapsed_time = %.9f, want %.9f", elapsed, 0.7)
	}
}

func TestParseWorkflowEventsAfter(t *testing.T) {
	value, hasAfter, err := parseWorkflowEventsAfter("12")
	if err != nil {
		t.Fatalf("parse valid after returned error: %v", err)
	}
	if !hasAfter || value != 12 {
		t.Fatalf("parse valid after = (%d, %v), want (12, true)", value, hasAfter)
	}

	value, hasAfter, err = parseWorkflowEventsAfter("")
	if err != nil {
		t.Fatalf("parse empty after returned error: %v", err)
	}
	if hasAfter || value != 0 {
		t.Fatalf("parse empty after = (%d, %v), want (0, false)", value, hasAfter)
	}

	if _, _, err := parseWorkflowEventsAfter("-1"); err == nil {
		t.Fatal("parse negative after should return error")
	}
	if _, _, err := parseWorkflowEventsAfter("abc"); err == nil {
		t.Fatal("parse non-numeric after should return error")
	}
}

func TestBuildApprovalCompletionEvent_ResultFilled(t *testing.T) {
	eventType, data := buildApprovalCompletionEvent(
		"run-1",
		"approval-1",
		"Approval",
		map[string]interface{}{
			"comment":                   "ok",
			"approval_action_id":        "approve",
			"approval_action_label":     "Approve",
			"approval_rendered_content": "Rendered",
		},
		map[string]interface{}{"form_id": "form-1"},
	)

	if eventType != workflowpause.EventApprovalResultFilled {
		t.Fatalf("event type = %s, want %s", eventType, workflowpause.EventApprovalResultFilled)
	}
	if data["form_id"] != "form-1" || data["workflow_run_id"] != "run-1" {
		t.Fatalf("data ids = %#v", data)
	}
	if data["action_id"] != "approve" || data["action_label"] != "Approve" {
		t.Fatalf("action data = %#v", data)
	}
	inputs, ok := data["inputs"].(map[string]interface{})
	if !ok {
		t.Fatalf("inputs type = %T, want map", data["inputs"])
	}
	if inputs["comment"] != "ok" {
		t.Fatalf("inputs[comment] = %#v, want ok", inputs["comment"])
	}
	if _, exists := inputs["approval_action_id"]; exists {
		t.Fatalf("approval_action_id should not be copied into inputs: %#v", inputs)
	}
}

func TestBuildApprovalCompletionEvent_Expired(t *testing.T) {
	eventType, data := buildApprovalCompletionEvent(
		"run-1",
		"approval-1",
		"Approval",
		map[string]interface{}{
			"approval_action_id": approvalruntime.ActionExpired,
		},
		map[string]interface{}{
			"form_id":    "form-1",
			"expires_at": int64(1700000000),
		},
	)

	if eventType != workflowpause.EventApprovalExpired {
		t.Fatalf("event type = %s, want %s", eventType, workflowpause.EventApprovalExpired)
	}
	if data["form_id"] != "form-1" || data["expires_at"] != int64(1700000000) {
		t.Fatalf("expired event data = %#v", data)
	}
}

func TestCollectStreamSelectorWatchConfig_MixedAnswerTemplateSuppressesUpstreamConversationMessage(t *testing.T) {
	nodeMap := map[string]map[string]interface{}{
		"answer-1": {
			"data": map[string]interface{}{
				"type":   "answer",
				"answer": "prefix {{#llm.text#}} suffix",
			},
		},
	}

	config := collectStreamSelectorWatchConfig(nodeMap)

	if !config.watchedSelectors["llm|text"] {
		t.Fatalf("watchedSelectors[llm|text] = false, want true")
	}
	if !config.watchedSelectors["answer-1|text"] {
		t.Fatalf("watchedSelectors[answer-1|text] = false, want true")
	}
	if config.conversationMessageSelectors["llm|text"] {
		t.Fatalf("conversationMessageSelectors[llm|text] = true, want false")
	}
	if shouldForwardConversationMessageChunk("llm", "llm|text", config) {
		t.Fatalf("shouldForwardConversationMessageChunk(llm, llm|text) = true, want false")
	}
	if !shouldForwardConversationMessageChunk("answer", "answer-1|text", config) {
		t.Fatalf("shouldForwardConversationMessageChunk(answer, answer-1|text) = false, want true")
	}
}

func TestCollectStreamSelectorWatchConfig_DirectAnswerTemplateAllowsUpstreamConversationMessage(t *testing.T) {
	nodeMap := map[string]map[string]interface{}{
		"answer-1": {
			"data": map[string]interface{}{
				"type":   "answer",
				"answer": "  {{#llm.text#}}  ",
			},
		},
	}

	config := collectStreamSelectorWatchConfig(nodeMap)

	if !config.conversationMessageSelectors["llm|text"] {
		t.Fatalf("conversationMessageSelectors[llm|text] = false, want true")
	}
	if !shouldForwardConversationMessageChunk("llm", "llm|text", config) {
		t.Fatalf("shouldForwardConversationMessageChunk(llm, llm|text) = false, want true")
	}
}

func TestShouldForwardConversationMessageChunk_PrefersAnswerChunksForMixedTemplate(t *testing.T) {
	nodeMap := map[string]map[string]interface{}{
		"answer-1": {
			"data": map[string]interface{}{
				"type":   "answer",
				"answer": "prefix {{#llm.text#}} suffix",
			},
		},
	}

	config := collectStreamSelectorWatchConfig(nodeMap)

	if shouldForwardConversationMessageChunk("llm", "llm|text", config) {
		t.Fatal("llm chunk should not be forwarded as conversation message for mixed answer template")
	}
	if !shouldForwardConversationMessageChunk("answer", "answer-1|text", config) {
		t.Fatal("answer chunk should be forwarded as conversation message for mixed answer template")
	}
}

func TestBuildConversationAnswerMessageEvent_UsesAnswerNodeOutput(t *testing.T) {
	event := buildConversationAnswerMessageEvent(
		"CONVERSATION_WORKFLOW",
		"run-1",
		"conversation-1",
		"answer-1",
		"answer",
		map[string]interface{}{"answer": "custom reply"},
		false,
	)

	if event == nil {
		t.Fatal("expected message event")
	}
	if event.EventType != "message" {
		t.Fatalf("event type = %s, want message", event.EventType)
	}
	if got := event.Data["answer"]; got != "custom reply" {
		t.Fatalf("answer = %#v, want custom reply", got)
	}
	if got := event.Data["conversation_id"]; got != "conversation-1" {
		t.Fatalf("conversation_id = %#v, want conversation-1", got)
	}
}

func TestBuildConversationAnswerMessageEvent_SkipsAlreadyStreamedAnswerNode(t *testing.T) {
	event := buildConversationAnswerMessageEvent(
		"CONVERSATION_WORKFLOW",
		"run-1",
		"conversation-1",
		"answer-1",
		"answer",
		map[string]interface{}{"answer": "custom reply"},
		true,
	)

	if event != nil {
		t.Fatalf("event = %#v, want nil", event)
	}
}
