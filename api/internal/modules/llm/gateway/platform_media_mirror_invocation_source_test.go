package gateway

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"
	apikeymodel "github.com/zgiai/zgi/api/internal/modules/llm/apikey/model"
	adapter "github.com/zgiai/zgi/api/internal/modules/llm/protocol/adapters"
)

type fakeMediaMirrorBillingProvider struct {
	fakeBillingProvider
	mirrorCalls int
	lastBC      *BillingContext
}

func (f *fakeMediaMirrorBillingProvider) MirrorPlatformMediaSettlement(_ context.Context, bc *BillingContext, _ *adapter.SettlementResult) error {
	f.mirrorCalls++
	f.lastBC = bc
	return nil
}

// A media call must persist a stable invocation source: product surfaces
// declare it on the context, while a public Gateway call resolves to "api".
// Before this was wired the media mirror wrote an empty source that normalized
// to "unknown", so Pre's statistics showed media calls as "来源未识别".
func TestMirrorPlatformMediaSettlementRecordsInvocationSource(t *testing.T) {
	cases := []struct {
		name string
		ctx  context.Context
		want InvocationSource
	}{
		{name: "product media call", ctx: WithInvocationSource(context.Background(), InvocationSourceProduct), want: InvocationSourceProduct},
		{name: "public gateway media call", ctx: context.Background(), want: InvocationSourceAPI},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			billing := &fakeMediaMirrorBillingProvider{}
			s := &llmGatewayServiceImpl{billing: billing}
			err := s.mirrorPlatformMediaSettlement(
				tc.ctx,
				&apikeymodel.TenantAPIKey{ID: "key-1"},
				&ProviderSelection{UseSystemProvider: true},
				uuid.New(),
				"req-1",
				"req-1",
				PricingOperationMusic,
				true,
				time.Now(),
				&adapter.SettlementResult{SettlementID: "deduction-1"},
			)
			if err != nil {
				t.Fatalf("mirrorPlatformMediaSettlement() error = %v", err)
			}
			if billing.mirrorCalls != 1 || billing.lastBC == nil {
				t.Fatalf("mirror calls = %d, bc = %v", billing.mirrorCalls, billing.lastBC)
			}
			if got := billing.lastBC.InvocationSource; got != tc.want {
				t.Fatalf("InvocationSource = %q, want %q", got, tc.want)
			}
		})
	}
}
