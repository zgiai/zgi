/**
 * Pure helpers for the skill availability notice. They stay free of runtime
 * imports so scripts/test-skill-availability.mjs can load them directly.
 */

/** Whether a failed request was refused for lack of permission. */
export function isPermissionError(error: unknown): boolean {
  if (!error || typeof error !== 'object') return false;
  const record = error as Record<string, unknown>;
  const response = record.response as Record<string, unknown> | undefined;
  const status = Number(record.status ?? record.statusCode ?? response?.status);
  if (status === 401 || status === 403) return true;
  const code = String(record.code ?? '');
  return /forbidden|permission|access_denied|unauthori[sz]ed/i.test(code);
}
