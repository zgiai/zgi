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

func TestApplyGraphQueryOverviewPrioritizesStrongConnectedEntities(t *testing.T) {
	graph := graphQueryFixture()
	graph.Edges = append(graph.Edges,
		model.GraphEdge{Source: "ent:3", Target: "ent:4", Label: "strong", ActiveWeight: 9},
	)

	result, err := applyGraphQuery(graph, model.GraphQuery{
		Overview:  true,
		NodeLimit: 2,
		EdgeLimit: 10,
	})
	require.NoError(t, err)
	require.Equal(t, []string{"ent:3", "ent:4"}, graphNodeIDs(result.Nodes))
	require.Equal(t, 4, result.TotalNodes)
	require.Equal(t, 3, result.TotalEdges)
	require.Empty(t, result.NextCursor)
	require.Equal(t, "strong", result.Edges[0].Label)
}

func TestApplyGraphQueryOverviewGrowsFromStablePrefix(t *testing.T) {
	graph := graphQueryFixture()

	smaller, err := applyGraphQuery(graph, model.GraphQuery{
		Overview:  true,
		NodeLimit: 2,
		EdgeLimit: 10,
	})
	require.NoError(t, err)

	larger, err := applyGraphQuery(graph, model.GraphQuery{
		Overview:  true,
		NodeLimit: 4,
		EdgeLimit: 10,
	})
	require.NoError(t, err)
	require.Equal(t, graphNodeIDs(smaller.Nodes), graphNodeIDs(larger.Nodes[:len(smaller.Nodes)]))
}

func TestApplyGraphQueryAllowsOverviewToReachFullGraphSize(t *testing.T) {
	result, err := applyGraphQuery(graphQueryFixture(), model.GraphQuery{
		Overview:  true,
		NodeLimit: maxGraphNodeLimit + 1,
		EdgeLimit: maxGraphEdgeLimit + 1,
	})

	require.NoError(t, err)
	require.Len(t, result.Nodes, 4)
}

func TestApplyGraphQueryNeighborhoodAlwaysIncludesSeedAndStrongestNeighbor(t *testing.T) {
	graph := graphQueryFixture()
	graph.Edges = append(graph.Edges,
		model.GraphEdge{Source: "ent:3", Target: "ent:4", Label: "strong", ActiveWeight: 9},
	)

	result, err := applyGraphQuery(graph, model.GraphQuery{
		SeedNodeID: "ent:4",
		HopDepth:   1,
		NodeLimit:  2,
		EdgeLimit:  10,
	})
	require.NoError(t, err)
	require.Equal(t, []string{"ent:4", "ent:3"}, graphNodeIDs(result.Nodes))
	require.Equal(t, 2, result.TotalNodes)
	require.Equal(t, 1, result.TotalEdges)
	require.Empty(t, result.NextCursor)
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
