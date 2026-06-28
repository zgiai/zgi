package vectordb

import (
	"context"

	"github.com/zgiai/zgi/api/config"
)

// VectorDB represents a generic vector database interface
type VectorDB interface {
	// StoreVector stores a vector with metadata
	StoreVector(ctx context.Context, id, className string, properties map[string]interface{}, vector []float64) error

	// DeleteVector deletes a vector by ID from a class/collection.
	DeleteVector(ctx context.Context, id, className string) error

	// SearchVectors performs similarity search
	SearchVectors(ctx context.Context, className string, vector []float64, limit int) ([]map[string]interface{}, error)

	// SearchByFullText performs BM25 full text search
	SearchByFullText(ctx context.Context, className, query string, limit int) ([]map[string]interface{}, error)

	// CreateClass creates a new class/collection schema
	CreateClass(ctx context.Context, className string, properties []map[string]interface{}) error

	// HealthCheck checks if the vector database is accessible
	HealthCheck(ctx context.Context) error
}

// VectorObject represents one vector object to be stored in the vector database.
type VectorObject struct {
	ID         string                 `json:"id"`
	Class      string                 `json:"class"`
	Properties map[string]interface{} `json:"properties"`
	Vector     []float64              `json:"vector"`
}

// BatchVectorDB represents a vector database that supports batch object writes.
type BatchVectorDB interface {
	VectorDB

	// StoreVectors stores multiple vectors with metadata.
	StoreVectors(ctx context.Context, objects []VectorObject) error
}

// FieldDeleteVectorDB represents a vector database that supports deleting
// multiple objects by a metadata field in one request.
type FieldDeleteVectorDB interface {
	VectorDB

	// DeleteObjectsByField deletes all objects whose metadata field equals the value.
	DeleteObjectsByField(ctx context.Context, className, fieldName, fieldValue string) error
}

// BatchVectorError reports per-object failures from a batch vector write.
type BatchVectorError struct {
	Errors map[string]error
}

func (e *BatchVectorError) Error() string {
	if e == nil || len(e.Errors) == 0 {
		return "vector batch write failed"
	}
	return "vector batch write failed for one or more objects"
}

// ExtendedVectorDB represents an extended vector database interface with additional methods
type ExtendedVectorDB interface {
	VectorDB

	// SearchVectorsWithQuestions performs similarity search including both regular segments and questions
	SearchVectorsWithQuestions(ctx context.Context, className, questionClassName string, vector []float64, limit int) ([]map[string]interface{}, error)
}

// NewVectorDB creates a new vector database client based on configuration
func NewVectorDB(cfg *config.VectorStoreConfig) (VectorDB, error) {
	switch cfg.Type {
	case "weaviate":
		return NewWeaviateClient(cfg), nil
	default:
		// Return a mock implementation for unsupported types
		return &MockVectorDB{}, nil
	}
}

// MockVectorDB is a mock implementation for testing or when no vector DB is configured
type MockVectorDB struct{}

func (m *MockVectorDB) StoreVector(ctx context.Context, id, className string, properties map[string]interface{}, vector []float64) error {
	// Mock implementation - just log the operation
	return nil
}

func (m *MockVectorDB) DeleteVector(ctx context.Context, id, className string) error {
	// Mock implementation - just log the operation
	return nil
}

func (m *MockVectorDB) DeleteObjectsByField(ctx context.Context, className, fieldName, fieldValue string) error {
	// Mock implementation - just log the operation
	return nil
}

func (m *MockVectorDB) StoreVectors(ctx context.Context, objects []VectorObject) error {
	// Mock implementation - just log the operation
	return nil
}

func (m *MockVectorDB) SearchVectors(ctx context.Context, className string, vector []float64, limit int) ([]map[string]interface{}, error) {
	// Mock implementation - return empty results
	return []map[string]interface{}{}, nil
}

func (m *MockVectorDB) SearchByFullText(ctx context.Context, className, query string, limit int) ([]map[string]interface{}, error) {
	// Mock implementation - return empty results
	return []map[string]interface{}{}, nil
}

func (m *MockVectorDB) CreateClass(ctx context.Context, className string, properties []map[string]interface{}) error {
	// Mock implementation - just log the operation
	return nil
}

func (m *MockVectorDB) HealthCheck(ctx context.Context) error {
	// Mock implementation - always healthy
	return nil
}
