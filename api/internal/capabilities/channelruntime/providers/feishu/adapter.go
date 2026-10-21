package feishu

import (
	"context"
	"github.com/zgiai/zgi/api/internal/capabilities/channelruntime/providertransport"

	"encoding/json"
	"net/http"
	"net/url"
	"strconv"
	"strings"
)

type Adapter struct {
	client *client
}

func New(httpClient *http.Client) (*Adapter, error) {
	apiClient, err := newClient(httpClient)
	if err != nil {
		return nil, err
	}
	return &Adapter{client: apiClient}, nil
}

func (adapter *Adapter) DriverID() string { return DriverID }

// ResolveChannelIdentity discovers the immutable tenant and bot identifiers
// used to route DigitalHuman channel events. Credentials remain request-scoped
// and the caller is responsible for destroying the resolved connection.
func (adapter *Adapter) ResolveChannelIdentity(ctx context.Context, connection *providertransport.CredentialLease) (string, string, error) {
	region, err := feishuRegion(connection)
	if err != nil {
		return "", "", err
	}
	if !isTenantAppConnection(connection) {
		return "", "", providertransport.NewError(providertransport.ErrorCodeConnectionInvalid, "Feishu tenant app connection is required", nil)
	}
	token, _, err := adapter.tenantAccessToken(ctx, connection, region)
	if err != nil {
		return "", "", err
	}
	tenantID := strings.TrimSpace(connection.AccountID)
	if tenantID == "" {
		var tenant feishuTenantData
		if _, err = adapter.client.getTenant(ctx, region, token, &tenant); err != nil {
			return "", "", err
		}
		tenantID = strings.TrimSpace(firstNonEmpty(tenant.Tenant.TenantKey, tenant.Tenant.DisplayID))
	}
	var bot feishuBotData
	if _, err = adapter.client.getBotInfo(ctx, region, token, &bot); err != nil {
		return "", "", err
	}
	botID := strings.TrimSpace(bot.OpenID)
	if !validFeishuToken(tenantID) || !validFeishuToken(botID) {
		return "", "", providertransport.NewError(providertransport.ErrorCodeResponseInvalid, "Feishu channel identity response is incomplete", nil)
	}
	return tenantID, botID, nil
}

func (adapter *Adapter) listMessages(ctx context.Context, region, token, selfSenderID string, input map[string]interface{}) (map[string]interface{}, responseMeta, error) {
	chatID := strings.TrimSpace(inputString(input, "chat_id"))
	if !validFeishuToken(chatID) {
		return nil, responseMeta{}, providertransport.NewError(providertransport.ErrorCodeInvalidInput, "Feishu chat id is invalid", nil)
	}
	pageSize, err := strictInputInteger(input, "page_size", 20, 1, 50)
	if err != nil {
		return nil, responseMeta{}, err
	}
	query := url.Values{
		"container_id_type": []string{"chat"},
		"container_id":      []string{chatID},
		"page_size":         []string{strconv.Itoa(pageSize)},
	}
	pageToken := strings.TrimSpace(inputString(input, "page_token"))
	if len([]rune(pageToken)) > 1024 {
		return nil, responseMeta{}, providertransport.NewError(providertransport.ErrorCodeInvalidInput, "Feishu page token is invalid", nil)
	}
	if pageToken != "" {
		query.Set("page_token", pageToken)
	}
	sortType := strings.TrimSpace(inputString(input, "sort_type"))
	switch sortType {
	case "", "newest_first":
		query.Set("sort_type", "ByCreateTimeDesc")
	case "oldest_first":
		query.Set("sort_type", "ByCreateTimeAsc")
	default:
		return nil, responseMeta{}, providertransport.NewError(providertransport.ErrorCodeInvalidInput, "Feishu message sort order is invalid", nil)
	}
	startTime, hasStart, err := optionalInputUnixSeconds(input, "start_time")
	if err != nil {
		return nil, responseMeta{}, err
	}
	endTime, hasEnd, err := optionalInputUnixSeconds(input, "end_time")
	if err != nil {
		return nil, responseMeta{}, err
	}
	if hasStart != hasEnd || (hasStart && (endTime <= startTime || endTime-startTime > 7*24*60*60)) {
		return nil, responseMeta{}, providertransport.NewError(
			providertransport.ErrorCodeInvalidInput,
			"Feishu message time range must include start and end and cannot exceed seven days",
			nil,
		)
	}
	if hasStart {
		query.Set("start_time", strconv.FormatInt(startTime, 10))
		query.Set("end_time", strconv.FormatInt(endTime, 10))
	}
	var data feishuMessagesData
	meta, err := adapter.client.listMessages(ctx, region, token, query, &data)
	if err != nil {
		return nil, meta, err
	}
	messages := make([]interface{}, 0, min(len(data.Items), pageSize))
	for index, message := range data.Items {
		if index >= pageSize {
			break
		}
		messages = append(messages, map[string]interface{}{
			"message_id": bounded(message.MessageID, 255), "root_id": bounded(message.RootID, 255),
			"parent_id": bounded(message.ParentID, 255), "thread_id": bounded(message.ThreadID, 255),
			"chat_id": bounded(message.ChatID, 255), "message_type": bounded(message.MsgType, 64),
			"text":      normalizedMessageText(message.MsgType, message.Body.Content),
			"sender_id": bounded(message.Sender.ID, 128), "sender_type": bounded(message.Sender.SenderType, 64),
			"sender_is_self": selfSenderID != "" && strings.EqualFold(strings.TrimSpace(message.Sender.ID), selfSenderID),
			"create_time":    bounded(message.CreateTime, 64), "update_time": bounded(message.UpdateTime, 64),
			"deleted": message.Deleted, "updated": message.Updated,
		})
	}
	return map[string]interface{}{
		"provider": IntegrationID, "request_id": bounded(meta.RequestID, 128),
		"messages": messages, "next_page_token": bounded(data.PageToken, 1024), "has_more": data.HasMore,
	}, meta, nil
}

func (adapter *Adapter) getMessage(ctx context.Context, region, token, selfSenderID string, input map[string]interface{}) (map[string]interface{}, responseMeta, error) {
	messageID := strings.TrimSpace(inputString(input, "message_id"))
	if !validFeishuToken(messageID) {
		return nil, responseMeta{}, providertransport.NewError(providertransport.ErrorCodeInvalidInput, "Feishu message id is invalid", nil)
	}
	var data feishuMessagesData
	meta, err := adapter.client.getMessage(ctx, region, token, messageID, &data)
	if err != nil {
		return nil, meta, err
	}
	return map[string]interface{}{
		"provider": IntegrationID, "request_id": bounded(meta.RequestID, 128),
		"messages": normalizedMessages(data.Items, 1, selfSenderID),
	}, meta, nil
}

func (adapter *Adapter) listChatMembers(ctx context.Context, region, token string, input map[string]interface{}) (map[string]interface{}, responseMeta, error) {
	chatID := strings.TrimSpace(inputString(input, "chat_id"))
	if !validFeishuToken(chatID) {
		return nil, responseMeta{}, providertransport.NewError(providertransport.ErrorCodeInvalidInput, "Feishu chat id is invalid", nil)
	}
	pageSize, err := strictInputInteger(input, "page_size", 100, 1, 100)
	if err != nil {
		return nil, responseMeta{}, err
	}
	query := url.Values{"member_id_type": []string{"open_id"}, "page_size": []string{strconv.Itoa(pageSize)}}
	if pageToken := bounded(inputString(input, "page_token"), 1024); pageToken != "" {
		query.Set("page_token", pageToken)
	}
	var data feishuChatMembersData
	meta, err := adapter.client.listChatMembers(ctx, region, token, chatID, query, &data)
	if err != nil {
		return nil, meta, err
	}
	members := make([]interface{}, 0, min(len(data.Items), pageSize))
	for index, member := range data.Items {
		if index >= pageSize {
			break
		}
		members = append(members, map[string]interface{}{
			"member_id": bounded(member.MemberID, 128), "name": bounded(member.Name, 500),
		})
	}
	return map[string]interface{}{
		"provider": IntegrationID, "request_id": bounded(meta.RequestID, 128), "members": members,
		"next_page_token": bounded(data.PageToken, 1024), "has_more": data.HasMore,
	}, meta, nil
}

func normalizedMessages(items []feishuMessage, limit int, selfSenderID string) []interface{} {
	messages := make([]interface{}, 0, min(len(items), limit))
	for index, message := range items {
		if index >= limit {
			break
		}
		messages = append(messages, map[string]interface{}{
			"message_id": bounded(message.MessageID, 255), "root_id": bounded(message.RootID, 255),
			"parent_id": bounded(message.ParentID, 255), "thread_id": bounded(message.ThreadID, 255),
			"chat_id": bounded(message.ChatID, 255), "message_type": bounded(message.MsgType, 64),
			"text":      normalizedMessageText(message.MsgType, message.Body.Content),
			"sender_id": bounded(message.Sender.ID, 128), "sender_type": bounded(message.Sender.SenderType, 64),
			"sender_is_self": selfSenderID != "" && strings.EqualFold(strings.TrimSpace(message.Sender.ID), selfSenderID),
			"create_time":    bounded(message.CreateTime, 64), "update_time": bounded(message.UpdateTime, 64),
			"deleted": message.Deleted, "updated": message.Updated,
		})
	}
	return messages
}

func (adapter *Adapter) sendMessage(
	ctx context.Context,
	region string,
	token string,
	connection *providertransport.CredentialLease,
	input map[string]interface{},
	allowSelf bool,
) (map[string]interface{}, responseMeta, error) {
	recipientType := inputEnum(input, "recipient_type", "open_id", "self", "open_id", "user_id", "union_id", "chat_id")
	receiveID := strings.TrimSpace(inputString(input, "recipient_id"))
	receiveIDType := recipientType
	if recipientType == "self" {
		if !allowSelf || connection == nil {
			return nil, responseMeta{}, providertransport.NewError(providertransport.ErrorCodeInvalidInput, "Feishu self recipient is unavailable for this action", nil)
		}
		receiveID = strings.TrimSpace(connection.AccountID)
		receiveIDType = "open_id"
	}
	text := strings.TrimSpace(inputString(input, "text"))
	if !validFeishuToken(receiveID) || text == "" || len([]rune(text)) > 10_000 {
		return nil, responseMeta{}, providertransport.NewError(providertransport.ErrorCodeInvalidInput, "Feishu message target or text is invalid", nil)
	}
	content, err := json.Marshal(map[string]string{"text": text})
	if err != nil {
		return nil, responseMeta{}, providertransport.NewError(providertransport.ErrorCodeInvalidInput, "Feishu message could not be encoded", err)
	}
	var data feishuMessageData
	meta, err := adapter.client.sendMessage(ctx, region, token, receiveIDType, map[string]interface{}{
		"receive_id": receiveID, "msg_type": "text", "content": string(content),
	}, &data)
	if err != nil {
		return nil, meta, err
	}
	if strings.TrimSpace(data.MessageID) == "" {
		return nil, meta, providertransport.NewError(providertransport.ErrorCodeResponseInvalid, "Feishu message response is incomplete", nil)
	}
	return map[string]interface{}{
		"provider": IntegrationID, "request_id": bounded(meta.RequestID, 128),
		"message": map[string]interface{}{
			"message_id": bounded(data.MessageID, 255), "root_id": bounded(data.RootID, 255),
			"parent_id": bounded(data.ParentID, 255), "create_time": bounded(data.CreateTime, 64),
		},
	}, meta, nil
}

func feishuConnectionAppID(connection *providertransport.CredentialLease) string {
	if !isTenantAppConnection(connection) {
		return ""
	}
	return strings.TrimSpace(connection.Credentials["app_id"])
}

func (adapter *Adapter) tenantAccessToken(ctx context.Context, connection *providertransport.CredentialLease, region string) (string, responseMeta, error) {
	if connection == nil || !isTenantAppConnection(connection) {
		return "", responseMeta{}, providertransport.NewError(providertransport.ErrorCodeConnectionInvalid, "Feishu tenant app connection is required", nil)
	}
	appID := strings.TrimSpace(connection.Credentials["app_id"])
	appSecret := strings.TrimSpace(connection.Credentials["app_secret"])
	if appID == "" || appSecret == "" {
		return "", responseMeta{}, providertransport.NewError(providertransport.ErrorCodeAuthInvalid, "Feishu tenant app credentials are unavailable", nil)
	}
	return adapter.client.tenantAccessToken(ctx, region, appID, appSecret)
}

func feishuRegion(connection *providertransport.CredentialLease) (string, error) {
	if connection == nil || !strings.EqualFold(connection.ProviderID, IntegrationID) ||
		!strings.EqualFold(connection.DriverID, DriverID) {
		return "", providertransport.NewError(providertransport.ErrorCodeConnectionInvalid, "Feishu connection is invalid", nil)
	}

	switch strings.ToLower(strings.TrimSpace(connection.Credentials["region"])) {
	case "", "feishu", RegionCN:
		return RegionCN, nil
	case "lark", RegionGlobal:
		return RegionGlobal, nil
	default:
		return "", providertransport.NewError(providertransport.ErrorCodeConnectionInvalid, "Feishu connection region is invalid", nil)
	}
}

func isTenantAppConnection(connection *providertransport.CredentialLease) bool {
	return connection != nil && strings.EqualFold(connection.AuthMethodID, TenantAppAuthMethodID)
}

func validFeishuToken(value string) bool {
	if value == "" || len(value) > 255 {
		return false
	}
	for _, char := range value {
		if (char >= 'a' && char <= 'z') || (char >= 'A' && char <= 'Z') ||
			(char >= '0' && char <= '9') || char == '_' || char == '-' {
			continue
		}
		return false
	}
	return true
}

func normalizedMessageText(messageType, raw string) string {
	if strings.EqualFold(strings.TrimSpace(messageType), "post") {
		return normalizedPostText(raw)
	}
	if !strings.EqualFold(strings.TrimSpace(messageType), "text") {
		return ""
	}
	var content struct {
		Text string `json:"text"`
	}
	if err := json.Unmarshal([]byte(raw), &content); err != nil {
		return ""
	}
	return bounded(content.Text, 4000)
}

func feishuActionResult(output map[string]interface{}, meta responseMeta, count int) *providertransport.ContextResult {
	diagnostics := meta.Diagnostics
	if diagnostics.RequestID == "" {
		diagnostics.RequestID = bounded(meta.RequestID, 128)
	}
	if output == nil && meta.Attempts <= 0 && diagnostics == (providertransport.ProviderDiagnostics{}) {
		return nil
	}
	if output == nil {
		count = 0
	}
	attempts := meta.Attempts
	if attempts <= 0 && output != nil {
		attempts = 1
	}
	return &providertransport.ContextResult{
		Output: output, ProviderRequestID: bounded(meta.RequestID, 128),
		ProviderDiagnostics: diagnostics,
		ResultCount:         max(count, 0), AttemptCount: max(attempts, 0),
	}
}

func outputCount(output map[string]interface{}, key string) int {
	if values, ok := output[key].([]interface{}); ok {
		return len(values)
	}
	return 0
}

type feishuMessagesData struct {
	Items     []feishuMessage `json:"items"`
	PageToken string          `json:"page_token"`
	HasMore   bool            `json:"has_more"`
}

type feishuChatMembersData struct {
	Items     []feishuChatMember `json:"items"`
	PageToken string             `json:"page_token"`
	HasMore   bool               `json:"has_more"`
}

type feishuChatMember struct {
	MemberID string `json:"member_id"`
	Name     string `json:"name"`
}

type feishuMessage struct {
	MessageID  string `json:"message_id"`
	RootID     string `json:"root_id"`
	ParentID   string `json:"parent_id"`
	ThreadID   string `json:"thread_id"`
	ChatID     string `json:"chat_id"`
	MsgType    string `json:"msg_type"`
	CreateTime string `json:"create_time"`
	UpdateTime string `json:"update_time"`
	Deleted    bool   `json:"deleted"`
	Updated    bool   `json:"updated"`
	Sender     struct {
		ID         string `json:"id"`
		IDType     string `json:"id_type"`
		SenderType string `json:"sender_type"`
	} `json:"sender"`
	Body struct {
		Content string `json:"content"`
	} `json:"body"`
}

type feishuMessageData struct {
	MessageID  string `json:"message_id"`
	RootID     string `json:"root_id"`
	ParentID   string `json:"parent_id"`
	CreateTime string `json:"create_time"`
}

type feishuTenantData struct {
	Tenant struct {
		TenantKey string `json:"tenant_key"`
		Name      string `json:"name"`
		DisplayID string `json:"display_id"`
	} `json:"tenant"`
}

type feishuBotData struct {
	OpenID  string `json:"open_id"`
	AppName string `json:"app_name"`
}

func bounded(value string, limit int) string {
	value = strings.TrimSpace(value)
	runes := []rune(value)
	if len(runes) > limit {
		return string(runes[:limit])
	}
	return value
}

func inputString(input map[string]interface{}, key string) string {
	value, _ := input[key].(string)
	return value
}

func strictInputInteger(input map[string]interface{}, key string, fallback, minimum, maximum int) (int, error) {
	raw, ok := input[key]
	if !ok || raw == nil {
		return fallback, nil
	}
	value, ok := integerValue(raw)
	if !ok || value < int64(minimum) || value > int64(maximum) {
		return 0, providertransport.NewError(providertransport.ErrorCodeInvalidInput, "Feishu integer input is invalid", nil)
	}
	return int(value), nil
}

func optionalInputUnixSeconds(input map[string]interface{}, key string) (int64, bool, error) {
	raw, ok := input[key]
	if !ok || raw == nil {
		return 0, false, nil
	}
	value, ok := integerValue(raw)
	if !ok || value < 1 || value > 4_102_444_800 {
		return 0, false, providertransport.NewError(providertransport.ErrorCodeInvalidInput, "Feishu Unix timestamp is invalid", nil)
	}
	return value, true, nil
}

func integerValue(raw interface{}) (int64, bool) {
	switch typed := raw.(type) {
	case int:
		return int64(typed), true
	case int32:
		return int64(typed), true
	case int64:
		return typed, true
	case float64:
		if typed == float64(int64(typed)) {
			return int64(typed), true
		}
	case json.Number:
		parsed, err := strconv.ParseInt(string(typed), 10, 64)
		return parsed, err == nil
	}
	return 0, false
}

func inputEnum(input map[string]interface{}, key, fallback string, allowed ...string) string {
	value := strings.ToLower(strings.TrimSpace(inputString(input, key)))
	for _, candidate := range allowed {
		if value == candidate {
			return value
		}
	}
	return fallback
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if value = strings.TrimSpace(value); value != "" {
			return value
		}
	}
	return ""
}
