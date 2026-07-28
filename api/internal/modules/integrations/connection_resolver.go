package integrations

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
)

const (
	ErrorCodeConnectionNotFound = "integration_connection_not_found"
	ErrorCodeConnectionInvalid  = "integration_connection_invalid"
	ErrorCodeConnectionConflict = "integration_connection_conflict"
	ErrorCodeConnectionInUse    = "integration_connection_in_use"
)

type ConnectionResolveRequest struct {
	OrganizationID            string
	IntegrationID             string
	DriverID                  string
	ConnectionID              string
	DisallowAccountCredential bool
}

type ConnectionResolver interface {
	Resolve(ctx context.Context, request ConnectionResolveRequest) (*ResolvedConnection, error)
}

// ResolvedConnection contains request-scoped secret material. It must never be
// serialized, cached, or retained by an Adapter. Call Destroy as soon as the
// provider invocation finishes.
type ResolvedConnection struct {
	ID                string                     `json:"-"`
	OrganizationID    string                     `json:"-"`
	IntegrationID     string                     `json:"-"`
	DriverID          string                     `json:"-"`
	CredentialSource  ConnectionCredentialSource `json:"-"`
	AuthType          ConnectionAuthType         `json:"-"`
	AuthMethodID      string                     `json:"-"`
	AccountID         string                     `json:"-"`
	DisplayName       string                     `json:"-"`
	Credentials       map[string]string          `json:"-"`
	Config            map[string]any             `json:"-"`
	GrantedScopes     []string                   `json:"-"`
	CredentialVersion int                        `json:"-"`
}

func (connection *ResolvedConnection) Destroy() {
	if connection == nil {
		return
	}
	destroyCredentialMap(connection.Credentials)
	connection.Credentials = nil
	for key := range connection.Config {
		delete(connection.Config, key)
	}
	connection.Config = nil
	connection.AccountID = ""
	connection.DisplayName = ""
	for index := range connection.GrantedScopes {
		connection.GrantedScopes[index] = ""
	}
	connection.GrantedScopes = nil
}

type DefaultConnectionResolver struct {
	repository ConnectionRepository
	cipher     CredentialCipher
}

func NewConnectionResolver(repository ConnectionRepository, cipher CredentialCipher) *DefaultConnectionResolver {
	return &DefaultConnectionResolver{repository: repository, cipher: cipher}
}

func (resolver *DefaultConnectionResolver) Resolve(ctx context.Context, request ConnectionResolveRequest) (*ResolvedConnection, error) {
	if resolver == nil || resolver.repository == nil {
		return nil, NewError(ErrorCodeConnectionInvalid, "integration connection resolver is unavailable", nil)
	}
	organizationID, err := uuid.Parse(strings.TrimSpace(request.OrganizationID))
	if err != nil || organizationID == uuid.Nil {
		return nil, invalidInput("organization id is required", err)
	}
	integrationID := strings.ToLower(strings.TrimSpace(request.IntegrationID))
	driverID := strings.ToLower(strings.TrimSpace(request.DriverID))
	if integrationID == "" || driverID == "" {
		return nil, invalidInput("integration id and driver id are required", nil)
	}
	if explicitID := strings.TrimSpace(request.ConnectionID); explicitID != "" {
		connectionID, parseErr := uuid.Parse(explicitID)
		if parseErr != nil || connectionID == uuid.Nil {
			return nil, NewError(ErrorCodeConnectionNotFound, "integration connection was not found", parseErr)
		}
		connection, lookupErr := resolver.repository.GetByID(ctx, organizationID, connectionID)
		if lookupErr != nil {
			if errors.Is(lookupErr, ErrConnectionNotFound) {
				return nil, NewError(ErrorCodeConnectionNotFound, "integration connection was not found", lookupErr)
			}
			return nil, NewError(ErrorCodeConnectionInvalid, "integration connection could not be loaded", lookupErr)
		}
		if request.DisallowAccountCredential && connection.CredentialSource == ConnectionCredentialSourceAccount {
			return nil, NewError(ErrorCodeAccessDenied, "personal integration credentials are not available to Agents", nil)
		}
		return resolver.resolveRecord(ctx, connection, integrationID, driverID, true)
	}

	// An omitted ID is never permission to use deployment-level credentials or
	// choose a connection implicitly. AIChat persists an explicit user
	// selection and Agents persist an explicit binding.
	return nil, NewError(ErrorCodeConnectionNotFound, "an explicit integration connection is required", nil)
}

func (resolver *DefaultConnectionResolver) ResolveRecordForTest(ctx context.Context, connection *IntegrationConnection) (*ResolvedConnection, error) {
	if connection == nil {
		return nil, NewError(ErrorCodeConnectionNotFound, "integration connection was not found", nil)
	}
	return resolver.resolveRecord(ctx, connection, connection.IntegrationID, connection.DriverID, false)
}

func (resolver *DefaultConnectionResolver) resolveRecord(ctx context.Context, connection *IntegrationConnection, integrationID, driverID string, requireActive bool) (*ResolvedConnection, error) {
	if connection == nil || connection.ID == uuid.Nil || connection.OrganizationID == uuid.Nil {
		return nil, NewError(ErrorCodeConnectionNotFound, "integration connection was not found", nil)
	}
	if !strings.EqualFold(connection.IntegrationID, integrationID) || !strings.EqualFold(connection.DriverID, driverID) {
		return nil, NewError(ErrorCodeConnectionNotFound, "integration connection was not found", nil)
	}
	if requireActive && connection.Status != ConnectionStatusActive {
		return nil, NewError(ErrorCodeConnectionInvalid, "integration connection is not active", nil)
	}
	if requireActive && connection.ExpiresAt != nil && !connection.ExpiresAt.After(time.Now().UTC()) {
		return nil, NewError(ErrorCodeConnectionInvalid, "integration connection has expired", nil)
	}
	if requireActive {
		switch connection.AuthStatus {
		case ConnectionAuthReconnectRequired:
			return nil, NewError(ErrorCodeReconnectRequired, "integration connection must be reauthorized", nil)
		case ConnectionAuthExpired:
			return nil, NewError(ErrorCodeConnectionExpired, "integration connection authentication has expired", nil)
		}
		if connection.TokenExpiresAt != nil && !connection.TokenExpiresAt.After(time.Now().UTC()) {
			return nil, NewError(ErrorCodeConnectionExpired, "integration connection authentication has expired", nil)
		}
		if connection.AuthType == ConnectionAuthTypeOAuth2 &&
			connection.RefreshTokenExpiresAt != nil &&
			!connection.RefreshTokenExpiresAt.After(time.Now().UTC()) {
			return nil, NewError(ErrorCodeReconnectRequired, "integration connection must be reauthorized", nil)
		}
	}
	if connection.Status == ConnectionStatusDisabled {
		return nil, NewError(ErrorCodeConnectionInvalid, "integration connection is disabled", nil)
	}

	if !supportedConnectionCredentialSource(connection.CredentialSource) {
		return nil, NewError(ErrorCodeConnectionInvalid, "integration connection credential source is invalid", nil)
	}
	if resolver.cipher == nil || connection.EncryptedCredentials == nil || strings.TrimSpace(*connection.EncryptedCredentials) == "" {
		return nil, NewError(ErrorCodeConnectionInvalid, "integration connection credentials are unavailable", nil)
	}
	credentials, err := resolver.cipher.DecryptCredentials(*connection.EncryptedCredentials, CredentialAAD{
		OrganizationID:    connection.OrganizationID,
		ConnectionID:      connection.ID,
		IntegrationID:     connection.IntegrationID,
		CredentialVersion: connection.CredentialVersion,
	})
	if err != nil {
		return nil, NewError(ErrorCodeConnectionInvalid, "integration connection credentials could not be decrypted", err)
	}

	return &ResolvedConnection{
		ID:                connection.ID.String(),
		OrganizationID:    connection.OrganizationID.String(),
		IntegrationID:     connection.IntegrationID,
		DriverID:          connection.DriverID,
		CredentialSource:  connection.CredentialSource,
		AuthType:          connection.AuthType,
		AuthMethodID:      connection.AuthMethodID,
		AccountID:         connectionStringValue(connection.AccountID),
		DisplayName:       connectionStringValue(connection.DisplayName),
		Credentials:       credentials,
		Config:            cloneAnyMap(connection.Config),
		GrantedScopes:     append([]string(nil), connection.GrantedScopes...),
		CredentialVersion: connection.CredentialVersion,
	}, nil
}

func connectionStringValue(value *string) string {
	if value == nil {
		return ""
	}
	return strings.TrimSpace(*value)
}

func resolvedConnectionDriverMismatch(connection *ResolvedConnection, driverID string) error {
	if connection == nil || !strings.EqualFold(connection.DriverID, driverID) {
		return fmt.Errorf("resolved integration connection driver does not match")
	}
	return nil
}
