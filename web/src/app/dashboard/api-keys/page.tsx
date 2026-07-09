'use client';

import React, { useMemo, useState, useCallback } from 'react';
import { useT } from '@/i18n';
import { useDebouncedValue } from '@/hooks/use-debounced-value';
import ApiKeyDialog from '@/components/apikey/apikey-dialog';
import { ApiKeyUsageGuide } from '@/components/apikey/apikey-usage-guide';
import { Input } from '@/components/ui/input';
import { Button } from '@/components/ui/button';
import { Badge } from '@/components/ui/badge';
import { Switch } from '@/components/ui/switch';
import { Skeleton } from '@/components/ui/skeleton';
import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from '@/components/ui/table';
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuTrigger,
} from '@/components/ui/dropdown-menu';
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from '@/components/ui/select';
import { ConfirmDialog } from '@/components/ui/confirm-dialog';
import { Copy, Ellipsis, Plus, Trash2, Check, Pencil } from 'lucide-react';
import { useApiKeys, useUpdateApiKey, useDeleteApiKey } from '@/hooks';
import { ApiKeyStatus } from '@/services/types/apikey';
import type { ApiKeyItem } from '@/services/types/apikey';
import { Pagination } from '@/components/ui/pagination';
import { formatDate } from '@/utils/format';

const TABLE_COLUMN_COUNT = 11;

type ApiKeyListStatus = ApiKeyStatus | 'expired' | 'exhausted';

function getStatusMeta(key: ApiKeyItem): {
  label: ApiKeyListStatus;
  variant: 'default' | 'secondary' | 'destructive' | 'outline';
} {
  if (key.status !== ApiKeyStatus.Active) {
    return {
      label: key.status,
      variant: key.status === ApiKeyStatus.Inactive ? 'secondary' : 'outline',
    };
  }

  if (key.expires_at && Date.parse(key.expires_at) <= Date.now()) {
    return { label: 'expired', variant: 'destructive' };
  }

  if (key.quota_limit != null && key.remain_quota <= 0) {
    return { label: 'exhausted', variant: 'destructive' };
  }

  return { label: key.status, variant: 'default' };
}

function splitAllowIps(allowIps?: string): string[] {
  if (!allowIps?.trim()) {
    return [];
  }

  return allowIps
    .split(/[,\n]/)
    .map(ip => ip.trim())
    .filter(Boolean);
}

export default function ApiKeysPage(): JSX.Element {
  const t = useT('apikeys');

  const [search, setSearch] = useState('');
  const debounced = useDebouncedValue(search, 300);
  const [status, setStatus] = useState<'all' | ApiKeyStatus>('all');
  // const [startDate, setStartDate] = useState<string>('');
  // const [endDate, setEndDate] = useState<string>('');
  const [page, setPage] = useState(1);
  const pageSize = 20;

  const statusParam = status === 'all' ? undefined : status;
  const { items, isLoading, isFetching, total, total_pages } = useApiKeys({
    limit: pageSize,
    page,
    search: debounced,
    status: statusParam,
    // start_date: startDate || undefined,
    // end_date: endDate || undefined,
  });
  const { updateApiKey } = useUpdateApiKey();
  const { deleteApiKey, isDeleting } = useDeleteApiKey();

  // Track which key is being toggled
  const [togglingKey, setTogglingKey] = useState<string | null>(null);
  // Track copied key ID for feedback
  const [copiedId, setCopiedId] = useState<string | null>(null);

  const filtered = useMemo(() => {
    return items;
  }, [items]);

  const [confirmId, setConfirmId] = useState<string | null>(null);
  const [dialogOpen, setDialogOpen] = useState<boolean>(false);
  const [dialogMode, setDialogMode] = useState<'create' | 'edit'>('create');
  const [dialogInitial, setDialogInitial] = useState<ApiKeyItem | null>(null);

  const openCreate = useCallback(() => {
    setDialogMode('create');
    setDialogInitial(null);
    setDialogOpen(true);
  }, []);

  const openEdit = useCallback((key: ApiKeyItem) => {
    setDialogMode('edit');
    setDialogInitial(key);
    setDialogOpen(true);
  }, []);

  const onToggle = useCallback(
    async (key: ApiKeyItem, enabled: boolean) => {
      setTogglingKey(key.id);
      try {
        await updateApiKey(key.id, {
          status: enabled ? ApiKeyStatus.Active : ApiKeyStatus.Inactive,
        });
      } finally {
        setTogglingKey(null);
      }
    },
    [updateApiKey]
  );

  const onCopyKey = useCallback(async (key: ApiKeyItem) => {
    if (!key.key) {
      return;
    }

    try {
      await navigator.clipboard.writeText(key.key);
      setCopiedId(key.id);
      setTimeout(() => setCopiedId(null), 2000);
    } catch {
      // Fallback for older browsers
      const textarea = document.createElement('textarea');
      textarea.value = key.key;
      document.body.appendChild(textarea);
      textarea.select();
      document.execCommand('copy');
      document.body.removeChild(textarea);
      setCopiedId(key.id);
      setTimeout(() => setCopiedId(null), 2000);
    }
  }, []);

  return (
    <div className="space-y-5 p-4">
      <div>
        <div className="text-xl font-semibold">{t('title')}</div>
        <div className="text-sm text-muted-foreground">{t('description')}</div>
      </div>

      <ApiKeyUsageGuide />

      <div className="flex items-center justify-between gap-3 flex-wrap">
        <div className="flex items-center gap-3 flex-wrap">
          <div className="w-[240px]">
            <Input
              placeholder={t('searchPlaceholder')}
              value={search}
              onChange={e => setSearch(e.target.value)}
            />
          </div>
          <div className="w-[140px]">
            <Select value={status} onValueChange={v => setStatus(v as 'all' | ApiKeyStatus)}>
              <SelectTrigger>
                <SelectValue placeholder={t('filters.allStatus')} />
              </SelectTrigger>
              <SelectContent>
                <SelectItem value="all">{t('filters.allStatus')}</SelectItem>
                <SelectItem value="active">{t('filters.active')}</SelectItem>
                <SelectItem value="inactive">{t('filters.inactive')}</SelectItem>
              </SelectContent>
            </Select>
          </div>
          {/* Date range filters */}
          {/* <div className="flex items-center gap-2">
            <div className="relative">
              <Input
                type="date"
                value={startDate}
                onChange={e => setStartDate(e.target.value)}
                className="w-[150px]"
                placeholder={t('filters.startDate')}
              />
            </div>
            <span className="text-muted-foreground">-</span>
            <div className="relative">
              <Input
                type="date"
                value={endDate}
                onChange={e => setEndDate(e.target.value)}
                className="w-[150px]"
                placeholder={t('filters.endDate')}
              />
            </div>
          </div> */}
        </div>

        <div className="flex items-center gap-2">
          <Button onClick={openCreate}>
            <Plus className="h-4 w-4" />
            {t('actions.add')}
          </Button>
        </div>
      </div>

      <div className="border rounded-lg overflow-hidden">
        {isLoading && filtered.length === 0 ? (
          <Table className="min-w-[1320px]">
            <TableHeader>
              <TableRow>
                <TableHead className="w-[180px]">{t('table.name')}</TableHead>
                <TableHead className="w-[100px]">{t('table.status')}</TableHead>
                <TableHead className="w-[220px]">{t('table.key')}</TableHead>
                <TableHead className="w-[150px]">{t('table.quotaUsage')}</TableHead>
                <TableHead className="w-[150px]">{t('table.modelLimits')}</TableHead>
                <TableHead className="w-[140px]">{t('table.ipLimits')}</TableHead>
                <TableHead className="w-[160px]">{t('table.createdAt')}</TableHead>
                <TableHead className="w-[160px]">{t('table.accessedAt')}</TableHead>
                <TableHead className="w-[160px]">{t('table.expiresAt')}</TableHead>
                <TableHead className="w-[80px] text-right">{t('table.enabled')}</TableHead>
                <TableHead className="w-[60px] text-right">{t('table.actions')}</TableHead>
              </TableRow>
            </TableHeader>
            <TableBody>
              {Array.from({ length: 5 }).map((_, i) => (
                <TableRow key={i}>
                  <TableCell>
                    <Skeleton className="h-4 w-32" />
                  </TableCell>
                  <TableCell>
                    <Skeleton className="h-5 w-14 rounded-full" />
                  </TableCell>
                  <TableCell>
                    <div className="flex items-center gap-2">
                      <Skeleton className="h-6 w-36 rounded" />
                      <Skeleton className="h-6 w-6 rounded" />
                    </div>
                  </TableCell>
                  <TableCell>
                    <Skeleton className="h-4 w-24" />
                  </TableCell>
                  <TableCell>
                    <Skeleton className="h-5 w-28" />
                  </TableCell>
                  <TableCell>
                    <Skeleton className="h-5 w-24" />
                  </TableCell>
                  <TableCell>
                    <Skeleton className="h-4 w-28" />
                  </TableCell>
                  <TableCell>
                    <Skeleton className="h-4 w-28" />
                  </TableCell>
                  <TableCell>
                    <Skeleton className="h-4 w-28" />
                  </TableCell>
                  <TableCell className="text-right">
                    <Skeleton className="h-5 w-9 rounded-full ml-auto" />
                  </TableCell>
                  <TableCell className="text-right">
                    <Skeleton className="h-8 w-8 rounded ml-auto" />
                  </TableCell>
                </TableRow>
              ))}
            </TableBody>
          </Table>
        ) : (
          <Table className="min-w-[1320px]">
            <TableHeader>
              <TableRow>
                <TableHead className="w-[180px]">{t('table.name')}</TableHead>
                <TableHead className="w-[100px]">{t('table.status')}</TableHead>
                <TableHead className="w-[220px]">{t('table.key')}</TableHead>
                <TableHead className="w-[150px]">{t('table.quotaUsage')}</TableHead>
                <TableHead className="w-[150px]">{t('table.modelLimits')}</TableHead>
                <TableHead className="w-[140px]">{t('table.ipLimits')}</TableHead>
                <TableHead className="w-[160px]">{t('table.createdAt')}</TableHead>
                <TableHead className="w-[160px]">{t('table.accessedAt')}</TableHead>
                <TableHead className="w-[160px]">{t('table.expiresAt')}</TableHead>
                <TableHead className="w-[80px] text-right">{t('table.enabled')}</TableHead>
                <TableHead className="w-[60px] text-right">{t('table.actions')}</TableHead>
              </TableRow>
            </TableHeader>
            <TableBody>
              {filtered.map(key => {
                const statusMeta = getStatusMeta(key);
                const modelLimits =
                  key.model_limits_enabled && key.model_limits ? key.model_limits : [];
                const allowIps = splitAllowIps(key.allow_ips);

                return (
                  <TableRow key={key.id} data-loading={isLoading || isFetching}>
                    <TableCell>
                      <div className="max-w-[160px] truncate text-sm font-medium">{key.name}</div>
                    </TableCell>
                    <TableCell>
                      <Badge variant={statusMeta.variant} className="text-xs">
                        {t(`status.${statusMeta.label}`)}
                      </Badge>
                    </TableCell>
                    <TableCell>
                      <div className="flex items-center gap-2">
                        <code className="block max-w-[180px] truncate rounded bg-muted px-2 py-1 font-mono text-xs">
                          {key.key_masked}
                        </code>
                        {key.key ? (
                          <Button
                            variant="ghost"
                            isIcon
                            className="h-6 w-6"
                            aria-label={t('actions.copy')}
                            onClick={() => onCopyKey(key)}
                          >
                            {copiedId === key.id ? (
                              <Check className="h-3 w-3 text-green-500" />
                            ) : (
                              <Copy className="h-3 w-3" />
                            )}
                          </Button>
                        ) : null}
                      </div>
                    </TableCell>
                    <TableCell>
                      <div className="space-y-0.5">
                        <div className="font-medium">
                          {key.quota_limit == null
                            ? t('table.unlimitedQuota')
                            : `${key.remain_quota.toLocaleString()} / ${key.quota_limit.toLocaleString()}`}
                        </div>
                        <div className="text-xs text-muted-foreground">
                          {t('table.usedQuota')} {key.used_quota.toLocaleString()}
                        </div>
                      </div>
                    </TableCell>
                    <TableCell>
                      {modelLimits.length > 0 ? (
                        <div className="flex max-w-[140px] flex-wrap gap-1">
                          {modelLimits.slice(0, 2).map(model => (
                            <Badge
                              key={model}
                              variant="outline"
                              title={model}
                              className="max-w-[110px] truncate text-xs"
                            >
                              {model}
                            </Badge>
                          ))}
                          {modelLimits.length > 2 ? (
                            <Badge variant="outline" className="text-xs">
                              +{modelLimits.length - 2}
                            </Badge>
                          ) : null}
                        </div>
                      ) : (
                        <span className="text-muted-foreground">{t('table.noLimit')}</span>
                      )}
                    </TableCell>
                    <TableCell>
                      {allowIps.length > 0 ? (
                        <div className="flex max-w-[130px] flex-wrap gap-1">
                          {allowIps.slice(0, 1).map(ip => (
                            <Badge
                              key={ip}
                              variant="outline"
                              title={ip}
                              className="max-w-[100px] truncate text-xs"
                            >
                              {ip}
                            </Badge>
                          ))}
                          {allowIps.length > 1 ? (
                            <Badge variant="outline" className="text-xs">
                              +{allowIps.length - 1}
                            </Badge>
                          ) : null}
                        </div>
                      ) : (
                        <span className="text-muted-foreground">{t('table.unconfigured')}</span>
                      )}
                    </TableCell>
                    <TableCell>{formatDate(key.created_at)}</TableCell>
                    <TableCell>
                      {key.accessed_at ? (
                        formatDate(key.accessed_at)
                      ) : (
                        <span className="text-muted-foreground">{t('table.neverAccessed')}</span>
                      )}
                    </TableCell>
                    <TableCell>
                      {key.expires_at ? (
                        formatDate(key.expires_at)
                      ) : (
                        <span className="text-muted-foreground">{t('table.noExpiration')}</span>
                      )}
                    </TableCell>
                    <TableCell className="text-right">
                      <Switch
                        checked={key.status === 'active'}
                        onCheckedChange={checked => onToggle(key, checked)}
                        disabled={togglingKey === key.id}
                        className="data-[state=checked]:bg-green-600"
                      />
                    </TableCell>
                    <TableCell className="text-right">
                      <DropdownMenu>
                        <DropdownMenuTrigger asChild>
                          <Button variant="ghost" isIcon>
                            <Ellipsis className="h-4 w-4" />
                          </Button>
                        </DropdownMenuTrigger>
                        <DropdownMenuContent align="end" className="w-36">
                          {key.key ? (
                            <DropdownMenuItem onClick={() => onCopyKey(key)}>
                              <Copy className="h-4 w-4 mr-2" /> {t('actions.copy')}
                            </DropdownMenuItem>
                          ) : null}
                          <DropdownMenuItem onClick={() => openEdit(key)}>
                            <Pencil className="h-4 w-4 mr-2" /> {t('actions.edit')}
                          </DropdownMenuItem>
                          <DropdownMenuItem
                            onClick={() => setConfirmId(key.id)}
                            className="text-destructive"
                          >
                            <Trash2 className="h-4 w-4 mr-2" /> {t('actions.delete')}
                          </DropdownMenuItem>
                        </DropdownMenuContent>
                      </DropdownMenu>
                    </TableCell>
                  </TableRow>
                );
              })}
              {!isLoading && filtered.length === 0 && (
                <TableRow>
                  <TableCell
                    colSpan={TABLE_COLUMN_COUNT}
                    className="text-center text-muted-foreground py-10"
                  >
                    {t('empty')}
                  </TableCell>
                </TableRow>
              )}
            </TableBody>
          </Table>
        )}
      </div>

      {total_pages > 1 && (
        <Pagination
          currentPage={page}
          totalPages={total_pages}
          total={total}
          pageSize={pageSize}
          onPageChange={setPage}
          showInfo
          showJump
          className="mt-4"
        />
      )}

      <ConfirmDialog
        variant="danger"
        open={Boolean(confirmId)}
        onOpenChange={open => !open && setConfirmId(null)}
        title={t('actions.confirmDeleteTitle')}
        description={t('actions.confirmDeleteDesc')}
        confirmText={t('actions.confirm')}
        cancelText={t('actions.cancel')}
        loading={isDeleting}
        onConfirm={async () => {
          if (confirmId) await deleteApiKey(confirmId);
          setConfirmId(null);
        }}
      />

      <ApiKeyDialog
        open={dialogOpen}
        onOpenChange={setDialogOpen}
        mode={dialogMode}
        initial={dialogInitial}
      />
    </div>
  );
}
