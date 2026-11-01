package gateway

import (
	"context"
	"net/http"
	"testing"
	"time"

	"github.com/zgiai/zgi/api/internal/modules/llm/gateway/requesttimeout"
	pb "github.com/zgiai/zgi/api/pkg/rpc/v1"
	"google.golang.org/grpc"
	"google.golang.org/grpc/metadata"
)

// The Console hop carries the header only when the caller sent one, so Console
// otherwise applies its own default and channel override.
func TestPlatformProxyHeadersForwardOnlyRequestedTimeout(t *testing.T) {
	requested, _ := http.NewRequestWithContext(
		requesttimeout.WithTimeout(context.Background(), 2400*time.Second),
		http.MethodPost, "http://console.internal/v1/internal/chat/completions", nil)
	applyPlatformProxyHeaders(requested)
	if got := requested.Header.Get(requesttimeout.HeaderName); got != "2400" {
		t.Fatalf("%s = %q, want 2400", requesttimeout.HeaderName, got)
	}

	plain, _ := http.NewRequestWithContext(context.Background(),
		http.MethodPost, "http://console.internal/v1/internal/chat/completions", nil)
	applyPlatformProxyHeaders(plain)
	if got := plain.Header.Get(requesttimeout.HeaderName); got != "" {
		t.Fatalf("%s = %q without a caller header", requesttimeout.HeaderName, got)
	}
}

type metadataRecordingQuotaRPCClient struct {
	pb.BillingServiceClient
	quotaMetadata   metadata.MD
	meteredMetadata metadata.MD
}

func (c *metadataRecordingQuotaRPCClient) PreDeductQuota(ctx context.Context, _ *pb.PreDeductQuotaRequest, _ ...grpc.CallOption) (*pb.PreDeductQuotaResponse, error) {
	c.quotaMetadata, _ = metadata.FromOutgoingContext(ctx)
	return &pb.PreDeductQuotaResponse{Success: true, DeductionId: "deduction-1"}, nil
}

func (c *metadataRecordingQuotaRPCClient) PreDeductMeteredUsage(ctx context.Context, _ *pb.PreDeductMeteredUsageRequest, _ ...grpc.CallOption) (*pb.PreDeductMeteredUsageResponse, error) {
	c.meteredMetadata, _ = metadata.FromOutgoingContext(ctx)
	return &pb.PreDeductMeteredUsageResponse{Success: true, DeductionId: "metered-deduction-1"}, nil
}

// Holds always carry the effective duration, so Console keeps them open past
// the old 10-minute TTL for long requests with or without the header.
func TestQuotaClientSendsRequestTimeoutMetadataOnHolds(t *testing.T) {
	cases := []struct {
		name string
		ctx  context.Context
		want string
	}{
		{"requested", requesttimeout.WithTimeout(context.Background(), 1800*time.Second), "1800"},
		{"default", context.Background(), "1200"},
	}
	for _, tc := range cases {
		rpcClient := &metadataRecordingQuotaRPCClient{}
		client := &QuotaClient{client: rpcClient}
		if _, err := client.PreDeductQuota(tc.ctx, &PreDeductQuotaRequest{OrganizationID: "organization-1", RequestID: "request-1"}); err != nil {
			t.Fatalf("%s: PreDeductQuota() error = %v", tc.name, err)
		}
		if _, err := client.PreDeductMeteredUsage(tc.ctx, &PreDeductMeteredUsageRequest{OrganizationID: "organization-1", RequestID: "request-1"}); err != nil {
			t.Fatalf("%s: PreDeductMeteredUsage() error = %v", tc.name, err)
		}
		for call, md := range map[string]metadata.MD{"PreDeductQuota": rpcClient.quotaMetadata, "PreDeductMeteredUsage": rpcClient.meteredMetadata} {
			if got := md.Get(requesttimeout.MetadataKey); len(got) != 1 || got[0] != tc.want {
				t.Fatalf("%s: %s metadata = %v, want %s", tc.name, call, md, tc.want)
			}
		}
	}
}
