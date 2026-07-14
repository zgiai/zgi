package service

import (
	"context"
	"errors"
	"strings"
	"time"

	"github.com/google/uuid"
	datalibModel "github.com/zgiai/zgi/api/internal/modules/datalibrary/model"
	datalibRepo "github.com/zgiai/zgi/api/internal/modules/datalibrary/repository"
	datasetModel "github.com/zgiai/zgi/api/internal/modules/dataset/model"
	fileModel "github.com/zgiai/zgi/api/internal/modules/file_process/model"
)

const (
	FileCandidateFilterAddable = "addable"
	FileCandidateFilterAdded   = "added"
	FileCandidateFilterAll     = "all"

	FileCandidateReasonNotReady                = "not_ready"
	FileCandidateReasonAlreadyAdded            = "already_added"
	FileCandidateReasonEmbeddingModelMismatch  = "embedding_model_mismatch"
	FileCandidateReasonMissingChunks           = "missing_chunks"
	FileCandidateReasonMissingEmbedding        = "missing_embedding"
	FileCandidateReasonMissingDatasetEmbedding = "missing_dataset_embedding"
	FileCandidateReasonDatasetModelMissing     = "dataset_embedding_model_missing"
)

type knowledgeBaseFileAssetReader interface {
	GetAssetByID(ctx context.Context, id uuid.UUID) (*datalibModel.DocumentAsset, error)
	ListAssets(ctx context.Context, filter datalibRepo.DocumentAssetListFilter) ([]*datalibModel.DocumentAsset, int64, error)
}

type knowledgeBaseFileChunkReader interface {
	List(ctx context.Context, filter datalibRepo.DocumentChunkListFilter) ([]*datalibModel.DocumentChunk, int64, error)
	CountByAssetGenerationAndTypes(ctx context.Context, organizationID string, assetID uuid.UUID, generationNo int64, chunkTypes []string) (int64, error)
}

type knowledgeBaseFileCandidateChunkCounter interface {
	CountByAssetGenerationAndTypesFiltered(ctx context.Context, organizationID string, assetID uuid.UUID, generationNo int64, chunkTypes []string, enabled *bool, status string) (int64, error)
}

type knowledgeBaseFileEmbeddingReader interface {
	CountReadyByAssetGeneration(ctx context.Context, organizationID string, assetID uuid.UUID, generationNo int64) (int64, error)
	CountReadyByAssetGenerationModel(ctx context.Context, organizationID string, assetID uuid.UUID, generationNo int64, provider string, embeddingModel string) (int64, error)
}

type knowledgeBaseFileRefStore interface {
	Create(ctx context.Context, item *datalibModel.KnowledgeBaseAssetRef) error
	GetByID(ctx context.Context, id uuid.UUID) (*datalibModel.KnowledgeBaseAssetRef, error)
	FindActiveByAsset(ctx context.Context, organizationID string, datasetID string, assetID uuid.UUID) (*datalibModel.KnowledgeBaseAssetRef, error)
	List(ctx context.Context, filter datalibRepo.KnowledgeBaseAssetRefListFilter) ([]*datalibModel.KnowledgeBaseAssetRef, int64, error)
	CountActiveByAssetID(ctx context.Context, organizationID string, assetID uuid.UUID) (int64, error)
	MarkPending(ctx context.Context, organizationID string, id uuid.UUID, syncRunID uuid.UUID, errorCode, errorMessage *string) (*datalibModel.KnowledgeBaseAssetRef, error)
	MarkFailed(ctx context.Context, organizationID string, id uuid.UUID, syncRunID uuid.UUID, errorCode, errorMessage string) (*datalibModel.KnowledgeBaseAssetRef, error)
	SoftDelete(ctx context.Context, organizationID string, id uuid.UUID) (*datalibModel.KnowledgeBaseAssetRef, error)
}

type knowledgeBaseFileFileReader interface {
	ListByTenantAndIDs(ctx context.Context, tenantID string, ids []string) (map[string]*fileModel.UploadFile, error)
}

type knowledgeBaseFileDatasetReader interface {
	GetByID(ctx context.Context, id string) (*datasetModel.Dataset, error)
}

type knowledgeBaseFileDocumentReader interface {
	GetDocumentsByIDs(ctx context.Context, ids []string) ([]*datasetModel.Document, error)
	GetSegmentCounts(ctx context.Context, documentID string) (completed int, total int, err error)
}

type knowledgeBaseFileProcessingProgressUpdater interface {
	UpdateRequestExecutionMetadata(ctx context.Context, organizationID string, id uuid.UUID, metadata map[string]any) (*ProcessingRequestView, error)
}

type KnowledgeBaseFileRefService interface {
	ListCandidates(ctx context.Context, req KnowledgeBaseFileCandidateRequest) (*KnowledgeBaseFileCandidateResult, error)
	ListRefs(ctx context.Context, req KnowledgeBaseFileRefListRequest) (*KnowledgeBaseFileRefListResult, error)
	CreateRefs(ctx context.Context, req KnowledgeBaseFileRefCreateRequest) (*KnowledgeBaseFileRefCreateResult, error)
	GenerateCandidateEmbeddings(ctx context.Context, req KnowledgeBaseFileCandidateEmbeddingRequest) (*KnowledgeBaseFileCandidateEmbeddingResult, error)
	GetRef(ctx context.Context, req KnowledgeBaseFileRefGetRequest) (*KnowledgeBaseAssetRefView, error)
	RetryRef(ctx context.Context, req KnowledgeBaseFileRefRetryRequest) (*KnowledgeBaseFileRefCreateItem, error)
	MarkRefSyncFailed(ctx context.Context, req KnowledgeBaseFileRefSyncFailureRequest) (*KnowledgeBaseAssetRefView, error)
	RemoveRef(ctx context.Context, req KnowledgeBaseFileRefGetRequest) (*KnowledgeBaseAssetRefView, error)
}

type KnowledgeBaseFileCandidateRequest struct {
	OrganizationID string
	WorkspaceID    *string
	DatasetID      string
	Filter         string
	Keyword        string
	Limit          int
	Offset         int
}

type KnowledgeBaseFileCandidateResult struct {
	Items []*KnowledgeBaseFileCandidate `json:"items"`
	Total int64                         `json:"total"`
}

type KnowledgeBaseFileCandidate struct {
	FileID                      string    `json:"file_id"`
	AssetID                     uuid.UUID `json:"asset_id"`
	Name                        string    `json:"name"`
	FileExtension               string    `json:"file_extension,omitempty"`
	FileSize                    *int64    `json:"file_size,omitempty"`
	UpdatedAt                   time.Time `json:"updated_at"`
	ProcessingStatus            string    `json:"processing_status"`
	GenerationNo                int64     `json:"generation_no"`
	Addable                     bool      `json:"addable"`
	Reason                      string    `json:"reason,omitempty"`
	EmbeddingProvider           *string   `json:"embedding_provider,omitempty"`
	EmbeddingModel              *string   `json:"embedding_model,omitempty"`
	TargetEmbeddingProvider     string    `json:"target_embedding_provider,omitempty"`
	TargetEmbeddingModel        string    `json:"target_embedding_model,omitempty"`
	AlreadyAdded                bool      `json:"already_added"`
	ReferenceCount              int64     `json:"reference_count"`
	ChunkCount                  int64     `json:"chunk_count"`
	EmbeddingCount              int64     `json:"embedding_count"`
	TargetEmbeddingCount        int64     `json:"target_embedding_count"`
	RequiresEmbeddingGeneration bool      `json:"requires_embedding_generation"`
}

type KnowledgeBaseFileRefCreateRequest struct {
	OrganizationID string
	WorkspaceID    *string
	DatasetID      string
	AssetIDs       []uuid.UUID
	CreatedBy      string
}

type KnowledgeBaseFileCandidateEmbeddingRequest struct {
	OrganizationID      string
	WorkspaceID         *string
	DatasetID           string
	AssetID             uuid.UUID
	RequestedBy         string
	ProcessingRequestID uuid.UUID
}

type KnowledgeBaseFileCandidateEmbeddingResult struct {
	AssetID              uuid.UUID              `json:"asset_id"`
	Accepted             bool                   `json:"accepted,omitempty"`
	ProcessingRequest    *ProcessingRequestView `json:"processing_request,omitempty"`
	GenerationNo         int64                  `json:"generation_no"`
	EmbeddingProvider    string                 `json:"embedding_provider,omitempty"`
	EmbeddingModel       string                 `json:"embedding_model,omitempty"`
	EmbeddingCount       int64                  `json:"embedding_count"`
	TargetEmbeddingCount int64                  `json:"target_embedding_count"`
	ChunkCount           int64                  `json:"chunk_count"`
	Addable              bool                   `json:"addable"`
	Reason               string                 `json:"reason,omitempty"`
}

type KnowledgeBaseFileRefListRequest struct {
	OrganizationID string
	WorkspaceID    *string
	DatasetID      string
	SyncStatus     string
	Limit          int
	Offset         int
}

type KnowledgeBaseFileRefRetryRequest struct {
	OrganizationID string
	WorkspaceID    *string
	DatasetID      string
	RefID          uuid.UUID
}

type KnowledgeBaseFileRefGetRequest struct {
	OrganizationID string
	WorkspaceID    *string
	DatasetID      string
	RefID          uuid.UUID
}

type KnowledgeBaseFileRefSyncFailureRequest struct {
	OrganizationID string
	WorkspaceID    *string
	DatasetID      string
	RefID          uuid.UUID
	SyncRunID      uuid.UUID
	ErrorCode      string
	ErrorMessage   string
}

type KnowledgeBaseFileRefListResult struct {
	Items []*KnowledgeBaseFileRefItem `json:"items"`
	Total int64                       `json:"total"`
}

type KnowledgeBaseFileRefItem struct {
	ID                          uuid.UUID  `json:"id"`
	DatasetID                   string     `json:"dataset_id"`
	AssetID                     uuid.UUID  `json:"asset_id"`
	FileID                      string     `json:"file_id"`
	FileName                    string     `json:"file_name"`
	SourceFileAvailable         bool       `json:"source_file_available"`
	ProcessingStatus            string     `json:"processing_status"`
	GenerationNo                int64      `json:"generation_no"`
	DatasetDocumentID           *uuid.UUID `json:"dataset_document_id,omitempty"`
	DatasetDocumentEnabled      *bool      `json:"dataset_document_enabled,omitempty"`
	DatasetDocumentSegmentCount *int       `json:"dataset_document_segment_count,omitempty"`
	SyncStatus                  string     `json:"sync_status"`
	SyncedGenerationNo          *int64     `json:"synced_generation_no,omitempty"`
	LastSyncedAt                *time.Time `json:"last_synced_at,omitempty"`
	SyncErrorCode               *string    `json:"sync_error_code,omitempty"`
	SyncErrorMessage            *string    `json:"sync_error_message,omitempty"`
}

type KnowledgeBaseFileRefCreateResult struct {
	Items []*KnowledgeBaseFileRefCreateItem `json:"items"`
}

type KnowledgeBaseFileRefCreateItem struct {
	AssetID      uuid.UUID                                  `json:"asset_id"`
	Ref          *KnowledgeBaseAssetRefView                 `json:"ref,omitempty"`
	SyncRunID    *uuid.UUID                                 `json:"sync_run_id,omitempty"`
	GenerationNo int64                                      `json:"generation_no,omitempty"`
	Success      bool                                       `json:"success"`
	Reason       string                                     `json:"reason,omitempty"`
	Errors       map[string]KnowledgeBaseFileRefCreateError `json:"errors,omitempty"`
}

type KnowledgeBaseFileRefCreateError struct {
	Reason  string `json:"reason"`
	Message string `json:"message,omitempty"`
}

type knowledgeBaseFileRefService struct {
	assets              knowledgeBaseFileAssetReader
	chunks              knowledgeBaseFileChunkReader
	embeddings          knowledgeBaseFileEmbeddingReader
	refs                knowledgeBaseFileRefStore
	files               knowledgeBaseFileFileReader
	datasets            knowledgeBaseFileDatasetReader
	documents           knowledgeBaseFileDocumentReader
	embeddingGeneration DocumentChunkEmbeddingService
	processingProgress  knowledgeBaseFileProcessingProgressUpdater
}

func NewKnowledgeBaseFileRefService(
	assets knowledgeBaseFileAssetReader,
	chunks knowledgeBaseFileChunkReader,
	embeddings knowledgeBaseFileEmbeddingReader,
	refs knowledgeBaseFileRefStore,
	files knowledgeBaseFileFileReader,
	datasets knowledgeBaseFileDatasetReader,
	optionalDeps ...any,
) KnowledgeBaseFileRefService {
	var documentReader knowledgeBaseFileDocumentReader
	var embeddingGeneration DocumentChunkEmbeddingService
	var processingProgress knowledgeBaseFileProcessingProgressUpdater
	for _, dep := range optionalDeps {
		switch typed := dep.(type) {
		case knowledgeBaseFileDocumentReader:
			documentReader = typed
		case DocumentChunkEmbeddingService:
			embeddingGeneration = typed
		case knowledgeBaseFileProcessingProgressUpdater:
			processingProgress = typed
		}
	}
	return &knowledgeBaseFileRefService{
		assets:              assets,
		chunks:              chunks,
		embeddings:          embeddings,
		refs:                refs,
		files:               files,
		datasets:            datasets,
		documents:           documentReader,
		embeddingGeneration: embeddingGeneration,
		processingProgress:  processingProgress,
	}
}

func (s *knowledgeBaseFileRefService) ListCandidates(ctx context.Context, req KnowledgeBaseFileCandidateRequest) (*KnowledgeBaseFileCandidateResult, error) {
	if err := validateKnowledgeBaseFileRefScope(req.OrganizationID, req.DatasetID); err != nil {
		return nil, err
	}
	dataset, err := s.datasets.GetByID(ctx, req.DatasetID)
	if err != nil {
		return nil, err
	}
	if dataset == nil || dataset.OrganizationID != req.OrganizationID {
		return nil, ErrDatasetNotFound
	}

	filter := datalibRepo.DocumentAssetListFilter{
		OrganizationID:       req.OrganizationID,
		WorkspaceID:          datasetWorkspaceID(dataset),
		ActiveSourceFileOnly: true,
		Limit:                req.Limit,
		Offset:               req.Offset,
	}
	if req.Filter == "" || req.Filter == FileCandidateFilterAddable {
		filter.ProductStatus = datalibModel.DocumentAssetProductStatusReady
	}
	assets, total, err := s.assets.ListAssets(ctx, filter)
	if err != nil {
		return nil, err
	}

	fileIDs := make([]string, 0, len(assets))
	for _, asset := range assets {
		fileIDs = append(fileIDs, asset.SourceFileID)
	}
	filesByID, err := s.files.ListByTenantAndIDs(ctx, req.OrganizationID, fileIDs)
	if err != nil {
		return nil, err
	}

	items := make([]*KnowledgeBaseFileCandidate, 0, len(assets))
	var skippedMissingSource int64
	for _, asset := range assets {
		file := filesByID[asset.SourceFileID]
		if file == nil || file.IsArchived {
			skippedMissingSource++
			continue
		}
		if !assetInDatasetWorkspace(dataset, asset) {
			continue
		}
		if keyword := strings.TrimSpace(req.Keyword); keyword != "" && !candidateMatchesKeyword(asset, file, keyword) {
			continue
		}
		candidate, err := s.buildCandidate(ctx, dataset, asset, file)
		if err != nil {
			return nil, err
		}
		switch req.Filter {
		case FileCandidateFilterAdded:
			if !candidate.AlreadyAdded {
				continue
			}
		case FileCandidateFilterAll, "":
		default:
			if req.Filter == FileCandidateFilterAddable && !candidateVisibleInAddable(candidate) {
				continue
			}
		}
		items = append(items, candidate)
	}
	if skippedMissingSource > 0 && total >= skippedMissingSource {
		total -= skippedMissingSource
	}
	return &KnowledgeBaseFileCandidateResult{Items: items, Total: total}, nil
}

func (s *knowledgeBaseFileRefService) ListRefs(ctx context.Context, req KnowledgeBaseFileRefListRequest) (*KnowledgeBaseFileRefListResult, error) {
	if err := validateKnowledgeBaseFileRefScope(req.OrganizationID, req.DatasetID); err != nil {
		return nil, err
	}
	dataset, err := s.datasets.GetByID(ctx, req.DatasetID)
	if err != nil {
		return nil, err
	}
	if dataset == nil || dataset.OrganizationID != req.OrganizationID {
		return nil, ErrDatasetNotFound
	}
	refs, total, err := s.refs.List(ctx, datalibRepo.KnowledgeBaseAssetRefListFilter{
		OrganizationID: req.OrganizationID,
		DatasetID:      req.DatasetID,
		SyncStatus:     req.SyncStatus,
		Limit:          req.Limit,
		Offset:         req.Offset,
	})
	if err != nil {
		return nil, err
	}
	assetByID := make(map[uuid.UUID]*datalibModel.DocumentAsset, len(refs))
	fileIDs := make([]string, 0, len(refs))
	for _, ref := range refs {
		asset, err := s.assets.GetAssetByID(ctx, ref.AssetID)
		if err != nil {
			return nil, err
		}
		if asset == nil {
			continue
		}
		assetByID[ref.AssetID] = asset
		fileIDs = append(fileIDs, asset.SourceFileID)
	}
	filesByID, err := s.files.ListByTenantAndIDs(ctx, req.OrganizationID, fileIDs)
	if err != nil {
		return nil, err
	}
	documentsByID, err := s.loadRefDocuments(ctx, refs)
	if err != nil {
		return nil, err
	}
	items := make([]*KnowledgeBaseFileRefItem, 0, len(refs))
	for _, ref := range refs {
		asset := assetByID[ref.AssetID]
		if asset == nil {
			continue
		}
		if !assetInDatasetWorkspace(dataset, asset) {
			continue
		}
		file := filesByID[asset.SourceFileID]
		fileName := asset.Title
		if file != nil && file.Name != "" {
			fileName = file.Name
		}
		var documentEnabled *bool
		documentSegmentCount := asset.ChunkCount
		if ref.DatasetDocumentID != nil {
			if document := documentsByID[ref.DatasetDocumentID.String()]; document != nil {
				enabled := document.Enabled
				documentEnabled = &enabled
				documentSegmentCount = document.SegmentCount
			}
		}
		items = append(items, &KnowledgeBaseFileRefItem{
			ID:                          ref.ID,
			DatasetID:                   ref.DatasetID,
			AssetID:                     ref.AssetID,
			FileID:                      asset.SourceFileID,
			FileName:                    fileName,
			SourceFileAvailable:         file != nil,
			ProcessingStatus:            asset.ProductStatus,
			GenerationNo:                asset.GenerationNo,
			DatasetDocumentID:           ref.DatasetDocumentID,
			DatasetDocumentEnabled:      documentEnabled,
			DatasetDocumentSegmentCount: &documentSegmentCount,
			SyncStatus:                  ref.SyncStatus,
			SyncedGenerationNo:          ref.SyncedGenerationNo,
			LastSyncedAt:                ref.LastSyncedAt,
			SyncErrorCode:               ref.SyncErrorCode,
			SyncErrorMessage:            ref.SyncErrorMessage,
		})
	}
	return &KnowledgeBaseFileRefListResult{Items: items, Total: total}, nil
}

func (s *knowledgeBaseFileRefService) loadRefDocuments(ctx context.Context, refs []*datalibModel.KnowledgeBaseAssetRef) (map[string]*datasetModel.Document, error) {
	documentsByID := map[string]*datasetModel.Document{}
	if s.documents == nil {
		return documentsByID, nil
	}
	documentIDs := make([]string, 0, len(refs))
	seen := map[string]struct{}{}
	for _, ref := range refs {
		if ref.DatasetDocumentID == nil {
			continue
		}
		id := ref.DatasetDocumentID.String()
		if _, ok := seen[id]; ok {
			continue
		}
		seen[id] = struct{}{}
		documentIDs = append(documentIDs, id)
	}
	if len(documentIDs) == 0 {
		return documentsByID, nil
	}
	documents, err := s.documents.GetDocumentsByIDs(ctx, documentIDs)
	if err != nil {
		return nil, err
	}
	for _, document := range documents {
		if document != nil {
			_, total, err := s.documents.GetSegmentCounts(ctx, document.ID)
			if err != nil {
				return nil, err
			}
			document.SegmentCount = total
			documentsByID[document.ID] = document
		}
	}
	return documentsByID, nil
}

func (s *knowledgeBaseFileRefService) CreateRefs(ctx context.Context, req KnowledgeBaseFileRefCreateRequest) (*KnowledgeBaseFileRefCreateResult, error) {
	if err := validateKnowledgeBaseFileRefScope(req.OrganizationID, req.DatasetID); err != nil {
		return nil, err
	}
	dataset, err := s.datasets.GetByID(ctx, req.DatasetID)
	if err != nil {
		return nil, err
	}
	if dataset == nil || dataset.OrganizationID != req.OrganizationID {
		return nil, ErrDatasetNotFound
	}
	result := &KnowledgeBaseFileRefCreateResult{Items: make([]*KnowledgeBaseFileRefCreateItem, 0, len(req.AssetIDs))}
	for _, assetID := range req.AssetIDs {
		item := &KnowledgeBaseFileRefCreateItem{AssetID: assetID}
		ref, syncRunID, generationNo, reason, err := s.createOneRef(ctx, dataset, req, assetID)
		if err != nil {
			return nil, err
		}
		if reason != "" {
			item.Success = false
			item.Reason = reason
			result.Items = append(result.Items, item)
			continue
		}
		item.Success = true
		item.Ref = newKnowledgeBaseAssetRefView(ref)
		item.SyncRunID = &syncRunID
		item.GenerationNo = generationNo
		result.Items = append(result.Items, item)
	}
	return result, nil
}

func (s *knowledgeBaseFileRefService) GetRef(ctx context.Context, req KnowledgeBaseFileRefGetRequest) (*KnowledgeBaseAssetRefView, error) {
	ref, err := s.getScopedRef(ctx, req)
	if err != nil {
		return nil, err
	}
	return newKnowledgeBaseAssetRefView(ref), nil
}

func (s *knowledgeBaseFileRefService) GenerateCandidateEmbeddings(ctx context.Context, req KnowledgeBaseFileCandidateEmbeddingRequest) (*KnowledgeBaseFileCandidateEmbeddingResult, error) {
	if err := validateKnowledgeBaseFileRefScope(req.OrganizationID, req.DatasetID); err != nil {
		return nil, err
	}
	if req.AssetID == uuid.Nil {
		return nil, ErrAssetIDRequired
	}
	if s.embeddingGeneration == nil {
		return nil, ErrEmbeddingServiceRequired
	}
	dataset, err := s.datasets.GetByID(ctx, req.DatasetID)
	if err != nil {
		return nil, err
	}
	if dataset == nil || dataset.OrganizationID != req.OrganizationID {
		return nil, ErrDatasetNotFound
	}
	asset, err := s.assets.GetAssetByID(ctx, req.AssetID)
	if err != nil {
		return nil, err
	}
	if asset == nil || asset.OrganizationID != req.OrganizationID || !assetInDatasetWorkspace(dataset, asset) {
		return nil, ErrDocumentAssetNotFound
	}
	targetProvider, targetModel := datasetEmbeddingTarget(dataset, asset)
	if targetModel == "" {
		return &KnowledgeBaseFileCandidateEmbeddingResult{
			AssetID:      asset.ID,
			GenerationNo: asset.GenerationNo,
			Reason:       FileCandidateReasonDatasetModelMissing,
		}, nil
	}
	chunks, chunkCount, err := s.listCurrentCandidateChunks(ctx, asset)
	if err != nil {
		return nil, err
	}
	if chunkCount == 0 {
		return &KnowledgeBaseFileCandidateEmbeddingResult{
			AssetID:      asset.ID,
			GenerationNo: asset.GenerationNo,
			Reason:       FileCandidateReasonMissingChunks,
		}, nil
	}
	s.updateCandidateEmbeddingProgress(ctx, req, asset, targetProvider, targetModel, 0, int(chunkCount))
	embeddingResult, err := s.embeddingGeneration.GenerateAdditionalEmbeddings(ctx, GenerateDocumentChunkEmbeddingsInput{
		OrganizationID:    req.OrganizationID,
		AssetID:           asset.ID,
		ProcessingRunID:   assetProcessingRunID(asset),
		GenerationNo:      asset.GenerationNo,
		EmbeddingProvider: targetProvider,
		EmbeddingModel:    targetModel,
		RequestedBy:       req.RequestedBy,
		Chunks:            chunks,
		OnProgress: func(snapshot GenerateDocumentChunkEmbeddingsProgress) {
			s.updateCandidateEmbeddingProgress(ctx, req, asset, targetProvider, targetModel, snapshot.Completed, snapshot.Total)
		},
	})
	if err != nil {
		return nil, err
	}
	if embeddingResult != nil {
		targetProvider = embeddingResult.EmbeddingProvider
		targetModel = embeddingResult.EmbeddingModel
	}
	targetEmbeddingCount, err := s.embeddings.CountReadyByAssetGenerationModel(ctx, asset.OrganizationID, asset.ID, asset.GenerationNo, targetProvider, targetModel)
	if err != nil {
		return nil, err
	}
	reason := evaluateFileCandidateReason(asset, false, chunkCount, targetEmbeddingCount, targetModel)
	return &KnowledgeBaseFileCandidateEmbeddingResult{
		AssetID:              asset.ID,
		GenerationNo:         asset.GenerationNo,
		EmbeddingProvider:    targetProvider,
		EmbeddingModel:       targetModel,
		EmbeddingCount:       int64(embeddingResultCount(embeddingResult)),
		TargetEmbeddingCount: targetEmbeddingCount,
		ChunkCount:           chunkCount,
		Addable:              reason == "",
		Reason:               reason,
	}, nil
}

func (s *knowledgeBaseFileRefService) updateCandidateEmbeddingProgress(ctx context.Context, req KnowledgeBaseFileCandidateEmbeddingRequest, asset *datalibModel.DocumentAsset, provider string, modelName string, completed int, total int) {
	if s == nil || s.processingProgress == nil || req.ProcessingRequestID == uuid.Nil || asset == nil {
		return
	}
	_, _ = s.processingProgress.UpdateRequestExecutionMetadata(ctx, req.OrganizationID, req.ProcessingRequestID, map[string]any{
		"task_type":           "file_candidate_embedding",
		"dataset_id":          req.DatasetID,
		"asset_id":            asset.ID.String(),
		"generation_no":       asset.GenerationNo,
		"embedding_provider":  provider,
		"embedding_model":     modelName,
		"chunk_count":         int64(total),
		"progress_completed":  int64(completed),
		"progress_total":      int64(total),
		"progress_stage":      "embedding",
		"progress_percentage": candidateEmbeddingProgressPercentage(completed, total),
	})
}

func candidateEmbeddingProgressPercentage(completed int, total int) int {
	if total <= 0 || completed <= 0 {
		return 0
	}
	if completed >= total {
		return 100
	}
	return completed * 100 / total
}

func (s *knowledgeBaseFileRefService) RetryRef(ctx context.Context, req KnowledgeBaseFileRefRetryRequest) (*KnowledgeBaseFileRefCreateItem, error) {
	if err := validateKnowledgeBaseFileRefScope(req.OrganizationID, req.DatasetID); err != nil {
		return nil, err
	}
	if req.RefID == uuid.Nil {
		return nil, ErrKnowledgeBaseFileRefNotFound
	}
	dataset, err := s.datasets.GetByID(ctx, req.DatasetID)
	if err != nil {
		return nil, err
	}
	if dataset == nil || dataset.OrganizationID != req.OrganizationID {
		return nil, ErrDatasetNotFound
	}
	ref, err := s.refs.GetByID(ctx, req.RefID)
	if err != nil {
		return nil, err
	}
	if ref == nil || ref.OrganizationID != req.OrganizationID || ref.DatasetID != req.DatasetID {
		return nil, ErrKnowledgeBaseFileRefNotFound
	}
	asset, err := s.assets.GetAssetByID(ctx, ref.AssetID)
	if err != nil {
		return nil, err
	}
	if asset == nil || asset.OrganizationID != req.OrganizationID {
		return &KnowledgeBaseFileRefCreateItem{AssetID: ref.AssetID, Success: false, Reason: FileCandidateReasonNotReady}, nil
	}
	if !assetInDatasetWorkspace(dataset, asset) {
		return nil, ErrKnowledgeBaseFileRefNotFound
	}
	reason, generationNo, err := s.evaluateAssetSyncReadiness(ctx, dataset, asset)
	if err != nil {
		return nil, err
	}
	if reason != "" {
		return &KnowledgeBaseFileRefCreateItem{AssetID: asset.ID, Ref: newKnowledgeBaseAssetRefView(ref), Success: false, Reason: reason, GenerationNo: generationNo}, nil
	}
	syncRunID := uuid.New()
	ref, err = s.refs.MarkPending(ctx, req.OrganizationID, ref.ID, syncRunID, nil, nil)
	if err != nil {
		return nil, err
	}
	if ref == nil {
		return nil, ErrKnowledgeBaseFileRefNotFound
	}
	return &KnowledgeBaseFileRefCreateItem{
		AssetID:      asset.ID,
		Ref:          newKnowledgeBaseAssetRefView(ref),
		SyncRunID:    &syncRunID,
		GenerationNo: generationNo,
		Success:      true,
	}, nil
}

func (s *knowledgeBaseFileRefService) MarkRefSyncFailed(ctx context.Context, req KnowledgeBaseFileRefSyncFailureRequest) (*KnowledgeBaseAssetRefView, error) {
	if req.SyncRunID == uuid.Nil {
		return nil, errors.New("sync_run_id is required")
	}
	if strings.TrimSpace(req.ErrorCode) == "" {
		return nil, errors.New("sync_error_code is required")
	}
	ref, err := s.getScopedRef(ctx, KnowledgeBaseFileRefGetRequest{
		OrganizationID: req.OrganizationID,
		WorkspaceID:    req.WorkspaceID,
		DatasetID:      req.DatasetID,
		RefID:          req.RefID,
	})
	if err != nil {
		return nil, err
	}
	updated, err := s.refs.MarkFailed(ctx, req.OrganizationID, ref.ID, req.SyncRunID, req.ErrorCode, req.ErrorMessage)
	if err != nil {
		return nil, err
	}
	if updated == nil {
		return nil, ErrKnowledgeBaseFileRefNotFound
	}
	return newKnowledgeBaseAssetRefView(updated), nil
}

func (s *knowledgeBaseFileRefService) RemoveRef(ctx context.Context, req KnowledgeBaseFileRefGetRequest) (*KnowledgeBaseAssetRefView, error) {
	ref, err := s.getScopedRef(ctx, req)
	if err != nil {
		return nil, err
	}
	removed, err := s.refs.SoftDelete(ctx, req.OrganizationID, ref.ID)
	if err != nil {
		return nil, err
	}
	if removed == nil {
		return nil, ErrKnowledgeBaseFileRefNotFound
	}
	return newKnowledgeBaseAssetRefView(removed), nil
}

func (s *knowledgeBaseFileRefService) getScopedRef(ctx context.Context, req KnowledgeBaseFileRefGetRequest) (*datalibModel.KnowledgeBaseAssetRef, error) {
	if err := validateKnowledgeBaseFileRefScope(req.OrganizationID, req.DatasetID); err != nil {
		return nil, err
	}
	if req.RefID == uuid.Nil {
		return nil, ErrKnowledgeBaseFileRefNotFound
	}
	dataset, err := s.datasets.GetByID(ctx, req.DatasetID)
	if err != nil {
		return nil, err
	}
	if dataset == nil || dataset.OrganizationID != req.OrganizationID {
		return nil, ErrDatasetNotFound
	}
	ref, err := s.refs.GetByID(ctx, req.RefID)
	if err != nil {
		return nil, err
	}
	if ref == nil || ref.OrganizationID != req.OrganizationID || ref.DatasetID != req.DatasetID {
		return nil, ErrKnowledgeBaseFileRefNotFound
	}
	asset, err := s.assets.GetAssetByID(ctx, ref.AssetID)
	if err != nil {
		return nil, err
	}
	if asset == nil || !assetInDatasetWorkspace(dataset, asset) {
		return nil, ErrKnowledgeBaseFileRefNotFound
	}
	return ref, nil
}

func (s *knowledgeBaseFileRefService) createOneRef(ctx context.Context, dataset *datasetModel.Dataset, req KnowledgeBaseFileRefCreateRequest, assetID uuid.UUID) (*datalibModel.KnowledgeBaseAssetRef, uuid.UUID, int64, string, error) {
	if assetID == uuid.Nil {
		return nil, uuid.Nil, 0, FileCandidateReasonNotReady, nil
	}
	asset, err := s.assets.GetAssetByID(ctx, assetID)
	if err != nil {
		return nil, uuid.Nil, 0, "", err
	}
	if asset == nil || asset.OrganizationID != req.OrganizationID {
		return nil, uuid.Nil, 0, FileCandidateReasonNotReady, nil
	}
	if !assetInDatasetWorkspace(dataset, asset) {
		return nil, uuid.Nil, 0, FileCandidateReasonNotReady, nil
	}
	candidate, err := s.buildCandidate(ctx, dataset, asset, nil)
	if err != nil {
		return nil, uuid.Nil, 0, "", err
	}
	if !candidate.Addable {
		return nil, uuid.Nil, 0, candidate.Reason, nil
	}
	syncRunID := uuid.New()
	ref := &datalibModel.KnowledgeBaseAssetRef{
		OrganizationID: req.OrganizationID,
		WorkspaceID:    asset.WorkspaceID,
		DatasetID:      req.DatasetID,
		AssetID:        asset.ID,
		SyncStatus:     datalibModel.KnowledgeBaseAssetRefSyncStatusPending,
		SyncRunID:      &syncRunID,
		CreatedBy:      req.CreatedBy,
		MetadataJSON: map[string]any{
			"source":        "file_asset_sync",
			"generation_no": asset.GenerationNo,
		},
	}
	if err := s.refs.Create(ctx, ref); err != nil {
		return nil, uuid.Nil, 0, "", err
	}
	return ref, syncRunID, asset.GenerationNo, "", nil
}

func (s *knowledgeBaseFileRefService) evaluateAssetSyncReadiness(ctx context.Context, dataset *datasetModel.Dataset, asset *datalibModel.DocumentAsset) (string, int64, error) {
	chunkCount, err := s.countCurrentCandidateChunks(ctx, asset)
	if err != nil {
		return "", asset.GenerationNo, err
	}
	targetProvider, targetModel := datasetEmbeddingTarget(dataset, asset)
	embeddingCount, err := s.embeddings.CountReadyByAssetGenerationModel(ctx, asset.OrganizationID, asset.ID, asset.GenerationNo, targetProvider, targetModel)
	if err != nil {
		return "", asset.GenerationNo, err
	}
	reason := evaluateFileCandidateReason(asset, false, chunkCount, embeddingCount, targetModel)
	return reason, asset.GenerationNo, nil
}

func (s *knowledgeBaseFileRefService) buildCandidate(ctx context.Context, dataset *datasetModel.Dataset, asset *datalibModel.DocumentAsset, file *fileModel.UploadFile) (*KnowledgeBaseFileCandidate, error) {
	chunkCount, err := s.countCurrentCandidateChunks(ctx, asset)
	if err != nil {
		return nil, err
	}
	embeddingCount, err := s.embeddings.CountReadyByAssetGeneration(ctx, asset.OrganizationID, asset.ID, asset.GenerationNo)
	if err != nil {
		return nil, err
	}
	targetProvider, targetModel := datasetEmbeddingTarget(dataset, asset)
	targetEmbeddingCount, err := s.embeddings.CountReadyByAssetGenerationModel(ctx, asset.OrganizationID, asset.ID, asset.GenerationNo, targetProvider, targetModel)
	if err != nil {
		return nil, err
	}
	existing, err := s.refs.FindActiveByAsset(ctx, asset.OrganizationID, dataset.ID, asset.ID)
	if err != nil {
		return nil, err
	}
	referenceCount, err := s.refs.CountActiveByAssetID(ctx, asset.OrganizationID, asset.ID)
	if err != nil {
		return nil, err
	}
	name := asset.Title
	fileExtension := ""
	var fileSize *int64
	if file != nil && file.Name != "" {
		name = file.Name
	}
	if file != nil {
		fileExtension = file.Extension
		size := file.Size
		fileSize = &size
	}
	candidate := &KnowledgeBaseFileCandidate{
		FileID:                  asset.SourceFileID,
		AssetID:                 asset.ID,
		Name:                    name,
		FileExtension:           fileExtension,
		FileSize:                fileSize,
		UpdatedAt:               asset.UpdatedAt,
		ProcessingStatus:        asset.ProductStatus,
		GenerationNo:            asset.GenerationNo,
		EmbeddingProvider:       asset.EmbeddingProvider,
		EmbeddingModel:          asset.EmbeddingModel,
		TargetEmbeddingProvider: targetProvider,
		TargetEmbeddingModel:    targetModel,
		AlreadyAdded:            existing != nil,
		ReferenceCount:          referenceCount,
		ChunkCount:              chunkCount,
		EmbeddingCount:          embeddingCount,
		TargetEmbeddingCount:    targetEmbeddingCount,
	}
	candidate.Reason = evaluateFileCandidateReason(asset, candidate.AlreadyAdded, chunkCount, targetEmbeddingCount, targetModel)
	candidate.Addable = candidate.Reason == ""
	candidate.RequiresEmbeddingGeneration = candidate.Reason == FileCandidateReasonMissingDatasetEmbedding
	return candidate, nil
}

func evaluateFileCandidateReason(asset *datalibModel.DocumentAsset, alreadyAdded bool, chunkCount int64, targetEmbeddingCount int64, targetModel string) string {
	if alreadyAdded {
		return FileCandidateReasonAlreadyAdded
	}
	if asset.ProductStatus != datalibModel.DocumentAssetProductStatusReady {
		return FileCandidateReasonNotReady
	}
	if chunkCount == 0 {
		return FileCandidateReasonMissingChunks
	}
	if targetModel == "" {
		return FileCandidateReasonDatasetModelMissing
	}
	if targetEmbeddingCount < chunkCount {
		return FileCandidateReasonMissingDatasetEmbedding
	}
	return ""
}

func validateKnowledgeBaseFileRefScope(organizationID string, datasetID string) error {
	if organizationID == "" {
		return ErrOrganizationIDRequired
	}
	if datasetID == "" {
		return ErrDatasetIDRequired
	}
	return nil
}

func datasetWorkspaceID(dataset *datasetModel.Dataset) *string {
	if dataset == nil || strings.TrimSpace(dataset.WorkspaceID) == "" {
		return nil
	}
	return &dataset.WorkspaceID
}

func assetInDatasetWorkspace(dataset *datasetModel.Dataset, asset *datalibModel.DocumentAsset) bool {
	if dataset == nil || asset == nil {
		return false
	}
	datasetWorkspace := strings.TrimSpace(dataset.WorkspaceID)
	if datasetWorkspace == "" {
		return asset.WorkspaceID == nil || strings.TrimSpace(*asset.WorkspaceID) == ""
	}
	return asset.WorkspaceID != nil && strings.TrimSpace(*asset.WorkspaceID) == datasetWorkspace
}

func candidateMatchesKeyword(asset *datalibModel.DocumentAsset, file *fileModel.UploadFile, keyword string) bool {
	normalizedKeyword := strings.ToLower(strings.TrimSpace(keyword))
	if normalizedKeyword == "" {
		return true
	}
	if file != nil && strings.Contains(strings.ToLower(file.Name), normalizedKeyword) {
		return true
	}
	return asset != nil && strings.Contains(strings.ToLower(asset.Title), normalizedKeyword)
}

func candidateVisibleInAddable(candidate *KnowledgeBaseFileCandidate) bool {
	return candidate != nil && (candidate.Addable || candidate.RequiresEmbeddingGeneration)
}

func datasetEmbeddingTarget(dataset *datasetModel.Dataset, asset *datalibModel.DocumentAsset) (string, string) {
	provider := ""
	modelName := ""
	if dataset != nil {
		if dataset.EmbeddingModelProvider != nil {
			provider = strings.TrimSpace(*dataset.EmbeddingModelProvider)
		}
		if dataset.EmbeddingModel != nil {
			modelName = strings.TrimSpace(*dataset.EmbeddingModel)
		}
	}
	if modelName == "" && asset != nil {
		if asset.EmbeddingProvider != nil {
			provider = strings.TrimSpace(*asset.EmbeddingProvider)
		}
		if asset.EmbeddingModel != nil {
			modelName = strings.TrimSpace(*asset.EmbeddingModel)
		}
	}
	return provider, modelName
}

func (s *knowledgeBaseFileRefService) countCurrentCandidateChunks(ctx context.Context, asset *datalibModel.DocumentAsset) (int64, error) {
	if asset == nil || asset.GenerationNo <= 0 {
		return 0, nil
	}
	if counter, ok := s.chunks.(knowledgeBaseFileCandidateChunkCounter); ok {
		enabled := true
		return counter.CountByAssetGenerationAndTypesFiltered(ctx, asset.OrganizationID, asset.ID, asset.GenerationNo, currentCandidateChunkTypes(), &enabled, datalibModel.DocumentChunkStatusReady)
	}
	_, total, err := s.listCurrentCandidateChunks(ctx, asset)
	return total, err
}

func (s *knowledgeBaseFileRefService) listCurrentCandidateChunks(ctx context.Context, asset *datalibModel.DocumentAsset) ([]*datalibModel.DocumentChunk, int64, error) {
	if asset == nil || asset.GenerationNo <= 0 {
		return nil, 0, nil
	}
	generationNo := asset.GenerationNo
	enabled := true
	out := make([]*datalibModel.DocumentChunk, 0)
	for offset := 0; ; offset += 500 {
		items, total, err := s.chunks.List(ctx, datalibRepo.DocumentChunkListFilter{
			OrganizationID: asset.OrganizationID,
			AssetID:        asset.ID,
			GenerationNo:   &generationNo,
			ChunkTypes:     currentCandidateChunkTypes(),
			Enabled:        &enabled,
			Status:         datalibModel.DocumentChunkStatusReady,
			Limit:          500,
			Offset:         offset,
		})
		if err != nil {
			return nil, 0, err
		}
		out = append(out, items...)
		if int64(len(out)) >= total || len(items) == 0 {
			return out, total, nil
		}
	}
}

func currentCandidateChunkTypes() []string {
	return []string{
		datalibModel.DocumentChunkTypeChild,
		datalibModel.DocumentChunkTypeAuto,
		datalibModel.DocumentChunkTypeManual,
	}
}

func assetProcessingRunID(asset *datalibModel.DocumentAsset) uuid.UUID {
	if asset == nil || asset.ProcessingRunID == nil {
		return uuid.Nil
	}
	return *asset.ProcessingRunID
}

func embeddingResultCount(result *GenerateDocumentChunkEmbeddingsResult) int {
	if result == nil {
		return 0
	}
	return result.EmbeddingCount
}

var ErrDatasetNotFound = errors.New("dataset not found")
var ErrKnowledgeBaseFileRefNotFound = errors.New("knowledge base file ref not found")
