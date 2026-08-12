package provider

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	adapter "github.com/zgiai/zgi/api/internal/modules/llm/protocol/adapters"
)

const doubaoSeedreamLiteTestModel = doubaoSeedreamModelPrefix + "-5-0-lite-260128"

func TestDoubaoAdapterChatCompletion_UsesArkChatCompletions(t *testing.T) {
	t.Helper()

	var (
		gotAuth string
		gotPath string
	)

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotAuth = r.Header.Get("Authorization")
		gotPath = r.URL.Path

		w.Header().Set("Content-Type", "application/json")
		fmt.Fprint(w, `{
			"id":"chatcmpl-doubao-1",
			"object":"chat.completion",
			"created":1732083164,
			"model":"doubao-seed-1-6-250615",
			"choices":[{"index":0,"message":{"role":"assistant","content":"ok"},"finish_reason":"stop"}],
			"usage":{"prompt_tokens":11,"completion_tokens":7,"total_tokens":18}
		}`)
	}))
	defer server.Close()

	a, err := NewDoubaoAdapter(&adapter.AdapterConfig{
		APIKey:  "test-key",
		BaseURL: server.URL + "/api/v3",
	})
	if err != nil {
		t.Fatalf("NewDoubaoAdapter() error = %v", err)
	}

	resp, err := a.ChatCompletion(context.Background(), &adapter.ChatRequest{
		Model:    "doubao-seed-1-6-250615",
		Messages: []adapter.Message{{Role: "user", Content: "hello"}},
	})
	if err != nil {
		t.Fatalf("ChatCompletion() error = %v", err)
	}

	if gotAuth != "Bearer test-key" {
		t.Fatalf("Authorization = %q, want %q", gotAuth, "Bearer test-key")
	}
	if gotPath != "/api/v3/chat/completions" {
		t.Fatalf("path = %q, want %q", gotPath, "/api/v3/chat/completions")
	}
	if resp.Model != "doubao-seed-1-6-250615" {
		t.Fatalf("response model = %q, want %q", resp.Model, "doubao-seed-1-6-250615")
	}
}

func TestDoubaoAdapterCreateResponse_UsesArkResponses(t *testing.T) {
	t.Helper()

	var (
		gotAuth string
		gotPath string
	)

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotAuth = r.Header.Get("Authorization")
		gotPath = r.URL.Path

		w.Header().Set("Content-Type", "application/json")
		fmt.Fprint(w, `{
			"id":"resp_123",
			"object":"response",
			"created_at":1732083164,
			"model":"doubao-seed-1-6-250615",
			"status":"completed",
			"output":[{"type":"message","role":"assistant","content":[{"type":"output_text","text":"ok"}]}],
			"usage":{"input_tokens":11,"output_tokens":7,"total_tokens":18}
		}`)
	}))
	defer server.Close()

	a, err := NewDoubaoAdapter(&adapter.AdapterConfig{
		APIKey:  "test-key",
		BaseURL: server.URL + "/api/v3",
	})
	if err != nil {
		t.Fatalf("NewDoubaoAdapter() error = %v", err)
	}

	resp, err := a.CreateResponse(context.Background(), &adapter.CreateResponseRequest{
		Model: "doubao-seed-1-6-250615",
		Input: "hello",
	})
	if err != nil {
		t.Fatalf("CreateResponse() error = %v", err)
	}

	if gotAuth != "Bearer test-key" {
		t.Fatalf("Authorization = %q, want %q", gotAuth, "Bearer test-key")
	}
	if gotPath != "/api/v3/responses" {
		t.Fatalf("path = %q, want %q", gotPath, "/api/v3/responses")
	}
	if resp.Model != "doubao-seed-1-6-250615" {
		t.Fatalf("response model = %q, want %q", resp.Model, "doubao-seed-1-6-250615")
	}
}

func TestDoubaoAdapterCreateEmbeddings_UsesArkEmbeddings(t *testing.T) {
	t.Helper()

	var (
		gotAuth    string
		gotPath    string
		gotPayload map[string]any
	)

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defer r.Body.Close()
		gotAuth = r.Header.Get("Authorization")
		gotPath = r.URL.Path
		if err := json.NewDecoder(r.Body).Decode(&gotPayload); err != nil {
			t.Fatalf("decode request body: %v", err)
		}

		w.Header().Set("Content-Type", "application/json")
		fmt.Fprint(w, `{
			"object":"list",
			"model":"doubao-embedding-text-240715",
			"data":[{"object":"embedding","index":0,"embedding":[0.1,0.2,0.3]}],
			"usage":{"prompt_tokens":7,"total_tokens":7}
		}`)
	}))
	defer server.Close()

	a, err := NewDoubaoAdapter(&adapter.AdapterConfig{
		APIKey:  "test-key",
		BaseURL: server.URL + "/api/v3",
	})
	if err != nil {
		t.Fatalf("NewDoubaoAdapter() error = %v", err)
	}

	resp, err := a.CreateEmbeddings(context.Background(), &adapter.EmbeddingsRequest{
		Model:      "doubao-embedding-text-240715",
		Input:      "hello",
		Dimensions: 1024,
	})
	if err != nil {
		t.Fatalf("CreateEmbeddings() error = %v", err)
	}

	if gotAuth != "Bearer test-key" {
		t.Fatalf("Authorization = %q, want %q", gotAuth, "Bearer test-key")
	}
	if gotPath != "/api/v3/embeddings" {
		t.Fatalf("path = %q, want %q", gotPath, "/api/v3/embeddings")
	}
	if got := gotPayload["dimensions"]; got != float64(1024) {
		t.Fatalf("payload.dimensions = %#v, want %d", got, 1024)
	}
	if resp.Model != "doubao-embedding-text-240715" {
		t.Fatalf("response model = %q, want %q", resp.Model, "doubao-embedding-text-240715")
	}
}

func TestDoubaoAdapterCreateImage_UsesArkImagesAndSeedreamNormalization(t *testing.T) {
	t.Helper()

	var (
		gotAuth    string
		gotPath    string
		gotPayload map[string]any
	)

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defer r.Body.Close()
		gotAuth = r.Header.Get("Authorization")
		gotPath = r.URL.Path
		if err := json.NewDecoder(r.Body).Decode(&gotPayload); err != nil {
			t.Fatalf("decode request body: %v", err)
		}

		w.Header().Set("Content-Type", "application/json")
		fmt.Fprint(w, `{
			"created":1732083164,
			"data":[{"url":"https://cdn.example.com/image.png"}]
		}`)
	}))
	defer server.Close()

	a, err := NewDoubaoAdapter(&adapter.AdapterConfig{
		APIKey:  "test-key",
		BaseURL: server.URL + "/api/v3",
	})
	if err != nil {
		t.Fatalf("NewDoubaoAdapter() error = %v", err)
	}

	resp, err := a.CreateImage(context.Background(), &adapter.ImageRequest{
		Model:  doubaoSeedreamLiteTestModel,
		Prompt: "a cat",
		Size:   "1024x1024",
	})
	if err != nil {
		t.Fatalf("CreateImage() error = %v", err)
	}

	if gotAuth != "Bearer test-key" {
		t.Fatalf("Authorization = %q, want %q", gotAuth, "Bearer test-key")
	}
	if gotPath != "/api/v3/images/generations" {
		t.Fatalf("path = %q, want %q", gotPath, "/api/v3/images/generations")
	}
	if got := gotPayload["size"]; got != "1920x1920" {
		t.Fatalf("payload.size = %#v, want %q", got, "1920x1920")
	}
	if _, exists := gotPayload[doubaoSeedreamSequentialGenerationKey]; exists {
		t.Fatalf("payload.%s exists for nil N: %#v", doubaoSeedreamSequentialGenerationKey, gotPayload[doubaoSeedreamSequentialGenerationKey])
	}
	if _, exists := gotPayload[doubaoSeedreamSequentialOptionsKey]; exists {
		t.Fatalf("payload.%s exists for nil N: %#v", doubaoSeedreamSequentialOptionsKey, gotPayload[doubaoSeedreamSequentialOptionsKey])
	}
	if len(resp.Data) != 1 || resp.Data[0].URL != "https://cdn.example.com/image.png" {
		t.Fatalf("response data = %#v, want generated image url", resp.Data)
	}
}

func TestDoubaoAdapterCreateImage_SeedreamMultiImageUsesSequentialOptions(t *testing.T) {
	t.Helper()

	var gotPayload map[string]any
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defer r.Body.Close()
		if err := json.NewDecoder(r.Body).Decode(&gotPayload); err != nil {
			t.Fatalf("decode request body: %v", err)
		}

		w.Header().Set("Content-Type", "application/json")
		fmt.Fprint(w, `{
			"created":1732083164,
			"data":[
				{"url":"https://cdn.example.com/image-1.png"},
				{"url":"https://cdn.example.com/image-2.png"},
				{"url":"https://cdn.example.com/image-3.png"}
			]
		}`)
	}))
	defer server.Close()

	a, err := NewDoubaoAdapter(&adapter.AdapterConfig{
		APIKey:  "test-key",
		BaseURL: server.URL + "/api/v3",
	})
	if err != nil {
		t.Fatalf("NewDoubaoAdapter() error = %v", err)
	}

	n := 3
	resp, err := a.CreateImage(context.Background(), &adapter.ImageRequest{
		Model:          doubaoSeedreamLiteTestModel,
		Prompt:         "a cat",
		Size:           "1024x1024",
		GenerationMode: "sequence",
		MaxImages:      &n,
	})
	if err != nil {
		t.Fatalf("CreateImage() error = %v", err)
	}

	if _, exists := gotPayload[doubaoImagePayloadKeyN]; exists {
		t.Fatalf("payload.%s must not represent a sequence upper bound", doubaoImagePayloadKeyN)
	}
	if got := gotPayload[doubaoImagePayloadKeyPrompt]; got != "a cat" {
		t.Fatalf("payload.%s = %#v, want unchanged prompt", doubaoImagePayloadKeyPrompt, got)
	}
	if got := gotPayload[doubaoSeedreamSequentialGenerationKey]; got != doubaoSeedreamSequentialGenerationAuto {
		t.Fatalf("payload.%s = %#v, want %q", doubaoSeedreamSequentialGenerationKey, got, doubaoSeedreamSequentialGenerationAuto)
	}
	options, ok := gotPayload[doubaoSeedreamSequentialOptionsKey].(map[string]any)
	if !ok {
		t.Fatalf("payload.%s = %#v, want object", doubaoSeedreamSequentialOptionsKey, gotPayload[doubaoSeedreamSequentialOptionsKey])
	}
	if got := options[doubaoSeedreamSequentialMaxImagesKey]; got != float64(n) {
		t.Fatalf("payload.%s.%s = %#v, want %d", doubaoSeedreamSequentialOptionsKey, doubaoSeedreamSequentialMaxImagesKey, got, n)
	}
	if len(resp.Data) != n {
		t.Fatalf("response data length = %d, want %d", len(resp.Data), n)
	}
}

func TestDoubaoAdapterCreateImage_SeedreamSingleImageDoesNotUseSequentialOptions(t *testing.T) {
	t.Helper()

	var gotPayload map[string]any
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defer r.Body.Close()
		if err := json.NewDecoder(r.Body).Decode(&gotPayload); err != nil {
			t.Fatalf("decode request body: %v", err)
		}

		w.Header().Set("Content-Type", "application/json")
		fmt.Fprint(w, `{
			"created":1732083164,
			"data":[{"url":"https://cdn.example.com/image.png"}]
		}`)
	}))
	defer server.Close()

	a, err := NewDoubaoAdapter(&adapter.AdapterConfig{
		APIKey:  "test-key",
		BaseURL: server.URL + "/api/v3",
	})
	if err != nil {
		t.Fatalf("NewDoubaoAdapter() error = %v", err)
	}

	_, err = a.CreateImage(context.Background(), &adapter.ImageRequest{
		Model:          doubaoSeedreamLiteTestModel,
		Prompt:         "a cat",
		Size:           "1024x1024",
		GenerationMode: "single",
	})
	if err != nil {
		t.Fatalf("CreateImage() error = %v", err)
	}

	if _, exists := gotPayload[doubaoImagePayloadKeyN]; exists {
		t.Fatalf("payload.%s must not be sent for Seedream single mode", doubaoImagePayloadKeyN)
	}
	if got := gotPayload[doubaoImagePayloadKeyPrompt]; got != "a cat" {
		t.Fatalf("payload.%s = %#v, want %q", doubaoImagePayloadKeyPrompt, got, "a cat")
	}
	if got := gotPayload[doubaoSeedreamSequentialGenerationKey]; got != "disabled" {
		t.Fatalf("payload.%s = %#v, want disabled", doubaoSeedreamSequentialGenerationKey, got)
	}
	if _, exists := gotPayload[doubaoSeedreamSequentialOptionsKey]; exists {
		t.Fatalf("payload.%s exists for N=1: %#v", doubaoSeedreamSequentialOptionsKey, gotPayload[doubaoSeedreamSequentialOptionsKey])
	}
}

func TestDoubaoAdapterCreateImage_AdditionalParametersOverrideSeedreamSequentialOptions(t *testing.T) {
	t.Helper()

	var gotPayload map[string]any
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defer r.Body.Close()
		if err := json.NewDecoder(r.Body).Decode(&gotPayload); err != nil {
			t.Fatalf("decode request body: %v", err)
		}

		w.Header().Set("Content-Type", "application/json")
		fmt.Fprint(w, `{
			"created":1732083164,
			"data":[{"url":"https://cdn.example.com/image.png"}]
		}`)
	}))
	defer server.Close()

	a, err := NewDoubaoAdapter(&adapter.AdapterConfig{
		APIKey:  "test-key",
		BaseURL: server.URL + "/api/v3",
	})
	if err != nil {
		t.Fatalf("NewDoubaoAdapter() error = %v", err)
	}

	n := 3
	const overrideSequentialGeneration = "disabled"
	const overrideMaxImages = 1
	_, err = a.CreateImage(context.Background(), &adapter.ImageRequest{
		Model:  doubaoSeedreamLiteTestModel,
		Prompt: "a cat",
		Size:   "1024x1024",
		N:      &n,
		AdditionalParameters: map[string]any{
			doubaoSeedreamSequentialGenerationKey: overrideSequentialGeneration,
			doubaoSeedreamSequentialOptionsKey: map[string]any{
				doubaoSeedreamSequentialMaxImagesKey: overrideMaxImages,
			},
		},
	})
	if err != nil {
		t.Fatalf("CreateImage() error = %v", err)
	}

	if got := gotPayload[doubaoSeedreamSequentialGenerationKey]; got != overrideSequentialGeneration {
		t.Fatalf("payload.%s = %#v, want %q", doubaoSeedreamSequentialGenerationKey, got, overrideSequentialGeneration)
	}
	options, ok := gotPayload[doubaoSeedreamSequentialOptionsKey].(map[string]any)
	if !ok {
		t.Fatalf("payload.%s = %#v, want object", doubaoSeedreamSequentialOptionsKey, gotPayload[doubaoSeedreamSequentialOptionsKey])
	}
	if got := options[doubaoSeedreamSequentialMaxImagesKey]; got != float64(overrideMaxImages) {
		t.Fatalf("payload.%s.%s = %#v, want %d", doubaoSeedreamSequentialOptionsKey, doubaoSeedreamSequentialMaxImagesKey, got, overrideMaxImages)
	}
}

func TestDoubaoAdapterCreateImage_NonSeedreamMultiImageDoesNotAppendPrompt(t *testing.T) {
	t.Helper()

	var gotPayload map[string]any
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defer r.Body.Close()
		if err := json.NewDecoder(r.Body).Decode(&gotPayload); err != nil {
			t.Fatalf("decode request body: %v", err)
		}

		w.Header().Set("Content-Type", "application/json")
		fmt.Fprint(w, `{
			"created":1732083164,
			"data":[{"url":"https://cdn.example.com/image.png"}]
		}`)
	}))
	defer server.Close()

	a, err := NewDoubaoAdapter(&adapter.AdapterConfig{
		APIKey:  "test-key",
		BaseURL: server.URL + "/api/v3",
	})
	if err != nil {
		t.Fatalf("NewDoubaoAdapter() error = %v", err)
	}

	n := 3
	_, err = a.CreateImage(context.Background(), &adapter.ImageRequest{
		Model:  "doubao-image-model",
		Prompt: "a cat",
		Size:   "1024x1024",
		N:      &n,
	})
	if err != nil {
		t.Fatalf("CreateImage() error = %v", err)
	}

	if got := gotPayload[doubaoImagePayloadKeyPrompt]; got != "a cat" {
		t.Fatalf("payload.%s = %#v, want %q", doubaoImagePayloadKeyPrompt, got, "a cat")
	}
	if _, exists := gotPayload[doubaoSeedreamSequentialGenerationKey]; exists {
		t.Fatalf("payload.%s exists for non-seedream model: %#v", doubaoSeedreamSequentialGenerationKey, gotPayload[doubaoSeedreamSequentialGenerationKey])
	}
	if _, exists := gotPayload[doubaoSeedreamSequentialOptionsKey]; exists {
		t.Fatalf("payload.%s exists for non-seedream model: %#v", doubaoSeedreamSequentialOptionsKey, gotPayload[doubaoSeedreamSequentialOptionsKey])
	}
}

func TestDoubaoAdapterCreateVideo_UsesArkVideoTasks(t *testing.T) {
	t.Helper()

	var (
		gotAuth    string
		gotPath    string
		gotPayload map[string]any
	)

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defer r.Body.Close()
		gotAuth = r.Header.Get("Authorization")
		gotPath = r.URL.Path
		if err := json.NewDecoder(r.Body).Decode(&gotPayload); err != nil {
			t.Fatalf("decode request body: %v", err)
		}

		w.Header().Set("Content-Type", "application/json")
		fmt.Fprint(w, `{
			"id":"task_video_123",
			"status":"running"
		}`)
	}))
	defer server.Close()

	a, err := NewDoubaoAdapter(&adapter.AdapterConfig{
		APIKey:  "test-key",
		BaseURL: server.URL + "/api/v3",
	})
	if err != nil {
		t.Fatalf("NewDoubaoAdapter() error = %v", err)
	}

	duration := 4
	generateAudio := false
	resp, err := a.CreateVideo(context.Background(), &adapter.VideoRequest{
		Model:         "doubao-seedance-2-0-mini-260615",
		Prompt:        "a bear by the sea",
		ImageURL:      "https://cdn.example.com/first.png",
		LastFrameURL:  "https://cdn.example.com/last.png",
		Ratio:         "1:1",
		Resolution:    "720p",
		Duration:      &duration,
		GenerateAudio: &generateAudio,
	})
	if err != nil {
		t.Fatalf("CreateVideo() error = %v", err)
	}

	if gotAuth != "Bearer test-key" {
		t.Fatalf("Authorization = %q, want %q", gotAuth, "Bearer test-key")
	}
	if gotPath != "/api/v3/contents/generations/tasks" {
		t.Fatalf("path = %q, want %q", gotPath, "/api/v3/contents/generations/tasks")
	}
	if got := gotPayload["model"]; got != "doubao-seedance-2-0-mini-260615" {
		t.Fatalf("payload.model = %#v, want model", got)
	}
	if got := gotPayload["resolution"]; got != "720p" {
		t.Fatalf("payload.resolution = %#v, want 720p", got)
	}
	if got := gotPayload["duration"]; got != float64(duration) {
		t.Fatalf("payload.duration = %#v, want %d", got, duration)
	}
	if got := gotPayload["generate_audio"]; got != generateAudio {
		t.Fatalf("payload.generate_audio = %#v, want false", got)
	}
	content, ok := gotPayload["content"].([]any)
	if !ok || len(content) != 3 {
		t.Fatalf("payload.content = %#v, want text plus two image entries", gotPayload["content"])
	}
	if resp.TaskID != "task_video_123" || resp.Status != "running" {
		t.Fatalf("response = %#v, want task id and running status", resp)
	}
}

func TestDoubaoAdapterCreateVideo_ReturnsBodyError(t *testing.T) {
	t.Helper()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprint(w, `{"error":{"code":"invalid_api_key","message":"Please Provide key from the platform!","type":"invalid_request_error"}}`)
	}))
	defer server.Close()

	a, err := NewDoubaoAdapter(&adapter.AdapterConfig{
		APIKey:  "test-key",
		BaseURL: server.URL + "/api/v3",
	})
	if err != nil {
		t.Fatalf("NewDoubaoAdapter() error = %v", err)
	}

	_, err = a.CreateVideo(context.Background(), &adapter.VideoRequest{
		Model:  "doubao-seedance-2-0-mini-260615",
		Prompt: "a bear by the sea",
	})
	if err == nil || !strings.Contains(err.Error(), "Please Provide key from the platform") {
		t.Fatalf("CreateVideo() error = %v, want upstream body error", err)
	}
}

func TestDoubaoAdapterCreateVideo_ReturnsInvalidImageMessageBeforeTaskIDFallback(t *testing.T) {
	t.Helper()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprint(w, `{
			"error": {
				"code": "InvalidParameter",
				"message": "Error while downloading image, error: expected the width to be at least 300px, but received a 153x161px image instead",
				"param": "image_url",
				"type": "BadRequest"
			}
		}`)
	}))
	defer server.Close()

	a, err := NewDoubaoAdapter(&adapter.AdapterConfig{
		APIKey:  "test-key",
		BaseURL: server.URL + "/api/v3",
	})
	if err != nil {
		t.Fatalf("NewDoubaoAdapter() error = %v", err)
	}

	_, err = a.CreateVideo(context.Background(), &adapter.VideoRequest{
		Model:  "doubao-seedance-2-0-mini-260615",
		Prompt: "a bear by the sea",
	})
	const want = "Error while downloading image, error: expected the width to be at least 300px, but received a 153x161px image instead"
	if err == nil || !strings.Contains(err.Error(), want) {
		t.Fatalf("CreateVideo() error = %v, want upstream invalid image message", err)
	}
	if strings.Contains(err.Error(), "task id") {
		t.Fatalf("CreateVideo() error = %v, should not fall back to task id error", err)
	}
}

func TestDoubaoAdapterGetVideoTask_UsesArkVideoTaskDetail(t *testing.T) {
	t.Helper()

	var gotPath string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.EscapedPath()
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprint(w, `{
			"task_id":"task_video_123",
			"status":"succeeded",
			"data":[{"url":"https://cdn.example.com/video.mp4"}]
		}`)
	}))
	defer server.Close()

	a, err := NewDoubaoAdapter(&adapter.AdapterConfig{
		APIKey:  "test-key",
		BaseURL: server.URL + "/api/v3",
	})
	if err != nil {
		t.Fatalf("NewDoubaoAdapter() error = %v", err)
	}

	resp, err := a.GetVideoTask(context.Background(), &adapter.VideoTaskRequest{TaskID: "task/video 123"})
	if err != nil {
		t.Fatalf("GetVideoTask() error = %v", err)
	}

	if gotPath != "/api/v3/contents/generations/tasks/task%2Fvideo%20123" {
		t.Fatalf("path = %q, want escaped task path", gotPath)
	}
	if resp.TaskID != "task_video_123" || resp.VideoURL != "https://cdn.example.com/video.mp4" {
		t.Fatalf("response = %#v, want task id and video url", resp)
	}
}
func TestDoubaoAdapterListModels_NormalizesRemoteCatalog(t *testing.T) {
	t.Helper()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v3/models" {
			t.Fatalf("path = %q, want %q", r.URL.Path, "/api/v3/models")
		}

		w.Header().Set("Content-Type", "application/json")
		fmt.Fprint(w, `{
			"data":[
				{"id":"doubao-seed-1-6-250615","created":1732083164,"owned_by":"bytedance"},
				{"id":"doubao-embedding-text-240715","created":1732083164,"owned_by":"bytedance"},
				{"id":"doubao-seedream-5-0-lite-260128","created":1732083164,"owned_by":"bytedance"}
			]
		}`)
	}))
	defer server.Close()

	a, err := NewDoubaoAdapter(&adapter.AdapterConfig{
		APIKey:  "test-key",
		BaseURL: server.URL + "/api/v3",
	})
	if err != nil {
		t.Fatalf("NewDoubaoAdapter() error = %v", err)
	}

	models, err := a.ListModels(context.Background(), "runtime-key")
	if err != nil {
		t.Fatalf("ListModels() error = %v", err)
	}
	if len(models) != 3 {
		t.Fatalf("len(models) = %d, want 3", len(models))
	}
	if models[0].Type != "chat" {
		t.Fatalf("models[0].Type = %q, want %q", models[0].Type, "chat")
	}
	if got := models[0].Capabilities; len(got) == 0 || got[0] != "chat" {
		t.Fatalf("models[0].Capabilities = %#v, want chat capabilities", got)
	}
	if models[1].Type != "embedding" {
		t.Fatalf("models[1].Type = %q, want %q", models[1].Type, "embedding")
	}
	if models[2].Type != "image" {
		t.Fatalf("models[2].Type = %q, want %q", models[2].Type, "image")
	}
	if model := normalizeDoubaoModel("doubao-seedance-2-0-mini-260615"); model.Type != "video" {
		t.Fatalf("seedance model type = %q, want video", model.Type)
	}
}

func TestDoubaoAdapterListModels_UnsupportedEndpointReturnsCapabilityUnsupported(t *testing.T) {
	t.Helper()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusNotFound)
		fmt.Fprint(w, `{"error":{"message":"not found","type":"not_found_error","code":"not_found"}}`)
	}))
	defer server.Close()

	a, err := NewDoubaoAdapter(&adapter.AdapterConfig{
		APIKey:  "test-key",
		BaseURL: server.URL + "/api/v3",
	})
	if err != nil {
		t.Fatalf("NewDoubaoAdapter() error = %v", err)
	}

	_, err = a.ListModels(context.Background(), "runtime-key")
	if !errors.Is(err, adapter.ErrCapabilityUnsupported) {
		t.Fatalf("ListModels() error = %v, want ErrCapabilityUnsupported", err)
	}
}

func TestDoubaoAdapterGetProviderInfo(t *testing.T) {
	a, err := NewDoubaoAdapter(&adapter.AdapterConfig{
		APIKey: "test-key",
	})
	if err != nil {
		t.Fatalf("NewDoubaoAdapter() error = %v", err)
	}

	info := a.GetProviderInfo()
	if info == nil {
		t.Fatal("GetProviderInfo() = nil, want non-nil")
	}
	if info.Name != "doubao" {
		t.Fatalf("info.Name = %q, want %q", info.Name, "doubao")
	}
	if info.BaseURL != doubaoDefaultBaseURL {
		t.Fatalf("info.BaseURL = %q, want %q", info.BaseURL, doubaoDefaultBaseURL)
	}
}
