package routing

import (
	"testing"

	"github.com/zgiai/zgi/api/internal/contracts"
)

func TestPlannerPrefersConfiguredProvidersBeforeFallback(t *testing.T) {
	planner := NewDefaultPlanner()
	catalog := &contracts.ParseProviderCatalog{
		Providers: []contracts.ParseProviderConfig{
			{Name: "local", Enabled: true, Priority: 1000, FallbackOnly: true, Adapter: "hyperparse_sdk", Engine: contracts.ParseEngineLocal},
			{Name: "vlm", Enabled: true, Priority: 1100, FallbackOnly: true, Adapter: "hyperparse_sdk", Engine: contracts.ParseEngineVLM},
			{Name: "mineru", Enabled: true, Priority: 200, Adapter: "hyperparse_sdk", Engine: contracts.ParseEngineMineru},
			{Name: "reducto", Enabled: true, Priority: 100, Adapter: "hyperparse_sdk", Engine: contracts.ParseEngineReducto},
		},
	}

	plan, err := planner.Plan(contracts.ParseRequest{
		Profile: contracts.ParseProfileHighQuality,
	}, catalog, nil)
	if err != nil {
		t.Fatalf("Plan() error = %v", err)
	}
	if plan.Primary == nil || plan.Primary.ProviderKey != "reducto" {
		t.Fatalf("primary=%+v", plan.Primary)
	}
	if len(plan.FallbackCandidates) == 0 || plan.FallbackCandidates[len(plan.FallbackCandidates)-1].ProviderKey != "vlm" {
		t.Fatalf("fallbacks=%+v", plan.FallbackCandidates)
	}
}

func TestPlannerUsesFallbackChainWhenNoConfiguredProviders(t *testing.T) {
	planner := NewDefaultPlanner()
	catalog := &contracts.ParseProviderCatalog{
		Providers: []contracts.ParseProviderConfig{
			{Name: "local", Enabled: true, Priority: 1000, FallbackOnly: true, Adapter: "hyperparse_sdk", Engine: contracts.ParseEngineLocal},
			{Name: "vlm", Enabled: true, Priority: 1100, FallbackOnly: true, Adapter: "hyperparse_sdk", Engine: contracts.ParseEngineVLM},
		},
	}

	plan, err := planner.Plan(contracts.ParseRequest{
		Profile: contracts.ParseProfileAuto,
	}, catalog, nil)
	if err != nil {
		t.Fatalf("Plan() error = %v", err)
	}
	if plan.Primary == nil || plan.Primary.ProviderKey != "local" {
		t.Fatalf("primary=%+v", plan.Primary)
	}
}

func TestPlannerHonorsLocalFirstProfile(t *testing.T) {
	planner := NewDefaultPlanner()
	catalog := &contracts.ParseProviderCatalog{
		Providers: []contracts.ParseProviderConfig{
			{Name: "local", Enabled: true, Priority: 1000, FallbackOnly: true, Adapter: "hyperparse_sdk", Engine: contracts.ParseEngineLocal},
			{Name: "mineru", Enabled: true, Priority: 200, Adapter: "hyperparse_sdk", Engine: contracts.ParseEngineMineru},
		},
	}

	plan, err := planner.Plan(contracts.ParseRequest{
		Profile: contracts.ParseProfileLocalFirst,
	}, catalog, nil)
	if err != nil {
		t.Fatalf("Plan() error = %v", err)
	}
	if plan.Primary == nil || plan.Primary.ProviderKey != "local" {
		t.Fatalf("primary=%+v", plan.Primary)
	}
	for _, item := range plan.FallbackCandidates {
		if item.ProviderKey == "mineru" {
			t.Fatalf("unexpected remote fallback in local_first plan: %+v", plan.FallbackCandidates)
		}
	}
}

func TestPlannerSkipsUnhealthyAdapters(t *testing.T) {
	planner := NewDefaultPlanner()
	catalog := &contracts.ParseProviderCatalog{
		Providers: []contracts.ParseProviderConfig{
			{Name: "reducto", Enabled: true, Priority: 100, Adapter: "remote_adapter", Engine: contracts.ParseEngineReducto},
			{Name: "mineru", Enabled: true, Priority: 200, Adapter: "healthy_adapter", Engine: contracts.ParseEngineMineru},
			{Name: "local", Enabled: true, Priority: 1000, FallbackOnly: true, Adapter: "healthy_adapter", Engine: contracts.ParseEngineLocal},
		},
	}
	health := &contracts.ParseHealth{
		Adapters: []contracts.AdapterHealth{
			{Name: "remote_adapter", Available: false},
			{Name: "healthy_adapter", Available: true},
		},
	}

	plan, err := planner.Plan(contracts.ParseRequest{
		Profile: contracts.ParseProfileHighQuality,
	}, catalog, health)
	if err != nil {
		t.Fatalf("Plan() error = %v", err)
	}
	if plan.Primary == nil || plan.Primary.ProviderKey != "mineru" {
		t.Fatalf("expected healthy provider primary, got %+v", plan.Primary)
	}
	for _, candidate := range plan.FallbackCandidates {
		if candidate.ProviderKey == "reducto" {
			t.Fatalf("unhealthy provider should not be a fallback: %+v", plan.FallbackCandidates)
		}
	}
}
