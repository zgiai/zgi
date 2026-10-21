'use client';

import { useEffect, useMemo, useState } from 'react';
import { Copy, Edit, KeyRound, MoreHorizontal, Plus, Search, Trash2, X } from 'lucide-react';
import { toast } from 'sonner';
import { Badge } from '@/components/ui/badge';
import { NotificationChannelManager } from '@/components/notifications/notification-channel-manager';
import { Button } from '@/components/ui/button';
import { ConfirmDialog } from '@/components/ui/confirm-dialog';
import {
  Dialog,
  DialogBody,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from '@/components/ui/dialog';
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuSeparator,
  DropdownMenuTrigger,
} from '@/components/ui/dropdown-menu';
import { Input } from '@/components/ui/input';
import { Label } from '@/components/ui/label';
import { Skeleton } from '@/components/ui/skeleton';
import { Switch } from '@/components/ui/switch';
import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from '@/components/ui/table';
import {
  useDigitalHumanAccess,
  useDigitalHumanAccessMutations,
  useDigitalHumanAPIKeys,
} from '@/hooks/digital-human/use-digital-human-access';
import { useDigitalHuman } from '@/hooks/digital-human/use-digital-humans';
import { useAccountPermissions } from '@/hooks/organization/use-account-permissions';
import { useT } from '@/i18n';
import type { DigitalHumanAPIKey } from '@/services/types/digital-human';
import { getErrorMessage } from '@/utils/error-notifications';
import { formatDate } from '@/utils/format';

export function DigitalHumanAPIKeys({ id }: { id: string }) {
  const detailQuery = useDigitalHuman(id);
  const detail = detailQuery.data?.data;
  const permissions = useAccountPermissions();

  if (detailQuery.isLoading || !detail) {
    return <DigitalHumanAPIKeysLoading />;
  }

  return (
    <DigitalHumanAPIKeysContent
      id={id}
      published={Boolean(detail.published)}
      canPublish={permissions.hasPermission('digital_human.publish')}
    />
  );
}

function DigitalHumanAPIKeysContent({
  id,
  published,
  canPublish,
}: {
  id: string;
  published: boolean;
  canPublish: boolean;
}) {
  const t = useT('digitalHumans.access');
  const tCommon = useT('common');
  const access = useDigitalHumanAccess(id, canPublish);
  const keys = useDigitalHumanAPIKeys(id, canPublish && published);
  const mutations = useDigitalHumanAccessMutations(id);
  const [search, setSearch] = useState('');
  const [editingKey, setEditingKey] = useState<DigitalHumanAPIKey | null>(null);
  const [keyDialogOpen, setKeyDialogOpen] = useState(false);
  const [revealedKey, setRevealedKey] = useState('');
  const [deleteKey, setDeleteKey] = useState<DigitalHumanAPIKey | null>(null);
  const apiKeys = useMemo(() => keys.data?.data.api_keys ?? [], [keys.data?.data.api_keys]);
  const filteredKeys = useMemo(() => {
    const query = search.trim().toLowerCase();
    if (!query) return apiKeys;
    return apiKeys.filter(
      key => key.name.toLowerCase().includes(query) || key.key_prefix.toLowerCase().includes(query)
    );
  }, [apiKeys, search]);

  const updateAPISurface = async (enabled: boolean) => {
    try {
      await mutations.updateAccess.mutateAsync({ api_enabled: enabled });
      toast.success(t('surfaceUpdated'));
    } catch (error) {
      toast.error(getErrorMessage(error) || t('surfaceUpdateFailed'));
    }
  };

  const openCreate = () => {
    setEditingKey(null);
    setKeyDialogOpen(true);
  };

  return (
    <div className="mx-auto w-full max-w-[1360px] space-y-5 p-4 sm:p-6">
      <div className="flex flex-wrap items-center justify-between gap-3">
        <div className="flex items-center gap-3">
          <div className="flex size-10 items-center justify-center rounded-lg bg-primary/10">
            <KeyRound className="size-5 text-primary" />
          </div>
          <div>
            <h2 className="text-2xl font-bold">{t('keysTitle')}</h2>
            <p className="text-muted-foreground">{t('keysDescription')}</p>
          </div>
        </div>
        <div className="flex items-center gap-2">
          <Button variant="outline" onClick={() => void keys.refetch()} disabled={keys.isFetching}>
            {t('refresh')}
          </Button>
          <Button disabled={!canPublish || !published} onClick={openCreate}>
            <Plus className="size-4" />
            {t('createKey')}
          </Button>
        </div>
      </div>

      {!published ? (
        <div className="rounded-lg border border-warning/30 bg-warning/5 px-3 py-2 text-xs text-warning-foreground">
          {t('publishFirst')}
        </div>
      ) : null}

      <section className="flex items-start justify-between gap-5 rounded-lg border bg-background px-4 py-4">
        <div>
          <div className="text-sm font-medium">{t('apiTitle')}</div>
          <p className="mt-1 text-xs leading-5 text-muted-foreground">{t('apiDescription')}</p>
        </div>
        <Switch
          checked={access.data?.data.api_enabled ?? false}
          disabled={
            !canPublish || !published || access.isLoading || mutations.updateAccess.isPending
          }
          onCheckedChange={enabled => void updateAPISurface(enabled)}
        />
      </section>

      {keys.isLoading ? (
        <div className="space-y-3">
          <Skeleton className="h-10 w-72" />
          <Skeleton className="h-12 w-full" />
          <Skeleton className="h-12 w-full" />
        </div>
      ) : apiKeys.length === 0 ? (
        <div className="rounded-lg border border-dashed p-8 text-center">
          <div className="mx-auto mb-4 flex size-12 items-center justify-center rounded-lg bg-primary/10">
            <KeyRound className="size-6 text-primary" />
          </div>
          <h3 className="text-base font-semibold">{t('noKeysTitle')}</h3>
          <p className="mx-auto mt-2 max-w-md text-sm leading-6 text-muted-foreground">
            {t('noKeys')}
          </p>
          <Button className="mt-5" disabled={!canPublish || !published} onClick={openCreate}>
            <Plus className="size-4" />
            {t('createKey')}
          </Button>
        </div>
      ) : (
        <div className="overflow-hidden rounded-lg border bg-background">
          <div className="flex items-center border-b bg-muted/20 px-4 py-3">
            <div className="relative w-full sm:w-80">
              <Search className="absolute left-3 top-1/2 size-4 -translate-y-1/2 text-muted-foreground" />
              <Input
                value={search}
                onChange={event => setSearch(event.target.value)}
                placeholder={t('searchKeys')}
                className="pl-9 pr-9"
              />
              {search ? (
                <button
                  type="button"
                  onClick={() => setSearch('')}
                  className="absolute right-3 top-1/2 -translate-y-1/2 text-muted-foreground hover:text-foreground"
                  aria-label={t('clearSearch')}
                >
                  <X className="size-4" />
                </button>
              ) : null}
            </div>
          </div>
          {filteredKeys.length === 0 ? (
            <div className="p-8 text-center text-sm text-muted-foreground">
              {t('noMatchingKeys')}
            </div>
          ) : (
            <Table>
              <TableHeader>
                <TableRow>
                  <TableHead>{t('keyName')}</TableHead>
                  <TableHead>{t('keyValue')}</TableHead>
                  <TableHead>{t('updatedAt')}</TableHead>
                  <TableHead>{t('keyExpiry')}</TableHead>
                  <TableHead>{t('statusLabel')}</TableHead>
                  <TableHead className="w-14 text-right">{t('actions')}</TableHead>
                </TableRow>
              </TableHeader>
              <TableBody>
                {filteredKeys.map(key => (
                  <TableRow key={key.id}>
                    <TableCell className="font-medium">{key.name}</TableCell>
                    <TableCell className="font-mono text-xs text-muted-foreground">
                      {key.key_prefix}…
                    </TableCell>
                    <TableCell>{formatDate(key.updated_at)}</TableCell>
                    <TableCell>
                      {key.expires_at ? formatDate(key.expires_at) : t('neverExpires')}
                    </TableCell>
                    <TableCell>
                      <Badge variant={key.status === 'active' ? 'success' : 'secondary'}>
                        {key.status === 'active' ? t('active') : t('revoked')}
                      </Badge>
                    </TableCell>
                    <TableCell className="text-right">
                      <DropdownMenu>
                        <DropdownMenuTrigger asChild>
                          <Button variant="ghost" size="sm" isIcon aria-label={t('actions')}>
                            <MoreHorizontal className="size-4" />
                          </Button>
                        </DropdownMenuTrigger>
                        <DropdownMenuContent align="end">
                          {key.status === 'active' ? (
                            <>
                              <DropdownMenuItem
                                onSelect={() => {
                                  setEditingKey(key);
                                  setKeyDialogOpen(true);
                                }}
                              >
                                <Edit className="size-4" />
                                {t('editKey')}
                              </DropdownMenuItem>
                              <DropdownMenuItem
                                onSelect={() =>
                                  void mutations.updateAPIKey
                                    .mutateAsync({
                                      keyId: key.id,
                                      request: { status: 'revoked' },
                                    })
                                    .then(() => toast.success(t('keyRevoked')))
                                    .catch(error =>
                                      toast.error(getErrorMessage(error) || t('keyUpdateFailed'))
                                    )
                                }
                              >
                                {t('revoke')}
                              </DropdownMenuItem>
                              <DropdownMenuSeparator />
                            </>
                          ) : null}
                          <DropdownMenuItem
                            variant="destructive"
                            onSelect={() => setDeleteKey(key)}
                          >
                            <Trash2 className="size-4" />
                            {t('delete')}
                          </DropdownMenuItem>
                        </DropdownMenuContent>
                      </DropdownMenu>
                    </TableCell>
                  </TableRow>
                ))}
              </TableBody>
            </Table>
          )}
        </div>
      )}

      <APIKeyEditorDialog
        open={keyDialogOpen}
        onOpenChange={setKeyDialogOpen}
        apiKey={editingKey}
        pending={mutations.createAPIKey.isPending || mutations.updateAPIKey.isPending}
        onSubmit={async values => {
          try {
            if (editingKey) {
              await mutations.updateAPIKey.mutateAsync({ keyId: editingKey.id, request: values });
              toast.success(t('keyUpdated'));
            } else {
              const response = await mutations.createAPIKey.mutateAsync(values);
              setRevealedKey(response.data.api_key);
            }
            setKeyDialogOpen(false);
          } catch (error) {
            toast.error(getErrorMessage(error) || t('keyUpdateFailed'));
          }
        }}
      />
      <ConfirmDialog
        open={Boolean(deleteKey)}
        onOpenChange={open => !open && setDeleteKey(null)}
        title={t('deleteTitle')}
        description={t('deleteDescription', { name: deleteKey?.name ?? '' })}
        confirmText={t('delete')}
        cancelText={t('cancel')}
        variant="danger"
        onConfirm={async () => {
          if (!deleteKey) return;
          try {
            await mutations.deleteAPIKey.mutateAsync(deleteKey.id);
            toast.success(t('keyDeleted'));
            setDeleteKey(null);
          } catch (error) {
            toast.error(getErrorMessage(error) || t('keyDeleteFailed'));
          }
        }}
      />
      <Dialog open={Boolean(revealedKey)} onOpenChange={open => !open && setRevealedKey('')}>
        <DialogContent size="md">
          <DialogHeader>
            <DialogTitle>{t('keyCreatedTitle')}</DialogTitle>
            <DialogDescription>{t('keyCreatedDescription')}</DialogDescription>
          </DialogHeader>
          <DialogBody>
            <div className="flex gap-2">
              <Input value={revealedKey} readOnly className="font-mono" />
              <Button
                type="button"
                variant="outline"
                onClick={() =>
                  void navigator.clipboard
                    .writeText(revealedKey)
                    .then(() => toast.success(t('keyCopied')))
                }
              >
                <Copy className="size-4" />
              </Button>
            </div>
          </DialogBody>
          <DialogFooter>
            <Button onClick={() => setRevealedKey('')}>{t('done')}</Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>
      <section className="rounded-xl border bg-background p-5">
        <h3 className="text-sm font-semibold">{tCommon('notifications.webhookSectionTitle')}</h3>
        <p className="mb-3 mt-1 text-sm text-muted-foreground">{tCommon('notifications.webhookSectionDescription')}</p>
        <NotificationChannelManager basePath={`/digital-humans/${id}/notification-channels`} types={['webhook']} canManage={canPublish} />
      </section>
    </div>
  );
}

function APIKeyEditorDialog({
  open,
  onOpenChange,
  apiKey,
  pending,
  onSubmit,
}: {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  apiKey: DigitalHumanAPIKey | null;
  pending: boolean;
  onSubmit: (values: { name: string; expires_at?: string }) => Promise<void>;
}) {
  const t = useT('digitalHumans.access');
  const [name, setName] = useState('');
  const [expiresAt, setExpiresAt] = useState('');
  const [hasExpiry, setHasExpiry] = useState(false);

  useEffect(() => {
    if (!open) return;
    setName(apiKey?.name ?? '');
    setHasExpiry(Boolean(apiKey?.expires_at));
    setExpiresAt(apiKey?.expires_at ? toLocalDateTime(apiKey.expires_at) : '');
  }, [apiKey, open]);

  const submit = () => {
    const normalized = name.trim();
    if (!normalized || (hasExpiry && !expiresAt)) return;
    void onSubmit({
      name: normalized,
      ...(hasExpiry && expiresAt ? { expires_at: new Date(expiresAt).toISOString() } : {}),
    });
  };

  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent size="md">
        <DialogHeader>
          <DialogTitle>{apiKey ? t('editKeyTitle') : t('createKeyTitle')}</DialogTitle>
          <DialogDescription>
            {apiKey ? t('editKeyDescription') : t('createKeyDescription')}
          </DialogDescription>
        </DialogHeader>
        <DialogBody className="space-y-4">
          <div className="space-y-2">
            <Label>{t('keyName')}</Label>
            <Input value={name} onChange={event => setName(event.target.value)} maxLength={255} />
          </div>
          <div className="space-y-3 rounded-lg border p-3">
            <div className="flex items-center justify-between gap-3">
              <div>
                <Label>{t('keyExpiry')}</Label>
                <p className="mt-1 text-xs text-muted-foreground">{t('keyExpiryDescription')}</p>
              </div>
              <Switch
                checked={hasExpiry}
                disabled={Boolean(apiKey?.expires_at)}
                onCheckedChange={setHasExpiry}
              />
            </div>
            {hasExpiry ? (
              <Input
                type="datetime-local"
                value={expiresAt}
                min={toLocalDateTime(new Date().toISOString())}
                onChange={event => setExpiresAt(event.target.value)}
              />
            ) : null}
          </div>
        </DialogBody>
        <DialogFooter>
          <Button variant="outline" onClick={() => onOpenChange(false)}>
            {t('cancel')}
          </Button>
          <Button
            disabled={!name.trim() || (hasExpiry && !expiresAt) || pending}
            loading={pending}
            onClick={submit}
          >
            {apiKey ? t('save') : t('create')}
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  );
}

function DigitalHumanAPIKeysLoading() {
  return (
    <div className="mx-auto w-full max-w-[1360px] space-y-5 p-4 sm:p-6">
      <Skeleton className="h-12 w-72" />
      <Skeleton className="h-24 w-full" />
      <Skeleton className="h-64 w-full" />
    </div>
  );
}

function toLocalDateTime(value: string): string {
  const date = new Date(value);
  const offset = date.getTimezoneOffset() * 60_000;
  return new Date(date.getTime() - offset).toISOString().slice(0, 16);
}
