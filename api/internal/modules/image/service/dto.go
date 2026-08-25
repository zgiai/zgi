package service

import (
	"time"

	"github.com/google/uuid"
)

type Scope struct {
	OrganizationID uuid.UUID
	AccountID      uuid.UUID
	WorkspaceID    *uuid.UUID
}

type GenerateRequest struct {
	Prompt          string          `json:"prompt"`
	Provider        string          `json:"provider"`
	Model           string          `json:"model"`
	ClientRequestID string          `json:"client_request_id,omitempty"`
	Options         GenerateOptions `json:"options"`
	ConversationID  string          `json:"conversation_id"`
	ReferenceImage  *ReferenceImage `json:"reference_image,omitempty"`
}

type GenerateOptions struct {
	Size           string `json:"size,omitempty"`
	Count          *int   `json:"count,omitempty"`
	GenerationMode string `json:"generation_mode,omitempty"`
	MaxImages      *int   `json:"max_images,omitempty"`
}

type ReferenceImage struct {
	FileID   string `json:"file_id"`
	URL      string `json:"url,omitempty"`
	Filename string `json:"filename,omitempty"`
	MimeType string `json:"mime_type,omitempty"`
}

type ImageFile struct {
	FileID         string `json:"file_id"`
	ToolFileID     string `json:"tool_file_id,omitempty"`
	URL            string `json:"url"`
	DownloadURL    string `json:"download_url"`
	Filename       string `json:"filename"`
	Extension      string `json:"extension"`
	MimeType       string `json:"mime_type"`
	TransferMethod string `json:"transfer_method,omitempty"`
	Lifecycle      string `json:"lifecycle,omitempty"`
	ExpiresAt      *int64 `json:"expires_at,omitempty"`
}

type ImageGenerationMetadata struct {
	Provider       string          `json:"provider"`
	Model          string          `json:"model"`
	ModelLabel     string          `json:"model_label"`
	Size           string          `json:"size"`
	Count          int             `json:"count"`
	GenerationMode string          `json:"generation_mode,omitempty"`
	MaxImages      *int            `json:"max_images,omitempty"`
	Files          []ImageFile     `json:"files"`
	ReferenceImage *ReferenceImage `json:"reference_image,omitempty"`
	Status         string          `json:"status"`
}

type GenerateResult struct {
	ConversationID  string                  `json:"conversation_id"`
	MessageID       string                  `json:"message_id"`
	Message         string                  `json:"message"`
	ImageGeneration ImageGenerationMetadata `json:"image_generation"`
}

type ImageTask struct {
	ID              string                   `json:"id"`
	TaskID          string                   `json:"task_id"`
	ClientRequestID string                   `json:"client_request_id,omitempty"`
	ConversationID  string                   `json:"conversation_id,omitempty"`
	MessageID       string                   `json:"message_id,omitempty"`
	Provider        string                   `json:"provider"`
	Model           string                   `json:"model"`
	ModelLabel      string                   `json:"model_label,omitempty"`
	Prompt          string                   `json:"prompt"`
	Status          string                   `json:"status"`
	Size            string                   `json:"size,omitempty"`
	Count           int                      `json:"count"`
	GenerationMode  string                   `json:"generation_mode,omitempty"`
	MaxImages       *int                     `json:"max_images,omitempty"`
	Files           []ImageFile              `json:"files,omitempty"`
	ReferenceImage  *ReferenceImage          `json:"reference_image,omitempty"`
	ErrorMessage    string                   `json:"error_message,omitempty"`
	RequestPayload  map[string]any           `json:"request_payload,omitempty"`
	ResponsePayload map[string]any           `json:"response_payload,omitempty"`
	ImageGeneration *ImageGenerationMetadata `json:"image_generation,omitempty"`
	CreatedAt       time.Time                `json:"created_at"`
	UpdatedAt       time.Time                `json:"updated_at"`
	CompletedAt     *time.Time               `json:"completed_at,omitempty"`
}

type ListTasksQuery struct {
	Limit  int
	Cursor string
	Search string
}

type ListTasksResult struct {
	Data       []ImageTask `json:"data"`
	Total      int64       `json:"total"`
	HasMore    bool        `json:"has_more"`
	NextCursor string      `json:"next_cursor,omitempty"`
}

type CreateTaskResult struct {
	Task ImageTask `json:"task"`
}
