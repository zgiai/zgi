import type {
  AgentMemorySlotConfig,
  UpdateAgentRuntimeConfigRequest,
} from '@/services/types/agent';
import { ICON_BG } from '@/lib/config';
import type { DbTable } from '@/services/types/db';

export type AgentMemorySlotValidationError =
  | 'required'
  | 'pattern'
  | 'duplicate'
  | 'too_many'
  | null;

const AGENT_MEMORY_SLOT_KEY_PATTERN = /^[a-z][a-z0-9_]*$/;

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
  const editablePayload = { ...payload };
  delete editablePayload.agent_memory_config_revision;
  delete editablePayload.execution_runtime;

  const editableMemorySlots = (payload.agent_memory_slots ?? []).map(slot => ({
    key: slot.key,
    name: slot.name ?? '',
    description: slot.description,
    max_chars: slot.max_chars,
    enabled: slot.enabled,
    sort_order: slot.sort_order,
    read_only: slot.read_only ?? false,
    auto_write: slot.auto_write ?? true,
  }));

  return JSON.stringify({
    ...editablePayload,
    execution_runtime: payload.execution_runtime
      ? {
          enabled: payload.execution_runtime.enabled,
        }
      : null,
    enabled_skill_ids: [...payload.enabled_skill_ids].sort(),
    knowledge_dataset_ids: [...(payload.knowledge_dataset_ids ?? [])].sort(),
    workflow_bindings: [...(payload.workflow_bindings ?? [])].sort((left, right) =>
      left.binding_id.localeCompare(right.binding_id)
    ),
    connector_bindings: [...(payload.connector_bindings ?? [])].sort((left, right) =>
      left.connector_id.localeCompare(right.connector_id)
    ),
    connector_reference_commands: payload.connector_reference_commands ?? [],
    agent_memory_slots: editableMemorySlots.sort((left, right) =>
      left.key.localeCompare(right.key)
    ),
  });
}

// Agent memory block limits (ADR 0013); the backend enforces the same.
export const AGENT_MEMORY_LIMITS = {
  maxBlocks: 10,
  maxDescriptionChars: 1000,
  minChars: 100,
  maxChars: 5000,
  defaultChars: 2000,
  maxInstructionsChars: 2000,
} as const;

export function clampAgentMemoryMaxChars(value: number | undefined): number {
  if (!value || !Number.isFinite(value)) return AGENT_MEMORY_LIMITS.defaultChars;
  return Math.min(
    AGENT_MEMORY_LIMITS.maxChars,
    Math.max(AGENT_MEMORY_LIMITS.minChars, Math.round(value))
  );
}

export function validateAgentMemorySlots(
  slots: AgentMemorySlotConfig[]
): AgentMemorySlotValidationError[] {
  const normalizedKeys = slots.map(slot => slot.key.trim().toLowerCase());
  const keyCounts = normalizedKeys.reduce<Record<string, number>>((acc, key) => {
    if (key) acc[key] = (acc[key] ?? 0) + 1;
    return acc;
  }, {});

  return normalizedKeys.map(key => {
    if (slots.length > AGENT_MEMORY_LIMITS.maxBlocks) return 'too_many';
    if (!key) return 'required';
    if (!AGENT_MEMORY_SLOT_KEY_PATTERN.test(key)) return 'pattern';
    if ((keyCounts[key] ?? 0) > 1) return 'duplicate';
    return null;
  });
}

export function pickAgentInitials(name?: string): string {
  const trimmed = name?.trim();
  if (!trimmed) return 'A';
  return trimmed.slice(0, 2).toUpperCase();
}

export function getAgentTextIconDisplay(
  iconType?: string,
  icon?: string,
  name?: string
): { text: string; background: string } {
  const fallback = {
    text: pickAgentInitials(name),
    background: ICON_BG,
  };

  if (iconType !== 'text' || !icon?.trim()) {
    return fallback;
  }

  const trimmed = icon.trim();
  try {
    const parsed = JSON.parse(trimmed) as { icon?: unknown; icon_background?: unknown };
    return {
      text:
        typeof parsed.icon === 'string' && parsed.icon.trim() ? parsed.icon.trim() : fallback.text,
      background:
        typeof parsed.icon_background === 'string' && parsed.icon_background.trim()
          ? parsed.icon_background.trim()
          : fallback.background,
    };
  } catch {
    return {
      text: trimmed,
      background: fallback.background,
    };
  }
}

export function tablesForDataSource(tables: DbTable[], dataSourceId: string): DbTable[] {
  const normalizedDataSourceId = dataSourceId.trim();
  if (!normalizedDataSourceId) return [];
  return tables.filter(table => {
    const tableDataSourceId = table.data_source_id?.trim();
    return !tableDataSourceId || tableDataSourceId === normalizedDataSourceId;
  });
}
