package repository

import (
	"context"
	"os"
	"testing"

	"github.com/google/uuid"
	runtimemodel "github.com/zgiai/zgi/api/internal/capabilities/chatruntime/model"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

// Runs against an isolated database named by ZGI_CHAT_RUNTIME_RECORDS_TEST_DSN.
func TestMessageRecordsPostgres(t *testing.T) {
	dsn := os.Getenv("ZGI_CHAT_RUNTIME_RECORDS_TEST_DSN")
	if dsn == "" {
		t.Skip("requires explicit isolated ZGI_CHAT_RUNTIME_RECORDS_TEST_DSN")
	}
	db, err := gorm.Open(postgres.Open(dsn), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	for _, statement := range []string{
		`DROP TABLE IF EXISTS chat_runtime_message_records, chat_runtime_system_prompts, chat_runtime_messages`,
		`CREATE TABLE chat_runtime_messages (id uuid PRIMARY KEY)`,
		`CREATE TABLE chat_runtime_message_records (
			id uuid PRIMARY KEY, message_id uuid NOT NULL REFERENCES chat_runtime_messages(id) ON DELETE CASCADE,
			organization_id uuid NOT NULL, kind varchar(64) NOT NULL, record_id varchar(255) NOT NULL,
			sequence integer NOT NULL DEFAULT 0, payload jsonb NOT NULL,
			created_at timestamptz NOT NULL DEFAULT CURRENT_TIMESTAMP, updated_at timestamptz NOT NULL DEFAULT CURRENT_TIMESTAMP)`,
		`CREATE UNIQUE INDEX uq_chat_runtime_message_records_entry ON chat_runtime_message_records (message_id, kind, record_id)`,
		`CREATE TABLE chat_runtime_system_prompts (hash varchar(64) PRIMARY KEY, content text NOT NULL, created_at timestamptz NOT NULL DEFAULT CURRENT_TIMESTAMP)`,
	} {
		if err := db.Exec(statement).Error; err != nil {
			t.Fatal(err)
		}
	}
	messageID, orgID := uuid.New(), uuid.New()
	if err := db.Exec(`INSERT INTO chat_runtime_messages (id) VALUES (?)`, messageID).Error; err != nil {
		t.Fatal(err)
	}
	repo := NewMessageRecordRepository(db)
	kind := runtimemodel.MessageRecordKindModelInvocation
	for _, record := range []*runtimemodel.MessageRecord{
		{MessageID: messageID, OrganizationID: orgID, Kind: kind, RecordID: "call-2", Sequence: 2, Payload: map[string]interface{}{"runtime_id": "call-2", "phase": "model_call"}},
		{MessageID: messageID, OrganizationID: orgID, Kind: kind, RecordID: "call-1", Sequence: 1, Payload: map[string]interface{}{"runtime_id": "call-1", "phase": "model_call"}},
		// A later write for call-1 adds its usage and keeps what was there.
		{MessageID: messageID, OrganizationID: orgID, Kind: kind, RecordID: "call-1", Payload: map[string]interface{}{"usage": map[string]interface{}{"total_tokens": 42}}},
	} {
		if err := repo.Upsert(ctx, record); err != nil {
			t.Fatal(err)
		}
	}
	listed, err := repo.ListByMessages(ctx, kind, []uuid.UUID{messageID})
	if err != nil {
		t.Fatal(err)
	}
	calls := listed[messageID]
	if len(calls) != 2 || calls[0]["runtime_id"] != "call-1" || calls[0]["phase"] != "model_call" || calls[0]["usage"] == nil || calls[1]["runtime_id"] != "call-2" {
		t.Fatalf("records = %#v, want call-1 merged then call-2", calls)
	}
	if count, err := repo.Count(ctx, messageID, kind); err != nil || count != 2 {
		t.Fatalf("count = %d, %v", count, err)
	}
	for range 2 {
		if err := repo.SaveSystemPrompt(ctx, "hash-1", "You are helpful."); err != nil {
			t.Fatal(err)
		}
	}
	prompts, err := repo.SystemPrompts(ctx, []string{"hash-1", "missing"})
	if err != nil || len(prompts) != 1 || prompts["hash-1"] != "You are helpful." {
		t.Fatalf("prompts = %#v, %v", prompts, err)
	}
	if err := db.Exec(`DELETE FROM chat_runtime_messages WHERE id = ?`, messageID).Error; err != nil {
		t.Fatal(err)
	}
	if count, _ := repo.Count(ctx, messageID, kind); count != 0 {
		t.Fatalf("records outlived their message: %d", count)
	}
}
