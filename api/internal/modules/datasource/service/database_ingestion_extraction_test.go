package service

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/zgiai/zgi/api/internal/dto"
	llmdefaultsvc "github.com/zgiai/zgi/api/internal/modules/llm/defaultmodel/service"
	llmmodelmodel "github.com/zgiai/zgi/api/internal/modules/llm/llmmodel/model"
	sharedmodel "github.com/zgiai/zgi/api/internal/modules/shared/model"
	"github.com/zgiai/zgi/api/internal/prompt"
)

type fakeDatabaseIngestionVisionResolver struct {
	resolved         *llmdefaultsvc.ResolvedModel
	err              error
	gotUseCase       llmmodelmodel.UseCase
	gotProvider      string
	gotModel         string
	gotExplicitModel bool
}

func (f *fakeDatabaseIngestionVisionResolver) ResolveUseCase(_ context.Context, _ string, useCase llmmodelmodel.UseCase, explicitProvider, explicitModel *string) (*llmdefaultsvc.ResolvedModel, error) {
	f.gotUseCase = useCase
	if explicitProvider != nil {
		f.gotProvider = *explicitProvider
	}
	if explicitModel != nil {
		f.gotModel = *explicitModel
		f.gotExplicitModel = true
	}
	if f.err != nil {
		return nil, f.err
	}
	return f.resolved, nil
}

func (f *fakeDatabaseIngestionVisionResolver) ResolveModelType(context.Context, string, *string, *string, sharedmodel.ModelType) (*llmdefaultsvc.ResolvedModel, error) {
	return nil, errors.New("not implemented")
}

func TestDatabaseIngestionFileTypeHelpers(t *testing.T) {
	if !isDatabaseIngestionImageFile("png", "") {
		t.Fatal("png extension should be treated as image")
	}
	if !isDatabaseIngestionImageFile("", "image/jpeg") {
		t.Fatal("image MIME type should be treated as image")
	}
	if isDatabaseIngestionImageFile("pdf", "application/pdf") {
		t.Fatal("pdf should not be treated as image")
	}
	if !isDatabaseIngestionPDFFile("pdf", "") {
		t.Fatal("pdf extension should be treated as pdf")
	}
	if !isDatabaseIngestionPDFFile("", "application/pdf") {
		t.Fatal("application/pdf MIME type should be treated as pdf")
	}
}

func TestImageBytesDataURLDetectsContentType(t *testing.T) {
	dataURL := imageBytesDataURL([]byte{0x89, 0x50, 0x4e, 0x47, 0x0d, 0x0a, 0x1a, 0x0a}, "", "")
	if !strings.HasPrefix(dataURL, "data:image/png;base64,") {
		t.Fatalf("data URL prefix = %q, want image/png", dataURL)
	}
}

func TestShouldRetryDatabaseIngestionPDFWithVisionOnlyForMinerUZeroRecords(t *testing.T) {
	fileInfo := &dto.UploadFile{Extension: "pdf", MimeType: "application/pdf"}
	extraction := databaseIngestionExtractionResult{
		Content:         "mineru text",
		PrimaryStrategy: dto.DocumentExtractionStrategyHyperParseMineru,
		ActualStrategy:  dto.DocumentExtractionStrategyHyperParseMineru,
	}

	if !shouldRetryDatabaseIngestionPDFWithVision(fileInfo, extraction, nil) {
		t.Fatal("expected pdf mineru zero-record extraction to request vision retry")
	}

	if shouldRetryDatabaseIngestionPDFWithVision(fileInfo, extraction, []map[string]interface{}{{"invoice_no": "INV-1"}}) {
		t.Fatal("should not retry when a record was recognized")
	}

	imageInfo := &dto.UploadFile{Extension: "png", MimeType: "image/png"}
	if shouldRetryDatabaseIngestionPDFWithVision(imageInfo, extraction, nil) {
		t.Fatal("should not retry non-pdf files")
	}

	visionExtraction := extraction
	visionExtraction.ActualStrategy = databaseIngestionStrategyVision
	if shouldRetryDatabaseIngestionPDFWithVision(fileInfo, visionExtraction, nil) {
		t.Fatal("should not retry after vision was already used")
	}

	emptyExtraction := extraction
	emptyExtraction.Content = " "
	if shouldRetryDatabaseIngestionPDFWithVision(fileInfo, emptyExtraction, nil) {
		t.Fatal("empty mineru content is handled by the existing empty fallback path")
	}
}

func TestFileIngestExtractionInfoIncludesSourceAndContentHash(t *testing.T) {
	info := fileIngestExtractionInfo(databaseIngestionExtractionResult{
		PrimaryStrategy: dto.DocumentExtractionStrategyHyperParseMineru,
		ActualStrategy:  databaseIngestionStrategyVision,
		FallbackReason:  databaseIngestionFallbackZeroRecords,
		SourceType:      databaseIngestionSourcePDFRendered,
	}, "recognized content")

	if info.SourceType != databaseIngestionSourcePDFRendered {
		t.Fatalf("source type = %q, want %q", info.SourceType, databaseIngestionSourcePDFRendered)
	}
	if info.ContentHash == "" {
		t.Fatal("content hash should be populated")
	}
	if info.PrimaryStrategy != dto.DocumentExtractionStrategyHyperParseMineru ||
		info.ActualStrategy != databaseIngestionStrategyVision ||
		info.FallbackReason != databaseIngestionFallbackZeroRecords {
		t.Fatalf("extraction info = %#v", info)
	}
}

func TestIsDataSourceTableNotFound(t *testing.T) {
	if !IsDataSourceTableNotFound(errDataSourceTableNotFound) {
		t.Fatal("sentinel error should be recognized")
	}
	wrapped := errors.New("another error")
	if IsDataSourceTableNotFound(wrapped) {
		t.Fatal("unrelated error should not be recognized")
	}
}

func TestDatasourceFileConversionPromptConstrainsDatesMoneyAndPartialRecords(t *testing.T) {
	tmpl, err := prompt.GetTemplate(prompt.DatasourceFileConversion)
	if err != nil {
		t.Fatalf("GetTemplate returned error: %v", err)
	}

	raw := tmpl.RawContent()
	for _, want := range []string{
		"Do not return Excel serial dates",
		"45160",
		"explicitly asks for an Excel serial date",
		"keep the original major currency unit",
		"never 3260000 cents/fen",
		"return one record with the recognized values",
		"Use null or an empty string for missing fields",
	} {
		if !strings.Contains(raw, want) {
			t.Fatalf("prompt template missing %q", want)
		}
	}
}

func TestResolveDatabaseIngestionVisionModelRejectsExplicitNonVision(t *testing.T) {
	resolver := &fakeDatabaseIngestionVisionResolver{err: errors.New("model unavailable")}
	svc := &dataSourceService{defaultModelResolver: resolver}

	_, err := svc.resolveDatabaseIngestionVisionModel(context.Background(), "org-1", &dto.ModelSpec{
		Provider: "deepseek",
		Name:     "deepseek-chat",
	})
	if err == nil || !strings.Contains(err.Error(), "selected model does not support image input") {
		t.Fatalf("error = %v, want explicit non-vision model message", err)
	}
	if resolver.gotUseCase != llmmodelmodel.UseCaseVision {
		t.Fatalf("use case = %q, want vision", resolver.gotUseCase)
	}
	if resolver.gotProvider != "deepseek" || resolver.gotModel != "deepseek-chat" || !resolver.gotExplicitModel {
		t.Fatalf("explicit model not forwarded: provider=%q model=%q explicit=%v", resolver.gotProvider, resolver.gotModel, resolver.gotExplicitModel)
	}
}

func TestResolveDatabaseIngestionVisionModelAcceptsVisionModel(t *testing.T) {
	resolver := &fakeDatabaseIngestionVisionResolver{
		resolved: &llmdefaultsvc.ResolvedModel{
			UseCase:  string(llmmodelmodel.UseCaseVision),
			Provider: "openai",
			Model:    "gpt-4o",
		},
	}
	svc := &dataSourceService{defaultModelResolver: resolver}

	resolved, err := svc.resolveDatabaseIngestionVisionModel(context.Background(), "org-1", &dto.ModelSpec{
		Provider: "openai",
		Name:     "gpt-4o",
	})
	if err != nil {
		t.Fatalf("resolveDatabaseIngestionVisionModel returned error: %v", err)
	}
	if resolved.Provider != "openai" || resolved.Model != "gpt-4o" {
		t.Fatalf("resolved = %#v, want openai/gpt-4o", resolved)
	}
}
