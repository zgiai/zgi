package httprequest

import (
	"bytes"
	"context"
	"crypto/tls"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"mime"
	"mime/multipart"
	"net/http"
	"net/textproto"
	"net/url"
	"strings"
	"time"

	workflowfile "github.com/zgiai/ginext/internal/modules/app/workflow/file"
	"github.com/zgiai/ginext/internal/modules/app/workflow/graph_engine/entities"
	"github.com/zgiai/ginext/internal/modules/app/workflow/shared"
	"github.com/zgiai/ginext/internal/observability"
	"github.com/zgiai/ginext/pkg/logger"
	"go.uber.org/zap"
)

// HTTPRequestProcessor HTTP request processor
type HTTPRequestProcessor struct {
	nodeData     *NodeData
	timeout      *HttpRequestNodeTimeout
	variablePool *entities.VariablePool
	maxRetries   int
	client       *http.Client
	ctx          context.Context
	fileService  fileDownloader
}

type fileDownloader interface {
	DownloadFile(ctx context.Context, fileID string) ([]byte, error)
}

// NewHTTPRequestProcessor creates new HTTP request processor
func NewHTTPRequestProcessor(
	nodeData *NodeData,
	timeout *HttpRequestNodeTimeout,
	variablePool *entities.VariablePool,
	maxRetries int,
	fileService fileDownloader,
) *HTTPRequestProcessor {
	if timeout == nil {
		timeout = NewDefaultTimeout()
	}

	// Create HTTP client
	transport := &http.Transport{
		TLSClientConfig: &tls.Config{
			InsecureSkipVerify: !nodeData.GetSSLVerify(),
		},
	}

	client := &http.Client{
		Transport: observability.HTTPTransport(transport),
		Timeout:   time.Duration(timeout.Read) * time.Second,
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			// Allow redirects, maximum 10 times
			if len(via) >= 10 {
				return fmt.Errorf("stopped after 10 redirects")
			}
			return nil
		},
	}

	return &HTTPRequestProcessor{
		nodeData:     nodeData,
		timeout:      timeout,
		variablePool: variablePool,
		maxRetries:   maxRetries,
		client:       client,
		fileService:  fileService,
	}
}

// Execute executes HTTP request
func (p *HTTPRequestProcessor) Execute(ctx context.Context) (*Response, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	p.ctx = ctx
	logCtx := logger.WithFields(ctx,
		zap.String("node_type", "http-request"),
		zap.String("method", string(p.nodeData.Method)),
		zap.Int("max_retries", p.maxRetries),
	)
	logger.DebugContext(logCtx, "HTTP request node execution started")

	// 1. Initialize URL
	processedURL, err := p.processURL()
	if err != nil {
		logger.ErrorContext(logCtx, "failed to process HTTP request node URL", err)
		return nil, NewInvalidURLError(fmt.Sprintf("failed to process URL: %v", err))
	}
	logCtx = logger.WithFields(logCtx, sanitizedURLFields(processedURL)...)
	logger.DebugContext(logCtx, "HTTP request node URL processed")

	// 2. Initialize parameters
	logger.DebugContext(logCtx, "processing HTTP request node parameters")
	params, err := p.processParams()
	if err != nil {
		logger.ErrorContext(logCtx, "failed to process HTTP request node parameters", err)
		return nil, fmt.Errorf("failed to process params: %w", err)
	}
	logger.DebugContext(logCtx, "HTTP request node parameters processed",
		zap.Int("params_count", len(params)),
	)

	// 3. Add query parameters to URL
	if len(params) > 0 {
		processedURL = p.addParamsToURL(processedURL, params)
		logger.DebugContext(logCtx, "HTTP request node query parameters appended",
			zap.Int("params_count", len(params)),
		)
	}

	// 4. Initialize request body
	logger.DebugContext(logCtx, "processing HTTP request node body")
	body, contentType, err := p.processBody()
	if err != nil {
		logger.ErrorContext(logCtx, "failed to process HTTP request node body", err)
		return nil, NewRequestBodyError(fmt.Sprintf("failed to process body: %v", err))
	}
	logger.DebugContext(logCtx, "HTTP request node body processed",
		zap.String("content_type", contentType),
		zap.Bool("has_body", body != nil),
	)

	// 5. Create request
	logger.DebugContext(logCtx, "creating outbound HTTP request")
	req, err := http.NewRequestWithContext(ctx, string(p.nodeData.Method), processedURL, body)
	if err != nil {
		logger.ErrorContext(logCtx, "failed to create outbound HTTP request", err)
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	// 6. Set request headers
	logger.DebugContext(logCtx, "setting outbound HTTP request headers")
	if err := p.setHeaders(req, contentType); err != nil {
		logger.ErrorContext(logCtx, "failed to set outbound HTTP request headers", err)
		return nil, fmt.Errorf("failed to set headers: %w", err)
	}

	// 7. Set authentication
	logger.DebugContext(logCtx, "setting outbound HTTP request authentication")
	if err := p.setAuthentication(req); err != nil {
		logger.ErrorContext(logCtx, "failed to set outbound HTTP request authentication", err)
		return nil, NewAuthorizationConfigError(fmt.Sprintf("failed to set authentication: %v", err))
	}

	// 8. Execute request
	logger.DebugContext(logCtx, "executing outbound HTTP request")
	response, err := p.doRequest(req)
	if err != nil {
		logger.CriticalContext(logCtx, "outbound HTTP request failed", err)
		return nil, err
	}
	logger.DebugContext(logCtx, "outbound HTTP request completed",
		zap.Int("status", response.StatusCode),
	)
	return response, nil
}

func sanitizedURLFields(rawURL string) []zap.Field {
	parsedURL, err := url.Parse(rawURL)
	if err != nil {
		return []zap.Field{zap.Bool("url_parse_error", true)}
	}
	return []zap.Field{
		zap.String("url_scheme", parsedURL.Scheme),
		zap.String("url_host", parsedURL.Host),
		zap.String("path", parsedURL.Path),
		zap.Bool("has_query", parsedURL.RawQuery != ""),
	}
}

func (p *HTTPRequestProcessor) resolveTemplateValue(input string) string {
	if input == "" || !strings.Contains(input, "{{#") || p.variablePool == nil {
		return input
	}
	segment := p.variablePool.ConvertTemplate(input)
	if segment == nil {
		return input
	}
	text := segment.Text()
	if text != input || !strings.Contains(input, "#}}") {
		return text
	}
	return p.resolveTemplateFallback(input)
}

func (p *HTTPRequestProcessor) resolveTemplateFallback(input string) string {
	var out strings.Builder
	remaining := input

	for {
		start := strings.Index(remaining, "{{#")
		if start == -1 {
			out.WriteString(remaining)
			break
		}
		out.WriteString(remaining[:start])

		rest := remaining[start+3:]
		end := strings.Index(rest, "#}}")
		if end == -1 {
			out.WriteString(remaining[start:])
			break
		}

		varPath := strings.TrimSpace(rest[:end])
		replacement := "{{#" + varPath + "#}}"
		if varPath != "" && p.variablePool != nil {
			selector := strings.Split(varPath, ".")
			if variable := p.variablePool.GetWithPath(selector); variable != nil {
				replacement = variable.Text()
			}
		}

		out.WriteString(replacement)
		remaining = rest[end+3:]
	}

	return out.String()
}

// processURL processes URL template
func (p *HTTPRequestProcessor) processURL() (string, error) {
	processedURL := p.resolveTemplateValue(p.nodeData.URL)
	if processedURL == "" {
		return "", NewInvalidURLError("URL is required")
	}

	if !strings.HasPrefix(processedURL, "http://") && !strings.HasPrefix(processedURL, "https://") {
		return "", NewInvalidURLError("URL should start with http:// or https://")
	}

	return processedURL, nil
}

func (p *HTTPRequestProcessor) GetURL() string {
	processedURL, _ := p.processURL()
	return processedURL
}

// processParams processes query parameters
func (p *HTTPRequestProcessor) processParams() (map[string]string, error) {
	params := make(map[string]string)

	if p.nodeData.Params == "" {
		return params, nil
	}

	lines := strings.Split(p.nodeData.Params, "\n")
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
				processedKey := p.resolveTemplateValue(key)
				processedValue := p.resolveTemplateValue(value)
				params[processedKey] = processedValue
			}
		}
	}

	return params, nil
}

// addParamsToURL adds query parameters to URL
func (p *HTTPRequestProcessor) addParamsToURL(rawURL string, params map[string]string) string {
	u, err := url.Parse(rawURL)
	if err != nil {
		return rawURL
	}

	query := u.Query()
	for key, value := range params {
		query.Add(key, value)
	}

	u.RawQuery = query.Encode()
	return u.String()
}

// processBody processes request body
func (p *HTTPRequestProcessor) processBody() (io.Reader, string, error) {
	if p.nodeData.Body == nil || p.nodeData.Body.Type == BodyTypeNone {
		return nil, "", nil
	}

	switch p.nodeData.Body.Type {
	case BodyTypeRawText:
		return p.processRawTextBody()
	case BodyTypeJSON:
		return p.processJSONBody()
	case BodyTypeURLEncoded:
		return p.processURLEncodedBody()
	case BodyTypeFormData:
		return p.processFormDataBody()
	case BodyTypeBinary:
		return p.processBinaryBody()
	default:
		return nil, "", NewRequestBodyError(fmt.Sprintf("unsupported body type: %s", p.nodeData.Body.Type))
	}
}

// processRawTextBody processes raw text request body
func (p *HTTPRequestProcessor) processRawTextBody() (io.Reader, string, error) {
	if len(p.nodeData.Body.Data) != 1 {
		return nil, "", NewRequestBodyError("raw-text body type should have exactly one item")
	}

	content := p.resolveTemplateValue(p.nodeData.Body.Data[0].Value)
	return strings.NewReader(content), "text/plain", nil
}

// processJSONBody processes JSON request body
func (p *HTTPRequestProcessor) processJSONBody() (io.Reader, string, error) {
	if len(p.nodeData.Body.Data) != 1 {
		return nil, "", NewRequestBodyError("json body type should have exactly one item")
	}

	jsonString := p.resolveTemplateValue(p.nodeData.Body.Data[0].Value)

	// Validate JSON format
	var jsonObj interface{}
	if err := json.Unmarshal([]byte(jsonString), &jsonObj); err != nil {
		return nil, "", NewRequestBodyError(fmt.Sprintf("invalid JSON: %v", err))
	}

	return strings.NewReader(jsonString), "application/json", nil
}

// processURLEncodedBody processes URL encoded request body
func (p *HTTPRequestProcessor) processURLEncodedBody() (io.Reader, string, error) {
	values := url.Values{}

	for _, item := range p.nodeData.Body.Data {
		if item.Key != "" {
			values.Add(p.resolveTemplateValue(item.Key), p.resolveTemplateValue(item.Value))
		}
	}

	return strings.NewReader(values.Encode()), "application/x-www-form-urlencoded", nil
}

// processFormDataBody processes form data request body
func (p *HTTPRequestProcessor) processFormDataBody() (io.Reader, string, error) {
	var buf bytes.Buffer
	writer := multipart.NewWriter(&buf)

	for _, item := range p.nodeData.Body.Data {
		switch item.Type {
		case BodyDataTypeText:
			if item.Key == "" {
				continue
			}
			value := p.resolveTemplateValue(item.Value)
			if err := writer.WriteField(item.Key, value); err != nil {
				return nil, "", fmt.Errorf("write form field: %w", err)
			}
		case BodyDataTypeFile:
			if item.Key == "" {
				return nil, "", fmt.Errorf("file field key is required")
			}
			if len(item.File) == 0 {
				return nil, "", fmt.Errorf("file selector is required for key %s", item.Key)
			}
			files, err := p.resolveFiles(item.File)
			if err != nil {
				return nil, "", err
			}
			if len(files) == 0 {
				return nil, "", fmt.Errorf("file selector for key %s resolved to empty", item.Key)
			}

			mode := item.Mode
			if mode == "" {
				mode = FileInputModeUpload
			}

			switch mode {
			case FileInputModeURL:
				for _, file := range files {
					url, err := p.resolveFileURL(file)
					if err != nil {
						return nil, "", err
					}
					if err := writer.WriteField(item.Key, url); err != nil {
						return nil, "", fmt.Errorf("write form field: %w", err)
					}
				}
			case FileInputModeUpload:
				if p.fileService == nil {
					return nil, "", fmt.Errorf("file service is required for file upload")
				}
				for _, file := range files {
					if file == nil {
						return nil, "", fmt.Errorf("file is nil for key %s", item.Key)
					}
					if file.ID == "" {
						return nil, "", fmt.Errorf("file id is required for key %s", item.Key)
					}
					content, err := p.fileService.DownloadFile(p.ctx, file.ID)
					if err != nil {
						return nil, "", fmt.Errorf("download file %s: %w", file.ID, err)
					}
					part, err := p.createFilePart(writer, item.Key, file)
					if err != nil {
						return nil, "", err
					}
					if _, err := part.Write(content); err != nil {
						return nil, "", fmt.Errorf("write file content: %w", err)
					}
				}
			default:
				return nil, "", fmt.Errorf("unsupported file mode: %s", mode)
			}
		}
	}

	if err := writer.Close(); err != nil {
		return nil, "", fmt.Errorf("close multipart writer: %w", err)
	}

	return &buf, writer.FormDataContentType(), nil
}

// processBinaryBody processes binary request body
func (p *HTTPRequestProcessor) processBinaryBody() (io.Reader, string, error) {
	if len(p.nodeData.Body.Data) != 1 {
		return nil, "", NewRequestBodyError("binary body type should have exactly one item")
	}

	item := p.nodeData.Body.Data[0]
	if item.Type != BodyDataTypeFile {
		return nil, "", NewRequestBodyError("binary body type requires a file item")
	}
	if item.Mode == FileInputModeURL {
		return nil, "", NewRequestBodyError("binary body type does not support url mode")
	}
	if len(item.File) == 0 {
		return nil, "", NewRequestBodyError("binary body file selector is required")
	}
	if p.fileService == nil {
		return nil, "", NewRequestBodyError("file service is required for binary body")
	}

	files, err := p.resolveFiles(item.File)
	if err != nil {
		return nil, "", NewRequestBodyError(err.Error())
	}
	if len(files) != 1 {
		return nil, "", NewRequestBodyError("binary body requires a single file")
	}

	file := files[0]
	if file == nil || file.ID == "" {
		return nil, "", NewRequestBodyError("binary body file id is required")
	}

	content, err := p.fileService.DownloadFile(p.ctx, file.ID)
	if err != nil {
		return nil, "", NewRequestBodyError(fmt.Sprintf("download file %s: %v", file.ID, err))
	}

	contentType := file.MimeType
	if contentType == "" {
		contentType = "application/octet-stream"
	}

	return bytes.NewReader(content), contentType, nil
}

func (p *HTTPRequestProcessor) resolveFiles(selector []string) ([]*entities.File, error) {
	if p.variablePool == nil {
		return nil, fmt.Errorf("variable pool is required for file resolution")
	}
	if len(selector) < 2 {
		return nil, fmt.Errorf("file selector must have at least 2 elements")
	}

	variable := p.variablePool.GetWithPath(selector)
	if variable == nil {
		return nil, fmt.Errorf("file variable not found for selector %v", selector)
	}

	switch variable.GetType() {
	case shared.SegmentTypeFile:
		file, ok := variable.ToObject().(*entities.File)
		if !ok || file == nil {
			return nil, fmt.Errorf("file variable has invalid type for selector %v", selector)
		}
		return []*entities.File{file}, nil
	case shared.SegmentTypeArrayFile:
		files, ok := variable.ToObject().([]*entities.File)
		if !ok {
			return nil, fmt.Errorf("file array variable has invalid type for selector %v", selector)
		}
		return files, nil
	default:
		return nil, fmt.Errorf("selector %v does not reference a file", selector)
	}
}

func (p *HTTPRequestProcessor) resolveFileURL(file *entities.File) (string, error) {
	if file == nil {
		return "", fmt.Errorf("file is nil")
	}
	if file.RemoteURL != "" {
		return file.RemoteURL, nil
	}
	if file.ID == "" {
		return "", fmt.Errorf("file id is required to build url")
	}
	return workflowfile.GetSignedFileURL(file.ID)
}

func (p *HTTPRequestProcessor) createFilePart(writer *multipart.Writer, fieldName string, file *entities.File) (io.Writer, error) {
	filename := file.Filename
	if filename == "" {
		filename = file.ID
		if filename == "" {
			filename = "file"
		}
	}

	header := make(textproto.MIMEHeader)
	header.Set("Content-Disposition", mime.FormatMediaType("form-data", map[string]string{
		"name":     fieldName,
		"filename": filename,
	}))

	contentType := file.MimeType
	if contentType == "" {
		contentType = "application/octet-stream"
	}
	header.Set("Content-Type", contentType)

	part, err := writer.CreatePart(header)
	if err != nil {
		return nil, fmt.Errorf("create multipart part: %w", err)
	}
	return part, nil
}

// setHeaders sets request headers
func (p *HTTPRequestProcessor) setHeaders(req *http.Request, contentType string) error {
	// Set content type
	if contentType != "" {
		req.Header.Set("Content-Type", contentType)
	}

	// Parse custom headers
	if p.nodeData.Headers != "" {
		lines := strings.Split(p.nodeData.Headers, "\n")
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
					req.Header.Set(key, value)
				}
			}
		}
	}

	return nil
}

// setAuthentication sets authentication information
func (p *HTTPRequestProcessor) setAuthentication(req *http.Request) error {
	if p.nodeData.Authorization.Type == AuthorizationTypeNoAuth {
		return nil
	}

	if p.nodeData.Authorization.Config == nil {
		return NewAuthorizationConfigError("authorization config is required")
	}

	config := p.nodeData.Authorization.Config
	header := config.Header
	if header == "" {
		header = "Authorization"
	}

	switch config.Type {
	case AuthorizationConfigTypeBearer:
		req.Header.Set(header, fmt.Sprintf("Bearer %s", config.APIKey))
	case AuthorizationConfigTypeBasic:
		// If API key contains colon, consider it already in username:password format
		if strings.Contains(config.APIKey, ":") {
			encoded := base64.StdEncoding.EncodeToString([]byte(config.APIKey))
			req.Header.Set(header, fmt.Sprintf("Basic %s", encoded))
		} else {
			// Otherwise use directly
			req.Header.Set(header, fmt.Sprintf("Basic %s", config.APIKey))
		}
	case AuthorizationConfigTypeCustom:
		req.Header.Set(header, config.APIKey)
	default:
		return NewAuthorizationConfigError(fmt.Sprintf("unsupported authorization type: %s", config.Type))
	}

	return nil
}

// doRequest executes HTTP request
func (p *HTTPRequestProcessor) doRequest(req *http.Request) (*Response, error) {
	resp, err := p.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("request failed: %w", err)
	}
	defer resp.Body.Close()

	// Read response body
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response body: %w", err)
	}

	// Create response object
	response := NewResponse(resp, body)

	// Check response size
	if err := p.validateResponseSize(response); err != nil {
		return nil, err
	}

	return response, nil
}

// validateResponseSize validates response size
func (p *HTTPRequestProcessor) validateResponseSize(resp *Response) error {
	var threshold int
	if resp.IsFile() {
		threshold = HTTPRequestNodeMaxBinarySize
	} else {
		threshold = HTTPRequestNodeMaxTextSize
	}

	if resp.Size() > threshold {
		fileType := "Text"
		if resp.IsFile() {
			fileType = "File"
		}
		return NewResponseSizeError(fmt.Sprintf(
			"%s size is too large, max size is %.2f MB, but current size is %s",
			fileType,
			float64(threshold)/(1024*1024),
			resp.ReadableSize(),
		))
	}

	return nil
}

// ToLog generates request log
func (p *HTTPRequestProcessor) ToLog() map[string]interface{} {
	// Process URL and parameters
	processedURL, _ := p.processURL()
	params, _ := p.processParams()

	if len(params) > 0 {
		processedURL = p.addParamsToURL(processedURL, params)
	}

	// Parse URL to get path
	u, err := url.Parse(processedURL)
	if err != nil {
		u = &url.URL{Path: "/"}
	}

	path := u.Path
	if path == "" {
		path = "/"
	}
	if u.RawQuery != "" {
		path += "?" + u.RawQuery
	}

	// Build log
	log := map[string]interface{}{
		"method": strings.ToUpper(string(p.nodeData.Method)),
		"url":    processedURL,
		"path":   path,
	}

	// Add header information (hide sensitive information)
	headers := make(map[string]string)
	if p.nodeData.Headers != "" {
		lines := strings.Split(p.nodeData.Headers, "\n")
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
					// Hide authentication information
					if strings.ToLower(key) == "authorization" ||
						(p.nodeData.Authorization.Config != nil &&
							strings.EqualFold(key, p.nodeData.Authorization.Config.Header)) {
						value = strings.Repeat("*", len(value))
					}
					headers[key] = value
				}
			}
		}
	}

	if len(headers) > 0 {
		log["headers"] = headers
	}

	return log
}
