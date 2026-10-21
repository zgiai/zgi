package preferences

import (
	"context"
	"database/sql/driver"
	"encoding/json"
	"errors"
	"fmt"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/google/uuid"
	connectorcatalog "github.com/zgiai/zgi/api/internal/modules/connectors/catalog"
	"github.com/zgiai/zgi/api/internal/modules/connectors/core"
	"github.com/zgiai/zgi/api/pkg/apperror"
	appcatalog "github.com/zgiai/zgi/api/pkg/apperror/catalog"
	apptransport "github.com/zgiai/zgi/api/pkg/apperror/transport"
	"gorm.io/datatypes"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

type testCatalog map[string]connectorcatalog.Definition

func (catalog testCatalog) Get(id string) (connectorcatalog.Definition, bool) {
	d, ok := catalog[id]
	return d, ok
}

func newPreferenceTestService(t *testing.T) (*Service, sqlmock.Sqlmock) {
	t.Helper()
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	gormDB, err := gorm.Open(postgres.New(postgres.Config{Conn: db}), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
	if err != nil {
		t.Fatal(err)
	}
	definitions, err := connectorcatalog.BuiltinDefinitions()
	if err != nil {
		t.Fatal(err)
	}
	definition := definitions[0]
	definition.Operations = definition.Operations[:1]
	definition.Operations[0].ID = "github.list_repositories"
	if err := connectorcatalog.Validate(definition); err != nil {
		t.Fatal(err)
	}
	service, err := NewService(gormDB, testCatalog{"github": definition})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := mock.ExpectationsWereMet(); err != nil {
			t.Error(err)
		}
	})
	return service, mock
}

func expectLock(mock sqlmock.Sqlmock) {
	mock.ExpectExec(`SELECT pg_advisory_xact_lock\(\$1\)`).WithArgs(sqlmock.AnyArg()).WillReturnResult(sqlmock.NewResult(0, 1))
}
func expectRecord(mock sqlmock.Sqlmock, org, owner uuid.UUID, revision int64, defaults string) {
	rows := sqlmock.NewRows([]string{"organization_id", "owner_account_id", "revision", "defaults"})
	if revision > 0 {
		rows.AddRow(org, owner, revision, defaults)
	}
	mock.ExpectQuery(`SELECT \* FROM "connector_chat_preferences" WHERE organization_id = \$1 AND owner_account_id = \$2 LIMIT \$3`).WithArgs(org, owner, 1).WillReturnRows(rows)
}
func expectNoReceipt(mock sqlmock.Sqlmock, command UpdateCommand) {
	mock.ExpectQuery(`SELECT \* FROM "connector_chat_preference_mutations" WHERE organization_id = \$1 AND owner_account_id = \$2 AND key_hash = \$3 LIMIT \$4`).WithArgs(command.OrganizationID, command.OwnerAccountID, hash(command.OrganizationID.String()+"/"+command.OwnerAccountID.String()+"/"+command.IdempotencyKey), 1).WillReturnRows(sqlmock.NewRows([]string{"fingerprint"}))
}
func testCommand() UpdateCommand {
	return UpdateCommand{OrganizationID: uuid.New(), OwnerAccountID: uuid.New(), ConnectorID: "github", Selection: Selection{Mode: ModeSelected, ConnectionID: uuid.NewString()}, IdempotencyKey: "test-preference-mutation-0001"}
}
func connectionRows(owner uuid.UUID, id string, state core.Readiness, operations string) *sqlmock.Rows {
	constraint, _ := json.Marshal(core.OwnerConstraint(owner, []core.Identifier{"github.list_repositories"}))
	return sqlmock.NewRows([]string{"id", "connector_id", "name", "external_display_name", "state", "driver_id", "auth_method_id", "usable_operation_ids", "unavailable_operations", "connection_constraint"}).AddRow(id, "github", "My GitHub", "octocat", state, "github-rest", "personal_access_token", operations, `{}`, string(constraint))
}
func expectConnectionLock(mock sqlmock.Sqlmock, command UpdateCommand, rows *sqlmock.Rows) {
	mock.ExpectQuery(regexp.QuoteMeta("SELECT "+summaryColumns+" FROM \"connector_connections\" WHERE organization_id = $1 AND owner_account_id = $2 AND id = $3 AND connector_id = $4 AND deleted_at IS NULL LIMIT $5 FOR UPDATE")).WithArgs(command.OrganizationID, command.OwnerAccountID, command.Selection.ConnectionID, command.ConnectorID, 1).WillReturnRows(rows)
}
func expectConnectionBatch(mock sqlmock.Sqlmock, org, owner uuid.UUID, ids []string, rows *sqlmock.Rows) {
	args := []driver.Value{org, owner}
	for _, id := range ids {
		args = append(args, id)
	}
	mock.ExpectQuery(regexp.QuoteMeta("SELECT "+summaryColumns+" FROM \"connector_connections\" WHERE organization_id = $1 AND owner_account_id = $2 AND id IN ") + `.* AND deleted_at IS NULL`).WithArgs(args...).WillReturnRows(rows)
}

func TestPreferenceValidationRejectsIdentityModeAndCredentialPayload(t *testing.T) {
	service, _ := newPreferenceTestService(t)
	for _, mutate := range []func(*UpdateCommand){
		func(c *UpdateCommand) { c.OrganizationID = uuid.Nil }, func(c *UpdateCommand) { c.OwnerAccountID = uuid.Nil },
		func(c *UpdateCommand) { c.ConnectorID = " GitHub " }, func(c *UpdateCommand) { c.Selection.Mode = "unset" },
		func(c *UpdateCommand) { c.Selection.ConnectionID = uuid.Nil.String() }, func(c *UpdateCommand) { c.Selection.ConnectionID = "not-a-uuid" },
		func(c *UpdateCommand) { c.Selection.Mode = ModeNone }, func(c *UpdateCommand) { c.ExpectedRevision = -1 },
		func(c *UpdateCommand) { c.IdempotencyKey = "short" }, func(c *UpdateCommand) { c.IdempotencyKey += "\n" },
	} {
		command := testCommand()
		mutate(&command)
		if _, err := service.Update(t.Context(), command); !apperror.IsCode(err, AppCodeInvalidInput) {
			t.Fatalf("invalid mutation: %v", err)
		}
	}
}

func TestPreferenceGetDoesNotGuessDefaultFromSavedConnections(t *testing.T) {
	service, mock := newPreferenceTestService(t)
	org, owner := uuid.New(), uuid.New()
	expectRecord(mock, org, owner, 0, "")
	snapshot, err := service.Get(t.Context(), org, owner)
	if err != nil || snapshot.Revision != 0 || snapshot.Defaults == nil || len(snapshot.Defaults) != 0 {
		t.Fatalf("new owner = %#v %v", snapshot, err)
	}
}

func TestPreferenceFirstSelectionCommitsBeforeReturningBatchedMetadata(t *testing.T) {
	service, mock := newPreferenceTestService(t)
	command := testCommand()
	mock.ExpectBegin()
	expectLock(mock)
	expectRecord(mock, command.OrganizationID, command.OwnerAccountID, 0, "")
	expectNoReceipt(mock, command)
	expectConnectionLock(mock, command, connectionRows(command.OwnerAccountID, command.Selection.ConnectionID, core.ReadinessReady, `["github.list_repositories"]`))
	mock.ExpectExec(`INSERT INTO "connector_chat_preferences"`).WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectExec(`INSERT INTO "connector_chat_preference_mutations"`).WillReturnResult(sqlmock.NewResult(0, 1))
	expectConnectionBatch(mock, command.OrganizationID, command.OwnerAccountID, []string{command.Selection.ConnectionID}, connectionRows(command.OwnerAccountID, command.Selection.ConnectionID, core.ReadinessReady, `["github.list_repositories"]`))
	mock.ExpectCommit()
	snapshot, err := service.Update(t.Context(), command)
	entry := snapshot.Defaults["github"]
	if err != nil || snapshot.Revision != 1 || entry.Connection == nil || entry.Available == nil || !*entry.Available {
		t.Fatalf("selected = %#v %v", snapshot, err)
	}
	data, _ := json.Marshal(snapshot)
	for _, forbidden := range []string{"credential", "secret", "token", "owner_account_id", "organization_id", "fingerprint", "key_hash"} {
		if strings.Contains(string(data), forbidden) {
			t.Fatalf("public response leaks %s", forbidden)
		}
	}
}

func TestPreferenceSelectionRejectsUnavailableOrForeignConnections(t *testing.T) {
	for _, test := range []struct {
		name              string
		state             core.Readiness
		ops               string
		missing, disabled bool
		code              apperror.Code
	}{
		{"foreign-or-deleted", core.ReadinessReady, `["github.list_repositories"]`, true, false, AppCodeConnectionNotFound},
		{"reconnect", core.ReadinessReconnectRequired, `["github.list_repositories"]`, false, false, AppCodeConnectionUnavailable},
		{"no-operations", core.ReadinessReady, `[]`, false, false, AppCodeConnectionUnavailable},
		{"unadmitted-operation", core.ReadinessReady, `["untrusted.run"]`, false, false, AppCodeConnectionUnavailable},
		{"plugin-disabled", core.ReadinessReady, `["github.list_repositories"]`, false, true, AppCodeConnectionUnavailable},
	} {
		t.Run(test.name, func(t *testing.T) {
			service, mock := newPreferenceTestService(t)
			command := testCommand()
			if test.disabled {
				service.catalog = testCatalog{}
			}
			mock.ExpectBegin()
			expectLock(mock)
			expectRecord(mock, command.OrganizationID, command.OwnerAccountID, 0, "")
			expectNoReceipt(mock, command)
			rows := connectionRows(command.OwnerAccountID, command.Selection.ConnectionID, test.state, test.ops)
			if test.missing {
				rows = sqlmock.NewRows([]string{"id"})
			}
			expectConnectionLock(mock, command, rows)
			mock.ExpectRollback()
			if _, err := service.Update(t.Context(), command); !apperror.IsCode(err, test.code) {
				t.Fatalf("selected error = %v", err)
			}
		})
	}
}

func TestPreferenceCASAndExplicitNoneAreIndependentOfBindings(t *testing.T) {
	service, mock := newPreferenceTestService(t)
	command := testCommand()
	command.Selection = Selection{Mode: ModeNone}
	mock.ExpectBegin()
	expectLock(mock)
	expectRecord(mock, command.OrganizationID, command.OwnerAccountID, 1, `{"github":{"mode":"none"}}`)
	expectNoReceipt(mock, command)
	mock.ExpectRollback()
	if _, err := service.Update(t.Context(), command); !apperror.IsCode(err, AppCodeRevisionConflict) {
		t.Fatalf("CAS = %v", err)
	}
	command.ExpectedRevision = 1
	mock.ExpectBegin()
	expectLock(mock)
	expectRecord(mock, command.OrganizationID, command.OwnerAccountID, 1, `{"web-search":{"mode":"none"}}`)
	expectNoReceipt(mock, command)
	mock.ExpectExec(`UPDATE "connector_chat_preferences" SET .* WHERE organization_id = \$[0-9]+ AND owner_account_id = \$[0-9]+ AND revision = \$[0-9]+`).WithArgs(jsonMapArgument(`{"github":{"mode":"none"},"web-search":{"mode":"none"}}`), int64(2), sqlmock.AnyArg(), command.OrganizationID, command.OwnerAccountID, int64(1)).WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectExec(`INSERT INTO "connector_chat_preference_mutations"`).WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectCommit()
	snapshot, err := service.Update(t.Context(), command)
	if err != nil || snapshot.Revision != 2 || len(snapshot.Defaults) != 2 || snapshot.Defaults["github"].Mode != ModeNone {
		t.Fatalf("explicit none = %#v %v", snapshot, err)
	}
}

type jsonMapArgument string

func (expected jsonMapArgument) Match(value driver.Value) bool {
	var raw []byte
	switch typed := value.(type) {
	case string:
		raw = []byte(typed)
	case []byte:
		raw = typed
	default:
		return false
	}
	var a, b any
	if json.Unmarshal(raw, &a) != nil || json.Unmarshal([]byte(expected), &b) != nil {
		return false
	}
	aJSON, _ := json.Marshal(a)
	bJSON, _ := json.Marshal(b)
	return string(aJSON) == string(bJSON)
}

func TestPreferenceReplayReturnsCurrentNoneWithoutRevalidatingDeletedKey(t *testing.T) {
	service, mock := newPreferenceTestService(t)
	command := testCommand()
	mock.ExpectBegin()
	expectLock(mock)
	expectRecord(mock, command.OrganizationID, command.OwnerAccountID, 2, `{"github":{"mode":"none"}}`)
	fingerprint := hash(command.ConnectorID + "/selected/" + command.Selection.ConnectionID + "/0")
	mock.ExpectQuery(`SELECT \* FROM "connector_chat_preference_mutations"`).WillReturnRows(sqlmock.NewRows([]string{"fingerprint", "result_revision"}).AddRow(fingerprint, 1))
	mock.ExpectCommit()
	snapshot, err := service.Update(t.Context(), command)
	if err != nil || snapshot.Revision != 2 || snapshot.Defaults["github"].Mode != ModeNone {
		t.Fatalf("replay resurrected selection: %#v %v", snapshot, err)
	}
	mock.ExpectBegin()
	expectLock(mock)
	expectRecord(mock, command.OrganizationID, command.OwnerAccountID, 2, `{"github":{"mode":"none"}}`)
	mock.ExpectQuery(`SELECT \* FROM "connector_chat_preference_mutations"`).WillReturnRows(sqlmock.NewRows([]string{"fingerprint"}).AddRow("different"))
	mock.ExpectRollback()
	if _, err := service.Update(t.Context(), command); !apperror.IsCode(err, AppCodeIdempotencyConflict) {
		t.Fatalf("key reuse = %v", err)
	}
}

func TestPreferenceBatchedReadRetainsUnavailableAndUsesLiveNames(t *testing.T) {
	service, mock := newPreferenceTestService(t)
	org, owner, id := uuid.New(), uuid.New(), uuid.NewString()
	for _, name := range []string{"Original name", "Renamed account"} {
		expectRecord(mock, org, owner, 4, fmt.Sprintf(`{"github":{"mode":"selected","connection_id":%q},"web-search":{"mode":"none"}}`, id))
		rows := sqlmock.NewRows([]string{"id", "connector_id", "name", "state", "driver_id", "auth_method_id", "usable_operation_ids"}).AddRow(id, "github", name, core.ReadinessReconnectRequired, "github-rest", "personal_access_token", `[]`)
		expectConnectionBatch(mock, org, owner, []string{id}, rows)
		snapshot, err := service.Get(t.Context(), org, owner)
		entry := snapshot.Defaults["github"]
		if err != nil || entry.Connection == nil || entry.Connection.Name != name || entry.Available == nil || *entry.Available || entry.ConnectionID != id {
			t.Fatalf("unavailable/rename = %#v %v", entry, err)
		}
	}
}

func TestPreferenceDeletionClearsOnlyExactIDAndRollsBackOnWriteFailure(t *testing.T) {
	service, mock := newPreferenceTestService(t)
	org, owner, id, other := uuid.New(), uuid.New(), uuid.New(), uuid.New()
	raw := fmt.Sprintf(`{"github":{"mode":"selected","connection_id":%q},"web-search":{"mode":"selected","connection_id":%q}}`, id.String(), other.String())
	want := jsonMapArgument(fmt.Sprintf(`{"github":{"mode":"none"},"web-search":{"mode":"selected","connection_id":%q}}`, other.String()))
	for _, fail := range []bool{false, true} {
		mock.ExpectBegin()
		expectLock(mock)
		expectRecord(mock, org, owner, 2, raw)
		update := mock.ExpectExec(`UPDATE "connector_chat_preferences"`).WithArgs(want, int64(3), sqlmock.AnyArg(), org, owner, int64(2))
		if fail {
			update.WillReturnError(errors.New("database private cause"))
			mock.ExpectRollback()
		} else {
			update.WillReturnResult(sqlmock.NewResult(0, 1))
			mock.ExpectCommit()
		}
		err := service.db.Transaction(func(tx *gorm.DB) error {
			if err := LockOwner(t.Context(), tx, org, owner); err != nil {
				return err
			}
			return ClearRemovedConnection(t.Context(), tx, org, owner, id)
		})
		if (err != nil) != fail {
			t.Fatalf("cleanup = %v", err)
		}
	}
}

func TestPreferenceBoundsAndCatalogProjection(t *testing.T) {
	defaults := map[string]Selection{}
	for i := 0; i < MaxDefaults+1; i++ {
		defaults[fmt.Sprintf("connector-%d", i)] = Selection{Mode: ModeNone}
	}
	raw, _ := json.Marshal(defaults)
	if _, err := decodeDefaults(raw); err == nil {
		t.Fatal("accepted oversized default map")
	}
	catalog, err := appcatalog.New(appcatalog.LocaleEnglishUS, appcatalog.CodeInternal, append(appcatalog.DefaultDefinitions(), CatalogDefinitions()...)...)
	if err != nil {
		t.Fatal(err)
	}
	projector, _ := apptransport.NewProjector(catalog)
	for _, definition := range CatalogDefinitions() {
		for _, locale := range []appcatalog.Locale{appcatalog.LocaleEnglishUS, appcatalog.LocaleChineseSimplified} {
			cause := errors.New("private-credential-and-database-cause")
			appErr := apperror.Wrap(cause, definition.Code)
			presentation := projector.Project(appErr, locale)
			if !errors.Is(appErr, cause) || presentation.Resolution != apptransport.ResolutionMatched || strings.Contains(presentation.Presentation.Message, "private-") || presentation.Presentation.Locale != locale || len(definition.Parameters) != 0 {
				t.Fatalf("unsafe/incomplete %s %s", definition.Code, locale)
			}
		}
	}
}

func TestPreferenceMutationLimitAndStorageFailureNeverBecomeEmptyDefaults(t *testing.T) {
	service, mock := newPreferenceTestService(t)
	command := testCommand()
	command.ExpectedRevision = 1
	command.Selection = Selection{Mode: ModeNone}
	defaults := map[string]Selection{}
	for i := 0; i < MaxDefaults; i++ {
		defaults[fmt.Sprintf("connector-%d", i)] = Selection{Mode: ModeNone}
	}
	raw, _ := json.Marshal(defaults)
	mock.ExpectBegin()
	expectLock(mock)
	expectRecord(mock, command.OrganizationID, command.OwnerAccountID, 1, string(raw))
	expectNoReceipt(mock, command)
	mock.ExpectRollback()
	if _, err := service.Update(t.Context(), command); !apperror.IsCode(err, AppCodeInvalidInput) {
		t.Fatalf("mutation bound: %v", err)
	}
	cause := errors.New("private database unavailable")
	mock.ExpectQuery(`SELECT \* FROM "connector_chat_preferences"`).WillReturnError(cause)
	snapshot, err := service.Get(t.Context(), command.OrganizationID, command.OwnerAccountID)
	if !errors.Is(err, cause) || !apperror.IsCode(err, AppCodeInternal) || snapshot.Defaults != nil {
		t.Fatalf("failed read became valid empty defaults: %#v %v", snapshot, err)
	}
	mock.ExpectBegin()
	expectLock(mock)
	expectRecord(mock, command.OrganizationID, command.OwnerAccountID, 1, `{}`)
	expectNoReceipt(mock, command)
	mock.ExpectExec(`UPDATE "connector_chat_preferences"`).WillReturnError(cause)
	mock.ExpectRollback()
	if _, err := service.Update(t.Context(), command); !errors.Is(err, cause) || !apperror.IsCode(err, AppCodeInternal) {
		t.Fatalf("failed write: %v", err)
	}
}

func TestPreferenceRevisionZeroStillLocksAbsentRowAndRejectsStaleSnapshot(t *testing.T) {
	service, mock := newPreferenceTestService(t)
	org, owner := uuid.New(), uuid.New()
	for _, revision := range []int64{0, 1} {
		mock.ExpectBegin()
		expectLock(mock)
		expectRecord(mock, org, owner, revision, `{}`)
		if revision == 0 {
			mock.ExpectCommit()
		} else {
			mock.ExpectRollback()
		}
		err := service.db.Transaction(func(tx *gorm.DB) error { return ValidateChatRevision(context.Background(), tx, org, owner, 0) })
		if revision == 0 && err != nil || revision == 1 && !apperror.IsCode(err, AppCodeRevisionConflict) {
			t.Fatalf("revision validation = %v", err)
		}
	}
}

func TestPreferenceReadinessUsesEffectiveNewChatConstraints(t *testing.T) {
	service, _ := newPreferenceTestService(t)
	owner := uuid.New()
	for _, test := range []struct {
		name      string
		mutate    func(*core.AccessConstraint, *summaryRecord)
		available bool
	}{
		{"ready wildcard", func(*core.AccessConstraint, *summaryRecord) {}, true},
		{"other audience", func(c *core.AccessConstraint, _ *summaryRecord) { c.AudienceOwner = uuid.New() }, false},
		{"denied", func(c *core.AccessConstraint, _ *summaryRecord) { c.DenyReasons = []string{"revoked"} }, false},
		{"expired", func(c *core.AccessConstraint, _ *summaryRecord) {
			expired := time.Now().Add(-time.Minute)
			c.ExpiresAt = &expired
		}, false},
		{"empty allowed operations", func(c *core.AccessConstraint, _ *summaryRecord) { c.AllowedOperationIDs = []core.Identifier{} }, false},
		{"another operation", func(c *core.AccessConstraint, _ *summaryRecord) {
			c.AllowedOperationIDs = []core.Identifier{"github.another"}
		}, false},
		{"agent only", func(c *core.AccessConstraint, _ *summaryRecord) {
			c.TargetBoundaries = []core.TargetBoundary{{Type: core.TargetTypeAgent, ID: "*", Scope: "*", VersionID: "*"}}
		}, false},
		{"existing chat only", func(c *core.AccessConstraint, _ *summaryRecord) {
			c.TargetBoundaries = []core.TargetBoundary{{Type: core.TargetTypeChat, ID: uuid.NewString(), Scope: "live", VersionID: "*"}}
		}, false},
		{"unavailable operation", func(_ *core.AccessConstraint, r *summaryRecord) {
			r.UnavailableOperations = datatypes.JSON([]byte(`{"github.list_repositories":"revoked"}`))
		}, false},
		{"invalid unavailable metadata", func(_ *core.AccessConstraint, r *summaryRecord) {
			r.UnavailableOperations = datatypes.JSON([]byte(`[]`))
		}, false},
		{"unsupported restriction schema", func(c *core.AccessConstraint, _ *summaryRecord) { c.SchemaVersion = 2 }, false},
	} {
		t.Run(test.name, func(t *testing.T) {
			constraint := core.OwnerConstraint(owner, []core.Identifier{"github.list_repositories"})
			record := summaryRecord{ID: uuid.New(), ConnectorID: "github", Name: "My account", State: core.ReadinessReady, DriverID: "github-rest", AuthMethodID: "personal_access_token", UsableOperationIDs: datatypes.JSON([]byte(`["github.list_repositories"]`)), UnavailableOperations: datatypes.JSON([]byte(`{}`))}
			test.mutate(constraint, &record)
			raw, _ := json.Marshal(constraint)
			record.ConnectionConstraint = datatypes.JSON(raw)
			summary, available := service.connectionSummary(record, owner)
			if available != test.available || summary.Name != "My account" {
				t.Fatalf("readiness %v want %v", available, test.available)
			}
		})
	}
}
