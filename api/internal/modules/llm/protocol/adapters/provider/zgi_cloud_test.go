package provider

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	adapter "github.com/zgiai/zgi/api/internal/modules/llm/protocol/adapters"
)

func TestZGICloudAdapterTranscribeStreamsPCMAndUnwrapsResponse(t *testing.T) {
	requestCount := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requestCount++
		if r.URL.Path != "/v1/internal/audio/transcriptions" {
			t.Errorf("path = %q, want /v1/internal/audio/transcriptions", r.URL.Path)
		}
		if got := r.Header.Get("Content-Type"); got != "audio/pcm" {
			t.Errorf("Content-Type = %q, want audio/pcm", got)
		}
		if got := r.Header.Get("X-ZGI-Request-ID"); got != "11111111-1111-1111-1111-111111111111" {
			t.Errorf("X-ZGI-Request-ID = %q", got)
		}
		if got := r.Header.Get("X-ZGI-Model-Name"); got != "volc.seedasr.sauc.duration" {
			t.Errorf("X-ZGI-Model-Name = %q", got)
		}
		body, err := io.ReadAll(r.Body)
		if err != nil {
			t.Errorf("read body: %v", err)
		}
		if got := string(body); got != "pcm-audio" {
			t.Errorf("body = %q, want pcm-audio", got)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = fmt.Fprint(w, `{"code":0,"message":"success","data":{"request_id":"11111111-1111-1111-1111-111111111111","text":"editable transcript"}}`)
	}))
	defer server.Close()

	a, err := NewZGICloudAdapter(&adapter.AdapterConfig{
		BaseURL:    server.URL + "/v1/internal",
		AuthHook:   func(*http.Request) {},
		MaxRetries: 3,
	})
	if err != nil {
		t.Fatalf("NewZGICloudAdapter() error = %v", err)
	}

	response, err := a.Transcribe(t.Context(), &adapter.TranscriptionRequest{
		RequestID: "11111111-1111-1111-1111-111111111111",
		Model:     "volc.seedasr.sauc.duration",
		Audio:     bytes.NewReader([]byte("pcm-audio")),
	})
	if err != nil {
		t.Fatalf("Transcribe() error = %v", err)
	}
	if response.RequestID != "11111111-1111-1111-1111-111111111111" || response.Text != "editable transcript" {
		t.Fatalf("response = %#v", response)
	}
	if requestCount != 1 {
		t.Fatalf("request count = %d, want 1", requestCount)
	}
}

func TestZGICloudAdapterTranscribeDoesNotRetryProviderFailure(t *testing.T) {
	requestCount := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		requestCount++
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusServiceUnavailable)
		_, _ = fmt.Fprint(w, `{"code":503,"message":"transcription model is unavailable"}`)
	}))
	defer server.Close()

	a, err := NewZGICloudAdapter(&adapter.AdapterConfig{
		BaseURL:    server.URL + "/v1/internal",
		AuthHook:   func(*http.Request) {},
		MaxRetries: 3,
	})
	if err != nil {
		t.Fatalf("NewZGICloudAdapter() error = %v", err)
	}

	_, err = a.Transcribe(t.Context(), &adapter.TranscriptionRequest{
		RequestID: "11111111-1111-1111-1111-111111111111",
		Model:     "volc.seedasr.sauc.duration",
		Audio:     bytes.NewReader([]byte("pcm-audio")),
	})
	if !errors.Is(err, adapter.ErrPlatformChannelUnavailable) {
		t.Fatalf("Transcribe() error = %v, want ErrPlatformChannelUnavailable", err)
	}
	if requestCount != 1 {
		t.Fatalf("request count = %d, want one non-retryable attempt", requestCount)
	}
}

func TestZGICloudAdapterGenerateSpeechStreamsMP3Once(t *testing.T) {
	requestCount := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requestCount++
		if got, want := r.URL.Path, "/v1/internal/audio/speech"; got != want {
			t.Errorf("path = %q, want %q", got, want)
		}
		if got, want := r.Header.Get("X-ZGI-Request-ID"), "11111111-1111-1111-1111-111111111111"; got != want {
			t.Errorf("X-ZGI-Request-ID = %q, want %q", got, want)
		}
		if got, want := r.Header.Get("X-ZGI-Model-Name"), "seed-tts-2.0"; got != want {
			t.Errorf("X-ZGI-Model-Name = %q, want %q", got, want)
		}
		if got, want := r.Header.Get("Accept"), "audio/mpeg"; got != want {
			t.Errorf("Accept = %q, want %q", got, want)
		}
		var request adapter.SpeechRequest
		if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
			t.Errorf("decode request: %v", err)
		}
		if request.RequestID != "" || request.Model != "seed-tts-2.0" || request.Input != "你好。" || request.Voice != "verified-voice" || request.ResponseFormat != "mp3" {
			t.Errorf("request = %#v", request)
		}

		w.Header().Set("Content-Type", "audio/mpeg")
		_, _ = w.Write([]byte("MP3-A"))
		w.(http.Flusher).Flush()
		_, _ = w.Write([]byte("MP3-B"))
	}))
	defer server.Close()

	a, err := NewZGICloudAdapter(&adapter.AdapterConfig{
		BaseURL:    server.URL + "/v1/internal",
		AuthHook:   func(*http.Request) {},
		MaxRetries: 3,
	})
	if err != nil {
		t.Fatalf("NewZGICloudAdapter() error = %v", err)
	}

	var audio bytes.Buffer
	err = a.GenerateSpeech(t.Context(), &adapter.SpeechRequest{
		RequestID:      "11111111-1111-1111-1111-111111111111",
		Model:          "seed-tts-2.0",
		Input:          "你好。",
		Voice:          "verified-voice",
		ResponseFormat: "mp3",
	}, &audio)
	if err != nil {
		t.Fatalf("GenerateSpeech() error = %v", err)
	}
	if got, want := audio.String(), "MP3-AMP3-B"; got != want {
		t.Fatalf("GenerateSpeech() audio = %q, want %q", got, want)
	}
	if got, want := requestCount, 1; got != want {
		t.Fatalf("request count = %d, want %d", got, want)
	}
}

func TestZGICloudAdapterGenerateSpeechDoesNotRetryProviderFailure(t *testing.T) {
	requestCount := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		requestCount++
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusServiceUnavailable)
		_, _ = fmt.Fprint(w, `{"code":503,"message":"speech model is unavailable"}`)
	}))
	defer server.Close()

	a, err := NewZGICloudAdapter(&adapter.AdapterConfig{
		BaseURL:    server.URL + "/v1/internal",
		AuthHook:   func(*http.Request) {},
		MaxRetries: 3,
	})
	if err != nil {
		t.Fatalf("NewZGICloudAdapter() error = %v", err)
	}

	err = a.GenerateSpeech(t.Context(), &adapter.SpeechRequest{
		RequestID:      "11111111-1111-1111-1111-111111111111",
		Model:          "seed-tts-2.0",
		Input:          "text",
		Voice:          "verified-voice",
		ResponseFormat: "mp3",
	}, io.Discard)
	if !errors.Is(err, adapter.ErrPlatformChannelUnavailable) {
		t.Fatalf("GenerateSpeech() error = %v, want ErrPlatformChannelUnavailable", err)
	}
	if got, want := requestCount, 1; got != want {
		t.Fatalf("request count = %d, want %d", got, want)
	}
}

func TestZGICloudAdapterGenerateSpeechPropagatesCancellationAfterDispatch(t *testing.T) {
	started := make(chan struct{})
	var requestCount atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(_ http.ResponseWriter, r *http.Request) {
		requestCount.Add(1)
		_, _ = io.Copy(io.Discard, r.Body)
		close(started)
		<-r.Context().Done()
	}))
	defer server.Close()

	a, err := NewZGICloudAdapter(&adapter.AdapterConfig{
		BaseURL:  server.URL + "/v1/internal",
		AuthHook: func(*http.Request) {},
	})
	if err != nil {
		t.Fatalf("NewZGICloudAdapter() error = %v", err)
	}

	ctx, cancel := context.WithCancel(t.Context())
	errCh := make(chan error, 1)
	go func() {
		errCh <- a.GenerateSpeech(ctx, &adapter.SpeechRequest{
			RequestID:      "11111111-1111-1111-1111-111111111111",
			Model:          "seed-tts-2.0",
			Input:          "text",
			Voice:          "verified-voice",
			ResponseFormat: "mp3",
		}, io.Discard)
	}()

	select {
	case <-started:
	case <-time.After(time.Second):
		t.Fatal("speech request was not dispatched")
	}
	cancel()
	select {
	case err := <-errCh:
		if !errors.Is(err, context.Canceled) {
			t.Fatalf("GenerateSpeech() error = %v, want context canceled", err)
		}
	case <-time.After(time.Second):
		t.Fatal("GenerateSpeech() did not stop after cancellation")
	}
	if got, want := requestCount.Load(), int32(1); got != want {
		t.Fatalf("request count = %d, want %d", got, want)
	}
}

func TestZGICloudAdapterGenerateSpeechDoesNotRetryInterruptedStream(t *testing.T) {
	var requestCount atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		requestCount.Add(1)
		w.Header().Set("Content-Type", "audio/mpeg")
		w.Header().Set("Content-Length", "20")
		_, _ = w.Write([]byte("partial"))
		w.(http.Flusher).Flush()
	}))
	defer server.Close()

	a, err := NewZGICloudAdapter(&adapter.AdapterConfig{
		BaseURL:    server.URL + "/v1/internal",
		AuthHook:   func(*http.Request) {},
		MaxRetries: 3,
	})
	if err != nil {
		t.Fatalf("NewZGICloudAdapter() error = %v", err)
	}

	var audio bytes.Buffer
	err = a.GenerateSpeech(t.Context(), &adapter.SpeechRequest{
		RequestID:      "11111111-1111-1111-1111-111111111111",
		Model:          "seed-tts-2.0",
		Input:          "text",
		Voice:          "verified-voice",
		ResponseFormat: "mp3",
	}, &audio)
	if err == nil {
		t.Fatal("GenerateSpeech() error = nil, want interrupted stream error")
	}
	if got, want := audio.String(), "partial"; got != want {
		t.Fatalf("GenerateSpeech() audio = %q, want %q", got, want)
	}
	if got, want := requestCount.Load(), int32(1); got != want {
		t.Fatalf("request count = %d, want %d", got, want)
	}
}

func TestZGICloudAdapterMapsPlatformChannelUnavailableAcrossProtocols(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadGateway)
		if strings.Contains(r.URL.Path, "/anthropic/") {
			_, _ = fmt.Fprint(w, `{"type":"error","error":{"type":"platform_channel_unavailable","message":"Platform model service is temporarily unavailable"}}`)
			return
		}
		_, _ = fmt.Fprint(w, `{"error":{"message":"Platform model service is temporarily unavailable","type":"server_error","code":"platform_channel_unavailable"}}`)
	}))
	defer server.Close()

	a, err := NewZGICloudAdapter(&adapter.AdapterConfig{
		BaseURL:  server.URL + "/v1/internal",
		AuthHook: func(*http.Request) {},
	})
	if err != nil {
		t.Fatalf("NewZGICloudAdapter() error = %v", err)
	}

	tests := []struct {
		name string
		call func() error
	}{
		{
			name: "chat",
			call: func() error {
				_, err := a.ChatCompletion(context.Background(), &adapter.ChatRequest{Model: "gpt-5"})
				return err
			},
		},
		{
			name: "chat stream",
			call: func() error {
				_, err := a.ChatCompletionStream(context.Background(), &adapter.ChatRequest{Model: "gpt-5"})
				return err
			},
		},
		{
			name: "responses",
			call: func() error {
				_, err := a.CreateResponseRaw(context.Background(), &adapter.RawResponseRequest{Body: json.RawMessage(`{"model":"gpt-5"}`)})
				return err
			},
		},
		{
			name: "responses stream",
			call: func() error {
				_, err := a.CreateResponseStream(context.Background(), &adapter.RawResponseRequest{Body: json.RawMessage(`{"model":"gpt-5","stream":true}`)})
				return err
			},
		},
		{
			name: "anthropic",
			call: func() error {
				_, err := a.CreateAnthropicMessage(context.Background(), &adapter.AnthropicMessageRequest{Body: json.RawMessage(`{"model":"claude-sonnet-4-6"}`)})
				return err
			},
		},
		{
			name: "anthropic stream",
			call: func() error {
				_, err := a.CreateAnthropicMessageStream(context.Background(), &adapter.AnthropicMessageRequest{Body: json.RawMessage(`{"model":"claude-sonnet-4-6","stream":true}`)})
				return err
			},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if err := test.call(); !errors.Is(err, adapter.ErrPlatformChannelUnavailable) {
				t.Fatalf("error = %v, want ErrPlatformChannelUnavailable", err)
			}
		})
	}
}

func TestZGICloudAdapterChatCompletion_ForwardsToConsoleInternal(t *testing.T) {
	t.Helper()

	var (
		gotPath string
		gotSig  string
		gotAuth string
		gotOrg  string
	)

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		gotSig = r.Header.Get("X-Test-Signature")
		gotAuth = r.Header.Get("Authorization")
		gotOrg = r.Header.Get("OpenAI-Organization")

		w.Header().Set("Content-Type", "application/json")
		w.Header().Set(headerSettlementID, "deduction-1")
		w.Header().Set(headerOfficialPoints, "7")
		w.Header().Set(headerRemainingBalance, "93")
		w.Header().Set(headerSettlementStatus, "settled")
		w.Header().Set(headerTotalCostUSD, "0.006601")
		w.Header().Set(headerTotalCostCNY, "0.0475272")
		w.Header().Set(headerCNYPerUSD, "7.2")
		fmt.Fprint(w, `{
			"id":"chatcmpl-zgi-cloud-1",
			"object":"chat.completion",
			"created":1732083164,
			"model":"gpt-4o-mini",
			"choices":[{"index":0,"message":{"role":"assistant","content":"ok"},"finish_reason":"stop"}],
			"usage":{"prompt_tokens":11,"completion_tokens":7,"total_tokens":18}
		}`)
	}))
	defer server.Close()

	a, err := NewZGICloudAdapter(&adapter.AdapterConfig{
		BaseURL:      server.URL + "/v1/internal",
		Organization: "should-not-forward",
		AuthHook: func(req *http.Request) {
			req.Header.Set("X-Test-Signature", "signed")
		},
	})
	if err != nil {
		t.Fatalf("NewZGICloudAdapter() error = %v", err)
	}

	resp, err := a.ChatCompletion(context.Background(), &adapter.ChatRequest{
		Model:    "gpt-4o-mini",
		Messages: []adapter.Message{{Role: "user", Content: "hello"}},
	})
	if err != nil {
		t.Fatalf("ChatCompletion() error = %v", err)
	}

	if gotPath != "/v1/internal/chat/completions" {
		t.Fatalf("path = %q, want %q", gotPath, "/v1/internal/chat/completions")
	}
	if gotSig != "signed" {
		t.Fatalf("X-Test-Signature = %q, want %q", gotSig, "signed")
	}
	if gotAuth != "" {
		t.Fatalf("Authorization = %q, want empty for HMAC-only official transport", gotAuth)
	}
	if gotOrg != "" {
		t.Fatalf("OpenAI-Organization = %q, want empty for console forward transport", gotOrg)
	}
	if resp.Model != "gpt-4o-mini" {
		t.Fatalf("response model = %q, want %q", resp.Model, "gpt-4o-mini")
	}
	if resp.Settlement == nil || resp.Settlement.SettlementID != "deduction-1" || resp.Settlement.OfficialPoints != 7 {
		t.Fatalf("settlement = %+v, want deduction-1/7", resp.Settlement)
	}
	if resp.Settlement.TotalCostUSD != "0.006601" || resp.Settlement.TotalCostCNY != "0.0475272" || resp.Settlement.CNYPerUSD != "7.2" {
		t.Fatalf("settlement cost snapshot = %+v", resp.Settlement)
	}
}

func TestZGICloudAdapterChatCompletionStream_ConsumesSettlementEvent(t *testing.T) {
	t.Helper()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/internal/chat/completions" {
			t.Fatalf("path = %q, want %q", r.URL.Path, "/v1/internal/chat/completions")
		}

		w.Header().Set("Content-Type", "text/event-stream")
		fmt.Fprint(w, "data: {\"id\":\"chatcmpl-1\",\"object\":\"chat.completion.chunk\",\"choices\":[{\"index\":0,\"delta\":{\"content\":\"ok\"}}]}\n\n")
		fmt.Fprint(w, "data: {\"id\":\"chatcmpl-1\",\"object\":\"chat.completion.chunk\",\"choices\":[{\"index\":0,\"delta\":{},\"finish_reason\":\"stop\"}],\"usage\":{\"prompt_tokens\":3,\"completion_tokens\":2,\"total_tokens\":5}}\n\n")
		fmt.Fprint(w, "event: zgi.settlement\n")
		fmt.Fprint(w, "data: {\"settlement_id\":\"deduction-stream\",\"official_points\":9,\"remaining_balance\":91,\"status\":\"settled\",\"total_cost_usd\":\"0.009\",\"total_cost_cny\":\"0.0648\",\"cny_per_usd\":\"7.2\"}\n\n")
		fmt.Fprint(w, "data: [DONE]\n\n")
	}))
	defer server.Close()

	a, err := NewZGICloudAdapter(&adapter.AdapterConfig{
		BaseURL: server.URL + "/v1/internal",
		AuthHook: func(req *http.Request) {
			req.Header.Set("X-Test-Signature", "signed")
		},
	})
	if err != nil {
		t.Fatalf("NewZGICloudAdapter() error = %v", err)
	}

	stream, err := a.ChatCompletionStream(context.Background(), &adapter.ChatRequest{
		Model:    "gpt-4o-mini",
		Messages: []adapter.Message{{Role: "user", Content: "hello"}},
	})
	if err != nil {
		t.Fatalf("ChatCompletionStream() error = %v", err)
	}

	var (
		chunkCount int
		done       adapter.StreamResponse
	)
	for event := range stream {
		if event.Error != nil {
			t.Fatalf("stream error = %v", event.Error)
		}
		if event.Done {
			done = event
			continue
		}
		chunkCount++
	}

	if chunkCount != 2 {
		t.Fatalf("chunk count = %d, want 2", chunkCount)
	}
	if done.Settlement == nil || done.Settlement.SettlementID != "deduction-stream" || done.Settlement.OfficialPoints != 9 {
		t.Fatalf("done settlement = %+v, want deduction-stream/9", done.Settlement)
	}
	if done.Settlement.TotalCostCNY != "0.0648" || done.Settlement.CNYPerUSD != "7.2" {
		t.Fatalf("done settlement cost snapshot = %+v", done.Settlement)
	}
	if done.Usage == nil || done.Usage.TotalTokens != 5 {
		t.Fatalf("done usage = %+v, want total 5", done.Usage)
	}
}

func TestZGICloudAdapterChatCompletionStream_SettlementErrorReturnsStreamError(t *testing.T) {
	t.Helper()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		fmt.Fprint(w, "data: {\"id\":\"chatcmpl-1\",\"object\":\"chat.completion.chunk\",\"choices\":[{\"index\":0,\"delta\":{\"content\":\"ok\"}}]}\n\n")
		fmt.Fprint(w, "data: {\"id\":\"chatcmpl-1\",\"object\":\"chat.completion.chunk\",\"choices\":[{\"index\":0,\"delta\":{},\"finish_reason\":\"stop\"}],\"usage\":{\"prompt_tokens\":3,\"completion_tokens\":2,\"total_tokens\":5}}\n\n")
		fmt.Fprint(w, "event: zgi.settlement_error\n")
		fmt.Fprint(w, "data: {\"code\":\"billing_settlement_failed\",\"message\":\"official settlement failed\",\"status\":\"failed\"}\n\n")
	}))
	defer server.Close()

	a, err := NewZGICloudAdapter(&adapter.AdapterConfig{
		BaseURL: server.URL + "/v1/internal",
		AuthHook: func(req *http.Request) {
			req.Header.Set("X-Test-Signature", "signed")
		},
	})
	if err != nil {
		t.Fatalf("NewZGICloudAdapter() error = %v", err)
	}

	stream, err := a.ChatCompletionStream(context.Background(), &adapter.ChatRequest{
		Model:    "gpt-4o-mini",
		Messages: []adapter.Message{{Role: "user", Content: "hello"}},
	})
	if err != nil {
		t.Fatalf("ChatCompletionStream() error = %v", err)
	}

	var (
		chunkCount int
		done       adapter.StreamResponse
	)
	for event := range stream {
		if event.Done {
			done = event
			continue
		}
		chunkCount++
	}

	if chunkCount != 2 {
		t.Fatalf("chunk count = %d, want 2", chunkCount)
	}
	if done.Error == nil || done.Error.Error() != "console proxy settlement failed: official settlement failed" {
		t.Fatalf("done error = %v, want explicit settlement failure", done.Error)
	}
	if done.Usage == nil || done.Usage.TotalTokens != 5 {
		t.Fatalf("done usage = %+v, want total 5", done.Usage)
	}
}

func TestZGICloudAdapterCreateResponseRaw_ForwardsToConsoleInternalResponses(t *testing.T) {
	t.Helper()

	var (
		gotPath    string
		gotSig     string
		gotAuth    string
		gotPayload map[string]any
	)

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defer r.Body.Close()

		gotPath = r.URL.Path
		gotSig = r.Header.Get("X-Test-Signature")
		gotAuth = r.Header.Get("Authorization")
		if err := json.NewDecoder(r.Body).Decode(&gotPayload); err != nil {
			t.Fatalf("decode request body: %v", err)
		}

		w.Header().Set("Content-Type", "application/json")
		fmt.Fprint(w, `{
			"id":"resp_zgi_cloud_1",
			"object":"response",
			"model":"gpt-4.1-mini",
			"output":[],
			"usage":{"input_tokens":6,"output_tokens":4,"total_tokens":10}
		}`)
	}))
	defer server.Close()

	a, err := NewZGICloudAdapter(&adapter.AdapterConfig{
		BaseURL: server.URL + "/v1/internal",
		AuthHook: func(req *http.Request) {
			req.Header.Set("X-Test-Signature", "signed")
		},
	})
	if err != nil {
		t.Fatalf("NewZGICloudAdapter() error = %v", err)
	}

	resp, err := a.CreateResponseRaw(context.Background(), &adapter.RawResponseRequest{
		Model: "gpt-4.1-mini",
		Body:  json.RawMessage(`{"model":"gpt-4.1-mini","input":"hello"}`),
	})
	if err != nil {
		t.Fatalf("CreateResponseRaw() error = %v", err)
	}

	if gotPath != "/v1/internal/responses" {
		t.Fatalf("path = %q, want %q", gotPath, "/v1/internal/responses")
	}
	if gotSig != "signed" {
		t.Fatalf("X-Test-Signature = %q, want signed", gotSig)
	}
	if gotAuth != "" {
		t.Fatalf("Authorization = %q, want empty for HMAC-only official transport", gotAuth)
	}
	if gotPayload["model"] != "gpt-4.1-mini" || gotPayload["input"] != "hello" {
		t.Fatalf("payload = %#v, want raw responses body", gotPayload)
	}
	if resp.Usage == nil || resp.Usage.PromptTokens != 6 || resp.Usage.CompletionTokens != 4 || resp.Usage.TotalTokens != 10 {
		t.Fatalf("usage = %+v, want prompt=6 completion=4 total=10", resp.Usage)
	}
}

func TestZGICloudAdapterCreateResponseStream_ConsumesSettlementEvent(t *testing.T) {
	t.Helper()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/internal/responses" {
			t.Fatalf("path = %q, want %q", r.URL.Path, "/v1/internal/responses")
		}

		w.Header().Set("Content-Type", "text/event-stream")
		fmt.Fprint(w, "event: response.created\n")
		fmt.Fprint(w, "data: {\"type\":\"response.created\",\"response\":{\"id\":\"resp_1\"}}\n\n")
		fmt.Fprint(w, "event: response.completed\n")
		fmt.Fprint(w, "data: {\"type\":\"response.completed\",\"response\":{\"id\":\"resp_1\",\"usage\":{\"input_tokens\":4,\"output_tokens\":3,\"total_tokens\":7}}}\n\n")
		fmt.Fprint(w, "event: zgi.settlement\n")
		fmt.Fprint(w, "data: {\"settlement_id\":\"deduction-response\",\"official_points\":11,\"remaining_balance\":89,\"status\":\"settled\"}\n\n")
	}))
	defer server.Close()

	a, err := NewZGICloudAdapter(&adapter.AdapterConfig{
		BaseURL: server.URL + "/v1/internal",
		AuthHook: func(req *http.Request) {
			req.Header.Set("X-Test-Signature", "signed")
		},
	})
	if err != nil {
		t.Fatalf("NewZGICloudAdapter() error = %v", err)
	}

	stream, err := a.CreateResponseStream(context.Background(), &adapter.RawResponseRequest{
		Model: "gpt-4.1-mini",
		Body:  json.RawMessage(`{"model":"gpt-4.1-mini","input":"hello","stream":true}`),
	})
	if err != nil {
		t.Fatalf("CreateResponseStream() error = %v", err)
	}

	var (
		events []string
		done   adapter.RawStreamEvent
	)
	for event := range stream {
		if event.Error != nil {
			t.Fatalf("stream error = %v", event.Error)
		}
		if event.Done {
			done = event
			continue
		}
		events = append(events, event.Event)
	}

	if len(events) != 2 || events[0] != "response.created" || events[1] != "response.completed" {
		t.Fatalf("events = %#v, want native response events only", events)
	}
	if done.Settlement == nil || done.Settlement.SettlementID != "deduction-response" || done.Settlement.OfficialPoints != 11 {
		t.Fatalf("done settlement = %+v, want deduction-response/11", done.Settlement)
	}
	if done.Usage == nil || done.Usage.TotalTokens != 7 {
		t.Fatalf("done usage = %+v, want total 7", done.Usage)
	}
}

func TestZGICloudAdapterCreateResponseStream_SettlementErrorReturnsStreamError(t *testing.T) {
	t.Helper()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		fmt.Fprint(w, "event: response.completed\n")
		fmt.Fprint(w, "data: {\"type\":\"response.completed\",\"response\":{\"id\":\"resp_1\",\"usage\":{\"input_tokens\":4,\"output_tokens\":3,\"total_tokens\":7}}}\n\n")
		fmt.Fprint(w, "event: zgi.settlement_error\n")
		fmt.Fprint(w, "data: {\"code\":\"billing_settlement_failed\",\"message\":\"official settlement failed\",\"status\":\"failed\"}\n\n")
	}))
	defer server.Close()

	a, err := NewZGICloudAdapter(&adapter.AdapterConfig{
		BaseURL: server.URL + "/v1/internal",
		AuthHook: func(req *http.Request) {
			req.Header.Set("X-Test-Signature", "signed")
		},
	})
	if err != nil {
		t.Fatalf("NewZGICloudAdapter() error = %v", err)
	}

	stream, err := a.CreateResponseStream(context.Background(), &adapter.RawResponseRequest{
		Model: "gpt-4.1-mini",
		Body:  json.RawMessage(`{"model":"gpt-4.1-mini","input":"hello","stream":true}`),
	})
	if err != nil {
		t.Fatalf("CreateResponseStream() error = %v", err)
	}

	var (
		events []string
		done   adapter.RawStreamEvent
	)
	for event := range stream {
		if event.Done {
			done = event
			continue
		}
		events = append(events, event.Event)
	}

	if len(events) != 1 || events[0] != "response.completed" {
		t.Fatalf("events = %#v, want response.completed only", events)
	}
	if done.Error == nil || done.Error.Error() != "console proxy settlement failed: official settlement failed" {
		t.Fatalf("done error = %v, want explicit settlement failure", done.Error)
	}
	if done.Usage == nil || done.Usage.TotalTokens != 7 {
		t.Fatalf("done usage = %+v, want total 7", done.Usage)
	}
}

func TestZGICloudAdapterCreateAnthropicMessage_ForwardsToConsoleInternalAnthropicMessages(t *testing.T) {
	t.Helper()

	var (
		gotPath    string
		gotSig     string
		gotVersion string
		gotPayload map[string]any
	)

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defer r.Body.Close()

		gotPath = r.URL.Path
		gotSig = r.Header.Get("X-Test-Signature")
		gotVersion = r.Header.Get("anthropic-version")
		if err := json.NewDecoder(r.Body).Decode(&gotPayload); err != nil {
			t.Fatalf("decode request body: %v", err)
		}

		w.Header().Set("Content-Type", "application/json")
		fmt.Fprint(w, `{
			"id":"msg_zgi_cloud_1",
			"type":"message",
			"role":"assistant",
			"model":"claude-sonnet-4-0",
			"content":[{"type":"text","text":"ok"}],
			"usage":{"input_tokens":30,"cache_read_input_tokens":20,"cache_creation_input_tokens":10,"output_tokens":5}
		}`)
	}))
	defer server.Close()

	a, err := NewZGICloudAdapter(&adapter.AdapterConfig{
		BaseURL: server.URL + "/v1/internal",
		AuthHook: func(req *http.Request) {
			req.Header.Set("X-Test-Signature", "signed")
		},
	})
	if err != nil {
		t.Fatalf("NewZGICloudAdapter() error = %v", err)
	}

	resp, err := a.CreateAnthropicMessage(context.Background(), &adapter.AnthropicMessageRequest{
		Model: "claude-sonnet-4-0",
		Body:  json.RawMessage(`{"model":"claude-sonnet-4-0","max_tokens":64,"messages":[{"role":"user","content":"hello"}]}`),
		Headers: map[string]string{
			"anthropic-version": "2023-06-01",
		},
	})
	if err != nil {
		t.Fatalf("CreateAnthropicMessage() error = %v", err)
	}

	if gotPath != "/v1/internal/anthropic/v1/messages" {
		t.Fatalf("path = %q, want %q", gotPath, "/v1/internal/anthropic/v1/messages")
	}
	if gotSig != "signed" {
		t.Fatalf("X-Test-Signature = %q, want signed", gotSig)
	}
	if gotVersion != "2023-06-01" {
		t.Fatalf("anthropic-version = %q, want 2023-06-01", gotVersion)
	}
	if gotPayload["model"] != "claude-sonnet-4-0" || gotPayload["max_tokens"] != float64(64) {
		t.Fatalf("payload = %#v, want raw messages body", gotPayload)
	}
	if resp.Usage == nil || resp.Usage.PromptTokens != 60 || resp.Usage.UncachedInputTokens != 30 || resp.Usage.CacheReadTokens != 20 || resp.Usage.CacheWriteTokens != 10 || resp.Usage.CompletionTokens != 5 || resp.Usage.TotalTokens != 65 {
		t.Fatalf("usage = %+v, want uncached=30 cache_read=20 cache_write=10 completion=5 total=65", resp.Usage)
	}
}

func TestZGICloudAdapterCreateAnthropicMessageStream_PreservesNativeEvents(t *testing.T) {
	t.Helper()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/internal/anthropic/v1/messages" {
			t.Fatalf("path = %q, want %q", r.URL.Path, "/v1/internal/anthropic/v1/messages")
		}

		flusher, ok := w.(http.Flusher)
		if !ok {
			t.Fatal("response writer does not support flushing")
		}
		w.Header().Set("Content-Type", "text/event-stream")
		fmt.Fprint(w, "event: message_start\n")
		fmt.Fprint(w, "data: {\"type\":\"message_start\",\"message\":{\"id\":\"msg_1\",\"usage\":{\"input_tokens\":28,\"cache_read_input_tokens\":18,\"cache_creation_input_tokens\":10,\"output_tokens\":0}}}\n\n")
		fmt.Fprint(w, "event: content_block_delta\n")
		fmt.Fprint(w, "data: {\"type\":\"content_block_delta\",\"index\":0,\"delta\":{\"type\":\"text_delta\",\"text\":\"ok\"}}\n\n")
		fmt.Fprint(w, "event: message_delta\n")
		fmt.Fprint(w, "data: {\"type\":\"message_delta\",\"usage\":{\"output_tokens\":52}}\n\n")
		fmt.Fprint(w, "event: message_stop\n")
		fmt.Fprint(w, "data: {\"type\":\"message_stop\"}\n\n")
		flusher.Flush()
	}))
	defer server.Close()

	a, err := NewZGICloudAdapter(&adapter.AdapterConfig{
		BaseURL: server.URL + "/v1/internal",
		AuthHook: func(req *http.Request) {
			req.Header.Set("X-Test-Signature", "signed")
		},
	})
	if err != nil {
		t.Fatalf("NewZGICloudAdapter() error = %v", err)
	}

	stream, err := a.CreateAnthropicMessageStream(context.Background(), &adapter.AnthropicMessageRequest{
		Model: "claude-sonnet-4-0",
		Body:  json.RawMessage(`{"model":"claude-sonnet-4-0","stream":true,"max_tokens":64,"messages":[{"role":"user","content":"hello"}]}`),
	})
	if err != nil {
		t.Fatalf("CreateAnthropicMessageStream() error = %v", err)
	}

	var (
		events []string
		usage  *adapter.Usage
	)
	for event := range stream {
		if event.Error != nil {
			t.Fatalf("stream event error = %v", event.Error)
		}
		if event.Done {
			usage = event.Usage
			continue
		}
		events = append(events, event.Event)
		if event.Usage != nil {
			usage = event.Usage
		}
	}

	if len(events) != 4 || events[0] != "message_start" || events[1] != "content_block_delta" || events[2] != "message_delta" || events[3] != "message_stop" {
		t.Fatalf("events = %#v, want native Anthropic events", events)
	}
	if usage == nil || usage.PromptTokens != 56 || usage.UncachedInputTokens != 28 || usage.CacheReadTokens != 18 || usage.CacheWriteTokens != 10 || usage.CompletionTokens != 52 || usage.TotalTokens != 108 {
		t.Fatalf("usage = %+v, want uncached=28 cache_read=18 cache_write=10 completion=52 total=108", usage)
	}
}

func TestZGICloudAdapterCreateImage_ForwardsToConsoleInternal(t *testing.T) {
	t.Helper()

	var gotPath string

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path

		w.Header().Set("Content-Type", "application/json")
		fmt.Fprint(w, `{
			"created":1732083164,
			"data":[{"url":"https://cdn.example.com/generated.png"}]
		}`)
	}))
	defer server.Close()

	a, err := NewZGICloudAdapter(&adapter.AdapterConfig{
		BaseURL: server.URL + "/v1/internal",
		AuthHook: func(req *http.Request) {
			req.Header.Set("X-Test-Signature", "signed")
		},
	})
	if err != nil {
		t.Fatalf("NewZGICloudAdapter() error = %v", err)
	}

	resp, err := a.CreateImage(context.Background(), &adapter.ImageRequest{
		Model:  "qwen-image-2.0",
		Prompt: "a flower",
	})
	if err != nil {
		t.Fatalf("CreateImage() error = %v", err)
	}

	if gotPath != "/v1/internal/images/generations" {
		t.Fatalf("path = %q, want %q", gotPath, "/v1/internal/images/generations")
	}
	if len(resp.Data) != 1 || resp.Data[0].URL != "https://cdn.example.com/generated.png" {
		t.Fatalf("response data = %#v, want generated image url", resp.Data)
	}
}

func TestZGICloudAdapterCreateEmbeddings_ForwardsToConsoleInternal(t *testing.T) {
	t.Helper()

	var (
		gotPath    string
		gotSig     string
		gotPayload map[string]any
	)

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defer r.Body.Close()

		gotPath = r.URL.Path
		gotSig = r.Header.Get("X-Test-Signature")
		if err := json.NewDecoder(r.Body).Decode(&gotPayload); err != nil {
			t.Fatalf("decode request body: %v", err)
		}

		w.Header().Set("Content-Type", "application/json")
		fmt.Fprint(w, `{
			"object":"list",
			"model":"text-embedding-3-small",
			"data":[{"object":"embedding","index":0,"embedding":[0.1,0.2,0.3]}],
			"usage":{"prompt_tokens":7,"total_tokens":7}
		}`)
	}))
	defer server.Close()

	a, err := NewZGICloudAdapter(&adapter.AdapterConfig{
		BaseURL: server.URL + "/v1/internal",
		AuthHook: func(req *http.Request) {
			req.Header.Set("X-Test-Signature", "signed")
		},
	})
	if err != nil {
		t.Fatalf("NewZGICloudAdapter() error = %v", err)
	}

	resp, err := a.CreateEmbeddings(context.Background(), &adapter.EmbeddingsRequest{
		Model:      "text-embedding-3-small",
		Input:      "hello",
		Dimensions: 1024,
	})
	if err != nil {
		t.Fatalf("CreateEmbeddings() error = %v", err)
	}

	if gotPath != "/v1/internal/embeddings" {
		t.Fatalf("path = %q, want %q", gotPath, "/v1/internal/embeddings")
	}
	if gotSig != "signed" {
		t.Fatalf("X-Test-Signature = %q, want %q", gotSig, "signed")
	}
	if got := gotPayload["dimensions"]; got != float64(1024) {
		t.Fatalf("payload.dimensions = %#v, want %d", got, 1024)
	}
	if resp.Model != "text-embedding-3-small" {
		t.Fatalf("response model = %q, want %q", resp.Model, "text-embedding-3-small")
	}
}

func TestZGICloudAdapterRerank_ForwardsToConsoleInternal(t *testing.T) {
	t.Helper()

	var (
		gotPath    string
		gotSig     string
		gotPayload map[string]any
	)

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defer r.Body.Close()

		gotPath = r.URL.Path
		gotSig = r.Header.Get("X-Test-Signature")
		if err := json.NewDecoder(r.Body).Decode(&gotPayload); err != nil {
			t.Fatalf("decode request body: %v", err)
		}

		w.Header().Set("Content-Type", "application/json")
		fmt.Fprint(w, `{
			"id":"rerank-zgi-cloud-1",
			"results":[
				{"index":1,"relevance_score":0.97,"document":{"text":"second"}},
				{"index":0,"relevance_score":0.66,"document":{"text":"first"}}
			]
		}`)
	}))
	defer server.Close()

	a, err := NewZGICloudAdapter(&adapter.AdapterConfig{
		BaseURL: server.URL + "/v1/internal",
		AuthHook: func(req *http.Request) {
			req.Header.Set("X-Test-Signature", "signed")
		},
	})
	if err != nil {
		t.Fatalf("NewZGICloudAdapter() error = %v", err)
	}

	returnDocuments := true
	resp, err := a.Rerank(context.Background(), &adapter.RerankRequest{
		Model:           "rerank-v1",
		Query:           "hello",
		Documents:       []string{"first", "second"},
		TopN:            intPtrZGICloudTest(1),
		ReturnDocuments: &returnDocuments,
	})
	if err != nil {
		t.Fatalf("Rerank() error = %v", err)
	}

	if gotPath != "/v1/internal/rerank" {
		t.Fatalf("path = %q, want %q", gotPath, "/v1/internal/rerank")
	}
	if gotSig != "signed" {
		t.Fatalf("X-Test-Signature = %q, want %q", gotSig, "signed")
	}
	if got := gotPayload["top_n"]; got != float64(1) {
		t.Fatalf("payload.top_n = %#v, want %d", got, 1)
	}
	if got := gotPayload["return_documents"]; got != true {
		t.Fatalf("payload.return_documents = %#v, want true", got)
	}
	if resp.ID != "rerank-zgi-cloud-1" {
		t.Fatalf("response ID = %q, want %q", resp.ID, "rerank-zgi-cloud-1")
	}
	if len(resp.Results) != 2 {
		t.Fatalf("results len = %d, want 2", len(resp.Results))
	}
}

func TestZGICloudAdapterRejectsStillUnsupportedCapabilities(t *testing.T) {
	t.Helper()

	a, err := NewZGICloudAdapter(&adapter.AdapterConfig{
		BaseURL: "http://console.internal/v1/internal",
		AuthHook: func(req *http.Request) {
			req.Header.Set("X-Test-Signature", "signed")
		},
	})
	if err != nil {
		t.Fatalf("NewZGICloudAdapter() error = %v", err)
	}

	_, err = a.CreateResponse(context.Background(), &adapter.CreateResponseRequest{
		Model: "gpt-4.1-mini",
		Input: "hello",
	})
	if !errors.Is(err, adapter.ErrCapabilityUnsupported) {
		t.Fatalf("CreateResponse() error = %v, want %v", err, adapter.ErrCapabilityUnsupported)
	}
}

func intPtrZGICloudTest(v int) *int {
	return &v
}
