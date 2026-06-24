'use client';

import { useEffect, useMemo, useRef, useState } from 'react';
import { useSearchParams } from 'next/navigation';
import { Plus, RefreshCw, Loader2, Search, Upload } from 'lucide-react';
import { Button } from '@/components/ui/button';
import { Input } from '@/components/ui/input';
import { Skeleton } from '@/components/ui/skeleton';
import { useAgents } from '@/hooks/agent/use-agents';
import { useT } from '@/i18n';
import { toast } from 'sonner';
import AgentCard from '@/components/agents/agent-card';
import { useQueryClient, type InfiniteData } from '@tanstack/react-query';
import type { ApiResponseData } from '@/services/types/common';
import type { AgentList } from '@/services/types/agent';
import { AGENT_KEYS } from '@/hooks/query-keys';
import AgentDialog from '@/components/agents/agent-dialog';
import { useDebouncedValue } from '@/hooks/use-debounced-value';
import ImportAgentDialog from '@/components/agents/import-agent-dialog';
import { TemplateGalleryDialog } from '@/components/agents/templates';
import {
  consumeAgentListRestoreIntent,
  markAgentListDetailEntry,
  readAgentListInitialKeyword,
  readAgentListNavigationState,
  writeAgentListNavigationState,
  type AgentListNavigationState,
} from '@/utils/agent-list-state';

import { useAccountPermissions } from '@/hooks/organization/use-account-permissions';
import { useCurrentWorkspace } from '@/store/workspace-store';
import { ShieldAlert } from 'lucide-react';
import { AgentEmptyElement, AgentEmptySearchResults } from '@/components/agents/empty-element';
import { AgentsAIChatContextRegistration } from '@/components/agents/aichat-context';

const PAGE_SIZE = 20;

export default function AgentsPage() {
  const t = useT();
  const searchParams = useSearchParams();
  const currentWorkspace = useCurrentWorkspace();

  // Permissions
  const { hasPermission, isLoading: isPermissionsLoading } = useAccountPermissions();
  const canView = hasPermission('agent.view');
  const canManage = hasPermission('agent.manage');

  const queryClient = useQueryClient();
  const [open, setOpen] = useState(false);
  const [templateOpen, setTemplateOpen] = useState(false);
  const [importOpen, setImportOpen] = useState(false);
  const [searchKeyword, setSearchKeyword] = useState(readAgentListInitialKeyword);
  const [queryKeywordOverride, setQueryKeywordOverride] = useState<string | null>(null);
  const [isRestoreChecked, setIsRestoreChecked] = useState(false);
  const [reloading, setReloading] = useState(false);
  const listScrollRef = useRef<HTMLDivElement>(null);
  const pendingRestoreRef = useRef<AgentListNavigationState | null>(null);
  const hasRestoredScrollRef = useRef(false);
  const hasRefreshedRestoredPagesRef = useRef(false);
  const scrollSaveFrameRef = useRef<number | null>(null);
  const debouncedSearchKeyword = useDebouncedValue(searchKeyword, 500);
  const effectiveSearchKeyword = queryKeywordOverride ?? debouncedSearchKeyword;
  const templateFromQuery = searchParams.get('template');
  const agentListParams = useMemo(
    () => ({
      limit: PAGE_SIZE,
      keyword: effectiveSearchKeyword || undefined,
      workspace_id: currentWorkspace?.id,
    }),
    [effectiveSearchKeyword, currentWorkspace?.id]
  );
  const agentListQueryKey = useMemo(() => AGENT_KEYS.list(agentListParams), [agentListParams]);

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

    const shouldRestore = consumeAgentListRestoreIntent();
    const savedState = shouldRestore ? readAgentListNavigationState() : null;
    if (!savedState || savedState.workspaceId !== currentWorkspace.id) {
      pendingRestoreRef.current = null;
      hasRestoredScrollRef.current = false;
      hasRefreshedRestoredPagesRef.current = false;
      setSearchKeyword('');
      setQueryKeywordOverride('');
      setIsRestoreChecked(true);
      return;
    }

    setSearchKeyword(savedState.keyword);
    setQueryKeywordOverride(savedState.keyword);
    pendingRestoreRef.current = savedState;

    const restoredParams = {
      limit: PAGE_SIZE,
      keyword: savedState.keyword || undefined,
      workspace_id: currentWorkspace.id,
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
  }, [currentWorkspace?.id, queryClient]);

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

  // Flatten all pages into a single array
  const agents = pages.flat();

  // Infinite scroll trigger
  const loadMoreRef = useRef<HTMLDivElement>(null);

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
        loadedPageCount: pages.length,
        scrollTop: nextScrollTop ?? listScrollRef.current?.scrollTop ?? 0,
        workspaceId: currentWorkspace.id,
        pages: cached?.pages,
        updatedAt: Date.now(),
      },
      { includePages }
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
    // React Query updates cached pages after this render, so run once more on the next frame.
    const frame = window.requestAnimationFrame(() => persistNavigationState(undefined, true));
    return () => window.cancelAnimationFrame(frame);
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [agentListQueryKey, currentWorkspace?.id, effectiveSearchKeyword, isRestoreChecked, pages]);

  useEffect(() => {
    const pendingRestore = pendingRestoreRef.current;
    if (!pendingRestore || !isRestoreChecked || !canView) return;
    if (pendingRestore.keyword !== effectiveSearchKeyword) return;
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
    pages.length,
  ]);

  useEffect(() => {
    const pendingRestore = pendingRestoreRef.current;
    if (!pendingRestore || hasRestoredScrollRef.current || !isRestoreChecked) return;
    if (pendingRestore.keyword !== effectiveSearchKeyword) return;
    if (pages.length < pendingRestore.loadedPageCount && hasNextPage) return;

    const frame = window.requestAnimationFrame(() => {
      const scrollContainer = listScrollRef.current;
      if (!scrollContainer) return;

      scrollContainer.scrollTop = pendingRestore.scrollTop;
      hasRestoredScrollRef.current = true;
    });

    return () => window.cancelAnimationFrame(frame);
  }, [effectiveSearchKeyword, hasNextPage, isRestoreChecked, pages.length]);

  useEffect(() => {
    const pendingRestore = pendingRestoreRef.current;
    if (!pendingRestore || hasRefreshedRestoredPagesRef.current || !isRestoreChecked) return;
    if (pendingRestore.keyword !== effectiveSearchKeyword) return;
    if (pages.length === 0) return;

    hasRefreshedRestoredPagesRef.current = true;
    void refetchFromPageAndAfter(0);
  }, [effectiveSearchKeyword, isRestoreChecked, pages.length, refetchFromPageAndAfter]);

  const handleSearchChange = (value: string) => {
    pendingRestoreRef.current = null;
    hasRestoredScrollRef.current = false;
    hasRefreshedRestoredPagesRef.current = false;
    if (listScrollRef.current) {
      listScrollRef.current.scrollTop = 0;
    }
    setQueryKeywordOverride(null);
    setSearchKeyword(value);
  };

  // Auto-fetch next page when load more trigger comes into view
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

  // Reload resets list and page
  const handleReload = () => {
    setReloading(true);
    Promise.resolve(refetchFromPageAndAfter(0))
      .then(() => {
        toast.success(t('common.refreshSuccess'));
      })
      .finally(() => setReloading(false));
  };

  const handleCreate = () => {
    if (!canManage) return;
    setTemplateOpen(true);
  };

  const handleCreateBlank = () => {
    if (!canManage) return;
    setTemplateOpen(false);
    setOpen(true);
  };

  const handleImport = () => {
    if (!canManage) return;
    setImportOpen(true);
  };

  // Access Denied State
  if (!isPermissionsLoading && !canView) {
    return (
      <div className="flex flex-col items-center justify-center h-full p-4 text-center">
        <ShieldAlert className="h-12 w-12 text-muted-foreground mb-4" />
        <h2 className="text-xl font-semibold mb-2">{t('common.accessDenied')}</h2>
        <p className="text-muted-foreground max-w-md">{t('common.unauthorizedDescription')}</p>
      </div>
    );
  }

  return (
    <>
      <AgentsAIChatContextRegistration
        agents={agents}
        pageSize={PAGE_SIZE}
        searchKeyword={debouncedSearchKeyword}
        workspaceId={currentWorkspace?.id}
        workspaceName={currentWorkspace?.name}
        canView={canView}
        canManage={canManage}
        isLoading={isLoading}
        hasNextPage={hasNextPage}
      />
      <div
        ref={listScrollRef}
        onScroll={handleListScroll}
        className="p-4 sm:p-6 lg:p-8 space-y-6 sm:space-y-8 lg:space-y-9 flex flex-col h-full overflow-y-auto"
      >
        {/* Header */}
        <div className="flex flex-col sm:flex-row sm:items-center justify-between gap-4">
          <div className="flex items-center gap-2">
            <h1 className="text-xl sm:text-2xl font-semibold">{t('agents.title')}</h1>
            <Button
              isIcon
              variant="ghost"
              className="size-7 rounded-sm hover:bg-muted cursor-pointer"
              onClick={handleReload}
              disabled={isFetching || reloading}
            >
              <RefreshCw
                size={16}
                className={`${isFetching || reloading ? 'animate-spin' : ''} h-4 w-4`}
              />
            </Button>
          </div>

          <div className="flex flex-col sm:flex-row gap-3 w-full sm:w-auto">
            {/* Search Bar */}
            <div className="relative w-full sm:max-w-md">
              <Search className="absolute left-3 top-1/2 h-4 w-4 -translate-y-1/2 text-muted-foreground bg-background rounded-lg text-sm" />
              <Input
                placeholder={t('agents.searchPlaceholder')}
                value={searchKeyword}
                onChange={e => handleSearchChange(e.target.value)}
                className="pl-9 bg-background rounded-lg text-sm w-full"
              />
            </div>
            {/* <Button onClick={handleCreateFolder}>
              <FolderPlus />
              {t('createFolder')}
            </Button> */}
            {canManage && (
              <>
                <Button variant="outline" onClick={handleImport} className="w-full sm:w-auto">
                  <Upload className="h-4 w-4" />
                  <span className="text-sm">{t('agents.importAgent')}</span>
                </Button>
                <Button onClick={handleCreate} className="w-full sm:w-auto">
                  <Plus className="h-4 w-4" />
                  <span className="text-sm">{t('agents.create')}</span>
                </Button>
              </>
            )}
          </div>
        </div>

        {/* List */}
        {isLoading && (
          <div className="grid grid-cols-1 sm:grid-cols-2 md:grid-cols-3 lg:grid-cols-4 2xl:grid-cols-5 gap-3 sm:gap-4 md:gap-6 lg:gap-8 xl:gap-10 2xl:gap-12">
            {Array.from({ length: 20 }).map((_, idx) => (
              <Skeleton key={idx} className="h-40 w-full" />
            ))}
          </div>
        )}

        {!isLoading &&
          agents.length === 0 &&
          (effectiveSearchKeyword ? (
            <AgentEmptySearchResults
              query={effectiveSearchKeyword}
              onClearSearch={() => handleSearchChange('')}
            />
          ) : (
            <AgentEmptyElement
              actions={
                canManage
                  ? [
                      {
                        label: t('agents.importAgent'),
                        icon: <Upload className="h-4 w-4" />,
                        onClick: handleImport,
                        variant: 'outline',
                      },
                      {
                        label: t('agents.createFirstAgent'),
                        icon: <Plus className="h-4 w-4" />,
                        onClick: handleCreate,
                      },
                    ]
                  : []
              }
            />
          ))}

        <div className="grid grid-cols-1 sm:grid-cols-2 md:grid-cols-3 lg:grid-cols-4 2xl:grid-cols-5 gap-3 md:gap-4 lg:gap-6 xl:gap-8 2xl:gap-10">
          {(pages || []).map((list, pIndex) =>
            list.map(agent => (
              <AgentCard
                key={agent.id}
                agent={agent}
                pageIndex={pIndex}
                onNavigate={() => markAgentListDetailEntry(agent.id)}
                onDeleted={(deletedId, pageIndex) => {
                  // Optimistically remove from cache for instantaneous UI update
                  queryClient.setQueriesData<InfiniteData<ApiResponseData<AgentList>>>(
                    { queryKey: ['agents', 'list'] },
                    old => {
                      if (!old) return old;
                      const nextPages = old.pages.map((page, idx) => {
                        if (idx < pageIndex) return page;
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
                  // Refetch from this page to ensure metadata (total/has_more) remains correct
                  void refetchFromPageAndAfter(pageIndex);
                }}
              />
            ))
          )}
        </div>

        {/* Infinite scroll sentinel */}
        <div ref={loadMoreRef} className="h-10" />

        {isFetchingNextPage && (
          <div className="flex justify-center py-4">
            <Loader2 className="h-4 w-4 animate-spin text-muted-foreground" />
          </div>
        )}
      </div>
      <ImportAgentDialog
        open={importOpen}
        workspaceId={currentWorkspace?.id}
        onOpenChange={setImportOpen}
        onImportComplete={async () => {
          await refetchFromPageAndAfter(0);
        }}
      />
      <TemplateGalleryDialog
        open={templateOpen}
        workspaceId={currentWorkspace?.id}
        onOpenChange={setTemplateOpen}
        onCreateBlank={handleCreateBlank}
        initialTemplateId={templateFromQuery}
        onTemplateCreated={async () => {
          await refetchFromPageAndAfter(0);
        }}
      />
      <AgentDialog open={open} mode="create" onOpenChange={setOpen} />
    </>
  );
}
