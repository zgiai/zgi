package vlm

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	extractcommon "github.com/zgiai/zgi/api/internal/capabilities/contentparse/engines/hyperparse/pkg/providers/common"
)

func TestToDocumentResult_Shape(t *testing.T) {
	batches := []*batchResult{
		{
			BatchIndex: 0,
			PageOffset: 0,
			Model:      "gemini-2.5-flash",
			Chunks: []chunkItem{
				{Type: "text", Page: 0, Bbox: []float64{100, 200, 500, 800}, Text: "hello", Markdown: "hello"},
			},
		},
	}
	doc, err := toDocumentResult("sample.pdf", 1, batches)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if doc.FileName != "sample.pdf" {
		t.Fatalf("filename mismatch: %q", doc.FileName)
	}
	if doc.PageCount != 1 || len(doc.Pages) != 1 {
		t.Fatalf("page shape mismatch: page_count=%d pages=%d", doc.PageCount, len(doc.Pages))
	}
	if len(doc.Chunks) != 1 {
		t.Fatalf("expected 1 chunk, got %d", len(doc.Chunks))
	}
	ch := doc.Chunks[0]
	if ch.BBox == nil {
		t.Fatalf("expected bbox")
	}
	if ch.Precision != "reliable" {
		t.Fatalf("expected precision=reliable, got %q", ch.Precision)
	}
	if doc.Source == "" {
		t.Fatalf("expected source")
	}
	if doc.Source != "vlm:gemini-2.5-flash" {
		t.Fatalf("source=%q", doc.Source)
	}
}

func TestImageFilenameAndMIME(t *testing.T) {
	cases := []struct {
		name     string
		filename string
		isImage  bool
		mime     string
	}{
		{name: "png", filename: "a.png", isImage: true, mime: "image/png"},
		{name: "jpg", filename: "a.jpg", isImage: true, mime: "image/jpeg"},
		{name: "jpeg", filename: "a.jpeg", isImage: true, mime: "image/jpeg"},
		{name: "webp", filename: "a.webp", isImage: true, mime: "image/webp"},
		{name: "tiff", filename: "a.tiff", isImage: true, mime: "image/tiff"},
		{name: "pdf", filename: "a.pdf", isImage: false, mime: "image/jpeg"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := isImageFilename(tc.filename); got != tc.isImage {
				t.Fatalf("isImageFilename(%q)=%v want=%v", tc.filename, got, tc.isImage)
			}
			if got := imageMIME(tc.filename); got != tc.mime {
				t.Fatalf("imageMIME(%q)=%q want=%q", tc.filename, got, tc.mime)
			}
		})
	}
}

func TestParseBytesUsesImageUnderstandingPromptAndAcceptsMarkdown(t *testing.T) {
	var captured ChatCompletionRequest
	client := NewWithChatCompletion("vision-model", "", func(ctx context.Context, req ChatCompletionRequest) (*ChatCompletionResponse, error) {
		captured = req
		return &ChatCompletionResponse{
			Content: "# 医院楼层导览\n\n## 8F\n\n- 儿科一病区",
			Model:   "vision-model",
		}, nil
	})

	doc, err := client.ParseBytes(context.Background(), "directory.jpg", []byte("image-bytes"), extractcommon.ParseOptions{})
	if err != nil {
		t.Fatalf("ParseBytes() error = %v", err)
	}
	if len(captured.UserContent) < 2 {
		t.Fatalf("user content = %#v, want prompt and image", captured.UserContent)
	}
	if captured.ResponseFormat != "" {
		t.Fatalf("response format = %q, want plain text", captured.ResponseFormat)
	}
	prompt, _ := captured.UserContent[0]["text"].(string)
	for _, required := range []string{"ONE coherent", "Preserve the source language exactly", "Flowchart or process diagram", "directly as Markdown"} {
		if !strings.Contains(prompt, required) {
			t.Fatalf("image prompt missing %q", required)
		}
	}
	if len(doc.Chunks) != 1 {
		t.Fatalf("chunks = %d, want one consolidated image document", len(doc.Chunks))
	}
	chunk := doc.Chunks[0]
	if chunk.Type != "text" || !strings.Contains(chunk.Markdown, "# 医院楼层导览") || !strings.Contains(chunk.Markdown, "儿科一病区") {
		t.Fatalf("consolidated chunk = %#v", chunk)
	}
	if chunk.BBox == nil || chunk.BBox.Left != 0 || chunk.BBox.Top != 0 || chunk.BBox.Right != 1 || chunk.BBox.Bottom != 1 {
		t.Fatalf("consolidated bbox = %#v, want full image", chunk.BBox)
	}
}

func TestParseBytesRejectsNonImageInput(t *testing.T) {
	client := NewWithChatCompletion("vision-model", "", func(ctx context.Context, req ChatCompletionRequest) (*ChatCompletionResponse, error) {
		t.Fatal("vision model must not be called for non-image input")
		return nil, nil
	})

	_, err := client.ParseBytes(context.Background(), "document.pdf", []byte("pdf"), extractcommon.ParseOptions{})
	if err == nil || !strings.Contains(err.Error(), "unsupported non-image input") {
		t.Fatalf("ParseBytes() error = %v, want non-image rejection", err)
	}
}

func TestCallContentOnceRequestsJSONObject(t *testing.T) {
	var captured ChatCompletionRequest
	client := NewWithChatCompletion("vision-model", "", func(ctx context.Context, req ChatCompletionRequest) (*ChatCompletionResponse, error) {
		captured = req
		return &ChatCompletionResponse{
			Content: `{"chunks":[{"type":"text","page":0,"text":"hello"}]}`,
			Model:   "vision-model",
		}, nil
	})

	_, err := client.callContentOnce(context.Background(), []map[string]any{{"type": "text", "text": "parse PDF"}}, "vision-model", "gemini")
	if err != nil {
		t.Fatalf("callContentOnce() error = %v", err)
	}
	if captured.ResponseFormat != "json_object" {
		t.Fatalf("response format = %q, want json_object", captured.ResponseFormat)
	}
}

func TestCompleteOmitsResponseFormatForPlainTextHTTPRequests(t *testing.T) {
	var captured map[string]any
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if err := json.NewDecoder(r.Body).Decode(&captured); err != nil {
			t.Errorf("decode request: %v", err)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"choices":[{"finish_reason":"stop","message":{"content":"# Markdown"}}],"usage":{"prompt_tokens":2}}`))
	}))
	defer server.Close()

	t.Setenv("VLM_API_KEY", "test-key")
	t.Setenv("VLM_BASE_URL", server.URL)
	client := New()
	content, _, _, _, err := client.complete(context.Background(), []map[string]any{{"type": "text", "text": "describe image"}}, "vision-model", "")
	if err != nil {
		t.Fatalf("complete() error = %v", err)
	}
	if content != "# Markdown" {
		t.Fatalf("content = %q, want Markdown", content)
	}
	if _, exists := captured["response_format"]; exists {
		t.Fatalf("request unexpectedly contains response_format: %#v", captured)
	}
}

func TestBuildPDFPageImageContentUsesImageURLParts(t *testing.T) {
	content := buildPDFPageImageContent("sample.pdf", []string{
		"data:image/png;base64,page1",
		"data:image/png;base64,page2",
	})

	if len(content) != 3 {
		t.Fatalf("content length=%d want 3", len(content))
	}
	if got := content[0]["type"]; got != "text" {
		t.Fatalf("first part type=%v want text", got)
	}
	for idx, part := range content[1:] {
		if got := part["type"]; got != "image_url" {
			t.Fatalf("part %d type=%v want image_url", idx+1, got)
		}
		if _, ok := part["file"]; ok {
			t.Fatalf("part %d unexpectedly contains file payload: %#v", idx+1, part)
		}
		imageURL, ok := part["image_url"].(map[string]any)
		if !ok || imageURL["url"] == "" {
			t.Fatalf("part %d image_url=%#v want url", idx+1, part["image_url"])
		}
	}
}

func TestVLMConfigPrefersUnifiedVLMEnv(t *testing.T) {
	clearVLMEnv(t)
	t.Setenv("VLM_API_KEY", "vlm-key")
	t.Setenv("DASHSCOPE_API_KEY", "dash-key")
	t.Setenv("GEMINI_API_KEY", "gem-key")
	t.Setenv("VLM_BASE_URL", "https://vlm.example/v1/")
	t.Setenv("DASHSCOPE_BASE_URL", "https://dash.example/v1")
	t.Setenv("GEMINI_BASE_URL", "https://gem.example/v1")
	t.Setenv("VLM_MODEL", "qwen-vl-max-latest")
	t.Setenv("DASHSCOPE_VL_MODEL", "dash-model")
	t.Setenv("GEMINI_MODEL", "gem-model")
	t.Setenv("VLM_MODEL_FAST", "qwen-vl-plus")
	t.Setenv("DASHSCOPE_VL_MODEL_FAST", "dash-fast")
	t.Setenv("GEMINI_FALLBACK_MODEL", "gem-fast")

	if got := apiKey(); got != "vlm-key" {
		t.Fatalf("apiKey=%q", got)
	}
	if got := baseURL(); got != "https://vlm.example/v1" {
		t.Fatalf("baseURL=%q", got)
	}
	if got := model(); got != "qwen-vl-max-latest" {
		t.Fatalf("model=%q", got)
	}
	if got := fallbackModel(); got != "qwen-vl-plus" {
		t.Fatalf("fallbackModel=%q", got)
	}
}

func TestVLMConfigFallsBackToGeminiCompatEnv(t *testing.T) {
	clearVLMEnv(t)
	t.Setenv("GEMINI_API_KEY", "gem-key")
	t.Setenv("GEMINI_BASE_URL", "https://gem.example/v1")
	t.Setenv("GEMINI_MODEL", "gem-model")
	t.Setenv("GEMINI_FALLBACK_MODEL", "gem-fast")

	if got := apiKey(); got != "gem-key" {
		t.Fatalf("apiKey=%q", got)
	}
	if got := baseURL(); got != "https://gem.example/v1" {
		t.Fatalf("baseURL=%q", got)
	}
	if got := model(); got != "gem-model" {
		t.Fatalf("model=%q", got)
	}
	if got := fallbackModel(); got != "gem-fast" {
		t.Fatalf("fallbackModel=%q", got)
	}
}

func TestVLMConfigRequiresExplicitBaseURLAndModel(t *testing.T) {
	clearVLMEnv(t)
	t.Setenv("VLM_API_KEY", "vlm-key")

	if got := baseURL(); got != "" {
		t.Fatalf("baseURL=%q", got)
	}
	if _, err := requiredBaseURL(); err == nil {
		t.Fatal("expected missing base URL error")
	}
	if got := model(); got != "" {
		t.Fatalf("model=%q", got)
	}
	if got := fallbackModel(); got != "" {
		t.Fatalf("fallbackModel=%q", got)
	}
}

func clearVLMEnv(t *testing.T) {
	t.Helper()
	for _, key := range []string{
		"VLM_API_KEY",
		"DASHSCOPE_API_KEY",
		"GEMINI_API_KEY",
		"VLM_BASE_URL",
		"DASHSCOPE_BASE_URL",
		"GEMINI_BASE_URL",
		"VLM_MODEL",
		"DASHSCOPE_VL_MODEL",
		"GEMINI_MODEL",
		"VLM_MODEL_FAST",
		"DASHSCOPE_VL_MODEL_FAST",
		"GEMINI_FALLBACK_MODEL",
	} {
		t.Setenv(key, "")
	}
}
