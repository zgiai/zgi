package shared

import (
	"context"
	"encoding/json"
)

type executionPathKey struct{}
type executionOwnerKey struct{}

type ExecutionStep struct {
	Kind   string `json:"kind"`
	NodeID string `json:"node_id"`
	Index  int    `json:"index"`
}

// WithExecutionStep carries server-owned loop coordinates through nested and
// parallel subgraphs. Model arguments and mutable node instance IDs are excluded.
func WithExecutionStep(ctx context.Context, kind, nodeID string, index int) context.Context {
	previous, _ := ctx.Value(executionPathKey{}).([]ExecutionStep)
	path := append(append([]ExecutionStep{}, previous...), ExecutionStep{kind, nodeID, index})
	return context.WithValue(ctx, executionPathKey{}, path)
}

func ExecutionStepID(ctx context.Context, nodeID string) string {
	path, _ := ctx.Value(executionPathKey{}).([]ExecutionStep)
	encoded, _ := json.Marshal(struct {
		Path   []ExecutionStep `json:"path"`
		NodeID string          `json:"node_id"`
	}{path, nodeID})
	return string(encoded)
}

type ExecutionOwner struct {
	RunID, ExecutionID string
	Generation         int64
}

func WithExecutionOwner(ctx context.Context, owner ExecutionOwner) context.Context {
	return context.WithValue(ctx, executionOwnerKey{}, owner)
}

func ExecutionOwnerFromContext(ctx context.Context) (ExecutionOwner, bool) {
	owner, ok := ctx.Value(executionOwnerKey{}).(ExecutionOwner)
	return owner, ok && owner.RunID != "" && owner.ExecutionID != "" && owner.Generation > 0
}
