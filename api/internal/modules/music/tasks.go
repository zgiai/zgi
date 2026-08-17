package music

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
	GenerationTaskType    = "music:generate"
	CompensationTaskType  = "music:compensate_delivery"
	generationTaskTimeout = 9 * time.Minute
)

type TaskRegistry interface {
	Register(string, func(context.Context, *asynq.Task) error) bool
}

type TaskTypePrefixer interface {
	GetTaskTypeWithPrefix(string) string
}

type DispatcherImpl struct {
	tasks *queue.TaskManager
}

func NewDispatcher(tasks *queue.TaskManager) *DispatcherImpl {
	if tasks == nil {
		panic("music dispatcher requires task manager")
	}
	return &DispatcherImpl{tasks: tasks}
}

func (d *DispatcherImpl) EnqueueGeneration(_ context.Context, id uuid.UUID) error {
	task, err := newMusicAsynqTask(d.tasks, GenerationTaskType, id,
		asynq.Queue("default"),
		asynq.MaxRetry(3),
		asynq.Timeout(generationTaskTimeout),
		asynq.TaskID("music-generate-"+id.String()),
	)
	if err != nil {
		return err
	}
	_, err = d.tasks.EnqueueTask(task)
	if errors.Is(err, asynq.ErrTaskIDConflict) {
		return nil
	}
	return err
}

func (d *DispatcherImpl) EnqueueCompensation(_ context.Context, id uuid.UUID) error {
	task, err := newMusicAsynqTask(d.tasks, CompensationTaskType, id,
		asynq.Queue("default"),
		asynq.MaxRetry(10),
		asynq.Timeout(time.Minute),
		asynq.Unique(45*time.Second),
	)
	if err != nil {
		return err
	}
	_, err = d.tasks.EnqueueTask(task)
	if errors.Is(err, asynq.ErrDuplicateTask) {
		return nil
	}
	return err
}

type taskPayload struct {
	TaskID string `json:"task_id"`
}

func newMusicAsynqTask(manager *queue.TaskManager, taskType string, id uuid.UUID, options ...asynq.Option) (*asynq.Task, error) {
	if manager == nil || id == uuid.Nil {
		return nil, ErrInvalidRequest
	}
	payload, err := json.Marshal(taskPayload{TaskID: id.String()})
	if err != nil {
		return nil, fmt.Errorf("marshal music task payload: %w", err)
	}
	return asynq.NewTask(manager.GetTaskTypeWithPrefix(taskType), payload, options...), nil
}

func RegisterTaskHandlers(registry TaskRegistry, prefixer TaskTypePrefixer, worker *Worker) {
	if registry == nil || prefixer == nil || worker == nil {
		panic("music task handlers require registry, task type prefixer, and worker")
	}
	if !registry.Register(prefixer.GetTaskTypeWithPrefix(GenerationTaskType), newTaskHandler(worker.Generate)) {
		panic("music generation task handler is already registered")
	}
	if !registry.Register(prefixer.GetTaskTypeWithPrefix(CompensationTaskType), newTaskHandler(worker.Compensate)) {
		panic("music compensation task handler is already registered")
	}
}

func newTaskHandler(run func(context.Context, uuid.UUID) error) func(context.Context, *asynq.Task) error {
	return func(ctx context.Context, task *asynq.Task) error {
		if task == nil || run == nil {
			return fmt.Errorf("music task handler is not configured: %w", asynq.SkipRetry)
		}
		var payload taskPayload
		decoder := json.NewDecoder(strings.NewReader(string(task.Payload())))
		decoder.DisallowUnknownFields()
		if err := decoder.Decode(&payload); err != nil {
			return fmt.Errorf("decode music task payload: %v: %w", err, asynq.SkipRetry)
		}
		if err := decoder.Decode(&struct{}{}); !errors.Is(err, io.EOF) {
			return fmt.Errorf("decode music task payload: trailing data: %w", asynq.SkipRetry)
		}
		id, err := uuid.Parse(strings.TrimSpace(payload.TaskID))
		if err != nil || id == uuid.Nil {
			return fmt.Errorf("music task payload has invalid task_id: %w", asynq.SkipRetry)
		}
		return run(ctx, id)
	}
}
