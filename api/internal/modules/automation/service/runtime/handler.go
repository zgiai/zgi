package runtime

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/hibiken/asynq"
)

// NewExecuteTaskHandler creates an asynq handler for automation execute tasks.
func NewExecuteTaskHandler(executor *Executor) func(context.Context, *asynq.Task) error {
	return func(ctx context.Context, task *asynq.Task) error {
		if executor == nil {
			return fmt.Errorf("automation executor is not configured: %w", asynq.SkipRetry)
		}

		var payload ExecuteTaskPayload
		if err := json.Unmarshal(task.Payload(), &payload); err != nil {
			return fmt.Errorf("unmarshal automation execute task payload: %v: %w", err, asynq.SkipRetry)
		}
		if payload.RunID == "" {
			return fmt.Errorf("automation execute task payload missing run_id: %w", asynq.SkipRetry)
		}

		if err := executor.ExecuteTaskRun(ctx, payload.RunID); err != nil {
			return fmt.Errorf("execute automation run %s: %w", payload.RunID, err)
		}
		return nil
	}
}
