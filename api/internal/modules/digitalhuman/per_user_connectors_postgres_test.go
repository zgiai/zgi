package digitalhuman

import (
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/zgiai/zgi/api/internal/capabilities/resourcebindings"
	"github.com/zgiai/zgi/api/internal/capabilities/usersubject"
	"github.com/zgiai/zgi/api/internal/modules/connectors/bindings"
	"github.com/zgiai/zgi/api/internal/modules/connectors/connections"
	"github.com/zgiai/zgi/api/internal/modules/connectors/core"
	"github.com/zgiai/zgi/api/pkg/apperror"
)

func TestPerUserConnectorSlotPostgres(t *testing.T) {
	t.Setenv("ZGI_DIGITAL_HUMAN_PER_USER_CONNECTORS_ENABLED", "true")
	f := newDigitalHumanConnectorPostgresFixture(t)
	db := f.repo.db
	exec := func(query string, args ...any) {
		t.Helper()
		if err := db.Exec(query, args...).Error; err != nil {
			t.Fatal(err)
		}
	}
	count := func(query string, args ...any) int64 {
		t.Helper()
		var n int64
		if err := db.Raw(query, args...).Scan(&n).Error; err != nil {
			t.Fatal(err)
		}
		return n
	}
	slotKey := uuid.NewString()
	operation := f.definition.Operations[0].ID
	slot := ResourceBindingInput{Type: resourcebindings.BindingTypeConnector, ResourceID: slotKey, ParentResourceID: f.definition.ID,
		DisplayName: "Their own search", AccessMode: resourcebindings.AccessModeExecute, ResourceRevision: "connector:per-user",
		Metadata: map[string]any{"allowed_operation_ids": []string{operation}, "credential_mode": ConnectorCredentialPerUser}}
	// Without a connector catalog no Connector selection is admitted, a
	// per-user slot included.
	uncataloged := NewService(NewRepository(db))
	if _, err := uncataloged.ReplaceDraftResourceBindings(t.Context(), f.scope, f.detail.Definition.ID, ReplaceResourceBindingsRequest{ExpectedDraftLockVersion: 1, Items: []ResourceBindingInput{slot}}); !apperror.IsCode(err, AppCodeResourceBindingInvalid) {
		t.Fatalf("a per-user slot was saved without a catalog: %v", err)
	}
	shared := f.input(0, 1)
	saved, err := f.service.ReplaceDraftResourceBindings(t.Context(), f.scope, f.detail.Definition.ID, ReplaceResourceBindingsRequest{ExpectedDraftLockVersion: 1, Items: []ResourceBindingInput{slot, shared}})
	if err != nil {
		t.Fatal(err)
	}
	if native := f.canonical(t, core.ScopeDraft, uuid.Nil); len(native) != 1 || native[0].ConnectionID != f.connections[0].ID {
		t.Fatalf("the per-user slot became an editor binding: %#v", native)
	}
	published, err := f.service.Publish(t.Context(), f.scope, f.detail.Definition.ID, PublishRequest{ExpectedRevision: f.detail.Definition.Revision, ExpectedDraftLockVersion: saved.DraftLockVersion})
	if err != nil {
		t.Fatal(err)
	}
	versionID := published.Published.ID

	endUser := func() uuid.UUID {
		t.Helper()
		subjectID := uuid.New()
		exec("INSERT INTO user_subjects (id, kind, issuer_type, issuer_id, created_at, updated_at) VALUES (?, 'api_end_user', 'runtime_workspace_api', ?, now(), now())", subjectID, uuid.New())
		return subjectID
	}
	alice, bob := endUser(), endUser()
	operations := []core.Identifier{core.Identifier(operation)}
	now := time.Now().UTC()
	connection := core.PersonalConnection{ID: uuid.New(), OrganizationID: f.scope.OrganizationID, OwnerAccountID: alice,
		ConnectorID: core.Identifier(f.definition.ID), ProviderID: core.Identifier(f.definition.ProviderID), DriverID: core.Identifier(f.definition.DriverID), AuthMethodID: core.Identifier(f.definition.AuthMethodID),
		Name: "Alice's key", EncryptedCredentials: "test-envelope-with-no-real-credentials", CredentialVersion: 1,
		State: core.ReadinessReady, Revision: 1, UsableOperationIDs: operations, UnavailableOperations: map[core.Identifier]string{},
		Constraint: core.OwnerConstraint(alice, operations), LastVerifiedAt: now, CreatedAt: now, UpdatedAt: now}
	if err := connections.NewGormRepository(db).CreateConnection(t.Context(), connection); err != nil {
		t.Fatal(err)
	}
	if n := count("SELECT count(*) FROM connector_connections WHERE id = ? AND owner_kind = 'end_user'", connection.ID); n != 1 {
		t.Fatal("an end user's connection was not marked as theirs")
	}
	if err := db.Create(&ConnectorSelection{ID: uuid.New(), OrganizationID: f.scope.OrganizationID, DigitalHumanID: f.detail.Definition.ID,
		OwnerKind: connectorOwnerEndUser, OwnerID: alice, SlotKey: slotKey, ConnectorID: f.definition.ID, ConnectionID: connection.ID,
		CreatedAt: now, UpdatedAt: now}).Error; err != nil {
		t.Fatal(err)
	}
	run := func(subject uuid.UUID) *Run {
		return &Run{ID: uuid.New(), OrganizationID: f.scope.OrganizationID, WorkspaceID: f.scope.WorkspaceID, DigitalHumanID: f.detail.Definition.ID,
			DigitalHumanVersionID: versionID, Surface: SurfaceAPI, UserSubjectKind: string(usersubject.APIEndUser), UserSubjectID: &subject}
	}

	missing, err := preparePerUserConnectors(t.Context(), db, f.repo.connectorCatalog, run(alice))
	if err != nil || len(missing) != 0 {
		t.Fatalf("alice's connected slot = %#v, %v", missing, err)
	}
	owned, err := bindings.NewGormRepository(db).ListBindingsByTarget(t.Context(), f.scope.OrganizationID, alice, core.TargetTypeDigitalHuman, f.detail.Definition.ID.String(), bindings.ListOptions{Scope: core.ScopePublished, VersionID: versionID.String()})
	if err != nil || len(owned) != 1 || owned[0].ConnectionID != connection.ID || owned[0].Status != core.BindingStatusActive || owned[0].Target.VersionID != versionID.String() {
		t.Fatalf("alice's own binding = %#v, %v", owned, err)
	}
	lifecycle := count("SELECT count(*) FROM connector_lifecycle_decisions WHERE owner_account_id = ?", alice)
	if _, err := preparePerUserConnectors(t.Context(), db, f.repo.connectorCatalog, run(alice)); err != nil || count("SELECT count(*) FROM connector_lifecycle_decisions WHERE owner_account_id = ?", alice) != lifecycle {
		t.Fatalf("a second run rebound alice's slot: %v", err)
	}
	// Alice removes her key and connects another; her binding to the
	// published slot follows the new Connection.
	if err := removeOwnerConnection(t.Context(), db, f.scope.OrganizationID, alice, connection.ID); err != nil {
		t.Fatal(err)
	}
	replacement := connection
	replacement.ID, replacement.Name, replacement.EncryptedCredentials = uuid.New(), "Alice's new key", "test-envelope-with-no-real-credentials-2"
	if err := connections.NewGormRepository(db).CreateConnection(t.Context(), replacement); err != nil {
		t.Fatal(err)
	}
	exec("UPDATE digital_human_connector_selections SET connection_id = ? WHERE owner_id = ?", replacement.ID, alice)
	if missing, err := preparePerUserConnectors(t.Context(), db, f.repo.connectorCatalog, run(alice)); err != nil || len(missing) != 0 {
		t.Fatalf("alice's replaced connection = %#v, %v", missing, err)
	}
	owned, err = bindings.NewGormRepository(db).ListBindingsByTarget(t.Context(), f.scope.OrganizationID, alice, core.TargetTypeDigitalHuman, f.detail.Definition.ID.String(), bindings.ListOptions{Scope: core.ScopePublished, VersionID: versionID.String()})
	if err != nil || len(owned) != 1 || owned[0].ConnectionID != replacement.ID || owned[0].Status != core.BindingStatusActive {
		t.Fatalf("alice's binding after reconnecting = %#v, %v", owned, err)
	}
	connection = replacement

	// Dropping the selection revokes alice's binding, and execution refuses
	// a binding that does not follow her current selection; choosing again
	// restores it.
	exec("DELETE FROM digital_human_connector_selections WHERE owner_id = ?", alice)
	if missing, err := preparePerUserConnectors(t.Context(), db, f.repo.connectorCatalog, run(alice)); err != nil || len(missing) != 1 {
		t.Fatalf("alice's slot without a selection = %#v, %v", missing, err)
	}
	if n := count("SELECT count(*) FROM connector_target_bindings WHERE owner_account_id = ? AND status = 'active'", alice); n != 0 {
		t.Fatalf("alice kept %d active bindings without a selection", n)
	}
	var slotRow ResourceBinding
	if err := db.Where("digital_human_version_id = ? AND resource_id = ?", versionID, slotKey).Take(&slotRow).Error; err != nil {
		t.Fatal(err)
	}
	slotRowID := slotRow.ID
	if selected, err := PerUserBindingSelected(t.Context(), db, perUserBindingID(slotRowID, alice), slotRowID, f.detail.Definition.ID, alice, slotKey, connection.ID); err != nil || selected {
		t.Fatalf("a binding without a selection was accepted for execution: %v %v", selected, err)
	}
	if err := db.Create(&ConnectorSelection{ID: uuid.New(), OrganizationID: f.scope.OrganizationID, DigitalHumanID: f.detail.Definition.ID,
		OwnerKind: connectorOwnerEndUser, OwnerID: alice, SlotKey: slotKey, ConnectorID: f.definition.ID, ConnectionID: connection.ID,
		CreatedAt: now, UpdatedAt: now}).Error; err != nil {
		t.Fatal(err)
	}
	if missing, err := preparePerUserConnectors(t.Context(), db, f.repo.connectorCatalog, run(alice)); err != nil || len(missing) != 0 {
		t.Fatalf("alice's slot after selecting again = %#v, %v", missing, err)
	}
	if selected, err := PerUserBindingSelected(t.Context(), db, perUserBindingID(slotRowID, alice), slotRowID, f.detail.Definition.ID, alice, slotKey, connection.ID); err != nil || !selected {
		t.Fatalf("alice's restored binding was refused: %v %v", selected, err)
	}
	if n := count("SELECT count(*) FROM connector_target_bindings WHERE owner_account_id = ? AND status = 'active'", alice); n != 1 {
		t.Fatalf("alice has %d active bindings after selecting again", n)
	}

	// A member clearing their selection loses the binding in the same
	// change, though their Connection stays.
	member := &RuntimeService{store: f.repo}
	if err := member.SelectMemberConnectorSlot(t.Context(), f.scope, f.detail.Definition.ID, slotKey, f.connections[0].ID); err != nil {
		t.Fatal(err)
	}
	actor := f.scope.ActorID
	console := &Run{ID: uuid.New(), OrganizationID: f.scope.OrganizationID, WorkspaceID: f.scope.WorkspaceID, DigitalHumanID: f.detail.Definition.ID,
		DigitalHumanVersionID: versionID, Surface: SurfaceConsole, ActorID: &actor}
	if missing, err := preparePerUserConnectors(t.Context(), db, f.repo.connectorCatalog, console); err != nil || len(missing) != 0 {
		t.Fatalf("the member's connected slot = %#v, %v", missing, err)
	}
	memberBinding := perUserBindingID(slotRowID, actor)
	if n := count("SELECT count(*) FROM connector_target_bindings WHERE id = ? AND status = 'active'", memberBinding); n != 1 {
		t.Fatal("the member's slot was not bound")
	}
	if err := member.ClearMemberConnectorSlot(t.Context(), f.scope, f.detail.Definition.ID, slotKey); err != nil {
		t.Fatal(err)
	}
	if n := count("SELECT count(*) FROM connector_target_bindings WHERE id = ? AND status = 'active'", memberBinding); n != 0 {
		t.Fatal("clearing the member's selection kept their binding active")
	}
	if n := count("SELECT count(*) FROM connector_connections WHERE id = ? AND deleted_at IS NULL", f.connections[0].ID); n != 1 {
		t.Fatal("clearing the selection removed the member's Connection")
	}
	if missing, err := preparePerUserConnectors(t.Context(), db, f.repo.connectorCatalog, console); err != nil || len(missing) != 1 {
		t.Fatalf("the member's cleared slot = %#v, %v", missing, err)
	}

	missing, err = preparePerUserConnectors(t.Context(), db, f.repo.connectorCatalog, run(bob))
	if err != nil || len(missing) != 1 || missing[0].SlotKey != slotKey || missing[0].State != PerUserSlotNeedsConnection || missing[0].Name != "Their own search" {
		t.Fatalf("bob's unconnected slot = %#v, %v", missing, err)
	}
	if note := perUserConnectorNote(missing); note == "" {
		t.Fatal("the model is not told bob must connect")
	}
	channel := run(alice)
	channel.Surface = SurfaceChannel
	if missing, err := preparePerUserConnectors(t.Context(), db, f.repo.connectorCatalog, channel); err != nil || missing != nil {
		t.Fatalf("a channel run resolved a per-user slot: %#v, %v", missing, err)
	}

	guard := func(name, query string, args ...any) {
		t.Helper()
		if err := db.Exec("SAVEPOINT guard").Error; err != nil {
			t.Fatal(err)
		}
		if err := db.Exec(query, args...).Error; err == nil {
			t.Fatalf("%s was accepted", name)
		}
		exec("ROLLBACK TO SAVEPOINT guard")
	}
	guard("a connection of nobody", "UPDATE connector_connections SET owner_account_id = ? WHERE id = ?", uuid.New(), connection.ID)
	guard("deleting an account with connections", "DELETE FROM accounts WHERE id = ?", f.scope.ActorID)

	if err := eraseEndUserConnections(db, alice, time.Now().UTC()); err != nil {
		t.Fatal(err)
	}
	if n := count("SELECT count(*) FROM connector_connections WHERE id = ? AND deleted_at IS NOT NULL AND encrypted_credentials = 'removed'", connection.ID); n != 1 {
		t.Fatal("erasure kept alice's credential")
	}
	if n := count("SELECT count(*) FROM connector_target_bindings WHERE owner_account_id = ? AND status <> 'disabled'", alice); n != 0 {
		t.Fatalf("erasure kept %d bindings of alice", n)
	}
	if n := count("SELECT count(*) FROM digital_human_connector_selections WHERE owner_id = ?", alice); n != 0 {
		t.Fatal("erasure kept alice's selection")
	}
	if n := count("SELECT count(*) FROM connector_connections WHERE owner_account_id = ? AND deleted_at IS NULL", f.scope.ActorID); n != 2 {
		t.Fatalf("erasure touched the editor's connections: %d", n)
	}
}
