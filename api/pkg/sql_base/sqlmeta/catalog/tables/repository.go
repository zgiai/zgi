package tables

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/zgiai/zgi/api/pkg/sql_base/sqlmeta/driver"
	"github.com/zgiai/zgi/api/pkg/sql_base/sqlmeta/types"
)

var defaultSystemSchemas = []string{"pg_catalog", "information_schema", "pg_toast"}

// Repository exposes catalog level access for table metadata mutations and reads.
type Repository interface {
	List(ctx context.Context, opts types.TableListOptions) ([]types.Table, error)
	GetByID(ctx context.Context, id int64, includeColumns bool) (*types.Table, error)
	GetByName(ctx context.Context, schema, name string, includeColumns bool) (*types.Table, error)
	Create(ctx context.Context, input types.TableCreateInput) (*types.Table, error)
	Update(ctx context.Context, id int64, input types.TableUpdateInput) (*types.Table, error)
	Delete(ctx context.Context, id int64, cascade bool) (*types.Table, error)
}

type repository struct {
	pool *driver.Pool
}

var _ Repository = (*repository)(nil)

// NewRepository wires a repository backed by the shared pgx pool.
func NewRepository(pool *driver.Pool) Repository {
	return &repository{pool: pool}
}

func (r *repository) List(ctx context.Context, opts types.TableListOptions) ([]types.Table, error) {
	raw := r.rawPool()
	if raw == nil {
		return nil, errors.New("sqlmeta: nil pool")
	}

	enrichedSQL, args := buildEnrichedTablesSQL(tableQueryOptions{
		IncludeColumns:       opts.IncludeColumns,
		IncludeSystemSchemas: opts.IncludeSystemSchemas,
		IncludedSchemas:      opts.IncludedSchemas,
		ExcludedSchemas:      opts.ExcludedSchemas,
		Limit:                opts.Limit,
		Offset:               opts.Offset,
	})

	rows, err := raw.Query(ctx, enrichedSQL, args...)
	if err != nil {
		return nil, fmt.Errorf("sqlmeta: list tables: %w", err)
	}
	defer rows.Close()

	return scanEnrichedRows(rows)
}

func (r *repository) GetByID(ctx context.Context, id int64, includeColumns bool) (*types.Table, error) {
	raw := r.rawPool()
	if raw == nil {
		return nil, errors.New("sqlmeta: nil pool")
	}

	enrichedSQL, args := buildEnrichedTablesSQL(tableQueryOptions{
		IncludeColumns: includeColumns,
		TableIDs:       []int64{id},
	})

	rows, err := raw.Query(ctx, enrichedSQL, args...)
	if err != nil {
		return nil, fmt.Errorf("sqlmeta: get table by id: %w", err)
	}
	defer rows.Close()

	table, err := scanSingleEnrichedRow(rows)
	if err != nil {
		return nil, err
	}
	if table == nil {
		return nil, fmt.Errorf("sqlmeta: table not found")
	}
	return table, nil
}

func (r *repository) GetByName(ctx context.Context, schema, name string, includeColumns bool) (*types.Table, error) {
	raw := r.rawPool()
	if raw == nil {
		return nil, errors.New("sqlmeta: nil pool")
	}

	identifier := fmt.Sprintf("%s.%s", schema, name)

	enrichedSQL, args := buildEnrichedTablesSQL(tableQueryOptions{
		IncludeColumns:   includeColumns,
		TableIdentifiers: []string{identifier},
	})

	rows, err := raw.Query(ctx, enrichedSQL, args...)
	if err != nil {
		return nil, fmt.Errorf("sqlmeta: get table by name: %w", err)
	}
	defer rows.Close()

	table, err := scanSingleEnrichedRow(rows)
	if err != nil {
		return nil, err
	}
	if table == nil {
		return nil, fmt.Errorf("sqlmeta: table not found")
	}
	return table, nil
}

func (r *repository) Create(ctx context.Context, input types.TableCreateInput) (*types.Table, error) {
	raw := r.rawPool()
	if raw == nil {
		return nil, errors.New("sqlmeta: nil pool")
	}

	schema := input.Schema
	if schema == "" {
		schema = "public"
	}

	ident := pgx.Identifier{schema, input.Name}.Sanitize()
	if _, err := raw.Exec(ctx, fmt.Sprintf("CREATE TABLE %s ();", ident)); err != nil {
		return nil, fmt.Errorf("sqlmeta: create table: %w", err)
	}
	if input.Comment != nil {
		if _, err := raw.Exec(ctx, fmt.Sprintf("COMMENT ON TABLE %s IS %s;", ident, quoteLiteral(*input.Comment))); err != nil {
			return nil, fmt.Errorf("sqlmeta: comment table: %w", err)
		}
	}

	return r.GetByName(ctx, schema, input.Name, true)
}

func (r *repository) Update(ctx context.Context, id int64, input types.TableUpdateInput) (*types.Table, error) {
	raw := r.rawPool()
	if raw == nil {
		return nil, errors.New("sqlmeta: nil pool")
	}

	current, err := r.GetByID(ctx, id, false)
	if err != nil {
		return nil, err
	}

	tx, err := raw.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return nil, fmt.Errorf("sqlmeta: begin table update: %w", err)
	}
	defer func() {
		if err != nil {
			_ = tx.Rollback(ctx)
		}
	}()

	sourceIdent := pgx.Identifier{current.Schema, current.Name}.Sanitize()

	if input.RLSEnabled != nil {
		stmt := fmt.Sprintf("ALTER TABLE %s ENABLE ROW LEVEL SECURITY;", sourceIdent)
		if !*input.RLSEnabled {
			stmt = fmt.Sprintf("ALTER TABLE %s DISABLE ROW LEVEL SECURITY;", sourceIdent)
		}
		if _, err = tx.Exec(ctx, stmt); err != nil {
			return nil, fmt.Errorf("sqlmeta: update rls: %w", err)
		}
	}

	if input.RLSForced != nil {
		stmt := fmt.Sprintf("ALTER TABLE %s FORCE ROW LEVEL SECURITY;", sourceIdent)
		if !*input.RLSForced {
			stmt = fmt.Sprintf("ALTER TABLE %s NO FORCE ROW LEVEL SECURITY;", sourceIdent)
		}
		if _, err = tx.Exec(ctx, stmt); err != nil {
			return nil, fmt.Errorf("sqlmeta: update rls forced: %w", err)
		}
	}

	if input.ReplicaIdentity != nil {
		replicaStmt := fmt.Sprintf("ALTER TABLE %s REPLICA IDENTITY %s;", sourceIdent, *input.ReplicaIdentity)
		if strings.EqualFold(*input.ReplicaIdentity, "INDEX") {
			if input.ReplicaIdentityIndex == nil || *input.ReplicaIdentityIndex == "" {
				return nil, fmt.Errorf("sqlmeta: replica_identity_index required when replica_identity is INDEX")
			}
			replicaStmt = fmt.Sprintf("ALTER TABLE %s REPLICA IDENTITY USING INDEX %s;", sourceIdent, pgx.Identifier{*input.ReplicaIdentityIndex}.Sanitize())
		}
		if _, err = tx.Exec(ctx, replicaStmt); err != nil {
			return nil, fmt.Errorf("sqlmeta: update replica identity: %w", err)
		}
	}

	if input.Comment != nil {
		if _, err = tx.Exec(ctx, fmt.Sprintf("COMMENT ON TABLE %s IS %s;", sourceIdent, quoteLiteral(*input.Comment))); err != nil {
			return nil, fmt.Errorf("sqlmeta: update table comment: %w", err)
		}
	}

	targetSchema := current.Schema
	if input.Schema != nil && *input.Schema != "" && *input.Schema != current.Schema {
		if _, err = tx.Exec(ctx, fmt.Sprintf("ALTER TABLE %s SET SCHEMA %s;", sourceIdent, pgx.Identifier{*input.Schema}.Sanitize())); err != nil {
			return nil, fmt.Errorf("sqlmeta: update schema: %w", err)
		}
		targetSchema = *input.Schema
		sourceIdent = pgx.Identifier{targetSchema, current.Name}.Sanitize()
	}

	if input.Name != nil && *input.Name != "" && *input.Name != current.Name {
		if _, err = tx.Exec(ctx, fmt.Sprintf("ALTER TABLE %s RENAME TO %s;", sourceIdent, pgx.Identifier{*input.Name}.Sanitize())); err != nil {
			return nil, fmt.Errorf("sqlmeta: rename table: %w", err)
		}
		sourceIdent = pgx.Identifier{targetSchema, *input.Name}.Sanitize()
	}

	if input.PrimaryKeys != nil {
		var constraintName sql.NullString
		err = tx.QueryRow(ctx, "SELECT conname FROM pg_constraint WHERE contype = 'p' AND conrelid = $1", id).Scan(&constraintName)
		if err != nil && !errors.Is(err, pgx.ErrNoRows) {
			return nil, fmt.Errorf("sqlmeta: fetch existing primary key constraint: %w", err)
		}
		if constraintName.Valid {
			dropStmt := fmt.Sprintf("ALTER TABLE %s DROP CONSTRAINT %s;", sourceIdent, pgx.Identifier{constraintName.String}.Sanitize())
			if _, err = tx.Exec(ctx, dropStmt); err != nil {
				return nil, fmt.Errorf("sqlmeta: drop existing primary key: %w", err)
			}
		}
		if len(*input.PrimaryKeys) > 0 {
			columns := make([]string, 0, len(*input.PrimaryKeys))
			for _, pk := range *input.PrimaryKeys {
				if pk.Name == "" {
					return nil, fmt.Errorf("sqlmeta: primary key column name cannot be empty")
				}
				columns = append(columns, pgx.Identifier{pk.Name}.Sanitize())
			}
			stmt := fmt.Sprintf("ALTER TABLE %s ADD PRIMARY KEY (%s);", sourceIdent, strings.Join(columns, ", "))
			if _, err = tx.Exec(ctx, stmt); err != nil {
				return nil, fmt.Errorf("sqlmeta: add primary key: %w", err)
			}
		}
	}

	if err = tx.Commit(ctx); err != nil {
		return nil, fmt.Errorf("sqlmeta: commit table update: %w", err)
	}

	finalName := current.Name
	if input.Name != nil && *input.Name != "" {
		finalName = *input.Name
	}
	return r.GetByName(ctx, targetSchema, finalName, true)
}

func (r *repository) Delete(ctx context.Context, id int64, cascade bool) (*types.Table, error) {
	raw := r.rawPool()
	if raw == nil {
		return nil, errors.New("sqlmeta: nil pool")
	}

	tbl, err := r.GetByID(ctx, id, true)
	if err != nil {
		return nil, err
	}

	ident := pgx.Identifier{tbl.Schema, tbl.Name}.Sanitize()
	clause := "RESTRICT"
	if cascade {
		clause = "CASCADE"
	}

	if _, err := raw.Exec(ctx, fmt.Sprintf("DROP TABLE %s %s;", ident, clause)); err != nil {
		return nil, fmt.Errorf("sqlmeta: drop table: %w", err)
	}

	return tbl, nil
}

func (r *repository) rawPool() *pgxpool.Pool {
	if r == nil || r.pool == nil {
		return nil
	}
	return r.pool.Raw()
}

// --- Query construction --------------------------------------------------------

type tableQueryOptions struct {
	IncludeColumns       bool
	IncludeSystemSchemas bool
	IncludedSchemas      []string
	ExcludedSchemas      []string
	TableIdentifiers     []string
	TableIDs             []int64
	Limit                int
	Offset               int
}

type sqlArgs struct {
	args []any
}

func (a *sqlArgs) add(value any) int {
	a.args = append(a.args, value)
	return len(a.args)
}

type schemaFilterClause struct {
	includeIdx int
	excludeIdx int
}

func (c schemaFilterClause) conditions(column string) []string {
	var conds []string
	if c.includeIdx > 0 {
		conds = append(conds, fmt.Sprintf("%s = ANY($%d)", column, c.includeIdx))
	}
	if c.excludeIdx > 0 {
		conds = append(conds, fmt.Sprintf("NOT (%s = ANY($%d))", column, c.excludeIdx))
	}
	return conds
}

func buildEnrichedTablesSQL(opts tableQueryOptions) (string, []any) {
	qb := &sqlArgs{}

	schemaFilter := prepareSchemaFilter(qb, opts.IncludedSchemas, opts.ExcludedSchemas, opts.IncludeSystemSchemas)

	identifierIdx := 0
	if len(opts.TableIdentifiers) > 0 {
		identifierIdx = qb.add(uniqueStrings(opts.TableIdentifiers))
	}

	idsIdx := 0
	if len(opts.TableIDs) > 0 {
		idsIdx = qb.add(opts.TableIDs)
	}

	limitIdx := 0
	if opts.Limit > 0 {
		limitIdx = qb.add(opts.Limit)
	}

	offsetIdx := 0
	if opts.Offset > 0 {
		offsetIdx = qb.add(opts.Offset)
	}

	var builder strings.Builder
	builder.WriteString("WITH tables AS (\n")
	buildTablesCTE(&builder, schemaFilter, identifierIdx, idsIdx, limitIdx, offsetIdx)
	builder.WriteString(")\n")

	if opts.IncludeColumns {
		builder.WriteString(", columns AS (\n")
		buildColumnsCTE(&builder, schemaFilter, identifierIdx, idsIdx)
		builder.WriteString(")\n")
	}

	builder.WriteString("SELECT\n  tables.id,\n  tables.schema,\n  tables.name,\n  tables.rls_enabled,\n  tables.rls_forced,\n  tables.replica_identity,\n  tables.bytes,\n  tables.size,\n  tables.live_rows_estimate,\n  tables.dead_rows_estimate,\n  tables.comment,\n  tables.primary_keys,\n  tables.relationships")
	if opts.IncludeColumns {
		builder.WriteString(",\n  " + coalesceRowsToArraySQL("columns", "columns.table_id = tables.id") + " AS columns")
	} else {
		builder.WriteString(",\n  '[]'::json AS columns")
	}
	builder.WriteString("\nFROM tables\n")

	return builder.String(), qb.args
}

func buildTablesCTE(builder *strings.Builder, schemaFilter schemaFilterClause, tableIdentifierIdx, idsIdx, limitIdx, offsetIdx int) {
	builder.WriteString(`  SELECT
    c.oid::int8 AS id,
    nc.nspname AS schema,
    c.relname AS name,
    c.relrowsecurity AS rls_enabled,
    c.relforcerowsecurity AS rls_forced,
    CASE
      WHEN c.relreplident = 'd' THEN 'DEFAULT'
      WHEN c.relreplident = 'i' THEN 'INDEX'
      WHEN c.relreplident = 'f' THEN 'FULL'
      ELSE 'NOTHING'
    END AS replica_identity,
    pg_total_relation_size(format('%I.%I', nc.nspname, c.relname))::int8 AS bytes,
    pg_size_pretty(pg_total_relation_size(format('%I.%I', nc.nspname, c.relname))) AS size,
    pg_stat_get_live_tuples(c.oid) AS live_rows_estimate,
    pg_stat_get_dead_tuples(c.oid) AS dead_rows_estimate,
    obj_description(c.oid) AS comment,
    COALESCE(pk.primary_keys, '[]'::jsonb) AS primary_keys,
    COALESCE(jsonb_agg(relationships) FILTER (WHERE relationships IS NOT NULL), '[]'::jsonb) AS relationships
  FROM
    pg_namespace nc
    JOIN pg_class c ON nc.oid = c.relnamespace
    LEFT JOIN (
      SELECT
        c.oid::int8 AS table_id,
        jsonb_agg(
          jsonb_build_object(
            'table_id', c.oid::int8,
            'schema', n.nspname,
            'table_name', c.relname,
            'name', a.attname
          )
          ORDER BY array_position(i.indkey, a.attnum)
        ) AS primary_keys
      FROM
        pg_index i
        JOIN pg_class c ON i.indrelid = c.oid
        JOIN pg_namespace n ON c.relnamespace = n.oid
        JOIN pg_attribute a ON a.attrelid = c.oid AND a.attnum = ANY(i.indkey)
`)

	pkConditions := append(schemaFilter.conditions("n.nspname"), tableIdentifierCondition("n.nspname || '.' || c.relname", tableIdentifierIdx))
	pkConditions = append(pkConditions, "i.indisprimary")
	pkConditions = collectConditions(pkConditions...)
	if len(pkConditions) > 0 {
		writeIndentedConditions(builder, "      WHERE", pkConditions, "        ")
	}

	builder.WriteString(`      GROUP BY c.oid
    ) AS pk ON pk.table_id = c.oid
    LEFT JOIN (
      SELECT
        c.conrelid::int8 AS source_table_id,
        c.confrelid::int8 AS target_table_id,
        c.oid::int8 AS id,
        c.conname AS constraint_name,
        nsa.nspname AS source_schema,
        csa.relname AS source_table_name,
        sa.attname AS source_column_name,
        nta.nspname AS target_table_schema,
        cta.relname AS target_table_name,
        ta.attname AS target_column_name
      FROM
        pg_constraint c
        JOIN pg_attribute sa ON sa.attrelid = c.conrelid AND sa.attnum = ANY(c.conkey)
        JOIN pg_class csa ON sa.attrelid = csa.oid
        JOIN pg_namespace nsa ON csa.relnamespace = nsa.oid
        JOIN pg_attribute ta ON ta.attrelid = c.confrelid AND ta.attnum = ANY(c.confkey)
        JOIN pg_class cta ON ta.attrelid = cta.oid
        JOIN pg_namespace nta ON cta.relnamespace = nta.oid
`)

	relConditions := collectConditions(
		relationshipSchemaCondition(schemaFilter, "nsa.nspname", "nta.nspname"),
		relationshipIdentifierCondition(tableIdentifierIdx),
		"c.contype = 'f'",
	)
	if len(relConditions) > 0 {
		writeIndentedConditions(builder, "      WHERE", relConditions, "        ")
	}

	builder.WriteString(`    ) AS relationships
      ON (relationships.source_schema = nc.nspname AND relationships.source_table_name = c.relname)
      OR (relationships.target_table_schema = nc.nspname AND relationships.target_table_name = c.relname)
`)

	baseConditions := append(schemaFilter.conditions("nc.nspname"),
		tableIdentifierCondition("nc.nspname || '.' || c.relname", tableIdentifierIdx))
	baseConditions = append(baseConditions,
		idsCondition("c.oid", idsIdx),
		"c.relkind IN ('r', 'p')",
		"NOT pg_is_other_temp_schema(nc.oid)",
		"(\n    pg_has_role(c.relowner, 'USAGE')\n    OR has_table_privilege(c.oid, 'SELECT, INSERT, UPDATE, DELETE, TRUNCATE, REFERENCES, TRIGGER')\n    OR has_any_column_privilege(c.oid, 'SELECT, INSERT, UPDATE, REFERENCES')\n  )",
	)
	conditions := collectConditions(baseConditions...)

	if len(conditions) > 0 {
		writeIndentedConditions(builder, "  WHERE", conditions, "    ")
	}

	builder.WriteString("  GROUP BY\n    c.oid,\n    c.relname,\n    c.relrowsecurity,\n    c.relforcerowsecurity,\n    c.relreplident,\n    nc.nspname,\n    pk.primary_keys\n")

	if limitIdx > 0 {
		builder.WriteString(fmt.Sprintf("  LIMIT $%d\n", limitIdx))
	}
	if offsetIdx > 0 {
		builder.WriteString(fmt.Sprintf("  OFFSET $%d\n", offsetIdx))
	}
}

func buildColumnsCTE(builder *strings.Builder, schemaFilter schemaFilterClause, tableIdentifierIdx, idsIdx int) {
	builder.WriteString(`  SELECT
    c.oid::int8 AS table_id,
    nc.nspname AS schema,
    c.relname AS table,
    (c.oid || '.' || a.attnum) AS id,
    a.attnum AS ordinal_position,
    a.attname AS name,
    CASE
      WHEN a.atthasdef THEN pg_get_expr(ad.adbin, ad.adrelid)
      ELSE NULL
    END AS default_value,
    CASE
      WHEN t.typtype = 'd' THEN CASE
        WHEN bt.typelem <> 0 :: oid AND bt.typlen = -1 THEN 'ARRAY'
        WHEN nbt.nspname = 'pg_catalog' THEN format_type(t.typbasetype, NULL)
        ELSE 'USER-DEFINED'
      END
      ELSE CASE
        WHEN t.typelem <> 0 :: oid AND t.typlen = -1 THEN 'ARRAY'
        WHEN nt.nspname = 'pg_catalog' THEN format_type(a.atttypid, NULL)
        ELSE 'USER-DEFINED'
      END
    END AS data_type,
    COALESCE(bt.typname, t.typname) AS format,
    a.attidentity IN ('a', 'd') AS is_identity,
    CASE a.attidentity
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
        WHERE enums.enumtypid = coalesce(bt.oid, t.oid)
          OR enums.enumtypid = coalesce(bt.typelem, t.typelem)
        ORDER BY enums.enumsortorder
      )
    ) AS enums,
    col_description(c.oid, a.attnum) AS comment
  FROM
    pg_attribute a
    LEFT JOIN pg_attrdef ad ON a.attrelid = ad.adrelid AND a.attnum = ad.adnum
    JOIN pg_class c ON a.attrelid = c.oid
    JOIN pg_namespace nc ON c.relnamespace = nc.oid
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
      ORDER BY table_id, ordinal_position, oid ASC
    ) AS check_constraints ON check_constraints.table_id = c.oid AND check_constraints.ordinal_position = a.attnum
`)

	colConditions := append(schemaFilter.conditions("nc.nspname"),
		tableIdentifierCondition("nc.nspname || '.' || c.relname", tableIdentifierIdx))
	colConditions = append(colConditions,
		idsCondition("c.oid", idsIdx),
		"NOT pg_is_other_temp_schema(nc.oid)",
		"a.attnum > 0",
		"NOT a.attisdropped",
		"(c.relkind IN ('r', 'v', 'm', 'f', 'p'))",
		"(\n    pg_has_role(c.relowner, 'USAGE')\n    OR has_column_privilege(c.oid, a.attnum, 'SELECT, INSERT, UPDATE, REFERENCES')\n  )",
	)
	conditions := collectConditions(colConditions...)

	if len(conditions) > 0 {
		writeIndentedConditions(builder, "  WHERE", conditions, "    ")
	}
}

func prepareSchemaFilter(qb *sqlArgs, include, exclude []string, includeSystem bool) schemaFilterClause {
	include = uniqueStrings(include)
	exclude = uniqueStrings(exclude)

	if !includeSystem {
		exclude = uniqueStrings(append(exclude, defaultSystemSchemas...))
	}

	var clause schemaFilterClause
	if len(include) > 0 {
		clause.includeIdx = qb.add(include)
	}
	if len(exclude) > 0 {
		clause.excludeIdx = qb.add(exclude)
	}
	return clause
}

func collectConditions(items ...string) []string {
	result := make([]string, 0, len(items))
	for _, item := range items {
		trimmed := strings.TrimSpace(item)
		if trimmed != "" {
			result = append(result, trimmed)
		}
	}
	return result
}

func writeIndentedConditions(builder *strings.Builder, prefix string, conditions []string, indent string) {
	if len(conditions) == 0 {
		return
	}
	builder.WriteString(prefix)
	builder.WriteString("\n")
	for i, cond := range conditions {
		builder.WriteString(indent)
		builder.WriteString(cond)
		if i < len(conditions)-1 {
			builder.WriteString(" AND\n")
		} else {
			builder.WriteString("\n")
		}
	}
}

func tableIdentifierCondition(column string, idx int) string {
	if idx == 0 {
		return ""
	}
	return fmt.Sprintf("%s = ANY($%d)", column, idx)
}

func idsCondition(column string, idx int) string {
	if idx == 0 {
		return ""
	}
	return fmt.Sprintf("%s = ANY($%d)", column, idx)
}

func relationshipIdentifierCondition(idx int) string {
	if idx == 0 {
		return ""
	}
	placeholder := fmt.Sprintf("$%d", idx)
	return fmt.Sprintf("((nsa.nspname || '.' || csa.relname) = ANY(%s) OR (nta.nspname || '.' || cta.relname) = ANY(%s))",
		placeholder, placeholder)
}

func relationshipSchemaCondition(filter schemaFilterClause, sourceColumn, targetColumn string) string {
	src := joinConditions(filter.conditions(sourceColumn))
	tgt := joinConditions(filter.conditions(targetColumn))

	switch {
	case src == "" && tgt == "":
		return ""
	case src != "" && tgt == "":
		return "(" + src + ")"
	case src == "" && tgt != "":
		return "(" + tgt + ")"
	default:
		return fmt.Sprintf("((%s) OR (%s))", src, tgt)
	}
}

func joinConditions(conds []string) string {
	if len(conds) == 0 {
		return ""
	}
	return strings.Join(conds, " AND ")
}

func coalesceRowsToArraySQL(source, filter string) string {
	return fmt.Sprintf("COALESCE((\n    SELECT json_agg(%[1]s) FILTER (WHERE %[2]s)\n    FROM %[1]s\n  ), '[]'::json)",
		source, filter)
}

func quoteLiteral(s string) string {
	return "'" + strings.ReplaceAll(s, "'", "''") + "'"
}

// --- Row scanning --------------------------------------------------------------

type enrichedRow struct {
	ID                int64
	Schema            string
	Name              string
	RLSEnabled        bool
	RLSForced         bool
	ReplicaIdentity   string
	Bytes             int64
	Size              string
	LiveRowsEstimate  int64
	DeadRowsEstimate  int64
	Comment           sql.NullString
	PrimaryKeysJSON   []byte
	RelationshipsJSON []byte
	ColumnsJSON       []byte
}

func scanEnrichedRows(rows pgx.Rows) ([]types.Table, error) {
	result := make([]types.Table, 0)
	for rows.Next() {
		tbl, err := scanRow(rows)
		if err != nil {
			return nil, err
		}
		result = append(result, *tbl)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("sqlmeta: iterate tables: %w", err)
	}
	return result, nil
}

func scanSingleEnrichedRow(rows pgx.Rows) (*types.Table, error) {
	if rows.Next() {
		tbl, err := scanRow(rows)
		if err != nil {
			return nil, err
		}
		if rows.Next() {
			return nil, fmt.Errorf("sqlmeta: expected single row")
		}
		return tbl, nil
	}
	return nil, nil
}

func scanRow(rows pgx.Rows) (*types.Table, error) {
	var er enrichedRow
	dest := []any{
		&er.ID,
		&er.Schema,
		&er.Name,
		&er.RLSEnabled,
		&er.RLSForced,
		&er.ReplicaIdentity,
		&er.Bytes,
		&er.Size,
		&er.LiveRowsEstimate,
		&er.DeadRowsEstimate,
		&er.Comment,
		&er.PrimaryKeysJSON,
		&er.RelationshipsJSON,
		&er.ColumnsJSON,
	}
	if err := rows.Scan(dest...); err != nil {
		return nil, fmt.Errorf("sqlmeta: scan table row: %w", err)
	}

	tbl := &types.Table{
		ID:               er.ID,
		Schema:           er.Schema,
		Name:             er.Name,
		RLSEnabled:       er.RLSEnabled,
		RLSForced:        er.RLSForced,
		ReplicaIdentity:  er.ReplicaIdentity,
		Bytes:            er.Bytes,
		Size:             er.Size,
		LiveRowsEstimate: er.LiveRowsEstimate,
		DeadRowsEstimate: er.DeadRowsEstimate,
	}
	if er.Comment.Valid {
		tbl.Comment = &er.Comment.String
	}

	if err := decodePrimaryKeys(er.PrimaryKeysJSON, &tbl.PrimaryKeys); err != nil {
		return nil, err
	}
	if err := decodeRelationships(er.RelationshipsJSON, &tbl.Relationships); err != nil {
		return nil, err
	}
	if err := decodeColumns(er.ColumnsJSON, &tbl.Columns); err != nil {
		return nil, err
	}

	return tbl, nil
}

type jsonPrimaryKey struct {
	TableID   int64  `json:"table_id"`
	Schema    string `json:"schema"`
	TableName string `json:"table_name"`
	Name      string `json:"name"`
}

type jsonRelationship struct {
	ID                int64  `json:"id"`
	ConstraintName    string `json:"constraint_name"`
	SourceSchema      string `json:"source_schema"`
	SourceTableName   string `json:"source_table_name"`
	SourceColumnName  string `json:"source_column_name"`
	TargetTableSchema string `json:"target_table_schema"`
	TargetTableName   string `json:"target_table_name"`
	TargetColumnName  string `json:"target_column_name"`
}

type jsonColumn struct {
	TableID            int64    `json:"table_id"`
	Schema             string   `json:"schema"`
	Table              string   `json:"table"`
	ID                 string   `json:"id"`
	OrdinalPosition    int      `json:"ordinal_position"`
	Name               string   `json:"name"`
	DefaultValue       any      `json:"default_value"`
	DataType           string   `json:"data_type"`
	Format             string   `json:"format"`
	IsIdentity         bool     `json:"is_identity"`
	IdentityGeneration *string  `json:"identity_generation"`
	IsGenerated        bool     `json:"is_generated"`
	IsNullable         bool     `json:"is_nullable"`
	IsUpdatable        bool     `json:"is_updatable"`
	IsUnique           bool     `json:"is_unique"`
	Check              *string  `json:"check"`
	Enums              []string `json:"enums"`
	Comment            *string  `json:"comment"`
}

func decodePrimaryKeys(data []byte, target *[]types.PrimaryKey) error {
	var raw []jsonPrimaryKey
	if err := json.Unmarshal(data, &raw); err != nil {
		return fmt.Errorf("sqlmeta: decode primary keys: %w", err)
	}
	keys := make([]types.PrimaryKey, len(raw))
	for i, pk := range raw {
		keys[i] = types.PrimaryKey{
			TableID:   pk.TableID,
			Schema:    pk.Schema,
			TableName: pk.TableName,
			Name:      pk.Name,
		}
	}
	*target = keys
	return nil
}

func decodeRelationships(data []byte, target *[]types.Relationship) error {
	var raw []jsonRelationship
	if err := json.Unmarshal(data, &raw); err != nil {
		return fmt.Errorf("sqlmeta: decode relationships: %w", err)
	}
	rels := make([]types.Relationship, len(raw))
	for i, rel := range raw {
		rels[i] = types.Relationship{
			ID:                rel.ID,
			ConstraintName:    rel.ConstraintName,
			SourceSchema:      rel.SourceSchema,
			SourceTableName:   rel.SourceTableName,
			SourceColumnName:  rel.SourceColumnName,
			TargetTableSchema: rel.TargetTableSchema,
			TargetTableName:   rel.TargetTableName,
			TargetColumnName:  rel.TargetColumnName,
		}
	}
	*target = rels
	return nil
}

func decodeColumns(data []byte, target *[]types.Column) error {
	if len(data) == 0 {
		*target = []types.Column{}
		return nil
	}
	var raw []jsonColumn
	if err := json.Unmarshal(data, &raw); err != nil {
		return fmt.Errorf("sqlmeta: decode columns: %w", err)
	}
	cols := make([]types.Column, len(raw))
	for i, col := range raw {
		cols[i] = types.Column{
			TableID:         col.TableID,
			Schema:          col.Schema,
			TableName:       col.Table,
			ID:              col.ID,
			OrdinalPosition: col.OrdinalPosition,
			Name:            col.Name,
			DefaultValue:    col.DefaultValue,
			DataType:        col.DataType,
			Format:          col.Format,
			IsIdentity:      col.IsIdentity,
			IsNullable:      col.IsNullable,
			IsUpdatable:     col.IsUpdatable,
			IsUnique:        col.IsUnique,
			Enums:           col.Enums,
			Check:           col.Check,
			Comment:         col.Comment,
			IsGenerated:     col.IsGenerated,
		}
		if col.IdentityGeneration != nil {
			cols[i].Identity = col.IdentityGeneration
		}
	}
	*target = cols
	return nil
}

func uniqueStrings(values []string) []string {
	if len(values) == 0 {
		return values
	}
	seen := make(map[string]struct{}, len(values))
	out := make([]string, 0, len(values))
	for _, v := range values {
		if _, ok := seen[v]; ok {
			continue
		}
		seen[v] = struct{}{}
		out = append(out, v)
	}
	return out
}
