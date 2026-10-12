'use client';

import { useState, type FormEvent } from 'react';
import Link from 'next/link';
import { ArrowLeft, Check, Copy, KeyRound, Plus, Trash2 } from 'lucide-react';
import { toast } from 'sonner';
import { Badge } from '@/components/ui/badge';
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
import { Input } from '@/components/ui/input';
import { Label } from '@/components/ui/label';
import { Skeleton } from '@/components/ui/skeleton';
import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from '@/components/ui/table';
import MarketEmptyState from '@/components/market/market-empty-state';
import { marketErrorCode } from '@/components/zgi-market/market-format';
import { MARKET_CREDENTIAL_ENV } from '@/components/zgi-market/market-usage';
import {
  useCreateZgiMarketCredential,
  useRevokeZgiMarketCredential,
  useZgiMarketCredentials,
  useZgiMarketMeta,
} from '@/hooks/use-zgi-market';
import { useLocale } from '@/hooks/use-locale';
import { useT } from '@/i18n';
import type { MarketCredential } from '@/services/types/zgi-market';
import { formatDate } from '@/utils/format';

const nameMaxLength = 100;
const columnCount = 7;

export default function ZgiMarketCredentialsPage() {
  const t = useT();
  const { locale } = useLocale();
  const meta = useZgiMarketMeta();
  const admin = meta.data?.admin === true;
  const credentials = useZgiMarketCredentials(admin);
  const create = useCreateZgiMarketCredential();
  const revoke = useRevokeZgiMarketCredential();

  const [creating, setCreating] = useState(false);
  const [name, setName] = useState('');
  // The secret of the credential just created. It lives here until the dialog closes and nowhere else.
  const [secret, setSecret] = useState<{ name: string; value: string } | null>(null);
  const [copied, setCopied] = useState(false);
  const [revoking, setRevoking] = useState<MarketCredential | null>(null);

  const dateLocale = locale.toLowerCase().startsWith('zh') ? 'zh-CN' : 'en-US';
  const date = (value: string | null) =>
    value ? formatDate(value, 'YYYY-MM-DD HH:mm', { locale: dateLocale }) : '—';

  const closeCreate = () => {
    setCreating(false);
    setName('');
    setSecret(null);
    setCopied(false);
    create.reset();
  };

  const submit = (event: FormEvent<HTMLFormElement>) => {
    event.preventDefault();
    const trimmed = name.trim();
    if (!trimmed || create.isPending) {
      return;
    }
    create.mutate(trimmed, {
      onSuccess: created => setSecret({ name: created.name, value: created.secret_key }),
      onError: error =>
        toast.error(
          marketErrorCode(error) === 'MARKET_ACCESS.CREDENTIAL_LIMIT_REACHED'
            ? t('market.credentials.limitReached')
            : t('market.credentials.createFailed')
        ),
    });
  };

  const copy = async () => {
    if (!secret) {
      return;
    }
    try {
      await navigator.clipboard.writeText(secret.value);
      setCopied(true);
    } catch {
      toast.error(t('market.credentials.copyFailed'));
    }
  };

  const rows = credentials.data ?? [];

  return (
    <div className="flex h-full flex-col overflow-y-auto px-6 py-5">
      <Link
        href="/dashboard/market/catalog"
        className="inline-flex w-fit items-center gap-1 text-sm text-muted-foreground hover:text-foreground"
      >
        <ArrowLeft className="size-4" aria-hidden="true" />
        {t('market.catalog.back')}
      </Link>
      <header className="mt-3 flex flex-wrap items-start justify-between gap-3">
        <div className="min-w-0">
          <h1 className="text-xl font-semibold">{t('market.credentials.title')}</h1>
          <p className="mt-1 max-w-3xl text-sm text-muted-foreground">
            {t('market.credentials.description', { env: MARKET_CREDENTIAL_ENV })}
          </p>
        </div>
        {admin ? (
          <Button onClick={() => setCreating(true)}>
            <Plus className="size-4" aria-hidden="true" />
            {t('market.credentials.create')}
          </Button>
        ) : null}
      </header>

      {meta.isPending ? (
        <Skeleton className="mt-6 h-40 w-full" />
      ) : !admin ? (
        <div className="mt-10">
          <MarketEmptyState title={t('market.credentials.adminOnly')} />
        </div>
      ) : (
        <div className="mt-6 overflow-hidden rounded-lg border">
          <div className="overflow-x-auto">
            <Table className="min-w-[900px]">
              <TableHeader>
                <TableRow>
                  <TableHead className="w-[220px]">{t('market.credentials.table.name')}</TableHead>
                  <TableHead className="w-[250px]">{t('market.credentials.table.key')}</TableHead>
                  <TableHead className="w-[130px]">
                    {t('market.credentials.table.source')}
                  </TableHead>
                  <TableHead className="w-[100px]">
                    {t('market.credentials.table.status')}
                  </TableHead>
                  <TableHead className="w-[150px]">
                    {t('market.credentials.table.created')}
                  </TableHead>
                  <TableHead className="w-[150px]">
                    {t('market.credentials.table.lastUsed')}
                  </TableHead>
                  <TableHead className="w-[80px] text-right">
                    {t('market.credentials.table.actions')}
                  </TableHead>
                </TableRow>
              </TableHeader>
              <TableBody>
                {credentials.isPending ? (
                  <TableRow>
                    <TableCell colSpan={columnCount}>
                      <Skeleton className="h-8 w-full" />
                    </TableCell>
                  </TableRow>
                ) : credentials.isError ? (
                  <TableRow>
                    <TableCell colSpan={columnCount} className="py-10 text-center">
                      <p className="text-sm text-muted-foreground">
                        {t('market.credentials.loadFailed')}
                      </p>
                      <Button
                        className="mt-3"
                        variant="outline"
                        size="sm"
                        onClick={() => void credentials.refetch()}
                      >
                        {t('market.catalog.retry')}
                      </Button>
                    </TableCell>
                  </TableRow>
                ) : rows.length === 0 ? (
                  <TableRow>
                    <TableCell colSpan={columnCount} className="py-12 text-center">
                      <KeyRound
                        className="mx-auto size-8 text-muted-foreground"
                        aria-hidden="true"
                      />
                      <p className="mt-3 text-sm font-medium">{t('market.credentials.empty')}</p>
                      <p className="mt-1 text-sm text-muted-foreground">
                        {t('market.credentials.emptyHint')}
                      </p>
                    </TableCell>
                  </TableRow>
                ) : (
                  rows.map(credential => {
                    const managed = credential.source !== 'external_api';
                    const active = credential.status === 'active';
                    return (
                      <TableRow key={credential.id}>
                        <TableCell className="max-w-[220px] truncate font-medium">
                          {credential.name}
                        </TableCell>
                        <TableCell className="font-mono text-xs">
                          {credential.access_key_id}
                          <span className="ml-2 text-muted-foreground">
                            {t('market.credentials.secretHint', { hint: credential.secret_hint })}
                          </span>
                        </TableCell>
                        <TableCell>
                          <Badge variant={managed ? 'subtle' : 'outline'}>
                            {t(
                              managed
                                ? 'market.credentials.source.binding'
                                : 'market.credentials.source.manual'
                            )}
                          </Badge>
                        </TableCell>
                        <TableCell>
                          <Badge variant={active ? 'success' : 'secondary'}>
                            {t(
                              active
                                ? 'market.credentials.status.active'
                                : 'market.credentials.status.revoked'
                            )}
                          </Badge>
                        </TableCell>
                        <TableCell className="text-sm text-muted-foreground">
                          {date(credential.created_at)}
                        </TableCell>
                        <TableCell className="text-sm text-muted-foreground">
                          {credential.last_used_at
                            ? date(credential.last_used_at)
                            : t('market.credentials.neverUsed')}
                        </TableCell>
                        <TableCell className="text-right">
                          {active && !managed ? (
                            <Button
                              variant="ghost"
                              size="sm"
                              isIcon
                              aria-label={t('market.credentials.revokeNamed', {
                                name: credential.name,
                              })}
                              onClick={() => setRevoking(credential)}
                            >
                              <Trash2 className="size-4" aria-hidden="true" />
                            </Button>
                          ) : null}
                        </TableCell>
                      </TableRow>
                    );
                  })
                )}
              </TableBody>
            </Table>
          </div>
        </div>
      )}
      {admin && rows.some(credential => credential.source !== 'external_api') ? (
        <p className="mt-3 text-xs text-muted-foreground">{t('market.credentials.bindingNote')}</p>
      ) : null}

      <Dialog open={creating} onOpenChange={open => !open && closeCreate()}>
        <DialogContent className="max-w-lg">
          {secret ? (
            <>
              <DialogHeader>
                <DialogTitle>{t('market.credentials.created.title')}</DialogTitle>
                <DialogDescription>{t('market.credentials.created.description')}</DialogDescription>
              </DialogHeader>
              <DialogBody className="space-y-3">
                <div className="flex items-start gap-3 rounded-lg border bg-muted/30 p-3">
                  <div className="min-w-0 flex-1 space-y-1">
                    <div className="truncate text-sm font-medium">{secret.name}</div>
                    <div
                      className="break-all font-mono text-xs leading-5"
                      data-testid="zgi-market-credential-secret"
                    >
                      {secret.value}
                    </div>
                  </div>
                  <Button
                    type="button"
                    variant="ghost"
                    size="sm"
                    isIcon
                    aria-label={t('market.credentials.created.copy')}
                    onClick={() => void copy()}
                  >
                    {copied ? (
                      <Check className="size-4" aria-hidden="true" />
                    ) : (
                      <Copy className="size-4" aria-hidden="true" />
                    )}
                  </Button>
                </div>
                <p className="text-xs leading-5 text-muted-foreground">
                  {t('market.credentials.created.usage', { env: MARKET_CREDENTIAL_ENV })}
                </p>
              </DialogBody>
              <DialogFooter>
                <Button onClick={closeCreate}>{t('market.credentials.created.done')}</Button>
              </DialogFooter>
            </>
          ) : (
            <form onSubmit={submit}>
              <DialogHeader>
                <DialogTitle>{t('market.credentials.create')}</DialogTitle>
                <DialogDescription>{t('market.credentials.createDescription')}</DialogDescription>
              </DialogHeader>
              <DialogBody className="space-y-2">
                <Label htmlFor="zgi-market-credential-name">
                  {t('market.credentials.nameLabel')}
                </Label>
                <Input
                  id="zgi-market-credential-name"
                  value={name}
                  maxLength={nameMaxLength}
                  autoComplete="off"
                  placeholder={t('market.credentials.namePlaceholder')}
                  onChange={event => setName(event.target.value)}
                />
              </DialogBody>
              <DialogFooter>
                <Button type="button" variant="ghost" onClick={closeCreate}>
                  {t('market.credentials.cancel')}
                </Button>
                <Button type="submit" disabled={!name.trim() || create.isPending}>
                  {t('market.credentials.create')}
                </Button>
              </DialogFooter>
            </form>
          )}
        </DialogContent>
      </Dialog>

      <ConfirmDialog
        variant="danger"
        open={Boolean(revoking)}
        onOpenChange={open => !open && setRevoking(null)}
        title={t('market.credentials.revokeTitle', { name: revoking?.name ?? '' })}
        description={t('market.credentials.revokeDescription')}
        confirmText={t('market.credentials.revoke')}
        cancelText={t('market.credentials.cancel')}
        loading={revoke.isPending}
        onConfirm={() => {
          if (!revoking) {
            return;
          }
          revoke.mutate(revoking.id, {
            onSuccess: () => toast.success(t('market.credentials.revoked')),
            onError: () => toast.error(t('market.credentials.revokeFailed')),
            onSettled: () => setRevoking(null),
          });
        }}
      />
    </div>
  );
}
