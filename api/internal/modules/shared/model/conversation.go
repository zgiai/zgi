package model

import (
	"time"
)

// Conversation shared conversation model
type Conversation struct {
	ID                      string     `gorm:"type:uuid;primaryKey;default:uuid_generate_v4()" json:"id"`
	AppID                   string     `gorm:"type:uuid;not null;index:conversation_app_idx" json:"app_id"`
	ModelProvider           *string    `gorm:"type:varchar(255)" json:"model_provider"`
	ModelID                 *string    `gorm:"type:varchar(255)" json:"model_id"`
	ModelConfig             JSONMap    `gorm:"type:jsonb" json:"model_config"`
	OverrideModelConfigs    *string    `gorm:"type:text" json:"override_model_configs"`
	Mode                    string     `gorm:"type:varchar(255);not null" json:"mode"`
	Name                    string     `gorm:"type:varchar(255);not null" json:"name"`
	Summary                 *string    `gorm:"type:text" json:"summary"`
	Inputs                  JSONMap    `gorm:"type:jsonb" json:"inputs"`
	Introduction            *string    `gorm:"type:text" json:"introduction"`
	SystemInstruction       *string    `gorm:"type:text" json:"system_instruction"`
	SystemInstructionTokens int        `gorm:"type:integer;not null;default:0" json:"system_instruction_tokens"`
	Status                  string     `gorm:"type:varchar(255);not null;default:'normal'" json:"status"`
	FromSource              string     `gorm:"type:varchar(255);not null" json:"from_source"`
	FromEndUserID           *string    `gorm:"type:uuid" json:"from_end_user_id"`
	FromEndUserSessionID    *string    `gorm:"type:varchar(255)" json:"from_end_user_session_id"`
	FromAccountID           *string    `gorm:"type:uuid" json:"from_account_id"`
	FromAccountName         *string    `gorm:"type:varchar(255)" json:"from_account_name"`
	ReadAt                  *time.Time `gorm:"" json:"read_at"`
	ReadAccountID           *string    `gorm:"type:uuid" json:"read_account_id"`
	CreatedAt               time.Time  `gorm:"not null;default:CURRENT_TIMESTAMP(0)" json:"created_at"`
	UpdatedAt               time.Time  `gorm:"not null;default:CURRENT_TIMESTAMP(0)" json:"updated_at"`
	IsDeleted               bool       `gorm:"not null;default:false" json:"is_deleted"`
	InvokeFrom              *string    `gorm:"type:varchar(255)" json:"invoke_from"`

	// Relationships
	Messages     []Message `gorm:"foreignKey:ConversationID" json:"messages,omitempty"`
	FirstMessage *Message  `gorm:"foreignKey:ConversationID;constraint:OnUpdate:CASCADE,OnDelete:SET NULL" json:"first_message,omitempty"`
}

// TableName specifies table name
func (Conversation) TableName() string {
	return "conversations"
}

// IsDeletedStatus Check if conversation is deleted
func (c *Conversation) IsDeletedStatus() bool {
	return c.IsDeleted
}

// GetSummaryOrQuery Get summary or first message query
func (c *Conversation) GetSummaryOrQuery() string {
	if c.Summary != nil && *c.Summary != "" {
		return *c.Summary
	}
	if c.FirstMessage != nil {
		return c.FirstMessage.Query
	}
	return c.Name
}
