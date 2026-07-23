package graphflow

import (
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/zgiai/zgi/api/internal/modules/dataset/graphflow/model"
)

func TestApplyGraphQueryFiltersKeywordCategoryAndDocument(t *testing.T) {
	graph := graphQueryFixture()
	result, err := applyGraphQuery(graph, model.GraphQuery{
		Keyword:    "alice",
		Category:   "Person",
		DocumentID: "doc-enabled",
		NodeLimit:  10,
		EdgeLimit:  10,
	})
	require.NoError(t, err)
	require.Equal(t, []string{"ent:1"}, graphNodeIDs(result.Nodes))
	require.Empty(t, result.Edges)
}

func TestApplyGraphQueryExpandsSeedByAtMostTwoHops(t *testing.T) {
	graph := graphQueryFixture()
	result, err := applyGraphQuery(graph, model.GraphQuery{
		SeedNodeID: "ent:1",
		HopDepth:   2,
		NodeLimit:  10,
		EdgeLimit:  10,
	})
	require.NoError(t, err)
	require.Equal(t, []string{"ent:1", "ent:2", "ent:3"}, graphNodeIDs(result.Nodes))
	require.Len(t, result.Edges, 2)
}

func TestApplyGraphQueryReturnsStableCursorAndEndpointClosedEdges(t *testing.T) {
	graph := graphQueryFixture()
	first, err := applyGraphQuery(graph, model.GraphQuery{NodeLimit: 2, EdgeLimit: 10})
	require.NoError(t, err)
	require.Equal(t, []string{"ent:1", "ent:2"}, graphNodeIDs(first.Nodes))
	require.Equal(t, "ent:2", first.NextCursor)
	for _, edge := range first.Edges {
		require.Contains(t, graphNodeIDSet(first.Nodes), edge.Source)
		require.Contains(t, graphNodeIDSet(first.Nodes), edge.Target)
	}

	second, err := applyGraphQuery(graph, model.GraphQuery{Cursor: first.NextCursor, NodeLimit: 2, EdgeLimit: 10})
	require.NoError(t, err)
	require.Equal(t, []string{"ent:3", "ent:4"}, graphNodeIDs(second.Nodes))
}

func graphQueryFixture() *model.GraphDataResponse {
	return &model.GraphDataResponse{
		Nodes: []model.GraphNode{
			{ID: "ent:4", Label: "Delta", Category: "Person", Data: model.GraphNodeData{ActiveSourceCount: 1, Sources: []model.GraphNodeSource{{Doc: model.GraphSourceDoc{ID: "doc:doc-enabled"}, Weight: 1}}}},
			{ID: "ent:2", Label: "Bob", Category: "Person", Data: model.GraphNodeData{ActiveSourceCount: 1, Sources: []model.GraphNodeSource{{Doc: model.GraphSourceDoc{ID: "doc:doc-enabled"}, Weight: 1}}}},
			{ID: "ent:1", Label: "Alice", Category: "Person", Data: model.GraphNodeData{ActiveSourceCount: 1, Sources: []model.GraphNodeSource{{Doc: model.GraphSourceDoc{ID: "doc:doc-enabled"}, Weight: 1}}}},
			{ID: "ent:3", Label: "Acme", Category: "Organization", Data: model.GraphNodeData{ActiveSourceCount: 1, Sources: []model.GraphNodeSource{{Doc: model.GraphSourceDoc{ID: "doc:doc-enabled"}, Weight: 1}}}},
		},
		Edges: []model.GraphEdge{
			{Source: "ent:1", Target: "ent:2", Label: "knows", ActiveWeight: 1},
			{Source: "ent:2", Target: "ent:3", Label: "works_at", ActiveWeight: 1},
			{Source: "ent:3", Target: "ent:4", Label: "ignored", ActiveWeight: 0},
		},
	}
}

func graphNodeIDs(nodes []model.GraphNode) []string {
	ids := make([]string, 0, len(nodes))
	for _, node := range nodes {
		ids = append(ids, node.ID)
	}
	return ids
}

func graphNodeIDSet(nodes []model.GraphNode) map[string]struct{} {
	result := make(map[string]struct{}, len(nodes))
	for _, node := range nodes {
		result[node.ID] = struct{}{}
	}
	return result
}
