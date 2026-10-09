package mcpclient

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

func (c *Connection) ListToolsPage(ctx context.Context, cursor *string) (Page[ToolDefinition], error) {
	var zero Page[ToolDefinition]
	if err := c.require("tools"); err != nil {
		return zero, err
	}
	ctx = context.WithValue(ctx, rawCacheKey{}, pageCacheKey("tools/list", cursor))
	var accepted map[string]bool
	raw, err := c.operation(ctx, "tools/list", func(op context.Context) (json.RawMessage, error) {
		op = context.WithValue(op, cursorKey{}, cursor)
		params := &mcp.ListToolsParams{}
		if cursor != nil {
			params.Cursor = *cursor
		}
		result, e := c.session.ListTools(op, params)
		if e == nil {
			accepted = make(map[string]bool, len(result.Tools))
			for _, item := range result.Tools {
				accepted[item.Name] = true
			}
		}
		return nil, e
	})
	if err != nil {
		return zero, err
	}
	page, items, err := parsePage(raw, "tools", func(item json.RawMessage) (ToolDefinition, error) {
		fields, e := rawObject(item)
		if e != nil {
			return ToolDefinition{}, e
		}
		var name, title, description string
		_ = json.Unmarshal(fields["name"], &name)
		_ = json.Unmarshal(fields["title"], &title)
		_ = json.Unmarshal(fields["description"], &description)
		if name == "" {
			return ToolDefinition{}, errors.New("MCP tool name is missing")
		}
		return ToolDefinition{Name: name, Title: title, Description: description, InputSchema: cloneRaw(fields["inputSchema"]), OutputSchema: rawValue(fields, "outputSchema"), Annotations: rawValue(fields, "annotations"), Raw: cloneRaw(item)}, nil
	})
	if err != nil {
		return zero, classify("tools/list", err, false)
	}
	page.Items = make([]ToolDefinition, 0, len(items))
	for _, item := range items {
		if accepted[item.Name] {
			page.Items = append(page.Items, item)
		}
	}
	return page, nil
}

func (c *Connection) RefreshTools(ctx context.Context) (ToolCatalog, error) {
	startVersion := c.catalogChanges.Load()
	var collected []ToolDefinition
	var cursor *string
	seen := map[string]bool{}
	for pageNumber := 0; pageNumber < c.config.Limits.MaxCatalogPages; pageNumber++ {
		page, err := c.ListToolsPage(ctx, cursor)
		if err != nil {
			return ToolCatalog{}, err
		}
		collected = append(collected, page.Items...)
		if len(collected) > c.config.Limits.MaxCatalogItems {
			return ToolCatalog{}, &MCPError{Kind: ErrLimitExceeded, Operation: "tools refresh"}
		}
		if page.NextCursor == nil {
			c.catalogMu.Lock()
			c.catalog.Generation++
			c.catalog.Items = append([]ToolDefinition(nil), collected...)
			c.catalog.Stale = c.catalogChanges.Load() != startVersion
			result := copyCatalog(c.catalog)
			c.catalogMu.Unlock()
			c.stale.Store(result.Stale)
			return result, nil
		}
		if seen[*page.NextCursor] {
			return ToolCatalog{}, &MCPError{Kind: ErrProtocol, Operation: "tools refresh", Cause: errors.New("repeated pagination cursor")}
		}
		seen[*page.NextCursor] = true
		cursor = page.NextCursor
	}
	return ToolCatalog{}, &MCPError{Kind: ErrLimitExceeded, Operation: "tools refresh", Cause: errors.New("too many pages")}
}
func (c *Connection) ToolCatalog() ToolCatalog {
	c.catalogMu.RLock()
	defer c.catalogMu.RUnlock()
	result := c.catalog
	result = copyCatalog(result)
	result.Stale = c.stale.Load()
	return result
}
func copyCatalog(source ToolCatalog) ToolCatalog {
	out := source
	out.Items = make([]ToolDefinition, len(source.Items))
	for i, item := range source.Items {
		item.InputSchema = cloneRaw(item.InputSchema)
		item.OutputSchema.JSON = cloneRaw(item.OutputSchema.JSON)
		item.Annotations.JSON = cloneRaw(item.Annotations.JSON)
		item.Raw = cloneRaw(item.Raw)
		out.Items[i] = item
	}
	return out
}

func (c *Connection) ListResourcesPage(ctx context.Context, cursor *string) (Page[ResourceDefinition], error) {
	var zero Page[ResourceDefinition]
	if err := c.require("resources"); err != nil {
		return zero, err
	}
	ctx = context.WithValue(ctx, rawCacheKey{}, pageCacheKey("resources/list", cursor))
	raw, err := c.operation(ctx, "resources/list", func(op context.Context) (json.RawMessage, error) {
		op = context.WithValue(op, cursorKey{}, cursor)
		params := &mcp.ListResourcesParams{}
		if cursor != nil {
			params.Cursor = *cursor
		}
		_, e := c.session.ListResources(op, params)
		return nil, e
	})
	if err != nil {
		return zero, err
	}
	page, items, err := parsePage(raw, "resources", func(item json.RawMessage) (ResourceDefinition, error) {
		var v struct{ URI, Name string }
		if err := json.Unmarshal(item, &v); err != nil {
			return ResourceDefinition{}, err
		}
		return ResourceDefinition{URI: v.URI, Name: v.Name, Raw: cloneRaw(item)}, nil
	})
	if err != nil {
		return zero, classify("resources/list", err, false)
	}
	page.Items = items
	return page, nil
}
func (c *Connection) ListResourceTemplatesPage(ctx context.Context, cursor *string) (Page[ResourceTemplate], error) {
	var zero Page[ResourceTemplate]
	if err := c.require("resources"); err != nil {
		return zero, err
	}
	ctx = context.WithValue(ctx, rawCacheKey{}, pageCacheKey("resources/templates/list", cursor))
	raw, err := c.operation(ctx, "resources/templates/list", func(op context.Context) (json.RawMessage, error) {
		op = context.WithValue(op, cursorKey{}, cursor)
		params := &mcp.ListResourceTemplatesParams{}
		if cursor != nil {
			params.Cursor = *cursor
		}
		_, e := c.session.ListResourceTemplates(op, params)
		return nil, e
	})
	if err != nil {
		return zero, err
	}
	page, items, err := parsePage(raw, "resourceTemplates", func(item json.RawMessage) (ResourceTemplate, error) {
		var v struct {
			URITemplate string `json:"uriTemplate"`
			Name        string `json:"name"`
		}
		if err := json.Unmarshal(item, &v); err != nil {
			return ResourceTemplate{}, err
		}
		return ResourceTemplate{URITemplate: v.URITemplate, Name: v.Name, Raw: cloneRaw(item)}, nil
	})
	if err != nil {
		return zero, classify("resources/templates/list", err, false)
	}
	page.Items = items
	return page, nil
}
func (c *Connection) ListPromptsPage(ctx context.Context, cursor *string) (Page[PromptDefinition], error) {
	var zero Page[PromptDefinition]
	if err := c.require("prompts"); err != nil {
		return zero, err
	}
	ctx = context.WithValue(ctx, rawCacheKey{}, pageCacheKey("prompts/list", cursor))
	raw, err := c.operation(ctx, "prompts/list", func(op context.Context) (json.RawMessage, error) {
		op = context.WithValue(op, cursorKey{}, cursor)
		params := &mcp.ListPromptsParams{}
		if cursor != nil {
			params.Cursor = *cursor
		}
		_, e := c.session.ListPrompts(op, params)
		return nil, e
	})
	if err != nil {
		return zero, err
	}
	page, items, err := parsePage(raw, "prompts", func(item json.RawMessage) (PromptDefinition, error) {
		var v struct{ Name, Description string }
		if err := json.Unmarshal(item, &v); err != nil {
			return PromptDefinition{}, err
		}
		return PromptDefinition{Name: v.Name, Description: v.Description, Raw: cloneRaw(item)}, nil
	})
	if err != nil {
		return zero, classify("prompts/list", err, false)
	}
	page.Items = items
	return page, nil
}

func (c *Connection) CallTool(ctx context.Context, req ToolCallRequest) (ToolCallResult, error) {
	var zero ToolCallResult
	if err := c.require("tools"); err != nil {
		return zero, err
	}
	if strings.TrimSpace(req.Name) == "" {
		return zero, &MCPError{Kind: ErrConfig, Operation: "tools/call", Cause: errors.New("tool name is required")}
	}
	args := req.Arguments
	if len(args) == 0 {
		args = []byte(`{}`)
	}
	var object map[string]json.RawMessage
	if json.Unmarshal(args, &object) != nil || object == nil {
		return zero, &MCPError{Kind: ErrConfig, Operation: "tools/call", Cause: errors.New("arguments must be a JSON object")}
	}
	if err := c.validateContinuation(req.Continuation); err != nil {
		return zero, err
	}
	token := req.ProgressToken
	if token == "" {
		token = fmt.Sprintf("%s-%d", c.config.ID, c.opSeq.Add(1))
	}
	raw, err := c.operation(ctx, "tools/call", func(op context.Context) (json.RawMessage, error) {
		op = context.WithValue(op, continuationKey{}, req.Continuation)
		params := &mcp.CallToolParams{Name: req.Name, Arguments: args}
		params.SetProgressToken(token)
		_, e := c.session.CallTool(op, params)
		return nil, e
	})
	if err != nil {
		return zero, err
	}
	fields, err := rawObject(raw)
	if err != nil {
		return zero, classify("tools/call", err, false)
	}
	result := ToolCallResult{OperationID: token, ResultType: resultType(fields), StructuredContent: rawValue(fields, "structuredContent"), RawResult: cloneRaw(raw)}
	_ = json.Unmarshal(fields["isError"], &result.IsError)
	result.Content, err = parseContentBlocks(fields["content"])
	if err != nil {
		return zero, classify("tools/call", err, false)
	}
	result.InputRequired = parseInputRequired(fields, raw)
	return result, nil
}

func (c *Connection) ReadResource(ctx context.Context, req ResourceReadRequest) (ResourceReadResult, error) {
	var zero ResourceReadResult
	if err := c.require("resources"); err != nil {
		return zero, err
	}
	if req.URI == "" {
		return zero, &MCPError{Kind: ErrConfig, Operation: "resources/read"}
	}
	ctx = context.WithValue(ctx, rawCacheKey{}, "resources/read:"+req.URI)
	if err := c.validateContinuation(req.Continuation); err != nil {
		return zero, err
	}
	raw, err := c.operation(ctx, "resources/read", func(op context.Context) (json.RawMessage, error) {
		op = context.WithValue(op, continuationKey{}, req.Continuation)
		_, e := c.session.ReadResource(op, &mcp.ReadResourceParams{URI: req.URI})
		return nil, e
	})
	if err != nil {
		return zero, err
	}
	fields, err := rawObject(raw)
	if err != nil {
		return zero, classify("resources/read", err, false)
	}
	result := ResourceReadResult{ResultType: resultType(fields), RawResult: cloneRaw(raw), InputRequired: parseInputRequired(fields, raw)}
	var items []json.RawMessage
	if len(fields["contents"]) > 0 {
		if err := json.Unmarshal(fields["contents"], &items); err != nil {
			return zero, classify("resources/read", err, false)
		}
	}
	for _, item := range items {
		var v struct {
			URI      string `json:"uri"`
			MIMEType string `json:"mimeType"`
		}
		_ = json.Unmarshal(item, &v)
		result.Contents = append(result.Contents, ResourceContent{URI: v.URI, MIMEType: v.MIMEType, Raw: cloneRaw(item)})
	}
	return result, nil
}
func (c *Connection) GetPrompt(ctx context.Context, req PromptGetRequest) (PromptGetResult, error) {
	var zero PromptGetResult
	if err := c.require("prompts"); err != nil {
		return zero, err
	}
	if req.Name == "" {
		return zero, &MCPError{Kind: ErrConfig, Operation: "prompts/get"}
	}
	if err := c.validateContinuation(req.Continuation); err != nil {
		return zero, err
	}
	raw, err := c.operation(ctx, "prompts/get", func(op context.Context) (json.RawMessage, error) {
		op = context.WithValue(op, continuationKey{}, req.Continuation)
		_, e := c.session.GetPrompt(op, &mcp.GetPromptParams{Name: req.Name, Arguments: req.Arguments})
		return nil, e
	})
	if err != nil {
		return zero, err
	}
	fields, err := rawObject(raw)
	if err != nil {
		return zero, classify("prompts/get", err, false)
	}
	result := PromptGetResult{ResultType: resultType(fields), RawResult: cloneRaw(raw), InputRequired: parseInputRequired(fields, raw)}
	_ = json.Unmarshal(fields["description"], &result.Description)
	var messages []json.RawMessage
	if len(fields["messages"]) > 0 {
		if err := json.Unmarshal(fields["messages"], &messages); err != nil {
			return zero, classify("prompts/get", err, false)
		}
	}
	for _, item := range messages {
		parts, _ := rawObject(item)
		var role string
		_ = json.Unmarshal(parts["role"], &role)
		content, err := parseContentBlocks(parts["content"])
		if err != nil {
			return zero, classify("prompts/get", err, false)
		}
		result.Messages = append(result.Messages, PromptMessage{Role: role, Contents: content, Raw: cloneRaw(item)})
	}
	return result, nil
}
func (c *Connection) validateContinuation(cont *Continuation) error {
	if cont != nil && (cont.Round < 1 || cont.Round > c.config.Limits.MaxInteractionRounds) {
		return &MCPError{Kind: ErrLimitExceeded, Operation: "interaction continuation"}
	}
	return nil
}

func parsePage[T any](raw json.RawMessage, key string, parse func(json.RawMessage) (T, error)) (Page[T], []T, error) {
	var page Page[T]
	fields, err := rawObject(raw)
	if err != nil {
		return page, nil, err
	}
	page.RawResult = cloneRaw(raw)
	if cursor, ok := fields["nextCursor"]; ok && string(cursor) != "null" {
		var value string
		if err := json.Unmarshal(cursor, &value); err != nil {
			return page, nil, err
		}
		page.NextCursor = &value
	}
	if ttl, ok := fields["ttlMs"]; ok {
		var millis float64
		if json.Unmarshal(ttl, &millis) == nil {
			d := time.Duration(millis * float64(time.Millisecond))
			page.TTL = &d
		}
	}
	_ = json.Unmarshal(fields["cacheScope"], &page.CacheScope)
	var rawItems []json.RawMessage
	if err := json.Unmarshal(fields[key], &rawItems); err != nil {
		return page, nil, fmt.Errorf("decode %s: %w", key, err)
	}
	items := make([]T, 0, len(rawItems))
	for _, rawItem := range rawItems {
		item, err := parse(rawItem)
		if err != nil {
			return page, nil, err
		}
		items = append(items, item)
	}
	return page, items, nil
}
func rawObject(raw json.RawMessage) (map[string]json.RawMessage, error) {
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(raw, &fields); err != nil || fields == nil {
		return nil, errors.New("MCP result must be an object")
	}
	return fields, nil
}
func cloneRaw(raw json.RawMessage) json.RawMessage { return append(json.RawMessage(nil), raw...) }
func pageCacheKey(method string, cursor *string) string {
	if cursor == nil {
		return method + ":<first>"
	}
	return method + ":" + *cursor
}
func rawValue(fields map[string]json.RawMessage, key string) RawValue {
	raw, ok := fields[key]
	return RawValue{Present: ok, JSON: cloneRaw(raw)}
}
func resultType(fields map[string]json.RawMessage) string {
	var value string
	_ = json.Unmarshal(fields["resultType"], &value)
	if value == "" {
		return "complete"
	}
	return value
}
func parseInputRequired(fields map[string]json.RawMessage, raw json.RawMessage) *InputRequired {
	if resultType(fields) != "input_required" {
		return nil
	}
	result := &InputRequired{Requests: make(map[string]json.RawMessage), RequestState: rawValue(fields, "requestState"), Raw: cloneRaw(raw)}
	_ = json.Unmarshal(fields["inputRequests"], &result.Requests)
	return result
}
func parseContentBlocks(raw json.RawMessage) ([]ContentBlock, error) {
	if len(raw) == 0 || string(raw) == "null" {
		return nil, nil
	}
	var items []json.RawMessage
	if raw[0] == '[' {
		if err := json.Unmarshal(raw, &items); err != nil {
			return nil, err
		}
	} else {
		items = []json.RawMessage{raw}
	}
	result := make([]ContentBlock, 0, len(items))
	for _, item := range items {
		var value struct {
			Type string `json:"type"`
		}
		if err := json.Unmarshal(item, &value); err != nil {
			return nil, err
		}
		result = append(result, ContentBlock{Type: value.Type, Raw: cloneRaw(item)})
	}
	return result, nil
}
