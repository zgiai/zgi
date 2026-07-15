// DB module types – strictly typed, no `any`

// Minimal DB entity for card display and detail
export interface Db {
  id: string;
  name: string;
  description: string;
  workspace_id: string;
  provider?: string;
  schema_name?: string;
  created_by: string;
  created_at: number;
  updated_by?: string;
  updated_at?: number;
  icon: string | null;
  icon_type: 'text' | 'image' | null;
  icon_background: string | null;
  icon_url: string | null;
  can_edit: boolean;
  workspace?: {
    id: string;
    name: string;
  };
}

export interface CreateDbRequest {
  name: string;
  description?: string;
  workspace_id?: string;
  icon_type: 'text' | 'image';
  icon: string;
  icon_background: string;
}

export interface UpdateDbRequest {
  name?: string;
  description?: string;
  workspace_id?: string;
  icon_type?: 'text' | 'image';
  icon?: string;
  icon_background?: string;
}

export interface DeleteDbResponse {
  result: 'success' | 'fail';
}

/* -------------------------------------------------------------------------- */
/* Tables (under a DB)                                                        */
/* -------------------------------------------------------------------------- */

// Table entity based on provided API structure
export interface DbTable {
  id: string;
  organization_id?: string;
  data_source_id: string;
  name: string;
  table_id: number;
  table_name: string;
  schema_name?: string;
  description: string;
  created_by: string;
  updated_by: string;
  created_at: string; // ISO timestamp
  updated_at: string; // ISO timestamp
}

// Create table request body
export interface CreateDbTableRequest {
  description: string;
  name: string;
}

// Update table request body (only allowed fields)
export interface UpdateDbTableRequest {
  name?: string;
  description?: string;
}

/* -------------------------------------------------------------------------- */
/* Table Columns (structure under a table)                                    */
/* -------------------------------------------------------------------------- */

// Column type enum – strictly follow provided declaration
export enum Type {
  Boolean = 'boolean',
  Integer = 'integer',
  Numeric = 'numeric',
  Text = 'text',
  Timestamp = 'timestamp',
}

// Table column entity based on API examples
export interface DbTableColumn {
  id: string;
  name: string;
  display_name?: string;
  source_column_name?: string;
  description: string;
  type: Type;
  is_required: boolean;
  // Whether the column is a system field; system fields are read-only in UI
  is_system_field?: boolean;
}

// GET /console/api/data-dbs/{db_id}/tables/{table_id}/columns response payload
export interface DbTableColumnsPayload {
  columns: DbTableColumn[];
  content?: string;
}

// PUT /console/api/data-dbs/{db_id}/tables/{table_id}/columns response payload
// Server returns only a message, not columns; use GET to sync columns after PUT
export interface UpdateDbTableColumnsResponse {
  message: string;
}

// Column update input for PUT request – `id` is optional for newly added columns
export interface DbTableColumnUpdateInput {
  id?: string;
  name: string;
  description: string;
  type: Type;
  is_required: boolean;
}

// PUT request body to update table columns
export interface UpdateDbTableColumnsRequest {
  columns: DbTableColumnUpdateInput[];
}

/* -------------------------------------------------------------------------- */
/* Table Records (data under a table)                                         */
/* -------------------------------------------------------------------------- */

// Allowed record value primitive types
export type DbRecordPrimitive = string | number | boolean | null;

// Allowed record array types (observed uuid is number[]; keep strict union)
export type DbRecordArray = number[] | string[];

// A single cell value in a record
export type DbRecordValue = DbRecordPrimitive | DbRecordArray | undefined;

// A single table record (row). Index signature allows dynamic fields.
export interface DbTableRecord {
  // System-provided identifier (returned by backend on GET)
  id?: number;
  // Other dynamic fields – strictly typed to allowed values only
  [key: string]: DbRecordValue;
}

// GET query params for records list
export interface GetDbTableRecordsParams {
  /** page size */
  limit: number;
  /** offset for pagination */
  offset: number;
  /** column_name + ASC/DESC, e.g., "created_time DESC" */
  order?: string;
}

// GET response payload for records list
export interface DbTableRecordsList {
  has_more: boolean;
  total_num: number;
  data: DbTableRecord[];
}

// POST request body to create new records
export interface CreateDbTableRecordsRequest {
  records: Array<Omit<DbTableRecord, 'id'>>;
}

// PUT request body to update existing records (id is required per item)
export interface UpdateDbTableRecordsRequest {
  records: Array<Required<Pick<DbTableRecord, 'id'>> & Partial<DbTableRecord>>;
}

/* -------------------------------------------------------------------------- */
/* Batch ingest from files into a table                                       */
/* -------------------------------------------------------------------------- */

// Request body for POST /console/api/data-dbs/ingest-file-to-table
export interface IngestFileToTableRequest {
  file_id: string;
  prompt: string;
  table_id: string;
  model: AiModelRef;
}

export type TableIngestStage = 'parse' | 'recognition';

export interface ParseFileForTableIngestRequest {
  file_id: string;
  table_id: string;
}

export interface ParseFileForTableIngestData {
  file_id?: string;
  file_name?: string;
  message: string;
  content?: string;
  extraction?: FileIngestExtractionInfo;
  stage?: TableIngestStage;
  error?: string;
}

export interface ExtractTextToTableRecordsRequest {
  file_id?: string;
  table_id: string;
  content: string;
  content_hash?: string;
  prompt: string;
  model: AiModelRef;
}

export interface ExtractTextToTableRecordsData {
  file_id?: string;
  message: string;
  records: DbTableRecord[];
  columns: DbTableColumn[];
  field_extraction?: FileIngestFieldExtraction;
  content_hash?: string;
  stage?: TableIngestStage;
  error?: string;
}

export interface IngestFileToTableData {
  file_id?: string;
  file_name?: string;
  message: string;
  records: DbTableRecord[];
  columns: DbTableColumn[];
  content?: string;
  extraction?: FileIngestExtractionInfo;
  field_extraction?: FileIngestFieldExtraction;
  stage?: TableIngestStage;
  error?: string;
}

// Request body for POST /console/api/data-dbs/batch-ingest-file-to-table
export interface BatchIngestFileToTableRequest {
  file_ids: string[];
  prompt: string;
  table_id: string;
  model: AiModelRef;
}

// Single file ingest result item
export interface BatchIngestResultItem {
  file_id: string;
  file_name: string;
  message: string;
  records: DbTableRecord[];
  content?: string;
  extraction?: FileIngestExtractionInfo;
  field_extraction?: FileIngestFieldExtraction;
  stage?: TableIngestStage;
  error?: string;
}

export interface FileIngestExtractionInfo {
  primary_strategy?: string;
  actual_strategy?: string;
  fallback_reason?: string;
  source_type?: string;
  content_hash?: string;
  attempts?: FileIngestAttempt[];
}

export interface FileIngestAttempt {
  method: 'file_parse' | string;
  status: 'completed' | 'failed' | string;
  result?: 'content' | 'records' | 'no_records' | 'empty_content' | 'error' | string;
  reason?: string;
  duration_ms?: number;
  record_count?: number;
}

export interface FileIngestFieldExtraction {
  records?: FileIngestRecordExtraction[];
}

export interface FileIngestRecordExtraction {
  fields?: FileIngestFieldMatch[];
}

export interface FileIngestFieldMatch {
  column_id: string;
  column_name?: string;
  value?: unknown;
  raw_value?: unknown;
  normalized_value?: unknown;
  normalization_status?: 'valid' | 'normalized' | 'invalid' | 'empty' | string;
  normalization_reason?: string;
  evidence?: string;
  confidence?: number;
  reason?: string;
}

// Response data payload for batch ingest API
export interface BatchIngestFileToTableData {
  results: Record<string, BatchIngestResultItem>;
  columns: DbTableColumn[];
  total_count?: number;
  success_count?: number;
  failed_count?: number;
}

/* -------------------------------------------------------------------------- */
/* Table prompt (AI ingest hint for a specific table)                          */
/* -------------------------------------------------------------------------- */

// GET/PUT /console/api/data-dbs/{db_id}/tables/{table_id}/prompt
export interface DbTablePrompt {
  id: string;
  table_id: string;
  prompt: string;
  created_by: string;
  updated_by: string;
  created_at: string; // ISO timestamp
  updated_at: string; // ISO timestamp
}

export interface UpdateDbTablePromptRequest {
  prompt: string;
}

/* -------------------------------------------------------------------------- */
/* AI Model Reference for analyze and ingest operations                      */
/* -------------------------------------------------------------------------- */

// Model reference for AI operations (analyze, ingest)
export interface AiModelRef {
  provider: string;
  name: string;
}

// Request body for POST /console/api/data-dbs/analyze-file-for-table
export interface AnalyzeFileForTableRequest {
  prompt: string;
  file_id?: string;
  data_source_id?: string;
  model: AiModelRef;
}

/* -------------------------------------------------------------------------- */
/* SQL Operations (history for a DB)                                          */
/* -------------------------------------------------------------------------- */

// Operation type enum as per API contract
export enum OperationType {
  Create = 'create',
  Delete = 'delete',
  Query = 'query',
  Update = 'update',
  Import = 'import',
}

// Execution status enum as per API contract
export enum SqlOperationStatus {
  Failed = 'failed',
  Success = 'success',
}

// Query parameters for GET /console/api/data-dbs/{db_id}/sql-operations
export interface GetDbSqlOperationsParams {
  created_by?: string;
  end_time?: string;
  // API expects string values for limit and page
  limit: string;
  operation_type?: OperationType;
  page: string;
  start_time?: string;
  status?: SqlOperationStatus;
}

// Single SQL operation record item
export interface DbSqlOperation {
  created_at: string;
  created_by: string;
  created_by_name: string;
  data_source_id: string;
  data_source_name: string;
  end_time: string;
  organization_id: string;
  id: string;
  operation_type: OperationType;
  sql_statement: string;
  start_time: string;
  status: SqlOperationStatus;
  table_id: string;
  table_name: string;
}

// Response payload data for SQL operations list
export interface DbSqlOperationList {
  data: DbSqlOperation[];
  has_more: boolean;
  limit: number;
  page: number;
  total: number;
}

/* -------------------------------------------------------------------------- */
/* Batch Import (template download and records import)                        */
/* -------------------------------------------------------------------------- */

// POST /console/api/data-dbs/{db_id}/tables/{table_id}/records/import response data
export interface ImportDbTableRecordsData {
  affected_rows: number;
  failed_count: number;
  total_count: number;
}

export interface ImportDbTableRecordsRequest {
  upload_file_id: string;
  skip_unmatched_columns?: boolean;
}

/* -------------------------------------------------------------------------- */
/* Excel Import: create a low-code table from a spreadsheet                    */
/* -------------------------------------------------------------------------- */

export type ExcelImportStatus =
  | 'analyzing'
  | 'needs_review'
  | 'importing'
  | 'completed'
  | 'failed'
  | 'partial_failed';

export interface AnalyzeExcelImportRequest {
  upload_file_id: string;
  sheet_name?: string;
  header_row?: number;
  sample_size?: number;
}

export interface ExcelImportSheet {
  name: string;
  row_count: number;
  column_count: number;
  hidden: boolean;
  recommended: boolean;
}

export interface ExcelImportWarning {
  code: string;
  message: string;
  row_index?: number;
  column_name?: string;
}

export interface InferredExcelColumn {
  source_column: string;
  source_column_index: number;
  name: string;
  display_name: string;
  type: Type;
  is_required: boolean;
  description: string;
  confidence: number;
  sample_values: string[];
  warnings: ExcelImportWarning[];
  enabled?: boolean;
}

export interface ExcelImportPreviewRow {
  row_index: number;
  values: Record<string, DbRecordPrimitive>;
}

export interface AnalyzeExcelImportData {
  job_id: string;
  source: {
    file_name: string;
    source_type: 'excel' | 'csv';
    sheets: ExcelImportSheet[];
  };
  selection: {
    sheet_name: string;
    header_row: number;
    start_row: number;
  };
  columns: InferredExcelColumn[];
  preview_rows: ExcelImportPreviewRow[];
  warnings: ExcelImportWarning[];
}

export interface ConfirmExcelImportRequest {
  table: {
    name: string;
    description?: string;
  };
  selection: {
    sheet_name: string;
    header_row: number;
    start_row: number;
  };
  columns: InferredExcelColumn[];
  options: {
    error_policy: 'fail_fast' | 'skip_invalid_rows';
    empty_row_policy: 'skip' | 'error';
    batch_size?: number;
  };
}

export interface RecognizeExcelImportTable {
  name: string;
  description: string;
}

export interface RecognizeExcelImportSource {
  file_name?: string;
  sheet_name?: string;
}

export interface RecognizeExcelImportRequest {
  table: RecognizeExcelImportTable;
  source?: RecognizeExcelImportSource;
  columns: InferredExcelColumn[];
  model: AiModelRef;
  operator_language?: string;
}

export interface RecognizeExcelImportData {
  table: RecognizeExcelImportTable;
  columns: InferredExcelColumn[];
}

export interface ExcelImportFailedItem {
  row_index: number;
  column_name?: string;
  raw_value?: string;
  error_code: string;
  error_message: string;
}

export interface ConfirmExcelImportData {
  job_id: string;
  table_id: string;
  status: ExcelImportStatus;
  total_rows: number;
  imported_rows: number;
  failed_rows: number;
  failed_items: ExcelImportFailedItem[];
}

export interface ExcelImportJob {
  id: string;
  organization_id: string;
  workspace_id?: string;
  data_source_id: string;
  table_id?: string;
  upload_file_id?: string;
  source_type: 'excel' | 'csv';
  source_file_name: string;
  status: ExcelImportStatus;
  total_rows: number;
  valid_rows: number;
  imported_rows: number;
  failed_rows: number;
  sheet_name?: string;
  header_row?: number;
  start_row?: number;
  created_by: string;
  updated_by: string;
  created_at: string;
  updated_at: string;
}

export interface ExcelImportErrorList {
  data: ExcelImportFailedItem[];
  has_more: boolean;
  limit: number;
  offset: number;
  total_num: number;
}
