package answer

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/zgiai/ginext/config"
	"github.com/zgiai/ginext/internal/modules/app/workflow/graph_engine/entities"
	"github.com/zgiai/ginext/internal/modules/app/workflow/nodes/base"
	"github.com/zgiai/ginext/internal/modules/app/workflow/shared"
	"github.com/zgiai/ginext/pkg/logger"
)

// Node represents an answer node in the workflow
type Node struct {
	base.NodeStruct
	NodeData
}

// NodeData represents the data structure for answer nodes
type NodeData struct {
	base.NodeData
	Answer string `json:"answer"`

	// Streaming configuration
	Streaming *StreamingConfig `json:"streaming,omitempty"`
}

// StreamingConfig controls Answer node streaming behavior
type StreamingConfig struct {
	Enabled   bool `json:"enabled"`    // Enable/disable streaming for this node
	ChunkSize int  `json:"chunk_size"` // Characters per chunk (default: 20)
}

// New creates a new answer node
func New(
	id string,
	config map[string]any,
	graphInitParams entities.GraphInitParams,
	graph *entities.Graph,
	graphRuntimeState *entities.GraphRuntimeState,
	previousNodeID *string,
	optionalDeps ...interface{},
) (shared.NodeInterface, error) {
	nd, nodeID, err := parseAnswerNodeDataFromConfig(config)
	if err != nil {
		return nil, err
	}

	return &Node{
		NodeStruct: base.NodeStruct{
			InstanceID: id,
			NodeID:     nodeID,
			NodeType:   shared.Answer,

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
		NodeData: nd,
	}, nil
}

// parseAnswerNodeDataFromConfig parses node data and id from config
func parseAnswerNodeDataFromConfig(config map[string]any) (NodeData, string, error) {
	// 1. Get node ID
	nodeID, ok := config["id"]
	if !ok {
		return NodeData{}, "", fmt.Errorf("node ID is required")
	}
	nodeIDStr, ok := nodeID.(string)
	if !ok {
		return NodeData{}, "", fmt.Errorf("node ID must be string")
	}

	// 2. Get node data
	data, ok := config["data"]
	if !ok {
		return NodeData{}, "", fmt.Errorf("node data is required")
	}

	// 3. Convert to JSON and back to parse structure into NodeData
	jsonBytes, err := json.Marshal(data)
	if err != nil {
		return NodeData{}, "", fmt.Errorf("failed to marshal node data: %w", err)
	}

	var nodeData NodeData
	if err := json.Unmarshal(jsonBytes, &nodeData); err != nil {
		return NodeData{}, "", fmt.Errorf("failed to unmarshal node data: %w", err)
	}

	return nodeData, nodeIDStr, nil
}

// Run executes the answer node
func (n *Node) Run(ctx context.Context, eventChan chan *shared.NodeEventCh) error {
	// Start event
	select {
	case eventChan <- &shared.NodeEventCh{
		Type:      shared.EventTypeRunStarted,
		NodeID:    n.NodeID,
		Timestamp: time.Now(),
	}:
	case <-ctx.Done():
		return ctx.Err()
	}

	// Execute the answer logic
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

// executeRun performs the actual answer node logic
func (n *Node) executeRun(ctx context.Context, eventChan chan *shared.NodeEventCh) (*shared.NodeRunResult, error) {
	// Get inputs from variable pool
	inputs := n.buildInputSnapshot()

	// Extract answer template from node data
	answerTemplate := n.NodeData.Answer

	// Process variables in the answer template using variable pool
	processedAnswer := answerTemplate
	if n.GraphRuntimeState != nil && n.GraphRuntimeState.VariablePool != nil {
		// Use variable pool to convert template
		segmentGroup := n.GraphRuntimeState.VariablePool.ConvertTemplate(answerTemplate)
		processedAnswer = n.segmentGroupToText(segmentGroup)
	}

	// Check if streaming is enabled and stream the answer
	shouldStreamValue := n.shouldStream()
	logger.Debug("Answer node streaming check",
		"node_id", n.NodeID,
		"workflow_type", n.WorkflowType,
		"should_stream", shouldStreamValue,
	)
	if shouldStreamValue {
		if err := n.streamAnswer(ctx, eventChan, answerTemplate, processedAnswer); err != nil {
			// Log error but continue execution (best-effort streaming)
			logger.Warn("Answer node streaming failed, continuing with completion event",
				"node_id", n.NodeID,
				"error", err.Error(),
			)
		}
	}

	// Create outputs
	outputs := make(map[string]any)
	outputs["answer"] = processedAnswer

	// Also add all inputs to outputs for consistency
	for k, v := range inputs {
		outputs[k] = v
	}

	return &shared.NodeRunResult{
		Status:  shared.SUCCEEDED,
		Inputs:  inputs,
		Outputs: outputs,
	}, nil
}

func (n *Node) buildInputSnapshot() map[string]any {
	inputs := make(map[string]any)
	if n.GraphRuntimeState == nil || n.GraphRuntimeState.VariablePool == nil {
		return inputs
	}

	matches := entities.VariablePattern.FindAllStringSubmatch(n.NodeData.Answer, -1)
	for _, match := range matches {
		if len(match) < 2 {
			continue
		}
		selector := strings.Split(match[1], ".")
		if len(selector) == 0 {
			continue
		}
		variable := n.GraphRuntimeState.VariablePool.GetWithPath(selector)
		if variable == nil {
			continue
		}
		inputs[strings.Join(selector, ".")] = variable.ToObject()
	}

	return inputs
}

// segmentGroupToText converts segment group to text
func (n *Node) segmentGroupToText(segmentGroup *entities.SegmentGroup) string {
	if segmentGroup == nil {
		return ""
	}

	var result strings.Builder
	for _, segment := range segmentGroup.Value {
		switch segment.GetType() {
		case shared.SegmentTypeFile, shared.SegmentTypeArrayFile:
			result.WriteString(segment.Markdown())
			continue
		}

		obj := segment.ToObject()
		if str, ok := obj.(string); ok {
			result.WriteString(str)
		} else {
			// For non-string types (maps, arrays, etc.), convert to JSON
			jsonBytes, err := json.Marshal(obj)
			if err != nil {
				// Fallback to fmt.Sprintf if JSON marshaling fails
				result.WriteString(fmt.Sprintf("%v", obj))
			} else {
				result.WriteString(string(jsonBytes))
			}
		}
	}
	return result.String()
}

// shouldStream determines if this Answer node should stream output
// Answer node streaming is disabled by default - only LLM nodes should stream
func (n *Node) shouldStream() bool {
	// Answer node should NOT stream by default
	// Only LLM nodes need streaming for real-time token generation
	// Answer node just formats and returns the final result

	// Check node-level configuration first (allow explicit override)
	if n.NodeData.Streaming != nil {
		return n.NodeData.Streaming.Enabled
	}

	// Default to false - no streaming for Answer nodes
	return false
}

// getChunkSize returns the chunk size for streaming
// Node-level configuration overrides global configuration
func (n *Node) getChunkSize() int {
	if n.NodeData.Streaming != nil && n.NodeData.Streaming.ChunkSize > 0 {
		return n.NodeData.Streaming.ChunkSize
	}

	if config.GlobalConfig != nil {
		return config.GlobalConfig.AnswerNodeStreaming.ChunkSize
	}

	// Default to 20 if config not loaded
	return 20
}

// chunkText splits text into chunks, attempting to break at word boundaries
// Returns empty slice for empty input, single chunk for text shorter than chunk size
func (n *Node) chunkText(text string, chunkSize int) []string {
	if len(text) == 0 {
		return []string{}
	}

	if len(text) <= chunkSize {
		return []string{text}
	}

	var chunks []string
	runes := []rune(text)

	for i := 0; i < len(runes); {
		end := i + chunkSize
		if end > len(runes) {
			end = len(runes)
		}

		// Try to break at word boundary (space, newline, punctuation)
		if end < len(runes) {
			// Look back up to 10 characters for a good break point
			for j := 0; j < 10 && end-j > i; j++ {
				char := runes[end-j]
				if char == ' ' || char == '\n' || char == ',' || char == '.' || char == '!' || char == '?' {
					end = end - j + 1
					break
				}
			}
		}

		chunks = append(chunks, string(runes[i:end]))
		i = end
	}

	return chunks
}

func (n *Node) templateStreamParts(answerTemplate, fallbackText string) []string {
	if n.GraphRuntimeState == nil || n.GraphRuntimeState.VariablePool == nil {
		return []string{fallbackText}
	}

	parts := entities.VariablePattern.Split(answerTemplate, -1)
	matches := entities.VariablePattern.FindAllStringSubmatch(answerTemplate, -1)
	streamParts := make([]string, 0, len(parts)+len(matches))

	for i, part := range parts {
		if part != "" {
			streamParts = append(streamParts, part)
		}

		if i >= len(matches) || len(matches[i]) < 2 {
			continue
		}

		selector := strings.Split(matches[i][1], ".")
		variable := n.GraphRuntimeState.VariablePool.GetWithPath(selector)
		if variable == nil {
			continue
		}

		rendered := n.segmentToText(variable)
		if rendered != "" {
			streamParts = append(streamParts, rendered)
		}
	}

	if len(streamParts) == 0 {
		return []string{fallbackText}
	}

	return streamParts
}

func (n *Node) segmentToText(segment entities.Segment) string {
	if segment == nil {
		return ""
	}

	switch segment.GetType() {
	case shared.SegmentTypeFile, shared.SegmentTypeArrayFile:
		return segment.Markdown()
	}

	obj := segment.ToObject()
	if str, ok := obj.(string); ok {
		return str
	}

	jsonBytes, err := json.Marshal(obj)
	if err != nil {
		return fmt.Sprintf("%v", obj)
	}
	return string(jsonBytes)
}

// streamAnswer emits stream chunk events for the answer text.
// This method preserves the template segment order instead of chunking only the final rendered answer.
func (n *Node) streamAnswer(ctx context.Context, eventChan chan *shared.NodeEventCh, answerTemplate, fallbackText string) error {
	chunkSize := n.getChunkSize()
	templateParts := n.templateStreamParts(answerTemplate, fallbackText)

	chunks := make([]string, 0, len(templateParts))
	for _, part := range templateParts {
		chunks = append(chunks, n.chunkText(part, chunkSize)...)
	}

	// Limit total chunks to 10,000 per node to prevent abuse
	maxChunks := 10000
	if len(chunks) > maxChunks {
		logger.Warn("Answer node chunk count exceeds maximum, truncating",
			"node_id", n.NodeID,
			"total_chunks", len(chunks),
			"max_chunks", maxChunks,
		)
		chunks = chunks[:maxChunks]
	}

	// Log streaming start
	logger.Debug("Starting Answer node streaming",
		"node_id", n.NodeID,
		"chunk_size", chunkSize,
		"text_length", len(fallbackText),
		"template_part_count", len(templateParts),
		"total_chunks", len(chunks),
	)

	for i, chunk := range chunks {
		// Check context cancellation
		select {
		case <-ctx.Done():
			logger.Debug("Answer node streaming cancelled by context",
				"node_id", n.NodeID,
				"chunks_sent", i,
				"total_chunks", len(chunks),
			)
			return ctx.Err()
		default:
		}

		// Emit stream chunk event (blocking to ensure no data loss)
		select {
		case eventChan <- &shared.NodeEventCh{
			Type:   shared.EventTypeRunStreamChunk,
			NodeID: n.NodeID,
			Data: &shared.RunStreamChunkEvent{
				ChunkContent:         chunk,
				FromVariableSelector: []string{n.NodeID, "text"},
			},
			Timestamp: time.Now(),
		}:
			// Successfully sent chunk
			logger.Debug("Answer node chunk emitted",
				"node_id", n.NodeID,
				"chunk_index", i,
				"chunk_length", len(chunk),
			)
		case <-ctx.Done():
			logger.Debug("Answer node streaming cancelled by context during send",
				"node_id", n.NodeID,
				"chunks_sent", i,
				"total_chunks", len(chunks),
			)
			return ctx.Err()
		}

	}

	logger.Debug("Answer node streaming completed",
		"node_id", n.NodeID,
		"total_chunks", len(chunks),
	)

	return nil
}
