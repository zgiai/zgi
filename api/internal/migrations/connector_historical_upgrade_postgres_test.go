package migrations

import (
	"context"
	"crypto/aes"
	"crypto/cipher"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/zgiai/zgi/api/internal/migrations/connectorfinalize"
	"github.com/zgiai/zgi/api/internal/modules/connectors/credentials"
)

// This exercises the published, independent historical tool instead of making
// its old secret format a dependency of the API runtime or finalizer.
func TestConnectorHistoricalUpgradePostgres(t *testing.T) {
	db := finalizationTestDatabase(t)
	ctx := context.Background()
	if err := RunToConnectorCheckpoint(db, "20260902190000_detach_connector_runtime"); err != nil {
		t.Fatal(err)
	}
	org, owner, connection := uuid.NewString(), uuid.NewString(), uuid.NewString()
	oldKey, newKey := strings.Repeat("l", 32), strings.Repeat("n", 32)
	mac := hmac.New(sha256.New, []byte(oldKey))
	mac.Write([]byte("zgi/integration-credentials/aes-256-gcm/v1"))
	block, err := aes.NewCipher(mac.Sum(nil))
	if err != nil {
		t.Fatal(err)
	}
	aead, err := cipher.NewGCM(block)
	if err != nil {
		t.Fatal(err)
	}
	nonce := make([]byte, aead.NonceSize())
	if _, err := rand.Read(nonce); err != nil {
		t.Fatal(err)
	}
	aad := []byte(fmt.Sprintf("zgi/integration-credentials/v2\nkey=legacy\norganization=%s\nconnection=%s\nintegration=github\nversion=1", org, connection))
	envelope := "v2.legacy." + base64.RawURLEncoding.EncodeToString(aead.Seal(nonce, nonce, []byte(`{"token":"historical-regression-only"}`), aad))
	for _, statement := range []struct {
		sql  string
		args []any
	}{
		{`INSERT INTO public.accounts(id,name,email) VALUES (?, 'Historical fixture','connector-upgrade@example.invalid')`, []any{owner}},
		{`INSERT INTO public.organizations(id,name) VALUES (?, 'Historical fixture')`, []any{org}},
		{`INSERT INTO public.integration_connections(id,organization_id,owner_account_id,integration_id,driver_id,name,credential_source,auth_type,auth_method_id,encrypted_credentials,status,health_status,auth_status,scope_status,last_health_checked_at) VALUES (?,?,?,'github','github-rest','Historical key','account','api_key','personal_access_token',?,'active','healthy','valid','verified',now())`, []any{connection, org, owner, envelope}},
		{`UPDATE public.connector_schema_readiness SET state='conversion_required' WHERE id=1`, nil},
	} {
		if err := db.Exec(statement.sql, statement.args...).Error; err != nil {
			t.Fatal(err)
		}
	}
	keyFile := filepath.Join(t.TempDir(), "keyrings.json")
	keyData, _ := json.Marshal(map[string]any{"active": "native", "native": map[string]string{"native": newKey}, "previous": map[string]string{"legacy": oldKey}})
	if err := os.WriteFile(keyFile, keyData, 0600); err != nil {
		t.Fatal(err)
	}
	clear(keyData)
	cfg, err := pgx.ParseConfig(os.Getenv("ZGI_CONNECTOR_FINALIZE_TEST_DSN"))
	if err != nil {
		t.Fatal("parse isolated DSN")
	}
	var dbName string
	if err := db.Raw("SELECT current_database()").Scan(&dbName).Error; err != nil {
		t.Fatal(err)
	}
	dsn := url.URL{Scheme: "postgres", User: url.UserPassword(cfg.User, cfg.Password), Host: fmt.Sprintf("%s:%d", cfg.Host, cfg.Port), Path: "/" + dbName}
	q := url.Values{}
	q.Set("sslmode", "disable")
	dsn.RawQuery = q.Encode()
	_, sourceFile, _, _ := runtime.Caller(0)
	toolDir := filepath.Clean(filepath.Join(filepath.Dir(sourceFile), "..", "..", "..", "tools", "connector-migrate"))
	run := func(action string, extra ...string) string {
		t.Helper()
		args := append([]string{"run", ".", action, "--keys-file", keyFile}, extra...)
		command := exec.CommandContext(ctx, "go", args...)
		command.Dir = toolDir
		for _, item := range os.Environ() {
			if !strings.HasPrefix(item, "CONNECTOR_MIGRATE_DSN=") {
				command.Env = append(command.Env, item)
			}
		}
		command.Env = append(command.Env, "CONNECTOR_MIGRATE_DSN="+dsn.String())
		output, err := command.CombinedOutput()
		if err != nil {
			t.Fatalf("historical %s failed: %s", action, output)
		}
		var report struct {
			SourceDigest string `json:"source_digest"`
			Issues       []any  `json:"issues"`
		}
		if json.Unmarshal(output, &report) != nil || len(report.Issues) > 0 {
			t.Fatalf("historical %s report failed: %s", action, output)
		}
		return report.SourceDigest
	}
	expected := run("dry-run")
	run("apply", "--expected-report", expected)
	run("verify")
	if err := RunToConnectorCheckpoint(db, connectorfinalize.CheckpointID); err != nil {
		t.Fatal(err)
	}
	store, err := credentials.NewStore("native", map[string]string{"native": newKey})
	if err != nil {
		t.Fatal(err)
	}
	option := connectorfinalize.Options{Credentials: store}
	plan, err := connectorfinalize.Plan(ctx, db, option)
	if err != nil {
		t.Fatal(err)
	}
	if len(plan.Issues) > 0 {
		t.Fatalf("historical finalization proof: %+v", plan.Issues)
	}
	archiveKey := make([]byte, 32)
	if _, err := rand.Read(archiveKey); err != nil {
		t.Fatal(err)
	}
	defer clear(archiveKey)
	archive, err := connectorfinalize.CreateArchive(ctx, db, plan.Digest, archiveKey, option)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := connectorfinalize.Apply(ctx, db, connectorfinalize.ApplyOptions{Options: option, ExpectedReport: plan.Digest, Archive: archive, ArchiveKey: archiveKey}); err != nil {
		t.Fatal(err)
	}
	if _, err := connectorfinalize.Verify(ctx, db, option); err != nil {
		t.Fatal(err)
	}
	var preserved struct{ ID, EncryptedCredentials string }
	if err := db.Table("connector_connections").Where("id=?", connection).Take(&preserved).Error; err != nil {
		t.Fatal(err)
	}
	if preserved.ID != connection || !strings.HasPrefix(preserved.EncryptedCredentials, "c1.") {
		t.Fatal("historical identity or native secret lost")
	}
}
