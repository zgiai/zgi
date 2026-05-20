package repository

import (
	"context"
	"time"

	"gorm.io/gorm"

	auth_model "github.com/zgiai/ginext/internal/modules/user/auth/model"
)

type InvitationRepository interface {
	Create(ctx context.Context, invitation *auth_model.InvitationCode) error
	GetByCode(ctx context.Context, code string) (*auth_model.InvitationCode, error)
	GetByID(ctx context.Context, id string) (*auth_model.InvitationCode, error)
	Update(ctx context.Context, invitation *auth_model.InvitationCode) error
	Delete(ctx context.Context, id string) error
	GetValidByCode(ctx context.Context, code string) (*auth_model.InvitationCode, error)
	GetByTenantID(ctx context.Context, tenantID string, limit, offset int) ([]*auth_model.InvitationCode, error)
	ExpireOldCodes(ctx context.Context, before time.Time) error
}

type invitationRepository struct {
	db *gorm.DB
}

func NewInvitationRepository(db *gorm.DB) InvitationRepository {
	return &invitationRepository{db: db}
}

func (r *invitationRepository) Create(ctx context.Context, invitation *auth_model.InvitationCode) error {
	return r.db.WithContext(ctx).Create(invitation).Error
}

func (r *invitationRepository) GetByCode(ctx context.Context, code string) (*auth_model.InvitationCode, error) {
	var invitation auth_model.InvitationCode
	err := r.db.WithContext(ctx).Where("code = ?", code).First(&invitation).Error
	if err != nil {
		return nil, err
	}
	return &invitation, nil
}

func (r *invitationRepository) GetByID(ctx context.Context, id string) (*auth_model.InvitationCode, error) {
	var invitation auth_model.InvitationCode
	err := r.db.WithContext(ctx).Where("id = ?", id).First(&invitation).Error
	if err != nil {
		return nil, err
	}
	return &invitation, nil
}

func (r *invitationRepository) Update(ctx context.Context, invitation *auth_model.InvitationCode) error {
	return r.db.WithContext(ctx).Save(invitation).Error
}

func (r *invitationRepository) Delete(ctx context.Context, id string) error {
	return r.db.WithContext(ctx).Where("id = ?", id).Delete(&auth_model.InvitationCode{}).Error
}

func (r *invitationRepository) GetValidByCode(ctx context.Context, code string) (*auth_model.InvitationCode, error) {
	var invitation auth_model.InvitationCode
	err := r.db.WithContext(ctx).Where("code = ? AND status = ? AND expires_at > ?",
		code, auth_model.InvitationStatusPending, time.Now()).First(&invitation).Error
	if err != nil {
		return nil, err
	}
	return &invitation, nil
}

func (r *invitationRepository) GetByTenantID(ctx context.Context, tenantID string, limit, offset int) ([]*auth_model.InvitationCode, error) {
	var invitations []*auth_model.InvitationCode
	err := r.db.WithContext(ctx).Where("tenant_id = ?", tenantID).Limit(limit).Offset(offset).Find(&invitations).Error
	return invitations, err
}

func (r *invitationRepository) ExpireOldCodes(ctx context.Context, before time.Time) error {
	return r.db.WithContext(ctx).Model(&auth_model.InvitationCode{}).Where("expires_at < ? AND status = ?",
		before, auth_model.InvitationStatusPending).Update("status", auth_model.InvitationStatusExpired).Error
}
