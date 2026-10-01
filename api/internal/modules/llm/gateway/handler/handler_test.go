package handler

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	llmerrors "github.com/zgiai/zgi/api/internal/modules/llm/errors"
	"github.com/zgiai/zgi/api/internal/modules/llm/gateway"
	adapter "github.com/zgiai/zgi/api/internal/modules/llm/protocol/adapters"
	"github.com/zgiai/zgi/api/internal/observability"
	"github.com/zgiai/zgi/api/pkg/database"
)

func TestClassifyProtocolErrorPreservesUpstreamGuardCode(t *testing.T) {
	got := classifyProtocolError(llmerrors.DomainErrPrivateChannelUpstreamUnavailable)
	// The status follows the frozen catalog entry for
	// llm.channel.private_unavailable (502, retryable); the legacy wire code
	// stays private_channel_upstream_unavailable so the guard remains
	// distinguishable from a generic invocation failure.
	if got.openAIStatus != http.StatusBadGateway {
		t.Fatalf("status = %d, want %d", got.openAIStatus, http.StatusBadGateway)
	}
	if got.openAICode != "private_channel_upstream_unavailable" {
		t.Fatalf("code = %q, want private_channel_upstream_unavailable", got.openAICode)
	}
	if got.appCode != "llm.channel.private_unavailable" {
		t.Fatalf("appCode = %q, want llm.channel.private_unavailable", got.appCode)
	}
}

func TestWriteRawProtocolErrorSanitizesProviderDetails(t *testing.T) {
	gin.SetMode(gin.TestMode)
	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	err := &adapter.RawProtocolError{
		StatusCode:  http.StatusTooManyRequests,
		ContentType: "application/json",
		Body:        []byte(`{"error":{"message":"provider dashscope rejected model qwen-upstream-2026","model":"qwen-upstream-2026","provider":"dashscope"}}`),
	}

	if !(&LLMHandler{}).writeRawProtocolError(c, err, "qwen-logical", "openai") {
		t.Fatal("raw protocol error was not handled")
	}
	if recorder.Code != http.StatusTooManyRequests {
		t.Fatalf("status = %d, want %d", recorder.Code, http.StatusTooManyRequests)
	}
	if got := recorder.Header().Get("Content-Type"); !strings.Contains(got, "application/json") {
		t.Fatalf("content type = %q, want application/json", got)
	}
	body := recorder.Body.String()
	for _, forbidden := range []string{"dashscope", "qwen-upstream-2026", `"provider"`} {
		if strings.Contains(body, forbidden) {
			t.Fatalf("public error leaked %q: %s", forbidden, body)
		}
	}
	if !strings.Contains(body, "qwen-logical") {
		t.Fatalf("public error missing logical model: %s", body)
	}
}

func TestWriteRawProtocolErrorReplacesNonJSONWithProtocolError(t *testing.T) {
	gin.SetMode(gin.TestMode)
	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	err := &adapter.RawProtocolError{
		StatusCode:  http.StatusBadGateway,
		ContentType: "text/plain",
		Body:        []byte("private provider host failed"),
	}

	if !(&LLMHandler{}).writeRawProtocolError(c, err, "claude-logical", "anthropic") {
		t.Fatal("raw protocol error was not handled")
	}
	body := recorder.Body.String()
	if recorder.Code != http.StatusBadGateway || !strings.Contains(body, `"type":"error"`) || !strings.Contains(body, "Upstream request failed") {
		t.Fatalf("fallback response = status %d body %s", recorder.Code, body)
	}
	if strings.Contains(body, "private provider host") {
		t.Fatalf("fallback response leaked raw body: %s", body)
	}
}

func TestClassifyProtocolErrorTreatsPricingFailureAsInternal(t *testing.T) {
	got := classifyProtocolError(fmt.Errorf("quote failed: %w", gateway.ErrPricingCalculationFailed))
	if got.openAIStatus != http.StatusBadGateway || got.openAICode != "internal_error" {
		t.Fatalf("protocol error = %#v, want internal error published as the catalog's 502", got)
	}
}

func TestServiceFailureReportHintPreservesPreSelectionDatabaseOwnership(t *testing.T) {
	err := gateway.NewProviderSelectionConversionError(
		&net.OpError{Op: "read", Net: "tcp", Err: errors.New("connection reset")},
	)
	hint := serviceFailureReportHint(err)

	if hint.Suppress {
		t.Fatal("database failure hint was suppressed")
	}
	classification := hint.Classification
	if classification.Source != observability.ErrorSourceInfrastructure ||
		classification.Category != observability.ErrorCategoryDatabase ||
		classification.Code != "database_transport_failed" ||
		!classification.Retryable {
		t.Fatalf("classification = %#v", classification)
	}
}

func TestServiceFailureReportHintPreservesModelListDatabaseOwnership(t *testing.T) {
	err := database.WrapOperationError("list models", context.DeadlineExceeded)
	hint := serviceFailureReportHint(err)

	classification := hint.Classification
	if hint.Suppress || classification.Source != observability.ErrorSourceInfrastructure ||
		classification.Category != observability.ErrorCategoryTimeout ||
		classification.Code != "database_timeout" || !classification.Retryable {
		t.Fatalf("hint = %#v", hint)
	}
}

func TestServiceFailureReportHintSuppressesAlreadyReportedProviderFailure(t *testing.T) {
	err := gateway.NewReportedProviderFailureError(context.DeadlineExceeded)
	if hint := serviceFailureReportHint(err); !hint.Suppress {
		t.Fatalf("hint = %#v, want already-reported provider failure suppressed", hint)
	}
}

func TestStreamFailureReportHintSuppressesDeterministicRejections(t *testing.T) {
	for _, err := range []error{
		adapter.ErrInvalidRequest,
		adapter.ErrContentPolicyViolation,
		adapter.ErrCapabilityUnsupported,
	} {
		if hint := streamFailureReportHint(err); !hint.Suppress {
			t.Fatalf("hint for %v = %#v, want suppressed", err, hint)
		}
	}
}

func TestStreamFailureReportHintDoesNotSuppressUntypedUnsupportedMessage(t *testing.T) {
	hint := streamFailureReportHint(errors.New("upstream operation not supported during maintenance"))
	if hint.Suppress || hint.Classification.Source != observability.ErrorSourceProvider {
		t.Fatalf("hint = %#v, want provider failure reported", hint)
	}
}

func TestServiceFailureReportHintSuppressesExpectedTenantQuotaErrors(t *testing.T) {
	for _, err := range []error{
		gateway.ErrInsufficientQuota,
		gateway.ErrInsufficientBalance,
		llmerrors.DomainErrInsufficientBalance,
		llmerrors.DomainErrRateLimitExceeded,
		adapter.ErrInsufficientBalance,
		adapter.ErrQuotaExhausted,
		&gateway.BillingUserError{Kind: gateway.BillingUserErrorKindModelPricingNotConfigured, Cause: gateway.ErrPricingNotConfigured},
	} {
		if hint := serviceFailureReportHint(fmt.Errorf("request failed: %w", err)); !hint.Suppress {
			t.Fatalf("hint for %v = %#v, want suppressed tenant quota failure", err, hint)
		}
	}
}

func TestFailureReportHintsSuppressGatewayOwnedChannelFailures(t *testing.T) {
	for _, err := range []error{adapter.ErrAuthFailed, adapter.ErrInvalidConfig, adapter.ErrBillingUnavailable} {
		wrapped := fmt.Errorf("gateway attempt failed: %w", err)
		if hint := serviceFailureReportHint(wrapped); !hint.Suppress {
			t.Fatalf("service hint for %v = %#v, want Gateway-owned fallback suppressed", err, hint)
		}
		if hint := streamFailureReportHint(wrapped); !hint.Suppress {
			t.Fatalf("stream hint for %v = %#v, want Gateway-owned fallback suppressed", err, hint)
		}
	}
}

func TestServiceFailureReportHintPreservesProviderRateLimit(t *testing.T) {
	hint := serviceFailureReportHint(adapter.ErrRateLimited)
	if hint.Suppress || hint.Classification.Source != observability.ErrorSourceProvider || !hint.Classification.Retryable {
		t.Fatalf("hint = %#v, want retryable provider rate limit", hint)
	}
}

func TestServiceFailureReportHintSuppressesNoProviderFallback(t *testing.T) {
	err := gateway.NewNoProviderAvailableError("qwen-plus", "org-1")
	if hint := serviceFailureReportHint(err); !hint.Suppress {
		t.Fatalf("hint = %#v, want selection fallback suppressed", hint)
	}

	providerHint := serviceFailureReportHint(adapter.ErrPlatformChannelUnavailable)
	if providerHint.Suppress || providerHint.Classification.Source != observability.ErrorSourceProvider {
		t.Fatalf("provider hint = %#v, want real provider outage reported", providerHint)
	}
	upstreamHint := serviceFailureReportHint(llmerrors.DomainErrUpstreamUnavailable)
	if upstreamHint.Suppress || upstreamHint.Classification.Source != observability.ErrorSourceProvider {
		t.Fatalf("upstream hint = %#v, want upstream outage reported", upstreamHint)
	}
}

func TestStreamFailureReportHintClassifiesProviderAndGatewayFailures(t *testing.T) {
	providerHint := streamFailureReportHint(adapter.NormalizeTransportError(context.DeadlineExceeded))
	if providerHint.Suppress || providerHint.Classification.Source != observability.ErrorSourceProvider || providerHint.Classification.Category != observability.ErrorCategoryTimeout {
		t.Fatalf("provider hint = %#v", providerHint)
	}

	gatewayHint := streamFailureReportHint(fmt.Errorf("settlement: %w", gateway.ErrBillingSettleFailed))
	if gatewayHint.Suppress || gatewayHint.Classification.Source != observability.ErrorSourceZGI || gatewayHint.Classification.Code != "stream_finalization_failed" {
		t.Fatalf("gateway hint = %#v", gatewayHint)
	}

	pricingHint := streamFailureReportHint(fmt.Errorf("quote: %w", gateway.ErrPricingCalculationFailed))
	if pricingHint.Classification.Source != observability.ErrorSourceZGI || pricingHint.Classification.Code != "stream_finalization_failed" {
		t.Fatalf("pricing hint = %#v", pricingHint)
	}
}

func TestRecordStreamServiceErrorPreservesReportHint(t *testing.T) {
	gin.SetMode(gin.TestMode)
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	want := fmt.Errorf("provider transport failed")

	recordStreamServiceError(c, want)

	if len(c.Errors) != 1 || !errors.Is(c.Errors[0].Err, want) {
		t.Fatalf("gin errors = %#v, want concrete stream error", c.Errors)
	}
	hint, ok := c.Errors[0].Meta.(observability.FailureReportHint)
	if !ok || hint.Classification.Source != observability.ErrorSourceProvider {
		t.Fatalf("report hint = %#v, want provider classification", c.Errors[0].Meta)
	}
}

func TestRecordServiceErrorPreservesConcreteFailureForOuterMiddleware(t *testing.T) {
	gin.SetMode(gin.TestMode)
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	want := fmt.Errorf("provider failed: %w", adapter.ErrUpstreamError)

	recordServiceError(c, want)

	if len(c.Errors) != 1 || !errors.Is(c.Errors[0].Err, want) {
		t.Fatalf("gin errors = %#v, want concrete service error", c.Errors)
	}
	hint, ok := c.Errors[0].Meta.(observability.FailureReportHint)
	if !ok || hint.EventName != "llm.request.failed" || hint.Classification.Source != observability.ErrorSourceProvider || hint.Classification.Code != "upstream_request_failed" {
		t.Fatalf("report hint = %#v, want provider classification", c.Errors[0].Meta)
	}
}

func TestExtractGatewayAPIKeySupportsGeminiHeaderAndRejectsConflicts(t *testing.T) {
	gin.SetMode(gin.TestMode)
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = httptest.NewRequest(http.MethodPost, "/v1beta/models/gemini-2.5-pro:generateContent", nil)
	c.Request.Header.Set("x-goog-api-key", "pre-key")

	key, message, ok := extractGatewayAPIKey(c)
	if !ok || key != "pre-key" || message != "" {
		t.Fatalf("key=%q message=%q ok=%t", key, message, ok)
	}

	c.Request.Header.Set("Authorization", "Bearer another-key")
	if _, message, ok := extractGatewayAPIKey(c); ok || message != "Conflicting API key headers" {
		t.Fatalf("conflicting headers: message=%q ok=%t", message, ok)
	}
}

func TestParseGeminiGenerateContentRequestUsesNativePathAndBody(t *testing.T) {
	gin.SetMode(gin.TestMode)
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = httptest.NewRequest(http.MethodPost, "/v1beta/models/gemini-logical:streamGenerateContent?alt=sse", strings.NewReader(`{"contents":[{"parts":[{"text":"hello"}]}]}`))
	c.Params = gin.Params{{Key: "modelAction", Value: "/gemini-logical:streamGenerateContent"}}

	req, err := parseGeminiGenerateContentRequest(c)
	if err != nil {
		t.Fatal(err)
	}
	if req.Model != "gemini-logical" || !req.Stream || !strings.Contains(string(req.Body), `"contents"`) {
		t.Fatalf("request = %#v body=%s", req, req.Body)
	}
}
