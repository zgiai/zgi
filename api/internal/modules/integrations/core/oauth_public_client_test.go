package integrations

import (
	"context"
	"testing"

	"github.com/google/uuid"
)

func TestOAuthClientConfigAcceptsPublicClientWithoutSecret(t *testing.T) {
	adapter := &fakeOAuthAdapter{}
	registration := oauthTestRegistration(adapter)
	fields := registration.Definition.AuthMethods[0].OAuth.ClientFields
	for index := range fields {
		if fields[index].Key == "client_secret" {
			fields[index].Required = false
		}
	}
	registration.Definition.AuthMethods[0].OAuth.ClientFields = fields

	registry := NewRegistry()
	if err := registry.Register(registration); err != nil {
		t.Fatal(err)
	}
	cipher, err := NewCredentialCipher("12345678901234567890123456789012")
	if err != nil {
		t.Fatal(err)
	}
	repository := &memoryOAuthClientConfigRepository{}
	service := NewOAuthClientConfigService(repository, cipher, registry, nil).
		WithFlowImpactRepository(newMemoryOAuthFlowRepository())
	organizationID := uuid.New()

	view, err := service.Put(context.Background(), PutOAuthClientConfigRequest{
		OrganizationID: organizationID,
		IntegrationID:  "fake",
		DriverID:       "fake-oauth",
		AuthMethodID:   "user_oauth",
		ClientID:       "public-client-id",
	})
	if err != nil {
		t.Fatalf("Put() public client error = %v", err)
	}
	if view.HasSecret {
		t.Fatalf("public client unexpectedly reports a configured secret: %#v", view)
	}

	resolved, err := service.ResolveOAuthClient(context.Background(), OAuthClientResolveRequest{
		OrganizationID: organizationID,
		IntegrationID:  "fake",
		DriverID:       "fake-oauth",
		AuthMethodID:   "user_oauth",
	})
	if err != nil {
		t.Fatalf("ResolveOAuthClient() error = %v", err)
	}
	defer resolved.Destroy()
	if resolved.ClientID != "public-client-id" || resolved.ClientSecret != "" {
		t.Fatalf("resolved public client = %#v", resolved)
	}
}
