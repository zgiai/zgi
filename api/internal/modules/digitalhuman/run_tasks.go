package digitalhuman

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/hibiken/asynq"
	"github.com/zgiai/zgi/api/pkg/queue"
)

const (
	RunTaskType       = "digital_human:run"
	ReconcileTaskType = "digital_human:reconcile_runs"
	runQueueName      = "default"
	runTaskTimeout    = 8 * time.Minute
)

type RunTaskRegistry interface {
	Register(string, func(context.Context, *asynq.Task) error) bool
}

type runTaskPayload struct {
	RunID string `json:"run_id"`
}

type AsynqRunDispatcher struct{ tasks *queue.TaskManager }

func NewAsynqRunDispatcher(tasks *queue.TaskManager) *AsynqRunDispatcher {
	if tasks == nil {
		panic("DigitalHuman dispatcher requires task manager")
	}
	return &AsynqRunDispatcher{tasks: tasks}
}

func (d *AsynqRunDispatcher) EnqueueRun(_ context.Context, runID uuid.UUID) error {
	if runID == uuid.Nil {
		return ErrInvalidRequest
	}
	payload, err := json.Marshal(runTaskPayload{RunID: runID.String()})
	if err != nil {
		return fmt.Errorf("encode DigitalHuman run task: %w", err)
	}
	taskID := digitalHumanRunTaskID(runID)
	newTask := func() *asynq.Task {
		return asynq.NewTask(d.tasks.GetTaskTypeWithPrefix(RunTaskType), payload,
			asynq.Queue(runQueueName), asynq.MaxRetry(3), asynq.Timeout(runTaskTimeout), asynq.TaskID(taskID))
	}
	if _, err := d.tasks.EnqueueTask(newTask()); err != nil {
		if !errors.Is(err, asynq.ErrTaskIDConflict) {
			return err
		}
		reset, resetErr := d.tasks.ResetArchivedTask(runQueueName, taskID)
		if resetErr != nil || !reset {
			return resetErr
		}
		_, err = d.tasks.EnqueueTask(newTask())
		return err
	}
	return nil
}

func (d *AsynqRunDispatcher) CancelRun(_ context.Context, runID uuid.UUID) error {
	if runID == uuid.Nil {
		return ErrInvalidRequest
	}
	return d.tasks.CancelTask(runQueueName, digitalHumanRunTaskID(runID))
}

func digitalHumanRunTaskID(runID uuid.UUID) string { return "digital-human-run-" + runID.String() }

func RegisterRunTaskHandler(registry RunTaskRegistry, prefixer interface{ GetTaskTypeWithPrefix(string) string }, runtime *RuntimeService) {
	if registry == nil || prefixer == nil || runtime == nil {
		panic("DigitalHuman run task handler requires registry, prefixer, and runtime")
	}
	taskType := prefixer.GetTaskTypeWithPrefix(RunTaskType)
	if !registry.Register(taskType, func(ctx context.Context, task *asynq.Task) error {
		runID, err := decodeRunTask(task)
		if err != nil {
			return fmt.Errorf("%v: %w", err, asynq.SkipRetry)
		}
		return runtime.Execute(ctx, runID)
	}) {
		panic("DigitalHuman run task handler is already registered")
	}
}

func decodeRunTask(task *asynq.Task) (uuid.UUID, error) {
	if task == nil {
		return uuid.Nil, ErrInvalidRequest
	}
	var payload runTaskPayload
	decoder := json.NewDecoder(strings.NewReader(string(task.Payload())))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&payload); err != nil {
		return uuid.Nil, fmt.Errorf("decode DigitalHuman run task: %w", err)
	}
	if err := decoder.Decode(&struct{}{}); !errors.Is(err, io.EOF) {
		return uuid.Nil, errors.New("DigitalHuman run task contains trailing data")
	}
	runID, err := uuid.Parse(strings.TrimSpace(payload.RunID))
	if err != nil || runID == uuid.Nil {
		return uuid.Nil, errors.New("DigitalHuman run task has invalid run_id")
	}
	return runID, nil
}

// ReconcileTask periodically repairs the durable DB-to-queue delivery window.
type ReconcileTask struct{}

func (*ReconcileTask) TaskType() string        { return ReconcileTaskType }
func (*ReconcileTask) CronSpec() string        { return "" }
func (*ReconcileTask) Interval() time.Duration { return 15 * time.Second }
func (*ReconcileTask) Payload() []byte         { return nil }
func (*ReconcileTask) Options() []asynq.Option {
	return []asynq.Option{asynq.MaxRetry(1), asynq.Timeout(time.Minute), asynq.Unique(10 * time.Second)}
}

type ReconcileHandler struct{ runtime *RuntimeService }

func NewReconcileHandler(runtime *RuntimeService) *ReconcileHandler {
	if runtime == nil {
		panic("DigitalHuman reconcile handler requires runtime")
	}
	return &ReconcileHandler{runtime: runtime}
}

func (h *ReconcileHandler) Handle(ctx context.Context, _ *asynq.Task) error {
	return h.runtime.Reconcile(ctx)
}
