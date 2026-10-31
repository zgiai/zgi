package gateway

import (
	"context"
	"testing"

	apikeymodel "github.com/zgiai/zgi/api/internal/modules/llm/apikey/model"
	"github.com/zgiai/zgi/api/internal/modules/llm/gateway/types"
	adapter "github.com/zgiai/zgi/api/internal/modules/llm/protocol/adapters"
	"github.com/zgiai/zgi/api/internal/modules/llm/shared"
)

func TestChatPublishesInvocationIDBeforeValidation(t *testing.T) {
	service := &llmGatewayServiceImpl{}
	var ids []string
	ctx := types.WithInvocationIDObserver(t.Context(), func(id string) { ids = append(ids, id) })
	key := &apikeymodel.TenantAPIKey{}
	if _, err := service.ChatCompletion(ctx, key, &adapter.ChatRequest{Model: "test"}); err != ErrMissingMessages {
		t.Fatalf("non-stream error = %v", err)
	}
	if _, err := service.ChatCompletionStream(ctx, key, &adapter.ChatRequest{Model: "test"}); err != ErrMissingMessages {
		t.Fatalf("stream error = %v", err)
	}
	if len(ids) != 2 || ids[0] == ids[1] {
		t.Fatalf("missing or reused invocation identity: %v", ids)
	}
}

func TestPublishedInvocationIDMatchesSettledBilling(t *testing.T) {
	var published string
	ctx := types.WithInvocationIDObserver(t.Context(), func(id string) { published = id })
	id := types.NewInvocationID(ctx)
	result := executeSuccessfulChatAttempt(t, id)
	if result.billing.lastSettled == nil || result.billing.lastSettled.RequestID != published {
		t.Fatalf("published=%q, billing=%+v", published, result.billing.lastSettled)
	}
}

func TestRerankPublishesOneInvocationIDBeforeValidation(t *testing.T) {
	service := &llmGatewayServiceImpl{}
	var ids []string
	ctx := types.WithInvocationIDObserver(t.Context(), func(id string) { ids = append(ids, id) })
	if _, err := service.Rerank(ctx, &apikeymodel.TenantAPIKey{}, &adapter.RerankRequest{}); err != ErrMissingModel {
		t.Fatalf("Rerank() error = %v, want ErrMissingModel", err)
	}
	if len(ids) != 1 || ids[0] == "" {
		t.Fatalf("published invocation IDs = %v, want exactly one", ids)
	}
}

func TestRerankInvocationIDKeepsTransportRequestID(t *testing.T) {
	service := &llmGatewayServiceImpl{}
	var published string
	ctx := context.WithValue(t.Context(), shared.ContextKeyRequestID, "req-rerank-1")
	ctx = types.WithInvocationIDObserver(ctx, func(id string) { published = id })
	_, _ = service.Rerank(ctx, &apikeymodel.TenantAPIKey{}, &adapter.RerankRequest{})
	if published != "req-rerank-1" {
		t.Fatalf("published = %q, want the transport request ID", published)
	}
}
