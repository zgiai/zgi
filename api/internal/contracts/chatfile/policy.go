// Package chatfile defines chat attachment admission independently of parsers
// and model capabilities. It does not grant access to files or execution tools.
package chatfile

import (
	"errors"
	"path/filepath"
	"slices"
	"strings"
)

const (
	MiB             = int64(1024 * 1024)
	MaxTurnBytes    = 100 * MiB
	DefaultCount    = 5
	MaxRequestBytes = MaxTurnBytes + MiB // Multipart envelope, not file content.
)

var (
	ErrUnsupportedType = errors.New("unsupported chat attachment type")
	ErrFileTooLarge    = errors.New("chat attachment exceeds file size limit")
	ErrTurnTooLarge    = errors.New("chat attachments exceed per-turn size limit")
	ErrTooManyFiles    = errors.New("too many chat attachments")
)

// Policy is application-owned and is frozen with the application's version.
// Nil means documents and images; an explicit empty list permits neither.
type Policy struct {
	AllowedFileTypes []string `json:"allowed_file_types"`
}

func (p *Policy) Validate() error {
	if p == nil {
		return nil
	}
	seen := make(map[string]bool, len(p.AllowedFileTypes))
	for _, kind := range p.AllowedFileTypes {
		if (kind != "document" && kind != "image") || seen[kind] {
			return ErrUnsupportedType
		}
		seen[kind] = true
	}
	return nil
}

func Clone(p *Policy) *Policy {
	if p == nil {
		return nil
	}
	return &Policy{AllowedFileTypes: slices.Clone(p.AllowedFileTypes)}
}

// Effective is a server-produced admission contract. All sizes are bytes.
type Effective struct {
	AllowedFileTypes  []string `json:"allowed_file_types"`
	AllowedExtensions []string `json:"allowed_extensions"`
	NumberLimit       int      `json:"number_limit"`
	DocumentMaxBytes  int64    `json:"document_max_bytes"`
	ImageMaxBytes     int64    `json:"image_max_bytes"`
	TurnMaxBytes      int64    `json:"turn_max_bytes"`
}

func Resolve(p *Policy, documentMiB, imageMiB int64, count int) Effective {
	if documentMiB <= 0 {
		documentMiB = 50
	}
	if imageMiB <= 0 {
		imageMiB = 10
	}
	if count <= 0 {
		count = DefaultCount
	}
	types := []string{"document", "image"}
	if p != nil {
		types = slices.Clone(p.AllowedFileTypes)
	}
	result := Effective{AllowedFileTypes: types, NumberLimit: count,
		DocumentMaxBytes: min(documentMiB, 100) * MiB, ImageMaxBytes: min(imageMiB, 100) * MiB, TurnMaxBytes: MaxTurnBytes}
	for _, kind := range types {
		switch kind {
		case "document":
			result.AllowedExtensions = append(result.AllowedExtensions, DocumentExtensions()...)
		case "image":
			result.AllowedExtensions = append(result.AllowedExtensions, ImageExtensions()...)
		}
	}
	return result
}

// DocumentExtensions preserves existing chat document/text formats. This list
// is not the parser capability list and never includes arbitrary executables.
func DocumentExtensions() []string {
	return []string{"txt", "md", "markdown", "mdx", "pdf", "html", "htm", "doc", "docx", "xls", "xlsx", "csv", "eml", "msg", "xml", "epub", "ppt", "pptx", "json", "key", "numbers", "java", "sql"}
}

func ImageExtensions() []string {
	return []string{"jpg", "jpeg", "png", "gif", "webp", "svg"}
}

func Extension(name string) string {
	return strings.TrimPrefix(strings.ToLower(filepath.Ext(name)), ".")
}

func (p Effective) ValidateFile(extension string, size int64) error {
	extension = strings.TrimPrefix(strings.ToLower(extension), ".")
	if !slices.Contains(p.AllowedExtensions, extension) {
		return ErrUnsupportedType
	}
	limit := p.DocumentMaxBytes
	if slices.Contains(ImageExtensions(), extension) {
		limit = p.ImageMaxBytes
	}
	if size < 0 || size > limit {
		return ErrFileTooLarge
	}
	return nil
}

// ValidateTurn uses unique, authorized file references supplied by its caller.
func (p Effective) ValidateTurn(sizes []int64) error {
	if len(sizes) > p.NumberLimit {
		return ErrTooManyFiles
	}
	var total int64
	for _, size := range sizes {
		if size < 0 || size > p.TurnMaxBytes-total {
			return ErrTurnTooLarge
		}
		total += size
	}
	return nil
}
