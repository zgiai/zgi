package graph_engine

import (
	"errors"
	"fmt"
	"reflect"
	"time"

	"github.com/zgiai/zgi/api/internal/modules/app/workflow/shared"
	"github.com/zgiai/zgi/api/pkg/logger"
)

func (e *WorkflowEngine) consumeNodeEvents(
	nodeID string,
	state *NodeState,
	eventChan <-chan *shared.NodeEventCh,
	result **shared.NodeRunResult,
	execErr *error,
) bool {
	logger.Info("Processing node events for nodeID: %s", nodeID)
	for event := range eventChan {
		if e.IsStopped() {
			logger.Info("Workflow stopped during node event processing: %s", nodeID)
			state.mu.Lock()
			state.Status = shared.FAILED
			state.Error = errors.New("workflow stopped by user")
			state.EndTime = time.Now()
			state.mu.Unlock()
			return false
		}

		logger.Info("Received event for nodeID: %s, eventType: %v", nodeID, event.Type)
		switch event.Type {
		case shared.EventTypeRunCompleted:
			logger.Info("Processing RunCompleted event for nodeID: %s", nodeID)
			if event.Data != nil {
				if runEvent, ok := event.Data.(*shared.RunCompletedEvent); ok {
					*result = runEvent.RunResult
					logger.Info("Got run result from event for nodeID: %s, hasInputs: %v, hasOutputs: %v", nodeID, (*result).Inputs != nil, (*result).Outputs != nil)

					// Check if this is an internal node event (from a subgraph)
					// If so, forward it via the internal node event callback
					if e.internalNodeEventCallback != nil && runEvent.RunResult != nil && runEvent.RunResult.Metadata != nil {
						iterID, hasIter := runEvent.RunResult.Metadata[shared.ITERATION_ID]
						loopID, hasLoop := runEvent.RunResult.Metadata[shared.LoopId]
						if (hasIter && iterID != "") || (hasLoop && loopID != "") {
							metadata := make(map[string]interface{})
							for k, v := range runEvent.RunResult.Metadata {
								metadata[string(k)] = v
							}
							e.internalNodeEventCallback(&NodeEvent{
								ExecutionID: buildInternalExecutionID(event.NodeID, metadata),
								Type:        "finished",
								NodeID:      event.NodeID,
								Inputs:      runEvent.RunResult.Inputs,
								Outputs:     runEvent.RunResult.Outputs,
								Status:      string(runEvent.RunResult.Status),
								Metadata:    metadata,
								Timestamp:   event.Timestamp,
								StartedAt:   nonZeroTime(runEvent.StartedAt, event.Timestamp),
								FinishedAt:  nonZeroTime(runEvent.FinishedAt, event.Timestamp),
							})
						}
					}
				} else {
					logger.Warn("RunCompleted event data is not RunCompletedEvent type for nodeID: %s", nodeID)
				}
			} else {
				logger.Warn("RunCompleted event has no data for nodeID: %s", nodeID)
			}
		case shared.EventTypeRunFailed:
			logger.Info("Processing RunFailed event for nodeID: %s", nodeID)
			if event.Data != nil {
				if failedEvent, ok := event.Data.(*shared.RunFailedEvent); ok && failedEvent.RunResult != nil {
					*result = failedEvent.RunResult
				}
			}
			if event.Error != nil {
				logger.Error(fmt.Sprintf("Got error from RunFailed event for nodeID: %s, error: %v", nodeID, event.Error), event.Error)
				*execErr = event.Error
			} else {
				logger.Warn("RunFailed event has no error for nodeID: %s", nodeID)
			}
		case shared.EventTypeModelInvokeCompleted:
			logger.Info("Processing ModelInvokeCompleted event for nodeID: %s", nodeID)
			if event.Data != nil {
				if modelEvent, ok := event.Data.(*shared.ModelInvokeCompletedEvent); ok {
					logger.Info("Model invocation completed for nodeID: %s, text length: %d", nodeID, len(modelEvent.Text))
				}
			}
		case shared.EventTypeRunStreamChunk:
			logger.Debug("Processing RunStreamChunk event for nodeID: %s", nodeID)
			if e.streamEventCallback != nil && event.Data != nil {
				if streamEvent, ok := event.Data.(*shared.RunStreamChunkEvent); ok {
					e.streamEventCallback(nodeID, streamEvent)
				}
			}

		case shared.EventTypeIterationStarted:
			logger.Info("Processing IterationStarted event for nodeID: %s", nodeID)
			if e.iterationEventCallback != nil && event.Data != nil {
				if iterData, ok := event.Data.(interface{ GetInputs() map[string]any }); ok {
					inputs := iterData.GetInputs()
					var metadata map[string]any
					if metaGetter, ok := event.Data.(interface{ GetMetadata() map[string]any }); ok {
						metadata = metaGetter.GetMetadata()
					}
					e.iterationEventCallback(&IterationEvent{
						Type:      "started",
						NodeID:    nodeID,
						Inputs:    inputs,
						Metadata:  metadata,
						Timestamp: event.Timestamp,
						StartedAt: eventStartedAtFromData(event.Data, event.Timestamp),
					})
				} else if iterMap, ok := event.Data.(map[string]any); ok {
					inputs, _ := iterMap["inputs"].(map[string]any)
					metadata, _ := iterMap["metadata"].(map[string]any)
					e.iterationEventCallback(&IterationEvent{
						Type:      "started",
						NodeID:    nodeID,
						Inputs:    inputs,
						Metadata:  metadata,
						Timestamp: event.Timestamp,
						StartedAt: eventStartedAtFromData(event.Data, event.Timestamp),
					})
				} else {
					// Fallback: try to extract fields using reflection or type assertion on the struct
					e.handleIterationStartedEvent(nodeID, event)
				}
			}

		case shared.EventTypeIterationNext:
			logger.Debug("Processing IterationNext event for nodeID: %s", nodeID)
			if e.iterationEventCallback != nil && event.Data != nil {
				if iterData, ok := event.Data.(interface{ GetIndex() int }); ok {
					e.iterationEventCallback(&IterationEvent{
						Type:      "next",
						NodeID:    nodeID,
						Index:     iterData.GetIndex(),
						Timestamp: event.Timestamp,
					})
				} else if iterMap, ok := event.Data.(map[string]any); ok {
					index, _ := iterMap["index"].(int)
					e.iterationEventCallback(&IterationEvent{
						Type:      "next",
						NodeID:    nodeID,
						Index:     index,
						Timestamp: event.Timestamp,
					})
				} else {
					e.handleIterationNextEvent(nodeID, event)
				}
			}

		case shared.EventTypeIterationSucceeded:
			logger.Info("Processing IterationSucceeded event for nodeID: %s", nodeID)
			if e.iterationEventCallback != nil && event.Data != nil {
				e.handleIterationCompletedEvent(nodeID, event, "completed", "")
			}

		case shared.EventTypeIterationFailed:
			logger.Info("Processing IterationFailed event for nodeID: %s", nodeID)
			if e.iterationEventCallback != nil && event.Data != nil {
				var errMsg string
				if errGetter, ok := event.Data.(interface{ GetError() string }); ok {
					errMsg = errGetter.GetError()
				}
				e.handleIterationCompletedEvent(nodeID, event, "failed", errMsg)
			}

		case shared.EventTypeLoopStarted:
			logger.Info("Processing LoopStarted event for nodeID: %s", nodeID)
			if e.loopEventCallback != nil && event.Data != nil {
				if loopData, ok := event.Data.(interface{ GetInputs() map[string]any }); ok {
					inputs := loopData.GetInputs()
					var metadata map[string]any
					if metaGetter, ok := event.Data.(interface{ GetMetadata() map[string]any }); ok {
						metadata = metaGetter.GetMetadata()
					}
					e.loopEventCallback(&LoopEvent{
						Type:      "started",
						NodeID:    nodeID,
						Inputs:    inputs,
						Metadata:  metadata,
						Timestamp: event.Timestamp,
						StartedAt: eventStartedAtFromData(event.Data, event.Timestamp),
					})
				} else if loopMap, ok := event.Data.(map[string]any); ok {
					inputs, _ := loopMap["inputs"].(map[string]any)
					metadata, _ := loopMap["metadata"].(map[string]any)
					e.loopEventCallback(&LoopEvent{
						Type:      "started",
						NodeID:    nodeID,
						Inputs:    inputs,
						Metadata:  metadata,
						Timestamp: event.Timestamp,
						StartedAt: eventStartedAtFromData(event.Data, event.Timestamp),
					})
				} else {
					e.handleLoopStartedEvent(nodeID, event)
				}
			}

		case shared.EventTypeLoopNext:
			logger.Debug("Processing LoopNext event for nodeID: %s", nodeID)
			if e.loopEventCallback != nil && event.Data != nil {
				if loopData, ok := event.Data.(interface{ GetIndex() int }); ok {
					var preLoopOutput map[string]any
					if outputGetter, ok := event.Data.(interface{ GetPreLoopOutput() map[string]any }); ok {
						preLoopOutput = outputGetter.GetPreLoopOutput()
					}
					e.loopEventCallback(&LoopEvent{
						Type:          "next",
						NodeID:        nodeID,
						Index:         loopData.GetIndex(),
						PreLoopOutput: preLoopOutput,
						Timestamp:     event.Timestamp,
					})
				} else if loopMap, ok := event.Data.(map[string]any); ok {
					index, _ := loopMap["index"].(int)
					preLoopOutput, _ := loopMap["pre_loop_output"].(map[string]any)
					e.loopEventCallback(&LoopEvent{
						Type:          "next",
						NodeID:        nodeID,
						Index:         index,
						PreLoopOutput: preLoopOutput,
						Timestamp:     event.Timestamp,
					})
				} else {
					e.handleLoopNextEvent(nodeID, event)
				}
			}

		case shared.EventTypeLoopSucceeded:
			logger.Info("Processing LoopSucceeded event for nodeID: %s", nodeID)
			if e.loopEventCallback != nil && event.Data != nil {
				e.handleLoopCompletedEvent(nodeID, event, "completed", "")
			}

		case shared.EventTypeLoopFailed:
			logger.Info("Processing LoopFailed event for nodeID: %s", nodeID)
			if e.loopEventCallback != nil && event.Data != nil {
				var errMsg string
				if errGetter, ok := event.Data.(interface{ GetError() string }); ok {
					errMsg = errGetter.GetError()
				}
				e.handleLoopCompletedEvent(nodeID, event, "failed", errMsg)
			}

		case shared.EventTypeInternalNodeStarted:
			logger.Info("Processing InternalNodeStarted event for nodeID: %s", nodeID)
			if e.internalNodeEventCallback != nil && event.Data != nil {
				if runEvent, ok := event.Data.(*shared.RunCompletedEvent); ok && runEvent.RunResult != nil {
					metadata := make(map[string]interface{})
					if runEvent.RunResult.Metadata != nil {
						for k, v := range runEvent.RunResult.Metadata {
							metadata[string(k)] = v
						}
					}
					e.internalNodeEventCallback(&NodeEvent{
						ExecutionID: buildInternalExecutionID(event.NodeID, metadata),
						Type:        "started",
						NodeID:      event.NodeID,
						Inputs:      runEvent.RunResult.Inputs,
						Metadata:    metadata,
						Timestamp:   event.Timestamp,
						StartedAt:   nonZeroTime(runEvent.StartedAt, event.Timestamp),
					})
				}
			}

		case shared.EventTypeInternalReadyBatch:
			logger.Debug("Processing InternalReadyBatch event for nodeID: %s", nodeID)
			if e.onReadyBatch != nil && event.Data != nil {
				if readyBatch, ok := event.Data.(*shared.ReadyBatchEvent); ok && len(readyBatch.NodeIDs) > 0 {
					e.onReadyBatch(ReadyBatchScope{
						Kind:         readyBatch.ScopeKind,
						ParentNodeID: readyBatch.ParentNodeID,
						Index:        readyBatch.Index,
					}, readyBatch.NodeIDs)
				}
			}

		case shared.EventTypeInternalNodeFinished:
			logger.Info("Processing InternalNodeFinished event for nodeID: %s", nodeID)
			if e.internalNodeEventCallback != nil && event.Data != nil {
				if runEvent, ok := event.Data.(*shared.RunCompletedEvent); ok && runEvent.RunResult != nil {
					metadata := make(map[string]interface{})
					if runEvent.RunResult.Metadata != nil {
						for k, v := range runEvent.RunResult.Metadata {
							metadata[string(k)] = v
						}
					}
					// Determine status
					status := "succeeded"
					var errMsg string
					if runEvent.RunResult.Status == shared.FAILED || runEvent.RunResult.Status == shared.EXCEPTION {
						status = "failed"
						errMsg = runEvent.RunResult.ErrMsg
					}
					e.internalNodeEventCallback(&NodeEvent{
						ExecutionID: buildInternalExecutionID(event.NodeID, metadata),
						Type:        "finished",
						NodeID:      event.NodeID,
						Inputs:      runEvent.RunResult.Inputs,
						Outputs:     runEvent.RunResult.Outputs,
						Status:      status,
						Error:       errMsg,
						Metadata:    metadata,
						Timestamp:   event.Timestamp,
						StartedAt:   nonZeroTime(runEvent.StartedAt, event.Timestamp),
						FinishedAt:  nonZeroTime(runEvent.FinishedAt, event.Timestamp),
					})
				}
			}

		default:
			logger.Debug("Event type for nodeID: %s, eventType: %v (not handled by engine)", nodeID, event.Type)
		}
	}
	logger.Info("Finished processing node events for nodeID: %s", nodeID)
	return true
}

func (e *WorkflowEngine) handleIterationStartedEvent(nodeID string, event *shared.NodeEventCh) {
	if e.iterationEventCallback == nil {
		return
	}
	// Use reflection to extract fields from the struct
	iterEvent := &IterationEvent{
		Type:      "started",
		NodeID:    nodeID,
		Timestamp: event.Timestamp,
		StartedAt: eventStartedAtFromData(event.Data, event.Timestamp),
	}

	// Try to extract Inputs field
	if val, ok := getFieldValue(event.Data, "Inputs"); ok {
		if inputs, ok := val.(map[string]any); ok {
			iterEvent.Inputs = inputs
		}
	}
	// Try to extract Metadata field
	if val, ok := getFieldValue(event.Data, "Metadata"); ok {
		if metadata, ok := val.(map[string]any); ok {
			iterEvent.Metadata = metadata
		}
	}

	e.iterationEventCallback(iterEvent)
}

// handleIterationNextEvent handles iteration next events with struct data
func (e *WorkflowEngine) handleIterationNextEvent(nodeID string, event *shared.NodeEventCh) {
	if e.iterationEventCallback == nil {
		return
	}
	iterEvent := &IterationEvent{
		Type:      "next",
		NodeID:    nodeID,
		Timestamp: event.Timestamp,
	}

	// Try to extract Index field
	if val, ok := getFieldValue(event.Data, "Index"); ok {
		if index, ok := val.(int); ok {
			iterEvent.Index = index
		}
	}

	e.iterationEventCallback(iterEvent)
}

// handleIterationCompletedEvent handles iteration completed/failed events
func (e *WorkflowEngine) handleIterationCompletedEvent(nodeID string, event *shared.NodeEventCh, eventType string, errMsg string) {
	if e.iterationEventCallback == nil {
		return
	}
	iterEvent := &IterationEvent{
		Type:      eventType,
		NodeID:    nodeID,
		Error:     errMsg,
		Timestamp: event.Timestamp,
		StartedAt: eventStartedAtFromData(event.Data, time.Time{}),
	}

	// Try to extract Inputs field
	if val, ok := getFieldValue(event.Data, "Inputs"); ok {
		if inputs, ok := val.(map[string]any); ok {
			iterEvent.Inputs = inputs
		}
	}
	// Try to extract Outputs field
	if val, ok := getFieldValue(event.Data, "Outputs"); ok {
		if outputs, ok := val.(map[string]any); ok {
			iterEvent.Outputs = outputs
		}
	}
	// Try to extract Steps field
	if val, ok := getFieldValue(event.Data, "Steps"); ok {
		if steps, ok := val.(int); ok {
			iterEvent.Steps = steps
		}
	}
	// Try to extract Metadata field
	if val, ok := getFieldValue(event.Data, "Metadata"); ok {
		if metadata, ok := val.(map[string]any); ok {
			iterEvent.Metadata = metadata
		}
	}
	// Try to extract Error field if not already set
	if errMsg == "" {
		if val, ok := getFieldValue(event.Data, "Error"); ok {
			if errStr, ok := val.(string); ok {
				iterEvent.Error = errStr
			}
		}
	}

	e.iterationEventCallback(iterEvent)
}

// handleLoopStartedEvent handles loop started events with struct data
func (e *WorkflowEngine) handleLoopStartedEvent(nodeID string, event *shared.NodeEventCh) {
	if e.loopEventCallback == nil {
		return
	}
	loopEvent := &LoopEvent{
		Type:      "started",
		NodeID:    nodeID,
		Timestamp: event.Timestamp,
		StartedAt: eventStartedAtFromData(event.Data, event.Timestamp),
	}

	if val, ok := getFieldValue(event.Data, "Inputs"); ok {
		if inputs, ok := val.(map[string]any); ok {
			loopEvent.Inputs = inputs
		}
	}
	if val, ok := getFieldValue(event.Data, "Metadata"); ok {
		if metadata, ok := val.(map[string]any); ok {
			loopEvent.Metadata = metadata
		}
	}

	e.loopEventCallback(loopEvent)
}

// handleLoopNextEvent handles loop next events with struct data
func (e *WorkflowEngine) handleLoopNextEvent(nodeID string, event *shared.NodeEventCh) {
	if e.loopEventCallback == nil {
		return
	}
	loopEvent := &LoopEvent{
		Type:      "next",
		NodeID:    nodeID,
		Timestamp: event.Timestamp,
	}

	if val, ok := getFieldValue(event.Data, "Index"); ok {
		if index, ok := val.(int); ok {
			loopEvent.Index = index
		}
	}
	if val, ok := getFieldValue(event.Data, "PreLoopOutput"); ok {
		if preLoopOutput, ok := val.(map[string]any); ok {
			loopEvent.PreLoopOutput = preLoopOutput
		}
	}

	e.loopEventCallback(loopEvent)
}

// handleLoopCompletedEvent handles loop completed/failed events
func (e *WorkflowEngine) handleLoopCompletedEvent(nodeID string, event *shared.NodeEventCh, eventType string, errMsg string) {
	if e.loopEventCallback == nil {
		return
	}
	loopEvent := &LoopEvent{
		Type:      eventType,
		NodeID:    nodeID,
		Error:     errMsg,
		Timestamp: event.Timestamp,
		StartedAt: eventStartedAtFromData(event.Data, time.Time{}),
	}

	if val, ok := getFieldValue(event.Data, "Inputs"); ok {
		if inputs, ok := val.(map[string]any); ok {
			loopEvent.Inputs = inputs
		}
	}
	if val, ok := getFieldValue(event.Data, "Outputs"); ok {
		if outputs, ok := val.(map[string]any); ok {
			loopEvent.Outputs = outputs
		}
	}
	if val, ok := getFieldValue(event.Data, "Steps"); ok {
		if steps, ok := val.(int); ok {
			loopEvent.Steps = steps
		}
	}
	if val, ok := getFieldValue(event.Data, "Metadata"); ok {
		if metadata, ok := val.(map[string]any); ok {
			loopEvent.Metadata = metadata
		}
	}
	if errMsg == "" {
		if val, ok := getFieldValue(event.Data, "Error"); ok {
			if errStr, ok := val.(string); ok {
				loopEvent.Error = errStr
			}
		}
	}

	e.loopEventCallback(loopEvent)
}

// getFieldValue extracts a field value from a struct using reflection
func getFieldValue(data any, fieldName string) (any, bool) {
	if data == nil {
		return nil, false
	}

	v := reflect.ValueOf(data)
	if v.Kind() == reflect.Ptr {
		v = v.Elem()
	}
	if v.Kind() != reflect.Struct {
		return nil, false
	}

	field := v.FieldByName(fieldName)
	if !field.IsValid() {
		return nil, false
	}

	return field.Interface(), true
}

func nonZeroTime(value, fallback time.Time) time.Time {
	if !value.IsZero() {
		return value
	}
	return fallback
}

func eventStartedAtFromData(data any, fallback time.Time) time.Time {
	if value, ok := getFieldValue(data, "StartAt"); ok {
		if startedAt, ok := value.(time.Time); ok && !startedAt.IsZero() {
			return startedAt
		}
	}
	if mapped, ok := data.(map[string]any); ok {
		if startedAt, ok := mapped["start_at"].(time.Time); ok && !startedAt.IsZero() {
			return startedAt
		}
	}
	return fallback
}

// syncNodeStatesToRuntimeState syncs node states from engine's steps to runtimeState.NodeRunState.NodeStateMapping
