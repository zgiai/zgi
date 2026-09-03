// Package feishu implements bounded, app-identity OpenAPI operations. It has
// no event receiver, target lookup, user OAuth or ZGI agent dependency.
package feishu

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/url"
	"sort"
	"strings"
	"time"

	"github.com/zgiai/zgi/api/internal/modules/connectors/catalog"
	"github.com/zgiai/zgi/api/internal/modules/connectors/core"
	"github.com/zgiai/zgi/api/internal/modules/connectors/providers"
)

const maxResponseBytes = 1 << 20

type Client struct {
	http       *http.Client
	definition catalog.Definition
	schemas    *providers.Schemas
	files      ResourceStore
}

var _ providers.CredentialDriver = (*Client)(nil)

func New(client *http.Client, files ...ResourceStore) (*Client, error) {
	d, err := catalog.FeishuDefinition()
	if err != nil {
		return nil, err
	}
	schemas, err := providers.NewSchemasForDefinition(d)
	if err != nil {
		return nil, err
	}
	h := http.Client{Timeout: 10 * time.Second}
	if client != nil {
		h = *client
		if h.Timeout <= 0 || h.Timeout > 10*time.Second {
			h.Timeout = 10 * time.Second
		}
	}
	h.CheckRedirect = func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }
	c := &Client{http: &h, definition: d, schemas: schemas}
	if len(files) > 0 {
		c.files = files[0]
	}
	return c, nil
}
func (c *Client) ConnectorID() string { return "feishu" }

func (c *Client) VerifyCredentials(ctx context.Context, input providers.CredentialInput) (providers.Verification, error) {
	if catalog.ValidateCredentialValues(c.definition, input.Values) != nil {
		return providers.Verification{}, providers.Invalid()
	}
	token, err := c.tenantToken(ctx, input)
	if err != nil {
		return providers.Verification{}, err
	}
	response, requestID, err := c.call(ctx, input, "GET", "/open-apis/bot/v3/info", nil, token, nil, false)
	if err != nil {
		return providers.Verification{}, err
	}
	bot, ok := response["bot"].(map[string]any)
	if !ok || providers.Text(bot, "open_id") == "" {
		return providers.Verification{}, failure(core.ErrorCodeApplicationNotReady)
	}
	if status, exists := bot["activate_status"]; exists && status != float64(2) {
		return providers.Verification{}, failure(core.ErrorCodeApplicationNotReady)
	}
	if id := providers.Text(bot, "app_id"); id != "" && id != input.Values["app_id"] {
		return providers.Verification{}, failure(core.ErrorCodeInvalidAppCredentials)
	}
	name := providers.SafeText(providers.Text(bot, "app_name"), 128)
	for _, secret := range []string{input.Values["app_secret"], token} {
		if secret != "" {
			name = strings.ReplaceAll(name, secret, "[redacted]")
		}
	}
	evidence := &core.ProviderEvidence{Region: input.Values["region"], AppID: input.Values["app_id"], AppName: name, TemplateID: c.definition.Setup.TemplateID, GrantedScopes: []string{}, ScopeStatus: "unknown", EventConfiguration: "unknown"}
	granted := map[string]bool{}
	scopeResponse, _, scopeErr := c.call(ctx, input, "GET", "/open-apis/application/v6/scopes", nil, token, nil, false)
	if scopeErr == nil {
		data, _ := scopeResponse["data"].(map[string]any)
		if scopes, ok := data["scopes"].([]any); ok && len(scopes) <= 256 {
			evidence.ScopeStatus = "verified"
			for _, raw := range scopes {
				scope, ok := raw.(map[string]any)
				if !ok {
					continue
				}
				name := providers.Text(scope, "scope_name")
				// The documented tenant-grant endpoint historically omits scope_type.
				// Accept that response under the tenant token; explicitly user-scoped
				// (or unknown-subject) rows never authorize app operations.
				subject, typed := scope["scope_type"]
				if scope["grant_status"] == float64(1) && (!typed || subject == "tenant") && name != "" && len(name) <= 128 && !strings.ContainsAny(name, "\r\n ") {
					granted[name] = true
				}
			}
		}
	}
	// The actual successful bot probe is evidence only for this operation.
	usable := []string{"feishu.bot.get"}
	for _, op := range c.definition.Operations {
		if op.ID != "feishu.bot.get" && scopesAllow(op.RequiredScopes, granted) {
			usable = append(usable, op.ID)
		}
	}
	for scope := range granted {
		evidence.GrantedScopes = append(evidence.GrantedScopes, scope)
	}
	sort.Strings(evidence.GrantedScopes)
	return providers.Verification{AccountID: input.Values["region"] + ":" + input.Values["app_id"], DisplayName: name, Operations: usable, RequestID: requestID, Evidence: evidence}, nil
}

func scopesAllow(groups [][]string, granted map[string]bool) bool {
	if len(groups) == 0 {
		return false
	}
	for _, group := range groups {
		ok := false
		for _, scope := range group {
			ok = ok || granted[scope]
		}
		if !ok {
			return false
		}
	}
	return true
}

func (c *Client) InvokeCredentials(ctx context.Context, operation string, input providers.CredentialInput, args map[string]any) (providers.Response, error) {
	if catalog.ValidateCredentialValues(c.definition, input.Values) != nil {
		return providers.Response{}, providers.Invalid()
	}
	values, err := c.schemas.Input(operation, args)
	if err != nil {
		return providers.Response{}, err
	}
	if operation == "feishu.resource.get" || operation == "feishu.resource.upload" {
		return c.resource(ctx, operation, input, values)
	}
	request, err := operationRequest(operation, values)
	if err != nil {
		return providers.Response{}, err
	}
	token, err := c.tenantToken(ctx, input)
	if err != nil {
		return providers.Response{}, err
	}
	response, requestID, err := c.call(ctx, input, request.method, request.path, request.query, token, request.body, request.write)
	if err != nil {
		return providers.Response{}, err
	}
	data, _ := response["data"].(map[string]any)
	if operation == "feishu.bot.get" {
		data, _ = response["bot"].(map[string]any)
	}
	if data == nil {
		data = map[string]any{}
	}
	if operation == "feishu.contact.basic.get" {
		user, _ := data["user"].(map[string]any)
		basic := map[string]any{}
		for _, key := range []string{"open_id", "name", "en_name", "avatar"} {
			if v, ok := user[key]; ok {
				basic[key] = v
			}
		}
		data = map[string]any{"user": basic}
	}
	truncated := false
	clean := boundedValue(data, 0, &truncated, []string{token, input.Values["app_secret"]})
	output := map[string]any{"provider": "feishu", "operation": operation, "data": clean, "truncated": truncated}
	if c.schemas.Output(operation, output) != nil {
		return providers.Response{}, &providers.Failure{Code: core.ErrorCodeOutcomeUnknown, Unknown: request.write}
	}
	return providers.Response{Value: output, RequestID: requestID}, nil
}

func (c *Client) tenantToken(ctx context.Context, input providers.CredentialInput) (string, error) {
	response, _, err := c.call(ctx, input, "POST", "/open-apis/auth/v3/tenant_access_token/internal", nil, "", map[string]any{"app_id": input.Values["app_id"], "app_secret": input.Values["app_secret"]}, false)
	if err != nil {
		return "", err
	}
	token := providers.Text(response, "tenant_access_token")
	if token == "" || len(token) > 16384 || strings.ContainsAny(token, "\r\n") || providers.Number(response, "expire") <= 0 {
		return "", failure(core.ErrorCodeProviderUnavailable)
	}
	// Tokens are request-scoped, not cached across users/connections/credential generations.
	return token, nil
}

func (c *Client) call(ctx context.Context, input providers.CredentialInput, method, path string, query url.Values, token string, body any, write bool) (map[string]any, string, error) {
	if ctx == nil {
		return nil, "", providers.Invalid()
	}
	origin := "https://open.feishu.cn"
	if input.Values["region"] == "lark" {
		origin = "https://open.larksuite.com"
	} else if input.Values["region"] != "feishu" {
		return nil, "", providers.Invalid()
	}
	var encoded []byte
	if body != nil {
		var err error
		encoded, err = json.Marshal(body)
		if err != nil || len(encoded) > 65536 {
			return nil, "", providers.Invalid()
		}
		defer clear(encoded)
	}
	endpoint := origin + path
	if len(query) > 0 {
		endpoint += "?" + query.Encode()
	}
	bounded, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(bounded, method, endpoint, bytes.NewReader(encoded))
	if err != nil {
		return nil, "", providers.Invalid()
	}
	req.Header.Set("Content-Type", "application/json; charset=utf-8")
	req.Header.Set("Accept", "application/json")
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	res, err := c.http.Do(req)
	if err != nil {
		return nil, "", &providers.Failure{Code: core.ErrorCodeProviderUnavailable, Unknown: write}
	}
	return c.decodeResponse(res, input, token, write)
}

func (c *Client) decodeResponse(res *http.Response, input providers.CredentialInput, token string, write bool) (map[string]any, string, error) {
	defer res.Body.Close()
	requestID := providers.SafeRequestID(providers.SafeRequestID(res.Header.Get("X-Tt-Logid"), token), input.Values["app_secret"])
	raw, err := io.ReadAll(io.LimitReader(res.Body, maxResponseBytes+1))
	defer clear(raw)
	if res.StatusCode == 429 {
		return nil, requestID, &providers.Failure{Code: core.ErrorCodeRateLimited, Retryable: true, RequestID: requestID}
	}
	if res.StatusCode >= 500 {
		return nil, requestID, &providers.Failure{Code: core.ErrorCodeProviderUnavailable, Unknown: write, RequestID: requestID}
	}
	var response map[string]any
	if err != nil || len(raw) > maxResponseBytes || json.Unmarshal(raw, &response) != nil {
		return nil, requestID, &providers.Failure{Code: core.ErrorCodeProviderUnavailable, Unknown: write, RequestID: requestID}
	}
	if code, ok := response["code"].(float64); ok && code != 0 {
		return nil, requestID, apiFailure(int(code), requestID, write)
	}
	if res.StatusCode == 403 {
		return nil, requestID, &providers.Failure{Code: core.ErrorCodeResourceAccessDenied, RequestID: requestID}
	}
	if res.StatusCode == 401 {
		code := core.ErrorCodeConnectionNotReady
		if token == "" {
			code = core.ErrorCodeInvalidAppCredentials
		}
		return nil, requestID, &providers.Failure{Code: code, RequestID: requestID}
	}
	if res.StatusCode < 200 || res.StatusCode >= 300 || response["code"] != float64(0) {
		return nil, requestID, &providers.Failure{Code: core.ErrorCodeProviderUnavailable, Unknown: write, RequestID: requestID}
	}
	return response, requestID, nil
}

func failure(code core.ErrorCode) error { return &providers.Failure{Code: code} }
func apiFailure(code int, requestID string, write bool) error {
	f := &providers.Failure{Code: core.ErrorCodeProviderUnavailable, RequestID: requestID}
	// Documented global error identities; never classify arbitrary text or all 403s as a bad secret.
	switch code {
	case 10015, 20002, 99991543:
		f.Code = core.ErrorCodeInvalidAppCredentials
	case 10014, 99991673, 212100:
		f.Code = core.ErrorCodeApplicationNotReady
	case 99991672, 99991676:
		f.Code = core.ErrorCodePermissionMissing
	case 99991679, 99991401:
		f.Code = core.ErrorCodeResourceAccessDenied
	case 99991663, 99991664, 99991668:
		f.Code = core.ErrorCodeConnectionNotReady
	case 99991400:
		f.Code = core.ErrorCodeRateLimited
		f.Retryable = true
	case 10003, 99992402:
		f.Code = core.ErrorCodeInvalidInput
	default:
		// An unrecognized provider failure cannot prove a consequential
		// operation was rejected before taking effect.
		f.Unknown = write
	}
	return f
}

func boundedValue(value any, depth int, truncated *bool, secrets []string) any {
	if depth > 12 {
		*truncated = true
		return nil
	}
	switch v := value.(type) {
	case string:
		if len([]rune(v)) > 16000 {
			*truncated = true
			v = providers.SafeText(v, 16000)
		}
		for _, secret := range secrets {
			if secret != "" {
				v = strings.ReplaceAll(v, secret, "[redacted]")
			}
		}
		return v
	case []any:
		if len(v) > 100 {
			*truncated = true
			v = v[:100]
		}
		out := make([]any, len(v))
		for i, item := range v {
			out[i] = boundedValue(item, depth+1, truncated, secrets)
		}
		return out
	case map[string]any:
		out := map[string]any{}
		keys := make([]string, 0, len(v))
		for key := range v {
			keys = append(keys, key)
		}
		sort.Strings(keys)
		for _, key := range keys {
			lower := strings.ToLower(key)
			if strings.Contains(lower, "access_token") || strings.Contains(lower, "refresh_token") || strings.Contains(lower, "secret") || lower == "authorization" || lower == "device_code" {
				continue
			}
			secretKey := false
			for _, secret := range secrets {
				secretKey = secretKey || (secret != "" && strings.Contains(key, secret))
			}
			if secretKey {
				continue
			}
			if len(out) >= 100 {
				*truncated = true
				break
			}
			out[providers.SafeText(key, 128)] = boundedValue(v[key], depth+1, truncated, secrets)
		}
		return out
	default:
		return value
	}
}
