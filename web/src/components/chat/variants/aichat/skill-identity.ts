import type { AIChatSkillMetadata } from '@/services/types/aichat';

export function normalizeAIChatSkillId(value: unknown): string {
  return typeof value === 'string' ? value.trim().toLowerCase() : '';
}

export function normalizeAIChatSkillIds(value: unknown): string[] {
  if (!Array.isArray(value)) return [];
  return Array.from(new Set(value.map(normalizeAIChatSkillId).filter(Boolean)));
}

export function normalizeAIChatSkills(value: unknown): AIChatSkillMetadata[] {
  if (!Array.isArray(value)) return [];

  const seen = new Set<string>();
  return value.flatMap(item => {
    if (!item || typeof item !== 'object' || Array.isArray(item)) return [];

    const skill = item as AIChatSkillMetadata;
    const skillId = normalizeAIChatSkillId(skill.skill_id);
    if (!skillId || seen.has(skillId)) return [];
    seen.add(skillId);

    const name = typeof skill.name === 'string' && skill.name.trim() ? skill.name.trim() : skillId;
    return [{ ...skill, skill_id: skillId, name }];
  });
}
