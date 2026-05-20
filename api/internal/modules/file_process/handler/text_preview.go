package handler

import (
	"bytes"
	"fmt"
	"strings"
	"unicode/utf8"

	"github.com/zgiai/zgi/api/internal/dto"
	"golang.org/x/text/encoding/simplifiedchinese"
	"golang.org/x/text/encoding/unicode"
)

const maxTextOriginalPreviewBytes = 10 * 1024 * 1024

var (
	utf8BOM    = []byte{0xEF, 0xBB, 0xBF}
	utf16BOMLE = []byte{0xFF, 0xFE}
	utf16BOMBE = []byte{0xFE, 0xFF}
)

func isTextOriginalPreviewFile(uploadFile *dto.UploadFile) bool {
	extension := strings.ToLower(strings.TrimPrefix(uploadFile.Extension, "."))
	if isTextOriginalPreviewExtension(extension) {
		return true
	}

	mimeType := strings.ToLower(uploadFile.MimeType)
	return isTextOriginalPreviewMIMEType(mimeType)
}

func normalizeTextPreviewContent(content []byte, uploadFile *dto.UploadFile) ([]byte, string, error) {
	if len(content) > maxTextOriginalPreviewBytes {
		return nil, "", fmt.Errorf("text preview exceeds size limit")
	}

	normalized, err := normalizeTextPreviewEncoding(content)
	if err != nil {
		return nil, "", err
	}

	return normalized, textPreviewContentType(uploadFile), nil
}

func normalizeTextPreviewEncoding(content []byte) ([]byte, error) {
	if bytes.HasPrefix(content, utf8BOM) {
		return content[len(utf8BOM):], nil
	}
	if utf8.Valid(content) {
		return content, nil
	}
	if bytes.HasPrefix(content, utf16BOMLE) || bytes.HasPrefix(content, utf16BOMBE) {
		return unicode.UTF16(unicode.LittleEndian, unicode.ExpectBOM).NewDecoder().Bytes(content)
	}

	decoded, err := simplifiedchinese.GB18030.NewDecoder().Bytes(content)
	if err != nil {
		return nil, fmt.Errorf("failed to decode text preview: %w", err)
	}
	if !utf8.Valid(decoded) {
		return nil, fmt.Errorf("decoded text preview is not utf-8")
	}

	return decoded, nil
}

func textPreviewContentType(uploadFile *dto.UploadFile) string {
	extension := strings.ToLower(strings.TrimPrefix(uploadFile.Extension, "."))
	switch extension {
	case "csv":
		return "text/csv; charset=utf-8"
	case "xml":
		return "application/xml; charset=utf-8"
	default:
		return "text/plain; charset=utf-8"
	}
}
