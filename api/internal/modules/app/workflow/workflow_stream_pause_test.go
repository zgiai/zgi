package workflow

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/zgiai/ginext/internal/dto"
	graph_engine "github.com/zgiai/ginext/internal/modules/app/workflow/graph_engine"
	graph_entities "github.com/zgiai/ginext/internal/modules/app/workflow/graph_engine/entities"
	workflowpause "github.com/zgiai/ginext/internal/modules/app/workflow/pause"
	workflow_shared "github.com/zgiai/ginext/internal/modules/app/workflow/shared"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func TestHandleWorkflowStreamPausePersistsAnswerOutputStateAndEmitsMessagesBeforePause(t *testing.T) {
	restoreAnswerCoordinatorConfig(t, 20, nil)
	db := newWorkflowStreamPauseTestDB(t)
	restoreDB := swapWorkflowRuntimeTestDB(db)
	defer restoreDB()

	coordinator, resultChan := newTestAnswerOutputCoordinator(t, []testAnswerNode{
		{id: "llm", nodeType: "llm"},
		{id: "answer", nodeType: "answer", answer: "prefix {{#llm.text#}}"},
	}, nil)
	coordinator.MarkAnswerActive("answer")
	assertAnswerMessages(t, resultChan, []string{"prefix "})

	variable := firstTestAnswerVariableBySource(t, coordinator, "llm")
	variable.chunks = []string{"tail"}

	handleWorkflowStreamPause(workflowStreamPauseParams{
		Ctx:                context.Background(),
		WorkspaceID:        "tenant-1",
		AppID:              "app-1",
		WorkflowRunID:      "run-1",
		WorkflowID:         "workflow-1",
		RunType:            "CONVERSATION_WORKFLOW",
		CurrentNodeID:      "approval",
		Title:              "Approval",
		SequenceNumber:     1,
		NodeIndex:          1,
		NodeType:           "approval",
		Outputs:            map[string]interface{}{},
		AllNodeOutputs:     map[string]interface{}{},
		RequestInputs:      map[string]interface{}{},
		ResponseMode:       "streaming",
		SharedVariablePool: graph_entities.NewVariablePool(),
		ResultChan:         resultChan,
		NodeStartTime:      time.Now(),
		AnswerCoordinator:  coordinator,
	})

	events := drainWorkflowStreamEvents(resultChan)
	if len(events) != 3 {
		t.Fatalf("events = %d, want 3", len(events))
	}
	if events[0].EventType != "message" {
		t.Fatalf("first event = %q, want message", events[0].EventType)
	}
	if got, _ := events[0].Data["answer"].(string); got != "tail" {
		t.Fatalf("message answer = %q, want tail", got)
	}
	if events[1].EventType != workflowEventAnswerSnapshotReady {
		t.Fatalf("second event = %q, want answer_snapshot_ready", events[1].EventType)
	}
	if events[2].EventType != workflowpause.EventWorkflowPaused {
		t.Fatalf("third event = %q, want workflow_paused", events[2].EventType)
	}

	pauseService := workflowpause.NewService(db)
	_, _, state, err := pauseService.GetActiveByWorkflowRunID(context.Background(), "run-1")
	if err != nil {
		t.Fatalf("GetActiveByWorkflowRunID() error = %v", err)
	}
	if state.AnswerOutput == nil {
		t.Fatal("state.AnswerOutput = nil, want snapshot")
	}
	if state.AnswerOutput.FullAnswer != "prefix tail" {
		t.Fatalf("state.AnswerOutput.FullAnswer = %q, want %q", state.AnswerOutput.FullAnswer, "prefix tail")
	}
	if !state.AnswerOutput.MessageSent {
		t.Fatal("state.AnswerOutput.MessageSent = false, want true")
	}
}

func TestHandleWorkflowGraphStreamPausePersistsAnswerOutputState(t *testing.T) {
	restoreAnswerCoordinatorConfig(t, 20, nil)
	db := newWorkflowStreamPauseTestDB(t)
	restoreDB := swapWorkflowRuntimeTestDB(db)
	defer restoreDB()

	coordinator, resultChan := newTestAnswerOutputCoordinator(t, []testAnswerNode{
		{id: "llm", nodeType: "llm"},
		{id: "answer", nodeType: "answer", answer: "prefix {{#llm.text#}}"},
	}, nil)
	coordinator.MarkAnswerActive("answer")
	assertAnswerMessages(t, resultChan, []string{"prefix "})

	handler := &WorkflowHandler{}
	executionResult := &WorkflowExecutionResult{
		NodeExecutions: []graph_engine.NodeExecutionSnapshot{
			{
				NodeID:    "approval",
				NodeType:  workflow_shared.Approval,
				Status:    workflow_shared.PAUSED,
				StartTime: time.Now(),
				Outputs:   map[string]interface{}{},
			},
		},
		RuntimeState: graph_entities.NewGraphRuntimeState(graph_entities.NewVariablePool()),
	}
	pausedSnapshots := workflowGraphPausedSnapshots(executionResult.NodeExecutions)

	handler.handleWorkflowGraphStreamPause(context.Background(), workflowGraphStreamPauseParams{
		WorkspaceID:    "tenant-1",
		AppID:          "app-1",
		WorkflowRunID:  "run-graph",
		WorkflowID:     "workflow-graph",
		SystemInputs:   map[string]interface{}{},
		SequenceNumber: 1,
		RunType:        "CONVERSATION_WORKFLOW",
		Request:        &dto.DraftWorkflowRunRequest{Inputs: map[string]interface{}{}, ResponseMode: "streaming"},
		ResultChan:     resultChan,
		StreamGraph: &workflowStreamGraph{
			NodeMap: map[string]map[string]interface{}{
				"approval": {
					"id": "approval",
					"data": map[string]interface{}{
						"type": "approval",
					},
				},
			},
			ReverseEdgeMap: map[string][]string{},
		},
		RunState: &workflowStreamGraphRunState{
			nodeStates: map[string]workflowStreamGraphNodeState{
				"approval": {
					NodeType:  "approval",
					Title:     "Approval",
					NodeIndex: 1,
					StartedAt: time.Now(),
				},
			},
			failedNodes:              map[string]string{},
			allNodeOutputs:           map[string]interface{}{},
			conversationMessageNodes: map[string]bool{},
		},
		ExecutionResult:   executionResult,
		PausedSnapshots:   pausedSnapshots,
		AnswerCoordinator: coordinator,
	})

	pauseService := workflowpause.NewService(db)
	_, _, state, err := pauseService.GetActiveByWorkflowRunID(context.Background(), "run-graph")
	if err != nil {
		t.Fatalf("GetActiveByWorkflowRunID() error = %v", err)
	}
	if state.AnswerOutput == nil {
		t.Fatal("state.AnswerOutput = nil, want snapshot")
	}
	if state.AnswerOutput.FullAnswer != "prefix " {
		t.Fatalf("state.AnswerOutput.FullAnswer = %q, want %q", state.AnswerOutput.FullAnswer, "prefix ")
	}
}

func TestHandleWorkflowStreamPausePersistsQuestionAnswerPause(t *testing.T) {
	db := newWorkflowStreamPauseTestDB(t)
	restoreDB := swapWorkflowRuntimeTestDB(db)
	defer restoreDB()

	conversationID := uuid.NewString()
	variablePool := graph_entities.NewVariablePool()
	variablePool.SystemVariables.ConversationID = conversationID
	variablePool.Add([]string{"sys", "conversation_id"}, conversationID)

	resultChan := make(chan *WorkflowStreamEvent, 10)
	handleWorkflowStreamPause(workflowStreamPauseParams{
		Ctx:                context.Background(),
		WorkspaceID:        "tenant-1",
		AppID:              "app-1",
		WorkflowRunID:      "run-question",
		WorkflowID:         "workflow-question",
		RunType:            "CONVERSATION_WORKFLOW",
		CurrentNodeID:      "question-node",
		Title:              "Question",
		SequenceNumber:     1,
		NodeIndex:          1,
		NodeType:           "question-answer",
		Outputs:            map[string]interface{}{"question": "Choose one", "round": 0},
		AllNodeOutputs:     map[string]interface{}{},
		RequestInputs:      map[string]interface{}{"sys.conversation_id": conversationID},
		ResponseMode:       "streaming",
		SharedVariablePool: variablePool,
		ResultChan:         resultChan,
		NodeStartTime:      time.Now(),
	})

	events := drainWorkflowStreamEvents(resultChan)
	if len(events) != 3 {
		t.Fatalf("events = %d, want 3", len(events))
	}
	if events[0].EventType != workflowEventMessage {
		t.Fatalf("first event = %q, want message", events[0].EventType)
	}
	if got, _ := events[0].Data["answer"].(string); got != "Choose one\n\n" {
		t.Fatalf("message answer = %q, want formatted question", got)
	}
	if got, _ := events[0].Data["message_kind"].(string); got != workflowMessageKindQuestionAnswerPrompt {
		t.Fatalf("message kind = %q, want question answer prompt", got)
	}
	if got, _ := events[0].Data["conversation_id"].(string); got != conversationID {
		t.Fatalf("message conversation_id = %q, want %q", got, conversationID)
	}
	if events[1].EventType != workflowpause.EventQuestionAnswerRequested {
		t.Fatalf("second event = %q, want question_answer_requested", events[1].EventType)
	}
	if got, _ := events[1].Data["round"].(int); got != 1 {
		t.Fatalf("question answer requested round = %d, want 1", got)
	}
	if events[2].EventType != workflowpause.EventWorkflowPaused {
		t.Fatalf("third event = %q, want workflow_paused", events[2].EventType)
	}
	reasons, ok := events[2].Data["reasons"].([]interface{})
	if !ok || len(reasons) != 1 {
		t.Fatalf("pause reasons = %#v, want one reason", events[2].Data["reasons"])
	}
	reason, ok := reasons[0].(map[string]interface{})
	if !ok {
		t.Fatalf("pause reason type = %T, want map", reasons[0])
	}
	if reason["type"] != workflowpause.ReasonTypeQuestionAnswerRequired {
		t.Fatalf("pause reason = %#v, want question answer required", reason["type"])
	}
	if reason["question"] != "Choose one" {
		t.Fatalf("pause reason question = %#v, want Choose one", reason["question"])
	}
	if reason["round"] != 1 {
		t.Fatalf("pause reason round = %#v, want 1", reason["round"])
	}

	pauseService := workflowpause.NewService(db)
	pause, _, state, err := pauseService.GetActiveByConversationID(context.Background(), "tenant-1", "app-1", conversationID, workflowpause.ReasonTypeQuestionAnswerRequired)
	if err != nil {
		t.Fatalf("GetActiveByConversationID() error = %v", err)
	}
	if pause.NodeID != "question-node" {
		t.Fatalf("pause node id = %q, want question-node", pause.NodeID)
	}
	if state.ExecutorState.PausedNodeID != "question-node" {
		t.Fatalf("paused node id = %q, want question-node", state.ExecutorState.PausedNodeID)
	}
}

func TestBuildQuestionAnswerSubmittedEventIncludesDisplayMetadata(t *testing.T) {
	state := &workflowpause.State{
		ExecutorState: workflowpause.ExecutorState{
			PausedNodeID: "question-node",
			ExecutionOutputs: map[string]map[string]interface{}{
				"question-node": {
					"answers": []interface{}{
						map[string]interface{}{
							"round":    1,
							"question": "City?",
							"answer":   "Beijing",
						},
					},
					"choices": []interface{}{
						map[string]interface{}{"id": "a", "label": "A", "value": "alpha"},
						map[string]interface{}{"id": "b", "label": "B", "value": "beta"},
					},
				},
			},
		},
	}

	event := buildQuestionAnswerSubmittedEvent("run-question", state, map[string]interface{}{
		"query":                     "B",
		"question_answer_option_id": "b",
	})

	if got, _ := event["workflow_run_id"].(string); got != "run-question" {
		t.Fatalf("workflow run id = %q, want run-question", got)
	}
	if got, _ := event["node_id"].(string); got != "question-node" {
		t.Fatalf("node id = %q, want question-node", got)
	}
	if got, _ := event["answer"].(string); got != "B" {
		t.Fatalf("answer = %q, want B", got)
	}
	if got, _ := event["round"].(int); got != 2 {
		t.Fatalf("round = %d, want 2", got)
	}
	if got, _ := event["choice_id"].(string); got != "b" {
		t.Fatalf("choice id = %q, want b", got)
	}
	if got, _ := event["choice_label"].(string); got != "B" {
		t.Fatalf("choice label = %q, want B", got)
	}
	if got, _ := event["choice_value"].(string); got != "beta" {
		t.Fatalf("choice value = %q, want beta", got)
	}
}

func drainWorkflowStreamEvents(resultChan <-chan *WorkflowStreamEvent) []*WorkflowStreamEvent {
	events := make([]*WorkflowStreamEvent, 0)
	for {
		select {
		case event := <-resultChan:
			if event != nil {
				events = append(events, event)
			}
		default:
			return events
		}
	}
}

func newWorkflowStreamPauseTestDB(t *testing.T) *gorm.DB {
	t.Helper()

	db, err := gorm.Open(sqlite.Open("file:"+uuid.NewString()+"?mode=memory&cache=shared"), &gorm.Config{})
	if err != nil {
		if strings.Contains(err.Error(), "requires cgo to work") {
			t.Skipf("sqlite test db unavailable in current environment: %v", err)
		}
		t.Fatalf("open sqlite: %v", err)
	}
	if err := db.AutoMigrate(
		&workflowpause.RunPause{},
		&workflowpause.RunPauseReason{},
		&workflowpause.RunEvent{},
	); err != nil {
		t.Fatalf("auto migrate workflow pause tables: %v", err)
	}
	return db
}
