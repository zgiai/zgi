package pipeline

import (
	"fmt"
	"github.com/zgiai/zgi/api/internal/capabilities/contentparse/engines/hyperparse/internal/core/apperr"
	"github.com/zgiai/zgi/api/internal/capabilities/contentparse/engines/hyperparse/internal/core/model"
	"path/filepath"
	"strings"
)

// Adapter defines the minimum contract for format-specific parsers.
type Adapter interface {
	Format() string
	Parse(path string) (*model.Document, error)
}

// Router maintains extension to adapter mapping.
type Router struct {
	adaptersByFormat map[string]Adapter
	extToFormat      map[string]string
}

func NewRouter() *Router {
	return &Router{
		adaptersByFormat: map[string]Adapter{},
		extToFormat:      map[string]string{},
	}
}

// RegisterAdapter binds a format adapter with one or more file extensions.
func (r *Router) RegisterAdapter(adapter Adapter, extensions ...string) {
	format := strings.ToLower(strings.TrimSpace(adapter.Format()))
	r.adaptersByFormat[format] = adapter
	for _, ext := range extensions {
		normExt := normalizeExt(ext)
		if normExt == "" {
			continue
		}
		r.extToFormat[normExt] = format
	}
}

func (r *Router) Parse(path string) (*model.Document, error) {
	ext := normalizeExt(filepath.Ext(path))
	if ext == "" {
		return nil, apperr.New(apperr.CodeBadInput, fmt.Sprintf("unable to detect file extension for path: %s", path))
	}

	format, ok := r.extToFormat[ext]
	if !ok {
		return nil, apperr.New(apperr.CodeUnknownFormat, fmt.Sprintf("unsupported file extension: %s", ext))
	}

	return r.ParseWithFormat(path, format)
}

// ParseWithFormat bypasses extension mapping and forces a specific adapter format.
func (r *Router) ParseWithFormat(path, format string) (*model.Document, error) {
	format = strings.TrimSpace(strings.ToLower(format))
	if format == "" {
		return nil, apperr.New(apperr.CodeBadInput, "format must not be empty")
	}

	adapter, ok := r.adaptersByFormat[format]
	if !ok {
		return nil, apperr.New(apperr.CodeNoAdapterRegistered, fmt.Sprintf("no adapter registered for format: %s", format))
	}

	doc, err := adapter.Parse(path)
	if err != nil {
		return nil, apperr.Wrap(apperr.CodeAdapterParseFailed, fmt.Sprintf("adapter parse failed for format: %s", format), err)
	}
	if doc != nil {
		doc.Format = format
	}
	return doc, nil
}

func normalizeExt(ext string) string {
	ext = strings.TrimSpace(strings.ToLower(ext))
	if ext == "" {
		return ""
	}
	if !strings.HasPrefix(ext, ".") {
		ext = "." + ext
	}
	return ext
}
