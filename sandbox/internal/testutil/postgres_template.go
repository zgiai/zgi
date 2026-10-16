package testutil

import (
	"context"
	"database/sql"
	"fmt"
	"math/rand"
	"net/url"
	"os"
	"strings"
	"sync"
	"testing"
	"time"
)

// TemplateDatabase prepares one database per test process and copies it for
// every test that asks for one, so a package whose tests each need the full
// schema runs its migrations once instead of once per test.
type TemplateDatabase struct {
	prepare func(dsn string) error

	once   sync.Once
	name   string
	err    error
	closed bool
	mu     sync.Mutex
}

// NewTemplateDatabase returns a template that prepare initializes on first
// use. prepare must close every connection it opened: a database with open
// sessions cannot be copied.
func NewTemplateDatabase(prepare func(dsn string) error) *TemplateDatabase {
	return &TemplateDatabase{prepare: prepare}
}

// DSN returns a new database copied from the prepared template; it is removed
// when the test ends.
func (d *TemplateDatabase) DSN(t *testing.T) string {
	t.Helper()
	admin, adminDSN := openTestAdmin(t)
	d.once.Do(func() {
		d.name = fmt.Sprintf("zgi_sandbox_template_%d_%d", os.Getpid(), rand.New(rand.NewSource(time.Now().UnixNano())).Intn(100000))
		if _, err := admin.Exec(`CREATE DATABASE ` + quoteIdentifier(d.name)); err != nil {
			d.err = fmt.Errorf("create template database: %w", err)
			return
		}
		d.err = d.prepare(databaseDSN(t, adminDSN, d.name))
	})
	if d.err != nil {
		t.Fatalf("prepare template database: %v", d.err)
	}
	return createOwnedTestDatabase(t, admin, adminDSN, `TEMPLATE `+quoteIdentifier(d.name))
}

// Close drops the template database; call it from TestMain once the tests
// have run.
func (d *TemplateDatabase) Close() error {
	d.mu.Lock()
	defer d.mu.Unlock()
	if d.name == "" || d.closed {
		return nil
	}
	d.closed = true
	admin, err := sql.Open("pgx", defaultAdminDSN())
	if err != nil {
		return err
	}
	defer admin.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	_, err = admin.ExecContext(ctx, `DROP DATABASE IF EXISTS `+quoteIdentifier(d.name)+` WITH (FORCE)`)
	return err
}

func databaseDSN(t *testing.T, adminDSN string, name string) string {
	t.Helper()
	parsed, err := url.Parse(adminDSN)
	if err != nil {
		t.Fatalf("parse admin dsn: %v", err)
	}
	parsed.Path = "/" + strings.TrimPrefix(name, "/")
	return parsed.String()
}
