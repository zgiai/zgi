package gateway

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
	"time"

	"github.com/zgiai/zgi/api/config"
	adapter "github.com/zgiai/zgi/api/internal/modules/llm/protocol/adapters"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/sdk/trace/tracetest"
)

func TestTraceNativeLLMOperationDisabledProducesNoSpanOrAllocations(t *testing.T) {
	restoreConfig := setGatewayOTELConfig(t, config.OpenTelemetryConfig{})
	defer restoreConfig()

	recorder := tracetest.NewSpanRecorder()
	provider := trace.NewTracerProvider(trace.WithSpanProcessor(recorder))
	restoreTracer := setGatewayTracerProvider(t, provider)
	defer restoreTracer()

	_, _, billing := successfulChatTraceFixture()
	service := &llmGatewayServiceImpl{}
	requestBody := json.RawMessage(`{"input":"protected"}`)
	responseBody := json.RawMessage(`{"output":"protected"}`)
	start := time.Unix(1_700_000_000, 0)
	end := start.Add(time.Millisecond)
	allocations := testing.AllocsPerRun(1000, func() {
		service.traceNativeLLMOperation(context.Background(), "llm.responses", "responses", requestBody, responseBody, nil, start, end, billing, nil)
	})
	if allocations != 0 {
		t.Fatalf("disabled native trace allocations = %v, want 0", allocations)
	}
	if got := len(recorder.Ended()); got != 0 {
		t.Fatalf("disabled native trace ended spans = %d, want 0", got)
	}
}

func TestGatewayLLMOperationTraceContract(t *testing.T) {
	otelConfig := enabledGatewayOTELConfig()
	otelConfig.LLMCaptureContent = otelLLMCaptureNone
	restoreConfig := setGatewayOTELConfig(t, otelConfig)
	defer restoreConfig()

	service := &llmGatewayServiceImpl{}
	tests := []struct {
		name            string
		spanName        string
		operation       string
		observationType string
		emit            func(context.Context, *BillingContext, time.Time, time.Time)
	}{
		{
			name:            "chat",
			spanName:        "llm.chat",
			operation:       "chat",
			observationType: otelObservationGeneration,
			emit: func(ctx context.Context, billing *BillingContext, start, end time.Time) {
				service.traceChatCompletion(ctx, nil, nil, start, end, billing, nil)
			},
		},
		{
			name:            "chat stream",
			spanName:        "llm.chat.stream",
			operation:       "chat",
			observationType: otelObservationGeneration,
			emit: func(ctx context.Context, billing *BillingContext, start, end time.Time) {
				service.traceStreamingChatCompletion(ctx, nil, "protected", start, end, billing, 11, 7, nil)
			},
		},
		{
			name:            "responses",
			spanName:        "llm.responses",
			operation:       "responses",
			observationType: otelObservationGeneration,
			emit: func(ctx context.Context, billing *BillingContext, start, end time.Time) {
				service.traceCreateResponse(ctx, nil, nil, start, end, billing, nil)
			},
		},
		{
			name:            "native responses",
			spanName:        "llm.responses",
			operation:       "responses",
			observationType: otelObservationGeneration,
			emit: func(ctx context.Context, billing *BillingContext, start, end time.Time) {
				service.traceNativeLLMOperation(ctx, "llm.responses", "responses", json.RawMessage(`{"input":"protected"}`), json.RawMessage(`{"output":"protected"}`), &adapter.Usage{PromptTokens: 11, CompletionTokens: 7, TotalTokens: 18}, start, end, billing, nil)
			},
		},
		{
			name:            "native responses stream",
			spanName:        "llm.responses.stream",
			operation:       "responses",
			observationType: otelObservationGeneration,
			emit: func(ctx context.Context, billing *BillingContext, start, end time.Time) {
				service.traceNativeLLMStreamOperation(ctx, "llm.responses.stream", "responses", json.RawMessage(`{"input":"protected"}`), "protected", &adapter.Usage{PromptTokens: 11, CompletionTokens: 7, TotalTokens: 18}, start, end, billing, nil)
			},
		},
		{
			name:            "native anthropic",
			spanName:        "llm.anthropic.messages",
			operation:       "messages",
			observationType: otelObservationGeneration,
			emit: func(ctx context.Context, billing *BillingContext, start, end time.Time) {
				service.traceNativeLLMOperation(ctx, "llm.anthropic.messages", "messages", json.RawMessage(`{"messages":[{"content":"protected"}]}`), json.RawMessage(`{"content":"protected"}`), &adapter.Usage{PromptTokens: 11, CompletionTokens: 7, TotalTokens: 18}, start, end, billing, nil)
			},
		},
		{
			name:            "native anthropic stream",
			spanName:        "llm.anthropic.messages.stream",
			operation:       "messages",
			observationType: otelObservationGeneration,
			emit: func(ctx context.Context, billing *BillingContext, start, end time.Time) {
				service.traceNativeLLMStreamOperation(ctx, "llm.anthropic.messages.stream", "messages", json.RawMessage(`{"messages":[{"content":"protected"}]}`), "protected", &adapter.Usage{PromptTokens: 11, CompletionTokens: 7, TotalTokens: 18}, start, end, billing, nil)
			},
		},
		{
			name:            "embeddings",
			spanName:        "llm.embeddings",
			operation:       "embeddings",
			observationType: otelObservationEmbedding,
			emit: func(ctx context.Context, billing *BillingContext, start, end time.Time) {
				service.traceEmbeddings(ctx, nil, nil, start, end, billing, nil)
			},
		},
		{
			name:            "rerank",
			spanName:        "llm.rerank",
			operation:       "rerank",
			observationType: otelObservationRetriever,
			emit: func(ctx context.Context, billing *BillingContext, start, end time.Time) {
				service.traceRerank(ctx, nil, nil, start, end, billing, nil)
			},
		},
		{
			name:            "images",
			spanName:        "llm.images",
			operation:       "image_generation",
			observationType: otelObservationGeneration,
			emit: func(ctx context.Context, billing *BillingContext, start, end time.Time) {
				service.traceImageGeneration(ctx, nil, nil, start, end, billing, nil)
			},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			recorder := tracetest.NewSpanRecorder()
			provider := trace.NewTracerProvider(trace.WithSpanProcessor(recorder))
			restoreTracer := setGatewayTracerProvider(t, provider)
			defer restoreTracer()

			_, _, billing := successfulChatTraceFixture()
			billing.IsStreaming = test.spanName == "llm.chat.stream" || test.spanName == "llm.responses.stream" || test.spanName == "llm.anthropic.messages.stream"
			start := time.Unix(1_700_000_000, 0)
			end := start.Add(250 * time.Millisecond)
			test.emit(context.Background(), billing, start, end)

			ended := recorder.Ended()
			if len(ended) != 1 {
				t.Fatalf("ended spans = %d, want 1", len(ended))
			}
			span := ended[0]
			if span.Name() != test.spanName {
				t.Fatalf("span name = %q, want %q", span.Name(), test.spanName)
			}
			if span.Status().Code != codes.Ok {
				t.Fatalf("span status = %v, want OK", span.Status().Code)
			}
			attrs := recordedAttributes(span.Attributes())
			assertStringAttribute(t, attrs, "gen_ai.operation.name", test.operation)
			assertStringAttribute(t, attrs, "langfuse.observation.type", test.observationType)
			assertStringAttribute(t, attrs, "zgi.invocation_id", "request-test")
			assertIntAttribute(t, attrs, "gen_ai.usage.total_tokens", 18)
			for _, key := range []string{"langfuse.observation.input", "langfuse.observation.output", "langfuse.observation.model.parameters"} {
				if _, ok := attrs[key]; ok {
					t.Fatalf("capture=none emitted %s", key)
				}
			}
		})
	}
}

func TestTraceNativeLLMOperationErrorContract(t *testing.T) {
	otelConfig := enabledGatewayOTELConfig()
	otelConfig.LLMCaptureContent = otelLLMCaptureNone
	restoreConfig := setGatewayOTELConfig(t, otelConfig)
	defer restoreConfig()

	recorder := tracetest.NewSpanRecorder()
	provider := trace.NewTracerProvider(trace.WithSpanProcessor(recorder))
	restoreTracer := setGatewayTracerProvider(t, provider)
	defer restoreTracer()

	_, _, billing := successfulChatTraceFixture()
	billing.Status = "error"
	(&llmGatewayServiceImpl{}).traceNativeLLMOperation(
		context.Background(),
		"llm.anthropic.messages",
		"messages",
		json.RawMessage(`{"messages":[{"content":"protected"}]}`),
		nil,
		nil,
		time.Now().Add(-time.Millisecond),
		time.Now(),
		billing,
		errors.New("provider unavailable"),
	)

	ended := recorder.Ended()
	if len(ended) != 1 {
		t.Fatalf("ended spans = %d, want 1", len(ended))
	}
	span := ended[0]
	if span.Status().Code != codes.Error {
		t.Fatalf("span status = %v, want Error", span.Status().Code)
	}
	attrs := recordedAttributes(span.Attributes())
	assertStringAttribute(t, attrs, "zgi.status", "error")
	assertStringAttribute(t, attrs, "langfuse.observation.level", otelObservationError)
	if _, ok := attrs["langfuse.observation.input"]; ok {
		t.Fatal("capture=none emitted native request body")
	}
}

func TestNativeTraceOperationNames(t *testing.T) {
	tests := map[string]string{
		"llm.responses":                 "responses",
		"llm.responses.stream":          "responses",
		"llm.anthropic.messages":        "messages",
		"llm.anthropic.messages.stream": "messages",
		"llm.custom":                    "custom",
	}
	for traceName, want := range tests {
		if got := nativeTraceOperation(traceName); got != want {
			t.Fatalf("nativeTraceOperation(%q) = %q, want %q", traceName, got, want)
		}
	}
}
