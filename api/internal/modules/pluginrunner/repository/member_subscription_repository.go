package repository

import (
	"context"

	"github.com/zgiai/zgi/api/internal/modules/pluginrunner/model"
	"gorm.io/gorm"
)

// MemberSubscriptionRepository defines the interface for member-level plugin subscriptions
type MemberSubscriptionRepository interface {
	// Create adds a new subscription record
	Create(ctx context.Context, sub *model.OrgPluginSubscription) error

	// GetByMemberAndInstallation finds a subscription by member and installation
	GetByMemberAndInstallation(ctx context.Context, groupID, accountID, installationID string) (*model.OrgPluginSubscription, error)

	// ListByMember returns all subscriptions for a specific member
	ListByMember(ctx context.Context, groupID, accountID string) ([]model.OrgPluginSubscription, error)

	// ListByGroup returns all subscriptions for an organization
	ListByGroup(ctx context.Context, groupID string) ([]model.OrgPluginSubscription, error)

	// ListByInstallation returns all subscribers for a specific installation
	ListByInstallation(ctx context.Context, installationID string) ([]model.OrgPluginSubscription, error)

	// Delete removes a subscription
	Delete(ctx context.Context, id uint) error

	// DeleteByMemberAndInstallation removes a subscription by member and installation
	DeleteByMemberAndInstallation(ctx context.Context, groupID, accountID, installationID string) error

	// CountByInstallation counts the number of subscribers for an installation
	CountByInstallation(ctx context.Context, installationID string) (int64, error)

	// Update updates a subscription record
	Update(ctx context.Context, sub *model.OrgPluginSubscription) error
}

type memberSubscriptionRepository struct {
	db *gorm.DB
}

// NewMemberSubscriptionRepository creates a new member subscription repository
func NewMemberSubscriptionRepository(db *gorm.DB) MemberSubscriptionRepository {
	return &memberSubscriptionRepository{db: db}
}

func (r *memberSubscriptionRepository) Create(ctx context.Context, sub *model.OrgPluginSubscription) error {
	return r.db.WithContext(ctx).Create(sub).Error
}

func (r *memberSubscriptionRepository) GetByMemberAndInstallation(ctx context.Context, groupID, accountID, installationID string) (*model.OrgPluginSubscription, error) {
	var sub model.OrgPluginSubscription
	err := r.db.WithContext(ctx).
		Where("group_id = ? AND account_id = ? AND installation_id = ?", groupID, accountID, installationID).
		First(&sub).Error
	if err != nil {
		return nil, err
	}
	return &sub, nil
}

func (r *memberSubscriptionRepository) ListByMember(ctx context.Context, groupID, accountID string) ([]model.OrgPluginSubscription, error) {
	var subs []model.OrgPluginSubscription
	err := r.db.WithContext(ctx).
		Where("group_id = ? AND account_id = ? AND enabled = true", groupID, accountID).
		Find(&subs).Error
	return subs, err
}

func (r *memberSubscriptionRepository) ListByGroup(ctx context.Context, groupID string) ([]model.OrgPluginSubscription, error) {
	var subs []model.OrgPluginSubscription
	err := r.db.WithContext(ctx).
		Where("group_id = ? AND enabled = true", groupID).
		Find(&subs).Error
	return subs, err
}

func (r *memberSubscriptionRepository) ListByInstallation(ctx context.Context, installationID string) ([]model.OrgPluginSubscription, error) {
	var subs []model.OrgPluginSubscription
	err := r.db.WithContext(ctx).
		Where("installation_id = ?", installationID).
		Find(&subs).Error
	return subs, err
}

func (r *memberSubscriptionRepository) Delete(ctx context.Context, id uint) error {
	return r.db.WithContext(ctx).Delete(&model.OrgPluginSubscription{}, id).Error
}

func (r *memberSubscriptionRepository) DeleteByMemberAndInstallation(ctx context.Context, groupID, accountID, installationID string) error {
	return r.db.WithContext(ctx).
		Where("group_id = ? AND account_id = ? AND installation_id = ?", groupID, accountID, installationID).
		Delete(&model.OrgPluginSubscription{}).Error
}

func (r *memberSubscriptionRepository) CountByInstallation(ctx context.Context, installationID string) (int64, error) {
	var count int64
	err := r.db.WithContext(ctx).
		Model(&model.OrgPluginSubscription{}).
		Where("installation_id = ?", installationID).
		Count(&count).Error
	return count, err
}

func (r *memberSubscriptionRepository) Update(ctx context.Context, sub *model.OrgPluginSubscription) error {
	return r.db.WithContext(ctx).Save(sub).Error
}
