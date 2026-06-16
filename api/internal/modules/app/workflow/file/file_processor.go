package file

import (
	"context"
	"fmt"

	"github.com/zgiai/zgi/api/internal/dto"
	"github.com/zgiai/zgi/api/internal/modules/app/workflow/filediag"
	"github.com/zgiai/zgi/api/internal/modules/shared/interface"
	"github.com/zgiai/zgi/api/pkg/logger"
	"go.uber.org/zap"
)

type FileProcessor struct {
	fileService interfaces.FileService
}

func NewFileProcessor(fileService interfaces.FileService) *FileProcessor {
	return &FileProcessor{
		fileService: fileService,
	}
}

func (fp *FileProcessor) ProcessFileForWorkflow(ctx context.Context, fileID string, tenantID string) (*File, error) {
	uploadFile, err := fp.fileService.GetFileByID(ctx, fileID)
	if err != nil {
		logger.ErrorContext(logger.WithFields(ctx,
			zap.String("event", "workflow_file_processor_lookup_failed"),
			zap.String("upload_file_id", fileID),
			zap.String("tenant_id", tenantID),
		), "workflow file processor failed to load upload file", err)
		filediag.AppendError(ctx, "workflow_file_processor_lookup_failed", "workflow file processor failed to load upload file", map[string]string{
			"upload_file_id": fileID,
			"tenant_id":      tenantID,
			"error":          err.Error(),
		})
		return nil, fmt.Errorf("failed to get file: %w", err)
	}
	if uploadFile == nil {
		logger.WarnContext(logger.WithFields(ctx,
			zap.String("event", "workflow_file_processor_lookup_missing"),
			zap.String("upload_file_id", fileID),
			zap.String("tenant_id", tenantID),
		), "workflow file processor upload file is nil")
		filediag.AppendError(ctx, "workflow_file_processor_lookup_missing", "workflow file processor upload file is nil", map[string]string{
			"upload_file_id": fileID,
			"tenant_id":      tenantID,
		})
		return nil, fmt.Errorf("file %s not found", fileID)
	}

	fileType := fp.determineFileType(uploadFile.Extension)

	workflowFile := NewFile(
		tenantID,
		fileType,
		FileTransferMethodLocalFile,
		WithID(uploadFile.ID),
		WithRelatedID(uploadFile.ID),
		WithFilename(uploadFile.Name),
		WithExtension("."+uploadFile.Extension),
		WithMimeType(uploadFile.MimeType),
		WithSize(int(uploadFile.Size)),
	)

	logger.InfoContext(logger.WithFields(ctx,
		zap.String("event", "workflow_file_processor_file_created"),
		zap.String("upload_file_id", uploadFile.ID),
		zap.String("related_id", stringPtrValue(workflowFile.RelatedID)),
		zap.String("tenant_id", tenantID),
		zap.String("file_type", string(fileType)),
		zap.String("filename", uploadFile.Name),
		zap.String("extension", uploadFile.Extension),
		zap.String("mime_type", uploadFile.MimeType),
		zap.Bool("has_related_id", workflowFile.RelatedID != nil && *workflowFile.RelatedID != ""),
	), "workflow file processor created file object")

	return workflowFile, nil
}

func (fp *FileProcessor) ProcessMultipleFiles(ctx context.Context, fileIDs []string, tenantID string) ([]*File, error) {
	var files []*File

	for _, fileID := range fileIDs {
		file, err := fp.ProcessFileForWorkflow(ctx, fileID, tenantID)
		if err != nil {
			return nil, fmt.Errorf("failed to process file %s: %w", fileID, err)
		}
		files = append(files, file)
	}

	return files, nil
}

func (fp *FileProcessor) determineFileType(extension string) FileType {
	return InferFileType(extension, "")
}

func (fp *FileProcessor) GetFileContent(ctx context.Context, fileID string) ([]byte, error) {
	return fp.fileService.DownloadFile(ctx, fileID)
}

func (fp *FileProcessor) GetFilePreview(ctx context.Context, fileID string) (string, error) {
	return fp.fileService.GetFilePreview(ctx, fileID)
}

func (fp *FileProcessor) ValidateFileForWorkflow(uploadFile *dto.UploadFile) error {
	if uploadFile == nil {
		return fmt.Errorf("file is nil")
	}

	if uploadFile.ID == "" {
		return fmt.Errorf("file ID is empty")
	}

	if uploadFile.Name == "" {
		return fmt.Errorf("file name is empty")
	}

	fileType := fp.determineFileType(uploadFile.Extension)
	if !fileType.IsValid() {
		return fmt.Errorf("unsupported file type: %s", uploadFile.Extension)
	}

	return nil
}
