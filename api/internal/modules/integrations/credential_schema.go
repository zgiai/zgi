package integrations

import "context"

// ConnectionCredentialValidator validates the exact provider-owned secret
// fields for an authentication method before they cross the encryption
// boundary. Implementations must not retain or log credentials.
type ConnectionCredentialValidator interface {
	ValidateProviderCredentials(context.Context, CredentialValidationRequest) error
}

type ConnectionCredentialValidatorFunc func(context.Context, CredentialValidationRequest) error

func (fn ConnectionCredentialValidatorFunc) ValidateProviderCredentials(ctx context.Context, request CredentialValidationRequest) error {
	return fn(ctx, request)
}
