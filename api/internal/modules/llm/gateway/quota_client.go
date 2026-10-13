package gateway

import (
	"context"
	"errors"
	"fmt"
	"math"
	"strings"
	"time"

	"github.com/shopspring/decimal"
	appconfig "github.com/zgiai/zgi/api/config"
	"github.com/zgiai/zgi/api/internal/infra/platform/consoleauth"
	"github.com/zgiai/zgi/api/internal/infra/platform/consolecred"
	"github.com/zgiai/zgi/api/internal/observability"
	pb "github.com/zgiai/zgi/api/pkg/rpc/v1"
	"google.golang.org/genproto/googleapis/rpc/errdetails"
	"google.golang.org/grpc"
	"google.golang.org/grpc/status"
)

type QuotaClient struct {
	conn   *grpc.ClientConn
	client pb.BillingServiceClient
}

func NewQuotaClient(address string) (*QuotaClient, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	transportCredentials, err := consolecred.TransportCredentials(address)
	if err != nil {
		return nil, err
	}
	dialOptions := []grpc.DialOption{
		grpc.WithTransportCredentials(transportCredentials),
		grpc.WithBlock(),
	}
	// Two distinct credentials travel on every billing RPC. The per-instance
	// control-plane credential is attached first: it contributes the X-Internal-*
	// HMAC headers (signed through consolecred over the full gRPC method) the console
	// instance interceptor verifies. The Console-issued downstream client key is
	// attached last so that it owns the "authorization" metadata key the console
	// downstream interceptor resolves. Neither credential is required to be present.
	dialOptions = append(dialOptions, grpc.WithPerRPCCredentials(consolecred.NewGRPCCredentials()))
	if cfg := appconfig.Current(); cfg != nil {
		if key := strings.TrimSpace(cfg.Console.InternalAPIKey); key != "" {
			dialOptions = append(dialOptions, grpc.WithPerRPCCredentials(consoleauth.Credentials{Token: key}))
		}
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

// BillingRPCError carries the stable Console ErrorInfo.Reason alongside the
// transport failure so callers can classify a billing failure without parsing
// transport or free-text messages.
type BillingRPCError struct {
	Reason string
	Err    error
}

func (e *BillingRPCError) Error() string {
	if e == nil {
		return ""
	}
	if e.Err == nil {
		return e.Reason
	}
	if strings.TrimSpace(e.Reason) == "" {
		return e.Err.Error()
	}
	return e.Reason + ": " + e.Err.Error()
}

func (e *BillingRPCError) Unwrap() error {
	if e == nil {
		return nil
	}
	return e.Err
}

func newBillingRPCError(err error) error {
	if err == nil {
		return nil
	}
	return &BillingRPCError{Reason: billingErrorInfoReason(err), Err: err}
}

// billingErrorInfoReason extracts google.rpc.ErrorInfo.Reason from a gRPC
// status, returning "" when the server predates the ErrorInfo contract.
func billingErrorInfoReason(err error) string {
	statusError, ok := status.FromError(err)
	if !ok || statusError == nil {
		return ""
	}
	for _, detail := range statusError.Details() {
		if info, ok := detail.(*errdetails.ErrorInfo); ok && info != nil {
			return strings.TrimSpace(info.Reason)
		}
	}
	return ""
}

// billingRPCErrorReason returns the stable reason recorded by this client, or
// "" when the error predates ErrorInfo.
func billingRPCErrorReason(err error) string {
	var rpcErr *BillingRPCError
	if errors.As(err, &rpcErr) && rpcErr != nil {
		return rpcErr.Reason
	}
	return ""
}

func (c *QuotaClient) Close() error {
	if c.conn != nil {
		return c.conn.Close()
	}
	return nil
}

func (c *QuotaClient) PreDeductQuota(ctx context.Context, req *PreDeductQuotaRequest) (*PreDeductQuotaResponse, error) {
	grpcReq := &pb.PreDeductQuotaRequest{
		OrganizationId:    req.OrganizationID,
		EstimatedCredits:  req.EstimatedCredits,
		ModelId:           req.ModelID,
		ModelName:         req.ModelName,
		ProviderId:        req.ProviderID,
		ProviderName:      req.ProviderName,
		RequestId:         req.RequestID,
		AttemptId:         req.AttemptID,
		ReservationPolicy: req.ReservationPolicy,
		PricingSnapshot:   req.PricingSnapshot,
	}

	resp, err := c.client.PreDeductQuota(ctx, grpcReq)
	if err != nil {
		return nil, fmt.Errorf("grpc call failed: %w", newBillingRPCError(err))
	}

	return &PreDeductQuotaResponse{
		Success:        resp.Success,
		ErrorCode:      resp.ErrorCode,
		ErrorMessage:   resp.ErrorMessage,
		RemainingQuota: resp.RemainingQuota,
		DeductionID:    resp.DeductionId,
	}, nil
}

func (c *QuotaClient) PreDeductMeteredUsage(ctx context.Context, req *PreDeductMeteredUsageRequest) (*PreDeductMeteredUsageResponse, error) {
	grpcReq := &pb.PreDeductMeteredUsageRequest{
		OrganizationId: req.OrganizationID,
		ModelId:        req.ModelID,
		ProviderName:   req.ProviderName,
		ModelName:      req.ModelName,
		RequestId:      req.RequestID,
		AttemptId:      req.AttemptID,
		ResourceType:   req.ResourceType,
		ResourceId:     req.ResourceID,
		ResourceName:   req.ResourceName,
	}
	if req.Usage.Operation != "" || req.Usage.Meter != "" || req.Usage.BaseUnit != "" || req.Usage.Quantity != 0 || len(req.Usage.Dimensions) > 0 {
		grpcReq.Usage = &pb.MeteredUsageFact{
			Operation:  req.Usage.Operation,
			Meter:      req.Usage.Meter,
			BaseUnit:   req.Usage.BaseUnit,
			Quantity:   req.Usage.Quantity,
			Dimensions: req.Usage.Dimensions,
		}
	}

	resp, err := c.client.PreDeductMeteredUsage(ctx, grpcReq)
	if err != nil {
		return nil, fmt.Errorf("grpc call failed: %w", newBillingRPCError(err))
	}

	return &PreDeductMeteredUsageResponse{
		Success:          resp.Success,
		ErrorCode:        resp.ErrorCode,
		ErrorMessage:     resp.ErrorMessage,
		DeductionID:      resp.DeductionId,
		EstimatedCredits: resp.EstimatedCredits,
		ReservedCredits:  resp.ReservedCredits,
		RemainingCredits: resp.RemainingCredits,
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
		InputCost:         decimalToWireFloat(req.InputCost),
		OutputCost:        decimalToWireFloat(req.OutputCost),
		TotalCost:         decimalToWireFloat(req.TotalCost),
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
		return nil, fmt.Errorf("grpc call failed: %w", newBillingRPCError(err))
	}

	return &SettleQuotaResponse{
		Success:         resp.Success,
		ErrorCode:       resp.ErrorCode,
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

func (c *QuotaClient) CalculateDualCost(ctx context.Context, modelID, provider, model string, promptTokens, completionTokens int) (*DualCostQuotaResponse, error) {
	const maxGRPCTokenCount = int64(1<<31 - 1)
	if promptTokens < 0 || completionTokens < 0 || int64(promptTokens) > maxGRPCTokenCount || int64(completionTokens) > maxGRPCTokenCount {
		return nil, fmt.Errorf("token count is outside the billing RPC range")
	}
	resp, err := c.client.CalculateDualCost(ctx, &pb.CalculateDualCostRequest{
		ModelId:          modelID,
		Provider:         provider,
		Model:            model,
		PromptTokens:     int32(promptTokens),
		CompletionTokens: int32(completionTokens),
	})
	if err != nil {
		// Carry the Console ErrorInfo.Reason so callers classify a stable billing
		// failure instead of a transport error, exactly like the pre-deduct paths.
		return nil, fmt.Errorf("grpc call failed: %w", newBillingRPCError(err))
	}
	inputUSD, err := wireFloatToDecimal(resp.InputUsd)
	if err != nil {
		return nil, fmt.Errorf("decode authoritative input usd: %w", err)
	}
	outputUSD, err := wireFloatToDecimal(resp.OutputUsd)
	if err != nil {
		return nil, fmt.Errorf("decode authoritative output usd: %w", err)
	}
	totalUSD, err := wireFloatToDecimal(resp.TotalUsd)
	if err != nil {
		return nil, fmt.Errorf("decode authoritative total usd: %w", err)
	}
	return &DualCostQuotaResponse{
		Success:       resp.Success,
		ErrorMessage:  resp.ErrorMessage,
		InputCredits:  resp.InputCredits,
		OutputCredits: resp.OutputCredits,
		TotalCredits:  resp.TotalCredits,
		InputUSD:      inputUSD,
		OutputUSD:     outputUSD,
		TotalUSD:      totalUSD,
	}, nil
}

type PreDeductQuotaRequest struct {
	OrganizationID    string
	EstimatedCredits  int64
	ModelID           string
	ModelName         string
	ProviderID        string
	ProviderName      string
	RequestID         string
	AttemptID         string
	ReservationPolicy string
	PricingSnapshot   string
}

type PreDeductQuotaResponse struct {
	Success        bool
	ErrorCode      string
	ErrorMessage   string
	RemainingQuota int64
	DeductionID    string
}

// MeteredUsageFact declares a non-token billing identity. Quantity is a raw
// base-unit count, never a point or token amount.
type MeteredUsageFact struct {
	Operation  string
	Meter      string
	BaseUnit   string
	Quantity   int64
	Dimensions map[string]string
}

type PreDeductMeteredUsageRequest struct {
	OrganizationID string
	ModelID        string
	ProviderName   string
	ModelName      string
	RequestID      string
	AttemptID      string
	ResourceType   string
	ResourceID     string
	ResourceName   string
	Usage          MeteredUsageFact
}

type PreDeductMeteredUsageResponse struct {
	Success          bool
	ErrorCode        string
	ErrorMessage     string
	DeductionID      string
	EstimatedCredits int64
	ReservedCredits  int64
	RemainingCredits int64
}

type SettleQuotaRequest struct {
	OrganizationID   string
	DeductionID      string
	EstimatedCredits int64
	ActualCredits    int64
	PromptTokens     int
	CompletionTokens int
	TotalTokens      int
	// InputCost/OutputCost/TotalCost are exact credit (points) amounts. The
	// legacy gRPC contract carries them as doubles; see decimalToWireFloat.
	InputCost         decimal.Decimal
	OutputCost        decimal.Decimal
	TotalCost         decimal.Decimal
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
	ErrorCode       string
	ErrorMessage    string
	RemainingQuota  int64
	UsedQuota       int64
	RefundedCredits int64
	SettledCredits  int64
}

type DualCostQuotaResponse struct {
	Success       bool
	ErrorMessage  string
	InputCredits  int64
	OutputCredits int64
	TotalCredits  int64
	// InputUSD/OutputUSD/TotalUSD are exact USD amounts decoded from the
	// legacy double wire fields; see wireFloatToDecimal.
	InputUSD  decimal.Decimal
	OutputUSD decimal.Decimal
	TotalUSD  decimal.Decimal
}

// decimalToWireFloat converts an internal decimal money amount to the legacy
// double field the existing Console billing peer consumes. This is the single
// explicit float boundary on the Pre side and no money arithmetic happens on
// the result. Internal decimal values remain the source of truth; a
// scaled-int/string wire field can replace this once both peers advertise it.
func decimalToWireFloat(value decimal.Decimal) float64 {
	return value.InexactFloat64()
}

// wireFloatToDecimal converts a legacy double wire amount into an exact
// internal decimal. The double is a transport representation only; every
// pricing and settlement computation after this boundary uses decimal. A
// non-finite peer value is rejected so fail-closed validation is preserved.
func wireFloatToDecimal(value float64) (decimal.Decimal, error) {
	if math.IsNaN(value) || math.IsInf(value, 0) {
		return decimal.Zero, fmt.Errorf("non-finite money value %v", value)
	}
	return decimal.NewFromFloat(value), nil
}
