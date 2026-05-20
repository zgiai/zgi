package graph_engine

import (
	"errors"
	"fmt"
	"time"

	"github.com/zgiai/zgi/api/internal/modules/app/workflow/graph_engine/entities"
	"github.com/zgiai/zgi/api/internal/modules/app/workflow/shared"
)

// NodeExecutionSnapshot is an immutable view of a workflow node after execution.
type NodeExecutionSnapshot struct {
	NodeID           string
	NodeType         shared.NodeType
	Status           shared.WorkflowNodeExecutionStatus
	Error            string
	Inputs           map[string]interface{}
	ProcessData      map[string]interface{}
	Outputs          map[string]interface{}
	Metadata         map[string]interface{}
	StartTime        time.Time
	EndTime          time.Time
	EdgeSourceHandle string
}

func (e *WorkflowEngine) signalStatusChange() {
	e.statusChange.L.Lock()
	defer e.statusChange.L.Unlock()
	e.statusChange.Signal()
}

func (e *WorkflowEngine) getExecutionResult() error {
	if e.IsStopped() {
		return errors.New("workflow execution stopped by user")
	}

	var failedNodes []string
	var firstError error

	for nodeID, state := range e.steps {
		state.mu.RLock()
		if state.Status == shared.FAILED {
			failedNodes = append(failedNodes, nodeID)
			// Capture the first error for detailed error message
			if firstError == nil && state.Error != nil {
				firstError = state.Error
			}
		}
		state.mu.RUnlock()
	}

	if len(failedNodes) > 0 {
		if firstError != nil {
			return fmt.Errorf("workflow execution failed at node %v: %w", failedNodes, firstError)
		}
		return fmt.Errorf("workflow execution failed, failed nodes: %v", failedNodes)
	}

	return nil
}

// GetNodeStatus gets node status
func (e *WorkflowEngine) GetNodeStatus(nodeID string) (*NodeState, bool) {
	state, exists := e.steps[nodeID]
	return state, exists
}

// GetWorkflowStatus gets workflow status
func (e *WorkflowEngine) GetWorkflowStatus() map[string]interface{} {
	result := make(map[string]interface{})

	for nodeID, state := range e.steps {
		state.mu.RLock()
		errMsg := ""
		if state.Error != nil {
			errMsg = state.Error.Error()
		}
		result[nodeID] = map[string]interface{}{
			"status":    state.Status,
			"error":     errMsg,
			"startTime": state.StartTime,
			"endTime":   state.EndTime,
		}
		state.mu.RUnlock()
	}

	return result
}

// GetNodeExecutionSnapshots returns node execution state captured after a workflow run.
func (e *WorkflowEngine) GetNodeExecutionSnapshots() []NodeExecutionSnapshot {
	if e == nil {
		return nil
	}

	snapshots := make([]NodeExecutionSnapshot, 0, len(e.steps))
	for nodeID, state := range e.steps {
		state.mu.RLock()
		snapshot := NodeExecutionSnapshot{
			NodeID:           nodeID,
			NodeType:         state.NodeType,
			Status:           state.Status,
			Inputs:           copyStringMap(state.Inputs),
			ProcessData:      copyStringMap(state.ProcessData),
			Outputs:          copyStringMap(state.Outputs),
			Metadata:         copyMetadataMap(state.Metadata),
			StartTime:        state.StartTime,
			EndTime:          state.EndTime,
			EdgeSourceHandle: state.EdgeSourceHandle,
		}
		if state.Error != nil {
			snapshot.Error = state.Error.Error()
		}
		state.mu.RUnlock()
		snapshots = append(snapshots, snapshot)
	}
	return snapshots
}

func copyStringMap(values map[string]interface{}) map[string]interface{} {
	if values == nil {
		return nil
	}
	result := make(map[string]interface{}, len(values))
	for key, value := range values {
		result[key] = value
	}
	return result
}

func copyMetadataMap(values map[shared.WorkflowNodeExecutionMetadataKey]any) map[string]interface{} {
	if values == nil {
		return nil
	}
	result := make(map[string]interface{}, len(values))
	for key, value := range values {
		result[string(key)] = value
	}
	return result
}

func (e *WorkflowEngine) syncNodeStatesToRuntimeState() {
	if e.runtimeState == nil || e.runtimeState.NodeRunState == nil {
		return
	}

	for nodeID, state := range e.steps {
		state.mu.RLock()
		// Convert WorkflowNodeExecutionStatus to RouteNodeStatus
		var routeStatus shared.RouteNodeStatus
		switch state.Status {
		case shared.SUCCEEDED:
			routeStatus = shared.RouteNodeStatusSuccess
		case shared.FAILED:
			routeStatus = shared.RouteNodeStatusFailed
		case shared.EXCEPTION:
			routeStatus = shared.RouteNodeStatusException
		case shared.SKIPPED:
			routeStatus = shared.RouteNodeStatusSuccess
		case shared.PAUSED:
			routeStatus = shared.RouteNodeStatusPaused
		default:
			routeStatus = shared.RouteNodeStatusRunning
		}

		// Create RouteNodeState
		routeNodeState := &entities.RouteNodeState{
			ID:      nodeID, // Use nodeID as ID for simplicity
			NodeID:  nodeID,
			Status:  routeStatus,
			StartAt: state.StartTime,
			Index:   0, // Index will be set based on iteration
			NodeRunResult: &shared.NodeRunResult{
				Status:  state.Status,
				Inputs:  state.Inputs,
				Outputs: state.Outputs,
			},
		}
		if !state.EndTime.IsZero() {
			finishedAt := state.EndTime
			routeNodeState.FinishedAt = &finishedAt
		}

		if state.Error != nil {
			errMsg := state.Error.Error()
			routeNodeState.FailedReason = &errMsg
			routeNodeState.NodeRunResult.ErrMsg = errMsg
		}

		state.mu.RUnlock()
		e.runtimeState.UpsertRouteNodeState(nodeID, routeNodeState)
	}
}
