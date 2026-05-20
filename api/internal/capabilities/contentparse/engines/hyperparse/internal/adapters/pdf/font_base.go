package pdf

import (
	"regexp"
	"strconv"
	"strings"
)

// Page Resources to Font /BaseFont names, mapping resource names such as F1 to
// PostScript names such as ABCDEE+Calibri.
var reBaseFontInFontObj = regexp.MustCompile(`(?i)/BaseFont\s*/([^\s/\[\]<>]+)`)

func extractBaseFontNameFromFontObjectBlock(block []byte) string {
	m := reBaseFontInFontObj.FindSubmatch(block)
	if len(m) < 2 {
		return ""
	}
	s := strings.TrimSpace(string(m[1]))
	return strings.TrimPrefix(s, "/")
}

// isDocMathFontBaseName mirrors chunking.IsDocMathFontBaseName for adapter-local use.
func isDocMathFontBaseName(base string) bool {
	base = strings.TrimSpace(base)
	if base == "" {
		return false
	}
	n := strings.ToLower(strings.ReplaceAll(base, " ", ""))
	subs := []string{
		"cambriamath", "timesnewroman", "latinmodernmath", "stix",
		"mathjax", "katex", "xitsmath", "texgyre", "asana",
		"eulermath", "lmmath", "unicode-math", "gyremath", "firamath",
	}
	for _, s := range subs {
		if strings.Contains(n, s) {
			return true
		}
	}
	return false
}

// buildPageFontBaseNameMap scans the same page font resources as Unicode maps
// and attaches BaseFont metadata to geometry runs.
func buildPageFontBaseNameMap(data []byte, sp PageRenderSpec, mode string) map[string]string {
	out := map[string]string{}
	resBody := resolveResourcesBodyForPageText(data, sp, mode)
	if resBody == "" {
		return out
	}
	fontFrag := extractFontDictFragmentFromResources(data, resBody, mode)
	if fontFrag == "" {
		return out
	}
	for _, m := range reNamedRef.FindAllStringSubmatch(fontFrag, -1) {
		if len(m) < 3 {
			continue
		}
		fontKey := m[1]
		fontObj, err := strconv.Atoi(m[2])
		if err != nil || fontObj <= 0 {
			continue
		}
		fontBlock, err := ExtractObjectBlockByNumberBytes(data, fontObj, mode)
		if err != nil {
			continue
		}
		if bf := extractBaseFontNameFromFontObjectBlock([]byte(fontBlock)); bf != "" {
			out[fontKey] = bf
		}
	}
	return out
}
