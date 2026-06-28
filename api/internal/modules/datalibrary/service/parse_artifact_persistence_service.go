package service

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"github.com/google/uuid"
	"github.com/zgiai/zgi/api/internal/contracts"
	contentparsemodel "github.com/zgiai/zgi/api/internal/modules/contentparse/model"
	contentparserepo "github.com/zgiai/zgi/api/internal/modules/contentparse/repository"
	contentparseservice "github.com/zgiai/zgi/api/internal/modules/contentparse/service"
	"github.com/zgiai/zgi/api/internal/modules/datalibrary/model"
	"github.com/zgiai/zgi/api/internal/modules/datalibrary/repository"
	"github.com/zgiai/zgi/api/pkg/storage"
)

var (
	ErrParseArtifactRequired      = errors.New("parse artifact is required")
	ErrArtifactStorageRequired    = errors.New("artifact storage is required")
	ErrArtifactStorageKeyRequired = errors.New("artifact storage key is required")
)

type ParseArtifactPersistenceService interface {
	PersistAssetParseArtifact(ctx context.Context, input PersistAssetParseArtifactInput) (*PersistAssetParseArtifactResult, error)
	UpdateAssetParseArtifact(ctx context.Context, input UpdateAssetParseArtifactInput) (*UpdateAssetParseArtifactResult, error)
	LoadParseArtifact(ctx context.Context, storageKey string) (*contracts.ParseArtifact, error)
}

type PersistAssetParseArtifactInput struct {
	OrganizationID    string
	AssetID           uuid.UUID
	ProcessingRunID   uuid.UUID
	GenerationNo      int64
	SourceFileID      string
	SourceContentHash string
	ParseRequest      contracts.ParseRequest
	Artifact          *contracts.ParseArtifact
	Summary           map[string]interface{}
}

type PersistAssetParseArtifactResult struct {
	Asset              *model.DocumentAsset
	Artifact           *contentparsemodel.Artifact
	ArtifactStorageKey string
}

type UpdateAssetParseArtifactInput struct {
	OrganizationID  string
	AssetID         uuid.UUID
	ProcessingRunID uuid.UUID
	GenerationNo    int64
	ArtifactID      uuid.UUID
	Artifact        *contracts.ParseArtifact
	SummaryPatch    map[string]any
}

type UpdateAssetParseArtifactResult struct {
	Asset              *model.DocumentAsset
	Artifact           *contentparsemodel.Artifact
	ArtifactStorageKey string
}

type parseArtifactPersistenceService struct {
	assets    repository.DocumentAssetRepository
	artifacts contentparserepo.ArtifactRepository
	storage   storage.Storage
}

func NewParseArtifactPersistenceService(assets repository.DocumentAssetRepository, artifacts contentparserepo.ArtifactRepository, storage storage.Storage) ParseArtifactPersistenceService {
	return &parseArtifactPersistenceService{
		assets:    assets,
		artifacts: artifacts,
		storage:   storage,
	}
}

func (s *parseArtifactPersistenceService) PersistAssetParseArtifact(ctx context.Context, input PersistAssetParseArtifactInput) (*PersistAssetParseArtifactResult, error) {
	if input.OrganizationID == "" {
		return nil, ErrOrganizationIDRequired
	}
	if input.AssetID == uuid.Nil {
		return nil, ErrAssetIDRequired
	}
	if input.ProcessingRunID == uuid.Nil || input.GenerationNo <= 0 {
		return nil, ErrProcessingRunMismatch
	}
	if input.Artifact == nil {
		return nil, ErrParseArtifactRequired
	}
	if s.storage == nil {
		return nil, ErrArtifactStorageRequired
	}

	asset, err := s.assets.GetAssetByID(ctx, input.AssetID)
	if err != nil {
		return nil, err
	}
	if asset == nil || asset.OrganizationID != input.OrganizationID {
		return nil, ErrDocumentAssetNotFound
	}
	if asset.ProcessingRunID == nil ||
		*asset.ProcessingRunID != input.ProcessingRunID ||
		asset.GenerationNo != input.GenerationNo {
		return nil, ErrProcessingRunMismatch
	}

	item := s.buildArtifactRecord(input)
	input.Artifact.ArtifactID = item.ID.String()
	storageKey := parseArtifactStorageKey(input.OrganizationID, input.AssetID, input.GenerationNo, input.ProcessingRunID, item.ID)
	item.ArtifactStorageKey = storageKey

	payload, err := json.Marshal(input.Artifact)
	if err != nil {
		return nil, err
	}
	if err := s.storage.Save(storageKey, payload); err != nil {
		return nil, err
	}
	if err := s.artifacts.Create(ctx, item); err != nil {
		return nil, err
	}

	updated, err := s.assets.UpdateCurrentResult(ctx, input.AssetID, repository.DocumentAssetCurrentResultPatch{
		OrganizationID:         input.OrganizationID,
		ParseArtifactID:        &item.ID,
		RequireProcessingRunID: &input.ProcessingRunID,
		RequireGenerationNo:    &input.GenerationNo,
	})
	if err != nil {
		return nil, err
	}
	if updated == nil || updated.ParseArtifactID == nil || *updated.ParseArtifactID != item.ID {
		return nil, ErrProcessingRunMismatch
	}

	return &PersistAssetParseArtifactResult{
		Asset:              updated,
		Artifact:           item,
		ArtifactStorageKey: storageKey,
	}, nil
}

func (s *parseArtifactPersistenceService) UpdateAssetParseArtifact(ctx context.Context, input UpdateAssetParseArtifactInput) (*UpdateAssetParseArtifactResult, error) {
	if input.OrganizationID == "" {
		return nil, ErrOrganizationIDRequired
	}
	if input.AssetID == uuid.Nil || input.ArtifactID == uuid.Nil {
		return nil, ErrAssetIDRequired
	}
	if input.ProcessingRunID == uuid.Nil || input.GenerationNo <= 0 {
		return nil, ErrProcessingRunMismatch
	}
	if input.Artifact == nil {
		return nil, ErrParseArtifactRequired
	}
	if s.storage == nil {
		return nil, ErrArtifactStorageRequired
	}
	asset, err := s.assets.GetAssetByID(ctx, input.AssetID)
	if err != nil {
		return nil, err
	}
	if asset == nil || asset.OrganizationID != input.OrganizationID {
		return nil, ErrDocumentAssetNotFound
	}
	if asset.ProcessingRunID == nil ||
		*asset.ProcessingRunID != input.ProcessingRunID ||
		asset.GenerationNo != input.GenerationNo ||
		asset.ParseArtifactID == nil ||
		*asset.ParseArtifactID != input.ArtifactID {
		return nil, ErrProcessingRunMismatch
	}
	item, err := s.artifacts.GetByID(ctx, input.ArtifactID)
	if err != nil {
		return nil, err
	}
	if item == nil {
		return nil, ErrParsePreviewNotReady
	}

	input.Artifact.ArtifactID = item.ID.String()
	storageKey := parseArtifactConfirmedStorageKey(input.OrganizationID, input.AssetID, input.GenerationNo, input.ProcessingRunID, item.ID)
	payload, err := json.Marshal(input.Artifact)
	if err != nil {
		return nil, err
	}
	if err := s.storage.Save(storageKey, payload); err != nil {
		return nil, err
	}
	summary := cloneAnyMap(item.SummaryJSON)
	for key, value := range input.SummaryPatch {
		summary[key] = value
	}
	if err := s.artifacts.UpdateStorageKeyAndSummary(ctx, item.ID, storageKey, summary); err != nil {
		return nil, err
	}
	item.ArtifactStorageKey = storageKey
	item.SummaryJSON = summary

	return &UpdateAssetParseArtifactResult{
		Asset:              asset,
		Artifact:           item,
		ArtifactStorageKey: storageKey,
	}, nil
}

func (s *parseArtifactPersistenceService) LoadParseArtifact(ctx context.Context, storageKey string) (*contracts.ParseArtifact, error) {
	key := strings.TrimSpace(storageKey)
	if key == "" {
		return nil, ErrArtifactStorageKeyRequired
	}
	if s.storage == nil {
		return nil, ErrArtifactStorageRequired
	}
	payload, err := s.storage.Load(key)
	if err != nil {
		return nil, err
	}
	var artifact contracts.ParseArtifact
	if err := json.Unmarshal(payload, &artifact); err != nil {
		return nil, err
	}
	return &artifact, nil
}

func (s *parseArtifactPersistenceService) buildArtifactRecord(input PersistAssetParseArtifactInput) *contentparsemodel.Artifact {
	summary := cloneInterfaceMap(input.Summary)
	if input.SourceContentHash != "" {
		summary["source_content_hash"] = input.SourceContentHash
	}
	build := contentparseservice.BuildParseArtifactItem(contentparseservice.ParseArtifactBuildInput{
		Request:  input.ParseRequest,
		Artifact: input.Artifact,
		Summary:  summary,
	})
	item := build.Item
	if item == nil {
		item = &contentparsemodel.Artifact{}
	}
	if item.ID == uuid.Nil {
		item.ID = uuid.New()
	}
	if item.SourceContentHash == "" {
		item.SourceContentHash = contentparseservice.SHA256Hex(item.ID.String())
	}
	if item.Profile == "" {
		item.Profile = string(contracts.ParseProfileDefault)
	}
	if item.CanonicalIRVersion == "" {
		item.CanonicalIRVersion = "v1"
	}
	baseProviderSignature := item.ProviderSignature
	if baseProviderSignature == "" {
		baseProviderSignature = contentparseservice.ProviderSignature("", input.Artifact.EngineUsed)
	}
	item.ProviderSignature = assetPrivateParseArtifactSignature(baseProviderSignature, input.AssetID, input.GenerationNo, input.ProcessingRunID)
	item.SummaryJSON = cloneAnyMap(item.SummaryJSON)
	item.SummaryJSON["asset_id"] = input.AssetID.String()
	item.SummaryJSON["processing_run_id"] = input.ProcessingRunID.String()
	item.SummaryJSON["generation_no"] = input.GenerationNo
	item.SummaryJSON["source_file_id"] = input.SourceFileID
	item.SummaryJSON["base_provider_signature"] = baseProviderSignature
	return item
}

func parseArtifactStorageKey(organizationID string, assetID uuid.UUID, generationNo int64, processingRunID uuid.UUID, artifactID uuid.UUID) string {
	return fmt.Sprintf(
		"data_library/parse_artifacts/%s/%s/%d/%s/%s.json",
		strings.TrimSpace(organizationID),
		assetID.String(),
		generationNo,
		processingRunID.String(),
		artifactID.String(),
	)
}

func parseArtifactConfirmedStorageKey(organizationID string, assetID uuid.UUID, generationNo int64, processingRunID uuid.UUID, artifactID uuid.UUID) string {
	return fmt.Sprintf(
		"data_library/parse_artifacts/%s/%s/%d/%s/%s-confirmed.json",
		strings.TrimSpace(organizationID),
		assetID.String(),
		generationNo,
		processingRunID.String(),
		artifactID.String(),
	)
}

func assetPrivateParseArtifactSignature(base string, assetID uuid.UUID, generationNo int64, processingRunID uuid.UUID) string {
	hash := contentparseservice.SHA256Hex(fmt.Sprintf("%s|%s|%d|%s", strings.TrimSpace(base), assetID.String(), generationNo, processingRunID.String()))
	return "asset:" + hash[:32]
}

func cloneInterfaceMap(input map[string]interface{}) map[string]interface{} {
	if input == nil {
		return map[string]interface{}{}
	}
	out := make(map[string]interface{}, len(input))
	for key, value := range input {
		out[key] = value
	}
	return out
}

func cloneAnyMap(input map[string]any) map[string]any {
	if input == nil {
		return map[string]any{}
	}
	out := make(map[string]any, len(input))
	for key, value := range input {
		out[key] = value
	}
	return out
}
