package hyperparse

import (
	"path/filepath"
	"sort"
	"strings"
)

// defaultExtToFormat mirrors the built-in RegisterAdapter extension mapping in defaults.go.
// Keep both places in sync when adding a built-in format.
var defaultExtToFormat = map[string]string{
	".pdf":      "pdf",
	".docx":     "docx",
	".md":       "markdown",
	".markdown": "markdown",
	".txt":      "text",
	".csv":      "text",
	".tsv":      "text",
	".png":      "image",
	".jpg":      "image",
	".jpeg":     "image",
	".tif":      "image",
	".tiff":     "image",
	".webp":     "image",
}

// FormatFromFilename returns a pipeline format key such as pdf or markdown from a filename.
// It returns an empty string when the extension is unknown.
func FormatFromFilename(filename string) string {
	ext := strings.ToLower(filepath.Ext(filename))
	if ext == "" {
		return ""
	}
	return defaultExtToFormat[ext]
}

// SupportedExtensions returns the default supported lowercase extensions, including dots.
func SupportedExtensions() []string {
	out := make([]string, 0, len(defaultExtToFormat))
	for e := range defaultExtToFormat {
		out = append(out, e)
	}
	sort.Strings(out)
	return out
}
