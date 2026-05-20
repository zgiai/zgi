package schemas

import (
	"fmt"
	"strings"

	"github.com/zgiai/ginext/pkg/sql_base/sqlmeta/types"
)

const baseSelect = `SELECT
  n.oid::int8 AS id,
  n.nspname AS name,
  u.rolname AS owner
FROM pg_namespace n
JOIN pg_roles u ON n.nspowner = u.oid
WHERE
  NOT pg_catalog.starts_with(n.nspname, 'pg_temp_')
  AND NOT pg_catalog.starts_with(n.nspname, 'pg_toast_temp_')`

func buildListQuery(opts types.SchemaListOptions) (string, []any) {
	clauses := make([]string, 0, 4)
	args := make([]any, 0, 3)
	idx := 1

	if len(opts.IncludedSchemas) > 0 {
		clauses = append(clauses, fmt.Sprintf("n.nspname = ANY($%d)", idx))
		args = append(args, opts.IncludedSchemas)
		idx++
	}

	if len(opts.ExcludedSchemas) > 0 {
		clauses = append(clauses, fmt.Sprintf("NOT (n.nspname = ANY($%d))", idx))
		args = append(args, opts.ExcludedSchemas)
		idx++
	}

	if !opts.IncludeSystemSchemas {
		clauses = append(clauses, "NOT pg_catalog.starts_with(n.nspname, 'pg_')")
	}

	builder := strings.Builder{}
	builder.WriteString(baseSelect)
	for _, clause := range clauses {
		builder.WriteString("\n  AND ")
		builder.WriteString(clause)
	}
	builder.WriteString("\n")

	if opts.Limit > 0 {
		builder.WriteString(fmt.Sprintf("LIMIT %d\n", opts.Limit))
	}
	if opts.Offset > 0 {
		builder.WriteString(fmt.Sprintf("OFFSET %d\n", opts.Offset))
	}

	return builder.String(), args
}

func buildRetrieveQuery(identifier types.SchemaIdentifier) (string, []any, error) {
	var (
		clause string
		arg    any
	)

	if identifier.ID != 0 {
		clause = "n.oid = $1"
		arg = identifier.ID
	} else if identifier.Name != "" {
		clause = "n.nspname = $1"
		arg = identifier.Name
	} else {
		return "", nil, fmt.Errorf("sqlmeta: schema identifier requires id or name")
	}

	builder := strings.Builder{}
	builder.WriteString(baseSelect)
	builder.WriteString("\n  AND ")
	builder.WriteString(clause)
	builder.WriteString("\nLIMIT 1")

	return builder.String(), []any{arg}, nil
}
