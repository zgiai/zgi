package file

import (
	"errors"
	"fmt"
	"strings"
)

// FILE_MODEL_IDENTITY is a special identifier used to distinguish between
// new and old data formats during serialization and deserialization.
const FILE_MODEL_IDENTITY = "__zgi__file__"

func MaybeFileObject(obj map[string]any) bool {
	if obj == nil {
		return false
	}
	identity, exists := obj["zgi_model_identity"]
	if !exists {
		return false
	}
	identityStr, ok := identity.(string)
	return ok && identityStr == FILE_MODEL_IDENTITY
}

type FileTransferMethod string

const (
	FileTransferMethodLocalFile      FileTransferMethod = "local_file"
	FileTransferMethodRemoteURL      FileTransferMethod = "remote_url"
	FileTransferMethodToolFile       FileTransferMethod = "tool_file"
	FileTransferMethodDatasourceFile FileTransferMethod = "datasource_file"
)

type File struct {
	ZgiModelIdentity string                 `json:"zgi_model_identity"`
	ID               *string                `json:"id,omitempty"`
	TenantID         string                 `json:"tenant_id"`
	Type             FileType               `json:"type"`
	TransferMethod   FileTransferMethod     `json:"transfer_method"`
	RemoteURL        *string                `json:"remote_url,omitempty"`
	RelatedID        *string                `json:"related_id,omitempty"`
	Filename         *string                `json:"filename,omitempty"`
	Extension        *string                `json:"extension,omitempty"`
	MimeType         *string                `json:"mime_type,omitempty"`
	Size             int64                  `json:"size"`
	storageKey       string
	URL              *string                `json:"url,omitempty"`
}

func NewFile(tenantID string, fileType FileType, transferMethod FileTransferMethod, opts ...FileOption) *File {
	f := &File{
		ZgiModelIdentity: FILE_MODEL_IDENTITY,
		TenantID:         tenantID,
		Type:             fileType,
		TransferMethod:   transferMethod,
		Size:             -1,
	}

	for _, opt := range opts {
		opt(f)
	}

	return f
}

type FileOption func(*File)

func WithID(id string) FileOption {
	return func(f *File) {
		f.ID = &id
	}
}

func WithRemoteURL(url string) FileOption {
	return func(f *File) {
		f.RemoteURL = &url
	}
}

func WithRelatedID(id string) FileOption {
	return func(f *File) {
		f.RelatedID = &id
	}
}

func WithFilename(filename string) FileOption {
	return func(f *File) {
		f.Filename = &filename
	}
}

func WithExtension(extension string) FileOption {
	return func(f *File) {
		f.Extension = &extension
	}
}

func WithMimeType(mimeType string) FileOption {
	return func(f *File) {
		f.MimeType = &mimeType
	}
}

func WithSize(size int) FileOption {
	return func(f *File) {
		f.Size = int64(size)
	}
}

func WithStorageKey(key string) FileOption {
	return func(f *File) {
		f.storageKey = key
	}
}

func WithURL(url string) FileOption {
	return func(f *File) {
		f.URL = &url
	}
}

func (f *File) IsLocal() bool {
	return f.TransferMethod == FileTransferMethodLocalFile
}

func (f *File) IsRemote() bool {
	return f.TransferMethod == FileTransferMethodRemoteURL
}

func (f *File) IsToolFile() bool {
	return f.TransferMethod == FileTransferMethodToolFile
}

func (f *File) ToDict() map[string]any {
	result := make(map[string]any)

	result["zgi_model_identity"] = f.ZgiModelIdentity
	if f.ID != nil {
		result["id"] = *f.ID
	}
	result["tenant_id"] = f.TenantID
	result["type"] = f.Type
	result["transfer_method"] = f.TransferMethod
	if f.RemoteURL != nil {
		result["remote_url"] = *f.RemoteURL
	}
	if f.RelatedID != nil {
		result["related_id"] = *f.RelatedID
	}
	if f.Filename != nil {
		result["filename"] = *f.Filename
	}
	if f.Extension != nil {
		result["extension"] = *f.Extension
	}
	if f.MimeType != nil {
		result["mime_type"] = *f.MimeType
	}
	result["size"] = f.Size

	if url, err := f.GenerateURL(); err == nil && url != nil {
		result["url"] = *url
	}

	return result
}

func (f *File) Markdown() string {
	url, err := f.GenerateURL()
	if err != nil || url == nil {
		return ""
	}

	if f.Type == FileTypeImage {
		filename := ""
		if f.Filename != nil {
			filename = *f.Filename
		}
		return fmt.Sprintf("![%s](%s)", filename, *url)
	} else {
		displayName := *url
		if f.Filename != nil {
			displayName = *f.Filename
		}
		return fmt.Sprintf("[%s](%s)", displayName, *url)
	}
}

func (f *File) GenerateURL() (*string, error) {
	switch f.TransferMethod {
	case FileTransferMethodRemoteURL:
		return f.RemoteURL, nil
	case FileTransferMethodLocalFile:
		if f.RelatedID == nil {
			return nil, errors.New("missing file related_id")
		}
		url, err := GetSignedFileURL(*f.RelatedID)
		return &url, err
	case FileTransferMethodToolFile, FileTransferMethodDatasourceFile:
		if f.RelatedID == nil {
			return nil, errors.New("missing file related_id")
		}
		if f.Extension == nil {
			return nil, errors.New("missing file extension")
		}
		url, err := SignToolFile(*f.RelatedID, *f.Extension)
		return &url, err
	default:
		return nil, fmt.Errorf("unsupported transfer method: %s", f.TransferMethod)
	}
}

func (f *File) ToPluginParameter() map[string]any {
	result := map[string]any{
		"zgi_model_identity": FILE_MODEL_IDENTITY,
		"type":               f.Type,
		"size":               f.Size,
	}

	if f.MimeType != nil {
		result["mime_type"] = *f.MimeType
	}
	if f.Filename != nil {
		result["filename"] = *f.Filename
	}
	if f.Extension != nil {
		result["extension"] = *f.Extension
	}

	if url, err := f.GenerateURL(); err == nil && url != nil {
		result["url"] = *url
	}

	return result
}

func (f *File) Validate() error {
	switch f.TransferMethod {
	case FileTransferMethodRemoteURL:
		if f.RemoteURL == nil || *f.RemoteURL == "" {
			return errors.New("missing file url")
		}
		if !strings.HasPrefix(*f.RemoteURL, "http") {
			return errors.New("invalid file url")
		}
	case FileTransferMethodLocalFile:
		if f.RelatedID == nil || *f.RelatedID == "" {
			return errors.New("missing file related_id")
		}
	case FileTransferMethodToolFile:
		if f.RelatedID == nil || *f.RelatedID == "" {
			return errors.New("missing file related_id")
		}
	default:
		return fmt.Errorf("unsupported transfer method: %s", f.TransferMethod)
	}
	return nil
}
