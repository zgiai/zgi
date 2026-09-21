package billing

import (
	"context"
	"fmt"
	"strings"

	"github.com/zgiai/zgi/api/internal/observability"
	"github.com/zgiai/zgi/api/pkg/logger"
	pb "github.com/zgiai/zgi/api/pkg/rpc/v1"
	"go.uber.org/zap"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
)

// Remote is the Cloud implementation that communicates with zgi-console via gRPC.
type Remote struct {
	client   pb.BillingServiceClient
	fallback bool // Whether to allow requests on gRPC failure
}

// NewRemote creates a new remote billing provider. apiKey is the Console-issued
// downstream client key; when set, every billing RPC carries it so Console can
// resolve the downstream identity. An empty key keeps the transitional
// unauthenticated behavior.
func NewRemote(addr, apiKey string) (*Remote, error) {
	dialOptions := []grpc.DialOption{grpc.WithTransportCredentials(insecure.NewCredentials())}
	if token := strings.TrimSpace(apiKey); token != "" {
		dialOptions = append(dialOptions, grpc.WithPerRPCCredentials(consoleAPICredentials{token: token}))
	}
	dialOptions = append(dialOptions, observability.GRPCDialOptions()...)
	conn, err := grpc.Dial(addr, dialOptions...)
	if err != nil {
		return nil, err
	}

	return &Remote{
		client:   pb.NewBillingServiceClient(conn),
		fallback: false,
	}, nil
}

// consoleAPICredentials attaches the Console-issued downstream API key to every
// billing RPC as "authorization: Bearer <key>". Console validates it server-side.
type consoleAPICredentials struct {
	token string
}

func (c consoleAPICredentials) GetRequestMetadata(context.Context, ...string) (map[string]string, error) {
	return map[string]string{"authorization": "Bearer " + c.token}, nil
}

func (c consoleAPICredentials) RequireTransportSecurity() bool { return false }

// SetFallbackBehavior controls what happens when gRPC is unavailable.
// If true, requests are allowed; if false, requests are rejected.
func (r *Remote) SetFallbackBehavior(allow bool) {
	r.fallback = allow
}

// PreCheck validates if the request should proceed (legacy method, now uses PreDeduct).
func (r *Remote) PreCheck(ctx context.Context, organizationID, model, provider string) (bool, string, error) {
	// For backward compatibility, use PreDeduct with estimated credits.
	resp, err := r.PreDeduct(ctx, &PreDeductRequest{
		OrganizationID:   organizationID,
		EstimatedCredits: 1000, // Default estimate
		Model:            model,
		Provider:         provider,
		RequestID:        "",
	})
	if err != nil {
		return false, "billing service error", err
	}
	return resp.Allowed, resp.Reason, nil
}

// RecordUsage reports consumption to zgi-console via gRPC (legacy method, now uses Settle).
func (r *Remote) RecordUsage(ctx context.Context, organizationID string, usage Usage) error {
	// For backward compatibility, use Settle.
	return r.Settle(ctx, &SettleRequest{
		OrganizationID:   organizationID,
		DeductionID:      usage.RequestID,
		EstimatedCredits: 0,
		ActualCredits:    int64(usage.PromptTokens + usage.CompletionTokens),
		PromptTokens:     int64(usage.PromptTokens),
		CompletionTokens: int64(usage.CompletionTokens),
		TotalTokens:      int64(usage.PromptTokens + usage.CompletionTokens),
		Model:            usage.Model,
		Provider:         usage.Provider,
		Status:           "success",
		RequestID:        usage.RequestID,
	})
}

// PreDeduct pre-deducts estimated credits via gRPC.
func (r *Remote) PreDeduct(ctx context.Context, req *PreDeductRequest) (*PreDeductResponse, error) {
	if req == nil {
		return nil, fmt.Errorf("billing pre-deduct request is required")
	}
	resp, err := r.client.PreDeductQuota(ctx, &pb.PreDeductQuotaRequest{
		OrganizationId:   req.OrganizationID,
		EstimatedCredits: req.EstimatedCredits,
		ModelId:          req.Model,
		ModelName:        req.Model,
		ProviderId:       req.Provider,
		ProviderName:     req.Provider,
		RequestId:        req.RequestID,
		AttemptId:        req.AttemptID,
		ResourceType:     req.ResourceType,
		ResourceId:       req.ResourceID,
		ResourceName:     req.ResourceName,
	})

	if err != nil {
		logger.WarnContext(ctx, "Remote billing pre-deduct failed",
			zap.String("organization_id", req.OrganizationID),
			zap.String("model", req.Model),
			zap.String("provider", req.Provider),
			zap.String("request_id", req.RequestID),
			zap.Bool("fallback_allowed", r.fallback),
			zap.Error(err),
		)
		if r.fallback {
			// Fallback: allow request to continue.
			return &PreDeductResponse{
				Allowed:        true,
				DeductionID:    req.RequestID,
				RemainingQuota: -1,
				Reason:         "billing service unavailable, fallback allowed",
			}, nil
		}
		return nil, err
	}
	if resp == nil {
		return nil, fmt.Errorf("billing pre-deduct returned an empty response")
	}

	return &PreDeductResponse{
		Allowed:        resp.Success,
		ErrorCode:      resp.ErrorCode,
		DeductionID:    resp.DeductionId,
		RemainingQuota: resp.RemainingQuota,
		Reason:         resp.ErrorMessage,
	}, nil
}

// Settle settles the actual credits consumption via gRPC.
func (r *Remote) Settle(ctx context.Context, req *SettleRequest) error {
	if req == nil {
		return fmt.Errorf("billing settle request is required")
	}
	resp, err := r.client.SettleQuota(ctx, &pb.SettleQuotaRequest{
		OrganizationId:   req.OrganizationID,
		DeductionId:      req.DeductionID,
		EstimatedCredits: req.EstimatedCredits,
		ActualCredits:    req.ActualCredits,
		PromptTokens:     int32(req.PromptTokens),
		CompletionTokens: int32(req.CompletionTokens),
		TotalTokens:      int32(req.TotalTokens),
		ModelId:          req.Model,
		ModelName:        req.Model,
		ProviderId:       req.Provider,
		ProviderName:     req.Provider,
		Status:           req.Status,
		ErrorMessage:     req.ErrorMessage,
		RequestId:        req.RequestID,
		AttemptId:        req.AttemptID,
		ResourceType:     req.ResourceType,
		ResourceId:       req.ResourceID,
		ResourceName:     req.ResourceName,
	})

	if err != nil {
		logger.WarnContext(ctx, "Remote billing settle failed",
			zap.String("organization_id", req.OrganizationID),
			zap.String("deduction_id", req.DeductionID),
			zap.String("model", req.Model),
			zap.String("provider", req.Provider),
			zap.String("request_id", req.RequestID),
			zap.Error(err),
		)
		return err
	}
	if resp == nil {
		return fmt.Errorf("billing settlement returned an empty response")
	}
	if !resp.Success {
		return fmt.Errorf("billing settlement failed: %s", resp.ErrorMessage)
	}

	return nil
}
