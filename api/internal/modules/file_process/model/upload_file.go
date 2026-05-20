package model

import (
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/zgiai/ginext/config"
	"gorm.io/gorm"
)

// CreatedByRole creator role enum
type CreatedByRole string

const (
	CreatedByRoleAccount CreatedByRole = "account"
	CreatedByRoleEndUser CreatedByRole = "end_user"
)

// UploadFile upload file model
type UploadFile struct {
	ID             string        `json:"id" gorm:"type:varchar(255);primaryKey"`
	OrganizationID string        `json:"organization_id" gorm:"type:varchar(255);not null;index"`
	WorkspaceID    *string       `json:"workspace_id" gorm:"type:varchar(255);index"`
	IsTemporary    bool          `json:"is_temporary" gorm:"default:false"`
	StorageType    string        `json:"storage_type" gorm:"type:varchar(255);not null"`
	Key            string        `json:"key" gorm:"type:varchar(255);not null"`
	Name           string        `json:"name" gorm:"type:varchar(255);not null"`
	Size           int64         `json:"size" gorm:"not null"`
	Extension      string        `json:"extension" gorm:"type:varchar(255);not null"`
	MimeType       string        `json:"mime_type" gorm:"type:varchar(255)"`
	CreatedByRole  CreatedByRole `json:"created_by_role" gorm:"type:varchar(255);not null"`
	CreatedBy      string        `json:"created_by" gorm:"type:varchar(255);not null"`
	CreatedAt      time.Time     `json:"created_at" gorm:"not null"`
	Used           bool          `json:"used" gorm:"default:false"`
	UsedBy         *string       `json:"used_by" gorm:"type:varchar(255)"`
	UsedAt         *time.Time    `json:"used_at"`
	Hash           string        `json:"hash" gorm:"type:varchar(255)"`
	SourceURL      string        `json:"source_url" gorm:"type:text"`
	ContentText    *string       `json:"content_text" gorm:"type:longtext"` // Parsed text content

	// Archive fields
	IsArchived bool       `json:"is_archived" gorm:"default:false"`
	ArchivedAt *time.Time `json:"archived_at"`
	ArchivedBy *string    `json:"archived_by" gorm:"type:varchar(255)"`
}

// BeforeCreate GORM hook, generate UUID before creation
func (u *UploadFile) BeforeCreate(tx *gorm.DB) error {
	if u.ID == "" {
		u.ID = uuid.New().String()
	}
	if u.CreatedAt.IsZero() {
		u.CreatedAt = time.Now()
	}
	return nil
}

// TableName specifies table name
func (UploadFile) TableName() string {
	return "upload_files"
}

// FileUploadConfig file upload configuration
type FileUploadConfig struct {
	FileSizeLimit           int64 `json:"file_size_limit"`            // File size limit (MB)
	BatchCountLimit         int   `json:"batch_count_limit"`          // Batch upload count limit
	ImageFileSizeLimit      int64 `json:"image_file_size_limit"`      // Image file size limit (MB)
	VideoFileSizeLimit      int64 `json:"video_file_size_limit"`      // Video file size limit (MB)
	AudioFileSizeLimit      int64 `json:"audio_file_size_limit"`      // Audio file size limit (MB)
	WorkflowFileUploadLimit int   `json:"workflow_file_upload_limit"` // Workflow file upload limit
}

// FileSource file source type
type FileSource string

const (
	FileSourceDatasets FileSource = "datasets"
	FileSourceWorkflow FileSource = "workflow"
)

// File extension constants
var (
	// Document extensions
	DocumentExtensions []string

	// Image extensions
	ImageExtensions = []string{
		"jpg", "jpeg", "png", "webp", "gif", "svg",
	}

	// Video extensions
	VideoExtensions = []string{
		"mp4", "mov", "avi", "mkv", "flv", "wmv", "webm",
	}

	// Audio extensions
	AudioExtensions = []string{
		"mp3", "wav", "flac", "aac", "ogg", "wma", "m4a",
	}

	// once ensures document extensions are initialized only once
	once sync.Once
)

// IsDocumentExtension checks if it's a document extension
func IsDocumentExtension(ext string) bool {
	// Lazy initialization of document extensions
	once.Do(initDocumentExtensions)

	for _, docExt := range DocumentExtensions {
		if ext == docExt {
			return true
		}
	}
	return false
}

// IsImageExtension checks if it's an image extension
func IsImageExtension(ext string) bool {
	for _, imgExt := range ImageExtensions {
		if ext == imgExt {
			return true
		}
	}
	return false
}

// IsVideoExtension checks if it's a video extension
func IsVideoExtension(ext string) bool {
	for _, vidExt := range VideoExtensions {
		if ext == vidExt {
			return true
		}
	}
	return false
}

// IsAudioExtension checks if it's an audio extension
func IsAudioExtension(ext string) bool {
	for _, audExt := range AudioExtensions {
		if ext == audExt {
			return true
		}
	}
	return false
}

// GetSupportedDocumentExtensions returns the supported document extensions
func GetSupportedDocumentExtensions() []string {
	// Lazy initialization of document extensions
	once.Do(initDocumentExtensions)
	return DocumentExtensions
}

// getDocumentExtensions returns document extensions based on ETL configuration
func getDocumentExtensions() []string {
	// Lazy initialization of document extensions
	once.Do(initDocumentExtensions)
	return DocumentExtensions
}

// initDocumentExtensions initializes document extensions based on ETL configuration
func initDocumentExtensions() {
	// Import config here to avoid circular dependency
	cfg := config.GlobalConfig

	// Base document types supported without any third-party API
	baseExtensions := []string{
		"txt", "markdown", "md", "mdx", "pdf", "html", "htm",
		"xlsx", "xls", "docx", "csv", "doc",
		"eml", "msg", "xml", "epub",
	}

	if cfg == nil {
		// Fallback when config is nil
		DocumentExtensions = baseExtensions
		addUppercaseExtensions()
		return
	}

	// Use map for automatic deduplication
	extMap := make(map[string]bool)

	// Add base extensions to map
	for _, ext := range baseExtensions {
		extMap[ext] = true
	}

	// Add additional extensions based on ETL type
	switch cfg.ETL.Type {
	case "Unstructured":
		// Unstructured ETL type
		additionalExts := []string{
			"abw", "cwk", "dbf", "dif", "dot", "dotm", "epub", "et", "eth", "fods", "hwp", "mcw", "msg", "mw", "odt", "org", "pbd", "pot", "pptm", "prn", "rst", "rtf", "sdp", "sxg", "tsv", "zabw", "doc", "docx",
			"p7s", "bmp", "heic", "jpeg", "jpg", "png", "tiff", "tif",
		}
		addExtensionsToMap(extMap, additionalExts)

	case "LandingAI":
		// LandingAI ETL type
		additionalExts := []string{
			"jpg", "jpeg", "png", "apng", "bmp", "gif", "webp", "tiff", "tif",
			"psd", "pcx", "dcx", "dds", "jp2", "ppm", "tga", "dib", "icns", "gd",
		}
		addExtensionsToMap(extMap, additionalExts)

	case "Reducto":
		// Reducto ETL type
		additionalExts := []string{
			"png", "jpg", "jpeg", "gif", "bmp", "tiff", "tif",
			"pcx", "ppm", "apng", "psd", "cur", "dcx", "heic", "ftex", "pixar",
			"rtf", "xlsm", "xltx", "xltm", "qpw", "dotx", "wpd", "doc", "docx",
		}
		addExtensionsToMap(extMap, additionalExts)

	case "Mixed":
		// Mixed ETL type
		if cfg.ETL.UnstructuredAPIKey != "" {
			unstructuredExts := []string{
				"abw", "cwk", "dbf", "dif", "dot", "dotm", "epub", "et", "eth", "fods", "hwp", "mcw", "msg", "mw", "odt", "org", "pbd", "pot", "pptm", "prn", "rst", "rtf", "sdp", "sxg", "tsv", "zabw", "doc", "docx",
				"p7s", "bmp", "heic", "jpeg", "jpg", "png", "tiff", "tif",
			}
			addExtensionsToMap(extMap, unstructuredExts)
		}

		if cfg.ETL.LandingAIAPIKey != "" {
			landingAIExts := []string{
				"jpg", "jpeg", "png", "apng", "bmp", "gif", "webp", "tiff", "tif",
				"psd", "pcx", "dcx", "dds", "jp2", "ppm", "tga", "dib", "icns", "gd",
			}
			addExtensionsToMap(extMap, landingAIExts)
		}

		if cfg.ETL.ReductoAPIKey != "" {
			reductoExts := []string{
				"png", "jpg", "jpeg", "gif", "bmp", "tiff", "tif",
				"pcx", "ppm", "apng", "psd", "cur", "dcx", "heic", "ftex", "pixar",
				"rtf", "xlsm", "xltx", "xltm", "qpw", "dotx", "wpd", "doc", "docx",
			}
			addExtensionsToMap(extMap, reductoExts)
		}

	default:
		// Default ETL type - only base extensions
		// extMap already contains base extensions
	}

	// Convert map to slice
	DocumentExtensions = make([]string, 0, len(extMap))
	for ext := range extMap {
		DocumentExtensions = append(DocumentExtensions, ext)
	}

	// Add uppercase extensions for case-insensitive matching
	addUppercaseExtensions()
}

// addExtensionsToMap adds extensions to the map for deduplication
func addExtensionsToMap(extMap map[string]bool, extensions []string) {
	for _, ext := range extensions {
		extMap[ext] = true
	}
}

// addUppercaseExtensions adds uppercase versions of all extensions
func addUppercaseExtensions() {
	extensions := make([]string, len(DocumentExtensions))
	copy(extensions, DocumentExtensions)
	for _, ext := range extensions {
		DocumentExtensions = append(DocumentExtensions, strings.ToUpper(ext))
	}
}
