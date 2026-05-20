package columns

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"regexp"
	"strings"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/zgiai/zgi/api/pkg/sql_base/sqlmeta/driver"
	"github.com/zgiai/zgi/api/pkg/sql_base/sqlmeta/types"
)

var (
	defaultSystemSchemas = []string{"pg_catalog", "information_schema", "pg_toast"}

	// columnIDRegexp matches the composite column identifier "tableOID.ordinal" (e.g., 12345.6).
	columnIDRegexp = regexp.MustCompile(`^(\d+)\.(\d+)$`)

	// simpleTypeRegexp recognises plain type aliases without schema or parameters (e.g., bigint, uuid).
	simpleTypeRegexp = regexp.MustCompile(`^[a-zA-Z_][a-zA-Z0-9_]*$`)

	// qualifiedTypeRegexp matches schema-qualified types like public.custom_type or myschema.sub.typ.
	qualifiedTypeRegexp = regexp.MustCompile(`^[a-zA-Z_][a-zA-Z0-9_]*(\.[a-zA-Z_][a-zA-Z0-9_]*)+$`)

	// paramTypeRegexp covers parameterised type expressions such as numeric(10,2) or timestamp(6).
	paramTypeRegexp = regexp.MustCompile(`^[a-zA-Z0-9_.(),\s]+$`)
)

// Repository exposes catalog-level operations for columns.
type Repository interface {
	List(ctx context.Context, opts types.ColumnListOptions) ([]types.Column, error)
	Retrieve(ctx context.Context, identifier types.ColumnIdentifier) (*types.Column, error)
	Create(ctx context.Context, input types.ColumnCreateInput) (*types.Column, error)
	Update(ctx context.Context, id string, input types.ColumnUpdateInput) (*types.Column, error)
	Delete(ctx context.Context, id string, cascade bool) (*types.Column, error)
}

type repository struct {
	pool *driver.Pool
}

var _ Repository = (*repository)(nil)

// NewRepository constructs a columns repository backed by a pgx pool.
func NewRepository(pool *driver.Pool) Repository {
	return &repository{pool: pool}
}

func (r *repository) List(ctx context.Context, opts types.ColumnListOptions) ([]types.Column, error) {
	raw := r.rawPool()
	if raw == nil {
		return nil, errors.New("sqlmeta: nil pool")
	}

	include := uniqueStrings(opts.IncludedSchemas)
	exclude := uniqueStrings(opts.ExcludedSchemas)
	if len(include) == 0 && !opts.IncludeSystemSchemas {
		exclude = uniqueStrings(append(exclude, defaultSystemSchemas...))
	}

	qOpts := columnsQueryOptions{
		includeSchemas: include,
		excludeSchemas: exclude,
		limit:          opts.Limit,
		offset:         opts.Offset,
	}
	if opts.TableID != 0 {
		qOpts.tableIDs = []int64{opts.TableID}
	}

	sqlStr, args := buildColumnsQuery(qOpts)
	rows, err := raw.Query(ctx, sqlStr, args...)
	if err != nil {
		return nil, fmt.Errorf("sqlmeta: list columns: %w", err)
	}
	defer rows.Close()

	return scanColumns(rows)
}

func (r *repository) Retrieve(ctx context.Context, identifier types.ColumnIdentifier) (*types.Column, error) {
	raw := r.rawPool()
	if raw == nil {
		return nil, errors.New("sqlmeta: nil pool")
	}

	switch {
	case identifier.ID != "":
		if !columnIDRegexp.MatchString(identifier.ID) {
			return nil, fmt.Errorf("sqlmeta: invalid column id format")
		}
		sqlStr, args := buildColumnsQuery(columnsQueryOptions{
			columnCompositeIDs: []string{identifier.ID},
			limit:              1,
		})
		return r.fetchSingle(ctx, sqlStr, args...)
	case identifier.Table != "" && identifier.Name != "":
		schema := identifier.Schema
		if schema == "" {
			schema = "public"
		}
		sqlStr, args := buildColumnsQuery(columnsQueryOptions{
			includeSchemas:    []string{schema},
			columnIdentifiers: []string{fmt.Sprintf("%s.%s", identifier.Table, identifier.Name)},
			limit:             1,
		})
		return r.fetchSingle(ctx, sqlStr, args...)
	default:
		return nil, fmt.Errorf("sqlmeta: column identifier requires id or schema/table/name")
	}
}

func (r *repository) Create(ctx context.Context, input types.ColumnCreateInput) (*types.Column, error) {
	raw := r.rawPool()
	if raw == nil {
		return nil, errors.New("sqlmeta: nil pool")
	}
	if input.TableID == 0 {
		return nil, fmt.Errorf("sqlmeta: column create requires table id")
	}
	if strings.TrimSpace(input.Name) == "" {
		return nil, fmt.Errorf("sqlmeta: column name required")
	}
	if strings.TrimSpace(input.Type) == "" {
		return nil, fmt.Errorf("sqlmeta: column type required")
	}
	if input.IsIdentity && input.DefaultValueSet {
		return nil, fmt.Errorf("sqlmeta: columns cannot both be identity and have a default value")
	}

	schema, tableName, err := r.fetchTableInfo(ctx, input.TableID)
	if err != nil {
		return nil, err
	}

	tableIdent := fmt.Sprintf("%s.%s", pgx.Identifier{schema}.Sanitize(), pgx.Identifier{tableName}.Sanitize())
	columnIdent := pgx.Identifier{input.Name}.Sanitize()
	typeExpr := typeIdentifier(input.Type)

	defaultClause := ""
	if input.IsIdentity {
		generation := input.IdentityGeneration
		if generation == "" {
			generation = "BY DEFAULT"
		}
		defaultClause = fmt.Sprintf("GENERATED %s AS IDENTITY", generation)
	} else if input.DefaultValueSet {
		if strings.EqualFold(input.DefaultValueFormat, "expression") {
			defaultClause = fmt.Sprintf("DEFAULT %s", fmt.Sprint(input.DefaultValue))
		} else {
			lit, err := formatLiteral(input.DefaultValue)
			if err != nil {
				return nil, fmt.Errorf("sqlmeta: format default literal: %w", err)
			}
			defaultClause = fmt.Sprintf("DEFAULT %s", lit)
		}
	}

	nullableClause := ""
	if input.IsNullable != nil {
		if *input.IsNullable {
			nullableClause = "NULL"
		} else {
			nullableClause = "NOT NULL"
		}
	}

	primaryKeyClause := ""
	if input.IsPrimaryKey {
		primaryKeyClause = "PRIMARY KEY"
	}

	uniqueClause := ""
	if input.IsUnique {
		uniqueClause = "UNIQUE"
	}

	checkClause := ""
	if input.Check != nil && strings.TrimSpace(*input.Check) != "" {
		checkClause = fmt.Sprintf("CHECK (%s)", *input.Check)
	}

	addColumnBuilder := strings.Builder{}
	addColumnBuilder.WriteString(fmt.Sprintf("ALTER TABLE %s ADD COLUMN %s %s", tableIdent, columnIdent, typeExpr))
	for _, clause := range []string{defaultClause, nullableClause, primaryKeyClause, uniqueClause, checkClause} {
		if strings.TrimSpace(clause) == "" {
			continue
		}
		addColumnBuilder.WriteString("\n    ")
		addColumnBuilder.WriteString(clause)
	}
	addColumnBuilder.WriteString(";")

	commentStmt := ""
	if input.Comment != nil {
		commentStmt = fmt.Sprintf("COMMENT ON COLUMN %s.%s.%s IS %s;",
			pgx.Identifier{schema}.Sanitize(),
			pgx.Identifier{tableName}.Sanitize(),
			columnIdent,
			quoteLiteral(*input.Comment),
		)
	}

	statements := strings.Builder{}
	statements.WriteString("BEGIN;\n  ")
	statements.WriteString(addColumnBuilder.String())
	statements.WriteString("\n")
	if commentStmt != "" {
		statements.WriteString("  ")
		statements.WriteString(commentStmt)
		statements.WriteString("\n")
	}
	statements.WriteString("COMMIT;")

	if _, err := raw.Exec(ctx, statements.String()); err != nil {
		return nil, fmt.Errorf("sqlmeta: create column: %w", err)
	}

	return r.Retrieve(ctx, types.ColumnIdentifier{Schema: schema, Table: tableName, Name: input.Name})
}

func (r *repository) Update(ctx context.Context, id string, input types.ColumnUpdateInput) (*types.Column, error) {
	raw := r.rawPool()
	if raw == nil {
		return nil, errors.New("sqlmeta: nil pool")
	}

	current, err := r.Retrieve(ctx, types.ColumnIdentifier{ID: id})
	if err != nil {
		return nil, err
	}

	tableIdent := fmt.Sprintf("%s.%s", pgx.Identifier{current.Schema}.Sanitize(), pgx.Identifier{current.TableName}.Sanitize())
	columnIdent := pgx.Identifier{current.Name}.Sanitize()

	nameSQL := ""
	if input.Name != nil && *input.Name != "" && *input.Name != current.Name {
		nameSQL = fmt.Sprintf("ALTER TABLE %s RENAME COLUMN %s TO %s;",
			tableIdent,
			columnIdent,
			pgx.Identifier{*input.Name}.Sanitize(),
		)
	}

	typeSQL := ""
	if input.Type != nil && *input.Type != "" {
		newType := typeIdentifier(*input.Type)
		typeSQL = fmt.Sprintf("ALTER TABLE %s ALTER COLUMN %s SET DATA TYPE %s USING %s::%s;",
			tableIdent,
			columnIdent,
			newType,
			columnIdent,
			newType,
		)
	}

	defaultSQL := ""
	if input.DropDefaultSet && input.DropDefault {
		defaultSQL = fmt.Sprintf("ALTER TABLE %s ALTER COLUMN %s DROP DEFAULT;", tableIdent, columnIdent)
	} else if input.DefaultValueSet {
		if strings.EqualFold(input.DefaultValueFormat, "expression") {
			defaultSQL = fmt.Sprintf("ALTER TABLE %s ALTER COLUMN %s SET DEFAULT %s;", tableIdent, columnIdent, fmt.Sprint(input.DefaultValue))
		} else {
			lit, err := formatLiteral(input.DefaultValue)
			if err != nil {
				return nil, fmt.Errorf("sqlmeta: format default literal: %w", err)
			}
			defaultSQL = fmt.Sprintf("ALTER TABLE %s ALTER COLUMN %s SET DEFAULT %s;", tableIdent, columnIdent, lit)
		}
	}

	identitySQL := ""
	if input.IsIdentity != nil && !*input.IsIdentity {
		identitySQL = fmt.Sprintf("ALTER TABLE %s ALTER COLUMN %s DROP IDENTITY IF EXISTS;", tableIdent, columnIdent)
	} else if current.IsIdentity {
		if input.IdentityGenerationSet && input.IdentityGeneration != nil {
			identitySQL = fmt.Sprintf("ALTER TABLE %s ALTER COLUMN %s SET GENERATED %s;", tableIdent, columnIdent, *input.IdentityGeneration)
		}
	} else if input.IsIdentity != nil && *input.IsIdentity {
		generation := "BY DEFAULT"
		if input.IdentityGeneration != nil && *input.IdentityGeneration != "" {
			generation = *input.IdentityGeneration
		}
		identitySQL = fmt.Sprintf("ALTER TABLE %s ALTER COLUMN %s ADD GENERATED %s AS IDENTITY;", tableIdent, columnIdent, generation)
	}

	nullableSQL := ""
	if input.IsNullable != nil {
		if *input.IsNullable {
			nullableSQL = fmt.Sprintf("ALTER TABLE %s ALTER COLUMN %s DROP NOT NULL;", tableIdent, columnIdent)
		} else {
			nullableSQL = fmt.Sprintf("ALTER TABLE %s ALTER COLUMN %s SET NOT NULL;", tableIdent, columnIdent)
		}
	}

	uniqueSQL := ""
	if current.IsUnique && input.IsUnique != nil && !*input.IsUnique {
		uniqueSQL = fmt.Sprintf(`
DO $$
DECLARE
  r record;
BEGIN
  FOR r IN
    SELECT conname FROM pg_constraint WHERE
      contype = 'u'
      AND cardinality(conkey) = 1
      AND conrelid = %d
      AND conkey[1] = %d
  LOOP
    EXECUTE %s || quote_ident(r.conname);
  END LOOP;
END
$$;`,
			current.TableID,
			current.OrdinalPosition,
			quoteLiteral(fmt.Sprintf("ALTER TABLE %s DROP CONSTRAINT ", tableIdent)),
		)
	} else if !current.IsUnique && input.IsUnique != nil && *input.IsUnique {
		uniqueSQL = fmt.Sprintf("ALTER TABLE %s ADD UNIQUE (%s);", tableIdent, columnIdent)
	}

	commentSQL := ""
	if input.CommentSet {
		if input.Comment == nil {
			commentSQL = fmt.Sprintf("COMMENT ON COLUMN %s.%s.%s IS NULL;",
				pgx.Identifier{current.Schema}.Sanitize(),
				pgx.Identifier{current.TableName}.Sanitize(),
				columnIdent,
			)
		} else {
			commentSQL = fmt.Sprintf("COMMENT ON COLUMN %s.%s.%s IS %s;",
				pgx.Identifier{current.Schema}.Sanitize(),
				pgx.Identifier{current.TableName}.Sanitize(),
				columnIdent,
				quoteLiteral(*input.Comment),
			)
		}
	}

	checkSQL := ""
	if input.CheckSet {
		additional := ""
		if input.Check != nil {
			constraintName := fmt.Sprintf("%s_%s_check", current.TableName, current.Name)
			additional = fmt.Sprintf(`
  ALTER TABLE %s ADD CONSTRAINT %s CHECK (%s);

  SELECT conkey INTO v_conkey FROM pg_constraint WHERE conname = %s;

  ASSERT v_conkey IS NOT NULL, 'error creating column constraint: check condition must refer to this column';
  ASSERT cardinality(v_conkey) = 1, 'error creating column constraint: check condition cannot refer to multiple columns';
  ASSERT v_conkey[1] = %d, 'error creating column constraint: check condition cannot refer to other columns';`,
				tableIdent,
				pgx.Identifier{constraintName}.Sanitize(),
				*input.Check,
				quoteLiteral(constraintName),
				current.OrdinalPosition,
			)
		}

		checkSQL = fmt.Sprintf(`
DO $$
DECLARE
  v_conname name;
  v_conkey int2[];
BEGIN
  SELECT conname INTO v_conname FROM pg_constraint WHERE
    contype = 'c'
    AND cardinality(conkey) = 1
    AND conrelid = %d
    AND conkey[1] = %d
    ORDER BY oid asc
    LIMIT 1;

  IF v_conname IS NOT NULL THEN
    EXECUTE format('ALTER TABLE %s DROP CONSTRAINT %%s', v_conname);
  END IF;
%s
END
$$;`,
			current.TableID,
			current.OrdinalPosition,
			tableIdent,
			additional,
		)
	}

	builder := strings.Builder{}
	builder.WriteString("BEGIN;\n")
	for _, stmt := range []string{nullableSQL, typeSQL, defaultSQL, identitySQL, uniqueSQL, commentSQL, checkSQL, nameSQL} {
		if strings.TrimSpace(stmt) == "" {
			continue
		}
		builder.WriteString(indentMultiline(stmt))
	}
	builder.WriteString("COMMIT;")

	if _, err := raw.Exec(ctx, builder.String()); err != nil {
		return nil, fmt.Errorf("sqlmeta: update column: %w", err)
	}

	if input.Name != nil && *input.Name != "" {
		return r.Retrieve(ctx, types.ColumnIdentifier{Schema: current.Schema, Table: current.TableName, Name: *input.Name})
	}
	return r.Retrieve(ctx, types.ColumnIdentifier{ID: id})
}

func (r *repository) Delete(ctx context.Context, id string, cascade bool) (*types.Column, error) {
	raw := r.rawPool()
	if raw == nil {
		return nil, errors.New("sqlmeta: nil pool")
	}
	col, err := r.Retrieve(ctx, types.ColumnIdentifier{ID: id})
	if err != nil {
		return nil, err
	}

	tableIdent := fmt.Sprintf("%s.%s", pgx.Identifier{col.Schema}.Sanitize(), pgx.Identifier{col.TableName}.Sanitize())
	columnIdent := pgx.Identifier{col.Name}.Sanitize()
	cascadeOption := "RESTRICT"
	if cascade {
		cascadeOption = "CASCADE"
	}

	stmt := fmt.Sprintf("ALTER TABLE %s DROP COLUMN %s %s;", tableIdent, columnIdent, cascadeOption)
	if _, err := raw.Exec(ctx, stmt); err != nil {
		return nil, fmt.Errorf("sqlmeta: delete column: %w", err)
	}

	return col, nil
}

func (r *repository) fetchSingle(ctx context.Context, sql string, args ...any) (*types.Column, error) {
	raw := r.rawPool()
	if raw == nil {
		return nil, errors.New("sqlmeta: nil pool")
	}
	rows, err := raw.Query(ctx, sql, args...)
	if err != nil {
		return nil, fmt.Errorf("sqlmeta: query column: %w", err)
	}
	defer rows.Close()

	cols, err := scanColumns(rows)
	if err != nil {
		return nil, err
	}
	if len(cols) == 0 {
		return nil, fmt.Errorf("sqlmeta: column not found")
	}
	return &cols[0], nil
}

func (r *repository) fetchTableInfo(ctx context.Context, id int64) (string, string, error) {
	raw := r.rawPool()
	if raw == nil {
		return "", "", errors.New("sqlmeta: nil pool")
	}

	var (
		schema    string
		tableName string
	)
	query := `
SELECT nc.nspname, c.relname
FROM pg_class c
JOIN pg_namespace nc ON c.relnamespace = nc.oid
WHERE c.oid = $1
LIMIT 1`
	if err := raw.QueryRow(ctx, query, id).Scan(&schema, &tableName); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return "", "", fmt.Errorf("sqlmeta: table not found for id %d", id)
		}
		return "", "", fmt.Errorf("sqlmeta: fetch table info: %w", err)
	}
	return schema, tableName, nil
}

func (r *repository) rawPool() *pgxpool.Pool {
	if r == nil || r.pool == nil {
		return nil
	}
	return r.pool.Raw()
}

func scanColumns(rows pgx.Rows) ([]types.Column, error) {
	result := make([]types.Column, 0)
	for rows.Next() {
		col, err := scanColumn(rows)
		if err != nil {
			return nil, err
		}
		result = append(result, *col)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("sqlmeta: iterate columns: %w", err)
	}
	return result, nil
}

func scanColumn(rows pgx.Rows) (*types.Column, error) {
	var (
		tableID      int64
		schema       string
		tableName    string
		id           string
		ordinal      int32
		name         string
		defaultValue sql.NullString
		dataType     string
		format       string
		isIdentity   bool
		identity     sql.NullString
		isGenerated  bool
		isNullable   bool
		isUpdatable  bool
		isUnique     bool
		enumsJSON    []byte
		checkValue   sql.NullString
		commentValue sql.NullString
	)

	if err := rows.Scan(
		&tableID,
		&schema,
		&tableName,
		&id,
		&ordinal,
		&name,
		&defaultValue,
		&dataType,
		&format,
		&isIdentity,
		&identity,
		&isGenerated,
		&isNullable,
		&isUpdatable,
		&isUnique,
		&enumsJSON,
		&checkValue,
		&commentValue,
	); err != nil {
		return nil, fmt.Errorf("sqlmeta: scan column: %w", err)
	}

	enums := make([]string, 0)
	if len(enumsJSON) > 0 {
		if err := json.Unmarshal(enumsJSON, &enums); err != nil {
			return nil, fmt.Errorf("sqlmeta: decode column enums: %w", err)
		}
	}

	var defaultAny any
	if defaultValue.Valid {
		defaultAny = defaultValue.String
	}

	var identityPtr *string
	if identity.Valid {
		val := identity.String
		identityPtr = &val
	}

	var checkPtr *string
	if checkValue.Valid {
		val := checkValue.String
		checkPtr = &val
	}

	var commentPtr *string
	if commentValue.Valid {
		val := commentValue.String
		commentPtr = &val
	}

	column := &types.Column{
		TableID:         tableID,
		Schema:          schema,
		TableName:       tableName,
		ID:              id,
		OrdinalPosition: int(ordinal),
		Name:            name,
		DefaultValue:    defaultAny,
		DataType:        dataType,
		Format:          format,
		IsIdentity:      isIdentity,
		Identity:        identityPtr,
		IsGenerated:     isGenerated,
		IsNullable:      isNullable,
		IsUpdatable:     isUpdatable,
		IsUnique:        isUnique,
		Enums:           enums,
		Check:           checkPtr,
		Comment:         commentPtr,
	}
	return column, nil
}

func uniqueStrings(values []string) []string {
	if len(values) == 0 {
		return values
	}
	seen := make(map[string]struct{}, len(values))
	out := make([]string, 0, len(values))
	for _, v := range values {
		v = strings.TrimSpace(v)
		if v == "" {
			continue
		}
		if _, ok := seen[v]; ok {
			continue
		}
		seen[v] = struct{}{}
		out = append(out, v)
	}
	return out
}

func quoteLiteral(s string) string {
	return "'" + strings.ReplaceAll(s, "'", "''") + "'"
}

func typeIdentifier(typeName string) string {
	typeName = strings.TrimSpace(typeName)
	if typeName == "" {
		return ""
	}
	if strings.HasSuffix(typeName, "[]") {
		base := strings.TrimSuffix(typeName, "[]")
		return typeIdentifier(base) + "[]"
	}
	if simpleTypeRegexp.MatchString(typeName) {
		return typeName
	}
	if qualifiedTypeRegexp.MatchString(typeName) {
		parts := strings.Split(typeName, ".")
		sanitized := make([]string, len(parts))
		for i, part := range parts {
			sanitized[i] = pgx.Identifier{part}.Sanitize()
		}
		return strings.Join(sanitized, ".")
	}
	if paramTypeRegexp.MatchString(typeName) {
		return typeName
	}
	return pgx.Identifier{typeName}.Sanitize()
}

func formatLiteral(value any) (string, error) {
	if value == nil {
		return "NULL", nil
	}
	switch v := value.(type) {
	case string:
		return quoteLiteral(v), nil
	case fmt.Stringer:
		return quoteLiteral(v.String()), nil
	case []byte:
		return quoteLiteral(string(v)), nil
	case bool:
		if v {
			return "TRUE", nil
		}
		return "FALSE", nil
	case int, int8, int16, int32, int64,
		uint, uint8, uint16, uint32, uint64,
		float32, float64:
		return fmt.Sprintf("%v", v), nil
	default:
		bytes, err := json.Marshal(v)
		if err != nil {
			return "", err
		}
		return quoteLiteral(string(bytes)), nil
	}
}

func indentMultiline(stmt string) string {
	if strings.TrimSpace(stmt) == "" {
		return ""
	}
	lines := strings.Split(stmt, "\n")
	for i, line := range lines {
		if strings.TrimSpace(line) == "" {
			continue
		}
		lines[i] = "  " + line
	}
	return strings.Join(lines, "\n") + "\n"
}
