package catalog

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	connectoraudit "github.com/zgiai/zgi/api/internal/modules/connectors/audit"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

var testTime = time.Date(2026, time.September, 1, 12, 0, 0, 0, time.UTC)

func TestGormDefinitionRepositoryAdmitsFrozenDefinitionsAtomically(t *testing.T) {
	repository, mock := openDefinitionRepositoryMock(t)
	mock.ExpectBegin()
	mock.ExpectExec("SELECT pg_advisory_xact_lock").WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectQuery(`SELECT .* FROM "connector_definitions" WHERE plugin_id = \$1 ORDER BY connector_id ASC`).
		WithArgs("zgi.github").WillReturnRows(sqlmock.NewRows([]string{"connector_id"}))
	mock.ExpectQuery(`SELECT connector_id, plugin_id FROM "connector_definitions" WHERE connector_id IN`).
		WillReturnRows(sqlmock.NewRows([]string{"connector_id", "plugin_id"}))
	mock.ExpectExec(`UPDATE "connector_plugin_versions"`).WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectExec(`UPDATE "connector_definition_versions"`).WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectExec("INSERT INTO connector_plugin_versions").WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectExec("INSERT INTO connector_definition_versions").WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectExec("INSERT INTO connector_definitions").WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectExec("INSERT INTO connector_definition_versions").WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectExec("INSERT INTO connector_definitions").WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectExec(`(?s)INSERT INTO "connector_platform_lifecycle_events" .*ON CONFLICT DO NOTHING`).
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectCommit()

	definitions := []Definition{definitionRepositoryFixture("github"), definitionRepositoryFixture("web-search")}
	definitions[1].ProviderID = "web-search"
	definitions[1].DriverID = "exa"
	definitions[1].AuthMethodID = "account_api_key"
	definitions[1].Name = "Exa"
	definitions[1].Classification = ClassificationDataSource
	if err := repository.Admit(systemPluginAuditContext(), definitions); err != nil {
		t.Fatalf("Admit() error = %v", err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func TestGormDefinitionRepositoryRejectsIncompatibleStoredIdentity(t *testing.T) {
	repository, mock := openDefinitionRepositoryMock(t)
	mock.ExpectBegin()
	mock.ExpectExec("SELECT pg_advisory_xact_lock").WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectQuery(`SELECT .* FROM "connector_definitions" WHERE plugin_id = \$1 ORDER BY connector_id ASC`).
		WillReturnRows(sqlmock.NewRows([]string{"connector_id"}))
	mock.ExpectQuery(`SELECT connector_id, plugin_id FROM "connector_definitions" WHERE connector_id IN`).
		WillReturnRows(sqlmock.NewRows([]string{"connector_id", "plugin_id"}))
	mock.ExpectExec(`UPDATE "connector_plugin_versions"`).WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectExec(`UPDATE "connector_definition_versions"`).WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectExec("INSERT INTO connector_plugin_versions").WillReturnResult(sqlmock.NewResult(0, 0))
	mock.ExpectRollback()

	err := repository.Admit(context.Background(), []Definition{definitionRepositoryFixture("github")})
	if !errors.Is(err, ErrIncompatibleStoredDefinition) {
		t.Fatalf("Admit() error = %v, want incompatible definition", err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func TestGormDefinitionRepositoryLoadsDurableActiveAndDisabledRuntimeState(t *testing.T) {
	repository, mock := openDefinitionRepositoryMock(t)
	active := pluginContractDefinitionFixture()
	active.PluginVersion += "-digitalhuman.1"
	active.ExtensionSchemaVersion, active.ContractVersion = ZGIMetadataSchemaVersion13, 4
	active.Surfaces = append(active.Surfaces, SurfaceDigitalHuman)
	disabled := cloneDefinition(active)
	disabled.ID = "disabled-search"
	disabled.PluginID = "zgi.disabled-search"
	disabled.ProviderID = "disabled-search"
	disabled.DriverID = "disabled-search"
	disabled.PluginDigest = strings.Repeat("b", 64)

	mock.ExpectQuery(`SELECT .* FROM "connector_definitions" ORDER BY connector_id ASC`).
		WillReturnRows(storedDefinitionRows(t,
			storedDefinitionFixture{definition: active, status: PluginDefinitionStatusActive},
			storedDefinitionFixture{definition: disabled, status: PluginDefinitionStatusDisabled},
		))

	state, err := repository.LoadRuntimeState(context.Background())
	if err != nil {
		t.Fatalf("LoadRuntimeState() error = %v", err)
	}
	if len(state.Definitions) != 2 || len(state.ActiveDefinitions()) != 1 ||
		state.Statuses[active.PluginID] != PluginDefinitionStatusActive ||
		state.Statuses[disabled.PluginID] != PluginDefinitionStatusDisabled {
		t.Fatalf("runtime state = %#v", state)
	}
	if state.ActiveDefinitions()[0].ID != active.ID {
		t.Fatalf("active definitions = %#v", state.ActiveDefinitions())
	}
	loaded := state.ActiveDefinitions()[0]
	if !loaded.Supports(SurfaceDigitalHuman) || loaded.PluginVersion != active.PluginVersion {
		t.Fatalf("durable extension contract changed: %#v", loaded)
	}
	if loaded.NameI18n["zh-Hans"] != active.NameI18n["zh-Hans"] ||
		loaded.DescriptionI18n["zh-Hans"] != active.DescriptionI18n["zh-Hans"] ||
		loaded.KeyFields[0].LabelI18n["zh-Hans"] != active.KeyFields[0].LabelI18n["zh-Hans"] ||
		loaded.KeyFields[0].PlaceholderI18n["zh-Hans"] != active.KeyFields[0].PlaceholderI18n["zh-Hans"] ||
		loaded.Operations[0].NameI18n["zh-Hans"] != active.Operations[0].NameI18n["zh-Hans"] ||
		loaded.Operations[0].DescriptionI18n["zh-Hans"] != active.Operations[0].DescriptionI18n["zh-Hans"] {
		t.Fatalf("durable localized definition = %#v", loaded)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func TestGormDefinitionRepositoryListsAndVerifiesAcceptedVersions(t *testing.T) {
	repository, mock := openDefinitionRepositoryMock(t)
	definition := pluginContractDefinitionFixture()
	contractDigest, err := DefinitionContractDigest(definition)
	if err != nil {
		t.Fatal(err)
	}
	encoded, err := json.Marshal(definition)
	if err != nil {
		t.Fatal(err)
	}
	activatedAt := testTime.Add(time.Minute)
	retiredAt := testTime.Add(time.Hour)
	mock.ExpectQuery(`SELECT connector_id, plugin_id, plugin_version, plugin_digest, contract_digest, CAST\(definition AS text\) AS definition, status, accepted_at, activated_at, retired_at FROM "connector_definition_versions" WHERE plugin_id = \$1 ORDER BY accepted_at DESC, connector_id ASC`).
		WithArgs(definition.PluginID).
		WillReturnRows(sqlmock.NewRows([]string{
			"connector_id", "plugin_id", "plugin_version", "plugin_digest", "contract_digest",
			"definition", "status", "accepted_at", "activated_at", "retired_at",
		}).AddRow(
			definition.ID, definition.PluginID, definition.PluginVersion, definition.PluginDigest,
			contractDigest, string(encoded), PluginDefinitionStatusSuperseded, testTime, activatedAt, retiredAt,
		))

	versions, err := repository.ListAcceptedVersions(context.Background(), definition.PluginID)
	if err != nil {
		t.Fatalf("ListAcceptedVersions() error = %v", err)
	}
	if len(versions) != 1 || versions[0].Definition.PluginDigest != definition.PluginDigest ||
		versions[0].ContractDigest != contractDigest || versions[0].Status != PluginDefinitionStatusSuperseded ||
		versions[0].ActivatedAt == nil || versions[0].RetiredAt == nil {
		t.Fatalf("accepted versions = %#v", versions)
	}
	if versions[0].Definition.NameI18n["zh-Hans"] != definition.NameI18n["zh-Hans"] ||
		versions[0].Definition.Operations[0].DescriptionI18n["zh-Hans"] != definition.Operations[0].DescriptionI18n["zh-Hans"] {
		t.Fatalf("accepted localized version = %#v", versions[0].Definition)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func TestGormDefinitionRepositoryActivatesCompatibleUpdateAndRetainsPriorVersion(t *testing.T) {
	repository, mock := openDefinitionRepositoryMock(t)
	current := pluginContractDefinitionFixture()
	candidate := cloneDefinition(current)
	candidate.PluginVersion = "1.1.0"
	candidate.PluginDigest = strings.Repeat("b", 64)
	candidate.Name = "Updated display"
	candidate.Operations = append(candidate.Operations, additiveOperationFixture())

	mock.ExpectBegin()
	mock.ExpectExec("SELECT pg_advisory_xact_lock").WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectQuery(`SELECT .* FROM "connector_definitions" WHERE plugin_id = \$1 ORDER BY connector_id ASC`).
		WithArgs(current.PluginID).
		WillReturnRows(storedDefinitionRows(t, storedDefinitionFixture{
			definition: current, status: PluginDefinitionStatusActive,
		}))
	mock.ExpectQuery(`SELECT connector_id, plugin_id FROM "connector_definitions" WHERE connector_id IN`).
		WillReturnRows(sqlmock.NewRows([]string{"connector_id", "plugin_id"}).AddRow(current.ID, current.PluginID))
	mock.ExpectExec(`UPDATE "connector_plugin_versions"`).
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectExec(`UPDATE "connector_definition_versions"`).
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectExec("INSERT INTO connector_plugin_versions").WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectExec("INSERT INTO connector_definition_versions").WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectExec("INSERT INTO connector_definitions").WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectExec(`(?s)INSERT INTO "connector_platform_lifecycle_events" .*ON CONFLICT DO NOTHING`).
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectCommit()

	if err := repository.Admit(systemPluginAuditContext(), []Definition{candidate}); err != nil {
		t.Fatalf("Admit(compatible update) error = %v", err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func systemPluginAuditContext() context.Context {
	return connectoraudit.WithPlatformActor(context.Background(), connectoraudit.PlatformActor{
		Kind: connectoraudit.PlatformActorSystem,
	})
}

func TestGormDefinitionRepositoryRollsBackAdmissionWhenPlatformAuditFails(t *testing.T) {
	repository, mock := openDefinitionRepositoryMock(t)
	candidate := pluginContractDefinitionFixture()

	mock.ExpectBegin()
	mock.ExpectExec("SELECT pg_advisory_xact_lock").WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectQuery(`SELECT .* FROM "connector_definitions" WHERE plugin_id = \$1 ORDER BY connector_id ASC`).
		WithArgs(candidate.PluginID).
		WillReturnRows(sqlmock.NewRows(storedDefinitionColumnNames()))
	mock.ExpectQuery(`SELECT connector_id, plugin_id FROM "connector_definitions" WHERE connector_id IN`).
		WillReturnRows(sqlmock.NewRows([]string{"connector_id", "plugin_id"}))
	mock.ExpectExec(`UPDATE "connector_plugin_versions"`).WillReturnResult(sqlmock.NewResult(0, 0))
	mock.ExpectExec(`UPDATE "connector_definition_versions"`).WillReturnResult(sqlmock.NewResult(0, 0))
	mock.ExpectExec("INSERT INTO connector_plugin_versions").WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectExec("INSERT INTO connector_definition_versions").WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectExec("INSERT INTO connector_definitions").WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectExec(`(?s)INSERT INTO "connector_platform_lifecycle_events" .*ON CONFLICT DO NOTHING`).
		WillReturnError(errors.New("forced plugin admission audit failure"))
	mock.ExpectRollback()

	if err := repository.Admit(systemPluginAuditContext(), []Definition{candidate}); err == nil {
		t.Fatal("Admit() succeeded without its platform lifecycle audit")
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func TestGormDefinitionRepositoryRejectsIncompatibleUpdateBeforeAnyWrite(t *testing.T) {
	repository, mock := openDefinitionRepositoryMock(t)
	current := pluginContractDefinitionFixture()
	candidate := cloneDefinition(current)
	candidate.PluginVersion = "2.0.0"
	candidate.PluginDigest = strings.Repeat("b", 64)
	candidate.Operations[0].RiskHint = OperationRiskHintHigh

	mock.ExpectBegin()
	mock.ExpectExec("SELECT pg_advisory_xact_lock").WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectQuery(`SELECT .* FROM "connector_definitions" WHERE plugin_id = \$1 ORDER BY connector_id ASC`).
		WithArgs(current.PluginID).
		WillReturnRows(storedDefinitionRows(t, storedDefinitionFixture{
			definition: current, status: PluginDefinitionStatusActive,
		}))
	mock.ExpectRollback()

	err := repository.Admit(context.Background(), []Definition{candidate})
	if !errors.Is(err, ErrIncompatibleDefinitionUpdate) {
		t.Fatalf("Admit(incompatible update) error = %v", err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func TestGormDefinitionRepositoryRejectsConnectorRemovalAndIdentityCollisionBeforeWrite(t *testing.T) {
	t.Run("connector removal", func(t *testing.T) {
		repository, mock := openDefinitionRepositoryMock(t)
		current := pluginContractDefinitionFixture()
		second := cloneDefinition(current)
		second.ID = "second-connector"
		second.ProviderID = "second-provider"
		second.DriverID = "second-driver"
		second.AuthMethodID = "second-key"
		candidate := cloneDefinition(current)
		candidate.PluginVersion = "1.1.0"
		candidate.PluginDigest = strings.Repeat("b", 64)

		mock.ExpectBegin()
		mock.ExpectExec("SELECT pg_advisory_xact_lock").WillReturnResult(sqlmock.NewResult(0, 1))
		mock.ExpectQuery(`SELECT .* FROM "connector_definitions" WHERE plugin_id = \$1 ORDER BY connector_id ASC`).
			WithArgs(current.PluginID).
			WillReturnRows(storedDefinitionRows(t,
				storedDefinitionFixture{definition: current, status: PluginDefinitionStatusActive},
				storedDefinitionFixture{definition: second, status: PluginDefinitionStatusActive},
			))
		mock.ExpectRollback()

		err := repository.Admit(context.Background(), []Definition{candidate})
		if !errors.Is(err, ErrIncompatibleDefinitionUpdate) {
			t.Fatalf("Admit(removing Connector) error = %v", err)
		}
		if err := mock.ExpectationsWereMet(); err != nil {
			t.Fatal(err)
		}
	})

	t.Run("identity collision", func(t *testing.T) {
		repository, mock := openDefinitionRepositoryMock(t)
		candidate := pluginContractDefinitionFixture()
		mock.ExpectBegin()
		mock.ExpectExec("SELECT pg_advisory_xact_lock").WillReturnResult(sqlmock.NewResult(0, 1))
		mock.ExpectQuery(`SELECT .* FROM "connector_definitions" WHERE plugin_id = \$1 ORDER BY connector_id ASC`).
			WithArgs(candidate.PluginID).
			WillReturnRows(sqlmock.NewRows(storedDefinitionColumnNames()))
		mock.ExpectQuery(`SELECT connector_id, plugin_id FROM "connector_definitions" WHERE connector_id IN`).
			WillReturnRows(sqlmock.NewRows([]string{"connector_id", "plugin_id"}).
				AddRow(candidate.ID, "another.plugin"))
		mock.ExpectRollback()

		err := repository.Admit(context.Background(), []Definition{candidate})
		if !errors.Is(err, ErrDuplicateConnector) {
			t.Fatalf("Admit(identity collision) error = %v", err)
		}
		if err := mock.ExpectationsWereMet(); err != nil {
			t.Fatal(err)
		}
	})
}

func TestDefinitionContractDigestIgnoresDisplayCopyButFreezesRuntimeIdentity(t *testing.T) {
	definition := definitionRepositoryFixture("github")
	baseline, err := definitionAdmissionRecord(definition, testTime)
	if err != nil {
		t.Fatal(err)
	}
	copyOnly := definition
	copyOnly.Name = "GitHub Cloud"
	copyOnly.Description = "Updated member-facing copy"
	copyOnly.Icon = "updated-icon"
	copyRecord, err := definitionAdmissionRecord(copyOnly, testTime)
	if err != nil {
		t.Fatal(err)
	}
	if baseline.ContractDigest != copyRecord.ContractDigest {
		t.Fatal("display-only update changed the frozen runtime contract")
	}

	changed := definition
	changed.Operations = append([]Operation(nil), definition.Operations...)
	changed.Operations[0].ToolName = "github.changed_tool"
	changedRecord, err := definitionAdmissionRecord(changed, testTime)
	if err != nil {
		t.Fatal(err)
	}
	if baseline.ContractDigest == changedRecord.ContractDigest {
		t.Fatal("tool identity change did not change the frozen contract digest")
	}
	for _, serialized := range []string{baseline.Display, baseline.KeySchema, baseline.CapabilitySummary} {
		if strings.Contains(serialized, "candidate-key") {
			t.Fatalf("definition metadata contains candidate value: %s", serialized)
		}
	}
}

func openDefinitionRepositoryMock(t *testing.T) (*GormDefinitionRepository, sqlmock.Sqlmock) {
	t.Helper()
	sqlDB, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherRegexp))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = sqlDB.Close() })
	db, err := gorm.Open(postgres.New(postgres.Config{
		Conn: sqlDB, DriverName: "postgres", PreferSimpleProtocol: true,
	}), &gorm.Config{DisableAutomaticPing: true, Logger: logger.Default.LogMode(logger.Silent)})
	if err != nil {
		t.Fatal(err)
	}
	return NewGormDefinitionRepository(db), mock
}

func definitionRepositoryFixture(id string) Definition {
	return Definition{
		ID: id, PluginID: "zgi.github", PluginVersion: "1.0.0",
		PluginDigest:  strings.Repeat("a", 64),
		SourceDialect: SourceDialectAgentPluginsV1, SourceSchemaVersion: SourceSchemaVersion1,
		ProviderID: "github", DriverID: "github-rest", AuthMethodID: "personal_access_token",
		Name: "GitHub", Description: "GitHub repositories", Classification: ClassificationConnector,
		CapabilityTypes: []CapabilityType{CapabilityTypeAction}, SetupMethod: SetupMethodKey,
		Ownership: OwnershipPersonal, Surfaces: FirstReleaseSurfaces(),
		KeyFields:  []KeyField{{Name: "token", Label: "Token", Required: true, Secret: true}},
		Operations: []Operation{{ID: "github.list_repositories", ToolName: "github.list_repositories", Name: "List repositories"}},
	}
}

type storedDefinitionFixture struct {
	definition Definition
	status     PluginDefinitionStatus
}

func storedDefinitionColumnNames() []string {
	return []string{
		"connector_id", "plugin_id", "plugin_version", "plugin_digest", "source_dialect", "schema_version",
		"provider_id", "driver_id", "classification", "display", "key_schema", "capability_summary",
		"contract_digest", "status",
	}
}

func storedDefinitionRows(t *testing.T, fixtures ...storedDefinitionFixture) *sqlmock.Rows {
	t.Helper()
	rows := sqlmock.NewRows(storedDefinitionColumnNames())
	for _, fixture := range fixtures {
		record, err := definitionAdmissionRecord(fixture.definition, testTime)
		if err != nil {
			t.Fatal(err)
		}
		rows.AddRow(
			record.ConnectorID, record.PluginID, record.PluginVersion, record.PluginDigest,
			record.SourceDialect, record.SchemaVersion, record.ProviderID, record.DriverID,
			record.Classification, record.Display, record.KeySchema, record.CapabilitySummary,
			record.ContractDigest, fixture.status,
		)
	}
	return rows
}
