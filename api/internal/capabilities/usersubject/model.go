// Package usersubject defines the stable data owner used by conversations,
// files, memories, and user-owned runtime work.
package usersubject

import (
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

// Kind identifies the class of user represented by a subject.
type Kind string

const (
	Account    Kind = "account"
	APIEndUser Kind = "api_end_user"
)

// Ref is the stable, non-sensitive identity of a user data owner.
type Ref struct {
	Kind Kind      `json:"kind"`
	ID   uuid.UUID `json:"id"`
}

// Subject stores the canonical identity and issuer of a user subject. Raw
// external user identifiers are deliberately never retained.
type Subject struct {
	ID         uuid.UUID  `gorm:"type:uuid;primaryKey" json:"id"`
	Kind       Kind       `gorm:"type:varchar(32);not null;index" json:"kind"`
	AccountID  *uuid.UUID `gorm:"type:uuid;uniqueIndex:idx_user_subject_account,where:account_id IS NOT NULL" json:"account_id,omitempty"`
	IssuerType string     `gorm:"type:varchar(32);not null;default:'';index:idx_user_subject_issuer,priority:1" json:"issuer_type,omitempty"`
	IssuerID   *uuid.UUID `gorm:"type:uuid;index:idx_user_subject_issuer,priority:2" json:"issuer_id,omitempty"`
	CreatedAt  time.Time  `json:"created_at"`
	UpdatedAt  time.Time  `json:"updated_at"`
}

func (Subject) TableName() string { return "user_subjects" }

func (s *Subject) BeforeCreate(*gorm.DB) error {
	if s.ID == uuid.Nil {
		s.ID = uuid.New()
	}
	return nil
}
