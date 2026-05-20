package worker

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/google/uuid"
	"github.com/hibiken/asynq"
	"github.com/zgiai/ginext/internal/modules/dataset/graphflow"
	"github.com/zgiai/ginext/internal/modules/dataset/graphflow/extractor"
	"github.com/zgiai/ginext/internal/modules/dataset/graphflow/model"
	dataset_model "github.com/zgiai/ginext/internal/modules/dataset/model"
	"github.com/zgiai/ginext/pkg/logger"
	"github.com/zgiai/ginext/pkg/queue"
	"github.com/zgiai/ginext/pkg/ratelimit"
	"go.uber.org/zap"
)

type datasetOrganizationGetter interface {
	GetByID(ctx context.Context, id string) (*dataset_model.Dataset, error)
}

func resolveExtractionOrganizationID(ctx context.Context, repo datasetOrganizationGetter, graphFlowTask *model.GraphFlowTask) string {
	if graphFlowTask == nil {
		return ""
	}

	if repo != nil {
		dataset, err := repo.GetByID(ctx, graphFlowTask.KBID.String())
		if err == nil && dataset != nil && dataset.OrganizationID != "" {
			return dataset.OrganizationID
		}
	}

	return graphFlowTask.TenantID.String()
}

// NewExtractionHandler creates a handler for extraction tasks.
// All segments are processed concurrently within this single handler using a goroutine pool.
func NewExtractionHandler(svc *graphflow.Service, taskManager *queue.TaskManager, limiter *ratelimit.KBLimiter) func(context.Context, *asynq.Task) error {
	return func(ctx context.Context, t *asynq.Task) error {
		// Parse payload
		var payload GraphFlowTaskPayload
		if err := json.Unmarshal(t.Payload(), &payload); err != nil {
			return fmt.Errorf("json.Unmarshal failed: %v: %w", err, asynq.SkipRetry)
		}

		taskID, err := uuid.Parse(payload.TaskID)
		if err != nil {
			return fmt.Errorf("failed to parse task_id: %v: %w", err, asynq.SkipRetry)
		}
		ctx = logger.WithFields(ctx, zap.String("task_id", taskID.String()))

		// Panic recovery to ensure task status is updated if the worker crashes
		defer func() {
			if r := recover(); r != nil {
				err := fmt.Errorf("extraction worker panicked: %v", r)
				logger.CriticalContext(ctx, "extraction worker panicked", err)
				svc.TaskRepo.UpdateTaskFailed(ctx, taskID, err.Error())
			}
		}()

		logger.Info("Starting GraphFlow extraction", map[string]interface{}{
			"task_id": taskID.String(),
		})

		// 1. Get the GraphFlow task from DB
		graphFlowTask, err := svc.TaskRepo.GetByID(ctx, taskID)
		if err != nil {
			return fmt.Errorf("failed to get graphflow task: %v: %w", err, asynq.SkipRetry)
		}
		if graphFlowTask == nil {
			return fmt.Errorf("graphflow task not found: %s: %w", taskID, asynq.SkipRetry)
		}

		// Check for idempotency - skip if already completed or failed
		if graphFlowTask.Status == "completed" || graphFlowTask.Status == "failed" {
			logger.Info("Task already processed, skipping", map[string]interface{}{
				"task_id": taskID.String(),
				"status":  graphFlowTask.Status,
			})
			return nil
		}

		// Apply Rate Limiting
		kbID := graphFlowTask.KBID.String()
		if allowed, err := limiter.Allow(ctx, kbID); err != nil {
			svc.TaskRepo.UpdateTaskFailed(ctx, taskID, fmt.Sprintf("rate limit check failed: %v", err))
			return fmt.Errorf("rate limit check failed: %w", err)
		} else if !allowed {
			return fmt.Errorf("rate limit exceeded for KB %s, retrying later", kbID)
		}
		defer func() {
			if err := limiter.Release(ctx, kbID); err != nil {
				logger.ErrorContext(ctx, "failed to release rate limit token", "kb_id", kbID, err)
			}
		}()

		// 2. Update task status to processing
		if err := svc.TaskRepo.UpdateTaskProcessing(ctx, taskID); err != nil {
			logger.ErrorContext(ctx, "failed to update task status to processing", err)
		}

		// 3. Get document segments
		segments, err := svc.DocumentRepo.GetSegmentsByDocumentID(ctx, graphFlowTask.DocumentID.String())
		if err != nil {
			svc.TaskRepo.UpdateTaskFailed(ctx, taskID, fmt.Sprintf("failed to get segments: %v", err))
			return fmt.Errorf("failed to get segments: %v: %w", err, asynq.SkipRetry)
		}

		// Check for eventual consistency: if segments are not found yet, trigger retry
		if len(segments) == 0 {
			retryCount, _ := asynq.GetRetryCount(ctx)
			maxRetry := 10 // Increase retry count for eventual consistency

			if retryCount < maxRetry {
				logger.Warn("No segments found for document, triggering retry", map[string]interface{}{
					"task_id":     taskID.String(),
					"document_id": graphFlowTask.DocumentID.String(),
					"retry_count": retryCount,
				})
				return fmt.Errorf("no segments found for document %s, waiting for consistency (retry %d/%d)", graphFlowTask.DocumentID, retryCount, maxRetry)
			}

			logger.Warn("No segments found after retries, proceeding to empty alignment (assuming empty document)", map[string]interface{}{
				"document_id": graphFlowTask.DocumentID.String(),
			})

			// Mark extraction as completed even if no segments (empty document case)
			if err := svc.TaskRepo.UpdateTaskCompleted(ctx, taskID); err != nil {
				logger.ErrorContext(ctx, "failed to update task completed status", err)
			}

			if err := enqueueNextAlignmentTask(ctx, svc, taskManager, graphFlowTask); err != nil {
				logger.CriticalContext(ctx, "failed to enqueue alignment task", err)
			}

			return nil
		}

		// Race condition fix: Check if we have all expected segments
		if payload.ExpectedSegments > 0 && len(segments) < payload.ExpectedSegments {
			retryCount, _ := asynq.GetRetryCount(ctx)
			maxRetry := 20 // Allow more retries for segment replication lag

			if retryCount < maxRetry {
				logger.Warn("Segments mismatch (replication lag), triggering retry", map[string]interface{}{
					"task_id":           taskID.String(),
					"document_id":       graphFlowTask.DocumentID.String(),
					"found_segments":    len(segments),
					"expected_segments": payload.ExpectedSegments,
					"retry_count":       retryCount,
				})
				// Return error to trigger Asynq retry
				return fmt.Errorf("segments mismatch: found %d, expected %d, waiting for consistency (retry %d/%d)", len(segments), payload.ExpectedSegments, retryCount, maxRetry)
			}

			errMsg := fmt.Sprintf("Segments mismatch persisted after retries: found %d, expected %d", len(segments), payload.ExpectedSegments)
			logger.ErrorContext(ctx, errMsg)

			// Update task to failed state with error message
			if err := svc.TaskRepo.UpdateTaskFailed(ctx, taskID, errMsg); err != nil {
				logger.ErrorContext(ctx, "failed to update task failed status", err)
			}

			// We return nil here to consume the task from queue as failed,
			// instead of returning error which would cause infinite retries (since we exhausted our custom maxRetry)
			return nil
		}

		// Check if all segments are completed (vectorization finished)
		// Filter out segments that are still indexing or have errors
		var completedSegments []*dataset_model.DocumentSegment
		var indexingSegments int
		var errorSegments int

		for _, seg := range segments {
			switch seg.Status {
			case "completed":
				completedSegments = append(completedSegments, seg)
			case "indexing", "waiting":
				indexingSegments++
			case "error":
				errorSegments++
			default:
				completedSegments = append(completedSegments, seg)
			}
		}

		// If some segments are still indexing, wait for them
		if indexingSegments > 0 {
			retryCount, _ := asynq.GetRetryCount(ctx)
			maxRetry := 10

			if retryCount < maxRetry {
				logger.Warn("Some segments are still indexing, triggering retry", map[string]interface{}{
					"task_id":            taskID.String(),
					"document_id":        graphFlowTask.DocumentID.String(),
					"indexing_segments":  indexingSegments,
					"completed_segments": len(completedSegments),
					"retry_count":        retryCount,
				})
				return fmt.Errorf("waiting for %d segments to complete indexing", indexingSegments)
			}

			logger.Warn("Segments still indexing after max retries, proceeding with completed segments only", map[string]interface{}{
				"document_id":        graphFlowTask.DocumentID.String(),
				"indexing_segments":  indexingSegments,
				"completed_segments": len(completedSegments),
			})
		}

		// If all segments have errors, mark task as failed
		if len(completedSegments) == 0 && errorSegments > 0 {
			logger.ErrorContext(ctx, "all segments have errors, cannot proceed with extraction")
			svc.TaskRepo.UpdateTaskFailed(ctx, taskID, "all document segments failed during indexing")
			return fmt.Errorf("all segments have errors: %w", asynq.SkipRetry)
		}

		// Use only completed segments for extraction
		segments = completedSegments

		// 4. Process all completed segments with in-task concurrency
		return handleConcurrentExtraction(ctx, svc, taskManager, graphFlowTask, segments, taskID)
	}
}

// handleConcurrentExtraction processes all segments concurrently using a goroutine pool.
func handleConcurrentExtraction(
	ctx context.Context,
	svc *graphflow.Service,
	taskManager *queue.TaskManager,
	graphFlowTask *model.GraphFlowTask,
	segments []*dataset_model.DocumentSegment,
	taskID uuid.UUID,
) error {

	totalSegments := len(segments)
	organizationID := resolveExtractionOrganizationID(ctx, svc.DatasetRepo, graphFlowTask)

	// FETCH DATASET: Get custom model settings if available
	dataset, err := svc.DatasetRepo.GetByID(ctx, graphFlowTask.KBID.String())
	var entityModel, entityModelProvider *string
	if err == nil && dataset != nil {
		entityModel = dataset.EntityModel
		entityModelProvider = dataset.EntityModelProvider
	}

	// Get the appropriate extractor
	entityExtractor := svc.GetExtractor(graphFlowTask.ExtractionStrategy, entityModel, entityModelProvider)

	// Generate Global Entities (Core Entity Pool) — done before concurrency starts
	var globalEntities []string
	if totalSegments > 0 {
		var contextTextBuilder strings.Builder
		for i := 0; i < totalSegments && i < 5; i++ {
			if contextTextBuilder.Len() > 2000 {
				break
			}
			contextTextBuilder.WriteString(segments[i].Content)
			contextTextBuilder.WriteString("\n")
		}

		contextText := contextTextBuilder.String()
		if len(contextText) > 0 {
			if gEntities, err := entityExtractor.GenerateGlobalEntities(ctx, organizationID, contextText); err != nil {
				logger.Warn("Failed to generate global entities", map[string]interface{}{
					"task_id":         taskID.String(),
					"organization_id": organizationID,
					"error":           err.Error(),
				})
			} else {
				globalEntities = gEntities
				logger.Info("Generated global entities", map[string]interface{}{
					"task_id":  taskID.String(),
					"entities": globalEntities,
				})
			}
		}
	}

	// Fetch Document Title once
	documentTitle := ""
	if doc, err := svc.DocumentRepo.GetByID(ctx, graphFlowTask.DocumentID.String()); err == nil && doc != nil {
		documentTitle = doc.Name
	} else {
		logger.Warn("Failed to fetch document title for extraction context", map[string]interface{}{
			"doc_id": graphFlowTask.DocumentID.String(),
			"error":  err,
		})
	}

	// Concurrent extraction with goroutine pool
	var (
		mu                sync.Mutex
		allEntityMentions []*model.EntityMention
		allTripleMentions []*model.TripleMention
		extractedTypes    = make(map[string]extractor.EntityType)
		completed         int32
		failed            int32
		firstFailureMsg   string
		firstFailureOnce  sync.Once
		wg                sync.WaitGroup
		semaphore         = make(chan struct{}, ExtractionConcurrency)
	)

	logger.Info("Starting concurrent extraction", map[string]interface{}{
		"task_id":        taskID.String(),
		"total_segments": totalSegments,
		"concurrency":    ExtractionConcurrency,
	})

	for i := range segments {
		// Check context cancellation before launching goroutine
		if ctx.Err() != nil {
			break
		}

		wg.Add(1)
		semaphore <- struct{}{} // Acquire slot

		go func(idx int) {
			defer wg.Done()
			defer func() { <-semaphore }() // Release slot

			// Panic recovery per goroutine
			defer func() {
				if r := recover(); r != nil {
					segmentCtx := logger.WithFields(ctx,
						zap.Int("segment_no", idx+1),
						zap.String("segment_id", segments[idx].ID),
					)
					logger.CriticalContext(segmentCtx, "extraction goroutine panicked", fmt.Errorf("%v", r))
				}
			}()

			seg := segments[idx]
			segmentCtx := logger.WithFields(ctx,
				zap.Int("segment_no", idx+1),
				zap.String("segment_id", seg.ID),
			)

			segmentID, err := uuid.Parse(seg.ID)
			if err != nil {
				atomic.AddInt32(&failed, 1)
				firstFailureOnce.Do(func() {
					firstFailureMsg = fmt.Sprintf("invalid segment ID %s: %v", seg.ID, err)
				})
				logger.ErrorContext(segmentCtx, "invalid segment id", err)
				return
			}

			logger.Info("Processing segment extraction", map[string]interface{}{
				"task_id":    taskID.String(),
				"segment_no": idx + 1,
				"total":      totalSegments,
			})

			result, err := entityExtractor.Extract(ctx, organizationID, seg.Content, documentTitle, globalEntities)
			if err != nil {
				atomic.AddInt32(&failed, 1)
				firstFailureOnce.Do(func() {
					firstFailureMsg = fmt.Sprintf("segment %s extraction failed: %v", seg.ID, err)
				})
				logger.ErrorContext(segmentCtx, "segment extraction failed", err)
				if updateErr := svc.DocumentRepo.UpdateSegmentGraphIndexingStatus(ctx, seg.ID, "failed"); updateErr != nil {
					logger.ErrorContext(segmentCtx, "failed to update segment graph indexing status to failed", updateErr)
				}
				return
			}

			normalizeExtractionResult(result, documentTitle)

			logger.Info(fmt.Sprintf("[DEBUG EXTRACTION] Segment %s extracted %d entities, %d relationships", seg.ID, len(result.Entities), len(result.Relationships)), nil)

			if updateErr := svc.DocumentRepo.UpdateSegmentGraphIndexingStatus(ctx, seg.ID, "extracted"); updateErr != nil {
				logger.ErrorContext(segmentCtx, "failed to update segment graph indexing status to extracted", updateErr)
			}

			// Collect results under lock
			mu.Lock()
			for _, entity := range result.Entities {
				mention := &model.EntityMention{
					KBID:       graphFlowTask.KBID,
					TenantID:   graphFlowTask.TenantID,
					SegmentID:  segmentID,
					RawName:    entity.Name,
					RawType:    entity.Type,
					Confidence: 1.0,
					Status:     "pending",
				}
				allEntityMentions = append(allEntityMentions, mention)

				if entity.Type != "" {
					extractedTypes[entity.Type] = entity.TypeInfo
				}
			}
			for _, rel := range result.Relationships {
				triple := &model.TripleMention{
					KBID:         graphFlowTask.KBID,
					TenantID:     graphFlowTask.TenantID,
					SegmentID:    segmentID,
					RawSubject:   rel.Source,
					RawPredicate: rel.Type,
					RawObject:    rel.Target,
					Status:       "pending",
				}
				allTripleMentions = append(allTripleMentions, triple)
			}
			mu.Unlock()

			// Update progress atomically
			done := atomic.AddInt32(&completed, 1)
			progress := int(float64(done) / float64(totalSegments) * 100)
			if err := svc.TaskRepo.UpdateTaskProgress(ctx, taskID, progress); err != nil {
				logger.Warn("Failed to update task progress", map[string]interface{}{
					"task_id": taskID.String(),
					"error":   err.Error(),
				})
			}
		}(i)
	}

	// Wait for all goroutines to finish
	wg.Wait()

	// Check if context was canceled during processing
	if ctx.Err() != nil {
		svc.TaskRepo.UpdateTaskFailed(ctx, taskID, fmt.Sprintf("context canceled during extraction: %v", ctx.Err()))
		return fmt.Errorf("context canceled during extraction: %w", ctx.Err())
	}

	if err := validateExtractionOutcome(
		totalSegments,
		int(atomic.LoadInt32(&failed)),
		len(allEntityMentions),
		len(allTripleMentions),
		firstFailureMsg,
	); err != nil {
		logger.CriticalContext(ctx,
			"graph extraction validation failed",
			err,
			"failed_segments", int(atomic.LoadInt32(&failed)),
			"entity_mentions", len(allEntityMentions),
			"triple_mentions", len(allTripleMentions),
		)
		svc.TaskRepo.UpdateTaskFailed(ctx, taskID, err.Error())
		return fmt.Errorf("graph extraction failed: %w: %w", err, asynq.SkipRetry)
	}

	// Save mentions to DB
	if len(allEntityMentions) > 0 {
		if err := svc.EntityMentionRepo.CreateBatch(ctx, allEntityMentions); err != nil {
			logger.CriticalContext(ctx, "failed to save entity mentions", err, "count", len(allEntityMentions))
			svc.TaskRepo.UpdateTaskFailed(ctx, taskID, fmt.Sprintf("failed to save entity mentions: %v", err))
			return fmt.Errorf("failed to save entity mentions: %v: %w", err, asynq.SkipRetry)
		}
	}

	if len(allTripleMentions) > 0 {
		if err := svc.TripleMentionRepo.CreateBatch(ctx, allTripleMentions); err != nil {
			logger.CriticalContext(ctx, "failed to save triple mentions", err, "count", len(allTripleMentions))
			svc.TaskRepo.UpdateTaskFailed(ctx, taskID, fmt.Sprintf("failed to save triple mentions: %v", err))
			return fmt.Errorf("failed to save triple mentions: %v: %w", err, asynq.SkipRetry)
		}
	}

	// Auto-upsert extracted types to type dictionary
	if len(extractedTypes) > 0 && svc.TypeDefinitionRepo != nil {
		var typeDefinitions []model.TypeDefinition
		for _, typeInfo := range extractedTypes {
			labelZh := typeInfo.LabelZh
			labelEn := typeInfo.LabelEn
			if labelZh == "" {
				labelZh = typeInfo.Key
			}
			if labelEn == "" {
				labelEn = typeInfo.Key
			}
			typeDefinitions = append(typeDefinitions, model.TypeDefinition{
				DatasetID:   graphFlowTask.KBID,
				TypeKey:     typeInfo.Key,
				LabelZh:     &labelZh,
				LabelEn:     &labelEn,
				StyleConfig: make(map[string]interface{}),
			})
		}
		if err := svc.TypeDefinitionRepo.UpsertBatch(ctx, typeDefinitions); err != nil {
			logger.Warn("Failed to upsert type definitions", map[string]interface{}{
				"task_id": taskID.String(),
				"error":   err.Error(),
				"count":   len(typeDefinitions),
			})
		} else {
			logger.Info("Auto-upserted type definitions", map[string]interface{}{
				"task_id": taskID.String(),
				"count":   len(typeDefinitions),
			})
		}
	}

	// Enqueue alignment task first (Reliability Fix)
	if err := enqueueNextAlignmentTask(ctx, svc, taskManager, graphFlowTask); err != nil {
		logger.CriticalContext(ctx, "failed to enqueue alignment task", err)
		svc.TaskRepo.UpdateTaskFailed(ctx, taskID, fmt.Sprintf("failed to enqueue alignment task: %v", err))
		return fmt.Errorf("failed to enqueue next task: %w", err)
	}

	// Update current task status to completed
	if err := svc.TaskRepo.UpdateTaskCompleted(ctx, taskID); err != nil {
		logger.ErrorContext(ctx, "failed to update task status to completed", err)
	}

	logger.Info("GraphFlow extraction completed", map[string]interface{}{
		"task_id":         taskID.String(),
		"entity_mentions": len(allEntityMentions),
		"triple_mentions": len(allTripleMentions),
	})

	return nil
}

func validateExtractionOutcome(totalSegments, failedSegments, entityMentions, tripleMentions int, firstFailure string) error {
	if totalSegments > 0 && failedSegments >= totalSegments {
		if firstFailure != "" {
			return fmt.Errorf("all %d segment extractions failed: first error: %s", totalSegments, firstFailure)
		}
		return fmt.Errorf("all %d segment extractions failed", totalSegments)
	}

	if entityMentions == 0 && tripleMentions == 0 {
		if failedSegments > 0 {
			if firstFailure != "" {
				return fmt.Errorf("graph extraction produced no entities or relationships after %d segment failures: first error: %s", failedSegments, firstFailure)
			}
			return fmt.Errorf("graph extraction produced no entities or relationships after %d segment failures", failedSegments)
		}
		return errors.New("graph extraction produced no entities or relationships")
	}

	return nil
}

func normalizeExtractionResult(result *extractor.ExtractionResult, documentTitle string) {
	if result == nil {
		return
	}

	documentName := strings.TrimSpace(documentTitle)
	if documentName == "" {
		documentName = "Untitled Document"
	}

	entityIndex := make(map[string]int, len(result.Entities))
	for i := range result.Entities {
		name := strings.TrimSpace(result.Entities[i].Name)
		if name == "" {
			continue
		}
		result.Entities[i].Name = name
		if name == documentName {
			setEntityType(&result.Entities[i], "Document", "文档", "Document")
		} else if strings.TrimSpace(result.Entities[i].Type) == "" {
			setEntityType(&result.Entities[i], "Concept", "概念", "Concept")
		}
		entityIndex[name] = i
	}

	ensureEntity := func(name, typeKey, labelZh, labelEn string) {
		name = strings.TrimSpace(name)
		if name == "" {
			return
		}
		if idx, ok := entityIndex[name]; ok {
			if strings.TrimSpace(result.Entities[idx].Type) == "" || name == documentName {
				setEntityType(&result.Entities[idx], typeKey, labelZh, labelEn)
			}
			return
		}

		entityIndex[name] = len(result.Entities)
		result.Entities = append(result.Entities, extractor.ExtractedEntity{
			Name: name,
			Type: typeKey,
			TypeInfo: extractor.EntityType{
				Key:     typeKey,
				LabelZh: labelZh,
				LabelEn: labelEn,
			},
		})
	}

	ensureEntity(documentName, "Document", "文档", "Document")

	for _, relation := range result.Relationships {
		ensureEntity(relation.Source, "Concept", "概念", "Concept")
		ensureEntity(relation.Target, "Concept", "概念", "Concept")
	}
}

func setEntityType(entity *extractor.ExtractedEntity, typeKey, labelZh, labelEn string) {
	entity.Type = typeKey
	entity.TypeInfo = extractor.EntityType{
		Key:     typeKey,
		LabelZh: labelZh,
		LabelEn: labelEn,
	}
}

// enqueueNextAlignmentTask creates a new alignment task and enqueues it
func enqueueNextAlignmentTask(ctx context.Context, svc *graphflow.Service, taskManager *queue.TaskManager, currentTask *model.GraphFlowTask) error {
	_ = time.Now()

	// Create alignment task record
	alignmentTask := &model.GraphFlowTask{
		TenantID:           currentTask.TenantID,
		KBID:               currentTask.KBID,
		DocumentID:         currentTask.DocumentID,
		TaskType:           "alignment",
		ExtractionStrategy: currentTask.ExtractionStrategy,
		Status:             "pending",
		Progress:           0,
		Metadata:           currentTask.Metadata,
	}

	newTaskID, err := svc.TaskRepo.CreateTaskAndReturnID(ctx, alignmentTask)
	if err != nil {
		return fmt.Errorf("failed to create alignment task: %w", err)
	}

	// Create and enqueue alignment task using asynq
	task, err := NewGraphFlowTask(TypeGraphFlowAlignment, newTaskID.String(), taskManager)
	if err != nil {
		svc.TaskRepo.UpdateTaskFailed(ctx, newTaskID, fmt.Sprintf("failed to create task: %v", err))
		return fmt.Errorf("failed to create alignment task: %w", err)
	}

	_, err = taskManager.EnqueueTask(task, asynq.Queue("graphflow"))
	if err != nil {
		svc.TaskRepo.UpdateTaskFailed(ctx, newTaskID, fmt.Sprintf("failed to enqueue: %v", err))
		return fmt.Errorf("failed to enqueue alignment task: %w", err)
	}

	logger.Info("Alignment task created and enqueued", map[string]interface{}{
		"task_id":     newTaskID.String(),
		"document_id": currentTask.DocumentID.String(),
	})

	return nil
}
