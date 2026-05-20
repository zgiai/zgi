package workflow

import (
	"context"
	"fmt"
	"time"

	"github.com/zgiai/zgi/api/internal/modules/app/workflow/graph_engine"
	workflow_shared "github.com/zgiai/zgi/api/internal/modules/app/workflow/shared"
	"github.com/zgiai/zgi/api/pkg/logger"
	"go.uber.org/zap"
)

func newWorkflowStreamChunkCallback(
	ctx context.Context,
	resultChan chan<- *WorkflowStreamEvent,
	nodeMap map[string]map[string]interface{},
	systemInputs map[string]interface{},
	runType string,
	watchedSelectors map[string]bool,
	streamWatchConfig streamSelectorWatchConfig,
	workflowRunID string,
	conversationMessageChunkNodes map[string]bool,
	answerCoordinator *answerOutputCoordinator,
) func(nodeID string, streamEvent *workflow_shared.RunStreamChunkEvent) {
	return func(nodeID string, streamEvent *workflow_shared.RunStreamChunkEvent) {
		logger.DebugContext(ctx, "workflow stream chunk callback called",
			zap.String("node_id", nodeID),
			zap.Int("selector_depth", len(streamEvent.FromVariableSelector)),
			zap.Int("chunk_length", len(streamEvent.ChunkContent)),
		)

		currentNodeType := workflowStreamNodeType(nodeMap, nodeID)
		if len(streamEvent.FromVariableSelector) < 2 {
			return
		}

		selector := streamEvent.FromVariableSelector[0] + "|" + streamEvent.FromVariableSelector[1]
		if runType == "CONVERSATION_WORKFLOW" && answerCoordinator != nil && answerCoordinator.Enabled() {
			if answerCoordinator.HandleStreamChunk(nodeID, streamEvent) {
				return
			}
		}

		logger.Debug("checking stream chunk selector",
			"node_type", currentNodeType,
			"run_type", runType,
			"is_watched", watchedSelectors[selector],
			"chunk_length", len(streamEvent.ChunkContent),
		)

		if runType == "CONVERSATION_WORKFLOW" && streamEvent.FromVariableSelector[1] == "text" && shouldForwardConversationMessageChunk(currentNodeType, selector, streamWatchConfig) {
			conversationID := ""
			if convID, ok := systemInputs["sys.conversation_id"].(string); ok {
				conversationID = convID
			} else {
				logger.WarnContext(ctx, "conversation id missing for stream chunk",
					zap.Int("system_input_count", len(systemInputs)),
				)
			}

			logger.DebugContext(ctx, "sending conversation workflow stream message",
				zap.String("conversation_id", conversationID),
				zap.Int("chunk_length", len(streamEvent.ChunkContent)),
			)

			resultChan <- &WorkflowStreamEvent{
				EventType: "message",
				Data: map[string]interface{}{
					"id":              workflowRunID,
					"message_id":      workflowRunID,
					"conversation_id": conversationID,
					"answer":          streamEvent.ChunkContent,
					"created_at":      time.Now().Unix(),
				},
			}
			if currentNodeType == "answer" {
				conversationMessageChunkNodes[nodeID] = true
			}
			return
		}

		if runType != "CONVERSATION_WORKFLOW" && watchedSelectors[selector] {
			logger.Debug("forwarding text chunk event for task workflow", "chunk_length", len(streamEvent.ChunkContent))
			resultChan <- &WorkflowStreamEvent{
				EventType: "text_chunk",
				Data: map[string]interface{}{
					"from_variable_selector": streamEvent.FromVariableSelector,
					"text":                   streamEvent.ChunkContent,
				},
			}
			logger.Debug("text chunk event sent")
			return
		}

		logger.Debug("stream chunk selector not watched", "run_type", runType)
	}
}

func newWorkflowIterationCallback(
	ctx context.Context,
	resultChan chan<- *WorkflowStreamEvent,
	nodeMap map[string]map[string]interface{},
) func(event *graph_engine.IterationEvent) {
	return func(event *graph_engine.IterationEvent) {
		logger.DebugContext(ctx, "iteration callback event received",
			zap.String("node_id", event.NodeID),
			zap.String("node_type", "iteration"),
			zap.String("event_type", event.Type),
			zap.Int("index", event.Index),
		)

		nodeTitle := workflowNodeTitle(nodeMap, event.NodeID, "迭代")
		switch event.Type {
		case "started":
			startedAt := event.StartedAt
			if startedAt.IsZero() {
				startedAt = event.Timestamp
			}
			resultChan <- &WorkflowStreamEvent{
				EventType: "iteration_started",
				Data: map[string]interface{}{
					"id":               event.NodeID,
					"node_id":          event.NodeID,
					"node_type":        "iteration",
					"title":            nodeTitle,
					"created_at":       startedAt.Unix(),
					"created_at_ms":    startedAt.UnixMilli(),
					"extras":           map[string]interface{}{},
					"metadata":         event.Metadata,
					"inputs":           event.Inputs,
					"inputs_truncated": false,
				},
			}
		case "next":
			resultChan <- &WorkflowStreamEvent{
				EventType: "iteration_next",
				Data: map[string]interface{}{
					"id":            event.NodeID,
					"node_id":       event.NodeID,
					"node_type":     "iteration",
					"title":         nodeTitle,
					"index":         event.Index,
					"created_at":    event.Timestamp.Unix(),
					"created_at_ms": event.Timestamp.UnixMilli(),
					"extras":        map[string]interface{}{},
				},
			}
		case "completed":
			startedAt := event.StartedAt
			if startedAt.IsZero() {
				startedAt = event.Timestamp
			}
			finishedAt := event.Timestamp
			elapsedTime := elapsedMillisecondsBetween(startedAt, finishedAt)
			totalTokens := 0
			if event.Metadata != nil {
				if tokens, ok := event.Metadata["total_tokens"].(int); ok {
					totalTokens = tokens
				}
			}
			resultChan <- &WorkflowStreamEvent{
				EventType: "iteration_completed",
				Data: map[string]interface{}{
					"id":                 event.NodeID,
					"node_id":            event.NodeID,
					"node_type":          "iteration",
					"title":              nodeTitle,
					"outputs":            event.Outputs,
					"outputs_truncated":  false,
					"created_at":         startedAt.Unix(),
					"created_at_ms":      startedAt.UnixMilli(),
					"extras":             map[string]interface{}{},
					"inputs":             event.Inputs,
					"inputs_truncated":   false,
					"status":             "succeeded",
					"error":              nil,
					"elapsed_time":       elapsedTime,
					"total_tokens":       totalTokens,
					"execution_metadata": event.Metadata,
					"finished_at":        finishedAt.Unix(),
					"finished_at_ms":     finishedAt.UnixMilli(),
					"steps":              event.Steps,
				},
			}
		case "failed":
			startedAt := event.StartedAt
			if startedAt.IsZero() {
				startedAt = event.Timestamp
			}
			finishedAt := event.Timestamp
			elapsedTime := elapsedMillisecondsBetween(startedAt, finishedAt)
			resultChan <- &WorkflowStreamEvent{
				EventType: "iteration_completed",
				Data: map[string]interface{}{
					"id":                 event.NodeID,
					"node_id":            event.NodeID,
					"node_type":          "iteration",
					"title":              nodeTitle,
					"outputs":            event.Outputs,
					"outputs_truncated":  false,
					"created_at":         startedAt.Unix(),
					"created_at_ms":      startedAt.UnixMilli(),
					"extras":             map[string]interface{}{},
					"inputs":             event.Inputs,
					"inputs_truncated":   false,
					"status":             "failed",
					"error":              event.Error,
					"elapsed_time":       elapsedTime,
					"total_tokens":       0,
					"execution_metadata": event.Metadata,
					"finished_at":        finishedAt.Unix(),
					"finished_at_ms":     finishedAt.UnixMilli(),
					"steps":              event.Steps,
				},
			}
		}
	}
}

func newWorkflowLoopCallback(
	ctx context.Context,
	resultChan chan<- *WorkflowStreamEvent,
	nodeMap map[string]map[string]interface{},
) func(event *graph_engine.LoopEvent) {
	return func(event *graph_engine.LoopEvent) {
		logger.DebugContext(ctx, "loop callback event received",
			zap.String("node_id", event.NodeID),
			zap.String("node_type", "loop"),
			zap.String("event_type", event.Type),
			zap.Int("index", event.Index),
		)

		nodeTitle := workflowNodeTitle(nodeMap, event.NodeID, "Loop")
		switch event.Type {
		case "started":
			startedAt := event.StartedAt
			if startedAt.IsZero() {
				startedAt = event.Timestamp
			}
			resultChan <- &WorkflowStreamEvent{
				EventType: "loop_started",
				Data: map[string]interface{}{
					"id":               event.NodeID,
					"node_id":          event.NodeID,
					"node_type":        "loop",
					"title":            nodeTitle,
					"created_at":       startedAt.Unix(),
					"created_at_ms":    startedAt.UnixMilli(),
					"extras":           map[string]interface{}{},
					"metadata":         event.Metadata,
					"inputs":           event.Inputs,
					"inputs_truncated": false,
				},
			}
		case "next":
			resultChan <- &WorkflowStreamEvent{
				EventType: "loop_next",
				Data: map[string]interface{}{
					"id":              event.NodeID,
					"node_id":         event.NodeID,
					"node_type":       "loop",
					"title":           nodeTitle,
					"index":           event.Index,
					"pre_loop_output": event.PreLoopOutput,
					"created_at":      event.Timestamp.Unix(),
					"created_at_ms":   event.Timestamp.UnixMilli(),
					"extras":          map[string]interface{}{},
				},
			}
		case "completed":
			startedAt := event.StartedAt
			if startedAt.IsZero() {
				startedAt = event.Timestamp
			}
			finishedAt := event.Timestamp
			elapsedTime := elapsedMillisecondsBetween(startedAt, finishedAt)
			totalTokens := 0
			if event.Metadata != nil {
				if tokens, ok := event.Metadata["total_tokens"].(int); ok {
					totalTokens = tokens
				}
			}
			resultChan <- &WorkflowStreamEvent{
				EventType: "loop_completed",
				Data: map[string]interface{}{
					"id":                 event.NodeID,
					"node_id":            event.NodeID,
					"node_type":          "loop",
					"title":              nodeTitle,
					"outputs":            event.Outputs,
					"outputs_truncated":  false,
					"created_at":         startedAt.Unix(),
					"created_at_ms":      startedAt.UnixMilli(),
					"extras":             map[string]interface{}{},
					"inputs":             event.Inputs,
					"inputs_truncated":   false,
					"status":             "succeeded",
					"error":              nil,
					"elapsed_time":       elapsedTime,
					"total_tokens":       totalTokens,
					"execution_metadata": event.Metadata,
					"finished_at":        finishedAt.Unix(),
					"finished_at_ms":     finishedAt.UnixMilli(),
					"steps":              event.Steps,
				},
			}
		case "failed":
			startedAt := event.StartedAt
			if startedAt.IsZero() {
				startedAt = event.Timestamp
			}
			finishedAt := event.Timestamp
			elapsedTime := elapsedMillisecondsBetween(startedAt, finishedAt)
			resultChan <- &WorkflowStreamEvent{
				EventType: "loop_completed",
				Data: map[string]interface{}{
					"id":                 event.NodeID,
					"node_id":            event.NodeID,
					"node_type":          "loop",
					"title":              nodeTitle,
					"outputs":            event.Outputs,
					"outputs_truncated":  false,
					"created_at":         startedAt.Unix(),
					"created_at_ms":      startedAt.UnixMilli(),
					"extras":             map[string]interface{}{},
					"inputs":             event.Inputs,
					"inputs_truncated":   false,
					"status":             "failed",
					"error":              event.Error,
					"elapsed_time":       elapsedTime,
					"total_tokens":       0,
					"execution_metadata": event.Metadata,
					"finished_at":        finishedAt.Unix(),
					"finished_at_ms":     finishedAt.UnixMilli(),
					"steps":              event.Steps,
				},
			}
		}
	}
}

func newWorkflowInternalNodeCallback(
	ctx context.Context,
	resultChan chan<- *WorkflowStreamEvent,
	nodeMap map[string]map[string]interface{},
	answerCoordinator *answerOutputCoordinator,
) func(event *graph_engine.NodeEvent) {
	return func(event *graph_engine.NodeEvent) {
		logger.DebugContext(ctx, "internal node callback event received",
			zap.String("node_id", event.NodeID),
			zap.String("node_type", event.Type),
			zap.String("status", event.Status),
		)

		nodeTitle := workflowNodeTitle(nodeMap, event.NodeID, "")
		nodeType := workflowStreamNodeType(nodeMap, event.NodeID)
		if answerCoordinator != nil {
			if scope, ok := answerScopeFromMetadata(event.Metadata); ok {
				if iterationOutputs, hasIterationOutputs := answerIterationContextFromMetadata(event.Metadata); hasIterationOutputs {
					answerCoordinator.MarkScopedSourceAvailable(scope, scope.parentNodeID, iterationOutputs)
				}
				switch event.Type {
				case "started":
					if nodeType == "answer" {
						answerCoordinator.MarkAnswerActiveScoped(scope, event.NodeID)
					}
				case "finished":
					answerCoordinator.MarkNodeFinishedScoped(scope, event.NodeID, nodeType, event.Status, event.Outputs, eventErrorFromString(event.Error))
				}
			}
		}
		if streamEvent := buildInternalNodeWorkflowStreamEvent(event, nodeType, nodeTitle); streamEvent != nil {
			if streamEvent.EventType == "node_finished" {
				now := time.Now()
				streamEvent.Data["finished_at"] = now.Unix()
				streamEvent.Data["finished_at_ms"] = now.UnixMilli()
			}
			resultChan <- streamEvent
		}
	}
}

func eventErrorFromString(message string) error {
	if message == "" {
		return nil
	}
	return fmt.Errorf("%s", message)
}

func workflowNodeTitle(nodeMap map[string]map[string]interface{}, nodeID, fallback string) string {
	if node, exists := nodeMap[nodeID]; exists {
		if nodeData, ok := node["data"].(map[string]interface{}); ok {
			if title, ok := nodeData["title"].(string); ok && title != "" {
				return title
			}
		}
	}
	return fallback
}

func workflowStreamNodeType(nodeMap map[string]map[string]interface{}, nodeID string) string {
	if node, exists := nodeMap[nodeID]; exists {
		if nodeData, ok := node["data"].(map[string]interface{}); ok {
			if nodeType, ok := nodeData["type"].(string); ok {
				return nodeType
			}
		}
	}
	return ""
}
