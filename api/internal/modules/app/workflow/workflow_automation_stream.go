package workflow

import (
	"context"
	"strings"
	"sync"

	"github.com/zgiai/zgi/api/internal/modules/app/workflow/graph_engine"
	workflowshared "github.com/zgiai/zgi/api/internal/modules/app/workflow/shared"
	automationaction "github.com/zgiai/zgi/api/internal/modules/automation/service/action"
	"github.com/zgiai/zgi/api/pkg/logger"
	"go.uber.org/zap"
)

// automationWorkflowDelegateAnswerBridge reuses the conversation workflow
// answer coordinator while publishing its ordered chunks through the embedded
// Agent workflow event sink. Its emitter is synchronous so answer chunks stay
// ordered relative to node lifecycle events.
type automationWorkflowDelegateAnswerBridge struct {
	mu sync.Mutex

	req           automationaction.WorkflowRunRequest
	workflow      *Workflow
	workflowRunID string
	eventSink     automationaction.WorkflowRunEventSink
	streamGraph   *workflowStreamGraph
	coordinator   *answerOutputCoordinator
	messageSent   bool
}

func newAutomationWorkflowDelegateAnswerBridge(
	ctx context.Context,
	req automationaction.WorkflowRunRequest,
	workflow *Workflow,
	workflowRunID string,
	graphData map[string]interface{},
	inputs map[string]interface{},
	eventSink automationaction.WorkflowRunEventSink,
) *automationWorkflowDelegateAnswerBridge {
	if workflowInvocationMode(req.Invocation) != automationaction.WorkflowInvocationModeAgentDelegate ||
		automationWorkflowPauseRunType(workflow) != "CONVERSATION_WORKFLOW" || eventSink == nil {
		return nil
	}

	streamGraph, err := buildWorkflowStreamGraph(ctx, map[string]any{
		"graph": copyWorkflowAnyMap(graphData),
	})
	if err != nil {
		logger.WarnContext(ctx, "failed to prepare Agent workflow answer stream; falling back to final output",
			zap.String("workflow_run_id", workflowRunID),
			zap.Error(err))
		return nil
	}

	bridge := &automationWorkflowDelegateAnswerBridge{
		req:           req,
		workflow:      workflow,
		workflowRunID: workflowRunID,
		eventSink:     eventSink,
		streamGraph:   streamGraph,
	}
	bridge.coordinator = newAnswerOutputCoordinatorWithEmitter(
		"CONVERSATION_WORKFLOW",
		workflowRunID,
		inputs,
		streamGraph,
		bridge.emitStreamEvent,
	)
	if bridge.coordinator == nil {
		return nil
	}
	return bridge
}

func (b *automationWorkflowDelegateAnswerBridge) handleStreamChunk(nodeID string, event *workflowshared.RunStreamChunkEvent) {
	if b == nil || event == nil || len(event.FromVariableSelector) < 2 {
		return
	}
	if b.coordinator != nil && b.coordinator.HandleStreamChunk(nodeID, event) {
		return
	}

	sourceNodeID := event.FromVariableSelector[0]
	if sourceNodeID == "" {
		sourceNodeID = nodeID
	}
	selector := sourceNodeID + "|" + event.FromVariableSelector[1]
	nodeMap := b.streamGraph.RuntimeNodeMap
	if nodeMap == nil {
		nodeMap = b.streamGraph.NodeMap
	}
	nodeType := workflowStreamNodeType(nodeMap, sourceNodeID)
	if event.FromVariableSelector[1] != "text" ||
		!shouldForwardConversationMessageChunk(nodeType, selector, b.streamGraph.WatchConfig) {
		return
	}
	b.emitAnswerChunk(sourceNodeID, event.ChunkContent)
}

func (b *automationWorkflowDelegateAnswerBridge) markNodeStarted(nodeID string, nodeType string) {
	if b == nil || b.coordinator == nil || nodeType != "answer" {
		return
	}
	b.coordinator.MarkAnswerActive(nodeID)
}

func (b *automationWorkflowDelegateAnswerBridge) markNodeFinished(event graph_engine.NodeFinishedEvent) {
	if b == nil || b.coordinator == nil {
		return
	}
	b.coordinator.MarkNodeFinished(event.NodeID, event.NodeType, event.Status, event.Outputs, event.Err)
	b.coordinator.MarkSelectedHandleReachable(event.NodeID, event.Status, event.EdgeSourceHandle)
}

func (b *automationWorkflowDelegateAnswerBridge) markNodeSkipped(nodeID string) {
	if b == nil || b.coordinator == nil {
		return
	}
	b.coordinator.MarkNodeSkipped(nodeID)
}

func (b *automationWorkflowDelegateAnswerBridge) registerReadyBatch(scope graph_engine.ReadyBatchScope, nodeIDs []string) {
	if b == nil || b.coordinator == nil {
		return
	}
	b.coordinator.RegisterReadyBatch(answerScopeFromReadyBatch(scope), nodeIDs)
}

func (b *automationWorkflowDelegateAnswerBridge) handleInternalNode(event *graph_engine.NodeEvent) {
	if b == nil || b.coordinator == nil || event == nil {
		return
	}
	scope, ok := answerScopeFromMetadata(event.Metadata)
	if !ok {
		return
	}
	if iterationOutputs, available := answerIterationContextFromMetadata(event.Metadata); available {
		b.coordinator.MarkScopedSourceAvailable(scope, scope.parentNodeID, iterationOutputs)
	}
	nodeMap := b.streamGraph.RuntimeNodeMap
	if nodeMap == nil {
		nodeMap = b.streamGraph.NodeMap
	}
	nodeType := workflowStreamNodeType(nodeMap, event.NodeID)
	switch event.Type {
	case "started":
		if nodeType == "answer" {
			b.coordinator.MarkAnswerActiveScoped(scope, event.NodeID)
		}
	case "finished":
		b.coordinator.MarkNodeFinishedScoped(scope, event.NodeID, nodeType, event.Status, event.Outputs, eventErrorFromString(event.Error))
	}
}

func (b *automationWorkflowDelegateAnswerBridge) emitStreamEvent(event *WorkflowStreamEvent) {
	if b == nil || event == nil || event.EventType != "message" {
		return
	}
	nodeID, _ := event.Data["node_id"].(string)
	b.emitAnswerChunk(strings.TrimSpace(nodeID), workflowMessageEventText(event.Data))
}

func (b *automationWorkflowDelegateAnswerBridge) emitAnswerChunk(nodeID string, chunk string) {
	if b == nil || chunk == "" {
		return
	}

	b.mu.Lock()
	defer b.mu.Unlock()
	payload := automationWorkflowBasePayload(b.req, b.workflow, b.workflowRunID, map[string]interface{}{
		"id":      b.workflowRunID,
		"node_id": nodeID,
		"answer":  chunk,
	})
	if b.req.Invocation != nil {
		if conversationID := strings.TrimSpace(b.req.Invocation.ParentConversationID); conversationID != "" {
			payload["conversation_id"] = conversationID
		}
		if messageID := strings.TrimSpace(b.req.Invocation.ParentMessageID); messageID != "" {
			payload["message_id"] = messageID
		}
	}
	emitAutomationWorkflowEvent(b.eventSink, "message", payload)
	b.messageSent = true
}

func (b *automationWorkflowDelegateAnswerBridge) streamed() bool {
	if b == nil {
		return false
	}
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.messageSent
}
