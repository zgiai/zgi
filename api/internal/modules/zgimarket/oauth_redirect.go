package zgimarket

import (
	"encoding/json"
	"net/http"
	"net/url"
	"regexp"
	"strings"
)

// oauthRedirectMember names the callback address of a connector authorization, both in the body
// that starts one and in the query of the callback that completes it.
const oauthRedirectMember = "redirect_url"

// reauthorizePattern is the path that starts a new authorization of an existing connection.
var reauthorizePattern = regexp.MustCompile(`^connections/[0-9]+/reauthorize$`)

// ValidOAuthRedirectURL reports whether raw can be the callback address of this instance at the
// market: an absolute https URL without credentials, query, or fragment. Plain http is accepted
// for localhost only, for local development. The market accepts nothing else either.
func ValidOAuthRedirectURL(raw string) bool {
	target, err := url.Parse(raw)
	if err != nil || raw != strings.TrimSpace(raw) || target.Hostname() == "" || target.User != nil ||
		strings.ContainsAny(raw, "?#") {
		return false
	}
	return target.Scheme == "https" || (target.Scheme == "http" && target.Hostname() == "localhost")
}

// startsAuthorization reports whether a request may start a connector authorization at a vendor.
func startsAuthorization(method, path string) bool {
	return method == http.MethodPost && (path == "connections" || reauthorizePattern.MatchString(path))
}

// nameOAuthRedirect makes this instance, never the browser, say where a connector vendor sends
// the browser back. The market keeps one callback address for each deployment in front of it and
// completes an authorization only at the address it was started with.
//
// A request that starts an authorization gets the configured address as redirect_url in its JSON
// body, and the callback gets it in its query. Whatever the browser put there is dropped, also
// when no address is configured: the market then uses its default. A body that is not a JSON
// object is handed on as it is, for the market to refuse.
func (p *Proxy) nameOAuthRedirect(method, path, rawQuery string, body []byte) (string, []byte) {
	address := p.config.OAuthRedirectURL
	if method == http.MethodGet && path == oauthCallbackPath {
		query, err := url.ParseQuery(rawQuery)
		if err != nil {
			return rawQuery, body
		}
		query.Del(oauthRedirectMember)
		if address != "" {
			query.Set(oauthRedirectMember, address)
		}
		return query.Encode(), body
	}
	if !startsAuthorization(method, path) {
		return rawQuery, body
	}
	members := map[string]json.RawMessage{}
	if len(strings.TrimSpace(string(body))) > 0 {
		if err := json.Unmarshal(body, &members); err != nil || members == nil {
			return rawQuery, body
		}
	}
	_, named := members[oauthRedirectMember]
	if !named && address == "" {
		return rawQuery, body
	}
	delete(members, oauthRedirectMember)
	if address != "" {
		encoded, err := json.Marshal(address)
		if err != nil {
			return rawQuery, body
		}
		members[oauthRedirectMember] = encoded
	}
	rewritten, err := json.Marshal(members)
	if err != nil {
		return rawQuery, body
	}
	return rawQuery, rewritten
}

// originOf is the scheme and host of an absolute URL, or empty when raw is not one.
func originOf(raw string) string {
	target, err := url.Parse(raw)
	if err != nil || target.Scheme == "" || target.Host == "" {
		return ""
	}
	return strings.ToLower(target.Scheme + "://" + target.Host)
}

// oauthFailureCodePattern is the shape of a market error code, such as CONNECTOR.OAUTH_STATE_INVALID.
var oauthFailureCodePattern = regexp.MustCompile(`^[A-Z][A-Z0-9_]*(\.[A-Z][A-Z0-9_]*)*$`)

// oauthFailureCode reads the market's error code from the page it wanted to open, and nothing
// that does not look like one.
func oauthFailureCode(location string) string {
	target, err := url.Parse(location)
	if err != nil {
		return ""
	}
	code := target.Query().Get("error_code")
	if len(code) > 80 || !oauthFailureCodePattern.MatchString(code) {
		return ""
	}
	return code
}
