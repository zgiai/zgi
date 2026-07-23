package imageasset

import (
	"context"
	"net/http"
	"net/url"
	"testing"
)

func TestValidateGeneratedImageDownloadURLRejectsLocalhost(t *testing.T) {
	parsed, err := url.Parse("https://localhost/image.png")
	if err != nil {
		t.Fatalf("parse url: %v", err)
	}
	if err := validateGeneratedImageDownloadURL(context.Background(), parsed); err == nil {
		t.Fatal("expected localhost image URL to be rejected")
	}
}

func TestValidateGeneratedImageDownloadURLRejectsPrivateAddress(t *testing.T) {
	parsed, err := url.Parse("https://10.0.0.5/image.png")
	if err != nil {
		t.Fatalf("parse url: %v", err)
	}
	if err := validateGeneratedImageDownloadURL(context.Background(), parsed); err == nil {
		t.Fatal("expected private image URL to be rejected")
	}
}

func TestValidateGeneratedImageDownloadURLRejectsIPv4MappedPrivateAddress(t *testing.T) {
	parsed, err := url.Parse("https://[::ffff:10.0.0.5]/image.png")
	if err != nil {
		t.Fatalf("parse url: %v", err)
	}
	if err := validateGeneratedImageDownloadURL(context.Background(), parsed); err == nil {
		t.Fatal("expected IPv4-mapped private image URL to be rejected")
	}
}

func TestValidateGeneratedImageDownloadURLRejectsUnsupportedScheme(t *testing.T) {
	parsed, err := url.Parse("file:///tmp/image.png")
	if err != nil {
		t.Fatalf("parse url: %v", err)
	}
	if err := validateGeneratedImageDownloadURL(context.Background(), parsed); err == nil {
		t.Fatal("expected non-http image URL to be rejected")
	}
}

func TestValidateGeneratedImageDownloadURLRejectsSpecialUseAddresses(t *testing.T) {
	tests := []string{
		"https://100.64.0.1/image.png",
		"https://198.18.0.1/image.png",
		"https://[fd00::1]/image.png",
		"https://[2001:db8::1]/image.png",
	}
	for _, rawURL := range tests {
		t.Run(rawURL, func(t *testing.T) {
			parsed, err := url.Parse(rawURL)
			if err != nil {
				t.Fatalf("parse url: %v", err)
			}
			if err := validateGeneratedImageDownloadURL(context.Background(), parsed); err == nil {
				t.Fatalf("expected special-use image URL %q to be rejected", rawURL)
			}
		})
	}
}

func TestGeneratedImageDownloadClientRejectsUnsafeRedirect(t *testing.T) {
	req, err := http.NewRequest(http.MethodGet, "https://100.64.0.1/image.png", nil)
	if err != nil {
		t.Fatalf("create request: %v", err)
	}
	if err := generatedImageDownloadClient().CheckRedirect(req, nil); err == nil {
		t.Fatal("expected redirect to special-use address to be rejected")
	}
}

func TestGeneratedImageDownloadClientDoesNotUseEnvironmentProxy(t *testing.T) {
	transport, ok := generatedImageDownloadClient().Transport.(*http.Transport)
	if !ok {
		t.Fatal("expected generated image client to use an HTTP transport")
	}
	if transport.Proxy != nil {
		t.Fatal("expected generated image client to bypass environment proxies")
	}
}
