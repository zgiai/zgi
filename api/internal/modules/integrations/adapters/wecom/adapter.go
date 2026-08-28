package wecom

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

const defaultBaseURL = "https://qyapi.weixin.qq.com"

type Adapter struct {
	client  *http.Client
	baseURL string
	mu      sync.Mutex
	tokens  map[string]cachedToken
}
type cachedToken struct {
	value          string
	expiresAt      time.Time
	credentialHash [32]byte
}
type credentials struct{ CorpID, AgentID, Secret, ConnectionID string }
type apiEnvelope struct {
	ErrCode      int    `json:"errcode"`
	ErrMsg       string `json:"errmsg"`
	RequestID    string `json:"requestid"`
	RequestIDAlt string `json:"request_id"`
}

func New(httpClient *http.Client) (*Adapter, error) { return newForBaseURL(httpClient, defaultBaseURL) }
func newForBaseURL(httpClient *http.Client, baseURL string) (*Adapter, error) {
	parsed, err := url.Parse(strings.TrimSpace(baseURL))
	if err != nil || parsed.Scheme == "" || parsed.Host == "" {
		return nil, fmt.Errorf("invalid WeCom base URL")
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
	return &Adapter{client: httpClient, baseURL: strings.TrimRight(baseURL, "/"), tokens: map[string]cachedToken{}}, nil
}
func (adapter *Adapter) DriverID() string { return DriverID }

func (adapter *Adapter) ValidateCredentials(_ context.Context, request integrations.CredentialValidationRequest) error {
	_, err := parseCredentials(&integrations.ResolvedConnection{
		ID:            "candidate",
		IntegrationID: request.IntegrationID,
		DriverID:      request.DriverID,
		AuthMethodID:  request.AuthMethodID,
		Credentials:   request.Credentials,
		Config:        request.Config,
	})
	return err
}

func (adapter *Adapter) Execute(ctx context.Context, request integrations.ActionRequest) (*integrations.ActionResult, error) {
	creds, err := parseCredentials(request.Connection)
	if err != nil {
		return nil, err
	}
	switch request.ActionID {
	case ActionAppGet:
		output, err := adapter.getApp(ctx, creds)
		return wecomResult(output, 1), err
	case ActionDepartmentList:
		output, count, err := adapter.listDepartments(ctx, creds, inputInt(request.Input, "department_id", 0))
		return wecomResult(output, count), err
	case ActionContactSearch:
		output, count, err := adapter.searchContacts(ctx, creds, request.Input)
		return wecomResult(output, count), err
	case ActionUserGet:
		output, err := adapter.getUser(ctx, creds, inputString(request.Input, "recipient_ref"))
		return wecomResult(output, 1), err
	case ActionMessageSendUser:
		output, err := adapter.sendUser(ctx, creds, request.Input)
		return wecomResult(output, 1), err
	default:
		return nil, wecomError(integrations.ErrorCodeInvalidInput, "WeCom action is not supported", nil)
	}
}
func (adapter *Adapter) ValidateConnection(ctx context.Context, connection *integrations.ResolvedConnection) (*integrations.ConnectionProfile, error) {
	creds, err := parseCredentials(connection)
	if err != nil {
		return nil, err
	}
	output, err := adapter.getApp(ctx, creds)
	if err != nil {
		return nil, err
	}
	app, _ := output["application"].(map[string]interface{})
	name, _ := app["name"].(string)
	return &integrations.ConnectionProfile{AccountID: creds.CorpID + ":" + creds.AgentID, DisplayName: firstNonEmpty(name, "WeCom application"), GrantedScopes: []string{ScopeApp, ScopeContacts, ScopeSend}}, nil
}
func (adapter *Adapter) ProbeConnection(ctx context.Context, connection *integrations.ResolvedConnection) (*integrations.HealthProbeReport, error) {
	profile, err := adapter.ValidateConnection(ctx, connection)
	if err != nil {
		status := integrations.HealthProbeStatusUnhealthy
		if code := integrations.ErrorCode(err); code == integrations.ErrorCodeTimeout || code == integrations.ErrorCodeUpstream || code == integrations.ErrorCodeRateLimited {
			status = integrations.HealthProbeStatusDegraded
		}
		return &integrations.HealthProbeReport{Status: status, Checks: []integrations.HealthProbeCheck{{Code: integrations.ErrorCode(err), Status: status, Message: "WeCom application check failed"}}}, err
	}
	return &integrations.HealthProbeReport{Status: integrations.HealthProbeStatusHealthy, Profile: profile, Checks: []integrations.HealthProbeCheck{{Code: "wecom_application_authenticated", Status: integrations.HealthProbeStatusHealthy}}}, nil
}

func (adapter *Adapter) getApp(ctx context.Context, creds credentials) (map[string]interface{}, error) {
	var response struct {
		apiEnvelope
		AgentID       int64  `json:"agentid"`
		Name          string `json:"name"`
		SquareLogoURL string `json:"square_logo_url"`
	}
	if err := adapter.authorizedJSON(ctx, creds, http.MethodGet, "/cgi-bin/agent/get", url.Values{"agentid": []string{creds.AgentID}}, nil, &response); err != nil {
		return nil, err
	}
	return map[string]interface{}{"provider": IntegrationID, "application": map[string]interface{}{"agent_id": strconv.FormatInt(response.AgentID, 10), "name": bounded(response.Name, 255), "square_logo_url": safeHTTPSURL(response.SquareLogoURL)}}, nil
}
func (adapter *Adapter) listDepartments(ctx context.Context, creds credentials, departmentID int) (map[string]interface{}, int, error) {
	var response struct {
		apiEnvelope
		Departments []struct {
			ID       int    `json:"id"`
			Name     string `json:"name"`
			ParentID int    `json:"parentid"`
		} `json:"department"`
	}
	query := url.Values{}
	if departmentID > 0 {
		query.Set("id", strconv.Itoa(departmentID))
	}
	if err := adapter.authorizedJSON(ctx, creds, http.MethodGet, "/cgi-bin/department/list", query, nil, &response); err != nil {
		return nil, 0, err
	}
	if len(response.Departments) > 200 {
		response.Departments = response.Departments[:200]
	}
	items := make([]map[string]interface{}, 0, len(response.Departments))
	for _, department := range response.Departments {
		items = append(items, map[string]interface{}{"id": department.ID, "name": bounded(department.Name, 255), "parent_id": department.ParentID})
	}
	return map[string]interface{}{"provider": IntegrationID, "departments": items}, len(items), nil
}
func (adapter *Adapter) searchContacts(ctx context.Context, creds credentials, input map[string]interface{}) (map[string]interface{}, int, error) {
	query := strings.ToLower(strings.TrimSpace(inputString(input, "query")))
	if query == "" {
		return nil, 0, wecomError(integrations.ErrorCodeInvalidInput, "WeCom member search query is required", nil)
	}
	departmentID := inputInt(input, "department_id", 1)
	if departmentID < 1 {
		departmentID = 1
	}
	limit := inputInt(input, "max_results", 10)
	if limit < 1 {
		limit = 10
	}
	if limit > 20 {
		limit = 20
	}
	var response struct {
		apiEnvelope
		Users []struct {
			UserID     string `json:"userid"`
			Name       string `json:"name"`
			Department []int  `json:"department"`
		} `json:"userlist"`
	}
	values := url.Values{"department_id": []string{strconv.Itoa(departmentID)}, "fetch_child": []string{"1"}}
	if err := adapter.authorizedJSON(ctx, creds, http.MethodGet, "/cgi-bin/user/simplelist", values, nil, &response); err != nil {
		return nil, 0, err
	}
	items := make([]map[string]interface{}, 0, limit)
	for _, user := range response.Users {
		if !strings.Contains(strings.ToLower(strings.TrimSpace(user.Name)), query) {
			continue
		}
		items = append(items, map[string]interface{}{"recipient_ref": encodeRecipientRef(user.UserID), "name": bounded(user.Name, 255), "department_ids": user.Department})
		if len(items) >= limit {
			break
		}
	}
	return map[string]interface{}{"provider": IntegrationID, "members": items}, len(items), nil
}
func (adapter *Adapter) getUser(ctx context.Context, creds credentials, recipientRef string) (map[string]interface{}, error) {
	userID, err := decodeRecipientRef(recipientRef)
	if err != nil {
		return nil, wecomError(integrations.ErrorCodeInvalidInput, "WeCom recipient reference is invalid", err)
	}
	var response struct {
		apiEnvelope
		UserID     string `json:"userid"`
		Name       string `json:"name"`
		Department []int  `json:"department"`
		Position   string `json:"position"`
		Status     int    `json:"status"`
	}
	if err := adapter.authorizedJSON(ctx, creds, http.MethodGet, "/cgi-bin/user/get", url.Values{"userid": []string{userID}}, nil, &response); err != nil {
		return nil, err
	}
	return map[string]interface{}{"provider": IntegrationID, "member": map[string]interface{}{"recipient_ref": recipientRef, "name": bounded(response.Name, 255), "department_ids": response.Department, "position": bounded(response.Position, 255), "status": response.Status}}, nil
}
func (adapter *Adapter) sendUser(ctx context.Context, creds credentials, input map[string]interface{}) (map[string]interface{}, error) {
	recipientRef := strings.TrimSpace(inputString(input, "recipient_ref"))
	userID, err := decodeRecipientRef(recipientRef)
	if err != nil {
		return nil, wecomError(integrations.ErrorCodeInvalidInput, "WeCom recipient reference is invalid", err)
	}
	content := strings.TrimSpace(inputString(input, "content"))
	if content == "" || len([]rune(content)) > 2048 {
		return nil, wecomError(integrations.ErrorCodeInvalidInput, "WeCom message content is invalid", nil)
	}
	agentID, err := strconv.ParseInt(creds.AgentID, 10, 64)
	if err != nil {
		return nil, wecomError(integrations.ErrorCodeConnectionInvalid, "WeCom Agent ID is invalid", err)
	}
	payload := map[string]interface{}{"touser": userID, "msgtype": "text", "agentid": agentID, "text": map[string]string{"content": content}, "safe": 0, "enable_duplicate_check": 1, "duplicate_check_interval": 1800}
	var response struct {
		apiEnvelope
		MsgID       string `json:"msgid"`
		InvalidUser string `json:"invaliduser"`
	}
	if err := adapter.authorizedJSON(ctx, creds, http.MethodPost, "/cgi-bin/message/send", nil, payload, &response); err != nil {
		return nil, err
	}
	if strings.TrimSpace(response.InvalidUser) != "" {
		return nil, wecomError(integrations.ErrorCodeProviderRejected, "WeCom rejected the intended recipient", nil)
	}
	return map[string]interface{}{"provider": IntegrationID, "message": map[string]interface{}{"message_id": bounded(response.MsgID, 255), "recipient_ref": recipientRef, "provider_accepted": true}}, nil
}

func (adapter *Adapter) authorizedJSON(ctx context.Context, creds credentials, method, path string, query url.Values, body interface{}, out interface{}) error {
	for attempt := 0; attempt < 2; attempt++ {
		token, err := adapter.accessToken(ctx, creds)
		if err != nil {
			return err
		}
		values := url.Values{}
		for key, list := range query {
			for _, value := range list {
				values.Add(key, value)
			}
		}
		values.Set("access_token", token)
		status, envelope, err := adapter.doJSON(ctx, method, path, values, body, out)
		if err != nil {
			return err
		}
		if envelope.ErrCode == 0 {
			return nil
		}
		if (envelope.ErrCode == 40014 || envelope.ErrCode == 42001) && attempt == 0 {
			adapter.evictToken(creds)
			continue
		}
		return mapProviderError(status, envelope)
	}
	return wecomError(integrations.ErrorCodeAuthInvalid, "WeCom access token is invalid", nil)
}
func (adapter *Adapter) accessToken(ctx context.Context, creds credentials) (string, error) {
	hash := sha256.Sum256([]byte(creds.CorpID + "\x00" + creds.AgentID + "\x00" + creds.Secret))
	adapter.mu.Lock()
	cached, ok := adapter.tokens[creds.ConnectionID]
	adapter.mu.Unlock()
	if ok && cached.credentialHash == hash && time.Now().Before(cached.expiresAt) {
		return cached.value, nil
	}
	var response struct {
		apiEnvelope
		AccessToken string `json:"access_token"`
		ExpiresIn   int    `json:"expires_in"`
	}
	status, envelope, err := adapter.doJSON(ctx, http.MethodGet, "/cgi-bin/gettoken", url.Values{"corpid": []string{creds.CorpID}, "corpsecret": []string{creds.Secret}}, nil, &response)
	if err != nil {
		return "", err
	}
	if envelope.ErrCode != 0 {
		return "", mapProviderError(status, envelope)
	}
	if strings.TrimSpace(response.AccessToken) == "" {
		return "", wecomError(integrations.ErrorCodeResponseInvalid, "WeCom access token response is incomplete", nil)
	}
	ttl := time.Duration(response.ExpiresIn)*time.Second - 5*time.Minute
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
func (adapter *Adapter) doJSON(ctx context.Context, method, path string, query url.Values, body interface{}, out interface{}) (int, apiEnvelope, error) {
	target := adapter.baseURL + path
	if len(query) > 0 {
		target += "?" + query.Encode()
	}
	var reader io.Reader
	if body != nil {
		raw, err := json.Marshal(body)
		if err != nil {
			return 0, apiEnvelope{}, err
		}
		reader = bytes.NewReader(raw)
	}
	request, err := http.NewRequestWithContext(ctx, method, target, reader)
	if err != nil {
		return 0, apiEnvelope{}, err
	}
	request.Header.Set("Accept", "application/json")
	if body != nil {
		request.Header.Set("Content-Type", "application/json")
	}
	response, err := adapter.client.Do(request)
	if err != nil {
		if ctx.Err() != nil {
			return 0, apiEnvelope{}, wecomError(integrations.ErrorCodeTimeout, "WeCom request timed out", ctx.Err())
		}
		return 0, apiEnvelope{}, wecomError(integrations.ErrorCodeUpstream, "WeCom service is unavailable", err)
	}
	defer response.Body.Close()
	limited, err := io.ReadAll(io.LimitReader(response.Body, 2<<20))
	if err != nil {
		return response.StatusCode, apiEnvelope{}, wecomError(integrations.ErrorCodeResponseInvalid, "WeCom response could not be read", err)
	}
	var envelope apiEnvelope
	if err := json.Unmarshal(limited, &envelope); err != nil {
		return response.StatusCode, envelope, wecomError(integrations.ErrorCodeResponseInvalid, "WeCom response is invalid", err)
	}
	if out != nil {
		if err := json.Unmarshal(limited, out); err != nil {
			return response.StatusCode, envelope, wecomError(integrations.ErrorCodeResponseInvalid, "WeCom response is invalid", err)
		}
	}
	return response.StatusCode, envelope, nil
}

func parseCredentials(connection *integrations.ResolvedConnection) (credentials, error) {
	if connection == nil || !strings.EqualFold(connection.IntegrationID, IntegrationID) || !strings.EqualFold(connection.DriverID, DriverID) {
		return credentials{}, wecomError(integrations.ErrorCodeConnectionInvalid, "WeCom connection is invalid", nil)
	}
	creds := credentials{CorpID: strings.TrimSpace(connection.Credentials["corp_id"]), AgentID: strings.TrimSpace(connection.Credentials["agent_id"]), Secret: strings.TrimSpace(connection.Credentials["secret"]), ConnectionID: connection.ID}
	if creds.CorpID == "" || creds.AgentID == "" || creds.Secret == "" || len(creds.CorpID) > 128 || len(creds.AgentID) > 32 || len(creds.Secret) > 512 {
		return credentials{}, wecomError(integrations.ErrorCodeConnectionInvalid, "WeCom credentials are unavailable", nil)
	}
	if _, err := strconv.ParseInt(creds.AgentID, 10, 64); err != nil {
		return credentials{}, wecomError(integrations.ErrorCodeConnectionInvalid, "WeCom Agent ID is invalid", err)
	}
	return creds, nil
}
func mapProviderError(status int, envelope apiEnvelope) error {
	diagnostics := integrations.ProviderDiagnostics{
		ErrorCode:  strconv.Itoa(envelope.ErrCode),
		RequestID:  firstNonEmpty(envelope.RequestID, envelope.RequestIDAlt),
		HTTPStatus: status,
	}
	providerError := func(code, message string) error {
		return integrations.NewProviderError(code, message, nil, diagnostics)
	}
	switch envelope.ErrCode {
	case 40013, 40014, 40001, 42001:
		return providerError(integrations.ErrorCodeAuthInvalid, "WeCom authentication failed")
	case 60020:
		return providerError(integrations.ErrorCodeAccessDenied, "WeCom trusted IP does not allow this server")
	case 60011, 48002, 50001:
		return providerError(integrations.ErrorCodeAccessDenied, "WeCom application does not have the required permission")
	case 45009:
		return providerError(integrations.ErrorCodeRateLimited, "WeCom rate limit was reached")
	case 40003, 60111:
		return providerError(integrations.ErrorCodeProviderRejected, "WeCom rejected the requested member")
	}
	if status == 429 {
		return providerError(integrations.ErrorCodeRateLimited, "WeCom rate limit was reached")
	}
	if status >= 500 {
		return providerError(integrations.ErrorCodeUpstream, "WeCom service is unavailable")
	}
	return providerError(integrations.ErrorCodeProviderRejected, "WeCom rejected the operation")
}
func wecomError(code, message string, err error) error {
	return integrations.NewError(code, message, err)
}
func encodeRecipientRef(userID string) string {
	raw, _ := json.Marshal(map[string]string{"v": "1", "u": userID})
	return base64.RawURLEncoding.EncodeToString(raw)
}
func decodeRecipientRef(value string) (string, error) {
	raw, err := base64.RawURLEncoding.DecodeString(strings.TrimSpace(value))
	if err != nil {
		return "", err
	}
	var data map[string]string
	if err = json.Unmarshal(raw, &data); err != nil {
		return "", err
	}
	userID := strings.TrimSpace(data["u"])
	if data["v"] != "1" || userID == "" || len(userID) > 64 || strings.ContainsAny(userID, "|\r\n") {
		return "", fmt.Errorf("invalid recipient")
	}
	return userID, nil
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
func safeHTTPSURL(value string) string {
	parsed, err := url.Parse(strings.TrimSpace(value))
	if err != nil || parsed.Scheme != "https" || parsed.Host == "" {
		return ""
	}
	return bounded(parsed.String(), 2048)
}
func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return strings.TrimSpace(value)
		}
	}
	return ""
}
func wecomResult(output map[string]interface{}, count int) *integrations.ActionResult {
	if output == nil {
		return nil
	}
	return &integrations.ActionResult{Output: output, ResultCount: count, AttemptCount: 1}
}
