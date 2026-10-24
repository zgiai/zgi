package dingtalk

import (
	"context"
	"encoding/json"
	"errors"
	"github.com/open-dingtalk/dingtalk-stream-sdk-go/chatbot"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/zgiai/zgi/api/internal/capabilities/channelruntime"
	providertransport "github.com/zgiai/zgi/api/internal/capabilities/channelruntime/providertransport"
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

func TestDingTalkChannelNormalizesConnectorCredentialNames(t *testing.T) {
	adapter := &ChannelAdapter{}
	prepared, err := adapter.NormalizeCredentials(map[string]string{
		"client_id": "connector-app", "client_secret": "connector-secret",
	}, "stream")
	if err != nil {
		t.Fatalf("NormalizeCredentials() error = %v", err)
	}
	if prepared["app_key"] != "connector-app" || prepared["app_secret"] != "connector-secret" {
		t.Fatalf("normalized credentials = %#v", prepared)
	}
	if _, ok := prepared["client_id"]; ok {
		t.Fatal("normalized credentials retained client_id")
	}
	destroyDingTalkChannelCredentials(prepared)
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
		OrganizationID: uuid.New(), WorkspaceID: uuid.New(), ActorID: uuid.New(), InstallationID: connectionID, ConnectionID: uuid.New(),
		RunID: uuid.New(), DeliveryID: uuid.New(), ExternalConversationID: "group:cid-9", Text: "处理完成",
		Mentions: []channelruntime.DeliveryMention{{ExternalID: "user-1", DisplayName: "康源"}},
	})
	if err != nil {
		t.Fatalf("Deliver() error = %v", err)
	}
	if receipt.ProviderMessageID != "query-1" || receivedPath != "/v1.0/robot/groupMessages/send" ||
		receivedBody["openConversationId"] != "cid-9" || receivedBody["robotCode"] != "ding-app" {
		t.Fatalf("Deliver() receipt=%#v path=%q body=%#v", receipt, receivedPath, receivedBody)
	}
	atUserIDs, ok := receivedBody["atUserIds"].([]interface{})
	if !ok || len(atUserIDs) != 1 || atUserIDs[0] != "user-1" {
		t.Fatalf("Deliver() atUserIds = %#v, want user-1", receivedBody["atUserIds"])
	}
	var parameters map[string]string
	if err := json.Unmarshal([]byte(receivedBody["msgParam"].(string)), &parameters); err != nil || receivedBody["msgKey"] != "sampleMarkdown" ||
		parameters["text"] != "@user-1 处理完成  " || parameters["title"] != "处理完成" {
		t.Fatalf("Deliver() msgParam = %#v, decoded=%#v error=%v", receivedBody["msgParam"], parameters, err)
	}
}

func TestDingTalkProcessingMarkerIsTheThinkingEmotion(t *testing.T) {
	paths := []string{}
	var lastBody map[string]any
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		if request.URL.Path == "/v1.0/oauth2/accessToken" {
			_ = json.NewEncoder(writer).Encode(map[string]interface{}{"accessToken": "token-1", "expireIn": 7200})
			return
		}
		paths = append(paths, request.URL.Path)
		lastBody = map[string]any{}
		_ = json.NewDecoder(request.Body).Decode(&lastBody)
		_ = json.NewEncoder(writer).Encode(map[string]any{"success": true, "requestId": "req-1"})
	}))
	defer server.Close()
	provider, err := newForBaseURLs(server.Client(), server.URL, server.URL)
	if err != nil {
		t.Fatal(err)
	}
	connectionID := uuid.New()
	adapter, _ := NewDirectChannelAdapter(provider, dingTalkChannelConnectionResolverStub{connectionID: connectionID})
	if !adapter.Capabilities().ProcessingMarker {
		t.Fatal("DingTalk channel has no processing marker")
	}
	request := channelruntime.ProcessingMarkerRequest{
		OrganizationID: uuid.New(), WorkspaceID: uuid.New(), InstallationID: connectionID, ConnectionID: uuid.New(), RunID: uuid.New(),
		InboundMessageID: "msg-1", InboundConversationID: "cid-1",
	}
	receipt, err := adapter.AddProcessingMarker(t.Context(), request)
	if err != nil || receipt.ProviderRequestID != "req-1" {
		t.Fatalf("AddProcessingMarker() = %#v, %v", receipt, err)
	}
	if lastBody["openMsgId"] != "msg-1" || lastBody["openConversationId"] != "cid-1" || lastBody["robotCode"] != "ding-app" ||
		lastBody["emotionName"] != "🤔思考中" {
		t.Fatalf("emotion body = %#v", lastBody)
	}
	if err := adapter.RemoveProcessingMarker(t.Context(), request, ""); err != nil {
		t.Fatalf("RemoveProcessingMarker() = %v", err)
	}
	if strings.Join(paths, ",") != "/v1.0/robot/emotion/reply,/v1.0/robot/emotion/recall" {
		t.Fatalf("emotion calls = %v", paths)
	}
	request.InboundConversationID = ""
	if _, err := adapter.AddProcessingMarker(t.Context(), request); !errors.Is(err, channelruntime.ErrInvalidWebhook) {
		t.Fatalf("a marker without the conversation = %v", err)
	}
}

func TestDingTalkMarkdownKeepsLinesAndFallsBackToText(t *testing.T) {
	body := map[string]interface{}{}
	setDingTalkMessage(body, "## 周报\n- 完成 A\n- 完成 B\n\n```\ncode\n```", "## 周报")
	var parameters map[string]string
	_ = json.Unmarshal([]byte(body["msgParam"].(string)), &parameters)
	if body["msgKey"] != "sampleMarkdown" || parameters["title"] != "周报" ||
		parameters["text"] != "## 周报  \n- 完成 A  \n- 完成 B  \n\n```\ncode\n```" {
		t.Fatalf("markdown = %#v", parameters)
	}
	long := strings.Repeat("长", maxDingTalkMarkdownParamBytes)
	setDingTalkMessage(body, long, long)
	if body["msgKey"] != "sampleText" {
		t.Fatalf("a reply over the Markdown limit = %v", body["msgKey"])
	}
}

func TestDingTalkDirectReplySkippedByDingTalkIsNotDelivered(t *testing.T) {
	for name, skipped := range map[string]map[string]any{
		"rate limited": {"processQueryKey": "q", "flowControlledStaffIdList": []string{"user-1"}},
		"invalid":      {"processQueryKey": "q", "invalidStaffIdList": []string{"user-1"}},
	} {
		server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
			if request.URL.Path == "/v1.0/oauth2/accessToken" {
				_ = json.NewEncoder(writer).Encode(map[string]interface{}{"accessToken": "token-1", "expireIn": 7200})
				return
			}
			_ = json.NewEncoder(writer).Encode(skipped)
		}))
		provider, err := newForBaseURLs(server.Client(), server.URL, server.URL)
		if err != nil {
			t.Fatal(err)
		}
		connectionID := uuid.New()
		adapter, _ := NewDirectChannelAdapter(provider, dingTalkChannelConnectionResolverStub{connectionID: connectionID})
		_, err = adapter.Deliver(t.Context(), channelruntime.DeliveryRequest{
			OrganizationID: uuid.New(), WorkspaceID: uuid.New(), ActorID: uuid.New(), InstallationID: connectionID, ConnectionID: uuid.New(),
			RunID: uuid.New(), DeliveryID: uuid.New(), ExternalConversationID: "direct:user-1", Text: "hi",
		})
		server.Close()
		want := providertransport.ErrorCodeInvalidInput
		if name == "rate limited" {
			want = providertransport.ErrorCodeRateLimited
		}
		if providertransport.ErrorCode(err) != want {
			t.Fatalf("%s: Deliver() = %v, want %s", name, err, want)
		}
	}
}

func TestDingTalkMessageWithoutUserIDNeedsAPublishedBot(t *testing.T) {
	adapter := &ChannelAdapter{}
	body := []byte(`{"conversationId":"cid","conversationType":"1","msgId":"m-1","msgtype":"text","createAt":1787479200000,"senderId":"sender-encrypted","text":{"content":"你好"}}`)
	if _, err := adapter.Normalize(t.Context(), &channelruntime.VerifiedWebhook{Body: body}, channelruntime.NormalizationContext{}); !errors.Is(err, ErrSenderUnpublishedBot) || !errors.Is(err, channelruntime.ErrUnsupportedEvent) {
		t.Fatalf("Normalize() without senderStaffId = %v", err)
	}
}

func TestDingTalkStreamReceiverAcknowledgesWhatItDoesNotTake(t *testing.T) {
	receiver := NewStreamReceiver(&ChannelAdapter{})
	if scope := receiver.LeaseScope(channelruntime.ResolvedChannelCredential{Values: map[string]string{"app_key": " ding-app "}}); scope != "dingtalk:stream:ding-app" {
		t.Fatalf("lease scope = %q", scope)
	}
	called := false
	handle := func(context.Context, *channelruntime.InboundMessage) error { called = true; return nil }
	picture := &chatbot.BotCallbackDataModel{ConversationType: "1", MsgId: "m-1", Msgtype: "picture", SenderStaffId: "user-1", CreateAt: 1787479200000}
	if ack, err := receiver.handleMessage(t.Context(), channelruntime.InstallationIdentity{}, picture, handle); err != nil || string(ack) != "{}" || called {
		t.Fatalf("an unsupported message = %q, %v, handled %v", ack, err, called)
	}
	text := &chatbot.BotCallbackDataModel{ConversationType: "1", MsgId: "m-2", Msgtype: "text", SenderStaffId: "user-1", CreateAt: 1787479200000,
		Text: chatbot.BotCallbackDataTextModel{Content: "你好"}}
	failing := func(context.Context, *channelruntime.InboundMessage) error { return errors.New("pq: secret detail") }
	if _, err := receiver.handleMessage(t.Context(), channelruntime.InstallationIdentity{}, text, failing); err == nil || strings.Contains(err.Error(), "secret") {
		t.Fatalf("a failed store leaked %v", err)
	}
	if mapped := dingTalkReceiverError(errors.New(`{"code":"invalidClientId"}`)); mapped.Code != "credentials_invalid" || mapped.Retryable {
		t.Fatalf("bad credentials = %#v", mapped)
	}
	if mapped := dingTalkReceiverError(errors.New("dial tcp: i/o timeout")); mapped.Code != "connection_failed" || !mapped.Retryable {
		t.Fatalf("a network failure = %#v", mapped)
	}
}

type dingTalkChannelConnectionResolverStub struct {
	connectionID uuid.UUID
}

func (stub dingTalkChannelConnectionResolverStub) Resolve(_ context.Context, request providertransport.CredentialRequest) (*providertransport.CredentialLease, error) {
	if request.InstallationID != stub.connectionID.String() || request.ProviderID != IntegrationID || request.DriverID != DriverID || request.WorkspaceID == "" {
		return nil, providertransport.NewError(providertransport.ErrorCodeConnectionNotFound, "connection not found", nil)
	}
	return &providertransport.CredentialLease{
		ID: stub.connectionID.String(), ProviderID: IntegrationID, DriverID: DriverID, AuthMethodID: AuthMethodID,
		Credentials: map[string]string{"app_key": "ding-app", "app_secret": "secret"},
		Config:      map[string]interface{}{},
	}, nil
}
