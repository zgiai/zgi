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
	"strconv"
	"strings"
	"sync"
	"time"
	"unicode/utf8"

	"github.com/google/uuid"
	"github.com/zgiai/zgi/api/internal/capabilities/channelruntime"
	providertransport "github.com/zgiai/zgi/api/internal/capabilities/channelruntime/providertransport"
)

const (
	feishuMessageReceiveEvent = "im.message.receive_v1"
	maxWebhookBodyBytes       = 1 << 20
	maxInboundTextRunes       = 32_000
	webhookTimestampSkew      = 5 * time.Minute
)

// ChannelAdapter implements the transport-neutral channel contract. DigitalHuman
// traffic resolves installation-owned credentials and calls Feishu directly.
type ChannelAdapter struct {
	provider      *Adapter
	connections   providertransport.CredentialResolver
	now           func() time.Time
	memberCacheMu sync.Mutex
	memberCache   map[string]feishuMemberCacheEntry
}

// NewDirectChannelAdapter creates the DigitalHuman channel adapter without the
// generic integration executor. The connection resolver is used only as a
// request-scoped credential store; provider calls, retries, and failures are
// handled directly by the Feishu adapter.
func NewDirectChannelAdapter(provider *Adapter, connections providertransport.CredentialResolver) (*ChannelAdapter, error) {
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
		NativeAttachments:          true,
		MaxAttachmentBytes:         maxChannelFileBytes,
		ContextRead:                true,
		Edit:                       adapter.directProviderAvailable(),
		ProcessingMarker:           adapter.directProviderAvailable(),
		MessageRoundTripTest:       true,
		ConversationCapabilityTest: true,
	}
}

func (*ChannelAdapter) ConnectionSpec() channelruntime.ConnectionSpec {
	return channelruntime.ConnectionSpec{
		IntegrationID:           IntegrationID,
		DriverID:                DriverID,
		AuthMethodID:            TenantAppAuthMethodID,
		CompatibleDriverIDs:     []string{"feishu-openapi-v1"},
		CompatibleAuthMethodIDs: []string{"feishu.app_credentials"},
	}
}

func (*ChannelAdapter) PrepareCredentials(current, updates map[string]string) (map[string]string, error) {
	prepared := make(map[string]string, len(current)+len(updates))
	for key, value := range current {
		key = strings.ToLower(strings.TrimSpace(key))
		if !isChannelCredentialKey(key) {
			destroyCredentialValues(prepared)
			return nil, providertransport.NewError(providertransport.ErrorCodeInvalidInput, "unsupported Feishu channel credential field", nil)
		}
		if value = strings.TrimSpace(value); value != "" {
			prepared[key] = value
		}
	}
	for key, value := range updates {
		key = strings.ToLower(strings.TrimSpace(key))
		if !isChannelCredentialKey(key) {
			destroyCredentialValues(prepared)
			return nil, providertransport.NewError(providertransport.ErrorCodeInvalidInput, "unsupported Feishu channel credential field", nil)
		}
		if value = strings.TrimSpace(value); value != "" {
			prepared[key] = value
		}
	}
	// Existing credentials without an explicit mode are legacy webhooks. New
	// credentials default to the SDK WebSocket transport, while an existing
	// webhook remains valid so migration cannot silently disable callbacks.
	if prepared["connection_mode"] == "" {
		prepared["connection_mode"] = "websocket"
		if len(current) > 0 || prepared["verification_token"] != "" {
			prepared["connection_mode"] = "webhook"
		}
	}
	if prepared["connection_mode"] != "websocket" && prepared["connection_mode"] != "webhook" {
		destroyCredentialValues(prepared)
		return nil, providertransport.NewError(providertransport.ErrorCodeInvalidInput, "unsupported Feishu connection mode", nil)
	}
	if prepared["app_id"] == "" || prepared["app_secret"] == "" ||
		(prepared["connection_mode"] == "webhook" && prepared["verification_token"] == "") {
		destroyCredentialValues(prepared)
		return nil, providertransport.NewError(providertransport.ErrorCodeInvalidInput, "required Feishu credentials are missing for the selected transport", nil)
	}
	return prepared, nil
}

func (adapter *ChannelAdapter) NormalizeCredentials(values map[string]string, transport string) (map[string]string, error) {
	// Connector-backed Feishu credentials carry no channel transport metadata;
	// the receiver resolves them to the SDK WebSocket transport.
	if strings.EqualFold(strings.TrimSpace(transport), "websocket") {
		prepared := make(map[string]string, len(values))
		for key, value := range values {
			key = strings.ToLower(strings.TrimSpace(key))
			if key != "region" && key != "app_id" && key != "app_secret" {
				destroyCredentialValues(prepared)
				return nil, providertransport.NewError(providertransport.ErrorCodeInvalidInput, "unsupported Feishu long connection credential field", nil)
			}
			if value = strings.TrimSpace(value); value != "" {
				prepared[key] = value
			}
		}
		if !validFeishuLongConnectionCredentials(prepared["app_id"], prepared["app_secret"]) {
			destroyCredentialValues(prepared)
			return nil, providertransport.NewError(providertransport.ErrorCodeInvalidInput, "Feishu long connection credentials are required", nil)
		}
		return prepared, nil
	}
	if strings.EqualFold(strings.TrimSpace(transport), "webhook") {
		webhookValues := make(map[string]string, len(values)+1)
		for key, value := range values {
			webhookValues[key] = value
		}
		webhookValues["connection_mode"] = "webhook"
		return adapter.PrepareCredentials(nil, webhookValues)
	}
	return adapter.PrepareCredentials(nil, values)
}

func validFeishuLongConnectionCredentials(appID, appSecret string) bool {
	return appID != "" && appSecret != "" && len(appID) <= 128 && len(appSecret) <= 16384 && !strings.ContainsAny(appID+appSecret, "\r\n\x00")
}

func (adapter *ChannelAdapter) ResolveIdentity(ctx context.Context, credential channelruntime.ResolvedChannelCredential) (channelruntime.InstallationIdentity, error) {
	if adapter == nil || adapter.provider == nil || len(credential.Values) == 0 {
		return channelruntime.InstallationIdentity{}, providertransport.NewError(providertransport.ErrorCodeConnectionInvalid, "Feishu channel credentials are unavailable", nil)
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
	// Switching transport disables the legacy inbound URL. Retain webhook
	// secrets only so an explicit rollback can restore the old path.
	if feishuConnectionMode(credential) != "webhook" {
		return nil, channelruntime.ErrWebhookUnauthorized
	}
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
	if adapter == nil || !adapter.directProviderAvailable() || request.OrganizationID == uuid.Nil || request.WorkspaceID == uuid.Nil ||
		request.ActorID == uuid.Nil || request.ConnectionID == uuid.Nil || request.RunID == uuid.Nil ||
		request.DeliveryID == uuid.Nil || strings.TrimSpace(request.ExternalConversationID) == "" || (strings.TrimSpace(request.Text) == "" && len(request.Attachments) == 0) {
		return nil, channelruntime.ErrInvalidWebhook
	}
	return adapter.deliverDirect(ctx, request)
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
	connection, cleanup, err := adapter.resolveDirectConnection(ctx, request.OrganizationID, request.WorkspaceID, request.InstallationID)
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
	if len(request.Attachments) != 0 {
		return adapter.deliverWithAttachments(ctx, region, token, request)
	}
	messageID, meta, err := adapter.sendChannelMessage(ctx, region, token, request)
	meta.Attempts += tokenMeta.Attempts
	if err != nil {

		switch providertransport.ErrorCode(err) {
		case providertransport.ErrorCodeTimeout, providertransport.ErrorCodeUpstream, providertransport.ErrorCodeResponseInvalid:
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

func (adapter *ChannelAdapter) resolveDirectConnection(ctx context.Context, organizationID, workspaceID, installationID uuid.UUID) (*providertransport.CredentialLease, func(), error) {
	if !adapter.directProviderAvailable() || organizationID == uuid.Nil || workspaceID == uuid.Nil || installationID == uuid.Nil {
		return nil, func() {}, channelruntime.ErrInvalidWebhook
	}
	connection, err := adapter.connections.Resolve(ctx, providertransport.CredentialRequest{
		OrganizationID: organizationID.String(),
		ProviderID:     IntegrationID,
		DriverID:       DriverID,
		InstallationID: installationID.String(),
		WorkspaceID:    workspaceID.String(),
	})
	if err != nil {
		return nil, func() {}, err
	}
	cleanup := func() { connection.Destroy() }
	if !isTenantAppConnection(connection) {
		cleanup()
		return nil, func() {}, providertransport.NewError(
			providertransport.ErrorCodeConnectionInvalid,
			"Feishu tenant app credentials are required for a DigitalHuman channel",
			nil,
		)
	}
	return connection, cleanup, nil
}

// Edit replaces a delivered structured card with its new state. Only cards
// can be edited: a prompt sent as text, or a request without a card, is left
// as it is.
func (adapter *ChannelAdapter) Edit(ctx context.Context, request channelruntime.DeliveryRequest, providerMessageID string) (*channelruntime.DeliveryReceipt, error) {
	providerMessageID = strings.TrimSpace(providerMessageID)
	if adapter == nil || !adapter.directProviderAvailable() || request.Card == nil || !validFeishuToken(providerMessageID) ||
		request.OrganizationID == uuid.Nil || request.WorkspaceID == uuid.Nil || request.InstallationID == uuid.Nil {
		return nil, channelruntime.ErrUnsupportedEvent
	}
	body, ok := buildChannelCard(request)
	if !ok {
		return nil, channelruntime.ErrUnsupportedEvent
	}
	connection, cleanup, err := adapter.resolveDirectConnection(ctx, request.OrganizationID, request.WorkspaceID, request.InstallationID)
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
	meta, err := adapter.provider.client.patchMessage(ctx, region, token, providerMessageID, map[string]string{"content": body.Content})
	if err != nil {
		return nil, err
	}
	return &channelruntime.DeliveryReceipt{
		ProviderMessageID: providerMessageID, ProviderRequestID: firstNonEmpty(meta.RequestID, tokenMeta.RequestID),
	}, nil
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
	case "region", "app_id", "app_secret", "verification_token", "encrypt_key", "connection_mode":
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

func resolvedConnectionFromChannelCredential(credential channelruntime.ResolvedChannelCredential) *providertransport.CredentialLease {
	return &providertransport.CredentialLease{
		ProviderID:        IntegrationID,
		DriverID:          DriverID,
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
