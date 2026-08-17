package github

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/zgiai/zgi/api/internal/capabilities/toolgovernance"
	"github.com/zgiai/zgi/api/internal/modules/integrations"
	"github.com/zgiai/zgi/api/internal/modules/tools"
)

func TestProviderDefinitionAndActionSchemas(t *testing.T) {
	definition := ProviderDefinition()
	if definition.ID != IntegrationID || definition.DriverID != DriverID {
		t.Fatalf("provider identity = %q/%q", definition.ID, definition.DriverID)
	}
	if definition.NameI18n[integrations.LocaleSimplifiedChinese] != "GitHub" || definition.DescriptionI18n[integrations.LocaleSimplifiedChinese] == "" {
		t.Fatalf("provider localization = %#v / %#v", definition.NameI18n, definition.DescriptionI18n)
	}
	if definition.DocumentationURLI18n[integrations.LocaleSimplifiedChinese] != "https://docs.github.com/zh/rest" {
		t.Fatalf("localized documentation urls = %#v", definition.DocumentationURLI18n)
	}
	assertDeclaredLabelsLocalized(t, "tag", definition.Tags, definition.TagLabelsI18n)
	assertDeclaredLabelsLocalized(t, "category", definition.Categories, definition.CategoryLabelsI18n)
	if len(definition.AuthMethods) != 2 {
		t.Fatalf("auth method count = %d, want 2", len(definition.AuthMethods))
	}
	for _, method := range definition.AuthMethods {
		if method.Type != integrations.AuthMethodTypeAPIKey ||
			method.IdentityKind != integrations.AuthIdentityKindUser ||
			method.AcquisitionStrategy != integrations.AuthAcquisitionStrategyManualForm ||
			method.LifecycleStrategy != integrations.AuthLifecycleStrategyStatic ||
			method.RequestAuthStrategy != integrations.RequestAuthStrategyBearerHeader ||
			len(method.Fields) != 1 || method.Fields[0].Key != "token" || !method.Fields[0].Secret ||
			method.SetupGuide == nil || len(method.SetupGuide.Steps) != 5 ||
			!strings.HasPrefix(method.SetupGuide.ConsoleURL, "https://") ||
			!strings.HasPrefix(method.SetupGuide.DocumentationURL, "https://") {
			t.Fatalf("unexpected GitHub auth method: %#v", method)
		}
		if method.SetupGuide.Steps[0].Action != integrations.AuthSetupStepActionOpenConsole ||
			method.SetupGuide.Steps[2].Action != integrations.AuthSetupStepActionOpenDocumentation {
			t.Fatalf("GitHub setup step actions = %#v", method.SetupGuide.Steps)
		}
	}
	if !definition.HealthProbe.Supported || definition.HealthProbe.MayIncurCost {
		t.Fatalf("unexpected health probe contract: %#v", definition.HealthProbe)
	}
	adapter, err := New(nil)
	if err != nil {
		t.Fatalf("create adapter: %v", err)
	}
	registry := integrations.NewRegistry()
	if err := registry.Register(integrations.Registration{Definition: definition, Adapter: adapter, ConnectionTester: adapter, HealthProbe: adapter}); err != nil {
		t.Fatalf("register GitHub provider with account credentials: %v", err)
	}

	seen := map[string]bool{}
	for _, action := range definition.Actions {
		seen[action.ID] = true
		if action.NameI18n[integrations.LocaleSimplifiedChinese] == "" || action.DescriptionI18n[integrations.LocaleSimplifiedChinese] == "" {
			t.Errorf("action %s is missing simplified Chinese metadata", action.ID)
		}
		assertDeclaredLabelsLocalized(t, "action "+action.ID+" scope", action.RequiredScopes, action.ScopeLabelsI18n)
		if err := tools.ValidateJSONSchema(action.InputSchema); err != nil {
			t.Errorf("action %s input schema: %v", action.ID, err)
		}
		if err := tools.ValidateJSONSchema(action.OutputSchema); err != nil {
			t.Errorf("action %s output schema: %v", action.ID, err)
		}
		properties, _ := action.InputSchema["properties"].(map[string]interface{})
		for field, rawSchema := range properties {
			property, _ := rawSchema.(map[string]interface{})
			titles, _ := property["title_i18n"].(integrations.LocalizedText)
			if titles[integrations.LocaleEnglishUS] == "" || titles[integrations.LocaleSimplifiedChinese] == "" {
				t.Errorf("action %s input %s is missing localized title metadata: %#v", action.ID, field, property["title_i18n"])
			}
			if _, hasEnum := property["enum"]; hasEnum {
				labels, _ := property["enum_labels_i18n"].(map[string]map[string]string)
				if len(labels[integrations.LocaleEnglishUS]) == 0 || len(labels[integrations.LocaleSimplifiedChinese]) == 0 {
					t.Errorf("action %s input %s is missing localized enum metadata: %#v", action.ID, field, property["enum_labels_i18n"])
				}
			}
		}
		if !action.DataEgress || action.ExternalDestination != "api.github.com" {
			t.Errorf("action %s egress = %v/%q", action.ID, action.DataEgress, action.ExternalDestination)
		}
		if action.DefaultPolicy == nil || !action.DefaultPolicy.DataEgressAllowed {
			t.Errorf("action %s default policy = %#v", action.ID, action.DefaultPolicy)
		}
		if action.Effect == "read" && !action.DefaultPolicy.Enabled {
			t.Errorf("read action %s must be enabled by default", action.ID)
		}
		if action.Effect != "read" && (action.DefaultPolicy.Enabled || action.DefaultPolicy.ApprovalPolicy != "always_ask") {
			t.Errorf("write action %s must be disabled and always ask by default: %#v", action.ID, action.DefaultPolicy)
		}
		switch action.ID {
		case ActionCreateIssue:
			assertGitHubWriteAction(t, action, toolgovernance.EffectCreate)
		case ActionCreateIssueComment:
			assertGitHubWriteAction(t, action, toolgovernance.EffectExternalSend)
		default:
			if action.Effect != toolgovernance.EffectRead || action.RiskLevel != toolgovernance.RiskLevelLow ||
				!action.Idempotent || action.DefaultPolicy.ApprovalPolicy != toolgovernance.ApprovalPolicyNeverAsk {
				t.Errorf("read action %s governance = %#v", action.ID, action)
			}
		}
	}
	for _, id := range []string{
		ActionGetAuthenticatedUser, ActionListRepositories, ActionSearchRepositories, ActionListIssues,
		ActionGetIssue, ActionListIssueComments, ActionCreateIssue, ActionCreateIssueComment,
	} {
		if !seen[id] {
			t.Errorf("missing action %s", id)
		}
	}
}

func assertGitHubWriteAction(t *testing.T, action integrations.ActionDefinition, effect toolgovernance.Effect) {
	t.Helper()
	if action.Effect != effect || action.RiskLevel != toolgovernance.RiskLevelHigh || action.Idempotent ||
		action.DefaultPolicy == nil || action.DefaultPolicy.Enabled ||
		action.DefaultPolicy.ApprovalPolicy != toolgovernance.ApprovalPolicyAlwaysAsk ||
		len(action.SupportedCallers) != 1 || action.SupportedCallers[0] != tools.ToolInvokeFromAIChat ||
		len(action.RequiredScopes) != 1 || action.RequiredScopes[0] != "issues:write" {
		t.Errorf("write action %s governance = %#v", action.ID, action)
	}
}

func assertDeclaredLabelsLocalized(t *testing.T, kind string, values []string, labels integrations.LocalizedLabelMap) {
	t.Helper()
	for _, value := range values {
		localized := labels[value]
		if localized[integrations.LocaleEnglishUS] == "" || localized[integrations.LocaleSimplifiedChinese] == "" {
			t.Errorf("%s %q is missing en-US or zh-Hans labels: %#v", kind, value, localized)
		}
	}
}

func TestGetAuthenticatedUserUsesRequiredHeadersAndBoundsOutput(t *testing.T) {
	const token = "github_pat_test-secret"
	adapter := newTestAdapter(t, func(w http.ResponseWriter, request *http.Request) {
		if request.URL.Path != "/user" {
			t.Errorf("path = %q", request.URL.Path)
		}
		if got := request.Header.Get("Authorization"); got != "Bearer "+token {
			t.Errorf("authorization = %q", got)
		}
		if got := request.Header.Get("X-GitHub-Api-Version"); got != githubAPIVersion {
			t.Errorf("api version = %q", got)
		}
		if got := request.Header.Get("Accept"); got != "application/vnd.github+json" {
			t.Errorf("accept = %q", got)
		}
		w.Header().Set("X-GitHub-Request-Id", "request-id")
		writeJSON(t, w, http.StatusOK, map[string]interface{}{
			"login":    "octocat",
			"name":     strings.Repeat("x", 400),
			"html_url": "https://github.com/octocat#discarded",
			"company":  "GitHub",
			"location": "Internet",
		})
	})

	result, err := adapter.Execute(context.Background(), integrations.ActionRequest{
		IntegrationID: IntegrationID,
		ActionID:      ActionGetAuthenticatedUser,
		Connection:    githubConnection(token),
		Input:         map[string]interface{}{},
	})
	if err != nil {
		t.Fatalf("execute: %v", err)
	}
	if result.ProviderRequestID != "request-id" || result.AttemptCount != 1 || result.ResultCount != 1 {
		t.Fatalf("metadata = %#v", result)
	}
	user := result.Output["user"].(map[string]interface{})
	if len([]rune(user["name"].(string))) != 256 {
		t.Fatalf("name length = %d", len([]rune(user["name"].(string))))
	}
	if user["html_url"] != "https://github.com/octocat" {
		t.Fatalf("safe URL = %#v", user["html_url"])
	}
	encoded, _ := json.Marshal(result.Output)
	if strings.Contains(string(encoded), token) {
		t.Fatal("credential leaked into GitHub output")
	}
	assertGitHubOutputSchema(t, ActionGetAuthenticatedUser, result.Output)
}

func TestListRepositoriesBuildsBoundedRequestAndOutput(t *testing.T) {
	adapter := newTestAdapter(t, func(w http.ResponseWriter, request *http.Request) {
		if request.URL.Path != "/user/repos" {
			t.Errorf("path = %q", request.URL.Path)
		}
		want := map[string]string{
			"visibility": "private", "affiliation": "collaborator", "sort": "pushed",
			"direction": "asc", "per_page": "2", "page": "3",
		}
		for key, value := range want {
			if got := request.URL.Query().Get(key); got != value {
				t.Errorf("query %s = %q, want %q", key, got, value)
			}
		}
		w.Header().Set("X-GitHub-Request-Id", "repo-request")
		writeJSON(t, w, http.StatusOK, []map[string]interface{}{
			{"full_name": "zgi/one", "html_url": "https://github.com/zgi/one", "description": strings.Repeat("d", 1200), "private": true, "archived": false, "default_branch": "main", "language": "Go", "updated_at": "2026-07-22T01:02:03Z", "pushed_at": "2026-07-22T01:02:03Z", "open_issues_count": 2},
			{"full_name": "zgi/two", "html_url": "https://github.com/zgi/two", "visibility": "private", "private": true, "archived": false, "default_branch": "main", "language": "TypeScript", "updated_at": "", "pushed_at": "", "open_issues_count": -1},
			{"full_name": "zgi/ignored", "html_url": "https://github.com/zgi/ignored"},
		})
	})

	result, err := adapter.Execute(context.Background(), integrations.ActionRequest{
		IntegrationID: IntegrationID,
		ActionID:      ActionListRepositories,
		Connection:    githubConnection("github_pat_repo"),
		Input: map[string]interface{}{
			"visibility": "private", "affiliation": "collaborator", "sort": "pushed",
			"direction": "asc", "per_page": float64(2), "page": json.Number("3"),
		},
	})
	if err != nil {
		t.Fatalf("execute: %v", err)
	}
	repositories := result.Output["repositories"].([]interface{})
	if len(repositories) != 2 || result.ResultCount != 2 {
		t.Fatalf("repository count = %d/%d", len(repositories), result.ResultCount)
	}
	first := repositories[0].(map[string]interface{})
	if len([]rune(first["description"].(string))) != 1000 || first["visibility"] != "private" {
		t.Fatalf("bounded repository = %#v", first)
	}
	second := repositories[1].(map[string]interface{})
	if second["open_issues_count"] != 0 {
		t.Fatalf("negative issue count was retained: %#v", second)
	}
	assertGitHubOutputSchema(t, ActionListRepositories, result.Output)
}

func TestListIssuesFiltersPullRequestsAndForwardsFilters(t *testing.T) {
	var calls atomic.Int32
	adapter := newTestAdapter(t, func(w http.ResponseWriter, request *http.Request) {
		calls.Add(1)
		if request.URL.Path != "/repos/zgiai/zgi/issues" {
			t.Errorf("path = %q", request.URL.Path)
		}
		if request.URL.Query().Get("labels") != "bug,security" || request.URL.Query().Get("since") != "2026-07-01T00:00:00Z" {
			t.Errorf("query = %s", request.URL.RawQuery)
		}
		writeJSON(t, w, http.StatusOK, []map[string]interface{}{
			{"number": 9, "title": "Issue", "state": "open", "html_url": "https://github.com/zgiai/zgi/issues/9", "user": map[string]interface{}{"login": "alice"}, "labels": []map[string]interface{}{{"name": "bug"}}, "comments": 3, "created_at": "2026-07-01T00:00:00Z", "updated_at": "2026-07-02T00:00:00Z"},
			{"number": 10, "title": "Pull request", "state": "open", "html_url": "https://github.com/zgiai/zgi/pull/10", "user": map[string]interface{}{"login": "bob"}, "labels": []interface{}{}, "comments": 1, "created_at": "2026-07-01T00:00:00Z", "updated_at": "2026-07-02T00:00:00Z", "pull_request": map[string]interface{}{}},
		})
	})

	request := integrations.ActionRequest{
		IntegrationID: IntegrationID,
		ActionID:      ActionListIssues,
		Connection:    githubConnection("github_pat_issues"),
		Input: map[string]interface{}{
			"owner": "zgiai", "repo": "zgi", "labels": []interface{}{"bug", "security", "bug"},
			"since": "2026-07-01T00:00:00Z", "per_page": 20,
		},
	}
	result, err := adapter.Execute(context.Background(), request)
	if err != nil {
		t.Fatalf("execute: %v", err)
	}
	issues := result.Output["issues"].([]interface{})
	if len(issues) != 1 || issues[0].(map[string]interface{})["kind"] != "issue" {
		t.Fatalf("filtered issues = %#v", issues)
	}
	assertGitHubOutputSchema(t, ActionListIssues, result.Output)

	request.Input["include_pull_requests"] = true
	result, err = adapter.Execute(context.Background(), request)
	if err != nil {
		t.Fatalf("execute with pull requests: %v", err)
	}
	if got := len(result.Output["issues"].([]interface{})); got != 2 {
		t.Fatalf("issue count with pull requests = %d", got)
	}
	if calls.Load() != 2 {
		t.Fatalf("request count = %d", calls.Load())
	}
}

func TestSearchRepositoriesBuildsBoundedRequestAndOutput(t *testing.T) {
	var calls atomic.Int32
	adapter := newTestAdapter(t, func(w http.ResponseWriter, request *http.Request) {
		calls.Add(1)
		if request.Method != http.MethodGet || request.URL.Path != "/search/repositories" {
			t.Errorf("request = %s %s", request.Method, request.URL.Path)
		}
		if got := request.URL.Query().Get("q"); got != "language:go stars:>100" {
			t.Errorf("query = %q", got)
		}
		if got := request.URL.Query().Get("sort"); got != "stars" || request.URL.Query().Get("order") != "asc" || request.URL.Query().Get("page") != "2" || request.URL.Query().Get("per_page") != "1" {
			t.Errorf("search parameters = %s", request.URL.RawQuery)
		}
		writeJSON(t, w, http.StatusOK, map[string]interface{}{
			"total_count": 25, "incomplete_results": false,
			"items": []map[string]interface{}{
				{"full_name": "zgiai/zgi", "html_url": "https://github.com/zgiai/zgi", "description": strings.Repeat("d", 1200), "visibility": "public", "default_branch": "main", "language": "Go"},
				{"full_name": "ignored/repository", "html_url": "https://github.com/ignored/repository"},
			},
		})
	})
	result, err := adapter.Execute(context.Background(), integrations.ActionRequest{
		IntegrationID: IntegrationID, ActionID: ActionSearchRepositories,
		Connection: githubConnection("github_pat_search"),
		Input: map[string]interface{}{
			"query": " language:go stars:>100 ", "sort": "stars", "order": "asc", "page": 2, "per_page": 1,
		},
	})
	if err != nil {
		t.Fatalf("search repositories: %v", err)
	}
	repositories := result.Output["repositories"].([]interface{})
	if calls.Load() != 1 || len(repositories) != 1 || result.ResultCount != 1 {
		t.Fatalf("search result = %#v, calls = %d", result, calls.Load())
	}
	if got := len([]rune(repositories[0].(map[string]interface{})["description"].(string))); got != 1000 {
		t.Fatalf("description length = %d", got)
	}
	assertGitHubOutputSchema(t, ActionSearchRepositories, result.Output)
}

func TestGetIssueAndListCommentsUseBoundedEndpoints(t *testing.T) {
	adapter := newTestAdapter(t, func(w http.ResponseWriter, request *http.Request) {
		switch request.URL.Path {
		case "/repos/zgiai/zgi/issues/42":
			writeJSON(t, w, http.StatusOK, map[string]interface{}{
				"number": 42, "title": "Issue", "state": "open", "body": strings.Repeat("b", 22000),
				"html_url": "https://github.com/zgiai/zgi/issues/42", "user": map[string]interface{}{"login": "alice"},
				"labels": []map[string]interface{}{{"name": "bug"}}, "comments": 1, "locked": false,
				"created_at": "2026-08-01T00:00:00Z", "updated_at": "2026-08-01T01:00:00Z",
			})
		case "/repos/zgiai/zgi/issues/42/comments":
			if request.URL.Query().Get("page") != "3" || request.URL.Query().Get("per_page") != "1" || request.URL.Query().Get("since") != "2026-08-01T00:00:00Z" {
				t.Errorf("comment query = %s", request.URL.RawQuery)
			}
			writeJSON(t, w, http.StatusOK, []map[string]interface{}{
				{"id": 9, "body": strings.Repeat("c", 22000), "html_url": "https://github.com/zgiai/zgi/issues/42#issuecomment-9", "user": map[string]interface{}{"login": "bob"}, "created_at": "2026-08-01T02:00:00Z", "updated_at": "2026-08-01T02:00:00Z"},
				{"id": 10, "body": "ignored"},
			})
		default:
			t.Errorf("unexpected path = %s", request.URL.Path)
		}
	})
	coordinates := map[string]interface{}{"owner": "zgiai", "repo": "zgi", "issue_number": 42}
	issueResult, err := adapter.Execute(context.Background(), integrations.ActionRequest{
		IntegrationID: IntegrationID, ActionID: ActionGetIssue,
		Connection: githubConnection("github_pat_issue"), Input: coordinates,
	})
	if err != nil {
		t.Fatalf("get issue: %v", err)
	}
	issue := issueResult.Output["issue"].(map[string]interface{})
	if len([]rune(issue["body"].(string))) != 20000 {
		t.Fatalf("issue body was not bounded")
	}
	assertGitHubOutputSchema(t, ActionGetIssue, issueResult.Output)

	commentInput := map[string]interface{}{
		"owner": "zgiai", "repo": "zgi", "issue_number": 42,
		"page": 3, "per_page": 1, "since": "2026-08-01T00:00:00Z",
	}
	commentResult, err := adapter.Execute(context.Background(), integrations.ActionRequest{
		IntegrationID: IntegrationID, ActionID: ActionListIssueComments,
		Connection: githubConnection("github_pat_comments"), Input: commentInput,
	})
	if err != nil {
		t.Fatalf("list comments: %v", err)
	}
	comments := commentResult.Output["comments"].([]interface{})
	if len(comments) != 1 || len([]rune(comments[0].(map[string]interface{})["body"].(string))) != 20000 {
		t.Fatalf("bounded comments = %#v", comments)
	}
	assertGitHubOutputSchema(t, ActionListIssueComments, commentResult.Output)
}

func TestCreateIssueAndCommentUseSingleBoundedPOST(t *testing.T) {
	var calls atomic.Int32
	adapter := newTestAdapter(t, func(w http.ResponseWriter, request *http.Request) {
		calls.Add(1)
		if request.Method != http.MethodPost || request.Header.Get("Content-Type") != "application/json" {
			t.Errorf("write request = %s, content-type %q", request.Method, request.Header.Get("Content-Type"))
		}
		var payload map[string]interface{}
		if err := json.NewDecoder(request.Body).Decode(&payload); err != nil {
			t.Errorf("decode request: %v", err)
		}
		switch request.URL.Path {
		case "/repos/zgiai/zgi/issues":
			if payload["title"] != "Release regression" || len(payload["labels"].([]interface{})) != 2 || len(payload["assignees"].([]interface{})) != 1 {
				t.Errorf("issue payload = %#v", payload)
			}
			writeJSON(t, w, http.StatusCreated, map[string]interface{}{
				"number": 51, "title": strings.Repeat("t", 600), "state": "open", "body": strings.Repeat("b", 22000),
				"html_url": "https://github.com/zgiai/zgi/issues/51", "user": map[string]interface{}{"login": "octocat"},
				"labels": []map[string]interface{}{}, "comments": 0, "locked": false,
			})
		case "/repos/zgiai/zgi/issues/51/comments":
			if payload["body"] != "Confirmed in production" {
				t.Errorf("comment payload = %#v", payload)
			}
			writeJSON(t, w, http.StatusCreated, map[string]interface{}{
				"id": 901, "body": strings.Repeat("c", 22000), "html_url": "https://github.com/zgiai/zgi/issues/51#issuecomment-901",
				"user": map[string]interface{}{"login": "octocat"}, "created_at": "2026-08-01T00:00:00Z", "updated_at": "2026-08-01T00:00:00Z",
			})
		default:
			t.Errorf("unexpected write path = %s", request.URL.Path)
		}
	})
	issueResult, err := adapter.Execute(context.Background(), integrations.ActionRequest{
		IntegrationID: IntegrationID, ActionID: ActionCreateIssue, Connection: githubConnection("github_pat_write"),
		Input: map[string]interface{}{
			"owner": "zgiai", "repo": "zgi", "title": " Release regression ", "body": "Details",
			"labels": []interface{}{"bug", "urgent"}, "assignees": []interface{}{"octocat"}, "milestone": 7,
		},
	})
	if err != nil {
		t.Fatalf("create issue: %v", err)
	}
	if issueResult.AttemptCount != 1 || len([]rune(issueResult.Output["issue"].(map[string]interface{})["body"].(string))) != 20000 {
		t.Fatalf("created issue = %#v", issueResult)
	}
	assertGitHubOutputSchema(t, ActionCreateIssue, issueResult.Output)

	commentResult, err := adapter.Execute(context.Background(), integrations.ActionRequest{
		IntegrationID: IntegrationID, ActionID: ActionCreateIssueComment, Connection: githubConnection("github_pat_write"),
		Input: map[string]interface{}{"owner": "zgiai", "repo": "zgi", "issue_number": 51, "body": " Confirmed in production "},
	})
	if err != nil {
		t.Fatalf("create comment: %v", err)
	}
	if calls.Load() != 2 || commentResult.AttemptCount != 1 || len([]rune(commentResult.Output["comment"].(map[string]interface{})["body"].(string))) != 20000 {
		t.Fatalf("created comment = %#v, calls = %d", commentResult, calls.Load())
	}
	assertGitHubOutputSchema(t, ActionCreateIssueComment, commentResult.Output)
}

func TestNewActionsRejectInvalidInputBeforeExternalRequest(t *testing.T) {
	var calls atomic.Int32
	adapter := newTestAdapter(t, func(w http.ResponseWriter, _ *http.Request) {
		calls.Add(1)
		writeJSON(t, w, http.StatusOK, map[string]interface{}{})
	})
	tests := []struct {
		action string
		input  map[string]interface{}
	}{
		{ActionSearchRepositories, map[string]interface{}{"query": "  "}},
		{ActionGetIssue, map[string]interface{}{"owner": "bad/owner", "repo": "zgi", "issue_number": 1}},
		{ActionListIssueComments, map[string]interface{}{"owner": "zgiai", "repo": "zgi", "issue_number": 0}},
		{ActionCreateIssue, map[string]interface{}{"owner": "zgiai", "repo": "zgi", "title": "\t"}},
		{ActionCreateIssueComment, map[string]interface{}{"owner": "zgiai", "repo": "zgi", "issue_number": 1, "body": "\n"}},
	}
	for _, test := range tests {
		_, err := adapter.Execute(context.Background(), integrations.ActionRequest{
			IntegrationID: IntegrationID, ActionID: test.action,
			Connection: githubConnection("github_pat_invalid"), Input: test.input,
		})
		if err == nil || integrations.ErrorCode(err) != integrations.ErrorCodeInvalidInput {
			t.Errorf("action %s error = %v", test.action, err)
		}
	}
	if calls.Load() != 0 {
		t.Fatalf("invalid inputs sent %d external requests", calls.Load())
	}
}

func TestGitHubWritesNeverRetry(t *testing.T) {
	tests := []struct {
		action string
		input  map[string]interface{}
	}{
		{ActionCreateIssue, map[string]interface{}{"owner": "zgiai", "repo": "zgi", "title": "One attempt only"}},
		{ActionCreateIssueComment, map[string]interface{}{"owner": "zgiai", "repo": "zgi", "issue_number": 1, "body": "One attempt only"}},
	}
	for _, test := range tests {
		t.Run(test.action, func(t *testing.T) {
			var calls atomic.Int32
			adapter := newTestAdapter(t, func(w http.ResponseWriter, _ *http.Request) {
				calls.Add(1)
				w.Header().Set("Retry-After", "0")
				writeJSON(t, w, http.StatusServiceUnavailable, map[string]string{"message": "temporary"})
			})
			result, err := adapter.Execute(context.Background(), integrations.ActionRequest{
				IntegrationID: IntegrationID, ActionID: test.action,
				Connection: githubConnection("github_pat_no_retry"), Input: test.input,
			})
			if err == nil || integrations.ErrorCode(err) != integrations.ErrorCodeUpstream {
				t.Fatalf("write error = %v", err)
			}
			if calls.Load() != 1 || result == nil || result.AttemptCount != 1 {
				t.Fatalf("write attempts = server %d/result %#v", calls.Load(), result)
			}
		})
	}
}

func TestClientRetriesTransientErrorsAndMapsPermanentErrors(t *testing.T) {
	var attempts atomic.Int32
	adapter := newTestAdapter(t, func(w http.ResponseWriter, _ *http.Request) {
		if attempts.Add(1) == 1 {
			w.Header().Set("Retry-After", "0")
			writeJSON(t, w, http.StatusInternalServerError, map[string]string{"message": "do not expose this"})
			return
		}
		writeJSON(t, w, http.StatusOK, map[string]interface{}{"login": "octocat", "html_url": "https://github.com/octocat"})
	})
	result, err := adapter.Execute(context.Background(), integrations.ActionRequest{
		IntegrationID: IntegrationID, ActionID: ActionGetAuthenticatedUser,
		Connection: githubConnection("github_pat_retry"), Input: map[string]interface{}{},
	})
	if err != nil {
		t.Fatalf("execute after retry: %v", err)
	}
	if attempts.Load() != 2 || result.AttemptCount != 2 {
		t.Fatalf("attempts = server %d/result %d", attempts.Load(), result.AttemptCount)
	}

	for _, test := range []struct {
		status int
		code   string
	}{
		{http.StatusUnauthorized, integrations.ErrorCodeAuthInvalid},
		{http.StatusForbidden, integrations.ErrorCodeAccessDenied},
		{http.StatusNotFound, integrations.ErrorCodeAccessDenied},
		{http.StatusUnprocessableEntity, integrations.ErrorCodeInvalidInput},
	} {
		t.Run(http.StatusText(test.status), func(t *testing.T) {
			const secret = "github_pat_never-print"
			failing := newTestAdapter(t, func(w http.ResponseWriter, _ *http.Request) {
				writeJSON(t, w, test.status, map[string]string{"message": secret})
			})
			result, err := failing.Execute(context.Background(), integrations.ActionRequest{
				IntegrationID: IntegrationID, ActionID: ActionGetAuthenticatedUser,
				Connection: githubConnection(secret), Input: map[string]interface{}{},
			})
			if err == nil || integrations.ErrorCode(err) != test.code {
				t.Fatalf("error = %v, want code %s", err, test.code)
			}
			if strings.Contains(err.Error(), secret) {
				t.Fatal("credential leaked into error")
			}
			if result == nil || result.AttemptCount != 1 ||
				result.ResultCount != 0 || result.Output != nil ||
				result.ProviderDiagnostics.HTTPStatus != test.status {
				t.Fatalf("failure result = %#v", result)
			}
		})
	}
}

func TestClientRejectsRedirectWithoutLeakingAuthorization(t *testing.T) {
	const token = "github_pat_redirect-secret"
	var redirectTargetCalls atomic.Int32
	var redirectTargetAuthorization atomic.Value
	redirectTarget := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, request *http.Request) {
		redirectTargetCalls.Add(1)
		redirectTargetAuthorization.Store(request.Header.Get("Authorization"))
		writeJSON(t, w, http.StatusOK, map[string]interface{}{"login": "redirected"})
	}))
	defer redirectTarget.Close()

	var originCalls atomic.Int32
	origin := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, request *http.Request) {
		originCalls.Add(1)
		if got := request.Header.Get("Authorization"); got != "Bearer "+token {
			t.Errorf("origin authorization = %q", got)
		}
		http.Redirect(w, request, redirectTarget.URL+"/credential-target", http.StatusFound)
	}))
	defer origin.Close()

	callerClient := origin.Client()
	adapter, err := newForBaseURL(callerClient, origin.URL)
	if err != nil {
		t.Fatalf("create GitHub adapter: %v", err)
	}
	if callerClient.CheckRedirect != nil {
		t.Fatal("constructor mutated the caller-provided HTTP client")
	}

	result, err := adapter.Execute(context.Background(), integrations.ActionRequest{
		IntegrationID: IntegrationID,
		ActionID:      ActionGetAuthenticatedUser,
		Connection:    githubConnection(token),
		Input:         map[string]interface{}{},
	})
	if err == nil || integrations.ErrorCode(err) != integrations.ErrorCodeUpstream {
		t.Fatalf("redirect error = %v", err)
	}
	if originCalls.Load() != 1 {
		t.Fatalf("origin calls = %d, want 1", originCalls.Load())
	}
	if redirectTargetCalls.Load() != 0 {
		t.Fatalf("redirect target was called %d times", redirectTargetCalls.Load())
	}
	if value := redirectTargetAuthorization.Load(); value != nil && value.(string) != "" {
		t.Fatalf("authorization reached redirect target: %q", value)
	}
	if result == nil || result.AttemptCount != 1 ||
		result.ProviderDiagnostics.HTTPStatus != http.StatusFound {
		t.Fatalf("redirect result = %#v", result)
	}
}

func TestClientBackoffDeadlineReturnsTimeoutWithDiagnostics(t *testing.T) {
	var calls atomic.Int32
	adapter := newTestAdapter(t, func(w http.ResponseWriter, _ *http.Request) {
		calls.Add(1)
		w.Header().Set("X-GitHub-Request-Id", "retry-timeout-request")
		w.Header().Set("Retry-After", "5")
		writeJSON(t, w, http.StatusServiceUnavailable, map[string]string{"message": "temporary"})
	})
	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()

	result, err := adapter.Execute(ctx, integrations.ActionRequest{
		IntegrationID: IntegrationID,
		ActionID:      ActionGetAuthenticatedUser,
		Connection:    githubConnection("github_pat_timeout"),
		Input:         map[string]interface{}{},
	})
	if err == nil || integrations.ErrorCode(err) != integrations.ErrorCodeTimeout {
		t.Fatalf("backoff error = %v, want timeout", err)
	}
	if calls.Load() != 1 {
		t.Fatalf("calls = %d, want 1", calls.Load())
	}
	if result == nil || result.AttemptCount != 1 ||
		result.ProviderRequestID != "retry-timeout-request" ||
		result.ProviderDiagnostics.RequestID != "retry-timeout-request" ||
		result.ProviderDiagnostics.HTTPStatus != http.StatusServiceUnavailable ||
		result.ProviderDiagnostics.RetryAfterAt == nil {
		t.Fatalf("timeout result = %#v", result)
	}
}

func TestClientDetectsSecondaryRateLimitAndPreservesDiagnostics(t *testing.T) {
	var calls atomic.Int32
	adapter := newTestAdapter(t, func(w http.ResponseWriter, _ *http.Request) {
		call := calls.Add(1)
		w.Header().Set("X-GitHub-Request-Id", "secondary-"+strconv.Itoa(int(call)))
		w.Header().Set("Retry-After", "0")
		writeJSON(t, w, http.StatusForbidden, map[string]string{
			"message": "You have exceeded a secondary rate limit. Please wait a few minutes before you try again.",
		})
	})
	result, err := adapter.Execute(context.Background(), integrations.ActionRequest{
		IntegrationID: IntegrationID, ActionID: ActionGetAuthenticatedUser,
		Connection: githubConnection("github_pat_secondary"), Input: map[string]interface{}{},
	})
	if err == nil || integrations.ErrorCode(err) != integrations.ErrorCodeRateLimited {
		t.Fatalf("error = %v", err)
	}
	if calls.Load() != 3 || result == nil || result.AttemptCount != 3 ||
		result.ProviderRequestID != "secondary-3" ||
		result.ProviderDiagnostics.ErrorCode != "secondary_rate_limit" ||
		result.ProviderDiagnostics.HTTPStatus != http.StatusForbidden ||
		result.ProviderDiagnostics.RetryAfterAt == nil {
		t.Fatalf("result = %#v, calls = %d", result, calls.Load())
	}
}

func TestGitHubRateLimitResetIsUsedAsRetryDeadline(t *testing.T) {
	reset := time.Now().Add(time.Minute).Unix()
	header := http.Header{
		"X-Ratelimit-Reset":     []string{strconv.FormatInt(reset, 10)},
		"X-Ratelimit-Remaining": []string{"0"},
	}
	delay := retryDelay(header, 1)
	if delay <= 0 || delay > 5*time.Second {
		t.Fatalf("delay = %v", delay)
	}
	err, diagnostics := mapGitHubStatus(
		http.StatusForbidden,
		header,
		[]byte(`{"message":"API rate limit exceeded"}`),
		"request-1",
	)
	if integrations.ErrorCode(err) != integrations.ErrorCodeRateLimited {
		t.Fatalf("error = %v", err)
	}
	if diagnostics.RetryAfterAt == nil || diagnostics.RetryAfterAt.Unix() != reset {
		t.Fatalf("diagnostics = %#v", diagnostics)
	}
}

func TestValidateAndProbeConnectionReturnSecretFreeProfile(t *testing.T) {
	adapter := newTestAdapter(t, func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("X-GitHub-Request-Id", "health-request")
		w.Header().Set("X-OAuth-Scopes", "repo, read:org, repo")
		writeJSON(t, w, http.StatusOK, map[string]interface{}{"login": "octocat", "name": "The Octocat", "html_url": "https://github.com/octocat"})
	})
	connection := githubConnection("github_pat_health")
	profile, err := adapter.ValidateConnection(context.Background(), connection)
	if err != nil {
		t.Fatalf("validate: %v", err)
	}
	if profile.AccountID != "octocat" || profile.DisplayName != "The Octocat" || profile.ProviderRequestID != "health-request" {
		t.Fatalf("profile = %#v", profile)
	}
	if strings.Join(profile.GrantedScopes, ",") != "issues:read,issues:write,metadata:read,read:org,repo" {
		t.Fatalf("scopes = %#v", profile.GrantedScopes)
	}
	report, err := adapter.ProbeConnection(context.Background(), connection)
	if err != nil || report.Status != integrations.HealthProbeStatusHealthy || report.Profile == nil {
		t.Fatalf("probe = %#v, %v", report, err)
	}
}

func newTestAdapter(t *testing.T, handler http.HandlerFunc) *Adapter {
	t.Helper()
	server := httptest.NewServer(handler)
	t.Cleanup(server.Close)
	adapter, err := newForBaseURL(server.Client(), server.URL)
	if err != nil {
		t.Fatalf("create GitHub adapter: %v", err)
	}
	return adapter
}

func githubConnection(token string) *integrations.ResolvedConnection {
	return &integrations.ResolvedConnection{
		IntegrationID: IntegrationID,
		DriverID:      DriverID,
		Credentials:   map[string]string{"token": token},
	}
}

func writeJSON(t *testing.T, writer http.ResponseWriter, status int, value interface{}) {
	t.Helper()
	writer.Header().Set("Content-Type", "application/json")
	writer.WriteHeader(status)
	if err := json.NewEncoder(writer).Encode(value); err != nil {
		t.Errorf("encode response: %v", err)
	}
}

func assertGitHubOutputSchema(t *testing.T, actionID string, output map[string]interface{}) {
	t.Helper()
	for _, action := range Actions() {
		if action.ID != actionID {
			continue
		}
		if err := tools.ValidateJSONSchemaValue(action.OutputSchema, output); err != nil {
			t.Fatalf("action %s output schema: %v\noutput=%#v", actionID, err, output)
		}
		return
	}
	t.Fatalf("action %s not found", actionID)
}
