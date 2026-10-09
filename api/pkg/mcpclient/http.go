package mcpclient

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"sync"

	"github.com/modelcontextprotocol/go-sdk/jsonrpc"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

func StaticHeaders(headers http.Header) CredentialProvider {
	copy := headers.Clone()
	return CredentialProviderFunc(func(context.Context, *url.URL) (http.Header, error) { return copy.Clone(), nil })
}
func BearerToken(token string) CredentialProvider {
	return CredentialProviderFunc(func(context.Context, *url.URL) (http.Header, error) {
		h := make(http.Header)
		h.Set("Authorization", "Bearer "+token)
		return h, nil
	})
}

func configuredHTTPClient(options Options, limits Limits) (*http.Client, *httpBoundary, error) {
	client := &http.Client{}
	if options.HTTPClient != nil {
		*client = *options.HTTPClient
	}
	base := client.Transport
	if base == nil {
		base = http.DefaultTransport
	}
	mode := options.HTTPAuth.Mode
	if mode == "" {
		mode = AuthNone
	}
	switch mode {
	case AuthNone:
		if options.HTTPAuth.Credentials != nil || options.HTTPAuth.OAuth != nil {
			return nil, nil, &MCPError{Kind: ErrConfig, Operation: "HTTP auth"}
		}
	case AuthHeaders, AuthBearer:
		if options.HTTPAuth.Credentials == nil || options.HTTPAuth.OAuth != nil {
			return nil, nil, &MCPError{Kind: ErrConfig, Operation: "HTTP auth"}
		}
	case AuthOAuth:
		if options.HTTPAuth.OAuth == nil || options.HTTPAuth.Credentials != nil {
			return nil, nil, &MCPError{Kind: ErrConfig, Operation: "HTTP auth"}
		}
	default:
		return nil, nil, &MCPError{Kind: ErrConfig, Operation: "HTTP auth"}
	}
	boundary := &httpBoundary{base: base, credentials: options.HTTPAuth.Credentials, mode: mode, limit: limits.MaxMessageBytes}
	client.Transport = boundary
	// Long-lived subscriptions must not inherit a global client timeout.
	client.Timeout = 0
	client.CheckRedirect = func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }
	return client, boundary, nil
}

type httpBoundary struct {
	base                http.RoundTripper
	credentials         CredentialProvider
	mode                HTTPAuthMode
	limit               int64
	mu                  sync.Mutex
	discovery           json.RawMessage
	protocolErr         error
	blockLegacyFallback bool
	onNotification      func(string, json.RawMessage)
}

func (t *httpBoundary) discoveryResult() json.RawMessage {
	t.mu.Lock()
	defer t.mu.Unlock()
	return cloneRaw(t.discovery)
}
func (t *httpBoundary) setProtocolError(err error) { t.mu.Lock(); t.protocolErr = err; t.mu.Unlock() }
func (t *httpBoundary) protocolError() error       { t.mu.Lock(); defer t.mu.Unlock(); return t.protocolErr }
func (t *httpBoundary) legacyFallbackBlocked() bool {
	t.mu.Lock()
	defer t.mu.Unlock()
	return t.blockLegacyFallback
}

func (t *httpBoundary) RoundTrip(req *http.Request) (*http.Response, error) {
	clone := req.Clone(req.Context())
	slot, _ := req.Context().Value(captureKey{}).(*captureSlot)
	method, prepareErr := t.prepareRequest(clone)
	if prepareErr != nil {
		if slot != nil {
			slot.setBoundaryError(prepareErr)
		}
		return nil, prepareErr
	}
	if method == "initialize" && t.legacyFallbackBlocked() {
		return nil, &MCPError{Kind: ErrProtocol, Operation: "initialize", Cause: errors.New("modern MCP server rejected discovery; legacy fallback is not allowed")}
	}
	if t.credentials != nil {
		headers, err := t.credentials.Headers(req.Context(), req.URL)
		if err != nil {
			return nil, &MCPError{Kind: ErrAuthFailed, Operation: "HTTP credentials", Cause: err}
		}
		for key, values := range headers {
			for _, value := range values {
				clone.Header.Add(key, value)
			}
		}
	}
	if slot != nil {
		slot.markSent()
	}
	resp, err := t.base.RoundTrip(clone)
	if err != nil {
		return nil, err
	}
	if t.mode != AuthOAuth && (resp.StatusCode == http.StatusUnauthorized || resp.StatusCode == http.StatusForbidden) {
		resp.Body.Close()
		status := resp.StatusCode
		kind := ErrAuthFailed
		if status == http.StatusUnauthorized {
			kind = ErrAuthRequired
		}
		authErr := &MCPError{Kind: kind, Operation: "HTTP request", HTTPStatus: &status}
		if slot, _ := req.Context().Value(captureKey{}).(*captureSlot); slot != nil {
			slot.setBoundaryError(authErr)
		}
		return nil, authErr
	}
	if method == "server/discover" && resp.StatusCode >= 400 && resp.StatusCode != http.StatusUnauthorized && resp.StatusCode != http.StatusForbidden && strings.Contains(resp.Header.Get("Content-Type"), "application/json") {
		body, readErr := io.ReadAll(io.LimitReader(resp.Body, t.limit+1))
		_ = resp.Body.Close()
		if readErr != nil {
			return nil, readErr
		}
		if int64(len(body)) > t.limit {
			return nil, &MCPError{Kind: ErrLimitExceeded, Operation: "server/discover"}
		}
		var envelope struct {
			Error struct {
				Code int `json:"code"`
			} `json:"error"`
		}
		decoded := json.Unmarshal(body, &envelope) == nil
		modernMarker := strings.HasPrefix(resp.Header.Get("Mcp-Protocol-Version"), "2026-") || envelope.Error.Code == mcp.CodeHeaderMismatch || envelope.Error.Code == mcp.CodeMissingRequiredClientCapabilities
		if decoded && modernMarker && envelope.Error.Code != 0 && envelope.Error.Code != jsonrpc.CodeMethodNotFound && envelope.Error.Code != mcp.CodeUnsupportedProtocolVersion {
			t.mu.Lock()
			t.blockLegacyFallback = true
			t.protocolErr = &MCPError{Kind: ErrProtocol, Operation: "server/discover", Cause: errors.New("modern MCP discovery returned a protocol error")}
			t.mu.Unlock()
		}
		resp.Body = io.NopCloser(bytes.NewReader(body))
		resp.ContentLength = int64(len(body))
	}
	if resp.StatusCode >= 200 && resp.StatusCode < 300 && strings.Contains(resp.Header.Get("Content-Type"), "application/json") && method != "" {
		body, readErr := io.ReadAll(io.LimitReader(resp.Body, t.limit+1))
		_ = resp.Body.Close()
		if readErr != nil {
			return nil, readErr
		}
		if int64(len(body)) > t.limit {
			limitErr := &MCPError{Kind: ErrLimitExceeded, Operation: "read HTTP body"}
			if slot != nil {
				slot.setBoundaryError(limitErr)
			}
			return nil, limitErr
		}
		forward := t.captureJSON(method, body, slot)
		resp.Body = io.NopCloser(bytes.NewReader(forward))
		resp.ContentLength = int64(len(forward))
	} else if t.limit > 0 {
		resp.Body = &limitedBody{ReadCloser: resp.Body, limit: t.limit, sse: strings.Contains(resp.Header.Get("Content-Type"), "text/event-stream"), slot: slot, onEvent: func(event []byte) { t.captureEvent(method, event, slot) }}
	}
	return resp, nil
}

type limitedBody struct {
	io.ReadCloser
	limit     int64
	used      int64
	sse       bool
	lineEmpty bool
	slot      *captureSlot
	onEvent   func([]byte)
	event     []byte
}

func (r *limitedBody) Read(buf []byte) (int, error) {
	n, err := r.ReadCloser.Read(buf)
	for i := 0; i < n; i++ {
		r.used++
		if r.sse {
			r.event = append(r.event, buf[i])
		}
		if r.sse && buf[i] == '\n' {
			if r.lineEmpty {
				if r.onEvent != nil {
					r.onEvent(r.event)
				}
				r.event = nil
				r.used = 0
			}
			r.lineEmpty = true
		} else if buf[i] != '\r' {
			r.lineEmpty = false
		}
		if r.used > r.limit {
			limitErr := &MCPError{Kind: ErrLimitExceeded, Operation: "read HTTP body", Cause: errors.New("MCP message exceeds configured size")}
			if r.slot != nil {
				r.slot.setBoundaryError(limitErr)
			}
			return i, limitErr
		}
	}
	return n, err
}

func classify(operation string, err error, sent bool) error {
	if err == nil {
		return nil
	}
	var known *MCPError
	if errors.As(err, &known) {
		copy := *known
		if copy.Operation == "" {
			copy.Operation = operation
		}
		copy.MayHaveExecuted = copy.MayHaveExecuted || sent
		return &copy
	}
	if errors.Is(err, context.Canceled) {
		return &MCPError{Kind: ErrCancelled, Operation: operation, MayHaveExecuted: sent, Cause: err}
	}
	if errors.Is(err, context.DeadlineExceeded) {
		return &MCPError{Kind: ErrTimeout, Operation: operation, MayHaveExecuted: sent, Cause: err}
	}
	var rpc *jsonrpc.Error
	if errors.As(err, &rpc) {
		code := int(rpc.Code)
		return &MCPError{Kind: ErrProtocol, Operation: operation, RPCCode: &code, MayHaveExecuted: sent, Cause: err}
	}
	return &MCPError{Kind: ErrTransport, Operation: operation, MayHaveExecuted: sent, Cause: fmt.Errorf("MCP operation: %w", err)}
}
