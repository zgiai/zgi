package gateway

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"
	llmmodel "github.com/zgiai/zgi/api/internal/modules/llm/llmmodel/model"
	adapter "github.com/zgiai/zgi/api/internal/modules/llm/protocol/adapters"
	providermodel "github.com/zgiai/zgi/api/internal/modules/llm/provider/model"
)

func TestHandleStreamBillingNormalizesPostConnectProviderDeadline(t *testing.T) {
	s := &llmGatewayServiceImpl{
		billing:       &fakeBillingProvider{checkBalanceResult: true},
		localBilling:  &fakeBillingProvider{checkBalanceResult: true},
		healthTracker: NewChannelHealthTracker(nil),
	}
	input := make(chan adapter.StreamResponse, 1)
	output := make(chan adapter.StreamResponse, 2)
	input <- adapter.StreamResponse{Error: context.DeadlineExceeded, Done: true}
	close(input)
	billingContext := &BillingContext{
		UseSystemProvider: false,
		OrganizationID:    "org-1",
		RequestID:         "req-chat-stream-timeout",
		ModelName:         "gpt-5",
		ProviderName:      "openai",
	}

	s.handleStreamBilling(context.Background(), input, output, billingContext, nil, nil, time.Now(), nil)

	responses := collectStreamResponses(output)
	if len(responses) != 1 {
		t.Fatalf("responses = %d, want one terminal provider error", len(responses))
	}
	assertTypedProviderDeadline(t, responses[0].Error)
}

func TestHandleNativeStreamBillingNormalizesPostConnectProviderDeadline(t *testing.T) {
	s := &llmGatewayServiceImpl{
		billing:       &fakeBillingProvider{checkBalanceResult: true},
		localBilling:  &fakeBillingProvider{checkBalanceResult: true},
		healthTracker: NewChannelHealthTracker(nil),
	}
	input := make(chan adapter.RawStreamEvent, 1)
	output := make(chan adapter.RawStreamEvent, 2)
	input <- adapter.RawStreamEvent{Error: context.DeadlineExceeded, Done: true}
	close(input)
	billingContext := &BillingContext{
		UseSystemProvider: false,
		OrganizationID:    "org-1",
		RequestID:         "req-native-stream-timeout",
		ModelName:         "gpt-5",
		ProviderName:      "openai",
	}
	selection := &ProviderSelection{
		UseSystemProvider: false,
		Model:             llmmodel.LLMModel{ID: uuid.New(), Model: "gpt-5"},
		Provider:          providermodel.LLMProvider{Provider: "openai"},
	}

	s.handleNativeStreamBilling(
		context.Background(), input, output, billingContext, selection, nil, 0,
		time.Now(), "gpt-5", nil, "llm.responses.stream", nativeUsageBodyFormatResponses,
	)

	events := collectRawStreamEvents(output)
	if len(events) != 1 {
		t.Fatalf("events = %d, want one terminal provider error", len(events))
	}
	assertTypedProviderDeadline(t, events[0].Error)
}

func collectStreamResponses(input <-chan adapter.StreamResponse) []adapter.StreamResponse {
	var responses []adapter.StreamResponse
	for response := range input {
		responses = append(responses, response)
	}
	return responses
}

func collectRawStreamEvents(input <-chan adapter.RawStreamEvent) []adapter.RawStreamEvent {
	var events []adapter.RawStreamEvent
	for event := range input {
		events = append(events, event)
	}
	return events
}

func assertTypedProviderDeadline(t *testing.T, err error) {
	t.Helper()
	if !errors.Is(err, adapter.ErrTimeout) {
		t.Fatalf("error = %v, want ErrTimeout", err)
	}
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("error = %v, want original deadline cause", err)
	}
}
