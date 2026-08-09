package handler

import (
	"errors"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	llmerrors "github.com/zgiai/zgi/api/internal/modules/llm/errors"
	adapter "github.com/zgiai/zgi/api/internal/modules/llm/protocol/adapters"
	"github.com/zgiai/zgi/api/pkg/apperror"
	appcatalog "github.com/zgiai/zgi/api/pkg/apperror/catalog"
	apptransport "github.com/zgiai/zgi/api/pkg/apperror/transport"
)

func TestLocalizedProtocolErrorProjectsProviderTimeoutWithoutChangingOpenAIContract(t *testing.T) {
	handler := &LLMHandler{errorProjector: testApplicationErrorProjector(t)}
	context := testProtocolContext(t, "/v1/chat/completions", "zh-CN, en;q=0.8")
	secretCause := errors.Join(adapter.ErrTimeout, errors.New("provider token sk-secret"))

	got := handler.localizedProtocolError(context, secretCause)

	if got.openAIStatus != 504 || got.openAIType != "server_error" || got.openAICode != "upstream_timeout" {
		t.Fatalf("protocol contract changed: %#v", got)
	}
	if got.message != "大模型服务响应超时，请重试或选择其他模型。" {
		t.Fatalf("message = %q", got.message)
	}
	if strings.Contains(got.message, "sk-secret") {
		t.Fatalf("public message leaked cause: %q", got.message)
	}
}

func TestLocalizedProtocolErrorProjectsProviderTimeoutForAnthropic(t *testing.T) {
	handler := &LLMHandler{errorProjector: testApplicationErrorProjector(t)}
	context := testProtocolContext(t, "/anthropic/v1/messages", "zh-Hans")

	got := handler.localizedProtocolError(context, llmerrors.DomainErrUpstreamTimeout)

	if got.anthropicStatus != 504 || got.anthropicType != "timeout_error" {
		t.Fatalf("protocol contract changed: %#v", got)
	}
	if got.message != "大模型服务响应超时，请重试或选择其他模型。" {
		t.Fatalf("message = %q", got.message)
	}
}

func TestLocalizedProtocolErrorFallsBackToEnglishAndLeavesOtherErrorsAlone(t *testing.T) {
	handler := &LLMHandler{errorProjector: testApplicationErrorProjector(t)}
	context := testProtocolContext(t, "/v1/chat/completions", "fr-FR")

	timeout := handler.localizedProtocolError(context, adapter.ErrTimeout)
	if timeout.message != "The model service took too long to respond. Try again or choose another model." {
		t.Fatalf("fallback message = %q", timeout.message)
	}

	other := handler.localizedProtocolError(context, adapter.ErrContentPolicyViolation)
	want := classifyProtocolError(adapter.ErrContentPolicyViolation)
	if other != want {
		t.Fatalf("unmigrated error changed: got %#v, want %#v", other, want)
	}
}

func TestRecordServiceErrorAddsStableAppCodeAndPreservesCause(t *testing.T) {
	context := testProtocolContext(t, "/v1/chat/completions", "en-US")

	recordServiceError(context, adapter.ErrTimeout)

	if len(context.Errors) != 1 {
		t.Fatalf("recorded errors = %d", len(context.Errors))
	}
	recorded := context.Errors[0].Err
	if !apperror.IsCode(recorded, llmerrors.AppCodeProviderTimeout) {
		t.Fatalf("recorded error has no stable provider timeout code: %v", recorded)
	}
	if !errors.Is(recorded, adapter.ErrTimeout) {
		t.Fatalf("recorded error lost original cause: %v", recorded)
	}
}

func TestLocalizedChatStreamErrorSanitizesOnlyMigratedTimeout(t *testing.T) {
	handler := &LLMHandler{errorProjector: testApplicationErrorProjector(t)}
	context := testProtocolContext(t, "/v1/chat/completions", "zh-CN")
	timeout := errors.Join(adapter.ErrTimeout, errors.New("provider token sk-secret"))

	got := handler.localizedChatStreamError(context, timeout)
	if got != "大模型服务响应超时，请重试或选择其他模型。" || strings.Contains(got, "sk-secret") {
		t.Fatalf("timeout stream message = %q", got)
	}

	unmigrated := errors.New("legacy stream message")
	if got := handler.localizedChatStreamError(context, unmigrated); got != unmigrated.Error() {
		t.Fatalf("unmigrated stream message = %q", got)
	}
}

func BenchmarkLocalizedProtocolErrorProviderTimeout(b *testing.B) {
	handler := &LLMHandler{errorProjector: testApplicationErrorProjector(b)}
	context := testProtocolContext(b, "/v1/chat/completions", "zh-CN")

	b.ReportAllocs()
	for range b.N {
		_ = handler.localizedProtocolError(context, adapter.ErrTimeout)
	}
}

type testingTB interface {
	Helper()
	Fatal(args ...any)
}

func testApplicationErrorProjector(tb testingTB) *apptransport.Projector {
	tb.Helper()
	definitions := append(appcatalog.DefaultDefinitions(), llmerrors.CatalogDefinitions()...)
	productCatalog, err := appcatalog.New(appcatalog.LocaleEnglishUS, appcatalog.CodeInternal, definitions...)
	if err != nil {
		tb.Fatal(err)
	}
	projector, err := apptransport.NewProjector(productCatalog)
	if err != nil {
		tb.Fatal(err)
	}
	return projector
}

func testProtocolContext(tb testingTB, path, language string) *gin.Context {
	tb.Helper()
	gin.SetMode(gin.TestMode)
	request := httptest.NewRequest("POST", path, nil)
	request.Header.Set("Accept-Language", language)
	context, _ := gin.CreateTestContext(httptest.NewRecorder())
	context.Request = request
	return context
}
