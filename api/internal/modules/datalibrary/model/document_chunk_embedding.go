package model

import (
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

const (
	DocumentChunkEmbeddingStatusReady    = "ready"
	DocumentChunkEmbeddingStatusIndexing = "indexing"
	DocumentChunkEmbeddingStatusError    = "error"
	DocumentChunkEmbeddingStatusDeleted  = "deleted"
)

type DocumentChunkEmbedding struct {
	ID                 uuid.UUID      `gorm:"type:uuid;primaryKey;default:uuid_generate_v4()" json:"id"`
	OrganizationID     string         `gorm:"type:varchar(255);not null;index:idx_data_library_chunk_embeddings_org_asset_generation,priority:1;column:organization_id" json:"organization_id"`
	WorkspaceID        *string        `gorm:"type:varchar(255);column:workspace_id" json:"workspace_id,omitempty"`
	AssetID            uuid.UUID      `gorm:"type:uuid;not null;index:idx_data_library_chunk_embeddings_org_asset_generation,priority:2;index:idx_data_library_chunk_embeddings_asset_status,priority:1;column:asset_id" json:"asset_id"`
	ChunkID            uuid.UUID      `gorm:"type:uuid;not null;uniqueIndex:uq_data_library_chunk_embeddings_active_model,priority:1;index:idx_data_library_chunk_embeddings_chunk_model,priority:1;column:chunk_id" json:"chunk_id"`
	ProcessingRunID    uuid.UUID      `gorm:"type:uuid;not null;column:processing_run_id" json:"processing_run_id"`
	GenerationNo       int64          `gorm:"not null;index:idx_data_library_chunk_embeddings_org_asset_generation,priority:3;column:generation_no" json:"generation_no"`
	EmbeddingProvider  string         `gorm:"type:varchar(128);not null;uniqueIndex:uq_data_library_chunk_embeddings_active_model,priority:2;index:idx_data_library_chunk_embeddings_chunk_model,priority:2;column:embedding_provider" json:"embedding_provider"`
	EmbeddingModel     string         `gorm:"type:varchar(255);not null;uniqueIndex:uq_data_library_chunk_embeddings_active_model,priority:3;index:idx_data_library_chunk_embeddings_chunk_model,priority:3;column:embedding_model" json:"embedding_model"`
	EmbeddingDimension int            `gorm:"not null;default:0;column:embedding_dimension" json:"embedding_dimension"`
	EmbeddingVector    Float32Array   `gorm:"type:real[];not null;default:'{}';column:embedding_vector" json:"embedding_vector"`
	ContentHash        string         `gorm:"type:varchar(255);not null;column:content_hash" json:"content_hash"`
	Status             string         `gorm:"type:varchar(32);not null;default:'ready';index:idx_data_library_chunk_embeddings_asset_status,priority:2;column:status" json:"status"`
	MetadataJSON       map[string]any `gorm:"type:jsonb;serializer:json;not null;default:'{}';column:metadata_json" json:"metadata_json,omitempty"`
	CreatedAt          time.Time      `gorm:"not null;default:CURRENT_TIMESTAMP" json:"created_at"`
	UpdatedAt          time.Time      `gorm:"not null;default:CURRENT_TIMESTAMP" json:"updated_at"`
	DeletedAt          gorm.DeletedAt `gorm:"index" json:"deleted_at,omitempty"`
}

func (DocumentChunkEmbedding) TableName() string {
	return "data_library_document_chunk_embeddings"
}

func (m *DocumentChunkEmbedding) BeforeCreate(tx *gorm.DB) error {
	if m.ID == uuid.Nil {
		m.ID = uuid.New()
	}
	if m.Status == "" {
		m.Status = DocumentChunkEmbeddingStatusReady
	}
	if m.EmbeddingVector == nil {
		m.EmbeddingVector = Float32Array{}
	}
	if m.MetadataJSON == nil {
		m.MetadataJSON = map[string]any{}
	}
	if m.CreatedAt.IsZero() {
		m.CreatedAt = time.Now()
	}
	if m.UpdatedAt.IsZero() {
		m.UpdatedAt = time.Now()
	}
	return nil
}

func (m *DocumentChunkEmbedding) BeforeUpdate(tx *gorm.DB) error {
	m.UpdatedAt = time.Now()
	return nil
}
