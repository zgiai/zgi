package service

import (
	"context"

	"github.com/zgiai/zgi/api/config"
	notificationsms "github.com/zgiai/zgi/api/internal/modules/notification/sms"
	interfaces "github.com/zgiai/zgi/api/internal/modules/shared/interface"
	"github.com/zgiai/zgi/api/internal/modules/system/model"
)

// featureService implements the FeatureService interface
type featureService struct {
	// Add any dependencies here if needed
}

// NewFeatureService creates a new feature service instance
func NewFeatureService() interfaces.FeatureService {
	return &featureService{}
}

func (s *featureService) GetSystemFeatures(ctx context.Context) (interface{}, error) {
	sf := model.NewDefaultSystemFeature()
	s.fillFromEnv(sf)
	if s.isEnterpriseEnabled() {
		sf.EnableWebSSOSwitchComponent = true
		s.fillFromEnterprise(sf)
	}
	if s.isMarketplaceEnabled() {
		sf.EnableMarketplace = true
	}
	return sf, nil
}

func (s *featureService) IsPublicDeployment() bool {
	return config.Current().Feature.PublicDeploymentEnabled
}

func (s *featureService) IsFeatureEnabled(featureName string) bool {
	features := config.Current().Feature
	switch featureName {
	case "email_code_login":
		return features.EnableEmailCodeLogin
	case "email_password_login":
		return features.EnableEmailPasswordLogin
	case "phone_login":
		return features.EnablePhoneLogin
	case "social_oauth_login":
		return features.EnableSocialOAuthLogin
	case "allow_register":
		return features.AllowRegister
	case "allow_create_workspace":
		return features.AllowCreateWorkspace
	case "enterprise":
		return s.isEnterpriseEnabled()
	case "marketplace":
		return s.isMarketplaceEnabled()
	default:
		return false
	}
}

func (s *featureService) fillFromEnv(sf *model.SystemFeature) {
	cfg := config.Current()
	features := cfg.Feature
	sf.EnableEmailCodeLogin = features.EnableEmailCodeLogin
	sf.EnableEmailPasswordLogin = features.EnableEmailPasswordLogin
	sf.EnablePhoneLogin = features.EnablePhoneLogin
	sf.EnableSocialOAuthLogin = features.EnableSocialOAuthLogin
	sf.IsAllowRegister = features.AllowRegister
	sf.IsAllowCreateWorkspace = features.AllowCreateWorkspace

	sf.IsEmailSetup = config.HasEmailDeliveryConfig(cfg)
	sf.IsPublicDeployment = s.IsPublicDeployment()

	if features.PluginMaxPackageSize > 0 {
		sf.MaxPluginPackageSize = features.PluginMaxPackageSize
	}

	smsCapability := notificationsms.ConfigFromLookup(config.Lookup).Capability()
	sf.NotificationSMS = smsCapability
	sf.WorkflowNodes = map[string]any{
		"notification-sms": map[string]any{
			"enabled": smsCapability.Enabled,
			"feature": notificationsms.FeatureNotificationSMS,
		},
	}
	sf.AutomationChannels = map[string]any{
		"sms": map[string]any{
			"enabled": smsCapability.Enabled,
			"feature": notificationsms.FeatureNotificationSMS,
		},
	}
}

func (s *featureService) isEnterpriseEnabled() bool {
	return config.Current().Feature.EnterpriseEnabled
}

func (s *featureService) isMarketplaceEnabled() bool {
	return config.Current().Feature.MarketplaceEnabled
}

func (s *featureService) fillFromEnterprise(sf *model.SystemFeature) {
	// TODO: integrate enterprise settings
}
