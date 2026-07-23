'use client';

import { PromptOptimizerDialog } from '@/components/prompts/prompt-optimizer-dialog';
import { AgentRuntimeKnowledgeDialog } from './knowledge-dialog';
import { AgentRuntimeMemoryValuesDialog } from './memory-values-dialog';
import { AgentRuntimeSkillDialog } from './skill-dialog';
import { AgentRuntimeWorkflowDialog } from './workflow-dialog';
import { AgentSuspendedBindingsDialog } from './binding-health';
import { AgentPublishVersionDialog } from './publish-version-dialog';
import type { AgentRuntimePageModel } from './hooks/use-agent-runtime-page-model';
import { ConfirmDialog } from '@/components/ui/confirm-dialog';

interface AgentRuntimeDialogsProps {
  model: AgentRuntimePageModel;
}

export function AgentRuntimeDialogs({ model }: AgentRuntimeDialogsProps) {
  return (
    <>
      <PromptOptimizerDialog {...model.dialogs.promptOptimizer} />
      <AgentRuntimeSkillDialog {...model.dialogs.skill} />
      <AgentRuntimeKnowledgeDialog {...model.dialogs.knowledge} />
      <AgentRuntimeWorkflowDialog {...model.dialogs.workflow} />
      <AgentRuntimeMemoryValuesDialog {...model.dialogs.memoryValues} />
      <AgentPublishVersionDialog {...model.dialogs.publishVersion} />
      <AgentSuspendedBindingsDialog {...model.dialogs.suspendedBindings} />
      <ConfirmDialog
        variant="danger"
        {...model.dialogs.cleanupSkills}
        confirmText={model.t('bindingHealth.removeUnavailableSkills')}
        cancelText={model.t('bindingHealth.cancel')}
      />
    </>
  );
}
