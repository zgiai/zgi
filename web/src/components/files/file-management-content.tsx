'use client';

import { useState, useCallback, useEffect, useRef } from 'react';
import { FileSidebar } from '@/components/files/file-sidebar';
import { FileList } from '@/components/files/file-list';
import { UploadDialog, type UploadConfig } from '@/components/files/upload-dialog';
import { CreateFolderDialog, type CreateFolderData } from '@/components/files/create-folder-dialog';
import {
  CreateTextFileDialog,
  type CreateTextFileData,
} from '@/components/files/create-text-file-dialog';

import CreateLocalFileDialog from '@/components/files/create-local-file-dialog';
import { useT } from '@/i18n';
import {
  Search,
  RefreshCw,
  Files,
  Check,
  ChevronsUpDown,
  Building2,
  Users,
  Info,
  Upload,
  PanelLeft,
  FolderPlus,
} from 'lucide-react';
import { Input } from '@/components/ui/input';
import { Button } from '@/components/ui/button';
import { Badge } from '@/components/ui/badge';
import { Pagination } from '@/components/ui/pagination';
import {
  Dialog,
  DialogBody,
  DialogContent,
  DialogDescription,
  DialogHeader,
  DialogTitle,
} from '@/components/ui/dialog';
import { Sheet, SheetContent, SheetHeader, SheetTitle } from '@/components/ui/sheet';
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuLabel,
  DropdownMenuSeparator,
  DropdownMenuTrigger,
} from '@/components/ui/dropdown-menu';
import { Alert, AlertDescription, AlertTitle } from '@/components/ui/alert';
import {
  useFiles,
  useCreateFolder,
  useCreateTextFile,
  useFileFolders,
  useDeleteFolder,
  FILE_FOLDERS_KEY,
  STORAGE_USAGE_KEY,
} from '@/hooks/use-files';
import { useQueryClient } from '@tanstack/react-query';
import { useAccountPermissions } from '@/hooks/organization/use-account-permissions';
import { useDebouncedValue } from '@/hooks/use-debounced-value';
import { useIsMobile } from '@/hooks/use-mobile';
import { filterLowercaseExtensions } from '@/utils/file-helpers';
import { ConfirmDialog } from '@/components/ui/confirm-dialog';
import { useAuthStore } from '@/store/auth-store';
import { useWorkspaceStore } from '@/store/workspace-store';
import type { FileFolder } from '@/services/types/file';
import type { FileItem } from '@/services/types/file';
import { cn } from '@/lib/utils';
import { useOrganizations } from '@/hooks/organization/use-organizations';
import { useJoinedWorkspaces } from '@/hooks/workspace/use-joined-workspaces';
import { useUpdateCurrentWorkspace } from '@/hooks/workspace/use-update-current-workspace';
import { fileManageService } from '@/services/file-manage.service';
import type { Organization } from '@/services/types/organization';
import type { Workspace } from '@/store/workspace-store';

export interface FileManagementContentProps {
  /** Enable file selection mode */
  selectionMode?: boolean;
  /** Selected file IDs (for selection mode) */
  selectedFileIds?: string[];
  /** Callback when selection changes (for selection mode) */
  onSelectionChange?: (selectedIds: string[], currentFiles: FileItem[]) => void;
  /** Callback when files list changes (for getting current files) */
  onFilesChange?: (files: FileItem[]) => void;
  /** Maximum number of files allowed to select */
  maxCount?: number;
  /** Allowed extensions like ['.jpg', '.png'] (case-insensitive) */
  acceptExt?: string[];
  /** Enable workspace switcher inside file selector empty state */
  allowWorkspaceSwitch?: boolean;
}

const SYSTEM_FILE_CATEGORIES = new Set(['all', 'uploaded', 'default']);

const waitForMinimumRefreshDuration = () =>
  new Promise<void>(resolve => {
    setTimeout(resolve, 1000);
  });

async function getFolderDepth(folderId: string) {
  let depth = 0;
  let currentId = folderId;

  while (currentId) {
    const response = await fileManageService.getFileFolder(currentId);
    const folder = response.data;
    depth += 1;

    if (!folder.parent_id) break;
    currentId = folder.parent_id;
  }

  return depth;
}

interface FileSelectorWorkspaceSwitcherProps {
  currentWorkspace: Workspace | null;
  compact?: boolean;
  hideTitle?: boolean;
  onWorkspaceSelected?: () => void;
}

interface FileSelectorOrganizationSwitcherProps {
  compact?: boolean;
  hideTitle?: boolean;
}

function FileSelectorOrganizationSwitcher({
  compact = false,
  hideTitle = false,
}: FileSelectorOrganizationSwitcherProps) {
  const tNavigation = useT('navigation');
  const isAuthenticated = useAuthStore.use.isAuthenticated();
  const { organizations, currentOrganization, switchOrganization } =
    useOrganizations(isAuthenticated);

  const handleSelectOrganization = useCallback(
    async (organization: Organization) => {
      await switchOrganization(organization);
    },
    [switchOrganization]
  );

  if (!isAuthenticated || organizations.length <= 1) {
    return null;
  }

  const currentOrganizationLabel =
    currentOrganization?.short_name || currentOrganization?.name || tNavigation('organizations');

  return (
    <div
      className={cn(
        'w-full text-left',
        compact ? '' : 'rounded-xl border border-border/80 bg-muted/30 p-3'
      )}
    >
      {!hideTitle ? (
        <p className="mb-1.5 text-[11px] font-medium uppercase tracking-[0.12em] text-muted-foreground">
          {tNavigation('organizations')}
        </p>
      ) : null}
      <DropdownMenu>
        <DropdownMenuTrigger asChild>
          <Button
            variant="outline"
            className={cn(
              'w-full justify-between border-border/80 bg-background px-3 shadow-none',
              compact ? 'h-8 rounded-lg' : 'h-10 rounded-lg shadow-sm'
            )}
          >
            <div className="flex min-w-0 items-center gap-2">
              <div
                className={cn(
                  'flex shrink-0 items-center justify-center rounded-md bg-primary/10 text-primary',
                  compact ? 'h-5 w-5' : 'h-7 w-7'
                )}
              >
                <Building2 className={cn(compact ? 'h-3 w-3' : 'h-4 w-4')} />
              </div>
              <span className={cn('truncate font-medium', compact ? 'text-[12px]' : 'text-sm')}>
                {currentOrganizationLabel}
              </span>
            </div>
            <ChevronsUpDown className="h-4 w-4 shrink-0 text-muted-foreground" />
          </Button>
        </DropdownMenuTrigger>
        <DropdownMenuContent align="center" className="w-[320px]">
          <DropdownMenuLabel>{tNavigation('organizations')}</DropdownMenuLabel>
          <DropdownMenuSeparator />
          {organizations.map(organization => (
            <DropdownMenuItem
              key={organization.id}
              onClick={() => handleSelectOrganization(organization)}
              className="flex cursor-pointer items-center justify-between"
              title={organization.name}
              disabled={organization.id === currentOrganization?.id}
            >
              <div className="flex min-w-0 items-center gap-2">
                <div className="flex h-6 w-6 shrink-0 items-center justify-center rounded-md bg-primary/10 text-primary">
                  <Building2 className="h-3.5 w-3.5" />
                </div>
                <span className="truncate text-xs">
                  {organization.short_name || organization.name}
                </span>
              </div>
              {organization.id === currentOrganization?.id ? (
                <Check className="h-4 w-4 text-primary" />
              ) : null}
            </DropdownMenuItem>
          ))}
        </DropdownMenuContent>
      </DropdownMenu>
    </div>
  );
}

function FileSelectorWorkspaceSwitcher({
  currentWorkspace,
  compact = false,
  hideTitle = false,
  onWorkspaceSelected,
}: FileSelectorWorkspaceSwitcherProps) {
  const t = useT('files');
  const tNavigation = useT('navigation');
  const workspaces = useWorkspaceStore.use.workspaces();
  const { mutateAsync: updateWorkspace, isPending: isUpdatingWorkspace } =
    useUpdateCurrentWorkspace();

  useJoinedWorkspaces({ syncToStore: true });

  const currentWorkspaceLabel = currentWorkspace?.name || tNavigation('switchWorkspace');

  const handleSelectWorkspace = useCallback(
    async (workspace: Workspace) => {
      await updateWorkspace(workspace);
      onWorkspaceSelected?.();
    },
    [onWorkspaceSelected, updateWorkspace]
  );

  const trigger = (
    <Button
      variant="outline"
      disabled={isUpdatingWorkspace}
      className={cn(
        'w-full justify-between border-border/80 bg-background px-3 shadow-none',
        compact ? 'h-8 rounded-lg' : 'h-10 rounded-lg shadow-sm'
      )}
    >
      <div className="flex min-w-0 items-center gap-2">
        <div
          className={cn(
            'flex shrink-0 items-center justify-center rounded-md bg-primary/10 text-primary',
            compact ? 'h-5 w-5' : 'h-7 w-7'
          )}
        >
          <Users className={cn(compact ? 'h-3 w-3' : 'h-4 w-4')} />
        </div>
        <span className={cn('truncate font-medium', compact ? 'text-[12px]' : 'text-sm')}>
          {currentWorkspaceLabel}
        </span>
      </div>
      <ChevronsUpDown className="h-4 w-4 shrink-0 text-muted-foreground" />
    </Button>
  );

  if (workspaces.length === 0) {
    return (
      <div
        className={cn(
          'w-full text-left',
          compact ? '' : 'rounded-lg border border-dashed border-border/70 px-3 py-2'
        )}
      >
        <p className="mb-1 text-[11px] font-medium uppercase tracking-[0.12em] text-muted-foreground">
          {tNavigation('switchWorkspace')}
        </p>
        <p className="text-xs leading-5 text-muted-foreground">
          {t('selectorEmptyState.noWorkspaces')}
        </p>
      </div>
    );
  }

  return (
    <div
      className={cn(
        'w-full text-left',
        compact ? '' : 'rounded-xl border border-border/80 bg-muted/30 p-3'
      )}
    >
      {!hideTitle ? (
        <p className="mb-1.5 text-[11px] font-medium uppercase tracking-[0.12em] text-muted-foreground">
          {tNavigation('switchWorkspace')}
        </p>
      ) : null}
      <DropdownMenu>
        <DropdownMenuTrigger asChild>{trigger}</DropdownMenuTrigger>
        <DropdownMenuContent align="start" className="w-[280px]">
          <DropdownMenuLabel>{tNavigation('switchWorkspace')}</DropdownMenuLabel>
          <DropdownMenuSeparator />
          {workspaces.map(workspace => (
            <DropdownMenuItem
              key={workspace.id}
              onClick={() => void handleSelectWorkspace(workspace)}
              className="flex cursor-pointer items-center justify-between"
              title={workspace.name}
            >
              <div className="flex min-w-0 items-center gap-2">
                <div className="flex h-[22px] w-[22px] shrink-0 items-center justify-center rounded-md bg-primary/10 text-primary">
                  <Users className="h-3.5 w-3.5" />
                </div>
                <span className="truncate text-xs">{workspace.name}</span>
              </div>
              {currentWorkspace?.id === workspace.id ? (
                <Check className="h-4 w-4 text-primary" />
              ) : null}
            </DropdownMenuItem>
          ))}
        </DropdownMenuContent>
      </DropdownMenu>
    </div>
  );
}

interface FileSelectorSpaceSwitcherDialogProps {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  currentWorkspace: Workspace | null;
  showOrganizationSwitcher: boolean;
}

function FileSelectorSpaceSwitcherDialog({
  open,
  onOpenChange,
  currentWorkspace,
  showOrganizationSwitcher,
}: FileSelectorSpaceSwitcherDialogProps) {
  const t = useT('files');
  const tNavigation = useT('navigation');
  const isMobile = useIsMobile();

  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent
        className={cn(
          'overflow-hidden p-0',
          isMobile
            ? 'left-0 top-auto bottom-0 h-auto max-h-[85dvh] w-screen max-w-none translate-x-0 translate-y-0 rounded-t-[28px] rounded-b-none border-x-0 border-b-0'
            : 'max-w-[520px]'
        )}
      >
        <DialogHeader className={cn('border-b', isMobile ? 'px-4 py-4' : 'px-5 py-4')}>
          <DialogTitle className="text-lg font-semibold">
            {t('selectorContext.dialogTitle')}
          </DialogTitle>
          <DialogDescription className="pt-1 text-sm text-muted-foreground">
            {t('selectorContext.dialogDescription')}
          </DialogDescription>
        </DialogHeader>
        <DialogBody className={cn('space-y-5', isMobile ? 'px-4 py-4' : 'px-5 py-5')}>
          {showOrganizationSwitcher ? (
            <div className="space-y-2">
              <p className="text-xs font-medium uppercase tracking-[0.12em] text-muted-foreground">
                {tNavigation('organizations')}
              </p>
              <FileSelectorOrganizationSwitcher hideTitle />
            </div>
          ) : null}

          <div className="space-y-2">
            <p className="text-xs font-medium uppercase tracking-[0.12em] text-muted-foreground">
              {tNavigation('switchWorkspace')}
            </p>
            <FileSelectorWorkspaceSwitcher
              currentWorkspace={currentWorkspace}
              hideTitle
              onWorkspaceSelected={() => onOpenChange(false)}
            />
          </div>

          <Alert className="border-border/70 bg-muted/30 text-left">
            <Info className="h-4 w-4 text-primary" />
            <AlertTitle className="text-sm font-semibold text-foreground">
              {t('selectorContext.tipTitle')}
            </AlertTitle>
            <AlertDescription className="text-sm text-muted-foreground">
              {t('selectorContext.tipDescription')}
            </AlertDescription>
          </Alert>
        </DialogBody>
      </DialogContent>
    </Dialog>
  );
}

const FileManagementContent = ({
  selectionMode = false,
  selectedFileIds = [],
  onSelectionChange,
  onFilesChange,
  acceptExt = [],
  maxCount,
  allowWorkspaceSwitch = false,
}: FileManagementContentProps): React.ReactNode => {
  const [searchValue, setSearchValue] = useState('');
  const [activeCategory, setActiveCategory] = useState('all');
  const [activeFolderDepth, setActiveFolderDepth] = useState(0);
  const [createFolderInitialParentId, setCreateFolderInitialParentId] = useState('');
  const [isRefreshing, setIsRefreshing] = useState(false);
  const [selectedFiles, setSelectedFiles] = useState<string[]>(selectedFileIds);
  const [spaceSwitcherOpen, setSpaceSwitcherOpen] = useState(false);
  const [mobileSidebarOpen, setMobileSidebarOpen] = useState(false);
  const t = useT();
  const tNavigation = useT('navigation');
  const queryClient = useQueryClient();
  const isMobile = useIsMobile();
  const isAuthenticated = useAuthStore.use.isAuthenticated();
  const { currentWorkspace, contextStatus } = useWorkspaceStore();
  const hasReadyWorkspace = contextStatus === 'ready' && !!currentWorkspace;
  const isWorkspaceRequired = contextStatus === 'workspace_required';
  const workspaceId = hasReadyWorkspace ? currentWorkspace.id : undefined;
  const isMobileSelectionMode = isMobile && selectionMode;
  const isDesktopSelectionMode = !isMobile && selectionMode;

  const { createFolder } = useCreateFolder();
  const { createTextFile, isCreating: isCreatingTextFile } = useCreateTextFile();
  const { deleteFolder, isDeleting: isDeletingFolder } = useDeleteFolder();
  const { folders } = useFileFolders(workspaceId);

  const { hasPermission } = useAccountPermissions();
  const canManage = hasPermission('file.manage');
  const canCreateFolder = hasPermission('file.move_create');
  const canUpload = hasPermission('file.upload_create');
  const canCreateInActiveFolder = canCreateFolder && activeFolderDepth >= 0 && activeFolderDepth < 2;
  const { organizations } = useOrganizations(isAuthenticated);
  const showOrganizationSwitcher = isAuthenticated && organizations.length > 1;
  const currentSpaceLabel = currentWorkspace?.name || tNavigation('switchWorkspace');
  const mobilePrimaryActionLabel =
    !selectionMode || !isMobileSelectionMode
      ? undefined
      : isWorkspaceRequired && allowWorkspaceSwitch
        ? t('files.mobileSelector.switchSpace')
        : canUpload
          ? t('files.mobileSelector.browseAndUpload')
          : allowWorkspaceSwitch
            ? t('files.mobileSelector.switchSpace')
            : t('files.mobileSelector.browse');
  const mobileEmptyDescription =
    !selectionMode || !isMobileSelectionMode
      ? undefined
      : isWorkspaceRequired
        ? t('files.selectorEmptyState.description')
        : canUpload
          ? t('files.mobileSelector.emptyDescriptionWithUpload')
          : t('files.mobileSelector.emptyDescriptionWithoutUpload');

  const debouncedSearchValue = useDebouncedValue(searchValue, 500);

  // Convert acceptExt array to extension string format (comma-separated, lowercase, no leading dots)
  const extensionParam =
    acceptExt.length > 0 ? filterLowercaseExtensions(acceptExt).join(',') : undefined;

  const { files, currentPage, totalPages, total, isLoading, isFetching, error, goToPage, reload } =
    useFiles('20', {
      category: activeCategory,
      keyword: debouncedSearchValue,
      sort: 'created_at',
      extension: extensionParam,
      workspaceId: workspaceId,
    });

  const prevPropRef = useRef<string[]>(selectedFileIds);
  const prevInternalRef = useRef<string[]>(selectedFiles);

  useEffect(() => {
    if (selectionMode) {
      const propChanged =
        prevPropRef.current.length !== selectedFileIds.length ||
        !prevPropRef.current.every((id, idx) => selectedFileIds[idx] === id);

      if (propChanged) {
        setSelectedFiles(selectedFileIds);
        prevPropRef.current = selectedFileIds;
        prevInternalRef.current = selectedFileIds;
      }
    }
  }, [selectionMode, selectedFileIds]);

  useEffect(() => {
    if (selectionMode && onSelectionChange) {
      const internalChanged =
        prevInternalRef.current.length !== selectedFiles.length ||
        !prevInternalRef.current.every((id, idx) => selectedFiles[idx] === id);

      if (internalChanged) {
        onSelectionChange(selectedFiles, files);
        prevInternalRef.current = selectedFiles;
      }
    }
  }, [selectionMode, selectedFiles, onSelectionChange, files]);

  const isRefreshPending = isRefreshing || isFetching;

  const handleRefresh = async () => {
    if (isRefreshing) return;

    setIsRefreshing(true);
    goToPage(1);
    try {
      await Promise.all([
        Promise.all([
          reload(),
          queryClient.invalidateQueries({ queryKey: [FILE_FOLDERS_KEY] }),
          queryClient.invalidateQueries({ queryKey: [STORAGE_USAGE_KEY] }),
        ]),
        waitForMinimumRefreshDuration(),
      ]);
    } finally {
      setIsRefreshing(false);
    }
  };

  const handleSelectionChange = (selectedIds: string[]) => {
    setSelectedFiles(selectedIds);
  };

  const handleCategoryChange = useCallback((category: string) => {
    setActiveCategory(category);
    setSelectedFiles([]);
    setMobileSidebarOpen(false);
  }, []);

  useEffect(() => {
    if (SYSTEM_FILE_CATEGORIES.has(activeCategory)) {
      setActiveFolderDepth(0);
      return;
    }

    let ignore = false;
    setActiveFolderDepth(-1);

    const loadActiveFolderDepth = async () => {
      try {
        const depth = await getFolderDepth(activeCategory);
        if (!ignore) {
          setActiveFolderDepth(depth);
        }
      } catch {
        if (!ignore) {
          setActiveFolderDepth(0);
        }
      }
    };

    void loadActiveFolderDepth();

    return () => {
      ignore = true;
    };
  }, [activeCategory]);

  const [createFolderDialogOpen, setCreateFolderDialogOpen] = useState(false);

  const handleNewFolder = useCallback(async () => {
    if (SYSTEM_FILE_CATEGORIES.has(activeCategory)) {
      setCreateFolderInitialParentId('');
      setCreateFolderDialogOpen(true);
      return;
    }

    try {
      const depth = await getFolderDepth(activeCategory);
      setCreateFolderInitialParentId(depth <= 1 ? activeCategory : '');
    } catch {
      setCreateFolderInitialParentId('');
    }
    setCreateFolderDialogOpen(true);
  }, [activeCategory]);

  const handleCreateFolderConfirm = useCallback(
    async (data: CreateFolderData) => {
      const createdFolder = await createFolder({
        name: data.name,
        parent_id: data.parent_id,
        workspace_id: data.workspaceId,
      });
      setCreateFolderDialogOpen(false);
      setActiveCategory(createdFolder.id);
      setSelectedFiles([]);
      reload();
      goToPage(1);
    },
    [createFolder, goToPage, reload]
  );

  const handleUpload = () => {
    openAddDialog();
  };

  const [addDialogOpen, setAddDialogOpen] = useState(false);
  const openAddDialog = useCallback(() => setAddDialogOpen(true), []);

  const [createTextFileDialogOpen, setCreateTextFileDialogOpen] = useState(false);
  const [selectedFolderId, setSelectedFolderId] = useState<string>('');
  const [selectedUploadWorkspaceId, setSelectedUploadWorkspaceId] = useState<string>('');
  const [selectedUploadProcessingMode, setSelectedUploadProcessingMode] =
    useState<UploadConfig['processingMode']>('process_now');

  const handleUploadConfirm = useCallback((config: UploadConfig) => {
    setAddDialogOpen(false);
    setSelectedUploadWorkspaceId(config.workspaceId);
    setSelectedUploadProcessingMode(config.processingMode);

    if (config.mode === 'text') {
      setSelectedFolderId(config.folderId);
      setCreateTextFileDialogOpen(true);
    } else {
      // Always use dialog for file upload
      setSelectedFolderId(config.folderId);
      setCreateLocalFileDialogOpen(true);
    }
  }, []);

  const [createLocalFileDialogOpen, setCreateLocalFileDialogOpen] = useState(false);

  const handleCreateTextFileConfirm = useCallback(
    async (data: CreateTextFileData) => {
      await createTextFile({
        filename: data.filename,
        content: data.content,
        folder_id: data.folder_id,
        workspace_id: selectedUploadWorkspaceId || workspaceId,
      });
      setCreateTextFileDialogOpen(false);
      setSelectedFolderId('');
      setSelectedUploadWorkspaceId('');
      setSelectedUploadProcessingMode('process_now');
      // Refresh file list after creating text file
      if (selectionMode) {
        goToPage(1);
      }
    },
    [createTextFile, selectionMode, goToPage, selectedUploadWorkspaceId, workspaceId]
  );

  const handleFileUploadComplete = useCallback(() => {
    setSelectedFolderId('');
    setSelectedUploadWorkspaceId('');
    setSelectedUploadProcessingMode('process_now');
    reload();
    goToPage(1);
  }, [goToPage, reload]);

  const initialUploadFolderId =
    SYSTEM_FILE_CATEGORIES.has(activeCategory) ? '' : activeCategory;

  const selectedFolder = folders.find(f => f.id === selectedFolderId);
  const selectedFolderName = selectedFolder?.name || t('files.upload.defaultFolder');

  const [deleteDialogOpen, setDeleteDialogOpen] = useState(false);
  const [folderToDelete, setFolderToDelete] = useState<FileFolder | null>(null);

  const handleFolderDelete = useCallback((folder: FileFolder) => {
    setFolderToDelete(folder);
    setDeleteDialogOpen(true);
  }, []);

  const handleDeleteConfirm = useCallback(async () => {
    if (!folderToDelete) return;

    try {
      await deleteFolder(folderToDelete.id);
      setDeleteDialogOpen(false);
      setFolderToDelete(null);
    } catch (error) {
      console.error('Failed to delete folder:', error);
    }
  }, [folderToDelete, deleteFolder]);

  const sidebarContent = (
    <FileSidebar
      activeItemId={activeCategory}
      onItemClick={handleCategoryChange}
      onNewFolder={selectionMode && canCreateInActiveFolder ? handleNewFolder : undefined}
      onUpload={selectionMode && canUpload ? handleUpload : undefined}
      onFolderDelete={canManage ? handleFolderDelete : undefined}
      workspaceId={workspaceId}
      flushTop
    />
  );

  const spaceSwitcherButton =
    selectionMode && allowWorkspaceSwitch ? (
      <Button
        variant="outline"
        className={cn(
          'justify-between rounded-lg border-border/80 bg-background shadow-none',
          isMobileSelectionMode ? 'h-10 min-w-0 flex-1 px-3' : 'h-9 w-full px-3'
        )}
        onClick={() => setSpaceSwitcherOpen(true)}
      >
          <div className="flex min-w-0 items-center gap-2">
            <div className="flex h-5 w-5 shrink-0 items-center justify-center rounded-md bg-primary/10 text-primary">
              <Users className="h-3 w-3" />
            </div>
          <span
            className={cn(
              'truncate font-medium',
              isMobileSelectionMode ? 'text-sm' : 'text-[12px]'
            )}
          >
            {currentSpaceLabel}
          </span>
        </div>
        <ChevronsUpDown className="h-4 w-4 shrink-0 text-muted-foreground" />
      </Button>
    ) : null;

  const selectorEmptyState =
    selectionMode &&
    isWorkspaceRequired &&
    activeCategory === 'all' &&
    !searchValue.trim() &&
    files.length === 0 ? (
      <div
        className={cn(
          'flex h-full items-center justify-center',
          isMobileSelectionMode ? 'px-4 py-6' : 'px-6 py-8'
        )}
      >
        <div
          className={cn(
            'flex w-full flex-col items-center justify-center border border-dashed border-border/80 bg-background/80 text-center shadow-sm',
            isMobileSelectionMode
              ? 'max-w-md rounded-2xl px-5 py-6'
              : 'max-w-2xl rounded-3xl px-8 py-10'
          )}
        >
          <Badge variant="info" className="mb-4 rounded-full px-3 py-1">
            {t('files.selectorEmptyState.badge')}
          </Badge>
          <div
            className={cn(
              'mb-5 flex items-center justify-center rounded-full bg-primary/10 text-primary',
              isMobileSelectionMode ? 'h-14 w-14' : 'h-16 w-16'
            )}
          >
            <Files className={cn(isMobileSelectionMode ? 'h-7 w-7' : 'h-8 w-8')} />
          </div>
          <h3
            className={cn(
              'mb-3 font-semibold text-foreground',
              isMobileSelectionMode ? 'text-xl' : 'text-2xl'
            )}
          >
            {t('files.selectorEmptyState.title')}
          </h3>
          <p
            className={cn(
              'text-sm text-muted-foreground',
              isMobileSelectionMode ? 'mb-5 max-w-sm leading-6' : 'mb-6 max-w-xl leading-6'
            )}
          >
            {t('files.selectorEmptyState.description')}
          </p>

          <Alert
            className={cn(
              'border-primary/15 bg-primary/5 text-left',
              isMobileSelectionMode ? 'mb-5 w-full rounded-2xl' : 'mb-6 w-full max-w-xl'
            )}
          >
            <Info className="h-4 w-4 text-primary" />
            <AlertTitle className="text-sm font-semibold text-foreground">
              {t('files.selectorEmptyState.noticeTitle')}
            </AlertTitle>
            <AlertDescription className="text-sm text-muted-foreground">
              {t('files.selectorEmptyState.noticeDescription')}
            </AlertDescription>
          </Alert>

          {allowWorkspaceSwitch ? (
            <div
              className={cn(
                'w-full text-left',
                isMobileSelectionMode
                  ? 'rounded-2xl border border-border/80 bg-muted/30 p-4'
                  : 'max-w-xl rounded-2xl border border-border/80 bg-muted/30 p-4 shadow-sm'
              )}
            >
              <div className="mb-4 flex items-start gap-3">
                <div className="flex h-9 w-9 shrink-0 items-center justify-center rounded-xl bg-primary/10 text-primary">
                  <Upload className="h-4 w-4" />
                </div>
                <div className="min-w-0">
                  <p className="text-sm font-semibold text-foreground">
                    {t('files.selectorEmptyState.quickActionTitle')}
                  </p>
                  <p className="mt-1 text-sm leading-6 text-muted-foreground">
                    {t('files.selectorEmptyState.quickActionDescription')}
                  </p>
                </div>
              </div>

              <Button
                variant="outline"
                className="h-10 w-full justify-between rounded-lg border-border/80 bg-background px-3 shadow-none"
                onClick={() => setSpaceSwitcherOpen(true)}
              >
                <div className="flex min-w-0 items-center gap-2">
                  <div className="flex h-7 w-7 shrink-0 items-center justify-center rounded-md bg-primary/10 text-primary">
                    <Users className="h-4 w-4" />
                  </div>
                  <span className="truncate text-sm font-medium">
                    {t('files.selectorContext.action')}
                  </span>
                </div>
                <ChevronsUpDown className="h-4 w-4 shrink-0 text-muted-foreground" />
              </Button>
            </div>
          ) : null}
        </div>
      </div>
    ) : null;

  const fileContent = error ? (
    <div className="flex h-full items-center justify-center">
      <div className="text-center">
        <p className="mb-4 text-red-500">{error}</p>
      </div>
    </div>
  ) : selectorEmptyState ? (
    selectorEmptyState
  ) : (
    <>
      <FileList
        files={files}
        total={total}
        selectedFiles={selectedFiles}
        onSelectionChange={ids => {
          handleSelectionChange(ids);
          onSelectionChange?.(ids, files);
          onFilesChange?.(files.filter(file => ids.includes(file.id)));
        }}
        maxCount={maxCount}
        isLoading={isLoading}
        selectionMode={selectionMode}
        activeCategory={activeCategory}
        mobileEmptyActionLabel={mobilePrimaryActionLabel}
        mobileEmptyDescription={mobileEmptyDescription}
        onMobileEmptyAction={() => {
          if (isWorkspaceRequired && allowWorkspaceSwitch) {
            setSpaceSwitcherOpen(true);
            return;
          }

          if (!canUpload && allowWorkspaceSwitch) {
            setSpaceSwitcherOpen(true);
            return;
          }

          setMobileSidebarOpen(true);
        }}
      />
      {!isLoading && files.length > 0 && totalPages > 1 ? (
        <div
          className={cn(
            'shrink-0 border-t',
            isMobileSelectionMode ? 'px-3 py-3' : 'flex justify-end px-4 py-3'
          )}
        >
          <Pagination
            currentPage={currentPage}
            totalPages={totalPages}
            total={total}
            pageSize={20}
            onPageChange={goToPage}
            showInfo={false}
          />
        </div>
      ) : null}
    </>
  );

  return (
    <>
      {isMobileSelectionMode ? (
        <div className="flex min-h-0 flex-1 flex-col bg-background">
          <div className="shrink-0 border-b bg-background px-3 py-3">
            <div className="flex items-center gap-2">
              {spaceSwitcherButton}
              <Button
                variant="outline"
                className={cn(
                  'h-10 gap-2 rounded-lg border-border/80 px-3 shadow-none',
                  spaceSwitcherButton ? 'shrink-0' : 'flex-1'
                )}
                onClick={() => setMobileSidebarOpen(true)}
              >
                <PanelLeft className="h-4 w-4" />
                <span>
                  {canUpload
                    ? t('files.mobileSelector.browseAndUpload')
                    : t('files.mobileSelector.browse')}
                </span>
              </Button>
            </div>
            <div className="relative mt-3">
              <Search className="absolute left-3 top-1/2 h-4 w-4 -translate-y-1/2 text-muted-foreground" />
              <Input
                placeholder={t('files.search.placeholder')}
                value={searchValue}
                onChange={e => setSearchValue(e.target.value)}
                className="h-10 pl-9"
              />
            </div>
          </div>

          <div className="min-h-0 flex-1 overflow-hidden">{fileContent}</div>

          <Sheet open={mobileSidebarOpen} onOpenChange={setMobileSidebarOpen}>
            <SheetContent
              side="bottom"
              className="h-[82dvh] rounded-t-[28px] border-x-0 border-b-0 p-0"
            >
              <SheetHeader className="border-b px-4 py-4 text-left">
                <SheetTitle className="text-base font-semibold">
                  {t('files.mobileSelector.browse')}
                </SheetTitle>
              </SheetHeader>
              <div className="min-h-0 flex-1 overflow-hidden">{sidebarContent}</div>
            </SheetContent>
          </Sheet>
        </div>
      ) : isDesktopSelectionMode ? (
        <div className="flex min-h-0 flex-1 flex-col">
          <div className="shrink-0 border-b bg-background">
            <div className="flex min-w-0 items-center gap-3 px-4 py-2">
              {spaceSwitcherButton ? (
                <div className="w-52 shrink-0">{spaceSwitcherButton}</div>
              ) : null}

              <div className="flex min-w-0 items-center gap-2">
                <h1 className="text-base font-semibold">{t('files.title')}</h1>
                <Button
                  isIcon
                  variant="ghost"
                  className="size-7 cursor-pointer rounded-sm hover:bg-muted"
                  onClick={handleRefresh}
                  disabled={isRefreshPending}
                >
                  <RefreshCw
                    size={16}
                    className={`${isRefreshPending ? 'animate-spin' : ''} h-4 w-4`}
                  />
                </Button>
              </div>

              <div className="ml-auto w-72 max-w-[38vw]">
                <div className="relative">
                  <Search className="absolute left-3 top-1/2 h-4 w-4 -translate-y-1/2 text-muted-foreground" />
                  <Input
                    placeholder={t('files.search.placeholder')}
                    value={searchValue}
                    onChange={e => setSearchValue(e.target.value)}
                    className="h-9 pl-9"
                  />
                </div>
              </div>
            </div>
          </div>

          <div className="flex min-h-0 flex-1">
            <div className="flex w-52 shrink-0 flex-col border-r bg-background">
              <div className="min-h-0 flex-1 overflow-y-auto">{sidebarContent}</div>
            </div>

            <div className="flex min-w-0 flex-1 flex-col overflow-hidden">
              <div className="flex min-h-0 flex-1 flex-col overflow-hidden">
                <div className="flex h-full flex-col">{fileContent}</div>
              </div>
            </div>
          </div>
        </div>
      ) : (
        <div className="flex min-h-0 flex-1 bg-bg-canvas">
          <div className="flex w-52 shrink-0 flex-col border-r bg-background">
            {spaceSwitcherButton ? (
              <div className="shrink-0 border-b px-4 py-2">{spaceSwitcherButton}</div>
            ) : null}

            <div className="min-h-0 flex-1 overflow-y-auto">{sidebarContent}</div>
          </div>

          <div className="flex min-w-0 flex-1 flex-col overflow-hidden">
            <div className="sticky top-0 z-10 border-b bg-bg-canvas/95 px-6 py-5 backdrop-blur">
              <div className="flex min-w-0 items-start justify-between gap-4">
                <div className="min-w-0">
                  <div className="text-xs font-medium uppercase tracking-[0.12em] text-muted-foreground">
                    {t('files.eyebrow')}
                  </div>
                  <h1 className="mt-1 text-2xl font-semibold tracking-tight text-foreground">
                    {t('files.title')}
                  </h1>
                  <p className="mt-2 max-w-2xl text-sm leading-6 text-muted-foreground">
                    {t('files.description')}
                  </p>
                </div>

                <div className="flex shrink-0 items-center gap-2">
                  <Button
                    isIcon
                    variant="outline"
                    className="size-9 rounded-md bg-background shadow-none"
                    onClick={handleRefresh}
                    disabled={isRefreshPending}
                    aria-label={t('common.refresh')}
                  >
                    <RefreshCw className={`${isRefreshPending ? 'animate-spin' : ''} h-4 w-4`} />
                  </Button>
                  {canCreateInActiveFolder ? (
                    <Button
                      variant="outline"
                      className="h-9 gap-2 rounded-md bg-background px-3 shadow-none"
                      onClick={handleNewFolder}
                    >
                      <FolderPlus className="h-4 w-4" />
                      {t('files.sidebar.newFolder')}
                    </Button>
                  ) : null}
                  {canUpload ? (
                    <Button className="h-9 gap-2 rounded-md px-3" onClick={handleUpload}>
                      <Upload className="h-4 w-4" />
                      {t('files.sidebar.uploadFile')}
                    </Button>
                  ) : null}
                </div>
              </div>

              <div className="mt-4 flex items-center gap-3">
                <div className="relative w-full max-w-xl">
                  <Search className="absolute left-3 top-1/2 h-4 w-4 -translate-y-1/2 text-muted-foreground" />
                  <Input
                    placeholder={t('files.search.placeholder')}
                    value={searchValue}
                    onChange={e => setSearchValue(e.target.value)}
                    className="h-9 rounded-md bg-background pl-9 shadow-none"
                  />
                </div>
                {searchValue.trim() ? (
                  <Button
                    variant="ghost"
                    className="h-9 rounded-md px-3 text-muted-foreground"
                    onClick={() => setSearchValue('')}
                  >
                    {t('common.clear')}
                  </Button>
                ) : null}
              </div>
            </div>

            <div className="flex min-h-0 flex-1 flex-col overflow-hidden px-6 pb-6 pt-4">
              <div className="flex h-full flex-col overflow-hidden rounded-lg border border-border/80 bg-background shadow-sm">
                {fileContent}
              </div>
            </div>
          </div>
        </div>
      )}
      {/* Upload Dialog */}
      <UploadDialog
        open={addDialogOpen}
        onOpenChange={setAddDialogOpen}
        onConfirm={handleUploadConfirm}
        initialFolderId={initialUploadFolderId}
      />
      {/* Create Folder Dialog */}
      <CreateFolderDialog
        open={createFolderDialogOpen}
        onOpenChange={setCreateFolderDialogOpen}
        onConfirm={handleCreateFolderConfirm}
        initialParentId={createFolderInitialParentId}
      />
      {/* Create Text File Dialog */}
      <CreateTextFileDialog
        open={createTextFileDialogOpen}
        onOpenChange={setCreateTextFileDialogOpen}
        onConfirm={handleCreateTextFileConfirm}
        folderId={selectedFolderId}
        folderName={selectedFolderName}
        isCreating={isCreatingTextFile}
      />
      {/* Create Local File Dialog */}
      <CreateLocalFileDialog
        open={createLocalFileDialogOpen}
        onOpenChange={setCreateLocalFileDialogOpen}
        folderId={selectedFolderId}
        workspaceId={selectedUploadWorkspaceId || workspaceId}
        processingMode={selectedUploadProcessingMode}
        acceptExt={acceptExt}
        onUploadComplete={handleFileUploadComplete}
      />
      {/* Delete Folder Confirmation Dialog (only for full page mode) */}
      <ConfirmDialog
        variant="danger"
        open={deleteDialogOpen}
        onOpenChange={setDeleteDialogOpen}
        title={t('files.delete.folderConfirmTitle', { name: folderToDelete?.name || '' })}
        description={t('files.delete.folderConfirmDescription')}
        confirmText={t('common.confirm')}
        cancelText={t('common.cancel')}
        onConfirm={handleDeleteConfirm}
        loading={isDeletingFolder}
      />
      <FileSelectorSpaceSwitcherDialog
        open={spaceSwitcherOpen}
        onOpenChange={setSpaceSwitcherOpen}
        currentWorkspace={currentWorkspace}
        showOrganizationSwitcher={showOrganizationSwitcher}
      />
    </>
  );
};

export default FileManagementContent;
