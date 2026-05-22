package llm

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"math"
	"net/url"
	"reflect"
	"regexp"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/shopspring/decimal"
	"github.com/zgiai/zgi/api/internal/modules/app/chat"
	"github.com/zgiai/zgi/api/internal/modules/app/conversation"
	"github.com/zgiai/zgi/api/internal/modules/app/workflow/file"
	"github.com/zgiai/zgi/api/internal/modules/app/workflow/graph_engine/entities"
	"github.com/zgiai/zgi/api/internal/modules/app/workflow/nodes/base"
	"github.com/zgiai/zgi/api/internal/modules/app/workflow/shared"
	"github.com/zgiai/zgi/api/internal/modules/app/workflow/template"
	file_repo "github.com/zgiai/zgi/api/internal/modules/file_process/repository"
	file_service "github.com/zgiai/zgi/api/internal/modules/file_process/service"
	llmClient "github.com/zgiai/zgi/api/internal/modules/llm/client"
	promptmodel "github.com/zgiai/zgi/api/internal/modules/prompts/model"
	promptservice "github.com/zgiai/zgi/api/internal/modules/prompts/service"
	"github.com/zgiai/zgi/api/pkg/database"
	"github.com/zgiai/zgi/api/pkg/logger"
	"github.com/zgiai/zgi/api/pkg/storage"
	"go.uber.org/zap"
	"gorm.io/gorm"
)

// Node represents an LLM node in the workflow.
type Node struct {
	base.NodeStruct
	nodeData NodeData

	fileOutputs        []*file.File
	llmFileSaver       FileSaver
	fileLoader         fileDownloader
	db                 *gorm.DB // Database connection for conversation history
	invoker            LLMInvoker
	promptResolver     promptservice.PromptService
	resolvedPromptMeta map[string]any
}

const (
	defaultConversationHistoryWindowSize = 3
	maxConversationHistoryWindowSize     = 50
)

type fileDownloader interface {
	DownloadFile(ctx context.Context, fileID string) ([]byte, error)
}

func (n *Node) logContext(ctx context.Context) context.Context {
	return logger.WithFields(ctx,
		zap.String("workflow_id", n.WorkflowID),
		zap.String("workflow_run_id", n.getWorkflowRunID()),
		zap.String("node_id", n.NodeID),
		zap.Any("node_type", n.NodeType),
		zap.String("tenant_id", n.TenantID),
		zap.String("app_id", n.APPID),
		zap.String("user_id", n.UserID),
	)
}

func New(
	id string,
	config map[string]any,
	graphInitParams entities.GraphInitParams,
	graph *entities.Graph,
	graphRuntimeState *entities.GraphRuntimeState,
	previousNodeID *string,
	optionalDeps ...interface{},
) (shared.NodeInterface, error) {

	nd, nodeID, err := parseLLMNodeDataFromConfig(config)
	if err != nil {
		return nil, err
	}

	bns := base.NodeStruct{
		InstanceID: id,
		NodeID:     nodeID,
		NodeType:   shared.LLM,

		TenantID:          graphInitParams.TenantID,
		WorkspaceID:       graphInitParams.WorkspaceID,
		OrganizationID:    graphInitParams.OrganizationID,
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
	}

	n := &Node{
		NodeStruct:   bns,
		nodeData:     nd,
		fileOutputs:  []*file.File{},
		llmFileSaver: NewFileSaverImplGlobal(graphInitParams.UserID, graphInitParams.TenantID),
		db:           database.GetDB(),
	}

	// Check optionalDeps for LLMClient
	for _, dep := range optionalDeps {
		if client, ok := dep.(llmClient.LLMClient); ok {
			invoker, invErr := NewGatewayLLMInvoker(client, graphInitParams.OrganizationID, graphInitParams.WorkspaceID, graphInitParams.BillingSubjectType)
			if invErr != nil {
				return nil, invErr
			}
			n.invoker = invoker
			continue
		}
		if downloader, ok := dep.(fileDownloader); ok {
			n.fileLoader = downloader
			continue
		}
		if resolver, ok := dep.(promptservice.PromptService); ok {
			n.promptResolver = resolver
		}
	}

	return n, nil
}

// Run executes the LLM node
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

	// Execute the LLM logic
	result, err := n.executeRun(ctx, eventChan)
	if err != nil {
		// Send failure event
		select {
		case eventChan <- &shared.NodeEventCh{
			Type:      shared.EventTypeRunFailed,
			NodeID:    n.NodeID,
			Data:      &shared.RunFailedEvent{RunResult: result},
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

// executeRun performs the core LLM execution logic
func (n *Node) executeRun(ctx context.Context, eventChan chan *shared.NodeEventCh) (*shared.NodeRunResult, error) {
	executionStartedAt := time.Now()
	nodeInputs := make(map[string]any)
	resultText := ""
	usage := &shared.LLMUsage{}
	finishReason := ""
	variablePool := n.GraphRuntimeState.VariablePool
	autoInjectedUserPrompt := false
	logCtx := n.logContext(ctx)

	logger.DebugContext(logCtx, "LLM node execution started",
		zap.String("provider", n.nodeData.Model.Provider),
		zap.String("model", n.nodeData.Model.Name),
		zap.Any("mode", n.nodeData.Model.Mode),
	)

	if err := n.resolveManagedPromptTemplate(ctx); err != nil {
		logger.ErrorContext(logCtx, "failed to resolve managed prompt", err)
		return nil, fmt.Errorf("failed to resolve managed prompt: %w", err)
	}

	// init messages template
	n.nodeData.PromptTemplate = n.transformChatMessages(n.nodeData.PromptTemplate)

	// fetch variables and fetch values from variable pool
	inputs, err := n.fetchInputs(n.nodeData)
	if err != nil {
		logger.ErrorContext(logCtx, "failed to fetch LLM node inputs", err)
		return nil, fmt.Errorf("failed to fetch inputs: %v", err)
	}
	logger.DebugContext(logCtx, "LLM node inputs fetched",
		zap.Int("inputs_count", len(inputs)),
	)

	// Fetch template inputs
	templateInputs, err := n.fetchTemplateInputs(n.nodeData)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch template inputs: %v", err)
	}

	// Merge inputs
	for k, v := range templateInputs {
		inputs[k] = v
	}

	// Make sure all fetched variables are recorded in the Node's input snapshot
	for k, v := range inputs {
		nodeInputs[k] = v
	}

	// Fetch files if vision is enabled
	files := make([]any, 0, 10)
	resolvedFiles := make([]*file.File, 0, 10)
	if n.nodeData.Vision.Enabled {
		fileList, err := n.fetchFiles(variablePool, n.nodeData.Vision.Configs.VariableSelector)
		if err != nil {
			return nil, fmt.Errorf("failed to fetch files: %v", err)
		}
		resolvedFiles = fileList
		for _, f := range fileList {
			files = append(files, f.ToDict())
		}
		if len(files) > 0 {
			nodeInputs["#files#"] = files
		}
	}

	// Legacy node context is intentionally ignored. Conversation context is resolved
	// through node-level conversation_history below or legacy workflow fallback.
	c := ""

	// Resolve model configuration with priority (override vs default)
	workflowRunID := n.getWorkflowRunID()
	resolvedModelConfig, resolvedModelSource, err := n.resolveModelConfig(variablePool)
	if err != nil {
		logger.CriticalContext(logCtx, "failed to resolve LLM node model config", err)
		return nil, fmt.Errorf("failed to resolve model config: %v", err)
	}

	stop, err := parseAndRemoveStopParameter(resolvedModelConfig.CompletionParams)
	if err != nil {
		logger.CriticalContext(logCtx, "failed to parse LLM node stop parameter",
			err,
			zap.String("provider", resolvedModelConfig.Provider),
			zap.String("model", resolvedModelConfig.Name),
		)
		return nil, fmt.Errorf("failed to parse stop parameter: %v", err)
	}

	modelConfig := &ModelConfigWithCredentialsEntity{
		Provider: resolvedModelConfig.Provider,
		Model:    resolvedModelConfig.Name,
		ModelSchema: ModelSchema{
			Provider:  Provider{Provider: resolvedModelConfig.Provider},
			ModelName: resolvedModelConfig.Name,
			ModelType: ModelTypeLLM,
		},
		Mode:       resolvedModelConfig.Mode,
		Parameters: resolvedModelConfig.CompletionParams,
		Stop:       stop,
	}
	logCtx = logger.WithFields(logCtx,
		zap.String("provider", resolvedModelConfig.Provider),
		zap.String("model", resolvedModelConfig.Name),
		zap.String("model_source", resolvedModelSource),
		zap.String("workflow_run_id", workflowRunID),
	)
	logger.DebugContext(logCtx, "LLM node model config resolved")

	// Fetch conversation history memory.
	memory, resolvedMemoryConfig := n.fetchMemory(logCtx, variablePool, n.APPID, n.nodeData.Memory, nil)

	// Get query from memory or system variable
	var query string
	if n.nodeData.Memory != nil {
		if n.WorkflowType == "advanced-chat" || n.WorkflowType == "chat" {
			if n.nodeData.Memory.QueryPromptTemplate != "" {
				segmentGroup := variablePool.ConvertTemplate(n.nodeData.Memory.QueryPromptTemplate)
				query = n.segmentGroupToText(segmentGroup)
			} else {
				if queryVar := variablePool.Get([]string{"sys", "query"}); queryVar != nil {
					query = queryVar.ToObject().(string)
				}
			}
			logger.DebugContext(logCtx, "LLM node chat query resolved",
				zap.Int("query_length", len(query)),
			)
		} else {
			if queryVar := variablePool.Get([]string{"sys", "query"}); queryVar != nil {
				if s, ok := queryVar.ToObject().(string); ok {
					query = s
					logger.DebugContext(logCtx, "LLM node task query resolved",
						zap.Int("query_length", len(query)),
					)
				}
			} else {
				logger.DebugContext(logCtx, "LLM node task query missing")
				query = ""
			}
		}
	}

	// Fetch prompt messages - this is the core functionality
	promptMessages, stop, autoInjectedUserPrompt, err := n.fetchPromptMessages(
		query,
		files,
		c,
		memory,
		modelConfig,
		n.nodeData.PromptTemplate,
		resolvedMemoryConfig,
		n.nodeData.Vision.Enabled,
		n.nodeData.Vision.Configs.Detail,
		variablePool,
		n.nodeData.PromptConfig.TemplateVariables,
	)
	if err != nil {
		failedErr := fmt.Errorf("failed to fetch prompt messages: %v", err)
		processData := n.buildLLMProcessData(
			modelConfig,
			resolvedModelSource,
			resolvedFiles,
			promptMessages,
			autoInjectedUserPrompt,
			nil,
			"",
		)
		processData["failed_stage"] = "prompt_messages"
		processData["failure_error"] = failedErr.Error()
		return &shared.NodeRunResult{
			Status:      shared.FAILED,
			Inputs:      nodeInputs,
			ProcessData: processData,
			Outputs:     map[string]any{},
			Metadata:    buildLLMMetadata(modelConfig, resolvedModelSource, nil),
			ErrMsg:      failedErr.Error(),
			ErrType:     "PromptPreparationError",
		}, failedErr
	}
	promptReadyAt := time.Now()
	logger.InfoContext(logCtx, "workflow llm prompt_ready",
		zap.Int("prompt_messages_count", len(promptMessages)),
		zap.Int("prompt_text_length", promptMessagesTextLength(promptMessages)),
		zap.Int64("elapsed_ms", promptReadyAt.Sub(executionStartedAt).Milliseconds()),
	)

	nodeInputs["prompt"] = promptMessages

	var structuredOutput map[string]any
	if n.nodeData.IsStructuredOutputEnabled() {
		structuredOutput = n.nodeData.StructuredOutput
	}

	invokeRequest := &LLMInvokeRequest{
		ProviderSlug:     modelConfig.Provider,
		ModelSlug:        modelConfig.Model,
		Messages:         promptMessages,
		Parameters:       modelConfig.Parameters,
		Stop:             stop,
		UserID:           n.UserID,
		StructuredOutput: structuredOutput,
	}
	llmGatewayRequest := buildGatewayRequestSnapshot(invokeRequest)

	logger.DebugContext(logCtx, "invoking LLM from workflow node",
		zap.Int("prompt_messages_count", len(promptMessages)),
		zap.Int("stop_count", len(stop)),
		zap.Bool("structured_output_enabled", structuredOutput != nil),
	)

	// Invoke LLM and get result directly
	invokeResult, err := n.invokeLLMWithResult(
		ctx,
		invokeRequest,
		n.llmFileSaver,
		&n.fileOutputs,
		eventChan,
	)
	if err != nil {
		logger.CriticalContext(logCtx, "failed to invoke LLM from workflow node", err)
		failedErr := fmt.Errorf("failed to invoke LLM: %w", err)
		processData := n.buildLLMProcessData(
			modelConfig,
			resolvedModelSource,
			resolvedFiles,
			promptMessages,
			autoInjectedUserPrompt,
			nil,
			"",
		)
		processData["llm_gateway_request"] = llmGatewayRequest
		processData["failed_stage"] = "llm_invoke"
		processData["failure_error"] = failedErr.Error()
		return &shared.NodeRunResult{
			Status:      shared.FAILED,
			Inputs:      nodeInputs,
			ProcessData: processData,
			Outputs:     map[string]any{},
			Metadata:    buildLLMMetadata(modelConfig, resolvedModelSource, nil),
			ErrMsg:      failedErr.Error(),
			ErrType:     "LLMInvokeError",
		}, failedErr
	}
	logger.DebugContext(logCtx, "LLM invocation completed for workflow node")

	// Extract result from LLM response
	resultText = invokeResult.Text
	usage = invokeResult.Usage
	if invokeResult.FinishReason != nil {
		finishReason = *invokeResult.FinishReason
	} else {
		finishReason = "completed"
	}

	usageTotalTokens := 0
	usagePromptTokens := 0
	usageCompletionTokens := 0
	if usage != nil {
		usageTotalTokens = usage.TotalTokens
		usagePromptTokens = usage.PromptTokens
		usageCompletionTokens = usage.CompletionTokens
	}
	logger.DebugContext(logCtx, "LLM node result received",
		zap.Int("result_text_length", len(resultText)),
		zap.String("finish_reason", finishReason),
		zap.Bool("has_structured_output", invokeResult.StructuredOutput != nil),
		zap.Int("total_tokens", usageTotalTokens),
		zap.Int("prompt_tokens", usagePromptTokens),
		zap.Int("completion_tokens", usageCompletionTokens),
	)

	processData := n.buildLLMProcessData(
		modelConfig,
		resolvedModelSource,
		resolvedFiles,
		promptMessages,
		autoInjectedUserPrompt,
		usage,
		finishReason,
	)
	processData["llm_gateway_request"] = llmGatewayRequest

	// Create outputs
	outputs := map[string]any{
		"text":          resultText,
		"usage":         usage,
		"finish_reason": finishReason,
	}

	// Add structured output if enabled and available
	if n.nodeData.IsStructuredOutputEnabled() {
		if invokeResult.StructuredOutput != nil {
			outputs["structured_output"] = invokeResult.StructuredOutput
			logger.DebugContext(logCtx, "using structured output from LLM",
				zap.Bool("has_structured_output", true),
			)
		} else {
			logger.DebugContext(logCtx, "LLM did not return structured output, generating from schema")
			structuredOutput, err := n.generateStructuredOutput()
			if err != nil {
				// Log error but don't fail the execution
				logger.WarnContext(logCtx, "failed to generate structured output from schema", err)
				structuredOutput = map[string]any{"error": "Failed to generate structured output"}
			}
			outputs["structured_output"] = structuredOutput
		}
	}

	// Add files if any
	if len(n.fileOutputs) > 0 {
		outputs["files"] = n.fileOutputs
	}

	// Create metadata
	metadata := buildLLMMetadata(modelConfig, resolvedModelSource, usage)

	return &shared.NodeRunResult{
		Status:      shared.SUCCEEDED,
		Inputs:      nodeInputs,
		ProcessData: processData,
		Outputs:     outputs,
		Metadata:    metadata,
		LLMUsage:    usage,
	}, nil
}

// generateStructuredOutput generates structured output based on the configured schema
func (n *Node) generateStructuredOutput() (map[string]any, error) {
	if n.nodeData.StructuredOutput == nil {
		return nil, fmt.Errorf("no structured output configuration found")
	}

	// Get the schema
	schema, err := n.fetchStructuredOutputSchema(n.nodeData.StructuredOutput)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch schema: %w", err)
	}

	// Generate output based on schema structure
	return n.generateOutputFromSchema(schema)
}

// generateOutputFromSchema generates sample output that conforms to the JSON schema
func (n *Node) generateOutputFromSchema(schema map[string]any) (map[string]any, error) {
	schemaType, ok := schema["type"].(string)
	if !ok {
		return nil, fmt.Errorf("schema missing type field")
	}

	switch schemaType {
	case "object":
		return n.generateObjectFromSchema(schema)
	case "array":
		return map[string]any{"array_result": []any{"sample_item"}}, nil
	case "string":
		return map[string]any{"result": "Generated response based on schema"}, nil
	case "number", "integer":
		return map[string]any{"result": 42}, nil
	case "boolean":
		return map[string]any{"result": true}, nil
	default:
		return map[string]any{"result": "Generated structured output"}, nil
	}
}

// generateObjectFromSchema generates an object that matches the schema properties
func (n *Node) generateObjectFromSchema(schema map[string]any) (map[string]any, error) {
	result := make(map[string]any)

	// Get properties from schema
	properties, ok := schema["properties"].(map[string]any)
	if !ok {
		// No properties defined, return a simple object
		return map[string]any{"generated": "structured output"}, nil
	}

	// Get required fields
	requiredFields := make(map[string]bool)
	if required, ok := schema["required"].([]any); ok {
		for _, field := range required {
			if fieldName, ok := field.(string); ok {
				requiredFields[fieldName] = true
			}
		}
	}

	// Generate values for each property
	for propName, propSchema := range properties {
		propMap, ok := propSchema.(map[string]any)
		if !ok {
			continue
		}

		propType, ok := propMap["type"].(string)
		if !ok {
			continue
		}

		// Generate value based on property type
		switch propType {
		case "string":
			if description, exists := propMap["description"]; exists {
				result[propName] = fmt.Sprintf("Generated %s: %v", propName, description)
			} else {
				result[propName] = fmt.Sprintf("Generated %s value", propName)
			}
		case "number", "integer":
			result[propName] = 42
		case "boolean":
			result[propName] = true
		case "array":
			result[propName] = []any{"item1", "item2"}
		case "object":
			// Recursively generate nested object
			nestedObj, _ := n.generateObjectFromSchema(propMap)
			result[propName] = nestedObj
		default:
			result[propName] = fmt.Sprintf("Unknown type: %s", propType)
		}
	}

	// Ensure all required fields are present
	for requiredField := range requiredFields {
		if _, exists := result[requiredField]; !exists {
			result[requiredField] = fmt.Sprintf("Required field: %s", requiredField)
		}
	}

	// If no properties were processed, return a default object
	if len(result) == 0 {
		result["generated_response"] = "Structured output generated successfully"
	}

	return result, nil
}

// transformChatMessages transforms chat messages based on edition type
func (n *Node) transformChatMessages(promptTemplate any) any {
	// Handle different prompt template types

	switch pt := promptTemplate.(type) {
	case []NodeChatModelMessage:
		for i := range pt {
			if pt[i].EditionType == "template" && pt[i].TemplateText != nil {
				pt[i].Text = *pt[i].TemplateText
			}
		}
		return pt
	case NodeCompletionModelPromptTemplate:
		if pt.EditionType == "template" && pt.TemplateText != nil {
			pt.Text = *pt.TemplateText
		}
		return pt
	default:
		return promptTemplate
	}
}

func resolveSelectorVariable(variablePool *entities.VariablePool, selector []string) entities.Variable {
	if variablePool == nil || len(selector) == 0 {
		return nil
	}
	return variablePool.GetWithPath(selector)
}

func normalizeTemplateInputValue(value any) any {
	switch v := value.(type) {
	case float64:
		if v == math.Trunc(v) {
			return int64(v)
		}
		return v
	case []any:
		result := make([]any, len(v))
		for i, item := range v {
			result[i] = normalizeTemplateInputValue(item)
		}
		return result
	case map[string]any:
		result := make(map[string]any, len(v))
		for key, item := range v {
			result[key] = normalizeTemplateInputValue(item)
		}
		return result
	default:
		return value
	}
}

// fetchInputs fetches inputs from variable pool based on prompt template
func (n *Node) fetchInputs(nodeData NodeData) (map[string]any, error) {
	inputs := make(map[string]any)
	// TODO: improve PromptTemplate
	promptTemplate := nodeData.PromptTemplate

	var variableSelectors []VariableSelector

	// Extract variable selectors based on prompt template type
	switch pt := promptTemplate.(type) {
	case []NodeChatModelMessage:
		for _, prompt := range pt {
			parser := NewVariableTemplateParser(prompt.Text)
			variableSelectors = append(variableSelectors, parser.ExtractVariableSelectors()...)
		}
	case NodeCompletionModelPromptTemplate:
		parser := NewVariableTemplateParser(pt.Text)
		variableSelectors = parser.ExtractVariableSelectors()
	}

	// Get variables from variable pool
	for _, selector := range variableSelectors {
		variable := resolveSelectorVariable(n.GraphRuntimeState.VariablePool, selector.ValueSelector)
		if variable == nil {
			return nil, fmt.Errorf("variable %s not found", selector.Variable)
		}
		inputs[selector.Variable] = variable.ToObject()
	}

	// Handle memory query prompt template
	if nodeData.Memory != nil && nodeData.Memory.QueryPromptTemplate != "" {
		parser := NewVariableTemplateParser(nodeData.Memory.QueryPromptTemplate)
		querySelectors := parser.ExtractVariableSelectors()
		for _, selector := range querySelectors {
			variable := resolveSelectorVariable(n.GraphRuntimeState.VariablePool, selector.ValueSelector)
			if variable != nil {
				inputs[selector.Variable] = variable.ToObject()
			}
		}
	}

	return inputs, nil
}

// fetchTemplateInputs fetches template inputs
func (n *Node) fetchTemplateInputs(nodeData NodeData) (map[string]any, error) {
	variables := make(map[string]any)

	if nodeData.PromptConfig.TemplateVariables == nil {
		return variables, nil
	}

	for _, selector := range nodeData.PromptConfig.TemplateVariables {
		variable := resolveSelectorVariable(n.GraphRuntimeState.VariablePool, selector.ValueSelector)
		if variable == nil {
			return nil, fmt.Errorf("variable %s not found", selector.Variable)
		}

		var value string
		switch variable.GetType() {
		case shared.SegmentTypeArrayAny,
			shared.SegmentTypeArrayString,
			shared.SegmentTypeArrayNumber,
			shared.SegmentTypeArrayObject,
			shared.SegmentTypeArrayFile,
			shared.SegmentTypeArrayBoolean:
			obj := variable.ToObject()
			if arr, ok := obj.([]any); ok {
				var result strings.Builder

				for _, item := range arr {
					if itemMap, ok := item.(map[string]any); ok {
						result.WriteString(parseDict(itemMap))
					} else {
						result.WriteString(fmt.Sprintf("%v", item))
					}
					result.WriteString("\n")
				}

				value = strings.TrimSpace(result.String())
			} else {
				value = variable.Text()
			}
		case shared.SegmentTypeObject:
			obj := variable.ToObject()
			if objMap, ok := obj.(map[string]any); ok {
				value = parseDict(objMap)
			} else {
				value = variable.Text()
			}
		default:
			value = variable.Text()
		}

		variables[selector.Variable] = value

	}

	return variables, nil
}

func parseDict(inputDict map[string]any) string {
	if metadata, ok := inputDict["metadata"]; ok {
		if metaMap, ok := metadata.(map[string]any); ok {
			if _, hasSource := metaMap["_source"]; hasSource {
				if content, hasContent := inputDict["content"]; hasContent {
					return fmt.Sprintf("%v", content)
				}
			}
		}
	}
	jsonBytes, err := json.Marshal(inputDict)
	if err != nil {
		return fmt.Sprintf("%v", inputDict)
	}
	return string(jsonBytes)
}

// convertEntityFileToWorkflowFile converts entities.File to file.File
func convertEntityFileToWorkflowFile(entityFile *entities.File) *file.File {
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
		file.WithStorageKey(entityFile.StorageKey),
	}
	if transferMethod == file.FileTransferMethodRemoteURL && entityFile.RemoteURL != "" {
		opts = append(opts, file.WithRemoteURL(entityFile.RemoteURL))
	}

	return file.NewFile(entityFile.WorkspaceID, kind, transferMethod, opts...)
}

// fetchFiles fetches files from variable pool for vision processing
func (n *Node) fetchFiles(variablePool *entities.VariablePool, selector []string) ([]*file.File, error) {
	if len(selector) == 0 {
		return []*file.File{}, nil
	}

	variable := variablePool.GetWithPath(selector)
	if variable == nil {
		if fallbackFiles := n.fetchFilesFromUserInputs(variablePool, selector); len(fallbackFiles) > 0 {
			logger.DebugContext(n.logContext(context.Background()), "vision selector recovered files from user inputs",
				zap.Int("selector_depth", len(selector)),
				zap.Int("file_count", len(fallbackFiles)),
			)
			return fallbackFiles, nil
		}
		return []*file.File{}, nil
	}

	return n.convertVariableToWorkflowFiles(variable)
}

func (n *Node) convertVariableToWorkflowFiles(variable entities.Variable) ([]*file.File, error) {
	// Check variable type using GetType() method
	varType := variable.GetType()
	switch varType {
	case shared.SegmentTypeFile:
		// FileSegment: return single file as array
		if fileValue := variable.GetValue(); fileValue != nil {
			if entityFile, ok := fileValue.(*entities.File); ok {
				// Enrich file info from database if StorageKey is missing
				n.enrichFileFromDB(entityFile)
				convertedFile := convertEntityFileToWorkflowFile(entityFile)
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
					// Enrich file info from database if StorageKey is missing
					n.enrichFileFromDB(entityFile)
					convertedFile := convertEntityFileToWorkflowFile(entityFile)
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

func (n *Node) fetchFilesFromUserInputs(variablePool *entities.VariablePool, selector []string) []*file.File {
	if variablePool == nil || len(selector) < 2 {
		return nil
	}

	rawValue, exists := variablePool.UserInputs[selector[1]]
	if !exists {
		return nil
	}

	switch value := rawValue.(type) {
	case map[string]any:
		if workflowFile := n.rawFileMapToWorkflowFile(value); workflowFile != nil {
			return []*file.File{workflowFile}
		}
	case []any:
		result := make([]*file.File, 0, len(value))
		for _, item := range value {
			fileMap, ok := item.(map[string]any)
			if !ok {
				continue
			}
			workflowFile := n.rawFileMapToWorkflowFile(fileMap)
			if workflowFile != nil {
				result = append(result, workflowFile)
			}
		}
		if len(result) > 0 {
			return result
		}
	}

	return nil
}

func (n *Node) rawFileMapToWorkflowFile(fileMap map[string]any) *file.File {
	if len(fileMap) == 0 {
		return nil
	}

	extension := firstNonEmptyString(getStringFromMap(fileMap, "extension"), getStringFromMap(fileMap, "ext"))
	mimeType := firstNonEmptyString(getStringFromMap(fileMap, "mime_type"), getStringFromMap(fileMap, "content_type"))

	entityFile := &entities.File{
		ID:             firstNonEmptyString(getStringFromMap(fileMap, "upload_file_id"), getStringFromMap(fileMap, "id"), getStringFromMap(fileMap, "related_id")),
		Type:           file.NormalizeFileType(getStringFromMap(fileMap, "type"), extension, mimeType),
		TransferMethod: firstNonEmptyString(getStringFromMap(fileMap, "transfer_method"), "local_file"),
		RemoteURL:      firstNonEmptyString(getStringFromMap(fileMap, "remote_url"), getStringFromMap(fileMap, "url")),
		Filename:       firstNonEmptyString(getStringFromMap(fileMap, "filename"), getStringFromMap(fileMap, "name")),
		Extension:      extension,
		MimeType:       mimeType,
		Size:           getInt64FromMap(fileMap, "size"),
	}

	if entityFile.ID == "" && entityFile.RemoteURL == "" {
		return nil
	}

	n.enrichFileFromDB(entityFile)
	return convertEntityFileToWorkflowFile(entityFile)
}

func firstNonEmptyString(values ...string) string {
	for _, value := range values {
		if value != "" {
			return value
		}
	}
	return ""
}

func getInt64FromMap(m map[string]any, key string) int64 {
	value, exists := m[key]
	if !exists {
		return 0
	}

	switch v := value.(type) {
	case int:
		return int64(v)
	case int64:
		return v
	case float64:
		return int64(v)
	default:
		return 0
	}
}

// fetchContext fetches context from variable pool
func (n *Node) fetchContext(nodeData NodeData, eventChan chan *shared.NodeEventCh) (string, error) {
	if !nodeData.Context.Enabled || len(nodeData.Context.VariableSelectors) == 0 {
		return "", nil
	}

	// Get the first variable selector's value selector
	variable := resolveSelectorVariable(n.GraphRuntimeState.VariablePool, nodeData.Context.VariableSelectors[0].ValueSelector)
	if variable == nil {
		return "", nil
	}

	// Handle different variable types
	obj := variable.ToObject()
	switch v := obj.(type) {
	case string:
		// Send retriever resource event
		select {
		case eventChan <- &shared.NodeEventCh{
			Type:   shared.EventTypeRetrieverResource,
			NodeID: n.NodeID,
			Data: RunRetrieverResourceEventData{
				RetrieverResources: []*shared.RetrievalSourceMetadata{},
				Context:            v,
			},
			Timestamp: time.Now(),
		}:
		default:
		}
		return v, nil
	case []any:
		// Handle array context
		var contextBuilder strings.Builder
		var retrieverResources []*shared.RetrievalSourceMetadata

		for _, item := range v {

			if str, ok := item.(string); ok {
				contextBuilder.WriteString(str)
				contextBuilder.WriteString("\n")
			} else {
				if itemMap, ok := item.(map[string]any); ok {
					content, ok := itemMap["content"]
					if !ok {
						return "", fmt.Errorf("invalid context structure: missing content field")
					}
					contextBuilder.WriteString(fmt.Sprintf("%v", content))
					contextBuilder.WriteString("\n")

					// Extract retriever resource if available using convertToOriginalRetrieverResource
					if retrieverResource := n.convertToOriginalRetrieverResource(itemMap); retrieverResource != nil {
						retrieverResources = append(retrieverResources, retrieverResource)
					}

				}
			}
		}

		contextStr := strings.TrimSpace(contextBuilder.String())

		// Send retriever resource event
		select {
		case eventChan <- &shared.NodeEventCh{
			Type:   "run_retriever_resource",
			NodeID: n.NodeID,
			Data: RunRetrieverResourceEventData{
				RetrieverResources: retrieverResources,
				Context:            contextStr,
			},
			Timestamp: time.Now(),
		}:
		default:
		}

		return contextStr, nil
	default:
		return "", nil
	}
}

// fetchMemory fetches memory for conversation history.
func (n *Node) fetchMemory(ctx context.Context, variablePool *entities.VariablePool, appID string, memoryConfig *MemoryConfig, modelInstance *ModelInstance) (*TokenBufferMemory, *MemoryConfig) {
	resolvedMemoryConfig := n.memoryConfigForConversationHistory(memoryConfig)
	conversationID := n.getConversationIDFromVariablePool(variablePool)
	messages := make([]PromptMessage, 0)

	switch {
	case n.nodeData.ConversationHistory != nil:
		if n.nodeData.ConversationHistory.Enabled {
			windowSize := clampConversationHistoryWindowSize(n.nodeData.ConversationHistory.HistoryWindowSize)
			var err error
			messages, err = n.loadConversationHistoryPromptMessages(ctx, conversationID, windowSize)
			if err != nil {
				logger.WarnContext(ctx, "LLM node failed to load node-level conversation history",
					err,
					zap.String("conversation_id", conversationID),
					zap.Int("history_window_size", windowSize),
				)
				messages = []PromptMessage{}
			}
			logger.DebugContext(ctx, "LLM node resolved node-level conversation history",
				zap.Bool("enabled", true),
				zap.Int("history_window_size", windowSize),
				zap.Int("prompt_messages_count", len(messages)),
			)
		} else {
			logger.DebugContext(ctx, "LLM node conversation history disabled by node config")
		}

	default:
		if providedMessages, ok := n.promptMessagesFromProvidedHistory(ctx, variablePool); ok {
			messages = providedMessages
			logger.DebugContext(ctx, "LLM node using explicitly provided conversation history",
				zap.Int("prompt_messages_count", len(messages)),
			)
			break
		}

		if fallback, ok := n.legacyWorkflowConversationHistoryConfig(); ok && fallback.Enabled {
			if fallback.HistoryWindowSize == 0 {
				logger.DebugContext(ctx, "LLM node legacy workflow conversation history disabled by zero window")
				break
			}
			windowSize := clampConversationHistoryWindowSize(fallback.HistoryWindowSize)
			var err error
			messages, err = n.loadConversationHistoryPromptMessages(ctx, conversationID, windowSize)
			if err != nil {
				logger.WarnContext(ctx, "LLM node failed to load legacy workflow conversation history",
					err,
					zap.String("conversation_id", conversationID),
					zap.Int("history_window_size", windowSize),
				)
				messages = []PromptMessage{}
			}
			logger.DebugContext(ctx, "LLM node resolved legacy workflow conversation history",
				zap.Bool("enabled", true),
				zap.Int("history_window_size", windowSize),
				zap.Int("prompt_messages_count", len(messages)),
			)
		} else {
			logger.DebugContext(ctx, "LLM node has no conversation history config; using empty history")
		}
	}

	convData := map[string]any{
		"id":       conversationID,
		"messages": []any{},
	}

	// TODO: the maxTokens 4000 is a fallback value, it should be calculated based on the model context
	// Calculate max tokens based on memory configuration and model context
	maxTokens := 4000 // Default fallback
	if resolvedMemoryConfig.Window.Size != 0 {
		// Calculate tokens more accurately based on:
		// - Window size (number of messages to keep)
		// - Average tokens per message (varies by content type)
		// - Model context size (if available)
		windowSize := resolvedMemoryConfig.Window.Size

		// Estimate average tokens per message based on typical conversation patterns
		// - Simple text messages: ~50-100 tokens
		// - Detailed messages: ~200-500 tokens
		// Using a conservative estimate of 150 tokens per message
		averageTokensPerMessage := 150

		// Adjust based on model context if available
		if modelInstance != nil {
			// Note: ModelInstance doesn't have ModelSchema field, so we can't get it from modelInstance
			// In a real implementation, this would be fetched from the provider or database

			// Fallback calculation based on window size only
			maxTokens = windowSize * averageTokensPerMessage
		} else {
			// Fallback calculation based on window size only
			maxTokens = windowSize * averageTokensPerMessage
		}
	}

	// Create TokenBufferMemory instance with proper configuration
	memory := NewTokenBufferMemory(
		convData,
		modelInstance,
		maxTokens,
		n.APPID,
		n.UserID,
	)
	memory.HistoryExplicitlyProvided = true
	memory.Messages = append(memory.Messages, messages...)

	return memory, resolvedMemoryConfig
}

func (n *Node) memoryConfigForConversationHistory(memoryConfig *MemoryConfig) *MemoryConfig {
	rolePrefix := RolePrefix{
		User:      "Human",
		Assistant: "Assistant",
	}
	queryPromptTemplate := ""
	if memoryConfig != nil {
		if memoryConfig.RolePrefix.User != "" {
			rolePrefix.User = memoryConfig.RolePrefix.User
		}
		if memoryConfig.RolePrefix.Assistant != "" {
			rolePrefix.Assistant = memoryConfig.RolePrefix.Assistant
		}
		queryPromptTemplate = memoryConfig.QueryPromptTemplate
	}

	return &MemoryConfig{
		RolePrefix: rolePrefix,
		Window: WindowConfig{
			Enabled: false,
			Size:    0,
		},
		QueryPromptTemplate: queryPromptTemplate,
	}
}

func clampConversationHistoryWindowSize(size int) int {
	if size <= 0 {
		return defaultConversationHistoryWindowSize
	}
	if size > maxConversationHistoryWindowSize {
		return maxConversationHistoryWindowSize
	}
	return size
}

func (n *Node) getConversationIDFromVariablePool(variablePool *entities.VariablePool) string {
	if variablePool == nil {
		return ""
	}
	conversationVar := variablePool.Get([]string{"sys", "conversation_id"})
	if conversationVar == nil {
		return ""
	}
	conversationID, ok := conversationVar.ToObject().(string)
	if !ok {
		return ""
	}
	return conversationID
}

func (n *Node) promptMessagesFromProvidedHistory(ctx context.Context, variablePool *entities.VariablePool) ([]PromptMessage, bool) {
	if variablePool == nil {
		return nil, false
	}
	historyVar := variablePool.Get([]string{"sys", "conversation_history"})
	if historyVar == nil {
		return nil, false
	}

	historyObject := historyVar.ToObject()
	logger.DebugContext(ctx, "LLM node found explicit conversation history in variable pool",
		zap.String("history_type", fmt.Sprintf("%T", historyObject)),
	)

	switch history := historyObject.(type) {
	case []map[string]interface{}:
		return promptMessagesFromHistoryMaps(history), true
	case []interface{}:
		items := make([]map[string]interface{}, 0, len(history))
		for _, raw := range history {
			if msg, ok := raw.(map[string]interface{}); ok {
				items = append(items, msg)
			}
		}
		return promptMessagesFromHistoryMaps(items), true
	default:
		logger.WarnContext(ctx, "LLM node explicit conversation history has unsupported type",
			zap.String("history_type", fmt.Sprintf("%T", historyObject)),
		)
		return []PromptMessage{}, true
	}
}

func promptMessagesFromHistoryMaps(history []map[string]interface{}) []PromptMessage {
	messages := make([]PromptMessage, 0, len(history))
	for _, msgMap := range history {
		role := PromptMessageRoleUser
		if rawRole, exists := msgMap["role"]; exists {
			if roleStr, ok := rawRole.(string); ok {
				role = PromptMessageRole(roleStr)
			}
		}

		content := ""
		if rawContent, exists := msgMap["content"]; exists {
			content = fmt.Sprintf("%v", rawContent)
		}
		if content == "" {
			continue
		}

		messages = append(messages, PromptMessage{
			Role:    role,
			Content: content,
		})
	}
	return messages
}

func (n *Node) legacyWorkflowConversationHistoryConfig() (*ConversationHistoryConfig, bool) {
	if n.GraphConfig == nil {
		return nil, false
	}

	featuresRaw, ok := n.GraphConfig["features"]
	if !ok {
		return nil, false
	}
	features, ok := featuresRaw.(map[string]interface{})
	if !ok {
		return nil, false
	}
	conversationHistoryRaw, ok := features["conversation_history"]
	if !ok {
		return nil, false
	}
	conversationHistory, ok := conversationHistoryRaw.(map[string]interface{})
	if !ok {
		return nil, false
	}

	enabled, _ := conversationHistory["enabled"].(bool)
	return &ConversationHistoryConfig{
		Enabled:           enabled,
		HistoryWindowSize: intFromAny(conversationHistory["history_window_size"], defaultConversationHistoryWindowSize),
	}, true
}

func intFromAny(value any, fallback int) int {
	switch v := value.(type) {
	case int:
		return v
	case int32:
		return int(v)
	case int64:
		return int(v)
	case float32:
		return int(v)
	case float64:
		return int(v)
	case json.Number:
		if i, err := v.Int64(); err == nil {
			return int(i)
		}
	}
	return fallback
}

func (n *Node) loadConversationHistoryPromptMessages(ctx context.Context, conversationID string, windowSize int) ([]PromptMessage, error) {
	if conversationID == "" {
		return []PromptMessage{}, nil
	}
	conversationUUID, err := uuid.Parse(conversationID)
	if err != nil {
		return nil, fmt.Errorf("invalid conversation id: %w", err)
	}

	repo := conversation.NewAgentMessageRepository(n.db)
	_, total, err := repo.GetByConversationID(ctx, conversationUUID, 1, 0)
	if err != nil {
		return nil, err
	}
	if total <= 0 {
		return []PromptMessage{}, nil
	}

	limit := clampConversationHistoryWindowSize(windowSize)
	offset := int(total) - limit
	if offset < 0 {
		offset = 0
	}
	records, _, err := repo.GetByConversationID(ctx, conversationUUID, limit, offset)
	if err != nil {
		return nil, err
	}
	return promptMessagesFromAgentMessages(records), nil
}

func promptMessagesFromAgentMessages(records []*conversation.AgentMessage) []PromptMessage {
	messages := make([]PromptMessage, 0, len(records)*2)
	for _, record := range records {
		if record == nil {
			continue
		}
		if record.Query != "" {
			messages = append(messages, PromptMessage{
				Role:    PromptMessageRoleUser,
				Content: record.Query,
			})
		}
		if record.Answer != "" {
			messages = append(messages, PromptMessage{
				Role:    PromptMessageRoleAssistant,
				Content: record.Answer,
			})
		}
	}
	return messages
}

func (n *Node) fetchChatPromptMessagesWithLayout(
	messages []NodeChatModelMessage,
	sysQuery string,
	contextText string,
	memory *TokenBufferMemory,
	modelConfig *ModelConfigWithCredentialsEntity,
	memoryConfig *MemoryConfig,
	variablePool *entities.VariablePool,
	templateVariables []VariableSelector,
	visionDetail ImagePromptMessageContentDetail,
) ([]PromptMessage, error) {
	promptMessages := make([]PromptMessage, 0, 20)

	var systemMessages []NodeChatModelMessage
	groups := make(map[string][]NodeChatModelMessage)
	for _, msg := range messages {
		if msg.Role == PromptMessageRoleSystem {
			systemMessages = append(systemMessages, msg)
			continue
		}
		if msg.GroupID == "" {
			continue
		}
		groups[msg.GroupID] = append(groups[msg.GroupID], msg)
	}

	if len(systemMessages) > 0 {
		renderedSystemMessages, err := n.handleChatModelTemplate(
			systemMessages,
			contextText,
			templateVariables,
			variablePool,
			visionDetail,
		)
		if err != nil {
			return nil, fmt.Errorf("failed to handle system template: %v", err)
		}
		promptMessages = append(promptMessages, renderedSystemMessages...)
	}

	memoryMessages := n.handleMemoryForChatMode(
		memory,
		memoryConfig,
		modelConfig,
	)
	promptMessages = append(promptMessages, memoryMessages...)
	currentUserAdded := false

	for _, item := range n.nodeData.PromptLayout.Items {
		switch item.Type {
		case PromptLayoutItemHistory:
			continue
		case PromptLayoutItemGroup:
			groupMessages := groups[item.GroupID]
			if len(groupMessages) == 0 {
				continue
			}
			if promptGroupContainsCurrentUser(groupMessages) {
				currentUserAdded = true
			}
			renderedGroupMessages, err := n.handleChatModelTemplate(
				groupMessages,
				contextText,
				templateVariables,
				variablePool,
				visionDetail,
			)
			if err != nil {
				return nil, fmt.Errorf("failed to handle prompt group %s: %v", item.GroupID, err)
			}
			promptMessages = append(promptMessages, renderedGroupMessages...)
		}
	}

	if !currentUserAdded && sysQuery != "" {
		queryMessages, err := n.addCurrentQueryForChat(
			sysQuery,
			variablePool,
			visionDetail,
		)
		if err != nil {
			return nil, fmt.Errorf("failed to add current query for chat: %v", err)
		}
		promptMessages = append(promptMessages, queryMessages...)
	}

	return promptMessages, nil
}

func promptGroupContainsCurrentUser(messages []NodeChatModelMessage) bool {
	for _, msg := range messages {
		if msg.GroupKind == PromptGroupKindCurrentUser {
			return true
		}
		if msg.Role == PromptMessageRoleUser && strings.Contains(msg.Text, "#sys.query#") {
			return true
		}
		if msg.Role == PromptMessageRoleUser && msg.TemplateText != nil && strings.Contains(*msg.TemplateText, "sys.query") {
			return true
		}
	}
	return false
}

// convertConversationToMemoryFormat converts a conversation model to memory format
func (n *Node) convertConversationToMemoryFormat(conv *chat.Conversation) map[string]any {
	if conv == nil {
		return map[string]any{"id": "", "messages": []any{}}
	}

	messages := make([]any, 0, len(conv.Messages)*2)
	for _, msg := range conv.Messages {
		if msg.Query != "" {
			userMessage := map[string]any{
				"role":       "user",
				"content":    msg.Query,
				"created_at": msg.CreatedAt,
				"message_id": msg.ID,
			}
			messages = append(messages, userMessage)
		}

		if msg.Answer != "" {
			assistantMessage := map[string]any{
				"role":       "assistant",
				"content":    msg.Answer,
				"created_at": msg.CreatedAt,
				"message_id": msg.ID,
			}
			messages = append(messages, assistantMessage)
		}
	}

	return map[string]any{
		"id":             conv.ID,
		"messages":       messages,
		"total_messages": len(messages),
		"created_at":     conv.CreatedAt,
		"updated_at":     conv.UpdatedAt,
	}
}

// convertRoleFromChatMessage converts chat message role to prompt message role
func (n *Node) convertRoleFromChatMessage(role string) PromptMessageRole {
	switch role {
	case "user":
		return PromptMessageRoleUser
	case "assistant":
		return PromptMessageRoleAssistant
	case "system":
		return PromptMessageRoleSystem
	default:
		return PromptMessageRoleUser
	}
}

// replaceBareTemplateToken preserves {{#...#}} placeholders for variable-pool resolution
// and only replaces standalone #...# tokens.
func replaceBareTemplateToken(template string, token string, replacement string) string {
	if template == "" || token == "" || !strings.Contains(template, token) {
		return template
	}

	wrappedToken := "{{" + token + "}}"
	var builder strings.Builder
	builder.Grow(len(template))

	for i := 0; i < len(template); {
		switch {
		case strings.HasPrefix(template[i:], wrappedToken):
			builder.WriteString(wrappedToken)
			i += len(wrappedToken)
		case strings.HasPrefix(template[i:], token):
			builder.WriteString(replacement)
			i += len(token)
		default:
			builder.WriteByte(template[i])
			i++
		}
	}

	return builder.String()
}

func replaceContextPlaceholder(template string, context string) string {
	if template == "" || context == "" {
		return template
	}

	template = strings.ReplaceAll(template, "{{#context#}}", context)
	return strings.ReplaceAll(template, "{#context#}", context)
}

// processConversationalSystemVariables processes conversational workflow system variables
func (n *Node) processConversationalSystemVariables(template string, variablePool *entities.VariablePool) string {
	if strings.Contains(template, "#sys.query#") {
		if queryVar := variablePool.Get([]string{"sys", "query"}); queryVar != nil {
			if query, ok := queryVar.ToObject().(string); ok {
				template = replaceBareTemplateToken(template, "#sys.query#", query)
			}
		}
	}

	if strings.Contains(template, "#sys.conversation_id#") {
		if convVar := variablePool.Get([]string{"sys", "conversation_id"}); convVar != nil {
			if convID, ok := convVar.ToObject().(string); ok {
				template = replaceBareTemplateToken(template, "#sys.conversation_id#", convID)
			}
		}
	}

	if strings.Contains(template, "#sys.dialogue_count#") {
		if countVar := variablePool.Get([]string{"sys", "dialogue_count"}); countVar != nil {
			if count, ok := countVar.ToObject().(int); ok {
				template = replaceBareTemplateToken(template, "#sys.dialogue_count#", fmt.Sprintf("%d", count))
			}
		}
	}

	if strings.Contains(template, "#sys.workflow_run_id#") {
		if runIDVar := variablePool.Get([]string{"sys", "workflow_run_id"}); runIDVar != nil {
			if runID, ok := runIDVar.ToObject().(string); ok {
				template = replaceBareTemplateToken(template, "#sys.workflow_run_id#", runID)
			}
		}
	}

	return template
}

func (n *Node) segmentGroupToText(segmentGroup *entities.SegmentGroup) string {
	if segmentGroup == nil {
		return ""
	}

	var result strings.Builder
	for _, segment := range segmentGroup.Value {
		obj := segment.ToObject()
		if str, ok := obj.(string); ok {
			result.WriteString(str)
		} else {
			result.WriteString(fmt.Sprintf("%v", obj))
		}
	}
	return result.String()
}

// handleChatModelTemplate handles chat model template processing
func (n *Node) handleChatModelTemplate(
	messages []NodeChatModelMessage,
	contextText string,
	templateVariables []VariableSelector, // template variables
	variablePool *entities.VariablePool,
	visionDetail ImagePromptMessageContentDetail,
) ([]PromptMessage, error) {
	promptMessages := make([]PromptMessage, 0, 20)
	logCtx := n.logContext(context.Background())

	for _, message := range messages {
		if message.EditionType == "template" {
			// Process template message (using Pongo2)
			resultText, err := n.renderTemplateMessage(
				message.TemplateText,
				templateVariables,
				variablePool,
			)
			if err != nil {
				return nil, err
			}

			promptMessage := PromptMessage{
				Role:    message.Role,
				Content: resultText,
			}
			promptMessages = append(promptMessages, promptMessage)
		} else {
			// Process basic message
			tmpl := message.Text

			logger.DebugContext(logCtx, "processing basic LLM message template",
				zap.Int("template_length", len(tmpl)),
			)

			if contextText != "" {
				tmpl = replaceContextPlaceholder(tmpl, contextText)
			}

			tmpl = n.processConversationalSystemVariables(tmpl, variablePool)

			// Auto-detect {{#variable#}} syntax and replace with content
			// This allows users to use {{#node.file#}} to reference file content
			contentVars := extractFileContentVariables(tmpl)
			if len(contentVars) > 0 {
				logger.DebugContext(logCtx, "detected file content variables in LLM message",
					zap.Int("content_variable_count", len(contentVars)),
				)
				for _, varPath := range contentVars {
					// varPath is like "1760721508950.f_ile"
					// We need to get "1760721508950.f_ile_content" from variable pool
					contentVarPath := varPath + "_content"

					// Parse the variable path to get selector
					parts := strings.Split(contentVarPath, ".")
					if len(parts) >= 2 {
						selector := parts
						contentVar := resolveSelectorVariable(variablePool, selector)
						if contentVar != nil {
							content := contentVar.ToObject()
							if contentStr, ok := content.(string); ok {
								// Replace {{#varPath#}} with actual content
								placeholder := "{{#" + varPath + "#}}"
								tmpl = strings.ReplaceAll(tmpl, placeholder, contentStr)
								logger.DebugContext(logCtx, "replaced file content variable in LLM message",
									zap.Int("content_length", len(contentStr)),
								)
							} else {
								logger.WarnContext(logCtx, "LLM file content variable is not a string",
									zap.String("content_variable_path", contentVarPath),
									zap.String("content_type", fmt.Sprintf("%T", content)),
								)
							}
						} else {
							logger.WarnContext(logCtx, "LLM file content variable not found",
								zap.String("content_variable_path", contentVarPath),
							)
						}
					}
				}
			}

			segmentGroup := variablePool.ConvertTemplate(tmpl)
			plainText := n.segmentGroupToText(segmentGroup)

			logger.DebugContext(logCtx, "LLM plain text prompt rendered",
				zap.Int("plain_text_length", len(plainText)),
			)

			if plainText != "" {
				promptMessage := PromptMessage{
					Role:    message.Role,
					Content: plainText,
				}
				promptMessages = append(promptMessages, promptMessage)
			}
		}
	}

	return promptMessages, nil
}

// handleCompletionModelTemplate handles completion model template processing
func (n *Node) handleCompletionModelTemplate(
	template NodeCompletionModelPromptTemplate,
	context string,
	templateVariables []VariableSelector,
	variablePool *entities.VariablePool,
) ([]PromptMessage, error) {
	var resultText string

	if template.EditionType == "template" {
		// Process template (using Pongo2)
		text, err := n.renderTemplateMessage(
			template.TemplateText,
			templateVariables,
			variablePool,
		)
		if err != nil {
			return nil, err
		}
		resultText = text
	} else {
		// Process basic template
		templateText := template.Text
		if context != "" {
			templateText = replaceContextPlaceholder(templateText, context)
		}

		templateText = n.processConversationalSystemVariables(templateText, variablePool)

		segmentGroup := variablePool.ConvertTemplate(templateText)
		resultText = n.segmentGroupToText(segmentGroup)
	}

	promptMessage := PromptMessage{
		Role:    PromptMessageRoleUser,
		Content: resultText,
	}

	return []PromptMessage{promptMessage}, nil
}

func isLocalWorkflowFile(transferMethod file.FileTransferMethod) bool {
	return transferMethod == file.FileTransferMethodLocalFile
}

func isSignedPreviewURL(rawURL string) bool {
	parsed, err := url.Parse(rawURL)
	if err != nil {
		return strings.Contains(rawURL, "/file-preview?")
	}
	return strings.HasSuffix(parsed.Path, "/file-preview")
}

func (n *Node) validateLocalVisionFileURL(rawURL string) (*file.ExternalURLInfo, error) {
	info, err := file.InspectExternalURL(rawURL)
	if err != nil {
		return nil, fmt.Errorf("invalid workflow vision file URL: %w", err)
	}
	if !file.IsDevelopmentEnvironment() && !info.IsPublic {
		return info, fmt.Errorf("invalid FILES_URL configuration for workflow vision inputs: generated host %q is not publicly accessible", info.Host)
	}
	return info, nil
}

func collectSelectedVisionURLTrace(promptMessages []PromptMessage) (string, string, string, bool) {
	for _, promptMessage := range promptMessages {
		contentList, ok := promptMessage.Content.([]PromptMessageContent)
		if !ok {
			continue
		}
		for _, part := range contentList {
			if part.Type == PromptMessageContentTypeText || part.URL == "" {
				continue
			}

			transport := "remote_url"
			if isSignedPreviewURL(part.URL) {
				transport = "signed_preview_url"
			}

			info, err := file.InspectExternalURL(part.URL)
			if err != nil {
				return transport, "", "", false
			}
			return transport, info.Host, info.Scheme, info.IsPublic
		}
	}

	return "", "", "", false
}

func (n *Node) collectSelectedVisionURLTraceFromFiles(resolvedFiles []*file.File) (string, string, string, bool) {
	for _, resolvedFile := range resolvedFiles {
		if resolvedFile == nil {
			continue
		}

		fileURL, err := n.getFileURL(resolvedFile)
		if err != nil || fileURL == "" {
			continue
		}

		transport := "remote_url"
		if isLocalWorkflowFile(resolvedFile.TransferMethod) || isSignedPreviewURL(fileURL) {
			transport = "signed_preview_url"
		}

		info, err := file.InspectExternalURL(fileURL)
		if err != nil {
			return transport, "", "", false
		}
		return transport, info.Host, info.Scheme, info.IsPublic
	}

	return "", "", "", false
}

func buildLLMMetadata(
	modelConfig *ModelConfigWithCredentialsEntity,
	resolvedModelSource string,
	usage *shared.LLMUsage,
) map[shared.WorkflowNodeExecutionMetadataKey]any {
	totalTokens := 0
	totalPrice := decimal.Zero
	currency := ""
	if usage != nil {
		totalTokens = usage.TotalTokens
		totalPrice = usage.TotalPrice
		currency = usage.Currency
	}

	return map[shared.WorkflowNodeExecutionMetadataKey]any{
		shared.TotalTokens:           totalTokens,
		shared.TotalPrice:            totalPrice,
		shared.Currency:              currency,
		shared.ResolvedModelProvider: modelConfig.Provider,
		shared.ResolvedModelName:     modelConfig.Model,
		shared.ResolvedModelSource:   resolvedModelSource,
	}
}

func (n *Node) buildLLMProcessData(
	modelConfig *ModelConfigWithCredentialsEntity,
	resolvedModelSource string,
	resolvedFiles []*file.File,
	promptMessages []PromptMessage,
	autoInjectedUserPrompt bool,
	usage *shared.LLMUsage,
	finishReason string,
) map[string]any {
	processData := map[string]any{
		"model_mode":              string(modelConfig.Mode),
		"model_provider":          modelConfig.Provider,
		"model_name":              modelConfig.Model,
		"resolved_model_provider": modelConfig.Provider,
		"resolved_model_name":     modelConfig.Model,
		"resolved_model_source":   resolvedModelSource,
		"usage":                   usage,
		"finish_reason":           finishReason,
	}

	visionTrace := n.buildVisionTrace(resolvedFiles, promptMessages, autoInjectedUserPrompt)
	if selectedTransport, _ := visionTrace["selected_file_transport"].(string); selectedTransport == "" {
		selectedTransport, selectedHost, selectedScheme, selectedIsPublic := n.collectSelectedVisionURLTraceFromFiles(resolvedFiles)
		visionTrace["selected_file_transport"] = selectedTransport
		visionTrace["selected_file_url_host"] = selectedHost
		visionTrace["selected_file_url_scheme"] = selectedScheme
		visionTrace["selected_file_url_is_public"] = selectedIsPublic
	}

	for key, value := range visionTrace {
		processData[key] = value
	}
	for key, value := range n.resolvedPromptMeta {
		processData[key] = value
	}
	return processData
}

func (n *Node) resolveManagedPromptTemplate(ctx context.Context) error {
	if strings.TrimSpace(n.nodeData.PromptSource) == "" || n.nodeData.PromptSource == "inline" {
		n.resolvedPromptMeta = map[string]any{
			"prompt_source": "inline",
		}
		return nil
	}

	if n.nodeData.PromptSource != "managed" {
		return fmt.Errorf("unsupported prompt source %q", n.nodeData.PromptSource)
	}
	if n.promptResolver == nil {
		return fmt.Errorf("prompt resolver is not configured")
	}
	if n.nodeData.PromptReference == nil || strings.TrimSpace(n.nodeData.PromptReference.PromptID) == "" {
		return fmt.Errorf("managed prompt requires prompt_reference.prompt_id")
	}

	ref := promptservice.RuntimePromptReference{
		PromptID: n.nodeData.PromptReference.PromptID,
		Version:  n.nodeData.PromptReference.Version,
		Label:    n.nodeData.PromptReference.Label,
	}

	resolved, err := n.promptResolver.ResolveRuntimeReference(ctx, n.OrganizationID, n.WorkspaceID, ref)
	if err != nil {
		return err
	}
	if resolved == nil || resolved.Prompt == nil || resolved.Version == nil {
		return fmt.Errorf("prompt resolution returned empty result")
	}

	template, err := n.promptVersionToTemplate(resolved.Version)
	if err != nil {
		return err
	}
	n.nodeData.PromptTemplate = template

	labels := append([]string{}, resolved.Version.Labels...)
	meta := map[string]any{
		"prompt_source":                 "managed",
		"managed_prompt_id":             resolved.Prompt.ID,
		"managed_prompt_name":           resolved.Prompt.Name,
		"managed_prompt_version":        resolved.Version.Version,
		"managed_prompt_labels":         labels,
		"managed_prompt_locale":         resolved.Prompt.Locale,
		"managed_prompt_catalog_source": string(resolved.Prompt.Source),
	}
	if n.nodeData.PromptReference.Label != nil {
		meta["managed_prompt_requested_label"] = *n.nodeData.PromptReference.Label
	}
	if n.nodeData.PromptReference.Version != nil {
		meta["managed_prompt_requested_version"] = *n.nodeData.PromptReference.Version
	}
	n.resolvedPromptMeta = meta
	return nil
}

func (n *Node) promptVersionToTemplate(version *promptmodel.PromptVersion) (any, error) {
	switch version.PromptType {
	case promptmodel.PromptTypeText:
		var text string
		if err := json.Unmarshal(version.Content, &text); err != nil {
			return nil, fmt.Errorf("decode managed text prompt: %w", err)
		}
		if n.nodeData.Model.Mode == ModeCompletion {
			return NodeCompletionModelPromptTemplate{
				Text:        text,
				EditionType: "basic",
			}, nil
		}
		return []NodeChatModelMessage{
			{
				Role:        PromptMessageRoleSystem,
				Text:        text,
				EditionType: "basic",
			},
		}, nil
	case promptmodel.PromptTypeChat:
		var messages []struct {
			Role    string `json:"role"`
			Content string `json:"content"`
		}
		if err := json.Unmarshal(version.Content, &messages); err != nil {
			return nil, fmt.Errorf("decode managed chat prompt: %w", err)
		}
		if n.nodeData.Model.Mode == ModeCompletion {
			var builder strings.Builder
			for _, message := range messages {
				if strings.TrimSpace(message.Content) == "" {
					continue
				}
				builder.WriteString(strings.ToUpper(message.Role))
				builder.WriteString(": ")
				builder.WriteString(message.Content)
				builder.WriteString("\n\n")
			}
			return NodeCompletionModelPromptTemplate{
				Text:        strings.TrimSpace(builder.String()),
				EditionType: "basic",
			}, nil
		}
		result := make([]NodeChatModelMessage, 0, len(messages))
		for _, message := range messages {
			result = append(result, NodeChatModelMessage{
				Role:        PromptMessageRole(message.Role),
				Text:        message.Content,
				EditionType: "basic",
			})
		}
		return result, nil
	default:
		return nil, fmt.Errorf("unsupported managed prompt type %q", version.PromptType)
	}
}

// processVisionFiles processes vision files
func (n *Node) processVisionFiles(
	promptMessages []PromptMessage,
	sysFiles []any,
	visionEnabled bool,
	visionDetail ImagePromptMessageContentDetail,
) ([]PromptMessage, bool, error) {
	if !visionEnabled || len(sysFiles) == 0 {
		return promptMessages, false, nil
	}

	filePrompts := make([]PromptMessageContent, 0, len(sysFiles))
	for _, item := range sysFiles {
		var filePrompt PromptMessageContent
		switch f := item.(type) {
		case *file.File:
			fileURL, err := n.getFileURL(f)
			if err != nil {
				return nil, false, err
			}
			if isLocalWorkflowFile(f.TransferMethod) {
				if _, err := n.validateLocalVisionFileURL(fileURL); err != nil {
					return nil, false, err
				}
			}
			filePrompt = PromptMessageContent{
				Type:     n.getFileContentType(f),
				URL:      fileURL,
				MimeType: safeDeref(f.MimeType),
				Detail:   visionDetail,
			}
		case map[string]any:
			transferMethod := file.FileTransferMethod(getStringFromMap(f, "transfer_method"))
			var (
				fileURL string
				err     error
			)
			if isLocalWorkflowFile(transferMethod) {
				fileURL, err = n.resolveFileURLFromMap(f)
				if err != nil {
					return nil, false, err
				}
				if _, err := n.validateLocalVisionFileURL(fileURL); err != nil {
					return nil, false, err
				}
			} else {
				fileURL = firstNonEmptyString(getStringFromMap(f, "url"), getStringFromMap(f, "remote_url"))
				if fileURL == "" {
					fileURL, err = n.resolveFileURLFromMap(f)
					if err != nil {
						return nil, false, err
					}
				}
			}
			filePrompt = PromptMessageContent{
				Type:     n.getFileContentTypeFromMap(f),
				URL:      fileURL,
				MimeType: getStringFromMap(f, "mime_type"),
				Base64:   getStringFromMap(f, "base64_data"),
				Detail:   visionDetail,
			}
		default:
			filePrompt = PromptMessageContent{
				Type:   PromptMessageContentTypeImage,
				URL:    fmt.Sprintf("%v", item),
				Detail: visionDetail,
			}
		}
		logger.DebugContext(n.logContext(context.Background()), "LLM vision file prompt built",
			zap.Any("file_prompt_type", filePrompt.Type),
			zap.String("mime_type", filePrompt.MimeType),
			zap.Any("detail", filePrompt.Detail),
			zap.Bool("has_url", filePrompt.URL != ""),
			zap.Int("base64_length", len(filePrompt.Base64)),
		)
		filePrompts = append(filePrompts, filePrompt)
	}

	logger.DebugContext(n.logContext(context.Background()), "LLM vision file prompts built",
		zap.Int("file_prompt_count", len(filePrompts)),
		zap.Int("system_file_count", len(sysFiles)),
	)
	if len(promptMessages) > 0 && promptMessages[len(promptMessages)-1].Role == PromptMessageRoleUser {
		if contentList, ok := promptMessages[len(promptMessages)-1].Content.([]PromptMessageContent); ok {
			promptMessages[len(promptMessages)-1].Content = append(filePrompts, contentList...)
		} else {
			textContent := PromptMessageContent{
				Type: PromptMessageContentTypeText,
				Data: fmt.Sprintf("%v", promptMessages[len(promptMessages)-1].Content),
			}

			promptMessages[len(promptMessages)-1].Content = append(filePrompts, textContent)
		}
	} else {
		promptMessages = append(promptMessages, PromptMessage{
			Role: PromptMessageRoleUser,
			Content: append(filePrompts, PromptMessageContent{
				Type: PromptMessageContentTypeText,
				Data: defaultVisionUserPromptText,
			}),
		})
		return promptMessages, true, nil
	}

	return promptMessages, false, nil
}

func (n *Node) buildVisionTrace(
	resolvedFiles []*file.File,
	promptMessages []PromptMessage,
	autoInjectedUserPrompt bool,
) map[string]any {
	fileTypes := make([]string, 0, len(resolvedFiles))
	fileURLPresence := make([]bool, 0, len(resolvedFiles))
	for _, resolvedFile := range resolvedFiles {
		if resolvedFile == nil {
			continue
		}
		fileTypes = append(fileTypes, string(n.getFileContentType(resolvedFile)))
		fileURL, err := n.getFileURL(resolvedFile)
		fileURLPresence = append(fileURLPresence, err == nil && fileURL != "")
	}

	selectedTransport, selectedHost, selectedScheme, selectedIsPublic := collectSelectedVisionURLTrace(promptMessages)

	return map[string]any{
		"vision_enabled":                    n.nodeData.Vision.Enabled,
		"vision_selector":                   append([]string(nil), n.nodeData.Vision.Configs.VariableSelector...),
		"resolved_file_count":               len(fileTypes),
		"resolved_file_types":               fileTypes,
		"resolved_file_urls_present":        fileURLPresence,
		"selected_file_transport":           selectedTransport,
		"selected_file_url_host":            selectedHost,
		"selected_file_url_is_public":       selectedIsPublic,
		"selected_file_url_scheme":          selectedScheme,
		"final_prompt_contains_inline_data": containsInlinePromptData(promptMessages),
		"final_prompt_roles":                collectPromptRoles(promptMessages),
		"final_prompt_content_types":        collectPromptContentTypes(promptMessages),
		"auto_injected_user_prompt":         autoInjectedUserPrompt,
	}
}

func collectPromptRoles(promptMessages []PromptMessage) []string {
	roles := make([]string, 0, len(promptMessages))
	for _, promptMessage := range promptMessages {
		roles = append(roles, string(promptMessage.Role))
	}
	return roles
}

func collectPromptContentTypes(promptMessages []PromptMessage) []string {
	contentTypes := make([]string, 0, len(promptMessages))
	for _, promptMessage := range promptMessages {
		switch content := promptMessage.Content.(type) {
		case string:
			contentTypes = append(contentTypes, string(PromptMessageContentTypeText))
		case []PromptMessageContent:
			for _, part := range content {
				contentTypes = append(contentTypes, string(part.Type))
			}
		default:
			if promptMessage.Content != nil {
				contentTypes = append(contentTypes, string(PromptMessageContentTypeText))
			}
		}
	}
	return contentTypes
}

func containsInlinePromptData(promptMessages []PromptMessage) bool {
	for _, promptMessage := range promptMessages {
		contentList, ok := promptMessage.Content.([]PromptMessageContent)
		if !ok {
			continue
		}
		for _, part := range contentList {
			if part.Base64 != "" {
				return true
			}
		}
	}
	return false
}

func (n *Node) inlineMediaContentIfNeeded(prompt *PromptMessageContent, workflowFile *file.File) {
	if prompt == nil || workflowFile == nil {
		return
	}
	if workflowFile.TransferMethod != file.FileTransferMethodLocalFile {
		return
	}
	fileID := firstNonEmptyString(safeDeref(workflowFile.RelatedID), safeDeref(workflowFile.ID))
	n.inlineMediaContentByFileID(prompt, fileID)
}

func (n *Node) inlineMediaContentFromMapIfNeeded(prompt *PromptMessageContent, fileMap map[string]any) {
	if prompt == nil {
		return
	}
	if getStringFromMap(fileMap, "transfer_method") != string(file.FileTransferMethodLocalFile) {
		return
	}
	fileID := firstNonEmptyString(
		getStringFromMap(fileMap, "upload_file_id"),
		getStringFromMap(fileMap, "id"),
		getStringFromMap(fileMap, "related_id"),
	)
	n.inlineMediaContentByFileID(prompt, fileID)
}

func (n *Node) inlineMediaContentByFileID(prompt *PromptMessageContent, fileID string) {
	if prompt == nil || prompt.Base64 != "" || fileID == "" || n.fileLoader == nil {
		return
	}
	switch prompt.Type {
	case PromptMessageContentTypeImage, PromptMessageContentTypeAudio, PromptMessageContentTypeVideo:
	default:
		return
	}

	content, err := n.fileLoader.DownloadFile(context.Background(), fileID)
	if err != nil || len(content) == 0 {
		return
	}

	prompt.Base64 = base64.StdEncoding.EncodeToString(content)
	prompt.URL = ""
}

// handleCompletionTemplate handles completion model template
func (n *Node) handleCompletionTemplate(
	template NodeCompletionModelPromptTemplate,
	context string,
	templateVariables []VariableSelector,
	variablePool *entities.VariablePool,
) ([]PromptMessage, error) {
	var resultText string

	if template.EditionType == "template" {
		// Handle template
		text, err := n.renderTemplateMessage(
			template.TemplateText,
			templateVariables,
			variablePool,
		)
		if err != nil {
			return nil, err
		}
		resultText = text
	} else {
		// Handle basic template
		templateText := template.Text
		if context != "" {
			templateText = replaceContextPlaceholder(templateText, context)
		}
		templateText = n.processConversationalSystemVariables(templateText, variablePool)
		segmentGroup := variablePool.ConvertTemplate(templateText)
		resultText = n.segmentGroupToText(segmentGroup)
	}

	promptMessage := PromptMessage{
		Role:    PromptMessageRoleUser,
		Content: resultText,
	}

	return []PromptMessage{promptMessage}, nil
}

// processTemplateMessageWithFiles processes template message with file support.
func (n *Node) processTemplateMessageWithFiles(
	message NodeChatModelMessage,
	templateVariables []VariableSelector,
	variablePool *entities.VariablePool,
	visionDetail ImagePromptMessageContentDetail,
) (PromptMessage, error) {
	if message.TemplateText == nil || *message.TemplateText == "" {
		return PromptMessage{Role: message.Role, Content: ""}, nil
	}

	// Build template inputs from variables
	templateInputs := make(map[string]any)
	for _, templateVariable := range templateVariables {
		variable := resolveSelectorVariable(variablePool, templateVariable.ValueSelector)
		if variable != nil {
			templateInputs[templateVariable.Variable] = normalizeTemplateInputValue(variable.ToObject())
		} else {
			templateInputs[templateVariable.Variable] = ""
		}
	}

	// Auto-detect {{#variable#}} syntax and add corresponding _content variables
	// This allows users to use {{#node.file#}} without explicitly declaring node.file_content
	if message.TemplateText != nil {
		contentVars := extractFileContentVariables(*message.TemplateText)
		for _, varPath := range contentVars {
			// varPath is like "1760721508950.f_ile"
			// We need to get "1760721508950.f_ile_content" from variable pool
			contentVarPath := varPath + "_content"

			// Parse the variable path to get selector
			// e.g., "1760721508950.f_ile_content" -> ["1760721508950", "f_ile_content"]
			parts := strings.Split(contentVarPath, ".")
			if len(parts) >= 2 {
				selector := parts
				contentVar := resolveSelectorVariable(variablePool, selector)
				if contentVar != nil {
					// Add the content variable to templateInputs
					// Use the full path as key: "1760721508950.f_ile_content"
					templateInputs[contentVarPath] = contentVar.ToObject()
					logger.DebugContext(n.logContext(context.Background()), "auto-added file content variable for LLM template",
						zap.String("content_variable_path", contentVarPath),
					)
				} else {
					logger.WarnContext(n.logContext(context.Background()), "file content variable not found for LLM template",
						zap.String("content_variable_path", contentVarPath),
					)
				}
			}
		}
	}

	logger.DebugContext(n.logContext(context.Background()), "LLM template inputs prepared",
		zap.Int("template_length", len(*message.TemplateText)),
		zap.Int("template_variables_count", len(templateVariables)),
		zap.Int("template_inputs_count", len(templateInputs)),
		zap.Strings("template_input_keys", getTemplateInputKeys(templateInputs)),
	)

	resultText, err := template.ExecutePongo2Template(*message.TemplateText, templateInputs)
	if err != nil {
		return PromptMessage{}, fmt.Errorf("failed to render pongo2 template: %v", err)
	}

	// Process file content from template variables
	fileContents, err := n.extractFileContentsFromVariables(templateVariables, variablePool, visionDetail)
	if err != nil {
		return PromptMessage{}, fmt.Errorf("failed to extract file contents: %v", err)
	}

	// Combine text and file contents
	promptMessage := n.combineTextAndFiles(message.Role, resultText, fileContents)
	return promptMessage, nil
}

// processBasicMessageWithFiles processes basic message with file support.
func (n *Node) processBasicMessageWithFiles(
	message NodeChatModelMessage,
	context string,
	variablePool *entities.VariablePool,
	visionDetail ImagePromptMessageContentDetail,
) (*PromptMessage, error) {
	template := message.Text
	if context != "" {
		template = replaceContextPlaceholder(template, context)
	}
	template = n.processConversationalSystemVariables(template, variablePool)

	// Convert template using variable pool
	segmentGroup := variablePool.ConvertTemplate(template)
	plainText := n.segmentGroupToText(segmentGroup)

	// Extract file contents from segment group
	fileContents, err := n.extractFileContentsFromSegmentGroup(segmentGroup, visionDetail)
	if err != nil {
		return nil, fmt.Errorf("failed to extract file contents from segment group: %v", err)
	}

	if plainText == "" && len(fileContents) == 0 {
		return nil, nil
	}

	// Combine text and file contents
	promptMessage := n.combineTextAndFiles(message.Role, plainText, fileContents)
	return &promptMessage, nil
}

// renderTemplateMessage renders template (legacy method for compatibility)
func (n *Node) renderTemplateMessage(
	templateParam *string,
	templateVariables []VariableSelector,
	variablePool *entities.VariablePool,
) (string, error) {
	logCtx := n.logContext(context.Background())
	logger.DebugContext(logCtx, "rendering LLM template message")

	if templateParam == nil || *templateParam == "" {
		logger.DebugContext(logCtx, "LLM template message is empty")
		return "", nil
	}

	logger.DebugContext(logCtx, "LLM template message metadata",
		zap.Int("template_length", len(*templateParam)),
		zap.Int("template_variables_count", len(templateVariables)),
	)

	// Build template inputs from variables
	templateInputs := make(map[string]any)
	for _, templateVariable := range templateVariables {
		variable := resolveSelectorVariable(variablePool, templateVariable.ValueSelector)
		if variable == nil {
			templateInputs[templateVariable.Variable] = ""
			continue
		}

		// Get variable value
		varValue := normalizeTemplateInputValue(variable.ToObject())

		// Check if it's a file object and extract content
		if fileMap, ok := varValue.(map[string]interface{}); ok {
			if uploadFileID, exists := fileMap["upload_file_id"]; exists {
				if fileIDStr, ok := uploadFileID.(string); ok && fileIDStr != "" {
					// This is a file object, try to get its content
					fileContent := n.extractFileContentFromMap(fileMap, fileIDStr)
					templateInputs[templateVariable.Variable] = fileContent
					continue
				}
			}
		}

		templateInputs[templateVariable.Variable] = varValue
	}

	// Auto-detect {{#variable#}} syntax and add corresponding _content variables
	// This allows users to use {{#node.file#}} without explicitly declaring node.file_content
	contentVars := extractFileContentVariables(*templateParam)
	for _, varPath := range contentVars {
		// varPath is like "1760721508950.f_ile"
		// We need to get "1760721508950.f_ile_content" from variable pool
		contentVarPath := varPath + "_content"

		// Parse the variable path to get selector
		// e.g., "1760721508950.f_ile_content" -> ["1760721508950", "f_ile_content"]
		parts := strings.Split(contentVarPath, ".")
		if len(parts) >= 2 {
			selector := parts
			contentVar := resolveSelectorVariable(variablePool, selector)
			if contentVar != nil {
				// Add the content variable to templateInputs
				// Use the full path as key: "1760721508950.f_ile_content"
				templateInputs[contentVarPath] = contentVar.ToObject()
				logger.DebugContext(logCtx, "auto-added file content variable for LLM template render",
					zap.String("content_variable_path", contentVarPath),
				)
			} else {
				logger.WarnContext(logCtx, "file content variable not found for LLM template render",
					zap.String("content_variable_path", contentVarPath),
				)
			}
		}
	}

	logger.DebugContext(logCtx, "LLM template inputs ready",
		zap.Int("template_inputs_count", len(templateInputs)),
		zap.Strings("template_input_keys", getTemplateInputKeys(templateInputs)),
	)

	resultText, err := template.ExecutePongo2Template(*templateParam, templateInputs)
	if err != nil {
		return "", fmt.Errorf("failed to render pongo2 template: %v", err)
	}
	return resultText, nil
}

// extractFileContentsFromVariables extracts file contents from template variables
func (n *Node) extractFileContentsFromVariables(
	templateVariables []VariableSelector,
	variablePool *entities.VariablePool,
	visionDetail ImagePromptMessageContentDetail,
) ([]PromptMessageContent, error) {
	var fileContents []PromptMessageContent

	for _, templateVariable := range templateVariables {
		variable := resolveSelectorVariable(variablePool, templateVariable.ValueSelector)
		if variable == nil {
			continue
		}

		// Check if variable contains file segments
		fileContent, err := n.extractFileContentFromSegment(variable, visionDetail)
		if err != nil {
			return nil, err
		}
		if fileContent != nil {
			fileContents = append(fileContents, fileContent...)
		}
	}

	return fileContents, nil
}

// extractFileContentsFromSegmentGroup extracts file contents from segment group
func (n *Node) extractFileContentsFromSegmentGroup(
	segmentGroup *entities.SegmentGroup,
	visionDetail ImagePromptMessageContentDetail,
) ([]PromptMessageContent, error) {
	var fileContents []PromptMessageContent

	if segmentGroup == nil {
		return fileContents, nil
	}

	for _, segment := range segmentGroup.Value {
		fileContent, err := n.extractFileContentFromSegment(segment, visionDetail)
		if err != nil {
			return nil, err
		}
		if fileContent != nil {
			fileContents = append(fileContents, fileContent...)
		}
	}

	return fileContents, nil
}

// extractFileContentFromSegment extracts file content from a single segment
func (n *Node) extractFileContentFromSegment(
	segment entities.Segment,
	visionDetail ImagePromptMessageContentDetail,
) ([]PromptMessageContent, error) {
	var fileContents []PromptMessageContent

	switch seg := segment.(type) {
	case *entities.FileSegment:
		if seg.Value != nil {
			content, err := n.convertFileToPromptContent(seg.Value, visionDetail)
			if err != nil {
				return nil, err
			}
			if content != nil {
				fileContents = append(fileContents, *content)
			}
		}
	case *entities.ArrayFileSegment:
		for _, file := range seg.Value {
			if file != nil {
				content, err := n.convertFileToPromptContent(file, visionDetail)
				if err != nil {
					return nil, err
				}
				if content != nil {
					fileContents = append(fileContents, *content)
				}
			}
		}
	}

	return fileContents, nil
}

// convertFileToPromptContent converts a File to PromptMessageContent
func (n *Node) convertFileToPromptContent(
	file *entities.File,
	visionDetail ImagePromptMessageContentDetail,
) (*PromptMessageContent, error) {
	if file == nil {
		return nil, nil
	}

	// Generate file URL - use RemoteURL if available
	var fileURL string
	if file.RemoteURL != "" {
		fileURL = file.RemoteURL
	} else {
		// Generate URL based on file ID
		fileURL = fmt.Sprintf("/api/v1/files/%s", file.ID)
	}

	// Handle different file types
	switch file.Type {
	case "image":
		// Create image content
		return &PromptMessageContent{
			Type: PromptMessageContentTypeImage,
			Data: fileURL,
		}, nil
	case "video":
		// Create video content
		return &PromptMessageContent{
			Type: PromptMessageContentTypeVideo,
			Data: fileURL,
		}, nil
	case "audio":
		// Create audio content
		return &PromptMessageContent{
			Type: PromptMessageContentTypeAudio,
			Data: fileURL,
		}, nil
	case "document":
		// For documents, we might want to extract text content
		// For now, just return the file URL as text
		filename := "document"
		if file.Filename != "" {
			filename = file.Filename
		}
		return &PromptMessageContent{
			Type: PromptMessageContentTypeText,
			Data: fmt.Sprintf("[%s](%s)", filename, fileURL),
		}, nil
	default:
		// Unknown file type, treat as document
		filename := "file"
		if file.Filename != "" {
			filename = file.Filename
		}
		return &PromptMessageContent{
			Type: PromptMessageContentTypeText,
			Data: fmt.Sprintf("[%s](%s)", filename, fileURL),
		}, nil
	}
}

// combineTextAndFiles combines text content with file contents into a PromptMessage
func (n *Node) combineTextAndFiles(
	role PromptMessageRole,
	textContent string,
	fileContents []PromptMessageContent,
) PromptMessage {
	if len(fileContents) == 0 {
		// Only text content
		return PromptMessage{
			Role:    role,
			Content: textContent,
		}
	}

	// Combine text and files
	var contents []PromptMessageContent

	// Add text content if not empty
	if textContent != "" {
		contents = append(contents, PromptMessageContent{
			Type: PromptMessageContentTypeText,
			Data: textContent,
		})
	}

	// Add file contents
	contents = append(contents, fileContents...)

	return PromptMessage{
		Role:    role,
		Content: contents,
	}
}

// handleMemoryCompletionMode handles memory for completion mode
func (n *Node) handleMemoryCompletionMode(
	memory any,
	memoryConfig *MemoryConfig,
	modelConfig *ModelConfigWithCredentialsEntity,
) string {
	if memory == nil || memoryConfig == nil {
		return ""
	}

	// Cast memory to TokenBufferMemory
	tokenBufferMemory, ok := memory.(*TokenBufferMemory)
	if !ok {
		return ""
	}

	// Check for required role prefix
	if memoryConfig.RolePrefix.User == "" || memoryConfig.RolePrefix.Assistant == "" {
		// Log error but don't fail - return empty string
		return ""
	}

	// Calculate remaining tokens for memory
	restTokens := 4000

	// Get message limit
	messageLimit := -1 // No limit by default
	if memoryConfig.Window.Enabled && memoryConfig.Window.Size != 0 {
		messageLimit = memoryConfig.Window.Size
	}

	// Get history prompt text from memory
	historyText := tokenBufferMemory.GetHistoryPromptText(
		restTokens,
		messageLimit,
		memoryConfig.RolePrefix.User,
		memoryConfig.RolePrefix.Assistant,
	)

	return historyText
}

// handleMemoryForChatMode handles memory for chat mode
func (n *Node) handleMemoryForChatMode(
	memory *TokenBufferMemory,
	memoryConfig *MemoryConfig,
	modelConfig *ModelConfigWithCredentialsEntity,
) []PromptMessage {
	// Call the existing handleMemoryChatMode function
	logCtx := n.logContext(context.Background())
	if memory == nil {
		logger.DebugContext(logCtx, "LLM chat memory is nil")
		return []PromptMessage{}
	}

	if memoryConfig == nil {
		logger.DebugContext(logCtx, "LLM chat memory config missing, using defaults")
		memoryConfig = &MemoryConfig{
			RolePrefix: RolePrefix{
				User:      "Human",
				Assistant: "Assistant",
			},
			Window: WindowConfig{
				Enabled: true,
				Size:    10,
			},
		}
	}

	logger.DebugContext(logCtx, "LLM chat memory loaded",
		zap.Int("memory_messages_count", len(memory.Messages)),
	)

	// Calculate remaining tokens for memory
	restTokens := 4000

	// Get history prompt messages from memory
	messageLimit := -1 // No limit by default
	if memoryConfig.Window.Enabled && memoryConfig.Window.Size != 0 {
		messageLimit = memoryConfig.Window.Size
	}

	logger.DebugContext(logCtx, "LLM chat memory limits resolved",
		zap.Int("rest_tokens", restTokens),
		zap.Int("message_limit", messageLimit),
	)

	// Get history messages
	historyMessages := memory.GetHistoryPromptMessages(
		restTokens,
		messageLimit,
	)

	logger.DebugContext(logCtx, "LLM chat memory history selected",
		zap.Int("history_messages_count", len(historyMessages)),
	)

	return historyMessages
}

// handleMemoryForCompletionMode handles memory for completion mode
func (n *Node) handleMemoryForCompletionMode(
	memory *TokenBufferMemory,
	memoryConfig *MemoryConfig,
	modelConfig *ModelConfigWithCredentialsEntity,
) string {
	// Call the existing handleMemoryCompletionMode function
	return n.handleMemoryCompletionMode(memory, memoryConfig, modelConfig)
}

// saveMultimodalImageOutput saves multimodal image output
func (n *Node) saveMultimodalImageOutput(
	content *PromptMessageContent,
	fileSaver FileSaver,
) (*file.File, error) {
	if content.URL != "" {
		// Save from URL
		return fileSaver.SaveRemoteURL(content.URL, file.FileTypeImage)
	} else if content.Base64 != "" {
		// Save from base64 data
		data, err := base64.StdEncoding.DecodeString(content.Base64)
		if err != nil {
			return nil, fmt.Errorf("failed to decode base64 data: %v", err)
		}
		return fileSaver.SaveBinaryString(data, content.MimeType, file.FileTypeImage, nil)
	}
	return nil, fmt.Errorf("no valid image data found")
}

// imageFileToMarkdown converts image file to markdown
func (n *Node) imageFileToMarkdown(f *file.File) string {
	// Generate proper file URL based on file configuration
	// In production, this would use a configured base URL and proper routing
	var url string
	if *f.URL != "" {
		// Use the file's existing URL if available
		url = *f.URL
	} else {
		// Generate URL based on file ID and tenant
		url = fmt.Sprintf("/api/v1/files/%s", *f.ID)
		if n.TenantID != "" {
			url = fmt.Sprintf("/api/v1/tenants/%s/files/%s", n.TenantID, *f.ID)
		}
	}

	// Return markdown formatted image with proper alt text
	altText := f.Filename
	if *altText == "" {
		*altText = "Generated Image"
	}
	return fmt.Sprintf("![%s](%s)", *altText, url)
}

// saveMultimodalOutputAndConvertResultToMarkdown processes multimodal content
func (n *Node) saveMultimodalOutputAndConvertResultToMarkdown(
	contents any,
	fileSaver FileSaver,
	fileOutputs []*file.File,
) []string {
	var results []string

	if contents == nil {
		return results
	}

	switch c := contents.(type) {
	case string:
		results = append(results, c)
	case []any:
		for _, item := range c {
			if contentItem, ok := item.(*PromptMessageContent); ok {
				if contentItem.Type == PromptMessageContentTypeText {
					results = append(results, contentItem.Data)
				} else if contentItem.Type == PromptMessageContentTypeImage {
					// Save image and convert to markdown
					if savedFile, err := n.saveMultimodalImageOutput(contentItem, fileSaver); err == nil {
						fileOutputs = append(fileOutputs, savedFile)
						results = append(results, n.imageFileToMarkdown(savedFile))
					}
				} else {
					// For other types, convert to string
					results = append(results, fmt.Sprintf("%v", item))
				}
			} else {
				results = append(results, fmt.Sprintf("%v", item))
			}
		}
	default:
		results = append(results, fmt.Sprintf("%v", contents))
	}

	return results
}

// fetchStructuredOutputSchema fetches structured output schema
func (n *Node) fetchStructuredOutputSchema(structuredOutput map[string]any) (map[string]any, error) {
	if structuredOutput == nil || len(structuredOutput) == 0 {
		return nil, fmt.Errorf("please provide a valid structured output schema")
	}

	schemaData, ok := structuredOutput["schema"]
	if !ok {
		return nil, fmt.Errorf("please provide a valid structured output schema")
	}

	// Convert to JSON and back to validate structure
	jsonBytes, err := json.Marshal(schemaData)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal schema: %v", err)
	}

	var schema map[string]any
	err = json.Unmarshal(jsonBytes, &schema)
	if err != nil {
		return nil, fmt.Errorf("structured_output_schema is not valid JSON format: %v", err)
	}

	return schema, nil
}

// convertToOriginalRetrieverResource converts context dict to retriever resource
func (n *Node) convertToOriginalRetrieverResource(contextDict map[string]any) *shared.RetrievalSourceMetadata {
	metadata, ok := contextDict["metadata"].(map[string]any)
	if !ok {
		return nil
	}

	source, ok := metadata["_source"]
	if !ok || source != "knowledge" {
		return nil
	}

	// Helper function to safely convert to pointer types
	toIntPtr := func(v any) *int {
		if v == nil {
			return nil
		}
		if i, ok := v.(int); ok {
			return &i
		}
		if f, ok := v.(float64); ok {
			i := int(f)
			return &i
		}
		return nil
	}

	toStringPtr := func(v any) *string {
		if v == nil {
			return nil
		}
		if s, ok := v.(string); ok {
			return &s
		}
		return nil
	}

	toFloat64Ptr := func(v any) *float64 {
		if v == nil {
			return nil
		}
		if f, ok := v.(float64); ok {
			return &f
		}
		if i, ok := v.(int); ok {
			f := float64(i)
			return &f
		}
		return nil
	}

	// Build retrieval source metadata
	sourceMetadata := &shared.RetrievalSourceMetadata{
		Position:        toIntPtr(metadata["position"]),
		DatasetID:       toStringPtr(metadata["dataset_id"]),
		DatasetName:     toStringPtr(metadata["dataset_name"]),
		DocumentID:      toStringPtr(metadata["document_id"]),
		DocumentName:    toStringPtr(metadata["document_name"]),
		DataSourceType:  toStringPtr(metadata["data_source_type"]),
		SegmentID:       toStringPtr(metadata["segment_id"]),
		RetrieverFrom:   toStringPtr(metadata["retriever_from"]),
		Score:           toFloat64Ptr(metadata["score"]),
		HitCount:        toIntPtr(metadata["segment_hit_count"]),
		WordCount:       toIntPtr(metadata["segment_word_count"]),
		SegmentPosition: toIntPtr(metadata["segment_position"]),
		IndexNodeHash:   toStringPtr(metadata["segment_index_node_hash"]),
		Content:         toStringPtr(contextDict["content"]),
		Page:            toIntPtr(metadata["page"]),
		Title:           toStringPtr(metadata["title"]),
	}

	// Handle doc_metadata
	if docMeta, ok := metadata["doc_metadata"].(map[string]any); ok {
		sourceMetadata.DocMetadata = docMeta
	}

	return sourceMetadata
}

// extractVariableSelectorToVariableMapping extracts variable mappings from node config
func (n *Node) extractVariableSelectorToVariableMapping(
	graphConfig map[string]any,
	nodeID string,
	nodeData map[string]any,
) (map[string][]string, error) {
	// Parse node data
	typedNodeData, _, err := parseLLMNodeDataFromConfig(map[string]any{"data": nodeData})
	if err != nil {
		return nil, err
	}

	variableMapping := make(map[string][]string)
	promptTemplate := typedNodeData.PromptTemplate

	// Extract variables from prompt template
	var variableSelectors []VariableSelector
	switch pt := promptTemplate.(type) {
	case []NodeChatModelMessage:
		for _, prompt := range pt {
			if prompt.EditionType != "template" {
				parser := NewVariableTemplateParser(prompt.Text)
				variableSelectors = append(variableSelectors, parser.ExtractVariableSelectors()...)
			}
		}
	case NodeCompletionModelPromptTemplate:
		if pt.EditionType != "template" {
			parser := NewVariableTemplateParser(pt.Text)
			variableSelectors = parser.ExtractVariableSelectors()
		}
	}

	// Build variable mapping
	for _, selector := range variableSelectors {
		variableMapping[selector.Variable] = selector.ValueSelector
	}

	// Add memory variables if present
	if typedNodeData.Memory != nil && typedNodeData.Memory.QueryPromptTemplate != "" {
		parser := NewVariableTemplateParser(typedNodeData.Memory.QueryPromptTemplate)
		querySelectors := parser.ExtractVariableSelectors()
		for _, selector := range querySelectors {
			variableMapping[selector.Variable] = selector.ValueSelector
		}
	}

	// Add files variable
	if typedNodeData.Vision.Enabled {
		variableMapping["#files#"] = typedNodeData.Vision.Configs.VariableSelector
	}

	// Add system query
	if typedNodeData.Memory != nil {
		variableMapping["#sys.query#"] = []string{"sys", "query"}
	}

	variableMapping["#sys.query#"] = []string{"sys", "query"}
	variableMapping["#sys.conversation_id#"] = []string{"sys", "conversation_id"}
	variableMapping["#sys.dialogue_count#"] = []string{"sys", "dialogue_count"}
	variableMapping["#sys.workflow_run_id#"] = []string{"sys", "workflow_run_id"}

	// Add template variables
	if typedNodeData.PromptConfig.TemplateVariables != nil {
		for _, selector := range typedNodeData.PromptConfig.TemplateVariables {
			variableMapping[selector.Variable] = selector.ValueSelector
		}
	}

	// Prefix with node ID
	prefixedMapping := make(map[string][]string)
	for key, value := range variableMapping {
		prefixedMapping[nodeID+"."+key] = value
	}

	return prefixedMapping, nil
}

// getDefaultConfig returns default configuration for LLM node
func (n *Node) getDefaultConfig(filters map[string]any) map[string]any {
	return map[string]any{
		"type": "llm",
		"config": map[string]any{
			"prompt_templates": map[string]any{
				"chat_model": map[string]any{
					"prompts": []map[string]any{
						{
							"role":         "system",
							"text":         renderDefaultChatSystemPrompt(),
							"edition_type": "basic",
						},
					},
				},
				"completion_model": map[string]any{
					"conversation_histories_role": map[string]string{
						"user_prefix":      "Human",
						"assistant_prefix": "Assistant",
					},
					"prompt": map[string]any{
						"text":         renderDefaultCompletionPrompt(),
						"edition_type": "basic",
					},
					"stop": []string{"Human:"},
				},
			},
		},
	}
}

// Properties for error handling
func (n *Node) ContinueOnError() bool {
	return n.nodeData.ErrorStrategy != ""
}

func (n *Node) Retry() bool {
	return n.nodeData.RetryConfig.Enable
}

// parseLLMNodeDataFromConfig parses LLM node data from configuration
func parseLLMNodeDataFromConfig(config map[string]any) (NodeData, string, error) {
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

	// Convert to JSON and back to properly parse the structure
	jsonBytes, err := json.Marshal(data)
	if err != nil {
		return NodeData{}, "", err
	}

	var nodeData NodeData
	err = json.Unmarshal(jsonBytes, &nodeData)
	if err != nil {
		return NodeData{}, "", err
	}

	// Set defaults if not provided
	if nodeData.PromptConfig.TemplateVariables == nil {
		nodeData.PromptConfig = NewPromptConfig()
	}
	if !nodeData.Vision.Enabled {
		nodeData.Vision = NewVisionConfig()
	}

	return nodeData, nodeIDStr, nil
}

// convertInterfaceArrayToChatMessages converts []interface{} to []NodeChatModelMessage
func (n *Node) convertInterfaceArrayToChatMessages(interfaceArray []interface{}) ([]NodeChatModelMessage, error) {
	chatMessages := make([]NodeChatModelMessage, 0, len(interfaceArray))

	for i, item := range interfaceArray {
		// Convert interface{} to map[string]interface{}
		itemMap, ok := item.(map[string]interface{})
		if !ok {
			return nil, fmt.Errorf("item at index %d is not a map, got %T", i, item)
		}

		// Extract fields
		role, ok := itemMap["role"].(string)
		if !ok {
			return nil, fmt.Errorf("item at index %d missing or invalid 'role' field", i)
		}

		text, ok := itemMap["text"].(string)
		if !ok {
			return nil, fmt.Errorf("item at index %d missing or invalid 'text' field", i)
		}

		// Create NodeChatModelMessage
		message := NodeChatModelMessage{
			Role: PromptMessageRole(role),
			Text: text,
		}

		if id, exists := itemMap["id"]; exists && id != nil {
			if idStr, ok := id.(string); ok {
				message.ID = idStr
			}
		}

		if groupID, exists := itemMap["group_id"]; exists && groupID != nil {
			if groupIDStr, ok := groupID.(string); ok {
				message.GroupID = groupIDStr
			}
		}

		if groupKind, exists := itemMap["group_kind"]; exists && groupKind != nil {
			if groupKindStr, ok := groupKind.(string); ok {
				message.GroupKind = PromptGroupKind(groupKindStr)
			}
		}

		// Handle optional fields
		if templateText, exists := itemMap["template_text"]; exists && templateText != nil {
			if templateTextStr, ok := templateText.(string); ok {
				message.TemplateText = &templateTextStr
			}
		}

		if editionType, exists := itemMap["edition_type"]; exists && editionType != nil {
			if editionTypeStr, ok := editionType.(string); ok {
				message.EditionType = editionTypeStr
			}
		}

		chatMessages = append(chatMessages, message)
	}

	return chatMessages, nil
}

// fetchPromptMessages builds prompt messages for LLM execution following the design document structure
func (n *Node) fetchPromptMessages(
	sysQuery string,
	sysFiles []any,
	contextText string,
	memory *TokenBufferMemory,
	modelConfig *ModelConfigWithCredentialsEntity,
	promptTemplate any, // []NodeChatModelMessage or NodeCompletionModelPromptTemplate
	memoryConfig *MemoryConfig,
	visionEnabled bool,
	visionDetail ImagePromptMessageContentDetail,
	variablePool *entities.VariablePool,
	templateVariables []VariableSelector,
) ([]PromptMessage, []string, bool, error) {
	promptMessages := make([]PromptMessage, 0, 20)
	autoInjectedUserPrompt := false
	var err error
	logCtx := n.logContext(context.Background())

	// 1. According to the template type selection processing path
	switch pt := promptTemplate.(type) {
	case []NodeChatModelMessage:
		if n.nodeData.PromptLayout != nil {
			promptMessages, err = n.fetchChatPromptMessagesWithLayout(
				pt,
				sysQuery,
				contextText,
				memory,
				modelConfig,
				memoryConfig,
				variablePool,
				templateVariables,
				visionDetail,
			)
			if err != nil {
				return nil, nil, false, err
			}
			break
		}

		// Chat mode processing
		// Split template messages into system and user parts for correct ordering:
		// system messages -> history messages -> user messages
		var systemMessages []NodeChatModelMessage
		var userMessages []NodeChatModelMessage
		for _, msg := range pt {
			if msg.Role == PromptMessageRoleSystem {
				systemMessages = append(systemMessages, msg)
			} else {
				userMessages = append(userMessages, msg)
			}
		}

		// 1. Process system messages first
		if len(systemMessages) > 0 {
			sysPromptMessages, err := n.handleChatModelTemplate(
				systemMessages,
				contextText,
				templateVariables,
				variablePool,
				visionDetail,
			)
			if err != nil {
				return nil, nil, false, fmt.Errorf("failed to handle system template: %v", err)
			}
			promptMessages = append(promptMessages, sysPromptMessages...)
		}

		// 2. Add memory/history messages
		memoryMessages := n.handleMemoryForChatMode(
			memory,
			memoryConfig,
			modelConfig,
		)
		logger.DebugContext(logCtx, "LLM memory messages fetched",
			zap.Int("memory_messages_count", len(memoryMessages)),
		)
		promptMessages = append(promptMessages, memoryMessages...)

		// 3. Process user messages from template (contains {{#sys.query#}})
		if len(userMessages) > 0 {
			userPromptMessages, err := n.handleChatModelTemplate(
				userMessages,
				contextText,
				templateVariables,
				variablePool,
				visionDetail,
			)
			if err != nil {
				return nil, nil, false, fmt.Errorf("failed to handle user template: %v", err)
			}
			promptMessages = append(promptMessages, userPromptMessages...)
			logger.DebugContext(logCtx, "LLM user prompt messages added from template",
				zap.Int("user_prompt_messages_count", len(userPromptMessages)),
			)
		} else {
			// No user messages in template, add current query directly
			queryMessages, err := n.addCurrentQueryForChat(
				sysQuery,
				variablePool,
				visionDetail,
			)
			if err != nil {
				return nil, nil, false, fmt.Errorf("failed to add current query for chat: %v", err)
			}
			logger.DebugContext(logCtx, "LLM current query added to prompt",
				zap.Int("query_length", len(sysQuery)),
			)
			promptMessages = append(promptMessages, queryMessages...)
		}

	case []interface{}:
		// Handle JSON unmarshaled interface{} array - convert to []NodeChatModelMessage
		chatMessages, err := n.convertInterfaceArrayToChatMessages(pt)
		if err != nil {
			return nil, nil, false, fmt.Errorf("failed to convert interface array to chat messages: %v", err)
		}
		if n.nodeData.PromptLayout != nil {
			promptMessages, err = n.fetchChatPromptMessagesWithLayout(
				chatMessages,
				sysQuery,
				contextText,
				memory,
				modelConfig,
				memoryConfig,
				variablePool,
				templateVariables,
				visionDetail,
			)
			if err != nil {
				return nil, nil, false, err
			}
			break
		}

		// Split template messages into system and user parts for correct ordering:
		// system messages -> history messages -> user messages
		var systemMessages []NodeChatModelMessage
		var userMessages []NodeChatModelMessage
		for _, msg := range chatMessages {
			if msg.Role == PromptMessageRoleSystem {
				systemMessages = append(systemMessages, msg)
			} else {
				userMessages = append(userMessages, msg)
			}
		}

		// 1. Process system messages first
		if len(systemMessages) > 0 {
			sysPromptMessages, err := n.handleChatModelTemplate(
				systemMessages,
				contextText,
				templateVariables,
				variablePool,
				visionDetail,
			)
			if err != nil {
				return nil, nil, false, fmt.Errorf("failed to handle system template: %v", err)
			}
			promptMessages = append(promptMessages, sysPromptMessages...)
		}

		// 2. Add memory/history messages
		memoryMessages := n.handleMemoryForChatMode(
			memory,
			memoryConfig,
			modelConfig,
		)
		logger.DebugContext(logCtx, "LLM memory messages fetched from interface prompt template",
			zap.Int("memory_messages_count", len(memoryMessages)),
		)
		promptMessages = append(promptMessages, memoryMessages...)

		// 3. Process user messages from template (contains {{#sys.query#}})
		if len(userMessages) > 0 {
			userPromptMessages, err := n.handleChatModelTemplate(
				userMessages,
				contextText,
				templateVariables,
				variablePool,
				visionDetail,
			)
			if err != nil {
				return nil, nil, false, fmt.Errorf("failed to handle user template: %v", err)
			}
			promptMessages = append(promptMessages, userPromptMessages...)
			logger.DebugContext(logCtx, "LLM user prompt messages added from interface template",
				zap.Int("user_prompt_messages_count", len(userPromptMessages)),
			)
		} else {
			// No user messages in template, add current query directly
			queryMessages, err := n.addCurrentQueryForChat(
				sysQuery,
				variablePool,
				visionDetail,
			)
			if err != nil {
				return nil, nil, false, fmt.Errorf("failed to add current query for chat: %v", err)
			}
			logger.DebugContext(logCtx, "LLM current query added from interface prompt template",
				zap.Int("query_length", len(sysQuery)),
			)
			promptMessages = append(promptMessages, queryMessages...)
		}

	case NodeCompletionModelPromptTemplate:
		// Completion mode processing
		messages, err := n.handleCompletionModelTemplate(
			pt,
			contextText,
			templateVariables,
			variablePool,
		)
		if err != nil {
			return nil, nil, false, fmt.Errorf("failed to handle completion model template: %v", err)
		}
		promptMessages = append(promptMessages, messages...)

		// Integrate memory history text
		memoryText := n.handleMemoryForCompletionMode(
			memory,
			memoryConfig,
			modelConfig,
		)

		// Insert history record into prompt
		if err = n.insertHistoryIntoPrompt(promptMessages, memoryText); err != nil {
			return nil, nil, false, fmt.Errorf("failed to insert history into prompt: %v", err)
		}

		// Add current query
		if err = n.addCurrentQueryForCompletion(sysQuery, promptMessages); err != nil {
			return nil, nil, false, fmt.Errorf("failed to add current query for completion: %v", err)
		}

	default:
		return nil, nil, false, fmt.Errorf("unsupported prompt template type: %T", promptTemplate)
	}

	// 2. Process vision files
	promptMessages, autoInjectedUserPrompt, err = n.processVisionFiles(
		promptMessages,
		sysFiles,
		visionEnabled,
		visionDetail,
	)
	if err != nil {
		return promptMessages, nil, autoInjectedUserPrompt, fmt.Errorf("failed to process vision files: %v", err)
	}

	// 3. Filter invalid messages and unsupported content
	filteredMessages := n.filterInvalidMessages(
		promptMessages,
		modelConfig,
	)

	// 4. Get stop sequences
	stopSequences := n.getStopSequences(modelConfig)

	return filteredMessages, stopSequences, autoInjectedUserPrompt, nil
}

// addCurrentQueryForChat adds current query for chat mode
func (n *Node) addCurrentQueryForChat(
	sysQuery string,
	variablePool *entities.VariablePool,
	visionDetail ImagePromptMessageContentDetail,
) ([]PromptMessage, error) {
	if sysQuery == "" {
		return []PromptMessage{}, nil
	}

	message := NodeChatModelMessage{
		Text:        sysQuery,
		Role:        PromptMessageRoleUser,
		EditionType: "basic",
	}

	// Reuse chat mode template processing logic
	return n.handleChatModelTemplate(
		[]NodeChatModelMessage{message},
		"",                   // context
		[]VariableSelector{}, // templateVariables
		variablePool,
		visionDetail,
	)
}

// addCurrentQueryForCompletion adds current query for completion mode
func (n *Node) addCurrentQueryForCompletion(sysQuery string, promptMessages []PromptMessage) error {
	if sysQuery == "" || len(promptMessages) == 0 {
		return nil
	}

	// Process string content type
	if promptContent, ok := promptMessages[0].Content.(string); ok {
		promptContent = replaceBareTemplateToken(promptContent, "#sys.query#", sysQuery)
		promptMessages[0].Content = promptContent
		return nil
	}

	// Process list content type (multimodal)
	if contentList, ok := promptMessages[0].Content.([]PromptMessageContent); ok {
		for i := range contentList {
			if contentList[i].Type == PromptMessageContentTypeText {
				contentList[i].Data = sysQuery + "\n" + contentList[i].Data
			}
		}
		return nil
	}

	return fmt.Errorf("unsupported prompt content type")
}

// insertHistoryIntoPrompt inserts history into prompt for completion mode
func (n *Node) insertHistoryIntoPrompt(
	promptMessages []PromptMessage,
	memoryText string,
) error {
	if len(promptMessages) == 0 {
		return nil
	}

	// Process string content type
	if promptContent, ok := promptMessages[0].Content.(string); ok {
		if strings.Contains(promptContent, "{{#histories#}}") {
			promptContent = strings.ReplaceAll(promptContent, "{{#histories#}}", memoryText)
		} else if strings.Contains(promptContent, "#histories#") {
			promptContent = strings.ReplaceAll(promptContent, "#histories#", memoryText)
		} else {
			promptContent = memoryText + "\n" + promptContent
		}
		promptMessages[0].Content = promptContent
		return nil
	}

	// Process list content type (multimodal)
	if contentList, ok := promptMessages[0].Content.([]PromptMessageContent); ok {
		for i := range contentList {
			if contentList[i].Type == PromptMessageContentTypeText {
				if strings.Contains(contentList[i].Data, "{{#histories#}}") {
					contentList[i].Data = strings.ReplaceAll(contentList[i].Data, "{{#histories#}}", memoryText)
				} else if strings.Contains(contentList[i].Data, "#histories#") {
					contentList[i].Data = strings.ReplaceAll(contentList[i].Data, "#histories#", memoryText)
				} else {
					contentList[i].Data = memoryText + "\n" + contentList[i].Data
				}
			}
		}
		return nil
	}

	return fmt.Errorf("unsupported prompt content type")
}

// getFileContentType determines the content type based on file properties
func (n *Node) getFileContentType(f *file.File) PromptMessageContentType {
	if f == nil {
		return PromptMessageContentTypeText
	}

	if mediaType, ok := n.getMediaContentTypeFromMimeType(safeDeref(f.MimeType)); ok {
		return mediaType
	}

	switch f.Type {
	case file.FileTypeImage:
		return PromptMessageContentTypeImage
	case file.FileTypeAudio:
		return PromptMessageContentTypeAudio
	case file.FileTypeVideo:
		return PromptMessageContentTypeVideo
	case file.FileTypeDocument:
		return PromptMessageContentTypeDocument
	default:
		// Try to determine from MIME type
		return n.getContentTypeFromMimeType(*f.MimeType)
	}
}

// getFileContentTypeFromMap determines content type from a file map representation
func (n *Node) getFileContentTypeFromMap(fileMap map[string]any) PromptMessageContentType {
	if kind, exists := fileMap["kind"]; exists {
		if kindStr, ok := kind.(string); ok {
			switch kindStr {
			case "image":
				return PromptMessageContentTypeImage
			case "audio":
				return PromptMessageContentTypeAudio
			case "video":
				return PromptMessageContentTypeVideo
			case "document":
				return PromptMessageContentTypeDocument
			}
		}
	}

	// Try MIME type
	if mimeType, exists := fileMap["mime_type"]; exists {
		if mimeTypeStr, ok := mimeType.(string); ok {
			return n.getContentTypeFromMimeType(mimeTypeStr)
		}
	}

	return PromptMessageContentTypeText
}

func (n *Node) getMediaContentTypeFromMimeType(mimeType string) (PromptMessageContentType, bool) {
	contentType := n.getContentTypeFromMimeType(mimeType)
	switch contentType {
	case PromptMessageContentTypeImage, PromptMessageContentTypeAudio, PromptMessageContentTypeVideo:
		return contentType, true
	default:
		return PromptMessageContentTypeText, false
	}
}

// getContentTypeFromMimeType determines content type from MIME type
func (n *Node) getContentTypeFromMimeType(mimeType string) PromptMessageContentType {
	if mimeType == "" {
		return PromptMessageContentTypeText
	}

	mimeType = strings.ToLower(mimeType)
	switch {
	case strings.HasPrefix(mimeType, "image/"):
		return PromptMessageContentTypeImage
	case strings.HasPrefix(mimeType, "audio/"):
		return PromptMessageContentTypeAudio
	case strings.HasPrefix(mimeType, "video/"):
		return PromptMessageContentTypeVideo
	case strings.HasPrefix(mimeType, "text/") || strings.Contains(mimeType, "pdf"):
		return PromptMessageContentTypeDocument
	default:
		return PromptMessageContentTypeText
	}
}

// getStringFromMap safely extracts a string from a map
func getStringFromMap(m map[string]any, key string) string {
	if val, exists := m[key]; exists {
		if str, ok := val.(string); ok {
			return str
		}
		rv := reflect.ValueOf(val)
		if rv.IsValid() && rv.Kind() == reflect.String {
			return rv.String()
		}
	}
	return ""
}

// filterInvalidMessages filters invalid messages and unsupported content
func (n *Node) filterInvalidMessages(
	promptMessages []PromptMessage,
	modelConfig *ModelConfigWithCredentialsEntity,
) []PromptMessage {
	filteredMessages := make([]PromptMessage, 0, len(promptMessages))

	for _, message := range promptMessages {
		// Process list content type
		if contentList, ok := message.Content.([]PromptMessageContent); ok {
			filteredContent := make([]PromptMessageContent, 0, len(contentList))
			for _, content := range contentList {
				// Filter content based on model features
				if n.isContentSupportedByModel(content, modelConfig) {
					filteredContent = append(filteredContent, content)
				}
			}

			// If there's only one text content, convert it to string
			if len(filteredContent) == 1 && filteredContent[0].Type == PromptMessageContentTypeText {
				message.Content = filteredContent[0].Data
			} else {
				message.Content = filteredContent
			}
		}

		// Check if message is empty
		if !n.isMessageEmpty(message) {
			filteredMessages = append(filteredMessages, message)
		}
	}

	return filteredMessages
}

// isContentSupportedByModel checks if content type is supported by the model
func (n *Node) isContentSupportedByModel(content PromptMessageContent, modelConfig *ModelConfigWithCredentialsEntity) bool {
	// Always support text content
	if content.Type == PromptMessageContentTypeText {
		return true
	}

	// Check model schema for supported features
	// ModelSchema is now a struct, not an interface
	// For now, we'll use provider-specific logic since ModelSchema is a struct
	// TODO: Add proper feature detection to ModelSchema struct

	// Provider-specific defaults
	switch strings.ToLower(strings.TrimSpace(modelConfig.Provider)) {
	case "openai":
		// OpenAI GPT-4 Vision and newer models support images
		return content.Type == PromptMessageContentTypeImage
	case "anthropic":
		// Claude models support images
		return content.Type == PromptMessageContentTypeImage
	case "google":
		// Gemini models support multimodal content
		return content.Type == PromptMessageContentTypeImage
	case "qwen":
		// Qwen VL models support images.
		return content.Type == PromptMessageContentTypeImage
	case "glm", "zhipu", "bigmodel":
		// GLM vision models accept image inputs through the chat interface.
		return content.Type == PromptMessageContentTypeImage
	case "doubao", "volcengine":
		// Doubao/Volcengine vision models support image inputs.
		return content.Type == PromptMessageContentTypeImage
	case "moonshot":
		// Moonshot/Kimi vision models support image inputs.
		return content.Type == PromptMessageContentTypeImage
	default:
		// Conservative default: only support text
		return false
	}
}

// invokeLLMWithResult invokes the LLM and returns the result directly
func (n *Node) invokeLLMWithResult(
	ctx context.Context,
	req *LLMInvokeRequest,
	fileSaver FileSaver,
	fileOutputs *[]*file.File,
	eventChan chan *shared.NodeEventCh,
) (*ModelInvokeCompletedEventData, error) {
	if n.invoker == nil {
		return nil, fmt.Errorf("llm invoker is not initialized")
	}
	if req == nil {
		return nil, fmt.Errorf("llm invoke request is nil")
	}
	n.fillObservabilityContext(req)

	invokeStartedAt := time.Now()
	logCtx := logger.WithFields(n.logContext(ctx),
		zap.String("provider", req.ProviderSlug),
		zap.String("model", req.ModelSlug),
	)
	logger.InfoContext(logCtx, "workflow llm llm_request_sent",
		zap.Int("prompt_messages_count", len(req.Messages)),
		zap.Int("prompt_text_length", promptMessagesTextLength(req.Messages)),
	)

	resultChan, errChan, err := n.invoker.InvokeStream(ctx, n.UserID, n.APPID, AppType, req)
	if err != nil {
		return nil, fmt.Errorf("failed to invoke LLM: %w", err)
	}

	// Handle invoke result stream and collect the result
	var model string
	var promptMessagesResult []PromptMessage
	usage := &shared.LLMUsage{}
	var finishReason string
	fullTextBuffer := strings.Builder{}
	var structuredOutputResult any
	var firstChunkAt time.Time

	// Process streaming chunks
	for result := range resultChan {
		if result == nil {
			continue
		}
		if firstChunkAt.IsZero() {
			firstChunkAt = time.Now()
			logger.InfoContext(logCtx, "workflow llm first_chunk_received",
				zap.Int64("first_chunk_latency_ms", firstChunkAt.Sub(invokeStartedAt).Milliseconds()),
			)
		}

		// Handle structured output if present
		if result.StructuredOutput != nil {
			// Save structured output result
			structuredOutputResult = result.StructuredOutput

			// Send structured output event
			select {
			case eventChan <- &shared.NodeEventCh{
				Type:      shared.EventTypeLLMStructuredOutput,
				NodeID:    n.NodeID,
				Data:      result.StructuredOutput,
				Timestamp: time.Now(),
			}:
			default:
				// Channel full, continue
			}
			continue
		}

		// Process text content from message
		if result.Delta != nil && result.Delta.Message != nil {
			contents := result.Delta.Message.Content

			// Save multimodal output and convert to markdown
			textResults := n.saveMultimodalOutputAndConvertResultToMarkdown(
				contents, fileSaver, *fileOutputs,
			)
			for _, textPart := range textResults {
				fullTextBuffer.WriteString(textPart)

				// Send stream chunk event (blocking to ensure no data loss)
				eventChan <- &shared.NodeEventCh{
					Type:   shared.EventTypeRunStreamChunk,
					NodeID: n.NodeID,
					Data: &shared.RunStreamChunkEvent{
						ChunkContent:         textPart,
						FromVariableSelector: []string{n.NodeID, "text"},
					},
					Timestamp: time.Now(),
				}
			}
		}

		// Update metadata
		if model == "" && result.Model != "" {
			model = result.Model
		}
		if len(promptMessagesResult) == 0 && len(result.PromptMessages) > 0 {
			promptMessagesResult = result.PromptMessages
		}
		if result.Delta != nil && result.Delta.Usage != nil && usage.PromptTokens == 0 {
			usage = result.Delta.Usage
		}
		if finishReason == "" && result.Delta != nil && result.Delta.FinishReason != "" {
			finishReason = result.Delta.FinishReason
		}
	}

	// Check for invocation error
	var invokeErr error
	if errChan != nil {
		if errVal, ok := <-errChan; ok {
			invokeErr = errVal
		}
	}
	if invokeErr != nil {
		return nil, fmt.Errorf("failed to invoke LLM stream: %w", invokeErr)
	}
	doneAt := time.Now()
	logger.InfoContext(logCtx, "workflow llm llm_done",
		zap.Bool("first_chunk_received", !firstChunkAt.IsZero()),
		zap.Int("result_text_length", fullTextBuffer.Len()),
		zap.Int64("total_latency_ms", doneAt.Sub(invokeStartedAt).Milliseconds()),
	)

	// Send final completion event
	select {
	case eventChan <- &shared.NodeEventCh{
		Type:   shared.EventTypeModelInvokeCompleted,
		NodeID: n.NodeID,
		Data: &shared.ModelInvokeCompletedEvent{
			Text:         fullTextBuffer.String(),
			Usage:        usage,
			FinishReason: finishReason,
		},
		Timestamp: time.Now(),
	}:
	default:
		// Channel full, continue
	}

	// Return the result
	return &ModelInvokeCompletedEventData{
		Text:             fullTextBuffer.String(),
		Usage:            usage,
		FinishReason:     &finishReason,
		StructuredOutput: structuredOutputResult,
	}, nil
}

func (n *Node) fillObservabilityContext(req *LLMInvokeRequest) {
	if n == nil || req == nil {
		return
	}
	if req.WorkflowID == "" {
		req.WorkflowID = n.WorkflowID
	}
	if req.WorkflowRunID == "" {
		req.WorkflowRunID = n.getWorkflowRunID()
	}
	if req.NodeID == "" {
		req.NodeID = n.NodeID
	}
	if req.NodeType == "" {
		req.NodeType = string(n.NodeType)
	}
	if n.GraphRuntimeState == nil || n.GraphRuntimeState.VariablePool == nil {
		return
	}
	if req.ConversationID == "" {
		if conversationVar := n.GraphRuntimeState.VariablePool.Get([]string{"sys", "conversation_id"}); conversationVar != nil {
			if conversationID, ok := conversationVar.ToObject().(string); ok {
				req.ConversationID = conversationID
			}
		}
	}
	if req.SessionID == "" {
		req.SessionID = req.ConversationID
	}
}

// getStopSequences gets stop sequences from model config
func (n *Node) getStopSequences(modelConfig *ModelConfigWithCredentialsEntity) []string {
	if modelConfig.Stop == nil {
		return []string{}
	}
	return modelConfig.Stop
}

// isMessageEmpty checks if a prompt message is empty
func (n *Node) isMessageEmpty(message PromptMessage) bool {
	switch content := message.Content.(type) {
	case string:
		return strings.TrimSpace(content) == ""
	case []PromptMessageContent:
		return len(content) == 0
	default:
		return false
	}
}

// extractFileContentFromMap extracts file content from a file map object
func (n *Node) extractFileContentFromMap(fileMap map[string]interface{}, fileID string) string {
	filename := "unknown"
	if name, ok := fileMap["name"].(string); ok && name != "" {
		filename = name
	}

	fileType := "unknown"
	if ftype, ok := fileMap["type"].(string); ok && ftype != "" {
		fileType = ftype
	}

	// For video and audio files, return a note that content extraction is not supported
	if fileType == "video" || fileType == "audio" {
		return fmt.Sprintf("[File: %s (Type: %s, ID: %s)]\n\nNote: Video and audio files cannot be directly processed as text. Please provide a description of the content or upload a text-based file (TXT, MD, PDF, DOCX, etc.).",
			filename, fileType, fileID)
	}

	// Try to get file service and extract content
	fileRepo := file_repo.NewFileRepository(n.db)
	storageClient := storage.GetStorage()
	// Pass nil for quota services since we're only reading files here
	fileService := file_service.NewFileService(fileRepo, storageClient, n.db, nil, nil)

	// Try to get file preview
	ctx := context.Background()
	content, err := fileService.GetFilePreview(ctx, fileID)
	if err != nil {
		// If preview fails, try to get file metadata
		uploadFile, err := fileService.GetFileByID(ctx, fileID)
		if err != nil {
			return fmt.Sprintf("[File: %s (Type: %s, ID: %s)]\n\nError: Unable to access file - %v\n\nPlease ensure the file was uploaded successfully.",
				filename, fileType, fileID, err)
		}

		// Return file metadata with helpful message
		return fmt.Sprintf("[File: %s]\nType: %s\nSize: %d bytes\nExtension: %s\nMIME Type: %s\n\nNote: Content preview is not available for this file type. Supported text formats include: TXT, MD, HTML, CSV, XML.",
			uploadFile.Name, fileType, uploadFile.Size, uploadFile.Extension, uploadFile.MimeType)
	}

	// Return the file content
	if content == "" {
		return fmt.Sprintf("[File: %s (Type: %s)]\n\nNote: File content is empty or has not been processed yet. Please wait a moment and try again.", filename, fileType)
	}

	return content
}

// getTemplateInputKeys returns a list of template input keys for debugging
func getTemplateInputKeys(inputs map[string]interface{}) []string {
	keys := make([]string, 0, len(inputs))
	for k := range inputs {
		keys = append(keys, k)
	}
	return keys
}

// extractFileContentVariables extracts variable paths from {{#variable#}} syntax
// Returns a list of variable paths like ["1760721508950.f_ile", "node2.file2"]
func extractFileContentVariables(templateStr string) []string {
	// Use regex to find all {{#...#}} patterns
	re := regexp.MustCompile(`\{\{#([^#]+)#\}\}`)
	matches := re.FindAllStringSubmatch(templateStr, -1)

	vars := make([]string, 0, len(matches))
	for _, match := range matches {
		if len(match) > 1 {
			vars = append(vars, match[1])
		}
	}

	return vars
}

// getWorkflowRunID retrieves the workflow run ID from the variable pool for logging context
func (n *Node) getWorkflowRunID() string {
	if n.GraphRuntimeState == nil || n.GraphRuntimeState.VariablePool == nil {
		return "unknown"
	}

	runIDVar := n.GraphRuntimeState.VariablePool.Get([]string{"sys", "workflow_run_id"})
	if runIDVar == nil {
		return "unknown"
	}

	if runID, ok := runIDVar.ToObject().(string); ok && runID != "" {
		return runID
	}

	return "unknown"
}

// resolveModelConfig resolves model configuration with priority:
// 1. model_config from GraphRuntimeState.VariablePool.UserInputs - highest priority
// 2. Node default configuration
func (n *Node) resolveModelConfig(variablePool *entities.VariablePool) (*ModelConfig, string, error) {
	logCtx := n.logContext(context.Background())

	// Check for model_config in UserInputs from GraphRuntimeState
	// This matches how SQL Generator node retrieves model_config
	if n.GraphRuntimeState != nil && n.GraphRuntimeState.VariablePool != nil {
		userInputs := n.GraphRuntimeState.VariablePool.UserInputs
		if userInputs != nil {
			if modelConfigRaw, exists := userInputs["model_config"]; exists {
				logger.DebugContext(logCtx, "found LLM model config override in user inputs")

				overrideConfig, err := n.convertToModelConfig(modelConfigRaw)
				if err != nil {
					logger.ErrorContext(logCtx, "failed to convert LLM model config override", err)
					return nil, "", fmt.Errorf("failed to convert model config: %v", err)
				}

				logger.DebugContext(logCtx, "using LLM model config override",
					zap.String("provider", overrideConfig.Provider),
					zap.String("model", overrideConfig.Name),
				)

				return overrideConfig, shared.ResolvedModelSourceInputOverride, nil
			}
		}
	}

	// No override specified, use node default configuration
	logger.DebugContext(logCtx, "using default LLM model config",
		zap.String("provider", n.nodeData.Model.Provider),
		zap.String("model", n.nodeData.Model.Name),
	)

	return &n.nodeData.Model, shared.ResolvedModelSourceNodeDefault, nil
}

// convertToModelConfig converts variable value to ModelConfig
func (n *Node) convertToModelConfig(value any) (*ModelConfig, error) {
	logCtx := n.logContext(context.Background())

	configMap, ok := value.(map[string]interface{})
	if !ok {
		logger.ErrorContext(logCtx, "invalid LLM model config type",
			zap.String("config_type", fmt.Sprintf("%T", value)),
		)
		return nil, fmt.Errorf("model config must be an object, got %T", value)
	}

	// Extract required fields with detailed error messages
	provider, ok := configMap["provider"].(string)
	if !ok {
		if _, exists := configMap["provider"]; !exists {
			logger.ErrorContext(logCtx, "LLM model config missing provider",
				zap.Strings("available_fields", getMapKeys(configMap)),
			)
			return nil, fmt.Errorf("model config missing required field 'provider'")
		}
		logger.ErrorContext(logCtx, "LLM model config provider has invalid type",
			zap.String("provider_type", fmt.Sprintf("%T", configMap["provider"])),
		)
		return nil, fmt.Errorf("model config field 'provider' must be a non-empty string")
	}
	provider = strings.TrimSpace(provider)
	if provider == "" {
		logger.ErrorContext(logCtx, "LLM model config provider is empty")
		return nil, fmt.Errorf("model config field 'provider' cannot be empty")
	}

	// Try to get model name from either "name" or "model" field (for compatibility)
	var model string
	var modelOk bool

	// First try "name" field (used by frontend)
	model, modelOk = configMap["name"].(string)
	if !modelOk || model == "" {
		// Fallback to "model" field (legacy)
		model, modelOk = configMap["model"].(string)
		if !modelOk {
			if _, existsName := configMap["name"]; !existsName {
				if _, existsModel := configMap["model"]; !existsModel {
					logger.ErrorContext(logCtx, "LLM model config missing model name",
						zap.Strings("available_fields", getMapKeys(configMap)),
					)
					return nil, fmt.Errorf("model config missing required field 'name' or 'model'")
				}
			}
			logger.ErrorContext(logCtx, "LLM model config name has invalid type",
				zap.String("name_type", fmt.Sprintf("%T", configMap["name"])),
				zap.String("model_type", fmt.Sprintf("%T", configMap["model"])),
			)
			return nil, fmt.Errorf("model config field 'name' or 'model' must be a non-empty string")
		}
	}
	model = strings.TrimSpace(model)

	if model == "" {
		logger.ErrorContext(logCtx, "LLM model config name is empty")
		return nil, fmt.Errorf("model config field 'name' or 'model' cannot be empty")
	}

	// Extract optional mode field
	mode := ModeChat // default
	if modeStr, ok := configMap["mode"].(string); ok && modeStr != "" {
		mode = Mode(modeStr)
	}

	// Extract optional completion parameters
	completionParams := make(map[string]any)
	if params, ok := configMap["completion_params"].(map[string]interface{}); ok {
		completionParams = params
	}

	logger.DebugContext(logCtx, "LLM model config converted",
		zap.String("provider", provider),
		zap.String("model", model),
		zap.Any("mode", mode),
	)

	return &ModelConfig{
		Provider:         provider,
		Name:             model,
		Mode:             mode,
		CompletionParams: completionParams,
	}, nil
}

// getMapKeys returns the keys of a map for debugging purposes
func getMapKeys(m map[string]interface{}) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	return keys
}

// mergeModelConfigs merges default and override configurations
// Override parameters take precedence over defaults
func (n *Node) mergeModelConfigs(defaultConfig, overrideConfig *ModelConfig) *ModelConfig {
	merged := &ModelConfig{
		Provider: overrideConfig.Provider,
		Name:     overrideConfig.Name,
		Mode:     overrideConfig.Mode,
	}

	// Merge completion parameters
	// Start with default parameters
	merged.CompletionParams = make(map[string]any)
	if defaultConfig.CompletionParams != nil {
		for k, v := range defaultConfig.CompletionParams {
			merged.CompletionParams[k] = v
		}
	}

	// Override with runtime parameters
	if overrideConfig.CompletionParams != nil {
		for k, v := range overrideConfig.CompletionParams {
			merged.CompletionParams[k] = v
		}
	}

	return merged
}

// parseVariableSelector parses variable selector string to selector array
// Supports formats: "node_id.variable_name" or "variable_name"
func parseVariableSelector(varName string) []string {
	parts := strings.Split(varName, ".")
	return parts
}

// safeDeref safely dereferences a string pointer, returns empty string if nil
func safeDeref(s *string) string {
	if s == nil {
		return ""
	}
	return *s
}

// getFileURL returns the canonical URL for a file.
func (n *Node) getFileURL(f *file.File) (string, error) {
	if f == nil {
		return "", nil
	}
	logCtx := n.logContext(context.Background())

	logger.DebugContext(logCtx, "resolving LLM file URL",
		zap.Any("transfer_method", f.TransferMethod),
		zap.Any("file_type", f.Type),
		zap.String("file_id", safeDeref(f.ID)),
		zap.String("related_id", safeDeref(f.RelatedID)),
		zap.String("mime_type", safeDeref(f.MimeType)),
		zap.Bool("has_remote_url", safeDeref(f.RemoteURL) != ""),
	)

	if isLocalWorkflowFile(f.TransferMethod) {
		fileID := firstNonEmptyString(safeDeref(f.RelatedID), safeDeref(f.ID))
		if fileID != "" {
			result, err := n.resolveFileURLFromID(fileID)
			logger.DebugContext(logCtx, "resolved local LLM file URL",
				err,
				zap.String("file_id", fileID),
				zap.Bool("has_url", result != ""),
			)
			return result, err
		}
	}

	if generatedURL, err := f.GenerateURL(); err == nil && generatedURL != nil && *generatedURL != "" {
		logger.DebugContext(logCtx, "resolved LLM file URL from generator",
			zap.Bool("has_url", true),
		)
		return *generatedURL, nil
	}

	if f.URL != nil && *f.URL != "" {
		logger.DebugContext(logCtx, "using existing LLM file URL",
			zap.Bool("has_url", true),
		)
		return *f.URL, nil
	}
	if f.RemoteURL != nil && *f.RemoteURL != "" {
		logger.DebugContext(logCtx, "using remote LLM file URL",
			zap.Bool("has_remote_url", true),
		)
		return *f.RemoteURL, nil
	}
	if f.ID != nil && *f.ID != "" {
		result, err := n.resolveFileURLFromID(*f.ID)
		logger.DebugContext(logCtx, "resolved fallback LLM file URL",
			err,
			zap.String("file_id", *f.ID),
			zap.Bool("has_url", result != ""),
		)
		return result, err
	}

	logger.DebugContext(logCtx, "no LLM file URL resolved")
	return "", nil
}

// resolveFileURLFromMap resolves file URL from a map structure.
func (n *Node) resolveFileURLFromMap(f map[string]any) (string, error) {
	transferMethod := file.FileTransferMethod(getStringFromMap(f, "transfer_method"))
	if isLocalWorkflowFile(transferMethod) {
		fileID := firstNonEmptyString(
			getStringFromMap(f, "upload_file_id"),
			getStringFromMap(f, "id"),
			getStringFromMap(f, "related_id"),
		)
		if fileID == "" {
			return "", nil
		}
		return n.resolveFileURLFromID(fileID)
	}

	if rawURL := firstNonEmptyString(getStringFromMap(f, "url"), getStringFromMap(f, "remote_url")); rawURL != "" {
		return rawURL, nil
	}

	var fileID string
	if id, ok := f["upload_file_id"].(string); ok && id != "" {
		fileID = id
	} else if id, ok := f["id"].(string); ok && id != "" {
		fileID = id
	} else if id, ok := f["related_id"].(string); ok && id != "" {
		fileID = id
	}

	if fileID == "" {
		return "", nil
	}

	return n.resolveFileURLFromID(fileID)
}

// enrichFileFromDB enriches entities.File with missing info from database
func (n *Node) enrichFileFromDB(entityFile *entities.File) {
	if entityFile == nil || entityFile.ID == "" || n.db == nil {
		return
	}

	// Skip if StorageKey is already set
	if entityFile.StorageKey != "" {
		return
	}

	// Query upload_files table to get complete file info
	var result struct {
		Key       string `gorm:"column:key"`
		MimeType  string `gorm:"column:mime_type"`
		Extension string `gorm:"column:extension"`
		Filename  string `gorm:"column:name"`
		Size      int64  `gorm:"column:size"`
	}
	err := n.db.Table("upload_files").
		Select("key, mime_type, extension, name, size").
		Where("id = ?", entityFile.ID).
		First(&result).Error

	if err != nil {
		logger.WarnContext(n.logContext(context.Background()), "failed to query file info for LLM node",
			err,
			zap.String("file_id", entityFile.ID),
		)
		return
	}

	// Update entity file with database info
	entityFile.StorageKey = result.Key
	if entityFile.MimeType == "" {
		entityFile.MimeType = result.MimeType
	}
	if entityFile.Extension == "" {
		entityFile.Extension = result.Extension
	}
	if entityFile.Filename == "" {
		entityFile.Filename = result.Filename
	}
	if entityFile.Size == 0 {
		entityFile.Size = result.Size
	}

}

// resolveFileURLFromID returns the signed preview URL for a workflow file.
func (n *Node) resolveFileURLFromID(fileID string) (string, error) {
	if fileID == "" {
		return "", nil
	}

	signedURL, err := file.GetSignedFileURL(fileID)
	if err != nil {
		logger.WarnContext(n.logContext(context.Background()), "failed to generate signed file URL for LLM node",
			err,
			zap.String("file_id", fileID),
		)
		return "", err
	}

	logger.DebugContext(n.logContext(context.Background()), "signed file URL generated for LLM node",
		zap.String("file_id", fileID),
		zap.Bool("has_signed_url", signedURL != ""),
	)
	return signedURL, nil
}
