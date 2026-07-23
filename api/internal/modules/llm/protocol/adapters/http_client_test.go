package adapter

import (
	"context"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/zgiai/zgi/api/internal/modules/llm/internal/urlguard"
)

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
