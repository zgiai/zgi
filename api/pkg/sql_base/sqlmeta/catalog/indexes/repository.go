package indexes

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/zgiai/ginext/pkg/sql_base/sqlmeta/driver"
	"github.com/zgiai/ginext/pkg/sql_base/sqlmeta/types"
)

var defaultSystemSchemas = []string{"pg_catalog", "information_schema", "pg_toast"}

// Repository exposes accessors for index metadata.
type Repository interface {
	List(ctx context.Context, opts types.IndexListOptions) ([]types.Index, error)
	GetByID(ctx context.Context, id int64) (*types.Index, error)
}

type repository struct {
	pool *driver.Pool
}

var _ Repository = (*repository)(nil)

// NewRepository constructs a repository backed by the shared pgx pool.
func NewRepository(pool *driver.Pool) Repository {
	return &repository{pool: pool}
}

func (r *repository) List(ctx context.Context, opts types.IndexListOptions) ([]types.Index, error) {
	raw := r.rawPool()
	if raw == nil {
		return nil, errors.New("sqlmeta: nil pool")
	}

	include := uniqueStrings(opts.IncludedSchemas)
	exclude := uniqueStrings(opts.ExcludedSchemas)
	if len(include) == 0 && !opts.IncludeSystemSchemas {
		exclude = uniqueStrings(append(exclude, defaultSystemSchemas...))
	}

	qOpts := indexQueryOptions{
		includeSchemas: include,
		excludeSchemas: exclude,
		limit:          opts.Limit,
		offset:         opts.Offset,
	}

	sqlStr, args := buildIndexesQuery(qOpts)
	rows, err := raw.Query(ctx, sqlStr, args...)
	if err != nil {
		return nil, fmt.Errorf("sqlmeta: list indexes: %w", err)
	}
	defer rows.Close()

	return scanIndexes(rows)
}

func (r *repository) GetByID(ctx context.Context, id int64) (*types.Index, error) {
	raw := r.rawPool()
	if raw == nil {
		return nil, errors.New("sqlmeta: nil pool")
	}

	qOpts := indexQueryOptions{
		ids:   []int64{id},
		limit: 1,
	}

	sqlStr, args := buildIndexesQuery(qOpts)
	rows, err := raw.Query(ctx, sqlStr, args...)
	if err != nil {
		return nil, fmt.Errorf("sqlmeta: get index by id: %w", err)
	}
	defer rows.Close()

	return scanSingleIndex(rows)
}

func (r *repository) rawPool() *pgxpool.Pool {
	if r == nil || r.pool == nil {
		return nil
	}
	return r.pool.Raw()
}

// --- Query construction --------------------------------------------------------

type indexQueryOptions struct {
	includeSchemas []string
	excludeSchemas []string
	ids            []int64
	limit          int
	offset         int
}

type sqlArgs struct {
	args []any
}

func (a *sqlArgs) add(value any) int {
	a.args = append(a.args, value)
	return len(a.args)
}

func buildIndexesQuery(opts indexQueryOptions) (string, []any) {
	qb := &sqlArgs{}

	includeIdx := 0
	if len(opts.includeSchemas) > 0 {
		includeIdx = qb.add(opts.includeSchemas)
	}

	excludeIdx := 0
	if len(opts.excludeSchemas) > 0 {
		excludeIdx = qb.add(opts.excludeSchemas)
	}

	idsIdx := 0
	if len(opts.ids) > 0 {
		idsIdx = qb.add(opts.ids)
	}

	limitIdx := 0
	if opts.limit > 0 {
		limitIdx = qb.add(opts.limit)
	}

	offsetIdx := 0
	if opts.offset > 0 {
		offsetIdx = qb.add(opts.offset)
	}

	var builder strings.Builder
	builder.WriteString("SELECT\n")
	builder.WriteString("    idx.indexrelid::int8 AS id,\n")
	builder.WriteString("    idx.indrelid::int8 AS table_id,\n")
	builder.WriteString("    n.nspname AS schema,\n")
	builder.WriteString("    idx.indnatts AS number_of_attributes,\n")
	builder.WriteString("    idx.indnkeyatts AS number_of_key_attributes,\n")
	builder.WriteString("    idx.indisunique AS is_unique,\n")
	builder.WriteString("    idx.indisprimary AS is_primary,\n")
	builder.WriteString("    idx.indisexclusion AS is_exclusion,\n")
	builder.WriteString("    idx.indimmediate AS is_immediate,\n")
	builder.WriteString("    idx.indisclustered AS is_clustered,\n")
	builder.WriteString("    idx.indisvalid AS is_valid,\n")
	builder.WriteString("    idx.indcheckxmin AS check_xmin,\n")
	builder.WriteString("    idx.indisready AS is_ready,\n")
	builder.WriteString("    idx.indislive AS is_live,\n")
	builder.WriteString("    idx.indisreplident AS is_replica_identity,\n")
	builder.WriteString("    idx.indkey::int[] AS key_attributes,\n")
	builder.WriteString("    idx.indcollation::int[] AS collation,\n")
	builder.WriteString("    idx.indclass::int[] AS class,\n")
	builder.WriteString("    idx.indoption::int[] AS options,\n")
	builder.WriteString("    idx.indpred::text AS index_predicate,\n")
	builder.WriteString("    obj_description(idx.indexrelid, 'pg_class') AS comment,\n")
	builder.WriteString("    ix.indexdef AS index_definition,\n")
	builder.WriteString("    am.amname AS access_method,\n")
	builder.WriteString("    COALESCE(\n")
	builder.WriteString("      jsonb_agg(\n")
	builder.WriteString("        jsonb_build_object(\n")
	builder.WriteString("          'attribute_number', a.attnum,\n")
	builder.WriteString("          'attribute_name', a.attname,\n")
	builder.WriteString("          'data_type', format_type(a.atttypid, a.atttypmod)\n")
	builder.WriteString("        )\n")
	builder.WriteString("        ORDER BY a.attnum\n")
	builder.WriteString("      ) FILTER (WHERE a.attnum IS NOT NULL),\n")
	builder.WriteString("      '[]'::jsonb\n")
	builder.WriteString("    ) AS index_attributes\n")
	builder.WriteString("  FROM\n")
	builder.WriteString("    pg_index idx\n")
	builder.WriteString("    JOIN pg_class c ON c.oid = idx.indexrelid\n")
	builder.WriteString("    JOIN pg_namespace n ON n.oid = c.relnamespace\n")
	builder.WriteString("    JOIN pg_am am ON am.oid = c.relam\n")
	builder.WriteString("    LEFT JOIN pg_attribute a ON a.attrelid = c.oid AND a.attnum = ANY(idx.indkey)\n")
	builder.WriteString("    JOIN pg_indexes ix ON ix.indexname = c.relname AND ix.schemaname = n.nspname\n")
	builder.WriteString("  WHERE true\n")

	if includeIdx > 0 {
		builder.WriteString(fmt.Sprintf("    AND n.nspname = ANY($%d)\n", includeIdx))
	}
	if excludeIdx > 0 {
		builder.WriteString(fmt.Sprintf("    AND NOT (n.nspname = ANY($%d))\n", excludeIdx))
	}
	if idsIdx > 0 {
		builder.WriteString(fmt.Sprintf("    AND idx.indexrelid = ANY($%d)\n", idsIdx))
	}

	builder.WriteString("  GROUP BY\n")
	builder.WriteString("    idx.indexrelid,\n")
	builder.WriteString("    idx.indrelid,\n")
	builder.WriteString("    n.nspname,\n")
	builder.WriteString("    idx.indnatts,\n")
	builder.WriteString("    idx.indnkeyatts,\n")
	builder.WriteString("    idx.indisunique,\n")
	builder.WriteString("    idx.indisprimary,\n")
	builder.WriteString("    idx.indisexclusion,\n")
	builder.WriteString("    idx.indimmediate,\n")
	builder.WriteString("    idx.indisclustered,\n")
	builder.WriteString("    idx.indisvalid,\n")
	builder.WriteString("    idx.indcheckxmin,\n")
	builder.WriteString("    idx.indisready,\n")
	builder.WriteString("    idx.indislive,\n")
	builder.WriteString("    idx.indisreplident,\n")
	builder.WriteString("    idx.indkey,\n")
	builder.WriteString("    idx.indcollation,\n")
	builder.WriteString("    idx.indclass,\n")
	builder.WriteString("    idx.indoption,\n")
	builder.WriteString("    idx.indpred,\n")
	builder.WriteString("    ix.indexdef,\n")
	builder.WriteString("    am.amname\n")
	builder.WriteString("  ORDER BY n.nspname, idx.indexrelid\n")

	if limitIdx > 0 {
		builder.WriteString(fmt.Sprintf("  LIMIT $%d\n", limitIdx))
	}
	if offsetIdx > 0 {
		builder.WriteString(fmt.Sprintf("  OFFSET $%d\n", offsetIdx))
	}

	return builder.String(), qb.args
}

// --- Row decoding -------------------------------------------------------------

type indexRow struct {
	ID                    int64
	TableID               int64
	Schema                string
	NumberOfAttributes    int32
	NumberOfKeyAttributes int32
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
	IndexPredicate        sql.NullString
	Comment               sql.NullString
	IndexDefinition       string
	AccessMethod          string
	AttributesJSON        []byte
}

type jsonIndexAttribute struct {
	AttributeNumber int    `json:"attribute_number"`
	AttributeName   string `json:"attribute_name"`
	DataType        string `json:"data_type"`
}

func scanIndexes(rows pgx.Rows) ([]types.Index, error) {
	var result []types.Index
	for rows.Next() {
		idx, err := scanIndex(rows)
		if err != nil {
			return nil, err
		}
		result = append(result, *idx)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("sqlmeta: iterate indexes: %w", err)
	}
	return result, nil
}

func scanSingleIndex(rows pgx.Rows) (*types.Index, error) {
	if rows.Next() {
		idx, err := scanIndex(rows)
		if err != nil {
			return nil, err
		}
		if rows.Next() {
			return nil, fmt.Errorf("sqlmeta: expected single index row")
		}
		return idx, nil
	}
	return nil, nil
}

func scanIndex(rows pgx.Rows) (*types.Index, error) {
	var ir indexRow
	dest := []any{
		&ir.ID,
		&ir.TableID,
		&ir.Schema,
		&ir.NumberOfAttributes,
		&ir.NumberOfKeyAttributes,
		&ir.IsUnique,
		&ir.IsPrimary,
		&ir.IsExclusion,
		&ir.IsImmediate,
		&ir.IsClustered,
		&ir.IsValid,
		&ir.CheckXmin,
		&ir.IsReady,
		&ir.IsLive,
		&ir.IsReplicaIdentity,
		&ir.KeyAttributes,
		&ir.Collation,
		&ir.Class,
		&ir.Options,
		&ir.IndexPredicate,
		&ir.Comment,
		&ir.IndexDefinition,
		&ir.AccessMethod,
		&ir.AttributesJSON,
	}
	if err := rows.Scan(dest...); err != nil {
		return nil, fmt.Errorf("sqlmeta: scan index row: %w", err)
	}

	attributes, err := decodeIndexAttributes(ir.AttributesJSON)
	if err != nil {
		return nil, err
	}

	idx := &types.Index{
		ID:                    ir.ID,
		TableID:               ir.TableID,
		Schema:                ir.Schema,
		NumberOfAttributes:    int(ir.NumberOfAttributes),
		NumberOfKeyAttributes: int(ir.NumberOfKeyAttributes),
		IsUnique:              ir.IsUnique,
		IsPrimary:             ir.IsPrimary,
		IsExclusion:           ir.IsExclusion,
		IsImmediate:           ir.IsImmediate,
		IsClustered:           ir.IsClustered,
		IsValid:               ir.IsValid,
		CheckXmin:             ir.CheckXmin,
		IsReady:               ir.IsReady,
		IsLive:                ir.IsLive,
		IsReplicaIdentity:     ir.IsReplicaIdentity,
		KeyAttributes:         ir.KeyAttributes,
		Collation:             ir.Collation,
		Class:                 ir.Class,
		Options:               ir.Options,
		IndexDefinition:       ir.IndexDefinition,
		AccessMethod:          ir.AccessMethod,
		IndexAttributes:       attributes,
	}

	if ir.IndexPredicate.Valid {
		idx.IndexPredicate = &ir.IndexPredicate.String
	}
	if ir.Comment.Valid {
		idx.Comment = &ir.Comment.String
	}

	return idx, nil
}

func decodeIndexAttributes(data []byte) ([]types.IndexAttribute, error) {
	if len(data) == 0 {
		return []types.IndexAttribute{}, nil
	}
	var raw []jsonIndexAttribute
	if err := json.Unmarshal(data, &raw); err != nil {
		return nil, fmt.Errorf("sqlmeta: decode index attributes: %w", err)
	}
	attrs := make([]types.IndexAttribute, len(raw))
	for i, attr := range raw {
		attrs[i] = types.IndexAttribute{
			AttributeNumber: attr.AttributeNumber,
			AttributeName:   attr.AttributeName,
			DataType:        attr.DataType,
		}
	}
	return attrs, nil
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
