package repository

import (
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/zgiai/zgi/api/internal/modules/system/model"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// SetupRepository handles system setup related data operations
// Cross-module operations are handled at the service layer to avoid circular dependencies
type SetupRepository interface {
	GetSetupStatus() (*model.Setup, error)
	GetSetupStatusForUpdate() (*model.Setup, error)
	GetTenantCount() (int64, error)
	GetInitValidateStatus() (bool, error)
	CreateSetup(organizationID, workspaceID string) error
	UpdateSetupScope(version, organizationID, workspaceID string) error
}

type setupRepository struct {
	db *gorm.DB
}

func NewSetupRepository(db *gorm.DB) SetupRepository {
	return &setupRepository{
		db: db,
	}
}

func (r *setupRepository) GetSetupStatus() (*model.Setup, error) {
	return r.getSetupStatus(r.db)
}

// GetSetupStatusForUpdate reads the setup marker while locking it for the
// caller's transaction. Callers must invoke it from an existing transaction.
func (r *setupRepository) GetSetupStatusForUpdate() (*model.Setup, error) {
	return r.getSetupStatus(r.db.Clauses(clause.Locking{Strength: "UPDATE"}))
}

func (r *setupRepository) getSetupStatus(db *gorm.DB) (*model.Setup, error) {
	var setup model.Setup
	result := db.First(&setup)
	if result.Error != nil {
		if errors.Is(result.Error, gorm.ErrRecordNotFound) || isMissingSetupTableError(result.Error) {
			return nil, nil
		}
		return nil, result.Error
	}
	return &setup, nil
}

func isMissingSetupTableError(err error) bool {
	if err == nil {
		return false
	}

	msg := strings.ToLower(err.Error())
	return strings.Contains(msg, `relation "zgi_setups" does not exist`) ||
		strings.Contains(msg, "no such table: zgi_setups") ||
		(strings.Contains(msg, "zgi_setups") && strings.Contains(msg, "42p01"))
}

func (r *setupRepository) GetTenantCount() (int64, error) {
	// Direct count from workspaces table to avoid cross-module dependency
	var count int64
	err := r.db.Table("workspaces").Count(&count).Error
	return count, err
}

func (r *setupRepository) GetInitValidateStatus() (bool, error) {
	return true, nil
}

func (r *setupRepository) CreateSetup(organizationID, workspaceID string) error {
	organizationID = strings.TrimSpace(organizationID)
	workspaceID = strings.TrimSpace(workspaceID)
	if organizationID == "" || workspaceID == "" {
		return fmt.Errorf("setup organization_id and workspace_id are required")
	}

	setup := model.Setup{
		Version:        "1.0",
		SetupAt:        time.Now(),
		OrganizationID: &organizationID,
		WorkspaceID:    &workspaceID,
	}
	result := r.db.Create(&setup)
	return result.Error
}

func (r *setupRepository) UpdateSetupScope(version, organizationID, workspaceID string) error {
	version = strings.TrimSpace(version)
	organizationID = strings.TrimSpace(organizationID)
	workspaceID = strings.TrimSpace(workspaceID)
	if version == "" || organizationID == "" || workspaceID == "" {
		return fmt.Errorf("setup version, organization_id, and workspace_id are required")
	}

	result := r.db.Model(&model.Setup{}).
		Where("version = ?", version).
		Updates(map[string]any{
			"organization_id": organizationID,
			"workspace_id":    workspaceID,
		})
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected != 1 {
		return fmt.Errorf("setup marker %q was not found", version)
	}
	return nil
}

// Cross-module operations (CreateWorkspace, CreateAccount, CreateWorkspaceMember)
// are removed from repository layer and will be handled at service layer
// to avoid circular dependencies in modular architecture
