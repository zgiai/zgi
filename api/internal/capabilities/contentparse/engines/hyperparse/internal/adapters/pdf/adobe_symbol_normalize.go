package pdf

import (
	"embed"
	"strconv"
	"strings"
	"unicode/utf8"
)

// Adobe Symbol encoded bytes to Unicode, based on Unicode, Inc. ADOBE/symbol.txt.
// Some PDF generators map formula fonts into U+F000-U+F0FF private-use code
// points. The low byte is restored through the Adobe Symbol table.

//go:embed adobe_symbol.txt
var adobeSymbolFS embed.FS

var adobeSymbolByteToRune [256]rune

func init() {
	data, err := adobeSymbolFS.ReadFile("adobe_symbol.txt")
	if err != nil {
		return
	}
	for _, line := range strings.Split(string(data), "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		fields := strings.Fields(line)
		if len(fields) < 2 {
			continue
		}
		u, err1 := strconv.ParseUint(fields[0], 16, 32)
		sym, err2 := strconv.ParseUint(fields[1], 16, 8)
		if err1 != nil || err2 != nil {
			continue
		}
		if sym >= 256 {
			continue
		}
		// Keep the first mapping when a Symbol byte has multiple entries.
		if adobeSymbolByteToRune[sym] == 0 {
			adobeSymbolByteToRune[sym] = rune(u)
		}
	}
}

// normalizeAdobeSymbolPUAText restores U+F000-U+F0FF private-use code points
// whose low byte matches Adobe Symbol encoding.
func normalizeAdobeSymbolPUAText(s string) string {
	if s == "" {
		return s
	}
	var b strings.Builder
	b.Grow(len(s))
	changed := false
	for i := 0; i < len(s); {
		r, sz := utf8.DecodeRuneInString(s[i:])
		if sz == 0 {
			break
		}
		i += sz
		if r >= 0xF000 && r <= 0xF0FF {
			low := byte(r & 0xFF)
			if u := adobeSymbolByteToRune[low]; u != 0 {
				b.WriteRune(u)
				changed = true
				continue
			}
		}
		b.WriteRune(r)
	}
	if !changed {
		return s
	}
	return b.String()
}
