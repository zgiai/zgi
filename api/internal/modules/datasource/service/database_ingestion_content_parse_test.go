package service

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/zgiai/zgi/api/internal/contracts"
	"github.com/zgiai/zgi/api/internal/dto"
	filemodel "github.com/zgiai/zgi/api/internal/modules/file_process/model"
	sharedinterfaces "github.com/zgiai/zgi/api/internal/modules/shared/interface"
)

type fakeContentParseService struct {
	artifact *contracts.ParseArtifact
	err      error
	gotReq   contracts.ParseRequest
}

func (f *fakeContentParseService) Parse(_ context.Context, req contracts.ParseRequest) (*contracts.ParseArtifact, error) {
	f.gotReq = req
	if f.err != nil {
		return nil, f.err
	}
	return f.artifact, nil
}

func (f *fakeContentParseService) Health(context.Context) (*contracts.ParseHealth, error) {
	return nil, nil
}

type fakeRoutedContentParseService struct {
	fakeContentParseService
	routeCalls int
}

func (f *fakeRoutedContentParseService) ParseWithRouting(_ context.Context, req contracts.ParseRequest) (*contracts.ParseArtifact, error) {
	f.routeCalls++
	f.gotReq = req
	if f.err != nil {
		return nil, f.err
	}
	return f.artifact, nil
}

type fakeDatabaseIngestionFileService struct {
	data           []byte
	downloadErr    error
	extractContent string
	extractErr     error
	extractCalls   int
	uploadFile     *dto.UploadFile
	getFileErr     error
}

func (f *fakeDatabaseIngestionFileService) GetUploadConfig() *sharedinterfaces.FileUploadConfigResponse {
	return nil
}

func (f *fakeDatabaseIngestionFileService) UploadFile(context.Context, string, []byte, string, string, string, filemodel.CreatedByRole, *sharedinterfaces.FileSource, *string, bool, bool) (*dto.UploadFile, error) {
	return nil, errors.New("not implemented")
}
func (f *fakeDatabaseIngestionFileService) ReplaceFileContent(context.Context, string, string, []byte, string, string, string) (*dto.UploadFile, error) {
	return nil, errors.New("not implemented")
}
func (f *fakeDatabaseIngestionFileService) GetFilePreview(context.Context, string) (string, error) {
	return "", errors.New("not implemented")
}

func (f *fakeDatabaseIngestionFileService) GetFilePreviewWithOCR(context.Context, string, bool) (string, error) {
	return "", errors.New("not implemented")
}

func (f *fakeDatabaseIngestionFileService) GetFile(context.Context, string) (string, error) {
	return "", errors.New("not implemented")
}

func (f *fakeDatabaseIngestionFileService) ExtractFileWithSetting(context.Context, string, sharedinterfaces.FileExtractionSetting) (string, error) {
	f.extractCalls++
	return f.extractContent, f.extractErr
}

func (f *fakeDatabaseIngestionFileService) GetSupportedFileTypes() []string {
	return nil
}

func (f *fakeDatabaseIngestionFileService) IsFileSizeWithinLimit(string, int64) bool {
	return true
}

func (f *fakeDatabaseIngestionFileService) ParseFileContent(context.Context, string) {}

func (f *fakeDatabaseIngestionFileService) GetFileByID(context.Context, string) (*dto.UploadFile, error) {
	if f.getFileErr != nil {
		return nil, f.getFileErr
	}
	if f.uploadFile != nil {
		return f.uploadFile, nil
	}
	return nil, errors.New("not implemented")
}

func (f *fakeDatabaseIngestionFileService) DownloadFile(context.Context, string) ([]byte, error) {
	if f.downloadErr != nil {
		return nil, f.downloadErr
	}
	return f.data, nil
}

func (f *fakeDatabaseIngestionFileService) ListFiles(context.Context, string, string, *dto.FileListRequest, []string) (*dto.FileListResponse, error) {
	return nil, errors.New("not implemented")
}

func (f *fakeDatabaseIngestionFileService) ListArchivedFiles(context.Context, string, string, *dto.FileListRequest, []string) (*dto.FileListResponse, error) {
	return nil, errors.New("not implemented")
}

func (f *fakeDatabaseIngestionFileService) GetStorageUsage(context.Context, string) (int64, error) {
	return 0, errors.New("not implemented")
}

func (f *fakeDatabaseIngestionFileService) DeleteFiles(context.Context, []string) error {
	return errors.New("not implemented")
}

func (f *fakeDatabaseIngestionFileService) UpdateContentText(context.Context, string, string) error {
	return errors.New("not implemented")
}

func (f *fakeDatabaseIngestionFileService) CleanupExpiredTemporaryFiles(context.Context, time.Duration) (int64, error) {
	return 0, errors.New("not implemented")
}

func (f *fakeDatabaseIngestionFileService) GetFileURL(context.Context, string) (string, error) {
	return "", errors.New("not implemented")
}

func TestExtractDatabaseIngestionFileInfoContentUsesContentParseFirst(t *testing.T) {
	contentParse := &fakeContentParseService{
		artifact: &contracts.ParseArtifact{
			Markdown:     "parsed markdown",
			EngineUsed:   contracts.ParseEngineLocal,
			Status:       contracts.ParseStatusSucceeded,
			QualityLevel: contracts.ParseQualityStandard,
		},
	}
	fileService := &fakeDatabaseIngestionFileService{data: []byte("%PDF")}
	svc := &dataSourceService{fileService: fileService, contentParseService: contentParse}

	result, err := svc.extractDatabaseIngestionFileInfoContent(context.Background(), "acct-1", &dto.UploadFile{
		ID:        "file-1",
		Name:      "sample.pdf",
		Extension: "pdf",
		MimeType:  "application/pdf",
	})
	if err != nil {
		t.Fatalf("extractDatabaseIngestionFileInfoContent returned error: %v", err)
	}
	if result.Content != "parsed markdown" {
		t.Fatalf("content = %q, want parsed markdown", result.Content)
	}
	if result.PrimaryStrategy != databaseIngestionStrategyContentParse ||
		result.ActualStrategy != databaseIngestionStrategyContentParse ||
		result.SourceType != databaseIngestionSourceContentParse {
		t.Fatalf("extraction result = %#v", result)
	}
	if fileService.extractCalls != 0 {
		t.Fatalf("legacy parser calls = %d, want 0", fileService.extractCalls)
	}
	if contentParse.gotReq.Intent != contracts.ParseIntentDatasetIndex ||
		contentParse.gotReq.Profile != contracts.ParseProfileLayoutFirst ||
		contentParse.gotReq.SourceType != contracts.ParseSourceTypeBytes ||
		contentParse.gotReq.SourceRef != "file-1" ||
		string(contentParse.gotReq.Data) != "%PDF" {
		t.Fatalf("content parse request = %#v", contentParse.gotReq)
	}
	if len(result.Attempts) != 1 || result.Attempts[0].Method != databaseIngestionAttemptMethodFileParse ||
		result.Attempts[0].Result != databaseIngestionAttemptResultContent {
		t.Fatalf("attempts = %#v", result.Attempts)
	}
}

func TestParseDatabaseIngestionFileForTableReturnsParsedContent(t *testing.T) {
	contentParse := &fakeContentParseService{
		artifact: &contracts.ParseArtifact{
			Markdown:     "parsed table markdown",
			EngineUsed:   contracts.ParseEngineLocal,
			Status:       contracts.ParseStatusSucceeded,
			QualityLevel: contracts.ParseQualityStandard,
		},
	}
	fileService := &fakeDatabaseIngestionFileService{
		data: []byte("image bytes"),
		uploadFile: &dto.UploadFile{
			ID:        "file-stage-1",
			Name:      "sample.png",
			Extension: "png",
			MimeType:  "image/png",
		},
	}
	svc := &dataSourceService{fileService: fileService, contentParseService: contentParse}

	result := svc.parseDatabaseIngestionFileForTable(context.Background(), "acct-1", "file-stage-1")

	if result.Error != nil {
		t.Fatalf("parseDatabaseIngestionFileForTable error = %v", *result.Error)
	}
	if result.Stage != fileIngestStageParse {
		t.Fatalf("stage = %q, want %q", result.Stage, fileIngestStageParse)
	}
	if result.Content != "parsed table markdown" {
		t.Fatalf("content = %q, want parsed table markdown", result.Content)
	}
	if result.Extraction == nil || result.Extraction.ContentHash == "" {
		t.Fatalf("extraction = %#v, want content hash", result.Extraction)
	}
	if contentParse.gotReq.SourceRef != "file-stage-1" {
		t.Fatalf("content parse source ref = %q, want file-stage-1", contentParse.gotReq.SourceRef)
	}
}

func TestExtractDatabaseIngestionFileInfoContentUsesContentParseRouting(t *testing.T) {
	contentParse := &fakeRoutedContentParseService{
		fakeContentParseService: fakeContentParseService{
			artifact: &contracts.ParseArtifact{
				Markdown:     "mineru routed content",
				EngineUsed:   contracts.ParseEngineMineru,
				Status:       contracts.ParseStatusSucceeded,
				QualityLevel: contracts.ParseQualityStandard,
				Metadata: map[string]any{
					"executed_provider_key": "mineru",
					"executed_adapter_name": "hyperparse_sdk",
					"executed_engine_name":  contracts.ParseEngineMineru,
				},
			},
		},
	}
	fileService := &fakeDatabaseIngestionFileService{data: []byte("png")}
	svc := &dataSourceService{fileService: fileService, contentParseService: contentParse}

	result, err := svc.extractDatabaseIngestionFileInfoContent(context.Background(), "acct-1", &dto.UploadFile{
		ID:        "file-img",
		Name:      "sample.png",
		Extension: "png",
		MimeType:  "image/png",
	})
	if err != nil {
		t.Fatalf("extractDatabaseIngestionFileInfoContent returned error: %v", err)
	}
	if contentParse.routeCalls != 1 {
		t.Fatalf("route calls = %d, want 1", contentParse.routeCalls)
	}
	if result.Content != "mineru routed content" {
		t.Fatalf("content = %q, want routed content", result.Content)
	}
	if len(result.Attempts) != 1 || result.Attempts[0].Result != databaseIngestionAttemptResultContent {
		t.Fatalf("attempts = %#v", result.Attempts)
	}
	reason := result.Attempts[0].Reason
	if !strings.Contains(reason, "provider=mineru") ||
		!strings.Contains(reason, "adapter=hyperparse_sdk") ||
		!strings.Contains(reason, "engine=mineru") {
		t.Fatalf("attempt reason = %q, want route metadata", reason)
	}
	if contentParse.gotReq.Intent != contracts.ParseIntentDatasetIndex ||
		contentParse.gotReq.Profile != contracts.ParseProfileLayoutFirst {
		t.Fatalf("content parse request = %#v", contentParse.gotReq)
	}
}

func TestExtractDatabaseIngestionFileInfoContentFallsBackAfterContentParseError(t *testing.T) {
	contentParse := &fakeContentParseService{err: errors.New("parser unavailable")}
	fileService := &fakeDatabaseIngestionFileService{
		data:           []byte("doc"),
		extractContent: "legacy mineru text",
	}
	svc := &dataSourceService{fileService: fileService, contentParseService: contentParse}

	result, err := svc.extractDatabaseIngestionFileInfoContent(context.Background(), "acct-1", &dto.UploadFile{
		ID:        "file-2",
		Name:      "sample.docx",
		Extension: "docx",
		MimeType:  "application/vnd.openxmlformats-officedocument.wordprocessingml.document",
	})
	if err != nil {
		t.Fatalf("extractDatabaseIngestionFileInfoContent returned error: %v", err)
	}
	if result.Content != "legacy mineru text" {
		t.Fatalf("content = %q, want legacy fallback", result.Content)
	}
	if result.PrimaryStrategy != dto.DocumentExtractionStrategyHyperParseMineru ||
		result.SourceType != databaseIngestionSourceMinerU {
		t.Fatalf("fallback result = %#v", result)
	}
	if fileService.extractCalls != 1 {
		t.Fatalf("legacy parser calls = %d, want 1", fileService.extractCalls)
	}
	if len(result.Attempts) != 2 {
		t.Fatalf("attempt count = %d, want 2: %#v", len(result.Attempts), result.Attempts)
	}
	if result.Attempts[0].Status != databaseIngestionAttemptStatusFailed ||
		result.Attempts[0].Result != databaseIngestionAttemptResultError {
		t.Fatalf("first attempt = %#v", result.Attempts[0])
	}
	if result.Attempts[1].Status != databaseIngestionAttemptStatusCompleted ||
		result.Attempts[1].Result != databaseIngestionAttemptResultContent {
		t.Fatalf("second attempt = %#v", result.Attempts[1])
	}
}

func TestDatabaseIngestionContentFromParseArtifactFallsBackToElements(t *testing.T) {
	content := databaseIngestionContentFromParseArtifact(&contracts.ParseArtifact{
		Elements: []contracts.ParsedElement{
			{Page: 2, Ordinal: 1, Content: "second page"},
			{Page: 1, Ordinal: 2, Content: "first page second"},
			{Page: 1, Ordinal: 1, Content: "first page first"},
		},
	})
	if content != "first page first\n\nfirst page second\n\nsecond page" {
		t.Fatalf("content = %q", content)
	}
}
