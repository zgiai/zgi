import type { AgentMemorySlotConfig } from '@/services/types/agent';

const MAX_AGENT_MEMORY_SLOTS = 5;
const DEFAULT_AGENT_MEMORY_MAX_CHARS = 2000;

export type AgentMemoryTemplateMode = 'merge' | 'replace';
export type AgentMemoryTemplateID =
  | 'general_assistant'
  | 'project_collaboration'
  | 'support_operations';

export interface AgentMemoryTemplateSlot {
  key: string;
  name: string;
  description: string;
  max_chars: number;
  enabled: boolean;
}

export interface AgentMemoryTemplate {
  id: AgentMemoryTemplateID;
  name: string;
  description: string;
  slots: AgentMemoryTemplateSlot[];
}

export type AgentMemoryTemplateResult =
  | { ok: true; slots: AgentMemorySlotConfig[] }
  | { ok: false; reason: 'too_many' };

type Translate = (key: string) => string;

const TEMPLATE_DEFINITIONS: Array<{
  id: AgentMemoryTemplateID;
  slots: string[];
}> = [
  {
    id: 'general_assistant',
    slots: ['profile', 'preferences', 'standing_instructions', 'project_context'],
  },
  {
    id: 'project_collaboration',
    slots: ['profile', 'project_context', 'standing_instructions', 'delivery_preferences'],
  },
  {
    id: 'support_operations',
    slots: [
      'customer_profile',
      'communication_preferences',
      'business_context',
      'standing_instructions',
    ],
  },
];

export function createAgentMemoryTemplates(t: Translate): AgentMemoryTemplate[] {
  return TEMPLATE_DEFINITIONS.map(template => ({
    id: template.id,
    name: t(`memory.templates.${template.id}.name`),
    description: t(`memory.templates.${template.id}.description`),
    slots: template.slots.map(key => createTemplateSlot(key, t)),
  }));
}

export function applyAgentMemoryTemplate(
  currentSlots: AgentMemorySlotConfig[],
  template: AgentMemoryTemplate,
  mode: AgentMemoryTemplateMode
): AgentMemoryTemplateResult {
  if (mode === 'replace') {
    return {
      ok: true,
      slots: template.slots.map((slot, index) => templateSlotToConfig(slot, index)),
    };
  }

  const existingKeys = new Set(currentSlots.map(slot => normalizeKey(slot.key)).filter(Boolean));
  const missingSlots = template.slots.filter(slot => !existingKeys.has(normalizeKey(slot.key)));
  if (currentSlots.length + missingSlots.length > MAX_AGENT_MEMORY_SLOTS) {
    return { ok: false, reason: 'too_many' };
  }

  const templateByKey = new Map(template.slots.map(slot => [normalizeKey(slot.key), slot]));
  const merged = currentSlots.map(slot => {
    const templateSlot = templateByKey.get(normalizeKey(slot.key));
    if (!templateSlot) return slot;
    return {
      ...slot,
      name: slot.name?.trim() || templateSlot.name,
      description: templateSlot.description,
      max_chars: templateSlot.max_chars,
      enabled: templateSlot.enabled,
    };
  });

  for (const slot of missingSlots) {
    merged.push(templateSlotToConfig(slot, merged.length));
  }
  return { ok: true, slots: merged.map((slot, index) => ({ ...slot, sort_order: index })) };
}

export function addAgentMemoryTemplateSlot(
  currentSlots: AgentMemorySlotConfig[],
  slot: AgentMemoryTemplateSlot
): AgentMemoryTemplateResult {
  const key = normalizeKey(slot.key);
  if (!key || currentSlots.some(item => normalizeKey(item.key) === key)) {
    return { ok: true, slots: currentSlots };
  }
  if (currentSlots.length >= MAX_AGENT_MEMORY_SLOTS) {
    return { ok: false, reason: 'too_many' };
  }
  return {
    ok: true,
    slots: [...currentSlots, templateSlotToConfig(slot, currentSlots.length)],
  };
}

export function validateAgentMemoryTemplates(templates: AgentMemoryTemplate[]): string[] {
  const errors: string[] = [];
  for (const template of templates) {
    if (template.slots.length > MAX_AGENT_MEMORY_SLOTS) {
      errors.push(`${template.id}: too many slots`);
    }
    const seen = new Set<string>();
    for (const slot of template.slots) {
      const key = normalizeKey(slot.key);
      if (!/^[a-z][a-z0-9_]*$/.test(key)) {
        errors.push(`${template.id}: invalid key ${slot.key}`);
      }
      if (seen.has(key)) {
        errors.push(`${template.id}: duplicate key ${slot.key}`);
      }
      seen.add(key);
    }
  }
  return errors;
}

function createTemplateSlot(key: string, t: Translate): AgentMemoryTemplateSlot {
  return {
    key,
    name: t(`memory.templateSlots.${key}.name`),
    description: t(`memory.templateSlots.${key}.description`),
    max_chars: DEFAULT_AGENT_MEMORY_MAX_CHARS,
    enabled: true,
  };
}

function templateSlotToConfig(
  slot: AgentMemoryTemplateSlot,
  sortOrder: number
): AgentMemorySlotConfig {
  return {
    key: normalizeKey(slot.key),
    name: slot.name.slice(0, 80),
    description: slot.description.slice(0, 200),
    max_chars: slot.max_chars,
    enabled: slot.enabled,
    sort_order: sortOrder,
  };
}

function normalizeKey(key: string): string {
  return key.trim().toLowerCase();
}
