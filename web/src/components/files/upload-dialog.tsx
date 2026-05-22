'use client';

import { useState, useCallback } from 'react';
import { useT } from '@/i18n';
import { FolderOpen } from 'lucide-react';
import { Button } from '@/components/ui/button';
import {
  Dialog,
  DialogContent,
  DialogHeader,
  DialogTitle,
  DialogBody,
  DialogFooter,
} from '@/components/ui/dialog';
import { RadioCard, RadioCardGroup } from '@/components/ui/radio-card';
import { Label } from '@/components/ui/label';
import { Skeleton } from '@/components/ui/skeleton';
import { cn } from '@/lib/utils';
import { useFileFolders } from '@/hooks/use-files';
import { FolderTreeNode } from './folder-tree-node';
import {
  WorkspaceSelector,
  type WorkspaceSelectorValue,
} from '@/components/common/workspace-selector';
import { useCurrentWorkspace, useIsOrganizationMode } from '@/store';

/**
 * Upload mode type
 */
export type UploadMode = 'file' | 'text';

/**
 * Upload configuration
 */
export interface UploadConfig {
  mode: UploadMode;
  folderId: string;
  workspaceId: string;
}

/**
 * Upload dialog props
 */
export interface UploadDialogProps {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  onConfirm: (config: UploadConfig) => void;
}

/**
 * File Upload Dialog Component
 * Allows users to select storage location and upload source type
 */
export function UploadDialog({ open, onOpenChange, onConfirm }: UploadDialogProps) {
  const t = useT();
  const currentWorkspace = useCurrentWorkspace();
  const isOrganizationMode = useIsOrganizationMode();
  const [selectedWorkspace, setSelectedWorkspace] = useState<WorkspaceSelectorValue | undefined>();
  const effectiveWorkspaceId = isOrganizationMode ? selectedWorkspace?.id : currentWorkspace?.id;
  const { folders, isLoading: isFoldersLoading } = useFileFolders(effectiveWorkspaceId, {
    enabled: !isOrganizationMode || !!effectiveWorkspaceId,
  });

  // Local state
  const [addMode, setAddMode] = useState<UploadMode>('file');
  const [selectedFolderId, setSelectedFolderId] = useState<string>('');
  const [expandedFolders, setExpandedFolders] = useState<Set<string>>(new Set());

  const handleWorkspaceChange = useCallback((workspace: WorkspaceSelectorValue) => {
    setSelectedWorkspace(workspace);
    setSelectedFolderId('');
    setExpandedFolders(new Set());
  }, []);

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

  // Handle confirm
  const handleConfirm = () => {
    if (!effectiveWorkspaceId) {
      return;
    }

    onConfirm({
      mode: addMode,
      folderId: selectedFolderId,
      workspaceId: effectiveWorkspaceId,
    });
    // Reset state after confirm
    setAddMode('file');
    setSelectedFolderId('');
    setSelectedWorkspace(undefined);
  };

  // Handle cancel
  const handleCancel = () => {
    onOpenChange(false);
    // Reset state after a short delay to avoid visual glitch
    setTimeout(() => {
      setAddMode('file');
      setSelectedFolderId('');
      setSelectedWorkspace(undefined);
      setExpandedFolders(new Set());
    }, 200);
  };

  const canContinue = !isOrganizationMode || !!effectiveWorkspaceId;

  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent className="max-w-[560px] p-0 overflow-hidden">
        <DialogHeader className="pb-2">
          <DialogTitle className="text-xl font-bold tracking-tight">
            {t('files.upload.selectSource')}
          </DialogTitle>
        </DialogHeader>

        <DialogBody className="space-y-8 py-6">
          {/* Source Type Selection */}
          <div className="space-y-3">
            <Label className="text-sm font-semibold">{t('files.upload.sourceType')}</Label>
            <RadioCardGroup
              value={addMode}
              onValueChange={value => setAddMode(value as UploadMode)}
              className="grid grid-cols-2 gap-4"
            >
              <RadioCard
                value="file"
                title={t('datasets.documents.addMode.local.title')}
                description={t('datasets.documents.addMode.local.desc')}
                className="h-full"
              />
              <RadioCard
                value="text"
                title={t('datasets.documents.addMode.text.title')}
                description={t('datasets.documents.addMode.text.desc')}
                className="h-full"
              />
            </RadioCardGroup>
          </div>

          {isOrganizationMode ? (
            <div className="space-y-3">
              <Label className="text-sm font-semibold">{t('files.upload.workspaceLabel')}</Label>
              <WorkspaceSelector
                value={selectedWorkspace}
                placeholder={t('files.upload.workspacePlaceholder')}
                autoSelectFirst
                onChange={handleWorkspaceChange}
              />
            </div>
          ) : null}

          {/* Storage Location Selection */}
          <div className="space-y-3">
            <Label className="text-sm font-semibold">{t('files.upload.storageLocation')}</Label>
            <div className="border rounded-xl bg-neutral-50/50 p-2 max-h-[280px] overflow-y-auto shadow-inner">
              {isOrganizationMode && !effectiveWorkspaceId ? (
                <div className="px-3 py-6 text-center text-sm text-muted-foreground">
                  {t('files.upload.workspaceRequired')}
                </div>
              ) : isFoldersLoading ? (
                // Loading skeleton
                <div className="space-y-1">
                  {[1, 2, 3].map(i => (
                    <div key={`skeleton-${i}`} className="flex items-center gap-3 p-3">
                      <Skeleton className="h-5 w-5 rounded" />
                      <Skeleton className="h-4 flex-1" />
                    </div>
                  ))}
                </div>
              ) : (
                <div className="space-y-1">
                  {/* Default option - root folder */}
                  <button
                    type="button"
                    onClick={() => setSelectedFolderId('')}
                    className={cn(
                      'w-full flex items-center gap-3 px-3 py-2.5 rounded-lg text-sm transition-all text-left group',
                      selectedFolderId === ''
                        ? 'bg-white shadow-sm ring-1 ring-neutral-200 text-primary'
                        : 'hover:bg-neutral-100 text-muted-foreground'
                    )}
                  >
                    <FolderOpen
                      className={cn(
                        'size-5 flex-shrink-0 transition-colors',
                        selectedFolderId === ''
                          ? 'text-primary'
                          : 'text-neutral-400 group-hover:text-neutral-600'
                      )}
                    />
                    <span className="flex-1 truncate font-semibold">
                      {t('files.upload.defaultFolder')}
                    </span>
                  </button>

                  {/* Folder tree - as children of root */}
                  <div className="pl-4 mt-1">
                    {folders.map(folder => (
                      <FolderTreeNode
                        key={folder.id}
                        folder={folder}
                        level={0}
                        activeItemId={selectedFolderId}
                        onItemClick={setSelectedFolderId}
                        expandedFolders={expandedFolders}
                        onToggleExpand={handleToggleExpand}
                        maxLevel={1}
                        variant="dialog"
                        workspaceId={folder.workspace_id}
                      />
                    ))}
                  </div>
                </div>
              )}
            </div>
          </div>
        </DialogBody>

        <DialogFooter className="bg-neutral-50/50 pt-4 pb-6">
          <Button variant="ghost" onClick={handleCancel} className="font-semibold">
            {t('common.cancel')}
          </Button>
          <Button
            onClick={handleConfirm}
            size="lg"
            className="px-10 font-bold"
            disabled={!canContinue}
          >
            {t('datasets.actions.continue')}
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  );
}
