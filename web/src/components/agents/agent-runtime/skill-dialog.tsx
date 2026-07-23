'use client';

import { useCallback, useEffect, useMemo, useState } from 'react';
import { useInfiniteQuery } from '@tanstack/react-query';
import { AlertCircle, Check, Puzzle, RefreshCw, SearchX } from 'lucide-react';
import { getAIChatSkillDisplayInfo } from '@/components/chat/variants/aichat/skill-display';
import { AIChatSkillIcon } from '@/components/chat/variants/aichat/skill-icon';
import { Button } from '@/components/ui/button';
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from '@/components/ui/select';
import { useDebouncedValue } from '@/hooks/use-debounced-value';
import { useInfiniteObserver } from '@/hooks/use-infinite-observer';
import { AGENT_KEYS } from '@/hooks/query-keys';
import { useT } from '@/i18n';
import { cn } from '@/lib/utils';
import agentService from '@/services/agent.service';
import type { AgentSkillBindingCandidate } from '@/services/types/agent';
import type { AIChatSkillMetadata } from '@/services/types/aichat';
import {
  AgentRuntimeSelectionDialog,
  AgentRuntimeSelectionCardIcon,
  AgentRuntimeSelectionEmptyState,
  AgentRuntimeSelectionGrid,
  AgentRuntimeSelectionPagination,
  AgentRuntimeSelectionSkeleton,
} from './selection-dialog';
import { useSelectionDialogDraftGuard } from './use-selection-dialog-draft-guard';

interface AgentRuntimeSkillDialogProps {
  agentId: string;
  open: boolean;
  locale: string;
  normalizedSelectedSkillIds: string[];
  onOpenChange: (open: boolean) => void;
  onConfirmSkills: (skillIds: string[]) => void;
}

export function AgentRuntimeSkillDialog({
  agentId,
  open,
  locale,
  normalizedSelectedSkillIds,
  onOpenChange,
  onConfirmSkills,
}: AgentRuntimeSkillDialogProps) {
  const t = useT('agents.agentRuntime');
  const [sourceFilter, setSourceFilter] = useState<'all' | 'system' | 'custom'>('all');
  const [localSelectedSkillIds, setLocalSelectedSkillIds] = useState<string[]>([]);
  const [search, setSearch] = useState('');
  const debouncedSearch = useDebouncedValue(search.trim(), 300);
  const normalizedSearch = debouncedSearch.trim();
  const selectedSet = useMemo(() => new Set(localSelectedSkillIds), [localSelectedSkillIds]);
  const candidatesQuery = useInfiniteQuery({
    queryKey: [
      ...AGENT_KEYS.skillBindingCandidates(agentId),
      'dialog',
      sourceFilter,
      normalizedSearch,
    ],
    initialPageParam: 1,
    queryFn: ({ pageParam }) =>
      agentService.getAgentSkillBindingCandidates(agentId, {
        query: normalizedSearch || undefined,
        source: sourceFilter === 'all' ? undefined : sourceFilter,
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
    setSourceFilter('all');
    setLocalSelectedSkillIds(normalizedSelectedSkillIds);
  }, [normalizedSelectedSkillIds, open]);

  const candidates = useMemo(
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

  const toggleSkill = (skillId: string, checked: boolean) => {
    setLocalSelectedSkillIds(current =>
      checked ? Array.from(new Set([...current, skillId])) : current.filter(id => id !== skillId)
    );
  };
  const isDirty = useMemo(() => {
    if (localSelectedSkillIds.length !== normalizedSelectedSkillIds.length) return true;
    const original = new Set(normalizedSelectedSkillIds);
    return localSelectedSkillIds.some(skillId => !original.has(skillId));
  }, [localSelectedSkillIds, normalizedSelectedSkillIds]);
  const commitSelection = useCallback(
    () => onConfirmSkills(localSelectedSkillIds),
    [localSelectedSkillIds, onConfirmSkills]
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
  const isSearchResult = Boolean(normalizedSearch);

  return (
    <>
      <AgentRuntimeSelectionDialog
        open={open}
        title={t('skills.dialogTitle')}
        description={t('skills.dialogDescription')}
        selectedCount={localSelectedSkillIds.length}
        search={search}
        searchPlaceholder={t('skills.searchPlaceholder')}
        isSearching={searching}
        onOpenChange={requestOpenChange}
        onChangeSearch={setSearch}
        toolbar={
          <Select
            value={sourceFilter}
            onValueChange={value => setSourceFilter(value as 'all' | 'system' | 'custom')}
          >
            <SelectTrigger className="h-9 w-full shrink-0 bg-background sm:w-48">
              <SelectValue />
            </SelectTrigger>
            <SelectContent>
              <SelectItem value="all">{t('skills.all')}</SelectItem>
              <SelectItem value="system">{t('skills.system')}</SelectItem>
              <SelectItem value="custom">{t('skills.custom')}</SelectItem>
            </SelectContent>
          </Select>
        }
        footer={
          <>
            <Button type="button" variant="ghost" onClick={requestClose}>
              {t('skills.cancel')}
            </Button>
            <Button type="button" onClick={saveAndClose}>
              {t('skills.done')}
            </Button>
          </>
        }
      >
        {candidatesQuery.isLoading ? (
          <AgentRuntimeSelectionSkeleton />
        ) : candidatesQuery.isError ? (
          <AgentRuntimeSelectionEmptyState
            icon={<AlertCircle />}
            title={t('skills.loadFailedTitle')}
            description={t('skills.loadFailedDescription')}
            action={
              <Button
                type="button"
                variant="outline"
                size="sm"
                onClick={() => void candidatesQuery.refetch()}
              >
                <RefreshCw className="size-4" />
                {t('skills.retryLoad')}
              </Button>
            }
          />
        ) : candidates.length === 0 ? (
          <AgentRuntimeSelectionEmptyState
            variant={isSearchResult ? 'search' : 'resource'}
            icon={isSearchResult ? <SearchX /> : <Puzzle />}
            title={isSearchResult ? t('skills.noMatch') : t('skills.enablePrompt')}
            description={
              isSearchResult ? t('skills.searchPlaceholder') : t('skills.dialogDescription')
            }
          />
        ) : (
          <>
            <AgentRuntimeSelectionGrid>
              {candidates.map(candidate => {
                const skill = candidateToSkillMetadata(candidate);
                const display = getAIChatSkillDisplayInfo(skill, locale);
                const checked = selectedSet.has(candidate.skill_id);
                return (
                  <button
                    key={candidate.skill_id}
                    type="button"
                    aria-pressed={checked}
                    className={cn(
                      'flex min-h-36 cursor-pointer flex-col rounded-lg border bg-background p-3.5 text-left shadow-sm transition-colors',
                      checked
                        ? 'border-primary bg-primary/5 hover:bg-primary/10'
                        : 'hover:border-primary/40 hover:bg-muted/20'
                    )}
                    onClick={() => toggleSkill(candidate.skill_id, !checked)}
                  >
                    <span className="flex min-h-8 items-center gap-3">
                      <AgentRuntimeSelectionCardIcon>
                        <AIChatSkillIcon icon={display.icon} />
                      </AgentRuntimeSelectionCardIcon>
                      <span className="flex min-h-8 min-w-0 flex-1 items-center">
                        <span className="block truncate text-sm font-semibold">
                          {display.label}
                        </span>
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
                      {display.description || candidate.description || display.whenToUse || ''}
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

function candidateToSkillMetadata(candidate: AgentSkillBindingCandidate): AIChatSkillMetadata {
  return {
    skill_id: candidate.skill_id,
    source: candidate.source === 'custom' ? 'custom' : 'system',
    name: candidate.name,
    description: candidate.description ?? '',
    when_to_use: candidate.when_to_use ?? '',
    runtime_type: (candidate.runtime_type || 'prompt') as AIChatSkillMetadata['runtime_type'],
    enabled: true,
    display: candidate.display,
    has_tools: candidate.has_tools,
    has_references: candidate.has_references,
    has_scripts: candidate.has_scripts,
    scripts_supported: candidate.scripts_supported,
    max_calls_per_turn: 0,
    timeout_seconds: 0,
    required_config: candidate.required_config,
  };
}
