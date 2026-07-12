package service

import "errors"

var (
	ErrPromptRequired            = errors.New("PROMPT_REQUIRED")
	ErrPromptTooLong             = errors.New("PROMPT_TOO_LONG")
	ErrModelNotAvailable         = errors.New("MODEL_NOT_AVAILABLE")
	ErrModelRouteAmbiguous       = errors.New("MODEL_ROUTE_AMBIGUOUS")
	ErrUnsupportedSize           = errors.New("UNSUPPORTED_SIZE")
	ErrUnsupportedCount          = errors.New("UNSUPPORTED_COUNT")
	ErrConversationNotAccessible = errors.New("CONVERSATION_NOT_ACCESSIBLE")
	ErrUpstreamFailed            = errors.New("UPSTREAM_FAILED")
	ErrImageSaveFailed           = errors.New("IMAGE_SAVE_FAILED")
)
