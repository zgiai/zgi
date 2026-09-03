package core

// ProviderEvidence is a bounded, non-secret projection of a verified app.
// Requested configuration is not evidence of a granted permission.
type ProviderEvidence struct {
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
	if value.Messaging != nil {
		copy := *value.Messaging
		copy.Operations = append([]OperationEvidence{}, value.Messaging.Operations...)
		result.Messaging = &copy
	}
	result.GrantedScopes = append([]string{}, value.GrantedScopes...)
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
