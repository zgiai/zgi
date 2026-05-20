package graph_engine

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/zgiai/ginext/internal/modules/app/workflow/shared"
	"github.com/zgiai/ginext/internal/observability"
	"github.com/zgiai/ginext/pkg/logger"
	"go.opentelemetry.io/otel/attribute"
)

func (e *WorkflowEngine) executeNode(ctx context.Context, nodeID string, state *NodeState) {
	logger.Info("Starting node execution for nodeID: %s, nodeType: %s", nodeID, state.NodeType)

	if e.IsStopped() {
		logger.Info("Workflow stopped, skipping node execution: %s", nodeID)
		state.mu.Lock()
		state.Status = shared.FAILED
		state.Error = errors.New("workflow stopped by user")
		state.EndTime = time.Now()
		state.mu.Unlock()
		return
	}

	state.mu.Lock()
	state.Status = shared.RUNNING
	state.StartTime = time.Now()
	state.mu.Unlock()

	// Invoke onNodeStarted callback for real-time event streaming (used by iteration subgraphs)
	if e.onNodeStarted != nil {
		e.onNodeStarted(nodeID, string(state.NodeType), state.Inputs)
	}

	var err error
	var result *shared.NodeRunResult
	ctx = observability.WithLangfuseTraceAttributes(ctx, e.langfuseWorkflowNodeTraceAttributes(nodeID, state.NodeType)...)
	ctx, span := observability.StartWorkflowNodeSpan(ctx, e.workflowNodeSpanAttributes(nodeID, state.NodeType)...)
	defer func() {
		state.mu.RLock()
		spanErr := err
		if spanErr == nil {
			spanErr = state.Error
		}
		status := state.Status
		state.mu.RUnlock()

		span.SetAttributes(attribute.String("zgi.workflow_node_status", string(status)))
		observability.EndSpan(span, spanErr)
	}()

	defer func() {
		state.mu.Lock()
		state.EndTime = time.Now()
		executionDuration := state.EndTime.Sub(state.StartTime)

		if result != nil {
			state.Inputs = result.Inputs
			state.ProcessData = result.ProcessData
			state.Outputs = result.Outputs
			state.Metadata = result.Metadata
			state.EdgeSourceHandle = result.EdgeSourceHandle
		}

		if err != nil {
			if result != nil && result.Status != "" {
				state.Status = result.Status
			} else {
				state.Status = shared.FAILED
			}
			state.Error = err
			logger.Error(fmt.Sprintf("Node execution failed for nodeID: %s, nodeType: %s, duration: %v, config: %+v", nodeID, state.NodeType, executionDuration, state.Config), err)
		} else {
			if result != nil {
				state.Status = result.Status

				if e.runtimeState != nil && e.runtimeState.VariablePool != nil {
					for outputKey, outputValue := range result.Outputs {
						selector := []string{nodeID, outputKey}
						e.runtimeState.VariablePool.Add(selector, outputValue)
						logger.Info("Added node output to variable pool: nodeID=%s, key=%s, value=%v", nodeID, outputKey, outputValue)
					}
				}
				e.updateRuntimeOutputsForNode(state.NodeType, result.Outputs)

				switch result.Status {
				case shared.SUCCEEDED:
					logger.Info("Node execution succeeded for nodeID: %s, nodeType: %s, duration: %v", nodeID, state.NodeType, executionDuration)
				case shared.PAUSED:
					e.markPaused(nodeID)
					logger.Info("Node execution paused workflow for nodeID: %s, nodeType: %s, duration: %v", nodeID, state.NodeType, executionDuration)
				case shared.SKIPPED:
					logger.Info("Node execution skipped for nodeID: %s, nodeType: %s, duration: %v", nodeID, state.NodeType, executionDuration)
				default:
					// Node returned a failed status, preserve the original error when available.
					state.Error = errorFromNodeRunResult(result)
					logger.Info("Node execution completed with status %s for nodeID: %s, nodeType: %s, duration: %v, error: %v", result.Status, nodeID, state.NodeType, executionDuration, state.Error)
				}
			} else {
				state.Status = shared.SUCCEEDED
				logger.Info("Node execution succeeded for nodeID: %s, nodeType: %s, duration: %v", nodeID, state.NodeType, executionDuration)
			}
		}
		state.mu.Unlock()

		// Invoke onNodeFinished callback for real-time event streaming (used by iteration subgraphs)
		if e.onNodeFinished != nil {
			var errToReport error
			if state.Error != nil {
				errToReport = state.Error
			}
			e.onNodeFinished(nodeID, string(state.NodeType), string(state.Status), state.Outputs, state.EdgeSourceHandle, errToReport)
		}
		if e.onNodeFinishedDetailed != nil {
			var errToReport error
			if state.Error != nil {
				errToReport = state.Error
			}
			e.onNodeFinishedDetailed(NodeFinishedEvent{
				NodeID:           nodeID,
				NodeType:         string(state.NodeType),
				Status:           string(state.Status),
				Outputs:          state.Outputs,
				Err:              errToReport,
				EdgeSourceHandle: state.EdgeSourceHandle,
			})
		}

		e.signalStatusChange()
	}()

	if e.nodeRunner == nil {
		err = fmt.Errorf("node runner is required")
		logger.Error(fmt.Sprintf("Node runner missing for nodeID: %s", nodeID), err)
		return
	}

	logger.Info("Starting node execution for nodeID: %s", nodeID)
	eventChan := make(chan *shared.NodeEventCh, 10)
	req := NodeRunRequest{
		NodeID:          nodeID,
		NodeType:        state.NodeType,
		Config:          state.Config,
		GraphInitParams: e.getGraphInitParams(),
		Graph:           e.graph,
		RuntimeState:    e.runtimeState,
	}
	var nodeResult *shared.NodeRunResult
	var nodeErr error
	go func() {
		defer close(eventChan)
		logger.Info("Running node runner for nodeID: %s", nodeID)
		runnerResult, runnerErr := e.nodeRunner.RunNode(ctx, req, eventChan)
		if runnerErr != nil {
			logger.Error(fmt.Sprintf("Node.Run returned error for nodeID: %s, error: %v", nodeID, runnerErr), runnerErr)
		} else {
			logger.Info("Node runner completed without error for nodeID: %s", nodeID)
		}
		nodeResult = runnerResult
		nodeErr = runnerErr
	}()

	if !e.consumeNodeEvents(nodeID, state, eventChan, &result, &err) {
		return
	}
	if result == nil && nodeResult != nil {
		result = nodeResult
	}
	if nodeErr != nil {
		err = nodeErr
	}
}
