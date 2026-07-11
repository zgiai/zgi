package provider

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	adapter "github.com/zgiai/zgi/api/internal/modules/llm/protocol/adapters"
)

func TestOpenRouterAdapterGetBalance_UsesCurrentKeyEndpoint(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/key" {
			t.Fatalf("path = %q, want %q", r.URL.Path, "/key")
		}
		if got := r.Header.Get("Authorization"); got != "Bearer runtime-key" {
			t.Fatalf("Authorization = %q, want Bearer runtime-key", got)
		}
		fmt.Fprint(w, `{"data":{"limit":10,"limit_remaining":2.5,"usage":7.5}}`)
	}))
	defer server.Close()

	a, err := NewOpenRouterAdapter(&adapter.AdapterConfig{APIKey: "config-key", BaseURL: server.URL})
	if err != nil {
		t.Fatalf("NewOpenRouterAdapter() error = %v", err)
	}

	balance, err := a.GetBalance(context.Background(), "runtime-key")
	if err != nil {
		t.Fatalf("GetBalance() error = %v", err)
	}
	if balance.Scope != adapter.BalanceScopeKeyLimit {
		t.Fatalf("Scope = %q, want %q", balance.Scope, adapter.BalanceScopeKeyLimit)
	}
	if got := balance.Remaining.String(); got != "2.5" {
		t.Fatalf("Remaining = %q, want %q", got, "2.5")
	}
	if balance.IsUnlimited {
		t.Fatal("IsUnlimited = true, want false")
	}
	if balance.Spendable == nil || !*balance.Spendable {
		t.Fatalf("Spendable = %v, want true", balance.Spendable)
	}
}

func TestOpenRouterAdapterGetBalance_NullLimitMeansUnlimited(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, `{"data":{"limit":null,"limit_remaining":null,"usage":4}}`)
	}))
	defer server.Close()

	a, err := NewOpenRouterAdapter(&adapter.AdapterConfig{APIKey: "config-key", BaseURL: server.URL})
	if err != nil {
		t.Fatalf("NewOpenRouterAdapter() error = %v", err)
	}
	balance, err := a.GetBalance(context.Background(), "runtime-key")
	if err != nil {
		t.Fatalf("GetBalance() error = %v", err)
	}
	if !balance.IsUnlimited {
		t.Fatal("IsUnlimited = false, want true")
	}
}

func TestOpenRouterAdapterGetBalance_RejectsMissingLimitFields(t *testing.T) {
	for _, body := range []string{
		`{}`,
		`{"data":{"usage":4}}`,
	} {
		t.Run(body, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				fmt.Fprint(w, body)
			}))
			defer server.Close()

			a, err := NewOpenRouterAdapter(&adapter.AdapterConfig{APIKey: "config-key", BaseURL: server.URL})
			if err != nil {
				t.Fatalf("NewOpenRouterAdapter() error = %v", err)
			}
			if _, err := a.GetBalance(context.Background(), "runtime-key"); err == nil {
				t.Fatal("GetBalance() error = nil, want malformed response error")
			}
		})
	}
}

func TestOpenAIAdapterGetBalance_IsUnsupportedForInferenceKey(t *testing.T) {
	a, err := NewOpenAIAdapter(&adapter.AdapterConfig{APIKey: "test-key"})
	if err != nil {
		t.Fatalf("NewOpenAIAdapter() error = %v", err)
	}
	if _, err := a.GetBalance(context.Background(), "runtime-key"); !errors.Is(err, adapter.ErrCapabilityUnsupported) {
		t.Fatalf("GetBalance() error = %v, want ErrCapabilityUnsupported", err)
	}
}

func TestCohereAdapterGetBalance_IsUnsupported(t *testing.T) {
	a, err := NewCohereAdapter(&adapter.AdapterConfig{APIKey: "test-key"})
	if err != nil {
		t.Fatalf("NewCohereAdapter() error = %v", err)
	}
	if _, err := a.GetBalance(context.Background(), "runtime-key"); !errors.Is(err, adapter.ErrCapabilityUnsupported) {
		t.Fatalf("GetBalance() error = %v, want ErrCapabilityUnsupported", err)
	}
}

func TestDeepSeekAdapterHandleError_MapsOfficialPaymentRequired(t *testing.T) {
	a := &DeepSeekAdapter{}
	err := a.handleError(http.StatusPaymentRequired, []byte(`{"error":{"message":"Insufficient Balance"}}`))
	if !errors.Is(err, adapter.ErrInsufficientBalance) {
		t.Fatalf("handleError() error = %v, want ErrInsufficientBalance", err)
	}
	var adapterErr *adapter.AdapterError
	if !errors.As(err, &adapterErr) || adapterErr.StatusCode != http.StatusPaymentRequired {
		t.Fatalf("handleError() error = %#v, want AdapterError status 402", err)
	}
}

func TestOpenRouterAdapterHandleError_AcceptsNumericOfficialCodes(t *testing.T) {
	a := &OpenRouterAdapter{}
	err := a.handleError(http.StatusPaymentRequired, []byte(`{"error":{"code":402,"message":"Insufficient credits"}}`))
	if !errors.Is(err, adapter.ErrInsufficientBalance) {
		t.Fatalf("handleError() error = %v, want ErrInsufficientBalance", err)
	}
	var adapterErr *adapter.AdapterError
	if !errors.As(err, &adapterErr) || adapterErr.Code != "402" {
		t.Fatalf("handleError() error = %#v, want numeric code preserved", err)
	}
}
