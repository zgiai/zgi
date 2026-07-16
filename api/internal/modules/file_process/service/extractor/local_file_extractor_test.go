package extractor

import (
	"bytes"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/xuri/excelize/v2"
	"github.com/zgiai/zgi/api/internal/modules/file_process/model"
	"github.com/zgiai/zgi/api/pkg/storage"
)

func TestExcelExtractorExtractsXlsxTextContent(t *testing.T) {
	filePath := filepath.Join(t.TempDir(), "people.xlsx")
	const sheetName = "People"

	workbook := excelize.NewFile()
	defer workbook.Close()

	if err := workbook.SetSheetName("Sheet1", sheetName); err != nil {
		t.Fatalf("SetSheetName() error = %v", err)
	}

	rows := [][]string{
		{"Name", "Role", "City"},
		{"Ada Lovelace", "Engineer", "London"},
		{"Grace Hopper", "Admiral", "Arlington"},
	}
	for rowIndex, row := range rows {
		for colIndex, value := range row {
			cell, err := excelize.CoordinatesToCellName(colIndex+1, rowIndex+1)
			if err != nil {
				t.Fatalf("CoordinatesToCellName(%d, %d) error = %v", colIndex+1, rowIndex+1, err)
			}
			if err := workbook.SetCellValue(sheetName, cell, value); err != nil {
				t.Fatalf("SetCellValue(%s, %s) error = %v", cell, value, err)
			}
		}
	}
	if err := workbook.SaveAs(filePath); err != nil {
		t.Fatalf("SaveAs(%s) error = %v", filePath, err)
	}

	output, err := NewExcelExtractor(filePath).Extract(t.Context())
	if err != nil {
		t.Fatalf("Extract() error = %v", err)
	}

	if output.Source != "zgi:excel" {
		t.Fatalf("output.Source = %q, want %q", output.Source, "zgi:excel")
	}
	if len(output.Elements) != 2 {
		t.Fatalf("len(output.Elements) = %d, want %d; output=%+v", len(output.Elements), 2, output)
	}

	expectedFirstRow := `"Name":"Ada Lovelace";"Role":"Engineer";"City":"London"`
	if output.Elements[0].Type != "table" {
		t.Fatalf("output.Elements[0].Type = %q, want %q", output.Elements[0].Type, "table")
	}
	if output.Elements[0].Content != expectedFirstRow {
		t.Fatalf("output.Elements[0].Content = %q, want %q", output.Elements[0].Content, expectedFirstRow)
	}
	if !strings.Contains(output.Markdown, expectedFirstRow) {
		t.Fatalf("output.Markdown = %q, want content containing %q", output.Markdown, expectedFirstRow)
	}
	if got := output.Elements[0].Metadata["sheet"]; got != sheetName {
		t.Fatalf("output.Elements[0].Metadata[\"sheet\"] = %v, want %q", got, sheetName)
	}
}

func TestPdfExtractorExtractsTextLayerContent(t *testing.T) {
	filePath := filepath.Join(t.TempDir(), "text-layer.pdf")
	expectedLines := []string{"ZGI-PDF-alpha", "Second-line-context"}
	writeTextLayerPDF(t, filePath, expectedLines)

	output, err := NewPdfExtractor(filePath).Extract(t.Context())
	if err != nil {
		t.Fatalf("Extract() error = %v", err)
	}

	if output.Source != "zgi:pdf" {
		t.Fatalf("output.Source = %q, want %q", output.Source, "zgi:pdf")
	}
	if len(output.Elements) != 1 {
		t.Fatalf("len(output.Elements) = %d, want %d; output=%+v", len(output.Elements), 1, output)
	}
	if output.Elements[0].Type != "text" {
		t.Fatalf("output.Elements[0].Type = %q, want %q", output.Elements[0].Type, "text")
	}

	extractedText := strings.Join(strings.Fields(output.Markdown), " ")
	for _, expected := range expectedLines {
		if !strings.Contains(extractedText, expected) {
			t.Fatalf("output.Markdown = %q, want content containing %q", output.Markdown, expected)
		}
	}
	if got := output.Elements[0].Metadata["source"]; got != filePath {
		t.Fatalf("output.Elements[0].Metadata[\"source\"] = %v, want %q", got, filePath)
	}
}

func TestExtractProcessorLoadFromUploadFileUsesMetadataExtensionWhenStorageKeyHasNoSuffix(t *testing.T) {
	key := "tenant/uploads/object-without-extension"
	workbookData := buildXLSXBytes(t, "Costs", [][]string{
		{"Item", "Amount"},
		{"Electricity", "128.50"},
	})

	processor := NewExtractProcessor(newMemoryStorage(map[string][]byte{key: workbookData}))
	uploadFile := &model.UploadFile{
		Key:       key,
		Name:      "utility-confirmation.xlsx",
		Extension: "xlsx",
		MimeType:  "application/vnd.openxmlformats-officedocument.spreadsheetml.sheet",
	}

	output, text, err := processor.LoadFromUploadFile(t.Context(), uploadFile, true, false)
	if err != nil {
		t.Fatalf("LoadFromUploadFile() error = %v", err)
	}
	if output.Source != "zgi:excel" {
		t.Fatalf("output.Source = %q, want %q", output.Source, "zgi:excel")
	}

	expected := `"Item":"Electricity";"Amount":"128.50"`
	if !strings.Contains(text, expected) {
		t.Fatalf("text = %q, want content containing %q", text, expected)
	}
}

func TestOrderedExtractionStrategiesStartsWithRequestedAvailableStrategy(t *testing.T) {
	strategies := orderedExtractionStrategies("local", []string{"mineru", "local", "unstructured"})

	if len(strategies) == 0 || strategies[0] != "local" {
		t.Fatalf("strategies = %#v, want requested local first", strategies)
	}
	if len(strategies) != 3 {
		t.Fatalf("strategies len = %d, want 3 without duplicates: %#v", len(strategies), strategies)
	}
}

func TestExtractProcessorLoadFromUploadFileUsesPDFMetadataExtensionWhenStorageKeyHasNoSuffix(t *testing.T) {
	key := "tenant/uploads/pdf-object-without-extension"
	filePath := filepath.Join(t.TempDir(), "source.pdf")
	expectedLines := []string{"ZGI-PDF-storage-alpha", "ZGI-PDF-storage-beta"}
	writeTextLayerPDF(t, filePath, expectedLines)
	pdfData, err := os.ReadFile(filePath)
	if err != nil {
		t.Fatalf("ReadFile(%s) error = %v", filePath, err)
	}

	processor := NewExtractProcessor(newMemoryStorage(map[string][]byte{key: pdfData}))
	uploadFile := &model.UploadFile{
		Key:       key,
		Name:      "confirmation.pdf",
		Extension: "pdf",
		MimeType:  "application/pdf",
	}

	output, text, err := processor.LoadFromUploadFile(t.Context(), uploadFile, true, false)
	if err != nil {
		t.Fatalf("LoadFromUploadFile() error = %v", err)
	}
	if output.Source != "zgi:pdf" {
		t.Fatalf("output.Source = %q, want %q", output.Source, "zgi:pdf")
	}

	extractedText := strings.Join(strings.Fields(text), " ")
	for _, expected := range expectedLines {
		if !strings.Contains(extractedText, expected) {
			t.Fatalf("text = %q, want content containing %q", text, expected)
		}
	}
}

func TestUploadFileExtractionExtensionUsesBestAvailableMetadata(t *testing.T) {
	tests := []struct {
		name       string
		uploadFile *model.UploadFile
		want       string
	}{
		{
			name:       "storage key extension wins",
			uploadFile: &model.UploadFile{Key: "tenant/report.PDF", Extension: "xlsx", Name: "report.xlsx", MimeType: "application/vnd.openxmlformats-officedocument.spreadsheetml.sheet"},
			want:       ".pdf",
		},
		{
			name:       "extension field without dot",
			uploadFile: &model.UploadFile{Key: "tenant/report", Extension: "XLSX", Name: "report", MimeType: "application/pdf"},
			want:       ".xlsx",
		},
		{
			name:       "display name fallback",
			uploadFile: &model.UploadFile{Key: "tenant/report", Name: "report.csv"},
			want:       ".csv",
		},
		{
			name:       "mime type fallback",
			uploadFile: &model.UploadFile{Key: "tenant/report", MimeType: "application/pdf"},
			want:       ".pdf",
		},
		{
			name:       "no metadata",
			uploadFile: &model.UploadFile{Key: "tenant/report"},
			want:       "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := uploadFileExtractionExtension(tt.uploadFile); got != tt.want {
				t.Fatalf("uploadFileExtractionExtension() = %q, want %q", got, tt.want)
			}
		})
	}
}

func writeTextLayerPDF(t *testing.T, filePath string, lines []string) {
	t.Helper()

	var content strings.Builder
	content.WriteString("BT\n/F1 18 Tf\n72 720 Td\n")
	for i, line := range lines {
		if i > 0 {
			content.WriteString("0 -24 Td\n")
		}
		content.WriteString("(")
		content.WriteString(escapePDFLiteral(line))
		content.WriteString(") Tj\n")
	}
	content.WriteString("ET\n")

	contentStream := content.String()
	objects := []string{
		"<< /Type /Catalog /Pages 2 0 R >>",
		"<< /Type /Pages /Kids [3 0 R] /Count 1 >>",
		"<< /Type /Page /Parent 2 0 R /MediaBox [0 0 612 792] /Resources << /Font << /F1 4 0 R >> >> /Contents 5 0 R >>",
		"<< /Type /Font /Subtype /Type1 /BaseFont /Helvetica >>",
		fmt.Sprintf("<< /Length %d >>\nstream\n%s\nendstream", len(contentStream), contentStream),
	}

	var pdf bytes.Buffer
	pdf.WriteString("%PDF-1.4\n")
	offsets := make([]int, len(objects)+1)
	for i, object := range objects {
		objectNumber := i + 1
		offsets[objectNumber] = pdf.Len()
		fmt.Fprintf(&pdf, "%d 0 obj\n%s\nendobj\n", objectNumber, object)
	}

	startXref := pdf.Len()
	fmt.Fprintf(&pdf, "xref\n0 %d\n", len(objects)+1)
	pdf.WriteString("0000000000 65535 f \n")
	for objectNumber := 1; objectNumber <= len(objects); objectNumber++ {
		fmt.Fprintf(&pdf, "%010d 00000 n \n", offsets[objectNumber])
	}
	fmt.Fprintf(&pdf, "trailer\n<< /Size %d /Root 1 0 R >>\nstartxref\n%d\n%%%%EOF\n", len(objects)+1, startXref)

	if err := os.WriteFile(filePath, pdf.Bytes(), 0o600); err != nil {
		t.Fatalf("WriteFile(%s) error = %v", filePath, err)
	}
}

func escapePDFLiteral(s string) string {
	replacer := strings.NewReplacer(`\`, `\\`, "(", `\(`, ")", `\)`)
	return replacer.Replace(s)
}

func buildXLSXBytes(t *testing.T, sheetName string, rows [][]string) []byte {
	t.Helper()

	workbook := excelize.NewFile()
	defer workbook.Close()

	if err := workbook.SetSheetName("Sheet1", sheetName); err != nil {
		t.Fatalf("SetSheetName() error = %v", err)
	}
	for rowIndex, row := range rows {
		for colIndex, value := range row {
			cell, err := excelize.CoordinatesToCellName(colIndex+1, rowIndex+1)
			if err != nil {
				t.Fatalf("CoordinatesToCellName(%d, %d) error = %v", colIndex+1, rowIndex+1, err)
			}
			if err := workbook.SetCellValue(sheetName, cell, value); err != nil {
				t.Fatalf("SetCellValue(%s, %s) error = %v", cell, value, err)
			}
		}
	}

	buffer, err := workbook.WriteToBuffer()
	if err != nil {
		t.Fatalf("WriteToBuffer() error = %v", err)
	}
	return buffer.Bytes()
}

type memoryStorage struct {
	files map[string][]byte
}

func newMemoryStorage(files map[string][]byte) *memoryStorage {
	copied := make(map[string][]byte, len(files))
	for filename, data := range files {
		copied[filename] = append([]byte(nil), data...)
	}
	return &memoryStorage{files: copied}
}

func (s *memoryStorage) Save(filename string, data []byte) error {
	s.files[filename] = append([]byte(nil), data...)
	return nil
}

func (s *memoryStorage) Load(filename string) ([]byte, error) {
	data, ok := s.files[filename]
	if !ok {
		return nil, errors.New("file not found")
	}
	return append([]byte(nil), data...), nil
}

func (s *memoryStorage) LoadStream(filename string) (<-chan []byte, error) {
	data, err := s.Load(filename)
	if err != nil {
		return nil, err
	}
	ch := make(chan []byte, 1)
	ch <- data
	close(ch)
	return ch, nil
}

func (s *memoryStorage) Download(filename string, targetPath string) error {
	data, err := s.Load(filename)
	if err != nil {
		return err
	}
	return os.WriteFile(targetPath, data, 0o600)
}

func (s *memoryStorage) Exists(filename string) (bool, error) {
	_, ok := s.files[filename]
	return ok, nil
}

func (s *memoryStorage) Delete(filename string) error {
	delete(s.files, filename)
	return nil
}

func (s *memoryStorage) List(prefix string) ([]storage.FileInfo, error) {
	infos := make([]storage.FileInfo, 0)
	for filename, data := range s.files {
		if !strings.HasPrefix(filename, prefix) {
			continue
		}
		infos = append(infos, storage.FileInfo{
			Key:          filename,
			Size:         int64(len(data)),
			LastModified: time.Unix(0, 0),
		})
	}
	return infos, nil
}

var _ storage.Storage = (*memoryStorage)(nil)
