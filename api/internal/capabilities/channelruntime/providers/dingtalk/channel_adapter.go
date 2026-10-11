package dingtalk

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"

	"github.com/google/uuid"
	"github.com/zgiai/zgi/api/internal/capabilities/channelruntime"
	providertransport "github.com/zgiai/zgi/api/internal/capabilities/channelruntime/providertransport"
)

const (
	maxDingTalkWebhookBodyBytes = 1 << 20
	maxDingTalkInboundTextRunes = 32_000
	dingTalkDirectTargetPrefix  = "direct:"
	dingTalkGroupTargetPrefix   = "group:"
)

// ChannelAdapter normalizes DingTalk Stream messages and sends replies without
// exposing DingTalk protocol details to DigitalHuman.
type ChannelAdapter struct {
	provider    *Adapter
	connections providertransport.CredentialResolver
}

// NewDirectChannelAdapter creates a DingTalk channel provider backed by the
// installation credential resolver. Credentials remain request scoped.
func NewDirectChannelAdapter(provider *Adapter, connections providertransport.CredentialResolver) (*ChannelAdapter, error) {
	if provider == nil || provider.client == nil || connections == nil {
		return nil, fmt.Errorf("initialize direct DingTalk channel adapter: provider and credential resolver are required")
	}
	return &ChannelAdapter{
		provider: provider, connections: connections,
	}, nil
}

func (*ChannelAdapter) Provider() string { return IntegrationID }

func (*ChannelAdapter) Capabilities() channelruntime.ProviderCapabilities {
	return channelruntime.ProviderCapabilities{}
}

func (*ChannelAdapter) ConnectionSpec() channelruntime.ConnectionSpec {
	return channelruntime.ConnectionSpec{
		IntegrationID: IntegrationID,
		DriverID:      DriverID,
		AuthMethodID:  AuthMethodID,
	}
}

func (*ChannelAdapter) PrepareCredentials(current, updates map[string]string) (map[string]string, error) {
	prepared := make(map[string]string, len(current)+len(updates))
	for key, value := range current {
		key = strings.ToLower(strings.TrimSpace(key))
		if key == "reception_mode" {

			continue
		}
		if !isDingTalkChannelCredentialKey(key) {
			destroyDingTalkChannelCredentials(prepared)
			return nil, providertransport.NewError(providertransport.ErrorCodeInvalidInput, "unsupported DingTalk channel credential field", nil)
		}
		if value = strings.TrimSpace(value); value != "" {
			prepared[key] = value
		}
	}
	for key, value := range updates {
		key = strings.ToLower(strings.TrimSpace(key))
		if !isDingTalkChannelCredentialKey(key) {
			destroyDingTalkChannelCredentials(prepared)
			return nil, providertransport.NewError(providertransport.ErrorCodeInvalidInput, "unsupported DingTalk channel credential field", nil)
		}
		if value = strings.TrimSpace(value); value != "" {
			prepared[key] = value
		}
	}
	if !validDingTalkChannelCredentials(prepared) {
		destroyDingTalkChannelCredentials(prepared)
		return nil, providertransport.NewError(providertransport.ErrorCodeInvalidInput, "DingTalk AppKey and AppSecret are required", nil)
	}
	return prepared, nil
}

func (adapter *ChannelAdapter) ResolveIdentity(ctx context.Context, credential channelruntime.ResolvedChannelCredential) (channelruntime.InstallationIdentity, error) {
	if adapter == nil || adapter.provider == nil || !validDingTalkChannelCredentials(credential.Values) {
		return channelruntime.InstallationIdentity{}, providertransport.NewError(providertransport.ErrorCodeConnectionInvalid, "DingTalk channel credentials are unavailable", nil)
	}
	appKey := strings.TrimSpace(credential.Values["app_key"])
	creds := credentials{
		AppKey: appKey, AppSecret: strings.TrimSpace(credential.Values["app_secret"]),
		AgentID: strings.TrimSpace(credential.Values["agent_id"]), ConnectionID: "channel:" + appKey,
	}
	if _, err := adapter.provider.accessToken(ctx, creds); err != nil {
		return channelruntime.InstallationIdentity{}, err
	}
	return channelruntime.InstallationIdentity{ExternalTenantID: appKey, ExternalSelfID: appKey}, nil
}

func (*ChannelAdapter) Verify(context.Context, channelruntime.WebhookRequest, channelruntime.ResolvedChannelCredential) (*channelruntime.VerifiedWebhook, error) {
	return nil, channelruntime.ErrUnsupportedEvent
}

func (*ChannelAdapter) Normalize(_ context.Context, verified *channelruntime.VerifiedWebhook, normalization channelruntime.NormalizationContext) (*channelruntime.InboundMessage, error) {
	if verified == nil || verified.ImmediateResponse != nil || len(verified.Body) == 0 || len(verified.Body) > maxDingTalkWebhookBodyBytes {
		return nil, channelruntime.ErrInvalidWebhook
	}
	var callback dingTalkRobotCallback
	if err := json.Unmarshal(verified.Body, &callback); err != nil {
		return nil, channelruntime.ErrInvalidWebhook
	}
	if !strings.EqualFold(strings.TrimSpace(callback.MessageType), "text") {
		return nil, channelruntime.ErrUnsupportedEvent
	}
	messageID := strings.TrimSpace(callback.MessageID)
	senderID := firstDingTalkWebhookValue(callback.SenderStaffID, callback.SenderID)
	if messageID == "" || senderID == "" {
		return nil, channelruntime.ErrInvalidWebhook
	}
	conversationType, target, err := dingTalkConversationTarget(callback, senderID)
	if err != nil {
		return nil, err
	}
	text := strings.TrimSpace(callback.Text.Content)
	wakeReason := channelruntime.WakeReasonNone
	mentions := []channelruntime.Mention(nil)
	mentionOnly := false
	if conversationType == channelruntime.ConversationTypeDirect {
		wakeReason = channelruntime.WakeReasonDirectMessage
	} else if callback.IsInAtList {
		wakeReason = channelruntime.WakeReasonMention
		mentions = []channelruntime.Mention{{ExternalID: strings.TrimSpace(normalization.Identity.ExternalSelfID)}}
		text = stripDingTalkRobotMention(text)
		mentionOnly = text == ""
	}
	if (!mentionOnly && text == "") || !utf8.ValidString(text) || utf8.RuneCountInString(text) > maxDingTalkInboundTextRunes {
		return nil, channelruntime.ErrInvalidWebhook
	}
	occurredAt, err := dingTalkCallbackTime(callback.CreatedAt)
	if err != nil {
		return nil, channelruntime.ErrInvalidWebhook
	}
	return &channelruntime.InboundMessage{
		Provider: IntegrationID, ProviderEventID: messageID, ProviderMessageID: messageID,
		ExternalTenantID:       strings.TrimSpace(normalization.Identity.ExternalTenantID),
		ExternalConversationID: target, ConversationType: conversationType,
		SenderExternalID: senderID, SenderDisplayName: boundedDingTalkSenderName(callback.SenderNick),
		Text: text, Mentions: mentions,
		WakeReason: wakeReason, MentionOnly: mentionOnly, OccurredAt: occurredAt,
	}, nil
}

func (*ChannelAdapter) Acknowledgement() channelruntime.WebhookResponse {
	return channelruntime.WebhookResponse{StatusCode: http.StatusOK}
}

func (adapter *ChannelAdapter) Deliver(ctx context.Context, request channelruntime.DeliveryRequest) (*channelruntime.DeliveryReceipt, error) {
	if adapter == nil || adapter.provider == nil || adapter.connections == nil || request.OrganizationID == uuid.Nil ||
		request.ConnectionID == uuid.Nil || request.DeliveryID == uuid.Nil || strings.TrimSpace(request.Text) == "" {
		return nil, channelruntime.ErrInvalidWebhook
	}
	connection, err := adapter.connections.Resolve(ctx, providertransport.CredentialRequest{
		OrganizationID: request.OrganizationID.String(), ProviderID: IntegrationID, DriverID: DriverID,
		InstallationID: request.InstallationID.String(), WorkspaceID: request.WorkspaceID.String(),
	})
	if err != nil {
		return nil, err
	}
	defer connection.Destroy()
	if !validDingTalkChannelCredentials(connection.Credentials) {
		return nil, providertransport.NewError(providertransport.ErrorCodeConnectionInvalid, "DingTalk channel credentials are unavailable", nil)
	}
	creds := credentials{
		AppKey: strings.TrimSpace(connection.Credentials["app_key"]), AppSecret: strings.TrimSpace(connection.Credentials["app_secret"]),
		AgentID: strings.TrimSpace(connection.Credentials["agent_id"]), ConnectionID: connection.ID,
	}
	path, body, err := dingTalkRobotDelivery(request.ExternalConversationID, creds.AppKey, request.Text, request.Mentions)
	if err != nil {
		return nil, err
	}
	response, err := adapter.sendRobotMessage(ctx, creds, path, body)
	if err != nil {
		switch providertransport.ErrorCode(err) {
		case providertransport.ErrorCodeTimeout, providertransport.ErrorCodeUpstream:
			return nil, errors.Join(channelruntime.ErrDeliveryOutcomeUnknown, err)
		default:
			return nil, err
		}
	}
	messageID := firstDingTalkWebhookValue(response.ProcessQueryKey, response.MessageID)
	if messageID == "" {
		messageID = "delivery:" + request.DeliveryID.String()
	}
	return &channelruntime.DeliveryReceipt{ProviderMessageID: messageID, ProviderRequestID: response.RequestID}, nil
}

func (adapter *ChannelAdapter) sendRobotMessage(ctx context.Context, creds credentials, path string, body map[string]interface{}) (dingTalkRobotSendResponse, error) {
	for attempt := 0; attempt < 2; attempt++ {
		token, err := adapter.provider.accessToken(ctx, creds)
		if err != nil {
			return dingTalkRobotSendResponse{}, err
		}
		status, raw, err := adapter.provider.doJSON(ctx, http.MethodPost, adapter.provider.apiBaseURL+path, nil, map[string]string{
			"x-acs-dingtalk-access-token": token,
		}, body)
		if err != nil {
			return dingTalkRobotSendResponse{}, err
		}
		if status >= 200 && status < 300 {
			var response dingTalkRobotSendResponse
			if len(strings.TrimSpace(string(raw))) > 0 && string(raw) != "null" {
				if err := json.Unmarshal(raw, &response); err != nil {
					return dingTalkRobotSendResponse{}, dingError(providertransport.ErrorCodeResponseInvalid, "DingTalk robot response is invalid", err)
				}
			}
			return response, nil
		}
		var envelope apiErrorEnvelope
		_ = json.Unmarshal(raw, &envelope)
		if isExpiredToken(status, envelope.Code) && attempt == 0 {
			adapter.provider.evictToken(creds)
			continue
		}
		return dingTalkRobotSendResponse{}, mapAPIError(status, envelope)
	}
	return dingTalkRobotSendResponse{}, dingError(providertransport.ErrorCodeAuthInvalid, "DingTalk access token is invalid", nil)
}

func (*ChannelAdapter) Edit(context.Context, channelruntime.DeliveryRequest, string) (*channelruntime.DeliveryReceipt, error) {
	return nil, channelruntime.ErrUnsupportedEvent
}

func (*ChannelAdapter) Retract(context.Context, channelruntime.DeliveryRequest, string) error {
	return channelruntime.ErrUnsupportedEvent
}

type dingTalkRobotCallback struct {
	ConversationID   string `json:"conversationId"`
	ConversationType string `json:"conversationType"`
	MessageID        string `json:"msgId"`
	MessageType      string `json:"msgtype"`
	CreatedAt        int64  `json:"createAt"`
	SenderID         string `json:"senderId"`
	SenderStaffID    string `json:"senderStaffId"`
	SenderNick       string `json:"senderNick"`
	ChatbotCorpID    string `json:"chatbotCorpId"`
	ChatbotUserID    string `json:"chatbotUserId"`
	IsInAtList       bool   `json:"isInAtList"`
	Text             struct {
		Content string `json:"content"`
	} `json:"text"`
}

type dingTalkRobotSendResponse struct {
	ProcessQueryKey string `json:"processQueryKey"`
	MessageID       string `json:"messageId"`
	RequestID       string `json:"requestId"`
}

func dingTalkConversationTarget(callback dingTalkRobotCallback, senderID string) (channelruntime.ConversationType, string, error) {
	switch strings.ToLower(strings.TrimSpace(callback.ConversationType)) {
	case "1", "direct", "private", "single":
		return channelruntime.ConversationTypeDirect, dingTalkDirectTargetPrefix + senderID, nil
	case "2", "group":
		conversationID := strings.TrimSpace(callback.ConversationID)
		if conversationID == "" {
			return "", "", channelruntime.ErrInvalidWebhook
		}
		return channelruntime.ConversationTypeGroup, dingTalkGroupTargetPrefix + conversationID, nil
	default:
		return "", "", channelruntime.ErrUnsupportedEvent
	}
}

func dingTalkRobotDelivery(
	target string,
	robotCode string,
	text string,
	mentions []channelruntime.DeliveryMention,
) (string, map[string]interface{}, error) {
	target = strings.TrimSpace(target)
	robotCode = strings.TrimSpace(robotCode)
	text = strings.TrimSpace(text)
	if target == "" || robotCode == "" || text == "" {
		return "", nil, channelruntime.ErrInvalidWebhook
	}
	parameter, err := json.Marshal(map[string]string{"content": text})
	if err != nil {
		return "", nil, channelruntime.ErrInvalidWebhook
	}
	body := map[string]interface{}{"robotCode": robotCode, "msgKey": "sampleText", "msgParam": string(parameter)}
	switch {
	case strings.HasPrefix(target, dingTalkDirectTargetPrefix):
		userID := strings.TrimSpace(strings.TrimPrefix(target, dingTalkDirectTargetPrefix))
		if userID == "" {
			return "", nil, channelruntime.ErrInvalidWebhook
		}
		body["userIds"] = []string{userID}
		return "/v1.0/robot/oToMessages/batchSend", body, nil
	case strings.HasPrefix(target, dingTalkGroupTargetPrefix):
		conversationID := strings.TrimSpace(strings.TrimPrefix(target, dingTalkGroupTargetPrefix))
		if conversationID == "" {
			return "", nil, channelruntime.ErrInvalidWebhook
		}
		mentionIDs := dingTalkMentionUserIDs(mentions)
		if len(mentionIDs) > 0 {
			prefixes := make([]string, 0, len(mentionIDs))
			for _, mentionID := range mentionIDs {
				prefixes = append(prefixes, "@"+mentionID)
			}
			parameter, err = json.Marshal(map[string]string{"content": strings.Join(prefixes, " ") + " " + text})
			if err != nil {
				return "", nil, channelruntime.ErrInvalidWebhook
			}
			body["msgParam"] = string(parameter)
			body["atUserIds"] = mentionIDs
		}
		body["openConversationId"] = conversationID
		return "/v1.0/robot/groupMessages/send", body, nil
	default:
		return "", nil, channelruntime.ErrInvalidWebhook
	}
}

func dingTalkMentionUserIDs(mentions []channelruntime.DeliveryMention) []string {
	if len(mentions) == 0 {
		return nil
	}
	result := make([]string, 0, len(mentions))
	seen := make(map[string]struct{}, len(mentions))
	for _, mention := range mentions {
		externalID := strings.TrimSpace(mention.ExternalID)
		if !validDingTalkMentionUserID(externalID) {
			continue
		}
		if _, exists := seen[externalID]; exists {
			continue
		}
		seen[externalID] = struct{}{}
		result = append(result, externalID)
	}
	return result
}

func validDingTalkMentionUserID(value string) bool {
	if value == "" || utf8.RuneCountInString(value) > 255 {
		return false
	}
	for _, character := range value {
		if unicode.IsSpace(character) || unicode.IsControl(character) || character == '@' {
			return false
		}
	}
	return true
}

func firstDingTalkWebhookValue(values ...string) string {
	for _, value := range values {
		if value = strings.TrimSpace(value); value != "" {
			return value
		}
	}
	return ""
}

func dingTalkCallbackTime(value int64) (time.Time, error) {
	if value <= 0 {
		return time.Time{}, fmt.Errorf("DingTalk callback time is missing")
	}
	if value < 10_000_000_000 {
		return time.Unix(value, 0).UTC(), nil
	}
	return time.UnixMilli(value).UTC(), nil
}

func stripDingTalkRobotMention(value string) string {
	value = strings.TrimSpace(value)
	if !strings.HasPrefix(value, "@") {
		return value
	}
	if index := strings.IndexAny(value, " \t\r\n"); index >= 0 {
		return strings.TrimSpace(value[index:])
	}
	return ""
}

func boundedDingTalkSenderName(value string) string {
	value = strings.TrimSpace(value)
	runes := []rune(value)
	if len(runes) > 128 {
		return string(runes[:128])
	}
	return value
}

func isDingTalkChannelCredentialKey(key string) bool {
	switch strings.ToLower(strings.TrimSpace(key)) {
	case "app_key", "app_secret", "agent_id":
		return true
	default:
		return false
	}
}

func validDingTalkChannelCredentials(values map[string]string) bool {
	appKey := strings.TrimSpace(values["app_key"])
	secret := strings.TrimSpace(values["app_secret"])
	return appKey != "" && secret != "" && len(appKey) <= 256 && len(secret) <= 512 &&
		!strings.ContainsAny(appKey+secret, "\r\n\x00")
}

func destroyDingTalkChannelCredentials(values map[string]string) {
	for key := range values {
		values[key] = ""
		delete(values, key)
	}
}
