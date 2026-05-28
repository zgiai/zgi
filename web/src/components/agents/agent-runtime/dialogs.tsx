'use client';

import { PromptOptimizerDialog } from '@/components/prompts/prompt-optimizer-dialog';
import { AgentRuntimeMemoryValuesDialog } from './memory-values-dialog';
import { AgentRuntimeSkillDialog } from './skill-dialog';
import type { AgentRuntimePageModel } from './hooks/use-agent-runtime-page-model';

interface AgentRuntimeDialogsProps {
  model: AgentRuntimePageModel;
}

export function AgentRuntimeDialogs({ model }: AgentRuntimeDialogsProps) {
  return (
    <>
      <PromptOptimizerDialog {...model.dialogs.promptOptimizer} />
      <AgentRuntimeSkillDialog {...model.dialogs.skill} />
      <AgentRuntimeMemoryValuesDialog {...model.dialogs.memoryValues} />
    </>
  );
}
