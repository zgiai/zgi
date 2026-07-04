package indexing

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/zgiai/zgi/api/config"
	"github.com/zgiai/zgi/api/internal/modules/dataset/model"
	dataset_repository "github.com/zgiai/zgi/api/internal/modules/dataset/repository"
	"github.com/zgiai/zgi/api/pkg/embedding"
	"github.com/zgiai/zgi/api/pkg/logger"
	"github.com/zgiai/zgi/api/pkg/sentry"
	"github.com/zgiai/zgi/api/pkg/vectordb"
)

type indexingItemType string

const (
	indexingItemTypeSegment  indexingItemType = "segment"
	indexingItemTypeChild    indexingItemType = "child"
	indexingItemTypeQuestion indexingItemType = "question"
)

type indexingItem struct {
	IndexNodeID       string
	Text              string
	ClassName         string
	Properties        map[string]interface{}
	ParentIndexNodeID string
	ItemType          indexingItemType
}

type indexingBatchOptions struct {
	Name          string
	FailOnPartial bool
}

type indexingItemResult struct {
	tokenCount int
	err        error
}

type indexingStatusTarget struct {
	itemType indexingItemType
	id       string
}

type indexingTargetProgress struct {
	remaining int
	firstErr  error
	finalized bool
}

func processIndexingItems(
	ctx context.Context,
	dataset *model.Dataset,
	items []indexingItem,
	embeddingService embedding.EmbeddingService,
	documentRepo dataset_repository.DocumentRepository,
	vectorDB vectordb.VectorDB,
	options indexingBatchOptions,
) (int, error) {
	if len(items) == 0 {
		return 0, nil
	}
	if embeddingService == nil {
		err := fmt.Errorf("embedding service is not configured")
		markIndexingItemsError(ctx, documentRepo, items, err.Error())
		return 0, err
	}

	if err := ensureIndexingVectorClasses(ctx, vectorDB, items); err != nil {
		markIndexingItemsError(ctx, documentRepo, items, fmt.Sprintf("Failed to ensure vector class: %v", err))
		return 0, err
	}

	if err := markIndexingItemsStarted(ctx, documentRepo, items); err != nil {
		markIndexingItemsError(ctx, documentRepo, items, fmt.Sprintf("Failed to update indexing status: %v", err))
		return 0, err
	}

	results := make([]indexingItemResult, len(items))
	targetProgress := buildIndexingTargetProgress(items)
	batchSize := indexingVectorBatchSize()
	for start := 0; start < len(items); start += batchSize {
		if err := ctx.Err(); err != nil {
			markPendingIndexingTargetsError(ctx, documentRepo, items, results, targetProgress, err.Error())
			return summarizeIndexingResults(items, results, options, err)
		}

		end := start + batchSize
		if end > len(items) {
			end = len(items)
		}

		batchItems := items[start:end]
		vectors, embedErrs := embedIndexingBatch(ctx, dataset, embeddingService, batchItems, options.Name)
		objects := make([]vectordb.VectorObject, 0, len(batchItems))
		objectIndexes := make([]int, 0, len(batchItems))
		for i, item := range batchItems {
			resultIndex := start + i
			if embedErrs[i] != nil {
				results[resultIndex].err = embedErrs[i]
				continue
			}
			if len(vectors[i]) == 0 {
				results[resultIndex].err = fmt.Errorf("empty embedding vector for index node %s", item.IndexNodeID)
				continue
			}
			results[resultIndex].tokenCount = len(vectors[i])
			objects = append(objects, vectordb.VectorObject{
				ID:         item.IndexNodeID,
				Class:      item.ClassName,
				Properties: item.Properties,
				Vector:     vectors[i],
			})
			objectIndexes = append(objectIndexes, resultIndex)
		}

		storeErrs := storeIndexingVectorObjects(ctx, vectorDB, objects, options.Name)
		for i, storeErr := range storeErrs {
			if storeErr != nil {
				results[objectIndexes[i]].err = storeErr
				results[objectIndexes[i]].tokenCount = 0
			}
		}

		if err := finalizeIndexingBatchTargets(ctx, documentRepo, items, results, targetProgress, start, end); err != nil {
			return 0, err
		}
	}

	return summarizeIndexingResults(items, results, options, nil)
}

func indexingVectorBatchSize() int {
	if config.GlobalConfig != nil && config.GlobalConfig.VectorStore.IndexingBatchSize > 0 {
		return config.GlobalConfig.VectorStore.IndexingBatchSize
	}
	return 4
}

func ensureIndexingVectorClasses(ctx context.Context, vectorDB vectordb.VectorDB, items []indexingItem) error {
	seen := make(map[string]struct{})
	for _, item := range items {
		if strings.TrimSpace(item.ClassName) == "" {
			return fmt.Errorf("empty vector class for index node %s", item.IndexNodeID)
		}
		if _, ok := seen[item.ClassName]; ok {
			continue
		}
		seen[item.ClassName] = struct{}{}
		if err := vectorDB.CreateClass(ctx, item.ClassName, defaultVectorClassProperties()); err != nil {
			return fmt.Errorf("failed to create vector class %s: %w", item.ClassName, err)
		}
	}
	return nil
}

func defaultVectorClassProperties() []map[string]interface{} {
	return []map[string]interface{}{
		{
			"name":            "text",
			"dataType":        []string{"text"},
			"tokenization":    "gse_ch",
			"indexSearchable": true,
		},
	}
}

func markIndexingItemsStarted(ctx context.Context, documentRepo dataset_repository.DocumentRepository, items []indexingItem) error {
	indexingAt := time.Now()
	for _, target := range uniqueIndexingTargets(items) {
		if target.itemType == indexingItemTypeQuestion {
			if err := documentRepo.UpdateQuestionIndexingStatus(ctx, target.id, model.SegmentStatusIndexing, &indexingAt); err != nil {
				return err
			}
			continue
		}
		if err := documentRepo.UpdateSegmentIndexingStatusByIndexNodeID(ctx, target.id, model.DocumentStatusIndexing, &indexingAt); err != nil {
			return err
		}
	}
	return nil
}

func markIndexingItemsError(ctx context.Context, documentRepo dataset_repository.DocumentRepository, items []indexingItem, message string) {
	for _, target := range uniqueIndexingTargets(items) {
		markIndexingTargetError(ctx, documentRepo, target, message)
	}
}

func markIndexingTargetError(ctx context.Context, documentRepo dataset_repository.DocumentRepository, target indexingStatusTarget, message string) {
	if target.id == "" {
		return
	}
	if target.itemType == indexingItemTypeQuestion {
		if err := documentRepo.UpdateQuestionIndexingCompleted(ctx, target.id, nil, &message); err != nil {
			logger.Error("Failed to update question error status", err)
		}
		return
	}
	if err := documentRepo.UpdateSegmentErrorByIndexNodeID(ctx, target.id, message); err != nil {
		logger.Error("Failed to update segment error status", err)
	}
}

func markIndexingTargetCompleted(ctx context.Context, documentRepo dataset_repository.DocumentRepository, target indexingStatusTarget) error {
	completedAt := time.Now()
	if target.itemType == indexingItemTypeQuestion {
		return documentRepo.UpdateQuestionIndexingCompleted(ctx, target.id, &completedAt, nil)
	}
	return documentRepo.UpdateSegmentVectorDataCompletedByIndexNodeID(ctx, target.id, &completedAt)
}

func buildIndexingTargetProgress(items []indexingItem) map[indexingStatusTarget]*indexingTargetProgress {
	progress := make(map[indexingStatusTarget]*indexingTargetProgress)
	for _, item := range items {
		target := item.statusTarget()
		if target.id == "" {
			continue
		}
		state := progress[target]
		if state == nil {
			state = &indexingTargetProgress{}
			progress[target] = state
		}
		state.remaining++
	}
	return progress
}

func finalizeIndexingBatchTargets(
	ctx context.Context,
	documentRepo dataset_repository.DocumentRepository,
	items []indexingItem,
	results []indexingItemResult,
	progress map[indexingStatusTarget]*indexingTargetProgress,
	start, end int,
) error {
	processedTargets := make(map[indexingStatusTarget]struct{})
	for i := start; i < end; i++ {
		target := items[i].statusTarget()
		if target.id == "" {
			continue
		}
		state := progress[target]
		if state == nil || state.finalized {
			continue
		}
		state.remaining--
		if results[i].err != nil && state.firstErr == nil {
			state.firstErr = results[i].err
		}
		processedTargets[target] = struct{}{}
	}

	for target := range processedTargets {
		state := progress[target]
		if state == nil || state.finalized || state.remaining > 0 {
			continue
		}
		state.finalized = true
		if state.firstErr != nil {
			markIndexingTargetError(ctx, documentRepo, target, state.firstErr.Error())
			continue
		}
		if err := markIndexingTargetCompleted(ctx, documentRepo, target); err != nil {
			return err
		}
	}
	return nil
}

func markPendingIndexingTargetsError(
	ctx context.Context,
	documentRepo dataset_repository.DocumentRepository,
	items []indexingItem,
	results []indexingItemResult,
	progress map[indexingStatusTarget]*indexingTargetProgress,
	message string,
) {
	for i, item := range items {
		target := item.statusTarget()
		state := progress[target]
		if target.id == "" || state == nil || state.finalized {
			continue
		}
		if results[i].err == nil {
			results[i].err = context.Canceled
		}
		state.finalized = true
		if state.firstErr == nil {
			state.firstErr = fmt.Errorf("%s", message)
		}
		markIndexingTargetError(ctx, documentRepo, target, message)
	}
}

func uniqueIndexingTargets(items []indexingItem) []indexingStatusTarget {
	seen := make(map[indexingStatusTarget]struct{})
	targets := make([]indexingStatusTarget, 0, len(items))
	for _, item := range items {
		target := item.statusTarget()
		if target.id == "" {
			continue
		}
		if _, ok := seen[target]; ok {
			continue
		}
		seen[target] = struct{}{}
		targets = append(targets, target)
	}
	return targets
}

func (item indexingItem) statusTarget() indexingStatusTarget {
	if item.ItemType == indexingItemTypeChild {
		return indexingStatusTarget{itemType: indexingItemTypeSegment, id: item.ParentIndexNodeID}
	}
	return indexingStatusTarget{itemType: item.ItemType, id: item.IndexNodeID}
}

func embedIndexingBatch(
	ctx context.Context,
	dataset *model.Dataset,
	embeddingService embedding.EmbeddingService,
	items []indexingItem,
	name string,
) ([][]float64, []error) {
	texts := make([]string, len(items))
	for i, item := range items {
		texts[i] = item.Text
	}

	vectors, err := embeddingService.EmbedTexts(ctx, texts)
	if err == nil && len(vectors) == len(items) {
		return vectors, make([]error, len(items))
	}

	if err == nil {
		err = fmt.Errorf("embedding batch returned %d vectors for %d texts", len(vectors), len(items))
	}
	logger.Warn("Embedding batch failed, falling back to single-item requests", map[string]interface{}{
		"name":  name,
		"count": len(items),
		"error": err.Error(),
	})

	vectors = make([][]float64, len(items))
	errs := make([]error, len(items))
	for i, item := range items {
		singleVectors, singleErr := embeddingService.EmbedTexts(ctx, []string{item.Text})
		if singleErr != nil {
			captureIndexingEmbeddingError(singleErr, dataset, item)
			errs[i] = singleErr
			continue
		}
		if len(singleVectors) == 0 {
			singleErr = fmt.Errorf("no embeddings generated for index node %s", item.IndexNodeID)
			captureIndexingEmbeddingError(singleErr, dataset, item)
			errs[i] = singleErr
			continue
		}
		vectors[i] = singleVectors[0]
	}
	return vectors, errs
}

func storeIndexingVectorObjects(ctx context.Context, vectorDB vectordb.VectorDB, objects []vectordb.VectorObject, name string) []error {
	errs := make([]error, len(objects))
	if len(objects) == 0 {
		return errs
	}

	if batchDB, ok := vectorDB.(vectordb.BatchVectorDB); ok {
		if err := batchDB.StoreVectors(ctx, objects); err == nil {
			return errs
		} else {
			if batchErr, ok := err.(*vectordb.BatchVectorError); ok {
				for i, object := range objects {
					if objectErr, exists := batchErr.Errors[object.ID]; exists {
						errs[i] = fmt.Errorf("failed to store vector %s: %w", object.ID, objectErr)
					}
				}
				return errs
			}
			logger.Warn("Vector batch write failed, falling back to single-object writes", map[string]interface{}{
				"name":  name,
				"count": len(objects),
				"error": err.Error(),
			})
		}
	}

	for i, object := range objects {
		if err := vectorDB.StoreVector(ctx, object.ID, object.Class, object.Properties, object.Vector); err != nil {
			errs[i] = fmt.Errorf("failed to store vector %s: %w", object.ID, err)
		}
	}
	return errs
}

func summarizeIndexingResults(
	items []indexingItem,
	results []indexingItemResult,
	options indexingBatchOptions,
	finalErr error,
) (int, error) {
	tokens := 0
	failedItems := 0
	var firstErr error

	for i := range items {
		result := results[i]
		if finalErr != nil && result.err == nil {
			result.err = finalErr
		}
		tokens += result.tokenCount

		if result.err != nil {
			failedItems++
			if firstErr == nil {
				firstErr = result.err
			}
		}
	}

	if finalErr != nil {
		return tokens, finalErr
	}
	if failedItems == len(items) && len(items) > 0 {
		if firstErr != nil {
			return tokens, fmt.Errorf("all %d %s items failed to process. first error: %w", failedItems, options.Name, firstErr)
		}
		return tokens, fmt.Errorf("all %d %s items failed to process", failedItems, options.Name)
	}
	if failedItems > 0 {
		if options.FailOnPartial {
			return tokens, fmt.Errorf("%d out of %d %s items failed to process", failedItems, len(items), options.Name)
		}
		logger.Warn(fmt.Sprintf("%d out of %d %s items failed to process", failedItems, len(items), options.Name))
	}
	return tokens, nil
}

func captureIndexingEmbeddingError(err error, dataset *model.Dataset, item indexingItem) {
	if err == nil || dataset == nil {
		return
	}

	provider := ""
	if dataset.EmbeddingModelProvider != nil {
		provider = *dataset.EmbeddingModelProvider
	}
	modelName := ""
	if dataset.EmbeddingModel != nil {
		modelName = *dataset.EmbeddingModel
	}

	sentry.CaptureEmbeddingError(err,
		provider,
		modelName,
		dataset.ID,
		item.statusTarget().id,
		map[string]interface{}{
			"tenant_id":    dataset.WorkspaceID,
			"index_node":   item.IndexNodeID,
			"item_type":    string(item.ItemType),
			"content_len":  len(item.Text),
			"error_detail": err.Error(),
		},
	)
}
