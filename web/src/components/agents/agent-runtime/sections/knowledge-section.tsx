'use client';

import { Badge } from '@/components/ui/badge';
import { Checkbox } from '@/components/ui/checkbox';
import { Skeleton } from '@/components/ui/skeleton';
import { useT } from '@/i18n';
import type { Dataset } from '@/services/types/dataset';
import { RuntimeSection } from '../runtime-section';
import type { AgentConfigSection } from '../types';

interface AgentRuntimeKnowledgeSectionProps {
  open: boolean;
  isDatasetsLoading: boolean;
  availableDatasets: Dataset[];
  selectedKnowledgeDatasetIds: string[];
  onToggleSection: (section: AgentConfigSection) => void;
  onToggleKnowledgeDataset: (datasetId: string, checked: boolean) => void;
}

export function AgentRuntimeKnowledgeSection({
  open,
  isDatasetsLoading,
  availableDatasets,
  selectedKnowledgeDatasetIds,
  onToggleSection,
  onToggleKnowledgeDataset,
}: AgentRuntimeKnowledgeSectionProps) {
  const t = useT('agents.agentRuntime');

  return (
    <RuntimeSection
      title={t('sections.knowledge')}
      section="knowledge"
      open={open}
      onToggle={onToggleSection}
      action={<Badge variant="subtle">{selectedKnowledgeDatasetIds.length}</Badge>}
    >
      {isDatasetsLoading ? (
        <div className="space-y-2">
          <Skeleton className="h-12 w-full" />
          <Skeleton className="h-12 w-full" />
        </div>
      ) : availableDatasets.length === 0 ? (
        <div className="rounded-md border border-dashed p-4 text-sm text-muted-foreground">
          {t('knowledge.empty')}
        </div>
      ) : (
        <div className="space-y-2">
          {availableDatasets.map(dataset => {
            const checked = selectedKnowledgeDatasetIds.includes(dataset.id);
            return (
              <label
                key={dataset.id}
                className="flex cursor-pointer items-start gap-3 rounded-md border bg-background p-3"
              >
                <Checkbox
                  checked={checked}
                  onCheckedChange={value => onToggleKnowledgeDataset(dataset.id, value === true)}
                />
                <span className="min-w-0 flex-1">
                  <span className="block truncate text-sm font-medium">{dataset.name}</span>
                  <span className="mt-1 line-clamp-2 text-xs leading-5 text-muted-foreground">
                    {dataset.description || t('knowledge.noDescription')}
                  </span>
                </span>
              </label>
            );
          })}
        </div>
      )}
    </RuntimeSection>
  );
}
