package gateway

import (
	"context"
	"testing"

	pb "github.com/zgiai/zgi/api/pkg/rpc/v1"
	"google.golang.org/grpc"
)

type recordingQuotaRPCClient struct {
	pb.BillingServiceClient
	request *pb.PreDeductQuotaRequest
}

func (c *recordingQuotaRPCClient) PreDeductQuota(_ context.Context, req *pb.PreDeductQuotaRequest, _ ...grpc.CallOption) (*pb.PreDeductQuotaResponse, error) {
	c.request = req
	return &pb.PreDeductQuotaResponse{Success: true, DeductionId: "deduction-1"}, nil
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
