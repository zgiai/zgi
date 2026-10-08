package mcpclient

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/url"
	"strings"
	"sync"
	"sync/atomic"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

type Client struct {
	config  ServerConfig
	options Options
}

func NewClient(config ServerConfig, options Options) (*Client, error) {
	if strings.TrimSpace(config.ID) == "" {
		return nil, &MCPError{Kind: ErrConfig, Operation: "new client", Cause: errors.New("server ID is required")}
	}
	switch config.Transport {
	case TransportStdio:
		if config.Process == nil || strings.TrimSpace(config.Process.Command) == "" || config.Endpoint != "" {
			return nil, &MCPError{Kind: ErrConfig, Operation: "new client", Cause: errors.New("stdio requires a process and no endpoint")}
		}
		for key, value := range config.Process.Env {
			if key == "" || strings.ContainsAny(key, "=\x00") || strings.ContainsRune(value, '\x00') {
				return nil, &MCPError{Kind: ErrConfig, Operation: "new client", Cause: errors.New("invalid stdio environment entry")}
			}
		}
		if (options.HTTPAuth.Mode != "" && options.HTTPAuth.Mode != AuthNone) || options.HTTPAuth.Credentials != nil || options.HTTPAuth.OAuth != nil {
			return nil, &MCPError{Kind: ErrConfig, Operation: "new client", Cause: errors.New("HTTP auth cannot be used with stdio")}
		}
	case TransportStreamableHTTP:
		u, err := url.Parse(config.Endpoint)
		if err != nil || u.Host == "" || (u.Scheme != "http" && u.Scheme != "https") || u.User != nil || u.Fragment != "" || config.Process != nil {
			return nil, &MCPError{Kind: ErrConfig, Operation: "new client", Cause: errors.New("invalid HTTP endpoint or process")}
		}
	default:
		return nil, &MCPError{Kind: ErrConfig, Operation: "new client", Cause: errors.New("unsupported transport")}
	}
	config.Limits = config.Limits.normalized()
	if config.Process != nil {
		process := *config.Process
		process.Args = append([]string(nil), process.Args...)
		process.Env = make(map[string]string, len(config.Process.Env))
		for key, value := range config.Process.Env {
			process.Env[key] = value
		}
		config.Process = &process
	}
	if options.HTTPAuth.OAuth != nil {
		oauth := *options.HTTPAuth.OAuth
		if oauth.DynamicRegistration != nil {
			registration := *oauth.DynamicRegistration
			registration.RedirectURIs = append([]string(nil), registration.RedirectURIs...)
			registration.GrantTypes = append([]string(nil), registration.GrantTypes...)
			oauth.DynamicRegistration = &registration
		}
		options.HTTPAuth.OAuth = &oauth
	}
	return &Client{config: config, options: options}, nil
}

type Connection struct {
	config         ServerConfig
	info           ConnectionInfo
	session        *mcp.ClientSession
	process        Process
	recorder       *captureTransport
	httpBoundary   *httpBoundary
	events         chan Event
	done           chan struct{}
	sem            chan struct{}
	mu             sync.RWMutex
	closed         bool
	terminalErr    error
	closeOnce      sync.Once
	eventMu        sync.Mutex
	stale          atomic.Bool
	catalogChanges atomic.Uint64
	opSeq          atomic.Uint64
	eventDropped   atomic.Uint64
	catalogMu      sync.RWMutex
	catalog        ToolCatalog
	rawCacheMu     sync.Mutex
	rawCache       map[string]json.RawMessage
	transportErrMu sync.Mutex
	transportErr   error
}

func (c *Client) Connect(ctx context.Context) (*Connection, error) {
	limits := c.config.Limits
	connectCtx, cancel := context.WithTimeout(ctx, limits.ConnectTimeout)
	defer cancel()
	buffer := limits.EventBuffer
	if c.options.EventBuffer > 0 {
		buffer = c.options.EventBuffer
	}
	conn := &Connection{config: c.config, events: make(chan Event, buffer), done: make(chan struct{}), sem: make(chan struct{}, limits.MaxConcurrentRequests)}
	var transport mcp.Transport
	if c.config.Transport == TransportStdio {
		launcher := c.options.ProcessLauncher
		if launcher == nil {
			launcher = LocalProcessLauncher{}
		}
		spec := *c.config.Process
		if spec.ShutdownGrace <= 0 {
			spec.ShutdownGrace = limits.CloseGrace
		}
		process, err := launcher.Launch(connectCtx, spec)
		if err != nil {
			return nil, classify("connect", err, false)
		}
		conn.process = process
		go conn.drainStderr(process.Stderr())
		transport = &mcp.IOTransport{Reader: &lineLimitedReader{ReadCloser: process.Stdout(), limit: limits.MaxMessageBytes, onLimit: conn.setTransportError}, Writer: process.Stdin()}
	} else {
		httpClient, boundary, err := configuredHTTPClient(c.options, c.config.Limits)
		if err != nil {
			return nil, err
		}
		boundary.onNotification = conn.notification
		conn.httpBoundary = boundary
		streamable := &mcp.StreamableClientTransport{Endpoint: c.config.Endpoint, HTTPClient: httpClient, MaxRetries: 3}
		if c.options.HTTPAuth.Mode == AuthOAuth {
			handler, err := newOAuthHandler(connectCtx, c.options.HTTPAuth.OAuth, httpClient)
			if err != nil {
				return nil, err
			}
			streamable.OAuthHandler = handler
		}
		transport = streamable
	}
	if c.config.Transport == TransportStdio {
		conn.recorder = &captureTransport{base: transport, maxMessageBytes: limits.MaxMessageBytes, onNotification: conn.notification}
		transport = conn.recorder
	}
	clientOptions := &mcp.ClientOptions{
		Capabilities:   &mcp.ClientCapabilities{},
		MultiRoundTrip: &mcp.MultiRoundTripOptions{Disabled: true},
	}
	if c.options.EnableCatalogNotifications {
		clientOptions.ToolListChangedHandler = func(context.Context, *mcp.ToolListChangedRequest) { conn.stale.Store(true) }
		clientOptions.PromptListChangedHandler = func(context.Context, *mcp.PromptListChangedRequest) {}
		clientOptions.ResourceListChangedHandler = func(context.Context, *mcp.ResourceListChangedRequest) {}
	}
	if c.options.InteractionHandler != nil {
		clientOptions.ElicitationHandler = func(ctx context.Context, request *mcp.ElicitRequest) (*mcp.ElicitResult, error) {
			encoded, _ := json.Marshal(request)
			response, err := c.options.InteractionHandler.Handle(ctx, InteractionRequest{Method: "elicitation/create", Raw: encoded})
			if err != nil {
				return nil, err
			}
			var result mcp.ElicitResult
			if err := json.Unmarshal(response.Raw, &result); err != nil {
				return nil, err
			}
			return &result, nil
		}
	}
	sdkClient := mcp.NewClient(&mcp.Implementation{Name: "zgi-mcpclient", Version: "0.1.0"}, clientOptions)
	session, err := sdkClient.Connect(connectCtx, transport, nil)
	if err != nil {
		if boundary := conn.getTransportError(); boundary != nil {
			err = boundary
		}
		if conn.recorder != nil {
			if boundary := conn.recorder.boundaryError(); boundary != nil {
				err = boundary
			}
		}
		if conn.httpBoundary != nil {
			if boundary := conn.httpBoundary.protocolError(); boundary != nil {
				err = boundary
			}
		}
		if conn.process != nil {
			_ = conn.process.Stdin().Close()
			closeCtx, done := context.WithTimeout(context.Background(), limits.CloseGrace)
			_ = conn.process.Terminate(closeCtx)
			done()
		}
		return nil, classify("connect", err, false)
	}
	conn.session = session
	init := session.InitializeResult()
	if init != nil {
		var discovery json.RawMessage
		if conn.recorder != nil {
			discovery = conn.recorder.discoveryResult()
		} else if conn.httpBoundary != nil {
			discovery = conn.httpBoundary.discoveryResult()
		}
		conn.info = ConnectionInfo{ID: c.config.ID, ProtocolVersion: init.ProtocolVersion, Instructions: init.Instructions, RawDiscovery: discovery}
		if init.ServerInfo != nil {
			encoded, _ := json.Marshal(init.ServerInfo)
			conn.info.ServerInfo = &ServerInfo{Name: init.ServerInfo.Name, Version: init.ServerInfo.Version, Raw: encoded}
		}
		if init.Capabilities != nil {
			encoded, _ := json.Marshal(init.Capabilities)
			conn.info.Capabilities = CapabilitySnapshot{Tools: init.Capabilities.Tools != nil, Resources: init.Capabilities.Resources != nil, Prompts: init.Capabilities.Prompts != nil, Raw: encoded}
		}
	}
	go func() { err := session.Wait(); conn.finish(err) }()
	return conn, nil
}

func (c *Connection) Info() ConnectionInfo {
	copy := c.info
	copy.RawDiscovery = append(json.RawMessage(nil), copy.RawDiscovery...)
	copy.Capabilities.Raw = append(json.RawMessage(nil), copy.Capabilities.Raw...)
	if copy.ServerInfo != nil {
		info := *copy.ServerInfo
		info.Raw = append(json.RawMessage(nil), info.Raw...)
		copy.ServerInfo = &info
	}
	return copy
}
func (c *Connection) Events() <-chan Event  { return c.events }
func (c *Connection) Done() <-chan struct{} { return c.done }
func (c *Connection) Err() error            { c.mu.RLock(); defer c.mu.RUnlock(); return c.terminalErr }

func (c *Connection) finish(err error) {
	c.closeOnce.Do(func() {
		c.mu.Lock()
		c.closed = true
		c.terminalErr = err
		c.mu.Unlock()
		c.emit(Event{Kind: EventDisconnected, Diagnostic: "connection closed"})
		close(c.done)
		c.eventMu.Lock()
		close(c.events)
		c.eventMu.Unlock()
	})
}
func (c *Connection) Close(ctx context.Context) error {
	c.mu.Lock()
	c.closed = true
	c.mu.Unlock()
	closed := make(chan error, 1)
	go func() { closed <- c.session.Close() }()
	var err error
	select {
	case err = <-closed:
	case <-ctx.Done():
		err = ctx.Err()
	}
	if c.process != nil {
		_ = c.process.Stdin().Close()
		graceCtx, cancel := context.WithTimeout(ctx, c.config.Limits.CloseGrace)
		if waitErr := c.process.Wait(graceCtx); waitErr != nil {
			killCtx, killCancel := context.WithTimeout(context.Background(), c.config.Limits.CloseGrace)
			_ = c.process.Terminate(killCtx)
			killCancel()
		}
		cancel()
	}
	select {
	case <-c.done:
	case <-ctx.Done():
		if err == nil {
			err = ctx.Err()
		}
	}
	return err
}
func (c *Connection) emit(event Event) {
	c.eventMu.Lock()
	defer c.eventMu.Unlock()
	select {
	case <-c.done:
		return
	default:
	}
	if dropped := c.eventDropped.Load(); dropped > 0 {
		select {
		case c.events <- Event{Kind: EventDiagnostic, Category: "events_dropped", Diagnostic: fmt.Sprintf("%d events were coalesced or dropped", dropped)}:
			c.eventDropped.Store(0)
		default:
		}
	}
	select {
	case c.events <- event:
	default: /* A slow observer must not block MCP. */
		c.eventDropped.Add(1)
	}
}
func (c *Connection) drainStderr(reader io.Reader) {
	buf := make([]byte, 4096)
	reported := false
	for {
		n, err := reader.Read(buf)
		if n > 0 && !reported {
			c.emit(Event{Kind: EventDiagnostic, Category: "stdio_stderr", Diagnostic: "MCP server wrote to stderr"})
			reported = true
		}
		if err != nil {
			return
		}
	}
}
func (c *Connection) notification(method string, raw json.RawMessage) {
	switch method {
	case "notifications/tools/list_changed":
		c.stale.Store(true)
		c.catalogChanges.Add(1)
		c.rawCacheMu.Lock()
		for key := range c.rawCache {
			if strings.HasPrefix(key, "tools/list:") {
				delete(c.rawCache, key)
			}
		}
		c.rawCacheMu.Unlock()
		c.emit(Event{Kind: EventCatalogChanged, Category: "tools", Raw: raw})
	case "notifications/resources/list_changed":
		c.emit(Event{Kind: EventCatalogChanged, Category: "resources", Raw: raw})
	case "notifications/prompts/list_changed":
		c.emit(Event{Kind: EventCatalogChanged, Category: "prompts", Raw: raw})
	case "notifications/resources/updated":
		c.emit(Event{Kind: EventResourceUpdated, Raw: raw})
	case "notifications/progress":
		var fields map[string]json.RawMessage
		_ = json.Unmarshal(raw, &fields)
		var token string
		_ = json.Unmarshal(fields["progressToken"], &token)
		c.emit(Event{Kind: EventProgress, OperationID: token, Raw: raw})
	}
}
func (c *Connection) operation(ctx context.Context, name string, fn func(context.Context) (json.RawMessage, error)) (json.RawMessage, error) {
	c.mu.RLock()
	closed := c.closed
	c.mu.RUnlock()
	if closed {
		return nil, &MCPError{Kind: ErrClosed, Operation: name}
	}
	select {
	case c.sem <- struct{}{}:
		defer func() { <-c.sem }()
	case <-ctx.Done():
		return nil, classify(name, ctx.Err(), false)
	case <-c.done:
		return nil, &MCPError{Kind: ErrClosed, Operation: name}
	}
	opCtx, cancel := context.WithTimeout(ctx, c.config.Limits.OperationTimeout)
	defer cancel()
	slot := &captureSlot{}
	opCtx = context.WithValue(opCtx, captureKey{}, slot)
	raw, err := fn(opCtx)
	if err != nil {
		if boundary := slot.boundaryError(); boundary != nil {
			err = boundary
		}
		if boundary := c.getTransportError(); boundary != nil {
			err = boundary
		}
		return nil, classify(name, err, slot.wasSent())
	}
	if len(raw) == 0 {
		raw = slot.result()
	}
	if key, ok := ctx.Value(rawCacheKey{}).(string); ok && key != "" {
		c.rawCacheMu.Lock()
		if len(raw) == 0 {
			raw = cloneRaw(c.rawCache[key])
		} else {
			if c.rawCache == nil {
				c.rawCache = make(map[string]json.RawMessage)
			}
			if len(c.rawCache) >= 1000 {
				for existing := range c.rawCache {
					delete(c.rawCache, existing)
					break
				}
			}
			c.rawCache[key] = cloneRaw(raw)
		}
		c.rawCacheMu.Unlock()
	}
	if len(raw) == 0 {
		return nil, &MCPError{Kind: ErrProtocol, Operation: name, Cause: errors.New("missing raw MCP result")}
	}
	return raw, nil
}
func (c *Connection) setTransportError(err error) {
	c.transportErrMu.Lock()
	c.transportErr = err
	c.transportErrMu.Unlock()
}
func (c *Connection) getTransportError() error {
	c.transportErrMu.Lock()
	defer c.transportErrMu.Unlock()
	return c.transportErr
}
func (c *Connection) require(capability string) error {
	var available bool
	switch capability {
	case "tools":
		available = c.info.Capabilities.Tools
	case "resources":
		available = c.info.Capabilities.Resources
	case "prompts":
		available = c.info.Capabilities.Prompts
	}
	if !available {
		return &MCPError{Kind: ErrUnsupportedCapability, Operation: capability}
	}
	return nil
}
func (c *Connection) String() string {
	return fmt.Sprintf("mcp connection %s (%s)", c.config.ID, c.info.ProtocolVersion)
}
