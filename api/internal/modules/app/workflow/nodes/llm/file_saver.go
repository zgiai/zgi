package llm

import (
	"context"
	"fmt"
	"mime"
	"net/url"
	"path"
	"strings"
	"time"

	"github.com/zgiai/ginext/internal/modules/app/workflow/file"
	"github.com/zgiai/ginext/internal/modules/app/workflow/tool_file"
)

// FileSaver is responsible for saving multimodal output returned by LLM
type FileSaver interface {
	// SaveBinaryString saves the inline file data returned by LLM
	// Currently (2024-04-30), only some of Google Gemini models will return
	// multimodal output as inline data.
	//
	// Parameters:
	// - data: the contents of the file
	// - mimeType: the media type of the file, specified by rfc6838
	// - fileType: The file type of the inline file
	// - extensionOverride: Override the auto-detected file extension while saving this file
	//
	// The default value is nil, which means do not override the file extension and guessing it
	// from the mimeType attribute while saving the file.
	//
	// Setting it to values other than nil means override the file's extension, and
	// will bypass the extension guessing saving the file.
	//
	// Specially, setting it to empty string ("") will leave the file extension empty.
	//
	// When it is not nil or empty string (""), it should be a string beginning with a
	// dot (`.`). For example, `.py` and `.tar.gz` are both valid values, while `py`
	// and `tar.gz` are not.
	SaveBinaryString(data []byte, mimeType string, fileType file.FileType, extensionOverride *string) (*file.File, error)

	// SaveRemoteURL saves the file from a remote url returned by LLM
	// Currently (2024-04-30), no model returns multimodel output as a url.
	//
	// Parameters:
	// - url: the url of the file
	// - fileType: the file type of the file, check FileType enum for reference
	SaveRemoteURL(url string, fileType file.FileType) (*file.File, error)
}

// FileSaverImpl implements FileSaver interface
type FileSaverImpl struct {
	userID          string
	tenantID        string
	toolFileManager *tool_file.ToolFileManager
	fileSignature   *tool_file.FileSignature
	conversationID  *string
	lifecycle       tool_file.ToolFileLifecycle
	expiresAt       *time.Time
	urlMode         tool_file.ToolFileURLMode
}

// NewFileSaverImpl creates a new FileSaverImpl instance
func NewFileSaverImpl(userID, tenantID string, toolFileManager *tool_file.ToolFileManager, fileSignature *tool_file.FileSignature) *FileSaverImpl {
	return &FileSaverImpl{
		userID:          userID,
		tenantID:        tenantID,
		toolFileManager: toolFileManager,
		fileSignature:   fileSignature,
		urlMode:         tool_file.ToolFileURLModeSigned,
	}
}

func NewFileSaverImplWithLifecycle(
	userID, tenantID string,
	toolFileManager *tool_file.ToolFileManager,
	fileSignature *tool_file.FileSignature,
	lifecycle tool_file.ToolFileLifecycle,
	expiresAt *time.Time,
) *FileSaverImpl {
	return &FileSaverImpl{
		userID:          userID,
		tenantID:        tenantID,
		toolFileManager: toolFileManager,
		fileSignature:   fileSignature,
		lifecycle:       lifecycle,
		expiresAt:       expiresAt,
		urlMode:         tool_file.ToolFileURLModeSigned,
	}
}

func NewFileSaverImplWithLifecycleAndURLMode(
	userID, tenantID string,
	toolFileManager *tool_file.ToolFileManager,
	fileSignature *tool_file.FileSignature,
	lifecycle tool_file.ToolFileLifecycle,
	expiresAt *time.Time,
	urlMode tool_file.ToolFileURLMode,
) *FileSaverImpl {
	return &FileSaverImpl{
		userID:          userID,
		tenantID:        tenantID,
		toolFileManager: toolFileManager,
		fileSignature:   fileSignature,
		lifecycle:       lifecycle,
		expiresAt:       expiresAt,
		urlMode:         urlMode,
	}
}

// NewFileSaverImplWithConversation creates a new FileSaverImpl instance with conversation ID
func NewFileSaverImplWithConversation(userID, tenantID string, conversationID *string, toolFileManager *tool_file.ToolFileManager, fileSignature *tool_file.FileSignature) *FileSaverImpl {
	return &FileSaverImpl{
		userID:          userID,
		tenantID:        tenantID,
		conversationID:  conversationID,
		toolFileManager: toolFileManager,
		fileSignature:   fileSignature,
		urlMode:         tool_file.ToolFileURLModeSigned,
	}
}

func NewFileSaverImplWithConversationAndLifecycle(
	userID, tenantID string,
	conversationID *string,
	toolFileManager *tool_file.ToolFileManager,
	fileSignature *tool_file.FileSignature,
	lifecycle tool_file.ToolFileLifecycle,
	expiresAt *time.Time,
) *FileSaverImpl {
	return &FileSaverImpl{
		userID:          userID,
		tenantID:        tenantID,
		conversationID:  conversationID,
		toolFileManager: toolFileManager,
		fileSignature:   fileSignature,
		lifecycle:       lifecycle,
		expiresAt:       expiresAt,
		urlMode:         tool_file.ToolFileURLModeSigned,
	}
}

// NewFileSaverImplGlobal creates a new FileSaverImpl instance using global managers
func NewFileSaverImplGlobal(userID, tenantID string) *FileSaverImpl {
	return &FileSaverImpl{
		userID:          userID,
		tenantID:        tenantID,
		toolFileManager: tool_file.GlobalToolFileManager,
		fileSignature:   tool_file.GlobalFileSignature,
		urlMode:         tool_file.ToolFileURLModeSigned,
	}
}

func NewFileSaverImplGlobalWithLifecycle(
	userID, tenantID string,
	lifecycle tool_file.ToolFileLifecycle,
	expiresAt *time.Time,
) *FileSaverImpl {
	return &FileSaverImpl{
		userID:          userID,
		tenantID:        tenantID,
		toolFileManager: tool_file.GlobalToolFileManager,
		fileSignature:   tool_file.GlobalFileSignature,
		lifecycle:       lifecycle,
		expiresAt:       expiresAt,
		urlMode:         tool_file.ToolFileURLModeSigned,
	}
}

func NewFileSaverImplGlobalWithLifecycleAndURLMode(
	userID, tenantID string,
	lifecycle tool_file.ToolFileLifecycle,
	expiresAt *time.Time,
	urlMode tool_file.ToolFileURLMode,
) *FileSaverImpl {
	return &FileSaverImpl{
		userID:          userID,
		tenantID:        tenantID,
		toolFileManager: tool_file.GlobalToolFileManager,
		fileSignature:   tool_file.GlobalFileSignature,
		lifecycle:       lifecycle,
		expiresAt:       expiresAt,
		urlMode:         urlMode,
	}
}

// SaveRemoteURL saves the file from a remote URL
func (fs *FileSaverImpl) SaveRemoteURL(fileURL string, fileType file.FileType) (*file.File, error) {
	if fs.toolFileManager == nil {
		return nil, fmt.Errorf("tool file manager not initialized")
	}

	// Use ToolFileManager to create file from URL
	ctx := context.Background()
	toolFile, err := fs.toolFileManager.CreateFileByURL(ctx, tool_file.CreateFileByURLParams{
		UserID:         fs.userID,
		TenantID:       fs.tenantID,
		ConversationID: fs.conversationID,
		FileURL:        fileURL,
		Lifecycle:      fs.lifecycle,
		ExpiresAt:      fs.expiresAt,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to create tool file from URL: %w", err)
	}

	// Get extension from tool file
	extension := toolFile.GetFileExtension()
	if extension == "" {
		extension = getExtension(toolFile.MimeType, nil)
	}

	// Generate signed URL
	signedURL, err := fs.generateSignedURL(toolFile.ID, extension)
	if err != nil {
		return nil, fmt.Errorf("failed to generate signed URL: %w", err)
	}

	return &file.File{
		ZgiModelIdentity: file.FILE_MODEL_IDENTITY,
		ID:               &toolFile.ID,
		TenantID:         fs.tenantID,
		Type:             fileType,
		TransferMethod:   file.FileTransferMethodToolFile,
		Filename:         &toolFile.Name,
		Extension:        &extension,
		MimeType:         &toolFile.MimeType,
		Size:             toolFile.Size,
		RelatedID:        &toolFile.ID,
		URL:              &signedURL,
	}, nil
}

// SaveBinaryString saves binary data as a file
func (fs *FileSaverImpl) SaveBinaryString(
	data []byte,
	mimeType string,
	fileType file.FileType,
	extensionOverride *string,
) (*file.File, error) {
	if fs.toolFileManager == nil {
		return nil, fmt.Errorf("tool file manager not initialized")
	}

	// Validate extension override
	extension, err := validateExtensionOverride(extensionOverride)
	if err != nil {
		return nil, err
	}

	// Get final extension
	finalExtension := getExtension(mimeType, extension)

	// Generate filename with extension
	var filename *string
	if finalExtension != "" {
		generatedFilename := fmt.Sprintf("generated_file%s", finalExtension)
		filename = &generatedFilename
	}

	// Use ToolFileManager to create file
	ctx := context.Background()
	toolFile, err := fs.toolFileManager.CreateFileByRaw(ctx, tool_file.CreateFileByRawParams{
		UserID:         fs.userID,
		TenantID:       fs.tenantID,
		ConversationID: fs.conversationID,
		FileData:       data,
		MimeType:       mimeType,
		Filename:       filename,
		Lifecycle:      fs.lifecycle,
		ExpiresAt:      fs.expiresAt,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to create tool file: %w", err)
	}

	// Generate signed URL
	signedURL, err := fs.generateSignedURL(toolFile.ID, finalExtension)
	if err != nil {
		return nil, fmt.Errorf("failed to generate signed URL: %w", err)
	}

	return &file.File{
		ZgiModelIdentity: file.FILE_MODEL_IDENTITY,
		ID:               &toolFile.ID,
		TenantID:         fs.tenantID,
		Type:             fileType,
		TransferMethod:   file.FileTransferMethodToolFile,
		Filename:         &toolFile.Name,
		Extension:        &finalExtension,
		MimeType:         &mimeType,
		Size:             int64(len(data)),
		RelatedID:        &toolFile.ID,
		URL:              &signedURL,
	}, nil
}

// generateSignedURL generates a signed URL for the tool file
func (fs *FileSaverImpl) generateSignedURL(toolFileID, extension string) (string, error) {
	if fs.fileSignature != nil {
		return fs.fileSignature.SignToolFileWithMode(toolFileID, extension, fs.getURLMode())
	}

	// Fallback to global file signature
	return tool_file.SignToolFileGlobalWithMode(toolFileID, extension, fs.getURLMode())
}

func (fs *FileSaverImpl) getURLMode() tool_file.ToolFileURLMode {
	if fs.urlMode == tool_file.ToolFileURLModePermanent {
		return tool_file.ToolFileURLModePermanent
	}
	return tool_file.ToolFileURLModeSigned
}

// getExtension returns the extension of file
// If the extensionOverride parameter is set, this function should honor it and return its value
func getExtension(mimeType string, extensionOverride *string) string {
	if extensionOverride != nil {
		return *extensionOverride
	}

	// Use Go's built-in mime package to guess extension
	extensions, err := mime.ExtensionsByType(mimeType)
	if err == nil && len(extensions) > 0 {
		return extensions[0]
	}

	// Default extension if unable to determine
	return ".dat"
}

// extractContentTypeAndExtension tries to guess content type of file from url and Content-Type header in response
func extractContentTypeAndExtension(fileURL string, contentTypeHeader string) (string, string) {
	if contentTypeHeader != "" {
		// Clean up content type (remove charset etc.)
		mediaType, _, _ := mime.ParseMediaType(contentTypeHeader)
		if mediaType != "" {
			extensions, err := mime.ExtensionsByType(mediaType)
			if err == nil && len(extensions) > 0 {
				return mediaType, extensions[0]
			}
			return mediaType, ".dat"
		}
	}

	// Try to guess from URL
	parsedURL, err := url.Parse(fileURL)
	if err == nil {
		ext := path.Ext(parsedURL.Path)
		if ext != "" {
			mimeType := mime.TypeByExtension(ext)
			if mimeType != "" {
				return mimeType, ext
			}
		}
	}

	// Default fallback
	return "application/octet-stream", ".dat"
}

// validateExtensionOverride validates the extension override parameter
func validateExtensionOverride(extensionOverride *string) (*string, error) {
	// extensionOverride is allowed to be nil or empty string
	if extensionOverride == nil {
		return nil, nil
	}

	if *extensionOverride == "" {
		return extensionOverride, nil
	}

	if !strings.HasPrefix(*extensionOverride, ".") {
		return nil, fmt.Errorf("extension_override should start with '.' if not nil or empty: %s", *extensionOverride)
	}

	return extensionOverride, nil
}
