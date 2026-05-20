import type { AIChatModelParameters } from '@/services/types/aichat';

export function toAIChatParameters(
  params: Record<string, number | string | boolean | string[]> | undefined
): AIChatModelParameters | undefined {
  if (!params) return undefined;

  const next: AIChatModelParameters = {};
  Object.entries(params).forEach(([key, value]) => {
    if (
      typeof value === 'number' ||
      typeof value === 'string' ||
      typeof value === 'boolean' ||
      (Array.isArray(value) && value.every(item => typeof item === 'string'))
    ) {
      next[key] = value;
    }
  });

  return Object.keys(next).length > 0 ? next : undefined;
}
