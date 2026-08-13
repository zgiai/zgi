package music

import (
	"context"
	"sort"
	"strings"
	"time"

	"github.com/google/uuid"
)

type memoryRepository struct {
	tasks map[uuid.UUID]*Task
}

func newMemoryRepository(tasks ...*Task) *memoryRepository {
	repo := &memoryRepository{tasks: make(map[uuid.UUID]*Task)}
	for _, task := range tasks {
		copy := *task
		repo.tasks[task.ID] = &copy
	}
	return repo
}

func (r *memoryRepository) Create(_ context.Context, task *Task) error {
	copy := *task
	r.tasks[task.ID] = &copy
	return nil
}

func (r *memoryRepository) Get(_ context.Context, id uuid.UUID) (*Task, error) {
	task, ok := r.tasks[id]
	if !ok {
		return nil, ErrTaskNotFound
	}
	copy := *task
	return &copy, nil
}

func (r *memoryRepository) GetScoped(_ context.Context, scope Scope, id uuid.UUID) (*Task, error) {
	task, err := r.Get(context.Background(), id)
	if err != nil || task.OrganizationID != scope.OrganizationID || task.WorkspaceID != scope.WorkspaceID || task.AccountID != scope.AccountID {
		return nil, ErrTaskNotFound
	}
	return task, nil
}

func (r *memoryRepository) GetByRequest(_ context.Context, scope Scope, requestID uuid.UUID) (*Task, error) {
	for _, task := range r.tasks {
		if task.RequestID == requestID && task.OrganizationID == scope.OrganizationID &&
			task.WorkspaceID == scope.WorkspaceID && task.AccountID == scope.AccountID {
			copy := *task
			return &copy, nil
		}
	}
	return nil, ErrTaskNotFound
}

func (r *memoryRepository) UpdateGeneratedLyrics(_ context.Context, id uuid.UUID, generated GeneratedLyrics) error {
	task, ok := r.tasks[id]
	if !ok {
		return ErrTaskNotFound
	}
	if task.Status != StatusGeneratingLyrics {
		return ErrInvalidTransition
	}
	task.Title = generated.Title
	task.StyleTags = append(task.StyleTags[:0], generated.StyleTags...)
	task.Lyrics = generated.Lyrics
	task.UpdatedAt = time.Now()
	return nil
}

func (r *memoryRepository) ListScoped(_ context.Context, scope Scope, query ListQuery) ([]*Task, int64, error) {
	if query.Page <= 0 || query.PageSize <= 0 {
		return nil, 0, ErrInvalidRequest
	}
	tasks := make([]*Task, 0)
	search := strings.ToLower(query.Search)
	for _, task := range r.tasks {
		if task.OrganizationID != scope.OrganizationID || task.WorkspaceID != scope.WorkspaceID || task.AccountID != scope.AccountID {
			continue
		}
		if search != "" && !strings.Contains(strings.ToLower(task.Prompt), search) {
			continue
		}
		copy := *task
		tasks = append(tasks, &copy)
	}
	sort.Slice(tasks, func(i, j int) bool {
		return tasks[i].CreatedAt.After(tasks[j].CreatedAt)
	})
	total := int64(len(tasks))
	start := (query.Page - 1) * query.PageSize
	if start >= len(tasks) {
		return []*Task{}, total, nil
	}
	end := min(start+query.PageSize, len(tasks))
	return tasks[start:end], total, nil
}

func (r *memoryRepository) Transition(_ context.Context, id uuid.UUID, from, to Status, update TaskUpdate) error {
	task, ok := r.tasks[id]
	if !ok {
		return ErrTaskNotFound
	}
	if task.Status != from {
		return ErrInvalidTransition
	}
	task.Status = to
	if update.FileID != nil {
		task.FileID = update.FileID
	}
	if update.DurationMS > 0 {
		task.DurationMS = update.DurationMS
	}
	if len(update.WaveformPeaks) > 0 {
		task.WaveformPeaks = append(task.WaveformPeaks[:0], update.WaveformPeaks...)
	}
	if update.ErrorCode != "" {
		task.ErrorCode = update.ErrorCode
	}
	if update.ErrorMessage != "" {
		task.ErrorMessage = update.ErrorMessage
	}
	now := time.Now()
	task.UpdatedAt = now
	if to == StatusGenerating {
		task.StartedAt = &now
	}
	if to == StatusSucceeded || to == StatusFailed {
		task.CompletedAt = &now
	}
	return nil
}

func (r *memoryRepository) ListIDsByStatus(_ context.Context, status Status, updatedBefore time.Time, limit int) ([]uuid.UUID, error) {
	if !isReconciledStatus(status) || updatedBefore.IsZero() || limit <= 0 {
		return nil, ErrInvalidRequest
	}
	ids := make([]uuid.UUID, 0)
	for id, task := range r.tasks {
		if task.Status == status && !task.UpdatedAt.After(updatedBefore) {
			ids = append(ids, id)
		}
	}
	sort.Slice(ids, func(i, j int) bool {
		return r.tasks[ids[i]].UpdatedAt.Before(r.tasks[ids[j]].UpdatedAt)
	})
	if len(ids) > limit {
		ids = ids[:limit]
	}
	return ids, nil
}

func (r *memoryRepository) TouchStatus(_ context.Context, id uuid.UUID, status Status) error {
	task, ok := r.tasks[id]
	if !ok {
		return ErrTaskNotFound
	}
	if !isReconciledStatus(status) || task.Status != status {
		return ErrInvalidTransition
	}
	task.UpdatedAt = time.Now()
	return nil
}

func (r *memoryRepository) DeleteScopedTerminal(_ context.Context, scope Scope, id uuid.UUID) error {
	task, err := r.GetScoped(context.Background(), scope, id)
	if err != nil {
		return err
	}
	if task.Status != StatusSucceeded && task.Status != StatusFailed {
		return ErrTaskNotDeletable
	}
	delete(r.tasks, id)
	return nil
}

type dispatcherStub struct {
	generated      uuid.UUID
	generateCalls  int
	compensated    uuid.UUID
	compensatedIDs []uuid.UUID
	generateErr    error
	compensateErr  error
}

func (d *dispatcherStub) EnqueueGeneration(_ context.Context, id uuid.UUID) error {
	d.generated = id
	d.generateCalls++
	return d.generateErr
}

func (d *dispatcherStub) EnqueueCompensation(_ context.Context, id uuid.UUID) error {
	d.compensated = id
	d.compensatedIDs = append(d.compensatedIDs, id)
	return d.compensateErr
}

func (d *dispatcherStub) hasCompensation(id uuid.UUID) bool {
	for _, candidate := range d.compensatedIDs {
		if candidate == id {
			return true
		}
	}
	return false
}
