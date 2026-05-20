package sqlgenerator

import (
	"context"
	"fmt"
)

// PromptMessage is a minimized role/content structure.
type PromptMessage struct {
	Role    string
	Content string
}

// InvokeRequest captures the fields required for non-streaming LLM calls.
type InvokeRequest struct {
	ModelSlug  string
	Messages   []PromptMessage
	Parameters map[string]any
	Stop       []string
	UserID     string
}

// InvokeResult returns collated SQL generation results.
type InvokeResult struct {
	Text       string
	Finish     string
	RawChoices any // for debugging when needed
}

// LLMInvoker isolates ChatCompletion calls.
type LLMInvoker interface {
	Invoke(ctx context.Context, accountID, appID, appType string, req *InvokeRequest) (*InvokeResult, error)
}

var ErrInvokerNotConfigured = fmt.Errorf("llm invoker not configured")
