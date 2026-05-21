package sms

import (
	"context"
	"fmt"
	"strings"
)

type NotificationSMSService struct {
	config    Config
	providers map[string]Provider
}

func NewService(config Config, providers ...Provider) *NotificationSMSService {
	byName := make(map[string]Provider, len(providers))
	for _, provider := range providers {
		if provider != nil {
			byName[provider.Provider()] = provider
		}
	}
	return &NotificationSMSService{config: config, providers: byName}
}

func (s *NotificationSMSService) IsEnabled() bool {
	return s != nil && s.config.Capability().Enabled
}

func (s *NotificationSMSService) Send(ctx context.Context, req Request) (*Result, error) {
	if s == nil {
		return nil, fmt.Errorf("notification sms is not enabled")
	}
	if s.config.ConfigError != "" {
		return nil, fmt.Errorf("notification sms config is invalid: %s", s.config.ConfigError)
	}
	if !s.IsEnabled() {
		return nil, fmt.Errorf("notification sms is not enabled")
	}
	if strings.TrimSpace(req.Provider) == "" {
		req.Provider = s.config.DefaultProvider
	}
	req.Template = strings.TrimSpace(req.Template)
	if req.Template == "" {
		return nil, fmt.Errorf("template is required")
	}
	req.TemplateParams = NormalizeTemplateParams(req.TemplateParams)
	template, ok := s.config.TemplateByKey(req.Template)
	if !ok {
		return nil, fmt.Errorf("notification sms template is not configured: %s", req.Template)
	}
	if !template.supportsProvider(req.Provider) {
		return nil, fmt.Errorf("notification sms template %s is not configured for provider %s", req.Template, req.Provider)
	}
	if err := validateRequest(req, template); err != nil {
		return nil, err
	}

	provider, ok := s.providers[req.Provider]
	if !ok {
		return nil, fmt.Errorf("notification sms provider is not configured: %s", req.Provider)
	}
	return provider.SendNotification(ctx, req, template)
}
