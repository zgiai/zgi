'use client';

import { useCallback } from 'react';
import {
  ChevronDown,
  ChevronUp,
  FolderInput,
  FolderOpen,
  FolderPlus,
  MoreHorizontal,
  Pencil,
  Trash2,
} from 'lucide-react';
import { cn } from '@/lib/utils';
import { useT } from '@/i18n';
import { useChildFolders } from '@/hooks/use-files';
import type { FileFolder } from '@/services/types/file';
import { Button } from '@/components/ui/button';
import {
  ContextMenu,
  ContextMenuContent,
  ContextMenuItem,
  ContextMenuTrigger,
} from '@/components/ui/context-menu';
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuTrigger,
} from '@/components/ui/dropdown-menu';
import { MAX_FILE_FOLDER_TREE_LEVEL } from './file-folder-levels';

/**
 * Folder Tree Node Props
 */
export interface FolderTreeNodeProps {
  folder: FileFolder;
  level: number;
  activeItemId?: string;
  onItemClick?: (itemId: string) => void;
  expandedFolders: Set<string>;
  onToggleExpand: (folderId: string) => void;
  maxLevel?: number; // 0-based maximum rendered folder level under Default Folder.
  variant?: 'sidebar' | 'dialog'; // UI variant
  onCreateChild?: (folder: FileFolder) => void;
  onRename?: (folder: FileFolder) => void;
  onMove?: (folder: FileFolder) => void;
  onDelete?: (folder: FileFolder) => void;
  workspaceId?: string;
}

/**
 * Folder Tree Node Component - Recursively renders folder and its children
 * Supports expandable/collapsible folders with lazy loading of children
 */
export function FolderTreeNode({
  folder,
  level,
  activeItemId,
  onItemClick,
  expandedFolders,
  onToggleExpand,
  maxLevel = MAX_FILE_FOLDER_TREE_LEVEL,
  variant = 'sidebar',
  onCreateChild,
  onRename,
  onMove,
  onDelete,
  workspaceId,
}: FolderTreeNodeProps) {
  const t = useT('files');
  const isFolderActive = activeItemId === folder.id;
  const isExpanded = expandedFolders.has(folder.id);

  const isMaxLevel = level >= maxLevel;
  const canCreateChild = !isMaxLevel && !!onCreateChild;
  const hasFolderActions = variant === 'sidebar' && (onCreateChild || onRename || onMove || onDelete);

  // Fetch child folders for expandable folders so empty folders do not show an expand icon.
  const { folders: childFolders, isLoading: isLoadingChildren } = useChildFolders(
    !isMaxLevel ? folder.id : undefined,
    workspaceId
  );
  const hasChildFolders = childFolders.length > 0;
  const canToggleExpand = !isMaxLevel && (hasChildFolders || isLoadingChildren);

  const handleClick = useCallback(() => {
    onItemClick?.(folder.id);

    if (!canToggleExpand) return;

    if (!isExpanded || isFolderActive) {
      onToggleExpand(folder.id);
    }
  }, [canToggleExpand, folder.id, isExpanded, isFolderActive, onItemClick, onToggleExpand]);

  const shouldShowArrow = !isMaxLevel && (hasChildFolders || isLoadingChildren);

  // Styling based on variant
  const paddingLeft = variant === 'sidebar' ? level * 14 + 12 : level * 16 + 12;
  const iconSize = variant === 'sidebar' ? 'h-4 w-4' : 'h-5 w-5';
  const textSize = variant === 'sidebar' ? 'text-sm' : 'text-sm';
  const padding = variant === 'sidebar' ? 'px-3 py-2' : 'px-3 py-2.5';

  const renderActionItems = (Item: typeof ContextMenuItem | typeof DropdownMenuItem) => (
    <>
      <Item
        disabled={!canCreateChild}
        onSelect={() => {
          if (canCreateChild) {
            onCreateChild?.(folder);
          }
        }}
      >
        <FolderPlus className="size-4" />
        {t('folder.actions.createChild')}
      </Item>
      <Item onSelect={() => onRename?.(folder)} disabled={!onRename}>
        <Pencil className="size-4" />
        {t('folder.actions.rename')}
      </Item>
      <Item onSelect={() => onMove?.(folder)} disabled={!onMove}>
        <FolderInput className="size-4" />
        {t('folder.actions.moveTo')}
      </Item>
      <Item
        onSelect={() => onDelete?.(folder)}
        disabled={!onDelete}
        className="text-destructive focus:bg-destructive/10 focus:text-destructive"
      >
        <Trash2 className="size-4 text-destructive" />
        {t('folder.actions.delete')}
      </Item>
    </>
  );

  const folderRow = (
    <div
      className={cn(
        'w-full flex items-center justify-between rounded-md font-medium transition-colors group cursor-pointer',
        padding,
        textSize,
        variant === 'sidebar'
          ? isFolderActive
            ? 'bg-muted text-primary'
            : 'text-muted-foreground hover:bg-muted/60 hover:text-foreground'
          : isFolderActive
            ? 'bg-muted text-primary hover:bg-muted'
            : 'hover:bg-gray-100 text-gray-700'
      )}
      style={{ paddingLeft: `${paddingLeft}px` }}
      role="button"
      tabIndex={0}
      onClick={handleClick}
      onKeyDown={event => {
        if (event.key === 'Enter' || event.key === ' ') {
          event.preventDefault();
          handleClick();
        }
      }}
    >
      <div className="flex items-center gap-1.5 flex-1 min-w-0">
        {shouldShowArrow &&
          (isLoadingChildren ? (
            <div className={cn('flex items-center justify-center', iconSize)}>
              <div className="h-2 w-2 rounded-full bg-gray-400 animate-pulse" />
            </div>
          ) : (
            <div className="flex-shrink-0 hover:bg-gray-200 rounded p-0.5">
              {isExpanded ? (
                <ChevronDown className={iconSize} />
              ) : (
                <ChevronUp className={iconSize} />
              )}
            </div>
          ))}
        <FolderOpen
          className={cn(iconSize, 'flex-shrink-0', variant === 'dialog' && 'text-gray-500')}
        />
        <span className={cn('truncate', variant === 'dialog' && 'flex-1')}>{folder.name}</span>
      </div>

      {hasFolderActions ? (
        <DropdownMenu>
          <DropdownMenuTrigger asChild>
            <Button
              variant="ghost"
              isIcon
              className="h-6 w-6 p-0 opacity-0 transition-opacity hover:bg-muted group-hover:opacity-100 data-[state=open]:opacity-100"
              onClick={event => {
                event.stopPropagation();
              }}
            >
              <MoreHorizontal className="h-3.5 w-3.5" />
            </Button>
          </DropdownMenuTrigger>
          <DropdownMenuContent align="end" className="w-40 rounded-xl p-1.5">
            {renderActionItems(DropdownMenuItem)}
          </DropdownMenuContent>
        </DropdownMenu>
      ) : null}
    </div>
  );

  return (
    <div>
      {hasFolderActions ? (
        <ContextMenu>
          <ContextMenuTrigger asChild>{folderRow}</ContextMenuTrigger>
          <ContextMenuContent className="w-40 rounded-xl p-1.5">
            {renderActionItems(ContextMenuItem)}
          </ContextMenuContent>
        </ContextMenu>
      ) : (
        folderRow
      )}

      {/* Render children when expanded - child folders will also display settings and delete buttons */}
      {isExpanded && hasChildFolders && (
        <div className="space-y-1">
          {!isLoadingChildren &&
            childFolders.map(childFolder => (
              <FolderTreeNode
                key={childFolder.id}
                folder={childFolder}
                level={level + 1}
                activeItemId={activeItemId}
                onItemClick={onItemClick}
                expandedFolders={expandedFolders}
                onToggleExpand={onToggleExpand}
                maxLevel={maxLevel}
                variant={variant}
                onCreateChild={onCreateChild}
                onRename={onRename}
                onMove={onMove}
                onDelete={onDelete}
                workspaceId={workspaceId}
              />
            ))}
        </div>
      )}
    </div>
  );
}
