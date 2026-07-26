package registry

import (
	"testing"

	"github.com/google/uuid"
	channelmodel "github.com/zgiai/zgi/api/internal/modules/llm/channel/model"
)

func TestResolveIntersectsEveryConfiguredRoute(t *testing.T) {
	registry := NewRegistry()
	routes := []*channelmodel.RouteQueryResult{
		{RouteID: uuid.New(), ChannelProvider: "dashscope", Models: []string{"qwen-image-2.0"}},
		{RouteID: uuid.New(), ChannelProvider: "openai-compatible", Models: []string{"qwen-image-2.0"}},
	}

	got := registry.Resolve("qwen", "qwen-image-2.0", routes)
	if !emptyProfile(got) {
		t.Fatalf("Resolve() = %#v, want empty profile when one route has unknown capabilities", got)
	}
}

func TestResolveKeepsProfileAcrossNativeRoutes(t *testing.T) {
	registry := NewRegistry()
	routes := []*channelmodel.RouteQueryResult{
		{RouteID: uuid.New(), ChannelProvider: "qwen", Models: []string{"qwen-image-2.0"}},
		{RouteID: uuid.New(), ChannelProvider: "dashscope", Models: []string{"qwen-image-2.0"}},
	}

	got := registry.Resolve("qwen", "qwen-image-2.0", routes)
	if got.Quantity == nil || got.Quantity.Mode != QuantityModeExact || got.Quantity.Max != 6 {
		t.Fatalf("Resolve().Quantity = %#v, want exact range 1..6", got.Quantity)
	}
}

func TestResolveUnknownModelReturnsEmptyProfile(t *testing.T) {
	registry := NewRegistry()
	routes := []*channelmodel.RouteQueryResult{
		{RouteID: uuid.New(), ChannelProvider: "openai", Models: []string{"future-image-model"}},
	}

	got := registry.Resolve("openai", "future-image-model", routes)
	if !emptyProfile(got) {
		t.Fatalf("Resolve() = %#v, want empty profile", got)
	}
}

func TestSeedream40UsesSequenceUpperBound(t *testing.T) {
	registry := NewRegistry()
	routes := []*channelmodel.RouteQueryResult{
		{RouteID: uuid.New(), ChannelProvider: "doubao", Models: []string{"doubao-seedream-4-0-250828"}},
	}

	got := registry.Resolve("doubao", "doubao-seedream-4-0-250828", routes)
	if got.Quantity == nil || got.Quantity.Mode != QuantityModeSequence || got.Quantity.Min != 2 || got.Quantity.Max != 15 {
		t.Fatalf("Resolve().Quantity = %#v, want sequence range 2..15", got.Quantity)
	}
}
