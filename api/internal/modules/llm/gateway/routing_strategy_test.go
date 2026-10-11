package gateway

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"
	apikeymodel "github.com/zgiai/zgi/api/internal/modules/llm/apikey/model"
	llmmodel "github.com/zgiai/zgi/api/internal/modules/llm/llmmodel/model"
	providermodel "github.com/zgiai/zgi/api/internal/modules/llm/provider/model"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func TestOrganizationRoutingStrategyReadsCurrentOrganizationSetting(t *testing.T) {
	db, err := gorm.Open(sqlite.Open("file:"+t.Name()+"?mode=memory&cache=shared"), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	if err := db.Exec(`CREATE TABLE organizations (id TEXT PRIMARY KEY, llm_routing_strategy TEXT NOT NULL)`).Error; err != nil {
		t.Fatal(err)
	}
	organizationID := uuid.New()
	if err := db.Exec(`INSERT INTO organizations (id, llm_routing_strategy) VALUES (?, ?)`, organizationID.String(), "price").Error; err != nil {
		t.Fatal(err)
	}

	service := &llmGatewayServiceImpl{db: db}
	if got := service.organizationRoutingStrategy(context.Background(), organizationID); got != "price" {
		t.Fatalf("organizationRoutingStrategy() = %q, want price", got)
	}

	if err := db.Exec(`UPDATE organizations SET llm_routing_strategy = ? WHERE id = ?`, "speed", organizationID.String()).Error; err != nil {
		t.Fatal(err)
	}
	if got := service.organizationRoutingStrategy(context.Background(), organizationID); got != "speed" {
		t.Fatalf("organizationRoutingStrategy() after update = %q, want speed", got)
	}
}

func TestOrganizationRoutingStrategyFallsBackToStability(t *testing.T) {
	service := &llmGatewayServiceImpl{}
	if got := service.organizationRoutingStrategy(context.Background(), uuid.New()); got != "stability" {
		t.Fatalf("organizationRoutingStrategy() = %q, want stability", got)
	}
}

func TestRoutingResolverReadFailureCannotUseRequestStrategy(t *testing.T) {
	ctx := context.WithValue(context.Background(), routingPolicyContextKey{}, routingPolicy{
		Strategy:              routingStrategySpeed,
		EstimatedInputTokens:  123,
		EstimatedOutputTokens: 0,
		Operation:             PricingOperationEmbedding,
	})
	policy := NewRoutingStrategyResolver(nil).Resolve(ctx, uuid.New())
	if policy.Strategy != routingStrategyStability || policy.EstimatedInputTokens != 123 || policy.EstimatedOutputTokens != 0 {
		t.Fatalf("fallback policy = %#v, want stability while preserving workload estimates", policy)
	}
}

func TestBillingContextUsesStrategyResolvedDuringChannelSelection(t *testing.T) {
	service := &llmGatewayServiceImpl{}
	selection := &ProviderSelection{
		OrganizationID:  uuid.New(),
		Provider:        providermodel.LLMProvider{ID: uuid.New(), Provider: "provider-a"},
		Model:           llmmodel.LLMModel{ID: uuid.New(), Model: "model-a"},
		BillingLane:     UsageBillingLanePlatform,
		RoutingStrategy: routingStrategyPrice,
	}
	billingCtx := service.createBillingContext(
		&apikeymodel.TenantAPIKey{ID: "key-a"}, nil, selection, nil,
		selection.OrganizationID, 0, false, time.Now(), "request-a", "request-a-a1",
	)
	if billingCtx.RoutingStrategy != routingStrategyPrice {
		t.Fatalf("billing routing strategy = %q, want the selection snapshot %q", billingCtx.RoutingStrategy, routingStrategyPrice)
	}
}
