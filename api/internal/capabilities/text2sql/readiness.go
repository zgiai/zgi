package text2sql

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/zgiai/zgi/api/pkg/sql_base"
)

// IndexedTable is an asset table together with its current physical schema.
type IndexedTable struct {
	AssetID string
	Table   *sql_base.Table
}

type indexedTableAsset struct {
	ID          string
	TableID     int
	Name        string
	Description string
}

// Snapshot loads the current catalog, without creating indexes or calling a model.
func (r *IndexRepository) Snapshot(ctx context.Context, organizationID, sourceID string, sql sql_base.SQLBase) ([]IndexedTable, error) {
	var assets []indexedTableAsset
	if err := r.db.WithContext(ctx).Table("data_source_tables").Select("id, table_id, name, description").Where("organization_id = ? AND data_source_id = ?", organizationID, sourceID).Order("id").Find(&assets).Error; err != nil {
		return nil, err
	}
	result := make([]IndexedTable, 0, len(assets))
	for _, asset := range assets {
		table, err := loadIndexedTable(ctx, sql, asset)
		if err != nil {
			return nil, fmt.Errorf("load index catalog: %w", err)
		}
		result = append(result, IndexedTable{asset.ID, table})
	}
	return result, nil
}

// SnapshotTable loads one asset together with the same metadata-enriched
// physical schema used by Snapshot and the indexer.
func (r *IndexRepository) SnapshotTable(ctx context.Context, assetID string, sql sql_base.SQLBase) (*IndexedTable, error) {
	var asset indexedTableAsset
	if err := r.db.WithContext(ctx).Table("data_source_tables").Select("id, table_id, name, description").Where("id = ?", assetID).Take(&asset).Error; err != nil {
		return nil, err
	}
	table, err := loadIndexedTable(ctx, sql, asset)
	if err != nil {
		return nil, err
	}
	return &IndexedTable{AssetID: asset.ID, Table: table}, nil
}

func loadIndexedTable(ctx context.Context, sql sql_base.SQLBase, asset indexedTableAsset) (*sql_base.Table, error) {
	table, err := sql.GetTable(ctx, asset.TableID)
	if err != nil {
		return nil, err
	}
	if table == nil {
		return nil, ErrSchemaLoad
	}
	copyTable := *table
	metadata, err := json.Marshal(struct {
		DisplayName     string  `json:"display_name"`
		Description     string  `json:"description"`
		PhysicalComment *string `json:"physical_comment,omitempty"`
	}{asset.Name, asset.Description, table.Comment})
	if err != nil {
		return nil, fmt.Errorf("encode table metadata: %w", err)
	}
	comment := string(metadata)
	copyTable.Comment = &comment
	return &copyTable, nil
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
