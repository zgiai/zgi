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
	if s == nil || !s.IsEnabled() {
		return nil, fmt.Errorf("notification sms is not enabled")
	}
	if strings.TrimSpace(req.Provider) == "" {
		req.Provider = s.config.DefaultProvider
	}
	if strings.TrimSpace(req.Template) == "" {
		req.Template = TemplatePendingActionNotification
	}
	if err := validateRequest(req); err != nil {
		return nil, err
	}

	provider, ok := s.providers[req.Provider]
	if !ok {
		return nil, fmt.Errorf("notification sms provider is not configured: %s", req.Provider)
	}
	return provider.SendNotification(ctx, req)
}
