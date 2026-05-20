package file

import (
	"fmt"
	"path/filepath"
	"strings"
	"time"

	"github.com/zgiai/ginext/config"
)

// WorkflowFileConfig holds configuration for workflow file handling and validation.
//
// This configuration controls file upload limits and restrictions for workflow execution.
// It is used to validate files before they are processed by workflows.
//
// Fields:
//   - MaxFileSize: Maximum allowed file size in bytes (default: 15MB)
//   - MaxFilesPerRun: Maximum number of files per workflow run (default: 10)
//   - AllowedExtensions: List of permitted file extensions without dots (e.g., "pdf", "docx")
//   - AllowedMimeTypes: List of permitted MIME types (e.g., "application/pdf")
//
// Configuration Source:
//   - Loaded from config.yaml under upload section
//   - Falls back to defaults if config is not available
//
// Example Configuration (config.yaml):
//
//	upload:
//	  file_size_limit: 15  # MB
//	  workflow_file_limit: 10
type WorkflowFileConfig struct {
	MaxFileSize       int64    `json:"max_file_size"`      // Maximum file size in bytes
	MaxFilesPerRun    int      `json:"max_files_per_run"`  // Maximum number of files per workflow run
	AllowedExtensions []string `json:"allowed_extensions"` // Allowed file extensions (without dots)
	AllowedMimeTypes  []string `json:"allowed_mime_types"` // Allowed MIME types
}

// Config holds configuration for ContentExtractor behavior.
//
// This configuration controls how file content extraction operates during workflow
// execution, including feature enablement, size limits, timeouts, and caching.
//
// Fields:
//   - Enabled: Whether file content extraction is enabled (default: true)
//   - MaxContentSize: Maximum size of extracted content in bytes (default: 1MB)
//   - ExtractionTimeout: Maximum time to wait for extraction (default: 120 seconds)
//   - CacheEnabled: Whether to cache extracted content in database (default: true)
//   - Strategy: Extraction strategy (mineru|local|reducto|unstructured|landingai|"")
//     Empty string inherits from ETL_HYPERPARSE_BACKEND when Hyperparse is enabled,
//     or falls back to the built-in default extractor.
//
// Configuration Source:
//   - Loaded from config.yaml under workflow_file_extraction section
//   - Falls back to defaults if config is not available
//
// Example Configuration (config.yaml):
//
//	workflow_file_extraction:
//	  enabled: true
//	  max_content_size: 1048576      # 1MB in bytes
//	  extraction_timeout: 120        # seconds
//	  cache_enabled: true
//	  strategy: ""                   # inherit from ETL_HYPERPARSE_BACKEND
//
// Behavior:
//   - When Enabled is false, extraction is skipped and empty content is returned
//   - Content exceeding MaxContentSize is truncated with a notice
//   - Extraction exceeding ExtractionTimeout is cancelled and logged
//   - When CacheEnabled is true, content is stored in upload_files.content_text
//   - Extraction failures automatically fall back through the configured strategy chain
type Config struct {
	Enabled           bool          // Whether content extraction is enabled
	MaxContentSize    int           // Maximum extracted content size in bytes
	ExtractionTimeout time.Duration // Maximum time to wait for extraction
	CacheEnabled      bool          // Whether to cache extracted content
	Strategy          string        // Extraction strategy: mineru|local|reducto|unstructured|landingai|""
}

// GetContentExtractorConfig returns the content extractor configuration from global config.
//
// This function loads configuration from the global config instance and returns a Config
// struct with all settings. If global config is not available, it returns default values.
//
// Returns:
//   - *Config: Configuration for ContentExtractor with all settings populated
//
// Default Values (when config is not available):
//   - Enabled: true
//   - MaxContentSize: 1048576 (1MB)
//   - ExtractionTimeout: 120 seconds
//   - CacheEnabled: true
//   - Strategy: "" (inherits from ETL config at runtime)
//
// Usage:
//
//	config := GetContentExtractorConfig()
//	extractor := NewContentExtractor(fileService, extractProcessor, config)
func GetContentExtractorConfig() *Config {
	cfg := config.GlobalConfig
	if cfg == nil {
		return getDefaultContentExtractorConfig()
	}

	return &Config{
		Enabled:           cfg.WorkflowFileExtraction.Enabled,
		MaxContentSize:    cfg.WorkflowFileExtraction.MaxContentSize,
		ExtractionTimeout: time.Duration(cfg.WorkflowFileExtraction.ExtractionTimeout) * time.Second,
		CacheEnabled:      cfg.WorkflowFileExtraction.CacheEnabled,
		Strategy:          cfg.WorkflowFileExtraction.Strategy,
	}
}

func getDefaultContentExtractorConfig() *Config {
	return &Config{
		Enabled:           true,
		MaxContentSize:    1048576, // 1MB
		ExtractionTimeout: 120 * time.Second,
		CacheEnabled:      true,
		Strategy:          "",
	}
}

// GetWorkflowFileConfig returns the workflow file configuration from global config.
//
// This function loads file handling configuration including size limits, file count
// limits, and allowed file types. Used for validating files before workflow execution.
//
// Returns:
//   - *WorkflowFileConfig: Configuration for file validation and limits
//
// Default Values (when config is not available):
//   - MaxFileSize: 15MB
//   - MaxFilesPerRun: 10
//   - AllowedExtensions: Common document and image formats
//   - AllowedMimeTypes: Corresponding MIME types
//
// Usage:
//
//	config := GetWorkflowFileConfig()
//	err := ValidateFileForWorkflow(filename, size, mimeType)
func GetWorkflowFileConfig() *WorkflowFileConfig {
	cfg := config.GlobalConfig
	if cfg == nil {
		return getDefaultWorkflowFileConfig()
	}

	return &WorkflowFileConfig{
		MaxFileSize:    int64(cfg.Upload.FileSizeLimit) * 1024 * 1024, // Convert MB to bytes
		MaxFilesPerRun: cfg.Upload.WorkflowFileLimit,
		AllowedExtensions: []string{
			// Document types
			"txt", "md", "mdx", "markdown", "pdf", "html", "htm",
			"xlsx", "xls", "doc", "docx", "csv", "eml", "msg",
			"xml", "epub",
			// Image types
			"jpg", "jpeg", "png", "gif", "webp", "svg",
			// Audio types (if needed)
			"mp3", "wav", "flac", "aac", "ogg", "wma", "m4a",
			// Video types (if needed)
			"mp4", "mov", "avi", "mkv", "flv", "wmv", "webm",
		},
		AllowedMimeTypes: []string{
			// Text types
			"text/plain", "text/markdown", "text/html", "text/xml", "text/csv",
			// Document types
			"application/pdf",
			"application/msword",
			"application/vnd.openxmlformats-officedocument.wordprocessingml.document",
			"application/vnd.ms-excel",
			"application/vnd.openxmlformats-officedocument.spreadsheetml.sheet",
			"application/epub+zip",
			// Image types
			"image/jpeg", "image/png", "image/gif", "image/webp", "image/svg+xml",
			// Audio types
			"audio/mpeg", "audio/wav", "audio/flac", "audio/aac", "audio/ogg",
			// Video types
			"video/mp4", "video/quicktime", "video/x-msvideo", "video/webm",
		},
	}
}

func getDefaultWorkflowFileConfig() *WorkflowFileConfig {
	return &WorkflowFileConfig{
		MaxFileSize:    15 * 1024 * 1024, // 15MB
		MaxFilesPerRun: 10,
		AllowedExtensions: []string{
			"txt", "md", "pdf", "docx", "xlsx",
			"jpg", "jpeg", "png", "gif", "webp",
		},
		AllowedMimeTypes: []string{
			"text/plain", "application/pdf",
			"application/vnd.openxmlformats-officedocument.wordprocessingml.document",
			"application/vnd.openxmlformats-officedocument.spreadsheetml.sheet",
			"image/jpeg", "image/png", "image/gif", "image/webp",
		},
	}
}

// ValidateFileForWorkflow validates if a file meets workflow requirements.
//
// This function checks if a file is suitable for workflow processing by validating
// its size, extension, and MIME type against configured limits and allowed types.
//
// Parameters:
//   - filename: Name of the file including extension (e.g., "document.pdf")
//   - size: File size in bytes
//   - mimeType: MIME type of the file (e.g., "application/pdf")
//
// Returns:
//   - error: Non-nil if validation fails, with descriptive error message
//
// Validation Rules:
//   - File size must not exceed MaxFileSize
//   - File extension must be in AllowedExtensions list
//   - MIME type must be in AllowedMimeTypes list
//
// Error Messages:
//   - "file size X exceeds maximum allowed size Y"
//   - "file extension X is not allowed"
//   - "MIME type X is not allowed"
//
// Example:
//
//	err := ValidateFileForWorkflow("report.pdf", 2048000, "application/pdf")
//	if err != nil {
//	    return fmt.Errorf("file validation failed: %w", err)
//	}
func ValidateFileForWorkflow(filename string, size int64, mimeType string) error {
	cfg := GetWorkflowFileConfig()

	if size > cfg.MaxFileSize {
		return fmt.Errorf("file size %d exceeds maximum allowed size %d", size, cfg.MaxFileSize)
	}

	ext := strings.ToLower(filepath.Ext(filename))
	if ext != "" && strings.HasPrefix(ext, ".") {
		ext = ext[1:]
	}

	extensionAllowed := false
	for _, allowedExt := range cfg.AllowedExtensions {
		if ext == allowedExt {
			extensionAllowed = true
			break
		}
	}

	if !extensionAllowed {
		return fmt.Errorf("file extension %s is not allowed", ext)
	}

	mimeTypeAllowed := false
	for _, allowedMime := range cfg.AllowedMimeTypes {
		if mimeType == allowedMime {
			mimeTypeAllowed = true
			break
		}
	}

	if !mimeTypeAllowed {
		return fmt.Errorf("MIME type %s is not allowed", mimeType)
	}

	return nil
}
