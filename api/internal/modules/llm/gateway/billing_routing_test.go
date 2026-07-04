package gateway

import (
	"context"
	"encoding/json"
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
	tokenCalls int
}

func (f *fakePricingEngine) QuoteTokens(ctx context.Context, model PricingModelRef, promptTokens, completionTokens int) (PricingQuote, error) {
	f.tokenCalls++
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

func TestBillingRouting_SettleChatSuccess_UsesLockedQuote(t *testing.T) {
	remote := &fakeBillingProvider{checkBalanceResult: true}
	local := &fakeBillingProvider{checkBalanceResult: true}
	engine := &fakePricingEngine{tokenErr: errors.New("pricing changed during request")}

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
		RequestID:         "req-locked-pricing",
		UseSystemProvider: false,
	}
	lockTokenPricingQuote(bc, withTokenPricingBasis(
		newUSDQuote(decimal.Zero, decimal.Zero, PricingSourceCodeDefaultFallback, "in,out", UsageSourceProviderUsage, nil),
		decimal.RequireFromString("1"),
		decimal.RequireFromString("2"),
		true,
		true,
		"in",
		"out",
	))

	err := s.settleChatSuccess(
		context.Background(),
		bc,
		ps,
		nil,
		&adapter.Usage{
			PromptTokens:     1000,
			CompletionTokens: 1000,
			TotalTokens:      2000,
		},
		nil,
		10,
	)
	if err != nil {
		t.Fatalf("settleChatSuccess returned error: %v", err)
	}
	if engine.tokenCalls != 0 {
		t.Fatalf("pricing engine token calls = %d, want 0", engine.tokenCalls)
	}
	if local.lastSettle == nil {
		t.Fatalf("expected settle context to be captured")
	}
	if local.lastSettle.ActualCredits != 3000 {
		t.Fatalf("actualCredits = %d, want 3000", local.lastSettle.ActualCredits)
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

func TestBillingRouting_SettleChatSuccess_PlatformClearsPreDeductTokensWhenUsageMissing(t *testing.T) {
	remote := &fakeProxySettlementBillingProvider{}
	s := &llmGatewayServiceImpl{
		billing:       remote,
		healthTracker: NewChannelHealthTracker(nil),
	}
	ps := &ProviderSelection{
		UseSystemProvider: true,
		BillingLane:       UsageBillingLanePlatform,
		Model:             llmmodel.LLMModel{ID: uuid.New(), Model: "gpt-5"},
		Provider:          providermodel.LLMProvider{Provider: "openai"},
	}
	bc := &BillingContext{
		RequestID:         "req-proxy-no-usage",
		AttemptID:         "req-proxy-no-usage-a1",
		DeductionID:       "deduction-proxy-no-usage",
		UseSystemProvider: true,
		BillingLane:       UsageBillingLanePlatform,
		PromptTokens:      111,
		CompletionTokens:  222,
		TotalTokens:       333,
	}
	settlement := &adapter.SettlementResult{
		SettlementID:     "deduction-proxy-no-usage",
		OfficialPoints:   7,
		RemainingBalance: 93,
		Status:           "settled",
	}

	if err := s.settleChatSuccess(context.Background(), bc, ps, nil, nil, settlement, 10); err != nil {
		t.Fatalf("settleChatSuccess returned error: %v", err)
	}
	if remote.lastProxySettle == nil {
		t.Fatalf("expected proxy settlement finalizer to be called")
	}
	if remote.lastProxySettle.PromptTokens != 0 || remote.lastProxySettle.CompletionTokens != 0 || remote.lastProxySettle.TotalTokens != 0 {
		t.Fatalf("platform proxy tokens = %d/%d/%d, want 0/0/0 when console usage is absent",
			remote.lastProxySettle.PromptTokens,
			remote.lastProxySettle.CompletionTokens,
			remote.lastProxySettle.TotalTokens,
		)
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

func TestBillingRouting_ImageSettlementReusesEstimatedQuote(t *testing.T) {
	wantErr := errors.New("image pricing unavailable")
	s := &llmGatewayServiceImpl{
		pricingEngine: &fakePricingEngine{imageErr: wantErr},
	}

	quote, err := s.quoteImagePricingForSettlement(
		context.Background(),
		PricingModelRef{ModelID: uuid.New(), Source: PricingModelSourceGlobal},
		&adapter.ImageRequest{Model: "gpt-image"},
		PricingQuote{TotalCredits: 123},
	)
	if err != nil {
		t.Fatalf("quoteImagePricingForSettlement returned error: %v", err)
	}
	if quote.TotalCredits != 123 {
		t.Fatalf("total credits = %d, want 123", quote.TotalCredits)
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

func TestBillingRouting_EstimatesMissingChatUsageFromResponseText(t *testing.T) {
	s := &llmGatewayServiceImpl{tokenEstimator: NewTokenEstimator()}
	req := &adapter.ChatRequest{
		Model:    "qwen3.5:4b",
		Messages: []adapter.Message{{Role: "user", Content: "12345678"}},
	}
	resp := &adapter.ChatResponse{
		Choices: []adapter.Choice{{
			Message: adapter.Message{Role: "assistant", Content: "12345678"},
		}},
	}

	usage := s.estimateMissingChatUsage(req, resp)
	if usage == nil {
		t.Fatal("usage = nil, want estimated usage")
	}
	expectedPrompt := s.tokenEstimatorForFallback().EstimateChatPromptTokens(req)
	expectedCompletion := s.tokenEstimatorForFallback().EstimateTextTokensForModel(req.Model, "12345678")
	if usage.PromptTokens != expectedPrompt || usage.CompletionTokens != expectedCompletion || usage.TotalTokens != expectedPrompt+expectedCompletion {
		t.Fatalf("usage = %+v, want prompt=%d completion=%d total=%d", usage, expectedPrompt, expectedCompletion, expectedPrompt+expectedCompletion)
	}
}

func TestBillingRouting_EstimatesMissingCreateResponseUsageFromOutputText(t *testing.T) {
	s := &llmGatewayServiceImpl{tokenEstimator: NewTokenEstimator()}
	req := &adapter.CreateResponseRequest{
		Model: "qwen3.5:4b",
		Input: "12345678",
	}
	resp := &adapter.CreateResponseResponse{
		Output: []adapter.Output{{
			Type: "message",
			Content: []adapter.OutputContent{{
				Type: "output_text",
				Text: "12345678",
			}},
		}},
	}

	usage, estimated := s.completeCreateResponseUsageFromText(req, resp.Usage, createResponseText(resp), 0)

	if !estimated {
		t.Fatal("estimated = false, want true")
	}
	if usage == nil {
		t.Fatal("usage = nil, want estimated usage")
	}
	if usage.PromptTokens <= 0 || usage.CompletionTokens <= 0 || usage.TotalTokens <= usage.PromptTokens {
		t.Fatalf("usage = %+v, want positive prompt and completion tokens", usage)
	}
}

func TestBillingRouting_PreservesKnownTotalWhenChatUsageIsIncomplete(t *testing.T) {
	s := &llmGatewayServiceImpl{tokenEstimator: NewTokenEstimator()}
	req := &adapter.ChatRequest{
		Model:    "qwen3.5:4b",
		Messages: []adapter.Message{{Role: "user", Content: "12345678"}},
	}
	resp := &adapter.ChatResponse{
		Usage: &adapter.Usage{PromptTokens: 8, TotalTokens: 8},
		Choices: []adapter.Choice{{
			Message: adapter.Message{Role: "assistant", Content: "12345678"},
		}},
	}

	usage := s.estimateMissingChatUsage(req, resp)
	if usage == nil {
		t.Fatal("usage = nil, want completed usage")
	}
	if usage.PromptTokens != 8 || usage.CompletionTokens != 0 || usage.TotalTokens != 8 {
		t.Fatalf("usage = %+v, want provider total preserved", usage)
	}
}

func TestBillingRouting_SplitsTotalOnlyChatUsageWithinKnownTotal(t *testing.T) {
	s := &llmGatewayServiceImpl{tokenEstimator: NewTokenEstimator()}
	req := &adapter.ChatRequest{
		Model:    "qwen3.5:4b",
		Messages: []adapter.Message{{Role: "user", Content: "12345678"}},
	}
	resp := &adapter.ChatResponse{
		Usage: &adapter.Usage{TotalTokens: 8},
		Choices: []adapter.Choice{{
			Message: adapter.Message{Role: "assistant", Content: "12345678"},
		}},
	}

	usage := s.estimateMissingChatUsage(req, resp)
	if usage == nil {
		t.Fatal("usage = nil, want split usage")
	}
	if usage.TotalTokens != 8 || usage.PromptTokens+usage.CompletionTokens != 8 {
		t.Fatalf("usage = %+v, want split capped to provider total 8", usage)
	}
}

func TestBillingRouting_EstimatesMissingChatUsageFromToolCall(t *testing.T) {
	s := &llmGatewayServiceImpl{tokenEstimator: NewTokenEstimator()}
	req := &adapter.ChatRequest{
		Model:    "qwen3.5:4b",
		Messages: []adapter.Message{{Role: "user", Content: "weather in paris"}},
	}
	resp := &adapter.ChatResponse{
		Choices: []adapter.Choice{{
			Message: adapter.Message{
				Role: "assistant",
				ToolCalls: []adapter.ToolCall{{
					ID:   "call_1",
					Type: "function",
					Function: adapter.FunctionCall{
						Name:      "get_weather",
						Arguments: `{"city":"Paris"}`,
					},
				}},
			},
		}},
	}

	usage := s.estimateMissingChatUsage(req, resp)
	if usage == nil {
		t.Fatal("usage = nil, want estimated usage from tool call")
	}
	if usage.CompletionTokens <= 0 || usage.TotalTokens <= usage.PromptTokens {
		t.Fatalf("usage = %+v, want positive tool-call completion tokens", usage)
	}
}

func TestBillingRouting_SplitsTotalOnlyChatUsage(t *testing.T) {
	s := &llmGatewayServiceImpl{tokenEstimator: NewTokenEstimator()}
	req := &adapter.ChatRequest{
		Model:    "qwen3.5:4b",
		Messages: []adapter.Message{{Role: "user", Content: "12345678"}},
	}
	resp := &adapter.ChatResponse{
		Usage: &adapter.Usage{TotalTokens: 20},
	}

	usage := s.estimateMissingChatUsage(req, resp)
	if usage == nil {
		t.Fatal("usage = nil, want split usage")
	}
	if usage.PromptTokens <= 0 || usage.CompletionTokens <= 0 || usage.TotalTokens != 20 {
		t.Fatalf("usage = %+v, want prompt/completion split with total 20", usage)
	}
}

func TestBillingRouting_SettleEmbeddingSuccessRejectsZeroTokens(t *testing.T) {
	remote := &fakeBillingProvider{checkBalanceResult: true}
	local := &fakeBillingProvider{checkBalanceResult: true}
	engine := &fakePricingEngine{tokenQuote: PricingQuote{TotalCredits: 30}}
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
			Model: "embed-mock",
		},
		Provider: providermodel.LLMProvider{
			Provider: "openai",
		},
	}
	bc := &BillingContext{
		RequestID:         "req-embedding-zero-usage",
		UseSystemProvider: false,
	}

	err := s.settleEmbeddingsSuccess(context.Background(), bc, ps, nil, 0, nil, 10)

	if err == nil {
		t.Fatal("expected missing usage error")
	}
	if !strings.Contains(err.Error(), "provider returned no token usage") {
		t.Fatalf("error = %v, want missing usage error", err)
	}
	if local.settleCalls != 1 {
		t.Fatalf("local settle calls = %d, want 1", local.settleCalls)
	}
	if local.lastSettle.Status != "error" {
		t.Fatalf("settle status = %s, want error", local.lastSettle.Status)
	}
	if local.lastSettle.TotalTokens != 0 || local.lastSettle.ActualCredits != 0 {
		t.Fatalf("error settle tokens/credits = %d/%d, want 0/0", local.lastSettle.TotalTokens, local.lastSettle.ActualCredits)
	}
}

func TestBillingRouting_EstimatesMissingNativeUsageForPrivateChannel(t *testing.T) {
	s := &llmGatewayServiceImpl{tokenEstimator: NewTokenEstimator()}
	resp := &adapter.RawResponse{
		Body: json.RawMessage(`{"id":"resp_1","output":[{"type":"message","content":[{"type":"output_text","text":"12345678"}]}]}`),
	}
	bc := &BillingContext{UseSystemProvider: false, PromptTokens: 9}
	selection := &ProviderSelection{UseSystemProvider: false}

	estimated, err := s.ensureNativeResponseUsageForSelection(selection, bc, resp, "gpt-5", nativeUsageBodyFormatResponses)

	if err != nil {
		t.Fatalf("ensure native usage error = %v", err)
	}
	if !estimated {
		t.Fatal("estimated = false, want true for private native response without usage")
	}
	if resp.Usage == nil {
		t.Fatal("native response usage = nil, want estimated usage")
	}
	if resp.Usage.PromptTokens != 9 || resp.Usage.CompletionTokens <= 0 {
		t.Fatalf("native response usage = %+v, want prompt=9 and positive completion", resp.Usage)
	}
	if bc.UsageSource != usageSourceEstimated {
		t.Fatalf("billing usage source = %q, want %q", bc.UsageSource, usageSourceEstimated)
	}
	var body map[string]any
	if err := json.Unmarshal(resp.Body, &body); err != nil {
		t.Fatalf("native response body is not JSON: %v", err)
	}
	usageBody, ok := body["usage"].(map[string]any)
	if !ok {
		t.Fatalf("native response body usage = %#v, want object", body["usage"])
	}
	if usageBody["total_tokens"].(float64) <= 0 {
		t.Fatalf("native response body usage = %#v, want positive total_tokens", usageBody)
	}
}

func TestBillingRouting_PreservesPromptOnlyNativeUsageForBillingWithoutOverwritingRawUsage(t *testing.T) {
	s := &llmGatewayServiceImpl{tokenEstimator: NewTokenEstimator()}
	resp := &adapter.RawResponse{
		Usage: &adapter.Usage{PromptTokens: 9, TotalTokens: 9},
		Body:  json.RawMessage(`{"id":"resp_1","output":[{"type":"message","content":[{"type":"output_text","text":"12345678"}]}],"usage":{"input_tokens":9}}`),
	}
	bc := &BillingContext{UseSystemProvider: false, PromptTokens: 9}
	selection := &ProviderSelection{UseSystemProvider: false}

	estimated, err := s.ensureNativeResponseUsageForSelection(selection, bc, resp, "gpt-5", nativeUsageBodyFormatResponses)

	if err != nil {
		t.Fatalf("ensure native usage error = %v", err)
	}
	if estimated {
		t.Fatal("estimated = true, want false when provider total is already known")
	}
	if resp.Usage.PromptTokens != 9 || resp.Usage.CompletionTokens != 0 || resp.Usage.TotalTokens != 9 {
		t.Fatalf("native response usage = %+v, want provider total preserved", resp.Usage)
	}
	var body map[string]any
	if err := json.Unmarshal(resp.Body, &body); err != nil {
		t.Fatalf("native response body is not JSON: %v", err)
	}
	usageBody, ok := body["usage"].(map[string]any)
	if !ok {
		t.Fatalf("native response body usage = %#v, want existing object", body["usage"])
	}
	if _, ok := usageBody["output_tokens"]; ok {
		t.Fatalf("native response body usage = %#v, want upstream usage preserved", usageBody)
	}
}

func TestBillingRouting_DoesNotEstimateMissingNativeUsageForPlatformChannel(t *testing.T) {
	s := &llmGatewayServiceImpl{tokenEstimator: NewTokenEstimator()}
	resp := &adapter.RawResponse{
		Body: json.RawMessage(`{"id":"resp_1","output":[{"type":"message","content":[{"type":"output_text","text":"12345678"}]}]}`),
	}
	bc := &BillingContext{UseSystemProvider: true, PromptTokens: 9}
	selection := &ProviderSelection{UseSystemProvider: true}

	estimated, err := s.ensureNativeResponseUsageForSelection(selection, bc, resp, "gpt-5", nativeUsageBodyFormatResponses)

	if err != nil {
		t.Fatalf("ensure native usage error = %v", err)
	}
	if estimated {
		t.Fatal("estimated = true, want false for platform native response")
	}
	if resp.Usage != nil {
		t.Fatalf("native response usage = %+v, want nil for platform missing usage", resp.Usage)
	}
	if bc.UsageSource != "" {
		t.Fatalf("billing usage source = %q, want empty", bc.UsageSource)
	}
}

func TestBillingRouting_EstimatesMissingAnthropicUsageInRawBody(t *testing.T) {
	s := &llmGatewayServiceImpl{tokenEstimator: NewTokenEstimator()}
	resp := &adapter.RawResponse{
		Body: json.RawMessage(`{"id":"msg_1","content":[{"type":"text","text":"hello world"}]}`),
	}
	bc := &BillingContext{UseSystemProvider: false, PromptTokens: 7}
	selection := &ProviderSelection{UseSystemProvider: false}

	estimated, err := s.ensureNativeResponseUsageForSelection(selection, bc, resp, "claude-3-5-sonnet", nativeUsageBodyFormatAnthropic)

	if err != nil {
		t.Fatalf("ensure native usage error = %v", err)
	}
	if !estimated {
		t.Fatal("estimated = false, want true for private anthropic response without usage")
	}
	var body map[string]any
	if err := json.Unmarshal(resp.Body, &body); err != nil {
		t.Fatalf("anthropic response body is not JSON: %v", err)
	}
	usageBody, ok := body["usage"].(map[string]any)
	if !ok {
		t.Fatalf("anthropic response body usage = %#v, want object", body["usage"])
	}
	if usageBody["input_tokens"].(float64) != 7 || usageBody["output_tokens"].(float64) <= 0 {
		t.Fatalf("anthropic response body usage = %#v, want prompt=7 and positive output", usageBody)
	}
	if _, ok := usageBody["total_tokens"]; ok {
		t.Fatalf("anthropic response body usage = %#v, want no total_tokens", usageBody)
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

func TestBillingRouting_HandleStreamBilling_EstimatesMissingUsageFromContent(t *testing.T) {
	remote := &fakeBillingProvider{checkBalanceResult: true}
	local := &fakeBillingProvider{checkBalanceResult: true}
	engine := &fakePricingEngine{
		tokenQuote: PricingQuote{
			InputCredits:  10,
			OutputCredits: 20,
			TotalCredits:  30,
		},
	}

	s := &llmGatewayServiceImpl{
		billing:        remote,
		localBilling:   local,
		pricingEngine:  engine,
		healthTracker:  NewChannelHealthTracker(nil),
		tokenEstimator: NewTokenEstimator(),
	}

	in := make(chan adapter.StreamResponse, 3)
	out := make(chan adapter.StreamResponse, 4)
	in <- adapter.StreamResponse{
		Choices: []adapter.StreamChoice{
			{Delta: adapter.Message{Content: "Hel"}},
		},
	}
	in <- adapter.StreamResponse{
		Choices: []adapter.StreamChoice{
			{Delta: adapter.Message{Content: "lo"}},
		},
	}
	in <- adapter.StreamResponse{Done: true}
	close(in)

	bc := &BillingContext{
		UseSystemProvider: false,
		PromptTokens:      10,
		CompletionTokens:  20,
		RequestID:         "req-stream-estimated-usage",
	}
	model := &llmmodel.LLMModel{ID: uuid.New(), Model: "qwen3.5:4b"}

	s.handleStreamBilling(context.Background(), in, out, bc, nil, model, time.Now(), nil)

	var got []adapter.StreamResponse
	for resp := range out {
		got = append(got, resp)
	}

	if len(got) != 3 {
		t.Fatalf("stream responses = %d, want 3", len(got))
	}
	if got[len(got)-1].Error != nil {
		t.Fatalf("last stream error = %v, want nil", got[len(got)-1].Error)
	}
	if !got[len(got)-1].Done {
		t.Fatalf("last stream response Done = false, want true")
	}
	if got[len(got)-1].Usage == nil {
		t.Fatalf("last stream usage = nil, want estimated usage")
	}
	expectedCompletion := s.tokenEstimatorForFallback().EstimateTextTokensForModel(model.Model, "Hello")
	if got[len(got)-1].Usage.PromptTokens != 10 || got[len(got)-1].Usage.CompletionTokens != expectedCompletion {
		t.Fatalf("last stream usage = %+v, want prompt=10 completion=%d", got[len(got)-1].Usage, expectedCompletion)
	}
	if local.lastSettle == nil {
		t.Fatalf("expected local settle context to be captured")
	}
	if local.lastSettle.Status != "success" {
		t.Fatalf("settle status = %s, want success", local.lastSettle.Status)
	}
	if local.lastSettle.TotalTokens != 10+expectedCompletion || local.lastSettle.ActualCredits != 30 {
		t.Fatalf("settle tokens/credits = %d/%d, want %d/30", local.lastSettle.TotalTokens, local.lastSettle.ActualCredits, 10+expectedCompletion)
	}
	if local.lastSettle.UsageSource != usageSourceEstimated {
		t.Fatalf("settle usage source = %s, want %s", local.lastSettle.UsageSource, usageSourceEstimated)
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

func TestBillingRouting_HandleNativeStreamBilling_InjectsResponsesUsageIntoTerminalEvent(t *testing.T) {
	remote := &fakeBillingProvider{checkBalanceResult: true}
	local := &fakeBillingProvider{checkBalanceResult: true}
	engine := &fakePricingEngine{
		tokenQuote: PricingQuote{
			InputCredits:  10,
			OutputCredits: 20,
			TotalCredits:  30,
		},
	}
	s := &llmGatewayServiceImpl{
		billing:        remote,
		localBilling:   local,
		pricingEngine:  engine,
		healthTracker:  NewChannelHealthTracker(nil),
		tokenEstimator: NewTokenEstimator(),
	}
	in := make(chan adapter.RawStreamEvent, 4)
	out := make(chan adapter.RawStreamEvent, 5)
	in <- adapter.RawStreamEvent{
		Event: "response.output_text.delta",
		Data:  json.RawMessage(`{"type":"response.output_text.delta","delta":"Hel"}`),
	}
	in <- adapter.RawStreamEvent{
		Event: "response.output_text.delta",
		Data:  json.RawMessage(`{"type":"response.output_text.delta","delta":"lo"}`),
	}
	in <- adapter.RawStreamEvent{
		Event: "response.completed",
		Data:  json.RawMessage(`{"type":"response.completed","response":{"id":"resp_1","output":[{"type":"message","content":[{"type":"output_text","text":"Hello"}]}]}}`),
	}
	in <- adapter.RawStreamEvent{Done: true}
	close(in)
	bc := &BillingContext{
		UseSystemProvider: false,
		PromptTokens:      10,
		CompletionTokens:  20,
		RequestID:         "req-native-responses-estimated-usage",
	}
	ps := &ProviderSelection{
		UseSystemProvider: false,
		Model:             llmmodel.LLMModel{ID: uuid.New(), Model: "gpt-5"},
		Provider:          providermodel.LLMProvider{Provider: "openai"},
	}

	s.handleNativeStreamBilling(context.Background(), in, out, bc, ps, nil, time.Now(), "gpt-5", nativeUsageBodyFormatResponses)

	var got []adapter.RawStreamEvent
	for event := range out {
		got = append(got, event)
	}
	if len(got) != 4 {
		t.Fatalf("raw events = %d, want 4", len(got))
	}
	if got[2].Event != "response.completed" {
		t.Fatalf("terminal event = %q, want response.completed", got[2].Event)
	}
	var payload map[string]any
	if err := json.Unmarshal(got[2].Data, &payload); err != nil {
		t.Fatalf("terminal data is not JSON: %v", err)
	}
	responseBody, _ := payload["response"].(map[string]any)
	usageBody, _ := responseBody["usage"].(map[string]any)
	expectedCompletion := float64(s.tokenEstimatorForFallback().EstimateTextTokensForModel("gpt-5", "Hello"))
	if usageBody["input_tokens"].(float64) != 10 || usageBody["output_tokens"].(float64) != expectedCompletion {
		t.Fatalf("terminal usage = %#v, want injected usage", usageBody)
	}
	if !got[3].Done || got[3].Usage == nil {
		t.Fatalf("done event = %+v, want done with usage", got[3])
	}
	if local.lastSettle == nil || local.lastSettle.Status != "success" {
		t.Fatalf("local settle = %+v, want success", local.lastSettle)
	}
	if local.lastSettle.UsageSource != usageSourceEstimated {
		t.Fatalf("settle usage source = %s, want %s", local.lastSettle.UsageSource, usageSourceEstimated)
	}
	if local.lastSettle.CompletionTokens != int(expectedCompletion) {
		t.Fatalf("settle completion tokens = %d, want %d without double-counting terminal output", local.lastSettle.CompletionTokens, int(expectedCompletion))
	}
}

func TestBillingRouting_HandleNativeStreamBilling_EmitsAnthropicUsageBeforeStop(t *testing.T) {
	remote := &fakeBillingProvider{checkBalanceResult: true}
	local := &fakeBillingProvider{checkBalanceResult: true}
	engine := &fakePricingEngine{
		tokenQuote: PricingQuote{
			InputCredits:  10,
			OutputCredits: 20,
			TotalCredits:  30,
		},
	}
	s := &llmGatewayServiceImpl{
		billing:        remote,
		localBilling:   local,
		pricingEngine:  engine,
		healthTracker:  NewChannelHealthTracker(nil),
		tokenEstimator: NewTokenEstimator(),
	}
	in := make(chan adapter.RawStreamEvent, 3)
	out := make(chan adapter.RawStreamEvent, 5)
	in <- adapter.RawStreamEvent{
		Event: "content_block_delta",
		Data:  json.RawMessage(`{"type":"content_block_delta","delta":{"type":"text_delta","text":"12345678"}}`),
	}
	in <- adapter.RawStreamEvent{
		Event: "message_stop",
		Data:  json.RawMessage(`{"type":"message_stop"}`),
	}
	in <- adapter.RawStreamEvent{Done: true}
	close(in)
	bc := &BillingContext{
		UseSystemProvider: false,
		PromptTokens:      7,
		CompletionTokens:  20,
		RequestID:         "req-native-anthropic-estimated-usage",
	}
	ps := &ProviderSelection{
		UseSystemProvider: false,
		Model:             llmmodel.LLMModel{ID: uuid.New(), Model: "claude-sonnet-4-5"},
		Provider:          providermodel.LLMProvider{Provider: "anthropic"},
	}

	s.handleNativeStreamBilling(context.Background(), in, out, bc, ps, nil, time.Now(), "claude-sonnet-4-5", nativeUsageBodyFormatAnthropic)

	var got []adapter.RawStreamEvent
	for event := range out {
		got = append(got, event)
	}
	if len(got) != 4 {
		t.Fatalf("raw events = %d, want 4", len(got))
	}
	if got[1].Event != "message_delta" || got[2].Event != "message_stop" {
		t.Fatalf("events = %q, %q, want usage delta before stop", got[1].Event, got[2].Event)
	}
	var payload map[string]any
	if err := json.Unmarshal(got[1].Data, &payload); err != nil {
		t.Fatalf("usage event data is not JSON: %v", err)
	}
	usageBody, _ := payload["usage"].(map[string]any)
	if usageBody["input_tokens"].(float64) != 7 || usageBody["output_tokens"].(float64) <= 0 {
		t.Fatalf("anthropic usage = %#v, want injected usage", usageBody)
	}
	if !got[3].Done || got[3].Usage == nil {
		t.Fatalf("done event = %+v, want done with usage", got[3])
	}
	if local.lastSettle == nil || local.lastSettle.Status != "success" {
		t.Fatalf("local settle = %+v, want success", local.lastSettle)
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

func TestBillingRouting_HandleStreamBilling_UsesLockedQuote(t *testing.T) {
	remote := &fakeBillingProvider{checkBalanceResult: true}
	local := &fakeBillingProvider{checkBalanceResult: true}
	engine := &fakePricingEngine{tokenErr: errors.New("pricing changed during stream")}

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
			PromptTokens:     1000,
			CompletionTokens: 1000,
			TotalTokens:      2000,
		},
	}
	close(in)

	bc := &BillingContext{
		UseSystemProvider: false,
		RequestID:         "req-stream-locked-pricing",
	}
	lockTokenPricingQuote(bc, withTokenPricingBasis(
		newUSDQuote(decimal.Zero, decimal.Zero, PricingSourceCodeDefaultFallback, "in,out", UsageSourceProviderUsage, nil),
		decimal.RequireFromString("1"),
		decimal.RequireFromString("2"),
		true,
		true,
		"in",
		"out",
	))

	s.handleStreamBilling(context.Background(), in, out, bc, nil, nil, time.Now(), nil)

	for range out {
	}
	if engine.tokenCalls != 0 {
		t.Fatalf("pricing engine token calls = %d, want 0", engine.tokenCalls)
	}
	if local.lastSettle == nil {
		t.Fatalf("expected local settle context to be captured")
	}
	if local.lastSettle.ActualCredits != 3000 {
		t.Fatalf("actualCredits = %d, want 3000", local.lastSettle.ActualCredits)
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
