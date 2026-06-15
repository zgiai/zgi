package dto

import (
	"encoding/json"
	"fmt"
	"time"

	"github.com/zgiai/zgi/api/config"
	shared_model "github.com/zgiai/zgi/api/internal/modules/shared/model"
	"github.com/zgiai/zgi/api/internal/util"
)

// DatasetListRequest represents request for dataset list
type DatasetListRequest struct {
	Page           int      `form:"page" binding:"omitempty,min=1"`
	Limit          int      `form:"limit" binding:"omitempty,min=1,max=100"`
	Sort           string   `form:"sort"`
	Keyword        string   `form:"keyword"`
	TagIDs         []string `form:"tag_ids"`
	IncludeAll     bool     `form:"include_all"`
	OrganizationID string   `form:"organization_id"`
	WorkspaceID    string   `form:"workspace_id"`
}

// GetDatasetsListRequest represents request for getting datasets list
type GetDatasetsListRequest struct {
	Page        int      `json:"page"`
	Limit       int      `json:"limit"`
	IDs         []string `json:"ids"`
	Keyword     *string  `json:"keyword"`
	TagIDs      []string `json:"tag_ids"`
	IncludeAll  bool     `json:"include_all"`
	WorkspaceID string   `json:"workspace_id"`
	AccountID   string   `json:"account_id"`
	Sort        string   `json:"sort"`
}

// DatasetCreateRequest represents request for creating dataset
type DatasetCreateRequest struct {
	WorkspaceID            *string                `json:"workspace_id"`
	Name                   string                 `json:"name" binding:"required"`
	Description            string                 `json:"description"`
	Provider               string                 `json:"provider"`
	Permission             *string                `json:"permission"`
	EmbeddingModel         *string                `json:"embedding_model"`
	EmbeddingModelProvider *string                `json:"embedding_model_provider"`
	RetrievalConfig        map[string]interface{} `json:"retrieval_config"`
	Icon                   *string                `json:"icon"`
	IconType               *string                `json:"icon_type"`
	IconBackground         *string                `json:"icon_background"`
	FolderID               *string                `json:"folder_id"`
	EntityModel            *string                `json:"entity_model"`
	EntityModelProvider    *string                `json:"entity_model_provider"`
	EnableGraphFlow        bool                   `json:"enable_graph_flow"`
}

// DatasetUpdateRequest represents request for updating dataset
type DatasetUpdateRequest struct {
	Name                   *string                `json:"name"`
	Description            *string                `json:"description"`
	EmbeddingModel         *string                `json:"embedding_model"`
	EmbeddingModelProvider *string                `json:"embedding_model_provider"`
	RetrievalConfig        map[string]interface{} `json:"retrieval_config"`
	Icon                   *string                `json:"icon"`
	IconType               *string                `json:"icon_type"`
	IconBackground         *string                `json:"icon_background"`
	WorkspaceID            *string                `json:"workspace_id"`
	EntityModel            *string                `json:"entity_model"`
	EntityModelProvider    *string                `json:"entity_model_provider"`
	EnableGraphFlow        *bool                  `json:"enable_graph_flow"`
}

// DatasetListResponse represents response for dataset list
type DatasetListResponse struct {
	Data    []DatasetResponse `json:"data"`
	HasMore bool              `json:"has_more"`
	Limit   int               `json:"limit"`
	Total   int64             `json:"total"`
	Page    int               `json:"page"`
}

// DatasetIndexingEstimateRequest represents request for dataset indexing estimate
type DatasetIndexingEstimateRequest struct {
	InfoList          map[string]interface{} `json:"info_list" binding:"required"`
	ProcessRule       map[string]interface{} `json:"process_rule" binding:"required"`
	IndexingTechnique string                 `json:"indexing_technique" binding:"required"`
	DocForm           string                 `json:"doc_form"`
	DatasetID         *string                `json:"dataset_id"`
	DocLanguage       string                 `json:"doc_language"`
}

// DatasetEditorPermissionResponse represents response for editor permission check
type DatasetEditorPermissionResponse struct {
	HasPermission bool `json:"has_permission"`
}

// EnterpriseGroupDatasetInitRequest represents request for enterprise group dataset initialization
type EnterpriseGroupDatasetInitRequest struct {
	IndexingTechnique  string                 `json:"indexing_technique" binding:"required"`
	DataSource         map[string]interface{} `json:"data_source" binding:"required"`
	ProcessRule        map[string]interface{} `json:"process_rule" binding:"required"`
	DocForm            string                 `json:"doc_form"`
	DocLanguage        string                 `json:"doc_language"`
	SegmentationMethod string                 `json:"segmentation_method"`
}

// DocumentInfo represents document information in response
type DocumentInfo struct {
	ID                string     `json:"id"`
	Name              string     `json:"name"`
	DataSourceType    string     `json:"data_source_type"`
	IndexingStatus    string     `json:"indexing_status"`
	CreatedAt         time.Time  `json:"created_at"`
	UpdatedAt         time.Time  `json:"updated_at"`
	CompletedAt       *time.Time `json:"completed_at"`
	Error             *string    `json:"error"`
	StoppedAt         *time.Time `json:"stopped_at"`
	WordCount         int        `json:"word_count"`
	TokenCount        int        `json:"token_count"`
	Position          int        `json:"position"`
	Enabled           bool       `json:"enabled"`
	DisabledAt        *time.Time `json:"disabled_at"`
	DisabledBy        *string    `json:"disabled_by"`
	Archived          bool       `json:"archived"`
	DisplayStatus     string     `json:"display_status"`
	CompletedSegments int        `json:"completed_segments"`
	TotalSegments     int        `json:"total_segments"`
}

// EnterpriseGroupDatasetInitResponse represents response for enterprise group dataset initialization
type EnterpriseGroupDatasetInitResponse struct {
	Dataset   DatasetResponse `json:"dataset"`
	Documents []DocumentInfo  `json:"documents"`
	Batch     string          `json:"batch"`
}

// PreviewDetail represents preview detail for text_model and hierarchical_model
type PreviewDetail struct {
	Content     string   `json:"content"`
	ChildChunks []string `json:"child_chunks,omitempty"`
}

// QAPreviewDetail represents preview detail for qa_model
type QAPreviewDetail struct {
	Question string `json:"question"`
	Answer   string `json:"answer"`
}

// IndexingEstimateResponse represents response for indexing estimate
type IndexingEstimateResponse struct {
	TotalSegments int               `json:"total_segments"`
	Preview       []PreviewDetail   `json:"preview"`
	QAPreview     []QAPreviewDetail `json:"qa_preview,omitempty"`
}

// DatasetResponse represents dataset response DTO
type DatasetResponse struct {
	ID                     string                 `json:"id"`
	WorkspaceID            string                 `json:"workspace_id"`
	Name                   string                 `json:"name"`
	Description            *string                `json:"description"`
	Provider               string                 `json:"provider"`
	CreatedBy              string                 `json:"created_by"`
	CreatedAt              time.Time              `json:"created_at"`
	UpdatedBy              *string                `json:"updated_by"`
	UpdatedAt              time.Time              `json:"updated_at"`
	Owner                  *string                `json:"owner"`
	EmbeddingModel         *string                `json:"embedding_model"`
	EmbeddingModelProvider *string                `json:"embedding_model_provider"`
	CollectionBindingID    *string                `json:"collection_binding_id"`
	RetrievalConfig        map[string]interface{} `json:"retrieval_config"`
	IconType               *string                `json:"icon_type"`
	Icon                   *string                `json:"icon"`
	IconBackground         *string                `json:"icon_background"`
	IconURL                string                 `json:"icon_url,omitempty"`
	AppCount               int                    `json:"app_count"`
	DocumentCount          int                    `json:"document_count"`
	AvailableDocumentCount int                    `json:"available_document_count"`
	AvailableSegmentCount  int                    `json:"available_segment_count"`
	WordCount              int                    `json:"word_count"`
	OwnerAccount           map[string]interface{} `json:"owner_account"`
	Tags                   []interface{}          `json:"tags"`
	DocForm                string                 `json:"doc_form"`
	EmbeddingAvailable     bool                   `json:"embedding_available"`
	PartialMemberList      []interface{}          `json:"partial_member_list"`
	WorkspaceInfo          *SimpleWorkspaceInfo   `json:"workspace_info,omitempty"`
	IsEditor               bool                   `json:"is_editor"`
	CanEdit                bool                   `json:"can_edit"` // NEW: Indicates if current user can edit this dataset
	EntityModel            *string                `json:"entity_model"`
	EntityModelProvider    *string                `json:"entity_model_provider"`
	EnableGraphFlow        bool                   `json:"enable_graph_flow"`
}

// MarshalJSON implements custom JSON marshaling to generate icon URLs
func (d *DatasetResponse) MarshalJSON() ([]byte, error) {
	// Generate icon URLs if needed
	icon := d.Icon
	iconType := d.IconType
	var iconUrl *string

	if icon != nil && iconType != nil && *iconType == string(shared_model.IconTypeImage) {
		// Icon is a file ID, generate signed preview URL
		signedURL, err := util.GetSignedFileURL(*icon)
		if err == nil {
			iconUrl = &signedURL
		} else {
			// Fallback: use simple URL without signature
			if config.GlobalConfig != nil && config.GlobalConfig.Console.APIURL != "" {
				consoleAPIURL := config.GlobalConfig.Console.APIURL
				iconUrlStr := fmt.Sprintf("%s/console/api/files/%s/file-preview", consoleAPIURL, *icon)
				iconUrl = &iconUrlStr
			}
		}
	}

	// Create alias to avoid infinite recursion
	type Alias DatasetResponse
	return json.Marshal(&struct {
		*Alias
		IconUrl *string `json:"icon_url"`
	}{
		Alias:   (*Alias)(d),
		IconUrl: iconUrl,
	})
}

// DatasetFolderResponse represents dataset folder response DTO
type DatasetFolderResponse struct {
	ID             string    `json:"id"`
	WorkspaceID    string    `json:"workspace_id"`
	Name           string    `json:"name"`
	Description    *string   `json:"description"`
	ParentID       *string   `json:"parent_id"`
	CreatedBy      string    `json:"created_by"`
	CreatedAt      time.Time `json:"created_at"`
	UpdatedBy      *string   `json:"updated_by"`
	UpdatedAt      time.Time `json:"updated_at"`
	Icon           *string   `json:"icon"`
	IconType       *string   `json:"icon_type"`
	IconBackground *string   `json:"icon_background"`
	Position       int       `json:"position"`
	Permission     string    `json:"permission"`
	CanEdit        bool      `json:"can_edit"` // Indicates if current user can edit this folder
}

// DatasetFolderDetailResponse represents extended dataset folder response DTO with tenant info
type DatasetFolderDetailResponse struct {
	ID             string                 `json:"id"`
	WorkspaceID    string                 `json:"workspace_id"`
	Name           string                 `json:"name"`
	Description    *string                `json:"description"`
	ParentID       *string                `json:"parent_id"`
	CreatedBy      string                 `json:"created_by"`
	CreatedAt      time.Time              `json:"created_at"`
	UpdatedBy      *string                `json:"updated_by"`
	UpdatedAt      time.Time              `json:"updated_at"`
	Icon           *string                `json:"icon"`
	IconType       *string                `json:"icon_type"`
	IconBackground *string                `json:"icon_background"`
	Position       int                    `json:"position"`
	Permission     string                 `json:"permission"`
	Tenant         map[string]interface{} `json:"tenant"`
	CanEdit        bool                   `json:"can_edit"` // Indicates if current user can edit this folder
}

// MarshalJSON implements custom JSON marshaling to generate icon URLs
func (d *DatasetFolderDetailResponse) MarshalJSON() ([]byte, error) {
	// Generate icon URLs if needed
	icon := d.Icon
	iconType := d.IconType
	var iconUrl *string

	if icon != nil && iconType != nil && *iconType == string(shared_model.IconTypeImage) {
		// Icon is a file ID, generate signed preview URL
		signedURL, err := util.GetSignedFileURL(*icon)
		if err == nil {
			iconUrl = &signedURL
		} else {
			// Fallback: use simple URL without signature
			if config.GlobalConfig != nil && config.GlobalConfig.Console.APIURL != "" {
				consoleAPIURL := config.GlobalConfig.Console.APIURL
				iconUrlStr := fmt.Sprintf("%s/console/api/files/%s/file-preview", consoleAPIURL, *icon)
				iconUrl = &iconUrlStr
			}
		}
	}

	// Create alias to avoid infinite recursion
	type Alias DatasetFolderDetailResponse
	return json.Marshal(&struct {
		*Alias
		IconUrl *string `json:"icon_url"`
	}{
		Alias:   (*Alias)(d),
		IconUrl: iconUrl,
	})
}

// DatasetWithFolderResponse represents dataset response with folder information
type DatasetWithFolderResponse struct {
	DatasetResponse
	Folder *DatasetFolderResponse `json:"folder"`
}

// DatasetListWithFoldersResponse represents response for dataset list with folder information
type DatasetListWithFoldersResponse struct {
	Datasets      []DatasetResponse       `json:"datasets"`
	Folders       []DatasetFolderResponse `json:"folders"`
	CurrentFolder *DatasetFolderResponse  `json:"current_folder,omitempty"`
}

// DatasetListWithFoldersRequest represents request for dataset list with folders
type DatasetListWithFoldersRequest struct {
	FolderID    string `form:"folder_id"`
	Keyword     string `form:"keyword"`
	Sort        string `form:"sort"`
	Page        int    `form:"page" binding:"omitempty,min=1"`
	Limit       int    `form:"limit" binding:"omitempty,min=1"`
	WorkspaceID string `form:"workspace_id"`
}

// DatasetListWithFoldersPaginatedResponse represents paginated response for dataset list with folder information
type DatasetListWithFoldersPaginatedResponse struct {
	Datasets       []DatasetResponse       `json:"datasets"`
	DatasetTotal   int64                   `json:"dataset_total"`
	DatasetPage    int                     `json:"dataset_page"`
	DatasetLimit   int                     `json:"dataset_limit"`
	DatasetHasMore bool                    `json:"dataset_has_more"`
	Folders        []DatasetFolderResponse `json:"folders"`
	FolderTotal    int64                   `json:"folder_total"`
	FolderPage     int                     `json:"folder_page"`
	FolderLimit    int                     `json:"folder_limit"`
	FolderHasMore  bool                    `json:"folder_has_more"`
	CurrentFolder  *DatasetFolderResponse  `json:"current_folder,omitempty"`
}

// DatasetFolderCreateRequest represents request for creating dataset folder
type DatasetFolderCreateRequest struct {
	Name           string  `json:"name" binding:"required"`
	WorkspaceID    *string `json:"workspace_id"`
	Description    *string `json:"description"`
	ParentID       *string `json:"parent_id"`
	Icon           *string `json:"icon"`
	IconType       *string `json:"icon_type"`
	IconBackground *string `json:"icon_background"`
	Position       *int    `json:"position"`
	Permission     *string `json:"permission"`
}

// DatasetFolderUpdateRequest represents request for updating dataset folder
type DatasetFolderUpdateRequest struct {
	Name           *string `json:"name"`
	TenantID       *string `json:"tenant_id"`
	WorkspaceID    *string `json:"workspace_id"`
	Description    *string `json:"description"`
	ParentID       *string `json:"parent_id"`
	Icon           *string `json:"icon"`
	IconType       *string `json:"icon_type"`
	IconBackground *string `json:"icon_background"`
	Position       *int    `json:"position"`
	Permission     *string `json:"permission"`
}

// DatasetFolderListRequest represents request for dataset folder list with pagination
type DatasetFolderListRequest struct {
	FolderID    string `form:"folder_id" binding:"omitempty"`
	Page        int    `form:"page" binding:"omitempty,min=1"`
	Limit       int    `form:"limit" binding:"omitempty,min=1,max=100"`
	Sort        string `form:"sort"`
	Keyword     string `form:"keyword"`
	WorkspaceID string `form:"workspace_id"`
}

// DatasetFolderPaginationResponse represents paginated response for dataset folders
type DatasetFolderPaginationResponse struct {
	Folders      []DatasetFolderResponse `json:"folders"`
	Total        int64                   `json:"total"`
	Page         int                     `json:"page"`
	Limit        int                     `json:"limit"`
	HasMore      bool                    `json:"has_more"`
	ParentFolder *DatasetFolderResponse  `json:"parent_folder,omitempty"`
}

// MoveDatasetToFolderRequest represents request for moving dataset to folder
type MoveDatasetToFolderRequest struct {
	DatasetID string `json:"dataset_id" binding:"required"`
	FolderID  string `json:"folder_id" binding:"omitempty"`
}

// ChildChunkCreateRequest represents the request for creating a child chunk
type ChildChunkCreateRequest struct {
	Content string `json:"content" binding:"required"` // Child chunk content (required)
	// Position *int    `json:"position"`                   // Position of the child chunk (optional)
	// Type     *string `json:"type"`                       // Type of the child chunk (optional)
}

// ChildChunkUpdateRequest represents the request for updating a child chunk
type ChildChunkUpdateRequest struct {
	Content  *string `json:"content"`  // Updated child chunk content (optional)
	Position *int    `json:"position"` // Updated position of the child chunk (optional)
	Type     *string `json:"type"`     // Updated type of the child chunk (optional)
}

// DocumentSegmentQuestionResponse represents the response for a document segment question
type DocumentSegmentQuestionResponse struct {
	ID             string `json:"id"`
	OrganizationID string `json:"organization_id"`
	DatasetID      string `json:"dataset_id"`
	DocumentID     string `json:"document_id"`
	SegmentID      string `json:"segment_id"`
	Question       string `json:"question"`
	CreatedBy      string `json:"created_by"`
	CreatedAt      int64  `json:"created_at"`
	UpdatedAt      int64  `json:"updated_at"`
	UpdatedBy      string `json:"updated_by,omitempty"`
}

// DocumentSegmentQuestionCreateRequest represents the request to create a document segment question
type DocumentSegmentQuestionCreateRequest struct {
	SegmentID string `json:"segment_id"`
	Question  string `json:"question" binding:"required"`
}

// DocumentSegmentQuestionUpdateRequest represents the request to update a document segment question
type DocumentSegmentQuestionUpdateRequest struct {
	Question string `json:"question" binding:"required"`
}

// DocumentSegmentQuestionListRequest represents the request to list document segment questions
type DocumentSegmentQuestionListRequest struct {
	Page  int `form:"page" binding:"omitempty,min=1"`
	Limit int `form:"limit" binding:"omitempty,min=1,max=100"`
}

// DocumentSegmentQuestionListResponse represents the response for listing document segment questions
type DocumentSegmentQuestionListResponse struct {
	Data    []DocumentSegmentQuestionResponse `json:"data"`
	Total   int64                             `json:"total"`
	Page    int                               `json:"page"`
	Limit   int                               `json:"limit"`
	HasMore bool                              `json:"has_more"`
}

// DocumentSegmentQuestionBatchCreateItem represents an item in the batch create request
type DocumentSegmentQuestionBatchCreateItem struct {
	SegmentID string `json:"segment_id"`
	Question  string `json:"question" binding:"required"`
}

// DocumentSegmentQuestionBatchCreateRequest represents the request to batch create document segment questions
type DocumentSegmentQuestionBatchCreateRequest struct {
	Questions []DocumentSegmentQuestionBatchCreateItem `json:"questions" binding:"required,min=1,max=100"`
}

// DocumentSegmentQuestionGenerateRequest represents the request to generate questions for a segment
type DocumentSegmentQuestionGenerateRequest struct {
	// Optional explicit model selection; when omitted, use tenant default
	Model *ModelSpec `json:"model,omitempty"`
	// Optional count override; when omitted, use query param or default
	Count *int `json:"count,omitempty"`
}

// DocumentSegmentQuestionBatchCreateResponse represents the response for batch creating document segment questions
type DocumentSegmentQuestionBatchCreateResponse struct {
	Questions []DocumentSegmentQuestionResponse `json:"questions"`
	Count     int                               `json:"count"`
}

// DatasetQueryResponse represents a dataset query response
type DatasetQueryResponse struct {
	ID            string    `json:"id"`
	DatasetID     string    `json:"dataset_id"`
	Content       string    `json:"content"`
	Source        string    `json:"source"`
	SourceAppID   *string   `json:"source_app_id"`
	CreatedByRole string    `json:"created_by_role"`
	CreatedBy     string    `json:"created_by"`
	CreatedAt     time.Time `json:"created_at"`

	Results     *HitTestingResponse `json:"results,omitempty"`
	ElapsedTime *float64            `json:"elapsed_time,omitempty"`
	HitCount    *int                `json:"hit_count,omitempty"`

	QueryType   string  `json:"query_type"`
	BatchTaskID *string `json:"batch_task_id,omitempty"`
	BatchName   *string `json:"batch_name,omitempty"`
}

// DatasetQueryListResponse represents a list of dataset queries
type DatasetQueryListResponse struct {
	Data    []DatasetQueryResponse `json:"data"`
	HasMore bool                   `json:"has_more"`
	Limit   int                    `json:"limit"`
	Total   int64                  `json:"total"`
	Page    int                    `json:"page"`
}

// SaveBatchHitTestingRequest represents the request to save batch hit testing results
type SaveBatchHitTestingRequest struct {
	BatchName string `json:"batch_name" binding:"required"`
}
