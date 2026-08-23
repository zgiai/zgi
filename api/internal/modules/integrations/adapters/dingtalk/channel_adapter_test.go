package dingtalk

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/google/uuid"
	"github.com/zgiai/zgi/api/internal/capabilities/channelruntime"
	"github.com/zgiai/zgi/api/internal/modules/integrations"
)

func TestDingTalkChannelPrepareCredentials(t *testing.T) {
	adapter := &ChannelAdapter{}
	prepared, err := adapter.PrepareCredentials(nil, map[string]string{"app_key": "ding-app", "app_secret": "secret"})
	if err != nil {
		t.Fatalf("PrepareCredentials() error = %v", err)
	}
	if prepared["app_key"] != "ding-app" || prepared["app_secret"] != "secret" {
		t.Fatalf("PrepareCredentials() = %#v", prepared)
	}
	if _, exists := prepared["reception_mode"]; exists {
		t.Fatalf("PrepareCredentials() retained removed reception_mode: %#v", prepared)
	}
	legacyPrepared, err := adapter.PrepareCredentials(map[string]string{
		"app_key": "ding-app", "app_secret": "secret", "reception_mode": "http",
	}, nil)
	if err != nil {
		t.Fatalf("PrepareCredentials(legacy) error = %v", err)
	}
	if _, exists := legacyPrepared["reception_mode"]; exists {
		t.Fatalf("PrepareCredentials(legacy) retained reception_mode: %#v", legacyPrepared)
	}
	if _, err := adapter.PrepareCredentials(prepared, map[string]string{"reception_mode": "http"}); err == nil {
		t.Fatal("PrepareCredentials() accepted removed HTTP reception mode")
	}
	if _, err := adapter.PrepareCredentials(prepared, map[string]string{"verification_token": "unexpected"}); err == nil {
		t.Fatal("PrepareCredentials() accepted an unknown provider field")
	}
}

func TestDingTalkChannelRejectsHTTPWebhook(t *testing.T) {
	adapter := &ChannelAdapter{}
	if _, err := adapter.Verify(t.Context(), channelruntime.WebhookRequest{Body: []byte(`{}`)}, channelruntime.ResolvedChannelCredential{}); err != channelruntime.ErrUnsupportedEvent {
		t.Fatalf("Verify() error = %v, want ErrUnsupportedEvent", err)
	}
}

func TestDingTalkChannelNormalizesStreamGroupMention(t *testing.T) {
	body := []byte(`{
		"conversationId":"cid-group-1","conversationType":"2","msgId":"msg-1","msgtype":"text",
		"createAt":1787479200000,"senderId":"sender-encrypted","senderStaffId":"user-1",
		"senderNick":"康源","chatbotCorpId":"corp-1","chatbotUserId":"bot-1","isInAtList":true,
		"text":{"content":"@数字人 请总结这段内容"}
	}`)
	message, err := (&ChannelAdapter{}).Normalize(t.Context(), &channelruntime.VerifiedWebhook{Body: body}, channelruntime.NormalizationContext{
		Identity: channelruntime.InstallationIdentity{ExternalTenantID: "ding-app", ExternalSelfID: "ding-app"},
	})
	if err != nil {
		t.Fatalf("Normalize() error = %v", err)
	}
	if message.ConversationType != channelruntime.ConversationTypeGroup || message.ExternalConversationID != "group:cid-group-1" ||
		message.WakeReason != channelruntime.WakeReasonMention || message.Text != "请总结这段内容" || message.SenderDisplayName != "康源" {
		t.Fatalf("Normalize() = %#v", message)
	}
}

func TestDingTalkChannelNormalizesDirectConversation(t *testing.T) {
	body := []byte(`{
		"conversationId":"cid-direct-1","conversationType":"1","msgId":"msg-direct","msgtype":"text",
		"createAt":1787479200000,"senderId":"sender-encrypted","senderStaffId":"user-7",
		"senderNick":"小王","isInAtList":false,"text":{"content":"你好"}
	}`)
	message, err := (&ChannelAdapter{}).Normalize(t.Context(), &channelruntime.VerifiedWebhook{Body: body}, channelruntime.NormalizationContext{
		Identity: channelruntime.InstallationIdentity{ExternalTenantID: "ding-app", ExternalSelfID: "ding-app"},
	})
	if err != nil {
		t.Fatalf("Normalize() error = %v", err)
	}
	if message.ConversationType != channelruntime.ConversationTypeDirect || message.ExternalConversationID != "direct:user-7" ||
		message.WakeReason != channelruntime.WakeReasonDirectMessage || message.Text != "你好" {
		t.Fatalf("Normalize() = %#v", message)
	}
}

func TestDingTalkChannelDeliverUsesRobotAPI(t *testing.T) {
	var receivedPath string
	var receivedBody map[string]interface{}
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		switch request.URL.Path {
		case "/v1.0/oauth2/accessToken":
			_ = json.NewEncoder(writer).Encode(map[string]interface{}{"accessToken": "token-1", "expireIn": 7200})
		case "/v1.0/robot/oToMessages/batchSend", "/v1.0/robot/groupMessages/send":
			receivedPath = request.URL.Path
			if request.Header.Get("x-acs-dingtalk-access-token") != "token-1" {
				t.Fatalf("access token header = %q", request.Header.Get("x-acs-dingtalk-access-token"))
			}
			if err := json.NewDecoder(request.Body).Decode(&receivedBody); err != nil {
				t.Fatalf("decode request body: %v", err)
			}
			_ = json.NewEncoder(writer).Encode(map[string]string{"processQueryKey": "query-1"})
		default:
			http.NotFound(writer, request)
		}
	}))
	defer server.Close()
	provider, err := newForBaseURLs(server.Client(), server.URL, server.URL)
	if err != nil {
		t.Fatalf("newForBaseURLs() error = %v", err)
	}
	connectionID := uuid.New()
	adapter, err := NewDirectChannelAdapter(provider, dingTalkChannelConnectionResolverStub{connectionID: connectionID})
	if err != nil {
		t.Fatalf("NewDirectChannelAdapter() error = %v", err)
	}
	receipt, err := adapter.Deliver(t.Context(), channelruntime.DeliveryRequest{
		OrganizationID: uuid.New(), WorkspaceID: uuid.New(), ActorID: uuid.New(), ConnectionID: connectionID,
		RunID: uuid.New(), DeliveryID: uuid.New(), ExternalConversationID: "group:cid-9", Text: "处理完成",
	})
	if err != nil {
		t.Fatalf("Deliver() error = %v", err)
	}
	if receipt.ProviderMessageID != "query-1" || receivedPath != "/v1.0/robot/groupMessages/send" ||
		receivedBody["openConversationId"] != "cid-9" || receivedBody["robotCode"] != "ding-app" {
		t.Fatalf("Deliver() receipt=%#v path=%q body=%#v", receipt, receivedPath, receivedBody)
	}
}

type dingTalkChannelConnectionResolverStub struct {
	connectionID uuid.UUID
}

func (stub dingTalkChannelConnectionResolverStub) Resolve(_ context.Context, request integrations.ConnectionResolveRequest) (*integrations.ResolvedConnection, error) {
	if request.ConnectionID != stub.connectionID.String() || request.IntegrationID != IntegrationID || request.DriverID != DriverID {
		return nil, integrations.NewError(integrations.ErrorCodeConnectionNotFound, "connection not found", nil)
	}
	return &integrations.ResolvedConnection{
		ID: stub.connectionID.String(), IntegrationID: IntegrationID, DriverID: DriverID, AuthMethodID: AuthMethodID,
		CredentialSource: integrations.ConnectionCredentialSourceOrganization,
		Credentials:      map[string]string{"app_key": "ding-app", "app_secret": "secret"},
		Config:           map[string]interface{}{},
	}, nil
}
