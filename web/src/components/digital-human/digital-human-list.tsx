'use client';

import { useState } from 'react';
import Link from 'next/link';
import { useQueryClient } from '@tanstack/react-query';
import {
  Bot,
  Clock3,
  MoreHorizontal,
  MoveRight,
  Pencil,
  Plus,
  Power,
  PowerOff,
  Search,
  Trash2,
} from 'lucide-react';
import { toast } from 'sonner';
import { DigitalHumanBasicInfoDialog } from '@/components/digital-human/digital-human-basic-info-dialog';
import { WorkspaceAssetMoveDialog } from '@/components/common/workspace-asset-move-dialog';
import { IconPreview } from '@/components/common/icon-input/icon-preview';
import { Badge } from '@/components/ui/badge';
import { Button } from '@/components/ui/button';
import { Card, CardContent } from '@/components/ui/card';
import { ConfirmDialog } from '@/components/ui/confirm-dialog';
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuTrigger,
} from '@/components/ui/dropdown-menu';
import { Input } from '@/components/ui/input';
import {
  useDigitalHumanMutations,
  useDigitalHumans,
} from '@/hooks/digital-human/use-digital-humans';
import { useAccountPermissions } from '@/hooks/organization/use-account-permissions';
import { DIGITAL_HUMAN_KEYS } from '@/hooks/query-keys';
import { useT } from '@/i18n';
import { digitalHumanService } from '@/services/digital-human.service';
import type { DigitalHumanDefinition } from '@/services/types/digital-human';
import { getErrorMessage } from '@/utils/error-notifications';
import { getAgentTextIconDisplay } from '@/components/agents/agent-runtime/utils';

/** Creation lives on its own route; this page only links to it. */
export function DigitalHumanList() {
  const t = useT('digitalHumans');
  const permissions = useAccountPermissions();
  const [search, setSearch] = useState('');
  const list = useDigitalHumans(true, search.trim());
  const items = list.data?.data?.items ?? [];
  const canCreate = permissions.hasPermission('digital_human.create');
  const canUpdate = permissions.hasPermission('digital_human.update');
  const canDeleteDigitalHuman = permissions.hasPermission('digital_human.delete');

  return (
    <div className="flex h-full flex-col space-y-6 overflow-y-auto p-4 md:p-6 xl:p-8">
      <div className="flex flex-col justify-between gap-4 lg:flex-row lg:items-center">
        <h1 className="text-xl font-semibold sm:text-2xl">{t('title')}</h1>
        <div className="flex w-full flex-col gap-3 sm:flex-row lg:w-auto lg:justify-end">
          <div className="relative w-full sm:w-64">
            <Search className="pointer-events-none absolute left-3 top-1/2 size-4 -translate-y-1/2 text-muted-foreground" />
            <Input
              value={search}
              onChange={event => setSearch(event.target.value)}
              placeholder={t('list.search')}
              className="w-full rounded-lg bg-background pl-9 text-sm"
            />
          </div>
          {canCreate ? (
            <Button asChild className="w-full sm:w-auto">
              <Link href="/console/digital-humans/create">
                <Plus className="size-4" />
                {t('list.create')}
              </Link>
            </Button>
          ) : null}
        </div>
      </div>

      {list.isLoading ? (
        <div className="grid grid-cols-[repeat(auto-fill,minmax(15rem,1fr))] gap-4">
          {Array.from({ length: 12 }).map((_, item) => (
            <div key={item} className="h-48 animate-pulse rounded-xl border bg-muted/30" />
          ))}
        </div>
      ) : items.length === 0 ? (
        <div className="flex min-h-72 flex-col items-center justify-center rounded-xl border border-dashed text-center">
          <Bot className="mb-3 size-9 text-muted-foreground" />
          <p className="max-w-md text-sm text-muted-foreground">{t('list.empty')}</p>
        </div>
      ) : (
        <div className="grid grid-cols-[repeat(auto-fill,minmax(15rem,1fr))] gap-4">
          {items.map(item => (
            <DigitalHumanCard
              key={item.id}
              item={item}
              canUpdate={canUpdate}
              canDeleteDigitalHuman={canDeleteDigitalHuman}
            />
          ))}
        </div>
      )}
    </div>
  );
}

function DigitalHumanCard({
  item,
  canUpdate,
  canDeleteDigitalHuman,
}: {
  item: DigitalHumanDefinition;
  canUpdate: boolean;
  canDeleteDigitalHuman: boolean;
}) {
  const t = useT('digitalHumans');
  const tCommon = useT('common');
  const queryClient = useQueryClient();
  const mutations = useDigitalHumanMutations();
  const [editOpen, setEditOpen] = useState(false);
  const [moveOpen, setMoveOpen] = useState(false);
  const [confirm, setConfirm] = useState<'offline' | 'online' | 'delete' | null>(null);
  const [pending, setPending] = useState(false);
  const textIcon = getAgentTextIconDisplay(item.icon_type, item.icon, item.name);
  const isOffline = item.status === 'disabled';
  const isPublished = Boolean(item.published_version_id);
  // Published versions are immutable, so an online digital human must be taken
  // offline before it can be deleted.
  const canDelete = !isPublished || isOffline;
  const badge = isOffline
    ? {
        className:
          'border-amber-500/40 bg-amber-500/10 text-[11px] font-normal text-amber-700 dark:text-amber-300',
        label: t('list.offline'),
      }
    : isPublished
      ? {
          className: 'border-success/40 bg-success/10 text-[11px] font-normal text-success',
          label: t('list.published'),
        }
      : {
          className: 'bg-muted text-[11px] font-normal text-muted-foreground',
          label: t('status.draft'),
        };

  const refreshList = () => queryClient.invalidateQueries({ queryKey: DIGITAL_HUMAN_KEYS.lists() });

  const applyStatus = async (status: 'active' | 'disabled') => {
    setPending(true);
    try {
      await mutations.update.mutateAsync({
        digitalHumanId: item.id,
        request: { expected_revision: item.revision, status },
      });
      await refreshList();
    } catch {
      // The mutation surfaces the failure toast itself.
    } finally {
      setPending(false);
      setConfirm(null);
    }
  };

  const removeDigitalHuman = async () => {
    setPending(true);
    try {
      await digitalHumanService.delete(item.id, item.revision);
      await refreshList();
    } catch (error) {
      toast.error(getErrorMessage(error) || t('list.deleteBlocked'));
    } finally {
      setPending(false);
      setConfirm(null);
    }
  };

  const confirmCopy =
    confirm === 'delete'
      ? {
          title: t('list.deleteTitle'),
          description: t('list.deleteDescription'),
          confirmText: t('list.delete'),
          variant: 'warning' as const,
        }
      : confirm === 'offline'
        ? {
            title: t('list.takeOfflineTitle'),
            description: t('list.takeOfflineDescription'),
            confirmText: t('list.takeOffline'),
            variant: 'warning' as const,
          }
        : {
            title: t('list.bringOnlineTitle'),
            description: t('list.bringOnlineDescription'),
            confirmText: t('list.bringOnline'),
            variant: 'default' as const,
          };

  return (
    <div className="relative h-48">
      <Link
        href={
          item.published_version_id
            ? `/console/digital-humans/${item.id}`
            : `/console/digital-humans/${item.id}/configuration`
        }
        className="block h-full rounded-xl outline-none focus-visible:ring-2 focus-visible:ring-ring focus-visible:ring-offset-2"
      >
        <Card className="flex h-full flex-col border border-border/80 shadow-sm transition-colors hover:border-border hover:bg-muted/20 hover:shadow-sm">
          <CardContent className="flex h-full min-w-0 flex-1 flex-col p-4">
            <div className="flex items-start justify-between gap-3">
              <div className="flex size-10 shrink-0 items-center justify-center">
                <IconPreview
                  icon={textIcon.text}
                  iconType={item.icon_type || 'text'}
                  src={item.icon_type === 'image' ? item.icon_url || item.avatar || '' : ''}
                  iconBackground={textIcon.background}
                  editable={false}
                  size="sm"
                />
              </div>
              <Badge variant="outline" className={badge.className}>
                {badge.label}
              </Badge>
            </div>
            <h2
              className="mt-3 line-clamp-2 min-h-10 break-all text-sm font-semibold leading-5 text-foreground"
              title={item.name}
            >
              {item.name}
            </h2>
            <p className="mt-3 line-clamp-2 min-h-10 text-xs leading-5 text-muted-foreground">
              {item.description || t('list.noDescription')}
            </p>
            <div className="mt-auto flex items-center gap-2 pr-8 text-xs text-muted-foreground">
              <Clock3 className="size-4 shrink-0" />
              <span className="truncate">
                {t('list.updatedAt', {
                  date: new Date(item.updated_at).toLocaleDateString(),
                })}
              </span>
            </div>
          </CardContent>
        </Card>
      </Link>

      {/* Deleting has its own permission, so a role that may delete but not
          edit still gets the menu with only that action. */}
      {canUpdate || canDeleteDigitalHuman ? (
        <div className="absolute bottom-2 right-2">
          <DropdownMenu>
            <DropdownMenuTrigger asChild>
              <button
                type="button"
                className="rounded-md p-1 text-muted-foreground outline-none hover:bg-muted hover:text-foreground focus-visible:ring-2 focus-visible:ring-ring"
                aria-label={t('list.actions')}
              >
                <MoreHorizontal className="size-4" />
              </button>
            </DropdownMenuTrigger>
            <DropdownMenuContent align="end">
              {canUpdate ? (
                <>
                  <DropdownMenuItem inset onSelect={() => setEditOpen(true)}>
                    <Pencil className="size-4" />
                    {t('basicInfo.edit')}
                  </DropdownMenuItem>
                  <DropdownMenuItem inset onSelect={() => setMoveOpen(true)}>
                    <MoveRight className="size-4" />
                    {tCommon('assetMove.title')}
                  </DropdownMenuItem>
                  {isPublished ? (
                    <DropdownMenuItem
                      inset
                      onSelect={() => setConfirm(isOffline ? 'online' : 'offline')}
                    >
                      {isOffline ? <Power className="size-4" /> : <PowerOff className="size-4" />}
                      {isOffline ? t('list.bringOnline') : t('list.takeOffline')}
                    </DropdownMenuItem>
                  ) : null}
                </>
              ) : null}
              {canDeleteDigitalHuman ? (
                <DropdownMenuItem
                  inset
                  disabled={!canDelete}
                  onSelect={() => {
                    if (canDelete) setConfirm('delete');
                  }}
                >
                  <Trash2 className="size-4" />
                  <span className="flex min-w-0 flex-col">
                    <span>{t('list.delete')}</span>
                    {!canDelete ? (
                      <span className="text-[11px] text-muted-foreground">
                        {t('list.deleteBlocked')}
                      </span>
                    ) : null}
                  </span>
                </DropdownMenuItem>
              ) : null}
            </DropdownMenuContent>
          </DropdownMenu>
        </div>
      ) : null}

      <DigitalHumanBasicInfoDialog definition={item} open={editOpen} onOpenChange={setEditOpen} />
      <WorkspaceAssetMoveDialog
        open={moveOpen}
        onOpenChange={setMoveOpen}
        assetType="digital_human"
        assetId={item.id}
        assetName={item.name}
        onMoved={() => void refreshList()}
      />

      <ConfirmDialog
        open={confirm !== null}
        onOpenChange={open => {
          if (!open) setConfirm(null);
        }}
        variant={confirmCopy.variant}
        title={confirmCopy.title}
        description={confirmCopy.description}
        cancelText={t('common.cancel')}
        confirmText={confirmCopy.confirmText}
        loading={pending}
        onConfirm={() => {
          if (confirm === 'delete') void removeDigitalHuman();
          else if (confirm === 'offline') void applyStatus('disabled');
          else if (confirm === 'online') void applyStatus('active');
        }}
      />
    </div>
  );
}
