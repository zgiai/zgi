import { memo, useState, useCallback } from 'react';
import { FolderPlus, Upload, Files, FolderOpen } from 'lucide-react';
import { useT, type FilesSuffix } from '@/i18n';
import { Button } from '@/components/ui/button';
import { Progress } from '@/components/ui/progress';
import { Skeleton } from '@/components/ui/skeleton';
import { cn } from '@/lib/utils';
import { useStorageUsage, useFileFolders } from '@/hooks/use-files';
import { FolderTreeNode } from './folder-tree-node';
import type { FileFolder } from '@/services/types/file';

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
  onUpload?: () => void;
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
  { id: 'uploaded', labelKey: 'sidebar.uploadedFiles', icon: Upload },
  // { id: 'favorites', labelKey: 'favorites', icon: Heart }, // TODO: Temporarily disabled, may restore later
  { id: 'default', labelKey: 'sidebar.defaultFolders', icon: FolderOpen },
];

/**
 * File sidebar component - displays navigation and storage info
 */
function FileSidebarBase({
  items = DEFAULT_ITEMS,
  activeItemId,
  onItemClick,
  onNewFolder,
  onUpload,
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

  return (
    <aside className={cn('flex h-full w-full flex-col bg-background', flushTop ? 'pt-0' : 'pt-4')}>
      {topContent ? <div className="px-3 pb-2.5">{topContent}</div> : null}

      {/* Storage Info */}
      <div className={cn('mb-3 space-y-1.5 px-3', flushTop ? 'pt-3' : '')}>
        <div className="flex items-center justify-between mb-1.5">
          <h3 className="text-xs font-semibold">{t('files.sidebar.storage')}</h3>
          <span className="text-[11px] text-muted-foreground">
            {isLoadingStorage ? '...' : `${storageUsed.toFixed(1)}GB / ${storageTotal}GB`}
          </span>
        </div>
        <Progress value={isLoadingStorage ? 0 : storagePercentage} className="h-1.5" />
      </div>

      {(onNewFolder || onUpload) && (
        <div className="space-y-1.5 px-3 pb-3">
          {onNewFolder && (
            <Button
              className="w-full gap-1.5 text-xs"
              variant="outline"
              size="sm"
              onClick={onNewFolder}
            >
              <FolderPlus className="h-4 w-4" />
              {t('files.sidebar.newFolder')}
            </Button>
          )}
          {onUpload && (
            <Button
              className="w-full gap-1.5 text-xs"
              variant="default"
              size="sm"
              onClick={onUpload}
            >
              <Upload className="h-4 w-4" />
              {t('files.sidebar.uploadFile')}
            </Button>
          )}
        </div>
      )}

      {/* Navigation Items */}
      <nav className="flex-1 space-y-0.5 overflow-y-auto border-t px-3 py-3">
        {/* Default Items */}
        {items.map(item => {
          const Icon = item.icon;
          const isActive = activeItemId === item.id;

          return (
            <div key={item.id}>
              <div
                className={cn(
                  'w-full flex items-center justify-between px-2 py-1.5 rounded-md text-xs font-medium transition-colors group',
                  isActive ? 'bg-muted text-primary' : 'text-gray-700 hover:bg-gray-50'
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

              {/* Show folders under "defaultFolders" item */}
              {item.id === 'default' && (
                <div className="ml-1.5 mt-0.5 space-y-0.5">
                  {/* Loading Skeleton for Folders */}
                  {isLoadingFolders && (
                    <>
                      {[1, 2, 3].map(i => (
                        <div
                          key={`skeleton-${i}`}
                          className="w-full flex items-center gap-1.5 px-2 py-1.5"
                        >
                          <Skeleton className="h-4 w-4 flex-shrink-0" />
                          <Skeleton className="h-4 flex-1" />
                        </div>
                      ))}
                    </>
                  )}

                  {/* Folder Tree Items */}
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
                        maxLevel={1}
                        variant="sidebar"
                        onDelete={onFolderDelete}
                        workspaceId={workspaceId}
                      />
                    ))}
                </div>
              )}
            </div>
          );
        })}
      </nav>
    </aside>
  );
}

export const FileSidebar = memo(FileSidebarBase);
