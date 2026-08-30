package sqlgenerator

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/zgiai/zgi/api/internal/capabilities/text2sql"
	"github.com/zgiai/zgi/api/internal/modules/app/workflow/databaseauthorization"
	"github.com/zgiai/zgi/api/internal/modules/app/workflow/graph_engine/entities"
	"github.com/zgiai/zgi/api/internal/modules/app/workflow/nodes/base"
	"github.com/zgiai/zgi/api/internal/modules/app/workflow/shared"
	llmclient "github.com/zgiai/zgi/api/internal/modules/llm/client"
	llmdefaultservice "github.com/zgiai/zgi/api/internal/modules/llm/defaultmodel/service"
	"github.com/zgiai/zgi/api/pkg/database"
	"github.com/zgiai/zgi/api/pkg/logger"
	"github.com/zgiai/zgi/api/pkg/sql_base"
	"go.uber.org/zap"
	"gorm.io/gorm"
)

const (
	defaultTimeoutSeconds = 30
	retryBackoffBase      = 150 * time.Millisecond
)

// Node represents the SQL generator workflow node.
type Node struct {
	base.NodeStruct
	NodeData NodeData

	db                 *gorm.DB
	llmInvoker         LLMInvoker
	llmClient          llmclient.LLMClient
	models             llmdefaultservice.DefaultModelResolver
	sqlClient          sql_base.SQLBase
	authorizer         databaseauthorization.Authorizer
	catalogLoader      text2sql.CatalogLoader
	billingSubjectType string
}

// New creates a new SQL generator node instance.
func New(
	id string,
	config map[string]any,
	graphInitParams entities.GraphInitParams,
	graph *entities.Graph,
	graphRuntimeState *entities.GraphRuntimeState,
	previousNodeID *string,
	optionalDeps ...any,
) (shared.NodeInterface, error) {
	nodeData, nodeID, err := parseNodeDataFromConfig(config)
	if err != nil {
		return nil, err
	}

	n := &Node{
		NodeStruct: base.NodeStruct{
			InstanceID: id,
			NodeID:     nodeID,
			NodeType:   shared.SQLGenerator,

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
		NodeData:           nodeData,
		db:                 database.GetDB(),
		billingSubjectType: graphInitParams.BillingSubjectType,
	}

	for _, dep := range optionalDeps {
		switch dependency := dep.(type) {
		case llmclient.LLMClient:
			client := dependency
			invoker, invErr := NewGatewayLLMInvoker(client, graphInitParams.OrganizationID, graphInitParams.WorkspaceID, graphInitParams.BillingSubjectType)
			if invErr != nil {
				return nil, invErr
			}
			n.llmInvoker = invoker
			n.llmClient = client
		case llmdefaultservice.DefaultModelResolver:
			n.models = dependency
		case sql_base.SQLBase:
			n.sqlClient = dependency
		case databaseauthorization.Authorizer:
			n.authorizer = dependency
		case text2sql.CatalogLoader:
			n.catalogLoader = dependency
		}
	}

	return n, nil
}

// Run executes the node.
func (n *Node) Run(ctx context.Context, eventChan chan *shared.NodeEventCh) error {
	start := &shared.NodeEventCh{
		Type:      shared.EventTypeRunStarted,
		NodeID:    n.NodeID,
		Timestamp: time.Now(),
	}

	select {
	case eventChan <- start:
	case <-ctx.Done():
		return ctx.Err()
	}

	result, err := n.executeRun(ctx)
	if err != nil {
		fail := &shared.NodeEventCh{
			Type:      shared.EventTypeRunFailed,
			NodeID:    n.NodeID,
			Error:     err,
			Timestamp: time.Now(),
		}
		select {
		case eventChan <- fail:
		case <-ctx.Done():
			return ctx.Err()
		}
		return err
	}

	complete := &shared.NodeEventCh{
		Type:      shared.EventTypeRunCompleted,
		NodeID:    n.NodeID,
		Data:      &shared.RunCompletedEvent{RunResult: result},
		Timestamp: time.Now(),
	}
	select {
	case eventChan <- complete:
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

func (n *Node) executeRun(ctx context.Context) (*shared.NodeRunResult, error) {
	logCtx := n.logContext(ctx)
	// Try to get runtime configuration from VariablePool
	runtimeDataSource, runtimeTables := n.getRuntimeDataSourceConfig()
	runtimeModelConfig := n.getRuntimeModelConfig()

	// Override static config with runtime config if provided
	configSource := "static"
	if runtimeDataSource != nil {
		n.NodeData.DataSource.Source = *runtimeDataSource
		n.NodeData.IsStaticConfig = false
		configSource = "runtime"
		logger.InfoContext(logCtx, "sql generator using runtime data source",
			zap.String("data_source_id", runtimeDataSource.ID),
		)
	}

	if runtimeTables != nil && len(runtimeTables) > 0 {
		n.NodeData.DataSource.Tables = runtimeTables
		n.NodeData.IsStaticConfig = false
		configSource = "runtime"
		logger.InfoContext(logCtx, "sql generator using runtime table selection",
			zap.Int("table_count", len(runtimeTables)),
			zap.String("data_source_id", n.NodeData.DataSource.Source.ID),
		)
	}

	if runtimeModelConfig != nil {
		n.NodeData.Model = *runtimeModelConfig
		n.NodeData.IsStaticConfig = false
		configSource = "runtime"
		logger.InfoContext(logCtx, "sql generator using runtime model config",
			zap.String("provider", runtimeModelConfig.Provider),
			zap.String("model", runtimeModelConfig.Name),
		)
	}

	logger.InfoContext(logCtx, "sql generator configuration resolved",
		zap.String("config_source", configSource),
		zap.String("provider", n.NodeData.Model.Provider),
		zap.String("model", n.NodeData.Model.Name),
		zap.String("data_source_id", n.NodeData.DataSource.Source.ID),
		zap.Int("table_count", len(n.NodeData.DataSource.Tables)),
	)

	if err := n.validateConfiguration(); err != nil {
		return n.failureResult(nil, nil, "InvalidConfiguration", err), nil
	}
	execTimeout := effectiveExecutionTimeout(n.NodeData.Execution.TimeoutSeconds, n.NodeData.TableSelectionMode)

	execCtx, cancel := context.WithTimeout(ctx, time.Duration(execTimeout)*time.Second)
	defer cancel()
	return n.executeText2SQL(execCtx)
}

func (n *Node) validateConfiguration() error {
	if strings.TrimSpace(n.NodeData.Model.Provider) == "" {
		return fmt.Errorf("model provider is required")
	}
	if strings.TrimSpace(n.NodeData.Model.Name) == "" {
		return fmt.Errorf("model name is required")
	}
	if strings.TrimSpace(n.NodeData.Prompt.User) == "" {
		return fmt.Errorf("user prompt cannot be empty")
	}
	return nil
}

func (n *Node) failureResult(
	inputs map[string]any,
	processData map[string]any,
	errType string,
	err error,
) *shared.NodeRunResult {
	errMsg := "unknown error"
	if err != nil {
		errMsg = err.Error()
	}

	if inputs == nil {
		inputs = map[string]any{}
	}
	if processData == nil {
		processData = map[string]any{}
	}

	return &shared.NodeRunResult{
		Status:      shared.FAILED,
		Inputs:      inputs,
		ProcessData: processData,
		Outputs:     map[string]any{},
		Err:         err,
		ErrMsg:      errMsg,
		ErrType:     errType,
	}
}

func (n *Node) invokeLLMWithRetry(
	ctx context.Context,
	req *InvokeRequest,
) (*InvokeResult, int, error) {
	if n.llmInvoker == nil {
		return nil, 0, fmt.Errorf("llm invoker not configured")
	}

	retries := n.NodeData.Execution.MaxRetries
	if retries < 0 {
		retries = 0
	}
	total := retries + 1
	var lastErr error

	for attempt := 1; attempt <= total; attempt++ {
		resp, err := n.llmInvoker.Invoke(ctx, n.UserID, n.APPID, AppType, req)
		if err == nil {
			return resp, attempt, nil
		}

		if ctx.Err() != nil {
			return nil, attempt, ctx.Err()
		}

		lastErr = err
		if attempt < total {
			sleep := time.Duration(attempt) * retryBackoffBase
			select {
			case <-time.After(sleep):
			case <-ctx.Done():
				return nil, attempt, ctx.Err()
			}
		}
	}

	if lastErr == nil {
		lastErr = fmt.Errorf("model invocation failed")
	}
	return nil, total, lastErr
}

func cloneParameters(params map[string]any) map[string]any {
	if len(params) == 0 {
		return map[string]any{}
	}
	c := make(map[string]any, len(params))
	for k, v := range params {
		c[k] = v
	}
	return c
}

func messageContentToString(content any) string {
	switch v := content.(type) {
	case string:
		return v
	case []byte:
		return string(v)
	case []any:
		builder := strings.Builder{}
		for _, item := range v {
			switch t := item.(type) {
			case string:
				builder.WriteString(t)
			case map[string]any:
				if text, ok := t["text"].(string); ok {
					builder.WriteString(text)
				} else {
					data, err := json.Marshal(t)
					if err == nil {
						builder.WriteString(string(data))
					}
				}
			default:
				data, err := json.Marshal(t)
				if err == nil {
					builder.WriteString(string(data))
				}
			}
		}
		return builder.String()
	case map[string]any:
		data, err := json.Marshal(v)
		if err != nil {
			return fmt.Sprintf("%v", v)
		}
		return string(data)
	default:
		data, err := json.Marshal(v)
		if err != nil {
			return fmt.Sprintf("%v", v)
		}
		return string(data)
	}
}

func parseNodeDataFromConfig(config map[string]any) (NodeData, string, error) {
	rawNodeID, ok := config["id"]
	if !ok {
		return NodeData{}, "", fmt.Errorf("node ID is required")
	}
	nodeID, ok := rawNodeID.(string)
	if !ok {
		return NodeData{}, "", fmt.Errorf("node ID must be string")
	}

	rawData, ok := config["data"]
	if !ok {
		return NodeData{}, "", fmt.Errorf("node data is required")
	}

	payloadSource := rawData
	if rawMap, mapOK := rawData.(map[string]any); mapOK {
		flattened := make(map[string]any, len(rawMap))
		for key, value := range rawMap {
			if key == "data" {
				if inner, innerOK := value.(map[string]any); innerOK {
					for innerKey, innerValue := range inner {
						flattened[innerKey] = innerValue
					}
					continue
				}
			}
			flattened[key] = value
		}
		payloadSource = flattened
	}

	payload, err := json.Marshal(payloadSource)
	if err != nil {
		return NodeData{}, "", fmt.Errorf("failed to marshal node data: %w", err)
	}

	var nodeData NodeData
	if err := json.Unmarshal(payload, &nodeData); err != nil {
		return NodeData{}, "", fmt.Errorf("failed to unmarshal node data: %w", err)
	}

	nodeData.ensureDefaults()

	// Mark this as static config from graph
	nodeData.IsStaticConfig = true

	return nodeData, nodeID, nil
}

// getRuntimeDataSourceConfig retrieves runtime data source configuration from VariablePool
func (n *Node) getRuntimeDataSourceConfig() (*calldatabase.DataSourceConfig, []calldatabase.TableRef) {
	if n.GraphRuntimeState == nil || n.GraphRuntimeState.VariablePool == nil {
		return nil, nil
	}

	userInputs := n.GraphRuntimeState.VariablePool.UserInputs
	if userInputs == nil {
		return nil, nil
	}

	var dataSource *calldatabase.DataSourceConfig
	var tables []calldatabase.TableRef

	// 1. Check for data_source in inputs (can be flat or nested structure)
	if dsRaw, exists := userInputs["data_source"]; exists {
		if dsMap, ok := dsRaw.(map[string]interface{}); ok {
			// Check if it's nested structure with "source" field (SQL Generator format)
			if sourceRaw, hasSource := dsMap["source"]; hasSource {
				if sourceMap, ok := sourceRaw.(map[string]interface{}); ok {
					ds := calldatabase.DataSourceConfig{}
					if id, ok := sourceMap["id"].(string); ok {
						ds.ID = id
					}
					if name, ok := sourceMap["name"].(string); ok {
						ds.Name = name
					}
					if dsType, ok := sourceMap["type"].(string); ok {
						ds.Type = dsType
					}
					if ds.ID != "" {
						dataSource = &ds
					}
				}
			} else {
				// Flat structure (Call Database format)
				ds := calldatabase.DataSourceConfig{}
				if id, ok := dsMap["id"].(string); ok {
					ds.ID = id
				}
				if name, ok := dsMap["name"].(string); ok {
					ds.Name = name
				}
				if dsType, ok := dsMap["type"].(string); ok {
					ds.Type = dsType
				}
				if ds.ID != "" {
					dataSource = &ds
				}
			}

			// Check for tables in nested structure
			if tablesRaw, hasTables := dsMap["tables"]; hasTables {
				if tablesList, ok := tablesRaw.([]interface{}); ok {
					tables = parseTablesList(tablesList)
				}
			}
		}
	}

	// 2. Check for table_selection in inputs (Call Database format)
	if len(tables) == 0 {
		if tablesRaw, exists := userInputs["table_selection"]; exists {
			if tablesList, ok := tablesRaw.([]interface{}); ok {
				tables = parseTablesList(tablesList)
			}
		}
	}

	return dataSource, tables
}

// parseTablesList parses a list of table objects into TableRef structs
func parseTablesList(tablesList []interface{}) []calldatabase.TableRef {
	tables := make([]calldatabase.TableRef, 0, len(tablesList))

	for _, tableRaw := range tablesList {
		if tableMap, ok := tableRaw.(map[string]interface{}); ok {
			table := calldatabase.TableRef{}

			if schema, ok := tableMap["schema"].(string); ok {
				table.Schema = schema
			}
			if name, ok := tableMap["name"].(string); ok {
				table.Name = name
			}
			// Handle both float64 (from JSON) and int
			if tableID, ok := tableMap["table_id"].(float64); ok {
				table.TableID = int(tableID)
			} else if tableID, ok := tableMap["table_id"].(int); ok {
				table.TableID = tableID
			}

			// Parse columns array
			if columnsRaw, ok := tableMap["columns"].([]interface{}); ok {
				table.Columns = make([]string, 0, len(columnsRaw))
				for _, colRaw := range columnsRaw {
					if col, ok := colRaw.(string); ok {
						table.Columns = append(table.Columns, col)
					}
				}
			}

			// Only add if at least name or table_id is present
			if table.Name != "" || table.TableID > 0 {
				tables = append(tables, table)
			}
		}
	}

	return tables
}

// getRuntimeModelConfig retrieves runtime model configuration from VariablePool
func (n *Node) getRuntimeModelConfig() *ModelSection {
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
			model := &ModelSection{
				Parameters: make(map[string]any),
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
					model.Parameters[k] = v
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
