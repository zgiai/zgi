package music

import (
	"errors"
	"time"

	"github.com/google/uuid"
	adapter "github.com/zgiai/zgi/api/internal/modules/llm/protocol/adapters"
	"gorm.io/datatypes"
)

type Status string

const (
	StatusQueued              Status = "queued"
	StatusGeneratingLyrics    Status = "generating_lyrics"
	StatusGenerating          Status = "generating"
	StatusSucceeded           Status = "succeeded"
	StatusFailed              Status = "failed"
	StatusCompensationPending Status = "compensation_pending"
)

const (
	ErrorCodeQueueUnavailable       = "queue_unavailable"
	ErrorCodeLyricsGenerationFailed = "lyrics_generation_failed"
	ErrorCodeGenerationFailed       = "generation_failed"
	ErrorCodeDeliveryFailed         = "delivery_failed"
	ErrorCodeDeliveryUnknown        = "delivery_unknown"
	ErrorCodeDeliveryFailedRefunded = "delivery_failed_refunded"
)

var (
	ErrInvalidRequest    = errors.New("invalid music request")
	ErrModelUnavailable  = errors.New("music model is unavailable")
	ErrTaskNotFound      = errors.New("music task not found")
	ErrTaskConflict      = errors.New("music request ID is already in use")
	ErrTaskNotDeletable  = errors.New("music task is not deletable")
	ErrTaskAssetMissing  = errors.New("music task asset is missing")
	ErrInvalidTransition = errors.New("invalid music task transition")
)

func isDeletableStatus(status Status) bool {
	return status == StatusSucceeded || status == StatusFailed
}

type Scope struct {
	OrganizationID uuid.UUID
	WorkspaceID    *uuid.UUID
	AccountID      uuid.UUID
}

func validScope(scope Scope) bool {
	return scope.OrganizationID != uuid.Nil && scope.AccountID != uuid.Nil &&
		(scope.WorkspaceID == nil || *scope.WorkspaceID != uuid.Nil)
}

func sameWorkspace(left, right *uuid.UUID) bool {
	if left == nil || right == nil {
		return left == nil && right == nil
	}
	return *left == *right
}

type CreateRequest struct {
	RequestID uuid.UUID         `json:"request_id"`
	Model     string            `json:"model"`
	Mode      adapter.MusicMode `json:"mode"`
	Prompt    string            `json:"prompt"`
	Lyrics    string            `json:"lyrics"`
}

type ListRequest struct {
	Page     int
	PageSize int
	Search   string
}

type ListQuery struct {
	Page     int
	PageSize int
	Search   string
}

type Task struct {
	ID             uuid.UUID                   `gorm:"type:uuid;primaryKey" json:"id"`
	OrganizationID uuid.UUID                   `gorm:"type:uuid;not null;index:idx_music_tasks_scope_created,priority:1;uniqueIndex:idx_music_tasks_request,priority:1" json:"-"`
	WorkspaceID    *uuid.UUID                  `gorm:"type:uuid;index:idx_music_tasks_scope_created,priority:2;uniqueIndex:idx_music_tasks_request,priority:2" json:"-"`
	AccountID      uuid.UUID                   `gorm:"type:uuid;not null;uniqueIndex:idx_music_tasks_request,priority:3" json:"-"`
	RequestID      uuid.UUID                   `gorm:"type:uuid;not null;uniqueIndex:idx_music_tasks_request,priority:4" json:"-"`
	Model          string                      `gorm:"type:varchar(255);not null" json:"model"`
	Mode           adapter.MusicMode           `gorm:"type:varchar(32);not null" json:"mode"`
	Prompt         string                      `gorm:"type:text;not null" json:"prompt"`
	Lyrics         string                      `gorm:"type:text;not null" json:"lyrics,omitempty"`
	Title          string                      `gorm:"type:varchar(255);not null;default:''" json:"title,omitempty"`
	StyleTags      datatypes.JSONSlice[string] `gorm:"type:jsonb;not null;default:'[]'" json:"style_tags"`
	ResponseFormat string                      `gorm:"type:varchar(16);not null" json:"response_format"`
	Status         Status                      `gorm:"type:varchar(32);not null;index:idx_music_tasks_status_updated,priority:1" json:"status"`
	FileID         *uuid.UUID                  `gorm:"type:uuid" json:"file_id,omitempty"`
	DurationMS     int64                       `gorm:"type:bigint;not null;default:0" json:"duration_ms"`
	WaveformPeaks  datatypes.JSONSlice[int16]  `gorm:"type:jsonb;not null;default:'[]'" json:"waveform_peaks"`
	ErrorCode      string                      `gorm:"type:varchar(64);not null;default:''" json:"error_code,omitempty"`
	ErrorMessage   string                      `gorm:"type:varchar(255);not null;default:''" json:"error_message,omitempty"`
	CreatedAt      time.Time                   `gorm:"not null;index:idx_music_tasks_scope_created,priority:3,sort:desc" json:"created_at"`
	UpdatedAt      time.Time                   `gorm:"not null;index:idx_music_tasks_status_updated,priority:2" json:"updated_at"`
	StartedAt      *time.Time                  `json:"started_at,omitempty"`
	CompletedAt    *time.Time                  `json:"completed_at,omitempty"`
}

func (Task) TableName() string { return "music_generation_tasks" }

type TaskUpdate struct {
	FileID        *uuid.UUID
	DurationMS    int64
	WaveformPeaks []int16
	ErrorCode     string
	ErrorMessage  string
}

type GeneratedLyrics struct {
	Title     string
	StyleTags []string
	Lyrics    string
}

type TaskView struct {
	ID             uuid.UUID         `json:"id"`
	Model          string            `json:"model"`
	Mode           adapter.MusicMode `json:"mode"`
	Prompt         string            `json:"prompt"`
	Lyrics         string            `json:"lyrics,omitempty"`
	Title          string            `json:"title,omitempty"`
	StyleTags      []string          `json:"style_tags"`
	ResponseFormat string            `json:"response_format"`
	Status         Status            `json:"status"`
	FileID         *uuid.UUID        `json:"file_id,omitempty"`
	URL            string            `json:"url,omitempty"`
	DurationMS     int64             `json:"duration_ms"`
	WaveformPeaks  []int16           `json:"waveform_peaks"`
	ErrorCode      string            `json:"error_code,omitempty"`
	ErrorMessage   string            `json:"error_message,omitempty"`
	CreatedAt      time.Time         `json:"created_at"`
	UpdatedAt      time.Time         `json:"updated_at"`
	StartedAt      *time.Time        `json:"started_at,omitempty"`
	CompletedAt    *time.Time        `json:"completed_at,omitempty"`
}

type TaskList struct {
	Items    []*TaskView `json:"items"`
	Total    int64       `json:"total"`
	Page     int         `json:"page"`
	PageSize int         `json:"page_size"`
	HasMore  bool        `json:"has_more"`
}
