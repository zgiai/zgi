'use client';

import React, { useState, memo } from 'react';
import { useRouter } from 'next/navigation';
import { Card, CardContent } from '@/components/ui/card';
import {
  DropdownMenu,
  DropdownMenuTrigger,
  DropdownMenuContent,
  DropdownMenuItem,
} from '@/components/ui/dropdown-menu';
import { ConfirmDialog } from '@/components/ui/confirm-dialog';
import { Pencil, Trash2, Database as DatabaseIcon, MoreHorizontal, MoveRight } from 'lucide-react';
import type { Db } from '@/services/types/db';
import { IconPreview } from '@/components/common/icon-input/icon-preview';
import { useDeleteDb } from '@/hooks/db/use-dbs';
import { useT } from '@/i18n';
import { ICON_BG } from '@/lib/config';
import { useOrganizations } from '@/hooks/organization/use-organizations';
import { WorkspaceAssetMoveDialog } from '@/components/common/workspace-asset-move-dialog';
import { DATABASE_MANAGE_PERMISSION_CODES } from '@/constants/permissions';

interface DbCardProps {
  db: Db;
  onEdit?: (db: Db) => void;
  onDeleted?: (id: string) => void;
  className?: string;
}

import { useAccountPermissions } from '@/hooks/organization/use-account-permissions';

function DbCardBase({ db, onEdit, onDeleted, className }: DbCardProps) {
  const { dbs: t, common } = useT();
  const deleteMutation = useDeleteDb();
  const [confirmOpen, setConfirmOpen] = useState(false);
  const [workspaceMoveOpen, setWorkspaceMoveOpen] = useState(false);
  const router = useRouter();
  const { currentOrganization } = useOrganizations();

  // Permissions
  const { hasAnyPermission } = useAccountPermissions();
  const canManage = hasAnyPermission(DATABASE_MANAGE_PERMISSION_CODES);
  const canMoveAssets = ['owner', 'admin'].includes(currentOrganization?.organization_role ?? '');

  return (
    <div className="relative h-36 sm:h-40">
      <Card
        className={`hover:shadow-md transition-shadow h-full flex flex-col shrink-0 cursor-pointer ${className || ''}`}
        role="link"
        aria-label={t('goToDetail', { defaultMessage: 'Go to database detail' })}
        onClick={() => router.push(`/console/db/${db.id}`)}
      >
        <CardContent className="p-3 sm:p-4 space-y-2 flex-1 flex flex-col shrink-0">
          <div className="flex items-center w-full">
            <div className="size-6 flex items-center justify-center mr-2 w-8 h-8 sm:w-10 sm:h-10">
              <IconPreview
                icon={db.icon || db.name.slice(0, 2).toUpperCase()}
                iconType={db.icon_type === 'image' ? 'image' : 'text'}
                src={db.icon_type === 'image' ? db.icon_url || undefined : undefined}
                iconBackground={db.icon_background || ICON_BG}
                editable={false}
                size="sm"
              />
            </div>
            <h3
              className="font-medium text-sm sm:text-md w-0 grow pl-2 break-words"
              title={db.name}
            >
              {db.name || t('noName')}
            </h3>
          </div>
          <p className="text-xs text-muted-foreground line-clamp-2 sm:line-clamp-3 grow h-0 overflow-ellipsis pt-1 sm:pt-2">
            {db.description || t('noDescription')}
          </p>
          <div className="text-xs flex items-center gap-2">
            <DatabaseIcon size={14} className="sm:w-4 sm:h-4" />
            <span className="truncate">{t('database')}</span>
          </div>
        </CardContent>
      </Card>

      {(canManage || canMoveAssets) && (
        <div className="absolute bottom-1.5 sm:bottom-2 right-1.5 sm:right-2 z-10">
          <DropdownMenu>
            <DropdownMenuTrigger asChild>
              <button
                className="p-1 rounded hover:bg-muted"
                aria-label="Database actions"
                onClick={e => e.stopPropagation()}
              >
                <MoreHorizontal className="h-3 w-3 sm:h-4 sm:w-4" />
              </button>
            </DropdownMenuTrigger>
            <DropdownMenuContent align="end">
              {canManage && (
                <DropdownMenuItem inset onSelect={() => onEdit?.(db)}>
                  <Pencil className="h-4 w-4" /> {common('edit')}
                </DropdownMenuItem>
              )}
              {canMoveAssets && (
                <DropdownMenuItem inset onSelect={() => setWorkspaceMoveOpen(true)}>
                  <MoveRight className="h-4 w-4" /> {common('assetMove.title')}
                </DropdownMenuItem>
              )}
              {canManage && (
                <DropdownMenuItem variant="destructive" inset onSelect={() => setConfirmOpen(true)}>
                  <Trash2 className="h-4 w-4" /> {common('delete')}
                </DropdownMenuItem>
              )}
            </DropdownMenuContent>
          </DropdownMenu>
        </div>
      )}

      <ConfirmDialog
        variant="danger"
        open={confirmOpen}
        onOpenChange={setConfirmOpen}
        title={t('deleteConfirmTitle', { name: db.name })}
        confirmText={common('confirm')}
        cancelText={common('close')}
        onConfirm={() =>
          deleteMutation.mutate(db.id, {
            onSuccess: () => {
              setConfirmOpen(false);
              onDeleted?.(db.id);
            },
          })
        }
      />
      <WorkspaceAssetMoveDialog
        open={workspaceMoveOpen}
        onOpenChange={setWorkspaceMoveOpen}
        assetType="database"
        assetId={db.id}
        assetName={db.name}
      />
    </div>
  );
}

const DbCard = memo(DbCardBase);
export default DbCard;
