package graph_engine

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"time"

	"github.com/zgiai/zgi/api/internal/modules/app/workflow/shared"
	"github.com/zgiai/zgi/api/internal/observability"
	"github.com/zgiai/zgi/api/pkg/logger"
	"go.opentelemetry.io/otel/attribute"
)

func (e *WorkflowEngine) Execute(ctx context.Context) (err error) {
	logger.Info("Starting workflow execution with %d nodes", len(e.steps))
	ctx = observability.WithLangfuseTraceAttributes(ctx, e.langfuseTraceAttributes()...)
	ctx, span := observability.StartWorkflowRunSpan(ctx, e.workflowRunSpanAttributes()...)
	defer func() {
		span.SetAttributes(attribute.String("zgi.workflow_status", workflowErrorStatus(err)))
		observability.EndSpan(span, err)
	}()

	if !e.tryLock() {
		return errors.New("workflow is already running")
	}
	defer e.unlock()

	if len(e.steps) == 0 {
		logger.Info("No nodes to execute, workflow completed")
		return nil
	}

	logger.Info("Resetting all node states")
	e.reset()

	e.statusMu.Lock()
	e.isStopped = false
	e.isPaused = false
	e.pausedNodeIDs = nil
	e.pausedNodeSet = make(map[string]struct{})
	e.statusMu.Unlock()

	logger.Info("Running preflight checks")
	if err := e.preflight(); err != nil {
		logger.Error("Preflight check failed: %v", err)
		return fmt.Errorf("preflight check failed: %w", err)
	}
	logger.Info("Preflight checks passed")

	logger.Info("Starting execution loop")
	e.statusChange.L.Lock()
	for {
		done, continueImmediately := e.tick(ctx)
		if done {
			logger.Info("Execution loop completed")
			break
		}
		if continueImmediately {
			// State changed within tick (e.g. node skipped), continue immediately
			continue
		}
		e.statusChange.Wait()
	}
	e.statusChange.L.Unlock()

	logger.Info("Waiting for all goroutines to complete")
	e.waitGroup.Wait()
	logger.Info("All goroutines completed")

	// Sync node states to runtimeState.NodeRunState for subgraph event emission
	e.syncNodeStatesToRuntimeState()

	result := e.getExecutionResult()
	if result != nil {
		logger.Error("Workflow execution failed: %v", result)
	} else {
		logger.Info("Workflow execution completed successfully")
	}
	return result
}

func workflowErrorStatus(err error) string {
	if err != nil {
		return "failed"
	}
	return "succeeded"
}

// tryLock attempts to lock the workflow
func (e *WorkflowEngine) tryLock() bool {
	e.statusMu.Lock()
	defer e.statusMu.Unlock()

	if e.isRunning {
		return false
	}

	e.isRunning = true
	return true
}

// unlock unlocks the workflow
func (e *WorkflowEngine) unlock() {
	e.statusMu.Lock()
	defer e.statusMu.Unlock()
	e.isRunning = false
}

// reset resets all node states
func (e *WorkflowEngine) reset() {
	for _, state := range e.steps {
		state.mu.Lock()
		state.Status = shared.PENDING
		state.Error = nil
		state.StartTime = time.Time{}
		state.EndTime = time.Time{}
		state.RetryCount = 0
		state.mu.Unlock()
	}
}

func (e *WorkflowEngine) ClearNodes() {
	e.statusMu.Lock()
	defer e.statusMu.Unlock()

	e.steps = make(map[string]*NodeState)
	logger.Info("Cleared all nodes from workflow engine")
}

func (e *WorkflowEngine) Stop() {
	e.statusMu.Lock()
	defer e.statusMu.Unlock()

	if e.isRunning {
		e.isStopped = true
		logger.Info("Workflow engine stop requested")
		e.statusChange.Signal()
	}
}

// IsStopped checks if the workflow is stopped
func (e *WorkflowEngine) IsStopped() bool {
	e.statusMu.RLock()
	defer e.statusMu.RUnlock()
	return e.isStopped
}

func (e *WorkflowEngine) markPaused(nodeID string) {
	e.statusMu.Lock()
	defer e.statusMu.Unlock()

	e.isPaused = true
	if e.pausedNodeSet == nil {
		e.pausedNodeSet = make(map[string]struct{})
	}
	if _, exists := e.pausedNodeSet[nodeID]; !exists {
		e.pausedNodeSet[nodeID] = struct{}{}
		e.pausedNodeIDs = append(e.pausedNodeIDs, nodeID)
	}
	e.statusChange.Signal()
}

func (e *WorkflowEngine) IsPaused() bool {
	e.statusMu.RLock()
	defer e.statusMu.RUnlock()
	return e.isPaused
}

func (e *WorkflowEngine) PausedNodeID() string {
	pausedNodeIDs := e.PausedNodeIDs()
	if len(pausedNodeIDs) == 0 {
		return ""
	}
	return pausedNodeIDs[0]
}

func (e *WorkflowEngine) PausedNodeIDs() []string {
	e.statusMu.RLock()
	defer e.statusMu.RUnlock()
	pausedNodeIDs := append([]string(nil), e.pausedNodeIDs...)
	sort.Strings(pausedNodeIDs)
	return pausedNodeIDs
}

func (e *WorkflowEngine) preflight() error {
	visited := make(map[string]bool)
	recStack := make(map[string]bool)

	for nodeID := range e.steps {
		if !visited[nodeID] {
			if e.hasCycle(nodeID, visited, recStack) {
				return fmt.Errorf("circular dependency detected")
			}
		}
	}

	return nil
}

// hasCycle checks for circular dependencies
func (e *WorkflowEngine) hasCycle(nodeID string, visited, recStack map[string]bool) bool {
	visited[nodeID] = true
	recStack[nodeID] = true

	state := e.steps[nodeID]
	for upstream := range state.Upstreams {
		if !visited[upstream] {
			if e.hasCycle(upstream, visited, recStack) {
				return true
			}
		} else if recStack[upstream] {
			return true
		}
	}

	recStack[nodeID] = false
	return false
}

func (e *WorkflowEngine) tick(ctx context.Context) (bool, bool) {

	if e.IsStopped() {
		logger.Info("Workflow execution stopped by user")
		return true, false
	}

	if e.IsPaused() {
		logger.Info("Workflow execution paused")
		return true, false
	}

	select {
	case <-ctx.Done():
		logger.Info("Workflow execution cancelled by context: %v", ctx.Err())
		return true, false
	default:
	}

	if e.isTerminated() {
		logger.Info("All nodes terminated, workflow completed")
		return true, false
	}

	pendingNodes := 0
	runningNodes := 0
	completedNodes := 0
	failedNodes := 0
	readyBatch := make([]scheduledNode, 0)

	forceTick := false
	for nodeID, state := range e.steps {
		if e.IsPaused() {
			logger.Info("Workflow execution paused while scheduling")
			return true, false
		}

		state.mu.RLock()
		status := state.Status
		state.mu.RUnlock()

		if status == shared.RUNNING {
			runningNodes++
		} else if status == shared.SUCCEEDED {
			completedNodes++
		} else if status == shared.FAILED || status == shared.EXCEPTION {
			failedNodes++
		}

		if status != shared.PENDING {
			continue
		}

		pendingNodes++

		if e.IsPaused() {
			logger.Info("Workflow execution paused before scheduling pending node %s", nodeID)
			return true, false
		}

		if !e.allUpstreamsTerminated(nodeID) {
			continue
		}

		if e.IsPaused() {
			logger.Info("Workflow execution paused before scheduling ready node %s", nodeID)
			return true, false
		}

		// All upstreams are terminated. If no upstream edge is active, this node must be skipped.
		// This propagates branch skipping across unconditional ("source") edges.
		if len(state.Upstreams) > 0 && !e.hasAnyActiveUpstreamEdge(nodeID) {
			nodeType := string(state.NodeType)
			state.mu.Lock()
			state.Status = shared.SKIPPED
			state.Outputs = make(map[string]interface{})
			state.EndTime = time.Now()
			state.mu.Unlock()
			logger.Info("Node %s skipped because no upstream branch is active", nodeID)
			if e.onNodeSkipped != nil {
				e.onNodeSkipped(nodeID, nodeType)
			}
			forceTick = true
			continue
		}

		if e.IsPaused() {
			logger.Info("Workflow execution paused before leasing node %s", nodeID)
			return true, false
		}

		if !e.lease() {
			continue
		}
		if e.IsPaused() {
			e.unlease()
			logger.Info("Workflow execution paused after leasing node %s", nodeID)
			return true, false
		}
		if !e.claimNodeForExecution(state) {
			e.unlease()
			continue
		}

		readyBatch = append(readyBatch, scheduledNode{nodeID: nodeID, state: state})
	}

	e.startReadyBatch(ctx, readyBatch)

	logger.Info("Tick summary - Pending: %d, Running: %d, Completed: %d, Failed: %d", pendingNodes, runningNodes, completedNodes, failedNodes)

	// If we forced a tick (e.g. by skipping a node), return false for done but true for continueImmediately
	if forceTick {
		return false, true
	}

	return false, false
}

type scheduledNode struct {
	nodeID string
	state  *NodeState
}

func (e *WorkflowEngine) startReadyBatch(ctx context.Context, batch []scheduledNode) {
	if len(batch) == 0 {
		return
	}
	sort.SliceStable(batch, func(i, j int) bool {
		leftOrder, leftOK := e.nodeOrder[batch[i].nodeID]
		rightOrder, rightOK := e.nodeOrder[batch[j].nodeID]
		if leftOK && rightOK && leftOrder != rightOrder {
			return leftOrder < rightOrder
		}
		if leftOK != rightOK {
			return leftOK
		}
		return batch[i].nodeID < batch[j].nodeID
	})

	nodeIDs := make([]string, 0, len(batch))
	for _, node := range batch {
		nodeIDs = append(nodeIDs, node.nodeID)
	}

	if e.IsPaused() {
		for _, node := range batch {
			node.state.mu.Lock()
			if node.state.Status == shared.RUNNING {
				node.state.Status = shared.PENDING
			}
			node.state.mu.Unlock()
			e.unlease()
		}
		logger.Info("Workflow execution paused before starting ready batch")
		return
	}

	if e.onReadyBatch != nil {
		e.onReadyBatch(e.readyScope, nodeIDs)
	}

	for _, node := range batch {
		logger.Info("Starting execution for node %s", node.nodeID)
		e.waitGroup.Add(1)
		go func(nodeID string, state *NodeState) {
			defer e.waitGroup.Done()
			defer e.unlease()

			e.executeNode(ctx, nodeID, state)
		}(node.nodeID, node.state)
	}
}

func (e *WorkflowEngine) claimNodeForExecution(state *NodeState) bool {
	state.mu.Lock()
	defer state.mu.Unlock()

	if state.Status != shared.PENDING {
		return false
	}
	state.Status = shared.RUNNING
	return true
}

// isTerminated checks if all nodes are completed
func (e *WorkflowEngine) isTerminated() bool {
	for _, state := range e.steps {
		state.mu.RLock()
		if !e.isStatusTerminated(state.Status) {
			state.mu.RUnlock()
			return false
		}
		state.mu.RUnlock()
	}
	return true
}

// isStatusTerminated checks if the status is terminated
func (e *WorkflowEngine) isStatusTerminated(status shared.WorkflowNodeExecutionStatus) bool {
	return status == shared.SUCCEEDED || status == shared.FAILED || status == shared.EXCEPTION || status == shared.SKIPPED || status == shared.PAUSED
}

// hasAnyActiveUpstreamEdge returns true if at least one upstream edge can activate the given node.
// Hard-failed or skipped upstreams never activate downstream nodes, even for default "source" edges.
func (e *WorkflowEngine) hasAnyActiveUpstreamEdge(nodeID string) bool {
	state := e.steps[nodeID]
	for upstreamID := range state.Upstreams {
		upstreamState := e.steps[upstreamID]
		upstreamState.mu.RLock()
		upstreamStatus := upstreamState.Status
		upstreamHandle := upstreamState.EdgeSourceHandle
		upstreamState.mu.RUnlock()

		if upstreamStatus == shared.SKIPPED || upstreamStatus == shared.FAILED || upstreamStatus == shared.PAUSED {
			continue
		}

		expectedHandles := state.UpstreamEdges[upstreamID]
		if len(expectedHandles) == 0 {
			return true
		}
		if _, ok := expectedHandles["source"]; ok {
			return true
		}
		if _, ok := expectedHandles[upstreamHandle]; ok {
			return true
		}
	}
	return false
}

// allUpstreamsTerminated checks if all upstream nodes have reached a terminal state.
// Branch routing is evaluated separately by hasAnyActiveUpstreamEdge after all
// upstreams are terminal. This allows merge nodes with multiple conditional
// upstreams to run when at least one upstream branch is active.
func (e *WorkflowEngine) allUpstreamsTerminated(nodeID string) bool {
	state := e.steps[nodeID]
	for upstreamID := range state.Upstreams {
		upstreamState := e.steps[upstreamID]
		upstreamState.mu.RLock()
		upstreamStatus := upstreamState.Status
		upstreamState.mu.RUnlock()

		if !e.isStatusTerminated(upstreamStatus) {
			return false
		}
	}
	return true
}

// lease acquires execution permission
func (e *WorkflowEngine) lease() bool {
	if e.leaseBucket == nil {
		return true
	}

	select {
	case e.leaseBucket <- struct{}{}:
		return true
	default:
		return false
	}
}

// unlease releases execution permission
func (e *WorkflowEngine) unlease() {
	if e.leaseBucket != nil {
		<-e.leaseBucket
	}
}
