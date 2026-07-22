package service

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/zgiai/zgi/api/internal/dto"
	"github.com/zgiai/zgi/api/internal/modules/datasource/model"
	excelimportmodel "github.com/zgiai/zgi/api/internal/modules/datasource/model/excelimport"
	excelimportrepo "github.com/zgiai/zgi/api/internal/modules/datasource/repository/excelimport"
	excelimportsvc "github.com/zgiai/zgi/api/internal/modules/datasource/service/excelimport"
	workspace_model "github.com/zgiai/zgi/api/internal/modules/workspace/model"
	"github.com/zgiai/zgi/api/pkg/logger"
)

const defaultExcelImportBatchSize = 500

type existingTableInsertResult struct {
	ImportedRows int
	FailedItems  []dto.ExcelImportFailedItem
}

type existingTableBatchInserter func(records []map[string]interface{}) (int, error)

func (s *dataSourceService) AnalyzeExistingTableExcelImport(ctx context.Context, organizationID, dataSourceID, tableID, accountID string, req dto.AnalyzeExcelImportRequest) (dto.AnalyzeExistingTableExcelImportData, error) {
	dataSource, table, err := s.validateDataSourceAndTable(ctx, organizationID, dataSourceID, tableID)
	if err != nil {
		return dto.AnalyzeExistingTableExcelImportData{}, err
	}
	if err := s.requireDataSourceWorkspacePermission(ctx, organizationID, accountID, dataSource, workspace_model.WorkspacePermissionDatabaseImportAnalyze); err != nil {
		return dto.AnalyzeExistingTableExcelImportData{}, err
	}

	analysis, err := s.AnalyzeExcelImport(ctx, organizationID, dataSourceID, accountID, req)
	if err != nil {
		return dto.AnalyzeExistingTableExcelImportData{}, err
	}
	targetColumns, err := s.GetTableColumns(ctx, organizationID, dataSourceID, table.ID, false)
	if err != nil {
		return dto.AnalyzeExistingTableExcelImportData{}, fmt.Errorf("failed to get target table columns: %w", err)
	}
	aliasRows, err := excelimportrepo.NewColumnAliasRepository(s.db).ListByTableID(ctx, organizationID, dataSourceID, table.ID)
	if err != nil {
		return dto.AnalyzeExistingTableExcelImportData{}, fmt.Errorf("failed to list import column aliases: %w", err)
	}
	aliases := make(map[string]map[string]struct{})
	for _, alias := range aliasRows {
		if aliases[alias.NormalizedHeader] == nil {
			aliases[alias.NormalizedHeader] = make(map[string]struct{})
		}
		aliases[alias.NormalizedHeader][alias.TargetColumnID] = struct{}{}
	}
	mappings, overallMatchRate := excelimportsvc.MatchExistingTableColumns(analysis.Columns, targetColumns.Columns, aliases)
	draft := dto.ExistingTableExcelImportDraftRequest{Mappings: mappings}

	jobRepo := excelimportrepo.NewJobRepository(s.db)
	job, err := jobRepo.FindByID(ctx, analysis.JobID)
	if err != nil {
		return dto.AnalyzeExistingTableExcelImportData{}, fmt.Errorf("failed to reload import job: %w", err)
	}
	if job == nil {
		return dto.AnalyzeExistingTableExcelImportData{}, fmt.Errorf("import job not found")
	}
	job.TableID = &table.ID
	job.SchemaSnapshot = mustJSON(targetColumns.Columns)
	job.PreviewSnapshot = mustJSON(draft)
	job.ErrorSummary = mustJSON(map[string]interface{}{"overall_match_rate": overallMatchRate, "skipped_rows": 0})
	job.UpdatedBy = accountID
	if err := jobRepo.Update(ctx, job); err != nil {
		return dto.AnalyzeExistingTableExcelImportData{}, fmt.Errorf("failed to update import job: %w", err)
	}

	return dto.AnalyzeExistingTableExcelImportData{
		JobID:            job.ID,
		Source:           analysis,
		TargetColumns:    targetColumns.Columns,
		Mappings:         mappings,
		OverallMatchRate: overallMatchRate,
		LowMatch:         overallMatchRate < 50,
	}, nil
}

func (s *dataSourceService) PreviewExistingTableExcelImport(ctx context.Context, organizationID, dataSourceID, tableID, accountID, jobID string, req dto.ExistingTableExcelImportDraftRequest) (dto.ExistingTableExcelImportPreviewData, error) {
	job, workbook, err := s.loadExistingTableImportWorkbook(ctx, organizationID, dataSourceID, tableID, accountID, jobID, workspace_model.WorkspacePermissionDatabaseImportAnalyze)
	if err != nil {
		return dto.ExistingTableExcelImportPreviewData{}, err
	}
	if job.Status != string(dto.ExcelImportStatusNeedsReview) {
		return dto.ExistingTableExcelImportPreviewData{}, fmt.Errorf("import job status %q cannot be previewed", job.Status)
	}
	selection, err := existingTableImportSelection(job)
	if err != nil {
		return dto.ExistingTableExcelImportPreviewData{}, err
	}
	validation, err := excelimportsvc.ValidateExistingTableDraft(workbook, selection, req)
	if err != nil {
		return dto.ExistingTableExcelImportPreviewData{}, err
	}

	job.PreviewSnapshot = mustJSON(req)
	job.ValidRows = validation.ValidRows
	job.FailedRows = validation.FailedRows
	job.ErrorSummary = mustJSON(map[string]interface{}{"skipped_rows": validation.SkippedRows})
	job.UpdatedBy = accountID
	if err := excelimportrepo.NewJobRepository(s.db).Update(ctx, job); err != nil {
		return dto.ExistingTableExcelImportPreviewData{}, fmt.Errorf("failed to save import preview: %w", err)
	}

	limit := req.Limit
	if limit <= 0 {
		limit = 100
	}
	if limit > 500 {
		limit = 500
	}
	offset := req.Offset
	if offset < 0 {
		offset = 0
	}
	return dto.ExistingTableExcelImportPreviewData{
		JobID:       job.ID,
		Rows:        validation.PreviewRows,
		TotalRows:   validation.TotalRows,
		ValidRows:   validation.ValidRows,
		FailedRows:  validation.FailedRows,
		SkippedRows: validation.SkippedRows,
		HasMore:     offset+len(validation.PreviewRows) < validation.TotalRows,
		Limit:       limit,
		Offset:      offset,
	}, nil
}

func (s *dataSourceService) ConfirmExistingTableExcelImport(ctx context.Context, organizationID, dataSourceID, tableID, accountID, jobID string) (dto.ConfirmExcelImportData, error) {
	job, workbook, err := s.loadExistingTableImportWorkbook(ctx, organizationID, dataSourceID, tableID, accountID, jobID, workspace_model.WorkspacePermissionDatabaseImportExecute)
	if err != nil {
		return dto.ConfirmExcelImportData{}, err
	}
	var draft dto.ExistingTableExcelImportDraftRequest
	if err := json.Unmarshal(job.PreviewSnapshot, &draft); err != nil || len(draft.Mappings) == 0 {
		return dto.ConfirmExcelImportData{}, fmt.Errorf("import preview must be confirmed before importing")
	}
	selection, err := existingTableImportSelection(job)
	if err != nil {
		return dto.ConfirmExcelImportData{}, err
	}
	validation, err := excelimportsvc.ValidateExistingTableDraft(workbook, selection, draft)
	if err != nil {
		return dto.ConfirmExcelImportData{}, err
	}

	jobRepo := excelimportrepo.NewJobRepository(s.db)
	claimed, err := jobRepo.MarkImporting(ctx, job.ID, organizationID, dataSourceID, accountID)
	if err != nil {
		return dto.ConfirmExcelImportData{}, fmt.Errorf("failed to update import job: %w", err)
	}
	if !claimed {
		return dto.ConfirmExcelImportData{}, fmt.Errorf("import job status %q cannot be confirmed", job.Status)
	}
	job.Status = string(dto.ExcelImportStatusImporting)

	dataSource, table, err := s.validateDataSourceAndTable(ctx, organizationID, dataSourceID, tableID)
	if err != nil {
		return s.failExistingTableImport(ctx, job, accountID, err)
	}
	currentColumns, err := s.GetTableColumns(ctx, organizationID, dataSourceID, tableID, false)
	if err != nil {
		return s.failExistingTableImport(ctx, job, accountID, fmt.Errorf("failed to reload target table columns: %w", err))
	}
	if !sameExistingTableImportSchema(job.SchemaSnapshot, currentColumns.Columns) {
		return s.failExistingTableImport(ctx, job, accountID, fmt.Errorf("target table structure changed; analyze the file again"))
	}
	insertResult := insertExistingTableRecords(validation.Records, defaultExcelImportBatchSize, func(records []map[string]interface{}) (int, error) {
		result, insertErr := s.addTableRecordsToTable(ctx, organizationID, dataSource, table, accountID, dto.AddRecordRequest{Records: records}, model.OperationTypeImport)
		return int(result.AffectedRows), insertErr
	})
	failedItems := append(append([]dto.ExcelImportFailedItem{}, validation.Errors...), insertResult.FailedItems...)
	failedRows := validation.FailedRows + len(insertResult.FailedItems)

	errorSummary := map[string]interface{}{"skipped_rows": validation.SkippedRows}
	if aliasErr := persistExistingTableImportAliases(insertResult.ImportedRows, func() error {
		return s.confirmExistingTableImportAliases(ctx, job, draft.Mappings, accountID)
	}); aliasErr != nil {
		errorSummary["alias_persistence_error"] = aliasErr.Error()
		logger.ErrorContext(ctx, "failed to save confirmed excel import aliases", "job_id", job.ID, "table_id", tableID, aliasErr)
	}

	errorRepo := excelimportrepo.NewErrorRepository(s.db)
	if err := errorRepo.DeleteByJobID(ctx, job.ID); err != nil {
		return dto.ConfirmExcelImportData{}, fmt.Errorf("failed to clear import errors: %w", err)
	}
	if err := errorRepo.CreateBatch(ctx, buildExcelImportErrors(job.ID, failedItems)); err != nil {
		return dto.ConfirmExcelImportData{}, fmt.Errorf("failed to save import errors: %w", err)
	}

	status := existingTableImportStatus(insertResult.ImportedRows, len(validation.Records), failedRows)
	job.Status = string(status)
	job.TotalRows = validation.TotalRows
	job.ValidRows = validation.ValidRows
	job.ImportedRows = insertResult.ImportedRows
	job.FailedRows = failedRows
	job.ErrorSummary = mustJSON(errorSummary)
	job.UpdatedBy = accountID
	if err := jobRepo.Update(ctx, job); err != nil {
		return dto.ConfirmExcelImportData{}, fmt.Errorf("failed to finalize import job: %w", err)
	}
	return dto.ConfirmExcelImportData{
		JobID:        job.ID,
		TableID:      tableID,
		Status:       status,
		TotalRows:    validation.TotalRows,
		ImportedRows: insertResult.ImportedRows,
		FailedRows:   failedRows,
		SkippedRows:  validation.SkippedRows,
		FailedItems:  failedItems,
	}, nil
}

func insertExistingTableRecords(records []excelimportsvc.ValidatedRecord, batchSize int, insert existingTableBatchInserter) existingTableInsertResult {
	if batchSize <= 0 {
		batchSize = defaultExcelImportBatchSize
	}
	result := existingTableInsertResult{FailedItems: []dto.ExcelImportFailedItem{}}
	for start := 0; start < len(records); start += batchSize {
		end := start + batchSize
		if end > len(records) {
			end = len(records)
		}
		batch := records[start:end]
		values := make([]map[string]interface{}, len(batch))
		for index := range batch {
			values[index] = batch[index].Values
		}
		affectedRows, err := insert(values)
		if err == nil {
			if affectedRows <= 0 {
				affectedRows = len(batch)
			}
			result.ImportedRows += affectedRows
			continue
		}
		for _, record := range batch {
			affectedRows, rowErr := insert([]map[string]interface{}{record.Values})
			if rowErr != nil {
				result.FailedItems = append(result.FailedItems, dto.ExcelImportFailedItem{
					RowIndex:     record.RowIndex,
					ErrorCode:    "database_insert_failed",
					ErrorMessage: rowErr.Error(),
				})
				continue
			}
			if affectedRows <= 0 {
				affectedRows = 1
			}
			result.ImportedRows += affectedRows
		}
	}
	return result
}

func existingTableImportStatus(importedRows, candidateRows, failedRows int) dto.ExcelImportStatus {
	if candidateRows > 0 && importedRows == 0 && failedRows > 0 {
		return dto.ExcelImportStatusFailed
	}
	if failedRows > 0 {
		return dto.ExcelImportStatusPartialFailed
	}
	return dto.ExcelImportStatusCompleted
}

func persistExistingTableImportAliases(importedRows int, persist func() error) error {
	if importedRows == 0 {
		return nil
	}
	return persist()
}

func (s *dataSourceService) loadExistingTableImportWorkbook(ctx context.Context, organizationID, dataSourceID, tableID, accountID, jobID string, permission workspace_model.WorkspacePermissionCode) (*excelimportmodel.ImportJob, *excelimportsvc.ParsedWorkbook, error) {
	job, err := excelimportrepo.NewJobRepository(s.db).FindByID(ctx, jobID)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to find import job: %w", err)
	}
	if job == nil || job.OrganizationID != organizationID || job.DataSourceID != dataSourceID || job.TableID == nil || *job.TableID != tableID {
		return nil, nil, fmt.Errorf("import job not found")
	}
	dataSource, err := s.requireDataSourceInOrganization(ctx, organizationID, dataSourceID)
	if err != nil {
		return nil, nil, err
	}
	if err := s.requireDataSourceWorkspacePermission(ctx, organizationID, accountID, dataSource, permission); err != nil {
		return nil, nil, err
	}
	if job.UploadFileID == nil || strings.TrimSpace(*job.UploadFileID) == "" {
		return nil, nil, fmt.Errorf("import job has no upload file")
	}
	fileInfo, err := s.fileService.GetFileByID(ctx, *job.UploadFileID)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to get file: %w", err)
	}
	if err := s.ensureExcelImportFileReadable(ctx, organizationID, accountID, fileInfo); err != nil {
		return nil, nil, err
	}
	content, err := s.fileService.DownloadFile(ctx, *job.UploadFileID)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to download file: %w", err)
	}
	workbook, err := excelimportsvc.ParseWorkbook(job.SourceFileName, content)
	if err != nil {
		return nil, nil, err
	}
	return job, workbook, nil
}

func existingTableImportSelection(job *excelimportmodel.ImportJob) (struct {
	SheetName string `json:"sheet_name"`
	HeaderRow int    `json:"header_row"`
	StartRow  int    `json:"start_row"`
}, error) {
	var selection struct {
		SheetName string `json:"sheet_name"`
		HeaderRow int    `json:"header_row"`
		StartRow  int    `json:"start_row"`
	}
	if job.SheetName == nil || job.HeaderRow == nil || job.StartRow == nil {
		return selection, fmt.Errorf("import job selection is incomplete")
	}
	selection.SheetName = *job.SheetName
	selection.HeaderRow = *job.HeaderRow
	selection.StartRow = *job.StartRow
	return selection, nil
}

func sameExistingTableImportSchema(snapshot []byte, current []dto.TableColumn) bool {
	var saved []dto.TableColumn
	if err := json.Unmarshal(snapshot, &saved); err != nil || len(saved) != len(current) {
		return false
	}
	for index := range saved {
		if saved[index].ID != current[index].ID || saved[index].Name != current[index].Name || saved[index].Type != current[index].Type || saved[index].IsRequired != current[index].IsRequired {
			return false
		}
	}
	return true
}

func (s *dataSourceService) failExistingTableImport(ctx context.Context, job *excelimportmodel.ImportJob, accountID string, cause error) (dto.ConfirmExcelImportData, error) {
	job.Status = string(dto.ExcelImportStatusFailed)
	job.ErrorSummary = mustJSON(map[string]string{"message": cause.Error()})
	job.UpdatedBy = accountID
	if err := excelimportrepo.NewJobRepository(s.db).Update(ctx, job); err != nil {
		return dto.ConfirmExcelImportData{}, fmt.Errorf("%w; also failed to mark import job failed: %v", cause, err)
	}
	return dto.ConfirmExcelImportData{}, cause
}

func (s *dataSourceService) confirmExistingTableImportAliases(ctx context.Context, job *excelimportmodel.ImportJob, mappings []dto.ExistingTableExcelImportMapping, accountID string) error {
	repo := excelimportrepo.NewColumnAliasRepository(s.db)
	for _, mapping := range mappings {
		if mapping.Action != dto.ExcelImportMappingMap || !mapping.Confirmed || mapping.SourceColumn == nil || mapping.TargetColumnID == "" {
			continue
		}
		normalized := excelimportsvc.NormalizeImportHeader(*mapping.SourceColumn)
		if normalized == "" {
			continue
		}
		if err := repo.Confirm(ctx, excelimportmodel.ColumnAlias{
			OrganizationID:   job.OrganizationID,
			DataSourceID:     job.DataSourceID,
			TableID:          *job.TableID,
			TargetColumnID:   mapping.TargetColumnID,
			TargetColumnName: mapping.TargetColumnName,
			SourceHeader:     *mapping.SourceColumn,
			NormalizedHeader: normalized,
			CreatedBy:        accountID,
			UpdatedBy:        accountID,
		}); err != nil {
			return err
		}
	}
	return nil
}
