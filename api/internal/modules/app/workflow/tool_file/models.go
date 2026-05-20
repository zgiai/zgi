package tool_file

import (
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

type ToolFileLifecycle string

const (
	ToolFileLifecyclePersistent ToolFileLifecycle = "persistent"
	ToolFileLifecycleTemporary  ToolFileLifecycle = "temporary"
)

// ToolFile represents a file stored in the tool file system
type ToolFile struct {
	ID             string     `gorm:"primaryKey" json:"id"`
	UserID         string     `gorm:"not null" json:"user_id"`
	TenantID       string     `gorm:"not null" json:"tenant_id"`
	ConversationID *string    `json:"conversation_id"`
	FileKey        string     `gorm:"not null" json:"file_key"`
	MimeType       string     `gorm:"column:mimetype;not null" json:"mimetype"`
	OriginalURL    *string    `json:"original_url"`
	Name           string     `gorm:"not null" json:"name"`
	Size           int64      `gorm:"not null;default:-1" json:"size"`
	Lifecycle      string     `gorm:"not null;default:persistent" json:"lifecycle"`
	ExpiresAt      *time.Time `json:"expires_at,omitempty"`
	CreatedAt      time.Time  `gorm:"not null" json:"created_at"`
	UpdatedAt      time.Time  `gorm:"not null" json:"updated_at"`
	DeletedAt      *time.Time `gorm:"index" json:"deleted_at,omitempty"`
}

// TableName specifies the table name for GORM
func (ToolFile) TableName() string {
	return "tool_files"
}

// BeforeCreate sets up the ID and timestamps before creating
func (tf *ToolFile) BeforeCreate(tx *gorm.DB) error {
	if tf.ID == "" {
		tf.ID = uuid.New().String()
	}
	now := time.Now()
	tf.CreatedAt = now
	tf.UpdatedAt = now
	return nil
}

// BeforeUpdate sets up the updated timestamp before updating
func (tf *ToolFile) BeforeUpdate(tx *gorm.DB) error {
	tf.UpdatedAt = time.Now()
	return nil
}

// IsValid checks if the tool file is valid
func (tf *ToolFile) IsValid() bool {
	return tf.ID != "" && tf.UserID != "" && tf.TenantID != "" && tf.FileKey != "" && tf.MimeType != ""
}

func (tf *ToolFile) LifecycleValue() ToolFileLifecycle {
	switch ToolFileLifecycle(tf.Lifecycle) {
	case ToolFileLifecycleTemporary:
		return ToolFileLifecycleTemporary
	default:
		return ToolFileLifecyclePersistent
	}
}

// GetFileExtension returns the file extension from the file key
func (tf *ToolFile) GetFileExtension() string {
	if len(tf.FileKey) == 0 {
		return ""
	}

	// Find the last dot in the file key
	for i := len(tf.FileKey) - 1; i >= 0; i-- {
		if tf.FileKey[i] == '.' {
			return tf.FileKey[i:]
		}
	}
	return ""
}

// GetStoragePath returns the storage path for this file
func (tf *ToolFile) GetStoragePath() string {
	return tf.FileKey
}
