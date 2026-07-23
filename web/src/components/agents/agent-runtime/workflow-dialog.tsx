'use client';

import { useCallback, useEffect, useMemo, useRef, useState } from 'react';
import { useInfiniteQuery } from '@tanstack/react-query';
import Link from 'next/link';
import { AlertCircle, Check, ExternalLink, RefreshCw, SearchX, Workflow } from 'lucide-react';
import { Button } from '@/components/ui/button';
import { WORKFLOW_PERMISSION_ACTIONS } from '@/constants/permissions';
import { useAccountPermissions } from '@/hooks/organization/use-account-permissions';
import { useDebouncedValue } from '@/hooks/use-debounced-value';
import { useInfiniteObserver } from '@/hooks/use-infinite-observer';
import { AGENT_KEYS } from '@/hooks/query-keys';
import { useT } from '@/i18n';
import { cn } from '@/lib/utils';
import agentService from '@/services/agent.service';
import type { AgentWorkflowBinding, AgentWorkflowBindingCandidate } from '@/services/types/agent';
import { formatDate } from '@/utils/format';
import {
  AgentRuntimeSelectionDialog,
  AgentRuntimeSelectionCardIcon,
  AgentRuntimeSelectionEmptyState,
  AgentRuntimeSelectionGrid,
  AgentRuntimeSelectionPagination,
  AgentRuntimeSelectionSkeleton,
} from './selection-dialog';
import { useSelectionDialogDraftGuard } from './use-selection-dialog-draft-guard';
import { AgentWorkflowTypeBadge, AgentWorkflowTypeIcon } from './workflow-type-display';

interface AgentRuntimeWorkflowDialogProps {
  agentId: string;
  open: boolean;
  bindings: AgentWorkflowBinding[];
  onOpenChange: (open: boolean) => void;
  onConfirmWorkflows: (bindings: AgentWorkflowBinding[]) => void;
}

export function AgentRuntimeWorkflowDialog({
  agentId,
  open,
  bindings,
  onOpenChange,
  onConfirmWorkflows,
}: AgentRuntimeWorkflowDialogProps) {
  const t = useT('agents.agentRuntime');
  const [selectedBindingIds, setSelectedBindingIds] = useState<string[]>([]);
  const [search, setSearch] = useState('');
  const candidateCacheRef = useRef(new Map<string, AgentWorkflowBindingCandidate>());
  const debouncedSearch = useDebouncedValue(search.trim(), 300);
  const { hasAnyPermission, isLoading: isPermissionsLoading } = useAccountPermissions();
  const canCreateWorkflow = hasAnyPermission(WORKFLOW_PERMISSION_ACTIONS.create);
  const selectedSet = useMemo(() => new Set(selectedBindingIds), [selectedBindingIds]);
  const normalizedSearch = debouncedSearch.trim();
  const candidatesQuery = useInfiniteQuery({
    queryKey: [...AGENT_KEYS.workflowBindingCandidates(agentId), 'dialog', normalizedSearch],
    initialPageParam: 1,
    queryFn: ({ pageParam }) =>
      agentService.getAgentWorkflowBindingCandidates(agentId, {
        query: normalizedSearch,
        page: pageParam as number,
        limit: 24,
      }),
    getNextPageParam: lastPage => {
      const page = lastPage.data;
      return page.has_more ? page.page + 1 : undefined;
    },
    enabled: open && Boolean(agentId),
    staleTime: 30 * 1000,
    retry: false,
  });

  useEffect(() => {
    if (!open) return;
    candidateCacheRef.current = new Map();
    setSearch('');
    setSelectedBindingIds(bindings.map(binding => binding.binding_id));
  }, [bindings, open]);

  const filteredCandidates = useMemo<AgentWorkflowBindingCandidate[]>(
    () => candidatesQuery.data?.pages.flatMap(page => page.data.data ?? []) ?? [],
    [candidatesQuery.data?.pages]
  );
  useEffect(() => {
    if (!open) return;
    filteredCandidates.forEach(candidate => {
      candidateCacheRef.current.set(candidate.binding_id, candidate);
    });
  }, [filteredCandidates, open]);
  const fetchNextCandidatePage = candidatesQuery.fetchNextPage;
  const loadNextPage = useCallback(() => fetchNextCandidatePage(), [fetchNextCandidatePage]);
  const sentinelRef = useInfiniteObserver({
    enabled: open,
    hasNextPage: Boolean(candidatesQuery.hasNextPage),
    isFetchingNextPage: candidatesQuery.isFetchingNextPage,
    fetchNextPage: loadNextPage,
    rootMargin: '240px',
  });

  const toggleWorkflow = (bindingId: string, checked: boolean) => {
    setSelectedBindingIds(current =>
      checked
        ? Array.from(new Set([...current, bindingId]))
        : current.filter(selectedId => selectedId !== bindingId)
    );
  };
  const originalBindingIds = useMemo(() => bindings.map(binding => binding.binding_id), [bindings]);
  const isDirty = useMemo(() => {
    if (selectedBindingIds.length !== originalBindingIds.length) return true;
    const original = new Set(originalBindingIds);
    return selectedBindingIds.some(bindingId => !original.has(bindingId));
  }, [originalBindingIds, selectedBindingIds]);
  const commitSelection = useCallback(() => {
    const existing = new Map(bindings.map(binding => [binding.binding_id, binding]));
    const next = selectedBindingIds.flatMap(bindingId => {
      const current = existing.get(bindingId);
      if (current) return [current];
      const candidate = candidateCacheRef.current.get(bindingId);
      return candidate ? [workflowBindingFromCandidate(candidate)] : [];
    });
    onConfirmWorkflows(next);
  }, [bindings, onConfirmWorkflows, selectedBindingIds]);
  const { requestOpenChange, requestClose, saveAndClose, closeGuard } =
    useSelectionDialogDraftGuard({
      open,
      isDirty,
      onOpenChange,
      onSave: commitSelection,
    });
  const searching =
    search.trim() !== debouncedSearch ||
    (candidatesQuery.isFetching && !candidatesQuery.isFetchingNextPage);
  const loadingCandidates = candidatesQuery.isLoading;

  return (
    <>
      <AgentRuntimeSelectionDialog
        open={open}
        title={t('workflow.dialogTitle')}
        description={t('workflow.dialogDescription')}
        selectedCount={selectedBindingIds.length}
        search={search}
        searchPlaceholder={t('workflow.searchPlaceholder')}
        isSearching={searching}
        onOpenChange={requestOpenChange}
        onChangeSearch={setSearch}
        footer={
          <>
            <Button type="button" variant="ghost" onClick={requestClose}>
              {t('workflow.cancel')}
            </Button>
            <Button type="button" onClick={saveAndClose}>
              {t('workflow.confirm')}
            </Button>
          </>
        }
      >
        {loadingCandidates ||
        (!normalizedSearch && filteredCandidates.length === 0 && isPermissionsLoading) ? (
          <AgentRuntimeSelectionSkeleton />
        ) : candidatesQuery.isError ? (
          <AgentRuntimeSelectionEmptyState
            icon={<AlertCircle />}
            title={t('workflow.loadFailedTitle')}
            description={t('workflow.loadFailedDescription')}
            action={
              <Button
                type="button"
                variant="outline"
                size="sm"
                onClick={() => void candidatesQuery.refetch()}
              >
                <RefreshCw className="size-4" />
                {t('workflow.retryLoad')}
              </Button>
            }
          />
        ) : filteredCandidates.length === 0 ? (
          normalizedSearch ? (
            <AgentRuntimeSelectionEmptyState
              variant="search"
              icon={<SearchX />}
              title={t('workflow.searchEmptyTitle')}
              description={t('workflow.searchEmptyDescription', { query: normalizedSearch })}
              action={
                <Button type="button" variant="outline" size="sm" onClick={() => setSearch('')}>
                  {t('selectionDialog.clearSearch')}
                </Button>
              }
            />
          ) : (
            <AgentRuntimeSelectionEmptyState
              icon={<Workflow />}
              title={t('workflow.emptyTitle')}
              description={t(
                canCreateWorkflow
                  ? 'workflow.emptyDescription'
                  : 'workflow.emptyUnavailableDescription'
              )}
              action={
                canCreateWorkflow ? (
                  <Button asChild size="sm">
                    <Link href="/console/workflows" target="_blank" rel="noreferrer">
                      {t('workflow.createAction')}
                      <ExternalLink className="size-4" />
                    </Link>
                  </Button>
                ) : undefined
              }
            />
          )
        ) : (
          <>
            <AgentRuntimeSelectionGrid>
              {filteredCandidates.map(candidate => (
                <WorkflowOption
                  key={candidate.binding_id}
                  candidate={candidate}
                  selected={selectedSet.has(candidate.binding_id)}
                  onSelect={toggleWorkflow}
                />
              ))}
            </AgentRuntimeSelectionGrid>
            <AgentRuntimeSelectionPagination
              sentinelRef={sentinelRef}
              isFetchingNextPage={candidatesQuery.isFetchingNextPage}
              hasNextPage={Boolean(candidatesQuery.hasNextPage)}
              hasItems={filteredCandidates.length > 0}
            />
          </>
        )}
      </AgentRuntimeSelectionDialog>
      {closeGuard}
    </>
  );
}

function workflowBindingFromCandidate(
  candidate: AgentWorkflowBindingCandidate
): AgentWorkflowBinding {
  return {
    binding_id: candidate.binding_id,
    label: candidate.label,
    description: candidate.description,
    agent_id: candidate.agent_id,
    workflow_id: candidate.workflow_id,
    agent_type: candidate.agent_type,
    version_strategy: 'latest_published',
    timeout_seconds: candidate.timeout_seconds ?? 600,
  };
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
      aria-pressed={selected}
      className={cn(
        'flex min-h-32 cursor-pointer flex-col rounded-lg border bg-background p-3.5 text-left shadow-sm transition-colors hover:border-primary/40 hover:bg-muted/20',
        selected && 'border-primary bg-primary/5 hover:bg-primary/10'
      )}
      onClick={() => onSelect(candidate.binding_id, !selected)}
    >
      <span className="flex min-h-8 items-center gap-3">
        <AgentRuntimeSelectionCardIcon className="text-primary">
          <AgentWorkflowTypeIcon agentType={candidate.agent_type} />
        </AgentRuntimeSelectionCardIcon>
        <span className="flex min-h-8 min-w-0 flex-1 items-center gap-2">
          <span className="min-w-0 flex-1 truncate text-sm font-semibold">
            {candidate.label || t('workflow.unnamedWorkflow')}
          </span>
          <AgentWorkflowTypeBadge agentType={candidate.agent_type} className="shrink-0" />
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
      <span className="mt-2.5 line-clamp-3 text-xs leading-5 text-muted-foreground">
        {candidate.description || t('workflow.noDescription')}
      </span>
      {candidate.updated_at ? (
        <span className="mt-auto pt-2.5 text-[11px] text-muted-foreground">
          {t('workflow.updatedAt', { time: formatDate(candidate.updated_at) })}
        </span>
      ) : null}
    </button>
  );
}
