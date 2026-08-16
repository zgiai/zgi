package dingtalk

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/zgiai/zgi/api/internal/modules/integrations"
)

const (
	defaultAPIBaseURL    = "https://api.dingtalk.com"
	defaultLegacyBaseURL = "https://oapi.dingtalk.com"
)

type Adapter struct {
	client        *http.Client
	apiBaseURL    string
	legacyBaseURL string
	mu            sync.Mutex
	tokens        map[string]cachedToken
}
type cachedToken struct {
	value          string
	expiresAt      time.Time
	credentialHash [32]byte
}
type credentials struct {
	AppKey, AppSecret, AgentID, ConnectionID string
}
type legacyEnvelope struct {
	ErrCode   int    `json:"errcode"`
	ErrMsg    string `json:"errmsg"`
	RequestID string `json:"request_id"`
}
type apiErrorEnvelope struct {
	Code      string `json:"code"`
	Message   string `json:"message"`
	RequestID string `json:"requestid"`
}

func New(httpClient *http.Client) (*Adapter, error) {
	return newForBaseURLs(httpClient, defaultAPIBaseURL, defaultLegacyBaseURL)
}
func newForBaseURLs(httpClient *http.Client, apiBaseURL, legacyBaseURL string) (*Adapter, error) {
	for _, raw := range []string{apiBaseURL, legacyBaseURL} {
		parsed, err := url.Parse(strings.TrimSpace(raw))
		if err != nil || parsed.Scheme == "" || parsed.Host == "" {
			return nil, fmt.Errorf("invalid DingTalk base URL")
		}
	}
	if httpClient == nil {
		httpClient = &http.Client{Timeout: 20 * time.Second}
	} else {
		clone := *httpClient
		httpClient = &clone
		if httpClient.Timeout <= 0 {
			httpClient.Timeout = 20 * time.Second
		}
	}
	httpClient.CheckRedirect = func(_ *http.Request, _ []*http.Request) error { return http.ErrUseLastResponse }
	return &Adapter{client: httpClient, apiBaseURL: strings.TrimRight(apiBaseURL, "/"), legacyBaseURL: strings.TrimRight(legacyBaseURL, "/"), tokens: map[string]cachedToken{}}, nil
}

func (adapter *Adapter) DriverID() string { return DriverID }

func (adapter *Adapter) ValidateCredentials(_ context.Context, request integrations.CredentialValidationRequest) error {
	_, err := parseCredentials(&integrations.ResolvedConnection{ID: "candidate", IntegrationID: request.IntegrationID, DriverID: request.DriverID, AuthMethodID: request.AuthMethodID, Credentials: request.Credentials, Config: request.Config})
	return err
}

func (adapter *Adapter) Execute(ctx context.Context, request integrations.ActionRequest) (*integrations.ActionResult, error) {
	creds, err := parseCredentials(request.Connection)
	if err != nil {
		return nil, err
	}
	switch request.ActionID {
	case ActionDepartmentList:
		output, count, err := adapter.listDepartments(ctx, creds, inputInt(request.Input, "department_id", 1))
		return actionResult(output, count), err
	case ActionDepartmentSearch:
		output, count, err := adapter.searchDepartments(ctx, creds, request.Input)
		return actionResult(output, count), err
	case ActionDepartmentGet:
		output, err := adapter.getDepartment(ctx, creds, inputString(request.Input, "department_ref"))
		return actionResult(output, 1), err
	case ActionDepartmentUsers:
		output, count, err := adapter.listDepartmentMembers(ctx, creds, request.Input)
		return actionResult(output, count), err
	case ActionContactSearch:
		output, count, err := adapter.searchContacts(ctx, creds, request.Input)
		return actionResult(output, count), err
	case ActionUserGet:
		output, err := adapter.getUser(ctx, creds, inputString(request.Input, "recipient_ref"))
		return actionResult(output, 1), err
	case ActionRoleList:
		output, count, err := adapter.listRoles(ctx, creds, request.Input)
		return actionResult(output, count), err
	case ActionRoleUsers:
		output, count, err := adapter.listRoleMembers(ctx, creds, request.Input)
		return actionResult(output, count), err
	case ActionAttendanceList:
		output, count, err := adapter.listAttendanceRecords(ctx, creds, request.Input)
		return actionResult(output, count), err
	case ActionMessageSendUser:
		output, err := adapter.sendUser(ctx, creds, request.Input)
		return actionResult(output, 1), err
	case ActionMessageSendDept:
		output, err := adapter.sendDepartment(ctx, creds, request.Input)
		return actionResult(output, 1), err
	case ActionMessageStatusGet:
		output, err := adapter.getMessageStatus(ctx, creds, inputString(request.Input, "message_ref"))
		return actionResult(output, 1), err
	default:
		return nil, dingError(integrations.ErrorCodeInvalidInput, "DingTalk action is not supported", nil)
	}
}

func (adapter *Adapter) ValidateConnection(ctx context.Context, connection *integrations.ResolvedConnection) (*integrations.ConnectionProfile, error) {
	creds, err := parseCredentials(connection)
	if err != nil {
		return nil, err
	}
	if _, _, err = adapter.listDepartments(ctx, creds, 1); err != nil {
		return nil, err
	}
	return &integrations.ConnectionProfile{
		AccountID:         creds.AppKey + ":" + creds.AgentID,
		DisplayName:       "DingTalk application " + creds.AgentID,
		GrantedScopes:     []string{},
		ScopeEvidence:     integrations.AuthScopeEvidenceConnectorDeclared,
		VerifiedActionIDs: []string{ActionDepartmentList},
	}, nil
}

func (adapter *Adapter) ProbeConnection(ctx context.Context, connection *integrations.ResolvedConnection) (*integrations.HealthProbeReport, error) {
	profile, err := adapter.ValidateConnection(ctx, connection)
	if err != nil {
		status := integrations.HealthProbeStatusUnhealthy
		if code := integrations.ErrorCode(err); code == integrations.ErrorCodeTimeout || code == integrations.ErrorCodeUpstream || code == integrations.ErrorCodeRateLimited {
			status = integrations.HealthProbeStatusDegraded
		}
		return &integrations.HealthProbeReport{Status: status, Checks: []integrations.HealthProbeCheck{{Code: integrations.ErrorCode(err), Status: status, Message: "DingTalk application check failed"}}}, err
	}
	return &integrations.HealthProbeReport{Status: integrations.HealthProbeStatusHealthy, Profile: profile, Checks: []integrations.HealthProbeCheck{{Code: "dingtalk_application_authenticated", Status: integrations.HealthProbeStatusHealthy}}}, nil
}

func (adapter *Adapter) listDepartments(ctx context.Context, creds credentials, departmentID int) (map[string]interface{}, int, error) {
	if departmentID < 1 {
		departmentID = 1
	}
	var response struct {
		legacyEnvelope
		Result []struct {
			DeptID   int    `json:"dept_id"`
			Name     string `json:"name"`
			ParentID int    `json:"parent_id"`
		} `json:"result"`
	}
	err := adapter.legacyJSON(ctx, creds, "/topapi/v2/department/listsub", map[string]interface{}{"dept_id": departmentID, "language": "zh_CN"}, &response)
	if err != nil {
		return nil, 0, err
	}
	if len(response.Result) > 200 {
		response.Result = response.Result[:200]
	}
	items := make([]map[string]interface{}, 0, len(response.Result))
	for _, department := range response.Result {
		items = append(items, map[string]interface{}{"department_ref": encodeDepartmentRef(creds.ConnectionID, int64(department.DeptID)), "id": department.DeptID, "name": bounded(department.Name, 255), "parent_id": department.ParentID})
	}
	output := map[string]interface{}{"provider": IntegrationID, "departments": items}
	if len(items) == 0 {
		// The provider endpoint lists children of the requested department. An
		// empty result is a successful observation and must not be presented as
		// an authorization failure.
		output["empty_reason"] = "no_child_departments"
	}
	return output, len(items), nil
}

func (adapter *Adapter) searchContacts(ctx context.Context, creds credentials, input map[string]interface{}) (map[string]interface{}, int, error) {
	query := strings.TrimSpace(inputString(input, "query"))
	if query == "" {
		return nil, 0, dingError(integrations.ErrorCodeInvalidInput, "DingTalk member search query is required", nil)
	}
	limit := inputInt(input, "max_results", 10)
	if limit < 1 {
		limit = 10
	}
	if limit > 20 {
		limit = 20
	}
	var response map[string]json.RawMessage
	err := adapter.apiJSON(ctx, creds, http.MethodPost, "/v1.0/contact/users/search", map[string]interface{}{"queryWord": query, "offset": 0, "size": limit}, &response)
	if err != nil {
		return nil, 0, err
	}
	rawItems := extractSearchItems(response)
	items := make([]map[string]interface{}, 0, min(limit, len(rawItems)))
	for _, raw := range rawItems {
		if len(items) >= limit {
			break
		}
		var candidate struct {
			UserID       string `json:"userId"`
			UserIDLegacy string `json:"userid"`
			Name         string `json:"name"`
			Title        string `json:"title"`
		}
		_ = json.Unmarshal(raw, &candidate)
		userID := strings.TrimSpace(candidate.UserID)
		if userID == "" {
			userID = strings.TrimSpace(candidate.UserIDLegacy)
		}
		if userID == "" {
			_ = json.Unmarshal(raw, &userID)
		}
		if !validOpaqueID(userID, 128) {
			continue
		}
		if strings.TrimSpace(candidate.Name) == "" {
			details, detailErr := adapter.getUserByID(ctx, creds, userID)
			if detailErr != nil {
				return nil, 0, detailErr
			}
			candidate.Name, candidate.Title = details.Name, details.Title
		}
		items = append(items, map[string]interface{}{"recipient_ref": encodeRecipientRef(creds.ConnectionID, userID), "name": bounded(candidate.Name, 255), "title": bounded(candidate.Title, 255)})
	}
	return map[string]interface{}{"provider": IntegrationID, "members": items}, len(items), nil
}

type userDetails struct {
	UserID        string `json:"userid"`
	Name          string `json:"name"`
	Title         string `json:"title"`
	DepartmentIDs []int  `json:"dept_id_list"`
	Active        bool   `json:"active"`
}

func (adapter *Adapter) getUser(ctx context.Context, creds credentials, recipientRef string) (map[string]interface{}, error) {
	userID, err := decodeRecipientRef(recipientRef, creds.ConnectionID)
	if err != nil {
		return nil, dingError(integrations.ErrorCodeInvalidInput, "DingTalk recipient reference is invalid", err)
	}
	details, err := adapter.getUserByID(ctx, creds, userID)
	if err != nil {
		return nil, err
	}
	return map[string]interface{}{"provider": IntegrationID, "member": map[string]interface{}{"recipient_ref": recipientRef, "name": bounded(details.Name, 255), "title": bounded(details.Title, 255), "department_ids": details.DepartmentIDs, "active": details.Active}}, nil
}

func (adapter *Adapter) getUserByID(ctx context.Context, creds credentials, userID string) (userDetails, error) {
	var response struct {
		legacyEnvelope
		Result userDetails `json:"result"`
	}
	if err := adapter.legacyJSON(ctx, creds, "/topapi/v2/user/get", map[string]interface{}{"userid": userID, "language": "zh_CN"}, &response); err != nil {
		return userDetails{}, err
	}
	if !validOpaqueID(response.Result.UserID, 128) || strings.TrimSpace(response.Result.Name) == "" {
		return userDetails{}, dingError(integrations.ErrorCodeResponseInvalid, "DingTalk member response is incomplete", nil)
	}
	return response.Result, nil
}

func (adapter *Adapter) sendUser(ctx context.Context, creds credentials, input map[string]interface{}) (map[string]interface{}, error) {
	recipientRef := strings.TrimSpace(inputString(input, "recipient_ref"))
	userID, err := decodeRecipientRef(recipientRef, creds.ConnectionID)
	if err != nil {
		return nil, dingError(integrations.ErrorCodeInvalidInput, "DingTalk recipient reference is invalid", err)
	}
	content := strings.TrimSpace(inputString(input, "content"))
	if content == "" || len([]rune(content)) > 2048 {
		return nil, dingError(integrations.ErrorCodeInvalidInput, "DingTalk notification content is invalid", nil)
	}
	agentID, _ := strconv.ParseInt(creds.AgentID, 10, 64)
	var response struct {
		legacyEnvelope
		TaskID int64 `json:"task_id"`
	}
	payload := map[string]interface{}{"agent_id": agentID, "userid_list": userID, "msg": map[string]interface{}{"msgtype": "text", "text": map[string]string{"content": content}}}
	if err := adapter.legacyJSON(ctx, creds, "/topapi/message/corpconversation/asyncsend_v2", payload, &response); err != nil {
		return nil, err
	}
	if response.TaskID <= 0 {
		return nil, dingError(integrations.ErrorCodeResponseInvalid, "DingTalk notification response is incomplete", nil)
	}
	messageRef := encodeMessageRef(creds.ConnectionID, response.TaskID, userID)
	return map[string]interface{}{"provider": IntegrationID, "notification": map[string]interface{}{"message_ref": messageRef, "recipient_ref": recipientRef, "provider_accepted": true, "delivery_status": "pending"}}, nil
}

func (adapter *Adapter) getMessageStatus(ctx context.Context, creds credentials, encoded string) (map[string]interface{}, error) {
	ref, err := decodeMessageRef(encoded, creds.ConnectionID)
	if err != nil {
		return nil, dingError(integrations.ErrorCodeInvalidInput, "DingTalk message reference is invalid", err)
	}
	agentID, _ := strconv.ParseInt(creds.AgentID, 10, 64)
	var response struct {
		legacyEnvelope
		Result struct {
			SendResult struct {
				Invalid   []string `json:"invalid_user_id_list"`
				Forbidden []string `json:"forbidden_user_id_list"`
				Failed    []string `json:"failed_user_id_list"`
				Read      []string `json:"read_user_id_list"`
				Unread    []string `json:"unread_user_id_list"`
			} `json:"send_result"`
		} `json:"result"`
	}
	if err := adapter.legacyJSON(ctx, creds, "/topapi/message/corpconversation/getsendresult", map[string]interface{}{"agent_id": agentID, "task_id": ref.TaskID}, &response); err != nil {
		return nil, err
	}
	status, reason := "pending", ""
	sendResult := response.Result.SendResult
	targetType := "member"
	deliveredCount, failedCount := 0, 0
	if ref.DepartmentID > 0 {
		targetType = "department"
		deliveredCount = len(sendResult.Read) + len(sendResult.Unread)
		failedCount = len(sendResult.Invalid) + len(sendResult.Forbidden) + len(sendResult.Failed)
		switch {
		case deliveredCount > 0 && failedCount > 0:
			status, reason = "partially_delivered", "some_recipients_failed"
		case len(sendResult.Read) > 0:
			status = "delivered_read"
		case len(sendResult.Unread) > 0:
			status = "delivered_unread"
		case failedCount > 0:
			status, reason = "failed", "provider_delivery_failed"
		}
	} else {
		switch {
		case contains(sendResult.Invalid, ref.UserID):
			status, reason, failedCount = "failed", "recipient_invalid", 1
		case contains(sendResult.Forbidden, ref.UserID):
			status, reason, failedCount = "failed", "recipient_forbidden", 1
		case contains(sendResult.Failed, ref.UserID):
			status, reason, failedCount = "failed", "provider_delivery_failed", 1
		case contains(sendResult.Read, ref.UserID):
			status, deliveredCount = "delivered_read", 1
		case contains(sendResult.Unread, ref.UserID):
			status, deliveredCount = "delivered_unread", 1
		}
	}
	return map[string]interface{}{"provider": IntegrationID, "notification": map[string]interface{}{"message_ref": encoded, "target_type": targetType, "delivery_status": status, "delivered_count": deliveredCount, "failed_count": failedCount, "failure_reason": reason}}, nil
}

func (adapter *Adapter) apiJSON(ctx context.Context, creds credentials, method, path string, body interface{}, out interface{}) error {
	for attempt := 0; attempt < 2; attempt++ {
		token, err := adapter.accessToken(ctx, creds)
		if err != nil {
			return err
		}
		status, raw, err := adapter.doJSON(ctx, method, adapter.apiBaseURL+path, nil, map[string]string{"x-acs-dingtalk-access-token": token}, body)
		if err != nil {
			return err
		}
		if status >= 200 && status < 300 {
			if err := json.Unmarshal(raw, out); err != nil {
				return dingError(integrations.ErrorCodeResponseInvalid, "DingTalk response is invalid", err)
			}
			return nil
		}
		var envelope apiErrorEnvelope
		_ = json.Unmarshal(raw, &envelope)
		if isExpiredToken(status, envelope.Code) && attempt == 0 {
			adapter.evictToken(creds)
			continue
		}
		return mapAPIError(status, envelope)
	}
	return dingError(integrations.ErrorCodeAuthInvalid, "DingTalk access token is invalid", nil)
}

func (adapter *Adapter) legacyJSON(ctx context.Context, creds credentials, path string, body interface{}, out interface{}) error {
	for attempt := 0; attempt < 2; attempt++ {
		token, err := adapter.accessToken(ctx, creds)
		if err != nil {
			return err
		}
		query := url.Values{"access_token": []string{token}}
		status, raw, err := adapter.doJSON(ctx, http.MethodPost, adapter.legacyBaseURL+path, query, nil, body)
		if err != nil {
			return err
		}
		var envelope legacyEnvelope
		if err := json.Unmarshal(raw, &envelope); err != nil {
			return dingError(integrations.ErrorCodeResponseInvalid, "DingTalk response is invalid", err)
		}
		if envelope.ErrCode == 0 && status >= 200 && status < 300 {
			if err := json.Unmarshal(raw, out); err != nil {
				return dingError(integrations.ErrorCodeResponseInvalid, "DingTalk response is invalid", err)
			}
			return nil
		}
		if isLegacyExpiredToken(envelope.ErrCode) && attempt == 0 {
			adapter.evictToken(creds)
			continue
		}
		return mapLegacyError(status, envelope)
	}
	return dingError(integrations.ErrorCodeAuthInvalid, "DingTalk access token is invalid", nil)
}

func (adapter *Adapter) accessToken(ctx context.Context, creds credentials) (string, error) {
	hash := sha256.Sum256([]byte(creds.AppKey + "\x00" + creds.AppSecret + "\x00" + creds.AgentID))
	adapter.mu.Lock()
	cached, ok := adapter.tokens[creds.ConnectionID]
	adapter.mu.Unlock()
	if ok && cached.credentialHash == hash && time.Now().Before(cached.expiresAt) {
		return cached.value, nil
	}
	status, raw, err := adapter.doJSON(ctx, http.MethodPost, adapter.apiBaseURL+"/v1.0/oauth2/accessToken", nil, nil, map[string]string{"appKey": creds.AppKey, "appSecret": creds.AppSecret})
	if err != nil {
		return "", err
	}
	if status < 200 || status >= 300 {
		var envelope apiErrorEnvelope
		_ = json.Unmarshal(raw, &envelope)
		return "", mapAPIError(status, envelope)
	}
	var response struct {
		AccessToken string `json:"accessToken"`
		ExpireIn    int    `json:"expireIn"`
	}
	if err := json.Unmarshal(raw, &response); err != nil || strings.TrimSpace(response.AccessToken) == "" {
		return "", dingError(integrations.ErrorCodeResponseInvalid, "DingTalk access token response is incomplete", err)
	}
	ttl := time.Duration(response.ExpireIn)*time.Second - 5*time.Minute
	if ttl < time.Minute {
		ttl = time.Minute
	}
	adapter.mu.Lock()
	adapter.tokens[creds.ConnectionID] = cachedToken{value: response.AccessToken, expiresAt: time.Now().Add(ttl), credentialHash: hash}
	adapter.mu.Unlock()
	return response.AccessToken, nil
}

func (adapter *Adapter) evictToken(creds credentials) {
	adapter.mu.Lock()
	delete(adapter.tokens, creds.ConnectionID)
	adapter.mu.Unlock()
}

func (adapter *Adapter) doJSON(ctx context.Context, method, target string, query url.Values, headers map[string]string, body interface{}) (int, []byte, error) {
	if len(query) > 0 {
		target += "?" + query.Encode()
	}
	var reader io.Reader
	if body != nil {
		raw, err := json.Marshal(body)
		if err != nil {
			return 0, nil, dingError(integrations.ErrorCodeInvalidInput, "DingTalk request could not be encoded", err)
		}
		reader = bytes.NewReader(raw)
	}
	request, err := http.NewRequestWithContext(ctx, method, target, reader)
	if err != nil {
		return 0, nil, dingError(integrations.ErrorCodeInvalidInput, "DingTalk request is invalid", err)
	}
	request.Header.Set("Accept", "application/json")
	if body != nil {
		request.Header.Set("Content-Type", "application/json")
	}
	for key, value := range headers {
		request.Header.Set(key, value)
	}
	response, err := adapter.client.Do(request)
	if err != nil {
		if ctx.Err() != nil {
			return 0, nil, dingError(integrations.ErrorCodeTimeout, "DingTalk request timed out", ctx.Err())
		}
		return 0, nil, dingError(integrations.ErrorCodeUpstream, "DingTalk service is unavailable", err)
	}
	defer response.Body.Close()
	raw, err := io.ReadAll(io.LimitReader(response.Body, 2<<20))
	if err != nil {
		return response.StatusCode, nil, dingError(integrations.ErrorCodeResponseInvalid, "DingTalk response could not be read", err)
	}
	return response.StatusCode, raw, nil
}

func parseCredentials(connection *integrations.ResolvedConnection) (credentials, error) {
	if connection == nil || !strings.EqualFold(connection.IntegrationID, IntegrationID) || !strings.EqualFold(connection.DriverID, DriverID) {
		return credentials{}, dingError(integrations.ErrorCodeConnectionInvalid, "DingTalk connection is invalid", nil)
	}
	creds := credentials{AppKey: strings.TrimSpace(connection.Credentials["app_key"]), AppSecret: strings.TrimSpace(connection.Credentials["app_secret"]), AgentID: strings.TrimSpace(connection.Credentials["agent_id"]), ConnectionID: strings.TrimSpace(connection.ID)}
	if creds.AppKey == "" || creds.AppSecret == "" || creds.AgentID == "" || creds.ConnectionID == "" || len(creds.AppKey) > 256 || len(creds.AppSecret) > 512 || len(creds.AgentID) > 32 || strings.ContainsAny(creds.AppKey+creds.AppSecret+creds.AgentID, "\r\n\x00") {
		return credentials{}, dingError(integrations.ErrorCodeConnectionInvalid, "DingTalk application credentials are unavailable", nil)
	}
	if value, err := strconv.ParseInt(creds.AgentID, 10, 64); err != nil || value <= 0 {
		return credentials{}, dingError(integrations.ErrorCodeConnectionInvalid, "DingTalk AgentId is invalid", err)
	}
	return creds, nil
}

type recipientReference struct {
	Version      int    `json:"v"`
	ConnectionID string `json:"c"`
	UserID       string `json:"u"`
}
type messageReference struct {
	Version      int    `json:"v"`
	ConnectionID string `json:"c"`
	TaskID       int64  `json:"t"`
	UserID       string `json:"u,omitempty"`
	DepartmentID int64  `json:"d,omitempty"`
}

func encodeRecipientRef(connectionID, userID string) string {
	raw, _ := json.Marshal(recipientReference{Version: 1, ConnectionID: connectionID, UserID: userID})
	return base64.RawURLEncoding.EncodeToString(raw)
}
func decodeRecipientRef(value, connectionID string) (string, error) {
	var ref recipientReference
	if err := decodeRef(value, &ref); err != nil {
		return "", err
	}
	if ref.Version != 1 || ref.ConnectionID != connectionID || !validOpaqueID(ref.UserID, 128) {
		return "", fmt.Errorf("invalid recipient reference")
	}
	return ref.UserID, nil
}
func encodeMessageRef(connectionID string, taskID int64, userID string) string {
	raw, _ := json.Marshal(messageReference{Version: 1, ConnectionID: connectionID, TaskID: taskID, UserID: userID})
	return base64.RawURLEncoding.EncodeToString(raw)
}
func decodeMessageRef(value, connectionID string) (messageReference, error) {
	var ref messageReference
	if err := decodeRef(value, &ref); err != nil {
		return messageReference{}, err
	}
	validMember := ref.DepartmentID == 0 && validOpaqueID(ref.UserID, 128)
	validDepartment := ref.UserID == "" && ref.DepartmentID > 0
	if ref.Version != 1 || ref.ConnectionID != connectionID || ref.TaskID <= 0 || (!validMember && !validDepartment) {
		return messageReference{}, fmt.Errorf("invalid message reference")
	}
	return ref, nil
}
func decodeRef(value string, out interface{}) error {
	raw, err := base64.RawURLEncoding.DecodeString(strings.TrimSpace(value))
	if err != nil {
		return err
	}
	return json.Unmarshal(raw, out)
}
func validOpaqueID(value string, max int) bool {
	value = strings.TrimSpace(value)
	return value != "" && len(value) <= max && !strings.ContainsAny(value, "|\r\n\x00")
}

func isExpiredToken(status int, code string) bool {
	lower := strings.ToLower(strings.TrimSpace(code))
	return status == http.StatusUnauthorized || strings.Contains(lower, "token") && (strings.Contains(lower, "invalid") || strings.Contains(lower, "expire"))
}
func isLegacyExpiredToken(code int) bool {
	return code == 88 || code == 40001 || code == 40014 || code == 42001
}
func mapAPIError(status int, envelope apiErrorEnvelope) error {
	diagnostics := integrations.ProviderDiagnostics{
		ErrorCode:  envelope.Code,
		RequestID:  envelope.RequestID,
		HTTPStatus: status,
	}
	providerError := func(code, message string) error {
		return integrations.NewProviderError(code, message, nil, diagnostics)
	}
	switch status {
	case http.StatusUnauthorized:
		return providerError(integrations.ErrorCodeAuthInvalid, "DingTalk authentication failed")
	case http.StatusForbidden:
		if isExplicitDingTalkScopeDenial(envelope.Code) {
			return providerError(integrations.ErrorCodeInsufficientScope, "DingTalk application has not granted this operation")
		}
		return providerError(integrations.ErrorCodeAccessDenied, "DingTalk application does not have the required permission")
	case http.StatusTooManyRequests:
		return providerError(integrations.ErrorCodeRateLimited, "DingTalk rate limit was reached")
	}
	if status >= 500 {
		return providerError(integrations.ErrorCodeUpstream, "DingTalk service is unavailable")
	}
	if isExplicitDingTalkScopeDenial(envelope.Code) {
		return providerError(integrations.ErrorCodeInsufficientScope, "DingTalk application has not granted this operation")
	}
	if strings.Contains(strings.ToLower(envelope.Code), "permission") {
		return providerError(integrations.ErrorCodeAccessDenied, "DingTalk application does not have the required permission")
	}
	return providerError(integrations.ErrorCodeProviderRejected, "DingTalk rejected the operation")
}

func isExplicitDingTalkScopeDenial(code string) bool {
	normalized := strings.NewReplacer(".", "", "_", "", "-", "", " ", "").Replace(strings.ToLower(strings.TrimSpace(code)))
	return strings.Contains(normalized, "accesstokenpermissiondenied") ||
		(strings.Contains(normalized, "scope") &&
			(strings.Contains(normalized, "missing") || strings.Contains(normalized, "denied")))
}

func mapLegacyError(status int, envelope legacyEnvelope) error {
	diagnostics := integrations.ProviderDiagnostics{
		ErrorCode:  strconv.Itoa(envelope.ErrCode),
		RequestID:  envelope.RequestID,
		HTTPStatus: status,
	}
	providerError := func(code, message string) error {
		return integrations.NewProviderError(code, message, nil, diagnostics)
	}
	if isLegacyExpiredToken(envelope.ErrCode) {
		return providerError(integrations.ErrorCodeAuthInvalid, "DingTalk authentication failed")
	}
	switch envelope.ErrCode {
	case 60011, 60020, 70001, 70004:
		return providerError(integrations.ErrorCodeAccessDenied, "DingTalk application does not have the required permission")
	case 90018, 90006, 71006:
		return providerError(integrations.ErrorCodeRateLimited, "DingTalk rate limit was reached")
	case 60121, 33012, 33013:
		return providerError(integrations.ErrorCodeProviderRejected, "DingTalk rejected the intended recipient")
	}
	if status == http.StatusTooManyRequests {
		return providerError(integrations.ErrorCodeRateLimited, "DingTalk rate limit was reached")
	}
	if status >= 500 {
		return providerError(integrations.ErrorCodeUpstream, "DingTalk service is unavailable")
	}
	return providerError(integrations.ErrorCodeProviderRejected, "DingTalk rejected the operation")
}
func dingError(code, message string, err error) error {
	return integrations.NewError(code, message, err)
}
func actionResult(output map[string]interface{}, count int) *integrations.ActionResult {
	if output == nil {
		return nil
	}
	return &integrations.ActionResult{Output: output, ResultCount: count, AttemptCount: 1}
}
func inputString(input map[string]interface{}, key string) string {
	value, _ := input[key].(string)
	return value
}
func inputInt(input map[string]interface{}, key string, fallback int) int {
	switch value := input[key].(type) {
	case float64:
		return int(value)
	case int:
		return value
	}
	return fallback
}
func bounded(value string, limit int) string {
	value = strings.TrimSpace(value)
	runes := []rune(value)
	if len(runes) > limit {
		return string(runes[:limit])
	}
	return value
}
func contains(values []string, target string) bool {
	for _, value := range values {
		if value == target {
			return true
		}
	}
	return false
}
