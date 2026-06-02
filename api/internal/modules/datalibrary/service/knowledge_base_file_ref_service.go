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

	FileCandidateReasonNotReady               = "not_ready"
	FileCandidateReasonAlreadyAdded           = "already_added"
	FileCandidateReasonEmbeddingModelMismatch = "embedding_model_mismatch"
	FileCandidateReasonMissingChunks          = "missing_chunks"
	FileCandidateReasonMissingEmbedding       = "missing_embedding"
)

type knowledgeBaseFileAssetReader interface {
	GetAssetByID(ctx context.Context, id uuid.UUID) (*datalibModel.DocumentAsset, error)
	ListAssets(ctx context.Context, filter datalibRepo.DocumentAssetListFilter) ([]*datalibModel.DocumentAsset, int64, error)
}

type knowledgeBaseFileChunkReader interface {
	CountByAssetGenerationAndTypes(ctx context.Context, organizationID string, assetID uuid.UUID, generationNo int64, chunkTypes []string) (int64, error)
}

type knowledgeBaseFileEmbeddingReader interface {
	CountReadyByAssetGeneration(ctx context.Context, organizationID string, assetID uuid.UUID, generationNo int64) (int64, error)
}

type knowledgeBaseFileRefStore interface {
	Create(ctx context.Context, item *datalibModel.KnowledgeBaseAssetRef) error
	FindActiveByAsset(ctx context.Context, organizationID string, datasetID string, assetID uuid.UUID) (*datalibModel.KnowledgeBaseAssetRef, error)
	List(ctx context.Context, filter datalibRepo.KnowledgeBaseAssetRefListFilter) ([]*datalibModel.KnowledgeBaseAssetRef, int64, error)
}

type knowledgeBaseFileFileReader interface {
	ListByTenantAndIDs(ctx context.Context, tenantID string, ids []string) (map[string]*fileModel.UploadFile, error)
}

type knowledgeBaseFileDatasetReader interface {
	GetByID(ctx context.Context, id string) (*datasetModel.Dataset, error)
}

type KnowledgeBaseFileRefService interface {
	ListCandidates(ctx context.Context, req KnowledgeBaseFileCandidateRequest) (*KnowledgeBaseFileCandidateResult, error)
	ListRefs(ctx context.Context, req KnowledgeBaseFileRefListRequest) (*KnowledgeBaseFileRefListResult, error)
	CreateRefs(ctx context.Context, req KnowledgeBaseFileRefCreateRequest) (*KnowledgeBaseFileRefCreateResult, error)
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
	FileID            string    `json:"file_id"`
	AssetID           uuid.UUID `json:"asset_id"`
	Name              string    `json:"name"`
	ProcessingStatus  string    `json:"processing_status"`
	GenerationNo      int64     `json:"generation_no"`
	Addable           bool      `json:"addable"`
	Reason            string    `json:"reason,omitempty"`
	EmbeddingProvider *string   `json:"embedding_provider,omitempty"`
	EmbeddingModel    *string   `json:"embedding_model,omitempty"`
	AlreadyAdded      bool      `json:"already_added"`
	ChunkCount        int64     `json:"chunk_count"`
	EmbeddingCount    int64     `json:"embedding_count"`
}

type KnowledgeBaseFileRefCreateRequest struct {
	OrganizationID string
	WorkspaceID    *string
	DatasetID      string
	AssetIDs       []uuid.UUID
	CreatedBy      string
}

type KnowledgeBaseFileRefListRequest struct {
	OrganizationID string
	WorkspaceID    *string
	DatasetID      string
	SyncStatus     string
	Limit          int
	Offset         int
}

type KnowledgeBaseFileRefListResult struct {
	Items []*KnowledgeBaseFileRefItem `json:"items"`
	Total int64                       `json:"total"`
}

type KnowledgeBaseFileRefItem struct {
	ID                 uuid.UUID  `json:"id"`
	DatasetID          string     `json:"dataset_id"`
	AssetID            uuid.UUID  `json:"asset_id"`
	FileID             string     `json:"file_id"`
	FileName           string     `json:"file_name"`
	ProcessingStatus   string     `json:"processing_status"`
	GenerationNo       int64      `json:"generation_no"`
	DatasetDocumentID  *uuid.UUID `json:"dataset_document_id,omitempty"`
	SyncStatus         string     `json:"sync_status"`
	SyncedGenerationNo *int64     `json:"synced_generation_no,omitempty"`
	LastSyncedAt       *time.Time `json:"last_synced_at,omitempty"`
	SyncErrorCode      *string    `json:"sync_error_code,omitempty"`
	SyncErrorMessage   *string    `json:"sync_error_message,omitempty"`
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
	assets     knowledgeBaseFileAssetReader
	chunks     knowledgeBaseFileChunkReader
	embeddings knowledgeBaseFileEmbeddingReader
	refs       knowledgeBaseFileRefStore
	files      knowledgeBaseFileFileReader
	datasets   knowledgeBaseFileDatasetReader
}

func NewKnowledgeBaseFileRefService(
	assets knowledgeBaseFileAssetReader,
	chunks knowledgeBaseFileChunkReader,
	embeddings knowledgeBaseFileEmbeddingReader,
	refs knowledgeBaseFileRefStore,
	files knowledgeBaseFileFileReader,
	datasets knowledgeBaseFileDatasetReader,
) KnowledgeBaseFileRefService {
	return &knowledgeBaseFileRefService{
		assets:     assets,
		chunks:     chunks,
		embeddings: embeddings,
		refs:       refs,
		files:      files,
		datasets:   datasets,
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
		OrganizationID: req.OrganizationID,
		WorkspaceID:    req.WorkspaceID,
		Limit:          req.Limit,
		Offset:         req.Offset,
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
	for _, asset := range assets {
		file := filesByID[asset.SourceFileID]
		if keyword := strings.TrimSpace(req.Keyword); keyword != "" && !candidateMatchesKeyword(file, keyword) {
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
			if req.Filter == FileCandidateFilterAddable && !candidate.Addable {
				continue
			}
		}
		items = append(items, candidate)
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
	items := make([]*KnowledgeBaseFileRefItem, 0, len(refs))
	for _, ref := range refs {
		asset := assetByID[ref.AssetID]
		if asset == nil {
			continue
		}
		if req.WorkspaceID != nil && (asset.WorkspaceID == nil || *asset.WorkspaceID != *req.WorkspaceID) {
			continue
		}
		file := filesByID[asset.SourceFileID]
		fileName := asset.Title
		if file != nil && file.Name != "" {
			fileName = file.Name
		}
		items = append(items, &KnowledgeBaseFileRefItem{
			ID:                 ref.ID,
			DatasetID:          ref.DatasetID,
			AssetID:            ref.AssetID,
			FileID:             asset.SourceFileID,
			FileName:           fileName,
			ProcessingStatus:   asset.ProductStatus,
			GenerationNo:       asset.GenerationNo,
			DatasetDocumentID:  ref.DatasetDocumentID,
			SyncStatus:         ref.SyncStatus,
			SyncedGenerationNo: ref.SyncedGenerationNo,
			LastSyncedAt:       ref.LastSyncedAt,
			SyncErrorCode:      ref.SyncErrorCode,
			SyncErrorMessage:   ref.SyncErrorMessage,
		})
	}
	return &KnowledgeBaseFileRefListResult{Items: items, Total: total}, nil
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
	if req.WorkspaceID != nil && (asset.WorkspaceID == nil || *asset.WorkspaceID != *req.WorkspaceID) {
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

func (s *knowledgeBaseFileRefService) buildCandidate(ctx context.Context, dataset *datasetModel.Dataset, asset *datalibModel.DocumentAsset, file *fileModel.UploadFile) (*KnowledgeBaseFileCandidate, error) {
	chunkCount, err := s.chunks.CountByAssetGenerationAndTypes(ctx, asset.OrganizationID, asset.ID, asset.GenerationNo, []string{
		datalibModel.DocumentChunkTypeChild,
		datalibModel.DocumentChunkTypeAuto,
		datalibModel.DocumentChunkTypeManual,
	})
	if err != nil {
		return nil, err
	}
	embeddingCount, err := s.embeddings.CountReadyByAssetGeneration(ctx, asset.OrganizationID, asset.ID, asset.GenerationNo)
	if err != nil {
		return nil, err
	}
	existing, err := s.refs.FindActiveByAsset(ctx, asset.OrganizationID, dataset.ID, asset.ID)
	if err != nil {
		return nil, err
	}
	name := asset.Title
	if file != nil && file.Name != "" {
		name = file.Name
	}
	candidate := &KnowledgeBaseFileCandidate{
		FileID:            asset.SourceFileID,
		AssetID:           asset.ID,
		Name:              name,
		ProcessingStatus:  asset.ProductStatus,
		GenerationNo:      asset.GenerationNo,
		EmbeddingProvider: asset.EmbeddingProvider,
		EmbeddingModel:    asset.EmbeddingModel,
		AlreadyAdded:      existing != nil,
		ChunkCount:        chunkCount,
		EmbeddingCount:    embeddingCount,
	}
	candidate.Reason = evaluateFileCandidateReason(dataset, asset, candidate.AlreadyAdded, chunkCount, embeddingCount)
	candidate.Addable = candidate.Reason == ""
	return candidate, nil
}

func evaluateFileCandidateReason(dataset *datasetModel.Dataset, asset *datalibModel.DocumentAsset, alreadyAdded bool, chunkCount int64, embeddingCount int64) string {
	if alreadyAdded {
		return FileCandidateReasonAlreadyAdded
	}
	if asset.ProductStatus != datalibModel.DocumentAssetProductStatusReady || asset.VectorStatus != datalibModel.DocumentAssetVectorStatusReady {
		return FileCandidateReasonNotReady
	}
	if chunkCount == 0 {
		return FileCandidateReasonMissingChunks
	}
	if embeddingCount < chunkCount {
		return FileCandidateReasonMissingEmbedding
	}
	if dataset.EmbeddingModelProvider != nil && asset.EmbeddingProvider != nil && *dataset.EmbeddingModelProvider != *asset.EmbeddingProvider {
		return FileCandidateReasonEmbeddingModelMismatch
	}
	if dataset.EmbeddingModel != nil && asset.EmbeddingModel != nil && *dataset.EmbeddingModel != *asset.EmbeddingModel {
		return FileCandidateReasonEmbeddingModelMismatch
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

func candidateMatchesKeyword(file *fileModel.UploadFile, keyword string) bool {
	if file == nil {
		return false
	}
	return strings.Contains(strings.ToLower(file.Name), strings.ToLower(keyword))
}

var ErrDatasetNotFound = errors.New("dataset not found")
