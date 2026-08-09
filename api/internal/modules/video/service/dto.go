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

type VideoModel struct {
	Provider   string `json:"provider"`
	Model      string `json:"model"`
	ModelLabel string `json:"model_label"`
}

type GenerateRequest struct {
	Prompt         string          `json:"prompt"`
	Provider       string          `json:"provider"`
	Model          string          `json:"model"`
	Options        GenerateOptions `json:"options"`
	CallbackURL    string          `json:"callback_url,omitempty"`
	ReferenceURL   string          `json:"reference_url,omitempty"`
	ReferenceURLs  []string        `json:"reference_urls,omitempty"`
	ReferenceTypes []string        `json:"reference_types,omitempty"`
	FirstFrameURL  string          `json:"first_frame_url,omitempty"`
	LastFrameURL   string          `json:"last_frame_url,omitempty"`
}

type GenerateOptions struct {
	Ratio         string `json:"ratio,omitempty"`
	Resolution    string `json:"resolution,omitempty"`
	Duration      int    `json:"duration,omitempty"`
	Count         int    `json:"count,omitempty"`
	GenerateAudio bool   `json:"generate_audio,omitempty"`
	PromptExtend  bool   `json:"prompt_extend,omitempty"`
	Watermark     bool   `json:"watermark,omitempty"`
	Voice         string `json:"voice,omitempty"`
}

type VideoTask struct {
	ID               string         `json:"id"`
	TaskID           string         `json:"task_id"`
	UpstreamTaskID   string         `json:"upstream_task_id,omitempty"`
	Provider         string         `json:"provider"`
	Model            string         `json:"model"`
	ModelLabel       string         `json:"model_label,omitempty"`
	Prompt           string         `json:"prompt"`
	Status           string         `json:"status"`
	VideoURL         string         `json:"video_url,omitempty"`
	ErrorMessage     string         `json:"error_message,omitempty"`
	DurationSeconds  int            `json:"duration_seconds,omitempty"`
	Resolution       string         `json:"resolution,omitempty"`
	Ratio            string         `json:"ratio,omitempty"`
	HasInputVideo    bool           `json:"has_input_video"`
	GenerateAudio    bool           `json:"generate_audio"`
	Voice            string         `json:"voice,omitempty"`
	EstimatedCredits int64          `json:"estimated_credits"`
	ActualCredits    int64          `json:"actual_credits"`
	RequestPayload   map[string]any `json:"request_payload,omitempty"`
	ResponsePayload  map[string]any `json:"response_payload,omitempty"`
	CreatedAt        time.Time      `json:"created_at"`
	UpdatedAt        time.Time      `json:"updated_at"`
	CompletedAt      *time.Time     `json:"completed_at,omitempty"`
}

type ListTasksQuery struct {
	Limit  int
	Cursor string
	Search string
}

type ListTasksResult struct {
	Data       []VideoTask `json:"data"`
	Total      int64       `json:"total"`
	HasMore    bool        `json:"has_more"`
	NextCursor string      `json:"next_cursor,omitempty"`
}

type GenerateResult struct {
	Task VideoTask `json:"task"`
}
