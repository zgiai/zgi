package model

// SystemFeature represents system feature configuration
type SystemFeature struct {
	EnableEmailCodeLogin        bool `json:"enable_email_code_login"`
	EnableEmailPasswordLogin    bool `json:"enable_email_password_login"`
	EnablePhoneLogin            bool `json:"enable_phone_login"`
	EnableSocialOAuthLogin      bool `json:"enable_social_oauth_login"`
	IsAllowRegister             bool `json:"is_allow_register"`
	IsAllowCreateWorkspace      bool `json:"is_allow_create_workspace"`
	IsEmailSetup                bool `json:"is_email_setup"`
	IsPublicDeployment          bool `json:"is_public_deployment"`
	EnableWebSSOSwitchComponent bool `json:"enable_web_sso_switch_component"`
	EnableMarketplace           bool `json:"enable_marketplace"`
	MaxPluginPackageSize        int  `json:"max_plugin_package_size"`
	NotificationSMS             any  `json:"notification_sms,omitempty"`
	WorkflowNodes               any  `json:"workflow_nodes,omitempty"`
	AutomationChannels          any  `json:"automation_channels,omitempty"`
}

// NewDefaultSystemFeature creates a new SystemFeature with default values
func NewDefaultSystemFeature() *SystemFeature {
	return &SystemFeature{
		EnableEmailCodeLogin:        false,
		EnableEmailPasswordLogin:    true,
		EnablePhoneLogin:            false,
		EnableSocialOAuthLogin:      false,
		IsAllowRegister:             false,
		IsAllowCreateWorkspace:      false,
		IsEmailSetup:                false,
		IsPublicDeployment:          false,
		EnableWebSSOSwitchComponent: false,
		EnableMarketplace:           true,
		MaxPluginPackageSize:        0,
	}
}

// NewSystemFeatureResponse creates a response wrapper for SystemFeature
func NewSystemFeatureResponse(features interface{}) interface{} {
	return map[string]interface{}{
		"features": features,
	}
}
