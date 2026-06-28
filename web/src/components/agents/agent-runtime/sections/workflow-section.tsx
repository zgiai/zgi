'use client';

import { AlertCircle, Trash2 } from 'lucide-react';
import { Button } from '@/components/ui/button';
import { useT } from '@/i18n';
import type { AgentWorkflowBinding, AgentWorkflowBindingCandidate } from '@/services/types/agent';
import { AgentRuntimeResourceCard, AgentRuntimeResourceSection } from '../resource-section';
import type { AgentConfigSection } from '../types';
import { AgentWorkflowTypeBadge, AgentWorkflowTypeIcon } from '../workflow-type-display';

interface AgentRuntimeWorkflowSectionProps {
  open: boolean;
  bindings: AgentWorkflowBinding[];
  candidatesByBindingID: Map<string, AgentWorkflowBindingCandidate>;
  isLoading: boolean;
  readOnly?: boolean;
  onToggleSection: (section: AgentConfigSection) => void;
  onOpenWorkflowDialog: () => void;
  onChangeBindings: (value: AgentWorkflowBinding[]) => void;
}

export function AgentRuntimeWorkflowSection({
  open,
  bindings,
  candidatesByBindingID,
  isLoading,
  readOnly = false,
  onToggleSection,
  onOpenWorkflowDialog,
  onChangeBindings,
}: AgentRuntimeWorkflowSectionProps) {
  const t = useT('agents.agentRuntime');

  const removeWorkflow = (bindingId: string) => {
    if (readOnly) return;
    onChangeBindings(bindings.filter(binding => binding.binding_id !== bindingId));
  };

  return (
    <AgentRuntimeResourceSection
      title={t('sections.workflows')}
      section="workflows"
      open={open}
      count={bindings.length}
      addLabel={t('workflow.add')}
      helpText={t('workflow.helpText')}
      emptyText={t('workflow.emptySelected')}
      isLoading={isLoading}
      onToggleSection={onToggleSection}
      onAdd={onOpenWorkflowDialog}
      readOnly={readOnly}
    >
      <div className="space-y-2">
        {bindings.map(binding => {
          const candidate = candidatesByBindingID.get(binding.binding_id);
          const unavailable = !candidate && !isLoading;
          const label = candidate?.label || binding.label || t('workflow.unavailableWorkflow');
          return (
            <AgentRuntimeResourceCard
              key={binding.binding_id}
              icon={
                unavailable ? (
                  <AlertCircle className="size-4" />
                ) : (
                  <AgentWorkflowTypeIcon
                    agentType={candidate?.agent_type || binding.agent_type}
                    className="size-4"
                  />
                )
              }
              title={label}
              description={
                unavailable
                  ? t('workflow.unavailableDescription')
                  : candidate?.description || binding.description || t('workflow.noDescription')
              }
              error={unavailable}
              action={
                <Button
                  type="button"
                  variant="ghost"
                  size="sm"
                  isIcon
                  className="size-8 shrink-0 text-muted-foreground hover:text-destructive"
                  aria-label={t('workflow.remove', { name: label })}
                  disabled={readOnly}
                  onClick={() => removeWorkflow(binding.binding_id)}
                >
                  <Trash2 className="size-4" />
                </Button>
              }
            >
              {!unavailable ? (
                <AgentWorkflowTypeBadge
                  agentType={candidate?.agent_type || binding.agent_type}
                  className="mt-2"
                />
              ) : null}
            </AgentRuntimeResourceCard>
          );
        })}
      </div>
    </AgentRuntimeResourceSection>
  );
}
