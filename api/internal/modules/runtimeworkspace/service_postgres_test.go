package runtimeworkspace

import (
	"errors"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/zgiai/zgi/api/internal/migrations"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
)

func TestCredentialCreationPostgresIdempotentReplayAndExpiry(t *testing.T) {
	dsn := os.Getenv("ZGI_MIGRATION_TEST_DSN")
	if dsn == "" {
		t.Skip("set ZGI_MIGRATION_TEST_DSN to run Runtime Workspace credential PostgreSQL integration")
	}
	// Candidates only report Agent memory when the fail-closed relationship memory runtime is on.
	t.Setenv("ZGI_RELATIONSHIP_MEMORY_ENABLED", "true")
	db, err := gorm.Open(postgres.Open(dsn), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	if err := migrations.RunWithDB(db); err != nil {
		t.Fatalf("apply repository migrations: %v", err)
	}
	tx := db.Begin()
	if tx.Error != nil {
		t.Fatal(tx.Error)
	}
	t.Cleanup(func() { _ = tx.Rollback().Error })

	organizationID, workspaceID, accountID := uuid.New(), uuid.New(), uuid.New()
	if err := tx.Exec("INSERT INTO organizations (id, name) VALUES (?, ?)", organizationID, "Runtime replay test").Error; err != nil {
		t.Fatal(err)
	}
	if err := tx.Exec("INSERT INTO accounts (id, name, email) VALUES (?, ?, ?)", accountID, "Runtime replay owner", uuid.NewString()+"@example.invalid").Error; err != nil {
		t.Fatal(err)
	}
	if err := tx.Exec("INSERT INTO workspaces (id, name, organization_id) VALUES (?, ?, ?)", workspaceID, "Runtime replay workspace", organizationID).Error; err != nil {
		t.Fatal(err)
	}
	if err := tx.Exec("INSERT INTO runtime_workspaces (workspace_id, organization_id, managed_by_account_id, status, mode) VALUES (?, ?, ?, 'active', 'attached')", workspaceID, organizationID, accountID).Error; err != nil {
		t.Fatal(err)
	}
	protector, err := NewCredentialReplayProtector("12345678901234567890123456789012")
	if err != nil {
		t.Fatal(err)
	}
	now := time.Date(2026, 9, 22, 8, 0, 0, 0, time.UTC)
	service := NewService(tx, WithCredentialReplayProtector(protector), WithClock(func() time.Time { return now }))
	agentID, webAppID, versionUUID := uuid.New(), uuid.New(), uuid.New()
	if err := tx.Exec("INSERT INTO agents (id, tenant_id, name, description, agent_type, enable_api, is_public, is_universal, internal, web_app_id, web_app_status) VALUES (?, ?, 'Published Runtime Agent', 'Published presentation', 'AGENT', true, false, false, false, ?, 'active')", agentID, workspaceID, webAppID).Error; err != nil {
		t.Fatal(err)
	}
	if err := tx.Exec("INSERT INTO agent_published_versions (id, agent_id, workspace_id, version, version_uuid, name, description, config_snapshot, created_at) VALUES (?, ?, ?, 'v-test', ?, 'Published Runtime Agent', 'Published presentation', ?::jsonb, ?)", uuid.New(), agentID, workspaceID, versionUUID, `{"file_upload_enabled":true,"supports_vision":true,"agent_memory_enabled":true,"opening_statement":"Hello","suggested_questions":["Start"]}`, now).Error; err != nil {
		t.Fatal(err)
	}
	candidateSet, err := service.ListAgentCandidates(t.Context(), workspaceID, accountID, "current", nil)
	if err != nil {
		t.Fatal(err)
	}
	var publishedCandidate *AgentCandidate
	for index := range candidateSet.Items {
		if candidateSet.Items[index].AgentID == agentID.String() {
			publishedCandidate = &candidateSet.Items[index]
			break
		}
	}
	if publishedCandidate == nil || publishedCandidate.VersionUUID != versionUUID.String() || !publishedCandidate.FileUploadEnabled || !publishedCandidate.SupportsVision || !publishedCandidate.AgentMemoryEnabled {
		t.Fatalf("published Runtime Agent candidate = %#v", publishedCandidate)
	}
	sourceWorkspaceID, sourceAgentID, sourceWebAppID, sourceVersionUUID, unauthorizedAccountID := uuid.New(), uuid.New(), uuid.New(), uuid.New(), uuid.New()
	if err := tx.Exec("INSERT INTO accounts (id, name, email) VALUES (?, 'Unauthorized Runtime manager', ?)", unauthorizedAccountID, uuid.NewString()+"@example.invalid").Error; err != nil {
		t.Fatal(err)
	}
	if err := tx.Exec("INSERT INTO members (organization_id, account_id, role, status) VALUES (?, ?, 'normal', 'active')", organizationID, accountID).Error; err != nil {
		t.Fatal(err)
	}
	if err := tx.Exec("INSERT INTO workspaces (id, name, organization_id) VALUES (?, 'Runtime source workspace', ?)", sourceWorkspaceID, organizationID).Error; err != nil {
		t.Fatal(err)
	}
	if err := tx.Exec("INSERT INTO workspace_members (workspace_id, account_id, role) VALUES (?, ?, 'admin'), (?, ?, 'owner')", workspaceID, accountID, sourceWorkspaceID, accountID).Error; err != nil {
		t.Fatal(err)
	}
	if err := tx.Exec("INSERT INTO agents (id, tenant_id, name, description, agent_type, enable_api, is_public, is_universal, internal, web_app_id, web_app_status) VALUES (?, ?, 'Cross Workspace Agent', 'Cross Workspace presentation', 'AGENT', true, false, false, false, ?, 'active')", sourceAgentID, sourceWorkspaceID, sourceWebAppID).Error; err != nil {
		t.Fatal(err)
	}
	if err := tx.Exec("INSERT INTO agent_published_versions (id, agent_id, workspace_id, version, version_uuid, name, description, config_snapshot, created_at) VALUES (?, ?, ?, 'v-source', ?, 'Cross Workspace Agent', 'Cross Workspace presentation', '{}'::jsonb, ?)", uuid.New(), sourceAgentID, sourceWorkspaceID, sourceVersionUUID, now).Error; err != nil {
		t.Fatal(err)
	}
	crossCandidates, err := service.ListAgentCandidates(t.Context(), workspaceID, accountID, "authorized", nil)
	if err != nil {
		t.Fatal(err)
	}
	if !crossCandidates.CrossWorkspaceEnabled || len(crossCandidates.SourceWorkspaces) != 1 || len(crossCandidates.Items) != 1 || crossCandidates.Items[0].AgentID != sourceAgentID.String() {
		t.Fatalf("cross Workspace candidates = %#v", crossCandidates)
	}
	if _, err := service.ListAgentCandidates(t.Context(), workspaceID, unauthorizedAccountID, "authorized", nil); !errors.Is(err, ErrSourceWorkspacePermission) {
		t.Fatalf("unauthorized cross Workspace candidates error = %v", err)
	}
	binding, err := service.BindAgent(t.Context(), workspaceID, agentID)
	if err != nil {
		t.Fatal(err)
	}
	if binding.PublicAppKey == "" {
		t.Fatal("system-generated App ID is empty")
	}
	if err := service.UnbindAgent(t.Context(), workspaceID, agentID); err != nil {
		t.Fatal(err)
	}
	restored, err := service.BindAgent(t.Context(), workspaceID, agentID)
	if err != nil {
		t.Fatal(err)
	}
	if restored.PublicAppKey != binding.PublicAppKey {
		t.Fatalf("restored App ID = %q, want %q", restored.PublicAppKey, binding.PublicAppKey)
	}
	if _, err := service.SyncAgentBindings(t.Context(), workspaceID, unauthorizedAccountID, 0, []uuid.UUID{agentID, sourceAgentID}); !errors.Is(err, ErrSourceWorkspacePermission) {
		t.Fatalf("unauthorized cross Workspace sync error = %v", err)
	}
	crossSet, err := service.SyncAgentBindings(t.Context(), workspaceID, accountID, 0, []uuid.UUID{agentID, sourceAgentID})
	if err != nil {
		t.Fatal(err)
	}
	if crossSet.Revision != 1 || len(crossSet.Items) != 2 {
		t.Fatalf("cross Workspace binding set = %#v", crossSet)
	}
	emptySet, err := service.SyncAgentBindings(t.Context(), workspaceID, accountID, 1, nil)
	if err != nil {
		t.Fatal(err)
	}
	if emptySet.Revision != 2 || len(emptySet.Items) != 0 {
		t.Fatalf("empty binding set = %#v", emptySet)
	}
	if _, err := service.SyncAgentBindings(t.Context(), workspaceID, accountID, 1, []uuid.UUID{agentID}); !errors.Is(err, ErrBindingRevisionConflict) {
		t.Fatalf("stale binding revision error = %v", err)
	}
	restoredSet, err := service.SyncAgentBindings(t.Context(), workspaceID, accountID, 2, []uuid.UUID{agentID})
	if err != nil {
		t.Fatal(err)
	}
	if restoredSet.Revision != 3 || len(restoredSet.Items) != 1 || restoredSet.Items[0].AppKey != binding.PublicAppKey || restoredSet.Items[0].Status != AgentBindingActive {
		t.Fatalf("restored binding set = %#v", restoredSet)
	}
	unchangedSet, err := service.SyncAgentBindings(t.Context(), workspaceID, accountID, 3, []uuid.UUID{agentID, agentID})
	if err != nil {
		t.Fatal(err)
	}
	if unchangedSet.Revision != 3 {
		t.Fatalf("idempotent binding revision = %d, want 3", unchangedSet.Revision)
	}
	idempotencyKey := "runtime-replay-request-0001"
	created, err := service.CreateCredential(t.Context(), workspaceID, accountID, "production", nil, idempotencyKey)
	if err != nil {
		t.Fatal(err)
	}
	var baselineSubjectCount int64
	if err := tx.Table("user_subjects").Where("issuer_type = ?", SubjectIssuerType).Count(&baselineSubjectCount).Error; err != nil {
		t.Fatal(err)
	}
	if err := tx.Table("workspaces").Where("id = ?", workspaceID).Update("status", "archived").Error; err != nil {
		t.Fatal(err)
	}
	if _, err := service.AuthenticateCredential(t.Context(), created.Secret); !errors.Is(err, ErrWorkspaceDisabled) {
		t.Fatalf("archived resource Workspace authentication error = %v, want ErrWorkspaceDisabled", err)
	}
	if err := tx.Table("workspaces").Where("id = ?", workspaceID).Update("status", "normal").Error; err != nil {
		t.Fatal(err)
	}
	principal, err := service.AuthenticateCredential(t.Context(), created.Secret)
	if err != nil {
		t.Fatal(err)
	}
	var subjectCount int64
	if err := tx.Table("user_subjects").Where("issuer_type = ?", SubjectIssuerType).Count(&subjectCount).Error; err != nil {
		t.Fatal(err)
	}
	if subjectCount != baselineSubjectCount {
		t.Fatalf("credential-only authentication changed Runtime Subject count from %d to %d", baselineSubjectCount, subjectCount)
	}
	if err := service.ResolveRequestPrincipal(t.Context(), principal, "external-user-1", ""); err != nil {
		t.Fatal(err)
	}
	if err := tx.Table("user_subjects").Where("issuer_type = ?", SubjectIssuerType).Count(&subjectCount).Error; err != nil {
		t.Fatal(err)
	}
	if subjectCount != baselineSubjectCount+1 {
		t.Fatalf("resolved Runtime principal changed Subject count from %d to %d, want +1", baselineSubjectCount, subjectCount)
	}
	var operation CredentialOperation
	if err := tx.First(&operation, "credential_id = ?", created.Credential.ID).Error; err != nil {
		t.Fatal(err)
	}
	if operation.SecretEnvelope == nil || strings.Contains(*operation.SecretEnvelope, created.Secret) {
		t.Fatal("credential replay operation did not store an encrypted Secret envelope")
	}
	if operation.IdempotencyKeyHash == idempotencyKey || operation.RequestFingerprint == "" {
		t.Fatal("credential replay operation persisted unsafe request identity")
	}
	replayed, err := service.CreateCredential(t.Context(), workspaceID, accountID, "production", nil, idempotencyKey)
	if err != nil {
		t.Fatal(err)
	}
	if !replayed.Replayed || replayed.Credential.ID != created.Credential.ID || replayed.Secret != created.Secret {
		t.Fatalf("replayed credential = %#v, want original %#v", replayed, created)
	}
	if _, err := service.CreateCredential(t.Context(), workspaceID, accountID, "staging", nil, idempotencyKey); !errors.Is(err, ErrIdempotencyConflict) {
		t.Fatalf("changed request error = %v, want ErrIdempotencyConflict", err)
	}
	revokedKey := "runtime-replay-request-0002"
	revoked, err := service.CreateCredential(t.Context(), workspaceID, accountID, "revoked", nil, revokedKey)
	if err != nil {
		t.Fatal(err)
	}
	if err := service.RevokeCredential(t.Context(), workspaceID, revoked.Credential.ID); err != nil {
		t.Fatal(err)
	}
	var envelopeCount int64
	if err := tx.Model(&CredentialOperation{}).Where("credential_id = ? AND secret_envelope IS NOT NULL", revoked.Credential.ID).Count(&envelopeCount).Error; err != nil {
		t.Fatal(err)
	}
	if envelopeCount != 0 {
		t.Fatal("revoked credential retained its replay envelope")
	}
	if _, err := service.CreateCredential(t.Context(), workspaceID, accountID, "revoked", nil, revokedKey); !errors.Is(err, ErrSecretReplayExpired) {
		t.Fatalf("revoked replay error = %v, want ErrSecretReplayExpired", err)
	}

	now = now.Add(credentialSecretReplayTTL + time.Minute)
	if _, err := service.CreateCredential(t.Context(), workspaceID, accountID, "production", nil, idempotencyKey); !errors.Is(err, ErrSecretReplayExpired) {
		t.Fatalf("expired replay error = %v, want ErrSecretReplayExpired", err)
	}
	// Active keys are limited; revoked and expired ones do not count.
	runtimeID, err := service.ResolveRuntimeID(t.Context(), workspaceID)
	if err != nil {
		t.Fatal(err)
	}
	var active int64
	if err := tx.Model(&Credential{}).Where("runtime_workspace_id = ? AND status = ?", runtimeID, CredentialActive).Count(&active).Error; err != nil {
		t.Fatal(err)
	}
	for i := active; i < MaxActiveCredentials; i++ {
		if err := tx.Create(&Credential{ID: uuid.New(), RuntimeWorkspaceID: runtimeID, KeyPrefix: "zgi_rt", KeyHash: uuid.NewString(), Name: "filler",
			Status: CredentialActive, CreatedByAccountID: accountID, CreatedAt: now, UpdatedAt: now}).Error; err != nil {
			t.Fatal(err)
		}
	}
	if _, err := service.CreateCredential(t.Context(), workspaceID, accountID, "one too many", nil, uuid.NewString()+uuid.NewString()); !errors.Is(err, ErrCredentialLimitExceeded) {
		t.Fatalf("creating past the key limit = %v", err)
	}
}
