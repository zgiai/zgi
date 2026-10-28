package digitalhuman

import (
	"context"
	"encoding/json"
	"fmt"
	"slices"
	"time"

	"github.com/google/uuid"
	"github.com/zgiai/zgi/api/internal/capabilities/resourcebindings"
	"github.com/zgiai/zgi/api/internal/modules/connectors/bindings"
	"github.com/zgiai/zgi/api/internal/modules/connectors/catalog"
	"github.com/zgiai/zgi/api/internal/modules/connectors/connections"
	"github.com/zgiai/zgi/api/internal/modules/connectors/core"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// ConnectorCatalog exposes only admitted definitions. Historical resource rows
// remain in the DigitalHuman aggregate and do not create execution authority.
type ConnectorCatalog interface {
	Get(string) (catalog.Definition, bool)
}

func (r *GormRepository) SetConnectorCatalog(c ConnectorCatalog) { r.connectorCatalog = c }

func digitalHumanConnectorTarget(id, version, instance uuid.UUID, phase core.TargetScope) core.TargetRef {
	target := core.TargetRef{Type: core.TargetTypeDigitalHuman, ID: id.String(), Scope: phase, BindingInstanceID: instance.String()}
	if phase == core.ScopePublished {
		target.VersionID = version.String()
	}
	return target
}

func bindingOperations(row *ResourceBinding) ([]core.Identifier, error) {
	var metadata struct {
		Operations []core.Identifier `json:"allowed_operation_ids"`
	}
	if json.Unmarshal(row.Metadata, &metadata) != nil || len(metadata.Operations) == 0 {
		return nil, ErrResourceBindingInvalid
	}
	seen := map[core.Identifier]bool{}
	for _, id := range metadata.Operations {
		if !id.Valid() || seen[id] {
			return nil, ErrResourceBindingInvalid
		}
		seen[id] = true
	}
	return metadata.Operations, nil
}

// Connector binding approval policies (ADR 0026). always_ask, the default,
// asks before every call; read_only_auto runs calls the connector proves
// read-only without asking.
const (
	ConnectorApprovalAlwaysAsk    = "always_ask"
	ConnectorApprovalReadOnlyAuto = "read_only_auto"
)

// bindingAuthorizationMode maps a binding's approval_policy to the authorization
// mode of its connector constraint. Only a connector that can prove a call
// read-only accepts read_only_auto.
func bindingAuthorizationMode(row *ResourceBinding, definition catalog.Definition) (string, error) {
	var metadata struct {
		Policy string `json:"approval_policy"`
	}
	_ = json.Unmarshal(row.Metadata, &metadata)
	switch metadata.Policy {
	case "", ConnectorApprovalAlwaysAsk:
		return core.AuthorizationModeAsk, nil
	case ConnectorApprovalReadOnlyAuto:
		if !definition.IsServerOpsSSHCredentials() {
			return "", ErrResourceBindingInvalid
		}
		return core.AuthorizationModeReadOnlyAuto, nil
	}
	return "", ErrResourceBindingInvalid
}

// syncConnectorBindings runs inside the same host transaction as resource saves
// and publication. The canonical repository also locks connection lifecycle
// authority, so force-removal cannot race with creating a host reference.
func (r *GormRepository) syncConnectorBindings(ctx context.Context, scope Scope, id, version uuid.UUID, phase core.TargetScope, rows []*ResourceBinding, keepOnly map[uuid.UUID]bool) error {
	if phase != core.ScopeDraft {
		keepOnly = nil
	}
	desired := map[uuid.UUID]*ResourceBinding{}
	for _, row := range rows {
		if row.BindingType == resourcebindings.BindingTypeConnector && connectorCredentialMode(row) == ConnectorCredentialShared {
			desired[row.ID] = row
		}
	}
	if r.connectorCatalog == nil {
		// Without a catalog no Connector selection, shared or per-user, can be
		// admitted; only unchanged rows are kept.
		for _, row := range rows {
			if row.BindingType == resourcebindings.BindingTypeConnector && !keepOnly[row.ID] {
				return ErrResourceBindingInvalid
			}
		}
		if len(desired) == 0 {
			return nil
		}
	}
	// Disable references belonging to resources removed from this mutable draft,
	// including references originally authorized by another editor.
	var old []struct {
		ID, OwnerAccountID uuid.UUID
		BindingInstanceID  string
		Revision           int
		Status             string
	}
	query := r.db.WithContext(ctx).Table("connector_target_bindings").Where("organization_id = ? AND target_type = ? AND target_id = ? AND target_scope = ? AND target_version_id = ?", scope.OrganizationID, core.TargetTypeDigitalHuman, id.String(), phase, "")
	if phase == core.ScopePublished {
		query = r.db.WithContext(ctx).Table("connector_target_bindings").Where("organization_id = ? AND target_type = ? AND target_id = ? AND target_scope = ? AND target_version_id = ?", scope.OrganizationID, core.TargetTypeDigitalHuman, id.String(), phase, version.String())
	}
	if err := query.Find(&old).Error; err != nil {
		return err
	}
	native := bindings.NewGormRepository(r.db.Session(&gorm.Session{DisableNestedTransaction: true}))
	oldByInstance := map[string]struct {
		ID, Owner uuid.UUID
		Revision  int
	}{}
	for _, prior := range old {
		oldByInstance[prior.BindingInstanceID] = struct {
			ID, Owner uuid.UUID
			Revision  int
		}{prior.ID, prior.OwnerAccountID, prior.Revision}
		instance, err := uuid.Parse(prior.BindingInstanceID)
		if err != nil {
			return ErrResourceBindingInvalid
		}
		if _, keep := desired[instance]; keep || prior.Status == string(core.BindingStatusDisabled) {
			continue
		}
		if phase == core.ScopePublished {
			return ErrPublishedImmutable
		}
		if _, err := native.SetBindingStatusWithLifecycle(ctx, scope.OrganizationID, prior.OwnerAccountID, prior.ID, core.BindingStatusDisabled, prior.Revision, bindings.BindingLifecycleMutation{ID: uuid.New(), MutationType: bindings.MutationBindingDetached, CorrelationID: "digital-human-resource-save:" + uuid.NewString(), CreatedAt: time.Now().UTC()}); err != nil {
			return connectorBindingMutationError(err)
		}
	}
	for _, row := range rows {
		if row.BindingType != resourcebindings.BindingTypeConnector {
			continue
		}
		if keepOnly[row.ID] {
			// Preserve even a missing/disabled reference without granting authority.
			// Only a changed selection passes through admission and lifecycle writes.
			continue
		}
		if connectorCredentialMode(row) == ConnectorCredentialPerUser {
			// A per-user slot names no Connection of its editor. Each requester
			// binds their own when they run the published version.
			if err := r.validatePerUserConnectorSlot(row); err != nil {
				return err
			}
			continue
		}
		if row.AuthorizedBy == nil || *row.AuthorizedBy == uuid.Nil || row.CredentialRef != "" {
			return ErrResourceBindingInvalid
		}
		connectionID, err := uuid.Parse(row.ResourceID)
		if err != nil || connectionID == uuid.Nil || connectionID.String() != row.ResourceID {
			return ErrResourceBindingInvalid
		}
		owner := *row.AuthorizedBy
		// Hold the lifecycle row before reading readiness and operation constraints;
		// reconnect/removal cannot change the authority between admission and save.
		var locked struct{ ID uuid.UUID }
		if err := r.db.WithContext(ctx).Table("connector_connections").Select("id").Clauses(clause.Locking{Strength: "UPDATE"}).Where("id = ? AND organization_id = ? AND owner_account_id = ?", connectionID, scope.OrganizationID, owner).Take(&locked).Error; err != nil {
			return ErrResourceBindingInvalid
		}
		connection, err := connections.NewGormRepository(r.db).GetConnection(ctx, scope.OrganizationID, owner, connectionID)
		if err != nil || connection.ConnectorID.String() != row.ParentResourceID || connection.State != core.ReadinessReady {
			return ErrResourceBindingInvalid
		}
		definition, ok := r.connectorCatalog.Get(row.ParentResourceID)
		if !ok || !definition.Supports(catalog.SurfaceDigitalHuman) || catalog.Validate(definition) != nil {
			return ErrResourceBindingInvalid
		}
		operations, err := bindingOperations(row)
		if err != nil {
			return err
		}
		authorizationMode, err := bindingAuthorizationMode(row, definition)
		if err != nil {
			return err
		}
		target := digitalHumanConnectorTarget(id, version, row.ID, phase)
		now := time.Now().UTC()
		bindingConstraint := core.OwnerConstraint(owner, operations)
		bindingConstraint.AuthorizationMode = authorizationMode
		bindingConstraint.TargetBoundaries = []core.TargetBoundary{{
			Type: target.Type, ID: target.ID, Scope: string(target.EffectiveScope()), VersionID: target.VersionID,
		}}
		for _, operation := range operations {
			if !slices.Contains(connection.UsableOperationIDs, operation) ||
				!core.ConstraintsAllow(connection.Constraint, bindingConstraint, owner, operation, target, now) {
				return ErrResourceBindingInvalid
			}
			found := false
			for _, op := range definition.Operations {
				if op.ID == operation.String() {
					found = true
					break
				}
			}
			if !found {
				return ErrResourceBindingInvalid
			}
		}
		bindingID := uuid.NewSHA1(row.ID, []byte("digital-human-connector:"+string(phase)))
		revision := 0
		if previous, ok := oldByInstance[row.ID.String()]; ok {
			if previous.Owner != owner {
				return ErrResourceBindingInvalid
			}
			bindingID, revision = previous.ID, previous.Revision
		}
		canonical := core.TargetBinding{ID: bindingID, OrganizationID: scope.OrganizationID, OwnerAccountID: owner, ConnectionID: connectionID, ConnectorID: connection.ConnectorID, Target: target, Constraint: bindingConstraint, Status: core.BindingStatusActive, Revision: 1, CreatedAt: now, UpdatedAt: now}
		if revision > 0 {
			canonical.Revision = revision
		}
		mutation := bindings.BindingLifecycleMutation{ID: uuid.New(), MutationType: bindings.MutationBindingUpserted, CorrelationID: "digital-human-resource-save:" + uuid.NewString(), CreatedAt: now}
		if phase == core.ScopeDraft && revision > 0 {
			if _, err := native.ReplaceDigitalHumanBindingWithLifecycle(ctx, scope.OrganizationID, owner, canonical, revision, mutation); err != nil {
				return connectorBindingMutationError(err)
			}
		} else if _, _, err := native.UpsertBindingWithLifecycle(ctx, scope.OrganizationID, owner, canonical, revision, mutation); err != nil {
			return connectorBindingMutationError(err)
		}
	}
	return nil
}

func connectorBindingMutationError(err error) error {
	if core.ErrorCodeOf(err) == core.ErrorCodeInternal {
		return fmt.Errorf("synchronize DigitalHuman Connector binding: %w", err)
	}
	return fmt.Errorf("%w: %w", ErrResourceBindingInvalid, err)
}

// ListConnectorCandidates projects the caller's own Connector connections for
// the pre-creation surface. It applies the same Digital Human eligibility,
// readiness, and operation constraints as the draft editor, so creation and
// configuration can never offer different connections. It is exported because
// creation happens before a draft id exists.
func (r *GormRepository) ListConnectorCandidates(ctx context.Context, scope Scope) ([]ResourceCandidate, error) {
	result := []ResourceCandidate{}
	if r.connectorCatalog == nil {
		return result, nil
	}
	rows, err := connections.NewGormRepository(r.db).ListConnections(ctx, scope.OrganizationID, scope.ActorID, connections.ConnectionListFilter{})
	if err != nil {
		return nil, err
	}
	for _, connection := range rows {
		definition, ok := r.connectorCatalog.Get(connection.ConnectorID.String())
		if !ok || !definition.Supports(catalog.SurfaceDigitalHuman) {
			continue
		}
		operations := []core.Identifier{}
		for _, op := range definition.Operations {
			operation := core.Identifier(op.ID)
			if slices.Contains(connection.UsableOperationIDs, operation) && connection.Constraint != nil &&
				connection.Constraint.Validate() == nil && connection.Constraint.AudienceOwner == scope.ActorID &&
				len(connection.Constraint.DenyReasons) == 0 &&
				(connection.Constraint.ExpiresAt == nil || time.Now().Before(*connection.Constraint.ExpiresAt)) &&
				slices.Contains(connection.Constraint.AllowedOperationIDs, operation) {
				operations = append(operations, operation)
			}
		}
		result = append(result, ResourceCandidate{Type: resourcebindings.BindingTypeConnector, ResourceID: connection.ID.String(), ParentResourceID: connection.ConnectorID.String(), DisplayName: connection.Name, Description: definition.Description, ResourceRevision: fmt.Sprintf("connector:%d", connection.Revision), AccessModes: []resourcebindings.AccessMode{resourcebindings.AccessModeExecute}, Metadata: map[string]any{"allowed_operation_ids": operations}, Status: string(connection.State), Available: connection.State == core.ReadinessReady && len(operations) > 0})
	}
	return result, nil
}
