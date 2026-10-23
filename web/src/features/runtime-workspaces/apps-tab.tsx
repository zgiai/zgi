'use client';

import * as React from 'react';
import { Check, Copy, Link2, Loader2, Plus, Search, Trash2 } from 'lucide-react';
import { toast } from 'sonner';
import { Badge } from '@/components/ui/badge';
import { Button } from '@/components/ui/button';
import { Card, CardContent } from '@/components/ui/card';
import { Checkbox } from '@/components/ui/checkbox';
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
  SheetFooter,
  SheetHeader,
  SheetTitle,
} from '@/components/ui/sheet';
import { Skeleton } from '@/components/ui/skeleton';
import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from '@/components/ui/table';
import { Tabs, TabsContent, TabsList, TabsTrigger } from '@/components/ui/tabs';
import {
  AppAvailability,
  CapabilityBadges,
  CatalogTagBadges,
  formatShortDate,
  RuntimeAppIcon,
} from '@/features/runtime-workspaces/app-presentation';
import {
  pointsFormat,
  quotaWindows,
  UsageTrendChart,
} from '@/features/runtime-workspaces/end-user-usage';
import { RuntimePageHeader } from '@/features/runtime-workspaces/runtime-page-header';
import {
  useRuntimeAppCandidates,
  useRuntimeManagedApps,
} from '@/hooks/runtime-workspace/use-runtime-apps';
import { useRuntimeEndUserUsageSeries } from '@/hooks/runtime-workspace/use-runtime-end-users';
import { useT } from '@/i18n';
import { cn } from '@/lib/utils';
import type {
  RuntimeAppType,
  RuntimeManagedApp,
  RuntimeQuotaLimit,
  RuntimeQuotaWindow,
  RuntimeUsageSeries,
} from '@/services/types/runtime-workspace';

const pageSize = 20;

function useDebounced(value: string, delay = 300) {
  const [debounced, setDebounced] = React.useState(value);
  React.useEffect(() => {
    const timer = window.setTimeout(() => setDebounced(value.trim()), delay);
    return () => window.clearTimeout(timer);
  }, [value, delay]);
  return debounced;
}

function capSummary(caps: RuntimeQuotaLimit[], unit: (window: RuntimeQuotaWindow) => string) {
  return caps.map(cap => `${unit(cap.window)} ${pointsFormat.format(cap.points)}`).join(' · ');
}

function parsePolicy(policyJson?: string): Record<string, unknown> {
  try {
    const parsed = JSON.parse(policyJson || '{}');
    return parsed && typeof parsed === 'object' && !Array.isArray(parsed) ? parsed : {};
  } catch {
    return {};
  }
}

/** The policy with an app's per-app caps replaced; no limits removes them. */
function withAppCaps(policyJson: string | undefined, appKey: string, limits: RuntimeQuotaLimit[]) {
  const policy = parsePolicy(policyJson);
  const quota = { ...((policy.end_user_quota as Record<string, unknown>) ?? {}) };
  const apps = (
    (quota.apps as Array<{ app_key: string; limits: RuntimeQuotaLimit[] }>) ?? []
  ).filter(app => app.app_key !== appKey);
  if (limits.length > 0) apps.push({ app_key: appKey, limits });
  if (apps.length > 0) quota.apps = apps;
  else delete quota.apps;
  if (Object.keys(quota).length > 0) policy.end_user_quota = quota;
  else delete policy.end_user_quota;
  return policy;
}

/**
 * @component RuntimeAppsTab
 * @category Feature
 * @status Beta
 * @description The apps an external access exposes (ADR 0012): a paged,
 * searchable table with type, availability, 30-day usage, and per-app caps;
 * adding apps from a server-searched picker within the app limit; removing
 * one or many; and a detail per app with its usage, cap, and capabilities.
 * @usage Rendered as the Apps section of the Runtime Workspace console.
 * @example
 * <RuntimeAppsTab runtimeWorkspaceId={id} policyJson={policy} onSavePolicy={save} savingPolicy={false} />
 */
export function RuntimeAppsTab({
  runtimeWorkspaceId,
  policyJson,
  onSavePolicy,
  savingPolicy,
}: {
  runtimeWorkspaceId: string;
  policyJson?: string;
  onSavePolicy: (policy: Record<string, unknown>) => Promise<void>;
  savingPolicy: boolean;
}) {
  const t = useT('workspace');
  const tCommon = useT('common');
  const [searchInput, setSearchInput] = React.useState('');
  const search = useDebounced(searchInput);
  const [type, setType] = React.useState<RuntimeAppType | ''>('');
  const [state, setState] = React.useState<'callable' | 'unavailable' | ''>('');
  const [page, setPage] = React.useState(1);
  const [selected, setSelected] = React.useState<Set<string>>(new Set());
  const [removing, setRemoving] = React.useState<string[] | null>(null);
  const [adding, setAdding] = React.useState(false);
  const [detailId, setDetailId] = React.useState<string | null>(null);
  React.useEffect(() => {
    setPage(1);
    setSelected(new Set());
  }, [search, type, state]);

  const { page: apps, change } = useRuntimeManagedApps(runtimeWorkspaceId, {
    search,
    type,
    state,
    page,
    limit: pageSize,
  });
  const data = apps.data;
  const items = data?.items ?? [];
  const limit = data?.limit ?? 100;
  const bound = data?.counts.all ?? 0;
  const detail = items.find(app => app.agent_id === detailId) ?? null;
  const pageChecked = items.filter(app => selected.has(app.agent_id)).length;
  const windowUnit = (window: RuntimeQuotaWindow) => t(`runtime.usage.windowShort.${window}`);

  const toggle = (id: string, on: boolean) =>
    setSelected(current => {
      const next = new Set(current);
      if (on) next.add(id);
      else next.delete(id);
      return next;
    });

  const remove = async (ids: string[]) => {
    if (!data) return;
    await change.mutateAsync({ expected_revision: data.revision, app_ids: ids });
    toast.success(t('runtime.messages.agentsRemoved', { count: ids.length }));
    setRemoving(null);
    setSelected(new Set());
    if (detailId && ids.includes(detailId)) setDetailId(null);
  };

  return (
    <section className="space-y-4">
      <RuntimePageHeader
        title={t('runtime.tabs.apps')}
        badge={
          <Badge variant={bound >= limit ? 'warning' : 'subtle'}>
            {t('runtime.apps.limitUsage', { count: bound, limit })}
          </Badge>
        }
        description={t('runtime.apps.description')}
        actions={
          <Button type="button" onClick={() => setAdding(true)} disabled={bound >= limit}>
            <Plus className="size-4" />
            {t('runtime.apps.bind')}
          </Button>
        }
      />

      <div className="flex flex-col gap-2 @3xl/console:flex-row @3xl/console:items-center">
        <div className="relative min-w-0 flex-1">
          <Search className="pointer-events-none absolute left-3 top-1/2 size-4 -translate-y-1/2 text-muted-foreground" />
          <Input
            value={searchInput}
            onChange={event => setSearchInput(event.target.value)}
            placeholder={t('runtime.appsTable.searchPlaceholder')}
            className="h-9 pl-9"
          />
        </div>
        <Select
          value={type || 'all'}
          onValueChange={value => setType(value === 'all' ? '' : (value as RuntimeAppType))}
        >
          <SelectTrigger className="h-9 w-full @3xl/console:w-40">
            <SelectValue />
          </SelectTrigger>
          <SelectContent>
            <SelectItem value="all">
              {t('runtime.appsTable.allTypes', { count: data?.counts.all ?? 0 })}
            </SelectItem>
            <SelectItem value="agent">
              {t('runtime.appsTable.agents', { count: data?.counts.agents ?? 0 })}
            </SelectItem>
            <SelectItem value="digital_human">
              {t('runtime.appsTable.digitalHumans', { count: data?.counts.digital_humans ?? 0 })}
            </SelectItem>
          </SelectContent>
        </Select>
        <Select
          value={state || 'all'}
          onValueChange={value =>
            setState(value === 'all' ? '' : (value as 'callable' | 'unavailable'))
          }
        >
          <SelectTrigger className="h-9 w-full @3xl/console:w-40">
            <SelectValue />
          </SelectTrigger>
          <SelectContent>
            <SelectItem value="all">{t('runtime.appsTable.allStates')}</SelectItem>
            <SelectItem value="callable">{t('runtime.apps.active')}</SelectItem>
            <SelectItem value="unavailable">
              {t('runtime.appsTable.unavailable', { count: data?.counts.unavailable ?? 0 })}
            </SelectItem>
          </SelectContent>
        </Select>
      </div>

      {selected.size > 0 ? (
        <div className="flex flex-wrap items-center gap-2 rounded-lg border border-primary/30 bg-primary/5 px-3 py-2">
          <span className="text-sm font-medium">
            {t('runtime.apps.selectedCount', { count: selected.size })}
          </span>
          <Button type="button" size="sm" variant="ghost" onClick={() => setSelected(new Set())}>
            {t('runtime.apps.clearSelection')}
          </Button>
          <Button
            type="button"
            size="sm"
            variant="outline"
            className="ml-auto text-destructive hover:text-destructive"
            onClick={() => setRemoving([...selected])}
          >
            <Trash2 className="size-4" />
            {t('runtime.apps.removeSelected', { count: selected.size })}
          </Button>
        </div>
      ) : null}

      <Card className="border-border/80">
        <CardContent className="p-0">
          {apps.isLoading ? (
            <div className="space-y-2 p-4">
              {Array.from({ length: 5 }).map((_, index) => (
                <Skeleton key={index} className="h-12 w-full" />
              ))}
            </div>
          ) : items.length === 0 ? (
            <div className="flex flex-col items-center gap-2 px-6 py-16 text-center">
              <Link2 className="size-8 text-muted-foreground" />
              <p className="text-sm font-medium">
                {bound === 0 ? t('runtime.apps.empty') : t('runtime.appsTable.noMatch')}
              </p>
              <p className="max-w-md text-xs leading-5 text-muted-foreground">
                {bound === 0
                  ? t('runtime.apps.emptyDescription')
                  : t('runtime.appsTable.noMatchDescription')}
              </p>
            </div>
          ) : (
            <div className={cn('overflow-x-auto', apps.isFetching && 'opacity-70')}>
              <Table>
                <TableHeader>
                  <TableRow>
                    <TableHead className="w-10">
                      <Checkbox
                        aria-label={t('runtime.appsTable.selectPage')}
                        checked={pageChecked > 0 && pageChecked === items.length}
                        onCheckedChange={value => {
                          for (const app of items) toggle(app.agent_id, value === true);
                        }}
                      />
                    </TableHead>
                    <TableHead>{t('runtime.appsTable.columns.app')}</TableHead>
                    <TableHead>{t('runtime.appsTable.columns.type')}</TableHead>
                    <TableHead>{t('runtime.appsTable.columns.status')}</TableHead>
                    <TableHead className="text-right">
                      {t('runtime.appsTable.columns.points')}
                    </TableHead>
                    <TableHead className="text-right">
                      {t('runtime.appsTable.columns.calls')}
                    </TableHead>
                    <TableHead className="text-right">
                      {t('runtime.appsTable.columns.users')}
                    </TableHead>
                    <TableHead>{t('runtime.appsTable.columns.cap')}</TableHead>
                    <TableHead className="w-12" />
                  </TableRow>
                </TableHeader>
                <TableBody>
                  {items.map(app => {
                    const crossWorkspace =
                      Boolean(app.workspace_name) && app.workspace_id !== runtimeWorkspaceId;
                    return (
                      <TableRow
                        key={app.agent_id}
                        data-state={selected.has(app.agent_id) ? 'selected' : undefined}
                        className="cursor-pointer"
                        onClick={() => setDetailId(app.agent_id)}
                      >
                        <TableCell onClick={event => event.stopPropagation()}>
                          <Checkbox
                            aria-label={t('runtime.appsTable.selectRow')}
                            checked={selected.has(app.agent_id)}
                            onCheckedChange={value => toggle(app.agent_id, value === true)}
                          />
                        </TableCell>
                        <TableCell className="max-w-72">
                          <div className="flex items-center gap-3">
                            <RuntimeAppIcon app={app} />
                            <div className="min-w-0">
                              <div className="truncate font-medium">{app.name || app.app_key}</div>
                              <div className="truncate text-xs text-muted-foreground">
                                {crossWorkspace
                                  ? t('runtime.apps.sourceWorkspaceLabel', {
                                      name: app.workspace_name,
                                    })
                                  : app.description || t('runtime.apps.noDescription')}
                              </div>
                            </div>
                          </div>
                        </TableCell>
                        <TableCell className="whitespace-nowrap text-sm">
                          {app.app_type === 'digital_human'
                            ? t('runtime.apps.digitalHuman')
                            : t('runtime.appsTable.agent')}
                        </TableCell>
                        <TableCell>
                          <AppAvailability app={app} />
                        </TableCell>
                        <TableCell className="text-right tabular-nums">
                          {pointsFormat.format(app.usage_30d.points)}
                        </TableCell>
                        <TableCell className="text-right tabular-nums">
                          {app.usage_30d.calls}
                        </TableCell>
                        <TableCell className="text-right tabular-nums">
                          {app.usage_30d.end_users}
                        </TableCell>
                        <TableCell className="whitespace-nowrap text-xs text-muted-foreground">
                          {app.quota_caps.length > 0 ? capSummary(app.quota_caps, windowUnit) : '—'}
                        </TableCell>
                        <TableCell className="text-right">
                          <Button
                            type="button"
                            size="sm"
                            variant="ghost"
                            className="size-8 p-0 text-muted-foreground hover:text-destructive"
                            title={t('runtime.apps.unbind')}
                            aria-label={t('runtime.apps.unbind')}
                            onClick={event => {
                              event.stopPropagation();
                              setRemoving([app.agent_id]);
                            }}
                          >
                            <Trash2 className="size-4" />
                          </Button>
                        </TableCell>
                      </TableRow>
                    );
                  })}
                </TableBody>
              </Table>
            </div>
          )}
        </CardContent>
      </Card>

      {(data?.total ?? 0) > pageSize ? (
        <Pagination
          currentPage={page}
          totalPages={Math.ceil((data?.total ?? 0) / pageSize)}
          total={data?.total ?? 0}
          pageSize={pageSize}
          onPageChange={setPage}
        />
      ) : null}

      <AddAppsDialog
        runtimeWorkspaceId={runtimeWorkspaceId}
        open={adding}
        onOpenChange={setAdding}
        remaining={Math.max(0, limit - bound)}
        limit={limit}
        saving={change.isPending}
        onAdd={async picked => {
          if (!data) return;
          await change.mutateAsync({ expected_revision: data.revision, apps: picked });
          toast.success(t('runtime.messages.agentsAdded', { count: picked.length }));
          setAdding(false);
        }}
      />

      <AppDetailSheet
        runtimeWorkspaceId={runtimeWorkspaceId}
        app={detail}
        policyJson={policyJson}
        saving={savingPolicy}
        onSavePolicy={async policy => {
          await onSavePolicy(policy);
          // The list shows each app's caps from the policy.
          await apps.refetch();
        }}
        onRemove={() => detail && setRemoving([detail.agent_id])}
        onClose={() => setDetailId(null)}
      />

      <ConfirmDialog
        open={removing !== null}
        onOpenChange={open => {
          if (!open) setRemoving(null);
        }}
        title={t('runtime.apps.batchRemoveTitle', { count: removing?.length ?? 0 })}
        description={t('runtime.apps.batchRemoveDescription', { count: removing?.length ?? 0 })}
        confirmText={t('runtime.apps.confirmUnbind')}
        cancelText={tCommon('cancel')}
        variant="danger"
        loading={change.isPending}
        closeOnConfirm={false}
        onConfirm={() => {
          if (removing) void remove(removing);
        }}
      />
    </section>
  );
}

function AddAppsDialog({
  runtimeWorkspaceId,
  open,
  onOpenChange,
  remaining,
  limit,
  saving,
  onAdd,
}: {
  runtimeWorkspaceId: string;
  open: boolean;
  onOpenChange: (open: boolean) => void;
  remaining: number;
  limit: number;
  saving: boolean;
  onAdd: (apps: Array<{ type: string; id: string }>) => Promise<void>;
}) {
  const t = useT('workspace');
  const tCommon = useT('common');
  const [scope, setScope] = React.useState<'current' | 'authorized'>('current');
  const [sourceWorkspaceId, setSourceWorkspaceId] = React.useState('');
  const [searchInput, setSearchInput] = React.useState('');
  const search = useDebounced(searchInput);
  const [type, setType] = React.useState<RuntimeAppType | ''>('');
  const [picked, setPicked] = React.useState<Map<string, string>>(new Map());
  const candidates = useRuntimeAppCandidates(
    runtimeWorkspaceId,
    { scope, sourceWorkspaceId: sourceWorkspaceId || undefined, search, type },
    open
  );
  React.useEffect(() => {
    if (!open) {
      setPicked(new Map());
      setSearchInput('');
      setScope('current');
      setSourceWorkspaceId('');
      setType('');
    }
  }, [open]);
  const overLimit = picked.size > remaining;
  const toggle = (id: string, appType: string) =>
    setPicked(current => {
      const next = new Map(current);
      if (next.has(id)) next.delete(id);
      else next.set(id, appType);
      return next;
    });

  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent className="max-w-3xl">
        <DialogHeader>
          <DialogTitle>{t('runtime.appsTable.addTitle')}</DialogTitle>
          <DialogDescription>{t('runtime.appsTable.addDescription')}</DialogDescription>
        </DialogHeader>
        <DialogBody className="space-y-3">
          {candidates.meta?.cross_workspace_enabled ? (
            <Tabs
              value={scope}
              onValueChange={value => setScope(value as 'current' | 'authorized')}
            >
              <TabsList>
                <TabsTrigger value="current">{t('runtime.apps.currentWorkspace')}</TabsTrigger>
                <TabsTrigger value="authorized">{t('runtime.apps.otherWorkspaces')}</TabsTrigger>
              </TabsList>
            </Tabs>
          ) : null}
          <div className="flex flex-col gap-2 sm:flex-row">
            <div className="relative min-w-0 flex-1">
              <Search className="pointer-events-none absolute left-3 top-1/2 size-4 -translate-y-1/2 text-muted-foreground" />
              <Input
                value={searchInput}
                onChange={event => setSearchInput(event.target.value)}
                placeholder={t('runtime.appsTable.candidateSearch')}
                className="h-9 pl-9"
              />
            </div>
            {scope === 'authorized' ? (
              <Select
                value={sourceWorkspaceId || 'all'}
                onValueChange={value => setSourceWorkspaceId(value === 'all' ? '' : value)}
              >
                <SelectTrigger className="h-9 w-full sm:w-48">
                  <SelectValue />
                </SelectTrigger>
                <SelectContent>
                  <SelectItem value="all">{t('runtime.apps.allSourceWorkspaces')}</SelectItem>
                  {(candidates.meta?.source_workspaces ?? []).map(workspace => (
                    <SelectItem key={workspace.id} value={workspace.id}>
                      {workspace.name}
                    </SelectItem>
                  ))}
                </SelectContent>
              </Select>
            ) : null}
            <Select
              value={type || 'all'}
              onValueChange={value => setType(value === 'all' ? '' : (value as RuntimeAppType))}
            >
              <SelectTrigger className="h-9 w-full sm:w-36">
                <SelectValue />
              </SelectTrigger>
              <SelectContent>
                <SelectItem value="all">{t('runtime.appsTable.anyType')}</SelectItem>
                <SelectItem value="agent">{t('runtime.appsTable.agent')}</SelectItem>
                <SelectItem value="digital_human">{t('runtime.apps.digitalHuman')}</SelectItem>
              </SelectContent>
            </Select>
          </div>
          {scope === 'authorized' ? (
            <p className="text-xs leading-5 text-muted-foreground">
              {t('runtime.apps.crossWorkspaceNote')}
            </p>
          ) : null}
          <div className="max-h-[50vh] divide-y overflow-y-auto rounded-lg border">
            {candidates.isLoading ? (
              <div className="space-y-2 p-3">
                {Array.from({ length: 4 }).map((_, index) => (
                  <Skeleton key={index} className="h-10 w-full" />
                ))}
              </div>
            ) : candidates.items.length === 0 ? (
              <p className="px-4 py-10 text-center text-sm text-muted-foreground">
                {search ? t('runtime.apps.noSearchResults') : t('runtime.apps.noCandidates')}
              </p>
            ) : (
              candidates.items.map(candidate => (
                <label
                  key={candidate.agent_id}
                  className="flex cursor-pointer items-center gap-3 px-3 py-2.5 hover:bg-muted/30"
                >
                  <Checkbox
                    checked={picked.has(candidate.agent_id)}
                    onCheckedChange={() =>
                      toggle(candidate.agent_id, candidate.app_type ?? 'agent')
                    }
                  />
                  <RuntimeAppIcon app={candidate} />
                  <div className="min-w-0 flex-1">
                    <div className="flex items-center gap-2">
                      <span className="truncate text-sm font-medium">{candidate.name}</span>
                      {candidate.app_type === 'digital_human' ? (
                        <Badge variant="subtle" className="shrink-0 font-normal">
                          {t('runtime.apps.digitalHuman')}
                        </Badge>
                      ) : null}
                    </div>
                    <div className="truncate text-xs text-muted-foreground">
                      {scope === 'authorized'
                        ? t('runtime.apps.sourceWorkspaceLabel', { name: candidate.workspace_name })
                        : candidate.description || t('runtime.apps.noDescription')}
                    </div>
                  </div>
                  <span className="shrink-0 text-xs text-muted-foreground">
                    {candidate.version} · {formatShortDate(candidate.published_at)}
                  </span>
                </label>
              ))
            )}
            {candidates.hasMore ? (
              <Button
                type="button"
                variant="ghost"
                size="sm"
                className="w-full rounded-none text-muted-foreground"
                disabled={candidates.loadingMore}
                onClick={candidates.loadMore}
              >
                {candidates.loadingMore ? <Loader2 className="size-4 animate-spin" /> : null}
                {t('runtime.common.loadMore')}
              </Button>
            ) : null}
          </div>
          <div className="flex items-center justify-between gap-2 text-xs">
            <span className={cn(overLimit ? 'text-warning' : 'text-muted-foreground')}>
              {overLimit
                ? t('runtime.apps.overLimit', { limit, remaining })
                : t('runtime.apps.selectedCount', { count: picked.size })}
            </span>
            {picked.size > 0 ? (
              <Button type="button" variant="ghost" size="sm" onClick={() => setPicked(new Map())}>
                {t('runtime.apps.clearSelection')}
              </Button>
            ) : null}
          </div>
        </DialogBody>
        <DialogFooter>
          <Button
            type="button"
            variant="ghost"
            onClick={() => onOpenChange(false)}
            disabled={saving}
          >
            {tCommon('cancel')}
          </Button>
          <Button
            type="button"
            disabled={picked.size === 0 || overLimit || saving}
            onClick={() => void onAdd([...picked].map(([id, appType]) => ({ id, type: appType })))}
          >
            {saving ? <Loader2 className="size-4 animate-spin" /> : <Link2 className="size-4" />}
            {t('runtime.appsTable.addSelected', { count: picked.size })}
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  );
}

function AppDetailSheet({
  runtimeWorkspaceId,
  app,
  policyJson,
  saving,
  onSavePolicy,
  onRemove,
  onClose,
}: {
  runtimeWorkspaceId: string;
  app: RuntimeManagedApp | null;
  policyJson?: string;
  saving: boolean;
  onSavePolicy: (policy: Record<string, unknown>) => Promise<void>;
  onRemove: () => void;
  onClose: () => void;
}) {
  const t = useT('workspace');
  const [tab, setTab] = React.useState('overview');
  const [caps, setCaps] = React.useState<Partial<Record<RuntimeQuotaWindow, string>>>({});
  const [copied, setCopied] = React.useState(false);
  const appId = app?.agent_id ?? null;
  const series = useRuntimeEndUserUsageSeries(runtimeWorkspaceId, null, 30, Boolean(app));
  React.useEffect(() => {
    setTab('overview');
  }, [appId]);
  React.useEffect(() => {
    const next: Partial<Record<RuntimeQuotaWindow, string>> = {};
    for (const cap of app?.quota_caps ?? []) next[cap.window] = String(cap.points);
    setCaps(next);
  }, [app]);
  const appSeries = React.useMemo<RuntimeUsageSeries | undefined>(() => {
    if (!series.data || !app) return undefined;
    return {
      apps: series.data.apps.filter(item => item.app_id === app.agent_id),
      days: series.data.days.map(day => {
        const points = day.apps[app.agent_id] ?? 0;
        return { ...day, points, apps: points > 0 ? { [app.agent_id]: points } : {} };
      }),
    };
  }, [series.data, app]);
  const capLimits = quotaWindows.flatMap(window => {
    const value = Math.round(Number.parseFloat(caps[window] ?? '') * 1000) / 1000;
    return Number.isFinite(value) && value >= 0 ? [{ window, points: value }] : [];
  });
  const capsDirty =
    JSON.stringify(capLimits) !==
    JSON.stringify(
      (app?.quota_caps ?? []).map(cap => ({ window: cap.window, points: cap.points }))
    );

  return (
    <Sheet open={Boolean(app)} onOpenChange={open => (!open ? onClose() : undefined)}>
      <SheetContent className="w-full overflow-y-auto sm:max-w-2xl">
        {app ? (
          <>
            <SheetHeader>
              <div className="flex items-center gap-3 pr-6">
                <RuntimeAppIcon app={app} />
                <div className="min-w-0">
                  <SheetTitle className="truncate">{app.name || app.app_key}</SheetTitle>
                  <SheetDescription className="flex flex-wrap items-center gap-2">
                    <span>
                      {app.app_type === 'digital_human'
                        ? t('runtime.apps.digitalHuman')
                        : t('runtime.appsTable.agent')}
                    </span>
                    <AppAvailability app={app} />
                  </SheetDescription>
                </div>
              </div>
            </SheetHeader>
            <Tabs value={tab} onValueChange={setTab} className="mt-5">
              <TabsList className="w-full justify-start">
                <TabsTrigger value="overview">{t('runtime.appsTable.tabs.overview')}</TabsTrigger>
                <TabsTrigger value="quota">{t('runtime.appsTable.tabs.quota')}</TabsTrigger>
                <TabsTrigger value="info">{t('runtime.appsTable.tabs.info')}</TabsTrigger>
              </TabsList>
              <TabsContent value="overview" className="mt-5 space-y-5">
                <div className="grid grid-cols-3 gap-2">
                  {(
                    [
                      ['points', pointsFormat.format(app.usage_30d.points)],
                      ['calls', String(app.usage_30d.calls)],
                      ['users', String(app.usage_30d.end_users)],
                    ] as const
                  ).map(([key, value]) => (
                    <div key={key} className="rounded-lg border p-3">
                      <div className="text-xs text-muted-foreground">
                        {t(`runtime.appsTable.usage.${key}`)}
                      </div>
                      <div className="mt-1 text-base font-semibold tabular-nums">{value}</div>
                    </div>
                  ))}
                </div>
                <section className="space-y-2">
                  <h3 className="text-sm font-medium">{t('runtime.endUsers.trend')}</h3>
                  {series.isLoading ? (
                    <Skeleton className="h-44 w-full" />
                  ) : (
                    <UsageTrendChart
                      series={appSeries}
                      appNames={new Map([[app.agent_id, app.name]])}
                      height={176}
                    />
                  )}
                </section>
              </TabsContent>
              <TabsContent value="quota" className="mt-5 space-y-4">
                <div>
                  <h3 className="text-sm font-medium">{t('runtime.appsTable.capTitle')}</h3>
                  <p className="mt-1 text-xs leading-5 text-muted-foreground">
                    {t('runtime.appsTable.capDescription')}
                  </p>
                </div>
                <div className="grid grid-cols-3 gap-2">
                  {quotaWindows.map(window => (
                    <label key={window} className="space-y-1">
                      <span className="text-xs text-muted-foreground">
                        {t(`runtime.quota.windows.${window}`)}
                      </span>
                      <Input
                        type="number"
                        min={0}
                        step="any"
                        inputMode="decimal"
                        placeholder={t('runtime.quota.unlimited')}
                        value={caps[window] ?? ''}
                        onChange={event => setCaps({ ...caps, [window]: event.target.value })}
                        className="h-9"
                      />
                    </label>
                  ))}
                </div>
                <p className="text-xs text-muted-foreground">
                  {t('runtime.appsTable.capPlansNote')}
                </p>
                <div className="flex justify-end">
                  <Button
                    type="button"
                    size="sm"
                    disabled={!capsDirty || saving}
                    onClick={() =>
                      void onSavePolicy(withAppCaps(policyJson, app.app_key, capLimits)).then(() =>
                        toast.success(t('runtime.appsTable.capSaved'))
                      )
                    }
                  >
                    {saving ? <Loader2 className="size-4 animate-spin" /> : null}
                    {t('runtime.quota.save')}
                  </Button>
                </div>
              </TabsContent>
              <TabsContent value="info" className="mt-5 space-y-4 text-sm">
                <dl className="grid grid-cols-[8rem_minmax(0,1fr)] gap-x-3 gap-y-2.5">
                  <dt className="text-muted-foreground">{t('runtime.apps.appId')}</dt>
                  <dd className="flex items-center gap-1.5">
                    <code className="truncate text-xs">{app.app_key}</code>
                    <Button
                      type="button"
                      size="sm"
                      variant="ghost"
                      className="size-7 p-0"
                      aria-label={t('runtime.apps.copyAppId')}
                      onClick={() =>
                        void navigator.clipboard.writeText(app.app_key).then(() => {
                          setCopied(true);
                          window.setTimeout(() => setCopied(false), 1500);
                        })
                      }
                    >
                      {copied ? <Check className="size-3.5" /> : <Copy className="size-3.5" />}
                    </Button>
                  </dd>
                  <dt className="text-muted-foreground">{t('runtime.apps.sourceWorkspace')}</dt>
                  <dd className="truncate">{app.workspace_name || '—'}</dd>
                  <dt className="text-muted-foreground">{t('runtime.apps.published')}</dt>
                  <dd>
                    {app.version || '—'} · {formatShortDate(app.published_at)}
                  </dd>
                </dl>
                <CapabilityBadges capabilities={app.capabilities} />
                <CatalogTagBadges tags={app.tags} />
                <p className="text-xs text-muted-foreground">{t('runtime.apps.followPublished')}</p>
              </TabsContent>
            </Tabs>
            <SheetFooter className="mt-6">
              <Button
                type="button"
                variant="outline"
                className="text-destructive hover:text-destructive"
                onClick={onRemove}
              >
                <Trash2 className="size-4" />
                {t('runtime.apps.unbind')}
              </Button>
            </SheetFooter>
          </>
        ) : null}
      </SheetContent>
    </Sheet>
  );
}
