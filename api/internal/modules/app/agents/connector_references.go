package agents

import (
	"context"
	"fmt"
	"github.com/google/uuid"
	"github.com/zgiai/zgi/api/internal/dto"
	"github.com/zgiai/zgi/api/internal/modules/connectors/bindings"
	"github.com/zgiai/zgi/api/internal/modules/connectors/core"
	"gorm.io/gorm"
)

func (s *agentsService) resolveAgentConnectorReferences(ctx context.Context, tx *gorm.DB, agent *Agent, accountID string, request *dto.AgentConfigRequest) error {
	if len(request.ConnectorReferenceCommands) == 0 {
		return nil
	}
	org, err := s.organizationUUIDForAgentWorkspace(ctx, agent.TenantID.String())
	if err != nil {
		return err
	}
	owner, err := uuid.Parse(accountID)
	if err != nil {
		return err
	}
	target := bindings.TargetIdentity{Type: core.TargetTypeAgent, ID: agent.ID.String(), Scope: core.ScopeDraft}
	refs, err := bindings.ListReferencesInTransaction(ctx, tx, org, owner, target)
	if err != nil {
		return err
	}
	former := map[uuid.UUID]string{}
	for _, ref := range refs {
		former[ref.ID] = ref.FormerReferenceID
	}
	seen := map[uuid.UUID]bool{}
	for _, command := range request.ConnectorReferenceCommands {
		if seen[command.ReferenceID] {
			return fmt.Errorf("duplicate Connector reference command")
		}
		seen[command.ReferenceID] = true
		binding, err := bindings.ResolveReferenceInTransaction(ctx, tx, org, owner, target, command)
		if err != nil {
			return err
		}
		kept := request.ConnectorBindings[:0]
		for _, existing := range request.ConnectorBindings {
			if existing.BindingID != former[command.ReferenceID] && existing.ConnectionID != former[command.ReferenceID] {
				kept = append(kept, existing)
			}
		}
		request.ConnectorBindings = kept
		if binding != nil {
			filtered := make([]dto.AgentConnectorBinding, 0, len(request.ConnectorBindings)+1)
			for _, existing := range request.ConnectorBindings {
				if existing.ConnectorID != binding.ConnectorID.String() {
					filtered = append(filtered, existing)
				}
			}
			request.ConnectorBindings = append(filtered, dto.AgentConnectorBinding{ConnectorID: binding.ConnectorID.String(), ConnectionID: binding.ConnectionID.String(), BindingID: binding.ID.String(), Revision: binding.Revision, AuthorizationMode: binding.Constraint.AuthorizationModeValue()})
		}
	}
	return nil
}
