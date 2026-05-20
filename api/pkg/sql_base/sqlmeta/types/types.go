package types

// Table models the shape returned by metadata queries. Fields mirror postgres-meta responses.
type Table struct {
	ID               int64
	Schema           string
	Name             string
	RLSEnabled       bool
	RLSForced        bool
	ReplicaIdentity  string
	Bytes            int64
	Size             string
	LiveRowsEstimate int64
	DeadRowsEstimate int64
	Comment          *string
	Columns          []Column
	PrimaryKeys      []PrimaryKey
	Relationships    []Relationship
}

// Column captures basic column metadata returned from catalog queries.
type Column struct {
	TableID         int64
	Schema          string
	TableName       string
	ID              string
	OrdinalPosition int
	Name            string
	DataType        string
	Format          string
	DefaultValue    any
	IsIdentity      bool
	Identity        *string
	IsGenerated     bool
	IsNullable      bool
	IsUpdatable     bool
	IsUnique        bool
	Enums           []string
	Check           *string
	Comment         *string
}

// IndexAttribute captures a single attribute that composes a database index.
type IndexAttribute struct {
	AttributeNumber int
	AttributeName   string
	DataType        string
}

// Index models the metadata returned for indexes.
type Index struct {
	ID                    int64
	TableID               int64
	Schema                string
	NumberOfAttributes    int
	NumberOfKeyAttributes int
	IsUnique              bool
	IsPrimary             bool
	IsExclusion           bool
	IsImmediate           bool
	IsClustered           bool
	IsValid               bool
	CheckXmin             bool
	IsReady               bool
	IsLive                bool
	IsReplicaIdentity     bool
	KeyAttributes         []int32
	Collation             []int32
	Class                 []int32
	Options               []int32
	IndexPredicate        *string
	Comment               *string
	IndexDefinition       string
	AccessMethod          string
	IndexAttributes       []IndexAttribute
}

// ColumnIdentifier uniquely identifies a column by composite id or schema/table/name.
type ColumnIdentifier struct {
	ID     string
	Schema string
	Table  string
	Name   string
}

// ColumnListOptions defines filters while listing columns.
type ColumnListOptions struct {
	TableID              int64
	IncludeSystemSchemas bool
	IncludedSchemas      []string
	ExcludedSchemas      []string
	Pagination
}

// IndexListOptions defines filters while listing indexes.
type IndexListOptions struct {
	IncludeSystemSchemas bool
	IncludedSchemas      []string
	ExcludedSchemas      []string
	Pagination
}

// ColumnCreateInput captures inputs for adding a new column.
type ColumnCreateInput struct {
	TableID            int64
	Name               string
	Type               string
	DefaultValue       any
	DefaultValueSet    bool
	DefaultValueFormat string
	IsIdentity         bool
	IdentityGeneration string
	IsNullable         *bool
	IsPrimaryKey       bool
	IsUnique           bool
	Comment            *string
	Check              *string
}

// ColumnUpdateInput represents mutable column attributes.
type ColumnUpdateInput struct {
	Name                  *string
	Type                  *string
	DropDefault           bool
	DropDefaultSet        bool
	DefaultValue          any
	DefaultValueSet       bool
	DefaultValueFormat    string
	DefaultValueFormatSet bool
	IsIdentity            *bool
	IdentityGeneration    *string
	IdentityGenerationSet bool
	IsNullable            *bool
	IsUnique              *bool
	Comment               *string
	CommentSet            bool
	Check                 *string
	CheckSet              bool
}

// PrimaryKey describes a primary key column binding.
type PrimaryKey struct {
	Schema    string
	TableName string
	Name      string
	TableID   int64
}

// Relationship stands for foreign key relations.
type Relationship struct {
	ID                int64
	ConstraintName    string
	SourceSchema      string
	SourceTableName   string
	SourceColumnName  string
	TargetTableSchema string
	TargetTableName   string
	TargetColumnName  string
}

// Schema represents a PostgreSQL schema entry.
type Schema struct {
	ID    int64
	Name  string
	Owner string
}

// TableCreateInput is used by higher layers to request a new table.
type TableCreateInput struct {
	Name    string
	Schema  string
	Comment *string
}

// TableUpdateInput carries mutable table attributes.
type TableUpdateInput struct {
	Name                 *string
	Schema               *string
	RLSEnabled           *bool
	RLSForced            *bool
	ReplicaIdentity      *string
	ReplicaIdentityIndex *string
	PrimaryKeys          *[]TablePrimaryKeyInput
	Comment              *string
}

// TablePrimaryKeyInput represents a single-column primary key definition.
type TablePrimaryKeyInput struct {
	Name string
}

// TableIdentifier references a table uniquely by schema/name.
type TableIdentifier struct {
	ID     int64
	Schema string
	Name   string
}

// Pagination expresses limit/offset consumption.
type Pagination struct {
	Limit  int
	Offset int
}

// TableListOptions defines filters for listing tables.
type TableListOptions struct {
	IncludeSystemSchemas bool
	IncludedSchemas      []string
	ExcludedSchemas      []string
	IncludeColumns       bool
	Pagination
}

// TableDeleteOptions controls DROP TABLE behaviour.
type TableDeleteOptions struct {
	Cascade bool
}

// ************************************************ SCHEMA *******************************************************

// SchemaListOptions controls schema listing behaviour.
type SchemaListOptions struct {
	IncludeSystemSchemas bool
	IncludedSchemas      []string
	ExcludedSchemas      []string
	Pagination
}

// SchemaIdentifier points to a schema either by ID or name.
type SchemaIdentifier struct {
	ID   int64
	Name string
}

// SchemaCreateInput represents inputs for CREATE SCHEMA.
type SchemaCreateInput struct {
	Name  string
	Owner string
}

// SchemaUpdateInput represents mutable fields on schemas.
type SchemaUpdateInput struct {
	Name  *string
	Owner *string
}

// SchemaDropOptions controls DROP SCHEMA behaviour.
type SchemaDropOptions struct {
	Cascade bool
}
