package imageasset

import (
	"context"
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
