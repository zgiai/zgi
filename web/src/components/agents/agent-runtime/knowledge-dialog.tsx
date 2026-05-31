'use client';

import { Check, Database, Search } from 'lucide-react';
import { Button } from '@/components/ui/button';
import { Checkbox } from '@/components/ui/checkbox';
import {
  Dialog,
  DialogBody,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from '@/components/ui/dialog';
import { Input } from '@/components/ui/input';
import { useT } from '@/i18n';
import { cn } from '@/lib/utils';
import type { Dataset } from '@/services/types/dataset';

interface AgentRuntimeKnowledgeDialogProps {
  open: boolean;
  datasets: Dataset[];
  selectedDatasetIds: string[];
  search: string;
  showSelectedOnly: boolean;
  isLoading: boolean;
  onOpenChange: (open: boolean) => void;
  onChangeSearch: (value: string) => void;
  onChangeShowSelectedOnly: (value: boolean) => void;
  onToggleDataset: (datasetId: string, checked: boolean) => void;
}

export function AgentRuntimeKnowledgeDialog({
  open,
  datasets,
  selectedDatasetIds,
  search,
  showSelectedOnly,
  isLoading,
  onOpenChange,
  onChangeSearch,
  onChangeShowSelectedOnly,
  onToggleDataset,
}: AgentRuntimeKnowledgeDialogProps) {
  const t = useT('agents.agentRuntime');

  const renderContent = () => {
    if (isLoading) {
      return (
        <div className="grid grid-cols-1 gap-3 lg:grid-cols-2">
          {Array.from({ length: 4 }).map((_, index) => (
            <div key={index} className="h-32 rounded-lg border bg-muted/30" />
          ))}
        </div>
      );
    }

    if (datasets.length === 0) {
      return (
        <div className="rounded-md border border-dashed p-4 text-sm text-muted-foreground">
          {t('knowledge.noMatch')}
        </div>
      );
    }

    return (
      <div className="grid grid-cols-1 gap-3 lg:grid-cols-2">
        {datasets.map(dataset => {
          const checked = selectedDatasetIds.includes(dataset.id);
          return (
            <button
              key={dataset.id}
              type="button"
              className={cn(
                'flex min-h-32 cursor-pointer flex-col rounded-lg border bg-background p-4 text-left transition-colors hover:border-primary/50 hover:bg-muted/30',
                checked ? 'border-primary bg-primary/5' : ''
              )}
              onClick={() => onToggleDataset(dataset.id, !checked)}
            >
              <span className="flex items-start gap-3">
                <span className="flex size-10 shrink-0 items-center justify-center rounded-lg border bg-muted text-primary">
                  <Database className="size-5" />
                </span>
                <span className="min-w-0 flex-1">
                  <span className="block truncate text-sm font-semibold">{dataset.name}</span>
                  <span className="mt-1 inline-flex rounded border bg-muted/40 px-1.5 py-0.5 text-[11px] text-muted-foreground">
                    {dataset.provider || dataset.indexing_technique}
                  </span>
                </span>
                <span
                  className={cn(
                    'flex size-5 shrink-0 items-center justify-center rounded-full border',
                    checked ? 'border-primary bg-primary text-primary-foreground' : 'bg-background'
                  )}
                >
                  {checked ? <Check className="size-3.5" /> : null}
                </span>
              </span>
              <span className="mt-3 line-clamp-2 text-xs leading-5 text-muted-foreground">
                {dataset.description || t('knowledge.noDescription')}
              </span>
            </button>
          );
        })}
      </div>
    );
  };

  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent size="lg">
        <DialogHeader>
          <DialogTitle>{t('knowledge.dialogTitle')}</DialogTitle>
          <DialogDescription>{t('knowledge.dialogDescription')}</DialogDescription>
        </DialogHeader>
        <DialogBody className="max-h-[560px]">
          <div className="space-y-3">
            <div className="flex items-center gap-2">
              <div className="relative min-w-0 flex-1">
                <Search className="pointer-events-none absolute left-2.5 top-1/2 size-4 -translate-y-1/2 text-muted-foreground" />
                <Input
                  value={search}
                  onChange={event => onChangeSearch(event.target.value)}
                  placeholder={t('knowledge.searchPlaceholder')}
                  className="pl-8"
                />
              </div>
              <label className="flex shrink-0 items-center gap-2 rounded-md border px-3 py-2 text-sm">
                <Checkbox
                  checked={showSelectedOnly}
                  onCheckedChange={value => onChangeShowSelectedOnly(value === true)}
                />
                {t('knowledge.selectedOnly')}
              </label>
            </div>
            {renderContent()}
          </div>
        </DialogBody>
        <DialogFooter>
          <Button onClick={() => onOpenChange(false)}>{t('knowledge.done')}</Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  );
}
