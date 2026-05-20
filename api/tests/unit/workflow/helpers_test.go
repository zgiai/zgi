package workflow_test

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/zgiai/ginext/internal/modules/app/workflow/graph_engine"
	"github.com/zgiai/ginext/internal/modules/app/workflow/graph_engine/entities"
	"github.com/zgiai/ginext/internal/modules/app/workflow/graph_engine/subgraph"
	workflowruntime "github.com/zgiai/ginext/internal/modules/app/workflow/runtime"
	"github.com/zgiai/ginext/internal/modules/app/workflow/shared"
)

func newGraphConfig(nodes []map[string]any, edges []map[string]any) map[string]any {
	result := make(map[string]any)

	nodeList := make([]interface{}, 0, len(nodes))
	for _, node := range nodes {
		nodeList = append(nodeList, node)
	}

	edgeList := make([]interface{}, 0, len(edges))
	for _, edge := range edges {
		edgeList = append(edgeList, edge)
	}

	result["nodes"] = nodeList
	result["edges"] = edgeList
	return result
}

func newInitParams(graphConfig map[string]any) entities.GraphInitParams {
	return entities.GraphInitParams{
		OrganizationID: "tenant-1",
		AppID:          "app-1",
		WorkflowType:   entities.WorkflowTypeWorkflow,
		WorkflowID:     "workflow-1",
		GraphConfig:    graphConfig,
		UserID:         "user-1",
		UserFrom:       entities.UserFromAccount,
		InvokeFrom:     entities.InvokeFromServiceAPI,
		CallDepth:      0,
	}
}

func runNode(t *testing.T, node shared.NodeInterface) ([]*shared.NodeEventCh, error) {
	t.Helper()

	eventChan := make(chan *shared.NodeEventCh, 128)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	done := make(chan error, 1)
	go func() {
		done <- node.Run(ctx, eventChan)
		close(eventChan)
	}()

	events := make([]*shared.NodeEventCh, 0)
	for event := range eventChan {
		events = append(events, event)
	}

	err := <-done
	return events, err
}

func findRunCompletedResult(t *testing.T, events []*shared.NodeEventCh) *shared.NodeRunResult {
	t.Helper()
	for _, event := range events {
		if event.Type != shared.EventTypeRunCompleted {
			continue
		}
		completed, ok := event.Data.(*shared.RunCompletedEvent)
		require.True(t, ok)
		require.NotNil(t, completed)
		require.NotNil(t, completed.RunResult)
		return completed.RunResult
	}
	return nil
}

func countEventType(events []*shared.NodeEventCh, eventType shared.NodeEventType) int {
	count := 0
	for _, event := range events {
		if event.Type == eventType {
			count++
		}
	}
	return count
}

func testSubgraphEngineFactory() subgraph.EngineFactory {
	runner := workflowruntime.NewNodeRunner(workflowruntime.Dependencies{})
	return func(parallelism int) subgraph.Engine {
		engine := graph_engine.NewWorkflowEngine(parallelism)
		engine.SetNodeRunner(runner)
		return engine
	}
}

func newTestWorkflowEngine(maxConcurrency int) *graph_engine.WorkflowEngine {
	engine := graph_engine.NewWorkflowEngine(maxConcurrency)
	engine.SetNodeRunner(workflowruntime.NewNodeRunner(workflowruntime.Dependencies{}))
	return engine
}
