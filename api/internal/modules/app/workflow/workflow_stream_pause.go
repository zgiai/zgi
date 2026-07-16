package workflow

import (
	"context"
	"strings"
	"time"

	graph_entities "github.com/zgiai/zgi/api/internal/modules/app/workflow/graph_engine/entities"
	workflowpause "github.com/zgiai/zgi/api/internal/modules/app/workflow/pause"
	"github.com/zgiai/zgi/api/pkg/logger"
)

const workflowMessageKindQuestionAnswerPrompt = "question_answer_prompt"

type workflowStreamPauseParams struct {
	Ctx                    context.Context
	WorkspaceID            string
	AppID                  string
	WorkflowRunID          string
	WorkflowID             string
	RunType                string
	TriggeredFrom          string
	CurrentNodeID          string
	Title                  string
	NodeLogID              string
	IsDraft                bool
	SequenceNumber         int
	NodeIndex              int
	TotalWorkflowTokens    int
	NodeType               string
	Outputs                map[string]interface{}
	ProcessData            map[string]any
	ExecutionMetadata      map[string]interface{}
	AllNodeOutputs         map[string]interface{}
	NodeQueue              []string
	CompletedNodes         map[string]bool
	FailedNodes            map[string]string
	ExecutionOutputs       map[string]map[string]interface{}
	PredecessorNodeID      *string
	RequestInputs          map[string]interface{}
	ResponseMode           string
	SharedVariablePool     *graph_entities.VariablePool
	WorkflowService        *WorkflowService
	WorkflowElapsedTracker *workflowElapsedTracker
	ResultChan             chan<- *WorkflowStreamEvent
	NodeStartTime          time.Time
	PausedNodes            []workflowStreamPausedNode
	AnswerCoordinator      *answerOutputCoordinator
}

type workflowStreamPausedNode struct {
	NodeID            string
	Title             string
	NodeLogID         string
	NodeType          string
	NodeIndex         int
	Outputs           map[string]interface{}
	ProcessData       map[string]interface{}
	ExecutionMetadata map[string]interface{}
	PredecessorNodeID *string
	NodeStartTime     time.Time
}

func handleWorkflowStreamPause(params workflowStreamPauseParams) {
	ctx := params.Ctx
	pausedAt := time.Now()
	pausedNodes := params.effectivePausedNodes()
	if len(pausedNodes) == 0 {
		return
	}

	var answerOutputState *workflowpause.AnswerOutputState
	if params.AnswerCoordinator != nil {
		var answerMessages []answerMessageChunk
		answerOutputState, answerMessages = params.AnswerCoordinator.PreparePauseSnapshot()
		params.AnswerCoordinator.emitMessages(answerMessages)
	}

	trackedElapsedTime := 0.0
	totalSteps := params.NodeIndex
	for _, pausedNode := range pausedNodes {
		elapsedTime := ElapsedMillisecondsSince(pausedNode.NodeStartTime)
		if pausedNode.NodeLogID != "" && params.WorkflowService != nil {
			updateErr := params.WorkflowService.PauseWorkflowNodeRuntimeLog(ctx, pausedNode.NodeLogID, pausedNode.Outputs, pausedNode.ProcessData, pausedNode.ExecutionMetadata, elapsedTime)
			if updateErr != nil {
				logger.ErrorContext(ctx, "failed to pause workflow node runtime log", "node_id", pausedNode.NodeID, "node_type", pausedNode.NodeType, updateErr)
			}
		}
		if params.WorkflowElapsedTracker != nil {
			trackedElapsedTime = params.WorkflowElapsedTracker.recordNodeElapsed(pausedNode.NodeLogID, elapsedTime)
		}
		if pausedNode.NodeIndex > totalSteps {
			totalSteps = pausedNode.NodeIndex
		}
	}

	workflowElapsedTime := trackedElapsedTime
	if params.WorkflowService != nil {
		workflowElapsedTime = params.WorkflowService.workflowRunElapsedMillisecondsForEvent(ctx, params.WorkflowRunID, trackedElapsedTime)
	}

	if params.WorkflowRunID != "" && params.WorkflowService != nil {
		if err := params.WorkflowService.PauseWorkflowRunLog(ctx, params.WorkflowRunID, params.AllNodeOutputs, workflowElapsedTime, int64(params.TotalWorkflowTokens), totalSteps); err != nil {
			logger.ErrorContext(ctx, "failed to pause workflow run log", "workflow_run_id", params.WorkflowRunID, err)
		}
	}

	pausedNodeIDs := make([]string, 0, len(pausedNodes))
	eventReasons := make([]interface{}, 0, len(pausedNodes))
	persistReasons := make([]workflowpause.Reason, 0, len(pausedNodes))
	pauseReason := workflowpause.ReasonTypeApprovalRequired
	for _, pausedNode := range pausedNodes {
		pausedNodeIDs = append(pausedNodeIDs, pausedNode.NodeID)
		if pausedNode.NodeType == "question-answer" {
			pauseReason = workflowpause.ReasonTypeQuestionAnswerRequired
			questionAnswerMessageEvent := buildQuestionAnswerMessageEvent(params, pausedNode)
			if len(questionAnswerMessageEvent) > 0 {
				params.ResultChan <- &WorkflowStreamEvent{
					EventType: workflowEventMessage,
					Data:      questionAnswerMessageEvent,
				}
			}
			questionAnswerRequestedEvent := buildQuestionAnswerRequestedEvent(params.WorkflowRunID, pausedNode)
			if len(questionAnswerRequestedEvent) > 0 {
				params.ResultChan <- &WorkflowStreamEvent{
					EventType: workflowpause.EventQuestionAnswerRequested,
					Data:      questionAnswerRequestedEvent,
				}
			}
			eventReason := map[string]interface{}{
				"type":       workflowpause.ReasonTypeQuestionAnswerRequired,
				"node_id":    pausedNode.NodeID,
				"node_title": pausedNode.Title,
			}
			if question, _ := questionAnswerRequestedEvent["question"].(string); strings.TrimSpace(question) != "" {
				eventReason["question"] = question
			}
			if round, ok := questionAnswerRequestedEvent["round"]; ok {
				eventReason["round"] = round
			}
			if choices, ok := questionAnswerRequestedEvent["choices"]; ok {
				eventReason["choices"] = choices
			}
			eventReasons = append(eventReasons, eventReason)
			persistReasons = append(persistReasons, workflowpause.Reason{
				Type:   workflowpause.ReasonTypeQuestionAnswerRequired,
				NodeID: pausedNode.NodeID,
			})
			continue
		}

		approvalRequestedEvent := buildApprovalRequestedEvent(ctx, approvalRequestedEventContext{
			WorkflowRunID: params.WorkflowRunID,
			NodeID:        pausedNode.NodeID,
			NodeTitle:     pausedNode.Title,
			IsDraft:       params.IsDraft,
			TriggeredFrom: params.TriggeredFrom,
		}, pausedNode.Outputs)
		if len(approvalRequestedEvent) > 0 {
			params.ResultChan <- &WorkflowStreamEvent{
				EventType: workflowpause.EventApprovalRequested,
				Data:      approvalRequestedEvent,
			}
		}
		formID, _ := approvalRequestedEvent["form_id"].(string)
		eventReasons = append(eventReasons, map[string]interface{}{
			"type":       workflowpause.ReasonTypeApprovalRequired,
			"node_id":    pausedNode.NodeID,
			"node_title": pausedNode.Title,
			"form_id":    formID,
		})
		persistReasons = append(persistReasons, workflowpause.Reason{
			Type:   workflowpause.ReasonTypeApprovalRequired,
			NodeID: pausedNode.NodeID,
			FormID: formID,
		})
	}

	pauseEvent := map[string]interface{}{
		"id":              params.WorkflowRunID,
		"workflow_id":     params.WorkflowID,
		"sequence_number": params.SequenceNumber,
		"status":          "paused",
		"paused_nodes":    pausedNodeIDs,
		"outputs":         map[string]interface{}{},
		"reasons":         eventReasons,
		"elapsed_time":    workflowElapsedTime,
		"total_tokens":    params.TotalWorkflowTokens,
		"total_steps":     totalSteps,
		"created_at":      time.Now().Unix(),
		"paused_at":       pausedAt.Unix(),
		"files":           []interface{}{},
	}

	pauseInputs := copyWorkflowAnyMap(params.RequestInputs)
	responseMode := params.ResponseMode
	if responseMode == "" {
		responseMode = "streaming"
	}
	pauseState := workflowpause.State{
		Version:       workflowpause.StateVersion,
		WorkflowRunID: params.WorkflowRunID,
		WorkflowID:    params.WorkflowID,
		AppID:         params.AppID,
		TenantID:      params.WorkspaceID,
		RunType:       params.RunType,
		TriggeredFrom: params.TriggeredFrom,
		Request: workflowpause.RequestState{
			Inputs:       pauseInputs,
			ResponseMode: responseMode,
		},
		ExecutorState: workflowpause.ExecutorState{
			PausedNodeID:      pausedNodeIDs[0],
			PausedNodeIDs:     append([]string(nil), pausedNodeIDs...),
			NodeQueue:         append([]string(nil), params.NodeQueue...),
			CompletedNodes:    copyWorkflowBoolMap(params.CompletedNodes),
			FailedNodes:       copyWorkflowStringMap(params.FailedNodes),
			ExecutionOutputs:  copyWorkflowNestedMap(params.ExecutionOutputs),
			AllNodeOutputs:    copyWorkflowAnyMap(params.AllNodeOutputs),
			NodeIndex:         totalSteps,
			TotalTokens:       params.TotalWorkflowTokens,
			PredecessorNodeID: pausedNodes[0].PredecessorNodeID,
		},
		VariablePool: workflowpause.SnapshotVariablePool(params.SharedVariablePool),
		AnswerOutput: answerOutputState,
	}
	if pauseReason == workflowpause.ReasonTypeQuestionAnswerRequired {
		persistQuestionAnswerPause(ctx, params.WorkspaceID, params.AppID, params.WorkflowRunID, pausedNodeIDs[0], persistReasons, pauseState)
	} else {
		persistApprovalPause(ctx, params.WorkspaceID, params.AppID, params.WorkflowRunID, pausedNodeIDs[0], persistReasons, pauseState)
	}

	params.ResultChan <- &WorkflowStreamEvent{
		EventType: workflowpause.EventWorkflowPaused,
		Data:      pauseEvent,
	}
}

func buildQuestionAnswerMessageEvent(params workflowStreamPauseParams, pausedNode workflowStreamPausedNode) map[string]interface{} {
	if params.RunType != "CONVERSATION_WORKFLOW" {
		return nil
	}
	question := workflowQuestionAnswerQuestion(pausedNode.Outputs)
	if question == "" {
		return nil
	}
	return map[string]interface{}{
		"id":              params.WorkflowRunID,
		"message_id":      params.WorkflowRunID,
		"conversation_id": workflowStreamPauseConversationID(params),
		"node_id":         pausedNode.NodeID,
		"message_kind":    workflowMessageKindQuestionAnswerPrompt,
		"answer":          workflowQuestionAnswerMessageText(question),
		"created_at":      time.Now().Unix(),
	}
}

func workflowQuestionAnswerMessageText(question string) string {
	question = strings.TrimSpace(question)
	if question == "" {
		return ""
	}
	return question + "\n\n"
}

func workflowQuestionAnswerQuestion(outputs map[string]interface{}) string {
	if outputs == nil {
		return ""
	}
	question, _ := outputs["question"].(string)
	return strings.TrimSpace(question)
}

func workflowStreamPauseConversationID(params workflowStreamPauseParams) string {
	if params.SharedVariablePool != nil && params.SharedVariablePool.SystemVariables != nil {
		if conversationID := strings.TrimSpace(params.SharedVariablePool.SystemVariables.ConversationID); conversationID != "" {
			return conversationID
		}
	}
	if conversationID, ok := params.RequestInputs["sys.conversation_id"].(string); ok {
		return strings.TrimSpace(conversationID)
	}
	return ""
}

func (params workflowStreamPauseParams) effectivePausedNodes() []workflowStreamPausedNode {
	if len(params.PausedNodes) > 0 {
		return params.PausedNodes
	}
	return []workflowStreamPausedNode{
		{
			NodeID:            params.CurrentNodeID,
			Title:             params.Title,
			NodeLogID:         params.NodeLogID,
			NodeType:          params.NodeType,
			NodeIndex:         params.NodeIndex,
			Outputs:           params.Outputs,
			ProcessData:       params.ProcessData,
			ExecutionMetadata: params.ExecutionMetadata,
			PredecessorNodeID: params.PredecessorNodeID,
			NodeStartTime:     params.NodeStartTime,
		},
	}
}
