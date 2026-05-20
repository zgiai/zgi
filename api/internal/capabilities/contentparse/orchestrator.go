package contentparse

import (
	"context"
	"fmt"

	"github.com/zgiai/zgi/api/internal/contracts"
)

type ParseAdapter interface {
	Name() string
	Parse(ctx context.Context, req contracts.ParseRequest) (*contracts.ParseArtifact, error)
	Health(ctx context.Context) (contracts.AdapterHealth, error)
}

type StrategyResolver interface {
	Resolve(req contracts.ParseRequest) (string, contracts.ParseRequest, error)
}

type Orchestrator struct {
	resolver StrategyResolver
	adapters map[string]ParseAdapter
}

func NewOrchestrator(resolver StrategyResolver, adapters []ParseAdapter) *Orchestrator {
	indexed := make(map[string]ParseAdapter, len(adapters))
	for _, adapter := range adapters {
		if adapter == nil {
			continue
		}
		indexed[adapter.Name()] = adapter
	}

	return &Orchestrator{
		resolver: resolver,
		adapters: indexed,
	}
}

func (o *Orchestrator) Parse(ctx context.Context, req contracts.ParseRequest) (*contracts.ParseArtifact, error) {
	if o == nil {
		return nil, fmt.Errorf("content parse orchestrator is not initialized")
	}

	adapterName, normalized, err := o.resolver.Resolve(req)
	if err != nil {
		return nil, err
	}

	adapter, ok := o.adapters[adapterName]
	if !ok {
		return nil, fmt.Errorf("content parse adapter %q is not registered", adapterName)
	}

	artifact, err := adapter.Parse(ctx, normalized)
	if err != nil {
		return nil, err
	}
	return attachRequestMetadata(normalized, artifact), nil
}

func (o *Orchestrator) ParseWithAdapter(ctx context.Context, adapterName string, req contracts.ParseRequest) (*contracts.ParseArtifact, error) {
	if o == nil {
		return nil, fmt.Errorf("content parse orchestrator is not initialized")
	}
	normalized, err := normalizeParseRequest(req)
	if err != nil {
		return nil, err
	}
	adapter, ok := o.adapters[adapterName]
	if !ok {
		return nil, fmt.Errorf("content parse adapter %q is not registered", adapterName)
	}
	artifact, err := adapter.Parse(ctx, normalized)
	if err != nil {
		return nil, err
	}
	return attachRequestMetadata(normalized, artifact), nil
}

func (o *Orchestrator) Health(ctx context.Context) (*contracts.ParseHealth, error) {
	if o == nil {
		return nil, fmt.Errorf("content parse orchestrator is not initialized")
	}

	health := &contracts.ParseHealth{
		Adapters: make([]contracts.AdapterHealth, 0, len(o.adapters)),
	}
	for _, adapter := range o.adapters {
		item, err := adapter.Health(ctx)
		if err != nil {
			return nil, err
		}
		health.Adapters = append(health.Adapters, item)
	}

	return health, nil
}

func attachRequestMetadata(req contracts.ParseRequest, artifact *contracts.ParseArtifact) *contracts.ParseArtifact {
	if artifact == nil {
		return nil
	}
	if artifact.Metadata == nil {
		artifact.Metadata = map[string]any{}
	}
	for key, value := range req.Metadata {
		artifact.Metadata[key] = value
	}
	if artifact.SourceRef == "" {
		artifact.SourceRef = req.SourceRef
	}
	if artifact.FileName == "" {
		artifact.FileName = req.FileName
	}
	if artifact.Intent == "" {
		artifact.Intent = req.Intent
	}
	if artifact.Profile == "" {
		artifact.Profile = req.Profile
	}
	return artifact
}
