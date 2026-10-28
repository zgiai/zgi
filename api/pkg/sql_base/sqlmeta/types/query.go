package types

import "time"

// QueryOptions controls behaviour when executing arbitrary SQL.
type QueryOptions struct {
	// Parameters binds positional parameters ($1, $2, ...) to the query.
	Parameters []any
	// StatementTimeout caps execution time. Zero means no explicit timeout.
	StatementTimeout time.Duration
}

// QueryRow captures a single result row mapped by column name.
type QueryRow map[string]any

// QueryResult is the raw outcome of a statement: the returned rows plus the
// PostgreSQL command tag information.
type QueryResult struct {
	Rows []QueryRow
	// Command is the command keyword from the tag (SELECT, INSERT, UPDATE, ...).
	// Empty when the driver did not report one.
	Command string
	// RowsAffected is the row count reported by the command tag. For SELECT it
	// is the number of returned rows; for INSERT/UPDATE/DELETE/MERGE it is the
	// number of rows written, whether or not RETURNING was used.
	RowsAffected int64
}

// QueryResponse mirrors postgres-meta query responses, embedding either data or an error.
type QueryResponse struct {
	Data []QueryRow
	// Command and RowsAffected carry the command tag of a successful statement.
	Command      string
	RowsAffected int64
	Error        *QueryError
}

// QueryError conveys PostgreSQL error metadata together with a formatted message.
type QueryError struct {
	Message        string
	Code           string
	Detail         string
	Hint           string
	Where          string
	SchemaName     string
	TableName      string
	ColumnName     string
	DataTypeName   string
	ConstraintName string
	FormattedError string
	Position       *int
}
