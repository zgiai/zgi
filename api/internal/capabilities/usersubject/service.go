package usersubject

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/google/uuid"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

var (
	ErrInvalidSubject = errors.New("invalid user subject")
	ErrIssuerMismatch = errors.New("user subject issuer mismatch")
)

// EnsureRequest describes the trusted issuer of a stable subject.
type EnsureRequest struct {
	Subject    Ref
	IssuerType string
	IssuerID   *uuid.UUID
}

// Service owns the canonical subject registry.
type Service struct{ db *gorm.DB }

// NewService constructs a subject registry backed by db.
func NewService(db *gorm.DB) *Service {
	if db == nil {
		panic("user subject service requires db")
	}
	return &Service{db: db}
}

// Ensure creates or verifies a canonical subject without retaining raw user
// input. Reusing an ID with a different kind or issuer is rejected.
func (s *Service) Ensure(ctx context.Context, req EnsureRequest) (*Subject, error) {
	if req.Subject.ID == uuid.Nil || (req.Subject.Kind != Account && req.Subject.Kind != APIEndUser) {
		return nil, ErrInvalidSubject
	}
	subject := Subject{ID: req.Subject.ID, Kind: req.Subject.Kind, IssuerType: strings.TrimSpace(req.IssuerType), IssuerID: cloneUUID(req.IssuerID)}
	if req.Subject.Kind == Account {
		accountID := req.Subject.ID
		subject.AccountID = &accountID
		subject.IssuerType = ""
		subject.IssuerID = nil
	} else if subject.IssuerType == "" || subject.IssuerID == nil || *subject.IssuerID == uuid.Nil {
		return nil, ErrInvalidSubject
	}
	if err := s.db.WithContext(ctx).Clauses(clause.OnConflict{DoNothing: true}).Create(&subject).Error; err != nil {
		return nil, fmt.Errorf("ensure user subject: %w", err)
	}
	var stored Subject
	if err := s.db.WithContext(ctx).First(&stored, "id = ?", req.Subject.ID).Error; err != nil {
		return nil, fmt.Errorf("read user subject: %w", err)
	}
	if stored.Kind != req.Subject.Kind || stored.IssuerType != subject.IssuerType || !sameUUID(stored.IssuerID, subject.IssuerID) {
		return nil, ErrIssuerMismatch
	}
	return &stored, nil
}

// Require loads a previously registered subject and verifies its kind. It is
// used when the caller does not possess the subject's private issuer input.
func (s *Service) Require(ctx context.Context, ref Ref) (*Subject, error) {
	if ref.ID == uuid.Nil || (ref.Kind != Account && ref.Kind != APIEndUser) {
		return nil, ErrInvalidSubject
	}
	var subject Subject
	if err := s.db.WithContext(ctx).First(&subject, "id = ?", ref.ID).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ErrInvalidSubject
		}
		return nil, fmt.Errorf("read user subject: %w", err)
	}
	if subject.Kind != ref.Kind {
		return nil, ErrIssuerMismatch
	}
	return &subject, nil
}

func cloneUUID(value *uuid.UUID) *uuid.UUID {
	if value == nil {
		return nil
	}
	cloned := *value
	return &cloned
}

func sameUUID(left, right *uuid.UUID) bool {
	if left == nil || right == nil {
		return left == nil && right == nil
	}
	return *left == *right
}
