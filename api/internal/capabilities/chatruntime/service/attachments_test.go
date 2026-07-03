package service

import (
	"context"
	"errors"
	"strings"
	"testing"

	runtimemodel "github.com/zgiai/zgi/api/internal/capabilities/chatruntime/model"
	"github.com/zgiai/zgi/api/internal/dto"
	interfaces "github.com/zgiai/zgi/api/internal/modules/shared/interface"
)

func TestFormatAttachmentSectionsIncludesFileID(t *testing.T) {
	sections := formatAttachmentSections([]attachmentFile{{
		ID:        "2d9cdfaa-5ecb-4f89-bc21-d2c5704844a7",
		Name:      "power-confirmation.xlsx",
		Extension: "xlsx",
		Content:   "index;meter;current",
	}}, func(file attachmentFile) string {
		return file.Content
	})

	if !strings.Contains(sections, "File: power-confirmation.xlsx\n") {
		t.Fatalf("formatted sections = %q, want display name without duplicate extension", sections)
	}
	if strings.Contains(sections, ".xlsx .xlsx") {
		t.Fatalf("formatted sections = %q, want no duplicate extension", sections)
	}
	if !strings.Contains(sections, "File ID: 2d9cdfaa-5ecb-4f89-bc21-d2c5704844a7\n") {
		t.Fatalf("formatted sections = %q, want file ID", sections)
	}
}

type fakeRuntimeAttachmentFileService struct {
	fileURL     string
	content     []byte
	downloadErr error
	imageLimit  int64
}

func (f *fakeRuntimeAttachmentFileService) GetUploadConfig() *interfaces.FileUploadConfigResponse {
	return &interfaces.FileUploadConfigResponse{WorkflowFileUploadLimit: 10, ImageFileSizeLimit: f.imageLimit}
}

func (f *fakeRuntimeAttachmentFileService) GetFileByID(context.Context, string) (*dto.UploadFile, error) {
	return &dto.UploadFile{ID: "file-1", Name: "cat.png", Extension: ".png", MimeType: "image/png"}, nil
}

func (f *fakeRuntimeAttachmentFileService) GetFileURL(context.Context, string) (string, error) {
	return f.fileURL, nil
}

func (f *fakeRuntimeAttachmentFileService) DownloadFile(context.Context, string) ([]byte, error) {
	if f.downloadErr != nil {
		return nil, f.downloadErr
	}
	return f.content, nil
}

func TestRuntimePrepareVisionImageURL_LocalImageExceedsLimitFails(t *testing.T) {
	svc := &service{fileService: &fakeRuntimeAttachmentFileService{
		fileURL:    "http://localhost:2670/console/api/files/file-1/file-preview?sign=test",
		content:    []byte("too-large"),
		imageLimit: 1,
	}}

	_, err := svc.prepareVisionImageURL(context.Background(), &attachmentFile{
		ID:        "file-1",
		Name:      "cat.png",
		Size:      bytesPerMegabyte + 1,
		Extension: ".png",
		MimeType:  "image/png",
		Kind:      attachmentKindImage,
	})
	if err == nil || !strings.Contains(err.Error(), "image file exceeds size limit") {
		t.Fatalf("prepareVisionImageURL() error = %v, want image size limit error", err)
	}
}

func TestRuntimePrepareVisionImageURL_InternalDNSImageURLUsesDataURL(t *testing.T) {
	svc := &service{fileService: &fakeRuntimeAttachmentFileService{
		fileURL: "http://files:2679/console/api/files/file-1/file-preview?sign=test",
		content: []byte("png-bytes"),
	}}

	got, err := svc.prepareVisionImageURL(context.Background(), &attachmentFile{
		ID:        "file-1",
		Name:      "cat.png",
		Extension: ".png",
		MimeType:  "image/png",
		Kind:      attachmentKindImage,
	})
	if err != nil {
		t.Fatalf("prepareVisionImageURL() error = %v", err)
	}
	if !strings.HasPrefix(got, "data:image/png;base64,") {
		t.Fatalf("image url = %q, want image data URL", got)
	}
}

func TestRuntimeHistoricalUserMessage_ImagePrepareFailureReturnsError(t *testing.T) {
	svc := &service{fileService: &fakeRuntimeAttachmentFileService{
		fileURL:     "http://localhost:2670/console/api/files/file-1/file-preview?sign=test",
		downloadErr: errors.New("download failed"),
	}}
	msg := &runtimemodel.Message{Metadata: map[string]interface{}{
		"files": []interface{}{map[string]interface{}{
			"id":             "file-1",
			"name":           "cat.png",
			"extension":      ".png",
			"mime_type":      "image/png",
			"kind":           attachmentKindImage,
			"content_status": attachmentContentStatusVision,
		}},
	}}

	_, err := svc.historicalUserMessage(context.Background(), msg, true)
	if err == nil || !strings.Contains(err.Error(), "failed to prepare historical image input") {
		t.Fatalf("historicalUserMessage() error = %v, want historical image error", err)
	}
}
