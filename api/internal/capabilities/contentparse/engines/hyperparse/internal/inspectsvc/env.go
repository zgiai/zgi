package inspectsvc

import (
	"strconv"
	"strings"

	"github.com/zgiai/zgi/api/internal/capabilities/contentparse/envconfig"
)

func contentParseEnv(key string) string {
	if strings.HasPrefix(key, "CONTENT_PARSE_") {
		if v := envconfig.String(key); v != "" {
			return v
		}
		return envconfig.String("DOCSTILL_" + strings.TrimPrefix(key, "CONTENT_PARSE_"))
	}
	if strings.HasPrefix(key, "DOCSTILL_") {
		primary := "CONTENT_PARSE_" + strings.TrimPrefix(key, "DOCSTILL_")
		if v := envconfig.String(primary); v != "" {
			return v
		}
	}
	return envconfig.String(key)
}

// VLMFallbackMaxPages reads the maximum number of pages for full-page VLM fallback.
// Values <=0 or unset mean unlimited.
func VLMFallbackMaxPages() int {
	s := contentParseEnv("CONTENT_PARSE_VLM_FALLBACK_MAX_PAGES")
	if s == "" {
		return 0
	}
	n, err := strconv.Atoi(s)
	if err != nil {
		return 0
	}
	if n <= 0 {
		return 0
	}
	return n
}

// VLMFallbackBatchPages reads the VLM fallback batch size. Default: 8.
func VLMFallbackBatchPages() int {
	s := contentParseEnv("CONTENT_PARSE_VLM_FALLBACK_BATCH_PAGES")
	if s == "" {
		return 8
	}
	n, err := strconv.Atoi(s)
	if err != nil || n <= 0 {
		return 8
	}
	if n > 32 {
		return 32
	}
	return n
}

// VLMFallbackConcurrency reads full-page VLM fallback concurrency. Default: 2.
func VLMFallbackConcurrency() int {
	s := contentParseEnv("CONTENT_PARSE_VLM_FALLBACK_CONCURRENCY")
	if s == "" {
		return 2
	}
	n, err := strconv.Atoi(s)
	if err != nil || n <= 0 {
		return 2
	}
	if n > 8 {
		return 8
	}
	return n
}

// PDFRenderConcurrency reads PDF render concurrency. Default: 2.
func PDFRenderConcurrency() int {
	s := contentParseEnv("CONTENT_PARSE_PDF_RENDER_CONCURRENCY")
	if s == "" {
		return 2
	}
	n, err := strconv.Atoi(s)
	if err != nil || n <= 0 {
		return 2
	}
	if n > 4 {
		return 4
	}
	return n
}

// VLMFallbackFastFirstPages reads the fast-first preview page count. Zero disables it.
func VLMFallbackFastFirstPages() int {
	s := contentParseEnv("CONTENT_PARSE_VLM_FALLBACK_FAST_FIRST_PAGES")
	if s == "" {
		return 0
	}
	n, err := strconv.Atoi(s)
	if err != nil || n <= 0 {
		return 0
	}
	if n > 32 {
		return 32
	}
	return n
}

// EnvIntDefault reads an integer environment variable and returns def when unset or invalid.
func EnvIntDefault(key string, def int) int {
	s := contentParseEnv(key)
	if s == "" {
		return def
	}
	n, err := strconv.Atoi(s)
	if err != nil || n < 0 {
		return def
	}
	return n
}

// EnvBoolLower reads 1/true/yes/on case-insensitively and returns def when unset.
func EnvBoolLower(key string, def bool) bool {
	v := contentParseEnv(key)
	if v == "" {
		return def
	}
	s := strings.ToLower(v)
	return s == "1" || s == "true" || s == "yes" || s == "on"
}

const (
	envForceVLM            = "CONTENT_PARSE_FORCE_VLM"
	envFullPageVLMFallback = "CONTENT_PARSE_VLM_FULL_PAGE_FALLBACK"
)

// ForceVLM forces the full-page VLM fallback path.
func ForceVLM() bool {
	return EnvBoolLower(envForceVLM, false)
}

// FullPageVLMFallbackEnabled controls whether suggest_vlm may trigger full-page
// VLM fallback. It is disabled by default so local rules plus regional/image VLM
// repair remain lightweight.
func FullPageVLMFallbackEnabled() bool {
	return ForceVLM() || EnvBoolLower(envFullPageVLMFallback, false)
}
