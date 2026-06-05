'use client';

import { useEffect, useMemo, useState } from 'react';
import { Check, Search, Workflow } from 'lucide-react';
import { Badge } from '@/components/ui/badge';
import { Button } from '@/components/ui/button';
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
import { Skeleton } from '@/components/ui/skeleton';
import { useT } from '@/i18n';
import { cn } from '@/lib/utils';
import type { AgentWorkflowBinding, AgentWorkflowBindingCandidate } from '@/services/types/agent';

interface AgentRuntimeWorkflowDialogProps {
  open: boolean;
  bindings: AgentWorkflowBinding[];
  candidates: AgentWorkflowBindingCandidate[];
  isLoading: boolean;
  onOpenChange: (open: boolean) => void;
  onConfirmWorkflows: (bindingIds: string[]) => void;
}

export function AgentRuntimeWorkflowDialog({
  open,
  bindings,
  candidates,
  isLoading,
  onOpenChange,
  onConfirmWorkflows,
}: AgentRuntimeWorkflowDialogProps) {
  const t = useT('agents.agentRuntime');
  const [selectedBindingIds, setSelectedBindingIds] = useState<string[]>([]);
  const [search, setSearch] = useState('');

  useEffect(() => {
    if (!open) return;
    setSearch('');
    setSelectedBindingIds(bindings.map(binding => binding.binding_id));
  }, [bindings, open]);

  const filteredCandidates = useMemo(() => {
    const keyword = search.trim().toLowerCase();
    if (!keyword) return candidates;
    return candidates.filter(candidate =>
      [
        candidate.label,
        candidate.description,
        candidate.agent_id,
        candidate.workflow_id,
        candidate.version,
      ]
        .filter(Boolean)
        .some(value => String(value).toLowerCase().includes(keyword))
    );
  }, [candidates, search]);

  const toggleWorkflow = (bindingId: string, checked: boolean) => {
    setSelectedBindingIds(current =>
      checked
        ? Array.from(new Set([...current, bindingId]))
        : current.filter(selectedId => selectedId !== bindingId)
    );
  };

  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent size="lg">
        <DialogHeader>
          <div className="flex items-start justify-between gap-3">
            <div>
              <DialogTitle>{t('workflow.dialogTitle')}</DialogTitle>
              <DialogDescription>{t('workflow.dialogDescription')}</DialogDescription>
            </div>
            <Badge variant="subtle" className="mt-0.5 shrink-0">
              {t('workflow.selectedCount', { count: selectedBindingIds.length })}
            </Badge>
          </div>
        </DialogHeader>
        <DialogBody className="max-h-[560px]">
          <div className="space-y-3">
            <Input
              value={search}
              onChange={event => setSearch(event.target.value)}
              placeholder={t('workflow.searchPlaceholder')}
              leftIcon={<Search className="size-4" />}
            />
            {isLoading ? (
              <div className="grid grid-cols-1 gap-3 lg:grid-cols-2">
                {Array.from({ length: 4 }).map((_, index) => (
                  <Skeleton key={index} className="h-32 w-full rounded-lg" />
                ))}
              </div>
            ) : filteredCandidates.length === 0 ? (
              <div className="rounded-md border border-dashed p-4 text-sm text-muted-foreground">
                {t('workflow.noWorkflows')}
              </div>
            ) : (
              <div className="grid grid-cols-1 gap-3 lg:grid-cols-2">
                {filteredCandidates.map(candidate => (
                  <WorkflowOption
                    key={candidate.binding_id}
                    candidate={candidate}
                    selected={selectedBindingIds.includes(candidate.binding_id)}
                    onSelect={toggleWorkflow}
                  />
                ))}
              </div>
            )}
          </div>
        </DialogBody>
        <DialogFooter>
          <Button type="button" variant="ghost" onClick={() => onOpenChange(false)}>
            {t('workflow.cancel')}
          </Button>
          <Button
            type="button"
            onClick={() => {
              onOpenChange(false);
              onConfirmWorkflows(selectedBindingIds);
            }}
          >
            {t('workflow.confirm')}
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  );
}

function WorkflowOption({
  candidate,
  selected,
  onSelect,
}: {
  candidate: AgentWorkflowBindingCandidate;
  selected: boolean;
  onSelect: (id: string, checked: boolean) => void;
}) {
  const t = useT('agents.agentRuntime');

  return (
    <button
      type="button"
      className={cn(
        'flex min-h-32 cursor-pointer flex-col rounded-lg border bg-background p-4 text-left transition-colors hover:border-primary/50 hover:bg-muted/30',
        selected ? 'border-primary bg-primary/5' : ''
      )}
      onClick={() => onSelect(candidate.binding_id, !selected)}
    >
      <span className="flex items-start gap-3">
        <span className="flex size-10 shrink-0 items-center justify-center rounded-lg border bg-muted text-primary">
          <Workflow className="size-5" />
        </span>
        <span className="min-w-0 flex-1">
          <span className="block truncate text-sm font-semibold">
            {candidate.label || t('workflow.unnamedWorkflow')}
          </span>
          <span className="mt-1 inline-flex rounded border bg-muted/40 px-1.5 py-0.5 text-[11px] text-muted-foreground">
            {t('workflow.latestPublished')}
          </span>
        </span>
        <span
          className={cn(
            'flex size-5 shrink-0 items-center justify-center rounded-full border',
            selected ? 'border-primary bg-primary text-primary-foreground' : 'bg-background'
          )}
        >
          {selected ? <Check className="size-3.5" /> : null}
        </span>
      </span>
      <span className="mt-3 line-clamp-2 text-xs leading-5 text-muted-foreground">
        {candidate.description || t('workflow.noDescription')}
      </span>
      {candidate.version ? (
        <Badge variant="subtle" className="mt-3 w-fit">
          {t('workflow.versionLabel', { version: candidate.version })}
        </Badge>
      ) : null}
    </button>
  );
}
