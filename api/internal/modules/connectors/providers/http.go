package providers

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
	"unicode"

	"github.com/zgiai/zgi/api/internal/modules/connectors/core"
)

const responseLimit = 5 << 20

type HTTP struct {
	client *http.Client
	origin string
}

func NewHTTP(client *http.Client, origin string) *HTTP {
	return NewBoundedHTTP(client, origin, 20*time.Second)
}

func NewBoundedHTTP(client *http.Client, origin string, timeout time.Duration) *HTTP {
	if timeout < time.Second || timeout > 40*time.Second {
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
	request, err := http.NewRequestWithContext(ctx, method, endpoint, bytes.NewReader(data))
	if err != nil {
		return nil, Invalid()
	}
	request.Header = headers.Clone()
	request.Header.Set("Accept", "application/json")
	request.Header.Set("Content-Type", "application/json")
	response, err := client.client.Do(request)
	if err != nil {
		return nil, &Failure{Code: core.ErrorCodeProviderUnavailable, Unknown: true}
	}
	defer response.Body.Close()
	requestID := SafeText(response.Header.Get("X-GitHub-Request-Id"), 128)
	if requestID == "" {
		requestID = SafeText(response.Header.Get("X-Request-Id"), 128)
	}
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		failure := &Failure{Code: core.ErrorCodeProviderUnavailable, RequestID: requestID}
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
	raw, err := io.ReadAll(io.LimitReader(response.Body, responseLimit+1))
	defer clear(raw)
	if err != nil || len(raw) > responseLimit || json.Unmarshal(raw, output) != nil {
		return response.Header, &Failure{Code: core.ErrorCodeOutcomeUnknown, Unknown: true, RequestID: requestID}
	}
	return response.Header, nil
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
