package service

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/zgiai/zgi/api/internal/contracts"
	"github.com/zgiai/zgi/api/internal/dto"
)

func (s *dataSourceService) extractDatabaseIngestionFileInfoWithContentParse(ctx context.Context, accountID string, fileInfo *dto.UploadFile) (databaseIngestionExtractionResult, error) {
	result := databaseIngestionExtractionResult{
		PrimaryStrategy: databaseIngestionStrategyContentParse,
		ActualStrategy:  databaseIngestionStrategyContentParse,
		FallbackReason:  databaseIngestionFallbackNone,
		SourceType:      databaseIngestionSourceContentParse,
	}
	started := time.Now()
	if s.contentParseService == nil {
		err := fmt.Errorf("content parse service is not initialized")
		result.Attempts = append(result.Attempts, databaseIngestionAttempt(
			databaseIngestionAttemptMethodFileParse,
			databaseIngestionAttemptStatusFailed,
			databaseIngestionAttemptResultError,
			err.Error(),
			started,
			0,
		))
		return result, err
	}
	if fileInfo == nil {
		err := fmt.Errorf("file info is required for content parse extraction")
		result.Attempts = append(result.Attempts, databaseIngestionAttempt(
			databaseIngestionAttemptMethodFileParse,
			databaseIngestionAttemptStatusFailed,
			databaseIngestionAttemptResultError,
			err.Error(),
			started,
			0,
		))
		return result, err
	}

	data, err := s.fileService.DownloadFile(ctx, fileInfo.ID)
	if err != nil {
		err = fmt.Errorf("failed to download file for content parse: %w", err)
		result.Attempts = append(result.Attempts, databaseIngestionAttempt(
			databaseIngestionAttemptMethodFileParse,
			databaseIngestionAttemptStatusFailed,
			databaseIngestionAttemptResultError,
			err.Error(),
			started,
			0,
		))
		return result, err
	}

	parseReq := contracts.ParseRequest{
		SourceType: contracts.ParseSourceTypeBytes,
		SourceRef:  fileInfo.ID,
		FileName:   fileInfo.Name,
		Data:       data,
		Intent:     contracts.ParseIntentDatasetIndex,
		Profile:    contracts.ParseProfileLayoutFirst,
		Metadata: map[string]any{
			"source":          "database_ingestion",
			"file_id":         fileInfo.ID,
			"account_id":      accountID,
			"cache_namespace": databaseIngestionCacheNamespace,
		},
	}
	artifact, err := s.parseDatabaseIngestionContentWithContentParse(ctx, parseReq)
	if err != nil {
		err = fmt.Errorf("content parse failed: %w", err)
		result.Attempts = append(result.Attempts, databaseIngestionAttempt(
			databaseIngestionAttemptMethodFileParse,
			databaseIngestionAttemptStatusFailed,
			databaseIngestionAttemptResultError,
			err.Error(),
			started,
			0,
		))
		return result, err
	}

	content := databaseIngestionContentFromParseArtifact(artifact)
	result.Content = content
	reason := databaseIngestionContentParseReason(artifact)
	if strings.TrimSpace(content) == "" {
		result.Attempts = append(result.Attempts, databaseIngestionAttempt(
			databaseIngestionAttemptMethodFileParse,
			databaseIngestionAttemptStatusCompleted,
			databaseIngestionAttemptResultEmpty,
			reason,
			started,
			0,
		))
		return result, fmt.Errorf("content parse returned empty content")
	}

	result.Attempts = append(result.Attempts, databaseIngestionAttempt(
		databaseIngestionAttemptMethodFileParse,
		databaseIngestionAttemptStatusCompleted,
		databaseIngestionAttemptResultContent,
		reason,
		started,
		0,
	))
	return result, nil
}

func databaseIngestionContentFromParseArtifact(artifact *contracts.ParseArtifact) string {
	if artifact == nil {
		return ""
	}
	if content := strings.TrimSpace(artifact.Markdown); content != "" {
		return content
	}
	if content := strings.TrimSpace(artifact.Text); content != "" {
		return content
	}
	if len(artifact.Elements) == 0 {
		return ""
	}

	elements := append([]contracts.ParsedElement(nil), artifact.Elements...)
	sort.SliceStable(elements, func(i, j int) bool {
		if elements[i].Page != elements[j].Page {
			return elements[i].Page < elements[j].Page
		}
		return elements[i].Ordinal < elements[j].Ordinal
	})

	var builder strings.Builder
	for _, element := range elements {
		content := strings.TrimSpace(element.Content)
		if content == "" {
			continue
		}
		if builder.Len() > 0 {
			builder.WriteString("\n\n")
		}
		builder.WriteString(content)
	}
	return builder.String()
}

func (s *dataSourceService) parseDatabaseIngestionContentWithContentParse(ctx context.Context, req contracts.ParseRequest) (*contracts.ParseArtifact, error) {
	if routed, ok := s.contentParseService.(contracts.RoutedContentParseService); ok {
		return routed.ParseWithRouting(ctx, req)
	}
	return s.contentParseService.Parse(ctx, req)
}

func databaseIngestionContentParseReason(artifact *contracts.ParseArtifact) string {
	if artifact == nil {
		return ""
	}
	parts := make([]string, 0, 4)
	if provider := databaseIngestionParseArtifactMetadataString(artifact, "executed_provider_key"); provider != "" {
		parts = append(parts, "provider="+provider)
	}
	if adapter := databaseIngestionParseArtifactMetadataString(artifact, "executed_adapter_name"); adapter != "" {
		parts = append(parts, "adapter="+adapter)
	}
	executedEngine := databaseIngestionParseArtifactMetadataString(artifact, "executed_engine_name")
	if executedEngine != "" {
		parts = append(parts, "engine="+executedEngine)
	} else if artifact.EngineUsed != "" {
		parts = append(parts, "engine="+string(artifact.EngineUsed))
	}
	if artifact.EngineUsed != "" && executedEngine != "" && executedEngine != string(artifact.EngineUsed) {
		parts = append(parts, "artifact_engine="+string(artifact.EngineUsed))
	}
	if artifact.Status != "" {
		parts = append(parts, "status="+string(artifact.Status))
	}
	if artifact.QualityLevel != "" {
		parts = append(parts, "quality="+string(artifact.QualityLevel))
	}
	if artifact.FallbackUsed {
		parts = append(parts, "fallback=true")
	}
	return strings.Join(parts, "; ")
}

func databaseIngestionParseArtifactMetadataString(artifact *contracts.ParseArtifact, key string) string {
	if artifact == nil || artifact.Metadata == nil {
		return ""
	}
	value, ok := artifact.Metadata[key]
	if !ok || value == nil {
		return ""
	}
	switch typed := value.(type) {
	case string:
		return strings.TrimSpace(typed)
	case contracts.ParseEngine:
		return strings.TrimSpace(string(typed))
	default:
		return strings.TrimSpace(fmt.Sprintf("%v", typed))
	}
}
