package filegenerator

import (
	"archive/zip"
	"bytes"
	"context"
	"net/url"
	"os"
	"strings"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/pdfcpu/pdfcpu/pkg/api"
	"github.com/stretchr/testify/require"
	"github.com/xuri/excelize/v2"
	"github.com/zgiai/zgi/api/config"
	workflowtoolfile "github.com/zgiai/zgi/api/internal/modules/app/workflow/tool_file"
	"github.com/zgiai/zgi/api/internal/modules/tools"
	"github.com/zgiai/zgi/api/pkg/storage"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
)

func TestResolveFormatSupportsOfficeAndPDF(t *testing.T) {
	tests := []struct {
		raw      string
		wantFmt  string
		wantExt  string
		wantMIME string
	}{
		{
			raw:      "docx",
			wantFmt:  "docx",
			wantExt:  ".docx",
			wantMIME: "application/vnd.openxmlformats-officedocument.wordprocessingml.document",
		},
		{
			raw:      "word",
			wantFmt:  "docx",
			wantExt:  ".docx",
			wantMIME: "application/vnd.openxmlformats-officedocument.wordprocessingml.document",
		},
		{
			raw:      "xlsx",
			wantFmt:  "xlsx",
			wantExt:  ".xlsx",
			wantMIME: "application/vnd.openxmlformats-officedocument.spreadsheetml.sheet",
		},
		{
			raw:      "excel",
			wantFmt:  "xlsx",
			wantExt:  ".xlsx",
			wantMIME: "application/vnd.openxmlformats-officedocument.spreadsheetml.sheet",
		},
		{
			raw:      "pdf",
			wantFmt:  "pdf",
			wantExt:  ".pdf",
			wantMIME: "application/pdf",
		},
	}

	for _, tt := range tests {
		t.Run(tt.raw, func(t *testing.T) {
			gotFmt, gotSpec, err := resolveFormat(tt.raw)
			require.NoError(t, err)
			require.Equal(t, tt.wantFmt, gotFmt)
			require.Equal(t, tt.wantExt, gotSpec.extension)
			require.Equal(t, tt.wantMIME, gotSpec.mimeType)
		})
	}
}

func TestRenderContentGeneratesValidOfficeAndPDF(t *testing.T) {
	t.Run("docx", func(t *testing.T) {
		data, err := renderContent("Hello\n中文", "docx", "Report")
		require.NoError(t, err)
		requireZipEntryContains(t, data, "word/document.xml", "Hello")
		requireZipEntryContains(t, data, "word/document.xml", "中文")
	})

	t.Run("xlsx", func(t *testing.T) {
		data, err := renderContent("Name,Score\n中文,10\n", "xlsx", "Report")
		require.NoError(t, err)

		workbook, err := excelize.OpenReader(bytes.NewReader(data))
		require.NoError(t, err)
		defer workbook.Close()

		rows, err := workbook.GetRows("Sheet1")
		require.NoError(t, err)
		require.Equal(t, [][]string{{"Name", "Score"}, {"中文", "10"}}, rows)
	})

	t.Run("pdf", func(t *testing.T) {
		data, err := renderContent("中文 PDF", "pdf", "报告")
		require.NoError(t, err)
		require.True(t, bytes.HasPrefix(data, []byte("%PDF-")))
		require.Contains(t, string(data), "4E2D6587")
		require.NoError(t, api.Validate(bytes.NewReader(data), nil))
	})
}

func TestGenerateFileToolReturnsDownloadableOfficeFileMetadata(t *testing.T) {
	db, mock, cleanup := openFileGeneratorMockDB(t)
	defer cleanup()
	mock.ExpectBegin()
	mock.ExpectExec(`INSERT INTO "tool_files"`).
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectCommit()

	oldManager := workflowtoolfile.GlobalToolFileManager
	oldSignature := workflowtoolfile.GlobalFileSignature
	t.Cleanup(func() {
		workflowtoolfile.GlobalToolFileManager = oldManager
		workflowtoolfile.GlobalFileSignature = oldSignature
	})

	fileStorage := newMemoryStorage()
	workflowtoolfile.GlobalToolFileManager = workflowtoolfile.NewToolFileManager(db, fileStorage)
	workflowtoolfile.GlobalFileSignature = workflowtoolfile.NewFileSignature(&config.Config{
		App: config.AppConfig{
			SecretKey:          "test-secret-key",
			FilesURL:           "http://files.example.test",
			FilesAccessTimeout: 3600,
		},
	})

	messages, err := NewGenerateFileTool("tenant-1").Invoke(
		context.Background(),
		"user-1",
		map[string]interface{}{
			"content":  "Name,Score\nAlice,10\n",
			"format":   "xlsx",
			"filename": "report",
		},
		nil,
		nil,
		nil,
	)
	require.NoError(t, err)
	require.Len(t, messages, 2)
	require.Equal(t, tools.ToolInvokeMessageTypeFile, messages[0].Type)

	require.Equal(t, tools.ToolInvokeMessageTypeJSON, messages[1].Type)
	jsonPayload := messages[1].Data
	require.Equal(t, "report.xlsx", jsonPayload["filename"])
	require.Equal(t, "xlsx", jsonPayload["format"])
	require.Equal(t, "application/vnd.openxmlformats-officedocument.spreadsheetml.sheet", jsonPayload["mime_type"])
	require.NotEmpty(t, jsonPayload["file_id"])
	require.NotEmpty(t, jsonPayload["url"])
	require.NotEmpty(t, jsonPayload["download_url"])

	parsed, err := url.Parse(jsonPayload["download_url"].(string))
	require.NoError(t, err)
	require.Equal(t, "1", parsed.Query().Get("download"))

	fileID := jsonPayload["file_id"].(string)
	require.NotEmpty(t, fileID)
	data := fileStorage.onlyFileData(t)
	require.NotEmpty(t, data)

	workbook, err := excelize.OpenReader(bytes.NewReader(data))
	require.NoError(t, err)
	defer workbook.Close()

	require.NoError(t, mock.ExpectationsWereMet())
}

type memoryStorage struct {
	files map[string][]byte
}

func newMemoryStorage() *memoryStorage {
	return &memoryStorage{files: make(map[string][]byte)}
}

func (s *memoryStorage) Save(filename string, data []byte) error {
	s.files[filename] = append([]byte(nil), data...)
	return nil
}

func (s *memoryStorage) Load(filename string) ([]byte, error) {
	data, ok := s.files[filename]
	if !ok {
		return nil, os.ErrNotExist
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
	return nil
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
	return nil, nil
}

func (s *memoryStorage) onlyFileData(t *testing.T) []byte {
	t.Helper()
	require.Len(t, s.files, 1)
	for _, data := range s.files {
		return append([]byte(nil), data...)
	}
	return nil
}

func openFileGeneratorMockDB(t *testing.T) (*gorm.DB, sqlmock.Sqlmock, func()) {
	t.Helper()

	sqlDB, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherRegexp))
	require.NoError(t, err)

	db, err := gorm.Open(postgres.New(postgres.Config{
		Conn:                 sqlDB,
		PreferSimpleProtocol: true,
	}), &gorm.Config{})
	require.NoError(t, err)
	mock.MatchExpectationsInOrder(false)

	return db, mock, func() {
		_ = sqlDB.Close()
	}
}

func requireZipEntryContains(t *testing.T, data []byte, entryName string, want string) {
	t.Helper()

	reader, err := zip.NewReader(bytes.NewReader(data), int64(len(data)))
	require.NoError(t, err)

	for _, file := range reader.File {
		if file.Name != entryName {
			continue
		}
		handle, err := file.Open()
		require.NoError(t, err)
		defer handle.Close()

		var buf bytes.Buffer
		_, err = buf.ReadFrom(handle)
		require.NoError(t, err)
		require.Contains(t, buf.String(), want)
		return
	}

	names := make([]string, 0, len(reader.File))
	for _, file := range reader.File {
		names = append(names, file.Name)
	}
	require.Failf(t, "missing zip entry", "entry %s not found in %s", entryName, strings.Join(names, ", "))
}
