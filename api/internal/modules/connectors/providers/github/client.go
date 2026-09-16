// Package github implements the accepted Connector operations over GitHub REST.
package github

import (
	"context"
	"encoding/json"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/zgiai/zgi/api/internal/modules/connectors/core"
	"github.com/zgiai/zgi/api/internal/modules/connectors/providers"
)

type Client struct {
	http    *providers.HTTP
	schemas *providers.Schemas
}

func New(client *http.Client) (*Client, error) {
	schemas, err := providers.NewSchemas("github")
	if err != nil {
		return nil, err
	}
	return &Client{http: providers.NewHTTP(client, "https://api.github.com"), schemas: schemas}, nil
}
func (*Client) ConnectorID() string { return "github" }
func headers(key string) http.Header {
	return http.Header{"Authorization": []string{"Bearer " + key}, "X-Github-Api-Version": []string{"2022-11-28"}}
}
func (client *Client) Verify(ctx context.Context, key string) (providers.Verification, error) {
	if strings.TrimSpace(key) == "" {
		return providers.Verification{}, providers.Invalid()
	}
	var account struct {
		ID    int64  `json:"id"`
		Login string `json:"login"`
		Name  string `json:"name"`
	}
	header, err := client.http.JSON(ctx, http.MethodGet, "/user", nil, headers(key), nil, &account)
	if err != nil {
		return providers.Verification{}, err
	}
	if account.ID < 1 || strings.TrimSpace(account.Login) == "" {
		return providers.Verification{}, &providers.Failure{Code: core.ErrorCodeProviderUnavailable}
	}
	display := account.Name
	if display == "" {
		display = account.Login
	}
	return providers.Verification{AccountID: strconv.FormatInt(account.ID, 10), DisplayName: providers.SafeText(display, 256), RequestID: providers.SafeRequestID(header.Get("X-GitHub-Request-Id"), key), Operations: []string{"github.user.get", "github.repository.list", "github.repository.search", "github.issue.list", "github.issue.get", "github.issue.comment.list", "github.issue.create", "github.issue.comment.create"}}, nil
}

func (client *Client) Invoke(ctx context.Context, operation, key string, input map[string]any) (providers.Response, error) {
	args, err := client.schemas.Input(operation, input)
	if err != nil || strings.TrimSpace(key) == "" {
		return providers.Response{}, providers.Invalid()
	}
	method := http.MethodGet
	endpoint := ""
	query := url.Values{}
	var body any
	owner, repo := providers.Text(args, "owner"), providers.Text(args, "repo")
	// Schema identifiers permit dots, but complete path traversal segments are
	// never valid repository coordinates.
	if owner == "." || owner == ".." || repo == "." || repo == ".." {
		return providers.Response{}, providers.Invalid()
	}
	issuePath := "/repos/" + url.PathEscape(owner) + "/" + url.PathEscape(repo) + "/issues"
	number := providers.Number(args, "issue_number")
	if since := providers.Text(args, "since"); since != "" {
		if _, err := time.Parse(time.RFC3339, since); err != nil {
			return providers.Response{}, providers.Invalid()
		}
	}
	switch operation {
	case "github.user.get":
		endpoint = "/user"
	case "github.repository.list":
		endpoint = "/user/repos"
		queryFields(query, args, "page", "per_page", "visibility", "affiliation", "sort", "direction")
	case "github.repository.search":
		endpoint = "/search/repositories"
		query.Set("q", providers.Text(args, "query"))
		queryFields(query, args, "page", "per_page", "order")
		if providers.Text(args, "sort") != "best_match" {
			query.Set("sort", providers.Text(args, "sort"))
		}
	case "github.issue.list":
		endpoint = issuePath
		queryFields(query, args, "page", "per_page", "state", "sort", "direction", "since")
		if labels := providers.TextList(args, "labels"); len(labels) > 0 {
			query.Set("labels", strings.Join(labels, ","))
		}
	case "github.issue.get":
		endpoint = issuePath + "/" + strconv.Itoa(number)
	case "github.issue.comment.list":
		endpoint = issuePath + "/" + strconv.Itoa(number) + "/comments"
		queryFields(query, args, "page", "per_page", "since")
	case "github.issue.create":
		endpoint = issuePath
		method = http.MethodPost
		payload := map[string]any{}
		for _, key := range []string{"title", "body", "labels", "assignees", "milestone"} {
			if value, ok := args[key]; ok {
				payload[key] = value
			}
		}
		body = payload
	case "github.issue.comment.create":
		endpoint = issuePath + "/" + strconv.Itoa(number) + "/comments"
		method = http.MethodPost
		body = map[string]any{"body": args["body"]}
	default:
		return providers.Response{}, providers.Invalid()
	}
	var raw json.RawMessage
	header, err := client.http.JSON(ctx, method, endpoint, query, headers(key), body, &raw)
	if err != nil {
		return providers.Response{}, err
	}
	requestID := providers.SafeRequestID(header.Get("X-GitHub-Request-Id"), key)
	output := map[string]any{"provider": "github", "request_id": requestID}
	if owner != "" {
		output["repository"] = owner + "/" + repo
	}
	page, limit := providers.Number(args, "page"), providers.Number(args, "per_page")
	switch operation {
	case "github.user.get":
		item, err := object(raw)
		if err != nil {
			return providers.Response{}, err
		}
		output["user"] = fields(item, map[string]int{"login": 128, "name": 256, "company": 256, "location": 256, "html_url": 2048})
	case "github.repository.list", "github.repository.search":
		var rows []map[string]any
		if operation == "github.repository.search" {
			var search struct {
				Items      []map[string]any `json:"items"`
				Total      int              `json:"total_count"`
				Incomplete bool             `json:"incomplete_results"`
			}
			if json.Unmarshal(raw, &search) != nil {
				return providers.Response{}, badResponse()
			}
			rows = search.Items
			output["query"] = args["query"]
			output["total_count"] = max(0, search.Total)
			output["incomplete_results"] = search.Incomplete
		} else if json.Unmarshal(raw, &rows) != nil {
			return providers.Response{}, badResponse()
		}
		items := make([]any, 0, min(len(rows), limit))
		for _, row := range rows[:min(len(rows), limit)] {
			items = append(items, repository(row))
		}
		output["page"] = page
		output["repositories"] = items
	case "github.issue.list", "github.issue.comment.list":
		var rows []map[string]any
		if json.Unmarshal(raw, &rows) != nil {
			return providers.Response{}, badResponse()
		}
		items := []any{}
		for _, row := range rows {
			if len(items) >= limit {
				break
			}
			if operation == "github.issue.list" {
				include, _ := args["include_pull_requests"].(bool)
				if row["pull_request"] != nil && !include {
					continue
				}
				items = append(items, issue(row, false))
			} else {
				items = append(items, comment(row))
			}
		}
		output["page"] = page
		if operation == "github.issue.list" {
			output["issues"] = items
			output["has_more"] = len(rows) >= limit
		} else {
			output["comments"] = items
			output["issue_number"] = number
		}
	case "github.issue.get", "github.issue.create":
		item, err := object(raw)
		if err != nil {
			return providers.Response{}, err
		}
		output["issue"] = issue(item, true)
	case "github.issue.comment.create":
		item, err := object(raw)
		if err != nil {
			return providers.Response{}, err
		}
		output["comment"] = comment(item)
		output["issue_number"] = number
	}
	providers.Redact(output, key)
	if client.schemas.Output(operation, output) != nil {
		return providers.Response{}, badResponse()
	}
	return providers.Response{Value: output, RequestID: requestID}, nil
}

func queryFields(query url.Values, args map[string]any, keys ...string) {
	for _, key := range keys {
		switch value := args[key].(type) {
		case string:
			if value != "" {
				query.Set(key, value)
			}
		case float64:
			query.Set(key, strconv.Itoa(int(value)))
		}
	}
}
func badResponse() error {
	return &providers.Failure{Code: core.ErrorCodeOutcomeUnknown, Unknown: true}
}
func object(raw []byte) (map[string]any, error) {
	var row map[string]any
	if json.Unmarshal(raw, &row) != nil || row == nil {
		return nil, badResponse()
	}
	return row, nil
}
func fields(row map[string]any, limits map[string]int) map[string]any {
	output := map[string]any{}
	for key, limit := range limits {
		value := providers.SafeText(providers.Text(row, key), limit)
		if key == "html_url" {
			value = providers.PublicURL(value)
		}
		output[key] = value
	}
	return output
}
func boolean(row map[string]any, key string) bool { value, _ := row[key].(bool); return value }
func author(row map[string]any) string {
	user, _ := row["user"].(map[string]any)
	return providers.SafeText(providers.Text(user, "login"), 128)
}
func repository(row map[string]any) map[string]any {
	output := fields(row, map[string]int{"full_name": 300, "html_url": 2048, "description": 1000, "visibility": 32, "default_branch": 255, "language": 100, "updated_at": 64, "pushed_at": 64})
	output["private"] = boolean(row, "private")
	output["archived"] = boolean(row, "archived")
	output["open_issues_count"] = max(0, providers.Number(row, "open_issues_count"))
	if output["visibility"] == "" {
		output["visibility"] = "public"
		if boolean(row, "private") {
			output["visibility"] = "private"
		}
	}
	return output
}
func issue(row map[string]any, detail bool) map[string]any {
	output := fields(row, map[string]int{"title": 500, "state": 32, "html_url": 2048, "created_at": 64, "updated_at": 64})
	output["number"] = max(1, providers.Number(row, "number"))
	output["comments"] = max(0, providers.Number(row, "comments"))
	output["author"] = author(row)
	output["kind"] = "issue"
	if row["pull_request"] != nil {
		output["kind"] = "pull_request"
	}
	labels := []any{}
	rows, _ := row["labels"].([]any)
	for _, raw := range rows[:min(len(rows), 20)] {
		item, _ := raw.(map[string]any)
		if name := providers.SafeText(providers.Text(item, "name"), 100); name != "" {
			labels = append(labels, name)
		}
	}
	output["labels"] = labels
	if detail {
		output["body"] = providers.SafeText(providers.Text(row, "body"), 20000)
		output["locked"] = boolean(row, "locked")
	}
	return output
}
func comment(row map[string]any) map[string]any {
	output := fields(row, map[string]int{"body": 20000, "html_url": 2048, "created_at": 64, "updated_at": 64})
	output["id"] = max(1, providers.Number(row, "id"))
	output["author"] = author(row)
	return output
}
