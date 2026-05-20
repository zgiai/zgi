'use client';

import { Suspense, useState, useCallback, useEffect, useMemo } from 'react';
import { useSearchParams, usePathname, useRouter } from 'next/navigation';
import { useT } from '@/i18n';
import { useDebouncedValue } from '@/hooks/use-debounced-value';
import { Input } from '@/components/ui/input';
import { Button } from '@/components/ui/button';
import { Badge } from '@/components/ui/badge';

import { Switch } from '@/components/ui/switch';
import { Skeleton } from '@/components/ui/skeleton';
import { Card, CardContent } from '@/components/ui/card';
import ChannelDialog from '@/components/channel/channel-dialog';
import ChannelConnectivityDialog from '@/components/channel/channel-connectivity-dialog';
import ModelsDialog from '@/components/channel/models-dialog';
import ChannelWalletAdjustDialog from '@/components/channel/channel-wallet-adjust-dialog';
import OfficialChannelGroup from '@/components/channel/official-channel-group';
import { Tooltip, TooltipContent, TooltipTrigger } from '@/components/ui/tooltip';
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuTrigger,
} from '@/components/ui/dropdown-menu';
import { ConfirmDialog } from '@/components/ui/confirm-dialog';
import { Pagination } from '@/components/ui/pagination';
import {
  Ellipsis,
  Pencil,
  Plus,
  Trash2,
  Activity,
  Plug,
  Eye,
  Info,
  Wallet,
  Search,
} from 'lucide-react';
import { useChannels, useUpdateChannel, useDeleteChannel } from '@/hooks';
import type { ChannelDetail, ChannelItem } from '@/services/types/channel';
import { IS_CLOUD } from '@/lib/config';
import { formatChannelCreditPoints, formatChannelCreditUsd } from '@/utils/ai-credits';
import { getChannelProviderOption } from '@/components/channel/channel-provider-selector';

function getChannelModelsCount(channel: ChannelItem): number {
  return Array.isArray(channel.models) ? channel.models.length : 0;
}

function getChannelProviderLabelKey(channel: ChannelItem): string | undefined {
  const provider = channel.channel_provider ?? channel.provider ?? '';
  const option = getChannelProviderOption(provider);

  return option?.labelKey;
}

function getChannelProviderValue(channel: ChannelItem): string {
  return channel.channel_provider ?? channel.provider ?? '-';
}

function ChannelPageContent(): JSX.Element {
  const t = useT('channels');
  const commonT = useT('common');
  const router = useRouter();
  const searchParams = useSearchParams();
  const pathname = usePathname();

  const searchParam = searchParams.get('search') || '';
  const pageParam = Number(searchParams.get('page')) || 1;

  const [search, setSearch] = useState(searchParam);
  const debounced = useDebouncedValue(search, 300);

  const [confirmId, setConfirmId] = useState<string | null>(null);
  const [dialogOpen, setDialogOpen] = useState<boolean>(false);
  const [dialogMode, setDialogMode] = useState<'create' | 'edit'>('create');
  const [dialogInitial, setDialogInitial] = useState<ChannelDetail | null>(null);

  const [connectOpen, setConnectOpen] = useState<boolean>(false);
  const [connectChannel, setConnectChannel] = useState<ChannelDetail | null>(null);
  const [modelsOpen, setModelsOpen] = useState<boolean>(false);
  const [modelsChannel, setModelsChannel] = useState<ChannelDetail | ChannelItem | null>(null);
  const [walletAdjustOpen, setWalletAdjustOpen] = useState<boolean>(false);
  const [walletAdjustChannel, setWalletAdjustChannel] = useState<
    ChannelDetail | ChannelItem | null
  >(null);

  // Default page size is 20
  const pageSize = 20;

  const {
    items: orgItems,
    isLoading: isOrgLoading,
    total: orgTotal,
    total_pages: orgTotalPages,
  } = useChannels({
    search: debounced,
    initialPage: pageParam,
  });

  const { updateChannel } = useUpdateChannel();
  const { deleteChannel, isDeleting } = useDeleteChannel();

  const createQueryString = useCallback(
    (updates: Record<string, string | number | undefined>) => {
      const params = new URLSearchParams(searchParams.toString());
      Object.entries(updates).forEach(([key, value]) => {
        if (value === undefined || value === '' || (key === 'page' && value === 1)) {
          params.delete(key);
        } else {
          params.set(key, String(value));
        }
      });
      return params.toString();
    },
    [searchParams]
  );

  // Helper to update URL params
  const updateUrl = useCallback(
    (updates: Record<string, string | number | undefined>) => {
      const queryString = createQueryString(updates);
      const currentQueryString = searchParams.toString();

      if (queryString === currentQueryString) {
        return;
      }

      router.replace(queryString ? `${pathname}?${queryString}` : pathname, { scroll: false });
    },
    [createQueryString, pathname, router, searchParams]
  );

  // Sync state to URL
  useEffect(() => {
    updateUrl({
      search: debounced,
      page: pageParam,
    });
  }, [debounced, pageParam, updateUrl]);

  const handlePageChange = (newPage: number) => {
    updateUrl({ page: newPage });
  };

  // Track which channel is being toggled (only disable that specific switch)
  const [togglingChannel, setTogglingChannel] = useState<string | null>(null);

  // No need for local filtering if backend handle it
  const filteredOrgItems = orgItems;
  const summary = useMemo(() => {
    const enabled = orgItems.filter(ch => ch.is_enabled).length;
    const modelCount = orgItems.reduce((total, ch) => total + getChannelModelsCount(ch), 0);
    const quotaTotal = orgItems.reduce((total, ch) => total + (ch.remaining_funds ?? 0), 0);

    return {
      total: orgItems.length,
      enabled,
      modelCount,
      quotaTotal,
    };
  }, [orgItems]);

  const openCreate = useCallback(() => {
    setDialogMode('create');
    setDialogInitial(null);
    setDialogOpen(true);
  }, []);

  const openEdit = useCallback((ch: ChannelDetail) => {
    setDialogMode('edit');
    setDialogInitial(ch);
    setDialogOpen(true);
  }, []);

  const onToggle = useCallback(
    async (id: string, next: boolean) => {
      setTogglingChannel(id);
      try {
        await updateChannel(id, { is_enabled: next });
      } finally {
        setTogglingChannel(null);
      }
    },
    [updateChannel]
  );

  return (
    <div className="space-y-5 p-5 h-full overflow-y-auto bg-bg-canvas/40">
      <div className="flex flex-col gap-2">
        <div>
          <div className="text-2xl font-semibold tracking-tight">{t('title')}</div>
          <div className="text-sm text-muted-foreground mt-1">{t('description')}</div>
        </div>
        <div className="grid grid-cols-2 lg:grid-cols-4 gap-3">
          <div className="rounded-md border bg-background px-4 py-3">
            <div className="text-xs text-muted-foreground">{t('overview.total')}</div>
            <div className="mt-1 text-xl font-semibold">{summary.total}</div>
          </div>
          <div className="rounded-md border bg-background px-4 py-3">
            <div className="text-xs text-muted-foreground">{t('overview.enabled')}</div>
            <div className="mt-1 text-xl font-semibold text-green-600">{summary.enabled}</div>
          </div>
          <div className="rounded-md border bg-background px-4 py-3">
            <div className="text-xs text-muted-foreground">{t('overview.models')}</div>
            <div className="mt-1 text-xl font-semibold">{summary.modelCount}</div>
          </div>
          <div className="rounded-md border bg-background px-4 py-3">
            <div className="text-xs text-muted-foreground">{t('overview.quota')}</div>
            <div className="mt-1 flex items-baseline gap-2">
              <span className="text-xl font-semibold">
                {formatChannelCreditPoints(summary.quotaTotal)}
              </span>
              <span className="text-xs text-muted-foreground">
                {t('credit.points')} / {formatChannelCreditUsd(summary.quotaTotal)}
              </span>
            </div>
          </div>
        </div>
      </div>

      <div className="flex flex-col sm:flex-row sm:items-center justify-between gap-3 rounded-md border bg-background p-3">
        <div className="flex items-center gap-3 flex-1">
          <div className="relative w-full max-w-[420px]">
            <Search className="pointer-events-none absolute left-3 top-1/2 h-4 w-4 -translate-y-1/2 text-muted-foreground" />
            <Input
              placeholder={t('searchPlaceholder')}
              value={search}
              onChange={e => setSearch(e.target.value)}
              className="pl-9"
            />
          </div>
        </div>

        <div className="flex items-center gap-2">
          {/* <Button variant="outline">{t('actions.batch')}</Button> */}
          <Button onClick={openCreate}>
            <Plus className="h-4 w-4 mr-1" />
            {t('actions.add')}
          </Button>
        </div>
      </div>

      <div className="space-y-6">
        {IS_CLOUD && <OfficialChannelGroup />}

        <div>
          <div className="text-base font-semibold mb-2 flex items-center gap-2">
            <span>{t('groups.user')}</span>
            <Tooltip>
              <TooltipTrigger asChild>
                <span className="inline-flex items-center">
                  <Info className="h-4 w-4" />
                </span>
              </TooltipTrigger>
              <TooltipContent
                side="top"
                className="max-w-[360px] whitespace-normal break-words leading-5 text-left p-2"
              >
                {t('groupsTips.user')}
              </TooltipContent>
            </Tooltip>
          </div>
          <div className="space-y-4">
            <div className="border rounded-md overflow-hidden p-1 md:p-2 bg-background min-h-[140px]">
              {isOrgLoading && orgItems.length === 0 ? (
                <div className="grid grid-cols-1 md:grid-cols-2 lg:grid-cols-3 gap-4 p-2">
                  {Array.from({ length: 6 }).map((_, i) => (
                    <Card key={i} className="border-none shadow-sm">
                      <CardContent className="p-4 flex flex-col gap-3">
                        <div className="flex items-center gap-3">
                          <Skeleton className="h-10 w-10 rounded-lg" />
                          <div className="space-y-2">
                            <Skeleton className="h-4 w-24" />
                            <Skeleton className="h-3 w-16" />
                          </div>
                        </div>
                        <div className="flex justify-between items-center mt-2">
                          <Skeleton className="h-4 w-20" />
                          <Skeleton className="h-5 w-9 rounded-full" />
                        </div>
                      </CardContent>
                    </Card>
                  ))}
                </div>
              ) : (
                <div className="grid grid-cols-1 md:grid-cols-2 lg:grid-cols-3 2xl:grid-cols-4 gap-4 p-2">
                  {filteredOrgItems.map(ch => (
                    <Card
                      key={ch.id}
                      className="group border shadow-sm hover:shadow-md transition-all duration-200 bg-background relative rounded-md"
                    >
                      <CardContent className="p-4 flex flex-col justify-between h-full gap-4">
                        <div className="flex items-start justify-between gap-2">
                          <div className="flex items-center gap-3 overflow-hidden">
                            <div className="flex flex-col overflow-hidden">
                              <div className="flex items-center gap-2 overflow-hidden">
                                <div className="font-semibold text-sm truncate group-hover:text-primary transition-colors">
                                  {ch.name}
                                </div>
                                <Badge
                                  variant="secondary"
                                  className={
                                    ch.is_enabled
                                      ? 'h-5 shrink-0 rounded-sm bg-green-50 px-1.5 text-[10px] font-medium text-green-700'
                                      : 'h-5 shrink-0 rounded-sm px-1.5 text-[10px] font-medium text-muted-foreground'
                                  }
                                >
                                  {ch.is_enabled ? t('status.enabled') : t('status.disabled')}
                                </Badge>
                              </div>
                              <div className="mt-1 text-xs text-muted-foreground truncate">
                                {getChannelProviderLabelKey(ch)
                                  ? t(getChannelProviderLabelKey(ch) as never)
                                  : getChannelProviderValue(ch)}
                              </div>
                            </div>
                          </div>
                          <div className="flex items-center gap-1">
                            <Switch
                              checked={Boolean(ch.is_enabled)}
                              onCheckedChange={checked => onToggle(ch.id, checked as boolean)}
                              disabled={togglingChannel === ch.id}
                              className="data-[state=checked]:bg-green-600 scale-75"
                            />
                            <DropdownMenu>
                              <DropdownMenuTrigger asChild>
                                <Button variant="ghost" size="sm" className="h-8 w-8 p-0">
                                  <Ellipsis className="h-4 w-4" />
                                </Button>
                              </DropdownMenuTrigger>
                              <DropdownMenuContent align="end" className="w-36">
                                <DropdownMenuItem onClick={() => openEdit(ch as ChannelDetail)}>
                                  <Pencil className="h-4 w-4" />
                                  {t('actions.edit')}
                                </DropdownMenuItem>
                                <DropdownMenuItem
                                  onClick={() => {
                                    setConnectChannel(ch as ChannelDetail);
                                    setConnectOpen(true);
                                  }}
                                >
                                  <Activity className="h-4 w-4" />
                                  {t('actions.testConnectivity')}
                                </DropdownMenuItem>
                                <DropdownMenuItem
                                  onClick={() => {
                                    setWalletAdjustChannel(ch);
                                    setWalletAdjustOpen(true);
                                  }}
                                >
                                  <Wallet className="h-4 w-4" />
                                  {t('walletAdjust.title')}
                                </DropdownMenuItem>
                                <DropdownMenuItem
                                  variant="destructive"
                                  onClick={() => setConfirmId(ch.id)}
                                  className="text-destructive"
                                >
                                  <Trash2 className="h-4 w-4 text-destructive" />{' '}
                                  {t('actions.delete')}
                                </DropdownMenuItem>
                              </DropdownMenuContent>
                            </DropdownMenu>
                          </div>
                        </div>

                        <div className="grid grid-cols-2 gap-3 text-sm">
                          <div className="rounded-md border bg-muted/20 px-3 py-2">
                            <div className="text-xs text-muted-foreground">{t('table.models')}</div>
                            <div className="mt-1 flex items-center gap-1">
                              <span className="font-semibold">{getChannelModelsCount(ch)}</span>
                              <Button
                                variant="ghost"
                                size="sm"
                                className="h-6 w-6 p-0 text-muted-foreground hover:text-primary"
                                onClick={() => {
                                  setModelsChannel(ch);
                                  setModelsOpen(true);
                                }}
                                aria-label={t('modelsDialog.title')}
                              >
                                <Eye className="h-3.5 w-3.5" />
                              </Button>
                            </div>
                          </div>
                          <div className="rounded-md border bg-muted/20 px-3 py-2">
                            <div className="text-xs text-muted-foreground">{t('table.quota')}</div>
                            <div className="mt-1 font-semibold leading-tight">
                              {formatChannelCreditPoints(ch.remaining_funds ?? 0)}
                              <span className="ml-1 text-xs font-normal text-muted-foreground">
                                {t('credit.points')}
                              </span>
                            </div>
                            <div className="text-xs text-muted-foreground">
                              {t('credit.approxUsd', {
                                amount: formatChannelCreditUsd(ch.remaining_funds ?? 0),
                              })}
                            </div>
                          </div>
                        </div>

                        <div className="flex items-center justify-between border-t pt-3 text-xs text-muted-foreground">
                          <div className="flex items-center gap-4">
                            <span>
                              {t('table.priority')}:{' '}
                              <b className="text-foreground">{ch.priority ?? 0}</b>
                            </span>
                            <span>
                              {t('table.weight')}:{' '}
                              <b className="text-foreground">{ch.weight ?? 0}</b>
                            </span>
                          </div>
                          <Button
                            variant="ghost"
                            size="sm"
                            className="h-7 px-2 text-xs"
                            onClick={() => {
                              setConnectChannel(ch as ChannelDetail);
                              setConnectOpen(true);
                            }}
                          >
                            <Activity className="mr-1 h-3.5 w-3.5" />
                            {t('actions.testConnectivityShort')}
                          </Button>
                        </div>
                      </CardContent>
                    </Card>
                  ))}
                  {!isOrgLoading && filteredOrgItems.length === 0 && (
                    <div className="col-span-full py-16 flex flex-col items-center justify-center text-center gap-3 bg-background rounded-lg border border-dashed">
                      <div className="mx-auto w-16 h-16 bg-muted/50 rounded-full flex items-center justify-center">
                        <Plug className="h-8 w-8 text-muted-foreground" />
                      </div>
                      <div className="text-base font-medium">{t('empty.title')}</div>
                      <div className="text-sm text-muted-foreground max-w-[400px]">
                        {t('empty.description')}
                      </div>
                      <Button onClick={openCreate} size="sm" className="mt-2">
                        <Plus className="h-4 w-4 mr-1" />
                        {t('actions.add')}
                      </Button>
                    </div>
                  )}
                </div>
              )}
            </div>
            {orgTotalPages > 1 && (
              <div className="flex justify-center pt-2">
                <Pagination
                  currentPage={pageParam}
                  totalPages={orgTotalPages}
                  total={orgTotal}
                  pageSize={pageSize}
                  onPageChange={handlePageChange}
                  showInfo
                  renderInfo={(start, end, total) =>
                    commonT('pagination.info', { start, end, total })
                  }
                />
              </div>
            )}
          </div>
        </div>
      </div>

      <ConfirmDialog
        variant="warning"
        open={Boolean(confirmId)}
        onOpenChange={open => !open && setConfirmId(null)}
        title={t('actions.confirmDeleteTitle')}
        description={t('actions.confirmDeleteDesc')}
        confirmText={t('actions.confirm')}
        cancelText={t('actions.cancel')}
        loading={isDeleting}
        onConfirm={async () => {
          if (confirmId) await deleteChannel(confirmId);
          setConfirmId(null);
        }}
      />

      <ChannelDialog
        open={dialogOpen}
        onOpenChange={setDialogOpen}
        mode={dialogMode}
        initial={dialogInitial}
      />

      <ChannelConnectivityDialog
        open={connectOpen}
        onOpenChange={open => {
          setConnectOpen(open);
          if (!open) setConnectChannel(null);
        }}
        channel={connectChannel}
      />
      <ModelsDialog
        open={modelsOpen}
        onOpenChange={open => {
          setModelsOpen(open);
          if (!open) setModelsChannel(null);
        }}
        channel={modelsChannel}
      />
      <ChannelWalletAdjustDialog
        open={walletAdjustOpen}
        onOpenChange={open => {
          setWalletAdjustOpen(open);
          if (!open) setWalletAdjustChannel(null);
        }}
        channel={
          walletAdjustChannel
            ? {
                id: walletAdjustChannel.id,
                name: walletAdjustChannel.name,
              }
            : null
        }
      />
    </div>
  );
}

export default function ChannelPage(): JSX.Element {
  return (
    <Suspense fallback={null}>
      <ChannelPageContent />
    </Suspense>
  );
}
