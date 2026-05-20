package imagegen

import (
	"context"
	"errors"

	llmadapter "github.com/zgiai/ginext/internal/modules/llm/protocol/adapters"
)

var ErrInvokerNotConfigured = errors.New("image invoker not configured")

const workflowAppType = "workflow"

type ImageInvoker interface {
	Invoke(ctx context.Context, accountID, appID, appType string, req *InvokeRequest) (*InvokeResult, error)
}

type InvokeRequest struct {
	ModelSlug string // Full model identifier passed through unchanged.
	Prompt    string
	N         int
	Size      string
	Quality   string
	Style     string
	UserID    string
}

type InvokeResult struct {
	Images []llmadapter.ImageItem
}
