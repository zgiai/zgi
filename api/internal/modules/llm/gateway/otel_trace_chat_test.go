package gateway

import (
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"sort"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/shopspring/decimal"
	"github.com/zgiai/zgi/api/config"
	apikeymodel "github.com/zgiai/zgi/api/internal/modules/llm/apikey/model"
	llmmodel "github.com/zgiai/zgi/api/internal/modules/llm/llmmodel/model"
	adapter "github.com/zgiai/zgi/api/internal/modules/llm/protocol/adapters"
	providermodel "github.com/zgiai/zgi/api/internal/modules/llm/provider/model"
	appLogger "github.com/zgiai/zgi/api/pkg/logger"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracehttp"
	"go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/sdk/trace/tracetest"
	oteltrace "go.opentelemetry.io/otel/trace"
	collectortracepb "go.opentelemetry.io/proto/otlp/collector/trace/v1"
	commonpb "go.opentelemetry.io/proto/otlp/common/v1"
	tracepb "go.opentelemetry.io/proto/otlp/trace/v1"
	"go.uber.org/zap"
	"google.golang.org/protobuf/proto"
)

func TestTraceChatCompletionDisabledProducesNoSpanOrAllocations(t *testing.T) {
	restoreConfig := setGatewayOTELConfig(t, config.OpenTelemetryConfig{})
	defer restoreConfig()

	recorder := tracetest.NewSpanRecorder()
	tp := trace.NewTracerProvider(trace.WithSpanProcessor(recorder))
	restoreTracerProvider := setGatewayTracerProvider(t, tp)
	defer restoreTracerProvider()

	service := &llmGatewayServiceImpl{}
	req, resp, billing := successfulChatTraceFixture()
	start := time.Unix(1_700_000_000, 0)
	end := start.Add(250 * time.Millisecond)
	baseContext := context.Background()
	if got := withLLMLangfuseTraceContext(baseContext, billing, "llm.chat"); got != baseContext {
		t.Fatal("disabled Langfuse context should be returned unchanged")
	}

	allocations := testing.AllocsPerRun(1000, func() {
		traceContext := withLLMLangfuseTraceContext(baseContext, billing, "llm.chat")
		service.traceChatCompletion(traceContext, req, resp, start, end, billing, nil)
	})
	if allocations != 0 {
		t.Fatalf("disabled trace allocations = %v, want 0", allocations)
	}
	if got := len(recorder.Ended()); got != 0 {
		t.Fatalf("disabled trace ended spans = %d, want 0", got)
	}
}

func TestTraceChatCompletionZeroSampleRateProducesNoSpanOrAllocations(t *testing.T) {
	otelConfig := enabledGatewayOTELConfig()
	otelConfig.TraceSampleRate = 0
	restoreConfig := setGatewayOTELConfig(t, otelConfig)
	defer restoreConfig()

	recorder := tracetest.NewSpanRecorder()
	tp := trace.NewTracerProvider(trace.WithSpanProcessor(recorder))
	restoreTracerProvider := setGatewayTracerProvider(t, tp)
	defer restoreTracerProvider()

	service := &llmGatewayServiceImpl{}
	req, resp, billing := successfulChatTraceFixture()
	start := time.Unix(1_700_000_000, 0)
	end := start.Add(250 * time.Millisecond)
	baseContext := context.Background()
	if got := withLLMLangfuseTraceContext(baseContext, billing, "llm.chat"); got != baseContext {
		t.Fatal("zero sample rate should return the context unchanged")
	}

	allocations := testing.AllocsPerRun(1000, func() {
		traceContext := withLLMLangfuseTraceContext(baseContext, billing, "llm.chat")
		service.traceChatCompletion(traceContext, req, resp, start, end, billing, nil)
	})
	if allocations != 0 {
		t.Fatalf("zero-sample trace allocations = %v, want 0", allocations)
	}
	if got := len(recorder.Ended()); got != 0 {
		t.Fatalf("zero-sample ended spans = %d, want 0", got)
	}
}

func TestTraceChatCompletionLeavesParentDecisionToSDKSampler(t *testing.T) {
	restoreConfig := setGatewayOTELConfig(t, enabledGatewayOTELConfig())
	defer restoreConfig()

	recorder := tracetest.NewSpanRecorder()
	tp := trace.NewTracerProvider(
		trace.WithSampler(trace.AlwaysSample()),
		trace.WithSpanProcessor(recorder),
	)
	restoreTracerProvider := setGatewayTracerProvider(t, tp)
	defer restoreTracerProvider()

	parent := oteltrace.NewSpanContext(oteltrace.SpanContextConfig{
		TraceID:    oteltrace.TraceID{1},
		SpanID:     oteltrace.SpanID{1},
		TraceFlags: 0,
		Remote:     true,
	})
	ctx := oteltrace.ContextWithRemoteSpanContext(context.Background(), parent)
	req, resp, billing := successfulChatTraceFixture()
	(&llmGatewayServiceImpl{}).traceChatCompletion(
		ctx,
		req,
		resp,
		time.Now().Add(-time.Millisecond),
		time.Now(),
		billing,
		nil,
	)

	if got := len(recorder.Ended()); got != 1 {
		t.Fatalf("AlwaysSample ended spans = %d, want 1", got)
	}
}

func TestTraceChatCompletionFractionalSamplingSkipsLocalUnsampledParent(t *testing.T) {
	otelConfig := enabledGatewayOTELConfig()
	otelConfig.TraceSampleRate = 0.5
	restoreConfig := setGatewayOTELConfig(t, otelConfig)
	defer restoreConfig()

	parentProvider := trace.NewTracerProvider(trace.WithSampler(trace.NeverSample()))
	_, parent := parentProvider.Tracer("test-parent").Start(context.Background(), "unsampled-parent")
	defer parent.End()
	defer func() { _ = parentProvider.Shutdown(context.Background()) }()
	ctx := oteltrace.ContextWithSpan(context.Background(), parent)

	recorder := tracetest.NewSpanRecorder()
	childProvider := trace.NewTracerProvider(
		trace.WithSampler(trace.ParentBased(trace.TraceIDRatioBased(0.5))),
		trace.WithSpanProcessor(recorder),
	)
	restoreTracerProvider := setGatewayTracerProvider(t, childProvider)
	defer restoreTracerProvider()

	service := &llmGatewayServiceImpl{}
	req, resp, billing := successfulChatTraceFixture()
	start := time.Unix(1_700_000_000, 0)
	end := start.Add(250 * time.Millisecond)
	allocations := testing.AllocsPerRun(1000, func() {
		traceContext := withLLMLangfuseTraceContext(ctx, billing, "llm.chat")
		service.traceChatCompletion(traceContext, req, resp, start, end, billing, nil)
	})
	if allocations != 0 {
		t.Fatalf("fractional unsampled trace allocations = %v, want 0", allocations)
	}
	if got := len(recorder.Ended()); got != 0 {
		t.Fatalf("fractional unsampled ended spans = %d, want 0", got)
	}
}

func TestTraceChatCompletionDefaultCaptureOmitsInputAndOutput(t *testing.T) {
	otelConfig := enabledGatewayOTELConfig()
	otelConfig.LLMCaptureContent = ""
	restoreConfig := setGatewayOTELConfig(t, otelConfig)
	defer restoreConfig()

	recorder := tracetest.NewSpanRecorder()
	tp := trace.NewTracerProvider(trace.WithSpanProcessor(recorder))
	restoreTracerProvider := setGatewayTracerProvider(t, tp)
	defer restoreTracerProvider()

	service := &llmGatewayServiceImpl{}
	req, resp, billing := successfulChatTraceFixture()
	service.traceChatCompletion(context.Background(), req, resp, time.Now().Add(-time.Millisecond), time.Now(), billing, nil)

	ended := recorder.Ended()
	if len(ended) != 1 {
		t.Fatalf("ended spans = %d, want 1", len(ended))
	}
	attrs := recordedAttributes(ended[0].Attributes())
	if _, ok := attrs["langfuse.observation.input"]; ok {
		t.Fatal("default capture emitted langfuse.observation.input")
	}
	if _, ok := attrs["langfuse.observation.output"]; ok {
		t.Fatal("default capture emitted langfuse.observation.output")
	}
	if _, ok := attrs["langfuse.observation.model.parameters"]; ok {
		t.Fatal("default capture emitted langfuse.observation.model.parameters")
	}
}

func TestTraceChatCompletionSuccessSpanContract(t *testing.T) {
	restoreConfig := setGatewayOTELConfig(t, enabledGatewayOTELConfig())
	defer restoreConfig()

	recorder := tracetest.NewSpanRecorder()
	tp := trace.NewTracerProvider(trace.WithSpanProcessor(recorder))
	restoreTracerProvider := setGatewayTracerProvider(t, tp)
	defer restoreTracerProvider()

	service := &llmGatewayServiceImpl{}
	req, resp, billing := successfulChatTraceFixture()
	start := time.Unix(1_700_000_000, 0)
	end := start.Add(250 * time.Millisecond)
	service.traceChatCompletion(context.Background(), req, resp, start, end, billing, nil)

	ended := recorder.Ended()
	if len(ended) != 1 {
		t.Fatalf("ended spans = %d, want 1", len(ended))
	}
	span := ended[0]
	if span.Name() != "llm.chat" {
		t.Fatalf("span name = %q, want llm.chat", span.Name())
	}
	if span.Status().Code != codes.Ok {
		t.Fatalf("span status = %v, want OK", span.Status().Code)
	}
	if got := span.EndTime().Sub(span.StartTime()); got != 250*time.Millisecond {
		t.Fatalf("span duration = %v, want 250ms", got)
	}

	attrs := recordedAttributes(span.Attributes())
	assertStringAttribute(t, attrs, "gen_ai.operation.name", "chat")
	assertStringAttribute(t, attrs, "gen_ai.system", "openai")
	assertStringAttribute(t, attrs, "gen_ai.request.model", "gpt-test")
	assertStringAttribute(t, attrs, "zgi.llm.schema_version", "v1")
	assertStringAttribute(t, attrs, "zgi.invocation_id", "request-test")
	assertStringAttribute(t, attrs, "zgi.status", "success")
	assertStringAttribute(t, attrs, "langfuse.trace.name", "llm.chat")
	assertStringAttribute(t, attrs, "langfuse.trace.metadata.schema_version", "v1")
	assertStringAttribute(t, attrs, "langfuse.trace.metadata.invocation_id", "request-test")
	assertStringAttribute(t, attrs, "langfuse.observation.type", "generation")
	assertStringAttribute(t, attrs, "langfuse.observation.model.name", "gpt-test")
	assertStringAttribute(t, attrs, "langfuse.user.id", "key-test")
	assertStringAttribute(t, attrs, "langfuse.session.id", "conversation-test")
	assertStringAttribute(t, attrs, "zgi.request_id", "request-test")
	assertStringAttribute(t, attrs, "zgi.organization_id", "organization-test")
	assertStringAttribute(t, attrs, "zgi.workspace_id", "workspace-test")
	assertStringAttribute(t, attrs, "zgi.app_id", "55555555-5555-5555-5555-555555555555")
	assertStringAttribute(t, attrs, "zgi.app_type", "agent")
	assertStringAttribute(t, attrs, "zgi.route_id", "33333333-3333-3333-3333-333333333333")
	assertStringAttribute(t, attrs, "zgi.channel_id", "44444444-4444-4444-4444-444444444444")
	assertIntAttribute(t, attrs, "zgi.actual_credits", 42)
	assertIntAttribute(t, attrs, "gen_ai.usage.input_tokens", 11)
	assertIntAttribute(t, attrs, "gen_ai.usage.output_tokens", 7)
	assertIntAttribute(t, attrs, "gen_ai.usage.total_tokens", 18)

	input := attrs["langfuse.observation.input"].Value.AsString()
	output := attrs["langfuse.observation.output"].Value.AsString()
	if strings.Contains(input, "do-not-export-user-content") || strings.Contains(output, "do-not-export-assistant-content") {
		t.Fatalf("summary capture leaked message content: input=%s output=%s", input, output)
	}
	if !strings.Contains(input, `"message_count":1`) || !strings.Contains(output, `"role":"assistant"`) {
		t.Fatalf("summary capture missing expected shape: input=%s output=%s", input, output)
	}
	if cost := attrs["langfuse.observation.cost_details"].Value.AsString(); !strings.Contains(cost, `"total":0.003`) {
		t.Fatalf("cost details = %s, want total 0.003", cost)
	}
	parameters := attrs["langfuse.observation.model.parameters"].Value.AsString()
	for _, safeFact := range []string{
		`"function_call_configured":true`,
		`"tool_choice_configured":true`,
		`"response_format_configured":true`,
		`"additional_parameter_count":1`,
	} {
		if !strings.Contains(parameters, safeFact) {
			t.Fatalf("model parameters = %s, missing %s", parameters, safeFact)
		}
	}
	assertNoSensitiveTraceValues(t, attrs)
}

func TestTraceChatCompletionExportsOTLP(t *testing.T) {
	otelConfig := enabledGatewayOTELConfig()
	otelConfig.LLMCaptureContent = otelLLMCaptureNone
	restoreConfig := setGatewayOTELConfig(t, otelConfig)
	defer restoreConfig()

	received := make(chan []byte, 1)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/traces" {
			t.Errorf("OTLP path = %q, want /v1/traces", r.URL.Path)
		}
		body, err := io.ReadAll(r.Body)
		if err != nil {
			t.Errorf("read OTLP body: %v", err)
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		received <- body
		w.Header().Set("Content-Type", "application/x-protobuf")
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	exporter, err := otlptracehttp.New(
		context.Background(),
		otlptracehttp.WithEndpointURL(server.URL+"/v1/traces"),
	)
	if err != nil {
		t.Fatalf("create OTLP exporter: %v", err)
	}
	tp := trace.NewTracerProvider(trace.WithBatcher(exporter))
	restoreTracerProvider := setGatewayTracerProvider(t, tp)
	defer restoreTracerProvider()

	result := executeSuccessfulChatAttempt(t, "request-otlp")
	if result.response != result.expectedResponse || result.providerAdapter.chatCalls != 1 {
		t.Fatal("Gateway success path did not return the controlled Provider response exactly once")
	}
	if result.billing.preDeductCalls != 1 || result.billing.settleCalls != 1 {
		t.Fatalf("billing calls pre-deduct=%d settle=%d, want 1 and 1", result.billing.preDeductCalls, result.billing.settleCalls)
	}
	flushContext, cancelFlush := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancelFlush()
	if err := tp.ForceFlush(flushContext); err != nil {
		t.Fatalf("force flush OTLP spans: %v", err)
	}

	var body []byte
	select {
	case body = <-received:
	case <-time.After(5 * time.Second):
		t.Fatal("timed out waiting for OTLP export")
	}

	exportRequest := &collectortracepb.ExportTraceServiceRequest{}
	if err := proto.Unmarshal(body, exportRequest); err != nil {
		t.Fatalf("decode OTLP export: %v", err)
	}
	if count := countOTLPGenerations(exportRequest); count != 1 {
		t.Fatalf("OTLP generation count = %d, want exactly 1", count)
	}
	span := findOTLPSpan(exportRequest, "llm.chat")
	if span == nil {
		t.Fatal("OTLP export did not contain llm.chat span")
	}
	attrs := otlpAttributes(span.Attributes)
	assertOTLPStringAttribute(t, attrs, "zgi.llm.schema_version", "v1")
	assertOTLPStringAttribute(t, attrs, "zgi.invocation_id", "request-otlp")
	assertOTLPStringAttribute(t, attrs, "langfuse.observation.type", "generation")
	assertOTLPStringAttribute(t, attrs, "langfuse.observation.model.name", "gpt-test")
	assertOTLPIntAttribute(t, attrs, "gen_ai.usage.total_tokens", 18)
	for _, key := range []string{
		"langfuse.observation.input",
		"langfuse.observation.output",
		"langfuse.observation.model.parameters",
	} {
		if _, ok := attrs[key]; ok {
			t.Fatalf("capture=none OTLP payload emitted %s", key)
		}
	}
	if result.billing.lastSettled == nil {
		t.Fatal("Gateway success path did not settle billing")
	}
	assertOTLPStringAttribute(t, attrs, "zgi.organization_id", result.billing.lastSettled.OrganizationID)
	assertOTLPIntAttribute(t, attrs, "zgi.actual_credits", 18)
	if cost := attrs["langfuse.observation.cost_details"]; cost == nil || !strings.Contains(cost.GetStringValue(), `"total":0.000018`) {
		t.Fatalf("OTLP cost details = %v, want total 0.000018", cost)
	}
	for _, sentinel := range sensitiveTraceSentinels() {
		if strings.Contains(string(body), sentinel) {
			t.Fatalf("OTLP payload leaked sensitive sentinel %q", sentinel)
		}
	}
}

func TestTraceChatCompletionDoesNotBlockOnExporterBackpressure(t *testing.T) {
	otelConfig := enabledGatewayOTELConfig()
	otelConfig.LLMCaptureContent = otelLLMCaptureNone
	restoreConfig := setGatewayOTELConfig(t, otelConfig)
	defer restoreConfig()

	exporter := newBlockingErrorExporter()
	tp := trace.NewTracerProvider(trace.WithBatcher(
		exporter,
		trace.WithMaxQueueSize(1),
		trace.WithMaxExportBatchSize(1),
		trace.WithBatchTimeout(time.Hour),
	))
	previousTracerProvider := otel.GetTracerProvider()
	previousErrorHandler := otel.GetErrorHandler()
	otel.SetTracerProvider(tp)
	otel.SetErrorHandler(otel.ErrorHandlerFunc(func(error) {}))
	defer func() {
		exporter.unblock()
		otel.SetTracerProvider(previousTracerProvider)
		otel.SetErrorHandler(previousErrorHandler)
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = tp.Shutdown(ctx)
	}()

	service := &llmGatewayServiceImpl{}
	req, resp, billing := successfulChatTraceFixture()
	service.traceChatCompletion(context.Background(), req, resp, time.Now().Add(-time.Millisecond), time.Now(), billing, nil)
	select {
	case <-exporter.started:
	case <-time.After(2 * time.Second):
		t.Fatal("batch exporter did not start")
	}

	done := make(chan struct{})
	go func() {
		defer close(done)
		for range 100 {
			service.traceChatCompletion(context.Background(), req, resp, time.Now().Add(-time.Millisecond), time.Now(), billing, nil)
		}
	}()
	select {
	case <-done:
	case <-time.After(500 * time.Millisecond):
		t.Fatal("Gateway trace path blocked on a stalled full exporter queue")
	}
}

func TestTryChatCompletionSucceedsWhenOTLPCollectorRejectsConnection(t *testing.T) {
	otelConfig := enabledGatewayOTELConfig()
	otelConfig.LLMCaptureContent = otelLLMCaptureNone
	restoreConfig := setGatewayOTELConfig(t, otelConfig)
	defer restoreConfig()

	server := httptest.NewServer(http.NotFoundHandler())
	endpoint := server.URL + "/v1/traces"
	server.Close()
	exporter, err := otlptracehttp.New(
		context.Background(),
		otlptracehttp.WithEndpointURL(endpoint),
		otlptracehttp.WithTimeout(250*time.Millisecond),
		otlptracehttp.WithRetry(otlptracehttp.RetryConfig{Enabled: false}),
	)
	if err != nil {
		t.Fatalf("create unavailable OTLP exporter: %v", err)
	}
	tp := trace.NewTracerProvider(trace.WithBatcher(
		exporter,
		trace.WithMaxQueueSize(8),
		trace.WithMaxExportBatchSize(1),
		trace.WithBatchTimeout(time.Millisecond),
	))
	previousTracerProvider := otel.GetTracerProvider()
	previousErrorHandler := otel.GetErrorHandler()
	exportErrors := make(chan error, 1)
	otel.SetTracerProvider(tp)
	otel.SetErrorHandler(otel.ErrorHandlerFunc(func(err error) {
		select {
		case exportErrors <- err:
		default:
		}
	}))
	defer func() {
		otel.SetTracerProvider(previousTracerProvider)
		otel.SetErrorHandler(previousErrorHandler)
		ctx, cancel := context.WithTimeout(context.Background(), time.Second)
		defer cancel()
		_ = tp.Shutdown(ctx)
	}()

	started := time.Now()
	result := executeSuccessfulChatAttempt(t, "request-collector-unavailable")
	if result.response != result.expectedResponse || result.providerAdapter.chatCalls != 1 {
		t.Fatal("collector failure changed the successful Gateway response")
	}
	if elapsed := time.Since(started); elapsed > 500*time.Millisecond {
		t.Fatalf("Gateway waited for unavailable collector: %v", elapsed)
	}
	select {
	case <-exportErrors:
	case <-time.After(2 * time.Second):
		t.Fatal("OTLP exporter did not report the expected connection failure")
	}
}

func TestTryChatCompletionSuccessReturnsResponseAndEmitsOneGeneration(t *testing.T) {
	otelConfig := enabledGatewayOTELConfig()
	otelConfig.LLMCaptureContent = otelLLMCaptureNone
	restoreConfig := setGatewayOTELConfig(t, otelConfig)
	defer restoreConfig()

	recorder := tracetest.NewSpanRecorder()
	tp := trace.NewTracerProvider(trace.WithSpanProcessor(recorder))
	restoreTracerProvider := setGatewayTracerProvider(t, tp)
	defer restoreTracerProvider()

	result := executeSuccessfulChatAttempt(t, "request-integration")
	if result.response != result.expectedResponse {
		t.Fatal("tryChatCompletion() did not return the provider response unchanged")
	}
	if result.providerAdapter.chatCalls != 1 {
		t.Fatalf("provider ChatCompletion calls = %d, want 1", result.providerAdapter.chatCalls)
	}
	if result.billing.preDeductCalls != 1 || result.billing.settleCalls != 1 {
		t.Fatalf("billing calls pre-deduct=%d settle=%d, want 1 and 1", result.billing.preDeductCalls, result.billing.settleCalls)
	}
	if result.billing.lastSettled == nil || result.billing.lastSettled.Status != "success" {
		t.Fatalf("settled billing context = %+v, want success", result.billing.lastSettled)
	}

	ended := recorder.Ended()
	if len(ended) != 1 {
		t.Fatalf("ended spans = %d, want exactly 1 generation", len(ended))
	}
	attrs := recordedAttributes(ended[0].Attributes())
	assertStringAttribute(t, attrs, "langfuse.observation.type", "generation")
	assertStringAttribute(t, attrs, "zgi.invocation_id", "request-integration")
	assertStringAttribute(t, attrs, "zgi.status", "success")
	assertIntAttribute(t, attrs, "zgi.actual_credits", 18)
	assertNoSensitiveTraceValues(t, attrs)
}

func BenchmarkTraceChatCompletionDisabled(b *testing.B) {
	previous := config.GlobalConfig
	config.GlobalConfig = &config.Config{}
	b.Cleanup(func() { config.GlobalConfig = previous })

	service := &llmGatewayServiceImpl{}
	req, resp, billing := successfulChatTraceFixture()
	start := time.Unix(1_700_000_000, 0)
	end := start.Add(250 * time.Millisecond)
	b.ReportAllocs()
	b.ResetTimer()
	for b.Loop() {
		service.traceChatCompletion(context.Background(), req, resp, start, end, billing, nil)
	}
}

func BenchmarkTraceChatCompletionEnabledCaptureNone(b *testing.B) {
	previousConfig := config.GlobalConfig
	otelConfig := enabledGatewayOTELConfig()
	otelConfig.LLMCaptureContent = otelLLMCaptureNone
	config.GlobalConfig = &config.Config{OpenTelemetry: otelConfig}
	b.Cleanup(func() { config.GlobalConfig = previousConfig })

	tp := trace.NewTracerProvider()
	previousTracerProvider := otel.GetTracerProvider()
	otel.SetTracerProvider(tp)
	b.Cleanup(func() {
		otel.SetTracerProvider(previousTracerProvider)
		_ = tp.Shutdown(context.Background())
	})

	service := &llmGatewayServiceImpl{}
	req, resp, billing := successfulChatTraceFixture()
	ctx := context.Background()
	start := time.Unix(1_700_000_000, 0)
	end := start.Add(250 * time.Millisecond)
	b.ReportAllocs()
	b.ResetTimer()
	for b.Loop() {
		service.traceChatCompletion(ctx, req, resp, start, end, billing, nil)
	}
}

func BenchmarkTraceChatCompletionEnabledBatchCaptureNone(b *testing.B) {
	previousConfig := config.GlobalConfig
	otelConfig := enabledGatewayOTELConfig()
	otelConfig.LLMCaptureContent = otelLLMCaptureNone
	config.GlobalConfig = &config.Config{OpenTelemetry: otelConfig}
	b.Cleanup(func() { config.GlobalConfig = previousConfig })

	tp := trace.NewTracerProvider(trace.WithBatcher(
		discardSpanExporter{},
		trace.WithMaxQueueSize(2048),
		trace.WithMaxExportBatchSize(512),
		trace.WithBatchTimeout(time.Second),
	))
	previousTracerProvider := otel.GetTracerProvider()
	otel.SetTracerProvider(tp)
	b.Cleanup(func() {
		otel.SetTracerProvider(previousTracerProvider)
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = tp.Shutdown(ctx)
	})

	service := &llmGatewayServiceImpl{}
	req, resp, billing := successfulChatTraceFixture()
	ctx := context.Background()
	start := time.Unix(1_700_000_000, 0)
	end := start.Add(250 * time.Millisecond)
	b.ReportAllocs()
	b.ResetTimer()
	for b.Loop() {
		service.traceChatCompletion(ctx, req, resp, start, end, billing, nil)
	}
}

func TestTryChatCompletionTelemetryLatencyComparison(t *testing.T) {
	if os.Getenv("ZGI_OTEL_PERF_TEST") != "1" {
		t.Skip("set ZGI_OTEL_PERF_TEST=1 to run the reproducible full Gateway latency comparison")
	}

	previousConfig := config.GlobalConfig
	previousTracerProvider := otel.GetTracerProvider()
	previousLogger := appLogger.L()
	appLogger.SetLogger(zap.NewNop())
	tp := trace.NewTracerProvider(trace.WithBatcher(
		discardSpanExporter{},
		trace.WithMaxQueueSize(2048),
		trace.WithMaxExportBatchSize(512),
		trace.WithBatchTimeout(time.Second),
	))
	otel.SetTracerProvider(tp)
	defer func() {
		config.GlobalConfig = previousConfig
		otel.SetTracerProvider(previousTracerProvider)
		appLogger.SetLogger(previousLogger)
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = tp.Shutdown(ctx)
	}()

	const samples = 200
	const providerDelay = 50 * time.Millisecond
	disabledDurations := make([]time.Duration, 0, samples)
	enabledDurations := make([]time.Duration, 0, samples)
	disabledConfig := config.OpenTelemetryConfig{}
	enabledConfig := enabledGatewayOTELConfig()
	enabledConfig.LLMCaptureContent = otelLLMCaptureNone

	measure := func(target *[]time.Duration, otelConfig config.OpenTelemetryConfig, requestID string) {
		config.GlobalConfig = &config.Config{OpenTelemetry: otelConfig}
		started := time.Now()
		result := executeSuccessfulChatAttemptWithDelay(t, requestID, providerDelay)
		*target = append(*target, time.Since(started))
		if result.response != result.expectedResponse || result.providerAdapter.chatCalls != 1 {
			t.Fatal("latency fixture did not complete exactly once")
		}
	}

	for index := range samples {
		if index%2 == 0 {
			measure(&disabledDurations, disabledConfig, "perf-disabled")
			measure(&enabledDurations, enabledConfig, "perf-enabled")
		} else {
			measure(&enabledDurations, enabledConfig, "perf-enabled")
			measure(&disabledDurations, disabledConfig, "perf-disabled")
		}
	}

	disabledP50, disabledP95, disabledP99 := latencyPercentiles(disabledDurations)
	enabledP50, enabledP95, enabledP99 := latencyPercentiles(enabledDurations)
	p50Delta := latencyDeltaPercent(disabledP50, enabledP50)
	p95Delta := latencyDeltaPercent(disabledP95, enabledP95)
	p99Delta := latencyDeltaPercent(disabledP99, enabledP99)
	t.Logf("disabled p50=%v p95=%v p99=%v", disabledP50, disabledP95, disabledP99)
	t.Logf("enabled  p50=%v p95=%v p99=%v", enabledP50, enabledP95, enabledP99)
	t.Logf("delta    p50=%.3f%% p95=%.3f%% p99=%.3f%%", p50Delta, p95Delta, p99Delta)
	if p95Delta >= 1 {
		t.Fatalf("enabled Gateway p95 regression = %.3f%%, want < 1%%", p95Delta)
	}
}

func enabledGatewayOTELConfig() config.OpenTelemetryConfig {
	return config.OpenTelemetryConfig{
		Enabled:               true,
		TraceSampleRate:       1,
		LLMLangfuseAttributes: true,
		LLMCaptureContent:     otelLLMCaptureSummary,
		LLMCaptureMaxChars:    65536,
	}
}

func setGatewayOTELConfig(t *testing.T, otelConfig config.OpenTelemetryConfig) func() {
	t.Helper()
	previous := config.GlobalConfig
	config.GlobalConfig = &config.Config{OpenTelemetry: otelConfig}
	return func() { config.GlobalConfig = previous }
}

func setGatewayTracerProvider(t *testing.T, provider *trace.TracerProvider) func() {
	t.Helper()
	previous := otel.GetTracerProvider()
	otel.SetTracerProvider(provider)
	return func() {
		otel.SetTracerProvider(previous)
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		if err := provider.Shutdown(ctx); err != nil {
			t.Errorf("shutdown tracer provider: %v", err)
		}
	}
}

func successfulChatTraceFixture() (*adapter.ChatRequest, *adapter.ChatResponse, *BillingContext) {
	modelID := uuid.MustParse("11111111-1111-1111-1111-111111111111")
	providerID := uuid.MustParse("22222222-2222-2222-2222-222222222222")
	req := &adapter.ChatRequest{
		Model: "gpt-test",
		Messages: []adapter.Message{{
			Role:    "user",
			Content: "do-not-export-user-content",
		}},
		FunctionCall: map[string]interface{}{"name": "do-not-export-function-call"},
		ToolChoice:   map[string]interface{}{"name": "do-not-export-tool-choice"},
		ResponseFormat: &adapter.ResponseFormat{
			Type:   "do-not-export-response-format-type",
			Schema: map[string]any{"description": "do-not-export-response-schema"},
		},
		AdditionalParameters: map[string]interface{}{
			"private_prompt": "do-not-export-additional-parameter",
		},
	}
	resp := &adapter.ChatResponse{
		Model: "gpt-test",
		Choices: []adapter.Choice{{
			Index: 0,
			Message: adapter.Message{
				Role:    "assistant",
				Content: "do-not-export-assistant-content",
			},
			FinishReason: "stop",
		}},
		Usage: &adapter.Usage{PromptTokens: 11, CompletionTokens: 7, TotalTokens: 18},
	}
	billing := &BillingContext{
		APIKeyID:         "key-test",
		OrganizationID:   "organization-test",
		AttemptID:        "request-test:1",
		ModelID:          modelID,
		ModelName:        "gpt-test",
		ProviderID:       providerID,
		ProviderName:     "openai",
		PromptTokens:     11,
		CompletionTokens: 7,
		TotalTokens:      18,
		InputUSD:         decimal.RequireFromString("0.001"),
		OutputUSD:        decimal.RequireFromString("0.002"),
		TotalUSD:         decimal.RequireFromString("0.003"),
		RequestID:        "request-test",
		ConversationID:   "conversation-test",
		ResponseTime:     250,
		ActualCredits:    42,
		Status:           "success",
	}
	routeID := uuid.MustParse("33333333-3333-3333-3333-333333333333")
	channelID := uuid.MustParse("44444444-4444-4444-4444-444444444444")
	appID := uuid.MustParse("55555555-5555-5555-5555-555555555555")
	appType := "agent"
	billing.RouteID = &routeID
	billing.ChannelID = &channelID
	billing.WorkspaceID = "workspace-test"
	billing.AppID = &appID
	billing.AppType = &appType
	return req, resp, billing
}

func sensitiveTraceSentinels() []string {
	return []string{
		"do-not-export-user-content",
		"do-not-export-assistant-content",
		"do-not-export-function-call",
		"do-not-export-tool-choice",
		"do-not-export-response-format-type",
		"do-not-export-response-schema",
		"do-not-export-additional-parameter",
		"provider-key-not-exported",
	}
}

func assertNoSensitiveTraceValues(t *testing.T, attrs map[string]attribute.KeyValue) {
	t.Helper()
	for key, attr := range attrs {
		var values []string
		switch attr.Value.Type() {
		case attribute.STRING:
			values = []string{attr.Value.AsString()}
		case attribute.STRINGSLICE:
			values = attr.Value.AsStringSlice()
		}
		for _, value := range values {
			for _, sentinel := range sensitiveTraceSentinels() {
				if strings.Contains(value, sentinel) {
					t.Fatalf("attribute %s leaked sensitive sentinel %q", key, sentinel)
				}
			}
		}
	}
}

func recordedAttributes(attrs []attribute.KeyValue) map[string]attribute.KeyValue {
	out := make(map[string]attribute.KeyValue, len(attrs))
	for _, attr := range attrs {
		out[string(attr.Key)] = attr
	}
	return out
}

func assertStringAttribute(t *testing.T, attrs map[string]attribute.KeyValue, key string, want string) {
	t.Helper()
	attr, ok := attrs[key]
	if !ok {
		t.Fatalf("missing attribute %s", key)
	}
	if got := attr.Value.AsString(); got != want {
		t.Fatalf("attribute %s = %q, want %q", key, got, want)
	}
}

func assertIntAttribute(t *testing.T, attrs map[string]attribute.KeyValue, key string, want int64) {
	t.Helper()
	attr, ok := attrs[key]
	if !ok {
		t.Fatalf("missing attribute %s", key)
	}
	if got := attr.Value.AsInt64(); got != want {
		t.Fatalf("attribute %s = %d, want %d", key, got, want)
	}
}

func findOTLPSpan(request *collectortracepb.ExportTraceServiceRequest, name string) *tracepb.Span {
	for _, resourceSpans := range request.ResourceSpans {
		for _, scopeSpans := range resourceSpans.ScopeSpans {
			for _, span := range scopeSpans.Spans {
				if span.Name == name {
					return span
				}
			}
		}
	}
	return nil
}

func countOTLPGenerations(request *collectortracepb.ExportTraceServiceRequest) int {
	count := 0
	for _, resourceSpans := range request.ResourceSpans {
		for _, scopeSpans := range resourceSpans.ScopeSpans {
			for _, span := range scopeSpans.Spans {
				attrs := otlpAttributes(span.Attributes)
				if observationType := attrs["langfuse.observation.type"]; observationType != nil && observationType.GetStringValue() == "generation" {
					count++
				}
			}
		}
	}
	return count
}

func otlpAttributes(attrs []*commonpb.KeyValue) map[string]*commonpb.AnyValue {
	out := make(map[string]*commonpb.AnyValue, len(attrs))
	for _, attr := range attrs {
		out[attr.Key] = attr.Value
	}
	return out
}

func assertOTLPStringAttribute(t *testing.T, attrs map[string]*commonpb.AnyValue, key string, want string) {
	t.Helper()
	attr, ok := attrs[key]
	if !ok {
		t.Fatalf("OTLP payload missing attribute %s", key)
	}
	if got := attr.GetStringValue(); got != want {
		t.Fatalf("OTLP attribute %s = %q, want %q", key, got, want)
	}
}

func assertOTLPIntAttribute(t *testing.T, attrs map[string]*commonpb.AnyValue, key string, want int64) {
	t.Helper()
	attr, ok := attrs[key]
	if !ok {
		t.Fatalf("OTLP payload missing attribute %s", key)
	}
	if got := attr.GetIntValue(); got != want {
		t.Fatalf("OTLP attribute %s = %d, want %d", key, got, want)
	}
}

type blockingErrorExporter struct {
	started     chan struct{}
	release     chan struct{}
	startedOnce sync.Once
	releaseOnce sync.Once
}

type chatTraceSuccessAdapter struct {
	response  *adapter.ChatResponse
	chatCalls int
	delay     time.Duration
}

type successfulChatAttemptResult struct {
	response         *adapter.ChatResponse
	expectedResponse *adapter.ChatResponse
	providerAdapter  *chatTraceSuccessAdapter
	billing          *chatTraceBillingProvider
}

func executeSuccessfulChatAttempt(t testing.TB, requestID string) successfulChatAttemptResult {
	return executeSuccessfulChatAttemptWithDelay(t, requestID, 0)
}

func executeSuccessfulChatAttemptWithDelay(t testing.TB, requestID string, delay time.Duration) successfulChatAttemptResult {
	t.Helper()
	req, expectedResponse, _ := successfulChatTraceFixture()
	providerAdapter := &chatTraceSuccessAdapter{response: expectedResponse, delay: delay}
	factory := adapter.NewDefaultAdapterFactory()
	factory.Register("openai", func(*adapter.AdapterConfig) (adapter.LLMProviderAdapter, error) {
		return providerAdapter, nil
	})
	billing := &chatTraceBillingProvider{canSpend: true}
	pricing := &chatTracePricingEngine{quote: PricingQuote{
		InputUSD:                 decimal.RequireFromString("0.000011"),
		OutputUSD:                decimal.RequireFromString("0.000007"),
		TotalUSD:                 decimal.RequireFromString("0.000018"),
		InputCredits:             11,
		OutputCredits:            7,
		TotalCredits:             18,
		InputTokenPriceUSDPer1M:  decimal.RequireFromString("1"),
		OutputTokenPriceUSDPer1M: decimal.RequireFromString("1"),
		InputTokenPriceResolved:  true,
		OutputTokenPriceResolved: true,
		PricingSource:            PricingSourceCodeDefaultFallback,
		UsageSource:              UsageSourceProviderUsage,
	}}
	service := &llmGatewayServiceImpl{
		adapterFactory: factory,
		tokenEstimator: NewTokenEstimator(),
		pricingEngine:  pricing,
		billing:        billing,
		localBilling:   billing,
		healthTracker:  NewChannelHealthTracker(nil),
	}

	organizationID := uuid.MustParse("66666666-6666-6666-6666-666666666666")
	shadowOrganizationID := uuid.MustParse("77777777-7777-7777-7777-777777777777")
	ownerID := uuid.MustParse("88888888-8888-8888-8888-888888888888")
	selection := &ProviderSelection{
		OrganizationID: shadowOrganizationID,
		Provider: providermodel.LLMProvider{
			ID:       uuid.MustParse("22222222-2222-2222-2222-222222222222"),
			Provider: "openai",
		},
		Model: llmmodel.LLMModel{
			ID:    uuid.MustParse("11111111-1111-1111-1111-111111111111"),
			Model: "gpt-test",
		},
		ModelSource:       PricingModelSourceGlobal,
		BillingLane:       UsageBillingLanePrivate,
		UseSystemProvider: false,
		RouteID:           uuid.MustParse("99999999-9999-9999-9999-999999999999"),
		ChannelProvider:   "openai",
		APIKey:            "provider-key-not-exported",
	}
	apiKey := &apikeymodel.TenantAPIKey{
		ID:             "key-test",
		OrganizationID: organizationID.String(),
		Status:         "active",
	}
	response, err := service.tryChatCompletion(
		context.Background(),
		apiKey,
		nil,
		req,
		req,
		selection,
		11,
		7,
		organizationID,
		shadowOrganizationID,
		ownerID,
		requestID,
		time.Now().Add(-250*time.Millisecond),
		0,
	)
	if err != nil {
		t.Fatalf("tryChatCompletion() error = %v", err)
	}
	return successfulChatAttemptResult{
		response:         response,
		expectedResponse: expectedResponse,
		providerAdapter:  providerAdapter,
		billing:          billing,
	}
}

func (a *chatTraceSuccessAdapter) Name() string { return "openai" }

func (a *chatTraceSuccessAdapter) ChatCompletion(ctx context.Context, _ *adapter.ChatRequest) (*adapter.ChatResponse, error) {
	a.chatCalls++
	if a.delay > 0 {
		timer := time.NewTimer(a.delay)
		defer timer.Stop()
		select {
		case <-timer.C:
		case <-ctx.Done():
			return nil, ctx.Err()
		}
	}
	return a.response, nil
}

func (a *chatTraceSuccessAdapter) ChatCompletionStream(context.Context, *adapter.ChatRequest) (<-chan adapter.StreamResponse, error) {
	return nil, nil
}

func (a *chatTraceSuccessAdapter) CreateResponse(context.Context, *adapter.CreateResponseRequest) (*adapter.CreateResponseResponse, error) {
	return nil, nil
}

func (a *chatTraceSuccessAdapter) CreateEmbeddings(context.Context, *adapter.EmbeddingsRequest) (*adapter.EmbeddingsResponse, error) {
	return nil, nil
}

func (a *chatTraceSuccessAdapter) CreateImage(context.Context, *adapter.ImageRequest) (*adapter.ImageResponse, error) {
	return nil, nil
}

func (a *chatTraceSuccessAdapter) Rerank(context.Context, *adapter.RerankRequest) (*adapter.RerankResponse, error) {
	return nil, nil
}

func (a *chatTraceSuccessAdapter) ListModels(context.Context, string) ([]adapter.Model, error) {
	return nil, nil
}

func (a *chatTraceSuccessAdapter) GetBalance(context.Context, string) (*adapter.Balance, error) {
	return nil, nil
}

func (a *chatTraceSuccessAdapter) ValidateConfig(*adapter.AdapterConfig) error { return nil }

func (a *chatTraceSuccessAdapter) GetProviderInfo() *adapter.ProviderInfo { return nil }

type chatTraceBillingProvider struct {
	canSpend       bool
	preDeductCalls int
	settleCalls    int
	lastSettled    *BillingContext
}

func (b *chatTraceBillingProvider) PreDeduct(context.Context, *BillingContext) error {
	b.preDeductCalls++
	return nil
}

func (b *chatTraceBillingProvider) Settle(_ context.Context, billing *BillingContext) error {
	b.settleCalls++
	b.lastSettled = billing
	return nil
}

func (b *chatTraceBillingProvider) CalculateCreditsFromTokens(int, int, uuid.UUID) (int64, int64, int64, error) {
	return 0, 0, 0, nil
}

func (b *chatTraceBillingProvider) CalculateImageCredits(*adapter.ImageRequest, uuid.UUID) (int64, error) {
	return 0, nil
}

func (b *chatTraceBillingProvider) CheckBalance(context.Context, uuid.UUID, uuid.UUID, int64) (bool, error) {
	return b.canSpend, nil
}

func (b *chatTraceBillingProvider) CheckPrivateChannelBalance(context.Context, uuid.UUID, uuid.UUID, int64) (bool, error) {
	return b.canSpend, nil
}

type chatTracePricingEngine struct {
	quote PricingQuote
}

func (p *chatTracePricingEngine) QuoteTokens(context.Context, PricingModelRef, int, int) (PricingQuote, error) {
	return p.quote, nil
}

func (p *chatTracePricingEngine) QuoteImage(context.Context, PricingModelRef, *adapter.ImageRequest) (PricingQuote, error) {
	return PricingQuote{}, nil
}

func newBlockingErrorExporter() *blockingErrorExporter {
	return &blockingErrorExporter{
		started: make(chan struct{}),
		release: make(chan struct{}),
	}
}

func (e *blockingErrorExporter) ExportSpans(ctx context.Context, _ []trace.ReadOnlySpan) error {
	e.startedOnce.Do(func() { close(e.started) })
	select {
	case <-e.release:
		return errors.New("test collector unavailable")
	case <-ctx.Done():
		return ctx.Err()
	}
}

func (e *blockingErrorExporter) Shutdown(context.Context) error {
	return nil
}

func (e *blockingErrorExporter) unblock() {
	e.releaseOnce.Do(func() { close(e.release) })
}

type discardSpanExporter struct{}

func (discardSpanExporter) ExportSpans(context.Context, []trace.ReadOnlySpan) error { return nil }

func (discardSpanExporter) Shutdown(context.Context) error { return nil }

func latencyPercentiles(values []time.Duration) (time.Duration, time.Duration, time.Duration) {
	ordered := append([]time.Duration(nil), values...)
	sort.Slice(ordered, func(left, right int) bool { return ordered[left] < ordered[right] })
	return ordered[percentileIndex(len(ordered), 0.50)],
		ordered[percentileIndex(len(ordered), 0.95)],
		ordered[percentileIndex(len(ordered), 0.99)]
}

func percentileIndex(length int, percentile float64) int {
	index := int(float64(length)*percentile+0.999999) - 1
	if index < 0 {
		return 0
	}
	if index >= length {
		return length - 1
	}
	return index
}

func latencyDeltaPercent(disabled time.Duration, enabled time.Duration) float64 {
	if disabled <= 0 {
		return 0
	}
	return float64(enabled-disabled) / float64(disabled) * 100
}
