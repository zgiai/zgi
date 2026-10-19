package digitalhuman

import (
	"context"
	"crypto/sha256"
	"errors"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"
	datasetindexing "github.com/zgiai/zgi/api/internal/modules/dataset/indexing"
	datasetmodel "github.com/zgiai/zgi/api/internal/modules/dataset/model"
	datasetservice "github.com/zgiai/zgi/api/internal/modules/dataset/service"
	llmclient "github.com/zgiai/zgi/api/internal/modules/llm/client"
	llmdefaultservice "github.com/zgiai/zgi/api/internal/modules/llm/defaultmodel/service"
	llmruntime "github.com/zgiai/zgi/api/internal/modules/llm/runtime"
	sharedmodel "github.com/zgiai/zgi/api/internal/modules/shared/model"
	"github.com/zgiai/zgi/api/pkg/logger"
	"github.com/zgiai/zgi/api/pkg/vectordb"
	"gorm.io/gorm"
)

// fallbackRecentMemoryLimit bounds the degraded recall used when the vector
// projection cannot serve a query. It deliberately exceeds the retrieval tool's
// maximum top_k so a broken projection still leaves the model enough material.
const fallbackRecentMemoryLimit = 50

// projectionRebuildLockShards bounds the per-subject rebuild lock table. Recall
// and the operator repair both rewrite the same derived dataset, so they must not
// interleave; sharding keeps that serialization without tracking one lock object
// per DigitalHuman.
const projectionRebuildLockShards = 64

// memoryProjectionScope identifies the DigitalHuman whose hidden projection is
// being built. Recall derives it from a run; operator repair derives it from the
// request scope, so both surfaces can reuse the same projection code.
type memoryProjectionScope struct {
	OrganizationID uuid.UUID
	WorkspaceID    uuid.UUID
	DigitalHumanID uuid.UUID
	ActorID        *uuid.UUID
	PrincipalID    *uuid.UUID
}

func projectionScopeForRun(run *Run) memoryProjectionScope {
	if run == nil {
		return memoryProjectionScope{}
	}
	return memoryProjectionScope{
		OrganizationID: run.OrganizationID,
		WorkspaceID:    run.WorkspaceID,
		DigitalHumanID: run.DigitalHumanID,
		ActorID:        run.ActorID,
		PrincipalID:    run.PrincipalID,
	}
}

// DigitalHumanMemoryProjectionProvider keeps the database memories as the
// source of truth and repairs a private dataset/vector projection just before
// a retrieval. It deliberately has no write path for memory facts.
type DigitalHumanMemoryProjectionProvider struct {
	db              *gorm.DB
	knowledge       *datasetservice.KnowledgeRetrievalService
	llmClient       llmclient.LLMClient
	defaultModelSvc llmdefaultservice.DefaultModelService
	vectorDB        vectordb.VectorDB
	now             func() time.Time
	// rebuildLocks serialize projection rebuilds per DigitalHuman so a recall and
	// a repair, or two repairs, cannot drop vectors the other one already
	// recorded as written.
	rebuildLocks [projectionRebuildLockShards]sync.Mutex
}

// lockProjectionRebuild reserves the projection of one DigitalHuman and returns
// the release function.
func (p *DigitalHumanMemoryProjectionProvider) lockProjectionRebuild(digitalHumanID uuid.UUID) func() {
	lock := &p.rebuildLocks[int(digitalHumanID[0]^digitalHumanID[15])%projectionRebuildLockShards]
	lock.Lock()
	return lock.Unlock
}

// The operator repair action is only reachable while the concrete provider
// satisfies the optional repairer contract.
var _ DigitalHumanMemoryProjectionRepairer = (*DigitalHumanMemoryProjectionProvider)(nil)

func NewDigitalHumanMemoryProjectionProvider(
	db *gorm.DB,
	knowledge *datasetservice.KnowledgeRetrievalService,
	llmClient llmclient.LLMClient,
	defaultModelSvc llmdefaultservice.DefaultModelService,
	vectorDB vectordb.VectorDB,
) *DigitalHumanMemoryProjectionProvider {
	return &DigitalHumanMemoryProjectionProvider{db: db, knowledge: knowledge, llmClient: llmClient, defaultModelSvc: defaultModelSvc, vectorDB: vectorDB, now: func() time.Time { return time.Now().UTC() }}
}

func (p *DigitalHumanMemoryProjectionProvider) Retrieve(ctx context.Context, req DigitalHumanMemoryRetrievalRequest) (*DigitalHumanMemoryRetrievalResult, error) {
	if p == nil || p.db == nil || req.Run == nil {
		return nil, fmt.Errorf("digital human memory retrieval is not configured")
	}
	query := strings.TrimSpace(req.Query)
	if query == "" {
		return nil, fmt.Errorf("query is required")
	}
	if p.knowledge == nil || p.vectorDB == nil {
		return p.fallbackToRecentMemories(ctx, req.Run, "projection backend unavailable", fmt.Errorf("digital human memory projection backend is not configured"))
	}
	scope := projectionScopeForRun(req.Run)
	// Recall repairs the projection as a side effect, so it shares the rebuild
	// lock with the operator repair action.
	releaseProjection := p.lockProjectionRebuild(scope.DigitalHumanID)
	dataset, err := p.ensureProjection(ctx, scope)
	if err != nil {
		releaseProjection()
		return p.fallbackToRecentMemories(ctx, req.Run, "projection unavailable", err)
	}
	memories, err := p.syncProjection(ctx, scope, dataset)
	releaseProjection()
	if err != nil {
		return p.fallbackToRecentMemories(ctx, req.Run, "projection sync failed", err)
	}
	if len(memories) == 0 {
		return &DigitalHumanMemoryRetrievalResult{Status: datasetservice.KnowledgeRetrieveStatusNoResults, Context: "", Warnings: []string{"no active long-term memory matches are available"}}, nil
	}
	topK := req.TopK
	if topK <= 0 || topK > 20 {
		topK = 5
	}
	response, err := p.knowledge.Retrieve(ctx, datasetservice.KnowledgeRetrieveRequest{
		Scope: datasetservice.KnowledgeScope{OrganizationID: req.Run.OrganizationID.String(), WorkspaceID: req.Run.WorkspaceID.String(), AccountID: dataset.CreatedBy},
		Query: query, DatasetIDs: []string{dataset.ID}, AgentBindingGrant: true, AllowRuntimeManagedDataset: true, TopK: topK,
		RetrievalMode:    strings.TrimSpace(req.RetrievalMode),
		ContextSeparator: "\n\n", MaxContextChars: 12000,
	})
	if err != nil {
		return p.fallbackToRecentMemories(ctx, req.Run, "vector recall failed", err)
	}
	return p.authoritativeResult(response, memories), nil
}

// fallbackToRecentMemories keeps recall usable when the vector projection
// cannot serve the query by returning the most recently updated active
// memories. Recency is not relevance, so the result is flagged for the model
// and the underlying cause is written to the operator log rather than into
// model-facing text.
func (p *DigitalHumanMemoryProjectionProvider) fallbackToRecentMemories(ctx context.Context, run *Run, cause string, causeErr error) (*DigitalHumanMemoryRetrievalResult, error) {
	items, err := NewRepository(p.db).ActiveMemories(ctx, Scope{OrganizationID: run.OrganizationID, WorkspaceID: run.WorkspaceID}, run.DigitalHumanID, fallbackRecentMemoryLimit, nil)
	if err != nil {
		return nil, fmt.Errorf("digital human memory recall failed (%s): %v: %w", cause, causeErr, err)
	}
	logger.WarnContext(ctx, "digital human memory recall degraded to recent memories",
		"cause", cause, "digital_human_id", run.DigitalHumanID.String(), "fallback_items", len(items), causeErr)
	return recentMemoryResult(items, cause), nil
}

// recentMemoryResult projects a recency-ordered memory list into the same shape
// as a scored recall. Score fields stay empty because recency carries no
// relevance signal.
func recentMemoryResult(items []*DigitalHumanMemory, cause string) *DigitalHumanMemoryRetrievalResult {
	result := &DigitalHumanMemoryRetrievalResult{
		Status:        datasetservice.KnowledgeRetrieveStatusNoResults,
		Items:         make([]*DigitalHumanMemory, 0, len(items)),
		SourceSummary: make([]map[string]any, 0, len(items)),
		ContextBlocks: make([]map[string]any, 0, len(items)),
		Warnings:      []string{fmt.Sprintf("vector recall was unavailable (%s); returned the most recently updated memories instead", cause)},
	}
	contextParts := make([]string, 0, len(items))
	for _, memory := range items {
		if memory == nil || memory.Status != MemoryStatusActive {
			continue
		}
		position := len(result.Items) + 1
		result.Items = append(result.Items, memory)
		contextParts = append(contextParts, fmt.Sprintf("[memory %d] %s", position, memory.Content))
		result.ContextBlocks = append(result.ContextBlocks, map[string]any{"position": position, "source": "digital_human_long_term_memory", "match_type": "recency", "memory_id": memory.ID.String(), "memory_key": memory.MemoryKey, "revision": memory.Revision})
		result.SourceSummary = append(result.SourceSummary, map[string]any{"position": position, "match_type": "recency"})
	}
	result.Context = strings.Join(contextParts, "\n\n")
	result.ResultCount = len(result.Items)
	if result.ResultCount > 0 {
		result.Status = datasetservice.KnowledgeRetrieveStatusSuccess
	}
	return result
}

func (p *DigitalHumanMemoryProjectionProvider) ensureProjection(ctx context.Context, scope memoryProjectionScope) (*datasetmodel.Dataset, error) {
	var state MemoryState
	if err := p.db.WithContext(ctx).Where("organization_id = ? AND workspace_id = ? AND digital_human_id = ?", scope.OrganizationID, scope.WorkspaceID, scope.DigitalHumanID).First(&state).Error; err != nil {
		return nil, fmt.Errorf("load DigitalHuman memory state: %w", err)
	}
	if state.MemoryDatasetID != nil && *state.MemoryDatasetID != uuid.Nil {
		if dataset, err := p.projectionDataset(ctx, scope, *state.MemoryDatasetID); err == nil {
			return dataset, nil
		}
	}
	createdBy := projectionAccountID(scope)
	if createdBy == "" {
		var member struct {
			AccountID string `gorm:"column:account_id"`
		}
		if err := p.db.WithContext(ctx).Table("workspace_members").Select("account_id").Where("workspace_id = ?", scope.WorkspaceID).Order("created_at ASC, account_id ASC").Limit(1).Scan(&member).Error; err != nil || strings.TrimSpace(member.AccountID) == "" {
			return nil, fmt.Errorf("DigitalHuman memory projection requires an account actor")
		}
		createdBy = member.AccountID
	}
	dataset := &datasetmodel.Dataset{ID: uuid.New().String(), OrganizationID: scope.OrganizationID.String(), WorkspaceID: scope.WorkspaceID.String(), Name: "DigitalHuman memory projection", Provider: datasetmodel.ProviderDigitalHumanMemory, Permission: string(datasetmodel.DatasetPermissionOnlyMe), CreatedBy: createdBy, Owner: &createdBy, RetrievalConfig: datasetmodel.JSONMap{"search_method": "semantic_search", "score_threshold": 0, "fallback_policy": "vector"}}
	tx := p.db.WithContext(ctx).Begin()
	if tx.Error != nil {
		return nil, tx.Error
	}
	if err := tx.Create(dataset).Error; err != nil {
		tx.Rollback()
		return nil, fmt.Errorf("create hidden DigitalHuman memory dataset: %w", err)
	}
	result := tx.Model(&MemoryState{}).Where("id = ? AND memory_dataset_id IS NULL", state.ID).Update("memory_dataset_id", dataset.ID)
	if result.Error != nil {
		tx.Rollback()
		return nil, fmt.Errorf("associate hidden DigitalHuman memory dataset: %w", result.Error)
	}
	if result.RowsAffected == 0 {
		tx.Rollback()
		var existing MemoryState
		if err := p.db.WithContext(ctx).First(&existing, state.ID).Error; err != nil {
			return nil, fmt.Errorf("load concurrent hidden DigitalHuman memory dataset: %w", err)
		}
		if existing.MemoryDatasetID == nil {
			return nil, fmt.Errorf("hidden DigitalHuman memory dataset association was not persisted")
		}
		// Another writer recorded an association while this call was creating one.
		// Resolve that id once: re-entering would spin forever whenever the
		// recorded projection can no longer be resolved.
		return p.projectionDataset(ctx, scope, *existing.MemoryDatasetID)
	}
	if err := tx.Commit().Error; err != nil {
		return nil, fmt.Errorf("commit hidden DigitalHuman memory dataset: %w", err)
	}
	return dataset, nil
}

// projectionDataset resolves the projection dataset recorded for one
// DigitalHuman. A recorded id that does not resolve is a broken association, so
// it has to surface as an error instead of triggering another rebuild attempt.
func (p *DigitalHumanMemoryProjectionProvider) projectionDataset(ctx context.Context, scope memoryProjectionScope, datasetID uuid.UUID) (*datasetmodel.Dataset, error) {
	var dataset datasetmodel.Dataset
	err := p.db.WithContext(ctx).Where(
		"id = ? AND provider = ? AND organization_id = ? AND workspace_id = ?",
		datasetID, datasetmodel.ProviderDigitalHumanMemory, scope.OrganizationID, scope.WorkspaceID,
	).First(&dataset).Error
	if err != nil {
		return nil, fmt.Errorf("DigitalHuman memory projection dataset %s is unavailable: %w", datasetID, err)
	}
	return &dataset, nil
}

func (p *DigitalHumanMemoryProjectionProvider) syncProjection(ctx context.Context, scope memoryProjectionScope, dataset *datasetmodel.Dataset) (map[uuid.UUID]*DigitalHumanMemory, error) {
	resolved, err := p.resolveProjectionEmbedding(ctx, scope)
	if err != nil {
		return nil, err
	}
	if projectionEmbeddingChanged(dataset, resolved) {
		if err := p.resetProjection(ctx, dataset); err != nil {
			return nil, err
		}
	}
	if err := p.recordProjectionEmbedding(ctx, dataset, resolved); err != nil {
		return nil, err
	}
	if err := p.vectorDB.CreateClass(ctx, datasetmodel.GenCollectionNameByID(dataset.ID), []map[string]interface{}{
		{"name": "text", "dataType": []string{"text"}}, {"name": "doc_id", "dataType": []string{"text"}}, {"name": "document_id", "dataType": []string{"text"}}, {"name": "dataset_id", "dataType": []string{"text"}}, {"name": "doc_hash", "dataType": []string{"text"}},
	}); err != nil && !strings.Contains(strings.ToLower(err.Error()), "already exists") {
		return nil, fmt.Errorf("create DigitalHuman memory vector class: %w", err)
	}
	var active []*DigitalHumanMemory
	if err := p.db.WithContext(ctx).Where("organization_id = ? AND workspace_id = ? AND digital_human_id = ? AND status = ?", scope.OrganizationID, scope.WorkspaceID, scope.DigitalHumanID, MemoryStatusActive).Find(&active).Error; err != nil {
		return nil, fmt.Errorf("load active DigitalHuman memories: %w", err)
	}
	accountID := dataset.CreatedBy
	embeddingService, err := datasetindexing.NewGatewayEmbeddingService(p.llmClient, accountID, dataset.ID, "dataset", resolved.Model, dataset.WorkspaceID)
	if err != nil {
		return nil, err
	}
	var document datasetmodel.Document
	documentID := stableProjectionUUID("document", scope.DigitalHumanID).String()
	if err := p.db.WithContext(ctx).Where("id = ?", documentID).First(&document).Error; errors.Is(err, gorm.ErrRecordNotFound) {
		now := p.now()
		document = datasetmodel.Document{ID: documentID, OrganizationID: dataset.OrganizationID, DatasetID: dataset.ID, Position: 1, DataSourceType: "digital_human_memory", Batch: "runtime", Name: "DigitalHuman long-term memory", CreatedFrom: "digital_human_runtime", CreatedBy: accountID, CreatedAt: now, UpdatedAt: now, CompletedAt: &now, ParsingCompletedAt: &now, CleaningCompletedAt: &now, SplittingCompletedAt: &now, IndexingStatus: "completed", Enabled: true, DocForm: "text_model"}
		if err := p.db.WithContext(ctx).Create(&document).Error; err != nil {
			return nil, fmt.Errorf("create DigitalHuman memory projection document: %w", err)
		}
	} else if err != nil {
		return nil, err
	}
	activeByID := make(map[uuid.UUID]*DigitalHumanMemory, len(active))
	className := datasetmodel.GenCollectionNameByID(dataset.ID)
	for _, memory := range active {
		if memory == nil {
			continue
		}
		activeByID[memory.ID] = memory
		// Dataset retrieval resolves a vector hit back to its segment through
		// index_node_id, and the vector's doc_id must carry that same key. One
		// memory is one index node, keyed by the memory id, so the projected row
		// and its vector always agree.
		indexNodeID := memory.ID.String()
		var segment datasetmodel.DocumentSegment
		err := p.db.WithContext(ctx).Where("id = ? AND dataset_id = ?", memory.ID, dataset.ID).First(&segment).Error
		if errors.Is(err, gorm.ErrRecordNotFound) {
			// A freshly created segment has not been indexed yet, so its hash is
			// left empty. The store gate below keys off this hash; pre-filling it
			// with the final digest would skip the initial vector write and leave
			// the projection permanently empty.
			segment = datasetmodel.DocumentSegment{ID: memory.ID.String(), OrganizationID: dataset.OrganizationID, DatasetID: dataset.ID, DocumentID: document.ID, Position: len(activeByID), Content: memory.Content, WordCount: len([]rune(memory.Content)), Tokens: len([]rune(memory.Content)), IndexNodeID: &indexNodeID, IndexNodeHash: new(string), HitCount: 0, Enabled: true, Status: "completed", CreatedBy: accountID, CreatedAt: p.now(), CompletedAt: timePtr(p.now()), UpdatedAt: p.now()}
			if err := p.db.WithContext(ctx).Create(&segment).Error; err != nil {
				return nil, fmt.Errorf("create DigitalHuman memory segment: %w", err)
			}
		} else if err != nil {
			return nil, err
		}
		// The index-node check keeps this gate honest for projections written
		// before the key existed: without it they would never be repaired and
		// every hit would keep resolving to nothing.
		if segment.Content != memory.Content || segment.IndexNodeHash == nil || *segment.IndexNodeHash != memory.ContentDigest || !segment.Enabled || segment.IndexNodeID == nil || *segment.IndexNodeID != indexNodeID {
			vector, err := embeddingService.EmbedText(ctx, memory.Content)
			if err != nil {
				return nil, fmt.Errorf("embed DigitalHuman memory %s: %w", memory.ID, err)
			}
			// The generic vector interface exposes create/delete rather than an
			// upsert operation. Delete first makes retries idempotent.
			_ = p.vectorDB.DeleteVector(ctx, memory.ID.String(), className)
			if err := p.vectorDB.StoreVector(ctx, memory.ID.String(), className, map[string]interface{}{
				"text": memory.Content, "doc_id": indexNodeID, "document_id": document.ID, "dataset_id": dataset.ID, "doc_hash": memory.ContentDigest,
				"digital_human_id": scope.DigitalHumanID.String(), "memory_id": memory.ID.String(), "memory_revision": memory.Revision,
				"content_digest": memory.ContentDigest, "memory_epoch": memory.MemoryEpoch, "memory_class": string(memory.MemoryClass), "memory_type": string(memory.MemoryType),
				"scope_type": string(memory.ScopeType), "scope_id": memory.ScopeID, "visibility": string(memory.Visibility), "status": string(memory.Status),
			}, vector); err != nil {
				return nil, fmt.Errorf("store DigitalHuman memory vector %s: %w", memory.ID, err)
			}
			if err := p.db.WithContext(ctx).Model(&datasetmodel.DocumentSegment{}).Where("id = ?", memory.ID).Updates(map[string]interface{}{"content": memory.Content, "index_node_id": indexNodeID, "index_node_hash": memory.ContentDigest, "enabled": true, "status": "completed", "updated_at": p.now(), "completed_at": p.now()}).Error; err != nil {
				return nil, err
			}
		}
	}
	var stale []datasetmodel.DocumentSegment
	if err := p.db.WithContext(ctx).Where("dataset_id = ? AND enabled = ?", dataset.ID, true).Find(&stale).Error; err != nil {
		return nil, err
	}
	for _, segment := range stale {
		memoryID, err := uuid.Parse(segment.ID)
		if err != nil {
			continue
		}
		if _, ok := activeByID[memoryID]; ok {
			continue
		}
		_ = p.vectorDB.DeleteVector(ctx, segment.ID, className)
		_ = p.db.WithContext(ctx).Model(&datasetmodel.DocumentSegment{}).Where("id = ?", segment.ID).Updates(map[string]interface{}{"enabled": false, "disabled_at": p.now(), "status": "completed", "updated_at": p.now()}).Error
	}
	return activeByID, nil
}

// Repair drops the hidden projection and rebuilds it from the database
// memories. Memory rows are the source of truth, so a repair only discards
// derived data: it is the operator escape hatch for a projection built by an
// older vector schema or left behind by a broken vector store.
func (p *DigitalHumanMemoryProjectionProvider) Repair(ctx context.Context, scope memoryProjectionScope) (*MemoryProjectionRepairResult, error) {
	if p == nil || p.db == nil || p.vectorDB == nil || scope.DigitalHumanID == uuid.Nil {
		return nil, fmt.Errorf("digital human memory projection is not configured")
	}
	// Dropping the vector class while another rebuild or recall is writing it
	// would leave the projection marked as written but partially empty.
	releaseProjection := p.lockProjectionRebuild(scope.DigitalHumanID)
	defer releaseProjection()
	dataset, err := p.ensureProjection(ctx, scope)
	if err != nil {
		return nil, err
	}
	if err := p.resetProjection(ctx, dataset); err != nil {
		return nil, err
	}
	active, err := p.syncProjection(ctx, scope, dataset)
	if err != nil {
		return nil, err
	}
	return &MemoryProjectionRepairResult{
		Status:            MemoryProjectionRepairStatusRepaired,
		MemoryCount:       len(active),
		EmbeddingProvider: derefString(dataset.EmbeddingModelProvider),
		EmbeddingModel:    derefString(dataset.EmbeddingModel),
	}, nil
}

// resetProjection drops the vector class and unindexes every segment so the
// following sync rebuilds the projection from scratch. Memories themselves are
// never touched.
func (p *DigitalHumanMemoryProjectionProvider) resetProjection(ctx context.Context, dataset *datasetmodel.Dataset) error {
	if classDeleter, ok := p.vectorDB.(interface {
		DeleteClass(context.Context, string) error
	}); ok {
		if err := classDeleter.DeleteClass(ctx, datasetmodel.GenCollectionNameByID(dataset.ID)); err != nil {
			return fmt.Errorf("drop DigitalHuman memory vector class: %w", err)
		}
	}
	if err := p.db.WithContext(ctx).Model(&datasetmodel.DocumentSegment{}).Where("dataset_id = ?", dataset.ID).Update("index_node_hash", nil).Error; err != nil {
		return fmt.Errorf("unindex DigitalHuman memory projection segments: %w", err)
	}
	dataset.EmbeddingModel = nil
	dataset.EmbeddingModelProvider = nil
	return nil
}

// resolveProjectionEmbedding always follows the current default embedding model
// of the organization. The projection is runtime-managed infrastructure, so it
// is never pinned to a per-dataset model choice, and a default change is
// detected below instead of silently mixing vector spaces.
func (p *DigitalHumanMemoryProjectionProvider) resolveProjectionEmbedding(ctx context.Context, scope memoryProjectionScope) (*llmruntime.ResolvedModel, error) {
	resolved, err := llmruntime.NewModelResolver(p.defaultModelSvc).ResolveDefault(ctx, scope.OrganizationID.String(), sharedmodel.ModelTypeEmbedding)
	if err != nil {
		return nil, fmt.Errorf("resolve DigitalHuman memory embedding model: %w", err)
	}
	if resolved == nil || strings.TrimSpace(resolved.Model) == "" {
		return nil, fmt.Errorf("no default embedding model is available for the DigitalHuman memory projection")
	}
	return resolved, nil
}

// recordProjectionEmbedding stores the model that produced the current vectors
// so a later default-model change can be detected and rebuilt.
func (p *DigitalHumanMemoryProjectionProvider) recordProjectionEmbedding(ctx context.Context, dataset *datasetmodel.Dataset, resolved *llmruntime.ResolvedModel) error {
	provider := strings.TrimSpace(resolved.Provider)
	model := strings.TrimSpace(resolved.Model)
	if derefString(dataset.EmbeddingModelProvider) == provider && derefString(dataset.EmbeddingModel) == model {
		return nil
	}
	err := p.db.WithContext(ctx).Model(&datasetmodel.Dataset{}).Where("id = ?", dataset.ID).
		Updates(map[string]interface{}{"embedding_model_provider": provider, "embedding_model": model, "updated_at": p.now()}).Error
	if err != nil {
		return fmt.Errorf("record DigitalHuman memory projection embedding model: %w", err)
	}
	dataset.EmbeddingModelProvider, dataset.EmbeddingModel = &provider, &model
	return nil
}

// projectionEmbeddingChanged reports whether the stored vectors were produced by
// a different embedding model than the current default. Vectors from another
// model live in another vector space and must be rebuilt instead of reused; an
// unrecorded model means nothing has been projected yet, so there is nothing to
// rebuild.
func projectionEmbeddingChanged(dataset *datasetmodel.Dataset, resolved *llmruntime.ResolvedModel) bool {
	if dataset == nil || resolved == nil {
		return false
	}
	recordedModel := strings.TrimSpace(derefString(dataset.EmbeddingModel))
	if recordedModel == "" {
		return false
	}
	if !strings.EqualFold(recordedModel, strings.TrimSpace(resolved.Model)) {
		return true
	}
	recordedProvider := strings.TrimSpace(derefString(dataset.EmbeddingModelProvider))
	resolvedProvider := strings.TrimSpace(resolved.Provider)
	if recordedProvider == "" || resolvedProvider == "" {
		return false
	}
	return !strings.EqualFold(recordedProvider, resolvedProvider)
}

func (p *DigitalHumanMemoryProjectionProvider) authoritativeResult(response *datasetservice.KnowledgeRetrieveResponse, memories map[uuid.UUID]*DigitalHumanMemory) *DigitalHumanMemoryRetrievalResult {
	if response == nil {
		return &DigitalHumanMemoryRetrievalResult{Status: datasetservice.KnowledgeRetrieveStatusNoResults}
	}
	items := make([]*DigitalHumanMemory, 0, len(response.Resources))
	blocks := make([]map[string]any, 0, len(response.Resources))
	sources := make([]map[string]any, 0, len(response.Resources))
	var contextParts []string
	for _, resource := range response.Resources {
		memoryID, err := uuid.Parse(resource.SegmentID)
		if err != nil {
			continue
		}
		memory := memories[memoryID]
		// Convergence: a DigitalHuman currently owns a single self memory store, so
		// every active memory is recallable regardless of class or business scope.
		// Scope partitioning is deferred as a future extension; legacy business rows
		// stay visible instead of being migrated or hidden.
		if memory == nil || memory.Status != MemoryStatusActive {
			continue
		}
		items = append(items, memory)
		position := len(items)
		contextParts = append(contextParts, fmt.Sprintf("[memory %d] %s", position, memory.Content))
		blocks = append(blocks, map[string]any{"position": position, "source": "digital_human_long_term_memory", "score": resource.Score, "memory_id": memory.ID.String(), "memory_key": memory.MemoryKey, "revision": memory.Revision})
		sources = append(sources, map[string]any{"position": position, "match_type": resource.MatchType, "score": resource.Score})
	}
	result := &DigitalHumanMemoryRetrievalResult{Status: datasetservice.KnowledgeRetrieveStatusNoResults, Context: strings.Join(contextParts, "\n\n"), ResultCount: len(items), SourceSummary: sources, ContextBlocks: blocks, Items: items}
	if len(items) > 0 {
		result.Status = datasetservice.KnowledgeRetrieveStatusSuccess
		if score, ok := sources[0]["score"].(float64); ok {
			result.TopScore = &score
		}
	}
	return result
}

func projectionAccountID(scope memoryProjectionScope) string {
	if scope.ActorID != nil && *scope.ActorID != uuid.Nil {
		return scope.ActorID.String()
	}
	if scope.PrincipalID != nil && *scope.PrincipalID != uuid.Nil {
		return scope.PrincipalID.String()
	}
	return ""
}

func derefString(value *string) string {
	if value == nil {
		return ""
	}
	return strings.TrimSpace(*value)
}

func stableProjectionUUID(kind string, id uuid.UUID) uuid.UUID {
	digest := sha256.Sum256([]byte(kind + ":" + id.String()))
	var out uuid.UUID
	copy(out[:], digest[:16])
	out[6] = (out[6] & 0x0f) | 0x50
	out[8] = (out[8] & 0x3f) | 0x80
	return out
}

func timePtr(value time.Time) *time.Time { return &value }
