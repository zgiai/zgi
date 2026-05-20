package local

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	imageadapter "github.com/zgiai/ginext/internal/capabilities/contentparse/engines/hyperparse/internal/adapters/image"
	coremodel "github.com/zgiai/ginext/internal/capabilities/contentparse/engines/hyperparse/internal/core/model"
	"github.com/zgiai/ginext/internal/capabilities/contentparse/engines/hyperparse/pkg/hyperparse"
	extractcommon "github.com/zgiai/ginext/internal/capabilities/contentparse/engines/hyperparse/pkg/providers/common"
	extractvlm "github.com/zgiai/ginext/internal/capabilities/contentparse/engines/hyperparse/pkg/providers/vlm"
)

// Client is the local parser entry point.
type Client struct{}

func New() *Client {
	return &Client{}
}

func (c *Client) ParseFile(ctx context.Context, path string, opts extractcommon.ParseOptions) (*extractcommon.DocumentResult, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	return c.ParseBytes(ctx, filepath.Base(path), data, opts)
}

func (c *Client) ParseBytes(ctx context.Context, filename string, data []byte, opts extractcommon.ParseOptions) (*extractcommon.DocumentResult, error) {
	startedAt := time.Now()
	if strings.EqualFold(filepath.Ext(filename), ".pdf") {
		doc, err := c.parsePDF(ctx, filename, data, opts)
		if err != nil {
			return nil, err
		}
		return extractcommon.EnrichStructuredOutput(doc), nil
	}
	if isLocalImageFilename(filename) {
		doc, err := c.parseLocalImageWithOptions(ctx, filename, data, opts)
		if err != nil {
			return nil, err
		}
		finalizeLocalParseObservability(doc, nil, startedAt)
		return extractcommon.EnrichStructuredOutput(doc), nil
	}
	doc, err := c.parseLight(ctx, filename, data)
	if err != nil {
		return nil, err
	}
	finalizeLocalParseObservability(doc, nil, startedAt)
	return extractcommon.EnrichStructuredOutput(doc), nil
}

func (c *Client) parseLocalImageWithOptions(ctx context.Context, filename string, data []byte, opts extractcommon.ParseOptions) (*extractcommon.DocumentResult, error) {
	if localImageVLMEnabled() || opts.ForceLocalVLM {
		doc, err := extractvlm.New().ParseBytes(ctx, filename, data, opts)
		if err == nil && doc != nil {
			doc.Source = "local+vlm:image"
			if doc.Diagnostics == nil {
				doc.Diagnostics = map[string]any{}
			}
			doc.Diagnostics["local_image_vlm_status"] = "applied"
			doc.Diagnostics["local_image_vlm_fallback"] = false
			return doc, nil
		}
	}

	doc, err := c.parseImageWithOCRBudget(filename, data, opts)
	if err != nil {
		return nil, err
	}
	if opts.EnableImageVLMFallback && looksLikePoorLocalImageDocument(doc) {
		vlmDoc, vlmErr := extractvlm.New().ParseBytes(ctx, filename, data, opts)
		if vlmErr == nil && vlmDoc != nil {
			vlmDoc.Source = "local+vlm:image"
			if vlmDoc.Diagnostics == nil {
				vlmDoc.Diagnostics = map[string]any{}
			}
			vlmDoc.Diagnostics["local_image_vlm_status"] = "fallback_to_vlm"
			vlmDoc.Diagnostics["local_image_vlm_fallback"] = true
			return vlmDoc, nil
		}
		if doc.Diagnostics == nil {
			doc.Diagnostics = map[string]any{}
		}
		doc.Diagnostics["local_image_vlm_status"] = "fallback_attempt_failed"
		if vlmErr != nil {
			doc.Diagnostics["local_image_vlm_warning"] = vlmErr.Error()
		}
	}
	return doc, nil
}

func (c *Client) parseImageWithOCRBudget(filename string, data []byte, opts extractcommon.ParseOptions) (*extractcommon.DocumentResult, error) {
	ext := filepath.Ext(filename)
	f, err := os.CreateTemp("", "hyperparse-local-image-*"+ext)
	if err != nil {
		return nil, fmt.Errorf("temp image file: %w", err)
	}
	tmpPath := f.Name()
	defer func() { _ = os.Remove(tmpPath) }()
	if _, err := f.Write(data); err != nil {
		_ = f.Close()
		return nil, fmt.Errorf("write temp image: %w", err)
	}
	if err := f.Close(); err != nil {
		return nil, fmt.Errorf("close temp image: %w", err)
	}

	doc, err := imageadapter.ParsePathWithOptions(tmpPath, imageadapter.ParseOptions{
		AggressiveRetry: opts.ImageRetryAggressive,
	})
	if err != nil {
		return nil, err
	}
	out := c.documentToResult(doc, filename)
	out.Source = "local:ocr:image"
	return out, nil
}

func isLocalImageFilename(filename string) bool {
	switch strings.ToLower(strings.TrimSpace(filepath.Ext(filename))) {
	case ".png", ".jpg", ".jpeg", ".webp", ".tif", ".tiff":
		return true
	default:
		return false
	}
}

func localImageVLMEnabled() bool {
	return localImageVLMSetting() == "force"
}

func localImageVLMSetting() string {
	raw := strings.ToLower(strings.TrimSpace(os.Getenv("LOCAL_IMAGE_VLM")))
	switch raw {
	case "1", "true", "yes", "on", "force", "always":
		return "force"
	case "0", "false", "no", "off", "disabled":
		return "disabled"
	default:
		return "disabled"
	}
}

func (c *Client) parseLight(ctx context.Context, filename string, data []byte) (*extractcommon.DocumentResult, error) {
	ext := strings.ToLower(strings.TrimSpace(filepath.Ext(filename)))
	doc, err := hyperparse.NewClient().ParseBytes(ctx, filename, data)
	if err != nil {
		if !supportsLocalExtraExt(ext) {
			return nil, err
		}
		return parseLocalExtraFormat(filename, data, ext)
	}
	out := c.documentToResult(doc, filename)
	return out, nil
}

func (c *Client) documentToResult(doc *coremodel.Document, filename string) *extractcommon.DocumentResult {
	out := &extractcommon.DocumentResult{
		DocID:       doc.ID,
		FileName:    filename,
		PageCount:   doc.PageCount,
		Source:      "local:light",
		Diagnostics: map[string]any{},
	}
	ord := 0
	for _, sec := range doc.Sections {
		for _, b := range sec.Blocks {
			ord++
			out.Chunks = append(out.Chunks, extractcommon.Chunk{
				ID:        makeChunkID(filename, ord),
				Type:      normalizeLightType(b.Type),
				Page:      b.Page,
				BBox:      blockBBoxToExtractBBox(b.BBox),
				Text:      strings.TrimSpace(b.Text),
				Markdown:  strings.TrimSpace(b.Text),
				Ordinal:   ord,
				Precision: b.Precision,
				Payload:   copyBlockPayload(b.Payload),
			})
		}
	}
	if len(out.Chunks) > 0 {
		md := make([]string, 0, len(out.Chunks))
		for _, ch := range out.Chunks {
			if ch.Markdown != "" {
				md = append(md, ch.Markdown)
			}
		}
		out.Markdown = strings.Join(md, "\n\n")
	}
	if len(doc.Metadata) > 0 {
		if out.Diagnostics == nil {
			out.Diagnostics = map[string]any{}
		}
		for key, value := range doc.Metadata {
			out.Diagnostics[key] = value
		}
		if isLocalImageFilename(filename) {
			out.Diagnostics["local_image_parse"] = true
		}
	}
	return out
}

func blockBBoxToExtractBBox(in *coremodel.BBox) *extractcommon.BBox {
	if in == nil {
		return nil
	}
	return &extractcommon.BBox{
		Left:   in.Left,
		Top:    in.Top,
		Right:  in.Right,
		Bottom: in.Bottom,
	}
}

func copyBlockPayload(in map[string]any) map[string]any {
	if len(in) == 0 {
		return nil
	}
	out := make(map[string]any, len(in))
	for key, value := range in {
		out[key] = value
	}
	return out
}

func looksLikePoorLocalImageDocument(doc *extractcommon.DocumentResult) bool {
	if doc == nil {
		return false
	}
	text := strings.TrimSpace(doc.Markdown)
	if len([]rune(text)) < 220 {
		return true
	}
	short := 0
	total := 0
	for _, chunk := range doc.Chunks {
		value := strings.TrimSpace(chunk.Text)
		if value == "" {
			continue
		}
		total++
		if len([]rune(value)) <= 2 {
			short++
		}
	}
	if total == 0 {
		return true
	}
	return float64(short)/float64(total) > 0.30
}

func makeChunkID(filename string, ord int) string {
	return fmt.Sprintf("local-%d", ord)
}

func normalizeLightType(t string) string {
	t = strings.ToLower(strings.TrimSpace(t))
	switch t {
	case "heading", "text", "table", "figure", "formula", "marginalia":
		return t
	default:
		return "text"
	}
}
