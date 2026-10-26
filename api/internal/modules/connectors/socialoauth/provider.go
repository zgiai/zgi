package socialoauth

import (
	"context"
	"encoding/json"
	"github.com/zgiai/zgi/api/internal/modules/connectors/core"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

// Only these deployment-controlled endpoints can receive client secrets.
func (s *Service) exchange(ctx context.Context, id, code, verifier string, refresh bool) (tokenGrant, error) {
	c, ok := s.opts.Providers[id]
	if !ok {
		return tokenGrant{}, fail(core.ErrorCodeConnectorUnavailable)
	}
	endpoint := "https://www.linkedin.com/oauth/v2/accessToken"
	form := url.Values{"client_id": {c.ClientID}, "client_secret": {c.ClientSecret}}
	if refresh {
		form.Set("grant_type", "refresh_token")
		form.Set("refresh_token", code)
	} else {
		form.Set("grant_type", "authorization_code")
		form.Set("code", code)
		form.Set("redirect_uri", c.RedirectURL)
	}
	switch id {
	case "github":
		endpoint = "https://github.com/login/oauth/access_token"
		if !refresh {
			form.Set("code_verifier", verifier)
		}
	case "x-social":
		endpoint = "https://api.x.com/2/oauth2/token"
		form.Del("client_secret")
		if !refresh {
			form.Set("code_verifier", verifier)
		}
	case "facebook-social":
		if refresh {
			return tokenGrant{}, fail(core.ErrorCodeInvalidKey)
		}
		endpoint = "https://graph.facebook.com/" + c.APIVersion + "/oauth/access_token"
	}
	var g tokenGrant
	if e := s.request(ctx, http.MethodPost, endpoint, form, "", id == "x-social", c, &g); e != nil {
		return g, e
	}
	if id == "facebook-social" {
		longForm := url.Values{"grant_type": {"fb_exchange_token"}, "client_id": {c.ClientID}, "client_secret": {c.ClientSecret}, "fb_exchange_token": {g.AccessToken}}
		if e := s.request(ctx, http.MethodPost, endpoint, longForm, "", false, c, &g); e != nil {
			return tokenGrant{}, e
		}
		g.RefreshToken = ""
	}
	if g.AccessToken == "" || len(g.AccessToken) > 65536 || len(g.RefreshToken) > 65536 || g.ExpiresIn < 0 || g.ExpiresIn > 10*365*24*3600 || g.RefreshExpiresIn < 0 || g.RefreshExpiresIn > 10*365*24*3600 {
		return tokenGrant{}, fail(core.ErrorCodeInvalidKey)
	}
	if g.ExpiresIn > 0 {
		g.ExpiresAt = s.now().Add(time.Duration(g.ExpiresIn) * time.Second)
	}
	if g.RefreshExpiresIn > 0 {
		g.RefreshExpiresAt = s.now().Add(time.Duration(g.RefreshExpiresIn) * time.Second)
	}
	return g, nil
}
func (s *Service) request(ctx context.Context, method, endpoint string, form url.Values, token string, basic bool, c ProviderConfig, out any) error {
	var body io.Reader
	if form != nil {
		body = strings.NewReader(form.Encode())
	}
	req, e := http.NewRequestWithContext(ctx, method, endpoint, body)
	if e != nil {
		return fail(core.ErrorCodeInternal)
	}
	req.Header.Set("Accept", "application/json")
	if form != nil {
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	}
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	if basic {
		req.SetBasicAuth(c.ClientID, c.ClientSecret)
	}
	client := *s.opts.HTTPClient
	client.CheckRedirect = func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }
	if client.Timeout == 0 {
		client.Timeout = 20 * time.Second
	}
	resp, e := client.Do(req)
	if e != nil {
		return fail(core.ErrorCodeProviderUnavailable)
	}
	defer resp.Body.Close()
	if resp.StatusCode == http.StatusTooManyRequests || resp.StatusCode >= 500 {
		return fail(core.ErrorCodeProviderUnavailable)
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fail(core.ErrorCodeInvalidKey)
	}
	b, e := io.ReadAll(io.LimitReader(resp.Body, 1024*1024+1))
	if e != nil || len(b) > 1024*1024 {
		return fail(core.ErrorCodeProviderUnavailable)
	}
	defer clear(b)
	if json.Unmarshal(b, out) != nil {
		return fail(core.ErrorCodeProviderUnavailable)
	}
	return nil
}
func (s *Service) facebookPages(ctx context.Context, token string) ([]secretPage, error) {
	base := "https://graph.facebook.com/" + s.opts.Providers["facebook-social"].APIVersion + "/me/accounts?fields=id,name,access_token&limit=100"
	endpoint := base
	var result []secretPage
	for page := 0; page < 10; page++ {
		var response struct {
			Data []struct {
				ID          string `json:"id"`
				Name        string `json:"name"`
				AccessToken string `json:"access_token"`
			} `json:"data"`
			Paging struct {
				Next    string `json:"next"`
				Cursors struct {
					After string `json:"after"`
				} `json:"cursors"`
			} `json:"paging"`
		}
		if e := s.request(ctx, http.MethodGet, endpoint, nil, token, false, ProviderConfig{}, &response); e != nil {
			return nil, e
		}
		for _, p := range response.Data {
			if p.ID != "" && len(p.ID) <= 255 && len(p.Name) <= 255 && p.AccessToken != "" && len(p.AccessToken) <= 65536 {
				result = append(result, secretPage{Page: Page{ID: p.ID, Name: p.Name}, Token: p.AccessToken})
			}
		}
		if response.Paging.Next == "" {
			return result, nil
		}
		if response.Paging.Cursors.After == "" {
			return nil, fail(core.ErrorCodeProviderUnavailable)
		}
		endpoint = base + "&after=" + url.QueryEscape(response.Paging.Cursors.After)
	}
	return nil, fail(core.ErrorCodeProviderUnavailable)
}
