package feishu

import (
	"context"
	"crypto/aes"
	"crypto/cipher"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"strconv"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/zgiai/zgi/api/internal/capabilities/channelruntime"
	"github.com/zgiai/zgi/api/internal/capabilities/runtimeidentity"
	"github.com/zgiai/zgi/api/internal/modules/integrations"
	"github.com/zgiai/zgi/api/internal/modules/tools"
)

func TestChannelAdapterVerifiesEncryptedSignedEventAndNormalizesMention(t *testing.T) {
	now := time.Date(2026, 8, 19, 10, 30, 0, 0, time.UTC)
	plain := []byte(`{
		"schema":"2.0",
		"header":{"event_id":"event-1","event_type":"im.message.receive_v1","create_time":"1787116200000","token":"verify-token","tenant_key":"tenant-1"},
		"event":{"sender":{"sender_id":{"open_id":"ou_sender"}},"message":{"message_id":"message-1","root_id":"root-1","create_time":"1787116200000","chat_id":"oc_chat","chat_type":"group","message_type":"text","content":"{\"text\":\"@_user_1 帮我总结一下\"}","mentions":[{"key":"@_user_1","id":{"open_id":"ou_bot"},"name":"数字人"}]}}}`)
	encrypted := encryptWebhookFixture(t, plain, "encrypt-key")
	body, err := json.Marshal(map[string]string{"encrypt": encrypted})
	if err != nil {
		t.Fatal(err)
	}
	timestamp, nonce := strconv.FormatInt(now.Unix(), 10), "nonce-1"
	signature := signatureFixture(timestamp, nonce, "encrypt-key", body)
	adapter := NewChannelAdapter(nil)
	adapter.now = func() time.Time { return now }
	verified, err := adapter.Verify(t.Context(), channelruntime.WebhookRequest{
		Body: body, Timestamp: timestamp, Nonce: nonce, Signature: signature, ReceivedAt: now,
	}, channelruntime.SecurityMaterial{VerificationToken: "verify-token", EncryptKey: "encrypt-key"})
	if err != nil {
		t.Fatalf("Verify() error = %v", err)
	}
	message, err := adapter.Normalize(t.Context(), verified)
	if err != nil {
		t.Fatalf("Normalize() error = %v", err)
	}
	if message.ProviderEventID != "event-1" || message.ProviderMessageID != "message-1" ||
		message.ConversationType != channelruntime.ConversationTypeGroup || message.Text != "帮我总结一下" ||
		len(message.Mentions) != 1 || message.Mentions[0].ExternalID != "ou_bot" {
		t.Fatalf("Normalize() = %#v", message)
	}
}

func TestChannelAdapterVerifiesPlainURLChallengeByToken(t *testing.T) {
	body := []byte(`{"challenge":"challenge-1","token":"verify-token","type":"url_verification"}`)
	verified, err := NewChannelAdapter(nil).Verify(t.Context(), channelruntime.WebhookRequest{Body: body}, channelruntime.SecurityMaterial{VerificationToken: "verify-token"})
	if err != nil {
		t.Fatalf("Verify() error = %v", err)
	}
	if verified.Challenge != "challenge-1" {
		t.Fatalf("challenge = %q", verified.Challenge)
	}
}

func TestChannelAdapterVerifiesEncryptedURLChallengeWithoutSignatureHeaders(t *testing.T) {
	plain := []byte(`{"challenge":"challenge-1","token":"verify-token","type":"url_verification"}`)
	encrypted := encryptWebhookFixture(t, plain, "encrypt-key")
	body, err := json.Marshal(map[string]string{"encrypt": encrypted})
	if err != nil {
		t.Fatal(err)
	}

	verified, err := NewChannelAdapter(nil).Verify(
		t.Context(),
		channelruntime.WebhookRequest{Body: body},
		channelruntime.SecurityMaterial{VerificationToken: "verify-token", EncryptKey: "encrypt-key"},
	)
	if err != nil {
		t.Fatalf("Verify() error = %v", err)
	}
	if verified.Challenge != "challenge-1" {
		t.Fatalf("challenge = %q", verified.Challenge)
	}
}

func TestChannelAdapterRejectsEncryptedPayloadWithWrongTokenWithoutSignatureHeaders(t *testing.T) {
	plain := []byte(`{"challenge":"challenge-1","token":"wrong-token","type":"url_verification"}`)
	encrypted := encryptWebhookFixture(t, plain, "encrypt-key")
	body, err := json.Marshal(map[string]string{"encrypt": encrypted})
	if err != nil {
		t.Fatal(err)
	}

	if _, err := NewChannelAdapter(nil).Verify(
		t.Context(),
		channelruntime.WebhookRequest{Body: body},
		channelruntime.SecurityMaterial{VerificationToken: "verify-token", EncryptKey: "encrypt-key"},
	); err == nil {
		t.Fatal("Verify() invalid token error = nil")
	}
}

func TestChannelAdapterRejectsInvalidSignatureAndToken(t *testing.T) {
	body := []byte(`{"schema":"2.0","header":{"token":"wrong"}}`)
	adapter := NewChannelAdapter(nil)
	adapter.now = func() time.Time { return time.Unix(100, 0).UTC() }
	if _, err := adapter.Verify(t.Context(), channelruntime.WebhookRequest{
		Body: body, Timestamp: "100", Nonce: "nonce", Signature: "invalid",
	}, channelruntime.SecurityMaterial{VerificationToken: "verify-token", EncryptKey: "encrypt-key"}); err == nil {
		t.Fatal("Verify() invalid signature error = nil")
	}
	if _, err := adapter.Verify(t.Context(), channelruntime.WebhookRequest{Body: body}, channelruntime.SecurityMaterial{VerificationToken: "verify-token"}); err == nil {
		t.Fatal("Verify() invalid token error = nil")
	}
}

func TestChannelAdapterDeliverUsesGovernedExecutorAndStableIdentity(t *testing.T) {
	executor := &channelExecutorStub{result: &integrations.ActionResult{
		Output:            map[string]interface{}{"message": map[string]interface{}{"message_id": "om_1"}},
		ProviderRequestID: "request-1", Replayed: true,
	}}
	adapter := NewChannelAdapter(executor)
	request := channelruntime.DeliveryRequest{
		OrganizationID: uuid.New(), WorkspaceID: uuid.New(), ActorID: uuid.New(), InstallationID: uuid.New(),
		ConnectionID: uuid.New(), Subject: runtimeidentity.SubjectRef{Type: runtimeidentity.SubjectTypeDigitalHuman, ID: uuid.New()},
		RunID: uuid.New(), DeliveryID: uuid.New(), ExternalConversationID: "oc_chat", Text: "answer",
	}
	receipt, err := adapter.Deliver(t.Context(), request)
	if err != nil {
		t.Fatalf("Deliver() error = %v", err)
	}
	if receipt.ProviderMessageID != "om_1" || !receipt.Replayed {
		t.Fatalf("receipt = %#v", receipt)
	}
	got := executor.request
	if got.InvokeFrom != tools.ToolInvokeFromAPI || got.ActionID != ActionSendBotMessage ||
		got.ConnectionID != request.ConnectionID.String() || got.MessageID != request.DeliveryID.String() ||
		got.ConversationID != request.RunID.String() || got.AppID != request.Subject.ID.String() ||
		got.Input["recipient_id"] != "oc_chat" || got.Input["text"] != "answer" {
		t.Fatalf("executor request = %#v", got)
	}
}

type channelExecutorStub struct {
	request integrations.ActionRequest
	result  *integrations.ActionResult
	err     error
}

func (executor *channelExecutorStub) Execute(_ context.Context, request integrations.ActionRequest) (*integrations.ActionResult, error) {
	executor.request = request
	return executor.result, executor.err
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
