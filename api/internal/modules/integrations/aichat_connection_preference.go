package integrations

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/zgiai/zgi/api/pkg/logger"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

type AIChatIntegrationPreference struct {
	ID                    uuid.UUID  `gorm:"type:uuid;primaryKey" json:"id"`
	OrganizationID        uuid.UUID  `gorm:"type:uuid;not null;index" json:"organization_id"`
	AccountID             uuid.UUID  `gorm:"type:uuid;not null;index" json:"account_id"`
	WorkspaceID           *uuid.UUID `gorm:"type:uuid" json:"workspace_id,omitempty"`
	IntegrationID         string     `gorm:"size:64;not null" json:"integration_id"`
	SelectedConnectionIDs []string   `gorm:"type:jsonb;serializer:json;not null;default:'[]'" json:"selected_connection_ids"`
	PreferredConnectionID *uuid.UUID `gorm:"type:uuid" json:"preferred_connection_id,omitempty"`
	Revision              int        `gorm:"not null;default:1" json:"revision"`
	CreatedAt             time.Time  `json:"created_at"`
	UpdatedAt             time.Time  `json:"updated_at"`
}

func (AIChatIntegrationPreference) TableName() string {
	return "aichat_integration_preferences"
}

func (preference *AIChatIntegrationPreference) BeforeCreate(_ *gorm.DB) error {
	if preference.ID == uuid.Nil {
		preference.ID = uuid.New()
	}
	if preference.Revision < 1 {
		preference.Revision = 1
	}
	return validateAIChatIntegrationPreference(preference)
}

type AIChatIntegrationPreferenceInput struct {
	IntegrationID         string
	SelectedConnectionIDs []uuid.UUID
	PreferredConnectionID *uuid.UUID
}

type AIChatIntegrationPreferenceRepository interface {
	List(ctx context.Context, organizationID, accountID uuid.UUID, workspaceID *uuid.UUID) ([]AIChatIntegrationPreference, error)
	Replace(ctx context.Context, organizationID, accountID uuid.UUID, workspaceID *uuid.UUID, preferences []AIChatIntegrationPreference) error
}

// AIChatIntegrationPreferenceRepairRepository is an optional repository
// capability used to persist read-time removal of stale selections without
// overwriting a preference update that raced the read. Runtime correctness
// never depends on repair support: List always returns the sanitized snapshot.
type AIChatIntegrationPreferenceRepairRepository interface {
	RepairIfUnchanged(
		ctx context.Context,
		organizationID, accountID uuid.UUID,
		workspaceID *uuid.UUID,
		observed, repaired []AIChatIntegrationPreference,
	) (bool, error)
}

type GormAIChatIntegrationPreferenceRepository struct{ db *gorm.DB }

func NewGormAIChatIntegrationPreferenceRepository(db *gorm.DB) *GormAIChatIntegrationPreferenceRepository {
	return &GormAIChatIntegrationPreferenceRepository{db: db}
}

func (repository *GormAIChatIntegrationPreferenceRepository) List(ctx context.Context, organizationID, accountID uuid.UUID, workspaceID *uuid.UUID) ([]AIChatIntegrationPreference, error) {
	if repository == nil || repository.db == nil {
		return nil, fmt.Errorf("AIChat integration preference repository is unavailable")
	}
	query := repository.db.WithContext(ctx).Where("organization_id = ? AND account_id = ?", organizationID, accountID)
	if workspaceID == nil || *workspaceID == uuid.Nil {
		query = query.Where("workspace_id IS NULL")
	} else {
		query = query.Where("workspace_id = ?", *workspaceID)
	}
	var preferences []AIChatIntegrationPreference
	if err := query.Order("integration_id ASC").Find(&preferences).Error; err != nil {
		return nil, fmt.Errorf("list AIChat integration preferences: %w", err)
	}
	return preferences, nil
}

func (repository *GormAIChatIntegrationPreferenceRepository) Replace(ctx context.Context, organizationID, accountID uuid.UUID, workspaceID *uuid.UUID, preferences []AIChatIntegrationPreference) error {
	if repository == nil || repository.db == nil {
		return fmt.Errorf("AIChat integration preference repository is unavailable")
	}
	return repository.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		deleteQuery := tx.Where("organization_id = ? AND account_id = ?", organizationID, accountID)
		if workspaceID == nil || *workspaceID == uuid.Nil {
			deleteQuery = deleteQuery.Where("workspace_id IS NULL")
		} else {
			deleteQuery = deleteQuery.Where("workspace_id = ?", *workspaceID)
		}
		if err := deleteQuery.Delete(&AIChatIntegrationPreference{}).Error; err != nil {
			return fmt.Errorf("replace AIChat integration preferences: %w", err)
		}
		for index := range preferences {
			preferences[index].OrganizationID = organizationID
			preferences[index].AccountID = accountID
			preferences[index].WorkspaceID = cloneUUIDPointer(workspaceID)
			if err := tx.Create(&preferences[index]).Error; err != nil {
				return fmt.Errorf("create AIChat integration preference: %w", err)
			}
		}
		return nil
	})
}

func (repository *GormAIChatIntegrationPreferenceRepository) RepairIfUnchanged(
	ctx context.Context,
	organizationID, accountID uuid.UUID,
	workspaceID *uuid.UUID,
	observed, repaired []AIChatIntegrationPreference,
) (bool, error) {
	if repository == nil || repository.db == nil {
		return false, fmt.Errorf("AIChat integration preference repository is unavailable")
	}
	repairedApplied := false
	err := repository.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		query := preferenceScopeQuery(tx, organizationID, accountID, workspaceID)
		var current []AIChatIntegrationPreference
		if err := query.Clauses(clause.Locking{Strength: "UPDATE"}).Order("integration_id ASC").Find(&current).Error; err != nil {
			return fmt.Errorf("lock AIChat integration preferences for repair: %w", err)
		}
		if !sameAIChatIntegrationPreferenceSnapshot(current, observed) {
			return nil
		}
		if err := preferenceScopeQuery(tx, organizationID, accountID, workspaceID).Delete(&AIChatIntegrationPreference{}).Error; err != nil {
			return fmt.Errorf("delete stale AIChat integration preferences: %w", err)
		}
		observedByIntegration := make(map[string]AIChatIntegrationPreference, len(observed))
		for _, preference := range observed {
			observedByIntegration[preference.IntegrationID] = preference
		}
		for _, preference := range repaired {
			preference.OrganizationID = organizationID
			preference.AccountID = accountID
			preference.WorkspaceID = cloneUUIDPointer(workspaceID)
			if previous, exists := observedByIntegration[preference.IntegrationID]; exists && !sameAIChatIntegrationPreferenceSelection(previous, preference) {
				preference.Revision = max(previous.Revision, 1) + 1
			}
			if err := tx.Create(&preference).Error; err != nil {
				return fmt.Errorf("persist repaired AIChat integration preference: %w", err)
			}
		}
		repairedApplied = true
		return nil
	})
	if err != nil {
		return false, err
	}
	return repairedApplied, nil
}

func preferenceScopeQuery(db *gorm.DB, organizationID, accountID uuid.UUID, workspaceID *uuid.UUID) *gorm.DB {
	query := db.Where("organization_id = ? AND account_id = ?", organizationID, accountID)
	if workspaceID == nil || *workspaceID == uuid.Nil {
		return query.Where("workspace_id IS NULL")
	}
	return query.Where("workspace_id = ?", *workspaceID)
}

type DefaultAIChatIntegrationPreferenceService struct {
	repository  AIChatIntegrationPreferenceRepository
	connections ConnectionRepository
	access      ConnectionPreferenceAccessChecker
}

func NewAIChatIntegrationPreferenceService(repository AIChatIntegrationPreferenceRepository, connections ConnectionRepository, access ConnectionPreferenceAccessChecker) *DefaultAIChatIntegrationPreferenceService {
	return &DefaultAIChatIntegrationPreferenceService{repository: repository, connections: connections, access: access}
}

func (service *DefaultAIChatIntegrationPreferenceService) List(ctx context.Context, organizationID, accountID uuid.UUID, workspaceID *uuid.UUID) ([]AIChatIntegrationPreference, error) {
	if service == nil || service.repository == nil || service.connections == nil || service.access == nil {
		return nil, fmt.Errorf("AIChat integration preference service is unavailable")
	}
	if organizationID == uuid.Nil || accountID == uuid.Nil {
		return nil, invalidInput("AIChat integration preference scope is invalid", nil)
	}
	stored, err := service.repository.List(ctx, organizationID, accountID, workspaceID)
	if err != nil {
		return nil, err
	}
	repaired, changed, err := service.sanitizeCurrentPreferences(ctx, organizationID, accountID, workspaceID, stored)
	if err != nil {
		return nil, err
	}
	if changed {
		if repairRepository, ok := service.repository.(AIChatIntegrationPreferenceRepairRepository); ok {
			if _, repairErr := repairRepository.RepairIfUnchanged(ctx, organizationID, accountID, workspaceID, stored, repaired); repairErr != nil {
				// The already-sanitized snapshot remains safe to use even if this
				// best-effort maintenance write fails. A later read will retry it.
				logger.WarnContext(ctx, "failed to persist repaired AIChat integration preferences", "error", repairErr)
			}
		}
	}
	return repaired, nil
}

func (service *DefaultAIChatIntegrationPreferenceService) Replace(ctx context.Context, organizationID, accountID uuid.UUID, workspaceID *uuid.UUID, inputs []AIChatIntegrationPreferenceInput) ([]AIChatIntegrationPreference, error) {
	if service == nil || service.repository == nil || service.connections == nil || service.access == nil {
		return nil, fmt.Errorf("AIChat integration preference service is unavailable")
	}
	if organizationID == uuid.Nil || accountID == uuid.Nil || len(inputs) > 32 {
		return nil, invalidInput("AIChat integration preferences are invalid", nil)
	}
	seenIntegrations := make(map[string]struct{}, len(inputs))
	preferences := make([]AIChatIntegrationPreference, 0, len(inputs))
	for _, input := range inputs {
		integrationID := strings.ToLower(strings.TrimSpace(input.IntegrationID))
		if !integrationIdentifierPattern.MatchString(integrationID) {
			return nil, invalidInput("AIChat integration preference integration is invalid", nil)
		}
		if _, duplicated := seenIntegrations[integrationID]; duplicated {
			return nil, invalidInput("AIChat integration preference is duplicated", nil)
		}
		seenIntegrations[integrationID] = struct{}{}
		selected := normalizePreferenceConnectionIDs(input.SelectedConnectionIDs)
		if len(selected) == 0 || len(selected) > 20 {
			return nil, invalidInput("AIChat integration preference must select between 1 and 20 connections", nil)
		}
		preferred := input.PreferredConnectionID
		if preferred == nil || *preferred == uuid.Nil || !containsPreferenceConnection(selected, *preferred) {
			return nil, invalidInput("preferred connection must be one of the selected connections", nil)
		}
		selectedStrings := make([]string, 0, len(selected))
		for _, connectionID := range selected {
			connection, err := service.connections.GetByID(ctx, organizationID, connectionID)
			if err != nil || connection == nil || !strings.EqualFold(connection.IntegrationID, integrationID) || !connectionAvailableForSelection(connection, time.Now().UTC()) {
				return nil, NewError(ErrorCodeAccessDenied, "selected integration connection is not available", err)
			}
			if err := service.access.AuthorizeConnectionPreference(ctx, organizationID, accountID, workspaceID, connectionID); err != nil {
				return nil, err
			}
			selectedStrings = append(selectedStrings, connectionID.String())
		}
		preferences = append(preferences, AIChatIntegrationPreference{
			IntegrationID: integrationID, SelectedConnectionIDs: selectedStrings,
			PreferredConnectionID: cloneUUIDPointer(preferred), Revision: 1,
		})
	}
	if err := service.repository.Replace(ctx, organizationID, accountID, workspaceID, preferences); err != nil {
		return nil, err
	}
	return service.List(ctx, organizationID, accountID, workspaceID)
}

func (service *DefaultAIChatIntegrationPreferenceService) sanitizeCurrentPreferences(
	ctx context.Context,
	organizationID, accountID uuid.UUID,
	workspaceID *uuid.UUID,
	stored []AIChatIntegrationPreference,
) ([]AIChatIntegrationPreference, bool, error) {
	now := time.Now().UTC()
	repaired := make([]AIChatIntegrationPreference, 0, len(stored))
	seenIntegrations := make(map[string]struct{}, len(stored))
	changed := false
	for _, preference := range stored {
		integrationID := strings.ToLower(strings.TrimSpace(preference.IntegrationID))
		if !integrationIdentifierPattern.MatchString(integrationID) {
			changed = true
			continue
		}
		if _, duplicate := seenIntegrations[integrationID]; duplicate {
			changed = true
			continue
		}
		seenIntegrations[integrationID] = struct{}{}

		selected := make([]uuid.UUID, 0, len(preference.SelectedConnectionIDs))
		seenConnections := make(map[uuid.UUID]struct{}, len(preference.SelectedConnectionIDs))
		for _, rawConnectionID := range preference.SelectedConnectionIDs {
			connectionID, parseErr := uuid.Parse(strings.TrimSpace(rawConnectionID))
			if parseErr != nil || connectionID == uuid.Nil {
				changed = true
				continue
			}
			if _, duplicate := seenConnections[connectionID]; duplicate {
				changed = true
				continue
			}
			seenConnections[connectionID] = struct{}{}
			connection, lookupErr := service.connections.GetByID(ctx, organizationID, connectionID)
			if lookupErr != nil {
				if errors.Is(lookupErr, ErrConnectionNotFound) {
					changed = true
					continue
				}
				return nil, false, fmt.Errorf("validate current AIChat integration connection: %w", lookupErr)
			}
			if connection == nil || !strings.EqualFold(connection.IntegrationID, integrationID) || !connectionAvailableForSelection(connection, now) {
				changed = true
				continue
			}
			if accessErr := service.access.AuthorizeConnectionPreference(ctx, organizationID, accountID, workspaceID, connectionID); accessErr != nil {
				if cause := errors.Unwrap(accessErr); cause != nil && !errors.Is(cause, ErrConnectionNotFound) {
					return nil, false, fmt.Errorf("validate current AIChat integration connection access: %w", cause)
				}
				// Authorization failures are intentionally indistinguishable here:
				// inaccessible connection identifiers and metadata must not escape
				// through either the preference API or the AIChat runtime snapshot.
				changed = true
				continue
			}
			selected = append(selected, connectionID)
		}

		if len(selected) == 0 {
			changed = true
			continue
		}
		selected = normalizePreferenceConnectionIDs(selected)
		selectedStrings := make([]string, 0, len(selected))
		for _, connectionID := range selected {
			selectedStrings = append(selectedStrings, connectionID.String())
		}
		preferred := preference.PreferredConnectionID
		if preferred == nil || *preferred == uuid.Nil || !containsPreferenceConnection(selected, *preferred) {
			fallback := selected[0]
			preferred = &fallback
			changed = true
		}
		sanitized := preference
		sanitized.IntegrationID = integrationID
		sanitized.SelectedConnectionIDs = selectedStrings
		sanitized.PreferredConnectionID = cloneUUIDPointer(preferred)
		if !sameAIChatIntegrationPreferenceSelection(preference, sanitized) {
			changed = true
		}
		repaired = append(repaired, sanitized)
	}
	sort.Slice(repaired, func(i, j int) bool { return repaired[i].IntegrationID < repaired[j].IntegrationID })
	if len(repaired) != len(stored) {
		changed = true
	}
	return repaired, changed, nil
}

func sameAIChatIntegrationPreferenceSnapshot(left, right []AIChatIntegrationPreference) bool {
	if len(left) != len(right) {
		return false
	}
	for index := range left {
		if left[index].ID != right[index].ID || left[index].Revision != right[index].Revision ||
			left[index].OrganizationID != right[index].OrganizationID || left[index].AccountID != right[index].AccountID ||
			!samePreferenceWorkspaceID(left[index].WorkspaceID, right[index].WorkspaceID) ||
			!sameAIChatIntegrationPreferenceSelection(left[index], right[index]) {
			return false
		}
	}
	return true
}

func samePreferenceWorkspaceID(left, right *uuid.UUID) bool {
	if left == nil || right == nil {
		return left == nil && right == nil
	}
	return *left == *right
}

func sameAIChatIntegrationPreferenceSelection(left, right AIChatIntegrationPreference) bool {
	if left.IntegrationID != right.IntegrationID || len(left.SelectedConnectionIDs) != len(right.SelectedConnectionIDs) {
		return false
	}
	for index := range left.SelectedConnectionIDs {
		if left.SelectedConnectionIDs[index] != right.SelectedConnectionIDs[index] {
			return false
		}
	}
	if left.PreferredConnectionID == nil || right.PreferredConnectionID == nil {
		return left.PreferredConnectionID == nil && right.PreferredConnectionID == nil
	}
	return *left.PreferredConnectionID == *right.PreferredConnectionID
}

func validateAIChatIntegrationPreference(preference *AIChatIntegrationPreference) error {
	if preference == nil || preference.OrganizationID == uuid.Nil || preference.AccountID == uuid.Nil || !integrationIdentifierPattern.MatchString(strings.ToLower(strings.TrimSpace(preference.IntegrationID))) {
		return invalidInput("AIChat integration preference identity is invalid", nil)
	}
	if len(preference.SelectedConnectionIDs) == 0 || len(preference.SelectedConnectionIDs) > 20 || preference.PreferredConnectionID == nil || *preference.PreferredConnectionID == uuid.Nil {
		return invalidInput("AIChat integration preference selection is invalid", nil)
	}
	for _, raw := range preference.SelectedConnectionIDs {
		connectionID, err := uuid.Parse(strings.TrimSpace(raw))
		if err != nil || connectionID == uuid.Nil {
			return invalidInput("AIChat integration preference connection is invalid", err)
		}
	}
	preferredFound := false
	for _, raw := range preference.SelectedConnectionIDs {
		if strings.EqualFold(strings.TrimSpace(raw), preference.PreferredConnectionID.String()) {
			preferredFound = true
			break
		}
	}
	if !preferredFound {
		return invalidInput("preferred connection must be selected", nil)
	}
	return nil
}

func connectionAvailableForSelection(connection *IntegrationConnection, now time.Time) bool {
	if connection == nil || !supportedConnectionCredentialSource(connection.CredentialSource) || connection.Status != ConnectionStatusActive {
		return false
	}
	if connection.HealthStatus == ConnectionHealthUnhealthy {
		return false
	}
	if connection.AuthStatus == ConnectionAuthReconnectRequired || connection.AuthStatus == ConnectionAuthExpired {
		return false
	}
	if connection.ExpiresAt != nil && !connection.ExpiresAt.After(now) {
		return false
	}
	if connection.RefreshTokenExpiresAt != nil && !connection.RefreshTokenExpiresAt.After(now) {
		return false
	}
	return connection.TokenExpiresAt == nil || connection.TokenExpiresAt.After(now)
}

func normalizePreferenceConnectionIDs(values []uuid.UUID) []uuid.UUID {
	seen := make(map[uuid.UUID]struct{}, len(values))
	result := make([]uuid.UUID, 0, len(values))
	for _, value := range values {
		if value == uuid.Nil {
			continue
		}
		if _, exists := seen[value]; exists {
			continue
		}
		seen[value] = struct{}{}
		result = append(result, value)
	}
	sort.Slice(result, func(i, j int) bool { return result[i].String() < result[j].String() })
	return result
}

func containsPreferenceConnection(values []uuid.UUID, target uuid.UUID) bool {
	for _, value := range values {
		if value == target {
			return true
		}
	}
	return false
}
