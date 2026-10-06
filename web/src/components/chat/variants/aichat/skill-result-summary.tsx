'use client';

import { useMemo } from 'react';
import { useT, type ScopedTranslations } from '@/i18n/translations';
import { cn } from '@/lib/utils';
import { sanitizeTimelineDisplayPayload } from '@/components/chat/variants/aichat/timeline-display-safety';

const KNOWLEDGE_RESULT_KEYS = [
  'status',
  'fallback_used',
  'result_count',
  'top_score',
  'warnings',
  'source_summary',
] as const;

const DATABASE_RESULT_KEYS = [
  'database_name',
  'schema_name',
  'table_name',
  'databases_count',
  'tables_count',
  'columns_count',
  'records_count',
  'affected_rows',
  'total_num',
  'has_more',
] as const;

const RESULT_LABEL_KEYS = {
  result: 'consoleChat.skills.trace.result.result',
  status: 'consoleChat.skills.trace.result.status',
  fallbackUsed: 'consoleChat.skills.trace.result.fallbackUsed',
  resultCount: 'consoleChat.skills.trace.result.resultCount',
  topScore: 'consoleChat.skills.trace.result.topScore',
  warnings: 'consoleChat.skills.trace.result.warnings',
  sources: 'consoleChat.skills.trace.result.sources',
  databaseName: 'consoleChat.skills.trace.result.databaseName',
  schemaName: 'consoleChat.skills.trace.result.schemaName',
  tableName: 'consoleChat.skills.trace.result.tableName',
  databasesCount: 'consoleChat.skills.trace.result.databasesCount',
  tablesCount: 'consoleChat.skills.trace.result.tablesCount',
  columnsCount: 'consoleChat.skills.trace.result.columnsCount',
  recordsCount: 'consoleChat.skills.trace.result.recordsCount',
  affectedRows: 'consoleChat.skills.trace.result.affectedRows',
  totalNum: 'consoleChat.skills.trace.result.totalNum',
  hasMore: 'consoleChat.skills.trace.result.hasMore',
  integration: 'consoleChat.skills.trace.result.integration',
  action: 'consoleChat.skills.trace.result.action',
  connection: 'consoleChat.skills.trace.result.connection',
  attemptCount: 'consoleChat.skills.trace.result.attemptCount',
  operationStatus: 'consoleChat.skills.trace.result.operationStatus',
  errorCode: 'consoleChat.skills.trace.result.errorCode',
  reasonCode: 'consoleChat.skills.trace.result.reasonCode',
  providerErrorCode: 'consoleChat.skills.trace.result.providerErrorCode',
  retrySafe: 'consoleChat.skills.trace.result.retrySafe',
  diagnostic: 'consoleChat.skills.trace.result.diagnostic',
} as const;

const OMITTED_RESULT_KEYS = new Set([
  'context',
  'context_blocks',
  'retriever_resources',
  'graph_executions',
  'data_source',
  'data_source_id',
  'table',
  'table_id',
  'physical_table_id',
  'physical_table_name',
  'records',
  'columns',
]);

interface ResultSummaryRow {
  key: string;
  labelKey: keyof typeof RESULT_LABEL_KEYS;
  value: string;
}

type WebappTranslator = ScopedTranslations<'webapp'>;

function isRecord(value: unknown): value is Record<string, unknown> {
  return Boolean(value && typeof value === 'object' && !Array.isArray(value));
}

function formatScalar(value: unknown): string | null {
  if (value === undefined || value === null || value === '') return null;
  if (typeof value === 'string') return value;
  if (typeof value === 'number') return Number.isFinite(value) ? String(value) : null;
  if (typeof value === 'boolean') return value ? 'true' : 'false';
  return null;
}

function formatJSON(value: unknown): string | null {
  if (value === undefined || value === null || value === '') return null;
  const scalar = formatScalar(value);
  if (scalar) return scalar;
  try {
    return JSON.stringify(value, null, 2);
  } catch {
    return String(value);
  }
}

function formatScore(value: unknown): string | null {
  if (typeof value !== 'number' || !Number.isFinite(value)) return formatScalar(value);
  return value.toFixed(4).replace(/0+$/, '').replace(/\.$/, '');
}

function formatWarnings(value: unknown): string | null {
  if (!Array.isArray(value)) return formatJSON(value);
  const warnings = value
    .map(item => formatScalar(item))
    .filter((item): item is string => Boolean(item));
  return warnings.length ? warnings.join('\n') : null;
}

function formatSourceItem(value: unknown): string | null {
  if (!isRecord(value)) return formatJSON(value);
  const parts = [
    formatScalar(value.position) ? `[${formatScalar(value.position)}]` : null,
    formatScalar(value.dataset_name),
    formatScalar(value.document_name),
    formatScalar(value.match_type),
    formatScore(value.score) ? `score=${formatScore(value.score)}` : null,
  ].filter((item): item is string => Boolean(item));
  return parts.length ? parts.join(' / ') : null;
}

function formatSourceSummary(value: unknown): string | null {
  if (!Array.isArray(value)) return formatJSON(value);
  const sources = value
    .map(item => formatSourceItem(item))
    .filter((item): item is string => Boolean(item));
  return sources.length ? sources.join('\n') : null;
}

function sanitizeGenericResult(result: Record<string, unknown>): Record<string, unknown> {
  return Object.fromEntries(
    Object.entries(result).filter(
      ([key, value]) => !OMITTED_RESULT_KEYS.has(key) && value !== undefined
    )
  );
}

function localizedBoolean(value: unknown, t: WebappTranslator): string | null {
  if (typeof value !== 'boolean') return null;
  return value ? t('consoleChat.governance.values.yes') : t('consoleChat.governance.values.no');
}

function localizedResultStatus(value: unknown, t: WebappTranslator): string | null {
  if (typeof value !== 'string' || !value.trim()) return formatScalar(value);
  switch (value.trim().toLowerCase()) {
    case 'success':
    case 'succeeded':
    case 'completed':
      return t('consoleChat.skills.trace.result.statuses.success');
    case 'running':
    case 'pending':
      return t('consoleChat.skills.trace.result.statuses.running');
    case 'failed':
    case 'error':
      return t('consoleChat.skills.trace.result.statuses.failed');
    case 'blocked':
      return t('consoleChat.skills.trace.result.statuses.blocked');
    default:
      return formatScalar(value);
  }
}

function buildKnowledgeRows(
  result: Record<string, unknown>,
  t: WebappTranslator
): ResultSummaryRow[] {
  const rows: ResultSummaryRow[] = [];
  const push = (key: string, labelKey: keyof typeof RESULT_LABEL_KEYS, value: string | null) => {
    if (value) rows.push({ key, labelKey, value });
  };

  push('status', 'status', localizedResultStatus(result.status, t));
  push(
    'fallback_used',
    'fallbackUsed',
    localizedBoolean(result.fallback_used, t) ?? formatScalar(result.fallback_used)
  );
  push('result_count', 'resultCount', formatScalar(result.result_count));
  push('top_score', 'topScore', formatScore(result.top_score));
  push('warnings', 'warnings', formatWarnings(result.warnings));
  push('source_summary', 'sources', formatSourceSummary(result.source_summary));
  return rows;
}

function buildDatabaseRows(
  result: Record<string, unknown>,
  t: WebappTranslator
): ResultSummaryRow[] {
  const rows: ResultSummaryRow[] = [];
  const push = (key: string, labelKey: keyof typeof RESULT_LABEL_KEYS, value: string | null) => {
    if (value) rows.push({ key, labelKey, value });
  };

  push('database_name', 'databaseName', formatScalar(result.database_name));
  push('schema_name', 'schemaName', formatScalar(result.schema_name));
  push('table_name', 'tableName', formatScalar(result.table_name));
  push('databases_count', 'databasesCount', formatScalar(result.databases_count));
  push('tables_count', 'tablesCount', formatScalar(result.tables_count));
  push('columns_count', 'columnsCount', formatScalar(result.columns_count));
  push('records_count', 'recordsCount', formatScalar(result.records_count));
  push('affected_rows', 'affectedRows', formatScalar(result.affected_rows));
  push('total_num', 'totalNum', formatScalar(result.total_num));
  push(
    'has_more',
    'hasMore',
    localizedBoolean(result.has_more, t) ?? formatScalar(result.has_more)
  );
  return rows;
}

function buildResultRows(result: Record<string, unknown>, t: WebappTranslator): ResultSummaryRow[] {
  const hasKnowledgeFields = KNOWLEDGE_RESULT_KEYS.some(key => result[key] !== undefined);
  if (hasKnowledgeFields) {
    const rows = buildKnowledgeRows(result, t);
    if (rows.length) return rows;
  }

  const hasDatabaseFields = DATABASE_RESULT_KEYS.some(key => result[key] !== undefined);
  if (hasDatabaseFields) {
    const rows = buildDatabaseRows(result, t);
    if (rows.length) return rows;
  }

  const sanitized = sanitizeGenericResult(result);
  if (Object.keys(sanitized).length === 0) return [];
  const formatted = formatJSON(sanitized);
  return formatted ? [{ key: 'result', labelKey: 'result', value: formatted }] : [];
}

interface AIChatSkillResultSummaryProps {
  invocation?: unknown;
  result?: Record<string, unknown> | null;
  skillId?: string;
  toolName?: string;
  className?: string;
}

export function AIChatSkillResultSummary({
  result,
  className,
}: AIChatSkillResultSummaryProps) {
  const t = useT('webapp');
  const summary = useMemo(() => {
    const safeResult = sanitizeTimelineDisplayPayload(
      result,
      t('consoleChat.skills.trace.values.hidden'),
      t('consoleChat.skills.trace.values.truncated')
    );
    if (!isRecord(safeResult)) return { rows: [], tone: null };
    return {
      rows: buildResultRows(safeResult, t),
      tone: null,
    };
  }, [result, t]);

  const { rows, tone } = summary;

  if (!rows.length) return null;

  const title =
    tone === 'error'
      ? t('consoleChat.skills.trace.result.summaryTitles.failed')
      : tone === 'warning'
        ? t('consoleChat.skills.trace.result.summaryTitles.warning')
        : tone === 'running'
          ? t('consoleChat.skills.trace.result.summaryTitles.running')
          : tone === 'success'
            ? t('consoleChat.skills.trace.result.summaryTitles.success')
            : t('consoleChat.skills.trace.debug.result');

  return (
    <div
      className={cn(
        'rounded-md p-2 text-[11px]',
        tone === 'error'
          ? 'bg-destructive/10'
          : tone === 'warning'
            ? 'bg-amber-500/10'
            : tone === 'running'
              ? 'bg-blue-500/10'
              : 'bg-emerald-500/5',
        className
      )}
    >
      <div
        className={cn(
          'mb-1 font-medium',
          tone === 'error'
            ? 'text-destructive'
            : tone === 'warning'
              ? 'text-amber-700 dark:text-amber-300'
              : tone === 'running'
                ? 'text-blue-700 dark:text-blue-300'
                : 'text-emerald-700 dark:text-emerald-300'
        )}
      >
        {title}
      </div>
      <dl className="grid gap-1">
        {rows.map(row => (
          <div key={row.key} className="grid grid-cols-[88px_minmax(0,1fr)] gap-2">
            <dt className="text-muted-foreground">{t(RESULT_LABEL_KEYS[row.labelKey])}</dt>
            <dd className="min-w-0 max-h-40 overflow-auto whitespace-pre-wrap break-all font-mono text-foreground/80">
              {row.value}
            </dd>
          </div>
        ))}
      </dl>
    </div>
  );
}
