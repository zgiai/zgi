package handler_test

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	apikeymodel "github.com/zgiai/zgi/api/internal/modules/llm/apikey/model"
	llmerrors "github.com/zgiai/zgi/api/internal/modules/llm/errors"
	gatewayhandler "github.com/zgiai/zgi/api/internal/modules/llm/gateway/handler"
	adapter "github.com/zgiai/zgi/api/internal/modules/llm/protocol/adapters"
	appcatalog "github.com/zgiai/zgi/api/pkg/apperror/catalog"
	apptransport "github.com/zgiai/zgi/api/pkg/apperror/transport"
)

func TestChatCompletionsStreamDoneFrameOnlyEmitsDoneSentinel(t *testing.T) {
	body := performStreamRequest(t, []adapter.StreamResponse{
		{Done: true},
	})

	if strings.Contains(body, `"choices"`) {
		t.Fatalf("done frame was serialized as JSON:\n%s", body)
	}
	if strings.Count(body, "data:[DONE]") != 1 {
		t.Fatalf("done sentinel count mismatch:\n%s", body)
	}
	if strings.Count(body, "event:message") != 1 {
		t.Fatalf("event count mismatch:\n%s", body)
	}
}

func TestChatCompletionsStreamDoneUsageEmitsValidUsageChunk(t *testing.T) {
	body := performStreamRequest(t, []adapter.StreamResponse{
		{
			Done: true,
			Usage: &adapter.Usage{
				PromptTokens:     9687,
				CompletionTokens: 448,
				TotalTokens:      10135,
			},
		},
	})

	if strings.Contains(body, `"choices":null`) {
		t.Fatalf("usage chunk has null choices:\n%s", body)
	}
	if !strings.Contains(body, `"choices":[]`) {
		t.Fatalf("usage chunk does not have an empty choices array:\n%s", body)
	}
	if !strings.Contains(body, `"object":"chat.completion.chunk"`) {
		t.Fatalf("usage chunk does not use chat completion chunk object:\n%s", body)
	}
	if !strings.Contains(body, `"model":"deepseek-chat"`) {
		t.Fatalf("usage chunk does not preserve request model:\n%s", body)
	}
	if strings.Count(body, "data:[DONE]") != 1 {
		t.Fatalf("done sentinel count mismatch:\n%s", body)
	}
}

func TestChatCompletionsStreamDoesNotDuplicateForwardedUsage(t *testing.T) {
	usage := &adapter.Usage{
		PromptTokens:     10,
		CompletionTokens: 2,
		TotalTokens:      12,
	}
	body := performStreamRequest(t, []adapter.StreamResponse{
		{
			ID:      "chatcmpl-upstream",
			Object:  "chat.completion.chunk",
			Created: 1710000000,
			Model:   "deepseek-chat",
			Usage:   usage,
		},
		{
			Done:  true,
			Usage: usage,
		},
	})

	if strings.Count(body, `"usage":`) != 1 {
		t.Fatalf("usage should be emitted once:\n%s", body)
	}
	if strings.Contains(body, `"choices":null`) {
		t.Fatalf("forwarded usage chunk has null choices:\n%s", body)
	}
	if strings.Count(body, "data:[DONE]") != 1 {
		t.Fatalf("done sentinel count mismatch:\n%s", body)
	}
}

func TestChatCompletionsStreamNormalChunkUsesEmptyChoicesArray(t *testing.T) {
	body := performStreamRequest(t, []adapter.StreamResponse{
		{
			ID:      "chatcmpl-empty",
			Object:  "chat.completion.chunk",
			Created: 1710000000,
			Model:   "deepseek-chat",
		},
		{Done: true},
	})

	if strings.Contains(body, `"choices":null`) {
		t.Fatalf("normal chunk has null choices:\n%s", body)
	}
	if !strings.Contains(body, `"choices":[]`) {
		t.Fatalf("normal chunk does not have an empty choices array:\n%s", body)
	}
	if strings.Count(body, "data:[DONE]") != 1 {
		t.Fatalf("done sentinel count mismatch:\n%s", body)
	}
}

func TestChatCompletionsStreamPreservesToolCallIndexAndOmitsEmptyDeltaFields(t *testing.T) {
	toolIndex := 0
	body := performStreamRequest(t, []adapter.StreamResponse{
		{
			ID:      "chatcmpl-tool",
			Object:  "chat.completion.chunk",
			Created: 1710000000,
			Model:   "deepseek-chat",
			Choices: []adapter.StreamChoice{
				{
					Index: 0,
					Delta: adapter.Message{
						ToolCalls: []adapter.ToolCall{
							{
								Index: &toolIndex,
								Function: adapter.FunctionCall{
									Arguments: `{"questions":[`,
								},
							},
						},
					},
				},
			},
		},
		{Done: true},
	})

	chunks := parseStreamJSONChunks(t, body)
	if len(chunks) != 1 {
		t.Fatalf("json chunks len = %d, body:\n%s", len(chunks), body)
	}
	toolCall := chunks[0].Choices[0].Delta.ToolCalls[0]
	if toolCall.Index == nil || *toolCall.Index != 0 {
		t.Fatalf("tool call index = %v, want 0", toolCall.Index)
	}
	if strings.Contains(body, `"name":""`) {
		t.Fatalf("tool call delta should not emit empty function name:\n%s", body)
	}
	if strings.Contains(body, `"id":""`) {
		t.Fatalf("tool call delta should not emit empty id:\n%s", body)
	}
	if strings.Contains(body, `"type":""`) {
		t.Fatalf("tool call delta should not emit empty type:\n%s", body)
	}
}

func TestResponsesStreamEmitsNativeEventsWithoutChatChoices(t *testing.T) {
	body := performResponsesStreamRequest(t, []adapter.RawStreamEvent{
		{
			Event: "response.created",
			Data:  json.RawMessage(`{"type":"response.created","response":{"id":"resp_1"}}`),
		},
		{
			Event: "response.output_text.delta",
			Data:  json.RawMessage(`{"type":"response.output_text.delta","delta":"hi"}`),
		},
		{
			Event: "response.completed",
			Data:  json.RawMessage(`{"type":"response.completed","response":{"usage":{"input_tokens":1,"output_tokens":1,"total_tokens":2}}}`),
		},
		{Done: true},
	})

	if strings.Contains(body, `"choices"`) {
		t.Fatalf("responses stream emitted chat choices:\n%s", body)
	}
	if strings.Contains(body, "[DONE]") {
		t.Fatalf("responses stream emitted chat done sentinel:\n%s", body)
	}
	if !strings.Contains(body, "event:response.output_text.delta") {
		t.Fatalf("responses stream did not preserve event names:\n%s", body)
	}
}

func TestAnthropicMessagesStreamEmitsNativeEventsWithoutChatChoices(t *testing.T) {
	body := performAnthropicStreamRequest(t, []adapter.RawStreamEvent{
		{
			Event: "message_start",
			Data:  json.RawMessage(`{"type":"message_start","message":{"id":"msg_1","usage":{"input_tokens":1,"output_tokens":0}}}`),
		},
		{
			Event: "content_block_delta",
			Data:  json.RawMessage(`{"type":"content_block_delta","delta":{"type":"text_delta","text":"hi"}}`),
		},
		{
			Event: "message_stop",
			Data:  json.RawMessage(`{"type":"message_stop"}`),
		},
		{Done: true},
	})

	if strings.Contains(body, `"choices"`) {
		t.Fatalf("anthropic stream emitted chat choices:\n%s", body)
	}
	if strings.Contains(body, "[DONE]") {
		t.Fatalf("anthropic stream emitted chat done sentinel:\n%s", body)
	}
	if !strings.Contains(body, "event:content_block_delta") {
		t.Fatalf("anthropic stream did not preserve event names:\n%s", body)
	}
}

func TestChatCompletionsTimeoutUsesLocalizedMessageWithoutChangingWireContract(t *testing.T) {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	handler := gatewayhandler.NewLLMHandler(
		fakeGatewayService{chatError: adapter.ErrTimeout},
		testErrorProjector(t),
	)
	router.POST("/v1/chat/completions", func(c *gin.Context) {
		c.Set("llm_api_key", &apikeymodel.TenantAPIKey{})
		handler.ChatCompletions(c)
	})

	request := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", strings.NewReader(`{
		"model": "deepseek-chat",
		"messages": [{"role": "user", "content": "hello"}]
	}`))
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("Accept-Language", "zh-CN")
	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, request)

	if recorder.Code != http.StatusGatewayTimeout {
		t.Fatalf("status = %d, body = %s", recorder.Code, recorder.Body.String())
	}
	var body struct {
		Error struct {
			Message string  `json:"message"`
			Type    string  `json:"type"`
			Param   *string `json:"param"`
			Code    string  `json:"code"`
		} `json:"error"`
	}
	if err := json.Unmarshal(recorder.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if body.Error.Message != "大模型服务响应超时，请重试或选择其他模型。" ||
		body.Error.Type != "server_error" || body.Error.Code != "upstream_timeout" || body.Error.Param != nil {
		t.Fatalf("wire contract = %#v", body.Error)
	}
}

func TestChatCompletionsStreamTimeoutUsesLocalizedSafeMessage(t *testing.T) {
	body := performStreamRequestWithLanguage(t, []adapter.StreamResponse{{
		Error: errors.Join(adapter.ErrTimeout, errors.New("provider token sk-secret")),
	}}, "zh-CN")

	if !strings.Contains(body, "event:error") || !strings.Contains(body, "大模型服务响应超时，请重试或选择其他模型。") {
		t.Fatalf("localized stream error missing:\n%s", body)
	}
	if strings.Contains(body, "sk-secret") {
		t.Fatalf("stream error leaked diagnostic cause:\n%s", body)
	}
}

func performStreamRequest(t *testing.T, responses []adapter.StreamResponse) string {
	return performStreamRequestWithLanguage(t, responses, "")
}

func performStreamRequestWithLanguage(t *testing.T, responses []adapter.StreamResponse, language string) string {
	t.Helper()

	gin.SetMode(gin.TestMode)
	router := gin.New()
	handler := gatewayhandler.NewLLMHandler(fakeGatewayService{responses: responses}, testErrorProjector(t))
	router.POST("/v1/chat/completions", func(c *gin.Context) {
		c.Set("llm_api_key", &apikeymodel.TenantAPIKey{})
		handler.ChatCompletions(c)
	})

	request := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", strings.NewReader(`{
		"model": "deepseek-chat",
		"messages": [{"role": "user", "content": "hello"}],
		"stream": true
	}`))
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("Accept-Language", language)
	recorder := httptest.NewRecorder()

	router.ServeHTTP(recorder, request)

	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", recorder.Code, recorder.Body.String())
	}
	return recorder.Body.String()
}

func performResponsesStreamRequest(t *testing.T, events []adapter.RawStreamEvent) string {
	t.Helper()

	gin.SetMode(gin.TestMode)
	router := gin.New()
	handler := gatewayhandler.NewLLMHandler(fakeGatewayService{rawEvents: events}, testErrorProjector(t))
	router.POST("/v1/responses", func(c *gin.Context) {
		c.Set("llm_api_key", &apikeymodel.TenantAPIKey{})
		handler.CreateResponse(c)
	})

	request := httptest.NewRequest(http.MethodPost, "/v1/responses", strings.NewReader(`{
		"model": "gpt-5-codex",
		"input": "hello",
		"stream": true
	}`))
	request.Header.Set("Content-Type", "application/json")
	recorder := httptest.NewRecorder()

	router.ServeHTTP(recorder, request)

	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", recorder.Code, recorder.Body.String())
	}
	return recorder.Body.String()
}

func performAnthropicStreamRequest(t *testing.T, events []adapter.RawStreamEvent) string {
	t.Helper()

	gin.SetMode(gin.TestMode)
	router := gin.New()
	handler := gatewayhandler.NewLLMHandler(fakeGatewayService{anthropicEvents: events}, testErrorProjector(t))
	router.POST("/anthropic/v1/messages", func(c *gin.Context) {
		c.Set("llm_api_key", &apikeymodel.TenantAPIKey{})
		handler.CreateAnthropicMessage(c)
	})

	request := httptest.NewRequest(http.MethodPost, "/anthropic/v1/messages", strings.NewReader(`{
		"model": "claude-sonnet-4-5",
		"max_tokens": 16,
		"messages": [{"role": "user", "content": "hello"}],
		"stream": true
	}`))
	request.Header.Set("Content-Type", "application/json")
	recorder := httptest.NewRecorder()

	router.ServeHTTP(recorder, request)

	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", recorder.Code, recorder.Body.String())
	}
	return recorder.Body.String()
}

func testErrorProjector(t *testing.T) *apptransport.Projector {
	t.Helper()
	definitions := append(appcatalog.DefaultDefinitions(), llmerrors.CatalogDefinitions()...)
	productCatalog, err := appcatalog.New(appcatalog.LocaleEnglishUS, appcatalog.CodeInternal, definitions...)
	if err != nil {
		t.Fatal(err)
	}
	projector, err := apptransport.NewProjector(productCatalog)
	if err != nil {
		t.Fatal(err)
	}
	return projector
}

func parseStreamJSONChunks(t *testing.T, body string) []adapter.StreamResponse {
	t.Helper()

	var chunks []adapter.StreamResponse
	for _, line := range strings.Split(body, "\n") {
		data, ok := strings.CutPrefix(line, "data:")
		if !ok || data == "[DONE]" || data == "" {
			continue
		}
		var chunk adapter.StreamResponse
		if err := json.Unmarshal([]byte(data), &chunk); err != nil {
			t.Fatalf("unmarshal stream chunk %q: %v", data, err)
		}
		chunks = append(chunks, chunk)
	}
	return chunks
}

type fakeGatewayService struct {
	responses       []adapter.StreamResponse
	chatError       error
	rawResponse     *adapter.RawResponse
	rawEvents       []adapter.RawStreamEvent
	anthropicResp   *adapter.RawResponse
	anthropicEvents []adapter.RawStreamEvent
}

func (s fakeGatewayService) ChatCompletion(context.Context, *apikeymodel.TenantAPIKey, *adapter.ChatRequest) (*adapter.ChatResponse, error) {
	return nil, s.chatError
}

func (s fakeGatewayService) ChatCompletionStream(context.Context, *apikeymodel.TenantAPIKey, *adapter.ChatRequest) (<-chan adapter.StreamResponse, error) {
	ch := make(chan adapter.StreamResponse, len(s.responses))
	for _, resp := range s.responses {
		ch <- resp
	}
	close(ch)
	return ch, nil
}

func (s fakeGatewayService) CreateResponse(context.Context, *apikeymodel.TenantAPIKey, *adapter.CreateResponseRequest) (*adapter.CreateResponseResponse, error) {
	return nil, nil
}

func (s fakeGatewayService) CreateResponseRaw(context.Context, *apikeymodel.TenantAPIKey, *adapter.RawResponseRequest) (*adapter.RawResponse, error) {
	return s.rawResponse, nil
}

func (s fakeGatewayService) CreateResponseStream(context.Context, *apikeymodel.TenantAPIKey, *adapter.RawResponseRequest) (<-chan adapter.RawStreamEvent, error) {
	ch := make(chan adapter.RawStreamEvent, len(s.rawEvents))
	for _, resp := range s.rawEvents {
		ch <- resp
	}
	close(ch)
	return ch, nil
}

func (s fakeGatewayService) CreateAnthropicMessage(context.Context, *apikeymodel.TenantAPIKey, *adapter.AnthropicMessageRequest) (*adapter.RawResponse, error) {
	return s.anthropicResp, nil
}

func (s fakeGatewayService) CreateAnthropicMessageStream(context.Context, *apikeymodel.TenantAPIKey, *adapter.AnthropicMessageRequest) (<-chan adapter.RawStreamEvent, error) {
	ch := make(chan adapter.RawStreamEvent, len(s.anthropicEvents))
	for _, resp := range s.anthropicEvents {
		ch <- resp
	}
	close(ch)
	return ch, nil
}

func (s fakeGatewayService) CreateEmbeddings(context.Context, *apikeymodel.TenantAPIKey, *adapter.EmbeddingsRequest) (*adapter.EmbeddingsResponse, error) {
	return nil, nil
}

func (s fakeGatewayService) CreateImage(context.Context, *apikeymodel.TenantAPIKey, *adapter.ImageRequest) (*adapter.ImageResponse, error) {
	return nil, nil
}

func (s fakeGatewayService) ListAvailableModels(context.Context, *apikeymodel.TenantAPIKey) ([]adapter.Model, error) {
	return nil, nil
}
