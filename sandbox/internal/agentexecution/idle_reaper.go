package agentexecution

import (
	"context"
	"errors"
	"fmt"
	"time"
)

type BindingSuspender interface {
	SuspendBinding(context.Context, Scope, string, int64) (Binding, error)
}

// IdleSuspendRetryDelay is persisted through the suspending Binding's UpdatedAt.
// Repositories exclude recent attempts before applying their result limit, so a
// failing oldest Binding cannot starve the rest of the expired queue.
const IdleSuspendRetryDelay = 15 * time.Minute

// Preserve the existing archive transport's ten-minute budget. The retry delay
// is longer than this deadline, avoiding a second reaper retry during capture.
const idleSuspendTimeout = 10 * time.Minute

type IdleReaperOptions struct {
	Now func() time.Time
}

// IdleReaper releases compute for expired Bindings. The observed generation is
// passed to SuspendBinding so concurrent data-plane activity fences a stale
// reap attempt instead of suspending an environment that just became active.
type IdleReaper struct {
	repository ExpiredBindingRepository
	control    BindingSuspender
	now        func() time.Time
}

func NewIdleReaper(repository ExpiredBindingRepository, control BindingSuspender, options IdleReaperOptions) (*IdleReaper, error) {
	if repository == nil || control == nil {
		return nil, fmt.Errorf("%w: expiry repository and binding control are required", ErrInvalidRequest)
	}
	if options.Now == nil {
		options.Now = func() time.Time { return time.Now().UTC() }
	}
	return &IdleReaper{repository: repository, control: control, now: options.Now}, nil
}

func (r *IdleReaper) RunOne(ctx context.Context) (bool, error) {
	bindings, err := r.repository.ExpiredBindings(ctx, r.now().UTC(), 1)
	if err != nil {
		return false, err
	}
	if len(bindings) == 0 {
		return false, nil
	}
	binding := bindings[0]
	attemptContext, cancel := context.WithTimeout(ctx, idleSuspendTimeout)
	defer cancel()
	_, err = r.control.SuspendBinding(attemptContext, Scope{
		OrganizationID: binding.OrganizationID,
		SubjectID:      binding.SubjectID,
		Identity:       cloneExecutionIdentity(binding.Identity),
	}, binding.ID, binding.Generation)
	if errors.Is(err, ErrGenerationConflict) || errors.Is(err, ErrNotFound) || errors.Is(err, ErrInvalidTransition) {
		return true, nil
	}
	return true, err
}
