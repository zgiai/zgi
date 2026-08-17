package retrieval

import "testing"

func TestFilterActiveGraphTraversalRejectsDisabledCandidatesAndEdges(t *testing.T) {
	candidates := []graphTraversalCandidate{
		{ID: "active", ActiveSourceCount: 1},
		{ID: "disabled", ActiveSourceCount: 0},
	}
	edges := []graphTraversalEdge{
		{ID: "active-edge", ActiveWeight: 1},
		{ID: "disabled-edge", ActiveWeight: 0},
	}
	activeCandidates, activeEdges := filterActiveGraphTraversal(candidates, edges)
	if len(activeCandidates) != 1 || activeCandidates[0].ID != "active" {
		t.Fatalf("candidates=%#v", activeCandidates)
	}
	if len(activeEdges) != 1 || activeEdges[0].ID != "active-edge" {
		t.Fatalf("edges=%#v", activeEdges)
	}
}

func TestGraphVisibilityRevisionGate(t *testing.T) {
	if graphVisibilityReady(4, 3) {
		t.Fatal("stale visibility projection was searchable")
	}
	if !graphVisibilityReady(4, 4) {
		t.Fatal("matching visibility revisions were rejected")
	}
}
