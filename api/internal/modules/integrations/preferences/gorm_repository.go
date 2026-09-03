package preferences

import (
	"context"
	"fmt"

	"github.com/google/uuid"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// GormRepository persists AIChat connection preferences. Domain validation
// and authorization stay in the parent integrations service.
type GormRepository struct{ db *gorm.DB }

var (
	_ AIChatIntegrationPreferenceRepository       = (*GormRepository)(nil)
	_ AIChatIntegrationPreferenceRepairRepository = (*GormRepository)(nil)
)

func NewGormRepository(db *gorm.DB) *GormRepository {
	return &GormRepository{db: db}
}

func (repository *GormRepository) List(ctx context.Context, organizationID, accountID uuid.UUID, workspaceID *uuid.UUID) ([]AIChatIntegrationPreference, error) {
	if repository == nil || repository.db == nil {
		return nil, fmt.Errorf("AIChat integration preference repository is unavailable")
	}
	query := scopeQuery(repository.db.WithContext(ctx), organizationID, accountID, workspaceID)
	var items []AIChatIntegrationPreference
	if err := query.Order("integration_id ASC").Find(&items).Error; err != nil {
		return nil, fmt.Errorf("list AIChat integration preferences: %w", err)
	}
	return items, nil
}

func (repository *GormRepository) Replace(ctx context.Context, organizationID, accountID uuid.UUID, workspaceID *uuid.UUID, items []AIChatIntegrationPreference) error {
	if repository == nil || repository.db == nil {
		return fmt.Errorf("AIChat integration preference repository is unavailable")
	}
	return repository.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := scopeQuery(tx, organizationID, accountID, workspaceID).Delete(&AIChatIntegrationPreference{}).Error; err != nil {
			return fmt.Errorf("replace AIChat integration preferences: %w", err)
		}
		for index := range items {
			items[index].OrganizationID = organizationID
			items[index].AccountID = accountID
			items[index].WorkspaceID = cloneUUID(workspaceID)
			if err := tx.Create(&items[index]).Error; err != nil {
				return fmt.Errorf("create AIChat integration preference: %w", err)
			}
		}
		return nil
	})
}

func (repository *GormRepository) RepairIfUnchanged(
	ctx context.Context,
	organizationID, accountID uuid.UUID,
	workspaceID *uuid.UUID,
	observed, repaired []AIChatIntegrationPreference,
) (bool, error) {
	if repository == nil || repository.db == nil {
		return false, fmt.Errorf("AIChat integration preference repository is unavailable")
	}
	applied := false
	err := repository.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var current []AIChatIntegrationPreference
		if err := scopeQuery(tx, organizationID, accountID, workspaceID).Clauses(clause.Locking{Strength: "UPDATE"}).Order("integration_id ASC").Find(&current).Error; err != nil {
			return fmt.Errorf("lock AIChat integration preferences for repair: %w", err)
		}
		if !sameSnapshot(current, observed) {
			return nil
		}
		if err := scopeQuery(tx, organizationID, accountID, workspaceID).Delete(&AIChatIntegrationPreference{}).Error; err != nil {
			return fmt.Errorf("delete stale AIChat integration preferences: %w", err)
		}
		observedByIntegration := make(map[string]AIChatIntegrationPreference, len(observed))
		for _, item := range observed {
			observedByIntegration[item.IntegrationID] = item
		}
		for _, item := range repaired {
			item.OrganizationID = organizationID
			item.AccountID = accountID
			item.WorkspaceID = cloneUUID(workspaceID)
			if previous, exists := observedByIntegration[item.IntegrationID]; exists && !sameSelection(previous, item) {
				item.Revision = max(previous.Revision, 1) + 1
			}
			if err := tx.Create(&item).Error; err != nil {
				return fmt.Errorf("persist repaired AIChat integration preference: %w", err)
			}
		}
		applied = true
		return nil
	})
	if err != nil {
		return false, err
	}
	return applied, nil
}

func scopeQuery(db *gorm.DB, organizationID, accountID uuid.UUID, workspaceID *uuid.UUID) *gorm.DB {
	query := db.Where("organization_id = ? AND account_id = ?", organizationID, accountID)
	if workspaceID == nil || *workspaceID == uuid.Nil {
		return query.Where("workspace_id IS NULL")
	}
	return query.Where("workspace_id = ?", *workspaceID)
}

func cloneUUID(value *uuid.UUID) *uuid.UUID {
	if value == nil {
		return nil
	}
	copyValue := *value
	return &copyValue
}

func sameSnapshot(left, right []AIChatIntegrationPreference) bool {
	if len(left) != len(right) {
		return false
	}
	for index := range left {
		if left[index].ID != right[index].ID || left[index].Revision != right[index].Revision ||
			left[index].OrganizationID != right[index].OrganizationID || left[index].AccountID != right[index].AccountID ||
			!sameUUID(left[index].WorkspaceID, right[index].WorkspaceID) || !sameSelection(left[index], right[index]) {
			return false
		}
	}
	return true
}

func sameUUID(left, right *uuid.UUID) bool {
	if left == nil || right == nil {
		return left == nil && right == nil
	}
	return *left == *right
}

func sameSelection(left, right AIChatIntegrationPreference) bool {
	if left.IntegrationID != right.IntegrationID || len(left.SelectedConnectionIDs) != len(right.SelectedConnectionIDs) {
		return false
	}
	for index := range left.SelectedConnectionIDs {
		if left.SelectedConnectionIDs[index] != right.SelectedConnectionIDs[index] {
			return false
		}
	}
	return sameUUID(left.PreferredConnectionID, right.PreferredConnectionID)
}
