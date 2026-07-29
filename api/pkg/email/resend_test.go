package email

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/zgiai/zgi/api/config"
)

func TestCodeMailTasksUseResendCompatibleContract(t *testing.T) {
	cwd, err := os.Getwd()
	if err != nil {
		t.Fatalf("getwd: %v", err)
	}
	moduleRoot := filepath.Join(cwd, "..", "..")
	if err := os.Chdir(moduleRoot); err != nil {
		t.Fatalf("chdir module root: %v", err)
	}
	t.Cleanup(func() { _ = os.Chdir(cwd) })

	var requests []EmailRequest
	var idempotencyKeys []string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var request EmailRequest
		if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
			t.Errorf("decode request: %v", err)
		}
		requests = append(requests, request)
		idempotencyKeys = append(idempotencyKeys, r.Header.Get("Idempotency-Key"))
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"id":"message-1"}`))
	}))
	defer server.Close()

	previous := Cfg
	Cfg = &config.Config{Email: config.EmailConfig{
		MailType:              "resend",
		MailDefaultSendFrom:   "ZGI <system@example.com>",
		MailTemplateBrandName: "ZGI",
		MailTemplateLogoUrl:   "https://example.com/logo.png",
		ResendAPIKey:          "test-key",
		ResendAPIURL:          server.URL,
	}}
	t.Cleanup(func() { Cfg = previous })

	requireNoError := func(name string, err error) {
		t.Helper()
		if err != nil {
			t.Fatalf("%s: %v", name, err)
		}
	}
	requireNoError("email code en", SendEmailCodeLoginMailTask(t.Context(), "en-US", "user@example.com", "123456", "login-en"))
	requireNoError("email code zh", SendEmailCodeLoginMailTask(t.Context(), "zh-Hans", "user@example.com", "123456", "login-zh"))
	requireNoError("deletion en", SendAccountDeletionCodeMailTask(t.Context(), "en-US", "user@example.com", "123456", "delete-en"))
	requireNoError("deletion zh", SendAccountDeletionCodeMailTask(t.Context(), "zh-Hans", "user@example.com", "123456", "delete-zh"))

	if len(requests) != 4 {
		t.Fatalf("request count = %d, want 4", len(requests))
	}
	for index, request := range requests {
		if request.From != "ZGI <system@example.com>" || len(request.To) != 1 || request.To[0] != "user@example.com" {
			t.Fatalf("request %d has wrong envelope: %#v", index, request)
		}
		if !strings.Contains(request.Html, "123456") || request.Subject == "" || idempotencyKeys[index] == "" {
			t.Fatalf("request %d missing code, subject, or idempotency key", index)
		}
	}
}

func TestResendCompatibleProviderContract(t *testing.T) {
	var gotPath string
	var gotAuthorization string
	var gotIdempotencyKey string
	var gotBody string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		gotAuthorization = r.Header.Get("Authorization")
		gotIdempotencyKey = r.Header.Get("Idempotency-Key")
		body, _ := io.ReadAll(r.Body)
		gotBody = string(body)
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusCreated)
		_, _ = w.Write([]byte(`{"id":"message-1"}`))
	}))
	defer server.Close()

	previous := Cfg
	Cfg = &config.Config{Email: config.EmailConfig{
		MailType:            "resend",
		MailDefaultSendFrom: "ZGI <system@example.com>",
		ResendAPIKey:        "test-key",
		ResendAPIURL:        server.URL + "/v1/",
	}}
	t.Cleanup(func() { Cfg = previous })

	err := SendEmailWithOptions(
		context.Background(),
		[]string{"user@example.com"},
		"Registration code",
		"<p>123456</p>",
		"text/html",
		SendOptions{IdempotencyKey: "register:challenge-1"},
	)
	if err != nil {
		t.Fatalf("SendEmailWithOptions() error = %v", err)
	}
	if gotPath != "/v1/emails" {
		t.Fatalf("path = %q, want /v1/emails", gotPath)
	}
	if gotAuthorization != "Bearer test-key" {
		t.Fatalf("Authorization = %q", gotAuthorization)
	}
	if gotIdempotencyKey != "register:challenge-1" {
		t.Fatalf("Idempotency-Key = %q", gotIdempotencyKey)
	}
	var request EmailRequest
	if err := json.Unmarshal([]byte(gotBody), &request); err != nil || request.Html != "<p>123456</p>" {
		t.Fatalf("request body does not use Resend-compatible html field: %s", gotBody)
	}
}

func TestResendCompatibleProviderRejectsEmptyMessageID(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{}`))
	}))
	defer server.Close()

	previous := Cfg
	Cfg = &config.Config{Email: config.EmailConfig{
		MailType:            "resend",
		MailDefaultSendFrom: "system@example.com",
		ResendAPIKey:        "test-key",
		ResendAPIURL:        server.URL,
	}}
	t.Cleanup(func() { Cfg = previous })

	err := SendEmail([]string{"user@example.com"}, "subject", "body")
	if err == nil || !strings.Contains(err.Error(), "empty message id") {
		t.Fatalf("SendEmail() error = %v, want empty message id", err)
	}
}

func TestResendCompatibleProviderParsesProxyErrorShape(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusTooManyRequests)
		_, _ = w.Write([]byte(`{"error":{"message":"rate limit exceeded"}}`))
	}))
	defer server.Close()

	previous := Cfg
	Cfg = &config.Config{Email: config.EmailConfig{
		MailType:            "resend",
		MailDefaultSendFrom: "system@example.com",
		ResendAPIKey:        "test-key",
		ResendAPIURL:        server.URL,
	}}
	t.Cleanup(func() { Cfg = previous })

	err := SendEmail([]string{"user@example.com"}, "subject", "body")
	if err == nil || !strings.Contains(err.Error(), "rate limit exceeded") {
		t.Fatalf("SendEmail() error = %v, want proxy error message", err)
	}
}
