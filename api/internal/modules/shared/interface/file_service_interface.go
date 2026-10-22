package interfaces

import (
	"context"
	"github.com/zgiai/zgi/api/internal/contracts/chatfile"

	"github.com/zgiai/zgi/api/internal/dto"
	"github.com/zgiai/zgi/api/internal/modules/file_process/model"
)

// FileSource file source type
type FileSource string

const (
	FileSourceDatasets FileSource = "datasets"
	FileSourceWorkflow FileSource = "workflow"
	FileSourceChat     FileSource = "chat"
)

// FileUploadConfigResponse file upload configuration response
type FileUploadConfigResponse struct {
	ChatUploadPolicy        chatfile.Effective `json:"chat_upload_policy"`
	FileSizeLimit           int64              `json:"file_size_limit"`            // File size limit (MB)
	BatchCountLimit         int                `json:"batch_count_limit"`          // File-management concurrency; count limit for other upload surfaces
	UploadQueueLimit        int                `json:"upload_queue_limit"`         // File-management upload queue limit
	ImageFileSizeLimit      int64              `json:"image_file_size_limit"`      // Image file size limit (MB)
	VideoFileSizeLimit      int64              `json:"video_file_size_limit"`      // Video file size limit (MB)
	AudioFileSizeLimit      int64              `json:"audio_file_size_limit"`      // Audio file size limit (MB)
	WorkflowFileUploadLimit int                `json:"workflow_file_upload_limit"` // Workflow file upload limit
}

// ChatPolicy resolves application selection against platform limits.
func (c *FileUploadConfigResponse) ChatPolicy(selection *chatfile.Policy) chatfile.Effective {
	return chatfile.ResolveLimits(selection, c.ChatLimits())
}

// ChatLimits are the platform's per-kind chat attachment size caps.
func (c *FileUploadConfigResponse) ChatLimits() chatfile.Limits {
	if c == nil {
		return chatfile.Limits{DocumentMiB: 50, ImageMiB: 10, Count: chatfile.DefaultCount}
	}
	return chatfile.Limits{DocumentMiB: c.FileSizeLimit, ImageMiB: c.ImageFileSizeLimit, AudioMiB: c.AudioFileSizeLimit, VideoMiB: c.VideoFileSizeLimit, Count: chatfile.DefaultCount}
}

// FileService defines the interface for file operations
type FileService interface {
	GetUploadConfig() *FileUploadConfigResponse
	UploadFile(ctx context.Context, filename string, content []byte, mimeType string, userID, tenantID string, userRole model.CreatedByRole, source *FileSource, teamTenantID *string, isTemporary bool, isIcon bool) (*dto.UploadFile, error)
	ReplaceFileContent(ctx context.Context, fileID string, filename string, content []byte, mimeType string, userID, tenantID string) (*dto.UploadFile, error)
	GetFilePreview(ctx context.Context, fileID string) (string, error)
	GetFilePreviewWithOCR(ctx context.Context, fileID string, enableOCR bool) (string, error)
	GetFile(ctx context.Context, fileID string) (string, error)
	ExtractFileWithSetting(ctx context.Context, fileID string, setting FileExtractionSetting) (string, error)
	GetSupportedFileTypes() []string
	IsFileSizeWithinLimit(extension string, fileSize int64) bool
	ParseFileContent(ctx context.Context, uploadFileID string)
	GetFileByID(ctx context.Context, fileID string) (*dto.UploadFile, error)
	IsRuntimeExternalFile(ctx context.Context, fileID string) (bool, error)
	DownloadFile(ctx context.Context, fileID string) ([]byte, error)
	// ListFiles paginated list of files
	ListFiles(ctx context.Context, tenantID, accountID string, req *dto.FileListRequest, visibleWorkspaceIDs []string) (*dto.FileListResponse, error)
	// ListArchivedFiles paginated list of archived files
	ListArchivedFiles(ctx context.Context, tenantID, accountID string, req *dto.FileListRequest, visibleWorkspaceIDs []string) (*dto.FileListResponse, error)
	// GetStorageUsage gets the storage usage for a tenant
	GetStorageUsage(ctx context.Context, tenantID string) (int64, error)
	// DeleteFiles deletes files by their IDs
	DeleteFiles(ctx context.Context, fileIDs []string) error
	// UpdateContentText updates the cached content text for a file
	UpdateContentText(ctx context.Context, fileID string, contentText string) error
	// GetFileURL gets the URL for a file by its ID
	GetFileURL(ctx context.Context, fileID string) (string, error)
}

// FileExtractionSetting controls explicit file extraction calls that should not
// reuse cached upload text.
type FileExtractionSetting struct {
	ExtractionStrategy        string
	ExtractionFallbackEnabled *bool
	EnableOCR                 *bool
	CacheNamespace            string
}
