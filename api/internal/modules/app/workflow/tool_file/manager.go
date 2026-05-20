package tool_file

import (
	"context"
	"errors"
	"fmt"
	"io"
	"mime"
	"net/http"
	"path/filepath"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/zgiai/ginext/internal/observability"
	"github.com/zgiai/ginext/pkg/storage"
	"gorm.io/gorm"
)

const DefaultTemporaryToolFileTTL = 24 * time.Hour

// ToolFileManager manages tool files in the system
type ToolFileManager struct {
	db      *gorm.DB
	storage storage.Storage
}

// NewToolFileManager creates a new tool file manager
func NewToolFileManager(db *gorm.DB, storage storage.Storage) *ToolFileManager {
	return &ToolFileManager{
		db:      db,
		storage: storage,
	}
}

// CreateFileByRaw creates a tool file from raw binary data
func (tm *ToolFileManager) CreateFileByRaw(ctx context.Context, params CreateFileByRawParams) (*ToolFile, error) {
	lifecycle, expiresAt, err := resolveLifecycle(params.Lifecycle, params.ExpiresAt)
	if err != nil {
		return nil, err
	}

	// Determine file extension
	extension := getExtensionFromMimeType(params.MimeType)
	if extension == "" {
		extension = ".bin"
	}

	// Generate unique filename
	uniqueName := uuid.New().String()
	uniqueFilename := uniqueName + extension

	// Determine present filename
	presentFilename := uniqueFilename
	if params.Filename != nil && *params.Filename != "" {
		hasExtension := strings.Contains(*params.Filename, ".")
		if hasExtension {
			presentFilename = *params.Filename
		} else {
			presentFilename = *params.Filename + extension
		}
	}

	// Create storage file path
	filepath := fmt.Sprintf("tools/%s/%s", params.TenantID, uniqueFilename)

	// Save file to storage
	if err := tm.storage.Save(filepath, params.FileData); err != nil {
		return nil, fmt.Errorf("failed to save file to storage: %w", err)
	}

	// Create tool file record
	toolFile := &ToolFile{
		UserID:         params.UserID,
		TenantID:       params.TenantID,
		ConversationID: params.ConversationID,
		FileKey:        filepath,
		MimeType:       params.MimeType,
		Name:           presentFilename,
		Size:           int64(len(params.FileData)),
		Lifecycle:      string(lifecycle),
		ExpiresAt:      expiresAt,
	}

	// Save to database
	if err := tm.db.WithContext(ctx).Create(toolFile).Error; err != nil {
		// Try to clean up the file from storage if database save fails
		_ = tm.storage.Delete(filepath)
		return nil, fmt.Errorf("failed to save tool file to database: %w", err)
	}

	return toolFile, nil
}

// CreateFileByURL creates a tool file from a remote URL
func (tm *ToolFileManager) CreateFileByURL(ctx context.Context, params CreateFileByURLParams) (*ToolFile, error) {
	lifecycle, expiresAt, err := resolveLifecycle(params.Lifecycle, params.ExpiresAt)
	if err != nil {
		return nil, err
	}

	// Download file from URL
	client := observability.HTTPClient(&http.Client{
		Timeout: 30 * time.Second,
	})

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, params.FileURL, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create download request for %s: %w", params.FileURL, err)
	}

	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to download file from %s: %w", params.FileURL, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("failed to download file from %s: status %d", params.FileURL, resp.StatusCode)
	}

	// Read file data
	fileData, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read file data from %s: %w", params.FileURL, err)
	}

	// Determine MIME type
	mimeType := guessMimeTypeFromURL(params.FileURL)
	if contentType := resp.Header.Get("Content-Type"); contentType != "" {
		if ct, _, err := mime.ParseMediaType(contentType); err == nil {
			mimeType = ct
		}
	}
	if detected := detectMimeTypeFromData(fileData); shouldPreferDetectedMimeType(mimeType, detected) {
		mimeType = detected
	}
	if mimeType == "" {
		mimeType = "application/octet-stream"
	}

	// Determine extension and filename
	extension := getExtensionFromMimeType(mimeType)
	if extension == "" {
		extension = ".bin"
	}

	uniqueName := uuid.New().String()
	filename := uniqueName + extension

	// Create storage file path
	filepath := fmt.Sprintf("tools/%s/%s", params.TenantID, filename)

	// Save file to storage
	if err := tm.storage.Save(filepath, fileData); err != nil {
		return nil, fmt.Errorf("failed to save file to storage: %w", err)
	}

	// Create tool file record
	toolFile := &ToolFile{
		UserID:         params.UserID,
		TenantID:       params.TenantID,
		ConversationID: params.ConversationID,
		FileKey:        filepath,
		MimeType:       mimeType,
		OriginalURL:    &params.FileURL,
		Name:           filename,
		Size:           int64(len(fileData)),
		Lifecycle:      string(lifecycle),
		ExpiresAt:      expiresAt,
	}

	// Save to database
	if err := tm.db.WithContext(ctx).Create(toolFile).Error; err != nil {
		// Try to clean up the file from storage if database save fails
		_ = tm.storage.Delete(filepath)
		return nil, fmt.Errorf("failed to save tool file to database: %w", err)
	}

	return toolFile, nil
}

// GetFileBinary retrieves file binary data by tool file ID
func (tm *ToolFileManager) GetFileBinary(ctx context.Context, toolFileID string) ([]byte, string, error) {
	// Find tool file in database
	var toolFile ToolFile
	if err := tm.db.WithContext(ctx).Where("id = ?", toolFileID).First(&toolFile).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			return nil, "", fmt.Errorf("tool file not found")
		}
		return nil, "", fmt.Errorf("failed to find tool file: %w", err)
	}

	// Load file from storage
	fileData, err := tm.storage.Load(toolFile.FileKey)
	if err != nil {
		return nil, "", fmt.Errorf("failed to load file from storage: %w", err)
	}

	return fileData, toolFile.MimeType, nil
}

// GetFileStream retrieves file stream by tool file ID
func (tm *ToolFileManager) GetFileStream(ctx context.Context, toolFileID string) (<-chan []byte, *ToolFile, error) {
	// Find tool file in database
	var toolFile ToolFile
	if err := tm.db.WithContext(ctx).Where("id = ?", toolFileID).First(&toolFile).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			return nil, nil, fmt.Errorf("tool file not found")
		}
		return nil, nil, fmt.Errorf("failed to find tool file: %w", err)
	}

	// Load file stream from storage
	stream, err := tm.storage.LoadStream(toolFile.FileKey)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to load file stream from storage: %w", err)
	}

	return stream, &toolFile, nil
}

// GetToolFileByID retrieves a tool file by ID
func (tm *ToolFileManager) GetToolFileByID(ctx context.Context, toolFileID string) (*ToolFile, error) {
	var toolFile ToolFile
	if err := tm.db.WithContext(ctx).Where("id = ?", toolFileID).First(&toolFile).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			return nil, fmt.Errorf("tool file not found")
		}
		return nil, fmt.Errorf("failed to find tool file: %w", err)
	}
	return &toolFile, nil
}

// DeleteToolFile deletes a tool file
func (tm *ToolFileManager) DeleteToolFile(ctx context.Context, toolFileID string) error {
	// Find tool file first
	var toolFile ToolFile
	if err := tm.db.WithContext(ctx).Where("id = ?", toolFileID).First(&toolFile).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			return fmt.Errorf("tool file not found")
		}
		return fmt.Errorf("failed to find tool file: %w", err)
	}

	// Delete from storage
	if err := tm.storage.Delete(toolFile.FileKey); err != nil {
		// Continue with database deletion even if storage deletion fails
	}

	// Delete from database
	if err := tm.db.WithContext(ctx).Delete(&toolFile).Error; err != nil {
		return fmt.Errorf("failed to delete tool file from database: %w", err)
	}

	return nil
}

func (tm *ToolFileManager) CleanupExpiredTemporaryFiles(ctx context.Context) (int, error) {
	var toolFiles []ToolFile
	if err := tm.db.WithContext(ctx).
		Where("lifecycle = ? AND expires_at IS NOT NULL AND expires_at <= ?", string(ToolFileLifecycleTemporary), time.Now()).
		Find(&toolFiles).Error; err != nil {
		return 0, fmt.Errorf("failed to query expired temporary tool files: %w", err)
	}

	deletedCount := 0
	var deleteErrors []error

	for _, toolFile := range toolFiles {
		if err := tm.storage.Delete(toolFile.FileKey); err != nil {
			deleteErrors = append(deleteErrors, fmt.Errorf("failed to delete storage object for tool file %s: %w", toolFile.ID, err))
			continue
		}

		if err := tm.db.WithContext(ctx).Delete(&toolFile).Error; err != nil {
			deleteErrors = append(deleteErrors, fmt.Errorf("failed to delete tool file %s from database: %w", toolFile.ID, err))
			continue
		}

		deletedCount++
	}

	if len(deleteErrors) > 0 {
		return deletedCount, errors.Join(deleteErrors...)
	}

	return deletedCount, nil
}

// CreateFileByRawParams parameters for creating file by raw data
type CreateFileByRawParams struct {
	UserID         string
	TenantID       string
	ConversationID *string
	FileData       []byte
	MimeType       string
	Filename       *string
	Lifecycle      ToolFileLifecycle
	ExpiresAt      *time.Time
}

// CreateFileByURLParams parameters for creating file by URL
type CreateFileByURLParams struct {
	UserID         string
	TenantID       string
	ConversationID *string
	FileURL        string
	Lifecycle      ToolFileLifecycle
	ExpiresAt      *time.Time
}

// Helper functions

// getExtensionFromMimeType gets file extension from MIME type
func getExtensionFromMimeType(mimeType string) string {
	extensions, err := mime.ExtensionsByType(mimeType)
	if err != nil || len(extensions) == 0 {
		return ""
	}
	return extensions[0]
}

// guessMimeTypeFromURL guesses MIME type from URL
func guessMimeTypeFromURL(url string) string {
	ext := filepath.Ext(url)
	if ext == "" {
		return ""
	}
	return mime.TypeByExtension(ext)
}

func detectMimeTypeFromData(fileData []byte) string {
	if len(fileData) == 0 {
		return ""
	}
	return http.DetectContentType(fileData)
}

func shouldPreferDetectedMimeType(currentMimeType, detectedMimeType string) bool {
	currentMimeType = strings.TrimSpace(strings.ToLower(currentMimeType))
	detectedMimeType = strings.TrimSpace(strings.ToLower(detectedMimeType))

	if detectedMimeType == "" || detectedMimeType == "application/octet-stream" {
		return false
	}
	return currentMimeType == "" || currentMimeType == "application/octet-stream"
}

func resolveLifecycle(lifecycle ToolFileLifecycle, expiresAt *time.Time) (ToolFileLifecycle, *time.Time, error) {
	switch lifecycle {
	case "":
		return ToolFileLifecyclePersistent, nil, nil
	case ToolFileLifecyclePersistent:
		return ToolFileLifecyclePersistent, nil, nil
	case ToolFileLifecycleTemporary:
		if expiresAt != nil {
			expiresCopy := *expiresAt
			return ToolFileLifecycleTemporary, &expiresCopy, nil
		}

		defaultExpiresAt := time.Now().Add(DefaultTemporaryToolFileTTL)
		return ToolFileLifecycleTemporary, &defaultExpiresAt, nil
	default:
		return "", nil, fmt.Errorf("unsupported tool file lifecycle: %s", lifecycle)
	}
}

// Global tool file manager instance
var GlobalToolFileManager *ToolFileManager

// InitToolFileManager initializes the global tool file manager
func InitToolFileManager(db *gorm.DB, storage storage.Storage) {
	GlobalToolFileManager = NewToolFileManager(db, storage)
}

// Helper functions for global access
func CreateFileByRawGlobal(ctx context.Context, params CreateFileByRawParams) (*ToolFile, error) {
	if GlobalToolFileManager == nil {
		return nil, fmt.Errorf("tool file manager not initialized")
	}
	return GlobalToolFileManager.CreateFileByRaw(ctx, params)
}

func CreateFileByURLGlobal(ctx context.Context, params CreateFileByURLParams) (*ToolFile, error) {
	if GlobalToolFileManager == nil {
		return nil, fmt.Errorf("tool file manager not initialized")
	}
	return GlobalToolFileManager.CreateFileByURL(ctx, params)
}

func GetFileBinaryGlobal(ctx context.Context, toolFileID string) ([]byte, string, error) {
	if GlobalToolFileManager == nil {
		return nil, "", fmt.Errorf("tool file manager not initialized")
	}
	return GlobalToolFileManager.GetFileBinary(ctx, toolFileID)
}

func CleanupExpiredTemporaryFilesGlobal(ctx context.Context) (int, error) {
	if GlobalToolFileManager == nil {
		return 0, fmt.Errorf("tool file manager not initialized")
	}
	return GlobalToolFileManager.CleanupExpiredTemporaryFiles(ctx)
}
