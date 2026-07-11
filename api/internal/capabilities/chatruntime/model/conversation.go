package model

import (
	"time"

	"github.com/google/uuid"
)

const (
	ConversationStatusNormal   = "normal"
	ConversationStatusArchived = "archived"

	ConversationRuntimeStatusIdle      = "idle"
	ConversationRuntimeStatusStreaming = "streaming"

	ConversationSourceConsole     = "console"
	ConversationSourceWebApp      = "webapp"
	ConversationSourceExternalAPI = "external-api"
	ConversationSourceMigration   = "migration"

	ConversationCallerAIChat = "aichat"
	ConversationCallerAgent  = "agent"
)

// Conversation stores one AIChat conversation owned by an organization member.
type Conversation struct {
	ID                   uuid.UUID              `gorm:"type:uuid;primaryKey" json:"id"`
	OrganizationID       uuid.UUID              `gorm:"type:uuid;not null;index:idx_chat_runtime_conversations_owner_updated,priority:1" json:"organization_id"`
	WorkspaceID          *uuid.UUID             `gorm:"type:uuid;index:idx_chat_runtime_conversations_workspace" json:"workspace_id,omitempty"`
	AccountID            uuid.UUID              `gorm:"type:uuid;not null;index:idx_chat_runtime_conversations_owner_updated,priority:2" json:"account_id"`
	CallerType           string                 `gorm:"type:varchar(32);not null;default:'aichat';index:idx_chat_runtime_conversations_caller_updated,priority:3" json:"caller_type"`
	CallerID             *uuid.UUID             `gorm:"type:uuid;index:idx_chat_runtime_conversations_caller_updated,priority:4" json:"caller_id,omitempty"`
	Title                string                 `gorm:"type:varchar(255);not null" json:"title"`
	Status               string                 `gorm:"type:varchar(32);not null;default:'normal'" json:"status"`
	RuntimeStatus        string                 `gorm:"type:varchar(32);not null;default:'idle';index:idx_chat_runtime_conversations_runtime_status" json:"runtime_status"`
	CurrentLeafMessageID *uuid.UUID             `gorm:"type:uuid" json:"current_leaf_message_id,omitempty"`
	ActiveMessageID      *uuid.UUID             `gorm:"type:uuid;index:idx_chat_runtime_conversations_active_message" json:"active_message_id,omitempty"`
	DialogueCount        int                    `gorm:"not null;default:0" json:"dialogue_count"`
	Source               string                 `gorm:"type:varchar(32);not null;default:'console'" json:"source"`
	SourceConversationID *uuid.UUID             `gorm:"type:uuid;uniqueIndex:idx_chat_runtime_conversations_source_conversation" json:"source_conversation_id,omitempty"`
	SourceWebAppID       *uuid.UUID             `gorm:"type:uuid;index:idx_chat_runtime_conversations_source_web_app" json:"source_web_app_id,omitempty"`
	Metadata             map[string]interface{} `gorm:"type:jsonb;serializer:json;not null;default:'{}'" json:"metadata,omitempty"`
	CreatedAt            time.Time              `gorm:"not null;default:CURRENT_TIMESTAMP" json:"created_at"`
	UpdatedAt            time.Time              `gorm:"not null;default:CURRENT_TIMESTAMP;index:idx_chat_runtime_conversations_owner_updated,priority:3;index:idx_chat_runtime_conversations_caller_updated,priority:5" json:"updated_at"`
	DeletedAt            *time.Time             `gorm:"index" json:"deleted_at,omitempty"`
}

func (Conversation) TableName() string {
	return "chat_runtime_conversations"
}
