package entities

import (
	"fmt"
	"strings"

	"github.com/zgiai/ginext/internal/modules/app/workflow/shared"
)

type File struct {
	ID             string `json:"id"`
	WorkspaceID    string `json:"workspace_id"`
	Type           string `json:"type"`
	TransferMethod string `json:"transfer_method"`
	RemoteURL      string `json:"remote_url"`
	Filename       string `json:"filename"`
	Extension      string `json:"extension"`
	MimeType       string `json:"mime_type"`
	Size           int64  `json:"size"`
	StorageKey     string `json:"storage_key"`
}

// FileAttribute enumeration
type FileAttribute string

const (
	FileAttributeURL            FileAttribute = "url"
	FileAttributeName           FileAttribute = "name"
	FileAttributeSize           FileAttribute = "size"
	FileAttributeType           FileAttribute = "type"
	FileAttributeExtension      FileAttribute = "extension"
	FileAttributeMimeType       FileAttribute = "mime_type"
	FileAttributeTransferMethod FileAttribute = "transfer_method"
)

type FileSegment struct {
	Value *File `json:"value"`
}

func (f *FileSegment) ToObject() any {
	return f.Value
}

func (f *FileSegment) GetValue() any {
	return f.Value
}

func (f *FileSegment) GetType() shared.SegmentType {
	return shared.SegmentTypeFile
}

func (f *FileSegment) Text() string {
	return ""
}

func (f *FileSegment) Log() string {
	return ""
}

func (f *FileSegment) Markdown() string {
	if f.Value == nil {
		return ""
	}
	if strings.HasPrefix(f.Value.MimeType, "image/") {
		return fmt.Sprintf("![%s](%s)", f.Value.Filename, f.Value.RemoteURL)
	}
	return fmt.Sprintf("[%s](%s)", f.Value.Filename, f.Value.RemoteURL)
}

func (f *FileSegment) Size() int {
	if f.Value == nil {
		return 0
	}
	return int(f.Value.Size)
}
