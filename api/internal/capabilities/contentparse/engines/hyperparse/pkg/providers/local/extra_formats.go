package local

import (
	"archive/zip"
	"bytes"
	"encoding/csv"
	"encoding/xml"
	"fmt"
	"io"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"unicode"

	"github.com/extrame/xls"
	"github.com/xuri/excelize/v2"
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
		return parseDelimitedRowsAsDoc(filename, data, ',', "local:csv")
	case ".tsv":
		return parseDelimitedRowsAsDoc(filename, data, '\t', "local:tsv")
	case ".xlsx":
		doc, err := parseXLSXRowsAsDoc(filename, data)
		if err == nil {
			return doc, nil
		}
		texts := extractTextFromZipXML(data, func(name string) bool {
			return strings.HasPrefix(name, "xl/worksheets/") || name == "xl/sharedStrings.xml"
		})
		return buildDocFromBlocks(filename, texts, "local:xlsx", "xlsx row parsing failed; fell back to xml text extraction")
	case ".pptx":
		texts := extractTextFromZipXML(data, func(name string) bool {
			return strings.HasPrefix(name, "ppt/slides/slide") && strings.HasSuffix(name, ".xml")
		})
		return buildDocFromBlocks(filename, texts, "local:pptx", "")
	case ".xls":
		return parseXLSRowsAsDoc(filename, data)
	case ".ppt", ".doc":
		blocks := extractLegacyOfficeText(data)
		return buildDocFromBlocks(filename, blocks, "local:legacy_binary", "legacy binary office extraction; text quality may be limited")
	default:
		return nil, fmt.Errorf("unsupported local format: %s", ext)
	}
}

func parseDelimitedRowsAsDoc(filename string, data []byte, comma rune, source string) (*extractcommon.DocumentResult, error) {
	reader := csv.NewReader(bytes.NewReader(data))
	reader.Comma = comma
	reader.FieldsPerRecord = -1
	reader.TrimLeadingSpace = true

	headers, err := reader.Read()
	if err != nil {
		if err == io.EOF {
			return nil, fmt.Errorf("no text extracted from %s", filepath.Ext(filename))
		}
		return nil, fmt.Errorf("read delimited header: %w", err)
	}

	rows := make([]spreadsheetRow, 0)
	rowIndex := 2
	for {
		record, err := reader.Read()
		if err != nil {
			if err == io.EOF {
				break
			}
			return nil, fmt.Errorf("read delimited row: %w", err)
		}
		content := spreadsheetRowContent(headers, record)
		if content == "" {
			rowIndex++
			continue
		}
		rows = append(rows, spreadsheetRow{Content: content, RowIndex: rowIndex})
		rowIndex++
	}
	return buildSpreadsheetDocFromRows(filename, source, rows)
}

func parseXLSXRowsAsDoc(filename string, data []byte) (*extractcommon.DocumentResult, error) {
	f, err := excelize.OpenReader(bytes.NewReader(data))
	if err != nil {
		return nil, fmt.Errorf("open xlsx: %w", err)
	}
	defer f.Close()

	rows := make([]spreadsheetRow, 0)
	for sheetIndex, sheetName := range f.GetSheetList() {
		sheetRows, err := f.GetRows(sheetName)
		if err != nil || len(sheetRows) == 0 {
			continue
		}
		headers := sheetRows[0]
		for rowIndex := 1; rowIndex < len(sheetRows); rowIndex++ {
			content := spreadsheetRowContent(headers, sheetRows[rowIndex])
			if content == "" {
				continue
			}
			rows = append(rows, spreadsheetRow{
				Content:  content,
				Sheet:    sheetName,
				Page:     sheetIndex,
				RowIndex: rowIndex + 1,
			})
		}
	}
	return buildSpreadsheetDocFromRows(filename, "local:xlsx", rows)
}

func parseXLSRowsAsDoc(filename string, data []byte) (*extractcommon.DocumentResult, error) {
	wb, err := xls.OpenReader(bytes.NewReader(data), "utf-8")
	if err != nil {
		return nil, fmt.Errorf("open xls: %w", err)
	}

	rows := make([]spreadsheetRow, 0)
	for sheetIndex := 0; sheetIndex < wb.NumSheets(); sheetIndex++ {
		sheet := wb.GetSheet(sheetIndex)
		if sheet == nil {
			continue
		}
		headerRow := sheet.Row(0)
		if headerRow == nil {
			continue
		}
		headers := make([]string, headerRow.LastCol())
		for colIndex := 0; colIndex < headerRow.LastCol(); colIndex++ {
			headers[colIndex] = headerRow.Col(colIndex)
		}
		for rowIndex := 1; rowIndex <= int(sheet.MaxRow); rowIndex++ {
			row := sheet.Row(rowIndex)
			if row == nil {
				continue
			}
			record := make([]string, row.LastCol())
			for colIndex := 0; colIndex < row.LastCol(); colIndex++ {
				record[colIndex] = row.Col(colIndex)
			}
			content := spreadsheetRowContent(headers, record)
			if content == "" {
				continue
			}
			rows = append(rows, spreadsheetRow{
				Content:  content,
				Sheet:    sheet.Name,
				Page:     sheetIndex,
				RowIndex: rowIndex + 1,
			})
		}
	}
	return buildSpreadsheetDocFromRows(filename, "local:xls", rows)
}

type spreadsheetRow struct {
	Content  string
	Sheet    string
	Page     int
	RowIndex int
}

func spreadsheetRowContent(headers, record []string) string {
	pageContent := make([]string, 0, len(record))
	for colIndex, cell := range record {
		cell = strings.TrimSpace(cell)
		if cell == "" {
			continue
		}
		columnName := ""
		if colIndex < len(headers) {
			columnName = strings.TrimSpace(headers[colIndex])
		}
		if columnName == "" {
			columnName = spreadsheetColumnName(colIndex + 1)
		}
		pageContent = append(pageContent, fmt.Sprintf("\"%s\":\"%s\"", columnName, cell))
	}
	return strings.Join(pageContent, ";")
}

func spreadsheetColumnName(index int) string {
	if index <= 0 {
		return ""
	}

	var result strings.Builder
	for index > 0 {
		index--
		result.WriteByte(byte('A' + (index % 26)))
		index /= 26
	}

	runes := []rune(result.String())
	for i, j := 0, len(runes)-1; i < j; i, j = i+1, j-1 {
		runes[i], runes[j] = runes[j], runes[i]
	}
	return string(runes)
}

func buildSpreadsheetDocFromRows(filename, source string, rows []spreadsheetRow) (*extractcommon.DocumentResult, error) {
	if len(rows) == 0 {
		return nil, fmt.Errorf("no text extracted from %s", filepath.Ext(filename))
	}

	markdown := make([]string, 0, len(rows))
	out := &extractcommon.DocumentResult{
		DocID:    makeChunkID(filename, 0),
		FileName: filename,
		Source:   source,
	}
	maxPage := 0
	for i, row := range rows {
		content := strings.TrimSpace(row.Content)
		if content == "" {
			continue
		}
		payload := map[string]any{
			"row_index": row.RowIndex,
		}
		if row.Sheet != "" {
			payload["sheet"] = row.Sheet
		}
		page := row.Page
		if page < 0 {
			page = 0
		}
		if page > maxPage {
			maxPage = page
		}
		out.Chunks = append(out.Chunks, extractcommon.Chunk{
			ID:        fmt.Sprintf("local-spreadsheet-row-%d", i),
			Type:      "table",
			Page:      page,
			Text:      content,
			Markdown:  content,
			Ordinal:   i + 1,
			Precision: "native",
			Payload:   payload,
		})
		markdown = append(markdown, content)
	}
	if len(out.Chunks) == 0 {
		return nil, fmt.Errorf("no text extracted from %s", filepath.Ext(filename))
	}
	out.PageCount = maxPage + 1
	out.Pages = make([]extractcommon.Page, 0, out.PageCount)
	for pageIndex := 0; pageIndex < out.PageCount; pageIndex++ {
		out.Pages = append(out.Pages, extractcommon.Page{PageIndex: pageIndex})
	}
	out.Markdown = strings.Join(markdown, "\n\n")
	return out, nil
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
