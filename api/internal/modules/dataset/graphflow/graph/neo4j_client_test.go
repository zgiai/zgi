package graph

import (
	"fmt"
	"strings"
	"testing"

	"github.com/neo4j/neo4j-go-driver/v5/neo4j"
)

func TestBuildEntityContextMultiHopQueryBindsTraversalPath(t *testing.T) {
	query := buildEntityContextMultiHopQuery(2)
	for _, snippet := range []string{
		"n.name IN names",
		"n.canonical_name IN names",
		"MATCH path = (n)-[*1..2]-(m)",
		"nodes(path)",
		"relationships(path)",
		"ORDER BY coalesce(m.name, ''), m.id",
		"[..50] as neighbors",
	} {
		if !strings.Contains(query, snippet) {
			t.Fatalf("multi-hop query missing %q:\n%s", snippet, query)
		}
	}
}

func TestIsEquivalentSchemaRuleAlreadyExists(t *testing.T) {
	tests := []struct {
		name string
		err  error
		want bool
	}{
		{
			name: "direct equivalent schema error",
			err:  &neo4j.Neo4jError{Code: equivalentSchemaRuleAlreadyExistsCode},
			want: true,
		},
		{
			name: "wrapped equivalent schema error",
			err:  fmt.Errorf("create vector index: %w", &neo4j.Neo4jError{Code: equivalentSchemaRuleAlreadyExistsCode}),
			want: true,
		},
		{
			name: "different neo4j error",
			err:  &neo4j.Neo4jError{Code: "Neo.ClientError.Schema.IndexAlreadyExists"},
			want: false,
		},
		{
			name: "plain error containing code",
			err:  fmt.Errorf("%s", equivalentSchemaRuleAlreadyExistsCode),
			want: false,
		},
		{
			name: "nil error",
			err:  nil,
			want: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := isEquivalentSchemaRuleAlreadyExists(tt.err); got != tt.want {
				t.Fatalf("isEquivalentSchemaRuleAlreadyExists() = %v, want %v", got, tt.want)
			}
		})
	}
}
