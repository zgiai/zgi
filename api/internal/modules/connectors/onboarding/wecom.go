package onboarding

import (
	"context"
	"github.com/zgiai/zgi/api/internal/modules/connectors/core"
	"net/http"
	"net/url"
	"runtime"
	"time"
)

type WeComAdapter struct{ protocolHTTP }

func NewWeComAdapter(client *http.Client, a ProtocolSettings) (*WeComAdapter, error) {
	h, err := newProtocolHTTP(client, a, true)
	if err != nil {
		return nil, err
	}
	return &WeComAdapter{h}, nil
}
func (*WeComAdapter) ConnectorID() string { return "wecom" }
func (a *WeComAdapter) Advance(ctx context.Context, f Flow, s ProtocolSecret) (ProtocolResult, error) {
	var response struct {
		ErrCode *int `json:"errcode"`
		Data    *struct {
			Scode   string  `json:"scode"`
			URL     string  `json:"auth_url"`
			Status  *string `json:"status"`
			BotInfo *struct {
				ID     string `json:"botid"`
				Secret string `json:"secret"`
			} `json:"bot_info"`
		} `json:"data"`
	}
	endpoint := "https://work.weixin.qq.com/ai/qc/"
	switch f.Step {
	case "begin":
		plat := map[string]string{"darwin": "1", "windows": "2", "linux": "3"}[runtime.GOOS]
		if plat == "" {
			return ProtocolResult{}, registrationError(core.ErrorCodeConnectorUnavailable)
		}
		endpoint += "generate?" + url.Values{"source": {a.settings.Source}, "plat": {plat}}.Encode()
	case "poll":
		endpoint += "query_result?" + url.Values{"scode": {s.DeviceCode}}.Encode()
	default:
		return ProtocolResult{}, registrationError(core.ErrorCodeInvalidInput)
	}
	err := a.json(ctx, "GET", endpoint, nil, &response, f.Step == "begin")
	if err != nil {
		if f.Step == "poll" && core.ErrorCodeOf(err) == core.ErrorCodeProviderUnavailable {
			return pendingResult(f, s), nil
		}
		return ProtocolResult{}, err
	}
	if response.ErrCode != nil && *response.ErrCode != 0 || response.Data == nil {
		return ProtocolResult{}, registrationError(core.ErrorCodeProviderUnavailable)
	}
	d := response.Data
	if f.Step == "begin" {
		if !protocolText(d.Scode, 8192) || !a.qr(d.URL) {
			return ProtocolResult{}, registrationError(core.ErrorCodeOutcomeUnknown)
		}
		return ProtocolResult{Step: "poll", Secret: ProtocolSecret{DeviceCode: d.Scode, QRPayload: d.URL}, Interval: 3 * time.Second, TTL: 5 * time.Minute, ExpirySource: "local_limit"}, nil
	}
	if d.Status == nil || !protocolText(*d.Status, 64) {
		return ProtocolResult{}, registrationError(core.ErrorCodeProviderUnavailable)
	}
	if *d.Status == "success" {
		if d.BotInfo == nil || !protocolText(d.BotInfo.ID, 128) || !protocolText(d.BotInfo.Secret, 16384) {
			return ProtocolResult{}, registrationError(core.ErrorCodeInvalidAppCredentials)
		}
		return ProtocolResult{Values: map[string]string{"bot_id": d.BotInfo.ID, "secret": d.BotInfo.Secret}}, nil
	}
	switch *d.Status {
	case "expired":
		return ProtocolResult{}, registrationError(core.ErrorCodeFlowExpired)
	case "failed", "fail", "cancelled", "denied":
		return ProtocolResult{}, registrationError(core.ErrorCodeAccessDenied)
	}
	// The official CLI waits for success instead of defining an exhaustive
	// pending-state enum. Preserve that bounded wait (observed initial state:
	// init) without treating a new waiting state as credentials or scan proof.
	return pendingResult(f, s), nil
}
