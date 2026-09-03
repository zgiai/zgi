package integrations

import (
	"context"
	"errors"
	"strings"
	"time"

	"github.com/google/uuid"
)

// Version 2 adds the explicit action-policy review to the connection setup
// contract. Connections completed by the original credential/grant-only flow
// must pass through setup again so users do not mistake a healthy credential
// for a fully usable external app.
const ConnectionSetupVersion = 2

type CompleteConnectionSetupInput struct {
	OrganizationID  uuid.UUID
	ConnectionID    uuid.UUID
	ActorID         uuid.UUID
	Personal        bool
	UsableActionIDs []string
}

// CompleteConnectionSetup persists only the connection-level setup milestone.
// Usage targets such as AIChat, Agents, and Workflows keep their own binding
// models and are deliberately configured independently from this milestone.
func CompleteConnectionSetup(
	ctx context.Context,
	connections ConnectionRepository,
	grants ConnectionGrantRepository,
	input CompleteConnectionSetupInput,
) error {
	if connections == nil || grants == nil {
		return NewError(ErrorCodeDisabled, "integration connection setup is unavailable", nil)
	}
	if input.OrganizationID == uuid.Nil || input.ConnectionID == uuid.Nil || input.ActorID == uuid.Nil {
		return invalidInput("connection setup context is incomplete", nil)
	}
	connection, err := connections.GetByID(ctx, input.OrganizationID, input.ConnectionID)
	if err != nil {
		return mapConnectionLookupError(err)
	}
	if err := rejectLegacyPlatformConnection(connection); err != nil {
		return err
	}
	if input.Personal {
		if err := authorizePersonalConnectionOwner(connection, &input.ActorID); err != nil {
			return err
		}
	} else if connection.CredentialSource != ConnectionCredentialSourceOrganization {
		return NewError(ErrorCodeConnectionNotFound, "integration connection was not found", nil)
	}
	if connection.Status != ConnectionStatusActive || connection.AuthStatus != ConnectionAuthValid || connection.HealthStatus == ConnectionHealthUnhealthy {
		return NewError(ErrorCodeConnectionInvalid, "test the connection successfully before completing setup", nil)
	}
	usableActions := make(map[string]struct{}, len(input.UsableActionIDs))
	for _, actionID := range input.UsableActionIDs {
		normalized := strings.ToLower(strings.TrimSpace(actionID))
		if normalized != "" && integrationIdentifierPattern.MatchString(normalized) {
			usableActions[normalized] = struct{}{}
		}
	}
	if len(usableActions) == 0 {
		return NewError(ErrorCodeInsufficientScope, "connection has no usable provider actions", nil)
	}
	if !input.Personal {
		items, listErr := grants.List(ctx, input.OrganizationID, input.ConnectionID)
		if listErr != nil {
			return NewError(ErrorCodeAccessDenied, "connection usage rules could not be verified", listErr)
		}
		hasUsableRule := false
		for _, grant := range items {
			for _, actionID := range grant.AllowedActionIDs {
				if _, ok := usableActions[strings.ToLower(strings.TrimSpace(actionID))]; ok {
					hasUsableRule = true
					break
				}
			}
			if hasUsableRule {
				break
			}
		}
		if !hasUsableRule {
			return NewError(ErrorCodeAccessDenied, "configure at least one usage rule for a usable action", nil)
		}
	}
	if connection.SetupCompletedAt != nil && connection.SetupVersion >= ConnectionSetupVersion {
		return nil
	}
	now := time.Now().UTC()
	connection.SetupVersion = ConnectionSetupVersion
	connection.SetupCompletedAt = &now
	connection.SetupCompletedBy = cloneUUIDPointer(&input.ActorID)
	connection.UpdatedBy = cloneUUIDPointer(&input.ActorID)
	if err := connections.Update(ctx, connection); err != nil {
		if errors.Is(err, ErrConnectionChanged) {
			return NewError(ErrorCodeConnectionConflict, "integration connection changed; reload it and retry", err)
		}
		return mapConnectionLookupError(err)
	}
	return nil
}
