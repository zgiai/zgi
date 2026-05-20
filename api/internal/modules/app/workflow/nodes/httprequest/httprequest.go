package httprequest

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/zgiai/ginext/internal/modules/app/workflow/graph_engine/entities"
	"github.com/zgiai/ginext/internal/modules/app/workflow/nodes/base"
	"github.com/zgiai/ginext/internal/modules/app/workflow/shared"
	"github.com/zgiai/ginext/pkg/logger"
	"go.uber.org/zap"
)

type HTTPRequestNode struct {
	base.NodeStruct
	NodeData
	fileService fileDownloader
}

func (n *HTTPRequestNode) logContext(ctx context.Context) context.Context {
	return logger.WithFields(ctx,
		zap.String("workflow_id", n.WorkflowID),
		zap.String("workflow_run_id", n.getWorkflowRunID()),
		zap.String("node_id", n.NodeID),
		zap.String("node_type", "http-request"),
		zap.String("tenant_id", n.TenantID),
		zap.String("app_id", n.APPID),
		zap.String("user_id", n.UserID),
	)
}

func (n *HTTPRequestNode) getWorkflowRunID() string {
	if n.GraphRuntimeState == nil || n.GraphRuntimeState.VariablePool == nil {
		return ""
	}
	if runID := n.GraphRuntimeState.VariablePool.Get([]string{"sys", "workflow_run_id"}); runID != nil {
		if value, ok := runID.ToObject().(string); ok {
			return value
		}
	}
	return ""
}

func NewHTTPRequestNode(
	id string,
	config map[string]any,
	graphInitParams entities.GraphInitParams,
	graph *entities.Graph,
	graphRuntimeState *entities.GraphRuntimeState,
	previousNodeID *string,
	optionalDeps ...interface{},
) (shared.NodeInterface, error) {
	nd, nodeID, err := getData(config)
	if err != nil {
		return nil, err
	}

	var fileService fileDownloader
	for _, dep := range optionalDeps {
		if fs, ok := dep.(fileDownloader); ok {
			fileService = fs
			break
		}
	}

	return &HTTPRequestNode{
		NodeStruct: base.NodeStruct{
			InstanceID: id,
			NodeID:     nodeID,
			NodeType:   shared.HTTPRequest,

			TenantID:          graphInitParams.TenantID,
			APPID:             graphInitParams.AppID,
			WorkflowType:      string(graphInitParams.WorkflowType),
			WorkflowID:        graphInitParams.WorkflowID,
			UserFrom:          string(graphInitParams.UserFrom),
			UserID:            graphInitParams.UserID,
			GraphConfig:       graphInitParams.GraphConfig,
			InvokeFrom:        string(graphInitParams.InvokeFrom),
			WorkflowCallDepth: graphInitParams.CallDepth,

			Graph:             graph,
			GraphRuntimeState: graphRuntimeState,
			PreviousNodeID:    previousNodeID,
		},
		NodeData:    nd,
		fileService: fileService,
	}, nil
}

func (n *HTTPRequestNode) Run(ctx context.Context, eventChan chan *shared.NodeEventCh) error {
	select {
	case eventChan <- &shared.NodeEventCh{
		Type:      shared.EventTypeRunStarted,
		NodeID:    n.NodeID,
		Timestamp: time.Now(),
	}:
	case <-ctx.Done():
		return ctx.Err()
	}

	result, err := n.executeRun(ctx)
	if err != nil {
		// Send failure event
		select {
		case eventChan <- &shared.NodeEventCh{
			Type:      shared.EventTypeRunFailed,
			NodeID:    n.NodeID,
			Error:     err,
			Timestamp: time.Now(),
		}:
		case <-ctx.Done():
			return ctx.Err()
		}
		return err
	}

	select {
	case eventChan <- &shared.NodeEventCh{
		Type:      shared.EventTypeRunCompleted,
		NodeID:    n.NodeID,
		Data:      &shared.RunCompletedEvent{RunResult: result},
		Timestamp: time.Now(),
	}:
	case <-ctx.Done():
		return ctx.Err()
	}

	return nil
}

func (n *HTTPRequestNode) executeRun(ctx context.Context) (*shared.NodeRunResult, error) {
	logCtx := n.logContext(ctx)
	logger.DebugContext(logCtx, "HTTP request workflow node execution started")
	processData := make(map[string]any)
	inputs := n.buildInputSnapshot()

	// 1. Create HTTP request processor
	logger.DebugContext(logCtx, "creating HTTP request processor")
	processor := NewHTTPRequestProcessor(
		&n.NodeData,
		nil, // Use default timeout
		n.GraphRuntimeState.VariablePool,
		n.NodeData.RetryConfig.MaxTimes, // Use retry config from node data
		n.fileService,
	)
	logger.DebugContext(logCtx, "HTTP request processor created")

	// 2. Record request information for logging
	logger.DebugContext(logCtx, "recording HTTP request workflow process data")
	processData["request"] = processor.ToLog()

	// 3. Execute HTTP request with retry
	logger.DebugContext(logCtx, "executing HTTP request workflow node")
	response, attempts, err := n.executeWithRetry(ctx, processor)
	processData["retry_attempts"] = attempts

	if err != nil {
		logger.CriticalContext(logCtx, "HTTP request workflow node failed",
			err,
			zap.Int("attempts", attempts),
		)

		errMsg := err.Error()
		errType := "HTTPRequestError"

		var httpErr *HTTPRequestNodeError
		if errors.As(err, &httpErr) {
			logger.ErrorContext(logCtx, "HTTP request workflow node returned typed error",
				httpErr,
				zap.String("error_type", fmt.Sprintf("%T", httpErr)),
			)
			errType = "HTTPRequestNodeError"
		}

		if result := n.applyErrorStrategy(errMsg, errType, processData); result != nil {
			return result, nil
		}

		return &shared.NodeRunResult{
			Status:      shared.FAILED,
			Inputs:      inputs,
			ErrMsg:      errMsg,
			ProcessData: processData,
			ErrType:     errType,
		}, nil
	}
	logger.DebugContext(logCtx, "HTTP request workflow node executed",
		zap.Int("attempts", attempts),
		zap.Int("status", response.StatusCode),
	)

	// 4. Extract files (if any) - directly use URL from NodeData
	files := n.extractFiles(n.NodeData.URL, response)

	// 5. Check HTTP response status
	logger.DebugContext(logCtx, "HTTP request workflow node status evaluated",
		zap.Int("status", response.StatusCode),
		zap.Bool("success_status", n.isSuccessStatusCode(response.StatusCode)),
	)

	if !n.isSuccessStatusCode(response.StatusCode) {
		logger.WarnContext(logCtx, "HTTP request workflow node returned non-success status",
			zap.Int("status", response.StatusCode),
		)

		outputs := map[string]any{
			"status_code": response.StatusCode,
			"body":        n.getResponseBody(response, files),
			"headers":     response.Headers,
			"files":       files,
		}

		if !n.shouldFailOnHTTPStatus() {
			return &shared.NodeRunResult{
				Status:      shared.SUCCEEDED,
				Inputs:      inputs,
				Outputs:     outputs,
				ProcessData: processData,
			}, nil
		}

		errMsg := fmt.Sprintf("Request failed with status code %d", response.StatusCode)
		errType := "HTTPResponseCodeError"
		if result := n.applyErrorStrategy(errMsg, errType, processData); result != nil {
			return result, nil
		}

		// Return failure status for non-success HTTP status codes
		return &shared.NodeRunResult{
			Status:      shared.FAILED,
			Inputs:      inputs,
			Outputs:     outputs,
			ProcessData: processData,
			ErrMsg:      errMsg,
			ErrType:     errType,
		}, nil
	}
	logger.DebugContext(logCtx, "HTTP request workflow node succeeded",
		zap.Int("status", response.StatusCode),
	)

	// 6. Return success result
	return &shared.NodeRunResult{
		Status: shared.SUCCEEDED,
		Inputs: inputs,
		Outputs: map[string]any{
			"status_code": response.StatusCode,
			"body":        n.getResponseBody(response, files),
			"headers":     response.Headers,
			"files":       files,
		},
		ProcessData: processData,
	}, nil
}

func (n *HTTPRequestNode) buildInputSnapshot() map[string]any {
	return map[string]any{
		"url":    n.NodeData.URL,
		"method": strings.ToUpper(string(n.NodeData.Method)),
		"header": parseLinesToMap(n.NodeData.Headers),
		"param":  parseLinesToMap(n.NodeData.Params),
		"body":   snapshotBody(n.NodeData.Body),
		"auth":   snapshotAuthorization(n.NodeData.Authorization),
	}
}

func parseLinesToMap(raw string) map[string]any {
	result := map[string]any{}
	for _, line := range strings.Split(raw, "\n") {
		parts := strings.SplitN(strings.TrimSpace(line), ":", 2)
		if len(parts) != 2 {
			continue
		}
		key := strings.TrimSpace(parts[0])
		if key == "" {
			continue
		}
		result[key] = strings.TrimSpace(parts[1])
	}
	return result
}

func snapshotBody(body *HttpRequestNodeBody) any {
	if body == nil || body.Type == BodyTypeNone {
		return nil
	}
	return body
}

func snapshotAuthorization(auth HttpRequestNodeAuthorization) any {
	if auth.Type == "" || auth.Type == AuthorizationTypeNoAuth {
		return nil
	}
	result := map[string]any{"type": auth.Type}
	if auth.Config != nil {
		result["config"] = map[string]any{
			"type":   auth.Config.Type,
			"header": auth.Config.Header,
		}
	}
	return result
}

func getData(config map[string]any) (NodeData, string, error) {
	nodeID, ok := config["id"]
	if !ok {
		return NodeData{}, "", fmt.Errorf("node ID is required")
	}
	nodeIDStr, ok := nodeID.(string)
	if !ok {
		return NodeData{}, "", fmt.Errorf("node ID Type is unsupported")
	}

	data, ok := config["data"]
	if !ok {
		return NodeData{}, "", fmt.Errorf("node data is required")
	}

	// Parse specific data for HTTP request node
	dataMap, ok := data.(map[string]any)
	if !ok {
		return NodeData{}, "", fmt.Errorf("node data must be a map")
	}

	nd := NodeData{
		Authorization: HttpRequestNodeAuthorization{
			Type: AuthorizationTypeNoAuth,
		},
	}

	// Parse URL
	if url, exists := dataMap["url"]; exists {
		if urlStr, ok := url.(string); ok {
			nd.URL = urlStr
		}
	}

	// Parse method
	if method, exists := dataMap["method"]; exists {
		if methodStr, ok := method.(string); ok {
			nd.Method = HTTPMethod(methodStr)
		}
	} else {
		// Default to GET if not specified
		nd.Method = HTTPMethodGET
	}

	// Parse headers
	if headers, exists := dataMap["headers"]; exists {
		if headersArray, ok := headers.([]interface{}); ok {
			// Handle array format headers (from workflow config)
			headerLines := make([]string, 0, len(headersArray))
			for _, headerItem := range headersArray {
				if headerMap, ok := headerItem.(map[string]interface{}); ok {
					if key, keyOk := headerMap["key"].(string); keyOk {
						if value, valueOk := headerMap["value"].(string); valueOk {
							headerLines = append(headerLines, fmt.Sprintf("%s: %s", key, value))
						}
					}
				}
			}
			nd.Headers = strings.Join(headerLines, "\n")
		} else if headersMap, ok := headers.(map[string]interface{}); ok {
			// Convert headers map to string format
			headerLines := make([]string, 0, len(headersMap))
			for key, value := range headersMap {
				if valueStr, ok := value.(string); ok {
					headerLines = append(headerLines, fmt.Sprintf("%s: %s", key, valueStr))
				}
			}
			nd.Headers = strings.Join(headerLines, "\n")
		} else if headersStr, ok := headers.(string); ok {
			// Handle string format headers directly
			nd.Headers = headersStr
		}
	}

	// Parse params
	if params, exists := dataMap["params"]; exists {
		if paramsStr, ok := params.(string); ok {
			nd.Params = paramsStr
		}
	}

	// Parse authorization
	if authorization, exists := dataMap["authorization"]; exists {
		if authMap, ok := authorization.(map[string]interface{}); ok {
			if authType, exists := authMap["type"]; exists {
				if authTypeStr, ok := authType.(string); ok {
					nd.Authorization.Type = AuthorizationType(authTypeStr)
				}
			}

			// Parse authorization config if type is api-key
			if nd.Authorization.Type == AuthorizationTypeAPIKey {
				if config, exists := authMap["config"]; exists {
					if configMap, ok := config.(map[string]interface{}); ok {
						nd.Authorization.Config = &HttpRequestNodeAuthorizationConfig{}

						if configType, exists := configMap["type"]; exists {
							if configTypeStr, ok := configType.(string); ok {
								nd.Authorization.Config.Type = AuthorizationConfigType(configTypeStr)
							}
						}

						if apiKey, exists := configMap["api_key"]; exists {
							if apiKeyStr, ok := apiKey.(string); ok {
								nd.Authorization.Config.APIKey = apiKeyStr
							}
						}

						if header, exists := configMap["header"]; exists {
							if headerStr, ok := header.(string); ok {
								nd.Authorization.Config.Header = headerStr
							}
						}
					}
				}
			}
		}
	}

	// Parse body
	if body, exists := dataMap["body"]; exists {
		if bodyMap, ok := body.(map[string]interface{}); ok {
			nd.Body = &HttpRequestNodeBody{}
			if bodyType, exists := bodyMap["type"]; exists {
				if bodyTypeStr, ok := bodyType.(string); ok {
					nd.Body.Type = BodyType(bodyTypeStr)
				}
			}
			if bodyData, exists := bodyMap["data"]; exists {
				if bodyDataSlice, ok := bodyData.([]interface{}); ok {
					nd.Body.Data = make([]BodyData, len(bodyDataSlice))
					for i, item := range bodyDataSlice {
						if itemMap, ok := item.(map[string]interface{}); ok {
							if key, exists := itemMap["key"]; exists {
								if keyStr, ok := key.(string); ok {
									nd.Body.Data[i].Key = keyStr
								}
							}
							if dataType, exists := itemMap["type"]; exists {
								if dataTypeStr, ok := dataType.(string); ok {
									nd.Body.Data[i].Type = BodyDataType(dataTypeStr)
								}
							}
							if value, exists := itemMap["value"]; exists {
								switch v := value.(type) {
								case string:
									nd.Body.Data[i].Value = v
								default:
									if selector, ok := parseStringSlice(v); ok {
										nd.Body.Data[i].File = selector
									}
								}
							}
							if fileSelector, exists := itemMap["file"]; exists {
								if selector, ok := parseStringSlice(fileSelector); ok {
									nd.Body.Data[i].File = selector
								}
							}
							if valueSelector, exists := itemMap["value_selector"]; exists {
								if selector, ok := parseStringSlice(valueSelector); ok {
									nd.Body.Data[i].File = selector
								}
							}
							if mode, exists := itemMap["mode"]; exists {
								if modeStr, ok := mode.(string); ok {
									nd.Body.Data[i].Mode = FileInputMode(modeStr)
								}
							}
							if nd.Body.Data[i].Type == BodyDataTypeFile && nd.Body.Data[i].Mode == "" {
								nd.Body.Data[i].Mode = FileInputModeUpload
							}
						}
					}
				}
			}
		}
	}

	if title, exists := dataMap["title"]; exists {
		if titleStr, ok := title.(string); ok {
			nd.Title = titleStr
		}
	}
	if desc, exists := dataMap["desc"]; exists {
		if descStr, ok := desc.(string); ok {
			nd.Desc = descStr
		}
	}
	if version, exists := dataMap["version"]; exists {
		if versionStr, ok := version.(string); ok {
			nd.Version = versionStr
		}
	}
	if errorStrategyRaw, exists := dataMap["error_strategy"]; exists {
		nd.ErrorStrategy = parseErrorStrategy(errorStrategyRaw)
	}
	if defaultValueRaw, exists := dataMap["default_value"]; exists {
		entries, defaults := parseDefaultValueEntries(defaultValueRaw)
		nd.defaultValueEntries = entries
		nd.DefaultValue = defaults
	}
	if retryRaw, exists := dataMap["retry_config"]; exists {
		nd.RetryConfig = parseRetryConfig(retryRaw)
	} else if retryRaw, exists := config["retry_config"]; exists {
		nd.RetryConfig = parseRetryConfig(retryRaw)
	}

	// Set default timeout
	nd.Timeout = NewDefaultTimeout()

	// Set default SSL verify
	sslVerify := HTTPRequestNodeSSLVerify
	nd.SSLVerify = &sslVerify

	return nd, nodeIDStr, nil
}

func parseStringSlice(value interface{}) ([]string, bool) {
	switch v := value.(type) {
	case []string:
		return v, true
	case []interface{}:
		result := make([]string, len(v))
		for i, item := range v {
			str, ok := item.(string)
			if !ok {
				return nil, false
			}
			result[i] = str
		}
		return result, true
	default:
		return nil, false
	}
}

// extractFiles extracts files from response
func (n *HTTPRequestNode) extractFiles(url string, response *Response) []any {
	// TODO: Implement file extraction logic
	// Here need to determine if it's a file based on response Content-Type and other headers
	return []any{}
}

// isSuccessStatusCode checks if HTTP status code indicates success
func (n *HTTPRequestNode) isSuccessStatusCode(statusCode int) bool {
	return statusCode >= 200 && statusCode < 300
}

// getResponseBody gets response body, returns empty string if there are files
func (n *HTTPRequestNode) getResponseBody(response *Response, files []any) string {
	if len(files) > 0 {
		return "" // If there are files, don't return text content
	}
	return string(response.Body)
}

func (n *HTTPRequestNode) shouldFailOnHTTPStatus() bool {
	return n.NodeData.ErrorStrategy != "" || n.NodeData.RetryConfig.Enable
}

func (n *HTTPRequestNode) applyErrorStrategy(errMsg, errType string, processData map[string]any) *shared.NodeRunResult {
	if n.NodeData.ErrorStrategy == "" {
		return nil
	}

	result := &shared.NodeRunResult{
		Status:      shared.EXCEPTION,
		Inputs:      n.buildInputSnapshot(),
		Outputs:     buildErrorOutputs(n.NodeData.ErrorStrategy, n.NodeData.defaultValueEntries, errMsg, errType),
		ProcessData: processData,
		ErrMsg:      errMsg,
		ErrType:     errType,
		Metadata: map[shared.WorkflowNodeExecutionMetadataKey]any{
			shared.ErrStrategy: n.NodeData.ErrorStrategy,
		},
	}

	if n.NodeData.ErrorStrategy == shared.FailBranch {
		result.EdgeSourceHandle = string(shared.FailedBranch)
	}

	return result
}

// executeWithRetry executes HTTP request with retry mechanism
// Returns the response, number of attempts made, and any error
func (n *HTTPRequestNode) executeWithRetry(ctx context.Context, processor *HTTPRequestProcessor) (*Response, int, error) {
	// Check if retry is enabled
	if !n.NodeData.RetryConfig.Enable {
		// No retry enabled, execute once
		resp, err := processor.Execute(ctx)
		return resp, 1, err
	}

	// Get max retries from retry config
	maxRetries := n.NodeData.RetryConfig.MaxTimes
	if maxRetries < 0 {
		maxRetries = 0
	}

	// Get retry interval from config (default 1000ms if not set)
	retryInterval := n.NodeData.RetryConfig.Interval
	if retryInterval <= 0 {
		retryInterval = 1000 // Default 1 second
	}

	// Total attempts = initial attempt + retries
	totalAttempts := maxRetries + 1
	var lastErr error

	for attempt := 1; attempt <= totalAttempts; attempt++ {
		// Attempt to execute HTTP request
		resp, err := processor.Execute(ctx)
		if err == nil {
			// Check if response status code indicates success
			if n.isSuccessStatusCode(resp.StatusCode) {
				// Success - return result with attempt count
				return resp, attempt, nil
			}
			// Non-success status code, but no execution error
			// This case will be handled by the caller based on error strategy
			return resp, attempt, nil
		}

		// Check if context was cancelled
		if ctx.Err() != nil {
			return nil, attempt, ctx.Err()
		}

		// Store error for potential return
		lastErr = err

		// If this is not the last attempt, wait before retrying
		if attempt < totalAttempts {
			// Use configured interval
			backoffDuration := time.Duration(retryInterval) * time.Millisecond

			logger.WarnContext(n.logContext(ctx), "HTTP request workflow node attempt failed, retrying",
				err,
				zap.Int("attempt", attempt),
				zap.Int("total_attempts", totalAttempts),
				zap.Int64("retry_after_ms", backoffDuration.Milliseconds()),
			)

			// Wait with context cancellation support
			select {
			case <-time.After(backoffDuration):
				// Continue to next attempt
			case <-ctx.Done():
				// Context cancelled during wait
				return nil, attempt, ctx.Err()
			}
		}
	}

	// All attempts failed - return last error
	logger.CriticalContext(n.logContext(ctx), "all HTTP request workflow node attempts failed",
		lastErr,
		zap.Int("total_attempts", totalAttempts),
	)
	return nil, totalAttempts, lastErr
}
