package github

import (
	"context"
	"crypto"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"crypto/x509"
	"encoding/base64"
	"encoding/json"
	"encoding/pem"
	"github.com/zgiai/zgi/api/internal/modules/connectors/providers"
	"io"
	"net/http"
	"os"
	"reflect"
	"strings"
	"testing"
	"time"
)

func installationFixture(t *testing.T) (string, *rsa.PrivateKey) {
	t.Helper()
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}
	data, _ := json.Marshal(map[string]any{"type": "github_app_installation_v1", "app_id": "42", "installation_id": 7, "repository_ids": []int{11}, "private_key": string(pem.EncodeToMemory(&pem.Block{Type: "RSA PRIVATE KEY", Bytes: x509.MarshalPKCS1PrivateKey(key)}))})
	return string(data), key
}

type installationServer struct {
	repositories                   string
	repositoryIDs                  []int
	requestID                      string
	echo                           string
	t                              *testing.T
	key                            *rsa.PrivateKey
	calls                          []string
	issues                         string
	wrongApp, suspended, failIssue bool
	expiry                         string
	permissions                    map[string]string
}

func (s *installationServer) RoundTrip(r *http.Request) (*http.Response, error) {
	s.calls = append(s.calls, r.Method+" "+r.URL.Path)
	if r.URL.Scheme != "https" || r.URL.Host != "api.github.com" {
		s.t.Fatal("unsafe origin")
	}
	status, body := 200, `{}`
	auth := r.Header.Get("Authorization")
	if strings.HasPrefix(r.URL.Path, "/app") {
		parts := strings.Split(strings.TrimPrefix(auth, "Bearer "), ".")
		if len(parts) != 3 {
			s.t.Fatal("missing JWT")
		}
		digest := sha256.Sum256([]byte(parts[0] + "." + parts[1]))
		sig, _ := base64.RawURLEncoding.DecodeString(parts[2])
		if rsa.VerifyPKCS1v15(&s.key.PublicKey, crypto.SHA256, digest[:], sig) != nil {
			s.t.Fatal("invalid JWT")
		}
		payload, _ := base64.RawURLEncoding.DecodeString(parts[1])
		var claims map[string]any
		_ = json.Unmarshal(payload, &claims)
		if claims["iss"] != "42" || claims["exp"].(float64)-claims["iat"].(float64) > 600 || claims["exp"].(float64) < float64(time.Now().Unix()) {
			s.t.Fatal("unsafe JWT claims")
		}
	} else if auth != "Bearer installation-secret" {
		s.t.Fatal("missing installation token")
	}
	switch r.Method + " " + r.URL.Path {
	case "GET /app":
		body = `{"id":42,"name":"Fixture App"}`
		if s.wrongApp {
			body = `{"id":99}`
		}
	case "GET /app/installations/7":
		body = `{"id":7,"app_id":42,"account":{"login":"owner"},"permissions":{"metadata":"read","issues":"` + s.issues + `"},"suspended_at":null}`
		if s.suspended {
			body = strings.Replace(body, `"suspended_at":null`, `"suspended_at":"2026-01-01T00:00:00Z"`, 1)
		}
	case "POST /app/installations/7/access_tokens":
		var payload struct {
			RepositoryIDs []int             `json:"repository_ids"`
			Permissions   map[string]string `json:"permissions"`
		}
		_ = json.NewDecoder(r.Body).Decode(&payload)
		expected := map[string]string{"metadata": "read"}
		if s.issues == "read" || s.issues == "write" {
			expected["issues"] = "read"
		}
		ids := s.repositoryIDs
		if ids == nil {
			ids = []int{11}
		}
		if !reflect.DeepEqual(payload.RepositoryIDs, ids) || !reflect.DeepEqual(payload.Permissions, expected) {
			s.t.Fatalf("unrestricted mint: %+v", payload)
		}
		expiry := s.expiry
		if expiry == "" {
			expiry = time.Now().Add(time.Hour).Format(time.RFC3339)
		}
		permissions := expected
		if s.permissions != nil {
			permissions = s.permissions
		}
		encoded, _ := json.Marshal(permissions)
		body = `{"expires_at":"` + expiry + `","token":"installation-secret","permissions":` + string(encoded) + `}`
	case "GET /installation/repositories":
		body = `{"total_count":1,"repositories":[{"id":11,"full_name":"o/r","html_url":"https://github.com/o/r","private":true}]}`
		if s.repositories != "" {
			body = s.repositories
		}
	case "GET /repos/o/r/issues":
		body = `[]`
		if s.failIssue {
			status = 503
			body = `{"message":"installation-secret"}`
		}
	case "GET /repos/o/r/issues/2":
		body = `{"number":2,"title":"Issue","state":"open"}`
		if s.echo != "" {
			data, _ := json.Marshal(map[string]any{"number": 2, "title": s.echo, "state": "open"})
			body = string(data)
		}
	case "GET /repos/o/r/issues/2/comments":
		body = `[]`
	case "DELETE /installation/token":
		status = 204
		body = ""
	default:
		s.t.Errorf("unexpected endpoint %s", r.URL.Path)
	}
	return &http.Response{StatusCode: status, Header: http.Header{"X-Github-Request-Id": []string{s.requestID}}, Body: io.NopCloser(strings.NewReader(body))}, nil
}
func TestInstallationVerifyAndRestrictedReads(t *testing.T) {
	credential, key := installationFixture(t)
	s := &installationServer{t: t, key: key, issues: "write"}
	c, _ := New(&http.Client{Transport: s})
	v, err := c.Verify(context.Background(), credential)
	if err != nil {
		t.Fatal(err)
	}
	if v.AccountID != "github-app-installation:42:7" || !strings.Contains(v.DisplayName, "installation") {
		t.Fatalf("ambiguous identity: %+v", v)
	}
	if !reflect.DeepEqual(v.Operations, []string{"github.repository.list", "github.issue.list", "github.issue.get", "github.issue.comment.list"}) {
		t.Fatalf("unsafe ops %v", v.Operations)
	}
	if s.calls[len(s.calls)-1] != "DELETE /installation/token" {
		t.Fatal("not revoked")
	}
	for _, op := range []string{"github.repository.list", "github.issue.list", "github.issue.get", "github.issue.comment.list"} {
		s.calls = nil
		args := map[string]any{}
		if op != "github.repository.list" {
			args = map[string]any{"owner": "o", "repo": "r"}
			if op != "github.issue.list" {
				args["issue_number"] = 2
			}
		}
		out, err := c.Invoke(context.Background(), op, credential, args)
		if err != nil {
			t.Fatalf("%s: %v", op, err)
		}
		if op == "github.repository.list" && len(out.Value["repositories"].([]any)) != 1 {
			t.Fatal("normalization failed")
		}
		if s.calls[0] != "GET /app" || s.calls[len(s.calls)-1] != "DELETE /installation/token" {
			t.Fatalf("not fresh/revoked: %v", s.calls)
		}
	}
}
func TestInstallationRejectsDescriptorsBeforeNetwork(t *testing.T) {
	credential, _ := installationFixture(t)
	var base map[string]any
	_ = json.Unmarshal([]byte(credential), &base)
	cases := []string{`{"type":"github_app_installation_v1"}`, credential + "\n", credential + `{}`, strings.Replace(credential, `"app_id":"42"`, `"app_id":"42","app_id":"42"`, 1)}
	for _, change := range []map[string]any{{"type": "other"}, {"app_id": "-1"}, {"installation_id": 0}, {"repository_ids": []int{}}, {"repository_ids": []int{11, 11}}, {"repository_ids": []int{-1}}, {"private_key": "invalid"}, {"unexpected": true}} {
		copy := map[string]any{}
		for k, v := range base {
			copy[k] = v
		}
		for k, v := range change {
			copy[k] = v
		}
		data, _ := json.Marshal(copy)
		cases = append(cases, string(data))
	}
	for _, value := range cases {
		calls := 0
		c, _ := New(&http.Client{Transport: roundTrip(func(*http.Request) (*http.Response, error) {
			calls++
			return &http.Response{StatusCode: 401, Header: make(http.Header), Body: io.NopCloser(strings.NewReader(`{}`))}, nil
		})})
		_, err := c.Verify(context.Background(), value)
		if err == nil || calls != 0 {
			t.Fatalf("invalid descriptor reached provider: calls=%d", calls)
		}
	}
}
func TestInstallationScopePermissionsAndCleanup(t *testing.T) {
	credential, key := installationFixture(t)
	for _, tc := range []struct {
		name   string
		mutate func(*installationServer)
		op     string
		args   map[string]any
		minted bool
	}{
		{"app mismatch", func(s *installationServer) { s.wrongApp = true }, "github.repository.list", nil, false},
		{"suspended", func(s *installationServer) { s.suspended = true }, "github.repository.list", nil, false},
		{"expired", func(s *installationServer) { s.expiry = time.Now().Add(-time.Minute).Format(time.RFC3339) }, "github.repository.list", nil, true},
		{"overprivileged", func(s *installationServer) { s.permissions = map[string]string{"metadata": "read", "issues": "write"} }, "github.repository.list", nil, true},
		{"public outside", func(*installationServer) {}, "github.issue.list", map[string]any{"owner": "public", "repo": "outside"}, true},
		{"upstream failure", func(s *installationServer) { s.failIssue = true }, "github.issue.list", map[string]any{"owner": "o", "repo": "r"}, true},
		{"missing issue grant", func(s *installationServer) { s.issues = "" }, "github.issue.list", map[string]any{"owner": "o", "repo": "r"}, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			s := &installationServer{t: t, key: key, issues: "read"}
			tc.mutate(s)
			c, _ := New(&http.Client{Transport: s})
			_, err := c.Invoke(context.Background(), tc.op, credential, tc.args)
			if err == nil || strings.Contains(err.Error(), "installation-secret") {
				t.Fatal("unsafe result")
			}
			if (s.calls[len(s.calls)-1] == "DELETE /installation/token") != tc.minted {
				t.Fatalf("cleanup mismatch %v", s.calls)
			}
		})
	}
	for _, op := range []string{"github.user.get", "github.repository.search", "github.issue.create", "github.issue.comment.create"} {
		calls := 0
		c, _ := New(&http.Client{Transport: roundTrip(func(*http.Request) (*http.Response, error) { calls++; return nil, nil })})
		_, err := c.Invoke(context.Background(), op, credential, map[string]any{"owner": "o", "repo": "r", "query": "q", "title": "x", "body": "x", "issue_number": 1})
		if err == nil || calls != 0 {
			t.Fatalf("unsupported op %s", op)
		}
	}
	s := &installationServer{t: t, key: key}
	c, _ := New(&http.Client{Transport: s})
	v, err := c.Verify(context.Background(), credential)
	if err != nil || !reflect.DeepEqual(v.Operations, []string{"github.repository.list"}) {
		t.Fatalf("ungranted issues advertised %v %v", v.Operations, err)
	}
}
func TestInstallationLive(t *testing.T) {
	path := os.Getenv("ZGI_GITHUB_APP_CREDENTIAL_FILE")
	if path == "" {
		t.Skip("requires explicit private credential file")
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal("cannot read credential fixture")
	}
	defer clear(raw)
	c, _ := New(nil)
	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()
	if _, err = c.Verify(ctx, string(raw)); err != nil {
		t.Fatalf("verification: %v", err)
	}
	repos, err := c.Invoke(ctx, "github.repository.list", string(raw), nil)
	if err != nil {
		t.Fatalf("repository read: %v", err)
	}
	owner, repo := os.Getenv("ZGI_GITHUB_APP_OWNER"), os.Getenv("ZGI_GITHUB_APP_REPO")
	if owner == "" || repo == "" {
		t.Fatal("explicit repo required")
	}
	found := false
	for _, value := range repos.Value["repositories"].([]any) {
		if value.(map[string]any)["full_name"] == owner+"/"+repo {
			found = true
		}
	}
	if !found {
		t.Fatal("selected repo missing")
	}
	if _, err = c.Invoke(ctx, "github.issue.list", string(raw), map[string]any{"owner": owner, "repo": repo}); err != nil {
		t.Fatalf("issue read: %v", err)
	}
}

func TestInstallationRepositoryListFiltersSortsBeforePaging(t *testing.T) {
	credential, key := installationFixture(t)
	var descriptor map[string]any
	_ = json.Unmarshal([]byte(credential), &descriptor)
	descriptor["repository_ids"] = []int{11, 12, 13}
	encoded, _ := json.Marshal(descriptor)
	credential = string(encoded)
	s := &installationServer{t: t, key: key, issues: "read", repositoryIDs: []int{11, 12, 13}, repositories: `{"total_count":3,"repositories":[{"id":11,"full_name":"o/c","private":true},{"id":12,"full_name":"o/b","private":false},{"id":13,"full_name":"o/a","private":false}]}`}
	c, _ := New(&http.Client{Transport: s})
	out, err := c.Invoke(context.Background(), "github.repository.list", credential, map[string]any{"visibility": "public", "sort": "full_name", "direction": "asc", "per_page": 1, "page": 2})
	if err != nil {
		t.Fatal(err)
	}
	rows := out.Value["repositories"].([]any)
	if len(rows) != 1 || rows[0].(map[string]any)["full_name"] != "o/b" {
		t.Fatal("filter/sort before pagination failed")
	}
	s.calls = nil
	_, err = c.Invoke(context.Background(), "github.repository.list", credential, map[string]any{"affiliation": "collaborator"})
	if err == nil || len(s.calls) != 0 {
		t.Fatal("unsupported user affiliation submitted")
	}
}
func TestInstallationEphemeralRequestIDIsRedacted(t *testing.T) {
	credential, key := installationFixture(t)
	s := &installationServer{t: t, key: key, issues: "read", requestID: "installation-secret"}
	c, _ := New(&http.Client{Transport: s})
	out, err := c.Invoke(context.Background(), "github.issue.list", credential, map[string]any{"owner": "o", "repo": "r"})
	if err != nil {
		t.Fatal(err)
	}
	if out.RequestID != "" {
		t.Fatal("ephemeral token leaked in response request ID")
	}
	s.failIssue = true
	_, err = c.Invoke(context.Background(), "github.issue.list", credential, map[string]any{"owner": "o", "repo": "r"})
	if err == nil || strings.Contains(providers.FailureOf(err).RequestID, "installation-secret") {
		t.Fatal("ephemeral token leaked in failure request ID")
	}
}
func TestInstallationInvalidExpiryStillRevokesKnownToken(t *testing.T) {
	credential, key := installationFixture(t)
	s := &installationServer{t: t, key: key, issues: "read", expiry: "invalid-time"}
	c, _ := New(&http.Client{Transport: s})
	_, err := c.Verify(context.Background(), credential)
	if err == nil || s.calls[len(s.calls)-1] != "DELETE /installation/token" {
		t.Fatalf("known token not revoked: %v", s.calls)
	}
}

func TestInstallationRejectsAmbiguousDocumentFieldsAndPEMPreamble(t *testing.T) {
	credential, _ := installationFixture(t)
	for _, value := range []string{strings.Replace(credential, `"app_id":`, `"App_ID":`, 1), strings.Replace(credential, `"private_key":"`, `"private_key":"preamble\n`, 1)} {
		calls := 0
		c, _ := New(&http.Client{Transport: roundTrip(func(*http.Request) (*http.Response, error) {
			calls++
			return &http.Response{StatusCode: 401, Header: make(http.Header), Body: io.NopCloser(strings.NewReader(`{}`))}, nil
		})})
		_, err := c.Verify(context.Background(), value)
		if err == nil || calls != 0 {
			t.Fatal("ambiguous descriptor reached provider")
		}
	}
}
func TestInstallationRedactsSecretsBeforeOutputTruncation(t *testing.T) {
	credential, key := installationFixture(t)
	var document map[string]any
	_ = json.Unmarshal([]byte(credential), &document)
	s := &installationServer{t: t, key: key, issues: "read", echo: document["private_key"].(string)}
	c, _ := New(&http.Client{Transport: s})
	out, err := c.Invoke(context.Background(), "github.issue.get", credential, map[string]any{"owner": "o", "repo": "r", "issue_number": 2})
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(out.Value["issue"].(map[string]any)["title"].(string), "PRIVATE KEY") {
		t.Fatal("truncated private key leaked")
	}
}
