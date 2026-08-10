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
		`CREATE TABLE llm_invocation_content_views (id TEXT PRIMARY KEY, organization_id TEXT NOT NULL, request_id TEXT NOT NULL, account_id TEXT NOT NULL, viewed_at DATETIME NOT NULL)`,
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
