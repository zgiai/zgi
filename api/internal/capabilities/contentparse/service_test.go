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
