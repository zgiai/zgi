package adapter

import (
	"context"
	"net"
	"net/http"
	"net/url"
	"strings"
	"testing"

	"github.com/zgiai/zgi/api/internal/modules/llm/internal/urlguard"
)

type fakeURLResolver map[string][]net.IPAddr

func (r fakeURLResolver) LookupIPAddr(_ context.Context, host string) ([]net.IPAddr, error) {
	return r[host], nil
}

func TestHTTPClientRejectsDomainResolvingToPrivateIP(t *testing.T) {
	client := NewHTTPClientWithOptions(0, 1, HTTPClientOptions{
		GuardOutboundURL: true,
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

func TestHTTPClientStreamRejectsDomainResolvingToPrivateIP(t *testing.T) {
	client := NewHTTPClientWithOptions(0, 1, HTTPClientOptions{
		GuardOutboundURL: true,
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
