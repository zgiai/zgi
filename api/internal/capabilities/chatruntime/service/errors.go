package service

import (
	"errors"

	"github.com/google/uuid"
)

var (
	ErrUnauthorized                = errors.New("unauthorized")
	ErrPermissionDenied            = errors.New("permission denied")
	ErrNotFound                    = errors.New("not found")
	ErrInvalidInput                = errors.New("invalid input")
	ErrInvalidModelParam           = errors.New("invalid model parameter")
	ErrConversationMissing         = errors.New("conversation is required")
	ErrMessageStopped              = errors.New("message stopped")
	ErrConversationRunning         = errors.New("conversation is already streaming")
	ErrConversationIdle            = errors.New("conversation has no active runtime")
	ErrRuntimeInputDisabled        = errors.New("runtime input is disabled for this run")
	ErrConversationWaitingApproval = errors.New("conversation is waiting for workflow approval")
	ErrConversationWaitingQuestion = errors.New("conversation is waiting for an answer to a question")
	ErrConversationWaitingAction   = errors.New("conversation is waiting for client action")
	ErrConversationWaitingBudget   = errors.New("conversation is waiting for token budget continuation")
	ErrStreamEventsUnavailable     = errors.New("stream events are unavailable")
	ErrContinuationAlreadyRunning  = errors.New("continuation is already running")
	ErrMessageReplaceNotAllowed    = errors.New("message replacement is only allowed for the only root message")
	ErrModelIdleTimeout            = errors.New("model idle timeout")
	ErrWorkflowBindingUnavailable  = errors.New("workflow binding unavailable")
)

// ConversationWaitingError names the message a new turn is blocked on, so a
// caller can continue that message instead. It wraps one of the
// ErrConversationWaiting* errors.
type ConversationWaitingError struct {
	Cause     error
	MessageID uuid.UUID
	Status    string
	// RequestID is the pending user-input request when the message waits for
	// an answer to a question.
	RequestID string
}

func (e *ConversationWaitingError) Error() string { return e.Cause.Error() }

func (e *ConversationWaitingError) Unwrap() error { return e.Cause }

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

func newContinuationAlreadyRunningError(message string) error {
	if message == "" {
		message = "reconnect to the active stream instead of retrying the action"
	}
	return errors.Join(ErrInvalidInput, ErrContinuationAlreadyRunning, errors.New(message))
}

func IsContinuationAlreadyRunningError(err error) bool {
	return errors.Is(err, ErrContinuationAlreadyRunning)
}
