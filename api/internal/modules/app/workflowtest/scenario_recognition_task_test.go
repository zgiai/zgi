package workflowtest

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/hibiken/asynq"
	"github.com/stretchr/testify/require"
)

func TestNewScenarioRecognitionTaskAsynqTaskPayloadAndType(t *testing.T) {
	task, err := NewScenarioRecognitionTaskAsynqTask("task-1", nil)

	require.NoError(t, err)
	require.Equal(t, WorkflowTestScenarioRecognitionTaskType, task.Type())
	require.JSONEq(t, `{"task_id":"task-1"}`, string(task.Payload()))
}

func TestScenarioRecognitionTaskAsynqOptions(t *testing.T) {
	opts := scenarioRecognitionTaskAsynqOptions()

	require.Len(t, opts, 3)
	requireTaskOption(t, opts, asynq.QueueOpt, "default")
	requireTaskOption(t, opts, asynq.MaxRetryOpt, 0)
	requireTaskOption(t, opts, asynq.TimeoutOpt, 10*time.Minute)
}

func TestNewScenarioRecognitionTaskHandlerReturnsSkipRetryForBadPayload(t *testing.T) {
	handler := NewScenarioRecognitionTaskHandler(NewService(nil), &fakeLLMClient{})

	err := handler(context.Background(), asynq.NewTask(WorkflowTestScenarioRecognitionTaskType, []byte(`{`)))
	require.Error(t, err)
	require.True(t, errors.Is(err, asynq.SkipRetry))

	err = handler(context.Background(), asynq.NewTask(WorkflowTestScenarioRecognitionTaskType, []byte(`{}`)))
	require.Error(t, err)
	require.True(t, errors.Is(err, asynq.SkipRetry))
}
