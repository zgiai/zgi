package channel

import (
	"context"
	"errors"
	"reflect"
	"testing"

	"github.com/google/uuid"
	"github.com/zgiai/zgi/api/internal/modules/llm/catalogvendor"
	channelmodel "github.com/zgiai/zgi/api/internal/modules/llm/channel/model"
	"github.com/zgiai/zgi/api/internal/modules/llm/shared"
)

type hydrationProvider struct {
	channels []*OfficialChannel
	err      error
	calls    int
}

func (p *hydrationProvider) ListChannels(context.Context, string) ([]*OfficialChannel, error) {
	p.calls++
	return p.channels, p.err
}

func TestHydrateOfficialRoutesUsesPlatformProviderModelCatalog(t *testing.T) {
	officialRoute := &channelmodel.LLMRoute{
		Type:                   shared.RouteTypeZGICloud,
		IsOfficial:             true,
		Models:                 []string{"stale-model"},
		OfficialProviderModels: []channelmodel.ProviderModel{{Provider: "old", Model: "stale-model"}},
	}
	privateRoute := &channelmodel.LLMRoute{
		Type:            shared.RouteTypePrivate,
		ChannelProvider: "openai",
		Models:          []string{"private-model"},
	}
	provider := &hydrationProvider{channels: []*OfficialChannel{
		{Provider: "openai", Models: []string{"gpt-5", " shared-model "}, ModelProtocols: map[string][]string{"gpt-5": {"openai_chat_completions", "openai_responses"}, " shared-model ": {"openai_responses"}}},
		{Provider: "anthropic", Models: []string{"claude-4", "shared-model"}, ModelProtocols: map[string][]string{"claude-4": {"anthropic_messages"}, "shared-model": {"anthropic_messages"}}},
	}}

	routes, err := HydrateOfficialRoutes(t.Context(), provider, uuid.New(), []*channelmodel.LLMRoute{officialRoute, privateRoute})
	if err != nil {
		t.Fatalf("HydrateOfficialRoutes() error = %v", err)
	}
	if provider.calls != 1 {
		t.Fatalf("ListChannels calls = %d, want 1", provider.calls)
	}
	if !reflect.DeepEqual(routes[0].Models, []string{"claude-4", "gpt-5", "shared-model"}) {
		t.Fatalf("official models = %#v", routes[0].Models)
	}
	wantPairs := []channelmodel.ProviderModel{
		{Provider: "anthropic", Model: "claude-4"},
		{Provider: "anthropic", Model: "shared-model"},
		{Provider: "openai", Model: "gpt-5"},
		{Provider: "openai", Model: "shared-model"},
	}
	if !reflect.DeepEqual(routes[0].OfficialProviderModels, wantPairs) {
		t.Fatalf("official provider/model pairs = %#v, want %#v", routes[0].OfficialProviderModels, wantPairs)
	}
	if !reflect.DeepEqual(routes[0].OfficialModelProtocols["shared-model"], []string{"openai_responses", "anthropic_messages"}) {
		t.Fatalf("shared model protocols = %#v", routes[0].OfficialModelProtocols["shared-model"])
	}
	if !reflect.DeepEqual(privateRoute.Models, []string{"private-model"}) {
		t.Fatalf("private route models changed = %#v", privateRoute.Models)
	}
}

func TestHydrateOfficialRoutesSkipsPlatformCallWithoutEnabledOfficialRoute(t *testing.T) {
	provider := &hydrationProvider{err: errors.New("must not be called")}
	routes := []*channelmodel.LLMRoute{{Type: shared.RouteTypePrivate, Models: []string{"private-model"}}}

	got, err := HydrateOfficialRoutes(t.Context(), provider, uuid.New(), routes)
	if err != nil {
		t.Fatalf("HydrateOfficialRoutes() error = %v", err)
	}
	if provider.calls != 0 {
		t.Fatalf("ListChannels calls = %d, want 0", provider.calls)
	}
	if !reflect.DeepEqual(got, routes) {
		t.Fatalf("routes changed = %#v", got)
	}
}

func TestHydrateOfficialRoutesReturnsPlatformError(t *testing.T) {
	wantErr := errors.New("platform unavailable")
	provider := &hydrationProvider{err: wantErr}
	routes := []*channelmodel.LLMRoute{{Type: shared.RouteTypeZGICloud, IsOfficial: true}}

	_, err := HydrateOfficialRoutes(t.Context(), provider, uuid.New(), routes)
	if !errors.Is(err, wantErr) {
		t.Fatalf("HydrateOfficialRoutes() error = %v, want %v", err, wantErr)
	}
}

func TestHydrateOfficialRoutesResolvesUnmappedSupplierWithoutChangingPriceIdentity(t *testing.T) {
	for _, tt := range []struct {
		name    string
		entries []catalogvendor.Entry
		want    bool
	}{
		{"unique owner", []catalogvendor.Entry{{Provider: "qwen", Model: "qwen3.8-max", Vendor: "qwen"}}, true},
		{"same owner suppliers", []catalogvendor.Entry{{Provider: "qwen", Model: "qwen3.8-max", Vendor: "qwen"}, {Provider: "supplier-b", Model: "qwen3.8-max", Vendor: "qwen"}}, true},
		{"ambiguous owners", []catalogvendor.Entry{{Provider: "qwen", Model: "qwen3.8-max", Vendor: "qwen"}, {Provider: "other", Model: "qwen3.8-max", Vendor: "other"}}, false},
		{"explicit different owner", []catalogvendor.Entry{{Provider: "qwen", Model: "qwen3.8-max", Vendor: "qwen"}, {Provider: "agicto 供应商", Model: "qwen3.8-max", Vendor: "other"}}, false},
	} {
		t.Run(tt.name, func(t *testing.T) {
			catalogvendor.Replace(tt.entries)
			t.Cleanup(func() { catalogvendor.Replace(nil) })
			official := &channelmodel.LLMRoute{Type: shared.RouteTypeZGICloud, IsOfficial: true}
			private := &channelmodel.LLMRoute{Type: shared.RouteTypePrivate, Models: []string{"private-only"}}
			provider := &hydrationProvider{channels: []*OfficialChannel{{Provider: "agicto 供应商", Models: []string{"qwen3.8-max"}}}}
			_, err := HydrateOfficialRoutes(t.Context(), provider, uuid.New(), []*channelmodel.LLMRoute{official, private})
			if err != nil {
				t.Fatal(err)
			}
			if got := official.SupportsPlatformModel("qwen", "qwen3.8-max"); got != tt.want {
				t.Fatalf("canonical model callable = %v, want %v", got, tt.want)
			}
			if official.SupportsPlatformModel("unrelated-owner", "qwen3.8-max") {
				t.Fatal("unrelated price identity accepted")
			}
			if !reflect.DeepEqual(private.Models, []string{"private-only"}) {
				t.Fatal("private route modified")
			}
			if catalogvendor.Lookup("agicto 供应商", "qwen3.8-max") == "qwen" {
				t.Fatal("hydration must not create a supplier price identity")
			}
		})
	}
}
