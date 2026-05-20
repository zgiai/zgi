package client

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	apikeymodel "github.com/zgiai/zgi/api/internal/modules/llm/apikey/model"
	apikeyrepo "github.com/zgiai/zgi/api/internal/modules/llm/apikey/repository"
	"github.com/zgiai/zgi/api/internal/modules/llm/gateway"
	adapter "github.com/zgiai/zgi/api/internal/modules/llm/protocol/adapters"
	"github.com/zgiai/zgi/api/internal/util"
	"gorm.io/gorm"
)

const systemKeyName = "Internal System Key"

func isUnsetOrganizationID(organizationID string) bool {
	return strings.TrimSpace(organizationID) == "" || strings.TrimSpace(organizationID) == uuid.Nil.String()
}

// llmClientImpl implements the LLMClient interface
type llmClientImpl struct {
	gateway    gateway.LLMGatewayService
	apiKeyRepo apikeyrepo.APIKeyRepository
	db         *gorm.DB
}

// New creates a new LLMClient instance
func New(
	gateway gateway.LLMGatewayService,
	apiKeyRepo apikeyrepo.APIKeyRepository,
	db *gorm.DB,
) LLMClient {
	return &llmClientImpl{
		gateway:    gateway,
		apiKeyRepo: apiKeyRepo,
		db:         db,
	}
}

// Chat performs a non-streaming chat completion request
func (c *llmClientImpl) Chat(ctx context.Context, organizationID string, req *adapter.ChatRequest) (*adapter.ChatResponse, error) {
	apiKey, err := c.getOrCreateSystemKey(ctx, organizationID)
	if err != nil {
		return nil, fmt.Errorf("failed to get system API key: %w", err)
	}
	return c.gateway.ChatCompletion(ctx, apiKey, req)
}

// ChatStream performs a streaming chat completion request
func (c *llmClientImpl) ChatStream(ctx context.Context, organizationID string, req *adapter.ChatRequest) (<-chan adapter.StreamResponse, error) {
	apiKey, err := c.getOrCreateSystemKey(ctx, organizationID)
	if err != nil {
		return nil, fmt.Errorf("failed to get system API key: %w", err)
	}
	return c.gateway.ChatCompletionStream(ctx, apiKey, req)
}

// CreateResponse performs a create response request
func (c *llmClientImpl) CreateResponse(ctx context.Context, organizationID string, req *adapter.CreateResponseRequest) (*adapter.CreateResponseResponse, error) {
	apiKey, err := c.getOrCreateSystemKey(ctx, organizationID)
	if err != nil {
		return nil, fmt.Errorf("failed to get system API key: %w", err)
	}
	return c.gateway.CreateResponse(ctx, apiKey, req)
}

// Embed creates text embeddings
func (c *llmClientImpl) Embed(ctx context.Context, organizationID string, req *adapter.EmbeddingsRequest) (*adapter.EmbeddingsResponse, error) {
	apiKey, err := c.getOrCreateSystemKey(ctx, organizationID)
	if err != nil {
		return nil, fmt.Errorf("failed to get system API key: %w", err)
	}
	return c.gateway.CreateEmbeddings(ctx, apiKey, req)
}

// CreateImage performs an image generation request
func (c *llmClientImpl) CreateImage(ctx context.Context, organizationID string, req *adapter.ImageRequest) (*adapter.ImageResponse, error) {
	apiKey, err := c.getOrCreateSystemKey(ctx, organizationID)
	if err != nil {
		return nil, fmt.Errorf("failed to get system API key: %w", err)
	}
	return c.gateway.CreateImage(ctx, apiKey, req)
}

// Rerank performs document reranking
func (c *llmClientImpl) Rerank(ctx context.Context, organizationID string, req *adapter.RerankRequest) (*adapter.RerankResponse, error) {
	apiKey, err := c.getOrCreateSystemKey(ctx, organizationID)
	if err != nil {
		return nil, fmt.Errorf("failed to get system API key: %w", err)
	}
	return c.gateway.Rerank(ctx, apiKey, req)
}

// AppChat performs a chat completion request with app context
func (c *llmClientImpl) AppChat(ctx context.Context, appCtx *AppContext, req *adapter.ChatRequest) (*adapter.ChatResponse, error) {
	if err := appCtx.Validate(); err != nil {
		return nil, err
	}

	organizationID, err := c.resolveOrganizationID(ctx, appCtx)
	if err != nil {
		return nil, err
	}

	apiKey, err := c.getOrCreateSystemKey(ctx, organizationID)
	if err != nil {
		return nil, fmt.Errorf("failed to get system API key: %w", err)
	}

	gwAppCtx, err := c.buildGatewayAppContext(appCtx)
	if err != nil {
		return nil, err
	}

	return c.gateway.ChatCompletionWithAppContext(ctx, apiKey, gwAppCtx, req)
}

// AppChatStream performs a streaming chat completion request with app context
func (c *llmClientImpl) AppChatStream(ctx context.Context, appCtx *AppContext, req *adapter.ChatRequest) (<-chan adapter.StreamResponse, error) {
	if err := appCtx.Validate(); err != nil {
		return nil, err
	}

	organizationID, err := c.resolveOrganizationID(ctx, appCtx)
	if err != nil {
		return nil, err
	}

	apiKey, err := c.getOrCreateSystemKey(ctx, organizationID)
	if err != nil {
		return nil, fmt.Errorf("failed to get system API key: %w", err)
	}

	gwAppCtx, err := c.buildGatewayAppContext(appCtx)
	if err != nil {
		return nil, err
	}

	return c.gateway.ChatCompletionStreamWithAppContext(ctx, apiKey, gwAppCtx, req)
}

// AppCreateResponse performs a create response request with app context
func (c *llmClientImpl) AppCreateResponse(ctx context.Context, appCtx *AppContext, req *adapter.CreateResponseRequest) (*adapter.CreateResponseResponse, error) {
	if err := appCtx.Validate(); err != nil {
		return nil, err
	}

	organizationID, err := c.resolveOrganizationID(ctx, appCtx)
	if err != nil {
		return nil, err
	}

	apiKey, err := c.getOrCreateSystemKey(ctx, organizationID)
	if err != nil {
		return nil, fmt.Errorf("failed to get system API key: %w", err)
	}

	gwAppCtx, err := c.buildGatewayAppContext(appCtx)
	if err != nil {
		return nil, err
	}

	return c.gateway.CreateResponseWithAppContext(ctx, apiKey, gwAppCtx, req)
}

// AppEmbed creates embeddings with app context
func (c *llmClientImpl) AppEmbed(ctx context.Context, appCtx *AppContext, req *adapter.EmbeddingsRequest) (*adapter.EmbeddingsResponse, error) {
	if err := appCtx.Validate(); err != nil {
		return nil, err
	}

	organizationID, err := c.resolveOrganizationID(ctx, appCtx)
	if err != nil {
		return nil, err
	}

	apiKey, err := c.getOrCreateSystemKey(ctx, organizationID)
	if err != nil {
		return nil, fmt.Errorf("failed to get system API key: %w", err)
	}

	gwAppCtx, err := c.buildGatewayAppContext(appCtx)
	if err != nil {
		return nil, err
	}

	return c.gateway.CreateEmbeddingsWithAppContext(ctx, apiKey, gwAppCtx, req)
}

// AppCreateImage performs an image generation request with app context
func (c *llmClientImpl) AppCreateImage(ctx context.Context, appCtx *AppContext, req *adapter.ImageRequest) (*adapter.ImageResponse, error) {
	if err := appCtx.Validate(); err != nil {
		return nil, err
	}

	organizationID, err := c.resolveOrganizationID(ctx, appCtx)
	if err != nil {
		return nil, err
	}

	apiKey, err := c.getOrCreateSystemKey(ctx, organizationID)
	if err != nil {
		return nil, fmt.Errorf("failed to get system API key: %w", err)
	}

	gwAppCtx, err := c.buildGatewayAppContext(appCtx)
	if err != nil {
		return nil, err
	}

	return c.gateway.CreateImageWithAppContext(ctx, apiKey, gwAppCtx, req)
}

// AppRerank performs reranking with app context
func (c *llmClientImpl) AppRerank(ctx context.Context, appCtx *AppContext, req *adapter.RerankRequest) (*adapter.RerankResponse, error) {
	if err := appCtx.Validate(); err != nil {
		return nil, err
	}

	organizationID, err := c.resolveOrganizationID(ctx, appCtx)
	if err != nil {
		return nil, err
	}

	apiKey, err := c.getOrCreateSystemKey(ctx, organizationID)
	if err != nil {
		return nil, fmt.Errorf("failed to get system API key: %w", err)
	}

	gwAppCtx, err := c.buildGatewayAppContext(appCtx)
	if err != nil {
		return nil, err
	}

	return c.gateway.RerankWithAppContext(ctx, apiKey, gwAppCtx, req)
}

// getOrCreateSystemKey retrieves or creates an internal system API key for the tenant
func (c *llmClientImpl) getOrCreateSystemKey(ctx context.Context, organizationID string) (*apikeymodel.TenantAPIKey, error) {
	// Try to find existing system key
	filters := map[string]interface{}{
		"is_internal": true,
	}
	keys, _, err := c.apiKeyRepo.List(ctx, organizationID, filters, 1, 1)
	if err != nil {
		return nil, err
	}

	if len(keys) > 0 {
		return keys[0], nil
	}

	// Create new internal system key
	rawKey := "sk-internal-" + uuid.New().String()

	encryptedKey, err := util.EncryptAPIKey(rawKey)
	if err != nil {
		return nil, fmt.Errorf("failed to encrypt key: %w", err)
	}

	keyHash := util.HashAPIKey(rawKey)

	newKey := &apikeymodel.TenantAPIKey{
		OrganizationID: organizationID,
		Name:           systemKeyName,
		Key:            encryptedKey,
		KeyHash:        keyHash,
		Status:         "active",
		IsInternal:     true,
		QuotaLimit:     nil, // Unlimited
		AllowIPs:       "",
		CreatedAt:      time.Now(),
		UpdatedAt:      time.Now(),
	}

	if err := c.apiKeyRepo.Create(ctx, newKey); err != nil {
		return nil, err
	}

	return newKey, nil
}

// resolveOrganizationID resolves organization_id from AppContext.
func (c *llmClientImpl) resolveOrganizationID(ctx context.Context, appCtx *AppContext) (string, error) {
	// If OrganizationID is provided, use it directly
	if !isUnsetOrganizationID(appCtx.OrganizationID) {
		return appCtx.OrganizationID, nil
	}

	// Workspace is the direct billing subject for workflow-scoped app calls.
	if !isUnsetOrganizationID(appCtx.WorkspaceID) {
		organizationID, err := c.getOrganizationIDFromWorkspace(ctx, appCtx.WorkspaceID)
		if err == nil {
			return organizationID, nil
		}
		if !errors.Is(err, gorm.ErrRecordNotFound) {
			return "", err
		}
	}

	// Built-in/system tools may carry zero UUID organization/workspace IDs.
	// In that case, attribute usage to the caller's current organization.
	if !isUnsetOrganizationID(appCtx.AccountID) {
		organizationID, err := c.getOrganizationIDFromCaller(ctx, appCtx.AccountID)
		if err == nil {
			return organizationID, nil
		}
		if !errors.Is(err, gorm.ErrRecordNotFound) {
			return "", err
		}
	}

	// Otherwise, look up from the app
	return c.getOrganizationIDFromApp(ctx, appCtx.AppID, appCtx.AppType)
}

func (c *llmClientImpl) getOrganizationIDFromWorkspace(ctx context.Context, workspaceID string) (string, error) {
	var workspace struct {
		OrganizationID string `gorm:"column:organization_id"`
	}
	if err := c.db.WithContext(ctx).Table("workspaces").
		Select("organization_id").
		Where("id = ?", workspaceID).
		Take(&workspace).Error; err != nil {
		return "", fmt.Errorf("failed to get workspace organization_id: %w", err)
	}
	return workspace.OrganizationID, nil
}

func (c *llmClientImpl) getOrganizationIDFromCaller(ctx context.Context, accountID string) (string, error) {
	if strings.TrimSpace(accountID) == "" {
		return "", gorm.ErrRecordNotFound
	}

	var accountContext struct {
		CurrentOrganizationID *string `gorm:"column:current_organization_id"`
		CurrentWorkspaceID    *string `gorm:"column:current_workspace_id"`
	}
	if err := c.db.WithContext(ctx).Table("account_contexts").
		Select("current_organization_id", "current_workspace_id").
		Where("account_id = ?", accountID).
		Take(&accountContext).Error; err != nil {
		return "", fmt.Errorf("failed to get caller organization_id: %w", err)
	}

	if accountContext.CurrentOrganizationID != nil && !isUnsetOrganizationID(*accountContext.CurrentOrganizationID) {
		return *accountContext.CurrentOrganizationID, nil
	}
	if accountContext.CurrentWorkspaceID != nil && !isUnsetOrganizationID(*accountContext.CurrentWorkspaceID) {
		return c.getOrganizationIDFromWorkspace(ctx, *accountContext.CurrentWorkspaceID)
	}

	return "", gorm.ErrRecordNotFound
}

// getOrganizationIDFromApp retrieves organization_id from an app.
func (c *llmClientImpl) getOrganizationIDFromApp(ctx context.Context, appID string, appType string) (string, error) {
	switch appType {
	case "agent":
		var agent struct {
			OrganizationID string `gorm:"column:organization_id"`
		}
		if err := c.db.WithContext(ctx).Table("agents AS a").
			Select("w.organization_id").
			Joins("JOIN workspaces w ON w.id = a.tenant_id").
			Where("a.id = ? AND a.deleted_at IS NULL", appID).
			Take(&agent).Error; err != nil {
			return "", fmt.Errorf("failed to get agent organization_id: %w", err)
		}
		return agent.OrganizationID, nil

	case "dataset":
		var dataset struct {
			OrganizationID string `gorm:"column:organization_id"`
		}
		if err := c.db.WithContext(ctx).Table("datasets").
			Select("organization_id").
			Where("id = ?", appID).
			First(&dataset).Error; err != nil {
			return "", fmt.Errorf("failed to get dataset organization_id: %w", err)
		}
		return dataset.OrganizationID, nil

	case "workflow":
		var workflow struct {
			OrganizationID uuid.UUID `gorm:"column:organization_id"`
		}
		if err := c.db.WithContext(ctx).Table("workflows").
			Select("organization_id").
			Where("id = ? AND deleted_at IS NULL", appID).
			First(&workflow).Error; err != nil {
			return "", fmt.Errorf("failed to get workflow organization_id: %w", err)
		}
		return workflow.OrganizationID.String(), nil

	default:
		return "", ErrInvalidAppType
	}
}

// buildGatewayAppContext converts AppContext to gateway.AppContext
func (c *llmClientImpl) buildGatewayAppContext(appCtx *AppContext) (*gateway.AppContext, error) {
	appUUID, err := uuid.Parse(appCtx.AppID)
	if err != nil {
		return nil, fmt.Errorf("invalid app ID: %w", err)
	}

	accountUUID, err := uuid.Parse(appCtx.AccountID)
	if err != nil {
		return nil, fmt.Errorf("invalid account ID: %w", err)
	}

	gatewayCtx := &gateway.AppContext{
		AppID:          &appUUID,
		AppType:        &appCtx.AppType,
		AccountID:      &accountUUID,
		SessionID:      appCtx.SessionID,
		ConversationID: appCtx.ConversationID,
		WorkflowID:     appCtx.WorkflowID,
		WorkflowRunID:  appCtx.WorkflowRunID,
		NodeID:         appCtx.NodeID,
		NodeType:       appCtx.NodeType,
	}
	if appCtx.BillingSubjectType != "" {
		billingSubjectType := appCtx.BillingSubjectType
		gatewayCtx.BillingSubjectType = &billingSubjectType
	}
	if appCtx.WorkspaceID != "" {
		workspaceID := appCtx.WorkspaceID
		gatewayCtx.WorkspaceID = &workspaceID
	}

	return gatewayCtx, nil
}
