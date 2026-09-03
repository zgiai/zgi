package wecom

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/zgiai/zgi/api/internal/modules/integrations/core"
)

const (
	defaultBaseURL                = "https://qyapi.weixin.qq.com"
	maxContactSearchProviderCalls = 32
)

type Adapter struct {
	client        *http.Client
	baseURL       string
	suite         SuiteConfig
	suiteState    *SuiteStateRepository
	mu            sync.Mutex
	tokens        map[string]cachedToken
	suiteToken    cachedToken
	providerToken cachedToken
}
type cachedToken struct {
	value          string
	expiresAt      time.Time
	credentialHash [32]byte
}
type credentials struct {
	AuthCorpID, AgentID, PermanentCode, ConnectionID string
	CredentialVersion, Revision                      int
}
type apiEnvelope struct {
	ErrCode      int    `json:"errcode"`
	ErrMsg       string `json:"errmsg"`
	RequestID    string `json:"requestid"`
	RequestIDAlt string `json:"request_id"`
}

type userSummary struct {
	UserID     string `json:"userid"`
	Name       string `json:"name"`
	Department []int  `json:"department"`
}

type userDetails struct {
	apiEnvelope
	UserID     string `json:"userid"`
	Name       string `json:"name"`
	Department []int  `json:"department"`
	Position   string `json:"position"`
	Status     int    `json:"status"`
}

type departmentSummary struct {
	ID       int64  `json:"id"`
	Name     string `json:"name"`
	ParentID int64  `json:"parentid"`
}

func New(httpClient *http.Client) (*Adapter, error) { return newForBaseURL(httpClient, defaultBaseURL) }

func NewSuite(httpClient *http.Client, config SuiteConfig, state *SuiteStateRepository) (*Adapter, error) {
	adapter, err := newForBaseURL(httpClient, defaultBaseURL)
	if err != nil {
		return nil, err
	}
	adapter.suite = config
	adapter.suiteState = state
	return adapter, nil
}
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
		output, count, err := adapter.searchMembers(ctx, creds, request.Input)
		return wecomResult(output, count), err
	case ActionUserGet:
		output, err := adapter.getUser(ctx, creds, inputString(request.Input, "recipient_ref"))
		return wecomResult(output, 1), err
	case ActionMessageSendUser:
		output, count, err := adapter.sendUser(ctx, creds, request.Input)
		return wecomResult(output, count), err
	default:
		if result, handled, customerErr := adapter.executeCustomerAction(ctx, request.ActionID, creds, request.Input); handled {
			return result, customerErr
		}
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
	if adapter.suite.CustomerContactEnabled {
		var customerScopeProbe struct {
			apiEnvelope
			FollowUsers []string `json:"follow_user"`
		}
		if err := adapter.authorizedJSON(ctx, creds, http.MethodGet, "/cgi-bin/externalcontact/get_follow_user_list", nil, nil, &customerScopeProbe); err != nil {
			return nil, err
		}
	}
	return &integrations.ConnectionProfile{AccountID: adapter.externalTenantHash(creds.AuthCorpID), DisplayName: firstNonEmpty(name, "WeCom application"), GrantedScopes: declaredGrantedScopes(adapter.suite), ScopeEvidence: integrations.AuthScopeEvidenceConnectorDeclared}, nil
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
		Departments []departmentSummary `json:"department"`
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
		items = append(items, map[string]interface{}{"id": department.ID, "name": providerDepartmentName(department.Name, department.ID), "parent_id": department.ParentID})
	}
	return map[string]interface{}{"provider": IntegrationID, "departments": items}, len(items), nil
}
func (adapter *Adapter) searchContacts(ctx context.Context, creds credentials, input map[string]interface{}) (map[string]interface{}, int, error) {
	query := strings.ToLower(strings.TrimSpace(inputString(input, "query")))
	if query == "" {
		return nil, 0, wecomError(integrations.ErrorCodeInvalidInput, "WeCom member search query is required", nil)
	}
	departmentID := inputInt(input, "department_id", 0)
	limit := inputInt(input, "max_results", 10)
	if limit < 1 {
		limit = 10
	}
	if limit > 20 {
		limit = 20
	}
	agentID, err := strconv.ParseInt(creds.AgentID, 10, 64)
	if err != nil {
		return nil, 0, wecomError(integrations.ErrorCodeConnectionInvalid, "WeCom Agent ID is invalid", err)
	}
	_, authorizedAgent, err := adapter.getAuthorizationInfo(ctx, creds.AuthCorpID, creds.PermanentCode, agentID)
	if err != nil {
		return nil, 0, err
	}
	if err := authorizeContactRead(authorizedAgent); err != nil {
		return nil, 0, err
	}
	authorizedUsers := authorizedAgent.Privilege.effectiveUsers()
	authorizedParties := authorizedAgent.Privilege.effectiveParties()
	authorizedTags := authorizedAgent.Privilege.effectiveTags()
	authMode := *authorizedAgent.AuthMode
	if authMode == wecomAuthorizationModeMember {
		// Member authorization cannot call department APIs, so party subjects are
		// not executable search roots in this mode.
		authorizedParties = nil
	}
	if len(authorizedUsers) == 0 && len(authorizedParties) == 0 && len(authorizedTags) == 0 {
		return nil, 0, integrations.NewErrorWithReason(
			integrations.ErrorCodeInsufficientScope,
			"wecom_visible_scope_empty",
			"WeCom authorization has no searchable visible subjects",
			nil,
		)
	}
	fixedProviderCalls := len(authorizedUsers) + len(authorizedTags) + len(authorizedParties)
	if fixedProviderCalls > maxContactSearchProviderCalls {
		return nil, 0, contactProviderCallBudgetError()
	}
	if authMode == wecomAuthorizationModeMember && departmentID > 0 {
		return nil, 0, integrations.NewErrorWithReason(
			integrations.ErrorCodeInsufficientScope,
			"wecom_member_authorization_has_no_department_access",
			"WeCom member authorization cannot query department members",
			nil,
		)
	}
	items := make([]map[string]interface{}, 0, limit)
	seen := make(map[string]struct{})
	appendUsers := func(users []userSummary) bool {
		for _, user := range users {
			userID := strings.TrimSpace(user.UserID)
			if userID == "" || len(userID) > 64 {
				continue
			}
			key := strings.ToLower(userID)
			if _, exists := seen[key]; exists {
				continue
			}
			displayName := providerDisplayName(user.Name, userID)
			nameMatches := strings.Contains(strings.ToLower(displayName), query)
			userIDMatches := strings.Contains(strings.ToLower(userID), query)
			if !nameMatches && !userIDMatches {
				continue
			}
			seen[key] = struct{}{}
			departments := user.Department
			if departments == nil {
				departments = []int{}
			}
			recipientRef, encodeErr := adapter.encodeSubjectRef(creds, SubjectTypeMember, SubjectIDKindUserID, userID, "", "")
			if encodeErr != nil {
				continue
			}
			items = append(items, map[string]interface{}{"recipient_ref": recipientRef, "name": displayName, "department_ids": departments})
			if len(items) >= limit {
				return true
			}
		}
		return false
	}

	if departmentID > 0 {
		users, listErr := adapter.listDepartmentUsers(ctx, creds, int64(departmentID))
		if listErr != nil {
			return nil, 0, listErr
		}
		appendUsers(users)
		return map[string]interface{}{"provider": IntegrationID, "members": items}, len(items), nil
	}

	// A single exact UserID lookup has a complete one-call plan and does not
	// require discovering unrelated authorized department trees first.
	if limit == 1 {
		for _, userID := range authorizedUsers {
			if !strings.EqualFold(strings.TrimSpace(userID), query) {
				continue
			}
			user, readErr := adapter.readUser(ctx, creds, userID)
			if readErr != nil {
				return nil, 0, readErr
			}
			appendUsers([]userSummary{{UserID: userID, Name: user.Name, Department: user.Department}})
			return map[string]interface{}{"provider": IntegrationID, "members": items}, len(items), nil
		}
	}

	// Preflight the complete worst-case subject-call budget before user/get,
	// user/simplelist, or tag/get is called. Department discovery is the only
	// provider work needed to learn the exact number of department member calls.
	departmentIDs := []int64(nil)
	if authMode == wecomAuthorizationModeAdministrator && len(authorizedParties) > 0 {
		departmentIDs, err = adapter.listAuthorizedDepartmentIDs(
			ctx,
			creds,
			authorizedParties,
			maxContactSearchProviderCalls-fixedProviderCalls,
		)
		if err != nil {
			return nil, 0, err
		}
	}

	// Explicitly authorized members are not necessarily descendants of an
	// authorized department, so resolve them before walking department roots.
	for _, userID := range authorizedUsers {
		user, readErr := adapter.readUser(ctx, creds, userID)
		if readErr != nil {
			return nil, 0, readErr
		}
		if appendUsers([]userSummary{{UserID: user.UserID, Name: user.Name, Department: user.Department}}) {
			return map[string]interface{}{"provider": IntegrationID, "members": items}, len(items), nil
		}
	}
	if authMode == wecomAuthorizationModeAdministrator {
		for _, partyID := range departmentIDs {
			users, listErr := adapter.listDepartmentUsers(ctx, creds, partyID)
			if listErr != nil {
				return nil, 0, listErr
			}
			if appendUsers(users) {
				return map[string]interface{}{"provider": IntegrationID, "members": items}, len(items), nil
			}
		}
	}
	for _, tagID := range authorizedTags {
		users, listErr := adapter.listTagUsers(ctx, creds, tagID)
		if listErr != nil {
			return nil, 0, listErr
		}
		if appendUsers(users) {
			return map[string]interface{}{"provider": IntegrationID, "members": items}, len(items), nil
		}
	}
	return map[string]interface{}{"provider": IntegrationID, "members": items}, len(items), nil
}

func authorizeContactRead(agent authorizationAgent) error {
	level := *agent.Privilege.Level
	if agent.IsCustomizedApp && level == 0 {
		return integrations.NewErrorWithReason(
			integrations.ErrorCodeInsufficientScope,
			"wecom_customized_app_permissions_unsupported",
			"WeCom customized application permissions require a separate permission endpoint",
			nil,
		)
	}
	if level == 5 {
		return integrations.NewErrorWithReason(
			integrations.ErrorCodeInsufficientScope,
			"wecom_contacts_read_permission_missing",
			"WeCom application privilege does not allow contact reads",
			nil,
		)
	}
	return nil
}

// listAuthorizedDepartmentIDs expands only the roots reported by get_auth_info.
// simplelist has no recursive option, so each resulting department is queried
// separately. Keep the traversal bounded so one broad authorization cannot turn
// one model action into an unbounded sequence of provider requests.
func (adapter *Adapter) listAuthorizedDepartmentIDs(ctx context.Context, creds credentials, roots []int64, maxDepartmentCalls int) ([]int64, error) {
	if len(roots) > maxDepartmentCalls {
		return nil, contactProviderCallBudgetError()
	}
	seen := make(map[int64]struct{}, len(roots))
	departmentIDs := make([]int64, 0, len(roots))
	appendDepartment := func(departmentID int64) error {
		if departmentID <= 0 {
			return nil
		}
		if _, exists := seen[departmentID]; exists {
			return nil
		}
		if len(departmentIDs) >= maxDepartmentCalls {
			return contactProviderCallBudgetError()
		}
		seen[departmentID] = struct{}{}
		departmentIDs = append(departmentIDs, departmentID)
		return nil
	}
	for _, rootID := range roots {
		if _, exists := seen[rootID]; exists {
			continue
		}
		if err := appendDepartment(rootID); err != nil {
			return nil, err
		}
		var response struct {
			apiEnvelope
			Departments []departmentSummary `json:"department"`
		}
		if err := adapter.authorizedJSON(ctx, creds, http.MethodGet, "/cgi-bin/department/list", url.Values{"id": []string{strconv.FormatInt(rootID, 10)}}, nil, &response); err != nil {
			return nil, err
		}
		for _, department := range response.Departments {
			if err := appendDepartment(department.ID); err != nil {
				return nil, err
			}
		}
	}
	return departmentIDs, nil
}

func contactProviderCallBudgetError() error {
	return integrations.NewErrorWithReason(
		integrations.ErrorCodeInvalidInput,
		"wecom_provider_call_budget_exceeded",
		"WeCom visible subject range is too broad for one contact search; specify a department ID",
		nil,
	)
}

func (adapter *Adapter) listDepartmentUsers(ctx context.Context, creds credentials, departmentID int64) ([]userSummary, error) {
	if departmentID <= 0 {
		return nil, wecomError(integrations.ErrorCodeInvalidInput, "WeCom department ID is invalid", nil)
	}
	var response struct {
		apiEnvelope
		Users []userSummary `json:"userlist"`
	}
	values := url.Values{"department_id": []string{strconv.FormatInt(departmentID, 10)}}
	if err := adapter.authorizedJSON(ctx, creds, http.MethodGet, "/cgi-bin/user/simplelist", values, nil, &response); err != nil {
		return nil, err
	}
	return response.Users, nil
}

func (adapter *Adapter) listTagUsers(ctx context.Context, creds credentials, tagID int64) ([]userSummary, error) {
	if tagID <= 0 {
		return nil, wecomError(integrations.ErrorCodeInvalidInput, "WeCom tag ID is invalid", nil)
	}
	var response struct {
		apiEnvelope
		Users []userSummary `json:"userlist"`
	}
	if err := adapter.authorizedJSON(ctx, creds, http.MethodGet, "/cgi-bin/tag/get", url.Values{"tagid": []string{strconv.FormatInt(tagID, 10)}}, nil, &response); err != nil {
		return nil, err
	}
	return response.Users, nil
}

func (adapter *Adapter) readUser(ctx context.Context, creds credentials, userID string) (userDetails, error) {
	response := userDetails{}
	userID = strings.TrimSpace(userID)
	if userID == "" || len(userID) > 64 || strings.ContainsAny(userID, "|\r\n") {
		return response, wecomError(integrations.ErrorCodeInvalidInput, "WeCom member ID is invalid", nil)
	}
	if err := adapter.authorizedJSON(ctx, creds, http.MethodGet, "/cgi-bin/user/get", url.Values{"userid": []string{userID}}, nil, &response); err != nil {
		return response, err
	}
	responseUserID := strings.TrimSpace(response.UserID)
	if responseUserID != "" && !strings.EqualFold(responseUserID, userID) {
		return userDetails{}, wecomError(integrations.ErrorCodeResponseInvalid, "WeCom member details returned a mismatched member identifier", nil)
	}
	if responseUserID == "" {
		response.UserID = userID
	}
	return response, nil
}

func (adapter *Adapter) getUser(ctx context.Context, creds credentials, recipientRef string) (map[string]interface{}, error) {
	claims, err := adapter.decodeSubjectRef(recipientRef, creds, SubjectTypeMember, SubjectIDKindUserID)
	if err != nil {
		return nil, wecomError(integrations.ErrorCodeInvalidInput, "WeCom recipient reference is invalid", err)
	}
	response, err := adapter.readUser(ctx, creds, claims.SubjectID)
	if err != nil {
		return nil, err
	}
	departments := response.Department
	if departments == nil {
		departments = []int{}
	}
	return map[string]interface{}{"provider": IntegrationID, "member": map[string]interface{}{"recipient_ref": recipientRef, "name": providerDisplayName(response.Name, response.UserID), "department_ids": departments, "position": bounded(response.Position, 255), "status": response.Status}}, nil
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
	hash := sha256.Sum256([]byte(
		creds.AuthCorpID + "\x00" + creds.AgentID + "\x00" + creds.PermanentCode + "\x00" +
			strconv.Itoa(creds.CredentialVersion) + "\x00" + strconv.Itoa(creds.Revision),
	))
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
	suiteToken, err := adapter.suiteAccessToken(ctx)
	if err != nil {
		return "", err
	}
	status, envelope, err := adapter.doJSON(ctx, http.MethodPost, "/cgi-bin/service/get_corp_token", url.Values{"suite_access_token": []string{suiteToken}}, map[string]string{"auth_corpid": creds.AuthCorpID, "permanent_code": creds.PermanentCode}, &response)
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
	creds := credentials{
		AuthCorpID: strings.TrimSpace(connection.Credentials["auth_corpid"]), AgentID: strings.TrimSpace(connection.Credentials["agent_id"]),
		PermanentCode: strings.TrimSpace(connection.Credentials["permanent_code"]), ConnectionID: connection.ID,
		CredentialVersion: connection.CredentialVersion, Revision: connection.Revision,
	}
	if creds.AuthCorpID == "" || creds.AgentID == "" || creds.PermanentCode == "" || len(creds.AuthCorpID) > 128 || len(creds.AgentID) > 32 || len(creds.PermanentCode) > 512 {
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
	providerErrorWithReason := func(code, reason, message string) error {
		return integrations.NewProviderErrorWithReason(code, reason, message, nil, diagnostics)
	}
	switch envelope.ErrCode {
	case 40013, 40014, 40001, 42001:
		return providerError(integrations.ErrorCodeAuthInvalid, "WeCom authentication failed")
	case 60020:
		return providerErrorWithReason(integrations.ErrorCodeAccessDenied, "wecom_trusted_ip_denied", "WeCom trusted IP does not allow this server")
	case 60011:
		return providerErrorWithReason(integrations.ErrorCodeInsufficientScope, "wecom_subject_outside_visible_scope", "WeCom member, department, or tag is outside the authorized visible range")
	case 60021, 660003:
		return providerErrorWithReason(integrations.ErrorCodeInsufficientScope, "wecom_member_outside_visible_scope", "WeCom member is outside the authorized visible range")
	case 660004:
		return providerErrorWithReason(integrations.ErrorCodeInsufficientScope, "wecom_department_outside_visible_scope", "WeCom department is outside the authorized visible range")
	case 660005:
		return providerErrorWithReason(integrations.ErrorCodeInsufficientScope, "wecom_tag_outside_visible_scope", "WeCom tag is outside the authorized visible range")
	case 48002:
		return providerErrorWithReason(integrations.ErrorCodeInsufficientScope, "wecom_api_permission_missing", "WeCom application does not have permission to call this API")
	case 50001:
		return providerErrorWithReason(integrations.ErrorCodeConnectionInvalid, "wecom_trusted_domain_mismatch", "WeCom trusted domain does not allow this request")
	case 45009:
		return providerError(integrations.ErrorCodeRateLimited, "WeCom rate limit was reached")
	case 40003, 60111:
		return providerError(integrations.ErrorCodeProviderRejected, "WeCom rejected the requested member")
	case 81013:
		return providerErrorWithReason(integrations.ErrorCodeProviderRejected, "wecom_all_recipients_invalid_or_unauthorized", "WeCom rejected all intended recipients")
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

func providerDisplayName(name, userID string) string {
	name = bounded(name, 255)
	if strings.EqualFold(name, strings.TrimSpace(userID)) {
		return ""
	}
	return name
}

func providerDepartmentName(name string, departmentID int64) string {
	name = bounded(name, 255)
	if name == strconv.FormatInt(departmentID, 10) {
		return ""
	}
	return name
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

func wecomResultWithRequestID(output map[string]interface{}, count int, requestID string) *integrations.ActionResult {
	result := wecomResult(output, count)
	if result == nil {
		return nil
	}
	result.ProviderRequestID = bounded(requestID, 255)
	if result.ProviderRequestID != "" {
		result.ProviderDiagnostics.RequestID = result.ProviderRequestID
	}
	return result
}
