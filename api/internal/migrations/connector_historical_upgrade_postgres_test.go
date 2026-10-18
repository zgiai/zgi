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
	credentialcrypto "github.com/zgiai/zgi/api/internal/infra/credentialcrypto"
	"github.com/zgiai/zgi/api/internal/migrations/connectorfinalize"
	"github.com/zgiai/zgi/api/internal/migrations/digitalhumanpreserve"
	"github.com/zgiai/zgi/api/internal/modules/connectors/credentials"
	"gorm.io/gorm"
)

// This exercises the published, independent historical tool instead of making
// its old secret format a dependency of the API runtime or finalizer.
func TestConnectorHistoricalUpgradePostgres(t *testing.T) {
	for _, recovery := range []bool{false, true} {
		name := "detach"
		if recovery {
			name = "recover_failed_up_with_later_video_migration"
		}
		t.Run(name, func(t *testing.T) { testConnectorHistoricalUpgradePostgres(t, recovery) })
	}
}

func testConnectorHistoricalUpgradePostgres(t *testing.T, recovery bool) {
	db := finalizationTestDatabase(t)
	ctx := context.Background()
	if err := RunToConnectorCheckpoint(db, "20260902190000_detach_connector_runtime"); err != nil {
		t.Fatal(err)
	}
	org, owner, workspace, connection := uuid.NewString(), uuid.NewString(), uuid.NewString(), uuid.NewString()
	workspaceConnection, globalChat, workspaceChat := uuid.NewString(), uuid.NewString(), uuid.NewString()
	channelConnection, installation := uuid.NewString(), uuid.NewString()
	digitalHuman, version, mapping, binding := uuid.NewString(), uuid.NewString(), uuid.NewString(), uuid.NewString()
	dhRun, inbound, delivery, marker := uuid.NewString(), uuid.NewString(), uuid.NewString(), uuid.NewString()
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
	sealLegacy := func(integration, connection, payload string) string {
		t.Helper()
		nonce := make([]byte, aead.NonceSize())
		if _, err := rand.Read(nonce); err != nil {
			t.Fatal(err)
		}
		aad := []byte(fmt.Sprintf("zgi/integration-credentials/v2\nkey=legacy\norganization=%s\nconnection=%s\nintegration=%s\nversion=1", org, connection, integration))
		return "v2.legacy." + base64.RawURLEncoding.EncodeToString(aead.Seal(nonce, nonce, []byte(payload), aad))
	}
	envelope := sealLegacy("github", connection, `{"token":"historical-regression-only"}`)
	workspaceEnvelope := sealLegacy("github", workspaceConnection, `{"token":"workspace-regression-only"}`)
	channelEnvelope := sealLegacy("feishu", channelConnection, `{"app_id":"historical-app","app_secret":"historical-secret"}`)
	for _, statement := range []struct {
		sql  string
		args []any
	}{
		{`INSERT INTO public.accounts(id,name,email) VALUES (?, 'Historical fixture','connector-upgrade@example.invalid')`, []any{owner}},
		{`INSERT INTO public.organizations(id,name) VALUES (?, 'Historical fixture')`, []any{org}},
		{`INSERT INTO public.workspaces(id,name,organization_id) VALUES (?, 'Historical workspace', ?)`, []any{workspace, org}},
		{`INSERT INTO public.integration_connections(id,organization_id,owner_account_id,integration_id,driver_id,name,credential_source,auth_type,auth_method_id,encrypted_credentials,status,health_status,auth_status,scope_status,last_health_checked_at) VALUES (?,?,?,'github','github-rest','Historical key','account','api_key','personal_access_token',?,'active','healthy','valid','verified',now())`, []any{connection, org, owner, envelope}},
		{`INSERT INTO public.integration_connections(id,organization_id,owner_account_id,integration_id,driver_id,name,credential_source,auth_type,auth_method_id,encrypted_credentials,status,health_status,auth_status,scope_status,last_health_checked_at) VALUES (?,?,?,'github','github-rest','Workspace key','account','api_key','personal_access_token',?,'active','healthy','valid','verified',now())`, []any{workspaceConnection, org, owner, workspaceEnvelope}},
		{`INSERT INTO public.aichat_integration_preferences(organization_id,account_id,workspace_id,integration_id,selected_connection_ids,preferred_connection_id) VALUES (?,?,NULL,'github',?,?)`, []any{org, owner, fmt.Sprintf(`["%s"]`, connection), connection}},
		{`INSERT INTO public.aichat_integration_preferences(organization_id,account_id,workspace_id,integration_id,selected_connection_ids,preferred_connection_id) VALUES (?,?,?,'github',?,?)`, []any{org, owner, workspace, fmt.Sprintf(`["%s"]`, workspaceConnection), workspaceConnection}},
		{`INSERT INTO public.chat_runtime_conversations(id,organization_id,account_id,workspace_id,caller_type,title) VALUES (?,?,?,NULL,'aichat','Unscoped chat')`, []any{globalChat, org, owner}},
		{`INSERT INTO public.chat_runtime_conversations(id,organization_id,account_id,workspace_id,caller_type,title) VALUES (?,?,?,?,'aichat','Workspace chat')`, []any{workspaceChat, org, owner, workspace}},
		{`INSERT INTO public.integration_connections(id,organization_id,owner_account_id,integration_id,driver_id,name,credential_source,auth_type,auth_method_id,encrypted_credentials,status,health_status,auth_status,scope_status,last_health_checked_at) VALUES (?,?,NULL,'feishu','feishu-open-platform','Historical channel','organization','api_key','app_credentials',?,'active','healthy','valid','verified',now())`, []any{channelConnection, org, channelEnvelope}},
		{`INSERT INTO public.channel_installations(id,organization_id,workspace_id,provider,connection_id,name,created_by,updated_by) VALUES (?,?,?,?,?,'Historical Feishu',?,?)`, []any{installation, org, workspace, "feishu", channelConnection, owner, owner}},
		// A pending delivery and an active processing marker keep a copy of the
		// installation's legacy connection locator.
		{`INSERT INTO public.digital_humans(id,organization_id,workspace_id,name) VALUES (?,?,?,'Historical assistant')`, []any{digitalHuman, org, workspace}},
		{`INSERT INTO public.digital_human_versions(id,digital_human_id,organization_id,workspace_id,state,revision) VALUES (?,?,?,?,'draft','sha256:historical')`, []any{version, digitalHuman, org, workspace}},
		{`INSERT INTO public.external_conversation_mappings(id,organization_id,workspace_id,installation_id,external_conversation_id,conversation_type) VALUES (?,?,?,?,'oc_historical','group')`, []any{mapping, org, workspace, installation}},
		{`INSERT INTO public.channel_bindings(id,organization_id,workspace_id,installation_id,conversation_mapping_id,subject_type,subject_id,created_by,updated_by) VALUES (?,?,?,?,?,'digital_human',?,?,?)`, []any{binding, org, workspace, installation, mapping, digitalHuman, owner, owner}},
		{`INSERT INTO public.digital_human_runs(id,organization_id,workspace_id,digital_human_id,digital_human_version_id,idempotency_key,invocation_event,run_spec,run_spec_revision,execution_snapshot,execution_snapshot_revision,query) VALUES (?,?,?,?,?,'historical-run','{}','{}','sha256:spec','{}','sha256:snapshot','Hello')`, []any{dhRun, org, workspace, digitalHuman, version}},
		{`INSERT INTO public.channel_inbound_events(id,organization_id,workspace_id,installation_id,binding_id,invocation_run_id,provider_event_id,event_type,payload_digest,received_at) VALUES (?,?,?,?,?,?,'evt_historical','message','sha256:inbound',now())`, []any{inbound, org, workspace, installation, binding, dhRun}},
		{`INSERT INTO public.channel_delivery_outbox(id,organization_id,workspace_id,installation_id,binding_id,inbound_event_id,run_id,provider,connection_id,actor_id,subject_type,subject_id,external_conversation_id,payload,payload_digest) VALUES (?,?,?,?,?,?,?,'feishu',?,?,'digital_human',?,'oc_historical','{}','sha256:delivery')`, []any{delivery, org, workspace, installation, binding, inbound, dhRun, channelConnection, owner, digitalHuman}},
		{`INSERT INTO public.channel_processing_markers(id,organization_id,workspace_id,installation_id,inbound_event_id,run_id,provider,connection_id,actor_id,subject_type,subject_id,provider_message_id,status,available_at,created_at,updated_at) VALUES (?,?,?,?,?,?,'feishu',?,?,'digital_human',?,'om_historical','active',now(),now(),now())`, []any{marker, org, workspace, installation, inbound, dhRun, channelConnection, owner, digitalHuman}},
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
	q.Set("timezone", "Asia/Shanghai")
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
	if recovery {
		// Reproduce the deployed legacy branch: a later unrelated migration is
		// already committed, then normal up stops at populated schema1.
		applyCheckpointVideoMigration(t, db)
		if err := RunWithDB(db); err == nil || !strings.Contains(err.Error(), "ready schema1 checkpoint required") {
			t.Fatalf("normal up must require historical conversion: %v", err)
		}
		var state string
		if err := db.Table("connector_schema_readiness").Select("state").Where("id=1").Scan(&state).Error; err != nil || state != "conversion_required" {
			t.Fatal("failed up attested readiness", state, err)
		}
		if err := RunToConnectorCheckpoint(db, connectorfinalize.DetachCheckpointID); err == nil {
			t.Fatal("checkpoint command silently moved backwards")
		}
	}
	expected := run("dry-run")
	q.Set("timezone", "America/New_York")
	dsn.RawQuery = q.Encode()
	if run("dry-run") != expected {
		t.Fatal("source commitment depends on session timezone")
	}
	run("apply", "--expected-report", expected)
	run("verify")
	for chat, expectedConnection := range map[string]string{globalChat: connection, workspaceChat: workspaceConnection} {
		var bindings []struct{ ConnectionID string }
		if err := db.Table("connector_target_bindings").Where("target_type='chat' AND target_id=? AND connector_id='github'", chat).Find(&bindings).Error; err != nil {
			t.Fatal(err)
		}
		if len(bindings) != 1 || bindings[0].ConnectionID != expectedConnection {
			t.Fatal("historical chat preference crossed workspace scope")
		}
	}
	if err := RunToConnectorCheckpoint(db, connectorfinalize.CheckpointID); err != nil {
		t.Fatal(err)
	}
	digitalHumanCipher, err := credentialcrypto.NewCredentialKeyring("legacy", map[string]string{"legacy": oldKey})
	if err != nil {
		t.Fatal(err)
	}
	store, err := credentials.NewStore("native", map[string]string{"native": newKey})
	if err != nil {
		t.Fatal(err)
	}
	option := connectorfinalize.Options{Credentials: store}
	// Finalization must refuse while an installation still resolves the
	// legacy organization Connection it is about to retire.
	unpreserved, err := connectorfinalize.Plan(ctx, db, option)
	if err != nil {
		t.Fatal(err)
	}
	if !finalizeIssue(unpreserved, "channel_installations", "installation_credential_preservation_required") {
		t.Fatalf("finalization accepted an unpreserved channel installation: %+v", unpreserved.Issues)
	}
	digitalHumanPlan, err := digitalhumanpreserve.Plan(ctx, db, digitalHumanCipher)
	if err != nil {
		t.Fatal(err)
	}
	if len(digitalHumanPlan.Issues) != 0 || len(digitalHumanPlan.Items) != 1 || digitalHumanPlan.Items[0].InstallationID != installation {
		t.Fatalf("DigitalHuman channel preservation plan: %+v", digitalHumanPlan)
	}
	if _, err := digitalhumanpreserve.Apply(ctx, db, digitalHumanCipher, digitalHumanPlan.Digest); err != nil {
		t.Fatal(err)
	}
	if _, err := digitalhumanpreserve.Verify(ctx, db, digitalHumanCipher); err != nil {
		t.Fatal(err)
	}
	assertChannelLocators(t, db, installation, delivery, marker)
	// A database preserved by an earlier release kept the legacy locators; the
	// next preservation run reports and repairs them.
	if err := db.Transaction(func(tx *gorm.DB) error {
		if err := tx.Exec("SET LOCAL session_replication_role = replica").Error; err != nil {
			return err
		}
		if err := tx.Exec("UPDATE public.channel_delivery_outbox SET connection_id = ? WHERE id = ?", channelConnection, delivery).Error; err != nil {
			return err
		}
		return tx.Exec("UPDATE public.channel_processing_markers SET connection_id = ? WHERE id = ?", channelConnection, marker).Error
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := digitalhumanpreserve.Verify(ctx, db, digitalHumanCipher); err == nil {
		t.Fatal("preservation verified stale delivery locators")
	}
	repairPlan, err := digitalhumanpreserve.Plan(ctx, db, digitalHumanCipher)
	if err != nil {
		t.Fatal(err)
	}
	if len(repairPlan.Items) != 0 || len(repairPlan.Repairs) != 1 || repairPlan.Repairs[0].Deliveries != 1 || repairPlan.Repairs[0].Markers != 1 {
		t.Fatalf("stale locator repair plan: %+v", repairPlan)
	}
	if _, err := digitalhumanpreserve.Apply(ctx, db, digitalHumanCipher, repairPlan.Digest); err != nil {
		t.Fatal(err)
	}
	if _, err := digitalhumanpreserve.Verify(ctx, db, digitalHumanCipher); err != nil {
		t.Fatal(err)
	}
	assertChannelLocators(t, db, installation, delivery, marker)
	var channelCredential struct {
		InstallationID       string
		EncryptedCredentials string
		CredentialVersion    int
	}
	if err := db.Table("channel_installation_credentials").Where("installation_id = ?", installation).Take(&channelCredential).Error; err != nil {
		t.Fatal(err)
	}
	installationID := uuid.MustParse(installation)
	organizationID := uuid.MustParse(org)
	preservedChannelValues, err := digitalHumanCipher.DecryptCredentials(channelCredential.EncryptedCredentials, credentialcrypto.CredentialAAD{
		OrganizationID: organizationID, ConnectionID: installationID, IntegrationID: "feishu", CredentialVersion: channelCredential.CredentialVersion,
	})
	if err != nil || preservedChannelValues["app_secret"] != "historical-secret" {
		t.Fatalf("preserved DigitalHuman channel credential is unreadable: %v", err)
	}
	credentialcrypto.DestroyMap(preservedChannelValues)
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
	var finalizedInstallation struct{ ConnectionID string }
	if err := db.Table("channel_installations").Where("id = ?", installation).Take(&finalizedInstallation).Error; err != nil {
		t.Fatal(err)
	}
	if finalizedInstallation.ConnectionID != installation {
		t.Fatal("DigitalHuman channel installation still references a retired Connection")
	}
	if err := RunWithDB(db); err != nil {
		t.Fatal("remaining migrations after historical conversion", err)
	}
	if _, err := connectorfinalize.Verify(ctx, db, option); err != nil {
		t.Fatal(err)
	}
}

func finalizeIssue(report connectorfinalize.Report, table, code string) bool {
	for _, issue := range report.Issues {
		if issue.Table == table && issue.Code == code {
			return true
		}
	}
	return false
}

// assertChannelLocators requires the delivery and marker to follow the
// installation-owned locator, and the delivery to stay claimable: the channel
// scope trigger checks the locator on every outbox update.
func assertChannelLocators(t *testing.T, db *gorm.DB, installation, delivery, marker string) {
	t.Helper()
	for table, id := range map[string]string{"channel_delivery_outbox": delivery, "channel_processing_markers": marker} {
		var connection string
		if err := db.Table(table).Select("connection_id").Where("id = ?", id).Scan(&connection).Error; err != nil {
			t.Fatal(err)
		}
		if connection != installation {
			t.Fatalf("%s still carries connection locator %s", table, connection)
		}
	}
	if err := db.Transaction(func(tx *gorm.DB) error {
		if err := tx.Exec("UPDATE public.channel_delivery_outbox SET status = 'processing', claimed_at = now() WHERE id = ? AND status = 'pending'", delivery).Error; err != nil {
			return err
		}
		return fmt.Errorf("rollback claim probe")
	}); err == nil || err.Error() != "rollback claim probe" {
		t.Fatalf("preserved delivery cannot be claimed: %v", err)
	}
}
