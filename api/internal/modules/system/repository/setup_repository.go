package repository

import (
	"errors"
	"strings"
	"time"

	"github.com/zgiai/zgi/api/internal/modules/system/model"
	"gorm.io/gorm"
)

// SetupRepository handles system setup related data operations
// Cross-module operations are handled at the service layer to avoid circular dependencies
type SetupRepository interface {
	GetSetupStatus() (*model.Setup, error)
	GetTenantCount() (int64, error)
	GetInitValidateStatus() (bool, error)
	CreateSetup() error
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
	var setup model.Setup
	result := r.db.First(&setup)
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

func (r *setupRepository) CreateSetup() error {
	setup := model.Setup{
		Version: "1.0",
		SetupAt: time.Now(),
	}
	result := r.db.Create(&setup)
	return result.Error
}

// Cross-module operations (CreateWorkspace, CreateAccount, CreateWorkspaceMember)
// are removed from repository layer and will be handled at service layer
// to avoid circular dependencies in modular architecture
