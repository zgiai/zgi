'use client';

import { AlertCircle, Database, Trash2 } from 'lucide-react';
import { Button } from '@/components/ui/button';
import { useT } from '@/i18n';
import type { Dataset } from '@/services/types/dataset';
import { AgentRuntimeResourceCard, AgentRuntimeResourceSection } from '../resource-section';
import type { AgentConfigSection } from '../types';

type AgentKnowledgeDataset = Dataset & { load_error?: boolean };

interface AgentRuntimeKnowledgeSectionProps {
  open: boolean;
  isDatasetsLoading: boolean;
  canBindKnowledge: boolean;
  selectedKnowledgeDatasets: AgentKnowledgeDataset[];
  selectedKnowledgeDatasetIds: string[];
  readOnly?: boolean;
  onToggleSection: (section: AgentConfigSection) => void;
  onOpenKnowledgeDialog: () => void;
  onToggleKnowledgeDataset: (datasetId: string, checked: boolean) => void;
}

export function AgentRuntimeKnowledgeSection({
  open,
  isDatasetsLoading,
  canBindKnowledge,
  selectedKnowledgeDatasets,
  selectedKnowledgeDatasetIds,
  readOnly = false,
  onToggleSection,
  onOpenKnowledgeDialog,
  onToggleKnowledgeDataset,
}: AgentRuntimeKnowledgeSectionProps) {
  const t = useT('agents.agentRuntime');

  return (
    <AgentRuntimeResourceSection
      title={t('sections.knowledge')}
      section="knowledge"
      open={open}
      count={selectedKnowledgeDatasetIds.length}
      addLabel={t('knowledge.add')}
      addTooltip={
        canBindKnowledge ? undefined : t('knowledge.bindingPermissionRequired')
      }
      helpText={t('knowledge.helpText')}
      emptyText={t('knowledge.emptySelected')}
      isLoading={isDatasetsLoading}
      onToggleSection={onToggleSection}
      onAdd={onOpenKnowledgeDialog}
      readOnly={readOnly || !canBindKnowledge}
    >
      <div className="space-y-2">
        {selectedKnowledgeDatasets.map(dataset => (
          <AgentRuntimeResourceCard
            key={dataset.id}
            icon={
              dataset.load_error ? (
                <AlertCircle className="size-4" />
              ) : (
                <Database className="size-4" />
              )
            }
            title={dataset.name}
            description={
              dataset.load_error
                ? t('knowledge.loadFailedDescription')
                : dataset.description || t('knowledge.noDescription')
            }
            error={dataset.load_error}
            action={
              <Button
                type="button"
                variant="ghost"
                size="sm"
                isIcon
                className="size-8 shrink-0 text-muted-foreground hover:text-destructive"
                aria-label={t('knowledge.remove', { name: dataset.name })}
                onClick={() => onToggleKnowledgeDataset(dataset.id, false)}
                disabled={readOnly || !canBindKnowledge}
              >
                <Trash2 className="size-4" />
              </Button>
            }
          />
        ))}
      </div>
    </AgentRuntimeResourceSection>
  );
}
