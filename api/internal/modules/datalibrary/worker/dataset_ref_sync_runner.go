package worker

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	datalibModel "github.com/zgiai/zgi/api/internal/modules/datalibrary/model"
	datalibRepo "github.com/zgiai/zgi/api/internal/modules/datalibrary/repository"
	datasetModel "github.com/zgiai/zgi/api/internal/modules/dataset/model"
	"github.com/zgiai/zgi/api/pkg/vectordb"
)

type datasetRefSyncRefStore interface {
	GetByID(ctx context.Context, id uuid.UUID) (*datalibModel.KnowledgeBaseAssetRef, error)
	MarkSyncing(ctx context.Context, organizationID string, id uuid.UUID, syncRunID uuid.UUID) (*datalibModel.KnowledgeBaseAssetRef, error)
	MarkSynced(ctx context.Context, organizationID string, id uuid.UUID, syncRunID uuid.UUID, datasetDocumentID uuid.UUID, generationNo int64, syncedAt time.Time) (*datalibModel.KnowledgeBaseAssetRef, error)
	MarkFailed(ctx context.Context, organizationID string, id uuid.UUID, syncRunID uuid.UUID, errorCode, errorMessage string) (*datalibModel.KnowledgeBaseAssetRef, error)
}

type datasetRefSyncAssetStore interface {
	GetAssetByID(ctx context.Context, id uuid.UUID) (*datalibModel.DocumentAsset, error)
}

type datasetRefSyncDatasetStore interface {
	GetByID(ctx context.Context, id string) (*datasetModel.Dataset, error)
}

type datasetRefSyncDocumentStore interface {
	GetByID(ctx context.Context, id string) (*datasetModel.Document, error)
	GetNextPosition(ctx context.Context, datasetID string) (int, error)
	Create(ctx context.Context, document *datasetModel.Document) error
	Update(ctx context.Context, document *datasetModel.Document) error
	EnableDocuments(ctx context.Context, datasetID string, documentIDs []string) error
	DisableDocuments(ctx context.Context, datasetID string, documentIDs []string, accountID string) error
	GetSegmentsByDocumentID(ctx context.Context, documentID string) ([]*datasetModel.DocumentSegment, error)
	GetChildChunksBySegmentID(ctx context.Context, segmentID string) ([]datasetModel.ChildChunk, error)
	CreateDocumentSegment(ctx context.Context, segment *datasetModel.DocumentSegment) error
	CreateChildChunk(ctx context.Context, childChunk *datasetModel.ChildChunk) error
	DeleteDocumentSegmentQuestionsByDocumentID(ctx context.Context, documentID string) error
	DeleteChildChunksByDocumentID(ctx context.Context, documentID string) error
	DeleteDocumentSegmentsByDocumentID(ctx context.Context, documentID string) error
	Delete(ctx context.Context, id string) error
}

type datasetRefSyncChunkStore interface {
	List(ctx context.Context, filter datalibRepo.DocumentChunkListFilter) ([]*datalibModel.DocumentChunk, int64, error)
}

type datasetRefSyncEmbeddingStore interface {
	List(ctx context.Context, filter datalibRepo.DocumentChunkEmbeddingListFilter) ([]*datalibModel.DocumentChunkEmbedding, int64, error)
}

type DatasetRefSyncRunnerDeps struct {
	Refs       datasetRefSyncRefStore
	Assets     datasetRefSyncAssetStore
	Datasets   datasetRefSyncDatasetStore
	Documents  datasetRefSyncDocumentStore
	Chunks     datasetRefSyncChunkStore
	Embeddings datasetRefSyncEmbeddingStore
	VectorDB   vectordb.VectorDB
}

type DatasetRefSyncRunner struct {
	refs       datasetRefSyncRefStore
	assets     datasetRefSyncAssetStore
	datasets   datasetRefSyncDatasetStore
	documents  datasetRefSyncDocumentStore
	chunks     datasetRefSyncChunkStore
	embeddings datasetRefSyncEmbeddingStore
	vectorDB   vectordb.VectorDB
}

func NewDatasetRefSyncRunner(deps DatasetRefSyncRunnerDeps) *DatasetRefSyncRunner {
	return &DatasetRefSyncRunner{
		refs:       deps.Refs,
		assets:     deps.Assets,
		datasets:   deps.Datasets,
		documents:  deps.Documents,
		chunks:     deps.Chunks,
		embeddings: deps.Embeddings,
		vectorDB:   deps.VectorDB,
	}
}

func (r *DatasetRefSyncRunner) Run(ctx context.Context, payload DatasetRefSyncPayload) error {
	if r == nil || r.refs == nil || r.assets == nil {
		return fmt.Errorf("dataset ref sync runner is not configured")
	}
	refID, err := uuid.Parse(payload.RefID)
	if err != nil || refID == uuid.Nil {
		return fmt.Errorf("invalid ref_id %q", payload.RefID)
	}
	assetID, err := uuid.Parse(payload.AssetID)
	if err != nil || assetID == uuid.Nil {
		return fmt.Errorf("invalid asset_id %q", payload.AssetID)
	}
	syncRunID, err := uuid.Parse(payload.SyncRunID)
	if err != nil || syncRunID == uuid.Nil {
		return fmt.Errorf("invalid sync_run_id %q", payload.SyncRunID)
	}

	ref, err := r.refs.GetByID(ctx, refID)
	if err != nil {
		return err
	}
	if ref == nil {
		return nil
	}
	if ref.SyncRunID == nil || *ref.SyncRunID != syncRunID {
		return nil
	}
	if ref.AssetID != assetID || ref.DatasetID != payload.DatasetID {
		_, markErr := r.refs.MarkFailed(ctx, ref.OrganizationID, ref.ID, syncRunID, "ref_payload_mismatch", "sync task payload does not match ref")
		return markErr
	}

	asset, err := r.assets.GetAssetByID(ctx, assetID)
	if err != nil {
		return err
	}
	if asset == nil || asset.OrganizationID != ref.OrganizationID {
		_, markErr := r.refs.MarkFailed(ctx, ref.OrganizationID, ref.ID, syncRunID, "asset_not_found", "asset not found")
		return markErr
	}
	if asset.ProductStatus != datalibModel.DocumentAssetProductStatusReady || asset.VectorStatus != datalibModel.DocumentAssetVectorStatusReady {
		_, markErr := r.refs.MarkFailed(ctx, ref.OrganizationID, ref.ID, syncRunID, "asset_not_ready", "asset is not ready for dataset sync")
		return markErr
	}
	if payload.GenerationNo != asset.GenerationNo {
		_, markErr := r.refs.MarkFailed(ctx, ref.OrganizationID, ref.ID, syncRunID, "generation_mismatch", "sync task generation does not match asset current generation")
		return markErr
	}

	_, err = r.refs.MarkSyncing(ctx, ref.OrganizationID, ref.ID, syncRunID)
	if err != nil {
		return err
	}

	if err := r.copyAssetToDataset(ctx, ref, asset, syncRunID); err != nil {
		_, markErr := r.refs.MarkFailed(ctx, ref.OrganizationID, ref.ID, syncRunID, "copy_failed", err.Error())
		if markErr != nil {
			return markErr
		}
		return nil
	}
	return nil
}

func (r *DatasetRefSyncRunner) copyAssetToDataset(ctx context.Context, ref *datalibModel.KnowledgeBaseAssetRef, asset *datalibModel.DocumentAsset, syncRunID uuid.UUID) error {
	if r.datasets == nil || r.documents == nil || r.chunks == nil || r.embeddings == nil || r.vectorDB == nil {
		return fmt.Errorf("dataset ref sync copy dependencies are not configured")
	}
	dataset, err := r.datasets.GetByID(ctx, ref.DatasetID)
	if err != nil {
		return fmt.Errorf("load dataset: %w", err)
	}
	if dataset == nil || dataset.OrganizationID != ref.OrganizationID {
		return fmt.Errorf("dataset not found")
	}

	oldDocumentID := refDatasetDocumentID(ref)
	if oldDocumentID != "" {
		if err := r.documents.DisableDocuments(ctx, ref.DatasetID, []string{oldDocumentID}, ref.CreatedBy); err != nil {
			return fmt.Errorf("disable old document: %w", err)
		}
	}

	chunks, err := r.listCurrentChunks(ctx, asset)
	if err != nil {
		return err
	}
	if len(chunks) == 0 {
		return fmt.Errorf("asset has no ready chunks")
	}
	embeddings, err := r.listCurrentEmbeddings(ctx, asset)
	if err != nil {
		return err
	}

	document, err := r.createDatasetDocument(ctx, ref, asset, chunks)
	if err != nil {
		return err
	}
	copyErr := r.copyChunksToDataset(ctx, dataset, document, chunks, embeddings)
	if copyErr != nil {
		_ = r.deleteDatasetDocumentTree(ctx, document.ID)
		return copyErr
	}

	now := time.Now()
	document.Enabled = true
	document.IndexingStatus = datasetModel.DocumentStatusCompleted
	document.CompletedAt = &now
	document.ParsingCompletedAt = &now
	document.CleaningCompletedAt = &now
	document.SplittingCompletedAt = &now
	document.UpdatedAt = now
	if err := r.documents.Update(ctx, document); err != nil {
		_ = r.deleteDatasetDocumentTree(ctx, document.ID)
		return fmt.Errorf("complete document: %w", err)
	}
	if err := r.documents.EnableDocuments(ctx, document.DatasetID, []string{document.ID}); err != nil {
		_ = r.deleteDatasetDocumentTree(ctx, document.ID)
		return fmt.Errorf("enable document: %w", err)
	}

	documentID, err := uuid.Parse(document.ID)
	if err != nil {
		_ = r.deleteDatasetDocumentTree(ctx, document.ID)
		return fmt.Errorf("parse new document id: %w", err)
	}
	if _, err := r.refs.MarkSynced(ctx, ref.OrganizationID, ref.ID, syncRunID, documentID, asset.GenerationNo, now); err != nil {
		_ = r.deleteDatasetDocumentTree(ctx, document.ID)
		return fmt.Errorf("mark ref synced: %w", err)
	}
	if oldDocumentID != "" && oldDocumentID != document.ID {
		if err := r.deleteDatasetDocumentTree(ctx, oldDocumentID); err != nil {
			return fmt.Errorf("delete old document: %w", err)
		}
	}
	return nil
}

func (r *DatasetRefSyncRunner) createDatasetDocument(ctx context.Context, ref *datalibModel.KnowledgeBaseAssetRef, asset *datalibModel.DocumentAsset, chunks []*datalibModel.DocumentChunk) (*datasetModel.Document, error) {
	position, err := r.documents.GetNextPosition(ctx, ref.DatasetID)
	if err != nil {
		return nil, fmt.Errorf("get next document position: %w", err)
	}
	documentID := uuid.New().String()
	createdBy := ref.CreatedBy
	if createdBy == "" {
		createdBy = asset.CreatedBy
	}
	now := time.Now()
	wordCount := 0
	for _, chunk := range chunks {
		if isDatasetPrimaryChunk(chunk) {
			wordCount += len([]rune(chunk.Content))
		}
	}
	dataSourceInfo, err := json.Marshal(map[string]any{
		"asset_id":       asset.ID.String(),
		"source_file_id": asset.SourceFileID,
		"generation_no":  asset.GenerationNo,
	})
	if err != nil {
		return nil, fmt.Errorf("marshal data source info: %w", err)
	}
	docForm := "text_model"
	if hasChildChunks(chunks) {
		docForm = "hierarchical_model"
	}
	document := &datasetModel.Document{
		ID:                  documentID,
		OrganizationID:      ref.OrganizationID,
		DatasetID:           ref.DatasetID,
		Position:            position,
		DataSourceType:      "file_asset",
		DataSourceInfo:      stringPtr(string(dataSourceInfo)),
		Batch:               ref.ID.String(),
		Name:                asset.Title,
		CreatedFrom:         "data_library",
		CreatedBy:           createdBy,
		CreatedAt:           now,
		FileID:              stringPtr(asset.SourceFileID),
		WordCount:           &wordCount,
		Tokens:              &wordCount,
		IndexingStatus:      datasetModel.DocumentStatusIndexing,
		Enabled:             false,
		UpdatedAt:           now,
		DocForm:             docForm,
		DocMetadata:         datasetModel.JSONMap{"asset_id": asset.ID.String(), "generation_no": asset.GenerationNo},
		ProcessingStartedAt: &now,
	}
	if err := r.documents.Create(ctx, document); err != nil {
		return nil, fmt.Errorf("create dataset document: %w", err)
	}
	return document, nil
}

func (r *DatasetRefSyncRunner) copyChunksToDataset(ctx context.Context, dataset *datasetModel.Dataset, document *datasetModel.Document, chunks []*datalibModel.DocumentChunk, embeddings map[uuid.UUID]*datalibModel.DocumentChunkEmbedding) error {
	className := datasetModel.GenCollectionNameByID(document.DatasetID)
	if err := r.vectorDB.CreateClass(ctx, className, defaultDatasetRefSyncVectorClassProperties()); err != nil {
		return fmt.Errorf("ensure vector class: %w", err)
	}

	childrenByParent := groupChildChunksByParent(chunks)
	position := 0
	for _, chunk := range chunks {
		if chunk == nil || !isDatasetPrimaryChunk(chunk) {
			continue
		}
		position++
		segmentID := uuid.New().String()
		indexNodeID := segmentID
		indexNodeHash := contentHash(chunk.Content)
		childChunks := childrenByParent[chunk.ID]
		if len(childChunks) > 0 {
			indexNodeID = uuid.New().String()
		}
		segment := &datasetModel.DocumentSegment{
			ID:                  segmentID,
			OrganizationID:      document.OrganizationID,
			DatasetID:           document.DatasetID,
			DocumentID:          document.ID,
			Position:            position,
			Content:             chunk.Content,
			WordCount:           len([]rune(chunk.Content)),
			Tokens:              len([]rune(chunk.Content)),
			Keywords:            datasetModel.JSONMap{},
			IndexNodeID:         &indexNodeID,
			IndexNodeHash:       &indexNodeHash,
			Enabled:             document.Enabled,
			Status:              datasetModel.SegmentStatusCompleted,
			GraphIndexingStatus: "pending",
			CreatedBy:           document.CreatedBy,
			CreatedAt:           time.Now(),
			CompletedAt:         timePtr(time.Now()),
		}
		if err := r.documents.CreateDocumentSegment(ctx, segment); err != nil {
			return fmt.Errorf("create segment: %w", err)
		}
		if len(childChunks) == 0 {
			embedding := embeddings[chunk.ID]
			if embedding == nil {
				return fmt.Errorf("missing embedding for chunk %s", chunk.ID)
			}
			if err := r.storeVector(ctx, className, indexNodeID, document.ID, document.DatasetID, chunk.Content, indexNodeHash, embedding); err != nil {
				return err
			}
			continue
		}
		for childPosition, child := range childChunks {
			embedding := embeddings[child.ID]
			if embedding == nil {
				return fmt.Errorf("missing embedding for child chunk %s", child.ID)
			}
			childIndexNodeID := uuid.New().String()
			childIndexNodeHash := contentHash(child.Content)
			childChunk := &datasetModel.ChildChunk{
				ID:             uuid.New().String(),
				OrganizationID: document.OrganizationID,
				DatasetID:      document.DatasetID,
				DocumentID:     document.ID,
				SegmentID:      segment.ID,
				Position:       childPosition + 1,
				Content:        child.Content,
				WordCount:      len([]rune(child.Content)),
				IndexNodeID:    &childIndexNodeID,
				IndexNodeHash:  &childIndexNodeHash,
				Type:           datasetModel.ChildChunkTypeAutomatic,
				CreatedBy:      document.CreatedBy,
				CreatedAt:      time.Now(),
				CompletedAt:    timePtr(time.Now()),
			}
			if err := r.documents.CreateChildChunk(ctx, childChunk); err != nil {
				return fmt.Errorf("create child chunk: %w", err)
			}
			if err := r.storeVector(ctx, className, childIndexNodeID, document.ID, document.DatasetID, child.Content, childIndexNodeHash, embedding); err != nil {
				return err
			}
		}
	}
	if position == 0 {
		return fmt.Errorf("asset has no primary chunks")
	}
	return nil
}

func (r *DatasetRefSyncRunner) storeVector(ctx context.Context, className, indexNodeID, documentID, datasetID, content, hash string, embedding *datalibModel.DocumentChunkEmbedding) error {
	vector := make([]float64, 0, len(embedding.EmbeddingVector))
	for _, item := range embedding.EmbeddingVector {
		vector = append(vector, float64(item))
	}
	if len(vector) == 0 {
		return fmt.Errorf("empty embedding vector for chunk %s", embedding.ChunkID)
	}
	properties := map[string]interface{}{
		"text":        content,
		"doc_id":      indexNodeID,
		"doc_hash":    hash,
		"document_id": documentID,
		"dataset_id":  datasetID,
	}
	if err := r.vectorDB.StoreVector(ctx, indexNodeID, className, properties, vector); err != nil {
		return fmt.Errorf("store vector %s: %w", indexNodeID, err)
	}
	return nil
}

func (r *DatasetRefSyncRunner) deleteDatasetDocumentTree(ctx context.Context, documentID string) error {
	if strings.TrimSpace(documentID) == "" {
		return nil
	}
	var firstErr error
	segments, err := r.documents.GetSegmentsByDocumentID(ctx, documentID)
	if err != nil {
		firstErr = err
	}
	for _, segment := range segments {
		if segment == nil {
			continue
		}
		children, err := r.documents.GetChildChunksBySegmentID(ctx, segment.ID)
		if err != nil && firstErr == nil {
			firstErr = err
		}
		for _, child := range children {
			if child.IndexNodeID != nil {
				if err := r.vectorDB.DeleteVector(ctx, *child.IndexNodeID, datasetModel.GenCollectionNameByID(child.DatasetID)); err != nil && firstErr == nil {
					firstErr = err
				}
			}
		}
		if segment.IndexNodeID != nil {
			if err := r.vectorDB.DeleteVector(ctx, *segment.IndexNodeID, datasetModel.GenCollectionNameByID(segment.DatasetID)); err != nil && firstErr == nil {
				firstErr = err
			}
		}
	}
	if err := r.documents.DeleteDocumentSegmentQuestionsByDocumentID(ctx, documentID); err != nil && firstErr == nil {
		firstErr = err
	}
	if err := r.documents.DeleteChildChunksByDocumentID(ctx, documentID); err != nil && firstErr == nil {
		firstErr = err
	}
	if err := r.documents.DeleteDocumentSegmentsByDocumentID(ctx, documentID); err != nil && firstErr == nil {
		firstErr = err
	}
	if err := r.documents.Delete(ctx, documentID); err != nil && firstErr == nil {
		firstErr = err
	}
	return firstErr
}

func (r *DatasetRefSyncRunner) listCurrentChunks(ctx context.Context, asset *datalibModel.DocumentAsset) ([]*datalibModel.DocumentChunk, error) {
	generationNo := asset.GenerationNo
	var out []*datalibModel.DocumentChunk
	for offset := 0; ; offset += 500 {
		items, total, err := r.chunks.List(ctx, datalibRepo.DocumentChunkListFilter{
			OrganizationID: asset.OrganizationID,
			AssetID:        asset.ID,
			GenerationNo:   &generationNo,
			Enabled:        boolPtr(true),
			Status:         datalibModel.DocumentChunkStatusReady,
			Limit:          500,
			Offset:         offset,
		})
		if err != nil {
			return nil, fmt.Errorf("list asset chunks: %w", err)
		}
		out = append(out, items...)
		if int64(len(out)) >= total || len(items) == 0 {
			return out, nil
		}
	}
}

func (r *DatasetRefSyncRunner) listCurrentEmbeddings(ctx context.Context, asset *datalibModel.DocumentAsset) (map[uuid.UUID]*datalibModel.DocumentChunkEmbedding, error) {
	if asset.EmbeddingProvider == nil || *asset.EmbeddingProvider == "" || asset.EmbeddingModel == nil || *asset.EmbeddingModel == "" {
		return nil, fmt.Errorf("asset embedding model is not configured")
	}
	generationNo := asset.GenerationNo
	out := map[uuid.UUID]*datalibModel.DocumentChunkEmbedding{}
	for offset := 0; ; offset += 500 {
		items, total, err := r.embeddings.List(ctx, datalibRepo.DocumentChunkEmbeddingListFilter{
			OrganizationID:    asset.OrganizationID,
			AssetID:           asset.ID,
			GenerationNo:      &generationNo,
			EmbeddingProvider: *asset.EmbeddingProvider,
			EmbeddingModel:    *asset.EmbeddingModel,
			Status:            datalibModel.DocumentChunkEmbeddingStatusReady,
			Limit:             500,
			Offset:            offset,
		})
		if err != nil {
			return nil, fmt.Errorf("list asset embeddings: %w", err)
		}
		for _, item := range items {
			out[item.ChunkID] = item
		}
		if int64(len(out)) >= total || len(items) == 0 {
			return out, nil
		}
	}
}

func groupChildChunksByParent(chunks []*datalibModel.DocumentChunk) map[uuid.UUID][]*datalibModel.DocumentChunk {
	out := map[uuid.UUID][]*datalibModel.DocumentChunk{}
	for _, chunk := range chunks {
		if chunk == nil || chunk.ParentChunkID == nil || chunk.ChunkType != datalibModel.DocumentChunkTypeChild {
			continue
		}
		out[*chunk.ParentChunkID] = append(out[*chunk.ParentChunkID], chunk)
	}
	return out
}

func hasChildChunks(chunks []*datalibModel.DocumentChunk) bool {
	for _, chunk := range chunks {
		if chunk != nil && chunk.ChunkType == datalibModel.DocumentChunkTypeChild {
			return true
		}
	}
	return false
}

func isDatasetPrimaryChunk(chunk *datalibModel.DocumentChunk) bool {
	if chunk == nil || !chunk.Enabled || chunk.Status != datalibModel.DocumentChunkStatusReady {
		return false
	}
	switch chunk.ChunkType {
	case datalibModel.DocumentChunkTypeParent, datalibModel.DocumentChunkTypeAuto, datalibModel.DocumentChunkTypeManual:
		return true
	default:
		return false
	}
}

func refDatasetDocumentID(ref *datalibModel.KnowledgeBaseAssetRef) string {
	if ref == nil || ref.DatasetDocumentID == nil || *ref.DatasetDocumentID == uuid.Nil {
		return ""
	}
	return ref.DatasetDocumentID.String()
}

func contentHash(content string) string {
	h := sha256.New()
	h.Write([]byte(content))
	return hex.EncodeToString(h.Sum(nil))
}

func defaultDatasetRefSyncVectorClassProperties() []map[string]interface{} {
	return []map[string]interface{}{
		{"name": "text", "dataType": []string{"text"}},
	}
}

func stringPtr(value string) *string {
	return &value
}

func boolPtr(value bool) *bool {
	return &value
}

func timePtr(value time.Time) *time.Time {
	return &value
}
