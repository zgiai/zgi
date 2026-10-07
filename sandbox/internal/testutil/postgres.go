package testutil

import (
	"context"
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
	t.Cleanup(func() {
		if err := admin.Close(); err != nil {
			t.Errorf("close test database administrator: %v", err)
		}
	})

	if err := admin.Ping(); err != nil {
		t.Skipf("postgres unavailable: %v", err)
	}

	dbName := fmt.Sprintf("zgi_sandbox_test_%d_%d", time.Now().UnixNano(), rand.New(rand.NewSource(time.Now().UnixNano())).Intn(1000))
	if _, err := admin.Exec(`CREATE DATABASE ` + quoteIdentifier(dbName)); err != nil {
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
