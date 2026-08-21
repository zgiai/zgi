package channelruntime

import (
	"fmt"
	"strings"
	"sync"
)

// Registry is a concurrency-safe, immutable-after-registration adapter catalog.
type Registry struct {
	mu       sync.RWMutex
	adapters map[string]Adapter
}

func NewRegistry(adapters ...Adapter) (*Registry, error) {
	registry := &Registry{adapters: make(map[string]Adapter, len(adapters))}
	for _, adapter := range adapters {
		if err := registry.Register(adapter); err != nil {
			return nil, err
		}
	}
	return registry, nil
}

func (registry *Registry) Register(adapter Adapter) error {
	if registry == nil || adapter == nil {
		return fmt.Errorf("channel adapter is required")
	}
	provider := strings.ToLower(strings.TrimSpace(adapter.Provider()))
	if provider == "" {
		return fmt.Errorf("channel adapter provider is required")
	}
	registry.mu.Lock()
	defer registry.mu.Unlock()
	if _, exists := registry.adapters[provider]; exists {
		return fmt.Errorf("channel adapter %q is already registered", provider)
	}
	registry.adapters[provider] = adapter
	return nil
}

func (registry *Registry) Resolve(provider string) (Adapter, error) {
	if registry == nil {
		return nil, fmt.Errorf("channel adapter registry is unavailable")
	}
	provider = strings.ToLower(strings.TrimSpace(provider))
	registry.mu.RLock()
	adapter := registry.adapters[provider]
	registry.mu.RUnlock()
	if adapter == nil {
		return nil, fmt.Errorf("channel adapter %q is not registered", provider)
	}
	return adapter, nil
}
