package condbranch

import (
	"testing"

	"github.com/zgiai/zgi/api/internal/modules/app/workflow/graph_engine/entities"
)

func TestEvaluateConditions_UsesNestedSelector(t *testing.T) {
	vp := entities.NewVariablePool()
	vp.Add([]string{"answer", "payload"}, map[string]any{
		"text": "nested value",
	})

	matched, _, err := EvaluateConditions(vp, []Condition{
		{
			VariableSelector:   []string{"answer", "payload", "text"},
			ComparisonOperator: ComparisonOperatorIs,
			Value:              "nested value",
		},
	}, LogicalOperatorAnd)
	if err != nil {
		t.Fatalf("EvaluateConditions() error = %v", err)
	}
	if !matched {
		t.Fatalf("expected nested selector condition to match")
	}
}

func TestEvaluateConditions_UsesNestedSelectorForNotExists(t *testing.T) {
	vp := entities.NewVariablePool()
	vp.Add([]string{"answer", "payload"}, map[string]any{
		"text": "nested value",
	})

	matched, _, err := EvaluateConditions(vp, []Condition{
		{
			VariableSelector:   []string{"answer", "payload", "missing"},
			ComparisonOperator: ComparisonOperatorNotExists,
		},
	}, LogicalOperatorAnd)
	if err != nil {
		t.Fatalf("EvaluateConditions() error = %v", err)
	}
	if !matched {
		t.Fatalf("expected missing nested selector to satisfy not exists")
	}
}
