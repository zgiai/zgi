package gateway

import (
	"context"
	"errors"
	"reflect"
	"testing"

	"github.com/google/uuid"
	llmmodel "github.com/zgiai/zgi/api/internal/modules/llm/llmmodel/model"
	adapter "github.com/zgiai/zgi/api/internal/modules/llm/protocol/adapters"
	providermodel "github.com/zgiai/zgi/api/internal/modules/llm/provider/model"
)

func TestQuoteTokenPricingWrapsMissingPricingAsBillingUserError(t *testing.T) {
	svc := &llmGatewayServiceImpl{pricingEngine: &fakePricingEngine{tokenErr: ErrPricingNotConfigured}}
	modelID := uuid.New()
	organizationID := uuid.New()

	_, err := svc.quoteTokenPricing(context.Background(), PricingModelRef{
		ModelID:        modelID,
		OrganizationID: organizationID,
		Source:         PricingModelSourceCustom,
		Operation:      PricingOperationRerank,
		Provider:       "qwen",
		Model:          "gte-rerank-v2",
	}, 1, 1)
	if err == nil {
		t.Fatalf("quoteTokenPricing error = nil, want error")
	}
	var userErr *BillingUserError
	if !errors.As(err, &userErr) {
		t.Fatalf("error = %v, want BillingUserError", err)
	}
	if userErr.Kind != BillingUserErrorKindModelPricingNotConfigured {
		t.Fatalf("kind = %s, want %s", userErr.Kind, BillingUserErrorKindModelPricingNotConfigured)
	}
	if userErr.Params["model_id"] != modelID.String() ||
		userErr.Params["organization_id"] != organizationID.String() ||
		userErr.Params["model_source"] != string(PricingModelSourceCustom) ||
		userErr.Params["operation"] != string(PricingOperationRerank) ||
		userErr.Params["provider"] != "qwen" ||
		userErr.Params["model"] != "gte-rerank-v2" {
		t.Fatalf("params = %#v, want model pricing params", userErr.Params)
	}
}

func TestQuoteImagePricingWrapsMissingPricingAsBillingUserError(t *testing.T) {
	svc := &llmGatewayServiceImpl{pricingEngine: &fakePricingEngine{imageErr: ErrPricingNotConfigured}}

	_, err := svc.quoteImagePricing(context.Background(), PricingModelRef{
		Provider: "qwen",
		Model:    "qwen-image",
	}, &adapter.ImageRequest{Model: "qwen-image"})
	if err == nil {
		t.Fatalf("quoteImagePricing error = nil, want error")
	}
	var userErr *BillingUserError
	if !errors.As(err, &userErr) {
		t.Fatalf("error = %v, want BillingUserError", err)
	}
	if userErr.Kind != BillingUserErrorKindModelPricingNotConfigured {
		t.Fatalf("kind = %s, want %s", userErr.Kind, BillingUserErrorKindModelPricingNotConfigured)
	}
	if userErr.Params["operation"] != string(PricingOperationImage) ||
		userErr.Params["provider"] != "qwen" ||
		userErr.Params["model"] != "qwen-image" {
		t.Fatalf("params = %#v, want image operation", userErr.Params)
	}
}

func TestQuoteTokenPricingForSelectionUsesRemoteAuthorityForOfficialRoute(t *testing.T) {
	engine := &fakePricingEngine{tokenErr: ErrPricingNotConfigured}
	want := PricingQuote{InputCredits: 10, OutputCredits: 20, TotalCredits: 30}
	billing := &fakeBillingProvider{platformQuote: want}
	svc := &llmGatewayServiceImpl{
		pricingEngine: engine,
		billing:       billing,
	}
	selection := &ProviderSelection{
		UseSystemProvider: true,
		BillingLane:       UsageBillingLanePlatform,
		Model:             llmmodel.LLMModel{ID: uuid.New()},
	}

	quote, err := svc.quoteTokenPricingForSelection(
		context.Background(),
		selection,
		PricingModelRef{ModelID: selection.Model.ID, Provider: "openai", Model: "gpt-5.4"},
		1,
		1,
	)
	if err != nil {
		t.Fatalf("quoteTokenPricingForSelection() error = %v, want nil", err)
	}
	if !reflect.DeepEqual(quote, want) {
		t.Fatalf("quote = %#v, want %#v", quote, want)
	}
	if engine.tokenCalls != 0 {
		t.Fatalf("local pricing calls = %d, want 0", engine.tokenCalls)
	}
	if billing.platformPrompt != 17 || billing.platformCompletion != 1 {
		t.Fatalf("authoritative quote tokens = (%d, %d), want (17, 1)", billing.platformPrompt, billing.platformCompletion)
	}
}

func TestQuoteTokenPricingForSelectionUsesReasoningFloorAndPlatformSlack(t *testing.T) {
	billing := &fakeBillingProvider{platformQuote: PricingQuote{TotalCredits: 1}}
	svc := &llmGatewayServiceImpl{billing: billing}
	selection := &ProviderSelection{
		UseSystemProvider: true,
		BillingLane:       UsageBillingLanePlatform,
		Model:             llmmodel.LLMModel{ID: uuid.New(), SupportsReasoning: true},
	}

	if _, err := svc.quoteTokenPricingForSelection(
		context.Background(), selection, PricingModelRef{ModelID: selection.Model.ID}, 12, 8,
	); err != nil {
		t.Fatal(err)
	}
	if billing.platformPrompt != 80 || billing.platformCompletion != reasoningCompletionReservationFloor {
		t.Fatalf("authoritative reasoning quote tokens = (%d, %d), want (80, %d)", billing.platformPrompt, billing.platformCompletion, reasoningCompletionReservationFloor)
	}
}

func TestQuoteTokenPricingForSelectionRequiresLocalPricingForPrivateRoute(t *testing.T) {
	engine := &fakePricingEngine{tokenErr: ErrPricingNotConfigured}
	svc := &llmGatewayServiceImpl{pricingEngine: engine}
	selection := &ProviderSelection{
		UseSystemProvider: false,
		BillingLane:       UsageBillingLanePrivate,
	}

	_, err := svc.quoteTokenPricingForSelection(
		context.Background(),
		selection,
		PricingModelRef{Provider: "openai", Model: "gpt-5.4"},
		1,
		1,
	)
	if err == nil {
		t.Fatal("quoteTokenPricingForSelection() error = nil, want error")
	}
	var userErr *BillingUserError
	if !errors.As(err, &userErr) {
		t.Fatalf("error = %v, want BillingUserError", err)
	}
	if userErr.Kind != BillingUserErrorKindModelPricingNotConfigured {
		t.Fatalf("kind = %s, want %s", userErr.Kind, BillingUserErrorKindModelPricingNotConfigured)
	}
	if engine.tokenCalls != 1 {
		t.Fatalf("local pricing calls = %d, want 1", engine.tokenCalls)
	}
}

func TestQuoteTokenPricingForSelectionUsesConservativeReasoningReservation(t *testing.T) {
	engine := &fakePricingEngine{}
	svc := &llmGatewayServiceImpl{pricingEngine: engine}
	selection := &ProviderSelection{
		BillingLane: UsageBillingLanePrivate,
		Model:       llmmodel.LLMModel{SupportsReasoning: true},
	}

	if _, err := svc.quoteTokenPricingForSelection(
		context.Background(),
		selection,
		PricingModelRef{Provider: "qwen", Model: "qwen-reasoning"},
		12,
		8,
	); err != nil {
		t.Fatal(err)
	}
	if engine.lastPromptTokens != reasoningPromptReservationFloor || engine.lastCompletionTokens != reasoningCompletionReservationFloor {
		t.Fatalf("quoted tokens = (%d, %d), want (%d, %d)", engine.lastPromptTokens, engine.lastCompletionTokens, reasoningPromptReservationFloor, reasoningCompletionReservationFloor)
	}

	if _, err := svc.quoteTokenPricingForSelection(
		context.Background(),
		selection,
		PricingModelRef{Provider: "qwen", Model: "qwen-reasoning"},
		100,
		0,
	); err != nil {
		t.Fatal(err)
	}
	if engine.lastPromptTokens != 100 || engine.lastCompletionTokens != 0 {
		t.Fatalf("non-generation quote tokens = (%d, %d), want (100, 0)", engine.lastPromptTokens, engine.lastCompletionTokens)
	}
}

func TestQuoteImagePricingForSelectionSkipsLocalPricingForOfficialRoute(t *testing.T) {
	svc := &llmGatewayServiceImpl{pricingEngine: &fakePricingEngine{imageErr: ErrPricingNotConfigured}}
	selection := &ProviderSelection{
		UseSystemProvider: true,
		BillingLane:       UsageBillingLanePlatform,
	}

	quote, err := svc.quoteImagePricingForSelection(
		context.Background(),
		selection,
		PricingModelRef{Provider: "openai", Model: "gpt-image-1"},
		&adapter.ImageRequest{Model: "gpt-image-1"},
	)
	if err != nil {
		t.Fatalf("quoteImagePricingForSelection() error = %v, want nil", err)
	}
	if !reflect.DeepEqual(quote, PricingQuote{}) {
		t.Fatalf("quote = %#v, want zero quote", quote)
	}
}

func TestQuoteTokenPricingForSelectionRejectsInvalidBillingLane(t *testing.T) {
	svc := &llmGatewayServiceImpl{pricingEngine: &fakePricingEngine{}}
	selection := &ProviderSelection{
		UseSystemProvider: true,
		BillingLane:       UsageBillingLane("invalid"),
	}

	_, err := svc.quoteTokenPricingForSelection(
		context.Background(),
		selection,
		PricingModelRef{Provider: "openai", Model: "gpt-5.4"},
		1,
		1,
	)
	if err == nil {
		t.Fatal("quoteTokenPricingForSelection() error = nil, want error")
	}
}

func TestPricingModelRefFromSelectionIncludesProviderAndModel(t *testing.T) {
	modelID := uuid.New()
	organizationID := uuid.New()
	selection := &ProviderSelection{
		Provider: providermodel.LLMProvider{Provider: "fallback-provider"},
		Model: llmmodel.LLMModel{
			ID:       modelID,
			Provider: "qwen",
			Model:    "qwen-plus",
		},
		ModelSource:    PricingModelSourceGlobal,
		OrganizationID: organizationID,
	}

	ref := pricingModelRefFromSelection(selection)

	if ref.ModelID != modelID ||
		ref.OrganizationID != organizationID ||
		ref.Provider != "qwen" ||
		ref.Model != "qwen-plus" {
		t.Fatalf("ref = %#v, want provider and model from selection", ref)
	}
}

func TestPricingModelRefFromBillingContextIncludesProviderAndModel(t *testing.T) {
	modelID := uuid.New()
	organizationID := uuid.New()

	ref := pricingModelRefFromBillingContext(&BillingContext{
		ModelID:        modelID,
		OrganizationID: organizationID.String(),
		ModelSource:    PricingModelSourceGlobal,
		ModelName:      "qwen-plus",
		ProviderName:   "qwen",
	})

	if ref.ModelID != modelID ||
		ref.OrganizationID != organizationID ||
		ref.Provider != "qwen" ||
		ref.Model != "qwen-plus" {
		t.Fatalf("ref = %#v, want provider and model from billing context", ref)
	}
}
