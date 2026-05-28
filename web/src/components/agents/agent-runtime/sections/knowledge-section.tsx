'use client';

import { Database, Plus, Trash2 } from 'lucide-react';
import { Badge } from '@/components/ui/badge';
import { Button } from '@/components/ui/button';
import { Skeleton } from '@/components/ui/skeleton';
import { useT } from '@/i18n';
import type { Dataset } from '@/services/types/dataset';
import { RuntimeSection } from '../runtime-section';
import type { AgentConfigSection } from '../types';

interface AgentRuntimeKnowledgeSectionProps {
  open: boolean;
  isDatasetsLoading: boolean;
  selectedKnowledgeDatasets: Dataset[];
  selectedKnowledgeDatasetIds: string[];
  onToggleSection: (section: AgentConfigSection) => void;
  onOpenKnowledgeDialog: () => void;
  onToggleKnowledgeDataset: (datasetId: string, checked: boolean) => void;
}

export function AgentRuntimeKnowledgeSection({
  open,
  isDatasetsLoading,
  selectedKnowledgeDatasets,
  selectedKnowledgeDatasetIds,
  onToggleSection,
  onOpenKnowledgeDialog,
  onToggleKnowledgeDataset,
}: AgentRuntimeKnowledgeSectionProps) {
  const t = useT('agents.agentRuntime');

  return (
    <RuntimeSection
      title={t('sections.knowledge')}
      section="knowledge"
      open={open}
      onToggle={onToggleSection}
      action={
        <div className="flex items-center gap-2">
          <Badge variant="subtle">{selectedKnowledgeDatasetIds.length}</Badge>
          <Button
            type="button"
            variant="outline"
            size="sm"
            isIcon
            className="size-8"
            aria-label={t('knowledge.add')}
            onClick={event => {
              event.stopPropagation();
              onOpenKnowledgeDialog();
            }}
          >
            <Plus className="size-4" />
          </Button>
        </div>
      }
    >
      {isDatasetsLoading ? (
        <div className="space-y-2">
          <Skeleton className="h-20 w-full" />
          <Skeleton className="h-20 w-full" />
        </div>
      ) : selectedKnowledgeDatasets.length === 0 ? (
        <div className="rounded-md border border-dashed p-4 text-sm text-muted-foreground">
          {t('knowledge.emptySelected')}
        </div>
      ) : (
        <div className="space-y-2">
          {selectedKnowledgeDatasets.map(dataset => (
            <div key={dataset.id} className="flex items-start gap-3 rounded-md border bg-background p-3">
              <div className="mt-0.5 flex size-8 shrink-0 items-center justify-center rounded-md border bg-muted text-primary">
                <Database className="size-4" />
              </div>
              <div className="min-w-0 flex-1">
                <div className="truncate text-sm font-medium">{dataset.name}</div>
                <div className="mt-1 line-clamp-2 text-xs leading-5 text-muted-foreground">
                  {dataset.description || t('knowledge.noDescription')}
                </div>
                <div className="mt-2 text-[11px] text-muted-foreground/70">
                  {t('knowledge.idLabel', { id: dataset.id })}
                </div>
              </div>
              <Button
                type="button"
                variant="ghost"
                size="sm"
                isIcon
                className="size-8 shrink-0 text-muted-foreground hover:text-destructive"
                aria-label={t('knowledge.remove', { name: dataset.name })}
                onClick={() => onToggleKnowledgeDataset(dataset.id, false)}
              >
                <Trash2 className="size-4" />
              </Button>
            </div>
          ))}
        </div>
      )}
    </RuntimeSection>
  );
}
