package executionruntime

import (
	"context"
	"fmt"
	"strings"
	"time"
)

// The conversation idempotency key returns the same Binding after an idle
// suspension. Resume it with its current generation before accessing jobs or
// files; the caller's context bounds any transitional-state polling.
func readyAgentExecutionBinding(ctx context.Context, gateway agentExecutionGateway, scope agentExecutionScope, agentID, conversationID, profileID, backend string, networkPolicyIDs ...string) (agentExecutionBinding, error) {
	binding, err := gateway.CreateBinding(ctx, scope, agentID, conversationID, profileID, backend, "", firstAgentExecutionNetworkPolicy(networkPolicyIDs...))
	if err != nil {
		return agentExecutionBinding{}, err
	}
	recreated := binding.Recreated
	seenTerminal := make(map[string]struct{})
	for {
		switch binding.Status {
		case "active":
			binding.Recreated = recreated
			return binding, nil
		case "suspended":
			binding, err = gateway.ResumeBinding(ctx, scope, binding.ID, binding.Generation)
			if err != nil {
				return agentExecutionBinding{}, fmt.Errorf("resume agent execution binding: %w", err)
			}
			if binding.Status != "active" && binding.Status != "resuming" {
				return agentExecutionBinding{}, fmt.Errorf("agent execution binding resume returned %s", binding.Status)
			}
		case "allocating", "suspending", "resuming":
			select {
			case <-ctx.Done():
				return agentExecutionBinding{}, ctx.Err()
			case <-time.After(200 * time.Millisecond):
			}
			binding, err = gateway.GetBinding(ctx, scope, binding.ID)
			if err != nil {
				return agentExecutionBinding{}, fmt.Errorf("read agent execution binding: %w", err)
			}
		case "deleted", "failed":
			if _, exists := seenTerminal[binding.ID]; exists {
				return agentExecutionBinding{}, fmt.Errorf("agent execution binding replacement did not advance")
			}
			seenTerminal[binding.ID] = struct{}{}
			binding, err = gateway.CreateBinding(ctx, scope, agentID, conversationID, profileID, backend, "", firstAgentExecutionNetworkPolicy(networkPolicyIDs...))
			if err != nil {
				return agentExecutionBinding{}, fmt.Errorf("replace terminal agent execution binding: %w", err)
			}
			recreated = recreated || binding.Recreated
		default:
			return agentExecutionBinding{}, fmt.Errorf("agent execution binding is %s", binding.Status)
		}
	}
}

func firstAgentExecutionNetworkPolicy(values ...string) string {
	if len(values) == 0 {
		return "offline"
	}
	if value := strings.TrimSpace(values[0]); value != "" {
		return value
	}
	return "offline"
}
