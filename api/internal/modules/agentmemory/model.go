package agentmemory

import (
	"time"

	"github.com/google/uuid"
	"gorm.io/datatypes"
	"gorm.io/gorm"
)

const (
	SkillID    = "agent-memory"
	ProviderID = "agent-memory"

	UserScopeAccount = "account"
	UserScopeEndUser = "end_user"

	EventActionSlotCreate  = "slot_create"
	EventActionSlotUpdate  = "slot_update"
	EventActionSlotDisable = "slot_disable"
	EventActionSlotDelete  = "slot_delete"
	EventActionValueUpdate = "value_update"
	EventActionValueClear  = "value_clear"

	EventActorOrganizer = "organizer"
	EventActorModel     = "model"
	EventActorSystem    = "system"

	EventSourceAPI   = "api"
	EventSourceAgent = "agent"
)

type AgentMemorySlot struct {
	ID          uuid.UUID `gorm:"type:uuid;default:uuid_generate_v4();primaryKey" json:"id"`
	WorkspaceID uuid.UUID `gorm:"type:uuid;not null;uniqueIndex:idx_agent_memory_slots_agent_key,priority:1;index:idx_agent_memory_slots_agent_sort,priority:1" json:"workspace_id"`
	AgentID     uuid.UUID `gorm:"type:uuid;not null;uniqueIndex:idx_agent_memory_slots_agent_key,priority:2;index:idx_agent_memory_slots_agent_sort,priority:2" json:"agent_id"`
	Key         string    `gorm:"type:varchar(64);not null;uniqueIndex:idx_agent_memory_slots_agent_key,priority:3" json:"key"`
	Description string    `gorm:"type:text;not null;default:''" json:"description"`
	MaxChars    int       `gorm:"not null;default:1000" json:"max_chars"`
	Enabled     bool      `gorm:"not null;default:true;index" json:"enabled"`
	SortOrder   int       `gorm:"not null;default:0;index:idx_agent_memory_slots_agent_sort,priority:3" json:"sort_order"`
	CreatedBy   uuid.UUID `gorm:"type:uuid" json:"created_by"`
	UpdatedBy   uuid.UUID `gorm:"type:uuid" json:"updated_by"`
	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`
}

func (AgentMemorySlot) TableName() string {
	return "agent_memory_slots"
}

func (s *AgentMemorySlot) BeforeCreate(tx *gorm.DB) error {
	if s.ID == uuid.Nil {
		s.ID = uuid.New()
	}
	return nil
}

type AgentMemoryValue struct {
	ID          uuid.UUID `gorm:"type:uuid;default:uuid_generate_v4();primaryKey" json:"id"`
	WorkspaceID uuid.UUID `gorm:"type:uuid;not null;uniqueIndex:idx_agent_memory_values_scope,priority:1;index:idx_agent_memory_values_agent_user,priority:1" json:"workspace_id"`
	AgentID     uuid.UUID `gorm:"type:uuid;not null;uniqueIndex:idx_agent_memory_values_scope,priority:2;index:idx_agent_memory_values_agent_user,priority:2" json:"agent_id"`
	SlotKey     string    `gorm:"type:varchar(64);not null;uniqueIndex:idx_agent_memory_values_scope,priority:3;index" json:"slot_key"`
	UserScope   string    `gorm:"type:varchar(32);not null;uniqueIndex:idx_agent_memory_values_scope,priority:4;index:idx_agent_memory_values_agent_user,priority:3" json:"user_scope"`
	UserID      uuid.UUID `gorm:"type:uuid;not null;uniqueIndex:idx_agent_memory_values_scope,priority:5;index:idx_agent_memory_values_agent_user,priority:4" json:"user_id"`
	Content     string    `gorm:"type:text;not null;default:''" json:"content"`
	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`
}

func (AgentMemoryValue) TableName() string {
	return "agent_memory_values"
}

func (v *AgentMemoryValue) BeforeCreate(tx *gorm.DB) error {
	if v.ID == uuid.Nil {
		v.ID = uuid.New()
	}
	return nil
}

type AgentMemoryEvent struct {
	ID                   uuid.UUID      `gorm:"type:uuid;default:uuid_generate_v4();primaryKey" json:"id"`
	WorkspaceID          uuid.UUID      `gorm:"type:uuid;not null;index:idx_agent_memory_events_agent_created,priority:1" json:"workspace_id"`
	AgentID              uuid.UUID      `gorm:"type:uuid;not null;index:idx_agent_memory_events_agent_created,priority:2" json:"agent_id"`
	SlotKey              string         `gorm:"type:varchar(64);not null;default:'';index" json:"slot_key,omitempty"`
	UserScope            string         `gorm:"type:varchar(32);index" json:"user_scope,omitempty"`
	UserID               *uuid.UUID     `gorm:"type:uuid;index" json:"user_id,omitempty"`
	Action               string         `gorm:"type:varchar(32);not null;index" json:"action"`
	ActorType            string         `gorm:"type:varchar(32);not null;default:'system';index" json:"actor_type"`
	Source               string         `gorm:"type:varchar(32);not null;default:'api';index" json:"source"`
	SourceConversationID *uuid.UUID     `gorm:"type:uuid;index" json:"source_conversation_id,omitempty"`
	SourceMessageID      *uuid.UUID     `gorm:"type:uuid;index" json:"source_message_id,omitempty"`
	BeforeSnapshot       datatypes.JSON `gorm:"type:jsonb" json:"before_snapshot,omitempty"`
	AfterSnapshot        datatypes.JSON `gorm:"type:jsonb" json:"after_snapshot,omitempty"`
	CreatedAt            time.Time      `gorm:"not null;default:CURRENT_TIMESTAMP;index:idx_agent_memory_events_agent_created,priority:3" json:"created_at"`
}

func (AgentMemoryEvent) TableName() string {
	return "agent_memory_events"
}

func (e *AgentMemoryEvent) BeforeCreate(tx *gorm.DB) error {
	if e.ID == uuid.Nil {
		e.ID = uuid.New()
	}
	return nil
}
