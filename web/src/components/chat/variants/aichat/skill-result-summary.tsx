'use client';

import { useMemo } from 'react';
import { useT } from '@/i18n/translations';
import { cn } from '@/lib/utils';

const KNOWLEDGE_RESULT_KEYS = [
  'status',
  'fallback_used',
  'result_count',
  'top_score',
  'warnings',
  'source_summary',
] as const;

const RESULT_LABEL_KEYS = {
  result: 'consoleChat.skills.trace.result.result',
  status: 'consoleChat.skills.trace.result.status',
  fallbackUsed: 'consoleChat.skills.trace.result.fallbackUsed',
  resultCount: 'consoleChat.skills.trace.result.resultCount',
  topScore: 'consoleChat.skills.trace.result.topScore',
  warnings: 'consoleChat.skills.trace.result.warnings',
  sources: 'consoleChat.skills.trace.result.sources',
} as const;

const OMITTED_RESULT_KEYS = new Set([
  'context',
  'context_blocks',
  'retriever_resources',
  'graph_executions',
]);

interface ResultSummaryRow {
  key: string;
  labelKey: keyof typeof RESULT_LABEL_KEYS;
  value: string;
}

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

function buildKnowledgeRows(result: Record<string, unknown>): ResultSummaryRow[] {
  const rows: ResultSummaryRow[] = [];
  const push = (key: string, labelKey: keyof typeof RESULT_LABEL_KEYS, value: string | null) => {
    if (value) rows.push({ key, labelKey, value });
  };

  push('status', 'status', formatScalar(result.status));
  push('fallback_used', 'fallbackUsed', formatScalar(result.fallback_used));
  push('result_count', 'resultCount', formatScalar(result.result_count));
  push('top_score', 'topScore', formatScore(result.top_score));
  push('warnings', 'warnings', formatWarnings(result.warnings));
  push('source_summary', 'sources', formatSourceSummary(result.source_summary));
  return rows;
}

function buildResultRows(result: Record<string, unknown>): ResultSummaryRow[] {
  const hasKnowledgeFields = KNOWLEDGE_RESULT_KEYS.some(key => result[key] !== undefined);
  if (hasKnowledgeFields) {
    const rows = buildKnowledgeRows(result);
    if (rows.length) return rows;
  }

  const sanitized = sanitizeGenericResult(result);
  if (Object.keys(sanitized).length === 0) return [];
  const formatted = formatJSON(sanitized);
  return formatted ? [{ key: 'result', labelKey: 'result', value: formatted }] : [];
}

interface AIChatSkillResultSummaryProps {
  result?: Record<string, unknown> | null;
  className?: string;
}

export function AIChatSkillResultSummary({ result, className }: AIChatSkillResultSummaryProps) {
  const t = useT('webapp');
  const rows = useMemo(() => (isRecord(result) ? buildResultRows(result) : []), [result]);

  if (!rows.length) return null;

  return (
    <div className={cn('rounded-md bg-emerald-500/5 p-2 text-[11px]', className)}>
      <div className="mb-1 font-medium text-emerald-700 dark:text-emerald-300">
        {t('consoleChat.skills.trace.debug.result')}
      </div>
      <dl className="grid gap-1">
        {rows.map(row => (
          <div key={row.key} className="grid grid-cols-[88px_minmax(0,1fr)] gap-2">
            <dt className="text-muted-foreground">
              {t(RESULT_LABEL_KEYS[row.labelKey])}
            </dt>
            <dd className="min-w-0 max-h-40 overflow-auto whitespace-pre-wrap break-all font-mono text-foreground/80">
              {row.value}
            </dd>
          </div>
        ))}
      </dl>
    </div>
  );
}
