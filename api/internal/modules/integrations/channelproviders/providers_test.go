package channelproviders

import (
	"context"
	"testing"

	"github.com/zgiai/zgi/api/internal/capabilities/channelruntime"
	"github.com/zgiai/zgi/api/internal/modules/integrations"
)

func TestRegisterProductionIncludesFeishuAndDingTalk(t *testing.T) {
	registry, err := channelruntime.NewRegistry()
	if err != nil {
		t.Fatal(err)
	}
	if err := RegisterProduction(registry, productionConnectionResolverStub{}); err != nil {
		t.Fatalf("RegisterProduction() error = %v", err)
	}
	for _, providerID := range []string{"feishu", "dingtalk"} {
		provider, resolveErr := registry.ResolveProvider(providerID)
		if resolveErr != nil {
			t.Fatalf("ResolveProvider(%q) error = %v", providerID, resolveErr)
		}
		if provider.Adapter.Provider() != providerID || provider.Provisioner == nil {
			t.Fatalf("ResolveProvider(%q) = %#v", providerID, provider)
		}
		if providerID == "dingtalk" && provider.Receiver == nil {
			t.Fatal("DingTalk provider did not register its Stream receiver")
		}
	}
}

type productionConnectionResolverStub struct{}

func (productionConnectionResolverStub) Resolve(context.Context, integrations.ConnectionResolveRequest) (*integrations.ResolvedConnection, error) {
	return nil, integrations.NewError(integrations.ErrorCodeConnectionNotFound, "connection not found", nil)
}
