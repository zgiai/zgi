package feishu

import (
	"context"
	"encoding/json"
	"net/http"
	"reflect"
	"slices"
	"strings"
	"testing"

	"github.com/zgiai/zgi/api/internal/modules/connectors/core"
	"github.com/zgiai/zgi/api/internal/modules/connectors/providers"
)

// Protocol and field names are from the official Directory search API:
// https://open.feishu.cn/document/directory-v1/employee/search
func TestContactSearchUsesTenantIdentityAndMinimalFields(t *testing.T) {
	for _, region := range []string{"feishu", "lark"} {
		for _, paginated := range []bool{false, true} {
			t.Run(region+map[bool]string{false: "/first", true: "/next"}[paginated], func(t *testing.T) {
				args := map[string]any{"query": " 张三 "}
				pageSize, pageToken := 20, ""
				if paginated {
					pageSize, pageToken = 2, "cursor_1"
					args["page_size"], args["page_token"] = pageSize, pageToken
				}
				calls := 0
				client, err := New(&http.Client{Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
					calls++
					wantHost := map[string]string{"feishu": "open.feishu.cn", "lark": "open.larksuite.com"}[region]
					if r.URL.Host != wantHost {
						t.Fatal("wrong region")
					}
					if calls == 1 {
						if r.URL.Path != "/open-apis/auth/v3/tenant_access_token/internal" {
							t.Fatal("must use application credentials")
						}
						return fixtureResponse(`{"code":0,"tenant_access_token":"fixture-token","expire":7200}`), nil
					}
					if r.Method != "POST" || r.URL.Path != "/open-apis/directory/v1/employees/search" || r.URL.RawQuery != "employee_id_type=open_id" || r.Header.Get("Authorization") != "Bearer fixture-token" {
						t.Fatal("search route, identity or ID type changed")
					}
					var body map[string]any
					if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
						t.Fatal(err)
					}
					want := map[string]any{"query": "张三", "page_request": map[string]any{"page_size": float64(pageSize), "page_token": pageToken}, "required_fields": []any{"base_info.employee_id", "base_info.name.name"}}
					if !reflect.DeepEqual(body, want) {
						t.Fatalf("unexpected search fields: %#v", body)
					}
					return fixtureResponse(`{"code":0,"data":{"employees":[
{"base_info":{"employee_id":"ou_one","name":{"name":{"default_value":"张三"}},"mobile":"PRIVATE","email":"PRIVATE"},"work_info":{"job_number":"PRIVATE"}},
{"base_info":{"employee_id":"ou_two","name":{"name":{"default_value":"张三"}}}}],
"page_response":{"has_more":true,"page_token":"cursor_2"},"abnormals":[]}}`), nil
				})})
				if err != nil {
					t.Fatal(err)
				}
				input := fixtureInput()
				input.Values["region"] = region
				res, err := client.InvokeCredentials(context.Background(), "feishu.contact.search", input, args)
				if err != nil || calls != 2 {
					t.Fatalf("search must be one page and must not send messages: %v, calls=%d", err, calls)
				}
				wantData := map[string]any{"users": []any{map[string]any{"open_id": "ou_one", "name": "张三"}, map[string]any{"open_id": "ou_two", "name": "张三"}}, "has_more": true, "page_token": "cursor_2"}
				if !reflect.DeepEqual(res.Value["data"], wantData) {
					t.Fatalf("duplicate names must stay distinct without exposing other directory fields: %#v", res.Value["data"])
				}
				op, _ := operationRequest("feishu.contact.search", args)
				if op.write {
					t.Fatal("a search POST is read-only")
				}
			})
		}
	}
}

func TestContactSearchRejectsUnboundedOrUntrustedInputBeforeNetwork(t *testing.T) {
	client, err := New(&http.Client{Transport: roundTripFunc(func(*http.Request) (*http.Response, error) {
		t.Fatal("invalid search reached network")
		return nil, nil
	})})
	if err != nil {
		t.Fatal(err)
	}
	for _, args := range []map[string]any{
		{}, {"query": ""}, {"query": " \n "}, {"query": "　"}, {"query": strings.Repeat("张", 101)},
		{"query": "张", "page_size": 0}, {"query": "张", "page_size": 51}, {"query": "张", "page_size": 1.5},
		{"query": "张", "page_token": strings.Repeat("x", 1025)},
		{"query": "张", "required_fields": []string{"base_info.mobile"}},
		{"query": "张", "employee_id_type": "employee_id"}, {"query": "张", "url": "https://evil.invalid"},
	} {
		if _, err := client.InvokeCredentials(context.Background(), "feishu.contact.search", fixtureInput(), args); err == nil || providers.FailureOf(err).Code != core.ErrorCodeInvalidInput {
			t.Fatalf("invalid search input accepted: %#v (%v)", args, err)
		}
	}
}

func TestContactSearchDistinguishesNoMatchFromPermissionAndProtocolErrors(t *testing.T) {
	for _, tc := range []struct {
		name, response string
		want           core.ErrorCode
	}{
		{"empty", `{"code":0,"data":{"employees":[],"page_response":{"has_more":false}}}`, ""},
		{"zero row status", `{"code":0,"data":{"employees":[],"page_response":{"has_more":false},"abnormals":[{"row_error":0,"field_errors":{}}]}}`, ""},
		{"missing scope", `{"code":99991672}`, core.ErrorCodePermissionMissing},
		{"row denied", `{"code":0,"data":{"abnormals":[{"row_error":1000}],"page_response":{"has_more":false}}}`, core.ErrorCodePermissionMissing},
		{"name denied", `{"code":0,"data":{"abnormals":[{"field_errors":{"base_info.name.name":1000}}]}}`, core.ErrorCodePermissionMissing},
		{"field failure", `{"code":0,"data":{"abnormals":[{"field_errors":{"base_info.name.name":2000}}]}}`, core.ErrorCodeProviderUnavailable},
		{"missing data", `{"code":0}`, core.ErrorCodeProviderUnavailable},
		{"missing cursor", `{"code":0,"data":{"employees":[],"page_response":{"has_more":true}}}`, core.ErrorCodeProviderUnavailable},
		{"repeated cursor", `{"code":0,"data":{"employees":[],"page_response":{"has_more":true,"page_token":"cursor_1"}}}`, core.ErrorCodeProviderUnavailable},
		{"unexpected shape", `{"code":0,"data":{"employees":{},"page_response":{"has_more":false}}}`, core.ErrorCodeProviderUnavailable},
		{"missing name", `{"code":0,"data":{"employees":[{"base_info":{"employee_id":"ou_one"}}],"page_response":{"has_more":false}}}`, core.ErrorCodeProviderUnavailable},
		{"wrong ID", `{"code":0,"data":{"employees":[{"base_info":{"employee_id":"employee_1","name":{"name":{"default_value":"张三"}}}}],"page_response":{"has_more":false}}}`, core.ErrorCodeProviderUnavailable},
	} {
		t.Run(tc.name, func(t *testing.T) {
			calls := 0
			client, err := New(&http.Client{Transport: roundTripFunc(func(*http.Request) (*http.Response, error) {
				calls++
				if calls == 1 {
					return fixtureResponse(`{"code":0,"tenant_access_token":"fixture-token","expire":7200}`), nil
				}
				return fixtureResponse(tc.response), nil
			})})
			if err != nil {
				t.Fatal(err)
			}
			res, err := client.InvokeCredentials(context.Background(), "feishu.contact.search", fixtureInput(), map[string]any{"query": "张三", "page_token": "cursor_1"})
			if calls != 2 {
				t.Fatal("search retried or silently used another endpoint")
			}
			if tc.want != "" {
				if err == nil || providers.FailureOf(err).Code != tc.want || providers.FailureOf(err).Unknown {
					t.Fatalf("incorrect read failure: %v", err)
				}
			} else if err != nil || !reflect.DeepEqual(res.Value["data"], map[string]any{"users": []any{}, "has_more": false}) {
				t.Fatalf("empty result: %#v %v", res.Value, err)
			}
		})
	}
}

func TestContactSearchRequiresVerifiedTenantSearchAndNameGrants(t *testing.T) {
	for _, tc := range []struct {
		name, grants string
		ready        bool
	}{
		{"none", `[]`, false},
		{"only search", `[{"scope_name":"directory:employee:search","grant_status":1}]`, false},
		{"user grants", `[{"scope_name":"directory:employee:search","grant_status":1,"scope_type":"user"},{"scope_name":"directory:employee.base.name.name:read","grant_status":1,"scope_type":"user"}]`, false},
		{"both tenant grants", `[{"scope_name":"directory:employee:search","grant_status":1,"scope_type":"tenant"},{"scope_name":"directory:employee.base.name.name:read","grant_status":1,"scope_type":"tenant"}]`, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			client, err := New(&http.Client{Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
				switch r.URL.Path {
				case "/open-apis/auth/v3/tenant_access_token/internal":
					return fixtureResponse(`{"code":0,"tenant_access_token":"fixture-token","expire":7200}`), nil
				case "/open-apis/bot/v3/info":
					return fixtureResponse(`{"code":0,"bot":{"open_id":"ou_bot"}}`), nil
				case "/open-apis/application/v6/scopes":
					return fixtureResponse(`{"code":0,"data":{"scopes":` + tc.grants + `}}`), nil
				default:
					t.Fatal("verification must not search directory contents")
					return nil, nil
				}
			})})
			if err != nil {
				t.Fatal(err)
			}
			v, err := client.VerifyCredentials(context.Background(), fixtureInput())
			if err != nil || slices.Contains(v.Operations, "feishu.contact.search") != tc.ready {
				t.Fatalf("unproven search permission: %v %#v", err, v.Operations)
			}
		})
	}
}
