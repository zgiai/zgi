'use client';

import type { AgentRuntimeSaveState } from '../types';
import { pickAgentInitials } from '../utils';
import type { AgentRuntimeDraftPersistenceSnapshot } from '../use-agent-runtime-draft-persistence';
import type { UpdateAgentRuntimeConfigRequest } from '@/services/types/agent';
import type { useT } from '@/i18n';

export interface VersionPreviewBackup {
  payload: UpdateAgentRuntimeConfigRequest;
  persistence: AgentRuntimeDraftPersistenceSnapshot;
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

interface AgentHomeBrandProps {
  iconType?: string;
  iconUrl?: string;
  name?: string;
}

export function AgentHomeBrand({ iconType, iconUrl, name }: AgentHomeBrandProps) {
  return (
    <div className="flex size-16 items-center justify-center rounded-2xl border border-primary/30 bg-primary/10 text-xl font-semibold text-primary shadow-sm">
      {iconType === 'image' && iconUrl ? (
        <img src={iconUrl} alt="" className="size-full rounded-2xl object-cover" />
      ) : (
        pickAgentInitials(name)
      )}
    </div>
  );
}
