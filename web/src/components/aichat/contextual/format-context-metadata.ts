import { sanitizeAIChatContextText } from '../page-context/sanitize';

const MAX_CONTEXT_METADATA_KEYS = 24;
const MAX_CONTEXT_METADATA_LENGTH = 1600;
const MAX_CONTEXT_METADATA_VALUE_LENGTH = 120;

type ContextMetadataValue = string | number | boolean | null | undefined;

function compactMetadataValue(key: string, value: ContextMetadataValue): string {
  const prefix = `${key}=`;
  const sanitized = sanitizeAIChatContextText(`${prefix}${value ?? ''}`);
  const text = (sanitized.startsWith(prefix) ? sanitized.slice(prefix.length) : sanitized)
    .replace(/\s+/g, ' ')
    .trim();
  if (text.length <= MAX_CONTEXT_METADATA_VALUE_LENGTH) return text;
  return `${text.slice(0, MAX_CONTEXT_METADATA_VALUE_LENGTH).trim()}...`;
}

export function formatContextMetadata(
  metadata: Record<string, ContextMetadataValue> | undefined
): string {
  const parts: string[] = [];
  let length = 0;

  for (const [key, value] of Object.entries(metadata ?? {})) {
    if (parts.length >= MAX_CONTEXT_METADATA_KEYS) break;
    const compactValue = compactMetadataValue(key, value);
    if (!compactValue) continue;

    const part = `${key}=${compactValue}`;
    const nextLength = length + (parts.length > 0 ? 2 : 0) + part.length;
    if (nextLength > MAX_CONTEXT_METADATA_LENGTH) break;

    parts.push(part);
    length = nextLength;
  }

  return parts.join(', ');
}
