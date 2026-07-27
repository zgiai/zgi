package x

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/zgiai/zgi/api/internal/modules/integrations"
)

func TestAppBearerCredentialValidationDoesNotCallProvider(t *testing.T) {
	const secret = "app-only-secret"
	var calls atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
		calls.Add(1)
	}))
	defer server.Close()

	adapter, err := newForBaseURL(server.Client(), server.URL)
	if err != nil {
		t.Fatal(err)
	}
	err = adapter.ValidateCredentials(context.Background(), integrations.CredentialValidationRequest{
		IntegrationID: IntegrationID,
		DriverID:      DriverID,
		AuthMethodID:  AppBearerAuthMethodID,
		Credentials:   map[string]string{"bearer_token": secret},
	})
	if err != nil || calls.Load() != 0 {
		t.Fatalf("ValidateCredentials() err = %v, calls = %d", err, calls.Load())
	}
}

func TestAppBearerConnectionValidationReturnsApplicationProfile(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		if request.Method != http.MethodGet || request.URL.Path != "/2/users/by/username/xdevelopers" {
			t.Errorf("request = %s %s", request.Method, request.URL)
		}
		if got := request.Header.Get("Authorization"); got != "Bearer app-token" {
			t.Errorf("Authorization = %q", got)
		}
		if got := request.URL.Query().Get("user.fields"); got != "id,name,username" {
			t.Errorf("user.fields = %q", got)
		}
		writer.Header().Set("X-Request-Id", "profile-request")
		_, _ = io.WriteString(writer, `{"data":{"id":"2244994945","name":"Developers","username":"XDevelopers"}}`)
	}))
	defer server.Close()
	adapter, err := newForBaseURL(server.Client(), server.URL)
	if err != nil {
		t.Fatal(err)
	}

	profile, err := adapter.ValidateConnection(context.Background(), appBearerConnection("app-token"))
	if err != nil {
		t.Fatal(err)
	}
	if profile.AccountID != "app-only" || profile.DisplayName != "X public data" ||
		profile.ProviderRequestID != "profile-request" ||
		!sameStrings(profile.GrantedScopes, []string{ScopeUsersRead, ScopePostsRead}) {
		t.Fatalf("profile = %#v", profile)
	}
}

func TestAppBearerExecutesOnlyRecentPublicSearch(t *testing.T) {
	const secret = "app-only-secret"
	var calls atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		calls.Add(1)
		if request.URL.Path != "/2/tweets/search/recent" ||
			request.Header.Get("Authorization") != "Bearer "+secret ||
			request.URL.Query().Get("query") != "golang" {
			t.Errorf("request = %s %s authorization=%q", request.Method, request.URL, request.Header.Get("Authorization"))
		}
		writer.Header().Set("X-Transaction-Id", "search-request")
		_, _ = io.WriteString(writer, `{
			"data":[{"id":"1","text":"bounded result","created_at":"2026-07-23T00:00:00Z","lang":"en","conversation_id":"1","possibly_sensitive":false,
			"public_metrics":{"retweet_count":1,"reply_count":2,"like_count":3,"quote_count":4,"bookmark_count":5,"impression_count":6}}],
			"meta":{"result_count":1}
		}`)
	}))
	defer server.Close()
	adapter, err := newForBaseURL(server.Client(), server.URL)
	if err != nil {
		t.Fatal(err)
	}

	result, err := adapter.Execute(context.Background(), integrations.ActionRequest{
		ActionID:   ActionSearchRecentPosts,
		Connection: appBearerConnection(secret),
		Input:      map[string]interface{}{"query": "golang", "max_results": 10},
	})
	if err != nil || result == nil || result.ProviderRequestID != "search-request" || result.ResultCount != 1 {
		t.Fatalf("result = %#v, err = %v", result, err)
	}
	if calls.Load() != 1 {
		t.Fatalf("calls = %d", calls.Load())
	}

	for _, actionID := range []string{ActionGetAccount, ActionListOwnPosts, ActionCreatePost} {
		_, err = adapter.Execute(context.Background(), integrations.ActionRequest{
			ActionID: actionID, Connection: appBearerConnection(secret),
		})
		if integrations.ErrorCode(err) != integrations.ErrorCodeAccessDenied {
			t.Fatalf("%s error = %v", actionID, err)
		}
	}
	if calls.Load() != 1 {
		t.Fatalf("user-context actions reached X: calls = %d", calls.Load())
	}
}

func TestAppBearerConnectionTestMapsProviderErrorsWithoutLeakingSecret(t *testing.T) {
	tests := []struct {
		status int
		code   string
	}{
		{status: http.StatusUnauthorized, code: integrations.ErrorCodeAuthInvalid},
		{status: http.StatusForbidden, code: integrations.ErrorCodeAccessDenied},
		{status: http.StatusTooManyRequests, code: integrations.ErrorCodeRateLimited},
	}
	for _, test := range tests {
		t.Run(fmt.Sprintf("status_%d", test.status), func(t *testing.T) {
			const secret = "must-not-leak"
			server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, _ *http.Request) {
				writer.Header().Set("Retry-After", "0")
				writer.WriteHeader(test.status)
				_, _ = io.WriteString(writer, `{"detail":"must-not-leak"}`)
			}))
			defer server.Close()
			adapter, err := newForBaseURL(server.Client(), server.URL)
			if err != nil {
				t.Fatal(err)
			}
			_, err = adapter.ValidateConnection(context.Background(), appBearerConnection(secret))
			if integrations.ErrorCode(err) != test.code {
				t.Fatalf("error = %v, code = %q", err, integrations.ErrorCode(err))
			}
			if strings.Contains(err.Error(), secret) {
				t.Fatalf("secret leaked in error: %v", err)
			}
		})
	}
}

func TestAppBearerRejectsWrongCredentialFieldAndMethod(t *testing.T) {
	adapter, err := New(nil)
	if err != nil {
		t.Fatal(err)
	}
	for _, request := range []integrations.CredentialValidationRequest{
		{
			IntegrationID: IntegrationID, DriverID: DriverID, AuthMethodID: AppBearerAuthMethodID,
			Credentials: map[string]string{"access_token": "wrong-field"},
		},
		{
			IntegrationID: IntegrationID, DriverID: DriverID, AuthMethodID: AccountOAuthAuthMethodID,
			Credentials: map[string]string{"bearer_token": "wrong-method"},
		},
	} {
		if err := adapter.ValidateCredentials(context.Background(), request); err == nil {
			t.Fatalf("ValidateCredentials(%#v) unexpectedly succeeded", request)
		}
	}
}

func appBearerConnection(token string) *integrations.ResolvedConnection {
	return &integrations.ResolvedConnection{
		IntegrationID: IntegrationID,
		DriverID:      DriverID,
		AuthMethodID:  AppBearerAuthMethodID,
		Credentials:   map[string]string{"bearer_token": token},
	}
}

func sameStrings(left, right []string) bool {
	if len(left) != len(right) {
		return false
	}
	for index := range left {
		if left[index] != right[index] {
			return false
		}
	}
	return true
}
