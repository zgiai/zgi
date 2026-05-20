package hyperparse

import (
	"context"
	"fmt"
	"os"
	"path/filepath"

	"github.com/zgiai/ginext/internal/capabilities/contentparse/engines/hyperparse/internal/core/model"
	"github.com/zgiai/ginext/internal/capabilities/contentparse/engines/hyperparse/internal/core/pipeline"
)

// Client wraps pipeline.Router for lightweight multi-format parsing and optional
// custom adapters. Use InspectPDFFullDocument for heavyweight structured PDF output.
type Client struct {
	router *pipeline.Router
}

// NewClient returns a client with the default pdf/docx/markdown/text/image routes.
func NewClient() *Client {
	return &Client{router: pipeline.NewDefaultRouter()}
}

// NewClientWithRouter returns a client backed by a caller-provided router.
func NewClientWithRouter(r *pipeline.Router) *Client {
	if r == nil {
		r = pipeline.NewDefaultRouter()
	}
	return &Client{router: r}
}

// Router returns the underlying router so callers can register additional formats.
func (c *Client) Router() *pipeline.Router {
	return c.router
}

// ParsePath selects an adapter by file extension and parses the file.
func (c *Client) ParsePath(ctx context.Context, path string) (*model.Document, error) {
	_ = ctx
	return c.router.Parse(path)
}

// ParsePathFormat parses a file using an explicit format such as "pdf" or "markdown".
func (c *Client) ParsePathFormat(ctx context.Context, path, format string) (*model.Document, error) {
	_ = ctx
	return c.router.ParseWithFormat(path, format)
}

// ParseBytes writes in-memory upload bytes to a temporary file and parses them.
// filename is used only for extension detection; the temporary file is removed.
func (c *Client) ParseBytes(ctx context.Context, filename string, data []byte) (*model.Document, error) {
	_ = ctx
	if len(data) == 0 {
		return nil, fmt.Errorf("empty file bytes")
	}
	format := FormatFromFilename(filename)
	if format == "" {
		return nil, fmt.Errorf("unsupported or missing extension in filename %q", filename)
	}
	ext := filepath.Ext(filename)
	f, err := os.CreateTemp("", "hyperparse-sdk-*"+ext)
	if err != nil {
		return nil, fmt.Errorf("temp file: %w", err)
	}
	tmpPath := f.Name()
	defer func() { _ = os.Remove(tmpPath) }()
	if _, err := f.Write(data); err != nil {
		_ = f.Close()
		return nil, fmt.Errorf("write temp: %w", err)
	}
	if err := f.Close(); err != nil {
		return nil, fmt.Errorf("close temp: %w", err)
	}
	return c.router.ParseWithFormat(tmpPath, format)
}
