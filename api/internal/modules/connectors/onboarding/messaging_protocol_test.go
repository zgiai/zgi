package onboarding

import (
	"context"
	"encoding/json"
	"github.com/zgiai/zgi/api/internal/modules/connectors/core"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"
)

type protocolTransport func(*http.Request) (*http.Response, error)

func (f protocolTransport) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }
func protocolResponse(body string) *http.Response {
	return &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader(body)), Header: http.Header{}}
}

func TestDingTalkStepsAndSecretIsolation(t *testing.T) {
	calls := 0
	client := &http.Client{Transport: protocolTransport(func(r *http.Request) (*http.Response, error) {
		calls++
		var body map[string]string
		if json.NewDecoder(r.Body).Decode(&body) != nil {
			t.Fatal("body")
		}
		if r.Method != "POST" || r.URL.Host != "oapi.dingtalk.com" {
			t.Fatal("endpoint")
		}
		switch calls {
		case 1:
			if body["source"] != "admitted_test_source" || len(body) != 1 {
				t.Fatal("source")
			}
			return protocolResponse(`{"errcode":0,"nonce":"PRIVATE_NONCE","expires_in":90}`), nil
		case 2:
			if body["nonce"] != "PRIVATE_NONCE" || len(body) != 1 {
				t.Fatal("nonce")
			}
			return protocolResponse(`{"errcode":0,"device_code":"PRIVATE_DEVICE","verification_uri_complete":"https://login.dingtalk.com/qr","interval":3,"expires_in":120}`), nil
		default:
			if body["device_code"] != "PRIVATE_DEVICE" {
				t.Fatal("device")
			}
			return protocolResponse(`{"errcode":0,"status":"SUCCESS","client_id":"app1","client_secret":"PRIVATE_SECRET"}`), nil
		}
	})}
	a, err := NewDingTalkAdapter(client, ProtocolSettings{Source: "admitted_test_source", QRHosts: []string{"login.dingtalk.com"}})
	if err != nil {
		t.Fatal(err)
	}
	f := Flow{Step: "init"}
	r, err := a.Advance(context.Background(), f, ProtocolSecret{})
	if err != nil || r.Step != "begin" || calls != 1 {
		t.Fatal(r, err)
	}
	f.Step = r.Step
	r, err = a.Advance(context.Background(), f, r.Secret)
	if err != nil || r.Interval != 3*time.Second || r.Secret.Nonce != "" {
		t.Fatal(r, err)
	}
	f.Step = r.Step
	r, err = a.Advance(context.Background(), f, r.Secret)
	if err != nil || r.Values["client_secret"] != "PRIVATE_SECRET" {
		t.Fatal(err)
	}
}

func TestDingTalkRegistrationKeepsWaitingAfterTheScan(t *testing.T) {
	for _, status := range []string{"WAITING", "SCANNED", "SOMETHING_NEW"} {
		a, err := NewDingTalkAdapter(&http.Client{Transport: protocolTransport(func(*http.Request) (*http.Response, error) {
			return protocolResponse(`{"errcode":0,"errmsg":"ok","status":"` + status + `"}`), nil
		})}, ProtocolSettings{Source: "s", QRHosts: []string{"login.dingtalk.com"}})
		if err != nil {
			t.Fatal(err)
		}
		flow := Flow{Step: "poll"}
		result, err := a.Advance(context.Background(), flow, ProtocolSecret{DeviceCode: "device"})
		if err != nil || result.Values != nil || result.Step != "poll" {
			t.Fatalf("%s: %#v, %v; want to keep waiting", status, result, err)
		}
	}
}

func TestDingTalkRegistrationRefusalsSayWhy(t *testing.T) {
	for name, tc := range map[string]struct {
		body string
		want core.ErrorCode
	}{
		"no developer permission": {`{"errcode":40001,"errmsg":"用户没有开发者权限，请联系主管理员开通"}`, core.ErrorCodeAccessDenied},
		"refused by status":       {`{"errcode":0,"status":"FAIL"}`, core.ErrorCodeAccessDenied},
		"other refusal":           {`{"errcode":50001,"errmsg":"system busy"}`, core.ErrorCodeProviderUnavailable},
	} {
		a, err := NewDingTalkAdapter(&http.Client{Transport: protocolTransport(func(*http.Request) (*http.Response, error) {
			return protocolResponse(tc.body), nil
		})}, ProtocolSettings{Source: "s", QRHosts: []string{"login.dingtalk.com"}})
		if err != nil {
			t.Fatal(err)
		}
		_, err = a.Advance(context.Background(), Flow{Step: "poll"}, ProtocolSecret{DeviceCode: "device"})
		if core.ErrorCodeOf(err) != tc.want {
			t.Fatalf("%s: %v, want %s", name, err, tc.want)
		}
	}
}

func TestWeixinEightStatesAndOriginFence(t *testing.T) {
	for _, status := range []string{"wait", "scaned", "need_verifycode", "expired", "verify_code_blocked", "binded_redirect", "confirmed", "scaned_but_redirect", "unknown"} {
		t.Run(status, func(t *testing.T) {
			a, _ := NewWeixinAdapter(&http.Client{Transport: protocolTransport(func(r *http.Request) (*http.Response, error) {
				if r.Method != "GET" || r.URL.Query().Get("verify_code") != "123" {
					t.Fatal("challenge")
				}
				return protocolResponse(`{"status":"` + status + `","bot_token":"private-token","ilink_bot_id":"bot1","baseurl":"https://ilinkai.weixin.qq.com","redirect_host":"127.0.0.1"}`), nil
			})}, ProtocolSettings{AppID: "fixture_app", QRHosts: []string{"liteapp.weixin.qq.com"}})
			r, err := a.Advance(context.Background(), Flow{Step: "poll", PollIntervalSeconds: 1}, ProtocolSecret{Origin: weixinOrigin, DeviceCode: "private-code", VerifyCode: "123"})
			switch status {
			case "wait", "scaned":
				if err != nil || r.Step != "poll" || r.Secret.VerifyCode != "" {
					t.Fatal(err)
				}
			case "need_verifycode":
				if err != nil || r.Step != "awaiting_code" {
					t.Fatal(err)
				}
			case "confirmed":
				if err != nil || len(r.Values) != 3 {
					t.Fatal(err)
				}
			default:
				if err == nil {
					t.Fatal("unsafe terminal accepted")
				}
			}
		})
	}
}
func TestWeComBoundedPendingAndPrivateScode(t *testing.T) {
	status := "init"
	a, _ := NewWeComAdapter(&http.Client{Transport: protocolTransport(func(r *http.Request) (*http.Response, error) {
		if r.URL.Query().Get("scode") != "secret-code" {
			t.Fatal("scode")
		}
		return protocolResponse(`{"data":{"status":"` + status + `"}}`), nil
	})}, DefaultMessagingProtocol("wecom"))
	f := Flow{Step: "poll", PollIntervalSeconds: 3}
	s := ProtocolSecret{DeviceCode: "secret-code"}
	if r, err := a.Advance(context.Background(), f, s); err != nil || r.Interval != 3*time.Second {
		t.Fatal(err)
	}
	status = "new_waiting_state"
	if r, err := a.Advance(context.Background(), f, s); err != nil || len(r.Values) != 0 || r.Step != "poll" {
		t.Fatal("waiting state treated as credentials or prematurely terminated")
	}
	status = "success"
	if _, err := a.Advance(context.Background(), f, s); core.ErrorCodeOf(err) != core.ErrorCodeInvalidAppCredentials {
		t.Fatal("success without credentials accepted")
	}
	status = "expired"
	if _, err := a.Advance(context.Background(), f, s); core.ErrorCodeOf(err) != core.ErrorCodeFlowExpired {
		t.Fatal("expiry not terminated")
	}
}
func TestMessagingProtocolSettingsAndUnsafeQR(t *testing.T) {
	if _, err := NewDingTalkAdapter(nil, ProtocolSettings{}); err == nil {
		t.Fatal("missing wire parameters accepted")
	}
	h, err := newProtocolHTTP(nil, ProtocolSettings{Source: "fixture", QRHosts: []string{"login.dingtalk.com"}}, true)
	if err != nil {
		t.Fatal(err)
	}
	for _, raw := range []string{"https://login.dingtalk.com.evil.example/x", "https://login.dingtalk.com:8443/x", "https://user@login.dingtalk.com/x", "http://login.dingtalk.com/x", "https://127.0.0.1/x"} {
		if h.qr(raw) {
			t.Fatal("unsafe qr")
		}
	}
}

func TestDefaultMessagingProtocolsCanGenerateQRWithoutEnvironment(t *testing.T) {
	for _, id := range []string{"dingtalk", "wecom", "weixin"} {
		t.Run(id, func(t *testing.T) {
			settings := DefaultMessagingProtocol(id)
			client := &http.Client{Transport: protocolTransport(func(r *http.Request) (*http.Response, error) {
				switch id {
				case "dingtalk":
					if r.URL.Path == "/app/registration/init" {
						var body map[string]string
						if json.NewDecoder(r.Body).Decode(&body) != nil || body["source"] != "DING_DWS_CLAW" {
							t.Fatal("default source")
						}
						return protocolResponse(`{"errcode":0,"nonce":"fixture-nonce"}`), nil
					}
					return protocolResponse(`{"errcode":0,"device_code":"fixture-device","verification_uri_complete":"https://open-dev.dingtalk.com/fixture","expires_in":7200,"interval":2}`), nil
				case "wecom":
					if r.URL.Query().Get("source") != "wecom_cli_external" {
						t.Fatal("default source")
					}
					return protocolResponse(`{"data":{"scode":"fixture-code","auth_url":"https://work.weixin.qq.com/fixture"}}`), nil
				default:
					if r.Header.Get("Ilink-App-Id") != "bot" || r.Method != "POST" {
						t.Fatal("default client ID")
					}
					return protocolResponse(`{"ret":0,"qrcode":"fixture-code","qrcode_img_content":"https://liteapp.weixin.qq.com/fixture"}`), nil
				}
			})}
			var adapter ProtocolAdapter
			var err error
			step := "begin"
			switch id {
			case "dingtalk":
				adapter, err = NewDingTalkAdapter(client, settings)
				step = "init"
			case "wecom":
				adapter, err = NewWeComAdapter(client, settings)
			case "weixin":
				adapter, err = NewWeixinAdapter(client, settings)
			}
			if err != nil {
				t.Fatal(err)
			}
			result, err := adapter.Advance(context.Background(), Flow{Step: step}, ProtocolSecret{})
			if err == nil && result.Step == "begin" {
				result, err = adapter.Advance(context.Background(), Flow{Step: "begin"}, result.Secret)
			}
			if err != nil || result.Step != "poll" || result.Secret.QRPayload == "" || len(result.Values) != 0 {
				t.Fatal("QR setup", err)
			}
		})
	}
}
