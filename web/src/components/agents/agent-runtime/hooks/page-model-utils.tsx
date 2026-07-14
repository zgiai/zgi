'use client';

import type { AgentRuntimeSaveState } from '../types';
import type { AgentRuntimeDraftPersistenceSnapshot } from '../use-agent-runtime-draft-persistence';
import type { AgentBindingHealth, UpdateAgentRuntimeConfigRequest } from '@/services/types/agent';
import type { useT } from '@/i18n';

export interface VersionPreviewBackup {
  payload: UpdateAgentRuntimeConfigRequest;
  persistence: AgentRuntimeDraftPersistenceSnapshot;
  bindingHealth?: AgentBindingHealth;
}

export function getAgentRuntimeSaveText(
  t: ReturnType<typeof useT<'agents.agentRuntime'>>,
  saveState: AgentRuntimeSaveState,
  lastSavedAt: number | null
) {
  if (saveState === 'saving') return t('saveState.saving');
  if (saveState === 'previewing') return t('saveState.previewing');
  if (saveState === 'dirty') return t('saveState.dirty');
  if (saveState === 'error') return t('saveState.error');
  if (lastSavedAt) {
    return t('saveState.savedAt', {
      time: new Date(lastSavedAt * 1000).toLocaleTimeString(undefined, { hour12: false }),
    });
  }
  return t('saveState.saved');
}
