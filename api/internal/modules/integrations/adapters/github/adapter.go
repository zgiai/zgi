package github

import (
	"context"
	"encoding/json"
	"net/http"
	"net/url"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/zgiai/zgi/api/internal/modules/integrations"
)

type Adapter struct {
	client *client
}

func New(httpClient *http.Client) (*Adapter, error) {
	apiClient, err := newClient(httpClient)
	if err != nil {
		return nil, err
	}
	return &Adapter{client: apiClient}, nil
}

func newForBaseURL(httpClient *http.Client, baseURL string) (*Adapter, error) {
	apiClient, err := newClientForBaseURL(httpClient, baseURL)
	if err != nil {
		return nil, err
	}
	return &Adapter{client: apiClient}, nil
}

func (adapter *Adapter) DriverID() string { return DriverID }

func (adapter *Adapter) Execute(ctx context.Context, request integrations.ActionRequest) (*integrations.ActionResult, error) {
	token, err := githubToken(request.Connection)
	if err != nil {
		return nil, err
	}
	switch request.ActionID {
	case ActionGetAuthenticatedUser:
		output, meta, err := adapter.getAuthenticatedUser(ctx, token)
		return actionResult(output, meta), err
	case ActionListRepositories:
		output, meta, err := adapter.listRepositories(ctx, token, request.Input)
		return actionResult(output, meta), err
	case ActionListIssues:
		output, meta, err := adapter.listIssues(ctx, token, request.Input)
		return actionResult(output, meta), err
	default:
		return nil, integrations.NewError(integrations.ErrorCodeInvalidInput, "GitHub action is not supported", nil)
	}
}

func (adapter *Adapter) ValidateConnection(ctx context.Context, connection *integrations.ResolvedConnection) (*integrations.ConnectionProfile, error) {
	token, err := githubToken(connection)
	if err != nil {
		return nil, err
	}
	var user githubUserResponse
	meta, err := adapter.client.getJSON(ctx, token, "/user", nil, &user)
	if err != nil {
		return nil, err
	}
	return &integrations.ConnectionProfile{
		AccountID: user.Login, DisplayName: firstNonEmpty(user.Name, user.Login),
		GrantedScopes: githubScopeSnapshot(meta.Scopes), ProviderRequestID: meta.RequestID,
	}, nil
}

func (adapter *Adapter) ProbeConnection(ctx context.Context, connection *integrations.ResolvedConnection) (*integrations.HealthProbeReport, error) {
	profile, err := adapter.ValidateConnection(ctx, connection)
	if err != nil {
		status := integrations.HealthProbeStatusUnhealthy
		if code := integrations.ErrorCode(err); code == integrations.ErrorCodeTimeout || code == integrations.ErrorCodeUpstream || code == integrations.ErrorCodeRateLimited {
			status = integrations.HealthProbeStatusDegraded
		}
		return &integrations.HealthProbeReport{
			Status: status,
			Checks: []integrations.HealthProbeCheck{{Code: integrations.ErrorCode(err), Status: status, Message: "GitHub connection check failed"}},
		}, err
	}
	return &integrations.HealthProbeReport{
		Status: integrations.HealthProbeStatusHealthy, Profile: profile,
		Checks: []integrations.HealthProbeCheck{{Code: "github_authenticated_user", Status: integrations.HealthProbeStatusHealthy}},
	}, nil
}

func (adapter *Adapter) getAuthenticatedUser(ctx context.Context, token string) (map[string]interface{}, responseMeta, error) {
	var user githubUserResponse
	meta, err := adapter.client.getJSON(ctx, token, "/user", nil, &user)
	if err != nil {
		return nil, meta, err
	}
	return map[string]interface{}{
		"provider": IntegrationID, "request_id": bounded(userSafe(meta.RequestID), 128),
		"user": map[string]interface{}{
			"login": bounded(user.Login, 128), "name": bounded(user.Name, 256),
			"html_url": safeGitHubURL(user.HTMLURL), "company": bounded(user.Company, 256),
			"location": bounded(user.Location, 256),
		},
	}, meta, nil
}

func (adapter *Adapter) listRepositories(ctx context.Context, token string, input map[string]interface{}) (map[string]interface{}, responseMeta, error) {
	page := inputInteger(input, "page", 1, 1, 1000)
	perPage := inputInteger(input, "per_page", 20, 1, 50)
	query := url.Values{}
	query.Set("visibility", inputEnum(input, "visibility", "all", "all", "public", "private"))
	query.Set("affiliation", inputEnum(input, "affiliation", "owner", "owner", "collaborator", "organization_member"))
	query.Set("sort", inputEnum(input, "sort", "updated", "created", "updated", "pushed", "full_name"))
	query.Set("direction", inputEnum(input, "direction", "desc", "asc", "desc"))
	query.Set("per_page", strconv.Itoa(perPage))
	query.Set("page", strconv.Itoa(page))
	var raw []githubRepositoryResponse
	meta, err := adapter.client.getJSON(ctx, token, "/user/repos", query, &raw)
	if err != nil {
		return nil, meta, err
	}
	items := make([]interface{}, 0, min(len(raw), perPage))
	for index, repository := range raw {
		if index >= perPage {
			break
		}
		visibility := strings.ToLower(strings.TrimSpace(repository.Visibility))
		if visibility == "" {
			if repository.Private {
				visibility = "private"
			} else {
				visibility = "public"
			}
		}
		items = append(items, map[string]interface{}{
			"full_name": bounded(repository.FullName, 300), "html_url": safeGitHubURL(repository.HTMLURL),
			"description": bounded(repository.Description, 1000), "visibility": bounded(visibility, 32),
			"private": repository.Private, "archived": repository.Archived,
			"default_branch": bounded(repository.DefaultBranch, 255), "language": bounded(repository.Language, 100),
			"updated_at": bounded(repository.UpdatedAt, 64), "pushed_at": bounded(repository.PushedAt, 64),
			"open_issues_count": max(repository.OpenIssuesCount, 0),
		})
	}
	return map[string]interface{}{
		"provider": IntegrationID, "request_id": bounded(userSafe(meta.RequestID), 128),
		"page": page, "repositories": items,
	}, meta, nil
}

func (adapter *Adapter) listIssues(ctx context.Context, token string, input map[string]interface{}) (map[string]interface{}, responseMeta, error) {
	owner := strings.TrimSpace(inputString(input, "owner"))
	repository := strings.TrimSpace(inputString(input, "repo"))
	if owner == "" || repository == "" {
		return nil, responseMeta{}, integrations.NewError(integrations.ErrorCodeInvalidInput, "GitHub repository owner and name are required", nil)
	}
	page := inputInteger(input, "page", 1, 1, 1000)
	perPage := inputInteger(input, "per_page", 20, 1, 50)
	query := url.Values{}
	query.Set("state", inputEnum(input, "state", "open", "open", "closed", "all"))
	query.Set("sort", inputEnum(input, "sort", "updated", "created", "updated", "comments"))
	query.Set("direction", inputEnum(input, "direction", "desc", "asc", "desc"))
	query.Set("per_page", strconv.Itoa(perPage))
	query.Set("page", strconv.Itoa(page))
	if labels := inputStrings(input, "labels", 10); len(labels) > 0 {
		query.Set("labels", strings.Join(labels, ","))
	}
	if since := strings.TrimSpace(inputString(input, "since")); since != "" {
		if _, err := time.Parse(time.RFC3339, since); err != nil {
			return nil, responseMeta{}, integrations.NewError(integrations.ErrorCodeInvalidInput, "GitHub issue since must be RFC 3339", err)
		}
		query.Set("since", since)
	}
	path := "/repos/" + url.PathEscape(owner) + "/" + url.PathEscape(repository) + "/issues"
	var raw []githubIssueResponse
	meta, err := adapter.client.getJSON(ctx, token, path, query, &raw)
	if err != nil {
		return nil, meta, err
	}
	includePullRequests, _ := input["include_pull_requests"].(bool)
	items := make([]interface{}, 0, min(len(raw), perPage))
	for _, issue := range raw {
		isPullRequest := issue.PullRequest != nil
		if isPullRequest && !includePullRequests {
			continue
		}
		if len(items) >= perPage {
			break
		}
		labels := make([]interface{}, 0, min(len(issue.Labels), 20))
		for index, label := range issue.Labels {
			if index >= 20 {
				break
			}
			if value := bounded(label.Name, 100); value != "" {
				labels = append(labels, value)
			}
		}
		kind := "issue"
		if isPullRequest {
			kind = "pull_request"
		}
		items = append(items, map[string]interface{}{
			"number": max(issue.Number, 1), "title": bounded(issue.Title, 500), "state": bounded(issue.State, 32),
			"kind": kind, "html_url": safeGitHubURL(issue.HTMLURL), "author": bounded(issue.User.Login, 128),
			"labels": labels, "comments": max(issue.Comments, 0), "created_at": bounded(issue.CreatedAt, 64),
			"updated_at": bounded(issue.UpdatedAt, 64),
		})
	}
	return map[string]interface{}{
		"provider": IntegrationID, "request_id": bounded(userSafe(meta.RequestID), 128),
		"repository": bounded(owner+"/"+repository, 300), "page": page, "issues": items,
	}, meta, nil
}

func githubToken(connection *integrations.ResolvedConnection) (string, error) {
	if connection == nil || !strings.EqualFold(connection.IntegrationID, IntegrationID) || !strings.EqualFold(connection.DriverID, DriverID) {
		return "", integrations.NewError(integrations.ErrorCodeConnectionInvalid, "GitHub connection is invalid", nil)
	}
	token := strings.TrimSpace(connection.Credentials["token"])
	if token == "" {
		return "", integrations.NewError(integrations.ErrorCodeAuthInvalid, "GitHub credentials are unavailable", nil)
	}
	return token, nil
}

func actionResult(output map[string]interface{}, meta responseMeta) *integrations.ActionResult {
	if output == nil && meta.Attempts == 0 && meta.RequestID == "" &&
		meta.Diagnostics == (integrations.ProviderDiagnostics{}) {
		return nil
	}
	count := 0
	if output != nil {
		count = 1
	}
	if values, ok := output["repositories"].([]interface{}); ok {
		count = len(values)
	}
	if values, ok := output["issues"].([]interface{}); ok {
		count = len(values)
	}
	attempts := meta.Attempts
	if output != nil && attempts == 0 {
		attempts = 1
	}
	return &integrations.ActionResult{
		Output:              output,
		ProviderRequestID:   meta.RequestID,
		ProviderDiagnostics: meta.Diagnostics,
		ResultCount:         count,
		AttemptCount:        attempts,
	}
}

type githubUserResponse struct {
	Login    string `json:"login"`
	Name     string `json:"name"`
	HTMLURL  string `json:"html_url"`
	Company  string `json:"company"`
	Location string `json:"location"`
}

type githubRepositoryResponse struct {
	FullName        string `json:"full_name"`
	HTMLURL         string `json:"html_url"`
	Description     string `json:"description"`
	Visibility      string `json:"visibility"`
	Private         bool   `json:"private"`
	Archived        bool   `json:"archived"`
	DefaultBranch   string `json:"default_branch"`
	Language        string `json:"language"`
	UpdatedAt       string `json:"updated_at"`
	PushedAt        string `json:"pushed_at"`
	OpenIssuesCount int    `json:"open_issues_count"`
}

type githubIssueResponse struct {
	Number      int                `json:"number"`
	Title       string             `json:"title"`
	State       string             `json:"state"`
	HTMLURL     string             `json:"html_url"`
	User        githubIssueUser    `json:"user"`
	Labels      []githubIssueLabel `json:"labels"`
	Comments    int                `json:"comments"`
	CreatedAt   string             `json:"created_at"`
	UpdatedAt   string             `json:"updated_at"`
	PullRequest *struct{}          `json:"pull_request"`
}

type githubIssueUser struct {
	Login string `json:"login"`
}
type githubIssueLabel struct {
	Name string `json:"name"`
}

func bounded(value string, limit int) string {
	value = strings.TrimSpace(value)
	runes := []rune(value)
	if len(runes) > limit {
		return string(runes[:limit])
	}
	return value
}

func safeGitHubURL(value string) string {
	parsed, err := url.Parse(strings.TrimSpace(value))
	if err != nil || parsed.Scheme != "https" || !strings.EqualFold(parsed.Hostname(), "github.com") || parsed.User != nil {
		return ""
	}
	parsed.Fragment = ""
	return bounded(parsed.String(), 2048)
}

func inputString(input map[string]interface{}, key string) string {
	value, _ := input[key].(string)
	return value
}

func inputStrings(input map[string]interface{}, key string, limit int) []string {
	raw, ok := input[key]
	if !ok {
		return nil
	}
	result := make([]string, 0, limit)
	seen := map[string]struct{}{}
	appendValue := func(value string) {
		value = strings.TrimSpace(value)
		if value == "" || len(result) >= limit {
			return
		}
		if _, exists := seen[value]; exists {
			return
		}
		seen[value] = struct{}{}
		result = append(result, value)
	}
	switch values := raw.(type) {
	case []string:
		for _, value := range values {
			appendValue(value)
		}
	case []interface{}:
		for _, item := range values {
			if value, ok := item.(string); ok {
				appendValue(value)
			}
		}
	}
	return result
}

func inputInteger(input map[string]interface{}, key string, fallback, minimum, maximum int) int {
	value := fallback
	switch typed := input[key].(type) {
	case int:
		value = typed
	case int64:
		value = int(typed)
	case float64:
		if typed == float64(int(typed)) {
			value = int(typed)
		}
	case json.Number:
		if parsed, err := strconv.Atoi(string(typed)); err == nil {
			value = parsed
		}
	}
	return min(max(value, minimum), maximum)
}

func inputEnum(input map[string]interface{}, key, fallback string, allowed ...string) string {
	value := strings.ToLower(strings.TrimSpace(inputString(input, key)))
	for _, candidate := range allowed {
		if value == candidate {
			return value
		}
	}
	return fallback
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if value = strings.TrimSpace(value); value != "" {
			return value
		}
	}
	return ""
}

func userSafe(value string) string {
	return strings.Map(func(char rune) rune {
		if char >= 32 && char != 127 {
			return char
		}
		return -1
	}, value)
}

func githubScopeSnapshot(scopes []string) []string {
	if len(scopes) == 0 {
		return nil
	}
	seen := make(map[string]struct{}, len(scopes)+2)
	for _, scope := range scopes {
		if scope = strings.ToLower(strings.TrimSpace(scope)); scope != "" {
			seen[scope] = struct{}{}
		}
	}
	// Classic PATs expose OAuth scope headers while fine-grained PATs usually
	// do not. Translate the classic repository grants into the stable action
	// permission vocabulary declared by this provider. An absent header remains
	// unknown and is verified by the actual action instead of being overclaimed.
	if _, granted := seen["repo"]; granted {
		seen["metadata:read"] = struct{}{}
		seen["issues:read"] = struct{}{}
	}
	if _, granted := seen["public_repo"]; granted {
		seen["metadata:read"] = struct{}{}
		seen["issues:read"] = struct{}{}
	}
	result := make([]string, 0, len(seen))
	for scope := range seen {
		result = append(result, scope)
	}
	sort.Strings(result)
	return result
}

var _ integrations.Adapter = (*Adapter)(nil)
var _ integrations.ConnectionTester = (*Adapter)(nil)
var _ integrations.HealthProbe = (*Adapter)(nil)
