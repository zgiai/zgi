package middleware

import (
	"net/url"
	"strings"
	"testing"
)

func TestSafeRequestLogPathRedactsOAuthCallbackSecrets(t *testing.T) {
	requestURL, err := url.Parse("/console/api/integrations/oauth/callback?code=one-time-code&state=csrf-secret&error=access_denied&error_description=private+provider+message&scope=mail.read")
	if err != nil {
		t.Fatalf("url.Parse() error = %v", err)
	}

	got := safeRequestLogPath(requestURL)
	for _, secret := range []string{"one-time-code", "csrf-secret", "access_denied", "private+provider+message", "mail.read"} {
		if strings.Contains(got, secret) {
			t.Fatalf("safeRequestLogPath() leaked %q in %q", secret, got)
		}
	}
	if !strings.Contains(got, "error=") || !strings.Contains(got, "error_description=") ||
		!strings.Contains(got, "code=") || !strings.Contains(got, "state=") {
		t.Fatalf("safeRequestLogPath() removed useful query structure: %q", got)
	}
}

func TestSafeRequestLogPathDropsMalformedQuery(t *testing.T) {
	got := safeRequestLogPath(&url.URL{Path: "/callback", RawQuery: "%zz"})
	if got != "/callback?redacted_query=true" {
		t.Fatalf("safeRequestLogPath() = %q", got)
	}
}
