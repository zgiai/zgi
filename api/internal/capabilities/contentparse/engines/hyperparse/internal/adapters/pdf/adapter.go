package pdf

import (
	"github.com/zgiai/zgi/api/internal/capabilities/contentparse/engines/hyperparse/internal/core/model"
	"path/filepath"
	"strconv"
	"strings"
)

type Adapter struct{}

func (a Adapter) Format() string {
	return "pdf"
}

func (a Adapter) Parse(path string) (*model.Document, error) {
	// Use the local structural probe first so the PDF foundation is validated.
	info, err := InspectBasic(path)
	if err != nil {
		return nil, err
	}
	title := strings.TrimSuffix(filepath.Base(path), filepath.Ext(path))

	return &model.Document{
		ID:        filepath.Base(path),
		Format:    "pdf",
		Title:     title,
		PageCount: info.PageCount,
		Metadata: map[string]string{
			"file_size_bytes": strconv.FormatInt(info.FileSize, 10),
			"pdf_version":     info.Version,
			"count_source":    info.CountSource,
			"title":           info.Title,
			"author":          info.Author,
			"subject":         info.Subject,
			"producer":        info.Producer,
			"creator":         info.Creator,
			"xref_type":       info.XRefType,
			"startxref":       strconv.Itoa(info.StartXRef),
			"has_trailer":     strconv.FormatBool(info.HasTrailer),
			"parser_status":   "self_developed_basic_validator",
		},
	}, nil
}
