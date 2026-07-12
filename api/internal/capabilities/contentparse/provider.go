package contentparse

import (
	"github.com/zgiai/zgi/api/internal/capabilities/chunking"
	hyperparseapi "github.com/zgiai/zgi/api/internal/capabilities/contentparse/adapters/hyperparse_api"
	hyperparsesdk "github.com/zgiai/zgi/api/internal/capabilities/contentparse/adapters/hyperparse_sdk"
	"github.com/zgiai/zgi/api/internal/capabilities/contentparse/routing"
	"github.com/zgiai/zgi/api/internal/contracts"
)

// Module keeps the new capability easy to instantiate and wire later without
// touching current runtime behavior.
type Module struct {
	Service       contracts.ContentParseService
	RoutedService *Service
	Orchestrator  *Orchestrator
	Resolver      StrategyResolver
	Planner       routing.Planner
	ChunkMapper   contracts.ChunkSourceMapper
	ChunkPlanner  contracts.ChunkPlanner
	SDKAdapter    ParseAdapter
	APIAdapter    ParseAdapter
	Catalog       *contracts.ParseProviderCatalog
}

type ModuleOption func(*moduleOptions)

type moduleOptions struct {
	extraAdapters         []ParseAdapter
	providerOverrides     []contracts.ParseProviderConfig
	figureSummaryEnhancer hyperparsesdk.FigureSummaryEnhancer
}

func WithAdapters(adapters ...ParseAdapter) ModuleOption {
	return func(opts *moduleOptions) {
		opts.extraAdapters = append(opts.extraAdapters, adapters...)
	}
}

func WithProviderOverrides(providers ...contracts.ParseProviderConfig) ModuleOption {
	return func(opts *moduleOptions) {
		opts.providerOverrides = append(opts.providerOverrides, providers...)
	}
}

func WithFigureSummaryEnhancer(enhancer hyperparsesdk.FigureSummaryEnhancer) ModuleOption {
	return func(opts *moduleOptions) {
		opts.figureSummaryEnhancer = enhancer
	}
}

func NewModule(options ...ModuleOption) *Module {
	opts := moduleOptions{}
	for _, option := range options {
		if option != nil {
			option(&opts)
		}
	}

	sdkAdapter := hyperparsesdk.NewAdapterWithFigureSummaryEnhancer(opts.figureSummaryEnhancer)
	apiAdapter := hyperparseapi.NewAdapter()
	catalog := DefaultProviderCatalog()
	for _, provider := range opts.providerOverrides {
		upsertProviderConfig(catalog, provider)
	}
	resolver := NewDefaultStrategyResolver(catalog, sdkAdapter.Name())
	planner := routing.NewDefaultPlanner()
	chunkMapper := chunking.NewCanonicalMapper()
	chunkPlanner := chunking.NewDefaultPlanner()
	adapters := []ParseAdapter{sdkAdapter, apiAdapter}
	adapters = append(adapters, opts.extraAdapters...)
	orchestrator := NewOrchestrator(resolver, adapters)
	service := NewService(orchestrator, planner, catalog)
	routedService, _ := service.(*Service)

	return &Module{
		Service:       service,
		RoutedService: routedService,
		Orchestrator:  orchestrator,
		Resolver:      resolver,
		Planner:       planner,
		ChunkMapper:   chunkMapper,
		ChunkPlanner:  chunkPlanner,
		SDKAdapter:    sdkAdapter,
		APIAdapter:    apiAdapter,
		Catalog:       catalog,
	}
}

func upsertProviderConfig(catalog *contracts.ParseProviderCatalog, provider contracts.ParseProviderConfig) {
	if catalog == nil || provider.Name == "" {
		return
	}
	for i := range catalog.Providers {
		if catalog.Providers[i].Name == provider.Name {
			catalog.Providers[i] = provider
			return
		}
	}
	catalog.Providers = append(catalog.Providers, provider)
}
