package service

import (
	"context"
	"fmt"

	"gorm.io/gorm"

	"github.com/zgiai/zgi/api/internal/infra/platform/console"
	interfaces "github.com/zgiai/zgi/api/internal/modules/shared/interface"
	"github.com/zgiai/zgi/api/internal/modules/workspace/model"
	"github.com/zgiai/zgi/api/pkg/logger"
)

// OrganizationServiceImpl implements interfaces.OrganizationService
type OrganizationServiceImpl struct {
	db              *gorm.DB
	consoleProvider console.ConsoleProvider
}

// NewOrganizationManagementService creates a new instance of OrganizationService
func NewOrganizationManagementService(db *gorm.DB, consoleProvider console.ConsoleProvider) interfaces.OrganizationManagementService {
	return &OrganizationServiceImpl{
		db:              db,
		consoleProvider: consoleProvider,
	}
}

// CreateOrganization creates a new organization organization
func (s *OrganizationServiceImpl) CreateOrganization(ctx context.Context, name string) (*model.Organization, error) {
	organization := &model.Organization{
		Name:   name,
		Status: model.OrganizationStatusActive,
	}

	if err := s.db.WithContext(ctx).Create(organization).Error; err != nil {
		return nil, fmt.Errorf("failed to create organization: %w", err)
	}

	// Async sync to Console-API (non-blocking, best-effort)
	if s.consoleProvider != nil && s.consoleProvider.IsAvailable() {
		if err := s.consoleProvider.RegisterOrganization(ctx, &console.RegisterOrganizationRequest{
			OrganizationID: organization.ID,
			Name:           organization.Name,
			CreatedAt:      organization.CreatedAt,
		}); err != nil {
			logger.Warn("Failed to sync organization to console: %v", err)
		}
	}

	return organization, nil
}

func (s *OrganizationServiceImpl) CheckOrganizationNameExists(ctx context.Context, name string) (bool, error) {
	var count int64
	err := s.db.WithContext(ctx).Model(&model.Organization{}).Where("name = ?", name).Count(&count).Error
	return count > 0, err
}

// UpsertOrganizationRole updates or inserts a organization role
func (s *OrganizationServiceImpl) UpsertOrganizationRole(ctx context.Context, organizationID string, accountID string, role model.OrganizationRole) error {
	organizationRole := &model.OrganizationMember{
		OrganizationID: organizationID,
		AccountID:      accountID,
		Role:           role,
	}

	result := s.db.WithContext(ctx).Where("organization_id = ? AND account_id = ?", organizationID, accountID).
		Assign(map[string]interface{}{"role": role}).
		FirstOrCreate(organizationRole)

	if result.Error != nil {
		return fmt.Errorf("failed to upsert organization role: %w", result.Error)
	}

	return nil
}

// AddWorkspace adds a tenant to a organization
func (s *OrganizationServiceImpl) AddWorkspace(ctx context.Context, organizationID string, workspaceID string) error {
	if err := s.db.WithContext(ctx).Table("workspaces").Where("id = ?", workspaceID).Update("organization_id", organizationID).Error; err != nil {
		return fmt.Errorf("failed to add tenant to organization: %w", err)
	}

	return nil
}

func (s *OrganizationServiceImpl) WithTx(tx *gorm.DB) interfaces.OrganizationManagementService {
	return NewOrganizationManagementService(tx, s.consoleProvider)
}
