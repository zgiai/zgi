package excelimport

import (
	"fmt"
	"math"
	"sort"
	"strings"
	"unicode"

	"github.com/zgiai/zgi/api/internal/dto"
)

const (
	exactMatchThreshold    = 85.0
	possibleMatchThreshold = 60.0
	defaultPreviewLimit    = 100
	maxPreviewLimit        = 500
)

type ExistingTableValidationResult struct {
	Records     []map[string]interface{}
	Errors      []dto.ExcelImportFailedItem
	PreviewRows []dto.ExistingTableExcelImportPreviewRow
	TotalRows   int
	ValidRows   int
	FailedRows  int
	SkippedRows int
}

func NormalizeImportHeader(value string) string {
	var b strings.Builder
	for _, r := range strings.ToLower(strings.TrimSpace(value)) {
		if unicode.IsLetter(r) || unicode.IsDigit(r) {
			b.WriteRune(r)
		}
	}
	return b.String()
}

func MatchExistingTableColumns(source []dto.InferredExcelColumn, target []dto.TableColumn, aliases map[string]map[string]struct{}) ([]dto.ExistingTableExcelImportMapping, float64) {
	type candidate struct {
		sourceIndex int
		targetIndex int
		score       float64
		reason      string
	}
	candidates := make([]candidate, 0, len(source)*len(target))
	for sourceIndex, sourceColumn := range source {
		for targetIndex, targetColumn := range target {
			score, reason := existingTableColumnScore(sourceColumn, targetColumn, aliases)
			candidates = append(candidates, candidate{sourceIndex: sourceIndex, targetIndex: targetIndex, score: score, reason: reason})
		}
	}
	sort.SliceStable(candidates, func(i, j int) bool { return candidates[i].score > candidates[j].score })

	assignedSource := make(map[int]candidate)
	assignedTarget := make(map[int]struct{})
	for _, item := range candidates {
		if item.score < possibleMatchThreshold {
			break
		}
		if _, exists := assignedSource[item.sourceIndex]; exists {
			continue
		}
		if _, exists := assignedTarget[item.targetIndex]; exists {
			continue
		}
		assignedSource[item.sourceIndex] = item
		assignedTarget[item.targetIndex] = struct{}{}
	}

	mappings := make([]dto.ExistingTableExcelImportMapping, 0, len(source)+len(target))
	totalScore := 0.0
	for sourceIndex, sourceColumn := range source {
		sourceName := sourceColumn.SourceColumn
		sourcePosition := sourceColumn.SourceColumnIndex
		mapping := dto.ExistingTableExcelImportMapping{
			SourceColumn:      &sourceName,
			SourceColumnIndex: &sourcePosition,
			SampleValues:      sourceColumn.SampleValues,
			Status:            dto.ExcelImportMatchUnmatched,
			Action:            dto.ExcelImportMappingSkip,
			Reason:            "no_matching_target",
		}
		if item, exists := assignedSource[sourceIndex]; exists {
			targetColumn := target[item.targetIndex]
			mapping.TargetColumnID = targetColumn.ID
			mapping.TargetColumnName = targetColumn.Name
			mapping.TargetDisplayName = tableColumnDisplayName(targetColumn)
			mapping.TargetType = targetColumn.Type
			mapping.TargetRequired = targetColumn.IsRequired
			mapping.Confidence = math.Round(item.score*10) / 10
			mapping.Status = matchStatus(item.score)
			mapping.Action = dto.ExcelImportMappingMap
			mapping.Reason = item.reason
			mapping.Confirmed = mapping.Status == dto.ExcelImportMatchExact
			totalScore += item.score
		}
		mappings = append(mappings, mapping)
	}

	for targetIndex, targetColumn := range target {
		if _, exists := assignedTarget[targetIndex]; exists {
			continue
		}
		mappings = append(mappings, dto.ExistingTableExcelImportMapping{
			TargetColumnID:    targetColumn.ID,
			TargetColumnName:  targetColumn.Name,
			TargetDisplayName: tableColumnDisplayName(targetColumn),
			TargetType:        targetColumn.Type,
			TargetRequired:    targetColumn.IsRequired,
			Status:            dto.ExcelImportMatchUnmatched,
			Action:            dto.ExcelImportMappingSkip,
			Reason:            "no_matching_source",
		})
	}

	denominator := len(source)
	if len(target) > denominator {
		denominator = len(target)
	}
	if denominator == 0 {
		return mappings, 0
	}
	return mappings, math.Round(totalScore/float64(denominator)*10) / 10
}

func ValidateExistingTableDraft(wb *ParsedWorkbook, selection struct {
	SheetName string `json:"sheet_name"`
	HeaderRow int    `json:"header_row"`
	StartRow  int    `json:"start_row"`
}, draft dto.ExistingTableExcelImportDraftRequest) (*ExistingTableValidationResult, error) {
	if err := validateExistingTableMappings(draft.Mappings); err != nil {
		return nil, err
	}
	sheet, err := selectSheet(wb.Sheets, &selection.SheetName)
	if err != nil {
		return nil, err
	}
	if selection.StartRow < 1 || selection.StartRow > len(sheet.Rows)+1 {
		return nil, fmt.Errorf("start row %d is outside sheet range", selection.StartRow)
	}

	skipped := make(map[int]struct{}, len(draft.SkippedRows))
	for _, rowIndex := range draft.SkippedRows {
		skipped[rowIndex] = struct{}{}
	}
	limit := draft.Limit
	if limit <= 0 {
		limit = defaultPreviewLimit
	}
	if limit > maxPreviewLimit {
		limit = maxPreviewLimit
	}
	offset := draft.Offset
	if offset < 0 {
		offset = 0
	}

	result := &ExistingTableValidationResult{Errors: []dto.ExcelImportFailedItem{}}
	failedRows := make(map[int]struct{})
	dataRows := sheet.Rows[selection.StartRow-1:]
	result.TotalRows = len(dataRows)
	for rowOffset, row := range dataRows {
		rowIndex := selection.StartRow + rowOffset
		original := make(map[string]string, len(row))
		for columnIndex, value := range row {
			original[fmt.Sprintf("column_%d", columnIndex+1)] = value
		}
		preview := dto.ExistingTableExcelImportPreviewRow{
			RowIndex: rowIndex,
			Original: original,
			Cells:    make(map[string]dto.ExistingTableExcelImportPreviewCell),
			Status:   "valid",
		}
		if _, isSkipped := skipped[rowIndex]; isSkipped {
			preview.Status = "skipped"
			result.SkippedRows++
			appendPreviewRow(result, preview, rowOffset, offset, limit)
			continue
		}

		record := make(map[string]interface{})
		rowFailed := false
		for _, mapping := range draft.Mappings {
			if mapping.Action == dto.ExcelImportMappingSkip || mapping.TargetColumnName == "" {
				continue
			}
			raw := ""
			if mapping.Action == dto.ExcelImportMappingFixed && mapping.FixedValue != nil {
				raw = *mapping.FixedValue
			} else if mapping.SourceColumnIndex != nil && *mapping.SourceColumnIndex >= 0 && *mapping.SourceColumnIndex < len(row) {
				raw = row[*mapping.SourceColumnIndex]
			}
			if changes, exists := draft.RowChanges[rowIndex]; exists {
				if changed, exists := changes[mapping.TargetColumnName]; exists {
					raw = changed
				}
			}
			cell := dto.ExistingTableExcelImportPreviewCell{OriginalValue: raw}
			value, convertErr := convertValue(strings.TrimSpace(raw), normalizeImportTargetType(mapping.TargetType), mapping.TargetRequired)
			if convertErr != nil {
				code := "invalid_" + normalizeImportTargetType(mapping.TargetType)
				message := convertErr.Error()
				cell.ErrorCode = &code
				cell.ErrorMessage = &message
				columnName := mapping.TargetColumnName
				rawValue := raw
				result.Errors = append(result.Errors, dto.ExcelImportFailedItem{RowIndex: rowIndex, ColumnName: &columnName, RawValue: &rawValue, ErrorCode: code, ErrorMessage: message})
				rowFailed = true
			} else {
				cell.TransformedValue = value
				record[mapping.TargetColumnName] = value
			}
			preview.Cells[mapping.TargetColumnName] = cell
		}
		if rowFailed {
			preview.Status = "failed"
			failedRows[rowIndex] = struct{}{}
		} else if !isEmptyRow(row) {
			result.Records = append(result.Records, record)
			result.ValidRows++
		}
		appendPreviewRow(result, preview, rowOffset, offset, limit)
	}
	result.FailedRows = len(failedRows)
	return result, nil
}

func validateExistingTableMappings(mappings []dto.ExistingTableExcelImportMapping) error {
	usedSources := make(map[int]struct{})
	usedTargets := make(map[string]struct{})
	for _, mapping := range mappings {
		if mapping.Status != dto.ExcelImportMatchExact && !mapping.Confirmed {
			return fmt.Errorf("field mapping for %q must be confirmed", mappingSourceLabel(mapping))
		}
		if mapping.Action == dto.ExcelImportMappingSkip {
			if mapping.TargetRequired {
				return fmt.Errorf("required field %q cannot be skipped", mapping.TargetColumnName)
			}
			continue
		}
		if mapping.TargetColumnName == "" {
			return fmt.Errorf("target field is required for %q", mappingSourceLabel(mapping))
		}
		if _, exists := usedTargets[mapping.TargetColumnName]; exists {
			return fmt.Errorf("target field %q is mapped more than once", mapping.TargetColumnName)
		}
		usedTargets[mapping.TargetColumnName] = struct{}{}
		if mapping.Action == dto.ExcelImportMappingFixed {
			if mapping.FixedValue == nil {
				return fmt.Errorf("fixed value is required for field %q", mapping.TargetColumnName)
			}
			continue
		}
		if mapping.SourceColumnIndex == nil {
			return fmt.Errorf("source field is required for target field %q", mapping.TargetColumnName)
		}
		if _, exists := usedSources[*mapping.SourceColumnIndex]; exists {
			return fmt.Errorf("source field %d is mapped more than once", *mapping.SourceColumnIndex)
		}
		usedSources[*mapping.SourceColumnIndex] = struct{}{}
	}
	return nil
}

func existingTableColumnScore(source dto.InferredExcelColumn, target dto.TableColumn, aliases map[string]map[string]struct{}) (float64, string) {
	sourceName := NormalizeImportHeader(source.SourceColumn)
	if sourceName == NormalizeImportHeader(target.Name) {
		return typeAdjustedScore(100, source.Type, target.Type), "sql_name_exact"
	}
	if target.SourceColumnName != nil && sourceName == NormalizeImportHeader(*target.SourceColumnName) {
		return typeAdjustedScore(100, source.Type, target.Type), "original_header_exact"
	}
	if targets, exists := aliases[sourceName]; exists {
		if _, exists := targets[target.ID]; exists {
			return typeAdjustedScore(100, source.Type, target.Type), "history_alias_exact"
		}
	}
	targetLabel := tableColumnDisplayName(target)
	similarity := runeBigramSimilarity(sourceName, NormalizeImportHeader(targetLabel))
	if sqlSimilarity := runeBigramSimilarity(sourceName, NormalizeImportHeader(target.Name)); sqlSimilarity > similarity {
		similarity = sqlSimilarity
	}
	score := similarity * 80
	if typesCompatible(source.Type, target.Type) {
		score += 10
	}
	if target.Description != nil && strings.Contains(NormalizeImportHeader(*target.Description), sourceName) && sourceName != "" {
		score += 10
	}
	if score > 90 {
		score = 90
	}
	return score, "similar_name"
}

func appendPreviewRow(result *ExistingTableValidationResult, row dto.ExistingTableExcelImportPreviewRow, rowOffset, offset, limit int) {
	if rowOffset < offset || rowOffset >= offset+limit {
		return
	}
	result.PreviewRows = append(result.PreviewRows, row)
}

func matchStatus(score float64) dto.ExcelImportMatchStatus {
	if score >= exactMatchThreshold {
		return dto.ExcelImportMatchExact
	}
	if score >= possibleMatchThreshold {
		return dto.ExcelImportMatchPossible
	}
	return dto.ExcelImportMatchUnmatched
}

func tableColumnDisplayName(column dto.TableColumn) string {
	if column.SourceColumnName != nil && strings.TrimSpace(*column.SourceColumnName) != "" {
		return strings.TrimSpace(*column.SourceColumnName)
	}
	return column.Name
}

func mappingSourceLabel(mapping dto.ExistingTableExcelImportMapping) string {
	if mapping.SourceColumn != nil {
		return *mapping.SourceColumn
	}
	return mapping.TargetColumnName
}

func typeAdjustedScore(score float64, sourceType, targetType string) float64 {
	if typesCompatible(sourceType, targetType) {
		return score
	}
	return math.Max(0, score-40)
}

func typesCompatible(sourceType, targetType string) bool {
	sourceType = normalizeImportTargetType(sourceType)
	targetType = normalizeImportTargetType(targetType)
	if sourceType == targetType || sourceType == "text" || targetType == "text" {
		return true
	}
	return (sourceType == "integer" && targetType == "numeric") || (sourceType == "numeric" && targetType == "integer")
}

func normalizeImportTargetType(value string) string {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "int", "int2", "int4", "int8", "smallint", "bigint", "integer":
		return "integer"
	case "float", "float4", "float8", "double", "decimal", "numeric", "real":
		return "numeric"
	case "bool", "boolean":
		return "boolean"
	case "date", "datetime", "timestamp", "timestamptz", "timestamp without time zone", "timestamp with time zone":
		return "timestamp"
	default:
		return "text"
	}
}

func runeBigramSimilarity(left, right string) float64 {
	if left == "" || right == "" {
		return 0
	}
	if left == right {
		return 1
	}
	leftPairs := runeBigrams(left)
	rightPairs := runeBigrams(right)
	intersection := 0
	for pair := range leftPairs {
		if _, exists := rightPairs[pair]; exists {
			intersection++
		}
	}
	union := len(leftPairs) + len(rightPairs) - intersection
	if union == 0 {
		return 0
	}
	return float64(intersection) / float64(union)
}

func runeBigrams(value string) map[string]struct{} {
	runes := []rune(value)
	result := make(map[string]struct{})
	if len(runes) == 1 {
		result[string(runes)] = struct{}{}
		return result
	}
	for index := 0; index+1 < len(runes); index++ {
		result[string(runes[index:index+2])] = struct{}{}
	}
	return result
}
