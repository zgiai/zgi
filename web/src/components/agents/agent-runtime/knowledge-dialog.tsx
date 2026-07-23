'use client';

import { useCallback, useEffect, useMemo, useState } from 'react';
import { useInfiniteQuery } from '@tanstack/react-query';
import Link from 'next/link';
import {
  AlertCircle,
  BookOpen,
  Check,
  Database,
  ExternalLink,
  RefreshCw,
  SearchX,
} from 'lucide-react';
import { Button } from '@/components/ui/button';
import { KNOWLEDGE_BASE_PERMISSION_ACTIONS } from '@/constants/permissions';
import { useAccountPermissions } from '@/hooks/organization/use-account-permissions';
import { useDebouncedValue } from '@/hooks/use-debounced-value';
import { useInfiniteObserver } from '@/hooks/use-infinite-observer';
import { AGENT_KEYS } from '@/hooks/query-keys';
import { useT } from '@/i18n';
import { cn } from '@/lib/utils';
import agentService from '@/services/agent.service';
import type { AgentKnowledgeBindingCandidate } from '@/services/types/agent';
import {
  AgentRuntimeSelectionDialog,
  AgentRuntimeSelectionCardIcon,
  AgentRuntimeSelectionEmptyState,
  AgentRuntimeSelectionGrid,
  AgentRuntimeSelectionPagination,
  AgentRuntimeSelectionSkeleton,
} from './selection-dialog';
import { useSelectionDialogDraftGuard } from './use-selection-dialog-draft-guard';

interface AgentRuntimeKnowledgeDialogProps {
  agentId: string;
  open: boolean;
  selectedDatasetIds: string[];
  onOpenChange: (open: boolean) => void;
  onConfirmDatasets: (datasetIds: string[]) => void;
}

export function AgentRuntimeKnowledgeDialog({
  agentId,
  open,
  selectedDatasetIds,
  onOpenChange,
  onConfirmDatasets,
}: AgentRuntimeKnowledgeDialogProps) {
  const t = useT('agents.agentRuntime');
  const [localSelectedDatasetIds, setLocalSelectedDatasetIds] = useState<string[]>([]);
  const [search, setSearch] = useState('');
  const debouncedSearch = useDebouncedValue(search.trim(), 300);
  const normalizedSearch = debouncedSearch.trim();
  const selectedSet = useMemo(() => new Set(localSelectedDatasetIds), [localSelectedDatasetIds]);
  const { hasAnyPermission, isLoading: isPermissionsLoading } = useAccountPermissions();
  const canCreateKnowledge = hasAnyPermission(KNOWLEDGE_BASE_PERMISSION_ACTIONS.create);
  const candidatesQuery = useInfiniteQuery({
    queryKey: [...AGENT_KEYS.knowledgeBindingCandidates(agentId), 'dialog', normalizedSearch],
    initialPageParam: 1,
    queryFn: ({ pageParam }) =>
      agentService.getAgentKnowledgeBindingCandidates(agentId, {
        query: normalizedSearch || undefined,
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
    setSearch('');
    setLocalSelectedDatasetIds(selectedDatasetIds);
  }, [open, selectedDatasetIds]);

  const candidates = useMemo<AgentKnowledgeBindingCandidate[]>(
    () => candidatesQuery.data?.pages.flatMap(page => page.data.data ?? []) ?? [],
    [candidatesQuery.data?.pages]
  );
  const fetchNextCandidatePage = candidatesQuery.fetchNextPage;
  const loadNextPage = useCallback(() => fetchNextCandidatePage(), [fetchNextCandidatePage]);
  const sentinelRef = useInfiniteObserver({
    enabled: open,
    hasNextPage: Boolean(candidatesQuery.hasNextPage),
    isFetchingNextPage: candidatesQuery.isFetchingNextPage,
    fetchNextPage: loadNextPage,
    rootMargin: '240px',
  });
  const isDirty = useMemo(() => {
    if (localSelectedDatasetIds.length !== selectedDatasetIds.length) return true;
    const original = new Set(selectedDatasetIds);
    return localSelectedDatasetIds.some(datasetId => !original.has(datasetId));
  }, [localSelectedDatasetIds, selectedDatasetIds]);
  const commitSelection = useCallback(
    () => onConfirmDatasets(localSelectedDatasetIds),
    [localSelectedDatasetIds, onConfirmDatasets]
  );
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

  const toggleDataset = (datasetId: string, checked: boolean) => {
    setLocalSelectedDatasetIds(current =>
      checked
        ? Array.from(new Set([...current, datasetId]))
        : current.filter(id => id !== datasetId)
    );
  };

  return (
    <>
      <AgentRuntimeSelectionDialog
        open={open}
        title={t('knowledge.dialogTitle')}
        description={t('knowledge.dialogDescription')}
        selectedCount={localSelectedDatasetIds.length}
        search={search}
        searchPlaceholder={t('knowledge.searchPlaceholder')}
        isSearching={searching}
        onOpenChange={requestOpenChange}
        onChangeSearch={setSearch}
        footer={
          <>
            <Button type="button" variant="ghost" onClick={requestClose}>
              {t('knowledge.cancel')}
            </Button>
            <Button type="button" onClick={saveAndClose}>
              {t('knowledge.done')}
            </Button>
          </>
        }
      >
        {candidatesQuery.isLoading ||
        (!normalizedSearch && candidates.length === 0 && isPermissionsLoading) ? (
          <AgentRuntimeSelectionSkeleton />
        ) : candidatesQuery.isError ? (
          <AgentRuntimeSelectionEmptyState
            icon={<AlertCircle />}
            title={t('knowledge.listLoadFailedTitle')}
            description={t('knowledge.listLoadFailedDescription')}
            action={
              <Button
                type="button"
                variant="outline"
                size="sm"
                onClick={() => void candidatesQuery.refetch()}
              >
                <RefreshCw className="size-4" />
                {t('knowledge.retryLoad')}
              </Button>
            }
          />
        ) : candidates.length === 0 ? (
          normalizedSearch ? (
            <AgentRuntimeSelectionEmptyState
              variant="search"
              icon={<SearchX />}
              title={t('knowledge.searchEmptyTitle')}
              description={t('knowledge.searchEmptyDescription', { query: normalizedSearch })}
              action={
                <Button type="button" variant="outline" size="sm" onClick={() => setSearch('')}>
                  {t('selectionDialog.clearSearch')}
                </Button>
              }
            />
          ) : (
            <AgentRuntimeSelectionEmptyState
              icon={<BookOpen />}
              title={t('knowledge.emptyTitle')}
              description={t(
                canCreateKnowledge
                  ? 'knowledge.emptyDescription'
                  : 'knowledge.emptyUnavailableDescription'
              )}
              action={
                canCreateKnowledge ? (
                  <Button asChild size="sm">
                    <Link href="/console/dataset" target="_blank" rel="noreferrer">
                      {t('knowledge.createAction')}
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
              {candidates.map(dataset => {
                const checked = selectedSet.has(dataset.dataset_id);
                return (
                  <button
                    key={dataset.dataset_id}
                    type="button"
                    aria-pressed={checked}
                    className={cn(
                      'flex min-h-32 cursor-pointer flex-col rounded-lg border bg-background p-3.5 text-left shadow-sm transition-colors hover:border-primary/40 hover:bg-muted/20',
                      checked && 'border-primary bg-primary/5 hover:bg-primary/10'
                    )}
                    onClick={() => toggleDataset(dataset.dataset_id, !checked)}
                  >
                    <span className="flex min-h-8 items-center gap-3">
                      <AgentRuntimeSelectionCardIcon className="text-primary">
                        <Database />
                      </AgentRuntimeSelectionCardIcon>
                      <span className="flex min-h-8 min-w-0 flex-1 items-center">
                        <span className="block truncate text-sm font-semibold">{dataset.name}</span>
                      </span>
                      <span
                        className={cn(
                          'flex size-5 shrink-0 items-center justify-center rounded-full border',
                          checked
                            ? 'border-primary bg-primary text-primary-foreground'
                            : 'bg-background'
                        )}
                      >
                        {checked ? <Check className="size-3.5" /> : null}
                      </span>
                    </span>
                    <span className="mt-2.5 line-clamp-3 text-xs leading-5 text-muted-foreground">
                      {dataset.description || t('knowledge.noDescription')}
                    </span>
                  </button>
                );
              })}
            </AgentRuntimeSelectionGrid>
            <AgentRuntimeSelectionPagination
              sentinelRef={sentinelRef}
              isFetchingNextPage={candidatesQuery.isFetchingNextPage}
              hasNextPage={Boolean(candidatesQuery.hasNextPage)}
              hasItems={candidates.length > 0}
            />
          </>
        )}
      </AgentRuntimeSelectionDialog>
      {closeGuard}
    </>
  );
}
