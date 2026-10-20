// Package socialmcp adapts a deployment-owned, isolated stdio MCP bridge to
// native Connector verification, binding, confirmation and audit boundaries.
package socialmcp

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"time"

	jsonschema "github.com/santhosh-tekuri/jsonschema/v6"
	"github.com/zgiai/zgi/api/internal/modules/connectors/catalog"
	"github.com/zgiai/zgi/api/internal/modules/connectors/core"
	"github.com/zgiai/zgi/api/internal/modules/connectors/providers"
	"github.com/zgiai/zgi/api/pkg/mcpclient"
)

const maxResultBytes = 64 << 10

type Options struct {
	// Both values are trusted deployment configuration, never Connector fields
	// or model inputs. Empty ScriptPath leaves verification unavailable.
	NodeCommand string
	ScriptPath  string
	Launcher    mcpclient.ProcessLauncher
	Events      EventStore
}

type Client struct {
	definition catalog.Definition
	schemas    *providers.Schemas
	options    Options
	open       func(context.Context, string) (*mcpclient.Connection, func(), error)
}

func New(id string, options Options) (*Client, error) {
	var definition catalog.Definition
	for _, d := range catalog.SocialMCPDefinitions() {
		if d.ID == id {
			definition = d
		}
	}
	if definition.ID == "" {
		return nil, providers.Invalid()
	}
	if options.NodeCommand == "" {
		options.NodeCommand = "node"
	}
	if strings.ContainsAny(options.NodeCommand, "\x00\r\n\t ") || (options.ScriptPath != "" && (!filepath.IsAbs(options.ScriptPath) || strings.ContainsAny(options.ScriptPath, "\x00\r\n"))) {
		return nil, providers.Invalid()
	}
	schemas, err := providers.NewSchemasForDefinition(definition)
	if err != nil {
		return nil, err
	}
	c := &Client{definition: definition, schemas: schemas, options: options}
	c.open = c.connect
	return c, nil
}

func (c *Client) ConnectorID() string { return c.definition.ID }

func (c *Client) processSpec(token string) (mcpclient.ProcessSpec, func(), error) {
	if c.options.ScriptPath == "" {
		return mcpclient.ProcessSpec{}, nil, &providers.Failure{Code: core.ErrorCodeConnectorUnavailable}
	}
	// A fresh HOME prevents third-party packages from loading another user's
	// cached credentials or leaving token/audit files in the API user's HOME.
	home, err := os.MkdirTemp("", "zgi-social-mcp-")
	if err != nil {
		return mcpclient.ProcessSpec{}, nil, &providers.Failure{Code: core.ErrorCodeConnectorUnavailable}
	}
	cleanup := func() { _ = os.RemoveAll(home) }
	contracts := []map[string]any{}
	for _, op := range c.definition.Operations {
		contracts = append(contracts, map[string]any{"name": remoteName(c.definition.ID, op.ID), "inputSchema": op.InputSchema, "description": op.Description})
	}
	encoded, err := json.Marshal(contracts)
	if err != nil {
		cleanup()
		return mcpclient.ProcessSpec{}, nil, providers.Invalid()
	}
	return mcpclient.ProcessSpec{Command: c.options.NodeCommand, Args: []string{c.options.ScriptPath}, WorkDir: home, Env: map[string]string{
		"HOME": home, "USERPROFILE": home, "APPDATA": home, "LOCALAPPDATA": home, "TMPDIR": home, "TEMP": home, "TMP": home,
		"ZGI_SOCIAL_PROVIDER": c.definition.ID, "ZGI_SOCIAL_TOKEN": token, "ZGI_SOCIAL_CONTRACT": string(encoded),
	}}, cleanup, nil
}

func (c *Client) connect(ctx context.Context, token string) (*mcpclient.Connection, func(), error) {
	spec, cleanup, err := c.processSpec(token)
	if err != nil {
		return nil, func() {}, err
	}
	client, err := mcpclient.NewClient(mcpclient.ServerConfig{ID: c.definition.ID, Transport: mcpclient.TransportStdio, Process: &spec, Limits: mcpclient.Limits{ConnectTimeout: 15 * time.Second, OperationTimeout: 50 * time.Second, CloseGrace: 2 * time.Second, MaxMessageBytes: 2 << 20, MaxCatalogItems: 32, MaxCatalogPages: 2, MaxConcurrentRequests: 1}}, mcpclient.Options{ProcessLauncher: c.options.Launcher})
	if err != nil {
		cleanup()
		return nil, func() {}, &providers.Failure{Code: core.ErrorCodeConnectorUnavailable}
	}
	connection, err := client.Connect(ctx)
	if err != nil {
		cleanup()
		return nil, func() {}, &providers.Failure{Code: core.ErrorCodeConnectorUnavailable}
	}
	return connection, func() {
		closeCtx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
		defer cancel()
		_ = connection.Close(closeCtx)
		cleanup()
	}, nil
}

func validToken(token string) bool {
	return token != "" && token == strings.TrimSpace(token) && len(token) <= 16384 && !strings.ContainsAny(token, "\x00\r\n\t")
}
func remoteName(provider, operation string) string {
	return strings.ReplaceAll(strings.TrimPrefix(operation, provider+"."), ".", "_")
}

func (c *Client) Verify(ctx context.Context, token string) (providers.Verification, error) {
	if !validToken(token) {
		return providers.Verification{}, providers.Invalid()
	}
	connection, close, err := c.open(ctx, token)
	if err != nil {
		return providers.Verification{}, err
	}
	defer close()
	tools, err := connection.RefreshTools(ctx)
	if err != nil {
		return providers.Verification{}, &providers.Failure{Code: core.ErrorCodeConnectorUnavailable}
	}
	if !toolExists(tools.Items, "account_get", map[string]any{}) {
		return providers.Verification{}, &providers.Failure{Code: core.ErrorCodeConnectorUnavailable}
	}
	result, err := connection.CallTool(ctx, mcpclient.ToolCallRequest{Name: "account_get", Arguments: json.RawMessage(`{}`)})
	if err != nil {
		return providers.Verification{}, &providers.Failure{Code: core.ErrorCodeProviderUnavailable}
	}
	account, err := decodeResult(result, false)
	if err != nil {
		return providers.Verification{}, err
	}
	id, name := providers.Text(account, "id"), providers.Text(account, "name")
	if id == "" || len(id) > 256 || name == "" || len(name) > 256 || strings.Contains(id, token) || strings.Contains(name, token) {
		return providers.Verification{}, &providers.Failure{Code: core.ErrorCodeProviderUnavailable}
	}
	available, ok := account["operations"].([]any)
	if !ok {
		return providers.Verification{}, &providers.Failure{Code: core.ErrorCodeProviderUnavailable}
	}
	operations := []string{}
	for _, op := range c.definition.Operations {
		tool := remoteName(c.definition.ID, op.ID)
		if c.unavailableMessaging(op.ID) || localEventOperation(op.ID) {
			continue
		}
		for _, raw := range available {
			if raw == tool && listed(tools.Items, tool) {
				operations = append(operations, op.ID)
				break
			}
		}
	}
	if c.definition.ID == "facebook-social" && c.options.Events != nil {
		operations = append(operations, "facebook-social.event.list", "facebook-social.event.ack")
	}
	if len(operations) == 0 {
		return providers.Verification{}, &providers.Failure{Code: core.ErrorCodeInsufficientKey}
	}
	return providers.Verification{AccountID: providers.SafeText(id, 256), DisplayName: providers.SafeText(name, 256), Operations: operations}, nil
}

func (c *Client) unavailableMessaging(operation string) bool {
	return c.definition.ID == "linkedin-social" && (strings.HasPrefix(operation, "linkedin-social.message.") || operation == "linkedin-social.conversation.list")
}

func (c *Client) Invoke(ctx context.Context, operation, token string, input map[string]any) (providers.Response, error) {
	if localEventOperation(operation) {
		return providers.Response{}, &providers.Failure{Code: core.ErrorCodeAccessDenied}
	}
	if !validToken(token) {
		return providers.Response{}, providers.Invalid()
	}
	// Own the map before normalizing any image fields.
	ownedInput := make(map[string]any, len(input))
	for key, value := range input {
		ownedInput[key] = value
	}
	args, err := c.schemas.Input(operation, ownedInput)
	if err != nil {
		return providers.Response{}, providers.Invalid()
	}
	if err := normalizeImageInput(args); err != nil {
		return providers.Response{}, err
	}
	if c.unavailableMessaging(operation) {
		return providers.Response{}, &providers.Failure{Code: core.ErrorCodeInsufficientKey}
	}
	write := false
	for _, op := range c.definition.Operations {
		if op.ID == operation {
			write = op.Effect != catalog.OperationEffectRead
			break
		}
	}
	connection, close, err := c.open(ctx, token)
	if err != nil {
		return providers.Response{}, err
	}
	defer close()
	tools, err := connection.RefreshTools(ctx)
	if err != nil {
		return providers.Response{}, &providers.Failure{Code: core.ErrorCodeConnectorUnavailable}
	}
	name := remoteName(c.definition.ID, operation)
	if !toolExists(tools.Items, name, args) {
		return providers.Response{}, &providers.Failure{Code: core.ErrorCodeInsufficientKey}
	}
	encoded, err := json.Marshal(args)
	if err != nil {
		return providers.Response{}, providers.Invalid()
	}
	result, err := connection.CallTool(ctx, mcpclient.ToolCallRequest{Name: name, Arguments: encoded})
	if err != nil {
		return providers.Response{}, &providers.Failure{Code: core.ErrorCodeOutcomeUnknown, Unknown: write}
	}
	data, err := decodeResult(result, write)
	if err != nil {
		return providers.Response{}, err
	}
	providers.Redact(data, token)
	output := map[string]any{"provider": c.definition.ID, "transport": "mcp", "data": data}
	if c.schemas.Output(operation, output) != nil {
		return providers.Response{}, &providers.Failure{Code: core.ErrorCodeProviderUnavailable, Unknown: write}
	}
	return providers.Response{Value: output}, nil
}

func listed(items []mcpclient.ToolDefinition, name string) bool {
	for _, item := range items {
		if item.Name == name {
			return true
		}
	}
	return false
}
func toolExists(items []mcpclient.ToolDefinition, name string, args map[string]any) bool {
	for _, item := range items {
		if item.Name != name {
			continue
		}
		var document map[string]any
		if json.Unmarshal(item.InputSchema, &document) != nil {
			return false
		}
		compiler := jsonschema.NewCompiler()
		if compiler.AddResource("urn:remote-social", document) != nil {
			return false
		}
		schema, err := compiler.Compile("urn:remote-social")
		if err != nil {
			return false
		}
		encoded, err := json.Marshal(args)
		if err != nil {
			return false
		}
		var normalized any
		if json.Unmarshal(encoded, &normalized) != nil {
			return false
		}
		return schema.Validate(normalized) == nil
	}
	return false
}

func decodeResult(result mcpclient.ToolCallResult, write bool) (map[string]any, error) {
	var raw []byte
	if result.StructuredContent.Present {
		raw = result.StructuredContent.JSON
	} else {
		for _, block := range result.Content {
			if block.Type == "text" {
				var content struct {
					Text string `json:"text"`
				}
				if json.Unmarshal(block.Raw, &content) == nil {
					raw = []byte(content.Text)
					break
				}
			}
		}
	}
	var value map[string]any
	if len(raw) == 0 || len(raw) > maxResultBytes || json.Unmarshal(raw, &value) != nil || value == nil {
		return nil, &providers.Failure{Code: core.ErrorCodeProviderUnavailable, Unknown: write}
	}
	if result.IsError {
		code := providers.Text(value, "code")
		switch code {
		case "invalid_key":
			return nil, &providers.Failure{Code: core.ErrorCodeInvalidKey}
		case "insufficient_key":
			return nil, &providers.Failure{Code: core.ErrorCodeInsufficientKey}
		case "invalid_input":
			return nil, providers.Invalid()
		default:
			return nil, &providers.Failure{Code: core.ErrorCodeOutcomeUnknown, Unknown: write}
		}
	}
	return value, nil
}

var _ providers.KeyDriver = (*Client)(nil)
