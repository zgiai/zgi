package text2sql

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/zgiai/zgi/api/pkg/sql_base"
	"github.com/zgiai/zgi/api/pkg/vectordb"
	"gorm.io/datatypes"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

type IndexStatus string

const (
	IndexPending  IndexStatus = "pending"
	IndexIndexing IndexStatus = "indexing"
	IndexReady    IndexStatus = "ready"
	IndexFailed   IndexStatus = "failed"
	IndexDeleting IndexStatus = "deleting"
)

type SchemaIndex struct {
	ID                string         `gorm:"type:uuid;primaryKey"`
	OrganizationID    string         `gorm:"type:uuid;not null"`
	WorkspaceID       string         `gorm:"type:uuid;not null"`
	DataSourceID      string         `gorm:"type:uuid;not null"`
	AssetTableID      string         `gorm:"type:uuid;not null"`
	PhysicalTableID   int            `gorm:"not null"`
	SchemaName        string         `gorm:"not null"`
	PhysicalTableName string         `gorm:"not null"`
	Fingerprint       string         `gorm:"not null"`
	EmbeddingProvider string         `gorm:"not null"`
	EmbeddingModel    string         `gorm:"not null"`
	CollectionName    string         `gorm:"not null"`
	VectorObjectID    string         `gorm:"not null"`
	Status            IndexStatus    `gorm:"not null"`
	ValueSamples      datatypes.JSON `gorm:"type:jsonb;not null"`
	LastErrorCode     *string
	IndexedAt         *time.Time
	CreatedAt         time.Time
	UpdatedAt         time.Time
}

func (SchemaIndex) TableName() string { return "text2sql_schema_indexes" }

type IndexRepository struct{ db *gorm.DB }

func NewIndexRepository(db *gorm.DB) *IndexRepository { return &IndexRepository{db: db} }

func (r *IndexRepository) Ready(ctx context.Context, workspaceID, dataSourceID, provider, model string) ([]SchemaIndex, error) {
	var rows []SchemaIndex
	err := r.db.WithContext(ctx).Where("workspace_id = ? AND data_source_id = ? AND embedding_provider = ? AND embedding_model = ? AND status = ?", workspaceID, dataSourceID, provider, model, IndexReady).Find(&rows).Error
	return rows, err
}

func (r *IndexRepository) Pending(ctx context.Context, limit int) ([]SchemaIndex, error) {
	var rows []SchemaIndex
	err := r.db.WithContext(ctx).Where("status IN ?", []IndexStatus{IndexPending, IndexFailed}).Order("updated_at ASC").Limit(limit).Find(&rows).Error
	return rows, err
}

func (r *IndexRepository) Save(ctx context.Context, row *SchemaIndex) error {
	if len(row.ValueSamples) == 0 {
		row.ValueSamples = datatypes.JSON([]byte(`{}`))
	}
	result := r.db.WithContext(ctx).Model(&SchemaIndex{}).Where("id = ?", row.ID).Select("*").Updates(row)
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected != 1 {
		return ErrIndexNotReady
	}
	return nil
}

func (r *IndexRepository) Superseded(ctx context.Context, current *SchemaIndex) ([]SchemaIndex, error) {
	var rows []SchemaIndex
	err := r.db.WithContext(ctx).Where(
		"workspace_id = ? AND data_source_id = ? AND asset_table_id = ? AND embedding_provider = ? AND embedding_model = ? AND id <> ? AND status IN ?",
		current.WorkspaceID, current.DataSourceID, current.AssetTableID, current.EmbeddingProvider, current.EmbeddingModel, current.ID,
		[]IndexStatus{IndexReady, IndexDeleting},
	).Find(&rows).Error
	return rows, err
}

func (r *IndexRepository) Delete(ctx context.Context, row *SchemaIndex) error {
	return r.db.WithContext(ctx).Delete(row).Error
}

func (r *IndexRepository) Ensure(ctx context.Context, row *SchemaIndex) error {
	identity := clause.OnConflict{
		Columns:   []clause.Column{{Name: "workspace_id"}, {Name: "data_source_id"}, {Name: "asset_table_id"}, {Name: "fingerprint"}, {Name: "embedding_provider"}, {Name: "embedding_model"}},
		DoNothing: true,
	}
	if err := r.db.WithContext(ctx).Clauses(identity).Create(row).Error; err != nil {
		return err
	}
	var existing SchemaIndex
	if err := r.db.WithContext(ctx).Where(
		"workspace_id = ? AND data_source_id = ? AND asset_table_id = ? AND fingerprint = ? AND embedding_provider = ? AND embedding_model = ?",
		row.WorkspaceID, row.DataSourceID, row.AssetTableID, row.Fingerprint, row.EmbeddingProvider, row.EmbeddingModel,
	).First(&existing).Error; err != nil {
		return err
	}
	*row = existing
	return nil
}

func NewSchemaIndex(organizationID, workspaceID, dataSourceID, assetTableID string, table *sql_base.Table, provider, model string) *SchemaIndex {
	if table == nil {
		return nil
	}
	return &SchemaIndex{
		ID: uuid.NewString(), OrganizationID: organizationID, WorkspaceID: workspaceID, DataSourceID: dataSourceID,
		AssetTableID: assetTableID, PhysicalTableID: table.ID, SchemaName: table.Schema, PhysicalTableName: table.Name,
		Fingerprint: schemaFingerprint(table), EmbeddingProvider: provider, EmbeddingModel: model,
		CollectionName: collectionName(workspaceID, provider, model), VectorObjectID: uuid.NewString(), Status: IndexPending,
		ValueSamples: datatypes.JSON([]byte(`{}`)),
	}
}

type Embedder interface {
	Embed(ctx context.Context, provider, model, text string) ([]float64, error)
}

type Indexer struct {
	repo     *IndexRepository
	sql      sql_base.SQLBase
	embedder Embedder
	vectors  vectordb.VectorDB
}

func NewIndexer(repo *IndexRepository, sql sql_base.SQLBase, embedder Embedder, vectors vectordb.VectorDB) *Indexer {
	return &Indexer{repo: repo, sql: sql, embedder: embedder, vectors: vectors}
}

func (i *Indexer) Process(ctx context.Context, row *SchemaIndex) error {
	if row == nil || i.repo == nil || i.sql == nil || i.embedder == nil || i.vectors == nil {
		return ErrIndexNotReady
	}
	if row.Status == IndexReady {
		return i.cleanupSuperseded(ctx, row)
	}
	row.Status = IndexIndexing
	if err := i.repo.Save(ctx, row); err != nil {
		return err
	}
	indexedTable, err := i.repo.snapshotTable(ctx, row.AssetTableID, i.sql)
	if err != nil {
		return i.fail(ctx, row, "schema_load_failed", err)
	}
	table := indexedTable.Table
	document := schemaDocument(table)
	fingerprint := schemaFingerprint(table)
	if fingerprint != row.Fingerprint {
		return i.fail(ctx, row, "fingerprint_changed", ErrIndexNotReady)
	}
	vector, err := i.embedder.Embed(ctx, row.EmbeddingProvider, row.EmbeddingModel, document)
	if err != nil {
		return i.fail(ctx, row, "embedding_failed", err)
	}
	if err := i.vectors.CreateClass(ctx, row.CollectionName, schemaIndexProperties()); err != nil {
		return i.fail(ctx, row, "vector_store_failed", err)
	}
	properties := map[string]any{
		"workspace_id": row.WorkspaceID, "data_source_id": row.DataSourceID, "asset_table_id": row.AssetTableID,
		"physical_table_id": row.PhysicalTableID, "schema_name": row.SchemaName, "table_name": row.PhysicalTableName,
		"fingerprint": row.Fingerprint, "content": document,
	}
	if err := i.vectors.StoreVector(ctx, row.VectorObjectID, row.CollectionName, properties, vector); err != nil {
		return i.fail(ctx, row, "vector_store_failed", err)
	}
	now := time.Now().UTC()
	row.Status = IndexReady
	row.LastErrorCode = nil
	row.IndexedAt = &now
	if err := i.repo.Save(ctx, row); err != nil {
		return err
	}
	return i.cleanupSuperseded(ctx, row)
}

func (i *Indexer) cleanupSuperseded(ctx context.Context, current *SchemaIndex) error {
	rows, err := i.repo.Superseded(ctx, current)
	if err != nil {
		return fmt.Errorf("list superseded schema indexes: %w", err)
	}
	for idx := range rows {
		old := &rows[idx]
		if old.Status == IndexReady {
			old.Status = IndexDeleting
			if err := i.repo.Save(ctx, old); err != nil {
				return fmt.Errorf("mark superseded schema index deleting: %w", err)
			}
		}
		if err := i.vectors.DeleteVector(ctx, old.VectorObjectID, old.CollectionName); err != nil {
			return fmt.Errorf("delete superseded schema vector: %w", err)
		}
		if err := i.repo.Delete(ctx, old); err != nil {
			return fmt.Errorf("delete superseded schema index: %w", err)
		}
	}
	return nil
}

func (i *Indexer) fail(ctx context.Context, row *SchemaIndex, code string, cause error) error {
	row.Status = IndexFailed
	row.LastErrorCode = &code
	writeCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 5*time.Second)
	defer cancel()
	if err := i.repo.Save(writeCtx, row); err != nil {
		return errors.Join(cause, fmt.Errorf("persist index failure: %w", err))
	}
	return cause
}

func schemaFingerprint(table *sql_base.Table) string {
	document := schemaDocument(table)
	sum := sha256.Sum256([]byte(document))
	return hex.EncodeToString(sum[:])
}

func schemaDocument(table *sql_base.Table) string {
	if table == nil {
		return ""
	}
	copyTable := *table
	copyTable.Columns = append([]sql_base.Column(nil), table.Columns...)
	copyTable.Relationships = append([]sql_base.Relation(nil), table.Relationships...)
	sort.Slice(copyTable.Columns, func(a, b int) bool {
		return copyTable.Columns[a].OrdinalPosition < copyTable.Columns[b].OrdinalPosition
	})
	sort.Slice(copyTable.Relationships, func(a, b int) bool {
		left := copyTable.Relationships[a]
		right := copyTable.Relationships[b]
		return left.SourceTableName+left.SourceColumnName+left.TargetTableName+left.TargetColumnName < right.SourceTableName+right.SourceColumnName+right.TargetTableName+right.TargetColumnName
	})
	payload := struct {
		Schema        string              `json:"schema"`
		Name          string              `json:"name"`
		Comment       *string             `json:"comment,omitempty"`
		Columns       []sql_base.Column   `json:"columns"`
		Relationships []sql_base.Relation `json:"relationships"`
	}{copyTable.Schema, copyTable.Name, copyTable.Comment, copyTable.Columns, copyTable.Relationships}
	data, _ := json.Marshal(payload)
	return string(data)
}

func collectionName(workspaceID, provider, model string) string {
	sum := sha256.Sum256([]byte(strings.Join([]string{workspaceID, provider, model}, "\x00")))
	return "Text2SQLSchema_" + hex.EncodeToString(sum[:8])
}

func schemaIndexProperties() []map[string]interface{} {
	text := func(name string) map[string]interface{} {
		return map[string]interface{}{"name": name, "dataType": []string{"text"}}
	}
	number := func(name string) map[string]interface{} {
		return map[string]interface{}{"name": name, "dataType": []string{"int"}}
	}
	return []map[string]interface{}{text("workspace_id"), text("data_source_id"), text("asset_table_id"), number("physical_table_id"), text("schema_name"), text("table_name"), text("fingerprint"), text("content")}
}
