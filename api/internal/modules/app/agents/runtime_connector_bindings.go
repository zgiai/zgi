package agents

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"

	"github.com/zgiai/zgi/api/internal/dto"
	connectorbindings "github.com/zgiai/zgi/api/internal/modules/connectors/bindings"
	connectorcore "github.com/zgiai/zgi/api/internal/modules/connectors/core"
	"gorm.io/gorm"
)

func (s *agentsService) projectOwnedAgentConnectorBindings(
	ctx context.Context,
	db *gorm.DB,
	agent *Agent,
	accountID string,
	config *dto.AgentConfigResponse,
) error {
	if config == nil {
		return fmt.Errorf("Agent config is required")
	}
	bindings, err := s.ownedAgentConnectorBindings(ctx, db, agent, accountID)
	if err != nil {
		return err
	}
	config.ConnectorBindings = bindings
	return nil
}

// ownedAgentConnectorBindings makes the canonical, owner-scoped Target
// Binding table authoritative for Agent draft reads. Agent mode JSON remains
// a portable snapshot, but it is never trusted to disclose another owner's
// Personal Connection.
func (s *agentsService) ownedAgentConnectorBindings(
	ctx context.Context,
	db *gorm.DB,
	agent *Agent,
	accountID string,
) ([]dto.AgentConnectorBinding, error) {
	if agent == nil || agent.ID == uuid.Nil {
		return nil, fmt.Errorf("Agent target is required")
	}
	// The dependency is injected by the production constructor. Its absence in
	// isolated unit services is a fail-closed capability boundary, not a signal
	// to trust mode JSON.
	if s == nil || s.connectorTargetBindings == nil || db == nil {
		return []dto.AgentConnectorBinding{}, nil
	}
	organizationID, err := s.organizationUUIDForAgentWorkspace(ctx, agent.TenantID.String())
	if err != nil {
		return nil, err
	}
	ownerID, err := uuid.Parse(strings.TrimSpace(accountID))
	if err != nil || ownerID == uuid.Nil {
		return nil, &agentBindingAPIError{Code: agentBindingsInvalidCode, Message: "Connector owner is invalid"}
	}
	target := connectorcore.TargetRef{Type: connectorcore.TargetTypeAgent, ID: agent.ID.String()}
	return ownedAgentConnectorBindingsWithRepository(
		ctx,
		connectorbindings.NewGormRepository(db),
		organizationID,
		ownerID,
		target,
	)
}

func ownedAgentConnectorBindingsWithRepository(
	ctx context.Context,
	repository connectorbindings.Repository,
	organizationID, ownerID uuid.UUID,
	target connectorcore.TargetRef,
) ([]dto.AgentConnectorBinding, error) {
	if repository == nil || organizationID == uuid.Nil || ownerID == uuid.Nil ||
		target.Type != connectorcore.TargetTypeAgent || target.Validate() != nil {
		return nil, fmt.Errorf("Agent Connector projection context is invalid")
	}
	bindings, err := repository.ListBindingsByTarget(
		ctx,
		organizationID,
		ownerID,
		target.Type,
		target.ID,
		connectorbindings.ListOptions{Scope: target.EffectiveScope(), VersionID: target.VersionID,
			Statuses: []connectorcore.BindingStatus{
				connectorcore.BindingStatusActive,
				connectorcore.BindingStatusNeedsReplacement,
			},
		},
	)
	if err != nil {
		return nil, fmt.Errorf("list authoritative Agent Connector Target Bindings: %w", err)
	}
	projected := make([]dto.AgentConnectorBinding, 0, len(bindings))
	for _, binding := range bindings {
		if binding.OrganizationID != organizationID || binding.OwnerAccountID != ownerID ||
			!binding.Target.SameIdentity(target) ||
			(binding.Status != connectorcore.BindingStatusActive && binding.Status != connectorcore.BindingStatusNeedsReplacement) {
			return nil, fmt.Errorf("authoritative Agent Connector Target Binding escaped its owner scope")
		}
		projected = append(projected, dto.AgentConnectorBinding{
			ConnectorID:  binding.ConnectorID.String(),
			ConnectionID: binding.ConnectionID.String(), BindingID: binding.ID.String(), Revision: binding.Revision,
			AuthorizationMode: binding.Constraint.AuthorizationModeValue(),
		})
	}
	return normalizeAgentConnectorBindings(projected), nil
}

// filterOwnedAgentConnectorSnapshotBindings validates historical snapshot
// identities against live owner-scoped Connection records. A publisher or
// rollback editor never inherits authorization merely because a foreign
// Connection UUID was embedded in an older snapshot.
func (s *agentsService) filterOwnedAgentConnectorSnapshotBindings(
	ctx context.Context,
	db *gorm.DB,
	agent *Agent,
	accountID string,
	input []dto.AgentConnectorBinding,
) ([]dto.AgentConnectorBinding, error) {
	if agent == nil || agent.ID == uuid.Nil {
		return nil, fmt.Errorf("Agent target is required")
	}
	if db == nil {
		return []dto.AgentConnectorBinding{}, nil
	}
	organizationID, err := s.organizationUUIDForAgentWorkspace(ctx, agent.TenantID.String())
	if err != nil {
		return nil, err
	}
	ownerID, err := uuid.Parse(strings.TrimSpace(accountID))
	if err != nil || ownerID == uuid.Nil {
		return nil, &agentBindingAPIError{Code: agentBindingsInvalidCode, Message: "Connector owner is invalid"}
	}
	lookup := *s
	lookup.db = db
	result := make([]dto.AgentConnectorBinding, 0, len(input))
	for _, binding := range normalizeAgentConnectorBindings(input) {
		connectionID, parseErr := uuid.Parse(binding.ConnectionID)
		connectorID, normalizeErr := connectorcore.NormalizeIdentifier(binding.ConnectorID)
		if parseErr != nil || connectionID == uuid.Nil || normalizeErr != nil {
			continue
		}
		state, loadErr := lookup.loadOwnedAgentConnectorConnection(ctx, organizationID, ownerID, connectionID)
		if errors.Is(loadErr, gorm.ErrRecordNotFound) {
			continue
		}
		if loadErr != nil {
			return nil, fmt.Errorf("authorize historical Agent Connector binding: %w", loadErr)
		}
		if state.OrganizationID != organizationID || state.OwnerAccountID != ownerID || state.ConnectorID != connectorID.String() {
			continue
		}
		result = append(result, binding)
	}
	return normalizeAgentConnectorBindings(result), nil
}

// syncAgentConnectorTargetBindings replaces only the Acting Principal's native
// Connector projection for one Agent. Foreign-owner and legacy_read_only rows
// are outside this mutation set. Omitted native rows are disabled rather than
// deleted so their canonical UUID remains stable if the owner attaches again.
func (s *agentsService) syncAgentConnectorTargetBindings(
	ctx context.Context,
	tx *gorm.DB,
	agent *Agent,
	accountID string,
	desired []dto.AgentConnectorBinding,
) ([]connectorcore.TargetBinding, error) {
	if tx == nil || agent == nil || agent.ID == uuid.Nil {
		return nil, fmt.Errorf("Agent Connector binding transaction and target are required")
	}
	organizationID, err := uuid.Parse(strings.TrimSpace(s.organizationIDForAgentWorkspace(ctx, agent.TenantID.String())))
	if err != nil || organizationID == uuid.Nil {
		return nil, fmt.Errorf("Agent organization is invalid")
	}
	ownerID, err := uuid.Parse(strings.TrimSpace(accountID))
	if err != nil || ownerID == uuid.Nil {
		return nil, &agentBindingAPIError{Code: agentBindingsInvalidCode, Message: "Connector owner is invalid"}
	}
	target := connectorcore.TargetRef{Type: connectorcore.TargetTypeAgent, ID: agent.ID.String()}
	repository := connectorbindings.NewGormRepository(tx)
	return syncAgentConnectorTargetBindingsWithRepository(ctx, repository, organizationID, ownerID, target, desired, tx)
}

func syncAgentConnectorTargetBindingsWithRepository(
	ctx context.Context,
	repository connectorbindings.Repository,
	organizationID, ownerID uuid.UUID,
	target connectorcore.TargetRef,
	desired []dto.AgentConnectorBinding,
	databases ...*gorm.DB,
) ([]connectorcore.TargetBinding, error) {
	if repository == nil || organizationID == uuid.Nil || ownerID == uuid.Nil || target.Validate() != nil || target.Type != connectorcore.TargetTypeAgent {
		return nil, fmt.Errorf("Agent Connector Target Binding context is invalid")
	}
	existing, err := repository.ListBindingsByTarget(ctx, organizationID, ownerID, target.Type, target.ID, connectorbindings.ListOptions{Scope: target.EffectiveScope(), VersionID: target.VersionID})
	if err != nil {
		return nil, fmt.Errorf("list Agent Connector Target Bindings: %w", err)
	}
	existingNative := make(map[string]connectorcore.TargetBinding, len(existing))
	for _, binding := range existing {
		existingNative[binding.ConnectorID.String()] = binding
	}

	normalized := normalizeAgentConnectorBindings(desired)
	stored := make([]connectorcore.TargetBinding, 0, len(normalized))
	retained := make(map[string]struct{}, len(normalized))
	now := time.Now().UTC()
	for _, item := range normalized {
		connectorID, normalizeErr := connectorcore.NormalizeIdentifier(item.ConnectorID)
		connectionID, connectionErr := uuid.Parse(item.ConnectionID)
		if normalizeErr != nil || connectionErr != nil || connectionID == uuid.Nil {
			return nil, &agentBindingAPIError{Code: agentBindingsInvalidCode, Message: "Connector binding identity is invalid"}
		}
		retained[connectorID.String()] = struct{}{}
		expectedRevision := 0
		candidate := connectorcore.TargetBinding{
			ID:             uuid.New(),
			Constraint:     connectorbindings.NewBindingConstraint(ownerID, connectorID),
			OrganizationID: organizationID,
			OwnerAccountID: ownerID,
			ConnectionID:   connectionID,
			ConnectorID:    connectorID,
			Target:         target,
			Status:         connectorcore.BindingStatusActive,

			Revision:  1,
			CreatedAt: now,
			UpdatedAt: now,
		}
		if previous, ok := existingNative[connectorID.String()]; ok {
			expectedRevision = previous.Revision
			candidate.ID = previous.ID
			candidate.Revision = previous.Revision
			candidate.CreatedAt = previous.CreatedAt
			candidate.Constraint = previous.Constraint
		}
		if item.AuthorizationMode != "" {
			candidate.Constraint.AuthorizationMode = item.AuthorizationMode
		}
		if item.BindingID != "" {
			sourceID, err := uuid.Parse(item.BindingID)
			if err != nil {
				return nil, connectorBindingMutationError(err)
			}
			source, err := repository.GetBinding(ctx, organizationID, ownerID, sourceID)
			if err != nil {
				return nil, err
			}
			if source.Target.Type != connectorcore.TargetTypeAgent || source.Target.ID != target.ID || source.ConnectorID != connectorID {
				return nil, fmt.Errorf("Connector snapshot identity is invalid")
			}
			candidate.Constraint, err = connectorcore.IntersectConstraints(candidate.Constraint, source.Constraint)
			if err != nil {
				return nil, err
			}
		}
		if candidate.Constraint.DirectlyAuthorized() && len(databases) > 0 && databases[0] != nil {
			if err := applyAgentAuthorizationSnapshot(ctx, databases[0], organizationID, ownerID, connectionID, connectorID, candidate.Constraint); err != nil {
				return nil, connectorBindingMutationError(err)
			}
		}
		binding, _, upsertErr := repository.UpsertBinding(ctx, organizationID, ownerID, candidate, expectedRevision)
		if upsertErr != nil {
			return nil, connectorBindingMutationError(upsertErr)
		}
		stored = append(stored, binding)
	}

	for connectorID, binding := range existingNative {
		if _, ok := retained[connectorID]; ok || binding.Status == connectorcore.BindingStatusDisabled {
			continue
		}
		if _, err := repository.SetBindingStatus(ctx, organizationID, ownerID, binding.ID, connectorcore.BindingStatusDisabled, binding.Revision); err != nil {
			return nil, connectorBindingMutationError(err)
		}
	}
	return stored, nil
}

func applyAgentAuthorizationSnapshot(
	ctx context.Context,
	db *gorm.DB,
	organizationID, ownerID, connectionID uuid.UUID,
	connectorID connectorcore.Identifier,
	constraint *connectorcore.AccessConstraint,
) error {
	if db == nil || constraint == nil || !constraint.DirectlyAuthorized() {
		return nil
	}
	var row struct {
		Revision          int    `gorm:"column:revision"`
		CredentialVersion int    `gorm:"column:credential_version"`
		ContractDigest    string `gorm:"column:contract_digest"`
	}
	err := db.WithContext(ctx).Table("connector_connections AS connection").
		Select("connection.revision, connection.credential_version, definition.contract_digest").
		Joins("JOIN connector_definitions AS definition ON definition.connector_id = connection.connector_id").
		Where("connection.id = ? AND connection.organization_id = ? AND connection.owner_account_id = ? AND connection.connector_id = ? AND connection.state = ? AND connection.deleted_at IS NULL", connectionID, organizationID, ownerID, connectorID.String(), connectorcore.ReadinessReady).
		Take(&row).Error
	if err != nil || row.Revision < 1 || row.CredentialVersion < 1 || row.ContractDigest == "" {
		if err != nil {
			return fmt.Errorf("load Agent Connector authorization snapshot: %w", err)
		}
		return fmt.Errorf("load Agent Connector authorization snapshot: incomplete")
	}
	constraint.AuthorizationConnectionRevision = row.Revision
	constraint.AuthorizationCredentialVersion = row.CredentialVersion
	constraint.AuthorizationContractDigest = row.ContractDigest
	return nil
}

func connectorBindingMutationError(err error) error {
	switch connectorcore.ErrorCodeOf(err) {
	case connectorcore.ErrorCodeConflict:
		return &agentBindingAPIError{Code: agentBindingRevisionConflictCode, Message: "Connector binding has changed"}
	case connectorcore.ErrorCodeInvalidInput, connectorcore.ErrorCodeNotFound,
		connectorcore.ErrorCodeAccessDenied, connectorcore.ErrorCodeConnectionNotReady:
		return &agentBindingAPIError{Code: agentBindingsInvalidCode, Message: "Connector Connection is not available to this account"}
	default:
		return fmt.Errorf("persist Agent Connector Target Binding: %w", err)
	}
}
