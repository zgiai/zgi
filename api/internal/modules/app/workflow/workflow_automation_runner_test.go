package workflow

import (
	"context"
	"errors"
	"testing"
	"time"

	workflowdto "github.com/zgiai/zgi/api/internal/dto"
	"github.com/zgiai/zgi/api/internal/modules/app/workflow/graph_engine"
	"github.com/zgiai/zgi/api/internal/modules/app/workflow/graph_engine/entities"
	workflowshared "github.com/zgiai/zgi/api/internal/modules/app/workflow/shared"
	automationaction "github.com/zgiai/zgi/api/internal/modules/automation/service/action"
)

func TestAutomationWorkflowRunOutcomePropagatesExecutionFailure(t *testing.T) {
	providerErr := errors.New("model route unavailable")
	tests := []struct {
		name       string
		result     *WorkflowExecutionResult
		execErr    error
		wantStatus string
		wantErr    error
	}{
		{
			name:       "executor error",
			result:     &WorkflowExecutionResult{Status: "succeeded"},
			execErr:    providerErr,
			wantStatus: string(workflowdto.WorkflowRunStatusFailed),
			wantErr:    providerErr,
		},
		{
			name:       "failed result error",
			result:     &WorkflowExecutionResult{Status: "failed", Error: providerErr},
			wantStatus: string(workflowdto.WorkflowRunStatusFailed),
			wantErr:    providerErr,
		},
		{
			name:       "failed result without error",
			result:     &WorkflowExecutionResult{Status: "failed"},
			wantStatus: string(workflowdto.WorkflowRunStatusFailed),
		},
		{
			name:       "paused result",
			result:     &WorkflowExecutionResult{Status: "paused"},
			wantStatus: string(workflowdto.WorkflowRunStatusPaused),
		},
		{
			name:       "stopped result",
			result:     &WorkflowExecutionResult{Status: "stopped"},
			wantStatus: string(workflowdto.WorkflowRunStatusStopped),
		},
		{
			name:       "stopped result cancellation",
			result:     &WorkflowExecutionResult{Status: "stopped", Error: context.Canceled},
			wantStatus: string(workflowdto.WorkflowRunStatusStopped),
		},
		{
			name:       "stopped executor cancellation",
			result:     &WorkflowExecutionResult{Status: "stopped", Error: context.Canceled},
			execErr:    context.Canceled,
			wantStatus: string(workflowdto.WorkflowRunStatusStopped),
		},
		{
			name:       "stopped result with non-cancellation executor error",
			result:     &WorkflowExecutionResult{Status: "stopped"},
			execErr:    providerErr,
			wantStatus: string(workflowdto.WorkflowRunStatusFailed),
			wantErr:    providerErr,
		},
		{
			name:       "successful result",
			result:     &WorkflowExecutionResult{Status: "succeeded"},
			wantStatus: string(workflowdto.WorkflowRunStatusSucceeded),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			status, err := automationWorkflowRunOutcome(tt.result, tt.execErr)
			if status != tt.wantStatus {
				t.Fatalf("status = %q, want %q", status, tt.wantStatus)
			}
			if tt.wantErr != nil && !errors.Is(err, tt.wantErr) {
				t.Fatalf("error = %v, want error wrapping %v", err, tt.wantErr)
			}
			if tt.wantStatus == string(workflowdto.WorkflowRunStatusFailed) && err == nil {
				t.Fatal("error = nil, want workflow failure")
			}
			if tt.wantStatus != string(workflowdto.WorkflowRunStatusFailed) && err != nil {
				t.Fatalf("error = %v, want nil", err)
			}
		})
	}
}

func TestEmitAutomationWorkflowDelegateAnswerUsesParentMessage(t *testing.T) {
	req := automationWorkflowEventTestRequest()
	req.Invocation = &automationaction.WorkflowInvocationContext{
		Mode:                 automationaction.WorkflowInvocationModeAgentDelegate,
		ParentConversationID: "conversation-1",
		ParentMessageID:      "message-1",
	}
	var events []automationaction.WorkflowRunEvent

	emitAutomationWorkflowDelegateAnswer(func(event automationaction.WorkflowRunEvent) {
		events = append(events, event)
	}, req, automationWorkflowEventTestWorkflow(), "run-1", map[string]interface{}{
		"answer": "workflow answer\n",
	})

	if len(events) != 1 {
		t.Fatalf("events = %#v, want one delegate message", events)
	}
	event := events[0]
	if event.Type != "message" {
		t.Fatalf("event type = %q, want message", event.Type)
	}
	if event.Payload["answer"] != "workflow answer\n" {
		t.Fatalf("answer = %#v, want original workflow output", event.Payload["answer"])
	}
	if event.Payload["conversation_id"] != "conversation-1" || event.Payload["message_id"] != "message-1" {
		t.Fatalf("message parent = %#v, want conversation-1/message-1", event.Payload)
	}
}

func TestEmitAutomationWorkflowDelegateAnswerSkipsTaskAndEmptyOutput(t *testing.T) {
	tests := []struct {
		name    string
		mode    string
		outputs map[string]interface{}
	}{
		{name: "task workflow", mode: automationaction.WorkflowInvocationModeAgentTaskTool, outputs: map[string]interface{}{"answer": "hidden"}},
		{name: "empty delegate output", mode: automationaction.WorkflowInvocationModeAgentDelegate, outputs: map[string]interface{}{"answer": "  "}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := automationWorkflowEventTestRequest()
			req.Invocation = &automationaction.WorkflowInvocationContext{Mode: tt.mode}
			emitted := false
			emitAutomationWorkflowDelegateAnswer(func(automationaction.WorkflowRunEvent) {
				emitted = true
			}, req, automationWorkflowEventTestWorkflow(), "run-1", tt.outputs)
			if emitted {
				t.Fatal("delegate answer emitted for non-displayable output")
			}
		})
	}
}

func TestAutomationWorkflowDelegateAnswerBridgeEmitsChunkSynchronously(t *testing.T) {
	req := automationWorkflowEventTestRequest()
	req.Invocation = &automationaction.WorkflowInvocationContext{
		Mode:                 automationaction.WorkflowInvocationModeAgentDelegate,
		ParentConversationID: "conversation-1",
		ParentMessageID:      "message-1",
	}
	nodes := []interface{}{
		map[string]interface{}{"id": "start", "data": map[string]interface{}{"type": "start"}},
		map[string]interface{}{"id": "llm", "data": map[string]interface{}{"type": "llm"}},
		map[string]interface{}{"id": "answer", "data": map[string]interface{}{"type": "answer", "answer": "{{#llm.text#}}"}},
	}
	nodeMap := map[string]map[string]interface{}{}
	for _, rawNode := range nodes {
		node := rawNode.(map[string]interface{})
		nodeMap[node["id"].(string)] = node
	}
	streamGraph := &workflowStreamGraph{
		GraphData: map[string]interface{}{
			"nodes": nodes,
			"edges": []interface{}{
				map[string]interface{}{"source": "start", "sourceHandle": workflowDefaultOutputHandle, "target": "llm"},
				map[string]interface{}{"source": "llm", "sourceHandle": workflowDefaultOutputHandle, "target": "answer"},
			},
		},
		NodeMap:        nodeMap,
		RuntimeNodeMap: nodeMap,
		EdgeMap: map[string]map[string][]string{
			"start": {workflowDefaultOutputHandle: {"llm"}},
			"llm":   {workflowDefaultOutputHandle: {"answer"}},
		},
		RuntimeEdgeMap: map[string]map[string][]string{
			"start": {workflowDefaultOutputHandle: {"llm"}},
			"llm":   {workflowDefaultOutputHandle: {"answer"}},
		},
		StartNodeID: "start",
	}
	streamGraph.WatchConfig = collectStreamSelectorWatchConfig(nodeMap)

	var events []automationaction.WorkflowRunEvent
	bridge := &automationWorkflowDelegateAnswerBridge{
		req: req, workflow: automationWorkflowEventTestWorkflow(), workflowRunID: "run-1",
		eventSink:   func(event automationaction.WorkflowRunEvent) { events = append(events, event) },
		streamGraph: streamGraph,
	}
	bridge.coordinator = newAnswerOutputCoordinatorWithEmitter(
		"CONVERSATION_WORKFLOW",
		"run-1",
		map[string]interface{}{"sys.conversation_id": "conversation-1"},
		streamGraph,
		bridge.emitStreamEvent,
	)

	bridge.handleStreamChunk("llm", &workflowshared.RunStreamChunkEvent{
		ChunkContent:         "live chunk",
		FromVariableSelector: []string{"llm", "text"},
	})
	if !bridge.streamed() {
		t.Fatal("streamed() = false, want live delegate chunk")
	}
	if len(events) != 1 || events[0].Type != "message" || events[0].Payload["answer"] != "live chunk" {
		t.Fatalf("events = %#v, want one synchronous delegate message", events)
	}
	if events[0].Payload["conversation_id"] != "conversation-1" || events[0].Payload["message_id"] != "message-1" {
		t.Fatalf("message parent = %#v, want parent Agent turn", events[0].Payload)
	}
}

func TestNewAutomationWorkflowDelegateAnswerBridgeAcceptsPreparedGraphData(t *testing.T) {
	req := automationaction.WorkflowRunRequest{
		Invocation: &automationaction.WorkflowInvocationContext{
			Mode:                 automationaction.WorkflowInvocationModeAgentDelegate,
			ParentConversationID: "conversation-1",
			ParentMessageID:      "message-1",
		},
	}
	graphData := map[string]interface{}{
		"nodes": []interface{}{
			map[string]interface{}{"id": "start", "data": map[string]interface{}{"type": "start"}},
			map[string]interface{}{"id": "llm", "data": map[string]interface{}{"type": "llm"}},
			map[string]interface{}{"id": "answer", "data": map[string]interface{}{"type": "answer", "answer": "{{#llm.text#}}"}},
		},
		"edges": []interface{}{
			map[string]interface{}{"source": "start", "sourceHandle": workflowDefaultOutputHandle, "target": "llm"},
			map[string]interface{}{"source": "llm", "sourceHandle": workflowDefaultOutputHandle, "target": "answer"},
		},
	}

	var events []automationaction.WorkflowRunEvent
	workflow := automationWorkflowEventTestWorkflow()
	workflow.Type = workflowdto.WorkflowTypeChat
	bridge := newAutomationWorkflowDelegateAnswerBridge(
		context.Background(),
		req,
		workflow,
		"run-1",
		graphData,
		map[string]interface{}{"sys.conversation_id": "conversation-1"},
		func(event automationaction.WorkflowRunEvent) { events = append(events, event) },
	)
	if bridge == nil {
		t.Fatal("bridge = nil, want prepared Agent delegate stream")
	}

	bridge.handleStreamChunk("llm", &workflowshared.RunStreamChunkEvent{
		ChunkContent:         "live chunk",
		FromVariableSelector: []string{"llm", "text"},
	})
	if len(events) != 1 || events[0].Type != "message" || events[0].Payload["answer"] != "live chunk" {
		t.Fatalf("events = %#v, want live message from prepared graph", events)
	}
}

func TestWorkflowRunTriggeredFrom(t *testing.T) {
	tests := []struct {
		name       string
		invocation *automationaction.WorkflowInvocationContext
		want       string
	}{
		{name: "automation", want: string(InvokeFromAutomation)},
		{
			name: "agent conversation delegate",
			invocation: &automationaction.WorkflowInvocationContext{
				Mode: automationaction.WorkflowInvocationModeAgentDelegate,
			},
			want: string(InvokeFromWorkflow),
		},
		{
			name: "agent task tool",
			invocation: &automationaction.WorkflowInvocationContext{
				Mode: automationaction.WorkflowInvocationModeAgentTaskTool,
			},
			want: string(InvokeFromWorkflow),
		},
		{
			name: "standalone invocation",
			invocation: &automationaction.WorkflowInvocationContext{
				Mode: automationaction.WorkflowInvocationModeStandalone,
			},
			want: string(InvokeFromAutomation),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := workflowRunTriggeredFrom(tt.invocation); got != tt.want {
				t.Fatalf("workflowRunTriggeredFrom() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestPersistAutomationWorkflowNodeRuntimeLogsSkipsUnexecutedGraphStates(t *testing.T) {
	repo := &mockWorkflowNodeRuntimeLogRepo{}
	service := &WorkflowService{workflowNodeRuntimeLogRepo: repo}
	startedAt := time.Unix(1700000000, 0)
	finishedAt := startedAt.Add(time.Second)
	result := &WorkflowExecutionResult{
		NodeExecutions: []graph_engine.NodeExecutionSnapshot{
			{NodeID: "start", NodeType: workflowshared.Start, Status: workflowshared.SUCCEEDED, StartTime: startedAt, EndTime: finishedAt},
			{NodeID: "approval", NodeType: workflowshared.Approval, Status: workflowshared.PAUSED, StartTime: finishedAt, EndTime: finishedAt},
			{NodeID: "inactive-loop", NodeType: workflowshared.Loop, Status: workflowshared.PENDING},
			{NodeID: "inactive-end", NodeType: workflowshared.End, Status: workflowshared.SKIPPED},
		},
	}
	workflow := &Workflow{ID: "workflow-1", AgentID: "agent-1"}

	err := service.persistAutomationWorkflowNodeRuntimeLogs(
		context.Background(),
		"workspace-1",
		"account-1",
		string(InvokeFromWorkflow),
		workflow,
		map[string]interface{}{},
		"run-1",
		result,
	)
	if err != nil {
		t.Fatalf("persist node runtime logs: %v", err)
	}

	if len(repo.createdLogs) != 2 {
		t.Fatalf("created node logs = %d, want 2: %#v", len(repo.createdLogs), repo.createdLogs)
	}
	if repo.createdLogs[0].NodeID != "start" || repo.createdLogs[1].NodeID != "approval" {
		t.Fatalf("created node IDs = [%s %s], want [start approval]", repo.createdLogs[0].NodeID, repo.createdLogs[1].NodeID)
	}
	for _, nodeLog := range repo.createdLogs {
		if nodeLog.TriggeredFrom != string(InvokeFromWorkflow) {
			t.Fatalf("node %s triggered_from = %q, want %q", nodeLog.NodeID, nodeLog.TriggeredFrom, InvokeFromWorkflow)
		}
	}
	if got := workflowExecutionResultStepCount(result); got != 2 {
		t.Fatalf("executed step count = %d, want 2", got)
	}
}

func TestAutomationWorkflowInputsKeepHostAndWorkflowConversationDomainsSeparate(t *testing.T) {
	inputs := automationWorkflowInputs(automationaction.WorkflowRunRequest{
		OrganizationID: "org-1",
		WorkspaceID:    "workspace-1",
		AccountID:      "account-1",
		Inputs:         map[string]interface{}{"query": "hello"},
		Invocation: &automationaction.WorkflowInvocationContext{
			InvocationID:         "invocation-1",
			Mode:                 automationaction.WorkflowInvocationModeAgentDelegate,
			ParentConversationID: "agent-conversation-1",
			ParentMessageID:      "agent-message-1",
		},
	}, &Workflow{ID: "workflow-1", AgentID: "workflow-agent-1", Type: workflowdto.WorkflowTypeChat})

	if got := inputs["sys.parent_conversation_id"]; got != "agent-conversation-1" {
		t.Fatalf("sys.parent_conversation_id = %#v, want agent-conversation-1", got)
	}
	if _, exists := inputs["sys.conversation_id"]; exists {
		t.Fatalf("host Agent conversation leaked into workflow conversation domain: %#v", inputs)
	}
	if got := inputs["sys.parent_message_id"]; got != "agent-message-1" {
		t.Fatalf("sys.parent_message_id = %#v, want agent-message-1", got)
	}
	if got := inputs["sys.workflow_type"]; got != string(workflowdto.WorkflowTypeChat) {
		t.Fatalf("sys.workflow_type = %#v, want chat", got)
	}
}

func TestAutomationWorkflowOutputsUsesRuntimeOutputs(t *testing.T) {
	runtimeState := entities.NewGraphRuntimeState(entities.NewVariablePool())
	runtimeState.UpdateOutputs(func(current map[string]any) map[string]any {
		current["answer"] = "春风入纸"
		current["extra"] = "visible"
		return current
	})
	result := &WorkflowExecutionResult{
		RuntimeState: runtimeState,
		NodeResults: map[string]interface{}{
			"answer-node": map[string]interface{}{
				"status": "succeeded",
			},
		},
	}

	outputs := automationWorkflowOutputs(result)

	if got := outputs["answer"]; got != "春风入纸" {
		t.Fatalf("outputs[answer] = %#v, want 春风入纸", got)
	}
	if got := outputs["extra"]; got != "visible" {
		t.Fatalf("outputs[extra] = %#v, want visible", got)
	}
	if _, ok := outputs["answer-node"]; ok {
		t.Fatalf("outputs[answer-node] contains node status summary")
	}
}

func TestAutomationWorkflowOutputsMergesApprovalFieldsFromNodeExecutions(t *testing.T) {
	runtimeState := entities.NewGraphRuntimeState(entities.NewVariablePool())
	runtimeState.UpdateOutputs(func(current map[string]any) map[string]any {
		current["answer"] = "waiting"
		return current
	})
	result := &WorkflowExecutionResult{
		RuntimeState: runtimeState,
		NodeResults: map[string]interface{}{
			"approval": map[string]interface{}{
				"status": "paused",
			},
		},
		NodeExecutions: []graph_engine.NodeExecutionSnapshot{
			{
				NodeID:   "approval",
				NodeType: workflowshared.Approval,
				Outputs: map[string]interface{}{
					"__approval_form_id": "form-1",
					"__approval_token":   "token-1",
					"__approval_form": map[string]interface{}{
						"id":    "form-1",
						"token": "token-1",
						"url":   "/workflow/approval/form-1",
					},
				},
			},
		},
	}

	outputs := automationWorkflowOutputs(result)

	if got := outputs["__approval_form_id"]; got != "form-1" {
		t.Fatalf("outputs[__approval_form_id] = %#v, want form-1", got)
	}
	if got := outputs["__approval_token"]; got != "token-1" {
		t.Fatalf("outputs[__approval_token] = %#v, want token-1", got)
	}
	form, ok := outputs["__approval_form"].(map[string]interface{})
	if !ok {
		t.Fatalf("outputs[__approval_form] = %#v, want map", outputs["__approval_form"])
	}
	if got := form["url"]; got != "/workflow/approval/form-1" {
		t.Fatalf("outputs[__approval_form].url = %#v, want /workflow/approval/form-1", got)
	}
	if _, ok := outputs["approval"]; ok {
		t.Fatalf("outputs[approval] contains node status summary")
	}
}

func TestAutomationWorkflowOutputsKeepsExistingApprovalFields(t *testing.T) {
	runtimeState := entities.NewGraphRuntimeState(entities.NewVariablePool())
	runtimeState.UpdateOutputs(func(current map[string]any) map[string]any {
		current["__approval_form_id"] = "existing-form"
		return current
	})
	result := &WorkflowExecutionResult{
		RuntimeState: runtimeState,
		NodeResults: map[string]interface{}{
			"__approval_form_id": "existing-form",
		},
		NodeExecutions: []graph_engine.NodeExecutionSnapshot{
			{
				NodeID:   "approval",
				NodeType: workflowshared.Approval,
				Outputs: map[string]interface{}{
					"__approval_form_id": "new-form",
				},
			},
		},
	}

	outputs := automationWorkflowOutputs(result)

	if got := outputs["__approval_form_id"]; got != "existing-form" {
		t.Fatalf("outputs[__approval_form_id] = %#v, want existing-form", got)
	}
}

func TestAutomationWorkflowOutputsDoesNotPromoteNonApprovalToken(t *testing.T) {
	result := &WorkflowExecutionResult{
		NodeExecutions: []graph_engine.NodeExecutionSnapshot{
			{
				NodeID:   "custom",
				NodeType: workflowshared.Code,
				Outputs: map[string]interface{}{
					"approval_token": "not-an-approval",
				},
			},
		},
	}

	outputs := automationWorkflowOutputs(result)

	if _, ok := outputs["approval_token"]; ok {
		t.Fatalf("outputs[approval_token] promoted from non-approval node")
	}
}

func TestEmitAutomationWorkflowNodeFinishedIncludesElapsedMilliseconds(t *testing.T) {
	startedAt := time.Unix(1700000000, 0)
	finishedAt := startedAt.Add(1500 * time.Millisecond)
	var got automationaction.WorkflowRunEvent

	emitAutomationWorkflowNodeFinished(func(event automationaction.WorkflowRunEvent) {
		got = event
	}, automationWorkflowEventTestRequest(), automationWorkflowEventTestWorkflow(), "run-1", automationWorkflowNodeMeta{
		NodeID:   "node-1",
		NodeType: "llm",
		Title:    "LLM",
	}, graph_engine.NodeFinishedEvent{
		NodeID:      "node-1",
		NodeType:    "llm",
		Status:      "succeeded",
		Outputs:     map[string]any{"text": "ok"},
		StartedAt:   startedAt,
		FinishedAt:  finishedAt,
		ElapsedTime: finishedAt.Sub(startedAt),
	})

	if got.Type != automationWorkflowEventNodeFinished {
		t.Fatalf("event type = %q, want %q", got.Type, automationWorkflowEventNodeFinished)
	}
	if elapsed, ok := got.Payload["elapsed_time"].(float64); !ok || elapsed != 1500 {
		t.Fatalf("elapsed_time = %#v, want 1500 ms", got.Payload["elapsed_time"])
	}
	if got.Payload["created_at"] != startedAt.Unix() || got.Payload["finished_at"] != finishedAt.Unix() {
		t.Fatalf("timestamps = %#v, want started/finished unix seconds", got.Payload)
	}
}

func TestEmitAutomationWorkflowIterationCompletedIncludesElapsedMilliseconds(t *testing.T) {
	startedAt := time.Unix(1700000000, 0)
	finishedAt := startedAt.Add(2200 * time.Millisecond)
	var got automationaction.WorkflowRunEvent

	emitAutomationWorkflowIterationEvent(func(event automationaction.WorkflowRunEvent) {
		got = event
	}, automationWorkflowEventTestRequest(), automationWorkflowEventTestWorkflow(), "run-1", map[string]automationWorkflowNodeMeta{
		"iter-1": {NodeID: "iter-1", NodeType: "iteration", Title: "Iteration"},
	}, &graph_engine.IterationEvent{
		Type:      "completed",
		NodeID:    "iter-1",
		Index:     2,
		StartedAt: startedAt,
		Timestamp: finishedAt,
		Outputs:   map[string]any{"items": 2},
	})

	if got.Type != automationWorkflowEventIterationFinished {
		t.Fatalf("event type = %q, want %q", got.Type, automationWorkflowEventIterationFinished)
	}
	if elapsed, ok := got.Payload["elapsed_time"].(float64); !ok || elapsed != 2200 {
		t.Fatalf("elapsed_time = %#v, want 2200 ms", got.Payload["elapsed_time"])
	}
}

func automationWorkflowEventTestRequest() automationaction.WorkflowRunRequest {
	return automationaction.WorkflowRunRequest{
		WorkflowRef: automationaction.WorkflowRef{
			AgentID:    "agent-1",
			WorkflowID: "workflow-1",
		},
	}
}

func automationWorkflowEventTestWorkflow() *Workflow {
	return &Workflow{
		ID:      "workflow-1",
		AgentID: "agent-1",
		Version: "v1",
	}
}
