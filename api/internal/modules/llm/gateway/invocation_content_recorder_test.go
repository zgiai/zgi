package gateway

import (
	"context"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/zgiai/zgi/api/config"
	adapter "github.com/zgiai/zgi/api/internal/modules/llm/protocol/adapters"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func TestInvocationContentRecorderRedactsTruncatesAndHonorsOrganizationSetting(t *testing.T) {
	db, err := gorm.Open(sqlite.Open("file:"+uuid.NewString()+"?mode=memory&cache=shared"), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	for _, statement := range []string{
		`CREATE TABLE organizations (id TEXT PRIMARY KEY, llm_content_capture_enabled BOOLEAN NOT NULL DEFAULT FALSE, llm_content_retention_days INTEGER)`,
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
	if err := db.Exec(`INSERT INTO organizations (id, llm_content_capture_enabled, llm_content_retention_days) VALUES ('org-on', TRUE, 3), ('org-off', FALSE, NULL)`).Error; err != nil {
		t.Fatal(err)
	}
	recorder := newInvocationContentRecorder(db, config.LLMInvocationContentConfig{
		MaxBytes: 80, RetentionDays: 7, QueueSize: 2, BatchSize: 2,
	})
	if recorder == nil {
		t.Fatal("expected recorder")
	}
	recorder.flush([]invocationContentRecord{
		recorder.prepareRecord(&BillingContext{RequestID: "req-on", OrganizationID: "org-on"}, map[string]any{"api_key": "sk-" + "secretvalue", "image_url": "https://example.com/file?signature=private", "content": strings.Repeat("x", 100)}, map[string]any{"content": "Bearer abcdefghijklmnop"}, "password="+"hunter2", strings.Repeat("y", 100)),
		recorder.prepareRecord(&BillingContext{RequestID: "req-off", OrganizationID: "org-off"}, "private", "private", "", ""),
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
	if remaining := time.Until(row.ExpiresAt); remaining < 71*time.Hour || remaining > 73*time.Hour {
		t.Fatalf("organization retention not applied, expires in %s", remaining)
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
	recorder := &invocationContentRecorder{
		config: config.LLMInvocationContentConfig{MaxBytes: 1024},
		queue:  make(chan invocationContentRecord, 1),
	}
	billing := &BillingContext{RequestID: "req", OrganizationID: "org"}
	input := map[string]any{"api_key": "sk-" + "secretvalue", "content": "first"}
	recorder.RecordChat(billing, input, "Bearer abcdefghijklmnop", "first", "first")
	input["content"] = "mutated"
	queued := <-recorder.queue
	if strings.Contains(queued.InputJSON, "secretvalue") || strings.Contains(queued.OutputJSON, "abcdefgh") || strings.Contains(queued.InputJSON, "mutated") {
		t.Fatalf("queue must contain an immutable sanitized snapshot: %#v", queued)
	}
	recorder.queue <- queued
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

func TestInvocationContentSettingsUpdateIsSharedAcrossGatewayServices(t *testing.T) {
	db, err := gorm.Open(sqlite.Open("file:"+uuid.NewString()+"?mode=memory&cache=shared"), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	first := &invocationContentRecorder{
		config: config.LLMInvocationContentConfig{MaxBytes: 1024},
		queue:  make(chan invocationContentRecord, 1), settings: invocationContentSettingsCache(db),
	}
	second := &invocationContentRecorder{
		config: config.LLMInvocationContentConfig{MaxBytes: 1024},
		queue:  make(chan invocationContentRecord, 1), settings: invocationContentSettingsCache(db),
	}
	first.settings.Store("org-1", invocationContentSettingCache{enabled: false, expiresAt: time.Now().Add(time.Hour)})
	second.RecordChat(&BillingContext{RequestID: "before", OrganizationID: "org-1"}, "input", "output", "input", "output")
	if len(second.queue) != 0 {
		t.Fatal("disabled setting should keep content off the queue")
	}

	UpdateInvocationContentSettingsCache(db, "org-1", true, 7)

	for index, recorder := range []*invocationContentRecorder{first, second} {
		cached, ok := recorder.settings.Load("org-1")
		if !ok {
			t.Fatalf("recorder %d did not receive updated setting", index)
		}
		setting := cached.(invocationContentSettingCache)
		if !setting.enabled || setting.retentionDays != 7 {
			t.Fatalf("recorder %d setting = %#v", index, setting)
		}
	}
	second.RecordChat(&BillingContext{RequestID: "after-enable", OrganizationID: "org-1"}, "input", "output", "input", "output")
	select {
	case record := <-second.queue:
		if record.RequestID != "after-enable" {
			t.Fatalf("first call after enable = %#v", record)
		}
	case <-time.After(100 * time.Millisecond):
		t.Fatal("first call after enable was not queued")
	}

	UpdateInvocationContentSettingsCache(db, "org-1", false, 7)
	first.RecordChat(&BillingContext{RequestID: "after-disable", OrganizationID: "org-1"}, "input", "output", "input", "output")
	if len(first.queue) != 0 {
		t.Fatal("first call after disable should stay off the queue")
	}
}

func TestInvocationContentRecorderExpiredDisabledSettingQueuesOnlyProbe(t *testing.T) {
	recorder := &invocationContentRecorder{
		config:   config.LLMInvocationContentConfig{MaxBytes: 1024},
		queue:    make(chan invocationContentRecord, 1),
		settings: &sync.Map{},
	}
	recorder.settings.Store("org-off", invocationContentSettingCache{
		enabled: false, expiresAt: time.Now().Add(-time.Second),
	})
	recorder.RecordChat(
		&BillingContext{RequestID: "req", OrganizationID: "org-off"},
		map[string]any{"content": strings.Repeat("sensitive", 100)}, "output", "input", "output",
	)
	probe := <-recorder.queue
	if !probe.SettingsProbe || probe.OrganizationID != "org-off" || probe.RequestID != "" || probe.InputJSON != "" {
		t.Fatalf("disabled refresh must contain no invocation content: %#v", probe)
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

func TestCleanupExpiredInvocationContentRunsWhenCaptureIsDisabled(t *testing.T) {
	db, err := gorm.Open(sqlite.Open("file:"+uuid.NewString()+"?mode=memory&cache=shared"), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	if err := db.Exec(`CREATE TABLE llm_invocation_contents (request_id TEXT PRIMARY KEY, expires_at DATETIME NOT NULL)`).Error; err != nil {
		t.Fatal(err)
	}
	now := time.Now().UTC()
	if err := db.Exec(`INSERT INTO llm_invocation_contents (request_id, expires_at) VALUES (?, ?), (?, ?)`, "expired", now.Add(-time.Minute), "active", now.Add(time.Minute)).Error; err != nil {
		t.Fatal(err)
	}
	deleted, err := cleanupExpiredInvocationContent(context.Background(), db, now)
	if err != nil || deleted != 1 {
		t.Fatal(err)
	}
	var count int64
	if err := db.Table("llm_invocation_contents").Count(&count).Error; err != nil || count != 1 {
		t.Fatalf("remaining content count=%d err=%v", count, err)
	}
}

func TestCleanupExpiredInvocationContentBoundsWorkPerRun(t *testing.T) {
	db, err := gorm.Open(sqlite.Open("file:"+uuid.NewString()+"?mode=memory&cache=shared"), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	if err := db.Exec(`CREATE TABLE llm_invocation_contents (request_id TEXT PRIMARY KEY, expires_at DATETIME NOT NULL)`).Error; err != nil {
		t.Fatal(err)
	}
	now := time.Now().UTC()
	if err := db.Transaction(func(tx *gorm.DB) error {
		for i := 0; i < invocationContentCleanupBatchSize+1; i++ {
			if err := tx.Exec(`INSERT INTO llm_invocation_contents (request_id, expires_at) VALUES (?, ?)`, uuid.NewString(), now.Add(-time.Minute)).Error; err != nil {
				return err
			}
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	deleted, err := cleanupExpiredInvocationContent(context.Background(), db, now)
	want := int64(invocationContentCleanupBatchSize + 1)
	if err != nil || deleted != want {
		t.Fatalf("deleted=%d want=%d err=%v", deleted, want, err)
	}
}

func BenchmarkInvocationContentRecorderDisabledFastPath(b *testing.B) {
	recorder := &invocationContentRecorder{queue: make(chan invocationContentRecord, 1), settings: &sync.Map{}}
	recorder.settings.Store("org-off", invocationContentSettingCache{
		enabled: false, expiresAt: time.Now().Add(time.Hour),
	})
	billing := &BillingContext{RequestID: "req", OrganizationID: "org-off"}
	b.ReportAllocs()
	b.ResetTimer()
	for range b.N {
		recorder.RecordChat(billing, "input", "output", "input", "output")
	}
}
