package graph_engine

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/zgiai/ginext/internal/modules/app/workflow/graph_engine/entities"
	"github.com/zgiai/ginext/internal/modules/app/workflow/shared"
)

type blockingNodeRunner struct {
	started chan string
	release chan struct{}
}

func (r *blockingNodeRunner) RunNode(ctx context.Context, req NodeRunRequest, eventChan chan<- *shared.NodeEventCh) (*shared.NodeRunResult, error) {
	select {
	case r.started <- req.NodeID:
	case <-ctx.Done():
		return nil, ctx.Err()
	}

	select {
	case <-r.release:
	case <-ctx.Done():
		return nil, ctx.Err()
	}

	return &shared.NodeRunResult{
		Status:  shared.SUCCEEDED,
		Inputs:  map[string]any{},
		Outputs: map[string]any{"result": req.NodeID},
	}, nil
}

type fastNodeRunner struct {
	mu     sync.Mutex
	counts map[string]int
}

func (r *fastNodeRunner) RunNode(ctx context.Context, req NodeRunRequest, eventChan chan<- *shared.NodeEventCh) (*shared.NodeRunResult, error) {
	if r.counts != nil {
		r.mu.Lock()
		r.counts[req.NodeID]++
		r.mu.Unlock()
	}

	return &shared.NodeRunResult{
		Status:  shared.SUCCEEDED,
		Inputs:  map[string]any{},
		Outputs: map[string]any{"result": req.NodeID},
	}, nil
}

type fixedStatusNodeRunner struct {
	mu       sync.Mutex
	counts   map[string]int
	statuses map[string]shared.WorkflowNodeExecutionStatus
}

func (r *fixedStatusNodeRunner) RunNode(ctx context.Context, req NodeRunRequest, eventChan chan<- *shared.NodeEventCh) (*shared.NodeRunResult, error) {
	r.mu.Lock()
	if r.counts == nil {
		r.counts = make(map[string]int)
	}
	r.counts[req.NodeID]++
	status := r.statuses[req.NodeID]
	r.mu.Unlock()

	if status == "" {
		status = shared.SUCCEEDED
	}
	return &shared.NodeRunResult{
		Status:  status,
		Inputs:  map[string]any{},
		Outputs: map[string]any{"result": req.NodeID},
	}, nil
}

type coordinatedStatusNodeRunner struct {
	mu        sync.Mutex
	counts    map[string]int
	statuses  map[string]shared.WorkflowNodeExecutionStatus
	waitNodes map[string]bool
	started   chan string
	release   chan struct{}
	releases  map[string]chan struct{}
}

func (r *coordinatedStatusNodeRunner) RunNode(ctx context.Context, req NodeRunRequest, eventChan chan<- *shared.NodeEventCh) (*shared.NodeRunResult, error) {
	r.mu.Lock()
	if r.counts == nil {
		r.counts = make(map[string]int)
	}
	r.counts[req.NodeID]++
	status := r.statuses[req.NodeID]
	wait := r.waitNodes[req.NodeID]
	r.mu.Unlock()

	if wait {
		select {
		case r.started <- req.NodeID:
		case <-ctx.Done():
			return nil, ctx.Err()
		}

		release := r.release
		if r.releases != nil && r.releases[req.NodeID] != nil {
			release = r.releases[req.NodeID]
		}
		if release != nil {
			select {
			case <-release:
			case <-ctx.Done():
				return nil, ctx.Err()
			}
		}
	}

	if status == "" {
		status = shared.SUCCEEDED
	}
	return &shared.NodeRunResult{
		Status:  status,
		Inputs:  map[string]any{},
		Outputs: map[string]any{"result": req.NodeID},
	}, nil
}

func TestGetGraphInitParams_IncludesOrganizationID(t *testing.T) {
	variablePool := entities.NewVariablePool()
	variablePool.SystemVariables.WorkspaceID = "ws-1"
	variablePool.SystemVariables.OrganizationID = "org-1"
	variablePool.SystemVariables.BillingSubjectType = "organization"
	variablePool.SystemVariables.AppID = "app-1"
	variablePool.SystemVariables.WorkflowID = "wf-1"
	variablePool.SystemVariables.UserID = "user-1"
	variablePool.SystemVariables.WorkflowType = "workflow"

	engine := &WorkflowEngine{
		runtimeState: entities.NewGraphRuntimeState(variablePool),
	}

	params := engine.getGraphInitParams()
	if params.WorkspaceID != "ws-1" {
		t.Fatalf("workspaceID = %q, want %q", params.WorkspaceID, "ws-1")
	}
	if params.OrganizationID != "org-1" {
		t.Fatalf("organizationID = %q, want %q", params.OrganizationID, "org-1")
	}
	if params.BillingSubjectType != "organization" {
		t.Fatalf("billingSubjectType = %q, want %q", params.BillingSubjectType, "organization")
	}
}

func TestTickSkipsDownstreamWhenUpstreamFailed(t *testing.T) {
	engine := NewWorkflowEngine(1)
	engine.AddNode("failed", shared.Answer, map[string]any{"id": "failed"})
	engine.AddNode("downstream", shared.Answer, map[string]any{"id": "downstream"})

	if err := engine.AddDependencyWithHandle("failed", "downstream", "source"); err != nil {
		t.Fatalf("AddDependencyWithHandle() error = %v", err)
	}

	engine.steps["failed"].Status = shared.FAILED

	done, continueImmediately := engine.tick(context.Background())
	if done {
		t.Fatalf("tick() done = true, want false")
	}
	if !continueImmediately {
		t.Fatalf("tick() continueImmediately = false, want true")
	}
	if got := engine.steps["downstream"].Status; got != shared.SKIPPED {
		t.Fatalf("downstream status = %q, want %q", got, shared.SKIPPED)
	}
}

func TestHasAnyActiveUpstreamEdge_AllowsExceptionFailBranch(t *testing.T) {
	engine := NewWorkflowEngine(1)
	engine.AddNode("upstream", shared.HTTPRequest, map[string]any{"id": "upstream"})
	engine.AddNode("recovery", shared.Answer, map[string]any{"id": "recovery"})

	if err := engine.AddDependencyWithHandle("upstream", "recovery", string(shared.FailedBranch)); err != nil {
		t.Fatalf("AddDependencyWithHandle() error = %v", err)
	}

	engine.steps["upstream"].Status = shared.EXCEPTION
	engine.steps["upstream"].EdgeSourceHandle = string(shared.FailedBranch)

	if !engine.hasAnyActiveUpstreamEdge("recovery") {
		t.Fatalf("hasAnyActiveUpstreamEdge() = false, want true")
	}
}

func TestUpdateRuntimeOutputsForNode_ResponseNodesMergeIntoRuntimeState(t *testing.T) {
	engine := NewWorkflowEngine(1)
	engine.runtimeState = entities.NewGraphRuntimeState(entities.NewVariablePool())

	engine.updateRuntimeOutputsForNode(shared.Answer, map[string]any{
		"answer": "hello",
		"text":   "ignored-text",
	})
	engine.updateRuntimeOutputsForNode(shared.End, map[string]any{
		"answer": " world",
		"result": 42,
	})

	outputs := engine.runtimeState.OutputsSnapshot()
	if got := outputs["answer"]; got != "hello world" {
		t.Fatalf("runtimeState.Outputs[answer] = %#v, want %#v", got, "hello world")
	}
	if got := outputs["result"]; got != 42 {
		t.Fatalf("runtimeState.Outputs[result] = %#v, want %#v", got, 42)
	}
}

func TestUpdateRuntimeOutputsForNode_IgnoresNonResponseNodes(t *testing.T) {
	engine := NewWorkflowEngine(1)
	engine.runtimeState = entities.NewGraphRuntimeState(entities.NewVariablePool())

	engine.updateRuntimeOutputsForNode(shared.LLM, map[string]any{
		"answer": "should-not-become-final-output",
	})

	if outputs := engine.runtimeState.OutputsSnapshot(); len(outputs) != 0 {
		t.Fatalf("runtimeState.Outputs = %#v, want empty", outputs)
	}
}

func TestExecuteRunsReadyNodesInParallel(t *testing.T) {
	runner := &blockingNodeRunner{
		started: make(chan string, 2),
		release: make(chan struct{}),
	}
	engine := NewWorkflowEngine(2)
	engine.SetNodeRunner(runner)
	engine.SetRuntimeState(entities.NewGraphRuntimeState(entities.NewVariablePool()), &entities.Graph{Config: map[string]any{}})
	engine.AddNode("a", shared.Answer, map[string]any{"id": "a"})
	engine.AddNode("b", shared.Answer, map[string]any{"id": "b"})

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	done := make(chan error, 1)
	go func() {
		done <- engine.Execute(ctx)
	}()

	started := map[string]bool{}
	for len(started) < 2 {
		select {
		case nodeID := <-runner.started:
			started[nodeID] = true
		case <-time.After(time.Second):
			t.Fatalf("timed out waiting for both ready nodes to start, got %v", started)
		}
	}

	close(runner.release)

	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("Execute() error = %v", err)
		}
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for workflow completion")
	}
}

func TestExecuteReportsReadyBatchInGraphOrder(t *testing.T) {
	runner := &blockingNodeRunner{
		started: make(chan string, 2),
		release: make(chan struct{}),
	}
	engine := NewWorkflowEngine(2)
	engine.SetNodeRunner(runner)
	engine.SetRuntimeState(entities.NewGraphRuntimeState(entities.NewVariablePool()), &entities.Graph{
		Config: map[string]any{
			"nodes": []interface{}{
				map[string]interface{}{"id": "b"},
				map[string]interface{}{"id": "a"},
			},
		},
	})
	engine.AddNode("a", shared.Answer, map[string]any{"id": "a"})
	engine.AddNode("b", shared.Answer, map[string]any{"id": "b"})

	batches := make(chan []string, 1)
	engine.SetReadyBatchCallback(ReadyBatchScope{Kind: ReadyBatchScopeTop}, func(scope ReadyBatchScope, nodeIDs []string) {
		batches <- append([]string(nil), nodeIDs...)
	})

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	done := make(chan error, 1)
	go func() {
		done <- engine.Execute(ctx)
	}()

	var batch []string
	select {
	case batch = <-batches:
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for ready batch")
	}
	if strings.Join(batch, ",") != "b,a" {
		t.Fatalf("ready batch = %#v, want [b a]", batch)
	}

	started := map[string]bool{}
	for len(started) < 2 {
		select {
		case nodeID := <-runner.started:
			started[nodeID] = true
		case <-time.After(time.Second):
			t.Fatalf("timed out waiting for both ready nodes to start, got %v", started)
		}
	}

	close(runner.release)
	if err := <-done; err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
}

func TestExecutePausesWorkflowWithoutRunningDownstream(t *testing.T) {
	runner := &fixedStatusNodeRunner{
		statuses: map[string]shared.WorkflowNodeExecutionStatus{
			"approval": shared.PAUSED,
		},
	}
	engine := NewWorkflowEngine(2)
	engine.SetNodeRunner(runner)
	engine.SetRuntimeState(entities.NewGraphRuntimeState(entities.NewVariablePool()), &entities.Graph{Config: map[string]any{}})
	engine.AddNode("start", shared.Start, map[string]any{"id": "start"})
	engine.AddNode("approval", shared.Approval, map[string]any{"id": "approval"})
	engine.AddNode("after", shared.Answer, map[string]any{"id": "after"})

	if err := engine.AddDependency("start", "approval"); err != nil {
		t.Fatalf("AddDependency(start, approval) error = %v", err)
	}
	if err := engine.AddDependency("approval", "after"); err != nil {
		t.Fatalf("AddDependency(approval, after) error = %v", err)
	}

	if err := engine.Execute(context.Background()); err != nil {
		t.Fatalf("Execute() error = %v", err)
	}

	if !engine.IsPaused() {
		t.Fatal("engine.IsPaused() = false, want true")
	}
	if got := engine.PausedNodeID(); got != "approval" {
		t.Fatalf("PausedNodeID() = %q, want %q", got, "approval")
	}
	if got := engine.steps["approval"].Status; got != shared.PAUSED {
		t.Fatalf("approval status = %q, want %q", got, shared.PAUSED)
	}
	if got := engine.steps["approval"].Error; got != nil {
		t.Fatalf("approval error = %v, want nil", got)
	}
	if got := engine.steps["after"].Status; got != shared.PENDING {
		t.Fatalf("after status = %q, want %q", got, shared.PENDING)
	}

	runner.mu.Lock()
	afterRuns := runner.counts["after"]
	runner.mu.Unlock()
	if afterRuns != 0 {
		t.Fatalf("after node ran %d times, want 0", afterRuns)
	}
}

func TestExecuteCollectsMultiplePausedNodesAfterRunningNodesFinish(t *testing.T) {
	approvalRelease := make(chan struct{})
	sideRelease := make(chan struct{})
	runner := &coordinatedStatusNodeRunner{
		statuses: map[string]shared.WorkflowNodeExecutionStatus{
			"approval-a": shared.PAUSED,
			"approval-b": shared.PAUSED,
			"side":       shared.SUCCEEDED,
		},
		waitNodes: map[string]bool{
			"approval-a": true,
			"approval-b": true,
			"side":       true,
		},
		started: make(chan string, 3),
		releases: map[string]chan struct{}{
			"approval-a": approvalRelease,
			"approval-b": approvalRelease,
			"side":       sideRelease,
		},
	}
	engine := NewWorkflowEngine(3)
	engine.SetNodeRunner(runner)
	engine.SetRuntimeState(entities.NewGraphRuntimeState(entities.NewVariablePool()), &entities.Graph{Config: map[string]any{}})
	engine.AddNode("start", shared.Start, map[string]any{"id": "start"})
	engine.AddNode("approval-a", shared.Approval, map[string]any{"id": "approval-a"})
	engine.AddNode("approval-b", shared.Approval, map[string]any{"id": "approval-b"})
	engine.AddNode("side", shared.Answer, map[string]any{"id": "side"})
	engine.AddNode("after-a", shared.Answer, map[string]any{"id": "after-a"})
	engine.AddNode("after-b", shared.Answer, map[string]any{"id": "after-b"})
	engine.AddNode("after-side", shared.Answer, map[string]any{"id": "after-side"})

	for _, edge := range [][2]string{
		{"start", "approval-a"},
		{"start", "approval-b"},
		{"start", "side"},
		{"approval-a", "after-a"},
		{"approval-b", "after-b"},
		{"side", "after-side"},
	} {
		if err := engine.AddDependency(edge[0], edge[1]); err != nil {
			t.Fatalf("AddDependency(%s, %s) error = %v", edge[0], edge[1], err)
		}
	}

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	done := make(chan error, 1)
	go func() {
		done <- engine.Execute(ctx)
	}()

	started := map[string]bool{}
	for len(started) < 3 {
		select {
		case nodeID := <-runner.started:
			started[nodeID] = true
		case <-time.After(time.Second):
			t.Fatalf("timed out waiting for parallel branches to start, got %#v", started)
		}
	}
	close(approvalRelease)
	deadline := time.After(time.Second)
	for len(engine.PausedNodeIDs()) < 2 {
		select {
		case <-deadline:
			t.Fatalf("timed out waiting for approval nodes to pause, got %#v", engine.PausedNodeIDs())
		case <-time.After(10 * time.Millisecond):
		}
	}
	close(sideRelease)

	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("Execute() error = %v", err)
		}
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for workflow completion")
	}

	if !engine.IsPaused() {
		t.Fatal("engine.IsPaused() = false, want true")
	}
	pausedIDs := engine.PausedNodeIDs()
	if len(pausedIDs) != 2 || pausedIDs[0] != "approval-a" || pausedIDs[1] != "approval-b" {
		t.Fatalf("PausedNodeIDs() = %#v, want [approval-a approval-b]", pausedIDs)
	}
	if got := engine.steps["side"].Status; got != shared.SUCCEEDED {
		t.Fatalf("side status = %q, want %q", got, shared.SUCCEEDED)
	}
	for _, nodeID := range []string{"after-a", "after-b", "after-side"} {
		if got := engine.steps[nodeID].Status; got != shared.PENDING {
			t.Fatalf("%s status = %q, want %q", nodeID, got, shared.PENDING)
		}
	}
}

func TestExecuteHonorsSerialConcurrency(t *testing.T) {
	runner := &blockingNodeRunner{
		started: make(chan string, 2),
		release: make(chan struct{}),
	}
	engine := NewWorkflowEngine(1)
	engine.SetNodeRunner(runner)
	engine.SetRuntimeState(entities.NewGraphRuntimeState(entities.NewVariablePool()), &entities.Graph{Config: map[string]any{}})
	engine.AddNode("a", shared.Answer, map[string]any{"id": "a"})
	engine.AddNode("b", shared.Answer, map[string]any{"id": "b"})

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	done := make(chan error, 1)
	go func() {
		done <- engine.Execute(ctx)
	}()

	var firstNode string
	select {
	case firstNode = <-runner.started:
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for first ready node to start")
	}

	select {
	case secondNode := <-runner.started:
		t.Fatalf("node %s started while node %s was still running with maxConcurrency=1", secondNode, firstNode)
	case <-time.After(100 * time.Millisecond):
	}

	close(runner.release)

	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("Execute() error = %v", err)
		}
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for workflow completion")
	}
}

func TestExecuteParallelNodesWriteVariablePoolSafely(t *testing.T) {
	engine := NewWorkflowEngine(8)
	runner := &fastNodeRunner{counts: make(map[string]int)}
	engine.SetNodeRunner(runner)
	state := entities.NewGraphRuntimeState(entities.NewVariablePool())
	engine.SetRuntimeState(state, &entities.Graph{Config: map[string]any{}})

	const nodeCount = 32
	for i := 0; i < nodeCount; i++ {
		nodeID := fmt.Sprintf("node-%02d", i)
		engine.AddNode(nodeID, shared.Answer, map[string]any{"id": nodeID})
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := engine.Execute(ctx); err != nil {
		t.Fatalf("Execute() error = %v", err)
	}

	var wg sync.WaitGroup
	for i := 0; i < nodeCount; i++ {
		nodeID := fmt.Sprintf("node-%02d", i)
		wg.Add(1)
		go func() {
			defer wg.Done()
			if variable := state.VariablePool.Get([]string{nodeID, "result"}); variable == nil {
				t.Errorf("missing variable for %s", nodeID)
			}

			runner.mu.Lock()
			count := runner.counts[nodeID]
			runner.mu.Unlock()
			if count != 1 {
				t.Errorf("node %s ran %d times, want 1", nodeID, count)
			}
		}()
	}
	wg.Wait()
}
