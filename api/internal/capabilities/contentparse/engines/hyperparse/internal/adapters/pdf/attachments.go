package pdf

import (
	"regexp"
	"strconv"
	"strings"
	"time"
)

type AttachmentEntry struct {
	FileSpecObject    int    `json:"filespec_object"`
	FileName          string `json:"file_name,omitempty"`
	UnicodeFileName   string `json:"unicode_file_name,omitempty"`
	EmbeddedFileObj   int    `json:"embedded_file_object,omitempty"`
	EmbeddedSizeBytes int    `json:"embedded_size_bytes,omitempty"`
	EmbeddedSubtype   string `json:"embedded_subtype,omitempty"`
}

var filespecObjRE = regexp.MustCompile(`(?s)(\d+)\s+\d+\s+obj\s*<<(.*?)>>\s*endobj`)
var embeddedSubtypeRE = regexp.MustCompile(`/Subtype\s*/([^\s<>\[\]\(\)]+)`)

func ExtractAttachmentEntriesFromBytes(data []byte, mode string) ([]AttachmentEntry, error) {
	t0 := time.Now()
	fullDocDebugf("ExtractAttachmentEntriesFromBytes begin bytes=%d", len(data))
	var out []AttachmentEntry
	defer func() {
		fullDocDebugf("ExtractAttachmentEntriesFromBytes done attachments=%d elapsed_ms=%d", len(out), time.Since(t0).Milliseconds())
	}()
	blocks := filespecObjRE.FindAllStringSubmatch(string(data), -1)
	if len(blocks) == 0 {
		return []AttachmentEntry{}, nil
	}
	out = make([]AttachmentEntry, 0, len(blocks))
	for _, m := range blocks {
		if len(m) < 3 {
			continue
		}
		objNum, err := strconv.Atoi(strings.TrimSpace(m[1]))
		if err != nil || objNum <= 0 {
			continue
		}
		body := m[2]
		if !strings.Contains(body, "/Filespec") || !strings.Contains(body, "/EF") {
			continue
		}
		row := AttachmentEntry{
			FileSpecObject:  objNum,
			FileName:        extractPDFStringByKey([]byte(body), "F"),
			UnicodeFileName: extractPDFStringByKey([]byte(body), "UF"),
		}
		efObj := parseEmbeddedFileObjFromFileSpec(body)
		if efObj > 0 {
			row.EmbeddedFileObj = efObj
			if efBlock, ok := findObjectBlockByNumber(data, efObj); ok {
				if m := embeddedSubtypeRE.FindSubmatch(efBlock); len(m) >= 2 {
					row.EmbeddedSubtype = strings.TrimPrefix(string(m[1]), "/")
				}
				row.EmbeddedSizeBytes = parseIntByKeyInBlock(efBlock, "/Length")
			}
		}
		out = append(out, row)
	}
	return out, nil
}

func parseEmbeddedFileObjFromFileSpec(body string) int {
	efPos := strings.Index(body, "/EF")
	if efPos < 0 {
		return 0
	}
	rest := strings.TrimSpace(body[efPos+len("/EF"):])
	if !strings.HasPrefix(rest, "<<") {
		return 0
	}
	end := strings.Index(rest, ">>")
	if end < 0 {
		return 0
	}
	efDict := rest[:end+2]
	if n, ok := parseIndirectRefObjectNumberByKey([]byte(efDict), "/F"); ok && n > 0 {
		return n
	}
	if n, ok := parseIndirectRefObjectNumberByKey([]byte(efDict), "/UF"); ok && n > 0 {
		return n
	}
	return 0
}

func parseIntByKeyInBlock(block []byte, key string) int {
	pos := strings.Index(string(block), key)
	if pos < 0 {
		return 0
	}
	fields := strings.Fields(string(block[pos+len(key):]))
	if len(fields) == 0 {
		return 0
	}
	n, _ := strconv.Atoi(fields[0])
	return n
}
