package service

import "errors"

var (
	ErrUnauthorized             = errors.New("unauthorized")
	ErrPermissionDenied         = errors.New("permission denied")
	ErrNotFound                 = errors.New("not found")
	ErrInvalidInput             = errors.New("invalid input")
	ErrInvalidModelParam        = errors.New("invalid model parameter")
	ErrConversationMissing      = errors.New("conversation is required")
	ErrMessageStopped           = errors.New("message stopped")
	ErrConversationRunning      = errors.New("conversation is already streaming")
	ErrStreamEventsUnavailable  = errors.New("stream events are unavailable")
	ErrMessageReplaceNotAllowed = errors.New("message replacement is only allowed for the only root message")
)
