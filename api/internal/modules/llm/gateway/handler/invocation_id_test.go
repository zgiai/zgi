package handler

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	apikeymodel "github.com/zgiai/zgi/api/internal/modules/llm/apikey/model"
	"github.com/zgiai/zgi/api/internal/modules/llm/gateway"
	"github.com/zgiai/zgi/api/internal/modules/llm/gateway/types"
	adapter "github.com/zgiai/zgi/api/internal/modules/llm/protocol/adapters"
)

type invocationGatewayStub struct {
	types.GatewayService
	id   string
	fail bool
}

func (s *invocationGatewayStub) ChatCompletion(ctx context.Context, _ *apikeymodel.TenantAPIKey, _ *adapter.ChatRequest) (*adapter.ChatResponse, error) {
	s.id = types.NewInvocationID(ctx)
	if s.fail {
		return nil, gateway.ErrModelNotAuthorized
	}
	return &adapter.ChatResponse{ID: "provider-completion-id"}, nil
}

func (s *invocationGatewayStub) ChatCompletionStream(ctx context.Context, _ *apikeymodel.TenantAPIKey, _ *adapter.ChatRequest) (<-chan adapter.StreamResponse, error) {
	s.id = types.NewInvocationID(ctx)
	if s.fail {
		return nil, gateway.ErrModelNotAuthorized
	}
	ch := make(chan adapter.StreamResponse, 1)
	ch <- adapter.StreamResponse{Done: true}
	close(ch)
	return ch, nil
}

func TestChatResponseExposesInvocationIDBeforeHeadersCommit(t *testing.T) {
	for _, stream := range []bool{false, true} {
		for _, fail := range []bool{false, true} {
			t.Run(fmt.Sprintf("stream=%v/fail=%v", stream, fail), func(t *testing.T) {
				stub := &invocationGatewayStub{fail: fail}
				handler := NewLLMHandler(stub, testApplicationErrorProjector(t))
				recorder := httptest.NewRecorder()
				c, _ := gin.CreateTestContext(recorder)
				c.Set("llm_api_key", &apikeymodel.TenantAPIKey{})
				c.Request = httptest.NewRequest(http.MethodPost, "/v1/chat/completions", strings.NewReader(fmt.Sprintf(`{"model":"test","messages":[{"role":"user","content":"test"}],"stream":%v}`, stream)))
				c.Request.Header.Set(types.InvocationIDHeader, "untrusted-client-invocation-id")
				c.Request.Header.Set("X-Request-ID", "client-transport-id")
				c.Header("X-Request-ID", "client-transport-id")
				handler.ChatCompletions(c)
				response := recorder.Result()
				defer response.Body.Close()
				if stub.id == "" || response.Header.Get(types.InvocationIDHeader) != stub.id || stub.id == "untrusted-client-invocation-id" {
					t.Fatalf("invocation header=%q, gateway=%q", response.Header.Get(types.InvocationIDHeader), stub.id)
				}
				if response.Header.Get("X-Request-ID") != "client-transport-id" {
					t.Fatal("transport ID was replaced")
				}
				wantStatus := http.StatusOK
				if fail {
					wantStatus = http.StatusForbidden
				}
				if response.StatusCode != wantStatus {
					t.Fatalf("status=%d want=%d", response.StatusCode, wantStatus)
				}
				if stream && !fail && !strings.Contains(recorder.Body.String(), "[DONE]") {
					t.Fatal("SSE protocol changed")
				}
				if !stream && !fail && !strings.Contains(recorder.Body.String(), "provider-completion-id") {
					t.Fatal("provider completion ID changed")
				}
			})
		}
	}
}
