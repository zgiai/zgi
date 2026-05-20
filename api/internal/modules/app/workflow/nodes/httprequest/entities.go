package httprequest

import (
	"fmt"
	"mime"
	"net/http"
	"strings"
	"unicode/utf8"

	"github.com/zgiai/ginext/internal/modules/app/workflow/nodes/base"
)

// Configuration constants
const (
	// HTTP request timeout configuration
	HTTPRequestMaxConnectTimeout = 10 // seconds
	HTTPRequestMaxReadTimeout    = 60 // seconds
	HTTPRequestMaxWriteTimeout   = 20 // seconds

	// HTTP request size limits
	HTTPRequestNodeMaxBinarySize = 10 * 1024 * 1024 // 10MB
	HTTPRequestNodeMaxTextSize   = 1 * 1024 * 1024  // 1MB

	// SSL verification configuration
	HTTPRequestNodeSSLVerify = true

	// SSRF protection configuration
	SSRFDefaultMaxRetries = 3
)

// HTTPMethod HTTP method type
type HTTPMethod string

const (
	HTTPMethodGET     HTTPMethod = "GET"
	HTTPMethodPOST    HTTPMethod = "POST"
	HTTPMethodPUT     HTTPMethod = "PUT"
	HTTPMethodPATCH   HTTPMethod = "PATCH"
	HTTPMethodDELETE  HTTPMethod = "DELETE"
	HTTPMethodHEAD    HTTPMethod = "HEAD"
	HTTPMethodOPTIONS HTTPMethod = "OPTIONS"
	// Lowercase versions (compatibility)
	HTTPMethodGet     HTTPMethod = "get"
	HTTPMethodPost    HTTPMethod = "post"
	HTTPMethodPut     HTTPMethod = "put"
	HTTPMethodPatch   HTTPMethod = "patch"
	HTTPMethodDelete  HTTPMethod = "delete"
	HTTPMethodHead    HTTPMethod = "head"
	HTTPMethodOptions HTTPMethod = "options"
)

// AuthorizationType authentication type
type AuthorizationType string

const (
	AuthorizationTypeNoAuth AuthorizationType = "no-auth"
	AuthorizationTypeAPIKey AuthorizationType = "api-key"
)

// AuthorizationConfigType authentication configuration type
type AuthorizationConfigType string

const (
	AuthorizationConfigTypeBasic  AuthorizationConfigType = "basic"
	AuthorizationConfigTypeBearer AuthorizationConfigType = "bearer"
	AuthorizationConfigTypeCustom AuthorizationConfigType = "custom"
)

// BodyDataType request body data type
type BodyDataType string

const (
	BodyDataTypeFile BodyDataType = "file"
	BodyDataTypeText BodyDataType = "text"
)

// FileInputMode specifies how file fields are sent.
type FileInputMode string

const (
	FileInputModeUpload FileInputMode = "upload"
	FileInputModeURL    FileInputMode = "url"
)

// BodyType request body type
type BodyType string

const (
	BodyTypeNone       BodyType = "none"
	BodyTypeFormData   BodyType = "form-data"
	BodyTypeURLEncoded BodyType = "x-www-form-urlencoded"
	BodyTypeRawText    BodyType = "raw-text"
	BodyTypeJSON       BodyType = "json"
	BodyTypeBinary     BodyType = "binary"
)

// Request body content type mapping
var BodyTypeToContentType = map[BodyType]string{
	BodyTypeJSON:       "application/json",
	BodyTypeURLEncoded: "application/x-www-form-urlencoded",
	BodyTypeFormData:   "multipart/form-data",
	BodyTypeRawText:    "text/plain",
}

// HttpRequestNodeAuthorizationConfig authentication configuration
type HttpRequestNodeAuthorizationConfig struct {
	Type   AuthorizationConfigType `json:"type" validate:"required,oneof=basic bearer custom"`
	APIKey string                  `json:"api_key" validate:"required"`
	Header string                  `json:"header,omitempty"`
}

// HttpRequestNodeAuthorization authentication information
type HttpRequestNodeAuthorization struct {
	Type   AuthorizationType                   `json:"type" validate:"required,oneof=no-auth api-key"`
	Config *HttpRequestNodeAuthorizationConfig `json:"config,omitempty"`
}

// Validate validates authentication configuration
func (auth *HttpRequestNodeAuthorization) Validate() error {
	if auth.Type == AuthorizationTypeNoAuth {
		if auth.Config != nil {
			return fmt.Errorf("config should be nil when type is no-auth")
		}
	} else if auth.Type == AuthorizationTypeAPIKey {
		if auth.Config == nil {
			return fmt.Errorf("config is required when type is api-key")
		}
		if auth.Config.APIKey == "" {
			return fmt.Errorf("api_key is required")
		}
	}
	return nil
}

// BodyData request body data item
type BodyData struct {
	Key   string        `json:"key,omitempty"`
	Type  BodyDataType  `json:"type" validate:"required,oneof=file text"`
	Value string        `json:"value,omitempty"`
	File  []string      `json:"file,omitempty"`
	Mode  FileInputMode `json:"mode,omitempty"`
}

// HttpRequestNodeBody request body configuration
type HttpRequestNodeBody struct {
	Type BodyType   `json:"type" validate:"required,oneof=none form-data x-www-form-urlencoded raw-text json binary"`
	Data []BodyData `json:"data,omitempty"`
}

// Validate validates request body configuration
func (body *HttpRequestNodeBody) Validate() error {
	if body.Data == nil {
		body.Data = []BodyData{}
	}
	return nil
}

// HttpRequestNodeTimeout timeout configuration
type HttpRequestNodeTimeout struct {
	Connect int `json:"connect" validate:"min=1"`
	Read    int `json:"read" validate:"min=1"`
	Write   int `json:"write" validate:"min=1"`
}

// NewDefaultTimeout creates default timeout configuration
func NewDefaultTimeout() *HttpRequestNodeTimeout {
	return &HttpRequestNodeTimeout{
		Connect: HTTPRequestMaxConnectTimeout,
		Read:    HTTPRequestMaxReadTimeout,
		Write:   HTTPRequestMaxWriteTimeout,
	}
}

// NodeData HTTP request node data
type NodeData struct {
	base.NodeData

	// HTTP configuration
	Method        HTTPMethod                   `json:"method" validate:"required"`
	URL           string                       `json:"url" validate:"required,url"`
	Authorization HttpRequestNodeAuthorization `json:"authorization"`
	Headers       string                       `json:"headers,omitempty"`
	Params        string                       `json:"params,omitempty"`
	Body          *HttpRequestNodeBody         `json:"body,omitempty"`
	Timeout       *HttpRequestNodeTimeout      `json:"timeout,omitempty"`
	SSLVerify     *bool                        `json:"ssl_verify,omitempty"`

	defaultValueEntries []defaultValueEntry `json:"-"`
}

// Validate validates node data
func (nd *NodeData) Validate() error {
	if nd.URL == "" {
		return fmt.Errorf("url is required")
	}

	if !strings.HasPrefix(nd.URL, "http://") && !strings.HasPrefix(nd.URL, "https://") {
		return fmt.Errorf("url should start with http:// or https://")
	}

	if err := nd.Authorization.Validate(); err != nil {
		return fmt.Errorf("authorization validation failed: %w", err)
	}

	if nd.Body != nil {
		if err := nd.Body.Validate(); err != nil {
			return fmt.Errorf("body validation failed: %w", err)
		}
	}

	// Set default values
	if nd.Timeout == nil {
		nd.Timeout = NewDefaultTimeout()
	}

	if nd.SSLVerify == nil {
		sslVerify := HTTPRequestNodeSSLVerify
		nd.SSLVerify = &sslVerify
	}

	return nil
}

// GetSSLVerify gets SSL verification setting
func (nd *NodeData) GetSSLVerify() bool {
	if nd.SSLVerify == nil {
		return HTTPRequestNodeSSLVerify
	}
	return *nd.SSLVerify
}

// Response HTTP response wrapper
type Response struct {
	Headers     map[string]string `json:"headers"`
	StatusCode  int               `json:"status_code"`
	Body        []byte            `json:"body"`
	ContentType string            `json:"content_type"`
}

// NewResponse creates a new response instance
func NewResponse(resp *http.Response, body []byte) *Response {
	headers := make(map[string]string)
	for key, values := range resp.Header {
		if len(values) > 0 {
			headers[key] = values[0]
		}
	}

	return &Response{
		Headers:     headers,
		StatusCode:  resp.StatusCode,
		Body:        body,
		ContentType: resp.Header.Get("Content-Type"),
	}
}

// IsFile determines if response is a file
func (r *Response) IsFile() bool {
	contentType := strings.ToLower(strings.Split(r.ContentType, ";")[0])

	// Check Content-Disposition header
	contentDisposition := r.Headers["Content-Disposition"]
	if contentDisposition != "" {
		if strings.Contains(strings.ToLower(contentDisposition), "attachment") {
			return true
		}
		if strings.Contains(contentDisposition, "filename=") {
			return true
		}
	}

	// For text/ types, only CSV should be downloaded as file
	if strings.HasPrefix(contentType, "text/") {
		return strings.Contains(contentType, "csv")
	}

	// For application types, check if it's text format
	if strings.HasPrefix(contentType, "application/") {
		// Common text formats
		textTypes := []string{"json", "xml", "javascript", "x-www-form-urlencoded", "yaml", "graphql"}
		for _, textType := range textTypes {
			if strings.Contains(contentType, textType) {
				return false
			}
		}

		// Try to detect if content is text
		if len(r.Body) > 0 {
			// Check first 1024 bytes
			sample := r.Body
			if len(sample) > 1024 {
				sample = sample[:1024]
			}

			// If can be parsed as UTF-8 and contains common text markers, might not be a file
			if utf8.Valid(sample) {
				textMarkers := [][]byte{
					[]byte("{"), []byte("["), []byte("<"),
					[]byte("function"), []byte("var "), []byte("const "), []byte("let "),
				}
				for _, marker := range textMarkers {
					if strings.Contains(string(sample), string(marker)) {
						return false
					}
				}
			}
		}
	}

	// Judge based on MIME type
	mainType := strings.Split(contentType, "/")[0]
	mediaTypes := []string{"image", "audio", "video", "application"}
	for _, mediaType := range mediaTypes {
		if mainType == mediaType {
			return true
		}
	}

	// Check if it's media type
	mediaContentTypes := []string{"image/", "audio/", "video/"}
	for _, mediaContentType := range mediaContentTypes {
		if strings.HasPrefix(contentType, mediaContentType) {
			return true
		}
	}

	return false
}

// GetContentType gets content type
func (r *Response) GetContentType() string {
	return r.Headers["Content-Type"]
}

// Text gets response text content
func (r *Response) Text() string {
	return string(r.Body)
}

// Content gets response binary content
func (r *Response) Content() []byte {
	return r.Body
}

// Size gets response size
func (r *Response) Size() int {
	return len(r.Body)
}

// ReadableSize gets readable response size
func (r *Response) ReadableSize() string {
	size := r.Size()
	if size < 1024 {
		return fmt.Sprintf("%d bytes", size)
	} else if size < 1024*1024 {
		return fmt.Sprintf("%.2f KB", float64(size)/1024)
	} else {
		return fmt.Sprintf("%.2f MB", float64(size)/(1024*1024))
	}
}

// GetFilename extracts filename from Content-Disposition header
func (r *Response) GetFilename() string {
	contentDisposition := r.Headers["Content-Disposition"]
	if contentDisposition == "" {
		return ""
	}

	// Simple parsing of filename= parameter
	if idx := strings.Index(contentDisposition, "filename="); idx != -1 {
		filename := contentDisposition[idx+9:] // len("filename=") = 9
		filename = strings.Trim(filename, `"'`)
		if idx := strings.Index(filename, ";"); idx != -1 {
			filename = filename[:idx]
		}
		return strings.TrimSpace(filename)
	}

	return ""
}

// GuessFileExtension guesses file extension based on content type
func (r *Response) GuessFileExtension() string {
	contentType := r.GetContentType()
	if contentType == "" {
		return ""
	}

	// Remove parameter part
	mimeType := strings.Split(contentType, ";")[0]
	mimeType = strings.TrimSpace(mimeType)

	// Get extension
	exts, err := mime.ExtensionsByType(mimeType)
	if err != nil || len(exts) == 0 {
		return ""
	}

	// Return first extension
	return exts[0]
}

// HTTPRequestNodeError HTTP request node error base class
type HTTPRequestNodeError struct {
	Message string
	Cause   error
}

func (e HTTPRequestNodeError) Error() string {
	if e.Cause != nil {
		return fmt.Sprintf("%s: %v", e.Message, e.Cause)
	}
	return e.Message
}

// Specific error types
type (
	// AuthorizationConfigError authentication configuration error
	AuthorizationConfigError struct {
		HTTPRequestNodeError
	}

	// FileFetchError file fetch error
	FileFetchError struct {
		HTTPRequestNodeError
	}

	// InvalidHTTPMethodError invalid HTTP method error
	InvalidHTTPMethodError struct {
		HTTPRequestNodeError
	}

	// ResponseSizeError response size error
	ResponseSizeError struct {
		HTTPRequestNodeError
	}

	// RequestBodyError request body error
	RequestBodyError struct {
		HTTPRequestNodeError
	}

	// InvalidURLError invalid URL error
	InvalidURLError struct {
		HTTPRequestNodeError
	}
)

// Error constructor functions
func NewAuthorizationConfigError(message string) *AuthorizationConfigError {
	return &AuthorizationConfigError{
		HTTPRequestNodeError: HTTPRequestNodeError{Message: message},
	}
}

func NewFileFetchError(message string) *FileFetchError {
	return &FileFetchError{
		HTTPRequestNodeError: HTTPRequestNodeError{Message: message},
	}
}

func NewInvalidHTTPMethodError(message string) *InvalidHTTPMethodError {
	return &InvalidHTTPMethodError{
		HTTPRequestNodeError: HTTPRequestNodeError{Message: message},
	}
}

func NewResponseSizeError(message string) *ResponseSizeError {
	return &ResponseSizeError{
		HTTPRequestNodeError: HTTPRequestNodeError{Message: message},
	}
}

func NewRequestBodyError(message string) *RequestBodyError {
	return &RequestBodyError{
		HTTPRequestNodeError: HTTPRequestNodeError{Message: message},
	}
}

func NewInvalidURLError(message string) *InvalidURLError {
	return &InvalidURLError{
		HTTPRequestNodeError: HTTPRequestNodeError{Message: message},
	}
}
