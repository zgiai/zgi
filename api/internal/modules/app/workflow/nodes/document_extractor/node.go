package documentextractor

import (
	"context"
	"fmt"
	"time"

	"github.com/zgiai/zgi/api/internal/modules/app/workflow/file"
	"github.com/zgiai/zgi/api/internal/modules/app/workflow/graph_engine/entities"
	"github.com/zgiai/zgi/api/internal/modules/app/workflow/nodes/base"
	"github.com/zgiai/zgi/api/internal/modules/app/workflow/shared"
	"github.com/zgiai/zgi/api/pkg/logger"
)

// RetryMetadata tracks retry execution details
type RetryMetadata struct {
	TotalAttempts int           `json:"total_attempts"`
	SuccessfulAt  int           `json:"successful_at"`   // Which attempt succeeded (0 if failed)
	TotalWaitTime time.Duration `json:"total_wait_time"` // Total time spent waiting
	AttemptLog    []AttemptInfo `json:"attempt_log"`
}

// AttemptInfo records information about a single retry attempt
type AttemptInfo struct {
	AttemptNumber int       `json:"attempt_number"`
	Timestamp     time.Time `json:"timestamp"`
	Status        string    `json:"status"`           // "success", "not_ready", "failed_permanent"
	WaitDuration  int64     `json:"wait_duration_ms"` // Milliseconds waited after this attempt
}

// Node represents a document extractor node in the workflow
type Node struct {
	base.NodeStruct
	NodeData
	contentExtractor file.ContentExtractor
}

// New creates a new document extractor node
func New(
	id string,
	config map[string]any,
	graphInitParams entities.GraphInitParams,
	graph *entities.Graph,
	graphRuntimeState *entities.GraphRuntimeState,
	previousNodeID *string,
	optionalDeps ...interface{},
) (shared.NodeInterface, error) {
	nd, nodeID, err := parseDocumentExtractorNodeDataFromConfig(config)
	if err != nil {
		return nil, err
	}

	// Get content extractor from optional dependencies or use global instance
	var contentExtractor file.ContentExtractor
	if len(optionalDeps) > 0 {
		if ce, ok := optionalDeps[0].(file.ContentExtractor); ok {
			contentExtractor = ce
		}
	}
	if contentExtractor == nil {
		contentExtractor = file.GetGlobalContentExtractor()
	}

	return &Node{
		NodeStruct: base.NodeStruct{
			InstanceID: id,
			NodeID:     nodeID,
			NodeType:   shared.DocumentExtractor,

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
		},
		NodeData:         nd,
		contentExtractor: contentExtractor,
	}, nil
}

// Run executes the document extractor node
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

	// Execute the extraction logic
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

// executeRun performs the actual document extraction logic
func (n *Node) executeRun(ctx context.Context, eventChan chan *shared.NodeEventCh) (*shared.NodeRunResult, error) {
	startTime := time.Now()

	// Get variable from variable pool
	variable := n.GraphRuntimeState.VariablePool.GetWithPath(n.NodeData.VariableSelector)
	if variable == nil {
		return nil, fmt.Errorf("variable not found: %v", n.NodeData.VariableSelector)
	}

	// Check variable type
	varType := variable.GetType()
	logger.Info("Document extractor node processing variable",
		"node_id", n.NodeID,
		"variable_type", varType,
		"tenant_id", n.TenantID,
	)

	var textResults []string
	var metadataResults []map[string]any
	var inputs map[string]any
	var retryMeta *RetryMetadata

	switch varType {
	case shared.SegmentTypeFile:
		// Single file with retry support
		text, metadata, retry, err := n.extractSingleFileWithRetry(ctx, variable, eventChan)
		if err != nil {
			return nil, fmt.Errorf("failed to extract file: %w", err)
		}
		textResults = []string{text}
		metadataResults = []map[string]any{metadata}
		retryMeta = retry
		inputs = map[string]any{
			"file": variable.ToObject(),
		}

		// Stream the extracted text for task workflows
		if err := n.streamText(ctx, eventChan, text); err != nil {
			logger.Warn("Failed to stream text chunk, continuing",
				"node_id", n.NodeID,
				"error", err.Error(),
			)
		}

	case shared.SegmentTypeArrayFile:
		// Multiple files with retry support
		texts, metadatas, retry, err := n.extractMultipleFiles(ctx, variable, eventChan)
		if err != nil {
			return nil, fmt.Errorf("failed to extract files: %w", err)
		}
		textResults = texts
		metadataResults = metadatas
		retryMeta = retry
		inputs = map[string]any{
			"files": variable.ToObject(),
		}

		// Stream each extracted text for task workflows
		for _, text := range texts {
			if err := n.streamText(ctx, eventChan, text); err != nil {
				logger.Warn("Failed to stream text chunk, continuing",
					"node_id", n.NodeID,
					"error", err.Error(),
				)
			}
		}

	default:
		return nil, fmt.Errorf("invalid variable type: %s, expected file or array[file]", varType)
	}

	extractionTime := time.Since(startTime).Milliseconds()

	// Create outputs
	outputs := map[string]any{
		"text":     textResults,
		"metadata": metadataResults,
	}

	// Create process data
	processData := map[string]any{
		"extraction_time_ms": extractionTime,
		"file_count":         len(textResults),
	}

	// Add retry metadata if available
	if retryMeta != nil {
		processData["retry_metadata"] = map[string]any{
			"total_attempts":  retryMeta.TotalAttempts,
			"successful_at":   retryMeta.SuccessfulAt,
			"total_wait_time": retryMeta.TotalWaitTime.Milliseconds(),
			"attempt_log":     retryMeta.AttemptLog,
		}
	}

	logger.Info("Document extractor node completed",
		"node_id", n.NodeID,
		"file_count", len(textResults),
		"extraction_time_ms", extractionTime,
		"tenant_id", n.TenantID,
	)

	return &shared.NodeRunResult{
		Status:      shared.SUCCEEDED,
		Inputs:      inputs,
		ProcessData: processData,
		Outputs:     outputs,
	}, nil
}

// extractSingleFile extracts content from a single file
func (n *Node) extractSingleFile(ctx context.Context, variable entities.Variable) (string, map[string]any, error) {
	// Get file entity from variable
	fileValue := variable.GetValue()
	if fileValue == nil {
		return "", nil, fmt.Errorf("file value is nil")
	}

	entityFile, ok := fileValue.(*entities.File)
	if !ok {
		return "", nil, fmt.Errorf("invalid file type: %T", fileValue)
	}

	// Extract file ID
	fileID := entityFile.ID
	if fileID == "" {
		return "", nil, fmt.Errorf("file ID is empty")
	}

	logger.Info("Extracting single file",
		"node_id", n.NodeID,
		"file_id", fileID,
		"filename", entityFile.Filename,
		"tenant_id", n.TenantID,
	)

	// Extract content using ContentExtractor
	fileContent, err := n.contentExtractor.ExtractFileContent(ctx, fileID, file.ContentExtractionScope{
		OrganizationID: n.OrganizationID,
		WorkspaceID:    n.WorkspaceID,
	})
	if err != nil {
		return "", nil, fmt.Errorf("failed to extract file content: %w", err)
	}

	if fileContent.Error != nil {
		return "", nil, fmt.Errorf("file extraction error: %w", fileContent.Error)
	}

	// Build metadata
	metadata := map[string]any{
		"file_id":      fileContent.FileID,
		"content_type": fileContent.ContentType,
		"size":         fileContent.Size,
		"filename":     entityFile.Filename,
		"extension":    entityFile.Extension,
	}

	return fileContent.Content, metadata, nil
}

// extractMultipleFiles extracts content from multiple files with retry support
func (n *Node) extractMultipleFiles(ctx context.Context, variable entities.Variable, eventChan chan *shared.NodeEventCh) ([]string, []map[string]any, *RetryMetadata, error) {
	// Get file array from variable
	arrayValue := variable.GetValue()
	if arrayValue == nil {
		return nil, nil, nil, fmt.Errorf("file array value is nil")
	}

	entityFiles, ok := arrayValue.([]*entities.File)
	if !ok {
		return nil, nil, nil, fmt.Errorf("invalid file array type: %T", arrayValue)
	}

	if len(entityFiles) == 0 {
		return []string{}, []map[string]any{}, nil, nil
	}

	logger.Info("Extracting multiple files",
		"node_id", n.NodeID,
		"file_count", len(entityFiles),
		"tenant_id", n.TenantID,
	)

	// Extract file IDs
	fileIDs := make([]string, 0, len(entityFiles))
	fileMap := make(map[string]*entities.File)
	for _, entityFile := range entityFiles {
		if entityFile.ID != "" {
			fileIDs = append(fileIDs, entityFile.ID)
			fileMap[entityFile.ID] = entityFile
		}
	}

	if len(fileIDs) == 0 {
		return nil, nil, nil, fmt.Errorf("no valid file IDs found")
	}

	// Extract content using ContentExtractor
	fileContents, err := n.contentExtractor.ExtractMultipleFiles(ctx, fileIDs, file.ContentExtractionScope{
		OrganizationID: n.OrganizationID,
		WorkspaceID:    n.WorkspaceID,
	})
	if err != nil {
		return nil, nil, nil, fmt.Errorf("failed to extract multiple files: %w", err)
	}

	// Process results
	texts := make([]string, 0, len(fileContents))
	metadatas := make([]map[string]any, 0, len(fileContents))

	for _, fileContent := range fileContents {
		if fileContent.Error != nil {
			// Fail on any extraction error
			return nil, nil, nil, fmt.Errorf("file %s extraction error: %w", fileContent.FileID, fileContent.Error)
		}

		texts = append(texts, fileContent.Content)

		// Get original file entity for additional metadata
		entityFile := fileMap[fileContent.FileID]
		metadata := map[string]any{
			"file_id":      fileContent.FileID,
			"content_type": fileContent.ContentType,
			"size":         fileContent.Size,
		}
		if entityFile != nil {
			metadata["filename"] = entityFile.Filename
			metadata["extension"] = entityFile.Extension
		}

		metadatas = append(metadatas, metadata)
	}

	logger.Info("Multiple files extraction completed",
		"node_id", n.NodeID,
		"successful_count", len(texts),
		"tenant_id", n.TenantID,
	)

	// No retry metadata for array files for now
	return texts, metadatas, nil, nil
}

// extractSingleFileWithRetry extracts content with automatic retry logic
func (n *Node) extractSingleFileWithRetry(ctx context.Context, variable entities.Variable, eventChan chan *shared.NodeEventCh) (string, map[string]any, *RetryMetadata, error) {
	retryMeta := &RetryMetadata{
		AttemptLog: make([]AttemptInfo, 0),
	}

	startTime := time.Now()
	attempt := 0

	// Loop until success or timeout
	for {
		attempt++

		// Check if we've exceeded maximum total time (60 seconds)
		elapsed := time.Since(startTime)
		if elapsed >= RetryTimeout {
			logger.Warn("Document extraction exceeded maximum retry time",
				"node_id", n.NodeID,
				"file_id", n.getFileID(variable),
				"total_time", elapsed,
				"attempts", attempt,
				"tenant_id", n.TenantID,
			)
			retryMeta.TotalAttempts = attempt
			retryMeta.TotalWaitTime = elapsed
			return "", nil, retryMeta, fmt.Errorf("content extraction timeout after %v (%d attempts)", elapsed, attempt)
		}

		// Check context cancellation
		select {
		case <-ctx.Done():
			retryMeta.TotalAttempts = attempt
			retryMeta.TotalWaitTime = time.Since(startTime)
			return "", nil, retryMeta, ctx.Err()
		default:
		}

		attemptStart := time.Now()
		retryMeta.TotalAttempts = attempt

		logger.Info("Document extraction attempt",
			"node_id", n.NodeID,
			"attempt", attempt,
			"elapsed", elapsed,
			"tenant_id", n.TenantID,
		)

		// Try extraction
		text, metadata, err := n.extractSingleFile(ctx, variable)

		attemptInfo := AttemptInfo{
			AttemptNumber: attempt,
			Timestamp:     attemptStart,
		}

		if err == nil {
			// Success!
			attemptInfo.Status = "success"
			retryMeta.AttemptLog = append(retryMeta.AttemptLog, attemptInfo)
			retryMeta.SuccessfulAt = attempt
			retryMeta.TotalWaitTime = time.Since(startTime)

			logger.Info("Document extraction succeeded",
				"node_id", n.NodeID,
				"attempt", attempt,
				"total_time", retryMeta.TotalWaitTime,
				"tenant_id", n.TenantID,
			)

			return text, metadata, retryMeta, nil
		}

		// Check if error is retryable
		if !n.isRetryableError(err) {
			attemptInfo.Status = "failed_permanent"
			retryMeta.AttemptLog = append(retryMeta.AttemptLog, attemptInfo)
			retryMeta.TotalWaitTime = time.Since(startTime)

			logger.Error("Document extraction failed permanently", err)
			logger.Info("Extraction failure details",
				"node_id", n.NodeID,
				"attempt", attempt,
				"tenant_id", n.TenantID,
			)

			return "", nil, retryMeta, err
		}

		// Content not ready, will retry
		attemptInfo.Status = "not_ready"
		attemptInfo.WaitDuration = RetryInterval.Milliseconds()
		retryMeta.AttemptLog = append(retryMeta.AttemptLog, attemptInfo)

		logger.Info("Content not ready, waiting before retry",
			"node_id", n.NodeID,
			"attempt", attempt,
			"wait_duration", RetryInterval,
			"tenant_id", n.TenantID,
		)

		// Wait 2 seconds with context cancellation support
		select {
		case <-time.After(RetryInterval):
			// Continue to next attempt
		case <-ctx.Done():
			retryMeta.TotalWaitTime = time.Since(startTime)
			return "", nil, retryMeta, ctx.Err()
		}
	}
}

// isRetryableError checks if an error indicates content might become available with retry
func (n *Node) isRetryableError(err error) bool {
	if err == nil {
		return false
	}

	errMsg := err.Error()

	// Non-retryable errors: file not found, permanent failures
	nonRetryablePatterns := []string{
		"failed to get file",
		"file not found",
		"failed to extract file content",
		"extraction failed",
		"unsupported",
	}

	for _, pattern := range nonRetryablePatterns {
		if contains(errMsg, pattern) {
			return false
		}
	}

	// If we get here, assume it might be a temporary issue (empty content, processing, etc.)
	// This is a conservative approach - retry by default unless we know it's permanent
	return true
}

// contains checks if a string contains a substring (case-insensitive)
func contains(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(substr) == 0 ||
		(len(s) > 0 && len(substr) > 0 && containsHelper(s, substr)))
}

func containsHelper(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		match := true
		for j := 0; j < len(substr); j++ {
			if toLower(s[i+j]) != toLower(substr[j]) {
				match = false
				break
			}
		}
		if match {
			return true
		}
	}
	return false
}

func toLower(b byte) byte {
	if b >= 'A' && b <= 'Z' {
		return b + ('a' - 'A')
	}
	return b
}

// getFileID extracts file ID from variable for logging
func (n *Node) getFileID(variable entities.Variable) string {
	fileValue := variable.GetValue()
	if fileValue == nil {
		return ""
	}

	if entityFile, ok := fileValue.(*entities.File); ok {
		return entityFile.ID
	}

	return ""
}

// streamText emits a text_chunk event for task workflows
// This is required for task workflows to receive the extracted text content
func (n *Node) streamText(ctx context.Context, eventChan chan *shared.NodeEventCh, text string) error {
	if text == "" {
		return nil
	}

	logger.Debug("Streaming document extractor text chunk",
		"node_id", n.NodeID,
		"text_length", len(text),
		"tenant_id", n.TenantID,
	)

	// Emit stream chunk event
	select {
	case eventChan <- &shared.NodeEventCh{
		Type:   shared.EventTypeRunStreamChunk,
		NodeID: n.NodeID,
		Data: &shared.RunStreamChunkEvent{
			ChunkContent:         text,
			FromVariableSelector: []string{n.NodeID, "text"},
		},
		Timestamp: time.Now(),
	}:
		logger.Debug("Document extractor text chunk emitted",
			"node_id", n.NodeID,
			"text_length", len(text),
		)
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}
