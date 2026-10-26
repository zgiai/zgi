package github

import (
	"bytes"
	"context"
	"crypto"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"crypto/x509"
	"encoding/base64"
	"encoding/json"
	"encoding/pem"
	"net/http"
	"net/url"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/zgiai/zgi/api/internal/modules/connectors/core"
	"github.com/zgiai/zgi/api/internal/modules/connectors/providers"
)

const installationCredentialType = "github_app_installation_v1"

type installationCredential struct {
	Type           string  `json:"type"`
	AppID          string  `json:"app_id"`
	InstallationID int64   `json:"installation_id"`
	RepositoryIDs  []int64 `json:"repository_ids"`
	PrivateKey     string  `json:"private_key"`
	key            *rsa.PrivateKey
}

// JSON-shaped credentials are never retried as personal access tokens.
func parseInstallation(value string) (*installationCredential, error) {
	trimmed := strings.TrimSpace(value)
	if !strings.HasPrefix(trimmed, "{") && !strings.HasPrefix(trimmed, "[") {
		return nil, nil
	}
	if len(value) > 16384 || strings.ContainsAny(value, "\r\n\x00") || !providers.ValidJSONObject([]byte(value)) {
		return nil, providers.Invalid()
	}
	var fields map[string]json.RawMessage
	if json.Unmarshal([]byte(value), &fields) != nil || len(fields) != 5 {
		return nil, providers.Invalid()
	}
	for _, name := range []string{"type", "app_id", "installation_id", "repository_ids", "private_key"} {
		if _, ok := fields[name]; !ok {
			return nil, providers.Invalid()
		}
	}
	var credential installationCredential
	decoder := json.NewDecoder(strings.NewReader(value))
	decoder.DisallowUnknownFields()
	if decoder.Decode(&credential) != nil || credential.Type != installationCredentialType || credential.InstallationID < 1 || len(credential.RepositoryIDs) == 0 || len(credential.RepositoryIDs) > 500 {
		return nil, providers.Invalid()
	}
	app, err := strconv.ParseInt(credential.AppID, 10, 64)
	if err != nil || app < 1 || strconv.FormatInt(app, 10) != credential.AppID {
		return nil, providers.Invalid()
	}
	seen := map[int64]bool{}
	for _, id := range credential.RepositoryIDs {
		if id < 1 || seen[id] {
			return nil, providers.Invalid()
		}
		seen[id] = true
	}
	block, rest := pem.Decode([]byte(credential.PrivateKey))
	if !strings.HasPrefix(strings.TrimSpace(credential.PrivateKey), "-----BEGIN ") || block == nil || len(bytes.TrimSpace(rest)) != 0 || len(block.Headers) != 0 {
		return nil, providers.Invalid()
	}
	switch block.Type {
	case "RSA PRIVATE KEY":
		credential.key, err = x509.ParsePKCS1PrivateKey(block.Bytes)
	case "PRIVATE KEY":
		var key any
		key, err = x509.ParsePKCS8PrivateKey(block.Bytes)
		credential.key, _ = key.(*rsa.PrivateKey)
	default:
		return nil, providers.Invalid()
	}
	if err != nil || credential.key == nil || credential.key.N.BitLen() < 2048 || credential.key.Validate() != nil {
		return nil, providers.Invalid()
	}
	return &credential, nil
}
func (c *installationCredential) jwt() (string, error) {
	payload, _ := json.Marshal(map[string]any{"iss": c.AppID, "iat": time.Now().Add(-time.Minute).Unix(), "exp": time.Now().Add(8 * time.Minute).Unix()})
	unsigned := base64.RawURLEncoding.EncodeToString([]byte(`{"alg":"RS256","typ":"JWT"}`)) + "." + base64.RawURLEncoding.EncodeToString(payload)
	digest := sha256.Sum256([]byte(unsigned))
	signature, err := rsa.SignPKCS1v15(rand.Reader, c.key, crypto.SHA256, digest[:])
	if err != nil {
		return "", providers.Invalid()
	}
	return unsigned + "." + base64.RawURLEncoding.EncodeToString(signature), nil
}

type installationSession struct {
	credential        *installationCredential
	jwt, token, login string
	permissions       map[string]string
	repositories      []map[string]any
}

func (s *installationSession) operations() []string {
	ops := []string{"github.repository.list"}
	if s.permissions["issues"] == "read" {
		ops = append(ops, "github.issue.list", "github.issue.get", "github.issue.comment.list")
	}
	return ops
}
func installationAllowed(op string) bool {
	return op == "github.repository.list" || op == "github.issue.list" || op == "github.issue.get" || op == "github.issue.comment.list"
}
func insufficient() error { return &providers.Failure{Code: core.ErrorCodeInsufficientKey} }
func (s *installationSession) redact(value any) {
	for _, secret := range []string{s.credential.PrivateKey, s.jwt, s.token} {
		providers.Redact(value, secret)
	}
}
func (s *installationSession) safeError(err error) error {
	if err == nil {
		return nil
	}
	f := *providers.FailureOf(err)
	for _, secret := range []string{s.credential.PrivateKey, s.jwt, s.token} {
		f.RequestID = providers.SafeRequestID(f.RequestID, secret)
	}
	return &f
}

// Revocation uses an independent bounded context so caller cancellation cannot skip cleanup.
// The shared JSON transport treats GitHub's empty 204 body as unrecognized, so
// cleanup intentionally ignores its parse result after the DELETE is sent once.
func (c *Client) revoke(s *installationSession) {
	if s.token == "" {
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	var result json.RawMessage
	_, _ = c.http.JSON(ctx, http.MethodDelete, "/installation/token", nil, headers(s.token), nil, &result)
}
func (c *Client) openInstallation(ctx context.Context, s *installationSession, op string) error {
	jwt, err := s.credential.jwt()
	if err != nil {
		return err
	}
	s.jwt = jwt
	var app struct {
		ID int64 `json:"id"`
	}
	if _, err = c.http.JSON(ctx, http.MethodGet, "/app", nil, headers(jwt), nil, &app); err != nil {
		return err
	}
	if strconv.FormatInt(app.ID, 10) != s.credential.AppID {
		return insufficient()
	}
	path := "/app/installations/" + strconv.FormatInt(s.credential.InstallationID, 10)
	var installation struct {
		ID          int64             `json:"id"`
		AppID       int64             `json:"app_id"`
		SuspendedAt json.RawMessage   `json:"suspended_at"`
		Permissions map[string]string `json:"permissions"`
		Account     struct {
			Login string `json:"login"`
		} `json:"account"`
	}
	if _, err = c.http.JSON(ctx, http.MethodGet, path, nil, headers(jwt), nil, &installation); err != nil {
		return err
	}
	if installation.ID != s.credential.InstallationID || strconv.FormatInt(installation.AppID, 10) != s.credential.AppID || string(installation.SuspendedAt) != "null" || strings.TrimSpace(installation.Account.Login) == "" {
		return insufficient()
	}
	requested := map[string]string{"metadata": "read"}
	if p := installation.Permissions["issues"]; p == "read" || p == "write" {
		requested["issues"] = "read"
	}
	if op != "github.repository.list" && requested["issues"] != "read" {
		return insufficient()
	}
	var minted struct {
		Token       string            `json:"token"`
		ExpiresAt   string            `json:"expires_at"`
		Permissions map[string]string `json:"permissions"`
	}
	_, err = c.http.JSON(ctx, http.MethodPost, path+"/access_tokens", nil, headers(jwt), struct {
		RepositoryIDs []int64           `json:"repository_ids"`
		Permissions   map[string]string `json:"permissions"`
	}{s.credential.RepositoryIDs, requested}, &minted)
	s.token = minted.Token // Retain a known token for cleanup even when metadata is invalid.
	if err != nil {
		return err
	}
	expiry, expiryErr := time.Parse(time.RFC3339, minted.ExpiresAt)
	if expiryErr != nil {
		return badResponse()
	}
	if s.token == "" || len(s.token) > 4096 || strings.ContainsAny(s.token, "\r\n\x00") || !expiry.After(time.Now().Add(30*time.Second)) || expiry.After(time.Now().Add(65*time.Minute)) {
		return badResponse()
	}
	if len(minted.Permissions) != len(requested) {
		return insufficient()
	}
	for name, value := range minted.Permissions {
		if value != "read" || requested[name] != value {
			return insufficient()
		}
	}
	s.permissions = minted.Permissions
	s.login = installation.Account.Login
	allowed := map[int64]bool{}
	for _, id := range s.credential.RepositoryIDs {
		allowed[id] = true
	}
	seen := map[int64]bool{}
	for page := 1; page <= 5; page++ {
		var repos struct {
			Total        int               `json:"total_count"`
			Repositories []json.RawMessage `json:"repositories"`
		}
		_, err = c.http.JSON(ctx, http.MethodGet, "/installation/repositories", url.Values{"page": {strconv.Itoa(page)}, "per_page": {"100"}}, headers(s.token), nil, &repos)
		if err != nil {
			return err
		}
		if repos.Total < 1 || repos.Total > len(allowed) || len(repos.Repositories) > 100 {
			return insufficient()
		}
		for _, raw := range repos.Repositories {
			var identity struct {
				ID int64 `json:"id"`
			}
			var repo map[string]any
			if json.Unmarshal(raw, &identity) != nil || json.Unmarshal(raw, &repo) != nil {
				return badResponse()
			}
			id := identity.ID
			name := providers.Text(repo, "full_name")
			coordinates := strings.Split(name, "/")
			if !allowed[id] || seen[id] || len(coordinates) != 2 || coordinates[0] == "" || coordinates[1] == "" {
				return insufficient()
			}
			seen[id] = true
			s.redact(repo)
			s.repositories = append(s.repositories, repo)
		}
		if len(s.repositories) == repos.Total {
			return nil
		}
		if len(repos.Repositories) < 100 {
			return badResponse()
		}
	}
	return badResponse()
}
func (c *Client) verifyInstallation(ctx context.Context, credential *installationCredential) (result providers.Verification, err error) {
	s := &installationSession{credential: credential}
	defer func() { c.revoke(s); err = s.safeError(err) }()
	if err = c.openInstallation(ctx, s, "github.repository.list"); err != nil {
		return result, err
	}
	display := map[string]any{"value": "GitHub App installation · " + s.login}
	s.redact(display)
	return providers.Verification{AccountID: "github-app-installation:" + credential.AppID + ":" + strconv.FormatInt(credential.InstallationID, 10), DisplayName: providers.SafeText(display["value"].(string), 256), Operations: s.operations()}, nil
}
func (c *Client) invokeInstallation(ctx context.Context, op string, credential *installationCredential, input map[string]any) (result providers.Response, err error) {
	if !installationAllowed(op) {
		return result, providers.Invalid()
	}
	args, err := c.schemas.Input(op, input)
	if err != nil {
		return result, providers.Invalid()
	}
	if op == "github.repository.list" && providers.Text(args, "affiliation") != "owner" {
		return result, providers.Invalid()
	}
	owner, repo := providers.Text(args, "owner"), providers.Text(args, "repo")
	if owner == "." || owner == ".." || repo == "." || repo == ".." {
		return result, providers.Invalid()
	}
	if since := providers.Text(args, "since"); since != "" {
		if _, err = time.Parse(time.RFC3339, since); err != nil {
			return result, providers.Invalid()
		}
	}
	s := &installationSession{credential: credential}
	defer func() {
		c.revoke(s)
		s.redact(result.Value)
		for _, secret := range []string{credential.PrivateKey, s.jwt, s.token} {
			result.RequestID = providers.SafeRequestID(result.RequestID, secret)
		}
		err = s.safeError(err)
	}()
	if err = c.openInstallation(ctx, s, op); err != nil {
		return result, err
	}
	if op != "github.repository.list" {
		found := false
		for _, row := range s.repositories {
			if strings.EqualFold(providers.Text(row, "full_name"), owner+"/"+repo) {
				found = true
			}
		}
		if !found {
			return result, insufficient()
		}
		return c.invokeToken(ctx, op, s.token, args, credential.PrivateKey, s.jwt)
	}
	filtered := make([]map[string]any, 0, len(s.repositories))
	visibility := providers.Text(args, "visibility")
	for _, row := range s.repositories {
		if visibility == "all" || visibility == repository(row)["visibility"] {
			filtered = append(filtered, row)
		}
	}
	field := map[string]string{"full_name": "full_name", "created": "created_at", "updated": "updated_at", "pushed": "pushed_at"}[providers.Text(args, "sort")]
	ascending := providers.Text(args, "direction") == "asc"
	sort.SliceStable(filtered, func(i, j int) bool {
		a, b := providers.Text(filtered[i], field), providers.Text(filtered[j], field)
		if a == b {
			a, b = providers.Text(filtered[i], "full_name"), providers.Text(filtered[j], "full_name")
		}
		if ascending {
			return a < b
		}
		return a > b
	})
	page, limit := providers.Number(args, "page"), providers.Number(args, "per_page")
	items := []any{}
	start := (page - 1) * limit
	for index := start; index < len(filtered) && len(items) < limit; index++ {
		items = append(items, repository(filtered[index]))
	}
	output := map[string]any{"provider": "github", "request_id": "", "page": page, "repositories": items}
	s.redact(output)
	if c.schemas.Output(op, output) != nil {
		return result, badResponse()
	}
	return providers.Response{Value: output}, nil
}
