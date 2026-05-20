package model

import (
	"database/sql/driver"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/zgiai/zgi/api/internal/dto"
)

// JSONMap custom JSON type for JSONB fields
type JSONMap map[string]interface{}

// Value Implements driver.Valuer interface
func (j JSONMap) Value() (driver.Value, error) {
	if j == nil {
		return nil, nil
	}
	return json.Marshal(j)
}

// Scan implements sql.Scanner interface
func (j *JSONMap) Scan(value interface{}) error {
	if value == nil {
		*j = nil
		return nil
	}

	var bytes []byte
	switch v := value.(type) {
	case []byte:
		bytes = v
	case string:
		bytes = []byte(v)
	default:
		return fmt.Errorf("cannot scan %T into JSONMap", value)
	}

	// Try to unmarshal as map first
	var result map[string]interface{}
	if err := json.Unmarshal(bytes, &result); err == nil {
		*j = JSONMap(result)
		return nil
	}

	// If it's not a map, try to unmarshal as array and convert to map
	var arrayResult []interface{}
	if err := json.Unmarshal(bytes, &arrayResult); err == nil {
		// Convert array to map with "keywords" key
		*j = JSONMap{"keywords": arrayResult}
		return nil
	}

	// If both fail, return the original error
	return json.Unmarshal(bytes, j)
}

// Dataset represents a dataset entity
type Dataset struct {
	ID                     string    `json:"id" gorm:"primaryKey;type:uuid;default:uuid_generate_v4()"`
	OrganizationID         string    `json:"organization_id" gorm:"type:uuid;not null;index:dataset_organization_idx"`
	WorkspaceID            string    `json:"workspace_id" gorm:"type:uuid;index:dataset_workspace_idx"`
	Name                   string    `json:"name" gorm:"type:varchar(255);not null"`
	Description            *string   `json:"description" gorm:"type:text"`
	Provider               string    `json:"provider" gorm:"type:varchar(255);not null;default:'vendor'"`
	EnableGraphFlow        bool      `json:"enable_graph_flow" gorm:"default:false"` // GraphFlow switch
	CreatedBy              string    `json:"created_by" gorm:"type:uuid;not null"`
	CreatedAt              time.Time `json:"created_at" gorm:"not null;default:CURRENT_TIMESTAMP(0)"`
	UpdatedBy              *string   `json:"updated_by" gorm:"type:uuid"`
	UpdatedAt              time.Time `json:"updated_at" gorm:"not null;default:CURRENT_TIMESTAMP(0)"`
	Owner                  *string   `json:"owner" gorm:"type:uuid"`
	EmbeddingModel         *string   `json:"embedding_model" gorm:"type:varchar(255)"`
	EmbeddingModelProvider *string   `json:"embedding_model_provider" gorm:"type:varchar(255)"`
	EntityModel            *string   `json:"entity_model" gorm:"type:varchar(255)"`
	EntityModelProvider    *string   `json:"entity_model_provider" gorm:"type:varchar(255)"`
	CollectionBindingID    *string   `json:"collection_binding_id" gorm:"type:uuid"`
	RetrievalConfig        JSONMap   `json:"retrieval_config" gorm:"type:jsonb"`
	IconType               *string   `json:"icon_type" gorm:"type:varchar(255)"`
	Icon                   *string   `json:"icon" gorm:"type:varchar(255)"`
	IconBackground         *string   `json:"icon_background" gorm:"type:varchar(255)"`
	ProcessRule            JSONMap   `json:"process_rule" gorm:"type:jsonb"`

	// Calculated fields (populated during queries)
	AppCount               int `json:"app_count" gorm:"-"`
	DocumentCount          int `json:"document_count" gorm:"-"`
	AvailableDocumentCount int `json:"available_document_count" gorm:"-"`
	AvailableSegmentCount  int `json:"available_segment_count" gorm:"-"`
	WordCount              int `json:"word_count" gorm:"-"`

	// Associated fields (for response, not stored in DB)
	OwnerAccount map[string]interface{} `json:"owner_account" gorm:"-"`
	Tags         []interface{}          `json:"tags" gorm:"-"`
	DocForm      string                 `json:"doc_form" gorm:"-"`
}

// TableName specifies table name
func (Dataset) TableName() string {
	return "datasets"
}

// GenCollectionNameByID generates collection name for vector database
// current logic: normalized_dataset_id = dataset_id.replace("-", "_"); return f"Vector_index_{normalized_dataset_id}_Node"
func GenCollectionNameByID(datasetID string) string {
	normalizedDatasetID := strings.ReplaceAll(datasetID, "-", "_")
	return fmt.Sprintf("Vector_index_%s_Node", normalizedDatasetID)
}

// GenQuestionCollectionNameByID generates question collection name for vector database
// This creates a separate class for questions associated with a dataset
func GenQuestionCollectionNameByID(datasetID string) string {
	normalizedDatasetID := strings.ReplaceAll(datasetID, "-", "_")
	return fmt.Sprintf("Vector_index_%s_Question_Node", normalizedDatasetID)
}

// GenCollectionName generates collection name for this dataset instance
func (d *Dataset) GenCollectionName() string {
	return GenCollectionNameByID(d.ID)
}

type DatasetPermissionEnum string

const (
	DatasetPermissionOnlyMe   DatasetPermissionEnum = "only_me"
	DatasetPermissionAllTeam  DatasetPermissionEnum = "all_team"
	DatasetPermissionPartial  DatasetPermissionEnum = "partial_members"
	DatasetPermissionAllGroup DatasetPermissionEnum = "all_group"
)

// DatasetPermission represents dataset access permissions
type DatasetPermission struct {
	ID            string    `json:"id" gorm:"primaryKey;type:uuid;default:uuid_generate_v4()"`
	DatasetID     string    `json:"dataset_id" gorm:"type:uuid;not null;index:idx_dataset_permissions_dataset_id"`
	AccountID     string    `json:"account_id" gorm:"type:uuid;not null;index:idx_dataset_permissions_account_id"`
	TenantID      string    `json:"tenant_id" gorm:"type:uuid;not null;index:idx_dataset_permissions_tenant_id"`
	HasPermission bool      `json:"has_permission" gorm:"not null;default:true"`
	CreatedAt     time.Time `json:"created_at" gorm:"not null;default:CURRENT_TIMESTAMP(0)"`
}

// TableName specifies table name
func (DatasetPermission) TableName() string {
	return "dataset_permissions"
}

// DatasetQuery represents dataset query logs
type DatasetQuery struct {
	ID            string    `json:"id" gorm:"primaryKey;type:uuid;default:uuid_generate_v4()"`
	DatasetID     string    `json:"dataset_id" gorm:"type:uuid;not null;index:dataset_query_dataset_id_idx"`
	Content       string    `json:"content" gorm:"type:text;not null"`
	Source        string    `json:"source" gorm:"type:varchar(255);not null"`
	SourceAppID   *string   `json:"source_app_id" gorm:"type:uuid"`
	CreatedByRole string    `json:"created_by_role" gorm:"not null"`
	CreatedBy     string    `json:"created_by" gorm:"type:uuid;not null"`
	CreatedAt     time.Time `json:"created_at" gorm:"not null;default:CURRENT_TIMESTAMP"`

	// store query results
	Results     *dto.HitTestingResponse `json:"results" gorm:"type:jsonb"`
	ElapsedTime *float64                `json:"elapsed_time" gorm:"type:decimal"`
	HitCount    *int                    `json:"hit_count" gorm:"type:integer"`

	// batch query
	QueryType   string  `json:"query_type" gorm:"type:varchar(50);default:'single'"`               // single, batch_saved
	BatchTaskID *string `json:"batch_task_id" gorm:"type:uuid;index:dataset_query_batch_task_idx"` // task ID
	BatchName   *string `json:"batch_name" gorm:"type:varchar(255)"`
}

// TableName specifies table name
func (DatasetQuery) TableName() string {
	return "dataset_queries"
}

// DatasetCollectionBinding represents dataset collection bindings
type DatasetCollectionBinding struct {
	ID             string    `json:"id" gorm:"primaryKey;type:uuid;default:uuid_generate_v4()"`
	ProviderName   string    `json:"provider_name" gorm:"type:varchar(40);not null;index:provider_model_name_idx"`
	ModelName      string    `json:"model_name" gorm:"type:varchar(255);not null;index:provider_model_name_idx"`
	Type           string    `json:"type" gorm:"type:varchar(40);not null;default:'dataset'"`
	CollectionName string    `json:"collection_name" gorm:"type:varchar(64);not null"`
	CreatedAt      time.Time `json:"created_at" gorm:"not null;default:CURRENT_TIMESTAMP(0)"`
}

// TableName specifies table name
func (DatasetCollectionBinding) TableName() string {
	return "dataset_collection_bindings"
}

// DatasetAutoDisableLog represents auto-disable logs
type DatasetAutoDisableLog struct {
	ID         string    `json:"id" gorm:"primaryKey;type:uuid;default:uuid_generate_v4()"`
	TenantID   string    `json:"tenant_id" gorm:"type:uuid;not null;index:dataset_auto_disable_log_tenant_idx"`
	DatasetID  string    `json:"dataset_id" gorm:"type:uuid;not null;index:dataset_auto_disable_log_dataset_idx"`
	DocumentID string    `json:"document_id" gorm:"type:uuid;not null"`
	Notified   bool      `json:"notified" gorm:"not null;default:false"`
	CreatedAt  time.Time `json:"created_at" gorm:"not null;default:CURRENT_TIMESTAMP(0);index:dataset_auto_disable_log_created_atx"`
}

// TableName specifies table name
func (DatasetAutoDisableLog) TableName() string {
	return "dataset_auto_disable_logs"
}

// ExternalKnowledgeBinding represents the binding between dataset and external knowledge API
type ExternalKnowledgeBinding struct {
	ID                     string    `json:"id" gorm:"primaryKey;type:uuid;default:uuid_generate_v4()"`
	DatasetID              string    `json:"dataset_id" gorm:"type:uuid;not null;index:external_knowledge_binding_dataset_idx"`
	ExternalKnowledgeApiID string    `json:"external_knowledge_api_id" gorm:"type:uuid;not null;index:external_knowledge_binding_api_idx"`
	ExternalKnowledgeID    string    `json:"external_knowledge_id" gorm:"type:varchar(255);not null"`
	TenantID               string    `json:"tenant_id" gorm:"type:uuid;not null;index:external_knowledge_binding_tenant_idx"`
	CreatedBy              string    `json:"created_by" gorm:"type:uuid;not null"`
	CreatedAt              time.Time `json:"created_at" gorm:"not null;default:CURRENT_TIMESTAMP(0)"`
}

func (ExternalKnowledgeBinding) TableName() string {
	return "external_knowledge_bindings"
}

// ExternalKnowledgeApi represents the external knowledge API config
type ExternalKnowledgeApi struct {
	ID          string    `json:"id" gorm:"primaryKey;type:uuid;default:uuid_generate_v4()"`
	TenantID    string    `json:"tenant_id" gorm:"type:uuid;not null;index:external_knowledge_api_tenant_idx"`
	Name        string    `json:"name" gorm:"type:varchar(255);not null"`
	Description string    `json:"description" gorm:"type:text"`
	Endpoint    string    `json:"endpoint" gorm:"type:varchar(255);not null"`
	ApiKey      string    `json:"api_key" gorm:"type:varchar(255);not null"`
	Settings    string    `json:"settings" gorm:"type:text"`
	CreatedBy   string    `json:"created_by" gorm:"type:uuid;not null"`
	CreatedAt   time.Time `json:"created_at" gorm:"not null;default:CURRENT_TIMESTAMP(0)"`
}

func (ExternalKnowledgeApi) TableName() string {
	return "external_knowledge_apis"
}

// DatasetPaginationResponse represents a paginated response of datasets
type DatasetPaginationResponse struct {
	Data    []*Dataset `json:"data"`
	Page    int        `json:"page"`
	Limit   int        `json:"limit"`
	Total   int64      `json:"total"`
	HasMore bool       `json:"has_more"`
	Search  string     `json:"search,omitempty"`
}

// DatasetProcessRule represents dataset processing rules
type DatasetProcessRule struct {
	ID        string    `json:"id" gorm:"primaryKey;type:uuid;default:uuid_generate_v4()"`
	DatasetID string    `json:"dataset_id" gorm:"type:uuid;not null;index:dataset_process_rule_dataset_idx"`
	Mode      string    `json:"mode" gorm:"type:varchar(255);not null"`
	Rules     JSONMap   `json:"rules" gorm:"type:jsonb"`
	CreatedBy string    `json:"created_by" gorm:"type:uuid;not null"`
	CreatedAt time.Time `json:"created_at" gorm:"not null;default:CURRENT_TIMESTAMP(0)"`
}

func (DatasetProcessRule) TableName() string {
	return "dataset_process_rules"
}

// DatasetMetadata represents dataset metadata definitions
type DatasetMetadata struct {
	ID        string    `json:"id" gorm:"primaryKey;type:uuid;default:uuid_generate_v4()"`
	TenantID  string    `json:"tenant_id" gorm:"type:uuid;not null;index:dataset_metadata_tenant_idx"`
	DatasetID string    `json:"dataset_id" gorm:"type:uuid;not null;index:dataset_metadata_dataset_idx"`
	Type      string    `json:"type" gorm:"type:varchar(255);not null"`
	Name      string    `json:"name" gorm:"type:varchar(255);not null"`
	CreatedAt time.Time `json:"created_at" gorm:"not null;default:CURRENT_TIMESTAMP(0)"`
	UpdatedAt time.Time `json:"updated_at" gorm:"not null;default:CURRENT_TIMESTAMP(0)"`
	CreatedBy string    `json:"created_by" gorm:"type:uuid;not null"`
	UpdatedBy *string   `json:"updated_by" gorm:"type:uuid"`
}

// TableName specifies table name
func (DatasetMetadata) TableName() string {
	return "dataset_metadatas"
}

// DatasetMetadataBinding represents the binding between datasets and metadata
type DatasetMetadataBinding struct {
	ID         string    `json:"id" gorm:"primaryKey;type:uuid;default:uuid_generate_v4()"`
	TenantID   string    `json:"tenant_id" gorm:"type:uuid;not null;index:dataset_metadata_binding_tenant_idx"`
	DatasetID  string    `json:"dataset_id" gorm:"type:uuid;not null;index:dataset_metadata_binding_dataset_idx"`
	MetadataID string    `json:"metadata_id" gorm:"type:uuid;not null;index:dataset_metadata_binding_metadata_idx"`
	DocumentID string    `json:"document_id" gorm:"type:uuid;not null;index:dataset_metadata_binding_document_idx"`
	CreatedAt  time.Time `json:"created_at" gorm:"not null;default:CURRENT_TIMESTAMP(0)"`
	CreatedBy  string    `json:"created_by" gorm:"type:uuid;not null"`
}

// TableName specifies table name
func (DatasetMetadataBinding) TableName() string {
	return "dataset_metadata_bindings"
}
