package billing

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"
	pb "github.com/zgiai/zgi/api/pkg/rpc/v1"
	"google.golang.org/grpc"
)

func TestRemotePreDeductPropagatesAttemptAndResourceIdentity(t *testing.T) {
	client := &recordingBillingServiceClient{
		preDeductResponse: &pb.PreDeductQuotaResponse{Success: true, ErrorCode: "TEST_CODE", DeductionId: "deduction-1"},
	}
	remote := &Remote{client: client}

	response, err := remote.PreDeduct(context.Background(), &PreDeductRequest{
		OrganizationID:   "organization-1",
		EstimatedCredits: 30,
		RequestID:        "request-1",
		AttemptID:        "attempt-1",
		ResourceType:     "market_api",
		ResourceID:       "api-1",
		ResourceName:     "Company Search",
	})

	require.NoError(t, err)
	require.True(t, response.Allowed)
	require.Equal(t, "TEST_CODE", response.ErrorCode)
	require.Equal(t, "attempt-1", client.preDeductRequest.AttemptId)
	require.Equal(t, "market_api", client.preDeductRequest.ResourceType)
	require.Equal(t, "api-1", client.preDeductRequest.ResourceId)
	require.Equal(t, "Company Search", client.preDeductRequest.ResourceName)
}

func TestRemoteSettlePropagatesIdentityAndReturnsBusinessFailure(t *testing.T) {
	client := &recordingBillingServiceClient{
		settleResponse: &pb.SettleQuotaResponse{Success: false, ErrorMessage: "resource mismatch"},
	}
	remote := &Remote{client: client}

	err := remote.Settle(context.Background(), &SettleRequest{
		OrganizationID: "organization-1",
		DeductionID:    "deduction-1",
		ActualCredits:  30,
		RequestID:      "request-1",
		AttemptID:      "attempt-1",
		ResourceType:   "market_api",
		ResourceID:     "api-1",
		ResourceName:   "Company Search",
	})

	require.EqualError(t, err, "billing settlement failed: resource mismatch")
	require.Equal(t, "attempt-1", client.settleRequest.AttemptId)
	require.Equal(t, "market_api", client.settleRequest.ResourceType)
	require.Equal(t, "api-1", client.settleRequest.ResourceId)
	require.Equal(t, "Company Search", client.settleRequest.ResourceName)
}

func TestRemoteRejectsNilBillingRequests(t *testing.T) {
	remote := &Remote{client: &recordingBillingServiceClient{}}

	_, err := remote.PreDeduct(context.Background(), nil)
	require.EqualError(t, err, "billing pre-deduct request is required")
	require.EqualError(t, remote.Settle(context.Background(), nil), "billing settle request is required")
}

type recordingBillingServiceClient struct {
	pb.BillingServiceClient
	preDeductRequest  *pb.PreDeductQuotaRequest
	preDeductResponse *pb.PreDeductQuotaResponse
	settleRequest     *pb.SettleQuotaRequest
	settleResponse    *pb.SettleQuotaResponse
}

func (c *recordingBillingServiceClient) PreDeductQuota(_ context.Context, req *pb.PreDeductQuotaRequest, _ ...grpc.CallOption) (*pb.PreDeductQuotaResponse, error) {
	c.preDeductRequest = req
	return c.preDeductResponse, nil
}

func (c *recordingBillingServiceClient) SettleQuota(_ context.Context, req *pb.SettleQuotaRequest, _ ...grpc.CallOption) (*pb.SettleQuotaResponse, error) {
	c.settleRequest = req
	return c.settleResponse, nil
}
