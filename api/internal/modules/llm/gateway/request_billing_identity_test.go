package gateway

import (
	"testing"
	"time"

	"github.com/google/uuid"
	apikeymodel "github.com/zgiai/zgi/api/internal/modules/llm/apikey/model"
	llmmodel "github.com/zgiai/zgi/api/internal/modules/llm/llmmodel/model"
	providermodel "github.com/zgiai/zgi/api/internal/modules/llm/provider/model"
)

func TestCreateBillingContextKeepsSupplierIdentityWhenModelVendorDiffers(t *testing.T) {
	supplierProviderID := uuid.New()
	routeModelID := uuid.New()
	vendorProviderID := uuid.New()
	vendorModelID := uuid.New()
	selection := &ProviderSelection{
		Provider: providermodel.LLMProvider{
			ID:       supplierProviderID,
			Provider: "agicto 供应商",
		},
		Model: llmmodel.LLMModel{
			ID:       routeModelID,
			Provider: "agicto 供应商",
			Model:    "doubao/doubao-seedance-2-0-mini-260615",
		},
		BillingLane:           UsageBillingLanePlatform,
		UseSystemProvider:     true,
		ModelVendor:           "doubao",
		ModelVendorProviderID: vendorProviderID,
		ModelVendorModelID:    vendorModelID,
	}
	apiKey := &apikeymodel.TenantAPIKey{ID: "key-1"}

	ctx := (&llmGatewayServiceImpl{}).createBillingContext(
		apiKey,
		nil,
		selection,
		nil,
		uuid.New(),
		0,
		false,
		time.Time{},
		"request-1",
		"attempt-1",
	)

	if ctx.ProviderName != "agicto 供应商" || ctx.ProviderID != supplierProviderID {
		t.Fatalf("billing provider = %q/%s, want supplier agicto 供应商/%s", ctx.ProviderName, ctx.ProviderID, supplierProviderID)
	}
	if ctx.ModelID != routeModelID {
		t.Fatalf("billing model id = %s, want selected supplier model %s", ctx.ModelID, routeModelID)
	}
}
