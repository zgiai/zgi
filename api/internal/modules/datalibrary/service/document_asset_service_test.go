package service

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/zgiai/zgi/api/internal/modules/datalibrary/model"
	"github.com/zgiai/zgi/api/internal/modules/datalibrary/repository"
)

func TestDocumentAssetServiceValidatesRequiredFields(t *testing.T) {
	svc := NewDocumentAssetService(&fakeDocumentAssetRepository{})
	ctx := context.Background()

	if err := svc.CreateAsset(ctx, &model.DocumentAsset{SourceFileID: "file-1"}); !errors.Is(err, ErrOrganizationIDRequired) {
		t.Fatalf("CreateAsset organization error=%v", err)
	}
	if err := svc.CreateAsset(ctx, &model.DocumentAsset{OrganizationID: "org-1"}); !errors.Is(err, ErrSourceFileIDRequired) {
		t.Fatalf("CreateAsset source file error=%v", err)
	}
	if _, err := svc.GetAssetByID(ctx, uuid.Nil); !errors.Is(err, ErrAssetIDRequired) {
		t.Fatalf("GetAssetByID error=%v", err)
	}
	if _, _, err := svc.ListAssets(ctx, repository.DocumentAssetListFilter{}); !errors.Is(err, ErrOrganizationIDRequired) {
		t.Fatalf("ListAssets error=%v", err)
	}
	if err := svc.CreateVersion(ctx, &model.DocumentVersion{SourceFileID: "file-1"}); !errors.Is(err, ErrAssetIDRequired) {
		t.Fatalf("CreateVersion asset error=%v", err)
	}
	if err := svc.CreateVersion(ctx, &model.DocumentVersion{AssetID: uuid.New()}); !errors.Is(err, ErrSourceFileIDRequired) {
		t.Fatalf("CreateVersion source file error=%v", err)
	}
}

func TestDocumentAssetServiceDelegatesToRepository(t *testing.T) {
	repo := &fakeDocumentAssetRepository{
		asset:   &model.DocumentAsset{ID: uuid.New(), OrganizationID: "org-1", SourceFileID: "file-1"},
		version: &model.DocumentVersion{ID: uuid.New(), AssetID: uuid.New(), SourceFileID: "file-1"},
	}
	svc := NewDocumentAssetService(repo)
	ctx := context.Background()

	if err := svc.CreateAsset(ctx, repo.asset); err != nil {
		t.Fatalf("CreateAsset: %v", err)
	}
	if repo.createAssetCalls != 1 {
		t.Fatalf("createAssetCalls=%d", repo.createAssetCalls)
	}

	got, err := svc.FindAssetBySourceFileID(ctx, "org-1", "file-1")
	if err != nil {
		t.Fatalf("FindAssetBySourceFileID: %v", err)
	}
	if got != repo.asset {
		t.Fatalf("asset=%+v", got)
	}

	if err := svc.CreateVersion(ctx, repo.version); err != nil {
		t.Fatalf("CreateVersion: %v", err)
	}
	if repo.createVersionCalls != 1 {
		t.Fatalf("createVersionCalls=%d", repo.createVersionCalls)
	}

	versions, err := svc.ListVersionsByAssetID(ctx, repo.version.AssetID)
	if err != nil {
		t.Fatalf("ListVersionsByAssetID: %v", err)
	}
	if len(versions) != 1 || versions[0] != repo.version {
		t.Fatalf("versions=%+v", versions)
	}
}

func TestDocumentAssetServiceBuildsReadOnlyAssetView(t *testing.T) {
	parseArtifactID := uuid.New()
	chunkArtifactSetID := uuid.New()
	versionID := uuid.New()
	now := time.Now()
	repo := &fakeDocumentAssetRepository{
		asset: &model.DocumentAsset{
			ID:               uuid.New(),
			OrganizationID:   "org-1",
			Title:            "Handbook.pdf",
			SourceFileID:     "file-1",
			CurrentVersionID: &versionID,
			Status:           model.DocumentAssetStatusReady,
			ProcessingLevel:  model.DocumentProcessingLevelSplit,
			CreatedAt:        now,
			UpdatedAt:        now,
		},
		version: &model.DocumentVersion{
			ID:                 versionID,
			AssetID:            uuid.New(),
			VersionNo:          3,
			SourceFileID:       "file-1",
			FileName:           "Handbook.pdf",
			ParseArtifactID:    &parseArtifactID,
			ChunkArtifactSetID: &chunkArtifactSetID,
			Status:             model.DocumentVersionStatusReady,
			CreatedAt:          now,
		},
	}
	repo.version.AssetID = repo.asset.ID
	reuseRepo := &fakeReuseEventRepository{
		reuseCount:      2,
		savedSeconds:    90,
		savedCostMicros: 1500,
	}
	processingRequestID := uuid.New()
	vectorArtifactID := uuid.New()
	extractionArtifactID := uuid.New()
	processingRepo := &fakeProcessingRequestRepository{
		items: []*model.ProcessingRequest{
			{
				ID:             processingRequestID,
				OrganizationID: "org-1",
				AssetID:        repo.asset.ID,
				TargetLevel:    model.DocumentProcessingLevelSplit,
				Status:         model.ProcessingRequestStatusPlanned,
				PlanJSON: map[string]any{
					"will_parse": true,
					"will_split": true,
				},
			},
		},
		total: 1,
		statusSummary: []repository.ProcessingRequestStatusSummary{
			{Status: model.ProcessingRequestStatusPlanned, Count: 1},
			{Status: model.ProcessingRequestStatusQueued, Count: 2},
			{Status: model.ProcessingRequestStatusFailed, Count: 1},
		},
	}
	vectorRepo := &fakeVectorArtifactRepository{
		item: &model.VectorArtifact{
			ID:                 vectorArtifactID,
			OrganizationID:     "org-1",
			AssetID:            repo.asset.ID,
			VersionID:          versionID,
			ChunkArtifactSetID: chunkArtifactSetID,
			EmbeddingProvider:  "openai",
			EmbeddingModel:     "text-embedding-3-large",
			EmbeddingDimension: 3072,
			VectorCollection:   "data_library_vectors",
			VectorNamespace:    "workspace-1",
			VectorCount:        42,
			Status:             model.VectorArtifactStatusReady,
			CreatedAt:          now,
			UpdatedAt:          now,
		},
	}
	extractionRepo := &fakeExtractionArtifactRepository{
		item: &model.ExtractionArtifact{
			ID:                extractionArtifactID,
			OrganizationID:    "org-1",
			AssetID:           repo.asset.ID,
			VersionID:         versionID,
			ParseArtifactID:   &parseArtifactID,
			SchemaName:        "invoice",
			SchemaHash:        "schema-v1",
			ExtractorProvider: "openai",
			ExtractorModel:    "gpt-4.1-mini",
			RecordCount:       9,
			FieldCount:        6,
			EvidenceCount:     12,
			Status:            model.ExtractionArtifactStatusReady,
			CreatedAt:         now,
			UpdatedAt:         now,
		},
	}
	kbRefRepo := &fakeKnowledgeBaseAssetRefRepository{activeCount: 3}
	dbRefRepo := &fakeDatabaseAssetRefRepository{activeCount: 2}
	svc := NewDocumentAssetServiceWithDownstreamRefs(repo, reuseRepo, processingRepo, vectorRepo, kbRefRepo, dbRefRepo, extractionRepo)

	view, err := svc.GetAssetViewByID(context.Background(), repo.asset.ID)
	if err != nil {
		t.Fatalf("GetAssetViewByID: %v", err)
	}
	if view == nil || view.ID != repo.asset.ID || view.CurrentVersion == nil {
		t.Fatalf("view=%+v", view)
	}
	if !view.ArtifactState.HasParseArtifact || !view.ArtifactState.HasChunkArtifactSet {
		t.Fatalf("artifact state=%+v", view.ArtifactState)
	}
	if !view.ArtifactState.HasVectorArtifact ||
		view.ArtifactState.VectorArtifactID == nil ||
		*view.ArtifactState.VectorArtifactID != vectorArtifactID ||
		view.ArtifactState.VectorArtifact == nil ||
		view.ArtifactState.VectorArtifact.VectorCount != 42 {
		t.Fatalf("vector artifact state=%+v", view.ArtifactState)
	}
	if !view.ArtifactState.HasExtractionArtifact ||
		view.ArtifactState.ExtractionArtifactID == nil ||
		*view.ArtifactState.ExtractionArtifactID != extractionArtifactID ||
		view.ArtifactState.ExtractionArtifact == nil ||
		view.ArtifactState.ExtractionArtifact.RecordCount != 9 {
		t.Fatalf("extraction artifact state=%+v", view.ArtifactState)
	}
	if view.CurrentVersion.ParseArtifactID == nil || *view.CurrentVersion.ParseArtifactID != parseArtifactID {
		t.Fatalf("parse artifact=%+v", view.CurrentVersion.ParseArtifactID)
	}
	if view.CurrentVersion.ChunkArtifactSetID == nil || *view.CurrentVersion.ChunkArtifactSetID != chunkArtifactSetID {
		t.Fatalf("chunk artifact=%+v", view.CurrentVersion.ChunkArtifactSetID)
	}
	if view.ReuseSummary.ReuseCount != 2 || view.ReuseSummary.SavedSeconds != 90 || view.ReuseSummary.SavedCostMicros != 1500 {
		t.Fatalf("reuse summary=%+v", view.ReuseSummary)
	}
	if reuseRepo.lastOrganizationID != "org-1" || reuseRepo.lastAssetID != repo.asset.ID {
		t.Fatalf("reuse lookup org=%s asset=%s", reuseRepo.lastOrganizationID, reuseRepo.lastAssetID)
	}
	if view.DownstreamSummary.KnowledgeBaseRefCount != 3 ||
		view.DownstreamSummary.DatabaseRefCount != 2 ||
		view.DownstreamSummary.TotalRefCount != 5 {
		t.Fatalf("downstream summary=%+v", view.DownstreamSummary)
	}
	if view.LatestProcessing == nil ||
		view.LatestProcessing.ID != processingRequestID ||
		view.LatestProcessing.TargetLevel != model.DocumentProcessingLevelSplit ||
		view.LatestProcessing.Plan == nil ||
		!view.LatestProcessing.Plan.WillParse ||
		!view.LatestProcessing.Plan.WillSplit {
		t.Fatalf("latest processing=%+v", view.LatestProcessing)
	}
	if processingRepo.lastFilter.OrganizationID != "org-1" ||
		processingRepo.lastFilter.AssetID != repo.asset.ID ||
		processingRepo.lastFilter.Limit != 1 {
		t.Fatalf("processing filter=%+v", processingRepo.lastFilter)
	}
	if view.ProcessingSummary.Total != 4 ||
		view.ProcessingSummary.Planned != 1 ||
		view.ProcessingSummary.Queued != 2 ||
		view.ProcessingSummary.Failed != 1 {
		t.Fatalf("processing summary=%+v", view.ProcessingSummary)
	}
	if processingRepo.lastSummaryOrganizationID != "org-1" ||
		processingRepo.lastSummaryAssetID != repo.asset.ID {
		t.Fatalf("processing summary lookup org=%s asset=%s", processingRepo.lastSummaryOrganizationID, processingRepo.lastSummaryAssetID)
	}
	if vectorRepo.lastOrganizationID != "org-1" || vectorRepo.lastVersionID != versionID {
		t.Fatalf("vector lookup org=%s version=%s", vectorRepo.lastOrganizationID, vectorRepo.lastVersionID)
	}
	if extractionRepo.lastOrganizationID != "org-1" || extractionRepo.lastVersionID != versionID {
		t.Fatalf("extraction lookup org=%s version=%s", extractionRepo.lastOrganizationID, extractionRepo.lastVersionID)
	}
}

func TestDocumentAssetServiceListsReadOnlyAssetViews(t *testing.T) {
	assetID := uuid.New()
	versionID := uuid.New()
	repo := &fakeDocumentAssetRepository{
		asset: &model.DocumentAsset{
			ID:             assetID,
			OrganizationID: "org-1",
			Title:          "Asset",
			SourceFileID:   "file-1",
		},
		version: &model.DocumentVersion{
			ID:           versionID,
			AssetID:      assetID,
			VersionNo:    1,
			SourceFileID: "file-1",
		},
	}
	svc := NewDocumentAssetService(repo)

	views, total, err := svc.ListAssetViews(context.Background(), repository.DocumentAssetListFilter{OrganizationID: "org-1"})
	if err != nil {
		t.Fatalf("ListAssetViews: %v", err)
	}
	if total != 1 || len(views) != 1 || views[0].CurrentVersion == nil || views[0].CurrentVersion.ID != versionID {
		t.Fatalf("views=%+v total=%d", views, total)
	}
}

func TestDocumentAssetServiceListsReuseEvents(t *testing.T) {
	assetID := uuid.New()
	eventID := uuid.New()
	reuseRepo := &fakeReuseEventRepository{
		events: []*model.ReuseEvent{
			{
				ID:              eventID,
				OrganizationID:  "org-1",
				AssetID:         assetID,
				ArtifactType:    model.ReuseArtifactChunkArtifact,
				ConsumerType:    model.ReuseConsumerKnowledgeBase,
				ConsumerID:      "kb-1",
				SavedSeconds:    30,
				SavedCostMicros: 400,
			},
		},
		total: 1,
	}
	svc := NewDocumentAssetService(&fakeDocumentAssetRepository{}, reuseRepo)

	events, total, err := svc.ListReuseEvents(context.Background(), repository.ReuseEventListFilter{
		OrganizationID: "org-1",
		AssetID:        &assetID,
	})
	if err != nil {
		t.Fatalf("ListReuseEvents: %v", err)
	}
	if total != 1 || len(events) != 1 || events[0].ID != eventID || events[0].ConsumerID != "kb-1" {
		t.Fatalf("events=%+v total=%d", events, total)
	}
	if reuseRepo.lastFilter.OrganizationID != "org-1" || reuseRepo.lastFilter.AssetID == nil || *reuseRepo.lastFilter.AssetID != assetID {
		t.Fatalf("filter=%+v", reuseRepo.lastFilter)
	}
}

type fakeDocumentAssetRepository struct {
	asset                *model.DocumentAsset
	version              *model.DocumentVersion
	assetsBySourceFileID map[string]*model.DocumentAsset
	versionsByID         map[uuid.UUID]*model.DocumentVersion
	createAssetCalls     int
	createVersionCalls   int
	createErr            error
	findSourceFileCalls  int
	skipFirstFind        bool
}

func (r *fakeDocumentAssetRepository) CreateAsset(ctx context.Context, item *model.DocumentAsset) error {
	r.createAssetCalls++
	if r.createErr != nil {
		return r.createErr
	}
	r.asset = item
	return nil
}

func (r *fakeDocumentAssetRepository) CreateAssetWithVersion(ctx context.Context, asset *model.DocumentAsset, version *model.DocumentVersion) error {
	r.createAssetCalls++
	r.createVersionCalls++
	if r.createErr != nil {
		return r.createErr
	}
	r.asset = asset
	r.version = version
	if r.assetsBySourceFileID != nil {
		r.assetsBySourceFileID[asset.SourceFileID] = asset
	}
	if r.versionsByID != nil {
		r.versionsByID[version.ID] = version
	}
	return nil
}

func (r *fakeDocumentAssetRepository) GetAssetByID(ctx context.Context, id uuid.UUID) (*model.DocumentAsset, error) {
	if r.assetsBySourceFileID != nil {
		for _, asset := range r.assetsBySourceFileID {
			if asset.ID == id {
				return asset, nil
			}
		}
	}
	return r.asset, nil
}

func (r *fakeDocumentAssetRepository) FindAssetBySourceFileID(ctx context.Context, organizationID string, sourceFileID string) (*model.DocumentAsset, error) {
	r.findSourceFileCalls++
	if r.skipFirstFind && r.findSourceFileCalls == 1 {
		return nil, nil
	}
	if r.assetsBySourceFileID != nil {
		return r.assetsBySourceFileID[sourceFileID], nil
	}
	return r.asset, nil
}

func (r *fakeDocumentAssetRepository) FindAssetsBySourceFileIDs(ctx context.Context, organizationID string, sourceFileIDs []string) (map[string]*model.DocumentAsset, error) {
	result := make(map[string]*model.DocumentAsset, len(sourceFileIDs))
	if r.assetsBySourceFileID != nil {
		for _, sourceFileID := range sourceFileIDs {
			if asset := r.assetsBySourceFileID[sourceFileID]; asset != nil {
				result[sourceFileID] = asset
			}
		}
		return result, nil
	}
	if r.asset != nil {
		for _, sourceFileID := range sourceFileIDs {
			result[sourceFileID] = r.asset
		}
	}
	return result, nil
}

func (r *fakeDocumentAssetRepository) ListAssets(ctx context.Context, filter repository.DocumentAssetListFilter) ([]*model.DocumentAsset, int64, error) {
	if r.asset == nil {
		return nil, 0, nil
	}
	return []*model.DocumentAsset{r.asset}, 1, nil
}

func (r *fakeDocumentAssetRepository) CreateVersion(ctx context.Context, item *model.DocumentVersion) error {
	r.createVersionCalls++
	r.version = item
	return nil
}

func (r *fakeDocumentAssetRepository) GetVersionByID(ctx context.Context, id uuid.UUID) (*model.DocumentVersion, error) {
	if r.versionsByID != nil {
		return r.versionsByID[id], nil
	}
	return r.version, nil
}

func (r *fakeDocumentAssetRepository) ListVersionsByAssetID(ctx context.Context, assetID uuid.UUID) ([]*model.DocumentVersion, error) {
	if r.version == nil {
		return nil, nil
	}
	return []*model.DocumentVersion{r.version}, nil
}

var _ repository.DocumentAssetRepository = (*fakeDocumentAssetRepository)(nil)

type fakeReuseEventRepository struct {
	reuseCount         int64
	savedSeconds       int64
	savedCostMicros    int64
	events             []*model.ReuseEvent
	total              int64
	lastFilter         repository.ReuseEventListFilter
	created            *model.ReuseEvent
	lastOrganizationID string
	lastAssetID        uuid.UUID
}

func (r *fakeReuseEventRepository) Create(_ context.Context, item *model.ReuseEvent) error {
	r.created = item
	return nil
}

func (r *fakeReuseEventRepository) GetByID(context.Context, uuid.UUID) (*model.ReuseEvent, error) {
	return nil, nil
}

func (r *fakeReuseEventRepository) List(_ context.Context, filter repository.ReuseEventListFilter) ([]*model.ReuseEvent, int64, error) {
	r.lastFilter = filter
	return r.events, r.total, nil
}

func (r *fakeReuseEventRepository) SummaryByAssetID(ctx context.Context, organizationID string, assetID uuid.UUID) (int64, int64, int64, error) {
	r.lastOrganizationID = organizationID
	r.lastAssetID = assetID
	return r.reuseCount, r.savedSeconds, r.savedCostMicros, nil
}

func (r *fakeReuseEventRepository) SumSavingsByAssetID(ctx context.Context, organizationID string, assetID uuid.UUID) (int64, int64, error) {
	r.lastOrganizationID = organizationID
	r.lastAssetID = assetID
	return r.savedSeconds, r.savedCostMicros, nil
}

var _ repository.ReuseEventRepository = (*fakeReuseEventRepository)(nil)

type fakeVectorArtifactRepository struct {
	item               *model.VectorArtifact
	items              []*model.VectorArtifact
	total              int64
	lastFilter         repository.VectorArtifactListFilter
	lastOrganizationID string
	lastVersionID      uuid.UUID
}

func (r *fakeVectorArtifactRepository) Create(context.Context, *model.VectorArtifact) error {
	return nil
}

func (r *fakeVectorArtifactRepository) GetByID(context.Context, uuid.UUID) (*model.VectorArtifact, error) {
	return r.item, nil
}

func (r *fakeVectorArtifactRepository) List(_ context.Context, filter repository.VectorArtifactListFilter) ([]*model.VectorArtifact, int64, error) {
	r.lastFilter = filter
	return r.items, r.total, nil
}

func (r *fakeVectorArtifactRepository) LatestReadyByVersionID(ctx context.Context, organizationID string, versionID uuid.UUID) (*model.VectorArtifact, error) {
	r.lastOrganizationID = organizationID
	r.lastVersionID = versionID
	return r.item, nil
}

var _ repository.VectorArtifactRepository = (*fakeVectorArtifactRepository)(nil)

type fakeExtractionArtifactRepository struct {
	item               *model.ExtractionArtifact
	items              []*model.ExtractionArtifact
	total              int64
	lastFilter         repository.ExtractionArtifactListFilter
	lastOrganizationID string
	lastVersionID      uuid.UUID
}

func (r *fakeExtractionArtifactRepository) Create(context.Context, *model.ExtractionArtifact) error {
	return nil
}

func (r *fakeExtractionArtifactRepository) GetByID(context.Context, uuid.UUID) (*model.ExtractionArtifact, error) {
	return r.item, nil
}

func (r *fakeExtractionArtifactRepository) List(_ context.Context, filter repository.ExtractionArtifactListFilter) ([]*model.ExtractionArtifact, int64, error) {
	r.lastFilter = filter
	return r.items, r.total, nil
}

func (r *fakeExtractionArtifactRepository) LatestReadyByVersionID(ctx context.Context, organizationID string, versionID uuid.UUID) (*model.ExtractionArtifact, error) {
	r.lastOrganizationID = organizationID
	r.lastVersionID = versionID
	return r.item, nil
}

var _ repository.ExtractionArtifactRepository = (*fakeExtractionArtifactRepository)(nil)
