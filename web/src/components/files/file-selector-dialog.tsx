'use client';

import { useState, useCallback, useEffect, useRef } from 'react';
import { useT } from '@/i18n';
import { Button } from '@/components/ui/button';
import { X } from 'lucide-react';
import { useIsMobile } from '@/hooks/use-mobile';
import { cn } from '@/lib/utils';
import {
  Dialog,
  DialogContent,
  DialogHeader,
  DialogTitle,
  DialogFooter,
  DialogBody,
} from '@/components/ui/dialog';
import { toast } from 'sonner';
import FileManagementContent from '@/components/files/file-management-content';
import type { FileItem } from '@/services/types/file';

export interface FileSelectorDialogProps {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  onConfirm: (selectedFiles: FileItem[]) => void;
  /** Initial selected files with full FileItem data for cross-page selection */
  initSelectedFiles?: FileItem[];
  maxCount?: number;
  acceptExt?: string[];
  allowWorkspaceSwitch?: boolean;
  confirmDisabled?: boolean;
  footerExtra?: React.ReactNode;
}

const FileSelectorDialog = ({
  open,
  onOpenChange,
  onConfirm,
  initSelectedFiles = [],
  maxCount,
  acceptExt = [],
  allowWorkspaceSwitch = false,
  confirmDisabled = false,
  footerExtra,
}: FileSelectorDialogProps): React.ReactNode => {
  const isMobile = useIsMobile();
  // Initialize state from initial files
  const [selectedFileIds, setSelectedFileIds] = useState<string[]>(() =>
    initSelectedFiles.map(f => f.id)
  );
  const [selectedFilesMap, setSelectedFilesMap] = useState<Map<string, FileItem>>(() => {
    const map = new Map<string, FileItem>();
    initSelectedFiles.forEach(file => map.set(file.id, file));
    return map;
  });
  const t = useT();

  const prevInitSelectedFilesRef = useRef<FileItem[]>(initSelectedFiles);

  // Sync state when initSelectedFiles prop changes
  useEffect(() => {
    const prevFiles = prevInitSelectedFilesRef.current;
    const prevIds = prevFiles.map(f => f.id);
    const newIds = initSelectedFiles.map(f => f.id);

    const propHasChanged =
      prevIds.length !== newIds.length || !prevIds.every((id, idx) => newIds[idx] === id);

    if (propHasChanged) {
      setSelectedFileIds(newIds);

      // Rebuild the map with new initial files
      const newMap = new Map<string, FileItem>();
      initSelectedFiles.forEach(file => newMap.set(file.id, file));
      setSelectedFilesMap(newMap);

      prevInitSelectedFilesRef.current = initSelectedFiles;
    }
  }, [initSelectedFiles]);

  const handleFilesChange = useCallback(
    (files: FileItem[]) => {
      setSelectedFilesMap(prev => {
        const newMap = new Map(prev);
        files.forEach(file => {
          if (selectedFileIds.includes(file.id)) {
            newMap.set(file.id, file);
          }
        });
        return newMap;
      });
    },
    [selectedFileIds]
  );

  const handleSelectionChange = useCallback(
    (selectedIds: string[], currentPageFiles: FileItem[]) => {
      setSelectedFileIds(current => {
        if (
          current.length === selectedIds.length &&
          current.every((id, idx) => selectedIds[idx] === id)
        ) {
          return current;
        }

        if (maxCount !== undefined) {
          if (selectedIds.length > maxCount) {
            const limited = selectedIds.slice(0, maxCount);
            if (
              current.length !== limited.length ||
              !limited.every((id, idx) => current[idx] === id)
            ) {
              toast.error(t('files.maxCountExceeded', { max: maxCount }));
              return limited;
            }
            return current;
          }
          if (current.length >= maxCount && selectedIds.length > current.length) {
            const newIds = selectedIds.filter(id => !current.includes(id));
            if (newIds.length > 0) {
              toast.error(t('files.maxCountExceeded', { max: maxCount }));
            }
            return current;
          }
        }

        // Update the selected files map
        setSelectedFilesMap(prev => {
          const newMap = new Map(prev);

          // Add newly selected files from current page
          const newlySelected = selectedIds.filter(id => !current.includes(id));
          newlySelected.forEach(id => {
            const file = currentPageFiles.find(f => f.id === id);
            if (file) {
              newMap.set(id, file);
            }
          });

          // Remove unselected files
          const unselected = current.filter(id => !selectedIds.includes(id));
          unselected.forEach(id => {
            newMap.delete(id);
          });

          return newMap;
        });

        return selectedIds;
      });
    },
    [maxCount, t]
  );

  const handleConfirm = useCallback(() => {
    if (selectedFileIds.length === 0) {
      return;
    }

    if (maxCount !== undefined && selectedFileIds.length > maxCount) {
      toast.error(t('files.maxCountExceeded', { max: maxCount }));
      return;
    }

    const selectedFiles = selectedFileIds
      .map(id => selectedFilesMap.get(id))
      .filter((file): file is FileItem => file !== undefined);

    onConfirm(selectedFiles);
    onOpenChange(false);
  }, [selectedFileIds, selectedFilesMap, maxCount, onConfirm, onOpenChange, t]);

  const handleCancel = useCallback(() => {
    onOpenChange(false);
  }, [onOpenChange]);

  const selectedCount = selectedFileIds.length;
  const canConfirm = selectedCount > 0 && !confirmDisabled;

  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent
        showCloseButton={false}
        className={cn(
          'flex flex-col overflow-hidden p-0 gap-0',
          isMobile
            ? 'left-0 top-0 h-[100dvh] max-h-[100dvh] w-screen max-w-none translate-x-0 translate-y-0 rounded-none border-0'
            : 'max-w-[90vw] h-[90vh] max-h-[90vh] rounded-xl'
        )}
      >
        <DialogHeader
          className={cn(
            'flex-row items-center justify-between space-y-0 border-b flex-shrink-0',
            isMobile ? 'px-4 py-3' : 'px-5 py-2.5'
          )}
        >
          <DialogTitle className="text-base font-semibold">{t('files.selectFiles')}</DialogTitle>
          <Button
            isIcon
            variant="ghost"
            className={cn('rounded-full', isMobile ? 'size-9' : 'size-8')}
            onClick={handleCancel}
            aria-label={t('common.close')}
          >
            <X className="size-4" />
          </Button>
        </DialogHeader>

        <DialogBody className="flex flex-col overflow-hidden p-0">
          <div className="flex flex-1 flex-col overflow-hidden min-h-0">
            <FileManagementContent
              selectionMode
              selectedFileIds={selectedFileIds}
              onSelectionChange={handleSelectionChange}
              onFilesChange={handleFilesChange}
              maxCount={maxCount}
              acceptExt={acceptExt}
              allowWorkspaceSwitch={allowWorkspaceSwitch}
            />
          </div>
        </DialogBody>
        <DialogFooter
          className={cn(
            'border-t gap-2',
            isMobile ? 'grid grid-cols-2 px-4 py-3' : 'items-center px-6 py-4'
          )}
        >
          {footerExtra && (
            <div className={cn(isMobile ? 'col-span-2' : 'mr-auto')}>{footerExtra}</div>
          )}
          <Button variant="outline" onClick={handleCancel}>
            {t('common.cancel')}
          </Button>
          <Button onClick={handleConfirm} disabled={!canConfirm}>
            {t('files.confirmSelect', { count: selectedCount })}
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  );
};

export default FileSelectorDialog;
