package provider

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	adapter "github.com/zgiai/zgi/api/internal/modules/llm/protocol/adapters"
)

func TestHandleOpenAICompatibleErrorMapsPlatformChannelUnavailable(t *testing.T) {
	err := handleOpenAICompatibleError(
		http.StatusBadGateway,
		[]byte(`{"error":{"message":"Platform model service is temporarily unavailable","type":"server_error","code":"platform_channel_unavailable"}}`),
	)

	if !errors.Is(err, adapter.ErrPlatformChannelUnavailable) {
		t.Fatalf("handleOpenAICompatibleError() error = %v, want ErrPlatformChannelUnavailable", err)
	}
}

func TestDecodeOpenAICompatibleVideoResponseReturnsUpstreamErrorMessage(t *testing.T) {
	_, err := decodeOpenAICompatibleVideoResponse([]byte(`{
		"error": {
			"code": "InvalidParameter",
			"message": "Error while downloading image, error: expected the width to be at least 300px, but received a 153x161px image instead",
			"param": "image_url",
			"type": "BadRequest"
		}
	}`))
	if err == nil {
		t.Fatal("decodeOpenAICompatibleVideoResponse() error = nil")
	}
	const want = "upstream error: Error while downloading image, error: expected the width to be at least 300px, but received a 153x161px image instead"
	if err.Error() != want {
		t.Fatalf("decodeOpenAICompatibleVideoResponse() error = %q, want %q", err.Error(), want)
	}
}

func TestDecodeOpenAICompatibleVideoResponseReturnsNestedDataErrorMessage(t *testing.T) {
	_, err := decodeOpenAICompatibleVideoResponse([]byte(`{
		"code": 0,
		"message": "success",
		"data": {
			"error": {
				"code": "InvalidParameter",
				"message": "Error while downloading image"
			}
		}
	}`))
	if err == nil {
		t.Fatal("decodeOpenAICompatibleVideoResponse() error = nil")
	}
	const want = "upstream error: Error while downloading image"
	if err.Error() != want {
		t.Fatalf("decodeOpenAICompatibleVideoResponse() error = %q, want %q", err.Error(), want)
	}
}

func TestHandleOpenAICompatibleErrorMapsOpenAICodes(t *testing.T) {
	tests := []struct {
		name     string
		status   int
		body     string
		wantCode string
		wantErr  error
	}{
		{
			name:     "insufficient quota code wins over rate limit status",
			status:   http.StatusTooManyRequests,
			body:     `{"error":{"message":"insufficient API quota","type":"insufficient_quota","code":"insufficient_quota"}}`,
			wantCode: "insufficient_quota",
			wantErr:  adapter.ErrQuotaExhausted,
		},
		{
			name:     "insufficient quota type works without code",
			status:   http.StatusOK,
			body:     `{"error":{"message":"insufficient API quota","type":"insufficient_quota"}}`,
			wantCode: "insufficient_quota",
			wantErr:  adapter.ErrQuotaExhausted,
		},
		{
			name:     "rate limit code",
			status:   http.StatusTooManyRequests,
			body:     `{"error":{"message":"rate limit reached","type":"rate_limit_error","code":"rate_limit_exceeded"}}`,
			wantCode: "rate_limit_exceeded",
			wantErr:  adapter.ErrRateLimited,
		},
		{
			name:     "billing hard limit code",
			status:   http.StatusBadRequest,
			body:     `{"error":{"message":"billing hard limit reached","type":"invalid_request_error","code":"billing_hard_limit_reached"}}`,
			wantCode: "billing_hard_limit_reached",
			wantErr:  adapter.ErrInsufficientBalance,
		},
		{
			name:     "content policy code",
			status:   http.StatusBadRequest,
			body:     `{"error":{"message":"content policy violation","type":"invalid_request_error","code":"content_policy_violation"}}`,
			wantCode: "content_policy_violation",
			wantErr:  adapter.ErrContentPolicyViolation,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := handleOpenAICompatibleError(tt.status, []byte(tt.body))
			if !errors.Is(err, tt.wantErr) {
				t.Fatalf("handleOpenAICompatibleError() error = %v, want %v", err, tt.wantErr)
			}

			var adapterErr *adapter.AdapterError
			if !errors.As(err, &adapterErr) {
				t.Fatalf("handleOpenAICompatibleError() error = %T %v, want AdapterError", err, err)
			}
			if adapterErr.Code != tt.wantCode {
				t.Fatalf("AdapterError.Code = %q, want %q", adapterErr.Code, tt.wantCode)
			}
			if adapterErr.StatusCode != tt.status {
				t.Fatalf("AdapterError.StatusCode = %d, want %d", adapterErr.StatusCode, tt.status)
			}
		})
	}
}

func TestOpenAIAdapterChatCompletionStreamParsesPlatformChannelError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadGateway)
		_, _ = fmt.Fprint(w, `{"error":{"message":"Platform model service is temporarily unavailable","type":"server_error","code":"platform_channel_unavailable"}}`)
	}))
	defer server.Close()

	a, err := NewOpenAIAdapter(&adapter.AdapterConfig{
		APIKey:  "test-key",
		BaseURL: server.URL + "/v1",
	})
	if err != nil {
		t.Fatalf("NewOpenAIAdapter() error = %v", err)
	}

	_, err = a.ChatCompletionStream(context.Background(), &adapter.ChatRequest{
		Model: "kimi-k2.6",
		Messages: []adapter.Message{
			{Role: "user", Content: "hello"},
		},
	})
	if !errors.Is(err, adapter.ErrPlatformChannelUnavailable) {
		t.Fatalf("ChatCompletionStream() error = %v, want ErrPlatformChannelUnavailable", err)
	}
}

func TestOpenAIAdapterChatCompletionStreamReturnsSSEError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		_, _ = fmt.Fprint(w, "data: {\"error\":{\"message\":\"insufficient API quota\",\"type\":\"insufficient_quota\",\"code\":\"insufficient_quota\"}}\n\ndata: [DONE]\n\n")
	}))
	defer server.Close()

	a, err := NewOpenAIAdapter(&adapter.AdapterConfig{
		APIKey:  "test-key",
		BaseURL: server.URL + "/v1",
	})
	if err != nil {
		t.Fatalf("NewOpenAIAdapter() error = %v", err)
	}

	stream, err := a.ChatCompletionStream(context.Background(), &adapter.ChatRequest{
		Model: "gpt-5.5",
		Messages: []adapter.Message{
			{Role: "user", Content: "hello"},
		},
	})
	if err != nil {
		t.Fatalf("ChatCompletionStream() error = %v", err)
	}

	var streamErr error
	for response := range stream {
		if response.Error != nil {
			streamErr = response.Error
		}
	}
	if streamErr == nil {
		t.Fatal("stream error = nil, want upstream insufficient quota error")
	}

	var adapterErr *adapter.AdapterError
	if !errors.As(streamErr, &adapterErr) {
		t.Fatalf("stream error = %T %v, want AdapterError", streamErr, streamErr)
	}
	if adapterErr.Code != "insufficient_quota" || adapterErr.Message != "insufficient API quota" {
		t.Fatalf("stream error = %+v, want code insufficient_quota and provider message", adapterErr)
	}
}

func TestOpenAIAdapterCreateResponseRaw_UsesResponsesEndpointAndRawBody(t *testing.T) {
	t.Helper()

	var (
		gotPath    string
		gotAuth    string
		gotPayload map[string]any
	)

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defer r.Body.Close()

		gotPath = r.URL.Path
		gotAuth = r.Header.Get("Authorization")
		if err := json.NewDecoder(r.Body).Decode(&gotPayload); err != nil {
			t.Fatalf("decode request body: %v", err)
		}

		w.Header().Set("Content-Type", "application/json")
		fmt.Fprint(w, `{
			"id":"resp_1",
			"object":"response",
			"created_at":1732083164,
			"status":"completed",
			"model":"gpt-4.1-mini",
			"output":[],
			"usage":{"input_tokens":4,"output_tokens":6,"total_tokens":10}
		}`)
	}))
	defer server.Close()

	a, err := NewOpenAIAdapter(&adapter.AdapterConfig{
		APIKey:  "test-key",
		BaseURL: server.URL + "/v1",
	})
	if err != nil {
		t.Fatalf("NewOpenAIAdapter() error = %v", err)
	}

	resp, err := a.CreateResponseRaw(context.Background(), &adapter.RawResponseRequest{
		Model: "gpt-4.1-mini",
		Body:  json.RawMessage(`{"model":"gpt-4.1-mini","input":"hello","tools":[{"type":"web_search_preview"}]}`),
	})
	if err != nil {
		t.Fatalf("CreateResponseRaw() error = %v", err)
	}

	if gotPath != "/v1/responses" {
		t.Fatalf("path = %q, want %q", gotPath, "/v1/responses")
	}
	if gotAuth != "Bearer test-key" {
		t.Fatalf("Authorization = %q, want Bearer test-key", gotAuth)
	}
	if gotPayload["model"] != "gpt-4.1-mini" || gotPayload["input"] != "hello" {
		t.Fatalf("payload = %#v, want raw responses body", gotPayload)
	}
	if resp.Usage == nil || resp.Usage.PromptTokens != 4 || resp.Usage.CompletionTokens != 6 || resp.Usage.TotalTokens != 10 {
		t.Fatalf("usage = %+v, want prompt=4 completion=6 total=10", resp.Usage)
	}
}

// A relay can answer a Responses request with an HTTP 200 error envelope (for
// example the agicto relay's container-reuse quota error). Returning it as a
// success hides the provider message and reports a missing usage downstream.
func TestOpenAIAdapterCreateResponseRaw_RejectsEmbeddedHTTP200Error(t *testing.T) {
	const providerBody = `{"error":{"code":null,"message":"用户 [1846] 配额不足","param":null,"type":"invalid_request_error"}}`

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprint(w, providerBody)
	}))
	defer server.Close()

	a, err := NewOpenAIAdapter(&adapter.AdapterConfig{
		APIKey:  "test-key",
		BaseURL: server.URL + "/v1",
	})
	if err != nil {
		t.Fatalf("NewOpenAIAdapter() error = %v", err)
	}

	resp, err := a.CreateResponseRaw(context.Background(), &adapter.RawResponseRequest{
		Model: "gpt-4o",
		Body:  json.RawMessage(`{"model":"gpt-4o","input":"hello","tools":[{"type":"code_interpreter","container":"cntr_x"}]}`),
	})
	if err == nil {
		t.Fatalf("CreateResponseRaw() error = nil, resp = %+v, want embedded provider error", resp)
	}
	var rawErr *adapter.RawProtocolError
	if !errors.As(err, &rawErr) {
		t.Fatalf("error = %T %v, want RawProtocolError", err, err)
	}
	if rawErr.StatusCode != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d", rawErr.StatusCode, http.StatusBadRequest)
	}
	if !strings.Contains(string(rawErr.Body), "配额不足") {
		t.Fatalf("raw error body = %s, want the provider message", rawErr.Body)
	}
}

func TestOpenAIAdapterCreateResponseStream_EmitsNativeResponsesEvents(t *testing.T) {
	t.Helper()

	var gotPayload map[string]any
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defer r.Body.Close()

		if r.URL.Path != "/v1/responses" {
			t.Fatalf("path = %q, want %q", r.URL.Path, "/v1/responses")
		}
		if err := json.NewDecoder(r.Body).Decode(&gotPayload); err != nil {
			t.Fatalf("decode request body: %v", err)
		}

		flusher, ok := w.(http.Flusher)
		if !ok {
			t.Fatal("response writer does not support flushing")
		}
		w.Header().Set("Content-Type", "text/event-stream")
		fmt.Fprint(w, "event: response.created\n")
		fmt.Fprint(w, "data: {\"type\":\"response.created\",\"response\":{\"id\":\"resp_1\",\"model\":\"gpt-4.1-mini\"}}\n\n")
		fmt.Fprint(w, "event: response.output_text.delta\n")
		fmt.Fprint(w, "data: {\"type\":\"response.output_text.delta\",\"delta\":\"ok\"}\n\n")
		fmt.Fprint(w, "event: response.completed\n")
		fmt.Fprint(w, "data: {\"type\":\"response.completed\",\"response\":{\"id\":\"resp_1\",\"usage\":{\"input_tokens\":3,\"output_tokens\":2,\"total_tokens\":5}}}\n\n")
		flusher.Flush()
	}))
	defer server.Close()

	a, err := NewOpenAIAdapter(&adapter.AdapterConfig{
		APIKey:  "test-key",
		BaseURL: server.URL + "/v1",
	})
	if err != nil {
		t.Fatalf("NewOpenAIAdapter() error = %v", err)
	}

	stream, err := a.CreateResponseStream(context.Background(), &adapter.RawResponseRequest{
		Model: "gpt-4.1-mini",
		Body:  json.RawMessage(`{"model":"gpt-4.1-mini","input":"hello"}`),
	})
	if err != nil {
		t.Fatalf("CreateResponseStream() error = %v", err)
	}

	var (
		events   []string
		usage    *adapter.Usage
		doneSeen bool
	)
	for event := range stream {
		if event.Error != nil {
			t.Fatalf("stream event error = %v", event.Error)
		}
		if event.Done {
			doneSeen = true
			usage = event.Usage
			continue
		}
		events = append(events, event.Event)
		if event.Usage != nil {
			usage = event.Usage
		}
	}

	if gotPayload["stream"] != true {
		t.Fatalf("payload.stream = %#v, want true", gotPayload["stream"])
	}
	if !doneSeen {
		t.Fatal("expected final done marker")
	}
	if len(events) != 3 || events[0] != "response.created" || events[1] != "response.output_text.delta" || events[2] != "response.completed" {
		t.Fatalf("events = %#v, want native responses events", events)
	}
	if usage == nil || usage.PromptTokens != 3 || usage.CompletionTokens != 2 || usage.TotalTokens != 5 {
		t.Fatalf("usage = %+v, want prompt=3 completion=2 total=5", usage)
	}
}

func TestOpenAIAdapterCreateImage_UsesGenerationsWithoutReferenceImage(t *testing.T) {
	t.Helper()

	var gotPayload map[string]interface{}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defer r.Body.Close()

		if r.URL.Path != "/v1/images/generations" {
			t.Fatalf("path = %q, want %q", r.URL.Path, "/v1/images/generations")
		}
		if ct := r.Header.Get("Content-Type"); !strings.HasPrefix(ct, "application/json") {
			t.Fatalf("Content-Type = %q, want application/json", ct)
		}
		if err := json.NewDecoder(r.Body).Decode(&gotPayload); err != nil {
			t.Fatalf("decode request body: %v", err)
		}

		w.Header().Set("Content-Type", "application/json")
		fmt.Fprint(w, `{"created":1732083164,"data":[{"url":"https://cdn.example.com/generated.png"}]}`)
	}))
	defer server.Close()

	a, err := NewOpenAIAdapter(&adapter.AdapterConfig{
		APIKey:  "test-key",
		BaseURL: server.URL + "/v1",
	})
	if err != nil {
		t.Fatalf("NewOpenAIAdapter() error = %v", err)
	}

	n := 2
	resp, err := a.CreateImage(context.Background(), &adapter.ImageRequest{
		Model:   "gpt-image-2",
		Prompt:  "draw a classroom",
		Size:    "1024x1024",
		Quality: "high",
		N:       &n,
	})
	if err != nil {
		t.Fatalf("CreateImage() error = %v", err)
	}

	if gotPayload["model"] != "gpt-image-2" || gotPayload["prompt"] != "draw a classroom" {
		t.Fatalf("payload = %#v, want model and prompt", gotPayload)
	}
	if gotPayload["size"] != "1024x1024" || gotPayload["quality"] != "high" || gotPayload["n"] != float64(2) {
		t.Fatalf("payload = %#v, want size, quality and n", gotPayload)
	}
	if len(resp.Data) != 1 || resp.Data[0].URL != "https://cdn.example.com/generated.png" {
		t.Fatalf("response data = %+v, want generated URL", resp.Data)
	}
}

func TestOpenAIAdapterCreateImage_UsesEditsWithReferenceImageBytes(t *testing.T) {
	t.Helper()

	const referenceContent = "PNGDATA"
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defer r.Body.Close()

		if r.URL.Path != "/v1/images/edits" {
			t.Fatalf("path = %q, want %q", r.URL.Path, "/v1/images/edits")
		}
		if ct := r.Header.Get("Content-Type"); !strings.HasPrefix(ct, "multipart/form-data;") {
			t.Fatalf("Content-Type = %q, want multipart/form-data", ct)
		}
		if err := r.ParseMultipartForm(1 << 20); err != nil {
			t.Fatalf("ParseMultipartForm() error = %v", err)
		}

		assertMultipartValue(t, r, "model", "gpt-image-2")
		assertMultipartValue(t, r, "prompt", "add a person")
		assertMultipartValue(t, r, "size", "1024x1024")
		assertMultipartValue(t, r, "n", "2")
		assertMultipartValue(t, r, "quality", "high")
		assertMultipartValue(t, r, "user", "account-1")
		assertMultipartValue(t, r, "input_fidelity", "high")
		assertMultipartValue(t, r, "background", "auto")
		assertMultipartValue(t, r, "output_format", "png")

		files := r.MultipartForm.File["image"]
		if len(files) != 1 {
			t.Fatalf("multipart image file count = %d, want 1", len(files))
		}
		if files[0].Filename != "reference.png" {
			t.Fatalf("image filename = %q, want reference.png", files[0].Filename)
		}
		if got := files[0].Header.Get("Content-Type"); got != "image/png" {
			t.Fatalf("image Content-Type = %q, want image/png", got)
		}
		file, err := files[0].Open()
		if err != nil {
			t.Fatalf("open multipart image file: %v", err)
		}
		defer file.Close()
		content, err := io.ReadAll(file)
		if err != nil {
			t.Fatalf("read multipart image file: %v", err)
		}
		if string(content) != referenceContent {
			t.Fatalf("image content = %q, want %q", content, referenceContent)
		}

		w.Header().Set("Content-Type", "application/json")
		fmt.Fprint(w, `{"created":1732083164,"data":[{"b64_json":"abc123"}]}`)
	}))
	defer server.Close()

	a, err := NewOpenAIAdapter(&adapter.AdapterConfig{
		APIKey:  "test-key",
		BaseURL: server.URL + "/v1",
	})
	if err != nil {
		t.Fatalf("NewOpenAIAdapter() error = %v", err)
	}

	n := 2
	resp, err := a.CreateImage(context.Background(), &adapter.ImageRequest{
		Model:                  "gpt-image-2",
		Prompt:                 "add a person",
		Size:                   "1024x1024",
		Quality:                "high",
		User:                   "account-1",
		N:                      &n,
		ReferenceImageBytes:    []byte(referenceContent),
		ReferenceImageFilename: "reference.png",
		ReferenceImageMimeType: "image/png",
		AdditionalParameters: map[string]interface{}{
			"background":        "auto",
			"output_format":     "png",
			"prompt":            "ignored",
			"extra_object":      map[string]string{"ignored": "true"},
			"extra_string_list": []string{"ignored"},
		},
	})
	if err != nil {
		t.Fatalf("CreateImage() error = %v", err)
	}

	if len(resp.Data) != 1 || resp.Data[0].B64JSON != "abc123" {
		t.Fatalf("response data = %+v, want b64_json abc123", resp.Data)
	}
}

func TestOpenAICompatibleAdapterCreateImage_AllowsQwenImageEdit(t *testing.T) {
	t.Helper()

	var gotModel string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defer r.Body.Close()

		if r.URL.Path != "/v1/images/edits" {
			t.Fatalf("path = %q, want %q", r.URL.Path, "/v1/images/edits")
		}
		if err := r.ParseMultipartForm(1 << 20); err != nil {
			t.Fatalf("ParseMultipartForm() error = %v", err)
		}
		gotModel = r.FormValue("model")
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprint(w, `{"created":1732083164,"data":[{"b64_json":"abc123"}]}`)
	}))
	defer server.Close()

	a, err := NewOpenAIAdapter(&adapter.AdapterConfig{
		ProviderName: "openai-compatible",
		APIKey:       "test-key",
		BaseURL:      server.URL + "/v1",
	})
	if err != nil {
		t.Fatalf("NewOpenAIAdapter() error = %v", err)
	}

	_, err = a.CreateImage(context.Background(), &adapter.ImageRequest{
		Model:               "qwen-image-2.0",
		Prompt:              "change style",
		Size:                "1024x1024",
		ReferenceImageBytes: []byte("PNGDATA"),
	})
	if err != nil {
		t.Fatalf("CreateImage() error = %v", err)
	}
	if gotModel != "qwen-image-2.0" {
		t.Fatalf("model = %q, want qwen-image-2.0", gotModel)
	}
}

func TestOpenAIAdapterCreateImage_RejectsQwenImageEditForOfficialOpenAI(t *testing.T) {
	t.Helper()

	a, err := NewOpenAIAdapter(&adapter.AdapterConfig{
		ProviderName: "openai",
		APIKey:       "test-key",
		BaseURL:      "https://api.example.com/v1",
	})
	if err != nil {
		t.Fatalf("NewOpenAIAdapter() error = %v", err)
	}

	_, err = a.CreateImage(context.Background(), &adapter.ImageRequest{
		Model:               "qwen-image-2.0",
		Prompt:              "change style",
		Size:                "1024x1024",
		ReferenceImageBytes: []byte("PNGDATA"),
	})
	if !errors.Is(err, adapter.ErrCapabilityUnsupported) {
		t.Fatalf("CreateImage() error = %v, want ErrCapabilityUnsupported", err)
	}
}

func TestOpenAIAdapterCreateImage_RejectsReferenceImageForDallE(t *testing.T) {
	t.Helper()

	a, err := NewOpenAIAdapter(&adapter.AdapterConfig{
		APIKey:  "test-key",
		BaseURL: "https://api.example.com/v1",
	})
	if err != nil {
		t.Fatalf("NewOpenAIAdapter() error = %v", err)
	}

	_, err = a.CreateImage(context.Background(), &adapter.ImageRequest{
		Model:               "dall-e-3",
		Prompt:              "add a person",
		Size:                "1024x1024",
		ReferenceImageBytes: []byte("PNGDATA"),
	})
	if !errors.Is(err, adapter.ErrCapabilityUnsupported) {
		t.Fatalf("CreateImage() error = %v, want ErrCapabilityUnsupported", err)
	}
}

func TestOpenAIAdapterCreateImage_RejectsUnsupportedEditSize(t *testing.T) {
	t.Helper()

	a, err := NewOpenAIAdapter(&adapter.AdapterConfig{
		APIKey:  "test-key",
		BaseURL: "https://api.example.com/v1",
	})
	if err != nil {
		t.Fatalf("NewOpenAIAdapter() error = %v", err)
	}

	_, err = a.CreateImage(context.Background(), &adapter.ImageRequest{
		Model:               "gpt-image-2",
		Prompt:              "add a person",
		Size:                "2048x2048",
		ReferenceImageBytes: []byte("PNGDATA"),
	})
	if !errors.Is(err, adapter.ErrInvalidRequest) {
		t.Fatalf("CreateImage() error = %v, want ErrInvalidRequest", err)
	}
}

func TestOpenAIAdapterCreateImage_RejectsReferenceImageURLWithoutBytes(t *testing.T) {
	t.Helper()

	a, err := NewOpenAIAdapter(&adapter.AdapterConfig{
		APIKey:  "test-key",
		BaseURL: "https://api.example.com/v1",
	})
	if err != nil {
		t.Fatalf("NewOpenAIAdapter() error = %v", err)
	}

	_, err = a.CreateImage(context.Background(), &adapter.ImageRequest{
		Model:             "gpt-image-2",
		Prompt:            "add a person",
		Size:              "1024x1024",
		ReferenceImageURL: "https://files.example.com/reference.png",
	})
	if !errors.Is(err, adapter.ErrInvalidRequest) {
		t.Fatalf("CreateImage() error = %v, want ErrInvalidRequest", err)
	}
}

func assertMultipartValue(t *testing.T, r *http.Request, key, want string) {
	t.Helper()

	values := r.MultipartForm.Value[key]
	if len(values) != 1 {
		t.Fatalf("multipart field %q count = %d, want 1", key, len(values))
	}
	if values[0] != want {
		t.Fatalf("multipart field %q = %q, want %q", key, values[0], want)
	}
}

func TestShouldTreatOpenAIListModelsAsCapabilityUnsupported(t *testing.T) {
	t.Helper()

	cases := []struct {
		name       string
		statusCode int
		body       string
		want       bool
	}{
		{
			name:       "404",
			statusCode: 404,
			body:       `{"error":{"message":"models endpoint is not implemented","code":"not_found"}}`,
			want:       true,
		},
		{
			name:       "405",
			statusCode: 405,
			body:       `{"error":{"message":"method not allowed","code":"method_not_allowed"}}`,
			want:       true,
		},
		{
			name:       "501",
			statusCode: 501,
			body:       `{"error":{"message":"not implemented","code":"not_implemented"}}`,
			want:       true,
		},
		{
			name:       "401",
			statusCode: 401,
			body:       `{"error":{"message":"bad key","code":"invalid_api_key"}}`,
			want:       false,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := shouldTreatOpenAIListModelsAsCapabilityUnsupported(tc.statusCode, []byte(tc.body))
			if got != tc.want {
				t.Fatalf("shouldTreatOpenAIListModelsAsCapabilityUnsupported(%d, %q) = %v, want %v", tc.statusCode, tc.body, got, tc.want)
			}
		})
	}
}

func TestNewAdapter_OpenAICompatibleKeyResolvesToOpenAIAdapter(t *testing.T) {
	t.Helper()

	instance, err := adapter.NewAdapter(&adapter.AdapterConfig{
		ProviderName: "openai-compatible",
		APIKey:       "test-key",
		BaseURL:      "https://proxy.example.com/v1",
	})
	if err != nil {
		t.Fatalf("NewAdapter() error = %v", err)
	}

	if _, ok := instance.(*OpenAIAdapter); !ok {
		t.Fatalf("instance type = %T, want *OpenAIAdapter", instance)
	}
}

func TestBuildChatPayloadToolChoice(t *testing.T) {
	req := &adapter.ChatRequest{
		Model: "deepseek-v4-pro",
		Messages: []adapter.Message{
			{Role: "user", Content: "查询北京明天天气"},
		},
		Tools: []adapter.Tool{{
			Type: "function",
			Function: adapter.Function{
				Name: "get_weather",
			},
		}},
		ToolChoice: map[string]interface{}{
			"type":     "function",
			"function": map[string]interface{}{"name": "get_weather"},
		},
	}

	agicto, err := NewOpenAIAdapter(&adapter.AdapterConfig{
		APIKey:       "test-key",
		ProviderName: "agicto",
		BaseURL:      "https://api.agicto.cn/v1",
	})
	if err != nil {
		t.Fatalf("NewOpenAIAdapter(agicto) error = %v", err)
	}
	if got := agicto.buildChatPayload(req)["tool_choice"]; got != "required" {
		t.Fatalf("agicto tool_choice = %#v, want required", got)
	}

	openai, err := NewOpenAIAdapter(&adapter.AdapterConfig{
		APIKey:       "test-key",
		ProviderName: "openai",
		BaseURL:      "https://api.openai.com/v1",
	})
	if err != nil {
		t.Fatalf("NewOpenAIAdapter(openai) error = %v", err)
	}
	got, _ := openai.buildChatPayload(req)["tool_choice"].(map[string]interface{})
	function, _ := got["function"].(map[string]string)
	if got["type"] != "function" || function["name"] != "get_weather" {
		t.Fatalf("openai tool_choice = %#v, want named function object", openai.buildChatPayload(req)["tool_choice"])
	}
}

// TestHandleOpenAICompatibleErrorHonoursConsoleContractCodes pins the platform
// channel contract: Console answers failures with the stable catalog identity
// at the top level of the envelope, and Pre must not flatten it into a generic
// retryable invocation failure. Each case also asserts the adapter preserves
// the Console code, so the egress identity can be traced back.
func TestHandleOpenAICompatibleErrorHonoursConsoleContractCodes(t *testing.T) {
	tests := []struct {
		name       string
		status     int
		consoleErr string
		legacyCode string
		wantErr    error
	}{
		{"none available", http.StatusServiceUnavailable, "llm.provider.none_available", "no_available_provider", adapter.ErrNoProviderAvailable},
		{"provider unavailable", http.StatusServiceUnavailable, "llm.provider.unavailable", "platform_channel_unavailable", adapter.ErrPlatformChannelUnavailable},
		{"timeout", http.StatusGatewayTimeout, "llm.provider.timeout", "upstream_timeout", adapter.ErrTimeout},
		{"auth failed", http.StatusBadGateway, "llm.provider.auth_failed", "upstream_auth_failed", adapter.ErrAuthFailed},
		{"provider rate limited", http.StatusTooManyRequests, "llm.provider.rate_limited", "upstream_error", adapter.ErrRateLimited},
		{"gateway rate limited", http.StatusTooManyRequests, "llm.rate_limit.exceeded", "rate_limit_exceeded", adapter.ErrRateLimitExceeded},
		{"pricing not configured", http.StatusPaymentRequired, "billing.model.pricing_not_configured", "pricing_not_configured", adapter.ErrPricingNotConfigured},
		{"balance insufficient", http.StatusForbidden, "llm.balance.insufficient", "insufficient_balance", adapter.ErrInsufficientBalance},
		{"exchange rate unavailable", http.StatusServiceUnavailable, "billing.exchange_rate.unavailable", "exchange_rate_unavailable", adapter.ErrExchangeRateUnavailable},
		{"settle failed", http.StatusInternalServerError, "billing.settle.failed", "upstream_error", adapter.ErrBillingSettleFailed},
		{"model not found", http.StatusNotFound, "llm.model.not_found", "model_not_found", adapter.ErrModelNotFound},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			body := fmt.Sprintf(`{"code":%q,"error_code":%q,"message":"console message","params":{},"retryable":false,"error_id":"e1","request_id":"r1","error":{"message":"console message","type":"server_error","param":null,"code":%q}}`,
				tt.consoleErr, tt.consoleErr, tt.legacyCode)
			err := handleOpenAICompatibleError(tt.status, []byte(body))
			if !errors.Is(err, tt.wantErr) {
				t.Fatalf("handleOpenAICompatibleError() error = %v, want %v", err, tt.wantErr)
			}
			var adapterErr *adapter.AdapterError
			if !errors.As(err, &adapterErr) {
				t.Fatalf("handleOpenAICompatibleError() error = %T %v, want AdapterError", err, err)
			}
			if adapterErr.Code != tt.consoleErr {
				t.Fatalf("AdapterError.Code = %q, want the Console code %q", adapterErr.Code, tt.consoleErr)
			}
		})
	}
}

// TestHandleOpenAICompatibleErrorKeepsProviderVocabularyForRealUpstreams guards
// the fallback: the nested code of a real provider must keep being classified
// by HTTP status, not by the platform channel's contract table.
func TestHandleOpenAICompatibleErrorKeepsProviderVocabularyForRealUpstreams(t *testing.T) {
	err := handleOpenAICompatibleError(
		http.StatusBadRequest,
		[]byte(`{"error":{"message":"bad request","type":"invalid_request_error","code":"model_not_found"}}`),
	)
	if errors.Is(err, adapter.ErrModelNotFound) {
		t.Fatalf("a real provider's 400 model_not_found must not become ErrModelNotFound: %v", err)
	}
	if !errors.Is(err, adapter.ErrInvalidRequest) {
		t.Fatalf("handleOpenAICompatibleError() error = %v, want ErrInvalidRequest", err)
	}
}

// TestConsoleContractCodeSurvivesTheRetryLoop composes the two halves Pre runs
// against the platform channel: the HTTP client's retry decision, driven by the
// Console envelope's retryable flag, and the adapter's classification of the
// final response. A non-retryable Console outcome must cost exactly one
// upstream call and must still surface as its own identity rather than a
// flattened, retryable invocation failure.
func TestConsoleContractCodeSurvivesTheRetryLoop(t *testing.T) {
	cases := []struct {
		name        string
		status      int
		consoleCode string
		legacyCode  string
		retryable   bool
		wantCalls   int
		wantErr     error
	}{
		{"auth failed is terminal and keeps its identity", http.StatusBadGateway, "llm.provider.auth_failed", "upstream_auth_failed", false, 1, adapter.ErrAuthFailed},
		{"none available retries and keeps its identity", http.StatusServiceUnavailable, "llm.provider.none_available", "no_available_provider", true, 2, adapter.ErrNoProviderAvailable},
		{"pricing not configured keeps its identity", http.StatusPaymentRequired, "billing.model.pricing_not_configured", "pricing_not_configured", false, 1, adapter.ErrPricingNotConfigured},
		{"exchange rate is terminal and keeps its identity", http.StatusServiceUnavailable, "billing.exchange_rate.unavailable", "exchange_rate_unavailable", false, 1, adapter.ErrExchangeRateUnavailable},
	}
	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			requestCount := 0
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				requestCount++
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(testCase.status)
				_, _ = fmt.Fprintf(w, `{"code":%q,"error_code":%q,"retryable":%t,"error":{"code":%q,"type":"api_error","message":"console message"}}`,
					testCase.consoleCode, testCase.consoleCode, testCase.retryable, testCase.legacyCode)
			}))
			defer server.Close()

			client := adapter.NewHTTPClientWithOptions(time.Second, 1, adapter.HTTPClientOptions{})
			response, requestErr := client.DoRequestDetailed(context.Background(), http.MethodPost, server.URL, nil, map[string]interface{}{})
			if requestCount != testCase.wantCalls {
				t.Fatalf("platform channel calls = %d, want %d", requestCount, testCase.wantCalls)
			}

			// A retryable 5xx exhausts its budget and comes back as an error; a
			// terminal response comes back as a body the adapter classifies.
			classified := requestErr
			if classified == nil {
				classified = handleOpenAICompatibleError(response.StatusCode, response.Body)
			}
			if !errors.Is(classified, testCase.wantErr) {
				t.Fatalf("classified error = %v, want %v", classified, testCase.wantErr)
			}
			if errors.Is(classified, adapter.ErrUpstreamError) {
				t.Fatalf("Console code %q was flattened into ErrUpstreamError", testCase.consoleCode)
			}
		})
	}
}
