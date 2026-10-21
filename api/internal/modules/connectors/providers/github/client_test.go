package github

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/zgiai/zgi/api/internal/modules/connectors/providers"
)

type roundTrip func(*http.Request) (*http.Response, error)

func (run roundTrip) RoundTrip(r *http.Request) (*http.Response, error) { return run(r) }
func TestNativeGitHubOperations(t *testing.T) {
	issue := `{"number":2,"title":"Issue","state":"open","html_url":"https://github.com/o/r/issues/2","user":{"login":"u"},"labels":[{"name":"bug"}]}`
	comment := `{"id":9,"body":"Comment","html_url":"https://github.com/o/r/issues/2#issuecomment-9","user":{"login":"u"}}`
	repository := `{"full_name":"o/r","html_url":"https://github.com/o/r","private":true}`
	cases := []struct {
		op, method, path, response string
		input                      map[string]any
	}{
		{"github.user.get", "GET", "/user", `{"id":123,"login":"u"}`, nil},
		{"github.repository.list", "GET", "/user/repos", "[" + repository + "]", nil},
		{"github.repository.search", "GET", "/search/repositories", `{"items":[` + repository + `],"total_count":1}`, map[string]any{"query": "zgi"}},
		{"github.issue.list", "GET", "/repos/o/r/issues", "[" + issue + "]", map[string]any{"owner": "o", "repo": "r"}},
		{"github.issue.get", "GET", "/repos/o/r/issues/2", issue, map[string]any{"owner": "o", "repo": "r", "issue_number": 2}},
		{"github.issue.comment.list", "GET", "/repos/o/r/issues/2/comments", "[" + comment + "]", map[string]any{"owner": "o", "repo": "r", "issue_number": 2}},
		{"github.issue.create", "POST", "/repos/o/r/issues", issue, map[string]any{"owner": "o", "repo": "r", "title": "Issue"}},
		{"github.issue.comment.create", "POST", "/repos/o/r/issues/2/comments", comment, map[string]any{"owner": "o", "repo": "r", "issue_number": 2, "body": "Comment"}},
	}
	for _, tc := range cases {
		t.Run(tc.op, func(t *testing.T) {
			calls := 0
			client, err := New(&http.Client{Transport: roundTrip(func(r *http.Request) (*http.Response, error) {
				calls++
				if r.Method != tc.method || r.URL.Path != tc.path || r.Header.Get("Authorization") != "Bearer fixture-key" {
					t.Errorf("unexpected request %s %s", r.Method, r.URL.Path)
				}
				return &http.Response{StatusCode: 200, Header: http.Header{"X-Github-Request-Id": []string{"request-1"}}, Body: io.NopCloser(strings.NewReader(tc.response))}, nil
			})})
			if err != nil {
				t.Fatal(err)
			}
			result, err := client.Invoke(context.Background(), tc.op, "fixture-key", tc.input)
			if err != nil {
				t.Fatal(err)
			}
			if calls != 1 {
				t.Fatal("operation resubmitted")
			}
			schemas, _ := providers.NewSchemas("github")
			if err := schemas.Output(tc.op, result.Value); err != nil {
				data, _ := json.Marshal(result.Value)
				t.Fatalf("invalid output: %s", data)
			}
		})
	}
}
func TestNativeGitHubNoSubmissionForInvalidArgumentsAndNoRetryAfterFailure(t *testing.T) {
	calls := 0
	client, _ := New(&http.Client{Transport: roundTrip(func(*http.Request) (*http.Response, error) {
		calls++
		return &http.Response{StatusCode: 503, Header: make(http.Header), Body: io.NopCloser(strings.NewReader("upstream private fixture-key"))}, nil
	})})
	if _, err := client.Invoke(context.Background(), "github.issue.create", "fixture-key", map[string]any{"owner": "../bad", "repo": "r", "title": "x"}); err == nil || calls != 0 {
		t.Fatal("invalid input submitted")
	}
	_, err := client.Invoke(context.Background(), "github.issue.create", "fixture-key", map[string]any{"owner": "o", "repo": "r", "title": "x"})
	if err == nil || calls != 1 || !providers.FailureOf(err).Unknown || strings.Contains(err.Error(), "fixture-key") {
		t.Fatal("unsafe failure/retry")
	}
}
