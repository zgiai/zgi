package llm

import "context"

// LLMInvokeRequest bundles parameters required for an LLM invocation.
type LLMInvokeRequest struct {
	ProviderSlug     string
	ModelSlug        string
	Messages         []PromptMessage
	Parameters       map[string]any
	Stop             []string
	UserID           string
	StructuredOutput map[string]any
	SessionID        string
	ConversationID   string
	WorkflowID       string
	WorkflowRunID    string
	NodeID           string
	NodeType         string
}

// LLMInvoker defines the streaming invocation contract for LLM calls.
type LLMInvoker interface {
	InvokeStream(ctx context.Context, accountID, appID, appType string, req *LLMInvokeRequest) (<-chan *ResultChunk, <-chan error, error)
}
