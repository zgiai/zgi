package extractor

import (
	"context"
	"fmt"
	"os"
	"strings"

	"github.com/zgiai/ginext/internal/dto"

	gooxmldoc "baliance.com/gooxml/document"
	docx "github.com/fumiama/go-docx"
)

type WordExtractor struct {
	filePath string
	tenantID string
	userID   string
}

func NewWordExtractor(filePath, tenantID, userID string) *WordExtractor {
	return &WordExtractor{
		filePath: filePath,
		tenantID: tenantID,
		userID:   userID,
	}
}

func (e *WordExtractor) Extract(ctx context.Context) (*dto.ExtractOutput, error) {
	content, err := e.parseDocx(ctx, e.filePath)
	if err != nil {
		return nil, fmt.Errorf("error parsing docx file: %w", err)
	}

	metadata := map[string]interface{}{"source": e.filePath}
	documents := []dto.Document{
		{
			PageContent: content,
			Metadata:    metadata,
		},
	}
	return dto.NewExtractOutputFromDocuments("zgi:word", documents), nil
}

func (e *WordExtractor) parseDocxWithGooxml(ctx context.Context, docxPath string) (string, error) {
	doc, err := gooxmldoc.Open(docxPath)
	if err != nil {
		return "", fmt.Errorf("failed to parse docx with gooxml: %w", err)
	}

	var content []string

	for _, para := range doc.Paragraphs() {
		select {
		case <-ctx.Done():
			return "", ctx.Err()
		default:
		}
		var texts []string
		for _, run := range para.Runs() {
			t := run.Text()
			if t != "" {
				texts = append(texts, t)
			}
		}
		if len(texts) > 0 {
			content = append(content, strings.Join(texts, ""))
		}
	}

	for _, table := range doc.Tables() {
		for _, row := range table.Rows() {
			var cells []string
			for _, cell := range row.Cells() {
				var texts []string
				for _, para := range cell.Paragraphs() {
					for _, run := range para.Runs() {
						t := run.Text()
						if t != "" {
							texts = append(texts, t)
						}
					}
				}
				if len(texts) > 0 {
					cells = append(cells, strings.Join(texts, ""))
				}
			}
			if len(cells) > 0 {
				content = append(content, strings.Join(cells, "\t"))
			}
		}
	}

	return strings.Join(content, "\n"), nil
}

func (e *WordExtractor) parseDocx(ctx context.Context, docxPath string) (string, error) {
	readFile, err := os.Open(docxPath)
	if err != nil {
		return "", fmt.Errorf("failed to open docx file: %w", err)
	}
	defer readFile.Close()

	fileInfo, err := readFile.Stat()
	if err != nil {
		return "", fmt.Errorf("failed to get file info: %w", err)
	}

	// Check file format by reading magic bytes
	header := make([]byte, 4)
	if _, err := readFile.Read(header); err != nil {
		return "", fmt.Errorf("failed to read file header: %w", err)
	}

	// Check if it's an old .doc format (CFB: D0 CF 11 E0)
	if header[0] == 0xD0 && header[1] == 0xCF && header[2] == 0x11 && header[3] == 0xE0 {
		return "", fmt.Errorf("file is in old Word 97-2003 (.doc) format, please convert to modern .docx format")
	}

	// Reset file pointer to beginning
	if _, err := readFile.Seek(0, 0); err != nil {
		return "", fmt.Errorf("failed to reset file pointer: %w", err)
	}

	gooxmlContent, gooxmlErr := e.parseDocxWithGooxml(ctx, docxPath)
	if gooxmlErr == nil {
		return gooxmlContent, nil
	}

	size := fileInfo.Size()
	doc, err := docx.Parse(readFile, size)
	if err != nil {
		return "", fmt.Errorf("failed to parse docx file: %w; gooxml error: %v", err, gooxmlErr)
	}

	var content []string
	// TODO: _extract_images_from_docx
	for _, item := range doc.Document.Body.Items {
		switch v := item.(type) {
		case *docx.Paragraph:
			if text := e.parseParagraph(v); text != "" {
				content = append(content, text)
			} else {
				content = append(content, "\n")
			}
		case *docx.Table:
			if markdown := e.tableToMarkdown(v); markdown != "" {
				content = append(content, markdown)
			}
		}
	}

	return strings.Join(content, "\n"), nil
}

func (e *WordExtractor) parseParagraph(para *docx.Paragraph) string {
	if para == nil {
		return ""
	}

	var texts []string

	text := para.String()
	if text != "" {
		texts = append(texts, text)
	}

	return strings.Join(texts, " ")
}

func (e *WordExtractor) tableToMarkdown(table *docx.Table) string {
	if table == nil {
		return ""
	}

	var markdownRows []string

	tableStr := table.String()
	if tableStr != "" {
		lines := strings.Split(tableStr, "\n")
		for _, line := range lines {
			if trimmed := strings.TrimSpace(line); trimmed != "" {
				markdownRows = append(markdownRows, trimmed)
			}
		}
	}

	return strings.Join(markdownRows, "\n")
}
