package sql_base

import (
	"context"
	"errors"
	"fmt"
	"sort"

	"github.com/samber/do/v2"

	"github.com/zgiai/ginext/pkg/logger"
	"github.com/zgiai/ginext/pkg/sql_base/sqlmeta/driver"

	"github.com/zgiai/ginext/pkg/sql_base/sqlmeta/catalog/columns"
	"github.com/zgiai/ginext/pkg/sql_base/sqlmeta/catalog/query"
	"github.com/zgiai/ginext/pkg/sql_base/sqlmeta/catalog/schemas"
	"github.com/zgiai/ginext/pkg/sql_base/sqlmeta/catalog/tables"

	metaService "github.com/zgiai/ginext/pkg/sql_base/sqlmeta/service"
	metaTypes "github.com/zgiai/ginext/pkg/sql_base/sqlmeta/types"
)

type internalClient struct {
	injector       do.Injector
	pool           *driver.Pool
	schemasService metaService.SchemasService
	tablesService  metaService.TablesService
	columnsService metaService.ColumnsService
	queryService   metaService.QueryService
}

func NewInternalClient(host, port, user, password, dbname string) (SQLBase, error) {
	cfg := driver.Config{
		DBHost: host,
		DBPort: port,
		DBUser: user,
		DBPass: password,
		DBName: dbname,
	}

	if cfg.DBHost == "" || cfg.DBPort == "" || cfg.DBUser == "" || cfg.DBName == "" {
		return nil, fmt.Errorf("database info are required")
	}

	ctx := context.Background()

	pool, err := driver.NewPool(ctx, cfg)
	if err != nil {
		if err := ensureDatabaseExists(ctx, host, port, user, password, dbname); err != nil {
			return nil, fmt.Errorf("sqlmeta: ensure database exists: %w", err)
		}

		pool, err = driver.NewPool(ctx, cfg)
		if err != nil {
			return nil, fmt.Errorf("sqlmeta: create pool after database creation: %w", err)
		}
	}

	injector := do.New()
	do.ProvideValue(injector, pool)
	do.Provide(injector, schemas.ProvideRepository)
	do.Provide(injector, tables.ProvideRepository)
	do.Provide(injector, columns.ProvideRepository)
	do.Provide(injector, query.ProvideRepository)
	do.Provide(injector, metaService.ProvideSchemasService)
	do.Provide(injector, metaService.ProvideTablesService)
	do.Provide(injector, metaService.ProvideColumnsService)
	do.Provide(injector, metaService.ProvideQueryService)

	schemasSvc := do.MustInvoke[metaService.SchemasService](injector)
	tablesSvc := do.MustInvoke[metaService.TablesService](injector)
	columnsSvc := do.MustInvoke[metaService.ColumnsService](injector)
	querySvc := do.MustInvoke[metaService.QueryService](injector)

	return &internalClient{
		injector:       injector,
		pool:           pool,
		schemasService: schemasSvc,
		tablesService:  tablesSvc,
		columnsService: columnsSvc,
		queryService:   querySvc,
	}, nil
}

// ensureDatabaseExists
func ensureDatabaseExists(ctx context.Context, host, port, user, password, dbname string) error {
	defaultCfg := driver.Config{
		DBHost: host,
		DBPort: port,
		DBUser: user,
		DBPass: password,
		DBName: "postgres",
	}

	defaultPool, err := driver.NewPool(ctx, defaultCfg)
	if err != nil {
		return fmt.Errorf("failed to connect to default postgres database: %w", err)
	}
	defer defaultPool.Close()

	rawPool := defaultPool.Raw()
	if rawPool == nil {
		return fmt.Errorf("failed to get raw pool")
	}

	var exists bool
	checkQuery := `SELECT EXISTS(SELECT 1 FROM pg_database WHERE datname = $1)`
	err = rawPool.QueryRow(ctx, checkQuery, dbname).Scan(&exists)
	if err != nil {
		return fmt.Errorf("failed to check database existence: %w", err)
	}

	if exists {
		return nil
	}

	createQuery := fmt.Sprintf(`CREATE DATABASE "%s"`, dbname)
	_, err = rawPool.Exec(ctx, createQuery)
	if err != nil {
		return fmt.Errorf("failed to create database %s: %w", dbname, err)
	}

	logger.InfoContext(ctx, "sql base database created", "database", dbname)
	return nil
}

// Schema operations
func (c *internalClient) ListSchemas(ctx context.Context, opts ListSchemasOptions) ([]Schema, error) {
	if c.schemasService == nil {
		return nil, errors.New("schemas service not initialized")
	}

	metaOpts := metaTypes.SchemaListOptions{
		IncludeSystemSchemas: opts.IncludeSystemSchemas,
		Pagination: metaTypes.Pagination{
			Limit:  opts.Limit,
			Offset: opts.Offset,
		},
	}

	data, err := c.schemasService.List(ctx, metaOpts)
	if err != nil {
		return nil, err
	}

	out := make([]Schema, 0, len(data))
	for _, item := range data {
		out = append(out, Schema{
			ID:    int(item.ID),
			Name:  item.Name,
			Owner: item.Owner,
		})
	}
	return out, nil
}

func (c *internalClient) GetSchema(ctx context.Context, id int) (*Schema, error) {
	if c.schemasService == nil {
		return nil, errors.New("schemas service not initialized")
	}

	res, err := c.schemasService.Retrieve(ctx, metaTypes.SchemaIdentifier{ID: int64(id)})
	if err != nil {
		return nil, err
	}
	return &Schema{
		ID:    int(res.ID),
		Name:  res.Name,
		Owner: res.Owner,
	}, nil
}

func (c *internalClient) CreateSchema(ctx context.Context, schema CreateSchemaRequest) (*Schema, error) {
	if c.schemasService == nil {
		return nil, errors.New("schemas service not initialized")
	}

	input := metaTypes.SchemaCreateInput{
		Name:  schema.Name,
		Owner: schema.Owner,
	}
	res, err := c.schemasService.Create(ctx, input)
	if err != nil {
		return nil, err
	}
	return &Schema{
		ID:    int(res.ID),
		Name:  res.Name,
		Owner: res.Owner,
	}, nil
}

func (c *internalClient) UpdateSchema(ctx context.Context, id int, schema UpdateSchemaRequest) (*Schema, error) {
	if c.schemasService == nil {
		return nil, errors.New("schemas service not initialized")
	}

	input := metaTypes.SchemaUpdateInput{}
	if schema.Name != "" {
		input.Name = &schema.Name
	}
	if schema.Owner != "" {
		input.Owner = &schema.Owner
	}

	res, err := c.schemasService.Update(ctx, int64(id), input)
	if err != nil {
		return nil, err
	}

	return &Schema{
		ID:    int(res.ID),
		Name:  res.Name,
		Owner: res.Owner,
	}, nil
}

func (c *internalClient) DeleteSchema(ctx context.Context, id int, cascade bool) (*Schema, error) {
	if c.schemasService == nil {
		return nil, errors.New("schemas service not initialized")
	}

	res, err := c.schemasService.Delete(ctx, int64(id), metaTypes.SchemaDropOptions{Cascade: cascade})
	if err != nil {
		return nil, err
	}
	return &Schema{
		ID:    int(res.ID),
		Name:  res.Name,
		Owner: res.Owner,
	}, nil
}

// Table operations
func (c *internalClient) ListTables(ctx context.Context, opts ListTablesOptions) ([]Table, error) {
	if c.tablesService == nil {
		return nil, errors.New("tables service not initialized")
	}

	metaOpts := metaTypes.TableListOptions{
		IncludeSystemSchemas: opts.IncludeSystemSchemas,
		IncludedSchemas:      opts.IncludedSchemas,
		ExcludedSchemas:      opts.ExcludedSchemas,
		IncludeColumns:       opts.IncludeColumns,
		Pagination: metaTypes.Pagination{
			Limit:  opts.Limit,
			Offset: opts.Offset,
		},
	}

	data, err := c.tablesService.List(ctx, metaOpts)
	if err != nil {
		return nil, err
	}

	out := make([]Table, 0, len(data))
	for _, item := range data {
		out = append(out, convertMetaTable(item))
	}
	return out, nil
}

func (c *internalClient) GetTable(ctx context.Context, id int) (*Table, error) {
	if c.tablesService == nil {
		return nil, errors.New("tables service not initialized")
	}

	res, err := c.tablesService.Retrieve(ctx, metaTypes.TableIdentifier{ID: int64(id)}, true)
	if err != nil {
		return nil, err
	}

	table := convertMetaTable(*res)
	return &table, nil
}

func (c *internalClient) CreateTable(ctx context.Context, table CreateTableRequest) (*Table, error) {
	if c.tablesService == nil {
		return nil, errors.New("tables service not initialized")
	}

	input := metaTypes.TableCreateInput{
		Name:    table.Name,
		Schema:  table.Schema,
		Comment: table.Comment,
	}

	res, err := c.tablesService.Create(ctx, input)
	if err != nil {
		return nil, err
	}

	created := convertMetaTable(*res)
	return &created, nil
}

func (c *internalClient) UpdateTable(ctx context.Context, id int, table UpdateTableRequest) (*Table, error) {
	if c.tablesService == nil {
		return nil, errors.New("tables service not initialized")
	}

	input := metaTypes.TableUpdateInput{}
	if table.Name != "" {
		input.Name = &table.Name
	}
	if table.Schema != "" {
		input.Schema = &table.Schema
	}
	if table.RlsEnabled != nil {
		input.RLSEnabled = table.RlsEnabled
	}
	if table.RlsForced != nil {
		input.RLSForced = table.RlsForced
	}
	if table.ReplicaIdentity != nil {
		input.ReplicaIdentity = table.ReplicaIdentity
	}
	if table.ReplicaIdentityIndex != nil {
		input.ReplicaIdentityIndex = table.ReplicaIdentityIndex
	}
	if table.Comment != nil {
		input.Comment = table.Comment
	}

	res, err := c.tablesService.Update(ctx, int64(id), input)
	if err != nil {
		return nil, err
	}

	updated := convertMetaTable(*res)
	return &updated, nil
}

func (c *internalClient) DeleteTable(ctx context.Context, id int, cascade bool) (*Table, error) {
	if c.tablesService == nil {
		return nil, errors.New("tables service not initialized")
	}

	res, err := c.tablesService.Delete(ctx, int64(id), metaTypes.TableDeleteOptions{Cascade: cascade})
	if err != nil {
		return nil, err
	}

	deleted := convertMetaTable(*res)
	return &deleted, nil
}

// Column operations
func (c *internalClient) ListColumns(ctx context.Context, opts ListColumnsOptions) ([]Column, error) {
	if c.columnsService == nil {
		return nil, errors.New("columns service not initialized")
	}

	metaOpts := metaTypes.ColumnListOptions{
		IncludeSystemSchemas: opts.IncludeSystemSchemas,
		IncludedSchemas:      opts.IncludedSchemas,
		ExcludedSchemas:      opts.ExcludedSchemas,
		Pagination: metaTypes.Pagination{
			Limit:  opts.Limit,
			Offset: opts.Offset,
		},
	}

	data, err := c.columnsService.ListColumns(ctx, metaOpts)
	if err != nil {
		return nil, err
	}

	out := make([]Column, 0, len(data))
	for _, item := range data {
		out = append(out, convertMetaColumn(item))
	}
	return out, nil
}

func (c *internalClient) GetColumn(ctx context.Context, tableId int, ordinalPosition int) (*Column, error) {
	if c.columnsService == nil {
		return nil, errors.New("columns service not initialized")
	}

	identifier := metaTypes.ColumnIdentifier{ID: fmt.Sprintf("%d.%d", tableId, ordinalPosition)}

	res, err := c.columnsService.GetColumn(ctx, identifier)
	if err != nil {
		return nil, err
	}

	column := convertMetaColumn(*res)
	return &column, nil
}

func (c *internalClient) CreateColumn(ctx context.Context, column CreateColumnRequest) (*Column, error) {
	if c.columnsService == nil {
		return nil, errors.New("columns service not initialized")
	}

	input := metaTypes.ColumnCreateInput{
		TableID:         int64(column.TableID),
		Name:            column.Name,
		Type:            column.Type,
		DefaultValue:    column.DefaultValue,
		DefaultValueSet: column.DefaultValue != nil,
		IsIdentity:      column.IsIdentity,
		IsPrimaryKey:    column.IsPrimaryKey,
		IsUnique:        column.IsUnique,
		Comment:         column.Comment,
		Check:           column.Check,
	}
	if column.DefaultValueFormat != nil {
		input.DefaultValueFormat = *column.DefaultValueFormat
	}
	if column.IdentityGeneration != nil {
		input.IdentityGeneration = *column.IdentityGeneration
	}
	if column.IsNullable != nil {
		input.IsNullable = column.IsNullable
	}

	res, err := c.columnsService.CreateColumn(ctx, input)
	if err != nil {
		return nil, err
	}

	created := convertMetaColumn(*res)
	return &created, nil
}

func (c *internalClient) UpdateColumn(ctx context.Context, id string, column UpdateColumnRequest) (*Column, error) {
	if c.columnsService == nil {
		return nil, errors.New("columns service not initialized")
	}

	input := metaTypes.ColumnUpdateInput{}
	if column.Name != "" {
		input.Name = &column.Name
	}
	if column.Type != "" {
		input.Type = &column.Type
	}
	if column.DropDefault {
		input.DropDefault = true
		input.DropDefaultSet = true
	}
	if column.DefaultValue != nil {
		input.DefaultValue = column.DefaultValue
		input.DefaultValueSet = true
	}
	if column.DefaultValueFormat != nil {
		input.DefaultValueFormat = *column.DefaultValueFormat
		input.DefaultValueFormatSet = true
	}
	if column.IsIdentity {
		val := column.IsIdentity
		input.IsIdentity = &val
	}
	if column.IdentityGeneration != nil {
		input.IdentityGeneration = column.IdentityGeneration
		input.IdentityGenerationSet = true
	}
	if column.IsNullable != nil {
		input.IsNullable = column.IsNullable
	}
	if column.IsUnique {
		val := column.IsUnique
		input.IsUnique = &val
	}
	if column.Comment != nil {
		input.Comment = column.Comment
		input.CommentSet = true
	}
	if column.Check != nil {
		input.Check = column.Check
		input.CheckSet = true
	}

	res, err := c.columnsService.UpdateColumn(ctx, id, input)
	if err != nil {
		return nil, err
	}

	updated := convertMetaColumn(*res)
	return &updated, nil
}

func (c *internalClient) DeleteColumn(ctx context.Context, id string, cascade bool) (*Column, error) {
	if c.columnsService == nil {
		return nil, errors.New("columns service not initialized")
	}

	res, err := c.columnsService.DeleteColumn(ctx, id, cascade)
	if err != nil {
		return nil, err
	}

	deleted := convertMetaColumn(*res)
	return &deleted, nil
}

// View operations
func (c *internalClient) ListViews(ctx context.Context, opts ListViewsOptions) ([]View, error) {
	return nil, errors.New("internal postgres meta service is not implemented yet")
}

func (c *internalClient) GetView(ctx context.Context, id int) (*View, error) {
	return nil, errors.New("internal postgres meta service is not implemented yet")
}

// Materialized views operations
func (c *internalClient) ListMaterializedViews(ctx context.Context, opts ListMaterializedViewsOptions) ([]MaterializedView, error) {
	return nil, errors.New("internal postgres meta service is not implemented yet")
}

func (c *internalClient) GetMaterializedView(ctx context.Context, id int) (*MaterializedView, error) {
	return nil, errors.New("internal postgres meta service is not implemented yet")
}

// Function operations
func (c *internalClient) ListFunctions(ctx context.Context) ([]Function, error) {
	return nil, errors.New("internal postgres meta service is not implemented yet")
}

func (c *internalClient) GetFunction(ctx context.Context, id string) (*Function, error) {
	return nil, errors.New("internal postgres meta service is not implemented yet")
}

func (c *internalClient) CreateFunction(ctx context.Context, function Function) (*Function, error) {
	return nil, errors.New("internal postgres meta service is not implemented yet")
}

func (c *internalClient) UpdateFunction(ctx context.Context, id string, function Function) (*Function, error) {
	return nil, errors.New("internal postgres meta service is not implemented yet")
}

func (c *internalClient) DeleteFunction(ctx context.Context, id string) (*Function, error) {
	return nil, errors.New("internal postgres meta service is not implemented yet")
}

// Trigger operations
func (c *internalClient) ListTriggers(ctx context.Context) ([]Trigger, error) {
	return nil, errors.New("internal postgres meta service is not implemented yet")
}

func (c *internalClient) GetTrigger(ctx context.Context, id string) (*Trigger, error) {
	return nil, errors.New("internal postgres meta service is not implemented yet")
}

func (c *internalClient) CreateTrigger(ctx context.Context, trigger Trigger) (*Trigger, error) {
	return nil, errors.New("internal postgres meta service is not implemented yet")
}

func (c *internalClient) UpdateTrigger(ctx context.Context, id string, trigger Trigger) (*Trigger, error) {
	return nil, errors.New("internal postgres meta service is not implemented yet")
}

func (c *internalClient) DeleteTrigger(ctx context.Context, id string) (*Trigger, error) {
	return nil, errors.New("internal postgres meta service is not implemented yet")
}

// Role operations
func (c *internalClient) ListRoles(ctx context.Context, opts ListRolesOptions) ([]Role, error) {
	return nil, errors.New("internal postgres meta service is not implemented yet")
}

func (c *internalClient) GetRole(ctx context.Context, id int) (*Role, error) {
	return nil, errors.New("internal postgres meta service is not implemented yet")
}

func (c *internalClient) CreateRole(ctx context.Context, role CreateRoleRequest) (*Role, error) {
	return nil, errors.New("internal postgres meta service is not implemented yet")
}

func (c *internalClient) UpdateRole(ctx context.Context, id int, role UpdateRoleRequest) (*Role, error) {
	return nil, errors.New("internal postgres meta service is not implemented yet")
}

func (c *internalClient) DeleteRole(ctx context.Context, id int, cascade bool) (*Role, error) {
	return nil, errors.New("internal postgres meta service is not implemented yet")
}

// Extension operations
func (c *internalClient) ListExtensions(ctx context.Context) ([]Extension, error) {
	return nil, errors.New("internal postgres meta service is not implemented yet")
}

func (c *internalClient) GetExtension(ctx context.Context, name string) (*Extension, error) {
	return nil, errors.New("internal postgres meta service is not implemented yet")
}

func (c *internalClient) CreateExtension(ctx context.Context, extension Extension) (*Extension, error) {
	return nil, errors.New("internal postgres meta service is not implemented yet")
}

func (c *internalClient) UpdateExtension(ctx context.Context, name string, extension Extension) (*Extension, error) {
	return nil, errors.New("internal postgres meta service is not implemented yet")
}

func (c *internalClient) DeleteExtension(ctx context.Context, name string) (*Extension, error) {
	return nil, errors.New("internal postgres meta service is not implemented yet")
}

// Policy operations
func (c *internalClient) ListPolicies(ctx context.Context) ([]Policy, error) {
	return nil, errors.New("internal postgres meta service is not implemented yet")
}

func (c *internalClient) GetPolicy(ctx context.Context, id string) (*Policy, error) {
	return nil, errors.New("internal postgres meta service is not implemented yet")
}

func (c *internalClient) CreatePolicy(ctx context.Context, policy Policy) (*Policy, error) {
	return nil, errors.New("internal postgres meta service is not implemented yet")
}

func (c *internalClient) UpdatePolicy(ctx context.Context, id string, policy Policy) (*Policy, error) {
	return nil, errors.New("internal postgres meta service is not implemented yet")
}

func (c *internalClient) DeletePolicy(ctx context.Context, id string) (*Policy, error) {
	return nil, errors.New("internal postgres meta service is not implemented yet")
}

// Publication operations
func (c *internalClient) ListPublications(ctx context.Context) ([]Publication, error) {
	return nil, errors.New("internal postgres meta service is not implemented yet")
}

func (c *internalClient) GetPublication(ctx context.Context, id string) (*Publication, error) {
	return nil, errors.New("internal postgres meta service is not implemented yet")
}

func (c *internalClient) CreatePublication(ctx context.Context, publication Publication) (*Publication, error) {
	return nil, errors.New("internal postgres meta service is not implemented yet")
}

func (c *internalClient) UpdatePublication(ctx context.Context, id string, publication Publication) (*Publication, error) {
	return nil, errors.New("internal postgres meta service is not implemented yet")
}

func (c *internalClient) DeletePublication(ctx context.Context, id string) (*Publication, error) {
	return nil, errors.New("internal postgres meta service is not implemented yet")
}

// Foreign table operations
func (c *internalClient) ListForeignTables(ctx context.Context, opts ListForeignTablesOptions) ([]ForeignTable, error) {
	return nil, errors.New("internal postgres meta service is not implemented yet")
}

func (c *internalClient) GetForeignTable(ctx context.Context, id int) (*ForeignTable, error) {
	return nil, errors.New("internal postgres meta service is not implemented yet")
}

// Index operations
func (c *internalClient) ListIndexes(ctx context.Context) ([]Index, error) {
	return nil, errors.New("internal postgres meta service is not implemented yet")
}

func (c *internalClient) GetIndex(ctx context.Context, id string) (*Index, error) {
	return nil, errors.New("internal postgres meta service is not implemented yet")
}

// Type operations
func (c *internalClient) ListTypes(ctx context.Context) ([]Type, error) {
	return nil, errors.New("internal postgres meta service is not implemented yet")
}

// Query operations
func (c *internalClient) ExecuteSQL(ctx context.Context, query string, params []interface{}) (*QueryResult, error) {
	if c.queryService == nil {
		return nil, errors.New("query service not initialized")
	}

	opts := metaTypes.QueryOptions{}
	if len(params) > 0 {
		opts.Parameters = make([]any, 0, len(params))
		for _, p := range params {
			opts.Parameters = append(opts.Parameters, p)
		}
	}

	resp, err := c.queryService.Execute(ctx, query, opts)
	if err != nil {
		return nil, err
	}
	if resp == nil {
		return &QueryResult{
			RowsAffected: 0,
			Columns:      []string{},
			Rows:         [][]interface{}{},
		}, nil
	}
	if resp.Error != nil {
		message := resp.Error.Message
		if resp.Error.FormattedError != "" {
			message = resp.Error.FormattedError
		}
		if resp.Error.Code != "" {
			return nil, fmt.Errorf("sqlmeta: query failed (%s): %s", resp.Error.Code, message)
		}
		return nil, fmt.Errorf("sqlmeta: query failed: %s", message)
	}

	result := &QueryResult{
		RowsAffected: int64(len(resp.Data)),
		Columns:      []string{},
		Rows:         [][]interface{}{},
	}

	if len(resp.Data) == 0 {
		return result, nil
	}

	columns := make([]string, 0, len(resp.Data[0]))
	for col := range resp.Data[0] {
		columns = append(columns, col)
	}
	sort.Strings(columns)

	rows := make([][]interface{}, 0, len(resp.Data))
	for _, rowMap := range resp.Data {
		row := make([]interface{}, len(columns))
		for idx, col := range columns {
			row[idx] = rowMap[col]
		}
		rows = append(rows, row)
	}

	result.Columns = columns
	result.Rows = rows

	return result, nil
}

func (c *internalClient) FormatQuery(ctx context.Context, req FormatQueryRequest) (string, error) {
	return "", errors.New("internal postgres meta service is not implemented yet")
}

func (c *internalClient) ParseQuery(ctx context.Context, req ParseQueryRequest) (*ParsedQuery, error) {
	return nil, errors.New("internal postgres meta service is not implemented yet")
}

func (c *internalClient) DeparseQuery(ctx context.Context, req DeparseQueryRequest) (string, error) {
	return "", errors.New("internal postgres meta service is not implemented yet")
}

// Table privileges
func (c *internalClient) ListTablePrivileges(ctx context.Context, opts ListTablePrivilegesOptions) ([]TablePrivilege, error) {
	return nil, errors.New("internal postgres meta service is not implemented yet")
}

func (c *internalClient) GrantTablePrivileges(ctx context.Context, privileges []TablePrivilegeGrant) ([]TablePrivilege, error) {
	return nil, errors.New("internal postgres meta service is not implemented yet")
}

func (c *internalClient) RevokeTablePrivileges(ctx context.Context, privileges []TablePrivilegeRevoke) ([]TablePrivilege, error) {
	return nil, errors.New("internal postgres meta service is not implemented yet")
}

// Column privileges
func (c *internalClient) ListColumnPrivileges(ctx context.Context, opts ListColumnPrivilegesOptions) ([]ColumnPrivilege, error) {
	return nil, errors.New("internal postgres meta service is not implemented yet")
}

func (c *internalClient) GrantColumnPrivileges(ctx context.Context, privileges []ColumnPrivilegeGrant) ([]ColumnPrivilege, error) {
	return nil, errors.New("internal postgres meta service is not implemented yet")
}

func (c *internalClient) RevokeColumnPrivileges(ctx context.Context, privileges []ColumnPrivilegeRevoke) ([]ColumnPrivilege, error) {
	return nil, errors.New("internal postgres meta service is not implemented yet")
}

func convertMetaTable(tbl metaTypes.Table) Table {
	result := Table{
		ID:               int(tbl.ID),
		Schema:           tbl.Schema,
		Name:             tbl.Name,
		RlsEnabled:       tbl.RLSEnabled,
		RlsForced:        tbl.RLSForced,
		ReplicaIdentity:  tbl.ReplicaIdentity,
		Bytes:            int(tbl.Bytes),
		Size:             tbl.Size,
		LiveRowsEstimate: int(tbl.LiveRowsEstimate),
		DeadRowsEstimate: int(tbl.DeadRowsEstimate),
		Comment:          tbl.Comment,
		Columns:          make([]Column, 0, len(tbl.Columns)),
		PrimaryKeys:      convertMetaPrimaryKeys(tbl.PrimaryKeys),
		Relationships:    convertMetaRelationships(tbl.Relationships),
	}

	for _, col := range tbl.Columns {
		result.Columns = append(result.Columns, convertMetaColumn(col))
	}
	return result
}

func convertMetaPrimaryKeys(keys []metaTypes.PrimaryKey) []PrimaryKey {
	out := make([]PrimaryKey, 0, len(keys))
	for _, key := range keys {
		out = append(out, PrimaryKey{
			Schema:    key.Schema,
			TableName: key.TableName,
			Name:      key.Name,
			TableID:   int(key.TableID),
		})
	}
	return out
}

func convertMetaRelationships(rels []metaTypes.Relationship) []Relation {
	out := make([]Relation, 0, len(rels))
	for _, rel := range rels {
		out = append(out, Relation{
			ID:                int(rel.ID),
			ConstraintName:    rel.ConstraintName,
			SourceSchema:      rel.SourceSchema,
			SourceTableName:   rel.SourceTableName,
			SourceColumnName:  rel.SourceColumnName,
			TargetTableSchema: rel.TargetTableSchema,
			TargetTableName:   rel.TargetTableName,
			TargetColumnName:  rel.TargetColumnName,
		})
	}
	return out
}

func convertMetaColumn(col metaTypes.Column) Column {
	return Column{
		TableID:            int(col.TableID),
		Schema:             col.Schema,
		Table:              col.TableName,
		ID:                 col.ID,
		OrdinalPosition:    col.OrdinalPosition,
		Name:               col.Name,
		DefaultValue:       col.DefaultValue,
		DataType:           col.DataType,
		Format:             col.Format,
		IsIdentity:         col.IsIdentity,
		IdentityGeneration: col.Identity,
		IsGenerated:        col.IsGenerated,
		IsNullable:         col.IsNullable,
		IsUpdatable:        col.IsUpdatable,
		IsUnique:           col.IsUnique,
		Enums:              append([]string(nil), col.Enums...),
		Check:              col.Check,
		Comment:            col.Comment,
	}
}
