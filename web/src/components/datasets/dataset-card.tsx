'use client';

import React, { memo } from 'react';
import Link from 'next/link';
import { useRouter } from 'next/navigation';
import { Card, CardContent } from '@/components/ui/card';
import {
  DropdownMenu,
  DropdownMenuTrigger,
  DropdownMenuContent,
  DropdownMenuItem,
} from '@/components/ui/dropdown-menu';
import { ConfirmDialog } from '@/components/ui/confirm-dialog';

import { BookOpenIcon, MoreHorizontal, Trash2, FolderOpen, Pencil, MoveRight } from 'lucide-react';
import { useT } from '@/i18n';
import { useDeleteDataset } from '@/hooks/dataset/use-datasets';
import type { Dataset } from '@/services/types/dataset';
import { useState } from 'react';
import { IconPreview } from '../common/icon-input/icon-preview';
import MoveDatasetModal from '@/components/datasets/modal/move-dataset-modal';
import { Badge } from '../ui/badge';
import { useAccountPermissions } from '@/hooks/organization/use-account-permissions';
import { ICON_BG } from '@/lib/config';
import { useOrganizations } from '@/hooks/organization/use-organizations';
import { WorkspaceAssetMoveDialog } from '@/components/common/workspace-asset-move-dialog';
import { KNOWLEDGE_BASE_MANAGE_PERMISSION_CODES } from '@/constants/permissions';

interface DatasetCardProps {
  dataset: Dataset;
  /** The page index where this dataset resides in the paged list */
  pageIndex: number;
  /** Callback when dataset is deleted; provides id and page index for incremental refetch */
  onDeleted?: (id: string, pageIndex: number) => void;
  /** Current folder id when viewing within a folder; undefined when in root view */
  currentFolderId?: string;
}

function DatasetCard({ dataset, onDeleted, pageIndex, currentFolderId }: DatasetCardProps) {
  const t = useT('datasets');
  const tCommon = useT('common');
  const router = useRouter();
  const deleteMutation = useDeleteDataset();
  const [confirmOpen, setConfirmOpen] = useState(false);
  const [moveOpen, setMoveOpen] = useState(false);
  const [workspaceMoveOpen, setWorkspaceMoveOpen] = useState(false);
  const { currentOrganization } = useOrganizations();

  // Permission checking - use new permission system
  const { hasAnyPermission } = useAccountPermissions();
  const canManage = hasAnyPermission(KNOWLEDGE_BASE_MANAGE_PERMISSION_CODES);
  const canMoveAssets = ['owner', 'admin'].includes(currentOrganization?.organization_role ?? '');

  return (
    <div className="relative h-36 sm:h-40">
      <div
        onClick={() => {
          sessionStorage.setItem('dataset_prev_folder_id', currentFolderId || '');
        }}
      >
        <Link href={`/console/dataset/${dataset.id}/documents`} className="block h-36 sm:h-40">
          <Card className="hover:shadow-md transition-shadow h-full flex flex-col shrink-0">
            <CardContent className="p-3 sm:p-4 space-y-2 flex-1 flex flex-col shrink-0">
              <div className="flex items-center w-full">
                <div className="size-6 flex items-center justify-center mr-2 w-8 h-8 sm:w-10 sm:h-10">
                  <IconPreview
                    icon={dataset.icon || dataset.name.slice(0, 2).toUpperCase()}
                    iconType={dataset.icon_type}
                    src={dataset.icon_type === 'image' ? dataset.icon_url : undefined}
                    iconBackground={dataset.icon_background || ICON_BG}
                    editable={false}
                    size="sm"
                  />
                </div>
                <h3
                  className="font-medium text-sm sm:text-md w-0 grow px-1 break-words line-clamp-2"
                  title={dataset.name}
                >
                  {dataset.name || t('noName')}
                </h3>
                {dataset.enable_graph_flow && (
                  <div className="flex">
                    <Badge
                      variant="outline"
                      className="border-highlight text-highlight bg-highlight/10 text-[11px] px-1.5 py-px"
                    >
                      {t('graphFlowBadge')}
                    </Badge>
                  </div>
                )}
              </div>
              <p className="text-xs text-muted-foreground line-clamp-2 sm:line-clamp-3 grow h-0 overflow-ellipsis pt-1 sm:pt-2">
                {dataset.description || t('noDescription')}
              </p>
              <div className="text-xs flex items-center gap-2">
                <BookOpenIcon size={14} className="sm:w-4 sm:h-4" />
                <span className="truncate">{t('dataset')}</span>
              </div>
            </CardContent>
          </Card>
        </Link>
      </div>
      {(canManage || canMoveAssets) && (
        <div className="absolute bottom-1.5 sm:bottom-2 right-1.5 sm:right-2">
          <DropdownMenu>
            <DropdownMenuTrigger asChild>
              <button className="p-1 rounded hover:bg-muted">
                <MoreHorizontal className="h-3 w-3 sm:h-4 sm:w-4" />
              </button>
            </DropdownMenuTrigger>
            <DropdownMenuContent align="end">
              {/* Edit dataset basic info via page-level dialog */}
              {canManage && (
                <>
                  <DropdownMenuItem
                    inset
                    onSelect={() => {
                      sessionStorage.setItem('dataset_prev_folder_id', currentFolderId || '');
                      router.push(`/console/dataset/${dataset.id}/settings`);
                    }}
                  >
                    <Pencil className="h-4 w-4" />
                    {t('actions.edit')}
                  </DropdownMenuItem>
                  {/* Move dataset to another folder */}
                  <DropdownMenuItem inset onSelect={() => setMoveOpen(true)}>
                    <FolderOpen className="h-4 w-4" />
                    {t('actions.move')}
                  </DropdownMenuItem>
                </>
              )}
              {canMoveAssets && (
                <DropdownMenuItem inset onSelect={() => setWorkspaceMoveOpen(true)}>
                  <MoveRight className="h-4 w-4" />
                  {tCommon('assetMove.title')}
                </DropdownMenuItem>
              )}
              {canManage && (
                <DropdownMenuItem variant="destructive" inset onSelect={() => setConfirmOpen(true)}>
                  <Trash2 className="h-4 w-4" />
                  {t('actions.delete')}
                </DropdownMenuItem>
              )}
            </DropdownMenuContent>
          </DropdownMenu>
        </div>
      )}
      {/* Delete confirmation dialog outside dropdown */}
      <ConfirmDialog
        variant="danger"
        open={confirmOpen}
        onOpenChange={setConfirmOpen}
        title={t('deleteConfirmTitle', { name: dataset.name })}
        description={t('deleteConfirmDescription')}
        confirmText={t('confirm')}
        cancelText={t('close')}
        onConfirm={() =>
          deleteMutation.mutate(dataset.id, {
            onSuccess: () => {
              setConfirmOpen(false);
              onDeleted?.(dataset.id, pageIndex);
            },
          })
        }
        loading={deleteMutation.status === 'pending'}
      />
      {/* Move dataset modal */}
      <MoveDatasetModal
        open={moveOpen}
        onOpenChange={setMoveOpen}
        datasetId={dataset.id}
        currentFolderId={currentFolderId}
      />
      <WorkspaceAssetMoveDialog
        open={workspaceMoveOpen}
        onOpenChange={setWorkspaceMoveOpen}
        assetType="dataset"
        assetId={dataset.id}
        assetName={dataset.name}
      />
    </div>
  );
}

DatasetCard.displayName = 'DatasetCard';

export default memo(DatasetCard);
