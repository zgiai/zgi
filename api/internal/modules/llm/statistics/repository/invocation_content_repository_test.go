package repository

import (
	"context"
	"testing"
	"time"

	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func TestGetInvocationContentIsTenantScopedAndAudited(t *testing.T) {
	db, err := gorm.Open(sqlite.Open("file:"+t.Name()+"?mode=memory&cache=shared"), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	for _, statement := range []string{
		`CREATE TABLE llm_invocation_contents (request_id TEXT PRIMARY KEY, organization_id TEXT NOT NULL, input_text TEXT, output_text TEXT, input_json TEXT, output_json TEXT, content_status TEXT, input_truncated BOOLEAN, output_truncated BOOLEAN, redaction_version TEXT, expires_at DATETIME)`,
		`CREATE TABLE llm_invocation_content_views (id TEXT PRIMARY KEY, organization_id TEXT NOT NULL, request_id TEXT NOT NULL, account_id TEXT NOT NULL, action TEXT NOT NULL DEFAULT 'view', viewed_at DATETIME NOT NULL)`,
	} {
		if err := db.Exec(statement).Error; err != nil {
			t.Fatal(err)
		}
	}
	if err := db.Exec(`INSERT INTO llm_invocation_contents (request_id, organization_id, input_text, output_text, input_json, output_json, content_status, input_truncated, output_truncated, redaction_version, expires_at) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		"req-1", "org-1", "question", "answer", `[]`, `{}`, "available", false, false, "v1", time.Now().Add(time.Hour)).Error; err != nil {
		t.Fatal(err)
	}
	repo := &statisticsRepositoryImpl{db: db}
	detail, err := repo.GetInvocationContent(context.Background(), "org-1", "account-1", "req-1")
	if err != nil || detail.InputText != "question" || detail.OutputText != "answer" {
		t.Fatalf("unexpected detail=%#v err=%v", detail, err)
	}
	var auditCount int64
	if err := db.Table("llm_invocation_content_views").Where("organization_id = ? AND request_id = ? AND account_id = ?", "org-1", "req-1", "account-1").Count(&auditCount).Error; err != nil || auditCount != 1 {
		t.Fatalf("audit count=%d err=%v", auditCount, err)
	}
	if _, err := repo.GetInvocationContent(context.Background(), "org-2", "account-2", "req-1"); err == nil {
		t.Fatal("cross-tenant content read should fail")
	}
}

func TestInvocationContentSettingsAndPurgeAreTenantScopedAndAudited(t *testing.T) {
	db, err := gorm.Open(sqlite.Open("file:"+t.Name()+"?mode=memory&cache=shared"), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	for _, statement := range []string{
		`CREATE TABLE organizations (id TEXT PRIMARY KEY, llm_content_capture_enabled BOOLEAN NOT NULL DEFAULT FALSE, llm_content_retention_days INTEGER)`,
		`CREATE TABLE llm_invocation_contents (request_id TEXT PRIMARY KEY, organization_id TEXT NOT NULL, expires_at DATETIME NOT NULL)`,
		`CREATE TABLE llm_invocation_content_views (id TEXT PRIMARY KEY, organization_id TEXT NOT NULL, request_id TEXT NOT NULL, account_id TEXT NOT NULL, action TEXT NOT NULL DEFAULT 'view', viewed_at DATETIME NOT NULL)`,
	} {
		if err := db.Exec(statement).Error; err != nil {
			t.Fatal(err)
		}
	}
	if err := db.Exec(`INSERT INTO organizations (id) VALUES ('org-1'), ('org-2')`).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Exec(`INSERT INTO llm_invocation_contents (request_id, organization_id, expires_at) VALUES ('one', 'org-1', ?), ('two', 'org-1', ?), ('other', 'org-2', ?)`, time.Now(), time.Now(), time.Now()).Error; err != nil {
		t.Fatal(err)
	}
	repo := &statisticsRepositoryImpl{db: db}
	if err := repo.UpdateInvocationContentSettings(context.Background(), "org-1", true, 7); err != nil {
		t.Fatal(err)
	}
	state, err := repo.GetInvocationContentSettings(context.Background(), "org-1")
	if err != nil || !state.Enabled || state.RetentionDays == nil || *state.RetentionDays != 7 || state.StoredCount != 2 {
		t.Fatalf("unexpected state=%#v err=%v", state, err)
	}
	deleted, hasMore, err := repo.PurgeInvocationContent(context.Background(), "org-1", "account-1")
	if err != nil || deleted != 2 || hasMore {
		t.Fatalf("deleted=%d hasMore=%v err=%v", deleted, hasMore, err)
	}
	var otherCount, auditCount int64
	if err := db.Table("llm_invocation_contents").Where("organization_id = ?", "org-2").Count(&otherCount).Error; err != nil || otherCount != 1 {
		t.Fatalf("other tenant count=%d err=%v", otherCount, err)
	}
	if err := db.Table("llm_invocation_content_views").Where("organization_id = ? AND account_id = ? AND action = ?", "org-1", "account-1", "purge_all").Count(&auditCount).Error; err != nil || auditCount != 1 {
		t.Fatalf("purge audit count=%d err=%v", auditCount, err)
	}
}

func TestInvocationContentPurgeIsBoundedForLargeOrganizations(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(t.TempDir()+"/content-purge.db"), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	for _, statement := range []string{
		`CREATE TABLE llm_invocation_contents (request_id TEXT PRIMARY KEY, organization_id TEXT NOT NULL, expires_at DATETIME NOT NULL)`,
		`CREATE TABLE llm_invocation_content_views (id TEXT PRIMARY KEY, organization_id TEXT NOT NULL, request_id TEXT NOT NULL, account_id TEXT NOT NULL, action TEXT NOT NULL DEFAULT 'view', viewed_at DATETIME NOT NULL)`,
	} {
		if err := db.Exec(statement).Error; err != nil {
			t.Fatal(err)
		}
	}
	if err := db.Exec(`
		WITH RECURSIVE sequence(value) AS (
			SELECT 1 UNION ALL SELECT value + 1 FROM sequence WHERE value < 10001
		)
		INSERT INTO llm_invocation_contents (request_id, organization_id, expires_at)
		SELECT printf('request-%05d', value), 'org-large', datetime('now') FROM sequence`).Error; err != nil {
		t.Fatal(err)
	}

	repo := &statisticsRepositoryImpl{db: db}
	deleted, hasMore, err := repo.PurgeInvocationContent(context.Background(), "org-large", "admin-1")
	if err != nil || deleted != invocationContentPurgeBatchSize*invocationContentPurgeMaxBatchCount || !hasMore {
		t.Fatalf("first purge deleted=%d hasMore=%v err=%v", deleted, hasMore, err)
	}
	deleted, hasMore, err = repo.PurgeInvocationContent(context.Background(), "org-large", "admin-1")
	if err != nil || deleted != 1 || hasMore {
		t.Fatalf("second purge deleted=%d hasMore=%v err=%v", deleted, hasMore, err)
	}
}

func TestGetInvocationContentFailsClosedWhenAuditCannotBeWritten(t *testing.T) {
	db, err := gorm.Open(sqlite.Open("file:"+t.Name()+"?mode=memory&cache=shared"), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	if err := db.Exec(`CREATE TABLE llm_invocation_contents (request_id TEXT PRIMARY KEY, organization_id TEXT NOT NULL, input_text TEXT, output_text TEXT, input_json TEXT, output_json TEXT, content_status TEXT, input_truncated BOOLEAN, output_truncated BOOLEAN, redaction_version TEXT, expires_at DATETIME)`).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Exec(`INSERT INTO llm_invocation_contents (request_id, organization_id, input_text, output_text, input_json, output_json, content_status, input_truncated, output_truncated, redaction_version, expires_at) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		"req-1", "org-1", "question", "answer", `[]`, `{}`, "available", false, false, "v1", time.Now().Add(time.Hour)).Error; err != nil {
		t.Fatal(err)
	}

	repo := &statisticsRepositoryImpl{db: db}
	detail, err := repo.GetInvocationContent(context.Background(), "org-1", "account-1", "req-1")
	if err == nil || detail != nil {
		t.Fatalf("sensitive read must fail closed when audit storage fails: detail=%#v err=%v", detail, err)
	}
}
