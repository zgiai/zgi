// Package indexing provides document indexing functionality for the dataset module.
package indexing

import (
	dataset_repository "github.com/zgiai/zgi/api/internal/modules/dataset/repository"
	llmclient "github.com/zgiai/zgi/api/internal/modules/llm/client"
	llmdefaultservice "github.com/zgiai/zgi/api/internal/modules/llm/defaultmodel/service"
	"github.com/zgiai/zgi/api/pkg/storage"
)

// IndexType represents the type of index processor
type IndexType string

const (
	ParagraphIndex   IndexType = "text_model"         // Paragraph indexing corresponds to text_model
	QAIndex          IndexType = "qa_model"           // QA indexing corresponds to qa_model
	ParentChildIndex IndexType = "hierarchical_model" // Parent-child indexing corresponds to hierarchical_model
	TableIndex       IndexType = "table_model"        // Table indexing corresponds to table_model
)

// IndexProcessorFactory handles creation of different index processors
type IndexProcessorFactory struct {
	indexType       IndexType
	storage         storage.Storage
	documentRepo    dataset_repository.DocumentRepository
	defaultModelSvc llmdefaultservice.DefaultModelService
	llmClient       llmclient.LLMClient
	tenantID        string
}

// NewIndexProcessorFactory creates a new IndexProcessorFactory instance
func NewIndexProcessorFactory(indexType IndexType, storage storage.Storage, documentRepo dataset_repository.DocumentRepository, defaultModelSvc llmdefaultservice.DefaultModelService, llmClient llmclient.LLMClient, tenantID string) *IndexProcessorFactory {
	return &IndexProcessorFactory{
		indexType:       indexType,
		storage:         storage,
		documentRepo:    documentRepo,
		defaultModelSvc: defaultModelSvc,
		llmClient:       llmClient,
		tenantID:        tenantID,
	}
}

// CreateIndexProcessor creates an index processor
func (f *IndexProcessorFactory) CreateIndexProcessor() (BaseIndexProcessor, error) {
	// Check if index type is specified
	if f.indexType == "" {
		return nil, &IndexProcessorError{Message: "index type must be specified"}
	}

	// Create the appropriate processor based on index type
	switch f.indexType {
	case "text_model":
		return NewParagraphIndexProcessor(f.storage, f.defaultModelSvc, f.llmClient, f.tenantID), nil
	case "qa_model":
		return NewQAIndexProcessor(f.storage, f.defaultModelSvc, f.llmClient, f.tenantID), nil
	case "hierarchical_model":
		return NewParentChildIndexProcessor(f.storage, f.documentRepo, f.defaultModelSvc, f.llmClient, f.tenantID), nil
	case "table_model":
		return NewTableIndexProcessor(f.storage, f.defaultModelSvc, f.llmClient, f.tenantID), nil
	default:
		return nil, &IndexProcessorError{Message: "index type " + string(f.indexType) + " is not supported"}
	}
}

// IndexProcessorError custom error type
type IndexProcessorError struct {
	Message string
}

func (e *IndexProcessorError) Error() string {
	return e.Message
}
