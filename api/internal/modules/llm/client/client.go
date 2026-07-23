// Package client provides a high-level LLM client for internal business modules.
// It abstracts away API key management and gateway complexity, providing a simple
// interface for Workflow, Dataset, Agent, and other modules to call LLM services.
package client

import (
	"context"
	"strings"

	"github.com/zgiai/zgi/api/internal/modules/llm/protocol/adapters"
)

const (
	BillingSubjectTypeAPIKey       = "key"
	BillingSubjectTypeWorkspace    = "workspace"
	BillingSubjectTypeOrganization = "organization"
)

// LLMClient provides LLM capabilities for internal business modules
// (Workflow, Dataset, Agent, etc.)
// It abstracts away API key management and gateway complexity.
type LLMClient interface {
	// Chat performs a non-streaming chat completion request
	Chat(ctx context.Context, organizationID string, req *adapter.ChatRequest) (*adapter.ChatResponse, error)

	// ChatStream performs a streaming chat completion request
	// Returns a channel of stream responses
	ChatStream(ctx context.Context, organizationID string, req *adapter.ChatRequest) (<-chan adapter.StreamResponse, error)

	// CreateResponse performs a create response request (OpenAI Responses API)
	CreateResponse(ctx context.Context, organizationID string, req *adapter.CreateResponseRequest) (*adapter.CreateResponseResponse, error)

	// Embed creates text embeddings
	Embed(ctx context.Context, organizationID string, req *adapter.EmbeddingsRequest) (*adapter.EmbeddingsResponse, error)

	// CreateImage performs an image generation request
	CreateImage(ctx context.Context, organizationID string, req *adapter.ImageRequest) (*adapter.ImageResponse, error)

	// Rerank performs document reranking
	Rerank(ctx context.Context, organizationID string, req *adapter.RerankRequest) (*adapter.RerankResponse, error)

	// AppChat performs a chat completion request with app context for usage tracking
	// accountID: the user who triggered the call
	// appID: the agent, dataset, or workflow ID
	// appType: "agent", "dataset", or "workflow"
	AppChat(ctx context.Context, appCtx *AppContext, req *adapter.ChatRequest) (*adapter.ChatResponse, error)

	// AppChatStream performs a streaming chat completion request with app context
	AppChatStream(ctx context.Context, appCtx *AppContext, req *adapter.ChatRequest) (<-chan adapter.StreamResponse, error)

	// AppCreateResponse performs a create response request with app context
	AppCreateResponse(ctx context.Context, appCtx *AppContext, req *adapter.CreateResponseRequest) (*adapter.CreateResponseResponse, error)

	// AppEmbed creates embeddings with app context for usage tracking
	AppEmbed(ctx context.Context, appCtx *AppContext, req *adapter.EmbeddingsRequest) (*adapter.EmbeddingsResponse, error)

	// AppCreateImage performs an image generation request with app context
	AppCreateImage(ctx context.Context, appCtx *AppContext, req *adapter.ImageRequest) (*adapter.ImageResponse, error)

	// AppRerank performs reranking with app context for usage tracking
	AppRerank(ctx context.Context, appCtx *AppContext, req *adapter.RerankRequest) (*adapter.RerankResponse, error)
}

// AppContext provides context for app-scoped LLM calls
// This is used for usage tracking and billing attribution
type AppContext struct {
	// OrganizationID is the organization ID used to resolve the billing principal.
	OrganizationID string

	// WorkspaceID is the optional workspace subject for workspace-level quota billing.
	WorkspaceID string

	// BillingSubjectType overrides the quota/billing subject used by Gateway.
	BillingSubjectType string

	// AppID is the agent, dataset, or workflow ID
	AppID string

	// AppType specifies the type of app: "agent", "dataset", or "workflow"
	AppType string

	// ModelUseCase explicitly selects the model routing contract for this call.
	// When empty, Gateway preserves the legacy AppType-derived behavior.
	ModelUseCase string

	// AccountID is the user who triggered the LLM call
	AccountID string

	// SessionID groups LLM calls in an end-user conversation or session.
	SessionID string

	// ConversationID keeps the product conversation identifier for trace metadata.
	ConversationID string

	// WorkflowID identifies the workflow definition that triggered the call.
	WorkflowID string

	// WorkflowRunID identifies the concrete workflow run that triggered the call.
	WorkflowRunID string

	// NodeID identifies the workflow node that triggered the call.
	NodeID string

	// NodeType identifies the workflow node type that triggered the call.
	NodeType string
}

// Validate checks if the AppContext has required fields
func (c *AppContext) Validate() error {
	if c.AppID == "" {
		return ErrAppIDRequired
	}
	if c.AppType == "" {
		return ErrAppTypeRequired
	}
	if c.AccountID == "" {
		return ErrAccountIDRequired
	}
	if strings.TrimSpace(c.BillingSubjectType) != BillingSubjectTypeOrganization && c.WorkspaceID == "" {
		return ErrWorkspaceIDRequired
	}
	return nil
}
