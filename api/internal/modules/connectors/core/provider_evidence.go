package core

import "time"

// ProviderEvidence is a bounded, non-secret projection of a verified app.
// Requested configuration is not evidence of a granted permission.
type ProviderEvidence struct {
	Social                   *SocialEvidence    `json:"social,omitempty"`
	Mail                     *MailEvidence      `json:"mail,omitempty"`
	Messaging                *MessagingEvidence `json:"messaging,omitempty"`
	Region                   string             `json:"region"`
	AppID                    string             `json:"app_id"`
	AppName                  string             `json:"app_name"`
	TemplateID               string             `json:"template_id"`
	GrantedScopes            []string           `json:"granted_scopes"`
	ScopeStatus              string             `json:"scope_status"`
	EventConfiguration       string             `json:"event_configuration"`
	EventsEnabled            bool               `json:"events_enabled"`
	UserAuthorizationEnabled bool               `json:"user_authorization_enabled"`
}

func (value *ProviderEvidence) Clone() *ProviderEvidence {
	if value == nil {
		return nil
	}
	result := *value
	if value.Social != nil {
		copy := *value.Social
		copy.AccountIDs = append([]string{}, value.Social.AccountIDs...)
		result.Social = &copy
	}
	if value.Mail != nil {
		copy := *value.Mail
		copy.Operations = append([]OperationEvidence{}, value.Mail.Operations...)
		result.Mail = &copy
	}
	if value.Messaging != nil {
		copy := *value.Messaging
		copy.Operations = append([]OperationEvidence{}, value.Messaging.Operations...)
		result.Messaging = &copy
	}
	result.GrantedScopes = append([]string{}, value.GrantedScopes...)
	return &result
}

// SocialEvidence records provider-observed managed accounts at credential
// verification. It is setup evidence; every write still rechecks ownership.
type SocialEvidence struct {
	Version    int       `json:"version"`
	Provider   string    `json:"provider"`
	Identity   string    `json:"identity"`
	AccountIDs []string  `json:"account_ids"`
	CheckedAt  time.Time `json:"checked_at"`
}

// MailEvidence records protocol observations, never a Google stable subject or
// remotely narrowed scope. Usage is informational; execution uses constraints.
type MailEvidence struct {
	Version            int                 `json:"version"`
	Provider           string              `json:"provider"`
	IdentityKind       string              `json:"identity_kind"`
	AuthenticatedLogin string              `json:"authenticated_login"`
	CredentialVerified bool                `json:"credential_verified"`
	IMAPStatus         string              `json:"imap_status"`
	SMTPStatus         string              `json:"smtp_status"`
	Usage              string              `json:"usage"`
	ScopeGranularity   string              `json:"scope_granularity"`
	Operations         []OperationEvidence `json:"operations"`
	CheckedAt          time.Time           `json:"checked_at"`
}

// MailVerification contains only the per-protocol status of the current
// attempt. It can accompany a rejected setup without retaining credentials.
type MailVerification struct {
	Usage      string `json:"usage"`
	IMAPStatus string `json:"imap_status"`
	SMTPStatus string `json:"smtp_status"`
}

func (m *MailVerification) SafeCopy() *MailVerification {
	if m == nil {
		return nil
	}
	if m.Usage != "read" && m.Usage != "send" && m.Usage != "read_send" {
		return nil
	}
	for _, status := range []string{m.IMAPStatus, m.SMTPStatus} {
		if status != "not_requested" && status != "verified" && status != "failed" {
			return nil
		}
	}
	if m.Usage == "read" && m.SMTPStatus != "not_requested" || m.Usage == "send" && m.IMAPStatus != "not_requested" {
		return nil
	}
	result := *m
	return &result
}

// MessagingEvidence describes observations, not requested grants. Resource
// authorization is always re-evaluated by the provider for the actual target.
type MessagingEvidence struct {
	Version            int                 `json:"version"`
	Provider           string              `json:"provider"`
	Subject            string              `json:"subject"`
	Identity           string              `json:"identity"`
	CredentialVerified bool                `json:"credential_verified"`
	Operations         []OperationEvidence `json:"operations"`
}
type OperationEvidence struct {
	ID           string `json:"id"`
	Availability string `json:"availability"`
	Permission   string `json:"permission"`
	Resource     string `json:"resource"`
}
