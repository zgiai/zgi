import { memo, useState, useCallback, useEffect } from 'react';
import { Clock3, FolderPlus, Upload, Files, FolderOpen, HardDrive } from 'lucide-react';
import { useT, type FilesSuffix } from '@/i18n';
import { Button } from '@/components/ui/button';
import { Progress } from '@/components/ui/progress';
import { Skeleton } from '@/components/ui/skeleton';
import { cn } from '@/lib/utils';
import { useStorageUsage, useFileFolders } from '@/hooks/use-files';
import { fileManageService } from '@/services/file-manage.service';
import { FolderTreeNode } from './folder-tree-node';
import type { FileFolder } from '@/services/types/file';
import { MAX_FILE_FOLDER_TREE_LEVEL } from './file-folder-levels';

export interface FileSidebarItem {
  id: string;
  labelKey: FilesSuffix;
  icon: React.ElementType;
  count?: number;
  active?: boolean;
}

export interface FileSidebarProps {
  items?: FileSidebarItem[];
  activeItemId?: string;
  onItemClick?: (itemId: string) => void;
  onNewFolder?: () => void;
  onCreateTextFile?: () => void;
  onUpload?: () => void;
  onFolderCreateChild?: (folder: FileFolder) => void;
  onFolderRename?: (folder: FileFolder) => void;
  onFolderDelete?: (folder: FileFolder) => void;
  workspaceId?: string;
  topContent?: React.ReactNode;
  flushTop?: boolean;
}

/**
 * Default sidebar items
 */
const DEFAULT_ITEMS: FileSidebarItem[] = [
  { id: 'all', labelKey: 'sidebar.allFiles', icon: Files },
  { id: 'needs_action', labelKey: 'sidebar.needsActionFiles', icon: Clock3 },
  { id: 'uploaded', labelKey: 'sidebar.uploadedFiles', icon: Upload },
  // { id: 'favorites', labelKey: 'favorites', icon: Heart }, // TODO: Temporarily disabled, may restore later
  { id: 'default', labelKey: 'sidebar.defaultFolders', icon: FolderOpen },
];

const SYSTEM_FILE_CATEGORIES = new Set(['all', 'needs_action', 'uploaded', 'default']);

/**
 * File sidebar component - displays navigation and storage info
 */
function FileSidebarBase({
  items = DEFAULT_ITEMS,
  activeItemId,
  onItemClick,
  onNewFolder,
  onUpload,
  onFolderCreateChild,
  onFolderRename,
  onFolderDelete,
  workspaceId,
  topContent,
  flushTop = false,
}: FileSidebarProps) {
  const t = useT();
  const { used: storageUsed, total: storageTotal, isLoading: isLoadingStorage } = useStorageUsage();
  const { folders, isLoading: isLoadingFolders } = useFileFolders(workspaceId);
  const storagePercentage = storageTotal > 0 ? (storageUsed / storageTotal) * 100 : 0;

  // Track which folders are expanded
  const [expandedFolders, setExpandedFolders] = useState<Set<string>>(new Set());

  useEffect(() => {
    if (!activeItemId || SYSTEM_FILE_CATEGORIES.has(activeItemId)) return;

    let ignore = false;

    const expandActiveAncestors = async () => {
      try {
        const ancestorIds: string[] = [];
        let currentId = activeItemId;

        while (currentId) {
          const response = await fileManageService.getFileFolder(currentId);
          const parentId = response.data?.parent_id;
          if (!parentId) break;

          ancestorIds.push(parentId);
          currentId = parentId;
        }

        if (ignore || ancestorIds.length === 0) return;

        setExpandedFolders(prev => {
          const next = new Set(prev);
          let changed = false;
          ancestorIds.forEach(id => {
            if (!next.has(id)) {
              next.add(id);
              changed = true;
            }
          });
          return changed ? next : prev;
        });
      } catch {
        // Keep the current expansion state if ancestor lookup fails.
      }
    };

    void expandActiveAncestors();

    return () => {
      ignore = true;
    };
  }, [activeItemId]);

  // Toggle folder expand/collapse
  const handleToggleExpand = useCallback((folderId: string) => {
    setExpandedFolders(prev => {
      const newSet = new Set(prev);
      if (newSet.has(folderId)) {
        newSet.delete(folderId);
      } else {
        newSet.add(folderId);
      }
      return newSet;
    });
  }, []);

  const viewItems = items.filter(item => item.id !== 'default');
  const rootItem = items.find(item => item.id === 'default');

  return (
    <aside className={cn('flex h-full w-full flex-col bg-background', flushTop ? 'pt-0' : 'pt-4')}>
      {topContent ? <div className="px-3 pb-2.5">{topContent}</div> : null}

      {/* Storage Info */}
      <div className={cn('space-y-2 px-4 pb-4', flushTop ? 'pt-4' : '')}>
        <div className="flex items-center justify-between mb-1.5">
          <h3 className="text-sm font-semibold text-foreground">{t('files.sidebar.storage')}</h3>
          <span className="text-sm font-medium text-muted-foreground">
            {isLoadingStorage ? '...' : `${storageUsed.toFixed(2)}GB / ${storageTotal}GB`}
          </span>
        </div>
        <Progress value={isLoadingStorage ? 0 : storagePercentage} className="h-1.5" />
      </div>

      {(onNewFolder || onUpload) && (
        <div className="space-y-2 px-4 pb-5">
          {onUpload && (
            <Button
              className="h-10 w-full justify-center gap-2 rounded-lg text-sm font-semibold shadow-sm shadow-primary/20"
              variant="default"
              onClick={onUpload}
            >
              <Upload className="h-4 w-4" />
              {t('files.sidebar.uploadFile')}
            </Button>
          )}
          {onNewFolder && (
            <Button
              className="h-10 w-full justify-center gap-2 rounded-lg text-sm font-semibold shadow-sm"
              variant="outline"
              onClick={onNewFolder}
            >
              <FolderPlus className="h-4 w-4" />
              {t('files.sidebar.newFolder')}
            </Button>
          )}
        </div>
      )}

      {/* Navigation Items */}
      <nav className="flex-1 space-y-5 overflow-y-auto border-t px-3 py-5">
        {/* Default Items */}
        <section className="space-y-1" aria-label={t('files.sidebar.viewsTitle')}>
          <p className="px-2 pb-2 text-xs font-medium text-muted-foreground">
            {t('files.sidebar.viewsTitle')}
          </p>
          {viewItems.map(item => {
            const Icon = item.icon;
            const isActive = activeItemId === item.id;

            return (
              <div key={item.id}>
                <div
                  className={cn(
                    'group flex h-9 w-full items-center justify-between rounded-lg px-3 text-sm font-medium transition-colors',
                    isActive
                      ? 'bg-muted text-primary'
                      : 'text-muted-foreground hover:bg-muted/60 hover:text-foreground'
                  )}
                >
                  <div
                    onClick={() => onItemClick?.(item.id)}
                    className="flex items-center gap-1.5 flex-1 min-w-0 cursor-pointer"
                  >
                    <Icon className="h-4 w-4 flex-shrink-0" />
                    <span className="truncate">{t(`files.${item.labelKey}`)}</span>
                  </div>
                  <div className="flex items-center gap-1">
                    {item.count !== undefined && (
                      <span
                        className={cn(
                          'text-[11px] flex-shrink-0',
                          isActive ? 'text-primary font-semibold' : 'text-muted-foreground'
                        )}
                      >
                        {item.count}
                      </span>
                    )}
                  </div>
                </div>
              </div>
            );
          })}
        </section>

        <section className="space-y-1" aria-label={t('files.sidebar.fileSpaceTitle')}>
          <div className="px-2 pb-2">
            <p className="text-xs font-medium text-muted-foreground">
              {t('files.sidebar.fileSpaceTitle')}
            </p>
          </div>
          {rootItem ? (
            <button
              type="button"
              className={cn(
                'flex h-9 w-full items-center gap-2 rounded-lg px-3 text-left text-sm font-medium transition-colors',
                activeItemId === rootItem.id
                  ? 'bg-muted text-primary'
                  : 'text-muted-foreground hover:bg-muted/60 hover:text-foreground'
              )}
              onClick={() => onItemClick?.(rootItem.id)}
            >
              <HardDrive className="h-4 w-4 shrink-0" />
              <span className="truncate">{t(`files.${rootItem.labelKey}`)}</span>
            </button>
          ) : null}

          <div className="space-y-0.5">
            {isLoadingFolders && (
              <>
                {[1, 2, 3].map(i => (
                  <div key={`skeleton-${i}`} className="flex w-full items-center gap-2 px-3 py-2">
                    <Skeleton className="h-4 w-4 flex-shrink-0" />
                    <Skeleton className="h-4 flex-1" />
                  </div>
                ))}
              </>
            )}

            {!isLoadingFolders &&
              folders.map(folder => (
                <FolderTreeNode
                  key={folder.id}
                  folder={folder}
                  level={0}
                  activeItemId={activeItemId}
                  onItemClick={onItemClick}
                  expandedFolders={expandedFolders}
                  onToggleExpand={handleToggleExpand}
                  maxLevel={MAX_FILE_FOLDER_TREE_LEVEL}
                  variant="sidebar"
                  onCreateChild={onFolderCreateChild}
                  onRename={onFolderRename}
                  onDelete={onFolderDelete}
                  workspaceId={workspaceId}
                />
              ))}
          </div>
        </section>
      </nav>
    </aside>
  );
}

export const FileSidebar = memo(FileSidebarBase);
