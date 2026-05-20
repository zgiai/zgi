package start

import (
	"context"
	"fmt"
	"time"

	"github.com/zgiai/ginext/internal/modules/app/workflow/file"
	"github.com/zgiai/ginext/internal/modules/app/workflow/graph_engine/entities"
	"github.com/zgiai/ginext/internal/modules/app/workflow/nodes/base"
	"github.com/zgiai/ginext/internal/modules/app/workflow/shared"
	"github.com/zgiai/ginext/pkg/logger"
)

type Node struct {
	base.NodeStruct
	NodeData
	contentExtractor file.ContentExtractor
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

	nd, nodeID, err := getData(config)
	if err != nil {
		return nil, err
	}
	
	// Extract ContentExtractor from optional dependencies if provided
	var contentExtractor file.ContentExtractor
	if len(optionalDeps) > 0 {
		if ce, ok := optionalDeps[0].(file.ContentExtractor); ok {
			contentExtractor = ce
		}
	}
	
	return &Node{
		NodeStruct: base.NodeStruct{
			InstanceID: id,
			NodeID:     nodeID,
			NodeType:   shared.Start,

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
		NodeData:         nd,
		contentExtractor: contentExtractor,
	}, nil
}

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

	// Execute the node
	result, err := n.executeRun(ctx)
	if err != nil {
		// Send failure event
		select {
		case eventChan <- &shared.NodeEventCh{
			Type:      shared.EventTypeRunFailed,
			NodeID:    n.NodeID,
			Timestamp: time.Now(),
			Error:     err,
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

func (n *Node) executeRun(ctx context.Context) (*shared.NodeRunResult, error) {

	nodeInputs := make(map[string]any)

	if n.GraphRuntimeState != nil && n.GraphRuntimeState.VariablePool != nil {
		for key, value := range n.GraphRuntimeState.VariablePool.UserInputs {
			// Validate model_config variables
			if n.isModelConfigVariable(key) {
				if err := n.validateModelConfigVariable(key, value); err != nil {
					return nil, err
				}
			}
			nodeInputs[key] = value
		}
	}

	systemInputs := n.getSystemVariables()
	for key, value := range systemInputs {
		nodeInputs[base.SYSTEM_VARIABLE_NODE_ID+"."+key] = value
	}

	outputs := make(map[string]any)
	for key, value := range nodeInputs {
		outputs[key] = value
	}
	if n.contentExtractor != nil {
		// Get workflow run ID from system variables for logging
		workflowRunID := n.getWorkflowRunID()
		
		logger.Info("File content extraction is enabled, processing file variables",
			"node_id", n.NodeID,
			"workflow_run_id", workflowRunID,
		)
		
		for key, value := range nodeInputs {
			// Skip system variables
			if len(key) > len(base.SYSTEM_VARIABLE_NODE_ID) && 
			   key[:len(base.SYSTEM_VARIABLE_NODE_ID)] == base.SYSTEM_VARIABLE_NODE_ID {
				continue
			}
			
			// Check if this is a file variable
			if n.isFileVariable(key, value) {
				logFields := []interface{}{
					"variable_name", key,
					"node_id", n.NodeID,
				}
				if workflowRunID != "" {
					logFields = append(logFields, "workflow_run_id", workflowRunID)
				}
				logger.Info("Detected file variable, extracting content", logFields...)
				
				// Process file variable to extract content
				processedVars, err := n.processFileWithContent(ctx, key, value)
				if err != nil {
					logFields := []interface{}{
						"variable_name", key,
						"node_id", n.NodeID,
						"error", err.Error(),
					}
					if workflowRunID != "" {
						logFields = append(logFields, "workflow_run_id", workflowRunID)
					}
					logger.Warn("File content extraction failed, continuing with metadata only", logFields...)
				} else {
					// Log successful extraction with content size
					if contentValue, hasContent := processedVars[key+"_content"]; hasContent {
						if contentStr, ok := contentValue.(string); ok {
							logFields := []interface{}{
								"variable_name", key,
								"node_id", n.NodeID,
								"content_size", len(contentStr),
							}
							if workflowRunID != "" {
								logFields = append(logFields, "workflow_run_id", workflowRunID)
							}
							logger.Info("File content extracted successfully", logFields...)
						}
					}
				}
				
				// Add both metadata and content variables to outputs
				for k, v := range processedVars {
					outputs[k] = v
				}
			}
		}
	} else {
		workflowRunID := n.getWorkflowRunID()
		logFields := []interface{}{
			"node_id", n.NodeID,
		}
		if workflowRunID != "" {
			logFields = append(logFields, "workflow_run_id", workflowRunID)
		}
		logger.Info("File content extraction is disabled (ContentExtractor not available), continuing with metadata only", logFields...)
	}

	return &shared.NodeRunResult{
		Status:  shared.SUCCEEDED,
		Inputs:  nodeInputs,
		Outputs: outputs,
	}, nil
}

// isFileVariable checks if a variable is a file type by examining the node's Variables list.
//
// This method determines whether a given variable should be processed for file content
// extraction by checking if it's defined as a File or FileList type in the node configuration
// and verifying that the value has the expected file metadata structure.
//
// Parameters:
//   - key: Variable name to check (e.g., "document", "attachments")
//   - value: Variable value to validate (should be map or slice for file types)
//
// Returns:
//   - bool: true if variable is a file type with valid structure, false otherwise
//
// Detection Logic:
//   - Checks if variable is defined in node's Variables list with kind File or FileList
//   - For File type: Validates value is a map with file ID field (upload_file_id, id, or related_id)
//   - For FileList type: Validates value is an array with at least one file having ID field
//
// Example:
//
//	value := map[string]interface{}{
//	    "upload_file_id": "abc-123",
//	    "name": "document.pdf",
//	}
//	if n.isFileVariable("document", value) {
//	    // Process as file variable
//	}
func (n *Node) isFileVariable(key string, value any) bool {
	// Check if the variable is defined as a file type in the node's Variables list
	for _, variable := range n.Variables {
		if variable.Val == key && (variable.Kind == File || variable.Kind == FileList) {
			// Verify the value has the expected file metadata structure
			if variable.Kind == File {
				// For single file, check if value is a map with file metadata
				if fileMap, ok := value.(map[string]interface{}); ok {
					// Check for file ID field (upload_file_id, id, or related_id)
					if _, hasUploadFileID := fileMap["upload_file_id"]; hasUploadFileID {
						return true
					}
					if _, hasID := fileMap["id"]; hasID {
						return true
					}
					if _, hasRelatedID := fileMap["related_id"]; hasRelatedID {
						return true
					}
				}
			} else if variable.Kind == FileList {
				// For file list, check if value is an array
				if fileList, ok := value.([]interface{}); ok && len(fileList) > 0 {
					// Check if first item has file metadata structure
					if firstFile, ok := fileList[0].(map[string]interface{}); ok {
						if _, hasUploadFileID := firstFile["upload_file_id"]; hasUploadFileID {
							return true
						}
						if _, hasID := firstFile["id"]; hasID {
							return true
						}
						if _, hasRelatedID := firstFile["related_id"]; hasRelatedID {
							return true
						}
					}
				}
			}
		}
	}
	return false
}

// processFileWithContent processes a file variable to extract its text content.
//
// This method handles both single files (File type) and file lists (FileList type),
// extracting text content from each file and creating content variables with the
// "_content" suffix. It delegates to ContentExtractor for the actual extraction.
//
// Parameters:
//   - ctx: Context for cancellation and timeout control
//   - key: Variable name (e.g., "document", "attachments")
//   - value: Variable value (map for File, slice for FileList)
//
// Returns:
//   - map[string]interface{}: Map containing both metadata and content variables
//       For File: {key: metadata, key_content: text}
//       For FileList: {key: [metadata array], key_content: combined text}
//   - error: Non-nil if processing fails (logged but doesn't stop workflow)
//
// Behavior:
//   - Adds workflow run ID to context for correlated logging
//   - Detects whether variable is File or FileList type
//   - Calls appropriate ContentExtractor method
//   - Logs processing status and any errors
//   - Returns partial results on error (metadata with empty/error content)
//
// Error Handling:
//   - Invalid type: Returns metadata with empty content, logs warning
//   - Extraction failure: Returns metadata with error message in content
//   - Missing file ID: Returns metadata with empty content
//
// Example:
//
//	// Single file
//	fileData := map[string]interface{}{"upload_file_id": "abc", "name": "doc.pdf"}
//	result, _ := n.processFileWithContent(ctx, "document", fileData)
//	// result = {"document": {...}, "document_content": "extracted text..."}
//	
//	// File list
//	fileList := []interface{}{fileData1, fileData2}
//	result, _ := n.processFileWithContent(ctx, "attachments", fileList)
//	// result = {"attachments": [...], "attachments_content": "=== File 1 ===\n..."}
func (n *Node) processFileWithContent(ctx context.Context, key string, value any) (map[string]interface{}, error) {
	result := make(map[string]interface{})
	
	// Get workflow run ID for logging
	workflowRunID := n.getWorkflowRunID()
	
	// Add workflow run ID to context for ContentExtractor logging
	if workflowRunID != "" {
		ctx = context.WithValue(ctx, "workflow_run_id", workflowRunID)
	}
	
	// Determine if this is a File or FileList type
	var isFileList bool
	for _, variable := range n.Variables {
		if variable.Val == key && variable.Kind == FileList {
			isFileList = true
			break
		}
	}
	
	if isFileList {
		// Handle file list
		fileList, ok := value.([]interface{})
		if !ok {
			logFields := []interface{}{
				"variable_name", key,
				"node_id", n.NodeID,
			}
			if workflowRunID != "" {
				logFields = append(logFields, "workflow_run_id", workflowRunID)
			}
			logger.Warn("File list variable has invalid type", logFields...)
			result[key] = value
			result[key+"_content"] = ""
			return result, fmt.Errorf("invalid file list type")
		}
		
		// Extract file IDs for logging
		fileCount := len(fileList)
		logFields := []interface{}{
			"variable_name", key,
			"node_id", n.NodeID,
			"file_count", fileCount,
		}
		if workflowRunID != "" {
			logFields = append(logFields, "workflow_run_id", workflowRunID)
		}
		logger.Info("Processing file list variable", logFields...)
		
		// Process file list variable
		processedVars, err := n.contentExtractor.ProcessFileListVariable(ctx, key, fileList, n.TenantID)
		if err != nil {
			logFields := []interface{}{
				"variable_name", key,
				"node_id", n.NodeID,
				"file_count", fileCount,
				"error", err.Error(),
			}
			if workflowRunID != "" {
				logFields = append(logFields, "workflow_run_id", workflowRunID)
			}
			logger.Warn("Failed to extract content for file list variable", logFields...)
		}
		
		// Merge processed variables into result
		for k, v := range processedVars {
			result[k] = v
		}
		
		return result, err
	}
	
	// Handle single file
	fileData, ok := value.(map[string]interface{})
	if !ok {
		logFields := []interface{}{
			"variable_name", key,
			"node_id", n.NodeID,
		}
		if workflowRunID != "" {
			logFields = append(logFields, "workflow_run_id", workflowRunID)
		}
		logger.Warn("File variable has invalid type", logFields...)
		result[key] = value
		result[key+"_content"] = ""
		return result, fmt.Errorf("invalid file data type")
	}
	
	// Extract file ID for logging
	fileID := ""
	if id, ok := fileData["upload_file_id"].(string); ok {
		fileID = id
	} else if id, ok := fileData["id"].(string); ok {
		fileID = id
	} else if id, ok := fileData["related_id"].(string); ok {
		fileID = id
	}
	
	logFields := []interface{}{
		"variable_name", key,
		"node_id", n.NodeID,
	}
	if fileID != "" {
		logFields = append(logFields, "file_id", fileID)
	}
	if workflowRunID != "" {
		logFields = append(logFields, "workflow_run_id", workflowRunID)
	}
	logger.Info("Processing file variable", logFields...)
	
	// Process file variable
	processedVars, err := n.contentExtractor.ProcessFileVariable(ctx, key, fileData, n.TenantID)
	if err != nil {
		logFields := []interface{}{
			"variable_name", key,
			"node_id", n.NodeID,
			"error", err.Error(),
		}
		if fileID != "" {
			logFields = append(logFields, "file_id", fileID)
		}
		if workflowRunID != "" {
			logFields = append(logFields, "workflow_run_id", workflowRunID)
		}
		logger.Warn("Failed to extract content for file variable", logFields...)
	}
	
	// Merge processed variables into result
	for k, v := range processedVars {
		result[k] = v
	}
	
	return result, err
}

// getSystemVariables gets system variables dictionary
func (n *Node) getSystemVariables() map[string]any {
	if n.GraphRuntimeState != nil && n.GraphRuntimeState.VariablePool != nil && n.GraphRuntimeState.VariablePool.SystemVariables != nil {
		return n.GraphRuntimeState.VariablePool.SystemVariables.ToDict()
	}
	return make(map[string]any)
}

// getWorkflowRunID extracts workflow run ID from system variables for logging
func (n *Node) getWorkflowRunID() string {
	if n.GraphRuntimeState != nil && 
	   n.GraphRuntimeState.VariablePool != nil && 
	   n.GraphRuntimeState.VariablePool.SystemVariables != nil {
		return n.GraphRuntimeState.VariablePool.SystemVariables.WorkflowRunID
	}
	return ""
}

// isModelConfigVariable checks if a variable is defined as model_config type
func (n *Node) isModelConfigVariable(key string) bool {
	for _, variable := range n.Variables {
		if variable.Val == key && variable.Kind == ModelConfig {
			return true
		}
	}
	return false
}

// validateModelConfigVariable validates model_config type variable value
func (n *Node) validateModelConfigVariable(key string, value any) error {
	configMap, ok := value.(map[string]interface{})
	if !ok {
		return fmt.Errorf("model_config variable '%s' must be an object", key)
	}
	
	// Validate required fields
	provider, hasProvider := configMap["provider"]
	if !hasProvider || provider == "" {
		return fmt.Errorf("model_config variable '%s' missing required field 'provider'", key)
	}
	
	model, hasModel := configMap["model"]
	if !hasModel || model == "" {
		return fmt.Errorf("model_config variable '%s' missing required field 'model'", key)
	}
	
	// Validate provider and model are strings
	if _, ok := provider.(string); !ok {
		return fmt.Errorf("model_config variable '%s' field 'provider' must be a string", key)
	}
	if _, ok := model.(string); !ok {
		return fmt.Errorf("model_config variable '%s' field 'model' must be a string", key)
	}
	
	return nil
}

func getData(config map[string]any) (NodeData, string, error) {
	nodeID, ok := config["id"]
	if !ok {
		return NodeData{}, "", fmt.Errorf("node ID is required")
	}
	nodeIDStr, ok := nodeID.(string)
	if !ok {
		return NodeData{}, "", fmt.Errorf("node ID must be string")
	}

	data, ok := config["data"]
	if !ok {
		return NodeData{}, "", fmt.Errorf("node data is required")
	}

	dataMap, ok := data.(map[string]any)
	if !ok {
		return NodeData{}, "", fmt.Errorf("node data must be object")
	}

	variables := make([]VariableEntity, 0)
	if variablesData, exists := dataMap["variables"]; exists {
		if variablesList, ok := variablesData.([]any); ok {
			for _, varData := range variablesList {
				if varMap, ok := varData.(map[string]any); ok {
					variable, err := parseVariableEntity(varMap)
					if err != nil {
						continue
					}
					variables = append(variables, variable)
				}
			}
		}
	}

	nodeData := NodeData{
		Variables: variables,
	}

	return nodeData, nodeIDStr, nil
}

func parseVariableEntity(data map[string]any) (VariableEntity, error) {
	var variable VariableEntity

	if val, ok := data["variable"].(string); ok {
		variable.Val = val
	}
	if label, ok := data["label"].(string); ok {
		variable.Label = label
	}
	if desc, ok := data["description"].(string); ok {
		variable.Desc = desc
	}
	if required, ok := data["required"].(bool); ok {
		variable.Required = required
	}
	if hide, ok := data["hide"].(bool); ok {
		variable.Hide = hide
	}
	if maxLen, ok := data["max_length"].(float64); ok {
		variable.MaxLen = int(maxLen)
	}

	if typeStr, ok := data["type"].(string); ok {
		variable.Kind = VariableEntityType(typeStr)
	}

	if options, ok := data["options"].([]any); ok {
		variable.Option = make([]string, 0, len(options))
		for _, opt := range options {
			if optStr, ok := opt.(string); ok {
				variable.Option = append(variable.Option, optStr)
			}
		}
	}

	if variable.Kind == File || variable.Kind == FileList {
		if allowedTypes, ok := data["allowed_types"].([]any); ok {
			variable.AllowFileTypes = make([]FileType, 0, len(allowedTypes))
			for _, typeVal := range allowedTypes {
				if typeStr, ok := typeVal.(string); ok {
					variable.AllowFileTypes = append(variable.AllowFileTypes, FileType(typeStr))
				}
			}
		}

		if extensions, ok := data["allowed_extensions"].([]any); ok {
			variable.AllowFileExtensions = make([]string, 0, len(extensions))
			for _, ext := range extensions {
				if extStr, ok := ext.(string); ok {
					variable.AllowFileExtensions = append(variable.AllowFileExtensions, extStr)
				}
			}
		}

		if methods, ok := data["upload_methods"].([]any); ok {
			variable.AllowFileUploadMethods = make([]FileTransferMethod, 0, len(methods))
			for _, method := range methods {
				if methodStr, ok := method.(string); ok {
					variable.AllowFileUploadMethods = append(variable.AllowFileUploadMethods, FileTransferMethod(methodStr))
				}
			}
		}
	}

	return variable, nil
}
