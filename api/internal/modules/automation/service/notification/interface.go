package notification

import (
	"context"

	automationmodel "github.com/zgiai/ginext/internal/modules/automation/model"
)

// Request represents a normalized notification request.
type Request struct {
	ChannelType    automationmodel.NotificationChannelType `json:"channel_type"`
	To             []string                                `json:"to"`
	Subject        string                                  `json:"subject"`
	Body           string                                  `json:"body"`
	BodyType       string                                  `json:"body_type"`
	Template       string                                  `json:"template,omitempty"`
	TemplateParams map[string]string                       `json:"template_params,omitempty"`
	Provider       string                                  `json:"provider,omitempty"`
}

// Result represents the normalized result returned by a sink.
type Result struct {
	ChannelType automationmodel.NotificationChannelType `json:"channel_type"`
	Accepted    bool                                    `json:"accepted"`
	ExternalID  *string                                 `json:"external_id,omitempty"`
}

// Sink sends a normalized notification request to one delivery channel.
type Sink interface {
	ChannelType() automationmodel.NotificationChannelType
	Send(ctx context.Context, req *Request) (*Result, error)
}
