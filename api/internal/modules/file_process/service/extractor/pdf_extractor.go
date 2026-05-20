package extractor

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"strings"

	"github.com/zgiai/zgi/api/internal/dto"

	"github.com/ledongthuc/pdf"
)

type PdfExtractor struct {
	filePath string
}

func NewPdfExtractor(filePath string) *PdfExtractor {
	return &PdfExtractor{
		filePath: filePath,
	}
}

func (e *PdfExtractor) Extract(ctx context.Context) (*dto.ExtractOutput, error) {
	documents, err := e.load(ctx)
	if err != nil {
		return nil, fmt.Errorf("error loading %s: %w", e.filePath, err)
	}

	return dto.NewExtractOutputFromDocuments("zgi:pdf", documents), nil
}

func (e *PdfExtractor) load(ctx context.Context) ([]dto.Document, error) {
	if _, err := os.Stat(e.filePath); os.IsNotExist(err) {
		return nil, fmt.Errorf("file does not exist: %s", e.filePath)
	}

	pdf.DebugOn = false

	file, pdfReader, err := pdf.Open(e.filePath)
	if err != nil {
		return nil, fmt.Errorf("failed to open PDF file: %w", err)
	}
	defer file.Close()

	numPages := pdfReader.NumPage()

	var documents []dto.Document

	for pageIndex := 1; pageIndex <= numPages; pageIndex++ {
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		default:
		}
		page := pdfReader.Page(pageIndex)

		if page.V.IsNull() || page.V.Key("Contents").Kind() == pdf.Null {
			continue
		}

		rows, err := page.GetTextByRow()
		if err != nil {
			continue
		}

		var pageText strings.Builder
		for _, row := range rows {
			for _, word := range row.Content {
				pageText.WriteString(word.S)
			}
			pageText.WriteString("\n")
		}

		content := strings.TrimSpace(pageText.String())
		if content != "" {
			doc := dto.Document{
				PageContent: content,
				Metadata: map[string]interface{}{
					"source": e.filePath,
					"page":   pageIndex - 1,
				},
			}
			documents = append(documents, doc)
		}
	}

	if len(documents) == 0 {
		text, err := getPlainText(pdfReader)
		if err != nil {
			return nil, fmt.Errorf("failed to extract text from PDF")
		}

		content := strings.TrimSpace(text)
		if content != "" {
			doc := dto.Document{
				PageContent: content,
				Metadata: map[string]interface{}{
					"source": e.filePath,
				},
			}
			documents = append(documents, doc)
		} else {
			return nil, fmt.Errorf("failed to extract text from PDF")
		}
	}

	return documents, nil
}

func getPlainText(reader *pdf.Reader) (string, error) {
	text, err := reader.GetPlainText()
	if err != nil {
		return "", err
	}

	var buf bytes.Buffer
	buf.ReadFrom(text)
	return buf.String(), nil
}
