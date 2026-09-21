package feishu

import (
	"context"
	"crypto/aes"
	"crypto/cipher"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/zgiai/zgi/api/internal/capabilities/channelruntime"
	providertransport "github.com/zgiai/zgi/api/internal/capabilities/channelruntime/providertransport"
	"github.com/zgiai/zgi/api/internal/capabilities/runtimeidentity"
)

func TestChannelAdapterVerifiesEncryptedSignedEventAndNormalizesMention(t *testing.T) {
	now := time.Date(2026, 8, 19, 10, 30, 0, 0, time.UTC)
	plain := []byte(`{
		"schema":"2.0",
		"header":{"event_id":"event-1","event_type":"im.message.receive_v1","create_time":"1787116200000","token":"verify-token","tenant_key":"tenant-1"},
		"event":{"sender":{"sender_id":{"open_id":"ou_sender"}},"message":{"message_id":"message-1","root_id":"root-1","parent_id":"parent-1","thread_id":"thread-1","create_time":"1787116200000","chat_id":"oc_chat","chat_type":"group","message_type":"text","content":"{\"text\":\"@_user_1 帮我总结一下\"}","mentions":[{"key":"@_user_1","id":{"open_id":"ou_bot"},"name":"数字人"}]}}}`)
	encrypted := encryptWebhookFixture(t, plain, "encrypt-key")
	body, err := json.Marshal(map[string]string{"encrypt": encrypted})
	if err != nil {
		t.Fatal(err)
	}
	timestamp, nonce := strconv.FormatInt(now.Unix(), 10), "nonce-1"
	signature := signatureFixture(timestamp, nonce, "encrypt-key", body)
	adapter := newWebhookTestAdapter()
	adapter.now = func() time.Time { return now }
	verified, err := adapter.Verify(t.Context(), channelruntime.WebhookRequest{
		Body: body, Headers: http.Header{
			"X-Lark-Request-Timestamp": []string{timestamp}, "X-Lark-Request-Nonce": []string{nonce},
			"X-Lark-Signature": []string{signature},
		}, ReceivedAt: now,
	}, channelCredentialFixture("verify-token", "encrypt-key"))
	if err != nil {
		t.Fatalf("Verify() error = %v", err)
	}
	message, err := adapter.Normalize(t.Context(), verified, channelruntime.NormalizationContext{
		Identity: channelruntime.InstallationIdentity{ExternalSelfID: "ou_bot"},
	})
	if err != nil {
		t.Fatalf("Normalize() error = %v", err)
	}
	if message.ProviderEventID != "event-1" || message.ProviderMessageID != "message-1" ||
		message.ConversationType != channelruntime.ConversationTypeGroup || message.Text != "帮我总结一下" ||
		message.RootMessageID != "root-1" || message.ParentMessageID != "parent-1" || message.ThreadID != "thread-1" ||
		len(message.Mentions) != 1 || message.Mentions[0].ExternalID != "ou_bot" || message.WakeReason != channelruntime.WakeReasonMention {
		t.Fatalf("Normalize() = %#v", message)
	}
}

func TestChannelAdapterNormalizesMentionOnlyAsWakeSignal(t *testing.T) {
	body := []byte(`{
		"schema":"2.0",
		"header":{"event_id":"event-wake","event_type":"im.message.receive_v1","create_time":"1787116200000","tenant_key":"tenant-1"},
		"event":{"sender":{"sender_id":{"open_id":"ou_sender"}},"message":{"message_id":"message-wake","create_time":"1787116200000","chat_id":"oc_chat","chat_type":"group","message_type":"text","content":"{\"text\":\"@_user_1\"}","mentions":[{"key":"@_user_1","id":{"open_id":"ou_bot"},"name":"丁康源的智能助手"}]}}}`)

	message, err := newWebhookTestAdapter().Normalize(t.Context(), &channelruntime.VerifiedWebhook{Body: body}, channelruntime.NormalizationContext{
		Identity: channelruntime.InstallationIdentity{ExternalSelfID: "ou_bot"},
	})
	if err != nil {
		t.Fatalf("Normalize() error = %v", err)
	}
	if !message.MentionOnly || message.Text != "" || len(message.Mentions) != 1 || message.Mentions[0].ExternalID != "ou_bot" || message.WakeReason != channelruntime.WakeReasonMention {
		t.Fatalf("Normalize() = %#v, want an attributed mention-only wake", message)
	}
}

func TestChannelAdapterOwnsCredentialMergeAndRejectsUnknownFields(t *testing.T) {
	adapter := newWebhookTestAdapter()
	current := map[string]string{
		"app_id": "cli_existing", "app_secret": "existing-secret", "verification_token": "existing-token",
	}
	prepared, err := adapter.PrepareCredentials(current, map[string]string{
		"app_secret": "", "encrypt_key": "new-encrypt-key",
	})
	if err != nil {
		t.Fatalf("PrepareCredentials() error = %v", err)
	}
	if prepared["app_id"] != "cli_existing" || prepared["app_secret"] != "existing-secret" ||
		prepared["verification_token"] != "existing-token" || prepared["encrypt_key"] != "new-encrypt-key" {
		t.Fatalf("prepared credentials = %#v", prepared)
	}
	destroyCredentialValues(prepared)
	if _, err := adapter.PrepareCredentials(current, map[string]string{"tenant_key": "not-a-credential"}); err == nil {
		t.Fatal("PrepareCredentials() unknown field error = nil")
	}
}

func TestChannelAdapterVerifiesPlainURLChallengeByToken(t *testing.T) {
	body := []byte(`{"challenge":"challenge-1","token":"verify-token","type":"url_verification"}`)
	verified, err := newWebhookTestAdapter().Verify(t.Context(), channelruntime.WebhookRequest{Body: body}, channelCredentialFixture("verify-token", ""))
	if err != nil {
		t.Fatalf("Verify() error = %v", err)
	}
	if verified.ImmediateResponse == nil || !strings.Contains(string(verified.ImmediateResponse.Body), "challenge-1") {
		t.Fatalf("response = %#v", verified.ImmediateResponse)
	}
}

func TestChannelAdapterVerifiesEncryptedURLChallengeWithoutSignatureHeaders(t *testing.T) {
	plain := []byte(`{"challenge":"challenge-1","token":"verify-token","type":"url_verification"}`)
	encrypted := encryptWebhookFixture(t, plain, "encrypt-key")
	body, err := json.Marshal(map[string]string{"encrypt": encrypted})
	if err != nil {
		t.Fatal(err)
	}

	verified, err := newWebhookTestAdapter().Verify(
		t.Context(),
		channelruntime.WebhookRequest{Body: body},
		channelCredentialFixture("verify-token", "encrypt-key"),
	)
	if err != nil {
		t.Fatalf("Verify() error = %v", err)
	}
	if verified.ImmediateResponse == nil || !strings.Contains(string(verified.ImmediateResponse.Body), "challenge-1") {
		t.Fatalf("response = %#v", verified.ImmediateResponse)
	}
}

func TestChannelAdapterRejectsEncryptedPayloadWithWrongTokenWithoutSignatureHeaders(t *testing.T) {
	plain := []byte(`{"challenge":"challenge-1","token":"wrong-token","type":"url_verification"}`)
	encrypted := encryptWebhookFixture(t, plain, "encrypt-key")
	body, err := json.Marshal(map[string]string{"encrypt": encrypted})
	if err != nil {
		t.Fatal(err)
	}

	if _, err := newWebhookTestAdapter().Verify(
		t.Context(),
		channelruntime.WebhookRequest{Body: body},
		channelCredentialFixture("verify-token", "encrypt-key"),
	); err == nil {
		t.Fatal("Verify() invalid token error = nil")
	}
}

func TestChannelAdapterRejectsInvalidSignatureAndToken(t *testing.T) {
	body := []byte(`{"schema":"2.0","header":{"token":"wrong"}}`)
	adapter := newWebhookTestAdapter()
	adapter.now = func() time.Time { return time.Unix(100, 0).UTC() }
	if _, err := adapter.Verify(t.Context(), channelruntime.WebhookRequest{
		Body: body, Headers: http.Header{
			"X-Lark-Request-Timestamp": []string{"100"}, "X-Lark-Request-Nonce": []string{"nonce"},
			"X-Lark-Signature": []string{"invalid"},
		},
	}, channelCredentialFixture("verify-token", "encrypt-key")); err == nil {
		t.Fatal("Verify() invalid signature error = nil")
	}
	if _, err := adapter.Verify(t.Context(), channelruntime.WebhookRequest{Body: body}, channelCredentialFixture("verify-token", "")); err == nil {
		t.Fatal("Verify() invalid token error = nil")
	}
}

func TestRenderFeishuDeliveryTextSkipsInvalidAndDuplicateMentions(t *testing.T) {
	text := renderFeishuDeliveryText("处理完成", []channelruntime.DeliveryMention{
		{ExternalID: "ou_sender", DisplayName: `<康源>`},
		{ExternalID: "ou_sender", DisplayName: "重复"},
		{ExternalID: `bad\" id`, DisplayName: "无效"},
	})
	want := `<at user_id="ou_sender">&lt;康源&gt;</at> 处理完成`
	if text != want {
		t.Fatalf("renderFeishuDeliveryText() = %q, want %q", text, want)
	}
}

func TestDirectChannelAdapterDeliversWithCredentialOnlyConnection(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		writer.Header().Set("Content-Type", "application/json")
		writer.Header().Set("X-Tt-Logid", "request-direct-send")
		switch request.URL.Path {
		case "/open-apis/auth/v3/tenant_access_token/internal":
			_, _ = writer.Write([]byte(`{"code":0,"msg":"success","tenant_access_token":"tenant-token","expire":7200}`))
		case "/open-apis/im/v1/messages":
			if request.Method != http.MethodPost || request.URL.Query().Get("receive_id_type") != "chat_id" || request.Header.Get("Authorization") != "Bearer tenant-token" {
				t.Errorf("unexpected direct send request: %s %s %#v", request.Method, request.URL.String(), request.Header)
			}
			_, _ = writer.Write([]byte(`{"code":0,"msg":"success","data":{"message_id":"om_direct"}}`))
		default:
			http.NotFound(writer, request)
		}
	}))
	defer server.Close()

	connectionID := uuid.New()
	resolver := &channelConnectionResolverStub{connection: directChannelConnection(connectionID)}
	adapter, err := NewDirectChannelAdapter(newTestAdapter(t, server), resolver)
	if err != nil {
		t.Fatal(err)
	}
	receipt, err := adapter.Deliver(t.Context(), channelruntime.DeliveryRequest{
		OrganizationID: uuid.New(), WorkspaceID: uuid.New(), ActorID: uuid.New(), InstallationID: connectionID,
		ConnectionID: uuid.New(), Subject: runtimeidentity.SubjectRef{Type: runtimeidentity.SubjectTypeDigitalHuman, ID: uuid.New()},
		RunID: uuid.New(), DeliveryID: uuid.New(), ExternalConversationID: "oc_direct", Text: "answer",
	})
	if err != nil {
		t.Fatalf("Deliver() error = %v", err)
	}
	if receipt.ProviderMessageID != "om_direct" || receipt.ProviderRequestID != "request-direct-send" || receipt.Replayed {
		t.Fatalf("receipt = %#v", receipt)
	}
	resolved := resolver.lastRequest()
	if resolved.InstallationID != connectionID.String() || resolved.ProviderID != IntegrationID || resolved.DriverID != DriverID || resolved.WorkspaceID == "" {
		t.Fatalf("credential request = %#v", resolved)
	}
}

func TestDirectChannelAdapterReadsContextWithoutActionPolicy(t *testing.T) {
	now := time.Date(2026, 8, 22, 10, 0, 0, 0, time.UTC)
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		writer.Header().Set("Content-Type", "application/json")
		writer.Header().Set("X-Tt-Logid", "request-"+request.URL.Path)
		switch request.URL.Path {
		case "/open-apis/auth/v3/tenant_access_token/internal":
			_, _ = writer.Write([]byte(`{"code":0,"msg":"success","tenant_access_token":"tenant-token","expire":7200}`))
		case "/open-apis/im/v1/messages":
			_, _ = writer.Write([]byte(`{"code":0,"msg":"success","data":{"items":[` +
				`{"message_id":"m3","chat_id":"oc_direct","msg_type":"text","create_time":"1787392800000","sender":{"id":"ou_c","sender_type":"user"},"body":{"content":"{\"text\":\"赏析一下\"}"}},` +
				`{"message_id":"m2","chat_id":"oc_direct","msg_type":"text","create_time":"1787392740000","sender":{"id":"ou_b","sender_type":"user"},"body":{"content":"{\"text\":\"这首诗真棒\"}"}},` +
				`{"message_id":"m1","chat_id":"oc_direct","msg_type":"text","create_time":"1787392680000","sender":{"id":"ou_a","sender_type":"user"},"body":{"content":"{\"text\":\"我写了一首诗\"}"}}` +
				`],"page_token":"","has_more":false}}`))
		case "/open-apis/im/v1/chats/oc_direct/members":
			_, _ = writer.Write([]byte(`{"code":0,"msg":"success","data":{"items":[` +
				`{"member_id":"ou_a","name":"康源"},{"member_id":"ou_b","name":"小王"},{"member_id":"ou_c","name":"小李"}` +
				`],"page_token":"","has_more":false}}`))
		default:
			http.NotFound(writer, request)
		}
	}))
	defer server.Close()

	connectionID := uuid.New()
	resolver := &channelConnectionResolverStub{connection: directChannelConnection(connectionID)}
	adapter, err := NewDirectChannelAdapter(newTestAdapter(t, server), resolver)
	if err != nil {
		t.Fatal(err)
	}
	pack, err := adapter.ReadContext(t.Context(), channelruntime.ChannelContextRequest{
		OrganizationID: uuid.New(), WorkspaceID: uuid.New(), ActorID: uuid.New(), InstallationID: connectionID, ConnectionID: uuid.New(),
		Subject: runtimeidentity.SubjectRef{Type: runtimeidentity.SubjectTypeDigitalHuman, ID: uuid.New()}, RunID: uuid.New(),
		ExternalConversationID: "oc_direct", CurrentMessageID: "m3", CurrentSenderID: "ou_c", OccurredAt: now, Limit: 50,
	})
	if err != nil {
		t.Fatalf("ReadContext() error = %v", err)
	}
	if len(pack.Messages) != 2 || pack.Messages[0].Author.Label != "康源" || pack.Messages[1].Author.Label != "小王" || pack.CurrentAuthor.Label != "小李" {
		t.Fatalf("context pack = %#v", pack)
	}
	if got := resolver.resolveCount(); got != 2 {
		t.Fatalf("credential resolve count = %d, want 2 direct provider operations", got)
	}
}

func TestDirectChannelAdapterTestsGroupCapabilitiesIndependently(t *testing.T) {
	now := time.Date(2026, 8, 29, 8, 0, 0, 0, time.UTC)
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		writer.Header().Set("Content-Type", "application/json")
		switch request.URL.Path {
		case "/open-apis/auth/v3/tenant_access_token/internal":
			_, _ = writer.Write([]byte(`{"code":0,"msg":"success","tenant_access_token":"tenant-token","expire":7200}`))
		case "/open-apis/im/v1/messages":
			_, _ = writer.Write([]byte(`{"code":230027,"msg":"forbidden"}`))
		case "/open-apis/im/v1/chats/oc_capability/members":
			_, _ = writer.Write([]byte(`{"code":0,"msg":"success","data":{"items":[],"page_token":"","has_more":false}}`))
		default:
			http.NotFound(writer, request)
		}
	}))
	defer server.Close()

	connectionID := uuid.New()
	resolver := &channelConnectionResolverStub{connection: directChannelConnection(connectionID)}
	adapter, err := NewDirectChannelAdapter(newTestAdapter(t, server), resolver)
	if err != nil {
		t.Fatal(err)
	}
	organizationID, workspaceID := uuid.New(), uuid.New()
	checks := adapter.TestConversationCapabilities(t.Context(), channelruntime.ConversationCapabilityTestRequest{
		OrganizationID: organizationID, WorkspaceID: workspaceID, ActorID: uuid.New(), InstallationID: uuid.New(),
		ConnectionID: connectionID,
		Subject: runtimeidentity.SubjectRef{
			Type: runtimeidentity.SubjectTypeDigitalHuman, ID: uuid.New(),
			OrganizationID: organizationID, WorkspaceID: workspaceID,
		},
		ExternalConversationID: "oc_capability", CurrentMessageID: "om_current", CurrentSenderID: "ou_sender", OccurredAt: now,
	})
	if len(checks) != 2 {
		t.Fatalf("capability checks = %#v", checks)
	}
	if checks[0].Capability != channelruntime.ConversationCapabilityMessageHistory ||
		checks[0].Status != channelruntime.CapabilityCheckFailed || checks[0].FailureCode != "permission_missing" ||
		len(checks[0].RequiredPermissions) != 1 || checks[0].RequiredPermissions[0] != "im:message:readonly" {
		t.Fatalf("history capability = %#v", checks[0])
	}
	if checks[1].Capability != channelruntime.ConversationCapabilityMemberNames || checks[1].Status != channelruntime.CapabilityCheckPassed {
		t.Fatalf("member capability = %#v", checks[1])
	}
	if got := resolver.resolveCount(); got != 1 {
		t.Fatalf("credential resolve count = %d, want 1", got)
	}
}

func TestContextMessagesPreserveConnectionSelfRelationship(t *testing.T) {
	messages := contextMessagesFromOutput(map[string]interface{}{
		"messages": []interface{}{map[string]interface{}{
			"message_id": "m-self", "sender_id": "cli_app", "sender_is_self": true,
			"message_type": "text", "text": "处理中", "create_time": "1787392800000",
		}},
	})
	if len(messages) != 1 || !messages[0].Author.IsSelf {
		t.Fatalf("messages = %#v, want provider-attributed self sender", messages)
	}
}

func contextMessageOutput(id, rootID, parentID, senderID, text string, occurredAt time.Time) map[string]interface{} {
	return map[string]interface{}{
		"message_id": id, "root_id": rootID, "parent_id": parentID, "thread_id": "", "chat_id": "oc_poem",
		"message_type": "text", "text": text, "sender_id": senderID, "sender_type": "user",
		"create_time": strconv.FormatInt(occurredAt.UnixMilli(), 10), "update_time": "", "deleted": false, "updated": false,
	}
}

type channelConnectionResolverStub struct {
	mu         sync.Mutex
	connection *providertransport.CredentialLease
	requests   []providertransport.CredentialRequest
	err        error
}

func (resolver *channelConnectionResolverStub) Resolve(_ context.Context, request providertransport.CredentialRequest) (*providertransport.CredentialLease, error) {
	resolver.mu.Lock()
	defer resolver.mu.Unlock()
	resolver.requests = append(resolver.requests, request)
	if resolver.err != nil {
		return nil, resolver.err
	}
	if resolver.connection == nil {
		return nil, providertransport.NewError(providertransport.ErrorCodeConnectionNotFound, "connection not found", nil)
	}
	clone := *resolver.connection
	clone.Credentials = make(map[string]string, len(resolver.connection.Credentials))
	for key, value := range resolver.connection.Credentials {
		clone.Credentials[key] = value
	}
	clone.Config = make(map[string]interface{}, len(resolver.connection.Config))
	for key, value := range resolver.connection.Config {
		clone.Config[key] = value
	}
	return &clone, nil
}

func (resolver *channelConnectionResolverStub) lastRequest() providertransport.CredentialRequest {
	resolver.mu.Lock()
	defer resolver.mu.Unlock()
	if len(resolver.requests) == 0 {
		return providertransport.CredentialRequest{}
	}
	return resolver.requests[len(resolver.requests)-1]
}

func (resolver *channelConnectionResolverStub) resolveCount() int {
	resolver.mu.Lock()
	defer resolver.mu.Unlock()
	return len(resolver.requests)
}

func directChannelConnection(id uuid.UUID) *providertransport.CredentialLease {
	return &providertransport.CredentialLease{
		ID: id.String(), ProviderID: IntegrationID, DriverID: DriverID,
		AuthMethodID: TenantAppAuthMethodID,
		Credentials: map[string]string{
			"app_id": "cli_direct", "app_secret": "secret", "verification_token": "verify", "encrypt_key": "encrypt",
		},
		Config: map[string]interface{}{"region": RegionCN},
		// No action grants or declared scopes are required. Feishu is the
		// authority for capability checks when the request is made.
	}
}

func encryptWebhookFixture(t *testing.T, plain []byte, encryptKey string) string {
	t.Helper()
	key := sha256.Sum256([]byte(encryptKey))
	block, err := aes.NewCipher(key[:])
	if err != nil {
		t.Fatal(err)
	}
	padding := aes.BlockSize - len(plain)%aes.BlockSize
	padded := append(append([]byte(nil), plain...), make([]byte, padding)...)
	for index := len(padded) - padding; index < len(padded); index++ {
		padded[index] = byte(padding)
	}
	iv := []byte("0123456789abcdef")
	encrypted := make([]byte, len(padded))
	cipher.NewCBCEncrypter(block, iv).CryptBlocks(encrypted, padded)
	return base64.StdEncoding.EncodeToString(append(append([]byte(nil), iv...), encrypted...))
}

func signatureFixture(timestamp, nonce, encryptKey string, body []byte) string {
	digest := sha256.New()
	_, _ = digest.Write([]byte(timestamp))
	_, _ = digest.Write([]byte(nonce))
	_, _ = digest.Write([]byte(encryptKey))
	_, _ = digest.Write(body)
	return hex.EncodeToString(digest.Sum(nil))
}

func channelCredentialFixture(verificationToken, encryptKey string) channelruntime.ResolvedChannelCredential {
	return channelruntime.ResolvedChannelCredential{Values: map[string]string{
		"verification_token": verificationToken,
		"encrypt_key":        encryptKey,
	}}
}

func newWebhookTestAdapter() *ChannelAdapter {
	return &ChannelAdapter{now: func() time.Time { return time.Now().UTC() }, memberCache: map[string]feishuMemberCacheEntry{}}
}

func newTestAdapter(t *testing.T, server *httptest.Server) *Adapter {
	t.Helper()
	client, err := newClientForBaseURLs(server.Client(), server.URL, server.URL)
	if err != nil {
		t.Fatal(err)
	}
	return &Adapter{client: client}
}
