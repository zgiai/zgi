package query

import (
	"context"
	"errors"
	"fmt"
	"math"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/zgiai/zgi/api/pkg/sql_base/sqlmeta/driver"
	"github.com/zgiai/zgi/api/pkg/sql_base/sqlmeta/types"
)

// Repository exposes the low-level capability to run arbitrary SQL.
type Repository interface {
	Execute(ctx context.Context, sql string, opts types.QueryOptions) ([]types.QueryRow, error)
}

type repository struct {
	pool *driver.Pool
}

var _ Repository = (*repository)(nil)

// NewRepository wires a query repository backed by the shared pgx pool.
func NewRepository(pool *driver.Pool) Repository {
	return &repository{pool: pool}
}

func (r *repository) Execute(ctx context.Context, sql string, opts types.QueryOptions) ([]types.QueryRow, error) {
	raw := r.rawPool()
	if raw == nil {
		return nil, errors.New("sqlmeta: nil pool")
	}
	if strings.TrimSpace(sql) == "" {
		return nil, fmt.Errorf("sqlmeta: empty query")
	}

	queryCtx := ctx
	var cancel context.CancelFunc
	if opts.StatementTimeout > 0 {
		queryCtx, cancel = context.WithTimeout(ctx, opts.StatementTimeout)
		defer cancel()
	}

	rows, err := raw.Query(queryCtx, sql, opts.Parameters...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	fieldDescs := rows.FieldDescriptions()
	columns := make([]string, len(fieldDescs))
	for i, fd := range fieldDescs {
		columns[i] = fd.Name
	}

	var data []types.QueryRow
	for rows.Next() {
		values, err := rows.Values()
		if err != nil {
			return nil, err
		}
		record := make(types.QueryRow, len(values))
		for idx, val := range values {
			record[columns[idx]] = normalizeValue(val)
		}
		data = append(data, record)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return data, nil
}

func (r *repository) rawPool() *pgxpool.Pool {
	if r == nil || r.pool == nil {
		return nil
	}
	return r.pool.Raw()
}

func normalizeValue(val any) any {
	switch v := val.(type) {
	case nil:
		return nil
	case []byte:
		if formatted, ok := formatUUIDFromBytes(v); ok {
			return formatted
		}
		if utf8.Valid(v) {
			return string(v)
		}
		return fmt.Sprintf("%x", v)
	case time.Time:
		return v.UTC()
	case [16]byte: // UUID as byte array
		if formatted, ok := formatUUIDFromBytes(v[:]); ok {
			return formatted
		}
		return fmt.Sprintf("%x-%x-%x-%x-%x",
			v[0:4], v[4:6], v[6:8], v[8:10], v[10:16])
	case pgtype.UUID:
		if v.Valid {
			if formatted, ok := formatUUIDFromBytes(v.Bytes[:]); ok {
				return formatted
			}
			return fmt.Sprintf("%x-%x-%x-%x-%x",
				v.Bytes[0:4], v.Bytes[4:6], v.Bytes[6:8], v.Bytes[8:10], v.Bytes[10:16])
		}
		return nil
	case pgtype.Date:
		if !v.Valid {
			return nil
		}
		if inf, ok := formatInfinityModifier(v.InfinityModifier); ok {
			return inf
		}
		return v.Time.UTC()
	case pgtype.Timestamp:
		if !v.Valid {
			return nil
		}
		if inf, ok := formatInfinityModifier(v.InfinityModifier); ok {
			return inf
		}
		return v.Time.UTC()
	case pgtype.Timestamptz:
		if !v.Valid {
			return nil
		}
		if inf, ok := formatInfinityModifier(v.InfinityModifier); ok {
			return inf
		}
		return v.Time.UTC()
	case pgtype.Array[pgtype.Text]:
		return normalizePgArray(v)
	case pgtype.Array[pgtype.Bool]:
		return normalizePgArray(v)
	case pgtype.Array[pgtype.Int2]:
		return normalizePgArray(v)
	case pgtype.Array[pgtype.Int4]:
		return normalizePgArray(v)
	case pgtype.Array[pgtype.Int8]:
		return normalizePgArray(v)
	case pgtype.Array[pgtype.Float4]:
		return normalizePgArray(v)
	case pgtype.Array[pgtype.Float8]:
		return normalizePgArray(v)
	case pgtype.Array[pgtype.Timestamp]:
		return normalizePgArray(v)
	case pgtype.Array[pgtype.Timestamptz]:
		return normalizePgArray(v)
	case pgtype.Array[pgtype.Date]:
		return normalizePgArray(v)
	case pgtype.Array[pgtype.UUID]:
		return normalizePgArray(v)
	case pgtype.Array[pgtype.Numeric]:
		return normalizePgArray(v)
	case []any:
		normalized := make([]any, len(v))
		for i, elem := range v {
			normalized[i] = normalizeValue(elem)
		}
		return collapseHomogeneousSlice(normalized)
	default:
		if valuer, ok := val.(sqlDriverValuer); ok {
			value, err := valuer.Value()
			if err == nil {
				if value == nil {
					return nil
				}
				return normalizeValue(value)
			}
		}
		return val
	}
}

type sqlDriverValuer interface {
	Value() (any, error)
}

func normalizePgArray[T any](arr pgtype.Array[T]) any {
	if !arr.Valid {
		return nil
	}
	values := make([]any, len(arr.Elements))
	for i, elem := range arr.Elements {
		values[i] = normalizeValue(any(elem))
	}
	return collapseHomogeneousSlice(values)
}

func collapseHomogeneousSlice(values []any) any {
	if len(values) == 0 {
		return []any{}
	}
	if strs, ok := tryStringSlice(values); ok {
		return strs
	}
	if bools, ok := tryBoolSlice(values); ok {
		return bools
	}
	if ints, ok := tryIntSlice(values); ok {
		return ints
	}
	if floats, ok := tryFloatSlice(values); ok {
		return floats
	}
	if times, ok := tryTimeSlice(values); ok {
		return times
	}
	return values
}

func tryStringSlice(values []any) ([]string, bool) {
	result := make([]string, len(values))
	for i, val := range values {
		if val == nil {
			return nil, false
		}
		str, ok := val.(string)
		if !ok {
			return nil, false
		}
		result[i] = str
	}
	return result, true
}

func tryBoolSlice(values []any) ([]bool, bool) {
	result := make([]bool, len(values))
	for i, val := range values {
		if val == nil {
			return nil, false
		}
		boolean, ok := val.(bool)
		if !ok {
			return nil, false
		}
		result[i] = boolean
	}
	return result, true
}

func tryIntSlice(values []any) ([]int64, bool) {
	result := make([]int64, len(values))
	for i, val := range values {
		if val == nil {
			return nil, false
		}
		intVal, ok := extractInt64(val)
		if !ok {
			return nil, false
		}
		result[i] = intVal
	}
	return result, true
}

func tryFloatSlice(values []any) ([]float64, bool) {
	result := make([]float64, len(values))
	for i, val := range values {
		if val == nil {
			return nil, false
		}
		floatVal, ok := extractFloat64(val)
		if !ok {
			return nil, false
		}
		result[i] = floatVal
	}
	return result, true
}

func tryTimeSlice(values []any) ([]time.Time, bool) {
	result := make([]time.Time, len(values))
	for i, val := range values {
		if val == nil {
			return nil, false
		}
		t, ok := val.(time.Time)
		if !ok {
			return nil, false
		}
		result[i] = t
	}
	return result, true
}

func extractInt64(value any) (int64, bool) {
	switch n := value.(type) {
	case int:
		return int64(n), true
	case int8:
		return int64(n), true
	case int16:
		return int64(n), true
	case int32:
		return int64(n), true
	case int64:
		return n, true
	case uint:
		if uint64(n) > math.MaxInt64 {
			return 0, false
		}
		return int64(n), true
	case uint8:
		return int64(n), true
	case uint16:
		return int64(n), true
	case uint32:
		return int64(n), true
	case uint64:
		if n > math.MaxInt64 {
			return 0, false
		}
		return int64(n), true
	default:
		return 0, false
	}
}

func extractFloat64(value any) (float64, bool) {
	switch n := value.(type) {
	case float32:
		return float64(n), true
	case float64:
		return n, true
	default:
		return 0, false
	}
}

func formatInfinityModifier(mod pgtype.InfinityModifier) (any, bool) {
	switch mod {
	case pgtype.Infinity:
		return "infinity", true
	case pgtype.NegativeInfinity:
		return "-infinity", true
	default:
		return nil, false
	}
}

func formatUUIDFromBytes(b []byte) (string, bool) {
	if len(b) != 16 {
		return "", false
	}
	u, err := uuid.FromBytes(b)
	if err != nil {
		return "", false
	}
	return u.String(), true
}

// FormatError renders a PgError with context around the failing SQL statement.
func FormatError(err *pgconn.PgError, sql string) string {
	if err == nil {
		return ""
	}
	var builder strings.Builder

	if err.Severity != "" {
		builder.WriteString(err.Severity)
		builder.WriteString(":  ")
	}
	if err.Code != "" {
		builder.WriteString(err.Code)
		builder.WriteString(": ")
	}
	builder.WriteString(err.Message)
	builder.WriteByte('\n')

	if err.Position > 0 {
		writeErrorLocation(&builder, sql, int(err.Position))
	}
	if err.Detail != "" {
		builder.WriteString("DETAIL:  ")
		builder.WriteString(err.Detail)
		builder.WriteByte('\n')
	}
	if err.Hint != "" {
		builder.WriteString("HINT:  ")
		builder.WriteString(err.Hint)
		builder.WriteByte('\n')
	}
	if err.InternalQuery != "" {
		builder.WriteString("QUERY:  ")
		builder.WriteString(err.InternalQuery)
		builder.WriteByte('\n')
	}
	if err.Where != "" {
		builder.WriteString("CONTEXT:  ")
		builder.WriteString(err.Where)
		builder.WriteByte('\n')
	}

	return builder.String()
}

func writeErrorLocation(builder *strings.Builder, sql string, position int) {
	// PostgreSQL position is 1-based.
	pos := position - 1
	if pos < 0 {
		pos = 0
	}
	if pos > len(sql) {
		pos = len(sql)
	}

	line := 1
	lineStart := 0
	for i, r := range sql {
		if i >= pos {
			break
		}
		if r == '\n' {
			line++
			lineStart = i + 1
		}
	}

	lineEnd := strings.IndexRune(sql[lineStart:], '\n')
	var lineText string
	if lineEnd == -1 {
		lineText = sql[lineStart:]
	} else {
		lineText = sql[lineStart : lineStart+lineEnd]
	}
	lineOffset := pos - lineStart
	builder.WriteString(fmt.Sprintf("LINE %d: %s\n", line, lineText))

	padding := 5 + len(fmt.Sprint(line)) + 2 + lineOffset
	builder.WriteString(strings.Repeat(" ", padding))
	builder.WriteString("^\n")
}
