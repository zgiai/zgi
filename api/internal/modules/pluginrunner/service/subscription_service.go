package service

import (
	"context"
	"fmt"

	"github.com/zgiai/ginext/internal/modules/pluginrunner/model"
	"github.com/zgiai/ginext/internal/modules/pluginrunner/repository"
)

// SubscriptionService handles organization plugin subscription operations
type SubscriptionService interface {
	// Subscribe subscribes an organization to a plugin
	Subscribe(ctx context.Context, groupID string, pluginID string, subscribedBy string, config string) (*model.OrgPluginSubscription, error)
	// Unsubscribe removes an organization's subscription to a plugin
	Unsubscribe(ctx context.Context, groupID string, pluginID string) error
	// IsSubscribed checks if an organization is subscribed to a plugin
	IsSubscribed(ctx context.Context, groupID string, pluginID string) (bool, error)
	// ListSubscriptions lists all subscriptions for an organization
	ListSubscriptions(ctx context.Context, groupID string) ([]model.OrgPluginSubscription, error)
	// GetSubscription gets a specific subscription
	GetSubscription(ctx context.Context, groupID string, pluginID string) (*model.OrgPluginSubscription, error)
}

type subscriptionServiceImpl struct {
	repo repository.SubscriptionRepository
}

// NewSubscriptionService creates a new subscription service
func NewSubscriptionService(repo repository.SubscriptionRepository) SubscriptionService {
	return &subscriptionServiceImpl{repo: repo}
}

// Subscribe subscribes an organization to a plugin
func (s *subscriptionServiceImpl) Subscribe(ctx context.Context, groupID string, pluginID string, subscribedBy string, config string) (*model.OrgPluginSubscription, error) {
	// Check if already subscribed
	existing, err := s.repo.GetByGroupAndPlugin(ctx, groupID, pluginID)
	if err != nil {
		return nil, fmt.Errorf("check existing subscription: %w", err)
	}

	if existing != nil {
		// Already subscribed, update if needed
		if !existing.Enabled {
			existing.Enabled = true
			existing.Config = config
			if err := s.repo.Update(ctx, existing); err != nil {
				return nil, fmt.Errorf("re-enable subscription: %w", err)
			}
		}
		return existing, nil
	}

	// Create new subscription
	sub := &model.OrgPluginSubscription{
		GroupID:      groupID,
		PluginID:     pluginID,
		Enabled:      true,
		Config:       config,
		SubscribedBy: subscribedBy,
	}
	if err := s.repo.Create(ctx, sub); err != nil {
		return nil, fmt.Errorf("create subscription: %w", err)
	}
	return sub, nil
}

// Unsubscribe removes an organization's subscription to a plugin
func (s *subscriptionServiceImpl) Unsubscribe(ctx context.Context, groupID string, pluginID string) error {
	// Soft delete by setting enabled = false
	existing, err := s.repo.GetByGroupAndPlugin(ctx, groupID, pluginID)
	if err != nil {
		return fmt.Errorf("get subscription: %w", err)
	}
	if existing == nil {
		return nil // Not subscribed, nothing to do
	}

	existing.Enabled = false
	return s.repo.Update(ctx, existing)
}

// IsSubscribed checks if an organization is subscribed to a plugin
func (s *subscriptionServiceImpl) IsSubscribed(ctx context.Context, groupID string, pluginID string) (bool, error) {
	return s.repo.IsSubscribed(ctx, groupID, pluginID)
}

// ListSubscriptions lists all subscriptions for an organization
func (s *subscriptionServiceImpl) ListSubscriptions(ctx context.Context, groupID string) ([]model.OrgPluginSubscription, error) {
	return s.repo.ListByGroup(ctx, groupID)
}

// GetSubscription gets a specific subscription
func (s *subscriptionServiceImpl) GetSubscription(ctx context.Context, groupID string, pluginID string) (*model.OrgPluginSubscription, error) {
	return s.repo.GetByGroupAndPlugin(ctx, groupID, pluginID)
}
