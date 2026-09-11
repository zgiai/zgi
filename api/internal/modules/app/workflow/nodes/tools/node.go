package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"reflect"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/zgiai/zgi/api/internal/capabilities/connectortools"
	"github.com/zgiai/zgi/api/internal/modules/app/workflow/file"
	"github.com/zgiai/zgi/api/internal/modules/app/workflow/graph_engine/entities"
	"github.com/zgiai/zgi/api/internal/modules/app/workflow/nodes/base"
	"github.com/zgiai/zgi/api/internal/modules/app/workflow/shared"
	connectorcore "github.com/zgiai/zgi/api/internal/modules/connectors/core"
	connectordispatch "github.com/zgiai/zgi/api/internal/modules/connectors/dispatch"
	connectorexecution "github.com/zgiai/zgi/api/internal/modules/connectors/execution"
	"github.com/zgiai/zgi/api/internal/modules/tools"
)

// Node represents a Tools node in the workflow for the new Plugin Runner system
type Node struct {
	base.NodeStruct
	nodeData                   NodeData
	toolEngine                 *tools.ToolEngine
	connectorConfirmationStore connectorexecution.ConfirmationContinuationStore
	connectorDispatcher        connectordispatch.Dispatcher
	connectorCatalog           connectorexecution.DefinitionCatalog
	connectorActionResolver    connectordispatch.ActionInstanceResolver
}

// ToolInput represents a tool parameter input with type information
type ToolInput struct {
	Value  interface{} `json:"value"`            // Can be string (for mixed/constant) or []string (for variable)
	Type   string      `json:"type"`             // "variable", "mixed", "constant"
	Coerce string      `json:"coerce,omitempty"` // Explicit compatible normalization, e.g. singleton_array
}

// NodeData contains the configuration for the tools node
type NodeData struct {
	Connector                  *ConnectorNodeReference    `json:"connector,omitempty"`
	Title                      string                     `json:"title"`
	ProviderType               string                     `json:"provider_type"`           // builtin, runner, plugin_runner, api, workflow, app, dataset-retrieval, mcp
	ProviderID                 string                     `json:"provider_id"`             // Tool provider identifier
	ProviderName               string                     `json:"provider_name,omitempty"` // Provider display name
	ToolName                   string                     `json:"tool_name"`               // Tool name to invoke
	ToolLabel                  string                     `json:"tool_label,omitempty"`    // Tool display label
	ToolParameters             map[string]*ToolInput      `json:"tool_parameters"`         // Tool parameters with type info
	WorkflowOutputs            []tools.ToolWorkflowOutput `json:"workflow_outputs,omitempty"`
	HasWorkflowOutputsSnapshot bool                       `json:"-"`
	ToolConfigurations         map[string]any             `json:"tool_configurations"` // Pre-configured tool settings (form type)
	CredentialID               string                     `json:"credential_id,omitempty"`
	PluginUniqueIdentifier     string                     `json:"plugin_unique_identifier,omitempty"`
	ToolNodeVersion            string                     `json:"tool_node_version,omitempty"` // For backward compatibility
}

// New creates a new Tools node instance
func New(
	id string,
	config map[string]any,
	graphInitParams entities.GraphInitParams,
	graph *entities.Graph,
	graphRuntimeState *entities.GraphRuntimeState,
	previousNodeID *string,
	optionalDeps ...interface{},
) (shared.NodeInterface, error) {

	nd, nodeID, err := parseNodeDataFromConfig(config)
	if err != nil {
		return nil, fmt.Errorf("failed to parse tools node config: %w", err)
	}

	bns := base.NodeStruct{
		InstanceID: id,
		NodeID:     nodeID,
		NodeType:   shared.Tools,

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
		NodeStruct: bns,
		nodeData:   nd,
	}

	// Extract application-scoped runtime dependencies from optionalDeps.
	for _, dep := range optionalDeps {
		switch typed := dep.(type) {
		case *tools.ToolEngine:
			te := typed
			n.toolEngine = te
		case connectordispatch.Dispatcher:
			n.connectorDispatcher = typed
		case connectorexecution.DefinitionCatalog:
			n.connectorCatalog = typed
		case connectorexecution.ConfirmationContinuationStore:
			n.connectorConfirmationStore = typed
		case connectordispatch.ActionInstanceResolver:
			n.connectorActionResolver = typed
		}
	}

	if n.toolEngine == nil && n.getProviderType() != tools.ToolProviderTypeConnector {
		return nil, fmt.Errorf("ToolEngine is required for Tools node")
	}

	return n, nil
}

// Run executes the Tools node
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

	// Execute the tool logic
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

// executeRun performs the actual tool invocation via ToolEngine
func (n *Node) executeRun(ctx context.Context, eventChan chan *shared.NodeEventCh) (*shared.NodeRunResult, error) {
	variablePool := n.GraphRuntimeState.VariablePool
	toolParams, err := n.prepareToolParameters(variablePool)
	if err != nil {
		return nil, fmt.Errorf("failed to prepare tool parameters: %w", err)
	}

	ctx = n.withWorkflowRunID(ctx, variablePool)

	providerType := n.getProviderType()
	tenantID := n.TenantID
	if providerType == tools.ToolProviderTypeConnector && strings.TrimSpace(n.OrganizationID) != "" {
		tenantID = n.OrganizationID
	}
	runtimeParameters := n.workflowToolRuntimeParameters(variablePool)
	connectorIdentity, err := n.workflowConnectorIdentity(ctx, variablePool)
	if err != nil {
		return nil, fmt.Errorf("resolve Workflow Connector execution identity: %w", err)
	}
	if connectorIdentity != nil {
		// Published Connector authorization is keyed by the immutable
		// workflows.version_uuid, while Workflow graph loading continues to use
		// the concrete workflows.id held by n.WorkflowID.
		runtimeParameters["workflow_id"] = connectorIdentity.Target.ID
	}
	var connectorInput map[string]interface{}
	if providerType == tools.ToolProviderTypeConnector {
		connectorInput, err = cloneWorkflowConnectorInput(toolParams)
		if err != nil {
			return nil, fmt.Errorf("snapshot Workflow Connector input: %w", err)
		}
	}

	req := tools.WorkflowToolInvokeRequest{
		TenantID:           tenantID,
		AppID:              n.APPID,
		NodeID:             n.NodeID,
		UserID:             n.UserID,
		ProviderType:       providerType,
		ProviderID:         n.nodeData.ProviderID,
		ToolName:           n.nodeData.ToolName,
		CredentialID:       n.nodeData.CredentialID,
		ToolConfigurations: toolParams,
		ToolCredentials:    n.nodeData.ToolConfigurations, // Pass tool_configurations as credentials for Plugin SDK
		RuntimeParameters:  runtimeParameters,
	}

	var result *tools.InvokeResult
	var invokeErr error
	if providerType == tools.ToolProviderTypeConnector {
		if n.connectorDispatcher == nil || connectorIdentity == nil {
			return nil, connectorcore.NewError(connectorcore.ErrorCodeConnectorUnavailable, false, nil)
		}
		// Normalize defaults before committing the durable action identity. The
		// confirmation snapshot and dispatch must carry the same business input.
		if n.connectorCatalog == nil {
			return nil, connectorcore.NewError(connectorcore.ErrorCodeConnectorUnavailable, false, nil)
		}
		definition, ok := n.connectorCatalog.Get(connectorIdentity.ConnectorID.String())
		if !ok {
			return nil, connectorcore.NewError(connectorcore.ErrorCodeConnectorUnavailable, false, nil)
		}
		for _, operation := range definition.Operations {
			if operation.ID == connectorIdentity.OperationID.String() {
				connectorInput = connectordispatch.InputWithDefaults(operation.InputSchema, connectorInput)
				break
			}
		}
		resolveAction := n.connectorActionResolver
		if resolveAction == nil {
			resolveAction = n.persistWorkflowConnectorAction
		}
		persisted, err := resolveAction(ctx, *connectorIdentity, connectorInput)
		if err != nil {
			return nil, fmt.Errorf("persist Workflow Connector action: %w", err)
		}
		connectorIdentity = &persisted
		native, err := n.connectorDispatcher.Dispatch(ctx, connectordispatch.Invocation{Identity: *connectorIdentity, Input: connectorInput})
		invokeErr = err
		result = &tools.InvokeResult{Success: err == nil, Messages: connectortools.MessagesForInvocation(native, *connectorIdentity, err)}
	} else {
		result, invokeErr = n.toolEngine.InvokeForWorkflow(ctx, req)
	}
	if providerType == tools.ToolProviderTypeConnector && result != nil && len(result.Messages) > 0 {
		paused, handled, confirmationErr := n.pauseForConnectorConfirmation(
			ctx, connectorIdentity, connectorInput, runtimeParameters, result, invokeErr,
		)
		if confirmationErr != nil {
			return nil, fmt.Errorf("persist Workflow Connector confirmation: %w", confirmationErr)
		}
		if handled {
			return paused, nil
		}
	}
	var connectorPresentation map[string]interface{}
	if providerType == tools.ToolProviderTypeConnector {
		metadata, found, metadataErr := workflowResultMetadata(result)
		connectorPresentation = workflowConnectorOutcomePresentation(metadata)
		if metadataErr != nil || !found || invokeErr != nil || metadata.Operation == nil || !metadata.Operation.Valid() ||
			metadata.State != connectorexecution.ExecutionStateCompleted || metadata.OutcomeID == uuid.Nil || metadata.OutcomeStatus != connectorcore.OutcomeStatusSuccessful {
			failure := invokeErr
			if failure == nil {
				code := connectorcore.ErrorCodeOutcomeUnknown
				if metadata.OutcomeID != uuid.Nil && metadata.OutcomeStatus == connectorcore.OutcomeStatusFailed {
					code = connectorcore.ErrorCodeProviderUnavailable
				}
				failure = connectorcore.NewError(code, false, nil)
			}
			return &shared.NodeRunResult{
				Status:  shared.FAILED,
				Outputs: connectorPresentation,
				Metadata: map[shared.WorkflowNodeExecutionMetadataKey]interface{}{shared.ToolInfo: map[string]interface{}{
					"provider_id": n.nodeData.ProviderID, "tool_name": n.nodeData.ToolName,
					"provider_type": n.nodeData.ProviderType, "runtime_source": "connector", "result": connectorPresentation,
				}},
				Err:     failure,
				ErrType: "ConnectorOperationNotSuccessful",
			}, nil
		}
	}
	if invokeErr != nil {
		return nil, fmt.Errorf("failed to invoke tool via ToolEngine: %w", invokeErr)
	}

	workflowOutputs := n.nodeData.WorkflowOutputs
	if providerType == tools.ToolProviderTypeConnector && n.connectorCatalog != nil && connectorIdentity != nil {
		if definition, ok := n.connectorCatalog.Get(connectorIdentity.ConnectorID.String()); ok {
			if entity, err := connectortools.OperationEntity(definition, connectorIdentity.OperationID.String()); err == nil {
				workflowOutputs = entity.WorkflowOutputs
			}
		}
	} else if providerType == tools.ToolProviderTypeConnector {
		return nil, fmt.Errorf("Connector output schema is unavailable")
	} else if !n.nodeData.HasWorkflowOutputsSnapshot {
		if entity, resolveErr := n.toolEngine.ResolveToolEntity(
			ctx,
			providerType,
			n.nodeData.ProviderID,
			n.nodeData.ToolName,
			tenantID,
		); resolveErr == nil {
			workflowOutputs = entity.WorkflowOutputs
		}
	}
	outputs := n.convertResultToOutputs(result, eventChan, workflowOutputs)

	return &shared.NodeRunResult{
		Status:  shared.SUCCEEDED,
		Outputs: outputs,
		Metadata: map[shared.WorkflowNodeExecutionMetadataKey]interface{}{
			shared.ToolInfo: map[string]interface{}{
				"provider_id":      n.nodeData.ProviderID,
				"tool_name":        n.nodeData.ToolName,
				"provider_type":    n.nodeData.ProviderType,
				"system":           n.nodeData.ProviderType,
				"connector_result": connectorPresentation,
			},
		},
	}, nil
}

func (n *Node) withWorkflowRunID(ctx context.Context, variablePool *entities.VariablePool) context.Context {
	if variablePool == nil {
		return ctx
	}

	runIDVariable := variablePool.Get([]string{"sys", "workflow_run_id"})
	if runIDVariable == nil {
		return ctx
	}

	runID, ok := runIDVariable.ToObject().(string)
	if !ok {
		return ctx
	}

	runID = strings.TrimSpace(runID)
	if runID == "" {
		return ctx
	}

	return context.WithValue(ctx, "workflow_run_id", runID)
}

func (n *Node) workflowToolRuntimeParameters(variablePool *entities.VariablePool) map[string]interface{} {
	workspaceID := strings.TrimSpace(n.WorkspaceID)
	if workspaceID == "" {
		workspaceID = strings.TrimSpace(n.TenantID)
	}
	parameters := map[string]interface{}{
		"workspace_id": workspaceID,
		"workflow_id":  strings.TrimSpace(n.WorkflowID),
		"node_id":      strings.TrimSpace(n.NodeID),
	}
	if variablePool != nil {
		if value := variablePool.Get([]string{"sys", "workflow_run_id"}); value != nil {
			if runID, ok := value.ToObject().(string); ok && strings.TrimSpace(runID) != "" {
				parameters["workflow_run_id"] = strings.TrimSpace(runID)
			}
		}
	}
	return parameters
}

// getProviderType converts string provider type to tools.ToolProviderType
// Currently supports: builtin, connector, runner, plugin_runner
func (n *Node) getProviderType() tools.ToolProviderType {
	switch n.nodeData.ProviderType {
	case "builtin":
		return tools.ToolProviderTypeBuiltin
	case "connector":
		return tools.ToolProviderTypeConnector
	case "runner", "plugin_runner", "plugin":
		// runner is the preferred public value. plugin_runner and plugin remain
		// compatibility aliases for existing workflow data.
		return tools.ToolProviderTypePluginRunner

	// Not supported - reserved for future extension
	// case "api":
	// 	return tools.ToolProviderTypeAPI // Custom OpenAPI tools - not supported
	// case "workflow":
	// 	return tools.ToolProviderTypeWorkflow // Workflow as tool - not supported
	// case "app":
	// 	return tools.ToolProviderTypeApp // App as tool - not supported
	// case "dataset-retrieval":
	// 	return tools.ToolProviderTypeDatasetRetrieval // Dataset retrieval - not supported
	// case "mcp":
	// 	return tools.ToolProviderTypeMCP // MCP protocol tools - not supported

	default:
		// Default to Plugin Runner for unknown types
		return tools.ToolProviderTypePluginRunner
	}
}

// prepareToolParameters prepares tool parameters from variable pool
// Implements variable/mixed/constant parameter resolution
func (n *Node) prepareToolParameters(variablePool *entities.VariablePool) (map[string]interface{}, error) {
	params := make(map[string]interface{})
	omitEmptyConnectorValues := n.getProviderType() == tools.ToolProviderTypeConnector

	// First, add tool configurations (form type parameters)
	for k, v := range n.nodeData.ToolConfigurations {
		params[k] = v
	}

	// Then process tool parameters with type resolution
	for paramName, input := range n.nodeData.ToolParameters {
		if input == nil {
			continue
		}

		switch input.Type {
		case "variable":
			if isEmptyValueSelector(input.Value) {
				continue
			}

			// Variable type: resolve from variable pool
			selector, err := n.parseValueSelector(input.Value)
			if err != nil {
				return nil, fmt.Errorf("invalid variable selector for parameter %s: %w", paramName, err)
			}
			variable := variablePool.GetWithPath(selector)
			if variable != nil {
				value := variable.ToObject()
				if input.Coerce == "singleton_array" && !isSliceOrArray(value) {
					value = []interface{}{value}
				}
				if omitEmptyConnectorValues && isEmptyToolParameterValue(value) {
					continue
				}
				params[paramName] = value
			}

		case "mixed":
			// Mixed type: template with variable references like {{#node.field#}}
			template, ok := input.Value.(string)
			if !ok {
				return nil, fmt.Errorf("mixed type value must be string for parameter %s", paramName)
			}
			resolved := n.resolveTemplateVariables(template, variablePool)
			if omitEmptyConnectorValues && isEmptyToolParameterValue(resolved) {
				continue
			}
			params[paramName] = resolved

		case "constant":
			// Constant type: use value directly
			if omitEmptyConnectorValues && isEmptyToolParameterValue(input.Value) {
				continue
			}
			params[paramName] = input.Value

		default:
			// Unknown type, treat as constant
			if omitEmptyConnectorValues && isEmptyToolParameterValue(input.Value) {
				continue
			}
			params[paramName] = input.Value
		}
	}

	return params, nil
}

func isSliceOrArray(value interface{}) bool {
	if value == nil {
		return false
	}
	kind := reflect.TypeOf(value).Kind()
	return kind == reflect.Slice || kind == reflect.Array
}

// isEmptyToolParameterValue identifies UI placeholders that should not be sent
// to connector JSON Schema validation. Optional connector fields are persisted
// as empty strings/collections by the workflow editor; sending those values is
// different from omitting the property and can invalidate typed fields such as
// date-time. False and numeric zero remain meaningful values and are retained.
func isEmptyToolParameterValue(value interface{}) bool {
	if value == nil {
		return true
	}

	reflected := reflect.ValueOf(value)
	for reflected.Kind() == reflect.Pointer || reflected.Kind() == reflect.Interface {
		if reflected.IsNil() {
			return true
		}
		reflected = reflected.Elem()
	}

	switch reflected.Kind() {
	case reflect.String:
		return strings.TrimSpace(reflected.String()) == ""
	case reflect.Array, reflect.Slice, reflect.Map:
		return reflected.Len() == 0
	default:
		return false
	}
}

func isEmptyValueSelector(value interface{}) bool {
	switch v := value.(type) {
	case nil:
		return true
	case []string:
		return len(v) == 0
	case []any:
		return len(v) == 0
	default:
		return false
	}
}

// parseValueSelector converts input value to a variable selector ([]string)
func (n *Node) parseValueSelector(value interface{}) ([]string, error) {
	switch v := value.(type) {
	case []string:
		return v, nil
	case []interface{}:
		result := make([]string, len(v))
		for i, item := range v {
			str, ok := item.(string)
			if !ok {
				return nil, fmt.Errorf("selector item at index %d is not a string", i)
			}
			result[i] = str
		}
		return result, nil
	default:
		return nil, fmt.Errorf("value must be a string array, got %T", value)
	}
}

// resolveTemplateVariables resolves template variables like {{#node.field#}}
func (n *Node) resolveTemplateVariables(template string, variablePool *entities.VariablePool) string {
	result := template

	// Find and replace all {{#...#}} patterns
	for {
		startIdx := strings.Index(result, "{{#")
		if startIdx == -1 {
			break
		}
		endIdx := strings.Index(result[startIdx:], "#}}")
		if endIdx == -1 {
			break
		}
		endIdx += startIdx

		// Extract variable path
		varPath := result[startIdx+3 : endIdx]
		selector := strings.Split(varPath, ".")

		// Get value from variable pool
		var replacement string
		variable := variablePool.GetWithPath(selector)
		if variable != nil {
			if str, ok := variable.ToObject().(string); ok {
				replacement = str
			} else {
				replacement = fmt.Sprintf("%v", variable.ToObject())
			}
		}

		// Replace the template variable
		result = result[:startIdx] + replacement + result[endIdx+3:]
	}

	return result
}

// convertResultToOutputs converts ToolEngine result to node outputs
// Implements proper file handling, dynamic variables, and JSON output
func (n *Node) convertResultToOutputs(
	result *tools.InvokeResult,
	eventChan chan *shared.NodeEventCh,
	workflowOutputs []tools.ToolWorkflowOutput,
) map[string]interface{} {
	outputs := make(map[string]interface{})
	var textOutput string
	var files []*file.File
	var jsonOutput []map[string]interface{}
	variables := make(map[string]interface{})

	for _, msg := range result.Messages {
		switch msg.Type {
		case tools.ToolInvokeMessageTypeText:
			textOutput += msg.Text
			// Send stream chunk event for text
			n.sendStreamChunk(eventChan, "text", msg.Text, false)

		case tools.ToolInvokeMessageTypeJSON:
			if msg.Data != nil {
				if jsonObj, ok := msg.Data["json_object"].(map[string]interface{}); ok {
					jsonOutput = append(jsonOutput, jsonObj)
				} else {
					// Fallback: use the entire Data as JSON
					jsonOutput = append(jsonOutput, msg.Data)
				}
			}

		case tools.ToolInvokeMessageTypeLink:
			// Check if this is a file link
			if msg.Meta != nil {
				if fileObj := n.extractFileFromMeta(msg.Meta); fileObj != nil {
					files = append(files, fileObj)
					streamText := fmt.Sprintf("File: %s\n", msg.Text)
					textOutput += streamText
					n.sendStreamChunk(eventChan, "text", streamText, false)
					continue
				}
			}
			streamText := fmt.Sprintf("Link: %s\n", msg.Text)
			textOutput += streamText
			n.sendStreamChunk(eventChan, "text", streamText, false)

		case tools.ToolInvokeMessageTypeImage, tools.ToolInvokeMessageTypeImageLink:
			if msg.Meta != nil {
				if fileObj := n.buildFileFromImageMessage(msg); fileObj != nil {
					files = append(files, fileObj)
				}
			}

		case tools.ToolInvokeMessageTypeFile:
			if msg.Meta != nil {
				if fileObj := n.extractFileFromMeta(msg.Meta); fileObj != nil {
					files = append(files, fileObj)
				}
			}

		case tools.ToolInvokeMessageTypeBlob:
			if msg.Meta != nil {
				if fileObj := n.buildFileFromBlobMessage(msg); fileObj != nil {
					files = append(files, fileObj)
				}
			}

		case tools.ToolInvokeMessageTypeVariable:
			// Handle dynamic variable output
			varName, _ := msg.Data["variable_name"].(string)
			varValue := msg.Data["variable_value"]
			isStream, _ := msg.Data["stream"].(bool)

			if varName != "" {
				if isStream {
					// Streaming variable: accumulate string values
					if strVal, ok := varValue.(string); ok {
						if existing, ok := variables[varName].(string); ok {
							variables[varName] = existing + strVal
						} else {
							variables[varName] = strVal
						}
						n.sendStreamChunk(eventChan, varName, strVal, false)
					}
				} else {
					variables[varName] = varValue
				}
			}

		case tools.ToolInvokeMessageTypeLog:
			// Log messages are typically for debugging, can be added to metadata if needed
			continue

		case tools.ToolInvokeMessageTypeRetrieverResources:
			// Handle retriever resources for RAG
			if msg.Data != nil {
				if resources, ok := msg.Data["retriever_resources"]; ok {
					outputs["retriever_resources"] = resources
				}
				if context, ok := msg.Data["context"]; ok {
					outputs["context"] = context
				}
			}
		}
	}

	// Send final stream chunks
	n.sendStreamChunk(eventChan, "text", "", true)
	for varName := range variables {
		n.sendStreamChunk(eventChan, varName, "", true)
	}

	// Build outputs
	outputs["text"] = textOutput

	// Add files output
	if len(files) > 0 {
		outputs["files"] = files
	} else {
		outputs["files"] = []*file.File{}
	}

	// Add JSON output
	if len(jsonOutput) > 0 {
		outputs["json"] = jsonOutput
		outputs["result"] = jsonOutput[0]
	} else {
		outputs["json"] = []map[string]interface{}{}
		outputs["result"] = map[string]interface{}{}
	}

	if len(jsonOutput) > 0 {
		for _, definition := range workflowOutputs {
			if definition.ID == "" || len(definition.Path) == 0 {
				continue
			}
			if value, ok := projectWorkflowOutput(jsonOutput[0], definition.Path); ok {
				outputs[definition.ID] = value
			}
		}
	}

	// Add dynamic variables to outputs
	for k, v := range variables {
		outputs[k] = v
	}

	return outputs
}

// projectWorkflowOutput resolves a backend-compiled structured output path.
// Wildcards are materialized here instead of being exposed as variable-pool
// path syntax, keeping all existing Workflow variable semantics unchanged.
func projectWorkflowOutput(value interface{}, path []string) (interface{}, bool) {
	if len(path) == 0 {
		return value, true
	}
	segment := path[0]
	if segment == "*" {
		items, ok := workflowOutputSlice(value)
		if !ok {
			return nil, false
		}
		projected := make([]interface{}, 0, len(items))
		for _, item := range items {
			child, found := projectWorkflowOutput(item, path[1:])
			if !found {
				continue
			}
			if nested, nestedOK := workflowOutputSlice(child); nestedOK {
				projected = append(projected, nested...)
			} else {
				projected = append(projected, child)
			}
		}
		return projected, true
	}

	object, ok := value.(map[string]interface{})
	if !ok {
		return nil, false
	}
	child, exists := object[segment]
	if !exists {
		return nil, false
	}
	return projectWorkflowOutput(child, path[1:])
}

func workflowOutputSlice(value interface{}) ([]interface{}, bool) {
	if value == nil {
		return nil, false
	}
	if items, ok := value.([]interface{}); ok {
		return items, true
	}
	reflected := reflect.ValueOf(value)
	if reflected.Kind() != reflect.Slice && reflected.Kind() != reflect.Array {
		return nil, false
	}
	items := make([]interface{}, reflected.Len())
	for index := 0; index < reflected.Len(); index++ {
		items[index] = reflected.Index(index).Interface()
	}
	return items, true
}

// sendStreamChunk sends a stream chunk event
func (n *Node) sendStreamChunk(eventChan chan *shared.NodeEventCh, selector string, chunk string, isFinal bool) {
	if eventChan == nil {
		return
	}

	event := &shared.NodeEventCh{
		Type:   shared.EventTypeRunStreamChunk,
		NodeID: n.NodeID,
		Data: &shared.RunStreamChunkEvent{
			ChunkContent:         chunk,
			FromVariableSelector: []string{n.NodeID, selector},
		},
		Timestamp: time.Now(),
	}

	select {
	case eventChan <- event:
	default:
		// Channel full, skip
	}
}

// extractFileFromMeta extracts a file object from message meta
func (n *Node) extractFileFromMeta(meta map[string]interface{}) *file.File {
	if meta == nil {
		return nil
	}

	// Check if meta contains a file object
	if fileData, ok := meta["file"].(map[string]interface{}); ok {
		return n.buildFileFromMap(fileData)
	}

	return nil
}

// buildFileFromImageMessage builds a file from an image message
func (n *Node) buildFileFromImageMessage(msg tools.ToolInvokeMessage) *file.File {
	if msg.Meta == nil {
		return nil
	}

	transferMethod := file.FileTransferMethodToolFile
	if tm, ok := msg.Meta["transfer_method"].(string); ok {
		transferMethod = file.FileTransferMethod(tm)
	}

	mimeType := "image/jpeg"
	if mt, ok := msg.Meta["mime_type"].(string); ok {
		mimeType = mt
	}

	opts := []file.FileOption{
		file.WithMimeType(mimeType),
	}

	if msg.Text != "" {
		opts = append(opts, file.WithRemoteURL(msg.Text))
	}

	// Extract tool file ID from URL if present
	if msg.Text != "" {
		parts := strings.Split(msg.Text, "/")
		if len(parts) > 0 {
			filenameParts := strings.Split(parts[len(parts)-1], ".")
			if len(filenameParts) > 0 {
				opts = append(opts, file.WithRelatedID(filenameParts[0]))
			}
			if len(filenameParts) > 1 {
				opts = append(opts, file.WithExtension("."+filenameParts[1]))
			}
		}
	}

	return file.NewFile(n.TenantID, file.FileTypeImage, transferMethod, opts...)
}

// buildFileFromBlobMessage builds a file from a blob message
func (n *Node) buildFileFromBlobMessage(msg tools.ToolInvokeMessage) *file.File {
	if msg.Meta == nil {
		return nil
	}

	mimeType := "application/octet-stream"
	if mt, ok := msg.Meta["mime_type"].(string); ok {
		mimeType = mt
	}

	// Determine file type from mime type
	fileType := file.FileTypeCustom
	if strings.HasPrefix(mimeType, "image/") {
		fileType = file.FileTypeImage
	} else if strings.HasPrefix(mimeType, "video/") {
		fileType = file.FileTypeVideo
	} else if strings.HasPrefix(mimeType, "audio/") {
		fileType = file.FileTypeAudio
	} else if strings.Contains(mimeType, "text") || strings.Contains(mimeType, "pdf") {
		fileType = file.FileTypeDocument
	}

	opts := []file.FileOption{
		file.WithMimeType(mimeType),
	}

	// Extract tool file ID from URL
	if msg.Text != "" {
		parts := strings.Split(msg.Text, "/")
		if len(parts) > 0 {
			filenameParts := strings.Split(parts[len(parts)-1], ".")
			if len(filenameParts) > 0 {
				opts = append(opts, file.WithRelatedID(filenameParts[0]))
			}
			if len(filenameParts) > 1 {
				opts = append(opts, file.WithExtension("."+filenameParts[1]))
			}
		}
	}

	return file.NewFile(n.TenantID, fileType, file.FileTransferMethodToolFile, opts...)
}

// buildFileFromMap builds a file from a map representation
func (n *Node) buildFileFromMap(data map[string]interface{}) *file.File {
	if data == nil {
		return nil
	}

	// Determine file type
	fileType := file.FileTypeCustom
	if ft, ok := data["type"].(string); ok {
		switch ft {
		case "image":
			fileType = file.FileTypeImage
		case "document":
			fileType = file.FileTypeDocument
		case "audio":
			fileType = file.FileTypeAudio
		case "video":
			fileType = file.FileTypeVideo
		}
	}

	// Determine transfer method
	transferMethod := file.FileTransferMethodToolFile
	if tm, ok := data["transfer_method"].(string); ok {
		transferMethod = file.FileTransferMethod(tm)
	}

	tenantID := n.TenantID
	if tid, ok := data["tenant_id"].(string); ok && tid != "" {
		tenantID = tid
	}

	opts := []file.FileOption{}

	if id, ok := data["id"].(string); ok {
		opts = append(opts, file.WithID(id))
	}
	if relatedID, ok := data["related_id"].(string); ok {
		opts = append(opts, file.WithRelatedID(relatedID))
	}
	if filename, ok := data["filename"].(string); ok {
		opts = append(opts, file.WithFilename(filename))
	}
	if ext, ok := data["extension"].(string); ok {
		opts = append(opts, file.WithExtension(ext))
	}
	if mimeType, ok := data["mime_type"].(string); ok {
		opts = append(opts, file.WithMimeType(mimeType))
	}
	if size, ok := data["size"].(float64); ok {
		opts = append(opts, file.WithSize(int(size)))
	}
	if url, ok := data["remote_url"].(string); ok {
		opts = append(opts, file.WithRemoteURL(url))
	}
	if url, ok := data["url"].(string); ok {
		opts = append(opts, file.WithURL(url))
	}

	return file.NewFile(tenantID, fileType, transferMethod, opts...)
}

// parseNodeDataFromConfig parses the tools node configuration
func parseNodeDataFromConfig(config map[string]any) (NodeData, string, error) {
	var nd NodeData

	// Get node ID
	nodeID, ok := config["id"].(string)
	if !ok {
		return nd, "", fmt.Errorf("node id is required")
	}

	// Get data section
	data, ok := config["data"].(map[string]interface{})
	if !ok {
		return nd, "", fmt.Errorf("node data is required")
	}

	// Parse title
	if title, ok := data["title"].(string); ok {
		nd.Title = title
	}

	// Parse provider_type (optional, defaults to runner)
	if providerType, ok := data["provider_type"].(string); ok {
		nd.ProviderType = providerType
	} else {
		nd.ProviderType = "runner"
	}

	// Parse provider_id (required)
	providerID, ok := data["provider_id"].(string)
	if !ok || providerID == "" {
		// Fallback to tool_provider for compatibility
		providerID, ok = data["tool_provider"].(string)
		if !ok || providerID == "" {
			return nd, "", fmt.Errorf("provider_id is required")
		}
	}
	nd.ProviderID = providerID

	// Parse tool_name (required)
	toolName, ok := data["tool_name"].(string)
	if !ok || toolName == "" {
		return nd, "", fmt.Errorf("tool_name is required")
	}
	nd.ToolName = toolName

	// Parse tool_parameters (with type info)
	if toolParams, ok := data["tool_parameters"].(map[string]interface{}); ok {
		nd.ToolParameters = make(map[string]*ToolInput)
		for paramName, paramValue := range toolParams {
			if paramValue == nil {
				continue
			}
			if paramMap, ok := paramValue.(map[string]interface{}); ok {
				input := &ToolInput{}
				if v, ok := paramMap["value"]; ok {
					input.Value = v
				}
				if t, ok := paramMap["type"].(string); ok {
					input.Type = t
				}
				if coerce, ok := paramMap["coerce"].(string); ok {
					input.Coerce = coerce
				}
				nd.ToolParameters[paramName] = input
			}
		}
	} else {
		nd.ToolParameters = make(map[string]*ToolInput)
	}

	// Preserve the output projection snapshot saved with the workflow. The
	// presence flag distinguishes an explicitly empty snapshot from legacy
	// drafts that predate workflow_outputs and may safely use the live catalog.
	if rawOutputs, exists := data["workflow_outputs"]; exists {
		nd.HasWorkflowOutputsSnapshot = true
		encodedOutputs, err := json.Marshal(rawOutputs)
		if err != nil {
			return nd, "", fmt.Errorf("invalid workflow_outputs: %w", err)
		}
		if err := json.Unmarshal(encodedOutputs, &nd.WorkflowOutputs); err != nil {
			return nd, "", fmt.Errorf("invalid workflow_outputs: %w", err)
		}
	}

	// Parse tool_configurations (form type settings)
	if toolConfigs, ok := data["tool_configurations"].(map[string]interface{}); ok {
		nd.ToolConfigurations = toolConfigs
	} else {
		nd.ToolConfigurations = make(map[string]interface{})
	}

	// Parse credential_id (optional)
	if credID, ok := data["credential_id"].(string); ok {
		nd.CredentialID = credID
	}

	if reference, ok := data["connector"].(map[string]interface{}); ok {
		encoded, err := json.Marshal(reference)
		if err != nil {
			return nd, "", err
		}
		nd.Connector = &ConnectorNodeReference{}
		if err = json.Unmarshal(encoded, nd.Connector); err != nil {
			return nd, "", err
		}
	}

	// Parse plugin_unique_identifier (optional)
	if pluginID, ok := data["plugin_unique_identifier"].(string); ok {
		nd.PluginUniqueIdentifier = pluginID
	}

	// Parse provider_name (optional)
	if providerName, ok := data["provider_name"].(string); ok {
		nd.ProviderName = providerName
	}

	// Parse tool_label (optional)
	if toolLabel, ok := data["tool_label"].(string); ok {
		nd.ToolLabel = toolLabel
	}

	// Parse tool_node_version (optional)
	if version, ok := data["tool_node_version"].(string); ok {
		nd.ToolNodeVersion = version
	}

	return nd, nodeID, nil
}
