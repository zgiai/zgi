import type { AIChatSkillMetadata } from '@/services/types/aichat';
import type { AgentSkillBindingCandidate } from '@/services/types/agent';
import { normalizeAIChatSkillId } from '../../chat/variants/aichat/skill-identity';

export function normalizeAgentSkillCandidates(
  candidates: readonly AgentSkillBindingCandidate[] | null | undefined
): AgentSkillBindingCandidate[] {
  const seen = new Set<string>();
  return (candidates ?? []).flatMap(candidate => {
    if (!candidate) return [];

    const skillId = normalizeAIChatSkillId(candidate.skill_id);
    if (!skillId || seen.has(skillId)) return [];
    seen.add(skillId);

    const name =
      typeof candidate.name === 'string' && candidate.name.trim() ? candidate.name.trim() : skillId;
    return [{ ...candidate, skill_id: skillId, name }];
  });
}

export function agentSkillCandidateToMetadata(
  candidate: AgentSkillBindingCandidate
): AIChatSkillMetadata {
  return {
    skill_id: candidate.skill_id,
    source: candidate.source === 'custom' ? 'custom' : 'system',
    name: candidate.name,
    description: candidate.description ?? '',
    when_to_use: candidate.when_to_use ?? '',
    runtime_type: (candidate.runtime_type || 'prompt') as AIChatSkillMetadata['runtime_type'],
    enabled: true,
    display: candidate.display,
    has_tools: candidate.has_tools,
    has_references: candidate.has_references,
    has_scripts: candidate.has_scripts,
    scripts_supported: candidate.scripts_supported,
    max_calls_per_turn: 0,
    timeout_seconds: 0,
    required_config: candidate.required_config,
  };
}
