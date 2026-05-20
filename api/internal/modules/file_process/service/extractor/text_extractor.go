package extractor

import (
	"context"
	"fmt"
	"os"

	"github.com/zgiai/zgi/api/internal/dto"
	"github.com/zgiai/zgi/api/pkg/logger"
)

type TextExtractor struct {
	filePath           string
	encoding           string
	autodetectEncoding bool
}

func NewTextExtractor(filePath string, encoding string, autodetectEncoding bool) *TextExtractor {
	return &TextExtractor{
		filePath:           filePath,
		encoding:           encoding,
		autodetectEncoding: autodetectEncoding,
	}
}

func (e *TextExtractor) Extract(ctx context.Context) (*dto.ExtractOutput, error) {
	text, err := e.loadFromFile()
	if err != nil {
		return nil, fmt.Errorf("error loading %s: %w", e.filePath, err)
	}

	metadata := map[string]interface{}{"source": e.filePath}
	documents := []dto.Document{
		{
			PageContent: text,
			Metadata:    metadata,
		},
	}
	return dto.NewExtractOutputFromDocuments("zgi:text", documents), nil
}

func (e *TextExtractor) loadFromFile() (string, error) {
	if _, err := os.Stat(e.filePath); os.IsNotExist(err) {
		return "", fmt.Errorf("file does not exist: %s", e.filePath)
	}

	text, err := e.readFileWithEncoding(e.encoding)
	if err != nil {
		if e.isUnicodeDecodeError(err) && e.autodetectEncoding {
			// TODO: autodetect Encoding
			// temp return error
			return "", fmt.Errorf("unicode decode error and autodetect not implemented: %w", err)
		}
		return "", err
	}

	return text, nil
}

func (e *TextExtractor) readFileWithEncoding(encoding string) (string, error) {
	data, err := os.ReadFile(e.filePath)
	if err != nil {
		return "", err
	}

	// TODO: use golang.org/x/text/encoding for other encodings
	if encoding != "" && encoding != "utf-8" {
		logger.Warn("encoding conversion not implemented, using raw data", "encoding", encoding)
	}

	return string(data), nil
}

func (e *TextExtractor) isUnicodeDecodeError(err error) bool {
	// TODO: check Unicode error
	return err != nil
}

func detectFileEncodings(filePath string) []string {
	// TODO: use github.com/saintfish/chardet check encoding
	return []string{"utf-8", "gbk", "gb18030"}
}
