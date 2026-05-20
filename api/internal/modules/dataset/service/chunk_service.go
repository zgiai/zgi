package service

import (
	"context"
	"fmt"
	"time"

	"github.com/zgiai/ginext/internal/modules/dataset/model"
	"github.com/zgiai/ginext/internal/modules/dataset/repository"
	"gorm.io/gorm"
)

type ChunkService interface {
	CreateChunk(ctx context.Context, chunk *model.DocumentSegment) error
	GetChunkByID(ctx context.Context, id string) (*model.DocumentSegment, error)
	GetChunksByDocumentID(ctx context.Context, documentID string) ([]*model.DocumentSegment, error)
	UpdateChunk(ctx context.Context, chunk *model.DocumentSegment) error
	DeleteChunk(ctx context.Context, id string) error
	DeleteChunksByDocumentID(ctx context.Context, documentID string) error

	UpdateChunkIndexingStatus(ctx context.Context, chunkID, status string) error
	UpdateChunkVectorData(ctx context.Context, chunkID, indexNodeID string) error
	UpdateChunkError(ctx context.Context, chunkID, errorMsg string) error

	CreateChunksBatch(ctx context.Context, chunks []*model.DocumentSegment) error
	GetChunksByDatasetID(ctx context.Context, datasetID string, limit int) ([]*model.DocumentSegment, error)

	GetChunkCounts(ctx context.Context, documentID string) (completed int, total int, err error)
	GetChunkProcessingStats(ctx context.Context, documentID string) (*ChunkProcessingStats, error)

	CreateChildChunk(ctx context.Context, childChunk *model.ChildChunk) error
	UpdateChildChunk(ctx context.Context, childChunk *model.ChildChunk) error
	GetChildChunkByID(ctx context.Context, childChunkID string) (*model.ChildChunk, error)
	GetChildChunksBySegmentID(ctx context.Context, segmentID string) ([]*model.ChildChunk, error)
	DeleteChildChunkByID(ctx context.Context, childChunkID string) error
	DeleteChildChunksBySegmentID(ctx context.Context, segmentID string) error
	GetMaxChildChunkPosition(ctx context.Context, datasetID, documentID, segmentID string) (int, error)

	GetChunkWithChildChunks(ctx context.Context, chunkID string) (*ChunkWithChildren, error)
	GetChunkWithDocument(ctx context.Context, chunkID string) (*ChunkWithDocument, error)
}

type ChunkProcessingStats struct {
	Total      int64 `json:"total"`
	Processing int64 `json:"processing"`
	Completed  int64 `json:"completed"`
	Failed     int64 `json:"failed"`
}

type ChunkWithChildren struct {
	Chunk       *model.DocumentSegment `json:"chunk"`
	ChildChunks []*model.ChildChunk    `json:"child_chunks"`
}

type ChunkWithDocument struct {
	Chunk    *model.DocumentSegment `json:"chunk"`
	Document *model.Document        `json:"document"`
}

type chunkService struct {
	chunkRepo    repository.ChunkRepository
	documentRepo repository.DocumentRepository
	db           *gorm.DB
}

func NewChunkService(
	chunkRepo repository.ChunkRepository,
	documentRepo repository.DocumentRepository,
	db *gorm.DB,
) ChunkService {
	return &chunkService{
		chunkRepo:    chunkRepo,
		documentRepo: documentRepo,
		db:           db,
	}
}

func (s *chunkService) CreateChunk(ctx context.Context, chunk *model.DocumentSegment) error {
	return s.chunkRepo.Create(ctx, chunk)
}

func (s *chunkService) GetChunkByID(ctx context.Context, id string) (*model.DocumentSegment, error) {
	return s.chunkRepo.GetByID(ctx, id)
}

func (s *chunkService) GetChunksByDocumentID(ctx context.Context, documentID string) ([]*model.DocumentSegment, error) {
	return s.chunkRepo.GetByDocumentID(ctx, documentID)
}

func (s *chunkService) UpdateChunk(ctx context.Context, chunk *model.DocumentSegment) error {
	return s.chunkRepo.Update(ctx, chunk)
}

func (s *chunkService) DeleteChunk(ctx context.Context, id string) error {
	return s.db.Transaction(func(tx *gorm.DB) error {
		if err := s.chunkRepo.WithTx(tx).DeleteChildChunksBySegmentID(ctx, id); err != nil {
			return fmt.Errorf("failed to delete child chunks: %w", err)
		}

		if err := s.chunkRepo.WithTx(tx).Delete(ctx, id); err != nil {
			return fmt.Errorf("failed to delete chunk: %w", err)
		}

		return nil
	})
}

func (s *chunkService) DeleteChunksByDocumentID(ctx context.Context, documentID string) error {
	chunks, err := s.chunkRepo.GetByDocumentID(ctx, documentID)
	if err != nil {
		return fmt.Errorf("failed to get chunks: %w", err)
	}

	return s.db.Transaction(func(tx *gorm.DB) error {
		for _, chunk := range chunks {
			if err := s.chunkRepo.WithTx(tx).DeleteChildChunksBySegmentID(ctx, chunk.ID); err != nil {
				return fmt.Errorf("failed to delete child chunks for segment %s: %w", chunk.ID, err)
			}
		}

		if err := s.chunkRepo.WithTx(tx).DeleteByDocumentID(ctx, documentID); err != nil {
			return fmt.Errorf("failed to delete chunks: %w", err)
		}

		return nil
	})
}

func (s *chunkService) UpdateChunkIndexingStatus(ctx context.Context, chunkID, status string) error {
	now := time.Now()
	return s.chunkRepo.UpdateIndexingStatus(ctx, chunkID, status, &now)
}

func (s *chunkService) UpdateChunkVectorData(ctx context.Context, chunkID, indexNodeID string) error {
	now := time.Now()
	return s.chunkRepo.UpdateVectorData(ctx, chunkID, indexNodeID, &now)
}

func (s *chunkService) UpdateChunkError(ctx context.Context, chunkID, errorMsg string) error {
	return s.chunkRepo.UpdateError(ctx, chunkID, errorMsg)
}

func (s *chunkService) CreateChunksBatch(ctx context.Context, chunks []*model.DocumentSegment) error {
	return s.db.Transaction(func(tx *gorm.DB) error {
		for _, chunk := range chunks {
			if err := s.chunkRepo.CreateWithTx(ctx, tx, chunk); err != nil {
				return fmt.Errorf("failed to create chunk: %w", err)
			}
		}
		return nil
	})
}

func (s *chunkService) GetChunksByDatasetID(ctx context.Context, datasetID string, limit int) ([]*model.DocumentSegment, error) {
	return s.chunkRepo.GetByDatasetID(ctx, datasetID, limit)
}

func (s *chunkService) GetChunkCounts(ctx context.Context, documentID string) (completed int, total int, err error) {
	return s.chunkRepo.GetSegmentCounts(ctx, documentID)
}

func (s *chunkService) GetChunkProcessingStats(ctx context.Context, documentID string) (*ChunkProcessingStats, error) {
	total, err := s.chunkRepo.CountByDocumentID(ctx, documentID)
	if err != nil {
		return nil, fmt.Errorf("failed to count total chunks: %w", err)
	}

	processing, err := s.chunkRepo.CountByStatus(ctx, documentID, "processing")
	if err != nil {
		processing = 0
	}

	completed, err := s.chunkRepo.CountByStatus(ctx, documentID, "completed")
	if err != nil {
		completed = 0
	}

	failed, err := s.chunkRepo.CountByStatus(ctx, documentID, "failed")
	if err != nil {
		failed = 0
	}

	return &ChunkProcessingStats{
		Total:      total,
		Processing: processing,
		Completed:  completed,
		Failed:     failed,
	}, nil
}

func (s *chunkService) CreateChildChunk(ctx context.Context, childChunk *model.ChildChunk) error {
	return s.chunkRepo.CreateChildChunk(ctx, childChunk)
}

func (s *chunkService) UpdateChildChunk(ctx context.Context, childChunk *model.ChildChunk) error {
	return s.chunkRepo.UpdateChildChunk(ctx, childChunk)
}

func (s *chunkService) GetChildChunkByID(ctx context.Context, childChunkID string) (*model.ChildChunk, error) {
	return s.chunkRepo.GetChildChunkByID(ctx, childChunkID)
}

func (s *chunkService) GetChildChunksBySegmentID(ctx context.Context, segmentID string) ([]*model.ChildChunk, error) {
	return s.chunkRepo.GetChildChunksBySegmentID(ctx, segmentID)
}

func (s *chunkService) DeleteChildChunkByID(ctx context.Context, childChunkID string) error {
	return s.db.WithContext(ctx).Delete(&model.ChildChunk{}, "id = ?", childChunkID).Error
}

func (s *chunkService) DeleteChildChunksBySegmentID(ctx context.Context, segmentID string) error {
	return s.chunkRepo.DeleteChildChunksBySegmentID(ctx, segmentID)
}

// GetMaxChildChunkPosition
func (s *chunkService) GetMaxChildChunkPosition(ctx context.Context, datasetID, documentID, segmentID string) (int, error) {
	return s.chunkRepo.GetMaxChildChunkPosition(ctx, datasetID, documentID, segmentID)
}

func (s *chunkService) GetChunkWithChildChunks(ctx context.Context, chunkID string) (*ChunkWithChildren, error) {
	chunk, err := s.chunkRepo.GetByID(ctx, chunkID)
	if err != nil {
		return nil, fmt.Errorf("failed to get chunk: %w", err)
	}

	childChunks, err := s.chunkRepo.GetChildChunksBySegmentID(ctx, chunkID)
	if err != nil {
		return nil, fmt.Errorf("failed to get child chunks: %w", err)
	}

	return &ChunkWithChildren{
		Chunk:       chunk,
		ChildChunks: childChunks,
	}, nil
}

func (s *chunkService) GetChunkWithDocument(ctx context.Context, chunkID string) (*ChunkWithDocument, error) {
	chunk, err := s.chunkRepo.GetByID(ctx, chunkID)
	if err != nil {
		return nil, fmt.Errorf("failed to get chunk: %w", err)
	}

	document, err := s.documentRepo.GetByID(ctx, chunk.DocumentID)
	if err != nil {
		return nil, fmt.Errorf("failed to get document: %w", err)
	}

	return &ChunkWithDocument{
		Chunk:    chunk,
		Document: document,
	}, nil
}
