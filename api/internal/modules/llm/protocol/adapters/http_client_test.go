package adapter

import (
	"context"
	"errors"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/zgiai/zgi/api/internal/modules/llm/internal/urlguard"
)

func TestHTTPClientClassifiesOnlyTransportDeadline(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		time.Sleep(50 * time.Millisecond)
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	client := &HTTPClient{
		client:     &http.Client{Timeout: 5 * time.Millisecond},
		maxRetries: 0,
	}
	_, err := client.DoRequestDetailed(context.Background(), http.MethodGet, server.URL, nil, nil)
	if !errors.Is(err, ErrTimeout) {
		t.Fatalf("DoRequestDetailed() error = %v, want ErrTimeout", err)
	}
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("DoRequestDetailed() error = %v, want original deadline cause", err)
	}

	internalDeadline := context.DeadlineExceeded
	if errors.Is(internalDeadline, ErrTimeout) {
		t.Fatal("unclassified internal deadline unexpectedly matches ErrTimeout")
	}
}

func TestNormalizeTransportErrorClassifiesStructuredNetworkTimeout(t *testing.T) {
	transportTimeout := &net.DNSError{
		Err:       "TLS handshake timeout",
		Name:      "provider.example.test",
		IsTimeout: true,
	}

	normalized := NormalizeTransportError(transportTimeout)
	if !errors.Is(normalized, ErrTimeout) {
		t.Fatalf("NormalizeTransportError() = %v, want ErrTimeout", normalized)
	}
	if !errors.Is(normalized, transportTimeout) {
		t.Fatalf("NormalizeTransportError() = %v, want original network cause", normalized)
	}

	nonTimeout := &net.DNSError{Err: "no such host", Name: "provider.example.test"}
	if normalized := NormalizeTransportError(nonTimeout); errors.Is(normalized, ErrTimeout) {
		t.Fatalf("non-timeout error = %v, must not match ErrTimeout", normalized)
	}
}

func TestHTTPClientClassifiesUpstreamGatewayTimeout(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusGatewayTimeout)
		_, _ = w.Write([]byte(`{"error":"private upstream detail"}`))
	}))
	defer server.Close()

	client := &HTTPClient{
		client:     &http.Client{},
		maxRetries: 0,
	}
	_, err := client.DoRequestDetailed(context.Background(), http.MethodGet, server.URL, nil, nil)
	if !errors.Is(err, ErrTimeout) {
		t.Fatalf("DoRequestDetailed() error = %v, want ErrTimeout", err)
	}
}

func TestHTTPClientRawRequestStreamsBodyWithoutRetry(t *testing.T) {
	requestCount := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requestCount++
		if got := r.Header.Get("Content-Type"); got != "audio/pcm" {
			t.Errorf("Content-Type = %q, want audio/pcm", got)
		}
		body, err := io.ReadAll(r.Body)
		if err != nil {
			t.Errorf("read body: %v", err)
		}
		if got := string(body); got != "pcm-audio" {
			t.Errorf("body = %q, want pcm-audio", got)
		}
		w.WriteHeader(http.StatusServiceUnavailable)
	}))
	defer server.Close()

	client := NewHTTPClientWithOptions(time.Second, 3, HTTPClientOptions{})
	response, err := client.DoRawRequestDetailed(
		context.Background(),
		http.MethodPost,
		server.URL,
		map[string]string{"Content-Type": "audio/pcm"},
		strings.NewReader("pcm-audio"),
	)
	if err != nil {
		t.Fatalf("DoRawRequestDetailed() error = %v", err)
	}
	if response.StatusCode != http.StatusServiceUnavailable {
		t.Fatalf("status = %d, want %d", response.StatusCode, http.StatusServiceUnavailable)
	}
	if requestCount != 1 {
		t.Fatalf("request count = %d, want one non-retryable attempt", requestCount)
	}
}

type fakeURLResolver map[string][]net.IPAddr

func (r fakeURLResolver) LookupIPAddr(_ context.Context, host string) ([]net.IPAddr, error) {
	return r[host], nil
}

func TestHTTPClientRejectsDomainResolvingToPrivateIP(t *testing.T) {
	client := NewHTTPClientWithOptions(0, 1, HTTPClientOptions{
		GuardOutboundURL: true,
		GuardOutboundDNS: true,
		URLPolicy: urlguard.Policy{
			Resolver: fakeURLResolver{
				"evil.example.test": {{IP: net.ParseIP("127.0.0.1")}},
			},
		},
	})

	_, _, err := client.DoRequest(context.Background(), "GET", "http://evil.example.test/v1/models", nil, nil)
	if err == nil {
		t.Fatal("DoRequest error = nil, want private target rejection")
	}
	if !strings.Contains(err.Error(), "blocked unsafe target") {
		t.Fatalf("DoRequest error = %q, want unsafe target rejection", err.Error())
	}
}

func TestHTTPClientDoesNotRetryTerminalPlatformChannelError(t *testing.T) {
	requestCount := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		requestCount++
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadGateway)
		_, _ = w.Write([]byte(`{"error":{"message":"Platform model service is temporarily unavailable","type":"server_error","code":"platform_channel_unavailable"}}`))
	}))
	defer server.Close()

	client := NewHTTPClientWithOptions(time.Second, 3, HTTPClientOptions{})
	response, err := client.DoRequestDetailed(context.Background(), http.MethodPost, server.URL, nil, map[string]interface{}{})
	if err != nil {
		t.Fatalf("DoRequestDetailed() error = %v, want structured response", err)
	}
	if response.StatusCode != http.StatusBadGateway {
		t.Fatalf("status = %d, want %d", response.StatusCode, http.StatusBadGateway)
	}
	if requestCount != 1 {
		t.Fatalf("request count = %d, want 1 terminal attempt", requestCount)
	}
}

func TestHTTPClientDoesNotRetryBillingSettlementFailure(t *testing.T) {
	requestCount := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		requestCount++
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = w.Write([]byte(`{"error":{"message":"Response could not be finalized","type":"api_error","code":"billing_settlement_failed"}}`))
	}))
	defer server.Close()

	client := NewHTTPClientWithOptions(time.Second, 3, HTTPClientOptions{})
	response, err := client.DoRequestDetailed(context.Background(), http.MethodPost, server.URL, nil, map[string]interface{}{})
	if err != nil {
		t.Fatalf("DoRequestDetailed() error = %v, want structured terminal response", err)
	}
	if response.StatusCode != http.StatusInternalServerError {
		t.Fatalf("status = %d, want %d", response.StatusCode, http.StatusInternalServerError)
	}
	if requestCount != 1 {
		t.Fatalf("request count = %d, want 1 because settlement failure must not repeat a billable upstream call", requestCount)
	}
}

func TestHTTPClientDoesNotReplayCatalogedSettlementPending(t *testing.T) {
	for _, body := range []string{
		`{"error_code":"billing.settle.failed","retryable":true,"error":{"code":"settlement_pending","type":"api_error","message":"pending"}}`,
		`{"code":"billing.settle.failed","retryable":true,"error":{"code":"settlement_pending"}}`,
		`{"error":{"code":"billing.settle.failed","message":"pending"}}`,
	} {
		t.Run(body, func(t *testing.T) {
			requests := 0
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				requests++
				w.WriteHeader(http.StatusInternalServerError)
				_, _ = w.Write([]byte(body))
			}))
			defer server.Close()
			client := NewHTTPClientWithOptions(time.Second, 3, HTTPClientOptions{})
			response, err := client.DoRequestDetailed(context.Background(), http.MethodPost, server.URL, nil, map[string]interface{}{})
			if err != nil || response.StatusCode != http.StatusInternalServerError || requests != 1 {
				t.Fatalf("requests=%d response=%+v error=%v", requests, response, err)
			}
		})
	}
}

func TestHTTPClientStreamRejectsDomainResolvingToPrivateIP(t *testing.T) {
	client := NewHTTPClientWithOptions(0, 1, HTTPClientOptions{
		GuardOutboundURL: true,
		GuardOutboundDNS: true,
		URLPolicy: urlguard.Policy{
			Resolver: fakeURLResolver{
				"evil.example.test": {{IP: net.ParseIP("127.0.0.1")}},
			},
		},
	})

	_, err := client.DoStreamRequest(context.Background(), "GET", "http://evil.example.test/v1/chat/completions", nil, nil)
	if err == nil {
		t.Fatal("DoStreamRequest error = nil, want private target rejection")
	}
	if !strings.Contains(err.Error(), "blocked unsafe target") {
		t.Fatalf("DoStreamRequest error = %q, want unsafe target rejection", err.Error())
	}
}

func TestParseSSEAcceptsDataLineWithoutSpace(t *testing.T) {
	dataChan := make(chan string, 2)
	errChan := make(chan error, 1)

	ParseSSE(strings.NewReader("event:message\ndata:{\"ok\":true}\n\ndata:[DONE]\n\n"), dataChan, errChan)

	select {
	case err := <-errChan:
		t.Fatalf("ParseSSE error = %v", err)
	default:
	}

	data, ok := <-dataChan
	if !ok {
		t.Fatal("data channel closed before first event")
	}
	if data != `{"ok":true}` {
		t.Fatalf("data = %q, want JSON payload", data)
	}
	if _, ok := <-dataChan; ok {
		t.Fatal("data channel still open after [DONE]")
	}
}

func TestHTTPClientAllowsFakeIPDNSWhenDNSGuardDisabled(t *testing.T) {
	client := NewHTTPClientWithOptions(0, 1, HTTPClientOptions{
		GuardOutboundURL: true,
		URLPolicy: urlguard.Policy{
			Resolver: fakeURLResolver{
				"api.deepseek.com": {{IP: net.ParseIP("198.18.0.159")}},
			},
		},
	})
	target, err := url.Parse("https://api.deepseek.com/v1/models")
	if err != nil {
		t.Fatal(err)
	}

	if err := client.validateOutboundURL(context.Background(), target); err != nil {
		t.Fatalf("validateOutboundURL(fake-ip DNS) error = %v, want nil", err)
	}
}

func TestHTTPClientRejectsUnsafeLiteralBeforeDial(t *testing.T) {
	client := NewHTTPClientWithOptions(0, 1, HTTPClientOptions{GuardOutboundURL: true})
	tests := []string{
		"http://127.0.0.1:11434/v1/models",
		"http://169.254.169.254/latest/meta-data",
		"http://198.18.0.159/v1/models",
	}

	for _, target := range tests {
		_, _, err := client.DoRequest(context.Background(), "GET", target, nil, nil)
		if err == nil {
			t.Fatalf("DoRequest(%q) error = nil, want unsafe target rejection", target)
		}
		if !strings.Contains(err.Error(), "blocked unsafe target") {
			t.Fatalf("DoRequest(%q) error = %q, want unsafe target rejection", target, err.Error())
		}
	}
}

func TestHTTPClientSkipsUnsafeLiteralWhenURLGuardDisabled(t *testing.T) {
	client := NewHTTPClientWithOptions(0, 1, HTTPClientOptions{GuardOutboundURL: false})
	target, err := url.Parse("http://198.18.0.159/v1/models")
	if err != nil {
		t.Fatal(err)
	}

	if err := client.validateOutboundURL(context.Background(), target); err != nil {
		t.Fatalf("validateOutboundURL(URL guard disabled) error = %v, want nil", err)
	}
}

func TestHTTPClientRejectsRedirectToMetadataIP(t *testing.T) {
	client := NewHTTPClientWithOptions(0, 1, HTTPClientOptions{
		GuardOutboundURL: true,
	})
	redirectURL, err := url.Parse("http://169.254.169.254/latest/meta-data")
	if err != nil {
		t.Fatal(err)
	}

	err = client.client.CheckRedirect(&http.Request{URL: redirectURL}, nil)
	if err == nil {
		t.Fatal("CheckRedirect error = nil, want metadata target rejection")
	}
	if !strings.Contains(err.Error(), "blocked unsafe target") {
		t.Fatalf("CheckRedirect error = %q, want unsafe target rejection", err.Error())
	}
}

func TestParseSSEAcceptsDataLineWithSpace(t *testing.T) {
	dataChan := make(chan string, 2)
	errChan := make(chan error, 1)

	ParseSSE(strings.NewReader("data: {\"output\":{\"text\":\"ok\"}}\n\n"), dataChan, errChan)

	select {
	case err := <-errChan:
		if err != nil {
			t.Fatalf("ParseSSE() error = %v", err)
		}
	default:
	}

	got, ok := <-dataChan
	if !ok {
		t.Fatal("data channel closed without parsed event")
	}
	if got != "{\"output\":{\"text\":\"ok\"}}" {
		t.Fatalf("data = %q, want output text JSON", got)
	}
}

// A Console envelope that says retryable:false is terminal: repeating the call
// cannot succeed (here the upstream rejected the platform credential) and only
// multiplies upstream calls. A retryable 5xx keeps the existing retries.
func TestHTTPClientHonoursConsoleRetryableFalse(t *testing.T) {
	cases := []struct {
		name      string
		body      string
		wantCalls int
	}{
		{"retryable false is terminal", `{"code":"llm.provider.auth_failed","error_code":"llm.provider.auth_failed","retryable":false,"error":{"code":"upstream_auth_failed","type":"api_error","message":"x"}}`, 1},
		{"retryable true still retries", `{"code":"llm.invocation.failed","error_code":"llm.invocation.failed","retryable":true,"error":{"code":"upstream_error","type":"api_error","message":"x"}}`, 2},
		{"plain provider body still retries", `{"retryable":false,"error":{"code":"server_error","message":"x"}}`, 2},
	}
	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			requestCount := 0
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				requestCount++
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(http.StatusBadGateway)
				_, _ = w.Write([]byte(testCase.body))
			}))
			defer server.Close()

			client := NewHTTPClientWithOptions(time.Second, 1, HTTPClientOptions{})
			response, _ := client.DoRequestDetailed(context.Background(), http.MethodPost, server.URL, nil, map[string]interface{}{})
			if response == nil || response.StatusCode != http.StatusBadGateway {
				t.Fatalf("response = %#v, want the 502", response)
			}
			if requestCount != testCase.wantCalls {
				t.Fatalf("request count = %d, want %d", requestCount, testCase.wantCalls)
			}
		})
	}
}

func TestHTTPClientDoesNotRetryTimeout(t *testing.T) {
	cases := []struct {
		name   string
		status int
		body   string
	}{
		{"upstream 504", http.StatusGatewayTimeout, `{"error":{"message":"upstream timed out","type":"server_error"}}`},
		{"proxy HTML 504", http.StatusGatewayTimeout, `<html><body>504 Gateway Time-out</body></html>`},
		{"console llm.provider.timeout", http.StatusGatewayTimeout, `{"code":"llm.provider.timeout","error_code":"llm.provider.timeout","retryable":true,"error":{"code":"upstream_timeout","type":"api_error","message":"x"}}`},
		{"console llm.provider.timeout on 502", http.StatusBadGateway, `{"code":"llm.provider.timeout","error_code":"llm.provider.timeout","retryable":true,"error":{"code":"upstream_timeout","type":"api_error","message":"x"}}`},
	}
	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			requestCount := 0
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				requestCount++
				w.WriteHeader(testCase.status)
				_, _ = w.Write([]byte(testCase.body))
			}))
			defer server.Close()

			client := NewHTTPClientWithOptions(time.Second, 3, HTTPClientOptions{})
			_, err := client.DoRequestDetailed(context.Background(), http.MethodPost, server.URL, nil, map[string]interface{}{})
			if !errors.Is(err, ErrTimeout) {
				t.Fatalf("DoRequestDetailed() error = %v, want ErrTimeout", err)
			}
			if requestCount != 1 {
				t.Fatalf("request count = %d, want 1", requestCount)
			}
		})
	}
}

func TestHTTPClientDoesNotRetryClientTimeout(t *testing.T) {
	requestCount := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		requestCount++
		time.Sleep(100 * time.Millisecond)
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	client := NewHTTPClientWithOptions(20*time.Millisecond, 3, HTTPClientOptions{})
	_, err := client.DoRequestDetailed(context.Background(), http.MethodPost, server.URL, nil, map[string]interface{}{})
	if !errors.Is(err, ErrTimeout) {
		t.Fatalf("DoRequestDetailed() error = %v, want ErrTimeout", err)
	}
	if requestCount != 1 {
		t.Fatalf("request count = %d, want 1", requestCount)
	}
}

func TestHTTPClientStillRetriesNonTimeoutServerError(t *testing.T) {
	for _, status := range []int{http.StatusInternalServerError, http.StatusBadGateway} {
		t.Run(http.StatusText(status), func(t *testing.T) {
			requestCount := 0
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				requestCount++
				w.WriteHeader(status)
				_, _ = w.Write([]byte(`{"error":{"message":"x","type":"server_error"}}`))
			}))
			defer server.Close()

			client := NewHTTPClientWithOptions(time.Second, 1, HTTPClientOptions{})
			_, err := client.DoRequestDetailed(context.Background(), http.MethodPost, server.URL, nil, map[string]interface{}{})
			if err == nil || errors.Is(err, ErrTimeout) {
				t.Fatalf("DoRequestDetailed() error = %v, want non-timeout failure", err)
			}
			if requestCount != 2 {
				t.Fatalf("request count = %d, want 2", requestCount)
			}
		})
	}
}

func TestHTTPClientStillRetriesConnectionRefused(t *testing.T) {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	addr := listener.Addr().String()
	_ = listener.Close()

	dials := 0
	client := NewHTTPClientWithOptions(time.Second, 1, HTTPClientOptions{})
	client.client.Transport = &http.Transport{
		DialContext: func(ctx context.Context, network, address string) (net.Conn, error) {
			dials++
			return (&net.Dialer{}).DialContext(ctx, network, address)
		},
	}
	_, err = client.DoRequestDetailed(context.Background(), http.MethodPost, "http://"+addr, nil, map[string]interface{}{})
	if err == nil || errors.Is(err, ErrTimeout) {
		t.Fatalf("DoRequestDetailed() error = %v, want connection failure", err)
	}
	if dials != 2 {
		t.Fatalf("dial count = %d, want 2", dials)
	}
}

func TestHTTPClientSkipsRetryWithoutTimeForAnotherAttempt(t *testing.T) {
	requestCount := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		requestCount++
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = w.Write([]byte(`{"error":{"message":"x","type":"server_error"}}`))
	}))
	defer server.Close()

	// One retry needs 1s backoff plus a 1s attempt; 1.5s left cannot fit it.
	ctx, cancel := context.WithTimeout(context.Background(), 1500*time.Millisecond)
	defer cancel()
	client := NewHTTPClientWithOptions(time.Second, 3, HTTPClientOptions{})
	_, err := client.DoRequestDetailed(ctx, http.MethodPost, server.URL, nil, map[string]interface{}{})
	if err == nil || !strings.Contains(err.Error(), "after 0 retries") {
		t.Fatalf("DoRequestDetailed() error = %v, want the first failure without retries", err)
	}
	if requestCount != 1 {
		t.Fatalf("request count = %d, want 1", requestCount)
	}
}
