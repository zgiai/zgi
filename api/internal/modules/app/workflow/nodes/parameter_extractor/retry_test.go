package parameterextractor

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/zgiai/zgi/api/internal/modules/app/workflow/nodes/base"
	"github.com/zgiai/zgi/api/internal/modules/app/workflow/shared"
)

// mockLLMInvoker is a mock implementation of LLMInvoker for testing
type mockLLMInvoker struct {
	failCount    int // Number of times to fail before succeeding
	currentCount int // Current attempt count
	result       *InvokeResult
	err          error
	lastRequest  *InvokeRequest
}

func (m *mockLLMInvoker) Invoke(ctx context.Context, accountID, appID, appType string, req *InvokeRequest) (*InvokeResult, error) {
	m.currentCount++
	m.lastRequest = req

	// Fail for the first failCount attempts
	if m.currentCount <= m.failCount {
		return nil, m.err
	}

	// Succeed after failCount attempts
	return m.result, nil
}

func (m *mockLLMInvoker) InvokeStream(ctx context.Context, accountID, appID, appType string, req *InvokeRequest) (<-chan *ResultChunk, <-chan error, error) {
	// Not used in this test
	return nil, nil, errors.New("not implemented")
}

func TestInvokeLLMWithRetry_Success(t *testing.T) {
	// Create a node with retry config
	node := &Node{
		nodeData: NodeData{
			NodeData: base.NodeData{
				RetryConfig: shared.RetryConfig{
					MaxTimes: 3,
					Interval: 100,
				},
			},
		},
		llmInvoker: &mockLLMInvoker{
			failCount: 0, // Succeed immediately
			result: &InvokeResult{
				Text:   "test response",
				Finish: "stop",
				Usage: &UsageInfo{
					PromptTokens:     10,
					CompletionTokens: 20,
					TotalTokens:      30,
					TotalPrice:       "0.001",
					Currency:         "USD",
				},
			},
		},
	}

	ctx := context.Background()
	req := &InvokeRequest{
		ModelSlug: "gpt-4",
		Messages:  []PromptMessage{{Role: "user", Content: "test"}},
	}

	result, attempts, err := node.invokeLLMWithRetry(ctx, "acc-1", "app-1", AppType, req)

	if err != nil {
		t.Fatalf("Expected no error, got: %v", err)
	}
	if attempts != 1 {
		t.Errorf("Expected 1 attempt, got: %d", attempts)
	}
	if result == nil {
		t.Fatal("Expected result, got nil")
	}
	if result.Text != "test response" {
		t.Errorf("Expected 'test response', got: %s", result.Text)
	}
}

func TestInvokeLLMWithRetry_SuccessAfterRetries(t *testing.T) {
	// Create a node with retry config
	node := &Node{
		NodeStruct: base.NodeStruct{
			NodeID: "test-node",
		},
		nodeData: NodeData{
			NodeData: base.NodeData{
				RetryConfig: shared.RetryConfig{
					MaxTimes: 3,
					Interval: 100,
				},
			},
		},
		llmInvoker: &mockLLMInvoker{
			failCount: 2, // Fail twice, then succeed
			err:       errors.New("temporary error"),
			result: &InvokeResult{
				Text:   "test response",
				Finish: "stop",
				Usage: &UsageInfo{
					PromptTokens:     10,
					CompletionTokens: 20,
					TotalTokens:      30,
					TotalPrice:       "0.001",
					Currency:         "USD",
				},
			},
		},
	}

	ctx := context.Background()
	req := &InvokeRequest{
		ModelSlug: "gpt-4",
		Messages:  []PromptMessage{{Role: "user", Content: "test"}},
	}

	start := time.Now()
	result, attempts, err := node.invokeLLMWithRetry(ctx, "acc-1", "app-1", AppType, req)
	duration := time.Since(start)

	if err != nil {
		t.Fatalf("Expected no error, got: %v", err)
	}
	if attempts != 3 {
		t.Errorf("Expected 3 attempts, got: %d", attempts)
	}
	if result == nil {
		t.Fatal("Expected result, got nil")
	}

	// Verify exponential backoff was applied
	// First retry: 150ms, Second retry: 300ms = 450ms total minimum
	minDuration := 450 * time.Millisecond
	if duration < minDuration {
		t.Errorf("Expected at least %v duration for retries, got: %v", minDuration, duration)
	}
}

func TestInvokeLLMWithRetry_AllAttemptsFail(t *testing.T) {
	// Create a node with retry config
	node := &Node{
		NodeStruct: base.NodeStruct{
			NodeID: "test-node",
		},
		nodeData: NodeData{
			NodeData: base.NodeData{
				RetryConfig: shared.RetryConfig{
					MaxTimes: 2,
					Interval: 100,
				},
			},
		},
		llmInvoker: &mockLLMInvoker{
			failCount: 10, // Fail more times than max retries
			err:       errors.New("persistent error"),
		},
	}

	ctx := context.Background()
	req := &InvokeRequest{
		ModelSlug: "gpt-4",
		Messages:  []PromptMessage{{Role: "user", Content: "test"}},
	}

	result, attempts, err := node.invokeLLMWithRetry(ctx, "acc-1", "app-1", AppType, req)

	if err == nil {
		t.Fatal("Expected error, got nil")
	}
	if err.Error() != "persistent error" {
		t.Errorf("Expected 'persistent error', got: %v", err)
	}
	if attempts != 3 { // 1 initial + 2 retries
		t.Errorf("Expected 3 attempts, got: %d", attempts)
	}
	if result != nil {
		t.Errorf("Expected nil result, got: %v", result)
	}
}

func TestInvokeLLMWithRetry_ContextCancellation(t *testing.T) {
	// Create a node with retry config
	node := &Node{
		NodeStruct: base.NodeStruct{
			NodeID: "test-node",
		},
		nodeData: NodeData{
			NodeData: base.NodeData{
				RetryConfig: shared.RetryConfig{
					MaxTimes: 5,
					Interval: 100,
				},
			},
		},
		llmInvoker: &mockLLMInvoker{
			failCount: 10, // Fail many times
			err:       errors.New("temporary error"),
		},
	}

	// Create a context that will be cancelled
	ctx, cancel := context.WithCancel(context.Background())

	// Cancel after a short delay
	go func() {
		time.Sleep(200 * time.Millisecond)
		cancel()
	}()

	req := &InvokeRequest{
		ModelSlug: "gpt-4",
		Messages:  []PromptMessage{{Role: "user", Content: "test"}},
	}

	result, attempts, err := node.invokeLLMWithRetry(ctx, "acc-1", "app-1", AppType, req)

	if err == nil {
		t.Fatal("Expected context cancellation error, got nil")
	}
	if err != context.Canceled {
		t.Errorf("Expected context.Canceled, got: %v", err)
	}
	if result != nil {
		t.Errorf("Expected nil result, got: %v", result)
	}
	// Should have attempted at least once but not all retries
	if attempts < 1 || attempts > 6 {
		t.Errorf("Expected 1-6 attempts due to cancellation, got: %d", attempts)
	}
}

func TestInvokeLLMWithRetry_NoRetries(t *testing.T) {
	// Create a node with no retries configured
	node := &Node{
		NodeStruct: base.NodeStruct{
			NodeID: "test-node",
		},
		nodeData: NodeData{
			NodeData: base.NodeData{
				RetryConfig: shared.RetryConfig{
					MaxTimes: 0, // No retries
					Interval: 100,
				},
			},
		},
		llmInvoker: &mockLLMInvoker{
			failCount: 1, // Fail once
			err:       errors.New("error"),
		},
	}

	ctx := context.Background()
	req := &InvokeRequest{
		ModelSlug: "gpt-4",
		Messages:  []PromptMessage{{Role: "user", Content: "test"}},
	}

	result, attempts, err := node.invokeLLMWithRetry(ctx, "acc-1", "app-1", AppType, req)

	if err == nil {
		t.Fatal("Expected error, got nil")
	}
	if attempts != 1 {
		t.Errorf("Expected 1 attempt (no retries), got: %d", attempts)
	}
	if result != nil {
		t.Errorf("Expected nil result, got: %v", result)
	}
}
