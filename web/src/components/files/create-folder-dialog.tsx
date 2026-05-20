'use client';

import { useState } from 'react';
import { useT } from '@/i18n';
import { FolderOpen } from 'lucide-react';
import { Button } from '@/components/ui/button';
import {
  Dialog,
  DialogContent,
  DialogHeader,
  DialogTitle,
  DialogFooter,
  DialogBody,
} from '@/components/ui/dialog';
import { Input } from '@/components/ui/input';
import { Label } from '@/components/ui/label';
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from '@/components/ui/select';
import { useFileFolders } from '@/hooks/use-files';
import {
  WorkspaceSelector,
  type WorkspaceSelectorValue,
} from '@/components/common/workspace-selector';
import { useCurrentWorkspace, useIsOrganizationMode } from '@/store';

/**
 * Create folder form data
 */
export interface CreateFolderData {
  name: string;
  parent_id: string;
  workspaceId: string;
}

/**
 * Create folder dialog props
 */
export interface CreateFolderDialogProps {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  onConfirm: (data: CreateFolderData) => void;
}

/**
 * Create Folder Dialog Component
 * Allows users to create a new folder with permissions and parent selection
 */
export function CreateFolderDialog({ open, onOpenChange, onConfirm }: CreateFolderDialogProps) {
  const t = useT();
  const currentWorkspace = useCurrentWorkspace();
  const isOrganizationMode = useIsOrganizationMode();
  const [selectedWorkspace, setSelectedWorkspace] = useState<WorkspaceSelectorValue | undefined>();
  const effectiveWorkspaceId = isOrganizationMode ? selectedWorkspace?.id : currentWorkspace?.id;
  const { folders, isLoading } = useFileFolders(effectiveWorkspaceId, {
    enabled: !isOrganizationMode || !!effectiveWorkspaceId,
  });

  // Form state
  const [folderName, setFolderName] = useState('');
  const [parentId, setParentId] = useState('root');

  // Reset form when dialog closes
  const handleOpenChange = (newOpen: boolean) => {
    if (!newOpen) {
      // Reset form
      setFolderName('');
      setParentId('root');
      setSelectedWorkspace(undefined);
    }
    onOpenChange(newOpen);
  };

  // Handle confirm
  const handleConfirm = () => {
    if (!folderName.trim() || !effectiveWorkspaceId) {
      return;
    }

    const data: CreateFolderData = {
      name: folderName.trim(),
      parent_id: parentId === 'root' ? '' : parentId,
      workspaceId: effectiveWorkspaceId,
    };

    onConfirm(data);
    handleOpenChange(false);
  };

  const handleWorkspaceChange = (workspace: WorkspaceSelectorValue) => {
    setSelectedWorkspace(workspace);
    setParentId('root');
  };

  // Check if can create
  const canCreate = folderName.trim().length > 0 && !!effectiveWorkspaceId;

  return (
    <Dialog open={open} onOpenChange={handleOpenChange}>
      <DialogContent className="max-w-[440px] p-0 overflow-hidden">
        <DialogHeader className="pb-2">
          <DialogTitle className="text-xl font-bold tracking-tight">
            {t('files.folder.createFolder')}
          </DialogTitle>
        </DialogHeader>

        <DialogBody className="space-y-6 py-6">
          <form
            id="create-folder-form"
            onSubmit={e => {
              e.preventDefault();
              handleConfirm();
            }}
            className="space-y-6"
          >
            {/* Folder Name */}
            <div className="space-y-2.5">
              <Label htmlFor="folder-name" className="text-sm font-semibold">
                {t('files.folder.folderName')}
              </Label>
              <Input
                id="folder-name"
                value={folderName}
                onChange={e => setFolderName(e.target.value)}
                placeholder={t('files.folder.folderNamePlaceholder')}
                className="h-11 shadow-sm"
                autoFocus
              />
            </div>

            {isOrganizationMode ? (
              <div className="space-y-2.5">
                <Label className="text-sm font-semibold">{t('files.folder.workspaceLabel')}</Label>
                <WorkspaceSelector
                  value={selectedWorkspace}
                  placeholder={t('files.folder.workspacePlaceholder')}
                  autoSelectFirst
                  onChange={handleWorkspaceChange}
                />
                {!effectiveWorkspaceId ? (
                  <p className="text-xs text-muted-foreground">
                    {t('files.folder.workspaceRequired')}
                  </p>
                ) : null}
              </div>
            ) : null}

            {/* Parent Folder */}
            <div className="space-y-2.5">
              <Label htmlFor="parent-folder" className="text-sm font-semibold">
                {t('files.folder.parentFolder')}
              </Label>
              <Select
                value={parentId}
                onValueChange={setParentId}
                disabled={isLoading || !effectiveWorkspaceId}
              >
                <SelectTrigger id="parent-folder" isLoading={isLoading} className="h-11 shadow-sm">
                  <SelectValue placeholder={t('files.folder.selectParentFolder')} />
                </SelectTrigger>
                <SelectContent>
                  <div className="max-h-[240px] overflow-y-auto p-1">
                    {/* Root folder option */}
                    <SelectItem value="root" className="rounded-md">
                      <div className="flex items-center gap-2 py-1">
                        <FolderOpen className="size-4 text-muted-foreground" />
                        <span className="font-medium text-sm">{t('files.folder.rootFolder')}</span>
                      </div>
                    </SelectItem>
                    {/* Existing folders - as children of root */}
                    {folders.map(folder => (
                      <SelectItem key={folder.id} value={folder.id} className="rounded-md">
                        <div className="flex items-center gap-2 py-1">
                          <FolderOpen className="size-4 text-muted-foreground" />
                          <span className="font-medium text-sm">{folder.name}</span>
                        </div>
                      </SelectItem>
                    ))}
                  </div>
                </SelectContent>
              </Select>
            </div>
          </form>
        </DialogBody>

        <DialogFooter className="bg-neutral-50/50 pt-4 pb-6">
          <Button variant="ghost" onClick={() => handleOpenChange(false)} className="font-semibold">
            {t('common.cancel')}
          </Button>
          <Button
            form="create-folder-form"
            onClick={handleConfirm}
            disabled={!canCreate}
            size="lg"
            className="px-8 font-bold"
          >
            {t('common.create')}
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  );
}
