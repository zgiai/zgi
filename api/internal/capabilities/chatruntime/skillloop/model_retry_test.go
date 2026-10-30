package skillloop

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"syscall"
	"testing"
	"time"

	llmclient "github.com/zgiai/zgi/api/internal/modules/llm/client"
	adapter "github.com/zgiai/zgi/api/internal/modules/llm/protocol/adapters"
)

func TestTransientModelErrorRetriesOnlyFailuresAnotherAttemptCanFix(t *testing.T) {
	tests := []struct {
		name string
		err  error
		want bool
	}{
		{"rate limited", adapter.NewAdapterError("rate_limit", "slow down", 429, adapter.ErrRateLimited), true},
		{"server error", adapter.NewAdapterError("server_error", "overloaded", 503, adapter.ErrUpstreamError), true},
		{"wrapped by the gateway", fmt.Errorf("all providers failed: %w", adapter.NewAdapterError("x", "bad gateway", 502, adapter.ErrUpstreamError)), true},
		{"proxy error", adapter.ErrProxyError, true},
		{"stream closed", adapter.ErrStreamClosed, true},
		{"client error under the upstream sentinel", adapter.NewAdapterError("x", "unprocessable", 422, adapter.ErrUpstreamError), false},
		{"invalid request", adapter.NewAdapterError("x", "bad", 400, adapter.ErrInvalidRequest), false},
		{"auth", adapter.NewAdapterError("x", "no", 401, adapter.ErrAuthFailed), false},
		{"quota", adapter.NewAdapterError("insufficient_quota", "no", 429, adapter.ErrQuotaExhausted), false},
		{"billing settled after the call", adapter.ErrBillingSettleFailed, false},
		{"provider timeout", fmt.Errorf("%w: %w", adapter.ErrTimeout, context.DeadlineExceeded), false},
		{"idle timeout", ErrModelIdleTimeout, false},
		{"answer already shown", wrapStreamedFinalAnswerError(adapter.ErrStreamClosed, "partial answer"), false},
		{"unknown", errors.New("boom"), false},
	}
	for _, tt := range tests {
		if got := transientModelError(tt.err); got != tt.want {
			t.Errorf("%s: transientModelError() = %v, want %v", tt.name, got, tt.want)
		}
	}
}

func retryTestRunner(client *runnerTestLLMClient) *Runner {
	return &Runner{
		LLMClient:        client,
		AppContext:       &llmclient.AppContext{},
		ModelRetryDelays: []time.Duration{time.Millisecond, time.Millisecond},
	}
}

func retryTestRequest() (*PreparedChat, *adapter.ChatRequest) {
	request := &adapter.ChatRequest{Messages: []adapter.Message{{Role: "user", Content: "hi"}}}
	return NewPreparedChat("conv-retry", "msg-retry", "", "auto", request), request
}

func TestModelToolRoundRetriesATransientFailure(t *testing.T) {
	client := &runnerTestLLMClient{
		appChatErrors: []error{adapter.NewAdapterError("server_error", "overloaded", 503, adapter.ErrUpstreamError)},
		appChatResponses: []*adapter.ChatResponse{nil, {Choices: []adapter.Choice{{
			Message: adapter.Message{Role: "assistant", Content: "hello"}, FinishReason: "stop",
		}}}},
	}
	prepared, request := retryTestRequest()
	result, err := retryTestRunner(client).runNativeAgentRound(context.Background(), prepared, request, 0, nil, false, false, false)
	if err != nil {
		t.Fatalf("runNativeAgentRound() error = %v", err)
	}
	if client.appChatCalls != 2 || messageContent(result.message.Content) != "hello" {
		t.Fatalf("calls = %d, answer = %q; want the second attempt's answer", client.appChatCalls, messageContent(result.message.Content))
	}
}

func TestModelToolRoundStopsAfterTheLastRetry(t *testing.T) {
	overloaded := adapter.NewAdapterError("server_error", "overloaded", 503, adapter.ErrUpstreamError)
	client := &runnerTestLLMClient{appChatErrors: []error{overloaded, overloaded, overloaded, overloaded}}
	prepared, request := retryTestRequest()
	_, err := retryTestRunner(client).runNativeAgentRound(context.Background(), prepared, request, 0, nil, false, false, false)
	if !errors.Is(err, adapter.ErrUpstreamError) || client.appChatCalls != 3 {
		t.Fatalf("error = %v after %d calls, want the upstream error after three attempts", err, client.appChatCalls)
	}
}

func TestModelToolRoundDoesNotRetryARejectedRequest(t *testing.T) {
	client := &runnerTestLLMClient{appChatErrors: []error{adapter.NewAdapterError("x", "bad", 400, adapter.ErrInvalidRequest)}}
	prepared, request := retryTestRequest()
	_, err := retryTestRunner(client).runNativeAgentRound(context.Background(), prepared, request, 0, nil, false, false, false)
	if !errors.Is(err, adapter.ErrInvalidRequest) || client.appChatCalls != 1 {
		t.Fatalf("error = %v after %d calls, want one call", err, client.appChatCalls)
	}
}

type timeoutNetError struct{}

func (timeoutNetError) Error() string   { return "i/o timeout" }
func (timeoutNetError) Timeout() bool   { return true }
func (timeoutNetError) Temporary() bool { return true }

func TestConnectionLostMarksABrokenConnectionAsAClosedStream(t *testing.T) {
	tlsRecord := &net.OpError{Op: "local error", Net: "tcp", Err: errors.New("tls: bad record MAC")}
	tests := []struct {
		name string
		err  error
		want bool
	}{
		{"TLS record failure", tlsRecord, true},
		{"closed before the response", fmt.Errorf("failed to execute request: Post %q: %w", "https://api.example.com", io.EOF), true},
		{"cut off mid-body", fmt.Errorf("read: %w", io.ErrUnexpectedEOF), true},
		{"reset by the peer", fmt.Errorf("all providers failed: %w", &net.OpError{Op: "read", Net: "tcp", Err: syscall.ECONNRESET}), true},
		{"network timeout", &net.OpError{Op: "read", Net: "tcp", Err: timeoutNetError{}}, false},
		{"cancelled", context.Canceled, false},
		{"rejected request", adapter.NewAdapterError("x", "bad", 400, adapter.ErrInvalidRequest), false},
		{"unknown", errors.New("boom"), false},
	}
	for _, tt := range tests {
		got := connectionLost(tt.err)
		if marked := errors.Is(got, adapter.ErrStreamClosed); marked != tt.want {
			t.Errorf("%s: marked as a closed stream = %v, want %v", tt.name, marked, tt.want)
		}
		if !errors.Is(got, tt.err) {
			t.Errorf("%s: connectionLost() = %v, want it to keep the original error", tt.name, got)
		}
		if transientModelError(got) != tt.want {
			t.Errorf("%s: transientModelError() = %v, want %v", tt.name, !tt.want, tt.want)
		}
	}
}

func TestModelToolRoundRetriesAStreamWhoseConnectionBroke(t *testing.T) {
	tlsRecord := &net.OpError{Op: "local error", Net: "tcp", Err: errors.New("tls: bad record MAC")}
	client := &runnerTestLLMClient{appChatStreams: [][]adapter.StreamResponse{
		{
			{Choices: []adapter.StreamChoice{{Delta: adapter.Message{ReasoningContent: "thinking"}}}},
			{Error: tlsRecord, Done: true},
		},
		{
			{Choices: []adapter.StreamChoice{{Delta: adapter.Message{Content: "hello"}}}},
			{Choices: []adapter.StreamChoice{{FinishReason: "stop"}}, Done: true},
		},
	}}
	runner := retryTestRunner(client)
	runner.ModelIdleTimeout = time.Second
	prepared := NewPreparedChat("conv-retry", "msg-retry", "deepseek", "auto", &adapter.ChatRequest{
		Model:    "deepseek-chat",
		Messages: []adapter.Message{{Role: "user", Content: "hi"}},
	})
	result, err := runner.runNativeAgentRound(context.Background(), prepared, prepared.LLMRequest, 0, nil, false, false, false)
	if err != nil {
		t.Fatalf("runNativeAgentRound() error = %v", err)
	}
	if client.appChatStreamCalls != 2 || messageContent(result.message.Content) != "hello" {
		t.Fatalf("stream calls = %d, answer = %q; want the second stream's answer", client.appChatStreamCalls, messageContent(result.message.Content))
	}
}
