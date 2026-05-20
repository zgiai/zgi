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

// QueryResponse mirrors postgres-meta query responses, embedding either data or an error.
type QueryResponse struct {
	Data  []QueryRow
	Error *QueryError
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
