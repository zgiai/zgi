package service

import (
	"encoding/json"
	"fmt"
	"regexp"
	"strconv"
	"strings"

	"github.com/zgiai/zgi/api/internal/dto"
)

const (
	fileIngestNormalizationValid      = "valid"
	fileIngestNormalizationNormalized = "normalized"
	fileIngestNormalizationInvalid    = "invalid"
	fileIngestNormalizationEmpty      = "empty"
)

var (
	fileIngestNumericTokenPattern = regexp.MustCompile(`[-+]?(?:\d{1,3}(?:,\d{3})+|\d+)(?:\.\d+)?`)
	fileIngestChineseDatePattern  = regexp.MustCompile(`^(\d{4})\s*年\s*(\d{1,2})\s*月\s*(\d{1,2})\s*日?$`)
)

type fileConversionColumnSchema struct {
	ColumnID         string `json:"column_id"`
	FieldName        string `json:"field_name"`
	DisplayName      string `json:"display_name,omitempty"`
	SourceColumnName string `json:"source_column_name,omitempty"`
	Type             string `json:"type"`
	Description      string `json:"description,omitempty"`
	Required         bool   `json:"required"`
}

type fileConversionLLMResponse struct {
	Records []fileConversionLLMRecord `json:"records"`
}

type fileConversionLLMRecord struct {
	Fields []fileConversionLLMField `json:"fields"`
}

type fileConversionLLMField struct {
	ColumnID   string      `json:"column_id"`
	ColumnName string      `json:"column_name,omitempty"`
	Value      interface{} `json:"value"`
	Evidence   string      `json:"evidence,omitempty"`
	Confidence *float64    `json:"confidence,omitempty"`
	Reason     string      `json:"reason,omitempty"`
}

func buildFileConversionColumnSchema(columns dto.GetTableColumnsResponse) (string, error) {
	schema := make([]fileConversionColumnSchema, 0, len(columns.Columns))
	for _, col := range columns.Columns {
		description := ""
		if col.Description != nil {
			description = strings.TrimSpace(*col.Description)
		}
		displayName := ""
		if col.DisplayName != nil {
			displayName = strings.TrimSpace(*col.DisplayName)
		}
		sourceColumnName := ""
		if col.SourceColumnName != nil {
			sourceColumnName = strings.TrimSpace(*col.SourceColumnName)
		}
		schema = append(schema, fileConversionColumnSchema{
			ColumnID:         fileConversionColumnID(col),
			FieldName:        col.Name,
			DisplayName:      displayName,
			SourceColumnName: sourceColumnName,
			Type:             col.Type,
			Description:      description,
			Required:         col.IsRequired,
		})
	}
	data, err := json.MarshalIndent(schema, "", "  ")
	if err != nil {
		return "", fmt.Errorf("failed to marshal field extraction column schema: %w", err)
	}
	return string(data), nil
}

func normalizeFileConversionOutput(raw string, columns dto.GetTableColumnsResponse) ([]map[string]interface{}, *dto.FileIngestFieldExtraction, error) {
	content := strings.TrimSpace(raw)
	if content == "" {
		return nil, nil, fmt.Errorf("field extraction response is empty")
	}
	if strings.HasPrefix(content, "[") {
		return normalizeLegacyFileConversionRecords(content, columns)
	}

	var parsed fileConversionLLMResponse
	if err := json.Unmarshal([]byte(content), &parsed); err != nil {
		return nil, nil, fmt.Errorf("failed to parse field extraction response object: %w", err)
	}
	return normalizeColumnIDFileConversionRecords(parsed, columns)
}

func normalizeColumnIDFileConversionRecords(parsed fileConversionLLMResponse, columns dto.GetTableColumnsResponse) ([]map[string]interface{}, *dto.FileIngestFieldExtraction, error) {
	lookup, orderedColumnIDs, err := fileConversionColumnLookup(columns)
	if err != nil {
		return nil, nil, err
	}

	records := make([]map[string]interface{}, 0, len(parsed.Records))
	extraction := &dto.FileIngestFieldExtraction{
		Records: make([]dto.FileIngestRecordExtraction, 0, len(parsed.Records)),
	}
	for recordIndex, parsedRecord := range parsed.Records {
		record := make(map[string]interface{}, len(columns.Columns))
		for _, col := range columns.Columns {
			record[col.Name] = nil
		}

		seen := make(map[string]struct{}, len(parsedRecord.Fields))
		recordExtraction := dto.FileIngestRecordExtraction{
			Fields: make([]dto.FileIngestFieldMatch, 0, len(parsedRecord.Fields)),
		}
		for _, field := range parsedRecord.Fields {
			columnID := strings.TrimSpace(field.ColumnID)
			if columnID == "" {
				return nil, nil, fmt.Errorf("field extraction record %d returned empty column_id", recordIndex)
			}
			col, exists := lookup[columnID]
			if !exists {
				return nil, nil, fmt.Errorf("field extraction record %d returned unknown column_id %q", recordIndex, columnID)
			}
			resolvedColumnID := fileConversionColumnID(col)
			if _, duplicate := seen[resolvedColumnID]; duplicate {
				return nil, nil, fmt.Errorf("field extraction record %d returned duplicate column_id %q", recordIndex, resolvedColumnID)
			}
			seen[resolvedColumnID] = struct{}{}

			normalized := normalizeFileIngestFieldValue(field.Value, col.Type)
			record[col.Name] = normalized.Value
			recordExtraction.Fields = append(recordExtraction.Fields, dto.FileIngestFieldMatch{
				ColumnID:            resolvedColumnID,
				ColumnName:          col.Name,
				Value:               normalized.Value,
				RawValue:            field.Value,
				NormalizedValue:     normalized.Value,
				NormalizationStatus: normalized.Status,
				NormalizationReason: normalized.Reason,
				Evidence:            strings.TrimSpace(field.Evidence),
				Confidence:          field.Confidence,
				Reason:              strings.TrimSpace(field.Reason),
			})
		}

		for _, columnID := range orderedColumnIDs {
			if _, exists := seen[columnID]; exists {
				continue
			}
			col := lookup[columnID]
			recordExtraction.Fields = append(recordExtraction.Fields, dto.FileIngestFieldMatch{
				ColumnID:            columnID,
				ColumnName:          col.Name,
				Value:               nil,
				RawValue:            nil,
				NormalizedValue:     nil,
				NormalizationStatus: fileIngestNormalizationEmpty,
				NormalizationReason: "value is missing",
			})
		}

		records = append(records, record)
		extraction.Records = append(extraction.Records, recordExtraction)
	}
	return records, extraction, nil
}

func normalizeLegacyFileConversionRecords(raw string, columns dto.GetTableColumnsResponse) ([]map[string]interface{}, *dto.FileIngestFieldExtraction, error) {
	var records []map[string]interface{}
	if err := json.Unmarshal([]byte(raw), &records); err != nil {
		return nil, nil, fmt.Errorf("failed to parse legacy field extraction records: %w", err)
	}

	extraction := &dto.FileIngestFieldExtraction{
		Records: make([]dto.FileIngestRecordExtraction, 0, len(records)),
	}
	for i, record := range records {
		recordExtraction := dto.FileIngestRecordExtraction{
			Fields: make([]dto.FileIngestFieldMatch, 0, len(columns.Columns)),
		}
		for _, col := range columns.Columns {
			value, exists := record[col.Name]
			if !exists {
				value = nil
			}
			normalized := normalizeFileIngestFieldValue(value, col.Type)
			record[col.Name] = normalized.Value
			recordExtraction.Fields = append(recordExtraction.Fields, dto.FileIngestFieldMatch{
				ColumnID:            fileConversionColumnID(col),
				ColumnName:          col.Name,
				Value:               normalized.Value,
				RawValue:            value,
				NormalizedValue:     normalized.Value,
				NormalizationStatus: normalized.Status,
				NormalizationReason: normalized.Reason,
			})
		}
		records[i] = record
		extraction.Records = append(extraction.Records, recordExtraction)
	}
	return records, extraction, nil
}

func fileConversionColumnLookup(columns dto.GetTableColumnsResponse) (map[string]dto.TableColumn, []string, error) {
	lookup := make(map[string]dto.TableColumn, len(columns.Columns)*2)
	orderedColumnIDs := make([]string, 0, len(columns.Columns))
	for _, col := range columns.Columns {
		columnID := fileConversionColumnID(col)
		if columnID == "" {
			return nil, nil, fmt.Errorf("field extraction column has empty id and name")
		}
		if _, exists := lookup[columnID]; exists {
			return nil, nil, fmt.Errorf("field extraction schema has duplicate column_id %q", columnID)
		}
		lookup[columnID] = col
		orderedColumnIDs = append(orderedColumnIDs, columnID)
	}
	return lookup, orderedColumnIDs, nil
}

func fileConversionColumnID(col dto.TableColumn) string {
	if id := strings.TrimSpace(col.ID); id != "" {
		return id
	}
	return strings.TrimSpace(col.Name)
}

type fileIngestNormalizedFieldValue struct {
	Value  interface{}
	Status string
	Reason string
}

func normalizeFileIngestFieldValue(value interface{}, columnType string) fileIngestNormalizedFieldValue {
	if isFileIngestEmptyValue(value) {
		return fileIngestNormalizedFieldValue{
			Value:  nil,
			Status: fileIngestNormalizationEmpty,
			Reason: "value is empty",
		}
	}

	switch strings.ToLower(strings.TrimSpace(columnType)) {
	case "integer", "int", "int4", "int8", "bigint", "smallint":
		return normalizeFileIngestInteger(value)
	case "numeric", "decimal", "float", "double", "real", "number":
		return normalizeFileIngestNumeric(value)
	case "boolean", "bool":
		return normalizeFileIngestBoolean(value)
	case "timestamp", "timestamptz", "timestamp without time zone", "timestamp with time zone", "date", "datetime":
		return normalizeFileIngestTimestamp(value)
	case "text", "varchar", "char", "string":
		return normalizeFileIngestText(value)
	default:
		return normalizeFileIngestText(value)
	}
}

func isFileIngestEmptyValue(value interface{}) bool {
	if value == nil {
		return true
	}
	if text, ok := value.(string); ok {
		return strings.TrimSpace(text) == ""
	}
	return false
}

func normalizeFileIngestInteger(value interface{}) fileIngestNormalizedFieldValue {
	switch v := value.(type) {
	case float64:
		if v != float64(int64(v)) {
			return invalidNormalizedValue("number is not an integer")
		}
		return fileIngestNormalizedFieldValue{Value: int64(v), Status: fileIngestNormalizationValid}
	case int:
		return fileIngestNormalizedFieldValue{Value: v, Status: fileIngestNormalizationValid}
	case int64:
		return fileIngestNormalizedFieldValue{Value: v, Status: fileIngestNormalizationValid}
	case json.Number:
		i, err := strconv.ParseInt(v.String(), 10, 64)
		if err != nil {
			return invalidNormalizedValue("number is not an integer")
		}
		return fileIngestNormalizedFieldValue{Value: i, Status: fileIngestNormalizationNormalized, Reason: "converted JSON number to integer"}
	default:
		num, status := normalizeFileIngestNumberFromText(fmt.Sprint(value), true)
		if status.Status != "" {
			return status
		}
		return fileIngestNormalizedFieldValue{Value: int64(num), Status: fileIngestNormalizationNormalized, Reason: "converted text to integer"}
	}
}

func normalizeFileIngestNumeric(value interface{}) fileIngestNormalizedFieldValue {
	switch v := value.(type) {
	case float64:
		return fileIngestNormalizedFieldValue{Value: v, Status: fileIngestNormalizationValid}
	case float32:
		return fileIngestNormalizedFieldValue{Value: float64(v), Status: fileIngestNormalizationNormalized, Reason: "converted float32 to number"}
	case int:
		return fileIngestNormalizedFieldValue{Value: float64(v), Status: fileIngestNormalizationNormalized, Reason: "converted integer to number"}
	case int64:
		return fileIngestNormalizedFieldValue{Value: float64(v), Status: fileIngestNormalizationNormalized, Reason: "converted integer to number"}
	case json.Number:
		num, err := strconv.ParseFloat(v.String(), 64)
		if err != nil {
			return invalidNormalizedValue("number is invalid")
		}
		return fileIngestNormalizedFieldValue{Value: num, Status: fileIngestNormalizationNormalized, Reason: "converted JSON number to number"}
	default:
		num, status := normalizeFileIngestNumberFromText(fmt.Sprint(value), false)
		if status.Status != "" {
			return status
		}
		return fileIngestNormalizedFieldValue{Value: num, Status: fileIngestNormalizationNormalized, Reason: "converted text to number"}
	}
}

func normalizeFileIngestNumberFromText(raw string, integer bool) (float64, fileIngestNormalizedFieldValue) {
	text := strings.TrimSpace(raw)
	if text == "" {
		return 0, fileIngestNormalizedFieldValue{Value: nil, Status: fileIngestNormalizationEmpty, Reason: "value is empty"}
	}
	matches := fileIngestNumericTokenPattern.FindAllString(text, -1)
	if len(matches) == 0 {
		return 0, invalidNormalizedValue("no numeric value found")
	}
	if len(matches) > 1 {
		return 0, invalidNormalizedValue("multiple numeric values found")
	}
	cleaned := strings.ReplaceAll(matches[0], ",", "")
	num, err := strconv.ParseFloat(cleaned, 64)
	if err != nil {
		return 0, invalidNormalizedValue("numeric value is invalid")
	}
	if integer && num != float64(int64(num)) {
		return 0, invalidNormalizedValue("number is not an integer")
	}
	return num, fileIngestNormalizedFieldValue{}
}

func normalizeFileIngestBoolean(value interface{}) fileIngestNormalizedFieldValue {
	if v, ok := value.(bool); ok {
		return fileIngestNormalizedFieldValue{Value: v, Status: fileIngestNormalizationValid}
	}
	switch strings.ToLower(strings.TrimSpace(fmt.Sprint(value))) {
	case "true", "yes", "y", "1", "on", "是", "对", "真":
		return fileIngestNormalizedFieldValue{Value: true, Status: fileIngestNormalizationNormalized, Reason: "converted text to boolean"}
	case "false", "no", "n", "0", "off", "否", "错", "假":
		return fileIngestNormalizedFieldValue{Value: false, Status: fileIngestNormalizationNormalized, Reason: "converted text to boolean"}
	default:
		return invalidNormalizedValue("boolean value is invalid")
	}
}

func normalizeFileIngestTimestamp(value interface{}) fileIngestNormalizedFieldValue {
	text := strings.TrimSpace(fmt.Sprint(value))
	if text == "" {
		return fileIngestNormalizedFieldValue{Value: nil, Status: fileIngestNormalizationEmpty, Reason: "value is empty"}
	}
	if fileIngestNumericTokenPattern.MatchString(text) && fileIngestNumericTokenPattern.FindString(text) == text {
		return invalidNormalizedValue("numeric date representation is not allowed")
	}
	if normalized, ok := normalizeLocalTimestampValue(text); ok {
		status := fileIngestNormalizationValid
		reason := ""
		if normalized != text {
			status = fileIngestNormalizationNormalized
			reason = "normalized date/time text"
		}
		return fileIngestNormalizedFieldValue{Value: normalized, Status: status, Reason: reason}
	}
	if normalized, ok := normalizeFileIngestChineseDate(text); ok {
		return fileIngestNormalizedFieldValue{Value: normalized, Status: fileIngestNormalizationNormalized, Reason: "normalized Chinese date text"}
	}
	return invalidNormalizedValue("date/time value is invalid")
}

func normalizeFileIngestChineseDate(text string) (string, bool) {
	matches := fileIngestChineseDatePattern.FindStringSubmatch(text)
	if len(matches) != 4 {
		return "", false
	}
	year, yearErr := strconv.Atoi(matches[1])
	month, monthErr := strconv.Atoi(matches[2])
	day, dayErr := strconv.Atoi(matches[3])
	if yearErr != nil || monthErr != nil || dayErr != nil {
		return "", false
	}
	normalized, ok := normalizeLocalTimestampValue(fmt.Sprintf("%04d-%02d-%02d", year, month, day))
	return normalized, ok
}

func normalizeFileIngestText(value interface{}) fileIngestNormalizedFieldValue {
	text := strings.TrimSpace(fmt.Sprint(value))
	if text == "" {
		return fileIngestNormalizedFieldValue{Value: nil, Status: fileIngestNormalizationEmpty, Reason: "value is empty"}
	}
	if text == fmt.Sprint(value) {
		return fileIngestNormalizedFieldValue{Value: text, Status: fileIngestNormalizationValid}
	}
	return fileIngestNormalizedFieldValue{Value: text, Status: fileIngestNormalizationNormalized, Reason: "trimmed text"}
}

func invalidNormalizedValue(reason string) fileIngestNormalizedFieldValue {
	return fileIngestNormalizedFieldValue{
		Value:  nil,
		Status: fileIngestNormalizationInvalid,
		Reason: reason,
	}
}
