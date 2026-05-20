package file

import (
	"fmt"
	"strings"
)

type FileType string

// FileType enumeration constants
const (
	FileTypeImage    FileType = "image"
	FileTypeDocument FileType = "document"
	FileTypeAudio    FileType = "audio"
	FileTypeVideo    FileType = "video"
	FileTypeCustom   FileType = "custom"
)

func (ft FileType) String() string {
	return string(ft)
}

func FileTypeValueOf(value string) (FileType, error) {
	switch value {
	case "image":
		return FileTypeImage, nil
	case "document":
		return FileTypeDocument, nil
	case "audio":
		return FileTypeAudio, nil
	case "video":
		return FileTypeVideo, nil
	case "custom":
		return FileTypeCustom, nil
	default:
		return "", fmt.Errorf("no matching enum found for value '%s'", value)
	}
}

func (ft FileType) IsValid() bool {
	_, err := FileTypeValueOf(string(ft))
	return err == nil
}

func AllFileTypes() []FileType {
	return []FileType{
		FileTypeImage,
		FileTypeDocument,
		FileTypeAudio,
		FileTypeVideo,
		FileTypeCustom,
	}
}

// InferFileType infers the workflow file type from MIME type or extension.
func InferFileType(extension, mimeType string) FileType {
	normalizedMimeType := strings.ToLower(strings.TrimSpace(mimeType))
	switch {
	case strings.HasPrefix(normalizedMimeType, "image/"):
		return FileTypeImage
	case strings.HasPrefix(normalizedMimeType, "audio/"):
		return FileTypeAudio
	case strings.HasPrefix(normalizedMimeType, "video/"):
		return FileTypeVideo
	case isDocumentMimeType(normalizedMimeType):
		return FileTypeDocument
	}

	normalizedExtension := strings.TrimPrefix(strings.ToLower(strings.TrimSpace(extension)), ".")
	switch normalizedExtension {
	case "jpg", "jpeg", "png", "gif", "webp", "svg", "bmp", "tiff", "tif":
		return FileTypeImage
	case "mp3", "wav", "flac", "aac", "ogg", "wma", "m4a":
		return FileTypeAudio
	case "mp4", "mov", "avi", "mkv", "flv", "wmv", "webm":
		return FileTypeVideo
	case "txt", "md", "mdx", "markdown", "pdf", "html", "htm",
		"xlsx", "xls", "doc", "docx", "csv", "eml", "msg",
		"xml", "epub":
		return FileTypeDocument
	default:
		return FileTypeCustom
	}
}

func isDocumentMimeType(mimeType string) bool {
	switch mimeType {
	case "application/pdf",
		"application/msword",
		"application/vnd.ms-excel",
		"application/vnd.openxmlformats-officedocument.wordprocessingml.document",
		"application/vnd.openxmlformats-officedocument.spreadsheetml.sheet",
		"application/json",
		"application/xml",
		"text/plain",
		"text/markdown",
		"text/html",
		"text/csv",
		"text/xml":
		return true
	default:
		return strings.HasPrefix(mimeType, "text/")
	}
}

// NormalizeFileType upgrades legacy raw file types like "file" to concrete types
// when metadata is sufficient, while preserving unknown values for compatibility.
func NormalizeFileType(rawType, extension, mimeType string) string {
	normalizedType := strings.ToLower(strings.TrimSpace(rawType))
	switch normalizedType {
	case string(FileTypeImage), string(FileTypeDocument), string(FileTypeAudio), string(FileTypeVideo), string(FileTypeCustom):
		return normalizedType
	case "", "file":
		inferredType := InferFileType(extension, mimeType)
		if inferredType != FileTypeCustom {
			return inferredType.String()
		}
		return "file"
	default:
		return normalizedType
	}
}
