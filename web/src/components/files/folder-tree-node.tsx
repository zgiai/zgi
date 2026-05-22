'use client';

import { useCallback } from 'react';
import { FolderOpen, ChevronDown, ChevronUp, Trash2 } from 'lucide-react';
import { cn } from '@/lib/utils';
import { useChildFolders } from '@/hooks/use-files';
import type { FileFolder } from '@/services/types/file';
import { Button } from '@/components/ui/button';

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
  maxLevel?: number; // Maximum depth level, default is 1 (2 levels)
  variant?: 'sidebar' | 'dialog'; // UI variant
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
  maxLevel = 1,
  variant = 'sidebar',
  onDelete,
  workspaceId,
}: FolderTreeNodeProps) {
  const isFolderActive = activeItemId === folder.id;
  const isExpanded = expandedFolders.has(folder.id);

  const isMaxLevel = level >= maxLevel;

  // Fetch child folders when this folder is clicked/expanded (only if not at max level)
  const { folders: childFolders, isLoading: isLoadingChildren } = useChildFolders(
    !isMaxLevel && isExpanded ? folder.id : undefined,
    workspaceId
  );

  const handleClick = useCallback(() => {
    onItemClick?.(folder.id);
    // Toggle expand/collapse when clicking folder (only if not at max level)
    if (!isMaxLevel) {
      onToggleExpand(folder.id);
    }
  }, [folder.id, onItemClick, isMaxLevel, onToggleExpand]);

  // Keep expandable folder affordance visible before children are lazy-loaded.
  const shouldShowArrow = !isMaxLevel;

  // Styling based on variant
  const paddingLeft = variant === 'sidebar' ? level * 10 + 10 : level * 16 + 12;
  const iconSize = variant === 'sidebar' ? 'h-4 w-4' : 'h-5 w-5';
  const textSize = variant === 'sidebar' ? 'text-xs' : 'text-sm';
  const padding = variant === 'sidebar' ? 'px-2 py-1.5' : 'px-3 py-2.5';

  return (
    <div>
      <div
        className={cn(
          'w-full flex items-center justify-between rounded-md font-medium transition-colors group cursor-pointer',
          padding,
          textSize,
          variant === 'sidebar'
            ? isFolderActive
              ? 'bg-muted text-primary'
              : 'text-gray-700 hover:bg-gray-50'
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
            (isExpanded && isLoadingChildren ? (
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

        {/* Action buttons - show delete button for both parent and child folders in sidebar variant */}
        {variant === 'sidebar' && onDelete && (
          <div className="flex items-center gap-1 opacity-0 group-hover:opacity-100 transition-opacity">
            {onDelete && (
              <Button
                variant="ghost"
                isIcon
                className="h-6 w-6 p-0 hover:bg-red-100"
                onClick={e => {
                  e.stopPropagation();
                  onDelete(folder);
                }}
              >
                <Trash2 className="h-3.5 w-3.5 text-red-600" />
              </Button>
            )}
          </div>
        )}
      </div>

      {/* Render children when expanded - child folders will also display settings and delete buttons */}
      {isExpanded && (
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
                onDelete={onDelete}
                workspaceId={workspaceId}
              />
            ))}
        </div>
      )}
    </div>
  );
}
