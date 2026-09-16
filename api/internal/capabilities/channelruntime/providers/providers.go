// Package channelproviders registers production chat-channel providers without
// leaking provider-specific construction into DigitalHuman orchestration.
package channelproviders

import (
	"fmt"

	"github.com/zgiai/zgi/api/internal/capabilities/channelruntime"
	"github.com/zgiai/zgi/api/internal/capabilities/channelruntime/providers/dingtalk"
	"github.com/zgiai/zgi/api/internal/capabilities/channelruntime/providers/feishu"
	providertransport "github.com/zgiai/zgi/api/internal/capabilities/channelruntime/providertransport"
)

// RegisterProduction installs every production channel provider.
func RegisterProduction(registry *channelruntime.Registry, connections providertransport.CredentialResolver) error {
	if registry == nil || connections == nil {
		return fmt.Errorf("register channel providers: registry and credential resolver are required")
	}
	provider, err := feishu.New(nil)
	if err != nil {
		return fmt.Errorf("initialize Feishu channel provider: %w", err)
	}
	adapter, err := feishu.NewDirectChannelAdapter(provider, connections)
	if err != nil {
		return fmt.Errorf("initialize Feishu channel adapter: %w", err)
	}
	if err := registry.Register(channelruntime.Provider{
		Adapter: adapter, Provisioner: adapter, Receiver: feishu.NewWebSocketReceiver(adapter),
	}); err != nil {
		return fmt.Errorf("register Feishu channel provider: %w", err)
	}
	dingTalkProvider, err := dingtalk.New(nil)
	if err != nil {
		return fmt.Errorf("initialize DingTalk channel provider: %w", err)
	}
	dingTalkAdapter, err := dingtalk.NewDirectChannelAdapter(dingTalkProvider, connections)
	if err != nil {
		return fmt.Errorf("initialize DingTalk channel adapter: %w", err)
	}
	if err := registry.Register(channelruntime.Provider{
		Adapter: dingTalkAdapter, Provisioner: dingTalkAdapter, Receiver: dingtalk.NewStreamReceiver(dingTalkAdapter),
	}); err != nil {
		return fmt.Errorf("register DingTalk channel provider: %w", err)
	}
	return nil
}
