package approval

import (
	"context"
	"time"
)

const (
	NodeTypeApproval = "approval"

	ActionExpired = "expired"

	FormStatusWaiting   = "waiting"
	FormStatusSubmitted = "submitted"
	FormStatusTimeout   = "timeout"
	FormStatusExpired   = "expired"

	RecipientTypeWebApp        = "webapp"
	RecipientTypeEmailMember   = "email_member"
	RecipientTypeEmailExternal = "email_external"
	RecipientTypeSMSMember     = "sms_member"
	RecipientTypeSMSExternal   = "sms_external"
	RecipientTypeConsole       = "console"

	DeliveryTypeWebApp = "webapp"
	DeliveryTypeEmail  = "email"
	DeliveryTypeSMS    = "sms"
)

type NodeConfig struct {
	Content       string        `json:"content"`
	Fields        []FieldConfig `json:"fields"`
	Actions       []Action      `json:"actions"`
	SubmitMethods SubmitMethods `json:"submit_methods"`
	Timeout       TimeoutConfig `json:"timeout"`
	Title         string        `json:"title,omitempty"`
}

type FieldConfig struct {
	Key      string         `json:"key"`
	Label    string         `json:"label"`
	Type     string         `json:"type"`
	Required bool           `json:"required"`
	Default  *DefaultConfig `json:"default,omitempty"`
}

type DefaultConfig struct {
	Type     string   `json:"type"`
	Value    string   `json:"value,omitempty"`
	Selector []string `json:"selector,omitempty"`
}

type Action struct {
	ID    string `json:"id"`
	Label string `json:"label"`
	Style string `json:"style,omitempty"`
}

type SubmitMethods struct {
	WebApp WebAppSubmitMethod `json:"webapp"`
	Email  EmailSubmitMethod  `json:"email"`
	SMS    SMSSubmitMethod    `json:"sms"`
}

type WebAppSubmitMethod struct {
	Enabled *bool `json:"enabled,omitempty"`
}

type EmailSubmitMethod struct {
	Enabled    bool             `json:"enabled"`
	Subject    string           `json:"subject"`
	Body       string           `json:"body"`
	Recipients []EmailRecipient `json:"recipients"`
}

type EmailRecipient struct {
	Type      string `json:"type"`
	AccountID string `json:"account_id,omitempty"`
	Email     string `json:"email,omitempty"`
}

type SMSSubmitMethod struct {
	Enabled           bool              `json:"enabled"`
	Provider          string            `json:"provider,omitempty"`
	Template          string            `json:"template,omitempty"`
	NotificationTitle string            `json:"notification_title"`
	TemplateParams    map[string]string `json:"template_params,omitempty"`
	Recipients        []SMSRecipient    `json:"recipients"`
}

type SMSRecipient struct {
	Type      string `json:"type"`
	AccountID string `json:"account_id,omitempty"`
	Phone     string `json:"phone,omitempty"`
}

type TimeoutConfig struct {
	Duration int    `json:"duration"`
	Unit     string `json:"unit"`
}

type FormDefinition struct {
	Content       string                 `json:"content"`
	Fields        []FieldConfig          `json:"fields"`
	Actions       []Action               `json:"actions"`
	SubmitMethods SubmitMethods          `json:"submit_methods"`
	Rendered      string                 `json:"rendered_content"`
	DefaultValues map[string]interface{} `json:"default_values"`
	Title         string                 `json:"title,omitempty"`
	ExpirationAt  time.Time              `json:"expiration_at"`
}

type FormPayload struct {
	ID                    string                 `json:"id"`
	Token                 string                 `json:"token,omitempty"`
	NodeID                string                 `json:"node_id"`
	NodeTitle             string                 `json:"node_title,omitempty"`
	Content               string                 `json:"content"`
	Fields                []FieldConfig          `json:"fields"`
	Actions               []Action               `json:"actions"`
	SubmitMethods         SubmitMethods          `json:"submit_methods"`
	ResolvedDefaultValues map[string]interface{} `json:"resolved_default_values"`
	ExpirationAt          int64                  `json:"expiration_at"`
}

type SubmitRequest struct {
	Inputs map[string]interface{} `json:"inputs"`
	Action string                 `json:"action"`
}

type CreateRuntimeFormParams struct {
	TenantID      string
	AppID         string
	WorkflowRunID string
	NodeID        string
	NodeTitle     string
	Config        NodeConfig
	Rendered      string
	DefaultValues map[string]interface{}
}

type RuntimeForm struct {
	Form    *Form
	Payload FormPayload
}

type ResumeCallback func(ctx context.Context, form *Form) error

type RunEventPayload struct {
	Sequence  int                    `json:"sequence"`
	Event     string                 `json:"event"`
	Data      map[string]interface{} `json:"data"`
	CreatedAt int64                  `json:"created_at"`
}

type RunEventsPayload struct {
	WorkflowRunID string            `json:"workflow_run_id"`
	Events        []RunEventPayload `json:"events"`
}
