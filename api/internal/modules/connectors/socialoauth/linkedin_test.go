package socialoauth

import (
	"context"
	"net/http"
	"testing"
	"time"
)

func TestLinkedInCodeAndRefreshUseConfidentialClientAndFixedEndpoint(t *testing.T) {
	s, _, _ := fixture(t)
	s.opts.Providers["linkedin-social"] = ProviderConfig{ClientID: "linkedin-client", ClientSecret: "linkedin-secret", RedirectURL: "https://api.example/console/api/connectors/linkedin-social/oauth/callback", Scopes: []string{"openid", "profile", "r_organization_social"}}
	calls := 0
	s.opts.HTTPClient.Transport = roundTripFunc(func(r *http.Request) (*http.Response, error) {
		calls++
		if r.Method != "POST" || r.URL.String() != "https://www.linkedin.com/oauth/v2/accessToken" {
			t.Fatal("unexpected token endpoint")
		}
		if err := r.ParseForm(); err != nil {
			t.Fatal(err)
		}
		if r.Form.Get("client_id") != "linkedin-client" || r.Form.Get("client_secret") != "linkedin-secret" || r.Form.Get("code_verifier") != "" {
			t.Fatal("incorrect confidential client exchange")
		}
		if calls == 1 {
			if r.Form.Get("grant_type") != "authorization_code" || r.Form.Get("code") != "code" || r.Form.Get("redirect_uri") != s.opts.Providers["linkedin-social"].RedirectURL {
				t.Fatal("incorrect code exchange")
			}
			return response(`{"access_token":"li-access","expires_in":5184000,"refresh_token":"li-refresh","refresh_token_expires_in":31536000}`, 200), nil
		}
		if r.Form.Get("grant_type") != "refresh_token" || r.Form.Get("refresh_token") != "li-refresh" {
			t.Fatal("incorrect refresh exchange")
		}
		return response(`{"access_token":"li-renewed","expires_in":5184000,"refresh_token":"li-refresh","refresh_token_expires_in":26352000}`, 200), nil
	})
	grant, err := s.exchange(context.Background(), "linkedin-social", "code", "", false)
	if err != nil || grant.RefreshToken != "li-refresh" || grant.RefreshExpiresAt.Before(s.now().Add(364*24*time.Hour)) {
		t.Fatal("partner refresh token was not retained", err)
	}
	renewed, err := s.exchange(context.Background(), "linkedin-social", grant.RefreshToken, "", true)
	if err != nil || renewed.AccessToken != "li-renewed" || calls != 2 {
		t.Fatal("refresh failed", err)
	}
	if renewal("linkedin-social", tokenGrant{AccessToken: "nonpartner"}) != "reauthorize" {
		t.Fatal("nonpartner LinkedIn falsely marked renewable")
	}
}
