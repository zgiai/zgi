package gateway

import (
	"context"
	"encoding/json"
	"regexp"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/zgiai/zgi/api/config"
	adapter "github.com/zgiai/zgi/api/internal/modules/llm/protocol/adapters"
	"github.com/zgiai/zgi/api/pkg/logger"
	"go.uber.org/zap"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

const invocationContentRedactionVersion = "v1"

var invocationContentSecretPatterns = []*regexp.Regexp{
	regexp.MustCompile(`(?i)\b(?:bearer|basic)\s+[a-z0-9._~+/=-]+`),
	regexp.MustCompile(`\bsk-[a-zA-Z0-9_-]{8,}\b`),
	regexp.MustCompile(`(?i)\b(?:api[_-]?key|access_token|refresh_token|password|secret)=([^\s&]+)`),
}

type invocationContentRecord struct {
	RequestID       string
	OrganizationID  string
	InputText       string
	OutputText      string
	InputJSON       string
	OutputJSON      string
	InputTruncated  bool
	OutputTruncated bool
}

type invocationContentRow struct {
	RequestID        string    `gorm:"column:request_id;primaryKey"`
	OrganizationID   string    `gorm:"column:organization_id"`
	InputText        string    `gorm:"column:input_text"`
	OutputText       string    `gorm:"column:output_text"`
	InputJSON        string    `gorm:"column:input_json"`
	OutputJSON       string    `gorm:"column:output_json"`
	ContentStatus    string    `gorm:"column:content_status"`
	InputTruncated   bool      `gorm:"column:input_truncated"`
	OutputTruncated  bool      `gorm:"column:output_truncated"`
	RedactionVersion string    `gorm:"column:redaction_version"`
	ExpiresAt        time.Time `gorm:"column:expires_at"`
	CreatedAt        time.Time `gorm:"column:created_at"`
	UpdatedAt        time.Time `gorm:"column:updated_at"`
}

func (invocationContentRow) TableName() string { return "llm_invocation_contents" }

// invocationContentRecorder is deliberately lossy under pressure. Content is
// optional audit data; billing and the metadata log must never wait for it.
type invocationContentRecorder struct {
	db       *gorm.DB
	config   config.LLMInvocationContentConfig
	queue    chan invocationContentRecord
	done     chan struct{}
	dropped  atomic.Uint64
	settings sync.Map
}

type invocationContentSettingCache struct {
	enabled   bool
	expiresAt time.Time
}

func newInvocationContentRecorder(db *gorm.DB, cfg config.LLMInvocationContentConfig) *invocationContentRecorder {
	if db == nil {
		return nil
	}
	recorder := &invocationContentRecorder{
		db: db, config: cfg, queue: make(chan invocationContentRecord, cfg.QueueSize), done: make(chan struct{}),
	}
	go recorder.run()
	return recorder
}

func (r *invocationContentRecorder) RecordChat(billing *BillingContext, input any, output any, inputText, outputText string) {
	if r == nil || billing == nil || strings.TrimSpace(billing.RequestID) == "" || strings.TrimSpace(billing.OrganizationID) == "" {
		return
	}
	if cached, ok := r.settings.Load(billing.OrganizationID); ok {
		setting := cached.(invocationContentSettingCache)
		if time.Now().Before(setting.expiresAt) && !setting.enabled {
			return
		}
	}
	// Avoid doing redaction work when backpressure is already visible. The
	// final non-blocking send below still handles races with other producers.
	if cap(r.queue) > 0 && len(r.queue) >= cap(r.queue) {
		r.dropped.Add(1)
		return
	}
	record := r.prepareRecord(billing, input, output, inputText, outputText)
	select {
	case r.queue <- record:
	default:
		r.dropped.Add(1)
	}
}

// prepareRecord creates an immutable, redacted and bounded snapshot before it
// enters the asynchronous queue. This prevents callers from racing with the
// worker by reusing request objects and keeps raw secrets out of queue memory.
func (r *invocationContentRecorder) prepareRecord(billing *BillingContext, input any, output any, inputText, outputText string) invocationContentRecord {
	inputJSON, inputTruncated := sanitizeInvocationContent(input, r.config.MaxBytes)
	outputJSON, outputTruncated := sanitizeInvocationContent(output, r.config.MaxBytes)
	sanitizedInputText, inputTextTruncated := sanitizeInvocationContentText(inputText, r.config.MaxBytes)
	sanitizedOutputText, outputTextTruncated := sanitizeInvocationContentText(outputText, r.config.MaxBytes)
	return invocationContentRecord{
		RequestID: billing.RequestID, OrganizationID: billing.OrganizationID,
		InputText: sanitizedInputText, OutputText: sanitizedOutputText,
		InputJSON: inputJSON, OutputJSON: outputJSON,
		InputTruncated:  inputTruncated || inputTextTruncated,
		OutputTruncated: outputTruncated || outputTextTruncated,
	}
}

func (r *invocationContentRecorder) run() {
	flushEvery := time.NewTicker(200 * time.Millisecond)
	dropReportEvery := time.NewTicker(time.Minute)
	defer flushEvery.Stop()
	defer dropReportEvery.Stop()
	batch := make([]invocationContentRecord, 0, r.config.BatchSize)
	flush := func() {
		if len(batch) == 0 {
			return
		}
		r.flush(batch)
		batch = batch[:0]
	}
	for {
		select {
		case record := <-r.queue:
			batch = append(batch, record)
			if len(batch) >= r.config.BatchSize {
				flush()
			}
		case <-flushEvery.C:
			flush()
		case <-dropReportEvery.C:
			if dropped := r.dropped.Swap(0); dropped > 0 {
				logger.Warn("llm invocation content dropped", zap.Uint64("count", dropped), zap.String("reason", "queue_full"))
			}
		case <-r.done:
			for {
				select {
				case record := <-r.queue:
					batch = append(batch, record)
				default:
					flush()
					return
				}
			}
		}
	}
}

func (r *invocationContentRecorder) flush(records []invocationContentRecord) {
	organizationIDs := make([]string, 0, len(records))
	seen := make(map[string]struct{}, len(records))
	for _, record := range records {
		if _, ok := seen[record.OrganizationID]; !ok {
			seen[record.OrganizationID] = struct{}{}
			organizationIDs = append(organizationIDs, record.OrganizationID)
		}
	}
	var enabledOrganizationIDs []string
	if err := r.db.WithContext(context.Background()).Table("organizations").
		Where("id IN ? AND llm_content_capture_enabled = ?", organizationIDs, true).
		Pluck("id", &enabledOrganizationIDs).Error; err != nil {
		logger.Warn("failed to resolve llm invocation content settings", zap.Error(err))
		return
	}
	enabled := make(map[string]struct{}, len(enabledOrganizationIDs))
	for _, id := range enabledOrganizationIDs {
		enabled[id] = struct{}{}
	}
	settingExpiresAt := time.Now().Add(5 * time.Second)
	for _, id := range organizationIDs {
		_, isEnabled := enabled[id]
		r.settings.Store(id, invocationContentSettingCache{enabled: isEnabled, expiresAt: settingExpiresAt})
	}

	now := time.Now().UTC()
	rows := make([]invocationContentRow, 0, len(records))
	for _, record := range records {
		if _, ok := enabled[record.OrganizationID]; !ok {
			continue
		}
		rows = append(rows, invocationContentRow{
			RequestID: record.RequestID, OrganizationID: record.OrganizationID,
			InputText: record.InputText, OutputText: record.OutputText, InputJSON: record.InputJSON, OutputJSON: record.OutputJSON,
			ContentStatus: "available", InputTruncated: record.InputTruncated,
			OutputTruncated: record.OutputTruncated, RedactionVersion: invocationContentRedactionVersion,
			ExpiresAt: now.AddDate(0, 0, r.config.RetentionDays), CreatedAt: now, UpdatedAt: now,
		})
	}
	if len(rows) == 0 {
		return
	}
	if err := r.db.WithContext(context.Background()).Clauses(clause.OnConflict{
		Columns:   []clause.Column{{Name: "request_id"}},
		DoUpdates: clause.AssignmentColumns([]string{"input_text", "output_text", "input_json", "output_json", "content_status", "input_truncated", "output_truncated", "redaction_version", "expires_at", "updated_at"}),
	}).Create(&rows).Error; err != nil {
		logger.Warn("failed to write llm invocation content batch", zap.Error(err), zap.Int("count", len(rows)))
	}
}

func sanitizeInvocationContent(value any, maxBytes int) (string, bool) {
	data, err := json.Marshal(value)
	if err != nil {
		data = []byte(`null`)
	}
	var decoded any
	if err := json.Unmarshal(data, &decoded); err == nil {
		decoded = redactInvocationContentValue("", decoded)
		if sanitized, marshalErr := json.Marshal(decoded); marshalErr == nil {
			data = sanitized
		}
	}
	return truncateInvocationContent(string(data), maxBytes)
}

func redactInvocationContentValue(key string, value any) any {
	if invocationContentSensitiveKey(key) {
		return "[REDACTED]"
	}
	switch typed := value.(type) {
	case map[string]any:
		out := make(map[string]any, len(typed))
		for nestedKey, nested := range typed {
			out[nestedKey] = redactInvocationContentValue(nestedKey, nested)
		}
		return out
	case []any:
		out := make([]any, len(typed))
		for i, nested := range typed {
			out[i] = redactInvocationContentValue("", nested)
		}
		return out
	case string:
		redacted := redactInvocationContentString(typed)
		if invocationContentURLKey(key) {
			if index := strings.IndexAny(redacted, "?#"); index >= 0 {
				return redacted[:index]
			}
		}
		return redacted
	default:
		return typed
	}
}

func invocationContentURLKey(key string) bool {
	normalized := strings.ToLower(strings.NewReplacer("-", "_", ".", "_").Replace(key))
	return normalized == "url" || strings.HasSuffix(normalized, "_url")
}

func invocationContentSensitiveKey(key string) bool {
	normalized := strings.ToLower(strings.NewReplacer("-", "_", ".", "_").Replace(key))
	for _, sensitive := range []string{"password", "passwd", "secret", "api_key", "apikey", "authorization", "cookie", "credential", "private_key", "access_token", "refresh_token"} {
		if normalized == sensitive || strings.Contains(normalized, sensitive) {
			return true
		}
	}
	return false
}

func sanitizeInvocationContentText(value string, maxBytes int) (string, bool) {
	return truncateInvocationContent(redactInvocationContentString(value), maxBytes)
}

func redactInvocationContentString(value string) string {
	value = strings.ToValidUTF8(value, "\uFFFD")
	for _, pattern := range invocationContentSecretPatterns {
		value = pattern.ReplaceAllString(value, "[REDACTED]")
	}
	return value
}

func truncateInvocationContent(value string, maxBytes int) (string, bool) {
	if maxBytes <= 0 || len(value) <= maxBytes {
		return value, false
	}
	return strings.ToValidUTF8(value[:maxBytes], "\uFFFD") + "...[truncated]", true
}

func lastUserMessageText(req *adapter.ChatRequest) string {
	if req == nil {
		return ""
	}
	for index := len(req.Messages) - 1; index >= 0; index-- {
		if strings.EqualFold(req.Messages[index].Role, "user") {
			return messageContentText(req.Messages[index].Content)
		}
	}
	return ""
}

func invocationChatResponseText(resp *adapter.ChatResponse) string {
	if resp == nil {
		return ""
	}
	var text strings.Builder
	for _, choice := range resp.Choices {
		appendText(&text, messageContentText(choice.Message.Content))
	}
	return text.String()
}
