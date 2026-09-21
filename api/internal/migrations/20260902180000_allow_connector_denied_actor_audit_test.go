package migrations

import (
	"context"
	"database/sql"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/lib/pq"
)

func TestConnectorDeniedActorAuditMigrationPreservesTruthAndOwnerSubmission(t *testing.T) {
	if !strings.Contains(allowConnectorDeniedActorAuditSQL, "CHECK (acting_principal_id = owner_account_id OR status = 'denied')") {
		t.Fatal("only denials may record a non-owner actor")
	}
	if strings.Contains(allowConnectorDeniedActorAuditSQL, "connector_operation_outcomes") {
		t.Fatal("operation outcome owner constraint must not be weakened")
	}
	if !strings.Contains(restoreConnectorDeniedActorAuditSQL, "RAISE EXCEPTION") ||
		!strings.Contains(restoreConnectorDeniedActorAuditSQL, "WHERE acting_principal_id <> owner_account_id") ||
		strings.Contains(strings.ToUpper(restoreConnectorDeniedActorAuditSQL), "DELETE FROM") {
		t.Fatal("downgrade must preserve denied audit records and fail closed")
	}
}

func TestConnectorDeniedActorAuditPostgres(t *testing.T) {
	dsn := os.Getenv("ZGI_CONNECTOR_REVERSAL_TEST_DSN")
	if dsn == "" {
		t.Skip("set ZGI_CONNECTOR_REVERSAL_TEST_DSN for isolated PostgreSQL audit constraint tests")
	}
	db, err := sql.Open("postgres", dsn)
	if err != nil {
		t.Fatal("invalid PostgreSQL test configuration")
	}
	defer db.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	conn, err := db.Conn(ctx)
	if err != nil {
		t.Fatal("connect to PostgreSQL test database")
	}
	defer conn.Close()
	schema := pq.QuoteIdentifier("connector_denied_audit_" + strings.ReplaceAll(uuid.NewString(), "-", ""))
	if _, err := conn.ExecContext(ctx, "CREATE SCHEMA "+schema); err != nil {
		t.Fatal(err)
	}
	defer func() {
		cleanupCtx, cleanupCancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cleanupCancel()
		if _, err := conn.ExecContext(cleanupCtx, "DROP SCHEMA "+schema+" CASCADE"); err != nil {
			t.Error("remove owned audit test schema:", err)
		}
	}()
	if _, err := conn.ExecContext(ctx, `CREATE TABLE `+schema+`.connector_lifecycle_decisions (
		id uuid PRIMARY KEY, owner_account_id uuid NOT NULL, acting_principal_id uuid NOT NULL, status text NOT NULL,
		CONSTRAINT connector_lifecycle_decisions_owner_check CHECK (acting_principal_id = owner_account_id))`); err != nil {
		t.Fatal(err)
	}
	if _, err := conn.ExecContext(ctx, strings.ReplaceAll(allowConnectorDeniedActorAuditSQL, "public.", schema+".")); err != nil {
		t.Fatal(err)
	}
	owner, actor, id := uuid.New(), uuid.New(), uuid.New()
	query := "INSERT INTO " + schema + ".connector_lifecycle_decisions VALUES ($1, $2, $3, $4)"
	if _, err := conn.ExecContext(ctx, query, id, owner, actor, "denied"); err != nil {
		t.Fatalf("truthful denied actor was rejected: %v", err)
	}
	if _, err := conn.ExecContext(ctx, query, uuid.New(), owner, actor, "submitted"); err == nil {
		t.Fatal("foreign actor was allowed to submit")
	}
	if _, err := conn.ExecContext(ctx, query, uuid.New(), owner, owner, "submitted"); err != nil {
		t.Fatalf("owner submission was rejected: %v", err)
	}
	if _, err := conn.ExecContext(ctx, strings.ReplaceAll(restoreConnectorDeniedActorAuditSQL, "public.", schema+".")); err == nil {
		t.Fatal("downgrade ignored retained foreign-actor denials")
	}
	var retained int
	if err := conn.QueryRowContext(ctx, "SELECT count(*) FROM "+schema+".connector_lifecycle_decisions WHERE id = $1", id).Scan(&retained); err != nil || retained != 1 {
		t.Fatalf("downgrade removed an immutable denial: %d %v", retained, err)
	}
}
