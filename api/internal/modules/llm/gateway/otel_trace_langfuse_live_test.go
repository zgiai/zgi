package gateway

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/shopspring/decimal"
	"github.com/zgiai/zgi/api/config"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracehttp"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/sdk/trace/tracetest"
)

func TestTraceChatCompletionLangfuseLive(t *testing.T) {
	if os.Getenv("ZGI_LANGFUSE_LIVE_TEST") != "1" {
		t.Skip("set ZGI_LANGFUSE_LIVE_TEST=1 to send a controlled trace to a dedicated Langfuse test project")
	}

	baseURL := strings.TrimSpace(os.Getenv("LANGFUSE_BASE_URL"))
	if baseURL == "" {
		baseURL = strings.TrimSpace(os.Getenv("LANGFUSE_HOST"))
	}
	publicKey := strings.TrimSpace(os.Getenv("LANGFUSE_PUBLIC_KEY"))
	secretKey := strings.TrimSpace(os.Getenv("LANGFUSE_SECRET_KEY"))
	if baseURL == "" || publicKey == "" || secretKey == "" {
		t.Fatal("LANGFUSE_BASE_URL, LANGFUSE_PUBLIC_KEY, and LANGFUSE_SECRET_KEY are required")
	}

	exportContext, cancelExport := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancelExport()
	auth := base64.StdEncoding.EncodeToString([]byte(publicKey + ":" + secretKey))
	exporter, err := otlptracehttp.New(
		exportContext,
		otlptracehttp.WithEndpointURL(langfuseLiveTraceEndpoint(baseURL)),
		otlptracehttp.WithHeaders(map[string]string{
			"Authorization":                "Basic " + auth,
			"x-langfuse-ingestion-version": "4",
		}),
	)
	if err != nil {
		t.Fatalf("create Langfuse OTLP exporter: %v", err)
	}

	recorder := tracetest.NewSpanRecorder()
	tp := sdktrace.NewTracerProvider(
		sdktrace.WithSpanProcessor(recorder),
		sdktrace.WithBatcher(exporter),
	)
	previousTracerProvider := otel.GetTracerProvider()
	previousConfig := config.GlobalConfig
	otelConfig := enabledGatewayOTELConfig()
	otelConfig.LLMCaptureContent = otelLLMCaptureNone
	config.GlobalConfig = &config.Config{OpenTelemetry: otelConfig}
	otel.SetTracerProvider(tp)
	defer func() {
		otel.SetTracerProvider(previousTracerProvider)
		config.GlobalConfig = previousConfig
		shutdownContext, shutdownCancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer shutdownCancel()
		if err := tp.Shutdown(shutdownContext); err != nil {
			t.Errorf("shutdown Langfuse tracer provider: %v", err)
		}
	}()

	testStartedAt := time.Now().UTC()
	testRunID := "zgi-pr01-" + uuid.NewString()
	result := executeSuccessfulChatAttempt(t, testRunID)
	if result.response != result.expectedResponse || result.providerAdapter.chatCalls != 1 {
		t.Fatal("controlled Provider success path did not complete exactly once")
	}
	if err := tp.ForceFlush(exportContext); err != nil {
		t.Fatalf("flush Langfuse trace: %v", err)
	}

	ended := recorder.Ended()
	if len(ended) != 1 {
		t.Fatalf("ended spans = %d, want exactly 1 generation", len(ended))
	}
	span := ended[0]
	if span.Name() != "llm.chat" {
		t.Fatalf("span name = %q, want llm.chat", span.Name())
	}
	traceID := span.SpanContext().TraceID().String()
	readbackContext, cancelReadback := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancelReadback()
	observation, err := waitForLangfuseLiveObservation(
		readbackContext,
		baseURL,
		publicKey,
		secretKey,
		traceID,
		"llm.chat",
		testStartedAt,
	)
	if err != nil {
		t.Fatalf("read back Langfuse generation: %v", err)
	}
	auditLangfuseLiveObservation(t, observation, traceID, testRunID, result.billing.lastSettled)

	t.Logf("LANGFUSE_TEST_RUN_ID=%s", testRunID)
	t.Logf("LANGFUSE_TRACE_ID=%s", traceID)
	t.Logf("LANGFUSE_OBSERVATION_ID=%s", observation.ID)
	if traceURL := langfuseLiveTraceURL(baseURL, observation.ProjectID, traceID); traceURL != "" {
		t.Logf("LANGFUSE_TRACE_URL=%s", traceURL)
	}

	nativeStartedAt := time.Now().UTC()
	nativeRunID := "zgi-gateway-native-" + uuid.NewString()
	_, _, nativeBilling := successfulChatTraceFixture()
	nativeBilling.RequestID = nativeRunID
	nativeBilling.AttemptID = nativeRunID + ":1"
	(&llmGatewayServiceImpl{}).traceNativeLLMOperation(
		context.Background(),
		"llm.responses",
		"responses",
		json.RawMessage(`{"input":"do-not-export-native-input"}`),
		json.RawMessage(`{"output":"do-not-export-native-output"}`),
		nil,
		nativeStartedAt,
		time.Time{},
		nativeBilling,
		nil,
	)
	nativeFlushContext, cancelNativeFlush := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancelNativeFlush()
	if err := tp.ForceFlush(nativeFlushContext); err != nil {
		t.Fatalf("flush native Langfuse trace: %v", err)
	}

	ended = recorder.Ended()
	if len(ended) != 2 {
		t.Fatalf("ended spans after native trace = %d, want exactly 2 total generations", len(ended))
	}
	nativeSpan := ended[1]
	if nativeSpan.Name() != "llm.responses" {
		t.Fatalf("native span name = %q, want llm.responses", nativeSpan.Name())
	}
	nativeTraceID := nativeSpan.SpanContext().TraceID().String()
	nativeReadbackContext, cancelNativeReadback := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancelNativeReadback()
	nativeObservation, err := waitForLangfuseLiveObservation(
		nativeReadbackContext,
		baseURL,
		publicKey,
		secretKey,
		nativeTraceID,
		"llm.responses",
		nativeStartedAt,
	)
	if err != nil {
		t.Fatalf("read back native Langfuse generation: %v", err)
	}
	auditNativeLangfuseLiveObservation(t, nativeObservation, nativeTraceID, nativeRunID)
	t.Logf("LANGFUSE_NATIVE_TEST_RUN_ID=%s", nativeRunID)
	t.Logf("LANGFUSE_NATIVE_TRACE_ID=%s", nativeTraceID)
	if traceURL := langfuseLiveTraceURL(baseURL, nativeObservation.ProjectID, nativeTraceID); traceURL != "" {
		t.Logf("LANGFUSE_NATIVE_TRACE_URL=%s", traceURL)
	}
}

func TestLangfuseLiveTraceEndpoint(t *testing.T) {
	tests := map[string]string{
		"https://cloud.langfuse.com":                   "https://cloud.langfuse.com/api/public/otel/v1/traces",
		"https://langfuse.example.com/":                "https://langfuse.example.com/api/public/otel/v1/traces",
		"https://langfuse.example.com/api/public/otel": "https://langfuse.example.com/api/public/otel/v1/traces",
	}
	for input, want := range tests {
		if got := langfuseLiveTraceEndpoint(input); got != want {
			t.Fatalf("langfuseLiveTraceEndpoint(%q) = %q, want %q", input, got, want)
		}
	}
}

func TestFetchLangfuseLiveObservation(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		username, password, ok := r.BasicAuth()
		if !ok || username != "public-test" || password != "secret-test" {
			t.Error("Langfuse readback request did not use the expected Basic Auth credentials")
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		if r.URL.Path != "/api/public/v2/observations" {
			t.Errorf("readback path = %q, want /api/public/v2/observations", r.URL.Path)
		}
		if got := r.URL.Query().Get("traceId"); got != "trace-test" {
			t.Errorf("traceId = %q, want trace-test", got)
		}
		if got := r.URL.Query().Get("type"); got != "GENERATION" {
			t.Errorf("type = %q, want GENERATION", got)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{"data":[{"id":"observation-test","traceId":"trace-test","type":"GENERATION","name":"llm.chat"}],"meta":{"cursor":null}}`)
	}))
	defer server.Close()

	observation, found, err := fetchLangfuseLiveObservation(
		context.Background(),
		server.Client(),
		server.URL,
		"public-test",
		"secret-test",
		"trace-test",
		"llm.chat",
		time.Now().Add(-time.Minute),
	)
	if err != nil {
		t.Fatalf("fetchLangfuseLiveObservation() error = %v", err)
	}
	if !found || observation.ID != "observation-test" {
		t.Fatalf("observation = %+v, found = %v", observation, found)
	}
}

func TestLiveValuePresent(t *testing.T) {
	tests := []struct {
		name  string
		value interface{}
		want  bool
	}{
		{name: "nil", value: nil, want: false},
		{name: "empty string", value: "", want: false},
		{name: "json null", value: "null", want: false},
		{name: "empty object", value: map[string]interface{}{}, want: false},
		{name: "empty array", value: []interface{}{}, want: false},
		{name: "non-empty object", value: map[string]interface{}{"temperature": 0.2}, want: true},
		{name: "content", value: "protected", want: true},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if got := liveValuePresent(test.value); got != test.want {
				t.Fatalf("liveValuePresent(%v) = %t, want %t", test.value, got, test.want)
			}
		})
	}
}

func langfuseLiveTraceEndpoint(baseURL string) string {
	baseURL = langfuseLiveHostURL(baseURL)
	if strings.HasSuffix(baseURL, "/api/public/otel") {
		return baseURL + "/v1/traces"
	}
	return baseURL + "/api/public/otel/v1/traces"
}

type langfuseLiveObservation struct {
	ID                string                 `json:"id"`
	TraceID           string                 `json:"traceId"`
	ProjectID         string                 `json:"projectId"`
	Type              string                 `json:"type"`
	Name              string                 `json:"name"`
	TraceName         string                 `json:"traceName"`
	UserID            string                 `json:"userId"`
	SessionID         string                 `json:"sessionId"`
	Model             string                 `json:"model"`
	ProvidedModelName string                 `json:"providedModelName"`
	Input             interface{}            `json:"input"`
	Output            interface{}            `json:"output"`
	ModelParameters   interface{}            `json:"modelParameters"`
	UsageDetails      map[string]interface{} `json:"usageDetails"`
	CostDetails       map[string]interface{} `json:"costDetails"`
	TotalUsage        interface{}            `json:"totalUsage"`
	TotalCost         interface{}            `json:"totalCost"`
	Metadata          map[string]interface{} `json:"metadata"`
}

type langfuseLiveObservationResponse struct {
	Data []langfuseLiveObservation `json:"data"`
}

func waitForLangfuseLiveObservation(
	ctx context.Context,
	baseURL string,
	publicKey string,
	secretKey string,
	traceID string,
	observationName string,
	startedAt time.Time,
) (langfuseLiveObservation, error) {
	client := &http.Client{Timeout: 5 * time.Second}
	var lastErr error
	for {
		observation, found, err := fetchLangfuseLiveObservation(
			ctx,
			client,
			baseURL,
			publicKey,
			secretKey,
			traceID,
			observationName,
			startedAt,
		)
		if err == nil && found {
			return observation, nil
		}
		if err != nil {
			lastErr = err
			var statusError *langfuseLiveHTTPStatusError
			if errors.As(err, &statusError) && statusError.Code >= 400 && statusError.Code < 500 && statusError.Code != http.StatusRequestTimeout && statusError.Code != http.StatusTooManyRequests {
				return langfuseLiveObservation{}, err
			}
		}

		timer := time.NewTimer(500 * time.Millisecond)
		select {
		case <-ctx.Done():
			timer.Stop()
			if lastErr != nil {
				return langfuseLiveObservation{}, fmt.Errorf("timed out waiting for trace %s: %w", traceID, lastErr)
			}
			return langfuseLiveObservation{}, fmt.Errorf("timed out waiting for trace %s: %w", traceID, ctx.Err())
		case <-timer.C:
		}
	}
}

func fetchLangfuseLiveObservation(
	ctx context.Context,
	client *http.Client,
	baseURL string,
	publicKey string,
	secretKey string,
	traceID string,
	observationName string,
	startedAt time.Time,
) (langfuseLiveObservation, bool, error) {
	endpoint, err := url.Parse(langfuseLiveHostURL(baseURL) + "/api/public/v2/observations")
	if err != nil {
		return langfuseLiveObservation{}, false, fmt.Errorf("parse Langfuse readback endpoint: %w", err)
	}
	query := endpoint.Query()
	query.Set("fields", "core,basic,io,metadata,model,usage,trace_context")
	query.Set("traceId", traceID)
	query.Set("name", observationName)
	query.Set("type", "GENERATION")
	query.Set("limit", "10")
	query.Set("fromStartTime", startedAt.Add(-time.Minute).UTC().Format(time.RFC3339Nano))
	query.Set("toStartTime", time.Now().Add(time.Minute).UTC().Format(time.RFC3339Nano))
	endpoint.RawQuery = query.Encode()

	request, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint.String(), nil)
	if err != nil {
		return langfuseLiveObservation{}, false, fmt.Errorf("create Langfuse readback request: %w", err)
	}
	request.SetBasicAuth(publicKey, secretKey)
	request.Header.Set("Accept", "application/json")
	response, err := client.Do(request)
	if err != nil {
		return langfuseLiveObservation{}, false, fmt.Errorf("query Langfuse observations: %w", err)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		_, _ = io.Copy(io.Discard, io.LimitReader(response.Body, 1<<20))
		return langfuseLiveObservation{}, false, &langfuseLiveHTTPStatusError{Code: response.StatusCode}
	}

	decoder := json.NewDecoder(io.LimitReader(response.Body, 2<<20))
	decoder.UseNumber()
	var payload langfuseLiveObservationResponse
	if err := decoder.Decode(&payload); err != nil {
		return langfuseLiveObservation{}, false, fmt.Errorf("decode Langfuse observation response: %w", err)
	}
	matches := make([]langfuseLiveObservation, 0, 1)
	for _, observation := range payload.Data {
		if observation.TraceID == traceID && observation.Name == observationName && strings.EqualFold(observation.Type, "generation") {
			matches = append(matches, observation)
		}
	}
	if len(matches) > 1 {
		return langfuseLiveObservation{}, false, fmt.Errorf("Langfuse returned %d %s generations for trace %s", len(matches), observationName, traceID)
	}
	if len(matches) == 1 {
		return matches[0], true, nil
	}
	return langfuseLiveObservation{}, false, nil
}

type langfuseLiveHTTPStatusError struct {
	Code int
}

func (e *langfuseLiveHTTPStatusError) Error() string {
	return fmt.Sprintf("query Langfuse observations: HTTP %d", e.Code)
}

func auditLangfuseLiveObservation(
	t *testing.T,
	observation langfuseLiveObservation,
	traceID string,
	testRunID string,
	billing *BillingContext,
) {
	t.Helper()
	if observation.TraceID != traceID || observation.Name != "llm.chat" || !strings.EqualFold(observation.Type, "generation") {
		t.Fatalf(
			"Langfuse observation identity mismatch: id=%q traceId=%q name=%q type=%q",
			observation.ID,
			observation.TraceID,
			observation.Name,
			observation.Type,
		)
	}
	if observation.TraceName != "llm.chat" {
		t.Fatalf("Langfuse trace name = %q, want llm.chat", observation.TraceName)
	}
	modelName := observation.Model
	if modelName == "" {
		modelName = observation.ProvidedModelName
	}
	if modelName != "gpt-test" {
		t.Fatalf("Langfuse model = %q, want gpt-test", modelName)
	}
	if observation.ProjectID == "" {
		t.Fatal("Langfuse readback omitted projectId")
	}
	if observation.UserID != "key-test" {
		t.Fatalf("Langfuse userId = %q, want key-test", observation.UserID)
	}
	assertLangfuseLiveNumber(t, observation.UsageDetails["input"], "usage.input", "11")
	assertLangfuseLiveNumber(t, observation.UsageDetails["output"], "usage.output", "7")
	assertLangfuseLiveNumber(t, observation.UsageDetails["total"], "usage.total", "18")
	assertLangfuseLiveNumber(t, observation.TotalUsage, "totalUsage", "18")
	assertLangfuseLiveNumber(t, observation.CostDetails["total"], "cost.total", "0.000018")
	assertLangfuseLiveNumber(t, observation.TotalCost, "totalCost", "0.000018")
	assertLangfuseLiveString(t, observation.Metadata, "schema_version", llmTraceSchemaVersion)
	assertLangfuseLiveString(t, observation.Metadata, "invocation_id", testRunID)
	if billing == nil {
		t.Fatal("controlled Gateway path did not expose settled billing context")
	}
	assertLangfuseLiveString(t, observation.Metadata, "organization_id", billing.OrganizationID)
	assertLangfuseLiveNumber(
		t,
		langfuseLiveAttribute(observation.Metadata, "zgi.actual_credits"),
		"metadata.attributes.zgi.actual_credits",
		"18",
	)
	if got := fmt.Sprint(langfuseLiveAttribute(observation.Metadata, "zgi.status")); got != "success" {
		t.Fatalf("Langfuse metadata attribute zgi.status = %q, want success", got)
	}
	if liveValuePresent(observation.Input) || liveValuePresent(observation.Output) || liveValuePresent(observation.ModelParameters) {
		t.Fatalf(
			"capture=none produced protected fields: id=%q input_present=%t output_present=%t model_parameters_present=%t",
			observation.ID,
			liveValuePresent(observation.Input),
			liveValuePresent(observation.Output),
			liveValuePresent(observation.ModelParameters),
		)
	}
	encoded, err := json.Marshal(observation)
	if err != nil {
		t.Fatalf("encode Langfuse readback for content audit: %v", err)
	}
	for _, sentinel := range sensitiveTraceSentinels() {
		if strings.Contains(string(encoded), sentinel) {
			t.Fatalf("Langfuse readback leaked sensitive sentinel %q", sentinel)
		}
	}
}

func auditNativeLangfuseLiveObservation(
	t *testing.T,
	observation langfuseLiveObservation,
	traceID string,
	testRunID string,
) {
	t.Helper()
	if observation.TraceID != traceID || observation.Name != "llm.responses" || !strings.EqualFold(observation.Type, "generation") {
		t.Fatalf(
			"native Langfuse observation identity mismatch: id=%q traceId=%q name=%q type=%q",
			observation.ID,
			observation.TraceID,
			observation.Name,
			observation.Type,
		)
	}
	if observation.TraceName != "llm.responses" {
		t.Fatalf("native Langfuse trace name = %q, want llm.responses", observation.TraceName)
	}
	assertLangfuseLiveString(t, observation.Metadata, "invocation_id", testRunID)
	assertLangfuseLiveNumber(t, observation.UsageDetails["input"], "native usage.input", "11")
	assertLangfuseLiveNumber(t, observation.UsageDetails["output"], "native usage.output", "7")
	assertLangfuseLiveNumber(t, observation.UsageDetails["total"], "native usage.total", "18")
	if liveValuePresent(observation.Input) || liveValuePresent(observation.Output) || liveValuePresent(observation.ModelParameters) {
		t.Fatalf(
			"capture=none produced protected native fields: id=%q input_present=%t output_present=%t model_parameters_present=%t",
			observation.ID,
			liveValuePresent(observation.Input),
			liveValuePresent(observation.Output),
			liveValuePresent(observation.ModelParameters),
		)
	}
	encoded, err := json.Marshal(observation)
	if err != nil {
		t.Fatalf("encode native Langfuse readback for content audit: %v", err)
	}
	for _, sentinel := range []string{"do-not-export-native-input", "do-not-export-native-output"} {
		if strings.Contains(string(encoded), sentinel) {
			t.Fatalf("native Langfuse readback leaked sensitive sentinel %q", sentinel)
		}
	}
}

func assertLangfuseLiveString(t *testing.T, values map[string]interface{}, key string, want string) {
	t.Helper()
	if got, ok := values[key].(string); !ok || got != want {
		t.Fatalf("Langfuse metadata %s = %v, want %q", key, values[key], want)
	}
}

func assertLangfuseLiveNumber(t *testing.T, value interface{}, field string, want string) {
	t.Helper()
	got, err := decimal.NewFromString(fmt.Sprint(value))
	if err != nil {
		t.Fatalf("Langfuse %s = %v, want %s: %v", field, value, want, err)
	}
	wantDecimal := decimal.RequireFromString(want)
	if !got.Equal(wantDecimal) {
		t.Fatalf("Langfuse %s = %s, want %s", field, got, wantDecimal)
	}
}

func langfuseLiveAttribute(metadata map[string]interface{}, key string) interface{} {
	if attributes, ok := metadata["attributes"].(map[string]interface{}); ok {
		if value, exists := attributes[key]; exists {
			return value
		}
	}
	return metadata["attributes."+key]
}

func liveValuePresent(value interface{}) bool {
	if value == nil {
		return false
	}
	switch typed := value.(type) {
	case string:
		text := strings.TrimSpace(typed)
		return text != "" && text != "null"
	case map[string]interface{}:
		return len(typed) > 0
	case []interface{}:
		return len(typed) > 0
	default:
		return true
	}
}

func langfuseLiveTraceURL(baseURL string, projectID string, traceID string) string {
	if strings.TrimSpace(projectID) == "" || strings.TrimSpace(traceID) == "" {
		return ""
	}
	return langfuseLiveHostURL(baseURL) + "/project/" + url.PathEscape(projectID) + "/traces/" + url.PathEscape(traceID)
}

func langfuseLiveHostURL(baseURL string) string {
	baseURL = strings.TrimRight(strings.TrimSpace(baseURL), "/")
	for _, suffix := range []string{"/api/public/otel/v1/traces", "/api/public/otel"} {
		if strings.HasSuffix(baseURL, suffix) {
			return strings.TrimSuffix(baseURL, suffix)
		}
	}
	return baseURL
}
