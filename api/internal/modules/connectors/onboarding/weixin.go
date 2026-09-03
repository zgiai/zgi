package onboarding

import (
	"context"
	"github.com/zgiai/zgi/api/internal/modules/connectors/core"
	"net/http"
	"net/url"
	"time"
)

const weixinOrigin = "https://ilinkai.weixin.qq.com"

type WeixinAdapter struct{ protocolHTTP }

func NewWeixinAdapter(client *http.Client, a ProtocolSettings) (*WeixinAdapter, error) {
	if !sourcePattern.MatchString(a.AppID) {
		return nil, registrationError(core.ErrorCodeConnectorUnavailable)
	}
	h, err := newProtocolHTTP(client, a, false)
	if err != nil {
		return nil, err
	}
	return &WeixinAdapter{h}, nil
}
func (*WeixinAdapter) ConnectorID() string { return "weixin" }
func (a *WeixinAdapter) Advance(ctx context.Context, f Flow, s ProtocolSecret) (ProtocolResult, error) {
	var response struct {
		QRCode       string `json:"qrcode"`
		QRPayload    string `json:"qrcode_img_content"`
		Status       string `json:"status"`
		Token        string `json:"bot_token"`
		BotID        string `json:"ilink_bot_id"`
		BaseURL      string `json:"baseurl"`
		RedirectHost string `json:"redirect_host"`
	}
	if f.Step == "begin" {
		err := a.json(ctx, "POST", weixinOrigin+"/ilink/bot/get_bot_qrcode?bot_type=3", map[string]any{"local_token_list": []string{}}, &response, true)
		if err != nil {
			return ProtocolResult{}, err
		}
		if !protocolText(response.QRCode, 8192) || !a.qr(response.QRPayload) {
			return ProtocolResult{}, registrationError(core.ErrorCodeOutcomeUnknown)
		}
		return ProtocolResult{Step: "poll", Secret: ProtocolSecret{DeviceCode: response.QRCode, QRPayload: response.QRPayload, Origin: weixinOrigin}, Interval: time.Second, TTL: 5 * time.Minute, ExpirySource: "local_limit"}, nil
	}
	if f.Step != "poll" || s.Origin != weixinOrigin {
		return ProtocolResult{}, registrationError(core.ErrorCodeInvalidInput)
	}
	query := url.Values{"qrcode": {s.DeviceCode}}
	if s.VerifyCode != "" {
		query.Set("verify_code", s.VerifyCode)
	}
	err := a.json(ctx, "GET", s.Origin+"/ilink/bot/get_qrcode_status?"+query.Encode(), nil, &response, false)
	// A read timeout is only a pending login, never verification evidence. A
	// submitted challenge with an unknown response is not sent a second time.
	if err != nil {
		if s.VerifyCode == "" && core.ErrorCodeOf(err) == core.ErrorCodeProviderUnavailable {
			return pendingResult(f, s), nil
		}
		return ProtocolResult{}, registrationError(core.ErrorCodeOutcomeUnknown)
	}
	s.VerifyCode = ""
	switch response.Status {
	case "wait", "scaned":
		return pendingResult(f, s), nil
	case "need_verifycode":
		if f.ChallengeAttempts >= 3 {
			return ProtocolResult{}, registrationError(core.ErrorCodeAccessDenied)
		}
		r := pendingResult(f, s)
		r.Step = "awaiting_code"
		return r, nil
	case "verify_code_blocked":
		return ProtocolResult{}, registrationError(core.ErrorCodeAccessDenied)
	case "expired":
		return ProtocolResult{}, registrationError(core.ErrorCodeFlowExpired)
	case "binded_redirect":
		return ProtocolResult{}, registrationError(core.ErrorCodeConflict)
	case "scaned_but_redirect":
		// Additional IDC hosts require a separately verified credential-origin
		// contract. Never forward a session to a suffix-matched or supplied URL.
		if response.RedirectHost != "ilinkai.weixin.qq.com" {
			return ProtocolResult{}, registrationError(core.ErrorCodeConnectorUnavailable)
		}
		return pendingResult(f, s), nil
	case "confirmed":
		if !protocolText(response.Token, 16384) || !protocolText(response.BotID, 128) || (response.BaseURL != weixinOrigin && response.BaseURL != weixinOrigin+"/") {
			return ProtocolResult{}, registrationError(core.ErrorCodeInvalidAppCredentials)
		}
		return ProtocolResult{Values: map[string]string{"account_id": response.BotID, "token": response.Token, "base_url": weixinOrigin}}, nil
	}
	return ProtocolResult{}, registrationError(core.ErrorCodeProviderUnavailable)
}
