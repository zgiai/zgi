// Package commandrules stores an organization's server command rules (ADR
// 0029). Administrators keep them; every SSH call a DigitalHuman makes in the
// organization is checked against them before approval.
package commandrules

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/zgiai/zgi/api/internal/capabilities/serverops"
	"gorm.io/gorm"
)

// maxRules bounds one organization's rules.
const maxRules = 100

var (
	// ErrInvalid is a rule that is malformed, duplicated, or over the limit.
	ErrInvalid = errors.New("command rule is invalid")
	// ErrNotFound is a rule that does not exist in the organization.
	ErrNotFound = errors.New("command rule not found")
)

// Rule is one stored rule.
type Rule struct {
	ID             uuid.UUID            `gorm:"type:uuid;primaryKey" json:"id"`
	OrganizationID uuid.UUID            `gorm:"type:uuid;not null" json:"-"`
	Prefix         string               `gorm:"not null" json:"prefix"`
	Action         serverops.RuleAction `gorm:"not null" json:"action"`
	Note           string               `gorm:"not null" json:"note"`
	CreatedBy      uuid.UUID            `gorm:"type:uuid;not null" json:"created_by"`
	CreatedAt      time.Time            `gorm:"not null" json:"created_at"`
}

func (Rule) TableName() string { return "connector_command_rules" }

// Service reads and changes rules. Callers check that a writer is an
// organization administrator.
type Service struct {
	db  *gorm.DB
	now func() time.Time
}

func NewService(db *gorm.DB) *Service {
	return &Service{db: db, now: func() time.Time { return time.Now().UTC() }}
}

// List returns an organization's rules, oldest first.
func (s *Service) List(ctx context.Context, organizationID uuid.UUID) ([]Rule, error) {
	rules := []Rule{}
	if err := s.db.WithContext(ctx).Where("organization_id = ?", organizationID).Order("created_at, id").Find(&rules).Error; err != nil {
		return nil, fmt.Errorf("list command rules: %w", err)
	}
	return rules, nil
}

// Create adds one rule.
func (s *Service) Create(ctx context.Context, organizationID, actorID uuid.UUID, prefix string, action serverops.RuleAction, note string) (Rule, error) {
	normalized, err := serverops.NormalizeRulePrefix(prefix)
	note = strings.TrimSpace(note)
	if err != nil || (action != serverops.RuleAsk && action != serverops.RuleForbid) || len(note) > 200 ||
		organizationID == uuid.Nil || actorID == uuid.Nil {
		return Rule{}, ErrInvalid
	}
	rule := Rule{ID: uuid.New(), OrganizationID: organizationID, Prefix: normalized, Action: action, Note: note, CreatedBy: actorID, CreatedAt: s.now()}
	err = s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var count int64
		if err := tx.Model(&Rule{}).Where("organization_id = ?", organizationID).Count(&count).Error; err != nil {
			return err
		}
		var duplicate int64
		if err := tx.Model(&Rule{}).Where("organization_id = ? AND prefix = ?", organizationID, normalized).Count(&duplicate).Error; err != nil {
			return err
		}
		if count >= maxRules || duplicate > 0 {
			return ErrInvalid
		}
		return tx.Create(&rule).Error
	})
	if errors.Is(err, ErrInvalid) {
		return Rule{}, ErrInvalid
	}
	if err != nil {
		return Rule{}, fmt.Errorf("create command rule: %w", err)
	}
	return rule, nil
}

// Delete removes one rule.
func (s *Service) Delete(ctx context.Context, organizationID, ruleID uuid.UUID) error {
	result := s.db.WithContext(ctx).Where("organization_id = ? AND id = ?", organizationID, ruleID).Delete(&Rule{})
	if result.Error != nil {
		return fmt.Errorf("delete command rule: %w", result.Error)
	}
	if result.RowsAffected == 0 {
		return ErrNotFound
	}
	return nil
}

// CommandRules returns the rules execution checks every SSH call against.
func (s *Service) CommandRules(ctx context.Context, organizationID uuid.UUID) ([]serverops.CommandRule, error) {
	rules, err := s.List(ctx, organizationID)
	if err != nil {
		return nil, err
	}
	result := make([]serverops.CommandRule, 0, len(rules))
	for _, rule := range rules {
		result = append(result, serverops.CommandRule{Prefix: rule.Prefix, Action: rule.Action})
	}
	return result, nil
}
