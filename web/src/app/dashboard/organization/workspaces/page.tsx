'use client';

import { Suspense, useState, useEffect, useCallback } from 'react';
import Link from 'next/link';
import { useT } from '@/i18n';
import { useRouter, usePathname, useSearchParams } from 'next/navigation';
import { toast } from 'sonner';
import { Input } from '@/components/ui/input';
import { Button } from '@/components/ui/button';
import { Skeleton } from '@/components/ui/skeleton';
import { Pagination } from '@/components/ui/pagination';
import { TableCell, TableRow } from '@/components/ui/table';
import { ConfirmDialog } from '@/components/ui/confirm-dialog';
import { Search, Plus, Users, ChevronRight, Pencil, Trash2 } from 'lucide-react';
import { useWorkspaces } from '@/hooks/workspace/use-workspaces';
import { useDeleteWorkspace } from '@/hooks/workspace/use-workspace-actions';
import { useDebouncedValue } from '@/hooks/use-debounced-value';
import { useWorkspaceActions } from '@/hooks/workspace/use-workspace-actions';
import { useOrganizations } from '@/hooks/organization/use-organizations';
import { WorkspaceDialog } from '@/components/dashboard/organization/workspace-dialog';
import { StickyDataTable } from '@/components/common/sticky-data-table';
import type {
  WorkspaceManagement,
  CreateWorkspaceRequest,
  UpdateWorkspaceRequest,
} from '@/services/types/workspace';
import { workspaceService as organizationService } from '@/services/workspace.service';
import { getErrorMessage } from '@/utils/error-notifications';
import { useWorkspaceStore } from '@/store/workspace-store';
import { Tooltip, TooltipContent, TooltipTrigger } from '@/components/ui/tooltip';

function WorkspaceManagementPageContent() {
  const t = useT('dashboard.organization.workspaceManagement');
  const router = useRouter();
  const searchParams = useSearchParams();
  const pathname = usePathname();
  const [searchKeyword, setSearchKeyword] = useState(searchParams.get('q') || '');
  const [currentPage, setCurrentPage] = useState(Number(searchParams.get('page')) || 1);
  const [isWorkspacePageChanging, setIsWorkspacePageChanging] = useState(false);
  const pageSize = 10;
  const [deleteConfirmOpen, setDeleteConfirmOpen] = useState(false);
  const [workspaceToDelete, setWorkspaceToDelete] = useState<WorkspaceManagement | null>(null);
  const assignMemberKeyword = searchParams.get('assignMember')?.trim() || '';
  const [workspaceDialog, setWorkspaceDialog] = useState<{
    open: boolean;
    mode: 'create' | 'edit';
    initialData: WorkspaceManagement | null;
  }>({
    open: false,
    mode: 'create',
    initialData: null,
  });

  // Get organization and mutations
  const { currentOrganization } = useOrganizations();
  const { createWorkspace, isCreating, updateWorkspace, isUpdating } = useWorkspaceActions();
  const currentWorkspace = useWorkspaceStore.use.currentWorkspace();
  const setCurrentWorkspace = useWorkspaceStore.use.setCurrentWorkspace();
  const workspacesFromStore = useWorkspaceStore.use.workspaces();
  const setWorkspaces = useWorkspaceStore.use.setWorkspaces();

  // Create URL update helper
  const createQueryString = useCallback(
    (params: Record<string, string | number | null>) => {
      const newParams = new URLSearchParams(searchParams.toString());
      Object.entries(params).forEach(([key, value]) => {
        if (value === null || value === '' || (key === 'page' && value === 1)) {
          newParams.delete(key);
        } else {
          newParams.set(key, String(value));
        }
      });
      return newParams.toString();
    },
    [searchParams]
  );

  // Debounce search keyword
  const debouncedSearchKeyword = useDebouncedValue(searchKeyword, 500);

  // Sync state to URL
  const updateUrl = useCallback(
    (params: Record<string, string | number | null>) => {
      const queryString = createQueryString(params);
      router.push(`${pathname}${queryString ? `?${queryString}` : ''}`, { scroll: false });
    },
    [createQueryString, pathname, router]
  );

  // Handle page change
  const handlePageChange = useCallback(
    (page: number) => {
      if (page === currentPage) return;
      setIsWorkspacePageChanging(true);
      setCurrentPage(page);
      updateUrl({ page });
    },
    [currentPage, updateUrl]
  );

  // Reset to first page when search changes
  useEffect(() => {
    // Only reset if the keyword changed or a legacy status query needs clearing.
    // This effect runs on mount too, so we need to be careful
    const currentQ = searchParams.get('q') || '';
    const hasLegacyStatus = searchParams.has('status');

    if (debouncedSearchKeyword !== currentQ || hasLegacyStatus) {
      setIsWorkspacePageChanging(true);
      setCurrentPage(1);
      updateUrl({
        q: debouncedSearchKeyword || null,
        status: null,
        page: 1,
      });
    }
  }, [debouncedSearchKeyword, updateUrl, searchParams]);

  // Fetch workspaces
  const {
    workspaces,
    total,
    hasMore,
    page: workspaceResponsePage,
    isLoading,
    isPlaceholderData: isPlaceholderWorkspaces,
  } = useWorkspaces(debouncedSearchKeyword, currentPage, pageSize, {
    keepPreviousData: true,
  });

  const loadedItemCount = (currentPage - 1) * pageSize + workspaces.length;
  const effectiveTotal = Math.max(total, loadedItemCount + (hasMore ? 1 : 0));
  const totalPages = Math.max(1, Math.ceil(effectiveTotal / pageSize));
  const shouldShowWorkspaceSkeleton = isLoading || isWorkspacePageChanging;

  // Handle case where current page > total pages (e.g. after deletion)
  useEffect(() => {
    const maxPage = Math.max(1, totalPages);
    if (currentPage > maxPage) {
      handlePageChange(maxPage);
    }
  }, [currentPage, totalPages, handlePageChange]);

  useEffect(() => {
    if (!isPlaceholderWorkspaces && workspaceResponsePage === currentPage) {
      setIsWorkspacePageChanging(false);
    }
  }, [currentPage, isPlaceholderWorkspaces, workspaceResponsePage]);

  // Delete workspace hook
  const { deleteWorkspace, isDeleting } = useDeleteWorkspace();

  // Handle delete workspace
  const handleDeleteWorkspace = async () => {
    if (!workspaceToDelete || !currentOrganization?.id) return;

    try {
      // Check if workspace has assets first
      const assetsResponse = await organizationService.getWorkspaceAssets(
        currentOrganization.id,
        workspaceToDelete.id
      );

      if (assetsResponse?.has_assets) {
        toast.error(t('deleteConfirm.hasAssetsError'), {
          description: t('deleteConfirm.hasAssetsDescription'),
        });
        setDeleteConfirmOpen(false);
        setWorkspaceToDelete(null);
        return;
      }

      await deleteWorkspace(workspaceToDelete.id);
      setDeleteConfirmOpen(false);
      setWorkspaceToDelete(null);
      // No need to manually refetch, mutation's onSuccess already invalidates queries
    } catch (error) {
      // Error is handled by the mutation
      console.error('Failed to delete workspace:', error);
    }
  };

  // Handle create workspace
  const handleCreateWorkspace = async (data: CreateWorkspaceRequest) => {
    if (!currentOrganization?.id) return;
    try {
      await createWorkspace(data);
      // No need to manually refetch, mutation's onSuccess already invalidates queries
    } catch (error) {
      const message = getErrorMessage(error) || t('loadError');
      toast.error(message);
      console.error('Failed to create workspace:', error);
      throw error; // Re-throw to let dialog handle it
    }
  };

  // Handle edit workspace
  const handleEditWorkspace = async (data: UpdateWorkspaceRequest) => {
    if (!currentOrganization?.id || !workspaceDialog.initialData) return;
    try {
      await updateWorkspace({
        workspaceId: workspaceDialog.initialData.id,
        data,
      });

      // Update local store if current workspace matched
      if (data.name) {
        const workspaceId = workspaceDialog.initialData.id;
        const newName = data.name;
        if (currentWorkspace?.id === workspaceId) {
          setCurrentWorkspace({ ...currentWorkspace, name: newName });
        }
        setWorkspaces(
          workspacesFromStore.map(w => (w.id === workspaceId ? { ...w, name: newName } : w))
        );
      }

      setWorkspaceDialog(prev => ({ ...prev, open: false, initialData: null }));
      // No need to manually refetch, mutation's onSuccess already invalidates queries
    } catch (error) {
      const message = getErrorMessage(error) || t('loadError');
      toast.error(message);
      console.error('Failed to update workspace:', error);
      throw error; // Re-throw to let dialog handle it
    }
  };

  // Handle open edit dialog
  const handleOpenEditDialog = (workspace: WorkspaceManagement) => {
    setWorkspaceDialog({
      open: true,
      mode: 'edit',
      initialData: workspace,
    });
  };

  const getWorkspaceDetailHref = (workspaceId: string) => {
    const query = assignMemberKeyword
      ? `?assignMember=${encodeURIComponent(assignMemberKeyword)}`
      : '';
    return `/dashboard/organization/workspaces/${workspaceId}${query}`;
  };

  return (
    <div className="flex h-full flex-col space-y-5 overflow-auto bg-bg-canvas/50 p-4 lg:p-6">
      {/* Header */}
      <div className="flex flex-col items-start justify-between gap-4 sm:flex-row sm:items-center">
        <div>
          <h1 className="text-2xl font-semibold tracking-tight text-text-primary">{t('title')}</h1>
          <p className="mt-1 max-w-2xl text-sm text-text-secondary">{t('description')}</p>
        </div>
        <Button
          onClick={() =>
            setWorkspaceDialog({
              open: true,
              mode: 'create',
              initialData: null,
            })
          }
          className="h-10 rounded-md bg-primary px-4 font-medium text-primary-foreground shadow-sm transition-colors hover:bg-primary-hover hover:text-primary-foreground"
        >
          <Plus className="mr-2 h-4 w-4" />
          {t('newWorkspace')}
        </Button>
      </div>

      {/* Main Content Area */}
      <div className="flex min-h-0 flex-1 flex-col overflow-hidden rounded-xl border border-border/80 bg-background shadow-sm">
        {assignMemberKeyword ? (
          <div className="flex flex-col gap-3 border-b border-warning/20 bg-warning/10 px-4 py-3 md:flex-row md:items-center md:justify-between">
            <div className="min-w-0">
              <p className="text-sm font-semibold text-warning">
                {t('assignMemberBannerTitle', { member: assignMemberKeyword })}
              </p>
              <p className="mt-0.5 text-xs text-muted-foreground">
                {t('assignMemberBannerDescription')}
              </p>
            </div>
            <Button asChild variant="outline" size="sm" className="shrink-0 bg-background">
              <Link href="/dashboard/organization/contacts">{t('assignMemberBackToContacts')}</Link>
            </Button>
          </div>
        ) : null}

        {/* Search Strip */}
        <div className="flex flex-col items-center gap-4 border-b border-border/60 bg-background p-4 md:flex-row">
          <div className="relative w-full max-w-md flex-1">
            <Search className="absolute left-3 top-1/2 -translate-y-1/2 h-4 w-4 text-text-placeholder" />
            <Input
              placeholder={t('searchPlaceholder')}
              value={searchKeyword}
              onChange={e => setSearchKeyword(e.target.value)}
              className="h-10 rounded-md bg-bg-canvas/50 pl-9 shadow-none transition-all focus:border-primary/40 focus:ring-0"
            />
          </div>
        </div>

        {/* Workspaces Table Section */}
        <div className="flex-1 overflow-hidden flex flex-col min-h-0">
          <StickyDataTable
            columns={[
              { key: 'workspaceName', header: t('workspaceName'), className: 'pl-6' },
              { key: 'manager', header: t('manager') },
              { key: 'department', header: t('department') },
              { key: 'memberCount', header: t('memberCount') },
              { key: 'actions', header: t('actions'), align: 'right', className: 'pr-6' },
            ]}
            data={workspaces}
            getRowKey={workspace => workspace.id}
            isLoading={shouldShowWorkspaceSkeleton}
            loadingRows={pageSize}
            renderSkeletonRow={index => (
              <tr key={`workspace-skeleton-${index}`} className="border-b border-border/10">
                <td colSpan={5} className="px-6 py-4">
                  <Skeleton className="h-16 w-full rounded-xl opacity-60" />
                </td>
              </tr>
            )}
            emptyState={
              <div className="flex flex-col items-center justify-center flex-1 py-20 text-text-placeholder">
                <div className="w-16 h-16 rounded-full bg-muted flex items-center justify-center mb-4">
                  <Search className="h-8 w-8 opacity-20" />
                </div>
                <p className="text-sm font-medium">{t('noWorkspaces')}</p>
              </div>
            }
            scrollClassName="scrollbar-thumb-muted-foreground/20"
            pagination={
              totalPages > 1 ? (
                <div className="p-4 bg-muted/30 border-t border-border/20 backdrop-blur-sm">
                  <Pagination
                    currentPage={currentPage}
                    totalPages={totalPages}
                    total={effectiveTotal}
                    pageSize={pageSize}
                    onPageChange={handlePageChange}
                    showInfo
                    showJump={false}
                    className="justify-center md:justify-end"
                  />
                </div>
              ) : null
            }
          >
            {!shouldShowWorkspaceSkeleton &&
              workspaces.map((workspace: WorkspaceManagement) => (
                <TableRow
                  key={workspace.id}
                  className="group border-b border-border/10 hover:bg-bg-canvas/40 transition-colors cursor-pointer interactive-subtle"
                  onClick={() => router.push(getWorkspaceDetailHref(workspace.id))}
                >
                  <TableCell className="py-4 pl-6">
                    <div className="flex items-center gap-3">
                      <div className="w-10 h-10 rounded-xl bg-primary text-primary-foreground flex items-center justify-center font-bold text-sm shadow-sm">
                        {workspace.name.charAt(0).toUpperCase()}
                      </div>
                      <div className="flex flex-col">
                        <span className="font-semibold text-text-primary group-hover:text-primary transition-colors">
                          {workspace.name}
                        </span>
                        <span className="text-[11px] text-text-placeholder uppercase font-medium tracking-tighter">
                          ID: {workspace.id.slice(0, 8)}
                        </span>
                      </div>
                      <ChevronRight className="h-4 w-4 text-text-placeholder opacity-0 -translate-x-2 group-hover:opacity-100 group-hover:translate-x-0 transition-all duration-300 ml-1" />
                    </div>
                  </TableCell>
                  <TableCell className="text-text-secondary font-medium">
                    {workspace.leader_name || (
                      <span className="text-text-placeholder italic">-</span>
                    )}
                  </TableCell>
                  <TableCell className="text-text-secondary font-medium">
                    {workspace.department_name || (
                      <span className="text-text-placeholder italic">-</span>
                    )}
                  </TableCell>
                  <TableCell className="text-text-secondary font-medium">
                    <div className="flex items-center gap-1.5">
                      <Users className="h-3.5 w-3.5 opacity-50" />
                      <span>
                        {workspace.member_count || 0}
                        <span className="text-[12px] ml-0.5 opacity-70">{t('people')}</span>
                      </span>
                    </div>
                  </TableCell>
                  <TableCell className="text-right pr-6" onClick={e => e.stopPropagation()}>
                    <div className="flex items-center justify-end gap-2">
                      <Tooltip>
                        <TooltipTrigger asChild>
                          <Button
                            variant="ghost"
                            size="sm"
                            isIcon
                            onClick={e => {
                              e.stopPropagation();
                              handleOpenEditDialog(workspace);
                            }}
                            className="h-8 w-8 rounded-lg hover:shadow-sm"
                          >
                            <Pencil className="h-4 w-4" />
                          </Button>
                        </TooltipTrigger>
                        <TooltipContent className="glass-panel border-none text-xs">
                          {t('edit')}
                        </TooltipContent>
                      </Tooltip>

                      <Tooltip>
                        <TooltipTrigger asChild>
                          <Button
                            variant="ghost"
                            size="sm"
                            isIcon
                            onClick={e => {
                              e.stopPropagation();
                              setWorkspaceToDelete(workspace);
                              setDeleteConfirmOpen(true);
                            }}
                            className="h-8 w-8 rounded-lg text-text-placeholder hover:text-destructive-foreground hover:bg-destructive transition-all"
                          >
                            <Trash2 className="h-4 w-4" />
                          </Button>
                        </TooltipTrigger>
                        <TooltipContent className="glass-panel border-none text-xs">
                          {t('disband')}
                        </TooltipContent>
                      </Tooltip>
                    </div>
                  </TableCell>
                </TableRow>
              ))}
          </StickyDataTable>
        </div>
      </div>

      {/* Dialogs remain functional but will benefit from global dialog styles */}
      <ConfirmDialog
        open={deleteConfirmOpen}
        onOpenChange={setDeleteConfirmOpen}
        title={t('deleteConfirm.title')}
        description={t('deleteConfirm.description', {
          workspaceName: workspaceToDelete?.name || '',
        })}
        confirmText={t('deleteConfirm.confirm')}
        cancelText={t('deleteConfirm.cancel')}
        loading={isDeleting}
        onConfirm={handleDeleteWorkspace}
        variant="danger"
      />

      <WorkspaceDialog
        open={workspaceDialog.open}
        onOpenChange={open => setWorkspaceDialog(prev => ({ ...prev, open }))}
        onCreate={handleCreateWorkspace}
        onUpdate={handleEditWorkspace}
        initialData={workspaceDialog.mode === 'edit' ? workspaceDialog.initialData : null}
        isLoading={workspaceDialog.mode === 'edit' ? isUpdating : isCreating}
      />
    </div>
  );
}

export default function WorkspaceManagementPage() {
  return (
    <Suspense fallback={null}>
      <WorkspaceManagementPageContent />
    </Suspense>
  );
}
