package file

import (
	"context"
	"fmt"

	"github.com/zgiai/zgi/api/internal/dto"
	"github.com/zgiai/zgi/api/internal/modules/shared/interface"
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
		return nil, fmt.Errorf("failed to get file: %w", err)
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
