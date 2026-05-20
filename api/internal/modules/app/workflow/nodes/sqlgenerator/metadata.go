package sqlgenerator

import (
	"context"
	"fmt"
	"sort"
	"strings"

	"github.com/zgiai/ginext/internal/modules/app/workflow/nodes/calldatabase"
	"github.com/zgiai/ginext/pkg/sql_base"
)

const sqlMetaTablePrefix = "zgi_base_"

type ColumnMetadata struct {
	Name         string  `json:"name"`
	DataType     string  `json:"data_type"`
	IsNullable   bool    `json:"is_nullable"`
	DefaultValue any     `json:"default_value,omitempty"`
	Comment      *string `json:"comment,omitempty"`
	Position     int     `json:"position"`
}

type TableMetadata struct {
	Schema      string           `json:"schema"`
	Name        string           `json:"name"`
	Comment     *string          `json:"comment,omitempty"`
	Columns     []ColumnMetadata `json:"columns"`
	Source      string           `json:"source"`
	ColumnCount int              `json:"column_count"`
}

func (n *Node) loadTableMetadata(ctx context.Context) ([]TableMetadata, error) {
	if n.sqlClient == nil || len(n.NodeData.DataSource.Tables) == 0 {
		return nil, nil
	}
	result := make([]TableMetadata, 0, len(n.NodeData.DataSource.Tables))
	for _, ref := range n.NodeData.DataSource.Tables {
		var meta *TableMetadata
		if ref.TableID > 0 {
			tbl, err := n.sqlClient.GetTable(ctx, ref.TableID)
			if err == nil && tbl != nil {
				converted := tableMetadataFromSQLTable(tbl, ref)
				meta = &converted
			}
		}
		if meta == nil {
			selMeta := tableMetadataFromSelection(ref)
			meta = &selMeta
		}
		result = append(result, *meta)
	}
	return result, nil
}

func convertColumns(cols []sql_base.Column) []ColumnMetadata {
	if len(cols) == 0 {
		return []ColumnMetadata{}
	}
	out := make([]ColumnMetadata, 0, len(cols))
	for _, col := range cols {
		out = append(out, ColumnMetadata{
			Name:         col.Name,
			DataType:     normalizeDataType(col),
			IsNullable:   col.IsNullable,
			DefaultValue: col.DefaultValue,
			Comment:      col.Comment,
			Position:     col.OrdinalPosition,
		})
	}
	sort.SliceStable(out, func(i, j int) bool {
		return out[i].Position < out[j].Position
	})
	return out
}

func tableMetadataFromSQLTable(tbl *sql_base.Table, ref calldatabase.TableRef) TableMetadata {
	columns := convertColumns(tbl.Columns)
	return TableMetadata{
		Schema:      displaySchema(ref.Schema, tbl.Schema),
		Name:        normalizeTableName(tbl.Name), // Remove zgi_base_ prefix
		Comment:     tbl.Comment,
		Columns:     columns,
		Source:      "metadata_service",
		ColumnCount: len(columns),
	}
}

func tableMetadataFromSelection(ref calldatabase.TableRef) TableMetadata {
	fallbackColumns := make([]ColumnMetadata, 0, len(ref.Columns))
	for idx, col := range ref.Columns {
		if strings.TrimSpace(col) == "" {
			continue
		}
		fallbackColumns = append(fallbackColumns, ColumnMetadata{
			Name:       col,
			DataType:   "text",
			IsNullable: true,
			Position:   idx + 1,
		})
	}
	return TableMetadata{
		Schema:      displaySchema(ref.Schema, ref.Schema),
		Name:        ref.Name,
		Columns:     fallbackColumns,
		Source:      "selection_fallback",
		ColumnCount: len(fallbackColumns),
	}
}

func normalizeDataType(col sql_base.Column) string {
	if normalized := (&col).NormalizeDataType(); normalized != "" {
		return normalized
	}
	return col.Format
}

func schemaKey(schema string) string {
	s := strings.TrimSpace(schema)
	if s == "" {
		return "public"
	}
	return strings.ToLower(s)
}

func displaySchema(requested string, fallback string) string {
	if strings.TrimSpace(requested) != "" {
		return requested
	}
	if strings.TrimSpace(fallback) != "" {
		return fallback
	}
	return "public"
}

func tableKey(schema, name string) string {
	return schemaKey(schema) + ":" + strings.ToLower(normalizeTableName(name))
}

func normalizeTableName(name string) string {
	if strings.HasPrefix(name, sqlMetaTablePrefix) {
		return strings.TrimPrefix(name, sqlMetaTablePrefix)
	}
	return name
}

func renderTableMetadata(tables []TableMetadata, cfg MetadataConfig) string {
	if len(tables) == 0 {
		return ""
	}

	var sb strings.Builder
	for idx, table := range tables {
		if idx > 0 {
			sb.WriteString("\n")
		}
		sb.WriteString(fmt.Sprintf("--- table: %s.%s ---\n", table.Schema, table.Name))
		if len(table.Columns) == 0 {
			sb.WriteString("(no column information)\n")
			continue
		}

		limit := cfg.MaxColumns
		for i, col := range table.Columns {
			if limit > 0 && i >= limit {
				sb.WriteString("... (more columns omitted)\n")
				break
			}
			line := fmt.Sprintf("%s %s", col.Name, col.DataType)
			if !col.IsNullable {
				line += " NOT NULL"
			}
			if col.DefaultValue != nil {
				line += fmt.Sprintf(" DEFAULT %s", formatAny(col.DefaultValue))
			}
			if cfg.IncludeComments && col.Comment != nil && strings.TrimSpace(*col.Comment) != "" {
				line += fmt.Sprintf(" -- %s", strings.TrimSpace(*col.Comment))
			}
			sb.WriteString(line)
			sb.WriteString("\n")
		}
	}

	return strings.TrimSpace(sb.String())
}

func renderDDLMetadata(tables []TableMetadata, cfg MetadataConfig) string {
	if len(tables) == 0 {
		return ""
	}

	var sb strings.Builder
	for idx, table := range tables {
		if idx > 0 {
			sb.WriteString("\n")
		}
		sb.WriteString(fmt.Sprintf("CREATE TABLE %s.%s (\n", table.Schema, table.Name))
		if len(table.Columns) == 0 {
			sb.WriteString(");\n")
			continue
		}

		limit := cfg.MaxColumns
		for i, col := range table.Columns {
			if limit > 0 && i >= limit {
				sb.WriteString("  -- more columns omitted\n")
				break
			}
			line := fmt.Sprintf("  %s %s", col.Name, col.DataType)
			if !col.IsNullable {
				line += " NOT NULL"
			}
			if col.DefaultValue != nil {
				line += fmt.Sprintf(" DEFAULT %s", formatAny(col.DefaultValue))
			}
			if cfg.IncludeComments && col.Comment != nil && strings.TrimSpace(*col.Comment) != "" {
				line += fmt.Sprintf(" -- %s", strings.TrimSpace(*col.Comment))
			}
			if i < len(table.Columns)-1 {
				line += ","
			}
			sb.WriteString(line)
			sb.WriteString("\n")
		}
		sb.WriteString(");\n")
	}
	return strings.TrimSpace(sb.String())
}

func fallbackMetadataFromSelection(refs []calldatabase.TableRef) []TableMetadata {
	result := make([]TableMetadata, 0, len(refs))
	for _, ref := range refs {
		columns := make([]ColumnMetadata, 0, len(ref.Columns))
		for idx, col := range ref.Columns {
			if strings.TrimSpace(col) == "" {
				continue
			}
			columns = append(columns, ColumnMetadata{
				Name:       col,
				DataType:   "text",
				IsNullable: true,
				Position:   idx + 1,
			})
		}
		result = append(result, TableMetadata{
			Schema:      displaySchema(ref.Schema, ref.Schema),
			Name:        ref.Name,
			Columns:     columns,
			Source:      "selection_fallback",
			ColumnCount: len(columns),
		})
	}
	return result
}
