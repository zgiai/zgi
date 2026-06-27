'use client';

import { Suspense, useRef, useState, useMemo, useEffect } from 'react';
import { BookOpen, FolderPlus, Folders, Loader2, Plus, Search, ShieldAlert } from 'lucide-react';
import { Button } from '@/components/ui/button';

import { useT } from '@/i18n';
import { toast } from 'sonner';
import DatasetCard from '@/components/datasets/dataset-card';
import { useQueryClient } from '@tanstack/react-query';
import type { Dataset } from '@/services/types/dataset';
import CreateDatasetDialog from '@/components/datasets/dialog/create-dataset-dialog';
import EditDatasetDialog from '@/components/datasets/dialog/edit-dataset-dialog';
import { useDebouncedValue } from '@/hooks/use-debounced-value';
import FolderCard from '@/components/datasets/folder-card';
import { useSearchParams, useRouter } from 'next/navigation';
import {
  useDatasetFolders,
  useFolderAncestors,
  useFolderDatasetsInfinite,
} from '@/hooks/dataset/use-dataset-folders';
import FolderModal from '@/components/datasets/modal/folder-modal';
import type { DatasetFolder } from '@/services/types/dataset-folder';
// Add EventBus imports for centralized modal control
import { useEventBus } from '@/hooks/use-event-bus';
import { eventBus } from '@/lib/event-bus';
import { useDatasetDeletionOptimistic } from '@/hooks/dataset/use-dataset-deletion-optimistic';
import { HeaderToolbar } from '@/components/datasets/page/header-toolbar';
import { DatasetBreadcrumbs } from '@/components/datasets/page/breadcrumbs';
import { SkeletonGrid } from '@/components/datasets/page/skeleton-grid';
import { VirtualContentGrid } from '@/components/datasets/page/virtual-content-grid';
import type { OpenDatasetDialogPayload } from '@/components/datasets/dialog/types';
import type { OpenFolderModalPayload } from '@/components/datasets/modal/folder-modal';
import { useInfiniteObserver } from '@/hooks/use-infinite-observer';
import { useAvailableModels } from '@/hooks/model/use-model';
import { useIsInitialized } from '@/store/auth-store';
import { useAccountPermissions } from '@/hooks/organization/use-account-permissions';
import { useCurrentWorkspace } from '@/store/workspace-store';
import { cn } from '@/lib/utils';
import { KNOWLEDGE_BASE_VISIBLE_PERMISSION_CODES } from '@/constants/permissions';

function DatasetModelsPreloader() {
  useAvailableModels({ use_case: 'text-chat' });
  useAvailableModels({ use_case: 'embedding' });
  useAvailableModels({ use_case: 'rerank' });
  return null;
}

function DatasetsPageContent() {
  const t = useT();
  const currentWorkspace = useCurrentWorkspace();
  const PAGE_SIZE = 20;
  const queryClient = useQueryClient();

  // Permission checking
  const { hasPermission, hasAnyPermission, isLoading: isPermissionsLoading } =
    useAccountPermissions();
  const canView = hasAnyPermission(KNOWLEDGE_BASE_VISIBLE_PERMISSION_CODES);
  const canManage = hasPermission('knowledge_base.create');
  const canManageFolders = hasPermission('knowledge_base.folder_manage');

  // Replace local create-only dialog with centralized dialog state
  // const [open, setOpen] = useState(false);
  const [datasetDialogOpen, setDatasetDialogOpen] = useState(false);
  const [datasetDialogMode, setDatasetDialogMode] = useState<'create' | 'edit'>('create');
  const [selectedDataset, setSelectedDataset] = useState<Dataset | undefined>(undefined);
  const [datasetDialogFolderId, setDatasetDialogFolderId] = useState<string | undefined>(undefined);
  // Centralized folder modal state
  const [folderModalOpen, setFolderModalOpen] = useState(false);
  const [folderModalMode, setFolderModalMode] = useState<'create' | 'edit'>('create');
  const [selectedFolder, setSelectedFolder] = useState<DatasetFolder | undefined>(undefined);
  const [parentFolderId, setParentFolderId] = useState<string | undefined>(undefined);
  const [searchKeyword, setSearchKeyword] = useState('');
  const debouncedSearchKeyword = useDebouncedValue(searchKeyword, 500);
  const [reloading, setReloading] = useState(false);
  const searchParams = useSearchParams();
  const router = useRouter();
  const activeFolderId = searchParams.get('folder') || undefined;
  const isRootView = !activeFolderId;

  // Track navigation state to prevent accidental clicks (debounce)
  const [isNavigating, setIsNavigating] = useState(false);

  // When folder changes, set navigating state briefly to prevent accidental clicks
  useEffect(() => {
    setIsNavigating(true);
    const timer = setTimeout(() => setIsNavigating(false), 500);
    return () => clearTimeout(timer);
  }, [activeFolderId]);

  // Navigate back to root view
  const handleBack = () => {
    router.push('/console/dataset');
  };

  // Subscribe to dataset dialog open events from cards or page triggers
  useEventBus<OpenDatasetDialogPayload>('dataset:open-dialog', payload => {
    setDatasetDialogMode(payload.mode);
    setSelectedDataset(payload.dataset);
    setDatasetDialogFolderId(payload.currentFolderId);
    setDatasetDialogOpen(true);
  });

  // Centralized folder modal open events
  useEventBus<OpenFolderModalPayload>('folder:open-modal', payload => {
    setFolderModalMode(payload.mode);
    setSelectedFolder(payload.folder);
    setParentFolderId(payload.parentFolderId);
    setFolderModalOpen(true);
  });

  // Root view: all folders + infinite datasets; Subfolder view: folder contents (folders + datasets)
  const {
    data: allFolders,
    isLoading: isFoldersLoading,
    refetch: refetchFolders,
  } = useDatasetFolders({
    enabled: isRootView,
    keyword: isRootView ? debouncedSearchKeyword : undefined,
    workspace_id: currentWorkspace?.id,
  });
  const rootFolders = useMemo(
    () => (isRootView ? (allFolders || []).filter(f => !f.parent_id) : []),
    [allFolders, isRootView]
  );

  const {
    pages,
    fetchNextPage,
    hasNextPage,
    isFetchingNextPage,
    isLoading: isDatasetsLoading,
    isFetching,
    refetchFromPageAndAfter,
  } = useFolderDatasetsInfinite(activeFolderId || undefined, PAGE_SIZE, {
    enabled: isRootView ? !isFoldersLoading : true,
    keyword: debouncedSearchKeyword,
    workspace_id: currentWorkspace?.id,
  });

  // Optimistic deletion handlers for root view and subfolder view
  const handleDeletedRoot = useDatasetDeletionOptimistic({
    queryClient,
    pageSize: PAGE_SIZE,
    keyword: debouncedSearchKeyword,
    refetchFromPageAndAfter,
    activeFolderId: undefined,
  });
  const handleDeletedSubfolder = useDatasetDeletionOptimistic({
    queryClient,
    pageSize: PAGE_SIZE,
    keyword: debouncedSearchKeyword,
    refetchFromPageAndAfter,
    activeFolderId: activeFolderId || undefined,
  });
  const allDatasetsRoot = (pages || []).flat();
  // Prepare dataset entries with pageIndex for unified grid rendering
  const datasetEntries = useMemo(
    () => (pages || []).flatMap((list, pIndex) => list.map(ds => ({ ds, pIndex }))),
    [pages]
  );

  // loadMoreRef is managed by useInfiniteObserver hook below
  const scrollRef = useRef<HTMLDivElement | null>(null);
  // Infinite scroll observer (root and subfolder views)

  const loadMoreRef = useInfiniteObserver({
    hasNextPage,
    isFetchingNextPage,
    fetchNextPage,
    rootMargin: '0px 0px 1000px 0px',
    rootRef: scrollRef,
  });
  // Reload resets list and page (root) or refetch folder contents (subfolder)
  const handleReload = () => {
    setReloading(true);
    if (isRootView) {
      Promise.all([refetchFromPageAndAfter(0), refetchFolders()])
        .then(() => {
          toast.success(t('common.refreshSuccess'));
        })
        .finally(() => setReloading(false));
    } else {
      Promise.resolve(refetchFromPageAndAfter(0))
        .then(() => {
          toast.success(t('common.refreshSuccess'));
        })
        .finally(() => setReloading(false));
    }
  };

  const handleCreateFolder = () => {
    // Publish centralized FolderModal open event
    eventBus.publish<OpenFolderModalPayload>('folder:open-modal', {
      mode: 'create',
      parentFolderId: isRootView ? undefined : activeFolderId || undefined,
    });
  };

  const handleCreate = () => {
    // Publish event to open centralized dialog in create mode
    eventBus.publish<OpenDatasetDialogPayload>('dataset:open-dialog', {
      mode: 'create',
      currentFolderId: isRootView ? undefined : activeFolderId || undefined,
    });
  };

  // Breadcrumbs (subfolder view)
  const { data: ancestors = [] } = useFolderAncestors(activeFolderId);

  // Derived states for skeletons
  const showFolderSkeletons = isRootView ? isFoldersLoading : false;
  const showDatasetSkeletons = isDatasetsLoading;

  // Virtualization decision is based on dataset entries only
  const enableVirtual = datasetEntries.length > 200;
  const rowHeight = 160; // Tailwind h-40
  const isAuthReady = useIsInitialized();

  // Access denied state
  if (!isPermissionsLoading && !canView) {
    return (
      <div className="flex flex-col items-center justify-center h-full gap-4 text-center p-8">
        <div className="w-16 h-16 rounded-full bg-muted flex items-center justify-center">
          <ShieldAlert className="w-8 h-8 text-muted-foreground" />
        </div>
        <div className="space-y-2">
          <h2 className="text-lg font-semibold text-foreground">{t('common.accessDenied')}</h2>
          <p className="text-sm text-muted-foreground max-w-md">
            {t('common.unauthorizedDescription')}
          </p>
        </div>
      </div>
    );
  }

  return (
    <>
      {isAuthReady && <DatasetModelsPreloader />}
      <div ref={scrollRef} className="p-8 space-y-6 flex flex-col h-full overflow-y-auto">
        {/* Header */}
        <HeaderToolbar
          titleText={t('datasets.title')}
          isFetching={isFetching || reloading}
          onReload={handleReload}
          isRootView={isRootView}
          searchKeyword={searchKeyword}
          onSearchChange={setSearchKeyword}
          searchPlaceholder={t('datasets.search.placeholder')}
          createFolderText={t('datasets.createFolder')}
          onCreateFolder={isRootView && canManageFolders ? handleCreateFolder : undefined}
          createText={t('datasets.create')}
          onCreateDataset={canManage ? handleCreate : undefined}
          onBack={handleBack}
        />

        {/* Breadcrumbs for subfolder view */}
        {!isRootView && (
          <DatasetBreadcrumbs ancestors={ancestors} titleText={t('datasets.title')} />
        )}

        {/* Split content: folders section then datasets section */}
        <section className={cn('space-y-6', isNavigating && 'pointer-events-none')}>
          {/* Folders Section (root view only) */}
          {isRootView && (showFolderSkeletons || rootFolders.length > 0) && (
            <div className="space-y-3">
              <h2 className="text-lg font-semibold flex items-center gap-2">
                <Folders className="h-5 w-5" /> {t('datasets.folder')}
              </h2>
              <SkeletonGrid
                showFolderSkeletons={showFolderSkeletons}
                showDatasetSkeletons={false}
                isRootView
                folderSkeletonCount={20}
              />
              {!showFolderSkeletons && (
                <div className="grid grid-cols-1 sm:grid-cols-2 md:grid-cols-3 lg:grid-cols-4 2xl:grid-cols-5 gap-3 sm:gap-4 md:gap-6 lg:gap-8 xl:gap-10 2xl:gap-12">
                  {rootFolders.map(folder => (
                    <FolderCard key={folder.id} folder={folder} />
                  ))}
                </div>
              )}
            </div>
          )}

          {/* Datasets Section - hide header when both folders and datasets are empty (page-level empty state will show) */}
          <div className="space-y-3">
            {isRootView &&
              (rootFolders.length > 0 || datasetEntries.length > 0 || showDatasetSkeletons) && (
                <h2 className="text-lg font-semibold flex items-center gap-2">
                  <BookOpen className="h-5 w-5" /> {t('datasets.dataset')}
                </h2>
              )}
            <SkeletonGrid
              showFolderSkeletons={false}
              showDatasetSkeletons={showDatasetSkeletons}
              isRootView={isRootView}
              datasetSkeletonCount={20}
            />

            {!showDatasetSkeletons && (
              <>
                {datasetEntries.length > 0 ? (
                  <>
                    {enableVirtual ? (
                      <VirtualContentGrid
                        items={datasetEntries}
                        itemKey={item => item.ds.id}
                        renderItem={({ ds, pIndex }) => (
                          <DatasetCard
                            dataset={ds}
                            pageIndex={pIndex}
                            onDeleted={(deletedId, pageIndex) => {
                              (isRootView ? handleDeletedRoot : handleDeletedSubfolder)(
                                deletedId,
                                pageIndex
                              );
                            }}
                            currentFolderId={isRootView ? undefined : activeFolderId || ''}
                          />
                        )}
                        rowHeight={rowHeight}
                        scrollElementRef={scrollRef}
                        columnGap={16}
                        rowGap={16}
                        overscan={6}
                        onScrollEnd={() => {
                          if (hasNextPage && !isFetchingNextPage) {
                            void fetchNextPage();
                          }
                        }}
                      />
                    ) : (
                      <div className="grid grid-cols-1 sm:grid-cols-2 md:grid-cols-3 lg:grid-cols-4 2xl:grid-cols-5 gap-3 sm:gap-4 md:gap-6 lg:gap-8 xl:gap-10 2xl:gap-12">
                        {datasetEntries.map(({ ds, pIndex }) => (
                          <DatasetCard
                            key={ds.id}
                            dataset={ds}
                            pageIndex={pIndex}
                            onDeleted={(deletedId, pageIndex) => {
                              (isRootView ? handleDeletedRoot : handleDeletedSubfolder)(
                                deletedId,
                                pageIndex
                              );
                            }}
                            currentFolderId={isRootView ? undefined : activeFolderId || ''}
                          />
                        ))}
                      </div>
                    )}
                    {hasNextPage && <div ref={loadMoreRef} className="h-1" />}
                  </>
                ) : // Dataset section empty state (only when folders exist but no datasets)
                // Don't show if page-level empty state will be shown (both folders and datasets are empty)
                // Don't show in organization mode (PersonalSpaceEmptyState handles it)
                isRootView && rootFolders.length > 0 ? (
                  <div className="flex flex-col items-center justify-center py-12 text-center">
                    <BookOpen className="h-12 w-12 text-muted-foreground mb-4" />
                    <h3 className="text-lg font-medium mb-2">{t('datasets.empty.noDatasets')}</h3>
                    {canManage && (
                      <Button onClick={handleCreate}>
                        <Plus size={16} />
                        {t('datasets.create')}
                      </Button>
                    )}
                  </div>
                ) : null}
              </>
            )}
          </div>

          {/* Loading footer and sentinel (for datasets infinite scroll) */}
          <div className="flex justify-center items-center py-4">
            {isFetchingNextPage && (
              <div className="flex items-center gap-2 text-sm text-muted-foreground">
                <Loader2 className="h-4 w-4 animate-spin" />
                {t('datasets.loading')}
              </div>
            )}
          </div>
          {/* Sentinel moved above to trigger earlier than visual spinner */}

          {/* Empty state: when both folders and datasets are empty and not loading */}
          {!showFolderSkeletons &&
            !showDatasetSkeletons &&
            (isRootView
              ? rootFolders.length + allDatasetsRoot.length === 0
              : (datasetEntries.length || 0) === 0) && (
              <div className="flex flex-col items-center justify-center py-12 text-center">
                {debouncedSearchKeyword ? (
                  <>
                    <Search className="h-12 w-12 text-muted-foreground mb-4" />
                    <h3 className="text-lg font-medium mb-2">{t('datasets.empty.noResults')}</h3>
                    <p className="text-muted-foreground mb-2 max-w-sm">
                      {t('datasets.empty.noResultsFor', { query: debouncedSearchKeyword })}
                    </p>
                    <Button onClick={() => setSearchKeyword('')}>
                      {t('datasets.messages.clearFilters')}
                    </Button>
                  </>
                ) : (
                  <>
                    <Search className="h-12 w-12 text-muted-foreground mb-4" />
                    <h3 className="text-lg font-medium mb-2">{t('datasets.empty.empty')}</h3>
                    <div className="flex gap-2">
                      {isRootView && canManageFolders && (
                        <Button variant="outline" onClick={handleCreateFolder}>
                          <FolderPlus size={16} />
                          {t('datasets.createFolder')}
                        </Button>
                      )}
                      {canManage && (
                        <Button onClick={handleCreate}>
                          <Plus size={16} />
                          {t('datasets.create')}
                        </Button>
                      )}
                    </div>
                  </>
                )}
              </div>
            )}
        </section>
      </div>

      <FolderModal
        open={folderModalOpen}
        onOpenChange={setFolderModalOpen}
        mode={folderModalMode}
        folder={selectedFolder}
        parentFolderId={parentFolderId}
      />
      <CreateDatasetDialog
        open={datasetDialogOpen && datasetDialogMode === 'create'}
        onOpenChange={setDatasetDialogOpen}
        currentFolderId={datasetDialogFolderId}
      />
      <EditDatasetDialog
        open={datasetDialogOpen && datasetDialogMode === 'edit'}
        onOpenChange={setDatasetDialogOpen}
        dataset={selectedDataset}
      />
    </>
  );
}

export default function DatasetsPage() {
  return (
    <Suspense fallback={null}>
      <DatasetsPageContent />
    </Suspense>
  );
}
