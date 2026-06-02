package service

import (
	"context"
	"errors"

	"github.com/google/uuid"
	"github.com/zgiai/zgi/api/internal/modules/datalibrary/model"
	"github.com/zgiai/zgi/api/internal/modules/datalibrary/repository"
)

var (
	ErrDocumentAssetNotFound = errors.New("document asset not found")
	ErrProcessingRunMismatch = errors.New("processing run does not match current asset state")
)

type FileAssetProcessingStateService interface {
	CreateOrReuseStoredAsset(ctx context.Context, input FileAssetCreateInput) (*model.DocumentAsset, bool, error)
	BeginProcessingRequest(ctx context.Context, input BeginProcessingRequestInput) (*BeginProcessingRequestResult, error)
	MarkParsing(ctx context.Context, input RunStateInput) (*model.DocumentAsset, error)
	MarkConfirming(ctx context.Context, input RunStateInput) (*model.DocumentAsset, error)
	MarkGenerating(ctx context.Context, input RunStateInput) (*model.DocumentAsset, error)
	MarkReady(ctx context.Context, input ReadyStateInput) (*model.DocumentAsset, error)
	MarkFailed(ctx context.Context, input FailedStateInput) (*model.DocumentAsset, error)
}

type FileAssetCreateInput struct {
	OrganizationID string
	WorkspaceID    *string
	Title          string
	SourceFileID   string
	ContentHash    string
	CreatedBy      string
}

type BeginProcessingRequestInput struct {
	OrganizationID string
	WorkspaceID    *string
	AssetID        uuid.UUID
	TargetLevel    string
	RequestedBy    string
	Force          bool
	IncrementRun   bool
	Metadata       map[string]any
}

type BeginProcessingRequestResult struct {
	Asset             *model.DocumentAsset
	ProcessingRequest *model.ProcessingRequest
	ProcessingRunID   uuid.UUID
	GenerationNo      int64
}

type RunStateInput struct {
	OrganizationID     string
	AssetID            uuid.UUID
	ProcessingRunID    uuid.UUID
	GenerationNo       int64
	ProcessingStage    string
	ProcessingProgress int
	ParseArtifactID    *uuid.UUID
}

type ReadyStateInput struct {
	RunStateInput
	ChunkArtifactSetID *uuid.UUID
	ChunkCount         int
	EmbeddingProvider  string
	EmbeddingModel     string
	EmbeddingDimension int
}

type FailedStateInput struct {
	RunStateInput
	ErrorCode    string
	ErrorMessage string
}

type fileAssetProcessingStateService struct {
	assets             repository.DocumentAssetRepository
	processingRequests repository.ProcessingRequestRepository
	refs               fileAssetProcessingRefStore
	documents          fileAssetProcessingDocumentStore
}

func NewFileAssetProcessingStateService(assets repository.DocumentAssetRepository, processingRequests repository.ProcessingRequestRepository) FileAssetProcessingStateService {
	return &fileAssetProcessingStateService{
		assets:             assets,
		processingRequests: processingRequests,
	}
}

func NewFileAssetProcessingStateServiceWithDatasetRefs(
	assets repository.DocumentAssetRepository,
	processingRequests repository.ProcessingRequestRepository,
	refs fileAssetProcessingRefStore,
	documents fileAssetProcessingDocumentStore,
) FileAssetProcessingStateService {
	return &fileAssetProcessingStateService{
		assets:             assets,
		processingRequests: processingRequests,
		refs:               refs,
		documents:          documents,
	}
}

type fileAssetProcessingRefStore interface {
	ListActiveByAsset(ctx context.Context, organizationID string, assetID uuid.UUID) ([]*model.KnowledgeBaseAssetRef, error)
	MarkPending(ctx context.Context, organizationID string, id uuid.UUID, syncRunID uuid.UUID, errorCode, errorMessage *string) (*model.KnowledgeBaseAssetRef, error)
}

type fileAssetProcessingDocumentStore interface {
	DisableDocuments(ctx context.Context, datasetID string, documentIDs []string, accountID string) error
}

func (s *fileAssetProcessingStateService) CreateOrReuseStoredAsset(ctx context.Context, input FileAssetCreateInput) (*model.DocumentAsset, bool, error) {
	if input.OrganizationID == "" {
		return nil, false, ErrOrganizationIDRequired
	}
	if input.SourceFileID == "" {
		return nil, false, ErrSourceFileIDRequired
	}
	existing, err := s.assets.FindAssetBySourceFileID(ctx, input.OrganizationID, input.SourceFileID)
	if err != nil || existing != nil {
		return existing, false, err
	}
	asset := &model.DocumentAsset{
		OrganizationID:     input.OrganizationID,
		WorkspaceID:        input.WorkspaceID,
		Title:              input.Title,
		SourceFileID:       input.SourceFileID,
		ContentHash:        input.ContentHash,
		Status:             model.DocumentAssetStatusArchived,
		ProcessingLevel:    model.DocumentProcessingLevelArchive,
		ProductStatus:      model.DocumentAssetProductStatusStoredOnly,
		VectorStatus:       model.DocumentAssetVectorStatusNone,
		ProcessingProgress: 0,
		CreatedBy:          input.CreatedBy,
	}
	if err := s.assets.CreateAsset(ctx, asset); err != nil {
		return nil, false, err
	}
	return asset, true, nil
}

func (s *fileAssetProcessingStateService) BeginProcessingRequest(ctx context.Context, input BeginProcessingRequestInput) (*BeginProcessingRequestResult, error) {
	if input.OrganizationID == "" {
		return nil, ErrOrganizationIDRequired
	}
	if input.AssetID == uuid.Nil {
		return nil, ErrAssetIDRequired
	}
	if input.TargetLevel == "" {
		return nil, ErrProcessingLevelRequired
	}
	if err := ValidateProcessingRequest(ProcessingRequest{
		OrganizationID: input.OrganizationID,
		WorkspaceID:    input.WorkspaceID,
		AssetID:        input.AssetID,
		TargetLevel:    input.TargetLevel,
	}); err != nil {
		return nil, err
	}
	asset, err := s.assets.GetAssetByID(ctx, input.AssetID)
	if err != nil {
		return nil, err
	}
	if asset == nil || asset.OrganizationID != input.OrganizationID {
		return nil, ErrDocumentAssetNotFound
	}

	plan, err := PlanProcessingRequest(ProcessingRequest{
		OrganizationID: input.OrganizationID,
		WorkspaceID:    input.WorkspaceID,
		AssetID:        input.AssetID,
		TargetLevel:    input.TargetLevel,
	})
	if err != nil {
		return nil, err
	}
	request := &model.ProcessingRequest{
		OrganizationID:  input.OrganizationID,
		WorkspaceID:     input.WorkspaceID,
		AssetID:         input.AssetID,
		TargetLevel:     input.TargetLevel,
		Status:          model.ProcessingRequestStatusPlanned,
		RequestedBy:     input.RequestedBy,
		Force:           input.Force,
		PlanJSON:        processingPlanToJSON(plan),
		RequestMetadata: cloneMap(input.Metadata),
	}
	if err := s.processingRequests.Create(ctx, request); err != nil {
		return nil, err
	}

	runID := request.ID
	generationNo := asset.GenerationNo
	if input.IncrementRun || generationNo == 0 {
		generationNo++
	}
	parsing := model.DocumentAssetProductStatusParsing
	stage := model.DocumentAssetProcessingStageParse
	progress := 5
	vectorStatus := model.DocumentAssetVectorStatusNone
	updated, err := s.assets.UpdateCurrentResult(ctx, input.AssetID, repository.DocumentAssetCurrentResultPatch{
		OrganizationID:            input.OrganizationID,
		ProductStatus:             &parsing,
		ProcessingStage:           &stage,
		ProcessingProgress:        &progress,
		ActiveProcessingRequestID: &request.ID,
		ProcessingRunID:           &runID,
		GenerationNo:              &generationNo,
		VectorStatus:              &vectorStatus,
		ClearError:                true,
	})
	if err != nil {
		return nil, err
	}
	if updated == nil || updated.ProcessingRunID == nil || *updated.ProcessingRunID != runID || updated.GenerationNo != generationNo {
		return nil, ErrProcessingRunMismatch
	}
	if err := s.invalidateDatasetRefsForAssetEdit(ctx, updated, input.RequestedBy); err != nil {
		return nil, err
	}
	return &BeginProcessingRequestResult{
		Asset:             updated,
		ProcessingRequest: request,
		ProcessingRunID:   runID,
		GenerationNo:      generationNo,
	}, nil
}

func (s *fileAssetProcessingStateService) MarkParsing(ctx context.Context, input RunStateInput) (*model.DocumentAsset, error) {
	status := model.DocumentAssetProductStatusParsing
	stage := input.ProcessingStage
	if stage == "" {
		stage = model.DocumentAssetProcessingStageParse
	}
	progress := normalizedProgress(input.ProcessingProgress, 10)
	return s.updateRunState(ctx, input, repository.DocumentAssetCurrentResultPatch{
		ProductStatus:      &status,
		ProcessingStage:    &stage,
		ProcessingProgress: &progress,
		ParseArtifactID:    input.ParseArtifactID,
		ClearError:         true,
	})
}

func (s *fileAssetProcessingStateService) MarkConfirming(ctx context.Context, input RunStateInput) (*model.DocumentAsset, error) {
	status := model.DocumentAssetProductStatusConfirming
	stage := model.DocumentAssetProcessingStageReview
	progress := normalizedProgress(input.ProcessingProgress, 35)
	return s.updateRunState(ctx, input, repository.DocumentAssetCurrentResultPatch{
		ProductStatus:      &status,
		ProcessingStage:    &stage,
		ProcessingProgress: &progress,
		ParseArtifactID:    input.ParseArtifactID,
		ClearError:         true,
	})
}

func (s *fileAssetProcessingStateService) MarkGenerating(ctx context.Context, input RunStateInput) (*model.DocumentAsset, error) {
	status := model.DocumentAssetProductStatusGenerating
	stage := input.ProcessingStage
	if stage == "" {
		stage = model.DocumentAssetProcessingStageChunk
	}
	progress := normalizedProgress(input.ProcessingProgress, 45)
	vectorStatus := model.DocumentAssetVectorStatusIndexing
	return s.updateRunState(ctx, input, repository.DocumentAssetCurrentResultPatch{
		ProductStatus:      &status,
		ProcessingStage:    &stage,
		ProcessingProgress: &progress,
		ParseArtifactID:    input.ParseArtifactID,
		VectorStatus:       &vectorStatus,
		ClearError:         true,
	})
}

func (s *fileAssetProcessingStateService) MarkReady(ctx context.Context, input ReadyStateInput) (*model.DocumentAsset, error) {
	status := model.DocumentAssetProductStatusReady
	stage := model.DocumentAssetProcessingStageVectorize
	progress := 100
	vectorStatus := model.DocumentAssetVectorStatusReady
	return s.updateRunState(ctx, input.RunStateInput, repository.DocumentAssetCurrentResultPatch{
		ProductStatus:      &status,
		ProcessingStage:    &stage,
		ProcessingProgress: &progress,
		ParseArtifactID:    input.ParseArtifactID,
		ChunkArtifactSetID: input.ChunkArtifactSetID,
		ChunkCount:         &input.ChunkCount,
		EmbeddingProvider:  &input.EmbeddingProvider,
		EmbeddingModel:     &input.EmbeddingModel,
		EmbeddingDimension: &input.EmbeddingDimension,
		VectorStatus:       &vectorStatus,
		ClearError:         true,
	})
}

func (s *fileAssetProcessingStateService) MarkFailed(ctx context.Context, input FailedStateInput) (*model.DocumentAsset, error) {
	status := model.DocumentAssetProductStatusParseFailed
	stage := input.ProcessingStage
	if stage == "" {
		stage = model.DocumentAssetProcessingStageParse
	}
	vectorStatus := model.DocumentAssetVectorStatusFailed
	progress := normalizedProgress(input.ProcessingProgress, 0)
	return s.updateRunState(ctx, input.RunStateInput, repository.DocumentAssetCurrentResultPatch{
		ProductStatus:      &status,
		ProcessingStage:    &stage,
		ProcessingProgress: &progress,
		VectorStatus:       &vectorStatus,
		LastErrorCode:      &input.ErrorCode,
		LastErrorMessage:   &input.ErrorMessage,
	})
}

func (s *fileAssetProcessingStateService) updateRunState(ctx context.Context, input RunStateInput, patch repository.DocumentAssetCurrentResultPatch) (*model.DocumentAsset, error) {
	if input.OrganizationID == "" {
		return nil, ErrOrganizationIDRequired
	}
	if input.AssetID == uuid.Nil {
		return nil, ErrAssetIDRequired
	}
	if input.ProcessingRunID == uuid.Nil || input.GenerationNo == 0 {
		return nil, ErrProcessingRunMismatch
	}
	patch.OrganizationID = input.OrganizationID
	patch.RequireProcessingRunID = &input.ProcessingRunID
	patch.RequireGenerationNo = &input.GenerationNo
	updated, err := s.assets.UpdateCurrentResult(ctx, input.AssetID, patch)
	if err != nil {
		return nil, err
	}
	if updated == nil || updated.ProcessingRunID == nil || *updated.ProcessingRunID != input.ProcessingRunID || updated.GenerationNo != input.GenerationNo {
		return nil, ErrProcessingRunMismatch
	}
	return updated, nil
}

func (s *fileAssetProcessingStateService) invalidateDatasetRefsForAssetEdit(ctx context.Context, asset *model.DocumentAsset, accountID string) error {
	if s == nil || s.refs == nil || s.documents == nil || asset == nil {
		return nil
	}
	refs, err := s.refs.ListActiveByAsset(ctx, asset.OrganizationID, asset.ID)
	if err != nil {
		return err
	}
	for _, ref := range refs {
		if ref == nil {
			continue
		}
		if ref.DatasetDocumentID != nil && *ref.DatasetDocumentID != uuid.Nil {
			if err := s.documents.DisableDocuments(ctx, ref.DatasetID, []string{ref.DatasetDocumentID.String()}, accountID); err != nil {
				return err
			}
		}
		syncRunID := uuid.New()
		if _, err := s.refs.MarkPending(ctx, asset.OrganizationID, ref.ID, syncRunID, nil, nil); err != nil {
			return err
		}
	}
	return nil
}

func normalizedProgress(value int, fallback int) int {
	if value == 0 {
		value = fallback
	}
	if value < 0 {
		return 0
	}
	if value > 100 {
		return 100
	}
	return value
}
