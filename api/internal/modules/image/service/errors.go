package service

import (
	"errors"

	"github.com/zgiai/zgi/api/pkg/apperror"
)

var (
	ErrPromptRequired            = errors.New("PROMPT_REQUIRED")
	ErrPromptTooLong             = errors.New("PROMPT_TOO_LONG")
	ErrModelNotAvailable         = errors.New("MODEL_NOT_AVAILABLE")
	ErrParameterNotSupported     = errors.New("IMAGE_PARAMETER_NOT_SUPPORTED")
	ErrUnsupportedSize           = errors.New("UNSUPPORTED_SIZE")
	ErrUnsupportedCount          = errors.New("UNSUPPORTED_COUNT")
	ErrGenerationModeInvalid     = errors.New("IMAGE_GENERATION_MODE_INVALID")
	ErrMaxImagesRequired         = errors.New("IMAGE_MAX_IMAGES_REQUIRED")
	ErrMaxImagesNotAllowed       = errors.New("IMAGE_MAX_IMAGES_NOT_ALLOWED")
	ErrMaxImagesOutOfRange       = errors.New("IMAGE_MAX_IMAGES_OUT_OF_RANGE")
	ErrConversationNotAccessible = errors.New("CONVERSATION_NOT_ACCESSIBLE")
	ErrBillingContextRequired    = errors.New("BILLING_CONTEXT_REQUIRED")
	ErrUpstreamFailed            = errors.New("UPSTREAM_FAILED")
	ErrTaskTimeout               = errors.New("IMAGE_TASK_TIMEOUT")
	ErrImageSaveFailed           = errors.New("IMAGE_SAVE_FAILED")
	ErrReferenceImageRequired    = errors.New("REFERENCE_IMAGE_REQUIRED")
	ErrReferenceImageInvalid     = errors.New("REFERENCE_IMAGE_INVALID")
	ErrReferenceImageUnsupported = errors.New("REFERENCE_IMAGE_UNSUPPORTED")
	ErrTaskNotFound              = errors.New("image task not found")
	ErrTaskConflict              = errors.New("image task conflict")
	ErrSearchTooLong             = errors.New("image task search too long")
	ErrInvalidCursor             = errors.New("image task cursor invalid")
)

var (
	AppCodeTaskNotFound  = apperror.MustCode("image.task.not_found")
	AppCodeTaskConflict  = apperror.MustCode("image.task.conflict")
	AppCodeSearchTooLong = apperror.MustCode("image.search.too_long")
	AppCodeInvalidCursor = apperror.MustCode("image.cursor.invalid")
)
