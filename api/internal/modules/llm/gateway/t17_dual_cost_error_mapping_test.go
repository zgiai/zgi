package gateway

import (
	"context"
	"errors"
	"testing"

	"github.com/google/uuid"
	pb "github.com/zgiai/zgi/api/pkg/rpc/v1"
	"google.golang.org/genproto/googleapis/rpc/errdetails"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// failingDualCostRPCClient returns a canned gRPC error for CalculateDualCost so
// the QuotaClient wrapping can be exercised without a live Console.
type failingDualCostRPCClient struct {
	pb.BillingServiceClient
	err error
}

func (c *failingDualCostRPCClient) CalculateDualCost(context.Context, *pb.CalculateDualCostRequest, ...grpc.CallOption) (*pb.DualCostResponse, error) {
	return nil, c.err
}

// T17 acceptance 2: the Console ErrorInfo.Reason survives the QuotaClient
// transport wrapper, so the pricing failure can be classified without parsing
// transport text.
func TestT17QuotaClientCalculateDualCostPreservesErrorInfoReason(t *testing.T) {
	statusError := status.New(codes.FailedPrecondition, "failed precondition")
	withDetails, err := statusError.WithDetails(&errdetails.ErrorInfo{
		Reason: "billing.model.pricing_not_configured",
		Domain: "zgi.billing",
	})
	if err != nil {
		t.Fatal(err)
	}
	client := &QuotaClient{client: &failingDualCostRPCClient{err: withDetails.Err()}}

	_, callErr := client.CalculateDualCost(context.Background(), "local-model-id", "openai", "text-embedding-3-small", 10, 0)
	if callErr == nil {
		t.Fatal("CalculateDualCost() error = nil, want gRPC failure")
	}
	if reason := billingRPCErrorReason(callErr); reason != "billing.model.pricing_not_configured" {
		t.Fatalf("billingRPCErrorReason() = %q, want billing.model.pricing_not_configured", reason)
	}
	mapped, ok := mapBillingRPCErrorCode(billingRPCErrorReason(callErr), "")
	if !ok || !errors.Is(mapped, ErrPricingNotConfigured) {
		t.Fatalf("mapped = %v, %v; want ErrPricingNotConfigured", mapped, ok)
	}
}

// T17 acceptance 2: the authoritative token-quote path turns a
// pricing_not_configured RPC failure into ErrPricingNotConfigured instead of a
// generic transport error, so the projector returns 402 and never a 500.
func TestT17QuotePlatformTokenPricingMapsPricingNotConfigured(t *testing.T) {
	client := &authoritativeQuoteQuotaClient{err: &BillingRPCError{
		Reason: "billing.model.pricing_not_configured",
		Err:    errors.New("grpc call failed: failed precondition"),
	}}
	rb := &RemoteBilling{grpcClient: client}

	_, err := rb.QuotePlatformTokenPricing(context.Background(), PricingModelRef{
		ModelID: uuid.New(), Provider: "openai", Model: "text-embedding-3-small",
	}, 10, 0)
	if !errors.Is(err, ErrPricingNotConfigured) {
		t.Fatalf("QuotePlatformTokenPricing() error = %v, want ErrPricingNotConfigured", err)
	}
}
