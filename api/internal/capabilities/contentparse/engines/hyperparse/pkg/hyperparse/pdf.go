package hyperparse

import (
	"context"
	"fmt"
	"math"
	"os"
	"strconv"
	"strings"
	"sync"

	pdfadapter "github.com/zgiai/zgi/api/internal/capabilities/contentparse/engines/hyperparse/internal/adapters/pdf"
	"github.com/zgiai/zgi/api/internal/capabilities/contentparse/engines/hyperparse/internal/export"
	"github.com/zgiai/zgi/api/internal/capabilities/contentparse/engines/hyperparse/internal/inspectsvc"
	pdforchestrator "github.com/zgiai/zgi/api/internal/capabilities/contentparse/engines/hyperparse/internal/orchestrators/pdf"
)

const (
	envContentParseForceVLM = "CONTENT_PARSE_FORCE_VLM"
	envLegacyForceVLM       = "DOCSTILL_FORCE_VLM"
)

var nativeOnlyForceVLMEnvMu sync.Mutex

// InspectPDFOptions configures the full_document PDF pipeline.
type InspectPDFOptions struct {
	// Filename is used for metadata and export display.
	Filename string
	// Data contains the raw PDF bytes.
	Data []byte
	// Mode controls validation strictness, usually "relaxed" or "strict".
	Mode string
	// Basic can be supplied when the caller already ran InspectBasicBytes.
	Basic *pdfadapter.BasicInfo
	// NativeOnly ignores process-level ForceVLM and runs only the native pass.
	NativeOnly bool
}

// InspectPDFFullDocument returns the root full_document map produced by the PDF
// orchestrator. By default it runs only native parsing; when ForceVLM is enabled
// it follows RunPDFInspect's full-page VLM fallback and merge path.
func InspectPDFFullDocument(ctx context.Context, opt InspectPDFOptions) (map[string]any, error) {
	if len(opt.Data) == 0 {
		return nil, fmt.Errorf("empty pdf bytes")
	}
	mode := opt.Mode
	if mode == "" {
		mode = "relaxed"
	}
	name := opt.Filename
	if name == "" {
		name = "upload.pdf"
	}
	if opt.NativeOnly {
		return inspectPDFFullDocumentNativeOnly(opt.Data, name, mode, opt.Basic)
	}
	if inspectsvc.ForceVLM() {
		res, err := inspectsvc.RunPDFInspect(ctx, inspectsvc.PDFInspectInput{
			Filename:   name,
			Data:       opt.Data,
			Mode:       mode,
			PreferFull: true,
		})
		if err != nil {
			return nil, err
		}
		fd, _ := res["full_document"].(map[string]any)
		if fd == nil {
			return nil, fmt.Errorf("inspect: missing full_document in result")
		}
		return fd, nil
	}
	return pdforchestrator.ParseFullDocumentBytesWithBasic(opt.Data, name, mode, opt.Basic)
}

func inspectPDFFullDocumentNativeOnly(data []byte, name string, mode string, basic *pdfadapter.BasicInfo) (map[string]any, error) {
	nativeOnlyForceVLMEnvMu.Lock()
	defer nativeOnlyForceVLMEnvMu.Unlock()

	prev, hadPrev := os.LookupEnv(envContentParseForceVLM)
	legacyPrev, legacyHadPrev := os.LookupEnv(envLegacyForceVLM)
	_ = os.Setenv(envContentParseForceVLM, "0")
	_ = os.Setenv(envLegacyForceVLM, "0")
	defer func() {
		if hadPrev {
			_ = os.Setenv(envContentParseForceVLM, prev)
		} else {
			_ = os.Unsetenv(envContentParseForceVLM)
		}
		if legacyHadPrev {
			_ = os.Setenv(envLegacyForceVLM, legacyPrev)
		} else {
			_ = os.Unsetenv(envLegacyForceVLM)
		}
	}()

	return pdforchestrator.ParseFullDocumentBytesWithBasic(data, name, mode, basic)
}

// BuildDPTExport converts a full_document result into the DPT-style export object.
func BuildDPTExport(fullDoc map[string]any, filename string, pageCount int) map[string]any {
	return export.BuildDPTExportFromFullDocument(fullDoc, filename, pageCount)
}

// PDFPageCountRelaxed returns the relaxed-mode PDF page count, or 0 on failure.
func PDFPageCountRelaxed(pdfBytes []byte) int {
	if len(pdfBytes) == 0 {
		return 0
	}
	info, err := pdfadapter.InspectBasicBytes(pdfBytes, "relaxed")
	if err != nil || info.PageCount < 1 {
		return 0
	}
	return info.PageCount
}

// RenderPDFPagesToDataURLs rasterizes PDF pages into image data URLs.
// maxPages<=0 lets the renderer choose based on document page count.
func RenderPDFPagesToDataURLs(pdfBytes []byte, maxPages int) ([]string, string, error) {
	return inspectsvc.RenderPDFPagesToDataURLs(pdfBytes, maxPages)
}

// RenderPDFPagesToDataURLsWithConcurrency rasterizes pages with caller-controlled concurrency.
func RenderPDFPagesToDataURLsWithConcurrency(pdfBytes []byte, maxPages int, concurrency int) ([]string, string, error) {
	pageResults, renderedPages, engine, err := inspectsvc.StreamRenderPDFSelectedPagesToDataURLs(pdfBytes, nil, maxPages, concurrency)
	if err != nil {
		return nil, "", err
	}
	if len(renderedPages) == 0 {
		return nil, engine, nil
	}
	urls := make([]string, len(renderedPages))
	var errs []string
	for result := range pageResults {
		if result.Err != nil {
			errs = append(errs, result.Err.Error())
			continue
		}
		if result.RenderIndex >= 0 && result.RenderIndex < len(urls) {
			urls[result.RenderIndex] = result.DataURL
		}
	}
	hasRenderedPage := false
	for _, url := range urls {
		if strings.TrimSpace(url) != "" {
			hasRenderedPage = true
			break
		}
	}
	if !hasRenderedPage && len(errs) > 0 {
		return nil, engine, fmt.Errorf("render pdf pages: %s", strings.Join(errs, " | "))
	}
	return urls, engine, nil
}

// RenderPDFPreviewPagesToDataURLs returns lighter page images for UI previews.
// It caps the long edge to avoid slow previews for oversized PDF canvases.
func RenderPDFPreviewPagesToDataURLs(pdfBytes []byte, maxPages int) ([]string, string, error) {
	return inspectsvc.RenderPDFPreviewPagesToDataURLs(pdfBytes, maxPages)
}

// PDFHasOversizedPagesRelaxed reports whether relaxed-mode page geometry contains oversized pages.
func PDFHasOversizedPagesRelaxed(pdfBytes []byte) bool {
	if len(pdfBytes) == 0 {
		return false
	}
	pageInfos, err := pdfadapter.DetectPageInfosBytes(pdfBytes, "relaxed")
	if err != nil {
		return false
	}
	for _, pageInfo := range pageInfos {
		widthPt, heightPt, ok := parsePDFBoxSizePt(pageInfo.MediaBox)
		if !ok {
			continue
		}
		longSide := math.Max(widthPt, heightPt)
		area := widthPt * heightPt
		if longSide >= 2000 || area >= 5_000_000 {
			return true
		}
	}
	return false
}

// GeometryToken is an atomic text token from the PDF content stream, usually a
// word or a continuous non-whitespace span. Coordinates are PDF-native and
// bottom-origin; callers can normalize them with PageGeometry.
type GeometryToken struct {
	PageIndex int     `json:"page_index"`
	Text      string  `json:"text"`
	Order     int     `json:"order"`
	Left      float64 `json:"left,omitempty"`
	Bottom    float64 `json:"bottom,omitempty"`
	Right     float64 `json:"right,omitempty"`
	Top       float64 `json:"top,omitempty"`
}

// PageGeometry describes page MediaBox coordinates for normalizing geometry tokens.
type PageGeometry struct {
	PageIndex int     `json:"page_index"`
	Left      float64 `json:"left"`
	Bottom    float64 `json:"bottom"`
	Right     float64 `json:"right"`
	Top       float64 `json:"top"`
}

// ExtractGeometryTokens exposes word-level PDF token extraction plus page geometry.
//
// Typical use: split a chunk that was over-merged by native extraction into
// smaller atomic chunks with tighter bboxes.
//
// On failure it returns nil, nil, err.
func ExtractGeometryTokens(pdfBytes []byte) ([]GeometryToken, []PageGeometry, error) {
	if len(pdfBytes) == 0 {
		return nil, nil, fmt.Errorf("empty pdf bytes")
	}
	specs, err := pdfadapter.DetectPageRenderSpecsBytes(pdfBytes, pdfadapter.ValidationModeRelaxed)
	if err != nil {
		return nil, nil, fmt.Errorf("detect page specs: %w", err)
	}
	rawTokens := pdfadapter.ExtractTextGeometryTokensFromBytesWithSpecs(pdfBytes, specs)
	tokens := make([]GeometryToken, 0, len(rawTokens))
	for i, t := range rawTokens {
		gt := GeometryToken{
			PageIndex: t.PageIndex,
			Text:      t.Text,
			Order:     i,
		}
		if t.BBox != nil {
			gt.Left = t.BBox.Left
			gt.Bottom = t.BBox.Bottom
			gt.Right = t.BBox.Right
			gt.Top = t.BBox.Top
		}
		tokens = append(tokens, gt)
	}
	pages := make([]PageGeometry, 0, len(specs))
	for i, sp := range specs {
		// Prefer MediaBox and fall back to CropBox.
		l, b, r, t, ok := parsePDFBoxRect(sp.MediaBox)
		if !ok {
			l, b, r, t, ok = parsePDFBoxRect(sp.CropBox)
		}
		if !ok {
			continue
		}
		pages = append(pages, PageGeometry{
			PageIndex: i,
			Left:      l,
			Bottom:    b,
			Right:     r,
			Top:       t,
		})
	}
	return tokens, pages, nil
}

// parsePDFBoxRect parses a PDF box string in "x0 y0 x1 y1" form.
func parsePDFBoxRect(raw string) (left, bottom, right, top float64, ok bool) {
	fields := strings.Fields(strings.TrimSpace(raw))
	if len(fields) != 4 {
		return 0, 0, 0, 0, false
	}
	x0, err := strconv.ParseFloat(fields[0], 64)
	if err != nil {
		return 0, 0, 0, 0, false
	}
	y0, err := strconv.ParseFloat(fields[1], 64)
	if err != nil {
		return 0, 0, 0, 0, false
	}
	x1, err := strconv.ParseFloat(fields[2], 64)
	if err != nil {
		return 0, 0, 0, 0, false
	}
	y1, err := strconv.ParseFloat(fields[3], 64)
	if err != nil {
		return 0, 0, 0, 0, false
	}
	if x1 < x0 {
		x0, x1 = x1, x0
	}
	if y1 < y0 {
		y0, y1 = y1, y0
	}
	if x1-x0 <= 0 || y1-y0 <= 0 {
		return 0, 0, 0, 0, false
	}
	return x0, y0, x1, y1, true
}

func parsePDFBoxSizePt(raw string) (widthPt, heightPt float64, ok bool) {
	fields := strings.Fields(strings.TrimSpace(raw))
	if len(fields) != 4 {
		return 0, 0, false
	}
	left, err := strconv.ParseFloat(fields[0], 64)
	if err != nil {
		return 0, 0, false
	}
	bottom, err := strconv.ParseFloat(fields[1], 64)
	if err != nil {
		return 0, 0, false
	}
	right, err := strconv.ParseFloat(fields[2], 64)
	if err != nil {
		return 0, 0, false
	}
	top, err := strconv.ParseFloat(fields[3], 64)
	if err != nil {
		return 0, 0, false
	}
	widthPt = math.Abs(right - left)
	heightPt = math.Abs(top - bottom)
	if widthPt <= 0 || heightPt <= 0 {
		return 0, 0, false
	}
	return widthPt, heightPt, true
}
