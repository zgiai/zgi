package hyperparse

import (
	"encoding/json"
	"fmt"
	"os"
)

// Config describes process-level runtime knobs. It can be loaded from JSON and
// applied to environment variables via ApplyEnviron.
type Config struct {
	// Vision configures an OpenAI Chat Completions-compatible multimodal endpoint.
	Vision VisionConfig `json:"vision,omitempty"`

	// VLM controls full-page fallback and image captioning runtime limits.
	VLM VLMRuntimeConfig `json:"vlm,omitempty"`

	// UI controls playground/debug cache and preview behavior.
	UI UIConfig `json:"ui,omitempty"`

	// PDF configures external rasterization tools. Absolute paths are recommended on Windows.
	PDF PDFRenderConfig `json:"pdf,omitempty"`

	// OCR configures local repair engines such as Tesseract or PaddleOCR.
	OCR OCRConfig `json:"ocr,omitempty"`
}

// VisionConfig describes any compatible multimodal chat-completions endpoint.
type VisionConfig struct {
	// Provider is only an ops/documentation label and does not affect branching.
	Provider string `json:"provider,omitempty"`
	// APIKey maps to VLM_API_KEY.
	APIKey string `json:"api_key,omitempty"`
	// BaseURL maps to VLM_BASE_URL.
	BaseURL string `json:"base_url,omitempty"`
	// Model maps to VLM_MODEL.
	Model string `json:"model,omitempty"`
	// ModelFast maps to VLM_MODEL_FAST.
	ModelFast string `json:"model_fast,omitempty"`
}

// VLMRuntimeConfig controls full-page fallback batching and image caption limits.
type VLMRuntimeConfig struct {
	// FallbackMaxPages maps to CONTENT_PARSE_VLM_FALLBACK_MAX_PAGES.
	// Nil means do not write the env var; 0 explicitly means unlimited.
	FallbackMaxPages *int `json:"fallback_max_pages,omitempty"`
	// FallbackBatchPages maps to CONTENT_PARSE_VLM_FALLBACK_BATCH_PAGES.
	FallbackBatchPages int `json:"fallback_batch_pages,omitempty"`
	// FallbackConcurrency maps to CONTENT_PARSE_VLM_FALLBACK_CONCURRENCY.
	FallbackConcurrency int `json:"fallback_concurrency,omitempty"`
	// FallbackFastFirstPages maps to CONTENT_PARSE_VLM_FALLBACK_FAST_FIRST_PAGES.
	FallbackFastFirstPages int `json:"fallback_fast_first_pages,omitempty"`
	// FullPageFallback maps to CONTENT_PARSE_VLM_FULL_PAGE_FALLBACK.
	FullPageFallback *bool `json:"full_page_fallback,omitempty"`
	// ImageCaptionEnabled maps to CONTENT_PARSE_VLM_IMAGE_CAPTION.
	ImageCaptionEnabled *bool `json:"image_caption_enabled,omitempty"`
	// ImageCaptionMax maps to CONTENT_PARSE_VLM_IMAGE_CAPTION_MAX.
	ImageCaptionMax int `json:"image_caption_max,omitempty"`
	// ImageCaptionConcurrency maps to CONTENT_PARSE_VLM_IMAGE_CAPTION_CONCURRENCY.
	ImageCaptionConcurrency int `json:"image_caption_concurrency,omitempty"`
	// ForceVLM maps to CONTENT_PARSE_FORCE_VLM.
	ForceVLM *bool `json:"force_vlm,omitempty"`
}

// UIConfig controls playground/debug behavior.
type UIConfig struct {
	// InspectCache maps to CONTENT_PARSE_UI_INSPECT_CACHE.
	InspectCache *bool `json:"inspect_cache,omitempty"`
	// InspectCacheMax maps to CONTENT_PARSE_UI_INSPECT_CACHE_MAX.
	InspectCacheMax int `json:"inspect_cache_max,omitempty"`
	// InspectDebugDumpDir maps to CONTENT_PARSE_UI_INSPECT_DEBUG_DUMP_DIR.
	InspectDebugDumpDir string `json:"inspect_debug_dump_dir,omitempty"`
	// PreviewMaxPages maps to CONTENT_PARSE_UI_PREVIEW_MAX_PAGES. Zero means unlimited.
	PreviewMaxPages int `json:"preview_max_pages,omitempty"`
}

// PDFRenderConfig contains external renderer executable paths.
type PDFRenderConfig struct {
	// PdftoppmPath maps to CONTENT_PARSE_PDFTOPPM_PATH.
	PdftoppmPath string `json:"pdftoppm_path,omitempty"`
	// MutoolPath maps to CONTENT_PARSE_MUTOOL_PATH.
	MutoolPath string `json:"mutool_path,omitempty"`
	// MagickPath maps to CONTENT_PARSE_MAGICK_PATH.
	MagickPath string `json:"magick_path,omitempty"`
	// RenderConcurrency maps to CONTENT_PARSE_PDF_RENDER_CONCURRENCY.
	RenderConcurrency int `json:"render_concurrency,omitempty"`
}

// OCRConfig describes the local OCR repair engine.
type OCRConfig struct {
	// Engine maps to CONTENT_PARSE_OCR_ENGINE. Supported values: tesseract / paddleocr.
	Engine string `json:"engine,omitempty"`
	// Lang maps to CONTENT_PARSE_OCR_LANG.
	Lang string `json:"lang,omitempty"`
	// TimeoutSeconds maps to CONTENT_PARSE_OCR_TIMEOUT_SECONDS.
	TimeoutSeconds int `json:"timeout_seconds,omitempty"`
	// Concurrency maps to CONTENT_PARSE_LOCAL_OCR_CONCURRENCY.
	Concurrency int `json:"concurrency,omitempty"`
	// TesseractPath maps to CONTENT_PARSE_TESSERACT_PATH.
	TesseractPath string `json:"tesseract_path,omitempty"`
	// PaddleCommand maps to CONTENT_PARSE_PADDLEOCR_CMD.
	PaddleCommand string `json:"paddleocr_cmd,omitempty"`
	// PaddleArgs maps to CONTENT_PARSE_PADDLEOCR_ARGS and supports
	// {image}, {lang}, and {output_dir} placeholders.
	PaddleArgs string `json:"paddleocr_args,omitempty"`
}

// LoadConfigJSON loads runtime config from a JSON file. Missing fields remain zero.
func LoadConfigJSON(path string) (Config, error) {
	var c Config
	b, err := os.ReadFile(path)
	if err != nil {
		return c, fmt.Errorf("read config: %w", err)
	}
	if err := json.Unmarshal(b, &c); err != nil {
		return c, fmt.Errorf("parse json: %w", err)
	}
	return c, nil
}
