package local

import (
	"archive/zip"
	"bytes"
	"encoding/xml"
	"fmt"
	"io"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"unicode"

	extractcommon "github.com/zgiai/zgi/api/internal/capabilities/contentparse/engines/hyperparse/pkg/providers/common"
)

func supportsLocalExtraExt(ext string) bool {
	switch ext {
	case ".csv", ".tsv", ".xlsx", ".pptx", ".xls", ".ppt", ".doc":
		return true
	default:
		return false
	}
}

func parseLocalExtraFormat(filename string, data []byte, ext string) (*extractcommon.DocumentResult, error) {
	switch ext {
	case ".csv":
		return parseDelimitedAsDoc(filename, data, ",", "local:csv")
	case ".tsv":
		return parseDelimitedAsDoc(filename, data, "\t", "local:tsv")
	case ".xlsx":
		texts := extractTextFromZipXML(data, func(name string) bool {
			return strings.HasPrefix(name, "xl/worksheets/") || name == "xl/sharedStrings.xml"
		})
		return buildDocFromBlocks(filename, texts, "local:xlsx", "")
	case ".pptx":
		texts := extractTextFromZipXML(data, func(name string) bool {
			return strings.HasPrefix(name, "ppt/slides/slide") && strings.HasSuffix(name, ".xml")
		})
		return buildDocFromBlocks(filename, texts, "local:pptx", "")
	case ".xls", ".ppt", ".doc":
		blocks := extractLegacyOfficeText(data)
		return buildDocFromBlocks(filename, blocks, "local:legacy_binary", "legacy binary office extraction; text quality may be limited")
	default:
		return nil, fmt.Errorf("unsupported local format: %s", ext)
	}
}

func parseDelimitedAsDoc(filename string, data []byte, sep, source string) (*extractcommon.DocumentResult, error) {
	text := strings.ReplaceAll(string(data), "\r\n", "\n")
	text = strings.ReplaceAll(text, "\r", "\n")
	lines := strings.Split(text, "\n")
	blocks := make([]string, 0, len(lines))
	for _, ln := range lines {
		ln = strings.TrimSpace(ln)
		if ln == "" {
			continue
		}
		parts := strings.Split(ln, sep)
		for i := range parts {
			parts[i] = strings.TrimSpace(parts[i])
		}
		blocks = append(blocks, strings.Join(parts, " | "))
	}
	return buildDocFromBlocks(filename, blocks, source, "")
}

func buildDocFromBlocks(filename string, blocks []string, source, hint string) (*extractcommon.DocumentResult, error) {
	clean := make([]string, 0, len(blocks))
	for _, b := range blocks {
		b = strings.TrimSpace(strings.Join(strings.Fields(b), " "))
		if b != "" {
			clean = append(clean, b)
		}
	}
	if len(clean) == 0 {
		return nil, fmt.Errorf("no text extracted from %s", filepath.Ext(filename))
	}
	out := &extractcommon.DocumentResult{
		DocID:     makeChunkID(filename, 0),
		FileName:  filename,
		PageCount: 1,
		Pages:     []extractcommon.Page{{PageIndex: 0}},
		Source:    source,
		Markdown:  strings.Join(clean, "\n\n"),
	}
	if hint != "" {
		out.Diagnostics = map[string]any{"hint": hint}
	}
	for i, b := range clean {
		out.Chunks = append(out.Chunks, extractcommon.Chunk{
			ID:        fmt.Sprintf("local-extra-%d", i),
			Type:      "text",
			Page:      0,
			Text:      b,
			Markdown:  b,
			Ordinal:   i + 1,
			Precision: "unreliable",
		})
	}
	return out, nil
}

func extractTextFromZipXML(data []byte, keep func(name string) bool) []string {
	zr, err := zip.NewReader(bytes.NewReader(data), int64(len(data)))
	if err != nil {
		return nil
	}
	var files []string
	content := make(map[string][]byte)
	for _, f := range zr.File {
		name := strings.ToLower(f.Name)
		if !keep(name) {
			continue
		}
		rc, e := f.Open()
		if e != nil {
			continue
		}
		b, e := io.ReadAll(rc)
		_ = rc.Close()
		if e != nil {
			continue
		}
		files = append(files, name)
		content[name] = b
	}
	sort.Strings(files)

	out := make([]string, 0, 64)
	for _, name := range files {
		out = append(out, extractXMLTextTokens(content[name])...)
	}
	return out
}

var xmlTagStripRE = regexp.MustCompile(`<[^>]+>`)

func extractXMLTextTokens(data []byte) []string {
	// Prefer token-level decode first.
	dec := xml.NewDecoder(bytes.NewReader(data))
	out := make([]string, 0, 32)
	for {
		tok, err := dec.Token()
		if err != nil {
			break
		}
		switch t := tok.(type) {
		case xml.CharData:
			s := strings.TrimSpace(string(t))
			if s != "" {
				out = append(out, s)
			}
		}
	}
	if len(out) > 0 {
		return out
	}
	// Fallback: strip tags if XML decoder gets nothing meaningful.
	plain := xmlTagStripRE.ReplaceAllString(string(data), " ")
	plain = strings.Join(strings.Fields(plain), " ")
	if plain == "" {
		return nil
	}
	return []string{plain}
}

func extractLegacyOfficeText(data []byte) []string {
	ascii := extractPrintableASCII(data, 6)
	utf16 := extractUTF16LEASCII(data, 4)
	all := append(ascii, utf16...)
	if len(all) == 0 {
		return nil
	}
	seen := map[string]bool{}
	out := make([]string, 0, len(all))
	for _, s := range all {
		s = strings.Join(strings.Fields(strings.TrimSpace(s)), " ")
		if s == "" || seen[s] {
			continue
		}
		seen[s] = true
		out = append(out, s)
		if len(out) >= 120 {
			break
		}
	}
	return out
}

func extractPrintableASCII(data []byte, minLen int) []string {
	var out []string
	var cur []rune
	flush := func() {
		if len(cur) >= minLen {
			out = append(out, string(cur))
		}
		cur = cur[:0]
	}
	for _, b := range data {
		r := rune(b)
		if r == '\n' || r == '\t' || (r >= 32 && r <= 126) {
			cur = append(cur, r)
			continue
		}
		flush()
	}
	flush()
	return out
}

func extractUTF16LEASCII(data []byte, minLen int) []string {
	var out []string
	var cur []rune
	flush := func() {
		if len(cur) >= minLen {
			out = append(out, string(cur))
		}
		cur = cur[:0]
	}
	for i := 0; i+1 < len(data); i += 2 {
		lo, hi := data[i], data[i+1]
		if hi == 0 && (unicode.IsPrint(rune(lo)) || lo == '\n' || lo == '\t') {
			cur = append(cur, rune(lo))
			continue
		}
		flush()
	}
	flush()
	return out
}
