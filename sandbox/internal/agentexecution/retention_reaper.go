package agentexecution

import (
	"context"
	"errors"
	"fmt"
	"time"
)

type BindingDestroyer interface {
	DestroyBinding(context.Context, Scope, string, int64) (Binding, error)
}

// RetentionReaper removes expired suspended workspaces after the seven-day
// recovery window. It is enabled only when durable checkpoint storage is
// configured; the generation fence protects a binding resumed concurrently.
type RetentionReaper struct {
	repository RetainedBindingRepository
	control    BindingDestroyer
	now        func() time.Time
}

func NewRetentionReaper(repository RetainedBindingRepository, control BindingDestroyer, now func() time.Time) (*RetentionReaper, error) {
	if repository == nil || control == nil {
		return nil, fmt.Errorf("%w: retention repository and binding control are required", ErrInvalidRequest)
	}
	if now == nil {
		now = func() time.Time { return time.Now().UTC() }
	}
	return &RetentionReaper{repository: repository, control: control, now: now}, nil
}

func (r *RetentionReaper) RunOne(ctx context.Context) (bool, error) {
	bindings, err := r.repository.ExpiredRetainedBindings(ctx, r.now().UTC(), 1)
	if err != nil || len(bindings) == 0 {
		return false, err
	}
	binding := bindings[0]
	_, err = r.control.DestroyBinding(ctx, Scope{
		OrganizationID: binding.OrganizationID,
		SubjectID:      binding.SubjectID,
		Identity:       cloneExecutionIdentity(binding.Identity),
	}, binding.ID, binding.Generation)
	if errors.Is(err, ErrGenerationConflict) || errors.Is(err, ErrNotFound) || errors.Is(err, ErrInvalidTransition) {
		return true, nil
	}
	return true, err
}
