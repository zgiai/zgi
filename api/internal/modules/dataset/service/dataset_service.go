package service

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"gorm.io/gorm"

	"github.com/google/uuid"
	"github.com/zgiai/zgi/api/internal/dto"
	graphflow_repo "github.com/zgiai/zgi/api/internal/modules/dataset/graphflow/repository"
	"github.com/zgiai/zgi/api/internal/modules/dataset/indexing"
	"github.com/zgiai/zgi/api/internal/modules/dataset/model"
	"github.com/zgiai/zgi/api/internal/modules/dataset/repository"
	"github.com/zgiai/zgi/api/internal/modules/dataset/retrieval"
	llmclient "github.com/zgiai/zgi/api/internal/modules/llm/client"
	llmdefaultservice "github.com/zgiai/zgi/api/internal/modules/llm/defaultmodel/service"
	quota_model "github.com/zgiai/zgi/api/internal/modules/quota/model"
	interfaces "github.com/zgiai/zgi/api/internal/modules/shared/interface"
	workspace_model "github.com/zgiai/zgi/api/internal/modules/workspace/model"
	"github.com/zgiai/zgi/api/pkg/queue"
	"github.com/zgiai/zgi/api/pkg/storage"
	"github.com/zgiai/zgi/api/pkg/vectordb"
)

var (
	ErrDatasetAccessDenied      = errors.New("dataset access denied")
	ErrInvalidDatasetPermission = errors.New("invalid dataset permission")
)

type DatasetService interface {
	SetOrganizationService(organizationService interfaces.OrganizationService)
	CreateDataset(ctx context.Context, req *CreateDatasetRequest) (*model.Dataset, error)
	GetDatasetByID(ctx context.Context, id string) (*model.Dataset, error)
	GetDatasetsByIDs(ctx context.Context, ids []string) ([]*model.Dataset, error)
	UpdateDataset(ctx context.Context, req *UpdateDatasetRequest) (*model.Dataset, error)
	DeleteDataset(ctx context.Context, datasetID, accountID, tenantID string) error
	GetDatasetCount(ctx context.Context, tenantID string) (int64, error)

	GetPaginateDatasetsByTenantIDs(ctx context.Context, req *GetPaginateDatasetsByTenantIDsRequest) (*model.DatasetPaginationResponse, error)
	GetDatasetsList(ctx context.Context, req *GetDatasetsListRequest) (*GetDatasetsListResponse, error)
	GetDatasetsListEx(ctx context.Context, req *GetDatasetsListExRequest) (*GetDatasetsListExResponse, error)

	GetDatasetWithPermissionCheck(ctx context.Context, datasetID, accountID, workspaceID string) (*model.Dataset, error)
	CheckDatasetPermission(ctx context.Context, datasetID, accountID, tenantID string) (bool, error)
	CheckEditorPermission(ctx context.Context, datasetID, accountID, tenantID string) (bool, error)

	GetDatasetAppDefault(ctx context.Context, datasetID, accountID, tenantID string) (map[string]interface{}, error)

	EstimateIndexing(ctx context.Context, req *IndexingEstimateRequest) (*IndexingEstimateResponse, error)

	UpdateDatasetEx(ctx context.Context, req *UpdateDatasetExRequest) (*model.Dataset, error)

	GetDatasetWithTenantInfo(ctx context.Context, datasetID string) (*DatasetWithTenant, error)
	GetDatasetsWithPermissions(ctx context.Context, accountID string) ([]*DatasetWithPermission, error)
	GetDatasetsByAccountAndTenant(ctx context.Context, accountID, tenantID string) ([]*model.Dataset, error)

	GetDatasetWithDocumentStats(ctx context.Context, datasetID string) (*DatasetWithStats, error)
	GetDatasetDocuments(ctx context.Context, datasetID string, offset, limit int) ([]*model.Document, error)

	// UpdateSegmentsStatus updates the status of multiple segments
	UpdateSegmentsStatus(ctx context.Context, segmentIDs []string, action string, dataset *model.Dataset, document *model.Document) error
}

type GetPaginateDatasetsByTenantIDsRequest struct {
	TenantIDs         []string `json:"tenant_ids" binding:"required"`
	Page              int      `json:"page"`
	Limit             int      `json:"limit"`
	Search            *string  `json:"search,omitempty"`
	DatasetAdmin      bool     `json:"dataset_admin"`
	AccountID         string   `json:"account_id" binding:"required"`
	GroupID           string   `json:"group_id"`
	IsGroupAdmin      bool     `json:"is_group_admin"`
	AllGroupTenantIDs []string `json:"all_group_tenant_ids"` // All tenant IDs in the group for all_group permission
	Sort              string   `json:"sort"`
}

// GetDatasetsListRequest represents request for datasets list API (matches DatasetListApi)
type GetDatasetsListRequest struct {
	Page       int      `json:"page"`
	Limit      int      `json:"limit"`
	IDs        []string `json:"ids"`
	Keyword    *string  `json:"keyword"`
	TagIDs     []string `json:"tag_ids"`
	IncludeAll bool     `json:"include_all"`
	TenantID   string   `json:"tenant_id"`
	AccountID  string   `json:"account_id"`
}

// GetDatasetsListExRequest represents request for extended datasets list API (matches DatasetListApiEx)
type GetDatasetsListExRequest struct {
	Page       int      `json:"page"`
	Limit      int      `json:"limit"`
	Keyword    *string  `json:"keyword"`
	TagIDs     []string `json:"tag_ids"`
	IncludeAll bool     `json:"include_all"`
	GroupID    string   `json:"group_id" binding:"required"`
	AccountID  string   `json:"account_id"`
}

// GetDatasetsListResponse represents response for datasets list
type GetDatasetsListResponse struct {
	Data    []*model.Dataset `json:"data"`
	HasMore bool             `json:"has_more"`
	Limit   int              `json:"limit"`
	Total   int64            `json:"total"`
	Page    int              `json:"page"`
}

// GetDatasetsListExResponse represents response for extended datasets list
type GetDatasetsListExResponse struct {
	Data    []*model.Dataset `json:"data"`
	HasMore bool             `json:"has_more"`
	Limit   int              `json:"limit"`
	Total   int64            `json:"total"`
	Page    int              `json:"page"`
}

// IndexingEstimateRequest represents request for indexing estimate
type IndexingEstimateRequest struct {
	InfoList          map[string]interface{} `json:"info_list" binding:"required"`
	ProcessRule       map[string]interface{} `json:"process_rule" binding:"required"`
	IndexingTechnique string                 `json:"indexing_technique" binding:"required"`
	DocForm           string                 `json:"doc_form"`
	DatasetID         *string                `json:"dataset_id"`
	DocLanguage       string                 `json:"doc_language"`
	TenantID          string                 `json:"tenant_id"`
	AccountID         string                 `json:"account_id"`
}

// IndexingEstimateResponse represents response for indexing estimate
type IndexingEstimateResponse struct {
	TotalSegments int           `json:"total_segments"`
	Preview       []interface{} `json:"preview"`
	QAPreview     []interface{} `json:"qa_preview,omitempty"`
}

// CreateDatasetRequest Dataset creation request structure
type CreateDatasetRequest struct {
	WorkspaceID            string                 `json:"workspace_id" binding:"required"`
	Name                   string                 `json:"name" binding:"required"`
	Description            *string                `json:"description"`
	Provider               string                 `json:"provider"`
	Permission             *string                `json:"permission"`
	EmbeddingModel         *string                `json:"embedding_model"`
	EmbeddingModelProvider *string                `json:"embedding_model_provider"`
	EntityModel            *string                `json:"entity_model"`
	EntityModelProvider    *string                `json:"entity_model_provider"`
	RetrievalConfig        map[string]interface{} `json:"retrieval_config"`
	Icon                   *string                `json:"icon"`
	IconType               *string                `json:"icon_type"`
	IconBackground         *string                `json:"icon_background"`
	CreatedBy              string                 `json:"created_by" binding:"required"`
	EnableGraphFlow        bool                   `json:"enable_graph_flow"`
}

// UpdateDatasetRequest Dataset update request structure
type UpdateDatasetRequest struct {
	ID                     string                 `json:"id" binding:"required"`
	Name                   *string                `json:"name"`
	Description            *string                `json:"description"`
	EmbeddingModel         *string                `json:"embedding_model"`
	EmbeddingModelProvider *string                `json:"embedding_model_provider"`
	EntityModel            *string                `json:"entity_model"`
	EntityModelProvider    *string                `json:"entity_model_provider"`
	RetrievalConfig        map[string]interface{} `json:"retrieval_config"`
	Icon                   *string                `json:"icon"`
	IconType               *string                `json:"icon_type"`
	IconBackground         *string                `json:"icon_background"`
	WorkspaceID            *string                `json:"workspace_id"`
	UpdatedBy              string                 `json:"updated_by" binding:"required"`
	EnableGraphFlow        *bool                  `json:"enable_graph_flow"`
}

// UpdateDatasetExRequest represents request for updating dataset in ex API
type UpdateDatasetExRequest struct {
	UpdateDatasetRequest
	// WorkspaceID is inherited from UpdateDatasetRequest
}

type DatasetWithTenant struct {
	Dataset *model.Dataset `json:"dataset"`
	Tenant  interface{}    `json:"tenant"`
}

type DatasetWithPermission struct {
	Dataset    *model.Dataset `json:"dataset"`
	Permission string         `json:"permission"`
}

type DatasetWithStats struct {
	*model.Dataset
	DocumentCount          int64 `json:"document_count"`
	ChunkCount             int64 `json:"chunk_count"`
	AvailableDocumentCount int64 `json:"available_document_count"`
	AvailableSegmentCount  int64 `json:"available_segment_count"`
}

type datasetService struct {
	datasetRepo       repository.DatasetRepository
	documentRepo      repository.DocumentRepository
	chunkRepo         repository.ChunkRepository
	tenantSvc         interfaces.WorkspaceManagementService
	fileService       interfaces.FileService
	embeddingService  retrieval.Embedding
	vectorDB          vectordb.VectorDB
	defaultModelSvc   llmdefaultservice.DefaultModelService
	indexingRunner    *indexing.IndexingRunner
	db                *gorm.DB
	quotaService      interfaces.QuotaService
	enterpriseService interfaces.OrganizationService
}

func (s *datasetService) SetOrganizationService(organizationService interfaces.OrganizationService) {
	s.enterpriseService = organizationService
}

// NewDatasetService creates a new DatasetService.
// The llmClient should be obtained from the DI container (ServiceContainer.GetLLMClient()).
func NewDatasetService(
	datasetRepo repository.DatasetRepository,
	documentRepo repository.DocumentRepository,
	chunkRepo repository.ChunkRepository,
	tenantSvc interfaces.WorkspaceManagementService,
	fileService interfaces.FileService,
	embeddingService retrieval.Embedding,
	vectorDB vectordb.VectorDB,
	defaultModelSvc llmdefaultservice.DefaultModelService,
	storage storage.Storage,
	db *gorm.DB,
	quotaService interfaces.QuotaService,
	enterpriseService interfaces.OrganizationService,
	llmClient llmclient.LLMClient,
	taskManager *queue.TaskManager,
) DatasetService {
	// Initialize GraphFlow task repository
	graphFlowTaskRepo := graphflow_repo.NewGraphFlowTaskRepository(db)

	return &datasetService{
		datasetRepo:       datasetRepo,
		documentRepo:      documentRepo,
		chunkRepo:         chunkRepo,
		tenantSvc:         tenantSvc,
		fileService:       fileService,
		embeddingService:  embeddingService,
		vectorDB:          vectorDB,
		defaultModelSvc:   defaultModelSvc,
		indexingRunner:    indexing.NewIndexingRunner(storage, documentRepo, datasetRepo, fileService, embeddingService, vectorDB, defaultModelSvc, llmClient, graphFlowTaskRepo, taskManager),
		db:                db,
		quotaService:      quotaService,
		enterpriseService: enterpriseService,
	}
}

func (s *datasetService) CreateDataset(ctx context.Context, req *CreateDatasetRequest) (*model.Dataset, error) {
	// Step 1: Get groupID from tenantID for quota checking
	var groupID *uuid.UUID
	if s.enterpriseService == nil {
		return nil, fmt.Errorf("enterprise service not available")
	}

	group, err := s.enterpriseService.GetOrganizationByWorkspaceID(ctx, req.WorkspaceID)
	if err != nil {
		return nil, fmt.Errorf("failed to get group by tenant ID: %w", err)
	}
	if group == nil {
		return nil, fmt.Errorf("group not found for tenant ID: %s", req.WorkspaceID)
	}

	// Parse groupID string to UUID
	parsedGroupID, parseErr := uuid.Parse(group.ID)
	if parseErr != nil {
		return nil, fmt.Errorf("failed to parse group ID: %w", parseErr)
	}
	groupID = &parsedGroupID

	// temp remove knowledge base quota check, will fix later
	// Step 2: Check knowledge base quota (always required)
	// if s.quotaService == nil {
	// 	return nil, fmt.Errorf("quota service not available")
	// }

	// canProceed, currentUsage, limit, err := s.quotaService.CheckQuota(ctx, *groupID, "knowledge_bases", 1)
	// if err != nil {
	// 	return nil, fmt.Errorf("failed to check knowledge base quota: %w", err)
	// }

	// // Step 3: If quota exceeded, return error
	// if !canProceed {
	// 	return nil, fmt.Errorf("knowledge base quota exceeded. current: %d, limit: %d", currentUsage, limit)
	// }

	permission := string(model.DatasetPermissionAllTeam)
	if req.Permission != nil {
		permission = model.NormalizeDatasetPermission(*req.Permission)
	}
	if !model.IsValidDatasetCreatePermission(permission) {
		return nil, ErrInvalidDatasetPermission
	}

	// Convert request to model
	dataset := &model.Dataset{
		OrganizationID:         group.ID,
		WorkspaceID:            req.WorkspaceID,
		Name:                   req.Name,
		Description:            req.Description,
		Provider:               req.Provider,
		Permission:             permission,
		EmbeddingModel:         req.EmbeddingModel,
		EmbeddingModelProvider: req.EmbeddingModelProvider,
		EntityModel:            req.EntityModel,
		EntityModelProvider:    req.EntityModelProvider,
		RetrievalConfig:        req.RetrievalConfig,
		Icon:                   req.Icon,
		IconType:               req.IconType,
		IconBackground:         req.IconBackground,
		CreatedBy:              req.CreatedBy,
		EnableGraphFlow:        req.EnableGraphFlow,
	}

	// Set default retrieval_config if not provided
	if dataset.RetrievalConfig == nil {
		dataset.RetrievalConfig = map[string]interface{}{
			"search_method":           "semantic_search",
			"reranking_enable":        false,
			"reranking_model":         map[string]interface{}{"reranking_provider_name": "", "reranking_model_name": ""},
			"top_k":                   4,
			"score_threshold_enabled": false,
			"score_threshold":         0.5,
		}
	}

	// Set default process_rule
	if dataset.ProcessRule == nil {
		dataset.ProcessRule = map[string]interface{}{
			"mode": "hierarchical",
			"rules": map[string]interface{}{
				"pre_processing_rules": []map[string]interface{}{
					{"enabled": true, "id": "remove_extra_spaces"},
				},
				"segmentation": map[string]interface{}{
					"separator":     "\n\n",
					"max_tokens":    500,
					"chunk_overlap": 50,
				},
				"subchunk_segmentation": map[string]interface{}{
					"separator":  "\n",
					"max_tokens": 50,
				},
			},
		}
	}

	// Step 4: Create dataset and record usage in transaction
	txErr := s.db.Transaction(func(tx *gorm.DB) error {
		// Create dataset
		if err := tx.WithContext(ctx).Create(dataset).Error; err != nil {
			return fmt.Errorf("failed to create dataset: %w", err)
		}

		// Step 5: Record usage history (groupID is guaranteed to exist at this point)
		// Parse createdBy to UUID
		accountUUID, err := uuid.Parse(req.CreatedBy)
		if err != nil {
			return fmt.Errorf("failed to parse created by ID: %w", err)
		}

		// Parse tenantID to UUID
		tenantUUID, err := uuid.Parse(req.WorkspaceID)
		if err != nil {
			return fmt.Errorf("failed to parse tenant ID: %w", err)
		}

		// Create usage history record
		usageRecord := &quota_model.QuotaUsageHistory{
			ID:           uuid.New().String(),
			GroupID:      *groupID,
			AccountID:    accountUUID,
			TenantID:     &tenantUUID,
			ResourceType: "knowledge_bases",
			Delta:        1, // +1 for creating a knowledge base
			ResourceID:   &dataset.ID,
			ResourceName: &dataset.Name,
			Metadata: &quota_model.JSONMap{
				"dataset_id":   dataset.ID,
				"dataset_name": dataset.Name,
			},
		}

		// Add description to metadata if present
		if dataset.Description != nil {
			(*usageRecord.Metadata)["description"] = *dataset.Description
		}

		if err := s.quotaService.RecordUsageInTx(ctx, tx, usageRecord); err != nil {
			return fmt.Errorf("failed to record knowledge base usage: %w", err)
		}

		if dataset.ProcessRule != nil && len(dataset.ProcessRule) > 0 {
			defaultProcessRule := &model.DatasetProcessRule{
				ID:        uuid.New().String(),
				DatasetID: dataset.ID,
				Mode:      "custom",
				Rules:     model.JSONMap(dataset.ProcessRule),
				CreatedBy: req.CreatedBy,
				CreatedAt: time.Now(),
			}
			if mode, ok := dataset.ProcessRule["mode"].(string); ok {
				defaultProcessRule.Mode = mode
			}
			if err := tx.WithContext(ctx).Create(defaultProcessRule).Error; err != nil {
				return fmt.Errorf("failed to create process rule: %w", err)
			}
		} else {
			defaultProcessRule := &model.DatasetProcessRule{
				ID:        uuid.New().String(),
				DatasetID: dataset.ID,
				Mode:      "automatic",
				Rules: model.JSONMap{
					"mode": "automatic",
					"rules": []model.JSONMap{
						{"id": "remove_extra_spaces", "enabled": true},
						{"id": "remove_urls_emails", "enabled": false},
						{"id": "image_content_recognition", "enabled": false},
						{"id": "segment_content_auto_fill", "enabled": false},
						{"id": "formula_accuracy_enhance", "enabled": false},
						{"id": "generate_recommend_questions", "enabled": false},
					},
					"segmentation": model.JSONMap{
						"separator":     "\n\n",
						"max_tokens":    1024,
						"chunk_overlap": 0,
					},
					"parent_mode": "parent_child",
					"subchunk_segmentation": model.JSONMap{
						"separator":     "\n",
						"max_tokens":    512,
						"chunk_overlap": 0,
					},
				},
				CreatedBy: req.CreatedBy,
				CreatedAt: time.Now(),
			}
			if err := tx.WithContext(ctx).Create(defaultProcessRule).Error; err != nil {
				return fmt.Errorf("failed to create default process rule: %w", err)
			}
		}

		return nil
	})

	if txErr != nil {
		return nil, txErr
	}

	return dataset, nil
}

func (s *datasetService) GetDatasetByID(ctx context.Context, id string) (*model.Dataset, error) {
	dataset, err := s.datasetRepo.GetByID(ctx, id)
	if err != nil {
		return nil, err
	}

	return dataset, nil
}

func (s *datasetService) GetDatasetsByTenantID(ctx context.Context, tenantID string) ([]*model.Dataset, error) {
	return s.datasetRepo.GetByTenantID(ctx, tenantID)
}

func (s *datasetService) UpdateDataset(ctx context.Context, req *UpdateDatasetRequest) (*model.Dataset, error) {
	// Get existing dataset
	dataset, err := s.datasetRepo.GetByID(ctx, req.ID)
	if err != nil {
		return nil, fmt.Errorf("failed to get dataset: %w", err)
	}

	// Update fields if provided
	if req.Name != nil {
		dataset.Name = *req.Name
	}
	if req.Description != nil {
		dataset.Description = req.Description
	}
	if req.EmbeddingModel != nil {
		dataset.EmbeddingModel = req.EmbeddingModel
	}
	if req.EmbeddingModelProvider != nil {
		dataset.EmbeddingModelProvider = req.EmbeddingModelProvider
	}
	if req.EntityModel != nil {
		dataset.EntityModel = req.EntityModel
	}
	if req.EntityModelProvider != nil {
		dataset.EntityModelProvider = req.EntityModelProvider
	}
	if req.RetrievalConfig != nil {
		dataset.RetrievalConfig = req.RetrievalConfig
	}
	if req.Icon != nil {
		dataset.Icon = req.Icon
	}
	if req.IconType != nil {
		dataset.IconType = req.IconType
	}
	if req.IconBackground != nil {
		dataset.IconBackground = req.IconBackground
	}
	// Update tenant ID if provided
	if req.WorkspaceID != nil {
		dataset.WorkspaceID = *req.WorkspaceID
	}
	// Process EnableGraphFlow with validation
	if req.EnableGraphFlow != nil && *req.EnableGraphFlow != dataset.EnableGraphFlow {
		// Check if there are any documents in the dataset
		docCount, err := s.documentRepo.GetDocumentCount(ctx, dataset.ID)
		if err != nil {
			return nil, fmt.Errorf("failed to check document count for graph flow validation: %w", err)
		}

		if docCount > 0 {
			return nil, fmt.Errorf("cannot modify knowledge graph setting: dataset already contains %d document(s)", docCount)
		}

		dataset.EnableGraphFlow = *req.EnableGraphFlow
	}
	dataset.UpdatedBy = &req.UpdatedBy

	// Process rule update removed - now using dataset-level process_rule field only

	err = s.datasetRepo.Update(ctx, dataset)
	if err != nil {
		return nil, fmt.Errorf("failed to update dataset: %w", err)
	}

	return dataset, nil
}

func (s *datasetService) DeleteDataset(ctx context.Context, datasetID, accountID, tenantID string) error {
	hasPermission, err := s.CheckEditorPermission(ctx, datasetID, accountID, tenantID)
	if err != nil {
		return fmt.Errorf("failed to check permission: %w", err)
	}
	if !hasPermission {
		return ErrDatasetAccessDenied
	}

	// Step 1: Get dataset information before deletion (for recording)
	dataset, err := s.datasetRepo.GetByID(ctx, datasetID)
	if err != nil {
		return fmt.Errorf("failed to get dataset: %w", err)
	}

	// Step 2: Get groupID from dataset.WorkspaceID for quota recording
	var groupID *uuid.UUID
	if s.enterpriseService != nil {
		group, err := s.enterpriseService.GetOrganizationByWorkspaceID(ctx, dataset.WorkspaceID)
		if err != nil {
			return fmt.Errorf("failed to get organization by workspace ID: %w", err)
		}
		if group == nil {
			return fmt.Errorf("organization not found for workspace ID: %s", dataset.WorkspaceID)
		}
		// Parse groupID string to UUID
		parsedGroupID, parseErr := uuid.Parse(group.ID)
		if parseErr != nil {
			return fmt.Errorf("failed to parse organization ID: %w", parseErr)
		}
		groupID = &parsedGroupID
	}

	// Step 3: Delete dataset and record usage decrease in transaction
	return s.db.Transaction(func(tx *gorm.DB) error {
		now := time.Now()

		if err := tx.WithContext(ctx).
			Table("data_library_knowledge_base_asset_refs").
			Where("dataset_id = ? AND deleted_at IS NULL", datasetID).
			Update("deleted_at", now).Error; err != nil {
			return fmt.Errorf("failed to delete dataset file asset refs: %w", err)
		}

		if err := tx.WithContext(ctx).
			Where("dataset_id = ?", datasetID).
			Delete(&model.ChildChunk{}).Error; err != nil {
			return fmt.Errorf("failed to delete dataset child chunks: %w", err)
		}

		if err := tx.WithContext(ctx).
			Where("dataset_id = ?", datasetID).
			Delete(&model.DocumentSegmentQuestion{}).Error; err != nil {
			return fmt.Errorf("failed to delete dataset segment questions: %w", err)
		}

		segmentUpdates := map[string]interface{}{
			"is_deleted":            true,
			"deleted_at":            &now,
			"graph_indexing_status": "deleted",
			"status":                "deleted",
		}
		if err := tx.WithContext(ctx).
			Model(&model.DocumentSegment{}).
			Where("dataset_id = ? AND deleted_at IS NULL", datasetID).
			Updates(segmentUpdates).Error; err != nil {
			return fmt.Errorf("failed to delete dataset document segments: %w", err)
		}

		if err := tx.WithContext(ctx).
			Where("dataset_id = ?", datasetID).
			Delete(&model.Document{}).Error; err != nil {
			return fmt.Errorf("failed to delete dataset documents: %w", err)
		}

		// Delete dataset
		if err := tx.WithContext(ctx).Delete(&model.Dataset{}, "id = ?", datasetID).Error; err != nil {
			return fmt.Errorf("failed to delete dataset: %w", err)
		}

		// Step 4: Record usage history if groupID exists
		if groupID != nil && s.quotaService != nil {
			// Parse accountID to UUID
			accountUUID, err := uuid.Parse(accountID)
			if err != nil {
				return fmt.Errorf("failed to parse account ID: %w", err)
			}

			// Parse tenantID to UUID (using dataset.WorkspaceID)
			tenantUUID, err := uuid.Parse(dataset.WorkspaceID)
			if err != nil {
				return fmt.Errorf("failed to parse tenant ID: %w", err)
			}

			// Create usage history record with negative delta
			usageRecord := &quota_model.QuotaUsageHistory{
				ID:           uuid.New().String(),
				GroupID:      *groupID,
				AccountID:    accountUUID,
				TenantID:     &tenantUUID,
				ResourceType: "knowledge_bases",
				Delta:        -1, // -1 for deleting a knowledge base
				ResourceID:   &dataset.ID,
				ResourceName: &dataset.Name,
				Metadata: &quota_model.JSONMap{
					"dataset_id":   dataset.ID,
					"dataset_name": dataset.Name,
					"action":       "deleted",
				},
			}

			if err := s.quotaService.RecordUsageInTx(ctx, tx, usageRecord); err != nil {
				return fmt.Errorf("failed to record knowledge base usage decrease: %w", err)
			}
		}

		return nil
	})
}

func (s *datasetService) GetDatasetCount(ctx context.Context, tenantID string) (int64, error) {
	return s.datasetRepo.CountByTenantID(ctx, tenantID)
}

func (s *datasetService) GetDatasetsByIDs(ctx context.Context, ids []string) ([]*model.Dataset, error) {
	return s.datasetRepo.GetByIDs(ctx, ids)
}

func (s *datasetService) GetPaginateDatasetsByTenantIDs(ctx context.Context, req *GetPaginateDatasetsByTenantIDsRequest) (*model.DatasetPaginationResponse, error) {
	// Parameter validation
	// if len(req.TenantIDs) == 0 {
	// 	return nil, fmt.Errorf("tenant_ids cannot be empty")
	// }

	// Set default values
	if req.Page <= 0 {
		req.Page = 1
	}
	if req.Limit <= 0 {
		req.Limit = 20
	}
	if req.Sort == "" || (req.Sort != "asc" && req.Sort != "desc") {
		req.Sort = "desc"
	}

	search := ""
	if req.Search != nil {
		search = *req.Search
	}

	// Get datasets with pagination and permission filtering
	datasets, total, err := s.datasetRepo.GetPaginatedByTenantIDsWithPermissions(
		ctx,
		req.TenantIDs,
		req.AccountID,
		req.IsGroupAdmin,
		req.AllGroupTenantIDs,
		req.Page,
		req.Limit,
		search,
		req.Sort,
	)
	if err != nil {
		return nil, fmt.Errorf("failed to get paginated datasets: %w", err)
	}

	return &model.DatasetPaginationResponse{
		Data:    datasets,
		Page:    req.Page,
		Limit:   req.Limit,
		Total:   total,
		HasMore: int64(req.Page*req.Limit) < total,
		Search:  search,
	}, nil
}

func (s *datasetService) GetDatasetWithTenantInfo(ctx context.Context, datasetID string) (*DatasetWithTenant, error) {
	dataset, err := s.datasetRepo.GetByID(ctx, datasetID)
	if err != nil {
		return nil, fmt.Errorf("failed to get dataset: %w", err)
	}

	tenant, err := s.tenantSvc.GetWorkspaceByID(ctx, dataset.WorkspaceID)
	if err != nil {
		return nil, fmt.Errorf("failed to get tenant: %w", err)
	}

	return &DatasetWithTenant{
		Dataset: dataset,
		Tenant:  tenant,
	}, nil
}

func (s *datasetService) GetDatasetsWithPermissions(ctx context.Context, accountID string) ([]*DatasetWithPermission, error) {
	tenants, err := s.tenantSvc.GetAccountWorkspaces(ctx, accountID)
	if err != nil {
		return nil, fmt.Errorf("failed to get tenants: %w", err)
	}

	var result []*DatasetWithPermission

	for _, tenant := range tenants {
		datasets, err := s.datasetRepo.GetByTenantID(ctx, tenant.ID)
		if err != nil {
			continue
		}

		for _, dataset := range datasets {
			result = append(result, &DatasetWithPermission{
				Dataset:    dataset,
				Permission: "read",
			})
		}
	}

	return result, nil
}

func (s *datasetService) GetDatasetsByAccountAndTenant(ctx context.Context, accountID, tenantID string) ([]*model.Dataset, error) {
	if !s.tenantSvc.CheckPermission(ctx, tenantID, accountID) {
		return nil, fmt.Errorf("account %s has no access to tenant %s", accountID, tenantID)
	}

	return s.datasetRepo.GetByTenantID(ctx, tenantID)
}

func (s *datasetService) GetDatasetWithDocumentStats(ctx context.Context, datasetID string) (*DatasetWithStats, error) {
	dataset, err := s.datasetRepo.GetByID(ctx, datasetID)
	if err != nil {
		return nil, fmt.Errorf("failed to get dataset: %w", err)
	}

	documentCount, err := s.documentRepo.GetDocumentCount(ctx, datasetID)
	if err != nil {
		return nil, fmt.Errorf("failed to count documents: %w", err)
	}

	availableDocumentCount, err := s.documentRepo.GetAvailableDocumentCount(ctx, datasetID)
	if err != nil {
		return nil, fmt.Errorf("failed to count available documents: %w", err)
	}

	availableSegmentCount, err := s.documentRepo.GetSegmentCount(ctx, datasetID)
	if err != nil {
		return nil, fmt.Errorf("failed to count available segments: %w", err)
	}

	documents, err := s.documentRepo.GetByDatasetIDSimple(ctx, datasetID, 0, 1000)
	if err != nil {
		return nil, fmt.Errorf("failed to get documents: %w", err)
	}

	var totalChunkCount int64
	var totalWordCount int
	for _, doc := range documents {
		chunkCount, err := s.chunkRepo.CountByDocumentID(ctx, doc.ID)
		if err != nil {
			continue
		}
		totalChunkCount += chunkCount

		if doc.WordCount != nil {
			totalWordCount += *doc.WordCount
		}
	}

	dataset.DocumentCount = int(documentCount)
	dataset.WordCount = totalWordCount
	dataset.AvailableSegmentCount = int(availableSegmentCount)
	dataset.AvailableDocumentCount = int(availableDocumentCount)

	return &DatasetWithStats{
		Dataset:                dataset,
		DocumentCount:          documentCount,
		ChunkCount:             totalChunkCount,
		AvailableDocumentCount: availableDocumentCount,
		AvailableSegmentCount:  availableSegmentCount,
	}, nil
}

func (s *datasetService) GetDatasetDocuments(ctx context.Context, datasetID string, offset, limit int) ([]*model.Document, error) {
	return s.documentRepo.GetByDatasetIDSimple(ctx, datasetID, offset, limit)
}

func (s *datasetService) CheckDatasetPermission(ctx context.Context, datasetID, accountID, tenantID string) (bool, error) {
	dataset, err := s.datasetRepo.GetByID(ctx, datasetID)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return false, nil
		}
		return false, fmt.Errorf("failed to get dataset: %w", err)
	}

	return s.canReadDataset(ctx, dataset, accountID)
}

// GetDatasetsList Get datasets list (matches DatasetListApi.get)
func (s *datasetService) GetDatasetsList(ctx context.Context, req *GetDatasetsListRequest) (*GetDatasetsListResponse, error) {
	// Set default values
	if req.Page <= 0 {
		req.Page = 1
	}
	if req.Limit <= 0 {
		req.Limit = 20
	}

	// Convert IDs to get specific datasets if provided
	if len(req.IDs) > 0 {
		// Filter out empty IDs to avoid UUID format errors
		var validIDs []string
		for _, id := range req.IDs {
			if strings.TrimSpace(id) != "" {
				validIDs = append(validIDs, id)
			}
		}

		// If no valid IDs after filtering, proceed with normal query
		if len(validIDs) > 0 {
			datasets, err := s.datasetRepo.GetByIDs(ctx, validIDs)
			if err != nil {
				return nil, fmt.Errorf("failed to get datasets by IDs: %w", err)
			}
			return &GetDatasetsListResponse{
				Data:    datasets,
				HasMore: false,
				Limit:   req.Limit,
				Total:   int64(len(datasets)),
				Page:    req.Page,
			}, nil
		}
	}

	// Get datasets by tenant with search and pagination
	search := ""
	if req.Keyword != nil {
		search = *req.Keyword
	}
	datasets, total, err := s.datasetRepo.GetPaginatedByTenantIDs(
		ctx,
		[]string{req.TenantID},
		req.Page,
		req.Limit,
		search,
		"",
	)
	if err != nil {
		return nil, fmt.Errorf("failed to get datasets: %w", err)
	}

	return &GetDatasetsListResponse{
		Data:    datasets,
		HasMore: int64(req.Page*req.Limit) < total,
		Limit:   req.Limit,
		Total:   total,
		Page:    req.Page,
	}, nil
}

// GetDatasetsListEx Get extended datasets list (matches DatasetListApiEx.get)
func (s *datasetService) GetDatasetsListEx(ctx context.Context, req *GetDatasetsListExRequest) (*GetDatasetsListExResponse, error) {
	// Set default values
	if req.Page <= 0 {
		req.Page = 1
	}
	if req.Limit <= 0 {
		req.Limit = 20
	}

	// This is a simplified implementation
	// In the real implementation, you would check enterprise group permissions
	// and get datasets from multiple tenants within the group
	search := ""
	if req.Keyword != nil {
		search = *req.Keyword
	}
	datasets, total, err := s.datasetRepo.GetPaginatedByTenantIDs(
		ctx,
		[]string{req.GroupID}, // Using GroupID as tenant for now
		req.Page,
		req.Limit,
		search,
		"",
	)
	if err != nil {
		return nil, fmt.Errorf("failed to get datasets ex: %w", err)
	}

	return &GetDatasetsListExResponse{
		Data:    datasets,
		HasMore: int64(req.Page*req.Limit) < total,
		Limit:   req.Limit,
		Total:   total,
		Page:    req.Page,
	}, nil
}

func (s *datasetService) GetDatasetWithPermissionCheck(ctx context.Context, datasetID, accountID, workspaceID string) (*model.Dataset, error) {
	dataset, err := s.datasetRepo.GetByID(ctx, datasetID)
	if err != nil {
		return nil, fmt.Errorf("failed to get dataset: %w", err)
	}

	hasPermission, err := s.canReadDataset(ctx, dataset, accountID)
	if err != nil {
		return nil, fmt.Errorf("failed to check dataset permission: %w", err)
	}
	if !hasPermission {
		return nil, ErrDatasetAccessDenied
	}

	return dataset, nil
}

func (s *datasetService) canReadDataset(ctx context.Context, dataset *model.Dataset, accountID string) (bool, error) {
	if dataset == nil {
		return false, nil
	}
	if dataset.CreatedBy == accountID {
		return true, nil
	}
	if s.tenantSvc != nil && s.tenantSvc.CheckPermission(ctx, dataset.WorkspaceID, accountID) {
		return true, nil
	}
	if !model.IsDatasetWorkspaceVisiblePermission(dataset.Permission) {
		return false, nil
	}
	if s.enterpriseService == nil {
		return false, nil
	}

	return s.enterpriseService.CheckWorkspaceOrganizationAnyPermission(
		ctx,
		dataset.OrganizationID,
		dataset.WorkspaceID,
		accountID,
		workspace_model.WorkspacePermissionKnowledgeBaseView,
		workspace_model.WorkspacePermissionKnowledgeBaseManage,
		workspace_model.WorkspacePermissionKnowledgeBaseFolderManage,
	)
}

func (s *datasetService) canEditDataset(ctx context.Context, dataset *model.Dataset, accountID string) bool {
	if dataset == nil {
		return false
	}
	if dataset.CreatedBy == accountID {
		return true
	}
	return s.tenantSvc != nil && s.tenantSvc.CheckPermission(ctx, dataset.WorkspaceID, accountID)
}

// CheckEditorPermission checks if user has editor permission for dataset
func (s *datasetService) CheckEditorPermission(ctx context.Context, datasetID, accountID, tenantID string) (bool, error) {
	dataset, err := s.datasetRepo.GetByID(ctx, datasetID)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return false, nil
		}
		return false, fmt.Errorf("failed to get dataset: %w", err)
	}

	return s.canEditDataset(ctx, dataset, accountID), nil
}

// EstimateIndexing estimates indexing requirements
func (s *datasetService) EstimateIndexing(ctx context.Context, req *IndexingEstimateRequest) (*IndexingEstimateResponse, error) {
	if req.InfoList == nil || req.ProcessRule == nil || req.IndexingTechnique == "" {
		err := fmt.Errorf("missing required parameters")
		return nil, err
	}

	dsType, ok := req.InfoList["data_source_type"].(string)
	if !ok {
		err := fmt.Errorf("missing data_source_type in info_list")
		return nil, err
	}

	if req.DocForm == "" {
		req.DocForm = "text_model"
	}
	var extractSettings []indexing.ExtractSetting

	switch dsType {
	case "upload_file":
		fileInfoList, ok := req.InfoList["file_info_list"].(map[string]interface{})
		if !ok {
			err := fmt.Errorf("missing file_info_list for upload_file")
			return nil, err
		}

		fileIDs, ok := fileInfoList["file_ids"].([]interface{})
		if !ok {
			err := fmt.Errorf("file_ids must be a list")
			return nil, err
		}

		for _, fileIDRaw := range fileIDs {
			fileID, ok := fileIDRaw.(string)
			if !ok {
				err := fmt.Errorf("file_id is not a string")
				return nil, err
			}

			// get dtoFile FileService
			dtoFile, err := s.fileService.GetFileByID(ctx, fileID)
			if err != nil {
				return nil, fmt.Errorf("failed to get file info for %s: %w", fileID, err)
			}

			content := "Sample content for file: " + dtoFile.Name

			var qaPreviewDetails []dto.QAPreviewDetail
			var previewDetails []dto.PreviewDetail
			const maxPreview = 5
			totalSegments := 0

			switch req.DocForm {
			case "qa_model":
				qaPairs := s.generateQAPairs(content)
				totalSegments += len(qaPairs) * 20
				if len(qaPreviewDetails) < maxPreview {
					remainingSlots := maxPreview - len(qaPreviewDetails)
					if len(qaPairs) > remainingSlots {
						qaPairs = qaPairs[:remainingSlots]
					}
					qaPreviewDetails = append(qaPreviewDetails, qaPairs...)
				}
			case "hierarchical_model":
				segments := s.generateHierarchicalSegments(content)
				totalSegments += len(segments)
				if len(previewDetails) < maxPreview {
					remainingSlots := maxPreview - len(previewDetails)
					if len(segments) > remainingSlots {
						segments = segments[:remainingSlots]
					}
					previewDetails = append(previewDetails, segments...)
				}
			default: // text_model
				segments := s.generateTextSegments(content)
				totalSegments += len(segments)
				if len(previewDetails) < maxPreview {
					remainingSlots := maxPreview - len(previewDetails)
					if len(segments) > remainingSlots {
						segments = segments[:remainingSlots]
					}
					previewDetails = append(previewDetails, segments...)
				}
			}

			uploadFile := &indexing.UploadFile{
				ID:       dtoFile.ID,
				Name:     dtoFile.Name,
				Size:     dtoFile.Size,
				FilePath: dtoFile.Key,
			}

			// ExtractSetting
			extractSetting := indexing.ExtractSetting{
				DataSourceType: "upload_file",
				DocumentModel:  req.DocForm,
				UploadFile:     uploadFile,
			}
			extractSettings = append(extractSettings, extractSetting)
		}

	case "notion_import":
		notionInfoList, ok := req.InfoList["notion_info_list"].([]interface{})
		if !ok {
			err := fmt.Errorf("missing notion_info_list for notion_import")
			return nil, err
		}

		// Create an ExtractSetting for each Notion page
		for _, notionInfo := range notionInfoList {
			notionInfoMap, ok := notionInfo.(map[string]interface{})
			if !ok {
				err := fmt.Errorf("notion_info must be a map")
				return nil, err
			}

			extractSetting := indexing.ExtractSetting{
				DataSourceType: "notion_import",
				DocumentModel:  req.DocForm,
				NotionInfo:     notionInfoMap,
			}
			extractSettings = append(extractSettings, extractSetting)
		}

	case "website_crawl":
		// TODO: implement handling for the website_crawl data source type
		// Simulate creating an ExtractSetting
		extractSetting := indexing.ExtractSetting{
			DataSourceType: "website_crawl",
			DocumentModel:  req.DocForm,
			WebsiteInfo:    req.InfoList, // simplified handling
		}
		extractSettings = append(extractSettings, extractSetting)

	default:
		err := fmt.Errorf("unsupported data_source_type: %s", dsType)
		return nil, err
	}

	indexingReq := &indexing.IndexingEstimateRequest{
		TenantID:          req.TenantID,
		ExtractSettings:   extractSettings,
		TmpProcessingRule: req.ProcessRule,
		DocForm:           req.DocForm,
		DocLanguage:       req.DocLanguage,
		DatasetID:         req.DatasetID,
		IndexingTechnique: req.IndexingTechnique,
	}

	// IndexingRunner Estimate
	result, err := s.indexingRunner.Estimate(ctx, indexingReq)
	if err != nil {
		return nil, fmt.Errorf("failed to estimate indexing: %w", err)
	}

	response := &IndexingEstimateResponse{
		TotalSegments: result.TotalSegments,
	}

	if req.DocForm == "qa_model" {
		qaPreview := make([]interface{}, len(result.QAPreview))
		for i, detail := range result.QAPreview {
			qaPreview[i] = detail
		}
		response.QAPreview = qaPreview
		response.Preview = []interface{}{}
	} else {
		preview := make([]interface{}, len(result.Preview))
		for i, detail := range result.Preview {
			preview[i] = detail
		}
		response.Preview = preview
		response.QAPreview = []interface{}{}
	}

	return response, nil
}

func (s *datasetService) generateQAPairs(content string) []dto.QAPreviewDetail {
	paragraphs := strings.Split(content, "\n\n")
	var qaPairs []dto.QAPreviewDetail

	for _, paragraph := range paragraphs {
		paragraph = strings.TrimSpace(paragraph)
		if paragraph == "" {
			continue
		}

		question := "What is the main topic of this paragraph?"
		answer := paragraph

		qaPairs = append(qaPairs, dto.QAPreviewDetail{
			Question: question,
			Answer:   answer,
		})
	}

	return qaPairs
}

func (s *datasetService) generateHierarchicalSegments(content string) []dto.PreviewDetail {
	paragraphs := strings.Split(content, "\n\n")
	var segments []dto.PreviewDetail

	for _, paragraph := range paragraphs {
		paragraph = strings.TrimSpace(paragraph)
		if paragraph == "" {
			continue
		}

		sentences := strings.Split(paragraph, ". ")
		var childChunks []string
		for _, sentence := range sentences {
			sentence = strings.TrimSpace(sentence)
			if sentence != "" {
				childChunks = append(childChunks, sentence)
			}
		}

		segments = append(segments, dto.PreviewDetail{
			Content:     paragraph,
			ChildChunks: childChunks,
		})
	}

	return segments
}

func (s *datasetService) generateTextSegments(content string) []dto.PreviewDetail {
	paragraphs := strings.Split(content, "\n\n")
	var segments []dto.PreviewDetail

	for _, paragraph := range paragraphs {
		paragraph = strings.TrimSpace(paragraph)
		if paragraph == "" {
			continue
		}

		segments = append(segments, dto.PreviewDetail{
			Content:     paragraph,
			ChildChunks: nil,
		})
	}

	return segments
}

// GetDatasetAppDefault gets default app for dataset
func (s *datasetService) GetDatasetAppDefault(ctx context.Context, datasetID, accountID, tenantID string) (map[string]interface{}, error) {
	// Check permission first
	hasPermission, err := s.CheckDatasetPermission(ctx, datasetID, accountID, tenantID)
	if err != nil {
		return nil, fmt.Errorf("failed to check permission: %w", err)
	}
	if !hasPermission {
		return nil, ErrDatasetAccessDenied
	}

	// Return empty for now - this would typically return default app configuration
	return map[string]interface{}{}, nil
}

// UpdateDatasetEx updates dataset with extended parameters
func (s *datasetService) UpdateDatasetEx(ctx context.Context, req *UpdateDatasetExRequest) (*model.Dataset, error) {
	// Convert to regular UpdateDatasetRequest
	updateReq := &UpdateDatasetRequest{
		ID:                     req.ID,
		Name:                   req.Name,
		Description:            req.Description,
		EmbeddingModel:         req.EmbeddingModel,
		EmbeddingModelProvider: req.EmbeddingModelProvider,
		RetrievalConfig:        req.RetrievalConfig,
		Icon:                   req.Icon,
		IconBackground:         req.IconBackground,
		UpdatedBy:              req.UpdatedBy,
	}

	return s.UpdateDataset(ctx, updateReq)
}

// UpdateSegmentsStatus updates the status of multiple segments
func (s *datasetService) UpdateSegmentsStatus(ctx context.Context, segmentIDs []string, action string, dataset *model.Dataset, document *model.Document) error {
	// Validate action
	validActions := map[string]bool{
		"enable":  true,
		"disable": true,
	}

	if !validActions[action] {
		return fmt.Errorf("invalid action: %s", action)
	}

	// TODO:
	// document_indexing_cache_key = "document_{}_indexing".format(document.id)
	// cache_result = redis_client.get(document_indexing_cache_key)
	// if cache_result is not None:
	//     raise InvalidActionError("Document is being indexed, please try again later")

	// Update segments based on action
	enabled := action == "enable"

	// Use chunk service to update segments
	for _, segmentID := range segmentIDs {
		segment, err := s.chunkRepo.GetByID(ctx, segmentID)
		if err != nil {
			// Skip segments that don't exist
			continue
		}

		// Check if segment belongs to the document
		if segment.DocumentID != document.ID {
			continue
		}

		// Update enabled status
		segment.Enabled = enabled

		// Set disabled timestamp and user if disabling
		if !enabled {
			now := time.Now()
			segment.DisabledAt = &now
			// TODO: set the disabled user ID
			// segment.DisabledBy = &currentUserID
		} else {
			// Clear disabled timestamp and user if enabling
			segment.DisabledAt = nil
			segment.DisabledBy = nil
		}

		// Save updated segment
		if err := s.chunkRepo.Update(ctx, segment); err != nil {
			return fmt.Errorf("failed to update segment %s: %w", segmentID, err)
		}
	}

	return nil
}
