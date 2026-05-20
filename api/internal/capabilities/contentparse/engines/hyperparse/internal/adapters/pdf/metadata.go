package pdf

import (
	"bytes"
	"fmt"
	"strings"
)

// ReadDocumentMetadataBytes reads common string fields from the Info dictionary.
func ReadDocumentMetadataBytes(data []byte) (map[string]string, error) {
	eofPos := bytes.LastIndex(data, []byte("%%EOF"))
	if eofPos < 0 {
		return nil, fmt.Errorf("missing %%EOF")
	}
	startXRefTagPos := bytes.LastIndex(data[:eofPos], []byte("startxref"))
	if startXRefTagPos < 0 {
		return nil, fmt.Errorf("missing startxref")
	}
	startXRef, err := parseStartXRefValue(data[startXRefTagPos+len("startxref") : eofPos])
	if err != nil || startXRef < 0 || startXRef >= len(data) {
		return nil, fmt.Errorf("invalid startxref")
	}
	lines := splitXRefRegionNonEmptyLines(data[startXRef:eofPos])
	trailerIdx := -1
	for i := 0; i < len(lines); i++ {
		if strings.TrimSpace(lines[i]) == "trailer" {
			trailerIdx = i
			break
		}
	}
	if trailerIdx < 0 {
		return map[string]string{}, nil
	}
	infoObjNum, ok := parseTrailerInfoObject(lines, trailerIdx+1)
	if !ok {
		return map[string]string{}, nil
	}
	objBlock, ok := findObjectBlockByNumber(data, infoObjNum)
	if !ok {
		return map[string]string{}, nil
	}
	out := map[string]string{}
	for _, k := range []string{
		"Title", "Author", "Subject", "Keywords",
		"Producer", "Creator", "CreationDate", "ModDate",
	} {
		if v := extractPDFStringByKey(objBlock, k); v != "" {
			out[k] = v
		}
	}
	return out, nil
}

// AppendIncrementalInfoMetadata appends an incremental update to traditional
// xref-table PDFs: new Info object, xref, and trailer with /Prev.
func AppendIncrementalInfoMetadata(data []byte, meta map[string]string) ([]byte, error) {
	eofPos := bytes.LastIndex(data, []byte("%%EOF"))
	if eofPos < 0 {
		return nil, fmt.Errorf("missing %%EOF")
	}
	startXRefTagPos := bytes.LastIndex(data[:eofPos], []byte("startxref"))
	if startXRefTagPos < 0 {
		return nil, fmt.Errorf("missing startxref")
	}
	startXRef, err := parseStartXRefValue(data[startXRefTagPos+len("startxref") : eofPos])
	if err != nil || startXRef < 0 || startXRef >= len(data) {
		return nil, fmt.Errorf("invalid startxref")
	}
	xType, hasTrailer := detectXRefType(data, startXRef, eofPos)
	if xType != "table" || !hasTrailer {
		return nil, fmt.Errorf("metadata append: only xref table PDFs supported (not xref stream)")
	}
	stats, err := validateXRefTable(data, startXRef, eofPos, ValidationModeRelaxed)
	if err != nil {
		return nil, err
	}
	if !stats.hasTrailerSize {
		return nil, fmt.Errorf("metadata append: trailer missing /Size")
	}
	rootObj, ok := parseRootObjectFromTrailer(data)
	if !ok {
		return nil, fmt.Errorf("metadata append: cannot parse /Root")
	}
	newObjNum := stats.trailerSize
	newSize := newObjNum + 1
	dictStr := buildInfoDictionaryLiteral(meta)
	var buf bytes.Buffer
	buf.Write(data)
	objOffset := buf.Len()
	buf.WriteString(fmt.Sprintf("%d 0 obj\n%s\nendobj\n", newObjNum, dictStr))
	xrefPos := buf.Len()
	buf.WriteString(fmt.Sprintf("xref\n%d 1\n%010d 00000 n \n", newObjNum, objOffset))
	buf.WriteString(fmt.Sprintf("trailer\n<< /Size %d /Root %d 0 R /Info %d 0 R /Prev %d >>\n", newSize, rootObj, newObjNum, startXRef))
	buf.WriteString(fmt.Sprintf("startxref\n%d\n", xrefPos))
	buf.WriteString("%%EOF\n")
	return buf.Bytes(), nil
}

func buildInfoDictionaryLiteral(meta map[string]string) string {
	order := []string{
		"Title", "Author", "Subject", "Keywords",
		"Creator", "Producer", "CreationDate", "ModDate",
	}
	var parts []string
	for _, k := range order {
		v := strings.TrimSpace(meta[k])
		if v == "" {
			continue
		}
		parts = append(parts, fmt.Sprintf("/%s (%s)", k, escapePDFLiteralForWrite(v)))
	}
	if len(parts) == 0 {
		return "<< >>"
	}
	return "<< " + strings.Join(parts, " ") + " >>"
}

func escapePDFLiteralForWrite(s string) string {
	var b strings.Builder
	for _, r := range s {
		switch r {
		case '\\', '(', ')':
			b.WriteByte('\\')
			b.WriteRune(r)
		default:
			b.WriteRune(r)
		}
	}
	return b.String()
}
