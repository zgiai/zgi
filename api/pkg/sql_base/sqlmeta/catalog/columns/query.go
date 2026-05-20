package columns

import (
	"fmt"
	"strings"
)

const columnsBaseQuery = `
SELECT
  c.oid::int8 AS table_id,
  nc.nspname AS schema,
  c.relname AS table,
  (c.oid || '.' || a.attnum)::text AS id,
  a.attnum AS ordinal_position,
  a.attname AS name,
  CASE
    WHEN a.atthasdef THEN pg_get_expr(ad.adbin, ad.adrelid)
    ELSE NULL
  END AS default_value,
  CASE
    WHEN t.typtype = 'd' THEN CASE
      WHEN bt.typelem <> 0::oid AND bt.typlen = -1 THEN 'ARRAY'
      WHEN nbt.nspname = 'pg_catalog' THEN format_type(t.typbasetype, NULL)
      ELSE 'USER-DEFINED'
    END
    ELSE CASE
      WHEN t.typelem <> 0::oid AND t.typlen = -1 THEN 'ARRAY'
      WHEN nt.nspname = 'pg_catalog' THEN format_type(a.atttypid, NULL)
      ELSE 'USER-DEFINED'
    END
  END AS data_type,
  COALESCE(bt.typname, t.typname) AS format,
  a.attidentity IN ('a', 'd') AS is_identity,
  CASE
    a.attidentity
    WHEN 'a' THEN 'ALWAYS'
    WHEN 'd' THEN 'BY DEFAULT'
    ELSE NULL
  END AS identity_generation,
  a.attgenerated IN ('s') AS is_generated,
  NOT (
    a.attnotnull
    OR t.typtype = 'd' AND t.typnotnull
  ) AS is_nullable,
  (
    c.relkind IN ('r', 'p')
    OR c.relkind IN ('v', 'f') AND pg_column_is_updatable(c.oid, a.attnum, FALSE)
  ) AS is_updatable,
  uniques.table_id IS NOT NULL AS is_unique,
  check_constraints.definition AS "check",
  array_to_json(
    array(
      SELECT enumlabel
      FROM pg_catalog.pg_enum enums
      WHERE
        enums.enumtypid = COALESCE(bt.oid, t.oid)
        OR enums.enumtypid = COALESCE(bt.typelem, t.typelem)
      ORDER BY enums.enumsortorder
    )
  ) AS enums,
  col_description(c.oid, a.attnum) AS comment
FROM pg_attribute a
LEFT JOIN pg_attrdef ad ON a.attrelid = ad.adrelid AND a.attnum = ad.adnum
JOIN (
  pg_class c
  JOIN pg_namespace nc ON c.relnamespace = nc.oid
) ON a.attrelid = c.oid
JOIN (
  pg_type t
  JOIN pg_namespace nt ON t.typnamespace = nt.oid
) ON a.atttypid = t.oid
LEFT JOIN (
  pg_type bt
  JOIN pg_namespace nbt ON bt.typnamespace = nbt.oid
) ON t.typtype = 'd' AND t.typbasetype = bt.oid
LEFT JOIN (
  SELECT DISTINCT ON (table_id, ordinal_position)
    conrelid AS table_id,
    conkey[1] AS ordinal_position
  FROM pg_catalog.pg_constraint
  WHERE contype = 'u' AND cardinality(conkey) = 1
) AS uniques ON uniques.table_id = c.oid AND uniques.ordinal_position = a.attnum
LEFT JOIN (
  SELECT DISTINCT ON (table_id, ordinal_position)
    conrelid AS table_id,
    conkey[1] AS ordinal_position,
    substring(
      pg_get_constraintdef(pg_constraint.oid, true),
      8,
      length(pg_get_constraintdef(pg_constraint.oid, true)) - 8
    ) AS definition
  FROM pg_constraint
  WHERE contype = 'c' AND cardinality(conkey) = 1
  ORDER BY table_id, ordinal_position, oid asc
) AS check_constraints ON check_constraints.table_id = c.oid AND check_constraints.ordinal_position = a.attnum
WHERE
  TRUE`

type columnsQueryOptions struct {
	includeSchemas     []string
	excludeSchemas     []string
	tableIDs           []int64
	tableIdentifiers   []string
	columnIdentifiers  []string
	columnCompositeIDs []string
	limit              int
	offset             int
}

func buildColumnsQuery(opts columnsQueryOptions) (string, []any) {
	clauses := make([]string, 0, 8)
	args := make([]any, 0, 6)
	idx := 1

	if len(opts.includeSchemas) > 0 {
		clauses = append(clauses, fmt.Sprintf("nc.nspname = ANY($%d)", idx))
		args = append(args, opts.includeSchemas)
		idx++
	} else if len(opts.excludeSchemas) > 0 {
		clauses = append(clauses, fmt.Sprintf("NOT (nc.nspname = ANY($%d))", idx))
		args = append(args, opts.excludeSchemas)
		idx++
	}

	if len(opts.tableIDs) > 0 {
		clauses = append(clauses, fmt.Sprintf("c.oid = ANY($%d)", idx))
		args = append(args, opts.tableIDs)
		idx++
	}

	if len(opts.tableIdentifiers) > 0 {
		clauses = append(clauses, fmt.Sprintf("(nc.nspname || '.' || c.relname) = ANY($%d)", idx))
		args = append(args, opts.tableIdentifiers)
		idx++
	}

	if len(opts.columnIdentifiers) > 0 {
		clauses = append(clauses, fmt.Sprintf("(c.relname || '.' || a.attname) = ANY($%d)", idx))
		args = append(args, opts.columnIdentifiers)
		idx++
	}

	if len(opts.columnCompositeIDs) > 0 {
		clauses = append(clauses, fmt.Sprintf("((c.oid || '.' || a.attnum)::text) = ANY($%d)", idx))
		args = append(args, opts.columnCompositeIDs)
		idx++
	}

	clauses = append(clauses,
		"NOT pg_is_other_temp_schema(nc.oid)",
		"a.attnum > 0",
		"NOT a.attisdropped",
		"(c.relkind IN ('r', 'v', 'm', 'f', 'p'))",
		"(pg_has_role(c.relowner, 'USAGE') OR has_column_privilege(c.oid, a.attnum, 'SELECT, INSERT, UPDATE, REFERENCES'))",
	)

	builder := strings.Builder{}
	builder.WriteString(columnsBaseQuery)
	for _, clause := range clauses {
		builder.WriteString("\n  AND ")
		builder.WriteString(clause)
	}
	builder.WriteString("\n")

	if opts.limit > 0 {
		builder.WriteString(fmt.Sprintf("LIMIT %d\n", opts.limit))
	}
	if opts.offset > 0 {
		builder.WriteString(fmt.Sprintf("OFFSET %d\n", opts.offset))
	}

	return builder.String(), args
}
