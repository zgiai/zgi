package sensitiveredaction

import (
	"context"
	"strings"
	"testing"

	"github.com/zgiai/zgi/api/internal/modules/tools"
)

func TestProviderRegistersRedactTextTool(t *testing.T) {
	provider := NewProvider()
	entity := provider.GetEntity()
	if entity.Identity.Name != ProviderID {
		t.Fatalf("provider name = %q, want %q", entity.Identity.Name, ProviderID)
	}
	tool, err := provider.GetTool("redact_text")
	if err != nil {
		t.Fatalf("GetTool(redact_text) error = %v", err)
	}
	if got := tool.GetEntity().OutputType; got != "json" {
		t.Fatalf("output type = %q, want json", got)
	}
}

func TestRedactTextToolRedactsCommonSensitiveFields(t *testing.T) {
	tool := NewRedactTextTool("")
	text := strings.Join([]string{
		"姓名：张三",
		"客户：李四",
		"手机号：13812345678",
		"邮箱：zhangsan@example.com",
		"身份证：110105199001011234",
		"银行卡：4111111111111111",
		"合同号：HT-2026-0001",
		"订单号：ORD-88991234",
		"公司：星河科技有限公司",
		"地址：北京市朝阳区望京街道88号A座1201室",
		"IP：192.168.1.20",
		"Authorization: Bearer abcdef1234567890",
		"password: MyPassword123",
		"https://example.com/callback?token=abc123456&ok=1",
	}, "\n")
	messages, err := tool.Invoke(context.Background(), "user", map[string]interface{}{
		"text":  text,
		"level": "high",
	}, nil, nil, nil)
	if err != nil {
		t.Fatalf("Invoke() error = %v", err)
	}
	payload := firstPayload(t, messages)
	redacted := payload["redacted_text"].(string)
	for _, forbidden := range []string{
		"13812345678",
		"zhangsan@example.com",
		"110105199001011234",
		"4111111111111111",
		"abcdef1234567890",
		"MyPassword123",
		"abc123456",
	} {
		if strings.Contains(redacted, forbidden) {
			t.Fatalf("redacted text still contains %q: %s", forbidden, redacted)
		}
	}
	stats := payload["stats"].(map[string]int)
	for _, entityType := range []string{"phone", "email", "id_card", "bank_card", "token", "password", "url_parameter", "ip"} {
		if stats[entityType] == 0 {
			t.Fatalf("stats[%s] = 0, payload = %#v", entityType, payload)
		}
	}
	fields := payload["fields"].([]map[string]interface{})
	if len(fields) == 0 {
		t.Fatalf("fields empty")
	}
	for _, field := range fields {
		replacement, _ := field["replacement"].(string)
		for _, forbidden := range []string{"13812345678", "zhangsan@example.com", "abcdef1234567890", "MyPassword123"} {
			if strings.Contains(replacement, forbidden) {
				t.Fatalf("field replacement leaked original %q: %#v", forbidden, field)
			}
		}
	}
}

func TestRedactTextToolPartialStrategyPreservesAllowedContext(t *testing.T) {
	tool := NewRedactTextTool("")
	messages, err := tool.Invoke(context.Background(), "user", map[string]interface{}{
		"text":     "phone 13812345678 email alice@example.com",
		"level":    "medium",
		"strategy": "partial",
		"locale":   "zh-CN",
		"preserve_rules": map[string]interface{}{
			"keep_last_digits":  4,
			"keep_email_domain": true,
		},
	}, nil, nil, nil)
	if err != nil {
		t.Fatalf("Invoke() error = %v", err)
	}
	redacted := firstPayload(t, messages)["redacted_text"].(string)
	if !strings.Contains(redacted, "138****5678") {
		t.Fatalf("redacted phone = %q, want partial phone", redacted)
	}
	if !strings.Contains(redacted, "a****@example.com") {
		t.Fatalf("redacted email = %q, want domain-preserving email", redacted)
	}
}

func TestRedactTextToolRedactsSeparatedPhoneNumbers(t *testing.T) {
	tool := NewRedactTextTool("")
	messages, err := tool.Invoke(context.Background(), "user", map[string]interface{}{
		"text": "phones: 138-1234-5678, 139 1234 5678, +86 137 1234 5678",
		"preserve_rules": map[string]interface{}{
			"keep_last_digits": 4,
		},
	}, nil, nil, nil)
	if err != nil {
		t.Fatalf("Invoke() error = %v", err)
	}
	payload := firstPayload(t, messages)
	redacted := payload["redacted_text"].(string)
	for _, forbidden := range []string{"138-1234-5678", "139 1234 5678", "+86 137 1234 5678"} {
		if strings.Contains(redacted, forbidden) {
			t.Fatalf("redacted text still contains %q: %s", forbidden, redacted)
		}
	}
	stats := payload["stats"].(map[string]int)
	if stats["phone"] != 3 {
		t.Fatalf("stats[phone] = %d, want 3; redacted = %q", stats["phone"], redacted)
	}
}

func TestRedactTextToolCanHideURLDomainAndPath(t *testing.T) {
	tool := NewRedactTextTool("")
	messages, err := tool.Invoke(context.Background(), "user", map[string]interface{}{
		"text": "callback https://example.com/private/path?token=abc123456&ok=1",
		"preserve_rules": map[string]interface{}{
			"keep_url_domain": false,
		},
	}, nil, nil, nil)
	if err != nil {
		t.Fatalf("Invoke() error = %v", err)
	}
	payload := firstPayload(t, messages)
	redacted := payload["redacted_text"].(string)
	for _, forbidden := range []string{"example.com", "/private/path", "abc123456"} {
		if strings.Contains(redacted, forbidden) {
			t.Fatalf("redacted text still contains %q: %s", forbidden, redacted)
		}
	}
	if !strings.Contains(redacted, "[REDACTED_URL]") {
		t.Fatalf("redacted URL = %q, want whole URL placeholder", redacted)
	}
	stats := payload["stats"].(map[string]int)
	if stats["url_parameter"] != 1 {
		t.Fatalf("stats[url_parameter] = %d, want 1", stats["url_parameter"])
	}
}

func TestRedactTextToolEntityFilterLimitsDetection(t *testing.T) {
	tool := NewRedactTextTool("")
	messages, err := tool.Invoke(context.Background(), "user", map[string]interface{}{
		"text":         "phone 13812345678 email alice@example.com",
		"entity_types": []interface{}{"email"},
	}, nil, nil, nil)
	if err != nil {
		t.Fatalf("Invoke() error = %v", err)
	}
	redacted := firstPayload(t, messages)["redacted_text"].(string)
	if !strings.Contains(redacted, "13812345678") {
		t.Fatalf("phone should remain when filtered out: %q", redacted)
	}
	if strings.Contains(redacted, "alice@example.com") {
		t.Fatalf("email should be redacted: %q", redacted)
	}
}

func TestRedactTextToolInvalidInputReturnsClearError(t *testing.T) {
	tool := NewRedactTextTool("")
	tests := []struct {
		name   string
		params map[string]interface{}
		want   string
	}{
		{name: "missing text", params: map[string]interface{}{}, want: "text is required"},
		{name: "invalid level", params: map[string]interface{}{"text": "hello", "level": "extreme"}, want: "level must be"},
		{name: "invalid strategy", params: map[string]interface{}{"text": "hello", "strategy": "blur"}, want: "strategy must be"},
		{name: "invalid preserve", params: map[string]interface{}{"text": "hello", "preserve_rules": "not-json"}, want: "preserve_rules"},
		{name: "invalid entity", params: map[string]interface{}{"text": "hello", "entity_types": "unknown"}, want: "unsupported entity type"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := tool.Invoke(context.Background(), "user", tt.params, nil, nil, nil)
			if err == nil || !strings.Contains(err.Error(), tt.want) {
				t.Fatalf("Invoke() error = %v, want %q", err, tt.want)
			}
		})
	}
}

func firstPayload(t *testing.T, messages []tools.ToolInvokeMessage) map[string]interface{} {
	t.Helper()
	if len(messages) != 1 {
		t.Fatalf("messages = %d, want 1", len(messages))
	}
	if messages[0].Type != tools.ToolInvokeMessageTypeJSON {
		t.Fatalf("message type = %s, want json", messages[0].Type)
	}
	return messages[0].Data
}
