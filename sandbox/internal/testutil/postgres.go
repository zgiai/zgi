package testutil

import (
	"database/sql"
	"fmt"
	"math/rand"
	"net/url"
	"os"
	"strings"
	"testing"
	"time"

	_ "github.com/jackc/pgx/v5/stdlib"
)

func CreateTestPostgresDSN(t *testing.T) string {
	t.Helper()

	adminDSN := defaultAdminDSN()
	admin, err := sql.Open("pgx", adminDSN)
	if err != nil {
		t.Skipf("postgres unavailable: %v", err)
	}
	defer admin.Close()

	if err := admin.Ping(); err != nil {
		t.Skipf("postgres unavailable: %v", err)
	}

	dbName := fmt.Sprintf("zgi_sandbox_test_%d_%d", time.Now().UnixNano(), rand.New(rand.NewSource(time.Now().UnixNano())).Intn(1000))
	if _, err := admin.Exec(`CREATE DATABASE ` + quoteIdentifier(dbName)); err != nil {
		t.Fatalf("create test database: %v", err)
	}

	t.Cleanup(func() {
		_, _ = admin.Exec(`
			SELECT pg_terminate_backend(pid)
			FROM pg_stat_activity
			WHERE datname = $1 AND pid <> pg_backend_pid()
		`, dbName)
		_, _ = admin.Exec(`DROP DATABASE IF EXISTS ` + quoteIdentifier(dbName))
	})

	parsed, err := url.Parse(adminDSN)
	if err != nil {
		t.Fatalf("parse admin dsn: %v", err)
	}
	parsed.Path = "/" + dbName
	return parsed.String()
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
