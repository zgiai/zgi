package sql_base

import (
	"context"
	"fmt"

	"github.com/zgiai/zgi/api/config"
	"github.com/zgiai/zgi/api/pkg/sql_base/audit"
)

// SQLBase defines the interface for postgres-meta service
type SQLBase interface {
	// Schema operations
	ListSchemas(ctx context.Context, opts ListSchemasOptions) ([]Schema, error)
	GetSchema(ctx context.Context, id int) (*Schema, error)
	CreateSchema(ctx context.Context, schema CreateSchemaRequest) (*Schema, error)
	UpdateSchema(ctx context.Context, id int, schema UpdateSchemaRequest) (*Schema, error)
	DeleteSchema(ctx context.Context, id int, cascade bool) (*Schema, error)

	// Table operations
	ListTables(ctx context.Context, opts ListTablesOptions) ([]Table, error)
	GetTable(ctx context.Context, id int) (*Table, error)
	CreateTable(ctx context.Context, table CreateTableRequest) (*Table, error)
	UpdateTable(ctx context.Context, id int, table UpdateTableRequest) (*Table, error)
	DeleteTable(ctx context.Context, id int, cascade bool) (*Table, error)

	// Column operations
	ListColumns(ctx context.Context, opts ListColumnsOptions) ([]Column, error)
	GetColumn(ctx context.Context, tableId int, ordinalPosition int) (*Column, error)
	CreateColumn(ctx context.Context, column CreateColumnRequest) (*Column, error)
	UpdateColumn(ctx context.Context, id string, column UpdateColumnRequest) (*Column, error)
	DeleteColumn(ctx context.Context, id string, cascade bool) (*Column, error)

	// View operations
	ListViews(ctx context.Context, opts ListViewsOptions) ([]View, error)
	GetView(ctx context.Context, id int) (*View, error)

	// Materialized views operations
	ListMaterializedViews(ctx context.Context, opts ListMaterializedViewsOptions) ([]MaterializedView, error)
	GetMaterializedView(ctx context.Context, id int) (*MaterializedView, error)

	// Function operations
	ListFunctions(ctx context.Context) ([]Function, error)
	GetFunction(ctx context.Context, id string) (*Function, error)
	CreateFunction(ctx context.Context, function Function) (*Function, error)
	UpdateFunction(ctx context.Context, id string, function Function) (*Function, error)
	DeleteFunction(ctx context.Context, id string) (*Function, error)

	// Trigger operations
	ListTriggers(ctx context.Context) ([]Trigger, error)
	GetTrigger(ctx context.Context, id string) (*Trigger, error)
	CreateTrigger(ctx context.Context, trigger Trigger) (*Trigger, error)
	UpdateTrigger(ctx context.Context, id string, trigger Trigger) (*Trigger, error)
	DeleteTrigger(ctx context.Context, id string) (*Trigger, error)

	// Role operations
	ListRoles(ctx context.Context, opts ListRolesOptions) ([]Role, error)
	GetRole(ctx context.Context, id int) (*Role, error)
	CreateRole(ctx context.Context, role CreateRoleRequest) (*Role, error)
	UpdateRole(ctx context.Context, id int, role UpdateRoleRequest) (*Role, error)
	DeleteRole(ctx context.Context, id int, cascade bool) (*Role, error)

	// Extension operations
	ListExtensions(ctx context.Context) ([]Extension, error)
	GetExtension(ctx context.Context, name string) (*Extension, error)
	CreateExtension(ctx context.Context, extension Extension) (*Extension, error)
	UpdateExtension(ctx context.Context, name string, extension Extension) (*Extension, error)
	DeleteExtension(ctx context.Context, name string) (*Extension, error)

	// Policy operations
	ListPolicies(ctx context.Context) ([]Policy, error)
	GetPolicy(ctx context.Context, id string) (*Policy, error)
	CreatePolicy(ctx context.Context, policy Policy) (*Policy, error)
	UpdatePolicy(ctx context.Context, id string, policy Policy) (*Policy, error)
	DeletePolicy(ctx context.Context, id string) (*Policy, error)

	// Publication operations
	ListPublications(ctx context.Context) ([]Publication, error)
	GetPublication(ctx context.Context, id string) (*Publication, error)
	CreatePublication(ctx context.Context, publication Publication) (*Publication, error)
	UpdatePublication(ctx context.Context, id string, publication Publication) (*Publication, error)
	DeletePublication(ctx context.Context, id string) (*Publication, error)

	// Foreign table operations
	ListForeignTables(ctx context.Context, opts ListForeignTablesOptions) ([]ForeignTable, error)
	GetForeignTable(ctx context.Context, id int) (*ForeignTable, error)

	// Index operations
	ListIndexes(ctx context.Context) ([]Index, error)
	GetIndex(ctx context.Context, id string) (*Index, error)

	// Type operations
	ListTypes(ctx context.Context) ([]Type, error)

	// Query operations
	ExecuteSQL(ctx context.Context, query string, params []interface{}, auditCtx *audit.Context) (*QueryResult, error)
	FormatQuery(ctx context.Context, req FormatQueryRequest) (string, error)
	ParseQuery(ctx context.Context, req ParseQueryRequest) (*ParsedQuery, error)
	DeparseQuery(ctx context.Context, req DeparseQueryRequest) (string, error)

	// Table privileges
	ListTablePrivileges(ctx context.Context, opts ListTablePrivilegesOptions) ([]TablePrivilege, error)
	GrantTablePrivileges(ctx context.Context, privileges []TablePrivilegeGrant) ([]TablePrivilege, error)
	RevokeTablePrivileges(ctx context.Context, privileges []TablePrivilegeRevoke) ([]TablePrivilege, error)

	// Column privileges
	ListColumnPrivileges(ctx context.Context, opts ListColumnPrivilegesOptions) ([]ColumnPrivilege, error)
	GrantColumnPrivileges(ctx context.Context, privileges []ColumnPrivilegeGrant) ([]ColumnPrivilege, error)
	RevokeColumnPrivileges(ctx context.Context, privileges []ColumnPrivilegeRevoke) ([]ColumnPrivilege, error)
}

// Options structs
type ListSchemasOptions struct {
	IncludeSystemSchemas bool
	Limit                int
	Offset               int
}

type ListTablesOptions struct {
	IncludeSystemSchemas bool
	IncludedSchemas      []string
	ExcludedSchemas      []string
	Limit                int
	Offset               int
	IncludeColumns       bool
}

type ListColumnsOptions struct {
	IncludeSystemSchemas bool
	IncludedSchemas      []string
	ExcludedSchemas      []string
	Limit                int
	Offset               int
}

type ListViewsOptions struct {
	IncludeSystemSchemas bool
	IncludedSchemas      []string
	ExcludedSchemas      []string
	Limit                int
	Offset               int
	IncludeColumns       bool
}

type ListMaterializedViewsOptions struct {
	IncludedSchemas []string
	ExcludedSchemas []string
	Limit           int
	Offset          int
	IncludeColumns  bool
}

type ListRolesOptions struct {
	IncludeSystemSchemas bool
	Limit                int
	Offset               int
}

type ListForeignTablesOptions struct {
	Limit          int
	Offset         int
	IncludeColumns bool
}

type ListTablePrivilegesOptions struct {
	IncludeSystemSchemas bool
	IncludedSchemas      []string
	ExcludedSchemas      []string
	Limit                int
	Offset               int
}

type ListColumnPrivilegesOptions struct {
	IncludeSystemSchemas bool
	IncludedSchemas      []string
	ExcludedSchemas      []string
	Limit                int
	Offset               int
}

// Request structs
type CreateSchemaRequest struct {
	Name  string `json:"name"`
	Owner string `json:"owner,omitempty"`
}

type UpdateSchemaRequest struct {
	Name  string `json:"name,omitempty"`
	Owner string `json:"owner,omitempty"`
}

type CreateTableRequest struct {
	Name    string  `json:"name"`
	Schema  string  `json:"schema,omitempty"`
	Comment *string `json:"comment,omitempty"`
}

type UpdateTableRequest struct {
	Name                 string  `json:"name,omitempty"`
	Schema               string  `json:"schema,omitempty"`
	RlsEnabled           *bool   `json:"rls_enabled,omitempty"`
	RlsForced            *bool   `json:"rls_forced,omitempty"`
	ReplicaIdentity      *string `json:"replica_identity,omitempty"`
	ReplicaIdentityIndex *string `json:"replica_identity_index,omitempty"`
	Comment              *string `json:"comment,omitempty"`
}

type CreateColumnRequest struct {
	TableID            int         `json:"table_id"`
	Name               string      `json:"name"`
	Type               string      `json:"type"`
	DefaultValue       interface{} `json:"default_value,omitempty"`
	DefaultValueFormat *string     `json:"default_value_format,omitempty"`
	IsIdentity         bool        `json:"is_identity,omitempty"`
	IdentityGeneration *string     `json:"identity_generation,omitempty"`
	IsNullable         *bool       `json:"is_nullable,omitempty"`
	IsPrimaryKey       bool        `json:"is_primary_key,omitempty"`
	IsUnique           bool        `json:"is_unique,omitempty"`
	Comment            *string     `json:"comment,omitempty"`
	Check              *string     `json:"check,omitempty"`
}

type UpdateColumnRequest struct {
	Name               string      `json:"name,omitempty"`
	Type               string      `json:"type,omitempty"`
	DropDefault        bool        `json:"drop_default,omitempty"`
	DefaultValue       interface{} `json:"default_value,omitempty"`
	DefaultValueFormat *string     `json:"default_value_format,omitempty"`
	IsIdentity         bool        `json:"is_identity,omitempty"`
	IdentityGeneration *string     `json:"identity_generation,omitempty"`
	IsNullable         *bool       `json:"is_nullable,omitempty"`
	IsUnique           bool        `json:"is_unique,omitempty"`
	Comment            *string     `json:"comment,omitempty"`
	Check              *string     `json:"check,omitempty"`
}

type CreateRoleRequest struct {
	Name              string            `json:"name"`
	Password          *string           `json:"password,omitempty"`
	InheritRole       *bool             `json:"inherit_role,omitempty"`
	CanLogin          *bool             `json:"can_login,omitempty"`
	IsSuperuser       *bool             `json:"is_superuser,omitempty"`
	CanCreateDB       *bool             `json:"can_create_db,omitempty"`
	CanCreateRole     *bool             `json:"can_create_role,omitempty"`
	IsReplicationRole *bool             `json:"is_replication_role,omitempty"`
	CanBypassRLS      *bool             `json:"can_bypass_rls,omitempty"`
	ConnectionLimit   *int              `json:"connection_limit,omitempty"`
	MemberOf          []string          `json:"member_of,omitempty"`
	Members           []string          `json:"members,omitempty"`
	Admins            []string          `json:"admins,omitempty"`
	ValidUntil        *string           `json:"valid_until,omitempty"`
	Config            map[string]string `json:"config,omitempty"`
}

type UpdateRoleRequest struct {
	Name              string                  `json:"name,omitempty"`
	Password          *string                 `json:"password,omitempty"`
	InheritRole       *bool                   `json:"inherit_role,omitempty"`
	CanLogin          *bool                   `json:"can_login,omitempty"`
	IsSuperuser       *bool                   `json:"is_superuser,omitempty"`
	CanCreateDB       *bool                   `json:"can_create_db,omitempty"`
	CanCreateRole     *bool                   `json:"can_create_role,omitempty"`
	IsReplicationRole *bool                   `json:"is_replication_role,omitempty"`
	CanBypassRLS      *bool                   `json:"can_bypass_rls,omitempty"`
	ConnectionLimit   *int                    `json:"connection_limit,omitempty"`
	ValidUntil        *string                 `json:"valid_until,omitempty"`
	Config            []UpdateRoleConfigEntry `json:"config,omitempty"`
}

type UpdateRoleConfigEntry struct {
	Op    string `json:"op"`
	Path  string `json:"path"`
	Value string `json:"value,omitempty"`
}

type FormatQueryRequest struct {
	Query string `json:"query"`
}

type ParseQueryRequest struct {
	Query string `json:"query"`
}

type DeparseQueryRequest struct {
	// Add fields as needed based on API specification
}

// Schema represents database schema
type Schema struct {
	ID    int    `json:"id"`
	Name  string `json:"name"`
	Owner string `json:"owner"`
}

// Table represents database table structure
type Table struct {
	ID               int          `json:"id"`
	Schema           string       `json:"schema"`
	Name             string       `json:"name"`
	RlsEnabled       bool         `json:"rls_enabled"`
	RlsForced        bool         `json:"rls_forced"`
	ReplicaIdentity  string       `json:"replica_identity"`
	Bytes            int          `json:"bytes"`
	Size             string       `json:"size"`
	LiveRowsEstimate int          `json:"live_rows_estimate"`
	DeadRowsEstimate int          `json:"dead_rows_estimate"`
	Comment          *string      `json:"comment"`
	Columns          []Column     `json:"columns,omitempty"`
	PrimaryKeys      []PrimaryKey `json:"primary_keys"`
	Relationships    []Relation   `json:"relationships"`
}

// Column represents database column
type Column struct {
	TableID            int         `json:"table_id"`
	Schema             string      `json:"schema"`
	Table              string      `json:"table"`
	ID                 string      `json:"id"`
	OrdinalPosition    int         `json:"ordinal_position"`
	Name               string      `json:"name"`
	DefaultValue       interface{} `json:"default_value"`
	DataType           string      `json:"data_type"`
	Format             string      `json:"format"`
	IsIdentity         bool        `json:"is_identity"`
	IdentityGeneration *string     `json:"identity_generation"`
	IsGenerated        bool        `json:"is_generated"`
	IsNullable         bool        `json:"is_nullable"`
	IsUpdatable        bool        `json:"is_updatable"`
	IsUnique           bool        `json:"is_unique"`
	Enums              []string    `json:"enums"`
	Check              *string     `json:"check"`
	Comment            *string     `json:"comment"`
}

// NormalizeDataType normalizes PostgreSQL data type names to a consistent format
// This helps resolve issues where the database returns type aliases that may not
// be recognized by the frontend
func (c *Column) NormalizeDataType() string {
	switch c.DataType {
	case "double precision", "float8":
		return "double"
	case "real", "float4":
		return "float"
	case "integer", "int", "int4":
		return "int"
	case "bigint", "int8":
		return "bigint"
	case "smallint", "int2":
		return "smallint"
	case "boolean", "bool":
		return "boolean"
	case "character varying", "varchar":
		return "varchar"
	case "timestamp without time zone":
		return "timestamp"
	case "timestamp with time zone":
		return "timestamptz"
	default:
		return c.DataType
	}
}

// View represents database view
type View struct {
	ID          int      `json:"id"`
	Schema      string   `json:"schema"`
	Name        string   `json:"name"`
	IsUpdatable bool     `json:"is_updatable"`
	Comment     *string  `json:"comment"`
	Columns     []Column `json:"columns,omitempty"`
}

// MaterializedView represents database materialized view
type MaterializedView struct {
	ID          int      `json:"id"`
	Schema      string   `json:"schema"`
	Name        string   `json:"name"`
	IsPopulated bool     `json:"is_populated"`
	Comment     *string  `json:"comment"`
	Columns     []Column `json:"columns,omitempty"`
}

// Function represents database function
type Function struct {
	ID   string `json:"id"`
	Name string `json:"name"`
	// Add more fields as needed
}

// Trigger represents database trigger
type Trigger struct {
	ID   string `json:"id"`
	Name string `json:"name"`
	// Add more fields as needed
}

// Role represents database role
type Role struct {
	ID                int                    `json:"id"`
	Name              string                 `json:"name"`
	IsSuperuser       bool                   `json:"is_superuser"`
	CanCreateDB       bool                   `json:"can_create_db"`
	CanCreateRole     bool                   `json:"can_create_role"`
	InheritRole       bool                   `json:"inherit_role"`
	CanLogin          bool                   `json:"can_login"`
	IsReplicationRole bool                   `json:"is_replication_role"`
	CanBypassRLS      bool                   `json:"can_bypass_rls"`
	ActiveConnections int                    `json:"active_connections"`
	ConnectionLimit   int                    `json:"connection_limit"`
	Password          string                 `json:"password"`
	ValidUntil        *string                `json:"valid_until"`
	Config            map[string]interface{} `json:"config"`
}

// Extension represents database extension
type Extension struct {
	Name string `json:"name"`
	// Add more fields as needed
}

// Policy represents database policy
type Policy struct {
	ID   string `json:"id"`
	Name string `json:"name"`
	// Add more fields as needed
}

// Publication represents database publication
type Publication struct {
	ID   string `json:"id"`
	Name string `json:"name"`
	// Add more fields as needed
}

// ForeignTable represents database foreign table
type ForeignTable struct {
	ID      int      `json:"id"`
	Schema  string   `json:"schema"`
	Name    string   `json:"name"`
	Comment *string  `json:"comment"`
	Columns []Column `json:"columns,omitempty"`
}

// Index represents database index
type Index struct {
	ID   string `json:"id"`
	Name string `json:"name"`
	// Add more fields as needed
}

// Type represents database type
type Type struct {
	ID   int    `json:"id"`
	Name string `json:"name"`
	// Add more fields as needed
}

// PrimaryKey represents table primary key
type PrimaryKey struct {
	Schema    string `json:"schema"`
	TableName string `json:"table_name"`
	Name      string `json:"name"`
	TableID   int    `json:"table_id"`
}

// Relation represents table relationship
type Relation struct {
	ID                int    `json:"id"`
	ConstraintName    string `json:"constraint_name"`
	SourceSchema      string `json:"source_schema"`
	SourceTableName   string `json:"source_table_name"`
	SourceColumnName  string `json:"source_column_name"`
	TargetTableSchema string `json:"target_table_schema"`
	TargetTableName   string `json:"target_table_name"`
	TargetColumnName  string `json:"target_column_name"`
}

// QueryResult represents SQL query result
type QueryResult struct {
	RowsAffected int64    `json:"rowsAffected"`
	Columns      []string `json:"columns"`
	Rows         [][]any  `json:"rows"`
}

// ParsedQuery represents parsed SQL query
type ParsedQuery struct {
	// Add fields as needed
}

// Privilege types
type TablePrivilege struct {
	RelationID int              `json:"relation_id"`
	Schema     string           `json:"schema"`
	Name       string           `json:"name"`
	Kind       string           `json:"kind"`
	Privileges []PrivilegeGrant `json:"privileges"`
}

type ColumnPrivilege struct {
	ColumnID   string           `json:"column_id"`
	Schema     string           `json:"relation_schema"`
	TableName  string           `json:"relation_name"`
	ColumnName string           `json:"column_name"`
	Privileges []PrivilegeGrant `json:"privileges"`
}

type PrivilegeGrant struct {
	Grantor       string `json:"grantor"`
	Grantee       string `json:"grantee"`
	PrivilegeType string `json:"privilege_type"`
	IsGrantable   bool   `json:"is_grantable"`
}

type TablePrivilegeGrant struct {
	RelationID    int    `json:"relation_id"`
	Grantee       string `json:"grantee"`
	PrivilegeType string `json:"privilege_type"`
	IsGrantable   bool   `json:"is_grantable"`
}

type TablePrivilegeRevoke struct {
	RelationID    int    `json:"relation_id"`
	Grantee       string `json:"grantee"`
	PrivilegeType string `json:"privilege_type"`
}

type ColumnPrivilegeGrant struct {
	ColumnID      string `json:"column_id"`
	Grantee       string `json:"grantee"`
	PrivilegeType string `json:"privilege_type"`
	IsGrantable   bool   `json:"is_grantable"`
}

type ColumnPrivilegeRevoke struct {
	ColumnID      string `json:"column_id"`
	Grantee       string `json:"grantee"`
	PrivilegeType string `json:"privilege_type"`
}

type SQLBaseType string

const (
	SQLBaseTypeExternal SQLBaseType = "external"
	SQLBaseTypeInternal SQLBaseType = "internal"
)

type clientOptions struct {
	recorder audit.Recorder
}

type Option func(*clientOptions)

func WithAuditRecorder(recorder audit.Recorder) Option {
	return func(options *clientOptions) {
		options.recorder = recorder
	}
}

func NewSQLBaseClient(opts ...Option) (SQLBase, error) {
	options := clientOptions{}
	for _, opt := range opts {
		opt(&options)
	}

	cfg := config.Current()
	sqlBaseCfg := cfg.SQLBase
	dbCfg := cfg.Database
	metaType := sqlBaseCfg.Type

	internalHost := dbCfg.Host
	internalPort := fmt.Sprintf("%d", dbCfg.Port)
	internalUser := dbCfg.Username
	internalPassword := dbCfg.Password
	internalDb := sqlBaseCfg.InternalDB

	switch SQLBaseType(metaType) {
	case SQLBaseTypeExternal:
		return NewExternalClient(options.recorder)
	case SQLBaseTypeInternal:
		return NewInternalClient(internalHost, internalPort, internalUser, internalPassword, internalDb, options.recorder)
	default:
		// Fall back to the internal client when SQL_BASE_TYPE is empty or invalid.
		return NewInternalClient(internalHost, internalPort, internalUser, internalPassword, internalDb, options.recorder)
	}
}
