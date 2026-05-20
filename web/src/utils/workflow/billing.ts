import type {
  WorkflowPrecheckResult,
  WorkflowPrecheckWarning,
  WorkflowRunBillingError,
} from '@/services/types/workflow';
import { normalizeAiCreditMetricValue } from '@/utils/ai-credits';

export type WorkflowBillingTranslationScope = 'agents' | 'webapp';

export interface WorkflowBillingMessageOptions {
  isAdmin?: boolean;
  workspaceId?: string | null;
}

export interface WorkflowTranslator {
  (key: string, values?: Record<string, string | number | Date>): string;
}

const PRECHECK_WARNING_PRIORITY: Record<string, number> = {
  '207009': 0,
  '207008': 1,
  '207010': 2,
};

export function normalizeWorkflowBillingCode(code: unknown): string | undefined {
  if (typeof code === 'number' && Number.isFinite(code)) {
    return String(Math.trunc(code));
  }

  if (typeof code === 'string') {
    const trimmed = code.trim();
    return trimmed.length > 0 ? trimmed : undefined;
  }

  return undefined;
}

export function isWorkflowBillingErrorCode(code: unknown): boolean {
  const normalized = normalizeWorkflowBillingCode(code);
  return normalized === '207011' || normalized === '207012' || normalized === '207013';
}

export function isWorkflowPrecheckWarningCode(code: unknown): boolean {
  const normalized = normalizeWorkflowBillingCode(code);
  return normalized === '207008' || normalized === '207009' || normalized === '207010';
}

export function sortWorkflowPrecheckWarnings(
  warnings: WorkflowPrecheckWarning[]
): WorkflowPrecheckWarning[] {
  return [...warnings].sort((left, right) => {
    const leftPriority = PRECHECK_WARNING_PRIORITY[normalizeWorkflowBillingCode(left.code) ?? ''] ?? 999;
    const rightPriority =
      PRECHECK_WARNING_PRIORITY[normalizeWorkflowBillingCode(right.code) ?? ''] ?? 999;

    if (leftPriority !== rightPriority) {
      return leftPriority - rightPriority;
    }

    return (normalizeWorkflowBillingCode(left.code) ?? '').localeCompare(
      normalizeWorkflowBillingCode(right.code) ?? ''
    );
  });
}

export function getWorkflowPrecheckWarnings(
  result?: WorkflowPrecheckResult | null
): WorkflowPrecheckWarning[] {
  if (!result) return [];

  const warnings = Array.isArray(result.warnings)
    ? result.warnings.filter(Boolean)
    : result.warning
      ? [result.warning]
      : [];

  return sortWorkflowPrecheckWarnings(warnings);
}

export function extractWorkflowRunError(error: unknown): WorkflowRunBillingError | null {
  if (!error) return null;

  if (typeof error === 'string') {
    const trimmed = error.trim();
    if (!trimmed) return null;

    try {
      return extractWorkflowRunError(JSON.parse(trimmed));
    } catch {
      return { message: trimmed };
    }
  }

  if (error instanceof Error) {
    return { message: error.message };
  }

  if (typeof error !== 'object') {
    return null;
  }

  const record = error as Record<string, unknown>;
  const nested =
    record['data'] && typeof record['data'] === 'object'
      ? (record['data'] as Record<string, unknown>)
      : undefined;
  const params =
    record['params'] && typeof record['params'] === 'object'
      ? (record['params'] as Record<string, unknown>)
      : nested?.['params'] && typeof nested['params'] === 'object'
        ? (nested['params'] as Record<string, unknown>)
        : undefined;

  return {
    code: (record['code'] as string | number | undefined) ?? (nested?.['code'] as string | number | undefined),
    message:
      (typeof record['message'] === 'string' ? record['message'] : undefined) ??
      (typeof nested?.['message'] === 'string' ? nested['message'] : undefined),
    params,
  };
}

export function inferWorkflowBillingCodeFromValue(value?: unknown): string | undefined {
  const message =
    typeof value === 'number' && Number.isFinite(value)
      ? String(Math.trunc(value))
      : typeof value === 'string'
        ? value
        : undefined;
  if (!message) return undefined;

  const normalized = message.toLowerCase();
  if (normalized === '207011' || normalized === '207012' || normalized === '207013') {
    return normalized;
  }

  if (
    normalized.includes('private_channel_balance_insufficient') ||
    normalized.includes('channel_balance_insufficient') ||
    normalized.includes('渠道余额不足') ||
    normalized.includes('为渠道充值') ||
    normalized.includes('切换渠道')
  ) {
    return '207013';
  }

  if (
    normalized.includes('workspace_quota') ||
    normalized.includes('workspace_balance') ||
    normalized.includes('工作空间额度')
  ) {
    return '207012';
  }

  if (
    normalized.includes('organization_balance') ||
    normalized.includes('account_balance') ||
    normalized.includes('组织余额不足')
  ) {
    return '207011';
  }

  return undefined;
}

export function resolveWorkflowBillingErrorCode(
  code?: unknown,
  message?: string
): string | undefined {
  const normalizedCode = normalizeWorkflowBillingCode(code);
  if (isWorkflowBillingErrorCode(normalizedCode)) {
    return normalizedCode;
  }

  return inferWorkflowBillingCodeFromValue(normalizedCode) ?? inferWorkflowBillingCodeFromValue(message);
}

function getPrecheckMetricValue(
  warning: WorkflowPrecheckWarning,
  key: 'current_value' | 'threshold'
): number | string | undefined {
  const direct = warning[key];
  if (typeof direct === 'number' || typeof direct === 'string') {
    return direct;
  }

  const nested = warning.params?.[key];
  if (typeof nested === 'number' || typeof nested === 'string') {
    return nested;
  }

  return undefined;
}

export function getWorkflowPrecheckWarningMessage(
  t: WorkflowTranslator,
  scope: WorkflowBillingTranslationScope,
  warning: WorkflowPrecheckWarning
): { title: string; description: string; code?: string } {
  const code = normalizeWorkflowBillingCode(warning.code);
  const prefix = scope === 'agents' ? 'agents.workflow.precheckWarnings' : 'webapp.billing.precheckWarnings';
  const currentValue = normalizeAiCreditMetricValue(getPrecheckMetricValue(warning, 'current_value'));
  const threshold = normalizeAiCreditMetricValue(getPrecheckMetricValue(warning, 'threshold'));
  const values = {
    currentValue: currentValue ?? '-',
    threshold: threshold ?? '-',
  };

  if (code === '207008' || code === '207009' || code === '207010') {
    return {
      code,
      title: t(`${prefix}.${code}.title`),
      description: t(`${prefix}.${code}.description`, values),
    };
  }

  return {
    code,
    title: t(`${prefix}.unknown.title`),
    description: warning.message?.trim()
      ? warning.message.trim()
      : t(`${prefix}.unknown.description`, values),
  };
}

export function getWorkflowBillingActionHref(
  code: unknown,
  workspaceId?: string | null
): string | null {
  const normalized = normalizeWorkflowBillingCode(code);

  switch (normalized) {
    case '207011':
      return '/dashboard/cost-center/overview';
    case '207012':
      return workspaceId
        ? `/dashboard/organization/workspaces/${workspaceId}`
        : '/dashboard/organization/workspaces';
    case '207013':
      return '/dashboard/channel';
    default:
      return null;
  }
}

export function getWorkflowBillingErrorMessage(
  t: WorkflowTranslator,
  scope: WorkflowBillingTranslationScope,
  error: WorkflowRunBillingError | null | undefined,
  options: WorkflowBillingMessageOptions = {}
): { title: string; description: string; actionLabel?: string; href?: string | null } | null {
  if (!error) return null;

  const code = resolveWorkflowBillingErrorCode(error.code, error.message);
  const prefix = scope === 'agents' ? 'agents.workflow.billingErrors' : 'webapp.billing.errors';
  const href = getWorkflowBillingActionHref(code, options.workspaceId);

  if (code && isWorkflowBillingErrorCode(code)) {
    return {
      title: t(`${prefix}.${code}.title`),
      description: options.isAdmin
        ? t(`${prefix}.${code}.description`)
        : t(`${prefix}.contactAdmin`),
      actionLabel:
        options.isAdmin && href ? t(`${prefix}.${code}.action`) : undefined,
      href,
    };
  }

  if (error.message?.trim()) {
    return {
      title:
        scope === 'agents'
          ? t('agents.workflow.errors.executionFailed')
          : t('webapp.chat.workflowRunFailed'),
      description: error.message.trim(),
    };
  }

  return {
    title:
      scope === 'agents'
        ? t('agents.workflow.errors.executionFailed')
        : t('webapp.chat.workflowRunFailed'),
    description:
      scope === 'agents'
        ? t('agents.workflow.errors.executionFailed')
        : t('webapp.chat.workflowRunFailed'),
  };
}
