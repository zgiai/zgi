package onboarding

import (
	"context"
	"github.com/zgiai/zgi/api/internal/modules/connectors/core"
	"net/http"
	"time"
)

type DingTalkAdapter struct{ protocolHTTP }

func NewDingTalkAdapter(client *http.Client, a ProtocolSettings) (*DingTalkAdapter, error) {
	h, err := newProtocolHTTP(client, a, true)
	if err != nil {
		return nil, err
	}
	return &DingTalkAdapter{h}, nil
}
func (*DingTalkAdapter) ConnectorID() string { return "dingtalk" }
func (a *DingTalkAdapter) Advance(ctx context.Context, f Flow, s ProtocolSecret) (ProtocolResult, error) {
	const base = "https://oapi.dingtalk.com/app/registration/"
	var response struct {
		ErrCode      *int   `json:"errcode"`
		Nonce        string `json:"nonce"`
		DeviceCode   string `json:"device_code"`
		UserCode     string `json:"user_code"`
		URL          string `json:"verification_uri_complete"`
		Expires      int    `json:"expires_in"`
		Interval     int    `json:"interval"`
		Status       string `json:"status"`
		ClientID     string `json:"client_id"`
		ClientSecret string `json:"client_secret"`
	}
	var body map[string]string
	switch f.Step {
	case "init":
		body = map[string]string{"source": a.settings.Source}
	case "begin":
		if !protocolText(s.Nonce, 8192) {
			return ProtocolResult{}, registrationError(core.ErrorCodeConflict)
		}
		body = map[string]string{"nonce": s.Nonce}
	case "poll":
		body = map[string]string{"device_code": s.DeviceCode}
	default:
		return ProtocolResult{}, registrationError(core.ErrorCodeInvalidInput)
	}
	err := a.json(ctx, "POST", base+f.Step, body, &response, f.Step != "poll")
	if err != nil {
		if f.Step == "poll" && core.ErrorCodeOf(err) == core.ErrorCodeProviderUnavailable {
			return pendingResult(f, s), nil
		}
		return ProtocolResult{}, err
	}
	if response.ErrCode == nil || *response.ErrCode != 0 {
		return ProtocolResult{}, registrationError(core.ErrorCodeProviderUnavailable)
	}
	switch f.Step {
	case "init":
		if !protocolText(response.Nonce, 8192) {
			return ProtocolResult{}, registrationError(core.ErrorCodeOutcomeUnknown)
		}
		ttl := time.Duration(response.Expires) * time.Second
		if ttl <= 0 || ttl > 2*time.Hour {
			ttl = 5 * time.Minute
		}
		return ProtocolResult{Step: "begin", Secret: ProtocolSecret{Nonce: response.Nonce}, Interval: time.Second, TTL: ttl, ExpirySource: "provider_or_local_limit"}, nil
	case "begin":
		if !protocolText(response.DeviceCode, 8192) || !a.qr(response.URL) || len(response.UserCode) > 128 {
			return ProtocolResult{}, registrationError(core.ErrorCodeOutcomeUnknown)
		}
		interval := response.Interval
		if interval <= 0 {
			interval = 3
		}
		if interval > 300 {
			return ProtocolResult{}, registrationError(core.ErrorCodeProviderUnavailable)
		}
		ttl := response.Expires
		if ttl <= 0 {
			ttl = 7200
		}
		if ttl > 7200 {
			ttl = 7200
		}
		return ProtocolResult{Step: "poll", Secret: ProtocolSecret{DeviceCode: response.DeviceCode, QRPayload: response.URL, UserCode: response.UserCode}, Interval: time.Duration(interval) * time.Second, TTL: time.Duration(ttl) * time.Second, ExpirySource: "provider_or_local_limit"}, nil
	case "poll":
		switch response.Status {
		case "WAITING":
			return pendingResult(f, s), nil
		case "EXPIRED":
			return ProtocolResult{}, registrationError(core.ErrorCodeFlowExpired)
		case "FAIL":
			return ProtocolResult{}, registrationError(core.ErrorCodeAccessDenied)
		case "SUCCESS":
			if !protocolText(response.ClientID, 128) || !protocolText(response.ClientSecret, 16384) {
				return ProtocolResult{}, registrationError(core.ErrorCodeInvalidAppCredentials)
			}
			return ProtocolResult{Values: map[string]string{"client_id": response.ClientID, "client_secret": response.ClientSecret}}, nil
		}
	}
	return ProtocolResult{}, registrationError(core.ErrorCodeProviderUnavailable)
}
