package service

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/zgiai/zgi/api/internal/dto"
	aichatmodel "github.com/zgiai/zgi/api/internal/modules/aichat/model"
	"github.com/zgiai/zgi/api/internal/modules/llm/multimodal"
	interfaces "github.com/zgiai/zgi/api/internal/modules/shared/interface"
)

type fakeAttachmentFileService struct {
	fileURL     string
	content     []byte
	downloadErr error
	imageLimit  int64
}

func (f *fakeAttachmentFileService) GetUploadConfig() *interfaces.FileUploadConfigResponse {
	return &interfaces.FileUploadConfigResponse{WorkflowFileUploadLimit: 10, ImageFileSizeLimit: f.imageLimit}
}

func (f *fakeAttachmentFileService) GetFileByID(context.Context, string) (*dto.UploadFile, error) {
	return &dto.UploadFile{ID: "file-1", Name: "cat.png", Extension: ".png", MimeType: "image/png"}, nil
}

func (f *fakeAttachmentFileService) GetFileURL(context.Context, string) (string, error) {
	return f.fileURL, nil
}

func (f *fakeAttachmentFileService) DownloadFile(context.Context, string) ([]byte, error) {
	if f.downloadErr != nil {
		return nil, f.downloadErr
	}
	return f.content, nil
}

func TestExtractPreparedAttachments_LocalImageURLUsesDataURL(t *testing.T) {
	svc := &service{fileService: &fakeAttachmentFileService{
		fileURL: "http://localhost:2670/console/api/files/file-1/file-preview?sign=test",
		content: []byte("png-bytes"),
	}}
	prepared := preparedChatWithImageAttachment()

	if err := svc.extractPreparedAttachments(context.Background(), prepared, nil); err != nil {
		t.Fatalf("extractPreparedAttachments() error = %v", err)
	}
	got := prepared.parts.Attachments.Files[0].ImageURL
	if !strings.HasPrefix(got, "data:image/png;base64,") {
		t.Fatalf("image url = %q, want image data URL", got)
	}
}

func TestExtractPreparedAttachments_FileServiceImageURLUsesDataURL(t *testing.T) {
	svc := &service{fileService: &fakeAttachmentFileService{
		fileURL: "https://cdn.example.com/cat.png",
		content: []byte("png-bytes"),
	}}
	prepared := preparedChatWithImageAttachment()

	if err := svc.extractPreparedAttachments(context.Background(), prepared, nil); err != nil {
		t.Fatalf("extractPreparedAttachments() error = %v", err)
	}
	got := prepared.parts.Attachments.Files[0].ImageURL
	if !strings.HasPrefix(got, "data:image/png;base64,") {
		t.Fatalf("image url = %q, want image data URL", got)
	}
}

func TestExtractPreparedAttachments_InternalDNSImageURLUsesDataURL(t *testing.T) {
	svc := &service{fileService: &fakeAttachmentFileService{
		fileURL: "http://files:2679/console/api/files/file-1/file-preview?sign=test",
		content: []byte("png-bytes"),
	}}
	prepared := preparedChatWithImageAttachment()

	if err := svc.extractPreparedAttachments(context.Background(), prepared, nil); err != nil {
		t.Fatalf("extractPreparedAttachments() error = %v", err)
	}
	got := prepared.parts.Attachments.Files[0].ImageURL
	if !strings.HasPrefix(got, "data:image/png;base64,") {
		t.Fatalf("image url = %q, want image data URL", got)
	}
}

func preparedChatWithImageAttachment() *PreparedChat {
	return &PreparedChat{
		Conversation: &aichatmodel.Conversation{ID: uuid.New()},
		Message:      &aichatmodel.Message{ID: uuid.New()},
		parts: &chatRequestParts{
			ModelSupportsVision: true,
			Attachments: &attachmentBundle{Files: []attachmentFile{{
				ID:            "file-1",
				Name:          "cat.png",
				Extension:     ".png",
				MimeType:      "image/png",
				Kind:          attachmentKindImage,
				ContentStatus: attachmentContentStatusPending,
				VisionDetail:  multimodal.ImageDetailHigh,
			}}},
		},
	}
}

func TestExtractPreparedAttachments_LocalImageExceedsLimitFails(t *testing.T) {
	svc := &service{fileService: &fakeAttachmentFileService{
		fileURL:    "http://localhost:2670/console/api/files/file-1/file-preview?sign=test",
		content:    []byte("too-large"),
		imageLimit: 1,
	}}
	prepared := preparedChatWithImageAttachment()
	prepared.parts.Attachments.Files[0].Size = bytesPerMegabyte + 1

	err := svc.extractPreparedAttachments(context.Background(), prepared, nil)
	if err == nil || !strings.Contains(err.Error(), "image file exceeds size limit") {
		t.Fatalf("extractPreparedAttachments() error = %v, want image size limit error", err)
	}
}

func TestHistoricalUserMessage_SkipsUnavailableHistoricalImage(t *testing.T) {
	svc := &service{fileService: &fakeAttachmentFileService{
		fileURL:     "http://localhost:2670/console/api/files/file-1/file-preview?sign=test",
		downloadErr: errors.New("download failed"),
	}}
	msg := &aichatmodel.Message{
		Query: "later question",
		Metadata: map[string]interface{}{
			"files": []interface{}{map[string]interface{}{
				"id":             "file-1",
				"name":           "cat.png",
				"extension":      ".png",
				"mime_type":      "image/png",
				"kind":           attachmentKindImage,
				"content_status": attachmentContentStatusVision,
			}},
		},
	}

	got, err := svc.historicalUserMessage(context.Background(), msg, true)
	if err != nil {
		t.Fatalf("historicalUserMessage() error = %v", err)
	}
	if got == nil || got.Content != "later question" {
		t.Fatalf("historicalUserMessage() = %#v, want text-only historical message", got)
	}
}
