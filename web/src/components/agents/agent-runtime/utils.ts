import type { UpdateAgentRuntimeConfigRequest } from '@/services/types/agent';

export function toModelParams(
  params: Record<string, unknown> | undefined
): Record<string, number | string | boolean> {
  const next: Record<string, number | string | boolean> = {};
  for (const [key, value] of Object.entries(params ?? {})) {
    if (typeof value === 'number' || typeof value === 'string' || typeof value === 'boolean') {
      next[key] = value;
    }
  }
  return next;
}

export function buildAgentRuntimeSignature(payload: UpdateAgentRuntimeConfigRequest): string {
  return JSON.stringify({
    ...payload,
    enabled_skill_ids: [...payload.enabled_skill_ids].sort(),
    knowledge_dataset_ids: [...(payload.knowledge_dataset_ids ?? [])].sort(),
  });
}

export function pickAgentInitials(name?: string): string {
  const trimmed = name?.trim();
  if (!trimmed) return 'A';
  return trimmed.slice(0, 2).toUpperCase();
}
