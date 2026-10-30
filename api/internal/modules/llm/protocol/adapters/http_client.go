package adapter

import (
	"bufio"
	"bytes"
	"context"
	"crypto/tls"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/zgiai/zgi/api/internal/modules/llm/internal/urlguard"
	"github.com/zgiai/zgi/api/internal/observability"
)

// MaxSSEFrameBytes bounds a single SSE frame for every stream parser in the
// gateway. The Responses response.completed frame embeds the entire response
// object (all output items plus reasoning), so it is the largest frame on the
// wire and sets the floor for this value. It is deliberately not raised
// further: the buffer is per in-flight stream, so the ceiling multiplies by
// concurrency. Console applies the same limit, so keep the two in step.
const MaxSSEFrameBytes = 8 << 20

// annotateSSEFrameLimit makes the frame cap visible. Left as a bare
// bufio.ErrTooLong it is indistinguishable from an ordinary network error, and
// the limit shows up in production only as "occasional stream interruptions".
func annotateSSEFrameLimit(err error) error {
	if !errors.Is(err, bufio.ErrTooLong) {
		return err
	}
	return fmt.Errorf("upstream SSE frame exceeds %d bytes: %w", MaxSSEFrameBytes, err)
}

// HTTPClient wrapped HTTP client
type HTTPClient struct {
	client       *http.Client
	streamClient *http.Client // Separate client for streaming (no timeout)
	maxRetries   int
	authHook     func(req *http.Request) // Optional: called before each request for custom auth
	urlPolicy    urlguard.Policy
	guardURL     bool
}

type HTTPClientOptions struct {
	AuthHook            func(req *http.Request)
	GuardOutboundURL    bool
	GuardOutboundDNS    bool
	AllowPrivateBaseURL bool
	URLPolicy           urlguard.Policy
}

// HTTPResponse carries the response pieces needed by transport-specific adapters.
type HTTPResponse struct {
	Body       []byte
	StatusCode int
	Header     http.Header
}

// NewHTTPClient creates an HTTP client
func NewHTTPClient(timeout time.Duration, maxRetries int) *HTTPClient {
	return NewHTTPClientWithAuthHook(timeout, maxRetries, nil)
}

// NewHTTPClientWithAuthHook creates an HTTP client with an optional auth hook.
// The authHook is called on every outgoing request before it is sent.
func NewHTTPClientWithAuthHook(timeout time.Duration, maxRetries int, authHook func(req *http.Request)) *HTTPClient {
	return NewHTTPClientWithOptions(timeout, maxRetries, HTTPClientOptions{AuthHook: authHook})
}

func NewHTTPClientFromConfig(config *AdapterConfig, timeout time.Duration, maxRetries int) *HTTPClient {
	opts := HTTPClientOptions{}
	if config != nil {
		opts.AuthHook = config.AuthHook
		opts.GuardOutboundURL = config.GuardOutboundURL
		opts.GuardOutboundDNS = config.GuardOutboundDNS
		opts.AllowPrivateBaseURL = config.AllowPrivateBaseURL
	}
	return NewHTTPClientWithOptions(timeout, maxRetries, opts)
}

// ValidateOutboundBaseURL applies the same URL policy used by HTTP adapters to
// non-HTTP transports such as WebSocket handshakes.
func ValidateOutboundBaseURL(ctx context.Context, rawURL string, config *AdapterConfig) error {
	if config == nil || !config.GuardOutboundURL {
		return nil
	}
	policy := urlguard.Policy{
		AllowPrivate: config.AllowPrivateBaseURL,
		GuardDNS:     config.GuardOutboundDNS,
	}
	if err := urlguard.ValidateBaseURL(ctx, rawURL, policy); err != nil {
		return fmt.Errorf("blocked unsafe target: %w", err)
	}
	return nil
}

// OutboundDialContext returns a DNS-rebinding-safe dialer when DNS guarding is
// enabled. A nil result lets the caller retain its transport default.
func OutboundDialContext(config *AdapterConfig) func(context.Context, string, string) (net.Conn, error) {
	if config == nil || !config.GuardOutboundURL || !config.GuardOutboundDNS {
		return nil
	}
	return guardedDialContext(urlguard.Policy{
		AllowPrivate: config.AllowPrivateBaseURL,
		GuardDNS:     true,
	})
}

func NewHTTPClientWithOptions(timeout time.Duration, maxRetries int, opts HTTPClientOptions) *HTTPClient {
	if timeout == 0 {
		timeout = 60 * time.Second
	}
	if maxRetries == 0 {
		maxRetries = 2 // Default to 2 retries for transient failures
	}

	policy := opts.URLPolicy
	if opts.AllowPrivateBaseURL {
		policy.AllowPrivate = true
	}
	policy.GuardDNS = opts.GuardOutboundDNS
	var dialContext func(ctx context.Context, network, address string) (net.Conn, error)
	var checkRedirect func(req *http.Request, via []*http.Request) error
	if opts.GuardOutboundURL {
		if opts.GuardOutboundDNS {
			dialContext = guardedDialContext(policy)
		}
		checkRedirect = func(req *http.Request, _ []*http.Request) error {
			if err := urlguard.ValidateURL(req.Context(), req.URL, policy); err != nil {
				return fmt.Errorf("blocked unsafe target: %w", err)
			}
			return nil
		}
	}

	// Non-streaming transport: force HTTP/1.1 to avoid HTTP/2 multiplexing issues
	// (e.g. "http2: response body closed" under high concurrency).
	transport := &http.Transport{
		Proxy:                 http.ProxyFromEnvironment,
		MaxIdleConns:          200,
		MaxIdleConnsPerHost:   50,
		MaxConnsPerHost:       100,
		IdleConnTimeout:       90 * time.Second,
		TLSHandshakeTimeout:   10 * time.Second,
		ExpectContinueTimeout: 1 * time.Second,
		DisableKeepAlives:     false,
		ForceAttemptHTTP2:     false,                                                                  // Disable HTTP/2
		TLSNextProto:          make(map[string]func(authority string, c *tls.Conn) http.RoundTripper), // Force HTTP/1.1
		DialContext:           dialContext,
	}

	// Streaming transport - disable keep-alives to prevent connection reuse issues
	// When streaming responses are cancelled or not fully consumed, leftover data
	// in the connection buffer can cause subsequent requests to read stale data.
	// Disabling keep-alives ensures each streaming request gets a fresh connection.
	streamTransport := &http.Transport{
		Proxy:                 http.ProxyFromEnvironment,
		MaxIdleConns:          200,
		MaxIdleConnsPerHost:   50,
		MaxConnsPerHost:       100,
		IdleConnTimeout:       90 * time.Second,
		TLSHandshakeTimeout:   10 * time.Second,
		ExpectContinueTimeout: 1 * time.Second,
		DisableKeepAlives:     true, // IMPORTANT: Disable keep-alives for streaming to avoid stale data issues
		ResponseHeaderTimeout: 600 * time.Second,
		DialContext:           dialContext,
	}

	return &HTTPClient{
		client: &http.Client{
			Timeout:       timeout,
			Transport:     observability.HTTPTransport(transport),
			CheckRedirect: checkRedirect,
		},
		// Stream client has no Timeout - relies on context cancellation
		// This allows streaming responses to run for as long as needed
		streamClient: &http.Client{
			Timeout:       0, // No timeout for streaming - context handles cancellation
			Transport:     observability.HTTPTransport(streamTransport),
			CheckRedirect: checkRedirect,
		},
		maxRetries: maxRetries,
		authHook:   opts.AuthHook,
		urlPolicy:  policy,
		guardURL:   opts.GuardOutboundURL,
	}
}

func (c *HTTPClient) StandardClient() *http.Client {
	if c == nil {
		return nil
	}
	return c.client
}

func guardedDialContext(policy urlguard.Policy) func(ctx context.Context, network, address string) (net.Conn, error) {
	dialer := &net.Dialer{}
	return func(ctx context.Context, network, address string) (net.Conn, error) {
		host, port, err := net.SplitHostPort(address)
		if err != nil {
			return nil, fmt.Errorf("blocked unsafe target %q: %w", address, err)
		}

		addrs, err := urlguard.ResolveSafeHost(ctx, host, policy)
		if err != nil {
			return nil, fmt.Errorf("blocked unsafe target %q: %w", host, err)
		}

		var lastErr error
		for _, addr := range addrs {
			conn, err := dialer.DialContext(ctx, network, net.JoinHostPort(addr.String(), port))
			if err == nil {
				return conn, nil
			}
			lastErr = err
		}
		if lastErr != nil {
			return nil, lastErr
		}
		return nil, fmt.Errorf("no resolved address for %q", host)
	}
}

func (c *HTTPClient) validateOutboundURL(ctx context.Context, parsed *url.URL) error {
	if c == nil || !c.guardURL {
		return nil
	}
	if err := urlguard.ValidateURL(ctx, parsed, c.urlPolicy); err != nil {
		return fmt.Errorf("blocked unsafe target: %w", err)
	}
	return nil
}

// isHTMLResponse checks whether the response body looks like an HTML error page
// (typically returned by nginx/reverse proxy instead of JSON).
func isHTMLResponse(body []byte) bool {
	trimmed := strings.TrimSpace(string(body))
	return strings.HasPrefix(trimmed, "<") || strings.HasPrefix(trimmed, "<!") || strings.HasPrefix(trimmed, "<html")
}

func (c *HTTPClient) DoRequest(ctx context.Context, method, url string, headers map[string]string, body interface{}) ([]byte, int, error) {
	resp, err := c.DoRequestDetailed(ctx, method, url, headers, body)
	if err != nil {
		if resp != nil {
			return resp.Body, resp.StatusCode, err
		}
		return nil, 0, err
	}
	return resp.Body, resp.StatusCode, nil
}

// DoRequestDetailed executes HTTP request with retry support and exposes response headers.
// Retries on: non-timeout network errors, non-timeout 5xx status, body-read
// failures, and proxy HTML error responses (nginx 400/502/etc).
// Timeouts (a local client/context deadline, an upstream 504, or Console's
// llm.provider.timeout) are returned at once: replaying a call that already
// ran out of time only multiplies the wait. A retry is also skipped when the
// request context cannot fit another backoff plus a full attempt.
func (c *HTTPClient) DoRequestDetailed(ctx context.Context, method, url string, headers map[string]string, body interface{}) (*HTTPResponse, error) {
	var jsonData []byte
	if body != nil {
		var err error
		jsonData, err = json.Marshal(body)
		if err != nil {
			return nil, fmt.Errorf("failed to marshal request body: %w", err)
		}
	}

	var lastErr error
	var lastStatusCode int
	var lastBody []byte
	var lastHeader http.Header
	retries := 0

	for attempt := 0; attempt <= c.maxRetries; attempt++ {
		if attempt > 0 {
			if isTimeoutFailure(lastStatusCode, lastErr) || !c.hasTimeForRetry(ctx, attempt) {
				break
			}
			retries = attempt
			// Exponential backoff
			backoff := time.Duration(attempt) * time.Second
			select {
			case <-ctx.Done():
				return nil, NormalizeTransportError(ctx.Err())
			case <-time.After(backoff):
			}
		}

		// Rebuild request each attempt (body reader must be reset)
		var reqBody io.Reader
		if jsonData != nil {
			reqBody = bytes.NewReader(jsonData)
		}
		req, err := http.NewRequestWithContext(ctx, method, url, reqBody)
		if err != nil {
			return nil, fmt.Errorf("failed to create request: %w", err)
		}
		if err := c.validateOutboundURL(ctx, req.URL); err != nil {
			return nil, err
		}

		// Set default headers
		req.Header.Set("Content-Type", "application/json")
		for key, value := range headers {
			req.Header.Set(key, value)
		}

		// Apply auth hook if set (e.g., HMAC signing for internal APIs)
		if c.authHook != nil {
			c.authHook(req)
		}

		// Execute request (with retry)
		var resp *http.Response

		resp, err = c.client.Do(req)
		if err != nil {
			lastErr = err
			continue // Network error, retry
		}

		// Read body inside the loop so read failures can be retried
		respBody, readErr := io.ReadAll(resp.Body)
		resp.Body.Close()

		if readErr != nil {
			lastErr = fmt.Errorf("failed to read response body: %w", readErr)
			lastStatusCode = resp.StatusCode
			lastHeader = resp.Header.Clone()
			continue // Body read failure, retry
		}

		lastStatusCode = resp.StatusCode
		lastBody = respBody
		lastHeader = resp.Header.Clone()
		if isTerminalPlatformChannelResponse(resp.StatusCode, respBody) {
			return &HTTPResponse{Body: respBody, StatusCode: resp.StatusCode, Header: resp.Header.Clone()}, nil
		}

		// 5xx server errors, retry. Carry the typed upstream-failure identity so
		// the gateway classifier and the error catalog can project the retried
		// failure; a bare fmt.Errorf reached the protocol boundary unclassified
		// and leaked the English fallback message in every locale.
		if resp.StatusCode >= 500 {
			bodySnippet := string(respBody)
			if len(bodySnippet) > 500 {
				bodySnippet = bodySnippet[:500] + "..."
			}
			// Keep the platform channel's own identity through the retry loop.
			// Without this a 5xx that exhausts its retries is reported as a
			// generic upstream failure, so the client sees llm.invocation.failed
			// instead of, say, llm.provider.none_available.
			sentinel := error(ErrUpstreamError)
			if consoleSentinel, ok := ConsoleSentinelFromBody(respBody); ok {
				sentinel = consoleSentinel
			}
			lastErr = fmt.Errorf("%w: server error %d: %s", sentinel, resp.StatusCode, bodySnippet)
			continue
		}

		// Detect proxy/nginx HTML responses (e.g. 400 Bad Request HTML page).
		// These are transient infrastructure errors, not real API errors.
		if isHTMLResponse(respBody) && resp.StatusCode >= 400 {
			bodySnippet := string(respBody)
			if len(bodySnippet) > 500 {
				bodySnippet = bodySnippet[:500] + "..."
			}
			lastErr = fmt.Errorf("received HTML error page from proxy (status %d): %s", resp.StatusCode, bodySnippet)
			if attempt < c.maxRetries {
				continue // Retry on proxy HTML errors
			}
		}

		return &HTTPResponse{Body: respBody, StatusCode: resp.StatusCode, Header: resp.Header.Clone()}, nil
	}

	if lastErr != nil {
		if lastStatusCode == http.StatusGatewayTimeout && !errors.Is(lastErr, ErrTimeout) {
			lastErr = fmt.Errorf("%w: %w", ErrTimeout, lastErr)
		}
		lastErr = NormalizeTransportError(lastErr)
		return &HTTPResponse{Body: lastBody, StatusCode: lastStatusCode, Header: lastHeader}, fmt.Errorf("request failed after %d retries: %w", retries, lastErr)
	}
	return &HTTPResponse{Body: lastBody, StatusCode: lastStatusCode, Header: lastHeader}, fmt.Errorf("request failed after %d retries", retries)
}

// isTimeoutFailure reports whether the previous attempt failed because it ran
// out of time. lastErr is the raw attempt error, so a local client or context
// deadline is recognised through NormalizeTransportError, and Console's
// llm.provider.timeout through the ErrTimeout sentinel it maps to.
func isTimeoutFailure(statusCode int, lastErr error) bool {
	if statusCode == http.StatusGatewayTimeout {
		return true
	}
	return lastErr != nil && errors.Is(NormalizeTransportError(lastErr), ErrTimeout)
}

// hasTimeForRetry reports whether the request context can still fit the
// backoff before attempt plus one full attempt bounded by the client timeout.
// Without a context deadline there is no total budget to protect.
func (c *HTTPClient) hasTimeForRetry(ctx context.Context, attempt int) bool {
	deadline, ok := ctx.Deadline()
	if !ok {
		return true
	}
	need := time.Duration(attempt) * time.Second
	if c.client != nil {
		need += c.client.Timeout
	}
	return time.Until(deadline) > need
}

// DoRawRequestDetailed sends one non-replayable request without retrying.
// Callers retain responsibility for choosing a content type appropriate to the body.
func (c *HTTPClient) DoRawRequestDetailed(
	ctx context.Context,
	method string,
	targetURL string,
	headers map[string]string,
	body io.Reader,
) (*HTTPResponse, error) {
	if c == nil || c.client == nil {
		return nil, fmt.Errorf("http client is not initialized")
	}

	req, err := http.NewRequestWithContext(ctx, method, targetURL, body)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}
	if err := c.validateOutboundURL(ctx, req.URL); err != nil {
		return nil, err
	}
	for key, value := range headers {
		req.Header.Set(key, value)
	}
	if c.authHook != nil {
		c.authHook(req)
	}

	resp, err := c.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to execute request: %w", err)
	}
	defer resp.Body.Close()

	responseBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response body: %w", err)
	}
	return &HTTPResponse{
		Body:       responseBody,
		StatusCode: resp.StatusCode,
		Header:     resp.Header.Clone(),
	}, nil
}

func isTerminalPlatformChannelResponse(statusCode int, body []byte) bool {
	if statusCode < http.StatusInternalServerError {
		return false
	}
	if sentinel, ok := ConsoleSentinelFromBody(body); ok && errors.Is(sentinel, ErrBillingSettleFailed) {
		// Retryability describes settlement recovery, never replaying inference.
		return true
	}
	var payload struct {
		// Console's cataloged envelope: a stable code plus whether repeating
		// the call can help. retryable:false (for example an upstream that
		// rejected the platform credential) is terminal; retrying only
		// multiplies upstream calls.
		ErrorCode string `json:"error_code"`
		Retryable *bool  `json:"retryable"`
		Error     struct {
			Code string `json:"code"`
			Type string `json:"type"`
		} `json:"error"`
	}
	if err := json.Unmarshal(body, &payload); err != nil {
		return false
	}
	if strings.TrimSpace(payload.ErrorCode) != "" && payload.Retryable != nil && !*payload.Retryable {
		return true
	}
	return payload.Error.Code == ErrorCodePlatformChannelUnavailable ||
		payload.Error.Type == ErrorCodePlatformChannelUnavailable ||
		payload.Error.Code == "billing_settlement_failed"
}

// DoStreamRequest executes streaming HTTP request
// Uses a separate HTTP client without client-level timeout to allow long-running streams.
// The context is used for cancellation instead of a fixed timeout.
func (c *HTTPClient) DoStreamRequest(ctx context.Context, method, url string, headers map[string]string, body interface{}) (*http.Response, error) {
	streamHeaders := map[string]string{
		"Accept":        "text/event-stream",
		"Cache-Control": "no-cache",
		"Connection":    "keep-alive",
	}
	for key, value := range headers {
		streamHeaders[key] = value
	}
	return c.doSingleRequest(ctx, method, url, streamHeaders, body, c.streamClient)
}

// DoSingleRequestNoRedirect executes exactly one request and rejects redirects.
// It is intended for generation endpoints that may charge as soon as a POST is dispatched.
func (c *HTTPClient) DoSingleRequestNoRedirect(ctx context.Context, method, url string, headers map[string]string, body interface{}) (*http.Response, error) {
	client := *c.streamClient
	client.CheckRedirect = func(*http.Request, []*http.Request) error {
		return http.ErrUseLastResponse
	}
	return c.doSingleRequest(ctx, method, url, headers, body, &client)
}

func (c *HTTPClient) doSingleRequest(
	ctx context.Context,
	method string,
	url string,
	headers map[string]string,
	body interface{},
	client *http.Client,
) (*http.Response, error) {
	var reqBody io.Reader
	if body != nil {
		jsonData, err := json.Marshal(body)
		if err != nil {
			return nil, fmt.Errorf("failed to marshal request body: %w", err)
		}
		reqBody = bytes.NewReader(jsonData)
	}

	req, err := http.NewRequestWithContext(ctx, method, url, reqBody)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}
	if err := c.validateOutboundURL(ctx, req.URL); err != nil {
		return nil, err
	}

	// Set headers
	req.Header.Set("Content-Type", "application/json")
	for key, value := range headers {
		req.Header.Set(key, value)
	}

	// Apply auth hook if set
	if c.authHook != nil {
		c.authHook(req)
	}

	// Streaming and paid-generation callers pass a client without a fixed timeout;
	// their request context remains the cancellation boundary.
	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to execute request: %w", NormalizeTransportError(err))
	}

	if resp.StatusCode != http.StatusOK {
		defer resp.Body.Close()
		body, _ := io.ReadAll(resp.Body)
		return nil, NewHTTPStatusError(resp.StatusCode, body)
	}

	return resp, nil
}

// ParseSSE parses Server-Sent Events stream
// It also handles non-SSE JSON error responses from upstream providers
func ParseSSE(reader io.Reader, dataChan chan<- string, errChan chan<- error) {
	parseSSE(reader, dataChan, errChan, true)
}

// ParseSSEToEOF parses an SSE response through EOF so HTTP response trailers
// are populated. This is intended for internal gateway transports that place
// settlement metadata in trailers after the public [DONE] marker.
func ParseSSEToEOF(reader io.Reader, dataChan chan<- string, errChan chan<- error) {
	parseSSE(reader, dataChan, errChan, false)
}

func parseSSE(reader io.Reader, dataChan chan<- string, errChan chan<- error, stopAtDone bool) {
	scanner := bufio.NewScanner(reader)
	scanner.Buffer(nil, MaxSSEFrameBytes)
	var dataBuffer strings.Builder

	lineCount := 0
	dataCount := 0
	var firstLine string

	for scanner.Scan() {
		line := scanner.Text()
		lineCount++

		// Save first line for error detection
		if lineCount == 1 {
			firstLine = line
		}

		// Empty line indicates event end
		if line == "" {
			if dataBuffer.Len() > 0 {
				dataCount++
				dataChan <- dataBuffer.String()
				dataBuffer.Reset()
			}
			continue
		}

		// Parse SSE data lines. Both "data: value" and "data:value" are valid.
		if data, ok := parseSSEDataLine(line); ok {
			// [DONE] indicates stream end
			if data == "[DONE]" {
				if stopAtDone {
					close(dataChan)
					return
				}
				continue
			}

			dataBuffer.WriteString(data)
		}
	}

	// Check for scanner errors
	if err := scanner.Err(); err != nil {
		errChan <- annotateSSEFrameLimit(err)
		close(dataChan)
		return
	}

	if dataCount == 0 && lineCount > 0 && firstLine != "" {
		if err := parseUpstreamJSONStreamError(firstLine); err != nil {
			errChan <- err
			close(dataChan)
			return
		}
	}

	close(dataChan)
}

func parseSSEDataLine(line string) (string, bool) {
	name, value, ok := strings.Cut(line, ":")
	if !ok || name != "data" {
		return "", false
	}
	if strings.HasPrefix(value, " ") {
		value = strings.TrimPrefix(value, " ")
	}
	return value, true
}

// ParseSSEEvents parses Server-Sent Events while preserving event names and raw data.
func ParseSSEEvents(reader io.Reader, eventChan chan<- RawStreamEvent, errChan chan<- error) {
	scanner := bufio.NewScanner(reader)
	scanner.Buffer(nil, MaxSSEFrameBytes)

	var dataBuffer strings.Builder
	eventName := ""
	lineCount := 0
	dataCount := 0
	firstLine := ""

	for scanner.Scan() {
		line := scanner.Text()
		lineCount++
		if lineCount == 1 {
			firstLine = line
		}

		if line == "" {
			if dataBuffer.Len() > 0 {
				data := strings.TrimSuffix(dataBuffer.String(), "\n")
				if data == "[DONE]" {
					close(eventChan)
					return
				}
				dataCount++
				eventChan <- RawStreamEvent{
					Event: eventName,
					Data:  json.RawMessage(data),
				}
				dataBuffer.Reset()
				eventName = ""
			}
			continue
		}

		name, value, _ := strings.Cut(line, ":")
		if strings.HasPrefix(value, " ") {
			value = strings.TrimPrefix(value, " ")
		}

		switch name {
		case "event":
			eventName = value
		case "data":
			dataBuffer.WriteString(value)
			dataBuffer.WriteString("\n")
		}
	}

	if err := scanner.Err(); err != nil {
		errChan <- annotateSSEFrameLimit(err)
		close(eventChan)
		return
	}

	if dataCount == 0 && lineCount > 0 && firstLine != "" {
		if err := parseUpstreamJSONStreamError(firstLine); err != nil {
			errChan <- err
			close(eventChan)
			return
		}
	}

	close(eventChan)
}

func parseUpstreamJSONStreamError(line string) error {
	line = strings.TrimSpace(line)
	if !strings.HasPrefix(line, "{") || !strings.Contains(line, "error") {
		return nil
	}

	var errorResp struct {
		Error struct {
			Message string `json:"message"`
			Type    string `json:"type"`
			Code    string `json:"code"`
		} `json:"error"`
	}
	if err := json.Unmarshal([]byte(line), &errorResp); err != nil || errorResp.Error.Message == "" {
		return nil
	}
	return fmt.Errorf("upstream provider error: %s (type: %s)", errorResp.Error.Message, errorResp.Error.Type)
}

// ParseJSONResponse parses JSON response
func ParseJSONResponse(data []byte, v interface{}) error {
	if err := json.Unmarshal(data, v); err != nil {
		return fmt.Errorf("failed to parse JSON response: %w", err)
	}
	return nil
}
