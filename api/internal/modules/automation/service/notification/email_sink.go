package notification

import (
	"context"
	"fmt"

	automationmodel "github.com/zgiai/ginext/internal/modules/automation/model"
	"github.com/zgiai/ginext/pkg/email"
)

// EmailSink delivers notifications through the email package.
type EmailSink struct{}

// NewEmailSink creates an email sink.
func NewEmailSink() *EmailSink {
	return &EmailSink{}
}

// ChannelType returns the channel supported by this sink.
func (s *EmailSink) ChannelType() automationmodel.NotificationChannelType {
	return automationmodel.NotificationChannelTypeEmail
}

// Send sends an email notification.
func (s *EmailSink) Send(ctx context.Context, req *Request) (*Result, error) {
	_ = ctx

	if req == nil {
		return nil, fmt.Errorf("email notification request is nil")
	}
	if req.ChannelType != automationmodel.NotificationChannelTypeEmail {
		return nil, fmt.Errorf("unsupported notification channel for email sink: %s", req.ChannelType)
	}
	if len(req.To) == 0 {
		return nil, fmt.Errorf("email notification recipients are empty")
	}
	if req.Subject == "" {
		return nil, fmt.Errorf("email notification subject is empty")
	}
	if req.Body == "" {
		return nil, fmt.Errorf("email notification body content is empty")
	}

	bodyType := req.BodyType
	if bodyType == "" {
		bodyType = "text/html"
	}

	if err := email.SendEmailWithBodyType(req.To, req.Subject, req.Body, bodyType); err != nil {
		return nil, fmt.Errorf("send email notification: %w", err)
	}

	return &Result{
		ChannelType: automationmodel.NotificationChannelTypeEmail,
		Accepted:    true,
	}, nil
}
