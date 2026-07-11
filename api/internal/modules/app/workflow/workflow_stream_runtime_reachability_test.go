package workflow

import (
	"context"
	"errors"
	"testing"

	"github.com/google/uuid"
	"github.com/zgiai/zgi/api/internal/modules/app/conversation"
	graph_entities "github.com/zgiai/zgi/api/internal/modules/app/workflow/graph_engine/entities"
)

func TestBuildWorkflowStreamGraphIgnoresNodesUnreachableFromStart(t *testing.T) {
	workflowData := map[string]interface{}{
		"graph": map[string]interface{}{
			"nodes": []interface{}{
				workflowStreamTestNode("start", "start"),
				workflowStreamTestNode("reachable-answer", "answer"),
				workflowStreamTestNode("orphan-llm", "llm"),
				workflowStreamTestNode("orphan-answer", "answer"),
			},
			"edges": []interface{}{
				workflowStreamTestEdge("start", "reachable-answer"),
				workflowStreamTestEdge("start", "missing-node"),
				workflowStreamTestEdge("missing-node", "orphan-llm"),
				workflowStreamTestEdge("orphan-llm", "orphan-answer"),
			},
		},
	}

	streamGraph, err := buildWorkflowStreamGraph(context.Background(), workflowData)

	if err != nil {
		t.Fatalf("buildWorkflowStreamGraph() error = %v", err)
	}
	if streamGraph.StartNodeID != "start" {
		t.Fatalf("StartNodeID = %q, want start", streamGraph.StartNodeID)
	}
	if _, exists := streamGraph.NodeMap["orphan-llm"]; exists {
		t.Fatalf("orphan-llm should not be present in stream node map")
	}
	if _, exists := streamGraph.NodeMap["orphan-answer"]; exists {
		t.Fatalf("orphan-answer should not be present in stream node map")
	}
	if _, exists := streamGraph.EdgeMap["orphan-llm"]; exists {
		t.Fatalf("orphan-llm should not be present in stream edge map")
	}
	if _, exists := streamGraph.ReverseEdgeMap["orphan-answer"]; exists {
		t.Fatalf("orphan-answer should not be present in stream reverse edge map")
	}

	nodes, ok := streamGraph.GraphData["nodes"].([]interface{})
	if !ok {
		t.Fatalf("filtered graph nodes type = %T, want []interface{}", streamGraph.GraphData["nodes"])
	}
	if len(nodes) != 2 {
		t.Fatalf("filtered graph node count = %d, want 2", len(nodes))
	}
}

func TestBuildWorkflowStreamGraphKeepsReachableIterationSubgraphInRuntimeView(t *testing.T) {
	workflowData := map[string]interface{}{
		"graph": map[string]interface{}{
			"nodes": []interface{}{
				workflowStreamTestNode("start", "start"),
				workflowStreamTestNode("prepare", "code"),
				workflowStreamTestContainerNode("iter", "iteration", "iter-start"),
				workflowStreamTestChildNode("iter-start", "iteration-start", "iter"),
				workflowStreamTestChildNode("iter-code", "code", "iter"),
				workflowStreamTestChildNode("iter-answer", "answer", "iter"),
				workflowStreamTestChildNode("iter-orphan-answer", "answer", "iter"),
			},
			"edges": []interface{}{
				workflowStreamTestEdge("start", "prepare"),
				workflowStreamTestEdge("prepare", "iter"),
				workflowStreamTestEdge("iter-start", "iter-code"),
				workflowStreamTestEdge("iter-code", "iter-answer"),
			},
		},
	}

	streamGraph, err := buildWorkflowStreamGraph(context.Background(), workflowData)

	if err != nil {
		t.Fatalf("buildWorkflowStreamGraph() error = %v", err)
	}
	assertWorkflowStreamNodePresence(t, streamGraph.NodeMap, "iter", true)
	assertWorkflowStreamNodePresence(t, streamGraph.NodeMap, "iter-start", false)
	assertWorkflowStreamNodePresence(t, streamGraph.NodeMap, "iter-code", false)
	assertWorkflowStreamNodePresence(t, streamGraph.RuntimeNodeMap, "iter-start", true)
	assertWorkflowStreamNodePresence(t, streamGraph.RuntimeNodeMap, "iter-code", true)
	assertWorkflowStreamNodePresence(t, streamGraph.RuntimeNodeMap, "iter-answer", true)
	assertWorkflowStreamNodePresence(t, streamGraph.RuntimeNodeMap, "iter-orphan-answer", false)

	if targets := streamGraph.EdgeMap["iter-start"]["source"]; len(targets) != 0 {
		t.Fatalf("execution edge map should not contain iteration internal edges")
	}
	if !workflowStreamGraphDataContainsNode(streamGraph.GraphData, "iter-start") {
		t.Fatalf("runtime graph data should contain iteration start")
	}
	if !workflowStreamGraphDataContainsEdge(streamGraph.GraphData, "iter-start", "iter-code") {
		t.Fatalf("runtime graph data should contain iteration internal edge")
	}
}

func TestBuildWorkflowStreamGraphKeepsReachableLoopSubgraphInRuntimeView(t *testing.T) {
	workflowData := map[string]interface{}{
		"graph": map[string]interface{}{
			"nodes": []interface{}{
				workflowStreamTestNode("start", "start"),
				workflowStreamTestContainerNode("loop", "loop", "loop-start"),
				workflowStreamTestChildNode("loop-start", "loop-start", "loop"),
				workflowStreamTestChildNode("loop-code", "code", "loop"),
				workflowStreamTestChildNode("loop-answer", "answer", "loop"),
				workflowStreamTestChildNode("loop-orphan-answer", "answer", "loop"),
			},
			"edges": []interface{}{
				workflowStreamTestEdge("start", "loop"),
				workflowStreamTestEdge("loop-start", "loop-code"),
				workflowStreamTestEdge("loop-code", "loop-answer"),
			},
		},
	}

	streamGraph, err := buildWorkflowStreamGraph(context.Background(), workflowData)

	if err != nil {
		t.Fatalf("buildWorkflowStreamGraph() error = %v", err)
	}
	assertWorkflowStreamNodePresence(t, streamGraph.NodeMap, "loop-start", false)
	assertWorkflowStreamNodePresence(t, streamGraph.RuntimeNodeMap, "loop-start", true)
	assertWorkflowStreamNodePresence(t, streamGraph.RuntimeNodeMap, "loop-code", true)
	assertWorkflowStreamNodePresence(t, streamGraph.RuntimeNodeMap, "loop-answer", true)
	assertWorkflowStreamNodePresence(t, streamGraph.RuntimeNodeMap, "loop-orphan-answer", false)
}

func TestBuildWorkflowStreamGraphKeepsNestedContainerSubgraphInRuntimeView(t *testing.T) {
	workflowData := map[string]interface{}{
		"graph": map[string]interface{}{
			"nodes": []interface{}{
				workflowStreamTestNode("start", "start"),
				workflowStreamTestContainerNode("iter", "iteration", "iter-start"),
				workflowStreamTestChildNode("iter-start", "iteration-start", "iter"),
				workflowStreamTestChildContainerNode("inner-loop", "loop", "iter", "inner-loop-start"),
				workflowStreamTestChildNode("inner-loop-start", "loop-start", "inner-loop"),
				workflowStreamTestChildNode("inner-loop-answer", "answer", "inner-loop"),
			},
			"edges": []interface{}{
				workflowStreamTestEdge("start", "iter"),
				workflowStreamTestEdge("iter-start", "inner-loop"),
				workflowStreamTestEdge("inner-loop-start", "inner-loop-answer"),
			},
		},
	}

	streamGraph, err := buildWorkflowStreamGraph(context.Background(), workflowData)

	if err != nil {
		t.Fatalf("buildWorkflowStreamGraph() error = %v", err)
	}
	assertWorkflowStreamNodePresence(t, streamGraph.NodeMap, "inner-loop", false)
	assertWorkflowStreamNodePresence(t, streamGraph.RuntimeNodeMap, "inner-loop", true)
	assertWorkflowStreamNodePresence(t, streamGraph.RuntimeNodeMap, "inner-loop-start", true)
	assertWorkflowStreamNodePresence(t, streamGraph.RuntimeNodeMap, "inner-loop-answer", true)
}

func TestBuildWorkflowStreamVariablePoolRejectsUnauthorizedConversationBeforeVariableLoad(t *testing.T) {
	conversationID := uuid.New()
	agentID := uuid.New()
	accountID := uuid.New()
	otherAccountID := uuid.New()
	service := &workflowStreamVariablePoolConversationService{
		conversation: &conversation.AgentConversation{
			ID:            conversationID,
			AgentID:       agentID,
			FromAccountID: &otherAccountID,
		},
		variables: map[string]interface{}{"profile": "should-not-load"},
	}

	pool, err := buildWorkflowStreamVariablePool(context.Background(),
		workflowStreamVariablePoolTestGraphData(),
		map[string]interface{}{"sys.conversation_id": conversationID.String()},
		nil,
		"start",
		workflowStreamVariablePoolScope{
			ConversationAccess: service,
			VariableLoader:     service,
			AgentID:            agentID.String(),
			AccountID:          accountID.String(),
		},
	)

	if !errors.Is(err, errWebAppConversationAccessDenied) {
		t.Fatalf("buildWorkflowStreamVariablePool() error = %v, want %v", err, errWebAppConversationAccessDenied)
	}
	if pool != nil {
		t.Fatalf("buildWorkflowStreamVariablePool() pool = %#v, want nil", pool)
	}
	if !service.accessCalled {
		t.Fatalf("conversation access check was not called")
	}
	if service.loadCalled {
		t.Fatalf("conversation variables were loaded before caller scope was accepted")
	}
}

func TestBuildWorkflowStreamVariablePoolLoadsConversationVariablesAfterAccessCheck(t *testing.T) {
	conversationID := uuid.New()
	agentID := uuid.New()
	accountID := uuid.New()
	service := &workflowStreamVariablePoolConversationService{
		conversation: &conversation.AgentConversation{
			ID:            conversationID,
			AgentID:       agentID,
			FromAccountID: &accountID,
		},
		variables: map[string]interface{}{"profile": "persisted-profile"},
	}

	pool, err := buildWorkflowStreamVariablePool(context.Background(),
		workflowStreamVariablePoolTestGraphData(),
		map[string]interface{}{"sys.conversation_id": conversationID.String()},
		nil,
		"start",
		workflowStreamVariablePoolScope{
			ConversationAccess: service,
			VariableLoader:     service,
			AgentID:            agentID.String(),
			AccountID:          accountID.String(),
		},
	)

	if err != nil {
		t.Fatalf("buildWorkflowStreamVariablePool() error = %v", err)
	}
	if !service.accessCalled {
		t.Fatalf("conversation access check was not called")
	}
	if !service.loadCalled {
		t.Fatalf("conversation variables were not loaded after caller scope was accepted")
	}
	variable := pool.Get([]string{graph_entities.ConversationVariableNodeId, "profile"})
	if variable == nil {
		t.Fatalf("conversation profile variable missing")
	}
	if got := variable.ToObject(); got != "persisted-profile" {
		t.Fatalf("conversation profile variable = %#v, want %#v", got, "persisted-profile")
	}
}

func TestBuildWorkflowStreamVariablePoolSkipsConversationAccessWithoutConversationVariables(t *testing.T) {
	conversationID := uuid.New()
	service := &workflowStreamVariablePoolConversationService{
		variables: map[string]interface{}{"profile": "should-not-load"},
	}

	pool, err := buildWorkflowStreamVariablePool(context.Background(),
		map[string]any{
			"environment_variables":  []any{},
			"conversation_variables": []any{},
		},
		map[string]interface{}{"sys.conversation_id": conversationID.String()},
		nil,
		"start",
		workflowStreamVariablePoolScope{
			ConversationAccess: service,
			VariableLoader:     service,
		},
	)

	if err != nil {
		t.Fatalf("buildWorkflowStreamVariablePool() error = %v", err)
	}
	if pool == nil {
		t.Fatalf("buildWorkflowStreamVariablePool() pool is nil")
	}
	if service.accessCalled {
		t.Fatalf("conversation access check should be skipped when no conversation variables are configured")
	}
	if service.loadCalled {
		t.Fatalf("conversation variables should not be loaded when no conversation variables are configured")
	}
}

func TestBuildWorkflowStreamVariablePoolFallsBackToDefaultWhenVariableLoadFails(t *testing.T) {
	conversationID := uuid.New()
	agentID := uuid.New()
	accountID := uuid.New()
	service := &workflowStreamVariablePoolConversationService{
		conversation: &conversation.AgentConversation{
			ID:            conversationID,
			AgentID:       agentID,
			FromAccountID: &accountID,
		},
		loadErr: errors.New("database unavailable"),
	}

	pool, err := buildWorkflowStreamVariablePool(context.Background(),
		workflowStreamVariablePoolTestGraphData(),
		map[string]interface{}{"sys.conversation_id": conversationID.String()},
		nil,
		"start",
		workflowStreamVariablePoolScope{
			ConversationAccess: service,
			VariableLoader:     service,
			AgentID:            agentID.String(),
			AccountID:          accountID.String(),
		},
	)

	if err != nil {
		t.Fatalf("buildWorkflowStreamVariablePool() error = %v", err)
	}
	if !service.accessCalled {
		t.Fatalf("conversation access check was not called")
	}
	if !service.loadCalled {
		t.Fatalf("conversation variables were not loaded after caller scope was accepted")
	}
	variable := pool.Get([]string{graph_entities.ConversationVariableNodeId, "profile"})
	if variable == nil {
		t.Fatalf("conversation profile variable missing")
	}
	if got := variable.ToObject(); got != "default-profile" {
		t.Fatalf("conversation profile variable = %#v, want %#v", got, "default-profile")
	}
}

func workflowStreamVariablePoolTestGraphData() map[string]any {
	return map[string]any{
		"environment_variables": []any{},
		"conversation_variables": []any{
			map[string]any{
				"name":  "profile",
				"type":  "string",
				"value": "default-profile",
			},
		},
	}
}

type workflowStreamVariablePoolConversationService struct {
	conversation *conversation.AgentConversation
	variables    map[string]interface{}
	loadErr      error

	accessCalled bool
	loadCalled   bool
}

func (s *workflowStreamVariablePoolConversationService) GetConversationByIDAndAgent(_ context.Context, conversationID, agentID uuid.UUID) (*conversation.AgentConversation, error) {
	s.accessCalled = true
	if s.conversation == nil || s.conversation.ID != conversationID || s.conversation.AgentID != agentID {
		return nil, errors.New("conversation not found")
	}
	return s.conversation, nil
}

func (s *workflowStreamVariablePoolConversationService) LoadConversationVariables(conversationID uuid.UUID) (map[string]interface{}, error) {
	s.loadCalled = true
	if s.loadErr != nil {
		return nil, s.loadErr
	}
	if s.conversation == nil || s.conversation.ID != conversationID {
		return nil, errors.New("conversation not found")
	}
	return s.variables, nil
}

func workflowStreamTestNode(id string, nodeType string) map[string]interface{} {
	return map[string]interface{}{
		"id": id,
		"data": map[string]interface{}{
			"type":  nodeType,
			"title": id,
		},
	}
}

func workflowStreamTestContainerNode(id string, nodeType string, startNodeID string) map[string]interface{} {
	node := workflowStreamTestNode(id, nodeType)
	node["data"].(map[string]interface{})["start_node_id"] = startNodeID
	return node
}

func workflowStreamTestChildContainerNode(id string, nodeType string, parentID string, startNodeID string) map[string]interface{} {
	node := workflowStreamTestContainerNode(id, nodeType, startNodeID)
	node["parentId"] = parentID
	return node
}

func workflowStreamTestChildNode(id string, nodeType string, parentID string) map[string]interface{} {
	node := workflowStreamTestNode(id, nodeType)
	node["parentId"] = parentID
	return node
}

func workflowStreamTestEdge(source string, target string) map[string]interface{} {
	return map[string]interface{}{
		"source": source,
		"target": target,
	}
}

func assertWorkflowStreamNodePresence(t *testing.T, nodeMap map[string]map[string]interface{}, nodeID string, want bool) {
	t.Helper()
	_, exists := nodeMap[nodeID]
	if exists != want {
		t.Fatalf("node %s presence = %t, want %t", nodeID, exists, want)
	}
}

func workflowStreamGraphDataContainsNode(graphData map[string]any, nodeID string) bool {
	nodes, _ := graphData["nodes"].([]interface{})
	for _, nodeInterface := range nodes {
		node, _ := nodeInterface.(map[string]interface{})
		if id, _ := node["id"].(string); id == nodeID {
			return true
		}
	}
	return false
}

func workflowStreamGraphDataContainsEdge(graphData map[string]any, source string, target string) bool {
	edges, _ := graphData["edges"].([]interface{})
	for _, edgeInterface := range edges {
		edge, _ := edgeInterface.(map[string]interface{})
		edgeSource, _ := edge["source"].(string)
		edgeTarget, _ := edge["target"].(string)
		if edgeSource == source && edgeTarget == target {
			return true
		}
	}
	return false
}
