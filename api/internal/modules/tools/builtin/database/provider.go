package database

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"strconv"
	"strings"

	"github.com/zgiai/zgi/api/internal/dto"
	datasourcemodel "github.com/zgiai/zgi/api/internal/modules/datasource/model"
	"github.com/zgiai/zgi/api/internal/modules/tools"
	"github.com/zgiai/zgi/api/internal/modules/tools/builtin"
	workspacemodel "github.com/zgiai/zgi/api/internal/modules/workspace/model"
)

const (
	ProviderID                  = "database"
	ToolListAccessibleDatabases = "list_accessible_databases"
	ToolListDatabaseTables      = "list_database_tables"
	ToolDescribeDatabaseTable   = "describe_database_table"
	ToolQueryTableRecords       = "query_table_records"
	ToolInsertTableRecords      = "insert_table_records"
	ToolUpdateTableRecords      = "update_table_records"
	ToolDeleteTableRecords      = "delete_table_records"

	defaultDatabaseListLimit = 20
	defaultTableListLimit    = 50
	defaultRecordLimit       = 20
	maxRecordLimit           = 100
)

type DataSourceService interface {
	ListDataSources(ctx context.Context, organizationID, accountID string, filterWorkspaceIDs []string) ([]*dto.DataSourceResponse, error)
	GetDataSourceByID(ctx context.Context, organizationID, id, accountID string) (*dto.DataSourceResponse, error)
	ListTables(ctx context.Context, organizationID, dataSourceID string, accountID string) ([]*datasourcemodel.Table, error)
	GetTable(ctx context.Context, organizationID, dataSourceID, tableID string, accountID string) (*datasourcemodel.Table, error)
	GetTableColumns(ctx context.Context, organizationID, dataSourceID, tableID string, includeSystemFields bool) (dto.GetTableColumnsResponse, error)
	QueryTableRecords(ctx context.Context, organizationID, dataSourceID, tableID, accountID string, limit, offset int, order string) (dto.QueryRecordResponse, error)
	AddTableRecords(ctx context.Context, organizationID, dataSourceID, tableID, accountID string, req dto.AddRecordRequest) (dto.AddRecordResponse, error)
	UpdateTableRecords(ctx context.Context, organizationID, dataSourceID, tableID, accountID string, req dto.UpdateRecordRequest) (dto.UpdateRecordResponse, error)
	DeleteTableRecords(ctx context.Context, organizationID, dataSourceID, tableID, accountID string, req dto.DeleteRecordRequest) (dto.DeleteRecordResponse, error)
}

type OrganizationService interface {
	CheckWorkspacePermission(ctx context.Context, organizationID, workspaceID, accountID string, permissionCode workspacemodel.WorkspacePermissionCode) (bool, error)
	CheckWorkspaceOrganizationAnyPermission(ctx context.Context, organizationID, workspaceID, accountID string, permissionCodes ...workspacemodel.WorkspacePermissionCode) (bool, error)
}

type Provider struct {
	*builtin.BuiltinProvider
	dataSources  DataSourceService
	organization OrganizationService
}

func NewProvider(dataSources DataSourceService, organization OrganizationService) *Provider {
	identity := tools.ToolProviderIdentity{
		Name:   ProviderID,
		Author: "System",
		Label: tools.I18nText{
			"en_US":   "Database Tools",
			"zh_Hans": "数据库工具",
		},
		Description: tools.I18nText{
			"en_US":   "Built-in tools for structured database table operations.",
			"zh_Hans": "用于结构化数据库表操作的内置工具。",
		},
		Icon: "database",
		Tags: []string{"database", "system"},
	}
	provider := &Provider{
		BuiltinProvider: builtin.NewBuiltinProvider(identity),
		dataSources:     dataSources,
		organization:    organization,
	}
	for _, name := range []string{
		ToolListAccessibleDatabases,
		ToolListDatabaseTables,
		ToolDescribeDatabaseTable,
		ToolQueryTableRecords,
		ToolInsertTableRecords,
		ToolUpdateTableRecords,
		ToolDeleteTableRecords,
	} {
		provider.RegisterTool(newDatabaseTool(dataSources, organization, name))
	}
	return provider
}

type databaseTool struct {
	*builtin.BuiltinTool
	dataSources  DataSourceService
	organization OrganizationService
	kind         string
}

func newDatabaseTool(dataSources DataSourceService, organization OrganizationService, kind string) tools.Tool {
	entity := tools.ToolEntity{
		Identity: tools.ToolIdentity{
			Name:     kind,
			Author:   "System",
			Provider: ProviderID,
			Label:    tools.I18nText{"en_US": databaseToolLabel(kind), "zh_Hans": databaseToolLabel(kind)},
			Icon:     "database",
		},
		Description: tools.ToolDescription{
			Human: tools.I18nText{"en_US": databaseToolDescription(kind), "zh_Hans": databaseToolDescription(kind)},
			LLM:   databaseToolDescription(kind),
		},
		Parameters: databaseToolParameters(kind),
		OutputType: "json",
		Tags:       []string{"database", "system"},
	}
	return &databaseTool{
		BuiltinTool:  builtin.NewBuiltinTool(entity, ""),
		dataSources:  dataSources,
		organization: organization,
		kind:         kind,
	}
}

func (t *databaseTool) Invoke(ctx context.Context, userID string, params map[string]interface{}, conversationID *string, appID *string, messageID *string) ([]tools.ToolInvokeMessage, error) {
	_ = conversationID
	_ = appID
	_ = messageID
	if t.dataSources == nil {
		return nil, fmt.Errorf("database service is not configured")
	}
	scope, err := t.scope(userID)
	if err != nil {
		return nil, err
	}
	bindings, err := t.agentBindings()
	if err != nil {
		return nil, err
	}

	switch t.kind {
	case ToolListAccessibleDatabases:
		return t.listAccessibleDatabases(ctx, scope, params, bindings)
	case ToolListDatabaseTables:
		return t.listDatabaseTables(ctx, scope, params, bindings)
	case ToolDescribeDatabaseTable:
		return t.describeDatabaseTable(ctx, scope, params, bindings)
	case ToolQueryTableRecords:
		return t.queryTableRecords(ctx, scope, params, bindings)
	case ToolInsertTableRecords:
		return t.insertTableRecords(ctx, scope, params, bindings)
	case ToolUpdateTableRecords:
		return t.updateTableRecords(ctx, scope, params, bindings)
	case ToolDeleteTableRecords:
		return t.deleteTableRecords(ctx, scope, params, bindings)
	default:
		return nil, fmt.Errorf("unknown database tool %s", t.kind)
	}
}

func (t *databaseTool) ForkToolRuntime(runtime *tools.ToolRuntime) tools.Tool {
	return &databaseTool{
		BuiltinTool:  t.BuiltinTool.ForkToolRuntime(runtime),
		dataSources:  t.dataSources,
		organization: t.organization,
		kind:         t.kind,
	}
}

type databaseScope struct {
	OrganizationID string
	WorkspaceID    string
	AccountID      string
	InvokeFrom     tools.ToolInvokeFrom
	BindingGrant   bool
}

func (t *databaseTool) scope(userID string) (databaseScope, error) {
	runtime := t.Runtime()
	tenantID := strings.TrimSpace(t.GetTenantID())
	if runtime != nil && strings.TrimSpace(runtime.TenantID) != "" {
		tenantID = strings.TrimSpace(runtime.TenantID)
	}
	organizationID := ""
	workspaceID := ""
	accountID := strings.TrimSpace(userID)
	if runtime != nil {
		organizationID = strings.TrimSpace(stringValue(runtime.RuntimeParameters, "organization_id"))
		workspaceID = strings.TrimSpace(stringValue(runtime.RuntimeParameters, "workspace_id"))
		if runtime.InvokeFrom == tools.ToolInvokeFromAgent {
			if boundBy := strings.TrimSpace(stringValue(runtime.RuntimeParameters, "database_bound_by_account_id")); boundBy != "" {
				accountID = boundBy
			}
		}
	}
	if accountID == "" {
		return databaseScope{}, fmt.Errorf("account_id is required")
	}
	if organizationID == "" && workspaceID == "" {
		if runtime != nil && runtime.InvokeFrom == tools.ToolInvokeFromAIChat {
			organizationID = tenantID
		} else {
			workspaceID = tenantID
		}
	}
	if organizationID == "" && workspaceID == "" {
		return databaseScope{}, fmt.Errorf("organization_id or workspace_id is required")
	}
	invokeFrom := tools.ToolInvokeFromAIChat
	if runtime != nil && runtime.InvokeFrom != "" {
		invokeFrom = runtime.InvokeFrom
	}
	return databaseScope{
		OrganizationID: organizationID,
		WorkspaceID:    workspaceID,
		AccountID:      accountID,
		InvokeFrom:     invokeFrom,
		BindingGrant:   runtime != nil && runtime.InvokeFrom == tools.ToolInvokeFromAgent && boolValue(runtime.RuntimeParameters, "database_binding_grant"),
	}, nil
}

type databaseBindingSet struct {
	Readable map[string]struct{}
	Writable map[string]struct{}
}

type databaseBindings map[string]databaseBindingSet

func (t *databaseTool) agentBindings() (databaseBindings, error) {
	runtime := t.Runtime()
	if runtime == nil || runtime.InvokeFrom != tools.ToolInvokeFromAgent {
		return nil, nil
	}
	raw, ok := runtime.RuntimeParameters["database_bindings"]
	if !ok || raw == nil {
		return databaseBindings{}, nil
	}
	payload, err := json.Marshal(raw)
	if err != nil {
		return nil, fmt.Errorf("invalid database bindings: %w", err)
	}
	var parsed []struct {
		DataSourceID     string   `json:"data_source_id"`
		TableIDs         []string `json:"table_ids"`
		WritableTableIDs []string `json:"writable_table_ids"`
	}
	if err := json.Unmarshal(payload, &parsed); err != nil {
		return nil, fmt.Errorf("invalid database bindings: %w", err)
	}
	out := databaseBindings{}
	for _, binding := range parsed {
		dataSourceID := strings.TrimSpace(binding.DataSourceID)
		if dataSourceID == "" {
			continue
		}
		if _, ok := out[dataSourceID]; !ok {
			out[dataSourceID] = databaseBindingSet{Readable: map[string]struct{}{}, Writable: map[string]struct{}{}}
		}
		set := out[dataSourceID]
		for _, rawTableID := range binding.TableIDs {
			tableID := strings.TrimSpace(rawTableID)
			if tableID != "" {
				set.Readable[tableID] = struct{}{}
			}
		}
		for _, rawTableID := range binding.WritableTableIDs {
			tableID := strings.TrimSpace(rawTableID)
			if tableID != "" {
				if _, ok := set.Readable[tableID]; ok {
					set.Writable[tableID] = struct{}{}
				}
			}
		}
		out[dataSourceID] = set
	}
	return out, nil
}

func (b databaseBindings) dataSourceAllowed(dataSourceID string) bool {
	if b == nil {
		return true
	}
	set, ok := b[strings.TrimSpace(dataSourceID)]
	return ok && len(set.Readable) > 0
}

func (b databaseBindings) tableAllowed(dataSourceID string, tableID string) bool {
	if b == nil {
		return true
	}
	set, ok := b[strings.TrimSpace(dataSourceID)]
	if !ok {
		return false
	}
	_, ok = set.Readable[strings.TrimSpace(tableID)]
	return ok
}

func (b databaseBindings) tableWritable(dataSourceID string, tableID string) bool {
	if b == nil {
		return true
	}
	set, ok := b[strings.TrimSpace(dataSourceID)]
	if !ok {
		return false
	}
	_, ok = set.Writable[strings.TrimSpace(tableID)]
	return ok
}

func (b databaseBindings) dataSourceIDs() []string {
	ids := make([]string, 0, len(b))
	for id, set := range b {
		if strings.TrimSpace(id) != "" && len(set.Readable) > 0 {
			ids = append(ids, id)
		}
	}
	sort.Strings(ids)
	return ids
}

func (t *databaseTool) listAccessibleDatabases(ctx context.Context, scope databaseScope, params map[string]interface{}, bindings databaseBindings) ([]tools.ToolInvokeMessage, error) {
	if scope.InvokeFrom == tools.ToolInvokeFromAgent && len(bindings) == 0 {
		return jsonMessages(map[string]interface{}{"databases": []interface{}{}})
	}
	query := strings.ToLower(stringValue(params, "query"))
	limit := boundedInt(params, "limit", defaultDatabaseListLimit, defaultDatabaseListLimit)
	var items []*dto.DataSourceResponse
	var err error
	if scope.BindingGrant {
		for _, dataSourceID := range bindings.dataSourceIDs() {
			item, loadErr := t.dataSources.GetDataSourceByID(ctx, scope.OrganizationID, dataSourceID, scope.AccountID)
			if loadErr != nil || item == nil {
				continue
			}
			items = append(items, item)
		}
	} else {
		items, err = t.dataSources.ListDataSources(ctx, scope.OrganizationID, scope.AccountID, nil)
	}
	if err != nil {
		return nil, err
	}
	out := make([]map[string]interface{}, 0, len(items))
	for _, item := range items {
		if item == nil || !bindings.dataSourceAllowed(item.ID) {
			continue
		}
		if strings.TrimSpace(item.OrganizationID) != "" && strings.TrimSpace(scope.OrganizationID) != "" && strings.TrimSpace(item.OrganizationID) != strings.TrimSpace(scope.OrganizationID) {
			continue
		}
		if query != "" && !containsFold(item.Name, item.Description, query) {
			continue
		}
		if !scope.BindingGrant {
			if err := t.authorizeDataSource(ctx, scope, item, false); err != nil {
				continue
			}
		}
		out = append(out, dataSourcePayload(item))
		if len(out) >= limit {
			break
		}
	}
	return jsonMessages(map[string]interface{}{"databases": out})
}

func (t *databaseTool) listDatabaseTables(ctx context.Context, scope databaseScope, params map[string]interface{}, bindings databaseBindings) ([]tools.ToolInvokeMessage, error) {
	dataSource, err := t.authorizedDataSourceFromParams(ctx, scope, params, false)
	if err != nil {
		return nil, err
	}
	if !bindings.dataSourceAllowed(dataSource.ID) {
		return nil, fmt.Errorf("database %s is not bound to the current agent", dataSource.ID)
	}
	query := strings.ToLower(stringValue(params, "query"))
	limit := boundedInt(params, "limit", defaultTableListLimit, defaultTableListLimit)
	tables, err := t.dataSources.ListTables(ctx, scope.OrganizationID, dataSource.ID, scope.AccountID)
	if err != nil {
		return nil, err
	}
	out := make([]map[string]interface{}, 0, len(tables))
	for _, table := range tables {
		if table == nil || !bindings.tableAllowed(dataSource.ID, table.ID) {
			continue
		}
		if query != "" && !containsFold(table.Name, table.Description, query) {
			continue
		}
		out = append(out, tablePayload(table))
		if len(out) >= limit {
			break
		}
	}
	return jsonMessages(map[string]interface{}{"data_source": dataSourcePayload(dataSource), "tables": out})
}

func (t *databaseTool) describeDatabaseTable(ctx context.Context, scope databaseScope, params map[string]interface{}, bindings databaseBindings) ([]tools.ToolInvokeMessage, error) {
	dataSource, table, err := t.authorizedTableFromParams(ctx, scope, params, bindings, false)
	if err != nil {
		return nil, err
	}
	columns, err := t.dataSources.GetTableColumns(ctx, scope.OrganizationID, dataSource.ID, table.ID, boolValue(params, "include_system_fields"))
	if err != nil {
		return nil, err
	}
	return jsonMessages(map[string]interface{}{
		"data_source": dataSourcePayload(dataSource),
		"table":       tablePayload(table),
		"columns":     columns.Columns,
	})
}

func (t *databaseTool) queryTableRecords(ctx context.Context, scope databaseScope, params map[string]interface{}, bindings databaseBindings) ([]tools.ToolInvokeMessage, error) {
	dataSource, table, err := t.authorizedTableFromParams(ctx, scope, params, bindings, false)
	if err != nil {
		return nil, err
	}
	limit := boundedInt(params, "limit", defaultRecordLimit, maxRecordLimit)
	offset := boundedInt(params, "offset", 0, 1000000)
	order := normalizeOrder(stringValue(params, "order"))
	result, err := t.dataSources.QueryTableRecords(ctx, scope.OrganizationID, dataSource.ID, table.ID, scope.AccountID, limit, offset, order)
	if err != nil {
		return nil, err
	}
	return jsonMessages(map[string]interface{}{
		"data_source": dataSourcePayload(dataSource),
		"table":       tablePayload(table),
		"records":     result.Data,
		"has_more":    result.HasMore,
		"total_num":   result.TotalNum,
	})
}

func (t *databaseTool) insertTableRecords(ctx context.Context, scope databaseScope, params map[string]interface{}, bindings databaseBindings) ([]tools.ToolInvokeMessage, error) {
	dataSource, table, records, err := t.authorizedMutationFromParams(ctx, scope, params, bindings)
	if err != nil {
		return nil, err
	}
	result, err := t.dataSources.AddTableRecords(ctx, scope.OrganizationID, dataSource.ID, table.ID, scope.AccountID, dto.AddRecordRequest{Records: records})
	if err != nil {
		return nil, err
	}
	return jsonMessages(mutationPayload(dataSource, table, result.AffectedRows))
}

func (t *databaseTool) updateTableRecords(ctx context.Context, scope databaseScope, params map[string]interface{}, bindings databaseBindings) ([]tools.ToolInvokeMessage, error) {
	dataSource, table, records, err := t.authorizedMutationFromParams(ctx, scope, params, bindings)
	if err != nil {
		return nil, err
	}
	result, err := t.dataSources.UpdateTableRecords(ctx, scope.OrganizationID, dataSource.ID, table.ID, scope.AccountID, dto.UpdateRecordRequest{Records: records})
	if err != nil {
		return nil, err
	}
	return jsonMessages(mutationPayload(dataSource, table, result.AffectedRows))
}

func (t *databaseTool) deleteTableRecords(ctx context.Context, scope databaseScope, params map[string]interface{}, bindings databaseBindings) ([]tools.ToolInvokeMessage, error) {
	dataSource, table, records, err := t.authorizedMutationFromParams(ctx, scope, params, bindings)
	if err != nil {
		return nil, err
	}
	result, err := t.dataSources.DeleteTableRecords(ctx, scope.OrganizationID, dataSource.ID, table.ID, scope.AccountID, dto.DeleteRecordRequest{Records: records})
	if err != nil {
		return nil, err
	}
	return jsonMessages(mutationPayload(dataSource, table, result.AffectedRows))
}

func (t *databaseTool) authorizedMutationFromParams(ctx context.Context, scope databaseScope, params map[string]interface{}, bindings databaseBindings) (*dto.DataSourceResponse, *datasourcemodel.Table, []map[string]interface{}, error) {
	dataSource, table, err := t.authorizedTableFromParams(ctx, scope, params, bindings, true)
	if err != nil {
		return nil, nil, nil, err
	}
	records, err := recordsValue(params, "records")
	if err != nil {
		return nil, nil, nil, err
	}
	return dataSource, table, records, nil
}

func (t *databaseTool) authorizedDataSourceFromParams(ctx context.Context, scope databaseScope, params map[string]interface{}, write bool) (*dto.DataSourceResponse, error) {
	dataSourceID := stringValue(params, "data_source_id")
	if dataSourceID == "" {
		return nil, fmt.Errorf("data_source_id is required")
	}
	dataSource, err := t.dataSources.GetDataSourceByID(ctx, scope.OrganizationID, dataSourceID, scope.AccountID)
	if err != nil {
		return nil, err
	}
	if dataSource == nil || strings.TrimSpace(dataSource.OrganizationID) != strings.TrimSpace(scope.OrganizationID) {
		return nil, fmt.Errorf("database %s not found", dataSourceID)
	}
	if !scope.BindingGrant {
		if err := t.authorizeDataSource(ctx, scope, dataSource, write); err != nil {
			return nil, err
		}
	}
	return dataSource, nil
}

func (t *databaseTool) authorizedTableFromParams(ctx context.Context, scope databaseScope, params map[string]interface{}, bindings databaseBindings, write bool) (*dto.DataSourceResponse, *datasourcemodel.Table, error) {
	dataSource, err := t.authorizedDataSourceFromParams(ctx, scope, params, write)
	if err != nil {
		return nil, nil, err
	}
	tableID := stringValue(params, "table_id")
	if tableID == "" {
		return nil, nil, fmt.Errorf("table_id is required")
	}
	if write && scope.BindingGrant {
		if !bindings.tableWritable(dataSource.ID, tableID) {
			return nil, nil, fmt.Errorf("table %s is not writable by the current agent binding", tableID)
		}
	} else if !bindings.tableAllowed(dataSource.ID, tableID) {
		return nil, nil, fmt.Errorf("table %s is not bound to the current agent", tableID)
	}
	table, err := t.dataSources.GetTable(ctx, scope.OrganizationID, dataSource.ID, tableID, scope.AccountID)
	if err != nil {
		return nil, nil, err
	}
	if table == nil || strings.TrimSpace(table.DataSourceID) != dataSource.ID {
		return nil, nil, fmt.Errorf("table %s not found in database %s", tableID, dataSource.ID)
	}
	return dataSource, table, nil
}

func (t *databaseTool) authorizeDataSource(ctx context.Context, scope databaseScope, dataSource *dto.DataSourceResponse, write bool) error {
	if t.organization == nil {
		return nil
	}
	workspaceID := strings.TrimSpace(scope.WorkspaceID)
	if dataSource != nil && dataSource.WorkspaceID != nil && strings.TrimSpace(*dataSource.WorkspaceID) != "" {
		workspaceID = strings.TrimSpace(*dataSource.WorkspaceID)
	}
	if workspaceID == "" {
		workspaceID = scope.OrganizationID
	}
	hasAIQuery, err := t.organization.CheckWorkspacePermission(ctx, scope.OrganizationID, workspaceID, scope.AccountID, workspacemodel.WorkspacePermissionDatabaseAIQuery)
	if err != nil {
		return err
	}
	if !hasAIQuery {
		return fmt.Errorf("database ai query permission is required")
	}
	if !write {
		hasView, err := t.organization.CheckWorkspacePermission(ctx, scope.OrganizationID, workspaceID, scope.AccountID, workspacemodel.WorkspacePermissionDatabaseView)
		if err != nil {
			return err
		}
		if !hasView {
			return fmt.Errorf("database view permission is required")
		}
		return nil
	}
	canWrite, err := t.organization.CheckWorkspaceOrganizationAnyPermission(ctx, scope.OrganizationID, workspaceID, scope.AccountID, workspacemodel.WorkspacePermissionDatabaseDataEdit, workspacemodel.WorkspacePermissionDatabaseManage)
	if err != nil {
		return err
	}
	if !canWrite {
		return fmt.Errorf("database data edit or manage permission is required")
	}
	return nil
}

func dataSourcePayload(item *dto.DataSourceResponse) map[string]interface{} {
	return map[string]interface{}{
		"data_source_id":  item.ID,
		"name":            item.Name,
		"description":     item.Description,
		"workspace_id":    item.WorkspaceID,
		"schema_name":     item.SchemaName,
		"status":          item.Status,
		"permission":      item.Permission,
		"icon":            item.Icon,
		"icon_type":       item.IconType,
		"icon_background": item.IconBackground,
	}
}

func tablePayload(item *datasourcemodel.Table) map[string]interface{} {
	return map[string]interface{}{
		"table_id":            item.ID,
		"data_source_id":      item.DataSourceID,
		"name":                item.Name,
		"description":         item.Description,
		"physical_table_id":   item.TableID,
		"physical_table_name": item.PhysicalTableName,
	}
}

func mutationPayload(dataSource *dto.DataSourceResponse, table *datasourcemodel.Table, affectedRows int64) map[string]interface{} {
	return map[string]interface{}{
		"data_source":   dataSourcePayload(dataSource),
		"table":         tablePayload(table),
		"affected_rows": affectedRows,
	}
}

func jsonMessages(payload map[string]interface{}) ([]tools.ToolInvokeMessage, error) {
	return []tools.ToolInvokeMessage{builtin.CreateJSONMessage(payload)}, nil
}

func databaseToolLabel(kind string) string {
	switch kind {
	case ToolListAccessibleDatabases:
		return "List Accessible Databases"
	case ToolListDatabaseTables:
		return "List Database Tables"
	case ToolDescribeDatabaseTable:
		return "Describe Database Table"
	case ToolQueryTableRecords:
		return "Query Table Records"
	case ToolInsertTableRecords:
		return "Insert Table Records"
	case ToolUpdateTableRecords:
		return "Update Table Records"
	case ToolDeleteTableRecords:
		return "Delete Table Records"
	default:
		return kind
	}
}

func databaseToolDescription(kind string) string {
	switch kind {
	case ToolListAccessibleDatabases:
		return "List databases accessible to the current user, or databases bound to the current Agent."
	case ToolListDatabaseTables:
		return "List tables in an accessible database."
	case ToolDescribeDatabaseTable:
		return "Describe a table and its columns."
	case ToolQueryTableRecords:
		return "Query table records with pagination. Does not accept SQL."
	case ToolInsertTableRecords:
		return "Insert records into a writable database table. Does not accept SQL."
	case ToolUpdateTableRecords:
		return "Update records in a writable database table. Does not accept SQL."
	case ToolDeleteTableRecords:
		return "Delete records from a writable database table. Does not accept SQL."
	default:
		return kind
	}
}

func databaseToolParameters(kind string) []tools.ToolParameter {
	dataSourceID := stringParam("data_source_id", "Database ID", "Database ID returned by list_accessible_databases.", true)
	tableID := stringParam("table_id", "Table ID", "Table metadata ID returned by list_database_tables.", true)
	query := stringParam("query", "Query", "Optional search text.", false)
	limit := numberParam("limit", "Limit", "Maximum items to return.")
	offset := numberParam("offset", "Offset", "Pagination offset.")
	order := stringParam("order", "Order", "Safe order clause such as id DESC.", false)
	includeSystemFields := boolParam("include_system_fields", "Include system fields", "Whether to include system fields.")
	records := jsonParam("records", "Records", "Array of record objects. Update and delete records must include id.", true)

	switch kind {
	case ToolListAccessibleDatabases:
		return []tools.ToolParameter{query, limit}
	case ToolListDatabaseTables:
		return []tools.ToolParameter{dataSourceID, query, limit}
	case ToolDescribeDatabaseTable:
		return []tools.ToolParameter{dataSourceID, tableID, includeSystemFields}
	case ToolQueryTableRecords:
		return []tools.ToolParameter{dataSourceID, tableID, limit, offset, order}
	case ToolInsertTableRecords, ToolUpdateTableRecords, ToolDeleteTableRecords:
		return []tools.ToolParameter{dataSourceID, tableID, records}
	default:
		return nil
	}
}

func stringParam(name, label, description string, required bool) tools.ToolParameter {
	return tools.ToolParameter{
		Name:            name,
		Label:           tools.I18nText{"en_US": label, "zh_Hans": label},
		LLMDescription:  description,
		Type:            tools.ToolParameterTypeString,
		Form:            tools.ToolParameterFormLLM,
		Required:        required,
		SupportVariable: true,
	}
}

func numberParam(name, label, description string) tools.ToolParameter {
	return tools.ToolParameter{
		Name:            name,
		Label:           tools.I18nText{"en_US": label, "zh_Hans": label},
		LLMDescription:  description,
		Type:            tools.ToolParameterTypeNumber,
		Form:            tools.ToolParameterFormLLM,
		SupportVariable: true,
	}
}

func boolParam(name, label, description string) tools.ToolParameter {
	return tools.ToolParameter{
		Name:            name,
		Label:           tools.I18nText{"en_US": label, "zh_Hans": label},
		LLMDescription:  description,
		Type:            tools.ToolParameterTypeBoolean,
		Form:            tools.ToolParameterFormLLM,
		SupportVariable: true,
	}
}

func jsonParam(name, label, description string, required bool) tools.ToolParameter {
	return tools.ToolParameter{
		Name:            name,
		Label:           tools.I18nText{"en_US": label, "zh_Hans": label},
		LLMDescription:  description,
		Type:            tools.ToolParameterTypeString,
		Form:            tools.ToolParameterFormLLM,
		Required:        required,
		SupportVariable: true,
	}
}

func stringValue(params map[string]interface{}, key string) string {
	if params == nil {
		return ""
	}
	value, ok := params[key]
	if !ok || value == nil {
		return ""
	}
	switch typed := value.(type) {
	case string:
		return strings.TrimSpace(typed)
	case fmt.Stringer:
		return strings.TrimSpace(typed.String())
	default:
		return strings.TrimSpace(fmt.Sprint(value))
	}
}

func boolValue(params map[string]interface{}, key string) bool {
	if params == nil {
		return false
	}
	value, ok := params[key]
	if !ok || value == nil {
		return false
	}
	switch typed := value.(type) {
	case bool:
		return typed
	case string:
		parsed, _ := strconv.ParseBool(strings.TrimSpace(typed))
		return parsed
	default:
		return false
	}
}

func boundedInt(params map[string]interface{}, key string, defaultValue int, maxValue int) int {
	value := intValue(params, key, defaultValue)
	if value < 0 {
		return defaultValue
	}
	if maxValue > 0 && value > maxValue {
		return maxValue
	}
	return value
}

func intValue(params map[string]interface{}, key string, defaultValue int) int {
	if params == nil {
		return defaultValue
	}
	value, ok := params[key]
	if !ok || value == nil {
		return defaultValue
	}
	switch typed := value.(type) {
	case int:
		return typed
	case int64:
		return int(typed)
	case float64:
		return int(typed)
	case json.Number:
		parsed, err := strconv.Atoi(typed.String())
		if err == nil {
			return parsed
		}
	case string:
		parsed, err := strconv.Atoi(strings.TrimSpace(typed))
		if err == nil {
			return parsed
		}
	}
	return defaultValue
}

func recordsValue(params map[string]interface{}, key string) ([]map[string]interface{}, error) {
	value, ok := params[key]
	if !ok || value == nil {
		return nil, fmt.Errorf("%s is required", key)
	}
	if raw, ok := value.(string); ok {
		trimmed := strings.TrimSpace(raw)
		if trimmed == "" {
			return nil, fmt.Errorf("%s is required", key)
		}
		value = json.RawMessage(trimmed)
	}
	payload, err := json.Marshal(value)
	if err != nil {
		return nil, fmt.Errorf("invalid records: %w", err)
	}
	var records []map[string]interface{}
	if err := json.Unmarshal(payload, &records); err != nil {
		return nil, fmt.Errorf("invalid records: %w", err)
	}
	if len(records) == 0 {
		return nil, fmt.Errorf("records must not be empty")
	}
	return records, nil
}

func normalizeOrder(raw string) string {
	parts := strings.Fields(strings.ToLower(strings.TrimSpace(raw)))
	if len(parts) != 2 {
		return "id DESC"
	}
	column := parts[0]
	direction := strings.ToUpper(parts[1])
	switch column {
	case "id", "created_time", "updated_time":
	default:
		return "id DESC"
	}
	switch direction {
	case "ASC", "DESC":
	default:
		return "id DESC"
	}
	return column + " " + direction
}

func containsFold(name string, description string, query string) bool {
	query = strings.ToLower(strings.TrimSpace(query))
	return strings.Contains(strings.ToLower(name), query) || strings.Contains(strings.ToLower(description), query)
}

func sortedKeys(values map[string]map[string]struct{}) []string {
	keys := make([]string, 0, len(values))
	for key := range values {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return keys
}

var _ tools.ToolProvider = (*Provider)(nil)
var _ tools.Tool = (*databaseTool)(nil)
