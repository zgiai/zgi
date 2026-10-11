package channelproviders

import (
	"context"
	"testing"

	"github.com/zgiai/zgi/api/internal/capabilities/channelruntime"
	"github.com/zgiai/zgi/api/internal/capabilities/channelruntime/providers/dingtalk"
	"github.com/zgiai/zgi/api/internal/capabilities/channelruntime/providers/feishu"
	providertransport "github.com/zgiai/zgi/api/internal/capabilities/channelruntime/providertransport"
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
		switch providerID {
		case "feishu":
			if _, ok := provider.Receiver.(*feishu.WebSocketReceiver); !ok {
				t.Fatalf("Feishu provider receiver = %T, want *feishu.WebSocketReceiver", provider.Receiver)
			}
		case "dingtalk":
			if _, ok := provider.Receiver.(*dingtalk.StreamReceiver); !ok {
				t.Fatalf("DingTalk provider receiver = %T, want *dingtalk.StreamReceiver", provider.Receiver)
			}
		}
	}
}

type productionConnectionResolverStub struct{}

func (productionConnectionResolverStub) Resolve(context.Context, providertransport.CredentialRequest) (*providertransport.CredentialLease, error) {
	return nil, providertransport.NewError(providertransport.ErrorCodeConnectionNotFound, "connection not found", nil)
}
