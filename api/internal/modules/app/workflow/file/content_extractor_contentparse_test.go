package file

import (
	"context"
	"testing"
	"time"

	appconfig "github.com/zgiai/zgi/api/config"
	"github.com/zgiai/zgi/api/internal/contracts"
	"github.com/zgiai/zgi/api/internal/dto"
	interfaces "github.com/zgiai/zgi/api/internal/modules/shared/interface"
)

type routedContentParseStub struct {
	req   contracts.ParseRequest
	calls int
}

func (s *routedContentParseStub) Parse(ctx context.Context, req contracts.ParseRequest) (*contracts.ParseArtifact, error) {
	return s.ParseWithRouting(ctx, req)
}

func (s *routedContentParseStub) ParseWithRouting(_ context.Context, req contracts.ParseRequest) (*contracts.ParseArtifact, error) {
	s.calls++
	s.req = req
	return &contracts.ParseArtifact{
		Status:       contracts.ParseStatusSucceeded,
		QualityLevel: contracts.ParseQualityStandard,
		Text:         "plain content",
		Markdown:     "# Routed content",
		Elements: []contracts.ParsedElement{
			{Type: "text", Content: "Routed content"},
		},
	}, nil
}

func (s *routedContentParseStub) Health(context.Context) (*contracts.ParseHealth, error) {
	return &contracts.ParseHealth{}, nil
}

type contentParseFileServiceStub struct {
	interfaces.FileService
	file           *dto.UploadFile
	data           []byte
	updatedFileID  string
	updatedContent string
}

func (s *contentParseFileServiceStub) GetFileByID(context.Context, string) (*dto.UploadFile, error) {
	return s.file, nil
}

func (s *contentParseFileServiceStub) DownloadFile(context.Context, string) ([]byte, error) {
	return s.data, nil
}

func (s *contentParseFileServiceStub) UpdateContentText(_ context.Context, fileID string, content string) error {
	s.updatedFileID = fileID
	s.updatedContent = content
	return nil
}

func TestContentExtractorUsesWorkflowScopeForTemporaryUpload(t *testing.T) {
	organizationID := "11111111-1111-1111-1111-111111111111"
	workspaceID := "22222222-2222-2222-2222-222222222222"
	fileService := &contentParseFileServiceStub{
		file: &dto.UploadFile{
			ID:             "file-1",
			TenantID:       appconfig.TempFileTenantID,
			OrganizationID: appconfig.TempFileTenantID,
			Name:           "report.pdf",
			Extension:      "pdf",
			MimeType:       "application/pdf",
			CreatedBy:      "33333333-3333-3333-3333-333333333333",
			WorkspaceID:    &workspaceID,
			IsTemporary:    true,
		},
		data: []byte("pdf bytes"),
	}
	parser := &routedContentParseStub{}
	extractor := NewContentExtractor(
		fileService,
		nil,
		&Config{
			Enabled:           true,
			MaxContentSize:    1024,
			ExtractionTimeout: time.Second,
			CacheEnabled:      true,
		},
		WithContentParseService(parser),
	)

	content, err := extractor.ExtractFileContent(context.Background(), "file-1", ContentExtractionScope{
		OrganizationID: organizationID,
		WorkspaceID:    workspaceID,
	})
	if err != nil {
		t.Fatalf("ExtractFileContent() error = %v", err)
	}
	if parser.calls != 1 {
		t.Fatalf("routed parser calls = %d, want 1", parser.calls)
	}
	if content.Content != "# Routed content" || content.FromCache {
		t.Fatalf("content = %#v", content)
	}
	if parser.req.Intent != contracts.ParseIntentChatContext || parser.req.Profile != contracts.ParseProfileAuto {
		t.Fatalf("parse request intent/profile = %q/%q", parser.req.Intent, parser.req.Profile)
	}
	if parser.req.Metadata["organization_id"] != organizationID {
		t.Fatalf("organization metadata = %#v", parser.req.Metadata["organization_id"])
	}
	if parser.req.Metadata["workspace_id"] != workspaceID {
		t.Fatalf("workspace metadata = %#v", parser.req.Metadata["workspace_id"])
	}
	if fileService.updatedFileID != "file-1" || fileService.updatedContent != "# Routed content" {
		t.Fatalf("cache update = %q/%q", fileService.updatedFileID, fileService.updatedContent)
	}
}

func TestContentExtractorFallsBackToUploadOrganizationWhenScopeIsPartial(t *testing.T) {
	organizationID := "44444444-4444-4444-4444-444444444444"
	workspaceID := "55555555-5555-5555-5555-555555555555"
	fileService := &contentParseFileServiceStub{
		file: &dto.UploadFile{
			ID:        "file-legacy-scope",
			TenantID:  organizationID,
			Name:      "legacy-scope.pdf",
			Extension: "pdf",
			MimeType:  "application/pdf",
			CreatedBy: "66666666-6666-6666-6666-666666666666",
		},
		data: []byte("pdf bytes"),
	}
	parser := &routedContentParseStub{}
	extractor := NewContentExtractor(
		fileService,
		nil,
		&Config{
			Enabled:           true,
			MaxContentSize:    1024,
			ExtractionTimeout: time.Second,
			CacheEnabled:      true,
		},
		WithContentParseService(parser),
	)

	if _, err := extractor.ExtractFileContent(context.Background(), "file-legacy-scope", ContentExtractionScope{
		WorkspaceID: workspaceID,
	}); err != nil {
		t.Fatalf("ExtractFileContent() error = %v", err)
	}
	if parser.req.Metadata["organization_id"] != organizationID {
		t.Fatalf("organization metadata = %#v", parser.req.Metadata["organization_id"])
	}
	if parser.req.Metadata["workspace_id"] != workspaceID {
		t.Fatalf("workspace metadata = %#v", parser.req.Metadata["workspace_id"])
	}
}

func TestContentExtractorKeepsCachedContentFastPath(t *testing.T) {
	cached := "cached content"
	fileService := &contentParseFileServiceStub{file: &dto.UploadFile{
		ID:          "file-2",
		Name:        "cached.pdf",
		Extension:   "pdf",
		MimeType:    "application/pdf",
		ContentText: &cached,
	}}
	parser := &routedContentParseStub{}
	extractor := NewContentExtractor(
		fileService,
		nil,
		&Config{Enabled: true, MaxContentSize: 1024, ExtractionTimeout: time.Second, CacheEnabled: true},
		WithContentParseService(parser),
	)

	content, err := extractor.ExtractFileContent(context.Background(), "file-2", ContentExtractionScope{
		OrganizationID: "11111111-1111-1111-1111-111111111111",
	})
	if err != nil {
		t.Fatalf("ExtractFileContent() error = %v", err)
	}
	if content.Content != cached || !content.FromCache {
		t.Fatalf("content = %#v", content)
	}
	if parser.calls != 0 {
		t.Fatalf("routed parser calls = %d, want 0", parser.calls)
	}
}
