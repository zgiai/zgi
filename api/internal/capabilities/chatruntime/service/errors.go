package service

import "errors"

var (
	ErrUnauthorized                = errors.New("unauthorized")
	ErrPermissionDenied            = errors.New("permission denied")
	ErrNotFound                    = errors.New("not found")
	ErrInvalidInput                = errors.New("invalid input")
	ErrInvalidModelParam           = errors.New("invalid model parameter")
	ErrConversationMissing         = errors.New("conversation is required")
	ErrMessageStopped              = errors.New("message stopped")
	ErrConversationRunning         = errors.New("conversation is already streaming")
	ErrConversationWaitingApproval = errors.New("conversation is waiting for workflow approval")
	ErrConversationWaitingQuestion = errors.New("conversation is waiting for workflow question answer")
	ErrConversationWaitingAction   = errors.New("conversation is waiting for client action")
	ErrStreamEventsUnavailable     = errors.New("stream events are unavailable")
	ErrMessageReplaceNotAllowed    = errors.New("message replacement is only allowed for the only root message")
)

type finalizedStreamError struct {
	cause error
}

func (e *finalizedStreamError) Error() string {
	if e == nil || e.cause == nil {
		return ""
	}
	return e.cause.Error()
}

func (e *finalizedStreamError) Unwrap() error {
	if e == nil {
		return nil
	}
	return e.cause
}

func newFinalizedStreamError(cause error) error {
	if cause == nil {
		return nil
	}
	return &finalizedStreamError{cause: cause}
}

func IsFinalizedStreamError(err error) bool {
	var target *finalizedStreamError
	return errors.As(err, &target)
}
