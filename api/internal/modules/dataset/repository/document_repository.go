package repository

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/zgiai/ginext/internal/modules/dataset/model"
	"gorm.io/gorm"
)

// DocumentRepository defines the interface for document repository
type DocumentRepository interface {
	GetByID(ctx context.Context, id string) (*model.Document, error)
	GetDocumentByID(ctx context.Context, id string) (*model.Document, error)
	GetByDatasetIDAndDocumentID(ctx context.Context, datasetID, documentID string) (*model.Document, error)
	GetByDatasetID(ctx context.Context, datasetID string, page, limit int, keyword, sort string, fetch bool, indexingStatus string) ([]*model.Document, int64, error)
	GetByDatasetIDSimple(ctx context.Context, datasetID string, offset, limit int) ([]*model.Document, error)
	GetErrorDocumentsByDatasetID(ctx context.Context, datasetID string) ([]*model.Document, error)
	GetByBatch(ctx context.Context, datasetID, batch string) ([]*model.Document, error)
	Create(ctx context.Context, document *model.Document) error
	Update(ctx context.Context, document *model.Document) error
	Delete(ctx context.Context, id string) error
	DeleteByIDs(ctx context.Context, ids []string) error

	// Document count methods
	GetDocumentCount(ctx context.Context, datasetID string) (int64, error)
	GetAvailableDocumentCount(ctx context.Context, datasetID string) (int64, error)
	CountByDatasetID(ctx context.Context, datasetID string) (int64, error)
	GetSegmentCount(ctx context.Context, datasetID string) (int64, error)
	GetNextPosition(ctx context.Context, datasetID string) (int, error)
	CheckArchived(ctx context.Context, document *model.Document) bool

	// Document indexing status methods
	UpdateDocumentIndexingStatus(ctx context.Context, documentID, status string) error
	UpdateDocumentProcessingStarted(ctx context.Context, documentID string, startTime *time.Time) error
	UpdateDocumentParsingCompleted(ctx context.Context, documentID string, completedTime *time.Time) error
	UpdateDocumentCleaningCompleted(ctx context.Context, documentID string, completedTime *time.Time) error
	UpdateDocumentSplittingCompleted(ctx context.Context, documentID string, completedTime *time.Time) error
	UpdateDocumentCompleted(ctx context.Context, documentID string, completedTime *time.Time) error
	UpdateDocumentError(ctx context.Context, documentID, errorMsg string, stoppedTime *time.Time) error
	UpdateDocumentWordCount(ctx context.Context, documentID string, wordCount int) error
	UpdateDocumentExtractionMetadata(ctx context.Context, documentID string, extraction map[string]interface{}) error
	UpdateDocumentMetadataField(ctx context.Context, documentID, key string, value interface{}) error
	UpdateDocumentTokens(ctx context.Context, documentID string, tokens int) error

	EnableDocuments(ctx context.Context, datasetID string, documentIDs []string) error
	DisableDocuments(ctx context.Context, datasetID string, documentIDs []string, accountID string) error
	ArchiveDocuments(ctx context.Context, datasetID string, documentIDs []string, accountID string) error
	UnArchiveDocuments(ctx context.Context, datasetID string, documentIDs []string, accountID string) error

	// Process rule related
	GetLatestProcessRule(ctx context.Context, datasetID string) (*model.DatasetProcessRule, error)
	CreateProcessRule(ctx context.Context, rule *model.DatasetProcessRule) error
	GetProcessRules(ctx context.Context, datasetID string) (*model.DatasetProcessRule, error)
	GetProcessRuleByID(ctx context.Context, id string) (*model.DatasetProcessRule, error)
	GetLatestProcessRulesByDatasetIDs(ctx context.Context, datasetIDs []string) (map[string]*model.DatasetProcessRule, error)
	ApplyDocumentRouting(ctx context.Context, documentID, docForm string, rule *model.DatasetProcessRule, routingMeta map[string]interface{}) error

	// Segment related
	GetSegmentCounts(ctx context.Context, documentID string) (completed int, total int, err error)
	GetSegmentsByDocumentID(ctx context.Context, documentID string) ([]*model.DocumentSegment, error)
	GetSegmentsByDatasetID(ctx context.Context, datasetID string, limit int) ([]*model.DocumentSegment, error)
	CreateDocumentSegment(ctx context.Context, segment *model.DocumentSegment) error
	GetDocumentSegmentByID(ctx context.Context, segmentID string) (*model.DocumentSegment, error)
	UpdateSegmentIndexingStatus(ctx context.Context, segmentID, status string, indexingTime *time.Time) error
	UpdateSegmentIndexingStatusByIndexNodeID(ctx context.Context, indexNodeID, status string, indexingTime *time.Time) error
	UpdateSegmentVectorData(ctx context.Context, segmentID, indexNodeID string, completedTime *time.Time) error
	UpdateSegmentVectorDataCompletedByIndexNodeID(ctx context.Context, indexNodeID string, completedTime *time.Time) error
	UpdateSegmentError(ctx context.Context, segmentID, errorMsg string) error
	UpdateSegmentErrorByIndexNodeID(ctx context.Context, indexNodeID, errorMsg string) error
	UpdateSegmentGraphIndexingStatus(ctx context.Context, segmentID, status string) error
	UpdateSegmentGraphIndexingStatusByDocumentID(ctx context.Context, documentID, status string, onlyCurrentStatus ...string) error
	DeleteDocumentSegmentByID(ctx context.Context, segmentID string) error
	DeleteDocumentSegmentsByDocumentID(ctx context.Context, documentID string) error
	DeleteDocumentSegmentsByDocumentIDs(ctx context.Context, documentIDs []string) error
	DeleteDocumentSegmentsByDatasetID(ctx context.Context, datasetID string) error
	IncrementSegmentHitCount(ctx context.Context, segmentIDs []string) error

	// Child chunk related
	GetChildChunksBySegmentID(ctx context.Context, segmentID string) ([]model.ChildChunk, error)
	CreateChildChunk(ctx context.Context, childChunk *model.ChildChunk) error
	GetChildChunkByIndexNodeID(ctx context.Context, indexNodeID string) (*model.ChildChunk, error)
	GetChildChunksByIndexNodeIDs(ctx context.Context, indexNodeIDs []string) ([]model.ChildChunk, error)
	DeleteChildChunkByID(ctx context.Context, chunkID string) error
	DeleteChildChunksBySegmentID(ctx context.Context, segmentID string) error
	DeleteChildChunksByIndexNodeIDs(ctx context.Context, indexNodeIDs []string) error
	DeleteChildChunksByDocumentID(ctx context.Context, documentID string) error
	DeleteChildChunksByDocumentIDs(ctx context.Context, documentIDs []string) error
	DeleteChildChunksByDatasetID(ctx context.Context, datasetID string) error

	// Document segment question related
	CreateDocumentSegmentQuestion(ctx context.Context, question *model.DocumentSegmentQuestion) error
	GetDocumentSegmentQuestionByID(ctx context.Context, id string) (*model.DocumentSegmentQuestion, error)
	ListDocumentSegmentQuestionsBySegmentID(ctx context.Context, segmentID string, page, limit int) ([]*model.DocumentSegmentQuestion, int64, error)
	ListDocumentSegmentQuestionsByDocumentID(ctx context.Context, documentID string, page, limit int) ([]*model.DocumentSegmentQuestion, int64, error)
	ListDocumentSegmentQuestionsByDatasetID(ctx context.Context, datasetID string, page, limit int) ([]*model.DocumentSegmentQuestion, int64, error)
	// RandomDocumentSegmentQuestionsByDatasetID randomly selects a specified number of questions from a dataset
	RandomDocumentSegmentQuestionsByDatasetID(ctx context.Context, datasetID string, limit int) ([]*model.DocumentSegmentQuestion, error)
	// GetDocumentSegmentQuestionCountByDatasetID returns the total count of questions for a dataset
	GetDocumentSegmentQuestionCountByDatasetID(ctx context.Context, datasetID string) (int64, error)
	UpdateDocumentSegmentQuestion(ctx context.Context, question *model.DocumentSegmentQuestion) error
	DeleteDocumentSegmentQuestion(ctx context.Context, id string) error
	DeleteDocumentSegmentQuestionsBySegmentID(ctx context.Context, segmentID string) error
	DeleteDocumentSegmentQuestionsBySegmentIDs(ctx context.Context, segmentIDs []string) error
	DeleteDocumentSegmentQuestionsByDocumentID(ctx context.Context, documentID string) error
	DeleteDocumentSegmentQuestionsByDocumentIDs(ctx context.Context, documentIDs []string) error
	DeleteDocumentSegmentQuestionsByDatasetID(ctx context.Context, datasetID string) error
	BatchCreateDocumentSegmentQuestions(ctx context.Context, questions []*model.DocumentSegmentQuestion) error

	// Question indexing status methods
	UpdateQuestionIndexingStatus(ctx context.Context, questionID, status string, indexingTime *time.Time) error
	UpdateQuestionIndexingCompleted(ctx context.Context, questionID string, completedTime *time.Time, error *string) error
	// GetAllDocumentSegmentQuestionsByDocumentID gets all questions for a document without pagination
	GetAllDocumentSegmentQuestionsByDocumentID(ctx context.Context, documentID string) ([]*model.DocumentSegmentQuestion, error)

	// Batch operations
	GetDocumentsByIDs(ctx context.Context, ids []string) ([]*model.Document, error)
	GetSegmentsByIDs(ctx context.Context, ids []string) ([]*model.DocumentSegment, error)
	GetDocumentSegmentByIndexNodeID(ctx context.Context, indexNodeID string) (*model.DocumentSegment, error)
	SoftDeleteSegmentsByDocumentID(ctx context.Context, documentID string) error
	SoftDeleteSegmentsByDocumentIDs(ctx context.Context, documentIDs []string) error

	// Utility methods
	GetDB() *gorm.DB
}

// DocumentRepositoryImpl implements the DocumentRepository interface
type DocumentRepositoryImpl struct {
	db *gorm.DB
}

// NewDocumentRepository creates a new DocumentRepository instance
func NewDocumentRepository(db *gorm.DB) DocumentRepository {
	return &DocumentRepositoryImpl{db: db}
}

// GetDB returns the underlying gorm database instance
func (r *DocumentRepositoryImpl) GetDB() *gorm.DB {
	return r.db
}

// GetByID retrieves a document by its ID
func (r *DocumentRepositoryImpl) GetByID(ctx context.Context, id string) (*model.Document, error) {
	var document model.Document
	if err := r.db.WithContext(ctx).Where("id = ?", id).First(&document).Error; err != nil {
		return nil, err
	}
	return &document, nil
}

// GetByDatasetIDAndDocumentID retrieves a document by dataset ID and document ID
func (r *DocumentRepositoryImpl) GetByDatasetIDAndDocumentID(ctx context.Context, datasetID, documentID string) (*model.Document, error) {
	var document model.Document
	if err := r.db.WithContext(ctx).Where("dataset_id = ? AND id = ?", datasetID, documentID).First(&document).Error; err != nil {
		return nil, err
	}
	return &document, nil
}

// GetByDatasetID retrieves documents by dataset ID with pagination and filters
func (r *DocumentRepositoryImpl) GetByDatasetID(ctx context.Context, datasetID string, page, limit int, keyword, sort string, fetch bool, indexingStatus string) ([]*model.Document, int64, error) {
	var documents []*model.Document
	var total int64

	query := r.db.WithContext(ctx).Model(&model.Document{}).Where("dataset_id = ?", datasetID)

	// Apply keyword search
	if keyword != "" {
		query = query.Where("name ILIKE ?", "%"+keyword+"%")
	}

	// Apply indexing status filter
	if indexingStatus != "" {
		query = query.Where("indexing_status = ?", indexingStatus)
	}

	// Count total records
	if err := query.Count(&total).Error; err != nil {
		return nil, 0, err
	}

	// Apply sorting
	sortDesc := false
	if strings.HasPrefix(sort, "-") {
		sortDesc = true
		sort = sort[1:]
	}

	switch sort {
	case "hit_count":
		// Join with document_segments for hit count
		subQuery := r.db.Model(&model.DocumentSegment{}).
			Select("document_id, COALESCE(SUM(hit_count), 0) as total_hit_count").
			Group("document_id")

		query = query.Joins("LEFT JOIN (?) as seg_hits ON documents.id = seg_hits.document_id", subQuery)
		if sortDesc {
			query = query.Order("COALESCE(seg_hits.total_hit_count, 0) DESC, documents.position DESC")
		} else {
			query = query.Order("COALESCE(seg_hits.total_hit_count, 0) ASC, documents.position ASC")
		}
	case "created_at":
		if sortDesc {
			query = query.Order("created_at DESC, position DESC")
		} else {
			query = query.Order("created_at ASC, position ASC")
		}
	default:
		query = query.Order("created_at DESC, position DESC")
	}

	// Apply pagination
	offset := (page - 1) * limit
	query = query.Offset(offset).Limit(limit)

	if err := query.Find(&documents).Error; err != nil {
		return nil, 0, err
	}

	// If fetch is true, populate segment counts
	if fetch {
		for _, doc := range documents {
			completed, total, err := r.GetSegmentCounts(ctx, doc.ID)
			if err != nil {
				continue // Skip on error, don't fail the entire request
			}
			doc.CompletedSegments = &completed
			doc.TotalSegments = &total
			doc.SegmentCount = total // Set segment_count field
		}
	}

	// Populate HitCount for each document
	for _, doc := range documents {
		var hitCount int64
		err := r.db.WithContext(ctx).
			Model(&model.DocumentSegment{}).
			Where("document_id = ?", doc.ID).
			Select("COALESCE(SUM(hit_count), 0)").
			Scan(&hitCount).Error

		if err == nil {
			doc.HitCount = int(hitCount)
		}
	}

	return documents, total, nil
}

// GetByDatasetIDSimple retrieves documents by dataset ID with simple pagination
func (r *DocumentRepositoryImpl) GetByDatasetIDSimple(ctx context.Context, datasetID string, offset, limit int) ([]*model.Document, error) {
	var documents []*model.Document
	query := r.db.WithContext(ctx).Where("dataset_id = ?", datasetID)

	if offset > 0 {
		query = query.Offset(offset)
	}
	if limit > 0 {
		query = query.Limit(limit)
	}

	if err := query.Find(&documents).Error; err != nil {
		return nil, err
	}
	return documents, nil
}

func (r *DocumentRepositoryImpl) GetErrorDocumentsByDatasetID(ctx context.Context, datasetID string) ([]*model.Document, error) {
	var documents []*model.Document
	// Query documents with indexing status "error" or "paused"
	if err := r.db.WithContext(ctx).Where("dataset_id = ? AND indexing_status IN ?", datasetID, []string{"failed", "error", "paused"}).Find(&documents).Error; err != nil {
		return nil, err
	}
	return documents, nil
}

// GetByBatch retrieves documents by dataset ID and batch
func (r *DocumentRepositoryImpl) GetByBatch(ctx context.Context, datasetID, batch string) ([]*model.Document, error) {
	var documents []*model.Document
	if err := r.db.WithContext(ctx).Where("dataset_id = ? AND batch = ?", datasetID, batch).Find(&documents).Error; err != nil {
		return nil, err
	}
	return documents, nil
}

// Create creates a new document
func (r *DocumentRepositoryImpl) Create(ctx context.Context, document *model.Document) error {
	return r.db.WithContext(ctx).Create(document).Error
}

// Update updates an existing document
func (r *DocumentRepositoryImpl) Update(ctx context.Context, document *model.Document) error {
	return r.db.WithContext(ctx).Save(document).Error
}

// Delete deletes a document by ID
func (r *DocumentRepositoryImpl) Delete(ctx context.Context, id string) error {
	return r.db.WithContext(ctx).Where("id = ?", id).Delete(&model.Document{}).Error
}

// DeleteByIDs deletes multiple documents by their IDs
func (r *DocumentRepositoryImpl) DeleteByIDs(ctx context.Context, ids []string) error {
	if len(ids) == 0 {
		return nil
	}
	return r.db.WithContext(ctx).Where("id IN ?", ids).Delete(&model.Document{}).Error
}

// GetDocumentCount returns the total number of documents in a dataset
func (r *DocumentRepositoryImpl) GetDocumentCount(ctx context.Context, datasetID string) (int64, error) {
	var count int64
	// Count all documents in the dataset, regardless of status
	if err := r.db.WithContext(ctx).Model(&model.Document{}).
		Where("dataset_id = ?", datasetID).
		Count(&count).Error; err != nil {
		return 0, err
	}
	return count, nil
}

// GetAvailableDocumentCount returns the count of available documents in a dataset
func (r *DocumentRepositoryImpl) GetAvailableDocumentCount(ctx context.Context, datasetID string) (int64, error) {
	var count int64
	if err := r.db.WithContext(ctx).Model(&model.Document{}).
		Where("dataset_id = ? AND enabled = ? AND indexing_status = ? AND archived = ?", datasetID, true, model.DocumentStatusCompleted, false).
		Count(&count).Error; err != nil {
		return 0, err
	}
	return count, nil
}

// CountByDatasetID returns the total number of documents in a dataset (alias for GetDocumentCount)
func (r *DocumentRepositoryImpl) CountByDatasetID(ctx context.Context, datasetID string) (int64, error) {
	return r.GetDocumentCount(ctx, datasetID)
}

// GetNextPosition returns the next position for a document in a dataset
func (r *DocumentRepositoryImpl) GetNextPosition(ctx context.Context, datasetID string) (int, error) {
	var maxPosition int
	result := r.db.WithContext(ctx).Model(&model.Document{}).
		Where("dataset_id = ?", datasetID).
		Select("COALESCE(MAX(position), 0)").
		Scan(&maxPosition)

	if result.Error != nil {
		return 0, result.Error
	}

	return maxPosition + 1, nil
}

// CheckArchived checks if a document is archived
func (r *DocumentRepositoryImpl) CheckArchived(ctx context.Context, document *model.Document) bool {
	return document.Archived
}

// GetLatestProcessRule retrieves the latest process rule for a dataset
func (r *DocumentRepositoryImpl) GetLatestProcessRule(ctx context.Context, datasetID string) (*model.DatasetProcessRule, error) {
	var rule model.DatasetProcessRule
	if err := r.db.WithContext(ctx).
		Where("dataset_id = ?", datasetID).
		Order("created_at DESC").
		First(&rule).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			return nil, nil
		}
		return nil, err
	}
	return &rule, nil
}

// CreateProcessRule creates a new process rule
func (r *DocumentRepositoryImpl) CreateProcessRule(ctx context.Context, rule *model.DatasetProcessRule) error {
	return r.db.WithContext(ctx).Create(rule).Error
}

// ApplyDocumentRouting updates a document-level process rule snapshot and binds routing metadata atomically.
func (r *DocumentRepositoryImpl) ApplyDocumentRouting(ctx context.Context, documentID, docForm string, rule *model.DatasetProcessRule, routingMeta map[string]interface{}) error {
	if strings.TrimSpace(documentID) == "" {
		return fmt.Errorf("document id is required")
	}
	if strings.TrimSpace(docForm) == "" {
		return fmt.Errorf("doc form is required")
	}
	if rule == nil {
		return fmt.Errorf("process rule is required")
	}
	if strings.TrimSpace(rule.ID) == "" {
		return fmt.Errorf("process rule id is required")
	}

	return r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var document model.Document
		if err := tx.Where("id = ?", documentID).First(&document).Error; err != nil {
			return fmt.Errorf("failed to load document %s: %w", documentID, err)
		}

		if document.DatasetProcessRuleID == nil || *document.DatasetProcessRuleID != rule.ID {
			return fmt.Errorf("document %s is not bound to process rule %s", documentID, rule.ID)
		}

		if err := tx.Model(&model.DatasetProcessRule{}).
			Where("id = ?", rule.ID).
			Updates(map[string]interface{}{
				"mode":  rule.Mode,
				"rules": rule.Rules,
			}).Error; err != nil {
			return fmt.Errorf("failed to update process rule snapshot: %w", err)
		}

		docMetadata := document.DocMetadata
		if docMetadata == nil {
			docMetadata = model.JSONMap{}
		}
		if routingMeta == nil {
			routingMeta = map[string]interface{}{}
		}
		docMetadata["routing"] = routingMeta

		updates := map[string]interface{}{
			"doc_form":     docForm,
			"doc_metadata": docMetadata,
		}

		if err := tx.Model(&model.Document{}).
			Where("id = ?", documentID).
			Updates(updates).Error; err != nil {
			return fmt.Errorf("failed to update document routing: %w", err)
		}

		return nil
	})
}

// GetProcessRules retrieves the latest process rule for a dataset (alias for GetLatestProcessRule)
func (r *DocumentRepositoryImpl) GetProcessRules(ctx context.Context, datasetID string) (*model.DatasetProcessRule, error) {
	return r.GetLatestProcessRule(ctx, datasetID)
}

// GetProcessRuleByID retrieves a process rule by its ID
func (r *DocumentRepositoryImpl) GetProcessRuleByID(ctx context.Context, id string) (*model.DatasetProcessRule, error) {
	var rule model.DatasetProcessRule
	if err := r.db.WithContext(ctx).
		Where("id = ?", id).
		First(&rule).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			return nil, nil
		}
		return nil, err
	}
	return &rule, nil
}

// GetLatestProcessRulesByDatasetIDs retrieves the latest process rules for a list of dataset IDs
func (r *DocumentRepositoryImpl) GetLatestProcessRulesByDatasetIDs(ctx context.Context, datasetIDs []string) (map[string]*model.DatasetProcessRule, error) {
	if len(datasetIDs) == 0 {
		return map[string]*model.DatasetProcessRule{}, nil
	}

	var rules []*model.DatasetProcessRule

	// Use DISTINCT ON to get the latest rule for each dataset (PostgreSQL specific)
	err := r.db.WithContext(ctx).
		Raw("SELECT DISTINCT ON (dataset_id) * FROM dataset_process_rules WHERE dataset_id IN ? ORDER BY dataset_id, created_at DESC", datasetIDs).
		Scan(&rules).Error

	if err != nil {
		return nil, err
	}

	result := make(map[string]*model.DatasetProcessRule)
	for _, rule := range rules {
		result[rule.DatasetID] = rule
	}
	return result, nil
}

// GetSegmentCounts returns the completed and total segment counts for a document
func (r *DocumentRepositoryImpl) GetSegmentCounts(ctx context.Context, documentID string) (completed int, total int, err error) {
	var completedCount int64
	var totalCount int64

	// Count completed segments (exclude re_segment status)
	err = r.db.WithContext(ctx).Model(&model.DocumentSegment{}).
		Where("document_id = ? AND completed_at IS NOT NULL AND status != ?", documentID, model.SegmentStatusReSegment).
		Count(&completedCount).Error
	if err != nil {
		return 0, 0, fmt.Errorf("failed to count completed segments: %w", err)
	}

	// Count total segments (exclude re_segment status)
	// Count total segments (exclude re_segment status)
	if err := r.db.WithContext(ctx).Model(&model.DocumentSegment{}).
		Where("document_id = ? AND status != ?", documentID, model.SegmentStatusReSegment).
		Count(&totalCount).Error; err != nil {
		return 0, 0, fmt.Errorf("failed to count total segments: %w", err)
	}

	return int(completedCount), int(totalCount), nil
}

// GetSegmentsByDocumentID retrieves all segments for a document
func (r *DocumentRepositoryImpl) GetSegmentsByDocumentID(ctx context.Context, documentID string) ([]*model.DocumentSegment, error) {
	var segments []*model.DocumentSegment
	if err := r.db.WithContext(ctx).Where("document_id = ?", documentID).Find(&segments).Error; err != nil {
		return nil, err
	}
	return segments, nil
}

// GetDocumentByID is an alias for GetByID for compatibility
func (r *DocumentRepositoryImpl) GetDocumentByID(ctx context.Context, id string) (*model.Document, error) {
	return r.GetByID(ctx, id)
}

// UpdateDocumentIndexingStatus updates the indexing status of a document
func (r *DocumentRepositoryImpl) UpdateDocumentIndexingStatus(ctx context.Context, documentID, status string) error {
	return r.db.WithContext(ctx).Model(&model.Document{}).
		Where("id = ?", documentID).
		Update("indexing_status", status).Error
}

// UpdateDocumentProcessingStarted updates the processing started time
func (r *DocumentRepositoryImpl) UpdateDocumentProcessingStarted(ctx context.Context, documentID string, startTime *time.Time) error {
	return r.db.WithContext(ctx).Model(&model.Document{}).
		Where("id = ?", documentID).
		Update("processing_started_at", startTime).Error
}

// UpdateDocumentParsingCompleted updates the parsing completed time
func (r *DocumentRepositoryImpl) UpdateDocumentParsingCompleted(ctx context.Context, documentID string, completedTime *time.Time) error {
	return r.db.WithContext(ctx).Model(&model.Document{}).
		Where("id = ?", documentID).
		Update("parsing_completed_at", completedTime).Error
}

// UpdateDocumentCleaningCompleted updates the cleaning completed time
func (r *DocumentRepositoryImpl) UpdateDocumentCleaningCompleted(ctx context.Context, documentID string, completedTime *time.Time) error {
	return r.db.WithContext(ctx).Model(&model.Document{}).
		Where("id = ?", documentID).
		Update("cleaning_completed_at", completedTime).Error
}

// UpdateDocumentSplittingCompleted updates the splitting completed time
func (r *DocumentRepositoryImpl) UpdateDocumentSplittingCompleted(ctx context.Context, documentID string, completedTime *time.Time) error {
	return r.db.WithContext(ctx).Model(&model.Document{}).
		Where("id = ?", documentID).
		Update("splitting_completed_at", completedTime).Error
}

// UpdateDocumentCompleted updates the completion time and status
func (r *DocumentRepositoryImpl) UpdateDocumentCompleted(ctx context.Context, documentID string, completedTime *time.Time) error {
	return r.db.WithContext(ctx).Model(&model.Document{}).
		Where("id = ?", documentID).
		Updates(map[string]interface{}{
			"completed_at":    completedTime,
			"indexing_status": "completed",
		}).Error
}

// UpdateDocumentError updates the error message and stopped time
func (r *DocumentRepositoryImpl) UpdateDocumentError(ctx context.Context, documentID, errorMsg string, stoppedTime *time.Time) error {
	return r.db.WithContext(ctx).Model(&model.Document{}).
		Where("id = ?", documentID).
		Updates(map[string]interface{}{
			"error":           errorMsg,
			"stopped_at":      stoppedTime,
			"indexing_status": model.DocumentStatusError,
		}).Error
}

// UpdateDocumentWordCount updates the word count of a document
func (r *DocumentRepositoryImpl) UpdateDocumentWordCount(ctx context.Context, documentID string, wordCount int) error {
	return r.db.WithContext(ctx).Model(&model.Document{}).
		Where("id = ?", documentID).
		Update("word_count", wordCount).Error
}

func (r *DocumentRepositoryImpl) UpdateDocumentExtractionMetadata(ctx context.Context, documentID string, extraction map[string]interface{}) error {
	if len(extraction) == 0 {
		return nil
	}

	return r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var document model.Document
		if err := tx.Where("id = ?", documentID).First(&document).Error; err != nil {
			return fmt.Errorf("failed to load document %s: %w", documentID, err)
		}

		docMetadata := document.DocMetadata
		if docMetadata == nil {
			docMetadata = model.JSONMap{}
		}
		docMetadata["extraction"] = extraction

		if err := tx.Model(&model.Document{}).
			Where("id = ?", documentID).
			Update("doc_metadata", docMetadata).Error; err != nil {
			return fmt.Errorf("failed to update document extraction metadata: %w", err)
		}

		return nil
	})
}

func (r *DocumentRepositoryImpl) UpdateDocumentMetadataField(ctx context.Context, documentID, key string, value interface{}) error {
	key = strings.TrimSpace(key)
	if key == "" {
		return nil
	}

	payload, err := json.Marshal(value)
	if err != nil {
		return fmt.Errorf("failed to marshal document metadata field %q: %w", key, err)
	}

	if err := r.db.WithContext(ctx).
		Model(&model.Document{}).
		Where("id = ?", documentID).
		Update("doc_metadata", gorm.Expr("jsonb_set(COALESCE(doc_metadata, '{}'::jsonb), ARRAY[?]::text[], ?::jsonb, true)", key, string(payload))).
		Error; err != nil {
		return fmt.Errorf("failed to update document metadata field %q: %w", key, err)
	}
	return nil
}

// UpdateDocumentTokens updates the tokens count of a document
func (r *DocumentRepositoryImpl) UpdateDocumentTokens(ctx context.Context, documentID string, tokens int) error {
	return r.db.WithContext(ctx).Model(&model.Document{}).
		Where("id = ?", documentID).
		Update("tokens", tokens).Error
}

// CreateDocumentSegment creates a new document segment
func (r *DocumentRepositoryImpl) CreateDocumentSegment(ctx context.Context, segment *model.DocumentSegment) error {
	return r.db.WithContext(ctx).Create(segment).Error
}

// GetDocumentSegmentByID retrieves a document segment by ID
func (r *DocumentRepositoryImpl) GetDocumentSegmentByID(ctx context.Context, segmentID string) (*model.DocumentSegment, error) {
	var segment model.DocumentSegment
	if err := r.db.WithContext(ctx).Where("id = ?", segmentID).First(&segment).Error; err != nil {
		return nil, err
	}
	return &segment, nil
}

// GetSegmentCount retrieves the count of segments for a dataset
func (r *DocumentRepositoryImpl) GetSegmentCount(ctx context.Context, datasetID string) (int64, error) {
	var count int64
	err := r.db.WithContext(ctx).Model(&model.DocumentSegment{}).
		Where("dataset_id = ? AND enabled = ? AND status = ?", datasetID, true, "completed").
		Count(&count).Error
	if err != nil {
		return 0, fmt.Errorf("failed to get segment count: %w", err)
	}
	return count, nil
}

// GetSegmentsByDatasetID retrieves all enabled segments for a dataset
func (r *DocumentRepositoryImpl) GetSegmentsByDatasetID(ctx context.Context, datasetID string, limit int) ([]*model.DocumentSegment, error) {
	var segments []*model.DocumentSegment
	query := r.db.WithContext(ctx).
		Preload("Document").
		Where("dataset_id = ? AND enabled = ? AND status = ?", datasetID, true, "completed").
		Order("created_at DESC")

	if limit > 0 {
		query = query.Limit(limit)
	}

	if err := query.Find(&segments).Error; err != nil {
		return nil, fmt.Errorf("failed to get segments by dataset ID: %w", err)
	}

	return segments, nil
}

// UpdateSegmentIndexingStatus updates the indexing status of a segment
func (r *DocumentRepositoryImpl) UpdateSegmentIndexingStatus(ctx context.Context, segmentID, status string, indexingTime *time.Time) error {
	updates := map[string]interface{}{
		"status": status,
	}
	if indexingTime != nil {
		updates["indexing_at"] = indexingTime
	}
	return r.db.WithContext(ctx).Model(&model.DocumentSegment{}).
		Where("id = ?", segmentID).
		Updates(updates).Error
}

// UpdateSegmentIndexingStatusByIndexNodeID updates the indexing status of a segment by index node ID.
func (r *DocumentRepositoryImpl) UpdateSegmentIndexingStatusByIndexNodeID(ctx context.Context, indexNodeID, status string, indexingTime *time.Time) error {
	updates := map[string]interface{}{
		"status": status,
	}
	if indexingTime != nil {
		updates["indexing_at"] = indexingTime
	}
	return r.db.WithContext(ctx).Model(&model.DocumentSegment{}).
		Where("index_node_id = ?", indexNodeID).
		Updates(updates).Error
}

// UpdateSegmentVectorData updates the vector data for a segment
func (r *DocumentRepositoryImpl) UpdateSegmentVectorData(ctx context.Context, segmentID, indexNodeID string, completedTime *time.Time) error {
	return r.db.WithContext(ctx).Model(&model.DocumentSegment{}).
		Where("id = ?", segmentID).
		Updates(map[string]interface{}{
			"completed_at": completedTime,
			"status":       "completed",
		}).Error
}

// UpdateSegmentVectorDataCompletedByIndexNodeID updates the vector data for a segment by index node ID.
func (r *DocumentRepositoryImpl) UpdateSegmentVectorDataCompletedByIndexNodeID(ctx context.Context, indexNodeID string, completedTime *time.Time) error {
	return r.db.WithContext(ctx).Model(&model.DocumentSegment{}).
		Where("index_node_id = ?", indexNodeID).
		Updates(map[string]interface{}{
			"completed_at": completedTime,
			"status":       "completed",
		}).Error
}

// UpdateSegmentError updates the error message for a segment
func (r *DocumentRepositoryImpl) UpdateSegmentError(ctx context.Context, segmentID, errorMsg string) error {
	return r.db.WithContext(ctx).Model(&model.DocumentSegment{}).
		Where("id = ?", segmentID).
		Updates(map[string]interface{}{
			"error":  errorMsg,
			"status": "error",
		}).Error
}

// UpdateSegmentErrorByIndexNodeID updates the error message for a segment by index node ID
func (r *DocumentRepositoryImpl) UpdateSegmentErrorByIndexNodeID(ctx context.Context, indexNodeID, errorMsg string) error {
	return r.db.WithContext(ctx).Model(&model.DocumentSegment{}).
		Where("index_node_id = ?", indexNodeID).
		Updates(map[string]interface{}{
			"error":  errorMsg,
			"status": "error",
		}).Error
}

// UpdateSegmentGraphIndexingStatus updates the graph indexing status of a segment
func (r *DocumentRepositoryImpl) UpdateSegmentGraphIndexingStatus(ctx context.Context, segmentID, status string) error {
	return r.db.WithContext(ctx).Model(&model.DocumentSegment{}).
		Where("id = ?", segmentID).
		Update("graph_indexing_status", status).Error
}

// UpdateSegmentGraphIndexingStatusByDocumentID updates the graph indexing status of all segments of a document
func (r *DocumentRepositoryImpl) UpdateSegmentGraphIndexingStatusByDocumentID(ctx context.Context, documentID, status string, onlyCurrentStatus ...string) error {
	query := r.db.WithContext(ctx).Model(&model.DocumentSegment{}).
		Where("document_id = ?", documentID)
	if len(onlyCurrentStatus) > 0 && onlyCurrentStatus[0] != "" {
		query = query.Where("graph_indexing_status = ?", onlyCurrentStatus[0])
	}
	return query.Update("graph_indexing_status", status).Error
}

// GetChildChunksBySegmentID retrieves child chunks for a given segment ID
func (r *DocumentRepositoryImpl) GetChildChunksBySegmentID(ctx context.Context, segmentID string) ([]model.ChildChunk, error) {
	var childChunks []model.ChildChunk
	err := r.db.WithContext(ctx).Where("segment_id = ?", segmentID).Order("position ASC").Find(&childChunks).Error
	return childChunks, err
}

// CreateChildChunk creates a new child chunk
func (r *DocumentRepositoryImpl) CreateChildChunk(ctx context.Context, childChunk *model.ChildChunk) error {
	return r.db.WithContext(ctx).Create(childChunk).Error
}

// GetChildChunkByIndexNodeID retrieves a child chunk by index node ID
func (r *DocumentRepositoryImpl) GetChildChunkByIndexNodeID(ctx context.Context, indexNodeID string) (*model.ChildChunk, error) {
	var childChunk model.ChildChunk
	if err := r.db.WithContext(ctx).Where("index_node_id = ?", indexNodeID).First(&childChunk).Error; err != nil {
		return nil, err
	}
	return &childChunk, nil
}

// GetChildChunksByIndexNodeIDs retrieves child chunks by a list of index node IDs
func (r *DocumentRepositoryImpl) GetChildChunksByIndexNodeIDs(ctx context.Context, indexNodeIDs []string) ([]model.ChildChunk, error) {
	var childChunks []model.ChildChunk
	if err := r.db.WithContext(ctx).Where("index_node_id IN ?", indexNodeIDs).Find(&childChunks).Error; err != nil {
		return nil, err
	}
	return childChunks, nil
}

// DocumentSegmentQuestion related methods

// CreateDocumentSegmentQuestion creates a new document segment question
func (r *DocumentRepositoryImpl) CreateDocumentSegmentQuestion(ctx context.Context, question *model.DocumentSegmentQuestion) error {
	return r.db.WithContext(ctx).Create(question).Error
}

// GetDocumentSegmentQuestionByID retrieves a document segment question by ID
func (r *DocumentRepositoryImpl) GetDocumentSegmentQuestionByID(ctx context.Context, id string) (*model.DocumentSegmentQuestion, error) {
	var question model.DocumentSegmentQuestion
	if err := r.db.WithContext(ctx).Where("id = ?", id).First(&question).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			return nil, nil
		}
		return nil, err
	}
	return &question, nil
}

// ListDocumentSegmentQuestionsBySegmentID retrieves document segment questions by segment ID with pagination
func (r *DocumentRepositoryImpl) ListDocumentSegmentQuestionsBySegmentID(ctx context.Context, segmentID string, page, limit int) ([]*model.DocumentSegmentQuestion, int64, error) {
	var questions []*model.DocumentSegmentQuestion
	var total int64

	offset := (page - 1) * limit
	query := r.db.WithContext(ctx).Model(&model.DocumentSegmentQuestion{}).Where("segment_id = ?", segmentID)

	if err := query.Count(&total).Error; err != nil {
		return nil, 0, err
	}

	if err := query.Offset(offset).Limit(limit).Order("created_at DESC").Find(&questions).Error; err != nil {
		return nil, 0, err
	}

	return questions, total, nil
}

// ListDocumentSegmentQuestionsByDocumentID retrieves document segment questions by document ID with pagination
func (r *DocumentRepositoryImpl) ListDocumentSegmentQuestionsByDocumentID(ctx context.Context, documentID string, page, limit int) ([]*model.DocumentSegmentQuestion, int64, error) {
	var questions []*model.DocumentSegmentQuestion
	var total int64

	offset := (page - 1) * limit
	query := r.db.WithContext(ctx).Model(&model.DocumentSegmentQuestion{}).Where("document_id = ?", documentID)

	if err := query.Count(&total).Error; err != nil {
		return nil, 0, err
	}

	if err := query.Offset(offset).Limit(limit).Order("created_at DESC").Find(&questions).Error; err != nil {
		return nil, 0, err
	}

	return questions, total, nil
}

// ListDocumentSegmentQuestionsByDatasetID retrieves document segment questions by dataset ID with pagination
func (r *DocumentRepositoryImpl) ListDocumentSegmentQuestionsByDatasetID(ctx context.Context, datasetID string, page, limit int) ([]*model.DocumentSegmentQuestion, int64, error) {
	var questions []*model.DocumentSegmentQuestion
	var total int64

	offset := (page - 1) * limit
	query := r.db.WithContext(ctx).Model(&model.DocumentSegmentQuestion{}).Where("dataset_id = ?", datasetID)

	if err := query.Count(&total).Error; err != nil {
		return nil, 0, err
	}

	if err := query.Offset(offset).Limit(limit).Order("created_at DESC").Find(&questions).Error; err != nil {
		return nil, 0, err
	}

	return questions, total, nil
}

// RandomDocumentSegmentQuestionsByDatasetID randomly selects a specified number of questions from a dataset
func (r *DocumentRepositoryImpl) RandomDocumentSegmentQuestionsByDatasetID(ctx context.Context, datasetID string, limit int) ([]*model.DocumentSegmentQuestion, error) {
	var questions []*model.DocumentSegmentQuestion

	// Using raw SQL to get random questions
	// PostgreSQL specific random function
	query := r.db.WithContext(ctx).
		Raw("SELECT * FROM document_segment_questions WHERE dataset_id = ? ORDER BY RANDOM() LIMIT ?", datasetID, limit).
		Scan(&questions)

	if query.Error != nil {
		return nil, query.Error
	}

	return questions, nil
}

// GetAllDocumentSegmentQuestionsByDocumentID gets all questions for a document without pagination
func (r *DocumentRepositoryImpl) GetAllDocumentSegmentQuestionsByDocumentID(ctx context.Context, documentID string) ([]*model.DocumentSegmentQuestion, error) {
	var questions []*model.DocumentSegmentQuestion

	query := r.db.WithContext(ctx).Model(&model.DocumentSegmentQuestion{}).Where("document_id = ?", documentID)
	if err := query.Order("created_at DESC").Find(&questions).Error; err != nil {
		return nil, err
	}

	return questions, nil
}

// GetDocumentSegmentQuestionCountByDatasetID returns the total count of questions for a dataset
func (r *DocumentRepositoryImpl) GetDocumentSegmentQuestionCountByDatasetID(ctx context.Context, datasetID string) (int64, error) {
	var count int64
	err := r.db.WithContext(ctx).
		Model(&model.DocumentSegmentQuestion{}).
		Where("dataset_id = ?", datasetID).
		Count(&count).Error

	return count, err
}

// UpdateDocumentSegmentQuestion updates a document segment question
func (r *DocumentRepositoryImpl) UpdateDocumentSegmentQuestion(ctx context.Context, question *model.DocumentSegmentQuestion) error {
	return r.db.WithContext(ctx).Save(question).Error
}

// DeleteDocumentSegmentQuestion deletes a document segment question by ID
func (r *DocumentRepositoryImpl) DeleteDocumentSegmentQuestion(ctx context.Context, id string) error {
	return r.db.WithContext(ctx).Where("id = ?", id).Delete(&model.DocumentSegmentQuestion{}).Error
}

// DeleteDocumentSegmentByID deletes a document segment by ID
func (r *DocumentRepositoryImpl) DeleteDocumentSegmentByID(ctx context.Context, segmentID string) error {
	return r.db.WithContext(ctx).Where("id = ?", segmentID).Delete(&model.DocumentSegment{}).Error
}

// DeleteDocumentSegmentsByDocumentID deletes all document segments by document ID
func (r *DocumentRepositoryImpl) DeleteDocumentSegmentsByDocumentID(ctx context.Context, documentID string) error {
	return r.db.WithContext(ctx).Where("document_id = ?", documentID).Delete(&model.DocumentSegment{}).Error
}

// SoftDeleteSegmentsByDocumentID performs a soft delete on document segments for a single document
func (r *DocumentRepositoryImpl) SoftDeleteSegmentsByDocumentID(ctx context.Context, documentID string) error {
	now := time.Now()
	updates := map[string]interface{}{
		"is_deleted":            true,
		"deleted_at":            &now,
		"graph_indexing_status": "deleted",
		"status":                "deleted",
	}
	return r.db.WithContext(ctx).Model(&model.DocumentSegment{}).
		Where("document_id = ?", documentID).
		Updates(updates).Error
}

// SoftDeleteSegmentsByDocumentIDs performs a soft delete on document segments for multiple documents
func (r *DocumentRepositoryImpl) SoftDeleteSegmentsByDocumentIDs(ctx context.Context, documentIDs []string) error {
	if len(documentIDs) == 0 {
		return nil
	}
	now := time.Now()
	updates := map[string]interface{}{
		"is_deleted":            true,
		"deleted_at":            &now,
		"graph_indexing_status": "deleted",
		"status":                "deleted",
	}
	return r.db.WithContext(ctx).Model(&model.DocumentSegment{}).
		Where("document_id IN ?", documentIDs).
		Updates(updates).Error
}

// GetSegmentsByIDs retrieves multiple document segments by their IDs
func (r *DocumentRepositoryImpl) GetSegmentsByIDs(ctx context.Context, ids []string) ([]*model.DocumentSegment, error) {
	if len(ids) == 0 {
		return nil, nil
	}
	var segments []*model.DocumentSegment
	if err := r.db.WithContext(ctx).Where("id IN ?", ids).Find(&segments).Error; err != nil {
		return nil, err
	}
	return segments, nil
}

// DeleteDocumentSegmentsByDocumentIDs deletes all document segments by document IDs
func (r *DocumentRepositoryImpl) DeleteDocumentSegmentsByDocumentIDs(ctx context.Context, documentIDs []string) error {
	if len(documentIDs) == 0 {
		return nil
	}
	return r.db.WithContext(ctx).Where("document_id IN ?", documentIDs).Delete(&model.DocumentSegment{}).Error
}

// DeleteDocumentSegmentsByDatasetID deletes all document segments by dataset ID
func (r *DocumentRepositoryImpl) DeleteDocumentSegmentsByDatasetID(ctx context.Context, datasetID string) error {
	return r.db.WithContext(ctx).Where("dataset_id = ?", datasetID).Delete(&model.DocumentSegment{}).Error
}

// DeleteDocumentSegmentQuestionsBySegmentID deletes all document segment questions by segment ID
func (r *DocumentRepositoryImpl) DeleteDocumentSegmentQuestionsBySegmentID(ctx context.Context, segmentID string) error {
	return r.db.WithContext(ctx).Where("segment_id = ?", segmentID).Delete(&model.DocumentSegmentQuestion{}).Error
}

// DeleteDocumentSegmentQuestionsBySegmentIDs deletes all document segment questions by segment IDs
func (r *DocumentRepositoryImpl) DeleteDocumentSegmentQuestionsBySegmentIDs(ctx context.Context, segmentIDs []string) error {
	if len(segmentIDs) == 0 {
		return nil
	}
	return r.db.WithContext(ctx).Where("segment_id IN ?", segmentIDs).Delete(&model.DocumentSegmentQuestion{}).Error
}

// DeleteDocumentSegmentQuestionsByDocumentID deletes all document segment questions by document ID
func (r *DocumentRepositoryImpl) DeleteDocumentSegmentQuestionsByDocumentID(ctx context.Context, documentID string) error {
	return r.db.WithContext(ctx).Where("document_id = ?", documentID).Delete(&model.DocumentSegmentQuestion{}).Error
}

// DeleteDocumentSegmentQuestionsByDocumentIDs deletes all document segment questions by document IDs
func (r *DocumentRepositoryImpl) DeleteDocumentSegmentQuestionsByDocumentIDs(ctx context.Context, documentIDs []string) error {
	if len(documentIDs) == 0 {
		return nil
	}
	return r.db.WithContext(ctx).Where("document_id IN ?", documentIDs).Delete(&model.DocumentSegmentQuestion{}).Error
}

// DeleteDocumentSegmentQuestionsByDatasetID deletes all document segment questions by dataset ID
func (r *DocumentRepositoryImpl) DeleteDocumentSegmentQuestionsByDatasetID(ctx context.Context, datasetID string) error {
	return r.db.WithContext(ctx).Where("dataset_id = ?", datasetID).Delete(&model.DocumentSegmentQuestion{}).Error
}

// DeleteChildChunkByID deletes a child chunk by ID
func (r *DocumentRepositoryImpl) DeleteChildChunkByID(ctx context.Context, chunkID string) error {
	return r.db.WithContext(ctx).Where("id = ?", chunkID).Delete(&model.ChildChunk{}).Error
}

// DeleteChildChunksBySegmentID deletes all child chunks by segment ID
func (r *DocumentRepositoryImpl) DeleteChildChunksBySegmentID(ctx context.Context, segmentID string) error {
	return r.db.WithContext(ctx).Where("segment_id = ?", segmentID).Delete(&model.ChildChunk{}).Error
}

// DeleteChildChunksByIndexNodeIDs deletes all child chunks by a list of index node IDs
func (r *DocumentRepositoryImpl) DeleteChildChunksByIndexNodeIDs(ctx context.Context, indexNodeIDs []string) error {
	if len(indexNodeIDs) == 0 {
		return nil
	}
	return r.db.WithContext(ctx).Where("index_node_id IN ?", indexNodeIDs).Delete(&model.ChildChunk{}).Error
}

// DeleteChildChunksByDocumentID deletes all child chunks by document ID
func (r *DocumentRepositoryImpl) DeleteChildChunksByDocumentID(ctx context.Context, documentID string) error {
	return r.db.WithContext(ctx).Where("document_id = ?", documentID).Delete(&model.ChildChunk{}).Error
}

// DeleteChildChunksByDocumentIDs deletes all child chunks by document IDs
func (r *DocumentRepositoryImpl) DeleteChildChunksByDocumentIDs(ctx context.Context, documentIDs []string) error {
	if len(documentIDs) == 0 {
		return nil
	}
	return r.db.WithContext(ctx).Where("document_id IN ?", documentIDs).Delete(&model.ChildChunk{}).Error
}

// DeleteChildChunksByDatasetID deletes all child chunks by dataset ID
func (r *DocumentRepositoryImpl) DeleteChildChunksByDatasetID(ctx context.Context, datasetID string) error {
	return r.db.WithContext(ctx).Where("dataset_id = ?", datasetID).Delete(&model.ChildChunk{}).Error
}

// GetDocumentsByIDs retrieves documents by their IDs
func (r *DocumentRepositoryImpl) GetDocumentsByIDs(ctx context.Context, ids []string) ([]*model.Document, error) {
	var documents []*model.Document
	if err := r.db.WithContext(ctx).Where("id IN ?", ids).Find(&documents).Error; err != nil {
		return nil, err
	}
	return documents, nil
}

// GetDocumentSegmentByIndexNodeID retrieves a document segment by index node ID
func (r *DocumentRepositoryImpl) GetDocumentSegmentByIndexNodeID(ctx context.Context, indexNodeID string) (*model.DocumentSegment, error) {
	var segment model.DocumentSegment
	if err := r.db.WithContext(ctx).Where("index_node_id = ?", indexNodeID).First(&segment).Error; err != nil {
		return nil, err
	}
	return &segment, nil
}

// BatchCreateDocumentSegmentQuestions creates multiple document segment questions
func (r *DocumentRepositoryImpl) BatchCreateDocumentSegmentQuestions(ctx context.Context, questions []*model.DocumentSegmentQuestion) error {
	return r.db.WithContext(ctx).CreateInBatches(questions, 100).Error
}

// UpdateQuestionIndexingStatus updates the indexing status of a question
func (r *DocumentRepositoryImpl) UpdateQuestionIndexingStatus(ctx context.Context, questionID, status string, indexingTime *time.Time) error {
	updates := map[string]interface{}{
		"status": status,
	}
	if indexingTime != nil {
		updates["indexing_at"] = indexingTime
	}
	return r.db.WithContext(ctx).Model(&model.DocumentSegmentQuestion{}).
		Where("id = ?", questionID).
		Updates(updates).Error
}

// UpdateQuestionIndexingCompleted updates the indexing completed status of a question
func (r *DocumentRepositoryImpl) UpdateQuestionIndexingCompleted(ctx context.Context, questionID string, completedTime *time.Time, error *string) error {
	updates := map[string]interface{}{
		"completed_at": completedTime,
	}

	if error != nil {
		updates["error"] = error
		if *error != "" {
			updates["status"] = "error"
		} else {
			updates["status"] = "completed"
		}
	} else {
		updates["status"] = "completed"
	}

	return r.db.WithContext(ctx).Model(&model.DocumentSegmentQuestion{}).
		Where("id = ?", questionID).
		Updates(updates).Error
}

// EnableDocuments enables multiple documents by setting their enabled flag to true
func (r *DocumentRepositoryImpl) EnableDocuments(ctx context.Context, datasetID string, documentIDs []string) error {
	if len(documentIDs) == 0 {
		return nil
	}

	now := time.Now()
	updates := map[string]interface{}{
		"enabled":     true,
		"disabled_at": nil,
		"disabled_by": nil,
		"updated_at":  now,
	}

	return r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.Model(&model.Document{}).
			Where("dataset_id = ? AND id IN ?", datasetID, documentIDs).
			Updates(updates).Error; err != nil {
			return err
		}

		return tx.Model(&model.DocumentSegment{}).
			Where("dataset_id = ? AND document_id IN ?", datasetID, documentIDs).
			Updates(updates).Error
	})
}

// DisableDocuments disables multiple documents by setting their enabled flag to false
func (r *DocumentRepositoryImpl) DisableDocuments(ctx context.Context, datasetID string, documentIDs []string, accountID string) error {
	if len(documentIDs) == 0 {
		return nil
	}

	now := time.Now()
	updates := map[string]interface{}{
		"enabled":     false,
		"disabled_at": now,
		"disabled_by": accountID,
		"updated_at":  now,
	}

	return r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.Model(&model.Document{}).
			Where("dataset_id = ? AND id IN ?", datasetID, documentIDs).
			Updates(updates).Error; err != nil {
			return err
		}

		return tx.Model(&model.DocumentSegment{}).
			Where("dataset_id = ? AND document_id IN ?", datasetID, documentIDs).
			Updates(updates).Error
	})
}

// ArchiveDocuments archives multiple documents by setting their archived flag to true
func (r *DocumentRepositoryImpl) ArchiveDocuments(ctx context.Context, datasetID string, documentIDs []string, accountID string) error {
	// Set archived to true and populate archived fields
	updates := map[string]interface{}{
		"archived":    true,
		"archived_at": time.Now(),
		"archived_by": accountID,
		"updated_at":  time.Now(),
		"enabled":     false, // Also disable when archiving
	}

	return r.db.WithContext(ctx).Model(&model.Document{}).
		Where("dataset_id = ? AND id IN ?", datasetID, documentIDs).
		Updates(updates).Error
}

// UnArchiveDocuments unarchives multiple documents by setting their archived flag to false
func (r *DocumentRepositoryImpl) UnArchiveDocuments(ctx context.Context, datasetID string, documentIDs []string, accountID string) error {
	// Set archived to false and clear archived fields
	updates := map[string]interface{}{
		"archived":    false,
		"archived_at": nil,
		"archived_by": nil,
		"updated_at":  time.Now(),
	}

	return r.db.WithContext(ctx).Model(&model.Document{}).
		Where("dataset_id = ? AND id IN ?", datasetID, documentIDs).
		Updates(updates).Error
}

// IncrementSegmentHitCount increments the hit count for the given segment IDs
func (r *DocumentRepositoryImpl) IncrementSegmentHitCount(ctx context.Context, segmentIDs []string) error {
	// Use raw SQL to efficiently increment hit counts for multiple segments
	query := `UPDATE document_segments SET hit_count = hit_count + 1 WHERE id IN ?`
	if err := r.db.WithContext(ctx).Exec(query, segmentIDs).Error; err != nil {
		return fmt.Errorf("failed to increment segment hit count: %w", err)
	}
	return nil
}
