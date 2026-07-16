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
import { WorkspaceAssetMoveDialog } from '@/components/common/workspace-asset-move-dialog';
import { DATABASE_PERMISSION_ACTIONS } from '@/constants/permissions';
import { AgentResourceBoundDialog } from '@/components/common/agent-resource-bound-dialog';
import type { AgentResourceBoundImpact } from '@/services/types/common';
import { getAgentResourceBoundImpact } from '@/utils/agent-resource-bound';
import { dbService } from '@/services/db.service';
import { toast } from 'sonner';

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
  const [bindingImpact, setBindingImpact] = useState<AgentResourceBoundImpact | null>(null);
  const [isCheckingDeleteImpact, setIsCheckingDeleteImpact] = useState(false);
  const router = useRouter();

  // Permissions
  const { hasAnyPermission } = useAccountPermissions();
  const canUpdateDatabase = hasAnyPermission(DATABASE_PERMISSION_ACTIONS.update);
  const canDeleteDatabase = hasAnyPermission(DATABASE_PERMISSION_ACTIONS.delete);
  const canMoveDatabase = hasAnyPermission(DATABASE_PERMISSION_ACTIONS.move);
  const canShowActions = canUpdateDatabase || canDeleteDatabase || canMoveDatabase;

  const deleteDatabase = async (impact?: AgentResourceBoundImpact) => {
    if (!canDeleteDatabase) return;
    try {
      await deleteMutation.mutateAsync({
        dbId: db.id,
        confirmation: impact
          ? { agent_binding_action: 'unbind', impact_token: impact.impact_token }
          : undefined,
      });
      setConfirmOpen(false);
      setBindingImpact(null);
      onDeleted?.(db.id);
    } catch (error) {
      const nextImpact = getAgentResourceBoundImpact(error);
      if (!nextImpact) return;
      setConfirmOpen(false);
      setBindingImpact(nextImpact);
    }
  };

  const requestDeleteDatabase = async () => {
    if (!canDeleteDatabase || isCheckingDeleteImpact) return;
    setIsCheckingDeleteImpact(true);
    try {
      const response = await dbService.previewDbDeleteImpact(db.id);
      if (response.data) {
        setBindingImpact(response.data);
        return;
      }
      setConfirmOpen(true);
    } catch {
      toast.error(common('agentResourceBound.previewFailed'));
    } finally {
      setIsCheckingDeleteImpact(false);
    }
  };

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

      {canShowActions && (
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
              {canUpdateDatabase && (
                <DropdownMenuItem inset onSelect={() => onEdit?.(db)}>
                  <Pencil className="h-4 w-4" /> {common('edit')}
                </DropdownMenuItem>
              )}
              {canMoveDatabase && (
                <DropdownMenuItem inset onSelect={() => setWorkspaceMoveOpen(true)}>
                  <MoveRight className="h-4 w-4" /> {common('assetMove.title')}
                </DropdownMenuItem>
              )}
              {canDeleteDatabase && (
                <DropdownMenuItem
                  variant="destructive"
                  inset
                  disabled={isCheckingDeleteImpact}
                  onSelect={() => void requestDeleteDatabase()}
                >
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
        onConfirm={() => void deleteDatabase()}
        loading={deleteMutation.isPending}
      />
      <AgentResourceBoundDialog
        open={Boolean(bindingImpact)}
        impact={bindingImpact}
        loading={deleteMutation.isPending}
        onOpenChange={open => {
          if (!open) setBindingImpact(null);
        }}
        onConfirm={() => {
          if (bindingImpact) void deleteDatabase(bindingImpact);
        }}
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
