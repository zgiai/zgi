package notification

import (
	"context"
	"fmt"
	"strings"

	automationmodel "github.com/zgiai/ginext/internal/modules/automation/model"
	notificationsms "github.com/zgiai/ginext/internal/modules/notification/sms"
)

type notificationSMSService interface {
	IsEnabled() bool
	Send(ctx context.Context, req notificationsms.Request) (*notificationsms.Result, error)
}

type NotificationSMSSink struct {
	service notificationSMSService
}

func NewNotificationSMSSink(service notificationSMSService) *NotificationSMSSink {
	return &NotificationSMSSink{service: service}
}

func (s *NotificationSMSSink) ChannelType() automationmodel.NotificationChannelType {
	return automationmodel.NotificationChannelTypeSMS
}

func (s *NotificationSMSSink) Send(ctx context.Context, req *Request) (*Result, error) {
	if req == nil {
		return nil, fmt.Errorf("sms notification request is nil")
	}
	if req.ChannelType != automationmodel.NotificationChannelTypeSMS {
		return nil, fmt.Errorf("unsupported notification channel for sms sink: %s", req.ChannelType)
	}
	if s.service == nil || !s.service.IsEnabled() {
		return nil, fmt.Errorf("notification sms is not enabled")
	}
	if len(req.To) == 0 {
		return nil, fmt.Errorf("sms notification recipients are empty")
	}
	if req.Template != notificationsms.TemplatePendingActionNotification {
		return nil, fmt.Errorf("unsupported sms notification template: %s", req.Template)
	}

	result, err := s.service.Send(ctx, notificationsms.Request{
		Provider:          req.Provider,
		Phone:             strings.Join(req.To, ","),
		Template:          req.Template,
		NotificationTitle: req.TemplateParams["notification_title"],
		LinkCode:          req.TemplateParams["link_code"],
		Source:            "automation",
	})
	if err != nil {
		return nil, err
	}

	externalID := result.MessageID
	return &Result{
		ChannelType: automationmodel.NotificationChannelTypeSMS,
		Accepted:    result.Accepted,
		ExternalID:  &externalID,
	}, nil
}
