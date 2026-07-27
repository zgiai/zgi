export const INTEGRATION_ERROR_TRANSLATION_KEYS = {
  integration_disabled: {
    integrations: 'errors.integration_disabled',
    webapp: 'consoleChat.skills.trace.errors.integrationDisabled',
  },
  integration_invalid_input: {
    integrations: 'errors.integration_invalid_input',
    webapp: 'consoleChat.skills.trace.errors.invalidInput',
  },
  integration_sensitive_input_blocked: {
    integrations: 'errors.integration_sensitive_input_blocked',
    webapp: 'consoleChat.skills.trace.errors.sensitiveInputBlocked',
  },
  integration_quota_exceeded: {
    integrations: 'errors.integration_quota_exceeded',
    webapp: 'consoleChat.skills.trace.errors.quotaExceeded',
  },
  integration_auth_invalid: {
    integrations: 'errors.integration_auth_invalid',
    webapp: 'consoleChat.skills.trace.errors.authInvalid',
  },
  integration_budget_exceeded: {
    integrations: 'errors.integration_budget_exceeded',
    webapp: 'consoleChat.skills.trace.errors.budgetExceeded',
  },
  integration_access_denied: {
    integrations: 'errors.integration_access_denied',
    webapp: 'consoleChat.skills.trace.errors.accessDenied',
  },
  integration_rate_limited: {
    integrations: 'errors.integration_rate_limited',
    webapp: 'consoleChat.skills.trace.errors.rateLimited',
  },
  integration_timeout: {
    integrations: 'errors.integration_timeout',
    webapp: 'consoleChat.skills.trace.errors.timeout',
  },
  integration_upstream_unavailable: {
    integrations: 'errors.integration_upstream_unavailable',
    webapp: 'consoleChat.skills.trace.errors.upstreamUnavailable',
  },
  integration_response_invalid: {
    integrations: 'errors.integration_response_invalid',
    webapp: 'consoleChat.skills.trace.errors.responseInvalid',
  },
  integration_audit_failed: {
    integrations: 'errors.integration_audit_failed',
    webapp: 'consoleChat.skills.trace.errors.auditFailed',
  },
  integration_policy_conflict: {
    integrations: 'errors.integration_policy_conflict',
    webapp: 'consoleChat.skills.trace.errors.policyConflict',
  },
  integration_reconnect_required: {
    integrations: 'errors.integration_reconnect_required',
    webapp: 'consoleChat.skills.trace.errors.reconnectRequired',
  },
  integration_connection_expired: {
    integrations: 'errors.integration_connection_expired',
    webapp: 'consoleChat.skills.trace.errors.connectionExpired',
  },
  integration_insufficient_scope: {
    integrations: 'errors.integration_insufficient_scope',
    webapp: 'consoleChat.skills.trace.errors.insufficientScope',
  },
  integration_connection_not_found: {
    integrations: 'errors.integration_connection_not_found',
    webapp: 'consoleChat.skills.trace.errors.connectionNotFound',
  },
  integration_connection_invalid: {
    integrations: 'errors.integration_connection_invalid',
    webapp: 'consoleChat.skills.trace.errors.connectionInvalid',
  },
  integration_connection_conflict: {
    integrations: 'errors.integration_connection_conflict',
    webapp: 'consoleChat.skills.trace.errors.connectionConflict',
  },
  integration_connection_in_use: {
    integrations: 'errors.integration_connection_in_use',
    webapp: 'consoleChat.skills.trace.errors.connectionInUse',
  },
} as const;

export type IntegrationErrorCode = keyof typeof INTEGRATION_ERROR_TRANSLATION_KEYS;
export type IntegrationErrorTranslationSurface = 'integrations' | 'webapp';
export type IntegrationErrorTranslationKey<Surface extends IntegrationErrorTranslationSurface> =
  (typeof INTEGRATION_ERROR_TRANSLATION_KEYS)[IntegrationErrorCode][Surface];

function recordValue(value: unknown, key: string): unknown {
  return value && typeof value === 'object' ? (value as Record<string, unknown>)[key] : undefined;
}

export function integrationErrorCode(value: unknown): IntegrationErrorCode | null {
  if (typeof value !== 'string') return null;
  const normalized = value.trim().toLowerCase();
  if (!normalized) return null;
  if (Object.prototype.hasOwnProperty.call(INTEGRATION_ERROR_TRANSLATION_KEYS, normalized)) {
    return normalized as IntegrationErrorCode;
  }
  const embedded = normalized.match(/\bintegration_[a-z0-9_]+\b/)?.[0];
  return embedded &&
    Object.prototype.hasOwnProperty.call(INTEGRATION_ERROR_TRANSLATION_KEYS, embedded)
    ? (embedded as IntegrationErrorCode)
    : null;
}

export function integrationErrorTranslationKey<Surface extends IntegrationErrorTranslationSurface>(
  value: unknown,
  surface: Surface
): IntegrationErrorTranslationKey<Surface> | null {
  const code = integrationErrorCode(value);
  return code ? INTEGRATION_ERROR_TRANSLATION_KEYS[code][surface] : null;
}

export function integrationErrorTranslationKeyFromError<
  Surface extends IntegrationErrorTranslationSurface,
>(error: unknown, surface: Surface): IntegrationErrorTranslationKey<Surface> | null {
  const response = recordValue(error, 'response');
  const data = recordValue(response, 'data');
  const nestedError = recordValue(data, 'error');
  const candidates = [
    recordValue(data, 'code'),
    recordValue(data, 'error_code'),
    recordValue(nestedError, 'code'),
    recordValue(error, 'code'),
    recordValue(data, 'message'),
    recordValue(error, 'message'),
  ];
  for (const candidate of candidates) {
    const key = integrationErrorTranslationKey(candidate, surface);
    if (key) return key;
  }
  return null;
}

export function looksLikeIntegrationErrorCode(value: unknown): boolean {
  return typeof value === 'string' && /\bintegration_[a-z0-9_]+\b/i.test(value);
}
