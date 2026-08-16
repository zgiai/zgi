package integrations

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"sort"
	"strings"

	"github.com/google/uuid"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

var ErrActionPolicyChanged = errors.New("integration action policy set changed concurrently")

type ActionPolicyRepository interface {
	Get(ctx context.Context, organizationID uuid.UUID, integrationID, actionID string) (*IntegrationActionPolicy, error)
	List(ctx context.Context, organizationID uuid.UUID, integrationID string) ([]IntegrationActionPolicy, error)
	Replace(ctx context.Context, organizationID uuid.UUID, integrationID string, policies []IntegrationActionPolicy) error
}

type VersionedActionPolicyRepository interface {
	ActionPolicyRepository
	ReplaceIfRevision(ctx context.Context, organizationID uuid.UUID, integrationID, expectedRevision string, actions []ActionDefinition, policies []IntegrationActionPolicy) error
}

type GormActionPolicyRepository struct {
	db *gorm.DB
}

func NewGormActionPolicyRepository(db *gorm.DB) *GormActionPolicyRepository {
	return &GormActionPolicyRepository{db: db}
}

func (repository *GormActionPolicyRepository) Get(ctx context.Context, organizationID uuid.UUID, integrationID, actionID string) (*IntegrationActionPolicy, error) {
	if repository == nil || repository.db == nil {
		return nil, fmt.Errorf("integration action policy repository is unavailable")
	}
	var policy IntegrationActionPolicy
	err := repository.db.WithContext(ctx).
		Where("organization_id = ? AND integration_id = ? AND action_id = ?", organizationID, strings.ToLower(strings.TrimSpace(integrationID)), strings.ToLower(strings.TrimSpace(actionID))).
		First(&policy).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("get integration action policy: %w", err)
	}
	return &policy, nil
}

func (repository *GormActionPolicyRepository) List(ctx context.Context, organizationID uuid.UUID, integrationID string) ([]IntegrationActionPolicy, error) {
	if repository == nil || repository.db == nil {
		return nil, fmt.Errorf("integration action policy repository is unavailable")
	}
	var policies []IntegrationActionPolicy
	if err := repository.db.WithContext(ctx).
		Where("organization_id = ? AND integration_id = ?", organizationID, strings.ToLower(strings.TrimSpace(integrationID))).
		Order("action_id ASC").
		Find(&policies).Error; err != nil {
		return nil, fmt.Errorf("list integration action policies: %w", err)
	}
	return policies, nil
}

func (repository *GormActionPolicyRepository) Replace(ctx context.Context, organizationID uuid.UUID, integrationID string, policies []IntegrationActionPolicy) error {
	if repository == nil || repository.db == nil {
		return fmt.Errorf("integration action policy repository is unavailable")
	}
	integrationID = strings.ToLower(strings.TrimSpace(integrationID))
	return repository.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		return replaceActionPolicies(tx, organizationID, integrationID, policies)
	})
}

func (repository *GormActionPolicyRepository) ReplaceIfRevision(ctx context.Context, organizationID uuid.UUID, integrationID, expectedRevision string, actions []ActionDefinition, policies []IntegrationActionPolicy) error {
	if repository == nil || repository.db == nil {
		return fmt.Errorf("integration action policy repository is unavailable")
	}
	integrationID = strings.ToLower(strings.TrimSpace(integrationID))
	expectedRevision = strings.ToLower(strings.TrimSpace(expectedRevision))
	if expectedRevision == "" {
		return ErrActionPolicyChanged
	}
	return repository.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		lockKey := organizationID.String() + "/" + integrationID
		if err := tx.Exec("SELECT pg_advisory_xact_lock(hashtextextended(?, 0))", lockKey).Error; err != nil {
			return fmt.Errorf("lock integration action policy set: %w", err)
		}
		var current []IntegrationActionPolicy
		if err := tx.Where("organization_id = ? AND integration_id = ?", organizationID, integrationID).
			Order("action_id ASC").Find(&current).Error; err != nil {
			return fmt.Errorf("load integration action policy revision: %w", err)
		}
		if actionPolicyRevision(actions, current) != expectedRevision {
			return ErrActionPolicyChanged
		}
		return replaceActionPolicies(tx, organizationID, integrationID, policies)
	})
}

func replaceActionPolicies(tx *gorm.DB, organizationID uuid.UUID, integrationID string, policies []IntegrationActionPolicy) error {
	if err := tx.Where("organization_id = ? AND integration_id = ?", organizationID, integrationID).
		Delete(&IntegrationActionPolicy{}).Error; err != nil {
		return fmt.Errorf("clear integration action policies: %w", err)
	}
	if len(policies) == 0 {
		return nil
	}
	if err := tx.Clauses(clause.OnConflict{
		Columns:   []clause.Column{{Name: "organization_id"}, {Name: "integration_id"}, {Name: "action_id"}},
		DoUpdates: clause.AssignmentColumns([]string{"enabled", "approval_policy", "data_egress_allowed", "updated_by", "updated_at"}),
	}).Create(&policies).Error; err != nil {
		return fmt.Errorf("replace integration action policies: %w", err)
	}
	return nil
}

func actionPolicyRevision(actions []ActionDefinition, policies []IntegrationActionPolicy) string {
	normalizedActions := append([]ActionDefinition(nil), actions...)
	sort.Slice(normalizedActions, func(left, right int) bool {
		return strings.ToLower(strings.TrimSpace(normalizedActions[left].ID)) < strings.ToLower(strings.TrimSpace(normalizedActions[right].ID))
	})
	byActionID := make(map[string]IntegrationActionPolicy, len(policies))
	for _, policy := range policies {
		byActionID[strings.ToLower(strings.TrimSpace(policy.ActionID))] = policy
	}
	hash := sha256.New()
	for _, action := range normalizedActions {
		actionID := strings.ToLower(strings.TrimSpace(action.ID))
		policy, exists := byActionID[actionID]
		if !exists {
			policy = IntegrationActionPolicy{
				ActionID:          actionID,
				Enabled:           true,
				ApprovalPolicy:    IntegrationApprovalPolicyInherit,
				DataEgressAllowed: true,
			}
		}
		callers := make([]string, 0, len(action.SupportedCallers))
		for _, caller := range action.SupportedCallers {
			callers = append(callers, strings.ToLower(strings.TrimSpace(string(caller))))
		}
		sort.Strings(callers)
		_, _ = fmt.Fprintf(hash, "action\x00%s\x00%s\x00%s\x00%t\x00%s\x00%t\x00%t\x00%s\x00%t\x00%s\x00%t\n",
			actionID,
			action.Effect,
			action.RiskLevel,
			action.DataEgress,
			strings.ToLower(strings.TrimSpace(action.ExternalDestination)),
			action.SensitiveDataAllowed,
			action.Idempotent,
			strings.Join(callers, ","),
			policy.Enabled,
			policy.ApprovalPolicy,
			policy.DataEgressAllowed,
		)
		delete(byActionID, actionID)
	}
	// Keep unexpected legacy rows in the revision so they cannot be silently
	// discarded by a stale administrator snapshot.
	orphanIDs := make([]string, 0, len(byActionID))
	for actionID := range byActionID {
		orphanIDs = append(orphanIDs, actionID)
	}
	sort.Strings(orphanIDs)
	for _, actionID := range orphanIDs {
		policy := byActionID[actionID]
		_, _ = fmt.Fprintf(hash, "orphan\x00%s\x00%t\x00%s\x00%t\n", actionID, policy.Enabled, policy.ApprovalPolicy, policy.DataEgressAllowed)
	}
	return hex.EncodeToString(hash.Sum(nil))
}
