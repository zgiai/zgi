package provider

import (
    "context"
    "fmt"
    "net/http"
    "net/http/httptest"
    "strings"
    "sync"
    "testing"

    adapter "github.com/zgiai/ginext/internal/modules/llm/protocol/adapters"
)

func TestCohereAdapterChatCompletionStream_UsesEventStreamAndEmitsChunks(t *testing.T) {
    t.Helper()

    var (
        mu         sync.Mutex
        seenAccept string
    )

    server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
        mu.Lock()
        seenAccept = r.Header.Get("Accept")
        mu.Unlock()

        flusher, ok := w.(http.Flusher)
        if !ok {
            t.Fatalf("response writer does not support flushing")
        }

        if strings.Contains(r.Header.Get("Accept"), "text/event-stream") {
            w.Header().Set("Content-Type", "text/event-stream")
            fmt.Fprint(w, "event: message-start\n")
            fmt.Fprint(w, "data: {\"id\":\"msg-1\",\"type\":\"message-start\",\"delta\":{\"message\":{\"role\":\"assistant\",\"content\":[]}}}\n\n")
            fmt.Fprint(w, "event: content-delta\n")
            fmt.Fprint(w, "data: {\"type\":\"content-delta\",\"index\":0,\"delta\":{\"message\":{\"content\":{\"text\":\"你\"}}}}\n\n")
            fmt.Fprint(w, "event: content-delta\n")
            fmt.Fprint(w, "data: {\"type\":\"content-delta\",\"index\":0,\"delta\":{\"message\":{\"content\":{\"text\":\"好\"}}}}\n\n")
            fmt.Fprint(w, "event: message-end\n")
            fmt.Fprint(w, "data: {\"type\":\"message-end\",\"delta\":{\"finish_reason\":\"COMPLETE\",\"usage\":{\"tokens\":{\"input_tokens\":28,\"output_tokens\":52},\"billed_units\":{\"input_tokens\":19,\"output_tokens\":52}}}}\n\n")
            flusher.Flush()
            return
        }

        w.Header().Set("Content-Type", "application/stream+json")
        fmt.Fprintln(w, `{"id":"msg-1","type":"message-start","delta":{"message":{"role":"assistant","content":[]}}}`)
        fmt.Fprintln(w, `{"type":"content-delta","index":0,"delta":{"message":{"content":{"text":"你"}}}}`)
        fmt.Fprintln(w, `{"type":"content-delta","index":0,"delta":{"message":{"content":{"text":"好"}}}}`)
        fmt.Fprintln(w, `{"type":"message-end","delta":{"finish_reason":"COMPLETE","usage":{"tokens":{"input_tokens":28,"output_tokens":52},"billed_units":{"input_tokens":19,"output_tokens":52}}}}`)
        flusher.Flush()
    }))
    defer server.Close()

    a, err := NewCohereAdapter(&adapter.AdapterConfig{
        APIKey:  "test-key",
        BaseURL: server.URL,
    })
    if err != nil {
        t.Fatalf("NewCohereAdapter() error = %v", err)
    }

    stream, err := a.ChatCompletionStream(context.Background(), &adapter.ChatRequest{
        Model: "c4ai-aya-vision-32b",
        Messages: []adapter.Message{{Role: "user", Content: "你好"}},
        Stream: true,
        StreamOptions: &adapter.StreamOptions{IncludeUsage: true},
    })
    if err != nil {
        t.Fatalf("ChatCompletionStream() error = %v", err)
    }

    var (
        parts      []string
        finish     string
        finalUsage *adapter.Usage
        doneSeen   bool
    )
    for resp := range stream {
        for _, choice := range resp.Choices {
            if text, ok := choice.Delta.Content.(string); ok && text != "" {
                parts = append(parts, text)
            }
            if choice.FinishReason != "" {
                finish = choice.FinishReason
            }
        }
        if resp.Usage != nil {
            finalUsage = resp.Usage
        }
        if resp.Done {
            doneSeen = true
        }
    }

    mu.Lock()
    gotAccept := seenAccept
    mu.Unlock()

    if !strings.Contains(gotAccept, "text/event-stream") {
        t.Fatalf("expected stream request Accept header to include text/event-stream, got %q", gotAccept)
    }
    if got := strings.Join(parts, ""); got != "你好" {
        t.Fatalf("expected streamed text %q, got %q", "你好", got)
    }
    if !doneSeen {
        t.Fatalf("expected final done chunk")
    }
    if finish != "stop" {
        t.Fatalf("expected finish reason %q, got %q", "stop", finish)
    }
    if finalUsage == nil || finalUsage.TotalTokens != 80 || finalUsage.PromptTokens != 28 || finalUsage.CompletionTokens != 52 {
        t.Fatalf("expected final usage prompt=28 completion=52 total=80, got %+v", finalUsage)
    }
}
