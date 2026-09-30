package bindings

import (
	"context"
	"database/sql/driver"
	"encoding/json"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/google/uuid"
	"github.com/zgiai/zgi/api/internal/modules/connectors/core"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

func TestUpsertBindingCreatesReadyOwnerScopedNativeBinding(t *testing.T) {
	repository, mock := openRepositoryMock(t)
	binding := nativeBinding(core.TargetTypeAgent, "agent-1", "")

	mock.ExpectBegin()
	expectReadyConnection(mock, binding)
	mock.ExpectExec(`INSERT INTO "connector_target_bindings".*ON CONFLICT DO NOTHING`).
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectCommit()

	stored, created, err := repository.UpsertBinding(
		context.Background(),
		binding.OrganizationID,
		binding.OwnerAccountID,
		binding,
		0,
	)
	if err != nil {
		t.Fatalf("UpsertBinding() error = %v", err)
	}
	if !created {
		t.Fatal("UpsertBinding() created = false, want true")
	}
	if stored.ID != binding.ID || stored.Revision != 1 || stored.Status != core.BindingStatusActive {
		t.Fatalf("UpsertBinding() = %#v", stored)
	}
	assertMock(t, mock)
}

func TestUpsertBindingReplayPreservesFirstUUID(t *testing.T) {
	repository, mock := openRepositoryMock(t)
	retry := nativeBinding(core.TargetTypeWorkflow, "workflow-1", "node-1")
	existing := retry
	existing.ID = uuid.New()

	mock.ExpectBegin()
	expectReadyConnection(mock, retry)
	mock.ExpectExec(`INSERT INTO "connector_target_bindings".*ON CONFLICT DO NOTHING`).
		WillReturnResult(sqlmock.NewResult(0, 0))
	expectLogicalBinding(mock, retry, bindingRows(existing))
	mock.ExpectCommit()

	stored, created, err := repository.UpsertBinding(
		context.Background(),
		retry.OrganizationID,
		retry.OwnerAccountID,
		retry,
		0,
	)
	if err != nil {
		t.Fatalf("UpsertBinding() error = %v", err)
	}
	if created {
		t.Fatal("UpsertBinding() created = true, want false")
	}
	if stored.ID != existing.ID || stored.ID == retry.ID {
		t.Fatalf("UpsertBinding() ID = %s, want stable first ID %s", stored.ID, existing.ID)
	}
	assertMock(t, mock)
}

func TestUpsertBindingReplacementUsesCASAndStableUUID(t *testing.T) {
	repository, mock := openRepositoryMock(t)
	desired := nativeBinding(core.TargetTypeAgent, "agent-1", "")
	desired.ConnectionID = uuid.New()
	desired.Revision = 4
	existing := desired
	existing.ID = uuid.New()
	existing.ConnectionID = uuid.New()
	existing.Revision = 4

	mock.ExpectBegin()
	expectReadyConnection(mock, desired)
	mock.ExpectExec(`INSERT INTO "connector_target_bindings".*ON CONFLICT DO NOTHING`).
		WillReturnResult(sqlmock.NewResult(0, 0))
	expectLogicalBinding(mock, desired, bindingRows(existing))
	mock.ExpectExec(`UPDATE "connector_target_bindings" SET .* WHERE organization_id = \$[0-9]+ AND owner_account_id = \$[0-9]+ AND id = \$[0-9]+ AND revision = \$[0-9]+`).
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectCommit()

	stored, created, err := repository.UpsertBinding(
		context.Background(),
		desired.OrganizationID,
		desired.OwnerAccountID,
		desired,
		4,
	)
	if err != nil {
		t.Fatalf("UpsertBinding() error = %v", err)
	}
	if created {
		t.Fatal("UpsertBinding() created = true, want false")
	}
	if stored.ID != existing.ID || stored.ConnectionID != desired.ConnectionID || stored.Revision != 5 {
		t.Fatalf("UpsertBinding() = %#v", stored)
	}
	assertMock(t, mock)
}

func TestUpsertBindingRejectsConnectionOutsideExactReadyOwnerScope(t *testing.T) {
	repository, mock := openRepositoryMock(t)
	binding := nativeBinding(core.TargetTypeChat, "chat-1", "")

	mock.ExpectBegin()
	mock.ExpectQuery(readyConnectionQuery()).
		WithArgs(
			binding.ConnectionID,
			binding.OrganizationID,
			binding.OwnerAccountID,
			binding.ConnectorID.String(),
			string(core.ReadinessReady),
			1,
		).
		WillReturnRows(sqlmock.NewRows([]string{"id"}))
	mock.ExpectRollback()

	_, _, err := repository.UpsertBinding(
		context.Background(),
		binding.OrganizationID,
		binding.OwnerAccountID,
		binding,
		0,
	)
	if !core.IsErrorCode(err, core.ErrorCodeConnectionNotReady) {
		t.Fatalf("UpsertBinding() error = %v, want %s", err, core.ErrorCodeConnectionNotReady)
	}
	assertMock(t, mock)
}

func TestGetBindingNeverResolvesByGlobalUUIDAlone(t *testing.T) {
	repository, mock := openRepositoryMock(t)
	binding := nativeBinding(core.TargetTypeAgent, "agent-1", "")

	mock.ExpectQuery(`SELECT \* FROM "connector_target_bindings" WHERE organization_id = \$1 AND owner_account_id = \$2 AND id = \$3.*LIMIT \$4`).
		WithArgs(binding.OrganizationID, binding.OwnerAccountID, binding.ID, 1).
		WillReturnRows(sqlmock.NewRows(bindingColumns()))

	_, err := repository.GetBinding(
		context.Background(), binding.OrganizationID, binding.OwnerAccountID, binding.ID,
	)
	if !core.IsErrorCode(err, core.ErrorCodeNotFound) {
		t.Fatalf("GetBinding() error = %v, want %s", err, core.ErrorCodeNotFound)
	}
	assertMock(t, mock)
}

func TestSetBindingStatusUsesRevisionCAS(t *testing.T) {
	repository, mock := openRepositoryMock(t)
	existing := nativeBinding(core.TargetTypeAgent, "agent-1", "")
	existing.Revision = 7

	mock.ExpectBegin()
	expectBindingByID(mock, existing, bindingRows(existing))
	mock.ExpectExec(`UPDATE "connector_target_bindings" SET .* WHERE organization_id = \$[0-9]+ AND owner_account_id = \$[0-9]+ AND id = \$[0-9]+ AND revision = \$[0-9]+`).
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectCommit()

	stored, err := repository.SetBindingStatus(
		context.Background(),
		existing.OrganizationID,
		existing.OwnerAccountID,
		existing.ID,
		core.BindingStatusNeedsReplacement,
		7,
	)
	if err != nil {
		t.Fatalf("SetBindingStatus() error = %v", err)
	}
	if stored.Status != core.BindingStatusNeedsReplacement || stored.Revision != 8 || stored.DisabledAt == nil {
		t.Fatalf("SetBindingStatus() = %#v", stored)
	}
	assertMock(t, mock)
}

func TestSetBindingStatusRejectsStaleRevisionAndLegacyMutation(t *testing.T) {
	t.Run("stale revision", func(t *testing.T) {
		repository, mock := openRepositoryMock(t)
		existing := nativeBinding(core.TargetTypeAgent, "agent-1", "")
		existing.Revision = 3

		mock.ExpectBegin()
		expectBindingByID(mock, existing, bindingRows(existing))
		mock.ExpectRollback()

		_, err := repository.SetBindingStatus(
			context.Background(), existing.OrganizationID, existing.OwnerAccountID, existing.ID, core.BindingStatusDisabled, 2,
		)
		if !core.IsErrorCode(err, core.ErrorCodeConflict) {
			t.Fatalf("SetBindingStatus() error = %v, want %s", err, core.ErrorCodeConflict)
		}
		assertMock(t, mock)
	})

}

func TestListBindingsSupportsTargetAndRemovalImpact(t *testing.T) {
	repository, mock := openRepositoryMock(t)
	first := nativeBinding(core.TargetTypeWorkflow, "workflow-1", "node-1")
	second := first
	second.ID = uuid.New()
	second.Target.BindingInstanceID = "node-2"

	mock.ExpectQuery(`SELECT \* FROM "connector_target_bindings" WHERE organization_id = \$1 AND owner_account_id = \$2 AND target_type = \$3 AND target_id = \$4 ORDER BY connector_id ASC, binding_instance_id ASC NULLS FIRST, created_at ASC, id ASC`).
		WithArgs(first.OrganizationID, first.OwnerAccountID, string(core.TargetTypeWorkflow), "workflow-1").
		WillReturnRows(bindingRows(first, second))

	bindings, err := repository.ListBindingsByTarget(
		context.Background(),
		first.OrganizationID,
		first.OwnerAccountID,
		core.TargetTypeWorkflow,
		"workflow-1",
		ListOptions{},
	)
	if err != nil {
		t.Fatalf("ListBindingsByTarget() error = %v", err)
	}
	if len(bindings) != 2 || bindings[0].Target.BindingInstanceID != "node-1" || bindings[1].Target.BindingInstanceID != "node-2" {
		t.Fatalf("ListBindingsByTarget() = %#v", bindings)
	}

	mock.ExpectQuery(`SELECT \* FROM "connector_target_bindings" WHERE \(organization_id = \$1 AND owner_account_id = \$2 AND connection_id = \$3\) AND status IN \(\$4\) ORDER BY target_type ASC, target_id ASC, binding_instance_id ASC NULLS FIRST, created_at ASC, id ASC`).
		WithArgs(first.OrganizationID, first.OwnerAccountID, first.ConnectionID, string(core.BindingStatusActive)).
		WillReturnRows(bindingRows(first))

	bindings, err = repository.ListBindingsByConnection(
		context.Background(),
		first.OrganizationID,
		first.OwnerAccountID,
		first.ConnectionID,
		ListOptions{Statuses: []core.BindingStatus{core.BindingStatusActive}},
	)
	if err != nil {
		t.Fatalf("ListBindingsByConnection() error = %v", err)
	}
	if len(bindings) != 1 || bindings[0].ID != first.ID {
		t.Fatalf("ListBindingsByConnection() = %#v", bindings)
	}
	assertMock(t, mock)
}

func TestListBindingsByConnectorSupportsDisableImpactAndOwnerScope(t *testing.T) {
	repository, mock := openRepositoryMock(t)
	binding := nativeBinding(core.TargetTypeAgent, "agent-1", "")

	mock.ExpectQuery(`SELECT \* FROM "connector_target_bindings" WHERE \(organization_id = \$1 AND owner_account_id = \$2 AND connector_id = \$3\) AND status IN \(\$4,\$5\) ORDER BY target_type ASC, target_id ASC, binding_instance_id ASC NULLS FIRST, created_at ASC, id ASC`).
		WithArgs(
			binding.OrganizationID,
			binding.OwnerAccountID,
			binding.ConnectorID.String(),
			string(core.BindingStatusActive),
			string(core.BindingStatusNeedsReplacement),
		).
		WillReturnRows(bindingRows(binding))

	bindings, err := repository.ListBindingsByConnector(
		context.Background(),
		binding.OrganizationID,
		binding.OwnerAccountID,
		binding.ConnectorID,
		ListOptions{Statuses: []core.BindingStatus{core.BindingStatusActive, core.BindingStatusNeedsReplacement}},
	)
	if err != nil {
		t.Fatalf("ListBindingsByConnector() error = %v", err)
	}
	if len(bindings) != 1 || bindings[0].ID != binding.ID {
		t.Fatalf("ListBindingsByConnector() = %#v", bindings)
	}
	assertMock(t, mock)
}

func openRepositoryMock(t *testing.T) (*GormRepository, sqlmock.Sqlmock) {
	t.Helper()
	sqlDB, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherRegexp))
	if err != nil {
		t.Fatalf("sqlmock.New() error = %v", err)
	}
	t.Cleanup(func() { _ = sqlDB.Close() })
	db, err := gorm.Open(postgres.New(postgres.Config{
		Conn:                 sqlDB,
		DriverName:           "postgres",
		PreferSimpleProtocol: true,
	}), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
	if err != nil {
		t.Fatalf("gorm.Open() error = %v", err)
	}
	return NewGormRepository(db), mock
}

func nativeBinding(targetType core.TargetType, targetID, bindingInstanceID string) core.TargetBinding {
	now := time.Date(2026, 9, 1, 8, 0, 0, 0, time.UTC)
	return core.TargetBinding{
		ID:             uuid.New(),
		OrganizationID: uuid.New(),
		OwnerAccountID: uuid.New(),
		ConnectionID:   uuid.New(),
		ConnectorID:    core.Identifier("github"),
		Target: core.TargetRef{
			Type:              targetType,
			ID:                targetID,
			BindingInstanceID: bindingInstanceID,
		},
		Status: core.BindingStatusActive,

		Revision:  1,
		CreatedAt: now,
		UpdatedAt: now,
	}
}

func readyConnectionQuery() string {
	return `SELECT "id" FROM "connector_connections" WHERE id = \$1 AND organization_id = \$2 AND owner_account_id = \$3 AND connector_id = \$4 AND state = \$5 AND deleted_at IS NULL.*LIMIT \$6`
}

func expectReadyConnection(mock sqlmock.Sqlmock, binding core.TargetBinding) {
	mock.ExpectQuery(readyConnectionQuery()).
		WithArgs(
			binding.ConnectionID,
			binding.OrganizationID,
			binding.OwnerAccountID,
			binding.ConnectorID.String(),
			string(core.ReadinessReady),
			1,
		).
		WillReturnRows(sqlmock.NewRows([]string{"id"}).AddRow(binding.ConnectionID))
}

func expectLogicalBinding(mock sqlmock.Sqlmock, binding core.TargetBinding, rows *sqlmock.Rows) {
	query := `SELECT \* FROM "connector_target_bindings" WHERE \(organization_id = \$1 AND owner_account_id = \$2 AND target_type = \$3 AND target_id = \$4 AND connector_id = \$5 AND target_scope = \$6 AND target_version_id = \$7\)`
	args := []driver.Value{
		binding.OrganizationID,
		binding.OwnerAccountID,
		string(binding.Target.Type),
		binding.Target.ID,
		binding.ConnectorID.String(), string(binding.Target.EffectiveScope()), binding.Target.VersionID,
	}
	if binding.Target.BindingInstanceID == "" {
		query += ` AND binding_instance_id IS NULL.*LIMIT \$8.*FOR UPDATE`
		args = append(args, 1)
	} else {
		query += ` AND binding_instance_id = \$8.*LIMIT \$9.*FOR UPDATE`
		args = append(args, binding.Target.BindingInstanceID, 1)
	}
	mock.ExpectQuery(query).WithArgs(args...).WillReturnRows(rows)
}

func expectBindingByID(mock sqlmock.Sqlmock, binding core.TargetBinding, rows *sqlmock.Rows) {
	mock.ExpectQuery(`SELECT \* FROM "connector_target_bindings" WHERE organization_id = \$1 AND owner_account_id = \$2 AND id = \$3.*LIMIT \$4.*FOR UPDATE`).
		WithArgs(binding.OrganizationID, binding.OwnerAccountID, binding.ID, 1).
		WillReturnRows(rows)
}

func bindingRows(bindings ...core.TargetBinding) *sqlmock.Rows {
	rows := sqlmock.NewRows(bindingColumns())
	for _, binding := range bindings {
		if binding.Constraint == nil {
			binding.Constraint = NewBindingConstraint(binding.OwnerAccountID, binding.ConnectorID)
		}
		var instanceID any
		if binding.Target.BindingInstanceID != "" {
			instanceID = binding.Target.BindingInstanceID
		}
		rows.AddRow(
			binding.ID,
			binding.OrganizationID,
			binding.OwnerAccountID,
			binding.ConnectionID,
			binding.ConnectorID.String(),
			string(binding.Target.Type),
			binding.Target.ID,
			instanceID,
			string(binding.Status),

			binding.Revision,
			binding.DisabledAt,
			binding.CreatedAt,
			binding.UpdatedAt, string(binding.Target.EffectiveScope()), binding.Target.VersionID, mustJSON(binding.Constraint),
		)
	}
	return rows
}

func bindingColumns() []string {
	return []string{
		"id",
		"organization_id",
		"owner_account_id",
		"connection_id",
		"connector_id",
		"target_type",
		"target_id",
		"binding_instance_id",
		"status",

		"revision",
		"disabled_at",
		"created_at",
		"updated_at", "target_scope", "target_version_id", "binding_constraint",
	}
}

func mustJSON(value any) []byte {
	if object, ok := value.(map[string]any); ok && len(object) == 0 {
		return []byte(`{}`)
	}
	data, err := json.Marshal(value)
	if err != nil {
		panic(err)
	}
	return data
}

func assertMock(t *testing.T, mock sqlmock.Sqlmock) {
	t.Helper()
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("sql expectations: %v", err)
	}
}

func TestBindingRecordTableName(t *testing.T) {
	if got := (bindingRecord{}).TableName(); got != "connector_target_bindings" {
		t.Fatalf("TableName() = %q", got)
	}
	if regexp.MustCompile(`(?i)(key|secret|credential|token)`).MatchString(strings.Join(bindingColumns(), ",")) {
		t.Fatalf("binding columns contain a secret-like field: %v", bindingColumns())
	}
}
