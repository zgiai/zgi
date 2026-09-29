// Package sqlanalysis parses PostgreSQL SQL into a transport-neutral description.
package sqlanalysis

import "github.com/auxten/postgresql-parser/pkg/sql/sem/tree"

// Operation is the normalized top-level SQL operation.
type Operation string

const (
	OperationUnknown Operation = "unknown"
	OperationSelect  Operation = "select"
	OperationInsert  Operation = "insert"
	OperationUpdate  Operation = "update"
	OperationDelete  Operation = "delete"
	OperationDDL     Operation = "ddl"
	OperationMixed   Operation = "mixed"
	OperationExplain Operation = "explain"
	OperationShow    Operation = "show"
	OperationOther   Operation = "other"
)

// TableReference identifies a table mentioned by SQL.
type TableReference struct {
	Schema string `json:"schema,omitempty"`
	Name   string `json:"name"`
}

// ColumnReference identifies a column and its optional qualifier.
type ColumnReference struct {
	Qualifier string `json:"qualifier,omitempty"`
	Name      string `json:"name"`
}

// Analysis describes SQL structure without making authorization decisions.
type Analysis struct {
	Statements     []tree.Statement  `json:"-"`
	StatementTags  []string          `json:"statements"`
	StatementCount int               `json:"statement_count"`
	Operation      Operation         `json:"operation"`
	ReadOnly       bool              `json:"read_only"`
	Tables         []TableReference  `json:"tables,omitempty"`
	Columns        []ColumnReference `json:"columns,omitempty"`
	Functions      []string          `json:"functions,omitempty"`
	JoinCount      int               `json:"join_count"`
	MaxJoinCount   int               `json:"max_join_count"`
	HasCTE         bool              `json:"has_cte"`
	CTENames       []string          `json:"cte_names,omitempty"`
	HasSubquery    bool              `json:"has_subquery"`
	HasWildcard    bool              `json:"has_wildcard"`
	HasLimit       bool              `json:"has_limit"`
	ParseError     string            `json:"parse_error,omitempty"`
	// OutputAliasReferences are unqualified ORDER BY/GROUP BY references that
	// resolve to an output alias in the same SELECT scope.
	OutputAliasReferences []ColumnReference `json:"-"`
	// RestrictedReferences are references from clauses where a same-level
	// SELECT alias is not visible (SELECT, WHERE, JOIN ON, and HAVING).
	RestrictedReferences []ColumnReference `json:"-"`
	// DerivedColumnReferences are references resolved against columns produced
	// by a CTE or a derived table in the same SELECT scope.
	DerivedColumnReferences []ColumnReference `json:"-"`
	// TableAliases maps relation aliases to their physical table references.
	TableAliases map[string]TableReference `json:"-"`
}

// Parsed reports whether parsing completed successfully.
func (a Analysis) Parsed() bool { return a.ParseError == "" }
