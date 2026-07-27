package feishu

import (
	"context"
	"encoding/json"
	"net/http"
	"net/url"
	"strconv"
	"strings"

	"github.com/zgiai/zgi/api/internal/modules/integrations"
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

func newForBaseURLs(httpClient *http.Client, cnBaseURL, globalBaseURL string) (*Adapter, error) {
	apiClient, err := newClientForBaseURLs(httpClient, cnBaseURL, globalBaseURL)
	if err != nil {
		return nil, err
	}
	return &Adapter{client: apiClient}, nil
}

func (adapter *Adapter) DriverID() string { return DriverID }

func (adapter *Adapter) Execute(ctx context.Context, request integrations.ActionRequest) (*integrations.ActionResult, error) {
	region, err := feishuRegion(request.Connection)
	if err != nil {
		return nil, err
	}
	switch request.ActionID {
	case ActionGetAccount:
		token, tokenErr := feishuUserAccessToken(request.Connection)
		if tokenErr != nil {
			return nil, tokenErr
		}
		output, meta, actionErr := adapter.getAccount(ctx, region, token)
		return feishuActionResult(output, meta, 1), actionErr
	case ActionListDriveFiles:
		token, tokenErr := feishuUserAccessToken(request.Connection)
		if tokenErr != nil {
			return nil, tokenErr
		}
		output, meta, actionErr := adapter.listDriveFiles(ctx, region, token, request.Input)
		return feishuActionResult(output, meta, outputCount(output, "files")), actionErr
	case ActionReadDocument:
		token, tokenErr := feishuUserAccessToken(request.Connection)
		if tokenErr != nil {
			return nil, tokenErr
		}
		output, meta, actionErr := adapter.readDocument(ctx, region, token, request.Input)
		return feishuActionResult(output, meta, 1), actionErr
	case ActionSendUserMessage:
		token, tokenErr := feishuUserAccessToken(request.Connection)
		if tokenErr != nil {
			return nil, tokenErr
		}
		output, meta, actionErr := adapter.sendMessage(ctx, region, token, request.Input)
		return feishuActionResult(output, meta, 1), actionErr
	case ActionSendBotMessage:
		token, tokenMeta, tokenErr := adapter.tenantAccessToken(ctx, request.Connection, region)
		if tokenErr != nil {
			return nil, tokenErr
		}
		output, meta, actionErr := adapter.sendMessage(ctx, region, token, request.Input)
		meta.Attempts += tokenMeta.Attempts
		return feishuActionResult(output, meta, 1), actionErr
	default:
		return nil, integrations.NewError(integrations.ErrorCodeInvalidInput, "Feishu action is not supported", nil)
	}
}

func (adapter *Adapter) ValidateConnection(ctx context.Context, connection *integrations.ResolvedConnection) (*integrations.ConnectionProfile, error) {
	region, err := feishuRegion(connection)
	if err != nil {
		return nil, err
	}
	if isTenantAppConnection(connection) {
		token, meta, err := adapter.tenantAccessToken(ctx, connection, region)
		if err != nil {
			return nil, err
		}
		var tenant feishuTenantData
		tenantMeta, err := adapter.client.getTenant(ctx, region, token, &tenant)
		if err != nil {
			return nil, err
		}
		return &integrations.ConnectionProfile{
			AccountID:         firstNonEmpty(tenant.Tenant.TenantKey, tenant.Tenant.DisplayID),
			DisplayName:       firstNonEmpty(tenant.Tenant.Name, tenant.Tenant.DisplayID, tenant.Tenant.TenantKey),
			GrantedScopes:     append([]string(nil), connection.GrantedScopes...),
			ProviderRequestID: firstNonEmpty(tenantMeta.RequestID, meta.RequestID),
		}, nil
	}
	token, err := feishuUserAccessToken(connection)
	if err != nil {
		return nil, err
	}
	var user feishuUserData
	meta, err := adapter.client.getUserInfo(ctx, region, token, &user)
	if err != nil {
		return nil, err
	}
	if strings.TrimSpace(firstNonEmpty(user.OpenID, user.UserID, user.UnionID)) == "" {
		return nil, integrations.NewError(integrations.ErrorCodeResponseInvalid, "Feishu identity response is incomplete", nil)
	}
	return &integrations.ConnectionProfile{
		AccountID:         firstNonEmpty(user.OpenID, user.UserID, user.UnionID),
		DisplayName:       firstNonEmpty(user.Name, user.EnName, user.Email, user.OpenID),
		GrantedScopes:     append([]string(nil), connection.GrantedScopes...),
		ProviderRequestID: meta.RequestID,
	}, nil
}

func (adapter *Adapter) ProbeConnection(ctx context.Context, connection *integrations.ResolvedConnection) (*integrations.HealthProbeReport, error) {
	profile, err := adapter.ValidateConnection(ctx, connection)
	if err != nil {
		status := integrations.HealthProbeStatusUnhealthy
		switch integrations.ErrorCode(err) {
		case integrations.ErrorCodeTimeout, integrations.ErrorCodeUpstream, integrations.ErrorCodeRateLimited:
			status = integrations.HealthProbeStatusDegraded
		}
		return &integrations.HealthProbeReport{
			Status: status,
			Checks: []integrations.HealthProbeCheck{{
				Code: integrations.ErrorCode(err), Status: status, Message: "Feishu connection check failed",
			}},
		}, err
	}
	return &integrations.HealthProbeReport{
		Status: integrations.HealthProbeStatusHealthy, Profile: profile,
		Checks: []integrations.HealthProbeCheck{{
			Code: "feishu_authenticated_identity", Status: integrations.HealthProbeStatusHealthy,
		}},
	}, nil
}

func (adapter *Adapter) getAccount(ctx context.Context, region, token string) (map[string]interface{}, responseMeta, error) {
	var user feishuUserData
	meta, err := adapter.client.getUserInfo(ctx, region, token, &user)
	if err != nil {
		return nil, meta, err
	}
	return map[string]interface{}{
		"provider": IntegrationID, "request_id": bounded(meta.RequestID, 128),
		"account": map[string]interface{}{
			"open_id": bounded(user.OpenID, 128), "union_id": bounded(user.UnionID, 128),
			"user_id": bounded(user.UserID, 128), "name": bounded(user.Name, 255),
			"email": bounded(user.Email, 320), "tenant_key": bounded(user.TenantKey, 128),
			"avatar_url": safeFeishuURL(user.AvatarURL),
		},
	}, meta, nil
}

func (adapter *Adapter) listDriveFiles(ctx context.Context, region, token string, input map[string]interface{}) (map[string]interface{}, responseMeta, error) {
	pageSize := inputInteger(input, "page_size", 20, 1, 50)
	query := url.Values{"page_size": []string{strconv.Itoa(pageSize)}}
	if folderToken := bounded(inputString(input, "folder_token"), 255); folderToken != "" {
		query.Set("folder_token", folderToken)
	}
	if pageToken := bounded(inputString(input, "page_token"), 1024); pageToken != "" {
		query.Set("page_token", pageToken)
	}
	var data feishuDriveFilesData
	meta, err := adapter.client.listDriveFiles(ctx, region, token, query, &data)
	if err != nil {
		return nil, meta, err
	}
	files := make([]interface{}, 0, min(len(data.Files), pageSize))
	for index, file := range data.Files {
		if index >= pageSize {
			break
		}
		files = append(files, map[string]interface{}{
			"token": bounded(file.Token, 255), "name": bounded(file.Name, 500),
			"type": bounded(file.Type, 64), "parent_token": bounded(file.ParentToken, 255),
			"url": safeFeishuURL(file.URL), "created_time": bounded(file.CreatedTime, 64),
			"modified_time": bounded(file.ModifiedTime, 64), "owner_id": bounded(file.OwnerID, 128),
		})
	}
	return map[string]interface{}{
		"provider": IntegrationID, "request_id": bounded(meta.RequestID, 128),
		"files": files, "next_page_token": bounded(data.NextPageToken, 1024), "has_more": data.HasMore,
	}, meta, nil
}

func (adapter *Adapter) readDocument(ctx context.Context, region, token string, input map[string]interface{}) (map[string]interface{}, responseMeta, error) {
	documentID := strings.TrimSpace(inputString(input, "document_id"))
	if !validFeishuToken(documentID) {
		return nil, responseMeta{}, integrations.NewError(integrations.ErrorCodeInvalidInput, "Feishu document id is invalid", nil)
	}
	maxCharacters := inputInteger(input, "max_characters", 20_000, 1, 50_000)
	var data feishuDocumentData
	meta, err := adapter.client.getDocumentRawContent(ctx, region, token, documentID, &data)
	if err != nil {
		return nil, meta, err
	}
	content, truncated := truncateRunes(strings.TrimSpace(data.Content), maxCharacters)
	return map[string]interface{}{
		"provider": IntegrationID, "request_id": bounded(meta.RequestID, 128),
		"document_id": documentID, "content": content, "truncated": truncated,
	}, meta, nil
}

func (adapter *Adapter) sendMessage(ctx context.Context, region, token string, input map[string]interface{}) (map[string]interface{}, responseMeta, error) {
	receiveID := strings.TrimSpace(inputString(input, "receive_id"))
	receiveIDType := inputEnum(input, "receive_id_type", "open_id", "open_id", "user_id", "union_id", "chat_id")
	text := strings.TrimSpace(inputString(input, "text"))
	if !validFeishuToken(receiveID) || text == "" || len([]rune(text)) > 10_000 {
		return nil, responseMeta{}, integrations.NewError(integrations.ErrorCodeInvalidInput, "Feishu message target or text is invalid", nil)
	}
	content, err := json.Marshal(map[string]string{"text": text})
	if err != nil {
		return nil, responseMeta{}, integrations.NewError(integrations.ErrorCodeInvalidInput, "Feishu message could not be encoded", err)
	}
	var data feishuMessageData
	meta, err := adapter.client.sendMessage(ctx, region, token, receiveIDType, map[string]interface{}{
		"receive_id": receiveID, "msg_type": "text", "content": string(content),
	}, &data)
	if err != nil {
		return nil, meta, err
	}
	if strings.TrimSpace(data.MessageID) == "" {
		return nil, meta, integrations.NewError(integrations.ErrorCodeResponseInvalid, "Feishu message response is incomplete", nil)
	}
	return map[string]interface{}{
		"provider": IntegrationID, "request_id": bounded(meta.RequestID, 128),
		"message": map[string]interface{}{
			"message_id": bounded(data.MessageID, 255), "root_id": bounded(data.RootID, 255),
			"parent_id": bounded(data.ParentID, 255), "create_time": bounded(data.CreateTime, 64),
		},
	}, meta, nil
}

func (adapter *Adapter) tenantAccessToken(ctx context.Context, connection *integrations.ResolvedConnection, region string) (string, responseMeta, error) {
	if connection == nil || !isTenantAppConnection(connection) {
		return "", responseMeta{}, integrations.NewError(integrations.ErrorCodeConnectionInvalid, "Feishu tenant app connection is required", nil)
	}
	appID := strings.TrimSpace(connection.Credentials["app_id"])
	appSecret := strings.TrimSpace(connection.Credentials["app_secret"])
	if appID == "" || appSecret == "" {
		return "", responseMeta{}, integrations.NewError(integrations.ErrorCodeAuthInvalid, "Feishu tenant app credentials are unavailable", nil)
	}
	return adapter.client.tenantAccessToken(ctx, region, appID, appSecret)
}

func feishuUserAccessToken(connection *integrations.ResolvedConnection) (string, error) {
	if connection == nil || !strings.EqualFold(connection.IntegrationID, IntegrationID) ||
		!strings.EqualFold(connection.DriverID, DriverID) || isTenantAppConnection(connection) {
		return "", integrations.NewError(integrations.ErrorCodeConnectionInvalid, "Feishu user OAuth connection is required", nil)
	}
	token := strings.TrimSpace(connection.Credentials["access_token"])
	if token == "" {
		return "", integrations.NewError(integrations.ErrorCodeAuthInvalid, "Feishu user credentials are unavailable", nil)
	}
	return token, nil
}

func feishuRegion(connection *integrations.ResolvedConnection) (string, error) {
	if connection == nil || !strings.EqualFold(connection.IntegrationID, IntegrationID) ||
		!strings.EqualFold(connection.DriverID, DriverID) {
		return "", integrations.NewError(integrations.ErrorCodeConnectionInvalid, "Feishu connection is invalid", nil)
	}
	// This provider is the Feishu China product. Lark uses distinct OAuth and
	// data-egress hosts and will be exposed as a separate provider so approval
	// records always name the exact external destination.
	return RegionCN, nil
}

func isTenantAppConnection(connection *integrations.ResolvedConnection) bool {
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

func feishuActionResult(output map[string]interface{}, meta responseMeta, count int) *integrations.ActionResult {
	if output == nil {
		return nil
	}
	return &integrations.ActionResult{
		Output: output, ProviderRequestID: bounded(meta.RequestID, 128),
		ResultCount: max(count, 1), AttemptCount: max(meta.Attempts, 1),
	}
}

func outputCount(output map[string]interface{}, key string) int {
	if values, ok := output[key].([]interface{}); ok {
		return len(values)
	}
	return 0
}

type feishuUserData struct {
	OpenID    string `json:"open_id"`
	UnionID   string `json:"union_id"`
	UserID    string `json:"user_id"`
	Name      string `json:"name"`
	EnName    string `json:"en_name"`
	AvatarURL string `json:"avatar_url"`
	Email     string `json:"email"`
	TenantKey string `json:"tenant_key"`
}

type feishuDriveFilesData struct {
	Files         []feishuDriveFile `json:"files"`
	NextPageToken string            `json:"next_page_token"`
	HasMore       bool              `json:"has_more"`
}

type feishuDriveFile struct {
	Token        string `json:"token"`
	Name         string `json:"name"`
	Type         string `json:"type"`
	ParentToken  string `json:"parent_token"`
	URL          string `json:"url"`
	CreatedTime  string `json:"created_time"`
	ModifiedTime string `json:"modified_time"`
	OwnerID      string `json:"owner_id"`
}

type feishuDocumentData struct {
	Content string `json:"content"`
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

func bounded(value string, limit int) string {
	value = strings.TrimSpace(value)
	runes := []rune(value)
	if len(runes) > limit {
		return string(runes[:limit])
	}
	return value
}

func safeFeishuURL(value string) string {
	parsed, err := url.Parse(strings.TrimSpace(value))
	if err != nil || parsed.Scheme != "https" || parsed.User != nil {
		return ""
	}
	host := strings.ToLower(parsed.Hostname())
	if host != "feishu.cn" && !strings.HasSuffix(host, ".feishu.cn") &&
		host != "larksuite.com" && !strings.HasSuffix(host, ".larksuite.com") {
		return ""
	}
	parsed.Fragment = ""
	return bounded(parsed.String(), 2048)
}

func truncateRunes(value string, limit int) (string, bool) {
	runes := []rune(value)
	if len(runes) <= limit {
		return value, false
	}
	return string(runes[:limit]), true
}

func inputString(input map[string]interface{}, key string) string {
	value, _ := input[key].(string)
	return value
}

func inputInteger(input map[string]interface{}, key string, fallback, minimum, maximum int) int {
	value := fallback
	switch typed := input[key].(type) {
	case int:
		value = typed
	case int64:
		value = int(typed)
	case float64:
		if typed == float64(int(typed)) {
			value = int(typed)
		}
	case json.Number:
		if parsed, err := strconv.Atoi(string(typed)); err == nil {
			value = parsed
		}
	}
	return min(max(value, minimum), maximum)
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

var _ integrations.Adapter = (*Adapter)(nil)
var _ integrations.ConnectionTester = (*Adapter)(nil)
var _ integrations.HealthProbe = (*Adapter)(nil)
