package feishu

import (
	"context"
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/zgiai/zgi/api/internal/modules/connectors/core"
	"github.com/zgiai/zgi/api/internal/modules/connectors/providers"
)

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }
func fixtureResponse(body string) *http.Response {
	return &http.Response{StatusCode: 200, Header: http.Header{}, Body: io.NopCloser(strings.NewReader(body))}
}
func fixtureInput() providers.CredentialInput {
	return providers.CredentialInput{Values: map[string]string{"region": "feishu", "app_id": "cli_fixture", "app_secret": "fixture-secret-not-real"}}
}

func TestVerifyAppCredentialsUsesTopLevelTokenAndActualTenantScopes(t *testing.T) {
	calls := []string{}
	client, err := New(&http.Client{Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
		calls = append(calls, r.URL.Path)
		if r.URL.Host != "open.feishu.cn" {
			t.Fatalf("unexpected host %s", r.URL.Host)
		}
		switch r.URL.Path {
		case "/open-apis/auth/v3/tenant_access_token/internal":
			return fixtureResponse(`{"code":0,"tenant_access_token":"fixture-token-not-real","expire":7200}`), nil
		case "/open-apis/bot/v3/info":
			if r.Header.Get("Authorization") != "Bearer fixture-token-not-real" {
				t.Fatal("wrong token")
			}
			return fixtureResponse(`{"code":0,"bot":{"open_id":"ou_bot","app_name":"Fixture","activate_status":2}}`), nil
		case "/open-apis/application/v6/scopes":
			return fixtureResponse(`{"code":0,"data":{"scopes":[{"scope_name":"im:chat:read","scope_type":"tenant","grant_status":1},{"scope_name":"im:message:readonly","scope_type":"tenant","grant_status":2},{"scope_name":"docx:document:readonly","scope_type":"user","grant_status":1}]}}`), nil
		default:
			t.Fatalf("verification must not invoke business writes: %s", r.URL.Path)
			return nil, nil
		}
	})})
	if err != nil {
		t.Fatal(err)
	}
	v, err := client.VerifyCredentials(context.Background(), fixtureInput())
	if err != nil || v.AccountID != "feishu:cli_fixture" || v.DisplayName != "Fixture" {
		t.Fatalf("verification %v %#v", err, v)
	}
	got := strings.Join(v.Operations, ",")
	if !strings.Contains(got, "feishu.chat.list") || strings.Contains(got, "feishu.message.list") || strings.Contains(got, "feishu.document.read") || len(calls) != 3 {
		t.Fatalf("unproven permissions exposed: %v", v.Operations)
	}
	if v.Evidence == nil || v.Evidence.EventsEnabled || v.Evidence.UserAuthorizationEnabled {
		t.Fatal("invented event/user readiness")
	}
}

func TestProviderFailuresAreNotAllInvalidSecrets(t *testing.T) {
	for _, tc := range []struct {
		body string
		want core.ErrorCode
	}{
		{`{"code":10015,"msg":"fixture-secret-not-real"}`, core.ErrorCodeInvalidAppCredentials},
		{`{"code":10014}`, core.ErrorCodeApplicationNotReady},
		{`{"code":99991672}`, core.ErrorCodePermissionMissing},
		{`{"code":99991400}`, core.ErrorCodeRateLimited},
		{`{"code":0,"data":{"tenant_access_token":"nested-invalid"}}`, core.ErrorCodeProviderUnavailable},
		{`{}`, core.ErrorCodeProviderUnavailable},
	} {
		client, _ := New(&http.Client{Transport: roundTripFunc(func(*http.Request) (*http.Response, error) { return fixtureResponse(tc.body), nil })})
		_, err := client.VerifyCredentials(context.Background(), fixtureInput())
		failure := providers.FailureOf(err)
		if err == nil || failure.Code != tc.want || strings.Contains(err.Error(), "fixture-secret") {
			t.Fatalf("%s: %v", tc.body, err)
		}
	}
}

func TestInvalidInputNeverCallsProvider(t *testing.T) {
	client, _ := New(&http.Client{Transport: roundTripFunc(func(*http.Request) (*http.Response, error) { t.Fatal("invalid input reached network"); return nil, nil })})
	bad := fixtureInput()
	bad.Values["region"] = "http://localhost"
	if _, err := client.VerifyCredentials(context.Background(), bad); err == nil {
		t.Fatal("arbitrary origin")
	}
	if _, err := client.InvokeCredentials(context.Background(), "feishu.message.get", fixtureInput(), map[string]any{"message_id": "../../auth"}); err == nil {
		t.Fatal("path traversal")
	}
}

func TestLegacyDocumentedTenantGrantResponseWithoutSubjectIsSupported(t *testing.T) {
	client, err := New(&http.Client{Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
		switch r.URL.Path {
		case "/open-apis/auth/v3/tenant_access_token/internal":
			return fixtureResponse(`{"code":0,"tenant_access_token":"fixture-token","expire":7200}`), nil
		case "/open-apis/bot/v3/info":
			return fixtureResponse(`{"code":0,"bot":{"open_id":"ou_bot"}}`), nil
		default:
			return fixtureResponse(`{"code":0,"data":{"scopes":[{"scope_name":"im:chat:read","grant_status":1},{"scope_name":"im:message:readonly","scope_type":"user","grant_status":1}]}}`), nil
		}
	})})
	if err != nil {
		t.Fatal(err)
	}
	verified, err := client.VerifyCredentials(context.Background(), fixtureInput())
	if err != nil {
		t.Fatal(err)
	}
	got := strings.Join(verified.Operations, ",")
	if !strings.Contains(got, "feishu.chat.list") || strings.Contains(got, "feishu.message.list") {
		t.Fatal("tenant/user evidence mixed")
	}
}
