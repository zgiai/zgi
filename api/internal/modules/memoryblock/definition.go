// Package memoryblock holds memory block definitions (ADR 0013): the
// consumer-declared files that replace Agent memory slots. A definition keeps
// one stable ID per consumer and label, so values and version history can
// refer to it while slots are rewritten, renamed, or restored from a
// published version.
package memoryblock

import (
	"context"
	"fmt"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

const (
	ConsumerAgent = "agent"

	PartitionPerson = "person"
	OwnerUser       = "user"
)

// Definition is one block a consumer declares. Removing a block only marks it
// deleted, so a block that comes back under the same label keeps its ID.
type Definition struct {
	ID           uuid.UUID  `gorm:"type:uuid;primaryKey" json:"id"`
	ConsumerType string     `gorm:"type:varchar(32);not null;uniqueIndex:idx_memory_block_definitions_label,priority:1" json:"consumer_type"`
	ConsumerID   uuid.UUID  `gorm:"type:uuid;not null;uniqueIndex:idx_memory_block_definitions_label,priority:2" json:"consumer_id"`
	Label        string     `gorm:"type:varchar(64);not null;uniqueIndex:idx_memory_block_definitions_label,priority:3" json:"label"`
	Name         string     `gorm:"type:varchar(80);not null;default:''" json:"name"`
	Description  string     `gorm:"type:text;not null;default:''" json:"description"`
	LimitChars   int        `gorm:"not null" json:"limit"`
	Partition    string     `gorm:"type:varchar(16);not null" json:"partition"`
	Owner        string     `gorm:"type:varchar(16);not null" json:"owner"`
	Audience     string     `gorm:"type:varchar(16);not null;default:''" json:"audience,omitempty"`
	ReadOnly     bool       `gorm:"not null;default:false" json:"read_only"`
	AutoWrite    bool       `gorm:"not null;default:true" json:"auto_write"`
	RetentionSec *int64     `json:"retention_seconds,omitempty"`
	Enabled      bool       `gorm:"not null;default:true" json:"enabled"`
	SortOrder    int        `gorm:"not null;default:0" json:"sort_order"`
	CreatedAt    time.Time  `json:"created_at"`
	UpdatedAt    time.Time  `json:"updated_at"`
	DeletedAt    *time.Time `json:"deleted_at,omitempty"`
}

func (Definition) TableName() string { return "memory_block_definitions" }

// DefinitionID returns the ID of a consumer's definition for label, deleted or
// not, or nil when there is none.
func DefinitionID(ctx context.Context, tx *gorm.DB, consumerType string, consumerID uuid.UUID, label string) (*uuid.UUID, error) {
	var ids []uuid.UUID
	if err := tx.WithContext(ctx).Model(&Definition{}).
		Where("consumer_type = ? AND consumer_id = ? AND label = ?", consumerType, consumerID, label).
		Limit(1).Pluck("id", &ids).Error; err != nil {
		return nil, fmt.Errorf("find memory block definition %s: %w", label, err)
	}
	if len(ids) == 0 {
		return nil, nil
	}
	return &ids[0], nil
}
