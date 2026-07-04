package service

import (
	"context"
	"errors"
	"time"

	"github.com/google/uuid"
	"github.com/zgiai/zgi/api/internal/modules/datalibrary/model"
	"github.com/zgiai/zgi/api/internal/modules/datalibrary/repository"
)

var (
	ErrOrganizationIDRequired = errors.New("organization_id is required")
	ErrSourceFileIDRequired   = errors.New("source_file_id is required")
	ErrAssetIDRequired        = errors.New("asset_id is required")
	ErrContentHashRequired    = errors.New("content_hash is required")
)

type DocumentAssetService interface {
	CreateAsset(ctx context.Context, item *model.DocumentAsset) error
	GetAssetByID(ctx context.Context, id uuid.UUID) (*model.DocumentAsset, error)
	GetAssetViewByID(ctx context.Context, id uuid.UUID) (*DocumentAssetView, error)
	FindAssetBySourceFileID(ctx context.Context, organizationID string, sourceFileID string) (*model.DocumentAsset, error)
	ListAssets(ctx context.Context, filter repository.DocumentAssetListFilter) ([]*model.DocumentAsset, int64, error)
	ListAssetViews(ctx context.Context, filter repository.DocumentAssetListFilter) ([]*DocumentAssetView, int64, error)
	ListReuseEvents(ctx context.Context, filter repository.ReuseEventListFilter) ([]*ReuseEventView, int64, error)

	CreateVersion(ctx context.Context, item *model.DocumentVersion) error
	GetVersionByID(ctx context.Context, id uuid.UUID) (*model.DocumentVersion, error)
	ListVersionsByAssetID(ctx context.Context, assetID uuid.UUID) ([]*model.DocumentVersion, error)
}

type DocumentAssetView struct {
	ID                uuid.UUID              `json:"id"`
	OrganizationID    string                 `json:"organization_id"`
	WorkspaceID       *string                `json:"workspace_id,omitempty"`
	Title             string                 `json:"title"`
	SourceFileID      string                 `json:"source_file_id"`
	CurrentVersionID  *uuid.UUID             `json:"current_version_id,omitempty"`
	ContentHash       string                 `json:"content_hash,omitempty"`
	Status            string                 `json:"status"`
	ProcessingLevel   string                 `json:"processing_level"`
	QualityScore      *float64               `json:"quality_score,omitempty"`
	CurrentVersion    *DocumentVersionView   `json:"current_version,omitempty"`
	ArtifactState     DocumentArtifactView   `json:"artifact_state"`
	ReuseSummary      ReuseSummaryView       `json:"reuse_summary"`
	DownstreamSummary DownstreamSummaryView  `json:"downstream_summary"`
	ProcessingSummary ProcessingSummaryView  `json:"processing_summary"`
	LatestProcessing  *ProcessingRequestView `json:"latest_processing_request,omitempty"`
	CreatedAt         time.Time              `json:"created_at"`
	UpdatedAt         time.Time              `json:"updated_at"`
}

type DocumentVersionView struct {
	ID                 uuid.UUID  `json:"id"`
	AssetID            uuid.UUID  `json:"asset_id"`
	VersionNo          int        `json:"version_no"`
	SourceFileID       string     `json:"source_file_id"`
	ContentHash        string     `json:"content_hash,omitempty"`
	FileName           string     `json:"file_name,omitempty"`
	FileSize           int64      `json:"file_size,omitempty"`
	MimeType           string     `json:"mime_type,omitempty"`
	ParseArtifactID    *uuid.UUID `json:"parse_artifact_id,omitempty"`
	ChunkArtifactSetID *uuid.UUID `json:"chunk_artifact_set_id,omitempty"`
	Status             string     `json:"status"`
	QualityScore       *float64   `json:"quality_score,omitempty"`
	CreatedAt          time.Time  `json:"created_at"`
}

type DocumentArtifactView struct {
	HasParseArtifact      bool                    `json:"has_parse_artifact"`
	HasChunkArtifactSet   bool                    `json:"has_chunk_artifact_set"`
	HasVectorArtifact     bool                    `json:"has_vector_artifact"`
	HasExtractionArtifact bool                    `json:"has_extraction_artifact"`
	ParseArtifactID       *uuid.UUID              `json:"parse_artifact_id,omitempty"`
	ChunkArtifactSetID    *uuid.UUID              `json:"chunk_artifact_set_id,omitempty"`
	VectorArtifactID      *uuid.UUID              `json:"vector_artifact_id,omitempty"`
	ExtractionArtifactID  *uuid.UUID              `json:"extraction_artifact_id,omitempty"`
	VectorArtifact        *VectorArtifactView     `json:"vector_artifact,omitempty"`
	ExtractionArtifact    *ExtractionArtifactView `json:"extraction_artifact,omitempty"`
}

type VectorArtifactView struct {
	ID                 uuid.UUID      `json:"id"`
	OrganizationID     string         `json:"organization_id"`
	WorkspaceID        *string        `json:"workspace_id,omitempty"`
	AssetID            uuid.UUID      `json:"asset_id"`
	VersionID          uuid.UUID      `json:"version_id"`
	ChunkArtifactSetID uuid.UUID      `json:"chunk_artifact_set_id"`
	EmbeddingProvider  string         `json:"embedding_provider"`
	EmbeddingModel     string         `json:"embedding_model"`
	EmbeddingDimension int            `json:"embedding_dimension"`
	VectorCollection   string         `json:"vector_collection"`
	VectorNamespace    string         `json:"vector_namespace,omitempty"`
	VectorCount        int64          `json:"vector_count"`
	Status             string         `json:"status"`
	ContentHash        string         `json:"content_hash,omitempty"`
	MetadataJSON       map[string]any `json:"metadata_json,omitempty"`
	CreatedAt          time.Time      `json:"created_at"`
	UpdatedAt          time.Time      `json:"updated_at"`
}

type ReuseSummaryView struct {
	ReuseCount      int64 `json:"reuse_count"`
	SavedSeconds    int64 `json:"saved_seconds"`
	SavedCostMicros int64 `json:"saved_cost_micros"`
}

type ProcessingSummaryView struct {
	Total     int64 `json:"total"`
	Planned   int64 `json:"planned"`
	Queued    int64 `json:"queued"`
	Running   int64 `json:"running"`
	Completed int64 `json:"completed"`
	Failed    int64 `json:"failed"`
	Cancelled int64 `json:"cancelled"`
}

type DownstreamSummaryView struct {
	KnowledgeBaseRefCount int64 `json:"knowledge_base_ref_count"`
	DatabaseRefCount      int64 `json:"database_ref_count"`
	TotalRefCount         int64 `json:"total_ref_count"`
}

type ReuseEventView struct {
	ID              uuid.UUID      `json:"id"`
	OrganizationID  string         `json:"organization_id"`
	WorkspaceID     *string        `json:"workspace_id,omitempty"`
	AssetID         uuid.UUID      `json:"asset_id"`
	VersionID       *uuid.UUID     `json:"version_id,omitempty"`
	ArtifactType    string         `json:"artifact_type"`
	ArtifactID      *uuid.UUID     `json:"artifact_id,omitempty"`
	ConsumerType    string         `json:"consumer_type"`
	ConsumerID      string         `json:"consumer_id"`
	ConsumerVersion string         `json:"consumer_version,omitempty"`
	SavedSeconds    int64          `json:"saved_seconds"`
	SavedCostMicros int64          `json:"saved_cost_micros"`
	MetadataJSON    map[string]any `json:"metadata_json,omitempty"`
	CreatedBy       string         `json:"created_by,omitempty"`
	CreatedAt       time.Time      `json:"created_at"`
}

type documentAssetService struct {
	repo                   repository.DocumentAssetRepository
	reuseRepo              repository.ReuseEventRepository
	processingRequestRepo  repository.ProcessingRequestRepository
	vectorArtifactRepo     repository.VectorArtifactRepository
	extractionArtifactRepo repository.ExtractionArtifactRepository
	knowledgeBaseRefRepo   repository.KnowledgeBaseAssetRefRepository
	databaseRefRepo        repository.DatabaseAssetRefRepository
}

func NewDocumentAssetService(repo repository.DocumentAssetRepository, reuseRepos ...repository.ReuseEventRepository) DocumentAssetService {
	var reuseRepo repository.ReuseEventRepository
	if len(reuseRepos) > 0 {
		reuseRepo = reuseRepos[0]
	}
	return &documentAssetService{repo: repo, reuseRepo: reuseRepo}
}

func NewDocumentAssetServiceWithProcessingRequests(repo repository.DocumentAssetRepository, reuseRepo repository.ReuseEventRepository, processingRequestRepo repository.ProcessingRequestRepository) DocumentAssetService {
	return &documentAssetService{
		repo:                  repo,
		reuseRepo:             reuseRepo,
		processingRequestRepo: processingRequestRepo,
	}
}

func NewDocumentAssetServiceWithArtifacts(repo repository.DocumentAssetRepository, reuseRepo repository.ReuseEventRepository, processingRequestRepo repository.ProcessingRequestRepository, vectorArtifactRepo repository.VectorArtifactRepository) DocumentAssetService {
	return &documentAssetService{
		repo:                  repo,
		reuseRepo:             reuseRepo,
		processingRequestRepo: processingRequestRepo,
		vectorArtifactRepo:    vectorArtifactRepo,
	}
}

func NewDocumentAssetServiceWithDownstreamRefs(repo repository.DocumentAssetRepository, reuseRepo repository.ReuseEventRepository, processingRequestRepo repository.ProcessingRequestRepository, vectorArtifactRepo repository.VectorArtifactRepository, knowledgeBaseRefRepo repository.KnowledgeBaseAssetRefRepository, databaseRefRepo repository.DatabaseAssetRefRepository, extractionArtifactRepos ...repository.ExtractionArtifactRepository) DocumentAssetService {
	var extractionArtifactRepo repository.ExtractionArtifactRepository
	if len(extractionArtifactRepos) > 0 {
		extractionArtifactRepo = extractionArtifactRepos[0]
	}
	return &documentAssetService{
		repo:                   repo,
		reuseRepo:              reuseRepo,
		processingRequestRepo:  processingRequestRepo,
		vectorArtifactRepo:     vectorArtifactRepo,
		extractionArtifactRepo: extractionArtifactRepo,
		knowledgeBaseRefRepo:   knowledgeBaseRefRepo,
		databaseRefRepo:        databaseRefRepo,
	}
}

func (s *documentAssetService) CreateAsset(ctx context.Context, item *model.DocumentAsset) error {
	if item == nil || item.OrganizationID == "" {
		return ErrOrganizationIDRequired
	}
	if item.SourceFileID == "" {
		return ErrSourceFileIDRequired
	}
	return s.repo.CreateAsset(ctx, item)
}

func (s *documentAssetService) GetAssetByID(ctx context.Context, id uuid.UUID) (*model.DocumentAsset, error) {
	if id == uuid.Nil {
		return nil, ErrAssetIDRequired
	}
	return s.repo.GetAssetByID(ctx, id)
}

func (s *documentAssetService) GetAssetViewByID(ctx context.Context, id uuid.UUID) (*DocumentAssetView, error) {
	asset, err := s.GetAssetByID(ctx, id)
	if err != nil || asset == nil {
		return nil, err
	}
	version, err := s.currentVersionForAsset(ctx, asset)
	if err != nil {
		return nil, err
	}
	view := newDocumentAssetView(asset, version)
	if err := s.applyReuseSummary(ctx, view); err != nil {
		return nil, err
	}
	if err := s.applyDownstreamSummary(ctx, view); err != nil {
		return nil, err
	}
	if err := s.applyLatestProcessingRequest(ctx, view); err != nil {
		return nil, err
	}
	if err := s.applyProcessingSummary(ctx, view); err != nil {
		return nil, err
	}
	if err := s.applyLatestVectorArtifact(ctx, view); err != nil {
		return nil, err
	}
	if err := s.applyLatestExtractionArtifact(ctx, view); err != nil {
		return nil, err
	}
	return view, nil
}

func (s *documentAssetService) FindAssetBySourceFileID(ctx context.Context, organizationID string, sourceFileID string) (*model.DocumentAsset, error) {
	if organizationID == "" {
		return nil, ErrOrganizationIDRequired
	}
	if sourceFileID == "" {
		return nil, ErrSourceFileIDRequired
	}
	return s.repo.FindAssetBySourceFileID(ctx, organizationID, sourceFileID)
}

func (s *documentAssetService) ListAssets(ctx context.Context, filter repository.DocumentAssetListFilter) ([]*model.DocumentAsset, int64, error) {
	if filter.OrganizationID == "" {
		return nil, 0, ErrOrganizationIDRequired
	}
	return s.repo.ListAssets(ctx, filter)
}

func (s *documentAssetService) ListAssetViews(ctx context.Context, filter repository.DocumentAssetListFilter) ([]*DocumentAssetView, int64, error) {
	assets, total, err := s.ListAssets(ctx, filter)
	if err != nil {
		return nil, 0, err
	}
	views := make([]*DocumentAssetView, 0, len(assets))
	for _, asset := range assets {
		version, err := s.currentVersionForAsset(ctx, asset)
		if err != nil {
			return nil, 0, err
		}
		view := newDocumentAssetView(asset, version)
		if err := s.applyReuseSummary(ctx, view); err != nil {
			return nil, 0, err
		}
		if err := s.applyDownstreamSummary(ctx, view); err != nil {
			return nil, 0, err
		}
		if err := s.applyLatestProcessingRequest(ctx, view); err != nil {
			return nil, 0, err
		}
		if err := s.applyProcessingSummary(ctx, view); err != nil {
			return nil, 0, err
		}
		if err := s.applyLatestVectorArtifact(ctx, view); err != nil {
			return nil, 0, err
		}
		if err := s.applyLatestExtractionArtifact(ctx, view); err != nil {
			return nil, 0, err
		}
		views = append(views, view)
	}
	return views, total, nil
}

func (s *documentAssetService) ListReuseEvents(ctx context.Context, filter repository.ReuseEventListFilter) ([]*ReuseEventView, int64, error) {
	if filter.OrganizationID == "" {
		return nil, 0, ErrOrganizationIDRequired
	}
	if filter.AssetID == nil || *filter.AssetID == uuid.Nil {
		return nil, 0, ErrAssetIDRequired
	}
	if s.reuseRepo == nil {
		return nil, 0, nil
	}
	events, total, err := s.reuseRepo.List(ctx, filter)
	if err != nil {
		return nil, 0, err
	}
	views := make([]*ReuseEventView, 0, len(events))
	for _, event := range events {
		views = append(views, newReuseEventView(event))
	}
	return views, total, nil
}

func (s *documentAssetService) CreateVersion(ctx context.Context, item *model.DocumentVersion) error {
	if item == nil || item.AssetID == uuid.Nil {
		return ErrAssetIDRequired
	}
	if item.SourceFileID == "" {
		return ErrSourceFileIDRequired
	}
	return s.repo.CreateVersion(ctx, item)
}

func (s *documentAssetService) GetVersionByID(ctx context.Context, id uuid.UUID) (*model.DocumentVersion, error) {
	if id == uuid.Nil {
		return nil, ErrAssetIDRequired
	}
	return s.repo.GetVersionByID(ctx, id)
}

func (s *documentAssetService) ListVersionsByAssetID(ctx context.Context, assetID uuid.UUID) ([]*model.DocumentVersion, error) {
	if assetID == uuid.Nil {
		return nil, ErrAssetIDRequired
	}
	return s.repo.ListVersionsByAssetID(ctx, assetID)
}

func (s *documentAssetService) currentVersionForAsset(ctx context.Context, asset *model.DocumentAsset) (*model.DocumentVersion, error) {
	if asset == nil {
		return nil, nil
	}
	if asset.CurrentVersionID != nil {
		return s.repo.GetVersionByID(ctx, *asset.CurrentVersionID)
	}
	versions, err := s.repo.ListVersionsByAssetID(ctx, asset.ID)
	if err != nil || len(versions) == 0 {
		return nil, err
	}
	return versions[0], nil
}

func newDocumentAssetView(asset *model.DocumentAsset, version *model.DocumentVersion) *DocumentAssetView {
	if asset == nil {
		return nil
	}
	view := &DocumentAssetView{
		ID:               asset.ID,
		OrganizationID:   asset.OrganizationID,
		WorkspaceID:      asset.WorkspaceID,
		Title:            asset.Title,
		SourceFileID:     asset.SourceFileID,
		CurrentVersionID: asset.CurrentVersionID,
		ContentHash:      asset.ContentHash,
		Status:           asset.Status,
		ProcessingLevel:  asset.ProcessingLevel,
		QualityScore:     asset.QualityScore,
		CreatedAt:        asset.CreatedAt,
		UpdatedAt:        asset.UpdatedAt,
	}
	if version == nil {
		return view
	}
	view.CurrentVersion = &DocumentVersionView{
		ID:                 version.ID,
		AssetID:            version.AssetID,
		VersionNo:          version.VersionNo,
		SourceFileID:       version.SourceFileID,
		ContentHash:        version.ContentHash,
		FileName:           version.FileName,
		FileSize:           version.FileSize,
		MimeType:           version.MimeType,
		ParseArtifactID:    version.ParseArtifactID,
		ChunkArtifactSetID: version.ChunkArtifactSetID,
		Status:             version.Status,
		QualityScore:       version.QualityScore,
		CreatedAt:          version.CreatedAt,
	}
	view.ArtifactState = DocumentArtifactView{
		HasParseArtifact:    version.ParseArtifactID != nil,
		HasChunkArtifactSet: version.ChunkArtifactSetID != nil,
		ParseArtifactID:     version.ParseArtifactID,
		ChunkArtifactSetID:  version.ChunkArtifactSetID,
	}
	return view
}

func (s *documentAssetService) applyReuseSummary(ctx context.Context, view *DocumentAssetView) error {
	if view == nil || s.reuseRepo == nil {
		return nil
	}
	reuseCount, savedSeconds, savedCostMicros, err := s.reuseRepo.SummaryByAssetID(ctx, view.OrganizationID, view.ID)
	if err != nil {
		return err
	}
	view.ReuseSummary = ReuseSummaryView{
		ReuseCount:      reuseCount,
		SavedSeconds:    savedSeconds,
		SavedCostMicros: savedCostMicros,
	}
	return nil
}

func (s *documentAssetService) applyDownstreamSummary(ctx context.Context, view *DocumentAssetView) error {
	if view == nil {
		return nil
	}
	if s.knowledgeBaseRefRepo != nil {
		count, err := s.knowledgeBaseRefRepo.CountActiveByAssetID(ctx, view.OrganizationID, view.ID)
		if err != nil {
			return err
		}
		view.DownstreamSummary.KnowledgeBaseRefCount = count
	}
	if s.databaseRefRepo != nil {
		count, err := s.databaseRefRepo.CountActiveByAssetID(ctx, view.OrganizationID, view.ID)
		if err != nil {
			return err
		}
		view.DownstreamSummary.DatabaseRefCount = count
	}
	view.DownstreamSummary.TotalRefCount = view.DownstreamSummary.KnowledgeBaseRefCount + view.DownstreamSummary.DatabaseRefCount
	return nil
}

func (s *documentAssetService) applyLatestVectorArtifact(ctx context.Context, view *DocumentAssetView) error {
	if view == nil || view.CurrentVersion == nil || s.vectorArtifactRepo == nil {
		return nil
	}
	item, err := s.vectorArtifactRepo.LatestReadyByVersionID(ctx, view.OrganizationID, view.CurrentVersion.ID)
	if err != nil {
		return err
	}
	if item == nil {
		return nil
	}
	artifactView := newVectorArtifactView(item)
	view.ArtifactState.HasVectorArtifact = true
	view.ArtifactState.VectorArtifactID = &item.ID
	view.ArtifactState.VectorArtifact = artifactView
	return nil
}

func (s *documentAssetService) applyLatestExtractionArtifact(ctx context.Context, view *DocumentAssetView) error {
	if view == nil || view.CurrentVersion == nil || s.extractionArtifactRepo == nil {
		return nil
	}
	item, err := s.extractionArtifactRepo.LatestReadyByVersionID(ctx, view.OrganizationID, view.CurrentVersion.ID)
	if err != nil {
		return err
	}
	if item == nil {
		return nil
	}
	artifactView := newExtractionArtifactView(item)
	view.ArtifactState.HasExtractionArtifact = true
	view.ArtifactState.ExtractionArtifactID = &item.ID
	view.ArtifactState.ExtractionArtifact = artifactView
	return nil
}

func (s *documentAssetService) applyLatestProcessingRequest(ctx context.Context, view *DocumentAssetView) error {
	if view == nil || s.processingRequestRepo == nil {
		return nil
	}
	items, _, err := s.processingRequestRepo.List(ctx, repository.ProcessingRequestListFilter{
		OrganizationID: view.OrganizationID,
		AssetID:        view.ID,
		Limit:          1,
	})
	if err != nil {
		return err
	}
	if len(items) > 0 {
		view.LatestProcessing = newProcessingRequestView(items[0])
	}
	return nil
}

func (s *documentAssetService) applyProcessingSummary(ctx context.Context, view *DocumentAssetView) error {
	if view == nil || s.processingRequestRepo == nil {
		return nil
	}
	summaries, err := s.processingRequestRepo.StatusSummaryByAssetID(ctx, view.OrganizationID, view.ID)
	if err != nil {
		return err
	}
	for _, summary := range summaries {
		view.ProcessingSummary.Total += summary.Count
		switch summary.Status {
		case model.ProcessingRequestStatusPlanned:
			view.ProcessingSummary.Planned = summary.Count
		case model.ProcessingRequestStatusQueued:
			view.ProcessingSummary.Queued = summary.Count
		case model.ProcessingRequestStatusRunning:
			view.ProcessingSummary.Running = summary.Count
		case model.ProcessingRequestStatusCompleted:
			view.ProcessingSummary.Completed = summary.Count
		case model.ProcessingRequestStatusFailed:
			view.ProcessingSummary.Failed = summary.Count
		case model.ProcessingRequestStatusCancelled:
			view.ProcessingSummary.Cancelled = summary.Count
		}
	}
	return nil
}

func newVectorArtifactView(item *model.VectorArtifact) *VectorArtifactView {
	if item == nil {
		return nil
	}
	return &VectorArtifactView{
		ID:                 item.ID,
		OrganizationID:     item.OrganizationID,
		WorkspaceID:        item.WorkspaceID,
		AssetID:            item.AssetID,
		VersionID:          item.VersionID,
		ChunkArtifactSetID: item.ChunkArtifactSetID,
		EmbeddingProvider:  item.EmbeddingProvider,
		EmbeddingModel:     item.EmbeddingModel,
		EmbeddingDimension: item.EmbeddingDimension,
		VectorCollection:   item.VectorCollection,
		VectorNamespace:    item.VectorNamespace,
		VectorCount:        item.VectorCount,
		Status:             item.Status,
		ContentHash:        item.ContentHash,
		MetadataJSON:       item.MetadataJSON,
		CreatedAt:          item.CreatedAt,
		UpdatedAt:          item.UpdatedAt,
	}
}

func newReuseEventView(event *model.ReuseEvent) *ReuseEventView {
	if event == nil {
		return nil
	}
	return &ReuseEventView{
		ID:              event.ID,
		OrganizationID:  event.OrganizationID,
		WorkspaceID:     event.WorkspaceID,
		AssetID:         event.AssetID,
		VersionID:       event.VersionID,
		ArtifactType:    event.ArtifactType,
		ArtifactID:      event.ArtifactID,
		ConsumerType:    event.ConsumerType,
		ConsumerID:      event.ConsumerID,
		ConsumerVersion: event.ConsumerVersion,
		SavedSeconds:    event.SavedSeconds,
		SavedCostMicros: event.SavedCostMicros,
		MetadataJSON:    event.MetadataJSON,
		CreatedBy:       event.CreatedBy,
		CreatedAt:       event.CreatedAt,
	}
}
