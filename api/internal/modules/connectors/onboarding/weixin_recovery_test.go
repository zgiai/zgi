package onboarding

import (
	"context"
	"net/http"
	"testing"

	"github.com/zgiai/zgi/api/internal/modules/connectors/core"
)

func TestWeixinDefaultOriginAndBoundWithoutCredentials(t *testing.T) {
	for _, tc := range []struct {
		body string
		want core.ErrorCode
	}{
		{`{"status":"confirmed","bot_token":"fixture","ilink_bot_id":"bot"}`, ""},
		{`{"status":"confirmed","bot_token":"fixture","ilink_bot_id":"bot","baseurl":"https://untrusted.example"}`, core.ErrorCodeInvalidAppCredentials},
		{`{"status":"binded_redirect","ilink_bot_id":"bot"}`, core.ErrorCodeWeixinAlreadyBound},
		{`{"status":"scaned_but_redirect","redirect_host":"regional.weixin.qq.com"}`, core.ErrorCodeWeixinRegionUnsupported},
	} {
		a, err := NewWeixinAdapter(&http.Client{Transport: protocolTransport(func(*http.Request) (*http.Response, error) { return protocolResponse(tc.body), nil })}, ProtocolSettings{AppID: "fixture", QRHosts: []string{"liteapp.weixin.qq.com"}})
		if err != nil {
			t.Fatal(err)
		}
		result, err := a.Advance(context.Background(), Flow{Step: "poll"}, ProtocolSecret{Origin: weixinOrigin, DeviceCode: "fixture"})
		if core.ErrorCodeOf(err) != tc.want {
			t.Fatalf("%v", err)
		}
		if err != nil && len(result.Values) != 0 {
			t.Fatal("invented credentials")
		}
		if err == nil && result.Values["base_url"] != weixinOrigin {
			t.Fatal("default origin")
		}
	}
}
