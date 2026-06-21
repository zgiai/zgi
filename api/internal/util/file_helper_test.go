package util

import (
	"net/url"
	"strings"
	"testing"

	appconfig "github.com/zgiai/zgi/api/config"
)

func TestSignedParserImageKeyURLVerifies(t *testing.T) {
	restoreParserImageTestConfig(t)

	const key = "mineru/images/doc/figure.png"
	signedURL := GetSignedParserImageKeyURL(key)
	parsed, err := url.Parse(signedURL)
	if err != nil {
		t.Fatalf("parse signed url: %v", err)
	}
	query := parsed.Query()
	if got := parsed.Path; got != parserImageEndpoint {
		t.Fatalf("path = %q, want %q", got, parserImageEndpoint)
	}
	if got := query.Get("key"); got != key {
		t.Fatalf("key = %q, want %q", got, key)
	}
	if query.Get("timestamp") == "" || query.Get("nonce") == "" || query.Get("sign") == "" {
		t.Fatalf("signed parser image url missing signature params: %q", signedURL)
	}
	if !VerifyParserImageSignature("key", key, query.Get("timestamp"), query.Get("nonce"), query.Get("sign")) {
		t.Fatalf("expected generated signature to verify")
	}
	if VerifyParserImageSignature("key", "mineru/images/doc/other.png", query.Get("timestamp"), query.Get("nonce"), query.Get("sign")) {
		t.Fatalf("signature verified for tampered key")
	}
}

func TestSignedParserImageURLFallsBackWhenConfigMissing(t *testing.T) {
	previous := appconfig.GlobalConfig
	appconfig.GlobalConfig = nil
	t.Cleanup(func() {
		appconfig.GlobalConfig = previous
	})

	signedURL := GetSignedParserImagePathURL("/tmp/hyperparse/mineru/images/doc/figure.png")

	if !strings.HasPrefix(signedURL, parserImageEndpoint+"?path=") {
		t.Fatalf("url = %q, want legacy parser image endpoint", signedURL)
	}
	if strings.Contains(signedURL, "sign=") {
		t.Fatalf("url = %q, should not include signature without config", signedURL)
	}
}

func restoreParserImageTestConfig(t *testing.T) {
	t.Helper()

	previous := appconfig.GlobalConfig
	appconfig.GlobalConfig = &appconfig.Config{
		App: appconfig.AppConfig{
			SecretKey:          "test-secret",
			FilesAccessTimeout: 3600,
		},
	}
	t.Cleanup(func() {
		appconfig.GlobalConfig = previous
	})
}
