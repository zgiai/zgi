package memory

import (
	"time"

	"github.com/google/uuid"
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
	ID        uuid.UUID `gorm:"type:uuid;default:uuid_generate_v4();primaryKey" json:"id"`
	AccountID uuid.UUID `gorm:"type:uuid;not null;index:idx_account_memory_entries_account_updated,priority:1" json:"account_id"`
	Content   string    `gorm:"type:text;not null" json:"content"`
	Category  string    `gorm:"type:varchar(32);not null;default:'other';index" json:"category"`
	Enabled   bool      `gorm:"not null;default:true" json:"enabled"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `gorm:"index:idx_account_memory_entries_account_updated,priority:2" json:"updated_at"`
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
