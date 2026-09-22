package text2sql

import (
	"context"
	"encoding/json"
	"regexp"
	"sort"
	"strings"

	"github.com/zgiai/zgi/api/pkg/sql_base"
	"gorm.io/datatypes"
)

const valueSamplesVersion = 1

var (
	digitsOnlyPattern  = regexp.MustCompile(`^\d+$`)
	datePrefixPattern  = regexp.MustCompile(`^\d{4}-\d{2}-\d{2}`)
	valueColumnPattern = regexp.MustCompile(`(?i)(_id|_no|_uuid|_name|_code|_date|_time|_at|_amount|_rate|_qty|_count)$`)
)

type ColumnValues map[string][]string

type valueSamplesPayload struct {
	Version int          `json:"v"`
	Columns ColumnValues `json:"columns"`
}

// SampleColumnValues reads low-cardinality text values from PostgreSQL
// statistics. Sampling is intentionally best-effort: callers should keep the
// index usable when a source does not expose pg_stats.
func SampleColumnValues(ctx context.Context, client sql_base.SQLBase, table *sql_base.Table) ColumnValues {
	if client == nil || table == nil {
		return ColumnValues{}
	}

	candidates := make(map[string]struct{})
	for _, column := range table.Columns {
		if isCategoricalColumn(column) {
			candidates[column.Name] = struct{}{}
		}
	}
	if len(candidates) == 0 {
		return ColumnValues{}
	}

	result, err := client.ExecuteSQL(ctx, `
		SELECT
			s.attname AS column_name,
			unnest(s.most_common_vals::text::text[]) AS value
		FROM pg_stats s
		JOIN information_schema.columns c
		  ON c.table_schema = s.schemaname
		 AND c.table_name = s.tablename
		 AND c.column_name = s.attname
		WHERE s.schemaname = $1
		  AND s.tablename = $2
		  AND c.data_type IN ('text', 'character varying', 'character', 'name', 'USER-DEFINED')
		  AND s.most_common_vals IS NOT NULL
		  AND s.n_distinct >= 2
		  AND s.n_distinct <= 64
		  AND s.avg_width <= 24`, []interface{}{table.Schema, table.Name}, nil)
	if err != nil || result == nil {
		return ColumnValues{}
	}

	columnIndex, valueIndex := -1, -1
	for index, column := range result.Columns {
		switch strings.ToLower(column) {
		case "column_name":
			columnIndex = index
		case "value":
			valueIndex = index
		}
	}
	if columnIndex < 0 || valueIndex < 0 {
		return ColumnValues{}
	}

	values := make(ColumnValues)
	for _, row := range result.Rows {
		if columnIndex >= len(row) || valueIndex >= len(row) {
			continue
		}
		columnName, ok := sampleStringValue(row[columnIndex])
		if !ok {
			continue
		}
		if _, ok := candidates[columnName]; !ok {
			continue
		}
		value, ok := sampleStringValue(row[valueIndex])
		if !ok || !acceptableSampleValue(value) {
			continue
		}
		if !contains(values[columnName], value) {
			values[columnName] = append(values[columnName], value)
		}
	}

	for column, columnValues := range values {
		sort.Strings(columnValues)
		if len(columnValues) > 16 {
			values[column] = columnValues[:16]
		}
		if len(values[column]) == 0 {
			delete(values, column)
		}
	}
	return values
}

func EncodeValueSamples(values ColumnValues) datatypes.JSON {
	encoded, err := json.Marshal(valueSamplesPayload{Version: valueSamplesVersion, Columns: values})
	if err != nil {
		return datatypes.JSON([]byte(`{}`))
	}
	return datatypes.JSON(encoded)
}

// ValueSamplesInitialized reports whether sampling has been attempted for a
// stored payload, including a valid empty result.
func ValueSamplesInitialized(raw datatypes.JSON) bool {
	var payload valueSamplesPayload
	return len(raw) > 0 && json.Unmarshal(raw, &payload) == nil && payload.Version == valueSamplesVersion
}

func DecodeValueSamples(raw datatypes.JSON) ColumnValues {
	if len(raw) == 0 {
		return ColumnValues{}
	}
	var payload valueSamplesPayload
	if err := json.Unmarshal(raw, &payload); err != nil || payload.Version != valueSamplesVersion {
		return ColumnValues{}
	}
	return payload.Columns
}

func isCategoricalColumn(column sql_base.Column) bool {
	if valueColumnPattern.MatchString(column.Name) {
		return false
	}
	if len(column.Enums) > 0 {
		return true
	}
	dataType := strings.ToLower(strings.TrimSpace(column.DataType))
	switch dataType {
	case "text", "varchar", "character varying", "char", "character", "bpchar", "name":
		return true
	default:
		return false
	}
}

func acceptableSampleValue(value string) bool {
	value = strings.TrimSpace(value)
	if value == "" || len([]rune(value)) > 24 || digitsOnlyPattern.MatchString(value) || datePrefixPattern.MatchString(value) {
		return false
	}
	switch strings.ToLower(value) {
	case "true", "false", "t", "f", "y", "n", "yes", "no":
		return false
	default:
		return true
	}
}

func sampleStringValue(value any) (string, bool) {
	switch value := value.(type) {
	case string:
		return strings.TrimSpace(value), true
	case []byte:
		return strings.TrimSpace(string(value)), true
	default:
		return "", false
	}
}

func contains(values []string, target string) bool {
	for _, value := range values {
		if value == target {
			return true
		}
	}
	return false
}
