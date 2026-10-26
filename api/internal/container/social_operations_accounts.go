package container

import (
	"context"
	"github.com/google/uuid"
	"github.com/zgiai/zgi/api/internal/modules/connectors/bindings"
	"github.com/zgiai/zgi/api/internal/modules/connectors/core"
	"github.com/zgiai/zgi/api/internal/modules/digitalhuman"
	"github.com/zgiai/zgi/api/internal/modules/socialops"
	"strings"
)

func (s *SocialOperationsSetupService) ListAccounts(ctx context.Context, scope digitalhuman.Scope, employee uuid.UUID) ([]socialops.AccountConnection, error) {
	_, version, err := s.restoreManagementScope(ctx, scope, employee)
	if err != nil {
		return nil, err
	}
	rows, err := s.restorer.bindings.ListBindingsByTarget(ctx, scope.OrganizationID, scope.ActorID, core.TargetTypeDigitalHuman, employee.String(), bindings.ListOptions{Scope: core.ScopePublished, VersionID: version.String(), Statuses: []core.BindingStatus{core.BindingStatusActive}})
	if err != nil {
		return nil, err
	}
	result := []socialops.AccountConnection{}
	seen := map[uuid.UUID]bool{}
	for _, row := range rows {
		connector := row.ConnectorID.String()
		if connector != "x-social" && connector != "linkedin-social" && connector != "facebook-social" {
			continue
		}
		if row.OrganizationID != scope.OrganizationID || row.OwnerAccountID != scope.ActorID || row.Target.Type != core.TargetTypeDigitalHuman || row.Target.ID != employee.String() || row.Target.VersionID != version.String() || row.Target.EffectiveScope() != core.ScopePublished || row.Status != core.BindingStatusActive || row.ConnectionID == uuid.Nil || seen[row.ConnectionID] {
			continue
		}
		seen[row.ConnectionID] = true
		connection, err := s.restorer.connections.GetConnection(ctx, scope.OrganizationID, scope.ActorID, row.ConnectionID)
		if err != nil {
			return nil, err
		}
		if connection.ID != row.ConnectionID || connection.OrganizationID != scope.OrganizationID || connection.OwnerAccountID != scope.ActorID || connection.ConnectorID != row.ConnectorID || connection.State != core.ReadinessReady {
			continue
		}
		item := socialops.AccountConnection{ConnectorID: connector, ConnectionID: connection.ID, Accounts: []socialops.AccountOption{}, VerifiedAt: connection.LastVerifiedAt}
		for _, id := range socialOperationsKnownAccountIDs(connection) {
			if connector == "linkedin-social" && !strings.HasPrefix(id, "urn:li:organization:") {
				continue
			}
			label := id
			if connector != "linkedin-social" && connection.ExternalDisplayName != nil && *connection.ExternalDisplayName != "" {
				label = *connection.ExternalDisplayName
			}
			item.Accounts = append(item.Accounts, socialops.AccountOption{ID: id, Label: label})
		}
		result = append(result, item)
	}
	return result, nil
}
