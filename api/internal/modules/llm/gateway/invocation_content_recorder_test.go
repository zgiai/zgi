package gateway

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/zgiai/zgi/api/config"
	adapter "github.com/zgiai/zgi/api/internal/modules/llm/protocol/adapters"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func TestInvocationContentRecorderRedactsTruncatesAndHonorsOrganizationSetting(t *testing.T) {
	db, err := gorm.Open(sqlite.Open("file:"+t.Name()+"?mode=memory&cache=shared"), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	for _, statement := range []string{
		`CREATE TABLE organizations (id TEXT PRIMARY KEY, llm_content_capture_enabled BOOLEAN NOT NULL DEFAULT FALSE)`,
		`CREATE TABLE llm_invocation_contents (
			request_id TEXT PRIMARY KEY, organization_id TEXT NOT NULL, input_text TEXT NOT NULL,
			output_text TEXT NOT NULL, input_json TEXT NOT NULL, output_json TEXT NOT NULL,
			content_status TEXT NOT NULL, input_truncated BOOLEAN NOT NULL, output_truncated BOOLEAN NOT NULL,
			redaction_version TEXT NOT NULL, expires_at DATETIME NOT NULL, created_at DATETIME NOT NULL, updated_at DATETIME NOT NULL
		)`,
	} {
		if err := db.Exec(statement).Error; err != nil {
			t.Fatal(err)
		}
	}
	if err := db.Exec(`INSERT INTO organizations (id, llm_content_capture_enabled) VALUES ('org-on', TRUE), ('org-off', FALSE)`).Error; err != nil {
		t.Fatal(err)
	}
	recorder := newInvocationContentRecorder(db, config.LLMInvocationContentConfig{
		Available: true, MaxBytes: 80, RetentionDays: 7, QueueSize: 2, BatchSize: 2,
	})
	if recorder == nil {
		t.Fatal("expected recorder")
	}
	recorder.flush([]invocationContentRecord{
		{RequestID: "req-on", OrganizationID: "org-on", Input: map[string]any{"api_key": "sk-" + "secretvalue", "image_url": "https://example.com/file?signature=private", "content": strings.Repeat("x", 100)}, Output: map[string]any{"content": "Bearer abcdefghijklmnop"}, InputText: "password=" + "hunter2", OutputText: strings.Repeat("y", 100)},
		{RequestID: "req-off", OrganizationID: "org-off", Input: "private", Output: "private"},
	})

	var row invocationContentRow
	if err := db.Where("request_id = ?", "req-on").First(&row).Error; err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(row.InputJSON, "[REDACTED]") || !strings.Contains(row.OutputJSON, "[REDACTED]") {
		t.Fatalf("secrets were not redacted: input=%s output=%s", row.InputJSON, row.OutputJSON)
	}
	sanitizedURL, _ := sanitizeInvocationContent(map[string]any{"image_url": "https://example.com/file?signature=private"}, 1000)
	if strings.Contains(sanitizedURL, "signature") {
		t.Fatalf("signed URL query was not redacted: %s", sanitizedURL)
	}
	if !row.InputTruncated || !row.OutputTruncated {
		t.Fatalf("expected bounded content: %#v", row)
	}
	var disabledCount int64
	if err := db.Table("llm_invocation_contents").Where("request_id = ?", "req-off").Count(&disabledCount).Error; err != nil || disabledCount != 0 {
		t.Fatalf("disabled organization content count=%d err=%v", disabledCount, err)
	}
	recorder.RecordChat(&BillingContext{RequestID: "req-off-2", OrganizationID: "org-off"}, "private", "private", "private", "private")
	if len(recorder.queue) != 0 {
		t.Fatal("known-disabled organization should stay off the gateway queue")
	}
	close(recorder.done)
	time.Sleep(time.Millisecond)
}

func TestInvocationContentRecorderQueuePressureNeverWritesSynchronously(t *testing.T) {
	recorder := &invocationContentRecorder{queue: make(chan invocationContentRecord, 1)}
	billing := &BillingContext{RequestID: "req", OrganizationID: "org"}
	recorder.RecordChat(billing, "first", "first", "first", "first")
	done := make(chan struct{})
	go func() {
		recorder.RecordChat(billing, "second", "second", "second", "second")
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(100 * time.Millisecond):
		t.Fatal("queue pressure blocked the model path")
	}
	if recorder.dropped.Load() != 1 {
		t.Fatalf("dropped count = %d, want 1", recorder.dropped.Load())
	}
}

func TestTraceChatRecordsContentWhenOpenTelemetryIsDisabled(t *testing.T) {
	previous := config.GlobalConfig
	t.Cleanup(func() { config.GlobalConfig = previous })
	config.GlobalConfig = &config.Config{}
	recorder := &invocationContentRecorder{queue: make(chan invocationContentRecord, 1)}
	service := &llmGatewayServiceImpl{invocationContent: recorder}
	service.traceChatCompletion(
		context.Background(),
		&adapter.ChatRequest{Messages: []adapter.Message{{Role: "user", Content: "hello"}}},
		&adapter.ChatResponse{Choices: []adapter.Choice{{Message: adapter.Message{Role: "assistant", Content: "world"}}}},
		time.Now().Add(-time.Second), time.Now(),
		&BillingContext{RequestID: "req", OrganizationID: "org"}, nil,
	)
	select {
	case record := <-recorder.queue:
		if record.InputText != "hello" || record.OutputText != "world" {
			t.Fatalf("unexpected content record: %#v", record)
		}
	default:
		t.Fatal("content was not recorded independently from OpenTelemetry")
	}
}
