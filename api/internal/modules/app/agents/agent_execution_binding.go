package agents

import (
	"context"
	"fmt"
	"time"
)

// The conversation idempotency key returns the same Binding after an idle
// suspension. Resume it with its current generation before accessing jobs or
// files; the caller's context bounds any transitional-state polling.
func readyAgentExecutionBinding(ctx context.Context, gateway agentExecutionGateway, scope agentExecutionScope, agentID, conversationID, profileID, backend string) (agentExecutionBinding, error) {
	binding, err := gateway.CreateBinding(ctx, scope, agentID, conversationID, profileID, backend)
	if err != nil {
		return agentExecutionBinding{}, err
	}
	for {
		switch binding.Status {
		case "active":
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
		default:
			return agentExecutionBinding{}, fmt.Errorf("agent execution binding is %s", binding.Status)
		}
	}
}
