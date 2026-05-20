package pdf

import (
	"context"
	"fmt"
	"image"
	"image/draw"
	"image/png"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/zgiai/ginext/internal/capabilities/contentparse/engines/hyperparse/internal/binutil"
	localocr "github.com/zgiai/ginext/internal/capabilities/contentparse/engines/hyperparse/internal/ocr"
)

const (
	localOCRCropLeft   = 0.62
	localOCRCropTop    = 0.02
	localOCRCropRight  = 0.985
	localOCRCropBottom = 0.985
)

func shouldOCRRightColumn(
	pageIndex int,
	pageLines []localLayoutLine,
	rightLines []localLayoutLine,
	pageChunks []localLayoutChunk,
	rightCoverageRatio float64,
) bool {
	if !localSidebarOCREnabled() || pageIndex <= 0 || pageIndex > localSidebarOCRMaxPages() {
		return false
	}
	if len(pageLines) < 6 {
		return false
	}
	for _, ch := range pageChunks {
		if ch.HasText && ch.HasBBox && ch.CenterX >= localQualityRightColumnMinX-0.04 && utf8.RuneCountInString(ch.Text) >= 80 {
			return false
		}
	}
	return len(rightLines) == 0 || rightCoverageRatio < 0.45
}

func shouldAddOCRRightColumn(ocrLines []localLayoutLine, pageChunks []localLayoutChunk) bool {
	if len(ocrLines) < 3 {
		return false
	}
	var ocrText strings.Builder
	for _, line := range ocrLines {
		ocrText.WriteString(line.Text)
		ocrText.WriteByte('\n')
	}
	if utf8.RuneCountInString(ocrText.String()) < 60 {
		return false
	}
	ocrNorm := comparableLayoutText(ocrText.String())
	var chunkText strings.Builder
	for _, ch := range pageChunks {
		if ch.HasText && !ch.IsDiagnostic {
			chunkText.WriteString(ch.Text)
			chunkText.WriteByte('\n')
		}
	}
	chunkNorm := comparableLayoutText(chunkText.String())
	return len(ocrNorm) >= 20 && !strings.Contains(chunkNorm, ocrNorm[:min(20, len(ocrNorm))])
}

func localOCRRightColumnLines(pdfBytes []byte, pageIndex int) ([]localLayoutLine, map[string]any) {
	ocrConfig := localocr.LoadConfig(localSidebarOCRTimeout())
	payload := map[string]any{
		"enabled":    localSidebarOCREnabled(),
		"engine":     ocrConfig.EngineName(),
		"page_index": pageIndex,
		"region": map[string]any{
			"left": localOCRCropLeft, "right": localOCRCropRight,
			"top": localOCRCropTop, "bottom": localOCRCropBottom,
		},
	}
	if !localSidebarOCREnabled() {
		payload["status"] = "disabled"
		return nil, payload
	}
	if len(pdfBytes) == 0 {
		payload["status"] = "skipped_no_pdf_bytes"
		return nil, payload
	}
	pdftoppm, err := resolveOCRRendererBinary("pdftoppm", "CONTENT_PARSE_PDFTOPPM_PATH")
	if err != nil {
		payload["status"] = "skipped_missing_pdftoppm"
		payload["warning"] = err.Error()
		return nil, payload
	}

	start := time.Now()
	tmpDir, err := os.MkdirTemp("", "hyperparse-local-sidebar-ocr-*")
	if err != nil {
		payload["status"] = "error"
		payload["warning"] = err.Error()
		return nil, payload
	}
	defer os.RemoveAll(tmpDir)
	pdfPath := filepath.Join(tmpDir, "input.pdf")
	if err := os.WriteFile(pdfPath, pdfBytes, 0600); err != nil {
		payload["status"] = "error"
		payload["warning"] = err.Error()
		return nil, payload
	}
	pagePath, err := renderSinglePagePNG(pdftoppm, pdfPath, tmpDir, pageIndex)
	if err != nil {
		payload["status"] = "render_error"
		payload["warning"] = err.Error()
		return nil, payload
	}
	cropPath, cropW, cropH, nonBlankRatio, err := cropRightColumnPNG(pagePath, tmpDir)
	payload["non_blank_ratio"] = roundFloat(nonBlankRatio, 4)
	if err != nil {
		payload["status"] = "crop_error"
		payload["warning"] = err.Error()
		return nil, payload
	}
	if nonBlankRatio < 0.003 {
		payload["status"] = "blank_crop"
		payload["duration_ms"] = time.Since(start).Milliseconds()
		return nil, payload
	}

	lines, ocrResult, err := runConfiguredOCRLines(ocrConfig, cropPath, cropW, cropH, pageIndex)
	if ocrResult.Engine != "" {
		payload["engine"] = ocrResult.Engine
	}
	if err != nil {
		payload["status"] = "ocr_error"
		payload["warning"] = err.Error()
		payload["duration_ms"] = time.Since(start).Milliseconds()
		return nil, payload
	}
	payload["status"] = "ok"
	payload["line_count"] = len(lines)
	payload["duration_ms"] = time.Since(start).Milliseconds()
	return lines, payload
}

func renderSinglePagePNG(pdftoppm, pdfPath, outDir string, pageIndex int) (string, error) {
	prefix := filepath.Join(outDir, "page")
	ctx, cancel := context.WithTimeout(context.Background(), localSidebarOCRTimeout())
	defer cancel()
	cmd := exec.CommandContext(ctx, pdftoppm, "-r", "220", "-png", "-f", strconv.Itoa(pageIndex), "-l", strconv.Itoa(pageIndex), pdfPath, prefix)
	if b, err := cmd.CombinedOutput(); err != nil {
		return "", fmt.Errorf("pdftoppm: %v (%s)", err, strings.TrimSpace(string(b)))
	}
	paths, _ := filepath.Glob(filepath.Join(outDir, "page-*.png"))
	if len(paths) == 0 {
		paths, _ = filepath.Glob(filepath.Join(outDir, "page_*.png"))
	}
	sort.Strings(paths)
	if len(paths) == 0 {
		return "", fmt.Errorf("pdftoppm rendered no page image")
	}
	return paths[0], nil
}

func resolveOCRRendererBinary(name string, envKey string) (string, error) {
	return binutil.Resolve(name, contentParseEnv(envKey))
}

func cropRightColumnPNG(pagePath, outDir string) (string, int, int, float64, error) {
	f, err := os.Open(pagePath)
	if err != nil {
		return "", 0, 0, 0, err
	}
	defer f.Close()
	img, _, err := image.Decode(f)
	if err != nil {
		return "", 0, 0, 0, err
	}
	b := img.Bounds()
	x0 := b.Min.X + int(float64(b.Dx())*localOCRCropLeft)
	x1 := b.Min.X + int(float64(b.Dx())*localOCRCropRight)
	y0 := b.Min.Y + int(float64(b.Dy())*localOCRCropTop)
	y1 := b.Min.Y + int(float64(b.Dy())*localOCRCropBottom)
	rect := image.Rect(x0, y0, x1, y1).Intersect(b)
	if rect.Empty() {
		return "", 0, 0, 0, fmt.Errorf("empty right column crop")
	}
	dst := image.NewRGBA(image.Rect(0, 0, rect.Dx(), rect.Dy()))
	draw.Draw(dst, dst.Bounds(), img, rect.Min, draw.Src)
	nonBlankRatio := estimateNonBlankRatio(dst)
	cropPath := filepath.Join(outDir, "right_column.png")
	cf, err := os.Create(cropPath)
	if err != nil {
		return "", 0, 0, 0, err
	}
	defer cf.Close()
	if err := png.Encode(cf, dst); err != nil {
		return "", 0, 0, 0, err
	}
	return cropPath, rect.Dx(), rect.Dy(), nonBlankRatio, nil
}

func estimateNonBlankRatio(img image.Image) float64 {
	b := img.Bounds()
	stepX := max(1, b.Dx()/240)
	stepY := max(1, b.Dy()/240)
	total, nonBlank := 0, 0
	for y := b.Min.Y; y < b.Max.Y; y += stepY {
		for x := b.Min.X; x < b.Max.X; x += stepX {
			r, g, bl, a := img.At(x, y).RGBA()
			if a == 0 {
				continue
			}
			total++
			if r < 61000 || g < 61000 || bl < 61000 {
				nonBlank++
			}
		}
	}
	if total == 0 {
		return 0
	}
	return float64(nonBlank) / float64(total)
}

func runConfiguredOCRLines(ocrConfig localocr.Config, cropPath string, cropW, cropH, pageIndex int) ([]localLayoutLine, localocr.Result, error) {
	res, err := ocrConfig.RunLinesFile(context.Background(), cropPath, cropW, cropH)
	if err != nil {
		return nil, res, err
	}
	return ocrLinesToLocalLayout(res.Lines, cropW, cropH, pageIndex), res, nil
}

func parseTesseractTSV(tsv string, cropW, cropH, pageIndex int) []localLayoutLine {
	return ocrLinesToLocalLayout(localocr.ParseTesseractTSV(tsv, cropW, cropH), cropW, cropH, pageIndex)
}

func ocrLinesToLocalLayout(lines []localocr.Line, cropW, cropH, pageIndex int) []localLayoutLine {
	if len(lines) == 0 || cropW <= 0 || cropH <= 0 {
		return nil
	}
	out := make([]localLayoutLine, 0, len(lines))
	for i, line := range lines {
		text := strings.TrimSpace(line.Text)
		if text == "" || line.Right <= line.Left || line.Bottom <= line.Top {
			continue
		}
		left := localOCRCropLeft + (float64(line.Left)/float64(cropW))*(localOCRCropRight-localOCRCropLeft)
		right := localOCRCropLeft + (float64(line.Right)/float64(cropW))*(localOCRCropRight-localOCRCropLeft)
		top := localOCRCropTop + (float64(line.Top)/float64(cropH))*(localOCRCropBottom-localOCRCropTop)
		bottom := localOCRCropTop + (float64(line.Bottom)/float64(cropH))*(localOCRCropBottom-localOCRCropTop)
		out = append(out, localLayoutLine{
			PageIndex: pageIndex,
			Order:     i,
			Text:      text,
			X:         (left + right) / 2,
			Y:         1 - ((top + bottom) / 2),
			BBox: map[string]float64{
				"left": left, "right": right, "top": top, "bottom": bottom,
			},
		})
	}
	sortLocalLayoutLines(out)
	return out
}

func localSidebarOCREnabled() bool {
	raw := strings.ToLower(contentParseEnv("CONTENT_PARSE_LOCAL_SIDEBAR_OCR"))
	switch raw {
	case "0", "false", "no", "off", "disabled":
		return false
	default:
		return true
	}
}

func localSidebarOCRMaxPages() int {
	n, _ := strconv.Atoi(contentParseEnv("CONTENT_PARSE_LOCAL_SIDEBAR_OCR_MAX_PAGES"))
	if n <= 0 {
		return 6
	}
	if n > 32 {
		return 32
	}
	return n
}

func localSidebarOCRTimeout() time.Duration {
	n, _ := strconv.Atoi(contentParseEnv("CONTENT_PARSE_LOCAL_SIDEBAR_OCR_TIMEOUT_SECONDS"))
	if n <= 0 {
		n = 12
	}
	if n > 60 {
		n = 60
	}
	return time.Duration(n) * time.Second
}
