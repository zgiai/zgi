package model

import (
	"time"

	"github.com/google/uuid"
)

const (
	MessageStatusPending   = "pending"
	MessageStatusStreaming = "streaming"
	MessageStatusCompleted = "completed"
	MessageStatusError     = "error"
	MessageStatusStopped   = "stopped"

	MessageBillingReasonSourceAIChat = "aichat"
)

// Message stores one user query and the final model answer for an AIChat turn.
type Message struct {
	ID                  uuid.UUID              `gorm:"type:uuid;primaryKey" json:"id"`
	ConversationID      uuid.UUID              `gorm:"type:uuid;not null;index:idx_aichat_messages_conversation_created,priority:1" json:"conversation_id"`
	ParentID            *uuid.UUID             `gorm:"type:uuid;index:idx_aichat_messages_parent" json:"parent_id,omitempty"`
	Query               string                 `gorm:"type:text;not null" json:"query"`
	Answer              string                 `gorm:"type:text;not null;default:''" json:"answer"`
	Status              string                 `gorm:"type:varchar(32);not null;default:'pending'" json:"status"`
	Error               *string                `gorm:"type:text" json:"error,omitempty"`
	ModelProvider       *string                `gorm:"type:varchar(255)" json:"model_provider,omitempty"`
	ModelName           string                 `gorm:"type:varchar(255);not null" json:"model_name"`
	BillingReasonSource *string                `gorm:"type:varchar(64)" json:"billing_reason_source,omitempty"`
	ModelParameters     map[string]interface{} `gorm:"type:jsonb;serializer:json;not null;default:'{}'" json:"model_parameters,omitempty"`
	Metadata            map[string]interface{} `gorm:"type:jsonb;serializer:json;not null;default:'{}'" json:"metadata,omitempty"`
	SourceMessageID     *uuid.UUID             `gorm:"type:uuid;uniqueIndex:idx_aichat_messages_source_message" json:"source_message_id,omitempty"`
	CreatedAt           time.Time              `gorm:"not null;default:CURRENT_TIMESTAMP;index:idx_aichat_messages_conversation_created,priority:2" json:"created_at"`
	UpdatedAt           time.Time              `gorm:"not null;default:CURRENT_TIMESTAMP" json:"updated_at"`
	DeletedAt           *time.Time             `gorm:"index" json:"deleted_at,omitempty"`
}

func (Message) TableName() string {
	return "aichat_messages"
}
