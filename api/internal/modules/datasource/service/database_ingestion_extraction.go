package service

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/zgiai/zgi/api/internal/dto"
	interfaces "github.com/zgiai/zgi/api/internal/modules/shared/interface"
	"github.com/zgiai/zgi/api/pkg/logger"
)

const (
	databaseIngestionCacheNamespace       = "database_ingestion_v2"
	databaseIngestionStrategyContentParse = "content_parse"
	databaseIngestionSourceContentParse   = "content_parse"
	databaseIngestionSourceMinerU         = "mineru"
	databaseIngestionFallbackNone         = "none"
	databaseIngestionFallbackError        = "mineru_error"
	databaseIngestionFallbackEmpty        = "mineru_empty"

	databaseIngestionAttemptMethodFileParse = "file_parse"
	databaseIngestionAttemptStatusCompleted = "completed"
	databaseIngestionAttemptStatusFailed    = "failed"
	databaseIngestionAttemptResultContent   = "content"
	databaseIngestionAttemptResultRecords   = "records"
	databaseIngestionAttemptResultNoRecords = "no_records"
	databaseIngestionAttemptResultEmpty     = "empty_content"
	databaseIngestionAttemptResultError     = "error"
)

type databaseIngestionExtractionResult struct {
	Content         string
	PrimaryStrategy string
	ActualStrategy  string
	FallbackReason  string
	SourceType      string
	Attempts        []dto.FileIngestAttempt
}

func databaseIngestionAttempt(method, status, result, reason string, started time.Time, recordCount int) dto.FileIngestAttempt {
	durationMS := int64(0)
	if !started.IsZero() {
		durationMS = time.Since(started).Milliseconds()
	}
	return dto.FileIngestAttempt{
		Method:      method,
		Status:      status,
		Result:      result,
		Reason:      reason,
		DurationMS:  durationMS,
		RecordCount: recordCount,
	}
}

func (s *dataSourceService) extractDatabaseIngestionFileContent(ctx context.Context, accountID, fileID string) (databaseIngestionExtractionResult, error) {
	fileInfo, err := s.fileService.GetFileByID(ctx, fileID)
	if err != nil {
		return databaseIngestionExtractionResult{
			PrimaryStrategy: dto.DocumentExtractionStrategyHyperParseMineru,
			ActualStrategy:  dto.DocumentExtractionStrategyHyperParseMineru,
			FallbackReason:  databaseIngestionFallbackNone,
			SourceType:      databaseIngestionSourceMinerU,
		}, fmt.Errorf("failed to get file info: %w", err)
	}
	return s.extractDatabaseIngestionFileInfoContent(ctx, accountID, fileInfo)
}

func (s *dataSourceService) extractDatabaseIngestionFileInfoContent(ctx context.Context, accountID string, fileInfo *dto.UploadFile) (databaseIngestionExtractionResult, error) {
	result := databaseIngestionExtractionResult{
		PrimaryStrategy: dto.DocumentExtractionStrategyHyperParseMineru,
		ActualStrategy:  dto.DocumentExtractionStrategyHyperParseMineru,
		FallbackReason:  databaseIngestionFallbackNone,
		SourceType:      databaseIngestionSourceMinerU,
	}

	contentParseAttempts := []dto.FileIngestAttempt(nil)
	if s.contentParseService != nil {
		contentParseResult, contentParseErr := s.extractDatabaseIngestionFileInfoWithContentParse(ctx, accountID, fileInfo)
		contentParseAttempts = append(contentParseAttempts, contentParseResult.Attempts...)
		if contentParseErr == nil && strings.TrimSpace(contentParseResult.Content) != "" {
			return contentParseResult, nil
		}
		if contentParseErr != nil {
			logger.WarnContext(ctx, "database ingestion content parse failed; falling back to legacy file parser", "file_id", fileInfo.ID, "error", contentParseErr)
		}
	}

	fallbackEnabled := false
	started := time.Now()
	content, parserErr := s.fileService.ExtractFileWithSetting(ctx, fileInfo.ID, interfaces.FileExtractionSetting{
		ExtractionStrategy:        dto.DocumentExtractionStrategyHyperParseMineru,
		ExtractionFallbackEnabled: &fallbackEnabled,
		CacheNamespace:            databaseIngestionCacheNamespace,
	})
	result.Content = content
	result.Attempts = append(result.Attempts, contentParseAttempts...)
	if parserErr == nil && strings.TrimSpace(content) != "" {
		result.Attempts = append(result.Attempts, databaseIngestionAttempt(
			databaseIngestionAttemptMethodFileParse,
			databaseIngestionAttemptStatusCompleted,
			databaseIngestionAttemptResultContent,
			"",
			started,
			0,
		))
		return result, nil
	}

	if parserErr != nil {
		result.FallbackReason = databaseIngestionFallbackError
		result.Attempts = append(result.Attempts, databaseIngestionAttempt(
			databaseIngestionAttemptMethodFileParse,
			databaseIngestionAttemptStatusFailed,
			databaseIngestionAttemptResultError,
			parserErr.Error(),
			started,
			0,
		))
		return result, parserErr
	}

	result.FallbackReason = databaseIngestionFallbackEmpty
	result.Attempts = append(result.Attempts, databaseIngestionAttempt(
		databaseIngestionAttemptMethodFileParse,
		databaseIngestionAttemptStatusCompleted,
		databaseIngestionAttemptResultEmpty,
		"",
		started,
		0,
	))
	return result, fmt.Errorf("file parser returned empty content")
}
