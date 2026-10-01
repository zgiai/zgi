package onboarding

import (
	"bytes"
	"context"
	"encoding/json"
	"github.com/zgiai/zgi/api/internal/modules/connectors/core"
	"github.com/zgiai/zgi/api/internal/modules/connectors/providers/ilink"
	"io"
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"time"
)

// ProtocolSettings contains compiled wire parameters. They do not act as a
// deployment enable switch and cannot be supplied by the browser.
type ProtocolSettings struct {
	AppID   string
	Source  string
	QRHosts []string
}
type protocolHTTP struct {
	client   *http.Client
	settings ProtocolSettings
}

var sourcePattern = regexp.MustCompile(`^[A-Za-z0-9_-]{1,80}$`)

func newProtocolHTTP(client *http.Client, a ProtocolSettings, sourceRequired bool) (protocolHTTP, error) {
	if len(a.QRHosts) == 0 || sourceRequired && !sourcePattern.MatchString(a.Source) {
		return protocolHTTP{}, registrationError(core.ErrorCodeConnectorUnavailable)
	}
	for _, host := range a.QRHosts {
		if host == "" || strings.ContainsAny(host, "/:@?#\\ \r\n") || !(strings.HasSuffix(host, ".dingtalk.com") || strings.HasSuffix(host, ".weixin.qq.com")) {
			return protocolHTTP{}, registrationError(core.ErrorCodeInvalidInput)
		}
	}
	c := http.Client{Timeout: 35 * time.Second}
	if client != nil {
		c = *client
		if c.Timeout <= 0 || c.Timeout > 35*time.Second {
			c.Timeout = 35 * time.Second
		}
	}
	c.CheckRedirect = func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }
	a.QRHosts = append([]string{}, a.QRHosts...)
	return protocolHTTP{&c, a}, nil
}
func (h protocolHTTP) qr(raw string) bool {
	u, err := url.Parse(raw)
	if err != nil || len(raw) > 8192 || u.Scheme != "https" || u.User != nil || u.Port() != "" || u.Fragment != "" {
		return false
	}
	for _, host := range h.settings.QRHosts {
		if u.Host == host {
			return true
		}
	}
	return false
}
func (h protocolHTTP) json(ctx context.Context, method, endpoint string, body any, out any, write bool) error {
	var data []byte
	var err error
	if body != nil {
		data, err = json.Marshal(body)
		if err != nil {
			return registrationError(core.ErrorCodeInvalidInput)
		}
		defer clear(data)
	}
	req, err := http.NewRequestWithContext(ctx, method, endpoint, bytes.NewReader(data))
	if err != nil {
		return registrationError(core.ErrorCodeInvalidInput)
	}
	req.Header.Set("Content-Type", "application/json")
	if req.URL.Host == "ilinkai.weixin.qq.com" {
		for key, values := range ilink.Headers(h.settings.AppID, "", method == "POST") {
			req.Header[key] = values
		}
	}
	req.Header.Set("Accept", "application/json")
	res, err := h.client.Do(req)
	if err != nil {
		if write {
			return registrationError(core.ErrorCodeOutcomeUnknown)
		}
		return registrationError(core.ErrorCodeProviderUnavailable)
	}
	defer res.Body.Close()
	if res.StatusCode < 200 || res.StatusCode >= 300 {
		if write && res.StatusCode >= 500 {
			return registrationError(core.ErrorCodeOutcomeUnknown)
		}
		return registrationError(core.ErrorCodeProviderUnavailable)
	}
	raw, err := io.ReadAll(io.LimitReader(res.Body, 65537))
	defer clear(raw)
	if err != nil || len(raw) > 65536 || json.Unmarshal(raw, out) != nil {
		if write {
			return registrationError(core.ErrorCodeOutcomeUnknown)
		}
		return registrationError(core.ErrorCodeProviderUnavailable)
	}
	return nil
}
func protocolText(s string, max int) bool {
	return s != "" && len(s) <= max && strings.TrimSpace(s) == s && !strings.ContainsAny(s, "\r\n")
}
func pendingResult(f Flow, s ProtocolSecret) ProtocolResult {
	return ProtocolResult{Step: "poll", Secret: s, Interval: time.Duration(max(1, f.PollIntervalSeconds)) * time.Second}
}
