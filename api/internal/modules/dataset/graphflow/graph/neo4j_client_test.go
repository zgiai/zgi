package graph

import (
	"strings"
	"testing"
)

func TestBuildEntityContextMultiHopQueryBindsTraversalPath(t *testing.T) {
	query := buildEntityContextMultiHopQuery(2)
	for _, snippet := range []string{
		"MATCH path = (n)-[*1..2]-(m)",
		"nodes(path)",
		"relationships(path)",
	} {
		if !strings.Contains(query, snippet) {
			t.Fatalf("multi-hop query missing %q:\n%s", snippet, query)
		}
	}
}
