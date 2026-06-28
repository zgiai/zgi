package repository

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/zgiai/zgi/api/internal/modules/datalibrary/model"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

func TestDocumentChunkRepositoryCreateBatchSplitsLargeInputs(t *testing.T) {
	queryLog := &documentChunkQueryLogger{}
	db := openDocumentChunkRepositoryTestDB(t, queryLog)
	repo := NewDocumentChunkRepository(db)
	ctx := context.Background()

	assetID := uuid.New()
	runID := uuid.New()
	chunks := make([]*model.DocumentChunk, 1200)
	for i := range chunks {
		chunks[i] = &model.DocumentChunk{
			OrganizationID:  "org-1",
			AssetID:         assetID,
			ProcessingRunID: runID,
			GenerationNo:    1,
			Position:        i,
			ChunkType:       model.DocumentChunkTypeAuto,
			Content:         "chunk content",
			ContentHash:     uuid.NewString(),
		}
	}

	if err := repo.CreateBatch(ctx, chunks); err != nil {
		t.Fatalf("CreateBatch: %v", err)
	}
	if queryLog.documentChunkInserts < 2 {
		t.Fatalf("expected large CreateBatch to split inserts, got %d insert statement(s)", queryLog.documentChunkInserts)
	}
	var count int64
	if err := db.Model(&model.DocumentChunk{}).Count(&count).Error; err != nil {
		t.Fatalf("count chunks: %v", err)
	}
	if count != int64(len(chunks)) {
		t.Fatalf("count=%d, want %d", count, len(chunks))
	}
}

type documentChunkQueryLogger struct {
	documentChunkInserts int
}

func (l *documentChunkQueryLogger) LogMode(level logger.LogLevel) logger.Interface {
	return l
}

func (l *documentChunkQueryLogger) Info(ctx context.Context, msg string, data ...any) {}

func (l *documentChunkQueryLogger) Warn(ctx context.Context, msg string, data ...any) {}

func (l *documentChunkQueryLogger) Error(ctx context.Context, msg string, data ...any) {}

func (l *documentChunkQueryLogger) Trace(ctx context.Context, begin time.Time, fc func() (string, int64), err error) {
	sql, _ := fc()
	normalized := strings.ToLower(strings.TrimSpace(sql))
	if strings.HasPrefix(normalized, `insert into "data_library_document_chunks"`) ||
		strings.HasPrefix(normalized, "insert into `data_library_document_chunks`") ||
		strings.HasPrefix(normalized, "insert into data_library_document_chunks") {
		l.documentChunkInserts++
	}
}

func openDocumentChunkRepositoryTestDB(t *testing.T, queryLog logger.Interface) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{Logger: queryLog})
	if err != nil {
		t.Fatal(err)
	}
	if err := db.Exec(`
		CREATE TABLE data_library_document_chunks (
			id text primary key,
			organization_id text not null,
			workspace_id text,
			asset_id text not null,
			processing_run_id text not null,
			generation_no integer not null,
			chunk_artifact_set_id text,
			parent_chunk_id text,
			position integer not null default 0,
			chunk_type text not null,
			content text not null,
			content_hash text not null,
			source_locator_json text not null default '{}',
			enabled boolean not null default true,
			status text not null default 'ready',
			metadata_json text not null default '{}',
			created_by text,
			updated_by text,
			created_at datetime,
			updated_at datetime,
			deleted_at datetime
		)
	`).Error; err != nil {
		t.Fatalf("create test table: %v", err)
	}
	return db
}
