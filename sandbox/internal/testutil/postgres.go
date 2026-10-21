package testutil

import (
	"context"
	"database/sql"
	"fmt"
	"math/rand"
	"os"
	"strings"
	"testing"
	"time"

	_ "github.com/jackc/pgx/v5/stdlib"
)

func CreateTestPostgresDSN(t *testing.T) string {
	t.Helper()
	admin, adminDSN := openTestAdmin(t)
	return createOwnedTestDatabase(t, admin, adminDSN, "")
}

// openTestAdmin connects to the administrative database, skipping the test
// when no PostgreSQL is available unless one was explicitly configured.
func openTestAdmin(t *testing.T) (*sql.DB, string) {
	t.Helper()
	adminDSN := defaultAdminDSN()
	admin, err := sql.Open("pgx", adminDSN)
	if err != nil {
		if strings.TrimSpace(os.Getenv("ZGI_SANDBOX_TEST_POSTGRES_ADMIN_DSN")) != "" {
			t.Fatalf("configured test postgres unavailable: %v", err)
		}
		t.Skipf("postgres unavailable: %v", err)
	}
	t.Cleanup(func() {
		if err := admin.Close(); err != nil {
			t.Errorf("close test database administrator: %v", err)
		}
	})

	if err := admin.Ping(); err != nil {
		if strings.TrimSpace(os.Getenv("ZGI_SANDBOX_TEST_POSTGRES_ADMIN_DSN")) != "" {
			t.Fatalf("configured test postgres unavailable: %v", err)
		}
		t.Skipf("postgres unavailable: %v", err)
	}
	return admin, adminDSN
}

// createOwnedTestDatabase creates a database the test owns, with the given
// CREATE DATABASE options, and drops it when the test ends.
func createOwnedTestDatabase(t *testing.T, admin *sql.DB, adminDSN string, options string) string {
	t.Helper()
	dbName := fmt.Sprintf("zgi_sandbox_test_%d_%d", time.Now().UnixNano(), rand.New(rand.NewSource(time.Now().UnixNano())).Intn(1000))
	statement := `CREATE DATABASE ` + quoteIdentifier(dbName)
	if options != "" {
		statement += " " + options
	}
	if _, err := admin.Exec(statement); err != nil {
		t.Fatalf("create test database: %v", err)
	}

	t.Cleanup(func() {
		// Keep the administrator open until this LIFO cleanup has dropped only
		// the database allocated above, including idle application connections.
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		if _, err := admin.ExecContext(ctx, `DROP DATABASE IF EXISTS `+quoteIdentifier(dbName)+` WITH (FORCE)`); err != nil {
			t.Errorf("remove owned test database: %v", err)
		}
	})
	return databaseDSN(t, adminDSN, dbName)
}

func defaultAdminDSN() string {
	if value := strings.TrimSpace(os.Getenv("ZGI_SANDBOX_TEST_POSTGRES_ADMIN_DSN")); value != "" {
		return value
	}
	return "postgres://postgres@127.0.0.1:5432/postgres?sslmode=disable"
}

func quoteIdentifier(value string) string {
	return `"` + strings.ReplaceAll(value, `"`, `""`) + `"`
}
