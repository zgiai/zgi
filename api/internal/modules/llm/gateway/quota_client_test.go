package gateway

import (
	"context"
	"testing"

	pb "github.com/zgiai/zgi/api/pkg/rpc/v1"
	"google.golang.org/grpc"
)

type recordingQuotaRPCClient struct {
	pb.BillingServiceClient
	request         *pb.PreDeductQuotaRequest
	meteredRequest  *pb.PreDeductMeteredUsageRequest
	dualCostRequest *pb.CalculateDualCostRequest
}

func (c *recordingQuotaRPCClient) PreDeductMeteredUsage(_ context.Context, req *pb.PreDeductMeteredUsageRequest, _ ...grpc.CallOption) (*pb.PreDeductMeteredUsageResponse, error) {
	c.meteredRequest = req
	return &pb.PreDeductMeteredUsageResponse{Success: true, DeductionId: "metered-deduction-1", EstimatedCredits: 100000, ReservedCredits: 100000}, nil
}

func (c *recordingQuotaRPCClient) PreDeductQuota(_ context.Context, req *pb.PreDeductQuotaRequest, _ ...grpc.CallOption) (*pb.PreDeductQuotaResponse, error) {
	c.request = req
	return &pb.PreDeductQuotaResponse{Success: true, DeductionId: "deduction-1"}, nil
}

func (c *recordingQuotaRPCClient) CalculateDualCost(_ context.Context, req *pb.CalculateDualCostRequest, _ ...grpc.CallOption) (*pb.DualCostResponse, error) {
	c.dualCostRequest = req
	return &pb.DualCostResponse{Success: true, TotalCredits: 1}, nil
}

func TestQuotaClientPropagatesReservationPolicy(t *testing.T) {
	rpcClient := &recordingQuotaRPCClient{}
	client := &QuotaClient{client: rpcClient}

	_, err := client.PreDeductQuota(context.Background(), &PreDeductQuotaRequest{
		OrganizationID:    "organization-1",
		EstimatedCredits:  10,
		RequestID:         "request-1",
		AttemptID:         "attempt-1",
		ReservationPolicy: reservationPolicyAuthoritativeQuoteV1,
	})
	if err != nil {
		t.Fatalf("PreDeductQuota() error = %v", err)
	}
	if rpcClient.request == nil || rpcClient.request.GetReservationPolicy() != reservationPolicyAuthoritativeQuoteV1 {
		t.Fatalf("reservation policy request = %#v", rpcClient.request)
	}
}

func TestQuotaClientPropagatesPricingSnapshot(t *testing.T) {
	rpcClient := &recordingQuotaRPCClient{}
	client := &QuotaClient{client: rpcClient}

	_, err := client.PreDeductQuota(context.Background(), &PreDeductQuotaRequest{
		OrganizationID:   "organization-1",
		EstimatedCredits: 10,
		RequestID:        "request-1",
		AttemptID:        "attempt-1",
		PricingSnapshot:  `{"reservation_policy":"authoritative_quote_v1"}`,
	})
	if err != nil {
		t.Fatalf("PreDeductQuota() error = %v", err)
	}
	if rpcClient.request == nil || rpcClient.request.GetPricingSnapshot() != `{"reservation_policy":"authoritative_quote_v1"}` {
		t.Fatalf("pricing snapshot request = %#v", rpcClient.request)
	}
}

func TestQuotaClientPropagatesMeteredUsageIdentity(t *testing.T) {
	rpcClient := &recordingQuotaRPCClient{}
	client := &QuotaClient{client: rpcClient}

	resp, err := client.PreDeductMeteredUsage(context.Background(), &PreDeductMeteredUsageRequest{
		OrganizationID: "organization-1",
		ModelID:        "local-model-id",
		ProviderName:   "openai",
		ModelName:      "gpt-image-2",
		RequestID:      "request-1",
		AttemptID:      "attempt-1",
		ResourceType:   "image_generation",
		ResourceID:     "generation-1",
		ResourceName:   "Image generation",
		Usage: MeteredUsageFact{
			Operation:  "image_generation",
			Meter:      "image",
			BaseUnit:   "per_image",
			Quantity:   1,
			Dimensions: map[string]string{"size": "1024x1024", "quality": "standard"},
		},
	})
	if err != nil {
		t.Fatalf("PreDeductMeteredUsage() error = %v", err)
	}
	if resp == nil || !resp.Success || resp.DeductionID != "metered-deduction-1" || resp.EstimatedCredits != 100000 || resp.ReservedCredits != 100000 {
		t.Fatalf("PreDeductMeteredUsage() response = %#v", resp)
	}
	got := rpcClient.meteredRequest
	if got == nil {
		t.Fatal("metered request was not recorded")
	}
	if got.GetOrganizationId() != "organization-1" || got.GetModelId() != "local-model-id" ||
		got.GetProviderName() != "openai" || got.GetModelName() != "gpt-image-2" ||
		got.GetRequestId() != "request-1" || got.GetAttemptId() != "attempt-1" ||
		got.GetResourceType() != "image_generation" || got.GetResourceId() != "generation-1" || got.GetResourceName() != "Image generation" {
		t.Fatalf("metered request identity = %#v", got)
	}
	usage := got.GetUsage()
	if usage == nil || usage.GetOperation() != "image_generation" || usage.GetMeter() != "image" ||
		usage.GetBaseUnit() != "per_image" || usage.GetQuantity() != 1 {
		t.Fatalf("metered request usage = %#v", usage)
	}
	if usage.GetDimensions()["size"] != "1024x1024" || usage.GetDimensions()["quality"] != "standard" {
		t.Fatalf("metered request dimensions = %#v", usage.GetDimensions())
	}
}

func TestQuotaClientPropagatesCanonicalModelIdentity(t *testing.T) {
	rpcClient := &recordingQuotaRPCClient{}
	client := &QuotaClient{client: rpcClient}

	_, err := client.CalculateDualCost(context.Background(), "local-model-id", "openai", "gpt-4o", 10, 2)
	if err != nil {
		t.Fatalf("CalculateDualCost() error = %v", err)
	}
	if rpcClient.dualCostRequest == nil {
		t.Fatal("CalculateDualCost() did not send an RPC request")
	}
	if rpcClient.dualCostRequest.GetModelId() != "local-model-id" || rpcClient.dualCostRequest.GetProvider() != "openai" || rpcClient.dualCostRequest.GetModel() != "gpt-4o" {
		t.Fatalf("model identity request = %#v, want local ID plus openai/gpt-4o", rpcClient.dualCostRequest)
	}
}
