package memory

import (
	"time"

	"github.com/google/uuid"
	"gorm.io/datatypes"
	"gorm.io/gorm"
)

const (
	SkillID    = "user-memory"
	ProviderID = "user-memory"

	CategoryPreference  = "preference"
	CategoryProfile     = "profile"
	CategoryInstruction = "instruction"
	CategoryFact        = "fact"
	CategoryOther       = "other"

	MemoryTypeLongTerm  = "long_term"
	MemoryTypeTemporary = "temporary"

	EventActionCreate  = "create"
	EventActionUpdate  = "update"
	EventActionDelete  = "delete"
	EventActionEnable  = "enable"
	EventActionDisable = "disable"

	EventActorUser   = "user"
	EventActorModel  = "model"
	EventActorSystem = "system"

	EventSourceAPI      = "api"
	EventSourceAIChat   = "aichat"
	EventSourceWorkflow = "workflow"
)

type AccountMemorySetting struct {
	AccountID uuid.UUID `gorm:"type:uuid;primaryKey" json:"account_id"`
	Enabled   bool      `gorm:"not null;default:false" json:"enabled"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

func (AccountMemorySetting) TableName() string {
	return "account_memory_settings"
}

type AccountMemoryEntry struct {
	ID         uuid.UUID  `gorm:"type:uuid;default:uuid_generate_v4();primaryKey" json:"id"`
	AccountID  uuid.UUID  `gorm:"type:uuid;not null;index:idx_account_memory_entries_account_updated,priority:1" json:"account_id"`
	Content    string     `gorm:"type:text;not null" json:"content"`
	Category   string     `gorm:"type:varchar(32);not null;default:'other';index" json:"category"`
	MemoryType string     `gorm:"column:memory_type;type:varchar(32);not null;default:'long_term';index:idx_account_memory_entries_type_expires,priority:2" json:"memory_type"`
	ExpiresAt  *time.Time `gorm:"column:expires_at;index:idx_account_memory_entries_type_expires,priority:3" json:"expires_at,omitempty"`
	Enabled    bool       `gorm:"not null;default:true" json:"enabled"`
	CreatedAt  time.Time  `json:"created_at"`
	UpdatedAt  time.Time  `gorm:"index:idx_account_memory_entries_account_updated,priority:2" json:"updated_at"`
}

func (AccountMemoryEntry) TableName() string {
	return "account_memory_entries"
}

func (e *AccountMemoryEntry) BeforeCreate(tx *gorm.DB) error {
	if e.ID == uuid.Nil {
		e.ID = uuid.New()
	}
	return nil
}

type AccountMemoryEvent struct {
	ID                   uuid.UUID      `gorm:"type:uuid;default:uuid_generate_v4();primaryKey" json:"id"`
	AccountID            uuid.UUID      `gorm:"type:uuid;not null;index:idx_account_memory_events_account_created,priority:1" json:"account_id"`
	EntryID              *uuid.UUID     `gorm:"type:uuid;index:idx_account_memory_events_entry" json:"entry_id,omitempty"`
	Action               string         `gorm:"type:varchar(32);not null;index" json:"action"`
	ActorType            string         `gorm:"type:varchar(32);not null;default:'user';index" json:"actor_type"`
	Source               string         `gorm:"type:varchar(32);not null;default:'api';index" json:"source"`
	SourceConversationID *uuid.UUID     `gorm:"type:uuid;index" json:"source_conversation_id,omitempty"`
	SourceMessageID      *uuid.UUID     `gorm:"type:uuid;index" json:"source_message_id,omitempty"`
	BeforeSnapshot       datatypes.JSON `gorm:"type:jsonb" json:"before_snapshot,omitempty"`
	AfterSnapshot        datatypes.JSON `gorm:"type:jsonb" json:"after_snapshot,omitempty"`
	CreatedAt            time.Time      `gorm:"not null;default:CURRENT_TIMESTAMP;index:idx_account_memory_events_account_created,priority:2" json:"created_at"`
}

func (AccountMemoryEvent) TableName() string {
	return "account_memory_events"
}

func (e *AccountMemoryEvent) BeforeCreate(tx *gorm.DB) error {
	if e.ID == uuid.Nil {
		e.ID = uuid.New()
	}
	return nil
}
