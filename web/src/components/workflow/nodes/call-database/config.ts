export type DatabaseType = 'postgres' | 'mysql' | 'sqlite' | 'mssql' | 'oracle';

export interface DatabaseSourceRef {
  id: string;
  name?: string;
  type: DatabaseType;
  schema_name?: string;
}

export interface TableRef {
  schema: string;
  name: string;
  label?: string;
  columns?: string[];
  id: string;
  table_id: number;
}

export interface DatabaseExecutionSettings {
  timeout_seconds: number; // 0 or missing means use backend default (e.g. 30s)
  max_retries: number; // 0 means no retry
}

export const DEFAULT_DATABASE_SOURCE: DatabaseSourceRef = {
  id: '',
  type: 'postgres',
  schema_name: 'public',
};

export const DEFAULT_DATABASE_EXECUTION: DatabaseExecutionSettings = {
  timeout_seconds: 30,
  max_retries: 0,
};

export interface CallDatabaseNodeInnerData {
  data_source: DatabaseSourceRef;
  table_selection: TableRef[];
  manual_sql: string;
  execution: DatabaseExecutionSettings;
}

export interface CallDatabaseNodeData {
  type: 'call-database';
  title: string;
  desc: string;
  data: CallDatabaseNodeInnerData;
  isInLoop: boolean;
  isInIteration: boolean;
}

export const DEFAULT_CALL_DATABASE_NODE_DATA: CallDatabaseNodeData = {
  type: 'call-database',
  title: 'Call Database',
  desc: '',
  data: {
    data_source: DEFAULT_DATABASE_SOURCE,
    table_selection: [],
    manual_sql: '',
    execution: DEFAULT_DATABASE_EXECUTION,
  },
  isInLoop: false,
  isInIteration: false,
};

// Validation
import type { ValidationResult, ValidationError } from '../common/validation';

export const checkValid = (nodeData: CallDatabaseNodeData): ValidationResult => {
  const errors: ValidationError[] = [];
  const warnings: ValidationError[] = [];

  const ds = nodeData.data?.data_source;
  const tables = Array.isArray(nodeData.data?.table_selection) ? nodeData.data.table_selection : [];

  if (!ds || !ds.id || ds.id.trim() === '') {
    errors.push({ code: 'callDatabase.validation.dataSourceRequired' });
  }

  if (tables.length === 0) {
    errors.push({ code: 'callDatabase.validation.tableSelectionRequired' });
  }

  const sql = nodeData.data?.manual_sql ?? '';
  if (!sql || sql.trim() === '') {
    errors.push({ code: 'callDatabase.validation.sqlRequired' });
  }

  return { isValid: errors.length === 0, errors, warnings };
};

// Optional outputs type for consumers (run panel / downstream variables)
export interface CallDatabaseNodeOutputs {
  sql: string;
  columns?: string[];
  rows?: Array<Record<string, unknown>>;
  row_count?: number;
  rows_affected?: number;
  duration_ms?: number;
  table_context?: TableRef[];
  data_source_id?: string;
  error?: string;
}
