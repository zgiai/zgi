'use client';

import React, { useMemo, useState, useEffect, useRef, useCallback } from 'react';
import { useT } from '@/i18n';
import {
  Dialog,
  DialogContent,
  DialogFooter,
  DialogHeader,
  DialogTitle,
  DialogBody,
} from '@/components/ui/dialog';
import { cn } from '@/lib/utils';
import { Button } from '@/components/ui/button';
import { Input } from '@/components/ui/input';
import { Skeleton } from '@/components/ui/skeleton';
import { ScrollArea } from '@/components/ui/scroll-area';
import { useDebouncedValue } from '@/hooks/use-debounced-value';
import { useDatasetFolders, useMoveDatasetToFolder } from '@/hooks/dataset/use-dataset-folders';
import VirtualList from '@/components/common/virtual-list';
import type { DatasetFolder } from '@/services/types/dataset-folder';
import { Folder } from 'lucide-react';
import { useCurrentWorkspace } from '@/store/workspace-store';

interface MoveDatasetModalProps {
  /** Controls dialog open state */
  open: boolean;
  /** Open state change handler */
  onOpenChange: (open: boolean) => void;
  /** The dataset id to move */
  datasetId: string;
  /** Optional current folder id to prevent no-op moves */
  currentFolderId?: string;
}

interface FolderRowProps {
  folder: DatasetFolder;
  selected: boolean;
  onSelect: (id: string) => void;
}

const FolderRow = React.memo(function FolderRow({ folder, selected, onSelect }: FolderRowProps) {
  return (
    <div
      role="option"
      aria-selected={selected}
      tabIndex={0}
      onClick={() => onSelect(folder.id)}
      onKeyDown={e => {
        if (e.key === 'Enter' || e.key === ' ') onSelect(folder.id);
      }}
      className={cn(
        'flex items-center justify-between px-4 py-3.5 cursor-pointer select-none transition-colors border-l-2',
        selected
          ? 'bg-blue-50/30 border-blue-500'
          : 'border-transparent hover:bg-neutral-50 active:bg-neutral-100'
      )}
      title={folder.name}
    >
      <div className="flex items-center gap-3 min-w-0">
        <Folder
          size={18}
          className={cn(
            'shrink-0 transition-colors',
            selected ? 'text-blue-500' : 'text-neutral-400'
          )}
        />
        <div
          className={cn(
            'truncate text-sm font-medium transition-colors',
            selected ? 'text-blue-700' : 'text-neutral-600'
          )}
        >
          {folder.name || '-'}
        </div>
      </div>
      {selected && <div className="h-1.5 w-1.5 rounded-full bg-blue-500 shadow-sm shadow-blue-500/50" />}
    </div>
  );
});

/**
 * Modal for moving a dataset into a folder or root.
 * - Fetches folders via useDatasetFolders
 * - Performs move with useMoveDatasetToFolder
 * - Provides local search with debounce for better UX
 */
export default function MoveDatasetModal({
  open,
  onOpenChange,
  datasetId,
  currentFolderId,
}: MoveDatasetModalProps) {
  const t = useT('datasets');
  const currentWorkspace = useCurrentWorkspace();
  const [query, setQuery] = useState<string>('');
  const debouncedQuery = useDebouncedValue(query, 200);
  const [selectedFolderId, setSelectedFolderId] = useState<string | null>(null);

  // Use server-side keyword filtering to reduce payload; only fetch when dialog is open
  const {
    data: folders,
    isLoading,
    isFetching,
  } = useDatasetFolders({
    enabled: open,
    keyword: debouncedQuery,
    workspace_id: currentWorkspace?.id,
  });
  const moveMutation = useMoveDatasetToFolder();

  // ScrollArea viewport ref for virtualization
  const viewportRef = useRef<HTMLDivElement | null>(null);

  // Preselect current folder when opening
  useEffect(() => {
    if (open) {
      setSelectedFolderId(currentFolderId ?? null);
    }
  }, [open, currentFolderId]);

  // Compute filtered list by name with case-insensitive matching (client-side fallback)
  const filteredFolders = useMemo<DatasetFolder[]>(() => {
    const list = (folders ?? []) as DatasetFolder[];
    const q = debouncedQuery.trim().toLowerCase();
    if (!q) return list;
    return list.filter(f => (f.name || '').toLowerCase().includes(q));
  }, [folders, debouncedQuery]);

  // Reset scroll to top on query change or dialog reopen
  useEffect(() => {
    const el = viewportRef.current;
    if (!el) return;
    el.scrollTop = 0;
  }, [debouncedQuery, open]);

  // Whether selected target equals current folder (including root)
  const isSameTarget = useMemo(() => {
    if (selectedFolderId === null) return false;
    if (currentFolderId === undefined) return false; // unknown current folder, allow move
    const current = currentFolderId || ''; // empty string means root
    const target = selectedFolderId || '';
    return target === current;
  }, [selectedFolderId, currentFolderId]);

  const canConfirm = selectedFolderId !== null && !isSameTarget && !moveMutation.isPending;

  const handleConfirm = () => {
    if (selectedFolderId === null) return;
    const folder_id = selectedFolderId || '';
    moveMutation.mutate(
      { dataset_id: datasetId, folder_id, source_folder_id: currentFolderId ?? '' },
      {
        onSuccess: () => {
          onOpenChange(false);
          setSelectedFolderId(null);
          setQuery('');
        },
      }
    );
  };

  const handleClose = (next: boolean) => {
    if (moveMutation.isPending && next === false) return;
    onOpenChange(next);
    if (!next) {
      setSelectedFolderId(null);
      setQuery('');
    }
  };

  // Virtualization enable threshold (fallback for small lists)
  const enableVirtual = filteredFolders.length > 200;

  // Stable select handler to avoid re-creating function per item
  const chooseFolder = useCallback((id: string) => {
    setSelectedFolderId(id);
  }, []);

  // Stable renderItem and itemKey to avoid re-renders
  const itemKey = useCallback((item: DatasetFolder, _index: number) => item.id, []);
  const renderItem = (folder: DatasetFolder, _index: number) => {
    const selected = selectedFolderId === folder.id;
    return <FolderRow folder={folder} selected={selected} onSelect={chooseFolder} />;
  };

  return (
    <Dialog open={open} onOpenChange={handleClose}>
      <DialogContent className="max-w-[500px] p-0 overflow-hidden flex flex-col">
        <DialogHeader>
          <DialogTitle className="text-xl font-bold tracking-tight">
            {t('moveModal.title')}
          </DialogTitle>
        </DialogHeader>

        <DialogBody className="space-y-6 py-6 flex-1">
          <div className="space-y-6">
            <div className="px-1">
              <Input
                value={query}
                onChange={e => setQuery(e.target.value)}
                placeholder={t('moveModal.searchPlaceholder')}
                className="h-11 shadow-sm"
              />
            </div>

            <div className="space-y-3">
              <div className="text-xs font-bold text-muted-foreground uppercase tracking-wider px-1">
                {t('moveModal.moveToRoot')}
              </div>
              {/* Root option */}
              <div
                role="button"
                tabIndex={0}
                onClick={() => setSelectedFolderId('')}
                onKeyDown={e => {
                  if (e.key === 'Enter' || e.key === ' ') setSelectedFolderId('');
                }}
                className={cn(
                  'flex items-center justify-between rounded-xl border p-4 cursor-pointer transition-all duration-200',
                  selectedFolderId === ''
                    ? 'border-blue-500 bg-blue-50/50 shadow-sm'
                    : 'border-neutral-100 hover:border-neutral-200 hover:bg-neutral-50'
                )}
                aria-pressed={selectedFolderId === ''}
              >
                <div className="flex items-center gap-3">
                  <div
                    className={cn(
                      'p-2 rounded-lg transition-colors',
                      selectedFolderId === ''
                        ? 'bg-blue-100 text-blue-600'
                        : 'bg-neutral-100 text-neutral-500'
                    )}
                  >
                    <Folder size={20} />
                  </div>
                  <span className="text-sm font-bold">{t('moveModal.moveToRoot')}</span>
                </div>
                {selectedFolderId === '' && (
                  <div className="h-2 w-2 rounded-full bg-blue-500 shadow-sm shadow-blue-500/50" />
                )}
              </div>
            </div>

            <div className="space-y-3">
              <div className="text-xs font-bold text-muted-foreground uppercase tracking-wider px-1">
                {t('folder')}
              </div>
              {/* Folders list */}
              <div className="min-h-[200px]">
                {isLoading ? (
                  <div className="space-y-3 px-1">
                    <Skeleton className="h-14 w-full rounded-xl" />
                    <Skeleton className="h-14 w-full rounded-xl" />
                    <Skeleton className="h-14 w-full rounded-xl" />
                  </div>
                ) : (
                  <div className="rounded-xl border border-neutral-100 overflow-hidden bg-white shadow-sm">
                    <ScrollArea className="max-h-64" viewportRef={viewportRef}>
                      {filteredFolders.length === 0 ? (
                        <div className="py-12 text-center">
                          <div className="text-sm font-medium text-muted-foreground">
                            {t('moveModal.empty')}
                          </div>
                        </div>
                      ) : (
                        <VirtualList<DatasetFolder>
                          items={filteredFolders}
                          itemKey={itemKey}
                          renderItem={renderItem}
                          estimateSize={56}
                          overscan={10}
                          scrollElementRef={viewportRef as React.RefObject<HTMLElement>}
                          disabled={!enableVirtual}
                          role="listbox"
                          emptyPlaceholder={
                            <div className="py-12 text-center text-muted-foreground">
                              {t('moveModal.empty')}
                            </div>
                          }
                        />
                      )}
                      {isFetching && !isLoading && (
                        <div className="py-3 text-center border-t border-neutral-50">
                          <span className="text-[10px] uppercase tracking-widest font-bold text-muted-foreground animate-pulse">
                            {t('moveModal.loading')}
                          </span>
                        </div>
                      )}
                    </ScrollArea>
                  </div>
                )}
              </div>
            </div>
          </div>
        </DialogBody>

        <DialogFooter className="bg-neutral-50/50 pt-4 pb-6 px-6 border-t font-medium">
          <Button
            variant="ghost"
            onClick={() => handleClose(false)}
            disabled={moveMutation.isPending}
            className="font-semibold"
          >
            {t('moveModal.cancel')}
          </Button>
          <Button
            onClick={handleConfirm}
            disabled={!canConfirm}
            size="lg"
            className="px-10 font-bold"
          >
            {t('moveModal.confirm')}
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  );
}
