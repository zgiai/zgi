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
	_, err = r.control.SuspendBinding(ctx, Scope{
		OrganizationID: binding.OrganizationID,
		SubjectID:      binding.SubjectID,
	}, binding.ID, binding.Generation)
	if errors.Is(err, ErrGenerationConflict) || errors.Is(err, ErrNotFound) || errors.Is(err, ErrInvalidTransition) {
		return true, nil
	}
	return true, err
}
