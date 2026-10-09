// Package mcpclient provides a host-independent client for MCP servers.
// Configuration, authorization policy, and presentation belong to its callers.
package mcpclient

import (
	"context"
	"encoding/json"
	"errors"
	"github.com/modelcontextprotocol/go-sdk/auth"
	"github.com/modelcontextprotocol/go-sdk/oauthex"
	"golang.org/x/oauth2"
	"io"
	"net/http"
	"net/url"
	"time"
)

type TransportKind string

const (
	TransportStdio          TransportKind = "stdio"
	TransportStreamableHTTP TransportKind = "streamable_http"
)

type ProcessSpec struct {
	Command       string            `json:"command"`
	Args          []string          `json:"args,omitempty"`
	Env           map[string]string `json:"env,omitempty"`
	WorkDir       string            `json:"work_dir,omitempty"`
	ShutdownGrace time.Duration     `json:"-"`
}

type Limits struct {
	ConnectTimeout        time.Duration
	OperationTimeout      time.Duration
	CloseGrace            time.Duration
	MaxConcurrentRequests int
	MaxMessageBytes       int64
	MaxCatalogPages       int
	MaxCatalogItems       int
	MaxInteractionRounds  int
	EventBuffer           int
}

func (l Limits) normalized() Limits {
	if l.ConnectTimeout <= 0 {
		l.ConnectTimeout = 15 * time.Second
	}
	if l.OperationTimeout <= 0 {
		l.OperationTimeout = 60 * time.Second
	}
	if l.CloseGrace <= 0 {
		l.CloseGrace = 5 * time.Second
	}
	if l.MaxConcurrentRequests <= 0 {
		l.MaxConcurrentRequests = 8
	}
	if l.MaxMessageBytes <= 0 {
		l.MaxMessageBytes = 16 << 20
	}
	if l.MaxCatalogPages <= 0 {
		l.MaxCatalogPages = 100
	}
	if l.MaxCatalogItems <= 0 {
		l.MaxCatalogItems = 10_000
	}
	if l.MaxInteractionRounds <= 0 {
		l.MaxInteractionRounds = 4
	}
	if l.EventBuffer <= 0 {
		l.EventBuffer = 256
	}
	return l
}

type ServerConfig struct {
	ID        string        `json:"id"`
	Transport TransportKind `json:"transport"`
	Endpoint  string        `json:"endpoint,omitempty"`
	Process   *ProcessSpec  `json:"process,omitempty"`
	Limits    Limits        `json:"-"`
}

type ProcessLauncher interface {
	Launch(context.Context, ProcessSpec) (Process, error)
}
type Process interface {
	Stdin() io.WriteCloser
	Stdout() io.ReadCloser
	Stderr() io.ReadCloser
	Wait(context.Context) error
	Terminate(context.Context) error
}

type CredentialProvider interface {
	Headers(context.Context, *url.URL) (http.Header, error)
}
type CredentialProviderFunc func(context.Context, *url.URL) (http.Header, error)

func (f CredentialProviderFunc) Headers(ctx context.Context, u *url.URL) (http.Header, error) {
	return f(ctx, u)
}

type HTTPAuthMode string

const (
	AuthNone    HTTPAuthMode = "none"
	AuthHeaders HTTPAuthMode = "static_headers"
	AuthBearer  HTTPAuthMode = "bearer"
	AuthOAuth   HTTPAuthMode = "oauth"
)

type OAuthTokens struct {
	Config oauth2.Config
	Token  oauth2.Token
}
type TokenStore interface {
	Load(context.Context, string) (OAuthTokens, bool, error)
	Save(context.Context, string, OAuthTokens) error
	Delete(context.Context, string) error
}
type OAuthCallbackResult struct{ Code, State, Issuer string }
type AuthorizationCallback func(context.Context, string) (OAuthCallbackResult, error)
type OAuthConfig struct {
	ScopeKey            string
	Store               TokenStore
	OnAuthorize         AuthorizationCallback
	ClientID            string
	ClientSecret        string
	ClientIDMetadataURL string
	RedirectURI         string
	Handler             auth.OAuthHandler
	DynamicRegistration *oauthex.ClientRegistrationMetadata
}
type HTTPAuthConfig struct {
	Mode        HTTPAuthMode
	Credentials CredentialProvider
	OAuth       *OAuthConfig
}

type InteractionRequest struct {
	Method string
	Raw    json.RawMessage
}
type InteractionResponse struct{ Raw json.RawMessage }
type InteractionHandler interface {
	Handle(context.Context, InteractionRequest) (InteractionResponse, error)
}
type InteractionHandlerFunc func(context.Context, InteractionRequest) (InteractionResponse, error)

func (f InteractionHandlerFunc) Handle(ctx context.Context, r InteractionRequest) (InteractionResponse, error) {
	return f(ctx, r)
}

type Options struct {
	HTTPAuth                   HTTPAuthConfig
	HTTPClient                 *http.Client
	ProcessLauncher            ProcessLauncher
	InteractionHandler         InteractionHandler
	EnableCatalogNotifications bool
	EventBuffer                int
}

type RawValue struct {
	Present bool            `json:"present"`
	JSON    json.RawMessage `json:"json,omitempty"`
}
type CapabilitySnapshot struct {
	Tools     bool            `json:"tools"`
	Resources bool            `json:"resources"`
	Prompts   bool            `json:"prompts"`
	Raw       json.RawMessage `json:"raw,omitempty"`
}
type ServerInfo struct {
	Name    string          `json:"name,omitempty"`
	Version string          `json:"version,omitempty"`
	Raw     json.RawMessage `json:"raw,omitempty"`
}
type ConnectionInfo struct {
	ID              string             `json:"id"`
	ProtocolVersion string             `json:"protocol_version"`
	ServerInfo      *ServerInfo        `json:"server_info,omitempty"`
	Capabilities    CapabilitySnapshot `json:"capabilities"`
	Instructions    string             `json:"instructions,omitempty"`
	RawDiscovery    json.RawMessage    `json:"raw_discovery,omitempty"`
}

type Page[T any] struct {
	Items      []T             `json:"items"`
	NextCursor *string         `json:"next_cursor,omitempty"`
	TTL        *time.Duration  `json:"ttl,omitempty"`
	CacheScope string          `json:"cache_scope,omitempty"`
	RawResult  json.RawMessage `json:"raw_result"`
}
type ToolDefinition struct {
	Name         string          `json:"name"`
	Title        string          `json:"title,omitempty"`
	Description  string          `json:"description,omitempty"`
	InputSchema  json.RawMessage `json:"input_schema"`
	OutputSchema RawValue        `json:"output_schema"`
	Annotations  RawValue        `json:"annotations"`
	Raw          json.RawMessage `json:"raw"`
}
type ResourceDefinition struct {
	URI, Name string
	Raw       json.RawMessage
}
type ResourceTemplate struct {
	URITemplate, Name string
	Raw               json.RawMessage
}
type PromptDefinition struct {
	Name, Description string
	Raw               json.RawMessage
}
type ToolCatalog struct {
	Items      []ToolDefinition
	Generation uint64
	Stale      bool
}

type ContentBlock struct {
	Type string          `json:"type"`
	Raw  json.RawMessage `json:"raw"`
}
type Continuation struct {
	RequestState   RawValue
	InputResponses map[string]json.RawMessage
	Round          int
}
type InputRequired struct {
	Requests     map[string]json.RawMessage
	RequestState RawValue
	Raw          json.RawMessage
}
type ToolCallRequest struct {
	Name          string
	Arguments     json.RawMessage
	Continuation  *Continuation
	ProgressToken string
}
type ToolCallResult struct {
	OperationID       string          `json:"operation_id"`
	ResultType        string          `json:"result_type"`
	Content           []ContentBlock  `json:"content"`
	StructuredContent RawValue        `json:"structured_content"`
	IsError           bool            `json:"is_error"`
	InputRequired     *InputRequired  `json:"input_required,omitempty"`
	RawResult         json.RawMessage `json:"raw_result"`
}
type ResourceReadRequest struct {
	URI          string
	Continuation *Continuation
}
type ResourceContent struct {
	URI, MIMEType string
	Raw           json.RawMessage
}
type ResourceReadResult struct {
	ResultType    string
	Contents      []ResourceContent
	InputRequired *InputRequired
	RawResult     json.RawMessage
}
type PromptGetRequest struct {
	Name         string
	Arguments    map[string]string
	Continuation *Continuation
}
type PromptMessage struct {
	Role     string
	Contents []ContentBlock
	Raw      json.RawMessage
}
type PromptGetResult struct {
	ResultType, Description string
	Messages                []PromptMessage
	InputRequired           *InputRequired
	RawResult               json.RawMessage
}

type EventKind string

const (
	EventProgress            EventKind = "progress"
	EventCatalogChanged      EventKind = "catalog_changed"
	EventResourceUpdated     EventKind = "resource_updated"
	EventInteractionRequired EventKind = "interaction_required"
	EventDisconnected        EventKind = "disconnected"
	EventDiagnostic          EventKind = "diagnostic"
)

type Event struct {
	Kind                  EventKind `json:"kind"`
	OperationID, Category string
	Raw                   json.RawMessage
	Diagnostic            string
}

type ErrorKind string

const (
	ErrConfig                ErrorKind = "config"
	ErrUnsupportedCapability ErrorKind = "unsupported_capability"
	ErrAuthRequired          ErrorKind = "auth_required"
	ErrAuthFailed            ErrorKind = "auth_failed"
	ErrProtocol              ErrorKind = "protocol"
	ErrTransport             ErrorKind = "transport"
	ErrTimeout               ErrorKind = "timeout"
	ErrCancelled             ErrorKind = "cancelled"
	ErrLimitExceeded         ErrorKind = "limit_exceeded"
	ErrClosed                ErrorKind = "closed"
)

type MCPError struct {
	Kind                ErrorKind
	Operation           string
	RPCCode, HTTPStatus *int
	MayHaveExecuted     bool
	Cause               error
}

func (e *MCPError) Error() string {
	if e.Operation == "" {
		return "mcp: " + string(e.Kind)
	}
	return "mcp " + e.Operation + ": " + string(e.Kind)
}
func (e *MCPError) Unwrap() error { return e.Cause }
func (e *MCPError) Is(target error) bool {
	var other *MCPError
	return errors.As(target, &other) && e.Kind == other.Kind
}
