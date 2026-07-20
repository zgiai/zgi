import { inferWorkflowBillingCodeFromValue } from '@/utils/workflow/billing';

export type WorkflowTestUserErrorKey =
  | 'modelRouteUnavailable'
  | 'modelRouteUnavailableNamed'
  | 'modelPricingNotConfigured'
  | 'defaultModelNotConfigured'
  | 'organizationBalanceInsufficient'
  | 'workspaceQuotaInsufficient'
  | 'channelBalanceInsufficient'
  | 'modelServiceUnavailable'
  | 'requestTimedOut'
  | 'networkUnavailable'
  | 'unknown';

export interface WorkflowTestErrorTranslator {
  (key: WorkflowTestUserErrorKey, values?: Record<string, string | number | Date>): string;
}

interface WorkflowTestUserError {
  key: WorkflowTestUserErrorKey;
  values?: Record<string, string | number | Date>;
}

const MODEL_ROUTE_PATTERNS = [
  /no enabled route for model\s+["']([^"']+)["']/i,
  /model not found:[\s\S]*?model\s+["']([^"']+)["']/i,
];

function extractUnavailableModel(message: string) {
  for (const pattern of MODEL_ROUTE_PATTERNS) {
    const match = message.match(pattern);
    if (match?.[1]) return match[1].trim();
  }
  return '';
}

export function classifyWorkflowTestError(message?: string | null): WorkflowTestUserError | null {
  const source = message?.trim();
  if (!source) return null;

  const normalized = source.toLowerCase();
  const billingCode = inferWorkflowBillingCodeFromValue(source);
  if (billingCode === '207014') return { key: 'modelPricingNotConfigured' };
  if (billingCode === '207013') return { key: 'channelBalanceInsufficient' };
  if (billingCode === '207012') return { key: 'workspaceQuotaInsufficient' };
  if (billingCode === '207011') return { key: 'organizationBalanceInsufficient' };

  const model = extractUnavailableModel(source);
  if (model) {
    return { key: 'modelRouteUnavailableNamed', values: { model } };
  }
  if (
    normalized.includes('no enabled routes found') ||
    normalized.includes('no provider available') ||
    normalized.includes('model unavailable')
  ) {
    return { key: 'modelRouteUnavailable' };
  }
  if (
    normalized.includes('workflow test model is not configured') ||
    normalized.includes('judge model is not configured') ||
    normalized.includes('model field is required') ||
    normalized.includes('provider field is required')
  ) {
    return { key: 'defaultModelNotConfigured' };
  }
  if (
    normalized.includes('deadline exceeded') ||
    normalized.includes('execution timed out') ||
    normalized.includes('request timeout') ||
    normalized.includes('request timed out')
  ) {
    return { key: 'requestTimedOut' };
  }
  if (
    normalized.includes('network error') ||
    normalized.includes('connection refused') ||
    normalized.includes('connection reset') ||
    normalized.includes('failed to connect')
  ) {
    return { key: 'networkUnavailable' };
  }
  if (
    normalized.includes('all providers failed') ||
    normalized.includes('current user api does not support http call') ||
    normalized.includes('upstream service error')
  ) {
    return { key: 'modelServiceUnavailable' };
  }

  return null;
}

export function getWorkflowTestUserError(
  message: string | null | undefined,
  t: WorkflowTestErrorTranslator
) {
  const error = classifyWorkflowTestError(message);
  return error ? t(error.key, error.values) : null;
}

export function localizeWorkflowTestError(
  message: string | null | undefined,
  t: WorkflowTestErrorTranslator,
  fallback?: string
) {
  const localized = getWorkflowTestUserError(message, t);
  if (localized) return localized;

  const source = message?.trim();
  if (!source) return fallback || t('unknown');
  if (/\p{Script=Han}/u.test(source)) return source;
  return fallback || t('unknown');
}
