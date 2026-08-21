package feishu

import (
	"context"
	"crypto/aes"
	"crypto/cipher"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/google/uuid"
	"github.com/zgiai/zgi/api/internal/capabilities/channelruntime"
	"github.com/zgiai/zgi/api/internal/modules/integrations"
	"github.com/zgiai/zgi/api/internal/modules/tools"
)

const (
	feishuMessageReceiveEvent = "im.message.receive_v1"
	maxWebhookBodyBytes       = 1 << 20
	maxInboundTextRunes       = 32_000
	webhookTimestampSkew      = 5 * time.Minute
)

type channelActionExecutor interface {
	Execute(context.Context, integrations.ActionRequest) (*integrations.ActionResult, error)
}

// ChannelAdapter implements the transport-neutral channel contract while
// delegating outbound side effects to the governed integrations executor.
type ChannelAdapter struct {
	executor channelActionExecutor
	now      func() time.Time
}

func NewChannelAdapter(executor channelActionExecutor) *ChannelAdapter {
	return &ChannelAdapter{executor: executor, now: func() time.Time { return time.Now().UTC() }}
}

func (*ChannelAdapter) Provider() string { return IntegrationID }

func (adapter *ChannelAdapter) Verify(_ context.Context, request channelruntime.WebhookRequest, material channelruntime.SecurityMaterial) (*channelruntime.VerifiedWebhook, error) {
	if adapter == nil || len(request.Body) == 0 || len(request.Body) > maxWebhookBodyBytes || !json.Valid(request.Body) {
		return nil, channelruntime.ErrInvalidWebhook
	}
	var envelope feishuWebhookEnvelope
	if err := json.Unmarshal(request.Body, &envelope); err != nil {
		return nil, channelruntime.ErrInvalidWebhook
	}
	isChallenge := strings.EqualFold(strings.TrimSpace(envelope.Type), "url_verification") || strings.TrimSpace(envelope.Challenge) != ""
	encryptKey := strings.TrimSpace(material.EncryptKey)
	if encryptKey != "" {
		hasSignature := strings.TrimSpace(request.Timestamp) != "" || strings.TrimSpace(request.Nonce) != "" || strings.TrimSpace(request.Signature) != ""
		if hasSignature {
			if err := adapter.verifySignature(request, encryptKey); err != nil {
				return nil, err
			}
		}
		if strings.TrimSpace(envelope.Encrypt) == "" {
			if !isChallenge {
				return nil, channelruntime.ErrInvalidWebhook
			}
		} else {
			decrypted, err := decryptWebhook(envelope.Encrypt, encryptKey)
			if err != nil || len(decrypted) > maxWebhookBodyBytes || !json.Valid(decrypted) {
				return nil, channelruntime.ErrWebhookUnauthorized
			}
			request.Body = decrypted
			if err := json.Unmarshal(decrypted, &envelope); err != nil {
				return nil, channelruntime.ErrInvalidWebhook
			}
			isChallenge = strings.EqualFold(strings.TrimSpace(envelope.Type), "url_verification") || strings.TrimSpace(envelope.Challenge) != ""
		}
	} else if strings.TrimSpace(request.Signature) != "" {
		return nil, channelruntime.ErrWebhookUnauthorized
	}

	verificationToken := strings.TrimSpace(material.VerificationToken)
	if verificationToken == "" || !secureEqual(verificationToken, webhookVerificationToken(request.Body)) {
		return nil, channelruntime.ErrWebhookUnauthorized
	}
	verified := &channelruntime.VerifiedWebhook{Body: append([]byte(nil), request.Body...)}
	if isChallenge {
		verified.Challenge = strings.TrimSpace(envelope.Challenge)
		if verified.Challenge == "" {
			return nil, channelruntime.ErrInvalidWebhook
		}
	}
	return verified, nil
}

func (adapter *ChannelAdapter) verifySignature(request channelruntime.WebhookRequest, encryptKey string) error {
	timestamp := strings.TrimSpace(request.Timestamp)
	nonce := strings.TrimSpace(request.Nonce)
	signature := strings.ToLower(strings.TrimSpace(request.Signature))
	if timestamp == "" || nonce == "" || len(signature) != sha256.Size*2 {
		return channelruntime.ErrWebhookUnauthorized
	}
	seconds, err := strconv.ParseInt(timestamp, 10, 64)
	if err != nil {
		return channelruntime.ErrWebhookUnauthorized
	}
	now := adapter.now()
	requestTime := time.Unix(seconds, 0).UTC()
	if requestTime.Before(now.Add(-webhookTimestampSkew)) || requestTime.After(now.Add(webhookTimestampSkew)) {
		return channelruntime.ErrWebhookUnauthorized
	}
	digest := sha256.New()
	_, _ = digest.Write([]byte(timestamp))
	_, _ = digest.Write([]byte(nonce))
	_, _ = digest.Write([]byte(encryptKey))
	_, _ = digest.Write(request.Body)
	expected := hex.EncodeToString(digest.Sum(nil))
	if !secureEqual(expected, signature) {
		return channelruntime.ErrWebhookUnauthorized
	}
	return nil
}

func (*ChannelAdapter) Normalize(_ context.Context, verified *channelruntime.VerifiedWebhook) (*channelruntime.InboundMessage, error) {
	if verified == nil || verified.Challenge != "" || len(verified.Body) == 0 || len(verified.Body) > maxWebhookBodyBytes {
		return nil, channelruntime.ErrInvalidWebhook
	}
	var event feishuEventV2
	if err := json.Unmarshal(verified.Body, &event); err != nil {
		return nil, channelruntime.ErrInvalidWebhook
	}
	if strings.TrimSpace(event.Header.EventType) != feishuMessageReceiveEvent {
		return nil, channelruntime.ErrUnsupportedEvent
	}
	message := event.Event.Message
	if strings.TrimSpace(event.Header.EventID) == "" || strings.TrimSpace(message.MessageID) == "" || strings.TrimSpace(message.ChatID) == "" {
		return nil, channelruntime.ErrInvalidWebhook
	}
	if strings.TrimSpace(message.MessageType) != "text" {
		return nil, channelruntime.ErrUnsupportedEvent
	}
	conversationType, err := normalizeConversationType(message.ChatType)
	if err != nil {
		return nil, err
	}
	var content struct {
		Text string `json:"text"`
	}
	if json.Unmarshal([]byte(message.Content), &content) != nil || !utf8.ValidString(content.Text) {
		return nil, channelruntime.ErrInvalidWebhook
	}
	mentions := make([]channelruntime.Mention, 0, len(message.Mentions))
	text := content.Text
	for _, mention := range message.Mentions {
		externalID := firstChannelValue(mention.ID.OpenID, mention.ID.UserID, mention.ID.UnionID)
		if externalID == "" {
			continue
		}
		key := strings.TrimSpace(mention.Key)
		mentions = append(mentions, channelruntime.Mention{Key: key, ExternalID: externalID, Name: strings.TrimSpace(mention.Name)})
		if key != "" {
			text = strings.ReplaceAll(text, key, "")
		}
	}
	text = strings.TrimSpace(text)
	if text == "" || utf8.RuneCountInString(text) > maxInboundTextRunes {
		return nil, channelruntime.ErrInvalidWebhook
	}
	occurredAt, err := feishuEventTime(message.CreateTime, event.Header.CreateTime)
	if err != nil {
		return nil, channelruntime.ErrInvalidWebhook
	}
	senderID := firstChannelValue(event.Event.Sender.SenderID.OpenID, event.Event.Sender.SenderID.UserID, event.Event.Sender.SenderID.UnionID)
	if senderID == "" {
		return nil, channelruntime.ErrInvalidWebhook
	}
	return &channelruntime.InboundMessage{
		Provider: IntegrationID, ProviderEventID: strings.TrimSpace(event.Header.EventID),
		ProviderMessageID: strings.TrimSpace(message.MessageID), ExternalTenantID: strings.TrimSpace(event.Header.TenantKey),
		ExternalConversationID: strings.TrimSpace(message.ChatID), ExternalThreadID: firstChannelValue(message.ThreadID, message.RootID, message.ParentID),
		ConversationType: conversationType, SenderExternalID: senderID, Text: text,
		Mentions: mentions, OccurredAt: occurredAt,
	}, nil
}

func (adapter *ChannelAdapter) Deliver(ctx context.Context, request channelruntime.DeliveryRequest) (*channelruntime.DeliveryReceipt, error) {
	if adapter == nil || adapter.executor == nil || request.OrganizationID == uuid.Nil || request.WorkspaceID == uuid.Nil ||
		request.ActorID == uuid.Nil || request.ConnectionID == uuid.Nil || request.RunID == uuid.Nil ||
		request.DeliveryID == uuid.Nil || strings.TrimSpace(request.ExternalConversationID) == "" || strings.TrimSpace(request.Text) == "" {
		return nil, channelruntime.ErrInvalidWebhook
	}
	result, err := adapter.executor.Execute(ctx, integrations.ActionRequest{
		OrganizationID: request.OrganizationID.String(), WorkspaceID: request.WorkspaceID.String(), UserID: request.ActorID.String(),
		ConversationID: request.RunID.String(), MessageID: request.DeliveryID.String(), ConnectionID: request.ConnectionID.String(),
		AppID: request.Subject.ID.String(), OperationItemID: request.DeliveryID.String(), InvokeFrom: tools.ToolInvokeFromAPI,
		IntegrationID: IntegrationID, ActionID: ActionSendBotMessage,
		Input: map[string]interface{}{
			"recipient_type": "chat_id", "recipient_id": strings.TrimSpace(request.ExternalConversationID), "text": strings.TrimSpace(request.Text),
		},
	})
	if err != nil {
		if status, _, handled := integrations.GuardedOperationErrorStatus(err); handled && status == integrations.OperationReceiptStatusOutcomeUnknown {
			return nil, errors.Join(channelruntime.ErrDeliveryOutcomeUnknown, err)
		}
		return nil, err
	}
	if result == nil {
		return nil, channelruntime.ErrDeliveryOutcomeUnknown
	}
	messageID := nestedString(result.Output, "message", "message_id")
	if messageID == "" {
		return nil, channelruntime.ErrDeliveryOutcomeUnknown
	}
	return &channelruntime.DeliveryReceipt{ProviderMessageID: messageID, ProviderRequestID: result.ProviderRequestID, Replayed: result.Replayed}, nil
}

func (*ChannelAdapter) Edit(context.Context, channelruntime.DeliveryRequest, string) (*channelruntime.DeliveryReceipt, error) {
	return nil, channelruntime.ErrUnsupportedEvent
}

func (*ChannelAdapter) Retract(context.Context, channelruntime.DeliveryRequest, string) error {
	return channelruntime.ErrUnsupportedEvent
}

type feishuWebhookEnvelope struct {
	Encrypt   string `json:"encrypt"`
	Challenge string `json:"challenge"`
	Token     string `json:"token"`
	Type      string `json:"type"`
}

type feishuEventV2 struct {
	Schema string `json:"schema"`
	Header struct {
		EventID    string `json:"event_id"`
		EventType  string `json:"event_type"`
		CreateTime string `json:"create_time"`
		Token      string `json:"token"`
		TenantKey  string `json:"tenant_key"`
	} `json:"header"`
	Event struct {
		Sender struct {
			SenderID feishuExternalID `json:"sender_id"`
		} `json:"sender"`
		Message struct {
			MessageID   string          `json:"message_id"`
			RootID      string          `json:"root_id"`
			ParentID    string          `json:"parent_id"`
			ThreadID    string          `json:"thread_id"`
			CreateTime  string          `json:"create_time"`
			ChatID      string          `json:"chat_id"`
			ChatType    string          `json:"chat_type"`
			MessageType string          `json:"message_type"`
			Content     string          `json:"content"`
			Mentions    []feishuMention `json:"mentions"`
		} `json:"message"`
	} `json:"event"`
}

type feishuExternalID struct {
	OpenID  string `json:"open_id"`
	UnionID string `json:"union_id"`
	UserID  string `json:"user_id"`
}

type feishuMention struct {
	Key  string           `json:"key"`
	ID   feishuExternalID `json:"id"`
	Name string           `json:"name"`
}

func webhookVerificationToken(body []byte) string {
	var envelope struct {
		Token  string `json:"token"`
		Header struct {
			Token string `json:"token"`
		} `json:"header"`
	}
	if json.Unmarshal(body, &envelope) != nil {
		return ""
	}
	return firstChannelValue(envelope.Token, envelope.Header.Token)
}

func decryptWebhook(value, encryptKey string) ([]byte, error) {
	encrypted, err := base64.StdEncoding.DecodeString(strings.TrimSpace(value))
	if err != nil || len(encrypted) < aes.BlockSize*2 || len(encrypted[aes.BlockSize:])%aes.BlockSize != 0 {
		return nil, channelruntime.ErrWebhookUnauthorized
	}
	key := sha256.Sum256([]byte(encryptKey))
	block, err := aes.NewCipher(key[:])
	if err != nil {
		return nil, channelruntime.ErrWebhookUnauthorized
	}
	plain := append([]byte(nil), encrypted[aes.BlockSize:]...)
	cipher.NewCBCDecrypter(block, encrypted[:aes.BlockSize]).CryptBlocks(plain, plain)
	plain, err = pkcs7Unpad(plain, aes.BlockSize)
	if err != nil {
		return nil, channelruntime.ErrWebhookUnauthorized
	}
	return plain, nil
}

func pkcs7Unpad(value []byte, blockSize int) ([]byte, error) {
	if len(value) == 0 || len(value)%blockSize != 0 {
		return nil, channelruntime.ErrWebhookUnauthorized
	}
	padding := int(value[len(value)-1])
	if padding < 1 || padding > blockSize || padding > len(value) {
		return nil, channelruntime.ErrWebhookUnauthorized
	}
	for _, item := range value[len(value)-padding:] {
		if int(item) != padding {
			return nil, channelruntime.ErrWebhookUnauthorized
		}
	}
	return value[:len(value)-padding], nil
}

func secureEqual(left, right string) bool {
	leftBytes, rightBytes := []byte(left), []byte(right)
	return len(leftBytes) == len(rightBytes) && subtle.ConstantTimeCompare(leftBytes, rightBytes) == 1
}

func normalizeConversationType(value string) (channelruntime.ConversationType, error) {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "p2p":
		return channelruntime.ConversationTypeDirect, nil
	case "group":
		return channelruntime.ConversationTypeGroup, nil
	default:
		return "", channelruntime.ErrUnsupportedEvent
	}
}

func feishuEventTime(values ...string) (time.Time, error) {
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value == "" {
			continue
		}
		raw, err := strconv.ParseInt(value, 10, 64)
		if err != nil {
			continue
		}
		if raw > 10_000_000_000 {
			return time.UnixMilli(raw).UTC(), nil
		}
		return time.Unix(raw, 0).UTC(), nil
	}
	return time.Time{}, fmt.Errorf("feishu event time is missing")
}

func firstChannelValue(values ...string) string {
	for _, value := range values {
		if value = strings.TrimSpace(value); value != "" {
			return value
		}
	}
	return ""
}

func nestedString(value map[string]interface{}, path ...string) string {
	var current interface{} = value
	for _, segment := range path {
		object, ok := current.(map[string]interface{})
		if !ok {
			return ""
		}
		current = object[segment]
	}
	result, _ := current.(string)
	return strings.TrimSpace(result)
}
