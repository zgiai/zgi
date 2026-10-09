package feishu

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/google/uuid"
	"github.com/zgiai/zgi/api/internal/capabilities/channelruntime"
)

func TestChannelAdapterProcessingMarkerAddsAndRemovesAppReaction(t *testing.T) {
	connectionID := uuid.New()
	var addBody map[string]interface{}
	var deletedMessageID, deletedReactionID string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/open-apis/auth/v3/tenant_access_token/internal":
			_, _ = w.Write([]byte(`{"code":0,"tenant_access_token":"tenant-token","expire":7200}`))
		case "/open-apis/im/v1/messages/om-inbound/reactions":
			if r.Method == http.MethodPost {
				if err := json.NewDecoder(r.Body).Decode(&addBody); err != nil {
					t.Fatalf("decode reaction body: %v", err)
				}
				_, _ = w.Write([]byte(`{"code":0,"data":{"reaction_id":"reaction-1"}}`))
				return
			}
			_, _ = w.Write([]byte(`{"code":0,"data":{"items":[{"reaction_id":"reaction-1","operator":{"operator_id":"cli_direct","operator_type":"app"},"reaction_type":{"emoji_type":"Typing"}}],"has_more":false}}`))
		case "/open-apis/im/v1/messages/om-inbound/reactions/reaction-1":
			if r.Method != http.MethodDelete {
				t.Fatalf("reaction cleanup method = %s", r.Method)
			}
			deletedMessageID, deletedReactionID = "om-inbound", "reaction-1"
			_, _ = w.Write([]byte(`{"code":0}`))
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	adapter, err := NewDirectChannelAdapter(newTestAdapter(t, server), &channelConnectionResolverStub{connection: directChannelConnection(connectionID)})
	if err != nil {
		t.Fatal(err)
	}
	request := channelruntime.ProcessingMarkerRequest{
		OrganizationID: uuid.New(), WorkspaceID: uuid.New(), InstallationID: uuid.New(),
		ConnectionID: connectionID, RunID: uuid.New(), InboundMessageID: "om-inbound",
	}
	receipt, err := adapter.AddProcessingMarker(context.Background(), request)
	if err != nil {
		t.Fatalf("AddProcessingMarker() error = %v", err)
	}
	if receipt == nil || receipt.ProviderMarkerID != "reaction-1" {
		t.Fatalf("AddProcessingMarker() receipt = %#v", receipt)
	}
	if got := addBody["reaction_type"].(map[string]interface{})["emoji_type"]; got != feishuProcessingEmoji {
		t.Fatalf("reaction emoji = %v, want %s", got, feishuProcessingEmoji)
	}
	if err := adapter.RemoveProcessingMarker(context.Background(), request, receipt.ProviderMarkerID); err != nil {
		t.Fatalf("RemoveProcessingMarker() error = %v", err)
	}
	if deletedMessageID != "om-inbound" || deletedReactionID != "reaction-1" {
		t.Fatalf("deleted reaction = %s/%s", deletedMessageID, deletedReactionID)
	}
}

func TestChannelAdapterCapabilitiesRequireDirectProviderForProcessingMarker(t *testing.T) {
	if (&ChannelAdapter{}).Capabilities().ProcessingMarker {
		t.Fatal("nil provider must not advertise processing markers")
	}
}
