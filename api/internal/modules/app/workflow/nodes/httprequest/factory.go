package httprequest

import (
	"context"
	"fmt"
	"strings"

	"github.com/zgiai/ginext/internal/modules/app/workflow/graph_engine/entities"
)

// HTTPRequestFactory HTTP request node factory
type HTTPRequestFactory struct{}

// NewHTTPRequestFactory creates HTTP request node factory
func NewHTTPRequestFactory() *HTTPRequestFactory {
	return &HTTPRequestFactory{}
}

// CreateProcessor creates HTTP request processor
func (f *HTTPRequestFactory) CreateProcessor(
	nodeData *NodeData,
	timeout *HttpRequestNodeTimeout,
	variablePool *entities.VariablePool,
	maxRetries int,
	fileService fileDownloader,
) *HTTPRequestProcessor {
	return NewHTTPRequestProcessor(nodeData, timeout, variablePool, maxRetries, fileService)
}

// GetDefaultConfig gets default configuration
func (f *HTTPRequestFactory) GetDefaultConfig() map[string]interface{} {
	defaultTimeout := NewDefaultTimeout()
	sslVerify := HTTPRequestNodeSSLVerify

	return map[string]interface{}{
		"type": "http-request",
		"config": map[string]interface{}{
			"method": string(HTTPMethodGet),
			"authorization": map[string]interface{}{
				"type": string(AuthorizationTypeNoAuth),
			},
			"body": map[string]interface{}{
				"type": string(BodyTypeNone),
			},
			"timeout": map[string]interface{}{
				"connect":             defaultTimeout.Connect,
				"read":                defaultTimeout.Read,
				"write":               defaultTimeout.Write,
				"max_connect_timeout": HTTPRequestMaxConnectTimeout,
				"max_read_timeout":    HTTPRequestMaxReadTimeout,
				"max_write_timeout":   HTTPRequestMaxWriteTimeout,
			},
			"ssl_verify": sslVerify,
		},
		"retry_config": map[string]interface{}{
			"max_retries":    SSRFDefaultMaxRetries,
			"retry_interval": 2.0, // 0.5 * (2^2)
			"retry_enabled":  true,
		},
	}
}

// HTTPRequestExecutor HTTP request executor
type HTTPRequestExecutor struct {
	factory      *HTTPRequestFactory
	nodeData     *NodeData
	variablePool *entities.VariablePool
	timeout      *HttpRequestNodeTimeout
	maxRetries   int
	fileService  fileDownloader
}

// NewHTTPRequestExecutor creates HTTP request executor
func NewHTTPRequestExecutor(
	nodeData *NodeData,
	variablePool *entities.VariablePool,
	timeout *HttpRequestNodeTimeout,
	maxRetries int,
	fileService fileDownloader,
) *HTTPRequestExecutor {
	if timeout == nil {
		timeout = NewDefaultTimeout()
	}

	if maxRetries < 0 {
		maxRetries = SSRFDefaultMaxRetries
	}

	return &HTTPRequestExecutor{
		factory:      NewHTTPRequestFactory(),
		nodeData:     nodeData,
		variablePool: variablePool,
		timeout:      timeout,
		maxRetries:   maxRetries,
		fileService:  fileService,
	}
}

// Execute executes HTTP request
func (e *HTTPRequestExecutor) Execute(ctx context.Context) (*Response, error) {
	// Validate node data
	if err := e.nodeData.Validate(); err != nil {
		return nil, err
	}

	// Create processor
	processor := e.factory.CreateProcessor(
		e.nodeData,
		e.timeout,
		e.variablePool,
		e.maxRetries,
		e.fileService,
	)

	// Execute request
	return processor.Execute(ctx)
}

// ToLog generates execution log
func (e *HTTPRequestExecutor) ToLog() map[string]interface{} {
	processor := e.factory.CreateProcessor(
		e.nodeData,
		e.timeout,
		e.variablePool,
		e.maxRetries,
		e.fileService,
	)

	return processor.ToLog()
}

// HTTPRequestResult HTTP request result
type HTTPRequestResult struct {
	StatusCode int                    `json:"status_code"`
	Body       string                 `json:"body"`
	Headers    map[string]string      `json:"headers"`
	Files      []interface{}          `json:"files"`
	Request    map[string]interface{} `json:"request"`
}

// NewHTTPRequestResult creates HTTP request result
func NewHTTPRequestResult(response *Response, executor *HTTPRequestExecutor) *HTTPRequestResult {
	result := &HTTPRequestResult{
		StatusCode: response.StatusCode,
		Headers:    response.Headers,
		Files:      []interface{}{},
		Request:    executor.ToLog(),
	}

	// If it's a file, don't return body content
	if response.IsFile() {
		result.Body = ""
		// TODO: This should handle file upload to file manager
		// result.Files = processFiles(response)
	} else {
		result.Body = response.Text()
	}

	return result
}

// HTTPRequestResultBuilder HTTP request result builder
type HTTPRequestResultBuilder struct {
	response *Response
	executor *HTTPRequestExecutor
	err      error
}

// NewHTTPRequestResultBuilder creates result builder
func NewHTTPRequestResultBuilder() *HTTPRequestResultBuilder {
	return &HTTPRequestResultBuilder{}
}

// WithResponse sets response
func (b *HTTPRequestResultBuilder) WithResponse(response *Response) *HTTPRequestResultBuilder {
	b.response = response
	return b
}

// WithExecutor sets executor
func (b *HTTPRequestResultBuilder) WithExecutor(executor *HTTPRequestExecutor) *HTTPRequestResultBuilder {
	b.executor = executor
	return b
}

// WithError sets error
func (b *HTTPRequestResultBuilder) WithError(err error) *HTTPRequestResultBuilder {
	b.err = err
	return b
}

// Build builds result
func (b *HTTPRequestResultBuilder) Build() (*HTTPRequestResult, error) {
	if b.err != nil {
		return nil, b.err
	}

	if b.response == nil || b.executor == nil {
		return nil, fmt.Errorf("response and executor are required")
	}

	return NewHTTPRequestResult(b.response, b.executor), nil
}

// HTTPRequestHelper HTTP request helper tools
type HTTPRequestHelper struct{}

// NewHTTPRequestHelper creates HTTP request helper tools
func NewHTTPRequestHelper() *HTTPRequestHelper {
	return &HTTPRequestHelper{}
}

// ValidateMethod validates HTTP method
func (h *HTTPRequestHelper) ValidateMethod(method HTTPMethod) error {
	validMethods := []HTTPMethod{
		HTTPMethodGET, HTTPMethodPOST, HTTPMethodPUT, HTTPMethodPATCH,
		HTTPMethodDELETE, HTTPMethodHEAD, HTTPMethodOPTIONS,
		HTTPMethodGet, HTTPMethodPost, HTTPMethodPut, HTTPMethodPatch,
		HTTPMethodDelete, HTTPMethodHead, HTTPMethodOptions,
	}

	for _, validMethod := range validMethods {
		if method == validMethod {
			return nil
		}
	}

	return NewInvalidHTTPMethodError(fmt.Sprintf("invalid HTTP method: %s", method))
}

// ValidateURL validates URL
func (h *HTTPRequestHelper) ValidateURL(url string) error {
	if url == "" {
		return NewInvalidURLError("URL is required")
	}

	if !strings.HasPrefix(url, "http://") && !strings.HasPrefix(url, "https://") {
		return NewInvalidURLError("URL should start with http:// or https://")
	}

	return nil
}

// ParseHeaders parses header string
func (h *HTTPRequestHelper) ParseHeaders(headers string) (map[string]string, error) {
	result := make(map[string]string)

	if headers == "" {
		return result, nil
	}

	lines := strings.Split(headers, "\n")
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}

		parts := strings.SplitN(line, ":", 2)
		if len(parts) == 2 {
			key := strings.TrimSpace(parts[0])
			value := strings.TrimSpace(parts[1])
			if key != "" {
				result[key] = value
			}
		}
	}

	return result, nil
}

// ParseParams parses parameter string
func (h *HTTPRequestHelper) ParseParams(params string) (map[string]string, error) {
	result := make(map[string]string)

	if params == "" {
		return result, nil
	}

	lines := strings.Split(params, "\n")
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}

		parts := strings.SplitN(line, ":", 2)
		if len(parts) == 2 {
			key := strings.TrimSpace(parts[0])
			value := strings.TrimSpace(parts[1])
			if key != "" {
				result[key] = value
			}
		}
	}

	return result, nil
}
