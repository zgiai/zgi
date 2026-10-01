package agents

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"github.com/zgiai/zgi/api/internal/modules/connectors/connections"
	"time"

	"github.com/google/uuid"
	"github.com/zgiai/zgi/api/internal/capabilities/agentbindings"
	"github.com/zgiai/zgi/api/internal/dto"
	connectorbindings "github.com/zgiai/zgi/api/internal/modules/connectors/bindings"
	connectorcore "github.com/zgiai/zgi/api/internal/modules/connectors/core"
	"gorm.io/gorm"
)

func connectorBindingViews(bindings []connectorcore.TargetBinding) []dto.AgentConnectorBinding {
	result := make([]dto.AgentConnectorBinding, 0, len(bindings))
	for _, binding := range bindings {
		result = append(result, dto.AgentConnectorBinding{BindingID: binding.ID.String(), Revision: binding.Revision, ConnectorID: binding.ConnectorID.String(), ConnectionID: binding.ConnectionID.String(), AuthorizationMode: binding.Constraint.AuthorizationModeValue()})
	}
	return normalizeAgentConnectorBindings(result)
}

func storeDraftConnectorSnapshot(config *AgentsConfig, bindings []connectorcore.TargetBinding) error {
	mode := agentRuntimeModeFromConfig(config)
	mode.ConnectorBindings = connectorBindingViews(bindings)
	encoded, err := json.Marshal(mode)
	if err != nil {
		return err
	}
	text := string(encoded)
	config.AgentMode = &text
	return nil
}

// A published version receives fresh identities and the exact draft restriction.
// Its rows remain immutable when a subsequent version becomes the head.
func (s *agentsService) snapshotAgentConnectorBindings(ctx context.Context, tx *gorm.DB, agent *Agent, accountID string, version uuid.UUID, expected []dto.AgentConnectorBinding) ([]dto.AgentConnectorBinding, error) {
	if tx == nil || agent == nil || version == uuid.Nil {
		return nil, fmt.Errorf("Connector snapshot context is required")
	}
	org, err := s.organizationUUIDForAgentWorkspace(ctx, agent.TenantID.String())
	if err != nil {
		return nil, err
	}
	owner, err := uuid.Parse(accountID)
	if err != nil || owner == uuid.Nil {
		return nil, fmt.Errorf("Connector owner is invalid")
	}
	refs, err := connectorbindings.ListReferencesInTransaction(ctx, tx, org, owner, connectorbindings.TargetIdentity{Type: connectorcore.TargetTypeAgent, ID: agent.ID.String(), Scope: connectorcore.ScopeDraft})
	if err != nil {
		return nil, err
	}
	for _, ref := range refs {
		if ref.Status == "needs_replacement" {
			return nil, fmt.Errorf("Connector references require replacement before publishing")
		}
	}
	repo := connectorbindings.NewGormRepository(tx)
	source, err := repo.ListBindingsByTarget(ctx, org, owner, connectorcore.TargetTypeAgent, agent.ID.String(), connectorbindings.ListOptions{Scope: connectorcore.ScopeDraft, Statuses: []connectorcore.BindingStatus{connectorcore.BindingStatusActive}})
	if err != nil {
		return nil, err
	}
	if len(expected) != len(source) {
		return nil, fmt.Errorf("Connector draft selection changed")
	}
	selected := map[string]dto.AgentConnectorBinding{}
	for _, binding := range expected {
		selected[binding.BindingID] = binding
	}
	for _, binding := range source {
		ref, found := selected[binding.ID.String()]
		if !found || ref.ConnectorID != binding.ConnectorID.String() || ref.ConnectionID != binding.ConnectionID.String() {
			return nil, fmt.Errorf("Connector draft selection changed")
		}
	}
	destination := make([]connectorcore.TargetBinding, 0, len(source))
	for _, binding := range source {
		if binding.Constraint.Validate() != nil {
			return nil, fmt.Errorf("Connector binding restrictions are unavailable")
		}
		binding.ID = uuid.New()
		binding.Target.Scope = connectorcore.ScopePublished
		binding.Target.VersionID = version.String()
		binding.Revision = 1
		binding.CreatedAt = time.Now().UTC()
		binding.UpdatedAt = binding.CreatedAt
		stored, _, err := repo.UpsertBinding(ctx, org, owner, binding, 0)
		if err != nil {
			return nil, err
		}
		destination = append(destination, stored)
	}
	return connectorBindingViews(destination), nil
}

func agentBindingRevisionWithConnectors(rows []agentbindings.Binding, bindings []dto.AgentConnectorBinding) string {
	base := agentBindingRevision(rows)
	if len(bindings) == 0 {
		return base
	}
	encoded, _ := json.Marshal(normalizeAgentConnectorBindings(bindings))
	hash := sha256.Sum256(append([]byte(base), encoded...))
	return hex.EncodeToString(hash[:])
}

// Connector health is presented alongside other resources without writing a
// second authorization ledger into agent_resource_bindings.
func (s *agentsService) nativeConnectorHealthItems(ctx context.Context, agent *Agent, accountID string, config *dto.AgentConfigResponse) []dto.AgentBindingHealthItem {
	if config == nil || agent == nil {
		return nil
	}
	org, orgErr := s.organizationUUIDForAgentWorkspace(ctx, agent.TenantID.String())
	owner, ownerErr := uuid.Parse(accountID)
	target := connectorcore.TargetRef{Type: connectorcore.TargetTypeAgent, ID: agent.ID.String(), Scope: connectorcore.TargetScope(config.ConnectorTargetScope), VersionID: config.ConnectorTargetVersionID}
	items := make([]dto.AgentBindingHealthItem, 0, len(config.ConnectorBindings))
	for _, selected := range normalizeAgentConnectorBindings(config.ConnectorBindings) {
		item := dto.AgentBindingHealthItem{BindingType: agentConnectorBindingType, ResourceID: selected.ConnectionID, ParentResourceID: selected.ConnectorID, AccessMode: "execute", Status: agentBindingStatusUnavailable, Reason: agentBindingReasonAuthorizationRevoked, Suggestion: "remove_or_replace_binding"}
		bindingID, parseErr := uuid.Parse(selected.BindingID)
		if orgErr == nil && ownerErr == nil && parseErr == nil && s.db != nil {
			binding, err := connectorbindings.NewGormRepository(s.db).GetBinding(ctx, org, owner, bindingID)
			if err == nil && binding.OwnerAccountID == owner && binding.OrganizationID == org && binding.Target.SameIdentity(target) && binding.ConnectionID.String() == selected.ConnectionID && binding.ConnectorID.String() == selected.ConnectorID && binding.Status == connectorcore.BindingStatusActive {
				connection, err := connections.NewGormRepository(s.db).GetConnection(ctx, org, owner, binding.ConnectionID)
				state, stateErr := s.loadOwnedAgentConnectorConnection(ctx, org, owner, binding.ConnectionID)
				if err == nil && stateErr == nil && state.DefinitionStatus == string(connectorcore.DefinitionStatusActive) && connection.State == connectorcore.ReadinessReady && connection.ConnectorID == binding.ConnectorID && hasUsableAgentConnectorOperation(binding, connection, time.Now().UTC()) {
					item.Status, item.Reason, item.Suggestion = agentBindingStatusActive, "", ""
					item.DisplayName = connection.Name
				}
			}
		}
		items = append(items, item)
	}
	if orgErr == nil && ownerErr == nil && s.db != nil && s.connectorTargetBindings != nil {
		refs, err := connectorbindings.ListReferencesInTransaction(ctx, s.db, org, owner, connectorbindings.TargetIdentity{Type: target.Type, ID: target.ID, Scope: target.EffectiveScope(), VersionID: target.VersionID})
		if err != nil {
			items = append(items, dto.AgentBindingHealthItem{BindingType: agentConnectorBindingType, Status: agentBindingStatusUnavailable, Reason: agentBindingReasonResolutionFailed})
		}
		for _, ref := range refs {
			items = append(items, dto.AgentBindingHealthItem{BindingType: agentConnectorBindingType, ResourceID: ref.ID.String(), DisplayName: ref.Label, AccessMode: "execute", Status: agentBindingStatusUnavailable, Reason: ref.ReasonCode, Suggestion: "remove_or_replace_binding"})
		}
	}
	return items
}

func hasUsableAgentConnectorOperation(binding connectorcore.TargetBinding, connection connectorcore.PersonalConnection, now time.Time) bool {
	if binding.Constraint.Validate() != nil || connection.Constraint.Validate() != nil {
		return false
	}
	for _, operation := range binding.Constraint.AllowedOperationIDs {
		if binding.Constraint.Allows(binding.OwnerAccountID, operation, binding.Target, now) && connection.Constraint.Allows(binding.OwnerAccountID, operation, binding.Target, now) {
			return true
		}
	}
	return false
}

func (s *agentsService) recordUnavailableRestoredConnectors(ctx context.Context, tx *gorm.DB, agent *Agent, owner, version uuid.UUID, original, retained []dto.AgentConnectorBinding) error {
	kept := map[string]bool{}
	for _, binding := range retained {
		kept[binding.ConnectorID+":"+binding.ConnectionID] = true
	}
	for _, binding := range original {
		if kept[binding.ConnectorID+":"+binding.ConnectionID] {
			continue
		}
		org, err := s.organizationUUIDForAgentWorkspace(ctx, agent.TenantID.String())
		if err != nil {
			return err
		}
		_, err = connectorbindings.EnsureUnavailableReference(ctx, tx, org, owner, connectorcore.TargetRef{Type: connectorcore.TargetTypeAgent, ID: agent.ID.String(), Scope: connectorcore.ScopeDraft}, "restore:"+version.String()+":"+binding.ConnectionID, binding.ConnectorID, "snapshot_connection_unavailable")
		if err != nil {
			return err
		}
	}
	return nil
}
