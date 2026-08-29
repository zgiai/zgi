package wecom

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strconv"
	"sync/atomic"
	"testing"
	"time"

	"github.com/zgiai/zgi/api/internal/modules/integrations/core"
)

func TestProviderAccessTokenCachesWithRefreshSkew(t *testing.T) {
	var calls atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		calls.Add(1)
		if request.Method != http.MethodPost || request.URL.Path != "/cgi-bin/service/get_provider_token" || request.URL.RawQuery != "" {
			t.Errorf("unexpected provider token request: %s %s", request.Method, request.URL.RequestURI())
		}
		var body map[string]string
		if err := json.NewDecoder(request.Body).Decode(&body); err != nil {
			t.Errorf("decode provider token request: %v", err)
		}
		if len(body) != 2 || body["corpid"] != "ww_provider" || body["provider_secret"] != "provider-secret" {
			t.Errorf("provider token request body = %#v", body)
		}
		_ = json.NewEncoder(writer).Encode(map[string]any{
			"errcode": 0, "provider_access_token": " provider-token ", "expires_in": 7200,
		})
	}))
	defer server.Close()

	adapter, err := newForBaseURL(server.Client(), server.URL)
	if err != nil {
		t.Fatal(err)
	}
	adapter.suite.ProviderCorpID = " ww_provider "
	adapter.suite.ProviderSecret = " provider-secret "

	token, err := adapter.providerAccessToken(context.Background())
	if err != nil || token != "provider-token" {
		t.Fatalf("providerAccessToken() = (%q, %v)", token, err)
	}
	if cached, err := adapter.providerAccessToken(context.Background()); err != nil || cached != token {
		t.Fatalf("cached providerAccessToken() = (%q, %v)", cached, err)
	}
	if calls.Load() != 1 {
		t.Fatalf("provider token requests = %d, want 1", calls.Load())
	}

	adapter.mu.Lock()
	cached := adapter.providerToken
	adapter.mu.Unlock()
	wantHash := providerTokenCredentialHash("ww_provider", "provider-secret")
	if cached.credentialHash != wantHash {
		t.Fatal("cached provider token does not bind to the provider credentials")
	}
	remaining := time.Until(cached.expiresAt)
	if remaining < 114*time.Minute || remaining > 116*time.Minute {
		t.Fatalf("cached provider token lifetime = %s, want expires_in minus five minutes", remaining)
	}
}

func TestProviderAccessTokenRefreshesAfterCredentialRotationAndEviction(t *testing.T) {
	var calls atomic.Int32
	var secrets []string
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		var body map[string]string
		if err := json.NewDecoder(request.Body).Decode(&body); err != nil {
			t.Errorf("decode provider token request: %v", err)
		}
		secrets = append(secrets, body["provider_secret"])
		call := calls.Add(1)
		_ = json.NewEncoder(writer).Encode(map[string]any{
			"errcode": 0, "provider_access_token": "token-" + strconv.FormatInt(int64(call), 10), "expires_in": 7200,
		})
	}))
	defer server.Close()

	adapter, err := newForBaseURL(server.Client(), server.URL)
	if err != nil {
		t.Fatal(err)
	}
	adapter.suite.ProviderCorpID = "ww_provider"
	adapter.suite.ProviderSecret = "secret-one"

	first, err := adapter.providerAccessToken(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	adapter.suite.ProviderSecret = "secret-two"
	second, err := adapter.providerAccessToken(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	adapter.evictProviderAccessToken()
	third, err := adapter.providerAccessToken(context.Background())
	if err != nil {
		t.Fatal(err)
	}

	if first != "token-1" || second != "token-2" || third != "token-3" {
		t.Fatalf("tokens after rotation and eviction = %q, %q, %q", first, second, third)
	}
	if calls.Load() != 3 {
		t.Fatalf("provider token requests = %d, want 3", calls.Load())
	}
	if len(secrets) != 3 || secrets[0] != "secret-one" || secrets[1] != "secret-two" || secrets[2] != "secret-two" {
		t.Fatalf("provider secrets used by requests = %#v", secrets)
	}
}

func TestProviderAccessTokenRejectsUnavailableAndInvalidResponses(t *testing.T) {
	t.Run("credentials unavailable", func(t *testing.T) {
		adapter, err := newForBaseURL(nil, defaultBaseURL)
		if err != nil {
			t.Fatal(err)
		}
		if _, err = adapter.providerAccessToken(context.Background()); integrations.ErrorCode(err) != integrations.ErrorCodeDisabled {
			t.Fatalf("providerAccessToken() error = %v, code = %q", err, integrations.ErrorCode(err))
		}
	})

	t.Run("provider rejection", func(t *testing.T) {
		adapter, closeServer := providerTokenTestAdapter(t, map[string]any{"errcode": 40001, "errmsg": "invalid secret"})
		defer closeServer()
		if _, err := adapter.providerAccessToken(context.Background()); integrations.ErrorCode(err) != integrations.ErrorCodeAuthInvalid {
			t.Fatalf("providerAccessToken() error = %v, code = %q", err, integrations.ErrorCode(err))
		}
	})

	t.Run("incomplete response", func(t *testing.T) {
		adapter, closeServer := providerTokenTestAdapter(t, map[string]any{"errcode": 0, "provider_access_token": "token"})
		defer closeServer()
		if _, err := adapter.providerAccessToken(context.Background()); integrations.ErrorCode(err) != integrations.ErrorCodeResponseInvalid {
			t.Fatalf("providerAccessToken() error = %v, code = %q", err, integrations.ErrorCode(err))
		}
	})
}

func TestProviderAccessTokenDoesNotCacheInsideRefreshSkew(t *testing.T) {
	var calls atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, _ *http.Request) {
		calls.Add(1)
		_ = json.NewEncoder(writer).Encode(map[string]any{
			"errcode": 0, "provider_access_token": "short-lived", "expires_in": 60,
		})
	}))
	defer server.Close()
	adapter, err := newForBaseURL(server.Client(), server.URL)
	if err != nil {
		t.Fatal(err)
	}
	adapter.suite.ProviderCorpID = "ww_provider"
	adapter.suite.ProviderSecret = "provider-secret"

	if _, err = adapter.providerAccessToken(context.Background()); err != nil {
		t.Fatal(err)
	}
	if _, err = adapter.providerAccessToken(context.Background()); err != nil {
		t.Fatal(err)
	}
	if calls.Load() != 2 {
		t.Fatalf("short-lived provider token requests = %d, want 2", calls.Load())
	}
}

func providerTokenTestAdapter(t *testing.T, response map[string]any) (*Adapter, func()) {
	t.Helper()
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, _ *http.Request) {
		_ = json.NewEncoder(writer).Encode(response)
	}))
	adapter, err := newForBaseURL(server.Client(), server.URL)
	if err != nil {
		server.Close()
		t.Fatal(err)
	}
	adapter.suite.ProviderCorpID = "ww_provider"
	adapter.suite.ProviderSecret = "provider-secret"
	return adapter, server.Close
}
