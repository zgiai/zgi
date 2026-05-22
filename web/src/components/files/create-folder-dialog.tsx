'use client';

import { useEffect, useState } from 'react';
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
import { fileManageService } from '@/services/file-manage.service';
import type { FileFolder } from '@/services/types/file';
import {
  WorkspaceSelector,
  type WorkspaceSelectorValue,
} from '@/components/common/workspace-selector';
import { useCurrentWorkspace, useIsOrganizationMode } from '@/store';

type FolderOption = FileFolder & { depth: number };

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
  initialParentId?: string;
}

async function getFolderPath(folderId: string) {
  const path: FileFolder[] = [];
  let currentId = folderId;

  while (currentId) {
    const response = await fileManageService.getFileFolder(currentId);
    const folder = response.data;
    path.unshift(folder);

    if (!folder.parent_id) break;
    currentId = folder.parent_id;
  }

  return path;
}

/**
 * Create Folder Dialog Component
 * Allows users to create a new folder with permissions and parent selection
 */
export function CreateFolderDialog({
  open,
  onOpenChange,
  onConfirm,
  initialParentId = '',
}: CreateFolderDialogProps) {
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
  const [folderOptions, setFolderOptions] = useState<FolderOption[]>([]);
  const [isFolderOptionsLoading, setIsFolderOptionsLoading] = useState(false);

  useEffect(() => {
    if (!open) return;
    setParentId(initialParentId || 'root');

    let ignore = false;

    const loadFolderOptions = async () => {
      setIsFolderOptionsLoading(true);
      const options: FolderOption[] = folders.map(folder => ({ ...folder, depth: 1 }));

      try {
        let nextParentId = 'root';
        if (initialParentId) {
          const existingInitialParent = options.find(folder => folder.id === initialParentId);
          if (existingInitialParent) {
            nextParentId = initialParentId;
          } else {
            const initialParentPath = await getFolderPath(initialParentId);
            if (initialParentPath.length <= 1) {
              const initialParentFolder = initialParentPath.at(-1);
              if (
                initialParentFolder &&
                !options.some(folder => folder.id === initialParentFolder.id)
              ) {
                options.push({ ...initialParentFolder, depth: initialParentPath.length });
              }
              nextParentId = initialParentId;
            }
          }
        }

        if (!ignore) {
          setFolderOptions(options);
          setParentId(nextParentId);
        }
      } catch {
        if (!ignore) {
          setFolderOptions(options);
          setParentId(
            options.some(folder => folder.id === initialParentId) ? initialParentId : 'root'
          );
        }
      } finally {
        if (!ignore) {
          setIsFolderOptionsLoading(false);
        }
      }
    };

    void loadFolderOptions();

    return () => {
      ignore = true;
    };
  }, [effectiveWorkspaceId, folders, initialParentId, open]);

  // Reset form when dialog closes
  const handleOpenChange = (newOpen: boolean) => {
    if (!newOpen) {
      // Reset form
      setFolderName('');
      setParentId('root');
      setFolderOptions([]);
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
    setFolderOptions([]);
  };

  // Check if can create
  const canCreate = folderName.trim().length > 0 && !!effectiveWorkspaceId;
  const isParentFolderLoading = isLoading || isFolderOptionsLoading;

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
                disabled={isParentFolderLoading || !effectiveWorkspaceId}
              >
                <SelectTrigger
                  id="parent-folder"
                  isLoading={isParentFolderLoading}
                  className="h-11 shadow-sm"
                >
                  <SelectValue placeholder={t('files.folder.selectParentFolder')} />
                </SelectTrigger>
                <SelectContent>
                  <div className="max-h-[240px] overflow-y-auto p-1">
                    {/* Root folder option */}
                    <SelectItem value="root" className="rounded-md">
                      <div className="flex items-center gap-2 py-1">
                        <FolderOpen className="size-4 text-muted-foreground" />
                        <span className="font-medium text-sm">
                          {t('files.upload.defaultFolder')}
                        </span>
                      </div>
                    </SelectItem>
                    {/* Existing folders - as children of root */}
                    {folderOptions.map(folder => (
                      <SelectItem key={folder.id} value={folder.id} className="rounded-md">
                        <div
                          className="flex items-center gap-2 py-1"
                          style={{ paddingLeft: `${(folder.depth - 1) * 16}px` }}
                        >
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
