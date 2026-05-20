package conversation

import (
	"time"

	"github.com/zgiai/zgi/api/internal/modules/app/chat"
)

// ConversationGroup conversation group model
type ConversationGroup struct {
	ID             string    `gorm:"type:uuid;primaryKey;default:uuid_generate_v4()" json:"id"`
	AppID          string    `gorm:"type:varchar(36);not null" json:"app_id"`
	GroupID        string    `gorm:"type:varchar(36);not null;index" json:"group_id"`
	ConversationID *string   `gorm:"type:varchar(36)" json:"conversation_id"`
	Name           string    `gorm:"type:varchar(255);not null;default:''" json:"name"`
	FromAccountID  string    `gorm:"type:varchar(36);not null" json:"from_account_id"`
	CreatedAt      time.Time `gorm:"not null;default:CURRENT_TIMESTAMP" json:"created_at"`
	UpdatedAt      time.Time `gorm:"not null;default:CURRENT_TIMESTAMP" json:"updated_at"`

	// Relationships
	Conversation *chat.Conversation `gorm:"foreignKey:ConversationID;references:ID" json:"conversation,omitempty"`
}

// TableName specifies table name
func (ConversationGroup) TableName() string {
	return "conversation_group"
}
