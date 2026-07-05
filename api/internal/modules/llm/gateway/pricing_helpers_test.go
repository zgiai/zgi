package gateway

import (
	"context"
	"errors"
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
