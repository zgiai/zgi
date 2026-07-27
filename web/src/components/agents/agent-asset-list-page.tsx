'use client';

import { useEffect, useMemo, useRef, useState } from 'react';
import { useSearchParams } from 'next/navigation';
import { Plus, RefreshCw, Loader2, Search, Upload, ShieldAlert } from 'lucide-react';
import { useQueryClient, type InfiniteData } from '@tanstack/react-query';
import { toast } from 'sonner';
import { Button } from '@/components/ui/button';
import { Input } from '@/components/ui/input';
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from '@/components/ui/select';
import { Skeleton } from '@/components/ui/skeleton';
import AgentCard from '@/components/agents/agent-card';
import { AgentsAIChatContextRegistration } from '@/components/agents/aichat-context';
import AgentDialog from '@/components/agents/agent-dialog';
import { AgentEmptyElement, AgentEmptySearchResults } from '@/components/agents/empty-element';
import ImportAgentDialog from '@/components/agents/import-agent-dialog';
import { TemplateGalleryDialog } from '@/components/agents/templates';
import { useAgents } from '@/hooks/agent/use-agents';
import { AGENT_KEYS } from '@/hooks/query-keys';
import { useDebouncedValue } from '@/hooks/use-debounced-value';
import { useAccountPermissions } from '@/hooks/organization/use-account-permissions';
import { useT } from '@/i18n';
import { useCurrentWorkspace } from '@/store/workspace-store';
import type { ApiResponseData } from '@/services/types/common';
import {
  AgentType,
  type AgentAssetKind,
  type AgentList,
  type WebAppStatus,
} from '@/services/types/agent';
import {
  consumeAgentListRestoreIntent,
  markAgentListDetailEntry,
  readAgentListInitialFilters,
  readAgentListInitialKeyword,
  readAgentListNavigationState,
  writeAgentListNavigationState,
  type AgentListPublishFilter,
  type AgentListNavigationState,
  type AgentListScope,
  type AgentListWorkflowTypeFilter,
} from '@/utils/agent-list-state';
import {
  AGENT_MANAGE_PERMISSION_CODES,
  AGENT_PERMISSION_ACTIONS,
  WORKFLOW_PERMISSION_ACTIONS,
} from '@/constants/permissions';

const PAGE_SIZE = 20;

interface AgentAssetListPageProps {
  assetKind: AgentAssetKind;
}

export function AgentAssetListPage({ assetKind }: AgentAssetListPageProps) {
  const t = useT();
  const searchParams = useSearchParams();
  const currentWorkspace = useCurrentWorkspace();
  const queryClient = useQueryClient();
  const isWorkflowList = assetKind === 'workflow';
  const listScope: AgentListScope = isWorkflowList ? 'workflows' : 'agents';

  const { hasAnyPermission, isLoading: isPermissionsLoading } = useAccountPermissions();
  const canView = hasAnyPermission(
    isWorkflowList ? WORKFLOW_PERMISSION_ACTIONS.page : AGENT_PERMISSION_ACTIONS.page
  );
  const canCreateAgent = hasAnyPermission(AGENT_PERMISSION_ACTIONS.create);
  const canCreateWorkflow = hasAnyPermission(WORKFLOW_PERMISSION_ACTIONS.create);
  const canManageAgent = hasAnyPermission(AGENT_MANAGE_PERMISSION_CODES);
  const canCreateBlank = isWorkflowList ? canCreateWorkflow : canCreateAgent;
  const canImportWorkflow = isWorkflowList && hasAnyPermission(WORKFLOW_PERMISSION_ACTIONS.import);
  const canCreate = isWorkflowList ? canCreateBlank || canImportWorkflow : canCreateBlank;

  const [open, setOpen] = useState(false);
  const [templateOpen, setTemplateOpen] = useState(false);
  const [importOpen, setImportOpen] = useState(false);
  const [searchKeyword, setSearchKeyword] = useState(() => readAgentListInitialKeyword(listScope));
  const [publishFilter, setPublishFilter] = useState<AgentListPublishFilter>(
    () => readAgentListInitialFilters(listScope).publishFilter
  );
  const [workflowTypeFilter, setWorkflowTypeFilter] = useState<AgentListWorkflowTypeFilter>(
    () => readAgentListInitialFilters(listScope).workflowTypeFilter
  );
  const [queryKeywordOverride, setQueryKeywordOverride] = useState<string | null>(null);
  const [isRestoreChecked, setIsRestoreChecked] = useState(false);
  const [reloading, setReloading] = useState(false);
  const listScrollRef = useRef<HTMLDivElement>(null);
  const loadMoreRef = useRef<HTMLDivElement>(null);
  const pendingRestoreRef = useRef<AgentListNavigationState | null>(null);
  const hasRestoredScrollRef = useRef(false);
  const hasRefreshedRestoredPagesRef = useRef(false);
  const scrollSaveFrameRef = useRef<number | null>(null);

  const debouncedSearchKeyword = useDebouncedValue(searchKeyword, 500);
  const effectiveSearchKeyword = queryKeywordOverride ?? debouncedSearchKeyword;
  const isPublished = publishFilter === 'all' ? undefined : publishFilter !== 'draft';
  const webAppStatus: WebAppStatus | undefined =
    publishFilter === 'published' ? 'active' : publishFilter === 'offline' ? 'inactive' : undefined;
  const filteredAgentType =
    isWorkflowList && workflowTypeFilter !== 'all' ? (workflowTypeFilter as AgentType) : undefined;
  const templateFromQuery = isWorkflowList ? searchParams.get('template') : null;
  const agentListParams = useMemo(
    () => ({
      limit: PAGE_SIZE,
      keyword: effectiveSearchKeyword || undefined,
      workspace_id: currentWorkspace?.id,
      asset_kind: assetKind,
      agent_type: filteredAgentType,
      is_published: isPublished,
      web_app_status: webAppStatus,
    }),
    [
      assetKind,
      effectiveSearchKeyword,
      filteredAgentType,
      currentWorkspace?.id,
      isPublished,
      webAppStatus,
    ]
  );
  const agentListQueryKey = useMemo(() => AGENT_KEYS.list(agentListParams), [agentListParams]);
  const title = isWorkflowList ? t('agents.workflowListTitle') : t('agents.title');
  const searchPlaceholder = isWorkflowList
    ? t('agents.workflowSearchPlaceholder')
    : t('agents.searchPlaceholder');
  const createLabel = isWorkflowList ? t('agents.createWorkflow') : t('agents.create');
  const createFirstLabel = isWorkflowList
    ? t('agents.createFirstWorkflow')
    : t('agents.createFirstAgent');
  const importLabel = t('agents.importWorkflow');
  const emptyTitle = isWorkflowList ? t('agents.noWorkflowsYet') : undefined;
  const emptyDescription = isWorkflowList ? t('agents.noWorkflowsDescription') : undefined;
  const dialogAgentTypes = useMemo(
    () =>
      isWorkflowList ? [AgentType.CONVERSATIONAL_AGENT, AgentType.WORKFLOW] : [AgentType.AGENT],
    [isWorkflowList]
  );

  useEffect(() => {
    if (templateFromQuery) {
      setTemplateOpen(true);
    }
  }, [templateFromQuery]);

  useEffect(() => {
    if (!currentWorkspace?.id) {
      setIsRestoreChecked(true);
      return;
    }

    const shouldRestore = consumeAgentListRestoreIntent(listScope);
    const savedState = shouldRestore ? readAgentListNavigationState(listScope) : null;
    if (!savedState || savedState.workspaceId !== currentWorkspace.id) {
      pendingRestoreRef.current = null;
      hasRestoredScrollRef.current = false;
      hasRefreshedRestoredPagesRef.current = false;
      setSearchKeyword('');
      setPublishFilter('all');
      setWorkflowTypeFilter('all');
      setQueryKeywordOverride('');
      setIsRestoreChecked(true);
      return;
    }

    setSearchKeyword(savedState.keyword);
    setPublishFilter(savedState.publishFilter);
    setWorkflowTypeFilter(isWorkflowList ? savedState.workflowTypeFilter : 'all');
    setQueryKeywordOverride(savedState.keyword);
    pendingRestoreRef.current = savedState;

    const restoredParams = {
      limit: PAGE_SIZE,
      keyword: savedState.keyword || undefined,
      workspace_id: currentWorkspace.id,
      asset_kind: assetKind,
      agent_type:
        isWorkflowList && savedState.workflowTypeFilter !== 'all'
          ? (savedState.workflowTypeFilter as AgentType)
          : undefined,
      is_published:
        savedState.publishFilter === 'all' ? undefined : savedState.publishFilter !== 'draft',
      web_app_status:
        savedState.publishFilter === 'published'
          ? 'active'
          : savedState.publishFilter === 'offline'
            ? 'inactive'
            : undefined,
    };
    const restoredQueryKey = AGENT_KEYS.list(restoredParams);
    const existingCachedPages =
      queryClient.getQueryData<InfiniteData<ApiResponseData<AgentList>, number>>(
        restoredQueryKey
      )?.pages;

    if (savedState.pages?.length && !existingCachedPages?.length) {
      queryClient.setQueryData<InfiniteData<ApiResponseData<AgentList>, number>>(restoredQueryKey, {
        pages: savedState.pages,
        pageParams: savedState.pages.map((_, index) => index + 1),
      });
    }

    setIsRestoreChecked(true);
  }, [assetKind, currentWorkspace?.id, isWorkflowList, listScope, queryClient]);

  const {
    pages,
    fetchNextPage,
    hasNextPage,
    isFetchingNextPage,
    isLoading: isAgentsLoading,
    isFetching,
    refetchFromPageAndAfter,
  } = useAgents(agentListParams, {
    enabled: canView && isRestoreChecked && Boolean(currentWorkspace?.id),
  });

  const isLoading = isAgentsLoading || isPermissionsLoading;
  const agents = pages.flat();

  const persistNavigationState = (nextScrollTop?: number, includePages = false) => {
    if (!isRestoreChecked || !currentWorkspace?.id) return;

    const cached = includePages
      ? queryClient.getQueryData<InfiniteData<ApiResponseData<AgentList>, number>>(
          agentListQueryKey
        )
      : undefined;
    writeAgentListNavigationState(
      {
        keyword: effectiveSearchKeyword,
        publishFilter,
        workflowTypeFilter: isWorkflowList ? workflowTypeFilter : 'all',
        loadedPageCount: pages.length,
        scrollTop: nextScrollTop ?? listScrollRef.current?.scrollTop ?? 0,
        workspaceId: currentWorkspace.id,
        pages: cached?.pages,
        updatedAt: Date.now(),
      },
      { includePages },
      listScope
    );
  };

  const handleListScroll = () => {
    if (scrollSaveFrameRef.current !== null) return;

    scrollSaveFrameRef.current = window.requestAnimationFrame(() => {
      scrollSaveFrameRef.current = null;
      persistNavigationState(listScrollRef.current?.scrollTop ?? 0);
    });
  };

  useEffect(() => {
    return () => {
      if (scrollSaveFrameRef.current !== null) {
        window.cancelAnimationFrame(scrollSaveFrameRef.current);
      }
    };
  }, []);

  useEffect(() => {
    if (!isRestoreChecked || !currentWorkspace?.id) return;

    persistNavigationState(undefined, true);
    const frame = window.requestAnimationFrame(() => persistNavigationState(undefined, true));
    return () => window.cancelAnimationFrame(frame);
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [
    agentListQueryKey,
    currentWorkspace?.id,
    effectiveSearchKeyword,
    isRestoreChecked,
    pages,
    publishFilter,
    workflowTypeFilter,
  ]);

  useEffect(() => {
    const pendingRestore = pendingRestoreRef.current;
    if (!pendingRestore || !isRestoreChecked || !canView) return;
    if (pendingRestore.keyword !== effectiveSearchKeyword) return;
    if (pendingRestore.publishFilter !== publishFilter) return;
    if (isWorkflowList && pendingRestore.workflowTypeFilter !== workflowTypeFilter) return;
    if (pages.length >= pendingRestore.loadedPageCount || !hasNextPage || isFetchingNextPage) {
      return;
    }

    void fetchNextPage();
  }, [
    canView,
    effectiveSearchKeyword,
    fetchNextPage,
    hasNextPage,
    isFetchingNextPage,
    isRestoreChecked,
    isWorkflowList,
    pages.length,
    publishFilter,
    workflowTypeFilter,
  ]);

  useEffect(() => {
    const pendingRestore = pendingRestoreRef.current;
    if (!pendingRestore || hasRestoredScrollRef.current || !isRestoreChecked) return;
    if (pendingRestore.keyword !== effectiveSearchKeyword) return;
    if (pendingRestore.publishFilter !== publishFilter) return;
    if (isWorkflowList && pendingRestore.workflowTypeFilter !== workflowTypeFilter) return;
    if (pages.length < pendingRestore.loadedPageCount && hasNextPage) return;

    const frame = window.requestAnimationFrame(() => {
      const scrollContainer = listScrollRef.current;
      if (!scrollContainer) return;

      scrollContainer.scrollTop = pendingRestore.scrollTop;
      hasRestoredScrollRef.current = true;
    });

    return () => window.cancelAnimationFrame(frame);
  }, [
    effectiveSearchKeyword,
    hasNextPage,
    isRestoreChecked,
    isWorkflowList,
    pages.length,
    publishFilter,
    workflowTypeFilter,
  ]);

  useEffect(() => {
    const pendingRestore = pendingRestoreRef.current;
    if (!pendingRestore || hasRefreshedRestoredPagesRef.current || !isRestoreChecked) return;
    if (pendingRestore.keyword !== effectiveSearchKeyword) return;
    if (pendingRestore.publishFilter !== publishFilter) return;
    if (isWorkflowList && pendingRestore.workflowTypeFilter !== workflowTypeFilter) return;
    if (pages.length === 0) return;

    hasRefreshedRestoredPagesRef.current = true;
    void refetchFromPageAndAfter(0);
  }, [
    effectiveSearchKeyword,
    isRestoreChecked,
    isWorkflowList,
    pages.length,
    publishFilter,
    refetchFromPageAndAfter,
    workflowTypeFilter,
  ]);

  useEffect(() => {
    if (!canView) return;
    const observer = new IntersectionObserver(
      entries => {
        if (entries[0]?.isIntersecting && hasNextPage && !isFetchingNextPage) {
          void fetchNextPage();
        }
      },
      { threshold: 0, rootMargin: '100px' }
    );

    const currentRef = loadMoreRef.current;
    if (currentRef) {
      observer.observe(currentRef);
    }

    return () => {
      if (currentRef) {
        observer.unobserve(currentRef);
      }
    };
  }, [hasNextPage, isFetchingNextPage, fetchNextPage, canView]);

  const resetListPosition = () => {
    pendingRestoreRef.current = null;
    hasRestoredScrollRef.current = false;
    hasRefreshedRestoredPagesRef.current = false;
    if (listScrollRef.current) {
      listScrollRef.current.scrollTop = 0;
    }
  };

  const handleSearchChange = (value: string) => {
    resetListPosition();
    setQueryKeywordOverride(null);
    setSearchKeyword(value);
  };

  const handlePublishFilterChange = (value: AgentListPublishFilter) => {
    resetListPosition();
    setPublishFilter(value);
  };

  const handleWorkflowTypeFilterChange = (value: AgentListWorkflowTypeFilter) => {
    resetListPosition();
    setWorkflowTypeFilter(value);
  };

  const handleClearCriteria = () => {
    resetListPosition();
    setSearchKeyword('');
    setQueryKeywordOverride('');
    setPublishFilter('all');
    setWorkflowTypeFilter('all');
  };

  const hasActiveFilters =
    publishFilter !== 'all' || (isWorkflowList && workflowTypeFilter !== 'all');
  const hasActiveCriteria = Boolean(effectiveSearchKeyword) || hasActiveFilters;

  const handleReload = () => {
    setReloading(true);
    Promise.resolve(refetchFromPageAndAfter(0))
      .then(() => {
        toast.success(t('common.refreshSuccess'));
      })
      .finally(() => setReloading(false));
  };

  const handleCreate = () => {
    if (!canCreate) return;
    if (isWorkflowList && canImportWorkflow) {
      setTemplateOpen(true);
      return;
    }
    if (canCreateBlank) {
      setOpen(true);
    }
  };

  const handleCreateBlank = () => {
    if (!canCreateBlank) return;
    setTemplateOpen(false);
    setOpen(true);
  };

  const handleImport = () => {
    if (!canImportWorkflow) return;
    setImportOpen(true);
  };

  if (!isPermissionsLoading && !canView) {
    return (
      <div className="flex h-full flex-col items-center justify-center p-4 text-center">
        <ShieldAlert className="mb-4 h-12 w-12 text-muted-foreground" />
        <h2 className="mb-2 text-xl font-semibold">{t('common.accessDenied')}</h2>
        <p className="max-w-md text-muted-foreground">{t('common.unauthorizedDescription')}</p>
      </div>
    );
  }

  return (
    <>
      <AgentsAIChatContextRegistration
        assetKind={assetKind}
        agents={agents}
        pageSize={PAGE_SIZE}
        searchKeyword={debouncedSearchKeyword}
        pageTitle={title}
        workspaceId={currentWorkspace?.id}
        workspaceName={currentWorkspace?.name}
        canView={canView}
        canManage={isWorkflowList ? false : canManageAgent}
        isLoading={isLoading}
        isFetching={isFetching}
        permissionsSettled={!isPermissionsLoading}
        hasNextPage={Boolean(hasNextPage)}
      />
      <div
        ref={listScrollRef}
        onScroll={handleListScroll}
        className="flex h-full flex-col space-y-6 overflow-y-auto p-4 @md/console:space-y-8 @md/console:p-6 @5xl/console:space-y-9 @5xl/console:p-8"
      >
        <div className="flex flex-col justify-between gap-4 @3xl/console:flex-row @3xl/console:items-center">
          <div className="flex items-center gap-2">
            <h1 className="text-xl font-semibold sm:text-2xl">{title}</h1>
            <Button
              isIcon
              variant="ghost"
              className="size-7 cursor-pointer rounded-sm hover:bg-muted"
              onClick={handleReload}
              disabled={isFetching || reloading}
            >
              <RefreshCw
                size={16}
                className={`${isFetching || reloading ? 'animate-spin' : ''} h-4 w-4`}
              />
            </Button>
          </div>

          <div className="flex w-full flex-col gap-3 @3xl/console:w-auto @3xl/console:flex-row @3xl/console:flex-wrap @3xl/console:justify-end">
            <Select
              value={publishFilter}
              onValueChange={value => handlePublishFilterChange(value as AgentListPublishFilter)}
            >
              <SelectTrigger
                className="w-full bg-background @3xl/console:w-36"
                aria-label={t('agents.listFilters.publishStatus')}
              >
                <SelectValue placeholder={t('agents.listFilters.publishStatus')} />
              </SelectTrigger>
              <SelectContent>
                <SelectItem value="all">{t('agents.listFilters.allPublishStatuses')}</SelectItem>
                <SelectItem value="published">{t('agents.status.published')}</SelectItem>
                <SelectItem value="draft">{t('agents.status.draft')}</SelectItem>
                <SelectItem value="offline">{t('agents.status.webappOffline')}</SelectItem>
              </SelectContent>
            </Select>
            {isWorkflowList && (
              <Select
                value={workflowTypeFilter}
                onValueChange={value =>
                  handleWorkflowTypeFilterChange(value as AgentListWorkflowTypeFilter)
                }
              >
                <SelectTrigger
                  className="w-full bg-background @3xl/console:w-40"
                  aria-label={t('agents.listFilters.workflowType')}
                >
                  <SelectValue placeholder={t('agents.listFilters.workflowType')} />
                </SelectTrigger>
                <SelectContent>
                  <SelectItem value="all">{t('agents.listFilters.allWorkflowTypes')}</SelectItem>
                  <SelectItem value={AgentType.CONVERSATIONAL_AGENT}>
                    {t('agents.modes.chatWorkflow')}
                  </SelectItem>
                  <SelectItem value={AgentType.WORKFLOW}>
                    {t('agents.modes.taskWorkflow')}
                  </SelectItem>
                </SelectContent>
              </Select>
            )}
            <div className="relative w-full @3xl/console:w-64">
              <Search className="absolute left-3 top-1/2 h-4 w-4 -translate-y-1/2 rounded-lg bg-background text-sm text-muted-foreground" />
              <Input
                placeholder={searchPlaceholder}
                value={searchKeyword}
                onChange={event => handleSearchChange(event.target.value)}
                className="w-full rounded-lg bg-background pl-9 text-sm"
              />
            </div>
            {(canImportWorkflow || canCreate) && (
              <>
                {canImportWorkflow && (
                  <Button
                    variant="outline"
                    onClick={handleImport}
                    className="w-full @3xl/console:w-auto"
                  >
                    <Upload className="h-4 w-4" />
                    <span className="text-sm">{importLabel}</span>
                  </Button>
                )}
                {canCreate && (
                  <Button onClick={handleCreate} className="w-full @3xl/console:w-auto">
                    <Plus className="h-4 w-4" />
                    <span className="text-sm">{createLabel}</span>
                  </Button>
                )}
              </>
            )}
          </div>
        </div>

        {isLoading && (
          <div className="grid grid-cols-[repeat(auto-fill,minmax(15rem,1fr))] gap-4">
            {Array.from({ length: 20 }).map((_, index) => (
              <Skeleton key={index} className="h-40 w-full" />
            ))}
          </div>
        )}

        {!isLoading &&
          agents.length === 0 &&
          (hasActiveCriteria ? (
            isWorkflowList || hasActiveFilters ? (
              <AgentEmptyElement
                type="search"
                title={
                  effectiveSearchKeyword && isWorkflowList
                    ? t('agents.workflowNoResultsDescription', {
                        keyword: effectiveSearchKeyword,
                      })
                    : t('agents.noResults')
                }
                description={t('agents.noResults')}
                actions={[
                  {
                    label: hasActiveFilters
                      ? t('agents.listFilters.clear')
                      : t('agents.clearSearch'),
                    onClick: handleClearCriteria,
                    variant: 'outline' as const,
                  },
                ]}
              />
            ) : (
              <AgentEmptySearchResults
                query={effectiveSearchKeyword}
                onClearSearch={handleClearCriteria}
              />
            )
          ) : (
            <AgentEmptyElement
              title={emptyTitle}
              description={emptyDescription}
              actions={[
                ...(canImportWorkflow
                  ? [
                      {
                        label: importLabel,
                        icon: <Upload className="h-4 w-4" />,
                        onClick: handleImport,
                        variant: 'outline' as const,
                      },
                    ]
                  : []),
                ...(canCreate
                  ? [
                      {
                        label: createFirstLabel,
                        icon: <Plus className="h-4 w-4" />,
                        onClick: handleCreate,
                      },
                    ]
                  : []),
              ]}
            />
          ))}

        <div className="grid grid-cols-[repeat(auto-fill,minmax(15rem,1fr))] gap-4">
          {pages.map((list, pageIndex) =>
            list.map(agent => (
              <AgentCard
                key={agent.id}
                agent={agent}
                pageIndex={pageIndex}
                onNavigate={() => markAgentListDetailEntry(agent.id, listScope)}
                onDeleted={(deletedId, deletedPageIndex) => {
                  queryClient.setQueriesData<InfiniteData<ApiResponseData<AgentList>>>(
                    { queryKey: ['agents', 'list'] },
                    old => {
                      if (!old) return old;
                      const nextPages = old.pages.map((page, index) => {
                        if (index < deletedPageIndex) return page;
                        const listData = page.data?.data ?? [];
                        const filtered = listData.filter(item => item.id !== deletedId);
                        return {
                          ...page,
                          data: page.data ? { ...page.data, data: filtered } : page.data,
                        };
                      });
                      return { ...old, pages: nextPages };
                    }
                  );
                  void refetchFromPageAndAfter(deletedPageIndex);
                }}
              />
            ))
          )}
        </div>

        <div ref={loadMoreRef} className="h-10" />

        {isFetchingNextPage && (
          <div className="flex justify-center py-4">
            <Loader2 className="h-4 w-4 animate-spin text-muted-foreground" />
          </div>
        )}
      </div>

      {isWorkflowList && (
        <ImportAgentDialog
          open={importOpen}
          workspaceId={currentWorkspace?.id}
          onOpenChange={setImportOpen}
          onImportComplete={async () => {
            await refetchFromPageAndAfter(0);
          }}
        />
      )}
      {isWorkflowList && (
        <TemplateGalleryDialog
          open={templateOpen}
          workspaceId={currentWorkspace?.id}
          canCreateBlank={canCreateBlank}
          onOpenChange={setTemplateOpen}
          onCreateBlank={handleCreateBlank}
          initialTemplateId={templateFromQuery}
          onTemplateCreated={async () => {
            await refetchFromPageAndAfter(0);
          }}
        />
      )}
      <AgentDialog
        open={open}
        mode="create"
        onOpenChange={setOpen}
        allowedAgentTypes={dialogAgentTypes}
        defaultAgentType={dialogAgentTypes[0] ?? AgentType.AGENT}
        hideTypeSelector={!isWorkflowList}
      />
    </>
  );
}

export default AgentAssetListPage;
