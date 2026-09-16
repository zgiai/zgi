package digitalhuman

import (
	"encoding/json"
	"os"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/zgiai/zgi/api/internal/capabilities/resourcebindings"
	connectoraudit "github.com/zgiai/zgi/api/internal/modules/connectors/audit"
	"github.com/zgiai/zgi/api/internal/modules/connectors/bindings"
	"github.com/zgiai/zgi/api/internal/modules/connectors/catalog"
	"github.com/zgiai/zgi/api/internal/modules/connectors/connections"
	"github.com/zgiai/zgi/api/internal/modules/connectors/core"
	"github.com/zgiai/zgi/api/pkg/apperror"
	"gorm.io/datatypes"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

// bindingInputFromRow and seedHistoricalBinding live in the companion
// resource-binding identity test file on the source branch. They are co-located
// here because only this file is in scope for the Connector host pick; move them
// back when the DTO identity suite is ported.
func bindingInputFromRow(t *testing.T, row *ResourceBinding) ResourceBindingInput {
	t.Helper()
	var input ResourceBindingInput
	raw, err := json.Marshal(map[string]any{
		"id": row.ID, "type": row.BindingType, "resource_id": row.ResourceID,
		"parent_resource_id": row.ParentResourceID, "display_name": row.DisplayName,
		"access_mode": row.AccessMode, "resource_revision": row.ResourceRevision,
		"credential_ref": row.CredentialRef, "metadata": json.RawMessage(row.Metadata),
	})
	if err != nil || json.Unmarshal(raw, &input) != nil {
		t.Fatal("encode binding input")
	}
	return input
}

func seedHistoricalBinding(t *testing.T, repo *GormRepository, scope Scope, detail *Detail, kind resourcebindings.BindingType) *ResourceBinding {
	t.Helper()
	owner, now := uuid.New(), time.Now().UTC().Truncate(time.Millisecond)
	if repo.db.Dialector.Name() == "postgres" {
		if err := repo.db.Exec("INSERT INTO accounts(id, name, email) VALUES (?, 'Historical owner', ?)", owner, owner.String()+"@example.invalid").Error; err != nil {
			t.Fatal(err)
		}
	}
	row := &ResourceBinding{
		ID: uuid.New(), OrganizationID: scope.OrganizationID, WorkspaceID: scope.WorkspaceID,
		DigitalHumanID: detail.Definition.ID, DigitalHumanVersionID: detail.Draft.ID,
		BindingType: kind, ResourceID: uuid.NewString(), ParentResourceID: "test.search",
		DisplayName: "Historical search", AccessMode: resourcebindings.AccessModeExecute,
		ResourceRevision: "connector:1", BindingRevision: "historical-binding-revision",
		Metadata:     datatypes.JSON(`{"allowed_operation_ids":["search"],"nested":{"label":"saved","limit":7}}`),
		AuthorizedBy: &owner, AuthorizedAt: &now, CreatedAt: now, UpdatedAt: now,
	}
	if err := repo.db.Create(row).Error; err != nil {
		t.Fatal(err)
	}
	return row
}

type digitalHumanConnectorFixture struct {
	service     *Service
	repo        *GormRepository
	scope       Scope
	detail      *Detail
	definition  catalog.Definition
	connections []core.PersonalConnection
}

func newDigitalHumanConnectorPostgresFixture(t *testing.T) digitalHumanConnectorFixture {
	t.Helper()
	dsn := strings.TrimSpace(os.Getenv("ZGI_MIGRATION_TEST_DSN"))
	if dsn == "" {
		t.Skip("set ZGI_MIGRATION_TEST_DSN to run native DigitalHuman resource transactions against migrated PostgreSQL")
	}
	db, err := gorm.Open(postgres.Open(dsn), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
	if err != nil {
		t.Fatal(err)
	}
	sqlDB, err := db.DB()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = sqlDB.Close() })
	tx := db.Begin()
	if tx.Error != nil {
		t.Fatal(tx.Error)
	}
	t.Cleanup(func() { _ = tx.Rollback().Error })
	scope := testScope()
	if err := tx.Exec("INSERT INTO accounts (id, name, email) VALUES (?, 'Connector editor', ?)", scope.ActorID, "connector-"+scope.ActorID.String()+"@example.invalid").Error; err != nil {
		t.Fatal(err)
	}
	if err := tx.Exec("INSERT INTO organizations (id, name) VALUES (?, 'Connector host tests')", scope.OrganizationID).Error; err != nil {
		t.Fatal(err)
	}
	if err := tx.Exec("INSERT INTO workspaces (id, organization_id, name) VALUES (?, ?, 'Connector host tests')", scope.WorkspaceID, scope.OrganizationID).Error; err != nil {
		t.Fatal(err)
	}
	definitions, err := catalog.BuiltinDefinitions()
	if err != nil {
		t.Fatal(err)
	}
	var definition catalog.Definition
	for _, item := range definitions {
		if len(item.Operations) >= 2 && item.SetupMethod == catalog.SetupMethodKey {
			definition = item
			break
		}
	}
	if definition.ID == "" {
		t.Fatal("test requires an admitted built-in with at least two operations")
	}
	definition, err = catalog.WithDigitalHumanSurface(definition, definition.PluginVersion+"-digitalhuman.1")
	if err != nil {
		t.Fatal(err)
	}
	manifest, err := catalog.BuildAgentPluginManifest([]catalog.Definition{definition})
	if err != nil {
		t.Fatal(err)
	}
	loader, err := catalog.NewPluginLoader()
	if err != nil {
		t.Fatal(err)
	}
	loaded, err := loader.LoadManifestPackage(t.Context(), manifest)
	if err != nil || len(loaded.Definitions) != 1 || len(loaded.Failures) != 0 {
		t.Fatalf("admit test package: %v, %#v", err, loaded.Failures)
	}
	definition = loaded.Definitions[0]
	registry, err := catalog.New(definition)
	if err != nil {
		t.Fatal(err)
	}
	admissionContext := connectoraudit.WithPlatformActor(t.Context(), connectoraudit.PlatformActor{Kind: connectoraudit.PlatformActorSystem})
	if err := catalog.NewGormDefinitionRepository(tx).Admit(admissionContext, []catalog.Definition{definition}); err != nil {
		t.Fatal(err)
	}
	repo := NewRepository(tx)
	repo.SetConnectorCatalog(registry)
	service := NewService(repo)
	fixture := digitalHumanConnectorFixture{service: service, repo: repo, scope: scope, definition: definition}
	fixture.detail = createPublishableDigitalHuman(t, service, scope, "Native connector host")
	operations := []core.Identifier{core.Identifier(definition.Operations[0].ID), core.Identifier(definition.Operations[1].ID)}
	for range 2 {
		now := time.Now().UTC()
		connection := core.PersonalConnection{ID: uuid.New(), OrganizationID: scope.OrganizationID, OwnerAccountID: scope.ActorID,
			ConnectorID: core.Identifier(definition.ID), ProviderID: core.Identifier(definition.ProviderID), DriverID: core.Identifier(definition.DriverID), AuthMethodID: core.Identifier(definition.AuthMethodID),
			Name: "Same connection name", EncryptedCredentials: "test-envelope-with-no-real-credentials", CredentialVersion: 1,
			State: core.ReadinessReady, Revision: 1, UsableOperationIDs: operations, UnavailableOperations: map[core.Identifier]string{},
			Constraint: core.OwnerConstraint(scope.ActorID, operations), LastVerifiedAt: now, CreatedAt: now, UpdatedAt: now}
		if err := connections.NewGormRepository(tx).CreateConnection(t.Context(), connection); err != nil {
			t.Fatal(err)
		}
		fixture.connections = append(fixture.connections, connection)
	}
	return fixture
}

func (f digitalHumanConnectorFixture) input(index int, operationCount int) ResourceBindingInput {
	operations := make([]string, operationCount)
	for i := range operations {
		operations[i] = f.definition.Operations[i].ID
	}
	return ResourceBindingInput{Type: resourcebindings.BindingTypeConnector, ResourceID: f.connections[index].ID.String(), ParentResourceID: f.definition.ID,
		DisplayName: f.connections[index].Name, AccessMode: resourcebindings.AccessModeExecute, ResourceRevision: "connector:1", Metadata: map[string]any{"allowed_operation_ids": operations}}
}

func (f digitalHumanConnectorFixture) canonical(t *testing.T, phase core.TargetScope, version uuid.UUID) []core.TargetBinding {
	t.Helper()
	versionID := ""
	if phase == core.ScopePublished {
		versionID = version.String()
	}
	items, err := bindings.NewGormRepository(f.repo.db).ListBindingsByTarget(t.Context(), f.scope.OrganizationID, f.scope.ActorID, core.TargetTypeDigitalHuman, f.detail.Definition.ID.String(), bindings.ListOptions{Scope: phase, VersionID: versionID})
	if err != nil {
		t.Fatal(err)
	}
	return items
}

func TestDigitalHumanConnectorPostgresSaveEditPublishRestore(t *testing.T) {
	f := newDigitalHumanConnectorPostgresFixture(t)
	items := []ResourceBindingInput{f.input(0, 1), f.input(1, 2), {Type: resourcebindings.BindingTypeSkill, ResourceID: "time", DisplayName: "Time", AccessMode: resourcebindings.AccessModeExecute, ResourceRevision: "time:1"}}
	saved, err := f.service.ReplaceDraftResourceBindings(t.Context(), f.scope, f.detail.Definition.ID, ReplaceResourceBindingsRequest{ExpectedDraftLockVersion: 1, Items: items})
	if err != nil {
		t.Fatal(err)
	}
	if saved.DraftVersionID != f.detail.Draft.ID {
		t.Fatal("saved bindings omitted current draft identity")
	}
	native := f.canonical(t, core.ScopeDraft, uuid.Nil)
	if len(native) != 2 || native[0].Target.BindingInstanceID == native[1].Target.BindingInstanceID {
		t.Fatalf("two instances were not retained: %#v", native)
	}
	row := saved.Items[0]
	var canonicalBefore core.TargetBinding
	for _, candidate := range native {
		if candidate.Target.BindingInstanceID == row.ID.String() {
			canonicalBefore = candidate
		}
	}
	inputs := make([]ResourceBindingInput, len(saved.Items))
	for i, item := range saved.Items {
		inputs[i] = bindingInputFromRow(t, item)
	}
	inputs[0].Metadata["allowed_operation_ids"] = []string{f.definition.Operations[0].ID, f.definition.Operations[1].ID}
	inputs[0].DisplayName = "Explicitly expanded search"
	edited, err := f.service.ReplaceDraftResourceBindings(t.Context(), f.scope, f.detail.Definition.ID, ReplaceResourceBindingsRequest{ExpectedDraftLockVersion: saved.DraftLockVersion, Items: inputs})
	if err != nil {
		t.Fatal(err)
	}
	if edited.Items[0].ID != row.ID || edited.Items[0].BindingRevision == row.BindingRevision {
		t.Fatalf("edit changed identity or missed revision: %#v", edited.Items[0])
	}
	for _, candidate := range f.canonical(t, core.ScopeDraft, uuid.Nil) {
		if candidate.Target.BindingInstanceID == row.ID.String() && (candidate.ID != canonicalBefore.ID || candidate.Revision != canonicalBefore.Revision+1 || len(candidate.Constraint.AllowedOperationIDs) != 2) {
			t.Fatalf("explicit operation edit was lost: %#v", candidate)
		}
	}
	if _, err := f.service.ReplaceDraftResourceBindings(t.Context(), f.scope, f.detail.Definition.ID, ReplaceResourceBindingsRequest{ExpectedDraftLockVersion: saved.DraftLockVersion, Items: inputs}); !apperror.IsCode(err, AppCodeRevisionConflict) {
		t.Fatalf("stale lock = %v", err)
	}
	for i, item := range edited.Items {
		inputs[i] = bindingInputFromRow(t, item)
	}
	inputs[0].DisplayName = "Second explicit edit"
	editedAgain, err := f.service.ReplaceDraftResourceBindings(t.Context(), f.scope, f.detail.Definition.ID, ReplaceResourceBindingsRequest{ExpectedDraftLockVersion: edited.DraftLockVersion, Items: inputs})
	if err != nil || editedAgain.Items[0].ID != row.ID {
		t.Fatalf("second edit changed stable instance: %#v, %v", editedAgain, err)
	}
	edited = editedAgain
	for _, candidate := range f.canonical(t, core.ScopeDraft, uuid.Nil) {
		if candidate.ID == canonicalBefore.ID && candidate.Revision != canonicalBefore.Revision+2 {
			t.Fatalf("second edit did not advance canonical revision: %#v", candidate)
		}
	}
	published, err := f.service.Publish(t.Context(), f.scope, f.detail.Definition.ID, PublishRequest{ExpectedRevision: f.detail.Definition.Revision, ExpectedDraftLockVersion: edited.DraftLockVersion})
	if err != nil {
		t.Fatal(err)
	}
	publishedBindings := f.canonical(t, core.ScopePublished, published.Published.ID)
	if len(publishedBindings) != 2 {
		t.Fatalf("published references: %#v", publishedBindings)
	}
	for _, binding := range publishedBindings {
		if binding.Target.VersionID != published.Published.ID.String() || binding.OwnerAccountID != f.scope.ActorID {
			t.Fatal("published authority changed owner/version")
		}
	}
	if err := f.repo.db.Transaction(func(tx *gorm.DB) error {
		return tx.Model(&ResourceBinding{}).Where("id = ?", row.ID).Update("display_name", "mutated publication").Error
	}); err == nil {
		t.Fatal("published binding was mutable")
	}
	cleared, err := f.service.ReplaceDraftResourceBindings(t.Context(), f.scope, f.detail.Definition.ID, ReplaceResourceBindingsRequest{ExpectedDraftLockVersion: published.Draft.LockVersion})
	if err != nil {
		t.Fatal(err)
	}
	preview, err := f.service.PreviewVersionRestore(t.Context(), f.scope, f.detail.Definition.ID, published.Published.ID)
	if err != nil {
		t.Fatal(err)
	}
	restored, err := f.service.RestoreVersion(t.Context(), f.scope, f.detail.Definition.ID, RestoreVersionRequest{VersionID: published.Published.ID, ExpectedRevision: published.Definition.Revision, ExpectedDraftLockVersion: cleared.DraftLockVersion, ImpactToken: preview.ImpactToken})
	if err != nil {
		t.Fatal(err)
	}
	current, err := f.service.ListDraftResourceBindings(t.Context(), f.scope, f.detail.Definition.ID)
	if err != nil || len(current.Items) != 3 || current.DraftLockVersion != restored.Draft.LockVersion {
		t.Fatalf("restore lost resources: %#v, %v", current, err)
	}
	if current.DraftVersionID != restored.Draft.ID || current.DraftVersionID == saved.DraftVersionID {
		t.Fatal("publication did not change draft response identity")
	}
	for _, item := range current.Items {
		if item.BindingType == resourcebindings.BindingTypeConnector && item.ID == row.ID {
			t.Fatal("restore reused immutable resource instance")
		}
	}
	if got := f.canonical(t, core.ScopePublished, published.Published.ID); !reflect.DeepEqual(got, publishedBindings) {
		t.Fatal("draft lifecycle changed published references")
	}
}

func TestDigitalHumanConnectorPostgresRetentionOwnerAndRollback(t *testing.T) {
	f := newDigitalHumanConnectorPostgresFixture(t)
	saved, err := f.service.ReplaceDraftResourceBindings(t.Context(), f.scope, f.detail.Definition.ID, ReplaceResourceBindingsRequest{ExpectedDraftLockVersion: 1, Items: []ResourceBindingInput{f.input(0, 1)}})
	if err != nil {
		t.Fatal(err)
	}
	before := f.canonical(t, core.ScopeDraft, uuid.Nil)
	// A user-selected operation edit must not erase another server restriction.
	restricted := *before[0].Constraint
	restricted.DenyReasons = []string{"server_restriction"}
	restrictedJSON, err := json.Marshal(restricted)
	if err != nil {
		t.Fatal(err)
	}
	if err := f.repo.db.Exec("UPDATE connector_target_bindings SET binding_constraint = ?::jsonb WHERE id = ?", string(restrictedJSON), before[0].ID).Error; err != nil {
		t.Fatal(err)
	}
	blocked := bindingInputFromRow(t, saved.Items[0])
	blocked.DisplayName = "Cannot clear the server restriction"
	if _, err := f.service.ReplaceDraftResourceBindings(t.Context(), f.scope, f.detail.Definition.ID, ReplaceResourceBindingsRequest{ExpectedDraftLockVersion: saved.DraftLockVersion, Items: []ResourceBindingInput{blocked}}); !apperror.IsCode(err, AppCodeResourceBindingInvalid) {
		t.Fatalf("explicit edit erased a server restriction: %v", err)
	}
	originalJSON, err := json.Marshal(before[0].Constraint)
	if err != nil {
		t.Fatal(err)
	}
	if err := f.repo.db.Exec("UPDATE connector_target_bindings SET binding_constraint = ?::jsonb WHERE id = ?", string(originalJSON), before[0].ID).Error; err != nil {
		t.Fatal(err)
	}
	otherScope := f.scope
	otherScope.ActorID = uuid.New()
	if err := f.repo.db.Exec("INSERT INTO accounts(id, name, email) VALUES (?, 'Other editor', ?)", otherScope.ActorID, otherScope.ActorID.String()+"@example.invalid").Error; err != nil {
		t.Fatal(err)
	}
	input := bindingInputFromRow(t, saved.Items[0])
	input.DisplayName = "Attempted ownership transfer"
	if _, err := f.service.ReplaceDraftResourceBindings(t.Context(), otherScope, f.detail.Definition.ID, ReplaceResourceBindingsRequest{ExpectedDraftLockVersion: saved.DraftLockVersion, Items: []ResourceBindingInput{input}}); !apperror.IsCode(err, AppCodeResourceBindingInvalid) {
		t.Fatalf("other editor changed private authorization: %v", err)
	}
	input = bindingInputFromRow(t, saved.Items[0])
	if err := f.repo.db.Exec("UPDATE connector_connections SET state = 'unavailable' WHERE id = ?", f.connections[0].ID).Error; err != nil {
		t.Fatal(err)
	}
	kept, err := f.service.ReplaceDraftResourceBindings(t.Context(), otherScope, f.detail.Definition.ID, ReplaceResourceBindingsRequest{ExpectedDraftLockVersion: saved.DraftLockVersion, Items: []ResourceBindingInput{input, {Type: resourcebindings.BindingTypeSkill, ResourceID: "time", AccessMode: resourcebindings.AccessModeExecute, ResourceRevision: "time:1"}}})
	if err != nil {
		t.Fatal(err)
	}
	if got := f.canonical(t, core.ScopeDraft, uuid.Nil); !reflect.DeepEqual(before, got) || *kept.Items[0].AuthorizedBy != f.scope.ActorID {
		t.Fatal("keep-only rewrote native authority")
	}
	if _, err := f.service.Publish(t.Context(), f.scope, f.detail.Definition.ID, PublishRequest{ExpectedRevision: f.detail.Definition.Revision, ExpectedDraftLockVersion: kept.DraftLockVersion}); !apperror.IsCode(err, AppCodeResourceBindingInvalid) {
		t.Fatalf("unavailable publish: %v", err)
	}
	// Omit the old row (which disables its reference), then fail a new selection.
	// The complete transaction must roll that detachment back.
	invalid := f.input(1, 1)
	invalid.Metadata["allowed_operation_ids"] = []string{"unknown.operation"}
	if _, err := f.service.ReplaceDraftResourceBindings(t.Context(), f.scope, f.detail.Definition.ID, ReplaceResourceBindingsRequest{ExpectedDraftLockVersion: kept.DraftLockVersion, Items: []ResourceBindingInput{invalid}}); !apperror.IsCode(err, AppCodeResourceBindingInvalid) {
		t.Fatalf("invalid selection = %v", err)
	}
	current, err := f.service.ListDraftResourceBindings(t.Context(), f.scope, f.detail.Definition.ID)
	if err != nil || current.DraftLockVersion != kept.DraftLockVersion || len(current.Items) != 2 || !reflect.DeepEqual(before, f.canonical(t, core.ScopeDraft, uuid.Nil)) {
		t.Fatalf("failed save did not roll back: %#v, %v", current, err)
	}
	if _, err := f.service.ReplaceDraftResourceBindings(t.Context(), f.scope, f.detail.Definition.ID, ReplaceResourceBindingsRequest{ExpectedDraftLockVersion: kept.DraftLockVersion, Items: []ResourceBindingInput{f.input(1, 1)}}); err != nil {
		t.Fatal(err)
	}
	for _, binding := range f.canonical(t, core.ScopeDraft, uuid.Nil) {
		if binding.ConnectionID == f.connections[0].ID && binding.Status != core.BindingStatusDisabled {
			t.Fatal("replaced reference remained active")
		}
	}
}

func TestDigitalHumanConnectorPostgresLegacyAndRestoreRevalidation(t *testing.T) {
	f := newDigitalHumanConnectorPostgresFixture(t)
	legacy := seedHistoricalBinding(t, f.repo, f.scope, f.detail, resourcebindings.BindingTypeIntegrationConnection)
	legacyInput := bindingInputFromRow(t, legacy)
	kept, err := f.service.ReplaceDraftResourceBindings(t.Context(), f.scope, f.detail.Definition.ID, ReplaceResourceBindingsRequest{ExpectedDraftLockVersion: 1, Items: []ResourceBindingInput{legacyInput, f.input(0, 1)}})
	if err != nil || kept.Items[0].ID != legacy.ID || *kept.Items[0].AuthorizedBy != *legacy.AuthorizedBy {
		t.Fatalf("legacy preservation: %#v, %v", kept, err)
	}
	for _, mutate := range []func(*ResourceBindingInput){func(input *ResourceBindingInput) { input.ID = nil }, func(input *ResourceBindingInput) { input.DisplayName = "Changed legacy" }} {
		input := bindingInputFromRow(t, legacy)
		mutate(&input)
		if _, err := f.service.ReplaceDraftResourceBindings(t.Context(), f.scope, f.detail.Definition.ID, ReplaceResourceBindingsRequest{ExpectedDraftLockVersion: kept.DraftLockVersion, Items: []ResourceBindingInput{input}}); !apperror.IsCode(err, AppCodeResourceBindingInvalid) {
			t.Fatalf("legacy write was accepted: %v", err)
		}
	}
	if _, err := f.service.Publish(t.Context(), f.scope, f.detail.Definition.ID, PublishRequest{ExpectedRevision: f.detail.Definition.Revision, ExpectedDraftLockVersion: kept.DraftLockVersion}); !apperror.IsCode(err, AppCodeResourceBindingInvalid) {
		t.Fatalf("legacy publication was accepted: %v", err)
	}
	// Replacing the historical row permits publication of independently authorized resources.
	cleaned, err := f.service.ReplaceDraftResourceBindings(t.Context(), f.scope, f.detail.Definition.ID, ReplaceResourceBindingsRequest{ExpectedDraftLockVersion: kept.DraftLockVersion, Items: []ResourceBindingInput{bindingInputFromRow(t, kept.Items[1])}})
	if err != nil {
		t.Fatal(err)
	}
	published, err := f.service.Publish(t.Context(), f.scope, f.detail.Definition.ID, PublishRequest{ExpectedRevision: f.detail.Definition.Revision, ExpectedDraftLockVersion: cleaned.DraftLockVersion})
	if err != nil {
		t.Fatal(err)
	}
	if err := f.repo.db.Exec("UPDATE connector_connections SET state = 'reconnect_required' WHERE id = ?", f.connections[0].ID).Error; err != nil {
		t.Fatal(err)
	}
	preview, err := f.service.PreviewVersionRestore(t.Context(), f.scope, f.detail.Definition.ID, published.Published.ID)
	if err != nil {
		t.Fatal(err)
	}
	before, err := f.service.ListDraftResourceBindings(t.Context(), f.scope, f.detail.Definition.ID)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := f.service.RestoreVersion(t.Context(), f.scope, f.detail.Definition.ID, RestoreVersionRequest{VersionID: published.Published.ID, ExpectedRevision: published.Definition.Revision, ExpectedDraftLockVersion: published.Draft.LockVersion, ImpactToken: preview.ImpactToken}); !apperror.IsCode(err, AppCodeResourceBindingInvalid) {
		t.Fatalf("restore resurrected unavailable connection: %v", err)
	}
	after, err := f.service.ListDraftResourceBindings(t.Context(), f.scope, f.detail.Definition.ID)
	if err != nil || !reflect.DeepEqual(before, after) {
		t.Fatalf("failed restore mutated draft: %v", err)
	}
}

func TestDigitalHumanConnectorPostgresHistoricalLegacyRestoreIsReadOnly(t *testing.T) {
	f := newDigitalHumanConnectorPostgresFixture(t)
	legacy := seedHistoricalBinding(t, f.repo, f.scope, f.detail, resourcebindings.BindingTypeIntegrationConnection)
	// Seed a publication produced before retirement, without using the new writer.
	if err := f.repo.db.Exec("UPDATE digital_human_versions SET state = 'published', version_number = 1, published_at = CURRENT_TIMESTAMP WHERE id = ?", f.detail.Draft.ID).Error; err != nil {
		t.Fatal(err)
	}
	next := *f.detail.Draft
	next.ID = uuid.New()
	if err := f.repo.CreateVersion(t.Context(), &next); err != nil {
		t.Fatal(err)
	}
	if err := f.repo.db.Model(&Definition{}).Where("id = ?", f.detail.Definition.ID).Updates(map[string]any{"draft_version_id": next.ID, "published_version_id": f.detail.Draft.ID}).Error; err != nil {
		t.Fatal(err)
	}
	preview, err := f.service.PreviewVersionRestore(t.Context(), f.scope, f.detail.Definition.ID, f.detail.Draft.ID)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := f.service.RestoreVersion(t.Context(), f.scope, f.detail.Definition.ID, RestoreVersionRequest{VersionID: f.detail.Draft.ID, ExpectedRevision: f.detail.Definition.Revision, ExpectedDraftLockVersion: next.LockVersion, ImpactToken: preview.ImpactToken}); !apperror.IsCode(err, AppCodeResourceBindingInvalid) {
		t.Fatalf("restored a new legacy instance: %v", err)
	}
	current, err := f.service.ListDraftResourceBindings(t.Context(), f.scope, f.detail.Definition.ID)
	if err != nil || len(current.Items) != 0 || current.DraftVersionID != next.ID || current.DraftLockVersion != next.LockVersion {
		t.Fatalf("legacy restore changed the current draft: %#v, %v", current, err)
	}
	history, err := f.repo.ListResourceBindingRows(t.Context(), f.scope, f.detail.Definition.ID, f.detail.Draft.ID)
	if err != nil || len(history) != 1 || history[0].ID != legacy.ID || *history[0].AuthorizedBy != *legacy.AuthorizedBy {
		t.Fatalf("historical read changed: %#v, %v", history, err)
	}
}

func TestDigitalHumanConnectorPostgresRejectsPreviousDraftGeneration(t *testing.T) {
	f := newDigitalHumanConnectorPostgresFixture(t)
	published, err := f.service.Publish(t.Context(), f.scope, f.detail.Definition.ID, PublishRequest{ExpectedRevision: f.detail.Definition.Revision, ExpectedDraftLockVersion: 1})
	if err != nil {
		t.Fatal(err)
	}
	if published.Draft.LockVersion != 1 || published.Draft.ID == f.detail.Draft.ID {
		t.Fatal("test did not create a fresh draft generation")
	}
	for _, items := range [][]ResourceBindingInput{nil, {f.input(0, 1)}} {
		if _, err := f.service.ReplaceDraftResourceBindings(t.Context(), f.scope, f.detail.Definition.ID, ReplaceResourceBindingsRequest{ExpectedDraftVersionID: &f.detail.Draft.ID, ExpectedDraftLockVersion: 1, Items: items}); !apperror.IsCode(err, AppCodeRevisionConflict) {
			t.Fatalf("old generation was accepted: %v", err)
		}
	}
	current, err := f.service.ListDraftResourceBindings(t.Context(), f.scope, f.detail.Definition.ID)
	if err != nil || current.DraftVersionID != published.Draft.ID || current.DraftLockVersion != 1 || len(current.Items) != 0 || len(f.canonical(t, core.ScopeDraft, uuid.Nil)) != 0 {
		t.Fatalf("stale generation changed new draft: %#v, %v", current, err)
	}
}
