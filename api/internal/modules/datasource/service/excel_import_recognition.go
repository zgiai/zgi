package service

import (
	"context"
	"encoding/json"
	"fmt"
	"regexp"
	"sort"
	"strings"

	"github.com/zgiai/zgi/api/internal/dto"
	excelimportrepo "github.com/zgiai/zgi/api/internal/modules/datasource/repository/excelimport"
	llmadapter "github.com/zgiai/zgi/api/internal/modules/llm/protocol/adapters"
	"github.com/zgiai/zgi/api/internal/prompt"
)

var (
	excelRecognitionIdentifierChars = regexp.MustCompile(`[^a-z0-9_]+`)
	excelRecognitionTablePattern    = regexp.MustCompile(`^[a-z][a-z0-9_]*$`)
	excelRecognitionFieldPattern    = regexp.MustCompile(`^[a-z][a-z0-9_]*$`)
	excelRecognitionReservedNames   = map[string]struct{}{
		"id":           {},
		"uuid":         {},
		"created_time": {},
		"updated_time": {},
	}
)

type excelFieldRecognitionPromptColumn struct {
	SourceColumnIndex int      `json:"source_column_index"`
	SourceColumn      string   `json:"source_column"`
	CurrentName       string   `json:"current_name"`
	DisplayName       string   `json:"display_name"`
	Type              string   `json:"type"`
	Description       string   `json:"description"`
	SampleValues      []string `json:"sample_values"`
	Enabled           *bool    `json:"enabled,omitempty"`
}

type excelFieldRecognitionLLMResponse struct {
	Table struct {
		Name        string `json:"name"`
		Description string `json:"description"`
	} `json:"table"`
	Columns []struct {
		SourceColumnIndex int    `json:"source_column_index"`
		SourceColumn      string `json:"source_column"`
		Name              string `json:"name"`
		DisplayName       string `json:"display_name"`
		Description       string `json:"description"`
	} `json:"columns"`
}

func (s *dataSourceService) RecognizeExcelImportFields(ctx context.Context, organizationID, dataSourceID, accountID, jobID string, req dto.RecognizeExcelImportRequest) (dto.RecognizeExcelImportData, error) {
	if req.Model == nil || strings.TrimSpace(req.Model.Name) == "" {
		return dto.RecognizeExcelImportData{}, fmt.Errorf("model is required")
	}
	if len(req.Columns) == 0 {
		return dto.RecognizeExcelImportData{}, fmt.Errorf("at least one column is required")
	}

	dataSource, err := s.repo.FindByID(ctx, dataSourceID)
	if err != nil {
		return dto.RecognizeExcelImportData{}, fmt.Errorf("failed to find data source: %w", err)
	}
	if dataSource == nil || dataSource.OrganizationID != organizationID {
		return dto.RecognizeExcelImportData{}, fmt.Errorf("data source not found")
	}

	jobRepo := excelimportrepo.NewJobRepository(s.db)
	job, err := jobRepo.FindByID(ctx, jobID)
	if err != nil {
		return dto.RecognizeExcelImportData{}, fmt.Errorf("failed to find import job: %w", err)
	}
	if job == nil || job.OrganizationID != organizationID || job.DataSourceID != dataSourceID {
		return dto.RecognizeExcelImportData{}, fmt.Errorf("import job not found")
	}

	columnsJSON, err := json.MarshalIndent(buildExcelRecognitionPromptColumns(req.Columns), "", "  ")
	if err != nil {
		return dto.RecognizeExcelImportData{}, fmt.Errorf("failed to encode columns for recognition: %w", err)
	}
	existingTableNames, err := s.listExcelRecognitionExistingTableNames(ctx, dataSourceID)
	if err != nil {
		return dto.RecognizeExcelImportData{}, err
	}
	existingTableNamesJSON, err := json.Marshal(existingTableNames)
	if err != nil {
		return dto.RecognizeExcelImportData{}, fmt.Errorf("failed to encode existing table names: %w", err)
	}

	tmpl, err := prompt.GetTemplate(prompt.DatasourceExcelFieldRecognition)
	if err != nil {
		return dto.RecognizeExcelImportData{}, fmt.Errorf("failed to get field recognition prompt template: %w", err)
	}
	promptText, err := tmpl.Render(struct {
		TableName         string
		TableDescription  string
		SourceFileName    string
		SourceSheetName   string
		OperatorLanguage  string
		ColumnsJSON       string
		ExistingNamesJSON string
	}{
		TableName:         req.Table.Name,
		TableDescription:  req.Table.Description,
		SourceFileName:    req.Source.FileName,
		SourceSheetName:   req.Source.SheetName,
		OperatorLanguage:  normalizeExcelRecognitionOperatorLanguage(req.OperatorLanguage),
		ColumnsJSON:       string(columnsJSON),
		ExistingNamesJSON: string(existingTableNamesJSON),
	})
	if err != nil {
		return dto.RecognizeExcelImportData{}, fmt.Errorf("failed to render field recognition prompt: %w", err)
	}

	resp, err := s.llmClient.Chat(ctx, organizationID, &llmadapter.ChatRequest{
		Model: s.getModelSlug(req.Model),
		Messages: []llmadapter.Message{
			{Role: "user", Content: promptText},
		},
		Stream: false,
		User:   accountID,
	})
	if err != nil {
		return dto.RecognizeExcelImportData{}, fmt.Errorf("failed to recognize fields with LLM: %w", err)
	}
	if resp == nil || len(resp.Choices) == 0 {
		return dto.RecognizeExcelImportData{}, fmt.Errorf("failed to recognize fields with LLM: empty response")
	}
	generatedContent, _ := resp.Choices[0].Message.Content.(string)
	if strings.TrimSpace(generatedContent) == "" {
		return dto.RecognizeExcelImportData{}, fmt.Errorf("failed to recognize fields with LLM: empty result")
	}

	cleanContent, err := s.extractJSONContent(generatedContent)
	if err != nil {
		return dto.RecognizeExcelImportData{}, fmt.Errorf("failed to extract JSON from field recognition response: %w", err)
	}
	var llmResult excelFieldRecognitionLLMResponse
	if err := json.Unmarshal([]byte(cleanContent), &llmResult); err != nil {
		return dto.RecognizeExcelImportData{}, fmt.Errorf("failed to parse field recognition response: %w", err)
	}

	result, err := normalizeExcelRecognitionResult(req, llmResult, excelRecognitionNameSet(existingTableNames))
	if err != nil {
		return dto.RecognizeExcelImportData{}, err
	}
	return result, nil
}

func (s *dataSourceService) listExcelRecognitionExistingTableNames(ctx context.Context, dataSourceID string) ([]string, error) {
	tables, err := s.tableRepo.ListByDataSource(ctx, dataSourceID)
	if err != nil {
		return nil, fmt.Errorf("failed to list existing tables for field recognition: %w", err)
	}
	seen := make(map[string]struct{}, len(tables))
	names := make([]string, 0, len(tables))
	for _, table := range tables {
		if table == nil {
			continue
		}
		name := strings.ToLower(strings.TrimSpace(table.Name))
		if name == "" {
			continue
		}
		if _, exists := seen[name]; exists {
			continue
		}
		seen[name] = struct{}{}
		names = append(names, name)
	}
	sort.Strings(names)
	return names, nil
}

func excelRecognitionNameSet(names []string) map[string]struct{} {
	out := make(map[string]struct{}, len(names))
	for _, name := range names {
		normalized := strings.ToLower(strings.TrimSpace(name))
		if normalized != "" {
			out[normalized] = struct{}{}
		}
	}
	return out
}

func buildExcelRecognitionPromptColumns(columns []dto.InferredExcelColumn) []excelFieldRecognitionPromptColumn {
	out := make([]excelFieldRecognitionPromptColumn, 0, len(columns))
	for _, col := range columns {
		samples := col.SampleValues
		if len(samples) > 5 {
			samples = samples[:5]
		}
		out = append(out, excelFieldRecognitionPromptColumn{
			SourceColumnIndex: col.SourceColumnIndex,
			SourceColumn:      col.SourceColumn,
			CurrentName:       col.Name,
			DisplayName:       col.DisplayName,
			Type:              col.Type,
			Description:       col.Description,
			SampleValues:      samples,
			Enabled:           col.Enabled,
		})
	}
	return out
}

func normalizeExcelRecognitionOperatorLanguage(raw string) string {
	switch strings.ToLower(strings.TrimSpace(raw)) {
	case "zh", "zh-cn", "zh-hans", "zh-hans-cn":
		return "Simplified Chinese (zh-Hans)"
	case "zh-tw", "zh-hant", "zh-hant-tw":
		return "Traditional Chinese (zh-Hant)"
	case "en", "en-us", "en-gb":
		return "English"
	default:
		if strings.TrimSpace(raw) != "" {
			return strings.TrimSpace(raw)
		}
		return "the operator's current UI language"
	}
}

func normalizeExcelRecognitionResult(req dto.RecognizeExcelImportRequest, llmResult excelFieldRecognitionLLMResponse, existingTableNames map[string]struct{}) (dto.RecognizeExcelImportData, error) {
	if len(llmResult.Columns) != len(req.Columns) {
		return dto.RecognizeExcelImportData{}, fmt.Errorf("field recognition returned %d columns, want %d", len(llmResult.Columns), len(req.Columns))
	}

	var result dto.RecognizeExcelImportData
	result.Table.Name = uniqueExcelRecognitionTableName(sanitizeExcelRecognitionTableName(llmResult.Table.Name, req.Table.Name), existingTableNames)
	result.Table.Description = strings.TrimSpace(llmResult.Table.Description)
	if result.Table.Description == "" {
		result.Table.Description = strings.TrimSpace(req.Table.Description)
	}

	used := make(map[string]int, len(req.Columns))
	result.Columns = make([]dto.InferredExcelColumn, 0, len(req.Columns))
	for i, inputCol := range req.Columns {
		modelCol := llmResult.Columns[i]
		nextCol := inputCol
		nextCol.Name = uniqueExcelRecognitionFieldName(sanitizeExcelRecognitionFieldName(modelCol.Name, inputCol.Name, i), used)
		if displayName := strings.TrimSpace(modelCol.DisplayName); displayName != "" {
			nextCol.DisplayName = displayName
		}
		if description := strings.TrimSpace(modelCol.Description); description != "" {
			nextCol.Description = description
		}
		if nextCol.SourceColumn == "" {
			nextCol.SourceColumn = modelCol.SourceColumn
		}
		result.Columns = append(result.Columns, nextCol)
	}
	return result, nil
}

func sanitizeExcelRecognitionTableName(raw, fallback string) string {
	name := sanitizeExcelRecognitionIdentifier(raw)
	if !excelRecognitionTablePattern.MatchString(name) {
		name = sanitizeExcelRecognitionIdentifier(fallback)
	}
	if name == "" {
		name = "imported_table"
	}
	if name[0] < 'a' || name[0] > 'z' {
		name = "table_" + name
	}
	if !excelRecognitionTablePattern.MatchString(name) {
		return "imported_table"
	}
	return name
}

func sanitizeExcelRecognitionFieldName(raw, fallback string, index int) string {
	name := sanitizeExcelRecognitionIdentifier(raw)
	if !excelRecognitionFieldPattern.MatchString(name) {
		name = sanitizeExcelRecognitionIdentifier(fallback)
	}
	if name == "" {
		name = fmt.Sprintf("field_%d", index+1)
	}
	if name[0] < 'a' || name[0] > 'z' {
		name = "field_" + name
	}
	if _, reserved := excelRecognitionReservedNames[name]; reserved {
		name += "_value"
	}
	if !excelRecognitionFieldPattern.MatchString(name) {
		name = fmt.Sprintf("field_%d", index+1)
	}
	return name
}

func sanitizeExcelRecognitionIdentifier(raw string) string {
	name := strings.ToLower(strings.TrimSpace(raw))
	name = strings.ReplaceAll(name, "-", "_")
	name = strings.ReplaceAll(name, " ", "_")
	name = excelRecognitionIdentifierChars.ReplaceAllString(name, "_")
	name = strings.Trim(name, "_")
	for strings.Contains(name, "__") {
		name = strings.ReplaceAll(name, "__", "_")
	}
	return name
}

func uniqueExcelRecognitionTableName(base string, existing map[string]struct{}) string {
	if len(existing) == 0 {
		return base
	}
	candidate := base
	suffix := 2
	for {
		if _, exists := existing[strings.ToLower(candidate)]; !exists {
			return candidate
		}
		candidate = fmt.Sprintf("%s_%d", base, suffix)
		suffix++
	}
}

func uniqueExcelRecognitionFieldName(base string, used map[string]int) string {
	name := base
	used[name]++
	if used[name] == 1 {
		return name
	}
	for {
		candidate := fmt.Sprintf("%s_%d", name, used[name])
		if _, exists := used[candidate]; !exists {
			used[candidate] = 1
			return candidate
		}
		used[name]++
	}
}
