package providers

import (
	"bytes"
	"context"
	"crypto/tls"
	"encoding/json"
	"errors"
	"io"
	"net"
	"net/http"
	"net/http/httptrace"
	"net/url"
	"strings"
	"sync/atomic"
	"time"
	"unicode"

	"github.com/google/uuid"
	"github.com/zgiai/zgi/api/internal/modules/connectors/core"
)

const responseLimit = 5 << 20

type HTTP struct {
	client        *http.Client
	origin        string
	responseLimit int
}

// WithResponseLimit narrows the parser budget without mutating a shared client.
func (client *HTTP) WithResponseLimit(limit int) *HTTP {
	copy := *client
	if limit > 0 && limit <= responseLimit {
		copy.responseLimit = limit
	}
	return &copy
}

func NewHTTP(client *http.Client, origin string) *HTTP {
	return NewBoundedHTTP(client, origin, 20*time.Second)
}

func NewBoundedHTTP(client *http.Client, origin string, timeout time.Duration) *HTTP {
	if timeout < time.Second || timeout > 50*time.Second {
		timeout = 20 * time.Second
	}
	copy := http.Client{Timeout: timeout}
	if client != nil {
		copy = *client
		if copy.Timeout <= 0 || copy.Timeout > timeout {
			copy.Timeout = timeout
		}
	}
	copy.CheckRedirect = func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }
	return &HTTP{client: &copy, origin: origin}
}

// JSON submits once. Transport ambiguity is never resolved by resubmitting an
// operation, including a billable read. Response bodies never become errors.
func (client *HTTP) JSON(ctx context.Context, method, path string, query url.Values, headers http.Header, body any, output any) (http.Header, error) {
	if ctx == nil || !strings.HasPrefix(path, "/") || strings.HasPrefix(path, "//") {
		return nil, Invalid()
	}
	for _, values := range headers {
		for _, value := range values {
			if strings.ContainsAny(value, "\r\n") {
				return nil, Invalid()
			}
		}
	}
	var data []byte
	if body != nil {
		var err error
		data, err = json.Marshal(body)
		if err != nil {
			return nil, Invalid()
		}
		defer clear(data)
	}
	endpoint := client.origin + path
	if len(query) > 0 {
		endpoint += "?" + query.Encode()
	}
	started := time.Now()
	var written atomic.Bool
	phase, _ := ctx.Value(diagnosticPhaseKey{}).(string)
	trace := &httptrace.ClientTrace{WroteRequest: func(info httptrace.WroteRequestInfo) { written.Store(true) }}
	request, err := http.NewRequestWithContext(httptrace.WithClientTrace(ctx, trace), method, endpoint, bytes.NewReader(data))
	if err != nil {
		return nil, Invalid()
	}
	request.Header = headers.Clone()
	request.Header.Set("Accept", "application/json")
	request.Header.Set("Content-Type", "application/json")
	response, err := client.client.Do(request)
	diagnostic := &Diagnostic{TraceID: uuid.NewString(), Phase: phase, ElapsedMS: time.Since(started).Milliseconds(), RequestWritten: written.Load()}
	if err != nil {
		diagnostic.Category = "connect"
		var dns *net.DNSError
		var certificate *tls.CertificateVerificationError
		var timeout net.Error
		var connection *net.OpError
		unknown := true
		switch {
		case errors.Is(err, context.Canceled):
			diagnostic.Category = "cancelled"
		case errors.Is(err, context.DeadlineExceeded):
			diagnostic.Category = "deadline"
		case errors.As(err, &dns):
			diagnostic.Category = "dns"
			unknown = written.Load()
		case errors.As(err, &certificate):
			diagnostic.Category = "tls"
			unknown = written.Load()
		case errors.As(err, &connection) && connection.Op == "dial":
			unknown = written.Load()
		case errors.As(err, &timeout) && timeout.Timeout():
			diagnostic.Category = "deadline"
		}
		return nil, &Failure{Code: core.ErrorCodeProviderUnavailable, Unknown: unknown, Retryable: !unknown && diagnostic.Category != "cancelled", Diagnostic: diagnostic}
	}
	diagnostic.ResponseReceived = true
	diagnostic.HTTPStatus = response.StatusCode
	defer response.Body.Close()
	requestID := SafeText(response.Header.Get("X-GitHub-Request-Id"), 128)
	if requestID == "" {
		requestID = SafeText(response.Header.Get("X-Request-Id"), 128)
	}
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		diagnostic.Category = "http"
		failure := &Failure{Code: core.ErrorCodeProviderUnavailable, RequestID: requestID, Diagnostic: diagnostic}
		switch response.StatusCode {
		case http.StatusUnauthorized:
			failure.Code = core.ErrorCodeInvalidKey
		case http.StatusForbidden, http.StatusNotFound:
			failure.Code = core.ErrorCodeInsufficientKey
		case http.StatusBadRequest, http.StatusUnprocessableEntity:
			failure.Code = core.ErrorCodeInvalidInput
		case http.StatusTooManyRequests:
			failure.Retryable = true
		default:
			failure.Unknown = response.StatusCode >= 500
		}
		if response.StatusCode == 403 && (response.Header.Get("X-RateLimit-Remaining") == "0" || response.Header.Get("Retry-After") != "") {
			failure.Code = core.ErrorCodeProviderUnavailable
			failure.Retryable = true
		}
		return response.Header, failure
	}
	limit := client.responseLimit
	if limit == 0 {
		limit = responseLimit
	}
	raw, err := io.ReadAll(io.LimitReader(response.Body, int64(limit)+1))
	diagnostic.ElapsedMS = time.Since(started).Milliseconds()
	defer clear(raw)
	if err != nil || len(raw) > limit || !validJSON(raw) || json.Unmarshal(raw, output) != nil {
		diagnostic.Category = "response_unrecognized"
		var timeout net.Error
		if errors.Is(err, context.Canceled) {
			diagnostic.Category = "cancelled"
		} else if errors.Is(err, context.DeadlineExceeded) || errors.As(err, &timeout) && timeout.Timeout() {
			diagnostic.Category = "deadline"
		}
		return response.Header, &Failure{Code: core.ErrorCodeOutcomeUnknown, Unknown: true, RequestID: requestID, Diagnostic: diagnostic}
	}
	return response.Header, nil
}

// ValidJSONObject rejects scalar/null envelopes, trailing values and duplicate
// fields (including nested duplicates) before a typed decoder loses presence.
func ValidJSONObject(raw []byte) bool {
	trimmed := bytes.TrimSpace(raw)
	return len(trimmed) > 0 && trimmed[0] == '{' && validJSON(raw)
}
func validJSON(raw []byte) bool {
	d := json.NewDecoder(bytes.NewReader(raw))
	d.UseNumber()
	var value func() bool
	value = func() bool {
		t, err := d.Token()
		if err != nil {
			return false
		}
		delim, ok := t.(json.Delim)
		if !ok {
			return true
		}
		switch delim {
		case '{':
			keys := map[string]bool{}
			for d.More() {
				key, err := d.Token()
				if err != nil {
					return false
				}
				name, ok := key.(string)
				if !ok || keys[name] {
					return false
				}
				keys[name] = true
				if !value() {
					return false
				}
			}
			end, err := d.Token()
			return err == nil && end == json.Delim('}')
		case '[':
			for d.More() {
				if !value() {
					return false
				}
			}
			end, err := d.Token()
			return err == nil && end == json.Delim(']')
		default:
			return false
		}
	}
	if !value() {
		return false
	}
	_, err := d.Token()
	return errors.Is(err, io.EOF)
}

func SafeText(value string, limit int) string {
	clean := strings.Map(func(r rune) rune {
		if unicode.IsControl(r) && r != '\n' && r != '\t' {
			return -1
		}
		return r
	}, value)
	runes := []rune(clean)
	if len(runes) > limit {
		return string(runes[:limit])
	}
	return clean
}
func PublicURL(raw string) string {
	u, err := url.Parse(strings.TrimSpace(raw))
	if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Hostname() == "" || u.User != nil || len(raw) > 2048 {
		return ""
	}
	return u.String()
}

func Redact(value any, key string) any {
	switch typed := value.(type) {
	case string:
		if key != "" {
			return strings.ReplaceAll(typed, key, "[redacted]")
		}
	case []any:
		for i, item := range typed {
			typed[i] = Redact(item, key)
		}
	case map[string]any:
		for name, item := range typed {
			typed[name] = Redact(item, key)
		}
	}
	return value
}

func SafeRequestID(value, key string) string {
	if len(value) == 0 || len(value) > 128 || key != "" && strings.Contains(value, key) {
		return ""
	}
	for _, r := range value {
		if !(r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z' || r >= '0' && r <= '9' || strings.ContainsRune("._:-", r)) {
			return ""
		}
	}
	return value
}
