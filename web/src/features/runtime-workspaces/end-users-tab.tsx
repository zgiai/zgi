'use client';

import * as React from 'react';
import {
  ArrowDown,
  ArrowUp,
  ArrowUpDown,
  Ban,
  ChevronDown,
  Filter,
  Crown,
  Download,
  Loader2,
  PlayCircle,
  RotateCcw,
  Search,
  SlidersHorizontal,
  Trash2,
  UserRound,
  X,
} from 'lucide-react';
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
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuSeparator,
  DropdownMenuTrigger,
} from '@/components/ui/dropdown-menu';
import { Input } from '@/components/ui/input';
import { Pagination } from '@/components/ui/pagination';
import { Popover, PopoverContent, PopoverTrigger } from '@/components/ui/popover';
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
import { Skeleton } from '@/components/ui/skeleton';
import { Tabs, TabsContent, TabsList, TabsTrigger } from '@/components/ui/tabs';
import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from '@/components/ui/table';
import { EndUserCreditsSection } from '@/features/runtime-workspaces/end-user-credits';
import { RuntimePageHeader } from '@/features/runtime-workspaces/runtime-page-header';
import {
  endUserRatio,
  parsePlans,
  PlanAssignmentEditor,
  PlanBadge,
  planDraftOf,
  planLabel,
  planOfDraft,
  pointsFormat,
  QuotaOverrideEditor,
  usageRatio,
  quotaDraftOf,
  quotaOfDraft,
  quotaWindows,
  UsageMeter,
  UsageTrendChart,
  type PlanDraft,
  type QuotaOverrideDraft,
} from '@/features/runtime-workspaces/end-user-usage';
import {
  useRuntimeEndUserEvents,
  useRuntimeEndUsers,
  useRuntimeEndUserUsageSeries,
} from '@/hooks/runtime-workspace/use-runtime-end-users';
import { useT } from '@/i18n';
import { cn } from '@/lib/utils';
import type {
  RuntimeApp,
  RuntimeEndUser,
  RuntimeEndUserPatch,
  RuntimeEndUserPlan,
  RuntimeEndUserQuotaState,
  RuntimeEndUserSort,
  RuntimeEndUserStatus,
  RuntimeQuotaWindow,
} from '@/services/types/runtime-workspace';

const pageSize = 20;
const windowSorts: Record<RuntimeQuotaWindow, RuntimeEndUserSort> = {
  '24h': 'usage_24h',
  '7d': 'usage_7d',
  '30d': 'usage_30d',
};
const sorts: RuntimeEndUserSort[] = ['last_seen', 'usage_24h', 'usage_7d', 'usage_30d', 'quota'];
const quotaStates: RuntimeEndUserQuotaState[] = ['over', 'near', 'custom'];

type BulkAction = 'suspend' | 'resume' | 'reset';

function formatTime(value?: string): string {
  if (!value) return '—';
  const date = new Date(value);
  if (Number.isNaN(date.getTime())) return '—';
  return new Intl.DateTimeFormat(undefined, { dateStyle: 'medium', timeStyle: 'short' }).format(
    date
  );
}

function StatusBadge({ status }: { status: RuntimeEndUserStatus }) {
  const t = useT('workspace');
  return (
    <Badge
      variant={status === 'active' ? 'secondary' : 'outline'}
      className={cn(
        'whitespace-nowrap',
        status === 'suspended' && 'border-warning/40 text-warning'
      )}
    >
      {t(`runtime.endUsers.status.${status}`)}
    </Badge>
  );
}

function displayName(user: RuntimeEndUser, unnamed: string) {
  return user.external_user_id || `${unnamed} · ${user.subject_id.slice(0, 8)}`;
}

function QuotaCell({ user }: { user: RuntimeEndUser }) {
  const t = useT('workspace');
  const ratio = endUserRatio(user);
  if (ratio === null) {
    return (
      <span className="text-xs text-muted-foreground">
        {user.quota_source === 'unlimited'
          ? t('runtime.quota.unlimited')
          : t('runtime.endUsers.noLimit')}
      </span>
    );
  }
  const usage = user.usage.find(item => usageRatio(item) === ratio);
  return (
    <div className="min-w-36 space-y-1">
      <div className="flex items-center justify-between gap-2 text-xs">
        <span
          className={cn(
            'font-medium tabular-nums',
            ratio >= 1 ? 'text-destructive' : ratio >= 0.8 ? 'text-warning' : undefined
          )}
        >
          {Math.round(ratio * 100)}%
        </span>
        {usage ? (
          <span className="text-muted-foreground">
            {t(`runtime.usage.windowShort.${usage.window}`)}
          </span>
        ) : null}
      </div>
      <div
        className="h-1.5 w-full overflow-hidden rounded-full bg-muted"
        title={
          usage?.limit !== undefined
            ? `${pointsFormat.format(usage.points)} / ${pointsFormat.format(usage.limit)}`
            : undefined
        }
      >
        <div
          className={cn(
            'h-full rounded-full',
            ratio >= 1 ? 'bg-destructive' : ratio >= 0.8 ? 'bg-warning' : 'bg-primary'
          )}
          style={{ width: `${Math.min(100, Math.max(ratio * 100, ratio > 0 ? 2 : 0))}%` }}
        />
      </div>
    </div>
  );
}

function SortHeader({
  label,
  sort,
  current,
  ascending,
  onSort,
  className,
}: {
  label: string;
  sort: RuntimeEndUserSort;
  current: RuntimeEndUserSort;
  ascending: boolean;
  onSort: (sort: RuntimeEndUserSort) => void;
  className?: string;
}) {
  const active = current === sort;
  const Icon = !active ? ArrowUpDown : ascending ? ArrowUp : ArrowDown;
  return (
    <TableHead className={cn('whitespace-nowrap', className)}>
      <button
        type="button"
        onClick={() => onSort(sort)}
        className={cn(
          'inline-flex items-center gap-1 hover:text-foreground',
          active && 'text-foreground'
        )}
      >
        {label}
        <Icon className={cn('size-3.5', !active && 'opacity-40')} />
      </button>
    </TableHead>
  );
}

/**
 * @component RuntimeEndUsersTab
 * @category Feature
 * @status Beta
 * @description Manages the end users of a Runtime Workspace (ADR 0010): lists
 * them with their points usage against their limits, filters them by status
 * and quota state, orders them by activity or usage, changes several at once,
 * and opens one to see its daily usage, usage per app, and change history, to
 * set its own quota, and to suspend, export, or erase it.
 * @usage Rendered as the End users section of the Runtime Workspace console.
 * @example
 * <RuntimeEndUsersTab runtimeWorkspaceId={id} apps={apps} policyJson={policy} />
 */
export function RuntimeEndUsersTab({
  runtimeWorkspaceId,
  apps,
  policyJson,
}: {
  runtimeWorkspaceId: string;
  apps: RuntimeApp[];
  policyJson?: string;
}) {
  const t = useT('workspace');
  const tCommon = useT('common');
  const [searchInput, setSearchInput] = React.useState('');
  const [search, setSearch] = React.useState('');
  const [status, setStatus] = React.useState<RuntimeEndUserStatus | ''>('');
  const [quotaState, setQuotaState] = React.useState<RuntimeEndUserQuotaState | ''>('');
  const [planFilter, setPlanFilter] = React.useState('');
  const [usageWindow, setUsageWindow] = React.useState<RuntimeQuotaWindow>('24h');
  const [bulkPlanOpen, setBulkPlanOpen] = React.useState(false);
  const [bulkPlan, setBulkPlan] = React.useState<PlanDraft>({ key: '', until: '' });
  const plans = React.useMemo(() => parsePlans(policyJson), [policyJson]);
  const [sort, setSort] = React.useState<RuntimeEndUserSort>('last_seen');
  const [ascending, setAscending] = React.useState(false);
  const [page, setPage] = React.useState(1);
  const [selectedId, setSelectedId] = React.useState<string | null>(null);
  const [checked, setChecked] = React.useState<Set<string>>(new Set());
  const [statusTarget, setStatusTarget] = React.useState<RuntimeEndUser | null>(null);
  const [eraseTarget, setEraseTarget] = React.useState<RuntimeEndUser | null>(null);
  const [bulkAction, setBulkAction] = React.useState<BulkAction | null>(null);
  const [bulkQuotaOpen, setBulkQuotaOpen] = React.useState(false);
  const [bulkQuota, setBulkQuota] = React.useState<QuotaOverrideDraft>({
    mode: 'custom',
    limits: {},
  });

  // Links from the quota page open the list filtered, ordered, or on one user.
  React.useEffect(() => {
    const params = new URLSearchParams(window.location.search);
    const linkedState = params.get('quota_state') as RuntimeEndUserQuotaState | null;
    if (linkedState && quotaStates.includes(linkedState)) setQuotaState(linkedState);
    const linkedStatus = params.get('status');
    if (linkedStatus === 'active' || linkedStatus === 'suspended' || linkedStatus === 'erased') {
      setStatus(linkedStatus);
    }
    const linkedSort = params.get('sort') as RuntimeEndUserSort | null;
    if (linkedSort && sorts.includes(linkedSort)) {
      setSort(linkedSort);
      const linkedWindow = quotaWindows.find(window => windowSorts[window] === linkedSort);
      if (linkedWindow) setUsageWindow(linkedWindow);
    }
    const linkedPlan = params.get('plan');
    if (linkedPlan) setPlanFilter(linkedPlan);
    const linkedUser = params.get('user');
    if (linkedUser) {
      setSearchInput(linkedUser);
      setSearch(linkedUser);
      setSelectedId(linkedUser);
    }
  }, []);

  React.useEffect(() => {
    const timer = window.setTimeout(() => {
      setSearch(searchInput.trim());
    }, 300);
    return () => window.clearTimeout(timer);
  }, [searchInput]);

  React.useEffect(() => {
    setChecked(new Set());
    setPage(1);
  }, [search, status, quotaState, planFilter]);

  const { list, update, batchUpdate, exportData, erase } = useRuntimeEndUsers(runtimeWorkspaceId, {
    search,
    status,
    quota_state: quotaState,
    plan: planFilter,
    sort,
    order: ascending ? 'asc' : 'desc',
    page,
    limit: pageSize,
  });
  const users = list.data?.items ?? [];
  const total = list.data?.total ?? 0;
  const selected = users.find(user => user.subject_id === selectedId) ?? null;
  const appNames = React.useMemo(() => new Map(apps.map(app => [app.agent_id, app.name])), [apps]);
  const selectable = users.filter(user => user.status !== 'erased');
  const pageChecked = selectable.filter(user => checked.has(user.subject_id)).length;
  const filtered = Boolean(search || status || quotaState || planFilter);
  const filterCount = [status, quotaState, planFilter].filter(Boolean).length;
  const clearFilters = () => {
    setSearchInput('');
    setSearch('');
    setStatus('');
    setQuotaState('');
    setPlanFilter('');
    setPage(1);
  };
  const changeUsageWindow = (window: RuntimeQuotaWindow) => {
    setUsageWindow(window);
    if (Object.values(windowSorts).includes(sort)) setSort(windowSorts[window]);
  };
  const filterChips = [
    status
      ? { key: 'status', label: t(`runtime.endUsers.status.${status}`), clear: () => setStatus('') }
      : null,
    quotaState
      ? {
          key: 'quota',
          label: t(`runtime.endUsers.quotaState.${quotaState}`),
          clear: () => setQuotaState(''),
        }
      : null,
    planFilter
      ? {
          key: 'plan',
          label:
            planFilter === 'none'
              ? t('runtime.plans.none')
              : planLabel(plans.find(plan => plan.key === planFilter) ?? { key: planFilter }),
          clear: () => setPlanFilter(''),
        }
      : null,
  ].filter((chip): chip is { key: string; label: string; clear: () => void } => chip !== null);

  const changeSort = (next: RuntimeEndUserSort) => {
    if (next === sort) setAscending(!ascending);
    else {
      setSort(next);
      setAscending(false);
    }
    setPage(1);
  };

  const toggle = (subjectId: string, on: boolean) => {
    setChecked(current => {
      const next = new Set(current);
      if (on) next.add(subjectId);
      else next.delete(subjectId);
      return next;
    });
  };

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

  const applyBulk = async (patch: RuntimeEndUserPatch) => {
    const result = await batchUpdate.mutateAsync({ subjectIds: [...checked], patch });
    if (result.failed.length > 0) {
      toast.warning(
        t('runtime.endUsers.bulk.partial', {
          updated: result.updated,
          failed: result.failed.length,
        })
      );
      setChecked(new Set(result.failed));
    } else {
      toast.success(t('runtime.endUsers.bulk.done', { count: result.updated }));
      setChecked(new Set());
    }
    setBulkAction(null);
    setBulkQuotaOpen(false);
    setBulkPlanOpen(false);
  };

  const bulkQuotaValue = quotaOfDraft(bulkQuota);

  return (
    <section className="space-y-4">
      <RuntimePageHeader
        title={t('runtime.endUsers.title')}
        description={t('runtime.endUsers.description')}
      />

      <div className="space-y-2">
        <div className="flex items-center gap-2">
          <div className="relative min-w-0 flex-1">
            <Search className="pointer-events-none absolute left-3 top-1/2 size-4 -translate-y-1/2 text-muted-foreground" />
            <Input
              value={searchInput}
              onChange={event => setSearchInput(event.target.value)}
              placeholder={t('runtime.endUsers.searchPlaceholder')}
              className="h-9 pl-9"
            />
          </div>
          <Popover>
            <PopoverTrigger asChild>
              <Button type="button" variant="outline" className="h-9 shrink-0">
                <Filter className="size-4" />
                {t('runtime.endUsers.filters')}
                {filterCount > 0 ? (
                  <Badge className="ml-0.5 h-5 min-w-5 justify-center rounded-full px-1.5 text-[11px]">
                    {filterCount}
                  </Badge>
                ) : null}
              </Button>
            </PopoverTrigger>
            <PopoverContent align="end" className="w-72 space-y-3">
              <label className="block space-y-1">
                <span className="text-xs text-muted-foreground">
                  {t('runtime.endUsers.columns.status')}
                </span>
                <Select
                  value={status || 'all'}
                  onValueChange={value => {
                    setStatus(value === 'all' ? '' : (value as RuntimeEndUserStatus));
                    setPage(1);
                  }}
                >
                  <SelectTrigger className="h-9">
                    <SelectValue />
                  </SelectTrigger>
                  <SelectContent>
                    <SelectItem value="all">{t('runtime.endUsers.allStatuses')}</SelectItem>
                    <SelectItem value="active">{t('runtime.endUsers.status.active')}</SelectItem>
                    <SelectItem value="suspended">
                      {t('runtime.endUsers.status.suspended')}
                    </SelectItem>
                    <SelectItem value="erased">{t('runtime.endUsers.status.erased')}</SelectItem>
                  </SelectContent>
                </Select>
              </label>
              <label className="block space-y-1">
                <span className="text-xs text-muted-foreground">
                  {t('runtime.endUsers.columns.quota')}
                </span>
                <Select
                  value={quotaState || 'all'}
                  onValueChange={value => {
                    setQuotaState(value === 'all' ? '' : (value as RuntimeEndUserQuotaState));
                    setPage(1);
                  }}
                >
                  <SelectTrigger className="h-9">
                    <SelectValue />
                  </SelectTrigger>
                  <SelectContent>
                    <SelectItem value="all">{t('runtime.endUsers.allQuotaStates')}</SelectItem>
                    {quotaStates.map(item => (
                      <SelectItem key={item} value={item}>
                        {t(`runtime.endUsers.quotaState.${item}`)}
                      </SelectItem>
                    ))}
                  </SelectContent>
                </Select>
              </label>
              {plans.length > 0 || planFilter ? (
                <label className="block space-y-1">
                  <span className="text-xs text-muted-foreground">{t('runtime.plans.plan')}</span>
                  <Select
                    value={planFilter || 'all'}
                    onValueChange={value => {
                      setPlanFilter(value === 'all' ? '' : value);
                      setPage(1);
                    }}
                  >
                    <SelectTrigger className="h-9">
                      <SelectValue />
                    </SelectTrigger>
                    <SelectContent>
                      <SelectItem value="all">{t('runtime.plans.allPlans')}</SelectItem>
                      <SelectItem value="none">{t('runtime.plans.none')}</SelectItem>
                      {plans.map(plan => (
                        <SelectItem key={plan.key} value={plan.key}>
                          {planLabel(plan)}
                        </SelectItem>
                      ))}
                    </SelectContent>
                  </Select>
                </label>
              ) : null}
            </PopoverContent>
          </Popover>
        </div>
        {filterChips.length > 0 || search ? (
          <div className="flex flex-wrap items-center gap-1.5">
            {filterChips.map(chip => (
              <Badge key={chip.key} variant="secondary" className="h-6 gap-1 pr-1 font-normal">
                {chip.label}
                <button
                  type="button"
                  className="rounded-sm p-0.5 hover:bg-muted"
                  aria-label={t('runtime.endUsers.removeFilter')}
                  onClick={() => {
                    chip.clear();
                    setPage(1);
                  }}
                >
                  <X className="size-3" />
                </button>
              </Badge>
            ))}
            <Button
              type="button"
              variant="ghost"
              size="sm"
              className="h-6 px-2 text-xs"
              onClick={clearFilters}
            >
              {t('runtime.endUsers.clearFilters')}
            </Button>
          </div>
        ) : null}
      </div>

      {checked.size > 0 ? (
        <div className="flex flex-wrap items-center gap-2 rounded-lg border border-primary/30 bg-primary/5 px-3 py-2">
          <span className="text-sm font-medium">
            {t('runtime.endUsers.bulk.selected', { count: checked.size })}
          </span>
          <Button type="button" size="sm" variant="ghost" onClick={() => setChecked(new Set())}>
            {t('runtime.endUsers.bulk.clear')}
          </Button>
          <div className="ml-auto flex flex-wrap gap-2">
            {plans.length > 0 ? (
              <Button
                type="button"
                size="sm"
                variant="outline"
                onClick={() => {
                  setBulkPlan({ key: plans[0]?.key ?? '', until: '' });
                  setBulkPlanOpen(true);
                }}
              >
                <Crown className="size-4" />
                {t('runtime.plans.setPlan')}
              </Button>
            ) : null}
            <Button
              type="button"
              size="sm"
              variant="outline"
              onClick={() => {
                setBulkQuota({ mode: 'custom', limits: {} });
                setBulkQuotaOpen(true);
              }}
            >
              <SlidersHorizontal className="size-4" />
              {t('runtime.endUsers.bulk.setQuota')}
            </Button>
            <DropdownMenu>
              <DropdownMenuTrigger asChild>
                <Button type="button" size="sm" variant="outline">
                  {t('runtime.endUsers.bulk.more')}
                  <ChevronDown className="size-4" />
                </Button>
              </DropdownMenuTrigger>
              <DropdownMenuContent align="end">
                <DropdownMenuItem onSelect={() => setBulkAction('resume')}>
                  <PlayCircle className="size-4" />
                  {t('runtime.endUsers.bulk.resume')}
                </DropdownMenuItem>
                <DropdownMenuItem onSelect={() => setBulkAction('reset')}>
                  <RotateCcw className="size-4" />
                  {t('runtime.endUsers.bulk.reset')}
                </DropdownMenuItem>
                <DropdownMenuSeparator />
                <DropdownMenuItem
                  className="text-destructive focus:text-destructive"
                  onSelect={() => setBulkAction('suspend')}
                >
                  <Ban className="size-4" />
                  {t('runtime.endUsers.bulk.suspend')}
                </DropdownMenuItem>
              </DropdownMenuContent>
            </DropdownMenu>
          </div>
        </div>
      ) : null}

      <Card className="border-border/80">
        <CardContent className="p-0">
          {list.isLoading ? (
            <div className="space-y-2 p-4">
              {Array.from({ length: 6 }).map((_, index) => (
                <Skeleton key={index} className="h-10 w-full" />
              ))}
            </div>
          ) : users.length === 0 ? (
            <div className="flex flex-col items-center gap-2 px-6 py-16 text-center">
              <UserRound className="size-8 text-muted-foreground" />
              <p className="text-sm font-medium">
                {filtered ? t('runtime.endUsers.noMatchTitle') : t('runtime.endUsers.emptyTitle')}
              </p>
              <p className="max-w-md text-xs leading-5 text-muted-foreground">
                {filtered
                  ? t('runtime.endUsers.noMatchDescription')
                  : t('runtime.endUsers.emptyDescription')}
              </p>
            </div>
          ) : (
            <div className={cn('overflow-x-auto', list.isFetching && 'opacity-70')}>
              <Table>
                <TableHeader>
                  <TableRow>
                    <TableHead className="w-10">
                      <Checkbox
                        aria-label={t('runtime.endUsers.bulk.selectPage')}
                        disabled={selectable.length === 0}
                        checked={pageChecked > 0 && pageChecked === selectable.length}
                        onCheckedChange={value => {
                          for (const user of selectable) toggle(user.subject_id, value === true);
                        }}
                      />
                    </TableHead>
                    <TableHead>{t('runtime.endUsers.columns.user')}</TableHead>
                    <TableHead>{t('runtime.endUsers.columns.status')}</TableHead>
                    <SortHeader
                      label={t('runtime.endUsers.columns.lastSeen')}
                      sort="last_seen"
                      current={sort}
                      ascending={ascending}
                      onSort={changeSort}
                    />
                    <TableHead className="whitespace-nowrap text-right">
                      <div className="flex items-center justify-end gap-2">
                        <button
                          type="button"
                          onClick={() => changeSort(windowSorts[usageWindow])}
                          className={cn(
                            'inline-flex items-center gap-1 hover:text-foreground',
                            sort === windowSorts[usageWindow] && 'text-foreground'
                          )}
                        >
                          {t('runtime.endUsers.columns.usage')}
                          {sort !== windowSorts[usageWindow] ? (
                            <ArrowUpDown className="size-3.5 opacity-40" />
                          ) : ascending ? (
                            <ArrowUp className="size-3.5" />
                          ) : (
                            <ArrowDown className="size-3.5" />
                          )}
                        </button>
                        <Select
                          value={usageWindow}
                          onValueChange={value => changeUsageWindow(value as RuntimeQuotaWindow)}
                        >
                          <SelectTrigger className="h-7 w-20 gap-1 px-2 text-xs">
                            <SelectValue />
                          </SelectTrigger>
                          <SelectContent align="end">
                            {quotaWindows.map(window => (
                              <SelectItem key={window} value={window}>
                                {t(`runtime.usage.windowShort.${window}`)}
                              </SelectItem>
                            ))}
                          </SelectContent>
                        </Select>
                      </div>
                    </TableHead>
                    <SortHeader
                      label={t('runtime.endUsers.columns.quota')}
                      sort="quota"
                      current={sort}
                      ascending={ascending}
                      onSort={changeSort}
                      className="min-w-40"
                    />
                    <TableHead className="text-right">{t('runtime.credits.title')}</TableHead>
                    <TableHead className="text-right">{tCommon('actions')}</TableHead>
                  </TableRow>
                </TableHeader>
                <TableBody>
                  {users.map(user => (
                    <TableRow
                      key={user.subject_id}
                      data-state={checked.has(user.subject_id) ? 'selected' : undefined}
                      className="cursor-pointer"
                      onClick={() => setSelectedId(user.subject_id)}
                    >
                      <TableCell onClick={event => event.stopPropagation()}>
                        <Checkbox
                          aria-label={t('runtime.endUsers.bulk.selectRow')}
                          disabled={user.status === 'erased'}
                          checked={checked.has(user.subject_id)}
                          onCheckedChange={value => toggle(user.subject_id, value === true)}
                        />
                      </TableCell>
                      <TableCell className="max-w-64">
                        <div className="truncate font-medium">
                          {displayName(user, t('runtime.endUsers.unnamed'))}
                        </div>
                        {user.plan ||
                        (user.quota_source !== 'default' && user.quota_source !== 'plan') ? (
                          <div className="mt-1 flex flex-wrap gap-1">
                            <PlanBadge plan={user.plan} />
                            {user.quota_source === 'custom' || user.quota_source === 'unlimited' ? (
                              <Badge
                                variant="outline"
                                className="h-5 px-1.5 text-[11px] font-normal"
                              >
                                {t(`runtime.endUsers.quotaSource.${user.quota_source}`)}
                              </Badge>
                            ) : null}
                          </div>
                        ) : null}
                      </TableCell>
                      <TableCell>
                        <StatusBadge status={user.status} />
                      </TableCell>
                      <TableCell className="whitespace-nowrap text-sm text-muted-foreground">
                        {formatTime(user.last_seen_at)}
                      </TableCell>
                      <TableCell className="whitespace-nowrap text-right tabular-nums">
                        {pointsFormat.format(
                          user.usage.find(item => item.window === usageWindow)?.points ?? 0
                        )}
                      </TableCell>
                      <TableCell>
                        <QuotaCell user={user} />
                      </TableCell>
                      <TableCell
                        className={cn(
                          'whitespace-nowrap text-right tabular-nums',
                          (user.credits?.balance ?? 0) < 0 && 'text-destructive'
                        )}
                      >
                        {user.credits ? (
                          pointsFormat.format(user.credits.balance)
                        ) : (
                          <span className="text-muted-foreground">—</span>
                        )}
                      </TableCell>
                      <TableCell className="text-right">
                        {user.status !== 'erased' ? (
                          <Button
                            type="button"
                            size="sm"
                            variant="ghost"
                            className="size-8 p-0"
                            title={
                              user.status === 'suspended'
                                ? t('runtime.endUsers.resume')
                                : t('runtime.endUsers.suspend')
                            }
                            aria-label={
                              user.status === 'suspended'
                                ? t('runtime.endUsers.resume')
                                : t('runtime.endUsers.suspend')
                            }
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
                          </Button>
                        ) : null}
                      </TableCell>
                    </TableRow>
                  ))}
                </TableBody>
              </Table>
            </div>
          )}
        </CardContent>
      </Card>

      {total > 0 ? (
        <div className="flex flex-col gap-2 @2xl/console:flex-row @2xl/console:items-center @2xl/console:justify-between">
          <span className="text-xs text-muted-foreground">
            {t('runtime.endUsers.total', { count: total })}
          </span>
          {total > pageSize ? (
            <Pagination
              currentPage={page}
              totalPages={Math.ceil(total / pageSize)}
              total={total}
              pageSize={pageSize}
              onPageChange={setPage}
            />
          ) : null}
        </div>
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

      <ConfirmDialog
        open={Boolean(bulkAction)}
        onOpenChange={open => {
          if (!open) setBulkAction(null);
        }}
        title={
          bulkAction ? t(`runtime.endUsers.bulk.${bulkAction}Title`, { count: checked.size }) : ''
        }
        description={
          bulkAction === 'suspend'
            ? t('runtime.endUsers.suspendDescription')
            : bulkAction === 'resume'
              ? t('runtime.endUsers.resumeDescription')
              : t('runtime.endUsers.bulk.resetDescription')
        }
        confirmText={bulkAction ? t(`runtime.endUsers.bulk.${bulkAction}`) : ''}
        cancelText={tCommon('cancel')}
        variant={bulkAction === 'suspend' ? 'warning' : 'default'}
        loading={batchUpdate.isPending}
        closeOnConfirm={false}
        onConfirm={() => {
          if (bulkAction === 'suspend') void applyBulk({ status: 'suspended' });
          if (bulkAction === 'resume') void applyBulk({ status: 'active' });
          if (bulkAction === 'reset') void applyBulk({ quota: null });
        }}
      />

      <Dialog open={bulkPlanOpen} onOpenChange={setBulkPlanOpen}>
        <DialogContent className="sm:max-w-lg">
          <DialogHeader>
            <DialogTitle>{t('runtime.plans.bulkTitle', { count: checked.size })}</DialogTitle>
            <DialogDescription>{t('runtime.plans.assignDescription')}</DialogDescription>
          </DialogHeader>
          <DialogBody>
            <PlanAssignmentEditor plans={plans} value={bulkPlan} onChange={setBulkPlan} />
          </DialogBody>
          <DialogFooter>
            <Button type="button" variant="outline" onClick={() => setBulkPlanOpen(false)}>
              {tCommon('cancel')}
            </Button>
            <Button
              type="button"
              disabled={batchUpdate.isPending}
              onClick={() => void applyBulk({ plan: planOfDraft(bulkPlan) })}
            >
              {batchUpdate.isPending ? <Loader2 className="size-4 animate-spin" /> : null}
              {t('runtime.plans.save')}
            </Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>

      <Dialog open={bulkQuotaOpen} onOpenChange={setBulkQuotaOpen}>
        <DialogContent className="sm:max-w-lg">
          <DialogHeader>
            <DialogTitle>
              {t('runtime.endUsers.bulk.quotaTitle', { count: checked.size })}
            </DialogTitle>
            <DialogDescription>{t('runtime.endUsers.ownQuotaDescription')}</DialogDescription>
          </DialogHeader>
          <DialogBody>
            <QuotaOverrideEditor value={bulkQuota} onChange={setBulkQuota} />
          </DialogBody>
          <DialogFooter>
            <Button type="button" variant="outline" onClick={() => setBulkQuotaOpen(false)}>
              {tCommon('cancel')}
            </Button>
            <Button
              type="button"
              disabled={batchUpdate.isPending || bulkQuotaValue === undefined}
              onClick={() => {
                if (bulkQuotaValue !== undefined) void applyBulk({ quota: bulkQuotaValue });
              }}
            >
              {batchUpdate.isPending ? <Loader2 className="size-4 animate-spin" /> : null}
              {t('runtime.endUsers.saveQuota')}
            </Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>

      <EndUserSheet
        runtimeWorkspaceId={runtimeWorkspaceId}
        user={selected}
        appNames={appNames}
        plans={plans}
        saving={update.isPending}
        onSavePlan={async plan => {
          if (!selected) return;
          await update.mutateAsync({ subjectId: selected.subject_id, patch: { plan } });
          toast.success(t('runtime.plans.saved'));
        }}
        onClose={() => setSelectedId(null)}
        onSaveQuota={async quota => {
          if (!selected) return;
          await update.mutateAsync({ subjectId: selected.subject_id, patch: { quota } });
          toast.success(t('runtime.endUsers.quotaSaved'));
        }}
        onToggleStatus={() => setStatusTarget(selected)}
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
  plans,
  saving,
  onClose,
  onSavePlan,
  onSaveQuota,
  onToggleStatus,
  exporting,
  onExport,
  onErase,
}: {
  runtimeWorkspaceId: string;
  user: RuntimeEndUser | null;
  appNames: Map<string, string>;
  plans: RuntimeEndUserPlan[];
  saving: boolean;
  onClose: () => void;
  onSavePlan: (plan: RuntimeEndUserPatch['plan']) => Promise<void>;
  onSaveQuota: (quota: RuntimeEndUser['quota'] | null) => Promise<void>;
  onToggleStatus: () => void;
  exporting: boolean;
  onExport: () => void;
  onErase: () => void;
}) {
  const t = useT('workspace');
  const subjectId = user?.subject_id ?? null;
  const events = useRuntimeEndUserEvents(runtimeWorkspaceId, subjectId);
  const series = useRuntimeEndUserUsageSeries(runtimeWorkspaceId, subjectId, 30, Boolean(user));
  const [draft, setDraft] = React.useState<QuotaOverrideDraft>(quotaDraftOf(null));
  const [planDraft, setPlanDraft] = React.useState<PlanDraft>(planDraftOf(null));
  const [tab, setTab] = React.useState('overview');
  React.useEffect(() => {
    setTab('overview');
  }, [subjectId]);

  React.useEffect(() => {
    if (user) {
      setDraft(quotaDraftOf(user));
      setPlanDraft(planDraftOf(user));
    }
  }, [user]);
  const planDirty = JSON.stringify(planDraft) !== JSON.stringify(planDraftOf(user));

  const quota = quotaOfDraft(draft);
  const dirty = JSON.stringify(quota) !== JSON.stringify(quotaOfDraft(quotaDraftOf(user)));
  const erased = user?.status === 'erased';

  return (
    <Sheet open={Boolean(user)} onOpenChange={open => (!open ? onClose() : undefined)}>
      <SheetContent className="w-full overflow-y-auto sm:max-w-2xl">
        {user ? (
          <>
            <SheetHeader>
              <SheetTitle className="break-all pr-6">
                {displayName(user, t('runtime.endUsers.unnamed'))}
              </SheetTitle>
              <SheetDescription className="flex flex-wrap items-center gap-x-3 gap-y-1">
                <StatusBadge status={user.status} />
                <span>
                  {t('runtime.endUsers.firstSeen')} {formatTime(user.first_seen_at)}
                </span>
                <span>
                  {t('runtime.endUsers.columns.lastSeen')} {formatTime(user.last_seen_at)}
                </span>
              </SheetDescription>
              {!erased ? (
                <div className="flex flex-wrap gap-2 pt-2">
                  <Button type="button" size="sm" variant="outline" onClick={onToggleStatus}>
                    {user.status === 'suspended' ? (
                      <PlayCircle className="size-4" />
                    ) : (
                      <Ban className="size-4" />
                    )}
                    {user.status === 'suspended'
                      ? t('runtime.endUsers.resume')
                      : t('runtime.endUsers.suspend')}
                  </Button>
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
              ) : null}
            </SheetHeader>

            <Tabs value={tab} onValueChange={setTab} className="mt-5">
              <TabsList className="w-full justify-start">
                {(erased
                  ? ['overview', 'history']
                  : ['overview', 'quota', 'credits', 'history']
                ).map(item => (
                  <TabsTrigger key={item} value={item}>
                    {t(
                      `runtime.endUsers.tabs.${item as 'overview' | 'quota' | 'credits' | 'history'}`
                    )}
                  </TabsTrigger>
                ))}
              </TabsList>
              <TabsContent value="overview" className="mt-5 space-y-6">
                <section className="space-y-2">
                  <h3 className="text-sm font-medium">{t('runtime.endUsers.usage')}</h3>
                  <div className="grid grid-cols-3 gap-2">
                    {quotaWindows.map(window => {
                      const usage = user.usage.find(item => item.window === window);
                      return (
                        <div key={window} className="space-y-1.5 rounded-lg border p-3">
                          <div className="text-xs text-muted-foreground">
                            {t(`runtime.quota.windows.${window}`)}
                          </div>
                          <UsageMeter
                            usage={usage}
                            align="left"
                            className="text-base font-semibold"
                          />
                          <div className="text-xs text-muted-foreground">
                            {usage?.remaining !== undefined
                              ? t('runtime.endUsers.remaining', {
                                  points: pointsFormat.format(usage.remaining),
                                })
                              : t('runtime.quota.unlimited')}
                          </div>
                        </div>
                      );
                    })}
                  </div>
                </section>

                <section className="space-y-2">
                  <h3 className="text-sm font-medium">{t('runtime.endUsers.trend')}</h3>
                  {series.isLoading ? (
                    <Skeleton className="h-44 w-full" />
                  ) : (
                    <UsageTrendChart series={series.data} appNames={appNames} height={176} />
                  )}
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
                          {quotaWindows.map(window => (
                            <UsageMeter
                              key={window}
                              className="w-24"
                              usage={app.usage.find(item => item.window === window)}
                            />
                          ))}
                        </div>
                      ))}
                    </div>
                  )}
                </section>
              </TabsContent>

              <TabsContent value="quota" className="mt-5 space-y-6">
                {!erased && (plans.length > 0 || user.plan) ? (
                  <section className="space-y-3">
                    <div className="flex items-start justify-between gap-2">
                      <div>
                        <h3 className="text-sm font-medium">{t('runtime.plans.plan')}</h3>
                        <p className="mt-1 text-xs leading-5 text-muted-foreground">
                          {t('runtime.plans.assignDescription')}
                        </p>
                      </div>
                      <PlanBadge plan={user.plan} />
                    </div>
                    {user.plan && !user.plan.active ? (
                      <p className="text-xs text-warning">{t('runtime.plans.inactive')}</p>
                    ) : null}
                    <PlanAssignmentEditor plans={plans} value={planDraft} onChange={setPlanDraft} />
                    <div className="flex justify-end">
                      <Button
                        type="button"
                        size="sm"
                        disabled={saving || !planDirty}
                        onClick={() => void onSavePlan(planOfDraft(planDraft))}
                      >
                        {saving ? <Loader2 className="size-4 animate-spin" /> : null}
                        {t('runtime.plans.save')}
                      </Button>
                    </div>
                  </section>
                ) : null}

                {!erased ? (
                  <section className="space-y-3">
                    <div>
                      <h3 className="text-sm font-medium">{t('runtime.endUsers.ownQuota')}</h3>
                      <p className="mt-1 text-xs leading-5 text-muted-foreground">
                        {t('runtime.endUsers.ownQuotaDescription')}
                      </p>
                    </div>
                    <QuotaOverrideEditor value={draft} onChange={setDraft} />
                    <div className="flex items-center justify-end gap-2">
                      {dirty ? (
                        <Button
                          type="button"
                          size="sm"
                          variant="ghost"
                          className="mr-auto"
                          onClick={() => setDraft(quotaDraftOf(user))}
                        >
                          <RotateCcw className="size-4" />
                          {t('runtime.quota.discard')}
                        </Button>
                      ) : null}
                      <Button
                        type="button"
                        size="sm"
                        disabled={saving || !dirty || quota === undefined}
                        onClick={() => {
                          if (quota !== undefined) void onSaveQuota(quota);
                        }}
                      >
                        {saving ? <Loader2 className="size-4 animate-spin" /> : null}
                        {t('runtime.endUsers.saveQuota')}
                      </Button>
                    </div>
                  </section>
                ) : null}
              </TabsContent>

              {!erased ? (
                <TabsContent value="credits" className="mt-5 space-y-6">
                  <EndUserCreditsSection
                    runtimeWorkspaceId={runtimeWorkspaceId}
                    subjectId={user.subject_id}
                  />
                </TabsContent>
              ) : null}

              <TabsContent value="history" className="mt-5 space-y-6">
                <section className="space-y-2">
                  <h3 className="text-sm font-medium">{t('runtime.endUsers.history')}</h3>
                  {(events.data ?? []).length === 0 ? (
                    <p className="text-xs text-muted-foreground">
                      {t('runtime.endUsers.noHistory')}
                    </p>
                  ) : (
                    <ol className="relative space-y-3 border-l pl-4">
                      {(events.data ?? []).map(event => (
                        <li key={event.id} className="relative text-sm">
                          <span className="absolute -left-[21px] top-1.5 size-2 rounded-full bg-border" />
                          <div className="flex items-start justify-between gap-3">
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
                          </div>
                        </li>
                      ))}
                    </ol>
                  )}
                  {events.hasMore ? (
                    <Button
                      type="button"
                      variant="ghost"
                      size="sm"
                      className="w-full text-muted-foreground"
                      disabled={events.loadingMore}
                      onClick={events.loadMore}
                    >
                      {events.loadingMore ? <Loader2 className="size-4 animate-spin" /> : null}
                      {t('runtime.common.loadMore')}
                    </Button>
                  ) : null}
                </section>
              </TabsContent>
            </Tabs>
          </>
        ) : null}
      </SheetContent>
    </Sheet>
  );
}
