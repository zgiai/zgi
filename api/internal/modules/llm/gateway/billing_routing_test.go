package gateway

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/shopspring/decimal"
	platformconsole "github.com/zgiai/zgi/api/internal/infra/platform/console"
	apikeymodel "github.com/zgiai/zgi/api/internal/modules/llm/apikey/model"
	llmerrors "github.com/zgiai/zgi/api/internal/modules/llm/errors"
	llmmodel "github.com/zgiai/zgi/api/internal/modules/llm/llmmodel/model"
	adapter "github.com/zgiai/zgi/api/internal/modules/llm/protocol/adapters"
	providermodel "github.com/zgiai/zgi/api/internal/modules/llm/provider/model"
)

type fakeBillingProvider struct {
	preDeductCalls    int
	settleCalls       int
	checkBalanceCalls int

	lastPreDeduct *BillingContext
	lastSettle    *BillingContext
	lastSettleErr error

	preDeductErr       error
	settleErr          error
	checkBalanceResult bool
	checkBalanceErr    error
}

type fakeProxySettlementBillingProvider struct {
	fakeBillingProvider
	lastProxySettle   *BillingContext
	lastProxyResponse *adapter.SettlementResult
	lastProxyErr      error
	proxyErr          error
}

func (f *fakeProxySettlementBillingProvider) FinalizePlatformProxySettlement(ctx context.Context, bc *BillingContext, settlement *adapter.SettlementResult) error {
	f.lastProxySettle = bc
	f.lastProxyResponse = settlement
	f.lastProxyErr = ctx.Err()
	return f.proxyErr
}

func (f *fakeBillingProvider) PreDeduct(ctx context.Context, bc *BillingContext) error {
	f.preDeductCalls++
	f.lastPreDeduct = bc
	return f.preDeductErr
}

func (f *fakeBillingProvider) Settle(ctx context.Context, bc *BillingContext) error {
	f.settleCalls++
	f.lastSettle = bc
	f.lastSettleErr = ctx.Err()
	return f.settleErr
}

func (f *fakeBillingProvider) CalculateCreditsFromTokens(promptTokens, completionTokens int, modelID uuid.UUID) (int64, int64, int64, error) {
	// Not needed for these routing tests.
	return 0, 0, 0, nil
}

func (f *fakeBillingProvider) CalculateImageCredits(req *adapter.ImageRequest, modelID uuid.UUID) (int64, error) {
	return 0, nil
}

func (f *fakeBillingProvider) CheckBalance(ctx context.Context, groupID uuid.UUID, ownerID uuid.UUID, estimatedCredits int64) (bool, error) {
	f.checkBalanceCalls++
	return f.checkBalanceResult, f.checkBalanceErr
}

func (f *fakeBillingProvider) CheckPrivateChannelBalance(ctx context.Context, organizationID uuid.UUID, channelID uuid.UUID, estimatedCredits int64) (bool, error) {
	return f.checkBalanceResult, f.checkBalanceErr
}

type fakePricingEngine struct {
	tokenQuote PricingQuote
	imageQuote PricingQuote
	tokenErr   error
	imageErr   error
	lastModel  PricingModelRef
}

func (f *fakePricingEngine) QuoteTokens(ctx context.Context, model PricingModelRef, promptTokens, completionTokens int) (PricingQuote, error) {
	f.lastModel = model
	if f.tokenErr != nil {
		return PricingQuote{}, f.tokenErr
	}
	return f.tokenQuote, nil
}

func (f *fakePricingEngine) QuoteImage(ctx context.Context, model PricingModelRef, req *adapter.ImageRequest) (PricingQuote, error) {
	f.lastModel = model
	if f.imageErr != nil {
		return PricingQuote{}, f.imageErr
	}
	return f.imageQuote, nil
}

func TestBillingRouting_CheckBalanceAndPreDeduct_Private_UsesLocalBilling(t *testing.T) {
	remote := &fakeBillingProvider{checkBalanceResult: true}
	local := &fakeBillingProvider{checkBalanceResult: true}

	s := &llmGatewayServiceImpl{
		billing:      remote,
		localBilling: local,
	}

	ps := &ProviderSelection{UseSystemProvider: false}
	channelID := uuid.New()
	bc := &BillingContext{APIKeyID: "k1", UseSystemProvider: false, ChannelID: &channelID}

	err := s.checkBalanceAndPreDeduct(context.Background(), ps, uuid.New(), uuid.New(), 123, bc)
	if err != nil {
		t.Fatalf("checkBalanceAndPreDeduct returned error: %v", err)
	}

	if local.preDeductCalls != 1 {
		t.Fatalf("local PreDeduct calls = %d, want 1", local.preDeductCalls)
	}
	if remote.preDeductCalls != 0 {
		t.Fatalf("remote PreDeduct calls = %d, want 0", remote.preDeductCalls)
	}
	if remote.checkBalanceCalls != 0 {
		t.Fatalf("remote CheckBalance calls = %d, want 0", remote.checkBalanceCalls)
	}
}

func TestBillingRouting_CheckBalanceAndPreDeduct_Private_InsufficientChannelBalance(t *testing.T) {
	remote := &fakeBillingProvider{checkBalanceResult: true}
	local := &fakeBillingProvider{checkBalanceResult: false}

	s := &llmGatewayServiceImpl{
		billing:      remote,
		localBilling: local,
	}

	ps := &ProviderSelection{UseSystemProvider: false}
	channelID := uuid.New()
	bc := &BillingContext{APIKeyID: "k1", UseSystemProvider: false, ChannelID: &channelID}

	err := s.checkBalanceAndPreDeduct(context.Background(), ps, uuid.New(), uuid.New(), 123, bc)
	if !errors.Is(err, ErrInsufficientBalance) {
		t.Fatalf("checkBalanceAndPreDeduct err = %v, want %v", err, ErrInsufficientBalance)
	}

	if local.preDeductCalls != 0 {
		t.Fatalf("local PreDeduct calls = %d, want 0 when private channel funds are insufficient", local.preDeductCalls)
	}
	if remote.preDeductCalls != 0 {
		t.Fatalf("remote PreDeduct calls = %d, want 0", remote.preDeductCalls)
	}
}

func TestBillingRouting_CheckBalanceAndPreDeduct_Official_UsesRemoteBilling(t *testing.T) {
	remote := &fakeBillingProvider{checkBalanceResult: true}
	local := &fakeBillingProvider{checkBalanceResult: true}

	s := &llmGatewayServiceImpl{
		billing:      remote,
		localBilling: local,
	}

	ps := &ProviderSelection{UseSystemProvider: true}
	channelID := uuid.New()
	bc := &BillingContext{APIKeyID: "k1", UseSystemProvider: true, ChannelID: &channelID}

	err := s.checkBalanceAndPreDeduct(context.Background(), ps, uuid.New(), uuid.New(), 123, bc)
	if err != nil {
		t.Fatalf("checkBalanceAndPreDeduct returned error: %v", err)
	}

	if remote.checkBalanceCalls != 1 {
		t.Fatalf("remote CheckBalance calls = %d, want 1", remote.checkBalanceCalls)
	}
	if remote.preDeductCalls != 1 {
		t.Fatalf("remote PreDeduct calls = %d, want 1", remote.preDeductCalls)
	}
	if local.preDeductCalls != 0 {
		t.Fatalf("local PreDeduct calls = %d, want 0", local.preDeductCalls)
	}
}

func TestBillingRouting_CheckBalanceAndPreDeduct_PlatformLaneWinsOverLegacyFlag(t *testing.T) {
	remote := &fakeBillingProvider{checkBalanceResult: true}
	local := &fakeBillingProvider{checkBalanceResult: true}

	s := &llmGatewayServiceImpl{
		billing:      remote,
		localBilling: local,
	}

	ps := &ProviderSelection{BillingLane: UsageBillingLanePlatform, UseSystemProvider: false}
	channelID := uuid.New()
	bc := &BillingContext{
		APIKeyID:          "k1",
		BillingLane:       UsageBillingLanePlatform,
		UseSystemProvider: false,
		ChannelID:         &channelID,
	}

	err := s.checkBalanceAndPreDeduct(context.Background(), ps, uuid.New(), uuid.New(), 123, bc)
	if err != nil {
		t.Fatalf("checkBalanceAndPreDeduct returned error: %v", err)
	}

	if remote.preDeductCalls != 1 {
		t.Fatalf("remote PreDeduct calls = %d, want 1", remote.preDeductCalls)
	}
	if local.preDeductCalls != 0 {
		t.Fatalf("local PreDeduct calls = %d, want 0", local.preDeductCalls)
	}
}

func TestBillingRouting_RollbackPreDeduction_Private_UsesLocalBilling(t *testing.T) {
	remote := &fakeBillingProvider{checkBalanceResult: true}
	local := &fakeBillingProvider{checkBalanceResult: true}

	s := &llmGatewayServiceImpl{
		billing:      remote,
		localBilling: local,
	}

	bc := &BillingContext{UseSystemProvider: false}
	if err := s.rollbackPreDeduction(context.Background(), bc); err != nil {
		t.Fatalf("rollbackPreDeduction returned error: %v", err)
	}

	if local.settleCalls != 1 {
		t.Fatalf("local Settle calls = %d, want 1", local.settleCalls)
	}
	if remote.settleCalls != 0 {
		t.Fatalf("remote Settle calls = %d, want 0", remote.settleCalls)
	}
}

func TestBillingRouting_RollbackPreDeduction_Official_UsesRemoteBilling(t *testing.T) {
	remote := &fakeBillingProvider{checkBalanceResult: true}
	local := &fakeBillingProvider{checkBalanceResult: true}

	s := &llmGatewayServiceImpl{
		billing:      remote,
		localBilling: local,
	}

	bc := &BillingContext{UseSystemProvider: true}
	if err := s.rollbackPreDeduction(context.Background(), bc); err != nil {
		t.Fatalf("rollbackPreDeduction returned error: %v", err)
	}

	if remote.settleCalls != 1 {
		t.Fatalf("remote Settle calls = %d, want 1", remote.settleCalls)
	}
	if local.settleCalls != 0 {
		t.Fatalf("local Settle calls = %d, want 0", local.settleCalls)
	}
}

func TestBillingRouting_RollbackPreDeduction_UsesDetachedBillingContext(t *testing.T) {
	remote := &fakeBillingProvider{checkBalanceResult: true}
	local := &fakeBillingProvider{checkBalanceResult: true}

	s := &llmGatewayServiceImpl{
		billing:      remote,
		localBilling: local,
	}

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	bc := &BillingContext{UseSystemProvider: true}
	if err := s.rollbackPreDeduction(ctx, bc); err != nil {
		t.Fatalf("rollbackPreDeduction returned error: %v", err)
	}
	if remote.settleCalls != 1 {
		t.Fatalf("remote Settle calls = %d, want 1", remote.settleCalls)
	}
	if remote.lastSettleErr != nil {
		t.Fatalf("settle context err = %v, want nil", remote.lastSettleErr)
	}
}

func TestBillingRouting_RollbackPreDeduction_Private_SettleFailure_ReturnsBillingSettleFailed(t *testing.T) {
	remote := &fakeBillingProvider{checkBalanceResult: true}
	local := &fakeBillingProvider{
		checkBalanceResult: true,
		settleErr:          errors.New("local settle failed"),
	}

	s := &llmGatewayServiceImpl{
		billing:      remote,
		localBilling: local,
	}

	bc := &BillingContext{
		UseSystemProvider: false,
		RequestID:         "req-refund-failed",
		AttemptID:         "req-refund-failed-a1",
	}
	err := s.rollbackPreDeduction(context.Background(), bc)
	if err == nil {
		t.Fatalf("expected rollbackPreDeduction to return error")
	}
	if !errors.Is(err, ErrBillingSettleFailed) {
		t.Fatalf("expected ErrBillingSettleFailed, got: %v", err)
	}
}

func TestBillingRouting_RollbackPreDeduction_CloudOfficialLaneMismatch(t *testing.T) {
	local := &fakeBillingProvider{checkBalanceResult: true}
	s := &llmGatewayServiceImpl{
		billing:         local,
		localBilling:    local,
		consoleProvider: platformconsole.NewRemote("http://localhost:2625", ""),
	}

	bc := &BillingContext{
		UseSystemProvider: true,
		RequestID:         "req-rollback-lane",
	}

	err := s.rollbackPreDeduction(context.Background(), bc)
	if err == nil {
		t.Fatalf("expected rollbackPreDeduction to return error")
	}
	if !errors.Is(err, llmerrors.DomainErrBillingFailed) {
		t.Fatalf("expected DomainErrBillingFailed, got: %v", err)
	}
	if !errors.Is(err, ErrBillingLaneMismatch) {
		t.Fatalf("expected ErrBillingLaneMismatch, got: %v", err)
	}
}

func TestBillingRouting_HandleProviderError_UsesDetachedBillingContext(t *testing.T) {
	remote := &fakeBillingProvider{checkBalanceResult: true}
	local := &fakeBillingProvider{checkBalanceResult: true}
	s := &llmGatewayServiceImpl{
		billing:       remote,
		localBilling:  local,
		healthTracker: NewChannelHealthTracker(nil),
	}

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	ps := &ProviderSelection{
		UseSystemProvider: true,
		Provider: providermodel.LLMProvider{
			Provider: "openai",
		},
		Model: llmmodel.LLMModel{
			Model: "gpt-5",
		},
	}
	bc := &BillingContext{
		RequestID:         "req-provider-error-detached",
		AttemptID:         "req-provider-error-detached-a1",
		UseSystemProvider: true,
	}

	err := s.handleProviderError(ctx, bc, ps, nil, 100, 0, context.DeadlineExceeded)
	if err != nil {
		t.Fatalf("handleProviderError returned error: %v", err)
	}
	if remote.settleCalls != 1 {
		t.Fatalf("remote Settle calls = %d, want 1", remote.settleCalls)
	}
	if remote.lastSettleErr != nil {
		t.Fatalf("settle context err = %v, want nil", remote.lastSettleErr)
	}
}

func TestBillingRouting_HandleStreamBilling_Private_UsesLocalBilling(t *testing.T) {
	remote := &fakeBillingProvider{checkBalanceResult: true}
	local := &fakeBillingProvider{checkBalanceResult: true}

	s := &llmGatewayServiceImpl{
		billing:       remote,
		localBilling:  local,
		healthTracker: NewChannelHealthTracker(nil),
	}

	in := make(chan adapter.StreamResponse, 1)
	out := make(chan adapter.StreamResponse, 1)

	// Force error path to avoid depending on model pricing calculations.
	in <- adapter.StreamResponse{Done: true, Error: errors.New("boom")}
	close(in)

	bc := &BillingContext{UseSystemProvider: false}
	s.handleStreamBilling(context.Background(), in, out, bc, nil, nil, time.Now(), nil)

	if local.settleCalls != 1 {
		t.Fatalf("local Settle calls = %d, want 1", local.settleCalls)
	}
	if remote.settleCalls != 0 {
		t.Fatalf("remote Settle calls = %d, want 0", remote.settleCalls)
	}
	if local.lastSettle == nil {
		t.Fatalf("expected local settle context to be captured")
	}
	if local.lastSettle.ActualCredits != 0 || local.lastSettle.PromptTokens != 0 || local.lastSettle.CompletionTokens != 0 || local.lastSettle.TotalTokens != 0 {
		t.Fatalf("expected stream error settle token usage to be zeroed, got actual=%d prompt=%d completion=%d total=%d",
			local.lastSettle.ActualCredits,
			local.lastSettle.PromptTokens,
			local.lastSettle.CompletionTokens,
			local.lastSettle.TotalTokens,
		)
	}
}

func TestBillingRouting_HandleProviderError_ZeroesTokenUsageBeforeSettle(t *testing.T) {
	remote := &fakeBillingProvider{checkBalanceResult: true}
	local := &fakeBillingProvider{checkBalanceResult: true}

	s := &llmGatewayServiceImpl{
		billing:       remote,
		localBilling:  local,
		healthTracker: NewChannelHealthTracker(nil),
	}

	ps := &ProviderSelection{UseSystemProvider: false}
	bc := &BillingContext{
		UseSystemProvider: false,
		PromptTokens:      111,
		CompletionTokens:  222,
		TotalTokens:       333,
		ActualCredits:     444,
	}

	err := s.handleProviderError(
		context.Background(),
		bc,
		ps,
		nil,
		15,
		0,
		errors.New("upstream failed"),
	)
	if err != nil {
		t.Fatalf("handleProviderError returned error: %v", err)
	}

	if local.settleCalls != 1 {
		t.Fatalf("local Settle calls = %d, want 1", local.settleCalls)
	}
	if local.lastSettle == nil {
		t.Fatalf("expected local settle context to be captured")
	}
	if local.lastSettle.ActualCredits != 0 || local.lastSettle.PromptTokens != 0 || local.lastSettle.CompletionTokens != 0 || local.lastSettle.TotalTokens != 0 {
		t.Fatalf("expected provider error settle token usage to be zeroed, got actual=%d prompt=%d completion=%d total=%d",
			local.lastSettle.ActualCredits,
			local.lastSettle.PromptTokens,
			local.lastSettle.CompletionTokens,
			local.lastSettle.TotalTokens,
		)
	}
}

func TestBillingRouting_HandleProviderError_SettleFailure_ReturnsBillingSettleFailed(t *testing.T) {
	remote := &fakeBillingProvider{checkBalanceResult: true}
	local := &fakeBillingProvider{
		checkBalanceResult: true,
		settleErr:          errors.New("local settle failed"),
	}

	s := &llmGatewayServiceImpl{
		billing:       remote,
		localBilling:  local,
		healthTracker: NewChannelHealthTracker(nil),
	}

	ps := &ProviderSelection{UseSystemProvider: false}
	bc := &BillingContext{
		UseSystemProvider: false,
		RequestID:         "req-provider-failed",
		AttemptID:         "req-provider-failed-a1",
	}

	err := s.handleProviderError(
		context.Background(),
		bc,
		ps,
		nil,
		15,
		0,
		errors.New("upstream failed"),
	)
	if err == nil {
		t.Fatalf("expected handleProviderError to return error")
	}
	if !errors.Is(err, ErrBillingSettleFailed) {
		t.Fatalf("expected ErrBillingSettleFailed, got: %v", err)
	}
}

func TestBillingRouting_CheckBalanceAndPreDeduct_CheckBalanceError_WrappedAsBillingPreDeductFailed(t *testing.T) {
	remote := &fakeBillingProvider{
		checkBalanceResult: false,
		checkBalanceErr:    errors.New("grpc unavailable"),
	}
	local := &fakeBillingProvider{checkBalanceResult: true}

	s := &llmGatewayServiceImpl{
		billing:      remote,
		localBilling: local,
	}

	ps := &ProviderSelection{UseSystemProvider: true}
	bc := &BillingContext{APIKeyID: "k1", UseSystemProvider: true, RequestID: "req-1"}

	err := s.checkBalanceAndPreDeduct(context.Background(), ps, uuid.New(), uuid.New(), 123, bc)
	if err == nil {
		t.Fatalf("expected checkBalanceAndPreDeduct to return error")
	}
	if !errors.Is(err, ErrBillingPreDeductFailed) {
		t.Fatalf("expected ErrBillingPreDeductFailed, got: %v", err)
	}
	if !errors.Is(err, llmerrors.DomainErrBillingFailed) {
		t.Fatalf("expected DomainErrBillingFailed, got: %v", err)
	}
}

func TestBillingRouting_SettleChatSuccess_ReturnsBillingDomainError(t *testing.T) {
	remote := &fakeBillingProvider{
		checkBalanceResult: true,
		settleErr:          errors.New("grpc settle unavailable"),
	}
	local := &fakeBillingProvider{checkBalanceResult: true}
	engine := &fakePricingEngine{
		tokenQuote: PricingQuote{
			TotalCredits: 30,
		},
	}

	s := &llmGatewayServiceImpl{
		billing:       remote,
		localBilling:  local,
		pricingEngine: engine,
		healthTracker: NewChannelHealthTracker(nil),
	}

	routeID := uuid.New()
	ps := &ProviderSelection{
		UseSystemProvider: true,
		RouteID:           routeID,
		Model: llmmodel.LLMModel{
			ID:    uuid.New(),
			Model: "gpt-4o-mini",
		},
		Provider: providermodel.LLMProvider{
			Provider: "openai",
		},
	}
	bc := &BillingContext{
		RequestID:         "req-1",
		UseSystemProvider: true,
	}

	err := s.settleChatSuccess(
		context.Background(),
		bc,
		ps,
		nil,
		&adapter.Usage{
			PromptTokens:     10,
			CompletionTokens: 20,
			TotalTokens:      30,
		},
		nil,
		10,
	)
	if err == nil {
		t.Fatalf("expected settleChatSuccess to return error")
	}
	if !errors.Is(err, ErrBillingSettleFailed) {
		t.Fatalf("expected ErrBillingSettleFailed, got: %v", err)
	}
	if !errors.Is(err, llmerrors.DomainErrBillingFailed) {
		t.Fatalf("expected DomainErrBillingFailed, got: %v", err)
	}
}

func TestBillingRouting_SettleChatSuccess_UsesPricingEngineQuote(t *testing.T) {
	remote := &fakeBillingProvider{checkBalanceResult: true}
	local := &fakeBillingProvider{checkBalanceResult: true}
	engine := &fakePricingEngine{
		tokenQuote: PricingQuote{
			InputUSD:      decimal.RequireFromString("0.012"),
			OutputUSD:     decimal.RequireFromString("0.006"),
			TotalUSD:      decimal.RequireFromString("0.018"),
			InputCredits:  12,
			OutputCredits: 6,
			TotalCredits:  18,
			Source:        PricingSourceUSDPrice,
		},
	}

	s := &llmGatewayServiceImpl{
		billing:       remote,
		localBilling:  local,
		pricingEngine: engine,
		healthTracker: NewChannelHealthTracker(nil),
	}

	ps := &ProviderSelection{
		UseSystemProvider: false,
		ModelSource:       PricingModelSourceGlobal,
		Model: llmmodel.LLMModel{
			ID:    uuid.New(),
			Model: "gpt-4o-mini",
		},
		Provider: providermodel.LLMProvider{
			Provider: "openai",
		},
	}
	bc := &BillingContext{
		RequestID:         "req-pricing-engine",
		UseSystemProvider: false,
	}

	err := s.settleChatSuccess(
		context.Background(),
		bc,
		ps,
		nil,
		&adapter.Usage{
			PromptTokens:     10,
			CompletionTokens: 20,
			TotalTokens:      30,
		},
		nil,
		10,
	)
	if err != nil {
		t.Fatalf("settleChatSuccess returned error: %v", err)
	}
	if local.lastSettle == nil {
		t.Fatalf("expected settle context to be captured")
	}
	if engine.lastModel.Source != PricingModelSourceGlobal {
		t.Fatalf("pricing model source = %s, want %s", engine.lastModel.Source, PricingModelSourceGlobal)
	}
	if local.lastSettle.ActualCredits != 18 {
		t.Fatalf("actualCredits = %d, want 18", local.lastSettle.ActualCredits)
	}
	if !local.lastSettle.InputCost.Equal(decimal.NewFromInt(12)) {
		t.Fatalf("inputCost = %s, want 12", local.lastSettle.InputCost)
	}
	if !local.lastSettle.OutputCost.Equal(decimal.NewFromInt(6)) {
		t.Fatalf("outputCost = %s, want 6", local.lastSettle.OutputCost)
	}
	if !local.lastSettle.TotalCost.Equal(decimal.NewFromInt(18)) {
		t.Fatalf("totalCost = %s, want 18", local.lastSettle.TotalCost)
	}
}

func TestBillingRouting_SettleChatSuccess_PlatformUsesProxySettlement(t *testing.T) {
	remote := &fakeProxySettlementBillingProvider{}
	local := &fakeBillingProvider{checkBalanceResult: true}
	engine := &fakePricingEngine{
		tokenQuote: PricingQuote{TotalCredits: 999},
	}
	s := &llmGatewayServiceImpl{
		billing:       remote,
		localBilling:  local,
		pricingEngine: engine,
		healthTracker: NewChannelHealthTracker(nil),
	}

	ps := &ProviderSelection{
		UseSystemProvider: true,
		BillingLane:       UsageBillingLanePlatform,
		ModelSource:       PricingModelSourceGlobal,
		Model: llmmodel.LLMModel{
			ID:    uuid.New(),
			Model: "gpt-4o-mini",
		},
		Provider: providermodel.LLMProvider{
			Provider: "openai",
		},
	}
	bc := &BillingContext{
		RequestID:         "req-proxy-settle",
		AttemptID:         "req-proxy-settle-a1",
		DeductionID:       "deduction-proxy-settle",
		UseSystemProvider: true,
		BillingLane:       UsageBillingLanePlatform,
	}
	settlement := &adapter.SettlementResult{
		SettlementID:     "deduction-proxy-settle",
		OfficialPoints:   7,
		RemainingBalance: 93,
		Status:           "settled",
	}

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	err := s.settleChatSuccess(
		ctx,
		bc,
		ps,
		nil,
		&adapter.Usage{
			PromptTokens:     10,
			CompletionTokens: 20,
			TotalTokens:      30,
		},
		settlement,
		10,
	)
	if err != nil {
		t.Fatalf("settleChatSuccess returned error: %v", err)
	}
	if remote.lastProxySettle == nil {
		t.Fatalf("expected proxy settlement finalizer to be called")
	}
	if remote.lastProxySettle.ActualCredits != 0 {
		t.Fatalf("actual credits should be assigned by finalizer, got %d", remote.lastProxySettle.ActualCredits)
	}
	if remote.lastProxyResponse != settlement {
		t.Fatalf("settlement response was not passed to finalizer")
	}
	if remote.lastProxyErr != nil {
		t.Fatalf("proxy settlement context err = %v, want nil", remote.lastProxyErr)
	}
	if engine.lastModel.ModelID != uuid.Nil {
		t.Fatalf("pricing engine should not be used for platform proxy settlement")
	}
	if remote.settleCalls != 0 {
		t.Fatalf("legacy settle calls = %d, want 0", remote.settleCalls)
	}
}

func TestBillingRouting_SettleImageSuccess_PlatformUsesProxySettlement(t *testing.T) {
	remote := &fakeProxySettlementBillingProvider{}
	local := &fakeBillingProvider{checkBalanceResult: true}
	s := &llmGatewayServiceImpl{
		billing:       remote,
		localBilling:  local,
		healthTracker: NewChannelHealthTracker(nil),
	}

	ps := &ProviderSelection{
		UseSystemProvider: true,
		BillingLane:       UsageBillingLanePlatform,
		ModelSource:       PricingModelSourceGlobal,
		Model: llmmodel.LLMModel{
			ID:    uuid.New(),
			Model: "qwen-image",
		},
		Provider: providermodel.LLMProvider{
			Provider: "qwen",
		},
	}
	bc := &BillingContext{
		RequestID:         "req-image-proxy-settle",
		AttemptID:         "req-image-proxy-settle-a1",
		DeductionID:       "deduction-image-proxy-settle",
		UseSystemProvider: true,
		BillingLane:       UsageBillingLanePlatform,
	}
	settlement := &adapter.SettlementResult{
		SettlementID:     "deduction-image-proxy-settle",
		OfficialPoints:   11,
		RemainingBalance: 89,
		Status:           "settled",
	}

	err := s.settleImageSuccess(
		context.Background(),
		bc,
		ps,
		PricingQuote{TotalCredits: 999},
		settlement,
		10,
	)
	if err != nil {
		t.Fatalf("settleImageSuccess returned error: %v", err)
	}
	if remote.lastProxySettle == nil {
		t.Fatalf("expected proxy settlement finalizer to be called")
	}
	if remote.lastProxyResponse != settlement {
		t.Fatalf("settlement response was not passed to finalizer")
	}
	if remote.settleCalls != 0 {
		t.Fatalf("legacy settle calls = %d, want 0", remote.settleCalls)
	}
	if local.settleCalls != 0 {
		t.Fatalf("local settle calls = %d, want 0", local.settleCalls)
	}
}

func TestBillingRouting_SettleChatSuccess_RejectsMissingUsage(t *testing.T) {
	remote := &fakeBillingProvider{checkBalanceResult: true}
	local := &fakeBillingProvider{checkBalanceResult: true}
	engine := &fakePricingEngine{
		tokenQuote: PricingQuote{
			TotalCredits: 18,
		},
	}

	s := &llmGatewayServiceImpl{
		billing:       remote,
		localBilling:  local,
		pricingEngine: engine,
		healthTracker: NewChannelHealthTracker(nil),
	}

	ps := &ProviderSelection{
		UseSystemProvider: false,
		Model: llmmodel.LLMModel{
			ID:    uuid.New(),
			Model: "qwen3.5:4b",
		},
		Provider: providermodel.LLMProvider{
			Provider: "ollama",
		},
	}
	bc := &BillingContext{
		RequestID:         "req-missing-usage",
		UseSystemProvider: false,
	}

	err := s.settleChatSuccess(context.Background(), bc, ps, nil, nil, nil, 10)
	if err == nil {
		t.Fatalf("expected settleChatSuccess to reject missing usage")
	}
	if !strings.Contains(err.Error(), "provider returned no token usage") {
		t.Fatalf("error = %v, want missing usage error", err)
	}
	if local.settleCalls != 1 {
		t.Fatalf("local settle calls = %d, want 1", local.settleCalls)
	}
	if remote.settleCalls != 0 {
		t.Fatalf("remote settle calls = %d, want 0", remote.settleCalls)
	}
	if local.lastSettle.Status != "error" {
		t.Fatalf("settle status = %s, want error", local.lastSettle.Status)
	}
	if local.lastSettle.TotalTokens != 0 || local.lastSettle.ActualCredits != 0 {
		t.Fatalf("error settle tokens/credits = %d/%d, want 0/0", local.lastSettle.TotalTokens, local.lastSettle.ActualCredits)
	}
}

func TestBillingRouting_HandleStreamBilling_SettleFailure_EmitsErrorWithoutDone(t *testing.T) {
	remote := &fakeBillingProvider{checkBalanceResult: true}
	local := &fakeBillingProvider{
		checkBalanceResult: true,
		settleErr:          errors.New("settle failed"),
	}
	engine := &fakePricingEngine{
		tokenQuote: PricingQuote{
			TotalCredits: 30,
		},
	}

	s := &llmGatewayServiceImpl{
		billing:       remote,
		localBilling:  local,
		pricingEngine: engine,
		healthTracker: NewChannelHealthTracker(nil),
	}

	in := make(chan adapter.StreamResponse, 2)
	out := make(chan adapter.StreamResponse, 4)

	in <- adapter.StreamResponse{}
	in <- adapter.StreamResponse{
		Done: true,
		Usage: &adapter.Usage{
			PromptTokens:     10,
			CompletionTokens: 20,
			TotalTokens:      30,
		},
	}
	close(in)

	bc := &BillingContext{
		UseSystemProvider: false,
		PromptTokens:      10,
		CompletionTokens:  20,
		RequestID:         "req-1",
	}
	model := &llmmodel.LLMModel{ID: uuid.New()}

	s.handleStreamBilling(context.Background(), in, out, bc, nil, model, time.Now(), nil)

	var got []adapter.StreamResponse
	for resp := range out {
		got = append(got, resp)
	}

	if len(got) < 2 {
		t.Fatalf("expected at least 2 stream responses, got %d", len(got))
	}
	if got[len(got)-1].Error == nil {
		t.Fatalf("expected last stream response to be error when settle fails")
	}
	for i, resp := range got {
		if resp.Done {
			t.Fatalf("response #%d unexpectedly marked as done", i)
		}
	}
}

func TestBillingRouting_HandleStreamBilling_MissingUsageMarksPartialAndEmitsError(t *testing.T) {
	remote := &fakeBillingProvider{checkBalanceResult: true}
	local := &fakeBillingProvider{checkBalanceResult: true}
	engine := &fakePricingEngine{
		tokenQuote: PricingQuote{
			TotalCredits: 30,
		},
	}

	s := &llmGatewayServiceImpl{
		billing:       remote,
		localBilling:  local,
		pricingEngine: engine,
		healthTracker: NewChannelHealthTracker(nil),
	}

	in := make(chan adapter.StreamResponse, 2)
	out := make(chan adapter.StreamResponse, 4)
	in <- adapter.StreamResponse{}
	in <- adapter.StreamResponse{Done: true}
	close(in)

	bc := &BillingContext{
		UseSystemProvider: false,
		PromptTokens:      10,
		CompletionTokens:  20,
		RequestID:         "req-stream-missing-usage",
	}
	model := &llmmodel.LLMModel{ID: uuid.New(), Model: "qwen3.5:4b"}

	s.handleStreamBilling(context.Background(), in, out, bc, nil, model, time.Now(), nil)

	var got []adapter.StreamResponse
	for resp := range out {
		got = append(got, resp)
	}

	if len(got) == 0 {
		t.Fatalf("expected stream error response")
	}
	if got[len(got)-1].Error == nil {
		t.Fatalf("expected last stream response to be error when usage is missing")
	}
	if !strings.Contains(got[len(got)-1].Error.Error(), "provider returned no token usage") {
		t.Fatalf("error = %v, want missing usage error", got[len(got)-1].Error)
	}
	for i, resp := range got {
		if resp.Done {
			t.Fatalf("response #%d unexpectedly marked as done", i)
		}
	}
	if local.settleCalls != 1 {
		t.Fatalf("local settle calls = %d, want 1", local.settleCalls)
	}
	if local.lastSettle.Status != "partial" {
		t.Fatalf("settle status = %s, want partial", local.lastSettle.Status)
	}
	if local.lastSettle.TotalTokens != 0 || local.lastSettle.ActualCredits != 0 {
		t.Fatalf("partial settle tokens/credits = %d/%d, want 0/0", local.lastSettle.TotalTokens, local.lastSettle.ActualCredits)
	}
}

func TestBillingRouting_HandleStreamBilling_UsesPricingEngineQuote(t *testing.T) {
	remote := &fakeBillingProvider{checkBalanceResult: true}
	local := &fakeBillingProvider{checkBalanceResult: true}
	engine := &fakePricingEngine{
		tokenQuote: PricingQuote{
			InputUSD:      decimal.RequireFromString("0.01"),
			OutputUSD:     decimal.RequireFromString("0.02"),
			TotalUSD:      decimal.RequireFromString("0.03"),
			InputCredits:  10,
			OutputCredits: 20,
			TotalCredits:  30,
			Source:        PricingSourceUSDPrice,
		},
	}

	s := &llmGatewayServiceImpl{
		billing:       remote,
		localBilling:  local,
		pricingEngine: engine,
		healthTracker: NewChannelHealthTracker(nil),
	}

	in := make(chan adapter.StreamResponse, 2)
	out := make(chan adapter.StreamResponse, 4)
	in <- adapter.StreamResponse{}
	in <- adapter.StreamResponse{
		Done: true,
		Usage: &adapter.Usage{
			PromptTokens:     10,
			CompletionTokens: 20,
			TotalTokens:      30,
		},
	}
	close(in)

	bc := &BillingContext{
		UseSystemProvider: false,
		RequestID:         "req-stream-pricing",
	}
	model := &llmmodel.LLMModel{ID: uuid.New()}
	bc.ModelID = model.ID
	bc.ModelSource = PricingModelSourceCustom

	s.handleStreamBilling(context.Background(), in, out, bc, nil, model, time.Now(), nil)

	for range out {
	}

	if local.lastSettle == nil {
		t.Fatalf("expected local settle context to be captured")
	}
	if engine.lastModel.Source != PricingModelSourceCustom {
		t.Fatalf("pricing model source = %s, want %s", engine.lastModel.Source, PricingModelSourceCustom)
	}
	if local.lastSettle.ActualCredits != 30 {
		t.Fatalf("actualCredits = %d, want 30", local.lastSettle.ActualCredits)
	}
	if !local.lastSettle.InputCost.Equal(decimal.NewFromInt(10)) {
		t.Fatalf("inputCost = %s, want 10", local.lastSettle.InputCost)
	}
	if !local.lastSettle.OutputCost.Equal(decimal.NewFromInt(20)) {
		t.Fatalf("outputCost = %s, want 20", local.lastSettle.OutputCost)
	}
	if !local.lastSettle.TotalCost.Equal(decimal.NewFromInt(30)) {
		t.Fatalf("totalCost = %s, want 30", local.lastSettle.TotalCost)
	}
}

func TestBillingRouting_BeginBillingAttempt_OfficialCheckBalanceFailThenPrivateSuccess(t *testing.T) {
	remote := &fakeBillingProvider{
		checkBalanceResult: false,
		checkBalanceErr:    errors.New("remote check balance failed"),
	}
	local := &fakeBillingProvider{checkBalanceResult: true}

	s := &llmGatewayServiceImpl{
		billing:      remote,
		localBilling: local,
	}

	apiKey := &apikeymodel.TenantAPIKey{
		ID:             "k1",
		OrganizationID: uuid.NewString(),
	}
	shadowOrgID := uuid.New()
	ownerID := uuid.New()

	official := &ProviderSelection{
		UseSystemProvider: true,
		RouteID:           uuid.New(),
		Model:             llmmodel.LLMModel{ID: uuid.New(), Model: "gpt-4o-mini"},
		Provider:          providermodel.LLMProvider{ID: uuid.New(), Provider: "openai"},
	}
	private := &ProviderSelection{
		UseSystemProvider: false,
		RouteID:           uuid.New(),
		Model:             llmmodel.LLMModel{ID: uuid.New(), Model: "gpt-4o-mini"},
		Provider:          providermodel.LLMProvider{ID: uuid.New(), Provider: "openai"},
	}

	_, err := s.beginBillingAttempt(context.Background(), apiKey, nil, official, shadowOrgID, ownerID, 100, false, time.Now(), "req-official", "req-official-a1")
	if err == nil {
		t.Fatalf("expected official beginBillingAttempt to fail")
	}
	if !errors.Is(err, ErrBillingPreDeductFailed) {
		t.Fatalf("expected ErrBillingPreDeductFailed, got: %v", err)
	}
	if !errors.Is(err, llmerrors.DomainErrBillingFailed) {
		t.Fatalf("expected DomainErrBillingFailed, got: %v", err)
	}
	if remote.checkBalanceCalls != 1 {
		t.Fatalf("remote CheckBalance calls = %d, want 1", remote.checkBalanceCalls)
	}
	if remote.preDeductCalls != 0 {
		t.Fatalf("remote PreDeduct calls = %d, want 0", remote.preDeductCalls)
	}
	if local.preDeductCalls != 0 {
		t.Fatalf("local PreDeduct calls = %d, want 0 before private attempt", local.preDeductCalls)
	}

	if _, err := s.beginBillingAttempt(context.Background(), apiKey, nil, private, shadowOrgID, ownerID, 100, false, time.Now(), "req-private", "req-private-a1"); err != nil {
		t.Fatalf("expected private beginBillingAttempt to succeed, got: %v", err)
	}
	if local.preDeductCalls != 1 {
		t.Fatalf("local PreDeduct calls = %d, want 1 after private attempt", local.preDeductCalls)
	}
	if remote.preDeductCalls != 0 {
		t.Fatalf("remote PreDeduct calls = %d, want 0", remote.preDeductCalls)
	}
}

func TestBillingRouting_BeginBillingAttempt_OfficialPreDeductFailThenPrivateSuccess(t *testing.T) {
	remote := &fakeBillingProvider{
		checkBalanceResult: true,
		preDeductErr:       errors.New("remote pre-deduct failed"),
	}
	local := &fakeBillingProvider{checkBalanceResult: true}

	s := &llmGatewayServiceImpl{
		billing:      remote,
		localBilling: local,
	}

	apiKey := &apikeymodel.TenantAPIKey{
		ID:             "k1",
		OrganizationID: uuid.NewString(),
	}
	shadowOrgID := uuid.New()
	ownerID := uuid.New()

	official := &ProviderSelection{
		UseSystemProvider: true,
		RouteID:           uuid.New(),
		Model:             llmmodel.LLMModel{ID: uuid.New(), Model: "gpt-4o-mini"},
		Provider:          providermodel.LLMProvider{ID: uuid.New(), Provider: "openai"},
	}
	private := &ProviderSelection{
		UseSystemProvider: false,
		RouteID:           uuid.New(),
		Model:             llmmodel.LLMModel{ID: uuid.New(), Model: "gpt-4o-mini"},
		Provider:          providermodel.LLMProvider{ID: uuid.New(), Provider: "openai"},
	}

	_, err := s.beginBillingAttempt(context.Background(), apiKey, nil, official, shadowOrgID, ownerID, 100, false, time.Now(), "req-official", "req-official-a1")
	if err == nil {
		t.Fatalf("expected official beginBillingAttempt to fail")
	}
	if !errors.Is(err, ErrBillingPreDeductFailed) {
		t.Fatalf("expected ErrBillingPreDeductFailed, got: %v", err)
	}
	if remote.preDeductCalls != 1 {
		t.Fatalf("remote PreDeduct calls = %d, want 1", remote.preDeductCalls)
	}

	if _, err := s.beginBillingAttempt(context.Background(), apiKey, nil, private, shadowOrgID, ownerID, 100, false, time.Now(), "req-private", "req-private-a1"); err != nil {
		t.Fatalf("expected private beginBillingAttempt to succeed, got: %v", err)
	}
	if local.preDeductCalls != 1 {
		t.Fatalf("local PreDeduct calls = %d, want 1 after private attempt", local.preDeductCalls)
	}
}

func TestBillingRouting_ResolveBillingDecision_CloudOfficialLaneMismatch(t *testing.T) {
	local := &fakeBillingProvider{checkBalanceResult: true}
	s := &llmGatewayServiceImpl{
		billing:         local, // intentional wrong wiring: remote lane points to local implementation
		localBilling:    local,
		consoleProvider: platformconsole.NewRemote("http://localhost:2625", ""),
	}

	ps := &ProviderSelection{
		UseSystemProvider: true,
		RouteID:           uuid.New(),
	}
	bc := &BillingContext{
		UseSystemProvider: true,
		RequestID:         "req-lane",
	}

	_, err := s.resolveBillingDecision(ps, bc)
	if err == nil {
		t.Fatalf("expected lane mismatch error in cloud mode")
	}
	if !errors.Is(err, ErrBillingLaneMismatch) {
		t.Fatalf("expected ErrBillingLaneMismatch, got: %v", err)
	}
}

func TestBillingRouting_CreateBillingContext_UsesWorkspaceSubjectWhenProvided(t *testing.T) {
	s := &llmGatewayServiceImpl{}
	apiKey := &apikeymodel.TenantAPIKey{ID: "key-1"}
	workspaceID := uuid.NewString()
	appType := "workflow"
	appID := uuid.New()
	accountID := uuid.New()

	appCtx := &AppContext{
		AppID:       &appID,
		AppType:     &appType,
		AccountID:   &accountID,
		WorkspaceID: &workspaceID,
	}
	ps := &ProviderSelection{
		UseSystemProvider: true,
		Model:             llmmodel.LLMModel{ID: uuid.New(), Model: "gpt-4o-mini"},
		Provider:          providermodel.LLMProvider{ID: uuid.New(), Provider: "openai"},
	}

	bc := s.createBillingContext(
		apiKey,
		appCtx,
		ps,
		nil,
		uuid.New(),
		123,
		false,
		time.Now(),
		"req-1",
		"req-1-a1",
	)

	if bc.QuotaSubjectType != quotaSubjectTypeWorkspace {
		t.Fatalf("quota subject type = %s, want %s", bc.QuotaSubjectType, quotaSubjectTypeWorkspace)
	}
	if bc.QuotaSubjectID != workspaceID {
		t.Fatalf("quota subject id = %s, want %s", bc.QuotaSubjectID, workspaceID)
	}
	if bc.WorkspaceID != workspaceID {
		t.Fatalf("workspace id = %s, want %s", bc.WorkspaceID, workspaceID)
	}
}
