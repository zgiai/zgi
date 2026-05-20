package adapter

import (
	"fmt"
	"strings"
)

// AdapterFactory adapter factory interface
type AdapterFactory interface {
	CreateAdapter(config *AdapterConfig) (LLMProviderAdapter, error)
}

// DefaultAdapterFactory default adapter factory
type DefaultAdapterFactory struct {
	constructors map[string]func(*AdapterConfig) (LLMProviderAdapter, error)
}

// NewDefaultAdapterFactory creates a default adapter factory
func NewDefaultAdapterFactory() *DefaultAdapterFactory {
	return &DefaultAdapterFactory{
		constructors: make(map[string]func(*AdapterConfig) (LLMProviderAdapter, error)),
	}
}

// Register registers adapter constructor function
// providerName is the unique identifier of the provider, such as "openai", "deepseek", "openrouter"
func (f *DefaultAdapterFactory) Register(providerName string, constructor func(*AdapterConfig) (LLMProviderAdapter, error)) {
	f.constructors[providerName] = constructor
}

// CreateAdapter creates an adapter instance.
// Adapter selection must come from a single source of truth: ProviderName.
func (f *DefaultAdapterFactory) CreateAdapter(config *AdapterConfig) (LLMProviderAdapter, error) {
	if config == nil {
		return nil, ErrInvalidConfig
	}

	providerName := strings.TrimSpace(config.ProviderName)
	if providerName == "" {
		return nil, fmt.Errorf("%w: provider_name is required", ErrInvalidConfig)
	}

	constructor, exists := f.constructors[providerName]
	if !exists {
		return nil, fmt.Errorf("unsupported provider: %s", providerName)
	}

	return constructor(config)
}

// GlobalFactory global adapter factory instance
var GlobalFactory = NewDefaultAdapterFactory()

// GetConstructors returns all registered constructors (for copying to other factories)
func (f *DefaultAdapterFactory) GetConstructors() map[string]func(*AdapterConfig) (LLMProviderAdapter, error) {
	return f.constructors
}

// NewAdapter creates an adapter using the global factory
func NewAdapter(config *AdapterConfig) (LLMProviderAdapter, error) {
	return GlobalFactory.CreateAdapter(config)
}
