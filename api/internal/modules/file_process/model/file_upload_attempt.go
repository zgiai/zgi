package model

import "time"

type FileUploadAttemptStatus string

const (
	FileUploadAttemptPending   FileUploadAttemptStatus = "pending"
	FileUploadAttemptCompleted FileUploadAttemptStatus = "completed"
	FileUploadAttemptClaimed   FileUploadAttemptStatus = "claimed"
	FileUploadAttemptCancelled FileUploadAttemptStatus = "cancelled"
)

// FileUploadAttempt tracks only chat attachment uploads that have an explicit client
// attempt id. It deliberately does not use is_temporary as a retention signal: ordinary
// temporary files remain retained by default.
type FileUploadAttempt struct {
	ID        string                  `gorm:"column:id;type:uuid;primaryKey"`
	OwnerID   string                  `gorm:"column:owner_id;type:varchar(255);not null"`
	FileID    string                  `gorm:"column:file_id;type:varchar(255);not null"`
	ObjectKey string                  `gorm:"column:object_key;type:varchar(1024);not null"`
	Status    FileUploadAttemptStatus `gorm:"column:status;type:varchar(32);not null"`
	ExpiresAt time.Time               `gorm:"column:expires_at;not null"`
	CreatedAt time.Time               `gorm:"column:created_at;not null"`
	UpdatedAt time.Time               `gorm:"column:updated_at;not null"`
}

func (FileUploadAttempt) TableName() string { return "file_upload_attempts" }
