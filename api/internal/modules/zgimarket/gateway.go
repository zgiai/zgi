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

func (g *gatewayClient) connect(ctx context.Context, slug, secret string) (*mcpclient.Connection, error) {
	if !marketSlugPattern.MatchString(slug) || secret == "" || strings.ContainsAny(secret, "\r\n") {
		return nil, ErrListingNotBindable
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
	}, mcpclient.Options{HTTPClient: g.http, HTTPAuth: mcpclient.HTTPAuthConfig{Mode: mcpclient.AuthBearer, Credentials: credentials}})
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
	connection, err := g.connect(ctx, slug, secret)
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
	connection, err := g.connect(ctx, slug, secret)
	if err != nil {
		return nil, err
	}
	defer closeQuietly(connection)
	result, err := connection.CallTool(ctx, mcpclient.ToolCallRequest{Name: tool, Arguments: encoded})
	if err != nil {
		return nil, err
	}
	return &GatewayResult{Text: resultText(result), IsError: result.IsError}, nil
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
