'use client';

import React, { memo, useState } from 'react';
import Link from 'next/link';
import { Card, CardContent } from '@/components/ui/card';
import { Folder as FolderIcon, MoreHorizontal, Trash2, Pencil } from 'lucide-react';
import { useT } from '@/i18n';
import type { DatasetFolder } from '@/services/types/dataset-folder';
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuTrigger,
} from '@/components/ui/dropdown-menu';
import { ConfirmDialog } from '@/components/ui/confirm-dialog';
import { useDeleteDatasetFolder } from '@/hooks/dataset/use-dataset-folders';
import { eventBus } from '@/lib/event-bus';
import type { OpenFolderModalPayload } from '@/components/datasets/modal/folder-modal';
import { useAccountPermissions } from '@/hooks/organization/use-account-permissions';
import { KNOWLEDGE_BASE_PERMISSION_ACTIONS } from '@/constants/permissions';

interface FolderCardProps {
  folder: DatasetFolder;
}

function FolderCard({ folder }: FolderCardProps) {
  const t = useT('datasets');
  const deleteMutation = useDeleteDatasetFolder();
  const [confirmOpen, setConfirmOpen] = useState(false);

  // Permission checking - use new permission system
  const { hasAnyPermission } = useAccountPermissions();
  const canManageFolders = hasAnyPermission(KNOWLEDGE_BASE_PERMISSION_ACTIONS.folderManage);

  return (
    <div className="relative h-36 sm:h-40">
      <Link
        href={{ pathname: '/console/dataset', query: { folder: folder.id } }}
        className="block h-36 sm:h-40"
      >
        <Card className="hover:shadow-md transition-shadow h-full flex flex-col shrink-0">
          <CardContent className="p-3 sm:p-4 space-y-2 flex-1 flex flex-col shrink-0">
            <div className="flex items-center w-full">
              <FolderIcon className="w-8 h-8 text-primary" />
              <h3
                className="font-medium text-sm sm:text-md w-0 grow pl-2 break-words"
                title={folder.name}
              >
                {folder.name || t('noName')}
              </h3>
            </div>
            <p className="text-xs text-muted-foreground line-clamp-2 sm:line-clamp-3 grow h-0 overflow-ellipsis pt-1 sm:pt-2">
              {folder.description || t('noDescription')}
            </p>
          </CardContent>
        </Card>
      </Link>
      {/* Actions dropdown - only show when user has folder manage permission */}
      {canManageFolders && (
        <div className="absolute bottom-1.5 sm:bottom-2 right-1.5 sm:right-2">
          <DropdownMenu>
            <DropdownMenuTrigger asChild>
              <button className="p-1 rounded hover:bg-muted" aria-label="Folder actions">
                <MoreHorizontal className="h-3 w-3 sm:h-4 sm:w-4" />
              </button>
            </DropdownMenuTrigger>
            <DropdownMenuContent align="end">
              <DropdownMenuItem
                inset
                onSelect={() =>
                  eventBus.publish<OpenFolderModalPayload>('folder:open-modal', {
                    mode: 'edit',
                    folder,
                    parentFolderId: folder.parent_id ?? undefined,
                  })
                }
              >
                <Pencil className="h-4 w-4" />
                {t('actions.edit')}
              </DropdownMenuItem>
              <DropdownMenuItem variant="destructive" inset onSelect={() => setConfirmOpen(true)}>
                <Trash2 className="h-4 w-4" />
                {t('actions.delete')}
              </DropdownMenuItem>
            </DropdownMenuContent>
          </DropdownMenu>
        </div>
      )}
      {/* Delete confirmation dialog */}
      <ConfirmDialog
        variant="danger"
        open={confirmOpen}
        onOpenChange={setConfirmOpen}
        title={t('deleteConfirmTitle', { name: folder.name })}
        description={t('deleteConfirmDescription') + t('deleteFolderConfirmDescription')}
        confirmText={t('confirm')}
        cancelText={t('close')}
        onConfirm={() =>
          deleteMutation.mutate(folder.id, {
            onSuccess: () => setConfirmOpen(false),
          })
        }
        loading={deleteMutation.status === 'pending'}
      />
      {/* Edit handled by centralized FolderModal via event bus; no local modal instance */}
    </div>
  );
}

FolderCard.displayName = 'FolderCard';

export default memo(FolderCard);
