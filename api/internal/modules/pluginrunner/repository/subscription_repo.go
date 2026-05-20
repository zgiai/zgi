package repository

import (
	"context"
	"errors"

	"github.com/zgiai/zgi/api/internal/modules/pluginrunner/model"
	"gorm.io/gorm"
)

// SubscriptionRepository handles database operations for OrgPluginSubscription
type SubscriptionRepository interface {
	Create(ctx context.Context, sub *model.OrgPluginSubscription) error
	GetByGroupAndPlugin(ctx context.Context, groupID string, pluginID string) (*model.OrgPluginSubscription, error)
	ListByGroup(ctx context.Context, groupID string) ([]model.OrgPluginSubscription, error)
	Update(ctx context.Context, sub *model.OrgPluginSubscription) error
	Delete(ctx context.Context, groupID string, pluginID string) error
	IsSubscribed(ctx context.Context, groupID string, pluginID string) (bool, error)
}

type subscriptionRepository struct {
	db *gorm.DB
}

// NewSubscriptionRepository creates a new subscription repository
func NewSubscriptionRepository(db *gorm.DB) SubscriptionRepository {
	return &subscriptionRepository{db: db}
}

// Create creates a new subscription
func (r *subscriptionRepository) Create(ctx context.Context, sub *model.OrgPluginSubscription) error {
	return r.db.WithContext(ctx).Create(sub).Error
}

// GetByGroupAndPlugin retrieves a subscription by organization and plugin ID
func (r *subscriptionRepository) GetByGroupAndPlugin(ctx context.Context, groupID string, pluginID string) (*model.OrgPluginSubscription, error) {
	var sub model.OrgPluginSubscription
	err := r.db.WithContext(ctx).
		Where("group_id = ? AND plugin_id = ?", groupID, pluginID).
		First(&sub).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil
		}
		return nil, err
	}
	return &sub, nil
}

// ListByGroup retrieves all subscriptions for an organization
func (r *subscriptionRepository) ListByGroup(ctx context.Context, groupID string) ([]model.OrgPluginSubscription, error) {
	var subs []model.OrgPluginSubscription
	err := r.db.WithContext(ctx).
		Where("group_id = ? AND enabled = ?", groupID, true).
		Find(&subs).Error
	return subs, err
}

// Update updates a subscription
func (r *subscriptionRepository) Update(ctx context.Context, sub *model.OrgPluginSubscription) error {
	return r.db.WithContext(ctx).Save(sub).Error
}

// Delete removes a subscription
func (r *subscriptionRepository) Delete(ctx context.Context, groupID string, pluginID string) error {
	return r.db.WithContext(ctx).
		Where("group_id = ? AND plugin_id = ?", groupID, pluginID).
		Delete(&model.OrgPluginSubscription{}).Error
}

// IsSubscribed checks if an organization is subscribed to a plugin
func (r *subscriptionRepository) IsSubscribed(ctx context.Context, groupID string, pluginID string) (bool, error) {
	var count int64
	err := r.db.WithContext(ctx).
		Model(&model.OrgPluginSubscription{}).
		Where("group_id = ? AND plugin_id = ? AND enabled = ?", groupID, pluginID, true).
		Count(&count).Error
	return count > 0, err
}
