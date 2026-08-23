package channelruntime

import (
	"fmt"
	"strings"
	"sync"
)

// Registry is a concurrency-safe, immutable-after-registration adapter catalog.
type Registry struct {
	mu        sync.RWMutex
	providers map[string]Provider
}

// Provider groups the required transport adapter and credential provisioner.
// Optional behavior remains discoverable through narrow capability interfaces
// implemented by Adapter.
type Provider struct {
	Adapter     Adapter
	Provisioner Provisioner
	Receiver    Receiver
}

func NewRegistry(providers ...Provider) (*Registry, error) {
	registry := &Registry{providers: make(map[string]Provider, len(providers))}
	for _, provider := range providers {
		if err := registry.Register(provider); err != nil {
			return nil, err
		}
	}
	return registry, nil
}

func (registry *Registry) Register(provider Provider) error {
	if registry == nil || provider.Adapter == nil || provider.Provisioner == nil {
		return fmt.Errorf("channel provider adapter and provisioner are required")
	}
	providerID := strings.ToLower(strings.TrimSpace(provider.Adapter.Provider()))
	if providerID == "" {
		return fmt.Errorf("channel adapter provider is required")
	}
	registry.mu.Lock()
	defer registry.mu.Unlock()
	if _, exists := registry.providers[providerID]; exists {
		return fmt.Errorf("channel adapter %q is already registered", providerID)
	}
	registry.providers[providerID] = provider
	return nil
}

func (registry *Registry) Resolve(provider string) (Adapter, error) {
	registered, err := registry.ResolveProvider(provider)
	if err != nil {
		return nil, err
	}
	return registered.Adapter, nil
}

func (registry *Registry) ResolveProvider(provider string) (Provider, error) {
	if registry == nil {
		return Provider{}, fmt.Errorf("channel adapter registry is unavailable")
	}
	provider = strings.ToLower(strings.TrimSpace(provider))
	registry.mu.RLock()
	registered, ok := registry.providers[provider]
	registry.mu.RUnlock()
	if !ok {
		return Provider{}, fmt.Errorf("channel adapter %q is not registered", provider)
	}
	return registered, nil
}
