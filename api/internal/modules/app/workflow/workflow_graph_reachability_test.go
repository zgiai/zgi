package workflow

import (
	"context"
	"errors"
	"strings"
	"testing"
)

func TestWorkflowThatCannotReachItsEndNamesTheDisconnectedNode(t *testing.T) {
	sqlGenerator := workflowStreamTestNode("sql-generator", "sql-generator")
	sqlGenerator["data"].(map[string]interface{})["title"] = "智能SQL生成"
	callDatabase := workflowStreamTestNode("call-database", "call-database")
	callDatabase["data"].(map[string]interface{})["title"] = "执行自定义SQL"
	workflowData := map[string]interface{}{
		"graph": map[string]interface{}{
			"nodes": []interface{}{workflowStreamTestNode("start", "start"), sqlGenerator, callDatabase, workflowStreamTestNode("end", "end")},
			// The edge from SQL generation to the database call was deleted.
			"edges": []interface{}{workflowStreamTestEdge("start", "sql-generator"), workflowStreamTestEdge("call-database", "end")},
		},
	}
	_, err := buildWorkflowStreamGraph(context.Background(), workflowData)
	if !errors.Is(err, ErrWorkflowEndUnreachable) || !strings.Contains(err.Error(), `"执行自定义SQL" has no incoming connection`) {
		t.Fatalf("disconnected workflow error = %v", err)
	}

	// Reconnected, the same graph runs.
	graph := workflowData["graph"].(map[string]interface{})
	graph["edges"] = append(graph["edges"].([]interface{}), workflowStreamTestEdge("sql-generator", "call-database"))
	if _, err := buildWorkflowStreamGraph(context.Background(), workflowData); err != nil {
		t.Fatalf("connected workflow: %v", err)
	}

	// A graph without any end node is left to the run's own checks.
	noEnd := map[string]interface{}{"graph": map[string]interface{}{
		"nodes": []interface{}{workflowStreamTestNode("start", "start"), workflowStreamTestNode("llm", "llm")},
		"edges": []interface{}{workflowStreamTestEdge("start", "llm")},
	}}
	if _, err := buildWorkflowStreamGraph(context.Background(), noEnd); err != nil {
		t.Fatalf("graph without an end node: %v", err)
	}
}
