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
	dualCostRequest *pb.CalculateDualCostRequest
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
