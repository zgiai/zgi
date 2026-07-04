package handler

import (
	"bytes"
	"encoding/csv"
	"fmt"
	"io"
	"path/filepath"
	"strings"

	"github.com/extrame/xls"
	"github.com/gin-gonic/gin"
	"github.com/xuri/excelize/v2"

	file_model "github.com/zgiai/zgi/api/internal/modules/file_process/model"
	"github.com/zgiai/zgi/api/pkg/response"
)

const (
	spreadsheetPreviewMaxRows    = 200
	spreadsheetPreviewMaxColumns = 100
)

type spreadsheetPreviewResponse struct {
	Engine string                    `json:"engine"`
	Sheets []spreadsheetPreviewSheet `json:"sheets"`
}

type spreadsheetPreviewSheet struct {
	Name          string                  `json:"name"`
	Rows          []spreadsheetPreviewRow `json:"rows"`
	ColumnCount   int                     `json:"columnCount"`
	TotalRowCount int                     `json:"totalRowCount"`
}

type spreadsheetPreviewRow struct {
	Number int      `json:"number"`
	Cells  []string `json:"cells"`
}

// GetFileSpreadsheetPreview returns a bounded table preview for spreadsheet files.
// GET /files/:file_id/spreadsheet-preview
func (h *FileHandler) GetFileSpreadsheetPreview(c *gin.Context) {
	fileID := c.Param("file_id")
	if fileID == "" {
		h.businessError(c, response.ErrFileIdRequired)
		return
	}

	uploadFile, ok := h.getAuthorizedFileForDownload(c, fileID)
	if !ok {
		return
	}

	content, err := h.fileService.DownloadFile(c.Request.Context(), fileID)
	if err != nil {
		switch err {
		case file_model.ErrFileNotFound:
			h.businessError(c, response.ErrFileNotFound)
		case file_model.ErrUnsupportedFileType:
			h.businessError(c, response.ErrUnsupportedFileType)
		default:
			h.businessError(c, response.ErrFilePreviewFailed)
		}
		return
	}

	ext := normalizedSpreadsheetPreviewExt(uploadFile.Name, uploadFile.Extension)
	preview, err := buildSpreadsheetPreview(uploadFile.Name, content, ext)
	if err != nil {
		h.businessErrorWithMessage(c, response.ErrFilePreviewFailed, err.Error())
		return
	}
	response.Success(c, preview)
}

func normalizedSpreadsheetPreviewExt(fileName, extension string) string {
	ext := strings.ToLower(strings.TrimSpace(extension))
	if ext == "" {
		ext = strings.ToLower(filepath.Ext(fileName))
	}
	return strings.TrimPrefix(ext, ".")
}

func buildSpreadsheetPreview(fileName string, data []byte, ext string) (*spreadsheetPreviewResponse, error) {
	switch ext {
	case "xlsx":
		sheets, err := buildXLSXSpreadsheetPreview(data)
		if err != nil {
			return nil, err
		}
		return &spreadsheetPreviewResponse{Engine: "excelize", Sheets: sheets}, nil
	case "xls":
		sheets, err := buildXLSSpreadsheetPreview(data)
		if err != nil {
			return nil, err
		}
		return &spreadsheetPreviewResponse{Engine: "xls", Sheets: sheets}, nil
	case "csv":
		sheet, err := buildDelimitedSpreadsheetPreview(fileName, data, ',')
		if err != nil {
			return nil, err
		}
		return &spreadsheetPreviewResponse{Engine: "csv", Sheets: []spreadsheetPreviewSheet{sheet}}, nil
	case "tsv":
		sheet, err := buildDelimitedSpreadsheetPreview(fileName, data, '\t')
		if err != nil {
			return nil, err
		}
		return &spreadsheetPreviewResponse{Engine: "tsv", Sheets: []spreadsheetPreviewSheet{sheet}}, nil
	default:
		return nil, fmt.Errorf("unsupported spreadsheet preview type: %s", ext)
	}
}

func buildXLSXSpreadsheetPreview(data []byte) ([]spreadsheetPreviewSheet, error) {
	f, err := excelize.OpenReader(bytes.NewReader(data))
	if err != nil {
		return nil, fmt.Errorf("open xlsx: %w", err)
	}
	defer f.Close()

	sheets := make([]spreadsheetPreviewSheet, 0)
	for _, sheetName := range f.GetSheetList() {
		rows, err := f.GetRows(sheetName)
		if err != nil {
			continue
		}
		sheets = append(sheets, previewSheetFromRows(sheetName, rows))
	}
	return sheets, nil
}

func buildXLSSpreadsheetPreview(data []byte) ([]spreadsheetPreviewSheet, error) {
	wb, err := xls.OpenReader(bytes.NewReader(data), "utf-8")
	if err != nil {
		return nil, fmt.Errorf("open xls: %w", err)
	}

	sheets := make([]spreadsheetPreviewSheet, 0)
	for sheetIndex := 0; sheetIndex < wb.NumSheets(); sheetIndex++ {
		sheet := wb.GetSheet(sheetIndex)
		if sheet == nil {
			continue
		}
		rows := make([][]string, 0, int(sheet.MaxRow)+1)
		for rowIndex := 0; rowIndex <= int(sheet.MaxRow); rowIndex++ {
			row := sheet.Row(rowIndex)
			if row == nil {
				rows = append(rows, nil)
				continue
			}
			cells := make([]string, row.LastCol())
			for colIndex := 0; colIndex < row.LastCol(); colIndex++ {
				cells[colIndex] = row.Col(colIndex)
			}
			rows = append(rows, cells)
		}
		sheets = append(sheets, previewSheetFromRows(sheet.Name, rows))
	}
	return sheets, nil
}

func buildDelimitedSpreadsheetPreview(fileName string, data []byte, comma rune) (spreadsheetPreviewSheet, error) {
	reader := csv.NewReader(bytes.NewReader(data))
	reader.Comma = comma
	reader.FieldsPerRecord = -1
	reader.TrimLeadingSpace = true

	rows := make([][]string, 0)
	for {
		record, err := reader.Read()
		if err != nil {
			if err == io.EOF {
				break
			}
			return spreadsheetPreviewSheet{}, fmt.Errorf("read delimited row: %w", err)
		}
		rows = append(rows, record)
	}
	name := strings.TrimSpace(fileName)
	if name == "" {
		name = "Sheet1"
	}
	return previewSheetFromRows(name, rows), nil
}

func previewSheetFromRows(name string, rows [][]string) spreadsheetPreviewSheet {
	if strings.TrimSpace(name) == "" {
		name = "Sheet"
	}

	out := spreadsheetPreviewSheet{
		Name:          name,
		Rows:          make([]spreadsheetPreviewRow, 0, minInt(len(rows), spreadsheetPreviewMaxRows)),
		TotalRowCount: len(rows),
	}
	for rowIndex, row := range rows {
		out.ColumnCount = minInt(maxInt(out.ColumnCount, len(row)), spreadsheetPreviewMaxColumns)
		if rowIndex >= spreadsheetPreviewMaxRows {
			continue
		}
		cells := make([]string, spreadsheetPreviewMaxColumns)
		for colIndex := 0; colIndex < len(row) && colIndex < spreadsheetPreviewMaxColumns; colIndex++ {
			cells[colIndex] = strings.TrimSpace(row[colIndex])
		}
		out.Rows = append(out.Rows, spreadsheetPreviewRow{
			Number: rowIndex + 1,
			Cells:  cells,
		})
	}
	return out
}

func minInt(a, b int) int {
	if a < b {
		return a
	}
	return b
}

func maxInt(a, b int) int {
	if a > b {
		return a
	}
	return b
}
