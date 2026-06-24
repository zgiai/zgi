package service

import (
	"context"
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/zgiai/zgi/api/internal/dto"
	aichatmodel "github.com/zgiai/zgi/api/internal/modules/aichat/model"
	"github.com/zgiai/zgi/api/internal/modules/llm/multimodal"
	interfaces "github.com/zgiai/zgi/api/internal/modules/shared/interface"
)

type fakeAttachmentFileService struct {
	fileURL string
	content []byte
}

func (f *fakeAttachmentFileService) GetUploadConfig() *interfaces.FileUploadConfigResponse {
	return &interfaces.FileUploadConfigResponse{WorkflowFileUploadLimit: 10}
}

func (f *fakeAttachmentFileService) GetFileByID(context.Context, string) (*dto.UploadFile, error) {
	return &dto.UploadFile{ID: "file-1", Name: "cat.png", Extension: ".png", MimeType: "image/png"}, nil
}

func (f *fakeAttachmentFileService) GetFileURL(context.Context, string) (string, error) {
	return f.fileURL, nil
}

func (f *fakeAttachmentFileService) DownloadFile(context.Context, string) ([]byte, error) {
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

func TestExtractPreparedAttachments_PublicImageURLKeepsURL(t *testing.T) {
	publicURL := "https://cdn.example.com/cat.png"
	svc := &service{fileService: &fakeAttachmentFileService{fileURL: publicURL}}
	prepared := preparedChatWithImageAttachment()

	if err := svc.extractPreparedAttachments(context.Background(), prepared, nil); err != nil {
		t.Fatalf("extractPreparedAttachments() error = %v", err)
	}
	if got := prepared.parts.Attachments.Files[0].ImageURL; got != publicURL {
		t.Fatalf("image url = %q, want %q", got, publicURL)
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
