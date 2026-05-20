package imagegen

import (
	"context"
	"fmt"
	"strings"

	llmclient "github.com/zgiai/zgi/api/internal/modules/llm/client"
	llmadapter "github.com/zgiai/zgi/api/internal/modules/llm/protocol/adapters"
)

type gatewayImageInvoker struct {
	client             llmclient.LLMClient
	organizationID     string
	workspaceID        string
	billingSubjectType string
}

func NewGatewayImageInvoker(client llmclient.LLMClient, organizationID string, workspaceID string, billingSubjectType string) (ImageInvoker, error) {
	if client == nil {
		return nil, nil
	}

	workspaceID = strings.TrimSpace(workspaceID)
	if workspaceID == "" {
		return nil, fmt.Errorf("workspace_id is required for workflow image invoker")
	}

	return &gatewayImageInvoker{
		client:             client,
		organizationID:     strings.TrimSpace(organizationID),
		workspaceID:        workspaceID,
		billingSubjectType: strings.TrimSpace(billingSubjectType),
	}, nil
}

func (g *gatewayImageInvoker) Invoke(ctx context.Context, accountID, appID, appType string, req *InvokeRequest) (*InvokeResult, error) {
	if g == nil || g.client == nil {
		return nil, ErrInvokerNotConfigured
	}

	imageReq := &llmadapter.ImageRequest{
		Model:          req.ModelSlug,
		Prompt:         req.Prompt,
		Size:           req.Size,
		Quality:        req.Quality,
		Style:          req.Style,
		ResponseFormat: "url",
		User:           req.UserID,
	}
	if req.N > 0 {
		imageReq.N = &req.N
	}

	appCtx := &llmclient.AppContext{
		AppID:              appID,
		AppType:            appType,
		AccountID:          accountID,
		OrganizationID:     g.organizationID,
		WorkspaceID:        g.workspaceID,
		BillingSubjectType: g.billingSubjectType,
	}

	resp, err := g.client.AppCreateImage(ctx, appCtx, imageReq)
	if err != nil {
		return nil, fmt.Errorf("image generation failed: %w", err)
	}
	if resp == nil {
		return nil, fmt.Errorf("empty image response: response is nil")
	}

	return &InvokeResult{
		Images: resp.Data,
	}, nil
}
