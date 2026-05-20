package service

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"github.com/google/uuid"
	"github.com/zgiai/ginext/internal/modules/pluginrunner/model"
	"github.com/zgiai/ginext/internal/modules/pluginrunner/repository"
	"github.com/zgiai/ginext/pkg/logger"
	"gorm.io/gorm"
)

// MemberSubscriptionService defines the interface for member subscription operations
type MemberSubscriptionService interface {
	// Subscribe adds a member subscription to a tenant-installed plugin
	Subscribe(ctx context.Context, groupID, accountID, installationID, pluginID, subscribedBy, config string) (*model.OrgPluginSubscription, error)

	// Unsubscribe removes a member's subscription
	Unsubscribe(ctx context.Context, groupID, accountID, pluginID string) error

	// ListSubscribedPlugins returns all plugins subscribed by a member
	ListSubscribedPlugins(ctx context.Context, groupID, accountID string) ([]model.OrgPluginSubscription, error)

	// IsSubscribed checks if a member is subscribed to a specific plugin
	IsSubscribed(ctx context.Context, groupID, accountID, pluginID string) (bool, error)

	// ListSubscribedDeclarations returns declarations for all plugins subscribed by a member
	ListSubscribedDeclarations(ctx context.Context, groupID, accountID string) ([]model.PluginDeclaration, error)

	// CanDeleteInstallation checks if an installation can be deleted (no subscribers)
	CanDeleteInstallation(ctx context.Context, installationID string) (bool, error)

	// GetSubscriberCount returns the number of subscribers for an installation
	GetSubscriberCount(ctx context.Context, installationID string) (int64, error)
}

type memberSubscriptionService struct {
	subRepo     repository.MemberSubscriptionRepository
	installRepo repository.AccountInstallationRepository
	infoRepo    repository.InstalledPluginInfoRepository
}

// NewMemberSubscriptionService creates a new member subscription service
func NewMemberSubscriptionService(
	subRepo repository.MemberSubscriptionRepository,
	installRepo repository.AccountInstallationRepository,
	infoRepo repository.InstalledPluginInfoRepository,
) MemberSubscriptionService {
	return &memberSubscriptionService{
		subRepo:     subRepo,
		installRepo: installRepo,
		infoRepo:    infoRepo,
	}
}

var errPluginNotInstalled = errors.New("plugin is not installed or active for this organization")

// Subscribe adds a member subscription to a plugin installation
func (s *memberSubscriptionService) Subscribe(ctx context.Context, groupID, accountID, installationID, pluginID, subscribedBy, config string) (*model.OrgPluginSubscription, error) {
	// 1. Resolve active installation ID
	realInstallationID, err := s.resolveInstallationID(ctx, groupID, pluginID, installationID)
	if err != nil {
		return nil, err
	}

	// Check if already subscribed using the real installation ID
	existing, err := s.subRepo.GetByMemberAndInstallation(ctx, groupID, accountID, realInstallationID)
	if err == nil {
		// Update config if exists
		if existing.Config != config {
			existing.Config = config
			if err := s.subRepo.Update(ctx, existing); err != nil {
				return nil, fmt.Errorf("failed to update subscription config: %w", err)
			}
		}
		return existing, nil // Already subscribed
	}
	if !errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, fmt.Errorf("failed to load subscription: %w", err)
	}

	sub := &model.OrgPluginSubscription{
		GroupID:        groupID,
		AccountID:      accountID,
		InstallationID: realInstallationID,
		PluginID:       pluginID,
		Enabled:        true,
		Config:         config,
		SubscribedBy:   subscribedBy,
	}

	if err := s.subRepo.Create(ctx, sub); err != nil {
		return nil, fmt.Errorf("failed to create subscription: %w", err)
	}

	return sub, nil
}

// Unsubscribe removes a member's subscription
func (s *memberSubscriptionService) Unsubscribe(ctx context.Context, groupID, accountID, pluginID string) error {
	realInstallationID, err := s.resolveActiveInstallation(ctx, groupID, pluginID)
	if err != nil {
		if errors.Is(err, errPluginNotInstalled) {
			return nil
		}
		return err
	}
	return s.subRepo.DeleteByMemberAndInstallation(ctx, groupID, accountID, realInstallationID)
}

// ListSubscribedPlugins returns all plugins subscribed by a member
func (s *memberSubscriptionService) ListSubscribedPlugins(ctx context.Context, groupID, accountID string) ([]model.OrgPluginSubscription, error) {
	return s.subRepo.ListByMember(ctx, groupID, accountID)
}

// IsSubscribed checks if a member is subscribed to a specific plugin
func (s *memberSubscriptionService) IsSubscribed(ctx context.Context, groupID, accountID, pluginID string) (bool, error) {
	realInstallationID, err := s.resolveActiveInstallation(ctx, groupID, pluginID)
	if err != nil {
		// If plugin is not installed, implicitly not subscribed
		if errors.Is(err, errPluginNotInstalled) {
			return false, nil
		}
		return false, err
	}

	_, err = s.subRepo.GetByMemberAndInstallation(ctx, groupID, accountID, realInstallationID)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return false, nil
		}
		return false, err
	}
	return true, nil
}

// CanDeleteInstallation checks if an installation can be deleted (no active subscribers)
func (s *memberSubscriptionService) CanDeleteInstallation(ctx context.Context, installationID string) (bool, error) {
	count, err := s.subRepo.CountByInstallation(ctx, installationID)
	if err != nil {
		return false, err
	}
	return count == 0, nil
}

// GetSubscriberCount returns the number of subscribers for an installation
func (s *memberSubscriptionService) GetSubscriberCount(ctx context.Context, installationID string) (int64, error) {
	return s.subRepo.CountByInstallation(ctx, installationID)
}

func (s *memberSubscriptionService) resolveInstallationID(ctx context.Context, groupID, pluginID, installationID string) (string, error) {
	if installationID != "" {
		return s.resolveInstallationByID(ctx, groupID, installationID)
	}
	return s.resolveActiveInstallation(ctx, groupID, pluginID)
}

// resolveActiveInstallation finds the active installation for a plugin in an organization.
func (s *memberSubscriptionService) resolveActiveInstallation(ctx context.Context, groupID, pluginID string) (string, error) {
	if pluginID == "" {
		return "", fmt.Errorf("plugin_id is required")
	}

	if isUUID(pluginID) {
		if id, found, err := s.findActiveByVersionID(ctx, groupID, pluginID); err != nil {
			return "", err
		} else if found {
			return id, nil
		}
		if id, found, err := s.findActiveByPluginID(ctx, groupID, pluginID); err != nil {
			return "", err
		} else if found {
			return id, nil
		}
	}

	if name, version, ok := splitPluginNameVersion(pluginID); ok {
		if id, found, err := s.findActiveByNameVersion(ctx, groupID, name, version); err != nil {
			return "", err
		} else if found {
			return id, nil
		}
	}

	return "", errPluginNotInstalled
}

func (s *memberSubscriptionService) resolveInstallationByID(ctx context.Context, groupID, installationID string) (string, error) {
	if !isUUID(installationID) {
		return "", fmt.Errorf("invalid installation_id")
	}
	inst, err := s.installRepo.GetByID(ctx, installationID)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return "", errPluginNotInstalled
		}
		return "", fmt.Errorf("failed to load installation: %w", err)
	}
	if inst.TenantID != groupID || inst.Status != model.InstallationStatusActive {
		return "", errPluginNotInstalled
	}
	return inst.ID, nil
}

func (s *memberSubscriptionService) findActiveByVersionID(ctx context.Context, groupID, versionID string) (string, bool, error) {
	inst, err := s.installRepo.GetByTenantAndVersion(ctx, groupID, versionID)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return "", false, nil
		}
		return "", false, fmt.Errorf("failed to load installation: %w", err)
	}
	if inst.Status != model.InstallationStatusActive {
		return "", false, nil
	}
	return inst.ID, true, nil
}

func (s *memberSubscriptionService) findActiveByPluginID(ctx context.Context, groupID, pluginID string) (string, bool, error) {
	installations, err := s.installRepo.ListByTenantAndPlugin(ctx, groupID, pluginID)
	if err != nil {
		return "", false, fmt.Errorf("failed to list installations: %w", err)
	}
	for _, inst := range installations {
		if inst.Status == model.InstallationStatusActive {
			return inst.ID, true, nil
		}
	}
	return "", false, nil
}

func (s *memberSubscriptionService) findActiveByNameVersion(ctx context.Context, groupID, name, version string) (string, bool, error) {
	if s.infoRepo == nil {
		return "", false, nil
	}
	info, err := s.infoRepo.GetByNameAndVersion(ctx, name, version)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return "", false, nil
		}
		return "", false, fmt.Errorf("failed to load plugin info: %w", err)
	}
	return s.findActiveByVersionID(ctx, groupID, info.MarketplaceVersionID)
}

func splitPluginNameVersion(pluginID string) (string, string, bool) {
	if pluginID == "" {
		return "", "", false
	}
	if parts := strings.Split(pluginID, "@"); len(parts) == 2 && parts[0] != "" && parts[1] != "" {
		return parts[0], parts[1], true
	}
	parts := strings.Split(pluginID, ":")
	if len(parts) == 2 && parts[0] != "" && parts[1] != "" {
		return parts[0], parts[1], true
	}
	if len(parts) == 3 && parts[1] != "" && parts[2] != "" {
		return parts[1], parts[2], true
	}
	return "", "", false
}

func isUUID(value string) bool {
	_, err := uuid.Parse(value)
	return err == nil
}

// ListSubscribedDeclarations returns declarations for all plugins subscribed by a member
func (s *memberSubscriptionService) ListSubscribedDeclarations(ctx context.Context, groupID, accountID string) ([]model.PluginDeclaration, error) {
	subscriptions, err := s.ListSubscribedPlugins(ctx, groupID, accountID)
	if err != nil {
		return nil, err
	}

	var declarations []model.PluginDeclaration
	for _, sub := range subscriptions {
		if !sub.Enabled {
			continue
		}

		installationID := sub.InstallationID
		// If InstallationID is empty (legacy), try to resolve it
		if installationID == "" {
			resolvedID, err := s.resolveActiveInstallation(ctx, groupID, sub.PluginID)
			if err != nil {
				logger.Warn("failed to resolve installation for subscription, skipping", "plugin_id", sub.PluginID, "error", err)
				continue
			}
			installationID = resolvedID
		}

		inst, err := s.installRepo.GetByID(ctx, installationID)
		if err != nil {
			logger.Warn("failed to load installation for subscription, skipping", "installation_id", installationID, "error", err)
			continue
		}

		if inst.Status != model.InstallationStatusActive {
			continue
		}

		info, err := s.infoRepo.GetByMarketplaceVersionID(ctx, inst.MarketplaceVersionID)
		if err != nil {
			logger.Warn("plugin info not found for installation, skipping", "version_id", inst.MarketplaceVersionID, "error", err)
			continue
		}

		var parsed model.PluginDeclaration
		if err := json.Unmarshal(info.Declaration, &parsed); err != nil {
			logger.Error(fmt.Sprintf("failed to unmarshal plugin declaration, skipping version_id=%s", inst.MarketplaceVersionID), err)
			continue
		}
		declarations = append(declarations, parsed)
	}

	return declarations, nil
}
