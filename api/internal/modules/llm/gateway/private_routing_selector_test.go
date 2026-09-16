package gateway

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/shopspring/decimal"
	channelmodel "github.com/zgiai/zgi/api/internal/modules/llm/channel/model"
	credentialmodel "github.com/zgiai/zgi/api/internal/modules/llm/credential/model"
	llmmodel "github.com/zgiai/zgi/api/internal/modules/llm/llmmodel/model"
	"github.com/zgiai/zgi/api/internal/modules/llm/shared"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func openPrivateRoutingTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(sqlite.Open("file:"+uuid.NewString()+"?mode=memory&cache=shared"), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	statements := []string{
		`CREATE TABLE organizations (id TEXT PRIMARY KEY, llm_routing_strategy TEXT)`,
		`CREATE TABLE llm_usage_bills (
			channel_id TEXT, organization_id TEXT, model_name TEXT, billing_lane TEXT,
			use_system_provider BOOLEAN, status TEXT, response_time_ms INTEGER,
			request_created_at DATETIME
		)`,
		`CREATE TABLE llm_custom_models (
			id TEXT PRIMARY KEY, organization_id TEXT, provider TEXT, name TEXT,
			input_price NUMERIC, output_price NUMERIC,
			input_price_configured BOOLEAN, output_price_configured BOOLEAN,
			pricing TEXT DEFAULT '{}',
			deleted_at DATETIME
		)`,
		`CREATE TABLE llm_models (
			id TEXT PRIMARY KEY, provider TEXT, name TEXT,
			input_price NUMERIC, output_price NUMERIC,
			input_price_configured BOOLEAN, output_price_configured BOOLEAN,
			pricing TEXT DEFAULT '{}',
			deleted_at DATETIME
		)`,
		`CREATE TABLE llm_model_configs (
			organization_id TEXT, model_id TEXT, input_price_override NUMERIC,
			output_price_override NUMERIC, deleted_at DATETIME
		)`,
	}
	for _, statement := range statements {
		if err := db.Exec(statement).Error; err != nil {
			t.Fatal(err)
		}
	}
	return db
}

func privateRoutingRoute(orgID uuid.UUID, provider string, priority, weight int) *channelmodel.LLMRoute {
	return &channelmodel.LLMRoute{
		ID:              uuid.New(),
		OrganizationID:  orgID,
		Type:            shared.RouteTypePrivate,
		ChannelProvider: provider,
		Priority:        priority,
		Weight:          weight,
	}
}

func insertCustomRoutingPrice(t *testing.T, db *gorm.DB, orgID uuid.UUID, provider, model string, input, output float64, inputConfigured, outputConfigured bool) {
	t.Helper()
	pricing := buildTokenPricingJSON(
		decimal.NewFromFloat(input).String(),
		decimal.NewFromFloat(output).String(),
		inputConfigured,
		outputConfigured,
		"", "", "", "",
	)
	if err := db.Exec(`INSERT INTO llm_custom_models
		(id, organization_id, provider, name, input_price, output_price, input_price_configured, output_price_configured, pricing)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)`, uuid.NewString(), orgID.String(), provider, model, input, output, inputConfigured, outputConfigured, pricing).Error; err != nil {
		t.Fatal(err)
	}
}

func insertPrivateRoutingBill(t *testing.T, db *gorm.DB, orgID, routeID uuid.UUID, model, status string, responseMS int64, at time.Time) {
	t.Helper()
	if err := db.Exec(`INSERT INTO llm_usage_bills
		(channel_id, organization_id, model_name, billing_lane, use_system_provider, status, response_time_ms, request_created_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?)`, routeID.String(), orgID.String(), model, UsageBillingLanePrivate, false, status, responseMS, at).Error; err != nil {
		t.Fatal(err)
	}
}

func TestPrivateRoutingPriceUsesEstimatedInputAndOutputAndBeatsWeight(t *testing.T) {
	db := openPrivateRoutingTestDB(t)
	orgID := uuid.New()
	highOutput := privateRoutingRoute(orgID, "provider-a", 10, 1000)
	lowOutput := privateRoutingRoute(orgID, "provider-b", 10, 1)
	insertCustomRoutingPrice(t, db, orgID, "provider-a", "shared-model", 1, 100, true, true)
	insertCustomRoutingPrice(t, db, orgID, "provider-b", "shared-model", 10, 1, true, true)

	router := &ChannelRouter{db: db}
	policy := routingPolicy{Strategy: routingStrategyPrice, EstimatedInputTokens: 100, EstimatedOutputTokens: 1000, Operation: PricingOperationChat}
	selected := router.selectRoutesByRoutingPolicy(context.Background(), orgID, "shared-model", &llmmodel.CustomModel{}, []*channelmodel.LLMRoute{highOutput, lowOutput}, 2, policy)
	if len(selected) != 2 || selected[0].ID != lowOutput.ID {
		t.Fatalf("price order = %v, want lower total-price route first", routeIDs(selected))
	}

	inputOnly := routingPolicy{Strategy: routingStrategyPrice, EstimatedInputTokens: 1000, EstimatedOutputTokens: 0, Operation: PricingOperationEmbedding}
	selected = router.selectRoutesByRoutingPolicy(context.Background(), orgID, "shared-model", &llmmodel.CustomModel{}, []*channelmodel.LLMRoute{highOutput, lowOutput}, 2, inputOnly)
	if selected[0].ID != highOutput.ID {
		t.Fatalf("input-only price order = %v, want lower input-price route first", routeIDs(selected))
	}
}

func TestSelectChannelsForProviderAppliesCurrentOrganizationStrategyBeforeSelection(t *testing.T) {
	db := openPrivateRoutingTestDB(t)
	orgID := uuid.New()
	if err := db.Exec(`INSERT INTO organizations (id, llm_routing_strategy) VALUES (?, ?)`, orgID.String(), routingStrategyPrice).Error; err != nil {
		t.Fatal(err)
	}
	expensive := privateRoutingRoute(orgID, "provider-a", 10, 1000)
	cheap := privateRoutingRoute(orgID, "provider-b", 10, 1)
	for _, route := range []*channelmodel.LLMRoute{expensive, cheap} {
		credentialID := uuid.New()
		route.CredentialID = &credentialID
		route.Models = []string{"shared-model"}
		route.TenantCredential = &credentialmodel.TenantCredential{
			ID:               credentialID,
			OrganizationID:   orgID,
			ChannelProvider:  route.ChannelProvider,
			APIKeyCiphertext: "test-key",
			IsActive:         true,
		}
	}
	insertCustomRoutingPrice(t, db, orgID, "provider-a", "shared-model", 20, 20, true, true)
	insertCustomRoutingPrice(t, db, orgID, "provider-b", "shared-model", 1, 1, true, true)
	privateModel := &llmmodel.CustomModel{
		ID:              uuid.New(),
		OrganizationID:  orgID,
		ProviderID:      uuid.New(),
		Provider:        "provider-a",
		Name:            "shared-model",
		DisplayName:     "Shared Model",
		IsActive:        true,
		ChatCompletions: true,
	}
	router := &ChannelRouter{
		db:                      db,
		organizationIDRouteRepo: &fakeCandidateRouteRepo{routes: []*channelmodel.LLMRoute{expensive, cheap}},
		cryptoService:           identityCryptoService{},
		strategyFactory:         NewStrategyFactory(),
		privateModels:           &fakePrivateModelLookup{model: privateModel},
		routingResolver:         NewRoutingStrategyResolver(db),
	}
	ctx := withRoutingTokenEstimate(context.Background(), 1000, 1000, PricingOperationChat)
	selections, err := router.SelectChannelsForProvider(ctx, orgID, "", "shared-model", 2)
	if err != nil {
		t.Fatal(err)
	}
	if len(selections) != 2 || selections[0].RouteID != cheap.ID {
		t.Fatalf("selected routes = %v, want cheap route first", channelSelectionRouteIDs(selections))
	}
	for _, selection := range selections {
		if selection.RoutingStrategy != routingStrategyPrice {
			t.Fatalf("selection strategy = %q, want price", selection.RoutingStrategy)
		}
	}
}

func TestPrivateRoutingPriceAllowsMissingOutputForEmbeddingAndNeverTreatsMissingPriceAsFree(t *testing.T) {
	db := openPrivateRoutingTestDB(t)
	orgID := uuid.New()
	priced := privateRoutingRoute(orgID, "priced", 1, 1)
	missing := privateRoutingRoute(orgID, "missing", 1, 1000)
	insertCustomRoutingPrice(t, db, orgID, "priced", "embed-model", 2, 0, true, false)
	insertCustomRoutingPrice(t, db, orgID, "missing", "embed-model", 0, 0, false, false)

	router := &ChannelRouter{db: db}
	policy := routingPolicy{Strategy: routingStrategyPrice, EstimatedInputTokens: 500, EstimatedOutputTokens: 0, Operation: PricingOperationEmbedding}
	selected := router.selectRoutesByRoutingPolicy(context.Background(), orgID, "embed-model", &llmmodel.CustomModel{}, []*channelmodel.LLMRoute{missing, priced}, 2, policy)
	if selected[0].ID != priced.ID {
		t.Fatalf("embedding price order = %v, want configured input price first", routeIDs(selected))
	}
}

func TestPrivateRoutingPriceTieUsesStabilityBeforeWeight(t *testing.T) {
	db := openPrivateRoutingTestDB(t)
	now := time.Now().UTC()
	orgID := uuid.New()
	stable := privateRoutingRoute(orgID, "stable", 1, 1)
	unstable := privateRoutingRoute(orgID, "unstable", 1, 1000)
	insertCustomRoutingPrice(t, db, orgID, "stable", "chat-model", 1, 2, true, true)
	insertCustomRoutingPrice(t, db, orgID, "unstable", "chat-model", 1, 2, true, true)
	for i := 0; i < 5; i++ {
		insertPrivateRoutingBill(t, db, orgID, stable.ID, "chat-model", usageBillStatusSuccess, 200, now.Add(-time.Duration(i)*time.Minute))
		insertPrivateRoutingBill(t, db, orgID, unstable.ID, "chat-model", usageBillStatusFailed, 100, now.Add(-time.Duration(i)*time.Minute))
	}

	router := &ChannelRouter{db: db, now: func() time.Time { return now }}
	policy := routingPolicy{Strategy: routingStrategyPrice, EstimatedInputTokens: 1000, EstimatedOutputTokens: 1000}
	selected := router.selectRoutesByRoutingPolicy(context.Background(), orgID, "chat-model", &llmmodel.CustomModel{}, []*channelmodel.LLMRoute{unstable, stable}, 2, policy)
	if selected[0].ID != stable.ID {
		t.Fatalf("equal-price order = %v, want stability before weight", routeIDs(selected))
	}
}

func TestPrivateRoutingStabilityUsesOrganizationChannelAndModelIsolatedHistory(t *testing.T) {
	db := openPrivateRoutingTestDB(t)
	now := time.Now().UTC()
	orgID := uuid.New()
	stable := privateRoutingRoute(orgID, "stable", 1, 1)
	unstable := privateRoutingRoute(orgID, "unstable", 1, 1000)
	for i := 0; i < 10; i++ {
		insertPrivateRoutingBill(t, db, orgID, stable.ID, "chat-model", usageBillStatusSuccess, 300, now.Add(-time.Duration(i)*time.Minute))
		status := usageBillStatusFailed
		if i >= 6 {
			status = usageBillStatusSuccess
		}
		insertPrivateRoutingBill(t, db, orgID, unstable.ID, "chat-model", status, 100, now.Add(-time.Duration(i)*time.Minute))
	}
	// These tempting records must not contaminate the selected organization/model.
	insertPrivateRoutingBill(t, db, uuid.New(), unstable.ID, "chat-model", usageBillStatusSuccess, 1, now)
	insertPrivateRoutingBill(t, db, orgID, unstable.ID, "other-model", usageBillStatusSuccess, 1, now)

	router := &ChannelRouter{db: db, now: func() time.Time { return now }}
	selected := router.selectRoutesByRoutingPolicy(context.Background(), orgID, "chat-model", nil, []*channelmodel.LLMRoute{unstable, stable}, 2, routingPolicy{Strategy: routingStrategyStability})
	if selected[0].ID != stable.ID {
		t.Fatalf("stability order = %v, want higher-success route first", routeIDs(selected))
	}
	if score := routingStabilityScore(nil); score != 0.5 {
		t.Fatalf("no-history stability score = %v, want neutral 0.5", score)
	}
}

func TestPrivateRoutingSpeedRequiresSamplesAndTrimsSingleOutlier(t *testing.T) {
	db := openPrivateRoutingTestDB(t)
	now := time.Now().UTC()
	orgID := uuid.New()
	fast := privateRoutingRoute(orgID, "fast", 1, 1)
	unknown := privateRoutingRoute(orgID, "unknown", 1, 1)
	slow := privateRoutingRoute(orgID, "slow", 1, 1)
	for i, duration := range []int64{100, 100, 100, 100, 10_000} {
		insertPrivateRoutingBill(t, db, orgID, fast.ID, "chat-model", usageBillStatusSuccess, duration, now.Add(-time.Duration(i)*time.Minute))
	}
	for i := 0; i < 5; i++ {
		insertPrivateRoutingBill(t, db, orgID, slow.ID, "chat-model", usageBillStatusSuccess, 900, now.Add(-time.Duration(i)*time.Minute))
	}
	// One very fast sample remains neutral because it is below the minimum sample count.
	insertPrivateRoutingBill(t, db, orgID, unknown.ID, "chat-model", usageBillStatusSuccess, 1, now)

	router := &ChannelRouter{db: db, now: func() time.Time { return now }}
	selected := router.selectRoutesByRoutingPolicy(context.Background(), orgID, "chat-model", nil, []*channelmodel.LLMRoute{slow, unknown, fast}, 3, routingPolicy{Strategy: routingStrategySpeed})
	if selected[0].ID != fast.ID || selected[2].ID != slow.ID {
		t.Fatalf("speed order = %v, want fast, neutral, slow", routeIDs(selected))
	}
}

func TestPrivateRoutingKeepsPriorityProbeAndMaxSelections(t *testing.T) {
	db := openPrivateRoutingTestDB(t)
	orgID := uuid.New()
	highPriorityExpensive := privateRoutingRoute(orgID, "high", 100, 1)
	lowPriorityCheap := privateRoutingRoute(orgID, "low", 1, 1000)
	probe := privateRoutingRoute(orgID, "probe", 0, 1)
	probe.UpstreamProbe = true
	insertCustomRoutingPrice(t, db, orgID, "high", "chat-model", 100, 100, true, true)
	insertCustomRoutingPrice(t, db, orgID, "low", "chat-model", 1, 1, true, true)
	insertCustomRoutingPrice(t, db, orgID, "probe", "chat-model", 1000, 1000, true, true)

	router := &ChannelRouter{db: db}
	selected := router.selectRoutesByRoutingPolicy(context.Background(), orgID, "chat-model", &llmmodel.CustomModel{}, []*channelmodel.LLMRoute{lowPriorityCheap, highPriorityExpensive, probe}, 2, routingPolicy{Strategy: routingStrategyPrice, EstimatedInputTokens: 1000, EstimatedOutputTokens: 1000})
	if len(selected) != 2 || selected[0].ID != probe.ID || selected[1].ID != highPriorityExpensive.ID {
		t.Fatalf("probe/priority/max order = %v, want probe then high priority and max 2", routeIDs(selected))
	}
}

func TestRoutingResolverOverridesRequestContextAndReadsUpdatesImmediately(t *testing.T) {
	db := openPrivateRoutingTestDB(t)
	orgID := uuid.New()
	if err := db.Exec(`INSERT INTO organizations (id, llm_routing_strategy) VALUES (?, ?)`, orgID.String(), routingStrategyStability).Error; err != nil {
		t.Fatal(err)
	}
	resolver := NewRoutingStrategyResolver(db)
	ctx := context.WithValue(context.Background(), routingPolicyContextKey{}, routingPolicy{Strategy: routingStrategySpeed, EstimatedInputTokens: 321, EstimatedOutputTokens: 0, Operation: PricingOperationEmbedding})
	first := resolver.Resolve(ctx, orgID)
	if first.Strategy != routingStrategyStability || first.EstimatedInputTokens != 321 || first.EstimatedOutputTokens != 0 {
		t.Fatalf("resolved policy = %#v, want organization stability with request estimates", first)
	}
	if err := db.Exec(`UPDATE organizations SET llm_routing_strategy = ? WHERE id = ?`, routingStrategyPrice, orgID.String()).Error; err != nil {
		t.Fatal(err)
	}
	if second := resolver.Resolve(ctx, orgID); second.Strategy != routingStrategyPrice {
		t.Fatalf("updated strategy = %q, want price", second.Strategy)
	}
}

func routeIDs(routes []*channelmodel.LLMRoute) []uuid.UUID {
	ids := make([]uuid.UUID, 0, len(routes))
	for _, route := range routes {
		ids = append(ids, route.ID)
	}
	return ids
}

func channelSelectionRouteIDs(selections []*ChannelSelection) []uuid.UUID {
	ids := make([]uuid.UUID, 0, len(selections))
	for _, selection := range selections {
		ids = append(ids, selection.RouteID)
	}
	return ids
}
