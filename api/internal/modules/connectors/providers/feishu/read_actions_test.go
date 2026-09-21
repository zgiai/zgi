package feishu

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"reflect"
	"slices"
	"strings"
	"testing"

	"github.com/zgiai/zgi/api/internal/modules/connectors/core"
	"github.com/zgiai/zgi/api/internal/modules/connectors/providers"
)

const tenantFixture = `{"code":0,"data":{"tenant":{"name":"企业名称","display_id":"F123456789","tenant_key":"tenant_fixture","tenant_tag":7,"domain":"PRIVATE","avatar":{"avatar_origin":"PRIVATE"},"extra":"PRIVATE"}}}`

func memberPageFixture(count int, hasMore bool, cursor string) string {
	items := make([]any, count)
	for i := range items {
		items[i] = map[string]any{"member_id_type": "open_id", "member_id": fmt.Sprintf("ou_%d", i), "name": "同名成员", "tenant_key": "PRIVATE", "email": "PRIVATE"}
	}
	raw, _ := json.Marshal(map[string]any{"code": 0, "data": map[string]any{"items": items, "has_more": hasMore, "page_token": cursor, "member_total": count + 10}})
	return string(raw)
}

func TestReadActionsFixedRoutesAndMinimalProjection(t *testing.T) {
	for _, region := range []string{"feishu", "lark"} {
		for _, tc := range []struct {
			operation, path, query, payload string
			args                            map[string]any
		}{
			{"feishu.tenant.get", "/open-apis/tenant/v2/tenant/query", "", tenantFixture, map[string]any{}},
			{"feishu.chat.members.list", "/open-apis/im/v1/chats/oc_group/members", "member_id_type=open_id&page_size=20", memberPageFixture(120, true, "next"), map[string]any{"chat_id": "oc_group"}},
			{"feishu.chat.members.list", "/open-apis/im/v1/chats/oc_group/members", "member_id_type=open_id&page_size=100&page_token=previous", memberPageFixture(0, true, "next"), map[string]any{"chat_id": "oc_group", "page_size": 100, "page_token": "previous"}},
		} {
			t.Run(region+"/"+tc.operation+"/"+tc.query, func(t *testing.T) {
				calls := 0
				client, err := New(&http.Client{Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
					calls++
					if r.URL.Host != map[string]string{"feishu": "open.feishu.cn", "lark": "open.larksuite.com"}[region] {
						t.Fatal("wrong region")
					}
					if calls == 1 {
						if r.URL.Path != "/open-apis/auth/v3/tenant_access_token/internal" {
							t.Fatal("must use application identity")
						}
						return fixtureResponse(`{"code":0,"tenant_access_token":"fixture-token","expire":7200}`), nil
					}
					if r.Method != "GET" || r.URL.Path != tc.path || r.URL.RawQuery != tc.query || r.Header.Get("Authorization") != "Bearer fixture-token" || r.ContentLength != 0 {
						t.Fatalf("unexpected business request: %s %s", r.Method, r.URL)
					}
					response := fixtureResponse(tc.payload)
					response.Header.Set("X-Tt-Logid", "safe-request-123")
					return response, nil
				})})
				if err != nil {
					t.Fatal(err)
				}
				input := fixtureInput()
				input.Values["region"] = region
				response, err := client.InvokeCredentials(context.Background(), tc.operation, input, tc.args)
				if err != nil || calls != 2 || response.Value["truncated"] != false || response.RequestID != "safe-request-123" {
					t.Fatalf("read must return one complete page: %#v %v; calls=%d", response, err, calls)
				}
				serialized, _ := json.Marshal(response.Value)
				if strings.Contains(string(serialized), "PRIVATE") || strings.Contains(string(serialized), "member_total") {
					t.Fatal("non-allowlisted fields exposed")
				}
				data := response.Value["data"].(map[string]any)
				if tc.operation == "feishu.tenant.get" {
					want := map[string]any{"tenant": map[string]any{"name": "企业名称", "display_id": "F123456789", "tenant_key": "tenant_fixture", "tenant_tag": float64(7)}}
					if !reflect.DeepEqual(data, want) {
						t.Fatalf("tenant projection: %#v", data)
					}
				} else {
					wantCount := 120
					if tc.args["page_token"] != nil {
						wantCount = 0
					}
					if len(data["members"].([]any)) != wantCount || data["has_more"] != true || data["page_token"] != "next" {
						t.Fatal("lost same-name members or inferred pagination from count")
					}
				}
				request, err := operationRequest(tc.operation, tc.args)
				if err != nil || request.write {
					t.Fatal("read classified as write")
				}
			})
		}
	}
}

func invokeReadFixture(t *testing.T, operation, payload string, args map[string]any) (providers.Response, error) {
	t.Helper()
	calls := 0
	client, err := New(&http.Client{Transport: roundTripFunc(func(*http.Request) (*http.Response, error) {
		calls++
		if calls == 1 {
			return fixtureResponse(`{"code":0,"tenant_access_token":"fixture-token","expire":7200}`), nil
		}
		response := fixtureResponse(payload)
		response.Header.Set("X-Tt-Logid", "safe-request-123")
		return response, nil
	})})
	if err != nil {
		t.Fatal(err)
	}
	response, err := client.InvokeCredentials(context.Background(), operation, fixtureInput(), args)
	if calls != 2 {
		t.Fatalf("expected token and one business call only, got %d: %v", calls, err)
	}
	return response, err
}

func TestReadActionsRejectIncompleteOrUnboundedResponses(t *testing.T) {
	cases := []struct{ operation, name, payload string }{
		{"feishu.tenant.get", "missing", `{"code":0}`},
		{"feishu.tenant.get", "missing key", `{"code":0,"data":{"tenant":{"name":"Tenant","display_id":"F1"}}}`},
		{"feishu.tenant.get", "invalid tag", strings.Replace(tenantFixture, `"tenant_tag":7`, `"tenant_tag":1.5`, 1)},
		{"feishu.tenant.get", "long name", strings.Replace(tenantFixture, "企业名称", strings.Repeat("企", 257), 1)},
		{"feishu.tenant.get", "long identity", strings.Replace(tenantFixture, "tenant_fixture", strings.Repeat("x", 129), 1)},
		{"feishu.tenant.get", "empty identity", strings.Replace(tenantFixture, "F123456789", "", 1)},
		{"feishu.tenant.get", "secret identity", strings.Replace(tenantFixture, "tenant_fixture", "fixture-token", 1)},
		{"feishu.chat.members.list", "missing items", `{"code":0,"data":{"has_more":false}}`},
		{"feishu.chat.members.list", "null items", `{"code":0,"data":{"items":null,"has_more":false}}`},
		{"feishu.chat.members.list", "missing has more", `{"code":0,"data":{"items":[]}}`},
		{"feishu.chat.members.list", "missing cursor", memberPageFixture(1, true, "")},
		{"feishu.chat.members.list", "repeated cursor", memberPageFixture(1, true, "previous")},
		{"feishu.chat.members.list", "long cursor", memberPageFixture(1, true, strings.Repeat("x", 1025))},
		{"feishu.chat.members.list", "invalid cursor type", `{"code":0,"data":{"items":[],"has_more":true,"page_token":123}}`},
		{"feishu.chat.members.list", "secret cursor", memberPageFixture(1, true, "fixture-secret-not-real")},
		{"feishu.chat.members.list", "wrong ID type", strings.Replace(memberPageFixture(1, false, ""), "open_id", "user_id", 1)},
		{"feishu.chat.members.list", "malformed ID", strings.Replace(memberPageFixture(1, false, ""), "ou_0", "ou_../invalid", 1)},
		{"feishu.chat.members.list", "secret ID", strings.Replace(memberPageFixture(1, false, ""), "ou_0", "ou_fixture-token", 1)},
		{"feishu.chat.members.list", "long ID", strings.Replace(memberPageFixture(1, false, ""), "ou_0", "ou_"+strings.Repeat("x", 126), 1)},
		{"feishu.chat.members.list", "missing name", strings.Replace(memberPageFixture(1, false, ""), "同名成员", " ", 1)},
		{"feishu.chat.members.list", "long name", strings.Replace(memberPageFixture(1, false, ""), "同名成员", strings.Repeat("名", 257), 1)},
		{"feishu.chat.members.list", "member limit", memberPageFixture(1001, true, "next")},
		{"feishu.chat.members.list", "byte limit", strings.Repeat("x", maxResponseBytes+1)},
	}
	for _, tc := range cases {
		t.Run(tc.operation+"/"+tc.name, func(t *testing.T) {
			args := map[string]any{}
			if tc.operation == "feishu.chat.members.list" {
				args = map[string]any{"chat_id": "oc_group", "page_token": "previous"}
			}
			response, err := invokeReadFixture(t, tc.operation, tc.payload, args)
			if err == nil || providers.FailureOf(err).Code != core.ErrorCodeProviderUnavailable || providers.FailureOf(err).Unknown || response.Value != nil || providers.FailureOf(err).RequestID != "safe-request-123" {
				t.Fatalf("malformed read returned success/cursor or wrong failure: %#v %v", response, err)
			}
		})
	}
}

func TestReadActionsPreserveCommonErrorsAndSafeRequestIDs(t *testing.T) {
	for _, operation := range []string{"feishu.tenant.get", "feishu.chat.members.list"} {
		for _, tc := range []struct {
			name, payload, requestID string
			status                   int
			want                     core.ErrorCode
			retryable                bool
		}{
			{"permission", `{"code":99991672}`, "safe-request-123", 403, core.ErrorCodePermissionMissing, false},
			{"expired token", `{"code":99991663}`, "safe-request-123", 401, core.ErrorCodeConnectionNotReady, false},
			{"http forbidden", `{"code":0}`, "safe-request-123", 403, core.ErrorCodeResourceAccessDenied, false},
			{"http unauthenticated", `{"code":0}`, "safe-request-123", 401, core.ErrorCodeConnectionNotReady, false},
			{"rate limit", `{"code":0}`, "safe-request-123", 429, core.ErrorCodeRateLimited, true},
			{"server failure", `{"code":0}`, "safe-request-123", 503, core.ErrorCodeProviderUnavailable, false},
			{"secret request id", `{"code":99991672}`, "request-fixture-secret-not-real", 403, core.ErrorCodePermissionMissing, false},
			{"token request id", `{"code":99991672}`, "request-fixture-token", 403, core.ErrorCodePermissionMissing, false},
			{"malformed request id", `{"code":99991672}`, "request id", 403, core.ErrorCodePermissionMissing, false},
		} {
			t.Run(operation+"/"+tc.name, func(t *testing.T) {
				calls := 0
				client, err := New(&http.Client{Transport: roundTripFunc(func(*http.Request) (*http.Response, error) {
					calls++
					if calls == 1 {
						return fixtureResponse(`{"code":0,"tenant_access_token":"fixture-token","expire":7200}`), nil
					}
					response := fixtureResponse(tc.payload)
					response.Header.Set("X-Tt-Logid", tc.requestID)
					response.StatusCode = tc.status
					return response, nil
				})})
				if err != nil {
					t.Fatal(err)
				}
				args := map[string]any{}
				if operation == "feishu.chat.members.list" {
					args["chat_id"] = "oc_group"
				}
				response, err := client.InvokeCredentials(context.Background(), operation, fixtureInput(), args)
				wantRequestID := ""
				if tc.requestID == "safe-request-123" {
					wantRequestID = tc.requestID
				}
				if err == nil || calls != 2 || response.Value != nil {
					t.Fatalf("failed read retried or returned success: %v %d", err, calls)
				}
				f := providers.FailureOf(err)
				if f.Code != tc.want || f.Unknown || f.Retryable != tc.retryable || f.RequestID != wantRequestID {
					t.Fatalf("common failure or request ID changed: %#v", f)
				}
			})
		}
	}
}

func TestReadActionsCompletePageAndSecretRedaction(t *testing.T) {
	for _, count := range []int{0, 1, 101, 1000} {
		payload := strings.ReplaceAll(memberPageFixture(count, false, "ignored"), "同名成员", "fixture-token fixture-secret-not-real")
		response, err := invokeReadFixture(t, "feishu.chat.members.list", payload, map[string]any{"chat_id": "oc_group", "page_size": 1})
		if err != nil {
			t.Fatal(err)
		}
		data := response.Value["data"].(map[string]any)
		serialized, _ := json.Marshal(response.Value)
		if len(data["members"].([]any)) != count || data["page_token"] != nil || response.Value["truncated"] != false || strings.Contains(string(serialized), "fixture-token") || strings.Contains(string(serialized), "fixture-secret") {
			t.Fatal("page silently truncated, names merged, terminal cursor leaked, or secret exposed")
		}
	}
	response, err := invokeReadFixture(t, "feishu.tenant.get", strings.Replace(tenantFixture, "企业名称", "fixture-token fixture-secret-not-real", 1), map[string]any{})
	serialized, _ := json.Marshal(response.Value)
	if err != nil || strings.Contains(string(serialized), "fixture-token") || strings.Contains(string(serialized), "fixture-secret") {
		t.Fatal("tenant name secret not redacted")
	}
}

func TestReadActionsRejectInvalidInputBeforeNetwork(t *testing.T) {
	client, err := New(&http.Client{Transport: roundTripFunc(func(*http.Request) (*http.Response, error) {
		t.Fatal("invalid input reached network")
		return nil, nil
	})})
	if err != nil {
		t.Fatal(err)
	}
	for _, args := range []map[string]any{{}, {"chat_id": ""}, {"chat_id": "../../auth"}, {"chat_id": "oc group"}, {"chat_id": strings.Repeat("x", 129)}, {"chat_id": "oc_group", "page_size": 0}, {"chat_id": "oc_group", "page_size": 101}, {"chat_id": "oc_group", "page_size": 1.5}, {"chat_id": "oc_group", "page_token": strings.Repeat("x", 1025)}, {"chat_id": "oc_group", "member_id_type": "user_id"}, {"chat_id": "oc_group", "url": "https://evil.invalid"}, {"chat_id": "oc_group", "connection_id": "other"}} {
		if _, err := client.InvokeCredentials(context.Background(), "feishu.chat.members.list", fixtureInput(), args); err == nil || providers.FailureOf(err).Code != core.ErrorCodeInvalidInput {
			t.Fatalf("invalid members input accepted: %#v %v", args, err)
		}
	}
	for _, key := range []string{"tenant_key", "tenant_id", "url", "token", "page_size"} {
		if _, err := client.InvokeCredentials(context.Background(), "feishu.tenant.get", fixtureInput(), map[string]any{key: "other"}); err == nil || providers.FailureOf(err).Code != core.ErrorCodeInvalidInput {
			t.Fatal("tenant input accepted")
		}
	}
}

func TestReadActionsVerifyOnlyGrantsWithoutBusinessProbes(t *testing.T) {
	for _, scope := range []string{"", "tenant:tenant:readonly", "im:chat", "im:chat:readonly", "im:chat.members:read", "im:chat.group_info:readonly"} {
		for _, subject := range []string{"tenant", "user"} {
			t.Run(scope+"/"+subject, func(t *testing.T) {
				calls := 0
				client, err := New(&http.Client{Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
					calls++
					switch r.URL.Path {
					case "/open-apis/auth/v3/tenant_access_token/internal":
						return fixtureResponse(`{"code":0,"tenant_access_token":"fixture-token","expire":7200}`), nil
					case "/open-apis/bot/v3/info":
						return fixtureResponse(`{"code":0,"bot":{"open_id":"ou_bot"}}`), nil
					case "/open-apis/application/v6/scopes":
						return fixtureResponse(fmt.Sprintf(`{"code":0,"data":{"scopes":[{"scope_name":%q,"scope_type":%q,"grant_status":1}]}}`, scope, subject)), nil
					default:
						t.Fatal("verification must not query tenant or group contents")
						return nil, nil
					}
				})})
				if err != nil {
					t.Fatal(err)
				}
				verified, err := client.VerifyCredentials(context.Background(), fixtureInput())
				wantTenant := subject == "tenant" && scope == "tenant:tenant:readonly"
				wantMembers := subject == "tenant" && strings.HasPrefix(scope, "im:chat")
				if err != nil || calls != 3 || slices.Contains(verified.Operations, "feishu.tenant.get") != wantTenant || slices.Contains(verified.Operations, "feishu.chat.members.list") != wantMembers {
					t.Fatalf("unproven permission or wrong OR requirement: %v %#v", err, verified.Operations)
				}
			})
		}
	}
}

func TestReadActionsEndpointErrorsStayLocalAndNeverReplay(t *testing.T) {
	for _, tc := range []struct {
		operation string
		codes     []int
		want      core.ErrorCode
	}{
		{"feishu.tenant.get", []int{1184000, 1184001}, core.ErrorCodeResourceAccessDenied},
		{"feishu.chat.members.list", []int{232001, 232006}, core.ErrorCodeInvalidInput},
		{"feishu.chat.members.list", []int{232009, 232010, 232011, 232033}, core.ErrorCodeResourceAccessDenied},
		{"feishu.chat.members.list", []int{232004, 232025, 232034}, core.ErrorCodeApplicationNotReady},
	} {
		for _, code := range tc.codes {
			t.Run(fmt.Sprintf("%s/%d", tc.operation, code), func(t *testing.T) {
				args := map[string]any{}
				if tc.operation == "feishu.chat.members.list" {
					args["chat_id"] = "oc_group"
				}
				payload := fmt.Sprintf(`{"code":%d,"msg":"fixture-token fixture-secret-not-real"}`, code)
				response, err := invokeReadFixture(t, tc.operation, payload, args)
				if err == nil || providers.FailureOf(err).Code != tc.want || providers.FailureOf(err).Unknown || providers.FailureOf(err).RequestID != "safe-request-123" || response.Value != nil || strings.Contains(err.Error(), "fixture-secret") {
					t.Fatalf("endpoint classification failed: %#v %v", response, err)
				}
				// Codes are not globally reclassified, especially for existing writes.
				_, oldErr := invokeReadFixture(t, "feishu.message.send", payload, map[string]any{"receive_id": "ou_user", "receive_id_type": "open_id", "text": "hello"})
				if oldErr == nil || providers.FailureOf(oldErr).Code != core.ErrorCodeProviderUnavailable || !providers.FailureOf(oldErr).Unknown {
					t.Fatal("endpoint-specific code changed existing operation behavior")
				}
			})
		}
	}
}
