import type { Agent, AgentDetail } from '@/services/types/agent';
import type { AgentRuntimeConfig } from '@/services/types/agent';
import type { AIChatSkillMetadata } from '@/services/types/aichat';

export type AgentRuntimeSaveState = 'idle' | 'dirty' | 'saving' | 'saved' | 'error' | 'previewing';
export type AgentConfigSection =
  | 'experience'
  | 'model'
  | 'skills'
  | 'knowledge'
  | 'databases'
  | 'workflows'
  | 'files'
  | 'memory';

export type AgentRuntimeAgent = Agent | AgentDetail | undefined;

export interface AgentPublishedVersionListItem {
  id: string;
  agent_id: string;
  version: string;
  version_uuid: string;
  description: string;
  config_snapshot: AgentRuntimeConfig;
  is_current: boolean;
  created_at: number;
}

export interface AgentRuntimeSelectedSkillItem {
  skillId: string;
  label: string;
  description: string;
  metadata?: AIChatSkillMetadata;
}
