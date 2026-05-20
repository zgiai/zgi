package indexing

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/zgiai/ginext/internal/capabilities/contentparse/routing"
	"github.com/zgiai/ginext/internal/dto"
	contentparsesvc "github.com/zgiai/ginext/internal/modules/contentparse/service"
	"github.com/zgiai/ginext/internal/modules/dataset/model"
	"github.com/zgiai/ginext/pkg/logger"
)

const (
	defaultContentParseShadowSamplingLimit = 10
	maxContentParseShadowSamplingLimit     = 50
	contentParseShadowSamplingPageSize     = 20
	maxContentParseShadowSamplingScanLimit = 200
)

type ContentParseShadowSamplingResult struct {
	DatasetID       string                           `json:"dataset_id"`
	RequestedLimit  int                              `json:"requested_limit"`
	MatchedCount    int                              `json:"matched_count"`
	ScheduledCount  int                              `json:"scheduled_count"`
	SkippedCount    int                              `json:"skipped_count"`
	FailedCount     int                              `json:"failed_count"`
	ConcurrencyHint int                              `json:"concurrency_hint"`
	Items           []ContentParseShadowSamplingItem `json:"items"`
}

type ContentParseShadowSamplingItem struct {
	DocumentID string `json:"document_id"`
	FileID     string `json:"file_id,omitempty"`
	FileName   string `json:"file_name,omitempty"`
	Status     string `json:"status"`
	Reason     string `json:"reason,omitempty"`
}

func (ir *IndexingRunner) StartContentParseShadowSampling(ctx context.Context, datasetID, organizationID string, limit int, documentIDs []string) (*ContentParseShadowSamplingResult, error) {
	if ir == nil || ir.contentParseShadowRunner == nil || ir.contentParseRunService == nil || ir.fileService == nil || ir.documentRepo == nil {
		return nil, fmt.Errorf("content parse shadow sampling is not configured")
	}
	limit = normalizeContentParseShadowSamplingLimit(limit)
	result := &ContentParseShadowSamplingResult{
		DatasetID:       datasetID,
		RequestedLimit:  limit,
		ConcurrencyHint: contentParseShadowConcurrencyLimit(),
		Items:           make([]ContentParseShadowSamplingItem, 0, limit),
	}

	if len(documentIDs) > 0 {
		docs, err := ir.loadContentParseShadowSamplingDocuments(ctx, datasetID, organizationID, 1, limit, documentIDs)
		if err != nil {
			return nil, err
		}
		result.MatchedCount = len(docs)
		ir.scheduleContentParseShadowSamples(ctx, docs, limit, result)
		return result, nil
	}

	pageSize := contentParseShadowSamplingQueryLimit(limit)
	scanLimit := contentParseShadowSamplingScanLimit(limit)
	for page := 1; result.ScheduledCount < limit && result.MatchedCount < scanLimit; page++ {
		docs, err := ir.loadContentParseShadowSamplingDocuments(ctx, datasetID, organizationID, page, pageSize, nil)
		if err != nil {
			return nil, err
		}
		if len(docs) == 0 {
			break
		}
		if remaining := scanLimit - result.MatchedCount; remaining < len(docs) {
			docs = docs[:remaining]
		}
		result.MatchedCount += len(docs)
		ir.scheduleContentParseShadowSamples(ctx, docs, limit, result)
		if len(docs) < pageSize {
			break
		}
	}
	return result, nil
}

func (ir *IndexingRunner) scheduleContentParseShadowSamples(ctx context.Context, docs []*model.Document, limit int, result *ContentParseShadowSamplingResult) {
	for _, doc := range docs {
		if result.ScheduledCount >= limit {
			return
		}
		if doc == nil {
			continue
		}
		item := ContentParseShadowSamplingItem{
			DocumentID: doc.ID,
			FileName:   doc.Name,
			Status:     "scheduled",
		}
		if doc.FileID != nil {
			item.FileID = strings.TrimSpace(*doc.FileID)
		}
		if item.FileID == "" {
			item.Status = "skipped"
			item.Reason = "missing_file"
			result.SkippedCount++
			result.Items = append(result.Items, item)
			continue
		}

		content, sourceHash, workspaceID, sourceErr := ir.prepareContentParseShadowSource(ctx, item.FileID)
		if sourceErr != nil {
			item.Status = "skipped"
			item.Reason = "source_unavailable"
			result.SkippedCount++
			result.Items = append(result.Items, item)
			continue
		}

		primary, chunks, baselineErr := ir.buildContentParseShadowBaseline(ctx, doc)
		if baselineErr != nil {
			item.Status = "failed"
			item.Reason = baselineErr.Error()
			result.FailedCount++
			result.Items = append(result.Items, item)
			continue
		}

		scheduled := ir.contentParseShadowRunner.EnqueueDatasetIndexingShadow(ctx, contentparsesvc.DatasetShadowInput{
			DocumentID:        doc.ID,
			DatasetID:         doc.DatasetID,
			OrganizationID:    doc.OrganizationID,
			FileID:            item.FileID,
			FileName:          doc.Name,
			Data:              content,
			SourceContentHash: sourceHash,
			WorkspaceID:       workspaceID,
			PrimaryOutput:     primary,
			LegacyChunks:      chunks,
			InitialSummary: map[string]interface{}{
				"enabled":                 true,
				"captured_at":             time.Now().Unix(),
				"manual_sampling":         true,
				"legacy_extract_baseline": summarizePrimaryExtractOutput(primary),
				"legacy_chunk_baseline":   summarizeLegacyTransformedChunks(chunks),
			},
			RecognitionSource: "dataset_shadow_sampling",
			Source:            "dataset_shadow_sampling",
		})
		if !scheduled {
			item.Status = "skipped"
			item.Reason = "concurrency_limit"
			result.SkippedCount++
			result.Items = append(result.Items, item)
			continue
		}
		result.ScheduledCount++
		result.Items = append(result.Items, item)
		if result.ScheduledCount >= limit {
			return
		}
	}
}

func (ir *IndexingRunner) prepareContentParseShadowSource(ctx context.Context, fileID string) ([]byte, string, string, error) {
	uploadFile, err := ir.fileService.GetFileByID(ctx, fileID)
	if err != nil {
		return nil, "", "", fmt.Errorf("load source file: %w", err)
	}
	content, err := ir.fileService.DownloadFile(ctx, fileID)
	if err != nil {
		return nil, "", "", fmt.Errorf("load source content: %w", err)
	}
	sourceHash := ""
	workspaceID := ""
	if uploadFile != nil {
		sourceHash = strings.TrimSpace(uploadFile.Hash)
		if uploadFile.WorkspaceID != nil {
			workspaceID = strings.TrimSpace(*uploadFile.WorkspaceID)
		}
	}
	return content, sourceHash, workspaceID, nil
}

func (ir *IndexingRunner) loadContentParseShadowSamplingDocuments(ctx context.Context, datasetID, organizationID string, page, limit int, documentIDs []string) ([]*model.Document, error) {
	if len(documentIDs) > 0 {
		docs, err := ir.documentRepo.GetDocumentsByIDs(ctx, contentparsesvc.CompactStringSlice(documentIDs))
		if err != nil {
			return nil, err
		}
		filtered := make([]*model.Document, 0, len(docs))
		for _, doc := range docs {
			if doc == nil || doc.DatasetID != datasetID || doc.OrganizationID != organizationID {
				continue
			}
			filtered = append(filtered, doc)
			if len(filtered) >= limit {
				break
			}
		}
		return filtered, nil
	}

	if page < 1 {
		page = 1
	}
	docs, _, err := ir.documentRepo.GetByDatasetID(ctx, datasetID, page, limit, "", "-created_at", false, "completed")
	if err != nil {
		return nil, err
	}
	filtered := make([]*model.Document, 0, len(docs))
	for _, doc := range docs {
		if doc == nil || doc.OrganizationID != organizationID {
			continue
		}
		filtered = append(filtered, doc)
	}
	return filtered, nil
}

func (ir *IndexingRunner) buildContentParseShadowBaseline(ctx context.Context, doc *model.Document) (*dto.ExtractOutput, []dto.TransformedChunk, error) {
	if doc == nil {
		return nil, nil, fmt.Errorf("document is required")
	}
	segments, err := ir.documentRepo.GetSegmentsByDocumentID(ctx, doc.ID)
	if err != nil {
		return nil, nil, fmt.Errorf("load legacy segments: %w", err)
	}
	sort.SliceStable(segments, func(i, j int) bool {
		return segments[i].Position < segments[j].Position
	})

	primary := &dto.ExtractOutput{
		Source: "dataset_segments",
		Metadata: map[string]any{
			"document_id": doc.ID,
			"dataset_id":  doc.DatasetID,
		},
	}
	markdownParts := make([]string, 0, len(segments))
	chunks := make([]dto.TransformedChunk, 0, len(segments))
	for index, segment := range segments {
		if segment == nil || strings.TrimSpace(segment.Content) == "" {
			continue
		}
		metadata := map[string]any{
			"document_id": doc.ID,
			"dataset_id":  doc.DatasetID,
			"segment_id":  segment.ID,
			"position":    segment.Position,
		}
		if segment.IndexNodeID != nil && strings.TrimSpace(*segment.IndexNodeID) != "" {
			metadata["doc_id"] = strings.TrimSpace(*segment.IndexNodeID)
		}
		if segment.IndexNodeHash != nil && strings.TrimSpace(*segment.IndexNodeHash) != "" {
			metadata["doc_hash"] = strings.TrimSpace(*segment.IndexNodeHash)
		}
		primary.Elements = append(primary.Elements, dto.ExtractElement{
			Type:     "text",
			Page:     0,
			Content:  segment.Content,
			Ordinal:  index,
			Metadata: cloneContentParseShadowMetadata(metadata),
		})
		markdownParts = append(markdownParts, strings.TrimSpace(segment.Content))

		children, childErr := ir.documentRepo.GetChildChunksBySegmentID(ctx, segment.ID)
		if childErr != nil {
			return nil, nil, fmt.Errorf("load legacy child chunks: %w", childErr)
		}
		sort.SliceStable(children, func(i, j int) bool {
			return children[i].Position < children[j].Position
		})
		chunk := dto.TransformedChunk{
			Content:  segment.Content,
			Metadata: cloneContentParseShadowMetadata(metadata),
		}
		for _, child := range children {
			if strings.TrimSpace(child.Content) == "" {
				continue
			}
			childMetadata := map[string]any{
				"document_id": doc.ID,
				"dataset_id":  doc.DatasetID,
				"segment_id":  segment.ID,
				"child_id":    child.ID,
				"position":    child.Position,
			}
			if child.IndexNodeID != nil && strings.TrimSpace(*child.IndexNodeID) != "" {
				childMetadata["doc_id"] = strings.TrimSpace(*child.IndexNodeID)
			}
			if child.IndexNodeHash != nil && strings.TrimSpace(*child.IndexNodeHash) != "" {
				childMetadata["doc_hash"] = strings.TrimSpace(*child.IndexNodeHash)
			}
			chunk.Children = append(chunk.Children, dto.TransformedChildChunk{
				Content:  child.Content,
				Metadata: childMetadata,
			})
		}
		chunks = append(chunks, chunk)
	}
	primary.Markdown = strings.Join(markdownParts, "\n\n")
	return primary, chunks, nil
}

func (ir *IndexingRunner) enqueueContentParseShadowJob(job contentparsesvc.DatasetShadowInput) bool {
	if ir == nil || ir.contentParseShadowRunner == nil || ir.fileService == nil || strings.TrimSpace(job.FileID) == "" {
		return false
	}
	if !tryAcquireContentParseShadowSlot() {
		logger.Warn("Skipping content parse shadow because concurrency limit is reached", map[string]interface{}{
			"document_id": job.DocumentID,
			"dataset_id":  job.DatasetID,
			"file_id":     job.FileID,
		})
		return false
	}
	go ir.runContentParseShadowJob(job)
	return true
}

func (ir *IndexingRunner) EnqueueDatasetIndexingShadow(ctx context.Context, job contentparsesvc.DatasetShadowInput) bool {
	return ir.enqueueContentParseShadowJob(job)
}

func (ir *IndexingRunner) runContentParseShadowJob(job contentparsesvc.DatasetShadowInput) {
	defer releaseContentParseShadowSlot()
	shadowCtx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()

	result, err := ir.contentParseShadowRunner.RunDatasetIndexingShadow(shadowCtx, job)
	summary := map[string]interface{}{
		"enabled":     true,
		"captured_at": time.Now().Unix(),
	}
	if result != nil && result.Summary != nil {
		summary = result.Summary
	}
	if err != nil {
		summary = contentparsesvc.ApplyDatasetShadowFailure(summary, err)
	}
	ir.persistContentParseShadow(shadowCtx, job.DocumentID, summary)
}

func (ir *IndexingRunner) RunDatasetIndexingShadow(ctx context.Context, job contentparsesvc.DatasetShadowInput) (*contentparsesvc.DatasetShadowResult, error) {
	if ir == nil || ir.contentParseSvc == nil {
		return nil, fmt.Errorf("content parse shadow service is not configured")
	}
	summary := contentparsesvc.NewDatasetShadowSummary(job, time.Now().Unix())
	request := contentparsesvc.NewDatasetShadowParseRequest(job)

	var routePlan *routing.RoutePlan
	if ir.contentParsePlanner != nil && ir.contentParseCatalog != nil {
		health, healthErr := ir.contentParseSvc.Health(ctx)
		if healthErr != nil {
			summary["route_plan_health_error"] = healthErr.Error()
		}
		planned, planErr := ir.contentParsePlanner.Plan(request, ir.contentParseCatalog, health)
		if planErr != nil {
			summary["route_plan_error"] = planErr.Error()
		} else {
			routePlan = planned
			summary["route_plan"] = contentparsesvc.RoutePlanSummary(planned)
		}
	}

	uploadFile, fileInfoErr := ir.fileService.GetFileByID(ctx, job.FileID)
	fileHash := ""
	fileWorkspaceID := ""
	if fileInfoErr == nil && uploadFile != nil {
		fileHash = uploadFile.Hash
		if uploadFile.WorkspaceID != nil {
			fileWorkspaceID = *uploadFile.WorkspaceID
		}
	}
	var sourceContext contentparsesvc.DatasetShadowSourceContext
	summary, sourceContext = contentparsesvc.ApplyDatasetShadowSourceContext(summary, job, fileHash, fileWorkspaceID)
	workspaceID := sourceContext.WorkspaceID

	content := job.Data
	if len(content) == 0 {
		downloaded, err := ir.fileService.DownloadFile(ctx, job.FileID)
		if err != nil {
			summary = contentparsesvc.ApplyDatasetShadowFailure(summary, fmt.Errorf("download file: %w", err))
			ir.persistContentParseRunShadow(ctx, job.DocumentID, job.DatasetID, workspaceID, job.OrganizationID, job.FileID, request, routePlan, nil, summary)
			return &contentparsesvc.DatasetShadowResult{Summary: summary}, err
		}
		content = downloaded
	}

	request.Data = content
	startedAt := time.Now()
	artifact, err := ir.runContentParseShadow(ctx, request, routePlan)
	summary["duration_ms"] = time.Since(startedAt).Milliseconds()
	if err != nil {
		summary = contentparsesvc.ApplyDatasetShadowFailure(summary, err)
		ir.persistContentParseRunShadow(ctx, job.DocumentID, job.DatasetID, workspaceID, job.OrganizationID, job.FileID, request, routePlan, nil, summary)
		return &contentparsesvc.DatasetShadowResult{Summary: summary}, err
	}

	summary = contentparsesvc.ApplyDatasetShadowArtifactSummary(summary, artifact)
	runID, artifactUUID := ir.persistContentParseRunShadow(ctx, job.DocumentID, job.DatasetID, workspaceID, job.OrganizationID, job.FileID, request, routePlan, artifact, summary)
	if runID != nil {
		summary["parse_run_id"] = runID.String()
		ir.persistChunkingShadow(ctx, *runID, artifactUUID, artifact, job.PrimaryOutput, job.LegacyChunks, summary)
	}
	if artifactUUID != nil {
		summary["artifact_uuid"] = artifactUUID.String()
	}

	parseRunID := ""
	if runID != nil {
		parseRunID = runID.String()
	}
	parseArtifactID := ""
	if artifactUUID != nil {
		parseArtifactID = artifactUUID.String()
	}
	return contentparsesvc.NewDatasetShadowResult(summary, parseRunID, parseArtifactID), nil
}

func normalizeContentParseShadowSamplingLimit(limit int) int {
	if limit <= 0 {
		return defaultContentParseShadowSamplingLimit
	}
	if limit > maxContentParseShadowSamplingLimit {
		return maxContentParseShadowSamplingLimit
	}
	return limit
}

func contentParseShadowSamplingQueryLimit(limit int) int {
	queryLimit := normalizeContentParseShadowSamplingLimit(limit) * 2
	if queryLimit < contentParseShadowSamplingPageSize {
		return contentParseShadowSamplingPageSize
	}
	if queryLimit > maxContentParseShadowSamplingLimit {
		return maxContentParseShadowSamplingLimit
	}
	return queryLimit
}

func contentParseShadowSamplingScanLimit(limit int) int {
	scanLimit := normalizeContentParseShadowSamplingLimit(limit) * 10
	if scanLimit < contentParseShadowSamplingPageSize {
		return contentParseShadowSamplingPageSize
	}
	if scanLimit > maxContentParseShadowSamplingScanLimit {
		return maxContentParseShadowSamplingScanLimit
	}
	return scanLimit
}
