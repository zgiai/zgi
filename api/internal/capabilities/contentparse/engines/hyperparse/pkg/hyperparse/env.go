package hyperparse

import (
	"os"
	"strconv"
	"strings"
)

// ApplyEnviron writes non-empty Config fields into process environment variables.
// CONTENT_PARSE_* is the public naming surface; DOCSTILL_* is still written as a
// backward-compatible alias for older internal runtime readers.
func (c *Config) ApplyEnviron() {
	setIf := func(key, val string) {
		val = strings.TrimSpace(val)
		if val == "" {
			return
		}
		if os.Getenv(key) != "" {
			return
		}
		_ = os.Setenv(key, val)
	}
	setIfInt := func(key string, n int) {
		if os.Getenv(key) != "" {
			return
		}
		_ = os.Setenv(key, strconv.Itoa(n))
	}
	setIfCompat := func(primary, legacy, val string) {
		setIf(primary, val)
		setIf(legacy, val)
	}
	setIfIntCompat := func(primary, legacy string, n int) {
		setIfInt(primary, n)
		setIfInt(legacy, n)
	}
	setBoolCompat := func(primary, legacy string, val *bool) {
		if val == nil {
			return
		}
		out := "0"
		if *val {
			out = "1"
		}
		setIfCompat(primary, legacy, out)
	}

	v := c.Vision
	setIf("VLM_API_KEY", v.APIKey)
	setIf("VLM_BASE_URL", v.BaseURL)
	setIf("VLM_MODEL", v.Model)
	setIf("VLM_MODEL_FAST", v.ModelFast)

	vlm := c.VLM
	if vlm.FallbackMaxPages != nil {
		setIfIntCompat("CONTENT_PARSE_VLM_FALLBACK_MAX_PAGES", "DOCSTILL_VLM_FALLBACK_MAX_PAGES", *vlm.FallbackMaxPages)
	}
	if vlm.FallbackBatchPages > 0 {
		setIfIntCompat("CONTENT_PARSE_VLM_FALLBACK_BATCH_PAGES", "DOCSTILL_VLM_FALLBACK_BATCH_PAGES", vlm.FallbackBatchPages)
	}
	if vlm.FallbackConcurrency > 0 {
		setIfIntCompat("CONTENT_PARSE_VLM_FALLBACK_CONCURRENCY", "DOCSTILL_VLM_FALLBACK_CONCURRENCY", vlm.FallbackConcurrency)
	}
	if vlm.FallbackFastFirstPages > 0 {
		setIfIntCompat("CONTENT_PARSE_VLM_FALLBACK_FAST_FIRST_PAGES", "DOCSTILL_VLM_FALLBACK_FAST_FIRST_PAGES", vlm.FallbackFastFirstPages)
	}
	setBoolCompat("CONTENT_PARSE_VLM_FULL_PAGE_FALLBACK", "DOCSTILL_VLM_FULL_PAGE_FALLBACK", vlm.FullPageFallback)
	setBoolCompat("CONTENT_PARSE_VLM_IMAGE_CAPTION", "DOCSTILL_VLM_IMAGE_CAPTION", vlm.ImageCaptionEnabled)
	if vlm.ImageCaptionMax > 0 {
		setIfIntCompat("CONTENT_PARSE_VLM_IMAGE_CAPTION_MAX", "DOCSTILL_VLM_IMAGE_CAPTION_MAX", vlm.ImageCaptionMax)
	}
	if vlm.ImageCaptionConcurrency > 0 {
		setIfIntCompat("CONTENT_PARSE_VLM_IMAGE_CAPTION_CONCURRENCY", "DOCSTILL_VLM_IMAGE_CAPTION_CONCURRENCY", vlm.ImageCaptionConcurrency)
	}
	setBoolCompat("CONTENT_PARSE_FORCE_VLM", "DOCSTILL_FORCE_VLM", vlm.ForceVLM)

	ui := c.UI
	setBoolCompat("CONTENT_PARSE_UI_INSPECT_CACHE", "DOCSTILL_UI_INSPECT_CACHE", ui.InspectCache)
	if ui.InspectCacheMax > 0 {
		setIfIntCompat("CONTENT_PARSE_UI_INSPECT_CACHE_MAX", "DOCSTILL_UI_INSPECT_CACHE_MAX", ui.InspectCacheMax)
	}
	setIfCompat("CONTENT_PARSE_UI_INSPECT_DEBUG_DUMP_DIR", "DOCSTILL_UI_INSPECT_DEBUG_DUMP_DIR", ui.InspectDebugDumpDir)
	if ui.PreviewMaxPages > 0 {
		setIfIntCompat("CONTENT_PARSE_UI_PREVIEW_MAX_PAGES", "DOCSTILL_UI_PREVIEW_MAX_PAGES", ui.PreviewMaxPages)
	}

	pdf := c.PDF
	setIfCompat("CONTENT_PARSE_PDFTOPPM_PATH", "DOCSTILL_PDFTOPPM_PATH", pdf.PdftoppmPath)
	setIfCompat("CONTENT_PARSE_MUTOOL_PATH", "DOCSTILL_MUTOOL_PATH", pdf.MutoolPath)
	setIfCompat("CONTENT_PARSE_MAGICK_PATH", "DOCSTILL_MAGICK_PATH", pdf.MagickPath)
	if pdf.RenderConcurrency > 0 {
		setIfIntCompat("CONTENT_PARSE_PDF_RENDER_CONCURRENCY", "DOCSTILL_PDF_RENDER_CONCURRENCY", pdf.RenderConcurrency)
	}

	ocr := c.OCR
	setIfCompat("CONTENT_PARSE_OCR_ENGINE", "DOCSTILL_OCR_ENGINE", ocr.Engine)
	setIfCompat("CONTENT_PARSE_OCR_LANG", "DOCSTILL_OCR_LANG", ocr.Lang)
	if ocr.TimeoutSeconds > 0 {
		setIfIntCompat("CONTENT_PARSE_OCR_TIMEOUT_SECONDS", "DOCSTILL_OCR_TIMEOUT_SECONDS", ocr.TimeoutSeconds)
	}
	if ocr.Concurrency > 0 {
		setIfIntCompat("CONTENT_PARSE_LOCAL_OCR_CONCURRENCY", "DOCSTILL_LOCAL_OCR_CONCURRENCY", ocr.Concurrency)
	}
	setIfCompat("CONTENT_PARSE_TESSERACT_PATH", "DOCSTILL_TESSERACT_PATH", ocr.TesseractPath)
	setIfCompat("CONTENT_PARSE_PADDLEOCR_CMD", "DOCSTILL_PADDLEOCR_CMD", ocr.PaddleCommand)
	setIfCompat("CONTENT_PARSE_PADDLEOCR_ARGS", "DOCSTILL_PADDLEOCR_ARGS", ocr.PaddleArgs)
}
