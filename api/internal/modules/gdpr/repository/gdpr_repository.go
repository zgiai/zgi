package repository

import (
	"context"

	"github.com/google/uuid"
	"github.com/zgiai/ginext/internal/modules/gdpr/model"
	"gorm.io/gorm"
)

// GDPRRepository defines the interface for GDPR data operations
type GDPRRepository interface {
	// Audit Logs
	CreateAuditLog(ctx context.Context, log *model.GDPRAuditLog) error

	// Retention Policies
	GetAllRetentionPolicies(ctx context.Context) ([]*model.DataRetentionPolicy, error)

	// User Consents
	GetConsentsByAccount(ctx context.Context, accountID uuid.UUID) ([]*model.UserConsent, error)
	GetConsent(ctx context.Context, accountID uuid.UUID, consentType model.ConsentType) (*model.UserConsent, error)
	UpsertConsent(ctx context.Context, consent *model.UserConsent) error

	// Data Operations (for erasure)
	AnonymizeAPIKeys(ctx context.Context, tenantID uuid.UUID, accountID uuid.UUID, anonymizedID string) (int64, error)
	GetAccountWorkspaces(ctx context.Context, accountID uuid.UUID) ([]uuid.UUID, error)
}

type gdprRepository struct {
	db *gorm.DB
}

// NewGDPRRepository creates a new GDPR repository
func NewGDPRRepository(db *gorm.DB) GDPRRepository {
	return &gdprRepository{db: db}
}

// ============================================================================
// Audit Logs
// ============================================================================

func (r *gdprRepository) CreateAuditLog(ctx context.Context, log *model.GDPRAuditLog) error {
	return r.db.WithContext(ctx).Create(log).Error
}

// ============================================================================
// Retention Policies
// ============================================================================

func (r *gdprRepository) GetAllRetentionPolicies(ctx context.Context) ([]*model.DataRetentionPolicy, error) {
	var policies []*model.DataRetentionPolicy
	err := r.db.WithContext(ctx).Find(&policies).Error
	return policies, err
}

// ============================================================================
// User Consents
// ============================================================================

func (r *gdprRepository) GetConsentsByAccount(ctx context.Context, accountID uuid.UUID) ([]*model.UserConsent, error) {
	var consents []*model.UserConsent
	err := r.db.WithContext(ctx).
		Where("account_id = ?", accountID).
		Order("consent_type ASC").
		Find(&consents).Error
	return consents, err
}

func (r *gdprRepository) GetConsent(ctx context.Context, accountID uuid.UUID, consentType model.ConsentType) (*model.UserConsent, error) {
	var consent model.UserConsent
	err := r.db.WithContext(ctx).
		Where("account_id = ? AND consent_type = ?", accountID, consentType).
		First(&consent).Error
	if err != nil {
		return nil, err
	}
	return &consent, nil
}

func (r *gdprRepository) UpsertConsent(ctx context.Context, consent *model.UserConsent) error {
	return r.db.WithContext(ctx).
		Where("account_id = ? AND consent_type = ?", consent.AccountID, consent.ConsentType).
		Assign(consent).
		FirstOrCreate(consent).Error
}

// ============================================================================
// Data Operations
// ============================================================================

func (r *gdprRepository) AnonymizeAPIKeys(ctx context.Context, tenantID uuid.UUID, accountID uuid.UUID, anonymizedID string) (int64, error) {
	// Note: API keys are tenant-scoped, not account-scoped
	// This is a placeholder for future implementation if needed
	return 0, nil
}

func (r *gdprRepository) GetAccountWorkspaces(ctx context.Context, accountID uuid.UUID) ([]uuid.UUID, error) {
	var tenantIDs []uuid.UUID
	err := r.db.WithContext(ctx).
		Table("workspace_members").
		Where("account_id = ?", accountID).
		Pluck("workspace_id", &tenantIDs).Error
	return tenantIDs, err
}
