package extractor

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"github.com/google/uuid"
	"github.com/ledongthuc/pdf"
	extractmineru "github.com/zgiai/zgi/api/internal/capabilities/contentparse/engines/hyperparse/pkg/providers/mineru"
	"github.com/zgiai/zgi/api/internal/dto"
	dataset_model "github.com/zgiai/zgi/api/internal/modules/dataset/model"
	"github.com/zgiai/zgi/api/internal/modules/file_process/model"
	"github.com/zgiai/zgi/api/internal/modules/file_process/service/extractor/hyperparse"
	"github.com/zgiai/zgi/api/internal/modules/file_process/service/extractor/landingai"
	"github.com/zgiai/zgi/api/internal/modules/file_process/service/extractor/reducto"
	"github.com/zgiai/zgi/api/internal/modules/file_process/service/extractor/unstructured"
	quota_model "github.com/zgiai/zgi/api/internal/modules/quota/model"
	interfaces "github.com/zgiai/zgi/api/internal/modules/shared/interface"
	"gorm.io/gorm"

	"github.com/zgiai/zgi/api/config"
	"github.com/zgiai/zgi/api/pkg/logger"
	"github.com/zgiai/zgi/api/pkg/storage"
)

// ExtractSetting
type ExtractSetting struct {
	DatasourceType string
	UploadFile     *model.UploadFile
	DocumentModel  string
	ProcessRule    *dataset_model.DatasetProcessRule
	// user choose extraction strategy: landingai | local | mineru.
	ExtractionStrategy        string
	ExtractionFallbackEnabled *bool
}

// DatasourceType
const (
	DatasourceTypeFile   = "upload_file"
	DatasourceTypeNotion = "notion"
	DatasourceTypeWeb    = "website"
)

// ETL Types
const (
	ETLTypeUnstructured = "Unstructured"
	ETLTypeLandingAI    = "LandingAI"
	ETLTypeReducto      = "Reducto"
	ETLTypeMixed        = "Mixed"
	ETLTypeHyperparse   = "Hyperparse"
	ETLTypeDefault      = "zgi"
)

func AvailableDocumentExtractionStrategies() []string {
	options := DocumentExtractionStrategyOptions()
	strategies := make([]string, 0, len(options))
	for _, option := range options {
		if option.Available {
			strategies = append(strategies, option.Strategy)
		}
	}
	return strategies
}

func DocumentExtractionStrategyOptions() []dto.DocumentExtractionStrategyStatus {
	cfg := config.Current()
	options := make([]dto.DocumentExtractionStrategyStatus, 0, 5)

	if cfg.ETL.HyperparseEnabled {
		mineruConfigured := extractmineru.Configured()
		options = append(options, dto.DocumentExtractionStrategyStatus{
			Strategy:   dto.DocumentExtractionStrategyHyperParseMineru,
			Available:  mineruConfigured,
			Configured: mineruConfigured,
			Reason:     strategyReason(mineruConfigured, "configured", "not_configured"),
		})

		reductoConfigured := strings.TrimSpace(cfg.ETL.ReductoAPIKey) != ""
		options = append(options, dto.DocumentExtractionStrategyStatus{
			Strategy:   dto.DocumentExtractionStrategyHyperParseReducto,
			Available:  reductoConfigured,
			Configured: reductoConfigured,
			Reason:     strategyReason(reductoConfigured, "configured", "not_configured"),
		})

		options = append(options, dto.DocumentExtractionStrategyStatus{
			Strategy:   dto.DocumentExtractionStrategyHyperParseLocal,
			Available:  true,
			Configured: true,
			Reason:     "builtin",
		})
	}

	unstructuredConfigured := strings.TrimSpace(cfg.ETL.UnstructuredAPIURL) != "" && strings.TrimSpace(cfg.ETL.UnstructuredAPIKey) != ""
	options = append(options, dto.DocumentExtractionStrategyStatus{
		Strategy:   dto.DocumentExtractionStrategyUnstructured,
		Available:  unstructuredConfigured,
		Configured: unstructuredConfigured,
		Reason:     strategyReason(unstructuredConfigured, "configured", "not_configured"),
	})

	landingAIConfigured := strings.TrimSpace(cfg.ETL.LandingAIAPIKey) != ""
	options = append(options, dto.DocumentExtractionStrategyStatus{
		Strategy:   dto.DocumentExtractionStrategyLandingAI,
		Available:  landingAIConfigured,
		Configured: landingAIConfigured,
		Reason:     strategyReason(landingAIConfigured, "configured", "not_configured"),
	})

	recommended := RecommendedDocumentExtractionStrategyFromOptions(options)
	for i := range options {
		options[i].Recommended = options[i].Strategy == recommended
	}

	return options
}

func RecommendedDocumentExtractionStrategy() string {
	return RecommendedDocumentExtractionStrategyFromOptions(DocumentExtractionStrategyOptions())
}

func RecommendedDocumentExtractionStrategyFromOptions(options []dto.DocumentExtractionStrategyStatus) string {
	for _, strategy := range []string{
		dto.DocumentExtractionStrategyHyperParseMineru,
		dto.DocumentExtractionStrategyHyperParseReducto,
		dto.DocumentExtractionStrategyLandingAI,
		dto.DocumentExtractionStrategyHyperParseLocal,
		dto.DocumentExtractionStrategyUnstructured,
	} {
		for _, option := range options {
			if option.Strategy == strategy && option.Available {
				return option.Strategy
			}
		}
	}
	return ""
}

func DocumentExtractionStrategyAvailable(strategy string) bool {
	normalized := normalizeExtractionStrategy(strategy)
	for _, option := range DocumentExtractionStrategyOptions() {
		if option.Strategy == normalized {
			return option.Available
		}
	}
	return false
}

func strategyReason(ok bool, availableReason, unavailableReason string) string {
	if ok {
		return availableReason
	}
	return unavailableReason
}

type BaseExtractor interface {
	Extract(ctx context.Context) (*dto.ExtractOutput, error)
}

type extractionAttempt struct {
	Strategy string `json:"strategy"`
	ETLType  string `json:"etl_type"`
	Success  bool   `json:"success"`
	Error    string `json:"error,omitempty"`
}

type ExtractProcessor struct {
	storage                    storage.Storage
	quotaService               interfaces.QuotaService
	db                         *gorm.DB
	imageSummaryClient         VisionSummaryClient
	defaultVisionModelResolver DefaultVisionModelResolver
}

func NewExtractProcessor(storage storage.Storage) *ExtractProcessor {
	return &ExtractProcessor{storage: storage}
}

// NewExtractProcessorWithQuota creates an extract processor with quota service support
func NewExtractProcessorWithQuota(storage storage.Storage, quotaService interfaces.QuotaService, db *gorm.DB) *ExtractProcessor {
	return &ExtractProcessor{
		storage:      storage,
		quotaService: quotaService,
		db:           db,
	}
}

func NewExtractProcessorWithQuotaAndVision(
	storage storage.Storage,
	quotaService interfaces.QuotaService,
	db *gorm.DB,
	imageSummaryClient VisionSummaryClient,
	defaultVisionModelResolver DefaultVisionModelResolver,
) *ExtractProcessor {
	return &ExtractProcessor{
		storage:                    storage,
		quotaService:               quotaService,
		db:                         db,
		imageSummaryClient:         imageSummaryClient,
		defaultVisionModelResolver: defaultVisionModelResolver,
	}
}

func (p *ExtractProcessor) LoadFromUploadFile(
	ctx context.Context,
	uploadFile *model.UploadFile,
	returnText bool,
	isAutomatic bool,
) (*dto.ExtractOutput, string, error) {
	setting := &ExtractSetting{
		DatasourceType:     DatasourceTypeFile,
		UploadFile:         uploadFile,
		DocumentModel:      "text_model",
		ProcessRule:        nil,
		ExtractionStrategy: "",
	}

	output, err := p.extract(ctx, setting, isAutomatic, "")
	if err != nil {
		return nil, "", err
	}

	if returnText {
		return output, dto.ExtractOutputText(output), nil
	}

	return output, "", nil
}

// LoadFromUploadFileWithSetting loads documents from an upload file with a specific setting
func (p *ExtractProcessor) LoadFromUploadFileWithSetting(
	ctx context.Context,
	uploadFile *model.UploadFile,
	returnText bool,
	isAutomatic bool,
	setting *ExtractSetting,
) (*dto.ExtractOutput, string, error) {
	// Override the upload file in the setting
	setting.UploadFile = uploadFile
	if setting.DocumentModel == "" {
		setting.DocumentModel = "text_model"
	}

	output, err := p.extract(ctx, setting, isAutomatic, "")
	if err != nil {
		return nil, "", err
	}

	if returnText {
		return output, dto.ExtractOutputText(output), nil
	}

	return output, "", nil
}

func (p *ExtractProcessor) LoadFromURL(ctx context.Context, url string, returnText bool) (*dto.ExtractOutput, string, error) {
	tempDir, err := os.MkdirTemp("", "url_extract")
	if err != nil {
		return nil, "", fmt.Errorf("failed to create temp dir: %w", err)
	}
	defer os.RemoveAll(tempDir)

	resp, err := http.Get(url)
	if err != nil {
		return nil, "", fmt.Errorf("failed to fetch URL: %w", err)
	}
	defer resp.Body.Close()

	ext := getFileExtensionFromURL(url, resp.Header)
	if ext == "" {
		return nil, "", errors.New("unable to determine file extension")
	}

	filePath := filepath.Join(tempDir, uuid.New().String()+ext)
	outFile, err := os.Create(filePath)
	if err != nil {
		return nil, "", fmt.Errorf("failed to create temp file: %w", err)
	}
	defer outFile.Close()

	_, err = io.Copy(outFile, resp.Body)
	if err != nil {
		return nil, "", fmt.Errorf("failed to save URL content: %w", err)
	}

	setting := &ExtractSetting{
		DatasourceType:     DatasourceTypeFile,
		DocumentModel:      "text_model",
		ProcessRule:        nil,
		ExtractionStrategy: "",
	}

	output, err := p.extract(ctx, setting, false, filePath)
	if err != nil {
		return nil, "", err
	}

	if returnText {
		return output, dto.ExtractOutputText(output), nil
	}

	return output, "", nil
}

func (p *ExtractProcessor) extract(
	ctx context.Context,
	setting *ExtractSetting,
	isAutomatic bool,
	filePath string,
) (*dto.ExtractOutput, error) {
	switch setting.DatasourceType {
	case DatasourceTypeFile:
		if filePath == "" {
			if setting.UploadFile == nil {
				return nil, errors.New("upload file is required")
			}

			tempDir, err := os.MkdirTemp("", "file_extract")
			if err != nil {
				return nil, fmt.Errorf("failed to create temp dir: %w", err)
			}
			defer os.RemoveAll(tempDir)

			filePath = filepath.Join(tempDir, uuid.New().String()+uploadFileExtractionExtension(setting.UploadFile))
			err = p.storage.Download(setting.UploadFile.Key, filePath)
			if err != nil {
				return nil, fmt.Errorf("failed to download file: %w", err)
			}
		}

		ext := strings.ToLower(filepath.Ext(filePath))

		extractionStrategy := normalizeExtractionStrategy(setting.ExtractionStrategy)
		output, err := p.extractByStrategy(ctx, filePath, ext, setting, extractionStrategy)
		if extractionFallbackEnabled(setting) && (err != nil || !hasExtractedContent(output)) {
			output, err = p.extractWithFallback(ctx, filePath, ext, setting)
		}
		return output, err
	case DatasourceTypeNotion:
		// TODO: Notion
		return nil, errors.New("Notion extraction not implemented")
	case DatasourceTypeWeb:
		// TODO: WEB
		return nil, errors.New("Web extraction not implemented")
	}

	return nil, errors.New("unsupported datasource type")
}

func (p *ExtractProcessor) ResolveETLType(ext, extractionStrategy string) string {
	switch strings.ToLower(strings.TrimSpace(ext)) {
	case ".xls", ".xlsx", ".csv":
		return ETLTypeDefault
	}

	switch strings.ToLower(strings.TrimSpace(extractionStrategy)) {
	case dto.DocumentExtractionStrategyLandingAI:
		return ETLTypeLandingAI
	case dto.DocumentExtractionStrategyUnstructured:
		return ETLTypeUnstructured
	case dto.DocumentExtractionStrategyHyperParseMineru,
		dto.DocumentExtractionStrategyHyperParseReducto,
		dto.DocumentExtractionStrategyHyperParseLocal:
		return ETLTypeHyperparse
	default:
		return ETLTypeDefault
	}
}

func (p *ExtractProcessor) extractWithFallback(ctx context.Context, filePath, ext string, setting *ExtractSetting) (*dto.ExtractOutput, error) {
	requestedStrategy := normalizeExtractionStrategy(setting.ExtractionStrategy)
	strategies := orderedExtractionStrategies(requestedStrategy, AvailableDocumentExtractionStrategies())
	if len(strategies) == 0 {
		return nil, errors.New("no document extraction strategy is available")
	}

	attempts := make([]extractionAttempt, 0, len(strategies))
	var lastErr error
	for _, strategy := range strategies {
		output, err := p.extractByStrategy(ctx, filePath, ext, setting, strategy)
		attempt := extractionAttempt{
			Strategy: strategy,
			ETLType:  p.ResolveETLType(ext, strategy),
			Success:  hasExtractedContent(output) && err == nil,
		}
		if err != nil {
			attempt.Error = err.Error()
			lastErr = err
		} else if !hasExtractedContent(output) {
			attempt.Error = "empty extraction result"
			lastErr = errors.New("empty extraction result")
		}
		attempts = append(attempts, attempt)

		if attempt.Success {
			attachExtractionMetadata(output, requestedStrategy, strategy, attempts)
			return output, nil
		}

		logger.WarnContext(ctx, "document extraction strategy failed", "strategy", strategy, "extension", ext, "error", attempt.Error)
	}

	attempted := make([]string, 0, len(attempts))
	for _, attempt := range attempts {
		attempted = append(attempted, attempt.Strategy)
	}
	if lastErr == nil {
		lastErr = errors.New("no extractor produced content")
	}
	return nil, fmt.Errorf("failed to extract document after fallback attempts: requested=%s attempted=%s: %w", requestedStrategy, strings.Join(attempted, ","), lastErr)
}

func (p *ExtractProcessor) extractByStrategy(ctx context.Context, filePath, ext string, setting *ExtractSetting, strategy string) (*dto.ExtractOutput, error) {
	switch p.ResolveETLType(ext, strategy) {
	case ETLTypeUnstructured:
		output, err := p.unstructuredExtract(ctx, filePath, ext, setting.UploadFile, setting.ProcessRule)
		if err != nil {
			return nil, err
		}
		return p.persistMarkdownImageAssets(ctx, output, setting.UploadFile), nil
	case ETLTypeLandingAI:
		output, err := p.landingAIExtract(ctx, filePath, ext, setting.UploadFile)
		if err != nil {
			return nil, err
		}
		return p.persistMarkdownImageAssets(ctx, output, setting.UploadFile), nil
	case ETLTypeHyperparse:
		output, err := p.hyperparseExtract(ctx, filePath, strategy, setting.UploadFile)
		if err != nil {
			return nil, err
		}
		output = p.processFigureElements(ctx, output, setting.UploadFile)
		return p.persistMarkdownImageAssets(ctx, output, setting.UploadFile), nil
	default:
		output, err := p.defaultExtract(ctx, filePath, ext, setting.UploadFile)
		if err != nil {
			return nil, err
		}
		return p.persistMarkdownImageAssets(ctx, output, setting.UploadFile), nil
	}
}

func normalizeExtractionStrategy(strategy string) string {
	return strings.ToLower(strings.TrimSpace(strategy))
}

func uploadFileExtractionExtension(uploadFile *model.UploadFile) string {
	if uploadFile == nil {
		return ""
	}

	for _, raw := range []string{
		filepath.Ext(uploadFile.Key),
		uploadFile.Extension,
		filepath.Ext(uploadFile.Name),
		uploadFileMimeTypeExtension(uploadFile.MimeType),
	} {
		ext := normalizeFileExtension(raw)
		if ext != "" {
			return ext
		}
	}
	return ""
}

func normalizeFileExtension(raw string) string {
	ext := strings.ToLower(strings.TrimSpace(raw))
	if ext == "" {
		return ""
	}
	if !strings.HasPrefix(ext, ".") {
		ext = "." + ext
	}
	if ext == "." {
		return ""
	}
	return ext
}

func uploadFileMimeTypeExtension(mimeType string) string {
	switch strings.ToLower(strings.TrimSpace(mimeType)) {
	case "application/pdf":
		return ".pdf"
	case "application/vnd.ms-excel":
		return ".xls"
	case "application/vnd.openxmlformats-officedocument.spreadsheetml.sheet":
		return ".xlsx"
	case "text/csv", "application/csv":
		return ".csv"
	case "text/plain":
		return ".txt"
	default:
		return ""
	}
}

func extractionFallbackEnabled(setting *ExtractSetting) bool {
	if setting == nil || setting.ExtractionFallbackEnabled == nil {
		return true
	}
	return *setting.ExtractionFallbackEnabled
}

func orderedExtractionStrategies(requested string, available []string) []string {
	availableSet := make(map[string]bool, len(available))
	for _, strategy := range available {
		normalized := normalizeExtractionStrategy(strategy)
		if normalized != "" {
			availableSet[normalized] = true
		}
	}

	order := []string{
		dto.DocumentExtractionStrategyHyperParseMineru,
		dto.DocumentExtractionStrategyHyperParseReducto,
		dto.DocumentExtractionStrategyLandingAI,
		dto.DocumentExtractionStrategyHyperParseLocal,
		dto.DocumentExtractionStrategyUnstructured,
	}

	strategies := make([]string, 0, len(order))
	seen := make(map[string]bool, len(order)+1)

	if requested != "" && availableSet[requested] {
		strategies = append(strategies, requested)
		seen[requested] = true
	}

	for _, strategy := range order {
		if availableSet[strategy] && !seen[strategy] {
			strategies = append(strategies, strategy)
			seen[strategy] = true
		}
	}

	return strategies
}

func hasExtractedContent(output *dto.ExtractOutput) bool {
	return output != nil && (len(output.Elements) > 0 || strings.TrimSpace(output.Markdown) != "")
}

func attachExtractionMetadata(output *dto.ExtractOutput, requested, actual string, attempts []extractionAttempt) {
	if output == nil {
		return
	}
	if output.Metadata == nil {
		output.Metadata = map[string]any{}
	}

	copiedAttempts := make([]map[string]any, 0, len(attempts))
	for _, attempt := range attempts {
		item := map[string]any{
			"strategy": attempt.Strategy,
			"etl_type": attempt.ETLType,
			"success":  attempt.Success,
		}
		if attempt.Error != "" {
			item["error"] = attempt.Error
		}
		copiedAttempts = append(copiedAttempts, item)
	}

	output.Metadata["extraction"] = map[string]any{
		"requested_strategy": requested,
		"actual_strategy":    actual,
		"fallback_used":      requested != "" && requested != actual,
		"attempts":           copiedAttempts,
	}
}

func isExcelExtension(ext string) bool {
	switch strings.ToLower(strings.TrimSpace(ext)) {
	case ".xls", ".xlsx":
		return true
	default:
		return false
	}
}

func (p *ExtractProcessor) selectOptimalETLMethod(ext, extractionStrategy string) string {
	switch ext {
	case ".pdf":
		if selected := p.selectPDFExtractorByStrategy(ext, extractionStrategy, ""); selected != "" {
			return selected
		}

		logger.Debug("etl extractor selected", "etl_type", ETLTypeHyperparse, "extension", ext)
		return ETLTypeHyperparse
	case ".doc", ".docx", ".ppt", ".pptx", ".xls", ".xlsx":
		if config.GlobalConfig.ETL.UnstructuredAPIKey != "" {
			return ETLTypeUnstructured
		}
	case ".jpg", ".jpeg", ".png", ".apng", ".bmp", ".gif", ".webp", ".tiff", ".tif", ".psd", ".pcx", ".dcx", ".dds", ".jp2", ".ppm", ".tga", ".dib", ".icns", ".gd":
		// image: Reducto > LandingAI > Unstructured
		if config.GlobalConfig.ETL.ReductoAPIKey != "" {
			logger.Debug("etl extractor selected", "etl_type", ETLTypeReducto, "extension", ext)
			return ETLTypeReducto
		}
		if config.GlobalConfig.ETL.LandingAIAPIKey != "" {
			return ETLTypeLandingAI
		}
		// else use Unstructured
		if config.GlobalConfig.ETL.UnstructuredAPIKey != "" {
			return ETLTypeUnstructured
		}
	}

	// default
	return ETLTypeDefault
}

func (p *ExtractProcessor) selectPDFExtractorByStrategy(ext, extractionStrategy, fallback string) string {
	if ext != ".pdf" {
		return ""
	}

	switch strings.ToLower(strings.TrimSpace(extractionStrategy)) {
	case "", "auto":
		return ""
	case "mineru", "hyperparse":
		return ETLTypeHyperparse
	case "landingai", "landing_ai":
		if config.GlobalConfig.ETL.LandingAIAPIKey != "" {
			return ETLTypeLandingAI
		}
	case "local", "zgi", "default":
		return ETLTypeHyperparse
	}

	if fallback != "" {
		return fallback
	}
	return ""
}

func (p *ExtractProcessor) unstructuredExtract(ctx context.Context, filePath, ext string, uploadFile *model.UploadFile, processRule *dataset_model.DatasetProcessRule) (*dto.ExtractOutput, error) {
	var extractor BaseExtractor
	unstructuredAPIURL := config.GlobalConfig.ETL.UnstructuredAPIURL
	unstructuredAPIKey := config.GlobalConfig.ETL.UnstructuredAPIKey

	// Check if OCR is enabled
	enableOCR := false
	enhanceFormula := false
	if processRule != nil {
		// Parse rules to check image_content_recognition setting
		if preProcessingRules, ok := processRule.Rules["pre_processing_rules"].([]interface{}); ok {
			for _, rule := range preProcessingRules {
				if ruleMap, ok := rule.(map[string]interface{}); ok {
					if id, ok := ruleMap["id"].(string); ok && id == "image_content_recognition" {
						if enabled, ok := ruleMap["enabled"].(bool); ok {
							enableOCR = enabled
							break
						}
					}
					// Check for formula accuracy enhancement setting
					if id, ok := ruleMap["id"].(string); ok && id == "formula_accuracy_enhance" {
						if enabled, ok := ruleMap["enabled"].(bool); ok {
							enhanceFormula = enabled
						}
					}
				}
			}
		}
	}

	// Create common options for all unstructured extractors
	extractorOptions := &unstructured.ExtractorOptions{
		EnableOCR:      enableOCR,
		EnhanceFormula: enhanceFormula,
	}

	switch ext {
	// case ".csv":
	// 	extractor = NewCSVExtractor(filePath)
	case ".doc":
		extractor = unstructured.NewUnstructuredWordExtractor(filePath, unstructuredAPIURL, unstructuredAPIKey, extractorOptions)
	case ".docx":
		extractor = NewWordExtractor(filePath, uploadFile.OrganizationID, uploadFile.CreatedBy)
	// case ".eml":
	// 	extractor = NewUnstructuredEmailExtractor(filePath, p.cfg.GetString("UNSTRUCTURED_API_URL", ""), p.cfg.GetString("UNSTRUCTURED_API_KEY", ""))
	case ".htm", ".html":
		extractor = NewHtmlExtractor(filePath)
	case ".md", ".markdown", ".mdx":
		extractor = NewTextExtractor(filePath, "", true)
	case ".pdf":
		// Decide whether to enable OCR based on enableOCR parameter
		extractor = unstructured.NewUnstructuredPDFExtractor(filePath, unstructuredAPIURL, unstructuredAPIKey, extractorOptions)
		// if enableOCR {
		// 	extractor = unstructured.NewUnstructuredPDFExtractorWithOCR(filePath, unstructuredAPIURL, unstructuredAPIKey, true)
		// } else {
		// 	extractor = NewPdfExtractor(filePath)
		// }
	case ".ppt", ".pptx":
		extractor = unstructured.NewUnstructuredPPTExtractor(filePath, unstructuredAPIURL, unstructuredAPIKey, extractorOptions)
	case ".xls", ".xlsx":
		extractor = NewExcelExtractor(filePath)
	// case ".msg":
	// 	extractor = NewUnstructuredMsgExtractor(filePath, p.cfg.GetString("UNSTRUCTURED_API_URL", ""), p.cfg.GetString("UNSTRUCTURED_API_KEY", ""))
	// case ".xml":
	// 	extractor = NewUnstructuredXmlExtractor(filePath, p.cfg.GetString("UNSTRUCTURED_API_URL", ""), p.cfg.GetString("UNSTRUCTURED_API_KEY", ""))
	// case ".epub":
	// 	extractor = NewUnstructuredEpubExtractor(filePath, p.cfg.GetString("UNSTRUCTURED_API_URL", ""), p.cfg.GetString("UNSTRUCTURED_API_KEY", ""))
	default:
		extractor = NewTextExtractor(filePath, "", true)
	}

	if extractor == nil {
		return nil, errors.New("no extractor registered for this file type")
	}

	return extractor.Extract(ctx)
}

func (p *ExtractProcessor) landingAIExtract(ctx context.Context, filePath, ext string, uploadFile *model.UploadFile) (*dto.ExtractOutput, error) {
	var extractor BaseExtractor
	landingAIAPIKey := config.GlobalConfig.ETL.LandingAIAPIKey
	switch ext {
	case ".pdf":
		extractor = landingai.NewLandingAIPDFExtractor(filePath, landingAIAPIKey)
	case ".htm", ".html":
		extractor = NewHtmlExtractor(filePath)
	case ".md", ".markdown", ".mdx":
		extractor = NewTextExtractor(filePath, "", true)
	case ".xls", ".xlsx":
		extractor = NewExcelExtractor(filePath)
	case ".doc", ".docx":
		// LandingAI does not support Word documents, use default extractor
		extractor = NewWordExtractor(filePath, uploadFile.OrganizationID, uploadFile.CreatedBy)
	case ".ppt", ".pptx":
		// LandingAI does not support PowerPoint documents, use text extractor as fallback
		extractor = NewTextExtractor(filePath, "", true)
	default:
		extractor = NewTextExtractor(filePath, "", true)
	}

	if extractor == nil {
		return nil, errors.New("no extractor registered for this file type")
	}

	return extractor.Extract(ctx)
}

func (p *ExtractProcessor) hyperparseExtract(ctx context.Context, filePath, extractionStrategy string, uploadFile *model.UploadFile) (*dto.ExtractOutput, error) {
	if ext := strings.ToLower(filepath.Ext(filePath)); ext == ".md" || ext == ".markdown" || ext == ".mdx" || ext == ".txt" {
		extractionStrategy = "local"
	}

	extractor := hyperparse.NewHyperparseExtractorWithStorage(filePath, extractionStrategy, p.storage, mineruAssetNamespace(uploadFile))
	return extractor.Extract(ctx)
}

func mineruAssetNamespace(uploadFile *model.UploadFile) string {
	if uploadFile == nil {
		return ""
	}
	parts := make([]string, 0, 2)
	if organizationID := strings.TrimSpace(uploadFile.OrganizationID); organizationID != "" {
		parts = append(parts, organizationID)
	}
	if id := strings.TrimSpace(uploadFile.ID); id != "" {
		parts = append(parts, id)
	}
	return strings.Join(parts, "/")
}

func (p *ExtractProcessor) reductoExtract(ctx context.Context, filePath, ext string, uploadFile *model.UploadFile) (*dto.ExtractOutput, error) {
	var extractor BaseExtractor
	reductoAPIKey := config.GlobalConfig.ETL.ReductoAPIKey

	// Determine if this is an OCR-capable format (PDF or images)
	isOCRFormat := false
	switch ext {
	case ".pdf", ".png", ".jpg", ".jpeg", ".gif", ".bmp", ".tiff", ".tif",
		".pcx", ".ppm", ".apng", ".psd", ".cur", ".dcx", ".heic", ".doc":
		isOCRFormat = true
	}

	// Check OCR quota BEFORE calling Reducto API
	if isOCRFormat && p.quotaService != nil && p.db != nil && uploadFile != nil {
		// Calculate expected page count
		expectedPages, err := p.calculateExpectedOCRPages(filePath, ext)
		if err != nil {
			logger.WarnContext(ctx, "failed to calculate expected ocr pages", "file_id", uploadFile.ID, "extension", ext, err)
			// Continue with default estimation
			expectedPages = 1
		}

		// Get groupID from tenantID
		// In this system, tenantID IS the groupID
		var groupID *uuid.UUID
		parsedGroupID, parseErr := uuid.Parse(uploadFile.OrganizationID)
		if parseErr == nil {
			groupID = &parsedGroupID
		} else {
			logger.WarnContext(ctx, "failed to parse tenant id for ocr quota", "tenant_id", uploadFile.OrganizationID, "file_id", uploadFile.ID, parseErr)
		}

		if groupID != nil {
			// Check quota before processing
			canProceed, used, limit, err := p.quotaService.CheckQuota(
				context.Background(),
				*groupID,
				quota_model.ResourceTypeOCRPages,
				int64(expectedPages),
			)

			if err != nil {
				return nil, fmt.Errorf("检查OCR配额失败: %w", err)
			}

			if !canProceed {
				return nil, fmt.Errorf("OCR quota exceeded: used %d/%d pages, required %d pages", used, limit, expectedPages)
			}

			logger.DebugContext(ctx, "ocr quota check passed", "file_id", uploadFile.ID, "expected_pages", expectedPages, "used", used, "limit", limit)
		}
	}

	// We'll record OCR usage after extraction when we know the actual page count

	switch ext {
	case ".pdf":
		extractor = reducto.NewReductoPDFExtractor(filePath, reductoAPIKey)
	// Image formats supported by Reducto
	case ".png", ".jpg", ".jpeg", ".gif", ".bmp", ".tiff", ".tif",
		".pcx", ".ppm", ".apng", ".psd", ".cur", ".dcx", ".heic":
		extractor = reducto.NewReductoImageExtractor(filePath, reductoAPIKey)
	// Spreadsheet formats
	// ".csv", ".xlsx", ".xlsm", ".xls", ".xltx", ".xltm", ".qpw"
	case ".xlsx", ".xls":
		extractor = NewExcelExtractor(filePath)
	// Document formats
	case ".txt", ".rtf":
		extractor = NewTextExtractor(filePath, "", true)
	case ".htm", ".html":
		extractor = NewHtmlExtractor(filePath)
	case ".md", ".markdown", ".mdx":
		extractor = NewTextExtractor(filePath, "", true)
	case ".doc":
		extractor = reducto.NewReductoPDFExtractor(filePath, reductoAPIKey)
	case ".docx":
		// Reducto does not support Word documents, use default extractor
		extractor = NewWordExtractor(filePath, uploadFile.OrganizationID, uploadFile.CreatedBy)
	// Presentation formats
	case ".ppt", ".pptx":
		// Reducto supports these formats via API
		extractor = reducto.NewReductoPDFExtractor(filePath, reductoAPIKey)

	default:
		extractor = NewTextExtractor(filePath, "", true)
	}

	if extractor == nil {
		return nil, errors.New("no extractor registered for this file type")
	}

	output, err := extractor.Extract(ctx)
	if err != nil {
		return nil, err
	}

	// Record OCR usage if applicable and quota service is available
	if isOCRFormat && p.quotaService != nil && p.db != nil && uploadFile != nil {
		// Try to get usage information from the extractor
		var usage *reducto.ParseUsage
		switch e := extractor.(type) {
		case *reducto.ReductoPDFExtractor:
			usage = e.GetLastUsage()
		case *reducto.ReductoImageExtractor:
			usage = e.GetLastUsage()
		}

		if usage != nil && usage.NumPages > 0 {
			// Get groupID from tenantID
			// In this system, tenantID IS the groupID
			var groupID *uuid.UUID
			parsedGroupID, parseErr := uuid.Parse(uploadFile.OrganizationID)
			if parseErr == nil {
				groupID = &parsedGroupID
			} else {
				logger.WarnContext(ctx, "failed to parse tenant id for ocr usage", "tenant_id", uploadFile.OrganizationID, "file_id", uploadFile.ID, parseErr)
			}

			if groupID != nil {
				// Parse IDs
				accountUUID, err := uuid.Parse(uploadFile.CreatedBy)
				if err != nil {
					logger.WarnContext(ctx, "failed to parse account id for ocr usage", "account_id", uploadFile.CreatedBy, "file_id", uploadFile.ID, err)
				} else {
					tenantUUID, err := uuid.Parse(uploadFile.OrganizationID)
					if err != nil {
						logger.WarnContext(ctx, "failed to parse tenant id for ocr usage", "tenant_id", uploadFile.OrganizationID, "file_id", uploadFile.ID, err)
					} else {
						// Record OCR usage in a transaction
						err := p.db.Transaction(func(tx *gorm.DB) error {
							// Create usage history record
							usageRecord := &quota_model.QuotaUsageHistory{
								ID:           uuid.New().String(),
								GroupID:      *groupID,
								AccountID:    accountUUID,
								TenantID:     &tenantUUID,
								ResourceType: quota_model.ResourceTypeOCRPages,
								Delta:        int64(usage.NumPages),
								ResourceID:   &uploadFile.ID,
								ResourceName: &uploadFile.Name,
								Metadata: &quota_model.JSONMap{
									"file_id":   uploadFile.ID,
									"file_name": uploadFile.Name,
									"num_pages": usage.NumPages,
								},
							}

							// Add credits if available
							if usage.Credits != nil {
								(*usageRecord.Metadata)["credits"] = *usage.Credits
							}

							return p.quotaService.RecordUsageInTx(context.Background(), tx, usageRecord)
						})

						if err != nil {
							// Log the error but don't fail the extraction
							logger.WarnContext(ctx, "failed to record ocr usage", "file_id", uploadFile.ID, err)
						}
					}
				}
			}
		}
	}

	return output, nil
}

func (p *ExtractProcessor) defaultExtract(ctx context.Context, filePath, ext string, uploadFile *model.UploadFile) (*dto.ExtractOutput, error) {
	var extractor BaseExtractor
	switch ext {
	case ".csv":
		extractor = NewCSVExtractor(filePath)
	case ".docx":
		extractor = NewWordExtractor(filePath, uploadFile.OrganizationID, uploadFile.CreatedBy)
	case ".htm", ".html":
		extractor = NewHtmlExtractor(filePath)
	case ".md", ".markdown", ".mdx":
		extractor = NewTextExtractor(filePath, "", true)
	case ".pdf":
		extractor = NewPdfExtractor(filePath)
	case ".xls", ".xlsx":
		extractor = NewExcelExtractor(filePath)
	default:
		extractor = NewTextExtractor(filePath, "", true)
	}

	if extractor == nil {
		return nil, errors.New("no extractor registered for this file type")
	}

	// Extract documents
	return extractor.Extract(ctx)
}

// calculateExpectedOCRPages calculates the expected number of OCR pages for a file
// This is used for quota checking BEFORE calling Reducto API
func (p *ExtractProcessor) calculateExpectedOCRPages(filePath, ext string) (int, error) {
	switch ext {
	case ".pdf":
		// For PDF files, read the page count without parsing content
		return getQuickPDFPageCount(filePath)
	case ".png", ".jpg", ".jpeg", ".gif", ".bmp", ".tiff", ".tif",
		".pcx", ".ppm", ".apng", ".psd", ".cur", ".dcx", ".heic":
		// For image files, count as 1 page
		return 1, nil
	default:
		// Unknown format, estimate as 1 page
		return 1, nil
	}
}

// getQuickPDFPageCount quickly reads the page count from a PDF file without parsing content
func getQuickPDFPageCount(filePath string) (int, error) {
	// Disable debug output
	pdf.DebugOn = false

	// Open PDF file
	file, reader, err := pdf.Open(filePath)
	if err != nil {
		return 0, fmt.Errorf("failed to open PDF: %w", err)
	}
	defer file.Close()

	// Get the number of pages
	numPages := reader.NumPage()
	return numPages, nil
}

func getFileExtensionFromURL(url string, header http.Header) string {
	if strings.Contains(url, ".") {
		parts := strings.Split(url, ".")
		return "." + parts[len(parts)-1]
	}

	contentType := header.Get("Content-Type")
	if contentType != "" {
		parts := strings.Split(contentType, "/")
		if len(parts) == 2 {
			return "." + parts[1]
		}
	}

	return ""
}

type ExtractorFactory struct {
	extractors map[string]func(filePath string) BaseExtractor
}

func NewExtractorFactory() *ExtractorFactory {
	return &ExtractorFactory{
		extractors: make(map[string]func(filePath string) BaseExtractor),
	}
}

func (f *ExtractorFactory) Register(ext string, factory func(filePath string) BaseExtractor) {
	f.extractors[ext] = factory
}

func (f *ExtractorFactory) GetExtractor(ext string) (BaseExtractor, error) {
	factory, ok := f.extractors[ext]
	if !ok {
		return nil, errors.New("no extractor registered for this file type")
	}
	return factory(""), nil
}
