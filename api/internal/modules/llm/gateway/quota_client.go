package gateway

import (
	"context"
	"fmt"
	"time"

	"github.com/zgiai/ginext/internal/observability"
	pb "github.com/zgiai/ginext/pkg/rpc/v1"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
)

type QuotaClient struct {
	conn   *grpc.ClientConn
	client pb.BillingServiceClient
}

func NewQuotaClient(address string) (*QuotaClient, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	dialOptions := []grpc.DialOption{
		grpc.WithTransportCredentials(insecure.NewCredentials()),
		grpc.WithBlock(),
	}
	dialOptions = append(dialOptions, observability.GRPCDialOptions()...)
	conn, err := grpc.DialContext(ctx, address, dialOptions...)
	if err != nil {
		return nil, fmt.Errorf("failed to connect to billing service: %w", err)
	}

	return &QuotaClient{
		conn:   conn,
		client: pb.NewBillingServiceClient(conn),
	}, nil
}

func (c *QuotaClient) Close() error {
	if c.conn != nil {
		return c.conn.Close()
	}
	return nil
}

func (c *QuotaClient) PreDeductQuota(ctx context.Context, req *PreDeductQuotaRequest) (*PreDeductQuotaResponse, error) {
	grpcReq := &pb.PreDeductQuotaRequest{
		OrganizationId:   req.OrganizationID,
		EstimatedCredits: req.EstimatedCredits,
		ModelId:          req.ModelID,
		ModelName:        req.ModelName,
		ProviderId:       req.ProviderID,
		ProviderName:     req.ProviderName,
		RequestId:        req.RequestID,
		AttemptId:        req.AttemptID,
	}

	resp, err := c.client.PreDeductQuota(ctx, grpcReq)
	if err != nil {
		return nil, fmt.Errorf("grpc call failed: %w", err)
	}

	return &PreDeductQuotaResponse{
		Success:        resp.Success,
		ErrorCode:      resp.ErrorCode,
		ErrorMessage:   resp.ErrorMessage,
		RemainingQuota: resp.RemainingQuota,
		DeductionID:    resp.DeductionId,
	}, nil
}

func (c *QuotaClient) SettleQuota(ctx context.Context, req *SettleQuotaRequest) (*SettleQuotaResponse, error) {
	grpcReq := &pb.SettleQuotaRequest{
		OrganizationId:    req.OrganizationID,
		DeductionId:       req.DeductionID,
		EstimatedCredits:  req.EstimatedCredits,
		ActualCredits:     req.ActualCredits,
		PromptTokens:      int32(req.PromptTokens),
		CompletionTokens:  int32(req.CompletionTokens),
		TotalTokens:       int32(req.TotalTokens),
		InputCost:         req.InputCost,
		OutputCost:        req.OutputCost,
		TotalCost:         req.TotalCost,
		ModelId:           req.ModelID,
		ModelName:         req.ModelName,
		ProviderId:        req.ProviderID,
		ProviderName:      req.ProviderName,
		ChannelId:         req.ChannelID,
		RequestId:         req.RequestID,
		ResponseTime:      req.ResponseTime,
		Status:            req.Status,
		ErrorMessage:      req.ErrorMessage,
		AccountId:         req.AccountID,
		AppId:             req.AppID,
		AppType:           req.AppType,
		IpAddress:         req.IPAddress,
		UserAgent:         req.UserAgent,
		IsStreaming:       req.IsStreaming,
		UseSystemProvider: req.UseSystemProvider,
		AttemptId:         req.AttemptID,
	}

	resp, err := c.client.SettleQuota(ctx, grpcReq)
	if err != nil {
		return nil, fmt.Errorf("grpc call failed: %w", err)
	}

	return &SettleQuotaResponse{
		Success:         resp.Success,
		ErrorMessage:    resp.ErrorMessage,
		RemainingQuota:  resp.RemainingQuota,
		UsedQuota:       resp.UsedQuota,
		RefundedCredits: resp.RefundedCredits,
		SettledCredits:  resp.SettledCredits,
	}, nil
}

func (c *QuotaClient) CheckCreditBalance(ctx context.Context, organizationID string, estimatedCredits int64) (bool, int64, error) {
	grpcReq := &pb.CheckCreditBalanceRequest{
		OrganizationId:   organizationID,
		EstimatedCredits: estimatedCredits,
	}

	resp, err := c.client.CheckCreditBalance(ctx, grpcReq)
	if err != nil {
		return false, 0, fmt.Errorf("grpc call failed: %w", err)
	}

	if !resp.Success {
		return false, 0, fmt.Errorf("check balance failed: %s", resp.ErrorMessage)
	}

	return resp.Sufficient, resp.Balance, nil
}

type PreDeductQuotaRequest struct {
	OrganizationID   string
	EstimatedCredits int64
	ModelID          string
	ModelName        string
	ProviderID       string
	ProviderName     string
	RequestID        string
	AttemptID        string
}

type PreDeductQuotaResponse struct {
	Success        bool
	ErrorCode      string
	ErrorMessage   string
	RemainingQuota int64
	DeductionID    string
}

type SettleQuotaRequest struct {
	OrganizationID    string
	DeductionID       string
	EstimatedCredits  int64
	ActualCredits     int64
	PromptTokens      int
	CompletionTokens  int
	TotalTokens       int
	InputCost         float64
	OutputCost        float64
	TotalCost         float64
	ModelID           string
	ModelName         string
	ProviderID        string
	ProviderName      string
	ChannelID         string
	RequestID         string
	ResponseTime      int64
	Status            string
	ErrorMessage      string
	AccountID         string
	AppID             string
	AppType           string
	IPAddress         string
	UserAgent         string
	IsStreaming       bool
	UseSystemProvider bool
	AttemptID         string
}

type SettleQuotaResponse struct {
	Success         bool
	ErrorMessage    string
	RemainingQuota  int64
	UsedQuota       int64
	RefundedCredits int64
	SettledCredits  int64
}
