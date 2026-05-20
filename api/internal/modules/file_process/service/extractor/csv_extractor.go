package extractor

import (
	"context"
	"encoding/csv"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/zgiai/ginext/internal/dto"
)

type CSVExtractor struct {
	filePath string
}

func NewCSVExtractor(filePath string) *CSVExtractor {
	return &CSVExtractor{
		filePath: filePath,
	}
}

func (e *CSVExtractor) Extract(ctx context.Context) (*dto.ExtractOutput, error) {
	file, err := os.Open(e.filePath)
	if err != nil {
		return nil, fmt.Errorf("failed to open .csv file: %w", err)
	}
	defer file.Close()

	reader := csv.NewReader(file)
	reader.FieldsPerRecord = -1
	reader.TrimLeadingSpace = true

	headers, err := reader.Read()
	if err != nil {
		if err == io.EOF {
			return &dto.ExtractOutput{Source: "zgi:csv"}, nil
		}
		return nil, fmt.Errorf("failed to read .csv header: %w", err)
	}

	output := &dto.ExtractOutput{
		Elements: make([]dto.ExtractElement, 0),
		Source:   "zgi:csv",
		Metadata: map[string]any{
			"source": e.filePath,
		},
	}
	var markdown strings.Builder
	ordinal := 0

	for {
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		default:
		}

		record, err := reader.Read()
		if err != nil {
			if err == io.EOF {
				break
			}
			return nil, fmt.Errorf("failed to read .csv row: %w", err)
		}

		content := e.recordContent(headers, record)
		if content == "" {
			continue
		}
		if markdown.Len() > 0 {
			markdown.WriteString("\n")
		}
		markdown.WriteString(content)

		output.Elements = append(output.Elements, dto.ExtractElement{
			Type:    "table",
			Content: content,
			Ordinal: ordinal,
			Metadata: map[string]any{
				"source": e.filePath,
			},
		})
		ordinal++
	}

	output.Markdown = markdown.String()
	return output, nil
}

func (e *CSVExtractor) recordContent(headers, record []string) string {
	pageContent := make([]string, 0, len(record))
	for colIdx, cell := range record {
		cell = strings.TrimSpace(cell)
		if cell == "" {
			continue
		}

		var columnName string
		if colIdx < len(headers) && strings.TrimSpace(headers[colIdx]) != "" {
			columnName = strings.TrimSpace(headers[colIdx])
		} else {
			columnName = e.columnName(colIdx + 1)
		}
		pageContent = append(pageContent, fmt.Sprintf("\"%s\":\"%s\"", columnName, cell))
	}

	return strings.Join(pageContent, ";")
}

func (e *CSVExtractor) columnName(index int) string {
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
