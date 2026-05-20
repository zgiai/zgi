package condbranch

import "github.com/zgiai/zgi/api/internal/modules/app/workflow/graph_engine/entities"

// EvaluateConditions evaluates conditions using the same logic as the branch node.
func EvaluateConditions(
	vp *entities.VariablePool,
	conditions []Condition,
	operator LogicalOperator,
) (bool, []map[string]any, error) {
	inputConditions, _, result, err := processConditions(vp, conditions, operator)
	return result, inputConditions, err
}
