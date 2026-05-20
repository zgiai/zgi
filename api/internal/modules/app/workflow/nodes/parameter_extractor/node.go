package parameterextractor

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/shopspring/decimal"
	"github.com/zgiai/ginext/internal/modules/app/workflow/file"
	"github.com/zgiai/ginext/internal/modules/app/workflow/graph_engine/entities"
	"github.com/zgiai/ginext/internal/modules/app/workflow/nodes/base"
	"github.com/zgiai/ginext/internal/modules/app/workflow/shared"
	"github.com/zgiai/ginext/internal/modules/app/workflow/template"
	llmclient "github.com/zgiai/ginext/internal/modules/llm/client"
	"github.com/zgiai/ginext/internal/modules/llm/gateway"
	"github.com/zgiai/ginext/pkg/database"
	"github.com/zgiai/ginext/pkg/logger"
	"go.uber.org/zap"
	"gorm.io/gorm"
)

// Node represents a parameter extractor node in the workflow
type Node struct {
	base.NodeStruct
	nodeData   NodeData
	db         *gorm.DB
	llmInvoker LLMInvoker
}

// New creates a new parameter extractor node
func New(
	id string,
	config map[string]any,
	graphInitParams entities.GraphInitParams,
	graph *entities.Graph,
	graphRuntimeState *entities.GraphRuntimeState,
	previousNodeID *string,
	optionalDeps ...interface{},
) (shared.NodeInterface, error) {
	nd, nodeID, err := parseParameterExtractorNodeDataFromConfig(config)
	if err != nil {
		return nil, err
	}

	// Get database connection
	db := database.GetDB()

	// Create node instance
	n := &Node{
		NodeStruct: base.NodeStruct{
			InstanceID: id,
			NodeID:     nodeID,
			NodeType:   shared.ParameterExtractor,

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
		nodeData: nd,
		db:       db,
	}

	// Check optionalDeps for LLMClient
	for _, dep := range optionalDeps {
		if client, ok := dep.(llmclient.LLMClient); ok {
			invoker, invErr := NewGatewayLLMInvoker(client, graphInitParams.OrganizationID, graphInitParams.WorkspaceID, graphInitParams.BillingSubjectType)
			if invErr != nil {
				return nil, invErr
			}
			n.llmInvoker = invoker
			break
		}
	}

	return n, nil
}

// Run executes the parameter extractor node
func (n *Node) Run(ctx context.Context, eventChan chan *shared.NodeEventCh) error {
	// Send start event
	select {
	case eventChan <- &shared.NodeEventCh{
		Type:      shared.EventTypeRunStarted,
		NodeID:    n.NodeID,
		Timestamp: time.Now(),
	}:
	case <-ctx.Done():
		return ctx.Err()
	}

	// Execute the parameter extraction logic
	result, err := n.executeRun(ctx, eventChan)
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

	// Send completion event
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

func (n *Node) logContext(ctx context.Context) context.Context {
	return logger.WithFields(ctx,
		zap.String("node_id", n.NodeID),
		zap.String("node_type", string(n.NodeType)),
		zap.String("workflow_id", n.WorkflowID),
		zap.String("workflow_run_id", n.workflowRunID()),
		zap.String("tenant_id", n.TenantID),
	)
}

func (n *Node) workflowRunID() string {
	if n.GraphRuntimeState == nil || n.GraphRuntimeState.VariablePool == nil {
		return ""
	}
	return n.GraphRuntimeState.VariablePool.SystemVariables.WorkflowRunID
}

// executeRun performs the actual parameter extraction logic
func (n *Node) executeRun(ctx context.Context, eventChan chan *shared.NodeEventCh) (*shared.NodeRunResult, error) {
	logCtx := n.logContext(ctx)
	variablePool := n.GraphRuntimeState.VariablePool
	nodeInputs := make(map[string]any)

	// Task 9.3: Fetch inputs
	// Fetch query text from variable pool
	queryVar := variablePool.GetWithPath(n.nodeData.Query)
	if queryVar == nil {
		return nil, fmt.Errorf("query variable not found at selector: %v", n.nodeData.Query)
	}
	query := queryVar.Text()
	// Use the variable name from the selector as the display key
	queryKey := n.nodeData.Query[1]
	nodeInputs[queryKey] = query
	if queryKey != "query" {
		nodeInputs["query"] = query
	}

	// Fetch files if vision is enabled
	var files []*file.File
	if n.nodeData.Vision.Enabled && len(n.nodeData.Vision.Configs.VariableSelector) > 0 {
		var err error
		files, err = n.fetchFiles(variablePool, n.nodeData.Vision.Configs.VariableSelector)
		if err != nil {
			return nil, fmt.Errorf("failed to fetch files: %w", err)
		}
		if len(files) > 0 {
			filesKey := n.nodeData.Vision.Configs.VariableSelector[1]
			nodeInputs[filesKey] = files
		}
	}

	// Resolve custom instructions for prompt generation (not stored in nodeInputs)
	var instruction string
	if n.nodeData.Instruction != nil && *n.nodeData.Instruction != "" {
		instruction = n.resolveInstructions(logCtx, *n.nodeData.Instruction, variablePool)
	}

	// Task 5: Check for runtime model configuration override
	configSource := "static"
	runtimeModelConfig := n.getRuntimeModelConfig()
	if runtimeModelConfig != nil {
		// Override static config with runtime config
		n.nodeData.Model = *runtimeModelConfig
		n.nodeData.IsStaticConfig = false
		configSource = "runtime"
		logger.InfoContext(logCtx, "parameter extractor using runtime model config",
			zap.String("provider", runtimeModelConfig.Provider),
			zap.String("model", runtimeModelConfig.Name),
		)
	} else {
		n.nodeData.IsStaticConfig = true
	}

	logger.InfoContext(logCtx, "parameter extractor configuration resolved",
		zap.String("config_source", configSource),
		zap.String("provider", n.nodeData.Model.Provider),
		zap.String("model", n.nodeData.Model.Name),
	)

	// Task 6: Simplified prompt generation (prompt engineering mode only)
	// Generate prompts using PromptGenerator
	promptGenerator := NewPromptGenerator(n.nodeData, variablePool)

	// Generate prompt engineering prompt (always use this mode now)
	promptMessages, err := promptGenerator.GeneratePromptEngineeringChatPrompt(query, files, instruction)
	if err != nil {
		return nil, fmt.Errorf("failed to generate prompt: %w", err)
	}

	// Build invoke request for Gateway. ModelSlug must match route model names exactly.
	modelSlug := n.nodeData.Model.Name
	invokeReq := &InvokeRequest{
		ModelSlug:  modelSlug,
		Messages:   promptMessages,
		Parameters: n.nodeData.Model.CompletionParams,
		UserID:     n.UserID,
		Stream:     false,
	}

	// Invoke LLM using Gateway Invoker with retry mechanism
	llmResult, attempts, err := n.invokeLLMWithRetry(ctx, n.UserID, n.APPID, AppType, invokeReq)
	if err != nil {
		logger.CriticalContext(logCtx, "parameter extractor llm invocation failed",
			zap.Int("attempts", attempts),
			zap.String("provider", n.nodeData.Model.Provider),
			zap.String("model", n.nodeData.Model.Name),
			zap.Error(err),
		)
		return n.createFailureResult(nodeInputs, err, nil)
	}

	// Log retry information if retries occurred
	if attempts > 1 {
		logger.InfoContext(logCtx, "parameter extractor llm invocation succeeded after retry",
			zap.Int("attempts", attempts),
			zap.String("provider", n.nodeData.Model.Provider),
			zap.String("model", n.nodeData.Model.Name),
		)
	}

	// Convert usage info to shared.LLMUsage format
	llmUsage := convertUsageInfoToLLMUsage(llmResult.Usage)

	// Task 6: Result extraction and validation
	// Extract JSON from LLM response (text only, no tool calls)
	extractor := NewJSONExtractor()
	extractedParams, err := extractor.ExtractFromText(llmResult.Text)
	if err != nil {
		// If extraction fails, use default values
		return n.createFailureResult(nodeInputs, err, llmUsage)
	}

	// Validate extracted parameters
	validator := NewValidator(n.nodeData.Parameters)
	if err := validator.Validate(extractedParams); err != nil {
		return n.createFailureResult(nodeInputs, err, llmUsage)
	}

	// Transform results to standard format
	transformer := NewResultTransformer(n.nodeData.Parameters)
	transformedParams, err := transformer.Transform(extractedParams)
	if err != nil {
		return n.createFailureResult(nodeInputs, err, llmUsage)
	}

	// Task 9.8: Output formatting
	// Create success output
	outputs := make(map[string]any)
	outputs["__is_success"] = 1
	outputs["__reason"] = nil

	// Add usage information
	if llmUsage != nil {
		outputs["__usage"] = map[string]any{
			"total_tokens":      llmUsage.TotalTokens,
			"prompt_tokens":     llmUsage.PromptTokens,
			"completion_tokens": llmUsage.CompletionTokens,
			"total_price":       llmUsage.TotalPrice.String(),
			"currency":          llmUsage.Currency,
		}
	}

	// Add all extracted parameters
	for paramName, paramValue := range transformedParams {
		outputs[paramName] = paramValue
	}

	// Create metadata
	metadata := make(map[shared.WorkflowNodeExecutionMetadataKey]any)
	if llmUsage != nil {
		metadata[shared.TotalTokens] = llmUsage.TotalTokens
		metadata[shared.TotalPrice] = llmUsage.TotalPrice.String()
		metadata[shared.Currency] = llmUsage.Currency
	}

	// Add retry count to metadata if retries occurred
	if attempts > 1 {
		metadata["retry_count"] = attempts - 1
		metadata["total_attempts"] = attempts
	}

	return &shared.NodeRunResult{
		Status:   shared.SUCCEEDED,
		Inputs:   nodeInputs,
		Outputs:  outputs,
		Metadata: metadata,
		LLMUsage: llmUsage,
	}, nil
}

// Helper methods for executeRun

// fetchFiles fetches files from variable pool for vision processing
func (n *Node) fetchFiles(variablePool *entities.VariablePool, selector []string) ([]*file.File, error) {
	if len(selector) == 0 {
		return []*file.File{}, nil
	}

	variable := variablePool.GetWithPath(selector)
	if variable == nil {
		return []*file.File{}, nil
	}

	// Check variable type using GetType() method
	varType := variable.GetType()
	switch varType {
	case shared.SegmentTypeFile:
		// FileSegment: return single file as array
		if fileValue := variable.GetValue(); fileValue != nil {
			if entityFile, ok := fileValue.(*entities.File); ok {
				convertedFile := n.convertEntityFileToWorkflowFile(entityFile)
				if convertedFile != nil {
					return []*file.File{convertedFile}, nil
				}
			}
		}
		return []*file.File{}, nil

	case shared.SegmentTypeArrayFile:
		// ArrayFileSegment: return file array
		if arrayValue := variable.GetValue(); arrayValue != nil {
			if entityFiles, ok := arrayValue.([]*entities.File); ok {
				var result []*file.File
				for _, entityFile := range entityFiles {
					convertedFile := n.convertEntityFileToWorkflowFile(entityFile)
					if convertedFile != nil {
						result = append(result, convertedFile)
					}
				}
				return result, nil
			}
		}
		return []*file.File{}, nil

	case shared.SegmentTypeNone:
		// NoneSegment: return empty array
		return []*file.File{}, nil

	case shared.SegmentTypeArrayAny:
		// ArrayAnySegment: return empty array
		return []*file.File{}, nil

	default:
		// Invalid variable type: return error
		return nil, fmt.Errorf("invalid variable type: %s", varType)
	}
}

// convertEntityFileToWorkflowFile converts entities.File to file.File
func (n *Node) convertEntityFileToWorkflowFile(entityFile *entities.File) *file.File {
	if entityFile == nil {
		return nil
	}

	// Convert transfer method
	var transferMethod file.FileTransferMethod
	switch entityFile.TransferMethod {
	case "local_file":
		transferMethod = file.FileTransferMethodLocalFile
	case "remote_url":
		transferMethod = file.FileTransferMethodRemoteURL
	case "tool_file":
		transferMethod = file.FileTransferMethodToolFile
	default:
		transferMethod = file.FileTransferMethodRemoteURL
	}

	// Convert file type
	var kind file.FileType
	switch entityFile.Type {
	case "image":
		kind = file.FileTypeImage
	case "document":
		kind = file.FileTypeDocument
	case "audio":
		kind = file.FileTypeAudio
	case "video":
		kind = file.FileTypeVideo
	default:
		kind = file.FileTypeCustom
	}

	opts := []file.FileOption{
		file.WithID(entityFile.ID),
		file.WithRelatedID(entityFile.ID),
		file.WithFilename(entityFile.Filename),
		file.WithExtension(entityFile.Extension),
		file.WithMimeType(entityFile.MimeType),
		file.WithSize(int(entityFile.Size)),
	}
	if transferMethod == file.FileTransferMethodRemoteURL && entityFile.RemoteURL != "" {
		opts = append(opts, file.WithRemoteURL(entityFile.RemoteURL))
	}
	if entityFile.RemoteURL != "" {
		opts = append(opts, file.WithURL(entityFile.RemoteURL))
	}

	return file.NewFile(entityFile.WorkspaceID, kind, transferMethod, opts...)
}

// resolveInstructions resolves variable templates in custom instructions
func (n *Node) resolveInstructions(ctx context.Context, instruction string, variablePool *entities.VariablePool) string {
	// Convert variable pool to map for template rendering
	variableMap := n.variablePoolToMap(variablePool)

	// Render template
	renderer := template.NewPongo2RendererWithVariablePool(variableMap)
	rendered, err := renderer.Render(instruction, variableMap)
	if err != nil {
		// If rendering fails, return original instruction
		logger.WarnContext(ctx, "parameter extractor failed to render instruction template",
			zap.Error(err),
		)
		return instruction
	}

	return rendered
}

// variablePoolToMap converts variable pool to a map for template rendering
func (n *Node) variablePoolToMap(variablePool *entities.VariablePool) map[string]interface{} {
	result := make(map[string]interface{})

	if variablePool == nil {
		return result
	}

	// Convert all variables from the variable pool
	// This creates a flat map with keys like "nodeID.variableName"
	for nodeID, nodeVars := range variablePool.VariableDictionary {
		for varName, varValue := range nodeVars {
			// Create flat key: nodeID.varName
			flatKey := fmt.Sprintf("%s.%s", nodeID, varName)

			// Convert variable to object
			if varValue != nil {
				result[flatKey] = varValue.ToObject()
			}
		}
	}

	return result
}

// getRuntimeModelConfig retrieves runtime model configuration from VariablePool
func (n *Node) getRuntimeModelConfig() *ModelConfig {
	if n.GraphRuntimeState == nil || n.GraphRuntimeState.VariablePool == nil {
		return nil
	}

	userInputs := n.GraphRuntimeState.VariablePool.UserInputs
	if userInputs == nil {
		return nil
	}

	// Check for model_config in inputs
	if modelConfigRaw, exists := userInputs["model_config"]; exists {
		if modelMap, ok := modelConfigRaw.(map[string]interface{}); ok {
			model := &ModelConfig{
				CompletionParams: make(map[string]any),
			}

			if provider, ok := modelMap["provider"].(string); ok {
				model.Provider = provider
			}
			if name, ok := modelMap["model"].(string); ok {
				model.Name = name
			}
			if mode, ok := modelMap["mode"].(string); ok {
				model.Mode = mode
			}

			// Parse completion_params
			if paramsRaw, ok := modelMap["completion_params"].(map[string]interface{}); ok {
				for k, v := range paramsRaw {
					model.CompletionParams[k] = v
				}
			}

			// Only return if we have at least provider and model name
			if model.Provider != "" && model.Name != "" {
				return model
			}
		}
	}

	return nil
}

// convertUsageInfoToLLMUsage converts UsageInfo to shared.LLMUsage
func convertUsageInfoToLLMUsage(usage *UsageInfo) *shared.LLMUsage {
	if usage == nil {
		return nil
	}

	// Parse total price from string to decimal
	totalPrice, err := decimal.NewFromString(usage.TotalPrice)
	if err != nil {
		totalPrice = decimal.Zero
	}

	return &shared.LLMUsage{
		PromptTokens:     usage.PromptTokens,
		CompletionTokens: usage.CompletionTokens,
		TotalTokens:      usage.TotalTokens,
		TotalPrice:       totalPrice,
		Currency:         usage.Currency,
	}
}

// invokeLLMWithRetry invokes the LLM with retry mechanism
// Returns the result, number of attempts made, and any error
func (n *Node) invokeLLMWithRetry(ctx context.Context, accountID, appID, appType string, req *InvokeRequest) (*InvokeResult, int, error) {
	// Get max retries from retry config
	maxRetries := n.nodeData.RetryConfig.MaxTimes
	if maxRetries < 0 {
		maxRetries = 0
	}

	// Total attempts = initial attempt + retries
	totalAttempts := maxRetries + 1
	var lastErr error

	for attempt := 1; attempt <= totalAttempts; attempt++ {
		// Attempt to invoke LLM
		resp, err := n.llmInvoker.Invoke(ctx, accountID, appID, appType, req)
		if err == nil {
			// Success - return result with attempt count
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
			// Calculate backoff duration using exponential backoff
			// Base interval: 150ms * attempt number
			backoffDuration := time.Duration(attempt) * 150 * time.Millisecond

			logger.WarnContext(n.logContext(ctx), "parameter extractor llm invocation retrying",
				zap.Int("attempt", attempt),
				zap.Int("max_attempts", totalAttempts),
				zap.Int64("retry_delay_ms", backoffDuration.Milliseconds()),
				zap.String("provider", n.nodeData.Model.Provider),
				zap.String("model", n.nodeData.Model.Name),
				zap.Error(err),
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
	return nil, totalAttempts, lastErr
}

// createFailureResult creates a failure result with default values
func (n *Node) createFailureResult(nodeInputs map[string]any, err error, usage *shared.LLMUsage) (*shared.NodeRunResult, error) {
	// Create outputs with failure status
	outputs := make(map[string]any)
	outputs["__is_success"] = 0
	outputs["__reason"] = err.Error()

	// Add usage information if available
	if usage != nil {
		outputs["__usage"] = map[string]any{
			"total_tokens":      usage.TotalTokens,
			"prompt_tokens":     usage.PromptTokens,
			"completion_tokens": usage.CompletionTokens,
			"total_price":       usage.TotalPrice.String(),
			"currency":          usage.Currency,
		}
	}

	// Add default values for all parameters
	transformer := NewResultTransformer(n.nodeData.Parameters)
	for _, param := range n.nodeData.Parameters {
		outputs[param.Name] = transformer.generateDefaultValue(param.Type)
	}

	// Create metadata
	metadata := make(map[shared.WorkflowNodeExecutionMetadataKey]any)
	if usage != nil {
		metadata[shared.TotalTokens] = usage.TotalTokens
		metadata[shared.TotalPrice] = usage.TotalPrice.String()
		metadata[shared.Currency] = usage.Currency
	}

	if isBillingUserError(err) {
		return &shared.NodeRunResult{
			Status:   shared.FAILED,
			Inputs:   nodeInputs,
			Outputs:  outputs,
			Metadata: metadata,
			LLMUsage: usage,
			Err:      err,
			ErrMsg:   err.Error(),
			ErrType:  "LLMInvokeError",
		}, err
	}

	return &shared.NodeRunResult{
		Status:   shared.SUCCEEDED, // Still mark as succeeded to allow workflow to continue
		Inputs:   nodeInputs,
		Outputs:  outputs,
		Metadata: metadata,
		LLMUsage: usage,
	}, nil
}

func isBillingUserError(err error) bool {
	var billingErr *gateway.BillingUserError
	return errors.As(err, &billingErr) && billingErr != nil
}
