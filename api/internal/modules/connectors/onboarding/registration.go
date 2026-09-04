// Package onboarding implements short-lived Connector credential registration.
// It never receives business events or invokes a ZGI agent.
package onboarding

import (
	"bytes"
	"compress/gzip"
	"context"
	"encoding/base64"
	"encoding/json"
	"io"
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"time"

	"github.com/zgiai/zgi/api/internal/modules/connectors/catalog"
	"github.com/zgiai/zgi/api/internal/modules/connectors/core"
)

// The wire protocol is pinned to the official Go SDK v3.11.0 scene/registration.
// Calls are separated so a durable worker can fence, cancel and rate-limit each
// request. No global HTTP client, SDK logging or in-memory polling loop is used.
type RegistrationClient struct {
	http         *http.Client
	addons       string
	resumeAddons string
}
type RegistrationPurpose string

const (
	CreateApplication RegistrationPurpose = "create_app"
	UpdateApplication RegistrationPurpose = "update_app"
	ResumeApplication RegistrationPurpose = "resume_app"
)

type RegistrationRequest struct {
	Purpose RegistrationPurpose
	Region  string
	AppID   string
}

// Same-app resume is enabled for the two supported regions. Admission still
// fixes the app identity and never falls back to application creation. Enabling
// this path does not certify the provider's configuration/Secret side effects.
func resumeQREnabled(region string) bool { return region == "feishu" || region == "lark" }

type RegistrationSession struct {
	DeviceCode      string        `json:"-"`
	VerificationURL string        `json:"-"`
	UserCode        string        `json:"-"`
	Interval        time.Duration `json:"-"`
	TTL             time.Duration `json:"-"`
}
type RegistrationResult struct {
	Status    string
	AppID     string `json:"-"`
	AppSecret string `json:"-"`
}
type Registrar interface {
	Begin(context.Context, RegistrationRequest) (RegistrationSession, error)
	Poll(context.Context, string, string, bool) (RegistrationResult, error)
}

var appIdentifier = regexp.MustCompile(`^[A-Za-z0-9_-]{1,128}$`)

func NewRegistrationClient(client *http.Client) (*RegistrationClient, error) {
	profile := catalog.FeishuPermissionTemplate()
	body, err := json.Marshal(profile["addons"])
	if err != nil {
		return nil, err
	}
	var compressed bytes.Buffer
	writer := gzip.NewWriter(&compressed)
	if _, err = writer.Write(body); err != nil {
		return nil, err
	}
	if err = writer.Close(); err != nil {
		return nil, err
	}
	h := http.Client{Timeout: 10 * time.Second}
	if client != nil {
		h = *client
	}
	if h.Timeout <= 0 || h.Timeout > 10*time.Second {
		h.Timeout = 10 * time.Second
	}
	h.CheckRedirect = func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }
	var resume bytes.Buffer
	zw := gzip.NewWriter(&resume)
	if _, err = zw.Write([]byte(`{"preset":false}`)); err != nil {
		return nil, err
	}
	if err = zw.Close(); err != nil {
		return nil, err
	}
	return &RegistrationClient{http: &h, addons: base64.RawURLEncoding.EncodeToString(compressed.Bytes()), resumeAddons: base64.RawURLEncoding.EncodeToString(resume.Bytes())}, nil
}
func registrationOrigin(region string) string {
	switch region {
	case "feishu":
		return "https://accounts.feishu.cn"
	case "lark":
		return "https://accounts.larksuite.com"
	}
	return ""
}
func (c *RegistrationClient) Begin(ctx context.Context, request RegistrationRequest) (RegistrationSession, error) {
	region, appID := request.Region, request.AppID
	if registrationOrigin(region) == "" ||
		(request.Purpose == CreateApplication && appID != "") ||
		(request.Purpose != CreateApplication && (request.Purpose != UpdateApplication && request.Purpose != ResumeApplication || !appIdentifier.MatchString(appID))) {
		return RegistrationSession{}, registrationError(core.ErrorCodeInvalidInput)
	}
	var response struct {
		DeviceCode string `json:"device_code"`
		URL        string `json:"verification_uri_complete"`
		UserCode   string `json:"user_code"`
		Interval   int    `json:"interval"`
		TTL        *int   `json:"expire_in"`
		ExpiresIn  *int   `json:"expires_in"`
	}
	if err := c.request(ctx, region, url.Values{"action": {"begin"}, "archetype": {"PersonalAgent"}, "auth_method": {"client_secret"}, "request_user_info": {"open_id"}}, &response); err != nil {
		return RegistrationSession{}, err
	}
	if response.DeviceCode == "" || len(response.DeviceCode) > 8192 || strings.ContainsAny(response.DeviceCode, "\r\n") || len(response.UserCode) > 64 || strings.ContainsAny(response.UserCode, "\r\n") || strings.Contains(response.URL, response.DeviceCode) {
		return RegistrationSession{}, registrationError(core.ErrorCodeOutcomeUnknown)
	}
	u, err := url.Parse(response.URL)
	if err != nil || !allowedVerificationURL(u) {
		return RegistrationSession{}, registrationError(core.ErrorCodeOutcomeUnknown)
	}
	q := u.Query()
	q.Set("from", "sdk")
	q.Set("tp", "sdk")
	q.Set("source", "go-sdk/zgi")
	q.Set("addons", c.addons)
	if request.Purpose == ResumeApplication {
		q.Set("addons", c.resumeAddons)
		q.Del("name")
		q.Del("avatar")
		q.Del("description")
	}
	q.Del("createOnly")
	q.Del("clientID")
	if appID == "" {
		q.Set("createOnly", "true")
	} else {
		q.Set("clientID", appID)
	}
	u.RawQuery = q.Encode()
	interval := response.Interval
	if interval <= 0 {
		interval = 5
	}
	if interval > 300 {
		// Never poll faster than the provider instructed. Unsupported intervals
		// require recovery instead of silently shortening the interval.
		return RegistrationSession{}, registrationError(core.ErrorCodeOutcomeUnknown)
	}
	ttl := 600
	for _, value := range []*int{response.TTL, response.ExpiresIn} {
		if value != nil {
			if *value <= 0 {
				return RegistrationSession{}, registrationError(core.ErrorCodeOutcomeUnknown)
			}
			ttl = min(ttl, *value)
		}
	}
	return RegistrationSession{DeviceCode: response.DeviceCode, VerificationURL: u.String(), UserCode: response.UserCode, Interval: time.Duration(interval) * time.Second, TTL: time.Duration(ttl) * time.Second}, nil
}
func allowedVerificationURL(u *url.URL) bool {
	if u == nil || u.Scheme != "https" || u.User != nil || u.Fragment != "" || (u.Port() != "" && u.Port() != "443") || len(u.String()) > 32768 {
		return false
	}
	switch u.Hostname() {
	case "accounts.feishu.cn", "accounts.larksuite.com", "open.feishu.cn", "open.larksuite.com":
		return true
	}
	return false
}
func (c *RegistrationClient) Poll(ctx context.Context, region, deviceCode string, switched bool) (RegistrationResult, error) {
	if registrationOrigin(region) == "" || deviceCode == "" || len(deviceCode) > 8192 || strings.ContainsAny(deviceCode, "\r\n") {
		return RegistrationResult{}, registrationError(core.ErrorCodeInvalidInput)
	}
	var response struct {
		AppID     string `json:"client_id"`
		AppSecret string `json:"client_secret"`
		Error     string `json:"error"`
		UserInfo  *struct {
			Brand string `json:"tenant_brand"`
		} `json:"user_info"`
	}
	if err := c.request(ctx, region, url.Values{"action": {"poll"}, "device_code": {deviceCode}}, &response); err != nil {
		return RegistrationResult{}, err
	}
	if response.AppID != "" || response.AppSecret != "" {
		if response.UserInfo != nil && response.UserInfo.Brand == "lark" && region != "lark" {
			return RegistrationResult{}, registrationError(core.ErrorCodeOutcomeUnknown)
		}
		if response.Error != "" || !appIdentifier.MatchString(response.AppID) || response.AppSecret == "" || len(response.AppSecret) > 16384 || strings.ContainsAny(response.AppSecret, "\r\n\x00") {
			return RegistrationResult{}, registrationError(core.ErrorCodeOutcomeUnknown)
		}
		return RegistrationResult{Status: "complete", AppID: response.AppID, AppSecret: response.AppSecret}, nil
	}
	if response.UserInfo != nil && response.UserInfo.Brand == "lark" && region != "lark" && !switched {
		return RegistrationResult{Status: "switch_region"}, nil
	}
	switch response.Error {
	case "", "authorization_pending":
		return RegistrationResult{Status: "pending"}, nil
	case "slow_down":
		return RegistrationResult{Status: "slow_down"}, nil
	case "access_denied":
		return RegistrationResult{}, registrationError(core.ErrorCodeAccessDenied)
	case "expired_token":
		return RegistrationResult{}, registrationError(core.ErrorCodeFlowExpired)
	}
	return RegistrationResult{}, registrationError(core.ErrorCodeOutcomeUnknown)
}
func (c *RegistrationClient) request(ctx context.Context, region string, form url.Values, result any) error {
	if c == nil || ctx == nil || registrationOrigin(region) == "" {
		return registrationError(core.ErrorCodeInvalidInput)
	}
	if ctx.Err() != nil {
		return registrationError(core.ErrorCodeConflict)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, registrationOrigin(region)+"/oauth/v1/app/registration", strings.NewReader(form.Encode()))
	if err != nil {
		return registrationError(core.ErrorCodeInvalidInput)
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("Accept", "application/json")
	resp, err := c.http.Do(req)
	if err != nil {
		return registrationError(core.ErrorCodeOutcomeUnknown)
	}
	defer resp.Body.Close()
	if resp.StatusCode == 429 {
		return registrationError(core.ErrorCodeRateLimited)
	}
	// OAuth pending/denial may use HTTP 400. All other unexpected statuses have
	// unknown outcomes and are never retried as a fresh begin.
	if resp.StatusCode != 200 && resp.StatusCode != 400 {
		return registrationError(core.ErrorCodeOutcomeUnknown)
	}
	body, err := io.ReadAll(io.LimitReader(resp.Body, 65537))
	if err != nil || len(body) > 65536 {
		return registrationError(core.ErrorCodeOutcomeUnknown)
	}
	defer func() {
		for i := range body {
			body[i] = 0
		}
	}()
	if trimmed := strings.TrimSpace(string(body)); len(trimmed) < 2 || trimmed[0] != '{' || json.Unmarshal(body, result) != nil {
		return registrationError(core.ErrorCodeOutcomeUnknown)
	}
	return nil
}
func registrationError(code core.ErrorCode) error {
	return core.NewError(code, code == core.ErrorCodeRateLimited, nil)
}
