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
	"html"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"sync"
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

// ChannelAdapter implements the transport-neutral channel contract. DigitalHuman
// production traffic resolves request-scoped credentials and calls Feishu
// directly; executor is retained only for compatibility with legacy callers.
type ChannelAdapter struct {
	executor      channelActionExecutor
	provider      *Adapter
	connections   integrations.ConnectionResolver
	now           func() time.Time
	memberCacheMu sync.Mutex
	memberCache   map[string]feishuMemberCacheEntry
}

func NewChannelAdapter(executor channelActionExecutor) *ChannelAdapter {
	return &ChannelAdapter{
		executor:    executor,
		now:         func() time.Time { return time.Now().UTC() },
		memberCache: make(map[string]feishuMemberCacheEntry),
	}
}

// NewDirectChannelAdapter creates the DigitalHuman channel adapter without the
// generic integration executor. The connection resolver is used only as a
// request-scoped credential store; provider calls, retries, and failures are
// handled directly by the Feishu adapter.
func NewDirectChannelAdapter(provider *Adapter, connections integrations.ConnectionResolver) (*ChannelAdapter, error) {
	if provider == nil || provider.client == nil || connections == nil {
		return nil, fmt.Errorf("initialize direct Feishu channel adapter: provider and credential resolver are required")
	}
	return &ChannelAdapter{
		provider:    provider,
		connections: connections,
		now:         func() time.Time { return time.Now().UTC() },
		memberCache: make(map[string]feishuMemberCacheEntry),
	}, nil
}

func (*ChannelAdapter) Provider() string { return IntegrationID }

func (adapter *ChannelAdapter) Capabilities() channelruntime.ProviderCapabilities {
	return channelruntime.ProviderCapabilities{
		ContextRead:                true,
		ProcessingMarker:           adapter.directProviderAvailable(),
		MessageRoundTripTest:       true,
		ConversationCapabilityTest: true,
	}
}

const feishuProcessingEmoji = "Typing"

type feishuMessageReaction struct {
	ReactionID string `json:"reaction_id"`
	Operator   struct {
		OperatorID   string `json:"operator_id"`
		OperatorType string `json:"operator_type"`
	} `json:"operator"`
	ReactionType struct {
		EmojiType string `json:"emoji_type"`
	} `json:"reaction_type"`
}

func (adapter *ChannelAdapter) AddProcessingMarker(ctx context.Context, request channelruntime.ProcessingMarkerRequest) (*channelruntime.ProcessingMarkerReceipt, error) {
	if !adapter.directProviderAvailable() || request.OrganizationID == uuid.Nil || request.ConnectionID == uuid.Nil ||
		request.RunID == uuid.Nil || strings.TrimSpace(request.InboundMessageID) == "" {
		return nil, channelruntime.ErrInvalidWebhook
	}
	connection, cleanup, err := adapter.resolveDirectConnection(ctx, request.OrganizationID, request.ConnectionID)
	if err != nil {
		return nil, err
	}
	defer cleanup()
	region, err := feishuRegion(connection)
	if err != nil {
		return nil, err
	}
	token, tokenMeta, err := adapter.provider.tenantAccessToken(ctx, connection, region)
	if err != nil {
		return nil, err
	}
	var reaction feishuMessageReaction
	meta, err := adapter.provider.client.addMessageReaction(ctx, region, token, strings.TrimSpace(request.InboundMessageID), map[string]any{
		"reaction_type": map[string]string{"emoji_type": feishuProcessingEmoji},
	}, &reaction)
	if err != nil {
		return nil, err
	}
	if strings.TrimSpace(reaction.ReactionID) == "" {
		return nil, integrations.NewError(integrations.ErrorCodeResponseInvalid, "Feishu reaction response is incomplete", nil)
	}
	return &channelruntime.ProcessingMarkerReceipt{
		ProviderMarkerID:  strings.TrimSpace(reaction.ReactionID),
		ProviderRequestID: firstNonEmpty(meta.RequestID, tokenMeta.RequestID),
	}, nil
}

func (adapter *ChannelAdapter) RemoveProcessingMarker(ctx context.Context, request channelruntime.ProcessingMarkerRequest, providerMarkerID string) error {
	if !adapter.directProviderAvailable() || request.OrganizationID == uuid.Nil || request.ConnectionID == uuid.Nil ||
		request.RunID == uuid.Nil || strings.TrimSpace(request.InboundMessageID) == "" {
		return channelruntime.ErrInvalidWebhook
	}
	connection, cleanup, err := adapter.resolveDirectConnection(ctx, request.OrganizationID, request.ConnectionID)
	if err != nil {
		return err
	}
	defer cleanup()
	region, err := feishuRegion(connection)
	if err != nil {
		return err
	}
	token, _, err := adapter.provider.tenantAccessToken(ctx, connection, region)
	if err != nil {
		return err
	}
	appID := feishuConnectionAppID(connection)
	pageToken := ""
	for page := 0; page < 10; page++ {
		query := url.Values{"reaction_type": []string{feishuProcessingEmoji}, "page_size": []string{"50"}}
		if pageToken != "" {
			query.Set("page_token", pageToken)
		}
		var response struct {
			Items     []feishuMessageReaction `json:"items"`
			HasMore   bool                    `json:"has_more"`
			PageToken string                  `json:"page_token"`
		}
		if _, err := adapter.provider.client.listMessageReactions(ctx, region, token, strings.TrimSpace(request.InboundMessageID), query, &response); err != nil {
			return err
		}
		for _, reaction := range response.Items {
			ownedByApp := strings.EqualFold(strings.TrimSpace(reaction.Operator.OperatorType), "app") &&
				strings.TrimSpace(reaction.Operator.OperatorID) == appID
			matchesSavedID := strings.TrimSpace(providerMarkerID) != "" && strings.TrimSpace(reaction.ReactionID) == strings.TrimSpace(providerMarkerID)
			if !matchesSavedID && !ownedByApp {
				continue
			}
			if _, err := adapter.provider.client.deleteMessageReaction(
				ctx, region, token, strings.TrimSpace(request.InboundMessageID), strings.TrimSpace(reaction.ReactionID), nil,
			); err != nil {
				return err
			}
		}
		if !response.HasMore || strings.TrimSpace(response.PageToken) == "" {
			return nil
		}
		pageToken = strings.TrimSpace(response.PageToken)
	}
	return integrations.NewError(integrations.ErrorCodeResponseInvalid, "Feishu reaction pagination did not terminate", nil)
}

func (*ChannelAdapter) ConnectionSpec() channelruntime.ConnectionSpec {
	return channelruntime.ConnectionSpec{
		IntegrationID: IntegrationID,
		DriverID:      DriverID,
		AuthMethodID:  TenantAppAuthMethodID,
	}
}

func (*ChannelAdapter) PrepareCredentials(current, updates map[string]string) (map[string]string, error) {
	prepared := make(map[string]string, len(current)+len(updates))
	for key, value := range current {
		key = strings.ToLower(strings.TrimSpace(key))
		if !isChannelCredentialKey(key) {
			destroyCredentialValues(prepared)
			return nil, integrations.NewError(integrations.ErrorCodeInvalidInput, "unsupported Feishu channel credential field", nil)
		}
		if value = strings.TrimSpace(value); value != "" {
			prepared[key] = value
		}
	}
	for key, value := range updates {
		key = strings.ToLower(strings.TrimSpace(key))
		if !isChannelCredentialKey(key) {
			destroyCredentialValues(prepared)
			return nil, integrations.NewError(integrations.ErrorCodeInvalidInput, "unsupported Feishu channel credential field", nil)
		}
		if value = strings.TrimSpace(value); value != "" {
			prepared[key] = value
		}
	}
	if prepared["app_id"] == "" || prepared["app_secret"] == "" || prepared["verification_token"] == "" {
		destroyCredentialValues(prepared)
		return nil, integrations.NewError(integrations.ErrorCodeInvalidInput, "Feishu App ID, App Secret, and Verification Token are required", nil)
	}
	return prepared, nil
}

func (adapter *ChannelAdapter) ResolveIdentity(ctx context.Context, credential channelruntime.ResolvedChannelCredential) (channelruntime.InstallationIdentity, error) {
	if adapter == nil || adapter.provider == nil || len(credential.Values) == 0 {
		return channelruntime.InstallationIdentity{}, integrations.NewError(integrations.ErrorCodeConnectionInvalid, "Feishu channel credentials are unavailable", nil)
	}
	connection := resolvedConnectionFromChannelCredential(credential)
	defer connection.Destroy()
	tenantID, selfID, err := adapter.provider.ResolveChannelIdentity(ctx, connection)
	if err != nil {
		return channelruntime.InstallationIdentity{}, err
	}
	return channelruntime.InstallationIdentity{
		ExternalTenantID: strings.TrimSpace(tenantID),
		ExternalSelfID:   strings.TrimSpace(selfID),
	}, nil
}

func (adapter *ChannelAdapter) Verify(_ context.Context, request channelruntime.WebhookRequest, credential channelruntime.ResolvedChannelCredential) (*channelruntime.VerifiedWebhook, error) {
	if adapter == nil || len(request.Body) == 0 || len(request.Body) > maxWebhookBodyBytes || !json.Valid(request.Body) {
		return nil, channelruntime.ErrInvalidWebhook
	}
	var envelope feishuWebhookEnvelope
	if err := json.Unmarshal(request.Body, &envelope); err != nil {
		return nil, channelruntime.ErrInvalidWebhook
	}
	isChallenge := strings.EqualFold(strings.TrimSpace(envelope.Type), "url_verification") || strings.TrimSpace(envelope.Challenge) != ""
	encryptKey := strings.TrimSpace(credential.Values["encrypt_key"])
	if encryptKey != "" {
		hasSignature := strings.TrimSpace(request.Headers.Get("X-Lark-Request-Timestamp")) != "" ||
			strings.TrimSpace(request.Headers.Get("X-Lark-Request-Nonce")) != "" ||
			strings.TrimSpace(request.Headers.Get("X-Lark-Signature")) != ""
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
	} else if strings.TrimSpace(request.Headers.Get("X-Lark-Signature")) != "" {
		return nil, channelruntime.ErrWebhookUnauthorized
	}

	verificationToken := strings.TrimSpace(credential.Values["verification_token"])
	if verificationToken == "" || !secureEqual(verificationToken, webhookVerificationToken(request.Body)) {
		return nil, channelruntime.ErrWebhookUnauthorized
	}
	verified := &channelruntime.VerifiedWebhook{Body: append([]byte(nil), request.Body...)}
	if isChallenge {
		challenge := strings.TrimSpace(envelope.Challenge)
		if challenge == "" {
			return nil, channelruntime.ErrInvalidWebhook
		}
		body, err := json.Marshal(map[string]string{"challenge": challenge})
		if err != nil {
			return nil, channelruntime.ErrInvalidWebhook
		}
		response := jsonWebhookResponse(body)
		verified.ImmediateResponse = &response
	}
	return verified, nil
}

func (adapter *ChannelAdapter) verifySignature(request channelruntime.WebhookRequest, encryptKey string) error {
	timestamp := strings.TrimSpace(request.Headers.Get("X-Lark-Request-Timestamp"))
	nonce := strings.TrimSpace(request.Headers.Get("X-Lark-Request-Nonce"))
	signature := strings.ToLower(strings.TrimSpace(request.Headers.Get("X-Lark-Signature")))
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

func (*ChannelAdapter) Normalize(_ context.Context, verified *channelruntime.VerifiedWebhook, normalization channelruntime.NormalizationContext) (*channelruntime.InboundMessage, error) {
	if verified == nil || verified.ImmediateResponse != nil || len(verified.Body) == 0 || len(verified.Body) > maxWebhookBodyBytes {
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
	mentionOnly := text == "" && len(mentions) > 0
	if (!mentionOnly && text == "") || utf8.RuneCountInString(text) > maxInboundTextRunes {
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
	wakeReason := channelruntime.WakeReasonNone
	if conversationType == channelruntime.ConversationTypeDirect {
		wakeReason = channelruntime.WakeReasonDirectMessage
	} else {
		selfID := strings.TrimSpace(normalization.Identity.ExternalSelfID)
		for _, mention := range mentions {
			if selfID != "" && mention.ExternalID == selfID {
				wakeReason = channelruntime.WakeReasonMention
				break
			}
		}
	}
	return &channelruntime.InboundMessage{
		Provider: IntegrationID, ProviderEventID: strings.TrimSpace(event.Header.EventID),
		ProviderMessageID: strings.TrimSpace(message.MessageID), ExternalTenantID: strings.TrimSpace(event.Header.TenantKey),
		ExternalConversationID: strings.TrimSpace(message.ChatID), ExternalThreadID: firstChannelValue(message.ThreadID, message.RootID, message.ParentID),
		RootMessageID: strings.TrimSpace(message.RootID), ParentMessageID: strings.TrimSpace(message.ParentID),
		ThreadID:         strings.TrimSpace(message.ThreadID),
		ConversationType: conversationType, SenderExternalID: senderID, Text: text,
		Mentions: mentions, WakeReason: wakeReason, MentionOnly: mentionOnly, OccurredAt: occurredAt,
	}, nil
}

func (*ChannelAdapter) Acknowledgement() channelruntime.WebhookResponse {
	return jsonWebhookResponse([]byte(`{"code":0}`))
}

func (adapter *ChannelAdapter) Deliver(ctx context.Context, request channelruntime.DeliveryRequest) (*channelruntime.DeliveryReceipt, error) {
	if adapter == nil || (adapter.executor == nil && !adapter.directProviderAvailable()) || request.OrganizationID == uuid.Nil || request.WorkspaceID == uuid.Nil ||
		request.ActorID == uuid.Nil || request.ConnectionID == uuid.Nil || request.RunID == uuid.Nil ||
		request.DeliveryID == uuid.Nil || strings.TrimSpace(request.ExternalConversationID) == "" || strings.TrimSpace(request.Text) == "" {
		return nil, channelruntime.ErrInvalidWebhook
	}
	if adapter.directProviderAvailable() {
		return adapter.deliverDirect(ctx, request)
	}
	request.Text = renderFeishuDeliveryText(request.Text, request.Mentions)
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

func renderFeishuDeliveryText(text string, mentions []channelruntime.DeliveryMention) string {
	text = strings.TrimSpace(text)
	if text == "" || len(mentions) == 0 {
		return text
	}
	var builder strings.Builder
	seen := make(map[string]struct{}, len(mentions))
	for _, mention := range mentions {
		externalID := strings.TrimSpace(mention.ExternalID)
		if !validFeishuToken(externalID) {
			continue
		}
		if _, exists := seen[externalID]; exists {
			continue
		}
		seen[externalID] = struct{}{}
		displayName := strings.TrimSpace(mention.DisplayName)
		if displayName == "" {
			displayName = "成员"
		}
		builder.WriteString(`<at user_id="`)
		builder.WriteString(externalID)
		builder.WriteString(`">`)
		builder.WriteString(html.EscapeString(displayName))
		builder.WriteString("</at> ")
	}
	builder.WriteString(text)
	return strings.TrimSpace(builder.String())
}

func (adapter *ChannelAdapter) deliverDirect(ctx context.Context, request channelruntime.DeliveryRequest) (*channelruntime.DeliveryReceipt, error) {
	connection, cleanup, err := adapter.resolveDirectConnection(ctx, request.OrganizationID, request.ConnectionID)
	if err != nil {
		return nil, err
	}
	defer cleanup()
	region, err := feishuRegion(connection)
	if err != nil {
		return nil, err
	}
	token, tokenMeta, err := adapter.provider.tenantAccessToken(ctx, connection, region)
	if err != nil {
		return nil, err
	}
	messageID, meta, err := adapter.sendChannelMessage(ctx, region, token, request)
	meta.Attempts += tokenMeta.Attempts
	if err != nil {
		// Feishu's UUID deduplication expires after one hour. It is not a
		// durable delivery guarantee: never resend an ambiguous provider write.
		switch integrations.ErrorCode(err) {
		case integrations.ErrorCodeTimeout, integrations.ErrorCodeUpstream, integrations.ErrorCodeResponseInvalid:
			return nil, errors.Join(channelruntime.ErrDeliveryOutcomeUnknown, err)
		default:
			return nil, err
		}
	}
	if messageID == "" {
		return nil, channelruntime.ErrDeliveryOutcomeUnknown
	}
	requestID := firstNonEmpty(meta.RequestID, tokenMeta.RequestID)
	return &channelruntime.DeliveryReceipt{ProviderMessageID: messageID, ProviderRequestID: requestID}, nil
}

func (adapter *ChannelAdapter) directProviderAvailable() bool {
	return adapter != nil && adapter.provider != nil && adapter.provider.client != nil && adapter.connections != nil
}

func (adapter *ChannelAdapter) resolveDirectConnection(ctx context.Context, organizationID, connectionID uuid.UUID) (*integrations.ResolvedConnection, func(), error) {
	if !adapter.directProviderAvailable() || organizationID == uuid.Nil || connectionID == uuid.Nil {
		return nil, func() {}, channelruntime.ErrInvalidWebhook
	}
	connection, err := adapter.connections.Resolve(ctx, integrations.ConnectionResolveRequest{
		OrganizationID:            organizationID.String(),
		IntegrationID:             IntegrationID,
		DriverID:                  DriverID,
		ConnectionID:              connectionID.String(),
		DisallowAccountCredential: true,
	})
	if err != nil {
		return nil, func() {}, err
	}
	cleanup := func() { connection.Destroy() }
	if !isTenantAppConnection(connection) {
		cleanup()
		return nil, func() {}, integrations.NewError(
			integrations.ErrorCodeConnectionInvalid,
			"Feishu tenant app credentials are required for a DigitalHuman channel",
			nil,
		)
	}
	return connection, cleanup, nil
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

func isChannelCredentialKey(key string) bool {
	switch strings.ToLower(strings.TrimSpace(key)) {
	case "app_id", "app_secret", "verification_token", "encrypt_key":
		return true
	default:
		return false
	}
}

func destroyCredentialValues(values map[string]string) {
	for key := range values {
		values[key] = ""
		delete(values, key)
	}
}

func resolvedConnectionFromChannelCredential(credential channelruntime.ResolvedChannelCredential) *integrations.ResolvedConnection {
	return &integrations.ResolvedConnection{
		IntegrationID:     IntegrationID,
		DriverID:          DriverID,
		CredentialSource:  integrations.ConnectionCredentialSourceOrganization,
		AuthType:          integrations.ConnectionAuthTypeCustomCredential,
		AuthMethodID:      TenantAppAuthMethodID,
		Credentials:       cloneCredentialValues(credential.Values),
		Config:            map[string]any{},
		CredentialVersion: credential.Version,
		Revision:          credential.Version,
	}
}

func cloneCredentialValues(values map[string]string) map[string]string {
	cloned := make(map[string]string, len(values))
	for key, value := range values {
		cloned[key] = value
	}
	return cloned
}

func jsonWebhookResponse(body []byte) channelruntime.WebhookResponse {
	return channelruntime.WebhookResponse{
		StatusCode: http.StatusOK,
		Headers:    http.Header{"Content-Type": []string{"application/json; charset=utf-8"}},
		Body:       append([]byte(nil), body...),
	}
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
