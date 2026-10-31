package queue

import (
	"errors"
	"fmt"
	"testing"

	"github.com/hibiken/asynq"
	"github.com/stretchr/testify/assert"
)

func TestClassifyTaskFailure(t *testing.T) {
	missing := fmt.Errorf("handler not found for task %q", "prod:digital_human:run")
	for _, tc := range []struct {
		name           string
		retried        int
		err            error
		exhausted      bool
		missingHandler bool
	}{
		{name: "retryable", retried: 1, err: errors.New("boom")},
		{name: "retries exhausted", retried: 3, err: errors.New("boom"), exhausted: true},
		{name: "retry skipped", retried: 0, err: fmt.Errorf("bad payload: %w", asynq.SkipRetry), exhausted: true},
		{name: "missing handler retried", retried: 1, err: missing, missingHandler: true},
		{name: "missing handler archived", retried: 3, err: missing, exhausted: true, missingHandler: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			exhausted, missingHandler := classifyTaskFailure(tc.retried, 3, tc.err)
			assert.Equal(t, tc.exhausted, exhausted)
			assert.Equal(t, tc.missingHandler, missingHandler)
		})
	}
}
