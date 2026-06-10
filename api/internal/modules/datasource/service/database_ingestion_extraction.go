package service

import (
	"context"
	"encoding/base64"
	"fmt"
	"mime"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/zgiai/zgi/api/internal/dto"
	llmdefaultsvc "github.com/zgiai/zgi/api/internal/modules/llm/defaultmodel/service"
	llmmodelmodel "github.com/zgiai/zgi/api/internal/modules/llm/llmmodel/model"
	llmadapter "github.com/zgiai/zgi/api/internal/modules/llm/protocol/adapters"
	interfaces "github.com/zgiai/zgi/api/internal/modules/shared/interface"
	"github.com/zgiai/zgi/api/pkg/logger"
)

const (
	databaseIngestionCacheNamespace       = "database_ingestion_v2"
	databaseIngestionExtractionModeAuto   = "auto"
	databaseIngestionExtractionModeVision = "vision"
	databaseIngestionStrategyVision       = "vision"
	databaseIngestionSourceMinerU         = "mineru"
	databaseIngestionSourceImageOriginal  = "image_original"
	databaseIngestionSourcePDFRendered    = "pdf_rendered_pages"
	databaseIngestionFallbackNone         = "none"
	databaseIngestionFallbackError        = "mineru_error"
	databaseIngestionFallbackEmpty        = "mineru_empty"
	databaseIngestionFallbackZeroRecords  = "mineru_zero_records"
	databaseIngestionMaxPDFPages          = 3

	databaseIngestionAttemptMethodFileParse   = "file_parse"
	databaseIngestionAttemptMethodModelVision = "model_vision"
	databaseIngestionAttemptStatusCompleted   = "completed"
	databaseIngestionAttemptStatusFailed      = "failed"
	databaseIngestionAttemptResultContent     = "content"
	databaseIngestionAttemptResultRecords     = "records"
	databaseIngestionAttemptResultNoRecords   = "no_records"
	databaseIngestionAttemptResultEmpty       = "empty_content"
	databaseIngestionAttemptResultError       = "error"
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

func (s *dataSourceService) extractDatabaseIngestionFileContent(ctx context.Context, organizationID, accountID, fileID string, modelSpec *dto.ModelSpec) (databaseIngestionExtractionResult, error) {
	fileInfo, err := s.fileService.GetFileByID(ctx, fileID)
	if err != nil {
		return databaseIngestionExtractionResult{
			PrimaryStrategy: dto.DocumentExtractionStrategyHyperParseMineru,
			ActualStrategy:  dto.DocumentExtractionStrategyHyperParseMineru,
			FallbackReason:  databaseIngestionFallbackNone,
			SourceType:      databaseIngestionSourceMinerU,
		}, fmt.Errorf("failed to get file info: %w", err)
	}
	return s.extractDatabaseIngestionFileInfoContent(ctx, organizationID, accountID, fileInfo, modelSpec, databaseIngestionExtractionModeAuto)
}

func (s *dataSourceService) extractDatabaseIngestionFileInfoContent(ctx context.Context, organizationID, accountID string, fileInfo *dto.UploadFile, modelSpec *dto.ModelSpec, extractionMode string) (databaseIngestionExtractionResult, error) {
	result := databaseIngestionExtractionResult{
		PrimaryStrategy: dto.DocumentExtractionStrategyHyperParseMineru,
		ActualStrategy:  dto.DocumentExtractionStrategyHyperParseMineru,
		FallbackReason:  databaseIngestionFallbackNone,
		SourceType:      databaseIngestionSourceMinerU,
	}

	mode := normalizeDatabaseIngestionExtractionMode(extractionMode)
	if mode == databaseIngestionExtractionModeVision {
		return s.extractDatabaseIngestionFileInfoWithVision(ctx, organizationID, accountID, fileInfo, modelSpec)
	}

	if isDatabaseIngestionImageFile(fileInfo.Extension, fileInfo.MimeType) {
		started := time.Now()
		content, err := s.extractDatabaseIngestionImageWithVision(ctx, organizationID, accountID, fileInfo, modelSpec)
		result.Content = content
		result.PrimaryStrategy = databaseIngestionStrategyVision
		result.ActualStrategy = databaseIngestionStrategyVision
		result.SourceType = databaseIngestionSourceImageOriginal
		if err != nil {
			result.Attempts = append(result.Attempts, databaseIngestionAttempt(
				databaseIngestionAttemptMethodModelVision,
				databaseIngestionAttemptStatusFailed,
				databaseIngestionAttemptResultError,
				err.Error(),
				started,
				0,
			))
			return result, err
		}
		result.Attempts = append(result.Attempts, databaseIngestionAttempt(
			databaseIngestionAttemptMethodModelVision,
			databaseIngestionAttemptStatusCompleted,
			databaseIngestionAttemptResultContent,
			"",
			started,
			0,
		))
		return result, nil
	}

	fallbackEnabled := false
	started := time.Now()
	content, mineruErr := s.fileService.ExtractFileWithSetting(ctx, fileInfo.ID, interfaces.FileExtractionSetting{
		ExtractionStrategy:        dto.DocumentExtractionStrategyHyperParseMineru,
		ExtractionFallbackEnabled: &fallbackEnabled,
		CacheNamespace:            databaseIngestionCacheNamespace,
	})
	result.Content = content
	if mineruErr == nil && strings.TrimSpace(content) != "" {
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

	if mineruErr != nil {
		result.FallbackReason = databaseIngestionFallbackError
		result.Attempts = append(result.Attempts, databaseIngestionAttempt(
			databaseIngestionAttemptMethodFileParse,
			databaseIngestionAttemptStatusFailed,
			databaseIngestionAttemptResultError,
			mineruErr.Error(),
			started,
			0,
		))
	} else {
		result.FallbackReason = databaseIngestionFallbackEmpty
		result.Attempts = append(result.Attempts, databaseIngestionAttempt(
			databaseIngestionAttemptMethodFileParse,
			databaseIngestionAttemptStatusCompleted,
			databaseIngestionAttemptResultEmpty,
			"",
			started,
			0,
		))
	}

	if !isDatabaseIngestionPDFFile(fileInfo.Extension, fileInfo.MimeType) {
		if mineruErr != nil {
			return result, mineruErr
		}
		return result, fmt.Errorf("mineru returned empty content")
	}

	visionStarted := time.Now()
	visionContent, visionErr := s.extractDatabaseIngestionPDFWithVision(ctx, organizationID, accountID, fileInfo, modelSpec)
	if visionErr != nil {
		result.Attempts = append(result.Attempts, databaseIngestionAttempt(
			databaseIngestionAttemptMethodModelVision,
			databaseIngestionAttemptStatusFailed,
			databaseIngestionAttemptResultError,
			visionErr.Error(),
			visionStarted,
			0,
		))
		if mineruErr != nil {
			return result, fmt.Errorf("mineru extraction failed: %v; vision fallback failed: %w", mineruErr, visionErr)
		}
		return result, fmt.Errorf("mineru returned empty content; vision fallback failed: %w", visionErr)
	}
	result.Attempts = append(result.Attempts, databaseIngestionAttempt(
		databaseIngestionAttemptMethodModelVision,
		databaseIngestionAttemptStatusCompleted,
		databaseIngestionAttemptResultContent,
		"",
		visionStarted,
		0,
	))

	result.Content = visionContent
	result.ActualStrategy = databaseIngestionStrategyVision
	result.SourceType = databaseIngestionSourcePDFRendered
	return result, nil
}

func (s *dataSourceService) extractDatabaseIngestionFileInfoWithVision(ctx context.Context, organizationID, accountID string, fileInfo *dto.UploadFile, modelSpec *dto.ModelSpec) (databaseIngestionExtractionResult, error) {
	result := databaseIngestionExtractionResult{
		PrimaryStrategy: databaseIngestionStrategyVision,
		ActualStrategy:  databaseIngestionStrategyVision,
		FallbackReason:  databaseIngestionFallbackNone,
	}
	if fileInfo == nil {
		return result, fmt.Errorf("file info is required for vision extraction")
	}
	if isDatabaseIngestionImageFile(fileInfo.Extension, fileInfo.MimeType) {
		started := time.Now()
		content, err := s.extractDatabaseIngestionImageWithVision(ctx, organizationID, accountID, fileInfo, modelSpec)
		result.Content = content
		result.SourceType = databaseIngestionSourceImageOriginal
		if err != nil {
			result.Attempts = append(result.Attempts, databaseIngestionAttempt(
				databaseIngestionAttemptMethodModelVision,
				databaseIngestionAttemptStatusFailed,
				databaseIngestionAttemptResultError,
				err.Error(),
				started,
				0,
			))
			return result, err
		}
		result.Attempts = append(result.Attempts, databaseIngestionAttempt(
			databaseIngestionAttemptMethodModelVision,
			databaseIngestionAttemptStatusCompleted,
			databaseIngestionAttemptResultContent,
			"",
			started,
			0,
		))
		return result, nil
	}
	if isDatabaseIngestionPDFFile(fileInfo.Extension, fileInfo.MimeType) {
		started := time.Now()
		content, err := s.extractDatabaseIngestionPDFWithVision(ctx, organizationID, accountID, fileInfo, modelSpec)
		result.Content = content
		result.SourceType = databaseIngestionSourcePDFRendered
		if err != nil {
			result.Attempts = append(result.Attempts, databaseIngestionAttempt(
				databaseIngestionAttemptMethodModelVision,
				databaseIngestionAttemptStatusFailed,
				databaseIngestionAttemptResultError,
				err.Error(),
				started,
				0,
			))
			return result, err
		}
		result.Attempts = append(result.Attempts, databaseIngestionAttempt(
			databaseIngestionAttemptMethodModelVision,
			databaseIngestionAttemptStatusCompleted,
			databaseIngestionAttemptResultContent,
			"",
			started,
			0,
		))
		return result, nil
	}
	return result, fmt.Errorf("vision extraction only supports image files and PDFs")
}

func (s *dataSourceService) extractDatabaseIngestionImageWithVision(ctx context.Context, organizationID, accountID string, fileInfo *dto.UploadFile, modelSpec *dto.ModelSpec) (string, error) {
	data, err := s.fileService.DownloadFile(ctx, fileInfo.ID)
	if err != nil {
		return "", fmt.Errorf("failed to download image for vision extraction: %w", err)
	}
	dataURL := imageBytesDataURL(data, fileInfo.Extension, fileInfo.MimeType)
	return s.extractDatabaseIngestionVisionContent(ctx, organizationID, accountID, fileInfo.Name, []string{dataURL}, modelSpec)
}

func (s *dataSourceService) extractDatabaseIngestionPDFWithVision(ctx context.Context, organizationID, accountID string, fileInfo *dto.UploadFile, modelSpec *dto.ModelSpec) (string, error) {
	data, err := s.fileService.DownloadFile(ctx, fileInfo.ID)
	if err != nil {
		return "", fmt.Errorf("failed to download pdf for vision fallback: %w", err)
	}
	dataURLs, err := renderPDFPagesToDataURLs(ctx, data, databaseIngestionMaxPDFPages)
	if err != nil {
		return "", fmt.Errorf("failed to render pdf pages for vision fallback: %w", err)
	}
	return s.extractDatabaseIngestionVisionContent(ctx, organizationID, accountID, fileInfo.Name, dataURLs, modelSpec)
}

func (s *dataSourceService) extractDatabaseIngestionVisionContent(ctx context.Context, organizationID, accountID, fileName string, dataURLs []string, modelSpec *dto.ModelSpec) (string, error) {
	resolved, err := s.resolveDatabaseIngestionVisionModel(ctx, organizationID, modelSpec)
	if err != nil {
		return "", err
	}

	parts := make([]llmadapter.MessageContentPart, 0, len(dataURLs)+1)
	parts = append(parts, llmadapter.MessageContentPart{
		Type: "text",
		Text: databaseIngestionVisionPrompt(fileName),
	})
	for _, dataURL := range dataURLs {
		if strings.TrimSpace(dataURL) == "" {
			continue
		}
		parts = append(parts, llmadapter.MessageContentPart{
			Type:     "image_url",
			ImageURL: &llmadapter.ImageURL{URL: dataURL, Detail: "high"},
		})
	}
	if len(parts) <= 1 {
		return "", fmt.Errorf("no image content is available for vision extraction")
	}

	temp := 0.1
	maxTokens := 2000
	resp, err := s.llmClient.Chat(ctx, organizationID, &llmadapter.ChatRequest{
		Provider: strings.TrimSpace(resolved.Provider),
		Model:    strings.TrimSpace(resolved.Model),
		Messages: []llmadapter.Message{
			{Role: "user", Content: parts},
		},
		Temperature: &temp,
		MaxTokens:   &maxTokens,
		Stream:      false,
		User:        accountID,
	})
	if err != nil {
		return "", fmt.Errorf("vision model request failed: %w", err)
	}
	if resp == nil || len(resp.Choices) == 0 {
		return "", fmt.Errorf("vision model returned no choices")
	}
	content := strings.TrimSpace(databaseIngestionChatContentText(resp.Choices[0].Message.Content))
	if content == "" {
		return "", fmt.Errorf("vision model returned empty content")
	}
	return content, nil
}

func (s *dataSourceService) resolveDatabaseIngestionVisionModel(ctx context.Context, organizationID string, modelSpec *dto.ModelSpec) (*llmdefaultsvc.ResolvedModel, error) {
	if s.defaultModelResolver == nil {
		return nil, fmt.Errorf("vision model resolver is not initialized")
	}

	var explicitProvider *string
	var explicitModel *string
	if modelSpec != nil {
		provider := strings.TrimSpace(modelSpec.Provider)
		modelName := strings.TrimSpace(modelSpec.Name)
		if provider != "" {
			explicitProvider = &provider
		}
		if modelName != "" {
			explicitModel = &modelName
		}
	}

	resolved, err := s.defaultModelResolver.ResolveUseCase(ctx, organizationID, llmmodelmodel.UseCaseVision, explicitProvider, explicitModel)
	if err != nil {
		if explicitModel != nil {
			return nil, fmt.Errorf("selected model does not support image input; choose a vision model to process images or scanned PDFs")
		}
		return nil, fmt.Errorf("vision model is required to process images or scanned PDFs: %w", err)
	}
	if resolved == nil || strings.TrimSpace(resolved.Model) == "" {
		return nil, fmt.Errorf("vision model is required to process images or scanned PDFs")
	}
	return resolved, nil
}

func databaseIngestionVisionPrompt(fileName string) string {
	fileName = strings.TrimSpace(fileName)
	if fileName == "" {
		fileName = "uploaded file"
	}
	return fmt.Sprintf(`Extract the visible business content from "%s" into clean Markdown text for downstream database field extraction.

Rules:
- Transcribe all visible key-value pairs, tables, totals, dates, identifiers, names, addresses, and notes exactly as shown.
- Preserve numbers, currency symbols, date formats, invoice/order/contract numbers, phone numbers, and tracking numbers.
- For tables, output Markdown tables with clear headers.
- If a value is unclear or not visible, omit it instead of guessing.
- Do not answer with JSON. Return only the extracted document text.`, fileName)
}

func renderPDFPagesToDataURLs(ctx context.Context, pdfBytes []byte, maxPages int) ([]string, error) {
	if len(pdfBytes) == 0 {
		return nil, fmt.Errorf("pdf content is empty")
	}
	if maxPages <= 0 {
		return nil, fmt.Errorf("invalid pdf render page limit")
	}

	tempDir, err := os.MkdirTemp("", "zgi-db-ingest-pdf-*")
	if err != nil {
		return nil, fmt.Errorf("create temp directory: %w", err)
	}
	defer os.RemoveAll(tempDir)

	inputPath := filepath.Join(tempDir, "input.pdf")
	if err := os.WriteFile(inputPath, pdfBytes, 0o600); err != nil {
		return nil, fmt.Errorf("write temp pdf: %w", err)
	}

	outPrefix := filepath.Join(tempDir, "page")
	cmd := exec.CommandContext(ctx, "pdftoppm", "-png", "-f", "1", "-l", strconv.Itoa(maxPages), "-scale-to", "1536", inputPath, outPrefix)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return nil, fmt.Errorf("pdftoppm failed: %w (%s)", err, strings.TrimSpace(string(output)))
	}

	files, err := filepath.Glob(filepath.Join(tempDir, "page-*.png"))
	if err != nil {
		return nil, fmt.Errorf("collect rendered pages: %w", err)
	}
	sort.Strings(files)
	if len(files) == 0 {
		return nil, fmt.Errorf("pdftoppm rendered no pages")
	}

	dataURLs := make([]string, 0, len(files))
	for _, file := range files {
		data, err := os.ReadFile(file)
		if err != nil {
			logger.WarnContext(ctx, "failed to read rendered pdf page", "file", file, err)
			continue
		}
		if len(data) == 0 {
			continue
		}
		dataURLs = append(dataURLs, "data:image/png;base64,"+base64.StdEncoding.EncodeToString(data))
	}
	if len(dataURLs) == 0 {
		return nil, fmt.Errorf("rendered pdf pages are empty")
	}
	return dataURLs, nil
}

func imageBytesDataURL(data []byte, extension, mimeType string) string {
	mimeType = strings.TrimSpace(mimeType)
	if mimeType == "" || mimeType == "application/octet-stream" {
		ext := strings.TrimPrefix(strings.ToLower(strings.TrimSpace(extension)), ".")
		if ext != "" {
			mimeType = mime.TypeByExtension("." + ext)
		}
	}
	if mimeType == "" || mimeType == "application/octet-stream" {
		mimeType = http.DetectContentType(data)
	}
	if mimeType == "" {
		mimeType = "image/png"
	}
	return "data:" + mimeType + ";base64," + base64.StdEncoding.EncodeToString(data)
}

func isDatabaseIngestionImageFile(extension, mimeType string) bool {
	mimeType = strings.ToLower(strings.TrimSpace(mimeType))
	if strings.HasPrefix(mimeType, "image/") {
		return true
	}
	switch strings.TrimPrefix(strings.ToLower(strings.TrimSpace(extension)), ".") {
	case "jpg", "jpeg", "png", "webp", "gif", "bmp", "tif", "tiff":
		return true
	default:
		return false
	}
}

func isDatabaseIngestionPDFFile(extension, mimeType string) bool {
	mimeType = strings.ToLower(strings.TrimSpace(mimeType))
	if mimeType == "application/pdf" {
		return true
	}
	return strings.TrimPrefix(strings.ToLower(strings.TrimSpace(extension)), ".") == "pdf"
}

func normalizeDatabaseIngestionExtractionMode(mode string) string {
	switch strings.ToLower(strings.TrimSpace(mode)) {
	case databaseIngestionExtractionModeVision:
		return databaseIngestionExtractionModeVision
	default:
		return databaseIngestionExtractionModeAuto
	}
}

func shouldRetryDatabaseIngestionPDFWithVision(fileInfo *dto.UploadFile, extraction databaseIngestionExtractionResult, records []map[string]interface{}) bool {
	if fileInfo == nil || len(records) > 0 {
		return false
	}
	if !isDatabaseIngestionPDFFile(fileInfo.Extension, fileInfo.MimeType) {
		return false
	}
	if extraction.PrimaryStrategy != dto.DocumentExtractionStrategyHyperParseMineru ||
		extraction.ActualStrategy != dto.DocumentExtractionStrategyHyperParseMineru {
		return false
	}
	return strings.TrimSpace(extraction.Content) != ""
}

func databaseIngestionChatContentText(content interface{}) string {
	switch value := content.(type) {
	case string:
		return value
	case []llmadapter.MessageContentPart:
		var builder strings.Builder
		for _, part := range value {
			if strings.TrimSpace(part.Text) == "" {
				continue
			}
			if builder.Len() > 0 {
				builder.WriteString("\n")
			}
			builder.WriteString(part.Text)
		}
		return builder.String()
	default:
		return fmt.Sprint(value)
	}
}
