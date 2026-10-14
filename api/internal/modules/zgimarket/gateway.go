package zgimarket

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/zgiai/zgi/api/pkg/mcpclient"
)

const (
	gatewayMCPPath        = "/v1/mcp/"
	gatewayConnectTimeout = 15 * time.Second
	gatewayCallTimeout    = 60 * time.Second
	gatewayMessageBytes   = 4 << 20
	// maxToolResultChars bounds the text of one tool result handed to the model.
	maxToolResultChars = 64 * 1024
)

// GatewayConfig is where the market gateway is reached.
type GatewayConfig struct {
	// URL is the public origin of the gateway, https outside local development.
	URL string
	// AllowPrivateTargets lets the gateway resolve to a private or loopback address.
	AllowPrivateTargets bool
	// ConfirmAbovePoints is the cost of one call, in points, above which a tool asks for approval
	// whatever it does. Zero never asks for cost.
	ConfirmAbovePoints int
}

// GatewayTool is a tool the gateway lists for a bound listing.
type GatewayTool struct {
	Name        string
	Description string
	InputSchema map[string]any
}

// GatewayResult is the answer of one tool call.
type GatewayResult struct {
	Text    string
	IsError bool
	// RequestID is the X-Request-Id the gateway answered the call with: what the market records
	// and charges the call under. Empty when the gateway named none.
	RequestID string
}

// requestIDHeader is the header the gateway names a call by.
const requestIDHeader = "X-Request-Id"

// lastRequestID keeps the request id of the latest POST one connection to the gateway answered.
// A connection sends one JSON-RPC request at a time and the tool call is its last POST, so after
// the call this is the call's own id rather than the handshake's.
type lastRequestID struct {
	next http.RoundTripper
	mu   sync.Mutex
	id   string
}

func (l *lastRequestID) RoundTrip(request *http.Request) (*http.Response, error) {
	response, err := l.next.RoundTrip(request)
	if response != nil && request.Method == http.MethodPost {
		if id := response.Header.Get(requestIDHeader); id != "" {
			l.mu.Lock()
			l.id = id
			l.mu.Unlock()
		}
	}
	return response, err
}

func (l *lastRequestID) value() string {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.id
}

type gatewayClient struct {
	origin string
	http   *http.Client
}

var errPrivateGateway = errors.New("the market gateway resolves to a private address")

func newGatewayClient(config GatewayConfig) (*gatewayClient, error) {
	target, err := url.Parse(strings.TrimRight(strings.TrimSpace(config.URL), "/"))
	if err != nil || target.Host == "" || target.User != nil || target.RawQuery != "" || target.Fragment != "" ||
		(target.Scheme != "https" && !(target.Scheme == "http" && config.AllowPrivateTargets)) {
		return nil, fmt.Errorf("ZGI_MARKET_GATEWAY_URL must be an https origin without credentials, query, or fragment")
	}
	dialer := &net.Dialer{Timeout: gatewayConnectTimeout}
	if !config.AllowPrivateTargets {
		// The address actually connected is checked, so a name that resolves elsewhere later
		// cannot redirect the credential to an internal service.
		dialer.Control = func(_, address string, _ syscall.RawConn) error {
			host, _, err := net.SplitHostPort(address)
			if err != nil {
				return errPrivateGateway
			}
			ip := net.ParseIP(host)
			if ip == nil || ip.IsLoopback() || ip.IsPrivate() || ip.IsLinkLocalUnicast() || ip.IsLinkLocalMulticast() ||
				ip.IsUnspecified() || ip.IsMulticast() || ip.IsInterfaceLocalMulticast() {
				return errPrivateGateway
			}
			return nil
		}
	}
	transport := &http.Transport{
		Proxy:                 nil,
		DialContext:           dialer.DialContext,
		TLSHandshakeTimeout:   gatewayConnectTimeout,
		ResponseHeaderTimeout: gatewayCallTimeout,
		MaxIdleConnsPerHost:   8,
		IdleConnTimeout:       90 * time.Second,
	}
	return &gatewayClient{
		origin: target.Scheme + "://" + target.Host + strings.TrimRight(target.Path, "/"),
		http: &http.Client{
			Transport: transport,
			// The gateway answers MCP requests itself; a redirect would carry the credential away.
			CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse },
		},
	}, nil
}

// connect opens an MCP connection to a bound listing. ids, when given, keeps the request id of
// each answer.
func (g *gatewayClient) connect(ctx context.Context, slug, secret string, ids *lastRequestID) (*mcpclient.Connection, error) {
	if !marketSlugPattern.MatchString(slug) || secret == "" || strings.ContainsAny(secret, "\r\n") {
		return nil, ErrListingNotBindable
	}
	httpClient := g.http
	if ids != nil {
		ids.next = g.http.Transport
		httpClient = &http.Client{Transport: ids, CheckRedirect: g.http.CheckRedirect}
	}
	credentials := mcpclient.CredentialProviderFunc(func(context.Context, *url.URL) (http.Header, error) {
		return http.Header{"Authorization": []string{"Bearer " + secret}}, nil
	})
	client, err := mcpclient.NewClient(mcpclient.ServerConfig{
		ID: "zgi-market-" + slug, Transport: mcpclient.TransportStreamableHTTP, Endpoint: g.origin + gatewayMCPPath + slug,
		Limits: mcpclient.Limits{
			ConnectTimeout: gatewayConnectTimeout, OperationTimeout: gatewayCallTimeout,
			MaxMessageBytes: gatewayMessageBytes, MaxConcurrentRequests: 2, MaxCatalogItems: 200,
		},
	}, mcpclient.Options{HTTPClient: httpClient, HTTPAuth: mcpclient.HTTPAuthConfig{Mode: mcpclient.AuthBearer, Credentials: credentials}})
	if err != nil {
		return nil, err
	}
	return client.Connect(ctx)
}

func closeQuietly(connection *mcpclient.Connection) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	_ = connection.Close(ctx)
}

// ListTools lists the tools of a bound listing.
func (g *gatewayClient) ListTools(ctx context.Context, slug, secret string) ([]GatewayTool, error) {
	connection, err := g.connect(ctx, slug, secret, nil)
	if err != nil {
		return nil, err
	}
	defer closeQuietly(connection)
	catalog, err := connection.RefreshTools(ctx)
	if err != nil {
		return nil, err
	}
	tools := make([]GatewayTool, 0, len(catalog.Items))
	for _, item := range catalog.Items {
		tool := GatewayTool{Name: item.Name, Description: strings.TrimSpace(item.Description)}
		if len(item.InputSchema) > 0 {
			var schema map[string]any
			if json.Unmarshal(item.InputSchema, &schema) == nil {
				tool.InputSchema = schema
			}
		}
		if tool.InputSchema == nil {
			tool.InputSchema = map[string]any{"type": "object", "properties": map[string]any{}}
		}
		tools = append(tools, tool)
	}
	return tools, nil
}

// CallTool calls one tool of a bound listing. The market bills the call.
func (g *gatewayClient) CallTool(ctx context.Context, slug, secret, tool string, arguments map[string]any) (*GatewayResult, error) {
	if !toolNamePattern.MatchString(tool) {
		return nil, ErrListingNotBindable
	}
	if arguments == nil {
		arguments = map[string]any{}
	}
	encoded, err := json.Marshal(arguments)
	if err != nil {
		return nil, err
	}
	ids := &lastRequestID{}
	connection, err := g.connect(ctx, slug, secret, ids)
	if err != nil {
		return nil, err
	}
	defer closeQuietly(connection)
	result, err := connection.CallTool(ctx, mcpclient.ToolCallRequest{Name: tool, Arguments: encoded})
	if err != nil {
		return &GatewayResult{RequestID: ids.value()}, err
	}
	return &GatewayResult{Text: resultText(result), IsError: result.IsError, RequestID: ids.value()}, nil
}

// resultText joins the text content of a tool result, falling back to its structured content.
func resultText(result mcpclient.ToolCallResult) string {
	var parts []string
	for _, block := range result.Content {
		var text struct {
			Type string `json:"type"`
			Text string `json:"text"`
		}
		if json.Unmarshal(block.Raw, &text) == nil && text.Type == "text" {
			parts = append(parts, text.Text)
		} else if len(block.Raw) > 0 {
			parts = append(parts, string(block.Raw))
		}
	}
	if len(parts) == 0 && result.StructuredContent.Present {
		parts = append(parts, string(result.StructuredContent.JSON))
	}
	joined := strings.Join(parts, "\n")
	if len(joined) > maxToolResultChars {
		joined = joined[:maxToolResultChars]
	}
	return joined
}
