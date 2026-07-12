package contentparse

import (
	"context"
	"testing"

	"github.com/zgiai/zgi/api/internal/capabilities/contentparse/routing"
	"github.com/zgiai/zgi/api/internal/contracts"
)

type routingTestAdapter struct {
	gotReq contracts.ParseRequest
}

func (a *routingTestAdapter) Name() string {
	return "routing_test_adapter"
}

func (a *routingTestAdapter) Parse(_ context.Context, req contracts.ParseRequest) (*contracts.ParseArtifact, error) {
	a.gotReq = req
	return &contracts.ParseArtifact{
		SourceType:   req.SourceType,
		SourceRef:    req.SourceRef,
		FileName:     req.FileName,
		Intent:       req.Intent,
		Profile:      req.Profile,
		Status:       contracts.ParseStatusSucceeded,
		QualityLevel: contracts.ParseQualityStandard,
		EngineUsed:   req.EngineHint,
		Markdown:     "routed content",
	}, nil
}

func (a *routingTestAdapter) Health(context.Context) (contracts.AdapterHealth, error) {
	return contracts.AdapterHealth{Name: a.Name(), Available: true}, nil
}

func TestServiceParseWithRoutingUsesPlannerProvider(t *testing.T) {
	adapter := &routingTestAdapter{}
	catalog := &contracts.ParseProviderCatalog{Providers: []contracts.ParseProviderConfig{
		{Name: "local", Enabled: true, Priority: 1000, FallbackOnly: true, Adapter: adapter.Name(), Engine: contracts.ParseEngineLocal},
		{Name: "mineru", Enabled: true, Priority: 200, Adapter: adapter.Name(), Engine: contracts.ParseEngineMineru},
	}}
	orchestrator := NewOrchestrator(NewDefaultStrategyResolver(catalog, adapter.Name()), []ParseAdapter{adapter})
	parser := NewService(orchestrator, routing.NewDefaultPlanner(), catalog).(contracts.RoutedContentParseService)

	artifact, err := parser.ParseWithRouting(context.Background(), contracts.ParseRequest{
		SourceType: contracts.ParseSourceTypeBytes,
		FileName:   "sample.png",
		Data:       []byte("png"),
		Intent:     contracts.ParseIntentDatasetIndex,
		Profile:    contracts.ParseProfileLayoutFirst,
	})
	if err != nil {
		t.Fatalf("ParseWithRouting() error = %v", err)
	}
	if adapter.gotReq.EngineHint != contracts.ParseEngineMineru {
		t.Fatalf("engine hint = %q, want %q", adapter.gotReq.EngineHint, contracts.ParseEngineMineru)
	}
	if artifact == nil || artifact.Metadata["executed_provider_key"] != "mineru" {
		t.Fatalf("artifact metadata = %#v", artifact)
	}
}

func TestServiceParseWithRoutingUsesRequestScopedCatalog(t *testing.T) {
	adapter := &routingTestAdapter{}
	staticCatalog := &contracts.ParseProviderCatalog{Providers: []contracts.ParseProviderConfig{
		{Name: "mineru", Enabled: true, Priority: 100, Adapter: adapter.Name(), Engine: contracts.ParseEngineMineru},
	}}
	dynamicCatalog := &contracts.ParseProviderCatalog{Providers: []contracts.ParseProviderConfig{
		{Name: "local", Enabled: true, Priority: 1000, FallbackOnly: true, Adapter: adapter.Name(), Engine: contracts.ParseEngineLocal},
	}}
	orchestrator := NewOrchestrator(NewDefaultStrategyResolver(staticCatalog, adapter.Name()), []ParseAdapter{adapter})
	service := NewService(orchestrator, routing.NewDefaultPlanner(), staticCatalog).(*Service)
	var resolvedOrganization string
	service.SetProviderCatalogResolver(func(_ context.Context, req contracts.ParseRequest) (*contracts.ParseProviderCatalog, string, error) {
		resolvedOrganization, _ = req.Metadata["organization_id"].(string)
		return dynamicCatalog, "database_merged", nil
	})

	artifact, err := service.ParseWithRouting(context.Background(), contracts.ParseRequest{
		SourceType: contracts.ParseSourceTypeBytes,
		FileName:   "sample.pdf",
		Data:       []byte("pdf"),
		Intent:     contracts.ParseIntentChatContext,
		Profile:    contracts.ParseProfileAuto,
		Metadata: map[string]any{
			"organization_id": "11111111-1111-1111-1111-111111111111",
		},
	})
	if err != nil {
		t.Fatalf("ParseWithRouting() error = %v", err)
	}
	if resolvedOrganization != "11111111-1111-1111-1111-111111111111" {
		t.Fatalf("resolver organization = %q", resolvedOrganization)
	}
	if adapter.gotReq.EngineHint != contracts.ParseEngineLocal {
		t.Fatalf("engine hint = %q, want %q", adapter.gotReq.EngineHint, contracts.ParseEngineLocal)
	}
	if artifact.Metadata["provider_catalog_source"] != "database_merged" {
		t.Fatalf("provider catalog source = %#v", artifact.Metadata["provider_catalog_source"])
	}
	if artifact.Metadata["executed_provider_key"] != "local" {
		t.Fatalf("executed provider = %#v", artifact.Metadata["executed_provider_key"])
	}
}

func TestRuntimeEnvOverridesForCandidate(t *testing.T) {
	catalog := &contracts.ParseProviderCatalog{Providers: []contracts.ParseProviderConfig{
		{
			Name: "mineru",
			Metadata: map[string]any{
				"env_overrides": map[string]any{
					"MINERU_MODE":    "official",
					"MINERU_API_KEY": "secret",
				},
			},
		},
	}}
	overrides := RuntimeEnvOverridesForCandidate(catalog, routing.RouteCandidate{ProviderKey: "mineru"})
	if overrides["MINERU_MODE"] != "official" || overrides["MINERU_API_KEY"] != "secret" {
		t.Fatalf("runtime overrides = %#v", overrides)
	}
}
