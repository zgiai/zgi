import type { DatabaseExecutionSettings } from '../call-database/config';
import type { DatabaseType } from '../call-database/config';

export interface SqlGeneratorModelConfig {
  provider: string;
  name: string;
  mode: 'chat' | 'completion';
  completion_params: Record<string, string | number | boolean>;
}

// Source ref for sql-generator (schema field name differs from call-database)
export interface SqlGeneratorSourceRef {
  id: string;
  name?: string;
  schema: string;
  type: DatabaseType;
}

// Table ref for sql-generator has both id (string) and table_id (number)
export interface SqlGeneratorTableRef {
  id: string; // unique id from table list (string)
  table_id?: number; // numeric id from table list (optional when unavailable)
  schema: string;
  name: string;
  label?: string;
  columns?: string[];
}

export interface SqlGeneratorDataSource {
  source: SqlGeneratorSourceRef;
  tables: SqlGeneratorTableRef[];
}

export interface SqlGeneratorInnerData {
  model: SqlGeneratorModelConfig;
  data_source: SqlGeneratorDataSource;
  prompt: string;
  execution: DatabaseExecutionSettings;
}

export interface SqlGeneratorNodeData {
  type: 'sql-generator';
  title: string;
  desc: string;
  data: SqlGeneratorInnerData;
  isInLoop: boolean;
  isInIteration: boolean;
}

export const DEFAULT_SQL_GENERATOR_MODEL: SqlGeneratorModelConfig = {
  provider: '',
  name: '',
  mode: 'chat',
  completion_params: {},
};

export const DEFAULT_SQL_GENERATOR_SOURCE: SqlGeneratorSourceRef = {
  id: '',
  name: '',
  schema: 'public',
  type: 'postgres',
};

export const DEFAULT_SQL_GENERATOR_DATA_SOURCE: SqlGeneratorDataSource = {
  source: { ...DEFAULT_SQL_GENERATOR_SOURCE },
  tables: [],
};

export const DEFAULT_SQL_GENERATOR_PROMPT = `User question: {{#start.query#}}

Generate an SQL query using only the allowed schema and tables provided above. Return only the SQL query without any explanation.`;

export const DEFAULT_SQL_GENERATOR_NODE_DATA: SqlGeneratorNodeData = {
  type: 'sql-generator',
  title: 'SQL Generator',
  desc: '',
  data: {
    model: { ...DEFAULT_SQL_GENERATOR_MODEL },
    data_source: { ...DEFAULT_SQL_GENERATOR_DATA_SOURCE },
    prompt: DEFAULT_SQL_GENERATOR_PROMPT,
    execution: {
      timeout_seconds: 60,
      max_retries: 3,
    },
  },
  isInLoop: false,
  isInIteration: false,
};

// Validation
import type { ValidationResult, ValidationError } from '../common/validation';

export const checkValid = (nodeData: SqlGeneratorNodeData): ValidationResult => {
  const errors: ValidationError[] = [];
  const warnings: ValidationError[] = [];

  const model = nodeData.data?.model;
  const ds = nodeData.data?.data_source?.source;
  const tables = Array.isArray(nodeData.data?.data_source?.tables)
    ? nodeData.data.data_source.tables
    : [];
  const prompt = nodeData.data?.prompt ?? '';

  // Model validation (warnings)
  if (!model?.provider || !model?.name) {
    warnings.push({ code: 'sqlGenerator.validation.modelRequired' });
  }

  // Prompt validation (error)
  if (!prompt || prompt.trim() === '') {
    errors.push({ code: 'sqlGenerator.validation.promptRequired' });
  }

  // Data source validation (error)
  if (!ds || !ds.id || ds.id.trim() === '') {
    errors.push({ code: 'sqlGenerator.validation.dataSourceRequired' });
  }

  // Table selection validation (error)
  if (tables.length === 0) {
    errors.push({ code: 'sqlGenerator.validation.tableSelectionRequired' });
  }

  // Execution settings validation (optional)
  const exec = nodeData.data?.execution;
  if (exec) {
    if (typeof exec.timeout_seconds === 'number' && exec.timeout_seconds < 0) {
      errors.push({ code: 'sqlGenerator.validation.timeoutPositive' });
    }
    if (typeof exec.max_retries === 'number' && exec.max_retries < 0) {
      errors.push({ code: 'sqlGenerator.validation.retriesPositive' });
    }
  }

  return { isValid: errors.length === 0, errors, warnings };
};
