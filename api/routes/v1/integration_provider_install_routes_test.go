package v1

import (
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/zgiai/zgi/api/internal/modules/integrations/core"
	wecomintegration "github.com/zgiai/zgi/api/internal/modules/integrations/providers/collaboration/wecom"
)

func TestProviderInstallResultRedirectContainsOnlyBoundedStatus(t *testing.T) {
	result, err := providerInstallResultRedirectURL(
		"https://cloud.vibeclass.cn/console/integrations?provider=wecom",
		integrations.ProviderInstallFlowSucceeded,
		"",
	)
	if err != nil {
		t.Fatal(err)
	}
	parsed, err := url.Parse(result)
	if err != nil {
		t.Fatal(err)
	}
	if parsed.Query().Get("provider") != "wecom" || parsed.Query().Get("install_status") != "succeeded" {
		t.Fatalf("redirect=%s", result)
	}
	for _, forbidden := range []string{"auth_code", "state", "permanent_code", "suite_ticket"} {
		if parsed.Query().Has(forbidden) {
			t.Fatalf("redirect leaked %s: %s", forbidden, result)
		}
	}
}

func TestProviderInstallResultRedirectRejectsUnsafeURL(t *testing.T) {
	if _, err := providerInstallResultRedirectURL("javascript:alert(1)", integrations.ProviderInstallFlowFailed, "auth_invalid"); err == nil {
		t.Fatal("unsafe provider-install result URL was accepted")
	}
}

func TestWeComCallbackVerificationDiagnosticFieldsRemainSafe(t *testing.T) {
	adapter, err := wecomintegration.NewSuite(nil, wecomintegration.SuiteConfig{
		SuiteID:            "ww_suite_sentinel",
		ProviderCorpID:     "ww_provider_sentinel",
		ProviderSecret:     "provider-secret-sentinel",
		SuiteSecret:        "suite-secret-sentinel",
		CallbackToken:      "token-secret-sentinel",
		EncodingAESKey:     "abcdefghijklmnopqrstuvwxyz0123456789ABCDEFG",
		InstallCallbackURL: "https://example.com/wecom/install/callback",
	}, nil)
	if err != nil {
		t.Fatal(err)
	}
	_, verificationErr := adapter.VerifySuiteCallbackURL("signature-secret", "timestamp", "nonce-secret", "ciphertext-secret")
	if verificationErr == nil {
		t.Fatal("expected callback verification failure")
	}

	fields := wecomCallbackVerificationDiagnosticFields("suite_command", verificationErr)
	if len(fields) != 3 {
		t.Fatalf("diagnostic fields=%#v", fields)
	}
	if fields["callback_kind"] != "suite_command" ||
		fields["failure_stage"] != "signature_mismatch" ||
		fields["error_code"] != integrations.ErrorCodeAuthInvalid {
		t.Fatalf("diagnostic fields=%#v", fields)
	}
	for _, forbidden := range []string{"error", "signature", "timestamp", "nonce", "echostr", "receiver_id", "suite_id"} {
		if _, exists := fields[forbidden]; exists {
			t.Fatalf("diagnostic fields contain forbidden key %q: %#v", forbidden, fields)
		}
	}
	serialized := fmt.Sprint(fields)
	for _, sentinel := range []string{
		"signature-secret", "nonce-secret", "ciphertext-secret", "ww_suite_sentinel", "ww_provider_sentinel",
		"suite-secret-sentinel", "token-secret-sentinel", "abcdefghijklmnopqrstuvwxyz0123456789ABCDEFG",
	} {
		if strings.Contains(serialized, sentinel) {
			t.Fatalf("diagnostic fields leaked sentinel %q: %s", sentinel, serialized)
		}
	}
}

func TestWriteWeComCallbackVerificationResponseReturnsExactPlaintext(t *testing.T) {
	recorder := httptest.NewRecorder()
	context, _ := gin.CreateTestContext(recorder)
	plaintext := "12345678901234567890123"

	writeWeComCallbackVerificationResponse(context, plaintext)

	if recorder.Code != http.StatusOK {
		t.Fatalf("status=%d", recorder.Code)
	}
	if recorder.Header().Get("Content-Type") != "text/plain; charset=utf-8" {
		t.Fatalf("content type=%q", recorder.Header().Get("Content-Type"))
	}
	if recorder.Body.String() != plaintext {
		t.Fatalf("body=%q want=%q", recorder.Body.String(), plaintext)
	}
}

func TestWeComInstallCallbackDiagnosticFieldsDoNotExposeCallbackSecrets(t *testing.T) {
	err := integrations.NewError(
		integrations.ErrorCodeUpstream,
		"provider install authorization could not be queued",
		errors.New("state=state-secret auth_code=authorization-secret"),
	)
	fields := wecomInstallCallbackDiagnosticFields(err)
	if len(fields) != 3 || fields["integration_id"] != wecomintegration.IntegrationID ||
		fields["callback_kind"] != "browser_return" || fields["error_code"] != integrations.ErrorCodeUpstream {
		t.Fatalf("diagnostic fields=%#v", fields)
	}
	serialized := fmt.Sprint(fields)
	for _, forbidden := range []string{"state-secret", "authorization-secret", "auth_code", "state="} {
		if strings.Contains(serialized, forbidden) {
			t.Fatalf("diagnostic fields leaked %q: %s", forbidden, serialized)
		}
	}
}
