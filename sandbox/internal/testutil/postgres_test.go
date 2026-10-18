package testutil

import (
	"database/sql"
	"net/url"
	"os"
	"strings"
	"testing"
)

func TestCreateTestPostgresDSNCleansUpDatabase(t *testing.T) {
	admin, err := sql.Open("pgx", defaultAdminDSN())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = admin.Close() })
	if err := admin.Ping(); err != nil {
		if strings.TrimSpace(os.Getenv("ZGI_SANDBOX_TEST_POSTGRES_ADMIN_DSN")) != "" {
			t.Fatalf("configured test postgres unavailable: %v", err)
		}
		t.Skipf("postgres unavailable: %v", err)
	}
	parent := t
	for _, name := range []string{"idle-database", "open-client"} {
		var databaseName string
		t.Run(name, func(t *testing.T) {
			dsn := CreateTestPostgresDSN(t)
			parsed, err := url.Parse(dsn)
			if err != nil {
				t.Fatal(err)
			}
			databaseName = parsed.Path[1:]
			if name == "open-client" {
				client, err := sql.Open("pgx", dsn)
				if err != nil {
					t.Fatal(err)
				}
				// Deliberately keep this client open through the child's cleanup.
				// Real server fixtures can leave idle pooled connections behind.
				parent.Cleanup(func() { _ = client.Close() })
				if err := client.PingContext(t.Context()); err != nil {
					t.Fatal(err)
				}
			}
		})
		if databaseName == "" {
			t.Fatal("child did not allocate a test database")
		}
		var count int
		if err := admin.QueryRowContext(t.Context(), "SELECT count(*) FROM pg_database WHERE datname = $1", databaseName).Scan(&count); err != nil {
			t.Fatal(err)
		}
		if count != 0 {
			t.Fatalf("remaining owned databases = %d, want 0", count)
		}
	}
}
