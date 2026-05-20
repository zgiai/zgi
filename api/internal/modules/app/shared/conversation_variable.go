package shared

import (
	"time"
)

// ConversationVariable represents conversation variable model
type ConversationVariable struct {
	ID             string    `gorm:"type:uuid;primary_key" json:"id"`
	ConversationID string    `gorm:"type:uuid;not null;primary_key" json:"conversation_id"`
	AppID          string    `gorm:"type:uuid;not null;index:idx_conversation_variables_app_id" json:"app_id"`
	Data           string    `gorm:"type:text;not null" json:"data"`
	CreatedAt      time.Time `gorm:"not null;default:CURRENT_TIMESTAMP;index:idx_conversation_variables_created_at" json:"created_at"`
	UpdatedAt      time.Time `gorm:"not null;default:CURRENT_TIMESTAMP" json:"updated_at"`
}

// TableName specifies table name
func (ConversationVariable) TableName() string {
	return "workflow_conversation_variables"
}
