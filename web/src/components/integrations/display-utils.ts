const CANONICAL_UUID_PATTERN = /\b[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}\b/i;
const COMPACT_UUID_PATTERN = /\b[0-9a-f]{32}\b/i;

export function containsOpaqueUUID(value: string | null | undefined): boolean {
  const normalized = value?.trim() ?? '';
  return Boolean(
    normalized && (CANONICAL_UUID_PATTERN.test(normalized) || COMPACT_UUID_PATTERN.test(normalized))
  );
}

export function safeIntegrationDisplayText(
  value: string | null | undefined,
  fallback: string
): string {
  const normalized = value?.trim() ?? '';
  return normalized && !containsOpaqueUUID(normalized) ? normalized : fallback;
}

export function safeOptionalIntegrationDisplayText(
  value: string | null | undefined
): string | null {
  const normalized = value?.trim() ?? '';
  return normalized && !containsOpaqueUUID(normalized) ? normalized : null;
}
