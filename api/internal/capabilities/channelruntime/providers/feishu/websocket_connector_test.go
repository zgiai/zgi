package feishu

import (
	"testing"

	lark "github.com/larksuite/oapi-sdk-go/v3"
	"github.com/zgiai/zgi/api/internal/capabilities/channelruntime"
)

// Connector-backed Feishu credentials are served on the WebSocket transport;
// normalization must accept them for that mode and keep the region so Lark
// connections dial the Lark endpoint.
func TestFeishuConnectorCredentialsNormalizeForWebSocketTransport(t *testing.T) {
	adapter := &ChannelAdapter{}
	receiver := NewWebSocketReceiver(adapter)
	transport := receiver.Mode(channelruntime.ResolvedChannelCredential{Source: channelruntime.CredentialSourcePersonalConnection})
	if transport != "websocket" {
		t.Fatalf("Connector-backed Feishu mode = %q, want websocket", transport)
	}
	normalized, err := adapter.NormalizeCredentials(map[string]string{
		"region": "lark", "app_id": "cli_connector", "app_secret": "connector-secret",
	}, transport)
	if err != nil {
		t.Fatalf("NormalizeCredentials(websocket) error = %v", err)
	}
	defer destroyCredentialValues(normalized)
	if normalized["app_id"] != "cli_connector" || normalized["app_secret"] != "connector-secret" || normalized["region"] != "lark" {
		t.Fatalf("normalized credentials = %v", normalized)
	}
	if _, err := adapter.NormalizeCredentials(map[string]string{"app_id": "cli_connector"}, transport); err == nil {
		t.Fatal("NormalizeCredentials(websocket) accepted a connection without app_secret")
	}
	if _, err := adapter.NormalizeCredentials(map[string]string{
		"app_id": "cli_connector", "app_secret": "connector-secret", "tenant_key": "unexpected",
	}, transport); err == nil {
		t.Fatal("NormalizeCredentials(websocket) accepted an unsupported credential field")
	}
}

func TestFeishuWebSocketDomainFollowsCredentialRegion(t *testing.T) {
	for region, want := range map[string]string{
		"": lark.FeishuBaseUrl, "feishu": lark.FeishuBaseUrl, RegionCN: lark.FeishuBaseUrl,
		"lark": lark.LarkBaseUrl, RegionGlobal: lark.LarkBaseUrl,
	} {
		got := feishuWebSocketDomain(channelruntime.ResolvedChannelCredential{Values: map[string]string{"region": region}})
		if got != want {
			t.Fatalf("region %q domain = %q, want %q", region, got, want)
		}
	}
}
