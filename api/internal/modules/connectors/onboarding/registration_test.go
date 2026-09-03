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
	"reflect"
	"slices"
	"strings"
	"testing"

	"github.com/zgiai/zgi/api/internal/modules/connectors/catalog"
	"github.com/zgiai/zgi/api/internal/modules/connectors/core"
)

type transportFunc func(*http.Request) (*http.Response, error)

func (f transportFunc) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }
func reply(body string) *http.Response {
	return &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader(body)), Header: http.Header{}}
}
func TestRegistrationCreateAndUpdateUseFixedTemplateAndNoSecretDTO(t *testing.T) {
	for _, tc := range []struct{ region, appID string }{{"feishu", ""}, {"feishu", "cli_existing"}, {"lark", ""}, {"lark", "cli_existing"}} {
		t.Run(tc.region+"/"+tc.appID, func(t *testing.T) {
			appID := tc.appID
			calls := 0
			client, err := NewRegistrationClient(&http.Client{Transport: transportFunc(func(r *http.Request) (*http.Response, error) {
				calls++
				if r.URL.String() != registrationOrigin(tc.region)+"/oauth/v1/app/registration" || r.Method != "POST" {
					t.Fatal("unexpected origin")
				}
				if err := r.ParseForm(); err != nil {
					t.Fatal(err)
				}
				if len(r.Form) != 4 || r.Form.Get("request_user_info") != "open_id" || r.Form.Get("action") != "begin" || r.Form.Get("archetype") != "PersonalAgent" || r.Form.Get("auth_method") != "client_secret" {
					t.Fatal("wrong protocol")
				}
				return reply(`{"device_code":"DEVICE-SENTINEL","verification_uri_complete":"` + registrationOrigin(tc.region) + `/app?user_code=ABCD-EFGH&addons=old&createOnly=true&clientID=cli_other","user_code":"ABCD-EFGH","interval":5,"expire_in":600}`), nil
			})})
			if err != nil {
				t.Fatal(err)
			}
			got, err := client.Begin(context.Background(), tc.region, appID)
			if err != nil {
				t.Fatal(err)
			}
			if calls != 1 || got.DeviceCode != "DEVICE-SENTINEL" {
				t.Fatal("wrong begin result")
			}
			u, _ := url.Parse(got.VerificationURL)
			q := u.Query()
			if appID == "" && (q.Get("createOnly") != "true" || q.Get("clientID") != "") {
				t.Fatal("create can select existing app")
			}
			if appID != "" && (q.Get("clientID") != appID || q.Get("createOnly") != "") {
				t.Fatal("update not pinned")
			}
			compressed, err := base64.RawURLEncoding.DecodeString(q.Get("addons"))
			if err != nil {
				t.Fatal(err)
			}
			reader, err := gzip.NewReader(bytes.NewReader(compressed))
			if err != nil {
				t.Fatal(err)
			}
			defer reader.Close()
			var addons struct {
				Preset bool
				Scopes struct{ Tenant, User []string }
				Events struct {
					Items struct{ Tenant, User []string }
				}
				Callbacks struct{ Items []string }
			}
			body, err := io.ReadAll(reader)
			if err != nil {
				t.Fatal(err)
			}
			var payload map[string]any
			if err = json.Unmarshal(body, &payload); err != nil || !reflect.DeepEqual(payload, catalog.FeishuPermissionTemplate()["addons"]) {
				t.Fatal("QR must encode exactly addons, not profile metadata or user dependencies")
			}
			if err = json.Unmarshal(body, &addons); err != nil {
				t.Fatal(err)
			}
			if addons.Preset || len(addons.Scopes.Tenant) != 34 || len(addons.Scopes.User) != 1 || len(addons.Events.Items.Tenant) != 6 || len(addons.Events.Items.User) != 3 || len(addons.Callbacks.Items) != 1 {
				t.Fatal("template drift")
			}
			if slices.Contains(addons.Scopes.Tenant, "cardkit:card:read") || !slices.Contains(addons.Scopes.Tenant, "cardkit:card:write") {
				t.Fatal("card read must be removed without disabling card create/update")
			}
			b, _ := json.Marshal(got)
			if strings.Contains(string(b), "DEVICE-SENTINEL") {
				t.Fatal("device secret serializable")
			}
		})
	}
}
func TestRegistrationPollClassifiesResultsWithoutEchoingSecrets(t *testing.T) {
	cases := []struct {
		body, status string
		code         core.ErrorCode
	}{
		{`{"error":"authorization_pending"}`, "pending", ""},
		{`{"error":"slow_down"}`, "slow_down", ""},
		{`{}`, "pending", ""},
		{`{"error":"access_denied","error_description":"SECRET"}`, "", core.ErrorCodeAccessDenied},
		{`{"error":"expired_token"}`, "", core.ErrorCodeFlowExpired},
		{`{"client_id":"cli_test","client_secret":"SECRET"}`, "complete", ""},
		{`{"user_info":{"tenant_brand":"lark"}}`, "switch_region", ""},
		{`not json SECRET`, "", core.ErrorCodeOutcomeUnknown},
		{`{"client_id":"cli_partial"}`, "", core.ErrorCodeOutcomeUnknown},
	}
	for _, tc := range cases {
		t.Run(tc.body, func(t *testing.T) {
			client, err := NewRegistrationClient(&http.Client{Transport: transportFunc(func(r *http.Request) (*http.Response, error) {
				_ = r.ParseForm()
				if r.Form.Get("action") != "poll" || r.Form.Get("device_code") != "device" {
					t.Fatal("wrong poll")
				}
				return reply(tc.body), nil
			})})
			if err != nil {
				t.Fatal(err)
			}
			got, err := client.Poll(context.Background(), "feishu", "device", false)
			if tc.code != "" {
				if !core.IsErrorCode(err, tc.code) || strings.Contains(err.Error(), "SECRET") {
					t.Fatalf("error %v", err)
				}
				return
			}
			if err != nil || got.Status != tc.status {
				t.Fatalf("poll=%s %v", got.Status, err)
			}
			b, _ := json.Marshal(got)
			if strings.Contains(string(b), "SECRET") {
				t.Fatal("credentials serialized")
			}
		})
	}
}
func TestRegistrationRejectsForeignURLsAndInvalidInputs(t *testing.T) {
	for _, raw := range []string{"https://accounts.feishu.cn.evil.test/a", "http://accounts.feishu.cn/a", "https://user@accounts.feishu.cn/a", "https://accounts.feishu.cn:444/a", "https://accounts.feishu.cn/a#SECRET"} {
		client, _ := NewRegistrationClient(&http.Client{Transport: transportFunc(func(*http.Request) (*http.Response, error) {
			b, _ := json.Marshal(map[string]any{"device_code": "device", "verification_uri_complete": raw})
			return reply(string(b)), nil
		})})
		if _, err := client.Begin(context.Background(), "feishu", ""); err == nil {
			t.Fatal("unsafe verification URL accepted")
		}
	}
	calls := 0
	client, _ := NewRegistrationClient(&http.Client{Transport: transportFunc(func(*http.Request) (*http.Response, error) { calls++; return reply(`{}`), nil })})
	if _, err := client.Begin(context.Background(), "evil", ""); err == nil {
		t.Fatal("unknown region")
	}
	if _, err := client.Begin(context.Background(), "feishu", "../bad"); err == nil {
		t.Fatal("invalid app ID")
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := client.Begin(ctx, "feishu", ""); err == nil || calls != 0 {
		t.Fatal("cancelled call reached provider")
	}
}

func TestRegistrationRejectsNullAndLateBrandChangeWithoutNewBegin(t *testing.T) {
	for _, body := range []string{`null`, `{"client_id":"cli_fixture","client_secret":"SECRET","user_info":{"tenant_brand":"lark"}}`} {
		calls := 0
		client, _ := NewRegistrationClient(&http.Client{Transport: transportFunc(func(*http.Request) (*http.Response, error) { calls++; return reply(body), nil })})
		if _, err := client.Poll(context.Background(), "feishu", "device", false); !core.IsErrorCode(err, core.ErrorCodeOutcomeUnknown) || calls != 1 {
			t.Fatalf("unsafe poll %v calls %d", err, calls)
		}
	}
}
