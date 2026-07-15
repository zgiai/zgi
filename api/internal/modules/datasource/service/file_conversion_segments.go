package service

import (
	"encoding/csv"
	"fmt"
	"io"
	"regexp"
	"sort"
	"strings"

	"github.com/PuerkitoBio/goquery"
)

const (
	fileConversionRowsPerSegment = 40
	fileConversionMaxTextRunes   = 12000
)

var markdownTableSeparatorPattern = regexp.MustCompile(`^\s*\|?\s*:?-{3,}:?\s*(?:\|\s*:?-{3,}:?\s*)+\|?\s*$`)

type fileConversionSegment struct {
	Content          string
	SourceRowIndexes []int
}

func splitFileConversionContent(content string) ([]fileConversionSegment, bool) {
	if rows, header := markdownTableRows(content); len(rows) > 0 {
		return tableRowSegments(header, rows), true
	}
	if rows, header := htmlTableRows(content); len(rows) > 0 {
		return tableRowSegments(header, rows), true
	}
	if rows, header := csvTableRows(content); len(rows) > 0 {
		return tableRowSegments(header, rows), true
	}
	return textSegments(content), false
}

func markdownTableRows(content string) ([]string, string) {
	lines := strings.Split(strings.ReplaceAll(content, "\r\n", "\n"), "\n")
	for i := 0; i+2 < len(lines); i++ {
		if !strings.Contains(lines[i], "|") || !markdownTableSeparatorPattern.MatchString(lines[i+1]) {
			continue
		}
		rows := make([]string, 0)
		for j := i + 2; j < len(lines); j++ {
			line := strings.TrimSpace(lines[j])
			if line == "" || !strings.Contains(line, "|") {
				break
			}
			rows = append(rows, line)
		}
		if len(rows) > 0 {
			return rows, strings.TrimSpace(lines[i])
		}
	}
	return nil, ""
}

func htmlTableRows(content string) ([]string, string) {
	if !strings.Contains(strings.ToLower(content), "<table") {
		return nil, ""
	}
	doc, err := goquery.NewDocumentFromReader(strings.NewReader(content))
	if err != nil {
		return nil, ""
	}
	var header string
	rows := make([]string, 0)
	doc.Find("table").First().Find("tr").Each(func(_ int, row *goquery.Selection) {
		cells := make([]string, 0)
		row.Find("th,td").Each(func(_ int, cell *goquery.Selection) {
			cells = append(cells, strings.TrimSpace(cell.Text()))
		})
		if len(cells) == 0 {
			return
		}
		line := strings.Join(cells, " | ")
		if header == "" && row.Find("th").Length() > 0 {
			header = line
			return
		}
		rows = append(rows, line)
	})
	if header == "" || len(rows) == 0 {
		return nil, ""
	}
	return rows, header
}

func csvTableRows(content string) ([]string, string) {
	reader := csv.NewReader(strings.NewReader(content))
	reader.FieldsPerRecord = -1
	records := make([][]string, 0)
	for {
		record, err := reader.Read()
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, ""
		}
		records = append(records, record)
	}
	if len(records) < 2 || len(records[0]) < 2 {
		return nil, ""
	}
	rows := make([]string, 0, len(records)-1)
	for _, record := range records[1:] {
		if len(record) != len(records[0]) {
			return nil, ""
		}
		rows = append(rows, strings.Join(record, " | "))
	}
	return rows, strings.Join(records[0], " | ")
}

func tableRowSegments(header string, rows []string) []fileConversionSegment {
	segments := make([]fileConversionSegment, 0, (len(rows)+fileConversionRowsPerSegment-1)/fileConversionRowsPerSegment)
	for start := 0; start < len(rows); start += fileConversionRowsPerSegment {
		end := min(start+fileConversionRowsPerSegment, len(rows))
		indexes := make([]int, 0, end-start)
		var builder strings.Builder
		fmt.Fprintf(&builder, "Source table header: %s\n", header)
		for i := start; i < end; i++ {
			rowIndex := i + 1
			indexes = append(indexes, rowIndex)
			fmt.Fprintf(&builder, "SOURCE_ROW_%d: %s\n", rowIndex, rows[i])
		}
		segments = append(segments, fileConversionSegment{Content: builder.String(), SourceRowIndexes: indexes})
	}
	return segments
}

func textSegments(content string) []fileConversionSegment {
	paragraphs := regexp.MustCompile(`\n\s*\n`).Split(strings.TrimSpace(content), -1)
	segments := make([]fileConversionSegment, 0, 1)
	var builder strings.Builder
	flush := func() {
		if builder.Len() == 0 {
			return
		}
		segments = append(segments, fileConversionSegment{Content: strings.TrimSpace(builder.String())})
		builder.Reset()
	}
	for _, paragraph := range paragraphs {
		paragraph = strings.TrimSpace(paragraph)
		if paragraph == "" {
			continue
		}
		paragraphRunes := []rune(paragraph)
		if len(paragraphRunes) > fileConversionMaxTextRunes {
			flush()
			for start := 0; start < len(paragraphRunes); start += fileConversionMaxTextRunes {
				end := min(start+fileConversionMaxTextRunes, len(paragraphRunes))
				segments = append(segments, fileConversionSegment{Content: string(paragraphRunes[start:end])})
			}
			continue
		}
		if builder.Len() > 0 && len([]rune(builder.String()))+len([]rune(paragraph)) > fileConversionMaxTextRunes {
			flush()
		}
		if builder.Len() > 0 {
			builder.WriteString("\n\n")
		}
		builder.WriteString(paragraph)
	}
	flush()
	return segments
}

func validateAndOrderSourceRows(parsed *fileConversionLLMResponse, expected []int) error {
	if len(parsed.Records) != len(expected) {
		return fmt.Errorf("field extraction returned %d records for %d source rows", len(parsed.Records), len(expected))
	}
	expectedSet := make(map[int]struct{}, len(expected))
	for _, index := range expected {
		expectedSet[index] = struct{}{}
	}
	seen := make(map[int]struct{}, len(expected))
	for _, record := range parsed.Records {
		if record.SourceRowIndex == nil {
			return fmt.Errorf("field extraction record is missing source_row_index")
		}
		index := *record.SourceRowIndex
		if _, ok := expectedSet[index]; !ok {
			return fmt.Errorf("field extraction returned unknown source_row_index %d", index)
		}
		if _, duplicate := seen[index]; duplicate {
			return fmt.Errorf("field extraction returned duplicate source_row_index %d", index)
		}
		seen[index] = struct{}{}
	}
	sort.Slice(parsed.Records, func(i, j int) bool {
		return *parsed.Records[i].SourceRowIndex < *parsed.Records[j].SourceRowIndex
	})
	return nil
}
