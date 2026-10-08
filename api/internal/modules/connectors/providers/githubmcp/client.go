// Package githubmcp adapts GitHub's official remote MCP server to the
// platform's existing personal Connector lifecycle and audited dispatch path.
package githubmcp

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/url"
	"sort"
	"strconv"
	"strings"
	"time"

	jsonschema "github.com/santhosh-tekuri/jsonschema/v6"
	"github.com/zgiai/zgi/api/internal/modules/connectors/core"
	"github.com/zgiai/zgi/api/internal/modules/connectors/providers"
	"github.com/zgiai/zgi/api/pkg/mcpclient"
)

const (
	connectorID    = "github-mcp"
	endpoint       = "https://api.githubcopilot.com/mcp/"
	toolGetMe      = "get_me"
	toolListIssues = "list_issues"
	toolIssueRead  = "issue_read"
	operationList  = "github-mcp.issue.list"
	operationGet   = "github-mcp.issue.get"
)

type Client struct {
	endpoint   string
	httpClient *http.Client
	schemas    *providers.Schemas
}

func New(httpClient *http.Client) (*Client, error) { return newAt(endpoint, httpClient) }
func newAt(serverURL string, httpClient *http.Client) (*Client, error) {
	schemas, err := providers.NewSchemas(connectorID)
	if err != nil {
		return nil, err
	}
	return &Client{endpoint: serverURL, httpClient: httpClient, schemas: schemas}, nil
}
func (*Client) ConnectorID() string { return connectorID }

func (c *Client) connect(ctx context.Context, key string) (*mcpclient.Connection, error) {
	if strings.TrimSpace(key) == "" || len(key) > 16384 || strings.ContainsAny(key, "\r\n") {
		return nil, providers.Invalid()
	}
	credentials := mcpclient.CredentialProviderFunc(func(context.Context, *url.URL) (http.Header, error) {
		return http.Header{
			"Authorization": []string{"Bearer " + key}, "X-MCP-Readonly": []string{"true"}, "X-MCP-Tools": []string{"get_me,list_issues,issue_read"},
		}, nil
	})
	client, err := mcpclient.NewClient(mcpclient.ServerConfig{ID: connectorID, Transport: mcpclient.TransportStreamableHTTP, Endpoint: c.endpoint,
		Limits: mcpclient.Limits{ConnectTimeout: 15 * time.Second, OperationTimeout: 30 * time.Second, MaxMessageBytes: 4 << 20, MaxConcurrentRequests: 2, MaxCatalogItems: 100}},
		mcpclient.Options{HTTPClient: c.httpClient, HTTPAuth: mcpclient.HTTPAuthConfig{Mode: mcpclient.AuthBearer, Credentials: credentials}})
	if err != nil {
		return nil, mcpFailure(err)
	}
	connection, err := client.Connect(ctx)
	if err != nil {
		return nil, mcpFailure(err)
	}
	return connection, nil
}
func closeConnection(connection *mcpclient.Connection) {
	if connection != nil {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = connection.Close(ctx)
	}
}

func (c *Client) Verify(ctx context.Context, key string) (providers.Verification, error) {
	connection, err := c.connect(ctx, key)
	if err != nil {
		return providers.Verification{}, err
	}
	defer closeConnection(connection)
	tools, err := connection.RefreshTools(ctx)
	if err != nil {
		return providers.Verification{}, mcpFailure(err)
	}
	for _, name := range []string{toolGetMe, toolListIssues, toolIssueRead} {
		if !hasTool(tools.Items, name) {
			return providers.Verification{}, &providers.Failure{Code: core.ErrorCodeInsufficientKey}
		}
	}
	for _, candidate := range []struct {
		name string
		args map[string]any
	}{
		{toolListIssues, listArguments("github", "github-mcp-server", 5)},
		{toolIssueRead, readArguments("github", "github-mcp-server", 1)},
	} {
		tool, _ := findTool(tools.Items, candidate.name)
		if validateRemoteArguments(tool.InputSchema, candidate.args) != nil {
			return providers.Verification{}, &providers.Failure{Code: core.ErrorCodeConnectorUnavailable}
		}
	}
	result, err := connection.CallTool(ctx, mcpclient.ToolCallRequest{Name: toolGetMe, Arguments: json.RawMessage(`{}`)})
	if err != nil {
		return providers.Verification{}, mcpFailure(err)
	}
	if result.IsError {
		return providers.Verification{}, &providers.Failure{Code: core.ErrorCodeInvalidKey}
	}
	profile, err := resultObject(result)
	if err != nil {
		return providers.Verification{}, badResponse()
	}
	login := providers.SafeText(text(profile, "login"), 128)
	if login == "" {
		return providers.Verification{}, badResponse()
	}
	accountID := numberString(profile["id"])
	if accountID == "" {
		accountID = login
	}
	return providers.Verification{AccountID: accountID, DisplayName: login, Operations: []string{operationList, operationGet}}, nil
}

func (c *Client) Invoke(ctx context.Context, operation, key string, input map[string]any) (providers.Response, error) {
	args, err := c.schemas.Input(operation, input)
	if err != nil {
		return providers.Response{}, providers.Invalid()
	}
	owner, repo := providers.Text(args, "owner"), providers.Text(args, "repo")
	if !safeSegment(owner) || !safeSegment(repo) {
		return providers.Response{}, providers.Invalid()
	}
	connection, err := c.connect(ctx, key)
	if err != nil {
		return providers.Response{}, err
	}
	defer closeConnection(connection)
	tools, err := connection.RefreshTools(ctx)
	if err != nil {
		return providers.Response{}, mcpFailure(err)
	}
	var toolName string
	var remoteArgs map[string]any
	switch operation {
	case operationList:
		toolName = toolListIssues
		limit := providers.Number(args, "limit")
		if limit < 1 || limit > 20 {
			return providers.Response{}, providers.Invalid()
		}
		remoteArgs = listArguments(owner, repo, limit)
	case operationGet:
		toolName = toolIssueRead
		issueNumber := providers.Number(args, "issue_number")
		if issueNumber < 1 {
			return providers.Response{}, providers.Invalid()
		}
		remoteArgs = readArguments(owner, repo, issueNumber)
	default:
		return providers.Response{}, providers.Invalid()
	}
	tool, found := findTool(tools.Items, toolName)
	if !found {
		return providers.Response{}, &providers.Failure{Code: core.ErrorCodeConnectorUnavailable}
	}
	if err := validateRemoteArguments(tool.InputSchema, remoteArgs); err != nil {
		return providers.Response{}, &providers.Failure{Code: core.ErrorCodeConnectorUnavailable}
	}
	encoded, _ := json.Marshal(remoteArgs)
	result, err := connection.CallTool(ctx, mcpclient.ToolCallRequest{Name: toolName, Arguments: encoded})
	if err != nil {
		return providers.Response{}, mcpFailure(err)
	}
	if result.IsError {
		return providers.Response{}, badResponse()
	}
	remote, err := resultObject(result)
	if err != nil {
		return providers.Response{}, badResponse()
	}
	output := map[string]any{"provider": connectorID, "transport": "mcp", "repository": owner + "/" + repo}
	if operation == operationList {
		limit := providers.Number(args, "limit")
		issues, err := listOutput(remote, owner, repo, limit)
		if err != nil {
			return providers.Response{}, badResponse()
		}
		output["issues"] = issues
		output["total_count"] = max(0, providers.Number(remote, "totalCount"))
		pageInfo, _ := remote["pageInfo"].(map[string]any)
		output["has_more"], _ = pageInfo["hasNextPage"].(bool)
	} else {
		issue, err := detailOutput(remote, owner, repo, providers.Number(args, "issue_number"))
		if err != nil {
			return providers.Response{}, badResponse()
		}
		output["issue"] = issue
	}
	providers.Redact(output, key)
	if c.schemas.Output(operation, output) != nil {
		return providers.Response{}, badResponse()
	}
	return providers.Response{Value: output}, nil
}

func hasTool(items []mcpclient.ToolDefinition, name string) bool {
	_, ok := findTool(items, name)
	return ok
}
func findTool(items []mcpclient.ToolDefinition, name string) (mcpclient.ToolDefinition, bool) {
	for _, item := range items {
		if item.Name == name {
			return item, true
		}
	}
	return mcpclient.ToolDefinition{}, false
}
func safeSegment(value string) bool {
	return value != "" && value != "." && value != ".." && value == strings.TrimSpace(value) && !strings.ContainsAny(value, "/\\\r\n?#")
}
func listArguments(owner, repo string, limit int) map[string]any {
	return map[string]any{"owner": owner, "repo": repo, "state": "OPEN", "orderBy": "UPDATED_AT", "direction": "DESC", "perPage": limit, "fields": []any{"number", "title", "state", "updated_at"}}
}
func readArguments(owner, repo string, issueNumber int) map[string]any {
	return map[string]any{"method": "get", "owner": owner, "repo": repo, "issue_number": issueNumber}
}
func issueURL(owner, repo string, number int) string {
	return "https://github.com/" + url.PathEscape(owner) + "/" + url.PathEscape(repo) + "/issues/" + strconv.Itoa(number)
}

type denyURLLoader struct{}

func (denyURLLoader) Load(string) (any, error) {
	return nil, errors.New("external schema references are unavailable")
}
func validateRemoteArguments(raw json.RawMessage, args map[string]any) error {
	var schemaDoc any
	if json.Unmarshal(raw, &schemaDoc) != nil {
		return errors.New("invalid remote schema")
	}
	compiler := jsonschema.NewCompiler()
	compiler.DefaultDraft(jsonschema.Draft2020)
	compiler.UseLoader(denyURLLoader{})
	if err := compiler.AddResource("urn:github-mcp:tool", schemaDoc); err != nil {
		return err
	}
	compiled, err := compiler.Compile("urn:github-mcp:tool")
	if err != nil {
		return err
	}
	return compiled.Validate(args)
}
func resultObject(result mcpclient.ToolCallResult) (map[string]any, error) {
	if result.StructuredContent.Present && string(result.StructuredContent.JSON) != "null" {
		var value map[string]any
		if json.Unmarshal(result.StructuredContent.JSON, &value) == nil && value != nil {
			return value, nil
		}
	}
	for _, block := range result.Content {
		if block.Type != "text" {
			continue
		}
		var textBlock struct {
			Text string `json:"text"`
		}
		if json.Unmarshal(block.Raw, &textBlock) != nil {
			continue
		}
		var value map[string]any
		if json.Unmarshal([]byte(textBlock.Text), &value) == nil && value != nil {
			return value, nil
		}
	}
	return nil, errors.New("GitHub MCP returned no JSON object")
}
func text(row map[string]any, key string) string { value, _ := row[key].(string); return value }
func numberString(value any) string {
	switch n := value.(type) {
	case float64:
		if n >= 1 && n == float64(int64(n)) {
			return strconv.FormatInt(int64(n), 10)
		}
	case string:
		return strings.TrimSpace(n)
	}
	return ""
}
func badResponse() error { return &providers.Failure{Code: core.ErrorCodeProviderUnavailable} }
func mcpFailure(err error) error {
	var typed *mcpclient.MCPError
	if !errors.As(err, &typed) {
		return badResponse()
	}
	switch typed.Kind {
	case mcpclient.ErrAuthRequired, mcpclient.ErrAuthFailed:
		return &providers.Failure{Code: core.ErrorCodeInvalidKey}
	case mcpclient.ErrTimeout, mcpclient.ErrCancelled, mcpclient.ErrTransport:
		return &providers.Failure{Code: core.ErrorCodeProviderUnavailable, Unknown: typed.MayHaveExecuted}
	default:
		return badResponse()
	}
}

func listOutput(remote map[string]any, owner, repo string, limit int) ([]any, error) {
	rows, ok := remote["issues"].([]any)
	if !ok {
		return nil, errors.New("issues missing")
	}
	result := make([]map[string]any, 0, min(len(rows), limit))
	for _, raw := range rows {
		row, ok := raw.(map[string]any)
		if !ok {
			return nil, errors.New("invalid issue")
		}
		number := providers.Number(row, "number")
		if number < 1 {
			return nil, errors.New("invalid issue number")
		}
		state := strings.ToLower(text(row, "state"))
		if state != "open" {
			continue
		}
		title := providers.SafeText(text(row, "title"), 500)
		updatedAt := providers.SafeText(text(row, "updated_at"), 64)
		if title == "" || updatedAt == "" {
			return nil, errors.New("incomplete issue")
		}
		result = append(result, map[string]any{"number": number, "title": title, "html_url": issueURL(owner, repo, number), "state": "open", "updated_at": updatedAt})
	}
	sort.SliceStable(result, func(i, j int) bool {
		left := result[i]["updated_at"].(string)
		right := result[j]["updated_at"].(string)
		leftTime, leftErr := time.Parse(time.RFC3339Nano, left)
		rightTime, rightErr := time.Parse(time.RFC3339Nano, right)
		if leftErr == nil && rightErr == nil {
			return leftTime.After(rightTime)
		}
		return left > right
	})
	if len(result) > limit {
		result = result[:limit]
	}
	items := make([]any, len(result))
	for i, item := range result {
		items[i] = item
	}
	return items, nil
}
func detailOutput(remote map[string]any, owner, repo string, wantNumber int) (map[string]any, error) {
	if text(remote, "method") != "get" {
		return nil, errors.New("unexpected issue method")
	}
	row, ok := remote["issue"].(map[string]any)
	if !ok {
		return nil, errors.New("issue missing")
	}
	number := providers.Number(row, "number")
	if number != wantNumber {
		return nil, errors.New("issue number mismatch")
	}
	if text(row, "title") == "" || text(row, "state") == "" {
		return nil, errors.New("incomplete issue detail")
	}
	user, _ := row["user"].(map[string]any)
	return map[string]any{"number": number, "title": providers.SafeText(text(row, "title"), 500), "body": providers.SafeText(text(row, "body"), 20000), "html_url": issueURL(owner, repo, number), "state": providers.SafeText(text(row, "state"), 32), "updated_at": providers.SafeText(text(row, "updated_at"), 64), "author": providers.SafeText(text(user, "login"), 128)}, nil
}

var _ providers.KeyDriver = (*Client)(nil)
