package integrations

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"
)

type countingCredentialCipher struct {
	decryptCalls int
}

func (*countingCredentialCipher) EncryptCredentials(map[string]string, CredentialAAD) (string, error) {
	return "", errors.New("not implemented")
}

func (cipher *countingCredentialCipher) DecryptCredentials(string, CredentialAAD) (map[string]string, error) {
	cipher.decryptCalls++
	return map[string]string{"token": "should-not-be-read"}, nil
}

func TestConnectionResolverRejectsExpiredExplicitConnection(t *testing.T) {
	repository := newMemoryConnectionRepository()
	organizationID := uuid.New()
	expiredAt := time.Now().UTC().Add(-time.Minute)
	connection := &IntegrationConnection{
		ID: uuid.New(), OrganizationID: organizationID, IntegrationID: IntegrationWebSearch, DriverID: DriverExa,
		CredentialSource: ConnectionCredentialSourceOrganization, AuthType: ConnectionAuthTypeAPIKey,
		Status: ConnectionStatusActive, CredentialVersion: 1, ExpiresAt: &expiredAt,
	}
	if err := repository.Create(t.Context(), connection); err != nil {
		t.Fatal(err)
	}
	resolver := NewConnectionResolver(repository, &countingCredentialCipher{})
	resolved, err := resolver.Resolve(t.Context(), ConnectionResolveRequest{
		OrganizationID: organizationID.String(), IntegrationID: IntegrationWebSearch,
		DriverID: DriverExa, ConnectionID: connection.ID.String(),
	})
	if resolved != nil || ErrorCode(err) != ErrorCodeConnectionInvalid {
		t.Fatalf("Resolve() = %#v, %v; want expired connection failure", resolved, err)
	}
}

func TestConnectionResolverResolvesExplicitOrganizationConnectionAndDestroysSecrets(t *testing.T) {
	repository := newMemoryConnectionRepository()
	cipher, err := NewCredentialCipher("12345678901234567890123456789012")
	if err != nil {
		t.Fatal(err)
	}
	organizationID := uuid.New()
	connectionID := uuid.New()
	envelope, err := cipher.EncryptCredentials(map[string]string{"api_key": "organization-key"}, CredentialAAD{
		OrganizationID: organizationID, ConnectionID: connectionID,
		IntegrationID: IntegrationWebSearch, CredentialVersion: 1,
	})
	if err != nil {
		t.Fatal(err)
	}
	connection := &IntegrationConnection{
		ID: connectionID, OrganizationID: organizationID, IntegrationID: IntegrationWebSearch, DriverID: DriverExa,
		CredentialSource: ConnectionCredentialSourceOrganization, AuthType: ConnectionAuthTypeAPIKey,
		EncryptedCredentials: &envelope, Config: map[string]any{"region": "global"}, GrantedScopes: []string{"search"},
		Status: ConnectionStatusActive, IsDefault: true, CredentialVersion: 1,
	}
	if err := repository.Create(t.Context(), connection); err != nil {
		t.Fatal(err)
	}
	repository.defaultID = connectionID
	resolver := NewConnectionResolver(repository, cipher)
	resolved, err := resolver.Resolve(t.Context(), ConnectionResolveRequest{
		OrganizationID: organizationID.String(), IntegrationID: IntegrationWebSearch,
		DriverID: DriverExa, ConnectionID: connectionID.String(),
	})
	if err != nil {
		t.Fatalf("Resolve() error = %v", err)
	}
	if resolved.ID != connectionID.String() || resolved.Credentials["api_key"] != "organization-key" {
		t.Fatalf("resolved connection = %#v", resolved)
	}
	resolved.Destroy()
	if resolved.Credentials != nil || resolved.Config != nil || resolved.GrantedScopes != nil {
		t.Fatalf("Destroy() left request-scoped material: %#v", resolved)
	}
}

func TestConnectionResolverAgentGuardRejectsPersonalCredentialBeforeDecrypt(t *testing.T) {
	repository := newMemoryConnectionRepository()
	organizationID, ownerID, connectionID := uuid.New(), uuid.New(), uuid.New()
	envelope := "encrypted-personal-token"
	if err := repository.Create(t.Context(), &IntegrationConnection{
		ID: connectionID, OrganizationID: organizationID, IntegrationID: "github", DriverID: "github-rest", Name: "Personal",
		CredentialSource: ConnectionCredentialSourceAccount, OwnerAccountID: &ownerID, AuthType: ConnectionAuthTypeAPIKey,
		EncryptedCredentials: &envelope, Status: ConnectionStatusActive, CredentialVersion: 1,
	}); err != nil {
		t.Fatal(err)
	}
	cipher := &countingCredentialCipher{}
	resolver := NewConnectionResolver(repository, cipher)
	resolved, err := resolver.Resolve(t.Context(), ConnectionResolveRequest{
		OrganizationID: organizationID.String(), IntegrationID: "github", DriverID: "github-rest",
		ConnectionID: connectionID.String(), DisallowAccountCredential: true,
	})
	if resolved != nil || ErrorCode(err) != ErrorCodeAccessDenied {
		t.Fatalf("Resolve() result=%#v error=%v", resolved, err)
	}
	if cipher.decryptCalls != 0 {
		t.Fatalf("personal credential decrypt calls = %d, want 0", cipher.decryptCalls)
	}
}

func TestConnectionResolverEmptyIDNeverSelectsAnyConnection(t *testing.T) {
	repository := newMemoryConnectionRepository()
	organizationID := uuid.New()
	organizationConnection := &IntegrationConnection{
		ID: uuid.New(), OrganizationID: organizationID, IntegrationID: IntegrationWebSearch, DriverID: DriverExa,
		Name: "Organization default", CredentialSource: ConnectionCredentialSourceOrganization,
		AuthType: ConnectionAuthTypeAPIKey, EncryptedCredentials: stringPointer("must-not-be-decrypted"),
		Status: ConnectionStatusActive, IsDefault: true, CredentialVersion: 1,
	}
	if err := repository.Create(t.Context(), organizationConnection); err != nil {
		t.Fatal(err)
	}
	repository.defaultID = organizationConnection.ID
	cipher := &countingCredentialCipher{}
	resolved, err := NewConnectionResolver(repository, cipher).Resolve(t.Context(), ConnectionResolveRequest{
		OrganizationID: organizationID.String(), IntegrationID: IntegrationWebSearch, DriverID: DriverExa,
	})
	if resolved != nil || ErrorCode(err) != ErrorCodeConnectionNotFound {
		t.Fatalf("Resolve() = %#v, %v; want explicit connection requirement", resolved, err)
	}
	if cipher.decryptCalls != 0 {
		t.Fatalf("implicit connection decrypt calls = %d, want 0", cipher.decryptCalls)
	}
}

func TestConnectionResolverExplicitFailureDoesNotSelectAnotherConnection(t *testing.T) {
	repository := newMemoryConnectionRepository()
	organizationID := uuid.New()
	resolver := NewConnectionResolver(repository, &countingCredentialCipher{})
	for _, connectionID := range []string{"not-a-uuid", uuid.NewString()} {
		resolved, err := resolver.Resolve(context.Background(), ConnectionResolveRequest{
			OrganizationID: organizationID.String(), IntegrationID: IntegrationWebSearch,
			DriverID: DriverExa, ConnectionID: connectionID,
		})
		if resolved != nil || ErrorCode(err) != ErrorCodeConnectionNotFound {
			t.Fatalf("Resolve(%q) = %#v, %v; want explicit failure", connectionID, resolved, err)
		}
	}
}

func TestConnectionResolverRejectsLegacyPlatformConnectionBeforeCredentialAccess(t *testing.T) {
	repository := newMemoryConnectionRepository()
	organizationID := uuid.New()
	connection := &IntegrationConnection{
		ID: uuid.New(), OrganizationID: organizationID, IntegrationID: IntegrationWebSearch, DriverID: DriverExa,
		CredentialSource: ConnectionCredentialSourcePlatform, AuthType: ConnectionAuthTypePlatform,
		Status: ConnectionStatusActive, CredentialVersion: 1,
	}
	if err := repository.Create(t.Context(), connection); err != nil {
		t.Fatal(err)
	}
	cipher := &countingCredentialCipher{}
	resolved, err := NewConnectionResolver(repository, cipher).Resolve(t.Context(), ConnectionResolveRequest{
		OrganizationID: organizationID.String(), IntegrationID: IntegrationWebSearch,
		DriverID: DriverExa, ConnectionID: connection.ID.String(),
	})
	if resolved != nil || ErrorCode(err) != ErrorCodeConnectionInvalid {
		t.Fatalf("Resolve(legacy platform) = %#v, %v", resolved, err)
	}
	if cipher.decryptCalls != 0 {
		t.Fatalf("legacy platform decrypt calls = %d, want 0", cipher.decryptCalls)
	}
}
