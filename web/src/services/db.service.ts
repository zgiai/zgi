import { BaseService } from '@/lib/http/services';
import type { ApiResponseData } from './types/common';
import type {
  CreateDbRequest,
  UpdateDbRequest,
  Db,
  DeleteDbResponse,
  DbTable,
  CreateDbTableRequest,
  UpdateDbTableRequest,
  DbTableColumnsPayload,
  UpdateDbTableColumnsRequest,
  UpdateDbTableColumnsResponse,
  DbTableRecordsList,
  GetDbTableRecordsParams,
  CreateDbTableRecordsRequest,
  UpdateDbTableRecordsRequest,
  BatchIngestFileToTableRequest,
  BatchIngestFileToTableData,
  IngestFileToTableRequest,
  IngestFileToTableData,
  DbTablePrompt,
  UpdateDbTablePromptRequest,
  AnalyzeFileForTableRequest,
  GetDbSqlOperationsParams,
  DbSqlOperationList,
  ImportDbTableRecordsData,
  ImportDbTableRecordsRequest,
  AnalyzeExcelImportRequest,
  AnalyzeExcelImportData,
  ConfirmExcelImportRequest,
  ConfirmExcelImportData,
  RecognizeExcelImportRequest,
  RecognizeExcelImportData,
  ExcelImportJob,
  ExcelImportErrorList,
} from './types/db';

/**
 * DbService
 * Handles all database module APIs under `/console/api/data-dbs`.
 */
class DbService extends BaseService {
  constructor() {
    super({ endpoint: 'main', basePath: '/console/api' });
  }

  /**
   * Get DB list (basic, non-paginated)
   * GET /console/api/data-dbs
   */
  getDbsBasic(params?: {
    keyword?: string;
    workspace_id?: string;
  }): Promise<ApiResponseData<Db[]>> {
    const apiParams = { ...params };

    return this.request('get', '/data-dbs', undefined, { params: apiParams });
  }

  /**
   * Get DB detail
   */
  getDb(dbId: string): Promise<ApiResponseData<Db>> {
    return this.request('get', `/data-dbs/${dbId}`);
  }

  /**
   * Create DB
   */
  createDb(data: CreateDbRequest): Promise<ApiResponseData<Db>> {
    const body = { ...data };

    return this.request('post', '/data-dbs', body);
  }

  /**
   * Update DB
   */
  updateDb(dbId: string, data: UpdateDbRequest): Promise<ApiResponseData<Db>> {
    const body = { ...data };

    return this.request('put', `/data-dbs/${dbId}`, body);
  }

  /**
   * Delete DB
   */
  deleteDb(dbId: string): Promise<ApiResponseData<DeleteDbResponse>> {
    return this.request('delete', `/data-dbs/${dbId}`);
  }

  /* ------------------------------------------------------------------------ */
  /* Tables under DB                                                          */
  /* ------------------------------------------------------------------------ */

  /**
   * Get tables in a DB
   */
  getDbTables(dbId: string): Promise<ApiResponseData<DbTable[]>> {
    return this.request('get', `/data-dbs/${dbId}/tables`);
  }

  /**
   * Create a table in a DB
   */
  createDbTable(dbId: string, data: CreateDbTableRequest): Promise<ApiResponseData<DbTable>> {
    return this.request('post', `/data-dbs/${dbId}/tables`, data);
  }

  /**
   * Get table detail
   */
  getDbTableDetail(dbId: string, id: string): Promise<ApiResponseData<DbTable>> {
    return this.request('get', `/data-dbs/${dbId}/tables/${id}`);
  }

  /**
   * Update a table (basic fields)
   */
  updateDbTable(
    dbId: string,
    id: string,
    data: UpdateDbTableRequest
  ): Promise<ApiResponseData<DbTable>> {
    return this.request('put', `/data-dbs/${dbId}/tables/${id}`, data);
  }

  /**
   * Delete a table
   */
  deleteDbTable(
    dbId: string,
    id: string
  ): Promise<ApiResponseData<{ result: 'success' | 'fail' }>> {
    return this.request('delete', `/data-dbs/${dbId}/tables/${id}`);
  }

  /* ------------------------------------------------------------------------ */
  /* Table Columns (structure)                                               */
  /* ------------------------------------------------------------------------ */

  /**
   * Get table columns
   */
  getDbTableColumns(
    dbId: string,
    tableId: string,
    includeSystemFields: boolean = true
  ): Promise<ApiResponseData<DbTableColumnsPayload>> {
    return this.request('get', `/data-dbs/${dbId}/tables/${tableId}/columns`, undefined, {
      params: { include_system_fields: includeSystemFields },
    });
  }

  /**
   * Update table columns
   */
  updateDbTableColumns(
    dbId: string,
    tableId: string,
    data: UpdateDbTableColumnsRequest
  ): Promise<ApiResponseData<UpdateDbTableColumnsResponse>> {
    return this.request('put', `/data-dbs/${dbId}/tables/${tableId}/columns`, data);
  }

  /**
   * Analyze a file for table structure
   */
  analyzeFileForTable(
    data: AnalyzeFileForTableRequest
  ): Promise<ApiResponseData<DbTableColumnsPayload>> {
    return this.request('post', `/data-dbs/analyze-file-for-table`, data, {
      timeout: 300000,
    });
  }

  /**
   * Ingest data from one file into a table
   */
  ingestFileToTable(
    data: IngestFileToTableRequest
  ): Promise<ApiResponseData<IngestFileToTableData>> {
    return this.request('post', `/data-dbs/ingest-file-to-table`, data, {
      timeout: 600000,
    });
  }

  /**
   * Batch ingest data from files into a table
   */
  batchIngestFileToTable(
    data: BatchIngestFileToTableRequest
  ): Promise<ApiResponseData<BatchIngestFileToTableData>> {
    return this.request('post', `/data-dbs/batch-ingest-file-to-table`, data, {
      timeout: 600000,
    });
  }

  /* ------------------------------------------------------------------------ */
  /* Table Prompt                                                             */
  /* ------------------------------------------------------------------------ */

  /**
   * Get table prompt
   */
  getDbTablePrompt(dbId: string, tableId: string): Promise<ApiResponseData<DbTablePrompt>> {
    return this.request('get', `/data-dbs/${dbId}/tables/${tableId}/prompt`);
  }

  /**
   * Update table prompt
   */
  updateDbTablePrompt(
    dbId: string,
    tableId: string,
    data: UpdateDbTablePromptRequest
  ): Promise<ApiResponseData<DbTablePrompt>> {
    return this.request('put', `/data-dbs/${dbId}/tables/${tableId}/prompt`, data);
  }

  /* ------------------------------------------------------------------------ */
  /* Table Records (data rows)                                                */
  /* ------------------------------------------------------------------------ */

  /**
   * Get table records
   */
  getDbTableRecords(
    dbId: string,
    tableId: string,
    params: GetDbTableRecordsParams
  ): Promise<ApiResponseData<DbTableRecordsList>> {
    const normalized = {
      ...params,
      limit: String(params.limit),
    } as unknown as { limit: string; offset: number; order?: string };
    return this.request('get', `/data-dbs/${dbId}/tables/${tableId}/records`, undefined, {
      params: normalized,
    });
  }

  /**
   * Create new records
   */
  createDbTableRecords(
    dbId: string,
    tableId: string,
    data: CreateDbTableRecordsRequest
  ): Promise<ApiResponseData<{ result: 'success' | 'fail' }>> {
    return this.request('post', `/data-dbs/${dbId}/tables/${tableId}/records`, data);
  }

  /**
   * Update existing records (by id)
   */
  updateDbTableRecords(
    dbId: string,
    tableId: string,
    data: UpdateDbTableRecordsRequest
  ): Promise<ApiResponseData<{ result: 'success' | 'fail' }>> {
    return this.request('put', `/data-dbs/${dbId}/tables/${tableId}/records`, data);
  }

  /**
   * Delete existing records (by ids)
   */
  deleteDbTableRecords(
    dbId: string,
    tableId: string,
    ids: number[]
  ): Promise<ApiResponseData<{ result: 'success' | 'fail' }>> {
    const normalizedIds = ids.map(n => Number(n)).filter(n => Number.isFinite(n));
    const params = new URLSearchParams();
    normalizedIds.forEach(id => params.append('ids', String(id)));
    return this.request(
      'delete',
      `/data-dbs/${dbId}/tables/${tableId}/records?${params.toString()}`
    );
  }

  /* ------------------------------------------------------------------------ */
  /* SQL Operations (history)                                                 */
  /* ------------------------------------------------------------------------ */

  /**
   * Get SQL operation records for a DB
   */
  getDbSqlOperations(
    dbId: string,
    params: GetDbSqlOperationsParams
  ): Promise<ApiResponseData<DbSqlOperationList>> {
    return this.request('get', `/data-dbs/${dbId}/sql-operations`, undefined, { params });
  }

  /* ------------------------------------------------------------------------ */
  /* Batch Import                                                             */
  /* ------------------------------------------------------------------------ */

  /**
   * Download table template file
   */
  downloadDbTableTemplate(dbId: string, tableId: string): Promise<Blob> {
    return this.request('get', `/data-dbs/${dbId}/tables/${tableId}/template`, undefined, {
      responseType: 'blob',
    });
  }

  /**
   * Import records from uploaded file
   */
  importDbTableRecords(
    dbId: string,
    tableId: string,
    data: ImportDbTableRecordsRequest
  ): Promise<ApiResponseData<ImportDbTableRecordsData>> {
    return this.request('post', `/data-dbs/${dbId}/tables/${tableId}/records/import`, data);
  }

  /**
   * Analyze a spreadsheet and infer a new low-code table schema.
   */
  analyzeExcelImport(
    dbId: string,
    data: AnalyzeExcelImportRequest
  ): Promise<ApiResponseData<AnalyzeExcelImportData>> {
    return this.request('post', `/data-dbs/${dbId}/excel-import/analyze`, data, {
      timeout: 300000,
    });
  }

  /**
   * Confirm inferred schema, create the table, and import rows.
   */
  confirmExcelImport(
    dbId: string,
    jobId: string,
    data: ConfirmExcelImportRequest
  ): Promise<ApiResponseData<ConfirmExcelImportData>> {
    return this.request('post', `/data-dbs/${dbId}/excel-import/jobs/${jobId}/import`, data, {
      timeout: 600000,
    });
  }

  recognizeExcelImport(
    dbId: string,
    jobId: string,
    data: RecognizeExcelImportRequest
  ): Promise<ApiResponseData<RecognizeExcelImportData>> {
    return this.request('post', `/data-dbs/${dbId}/excel-import/jobs/${jobId}/recognize`, data, {
      timeout: 300000,
    });
  }

  getExcelImportJob(dbId: string, jobId: string): Promise<ApiResponseData<ExcelImportJob>> {
    return this.request('get', `/data-dbs/${dbId}/excel-import/jobs/${jobId}`);
  }

  getExcelImportErrors(
    dbId: string,
    jobId: string,
    params: { limit: number; offset: number }
  ): Promise<ApiResponseData<ExcelImportErrorList>> {
    return this.request('get', `/data-dbs/${dbId}/excel-import/jobs/${jobId}/errors`, undefined, {
      params,
    });
  }
}

export const dbService = new DbService();
export default dbService;
