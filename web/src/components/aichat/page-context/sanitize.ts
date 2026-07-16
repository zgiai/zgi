const SENSITIVE_ASSIGNMENT_RE =
  /\b(api[_-]?key|access[_-]?token|refresh[_-]?token|auth[_-]?token|bearer[_-]?token|client[_-]?secret|secret|password|passwd|pwd)\b\s*[:=]\s*(?:"[^"]*"|'[^']*'|[^'",;\s]+)/gi;
const AUTHORIZATION_HEADER_RE = /\bAuthorization\s*[:=]\s*(?:Bearer\s+)?[^\n\r,;]+/gi;
const BEARER_TOKEN_RE = /\bBearer\s+[A-Za-z0-9._~+/=-]{8,}/gi;
const URL_SECRET_QUERY_RE =
  /([?&](?:api[_-]?key|access[_-]?token|refresh[_-]?token|auth[_-]?token|token|secret|password|passwd|pwd|code)=)[^&#\s]+/gi;
const COMMON_SECRET_TOKEN_RE =
  /\b(sk-[A-Za-z0-9_-]{10,}|AKIA[0-9A-Z]{12,}|eyJ[A-Za-z0-9_-]{20,}\.[A-Za-z0-9_-]{10,}\.[A-Za-z0-9_-]{10,})\b/g;

export function sanitizeAIChatContextText(value: string): string {
  return value
    .replace(AUTHORIZATION_HEADER_RE, 'Authorization=[redacted]')
    .replace(BEARER_TOKEN_RE, 'Bearer [redacted]')
    .replace(SENSITIVE_ASSIGNMENT_RE, '$1=[redacted]')
    .replace(URL_SECRET_QUERY_RE, '$1[redacted]')
    .replace(COMMON_SECRET_TOKEN_RE, '[redacted]');
}
