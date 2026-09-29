package text2sql

import (
	"context"
	"fmt"

	"github.com/zgiai/zgi/api/pkg/sql_base"
)

// IndexedTable is an asset table together with its current physical schema.
type IndexedTable struct {
	AssetID string
	Table   *sql_base.Table
}

// Snapshot loads the current catalog, without creating indexes or calling a model.
func (r *IndexRepository) Snapshot(ctx context.Context, organizationID, sourceID string, sql sql_base.SQLBase) ([]IndexedTable, error) {
	var assets []struct {
		ID      string
		TableID int
	}
	if err := r.db.WithContext(ctx).Table("data_source_tables").Select("id, table_id").Where("organization_id = ? AND data_source_id = ?", organizationID, sourceID).Order("id").Find(&assets).Error; err != nil {
		return nil, err
	}
	result := make([]IndexedTable, 0, len(assets))
	for _, asset := range assets {
		table, err := sql.GetTable(ctx, asset.TableID)
		if err != nil {
			return nil, fmt.Errorf("load index catalog: %w", err)
		}
		if table == nil {
			return nil, ErrSchemaLoad
		}
		result = append(result, IndexedTable{asset.ID, table})
	}
	return result, nil
}

// CompleteIndexes selects exactly one current ready index for every asset.
// Historical fingerprints and partial builds must never narrow the query scope.
func CompleteIndexes(tables []IndexedTable, rows []SchemaIndex) ([]SchemaIndex, bool) {
	current := make([]SchemaIndex, 0, len(tables))
	for _, table := range tables {
		for _, row := range rows {
			if row.AssetTableID == table.AssetID && row.PhysicalTableID == table.Table.ID && row.Fingerprint == schemaFingerprint(table.Table) && row.Status == IndexReady {
				current = append(current, row)
				break
			}
		}
	}
	return current, len(tables) > 0 && len(current) == len(tables)
}

func MatchesSchema(row SchemaIndex, table *sql_base.Table) bool {
	return table != nil && row.PhysicalTableID == table.ID && row.Fingerprint == schemaFingerprint(table)
}
