package calldatabase

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"
	"unicode"

	"github.com/zgiai/zgi/api/internal/modules/app/workflow/graph_engine/entities"
	"github.com/zgiai/zgi/api/internal/modules/app/workflow/nodes/base"
	"github.com/zgiai/zgi/api/internal/modules/app/workflow/shared"
	"github.com/zgiai/zgi/api/pkg/logger"
	"github.com/zgiai/zgi/api/pkg/sql_base"
	"github.com/zgiai/zgi/api/pkg/sql_base/audit"
	"go.uber.org/zap"
)

const (
	defaultTimeoutSeconds = 30
	retryBackoffBase      = 150 * time.Millisecond
	tableNamePrefix       = "zgi_base_"
)

// Node represents the call-database workflow node.
type Node struct {
	base.NodeStruct
	NodeData NodeData

	sqlClient sql_base.SQLBase
}

// New creates a new call-database node instance.
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

	var sqlClient sql_base.SQLBase
	if len(optionalDeps) > 0 {
		if dep, ok := optionalDeps[0].(sql_base.SQLBase); ok {
			sqlClient = dep
		}
	}

	if sqlClient == nil {
		sqlClient, err = sql_base.NewSQLBaseClient()
		if err != nil {
			return nil, fmt.Errorf("failed to init sql base client: %w", err)
		}
	}

	workspaceID := strings.TrimSpace(graphInitParams.WorkspaceID)
	if workspaceID == "" {
		workspaceID = strings.TrimSpace(graphInitParams.TenantID)
	}

	return &Node{
		NodeStruct: base.NodeStruct{
			InstanceID: id,
			NodeID:     nodeID,
			NodeType:   shared.CallDatabase,

			TenantID:          graphInitParams.TenantID,
			APPID:             graphInitParams.AppID,
			WorkflowType:      string(graphInitParams.WorkflowType),
			WorkflowID:        graphInitParams.WorkflowID,
			WorkspaceID:       workspaceID,
			OrganizationID:    graphInitParams.OrganizationID,
			UserFrom:          string(graphInitParams.UserFrom),
			UserID:            graphInitParams.UserID,
			GraphConfig:       graphInitParams.GraphConfig,
			InvokeFrom:        string(graphInitParams.InvokeFrom),
			WorkflowCallDepth: graphInitParams.CallDepth,

			Graph:             graph,
			GraphRuntimeState: graphRuntimeState,
			PreviousNodeID:    previousNodeID,
		},
		NodeData:  nodeData,
		sqlClient: sqlClient,
	}, nil
}

// Run executes the node.
func (n *Node) Run(ctx context.Context, eventChan chan *shared.NodeEventCh) error {
	startEvent := &shared.NodeEventCh{
		Type:      shared.EventTypeRunStarted,
		NodeID:    n.NodeID,
		Timestamp: time.Now(),
	}

	select {
	case eventChan <- startEvent:
	case <-ctx.Done():
		return ctx.Err()
	}

	result, err := n.executeRun(ctx)
	if err != nil {
		failEvent := &shared.NodeEventCh{
			Type:      shared.EventTypeRunFailed,
			NodeID:    n.NodeID,
			Error:     err,
			Timestamp: time.Now(),
		}
		select {
		case eventChan <- failEvent:
		case <-ctx.Done():
			return ctx.Err()
		}
		return err
	}

	completeEvent := &shared.NodeEventCh{
		Type:      shared.EventTypeRunCompleted,
		NodeID:    n.NodeID,
		Data:      &shared.RunCompletedEvent{RunResult: result},
		Timestamp: time.Now(),
	}
	select {
	case eventChan <- completeEvent:
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

// executeRun performs the SQL execution.
func (n *Node) executeRun(ctx context.Context) (*shared.NodeRunResult, error) {
	logCtx := n.logContext(ctx)
	// Try to get runtime configuration from VariablePool
	runtimeDataSource, runtimeTables := n.getRuntimeDataSourceConfig()

	// Override static config with runtime config if provided
	configSource := "static"
	if runtimeDataSource != nil {
		n.NodeData.DataSource = *runtimeDataSource
		n.NodeData.IsStaticConfig = false
		configSource = "runtime"
		logger.InfoContext(logCtx, "call database using runtime data source",
			zap.String("data_source_id", runtimeDataSource.ID),
		)
	}

	if runtimeTables != nil && len(runtimeTables) > 0 {
		n.NodeData.TableSelection = runtimeTables
		n.NodeData.IsStaticConfig = false
		configSource = "runtime"
		logger.InfoContext(logCtx, "call database using runtime table selection",
			zap.String("data_source_id", n.NodeData.DataSource.ID),
			zap.Int("table_count", len(runtimeTables)),
		)
	}

	logger.InfoContext(logCtx, "call database configuration resolved",
		zap.String("config_source", configSource),
		zap.String("data_source_id", n.NodeData.DataSource.ID),
		zap.Int("table_count", len(n.NodeData.TableSelection)),
	)

	sqlText := strings.TrimSpace(n.NodeData.ManualSQL)
	if sqlText == "" {
		return nil, fmt.Errorf("sql text is empty")
	}

	// Render template variables if present
	if strings.Contains(sqlText, "{{#") && strings.Contains(sqlText, "#}}") {
		renderedSQL, err := n.renderSQLTemplate(sqlText)
		if err != nil {
			return nil, fmt.Errorf("failed to render SQL template: %w", err)
		}
		sqlText = renderedSQL
	}

	sqlText = ensureQuotedIdentifiers(sqlText, n.NodeData.TableSelection)

	execTimeout := n.NodeData.Execution.TimeoutSeconds
	if execTimeout == 0 {
		execTimeout = defaultTimeoutSeconds
	}
	execCtx := ctx
	var cancel context.CancelFunc
	if execTimeout > 0 {
		execCtx, cancel = context.WithTimeout(ctx, time.Duration(execTimeout)*time.Second)
		defer cancel()
	}

	start := time.Now()
	auditTableID, auditTableName := auditTableContext(n.NodeData.TableSelection)
	auditCtx := &audit.Context{
		OrganizationID: n.OrganizationID,
		WorkspaceID:    n.WorkspaceID,
		DataSourceID:   n.NodeData.DataSource.ID,
		DataSourceName: n.NodeData.DataSource.Name,
		TableID:        auditTableID,
		TableName:      auditTableName,
		ClientType:     audit.ClientTypeWorkflow,
		WorkflowRunID:  n.workflowRunID(),
		NodeID:         n.NodeID,
		CreatedBy:      n.UserID,
		OperationType:  inferOperationType(sqlText),
	}
	result, attempts, execErr := n.executeWithRetry(execCtx, sqlText, auditCtx)
	duration := time.Since(start)

	processData := map[string]any{
		"data_source":     n.NodeData.DataSource,
		"table_selection": n.NodeData.TableSelection,
		"attempts":        attempts,
		"duration_ms":     duration.Milliseconds(),
	}

	inputs := map[string]any{
		"sql": sqlText,
	}

	if execErr != nil {
		logger.CriticalContext(logCtx, "call database sql execution failed",
			zap.String("data_source_id", n.NodeData.DataSource.ID),
			zap.Int("table_count", len(n.NodeData.TableSelection)),
			zap.Int("attempts", attempts),
			zap.Error(execErr),
		)
		errorOutputs := map[string]any{
			"sql":   sqlText,
			"error": execErr.Error(),
		}
		return &shared.NodeRunResult{
			Status: shared.FAILED,
			Inputs: inputs,
			ProcessData: mergeMaps(processData, map[string]any{
				"error": execErr.Error(),
			}),
			Outputs: errorOutputs,
			ErrMsg:  execErr.Error(),
			ErrType: func() string {
				if errors.Is(execErr, context.DeadlineExceeded) {
					return "QueryTimeout"
				}
				if errors.Is(execErr, context.Canceled) {
					return "QueryCanceled"
				}
				return "QueryExecutionError"
			}(),
		}, nil
	}

	rowMaps := convertRows(result)
	outputs := map[string]any{
		"sql":            sqlText,
		"columns":        result.Columns,
		"rows":           rowMaps,
		"row_count":      len(rowMaps),
		"rows_affected":  result.RowsAffected,
		"duration_ms":    duration.Milliseconds(),
		"table_context":  n.NodeData.TableSelection,
		"data_source_id": n.NodeData.DataSource.ID,
	}

	return &shared.NodeRunResult{
		Status:      shared.SUCCEEDED,
		Inputs:      inputs,
		ProcessData: processData,
		Outputs:     outputs,
		Metadata: map[shared.WorkflowNodeExecutionMetadataKey]any{
			shared.ToolInfo: map[string]any{
				"type":      "database",
				"row_count": len(rowMaps),
			},
		},
	}, nil
}

func (n *Node) executeWithRetry(ctx context.Context, sqlText string, auditCtx *audit.Context) (*sql_base.QueryResult, int, error) {
	retryTimes := n.NodeData.Execution.MaxRetries
	if retryTimes < 0 {
		retryTimes = 0
	}
	attempts := retryTimes + 1

	var lastErr error
	for attempt := 1; attempt <= attempts; attempt++ {
		result, err := n.sqlClient.ExecuteSQL(ctx, sqlText, nil, auditCtx)
		if err == nil {
			return result, attempt, nil
		}

		lastErr = err
		if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
			return nil, attempt, err
		}

		if attempt < attempts {
			sleep := time.Duration(attempt) * retryBackoffBase
			select {
			case <-time.After(sleep):
			case <-ctx.Done():
				return nil, attempt, ctx.Err()
			}
		}
	}

	if lastErr == nil {
		lastErr = fmt.Errorf("unknown error executing query")
	}

	return nil, attempts, lastErr
}

func inferOperationType(sqlText string) string {
	parts := strings.Fields(strings.TrimSpace(sqlText))
	if len(parts) == 0 {
		return "query"
	}
	switch strings.ToLower(parts[0]) {
	case "select", "with", "show":
		return "query"
	case "insert":
		return "create"
	case "update":
		return "update"
	case "delete", "truncate":
		return "delete"
	case "create":
		return "create"
	default:
		return "query"
	}
}

func auditTableContext(tables []TableRef) (string, string) {
	if len(tables) != 1 {
		return "", ""
	}
	table := tables[0]
	tableName := table.Label
	if tableName == "" {
		tableName = table.Name
	}
	return table.ID, tableName
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
	if nodeData.Execution.TimeoutSeconds == 0 {
		nodeData.Execution.TimeoutSeconds = defaultTimeoutSeconds
	}

	// Mark this as static config from graph
	nodeData.IsStaticConfig = true

	return nodeData, nodeID, nil
}

func convertRows(result *sql_base.QueryResult) []map[string]any {
	if result == nil || len(result.Rows) == 0 {
		return []map[string]any{}
	}

	rows := make([]map[string]any, 0, len(result.Rows))
	for _, row := range result.Rows {
		record := make(map[string]any, len(result.Columns))
		for idx, col := range result.Columns {
			var value any
			if idx < len(row) {
				value = row[idx]
			}
			record[col] = value
		}
		rows = append(rows, record)
	}
	return rows
}

func mergeMaps(maps ...map[string]any) map[string]any {
	result := make(map[string]any)
	for _, m := range maps {
		for k, v := range m {
			result[k] = v
		}
	}
	return result
}

func buildSchemaTablesSnapshot(dataSource DataSourceConfig, tables []TableRef) []string {
	result := make([]string, 0, len(tables))
	sourceName := dataSource.Name
	if sourceName == "" {
		sourceName = dataSource.ID
	}
	for _, table := range tables {
		tableName := table.Name
		if tableName == "" {
			continue
		}
		if sourceName != "" {
			result = append(result, sourceName+"."+tableName)
			continue
		}
		result = append(result, tableName)
	}
	return result
}

type identifierReplacement struct {
	targetRunes []rune
	replacement string
	qualified   bool
}

func ensureQuotedIdentifiers(sql string, tables []TableRef) string {
	if len(sql) == 0 || len(tables) == 0 {
		return sql
	}

	replacements := buildIdentifierReplacements(tables)
	if len(replacements) == 0 {
		return sql
	}

	runes := []rune(sql)
	var builder strings.Builder
	builder.Grow(len(sql) + len(sql)/4)

	inSingle := false
	inDouble := false

	for i := 0; i < len(runes); {
		r := runes[i]

		if r == '\'' && !inDouble {
			inSingle = !inSingle
			builder.WriteRune(r)
			i++
			continue
		}
		if r == '"' && !inSingle {
			inDouble = !inDouble
			builder.WriteRune(r)
			i++
			continue
		}
		if inSingle || inDouble {
			builder.WriteRune(r)
			i++
			continue
		}

		matched := false
		for _, repl := range replacements {
			if len(runes)-i < len(repl.targetRunes) {
				continue
			}

			match := true
			for k := 0; k < len(repl.targetRunes); k++ {
				if runes[i+k] != repl.targetRunes[k] {
					match = false
					break
				}
			}
			if !match {
				continue
			}

			if i > 0 {
				prev := runes[i-1]
				if isIdentifierRune(prev) || prev == '"' {
					match = false
				}
			}
			if match {
				nextIdx := i + len(repl.targetRunes)
				if nextIdx < len(runes) {
					next := runes[nextIdx]
					if isIdentifierRune(next) || next == '"' {
						match = false
					}
				}
			}
			if !match {
				continue
			}

			builder.WriteString(repl.replacement)
			i += len(repl.targetRunes)
			matched = true
			break
		}

		if matched {
			continue
		}

		builder.WriteRune(r)
		i++
	}

	return builder.String()
}

func buildIdentifierReplacements(tables []TableRef) []identifierReplacement {
	seen := make(map[string]struct{})
	replacements := make([]identifierReplacement, 0, len(tables)*3)

	addReplacement := func(target string, replacement string, qualified bool) {
		if target == "" {
			return
		}
		key := fmt.Sprintf("%s|%t", target, qualified)
		if _, exists := seen[key]; exists {
			return
		}
		replacements = append(replacements, identifierReplacement{
			targetRunes: []rune(target),
			replacement: replacement,
			qualified:   qualified,
		})
		seen[key] = struct{}{}
	}

	for _, table := range tables {
		name := strings.TrimSpace(table.Name)
		if name == "" {
			continue
		}

		physicalName := applyTableNamePrefix(name)
		logicalName := strings.TrimPrefix(name, tableNamePrefix)
		if logicalName == "" {
			logicalName = name
		}

		schema := strings.TrimSpace(table.Schema)
		if schema != "" {
			replacement := fmt.Sprintf("%s.%s", quoteIdentifier(schema), quoteIdentifier(physicalName))
			addReplacement(fmt.Sprintf("%s.%s", schema, logicalName), replacement, true)
			addReplacement(fmt.Sprintf("%s.%s", schema, physicalName), replacement, true)
		}

		unqualifiedReplacement := quoteIdentifier(physicalName)
		addReplacement(logicalName, unqualifiedReplacement, false)
		addReplacement(physicalName, unqualifiedReplacement, false)
	}

	sort.Slice(replacements, func(i, j int) bool {
		return len(replacements[i].targetRunes) > len(replacements[j].targetRunes)
	})

	return replacements
}

func applyTableNamePrefix(name string) string {
	if name == "" || strings.HasPrefix(name, tableNamePrefix) {
		return name
	}
	return tableNamePrefix + name
}

func quoteIdentifier(identifier string) string {
	escaped := strings.ReplaceAll(identifier, `"`, `""`)
	return `"` + escaped + `"`
}

func isIdentifierRune(r rune) bool {
	return r == '_' || r == '$' || unicode.IsLetter(r) || unicode.IsDigit(r)
}

// renderSQLTemplate renders template variables in SQL using the variable pool
func (n *Node) renderSQLTemplate(sqlTemplate string) (string, error) {
	if n.GraphRuntimeState == nil || n.GraphRuntimeState.VariablePool == nil {
		return "", fmt.Errorf("variable pool not available for template rendering")
	}

	// Extract variable references from template (e.g., {{#nodeID.varName#}})
	// Simple regex-like parsing for {{#...#}} patterns
	result := sqlTemplate
	startIdx := 0

	for {
		start := strings.Index(result[startIdx:], "{{#")
		if start == -1 {
			break
		}
		start += startIdx

		end := strings.Index(result[start:], "#}}")
		if end == -1 {
			return "", fmt.Errorf("unclosed template variable at position %d", start)
		}
		end += start

		// Extract variable path (e.g., "nodeID.varName")
		varPath := result[start+3 : end]
		parts := strings.Split(varPath, ".")

		if len(parts) < 2 {
			return "", fmt.Errorf("invalid variable reference: %s (expected format: nodeID.varName)", varPath)
		}

		// Get value from variable pool
		value := n.GraphRuntimeState.VariablePool.GetWithPath(parts)
		if value == nil {
			return "", fmt.Errorf("variable not found in pool: %s", varPath)
		}

		// Convert value to string
		var strValue string
		switch v := value.ToObject().(type) {
		case string:
			strValue = v
		case nil:
			return "", fmt.Errorf("variable %s is nil", varPath)
		default:
			strValue = fmt.Sprintf("%v", v)
		}

		// Replace template with value
		result = result[:start] + strValue + result[end+3:]
		startIdx = start + len(strValue)
	}

	return result, nil
}

// getRuntimeDataSourceConfig retrieves runtime data source configuration from VariablePool
func (n *Node) getRuntimeDataSourceConfig() (*DataSourceConfig, []TableRef) {
	if n.GraphRuntimeState == nil || n.GraphRuntimeState.VariablePool == nil {
		return nil, nil
	}

	userInputs := n.GraphRuntimeState.VariablePool.UserInputs
	if userInputs == nil {
		return nil, nil
	}

	var dataSource *DataSourceConfig
	var tables []TableRef

	// 1. Check for data_source in inputs (can be flat or nested structure)
	if dsRaw, exists := userInputs["data_source"]; exists {
		if dsMap, ok := dsRaw.(map[string]interface{}); ok {
			// Check if it's nested structure with "source" field (SQL Generator format)
			if sourceRaw, hasSource := dsMap["source"]; hasSource {
				if sourceMap, ok := sourceRaw.(map[string]interface{}); ok {
					ds := DataSourceConfig{}
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
				ds := DataSourceConfig{}
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
					tables = parseCallDatabaseTablesList(tablesList)
				}
			}
		}
	}

	// 2. Check for table_selection in inputs (Call Database format)
	if len(tables) == 0 {
		if tablesRaw, exists := userInputs["table_selection"]; exists {
			if tablesList, ok := tablesRaw.([]interface{}); ok {
				tables = parseCallDatabaseTablesList(tablesList)
			}
		}
	}

	return dataSource, tables
}

// parseCallDatabaseTablesList parses a list of table objects into TableRef structs
func parseCallDatabaseTablesList(tablesList []interface{}) []TableRef {
	tables := make([]TableRef, 0, len(tablesList))

	for _, tableRaw := range tablesList {
		if tableMap, ok := tableRaw.(map[string]interface{}); ok {
			table := TableRef{}

			if schema, ok := tableMap["schema"].(string); ok {
				table.Schema = schema
			}
			if name, ok := tableMap["name"].(string); ok {
				table.Name = name
			}
			if label, ok := tableMap["label"].(string); ok {
				table.Label = label
			}
			if id, ok := tableMap["id"].(string); ok {
				table.ID = id
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

			// Only add if at least a display name or id is present
			if table.Name != "" || table.ID != "" || table.TableID > 0 {
				tables = append(tables, table)
			}
		}
	}

	return tables
}
