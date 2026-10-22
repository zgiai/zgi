'use client';

import * as React from 'react';
import { Ban, Download, Loader2, PlayCircle, Search, Trash2, UserRound } from 'lucide-react';
import { toast } from 'sonner';
import { Badge } from '@/components/ui/badge';
import { Button } from '@/components/ui/button';
import { Card, CardContent } from '@/components/ui/card';
import { ConfirmDialog } from '@/components/ui/confirm-dialog';
import { Input } from '@/components/ui/input';
import { Pagination } from '@/components/ui/pagination';
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from '@/components/ui/select';
import {
  Sheet,
  SheetContent,
  SheetDescription,
  SheetHeader,
  SheetTitle,
} from '@/components/ui/sheet';
import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from '@/components/ui/table';
import { ToggleGroup, ToggleGroupItem } from '@/components/ui/toggle-group';
import {
  useRuntimeEndUserEvents,
  useRuntimeEndUsers,
} from '@/hooks/runtime-workspace/use-runtime-end-users';
import { useT } from '@/i18n';
import type {
  RuntimeApp,
  RuntimeEndUser,
  RuntimeEndUserStatus,
  RuntimeQuotaWindow,
  RuntimeWindowUsage,
} from '@/services/types/runtime-workspace';

const windows: RuntimeQuotaWindow[] = ['24h', '7d', '30d'];
const pageSize = 20;
type QuotaMode = 'default' | 'custom' | 'unlimited';

const pointsFormat = new Intl.NumberFormat(undefined, { maximumFractionDigits: 3 });

function formatTime(value?: string): string {
  if (!value) return '—';
  const date = new Date(value);
  if (Number.isNaN(date.getTime())) return '—';
  return new Intl.DateTimeFormat(undefined, { dateStyle: 'medium', timeStyle: 'short' }).format(
    date
  );
}

function UsageCell({ usage }: { usage?: RuntimeWindowUsage }) {
  if (!usage) return <span className="text-muted-foreground">—</span>;
  const spent = usage.limit !== undefined && (usage.remaining ?? 0) <= 0;
  return (
    <span className={spent ? 'font-medium text-destructive' : undefined}>
      {pointsFormat.format(usage.points)}
      {usage.limit !== undefined ? (
        <span className="text-muted-foreground"> / {pointsFormat.format(usage.limit)}</span>
      ) : null}
    </span>
  );
}

function StatusBadge({ status }: { status: RuntimeEndUserStatus }) {
  const t = useT('workspace');
  return (
    <Badge variant={status === 'active' ? 'secondary' : 'outline'} className="whitespace-nowrap">
      {t(`runtime.endUsers.status.${status}`)}
    </Badge>
  );
}

function displayName(user: RuntimeEndUser, unnamed: string) {
  return user.external_user_id || `${unnamed} · ${user.subject_id.slice(0, 8)}`;
}

/**
 * @component RuntimeEndUsersTab
 * @category Feature
 * @status Beta
 * @description Manages the end users of a Runtime Workspace (ADR 0010): lists
 * them with their points usage over the quota windows, opens one to see its
 * usage per app and change history, sets its own quota, and suspends or
 * resumes it.
 * @usage Rendered as the End users section of the Runtime Workspace console.
 * @example
 * <RuntimeEndUsersTab runtimeWorkspaceId={id} apps={apps} />
 */
export function RuntimeEndUsersTab({
  runtimeWorkspaceId,
  apps,
}: {
  runtimeWorkspaceId: string;
  apps: RuntimeApp[];
}) {
  const t = useT('workspace');
  const tCommon = useT('common');
  const [searchInput, setSearchInput] = React.useState('');
  const [search, setSearch] = React.useState('');
  const [status, setStatus] = React.useState<RuntimeEndUserStatus | ''>('');
  const [page, setPage] = React.useState(1);
  const [selectedId, setSelectedId] = React.useState<string | null>(null);
  const [statusTarget, setStatusTarget] = React.useState<RuntimeEndUser | null>(null);

  React.useEffect(() => {
    const timer = window.setTimeout(() => {
      setSearch(searchInput.trim());
      setPage(1);
    }, 300);
    return () => window.clearTimeout(timer);
  }, [searchInput]);

  const [eraseTarget, setEraseTarget] = React.useState<RuntimeEndUser | null>(null);
  const { list, update, exportData, erase } = useRuntimeEndUsers(runtimeWorkspaceId, {
    search,
    status,
    page,
    limit: pageSize,
  });
  const users = list.data?.items ?? [];
  const selected = users.find(user => user.subject_id === selectedId) ?? null;
  const appNames = React.useMemo(() => new Map(apps.map(app => [app.agent_id, app.name])), [apps]);

  const downloadExport = async (user: RuntimeEndUser) => {
    const data = await exportData.mutateAsync(user.subject_id);
    const url = URL.createObjectURL(
      new Blob([JSON.stringify(data, null, 2)], { type: 'application/json' })
    );
    const link = document.createElement('a');
    link.href = url;
    link.download = `end-user-${user.subject_id.slice(0, 8)}.json`;
    link.click();
    URL.revokeObjectURL(url);
  };

  const eraseUser = async (user: RuntimeEndUser) => {
    await erase.mutateAsync(user.subject_id);
    setEraseTarget(null);
    setSelectedId(null);
    toast.success(t('runtime.endUsers.erased'));
  };

  const changeStatus = async (user: RuntimeEndUser) => {
    await update.mutateAsync({
      subjectId: user.subject_id,
      patch: { status: user.status === 'suspended' ? 'active' : 'suspended' },
    });
    setStatusTarget(null);
  };

  return (
    <section className="space-y-4">
      <div className="flex flex-col gap-1">
        <h2 className="text-lg font-semibold">{t('runtime.endUsers.title')}</h2>
        <p className="max-w-3xl text-sm leading-6 text-muted-foreground">
          {t('runtime.endUsers.description')}
        </p>
      </div>

      <div className="flex flex-col gap-2 @2xl/console:flex-row @2xl/console:items-center">
        <div className="relative min-w-0 flex-1">
          <Search className="pointer-events-none absolute left-3 top-1/2 size-4 -translate-y-1/2 text-muted-foreground" />
          <Input
            value={searchInput}
            onChange={event => setSearchInput(event.target.value)}
            placeholder={t('runtime.endUsers.searchPlaceholder')}
            className="h-9 pl-9"
          />
        </div>
        <Select
          value={status || 'all'}
          onValueChange={value => {
            setStatus(value === 'all' ? '' : (value as RuntimeEndUserStatus));
            setPage(1);
          }}
        >
          <SelectTrigger className="h-9 w-full @2xl/console:w-40">
            <SelectValue />
          </SelectTrigger>
          <SelectContent>
            <SelectItem value="all">{t('runtime.endUsers.allStatuses')}</SelectItem>
            <SelectItem value="active">{t('runtime.endUsers.status.active')}</SelectItem>
            <SelectItem value="suspended">{t('runtime.endUsers.status.suspended')}</SelectItem>
          </SelectContent>
        </Select>
      </div>

      <Card className="border-border/80">
        <CardContent className="p-0">
          {list.isLoading ? (
            <div className="flex items-center justify-center py-16 text-muted-foreground">
              <Loader2 className="size-5 animate-spin" />
            </div>
          ) : users.length === 0 ? (
            <div className="flex flex-col items-center gap-2 px-6 py-16 text-center">
              <UserRound className="size-8 text-muted-foreground" />
              <p className="text-sm font-medium">{t('runtime.endUsers.emptyTitle')}</p>
              <p className="max-w-md text-xs leading-5 text-muted-foreground">
                {t('runtime.endUsers.emptyDescription')}
              </p>
            </div>
          ) : (
            <div className="overflow-x-auto">
              <Table>
                <TableHeader>
                  <TableRow>
                    <TableHead>{t('runtime.endUsers.columns.user')}</TableHead>
                    <TableHead>{t('runtime.endUsers.columns.status')}</TableHead>
                    <TableHead>{t('runtime.endUsers.columns.lastSeen')}</TableHead>
                    {windows.map(window => (
                      <TableHead key={window} className="whitespace-nowrap text-right">
                        {t(`runtime.quota.windows.${window}`)}
                      </TableHead>
                    ))}
                    <TableHead className="text-right">{tCommon('actions')}</TableHead>
                  </TableRow>
                </TableHeader>
                <TableBody>
                  {users.map(user => (
                    <TableRow
                      key={user.subject_id}
                      className="cursor-pointer"
                      onClick={() => setSelectedId(user.subject_id)}
                    >
                      <TableCell className="max-w-64">
                        <div className="truncate font-medium">
                          {displayName(user, t('runtime.endUsers.unnamed'))}
                        </div>
                        {user.quota_source !== 'default' ? (
                          <div className="mt-0.5 text-xs text-muted-foreground">
                            {t(`runtime.endUsers.quotaSource.${user.quota_source}`)}
                          </div>
                        ) : null}
                      </TableCell>
                      <TableCell>
                        <StatusBadge status={user.status} />
                      </TableCell>
                      <TableCell className="whitespace-nowrap text-sm text-muted-foreground">
                        {formatTime(user.last_seen_at)}
                      </TableCell>
                      {windows.map(window => (
                        <TableCell
                          key={window}
                          className="whitespace-nowrap text-right tabular-nums"
                        >
                          <UsageCell usage={user.usage.find(item => item.window === window)} />
                        </TableCell>
                      ))}
                      <TableCell className="text-right">
                        <Button
                          type="button"
                          size="sm"
                          variant="ghost"
                          onClick={event => {
                            event.stopPropagation();
                            setStatusTarget(user);
                          }}
                        >
                          {user.status === 'suspended' ? (
                            <PlayCircle className="size-4" />
                          ) : (
                            <Ban className="size-4" />
                          )}
                          {user.status === 'suspended'
                            ? t('runtime.endUsers.resume')
                            : t('runtime.endUsers.suspend')}
                        </Button>
                      </TableCell>
                    </TableRow>
                  ))}
                </TableBody>
              </Table>
            </div>
          )}
        </CardContent>
      </Card>

      {(list.data?.total ?? 0) > pageSize ? (
        <Pagination
          currentPage={page}
          totalPages={Math.ceil((list.data?.total ?? 0) / pageSize)}
          total={list.data?.total ?? 0}
          pageSize={pageSize}
          onPageChange={setPage}
        />
      ) : null}

      <ConfirmDialog
        open={Boolean(statusTarget)}
        onOpenChange={open => {
          if (!open) setStatusTarget(null);
        }}
        title={
          statusTarget?.status === 'suspended'
            ? t('runtime.endUsers.resumeTitle')
            : t('runtime.endUsers.suspendTitle')
        }
        description={
          statusTarget?.status === 'suspended'
            ? t('runtime.endUsers.resumeDescription')
            : t('runtime.endUsers.suspendDescription')
        }
        confirmText={
          statusTarget?.status === 'suspended'
            ? t('runtime.endUsers.resume')
            : t('runtime.endUsers.suspend')
        }
        cancelText={tCommon('cancel')}
        variant={statusTarget?.status === 'suspended' ? 'default' : 'warning'}
        loading={update.isPending}
        closeOnConfirm={false}
        onConfirm={() => {
          if (statusTarget) void changeStatus(statusTarget);
        }}
      />

      <EndUserSheet
        runtimeWorkspaceId={runtimeWorkspaceId}
        user={selected}
        appNames={appNames}
        saving={update.isPending}
        onClose={() => setSelectedId(null)}
        onSaveQuota={async quota => {
          if (!selected) return;
          await update.mutateAsync({ subjectId: selected.subject_id, patch: { quota } });
        }}
        exporting={exportData.isPending}
        onExport={() => {
          if (selected) void downloadExport(selected);
        }}
        onErase={() => setEraseTarget(selected)}
      />

      <ConfirmDialog
        open={Boolean(eraseTarget)}
        onOpenChange={open => {
          if (!open) setEraseTarget(null);
        }}
        title={t('runtime.endUsers.eraseTitle')}
        description={t('runtime.endUsers.eraseDescription')}
        confirmText={t('runtime.endUsers.erase')}
        cancelText={tCommon('cancel')}
        variant="danger"
        loading={erase.isPending}
        closeOnConfirm={false}
        onConfirm={() => {
          if (eraseTarget) void eraseUser(eraseTarget);
        }}
      />
    </section>
  );
}

function EndUserSheet({
  runtimeWorkspaceId,
  user,
  appNames,
  saving,
  onClose,
  onSaveQuota,
  exporting,
  onExport,
  onErase,
}: {
  runtimeWorkspaceId: string;
  user: RuntimeEndUser | null;
  appNames: Map<string, string>;
  saving: boolean;
  onClose: () => void;
  onSaveQuota: (quota: RuntimeEndUser['quota'] | null) => Promise<void>;
  exporting: boolean;
  onExport: () => void;
  onErase: () => void;
}) {
  const t = useT('workspace');
  const events = useRuntimeEndUserEvents(runtimeWorkspaceId, user?.subject_id ?? null);
  const [mode, setMode] = React.useState<QuotaMode>('default');
  const [limits, setLimits] = React.useState<Partial<Record<RuntimeQuotaWindow, string>>>({});

  React.useEffect(() => {
    if (!user) return;
    setMode(user.quota_source);
    const next: Partial<Record<RuntimeQuotaWindow, string>> = {};
    for (const limit of user.quota.limits ?? []) next[limit.window] = String(limit.points);
    setLimits(next);
  }, [user]);

  const customLimits = windows.flatMap(window => {
    const value = Math.round(Number.parseFloat(limits[window] ?? '') * 1000) / 1000;
    return Number.isFinite(value) && value > 0 ? [{ window, points: value }] : [];
  });
  const canSave = mode !== 'custom' || customLimits.length > 0;

  return (
    <Sheet open={Boolean(user)} onOpenChange={open => (!open ? onClose() : undefined)}>
      <SheetContent className="w-full overflow-y-auto sm:max-w-xl">
        {user ? (
          <>
            <SheetHeader>
              <SheetTitle className="break-all pr-6">
                {displayName(user, t('runtime.endUsers.unnamed'))}
              </SheetTitle>
              <SheetDescription className="flex flex-wrap items-center gap-2">
                <StatusBadge status={user.status} />
                <span>
                  {t('runtime.endUsers.firstSeen')} {formatTime(user.first_seen_at)}
                </span>
              </SheetDescription>
            </SheetHeader>

            <div className="mt-6 space-y-6">
              <section className="space-y-2">
                <h3 className="text-sm font-medium">{t('runtime.endUsers.usage')}</h3>
                <div className="grid grid-cols-3 gap-2">
                  {windows.map(window => {
                    const usage = user.usage.find(item => item.window === window);
                    return (
                      <div key={window} className="rounded-lg border p-3">
                        <div className="text-xs text-muted-foreground">
                          {t(`runtime.quota.windows.${window}`)}
                        </div>
                        <div className="mt-1 text-base font-semibold tabular-nums">
                          <UsageCell usage={usage} />
                        </div>
                        {usage?.remaining !== undefined ? (
                          <div className="mt-0.5 text-xs text-muted-foreground">
                            {t('runtime.endUsers.remaining', {
                              points: pointsFormat.format(usage.remaining),
                            })}
                          </div>
                        ) : null}
                      </div>
                    );
                  })}
                </div>
              </section>

              <section className="space-y-2">
                <h3 className="text-sm font-medium">{t('runtime.endUsers.byApp')}</h3>
                {user.apps.length === 0 ? (
                  <p className="text-xs text-muted-foreground">{t('runtime.endUsers.noUsage')}</p>
                ) : (
                  <div className="divide-y rounded-lg border">
                    {user.apps.map(app => (
                      <div key={app.app_id} className="flex items-center gap-3 px-3 py-2 text-sm">
                        <span className="min-w-0 flex-1 truncate">
                          {appNames.get(app.app_id) ?? app.app_key ?? app.app_id}
                        </span>
                        {windows.map(window => (
                          <span key={window} className="w-20 text-right tabular-nums">
                            <UsageCell usage={app.usage.find(item => item.window === window)} />
                          </span>
                        ))}
                      </div>
                    ))}
                  </div>
                )}
              </section>

              <section className="space-y-3">
                <div>
                  <h3 className="text-sm font-medium">{t('runtime.endUsers.ownQuota')}</h3>
                  <p className="mt-1 text-xs leading-5 text-muted-foreground">
                    {t('runtime.endUsers.ownQuotaDescription')}
                  </p>
                </div>
                <ToggleGroup
                  type="single"
                  variant="outline"
                  value={mode}
                  onValueChange={value => {
                    if (value) setMode(value as QuotaMode);
                  }}
                  className="justify-start"
                >
                  {(['default', 'custom', 'unlimited'] as const).map(item => (
                    <ToggleGroupItem key={item} value={item} className="px-3 text-xs">
                      {t(`runtime.endUsers.quotaMode.${item}`)}
                    </ToggleGroupItem>
                  ))}
                </ToggleGroup>
                {mode === 'custom' ? (
                  <div className="grid grid-cols-3 gap-2">
                    {windows.map(window => (
                      <label key={window} className="space-y-1">
                        <span className="text-xs text-muted-foreground">
                          {t(`runtime.quota.windows.${window}`)}
                        </span>
                        <Input
                          type="number"
                          min={0.001}
                          step="any"
                          inputMode="decimal"
                          placeholder={t('runtime.quota.unlimited')}
                          value={limits[window] ?? ''}
                          onChange={event => setLimits({ ...limits, [window]: event.target.value })}
                          className="h-9"
                        />
                      </label>
                    ))}
                  </div>
                ) : null}
                <div className="flex justify-end">
                  <Button
                    type="button"
                    size="sm"
                    disabled={saving || !canSave}
                    onClick={() =>
                      void onSaveQuota(
                        mode === 'default'
                          ? null
                          : mode === 'unlimited'
                            ? { unlimited: true }
                            : { limits: customLimits }
                      )
                    }
                  >
                    {saving ? <Loader2 className="size-4 animate-spin" /> : null}
                    {t('runtime.endUsers.saveQuota')}
                  </Button>
                </div>
              </section>

              <section className="space-y-3">
                <div>
                  <h3 className="text-sm font-medium">{t('runtime.endUsers.data')}</h3>
                  <p className="mt-1 text-xs leading-5 text-muted-foreground">
                    {t('runtime.endUsers.dataDescription')}
                  </p>
                </div>
                <div className="flex flex-wrap gap-2">
                  <Button
                    type="button"
                    size="sm"
                    variant="outline"
                    disabled={exporting}
                    onClick={onExport}
                  >
                    {exporting ? (
                      <Loader2 className="size-4 animate-spin" />
                    ) : (
                      <Download className="size-4" />
                    )}
                    {t('runtime.endUsers.export')}
                  </Button>
                  <Button
                    type="button"
                    size="sm"
                    variant="outline"
                    className="text-destructive hover:text-destructive"
                    onClick={onErase}
                  >
                    <Trash2 className="size-4" />
                    {t('runtime.endUsers.erase')}
                  </Button>
                </div>
              </section>

              <section className="space-y-2">
                <h3 className="text-sm font-medium">{t('runtime.endUsers.history')}</h3>
                {(events.data ?? []).length === 0 ? (
                  <p className="text-xs text-muted-foreground">{t('runtime.endUsers.noHistory')}</p>
                ) : (
                  <ul className="space-y-2">
                    {(events.data ?? []).map(event => (
                      <li key={event.id} className="flex items-start justify-between gap-3 text-sm">
                        <span>
                          {t(`runtime.endUsers.events.${event.action}`)}
                          <span className="text-muted-foreground">
                            {' · '}
                            {t(`runtime.endUsers.actors.${event.actor_kind}`)}
                          </span>
                        </span>
                        <span className="shrink-0 text-xs text-muted-foreground">
                          {formatTime(event.created_at)}
                        </span>
                      </li>
                    ))}
                  </ul>
                )}
              </section>
            </div>
          </>
        ) : null}
      </SheetContent>
    </Sheet>
  );
}
