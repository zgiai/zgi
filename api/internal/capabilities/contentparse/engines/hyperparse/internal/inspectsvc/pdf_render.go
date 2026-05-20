package inspectsvc

import (
	"bytes"
	"encoding/base64"
	"fmt"
	"image"
	"image/jpeg"
	"image/png"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"golang.org/x/image/draw"

	pdfadapter "github.com/zgiai/ginext/internal/capabilities/contentparse/engines/hyperparse/internal/adapters/pdf"
	"github.com/zgiai/ginext/internal/capabilities/contentparse/engines/hyperparse/internal/binutil"

	_ "image/jpeg" // image.Decode
	_ "image/png"
)

type renderedPDFPageResult struct {
	RenderIndex int
	PageNumber  int
	DataURL     string
	Engine      string
	ElapsedMs   int64
	Err         error
}

type pdfRenderProfile struct {
	previewMaxSide int
	pdftoppmScale  int
}

type pageRenderScaleOverrides map[int]int

var previewPDFRenderProfile = pdfRenderProfile{
	previewMaxSide: 1536,
	pdftoppmScale:  1536,
}

// RenderPDFPagesToDataURLs renders PDF pages to PNG/JPEG data URLs for VLM use.
func RenderPDFPagesToDataURLs(pdfBytes []byte, maxPages int) ([]string, string, error) {
	urls, _, engine, err := renderPDFPagesToDataURLsWithSelectionAndProfile(pdfBytes, maxPages, nil, pdfRenderProfile{}, nil)
	return urls, engine, err
}

// RenderPDFPreviewPagesToDataURLs renders PDF pages for UI preview.
// Unlike VLM images, preview rendering first limits the long edge to avoid
// oversized canvas PDFs blocking the preview stage.
func RenderPDFPreviewPagesToDataURLs(pdfBytes []byte, maxPages int) ([]string, string, error) {
	urls, _, engine, err := renderPDFPagesToDataURLsWithSelectionAndProfile(pdfBytes, maxPages, nil, previewPDFRenderProfile, nil)
	return urls, engine, err
}

// RenderPDFSelectedPagesToDataURLs returns only the requested rendered pages, preserving page order.
func RenderPDFSelectedPagesToDataURLs(pdfBytes []byte, pageNumbers []int, maxPages int) ([]string, []int, string, error) {
	return RenderPDFSelectedPagesToDataURLsWithScales(pdfBytes, pageNumbers, maxPages, nil)
}

// StreamRenderPDFSelectedPagesToDataURLs renders selected pages incrementally and emits page results as soon as they are ready.
func StreamRenderPDFSelectedPagesToDataURLs(pdfBytes []byte, pageNumbers []int, maxPages int, concurrency int) (<-chan renderedPDFPageResult, []int, string, error) {
	return StreamRenderPDFSelectedPagesToDataURLsWithScales(pdfBytes, pageNumbers, maxPages, concurrency, nil)
}

func RenderPDFSelectedPagesToDataURLsWithScales(pdfBytes []byte, pageNumbers []int, maxPages int, scaleOverrides pageRenderScaleOverrides) ([]string, []int, string, error) {
	return renderPDFPagesToDataURLsWithSelectionAndProfile(pdfBytes, maxPages, pageNumbers, pdfRenderProfile{}, scaleOverrides)
}

// StreamRenderPDFSelectedPagesToDataURLsWithScales renders selected pages incrementally and applies optional per-page scale overrides.
func StreamRenderPDFSelectedPagesToDataURLsWithScales(pdfBytes []byte, pageNumbers []int, maxPages int, concurrency int, scaleOverrides pageRenderScaleOverrides) (<-chan renderedPDFPageResult, []int, string, error) {
	maxPages = detectRenderPageLimit(pdfBytes, maxPages)
	requestedPages, err := resolveRenderPages(maxPages, pageNumbers)
	if err != nil {
		return nil, nil, "", err
	}
	if len(requestedPages) == 0 {
		ch := make(chan renderedPDFPageResult)
		close(ch)
		return ch, nil, "", nil
	}
	tmpDir, err := os.MkdirTemp("", "hyperparse-vlm-pages-*")
	if err != nil {
		return nil, nil, "", fmt.Errorf("create temp dir: %w", err)
	}
	pdfPath := filepath.Join(tmpDir, "input.pdf")
	if err := os.WriteFile(pdfPath, pdfBytes, 0600); err != nil {
		_ = os.RemoveAll(tmpDir)
		return nil, nil, "", fmt.Errorf("write temp pdf: %w", err)
	}

	type singlePageRenderer struct {
		engine string
		render func(pdfPath, outDir string, page int, scaleTo int) (string, error)
	}
	renderers := []singlePageRenderer{
		{engine: "pdftoppm", render: renderWithPDFToPPMSinglePage},
		{engine: "mutool", render: renderWithMuToolSinglePage},
		{engine: "magick", render: renderWithImageMagickSinglePage},
	}

	firstIndex := 0
	firstPage := requestedPages[firstIndex]
	var renderer singlePageRenderer
	var firstDataURL string
	var firstRenderElapsedMs int64
	var errs []string
	for _, candidate := range renderers {
		renderStartedAt := time.Now()
		path, renderErr := candidate.render(pdfPath, tmpDir, firstPage, scaleOverrideForPage(scaleOverrides, firstPage))
		renderElapsedMs := time.Since(renderStartedAt).Milliseconds()
		if renderErr != nil {
			errs = append(errs, candidate.engine+"="+renderErr.Error())
			continue
		}
		firstDataURL, renderErr = imagePathToDataURL(path)
		if renderErr != nil {
			errs = append(errs, candidate.engine+"="+renderErr.Error())
			continue
		}
		firstRenderElapsedMs = renderElapsedMs
		renderer = candidate
		break
	}
	if renderer.engine == "" {
		_ = os.RemoveAll(tmpDir)
		if len(errs) > 0 {
			log.Printf("[ui.pdf.render] all streaming engines failed: %s", strings.Join(errs, " | "))
			return nil, nil, "", fmt.Errorf("PDF rendering failed: %s", strings.Join(errs, " | "))
		}
		return nil, nil, "", fmt.Errorf("no available PDF rendering tool found; install pdftoppm, mutool, or ImageMagick")
	}

	if concurrency <= 0 {
		concurrency = 1
	}
	if concurrency > len(requestedPages) {
		concurrency = len(requestedPages)
	}

	out := make(chan renderedPDFPageResult, len(requestedPages))
	go func() {
		defer close(out)
		defer os.RemoveAll(tmpDir)

		out <- renderedPDFPageResult{
			RenderIndex: firstIndex,
			PageNumber:  firstPage,
			DataURL:     firstDataURL,
			Engine:      renderer.engine,
			ElapsedMs:   firstRenderElapsedMs,
		}

		if len(requestedPages) == 1 {
			return
		}

		type renderJob struct {
			RenderIndex int
			PageNumber  int
		}
		jobs := make(chan renderJob)
		var wg sync.WaitGroup
		for workerIdx := 0; workerIdx < concurrency; workerIdx++ {
			wg.Add(1)
			go func() {
				defer wg.Done()
				for job := range jobs {
					renderStartedAt := time.Now()
					path, renderErr := renderer.render(pdfPath, tmpDir, job.PageNumber, scaleOverrideForPage(scaleOverrides, job.PageNumber))
					renderElapsedMs := time.Since(renderStartedAt).Milliseconds()
					if renderErr != nil {
						out <- renderedPDFPageResult{
							RenderIndex: job.RenderIndex,
							PageNumber:  job.PageNumber,
							Engine:      renderer.engine,
							ElapsedMs:   renderElapsedMs,
							Err:         fmt.Errorf("render page %d: %w", job.PageNumber, renderErr),
						}
						continue
					}
					dataURL, dataErr := imagePathToDataURL(path)
					if dataErr != nil {
						out <- renderedPDFPageResult{
							RenderIndex: job.RenderIndex,
							PageNumber:  job.PageNumber,
							Engine:      renderer.engine,
							ElapsedMs:   renderElapsedMs,
							Err:         fmt.Errorf("render page %d: %w", job.PageNumber, dataErr),
						}
						continue
					}
					out <- renderedPDFPageResult{
						RenderIndex: job.RenderIndex,
						PageNumber:  job.PageNumber,
						DataURL:     dataURL,
						Engine:      renderer.engine,
						ElapsedMs:   renderElapsedMs,
					}
				}
			}()
		}
		for idx := 1; idx < len(requestedPages); idx++ {
			jobs <- renderJob{RenderIndex: idx, PageNumber: requestedPages[idx]}
		}
		close(jobs)
		wg.Wait()
	}()

	return out, requestedPages, renderer.engine, nil
}

func renderPDFPagesToDataURLsWithSelection(pdfBytes []byte, maxPages int, pageNumbers []int) ([]string, []int, string, error) {
	return renderPDFPagesToDataURLsWithSelectionAndProfile(pdfBytes, maxPages, pageNumbers, pdfRenderProfile{}, nil)
}

func renderPDFPagesToDataURLsWithSelectionAndProfile(pdfBytes []byte, maxPages int, pageNumbers []int, profile pdfRenderProfile, scaleOverrides pageRenderScaleOverrides) ([]string, []int, string, error) {
	maxPages = detectRenderPageLimit(pdfBytes, maxPages)
	requestedPages, err := resolveRenderPages(maxPages, pageNumbers)
	if err != nil {
		return nil, nil, "", err
	}
	tmpDir, err := os.MkdirTemp("", "hyperparse-vlm-pages-*")
	if err != nil {
		return nil, nil, "", fmt.Errorf("create temp dir: %w", err)
	}
	defer os.RemoveAll(tmpDir)
	pdfPath := filepath.Join(tmpDir, "input.pdf")
	if err := os.WriteFile(pdfPath, pdfBytes, 0600); err != nil {
		return nil, nil, "", fmt.Errorf("write temp pdf: %w", err)
	}

	useSparse := (len(pageNumbers) > 0 && !pageListIsSequential(requestedPages, maxPages)) || len(scaleOverrides) > 0
	var errs []string
	if paths, e := renderWithPDFToPPMForPages(pdfPath, tmpDir, requestedPages, useSparse, profile, scaleOverrides); e == nil && len(paths) > 0 {
		urls, err := imagePathsToDataURLsWithMaxSide(paths, profile.previewMaxSide)
		return urls, requestedPages, "pdftoppm", err
	} else if e != nil {
		errs = append(errs, "pdftoppm="+e.Error())
	}
	if paths, e := renderWithMuToolForPages(pdfPath, tmpDir, requestedPages, useSparse); e == nil && len(paths) > 0 {
		urls, err := imagePathsToDataURLsWithMaxSide(paths, profile.previewMaxSide)
		return urls, requestedPages, "mutool", err
	} else if e != nil {
		errs = append(errs, "mutool="+e.Error())
	}
	if paths, e := renderWithImageMagickForPages(pdfPath, tmpDir, requestedPages, useSparse); e == nil && len(paths) > 0 {
		urls, err := imagePathsToDataURLsWithMaxSide(paths, profile.previewMaxSide)
		return urls, requestedPages, "magick", err
	} else if e != nil {
		errs = append(errs, "magick="+e.Error())
	}
	if len(errs) > 0 {
		log.Printf("[ui.pdf.render] all engines failed: %s", strings.Join(errs, " | "))
		return nil, nil, "", fmt.Errorf("PDF rendering failed: %s", strings.Join(errs, " | "))
	}
	return nil, nil, "", fmt.Errorf("no available PDF rendering tool found; install pdftoppm, mutool, or ImageMagick")
}

func detectRenderPageLimit(pdfBytes []byte, maxPages int) int {
	if maxPages > 0 {
		return maxPages
	}
	if info, err := pdfadapter.InspectBasicBytes(pdfBytes, "relaxed"); err == nil && info.PageCount > 0 {
		return info.PageCount
	}
	if pis, err := pdfadapter.DetectPageInfosBytes(pdfBytes, "relaxed"); err == nil && len(pis) > 0 {
		return len(pis)
	}
	return 256
}

func resolveRenderPages(maxPages int, pageNumbers []int) ([]int, error) {
	if maxPages <= 0 {
		return nil, fmt.Errorf("invalid render page limit")
	}
	if len(pageNumbers) == 0 {
		return sequentialProcessedPages(maxPages), nil
	}
	pages := limitPages(pageNumbers, maxPages)
	if len(pages) == 0 {
		return nil, fmt.Errorf("candidate pages exceed render range")
	}
	return pages, nil
}

func pageListIsSequential(pages []int, maxPages int) bool {
	if len(pages) == 0 {
		return maxPages <= 0
	}
	if len(pages) != maxPages {
		return false
	}
	for i, page := range pages {
		if page != i+1 {
			return false
		}
	}
	return true
}

func renderWithPDFToPPM(pdfPath, outDir string, maxPages int, scaleTo int) ([]string, error) {
	bin, err := resolveRendererBinary("pdftoppm", "CONTENT_PARSE_PDFTOPPM_PATH")
	if err != nil {
		return nil, err
	}
	prefix := filepath.Join(outDir, "ppm_page")
	args := []string{"-png"}
	if scaleTo > 0 {
		args = append(args, "-scale-to", strconv.Itoa(scaleTo))
	}
	args = append(args, "-f", "1", "-l", strconv.Itoa(maxPages), pdfPath, prefix)
	cmd := exec.Command(bin, args...)
	if b, err := cmd.CombinedOutput(); err != nil {
		return nil, fmt.Errorf("pdftoppm failed: %v (%s)", err, strings.TrimSpace(string(b)))
	}
	paths, _ := filepath.Glob(filepath.Join(outDir, "ppm_page-*.png"))
	if len(paths) == 0 {
		paths, _ = filepath.Glob(filepath.Join(outDir, "ppm_page_*.png"))
	}
	sort.Strings(paths)
	if len(paths) == 0 {
		return nil, fmt.Errorf("pdftoppm rendered 0 pages")
	}
	return paths, nil
}

func renderWithPDFToPPMSelected(pdfPath, outDir string, pageNumbers []int, scaleTo int) ([]string, error) {
	bin, err := resolveRendererBinary("pdftoppm", "CONTENT_PARSE_PDFTOPPM_PATH")
	if err != nil {
		return nil, err
	}
	paths := make([]string, 0, len(pageNumbers))
	for _, page := range pageNumbers {
		prefix := filepath.Join(outDir, fmt.Sprintf("ppm_page_%03d", page))
		args := []string{"-png"}
		if scaleTo > 0 {
			args = append(args, "-scale-to", strconv.Itoa(scaleTo))
		}
		args = append(args, "-singlefile", "-f", strconv.Itoa(page), "-l", strconv.Itoa(page), pdfPath, prefix)
		cmd := exec.Command(bin, args...)
		if b, err := cmd.CombinedOutput(); err != nil {
			return nil, fmt.Errorf("pdftoppm page %d failed: %v (%s)", page, err, strings.TrimSpace(string(b)))
		}
		path, err := resolveSingleRenderedPath(prefix+".png", prefix+"*.png")
		if err != nil {
			return nil, fmt.Errorf("pdftoppm page %d: %w", page, err)
		}
		paths = append(paths, path)
	}
	return paths, nil
}

func renderWithPDFToPPMSinglePage(pdfPath, outDir string, page int, scaleTo int) (string, error) {
	bin, err := resolveRendererBinary("pdftoppm", "CONTENT_PARSE_PDFTOPPM_PATH")
	if err != nil {
		return "", err
	}
	prefix := filepath.Join(outDir, fmt.Sprintf("ppm_page_%03d", page))
	args := []string{"-png"}
	if scaleTo > 0 {
		args = append(args, "-scale-to", strconv.Itoa(scaleTo))
	}
	args = append(args, "-singlefile", "-f", strconv.Itoa(page), "-l", strconv.Itoa(page), pdfPath, prefix)
	cmd := exec.Command(bin, args...)
	if b, err := cmd.CombinedOutput(); err != nil {
		return "", fmt.Errorf("pdftoppm page %d failed: %v (%s)", page, err, strings.TrimSpace(string(b)))
	}
	return resolveSingleRenderedPath(prefix+".png", prefix+"*.png")
}

func renderWithPDFToPPMForPages(pdfPath, outDir string, pageNumbers []int, sparse bool, profile pdfRenderProfile, scaleOverrides pageRenderScaleOverrides) ([]string, error) {
	if sparse {
		return renderWithPDFToPPMSelectedWithScales(pdfPath, outDir, pageNumbers, profile.pdftoppmScale, scaleOverrides)
	}
	return renderWithPDFToPPM(pdfPath, outDir, len(pageNumbers), profile.pdftoppmScale)
}

func renderWithPDFToPPMSelectedWithScales(pdfPath, outDir string, pageNumbers []int, defaultScale int, scaleOverrides pageRenderScaleOverrides) ([]string, error) {
	bin, err := resolveRendererBinary("pdftoppm", "CONTENT_PARSE_PDFTOPPM_PATH")
	if err != nil {
		return nil, err
	}
	paths := make([]string, 0, len(pageNumbers))
	for _, page := range pageNumbers {
		prefix := filepath.Join(outDir, fmt.Sprintf("ppm_page_%03d", page))
		args := []string{"-png"}
		scaleTo := scaleOverrideForPage(scaleOverrides, page)
		if scaleTo <= 0 {
			scaleTo = defaultScale
		}
		if scaleTo > 0 {
			args = append(args, "-scale-to", strconv.Itoa(scaleTo))
		}
		args = append(args, "-singlefile", "-f", strconv.Itoa(page), "-l", strconv.Itoa(page), pdfPath, prefix)
		cmd := exec.Command(bin, args...)
		if b, err := cmd.CombinedOutput(); err != nil {
			return nil, fmt.Errorf("pdftoppm page %d failed: %v (%s)", page, err, strings.TrimSpace(string(b)))
		}
		path, err := resolveSingleRenderedPath(prefix+".png", prefix+"*.png")
		if err != nil {
			return nil, fmt.Errorf("pdftoppm page %d: %w", page, err)
		}
		paths = append(paths, path)
	}
	return paths, nil
}

func renderWithMuTool(pdfPath, outDir string, maxPages int) ([]string, error) {
	bin, err := resolveRendererBinary("mutool", "CONTENT_PARSE_MUTOOL_PATH")
	if err != nil {
		return nil, err
	}
	outPattern := filepath.Join(outDir, "mutool_%03d.png")
	args := []string{"draw", "-F", "png", "-o", outPattern, pdfPath, fmt.Sprintf("1-%d", maxPages)}
	cmd := exec.Command(bin, args...)
	if b, err := cmd.CombinedOutput(); err != nil {
		return nil, fmt.Errorf("mutool draw failed: %v (%s)", err, strings.TrimSpace(string(b)))
	}
	paths, _ := filepath.Glob(filepath.Join(outDir, "mutool_*.png"))
	sort.Strings(paths)
	return paths, nil
}

func renderWithMuToolSelected(pdfPath, outDir string, pageNumbers []int) ([]string, error) {
	bin, err := resolveRendererBinary("mutool", "CONTENT_PARSE_MUTOOL_PATH")
	if err != nil {
		return nil, err
	}
	paths := make([]string, 0, len(pageNumbers))
	for _, page := range pageNumbers {
		outPath := filepath.Join(outDir, fmt.Sprintf("mutool_%03d.png", page))
		args := []string{"draw", "-F", "png", "-o", outPath, pdfPath, strconv.Itoa(page)}
		cmd := exec.Command(bin, args...)
		if b, err := cmd.CombinedOutput(); err != nil {
			return nil, fmt.Errorf("mutool page %d failed: %v (%s)", page, err, strings.TrimSpace(string(b)))
		}
		path, err := resolveSingleRenderedPath(outPath, strings.TrimSuffix(outPath, ".png")+"*.png")
		if err != nil {
			return nil, fmt.Errorf("mutool page %d: %w", page, err)
		}
		paths = append(paths, path)
	}
	return paths, nil
}

func renderWithMuToolSinglePage(pdfPath, outDir string, page int, _ int) (string, error) {
	bin, err := resolveRendererBinary("mutool", "CONTENT_PARSE_MUTOOL_PATH")
	if err != nil {
		return "", err
	}
	outPath := filepath.Join(outDir, fmt.Sprintf("mutool_%03d.png", page))
	args := []string{"draw", "-F", "png", "-o", outPath, pdfPath, strconv.Itoa(page)}
	cmd := exec.Command(bin, args...)
	if b, err := cmd.CombinedOutput(); err != nil {
		return "", fmt.Errorf("mutool page %d failed: %v (%s)", page, err, strings.TrimSpace(string(b)))
	}
	return resolveSingleRenderedPath(outPath, strings.TrimSuffix(outPath, ".png")+"*.png")
}

func renderWithMuToolForPages(pdfPath, outDir string, pageNumbers []int, sparse bool) ([]string, error) {
	if sparse {
		return renderWithMuToolSelected(pdfPath, outDir, pageNumbers)
	}
	return renderWithMuTool(pdfPath, outDir, len(pageNumbers))
}

func renderWithImageMagick(pdfPath, outDir string, maxPages int) ([]string, error) {
	bin, err := resolveRendererBinary("magick", "CONTENT_PARSE_MAGICK_PATH")
	if err != nil {
		return nil, err
	}
	outPattern := filepath.Join(outDir, "magick_%03d.png")
	spec := fmt.Sprintf("%s[0-%d]", pdfPath, maxPages-1)
	cmd := exec.Command(bin, "-density", "200", spec, outPattern)
	if b, err := cmd.CombinedOutput(); err != nil {
		return nil, fmt.Errorf("magick failed: %v (%s)", err, strings.TrimSpace(string(b)))
	}
	paths, _ := filepath.Glob(filepath.Join(outDir, "magick_*.png"))
	sort.Strings(paths)
	return paths, nil
}

func renderWithImageMagickSelected(pdfPath, outDir string, pageNumbers []int) ([]string, error) {
	bin, err := resolveRendererBinary("magick", "CONTENT_PARSE_MAGICK_PATH")
	if err != nil {
		return nil, err
	}
	paths := make([]string, 0, len(pageNumbers))
	for _, page := range pageNumbers {
		outPath := filepath.Join(outDir, fmt.Sprintf("magick_%03d.png", page))
		spec := fmt.Sprintf("%s[%d]", pdfPath, page-1)
		cmd := exec.Command(bin, "-density", "200", spec, outPath)
		if b, err := cmd.CombinedOutput(); err != nil {
			return nil, fmt.Errorf("magick page %d failed: %v (%s)", page, err, strings.TrimSpace(string(b)))
		}
		path, err := resolveSingleRenderedPath(outPath, strings.TrimSuffix(outPath, ".png")+"*.png")
		if err != nil {
			return nil, fmt.Errorf("magick page %d: %w", page, err)
		}
		paths = append(paths, path)
	}
	return paths, nil
}

func renderWithImageMagickSinglePage(pdfPath, outDir string, page int, _ int) (string, error) {
	bin, err := resolveRendererBinary("magick", "CONTENT_PARSE_MAGICK_PATH")
	if err != nil {
		return "", err
	}
	outPath := filepath.Join(outDir, fmt.Sprintf("magick_%03d.png", page))
	spec := fmt.Sprintf("%s[%d]", pdfPath, page-1)
	cmd := exec.Command(bin, "-density", "200", spec, outPath)
	if b, err := cmd.CombinedOutput(); err != nil {
		return "", fmt.Errorf("magick page %d failed: %v (%s)", page, err, strings.TrimSpace(string(b)))
	}
	return resolveSingleRenderedPath(outPath, strings.TrimSuffix(outPath, ".png")+"*.png")
}

func renderWithImageMagickForPages(pdfPath, outDir string, pageNumbers []int, sparse bool) ([]string, error) {
	if sparse {
		return renderWithImageMagickSelected(pdfPath, outDir, pageNumbers)
	}
	return renderWithImageMagick(pdfPath, outDir, len(pageNumbers))
}

func resolveSingleRenderedPath(exactPath string, globPattern string) (string, error) {
	if _, err := os.Stat(exactPath); err == nil {
		return exactPath, nil
	}
	paths, _ := filepath.Glob(globPattern)
	sort.Strings(paths)
	if len(paths) == 0 {
		return "", fmt.Errorf("rendered 0 pages")
	}
	return paths[0], nil
}

func resolveRendererBinary(name string, envKey string) (string, error) {
	return binutil.Resolve(name, contentParseEnv(envKey))
}

const dashscopeMaxDataURIItemBytes = 10485760

func dataURLByteLen(mime string, raw []byte) int {
	return len("data:") + len(mime) + len(";base64,") + base64.StdEncoding.EncodedLen(len(raw))
}

func scaleImageMaxSide(img image.Image, maxSide int) image.Image {
	b := img.Bounds()
	w, h := b.Dx(), b.Dy()
	if w <= maxSide && h <= maxSide {
		return img
	}
	var nw, nh int
	if w >= h {
		nw = maxSide
		nh = max(1, h*maxSide/w)
	} else {
		nh = maxSide
		nw = max(1, w*maxSide/h)
	}
	dst := image.NewRGBA(image.Rect(0, 0, nw, nh))
	draw.ApproxBiLinear.Scale(dst, dst.Bounds(), img, b, draw.Over, nil)
	return dst
}

// ClampImageBytesForVLM compresses an image below the provider data URI item limit.
// The input must decode as standard JPEG/PNG; raw pixels must not be mislabeled as PNG.
func ClampImageBytesForVLM(b []byte) ([]byte, string, error) {
	img, format, err := image.Decode(bytes.NewReader(b))
	if err != nil {
		return nil, "", fmt.Errorf("failed to decode as JPEG/PNG: %w", err)
	}
	if format == "jpeg" && dataURLByteLen("image/jpeg", b) <= dashscopeMaxDataURIItemBytes {
		return b, "image/jpeg", nil
	}
	if format == "png" && dataURLByteLen("image/png", b) <= dashscopeMaxDataURIItemBytes {
		return b, "image/png", nil
	}
	maxSides := []int{4096, 3072, 2560, 2048, 1600, 1280, 1024, 896, 768, 640, 512, 400, 320}
	for _, side := range maxSides {
		simg := scaleImageMaxSide(img, side)
		var buf bytes.Buffer
		if err := png.Encode(&buf, simg); err != nil {
			continue
		}
		raw := buf.Bytes()
		if dataURLByteLen("image/png", raw) <= dashscopeMaxDataURIItemBytes {
			return raw, "image/png", nil
		}
	}
	for q := 88; q >= 28; q -= 10 {
		for _, side := range maxSides {
			simg := scaleImageMaxSide(img, side)
			var buf bytes.Buffer
			if err := jpeg.Encode(&buf, simg, &jpeg.Options{Quality: q}); err != nil {
				continue
			}
			raw := buf.Bytes()
			if dataURLByteLen("image/jpeg", raw) <= dashscopeMaxDataURIItemBytes {
				return raw, "image/jpeg", nil
			}
		}
	}
	return nil, "", fmt.Errorf("failed to compress one page image under the data URI limit (%d bytes) while preserving recognizability", dashscopeMaxDataURIItemBytes)
}

func imagePathsToDataURLs(paths []string) ([]string, error) {
	return imagePathsToDataURLsWithMaxSide(paths, 0)
}

func imagePathsToDataURLsWithMaxSide(paths []string, maxSide int) ([]string, error) {
	out := make([]string, 0, len(paths))
	for _, p := range paths {
		dataURL, err := imagePathToDataURLWithMaxSide(p, maxSide)
		if err != nil {
			return nil, err
		}
		out = append(out, dataURL)
	}
	return out, nil
}

func imagePathToDataURL(path string) (string, error) {
	return imagePathToDataURLWithMaxSide(path, 0)
}

func imagePathToDataURLWithMaxSide(path string, maxSide int) (string, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return "", fmt.Errorf("read rendered image %s: %w", filepath.Base(path), err)
	}
	var mime string
	if maxSide > 0 {
		b, mime, err = ClampImageBytesForPreview(b, maxSide)
	} else {
		b, mime, err = ClampImageBytesForVLM(b)
	}
	if err != nil {
		return "", fmt.Errorf("%s: %w", filepath.Base(path), err)
	}
	return "data:" + mime + ";base64," + base64.StdEncoding.EncodeToString(b), nil
}

// ClampImageBytesForPreview resizes an image for UI preview and limits time/memory use.
func ClampImageBytesForPreview(b []byte, maxSide int) ([]byte, string, error) {
	img, format, err := image.Decode(bytes.NewReader(b))
	if err != nil {
		return nil, "", fmt.Errorf("failed to decode as JPEG/PNG: %w", err)
	}
	if maxSide > 0 {
		img = scaleImageMaxSide(img, maxSide)
	}
	if format == "jpeg" {
		var buf bytes.Buffer
		if err := jpeg.Encode(&buf, img, &jpeg.Options{Quality: 86}); err != nil {
			return nil, "", err
		}
		return buf.Bytes(), "image/jpeg", nil
	}
	var buf bytes.Buffer
	if err := png.Encode(&buf, img); err != nil {
		return nil, "", err
	}
	return buf.Bytes(), "image/png", nil
}

func scaleOverrideForPage(scaleOverrides pageRenderScaleOverrides, page int) int {
	if len(scaleOverrides) == 0 {
		return 0
	}
	return scaleOverrides[page]
}
