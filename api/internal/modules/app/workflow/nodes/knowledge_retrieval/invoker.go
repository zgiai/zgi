package knowledgeretrieval

import (
	"context"
	"fmt"

	llmadapter "github.com/zgiai/ginext/internal/modules/llm/protocol/adapters"
)

// PromptMessage is a minimal role/content pair used for LLM requests.
type PromptMessage struct {
	Role    string
	Content string
}

// PromptMessageTool is a minimal function tool description used for LLM requests.
type PromptMessageTool struct {
	Type     string
	Function any
}

// InvokeRequest captures the inputs required for a non-stream LLM call.
type InvokeRequest struct {
	ModelSlug  string
	Messages   []PromptMessage
	Parameters map[string]any
	Stop       []string
	UserID     string
	Tools      []llmadapter.Tool
	ToolChoice any
}

// InvokeResult represents a trimmed LLM response for routing decisions.
type InvokeResult struct {
	Text       string
	Finish     string
	RawChoices any
}

type llmInvoker interface {
	Invoke(ctx context.Context, accountID, appID, appType string, req *InvokeRequest) (*InvokeResult, error)
}

var ErrInvokerNotConfigured = fmt.Errorf("llm invoker not configured")
