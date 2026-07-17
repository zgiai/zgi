package service

import "github.com/google/uuid"

type Scope struct {
	OrganizationID uuid.UUID
	AccountID      uuid.UUID
	WorkspaceID    *uuid.UUID
}

type GenerateRequest struct {
	Prompt         string `json:"prompt"`
	Provider       string `json:"provider"`
	Model          string `json:"model"`
	Size           string `json:"size"`
	Count          int    `json:"count"`
	ConversationID string `json:"conversation_id"`
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
	Provider   string      `json:"provider"`
	Model      string      `json:"model"`
	ModelLabel string      `json:"model_label"`
	Size       string      `json:"size"`
	Count      int         `json:"count"`
	Files      []ImageFile `json:"files"`
	Status     string      `json:"status"`
}

type GenerateResult struct {
	ConversationID  string                  `json:"conversation_id"`
	MessageID       string                  `json:"message_id"`
	Message         string                  `json:"message"`
	ImageGeneration ImageGenerationMetadata `json:"image_generation"`
}
